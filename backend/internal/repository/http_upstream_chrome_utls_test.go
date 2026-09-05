package repository

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
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
)

func TestShouldUseOpenAIChromeUTLS(t *testing.T) {
	cfg := &config.Config{Gateway: config.GatewayConfig{
		OpenAIHTTP2: config.GatewayOpenAIHTTP2Config{Enabled: true},
	}}
	cfg.SetOpenAIChromeUTLSEnabled(true)
	svc, ok := NewHTTPUpstream(cfg).(*httpUpstreamService)
	require.True(t, ok)

	tests := []struct {
		name    string
		target  string
		profile service.HTTPUpstreamProfile
		want    bool
	}{
		{name: "chatgpt OpenAI request", target: "https://chatgpt.com/backend-api/codex", profile: service.HTTPUpstreamProfileOpenAI, want: true},
		{name: "non OpenAI profile", target: "https://chatgpt.com/backend-api/codex", profile: service.HTTPUpstreamProfileDefault},
		{name: "different host", target: "https://api.openai.com/v1/responses", profile: service.HTTPUpstreamProfileOpenAI},
		{name: "subdomain", target: "https://api.chatgpt.com/backend-api/codex", profile: service.HTTPUpstreamProfileOpenAI},
		{name: "plain HTTP", target: "http://chatgpt.com/backend-api/codex", profile: service.HTTPUpstreamProfileOpenAI},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, tt.target, nil)
			require.NoError(t, err)
			require.Equal(t, tt.want, svc.shouldUseOpenAIChromeUTLS(req, "", tt.profile))
		})
	}

	req, err := http.NewRequest(http.MethodGet, "https://chatgpt.com/backend-api/codex", nil)
	require.NoError(t, err)
	cfg.SetOpenAIChromeUTLSEnabled(false)
	require.False(t, svc.shouldUseOpenAIChromeUTLS(req, "", service.HTTPUpstreamProfileOpenAI))
	cfg.SetOpenAIChromeUTLSEnabled(true)
	cfg.Gateway.OpenAIHTTP2.Enabled = false
	require.False(t, svc.shouldUseOpenAIChromeUTLS(req, "", service.HTTPUpstreamProfileOpenAI))
}

func TestShouldUseOpenAIChromeUTLSStopsDuringProxyFallback(t *testing.T) {
	cfg := &config.Config{Gateway: config.GatewayConfig{
		OpenAIHTTP2: config.GatewayOpenAIHTTP2Config{
			Enabled:                   true,
			AllowProxyFallbackToHTTP1: true,
			FallbackErrorThreshold:    1,
			FallbackWindowSeconds:     60,
			FallbackTTLSeconds:        600,
		},
	}}
	cfg.SetOpenAIChromeUTLSEnabled(true)
	svc, ok := NewHTTPUpstream(cfg).(*httpUpstreamService)
	require.True(t, ok)
	proxyURL := "http://proxy.local:8080"
	req, err := http.NewRequest(http.MethodGet, "https://chatgpt.com/backend-api/codex", nil)
	require.NoError(t, err)
	require.True(t, svc.shouldUseOpenAIChromeUTLS(req, proxyURL, service.HTTPUpstreamProfileOpenAI))

	svc.recordOpenAIHTTP2Failure(
		service.HTTPUpstreamProfileOpenAI,
		upstreamProtocolModeOpenAIH2,
		proxyURL,
		errors.New(`chrome uTLS ALPN negotiated "http/1.1" instead of h2`),
	)
	require.False(t, svc.shouldUseOpenAIChromeUTLS(req, proxyURL, service.HTTPUpstreamProfileOpenAI))
}

func TestOpenAIChromeUTLSALPNErrorIsProxyFallbackCompatible(t *testing.T) {
	require.True(t, isOpenAIHTTP2CompatibilityError(
		errors.New(`chrome uTLS ALPN negotiated "http/1.1" instead of h2`),
	))
}

