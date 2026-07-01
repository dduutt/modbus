package modbus

import (
	"context"
	"errors"
	"io"
)

type RTUServer struct {
	Handler Handler
}

func NewRTUServer(handler Handler) *RTUServer {
	return &RTUServer{Handler: handler}
}

func (s *RTUServer) Serve(ctx context.Context, conn io.ReadWriteCloser) error {
	defer conn.Close()
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		frame, err := readRTURequest(conn)
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
		out, err := RTUCodec{}.Encode(frame.UnitID, 0, resp)
		if err != nil {
			return err
		}
		if _, err := conn.Write(out); err != nil {
			return err
		}
	}
}

func (s *RTUServer) handle(ctx context.Context, unitID byte, req PDU) (PDU, error) {
	if s.Handler == nil {
		return exceptionPDU(req.Function, ExceptionServerDeviceFailure), nil
	}
	return s.Handler.Handle(ctx, unitID, req)
}

func readRTURequest(r io.Reader) (Frame, error) {
	var head [2]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return Frame{}, err
	}
	fn := FunctionCode(head[1])
	var size int
	switch fn {
	case FuncReadCoils, FuncReadDiscreteInputs, FuncReadHoldingRegisters, FuncReadInputRegisters,
		FuncWriteSingleCoil, FuncWriteSingleRegister, FuncDiagnostic:
		size = 8
	case FuncReadExceptionStatus, FuncGetCommEventCounter, FuncGetCommEventLog, FuncReportServerID:
		size = 4
	case FuncMaskWriteRegister:
		size = 10
	case FuncReadFIFOQueue:
		size = 6
	case FuncReadDeviceIdentification:
		size = 7
	case FuncReadFileRecord, FuncWriteFileRecord:
		var byteCount [1]byte
		if _, err := io.ReadFull(r, byteCount[:]); err != nil {
			return Frame{}, err
		}
		buf := make([]byte, 3+int(byteCount[0])+2)
		copy(buf[0:2], head[:])
		buf[2] = byteCount[0]
		if _, err := io.ReadFull(r, buf[3:]); err != nil {
			return Frame{}, err
		}
		return RTUCodec{}.DecodeFrame(buf)
	case FuncWriteMultipleCoils, FuncWriteMultipleRegisters, FuncReadWriteMultipleRegisters:
		fixedLen := 7
		byteCountIndex := 6
		if fn == FuncReadWriteMultipleRegisters {
			fixedLen = 11
			byteCountIndex = 10
		}
		fixed := make([]byte, fixedLen)
		copy(fixed[0:2], head[:])
		if _, err := io.ReadFull(r, fixed[2:]); err != nil {
			return Frame{}, err
		}
		byteCount := int(fixed[byteCountIndex])
		buf := make([]byte, fixedLen+byteCount+2)
		copy(buf, fixed[:])
		if _, err := io.ReadFull(r, buf[fixedLen:]); err != nil {
			return Frame{}, err
		}
		return RTUCodec{}.DecodeFrame(buf)
	default:
		size = 8
	}
	buf := make([]byte, size)
	copy(buf, head[:])
	if _, err := io.ReadFull(r, buf[2:]); err != nil {
		return Frame{}, err
	}
	return RTUCodec{}.DecodeFrame(buf)
}
