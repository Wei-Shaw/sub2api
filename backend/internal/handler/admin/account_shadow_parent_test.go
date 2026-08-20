package admin

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestEnrichShadowParentInfo(t *testing.T) {
	pid := int64(100)
	parent := &service.Account{
		ID:       100,
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeOAuth,
		Credentials: map[string]any{
			"email":                   "owner@example.com",
			"plan_type":               "pro",
			"subscription_expires_at": "2026-12-31T00:00:00Z",
			"chatgpt_account_id":      "acct_123",
			"access_token":            "parent-secret",
			"base_url":                "https://parent.example.test",
		},
		Extra: map[string]any{
			"privacy_mode":                        "training_off",
			"openai_device_id":                    "parent-device",
			"codex_fingerprint_seed":              "parent-seed",
			"codex_fingerprint_mode":              "session",
			"openai_long_context_billing_enabled": true,
		},
	}
	parents := map[int64]*service.Account{100: parent}

	shadow := AccountWithConcurrency{Account: &dto.Account{
		ID:              200,
		Platform:        service.PlatformOpenAI,
		Type:            service.AccountTypeOAuth,
		ParentAccountID: &pid,
		Credentials: map[string]any{
			"base_url":      "https://stale-shadow.example.test",
			"model_mapping": map[string]any{"gpt-5.3-codex-spark": "gpt-5.3-codex-spark"},
		},
		Extra: map[string]any{
			"openai_device_id":                    "shadow-device",
			"codex_fingerprint_seed":              "shadow-seed",
			"codex_fingerprint_mode":              "device",
			"openai_long_context_billing_enabled": false,
			"codex_7d_used_percent":               25.0,
		},
	}}
	normal := AccountWithConcurrency{Account: &dto.Account{ID: 1}}
	orphan := AccountWithConcurrency{Account: &dto.Account{ID: 201, ParentAccountID: ptrInt64(999)}}
	items := []AccountWithConcurrency{shadow, normal, orphan}

	enrichShadowParentInfo(items, parents)

	require.Equal(t, "owner@example.com", items[0].ParentEmail, "影子回填母账号邮箱")
	require.Equal(t, "pro", items[0].ParentPlanType)
	require.Equal(t, "training_off", items[0].ParentPrivacyMode)
	require.Equal(t, "2026-12-31T00:00:00Z", items[0].ParentSubscriptionExpiresAt)
	require.Equal(t, "acct_123", items[0].ParentChatGPTAccountID)
	require.Equal(t, "https://parent.example.test", items[0].Credentials["base_url"])
	require.Equal(t, map[string]any{"gpt-5.3-codex-spark": "gpt-5.3-codex-spark"}, items[0].Credentials["model_mapping"])
	require.NotContains(t, items[0].Credentials, "access_token")
	require.True(t, items[0].CredentialsStatus["has_access_token"])
	require.Equal(t, "parent-device", items[0].Extra["openai_device_id"])
	require.Equal(t, "session", items[0].Extra["codex_fingerprint_mode"])
	require.Equal(t, true, items[0].Extra["openai_long_context_billing_enabled"])
	require.Equal(t, 25.0, items[0].Extra["codex_7d_used_percent"])
	require.NotContains(t, items[0].Extra, "codex_fingerprint_seed")

	require.Empty(t, items[1].ParentEmail, "非影子不回填")
	require.Empty(t, items[2].ParentEmail, "母账号缺失时优雅留空")
}

func ptrInt64(v int64) *int64 { return &v }
