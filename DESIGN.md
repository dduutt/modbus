# Design Notes

See `STATUS.md` for the current implementation coverage and verification
summary.

## Dependency Policy

This library keeps the Modbus protocol core self-contained:

- PDU construction and validation.
- TCP MBAP codec.
- RTU CRC and frame codec.
- Function-code client APIs.
- Tag/value conversion.
- Slave simulation and memory data store.

These parts are small, protocol-specific, and benefit from explicit control and
test coverage. Pulling in a large Modbus implementation as a dependency would
make the public API harder to control.

For operating-system integrations, prefer mature third-party packages behind a
small adapter:

- Serial port opening/configuration.
- Platform-specific port enumeration.
- Optional logging integrations.

The core RTU transport therefore accepts an `io.ReadWriteCloser`. A separate
adapter package can open a serial port and pass it into `NewRTUTransport`.

## References

The local reference sources are under `.refs/`.

Observed dependency choices:

- `goburrow/modbus`: implements Modbus protocol logic itself; uses
  `github.com/goburrow/serial` for serial ports.
- `simonvetter/modbus`: implements Modbus protocol logic itself; uses
  `github.com/goburrow/serial` for serial ports.
- `thinkgos/gomodbus`: implements Modbus protocol logic itself; uses
  `github.com/goburrow/serial` for serial ports.
- `grid-x/modbus`: implements Modbus protocol logic itself; uses
  `github.com/grid-x/serial` for serial ports.
- `apache/plc4x/plc4go`: generated and framework-based protocol stack; uses
  `github.com/jacobsa/go-serial` for serial transport.

## Project Decision

Use self-owned code for Modbus protocol behavior and keep serial support
pluggable.

Implemented adapter:

```text
serial/
  serial.go
```

The adapter depends on a maintained serial package, exposes a compact
configuration struct, and returns `io.ReadWriteCloser`.

Candidate packages:

- `go.bug.st/serial`: actively maintained, broad platform support.
- `github.com/goburrow/serial`: proven in Modbus libraries, smaller API.

Prefer `go.bug.st/serial` for new code unless compatibility with the goburrow
ecosystem becomes a specific requirement.

- `serial.Open(Config)` returns `io.ReadWriteCloser`.
- `serial.GetPortsList()` exposes port enumeration.
- The root `modbus` package remains independent from serial dependencies.

## Transport And Recovery Decisions

The transport API is intentionally small:

- `Transport.Do(ctx, unitID, request)` sends one request and returns one PDU.
- TCP uses MBAP framing and validates transaction id and unit id.
- RTU uses RTU ADU framing, CRC validation, and optional bus timing.
- RTU-over-TCP uses RTU ADU framing over a TCP stream for serial gateways.

The library does not automatically retry the current request after a connection
error. This is deliberate: write requests may have been processed by the device
even if the response was lost, so blind retry can duplicate side effects.

Project decision:

- TCP and RTU-over-TCP close a bad connection after read/write errors.
- The failed request returns an error to the caller.
- The next request dials a fresh connection if the client was not explicitly
  closed.
- Serial RTU does not reopen ports in the root package because the root package
  only owns an `io.ReadWriteCloser`; reopening belongs in `serial` or the
  caller's application layer.

## Client Configuration

Reference libraries often expose a large client configuration object. This
library keeps the direct constructors as the primary API and adds a lightweight
config path for file/flag driven applications.

Project decision:

- `ClientConfig` supports `ModeTCP`, `ModeRTU`, and `ModeRTUOverTCP`.
- TCP and RTU-over-TCP configs use `Address`.
- RTU configs use a caller-supplied `Conn`, preserving the serial dependency
  boundary.
- `NewTransportFromConfig` and `NewClientFromConfig` are convenience entry
  points; they do not replace `NewClient`, `NewTCPTransport`, or
  `NewRTUTransport`.

## Tag And Value API

PLC4Go has a rich string tag parser, while lightweight Go Modbus libraries
usually expose explicit method calls. This library chooses explicit Go builders
as the primary API:

```go
HoldingRegister(0).As(TypeFloat32).WithQuantity(1)
```

Configuration strings are intentionally short and limited:

```text
hr:0:f32:1
```

Project decision:

- Use short area names only: `c`, `di`, `hr`, `ir`.
- Use short type names only: `b`, `u16`, `i16`, `u32`, `i32`, `f32`, `u64`,
  `i64`, `f64`, `by`, `str`.
- Keep parsed addresses zero-based to match the direct API.
- Keep typed value accessors on `Value`, such as `Float32()` and `UInt16s()`,
  so callers do not need repeated type assertions.

## RTU Timing References

Observed RTU behavior in the reference implementations:

- `goburrow/modbus` and `thinkgos/gomodbus` estimate the response length from
  the request, sleep for request+response transmit time plus the RTU frame gap,
  then read the expected frame length.
- `grid-x/modbus` keeps the same delay calculation, adds context-aware reads,
  incremental frame scanning, and reconnection logic.
- `simonvetter/modbus` explicitly models `t1` and `t3.5`, waits for bus idle
  before transmitting, estimates write completion time because serial writes may
  be buffered, and discards stale bytes after protocol/CRC errors.
