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
	if err := store.WriteCoils(0, []bool{true, false, true, false}); err != nil {
		log.Fatal(err)
	}

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

	server := modbus.NewTCPServer(handler)
	log.Println("advanced tcp slave listening on 127.0.0.1:1503")
	if err := server.ListenAndServe(context.Background(), "127.0.0.1:1503"); err != nil {
		log.Fatal(err)
	}
}
