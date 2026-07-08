//go:build unit

package tlsfingerprint

import (
	"context"
	"errors"
	"net"
	"net/url"
	"testing"
	"time"
)

// 建连(TCP 拨号 + 代理隧道 + TLS 握手)必须有超时包络：上游/代理黑洞时，
// 没有包络的连接会无限挂起，占用账号并发槽位直到客户端断开。
// 包络只作用于 DialTLSContext 内部，返回后的流式读写不受影响。

// TestDialerDialTLSContext_AppliesEstablishDeadline verifies the direct dialer
// wraps the incoming context with a connection-establishment deadline.
func TestDialerDialTLSContext_AppliesEstablishDeadline(t *testing.T) {
	var captured context.Context
	base := func(ctx context.Context, network, addr string) (net.Conn, error) {
		captured = ctx
		return nil, errors.New("dial intercepted")
	}
	d := NewDialer(&Profile{Name: "test"}, base)

	_, err := d.DialTLSContext(context.Background(), "tcp", "example.com:443")
	if err == nil {
		t.Fatal("expected dial error from intercepting base dialer")
	}
	if captured == nil {
		t.Fatal("base dialer was not invoked")
	}
	deadline, ok := captured.Deadline()
	if !ok {
		t.Fatal("建连必须有 deadline 包络（当前 ctx 无 deadline 时补 connectionEstablishTimeout）")
	}
	want := time.Now().Add(connectionEstablishTimeout)
	if deadline.After(want.Add(2*time.Second)) || deadline.Before(want.Add(-2*time.Second)) {
		t.Fatalf("deadline %v 与预期包络 %v 偏差过大", deadline, want)
	}
}

// TestDialerDialTLSContext_KeepsShorterCallerDeadline verifies an existing
// (shorter) caller deadline is preserved rather than extended.
func TestDialerDialTLSContext_KeepsShorterCallerDeadline(t *testing.T) {
	var captured context.Context
	base := func(ctx context.Context, network, addr string) (net.Conn, error) {
		captured = ctx
		return nil, errors.New("dial intercepted")
	}
	d := NewDialer(&Profile{Name: "test"}, base)

	callerDeadline := time.Now().Add(500 * time.Millisecond)
	ctx, cancel := context.WithDeadline(context.Background(), callerDeadline)
	defer cancel()

	_, _ = d.DialTLSContext(ctx, "tcp", "example.com:443")
	deadline, ok := captured.Deadline()
	if !ok {
		t.Fatal("deadline missing")
	}
	if deadline.After(callerDeadline.Add(50 * time.Millisecond)) {
		t.Fatalf("包络不得放宽调用方已有的更短 deadline: got %v want <= %v", deadline, callerDeadline)
	}
}

// blackholeListener accepts connections and reads but never responds,
// simulating a hung proxy.
func blackholeListener(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			// Hold the connection open, drain input, never reply.
			go func(c net.Conn) {
				buf := make([]byte, 1024)
				for {
					if _, err := c.Read(buf); err != nil {
						_ = c.Close()
						return
					}
				}
			}(conn)
		}
	}()
	return ln
}

// TestSOCKS5ProxyDialer_HonorsContextDeadline: a SOCKS5 proxy that accepts the
// TCP connection but never completes the negotiation must not hang past the
// context deadline. (The legacy implementation used socksDialer.Dial, which
// ignores ctx entirely.)
func TestSOCKS5ProxyDialer_HonorsContextDeadline(t *testing.T) {
	ln := blackholeListener(t)
	defer func() { _ = ln.Close() }()

	d := NewSOCKS5ProxyDialer(&Profile{Name: "test"}, &url.URL{Scheme: "socks5", Host: ln.Addr().String()})

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := d.DialTLSContext(ctx, "tcp", "example.com:443")
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error from hung SOCKS5 proxy")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SOCKS5 建连未受 ctx deadline 约束（挂起）")
	}
}

// TestHTTPProxyDialer_BoundsConnectPhase: an HTTP proxy that accepts the TCP
// connection but never answers the CONNECT request must not hang past the
// context deadline. (The legacy implementation read the CONNECT response with
// no deadline at all.)
func TestHTTPProxyDialer_BoundsConnectPhase(t *testing.T) {
	ln := blackholeListener(t)
	defer func() { _ = ln.Close() }()

	d := NewHTTPProxyDialer(&Profile{Name: "test"}, &url.URL{Scheme: "http", Host: ln.Addr().String()})

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := d.DialTLSContext(ctx, "tcp", "example.com:443")
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error from hung HTTP proxy CONNECT")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("HTTP 代理 CONNECT 阶段未受 ctx deadline 约束（挂起）")
	}
}
