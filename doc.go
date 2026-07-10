// Package modbus provides Modbus TCP, Modbus RTU, RTU-over-TCP, and slave
// simulation support.
//
// TCP clients use MBAP framing:
//
//	client := modbus.NewClient(
//		modbus.NewTCPTransport("127.0.0.1:502"),
//		modbus.WithUnitID(1),
//	)
//
// RTU clients use any io.ReadWriteCloser. The root package does not open
// operating-system serial ports directly; use the modbus/serial adapter or pass
// an application-owned connection into NewRTUTransport.
//
// RTU-over-TCP keeps RTU ADU and CRC framing over a TCP stream:
//
//	client := modbus.NewClient(
//		modbus.NewRTUOverTCPTransport("127.0.0.1:1502"),
//		modbus.WithUnitID(1),
//	)
//
// Configuration-driven clients can be created with ClientConfig and
// NewClientFromConfig. For serial RTU, ClientConfig.Conn must already be open.
// Slave simulations can be run directly with Serve/ListenAndServe or started
// with StartTCPServer/StartRTUServer when callers need a Close handle.
//
// Tag reads and writes support builder-style tags:
//
//	tag := modbus.HoldingRegister(0).As(modbus.TypeFloat32)
//
// ParseTag is intended for configuration files and accepts short names only,
// for example "hr:0:f32:1".
//
// The package does not automatically retry the current request after a
// connection error. TCP and RTU-over-TCP close bad connections and let the next
// request dial a fresh connection.
package modbus
