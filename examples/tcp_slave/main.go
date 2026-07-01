package main

import (
	"context"
	"log"

	"github.com/dduutt/modbus"
)

func main() {
	store := modbus.NewMemoryDataStore()
	if err := store.WriteHoldingRegisters(0, []uint16{100, 200, 300}); err != nil {
		log.Fatal(err)
	}
	if err := store.WriteCoils(0, []bool{true, false, true}); err != nil {
		log.Fatal(err)
	}

	server := modbus.NewTCPServer(modbus.NewDataStoreHandler(store))
	log.Println("listening on 127.0.0.1:1502")
	if err := server.ListenAndServe(context.Background(), "127.0.0.1:1502"); err != nil {
		log.Fatal(err)
	}
}
