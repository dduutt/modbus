package modbus

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidResponse = errors.New("modbus: invalid response")
	ErrInvalidRequest  = errors.New("modbus: invalid request")
	ErrTimeout         = errors.New("modbus: timeout")
	ErrClosed          = errors.New("modbus: closed")
)

type ExceptionCode byte

const (
	ExceptionIllegalFunction                    ExceptionCode = 0x01
	ExceptionIllegalDataAddress                 ExceptionCode = 0x02
	ExceptionIllegalDataValue                   ExceptionCode = 0x03
	ExceptionServerDeviceFailure                ExceptionCode = 0x04
	ExceptionAcknowledge                        ExceptionCode = 0x05
	ExceptionServerDeviceBusy                   ExceptionCode = 0x06
	ExceptionMemoryParityError                  ExceptionCode = 0x08
	ExceptionGatewayPathUnavailable             ExceptionCode = 0x0A
	ExceptionGatewayTargetDeviceFailedToRespond ExceptionCode = 0x0B
)

type ExceptionError struct {
	Function FunctionCode
	Code     ExceptionCode
}

func (e *ExceptionError) Error() string {
	return fmt.Sprintf("modbus: exception function=0x%02x code=0x%02x (%s)", byte(e.Function), byte(e.Code), e.Code)
}

func (c ExceptionCode) String() string {
	switch c {
	case ExceptionIllegalFunction:
		return "illegal function"
	case ExceptionIllegalDataAddress:
		return "illegal data address"
	case ExceptionIllegalDataValue:
		return "illegal data value"
	case ExceptionServerDeviceFailure:
		return "server device failure"
	case ExceptionAcknowledge:
		return "acknowledge"
	case ExceptionServerDeviceBusy:
		return "server device busy"
	case ExceptionMemoryParityError:
		return "memory parity error"
	case ExceptionGatewayPathUnavailable:
		return "gateway path unavailable"
	case ExceptionGatewayTargetDeviceFailedToRespond:
		return "gateway target device failed to respond"
	default:
		return "unknown"
	}
}
