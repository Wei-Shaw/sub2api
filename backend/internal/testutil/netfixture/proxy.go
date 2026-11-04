// Package netfixture provides local DNS and proxy fixtures without external traffic.
package netfixture

import (
	"context"
	"crypto/x509"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// IPv6DNS replaces the resolver for this serial test. No public DNS is queried.
func IPv6DNS(t *testing.T, ips ...string) {
	t.Helper()
	ln, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	old := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "udp", ln.LocalAddr().String())
	}}
	t.Cleanup(func() { net.DefaultResolver = old; _ = ln.Close() })
	go func() {
		b := make([]byte, 4096)
		for {
			n, peer, err := ln.ReadFrom(b)
			if err != nil {
				return
			}
			var q dnsmessage.Message
			if q.Unpack(b[:n]) != nil {
				continue
			}
			q.Response = true
			q.RecursionAvailable = true
			for _, question := range q.Questions {
				if question.Type != dnsmessage.TypeAAAA {
					continue
				}
				for _, raw := range ips {
					var ip [16]byte
					copy(ip[:], net.ParseIP(raw).To16())
					q.Answers = append(q.Answers, dnsmessage.Resource{Header: dnsmessage.ResourceHeader{Name: question.Name, Type: dnsmessage.TypeAAAA, Class: dnsmessage.ClassINET, TTL: 60}, Body: &dnsmessage.AAAAResource{AAAA: ip}})
				}
			}
			response, err := q.Pack()
			if err == nil {
				_, _ = ln.WriteTo(response, peer)
			}
		}
	}()
}

type ProxyRequest struct {
	Target, Authorization string
	AddressType           byte
}

// RecordingProxy rejects the tunnel after recording its destination and auth.
func RecordingProxy(t *testing.T, scheme string) (*url.URL, <-chan ProxyRequest) {
	t.Helper()
	seen := make(chan ProxyRequest, 16)
	if scheme == "http" {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			seen <- ProxyRequest{Target: r.Host, Authorization: r.Header.Get("Proxy-Authorization")}
			w.WriteHeader(http.StatusBadGateway)
		}))
		t.Cleanup(s.Close)
		u, _ := url.Parse(s.URL)
		u.User = url.UserPassword("user", "pass")
		return u, seen
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go serveSOCKS(c, seen, false)
		}
	}()
	return &url.URL{Scheme: scheme, Host: ln.Addr().String(), User: url.UserPassword("user", "pass")}, seen
}
func serveSOCKS(c net.Conn, seen chan<- ProxyRequest, tunnel bool) {
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	h := make([]byte, 2)
	if _, err := io.ReadFull(c, h); err != nil {
		return
	}
	methods := make([]byte, int(h[1]))
	if _, err := io.ReadFull(c, methods); err != nil {
		return
	}
	_, _ = c.Write([]byte{5, 2})
	if _, err := io.ReadFull(c, h); err != nil {
		return
	}
	user := make([]byte, int(h[1]))
	if _, err := io.ReadFull(c, user); err != nil {
		return
	}
	if _, err := io.ReadFull(c, h[:1]); err != nil {
		return
	}
	pass := make([]byte, int(h[0]))
	if _, err := io.ReadFull(c, pass); err != nil {
		return
	}
	_, _ = c.Write([]byte{1, 0})
	request := make([]byte, 4)
	if _, err := io.ReadFull(c, request); err != nil {
		return
	}
	size := 16
	switch request[3] {
	case 1:
		size = 4
	case 3:
		if _, err := io.ReadFull(c, h[:1]); err != nil {
			return
		}
		size = int(h[0])
	case 4:
	default:
		return
	}
	host := make([]byte, size)
	if _, err := io.ReadFull(c, host); err != nil {
		return
	}
	if _, err := io.ReadFull(c, h); err != nil {
		return
	}
	name := string(host)
	if request[3] != 3 {
		name = net.IP(host).String()
	}
	seen <- ProxyRequest{Target: net.JoinHostPort(name, strconv.Itoa(int(binary.BigEndian.Uint16(h)))), Authorization: string(user) + ":" + string(pass), AddressType: request[3]}
	if !tunnel {
		_, _ = c.Write([]byte{5, 5, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	dest, err := net.DialTimeout("tcp", net.JoinHostPort(name, strconv.Itoa(int(binary.BigEndian.Uint16(h)))), 3*time.Second)
	if err != nil {
		return
	}
	defer func() { _ = dest.Close() }()
	_, _ = c.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0})
	_ = c.SetDeadline(time.Time{})
	pipe(c, dest)
}

func AssertIPv6Proxy(t *testing.T, seen <-chan ProxyRequest, port string, socks bool) {
	t.Helper()
	select {
	case got := <-seen:
		if got.Target != net.JoinHostPort("::1", port) {
			t.Fatalf("proxy target %q", got.Target)
		}
		auth := "Basic dXNlcjpwYXNz"
		if socks {
			auth = "user:pass"
			if got.AddressType != 4 {
				t.Fatalf("SOCKS address type %d, want IPv6", got.AddressType)
			}
		}
		if got.Authorization != auth {
			t.Fatalf("proxy auth %q", got.Authorization)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("proxy did not receive the tunnel request")
	}
}

// TunnelProxy forwards actual bytes to the requested address, including ::1.
func TunnelProxy(t *testing.T, scheme string) (*url.URL, <-chan ProxyRequest, *x509.Certificate) {
	t.Helper()
	seen := make(chan ProxyRequest, 32)
	if scheme == "socks5" || scheme == "socks5h" {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = ln.Close() })
		go func() {
			for {
				c, err := ln.Accept()
				if err != nil {
					return
				}
				go serveSOCKS(c, seen, true)
			}
		}()
		return &url.URL{Scheme: scheme, Host: ln.Addr().String(), User: url.UserPassword("user", "pass")}, seen, nil
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- ProxyRequest{Target: r.Host, Authorization: r.Header.Get("Proxy-Authorization")}
		if r.Method != http.MethodConnect {
			w.WriteHeader(400)
			return
		}
		host, _, err := net.SplitHostPort(r.Host)
		ip := net.ParseIP(host)
		if err != nil || ip == nil || !ip.IsLoopback() {
			http.Error(w, "test proxy accepts only loopback destinations", http.StatusBadRequest)
			return
		}
		//nolint:gosec // Test fixture rejects all destinations except literal loopback IPs above.
		dest, err := net.DialTimeout("tcp", r.Host, 3*time.Second)
		if err != nil {
			w.WriteHeader(502)
			return
		}
		defer func() { _ = dest.Close() }()
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Error("HTTP hijacking unavailable")
			return
		}
		conn, _, err := hijacker.Hijack()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_, _ = conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
		pipe(conn, dest)
	})
	var server *httptest.Server
	if scheme == "https" {
		server = httptest.NewTLSServer(handler)
	} else {
		server = httptest.NewServer(handler)
	}
	t.Cleanup(server.Close)
	u, _ := url.Parse(server.URL)
	u.User = url.UserPassword("user", "pass")
	var cert *x509.Certificate
	if scheme == "https" {
		cert = server.Certificate()
	}
	return u, seen, cert
}
func pipe(client, dest net.Conn) {
	done := make(chan struct{})
	go func() { _, _ = io.Copy(dest, client); _ = dest.Close(); close(done) }()
	_, _ = io.Copy(client, dest)
	_ = client.Close()
	<-done
}
