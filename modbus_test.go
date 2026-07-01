package modbus

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"net"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func TestCRC16(t *testing.T) {
	data := []byte{0x01, 0x03, 0x00, 0x00, 0x00, 0x0A}
	if got, want := CRC16(data), uint16(0xCDC5); got != want {
		t.Fatalf("CRC16()=%04x want %04x", got, want)
	}
}

func TestTCPClientServer(t *testing.T) {
	store := NewMemoryDataStore()
	if err := store.WriteHoldingRegisters(0, []uint16{10, 20, 30}); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteCoils(0, []bool{true, false, true}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := NewTCPServer(NewDataStoreHandler(store))
	go func() {
		_ = server.Serve(ctx, ln)
	}()
	client := NewClient(NewTCPTransport(ln.Addr().String()), WithTimeout(time.Second))
	defer client.Close()

	regs, err := client.ReadHoldingRegisters(ctx, 0, 3)
	if err != nil {
		t.Fatal(err)
	}
	if regs[0] != 10 || regs[1] != 20 || regs[2] != 30 {
		t.Fatalf("unexpected registers: %#v", regs)
	}
	bits, err := client.ReadCoils(ctx, 0, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !bits[0] || bits[1] || !bits[2] {
		t.Fatalf("unexpected bits: %#v", bits)
	}
	if err := client.WriteSingleRegister(ctx, 1, 99); err != nil {
		t.Fatal(err)
	}
	regs, err = client.ReadHoldingRegisters(ctx, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if regs[0] != 99 {
		t.Fatalf("write did not persist: %#v", regs)
	}
}

func TestTCPTransportDoesNotRetryCurrentRequestAfterConnectionError(t *testing.T) {
	ctx := context.Background()
	var dials atomic.Int32
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	transport := NewTCPTransport("unused", WithTCPDialer(func(context.Context, string, string) (net.Conn, error) {
		dials.Add(1)
		return net.Dial("tcp", ln.Addr().String())
	}), WithTCPTimeout(time.Second))
	client := NewClient(transport, WithTimeout(time.Second))
	defer client.Close()

	firstDone := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			firstDone <- err
			return
		}
		firstDone <- conn.Close()
	}()
	if _, err := client.ReadHoldingRegisters(ctx, 0, 1); err == nil {
		t.Fatal("expected first request to fail")
	}
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if got := dials.Load(); got != 1 {
		t.Fatalf("current request was retried: dials=%d", got)
	}

	store := NewMemoryDataStore()
	if err := store.WriteHoldingRegisters(0, []uint16{55}); err != nil {
		t.Fatal(err)
	}
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			_ = NewTCPServer(NewDataStoreHandler(store)).serveConn(ctx, conn)
		}
	}()
	values, err := client.ReadHoldingRegisters(ctx, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := dials.Load(); got != 2 {
		t.Fatalf("next request did not redial exactly once: dials=%d", got)
	}
	if !uint16SlicesEqual(values, []uint16{55}) {
		t.Fatalf("unexpected values: %#v", values)
	}
}

func TestReadWriteMultipleRegistersTCP(t *testing.T) {
	store := NewMemoryDataStore()
	if err := store.WriteHoldingRegisters(0, []uint16{10, 20, 30, 40}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = NewTCPServer(NewDataStoreHandler(store)).Serve(ctx, ln)
	}()
	client := NewClient(NewTCPTransport(ln.Addr().String()), WithTimeout(time.Second))
	defer client.Close()

	read, err := client.ReadWriteMultipleRegisters(ctx, 0, 4, 2, []uint16{300, 400})
	if err != nil {
		t.Fatal(err)
	}
	if !uint16SlicesEqual(read, []uint16{10, 20, 300, 400}) {
		t.Fatalf("unexpected read values: %#v", read)
	}
	regs, err := client.ReadHoldingRegisters(ctx, 0, 4)
	if err != nil {
		t.Fatal(err)
	}
	if !uint16SlicesEqual(regs, []uint16{10, 20, 300, 400}) {
		t.Fatalf("write did not persist: %#v", regs)
	}
}

func TestMaskWriteRegisterTCP(t *testing.T) {
	store := NewMemoryDataStore()
	if err := store.WriteHoldingRegisters(0, []uint16{0x00F0}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = NewTCPServer(NewDataStoreHandler(store)).Serve(ctx, ln)
	}()
	client := NewClient(NewTCPTransport(ln.Addr().String()), WithTimeout(time.Second))
	defer client.Close()

	if err := client.MaskWriteRegister(ctx, 0, 0x0FF0, 0x0005); err != nil {
		t.Fatal(err)
	}
	regs, err := client.ReadHoldingRegisters(ctx, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if regs[0] != 0x00F5 {
		t.Fatalf("unexpected masked value: 0x%04x", regs[0])
	}
}

func TestReadDeviceIdentificationTCP(t *testing.T) {
	store := NewMemoryDataStore()
	handler := NewDataStoreHandler(store)
	handler.DeviceIdentification = map[byte][]byte{
		0x00: []byte("vendor"),
		0x01: []byte("product"),
		0x02: []byte("revision"),
	}
	handler.DeviceIdentificationConformityLevel = 0x81
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = NewTCPServer(handler).Serve(ctx, ln)
	}()
	client := NewClient(NewTCPTransport(ln.Addr().String()), WithTimeout(time.Second))
	defer client.Close()

	info, err := client.ReadDeviceIdentification(ctx, ReadDeviceIDCodeBasic)
	if err != nil {
		t.Fatal(err)
	}
	if info.ReadDeviceIDCode != ReadDeviceIDCodeBasic || info.ConformityLevel != 0x81 || info.MoreFollows {
		t.Fatalf("unexpected metadata: %#v", info)
	}
	if string(info.Objects[0x00]) != "vendor" || string(info.Objects[0x01]) != "product" || string(info.Objects[0x02]) != "revision" {
		t.Fatalf("unexpected objects: %#v", info.Objects)
	}
}

func TestReadFIFOQueueTCP(t *testing.T) {
	handler := NewDataStoreHandler(NewMemoryDataStore())
	handler.FIFOQueues = map[uint16][]uint16{
		0x04DE: {10, 20, 30},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = NewTCPServer(handler).Serve(ctx, ln)
	}()
	client := NewClient(NewTCPTransport(ln.Addr().String()), WithTimeout(time.Second))
	defer client.Close()

	values, err := client.ReadFIFOQueue(ctx, 0x04DE)
	if err != nil {
		t.Fatal(err)
	}
	if !uint16SlicesEqual(values, []uint16{10, 20, 30}) {
		t.Fatalf("unexpected fifo values: %#v", values)
	}
}

