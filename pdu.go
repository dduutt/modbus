package modbus

import "fmt"

type FunctionCode byte

const (
	FuncReadCoils                  FunctionCode = 0x01
	FuncReadDiscreteInputs         FunctionCode = 0x02
	FuncReadHoldingRegisters       FunctionCode = 0x03
	FuncReadInputRegisters         FunctionCode = 0x04
	FuncWriteSingleCoil            FunctionCode = 0x05
	FuncWriteSingleRegister        FunctionCode = 0x06
	FuncReadExceptionStatus        FunctionCode = 0x07
	FuncDiagnostic                 FunctionCode = 0x08
	FuncGetCommEventCounter        FunctionCode = 0x0B
	FuncGetCommEventLog            FunctionCode = 0x0C
	FuncWriteMultipleCoils         FunctionCode = 0x0F
	FuncWriteMultipleRegisters     FunctionCode = 0x10
	FuncReportServerID             FunctionCode = 0x11
	FuncReadFileRecord             FunctionCode = 0x14
	FuncWriteFileRecord            FunctionCode = 0x15
	FuncMaskWriteRegister          FunctionCode = 0x16
	FuncReadWriteMultipleRegisters FunctionCode = 0x17
	FuncReadFIFOQueue              FunctionCode = 0x18
	FuncReadDeviceIdentification   FunctionCode = 0x2B
)

type PDU struct {
	Function FunctionCode
	Data     []byte
}

type Frame struct {
	TransactionID uint16
	UnitID        byte
	PDU           PDU
}

func (p PDU) Bytes() []byte {
	out := make([]byte, 1+len(p.Data))
	out[0] = byte(p.Function)
	copy(out[1:], p.Data)
	return out
}

func ParsePDU(b []byte) (PDU, error) {
	if len(b) < 1 {
		return PDU{}, ErrInvalidResponse
	}
	p := PDU{Function: FunctionCode(b[0])}
	if len(b) > 1 {
		p.Data = append([]byte(nil), b[1:]...)
	}
	return p, nil
}

func exceptionPDU(fn FunctionCode, code ExceptionCode) PDU {
	return PDU{Function: fn | 0x80, Data: []byte{byte(code)}}
}

func parseException(pdu PDU, expected FunctionCode) error {
	if pdu.Function == expected {
		return nil
	}
	if pdu.Function == expected|0x80 && len(pdu.Data) == 1 {
		return &ExceptionError{Function: expected, Code: ExceptionCode(pdu.Data[0])}
	}
	return fmt.Errorf("%w: expected function 0x%02x got 0x%02x", ErrInvalidResponse, expected, pdu.Function)
}

func isExceptionFunction(fn FunctionCode) bool {
	return fn&0x80 != 0
}

func baseFunction(fn FunctionCode) FunctionCode {
	return fn & 0x7F
}

func putUint16(b []byte, v uint16) {
	b[0] = byte(v >> 8)
	b[1] = byte(v)
}

func uint16At(b []byte) uint16 {
	return uint16(b[0])<<8 | uint16(b[1])
}

func quantityRange(quantity, max uint16) error {
	if quantity == 0 || quantity > max {
		return fmt.Errorf("%w: quantity must be 1..%d", ErrInvalidRequest, max)
	}
	return nil
}
