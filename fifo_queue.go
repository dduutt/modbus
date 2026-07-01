package modbus

import "fmt"

func parseFIFOQueueResponse(data []byte) ([]uint16, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("%w: fifo queue response too short", ErrInvalidResponse)
	}
	byteCount := int(uint16At(data[0:]))
	fifoCount := int(uint16At(data[2:]))
	if fifoCount > 31 {
		return nil, fmt.Errorf("%w: fifo count %d exceeds 31", ErrInvalidResponse, fifoCount)
	}
	if byteCount != 2+fifoCount*2 {
		return nil, fmt.Errorf("%w: invalid fifo byte count", ErrInvalidResponse)
	}
	if len(data) != 2+byteCount {
		return nil, fmt.Errorf("%w: fifo queue response length mismatch", ErrInvalidResponse)
	}
	return bytesToRegisters(data[4:]), nil
}

func buildReadFIFOQueueResponse(values []uint16) PDU {
	payload := registersToBytes(values)
	byteCount := 2 + len(payload)
	data := make([]byte, 4+len(payload))
	putUint16(data[0:], uint16(byteCount))
	putUint16(data[2:], uint16(len(values)))
	copy(data[4:], payload)
	return PDU{Function: FuncReadFIFOQueue, Data: data}
}
