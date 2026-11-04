package httpclient

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/chatgptnetwork"
	"github.com/Wei-Shaw/sub2api/internal/testutil/netfixture"
)

func TestChatGPTIPv6SharedClientPolicy(t *testing.T) {
	t.Setenv(chatgptnetwork.EnvIPv6Only, "true")
	netfixture.IPv6DNS(t, "::1")
	for _, scheme := range []string{"http", "socks5", "socks5h"} {
		t.Run(scheme, func(t *testing.T) {
			proxy, seen := netfixture.RecordingProxy(t, scheme)
			tr, err := buildTransport(Options{ProxyURL: proxy.String()})
			if err != nil {
				t.Fatal(err)
			}
			client := &http.Client{Transport: chatgptnetwork.Wrap(tr, true), Timeout: 3 * time.Second}
			defer client.CloseIdleConnections()
			req, _ := http.NewRequestWithContext(context.Background(), "GET", "https://chatgpt.com/backend-api/wham/usage", nil)
			if resp, err := client.Do(req); err == nil {
				_ = resp.Body.Close()
				t.Fatal("proxy rejection should fail")
			}
			netfixture.AssertIPv6Proxy(t, seen, "443", scheme != "http")
		})
	}
}
