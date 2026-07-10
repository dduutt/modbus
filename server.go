package modbus

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
)

type Handler interface {
	Handle(ctx context.Context, unitID byte, request PDU) (PDU, error)
}

type DataStoreHandler struct {
	Store                               DataStore
	FileRecords                         map[uint16][]uint16
	FIFOQueues                          map[uint16][]uint16
	ExceptionStatus                     *byte
	EnableDiagnostics                   bool
	DiagnosticResponses                 map[uint16]uint16
	CommEventCounter                    *CommEventCounter
	CommEventLog                        *CommEventLog
	ServerID                            []byte
	DeviceIdentification                map[byte][]byte
	DeviceIdentificationConformityLevel byte
}

func NewDataStoreHandler(store DataStore) *DataStoreHandler {
	return &DataStoreHandler{Store: store}
}

func (h *DataStoreHandler) Handle(_ context.Context, _ byte, request PDU) (PDU, error) {
	if h.Store == nil && request.Function != FuncReadDeviceIdentification && request.Function != FuncReadFIFOQueue &&
		request.Function != FuncReadFileRecord && request.Function != FuncWriteFileRecord &&
		request.Function != FuncReadExceptionStatus && request.Function != FuncDiagnostic &&
		request.Function != FuncGetCommEventCounter && request.Function != FuncGetCommEventLog &&
		request.Function != FuncReportServerID {
		return exceptionPDU(baseFunction(request.Function), ExceptionServerDeviceFailure), nil
	}
	switch request.Function {
	case FuncReadCoils:
		return h.handleReadBits(request, h.Store.ReadCoils, 2000)
	case FuncReadDiscreteInputs:
		return h.handleReadBits(request, h.Store.ReadDiscreteInputs, 2000)
	case FuncReadHoldingRegisters:
		return h.handleReadRegisters(request, h.Store.ReadHoldingRegisters, 125)
	case FuncReadInputRegisters:
		return h.handleReadRegisters(request, h.Store.ReadInputRegisters, 125)
	case FuncWriteSingleCoil:
		return h.handleWriteSingleCoil(request)
	case FuncWriteSingleRegister:
		return h.handleWriteSingleRegister(request)
	case FuncReadExceptionStatus:
		return h.handleReadExceptionStatus(request)
	case FuncDiagnostic:
		return h.handleDiagnostic(request)
	case FuncGetCommEventCounter:
		return h.handleGetCommEventCounter(request)
	case FuncGetCommEventLog:
		return h.handleGetCommEventLog(request)
	case FuncReportServerID:
		return h.handleReportServerID(request)
	case FuncWriteMultipleCoils:
		return h.handleWriteMultipleCoils(request)
	case FuncWriteMultipleRegisters:
		return h.handleWriteMultipleRegisters(request)
	case FuncMaskWriteRegister:
		return h.handleMaskWriteRegister(request)
	case FuncReadWriteMultipleRegisters:
		return h.handleReadWriteMultipleRegisters(request)
	case FuncReadFileRecord:
		return h.handleReadFileRecord(request)
	case FuncWriteFileRecord:
		return h.handleWriteFileRecord(request)
	case FuncReadFIFOQueue:
		return h.handleReadFIFOQueue(request)
	case FuncReadDeviceIdentification:
		return h.handleReadDeviceIdentification(request)
	default:
		return exceptionPDU(request.Function, ExceptionIllegalFunction), nil
	}
}

func (h *DataStoreHandler) handleReadBits(request PDU, read func(uint16, uint16) ([]bool, error), max uint16) (PDU, error) {
	if len(request.Data) != 4 {
		return exceptionPDU(request.Function, ExceptionIllegalDataValue), nil
	}
	address, quantity := uint16At(request.Data[0:]), uint16At(request.Data[2:])
	if quantity == 0 || quantity > max {
		return exceptionPDU(request.Function, ExceptionIllegalDataValue), nil
	}
	values, err := read(address, quantity)
	if err != nil {
		return exceptionPDU(request.Function, ExceptionIllegalDataAddress), nil
	}
	payload := packBits(values)
	data := make([]byte, 1+len(payload))
	data[0] = byte(len(payload))
	copy(data[1:], payload)
	return PDU{Function: request.Function, Data: data}, nil
}

