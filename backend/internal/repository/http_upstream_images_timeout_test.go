package repository

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpenAIImagesHeaderTimeoutIsolatedFromText(t *testing.T) {
	for _, fingerprint := range []bool{false, true} {
		name := "standard"
		if fingerprint {
			name = "tls_fingerprint"
		}
		t.Run(name, func(t *testing.T) {
			cfg := &config.Config{Gateway: config.GatewayConfig{
				OpenAIResponseHeaderTimeout: 60,
				OpenAIHTTP2:                 config.GatewayOpenAIHTTP2Config{Enabled: true},
			}}
			svc, ok := NewHTTPUpstream(cfg).(*httpUpstreamService)
			require.True(t, ok)
			get := func(profile service.HTTPUpstreamProfile) *upstreamClientEntry {
				t.Helper()
				var entry *upstreamClientEntry
				var err error
				if fingerprint {
					entry, err = svc.getClientEntryWithTLS("", 75, 4, &tlsfingerprint.Profile{Name: "test"}, profile, false, false)
				} else {
					entry, err = svc.getClientEntry("", 75, 4, profile, false, false)
				}
				require.NoError(t, err)
				return entry
			}
			textEntry := get(service.HTTPUpstreamProfileOpenAI)
			require.Same(t, textEntry, get(service.HTTPUpstreamProfileOpenAIImages), "without an override, images must retain the existing pool and timeout")
			imageSeconds := 600
			cfg.Gateway.ImageResponseHeaderTimeout = &imageSeconds
			imageEntry := get(service.HTTPUpstreamProfileOpenAIImages)
			require.NotSame(t, textEntry, imageEntry)
			require.Equal(t, 60*time.Second, imageTimeoutTestTransport(t, textEntry).ResponseHeaderTimeout)
			require.Equal(t, 600*time.Second, imageTimeoutTestTransport(t, imageEntry).ResponseHeaderTimeout)
			require.Same(t, textEntry, get(service.HTTPUpstreamProfileOpenAI), "image requests must not evict the text connection pool")
			require.Same(t, imageEntry, get(service.HTTPUpstreamProfileOpenAIImages))
			if !fingerprint {
				require.Equal(t, upstreamProtocolModeOpenAIH2, imageEntry.protocolMode)
				require.True(t, imageTimeoutTestTransport(t, imageEntry).ForceAttemptHTTP2)
			}
			*cfg.Gateway.ImageResponseHeaderTimeout = 300
			updated := get(service.HTTPUpstreamProfileOpenAIImages)
			require.NotSame(t, imageEntry, updated)
			require.Equal(t, 300*time.Second, imageTimeoutTestTransport(t, updated).ResponseHeaderTimeout)
			require.Same(t, textEntry, get(service.HTTPUpstreamProfileOpenAI))
			*cfg.Gateway.ImageResponseHeaderTimeout = 0
			require.Zero(t, imageTimeoutTestTransport(t, get(service.HTTPUpstreamProfileOpenAIImages)).ResponseHeaderTimeout)
			cfg.Gateway.ImageResponseHeaderTimeout = nil
			require.Same(t, textEntry, get(service.HTTPUpstreamProfileOpenAIImages))
		})
	}
}

// Use the real HTTP transport with a short text limit: the delayed image must
// complete while the same account's text request still times out.
func TestOpenAIImagesHeaderTimeoutAllowsSlowHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(1500 * time.Millisecond):
			_, _ = io.WriteString(w, `{"data":[{"b64_json":"aGVsbG8="}]}`)
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(server.Close)
	imageSeconds := 3
	svc := NewHTTPUpstream(&config.Config{Gateway: config.GatewayConfig{
		OpenAIResponseHeaderTimeout: 1,
		ImageResponseHeaderTimeout:  &imageSeconds,
	}})
	for _, profile := range []service.HTTPUpstreamProfile{service.HTTPUpstreamProfileOpenAIImages, service.HTTPUpstreamProfileOpenAI} {
		t.Run(string(profile), func(t *testing.T) {
			req, err := http.NewRequestWithContext(service.WithHTTPUpstreamProfile(t.Context(), profile), http.MethodPost, server.URL, nil)
			require.NoError(t, err)
			resp, err := svc.Do(req, "", 75, 4)
			if profile == service.HTTPUpstreamProfileOpenAI {
				require.Error(t, err)
				require.True(t, isUpstreamTimeoutError(err))
				return
			}
			require.NoError(t, err)
			defer func() { require.NoError(t, resp.Body.Close()) }()
			require.Equal(t, http.StatusOK, resp.StatusCode)
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Contains(t, string(body), "b64_json")
		})
	}
}

func TestOpenAIImagesHTTP2ProxyFallback(t *testing.T) {
	imageSeconds := 600
	svc, ok := NewHTTPUpstream(&config.Config{Gateway: config.GatewayConfig{
		ImageResponseHeaderTimeout: &imageSeconds,
		OpenAIHTTP2: config.GatewayOpenAIHTTP2Config{
			Enabled: true, AllowProxyFallbackToHTTP1: true,
			FallbackErrorThreshold: 1, FallbackWindowSeconds: 60, FallbackTTLSeconds: 600,
		},
	}}).(*httpUpstreamService)
	require.True(t, ok)
	proxy := "http://proxy.local:8080"
	svc.recordOpenAIHTTP2Failure(service.HTTPUpstreamProfileOpenAIImages, upstreamProtocolModeOpenAIH2, proxy, errors.New("http2: timeout awaiting response headers"))
	require.False(t, svc.isOpenAIHTTP2FallbackActive(proxy))
	svc.recordOpenAIHTTP2Failure(service.HTTPUpstreamProfileOpenAIImages, upstreamProtocolModeOpenAIH2, proxy, errors.New("http2: protocol error"))
	require.True(t, svc.isOpenAIHTTP2FallbackActive(proxy))
	entry, err := svc.getClientEntry(proxy, 75, 4, service.HTTPUpstreamProfileOpenAIImages, false, false)
	require.NoError(t, err)
	require.Equal(t, upstreamProtocolModeOpenAIH1Fallback, entry.protocolMode)
	require.Equal(t, 600*time.Second, imageTimeoutTestTransport(t, entry).ResponseHeaderTimeout)
}

func imageTimeoutTestTransport(t *testing.T, entry *upstreamClientEntry) *http.Transport {
	t.Helper()
	transport, ok := entry.client.Transport.(*http.Transport)
	require.True(t, ok, "expected *http.Transport")
	return transport
}
