package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/httpdrain"
)

// connectionTracker keeps ownership of accepted connections after net/http
// stops tracking hijacked connections such as WebSockets.
type connectionTracker struct {
	mu              sync.Mutex
	connections     map[*trackedConnection]struct{}
	acceptsInFlight int
	empty           chan struct{}
}

func newConnectionTracker() *connectionTracker {
	empty := make(chan struct{})
	close(empty)
	return &connectionTracker{
		connections: make(map[*trackedConnection]struct{}),
		empty:       empty,
	}
}

func (t *connectionTracker) wrap(listener net.Listener) net.Listener {
	return &trackingListener{Listener: listener, tracker: t}
}

func (t *connectionTracker) attach(server *http.Server) {
	previous := server.ConnState
	server.ConnState = func(connection net.Conn, state http.ConnState) {
		t.recordState(connection, state)
		if previous != nil {
			previous(connection, state)
		}
	}
}

func (t *connectionTracker) recordState(connection net.Conn, state http.ConnState) {
	if state != http.StateHijacked {
		return
	}
	tracked := unwrapTrackedConnection(connection)
	if tracked == nil {
		return
	}
	t.mu.Lock()
	_, owned := t.connections[tracked]
	newlyHijacked := owned && !tracked.hijacked
	if newlyHijacked {
		tracked.hijacked = true
	}
	t.mu.Unlock()
	if newlyHijacked {
		httpdrain.BeginHijackedConnection()
	}
}

// Register the Accept call before entering the underlying listener so a drain
// cannot observe the old empty state after a socket is accepted but before it
// is added to connections.
func (t *connectionTracker) beginAccept() {
	t.mu.Lock()
	if len(t.connections) == 0 && t.acceptsInFlight == 0 {
		t.empty = make(chan struct{})
	}
	t.acceptsInFlight++
	t.mu.Unlock()
}

func (t *connectionTracker) finishAccept(connection net.Conn) net.Conn {
	var tracked *trackedConnection
	if connection != nil {
		tracked = &trackedConnection{Conn: connection, tracker: t}
	}

	t.mu.Lock()
	t.acceptsInFlight--
	if tracked != nil {
		t.connections[tracked] = struct{}{}
	}
	if len(t.connections) == 0 && t.acceptsInFlight == 0 {
		close(t.empty)
	}
	t.mu.Unlock()

	if tracked == nil {
		return nil
	}
	return preserveHalfClose(tracked)
}

func (t *connectionTracker) remove(connection *trackedConnection) {
	t.mu.Lock()
	if _, ok := t.connections[connection]; !ok {
		t.mu.Unlock()
		return
	}
	delete(t.connections, connection)
	wasHijacked := connection.hijacked
	if len(t.connections) == 0 && t.acceptsInFlight == 0 {
		close(t.empty)
	}
	t.mu.Unlock()
	if wasHijacked {
		httpdrain.EndHijackedConnection()
	}
}

func (t *connectionTracker) wait(ctx context.Context) error {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	empty := t.empty
	t.mu.Unlock()
	select {
	case <-empty:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (t *connectionTracker) closeAll() {
	if t == nil {
		return
	}
	t.mu.Lock()
	connections := make([]*trackedConnection, 0, len(t.connections))
	for connection := range t.connections {
		connections = append(connections, connection)
	}
	t.mu.Unlock()
	for _, connection := range connections {
		_ = connection.Close()
	}
}

type trackingListener struct {
	net.Listener
	tracker *connectionTracker
}

func (l *trackingListener) Accept() (net.Conn, error) {
	l.tracker.beginAccept()
	connection, err := l.Listener.Accept()
	if err != nil {
		l.tracker.finishAccept(nil)
		return nil, err
	}
	return l.tracker.finishAccept(connection), nil
}

type closeWriter interface {
	CloseWrite() error
}

type closeReader interface {
	CloseRead() error
}

type trackedConnection struct {
	net.Conn
	tracker   *connectionTracker
	closeOnce sync.Once
	closeErr  error
	hijacked  bool
}

func (c *trackedConnection) Close() error {
	c.closeOnce.Do(func() {
		c.closeErr = c.Conn.Close()
		c.tracker.remove(c)
	})
	return c.closeErr
}

type trackedConnectionWithCloseWrite struct {
	*trackedConnection
	closeWriter closeWriter
}

func (c *trackedConnectionWithCloseWrite) CloseWrite() error {
	return c.closeWriter.CloseWrite()
}

type trackedConnectionWithCloseRead struct {
	*trackedConnection
	closeReader closeReader
}

func (c *trackedConnectionWithCloseRead) CloseRead() error {
	return c.closeReader.CloseRead()
}

type trackedConnectionWithHalfClose struct {
	*trackedConnection
	closeWriter closeWriter
	closeReader closeReader
}

func (c *trackedConnectionWithHalfClose) CloseWrite() error {
	return c.closeWriter.CloseWrite()
}

func (c *trackedConnectionWithHalfClose) CloseRead() error {
	return c.closeReader.CloseRead()
}

func preserveHalfClose(connection *trackedConnection) net.Conn {
	writer, canCloseWrite := connection.Conn.(closeWriter)
	reader, canCloseRead := connection.Conn.(closeReader)
	switch {
	case canCloseWrite && canCloseRead:
		return &trackedConnectionWithHalfClose{
			trackedConnection: connection,
			closeWriter:       writer,
			closeReader:       reader,
		}
	case canCloseWrite:
		return &trackedConnectionWithCloseWrite{
			trackedConnection: connection,
			closeWriter:       writer,
		}
	case canCloseRead:
		return &trackedConnectionWithCloseRead{
			trackedConnection: connection,
			closeReader:       reader,
		}
	default:
		return connection
	}
}

func unwrapTrackedConnection(connection net.Conn) *trackedConnection {
	switch value := connection.(type) {
	case *trackedConnection:
		return value
	case *trackedConnectionWithCloseWrite:
		return value.trackedConnection
	case *trackedConnectionWithCloseRead:
		return value.trackedConnection
	case *trackedConnectionWithHalfClose:
		return value.trackedConnection
	default:
		return nil
	}
}

func listenAndServeTracked(server *http.Server, tracker *connectionTracker) error {
	address := server.Addr
	if address == "" {
		address = ":http"
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	return server.Serve(tracker.wrap(listener))
}

// shutdownTrackedServer uses one deadline for normal HTTP requests and
// hijacked connections. Only connections still open at the deadline are
// forcibly closed.
func shutdownTrackedServer(ctx context.Context, server *http.Server, tracker *connectionTracker) error {
	shutdownErr := server.Shutdown(ctx)
	waitErr := tracker.wait(ctx)
	if waitErr != nil {
		tracker.closeAll()
	}
	return errors.Join(shutdownErr, waitErr)
}
