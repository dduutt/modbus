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
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		server := modbus.NewTCPServer(modbus.NewDataStoreHandler(store))
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

	if err := client.WriteTags(ctx, map[string]modbus.TagValue{
		"temperature": {
			Tag:   modbus.HoldingRegister(0).As(modbus.TypeFloat32),
			Value: float32(26.75),
		},
		"running": {
			Tag:   modbus.Coil(0),
			Value: true,
		},
	}); err != nil {
		log.Fatal(err)
	}

	values, err := client.ReadTags(ctx, map[string]modbus.Tag{
		"temperature": modbus.HoldingRegister(0).As(modbus.TypeFloat32),
		"running":     modbus.Coil(0),
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("values: %#v\n", values)
}