- `plc4go` delegates low-level serial timing to its serial transport by setting
  an inter-character timeout.

Project decision:

- Keep RTU frame parsing deterministic by function code.
- Add optional timing configuration to `RTUTransport`.
- Default behavior stays compatible with the existing tests.
- When a baud rate is configured, enforce bus idle before writes and wait for
  estimated request transmission plus `t3.5` before reading.
- RTU response reading uses a lightweight scanner: it skips leading noise,
  matches unit id and expected function/exception function, validates response
  shape, and retries synchronization after CRC failures.

## Device Identification References

`grid-x/modbus` exposes Read Device Identification (`0x2B`, MEI type `0x0E`)
with separate APIs for category reads and specific-object reads. It follows the
`More Follows` / `Next Object ID` fields to merge paginated results.

PLC4X/PLC4Go models the same operation in generated request/response PDUs. Its
model confirms the wire order: MEI type, read device id code, conformity level,
more follows, next object id, object count, then repeated object id, length, and
value tuples. PLC4X splits the high conformity bit into an `individualAccess`
flag, but this library keeps the raw byte so values such as `0x81` remain
visible to callers.

Project decision:

- Implement protocol parsing and pagination in the core package.
- Keep API results structured so callers can inspect metadata and objects.
- Add optional simulator support through `DataStoreHandler.DeviceIdentification`
  without coupling it to register storage.
- Return illegal function when the simulator has no device identification data
  configured.

## FIFO Queue References

`grid-x/modbus` exposes Read FIFO Queue (`0x18`) as a client API and parses the
two-byte byte count plus two-byte FIFO count fields before returning the FIFO
payload. PLC4X/PLC4Go models the response as `[]uint16`, with `byteCount =
len(values)*2 + 2` and `fifoCount = len(values)`.

Project decision:

- Expose `ReadFIFOQueue(ctx, address)` as `[]uint16`, because FIFO values are
  Modbus registers rather than untyped bytes.
- Validate both response count fields and the protocol limit of 31 FIFO values.
- Add optional simulator support through `DataStoreHandler.FIFOQueues`, keyed by
  FIFO pointer address.
- Return illegal function when FIFO queues are not configured, and illegal data
  address when a requested pointer address is missing.

## File Record References

The lightweight Go reference libraries do not expose a complete file-record API.
PLC4X/PLC4Go has generated models for Read File Record (`0x14`) and Write File
Record (`0x15`), including request byte-count arrays and item-level record
metadata.

Project decision:

- Add `FileRecordRequest` and `FileRecord` structs rather than exposing raw item
  byte slices.
- Expose `ReadFileRecords(ctx, requests)` and `WriteFileRecords(ctx, records)`.
- Support standard reference type `0x06`; a zero reference type in user-facing
  structs defaults to `0x06`.
- Validate the Modbus file-record byte-count limit and reject empty record
  lengths.
- Add optional simulator support through `DataStoreHandler.FileRecords`, keyed
  by file number and storing 16-bit record values.
- Treat missing files or out-of-range record spans as illegal data address.

## Diagnostics References

PLC4X/PLC4Go models the serial-line diagnostics and communication event
functions:

- Read Exception Status (`0x07`): no request data; response has one status byte.
- Diagnostics (`0x08`): two 16-bit fields, `subFunction` and `data`.
- Get Comm Event Counter (`0x0B`): no request data; response has `status` and
  `eventCount`.
- Get Comm Event Log (`0x0C`): no request data; response has `byteCount`,
  `status`, `eventCount`, `messageCount`, and event bytes.
- Report Server ID (`0x11`): no request data; response has `byteCount` followed
  by opaque server-id bytes.

Project decision:

- Expose `ReadExceptionStatus(ctx)` as a single byte.
- Expose `Diagnostic(ctx, subFunction, data)` and return the response data after
  validating the echoed subfunction.
- Expose `GetCommEventCounter(ctx)` and `GetCommEventLog(ctx)` as structured
  results.
- Expose `ReportServerID(ctx)` as opaque bytes because the exact payload is
  vendor-defined.
- Add optional simulator support through `EnableDiagnostics`,
  `DiagnosticResponses`, `ExceptionStatus`, `CommEventCounter`, `CommEventLog`,
  and `ServerID`.
- Keep diagnostic simulation conservative: diagnostics must be explicitly
  enabled; otherwise the simulator returns illegal function.
- Allow 4-byte RTU frames in the codec because no-data requests such as `0x0B`
  and `0x0C` consist of unit id, function code, and CRC only.

## Function Code Coverage

Implemented common operations:

- `0x01` read coils.
- `0x02` read discrete inputs.
- `0x03` read holding registers.
- `0x04` read input registers.
- `0x05` write single coil.
- `0x06` write single register.
- `0x07` read exception status.
- `0x08` diagnostics.
- `0x0B` get communication event counter.
- `0x0C` get communication event log.
- `0x0F` write multiple coils.
- `0x10` write multiple holding registers.
- `0x11` report server id.
- `0x14` read file record.
- `0x15` write file record.
- `0x16` mask write holding register.
- `0x17` read/write multiple holding registers.
- `0x18` read FIFO queue.
- `0x2B/0x0E` read device identification.
