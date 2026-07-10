package modbus

import (
	"context"
	"io"
	"net"
	"sync"
)

// ServerHandle represents a running slave/server instance.
type ServerHandle struct {
	cancel    context.CancelFunc
	done      chan struct{}
	closeOnce sync.Once
	err       error
}

func newServerHandle(cancel context.CancelFunc) *ServerHandle {
	return &ServerHandle{
		cancel: cancel,
		done:   make(chan struct{}),
	}
}

func (h *ServerHandle) run(serve func() error) {
	h.err = serve()
	close(h.done)
}

// Close stops the server and waits for its serving goroutine to exit.
func (h *ServerHandle) Close() error {
	h.closeOnce.Do(h.cancel)
	return h.Wait()
}

// Wait blocks until the server exits and returns the serving error, if any.
func (h *ServerHandle) Wait() error {
	<-h.done
	return h.err
}

// Done is closed when the server has exited.
func (h *ServerHandle) Done() <-chan struct{} {
	return h.done
}

// StartTCPServer starts a TCP slave/server on address and returns a handle that
// can be closed to stop it.
func StartTCPServer(ctx context.Context, address string, handler Handler) (*ServerHandle, error) {
	ln, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	return StartTCPServerOn(ctx, ln, handler), nil
}

// StartTCPServerOn starts a TCP slave/server on an existing listener and
// returns a handle that can be closed to stop it.
func StartTCPServerOn(ctx context.Context, ln net.Listener, handler Handler) *ServerHandle {
	serverCtx, cancel := context.WithCancel(ctx)
	handle := newServerHandle(cancel)
	server := NewTCPServer(handler)
	go handle.run(func() error {
		return server.Serve(serverCtx, ln)
	})
	return handle
}

// StartRTUServer starts an RTU slave/server on conn and returns a handle that
// can be closed to stop it.
func StartRTUServer(ctx context.Context, conn io.ReadWriteCloser, handler Handler) *ServerHandle {
	serverCtx, cancel := context.WithCancel(ctx)
	handle := newServerHandle(cancel)
	server := NewRTUServer(handler)
	go handle.run(func() error {
		return server.Serve(serverCtx, conn)
	})
	return handle
}
