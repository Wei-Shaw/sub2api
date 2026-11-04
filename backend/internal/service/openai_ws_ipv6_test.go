package service

import (
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/chatgptnetwork"
	"github.com/Wei-Shaw/sub2api/internal/testutil/netfixture"
)

func TestOpenAIWSChatGPTIPv6Policy(t *testing.T) {
	t.Setenv(chatgptnetwork.EnvIPv6Only, "true")
	d, ok := newDefaultOpenAIWSClientDialer().(*coderOpenAIWSClientDialer)
	if !ok {
		t.Fatal("unexpected WebSocket dialer type")
	}
	netfixture.IPv6DNS(t, "::1")
	for _, scheme := range []string{"http", "socks5", "socks5h"} {
		proxy, seen := netfixture.RecordingProxy(t, scheme)
		client, err := d.proxyHTTPClient(proxy.String())
		if err != nil {
			t.Fatal(err)
		}
		client.Timeout = 3 * time.Second
		defer client.CloseIdleConnections()
		req, _ := http.NewRequest("GET", "https://chatgpt.com/backend-api/codex/responses", nil)
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			t.Fatal("proxy rejection should fail")
		}
		netfixture.AssertIPv6Proxy(t, seen, "443", scheme != "http")
	}
}
