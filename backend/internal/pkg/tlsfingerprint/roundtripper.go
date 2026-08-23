package tlsfingerprint

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
)

// UtlsDialFunc dials a uTLS-fingerprinted TLS connection to addr.
// Dialer.DialTLSContext, HTTPProxyDialer.DialTLSContext and
// SOCKS5ProxyDialer.DialTLSContext all satisfy this signature.
type UtlsDialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// TLSRoundTripper is an http.RoundTripper that performs uTLS-fingerprinted
// handshakes and routes traffic over HTTP/2 or HTTP/1.1 based on the ALPN
// protocol negotiated during the handshake.
//
// Background: net/http.Transport cannot upgrade a custom DialTLSContext
// connection to HTTP/2, because its H2 hook type-asserts the connection to
// *tls.Conn and *utls.UConn fails that assertion. The negotiated ALPN is never
// read and the connection silently degrades to HTTP/1.1 even when "h2" was
// negotiated (refraction-networking/utls#16, golang/go#41236).
// TLSRoundTripper works around this by performing a bootstrap uTLS dial on the
// first HTTPS request, inspecting the negotiated ALPN, and then delegating to
// http2.Transport (h2) or http.Transport (h1). The bootstrap connection is
// handed to the winning transport so the first request does not pay a second
// handshake.
//
// The negotiated protocol is decided once per RoundTripper instance. Cached
// upstream clients are keyed per account/proxy/profile and effectively serve a
// single upstream host, so a per-instance decision matches that usage.
//
// Known trade-off: on the h2 path the HTTP/2 frames (SETTINGS, window sizes,
// header order) are Go's x/net/http2 defaults, which do not match the browser
// implied by the ClientHello fingerprint. Frame-level browser alignment is only
// provided by the built-in Chrome preset path (req ImpersonateChrome stack);
// custom profiles trade frame-level consistency for protocol correctness.
type TLSRoundTripper struct {
	dial UtlsDialFunc

	h1 *http.Transport  // h1 path; caller may pre-configure pool settings
	h2 *http2.Transport // h2 path; caller may pre-configure keepalive settings

	mu        sync.Mutex
	decided   http.RoundTripper // non-nil once ALPN has been sniffed
	bootstrap net.Conn          // sniffed connection, consumed by the winning transport
}

// NewTLSRoundTripper creates a RoundTripper that dials all TLS connections via
// dial. h1 and h2 may be nil, in which case defaults are used; when provided,
// only their dial hooks (and the h1 HTTP/2 kill switch) are overwritten, pool
// and timeout settings are preserved.
func NewTLSRoundTripper(dial UtlsDialFunc, h1 *http.Transport, h2 *http2.Transport) *TLSRoundTripper {
	if h1 == nil {
		h1 = &http.Transport{}
	}
	if h2 == nil {
		h2 = &http2.Transport{}
	}
	rt := &TLSRoundTripper{dial: dial, h1: h1, h2: h2}

	// The h1 transport must never attempt HTTP/2 on its own: its uTLS
	// connections fail net/http's *tls.Conn assertion, so the built-in upgrade
	// path cannot work. h2 dispatch is handled exclusively by this RoundTripper.
	h1.DialTLSContext = rt.dialH1
	h1.ForceAttemptHTTP2 = false
	h1.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}

	h2.DialTLSContext = rt.dialH2
	h2.AllowHTTP = false
	return rt
}

// RoundTrip implements http.RoundTripper. The first HTTPS request performs the
// bootstrap dial and ALPN sniff; subsequent requests go directly to the
// selected inner transport.
func (rt *TLSRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL == nil || !strings.EqualFold(req.URL.Scheme, "https") {
		// No TLS handshake to fingerprint or sniff; plain HTTP goes through the
		// h1 transport's default dial.
		return rt.h1.RoundTrip(req)
	}

	rt.mu.Lock()
	if rt.decided == nil {
		if err := rt.sniffLocked(req); err != nil {
			rt.mu.Unlock()
			return nil, err
		}
	}
	transport := rt.decided
	rt.mu.Unlock()

	return transport.RoundTrip(req)
}

// sniffLocked dials a bootstrap connection and selects the inner transport
// based on the negotiated ALPN protocol. rt.mu must be held.
func (rt *TLSRoundTripper) sniffLocked(req *http.Request) error {
	conn, err := rt.dial(req.Context(), "tcp", canonicalTLSAddr(req.URL))
	if err != nil {
		return fmt.Errorf("bootstrap TLS dial: %w", err)
	}

	proto := ""
	if state, ok := conn.(interface{ ConnectionState() utls.ConnectionState }); ok {
		proto = state.ConnectionState().NegotiatedProtocol
	}
	rt.bootstrap = conn
	if proto == "h2" {
		rt.decided = rt.h2
	} else {
		rt.decided = rt.h1
	}
	return nil
}

// dialH1 is the DialTLSContext installed on the h1 transport.
func (rt *TLSRoundTripper) dialH1(ctx context.Context, network, addr string) (net.Conn, error) {
	if conn := rt.takeBootstrap(); conn != nil {
		return conn, nil
	}
	return rt.dial(ctx, network, addr)
}

// dialH2 is the DialTLSContext installed on the h2 transport.
func (rt *TLSRoundTripper) dialH2(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
	if conn := rt.takeBootstrap(); conn != nil {
		return conn, nil
	}
	return rt.dial(ctx, network, addr)
}

// takeBootstrap returns the sniffed bootstrap connection exactly once.
//
// Concurrency note: RoundTrip releases rt.mu before delegating to the inner
// transport, so a concurrent request B can win the bootstrap connection that
// request A sniffed. This is intentional and safe: a cached upstream client
// serves a single upstream host, so every dial — including the bootstrap one —
// targets the same address; whoever receives the connection may use it.
func (rt *TLSRoundTripper) takeBootstrap() net.Conn {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	conn := rt.bootstrap
	rt.bootstrap = nil
	return conn
}

// CloseIdleConnections closes idle connections on both inner transports so
// client-cache eviction semantics match a plain *http.Transport.
func (rt *TLSRoundTripper) CloseIdleConnections() {
	rt.h1.CloseIdleConnections()
	rt.h2.CloseIdleConnections()
}

// HTTP1Transport returns the inner HTTP/1.1 transport. It exists for tests
// and introspection only (e.g. asserting pool/timeout settings); production
// code should interact with TLSRoundTripper itself.
func (rt *TLSRoundTripper) HTTP1Transport() *http.Transport {
	return rt.h1
}

// canonicalTLSAddr returns host:port for the TLS dial, defaulting to 443.
func canonicalTLSAddr(u *url.URL) string {
	port := u.Port()
	if port == "" {
		port = "443"
	}
	return net.JoinHostPort(u.Hostname(), port)
}
