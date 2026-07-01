package modbus_test

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/dduutt/modbus"
)

func ExampleClient_ReadHoldingRegisters() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := modbus.NewMemoryDataStore()
	_ = store.WriteHoldingRegisters(0, []uint16{10, 20})
	address := startTCPExampleServer(ctx, modbus.NewDataStoreHandler(store))

	client := modbus.NewClient(
		modbus.NewTCPTransport(address),
		modbus.WithUnitID(1),
		modbus.WithTimeout(time.Second),
	)
	defer client.Close()

	values, err := client.ReadHoldingRegisters(ctx, 0, 2)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(values)
	// Output: [10 20]
}

func ExampleNewClientFromConfig() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := modbus.NewMemoryDataStore()
	_ = store.WriteHoldingRegisters(0, []uint16{42})
	address := startTCPExampleServer(ctx, modbus.NewDataStoreHandler(store))

	client, err := modbus.NewClientFromConfig(modbus.ClientConfig{
		Mode:    modbus.ModeTCP,
		Address: address,
		UnitID:  1,
		Timeout: time.Second,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	values, err := client.ReadHoldingRegisters(ctx, 0, 1)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(values[0])
	// Output: 42
}

func ExampleNewRTUOverTCPTransport() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := modbus.NewMemoryDataStore()
	_ = store.WriteHoldingRegisters(0, []uint16{7})
	address := startRTUOverTCPExampleServer(ctx, modbus.NewDataStoreHandler(store))

	client := modbus.NewClient(
		modbus.NewRTUOverTCPTransport(address),
		modbus.WithUnitID(1),
		modbus.WithTimeout(time.Second),
	)
	defer client.Close()

	values, err := client.ReadHoldingRegisters(ctx, 0, 1)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(values[0])
	// Output: 7
}

func ExampleParseTag() {
	tag, err := modbus.ParseTag("hr:0:f32:1")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s %d %s %d\n", tag.Area, tag.Address, tag.DataType, tag.Quantity)
	// Output: holding-register 0 float32 1
}

func ExampleClient_ReadTag() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	raw, _ := modbus.EncodeValue(modbus.HoldingRegister(0).As(modbus.TypeFloat32), float32(26.75))
	store := modbus.NewMemoryDataStore()
	_ = store.WriteHoldingRegisters(0, raw.Registers)
	address := startTCPExampleServer(ctx, modbus.NewDataStoreHandler(store))

	client := modbus.NewClient(
		modbus.NewTCPTransport(address),
		modbus.WithUnitID(1),
		modbus.WithTimeout(time.Second),
	)
	defer client.Close()

	value, err := client.ReadTag(ctx, modbus.HoldingRegister(0).As(modbus.TypeFloat32))
	if err != nil {
		log.Fatal(err)
	}
	temperature, ok := value.Float32()
	if !ok {
		log.Fatal("unexpected value type")
	}
	fmt.Printf("%.2f\n", temperature)
	// Output: 26.75
}

func ExampleClient_ReadDeviceIdentification() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := modbus.NewDataStoreHandler(modbus.NewMemoryDataStore())
	handler.DeviceIdentification = map[byte][]byte{
		0x00: []byte("Vendor"),
		0x01: []byte("Product"),
		0x02: []byte("1.0.0"),
	}
	address := startTCPExampleServer(ctx, handler)

	client := modbus.NewClient(
		modbus.NewTCPTransport(address),
		modbus.WithUnitID(1),
		modbus.WithTimeout(time.Second),
	)
	defer client.Close()

	info, err := client.ReadDeviceIdentification(ctx, modbus.ReadDeviceIDCodeBasic)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(info.Objects[0x00]))
	// Output: Vendor
}

func startTCPExampleServer(ctx context.Context, handler modbus.Handler) string {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		_ = modbus.NewTCPServer(handler).Serve(ctx, ln)
	}()
	return ln.Addr().String()
}

func startRTUOverTCPExampleServer(ctx context.Context, handler modbus.Handler) string {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				_ = modbus.NewRTUServer(handler).Serve(ctx, conn)
			}()
		}
	}()
	return ln.Addr().String()
}