func TestOpenAIChromeUTLSUsesSeparateClientPool(t *testing.T) {
	cfg := &config.Config{Gateway: config.GatewayConfig{
		OpenAIHTTP2: config.GatewayOpenAIHTTP2Config{Enabled: true},
	}}
	cfg.SetOpenAIChromeUTLSEnabled(true)
	svc, ok := NewHTTPUpstream(cfg).(*httpUpstreamService)
	require.True(t, ok)

	standard, err := svc.getClientEntry("", 7, 2, service.HTTPUpstreamProfileOpenAI, false, false)
	require.NoError(t, err)
	chrome, err := svc.getOpenAIChromeUTLSClientEntry("", 7, 2, false, false)
	require.NoError(t, err)

	require.NotSame(t, standard, chrome)
	require.IsType(t, &http.Transport{}, standard.client.Transport)
	require.IsType(t, &http2.Transport{}, chrome.client.Transport)
	require.Len(t, svc.clients, 2)
	require.Contains(t, chrome.poolKey, "chrome_utls:true")
}

func TestBuildOpenAIChromeUTLSTransportNegotiatesHTTP2(t *testing.T) {
	srv := newChromeUTLSTestServer(t)
	transport := newChromeUTLSTestTransport(t, srv, nil)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	require.NoError(t, err)
	resp, err := transport.RoundTrip(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, 2, resp.ProtoMajor)
	require.NotNil(t, resp.TLS)
	require.Equal(t, "h2", resp.TLS.NegotiatedProtocol)
}

func TestBuildOpenAIChromeUTLSTransportUsesHTTPConnectProxy(t *testing.T) {
	srv := newChromeUTLSTestServer(t)
	proxyURL, connects := newChromeUTLSTestHTTPProxy(t)
	transport := newChromeUTLSTestTransport(t, srv, proxyURL)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	require.NoError(t, err)
	resp, err := transport.RoundTrip(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, 2, resp.ProtoMajor)
	require.Equal(t, int64(1), connects.Load())
}

func TestResponseHeaderTimeoutRoundTripper(t *testing.T) {
	t.Run("nil base", func(t *testing.T) {
		transport := &responseHeaderTimeoutRoundTripper{timeout: time.Second}
		req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
		require.NoError(t, err)
		_, err = transport.RoundTrip(req)
		require.EqualError(t, err, "response header timeout transport is not configured")
		transport.CloseIdleConnections()
	})

	t.Run("timeout", func(t *testing.T) {
		transport := &responseHeaderTimeoutRoundTripper{
			base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				<-req.Context().Done()
				return nil, req.Context().Err()
			}),
			timeout: 10 * time.Millisecond,
		}
		req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
		require.NoError(t, err)
		_, err = transport.RoundTrip(req)
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})
}

func newChromeUTLSTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

func newChromeUTLSTestTransport(t *testing.T, srv *httptest.Server, proxyURL *url.URL) *http2.Transport {
	t.Helper()
	roundTripper, err := buildOpenAIChromeUTLSTransport(poolSettings{idleConnTimeout: time.Minute}, proxyURL)
	require.NoError(t, err)
	transport, ok := roundTripper.(*http2.Transport)
	require.True(t, ok, "expected *http2.Transport, got %T", roundTripper)
	roots := x509.NewCertPool()
	roots.AddCert(srv.Certificate())
	transport.TLSClientConfig = &tls.Config{RootCAs: roots}
	t.Cleanup(transport.CloseIdleConnections)
	return transport
}

func newChromeUTLSTestHTTPProxy(t *testing.T) (*url.URL, *atomic.Int64) {
	t.Helper()
	connects := &atomic.Int64{}
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodConnect {
			http.Error(w, "CONNECT required", http.StatusMethodNotAllowed)
			return
		}
		upstream, err := net.Dial("tcp", req.Host)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			_ = upstream.Close()
			http.Error(w, "hijacking unavailable", http.StatusInternalServerError)
			return
		}
		client, _, err := hijacker.Hijack()
		if err != nil {
			_ = upstream.Close()
			return
		}
		connects.Add(1)
		_, _ = io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n")
		go func() {
			defer func() { _ = upstream.Close() }()
			defer func() { _ = client.Close() }()
			_, _ = io.Copy(upstream, client)
		}()
		_, _ = io.Copy(client, upstream)
		_ = client.Close()
		_ = upstream.Close()
	}))
	t.Cleanup(proxyServer.Close)
	proxyURL, err := url.Parse(proxyServer.URL)
	require.NoError(t, err)
	return proxyURL, connects
}
