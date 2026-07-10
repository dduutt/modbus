package modbus

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestReliabilityTCPScenarioMatrix(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := NewMemoryDataStoreSized(512, 512, 512, 512)
	must(t, store.WriteCoils(0, []bool{true, false, true, false, true, false}))
	must(t, store.WriteDiscreteInputs(0, []bool{false, true, false, true}))
	must(t, store.WriteHoldingRegisters(0, []uint16{100, 200, 300, 400, 500, 600}))
	must(t, store.WriteInputRegisters(0, []uint16{11, 22, 33, 44}))

	exceptionStatus := byte(0x5A)
	handler := NewDataStoreHandler(store)
	handler.ExceptionStatus = &exceptionStatus
	handler.EnableDiagnostics = true
	handler.CommEventCounter = &CommEventCounter{Status: 0xFFFF, EventCount: 12}
	handler.CommEventLog = &CommEventLog{
		Status:       0,
		EventCount:   12,
		MessageCount: 5,
		Events:       []byte{0xAA, 0xBB},
	}
	handler.ServerID = []byte{0x01, 0xFF, 'G', 'o'}
	handler.FileRecords = map[uint16][]uint16{7: {10, 20, 30, 40}}
	handler.FIFOQueues = map[uint16][]uint16{0x04DE: {100, 200, 300}}
	handler.DeviceIdentification = map[byte][]byte{
		0x00: []byte("Vendor"),
		0x01: []byte("Product"),
		0x02: []byte("1.0.0"),
	}

	client := newTCPReliabilityClient(t, ctx, handler)
	defer client.Close()

	if got, err := client.ReadCoils(ctx, 0, 6); err != nil || !reflectBoolSlicesEqual(got, []bool{true, false, true, false, true, false}) {
		t.Fatalf("ReadCoils()=%#v, %v", got, err)
	}
	if got, err := client.ReadDiscreteInputs(ctx, 0, 4); err != nil || !reflectBoolSlicesEqual(got, []bool{false, true, false, true}) {
		t.Fatalf("ReadDiscreteInputs()=%#v, %v", got, err)
	}
	if got, err := client.ReadHoldingRegisters(ctx, 0, 6); err != nil || !uint16SlicesEqual(got, []uint16{100, 200, 300, 400, 500, 600}) {
		t.Fatalf("ReadHoldingRegisters()=%#v, %v", got, err)
	}
	if got, err := client.ReadInputRegisters(ctx, 0, 4); err != nil || !uint16SlicesEqual(got, []uint16{11, 22, 33, 44}) {
		t.Fatalf("ReadInputRegisters()=%#v, %v", got, err)
	}

	must(t, client.WriteSingleCoil(ctx, 1, true))
	must(t, client.WriteMultipleCoils(ctx, 8, []bool{true, true, false, true}))
	must(t, client.WriteSingleRegister(ctx, 1, 222))
	must(t, client.WriteMultipleRegisters(ctx, 10, []uint16{1, 2, 3, 4}))
	must(t, client.MaskWriteRegister(ctx, 10, 0x0FF0, 0x0005))
	if got, err := client.ReadHoldingRegisters(ctx, 10, 4); err != nil || !uint16SlicesEqual(got, []uint16{5, 2, 3, 4}) {
		t.Fatalf("post-write holding registers=%#v, %v", got, err)
	}

	got, err := client.ReadWriteMultipleRegisters(ctx, 10, 2, 20, []uint16{77, 88})
	if err != nil {
		t.Fatal(err)
	}
	if !uint16SlicesEqual(got, []uint16{5, 2}) {
		t.Fatalf("ReadWriteMultipleRegisters()=%#v", got)
	}

	if got, err := client.Diagnostic(ctx, 0, 0xCAFE); err != nil || got != 0xCAFE {
		t.Fatalf("Diagnostic()=0x%04X, %v", got, err)
	}
	if got, err := client.ReadExceptionStatus(ctx); err != nil || got != 0x5A {
		t.Fatalf("ReadExceptionStatus()=0x%02X, %v", got, err)
	}
	if got, err := client.GetCommEventCounter(ctx); err != nil || got.Status != 0xFFFF || got.EventCount != 12 {
		t.Fatalf("GetCommEventCounter()=%#v, %v", got, err)
	}
	if got, err := client.GetCommEventLog(ctx); err != nil || got.MessageCount != 5 || !bytesEqual(got.Events, []byte{0xAA, 0xBB}) {
		t.Fatalf("GetCommEventLog()=%#v, %v", got, err)
	}
	if got, err := client.ReportServerID(ctx); err != nil || !bytesEqual(got, []byte{0x01, 0xFF, 'G', 'o'}) {
		t.Fatalf("ReportServerID()=% X, %v", got, err)
	}

	must(t, client.WriteFileRecords(ctx, []FileRecord{{FileNumber: 7, RecordNumber: 1, Values: []uint16{21, 31}}}))
	records, err := client.ReadFileRecords(ctx, []FileRecordRequest{{FileNumber: 7, RecordNumber: 0, RecordLength: 4}})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || !uint16SlicesEqual(records[0].Values, []uint16{10, 21, 31, 40}) {
		t.Fatalf("ReadFileRecords()=%#v", records)
	}
	if got, err := client.ReadFIFOQueue(ctx, 0x04DE); err != nil || !uint16SlicesEqual(got, []uint16{100, 200, 300}) {
		t.Fatalf("ReadFIFOQueue()=%#v, %v", got, err)
	}
	info, err := client.ReadDeviceIdentification(ctx, ReadDeviceIDCodeBasic)
	if err != nil {
		t.Fatal(err)
	}
	if string(info.Objects[0x00]) != "Vendor" || string(info.Objects[0x01]) != "Product" || string(info.Objects[0x02]) != "1.0.0" {
		t.Fatalf("ReadDeviceIdentification()=%#v", info.Objects)
	}
}

