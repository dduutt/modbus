package modbus

import (
	"context"
	"net"
	"sync"
	"time"
)

type RTUOverTCPTransport struct {
	address string
	dialer  DialContextFunc
	timeout time.Duration
	rtuOpts []RTUOption

	mu        sync.Mutex
	transport *RTUTransport
	closed    bool
}

func NewRTUOverTCPTransport(address string, opts ...RTUOverTCPOption) *RTUOverTCPTransport {
	cfg := rtuOverTCPOptions{timeout: 5 * time.Second}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.dialer == nil {
		d := net.Dialer{Timeout: cfg.timeout}
		cfg.dialer = d.DialContext
	}
	return &RTUOverTCPTransport{
		address: address,
		dialer:  cfg.dialer,
		timeout: cfg.timeout,
		rtuOpts: append([]RTUOption(nil), cfg.rtuOpts...),
	}
}

func NewRTUOverTCPClient(address string, opts ...Option) *Client {
	return NewClient(NewRTUOverTCPTransport(address), opts...)
}

func (t *RTUOverTCPTransport) Do(ctx context.Context, unitID byte, request PDU) (PDU, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return PDU{}, ErrClosed
	}
	transport, err := t.ensureTransport(ctx)
	if err != nil {
		return PDU{}, err
	}
	resp, err := transport.Do(ctx, unitID, request)
	if err != nil {
		_ = t.closeTransportLocked()
		return PDU{}, err
	}
	return resp, nil
}

func (t *RTUOverTCPTransport) Connect(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return ErrClosed
	}
	_, err := t.ensureTransport(ctx)
	return err
}

func (t *RTUOverTCPTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	return t.closeTransportLocked()
}

func (t *RTUOverTCPTransport) ensureTransport(ctx context.Context) (*RTUTransport, error) {
	if t.transport != nil {
		return t.transport, nil
	}
	conn, err := t.dialer(ctx, "tcp", t.address)
	if err != nil {
		return nil, err
	}
	opts := append([]RTUOption{WithRTUTimeout(t.timeout)}, t.rtuOpts...)
	t.transport = NewRTUTransport(conn, opts...)
	return t.transport, nil
}

func (t *RTUOverTCPTransport) closeTransportLocked() error {
	if t.transport == nil {
		return nil
	}
	err := t.transport.Close()
	t.transport = nil
	return err
}
