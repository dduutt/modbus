package serial

import (
	"fmt"
	"io"
	"time"

	goserial "go.bug.st/serial"
)

type Parity string

const (
	NoParity    Parity = "none"
	OddParity   Parity = "odd"
	EvenParity  Parity = "even"
	MarkParity  Parity = "mark"
	SpaceParity Parity = "space"
)

type StopBits string

const (
	OneStopBit          StopBits = "1"
	OnePointFiveStopBit StopBits = "1.5"
	TwoStopBits         StopBits = "2"
)

type Config struct {
	PortName string
	BaudRate int
	DataBits int
	StopBits StopBits
	Parity   Parity
	Timeout  time.Duration
}

func Open(config Config) (io.ReadWriteCloser, error) {
	if config.PortName == "" {
		return nil, fmt.Errorf("serial: port name is required")
	}
	if config.BaudRate == 0 {
		config.BaudRate = 9600
	}
	if config.DataBits == 0 {
		config.DataBits = 8
	}
	if config.StopBits == "" {
		config.StopBits = OneStopBit
	}
	if config.Parity == "" {
		config.Parity = NoParity
	}
	mode := &goserial.Mode{
		BaudRate: config.BaudRate,
		DataBits: config.DataBits,
		Parity:   convertParity(config.Parity),
		StopBits: convertStopBits(config.StopBits),
	}
	port, err := goserial.Open(config.PortName, mode)
	if err != nil {
		return nil, err
	}
	if config.Timeout > 0 {
		if err := port.SetReadTimeout(config.Timeout); err != nil {
			_ = port.Close()
			return nil, err
		}
	}
	return port, nil
}

func GetPortsList() ([]string, error) {
	return goserial.GetPortsList()
}

func convertParity(parity Parity) goserial.Parity {
	switch parity {
	case NoParity:
		return goserial.NoParity
	case OddParity:
		return goserial.OddParity
	case EvenParity:
		return goserial.EvenParity
	case MarkParity:
		return goserial.MarkParity
	case SpaceParity:
		return goserial.SpaceParity
	default:
		return goserial.NoParity
	}
}

func convertStopBits(stopBits StopBits) goserial.StopBits {
	switch stopBits {
	case OneStopBit:
		return goserial.OneStopBit
	case OnePointFiveStopBit:
		return goserial.OnePointFiveStopBits
	case TwoStopBits:
		return goserial.TwoStopBits
	default:
		return goserial.OneStopBit
	}
}
