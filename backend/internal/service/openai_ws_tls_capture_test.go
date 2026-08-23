//go:build tls_capture

package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/testutil/tlscapture"
	"github.com/stretchr/testify/require"
)

// Run manually with:
//
//	go test -tags='unit,tls_capture' -v ./internal/service \
//	  -run TestOpenAIWSHandshakeTLSProfileCapture -count=1
//
// The loopback server reads only ClientHello and then closes, so both dials
// fail by design. No external endpoint or trusted certificate is required.
func TestOpenAIWSHandshakeTLSProfileCapture(t *testing.T) {
	dialer := newDefaultOpenAIWSClientDialer()
	tlsDialer, ok := dialer.(openAIWSTLSClientDialer)
	require.True(t, ok, "default OpenAI WS dialer must support TLS profiles")

	goHello := tlscapture.Capture(t, "https", func(ctx context.Context, captureURL string) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, captureURL, nil)
		if err != nil {
			return err
		}
		_, err = (&http.Client{Transport: &http.Transport{Proxy: nil}}).Do(req)
		return err
	})

	wsHello := tlscapture.Capture(t, "wss", func(ctx context.Context, captureURL string) error {
		_, _, _, err := tlsDialer.DialWithTLS(
			ctx,
			captureURL,
			http.Header{"Originator": []string{"codex_cli_rs"}},
			"",
			tlsfingerprint.NewOpenAIChrome120Profile(),
		)
		return err
	})

	t.Logf("Go baseline cipher suites: %v", goHello.CipherSuites)
	t.Logf("Go baseline extension order: %v", goHello.Extensions)
	t.Logf("OpenAI WS cipher suites: %v", wsHello.CipherSuites)
	t.Logf("OpenAI WS extension order: %v", wsHello.Extensions)

	require.NotEqual(t, goHello.CipherSuites, wsHello.CipherSuites,
		"OpenAI WS handshake must not emit the Go default cipher-suite sequence")
	require.NotEqual(t, goHello.Extensions, wsHello.Extensions,
		"OpenAI WS handshake must not emit the Go default extension sequence")
	require.True(t, tlscapture.IsGREASE(wsHello.CipherSuites[0]),
		"OpenAI WS Chrome ClientHello should begin with GREASE")
	require.Contains(t, wsHello.Extensions, uint16(27),
		"OpenAI WS Chrome ClientHello should advertise certificate compression")
}
