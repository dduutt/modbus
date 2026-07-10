# modbus

`github.com/dduutt/modbus` 是一个 Go 语言 Modbus 通信库，提供客户端、服务端模拟、RTU 串口适配和配置化 Tag 读写能力。

项目目标是让设备通信、网关接入、本地回环测试和从站模拟都能使用同一套 API。所有客户端请求都接受 `context.Context`，便于设置超时、取消请求和接入上层任务生命周期。

## 特性

- 支持 Modbus TCP、Modbus RTU、RTU-over-TCP。
- 支持 TCP 从站模拟和 RTU 从站模拟。
- 支持常用线圈、离散输入、保持寄存器、输入寄存器读写。
- 支持诊断、通信事件、文件记录、FIFO 队列、设备识别等扩展功能码。
- 提供 `Tag` / `Value` API，可按配置地址直接读写 `bool`、整数、浮点、字节和字符串。
- 提供批量 `ReadTags` / `WriteTags`，会合并相邻地址以减少请求数量。
- 根包不直接打开系统串口，RTU 通过 `io.ReadWriteCloser` 接入；需要串口时使用 `github.com/dduutt/modbus/serial`。

## 安装

```powershell
go get github.com/dduutt/modbus
```

如需访问系统串口：

```go
import "github.com/dduutt/modbus/serial"
```

## 快速开始

### TCP 客户端

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/dduutt/modbus"
)

func main() {
    ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
    defer cancel()

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

也可以使用便捷构造函数：

```go
client := modbus.NewTCPClient("127.0.0.1:502", modbus.WithUnitID(1))
defer client.Close()
```

如果希望在启动阶段提前发现设备离线、端口不可达等连接问题，可以显式连接：

```go
if err := client.Connect(ctx); err != nil {
    panic(err)
}
```

`Connect` 是可选的。不调用时，TCP 和 RTU-over-TCP 会在第一次请求时自动连接；RTU 串口连接仍由调用方打开后传入。

### TCP 从站模拟

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"

    "github.com/dduutt/modbus"
)

func main() {
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    store := modbus.NewMemoryDataStore()
    _ = store.WriteHoldingRegisters(0, []uint16{100, 200, 300})
    _ = store.WriteCoils(0, []bool{true, false, true})

    handle, err := modbus.StartTCPServer(ctx, "127.0.0.1:1502", modbus.NewDataStoreHandler(store))
    if err != nil {
        log.Fatal(err)
    }
    defer handle.Close()

    <-ctx.Done()
}
```

RTU 从站也可以用同样的关闭模型：

```go
handle := modbus.StartRTUServer(ctx, port, modbus.NewDataStoreHandler(store))
defer handle.Close()
```

### RTU 客户端

RTU 传输层接收任意 `io.ReadWriteCloser`。真实串口可使用 `modbus/serial`：

```go
package main

import (
    "context"

    "github.com/dduutt/modbus"
    "github.com/dduutt/modbus/serial"
)