func (h *DataStoreHandler) handleReadRegisters(request PDU, read func(uint16, uint16) ([]uint16, error), max uint16) (PDU, error) {
	if len(request.Data) != 4 {
		return exceptionPDU(request.Function, ExceptionIllegalDataValue), nil
	}
	address, quantity := uint16At(request.Data[0:]), uint16At(request.Data[2:])
	if quantity == 0 || quantity > max {
		return exceptionPDU(request.Function, ExceptionIllegalDataValue), nil
	}
	values, err := read(address, quantity)
	if err != nil {
		return exceptionPDU(request.Function, ExceptionIllegalDataAddress), nil
	}
	payload := registersToBytes(values)
	data := make([]byte, 1+len(payload))
	data[0] = byte(len(payload))
	copy(data[1:], payload)
	return PDU{Function: request.Function, Data: data}, nil
}

func (h *DataStoreHandler) handleWriteSingleCoil(request PDU) (PDU, error) {
	if len(request.Data) != 4 {
		return exceptionPDU(request.Function, ExceptionIllegalDataValue), nil
	}
	address, raw := uint16At(request.Data[0:]), uint16At(request.Data[2:])
	var value bool
	switch raw {
	case 0xFF00:
		value = true
	case 0x0000:
		value = false
	default:
		return exceptionPDU(request.Function, ExceptionIllegalDataValue), nil
	}
	if err := h.Store.WriteCoils(address, []bool{value}); err != nil {
		return exceptionPDU(request.Function, ExceptionIllegalDataAddress), nil
	}
	return request, nil
}

func (h *DataStoreHandler) handleWriteSingleRegister(request PDU) (PDU, error) {
	if len(request.Data) != 4 {
		return exceptionPDU(request.Function, ExceptionIllegalDataValue), nil
	}
	address, value := uint16At(request.Data[0:]), uint16At(request.Data[2:])
	if err := h.Store.WriteHoldingRegisters(address, []uint16{value}); err != nil {
		return exceptionPDU(request.Function, ExceptionIllegalDataAddress), nil
	}
	return request, nil
}

func (h *DataStoreHandler) handleWriteMultipleCoils(request PDU) (PDU, error) {
	if len(request.Data) < 5 {
		return exceptionPDU(request.Function, ExceptionIllegalDataValue), nil
	}
	address, quantity := uint16At(request.Data[0:]), uint16At(request.Data[2:])
	byteCount := int(request.Data[4])
	if quantity == 0 || quantity > 1968 || len(request.Data) != 5+byteCount || byteCount != int((quantity+7)/8) {
		return exceptionPDU(request.Function, ExceptionIllegalDataValue), nil
	}
	values := unpackBits(request.Data[5:], quantity)
	if err := h.Store.WriteCoils(address, values); err != nil {
		return exceptionPDU(request.Function, ExceptionIllegalDataAddress), nil
	}
	return writeMultipleResponse(request.Function, address, quantity), nil
}

func (h *DataStoreHandler) handleWriteMultipleRegisters(request PDU) (PDU, error) {
	if len(request.Data) < 5 {
		return exceptionPDU(request.Function, ExceptionIllegalDataValue), nil
	}
	address, quantity := uint16At(request.Data[0:]), uint16At(request.Data[2:])
	byteCount := int(request.Data[4])
	if quantity == 0 || quantity > 123 || len(request.Data) != 5+byteCount || byteCount != int(quantity*2) {
		return exceptionPDU(request.Function, ExceptionIllegalDataValue), nil
	}
	values := bytesToRegisters(request.Data[5:])
	if err := h.Store.WriteHoldingRegisters(address, values); err != nil {
		return exceptionPDU(request.Function, ExceptionIllegalDataAddress), nil
	}
	return writeMultipleResponse(request.Function, address, quantity), nil
}

