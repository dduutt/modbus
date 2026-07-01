package modbus

import (
	"context"
	"io"
	"net"
	"time"
)

type Transport interface {
	Do(ctx context.Context, unitID byte, request PDU) (PDU, error)
	Close() error
}

type Option func(*clientOptions)

type clientOptions struct {
	unitID  byte
	timeout time.Duration
}

func WithUnitID(unitID byte) Option {
	return func(o *clientOptions) {
		o.unitID = unitID
	}
}

func WithTimeout(timeout time.Duration) Option {
	return func(o *clientOptions) {
		o.timeout = timeout
	}
}

type deadlineConn interface {
	io.ReadWriteCloser
	SetDeadline(time.Time) error
}

type DialContextFunc func(ctx context.Context, network, address string) (net.Conn, error)

type TCPOption func(*tcpOptions)

type tcpOptions struct {
	dialer  DialContextFunc
	timeout time.Duration
}

func WithTCPDialer(dialer DialContextFunc) TCPOption {
	return func(o *tcpOptions) {
		o.dialer = dialer
	}
}

func WithTCPTimeout(timeout time.Duration) TCPOption {
	return func(o *tcpOptions) {
		o.timeout = timeout
	}
}

type RTUOption func(*rtuOptions)

type rtuOptions struct {
	timeout time.Duration
	timing  RTUTiming
}

func WithRTUTimeout(timeout time.Duration) RTUOption {
	return func(o *rtuOptions) {
		o.timeout = timeout
	}
}

type RTUTiming struct {
	BaudRate        int
	DataBits        int
	Parity          bool
	StopBits        int
	TurnaroundDelay time.Duration
	PreDelay        time.Duration
	PostDelay       time.Duration
}

func WithRTUTiming(timing RTUTiming) RTUOption {
	return func(o *rtuOptions) {
		o.timing = timing
	}
}

type RTUOverTCPOption func(*rtuOverTCPOptions)

type rtuOverTCPOptions struct {
	dialer  DialContextFunc
	timeout time.Duration
	rtuOpts []RTUOption
}

func WithRTUOverTCPDialer(dialer DialContextFunc) RTUOverTCPOption {
	return func(o *rtuOverTCPOptions) {
		o.dialer = dialer
	}
}

func WithRTUOverTCPTimeout(timeout time.Duration) RTUOverTCPOption {
	return func(o *rtuOverTCPOptions) {
		o.timeout = timeout
	}
}

func WithRTUOverTCPRTUOptions(opts ...RTUOption) RTUOverTCPOption {
	return func(o *rtuOverTCPOptions) {
		o.rtuOpts = append(o.rtuOpts, opts...)
	}
}
