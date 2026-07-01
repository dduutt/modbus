# Implementation Status

## Supported Transports

- Modbus TCP client transport with MBAP framing and transaction validation.
- Modbus RTU client transport over caller-supplied `io.ReadWriteCloser`.
- RTU-over-TCP client transport using RTU ADU and CRC framing on a TCP stream.
- TCP slave simulation through `TCPServer`.
- RTU slave simulation through `RTUServer`.

## Client APIs

- `ReadCoils`
- `ReadDiscreteInputs`
- `ReadHoldingRegisters`
- `ReadInputRegisters`
- `WriteSingleCoil`
- `WriteSingleRegister`
- `WriteMultipleCoils`
- `WriteMultipleRegisters`
- `ReadWriteMultipleRegisters`
- `MaskWriteRegister`
- `ReadExceptionStatus`
- `Diagnostic`
- `GetCommEventCounter`
- `GetCommEventLog`
- `ReportServerID`
- `ReadFileRecords`
- `WriteFileRecords`
- `ReadFIFOQueue`
- `ReadDeviceIdentification`
- `ReadDeviceIdentificationObject`
- `ReadTag`
- `ReadTags`
- `WriteTag`
- `WriteTags`

## Function Code Coverage

- `0x01` Read Coils
- `0x02` Read Discrete Inputs
- `0x03` Read Holding Registers
- `0x04` Read Input Registers
- `0x05` Write Single Coil
- `0x06` Write Single Register
- `0x07` Read Exception Status
- `0x08` Diagnostics
- `0x0B` Get Comm Event Counter
- `0x0C` Get Comm Event Log
- `0x0F` Write Multiple Coils
- `0x10` Write Multiple Registers
- `0x11` Report Server ID
- `0x14` Read File Record
- `0x15` Write File Record
- `0x16` Mask Write Register
- `0x17` Read/Write Multiple Registers
- `0x18` Read FIFO Queue
- `0x2B/0x0E` Read Device Identification

## Slave Simulation

`DataStoreHandler` supports:

- Coils
- Discrete inputs
- Holding registers
- Input registers
- File records
- FIFO queues
- Exception status
- Diagnostics
- Communication event counter
- Communication event log
- Server ID
- Device identification

## Tag And Value API

- Builder tags: `Coil`, `DiscreteInput`, `HoldingRegister`, `InputRegister`.
- Short configuration tags: `c`, `di`, `hr`, `ir`.
- Short data types: `b`, `u16`, `i16`, `u32`, `i32`, `f32`, `u64`, `i64`,
  `f64`, `by`, `str`.
- Typed value accessors for scalar and slice values.
- Batch read/write range merging for tag maps.

## Serial Strategy

- The root package does not open operating-system serial ports.
- RTU transports accept `io.ReadWriteCloser`.
- `modbus/serial` uses `go.bug.st/serial` to open and list serial ports.

## Connection Strategy

- The library does not retry the current request after connection errors.
- TCP and RTU-over-TCP close bad connections and let the next request dial
  again.
- Serial RTU reconnect/reopen behavior belongs in the caller or adapter layer.

## Not Implemented

- Modbus ASCII.
- UDP transports.
- TLS/MBAPS.
- Automatic retry of the current request.
- Root-package serial port opening.
- Vendor-specific function codes outside the list above.

## Verification

Current verification set:

```text
go test -count=1 ./...
go vet ./...
go test -race -count=1 ./...
go run ./examples/advanced_loopback
go run ./examples/rtu_over_tcp_loopback
go run ./examples/tags_loopback
```

Hardware verification on the current TCP -> LoRa gateway -> LoRa node ->
serial -> PC loopback:

- `advanced` passed with `-Timeout 5s`.
- `boundary` passed with `-BoundaryBits 500 -BoundaryRegisters 80
  -BoundaryWriteBits 500 -BoundaryWriteRegisters 80 -Timeout 10s`.
- `stress` passed with `-StressCount 100 -StressDelay 50ms -Timeout 5s`.
- Full protocol-limit boundary frames timed out on this LoRa path, so those
  failures are treated as link-capacity/latency limits rather than protocol
  implementation failures.

## Optional Future Work

- Modbus ASCII transport if a target device requires it.
- TLS/MBAPS for deployments that need transport security.
- UDP or RTU-over-UDP only if there is a concrete gateway requirement.
- Adapter-level serial reopen policy.
- More executable examples for every advanced function-code API.
