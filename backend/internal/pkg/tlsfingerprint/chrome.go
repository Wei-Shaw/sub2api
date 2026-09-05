package tlsfingerprint

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/proxy"
)

// ChromeDialer establishes a proxy tunnel when needed and performs the target
// TLS handshake with uTLS HelloChrome_Auto. The target-facing connection is
// suitable for golang.org/x/net/http2.Transport.
type ChromeDialer struct {
	proxyURL            *url.URL
	baseDialer          func(ctx context.Context, network, addr string) (net.Conn, error)
	tlsHandshakeTimeout time.Duration
}

// NewChromeDialer creates a Chrome uTLS dialer. proxyURL may be nil for direct
// connections. baseDialer must provide the caller's timeout/keepalive policy.
func NewChromeDialer(proxyURL *url.URL, baseDialer func(context.Context, string, string) (net.Conn, error), tlsHandshakeTimeout time.Duration) *ChromeDialer {
	if baseDialer == nil {
		baseDialer = (&net.Dialer{}).DialContext
	}
	return &ChromeDialer{
		proxyURL:            proxyURL,
		baseDialer:          baseDialer,
		tlsHandshakeTimeout: tlsHandshakeTimeout,
	}
}

// DialTLSContext matches http2.Transport.DialTLSContext.
func (d *ChromeDialer) DialTLSContext(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
	if d == nil || d.baseDialer == nil {
		return nil, fmt.Errorf("chrome utls dialer is not configured")
	}
	if d.tlsHandshakeTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, d.tlsHandshakeTimeout)
		defer cancel()
	}

	conn, err := d.dialTarget(ctx, network, addr)
	if err != nil {
		return nil, err
	}
	return performChromeTLSHandshake(ctx, conn, addr, cfg)
}

func (d *ChromeDialer) dialTarget(ctx context.Context, network, addr string) (net.Conn, error) {
	if d.proxyURL == nil {
		return d.baseDialer(ctx, network, addr)
	}

	switch strings.ToLower(d.proxyURL.Scheme) {
	case "http", "https":
		return d.dialHTTPProxyTunnel(ctx, network, addr)
	case "socks5", "socks5h":
		forward := &chromeContextDialer{dialContext: d.baseDialer}
		dialer, err := proxy.FromURL(d.proxyURL, forward)
		if err != nil {
			return nil, fmt.Errorf("create chrome utls SOCKS5 dialer: %w", err)
		}
		if contextDialer, ok := dialer.(proxy.ContextDialer); ok {
			return contextDialer.DialContext(ctx, network, addr)
		}
		return dialer.Dial(network, addr)
	default:
		return nil, fmt.Errorf("unsupported chrome utls proxy scheme: %s", d.proxyURL.Scheme)
	}
}

func (d *ChromeDialer) dialHTTPProxyTunnel(ctx context.Context, network, targetAddr string) (net.Conn, error) {
	proxyAddr := d.proxyURL.Host
	if d.proxyURL.Port() == "" {
		port := "80"
		if strings.EqualFold(d.proxyURL.Scheme, "https") {
			port = "443"
		}
		proxyAddr = net.JoinHostPort(d.proxyURL.Hostname(), port)
	}

	conn, err := d.baseDialer(ctx, network, proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("connect to chrome utls proxy: %w", err)
	}
	closeOnError := true
	defer func() {
		if closeOnError {
			_ = conn.Close()
		}
	}()

	if strings.EqualFold(d.proxyURL.Scheme, "https") {
		proxyTLS := tls.Client(conn, &tls.Config{
			ServerName: d.proxyURL.Hostname(),
			MinVersion: tls.VersionTLS12,
		})
		if err := proxyTLS.HandshakeContext(ctx); err != nil {
			return nil, fmt.Errorf("TLS handshake with HTTPS proxy: %w", err)
		}
		conn = proxyTLS
	}

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
		defer func() { _ = conn.SetDeadline(time.Time{}) }()
	}

	req := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Opaque: targetAddr},
		Host:   targetAddr,
		Header: make(http.Header),
	}
	if d.proxyURL.User != nil {
		username := d.proxyURL.User.Username()
		password, _ := d.proxyURL.User.Password()
		token := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
		req.Header.Set("Proxy-Authorization", "Basic "+token)
	}
	if err := req.Write(conn); err != nil {
		return nil, fmt.Errorf("write chrome utls proxy CONNECT: %w", err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		return nil, fmt.Errorf("read chrome utls proxy CONNECT response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("chrome utls proxy CONNECT failed: %s", resp.Status)
	}

	closeOnError = false
	return conn, nil
}

type chromeContextDialer struct {
	dialContext func(context.Context, string, string) (net.Conn, error)
}

func (d *chromeContextDialer) Dial(network, addr string) (net.Conn, error) {
	return d.dialContext(context.Background(), network, addr)
}

func (d *chromeContextDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return d.dialContext(ctx, network, addr)
}

func performChromeTLSHandshake(ctx context.Context, conn net.Conn, addr string, cfg *tls.Config) (net.Conn, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if cfg != nil && strings.TrimSpace(cfg.ServerName) != "" {
		host = cfg.ServerName
	}

	uCfg := &utls.Config{
		ServerName: host,
		NextProtos: []string{"h2", "http/1.1"},
	}
	if cfg != nil {
		uCfg.RootCAs = cfg.RootCAs
		uCfg.InsecureSkipVerify = cfg.InsecureSkipVerify
		uCfg.MinVersion = cfg.MinVersion
		uCfg.MaxVersion = cfg.MaxVersion
	}

	uConn := utls.UClient(conn, uCfg, utls.HelloChrome_Auto)
	if err := uConn.HandshakeContext(ctx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("chrome uTLS handshake failed: %w", err)
	}
	if negotiated := uConn.ConnectionState().NegotiatedProtocol; negotiated != "h2" {
		_ = uConn.Close()
		return nil, fmt.Errorf("chrome uTLS ALPN negotiated %q instead of h2", negotiated)
	}
	return &chromeTLSConn{UConn: uConn}, nil
}

// chromeTLSConn exposes a crypto/tls-compatible ConnectionState so net/http
// tracing and response metadata keep working with the uTLS connection.
type chromeTLSConn struct {
	*utls.UConn
}

func (c *chromeTLSConn) ConnectionState() tls.ConnectionState {
	state := c.UConn.ConnectionState()
	return tls.ConnectionState{
		Version:                     state.Version,
		HandshakeComplete:           state.HandshakeComplete,
		DidResume:                   state.DidResume,
		CipherSuite:                 state.CipherSuite,
		NegotiatedProtocol:          state.NegotiatedProtocol,
		ServerName:                  state.ServerName,
		PeerCertificates:            state.PeerCertificates,
		VerifiedChains:              state.VerifiedChains,
		SignedCertificateTimestamps: state.SignedCertificateTimestamps,
		OCSPResponse:                state.OCSPResponse,
		TLSUnique:                   state.TLSUnique,
		ECHAccepted:                 state.ECHAccepted,
	}
}
