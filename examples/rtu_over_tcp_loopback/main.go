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
	if err := store.WriteHoldingRegisters(0, []uint16{10, 20, 30}); err != nil {
		log.Fatal(err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				server := modbus.NewRTUServer(modbus.NewDataStoreHandler(store))
				if err := server.Serve(ctx, conn); err != nil {
					log.Println(err)
				}
			}()
		}
	}()
	defer ln.Close()

	client := modbus.NewClient(
		modbus.NewRTUOverTCPTransport(ln.Addr().String()),
		modbus.WithUnitID(1),
		modbus.WithTimeout(3*time.Second),
	)
	defer client.Close()

	values, err := client.ReadHoldingRegisters(ctx, 0, 3)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("holding registers: %#v\n", values)
}
