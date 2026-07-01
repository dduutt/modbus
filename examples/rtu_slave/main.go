package main

import (
	"context"
	"log"
	"time"

	"github.com/dduutt/modbus"
	modbusserial "github.com/dduutt/modbus/serial"
)

func main() {
	port, err := modbusserial.Open(modbusserial.Config{
		PortName: "COM4",
		BaudRate: 9600,
		DataBits: 8,
		StopBits: modbusserial.OneStopBit,
		Parity:   modbusserial.NoParity,
		Timeout:  time.Second,
	})
	if err != nil {
		log.Fatal(err)
	}

	store := modbus.NewMemoryDataStore()
	if err := store.WriteHoldingRegisters(0, []uint16{10, 20, 30}); err != nil {
		log.Fatal(err)
	}

	log.Println("serving RTU slave on COM4")
	server := modbus.NewRTUServer(modbus.NewDataStoreHandler(store))
	if err := server.Serve(context.Background(), port); err != nil {
		log.Fatal(err)
	}
}
