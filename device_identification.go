package modbus

import (
	"fmt"
	"sort"
)

const (
	meiTypeReadDeviceIdentification byte = 0x0E

	moreFollowsNo  byte = 0x00
	moreFollowsYes byte = 0xFF
)

type ReadDeviceIDCode byte

const (
	ReadDeviceIDCodeBasic    ReadDeviceIDCode = 0x01
	ReadDeviceIDCodeRegular  ReadDeviceIDCode = 0x02
	ReadDeviceIDCodeExtended ReadDeviceIDCode = 0x03
	ReadDeviceIDCodeSpecific ReadDeviceIDCode = 0x04
)

type DeviceIdentification struct {
	ReadDeviceIDCode ReadDeviceIDCode
	ConformityLevel  byte
	MoreFollows      bool
	NextObjectID     byte
	Objects          map[byte][]byte
}

func startDeviceIdentificationObjectID(code ReadDeviceIDCode) (byte, error) {
	switch code {
	case ReadDeviceIDCodeBasic:
		return 0x00, nil
	case ReadDeviceIDCodeRegular:
		return 0x03, nil
	case ReadDeviceIDCodeExtended:
		return 0x80, nil
	default:
		return 0, fmt.Errorf("%w: unsupported read device id code 0x%02x", ErrInvalidRequest, byte(code))
	}
}

func parseDeviceIdentificationResponse(data []byte) (DeviceIdentification, error) {
	if len(data) < 6 {
		return DeviceIdentification{}, fmt.Errorf("%w: device identification response too short", ErrInvalidResponse)
	}
	if data[0] != meiTypeReadDeviceIdentification {
		return DeviceIdentification{}, fmt.Errorf("%w: unexpected mei type 0x%02x", ErrInvalidResponse, data[0])
	}
	code := ReadDeviceIDCode(data[1])
	moreFollows := data[3]
	if moreFollows != moreFollowsNo && moreFollows != moreFollowsYes {
		return DeviceIdentification{}, fmt.Errorf("%w: invalid more-follows value 0x%02x", ErrInvalidResponse, moreFollows)
	}
	objectCount := int(data[5])
	objects := make(map[byte][]byte, objectCount)
	offset := 6
	for i := 0; i < objectCount; i++ {
		if len(data)-offset < 2 {
			return DeviceIdentification{}, fmt.Errorf("%w: missing device identification object header", ErrInvalidResponse)
		}
		objectID := data[offset]
		objectLen := int(data[offset+1])
		offset += 2
		if len(data)-offset < objectLen {
			return DeviceIdentification{}, fmt.Errorf("%w: device identification object value truncated", ErrInvalidResponse)
		}
		objects[objectID] = append([]byte(nil), data[offset:offset+objectLen]...)
		offset += objectLen
	}
	if offset != len(data) {
		return DeviceIdentification{}, fmt.Errorf("%w: trailing device identification bytes", ErrInvalidResponse)
	}
	return DeviceIdentification{
		ReadDeviceIDCode: code,
		ConformityLevel:  data[2],
		MoreFollows:      moreFollows == moreFollowsYes,
		NextObjectID:     data[4],
		Objects:          objects,
	}, nil
}

func buildDeviceIdentificationResponse(code ReadDeviceIDCode, conformityLevel byte, moreFollows bool, nextObjectID byte, objects map[byte][]byte) (PDU, error) {
	if conformityLevel == 0 {
		conformityLevel = 0x01
	}
	data := []byte{
		meiTypeReadDeviceIdentification,
		byte(code),
		conformityLevel,
		moreFollowsNo,
		nextObjectID,
		0,
	}
	if moreFollows {
		data[3] = moreFollowsYes
	}
	ids := sortedDeviceIdentificationObjectIDs(objects)
	for _, id := range ids {
		value := objects[id]
		if len(value) > 255 {
			return PDU{}, fmt.Errorf("%w: device identification object 0x%02x too large", ErrInvalidRequest, id)
		}
		if len(data)+2+len(value) > maxPDUSize-1 {
			return PDU{}, fmt.Errorf("%w: device identification response too large", ErrInvalidRequest)
		}
		data = append(data, id, byte(len(value)))
		data = append(data, value...)
		data[5]++
	}
	return PDU{Function: FuncReadDeviceIdentification, Data: data}, nil
}

func sortedDeviceIdentificationObjectIDs(objects map[byte][]byte) []byte {
	ids := make([]byte, 0, len(objects))
	for id := range objects {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
