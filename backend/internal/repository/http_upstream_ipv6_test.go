package repository

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/chatgptnetwork"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/testutil/netfixture"
)

func TestChatGPTIPv6UpstreamTransportCoverage(t *testing.T) {
	t.Setenv(chatgptnetwork.EnvIPv6Only, "true")
	netfixture.IPv6DNS(t, "::1")
	proxy, seen := netfixture.RecordingProxy(t, "http")
	for _, fingerprint := range []bool{false, true} {
		var tr *http.Transport
		var err error
		if fingerprint {
			tr, err = buildUpstreamTransportWithTLSFingerprint(poolSettings{}, proxy, tlsfingerprint.CodexCLIProfile())
		} else {
			tr, err = buildUpstreamTransport(poolSettings{}, proxy, upstreamProtocolModeOpenAIH2)
		}
		if err != nil {
			t.Fatal(err)
		}
		rt := wrapUpstreamNetwork(tr, true)
		closer, ok := rt.(interface{ CloseIdleConnections() })
		if !ok {
			t.Fatal("transport cannot close idle connections")
		}
		defer closer.CloseIdleConnections()
		for _, path := range []string{"/backend-api/codex/responses", "/backend-api/f/conversation", "/backend-api/codex/images/generations", "/backend-api/codex/images/edits"} {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			req, _ := http.NewRequestWithContext(ctx, "POST", "https://chatgpt.com"+path, nil)
			if resp, err := rt.RoundTrip(req); err == nil {
				_ = resp.Body.Close()
				t.Fatalf("proxy rejection ignored fingerprint=%v path=%s", fingerprint, path)
			}
			cancel()
			netfixture.AssertIPv6Proxy(t, seen, "443", false)
		}
	}
}
