# modbus

`github.com/dduutt/modbus` 是一个 Go 语言 Modbus 通信库，提供客户端、传输层、协议编解码、数据模型和从站模拟能力。

当前支持 Modbus TCP、Modbus RTU、RTU-over-TCP，适用于设备通信、网关接入、协议测试、从站模拟和本地自动化验证场景。

## 功能特性

- Modbus TCP 客户端，支持 MBAP 编解码、事务 ID 校验、Unit ID 校验。
- Modbus RTU 客户端，基于调用方传入的 `io.ReadWriteCloser`。
- RTU-over-TCP 客户端，适用于 TCP 透明转发 RTU ADU 的网关设备。
- TCP 从站模拟。
- RTU 从站模拟。
- 线程安全的内存 `DataStore`，适合测试、模拟器和本地联调。
- `ClientConfig`，支持通过配置创建 TCP、RTU、RTU-over-TCP 客户端。
- 支持线圈、离散输入、保持寄存器、输入寄存器的读写。
- 支持诊断、通信事件、Server ID、文件记录、FIFO、Device Identification 等高级功能码。
- Tag/Value API，支持 PLC 风格变量访问和常见数据类型转换。
- 串口适配包 `github.com/dduutt/modbus/serial` 使用成熟串口库 `go.bug.st/serial`。

## 安装

```powershell
go get github.com/dduutt/modbus
```

核心包：

```go
import "github.com/dduutt/modbus"
```

串口适配包：

```go
import "github.com/dduutt/modbus/serial"
```

## 快速开始：Modbus TCP 客户端

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/dduutt/modbus"
)

func main() {
	ctx := context.Background()

	client := modbus.NewClient(
		modbus.NewTCPTransport("127.0.0.1:502"),
		modbus.WithUnitID(1),
		modbus.WithTimeout(3*time.Second),
	)
	defer client.Close()

	values, err := client.ReadHoldingRegisters(ctx, 0, 3)
	if err != nil {
		panic(err)
	}
	fmt.Printf("holding registers: %#v\n", values)
}
```

也可以通过配置创建客户端：

```go
client, err := modbus.NewClientFromConfig(modbus.ClientConfig{
	Mode:    modbus.ModeTCP,
	Address: "127.0.0.1:502",
	UnitID:  1,
	Timeout: 3 * time.Second,
})
if err != nil {
	panic(err)
}
defer client.Close()
```

## RTU 客户端

核心包不直接打开系统串口。RTU 传输层只依赖 `io.ReadWriteCloser`，串口打开由调用方或 `serial` 适配包负责。

```go
client := modbus.NewClient(
	modbus.NewRTUTransport(serialPort),
	modbus.WithUnitID(1),
	modbus.WithTimeout(3*time.Second),
)
defer client.Close()
```

使用内置串口适配包：

```go
package main

import (
	"time"

	"github.com/dduutt/modbus"
	"github.com/dduutt/modbus/serial"
)

func main() {
	port, err := serial.Open(serial.Config{
		PortName: "COM3",
		BaudRate: 9600,
		DataBits: 8,
		StopBits: serial.OneStopBit,
		Parity:   serial.NoParity,
		Timeout:  time.Second,
	})
	if err != nil {
		panic(err)
	}
	defer port.Close()

	client := modbus.NewClient(
		modbus.NewRTUTransport(port),
		modbus.WithUnitID(1),
	)
	defer client.Close()
}
```

配置 RTU 总线时序：

```go
transport := modbus.NewRTUTransport(port, modbus.WithRTUTiming(modbus.RTUTiming{
	BaudRate: 9600,
	DataBits: 8,
	Parity:   false,
	StopBits: 1,
}))

client := modbus.NewClient(transport, modbus.WithUnitID(1))
```

## RTU-over-TCP 客户端

RTU-over-TCP 使用 RTU ADU 和 CRC 帧格式，但承载在 TCP 流上。

```go
client := modbus.NewClient(
	modbus.NewRTUOverTCPTransport("127.0.0.1:1502"),
	modbus.WithUnitID(1),
	modbus.WithTimeout(3*time.Second),
)
defer client.Close()
```

通过配置创建：

```go
client, err := modbus.NewClientFromConfig(modbus.ClientConfig{
	Mode:    modbus.ModeRTUOverTCP,
	Address: "127.0.0.1:1502",
	UnitID:  1,
	Timeout: 3 * time.Second,
})
if err != nil {
	panic(err)
}
defer client.Close()
```

## 常用 API

下面示例假设已经创建好 `client`，并在业务函数中统一处理错误：

```go
func useClient(ctx context.Context, client *modbus.Client) error {
	coils, err := client.ReadCoils(ctx, 0, 8)
	if err != nil {
		return err
	}
	fmt.Printf("coils: %#v\n", coils)

	if err := client.WriteSingleCoil(ctx, 0, true); err != nil {
		return err
	}

	registers, err := client.ReadHoldingRegisters(ctx, 0, 3)
	if err != nil {
		return err
	}
	fmt.Printf("holding registers: %#v\n", registers)

	if err := client.WriteSingleRegister(ctx, 1, 222); err != nil {
		return err
	}

	if err := client.WriteMultipleRegisters(ctx, 10, []uint16{11, 22, 33}); err != nil {
		return err
	}

	return nil
}
```

等价的单项 API 包括：

```go
coils, err := client.ReadCoils(ctx, 0, 8)
registers, err := client.ReadHoldingRegisters(ctx, 0, 3)
err = client.WriteSingleCoil(ctx, 0, true)
err = client.WriteSingleRegister(ctx, 1, 222)
err = client.WriteMultipleRegisters(ctx, 10, []uint16{11, 22, 33})
```

高级功能码：

```go
diagnosticValue, err := client.Diagnostic(ctx, 0x0000, 0xCAFE)
if err != nil {
	return err
}
fmt.Printf("diagnostic: 0x%04X\n", diagnosticValue)

