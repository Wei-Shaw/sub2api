// Package chatgptnetwork scopes IP-family policy to ChatGPT upstreams.
package chatgptnetwork

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyutil"
)

const EnvIPv6Only = "SUB2API_CHATGPT_IPV6_ONLY"

func isChatGPT(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	return host == "chatgpt.com" || strings.HasSuffix(host, ".chatgpt.com")
}

// Apply snapshots startup configuration when constructing a transport. Changing
// the setting requires a restart so existing pooled connections cannot bypass it.
// Proxy tunnels receive an explicit IPv6 destination, while TLS keeps its hostname.
func Apply(t *http.Transport, configuredProxy bool) {
	value := strings.TrimSpace(os.Getenv(EnvIPv6Only))
	if value == "" {
		return
	}
	enabled, parseErr := strconv.ParseBool(value)
	if parseErr == nil && !enabled {
		return
	}
	applyIPv6(t, configuredProxy, parseErr)
}

func applyIPv6(t *http.Transport, _ bool, parseErr error) {
	check := func(host string) error {
		if isChatGPT(host) && parseErr != nil {
			return fmt.Errorf("%s must be a boolean", EnvIPv6Only)
		}
		return nil
	}
	originalProxy := t.Proxy
	originalTLS := t.DialTLSContext
	dial := t.DialContext
	if dial == nil {
		dial = (&net.Dialer{}).DialContext
	}
	// For ChatGPT we establish the tunnel ourselves so CONNECT carries an IPv6
	// literal. The request URL and Host stay unchanged for HTTP routing and TLS.
	t.Proxy = func(r *http.Request) (*url.URL, error) {
		if isChatGPT(r.URL.Hostname()) {
			return nil, check(r.URL.Hostname())
		}
		if originalProxy != nil {
			return originalProxy(r)
		}
		return nil, nil
	}
	t.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		if err := check(host); err != nil {
			return nil, err
		}
		if !isChatGPT(host) {
			return dial(ctx, network, addr)
		}
		if originalProxy != nil {
			// Wrap pins this selector to the actual request's proxy before cloning.
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+addr, nil)
			if err != nil {
				return nil, err
			}
			p, err := originalProxy(req)
			if err != nil {
				return nil, err
			}
			if p != nil {
				header := t.ProxyConnectHeader
				if t.GetProxyConnectHeader != nil {
					header, err = t.GetProxyConnectHeader(ctx, p, addr)
					if err != nil {
						return nil, err
					}
				}
				return (&proxyutil.TunnelDialer{ProxyURL: p, Forward: dial, TLSConfig: t.TLSClientConfig, ConnectHeader: header, OnConnectResponse: t.OnProxyConnectResponse}).DialContext(ctx, "tcp6", addr)
			}
		}
		return dial(ctx, "tcp6", addr)
	}
	if originalProxy != nil {
		t.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			if !isChatGPT(host) && originalTLS != nil {
				return originalTLS(ctx, network, addr)
			}
			conn, err := t.DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			config := &tls.Config{}
			if t.TLSClientConfig != nil {
				config = t.TLSClientConfig.Clone()
			}
			if config.ServerName == "" {
				config.ServerName, _, _ = net.SplitHostPort(addr)
			}
			if t.TLSHandshakeTimeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, t.TLSHandshakeTimeout)
				defer cancel()
			}
			secure := tls.Client(conn, config)
			if err := secure.HandshakeContext(ctx); err != nil {
				_ = conn.Close()
				return nil, err
			}
			return secure, nil
		}
	} else if originalTLS != nil {
		t.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			if err := check(host); err != nil {
				return nil, err
			}
			if isChatGPT(host) {
				network = "tcp6"
			}
			// Fingerprint dialers retain addr for SNI and resolve the tunnel destination.
			return originalTLS(ctx, network, addr)
		}
	}
}
