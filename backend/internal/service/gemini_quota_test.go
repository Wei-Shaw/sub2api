//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestAccountIsGeminiThirdPartyAPIKey(t *testing.T) {
	tests := []struct {
		name     string
		account  *Account
		expected bool
	}{
		{"nil_account", nil, false},
		{"empty_base_url_is_official", geminiQuotaAPIKeyAccount(""), false},
		{"official_base_url", geminiQuotaAPIKeyAccount("https://generativelanguage.googleapis.com"), false},
		{"official_base_url_trailing_slash", geminiQuotaAPIKeyAccount("https://generativelanguage.googleapis.com/"), false},
		{"official_host_over_http", geminiQuotaAPIKeyAccount("http://generativelanguage.googleapis.com"), true},
		{"official_host_with_path_prefix", geminiQuotaAPIKeyAccount("https://generativelanguage.googleapis.com/proxy"), true},
		{"relay_base_url", geminiQuotaAPIKeyAccount("https://relay.example.com"), true},
		{"oauth_account", &Account{Platform: PlatformGemini, Type: AccountTypeOAuth, Credentials: map[string]any{"base_url": "https://relay.example.com"}}, false},
		{"vertex_service_account", &Account{Platform: PlatformGemini, Type: AccountTypeServiceAccount}, false},
		{"non_gemini_platform", &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "https://relay.example.com"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, tt.account.IsGeminiThirdPartyAPIKey())
		})
	}
}

func TestGeminiQuotaService_QuotaForAccountSkipsThirdPartyAndVertex(t *testing.T) {
	svc := NewGeminiQuotaService(&config.Config{}, nil)
	ctx := context.Background()

	quota, ok := svc.QuotaForAccount(ctx, geminiQuotaAPIKeyAccount(""))
	require.True(t, ok)
	require.Equal(t, int64(50), quota.ProRPD)

	paid := geminiQuotaAPIKeyAccount("https://generativelanguage.googleapis.com")
	paid.Credentials["tier_id"] = GeminiTierAIStudioPaid
	quota, ok = svc.QuotaForAccount(ctx, paid)
	require.True(t, ok)
	require.Equal(t, int64(-1), quota.ProRPD)

	relay := geminiQuotaAPIKeyAccount("https://relay.example.com")
	relay.Credentials["tier_id"] = GeminiTierAIStudioFree
	_, ok = svc.QuotaForAccount(ctx, relay)
	require.False(t, ok, "第三方上游不套用 AI Studio 模拟配额")

	vertex := &Account{
		Platform:    PlatformGemini,
		Type:        AccountTypeServiceAccount,
		Credentials: map[string]any{"project_id": "my-project", "tier_id": "vertex"},
	}
	_, ok = svc.QuotaForAccount(ctx, vertex)
	require.False(t, ok, "Vertex 按量付费不套用模拟配额")

	googleOne := &Account{
		Platform:    PlatformGemini,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"oauth_type": "google_one", "tier_id": GeminiTierGoogleOneFree},
	}
	quota, ok = svc.QuotaForAccount(ctx, googleOne)
	require.True(t, ok)
	require.Positive(t, quota.SharedRPD)
}

func geminiQuotaAPIKeyAccount(baseURL string) *Account {
	credentials := map[string]any{}
	if baseURL != "" {
		credentials["base_url"] = baseURL
	}
	return &Account{Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: credentials}
}