func TestReliabilityRTUOverTCPTagBatching(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := NewMemoryDataStoreSized(256, 256, 256, 256)
	client := newRTUOverTCPReliabilityClient(t, ctx, NewDataStoreHandler(store))
	defer client.Close()

	must(t, client.WriteTags(ctx, map[string]TagValue{
		"temperature": {Tag: HoldingRegister(0).As(TypeFloat32), Value: float32(26.75)},
		"pressure":    {Tag: HoldingRegister(2).As(TypeUInt32), Value: uint32(123456)},
		"running":     {Tag: Coil(0), Value: true},
		"fault":       {Tag: Coil(1), Value: false},
	}))

	values, err := client.ReadTags(ctx, map[string]Tag{
		"temperature": HoldingRegister(0).As(TypeFloat32),
		"pressure":    HoldingRegister(2).As(TypeUInt32),
		"running":     Coil(0),
		"fault":       Coil(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := values["temperature"].Float32(); !ok || got != float32(26.75) {
		t.Fatalf("temperature=%#v ok=%v", got, ok)
	}
	if got, ok := values["pressure"].UInt32(); !ok || got != uint32(123456) {
		t.Fatalf("pressure=%#v ok=%v", got, ok)
	}
	if got, ok := values["running"].Bool(); !ok || !got {
		t.Fatalf("running=%#v ok=%v", got, ok)
	}
	if got, ok := values["fault"].Bool(); !ok || got {
		t.Fatalf("fault=%#v ok=%v", got, ok)
	}
}

func TestReliabilityTagAcquisitionAccuracy(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := NewMemoryDataStoreSized(256, 256, 256, 256)
	must(t, store.WriteCoils(0, []bool{true, false, true, true}))
	must(t, store.WriteDiscreteInputs(0, []bool{false, true, true, false}))

	seedTagValue(t, store, HoldingRegister(0).As(TypeUInt16), uint16(0x1234))
	seedTagValue(t, store, HoldingRegister(1).As(TypeInt16), int16(-1234))
	seedTagValue(t, store, HoldingRegister(10).As(TypeUInt32), uint32(0x11223344))
	seedTagValue(t, store, HoldingRegister(12).As(TypeInt32), int32(-1234567))
	seedTagValue(t, store, HoldingRegister(14).As(TypeFloat32), float32(26.75))
	seedTagValue(t, store, HoldingRegister(20).As(TypeUInt64).WithOrder(ByteOrderLittleEndian, WordOrderLowFirst), uint64(0x1122334455667788))
	seedTagValue(t, store, HoldingRegister(24).As(TypeInt64), int64(-9876543210))
	seedTagValue(t, store, HoldingRegister(28).As(TypeFloat64), float64(12345.625))
	seedTagValue(t, store, HoldingRegister(40).As(TypeBytes).WithQuantity(3), []byte{0xDE, 0xAD, 0xBE, 0xEF, 0xCA, 0xFE})
	seedTagValue(t, store, HoldingRegister(50).As(TypeString).WithQuantity(4), "MODBUS")
	seedTagValue(t, store, InputRegister(0).As(TypeFloat32), float32(-12.5))
	seedTagValue(t, store, InputRegister(10).As(TypeUInt32).WithOrder(ByteOrderLittleEndian, WordOrderLowFirst), uint32(0xAABBCCDD))

	client := newTCPReliabilityClient(t, ctx, NewDataStoreHandler(store))
	defer client.Close()

	values, err := client.ReadTags(ctx, map[string]Tag{
		"coil":             Coil(0),
		"coil_slice":       Coil(0).WithQuantity(4),
		"discrete":         DiscreteInput(1),
		"discrete_slice":   DiscreteInput(0).WithQuantity(4),
		"u16":              HoldingRegister(0).As(TypeUInt16),
		"i16":              HoldingRegister(1).As(TypeInt16),
		"u32":              HoldingRegister(10).As(TypeUInt32),
		"i32":              HoldingRegister(12).As(TypeInt32),
		"f32":              HoldingRegister(14).As(TypeFloat32),
		"u64_le_low":       HoldingRegister(20).As(TypeUInt64).WithOrder(ByteOrderLittleEndian, WordOrderLowFirst),
		"i64":              HoldingRegister(24).As(TypeInt64),
		"f64":              HoldingRegister(28).As(TypeFloat64),
		"bytes":            HoldingRegister(40).As(TypeBytes).WithQuantity(3),
		"string":           HoldingRegister(50).As(TypeString).WithQuantity(4),
		"input_f32":        InputRegister(0).As(TypeFloat32),
		"input_u32_le_low": InputRegister(10).As(TypeUInt32).WithOrder(ByteOrderLittleEndian, WordOrderLowFirst),
	})
	if err != nil {
		t.Fatal(err)
	}

	assertBoolValue(t, values, "coil", true)
	assertBoolSliceValue(t, values, "coil_slice", []bool{true, false, true, true})
	assertBoolValue(t, values, "discrete", true)
	assertBoolSliceValue(t, values, "discrete_slice", []bool{false, true, true, false})
	assertUInt16Value(t, values, "u16", 0x1234)
	assertInt16Value(t, values, "i16", -1234)
	assertUInt32Value(t, values, "u32", 0x11223344)
	assertInt32Value(t, values, "i32", -1234567)
	assertFloat32Value(t, values, "f32", 26.75)
	assertUInt64Value(t, values, "u64_le_low", 0x1122334455667788)
	assertInt64Value(t, values, "i64", -9876543210)
	assertFloat64Value(t, values, "f64", 12345.625)
	assertBytesValue(t, values, "bytes", []byte{0xDE, 0xAD, 0xBE, 0xEF, 0xCA, 0xFE})
	assertStringValue(t, values, "string", "MODBUS")
	assertFloat32Value(t, values, "input_f32", -12.5)
	assertUInt32Value(t, values, "input_u32_le_low", 0xAABBCCDD)
}

func TestReliabilityTagAcquisitionErrorDoesNotReturnPartialData(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := NewMemoryDataStoreSized(8, 8, 4, 4)
	must(t, store.WriteCoils(0, []bool{true}))
	must(t, store.WriteHoldingRegisters(0, []uint16{1234}))

	client := newTCPReliabilityClient(t, ctx, NewDataStoreHandler(store))
	defer client.Close()

	values, err := client.ReadTags(ctx, map[string]Tag{
		"valid_coil": Coil(0),
		"bad_hr":     HoldingRegister(20).As(TypeUInt16),
	})
	if err == nil {
		t.Fatal("expected acquisition error")
	}
	if values != nil {
		t.Fatalf("ReadTags returned partial data on failure: %#v", values)
	}
	var ex *ExceptionError
	if !errors.As(err, &ex) || ex.Code != ExceptionIllegalDataAddress {
		t.Fatalf("unexpected error: %T %[1]v", err)
	}
}

func TestReliabilityConcurrentTCPClients(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const workers = 32
	const opsPerWorker = 200
	const registerCount = 1024
	const coilCount = 1024

	store := NewMemoryDataStoreSized(coilCount, coilCount, registerCount, registerCount)
	initial := make([]uint16, registerCount)
	for i := range initial {
		initial[i] = uint16(i)
	}
	must(t, store.WriteHoldingRegisters(0, initial))

	addr := startReliabilityTCPServer(t, ctx, NewDataStoreHandler(store))
	var okOps atomic.Uint64
	errCh := make(chan error, workers)
	var wg sync.WaitGroup

	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			client := NewClient(
				NewTCPTransport(addr),
				WithUnitID(1),
				WithTimeout(5*time.Second),
			)
			defer client.Close()

			for i := 0; i < opsPerWorker; i++ {
				base := uint16((worker*37 + i*11) % (registerCount - 16))
				coilBase := uint16((worker*41 + i*7) % (coilCount - 16))
				var err error
				switch i % 6 {
				case 0:
					_, err = client.ReadHoldingRegisters(ctx, base, 16)
				case 1:
					err = client.WriteSingleRegister(ctx, base, uint16(worker+i))
				case 2:
					err = client.WriteMultipleRegisters(ctx, base, []uint16{1, 2, 3, 4, 5, 6})
				case 3:
					err = client.WriteMultipleCoils(ctx, coilBase, []bool{true, false, true, true, false, false, true, false})
				case 4:
					tag := HoldingRegister(base).As(TypeFloat32)
					if err = client.WriteTag(ctx, tag, float32(worker)+float32(i)/1000); err == nil {
						_, err = client.ReadTag(ctx, tag)
					}
				case 5:
					_, err = client.ReadWriteMultipleRegisters(ctx, base, 4, base+4, []uint16{9, 8, 7, 6})
				}
				if err != nil {
					errCh <- err
					return
				}
				okOps.Add(1)
			}
		}(worker)
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got, want := okOps.Load(), uint64(workers*opsPerWorker); got != want {
		t.Fatalf("completed ops=%d want %d", got, want)
	}
}

func newTCPReliabilityClient(t *testing.T, ctx context.Context, handler Handler) *Client {
	t.Helper()
	return NewClient(
		NewTCPTransport(startReliabilityTCPServer(t, ctx, handler)),
		WithUnitID(1),
		WithTimeout(3*time.Second),
	)
}

func startReliabilityTCPServer(t *testing.T, ctx context.Context, handler Handler) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() { _ = NewTCPServer(handler).Serve(ctx, ln) }()
	return ln.Addr().String()
}

func newRTUOverTCPReliabilityClient(t *testing.T, ctx context.Context, handler Handler) *Client {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { _ = NewRTUServer(handler).Serve(ctx, conn) }()
		}
	}()
	return NewClient(
		NewRTUOverTCPTransport(ln.Addr().String(), WithRTUOverTCPTimeout(3*time.Second)),
		WithUnitID(1),
		WithTimeout(3*time.Second),
	)
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func seedTagValue(t *testing.T, store *MemoryDataStore, tag Tag, value any) {
	t.Helper()
	raw, err := EncodeValue(tag, value)
	if err != nil {
		t.Fatal(err)
	}
	switch tag.Area {
	case AreaHoldingRegister:
		must(t, store.WriteHoldingRegisters(tag.Address, raw.Registers))
	case AreaInputRegister:
		must(t, store.WriteInputRegisters(tag.Address, raw.Registers))
	default:
		t.Fatalf("unsupported seed area %s", tag.Area)
	}
}

