//go:build unit

package tlsfingerprint

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestSOCKS5DialerRespectsContextCancel verifies that DialTLSContext through a
// SOCKS5 proxy honors ctx cancellation/timeout. We start a TCP listener that
// accepts connections but never writes anything (simulating a hung SOCKS5
// server), then dial through it with a short ctx deadline. The call must
// return within a small window after the deadline, not block until the OS
// connect timeout.
func TestSOCKS5DialerRespectsContextCancel(t *testing.T) {
	// Hung SOCKS5 server: accept and then sleep, never sending the SOCKS5
	// method-selection reply. This blocks the SOCKS5 handshake indefinitely
	// from the client's perspective.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			// Hold the connection open until the test ends.
			go func(c net.Conn) {
				defer c.Close()
				<-stop
			}(conn)
		}
	}()
	defer func() {
		close(stop)
		ln.Close()
		wg.Wait()
	}()

	proxyURL := &url.URL{
		Scheme: "socks5",
		Host:   ln.Addr().String(),
	}
	dialer := NewSOCKS5ProxyDialer(&Profile{Name: "test"}, proxyURL)

	const deadline = 200 * time.Millisecond
	const slack = 800 * time.Millisecond // CI jitter cushion

	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()

	start := time.Now()
	conn, err := dialer.DialTLSContext(ctx, "tcp", "example.com:443")
	elapsed := time.Since(start)

	if conn != nil {
		conn.Close()
		t.Fatalf("expected no connection, got one")
	}
	if err == nil {
		t.Fatalf("expected error from canceled ctx, got nil after %v", elapsed)
	}

	if elapsed < deadline {
		t.Fatalf("returned too early (%v < %v); ctx deadline likely ignored", elapsed, deadline)
	}
	if elapsed > deadline+slack {
		t.Fatalf("returned too late (%v > %v); ctx not honored by SOCKS5 dialer", elapsed, deadline+slack)
	}

	// The error chain should reflect a timeout. x/net/proxy converts the
	// ctx deadline into a socket read deadline, so the surfaced error may be
	// "i/o timeout" rather than context.DeadlineExceeded. Accept either: an
	// errors.Is match, a net.Error with Timeout(), or a string containing
	// "timeout"/"deadline"/"context".
	if errors.Is(err, context.DeadlineExceeded) {
		return
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "timeout") &&
		!strings.Contains(msg, "deadline") &&
		!strings.Contains(msg, "context") {
		t.Fatalf("expected timeout/deadline-related error, got: %v", err)
	}
}