func TestReadFileRecordsTCP(t *testing.T) {
	handler := NewDataStoreHandler(NewMemoryDataStore())
	handler.FileRecords = map[uint16][]uint16{
		7: {10, 20, 30, 40, 50},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = NewTCPServer(handler).Serve(ctx, ln)
	}()
	client := NewClient(NewTCPTransport(ln.Addr().String()), WithTimeout(time.Second))
	defer client.Close()

	records, err := client.ReadFileRecords(ctx, []FileRecordRequest{
		{FileNumber: 7, RecordNumber: 1, RecordLength: 2},
		{FileNumber: 7, RecordNumber: 3, RecordLength: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("unexpected record count: %d", len(records))
	}
	if records[0].FileNumber != 7 || records[0].RecordNumber != 1 || !uint16SlicesEqual(records[0].Values, []uint16{20, 30}) {
		t.Fatalf("unexpected first record: %#v", records[0])
	}
	if records[1].RecordNumber != 3 || !uint16SlicesEqual(records[1].Values, []uint16{40}) {
		t.Fatalf("unexpected second record: %#v", records[1])
	}
}

func TestWriteFileRecordsTCP(t *testing.T) {
	handler := NewDataStoreHandler(NewMemoryDataStore())
	handler.FileRecords = map[uint16][]uint16{
		7: {10, 20, 30, 40, 50},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = NewTCPServer(handler).Serve(ctx, ln)
	}()
	client := NewClient(NewTCPTransport(ln.Addr().String()), WithTimeout(time.Second))
	defer client.Close()

	err = client.WriteFileRecords(ctx, []FileRecord{
		{FileNumber: 7, RecordNumber: 2, Values: []uint16{300, 400}},
	})
	if err != nil {
		t.Fatal(err)
	}
	records, err := client.ReadFileRecords(ctx, []FileRecordRequest{
		{FileNumber: 7, RecordNumber: 0, RecordLength: 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !uint16SlicesEqual(records[0].Values, []uint16{10, 20, 300, 400, 50}) {
		t.Fatalf("write did not persist: %#v", records[0].Values)
	}
}

func TestReadFIFOQueueEmptyTCP(t *testing.T) {
	handler := NewDataStoreHandler(NewMemoryDataStore())
	handler.FIFOQueues = map[uint16][]uint16{
		0x04DE: nil,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = NewTCPServer(handler).Serve(ctx, ln)
	}()
	client := NewClient(NewTCPTransport(ln.Addr().String()), WithTimeout(time.Second))
	defer client.Close()

	values, err := client.ReadFIFOQueue(ctx, 0x04DE)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 0 {
		t.Fatalf("unexpected fifo values: %#v", values)
	}
}

func TestReadFIFOQueueUnconfigured(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = NewTCPServer(NewDataStoreHandler(NewMemoryDataStore())).Serve(ctx, ln)
	}()
	client := NewClient(NewTCPTransport(ln.Addr().String()), WithTimeout(time.Second))
	defer client.Close()

	_, err = client.ReadFIFOQueue(ctx, 0x04DE)
	var ex *ExceptionError
	if !errors.As(err, &ex) || ex.Code != ExceptionIllegalFunction {
		t.Fatalf("expected illegal function exception, got %v", err)
	}
}

func TestReadFileRecordUnconfigured(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = NewTCPServer(NewDataStoreHandler(NewMemoryDataStore())).Serve(ctx, ln)
	}()
	client := NewClient(NewTCPTransport(ln.Addr().String()), WithTimeout(time.Second))
	defer client.Close()

	_, err = client.ReadFileRecords(ctx, []FileRecordRequest{{FileNumber: 7, RecordNumber: 0, RecordLength: 1}})
	var ex *ExceptionError
	if !errors.As(err, &ex) || ex.Code != ExceptionIllegalFunction {
		t.Fatalf("expected illegal function exception, got %v", err)
	}
}

func TestParseFIFOQueueInvalidResponse(t *testing.T) {
	_, err := parseFIFOQueueResponse([]byte{0x00, 0x04, 0x00, 0x03, 0x00, 0x01})
	if err == nil {
		t.Fatal("expected invalid fifo response error")
	}
}

func TestParseReadFileRecordInvalidResponse(t *testing.T) {
	_, err := parseReadFileRecordResponseData([]byte{0x03, 0x03, 0x06, 0x00}, []FileRecordRequest{
		{FileNumber: 7, RecordNumber: 0, RecordLength: 2},
	})
	if err == nil {
		t.Fatal("expected invalid file record response error")
	}
}

func TestReadDeviceIdentificationObjectTCP(t *testing.T) {
	store := NewMemoryDataStore()
	handler := NewDataStoreHandler(store)
	handler.DeviceIdentification = map[byte][]byte{
		0x00: []byte("vendor"),
		0x01: []byte("product"),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = NewTCPServer(handler).Serve(ctx, ln)
	}()
	client := NewClient(NewTCPTransport(ln.Addr().String()), WithTimeout(time.Second))
	defer client.Close()

	info, err := client.ReadDeviceIdentificationObject(ctx, 0x01)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Objects) != 1 || string(info.Objects[0x01]) != "product" {
		t.Fatalf("unexpected objects: %#v", info.Objects)
	}
}

func TestReadDeviceIdentificationPagination(t *testing.T) {
	first := bytes.Repeat([]byte{'a'}, 120)
	second := bytes.Repeat([]byte{'b'}, 120)
	third := bytes.Repeat([]byte{'c'}, 120)
	handler := NewDataStoreHandler(NewMemoryDataStore())
	handler.DeviceIdentification = map[byte][]byte{
		0x00: first,
		0x01: second,
		0x02: third,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = NewTCPServer(handler).Serve(ctx, ln)
	}()
	client := NewClient(NewTCPTransport(ln.Addr().String()), WithTimeout(time.Second))
	defer client.Close()

	info, err := client.ReadDeviceIdentification(ctx, ReadDeviceIDCodeBasic)
	if err != nil {
		t.Fatal(err)
	}
	if info.MoreFollows || len(info.Objects) != 3 {
		t.Fatalf("unexpected pagination result: %#v", info)
	}
	if !bytes.Equal(info.Objects[0x00], first) || !bytes.Equal(info.Objects[0x01], second) || !bytes.Equal(info.Objects[0x02], third) {
		t.Fatalf("unexpected objects after pagination")
	}
}

func TestReadDeviceIdentificationUnconfigured(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = NewTCPServer(NewDataStoreHandler(NewMemoryDataStore())).Serve(ctx, ln)
	}()
	client := NewClient(NewTCPTransport(ln.Addr().String()), WithTimeout(time.Second))
	defer client.Close()

	_, err = client.ReadDeviceIdentification(ctx, ReadDeviceIDCodeBasic)
	var ex *ExceptionError
	if !errors.As(err, &ex) || ex.Code != ExceptionIllegalFunction {
		t.Fatalf("expected illegal function exception, got %v", err)
	}
}

func TestException(t *testing.T) {
	store := NewMemoryDataStoreSized(1, 1, 1, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = NewTCPServer(NewDataStoreHandler(store)).Serve(ctx, ln)
	}()
	client := NewClient(NewTCPTransport(ln.Addr().String()), WithTimeout(time.Second))
	defer client.Close()

	_, err = client.ReadHoldingRegisters(ctx, 10, 1)
	var ex *ExceptionError
	if !errors.As(err, &ex) || ex.Code != ExceptionIllegalDataAddress {
		t.Fatalf("expected illegal data address exception, got %v", err)
	}
}

func TestDiagnosticTCP(t *testing.T) {
	handler := NewDataStoreHandler(NewMemoryDataStore())
	handler.EnableDiagnostics = true
	handler.DiagnosticResponses = map[uint16]uint16{
		0x0001: 0x1234,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = NewTCPServer(handler).Serve(ctx, ln)
	}()
	client := NewClient(NewTCPTransport(ln.Addr().String()), WithTimeout(time.Second))
	defer client.Close()

	value, err := client.Diagnostic(ctx, 0x0000, 0xABCD)
	if err != nil {
		t.Fatal(err)
	}
	if value != 0xABCD {
		t.Fatalf("unexpected diagnostic echo: 0x%04x", value)
	}
	value, err = client.Diagnostic(ctx, 0x0001, 0x0000)
	if err != nil {
		t.Fatal(err)
	}
	if value != 0x1234 {
		t.Fatalf("unexpected configured diagnostic value: 0x%04x", value)
	}
}

func TestReadExceptionStatusTCP(t *testing.T) {
	status := byte(0x5A)
	handler := NewDataStoreHandler(NewMemoryDataStore())
	handler.ExceptionStatus = &status
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = NewTCPServer(handler).Serve(ctx, ln)
	}()
	client := NewClient(NewTCPTransport(ln.Addr().String()), WithTimeout(time.Second))
	defer client.Close()

	got, err := client.ReadExceptionStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got != status {
		t.Fatalf("unexpected exception status: 0x%02x", got)
	}
}