func (h *DataStoreHandler) handleReadWriteMultipleRegisters(request PDU) (PDU, error) {
	if len(request.Data) < 9 {
		return exceptionPDU(request.Function, ExceptionIllegalDataValue), nil
	}
	readAddress, readQuantity := uint16At(request.Data[0:]), uint16At(request.Data[2:])
	writeAddress, writeQuantity := uint16At(request.Data[4:]), uint16At(request.Data[6:])
	byteCount := int(request.Data[8])
	if readQuantity == 0 || readQuantity > 125 || writeQuantity == 0 || writeQuantity > 121 {
		return exceptionPDU(request.Function, ExceptionIllegalDataValue), nil
	}
	if len(request.Data) != 9+byteCount || byteCount != int(writeQuantity*2) {
		return exceptionPDU(request.Function, ExceptionIllegalDataValue), nil
	}
	values := bytesToRegisters(request.Data[9:])
	if err := h.Store.WriteHoldingRegisters(writeAddress, values); err != nil {
		return exceptionPDU(request.Function, ExceptionIllegalDataAddress), nil
	}
	readValues, err := h.Store.ReadHoldingRegisters(readAddress, readQuantity)
	if err != nil {
		return exceptionPDU(request.Function, ExceptionIllegalDataAddress), nil
	}
	payload := registersToBytes(readValues)
	data := make([]byte, 1+len(payload))
	data[0] = byte(len(payload))
	copy(data[1:], payload)
	return PDU{Function: request.Function, Data: data}, nil
}

func (h *DataStoreHandler) handleMaskWriteRegister(request PDU) (PDU, error) {
	if len(request.Data) != 6 {
		return exceptionPDU(request.Function, ExceptionIllegalDataValue), nil
	}
	address := uint16At(request.Data[0:])
	andMask := uint16At(request.Data[2:])
	orMask := uint16At(request.Data[4:])
	values, err := h.Store.ReadHoldingRegisters(address, 1)
	if err != nil {
		return exceptionPDU(request.Function, ExceptionIllegalDataAddress), nil
	}
	newValue := (values[0] & andMask) | (orMask & ^andMask)
	if err := h.Store.WriteHoldingRegisters(address, []uint16{newValue}); err != nil {
		return exceptionPDU(request.Function, ExceptionIllegalDataAddress), nil
	}
	return request, nil
}

func (h *DataStoreHandler) handleReadExceptionStatus(request PDU) (PDU, error) {
	if h.ExceptionStatus == nil {
		return exceptionPDU(request.Function, ExceptionIllegalFunction), nil
	}
	if len(request.Data) != 0 {
		return exceptionPDU(request.Function, ExceptionIllegalDataValue), nil
	}
	return PDU{Function: request.Function, Data: []byte{*h.ExceptionStatus}}, nil
}

func (h *DataStoreHandler) handleDiagnostic(request PDU) (PDU, error) {
	if !h.EnableDiagnostics {
		return exceptionPDU(request.Function, ExceptionIllegalFunction), nil
	}
	subFunction, data, err := parseDiagnosticData(request.Data)
	if err != nil {
		return exceptionPDU(request.Function, ExceptionIllegalDataValue), nil
	}
	if h.DiagnosticResponses != nil {
		if configured, ok := h.DiagnosticResponses[subFunction]; ok {
			data = configured
		}
	}
	return PDU{Function: request.Function, Data: buildDiagnosticData(subFunction, data)}, nil
}

func (h *DataStoreHandler) handleGetCommEventCounter(request PDU) (PDU, error) {
	if h.CommEventCounter == nil {
		return exceptionPDU(request.Function, ExceptionIllegalFunction), nil
	}
	if len(request.Data) != 0 {
		return exceptionPDU(request.Function, ExceptionIllegalDataValue), nil
	}
	return PDU{Function: request.Function, Data: buildCommEventCounterData(*h.CommEventCounter)}, nil
}

func (h *DataStoreHandler) handleGetCommEventLog(request PDU) (PDU, error) {
	if h.CommEventLog == nil {
		return exceptionPDU(request.Function, ExceptionIllegalFunction), nil
	}
	if len(request.Data) != 0 {
		return exceptionPDU(request.Function, ExceptionIllegalDataValue), nil
	}
	data, err := buildCommEventLogData(*h.CommEventLog)
	if err != nil {
		return exceptionPDU(request.Function, ExceptionIllegalDataValue), nil
	}
	return PDU{Function: request.Function, Data: data}, nil
}

