package modbus

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"testing"
	"time"

	smodbus "github.com/simonvetter/modbus"
)

func BenchmarkComparisonTCP(b *testing.B) {
	b.Run("dduutt/read_holding_registers", benchmarkDduuttReadHoldingRegisters)
	b.Run("simonvetter/read_holding_registers", benchmarkSimonReadHoldingRegisters)
	b.Run("dduutt/write_holding_registers", benchmarkDduuttWriteHoldingRegisters)
	b.Run("simonvetter/write_holding_registers", benchmarkSimonWriteHoldingRegisters)
	b.Run("dduutt/read_coils", benchmarkDduuttReadCoils)
	b.Run("simonvetter/read_coils", benchmarkSimonReadCoils)
	b.Run("dduutt/write_coils", benchmarkDduuttWriteCoils)
	b.Run("simonvetter/write_coils", benchmarkSimonWriteCoils)
	b.Run("dduutt/mixed_read_write", benchmarkDduuttMixedReadWrite)
	b.Run("simonvetter/mixed_read_write", benchmarkSimonMixedReadWrite)
}

func benchmarkDduuttReadHoldingRegisters(b *testing.B) {
	client, cancel := newDduuttBenchmarkClient(b)
	defer cancel()
	benchmarkLoop(b, func(i int) error {
		_, err := client.ReadHoldingRegisters(context.Background(), uint16(i%64), 8)
		return err
	})
}

func benchmarkDduuttWriteHoldingRegisters(b *testing.B) {
	client, cancel := newDduuttBenchmarkClient(b)
	defer cancel()
	benchmarkLoop(b, func(i int) error {
		return client.WriteMultipleRegisters(context.Background(), uint16(i%64), []uint16{1, 2, 3, 4, 5, 6, 7, 8})
	})
}

func benchmarkDduuttReadCoils(b *testing.B) {
	client, cancel := newDduuttBenchmarkClient(b)
	defer cancel()
	benchmarkLoop(b, func(i int) error {
		_, err := client.ReadCoils(context.Background(), uint16(i%64), 16)
		return err
	})
}

func benchmarkDduuttWriteCoils(b *testing.B) {
	client, cancel := newDduuttBenchmarkClient(b)
	defer cancel()
	values := []bool{true, false, true, true, false, false, true, false, true, false, true, false, true, true, false, true}
	benchmarkLoop(b, func(i int) error {
		return client.WriteMultipleCoils(context.Background(), uint16(i%64), values)
	})
}

func benchmarkDduuttMixedReadWrite(b *testing.B) {
	client, cancel := newDduuttBenchmarkClient(b)
	defer cancel()
	coils := []bool{true, false, true, true, false, false, true, false}
	benchmarkLoop(b, func(i int) error {
		addr := uint16(i % 64)
		switch i % 4 {
		case 0:
			_, err := client.ReadHoldingRegisters(context.Background(), addr, 8)
			return err
		case 1:
			return client.WriteMultipleRegisters(context.Background(), addr, []uint16{1, 2, 3, 4})
		case 2:
			_, err := client.ReadCoils(context.Background(), addr, 8)
			return err
		default:
			return client.WriteMultipleCoils(context.Background(), addr, coils)
		}
	})
}

func benchmarkSimonReadHoldingRegisters(b *testing.B) {
	client, stop := newSimonBenchmarkClient(b)
	defer stop()
	benchmarkLoop(b, func(i int) error {
		_, err := client.ReadRegisters(uint16(i%64), 8, smodbus.HOLDING_REGISTER)
		return err
	})
}

func benchmarkSimonWriteHoldingRegisters(b *testing.B) {
	client, stop := newSimonBenchmarkClient(b)
	defer stop()
	benchmarkLoop(b, func(i int) error {
		return client.WriteRegisters(uint16(i%64), []uint16{1, 2, 3, 4, 5, 6, 7, 8})
	})
}

func benchmarkSimonReadCoils(b *testing.B) {
	client, stop := newSimonBenchmarkClient(b)
	defer stop()
	benchmarkLoop(b, func(i int) error {
		_, err := client.ReadCoils(uint16(i%64), 16)
		return err
	})
}

func benchmarkSimonWriteCoils(b *testing.B) {
	client, stop := newSimonBenchmarkClient(b)
	defer stop()
	values := []bool{true, false, true, true, false, false, true, false, true, false, true, false, true, true, false, true}
	benchmarkLoop(b, func(i int) error {
		return client.WriteCoils(uint16(i%64), values)
	})
}