exceptionStatus, err := client.ReadExceptionStatus(ctx)
if err != nil {
	return err
}
fmt.Printf("exception status: 0x%02X\n", exceptionStatus)

counter, err := client.GetCommEventCounter(ctx)
if err != nil {
	return err
}
fmt.Printf("event counter: %#v\n", counter)

eventLog, err := client.GetCommEventLog(ctx)
if err != nil {
	return err
}
fmt.Printf("event log: %#v\n", eventLog)

serverID, err := client.ReportServerID(ctx)
if err != nil {
	return err
}
fmt.Printf("server id: % X\n", serverID)
```

文件记录：

```go
records, err := client.ReadFileRecords(ctx, []modbus.FileRecordRequest{
	{FileNumber: 7, RecordNumber: 0, RecordLength: 2},
})
if err != nil {
	return err
}
fmt.Printf("file records: %#v\n", records)
```

FIFO：

```go
fifoValues, err := client.ReadFIFOQueue(ctx, 0x04DE)
if err != nil {
	return err
}
fmt.Printf("fifo: %#v\n", fifoValues)
```

设备识别：

```go
deviceInfo, err := client.ReadDeviceIdentification(ctx, modbus.ReadDeviceIDCodeBasic)
if err != nil {
	return err
}
vendorName := string(deviceInfo.Objects[0x00])
fmt.Printf("vendor: %s\n", vendorName)
```

## 从站模拟

TCP 从站：

```go
store := modbus.NewMemoryDataStore()
_ = store.WriteHoldingRegisters(0, []uint16{100, 200, 300})
_ = store.WriteCoils(0, []bool{true, false, true})

handler := modbus.NewDataStoreHandler(store)
server := modbus.NewTCPServer(handler)

err := server.ListenAndServe(context.Background(), "127.0.0.1:1502")
_ = err
```

配置高级从站数据：

```go
exceptionStatus := byte(0x5A)
handler.ExceptionStatus = &exceptionStatus
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
```

RTU 从站：

```go
server := modbus.NewRTUServer(handler)
err := server.Serve(ctx, serialPort)
_ = err
```

## Tag/Value API

代码方式创建 tag：

```go
tag := modbus.HoldingRegister(0).As(modbus.TypeFloat32)

value, err := client.ReadTag(ctx, tag)
if err != nil {
	panic(err)
}

temperature, ok := value.Float32()
if !ok {
	panic("unexpected value type")
}
_ = temperature
```

配置字符串只支持短名，不使用 `holding-register:1:REAL[2]` 这种长格式。

```go
tag, err := modbus.ParseTag("hr:0:f32:1")
if err != nil {
	panic(err)
}
_ = tag
```

格式：

```text
area:address:type:quantity
```

区域短名：

```text
c   coils
di  discrete inputs
hr  holding registers
ir  input registers
```

类型短名：

```text
b    bool
u16  uint16
i16  int16
u32  uint32
i32  int32
f32  float32
u64  uint64
i64  int64
f64  float64
by   bytes
str  string
```

批量接口：

- `ReadTags` 会按 unit id 和区域分组，并在协议限制内合并相邻或重叠地址范围。
- `WriteTags` 支持 coils 和 holding registers，也会在协议限制内合并写入范围。

## 连接错误处理策略

本库不自动重试当前失败请求。

原因是写操作存在不确定性：设备可能已经执行了请求，但响应在链路上丢失。如果库自动重试，可能造成重复写入。

当前策略：

- TCP 和 RTU-over-TCP 在读写错误后关闭坏连接。
- 当前请求直接返回错误。
- 下一次请求会重新拨号建立连接。
- 串口 RTU 使用调用方传入的 `io.ReadWriteCloser`，串口重开或重连由调用方或适配层处理。

## 示例程序

```powershell
go run ./examples/tcp_slave
go run ./examples/tcp_client
go run ./examples/tcp_slave_advanced
go run ./examples/tcp_client_advanced
go run ./examples/rtu_slave
go run ./examples/rtu_client
go run ./examples/rtu_over_tcp_loopback
go run ./examples/tags
go run ./examples/tags_loopback
go run ./examples/advanced_loopback
go run ./examples/list_serial_ports
```

## 本地测试

快速本地全量测试：

```powershell
.\scripts\test_all_local.ps1
```

包含 race 测试：

```powershell
.\scripts\test_all_local.ps1 -Race
```

单独测试 TCP 从站：

```powershell
.\scripts\test_tcp_slave.ps1
.\scripts\test_tcp_slave_advanced.ps1
```

常规 Go 验证：

```powershell
go test -count=1 ./...
go vet ./...
go test -race -count=1 ./...
```

## 已支持的功能码

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

## 当前未实现

- Modbus ASCII。
- UDP transport。
- TLS/MBAPS。
- 自动重试当前失败请求。
- 根包直接打开串口。
- 本项目支持列表之外的厂商私有功能码。
