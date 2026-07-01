package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"reflect"
	"time"

	"github.com/dduutt/modbus"
	modbusserial "github.com/dduutt/modbus/serial"
)

type testEnv struct {
	ctx    context.Context
	client *modbus.Client
	store  *modbus.MemoryDataStore
}

func main() {
	serialPort := flag.String("serial", "COM4", "local serial port used by the RTU slave")
	gateway := flag.String("gateway", "10.83.2.40:1234", "serial server TCP address")
	unitID := flag.Int("unit", 1, "Modbus unit id")
	baudRate := flag.Int("baud", 9600, "serial baud rate")
	timeout := flag.Duration("timeout", 3*time.Second, "client request timeout")
	startupDelay := flag.Duration("startup-delay", 500*time.Millisecond, "delay after starting the local RTU slave")
	suite := flag.String("suite", "basic", "test suite: basic, advanced, boundary, stress, all")
	boundaryBits := flag.Int("boundary-bits", 2000, "boundary suite bit read quantity")
	boundaryRegisters := flag.Int("boundary-registers", 125, "boundary suite register read quantity")
	boundaryWriteBits := flag.Int("boundary-write-bits", 1968, "boundary suite multiple coil write quantity")
	boundaryWriteRegisters := flag.Int("boundary-write-registers", 123, "boundary suite multiple register write quantity")
	stressCount := flag.Int("stress-count", 100, "stress suite iteration count")
	stressDelay := flag.Duration("stress-delay", 100*time.Millisecond, "delay between stress iterations")
	flag.Parse()
	if *unitID <= 0 || *unitID > 247 {
		log.Fatalf("unit id must be 1..247, got %d", *unitID)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	port, err := modbusserial.Open(modbusserial.Config{
		PortName: *serialPort,
		BaudRate: *baudRate,
		DataBits: 8,
		StopBits: modbusserial.OneStopBit,
		Parity:   modbusserial.NoParity,
	})
	if err != nil {
		log.Fatalf("open local serial port %s: %v", *serialPort, err)
	}
	defer port.Close()

	store := newTestStore()
	handler := newTestHandler(store)
	go func() {
		server := modbus.NewRTUServer(handler)
		if err := server.Serve(ctx, port); err != nil {
			log.Printf("RTU slave stopped: %v", err)
		}
	}()
	time.Sleep(*startupDelay)

	client := modbus.NewClient(
		modbus.NewRTUOverTCPTransport(*gateway),
		modbus.WithUnitID(byte(*unitID)),
		modbus.WithTimeout(*timeout),
	)
	defer client.Close()

	env := testEnv{ctx: ctx, client: client, store: store}
	fmt.Printf("testing suite=%s gateway=%s local_serial=%s unit=%d baud=%d\n", *suite, *gateway, *serialPort, *unitID, *baudRate)

	switch *suite {
	case "basic":
		runBasic(env)
	case "advanced":
		runAdvanced(env)
	case "boundary":
		runBoundary(env, boundaryOptions{
			Bits:           *boundaryBits,
			Registers:      *boundaryRegisters,
			WriteBits:      *boundaryWriteBits,
			WriteRegisters: *boundaryWriteRegisters,
		})
	case "stress":
		runStress(env, *stressCount, *stressDelay)
	case "all":
		runBasic(env)
		runAdvanced(env)
		runBoundary(env, boundaryOptions{
			Bits:           *boundaryBits,
			Registers:      *boundaryRegisters,
			WriteBits:      *boundaryWriteBits,
			WriteRegisters: *boundaryWriteRegisters,
		})
	default:
		log.Fatalf("unknown suite %q", *suite)
	}

	fmt.Printf("serial gateway %s suite passed\n", *suite)
}

func newTestStore() *modbus.MemoryDataStore {
	store := modbus.NewMemoryDataStore()
	must(store.WriteHoldingRegisters(0, []uint16{100, 200, 300}))
	must(store.WriteHoldingRegisters(100, sequenceRegisters(125)))
	must(store.WriteInputRegisters(0, []uint16{400, 500, 600}))
	must(store.WriteInputRegisters(100, sequenceRegisters(125)))
	must(store.WriteCoils(0, []bool{true, false, true, false}))
	must(store.WriteCoils(100, alternatingBools(1968)))
	must(store.WriteDiscreteInputs(0, []bool{false, true, false, true}))
	must(store.WriteDiscreteInputs(100, alternatingBools(2000)))
	return store
}

func newTestHandler(store *modbus.MemoryDataStore) *modbus.DataStoreHandler {
	status := byte(0x5A)
	handler := modbus.NewDataStoreHandler(store)
	handler.ExceptionStatus = &status
	handler.EnableDiagnostics = true
	handler.DiagnosticResponses = map[uint16]uint16{0x0001: 0x1234}
	handler.CommEventCounter = &modbus.CommEventCounter{Status: 0xFFFF, EventCount: 12}
	handler.CommEventLog = &modbus.CommEventLog{
		Status:       0,
		EventCount:   12,
		MessageCount: 5,
		Events:       []byte{0xAA, 0xBB},
	}
	handler.ServerID = []byte{0x01, 0xFF, 'G', 'W'}
	handler.FileRecords = map[uint16][]uint16{
		7: []uint16{10, 20, 30, 40, 50},
	}
	handler.FIFOQueues = map[uint16][]uint16{
		0x04DE: []uint16{100, 200, 300},
	}
	handler.DeviceIdentification = map[byte][]byte{
		0x00: []byte("LocalVendor"),
		0x01: []byte("SerialGatewayLoopback"),
		0x02: []byte("1.0.0"),
	}
	return handler
}

func runBasic(env testEnv) {
	values, err := env.client.ReadHoldingRegisters(env.ctx, 0, 3)
	check("read holding registers", err)
	expect("holding registers", values, []uint16{100, 200, 300})

	check("write single register", env.client.WriteSingleRegister(env.ctx, 1, 222))
	values, err = env.client.ReadHoldingRegisters(env.ctx, 0, 3)
	check("read holding registers after write", err)
	expect("holding registers after write", values, []uint16{100, 222, 300})

	coils, err := env.client.ReadCoils(env.ctx, 0, 4)
	check("read coils", err)
	expect("coils", coils, []bool{true, false, true, false})

	check("write single coil", env.client.WriteSingleCoil(env.ctx, 1, true))
	coils, err = env.client.ReadCoils(env.ctx, 0, 4)
	check("read coils after write", err)
	expect("coils after write", coils, []bool{true, true, true, false})

	info, err := env.client.ReadDeviceIdentification(env.ctx, modbus.ReadDeviceIDCodeBasic)
	check("read device identification", err)
	vendor := string(info.Objects[0x00])
	if vendor != "LocalVendor" {
		log.Fatalf("device vendor mismatch: got %q", vendor)
	}
	fmt.Printf("device identification vendor: %s\n", vendor)
}

func runAdvanced(env testEnv) {
	discrete, err := env.client.ReadDiscreteInputs(env.ctx, 0, 4)
	check("read discrete inputs", err)
	expect("discrete inputs", discrete, []bool{false, true, false, true})

	inputs, err := env.client.ReadInputRegisters(env.ctx, 0, 3)
	check("read input registers", err)
	expect("input registers", inputs, []uint16{400, 500, 600})

	check("write multiple coils", env.client.WriteMultipleCoils(env.ctx, 10, []bool{true, true, false, true}))
	coils, err := env.client.ReadCoils(env.ctx, 10, 4)
	check("read multiple coils after write", err)
	expect("multiple coils", coils, []bool{true, true, false, true})

	check("write multiple registers", env.client.WriteMultipleRegisters(env.ctx, 10, []uint16{11, 22, 33}))
	values, err := env.client.ReadHoldingRegisters(env.ctx, 10, 3)
	check("read multiple registers after write", err)
	expect("multiple registers", values, []uint16{11, 22, 33})

	check("mask write register", env.client.MaskWriteRegister(env.ctx, 10, 0x0FF0, 0x0005))
	values, err = env.client.ReadHoldingRegisters(env.ctx, 10, 1)
	check("read masked register", err)
	expect("masked register", values, []uint16{0x0005})

	values, err = env.client.ReadWriteMultipleRegisters(env.ctx, 10, 2, 20, []uint16{77, 88})
	check("read/write multiple registers", err)
	expect("read/write result", values, []uint16{0x0005, 22})
	values, err = env.client.ReadHoldingRegisters(env.ctx, 20, 2)
	check("read written registers", err)
	expect("read/write written registers", values, []uint16{77, 88})

	status, err := env.client.ReadExceptionStatus(env.ctx)
	check("read exception status", err)
	if status != 0x5A {
		log.Fatalf("exception status mismatch: got 0x%02X", status)
	}

	diagnostic, err := env.client.Diagnostic(env.ctx, 0x0001, 0)
	check("diagnostic", err)
	if diagnostic != 0x1234 {
		log.Fatalf("diagnostic mismatch: got 0x%04X", diagnostic)
	}

	counter, err := env.client.GetCommEventCounter(env.ctx)
	check("comm event counter", err)
	if counter.EventCount != 12 {
		log.Fatalf("counter mismatch: %#v", counter)
	}

	eventLog, err := env.client.GetCommEventLog(env.ctx)
	check("comm event log", err)
	expect("comm event log events", eventLog.Events, []byte{0xAA, 0xBB})

	serverID, err := env.client.ReportServerID(env.ctx)
	check("report server id", err)
	expect("server id", serverID, []byte{0x01, 0xFF, 'G', 'W'})

	records, err := env.client.ReadFileRecords(env.ctx, []modbus.FileRecordRequest{{FileNumber: 7, RecordNumber: 1, RecordLength: 2}})
	check("read file records", err)
	expect("file record values", records[0].Values, []uint16{20, 30})

	check("write file records", env.client.WriteFileRecords(env.ctx, []modbus.FileRecord{{FileNumber: 7, RecordNumber: 1, Values: []uint16{21, 31}}}))
	records, err = env.client.ReadFileRecords(env.ctx, []modbus.FileRecordRequest{{FileNumber: 7, RecordNumber: 0, RecordLength: 4}})
	check("read file records after write", err)
	expect("file record values after write", records[0].Values, []uint16{10, 21, 31, 40})

	fifoValues, err := env.client.ReadFIFOQueue(env.ctx, 0x04DE)
	check("read fifo queue", err)
	expect("fifo values", fifoValues, []uint16{100, 200, 300})

	tag := modbus.HoldingRegister(30).As(modbus.TypeFloat32)
	check("write tag float32", env.client.WriteTag(env.ctx, tag, float32(26.75)))
	value, err := env.client.ReadTag(env.ctx, tag)
	check("read tag float32", err)
	got, ok := value.Float32()
	if !ok || math.Abs(float64(got-26.75)) > 0.001 {
		log.Fatalf("float32 tag mismatch: %#v ok=%v", value, ok)
	}
	fmt.Printf("tag float32: %.2f\n", got)
}

type boundaryOptions struct {
	Bits           int
	Registers      int
	WriteBits      int
	WriteRegisters int
}

func runBoundary(env testEnv, opts boundaryOptions) {
	if opts.Bits <= 0 || opts.Bits > 2000 {
		log.Fatalf("boundary-bits must be 1..2000, got %d", opts.Bits)
	}
	if opts.Registers <= 0 || opts.Registers > 125 {
		log.Fatalf("boundary-registers must be 1..125, got %d", opts.Registers)
	}
	if opts.WriteBits <= 0 || opts.WriteBits > 1968 {
		log.Fatalf("boundary-write-bits must be 1..1968, got %d", opts.WriteBits)
	}
	if opts.WriteRegisters <= 0 || opts.WriteRegisters > 123 {
		log.Fatalf("boundary-write-registers must be 1..123, got %d", opts.WriteRegisters)
	}
	bitQuantity := uint16(opts.Bits)
	registerQuantity := uint16(opts.Registers)
	writeBitQuantity := uint16(opts.WriteBits)
	writeRegisterQuantity := uint16(opts.WriteRegisters)

	coils, err := env.client.ReadCoils(env.ctx, 100, bitQuantity)
	check(fmt.Sprintf("read %d coils", bitQuantity), err)
	if len(coils) != int(bitQuantity) {
		log.Fatalf("coil boundary length mismatch: %d", len(coils))
	}

	discrete, err := env.client.ReadDiscreteInputs(env.ctx, 100, bitQuantity)
	check(fmt.Sprintf("read %d discrete inputs", bitQuantity), err)
	if len(discrete) != int(bitQuantity) {
		log.Fatalf("discrete boundary length mismatch: %d", len(discrete))
	}

	values, err := env.client.ReadHoldingRegisters(env.ctx, 100, registerQuantity)
	check(fmt.Sprintf("read %d holding registers", registerQuantity), err)
	if len(values) != int(registerQuantity) {
		log.Fatalf("holding boundary length mismatch: %d", len(values))
	}

	inputs, err := env.client.ReadInputRegisters(env.ctx, 100, registerQuantity)
	check(fmt.Sprintf("read %d input registers", registerQuantity), err)
	if len(inputs) != int(registerQuantity) {
		log.Fatalf("input boundary length mismatch: %d", len(inputs))
	}

	check(fmt.Sprintf("write %d coils", writeBitQuantity), env.client.WriteMultipleCoils(env.ctx, 3000, alternatingBools(int(writeBitQuantity))))
	coils, err = env.client.ReadCoils(env.ctx, 3000, writeBitQuantity)
	check(fmt.Sprintf("read %d written coils", writeBitQuantity), err)
	if len(coils) != int(writeBitQuantity) {
		log.Fatalf("written coil boundary length mismatch: %d", len(coils))
	}

	check(fmt.Sprintf("write %d registers", writeRegisterQuantity), env.client.WriteMultipleRegisters(env.ctx, 3000, sequenceRegisters(int(writeRegisterQuantity))))
	values, err = env.client.ReadHoldingRegisters(env.ctx, 3000, writeRegisterQuantity)
	check(fmt.Sprintf("read %d written registers", writeRegisterQuantity), err)
	if len(values) != int(writeRegisterQuantity) {
		log.Fatalf("written register boundary length mismatch: %d", len(values))
	}

	if _, err := env.client.ReadHoldingRegisters(env.ctx, 0, 126); !isInvalidRequestOrException(err) {
		log.Fatalf("expected invalid quantity error for 126 registers, got %v", err)
	}
	fmt.Println("invalid quantity 126 registers: ok")
}

func runStress(env testEnv, count int, delay time.Duration) {
	if count <= 0 {
		log.Fatalf("stress-count must be positive, got %d", count)
	}
	var total time.Duration
	var max time.Duration
	for i := 0; i < count; i++ {
		start := time.Now()
		value := uint16(i)
		check("stress write register", env.client.WriteSingleRegister(env.ctx, 50, value))
		values, err := env.client.ReadHoldingRegisters(env.ctx, 50, 1)
		check("stress read register", err)
		if values[0] != value {
			log.Fatalf("stress value mismatch at iteration %d: got %d want %d", i, values[0], value)
		}
		elapsed := time.Since(start)
		total += elapsed
		if elapsed > max {
			max = elapsed
		}
		if delay > 0 {
			time.Sleep(delay)
		}
	}
	fmt.Printf("stress iterations=%d avg=%s max=%s\n", count, total/time.Duration(count), max)
}

func isInvalidRequestOrException(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, modbus.ErrInvalidRequest) {
		return true
	}
	var exception *modbus.ExceptionError
	return errors.As(err, &exception)
}

func sequenceRegisters(n int) []uint16 {
	out := make([]uint16, n)
	for i := range out {
		out[i] = uint16(i + 1)
	}
	return out
}

func alternatingBools(n int) []bool {
	out := make([]bool, n)
	for i := range out {
		out[i] = i%2 == 0
	}
	return out
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func check(name string, err error) {
	if err != nil {
		log.Fatalf("%s failed: %v", name, err)
	}
	fmt.Printf("%s: ok\n", name)
}

func expect[T comparable](name string, got, want []T) {
	if !reflect.DeepEqual(got, want) {
		log.Fatalf("%s mismatch: got %#v want %#v", name, got, want)
	}
	fmt.Printf("%s: %#v\n", name, got)
}
