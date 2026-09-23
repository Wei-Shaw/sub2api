package service

// The optional outer HTTP CONNECT proxy is an operator startup setting. It applies
// only to dynamic harvesting and never changes an account's business proxy.
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
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyurl"
	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyutil"
	"golang.org/x/net/proxy"
)

func newCodexTicketChainedClient(proxyURL, upstreamHTTPProxyURL string) (*http.Client, error) {
	transport := &http.Transport{ForceAttemptHTTP2: false, DisableKeepAlives: true, MaxIdleConns: 0, MaxIdleConnsPerHost: 0, TLSHandshakeTimeout: 15 * time.Second, ResponseHeaderTimeout: 45 * time.Second, ExpectContinueTimeout: 1 * time.Second}
	_, parsed, err := proxyurl.Parse(proxyURL)
	if err != nil {
		return nil, err
	}
	if parsed != nil && upstreamHTTPProxyURL != "" {
		_, upstream, parseErr := proxyurl.Parse(upstreamHTTPProxyURL)
		if parseErr != nil || upstream == nil || (upstream.Scheme != "http" && upstream.Scheme != "https") {
			return nil, errors.New("codex ticket upstream HTTP proxy is invalid")
		}
		baseDialer := &codexTicketHTTPConnectDialer{proxy: upstream, timeout: 15 * time.Second}
		if parsed.Scheme == "http" || parsed.Scheme == "https" {
			transport.Proxy = http.ProxyURL(parsed)
			transport.DialContext = baseDialer.DialContext
		} else {
			auth := (*proxy.Auth)(nil)
			if parsed.User != nil {
				password, _ := parsed.User.Password()
				auth = &proxy.Auth{User: parsed.User.Username(), Password: password}
			}
			socksDialer, socksErr := proxy.SOCKS5("tcp", parsed.Host, auth, baseDialer)
			if socksErr != nil {
				return nil, errors.New("codex ticket SOCKS proxy is invalid")
			}
			if contextDialer, ok := socksDialer.(proxy.ContextDialer); ok {
				transport.DialContext = contextDialer.DialContext
			} else {
				transport.DialContext = func(_ context.Context, network, address string) (net.Conn, error) {
					return socksDialer.Dial(network, address)
				}
			}
		}
	} else if parsed != nil {
		if err := proxyutil.ConfigureTransportProxy(transport, parsed); err != nil {
			return nil, err
		}
	}
	return &http.Client{Transport: transport, Timeout: 60 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}, nil
}

type codexTicketHTTPConnectDialer struct {
	proxy   *url.URL
	timeout time.Duration
}

func (d *codexTicketHTTPConnectDialer) Dial(network, address string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, address)
}

func (d *codexTicketHTTPConnectDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if d == nil || d.proxy == nil {
		return nil, errors.New("codex ticket tunnel is not configured")
	}
	timeout := d.timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	connectCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	proxyAddress := d.proxy.Host
	if d.proxy.Port() == "" {
		port := "80"
		if d.proxy.Scheme == "https" {
			port = "443"
		}
		proxyAddress = net.JoinHostPort(d.proxy.Hostname(), port)
	}
	dialer := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	conn, err := dialer.DialContext(connectCtx, "tcp", proxyAddress)
	if err != nil {
		return nil, err
	}
	closeOnErr := true
	defer func() {
		if closeOnErr {
			_ = conn.Close()
		}
	}()
	rawConn := conn
	stopClose := context.AfterFunc(connectCtx, func() { _ = rawConn.Close() })
	defer stopClose()
	if deadline, ok := connectCtx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	if d.proxy.Scheme == "https" {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: d.proxy.Hostname(), MinVersion: tls.VersionTLS12})
		if err := tlsConn.HandshakeContext(connectCtx); err != nil {
			return nil, err
		}
		conn = tlsConn
	}
	request := &http.Request{Method: http.MethodConnect, URL: &url.URL{Opaque: address}, Host: address, Header: make(http.Header)}
	if d.proxy.User != nil {
		password, _ := d.proxy.User.Password()
		request.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(d.proxy.User.Username()+":"+password)))
	}
	if err := request.Write(conn); err != nil {
		return nil, err
	}
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("codex ticket upstream CONNECT returned HTTP %d", response.StatusCode)
	}
	if err := connectCtx.Err(); err != nil {
		return nil, err
	}
	_ = conn.SetDeadline(time.Time{})
	closeOnErr = false
	return &codexTicketBufferedConn{Conn: conn, reader: reader}, nil
}

type codexTicketBufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *codexTicketBufferedConn) Read(p []byte) (int, error) { return c.reader.Read(p) }
