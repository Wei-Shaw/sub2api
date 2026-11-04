package proxyutil

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/proxy"
)

// TunnelDialer connects through a proxy. tcp6 constrains the destination, not
// the application-to-proxy connection. The proxy receives an IPv6 literal and
// cannot select an IPv4 destination by resolving the original hostname.
type TunnelDialer struct {
	ProxyURL          *url.URL
	Forward           func(context.Context, string, string) (net.Conn, error)
	TLSConfig         *tls.Config // HTTPS proxy trust; the destination has its own TLS layer.
	ConnectHeader     http.Header
	OnConnectResponse func(context.Context, *url.URL, *http.Request, *http.Response) error
}

func (d *TunnelDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, socks5DialTimeout)
	defer cancel()
	targets := []string{addr}
	if network == "tcp6" {
		var err error
		targets, err = ipv6Targets(ctx, addr)
		if err != nil {
			return nil, err
		}
	}
	var failures []error
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		conn, err := d.dialTarget(ctx, target)
		if err == nil {
			return conn, nil
		}
		failures = append(failures, err)
	}
	return nil, errors.Join(failures...)
}

func ipv6Targets(ctx context.Context, addr string) ([]string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip6", host)
	if err != nil {
		return nil, fmt.Errorf("resolve IPv6 proxy destination %s: %w", host, err)
	}
	var targets []string
	for _, ip := range ips {
		if ip.To4() == nil && ip.To16() != nil {
			targets = append(targets, net.JoinHostPort(ip.String(), port))
		}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("no IPv6 address for proxy destination %s", host)
	}
	return targets, nil
}

type forwardDialer struct {
	dial func(context.Context, string, string) (net.Conn, error)
}

func (d forwardDialer) Dial(network, addr string) (net.Conn, error) {
	return d.dial(context.Background(), network, addr)
}
func (d forwardDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return d.dial(ctx, network, addr)
}

func (d *TunnelDialer) dialTarget(ctx context.Context, target string) (net.Conn, error) {
	forward := d.Forward
	if forward == nil {
		forward = socks5ForwardDialer.DialContext
	}
	p := d.ProxyURL
	switch strings.ToLower(p.Scheme) {
	case "socks5", "socks5h":
		var auth *proxy.Auth
		if p.User != nil {
			password, _ := p.User.Password()
			auth = &proxy.Auth{User: p.User.Username(), Password: password}
		}
		addr := p.Host
		if p.Port() == "" {
			addr = net.JoinHostPort(p.Hostname(), "1080")
		}
		dialer, err := proxy.SOCKS5("tcp", addr, auth, forwardDialer{forward})
		if err != nil {
			return nil, err
		}
		contextDialer, ok := dialer.(proxy.ContextDialer)
		if !ok {
			return nil, fmt.Errorf("SOCKS5 dialer does not support context cancellation")
		}
		return contextDialer.DialContext(ctx, "tcp", target)
	case "http", "https":
	default:
		return nil, fmt.Errorf("unsupported proxy scheme: %s", p.Scheme)
	}
	addr := p.Host
	if p.Port() == "" {
		port := "80"
		if strings.EqualFold(p.Scheme, "https") {
			port = "443"
		}
		addr = net.JoinHostPort(p.Hostname(), port)
	}
	conn, err := forward(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("connect to proxy: %w", err)
	}
	success := false
	defer func() {
		if !success {
			_ = conn.Close()
		}
	}()
	// Context cancellation must interrupt CONNECT reads, not just TCP dialing.
	raw := conn
	stop := context.AfterFunc(ctx, func() { _ = raw.Close() })
	defer stop()
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return nil, err
		}
	}
	if strings.EqualFold(p.Scheme, "https") {
		config := &tls.Config{}
		if d.TLSConfig != nil {
			config = d.TLSConfig.Clone()
		}
		config.ServerName = p.Hostname()
		config.NextProtos = []string{"http/1.1"} // CONNECT is HTTP/1.1, even when the target is H2.
		secure := tls.Client(conn, config)
		if err := secure.HandshakeContext(ctx); err != nil {
			return nil, fmt.Errorf("TLS handshake with HTTPS proxy: %w", err)
		}
		conn = secure
	}
	header := d.ConnectHeader.Clone()
	if header == nil {
		header = make(http.Header)
	}
	if p.User != nil {
		password, _ := p.User.Password()
		header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(p.User.Username()+":"+password)))
	}
	req := &http.Request{Method: http.MethodConnect, URL: &url.URL{Opaque: target}, Host: target, Header: header}
	if err := req.Write(conn); err != nil {
		return nil, fmt.Errorf("write CONNECT request: %w", err)
	}
	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, req)
	if err != nil {
		return nil, fmt.Errorf("read CONNECT response: %w", err)
	}
	if d.OnConnectResponse != nil {
		if err := d.OnConnectResponse(ctx, p, req, resp); err != nil {
			return nil, err
		}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("proxy CONNECT failed: %s", resp.Status)
	}
	// A successful CONNECT body owns the tunnel; do not close it.
	if err := conn.SetDeadline(time.Time{}); err != nil {
		return nil, err
	}
	success = true
	if reader.Buffered() != 0 {
		return &bufferedTunnel{Conn: conn, reader: reader}, nil
	}
	return conn, nil
}

type bufferedTunnel struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedTunnel) Read(p []byte) (int, error) { return c.reader.Read(p) }
