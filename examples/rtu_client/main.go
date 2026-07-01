package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/dduutt/modbus"
	modbusserial "github.com/dduutt/modbus/serial"
)

func main() {
	port, err := modbusserial.Open(modbusserial.Config{
		PortName: "COM3",
		BaudRate: 9600,
		DataBits: 8,
		StopBits: modbusserial.OneStopBit,
		Parity:   modbusserial.NoParity,
		Timeout:  time.Second,
	})
	if err != nil {
		log.Fatal(err)
	}

	client := modbus.NewClient(
		modbus.NewRTUTransport(port),
		modbus.WithUnitID(1),
		modbus.WithTimeout(3*time.Second),
	)
	defer client.Close()

	values, err := client.ReadHoldingRegisters(context.Background(), 0, 2)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("holding registers: %#v\n", values)
}
