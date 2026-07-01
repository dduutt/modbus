package modbus

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"
)

type RTUTransport struct {
	conn         io.ReadWriteCloser
	timeout      time.Duration
	timing       RTUTiming
	charTime     time.Duration
	frameGap     time.Duration
	codec        RTUCodec
	mu           sync.Mutex
	closed       bool
	lastActivity time.Time
}

func NewRTUTransport(conn io.ReadWriteCloser, opts ...RTUOption) *RTUTransport {
	cfg := rtuOptions{timeout: 2 * time.Second}
	for _, opt := range opts {
		opt(&cfg)
	}
	charTime, frameGap := rtuTimingDurations(cfg.timing)
	return &RTUTransport{
		conn:     conn,
		timeout:  cfg.timeout,
		timing:   cfg.timing,
		charTime: charTime,
		frameGap: frameGap,
		codec:    RTUCodec{},
	}
}

func (t *RTUTransport) Do(ctx context.Context, unitID byte, request PDU) (PDU, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return PDU{}, ErrClosed
	}
	if dc, ok := t.conn.(deadlineConn); ok {
		if err := setConnDeadline(ctx, dc, t.timeout); err != nil {
			return PDU{}, err
		}
	}
	req, err := t.codec.Encode(unitID, 0, request)
	if err != nil {
		return PDU{}, err
	}
	if err := t.waitBeforeWrite(ctx); err != nil {
		return PDU{}, err
	}
	if t.timing.PreDelay > 0 {
		if err := sleepContext(ctx, t.timing.PreDelay); err != nil {
			return PDU{}, err
		}
	}
	started := time.Now()
	if _, err := t.conn.Write(req); err != nil {
		return PDU{}, err
	}
	t.markWriteActivity(started, len(req))
	if err := t.waitAfterWrite(ctx, len(req), expectedRTUResponseLength(request)); err != nil {
		return PDU{}, err
	}
	frame, err := readRTUFrame(t.conn, unitID, request.Function)
	if err != nil {
		return PDU{}, err
	}
	if frame.UnitID != unitID {
		return PDU{}, fmt.Errorf("%w: unit id mismatch", ErrInvalidResponse)
	}
	return frame.PDU, nil
}

func (t *RTUTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	return t.conn.Close()
}

func (t *RTUTransport) waitBeforeWrite(ctx context.Context) error {
	if t.frameGap <= 0 || t.lastActivity.IsZero() {
		return nil
	}
	wait := time.Until(t.lastActivity.Add(t.frameGap))
	if wait <= 0 {
		return nil
	}
	return sleepContext(ctx, wait)
}

func (t *RTUTransport) waitAfterWrite(ctx context.Context, requestLen, responseLen int) error {
	wait := t.timing.PostDelay + t.timing.TurnaroundDelay
	if t.charTime > 0 {
		// References either wait for request transmission + frame gap or
		// request+response estimate. Keep this configurable behavior conservative.
		wait += time.Duration(requestLen)*t.charTime + t.frameGap
		_ = responseLen
	}
	if wait <= 0 {
		return nil
	}
	return sleepContext(ctx, wait)
}

func (t *RTUTransport) markWriteActivity(started time.Time, bytesWritten int) {
	if t.charTime > 0 {
		t.lastActivity = started.Add(time.Duration(bytesWritten) * t.charTime)
		return
	}
	t.lastActivity = time.Now()
}