func main() {
    port, err := serial.Open(serial.Config{
        PortName: "COM2",
        BaudRate: 9600,
        DataBits: 8,
        Parity:   serial.NoParity,
        StopBits: serial.OneStopBit,
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

    coils, err := client.ReadCoils(context.Background(), 0, 3)
    if err != nil {
        panic(err)
    }
    _ = coils
}
```

### RTU-over-TCP

RTU-over-TCP 在 TCP 流上保留 RTU ADU 和 CRC 帧格式：

```go
client := modbus.NewClient(
    modbus.NewRTUOverTCPTransport("127.0.0.1:1502"),
    modbus.WithUnitID(1),
)
defer client.Close()
```

## 配置化客户端

`ClientConfig` 可用于从配置文件或启动参数创建客户端：

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

可选模式：

- `modbus.ModeTCP`
- `modbus.ModeRTU`
- `modbus.ModeRTUOverTCP`

## Tag / Value API

构造式 Tag 适合在代码中使用：

```go
temperature := modbus.HoldingRegister(0).As(modbus.TypeFloat32)
running := modbus.Coil(0)

value, err := client.ReadTag(context.Background(), temperature)
if err != nil {
    panic(err)
}

f, ok := value.Float32()
if !ok {
    panic("unexpected value type")
}

fmt.Printf("temperature: %.2f\n", f)

err = client.WriteTag(context.Background(), running, true)
```

配置字符串适合放在配置文件中，格式为：

```text
<area>:<address>[:<type>[:<quantity>]]
```

示例：

```go
tag, err := modbus.ParseTag("hr:0:f32:1")
if err != nil {
    panic(err)
}
```

可用区域短名：

| 短名 | 区域 |
| --- | --- |
| `c` | coil |
| `di` | discrete input |
| `hr` | holding register |
| `ir` | input register |

可用数据类型短名：

| 短名 | 类型 |
| --- | --- |
| `b` | bool |
| `u16` | uint16 |
| `i16` | int16 |
| `u32` | uint32 |
| `i32` | int32 |
| `f32` | float32 |
| `u64` | uint64 |
| `i64` | int64 |
| `f64` | float64 |
| `by` | bytes |
| `str` | string |

批量读写示例：

```go
values, err := client.ReadTags(ctx, map[string]modbus.Tag{
    "temperature": modbus.HoldingRegister(0).As(modbus.TypeFloat32),
    "running":     modbus.Coil(0),
})

err = client.WriteTags(ctx, map[string]modbus.TagValue{
    "temperature": {
        Tag:   modbus.HoldingRegister(0).As(modbus.TypeFloat32),
        Value: float32(26.75),
    },
    "running": {
        Tag:   modbus.Coil(0),
        Value: true,
    },
})
```

## 支持的客户端 API

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

## 功能码覆盖

| 功能码 | 名称 |
| --- | --- |
| `0x01` | Read Coils |
| `0x02` | Read Discrete Inputs |
| `0x03` | Read Holding Registers |
| `0x04` | Read Input Registers |
| `0x05` | Write Single Coil |
| `0x06` | Write Single Register |
| `0x07` | Read Exception Status |
| `0x08` | Diagnostics |
| `0x0B` | Get Comm Event Counter |
| `0x0C` | Get Comm Event Log |
| `0x0F` | Write Multiple Coils |
| `0x10` | Write Multiple Registers |
| `0x11` | Report Server ID |
| `0x14` | Read File Record |
| `0x15` | Write File Record |
| `0x16` | Mask Write Register |
| `0x17` | Read/Write Multiple Registers |
| `0x18` | Read FIFO Queue |
| `0x2B/0x0E` | Read Device Identification |

## 从站模拟能力

`DataStoreHandler` 基于 `DataStore` 提供默认从站行为，支持：

- coils
- discrete inputs
- holding registers
- input registers
- file records
- FIFO queues
- exception status
- diagnostics
- communication event counter
- communication event log
- server ID
- device identification

如需自定义行为，实现 `Handler` 接口即可：

```go
type Handler interface {
    Handle(ctx context.Context, unitID byte, request modbus.PDU) (modbus.PDU, error)
}
```

## 示例

项目示例位于 `examples/`：

- `examples/tcp_client`
- `examples/tcp_slave`
- `examples/tcp_client_advanced`
- `examples/tcp_slave_advanced`
- `examples/rtu_client`
- `examples/rtu_slave`
- `examples/rtu_over_tcp_loopback`
- `examples/tags`
- `examples/tags_loopback`
- `examples/advanced_loopback`
- `examples/list_serial_ports`

运行示例：

```powershell
go run ./examples/tags_loopback
go run ./examples/rtu_over_tcp_loopback
go run ./examples/advanced_loopback
```

## 测试

基础测试：

```text
go test -count=1 ./...
```

race 检测：

```text
go test -race -count=1 ./...
```

静态检查：

```text
go vet ./...
```

对比 benchmark：

```text
go test -run '^$' -bench BenchmarkComparisonTCP -benchmem -benchtime=2s -count=3
```

## 可靠性测试

可靠性测试覆盖常见采集、读写、协议扩展和并发场景：

- `TestReliabilityTCPScenarioMatrix`：覆盖 TCP 读写线圈、离散输入、保持寄存器、输入寄存器、诊断、通信事件、Server ID、文件记录、FIFO 和设备识别。
- `TestReliabilityRTUOverTCPTagBatching`：覆盖 RTU-over-TCP 上的 `ReadTags` / `WriteTags`、浮点、整数和线圈批量读写。
- `TestReliabilityTagAcquisitionAccuracy`：覆盖 `ReadTags` 批量采集准确性，包含 coils、discrete inputs、holding registers、input registers、`bool`、`int16`、`uint16`、`int32`、`uint32`、`float32`、`int64`、`uint64`、`float64`、`bytes`、`string` 以及 byte/word order 组合。
- `TestReliabilityTagAcquisitionErrorDoesNotReturnPartialData`：验证采集异常时返回错误，不返回部分成功数据。
- `TestReliabilityConcurrentTCPClients`：启动 TCP server，并使用 32 个 TCP client 并发执行 6,400 次读写操作。

并发测试包含：

- `ReadHoldingRegisters`
- `WriteSingleRegister`
- `WriteMultipleRegisters`
- `WriteMultipleCoils`
- `ReadTag`
- `WriteTag`
- `ReadWriteMultipleRegisters`

推荐验证命令：

| 测试项 | 命令 | 结果 |
| --- | --- | --- |
| 单元与可靠性测试 | `go test -count=1 ./...` | 通过 |
| race 检测 | `go test -race -count=1 ./...` | 通过 |
| 静态检查 | `go vet ./...` | 通过 |

## 性能对比

以下结果用于和 [`github.com/simonvetter/modbus`](https://github.com/simonvetter/modbus) 做同机参考对比，不代表不同硬件、系统或真实工业网络下的绝对性能。对比代码在 `comparison_benchmark_test.go` 中，可直接通过 `go test -bench` 复现。

测试口径：

- 两边都使用各自的 TCP client 和 TCP server。
- 本机 `127.0.0.1` 回环，长连接复用。
- 覆盖读保持寄存器、写保持寄存器、读线圈、写线圈和混合读写。
- 不包含 Tag/Value 解码。
- 对方库版本：`github.com/simonvetter/modbus v1.6.4`。

测试环境：

```text
go version go1.26.4 windows/amd64
cpu: 13th Gen Intel(R) Core(TM) i7-1370P
```

测试命令：

```text
go test -run '^$' -bench BenchmarkComparisonTCP -benchmem -benchtime=2s -count=3
```

结果按 3 轮中位数统计：

| 场景 | 本项目 ns/op | 本项目 B/op | 本项目 allocs/op | simonvetter ns/op | simonvetter B/op | simonvetter allocs/op |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| read holding registers | 15,882 | 568 | 17 | 15,688 | 424 | 24 |
| write holding registers | 15,825 | 552 | 16 | 15,548 | 416 | 23 |
| read coils | 15,796 | 512 | 17 | 15,594 | 352 | 22 |
| write coils | 15,884 | 496 | 16 | 15,615 | 328 | 20 |
| mixed read/write | 15,999 | 516 | 16 | 15,839 | 356 | 21 |

结论：

- 两者在本机 TCP 回环场景下延迟接近。
- `simonvetter/modbus` 在该组多场景 benchmark 中延迟和每次操作分配字节略低。
- 本项目在所有对比场景中每次操作分配次数更少。
- Windows 本机 TCP 回环存在调度抖动，性能结论建议以多轮中位数为准。

## 连接与重试策略

- TCP 和 RTU-over-TCP 在连接错误后会关闭坏连接，下一次请求会重新建立连接。
- 库不会在同一次请求失败后自动重试该请求。
- 串口 RTU 的重新打开和重连策略应由调用方或适配层负责。

## 当前未实现

- Modbus ASCII
- UDP transport
- TLS / MBAPS
- 当前请求失败后的自动重试
- 根包直接打开系统串口
- 未列出的厂商私有功能码