func benchmarkSimonMixedReadWrite(b *testing.B) {
	client, stop := newSimonBenchmarkClient(b)
	defer stop()
	coils := []bool{true, false, true, true, false, false, true, false}
	benchmarkLoop(b, func(i int) error {
		addr := uint16(i % 64)
		switch i % 4 {
		case 0:
			_, err := client.ReadRegisters(addr, 8, smodbus.HOLDING_REGISTER)
			return err
		case 1:
			return client.WriteRegisters(addr, []uint16{1, 2, 3, 4})
		case 2:
			_, err := client.ReadCoils(addr, 8)
			return err
		default:
			return client.WriteCoils(addr, coils)
		}
	})
}

func benchmarkLoop(b *testing.B, op func(int) error) {
	b.Helper()
	if err := op(0); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := op(i); err != nil {
			b.Fatal(err)
		}
	}
}

func newDduuttBenchmarkClient(b *testing.B) (*Client, context.CancelFunc) {
	b.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	store := NewMemoryDataStoreSized(256, 256, 256, 256)
	registers := make([]uint16, 256)
	coils := make([]bool, 256)
	for i := range registers {
		registers[i] = uint16(i)
		coils[i] = i%2 == 0
	}
	if err := store.WriteHoldingRegisters(0, registers); err != nil {
		b.Fatal(err)
	}
	if err := store.WriteCoils(0, coils); err != nil {
		b.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatal(err)
	}
	go func() { _ = NewTCPServer(NewDataStoreHandler(store)).Serve(ctx, ln) }()
	client := NewClient(NewTCPTransport(ln.Addr().String()), WithUnitID(1), WithTimeout(3*time.Second))
	return client, func() {
		_ = client.Close()
		cancel()
		_ = ln.Close()
	}
}

func newSimonBenchmarkClient(b *testing.B) (*smodbus.ModbusClient, func()) {
	b.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatal(err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		b.Fatal(err)
	}

	silent := log.New(io.Discard, "", 0)
	server, err := smodbus.NewServer(&smodbus.ServerConfiguration{
		URL:        fmt.Sprintf("tcp://%s", addr),
		Timeout:    3 * time.Second,
		MaxClients: 1,
		Logger:     silent,
	}, newSimonBenchmarkHandler(256))
	if err != nil {
		b.Fatal(err)
	}
	if err := server.Start(); err != nil {
		b.Fatal(err)
	}

	client, err := smodbus.NewClient(&smodbus.ClientConfiguration{
		URL:     fmt.Sprintf("tcp://%s", addr),
		Timeout: 3 * time.Second,
		Logger:  silent,
	})
	if err != nil {
		_ = server.Stop()
		b.Fatal(err)
	}
	if err := client.Open(); err != nil {
		_ = server.Stop()
		b.Fatal(err)
	}
	if err := client.SetUnitId(1); err != nil {
		_ = client.Close()
		_ = server.Stop()
		b.Fatal(err)
	}
	return client, func() {
		_ = client.Close()
		_ = server.Stop()
	}
}

type simonBenchmarkHandler struct {
	coils   []bool
	holding []uint16
	input   []uint16
}

func newSimonBenchmarkHandler(size int) *simonBenchmarkHandler {
	h := &simonBenchmarkHandler{
		coils:   make([]bool, size),
		holding: make([]uint16, size),
		input:   make([]uint16, size),
	}
	for i := range h.holding {
		h.coils[i] = i%2 == 0
		h.holding[i] = uint16(i)
		h.input[i] = uint16(i)
	}
	return h
}

func (h *simonBenchmarkHandler) HandleCoils(req *smodbus.CoilsRequest) ([]bool, error) {
	if req.IsWrite {
		copy(h.coils[req.Addr:], req.Args)
		return nil, nil
	}
	out := make([]bool, req.Quantity)
	copy(out, h.coils[req.Addr:req.Addr+req.Quantity])
	return out, nil
}

func (h *simonBenchmarkHandler) HandleDiscreteInputs(req *smodbus.DiscreteInputsRequest) ([]bool, error) {
	out := make([]bool, req.Quantity)
	copy(out, h.coils[req.Addr:req.Addr+req.Quantity])
	return out, nil
}

func (h *simonBenchmarkHandler) HandleHoldingRegisters(req *smodbus.HoldingRegistersRequest) ([]uint16, error) {
	if req.IsWrite {
		copy(h.holding[req.Addr:], req.Args)
		return nil, nil
	}
	out := make([]uint16, req.Quantity)
	copy(out, h.holding[req.Addr:req.Addr+req.Quantity])
	return out, nil
}

func (h *simonBenchmarkHandler) HandleInputRegisters(req *smodbus.InputRegistersRequest) ([]uint16, error) {
	out := make([]uint16, req.Quantity)
	copy(out, h.input[req.Addr:req.Addr+req.Quantity])
	return out, nil
}