func rtuTimingDurations(timing RTUTiming) (time.Duration, time.Duration) {
	if timing.BaudRate <= 0 {
		return 0, 0
	}
	dataBits := timing.DataBits
	if dataBits == 0 {
		dataBits = 8
	}
	stopBits := timing.StopBits
	if stopBits == 0 {
		stopBits = 1
	}
	bitsPerChar := 1 + dataBits + stopBits
	if timing.Parity {
		bitsPerChar++
	}
	charTime := time.Duration(bitsPerChar) * time.Second / time.Duration(timing.BaudRate)
	if timing.BaudRate > 19200 {
		return charTime, 1750 * time.Microsecond
	}
	return charTime, charTime * 35 / 10
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func expectedRTUResponseLength(request PDU) int {
	length := 4
	switch request.Function {
	case FuncReadCoils, FuncReadDiscreteInputs:
		if len(request.Data) >= 4 {
			quantity := uint16At(request.Data[2:])
			length += 1 + int((quantity+7)/8)
		}
	case FuncReadHoldingRegisters, FuncReadInputRegisters:
		if len(request.Data) >= 4 {
			quantity := uint16At(request.Data[2:])
			length += 1 + int(quantity)*2
		}
	case FuncWriteSingleCoil, FuncWriteSingleRegister, FuncWriteMultipleCoils, FuncWriteMultipleRegisters:
		length += 4
	case FuncReadExceptionStatus:
		length += 1
	case FuncDiagnostic, FuncGetCommEventCounter:
		length += 4
	case FuncMaskWriteRegister:
		length += 6
	case FuncReadWriteMultipleRegisters:
		if len(request.Data) >= 4 {
			quantity := uint16At(request.Data[2:])
			length += 1 + int(quantity)*2
		}
	case FuncReadFileRecord, FuncWriteFileRecord:
		// Variable length; the scanner reads the one-byte byte count.
	case FuncGetCommEventLog:
		// Variable length; the scanner reads the one-byte byte count.
	case FuncReportServerID:
		// Variable length; the scanner reads the one-byte byte count.
	case FuncReadFIFOQueue:
		// Variable length; the scanner reads the 16-bit byte count.
	case FuncReadDeviceIdentification:
		// Variable length; the scanner reads object headers to determine the frame.
	}
	return length
}

func readRTUFrame(r io.Reader, unitID byte, expected FunctionCode) (Frame, error) {
	return newRTUFrameScanner(r, unitID, expected).Read()
}

type rtuFrameScanner struct {
	reader   io.Reader
	unitID   byte
	expected FunctionCode
	codec    RTUCodec
}

func newRTUFrameScanner(r io.Reader, unitID byte, expected FunctionCode) *rtuFrameScanner {
	return &rtuFrameScanner{
		reader:   r,
		unitID:   unitID,
		expected: expected,
		codec:    RTUCodec{},
	}
}

func (s *rtuFrameScanner) Read() (Frame, error) {
	var b [1]byte
	for {
		if _, err := io.ReadFull(s.reader, b[:]); err != nil {
			return Frame{}, err
		}
		if b[0] != s.unitID {
			continue
		}
		frame, retry, err := s.readCandidate(b[0])
		if retry {
			continue
		}
		if err != nil {
			return Frame{}, err
		}
		return frame, nil
	}
}

func (s *rtuFrameScanner) readCandidate(unitID byte) (Frame, bool, error) {
	var fnBuf [1]byte
	if _, err := io.ReadFull(s.reader, fnBuf[:]); err != nil {
		return Frame{}, false, err
	}
	fn := FunctionCode(fnBuf[0])
	if fn != s.expected && fn != s.expected|0x80 {
		return Frame{}, true, nil
	}

	frameLen, firstPayload, err := s.responseFrameShape(fn)
	if err != nil {
		return Frame{}, false, err
	}
	buf := make([]byte, frameLen)
	buf[0] = unitID
	buf[1] = byte(fn)
	copy(buf[2:], firstPayload)
	if _, err := io.ReadFull(s.reader, buf[2+len(firstPayload):]); err != nil {
		return Frame{}, false, err
	}
	frame, err := s.codec.DecodeFrame(buf)
	if err != nil {
		return Frame{}, true, nil
	}
	return frame, false, nil
}

func (s *rtuFrameScanner) responseFrameShape(fn FunctionCode) (int, []byte, error) {
	if isExceptionFunction(fn) {
		return 5, nil, nil
	}
	switch fn {
	case FuncReadCoils, FuncReadDiscreteInputs, FuncReadHoldingRegisters, FuncReadInputRegisters,
		FuncReadWriteMultipleRegisters, FuncReadFileRecord, FuncWriteFileRecord, FuncGetCommEventLog,
		FuncReportServerID:
		var byteCount [1]byte
		if _, err := io.ReadFull(s.reader, byteCount[:]); err != nil {
			return 0, nil, err
		}
		if byteCount[0] == 0 || byteCount[0] > maxPDUSize-2 {
			return 0, nil, fmt.Errorf("%w: invalid rtu byte count %d", ErrInvalidResponse, byteCount[0])
		}
		return 3 + int(byteCount[0]) + 2, byteCount[:], nil
	case FuncWriteSingleCoil, FuncWriteSingleRegister, FuncWriteMultipleCoils, FuncWriteMultipleRegisters:
		return 8, nil, nil
	case FuncReadExceptionStatus:
		return 5, nil, nil
	case FuncDiagnostic, FuncGetCommEventCounter:
		return 8, nil, nil
	case FuncMaskWriteRegister:
		return 10, nil, nil
	case FuncReadFIFOQueue:
		return s.readFIFOQueueResponseShape()
	case FuncReadDeviceIdentification:
		return s.readDeviceIdentificationResponseShape()
	default:
		return 0, nil, fmt.Errorf("%w: unsupported rtu response function 0x%02x", ErrInvalidResponse, fn)
	}
}

func (s *rtuFrameScanner) readFIFOQueueResponseShape() (int, []byte, error) {
	var header [2]byte
	if _, err := io.ReadFull(s.reader, header[:]); err != nil {
		return 0, nil, err
	}
	byteCount := int(uint16At(header[:]))
	if byteCount < 2 || byteCount > maxPDUSize-3 {
		return 0, nil, fmt.Errorf("%w: invalid fifo byte count %d", ErrInvalidResponse, byteCount)
	}
	if byteCount%2 != 0 {
		return 0, nil, fmt.Errorf("%w: odd fifo byte count %d", ErrInvalidResponse, byteCount)
	}
	return 2 + len(header) + byteCount + 2, header[:], nil
}

func (s *rtuFrameScanner) readDeviceIdentificationResponseShape() (int, []byte, error) {
	var header [6]byte
	if _, err := io.ReadFull(s.reader, header[:]); err != nil {
		return 0, nil, err
	}
	if header[0] != meiTypeReadDeviceIdentification {
		return 0, nil, fmt.Errorf("%w: unexpected mei type 0x%02x", ErrInvalidResponse, header[0])
	}
	if header[3] != moreFollowsNo && header[3] != moreFollowsYes {
		return 0, nil, fmt.Errorf("%w: invalid more-follows value 0x%02x", ErrInvalidResponse, header[3])
	}
	payload := append([]byte(nil), header[:]...)
	objectCount := int(header[5])
	for i := 0; i < objectCount; i++ {
		var objectHeader [2]byte
		if _, err := io.ReadFull(s.reader, objectHeader[:]); err != nil {
			return 0, nil, err
		}
		payload = append(payload, objectHeader[:]...)
		objectLen := int(objectHeader[1])
		if len(payload)+objectLen > maxPDUSize-1 {
			return 0, nil, fmt.Errorf("%w: device identification response too large", ErrInvalidResponse)
		}
		value := make([]byte, objectLen)
		if _, err := io.ReadFull(s.reader, value); err != nil {
			return 0, nil, err
		}
		payload = append(payload, value...)
	}
	return 2 + len(payload) + 2, payload, nil
}
