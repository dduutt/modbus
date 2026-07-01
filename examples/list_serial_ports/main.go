package main

import (
	"fmt"
	"log"

	modbusserial "github.com/dduutt/modbus/serial"
)

func main() {
	ports, err := modbusserial.GetPortsList()
	if err != nil {
		log.Fatal(err)
	}
	for _, port := range ports {
		fmt.Println(port)
	}
}