func (h *DataStoreHandler) handleReportServerID(request PDU) (PDU, error) {
	if h.ServerID == nil {
		return exceptionPDU(request.Function, ExceptionIllegalFunction), nil
	}
	if len(request.Data) != 0 {
		return exceptionPDU(request.Function, ExceptionIllegalDataValue), nil
	}
	data, err := buildReportServerIDData(h.ServerID)
	if err != nil {
		return exceptionPDU(request.Function, ExceptionIllegalDataValue), nil
	}
	return PDU{Function: request.Function, Data: data}, nil
}

func (h *DataStoreHandler) handleReadFileRecord(request PDU) (PDU, error) {
	if h.FileRecords == nil {
		return exceptionPDU(request.Function, ExceptionIllegalFunction), nil
	}
	items, err := parseReadFileRecordRequestData(request.Data)
	if err != nil {
		return exceptionPDU(request.Function, ExceptionIllegalDataValue), nil
	}
	records := make([]FileRecord, 0, len(items))
	for _, item := range items {
		file, ok := h.FileRecords[item.FileNumber]
		if !ok {
			return exceptionPDU(request.Function, ExceptionIllegalDataAddress), nil
		}
		start := int(item.RecordNumber)
		end := start + int(item.RecordLength)
		if start > len(file) || end > len(file) {
			return exceptionPDU(request.Function, ExceptionIllegalDataAddress), nil
		}
		records = append(records, FileRecord{
			ReferenceType: item.ReferenceType,
			FileNumber:    item.FileNumber,
			RecordNumber:  item.RecordNumber,
			Values:        append([]uint16(nil), file[start:end]...),
		})
	}
	data, err := buildReadFileRecordResponseData(records)
	if err != nil {
		return exceptionPDU(request.Function, ExceptionIllegalDataValue), nil
	}
	return PDU{Function: request.Function, Data: data}, nil
}

func (h *DataStoreHandler) handleWriteFileRecord(request PDU) (PDU, error) {
	if h.FileRecords == nil {
		return exceptionPDU(request.Function, ExceptionIllegalFunction), nil
	}
	records, err := parseWriteFileRecordData(request.Data)
	if err != nil {
		return exceptionPDU(request.Function, ExceptionIllegalDataValue), nil
	}
	for _, record := range records {
		file, ok := h.FileRecords[record.FileNumber]
		if !ok {
			return exceptionPDU(request.Function, ExceptionIllegalDataAddress), nil
		}
		start := int(record.RecordNumber)
		end := start + len(record.Values)
		if start > len(file) || end > len(file) {
			return exceptionPDU(request.Function, ExceptionIllegalDataAddress), nil
		}
		copy(file[start:end], record.Values)
	}
	return request, nil
}

func (h *DataStoreHandler) handleReadFIFOQueue(request PDU) (PDU, error) {
	if h.FIFOQueues == nil {
		return exceptionPDU(request.Function, ExceptionIllegalFunction), nil
	}
	if len(request.Data) != 2 {
		return exceptionPDU(request.Function, ExceptionIllegalDataValue), nil
	}
	address := uint16At(request.Data)
	values, ok := h.FIFOQueues[address]
	if !ok {
		return exceptionPDU(request.Function, ExceptionIllegalDataAddress), nil
	}
	if len(values) > 31 {
		return exceptionPDU(request.Function, ExceptionIllegalDataValue), nil
	}
	return buildReadFIFOQueueResponse(values), nil
}

func (h *DataStoreHandler) handleReadDeviceIdentification(request PDU) (PDU, error) {
	if h.DeviceIdentification == nil {
		return exceptionPDU(request.Function, ExceptionIllegalFunction), nil
	}
	if len(request.Data) != 3 || request.Data[0] != meiTypeReadDeviceIdentification {
		return exceptionPDU(request.Function, ExceptionIllegalDataValue), nil
	}
	code := ReadDeviceIDCode(request.Data[1])
	objectID := request.Data[2]
	objects, moreFollows, nextObjectID, err := h.deviceIdentificationObjects(code, objectID)
	if err != nil {
		return exceptionPDU(request.Function, ExceptionIllegalDataValue), nil
	}
	if len(objects) == 0 {
		return exceptionPDU(request.Function, ExceptionIllegalDataAddress), nil
	}
	resp, err := buildDeviceIdentificationResponse(code, h.DeviceIdentificationConformityLevel, moreFollows, nextObjectID, objects)
	if err != nil {
		return exceptionPDU(request.Function, ExceptionIllegalDataValue), nil
	}
	return resp, nil
}

