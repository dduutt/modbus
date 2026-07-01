package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/dduutt/modbus"
)

func main() {
	ctx := context.Background()
	client := modbus.NewClient(
		modbus.NewTCPTransport("127.0.0.1:1502"),
		modbus.WithUnitID(1),
		modbus.WithTimeout(3*time.Second),
	)
	defer client.Close()

	if err := client.WriteTags(ctx, map[string]modbus.TagValue{
		"setpoint": {
			Tag:   modbus.HoldingRegister(0).As(modbus.TypeFloat32),
			Value: float32(23.5),
		},
		"enabled": {
			Tag:   modbus.Coil(0),
			Value: true,
		},
	}); err != nil {
		log.Fatal(err)
	}

	values, err := client.ReadTags(ctx, map[string]modbus.Tag{
		"setpoint": modbus.HoldingRegister(0).As(modbus.TypeFloat32),
		"enabled":  modbus.Coil(0),
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("values: %#v\n", values)
}