func TestCommEventCounterTCP(t *testing.T) {
	handler := NewDataStoreHandler(NewMemoryDataStore())
	handler.CommEventCounter = &CommEventCounter{Status: 0xFFFF, EventCount: 9}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = NewTCPServer(handler).Serve(ctx, ln)
	}()
	client := NewClient(NewTCPTransport(ln.Addr().String()), WithTimeout(time.Second))
	defer client.Close()

	counter, err := client.GetCommEventCounter(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if counter.Status != 0xFFFF || counter.EventCount != 9 {
		t.Fatalf("unexpected counter: %#v", counter)
	}
}

func TestCommEventLogTCP(t *testing.T) {
	handler := NewDataStoreHandler(NewMemoryDataStore())
	handler.CommEventLog = &CommEventLog{
		Status:       0x0000,
		EventCount:   7,
		MessageCount: 3,
		Events:       []byte{0x11, 0x22, 0x33},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = NewTCPServer(handler).Serve(ctx, ln)
	}()
	client := NewClient(NewTCPTransport(ln.Addr().String()), WithTimeout(time.Second))
	defer client.Close()

	log, err := client.GetCommEventLog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if log.Status != 0 || log.EventCount != 7 || log.MessageCount != 3 || !bytes.Equal(log.Events, []byte{0x11, 0x22, 0x33}) {
		t.Fatalf("unexpected log: %#v", log)
	}
}

func TestReportServerIDTCP(t *testing.T) {
	handler := NewDataStoreHandler(NewMemoryDataStore())
	handler.ServerID = []byte{0x01, 0xFF, 'G', 'o'}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = NewTCPServer(handler).Serve(ctx, ln)
	}()
	client := NewClient(NewTCPTransport(ln.Addr().String()), WithTimeout(time.Second))
	defer client.Close()

	value, err := client.ReportServerID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(value, []byte{0x01, 0xFF, 'G', 'o'}) {
		t.Fatalf("unexpected server id: %#v", value)
	}
}

func TestDiagnosticUnconfigured(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = NewTCPServer(NewDataStoreHandler(NewMemoryDataStore())).Serve(ctx, ln)
	}()
	client := NewClient(NewTCPTransport(ln.Addr().String()), WithTimeout(time.Second))
	defer client.Close()

	_, err = client.Diagnostic(ctx, 0, 0)
	var ex *ExceptionError
	if !errors.As(err, &ex) || ex.Code != ExceptionIllegalFunction {
		t.Fatalf("expected illegal function exception, got %v", err)
	}
}

func TestTagFloat32(t *testing.T) {
	raw, err := EncodeValue(HoldingRegister(0).As(TypeFloat32), float32(12.5))
	if err != nil {
		t.Fatal(err)
	}
	value, err := DecodeValue(HoldingRegister(0).As(TypeFloat32), raw)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := value.Data.(float32)
	if !ok || math.Abs(float64(got-12.5)) > 0.001 {
		t.Fatalf("unexpected value %#v", value.Data)
	}
}

func TestValueFloat64(t *testing.T) {
	tag := HoldingRegister(0).As(TypeFloat64)
	raw, err := EncodeValue(tag, float64(123.75))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw.Registers) != 4 {
		t.Fatalf("expected 4 registers, got %d", len(raw.Registers))
	}
	value, err := DecodeValue(tag, raw)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := value.Data.(float64)
	if !ok || math.Abs(got-123.75) > 0.000001 {
		t.Fatalf("unexpected value %#v", value.Data)
	}
}

func TestValueUInt64LowWordLittleByteOrder(t *testing.T) {
	tag := HoldingRegister(0).As(TypeUInt64).WithOrder(ByteOrderLittleEndian, WordOrderLowFirst)
	raw, err := EncodeValue(tag, uint64(0x0102030405060708))
	if err != nil {
		t.Fatal(err)
	}
	value, err := DecodeValue(tag, raw)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := value.Data.(uint64)
	if !ok || got != 0x0102030405060708 {
		t.Fatalf("unexpected value %#v raw=%#v", value.Data, raw.Registers)
	}
}

