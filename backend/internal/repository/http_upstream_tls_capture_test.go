//go:build tls_capture

package repository

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/testutil/tlscapture"
	"github.com/stretchr/testify/require"
)

// Run manually with:
//
//	go test -tags=tls_capture -v ./internal/repository \
//	  -run TestHTTPUpstreamOpenAIDefaultTLSProfileCapture -count=1
//
// The loopback capture server intentionally stops after ClientHello, so both
// HTTP calls return an EOF-like handshake error. The recorded handshake is the
// assertion target; no external service or trusted test certificate is needed.
func TestHTTPUpstreamOpenAIDefaultTLSProfileCapture(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			OpenAIHTTP2: config.GatewayOpenAIHTTP2Config{Enabled: false},
		},
	}
	upstream := NewHTTPUpstream(cfg)

	goHello := captureHTTPUpstreamClientHello(t, func(ctx context.Context, captureURL string) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, captureURL, nil)
		if err != nil {
			return err
		}
		req = req.WithContext(service.WithHTTPUpstreamProfile(req.Context(), service.HTTPUpstreamProfileOpenAI))
		_, err = upstream.Do(req, "", 991, 1)
		return err
	})

	chromeHello := captureHTTPUpstreamClientHello(t, func(ctx context.Context, captureURL string) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, captureURL, nil)
		if err != nil {
			return err
		}
		req = req.WithContext(service.WithHTTPUpstreamProfile(req.Context(), service.HTTPUpstreamProfileOpenAI))
		_, err = upstream.DoWithTLS(req, "", 991, 1, tlsfingerprint.NewOpenAIChrome120Profile())
		return err
	})

	t.Logf("Go baseline cipher suites: %v", goHello.CipherSuites)
	t.Logf("Go baseline extension order: %v", goHello.Extensions)
	t.Logf("OpenAI uTLS cipher suites: %v", chromeHello.CipherSuites)
	t.Logf("OpenAI uTLS extension order: %v", chromeHello.Extensions)

	require.NotEqual(t, goHello.CipherSuites, chromeHello.CipherSuites,
		"DoWithTLS must not emit the Go default cipher-suite sequence")
	require.NotEqual(t, goHello.Extensions, chromeHello.Extensions,
		"DoWithTLS must not emit the Go default extension sequence")
	require.True(t, tlscapture.IsGREASE(chromeHello.CipherSuites[0]),
		"Chrome ClientHello should begin with a GREASE cipher suite")
	require.Contains(t, chromeHello.Extensions, uint16(27),
		"Chrome ClientHello should advertise certificate compression")
	require.Contains(t, chromeHello.Extensions, uint16(65037),
		"Chrome ClientHello should carry GREASE ECH")
}

// Run manually with:
//
//	go test -tags=tls_capture -v ./internal/repository \
//	  -run TestOpenAIOAuthRefreshTLSProfileCapture -count=1
func TestOpenAIOAuthRefreshTLSProfileCapture(t *testing.T) {
	goHello := captureHTTPUpstreamClientHello(t, func(ctx context.Context, captureURL string) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, captureURL, nil)
		if err != nil {
			return err
		}
		_, err = (&http.Client{Transport: &http.Transport{Proxy: nil}}).Do(req)
		return err
	})

	oauthHello := captureHTTPUpstreamClientHello(t, func(ctx context.Context, captureURL string) error {
		client := &openaiOAuthService{tokenURL: captureURL}
		_, err := client.RefreshToken(ctx, "capture-refresh-token", "")
		return err
	})

	t.Logf("Go baseline cipher suites: %v", goHello.CipherSuites)
	t.Logf("Go baseline extension order: %v", goHello.Extensions)
	t.Logf("OAuth refresh cipher suites: %v", oauthHello.CipherSuites)
	t.Logf("OAuth refresh extension order: %v", oauthHello.Extensions)

	require.NotEqual(t, goHello.CipherSuites, oauthHello.CipherSuites,
		"OAuth refresh must not emit the Go default cipher-suite sequence")
	require.NotEqual(t, goHello.Extensions, oauthHello.Extensions,
		"OAuth refresh must not emit the Go default extension sequence")
	require.True(t, tlscapture.IsGREASE(oauthHello.CipherSuites[0]),
		"OAuth refresh Chrome ClientHello should begin with GREASE")
	require.Contains(t, oauthHello.Extensions, uint16(27),
		"OAuth refresh Chrome ClientHello should advertise certificate compression")
}

func captureHTTPUpstreamClientHello(
	t *testing.T,
	send func(context.Context, string) error,
) tlscapture.ClientHello {
	t.Helper()
	return tlscapture.Capture(t, "https", send)
}
