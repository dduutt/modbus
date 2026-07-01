package modbus

import "fmt"

type CommEventCounter struct {
	Status     uint16
	EventCount uint16
}

type CommEventLog struct {
	Status       uint16
	EventCount   uint16
	MessageCount uint16
	Events       []byte
}

func buildDiagnosticData(subFunction, data uint16) []byte {
	out := make([]byte, 4)
	putUint16(out[0:], subFunction)
	putUint16(out[2:], data)
	return out
}

func parseDiagnosticData(data []byte) (uint16, uint16, error) {
	if len(data) != 4 {
		return 0, 0, fmt.Errorf("%w: diagnostic response length mismatch", ErrInvalidResponse)
	}
	return uint16At(data[0:]), uint16At(data[2:]), nil
}

func buildCommEventCounterData(counter CommEventCounter) []byte {
	out := make([]byte, 4)
	putUint16(out[0:], counter.Status)
	putUint16(out[2:], counter.EventCount)
	return out
}

func parseCommEventCounterData(data []byte) (CommEventCounter, error) {
	if len(data) != 4 {
		return CommEventCounter{}, fmt.Errorf("%w: comm event counter response length mismatch", ErrInvalidResponse)
	}
	return CommEventCounter{
		Status:     uint16At(data[0:]),
		EventCount: uint16At(data[2:]),
	}, nil
}

func buildCommEventLogData(log CommEventLog) ([]byte, error) {
	if len(log.Events) > maxPDUSize-2-6 {
		return nil, fmt.Errorf("%w: comm event log too large", ErrInvalidRequest)
	}
	byteCount := 6 + len(log.Events)
	out := make([]byte, 1+byteCount)
	out[0] = byte(byteCount)
	putUint16(out[1:], log.Status)
	putUint16(out[3:], log.EventCount)
	putUint16(out[5:], log.MessageCount)
	copy(out[7:], log.Events)
	return out, nil
}

func parseCommEventLogData(data []byte) (CommEventLog, error) {
	if len(data) < 7 {
		return CommEventLog{}, fmt.Errorf("%w: comm event log response too short", ErrInvalidResponse)
	}
	byteCount := int(data[0])
	if byteCount < 6 || byteCount > maxPDUSize-2 || len(data) != 1+byteCount {
		return CommEventLog{}, fmt.Errorf("%w: invalid comm event log byte count", ErrInvalidResponse)
	}
	return CommEventLog{
		Status:       uint16At(data[1:]),
		EventCount:   uint16At(data[3:]),
		MessageCount: uint16At(data[5:]),
		Events:       append([]byte(nil), data[7:]...),
	}, nil
}

func buildReportServerIDData(value []byte) ([]byte, error) {
	if len(value) > maxPDUSize-2 {
		return nil, fmt.Errorf("%w: report server id too large", ErrInvalidRequest)
	}
	out := make([]byte, 1+len(value))
	out[0] = byte(len(value))
	copy(out[1:], value)
	return out, nil
}

func parseReportServerIDData(data []byte) ([]byte, error) {
	if len(data) < 1 {
		return nil, fmt.Errorf("%w: report server id response too short", ErrInvalidResponse)
	}
	byteCount := int(data[0])
	if byteCount > maxPDUSize-2 || len(data) != 1+byteCount {
		return nil, fmt.Errorf("%w: invalid report server id byte count", ErrInvalidResponse)
	}
	return append([]byte(nil), data[1:]...), nil
}