func TestValueString(t *testing.T) {
	tag := HoldingRegister(0).As(TypeString).WithQuantity(3)
	raw, err := EncodeValue(tag, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if len(raw.Registers) != 3 {
		t.Fatalf("expected 3 registers, got %d", len(raw.Registers))
	}
	value, err := DecodeValue(tag, raw)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := value.Data.(string)
	if !ok || got != "hello" {
		t.Fatalf("unexpected string %#v", value.Data)
	}
}

func TestValueAccessors(t *testing.T) {
	if got, ok := (Value{Data: true}).Bool(); !ok || !got {
		t.Fatalf("Bool()=%v %v", got, ok)
	}
	if got, ok := (Value{Data: []bool{true, false}}).Bools(); !ok || !reflect.DeepEqual(got, []bool{true, false}) {
		t.Fatalf("Bools()=%#v %v", got, ok)
	}
	if got, ok := (Value{Data: uint16(12)}).UInt16(); !ok || got != 12 {
		t.Fatalf("UInt16()=%v %v", got, ok)
	}
	if got, ok := (Value{Data: []uint16{1, 2}}).UInt16s(); !ok || !reflect.DeepEqual(got, []uint16{1, 2}) {
		t.Fatalf("UInt16s()=%#v %v", got, ok)
	}
	if got, ok := (Value{Data: int16(-12)}).Int16(); !ok || got != -12 {
		t.Fatalf("Int16()=%v %v", got, ok)
	}
	if got, ok := (Value{Data: []int16{-1, 2}}).Int16s(); !ok || !reflect.DeepEqual(got, []int16{-1, 2}) {
		t.Fatalf("Int16s()=%#v %v", got, ok)
	}
	if got, ok := (Value{Data: uint32(12)}).UInt32(); !ok || got != 12 {
		t.Fatalf("UInt32()=%v %v", got, ok)
	}
	if got, ok := (Value{Data: []uint32{1, 2}}).UInt32s(); !ok || !reflect.DeepEqual(got, []uint32{1, 2}) {
		t.Fatalf("UInt32s()=%#v %v", got, ok)
	}
	if got, ok := (Value{Data: int32(-12)}).Int32(); !ok || got != -12 {
		t.Fatalf("Int32()=%v %v", got, ok)
	}
	if got, ok := (Value{Data: []int32{-1, 2}}).Int32s(); !ok || !reflect.DeepEqual(got, []int32{-1, 2}) {
		t.Fatalf("Int32s()=%#v %v", got, ok)
	}
	if got, ok := (Value{Data: float32(12.5)}).Float32(); !ok || got != 12.5 {
		t.Fatalf("Float32()=%v %v", got, ok)
	}
	if got, ok := (Value{Data: []float32{1.5, 2.5}}).Float32s(); !ok || !reflect.DeepEqual(got, []float32{1.5, 2.5}) {
		t.Fatalf("Float32s()=%#v %v", got, ok)
	}
	if got, ok := (Value{Data: uint64(12)}).UInt64(); !ok || got != 12 {
		t.Fatalf("UInt64()=%v %v", got, ok)
	}
	if got, ok := (Value{Data: []uint64{1, 2}}).UInt64s(); !ok || !reflect.DeepEqual(got, []uint64{1, 2}) {
		t.Fatalf("UInt64s()=%#v %v", got, ok)
	}
	if got, ok := (Value{Data: int64(-12)}).Int64(); !ok || got != -12 {
		t.Fatalf("Int64()=%v %v", got, ok)
	}
	if got, ok := (Value{Data: []int64{-1, 2}}).Int64s(); !ok || !reflect.DeepEqual(got, []int64{-1, 2}) {
		t.Fatalf("Int64s()=%#v %v", got, ok)
	}
	if got, ok := (Value{Data: float64(12.5)}).Float64(); !ok || got != 12.5 {
		t.Fatalf("Float64()=%v %v", got, ok)
	}
	if got, ok := (Value{Data: []float64{1.5, 2.5}}).Float64s(); !ok || !reflect.DeepEqual(got, []float64{1.5, 2.5}) {
		t.Fatalf("Float64s()=%#v %v", got, ok)
	}
	if got, ok := (Value{Data: []byte{1, 2}}).Bytes(); !ok || !bytes.Equal(got, []byte{1, 2}) {
		t.Fatalf("Bytes()=%#v %v", got, ok)
	}
	if got, ok := (Value{Data: "hello"}).String(); !ok || got != "hello" {
		t.Fatalf("String()=%q %v", got, ok)
	}
}

func TestValueAccessorSlicesAreCopied(t *testing.T) {
	value := Value{Data: []uint16{1, 2}}
	got, ok := value.UInt16s()
	if !ok {
		t.Fatal("UInt16s() ok=false")
	}
	got[0] = 99
	again, ok := value.UInt16s()
	if !ok || again[0] != 1 {
		t.Fatalf("slice accessor exposed internal data: %#v ok=%v", again, ok)
	}
}

func TestValueAccessorTypeMismatch(t *testing.T) {
	if _, ok := (Value{Data: uint16(1)}).Float32(); ok {
		t.Fatal("Float32() ok=true for uint16 value")
	}
	if got, ok := (Value{Data: "not bytes"}).Bytes(); ok || got != nil {
		t.Fatalf("Bytes()=%#v %v", got, ok)
	}
}

func TestParseTagShortNames(t *testing.T) {
	tests := []struct {
		in   string
		want Tag
	}{
		{
			in:   "c:0:b:1",
			want: Tag{Area: AreaCoil, Address: 0, Quantity: 1, DataType: TypeBool, ByteOrder: ByteOrderBigEndian, WordOrder: WordOrderHighFirst},
		},
		{
			in:   "di:2",
			want: Tag{Area: AreaDiscreteInput, Address: 2, Quantity: 1, DataType: TypeBool, ByteOrder: ByteOrderBigEndian, WordOrder: WordOrderHighFirst},
		},
		{
			in:   "hr:10:f32:2",
			want: Tag{Area: AreaHoldingRegister, Address: 10, Quantity: 2, DataType: TypeFloat32, ByteOrder: ByteOrderBigEndian, WordOrder: WordOrderHighFirst},
		},
		{
			in:   "ir:20:u16:4",
			want: Tag{Area: AreaInputRegister, Address: 20, Quantity: 4, DataType: TypeUInt16, ByteOrder: ByteOrderBigEndian, WordOrder: WordOrderHighFirst},
		},
		{
			in:   "hr:30:str:8",
			want: Tag{Area: AreaHoldingRegister, Address: 30, Quantity: 8, DataType: TypeString, ByteOrder: ByteOrderBigEndian, WordOrder: WordOrderHighFirst},
		},
		{
			in:   "hr:40:by:2",
			want: Tag{Area: AreaHoldingRegister, Address: 40, Quantity: 2, DataType: TypeBytes, ByteOrder: ByteOrderBigEndian, WordOrder: WordOrderHighFirst},
		},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseTag(tt.in)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("ParseTag(%q)=%#v want %#v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseTagRejectsLongNames(t *testing.T) {
	for _, in := range []string{
		"holding-register:0:float32:1",
		"hr:0:float32:1",
		"coil:0:b:1",
	} {
		t.Run(in, func(t *testing.T) {
			if _, err := ParseTag(in); err == nil {
				t.Fatalf("ParseTag(%q) expected error", in)
			}
		})
	}
}

func TestReadTagsBatchOptimizer(t *testing.T) {
	floatRaw, err := EncodeValue(HoldingRegister(1).As(TypeFloat32), float32(12.5))
	if err != nil {
		t.Fatal(err)
	}
	store := NewMemoryDataStore()
	if err := store.WriteHoldingRegisters(0, append([]uint16{11}, floatRaw.Registers...)); err != nil {
		t.Fatal(err)
	}
	handler := &countingHandler{next: NewDataStoreHandler(store)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = NewTCPServer(handler).Serve(ctx, ln)
	}()
	client := NewClient(NewTCPTransport(ln.Addr().String()), WithTimeout(time.Second))
	defer client.Close()

	values, err := client.ReadTags(ctx, map[string]Tag{
		"status":      HoldingRegister(0),
		"temperature": HoldingRegister(1).As(TypeFloat32),
	})
	if err != nil {
		t.Fatal(err)
	}
	status, ok := values["status"].Data.(uint16)
	if !ok || status != 11 {
		t.Fatalf("unexpected status: %#v", values["status"].Data)
	}
	temperature, ok := values["temperature"].Data.(float32)
	if !ok || math.Abs(float64(temperature-12.5)) > 0.001 {
		t.Fatalf("unexpected temperature: %#v", values["temperature"].Data)
	}
	if got := handler.readHoldingCount.Load(); got != 1 {
		t.Fatalf("expected one merged read, got %d", got)
	}
	if handler.lastAddress.Load() != 0 || handler.lastQuantity.Load() != 3 {
		t.Fatalf("unexpected merged range address=%d quantity=%d", handler.lastAddress.Load(), handler.lastQuantity.Load())
	}
}

func TestWriteTagsBatchOptimizer(t *testing.T) {
	store := NewMemoryDataStore()
	handler := &countingHandler{next: NewDataStoreHandler(store)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = NewTCPServer(handler).Serve(ctx, ln)
	}()
	client := NewClient(NewTCPTransport(ln.Addr().String()), WithTimeout(time.Second))
	defer client.Close()

	err = client.WriteTags(ctx, map[string]TagValue{
		"status": {
			Tag:   HoldingRegister(0),
			Value: uint16(7),
		},
		"temperature": {
			Tag:   HoldingRegister(1).As(TypeFloat32),
			Value: float32(12.5),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	regs, err := client.ReadHoldingRegisters(ctx, 0, 3)
	if err != nil {
		t.Fatal(err)
	}
	value, err := DecodeValue(HoldingRegister(1).As(TypeFloat32), RawValue{Registers: regs[1:]})
	if err != nil {
		t.Fatal(err)
	}
	temperature, ok := value.Data.(float32)
	if regs[0] != 7 || !ok || math.Abs(float64(temperature-12.5)) > 0.001 {
		t.Fatalf("unexpected registers after WriteTags: regs=%#v value=%#v", regs, value.Data)
	}
	if got := handler.writeHoldingCount.Load(); got != 1 {
		t.Fatalf("expected one merged holding write, got %d", got)
	}
	if handler.lastWriteAddress.Load() != 0 || handler.lastWriteQuantity.Load() != 3 {
		t.Fatalf("unexpected merged write range address=%d quantity=%d", handler.lastWriteAddress.Load(), handler.lastWriteQuantity.Load())
	}
}

func TestWriteTagsCoils(t *testing.T) {
	store := NewMemoryDataStore()
	handler := &countingHandler{next: NewDataStoreHandler(store)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = NewTCPServer(handler).Serve(ctx, ln)
	}()
	client := NewClient(NewTCPTransport(ln.Addr().String()), WithTimeout(time.Second))
	defer client.Close()

	err = client.WriteTags(ctx, map[string]TagValue{
		"a": {Tag: Coil(0), Value: true},
		"b": {Tag: Coil(1), Value: false},
		"c": {Tag: Coil(2), Value: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	bits, err := client.ReadCoils(ctx, 0, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !bits[0] || bits[1] || !bits[2] {
		t.Fatalf("unexpected coils after WriteTags: %#v", bits)
	}
	if got := handler.writeCoilCount.Load(); got != 1 {
		t.Fatalf("expected one merged coil write, got %d", got)
	}
}

func TestWriteTagsRejectsReadOnlyArea(t *testing.T) {
	client := NewClient(NewTCPTransport("127.0.0.1:1"), WithTimeout(time.Millisecond))
	err := client.WriteTags(context.Background(), map[string]TagValue{
		"input": {Tag: InputRegister(0), Value: uint16(1)},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRTUClientServer(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	store := NewMemoryDataStore()
	if err := store.WriteHoldingRegisters(0, []uint16{42}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = NewRTUServer(NewDataStoreHandler(store)).Serve(ctx, serverConn)
	}()
	client := NewClient(NewRTUTransport(clientConn), WithTimeout(time.Second))
	defer client.Close()

	regs, err := client.ReadHoldingRegisters(ctx, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if regs[0] != 42 {
		t.Fatalf("unexpected registers: %#v", regs)
	}
}

func TestRTUTimingDurations(t *testing.T) {
	charTime, frameGap := rtuTimingDurations(RTUTiming{
		BaudRate: 9600,
		DataBits: 8,
		Parity:   true,
		StopBits: 1,
	})
	if charTime != 11*time.Second/9600 {
		t.Fatalf("unexpected char time: %v", charTime)
	}
	if frameGap != charTime*35/10 {
		t.Fatalf("unexpected frame gap: %v", frameGap)
	}

	_, highSpeedGap := rtuTimingDurations(RTUTiming{BaudRate: 38400})
	if highSpeedGap != 1750*time.Microsecond {
		t.Fatalf("unexpected high speed gap: %v", highSpeedGap)
	}
}

func TestRTUClientServerWithTiming(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	store := NewMemoryDataStore()
	if err := store.WriteHoldingRegisters(0, []uint16{55}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = NewRTUServer(NewDataStoreHandler(store)).Serve(ctx, serverConn)
	}()
	client := NewClient(NewRTUTransport(clientConn, WithRTUTiming(RTUTiming{
		BaudRate: 115200,
		DataBits: 8,
		Parity:   false,
		StopBits: 1,
	})), WithTimeout(time.Second))
	defer client.Close()

	regs, err := client.ReadHoldingRegisters(ctx, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if regs[0] != 55 {
		t.Fatalf("unexpected registers: %#v", regs)
	}
}

func TestReadWriteMultipleRegistersRTU(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	store := NewMemoryDataStore()
	if err := store.WriteHoldingRegisters(0, []uint16{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = NewRTUServer(NewDataStoreHandler(store)).Serve(ctx, serverConn)
	}()
	client := NewClient(NewRTUTransport(clientConn), WithTimeout(time.Second))
	defer client.Close()

	read, err := client.ReadWriteMultipleRegisters(ctx, 0, 3, 1, []uint16{22, 33})
	if err != nil {
		t.Fatal(err)
	}
	if !uint16SlicesEqual(read, []uint16{1, 22, 33}) {
		t.Fatalf("unexpected read values: %#v", read)
	}
}

func TestMaskWriteRegisterRTU(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	store := NewMemoryDataStore()
	if err := store.WriteHoldingRegisters(0, []uint16{0x00F0}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = NewRTUServer(NewDataStoreHandler(store)).Serve(ctx, serverConn)
	}()
	client := NewClient(NewRTUTransport(clientConn), WithTimeout(time.Second))
	defer client.Close()

	if err := client.MaskWriteRegister(ctx, 0, 0x0FF0, 0x0005); err != nil {
		t.Fatal(err)
	}
	regs, err := client.ReadHoldingRegisters(ctx, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if regs[0] != 0x00F5 {
		t.Fatalf("unexpected masked value: 0x%04x", regs[0])
	}
}

func TestReadDeviceIdentificationRTU(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	handler := NewDataStoreHandler(NewMemoryDataStore())
	handler.DeviceIdentification = map[byte][]byte{
		0x00: []byte("vendor"),
		0x01: []byte("product"),
		0x02: []byte("revision"),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = NewRTUServer(handler).Serve(ctx, serverConn)
	}()
	client := NewClient(NewRTUTransport(clientConn), WithTimeout(time.Second))
	defer client.Close()

	info, err := client.ReadDeviceIdentification(ctx, ReadDeviceIDCodeBasic)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Objects) != 3 || string(info.Objects[0x00]) != "vendor" || string(info.Objects[0x02]) != "revision" {
		t.Fatalf("unexpected objects: %#v", info.Objects)
	}
}

func TestReadFIFOQueueRTU(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	handler := NewDataStoreHandler(NewMemoryDataStore())
	handler.FIFOQueues = map[uint16][]uint16{
		0x04DE: {100, 200, 300},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = NewRTUServer(handler).Serve(ctx, serverConn)
	}()
	client := NewClient(NewRTUTransport(clientConn), WithTimeout(time.Second))
	defer client.Close()

	values, err := client.ReadFIFOQueue(ctx, 0x04DE)
	if err != nil {
		t.Fatal(err)
	}
	if !uint16SlicesEqual(values, []uint16{100, 200, 300}) {
		t.Fatalf("unexpected fifo values: %#v", values)
	}
}

func TestRTUOverTCPClientServer(t *testing.T) {
	store := NewMemoryDataStore()
	if err := store.WriteHoldingRegisters(0, []uint16{10, 20, 30}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		defer ln.Close()
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				_ = NewRTUServer(NewDataStoreHandler(store)).Serve(ctx, conn)
			}()
		}
	}()

	client := NewClient(
		NewRTUOverTCPTransport(ln.Addr().String(), WithRTUOverTCPTimeout(time.Second)),
		WithUnitID(1),
		WithTimeout(time.Second),
	)
	defer client.Close()

	values, err := client.ReadHoldingRegisters(ctx, 0, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !uint16SlicesEqual(values, []uint16{10, 20, 30}) {
		t.Fatalf("unexpected holding registers: %#v", values)
	}
	if err := client.WriteSingleRegister(ctx, 1, 99); err != nil {
		t.Fatal(err)
	}
	values, err = client.ReadHoldingRegisters(ctx, 0, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !uint16SlicesEqual(values, []uint16{10, 99, 30}) {
		t.Fatalf("unexpected holding registers after write: %#v", values)
	}
}

func TestRTUOverTCPTransportDoesNotRetryCurrentRequestAfterConnectionError(t *testing.T) {
	ctx := context.Background()
	var dials atomic.Int32
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	transport := NewRTUOverTCPTransport("unused",
		WithRTUOverTCPDialer(func(context.Context, string, string) (net.Conn, error) {
			dials.Add(1)
			return net.Dial("tcp", ln.Addr().String())
		}),
		WithRTUOverTCPTimeout(time.Second),
	)
	client := NewClient(transport, WithTimeout(time.Second))
	defer client.Close()

	firstDone := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			firstDone <- err
			return
		}
		firstDone <- conn.Close()
	}()
	if _, err := client.ReadHoldingRegisters(ctx, 0, 1); err == nil {
		t.Fatal("expected first request to fail")
	}
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if got := dials.Load(); got != 1 {
		t.Fatalf("current request was retried: dials=%d", got)
	}

	store := NewMemoryDataStore()
	if err := store.WriteHoldingRegisters(0, []uint16{56}); err != nil {
		t.Fatal(err)
	}
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			_ = NewRTUServer(NewDataStoreHandler(store)).Serve(ctx, conn)
		}
	}()
	values, err := client.ReadHoldingRegisters(ctx, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := dials.Load(); got != 2 {
		t.Fatalf("next request did not redial exactly once: dials=%d", got)
	}
	if !uint16SlicesEqual(values, []uint16{56}) {
		t.Fatalf("unexpected values: %#v", values)
	}
}

func TestNewClientFromConfigTCP(t *testing.T) {
	store := NewMemoryDataStore()
	if err := store.WriteHoldingRegisters(0, []uint16{42}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = NewTCPServer(NewDataStoreHandler(store)).Serve(ctx, ln)
	}()

	client, err := NewClientFromConfig(ClientConfig{
		Mode:    ModeTCP,
		Address: ln.Addr().String(),
		UnitID:  1,
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	values, err := client.ReadHoldingRegisters(ctx, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !uint16SlicesEqual(values, []uint16{42}) {
		t.Fatalf("unexpected values: %#v", values)
	}
}

func TestNewClientFromConfigRTU(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	store := NewMemoryDataStore()
	if err := store.WriteHoldingRegisters(0, []uint16{43}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = NewRTUServer(NewDataStoreHandler(store)).Serve(ctx, serverConn)
	}()

	client, err := NewClientFromConfig(ClientConfig{
		Mode:    ModeRTU,
		Conn:    clientConn,
		UnitID:  1,
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	values, err := client.ReadHoldingRegisters(ctx, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !uint16SlicesEqual(values, []uint16{43}) {
		t.Fatalf("unexpected values: %#v", values)
	}
}

func TestNewClientFromConfigRTUOverTCP(t *testing.T) {
	store := NewMemoryDataStore()
	if err := store.WriteHoldingRegisters(0, []uint16{44}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		defer ln.Close()
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				_ = NewRTUServer(NewDataStoreHandler(store)).Serve(ctx, conn)
			}()
		}
	}()

	client, err := NewClientFromConfig(ClientConfig{
		Mode:    ModeRTUOverTCP,
		Address: ln.Addr().String(),
		UnitID:  1,
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	values, err := client.ReadHoldingRegisters(ctx, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !uint16SlicesEqual(values, []uint16{44}) {
		t.Fatalf("unexpected values: %#v", values)
	}
}

func TestNewTransportFromConfigValidation(t *testing.T) {
	tests := []ClientConfig{
		{Mode: ModeTCP},
		{Mode: ModeRTU},
		{Mode: ModeRTUOverTCP},
		{Mode: ClientMode("ascii")},
	}
	for _, config := range tests {
		if _, err := NewTransportFromConfig(config); err == nil {
			t.Fatalf("NewTransportFromConfig(%#v) expected error", config)
		}
	}
}

func TestReadWriteFileRecordsRTU(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	handler := NewDataStoreHandler(NewMemoryDataStore())
	handler.FileRecords = map[uint16][]uint16{
		7: {1, 2, 3, 4, 5},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = NewRTUServer(handler).Serve(ctx, serverConn)
	}()
	client := NewClient(NewRTUTransport(clientConn), WithTimeout(time.Second))
	defer client.Close()

	if err := client.WriteFileRecords(ctx, []FileRecord{{FileNumber: 7, RecordNumber: 1, Values: []uint16{20, 30}}}); err != nil {
		t.Fatal(err)
	}
	records, err := client.ReadFileRecords(ctx, []FileRecordRequest{{FileNumber: 7, RecordNumber: 0, RecordLength: 4}})
	if err != nil {
		t.Fatal(err)
	}
	if !uint16SlicesEqual(records[0].Values, []uint16{1, 20, 30, 4}) {
		t.Fatalf("unexpected file record values: %#v", records[0].Values)
	}
}

func TestDiagnosticsRTU(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	handler := NewDataStoreHandler(NewMemoryDataStore())
	status := byte(0xA5)
	handler.ExceptionStatus = &status
	handler.EnableDiagnostics = true
	handler.CommEventCounter = &CommEventCounter{Status: 0xFFFF, EventCount: 12}
	handler.CommEventLog = &CommEventLog{Status: 0x0000, EventCount: 12, MessageCount: 5, Events: []byte{0xAA, 0xBB}}
	handler.ServerID = []byte{0x01, 0xFF, 'R', 'T', 'U'}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = NewRTUServer(handler).Serve(ctx, serverConn)
	}()
	client := NewClient(NewRTUTransport(clientConn), WithTimeout(time.Second))
	defer client.Close()

	value, err := client.Diagnostic(ctx, 0x0000, 0xCAFE)
	if err != nil {
		t.Fatal(err)
	}
	if value != 0xCAFE {
		t.Fatalf("unexpected diagnostic value: 0x%04x", value)
	}
	gotStatus, err := client.ReadExceptionStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if gotStatus != status {
		t.Fatalf("unexpected exception status: 0x%02x", gotStatus)
	}
	counter, err := client.GetCommEventCounter(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if counter.EventCount != 12 {
		t.Fatalf("unexpected counter: %#v", counter)
	}
	log, err := client.GetCommEventLog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if log.MessageCount != 5 || !bytes.Equal(log.Events, []byte{0xAA, 0xBB}) {
		t.Fatalf("unexpected event log: %#v", log)
	}
	serverID, err := client.ReportServerID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(serverID, []byte{0x01, 0xFF, 'R', 'T', 'U'}) {
		t.Fatalf("unexpected server id: %#v", serverID)
	}
}

func TestRTUFrameScannerReportServerID(t *testing.T) {
	data, err := buildReportServerIDData([]byte{0x01, 0x02, 'I', 'D'})
	if err != nil {
		t.Fatal(err)
	}
	good := mustRTUFrame(t, 1, PDU{Function: FuncReportServerID, Data: data})
	frame, err := readRTUFrame(&chunkedReader{data: good, chunkSize: 1}, 1, FuncReportServerID)
	if err != nil {
		t.Fatal(err)
	}
	value, err := parseReportServerIDData(frame.PDU.Data)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(value, []byte{0x01, 0x02, 'I', 'D'}) {
		t.Fatalf("unexpected server id: %#v", value)
	}
}

func TestRTUFrameScannerCommEventLog(t *testing.T) {
	data, err := buildCommEventLogData(CommEventLog{Status: 0, EventCount: 1, MessageCount: 2, Events: []byte{0x10, 0x20}})
	if err != nil {
		t.Fatal(err)
	}
	good := mustRTUFrame(t, 1, PDU{Function: FuncGetCommEventLog, Data: data})
	frame, err := readRTUFrame(&chunkedReader{data: good, chunkSize: 1}, 1, FuncGetCommEventLog)
	if err != nil {
		t.Fatal(err)
	}
	log, err := parseCommEventLogData(frame.PDU.Data)
	if err != nil {
		t.Fatal(err)
	}
	if log.EventCount != 1 || !bytes.Equal(log.Events, []byte{0x10, 0x20}) {
		t.Fatalf("unexpected event log: %#v", log)
	}
}

func TestRTUFrameScannerReadFileRecord(t *testing.T) {
	data, err := buildReadFileRecordResponseData([]FileRecord{{FileNumber: 7, RecordNumber: 0, Values: []uint16{1, 2}}})
	if err != nil {
		t.Fatal(err)
	}
	good := mustRTUFrame(t, 1, PDU{Function: FuncReadFileRecord, Data: data})
	frame, err := readRTUFrame(&chunkedReader{data: good, chunkSize: 1}, 1, FuncReadFileRecord)
	if err != nil {
		t.Fatal(err)
	}
	records, err := parseReadFileRecordResponseData(frame.PDU.Data, []FileRecordRequest{{FileNumber: 7, RecordNumber: 0, RecordLength: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if !uint16SlicesEqual(records[0].Values, []uint16{1, 2}) {
		t.Fatalf("unexpected file record values: %#v", records[0].Values)
	}
}

func TestRTUFrameScannerFIFOQueue(t *testing.T) {
	good := mustRTUFrame(t, 1, buildReadFIFOQueueResponse([]uint16{1, 2, 3}))
	frame, err := readRTUFrame(&chunkedReader{data: good, chunkSize: 1}, 1, FuncReadFIFOQueue)
	if err != nil {
		t.Fatal(err)
	}
	values, err := parseFIFOQueueResponse(frame.PDU.Data)
	if err != nil {
		t.Fatal(err)
	}
	if !uint16SlicesEqual(values, []uint16{1, 2, 3}) {
		t.Fatalf("unexpected fifo values: %#v", values)
	}
}

func TestRTUFrameScannerDeviceIdentification(t *testing.T) {
	pdu, err := buildDeviceIdentificationResponse(ReadDeviceIDCodeBasic, 0x81, false, 0, map[byte][]byte{
		0x00: []byte("vendor"),
		0x01: []byte("product"),
	})
	if err != nil {
		t.Fatal(err)
	}
	good := mustRTUFrame(t, 1, pdu)
	frame, err := readRTUFrame(&chunkedReader{data: good, chunkSize: 1}, 1, FuncReadDeviceIdentification)
	if err != nil {
		t.Fatal(err)
	}
	info, err := parseDeviceIdentificationResponse(frame.PDU.Data)
	if err != nil {
		t.Fatal(err)
	}
	if info.ConformityLevel != 0x81 || string(info.Objects[0x01]) != "product" {
		t.Fatalf("unexpected device identification: %#v", info)
	}
}

func TestRTUFrameScannerSkipsNoise(t *testing.T) {
	good := mustRTUFrame(t, 1, PDU{Function: FuncReadHoldingRegisters, Data: []byte{0x02, 0x00, 0x2A}})
	input := append([]byte{0x99, 0x01, 0x7F, 0x00}, good...)
	frame, err := readRTUFrame(bytes.NewReader(input), 1, FuncReadHoldingRegisters)
	if err != nil {
		t.Fatal(err)
	}
	if frame.UnitID != 1 || frame.PDU.Function != FuncReadHoldingRegisters || !bytes.Equal(frame.PDU.Data, []byte{0x02, 0x00, 0x2A}) {
		t.Fatalf("unexpected frame: %#v", frame)
	}
}

func TestRTUFrameScannerSkipsBadCRCFrame(t *testing.T) {
	bad := mustRTUFrame(t, 1, PDU{Function: FuncReadHoldingRegisters, Data: []byte{0x02, 0x00, 0x01}})
	bad[len(bad)-1] ^= 0xFF
	good := mustRTUFrame(t, 1, PDU{Function: FuncReadHoldingRegisters, Data: []byte{0x02, 0x00, 0x02}})
	input := append(bad, good...)
	frame, err := readRTUFrame(bytes.NewReader(input), 1, FuncReadHoldingRegisters)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(frame.PDU.Data, []byte{0x02, 0x00, 0x02}) {
		t.Fatalf("unexpected data: %#v", frame.PDU.Data)
	}
}

func TestRTUFrameScannerException(t *testing.T) {
	exception := mustRTUFrame(t, 1, PDU{Function: FuncReadHoldingRegisters | 0x80, Data: []byte{byte(ExceptionIllegalDataAddress)}})
	frame, err := readRTUFrame(bytes.NewReader(exception), 1, FuncReadHoldingRegisters)
	if err != nil {
		t.Fatal(err)
	}
	err = parseException(frame.PDU, FuncReadHoldingRegisters)
	var ex *ExceptionError
	if !errors.As(err, &ex) || ex.Code != ExceptionIllegalDataAddress {
		t.Fatalf("expected illegal data address exception, got %v", err)
	}
}

func TestRTUFrameScannerChunkedRead(t *testing.T) {
	good := mustRTUFrame(t, 1, PDU{Function: FuncReadHoldingRegisters, Data: []byte{0x02, 0x00, 0x04}})
	frame, err := readRTUFrame(&chunkedReader{data: good, chunkSize: 1}, 1, FuncReadHoldingRegisters)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(frame.PDU.Data, []byte{0x02, 0x00, 0x04}) {
		t.Fatalf("unexpected data: %#v", frame.PDU.Data)
	}
}

func TestRTUFrameScannerFunctionMismatchThenGoodFrame(t *testing.T) {
	wrongFunction := mustRTUFrame(t, 1, PDU{Function: FuncReadCoils, Data: []byte{0x01, 0x01}})
	good := mustRTUFrame(t, 1, PDU{Function: FuncReadHoldingRegisters, Data: []byte{0x02, 0x00, 0x03}})
	input := append(wrongFunction, good...)
	frame, err := readRTUFrame(bytes.NewReader(input), 1, FuncReadHoldingRegisters)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(frame.PDU.Data, []byte{0x02, 0x00, 0x03}) {
		t.Fatalf("unexpected data: %#v", frame.PDU.Data)
	}
}

type chunkedReader struct {
	data      []byte
	chunkSize int
}

func (r *chunkedReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := r.chunkSize
	if n <= 0 || n > len(r.data) {
		n = len(r.data)
	}
	if n > len(p) {
		n = len(p)
	}
	copy(p, r.data[:n])
	r.data = r.data[n:]
	return n, nil
}

func mustRTUFrame(t *testing.T, unitID byte, pdu PDU) []byte {
	t.Helper()
	frame, err := RTUCodec{}.Encode(unitID, 0, pdu)
	if err != nil {
		t.Fatal(err)
	}
	return frame
}

func uint16SlicesEqual(a, b []uint16) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

type countingHandler struct {
	next              Handler
	readHoldingCount  atomic.Int32
	writeHoldingCount atomic.Int32
	writeCoilCount    atomic.Int32
	lastAddress       atomic.Uint32
	lastQuantity      atomic.Uint32
	lastWriteAddress  atomic.Uint32
	lastWriteQuantity atomic.Uint32
}

func (h *countingHandler) Handle(ctx context.Context, unitID byte, request PDU) (PDU, error) {
	if request.Function == FuncReadHoldingRegisters && len(request.Data) == 4 {
		h.readHoldingCount.Add(1)
		h.lastAddress.Store(uint32(uint16At(request.Data[0:])))
		h.lastQuantity.Store(uint32(uint16At(request.Data[2:])))
	}
	if request.Function == FuncWriteMultipleRegisters && len(request.Data) >= 5 {
		h.writeHoldingCount.Add(1)
		h.lastWriteAddress.Store(uint32(uint16At(request.Data[0:])))
		h.lastWriteQuantity.Store(uint32(uint16At(request.Data[2:])))
	}
	if request.Function == FuncWriteMultipleCoils && len(request.Data) >= 5 {
		h.writeCoilCount.Add(1)
		h.lastWriteAddress.Store(uint32(uint16At(request.Data[0:])))
		h.lastWriteQuantity.Store(uint32(uint16At(request.Data[2:])))
	}
	return h.next.Handle(ctx, unitID, request)
}