func assertBoolValue(t *testing.T, values map[string]Value, name string, want bool) {
	t.Helper()
	got, ok := values[name].Bool()
	if !ok || got != want {
		t.Fatalf("%s=%#v ok=%v want %v", name, got, ok, want)
	}
}

func assertBoolSliceValue(t *testing.T, values map[string]Value, name string, want []bool) {
	t.Helper()
	got, ok := values[name].Bools()
	if !ok || !reflectBoolSlicesEqual(got, want) {
		t.Fatalf("%s=%#v ok=%v want %#v", name, got, ok, want)
	}
}

func assertUInt16Value(t *testing.T, values map[string]Value, name string, want uint16) {
	t.Helper()
	got, ok := values[name].UInt16()
	if !ok || got != want {
		t.Fatalf("%s=%#v ok=%v want %#v", name, got, ok, want)
	}
}

func assertInt16Value(t *testing.T, values map[string]Value, name string, want int16) {
	t.Helper()
	got, ok := values[name].Int16()
	if !ok || got != want {
		t.Fatalf("%s=%#v ok=%v want %#v", name, got, ok, want)
	}
}

func assertUInt32Value(t *testing.T, values map[string]Value, name string, want uint32) {
	t.Helper()
	got, ok := values[name].UInt32()
	if !ok || got != want {
		t.Fatalf("%s=%#v ok=%v want %#v", name, got, ok, want)
	}
}

