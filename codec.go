package modbus

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	tcpHeaderSize = 7
	maxPDUSize    = 253
)

type TCPCodec struct{}

func (TCPCodec) Encode(unitID byte, transactionID uint16, pdu PDU) ([]byte, error) {
	pduBytes := pdu.Bytes()
	if len(pduBytes) == 0 || len(pduBytes) > maxPDUSize {
		return nil, fmt.Errorf("%w: pdu length %d", ErrInvalidRequest, len(pduBytes))
	}
	adu := make([]byte, tcpHeaderSize+len(pduBytes))
	binary.BigEndian.PutUint16(adu[0:], transactionID)
	binary.BigEndian.PutUint16(adu[2:], 0)
	binary.BigEndian.PutUint16(adu[4:], uint16(len(pduBytes)+1))
	adu[6] = unitID
	copy(adu[7:], pduBytes)
	return adu, nil
}

func (TCPCodec) Decode(r io.Reader) (Frame, error) {
	var header [tcpHeaderSize]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return Frame{}, err
	}
	txID := binary.BigEndian.Uint16(header[0:])
	protocolID := binary.BigEndian.Uint16(header[2:])
	length := binary.BigEndian.Uint16(header[4:])
	if protocolID != 0 {
		return Frame{}, fmt.Errorf("%w: protocol id %d", ErrInvalidResponse, protocolID)
	}
	if length < 2 || length > maxPDUSize+1 {
		return Frame{}, fmt.Errorf("%w: tcp length %d", ErrInvalidResponse, length)
	}
	body := make([]byte, int(length)-1)
	if _, err := io.ReadFull(r, body); err != nil {
		return Frame{}, err
	}
	pdu, err := ParsePDU(body)
	if err != nil {
		return Frame{}, err
	}
	return Frame{TransactionID: txID, UnitID: header[6], PDU: pdu}, nil
}

type RTUCodec struct{}

func (RTUCodec) Encode(unitID byte, _ uint16, pdu PDU) ([]byte, error) {
	pduBytes := pdu.Bytes()
	if unitID == 0 {
		return nil, fmt.Errorf("%w: rtu unit id must be non-zero", ErrInvalidRequest)
	}
	if len(pduBytes) == 0 || len(pduBytes) > maxPDUSize {
		return nil, fmt.Errorf("%w: pdu length %d", ErrInvalidRequest, len(pduBytes))
	}
	frame := make([]byte, 1+len(pduBytes)+2)
	frame[0] = unitID
	copy(frame[1:], pduBytes)
	crc := CRC16(frame[:len(frame)-2])
	frame[len(frame)-2] = byte(crc)
	frame[len(frame)-1] = byte(crc >> 8)
	return frame, nil
}

func (RTUCodec) DecodeFrame(data []byte) (Frame, error) {
	if len(data) < 4 {
		return Frame{}, fmt.Errorf("%w: rtu frame too short", ErrInvalidResponse)
	}
	got := uint16(data[len(data)-2]) | uint16(data[len(data)-1])<<8
	want := CRC16(data[:len(data)-2])
	if got != want {
		return Frame{}, fmt.Errorf("%w: crc mismatch", ErrInvalidResponse)
	}
	pdu, err := ParsePDU(data[1 : len(data)-2])
	if err != nil {
		return Frame{}, err
	}
	return Frame{UnitID: data[0], PDU: pdu}, nil
}