func (h *DataStoreHandler) deviceIdentificationObjects(code ReadDeviceIDCode, objectID byte) (map[byte][]byte, bool, byte, error) {
	switch code {
	case ReadDeviceIDCodeBasic, ReadDeviceIDCodeRegular, ReadDeviceIDCodeExtended, ReadDeviceIDCodeSpecific:
	default:
		return nil, false, 0, fmt.Errorf("%w: unsupported read device id code 0x%02x", ErrInvalidRequest, byte(code))
	}

	selected := make(map[byte][]byte)
	dataLen := 6
	for _, id := range sortedDeviceIdentificationObjectIDs(h.DeviceIdentification) {
		if !deviceIdentificationObjectMatches(code, objectID, id) {
			continue
		}
		value := h.DeviceIdentification[id]
		if len(value) > maxPDUSize-1-6-2 {
			return nil, false, 0, fmt.Errorf("%w: device identification object 0x%02x too large", ErrInvalidRequest, id)
		}
		nextLen := dataLen + 2 + len(value)
		if nextLen > maxPDUSize-1 {
			return selected, true, id, nil
		}
		selected[id] = append([]byte(nil), value...)
		dataLen = nextLen
		if code == ReadDeviceIDCodeSpecific {
			break
		}
	}
	return selected, false, 0, nil
}

func deviceIdentificationObjectMatches(code ReadDeviceIDCode, requested, id byte) bool {
	if id < requested {
		return false
	}
	switch code {
	case ReadDeviceIDCodeBasic:
		return id <= 0x02
	case ReadDeviceIDCodeRegular:
		return id >= 0x03 && id <= 0x7F
	case ReadDeviceIDCodeExtended:
		return id >= 0x80
	case ReadDeviceIDCodeSpecific:
		return id == requested
	default:
		return false
	}
}

func writeMultipleResponse(fn FunctionCode, address, quantity uint16) PDU {
	data := make([]byte, 4)
	putUint16(data[0:], address)
	putUint16(data[2:], quantity)
	return PDU{Function: fn, Data: data}
}

type TCPServer struct {
	Handler Handler
}

func NewTCPServer(handler Handler) *TCPServer {
	return &TCPServer{Handler: handler}
}

func (s *TCPServer) ListenAndServe(ctx context.Context, address string) error {
	ln, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	return s.Serve(ctx, ln)
}

func (s *TCPServer) Serve(ctx context.Context, ln net.Listener) error {
	var wg sync.WaitGroup
	errCh := make(chan error, 1)
	conns := make(map[net.Conn]struct{})
	var connsMu sync.Mutex
	closeActiveConns := func() {
		connsMu.Lock()
		for conn := range conns {
			_ = conn.Close()
		}
		connsMu.Unlock()
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = ln.Close()
			closeActiveConns()
		case <-done:
		}
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			closeActiveConns()
			if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
				wg.Wait()
				return nil
			}
			select {
			case errCh <- err:
			default:
			}
			wg.Wait()
			return <-errCh
		}
		connsMu.Lock()
		conns[conn] = struct{}{}
		connsMu.Unlock()
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				connsMu.Lock()
				delete(conns, conn)
				connsMu.Unlock()
			}()
			_ = s.serveConn(ctx, conn)
		}()
	}
}

func (s *TCPServer) serveConn(ctx context.Context, conn net.Conn) error {
	defer conn.Close()
	codec := TCPCodec{}
	for {
		frame, err := codec.Decode(conn)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		resp, err := s.handle(ctx, frame.UnitID, frame.PDU)
		if err != nil {
			return err
		}
		out, err := codec.Encode(frame.UnitID, frame.TransactionID, resp)
		if err != nil {
			return err
		}
		if _, err := conn.Write(out); err != nil {
			return err
		}
	}
}

func (s *TCPServer) handle(ctx context.Context, unitID byte, req PDU) (PDU, error) {
	if s.Handler == nil {
		return PDU{}, fmt.Errorf("%w: nil handler", ErrInvalidRequest)
	}
	return s.Handler.Handle(ctx, unitID, req)
}
