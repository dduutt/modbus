package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/dduutt/modbus"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := modbus.NewClient(
		modbus.NewTCPTransport("127.0.0.1:1502"),
		modbus.WithUnitID(1),
		modbus.WithTimeout(3*time.Second),
	)
	defer client.Close()

	registers, err := client.ReadHoldingRegisters(ctx, 0, 3)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("holding registers: %#v\n", registers)

	if err := client.WriteSingleRegister(ctx, 1, 222); err != nil {
		log.Fatal(err)
	}
	registers, err = client.ReadHoldingRegisters(ctx, 0, 3)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("holding registers after write: %#v\n", registers)
}
