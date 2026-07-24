package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/httpdrain"
)

type gatedAcceptListener struct {
	connection net.Conn
	entered    chan struct{}
	release    chan struct{}
}

func (l *gatedAcceptListener) Accept() (net.Conn, error) {
	close(l.entered)
	<-l.release
	return l.connection, nil
}

func (l *gatedAcceptListener) Close() error {
	return nil
}

func (l *gatedAcceptListener) Addr() net.Addr {
	return l.connection.LocalAddr()
}

func startHijackedConnectionServer(t *testing.T) (*http.Server, *connectionTracker, net.Conn) {
	t.Helper()
	hijacked := make(chan net.Conn, 1)
	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			connection, rw, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Errorf("hijack: %v", err)
				return
			}
			_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: test\r\n\r\n")
			_ = rw.Flush()
			hijacked <- connection
		}),
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	tracker := newConnectionTracker()
	tracker.attach(server)
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- server.Serve(tracker.wrap(listener))
	}()

	var client net.Conn
	t.Cleanup(func() {
		if client != nil {
			_ = client.Close()
		}
		tracker.closeAll()
		_ = server.Close()
		select {
		case <-serveDone:
		case <-time.After(time.Second):
			t.Error("server did not stop")
		}
	})
	client, err = net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Write([]byte("GET / HTTP/1.1\r\nHost: test\r\nConnection: Upgrade\r\nUpgrade: test\r\n\r\n")); err != nil {
		t.Fatal(err)
	}
	select {
	case connection := <-hijacked:
		return server, tracker, connection
	case <-time.After(time.Second):
		t.Fatal("handler did not hijack connection")
		return nil, nil, nil
	}
}

func TestShutdownTrackedServerWaitsForHijackedConnection(t *testing.T) {
	server, tracker, connection := startHijackedConnectionServer(t)
	if got := httpdrain.Status().HijackedConnections; got < 1 {
		t.Fatalf("hijacked connection was not exposed as a drain blocker: %d", got)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- shutdownTrackedServer(ctx, server, tracker)
	}()

	select {
	case err := <-done:
		t.Fatalf("shutdown returned while hijacked connection was open: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("shutdown failed after connection closed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown did not finish after hijacked connection closed")
	}
}

func TestShutdownTrackedServerForceClosesAtOverallDeadline(t *testing.T) {
	server, tracker, _ := startHijackedConnectionServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	err := shutdownTrackedServer(ctx, server, tracker)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed < 75*time.Millisecond {
		t.Fatalf("shutdown returned before overall deadline: %v", elapsed)
	}
	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()
	if err := tracker.wait(waitCtx); err != nil {
		t.Fatalf("forced connection close did not drain tracker: %v", err)
	}
}

func TestConnectionTrackerWaitIncludesAcceptInProgress(t *testing.T) {
	serverConnection, peerConnection := net.Pipe()
	defer peerConnection.Close()
	defer serverConnection.Close()

	listener := &gatedAcceptListener{
		connection: serverConnection,
		entered:    make(chan struct{}),
		release:    make(chan struct{}),
	}
	released := false
	defer func() {
		if !released {
			close(listener.release)
		}
	}()
	tracker := newConnectionTracker()
	type acceptResult struct {
		connection net.Conn
		err        error
	}
	accepted := make(chan acceptResult, 1)
	go func() {
		connection, err := tracker.wrap(listener).Accept()
		accepted <- acceptResult{connection: connection, err: err}
	}()

	select {
	case <-listener.entered:
	case <-time.After(time.Second):
		t.Fatal("listener did not enter Accept")
	}
	pendingCtx, cancelPending := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancelPending()
	if err := tracker.wait(pendingCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wait during Accept = %v, want deadline exceeded", err)
	}

	close(listener.release)
	released = true
	var result acceptResult
	select {
	case result = <-accepted:
	case <-time.After(time.Second):
		t.Fatal("listener did not finish Accept")
	}
	if result.err != nil {
		t.Fatalf("Accept failed: %v", result.err)
	}

	trackedCtx, cancelTracked := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancelTracked()
	if err := tracker.wait(trackedCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wait with accepted connection = %v, want deadline exceeded", err)
	}
	if err := result.connection.Close(); err != nil {
		t.Fatalf("close accepted connection: %v", err)
	}
	drainedCtx, cancelDrained := context.WithTimeout(context.Background(), time.Second)
	defer cancelDrained()
	if err := tracker.wait(drainedCtx); err != nil {
		t.Fatalf("wait after accepted connection closed: %v", err)
	}
}

func TestTrackedTCPConnectionPreservesHalfClose(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	tracker := newConnectionTracker()
	type acceptResult struct {
		connection net.Conn
		err        error
	}
	accepted := make(chan acceptResult, 1)
	go func() {
		connection, acceptErr := tracker.wrap(listener).Accept()
		accepted <- acceptResult{connection: connection, err: acceptErr}
	}()

	client, err := net.Dial("tcp4", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	var result acceptResult
	select {
	case result = <-accepted:
	case <-time.After(time.Second):
		t.Fatal("listener did not accept TCP connection")
	}
	if result.err != nil {
		t.Fatalf("Accept failed: %v", result.err)
	}
	defer result.connection.Close()

	writer, ok := result.connection.(closeWriter)
	if !ok {
		t.Fatal("tracked TCP connection does not preserve CloseWrite")
	}
	reader, ok := result.connection.(closeReader)
	if !ok {
		t.Fatal("tracked TCP connection does not preserve CloseRead")
	}

	const payload = "response before FIN"
	if _, err := io.WriteString(result.connection, payload); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	if err := writer.CloseWrite(); err != nil {
		t.Fatalf("CloseWrite: %v", err)
	}
	if err := client.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set client read deadline: %v", err)
	}
	received, err := io.ReadAll(client)
	if err != nil {
		t.Fatalf("read through TCP FIN: %v", err)
	}
	if string(received) != payload {
		t.Fatalf("received payload = %q, want %q", received, payload)
	}
	if err := reader.CloseRead(); err != nil {
		t.Fatalf("CloseRead: %v", err)
	}

	halfClosedCtx, cancelHalfClosed := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancelHalfClosed()
	if err := tracker.wait(halfClosedCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wait after half-close = %v, want deadline exceeded", err)
	}
	if err := result.connection.Close(); err != nil {
		t.Fatalf("close tracked TCP connection: %v", err)
	}
	drainedCtx, cancelDrained := context.WithTimeout(context.Background(), time.Second)
	defer cancelDrained()
	if err := tracker.wait(drainedCtx); err != nil {
		t.Fatalf("wait after full close: %v", err)
	}
}
