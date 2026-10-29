//go:build unit

package service

import (
	"strings"
	"testing"
)

func TestBuildAnthropicMessagesURL(t *testing.T) {
	const path = "/client/v4/accounts/0123456789abcdef0123456789abcdef/ai"
	for _, tt := range []struct {
		name     string
		baseURL  string
		omitBeta bool
	}{
		{"REST", "https://api.cloudflare.com" + path, true},
		{"trailing slash", "https://api.cloudflare.com" + path + "/", true},
		{"HTTPS port", "https://api.cloudflare.com:443" + path, true},
		{"case insensitive host", "https://API.CLOUDFLARE.COM" + path, true},
		{"anthropic", "https://api.anthropic.com", false},
		{"custom upstream", "https://anthropic.example.com", false},
		{"native gateway", "https://gateway.ai.cloudflare.com/v1/account/gateway/anthropic", false},
		{"lookalike suffix", "https://api.cloudflare.com.example.com" + path, false},
		{"lookalike prefix", "https://not-api.cloudflare.com" + path, false},
		{"host in userinfo", "https://api.cloudflare.com@example.com" + path, false},
		{"userinfo", "https://user@api.cloudflare.com" + path, false},
		{"HTTP", "http://api.cloudflare.com" + path, false},
		{"other port", "https://api.cloudflare.com:8443" + path, false},
		{"root path", "https://api.cloudflare.com", false},
		{"other API version", "https://api.cloudflare.com/client/v5/accounts/account/ai", false},
		{"other account route", "https://api.cloudflare.com/client/v4/accounts/account/ai-gateway", false},
		{"extra path", "https://api.cloudflare.com" + path + "/other", false},
		{"missing account", "https://api.cloudflare.com/client/v4/accounts//ai", false},
		{"dot account", "https://api.cloudflare.com/client/v4/accounts/./ai", false},
		{"parent account", "https://api.cloudflare.com/client/v4/accounts/../ai", false},
		{"encoded separator", "https://api.cloudflare.com/client/v4/accounts/one%2Ftwo/ai", false},
		{"query", "https://api.cloudflare.com" + path + "?custom=true", false},
		{"empty query", "https://api.cloudflare.com" + path + "?", false},
		{"fragment", "https://api.cloudflare.com" + path + "#fragment", false},
		{"malformed URL", "https://%invalid", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			want := strings.TrimRight(tt.baseURL, "/") + "/v1/messages"
			if !tt.omitBeta {
				want += "?beta=true"
			}
			if got := buildAnthropicMessagesURL(tt.baseURL); got != want {
				t.Errorf("buildAnthropicMessagesURL(%q) = %q, want %q", tt.baseURL, got, want)
			}
		})
	}
}
