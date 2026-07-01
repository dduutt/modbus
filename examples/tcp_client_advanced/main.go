package main

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"time"

	"github.com/dduutt/modbus"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := modbus.NewClient(
		modbus.NewTCPTransport("127.0.0.1:1503"),
		modbus.WithUnitID(1),
		modbus.WithTimeout(3*time.Second),
	)
	defer client.Close()

	registers, err := client.ReadHoldingRegisters(ctx, 0, 3)
	check("read holding registers", err)
	expect("holding registers", registers, []uint16{100, 200, 300})

	check("write multiple registers", client.WriteMultipleRegisters(ctx, 10, []uint16{11, 22, 33}))
	registers, err = client.ReadHoldingRegisters(ctx, 10, 3)
	check("read multiple registers", err)
	expect("multiple registers", registers, []uint16{11, 22, 33})

	check("mask write register", client.MaskWriteRegister(ctx, 10, 0x0FF0, 0x0005))
	registers, err = client.ReadHoldingRegisters(ctx, 10, 1)
	check("read masked register", err)
	expect("masked register", registers, []uint16{0x0005})

	registers, err = client.ReadWriteMultipleRegisters(ctx, 10, 2, 20, []uint16{77, 88})
	check("read/write multiple registers", err)
	expect("read/write result", registers, []uint16{0x0005, 22})

	coils, err := client.ReadCoils(ctx, 0, 4)
	check("read coils", err)
	expect("coils", coils, []bool{true, false, true, false})
	check("write multiple coils", client.WriteMultipleCoils(ctx, 10, []bool{true, true, false, true}))
	coils, err = client.ReadCoils(ctx, 10, 4)
	check("read multiple coils", err)
	expect("multiple coils", coils, []bool{true, true, false, true})

	diagnostic, err := client.Diagnostic(ctx, 0x0000, 0xCAFE)
	check("diagnostic", err)
	if diagnostic != 0xCAFE {
		log.Fatalf("diagnostic mismatch: got 0x%04X", diagnostic)
	}

	exceptionStatus, err := client.ReadExceptionStatus(ctx)
	check("read exception status", err)
	if exceptionStatus != 0x5A {
		log.Fatalf("exception status mismatch: got 0x%02X", exceptionStatus)
	}

	counter, err := client.GetCommEventCounter(ctx)
	check("comm event counter", err)
	if counter.Status != 0xFFFF || counter.EventCount != 12 {
		log.Fatalf("comm event counter mismatch: %#v", counter)
	}

	eventLog, err := client.GetCommEventLog(ctx)
	check("comm event log", err)
	if eventLog.MessageCount != 5 || !reflect.DeepEqual(eventLog.Events, []byte{0xAA, 0xBB}) {
		log.Fatalf("comm event log mismatch: %#v", eventLog)
	}

	serverID, err := client.ReportServerID(ctx)
	check("report server id", err)
	expect("server id", serverID, []byte{0x01, 0xFF, 'G', 'o'})

	check("write file records", client.WriteFileRecords(ctx, []modbus.FileRecord{
		{FileNumber: 7, RecordNumber: 1, Values: []uint16{21, 31}},
	}))
	records, err := client.ReadFileRecords(ctx, []modbus.FileRecordRequest{
		{FileNumber: 7, RecordNumber: 0, RecordLength: 4},
	})
	check("read file records", err)
	expect("file records", records[0].Values, []uint16{10, 21, 31, 40})

	fifoValues, err := client.ReadFIFOQueue(ctx, 0x04DE)
	check("read fifo queue", err)
	expect("fifo", fifoValues, []uint16{100, 200, 300})

	deviceInfo, err := client.ReadDeviceIdentification(ctx, modbus.ReadDeviceIDCodeBasic)
	check("read device identification", err)
	if string(deviceInfo.Objects[0x00]) != "Vendor" ||
		string(deviceInfo.Objects[0x01]) != "Product" ||
		string(deviceInfo.Objects[0x02]) != "1.0.0" {
		log.Fatalf("device identification mismatch: %#v", deviceInfo.Objects)
	}
	fmt.Printf("device: vendor=%s product=%s revision=%s\n",
		deviceInfo.Objects[0x00],
		deviceInfo.Objects[0x01],
		deviceInfo.Objects[0x02],
	)

	fmt.Println("advanced tcp client test passed")
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
