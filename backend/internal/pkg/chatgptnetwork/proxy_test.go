package chatgptnetwork

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyutil"
	"github.com/Wei-Shaw/sub2api/internal/testutil/netfixture"
	"github.com/coder/websocket"
	"golang.org/x/net/http2"
)

func chatGPTCertificate(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), DNSNames: []string{"chatgpt.com"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(parsed)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, roots
}

func TestIPv6ProxyTLSHTTP2AndWebSocket(t *testing.T) {
	t.Setenv(EnvIPv6Only, "true")
	netfixture.IPv6DNS(t, "::1")
	certificate, roots := chatGPTCertificate(t)
	listener, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skip(err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS.ServerName != "chatgpt.com" {
			t.Errorf("SNI = %q", r.TLS.ServerName)
		}
		if !strings.HasPrefix(r.Host, "chatgpt.com:") {
			t.Errorf("Host = %q", r.Host)
		}
		if r.URL.Path == "/ws" {
			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				t.Error(err)
				return
			}
			defer func() { _ = conn.CloseNow() }()
			typ, data, err := conn.Read(r.Context())
			if err != nil {
				t.Error(err)
				return
			}
			_ = conn.Write(r.Context(), typ, data)
			return
		}
		_, _ = fmt.Fprint(w, r.Proto)
	}))
	_ = server.Listener.Close()
	server.Listener = listener
	server.EnableHTTP2 = true
	server.TLS = &tls.Config{Certificates: []tls.Certificate{certificate}}
	server.StartTLS()
	defer server.Close()
	port := strings.Split(listener.Addr().String(), "]:")[1]
	for _, scheme := range []string{"http", "https", "socks5", "socks5h"} {
		t.Run(scheme, func(t *testing.T) {
			p, seen, proxyCert := netfixture.TunnelProxy(t, scheme)
			trust := roots.Clone()
			if proxyCert != nil {
				trust.AddCert(proxyCert)
			}
			for _, h2 := range []bool{false, true} {
				tr := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: trust}, TLSHandshakeTimeout: time.Second, ForceAttemptHTTP2: h2}
				if scheme == "http" || scheme == "https" {
					tr.Proxy = http.ProxyURL(p)
				} else {
					// Exercise the same SOCKS transport configuration used by production.
					if err := proxyutil.ConfigureTransportProxy(tr, p); err != nil {
						t.Fatal(err)
					}
				}
				client := &http.Client{Transport: Wrap(tr, true, func(clone *http.Transport) error {
					if !clone.ForceAttemptHTTP2 {
						return nil
					}
					clone.TLSNextProto = nil
					//nolint:staticcheck // Exercise the existing upstream HTTP/2 clone configuration.
					_, err := http2.ConfigureTransports(clone)
					return err
				}), Timeout: 3 * time.Second}
				resp, err := client.Get("https://chatgpt.com:" + port + "/response")
				if err != nil {
					t.Fatal(err)
				}
				body, _ := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				want := "HTTP/1.1"
				if h2 {
					want = "HTTP/2.0"
				}
				if string(body) != want {
					t.Fatalf("protocol %q want %q", body, want)
				}
				netfixture.AssertIPv6Proxy(t, seen, port, strings.HasPrefix(scheme, "socks"))
				if !h2 {
					ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
					conn, _, err := websocket.Dial(ctx, "wss://chatgpt.com:"+port+"/ws", &websocket.DialOptions{HTTPClient: client})
					if err != nil {
						cancel()
						t.Fatal(err)
					}
					if err := conn.Write(ctx, websocket.MessageText, []byte("ok")); err != nil {
						t.Fatal(err)
					}
					_, data, err := conn.Read(ctx)
					if err != nil || string(data) != "ok" {
						t.Fatalf("WS echo %q: %v", data, err)
					}
					_ = conn.CloseNow()
					cancel()
					netfixture.AssertIPv6Proxy(t, seen, port, strings.HasPrefix(scheme, "socks"))
				}
				client.CloseIdleConnections()
			}
		})
	}
}

func TestIPv6ProxyNoDNSAddressAndNoFallback(t *testing.T) {
	t.Setenv(EnvIPv6Only, "true")
	netfixture.IPv6DNS(t)
	var calls atomic.Int32
	p, _ := url.Parse("http://127.0.0.1:1234")
	tr := &http.Transport{Proxy: http.ProxyURL(p), DialContext: func(context.Context, string, string) (net.Conn, error) {
		calls.Add(1)
		return nil, errors.New("must not dial")
	}}
	client := &http.Client{Transport: Wrap(tr, true), Timeout: time.Second}
	defer client.CloseIdleConnections()
	if resp, err := client.Get("https://chatgpt.com"); err == nil {
		_ = resp.Body.Close()
		t.Fatal("missing AAAA must fail")
	}
	if calls.Load() != 0 {
		t.Fatal("dialed without an IPv6 destination")
	}
}

func TestIPv6ProxySelectionAndPools(t *testing.T) {
	t.Setenv(EnvIPv6Only, "true")
	netfixture.IPv6DNS(t, "::1")
	first, seen1 := netfixture.RecordingProxy(t, "http")
	second, seen2 := netfixture.RecordingProxy(t, "http")
	var selections atomic.Int32
	tr := &http.Transport{Proxy: func(r *http.Request) (*url.URL, error) {
		selections.Add(1)
		if r.Method != "POST" {
			return nil, errors.New("lost actual request")
		}
		if r.Header.Get("X-Proxy") == "second" {
			return second, nil
		}
		return first, nil
	}}
	rt, ok := Wrap(tr, false).(*transport)
	if !ok {
		t.Fatal("unexpected transport type")
	}
	defer rt.CloseIdleConnections()
	for _, name := range []string{"first", "second", "first"} {
		req, _ := http.NewRequest("POST", "https://chatgpt.com/test", nil)
		req.Header.Set("X-Proxy", name)
		if resp, err := rt.RoundTrip(req); err == nil {
			_ = resp.Body.Close()
			t.Fatal("proxy rejection ignored")
		}
		seen := seen1
		if name == "second" {
			seen = seen2
		}
		netfixture.AssertIPv6Proxy(t, seen, "443", false)
	}
	if selections.Load() != 3 {
		t.Fatalf("proxy selected %d times", selections.Load())
	}
	if len(rt.ipv6) != 2 {
		t.Fatal("proxy pools not isolated")
	}
}

func TestIPv6ProxyStillVerifiesTargetCertificate(t *testing.T) {
	t.Setenv(EnvIPv6Only, "true")
	netfixture.IPv6DNS(t, "::1")
	certificate, _ := chatGPTCertificate(t)
	listener, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skip(err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("untrusted target accepted") }))
	_ = server.Listener.Close()
	server.Listener = listener
	server.TLS = &tls.Config{Certificates: []tls.Certificate{certificate}}
	server.StartTLS()
	defer server.Close()
	p, seen, _ := netfixture.TunnelProxy(t, "http")
	tr := &http.Transport{Proxy: http.ProxyURL(p), TLSClientConfig: &tls.Config{RootCAs: x509.NewCertPool()}, TLSHandshakeTimeout: time.Second}
	client := &http.Client{Transport: Wrap(tr, true), Timeout: 3 * time.Second}
	defer client.CloseIdleConnections()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Get("https://chatgpt.com:" + port)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("accepted untrusted certificate")
	}
	var unknown x509.UnknownAuthorityError
	if !errors.As(err, &unknown) {
		t.Fatalf("expected certificate verification failure, got %v", err)
	}
	netfixture.AssertIPv6Proxy(t, seen, port, false)
}
