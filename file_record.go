package modbus

import "fmt"

const (
	FileRecordReferenceType byte = 0x06
	maxFileRecordByteCount  int  = 245
)

type FileRecordRequest struct {
	ReferenceType byte
	FileNumber    uint16
	RecordNumber  uint16
	RecordLength  uint16
}

type FileRecord struct {
	ReferenceType byte
	FileNumber    uint16
	RecordNumber  uint16
	Values        []uint16
}

func buildReadFileRecordRequestData(items []FileRecordRequest) ([]byte, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("%w: file record request must contain at least one item", ErrInvalidRequest)
	}
	byteCount := len(items) * 7
	if byteCount > maxFileRecordByteCount {
		return nil, fmt.Errorf("%w: file record byte count exceeds %d", ErrInvalidRequest, maxFileRecordByteCount)
	}
	data := make([]byte, 1+byteCount)
	data[0] = byte(byteCount)
	offset := 1
	for _, item := range items {
		refType := normalizedFileRecordReferenceType(item.ReferenceType)
		if refType != FileRecordReferenceType {
			return nil, fmt.Errorf("%w: unsupported file record reference type 0x%02x", ErrInvalidRequest, refType)
		}
		if item.RecordLength == 0 {
			return nil, fmt.Errorf("%w: file record length must be non-zero", ErrInvalidRequest)
		}
		data[offset] = refType
		putUint16(data[offset+1:], item.FileNumber)
		putUint16(data[offset+3:], item.RecordNumber)
		putUint16(data[offset+5:], item.RecordLength)
		offset += 7
	}
	return data, nil
}

func parseReadFileRecordRequestData(data []byte) ([]FileRecordRequest, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("%w: read file record request too short", ErrInvalidRequest)
	}
	byteCount := int(data[0])
	if byteCount == 0 || byteCount > maxFileRecordByteCount || len(data) != 1+byteCount || byteCount%7 != 0 {
		return nil, fmt.Errorf("%w: invalid read file record byte count", ErrInvalidRequest)
	}
	items := make([]FileRecordRequest, 0, byteCount/7)
	for offset := 1; offset < len(data); offset += 7 {
		refType := data[offset]
		if refType != FileRecordReferenceType {
			return nil, fmt.Errorf("%w: unsupported file record reference type 0x%02x", ErrInvalidRequest, refType)
		}
		recordLength := uint16At(data[offset+5:])
		if recordLength == 0 {
			return nil, fmt.Errorf("%w: file record length must be non-zero", ErrInvalidRequest)
		}
		items = append(items, FileRecordRequest{
			ReferenceType: refType,
			FileNumber:    uint16At(data[offset+1:]),
			RecordNumber:  uint16At(data[offset+3:]),
			RecordLength:  recordLength,
		})
	}
	return items, nil
}

func buildReadFileRecordResponseData(records []FileRecord) ([]byte, error) {
	if len(records) == 0 {
		return nil, fmt.Errorf("%w: file record response must contain at least one item", ErrInvalidRequest)
	}
	byteCount := 0
	for _, record := range records {
		if len(record.Values) == 0 {
			return nil, fmt.Errorf("%w: file record values must be non-empty", ErrInvalidRequest)
		}
		byteCount += 2 + len(record.Values)*2
	}
	if byteCount > maxFileRecordByteCount {
		return nil, fmt.Errorf("%w: file record byte count exceeds %d", ErrInvalidRequest, maxFileRecordByteCount)
	}
	data := make([]byte, 1+byteCount)
	data[0] = byte(byteCount)
	offset := 1
	for _, record := range records {
		refType := normalizedFileRecordReferenceType(record.ReferenceType)
		if refType != FileRecordReferenceType {
			return nil, fmt.Errorf("%w: unsupported file record reference type 0x%02x", ErrInvalidRequest, refType)
		}
		payload := registersToBytes(record.Values)
		data[offset] = byte(len(payload) + 1)
		data[offset+1] = refType
		copy(data[offset+2:], payload)
		offset += 2 + len(payload)
	}
	return data, nil
}

