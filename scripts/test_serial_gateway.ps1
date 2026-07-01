param(
    [string]$Serial = "COM4",
    [string]$Gateway = "10.83.2.40:1234",
    [int]$Unit = 1,
    [int]$Baud = 9600,
    [string]$Timeout = "3s",
    [string]$Suite = "basic",
    [int]$BoundaryBits = 2000,
    [int]$BoundaryRegisters = 125,
    [int]$BoundaryWriteBits = 1968,
    [int]$BoundaryWriteRegisters = 123,
    [int]$StressCount = 100,
    [string]$StressDelay = "100ms"
)

$ErrorActionPreference = "Stop"

go run ./examples/serial_gateway_loopback `
    -serial $Serial `
    -gateway $Gateway `
    -unit $Unit `
    -baud $Baud `
    -timeout $Timeout `
    -suite $Suite `
    -boundary-bits $BoundaryBits `
    -boundary-registers $BoundaryRegisters `
    -boundary-write-bits $BoundaryWriteBits `
    -boundary-write-registers $BoundaryWriteRegisters `
    -stress-count $StressCount `
    -stress-delay $StressDelay
