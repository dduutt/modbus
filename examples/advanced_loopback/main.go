package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/dduutt/modbus"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := modbus.NewMemoryDataStore()
	status := byte(0x5A)
	handler := modbus.NewDataStoreHandler(store)
	handler.ExceptionStatus = &status
	handler.EnableDiagnostics = true
	handler.CommEventCounter = &modbus.CommEventCounter{Status: 0xFFFF, EventCount: 12}
	handler.CommEventLog = &modbus.CommEventLog{
		Status:       0x0000,
		EventCount:   12,
		MessageCount: 5,
		Events:       []byte{0xAA, 0xBB},
	}
	handler.ServerID = []byte{0x01, 0xFF, 'G', 'o'}
	handler.FileRecords = map[uint16][]uint16{
		7: []uint16{10, 20, 30, 40},
	}
	handler.FIFOQueues = map[uint16][]uint16{
		0x04DE: []uint16{100, 200, 300},
	}
	handler.DeviceIdentification = map[byte][]byte{
		0x00: []byte("Vendor"),
		0x01: []byte("Product"),
		0x02: []byte("1.0.0"),
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		server := modbus.NewTCPServer(handler)
		if err := server.Serve(ctx, ln); err != nil {
			log.Println(err)
		}
	}()

	client := modbus.NewClient(
		modbus.NewTCPTransport(ln.Addr().String()),
		modbus.WithUnitID(1),
		modbus.WithTimeout(3*time.Second),
	)
	defer client.Close()

	diagnostic, err := client.Diagnostic(ctx, 0x0000, 0xCAFE)
	if err != nil {
		log.Fatal(err)
	}
	exceptionStatus, err := client.ReadExceptionStatus(ctx)
	if err != nil {
		log.Fatal(err)
	}
	counter, err := client.GetCommEventCounter(ctx)
	if err != nil {
		log.Fatal(err)
	}
	eventLog, err := client.GetCommEventLog(ctx)
	if err != nil {
		log.Fatal(err)
	}
	serverID, err := client.ReportServerID(ctx)
	if err != nil {
		log.Fatal(err)
	}

	if err := client.WriteFileRecords(ctx, []modbus.FileRecord{
		{FileNumber: 7, RecordNumber: 1, Values: []uint16{21, 31}},
	}); err != nil {
		log.Fatal(err)
	}
	records, err := client.ReadFileRecords(ctx, []modbus.FileRecordRequest{
		{FileNumber: 7, RecordNumber: 0, RecordLength: 4},
	})
	if err != nil {
		log.Fatal(err)
	}
	fifoValues, err := client.ReadFIFOQueue(ctx, 0x04DE)
	if err != nil {
		log.Fatal(err)
	}
	deviceInfo, err := client.ReadDeviceIdentification(ctx, modbus.ReadDeviceIDCodeBasic)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("diagnostic: 0x%04X\n", diagnostic)
	fmt.Printf("exception status: 0x%02X\n", exceptionStatus)
	fmt.Printf("comm counter: status=0x%04X events=%d\n", counter.Status, counter.EventCount)
	fmt.Printf("comm log: messages=%d events=% X\n", eventLog.MessageCount, eventLog.Events)
	fmt.Printf("server id: % X\n", serverID)
	fmt.Printf("file records: %#v\n", records[0].Values)
	fmt.Printf("fifo: %#v\n", fifoValues)
	fmt.Printf("device: vendor=%s product=%s revision=%s\n",
		deviceInfo.Objects[0x00],
		deviceInfo.Objects[0x01],
		deviceInfo.Objects[0x02],
	)
}