func assertInt32Value(t *testing.T, values map[string]Value, name string, want int32) {
	t.Helper()
	got, ok := values[name].Int32()
	if !ok || got != want {
		t.Fatalf("%s=%#v ok=%v want %#v", name, got, ok, want)
	}
}

func assertFloat32Value(t *testing.T, values map[string]Value, name string, want float32) {
	t.Helper()
	got, ok := values[name].Float32()
	if !ok || got != want {
		t.Fatalf("%s=%#v ok=%v want %#v", name, got, ok, want)
	}
}

func assertUInt64Value(t *testing.T, values map[string]Value, name string, want uint64) {
	t.Helper()
	got, ok := values[name].UInt64()
	if !ok || got != want {
		t.Fatalf("%s=%#v ok=%v want %#v", name, got, ok, want)
	}
}

func assertInt64Value(t *testing.T, values map[string]Value, name string, want int64) {
	t.Helper()
	got, ok := values[name].Int64()
	if !ok || got != want {
		t.Fatalf("%s=%#v ok=%v want %#v", name, got, ok, want)
	}
}

func assertFloat64Value(t *testing.T, values map[string]Value, name string, want float64) {
	t.Helper()
	got, ok := values[name].Float64()
	if !ok || got != want {
		t.Fatalf("%s=%#v ok=%v want %#v", name, got, ok, want)
	}
}

func assertBytesValue(t *testing.T, values map[string]Value, name string, want []byte) {
	t.Helper()
	got, ok := values[name].Bytes()
	if !ok || !bytesEqual(got, want) {
		t.Fatalf("%s=%#v ok=%v want %#v", name, got, ok, want)
	}
}

func assertStringValue(t *testing.T, values map[string]Value, name string, want string) {
	t.Helper()
	got, ok := values[name].String()
	if !ok || got != want {
		t.Fatalf("%s=%#v ok=%v want %#v", name, got, ok, want)
	}
}

func reflectBoolSlicesEqual(a, b []bool) bool {
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

func bytesEqual(a, b []byte) bool {
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
