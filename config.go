package modbus

import (
	"fmt"
	"io"
	"time"
)

type ClientMode string

const (
	ModeTCP        ClientMode = "tcp"
	ModeRTU        ClientMode = "rtu"
	ModeRTUOverTCP ClientMode = "rtu-over-tcp"
)

type ClientConfig struct {
	Mode      ClientMode
	Address   string
	UnitID    byte
	Timeout   time.Duration
	Conn      io.ReadWriteCloser
	Dialer    DialContextFunc
	RTUTiming RTUTiming
}

func NewClientFromConfig(config ClientConfig) (*Client, error) {
	transport, err := NewTransportFromConfig(config)
	if err != nil {
		return nil, err
	}
	opts := make([]Option, 0, 2)
	if config.UnitID != 0 {
		opts = append(opts, WithUnitID(config.UnitID))
	}
	if config.Timeout > 0 {
		opts = append(opts, WithTimeout(config.Timeout))
	}
	return NewClient(transport, opts...), nil
}

func NewTransportFromConfig(config ClientConfig) (Transport, error) {
	switch config.Mode {
	case ModeTCP:
		if config.Address == "" {
			return nil, fmt.Errorf("%w: tcp address is required", ErrInvalidRequest)
		}
		opts := make([]TCPOption, 0, 2)
		if config.Timeout > 0 {
			opts = append(opts, WithTCPTimeout(config.Timeout))
		}
		if config.Dialer != nil {
			opts = append(opts, WithTCPDialer(config.Dialer))
		}
		return NewTCPTransport(config.Address, opts...), nil
	case ModeRTU:
		if config.Conn == nil {
			return nil, fmt.Errorf("%w: rtu connection is required", ErrInvalidRequest)
		}
		opts := makeRTUOptions(config)
		return NewRTUTransport(config.Conn, opts...), nil
	case ModeRTUOverTCP:
		if config.Address == "" {
			return nil, fmt.Errorf("%w: rtu-over-tcp address is required", ErrInvalidRequest)
		}
		opts := make([]RTUOverTCPOption, 0, 3)
		if config.Timeout > 0 {
			opts = append(opts, WithRTUOverTCPTimeout(config.Timeout))
		}
		if config.Dialer != nil {
			opts = append(opts, WithRTUOverTCPDialer(config.Dialer))
		}
		if rtuOpts := makeRTUOptions(config); len(rtuOpts) > 0 {
			opts = append(opts, WithRTUOverTCPRTUOptions(rtuOpts...))
		}
		return NewRTUOverTCPTransport(config.Address, opts...), nil
	default:
		return nil, fmt.Errorf("%w: unsupported client mode %q", ErrInvalidRequest, config.Mode)
	}
}

func makeRTUOptions(config ClientConfig) []RTUOption {
	opts := make([]RTUOption, 0, 2)
	if config.Timeout > 0 {
		opts = append(opts, WithRTUTimeout(config.Timeout))
	}
	if config.RTUTiming != (RTUTiming{}) {
		opts = append(opts, WithRTUTiming(config.RTUTiming))
	}
	return opts
}
