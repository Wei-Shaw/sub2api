package proxyutil

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/testutil/netfixture"
)

func TestTunnelIPv6OnlyNoIPv4Fallback(t *testing.T) {
	netfixture.IPv6DNS(t, "::1", "::2")
	for _, scheme := range []string{"http", "socks5", "socks5h"} {
		t.Run(scheme, func(t *testing.T) {
			p, seen := netfixture.RecordingProxy(t, scheme)
			dialer := &TunnelDialer{ProxyURL: p}
			if c, err := dialer.DialContext(context.Background(), "tcp6", "chatgpt.com:443"); err == nil {
				_ = c.Close()
				t.Fatal("proxy rejected IPv6; must fail")
			}
			for _, want := range []string{"[::1]:443", "[::2]:443"} {
				select {
				case got := <-seen:
					if got.Target != want {
						t.Fatalf("target %s want %s", got.Target, want)
					}
					if scheme != "http" && got.AddressType != 4 {
						t.Fatal("not a SOCKS IPv6 target")
					}
				case <-time.After(time.Second):
					t.Fatal("did not try all IPv6 addresses")
				}
			}
			select {
			case extra := <-seen:
				t.Fatalf("unexpected fallback %v", extra)
			default:
			}
		})
	}
	var called atomic.Bool
	dialer := &TunnelDialer{ProxyURL: &url.URL{Scheme: "http", Host: "proxy.test:80"}, Forward: func(context.Context, string, string) (net.Conn, error) {
		called.Store(true)
		return nil, errors.New("not allowed")
	}}
	if c, err := dialer.DialContext(context.Background(), "tcp6", "127.0.0.1:443"); err == nil {
		_ = c.Close()
		t.Fatal("accepted IPv4 literal")
	}
	if called.Load() {
		t.Fatal("contacted proxy for IPv4 destination")
	}
}

func TestHTTPTunnelCancellation(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	connected := make(chan struct{})
	closed := make(chan struct{})
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = c.Close() }()
		close(connected)
		_, _ = io.Copy(io.Discard, c)
		close(closed)
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		c, err := (&TunnelDialer{ProxyURL: &url.URL{Scheme: "http", Host: ln.Addr().String()}}).DialContext(ctx, "tcp6", "[::1]:443")
		if c != nil {
			_ = c.Close()
		}
		result <- err
	}()
	select {
	case <-connected:
	case <-time.After(time.Second):
		t.Fatal("did not connect")
	}
	cancel()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("cancellation did not fail")
		}
	case <-time.After(time.Second):
		t.Fatal("CONNECT read ignored cancellation")
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("cancelled tunnel left open")
	}
}

func TestSOCKSTunnelCancellation(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = c.Close() }()
		_, _ = io.Copy(io.Discard, c)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = (&TunnelDialer{ProxyURL: &url.URL{Scheme: "socks5h", Host: ln.Addr().String()}}).DialContext(ctx, "tcp6", "[::1]:443")
	if err == nil {
		t.Fatal("SOCKS negotiation ignored deadline")
	}
}

func TestTunnelPreservesBufferedBytesAndConnectHooks(t *testing.T) {
	var hook atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Test") != "connect" {
			t.Error("lost CONNECT headers")
		}
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Error("HTTP hijacking unavailable")
			return
		}
		c, _, err := hijacker.Hijack()
		if err != nil {
			return
		}
		defer func() { _ = c.Close() }()
		_, _ = io.WriteString(c, "HTTP/1.1 200 Connection Established\r\n\r\nhello")
	}))
	defer server.Close()
	p, _ := url.Parse(server.URL)
	d := &TunnelDialer{ProxyURL: p, ConnectHeader: http.Header{"X-Test": []string{"connect"}}, OnConnectResponse: func(_ context.Context, got *url.URL, r *http.Request, resp *http.Response) error {
		hook.Store(got == p && r.Host == "[::1]:443" && resp.StatusCode == 200)
		return nil
	}}
	c, err := d.DialContext(context.Background(), "tcp6", "[::1]:443")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	body, err := io.ReadAll(c)
	if err != nil || strings.TrimSpace(string(body)) != "hello" {
		t.Fatalf("tunnel bytes %q: %v", body, err)
	}
	if !hook.Load() {
		t.Fatal("CONNECT hook not called")
	}
}

// A dial timeout constrains establishment only. Once a tunnel is returned,
// cancelling that context must not terminate a long-lived SSE/WS connection.
func TestTunnelOutlivesDialContext(t *testing.T) {
	listener, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skip(err)
	}
	defer func() { _ = listener.Close() }()
	go func() {
		for {
			c, err := listener.Accept()
			if err != nil {
				return
			}
			go func() { defer func() { _ = c.Close() }(); _, _ = io.Copy(c, c) }()
		}
	}()
	for _, scheme := range []string{"http", "https", "socks5", "socks5h"} {
		t.Run(scheme, func(t *testing.T) {
			p, _, cert := netfixture.TunnelProxy(t, scheme)
			dialer := &TunnelDialer{ProxyURL: p}
			if cert != nil {
				roots := x509.NewCertPool()
				roots.AddCert(cert)
				dialer.TLSConfig = &tls.Config{RootCAs: roots}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			conn, err := dialer.DialContext(ctx, "tcp6", listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = conn.Close() }()
			cancel()
			if err := conn.SetDeadline(time.Now().Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			if _, err := conn.Write([]byte("alive")); err != nil {
				t.Fatal(err)
			}
			got := make([]byte, 5)
			if _, err := io.ReadFull(conn, got); err != nil || string(got) != "alive" {
				t.Fatalf("returned tunnel was closed: %q %v", got, err)
			}
		})
	}
}
