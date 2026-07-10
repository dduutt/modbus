package modbus

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

type TCPTransport struct {
	address string
	dialer  DialContextFunc
	timeout time.Duration
	codec   TCPCodec

	mu     sync.Mutex
	conn   net.Conn
	nextID uint16
	closed bool
}

func NewTCPTransport(address string, opts ...TCPOption) *TCPTransport {
	cfg := tcpOptions{timeout: 5 * time.Second}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.dialer == nil {
		d := net.Dialer{Timeout: cfg.timeout}
		cfg.dialer = d.DialContext
	}
	return &TCPTransport{
		address: address,
		dialer:  cfg.dialer,
		timeout: cfg.timeout,
		codec:   TCPCodec{},
		nextID:  1,
	}
}

func (t *TCPTransport) Do(ctx context.Context, unitID byte, request PDU) (PDU, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return PDU{}, ErrClosed
	}
	conn, err := t.ensureConn(ctx)
	if err != nil {
		return PDU{}, err
	}
	txID := t.nextID
	t.nextID++
	req, err := t.codec.Encode(unitID, txID, request)
	if err != nil {
		return PDU{}, err
	}
	if err := setConnDeadline(ctx, conn, t.timeout); err != nil {
		return PDU{}, err
	}
	if _, err := conn.Write(req); err != nil {
		t.closeLocked()
		return PDU{}, err
	}
	frame, err := t.codec.Decode(conn)
	if err != nil {
		t.closeLocked()
		return PDU{}, err
	}
	if frame.TransactionID != txID {
		t.closeLocked()
		return PDU{}, fmt.Errorf("%w: transaction id mismatch", ErrInvalidResponse)
	}
	if frame.UnitID != unitID {
		return PDU{}, fmt.Errorf("%w: unit id mismatch", ErrInvalidResponse)
	}
	return frame.PDU, nil
}

func (t *TCPTransport) Connect(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return ErrClosed
	}
	_, err := t.ensureConn(ctx)
	return err
}

func (t *TCPTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	return t.closeLocked()
}

func (t *TCPTransport) ensureConn(ctx context.Context) (net.Conn, error) {
	if t.conn != nil {
		return t.conn, nil
	}
	conn, err := t.dialer(ctx, "tcp", t.address)
	if err != nil {
		return nil, err
	}
	t.conn = conn
	return conn, nil
}

func (t *TCPTransport) closeLocked() error {
	if t.conn == nil {
		return nil
	}
	err := t.conn.Close()
	t.conn = nil
	return err
}

func setConnDeadline(ctx context.Context, conn interface{ SetDeadline(time.Time) error }, fallback time.Duration) error {
	deadline, ok := ctx.Deadline()
	if !ok && fallback > 0 {
		deadline = time.Now().Add(fallback)
		ok = true
	}
	if !ok {
		return conn.SetDeadline(time.Time{})
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return errors.Join(ctx.Err(), ErrTimeout)
	default:
		return nil
	}
}