func parseReadFileRecordResponseData(data []byte, requests []FileRecordRequest) ([]FileRecord, error) {
	if len(data) < 3 {
		return nil, fmt.Errorf("%w: read file record response too short", ErrInvalidResponse)
	}
	byteCount := int(data[0])
	if byteCount == 0 || byteCount > maxFileRecordByteCount || len(data) != 1+byteCount {
		return nil, fmt.Errorf("%w: invalid read file record byte count", ErrInvalidResponse)
	}
	records := make([]FileRecord, 0, len(requests))
	offset := 1
	for offset < len(data) {
		if len(data)-offset < 2 {
			return nil, fmt.Errorf("%w: truncated read file record response item", ErrInvalidResponse)
		}
		dataLength := int(data[offset])
		if dataLength < 3 || len(data)-offset-1 < dataLength {
			return nil, fmt.Errorf("%w: invalid read file record item length", ErrInvalidResponse)
		}
		refType := data[offset+1]
		payload := data[offset+2 : offset+1+dataLength]
		if refType != FileRecordReferenceType || len(payload)%2 != 0 {
			return nil, fmt.Errorf("%w: invalid read file record response item", ErrInvalidResponse)
		}
		record := FileRecord{
			ReferenceType: refType,
			Values:        bytesToRegisters(payload),
		}
		if len(records) < len(requests) {
			req := requests[len(records)]
			record.FileNumber = req.FileNumber
			record.RecordNumber = req.RecordNumber
			if int(req.RecordLength) != len(record.Values) {
				return nil, fmt.Errorf("%w: read file record length mismatch", ErrInvalidResponse)
			}
		}
		records = append(records, record)
		offset += 1 + dataLength
	}
	if len(requests) > 0 && len(records) != len(requests) {
		return nil, fmt.Errorf("%w: read file record item count mismatch", ErrInvalidResponse)
	}
	return records, nil
}

func buildWriteFileRecordData(records []FileRecord) ([]byte, error) {
	if len(records) == 0 {
		return nil, fmt.Errorf("%w: file record request must contain at least one item", ErrInvalidRequest)
	}
	byteCount := 0
	for _, record := range records {
		if len(record.Values) == 0 {
			return nil, fmt.Errorf("%w: file record values must be non-empty", ErrInvalidRequest)
		}
		byteCount += 7 + len(record.Values)*2
	}
	if byteCount > maxFileRecordByteCount {
		return nil, fmt.Errorf("%w: file record byte count exceeds %d", ErrInvalidRequest, maxFileRecordByteCount)
	}
	data := make([]byte, 1+byteCount)
	data[0] = byte(byteCount)
	offset := 1
	for _, record := range records {
		refType := normalizedFileRecordReferenceType(record.ReferenceType)
		if refType != FileRecordReferenceType {
			return nil, fmt.Errorf("%w: unsupported file record reference type 0x%02x", ErrInvalidRequest, refType)
		}
		payload := registersToBytes(record.Values)
		data[offset] = refType
		putUint16(data[offset+1:], record.FileNumber)
		putUint16(data[offset+3:], record.RecordNumber)
		putUint16(data[offset+5:], uint16(len(record.Values)))
		copy(data[offset+7:], payload)
		offset += 7 + len(payload)
	}
	return data, nil
}

func parseWriteFileRecordData(data []byte) ([]FileRecord, error) {
	if len(data) < 10 {
		return nil, fmt.Errorf("%w: write file record data too short", ErrInvalidRequest)
	}
	byteCount := int(data[0])
	if byteCount == 0 || byteCount > maxFileRecordByteCount || len(data) != 1+byteCount {
		return nil, fmt.Errorf("%w: invalid write file record byte count", ErrInvalidRequest)
	}
	records := make([]FileRecord, 0)
	offset := 1
	for offset < len(data) {
		if len(data)-offset < 7 {
			return nil, fmt.Errorf("%w: truncated write file record item", ErrInvalidRequest)
		}
		refType := data[offset]
		if refType != FileRecordReferenceType {
			return nil, fmt.Errorf("%w: unsupported file record reference type 0x%02x", ErrInvalidRequest, refType)
		}
		recordLength := int(uint16At(data[offset+5:]))
		if recordLength == 0 {
			return nil, fmt.Errorf("%w: file record length must be non-zero", ErrInvalidRequest)
		}
		payloadLen := recordLength * 2
		if len(data)-offset-7 < payloadLen {
			return nil, fmt.Errorf("%w: truncated write file record payload", ErrInvalidRequest)
		}
		records = append(records, FileRecord{
			ReferenceType: refType,
			FileNumber:    uint16At(data[offset+1:]),
			RecordNumber:  uint16At(data[offset+3:]),
			Values:        bytesToRegisters(data[offset+7 : offset+7+payloadLen]),
		})
		offset += 7 + payloadLen
	}
	return records, nil
}

func normalizedFileRecordReferenceType(referenceType byte) byte {
	if referenceType == 0 {
		return FileRecordReferenceType
	}
	return referenceType
}
