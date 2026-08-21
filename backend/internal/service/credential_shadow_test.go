package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// stubCredRepo 是最小化 AccountRepository stub，仅实现 GetByID，供 credential_shadow_test 使用。
// 嵌入接口满足完整方法集；未实现的方法若被调用会 panic，从而快速暴露误调用。
type stubCredRepo struct {
	AccountRepository
	parent *Account
}

func (s *stubCredRepo) GetByID(_ context.Context, _ int64) (*Account, error) {
	return s.parent, nil
}

func newStubCredRepo(parent *Account) AccountRepository {
	return &stubCredRepo{parent: parent}
}

func TestResolveCredentialAccount(t *testing.T) {
	ctx := context.Background()
	pid := int64(100)

	// 普通账号（非影子）→ 返回自身
	parent := &Account{ID: 100, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive}
	repo := newStubCredRepo(parent)
	got, err := resolveCredentialAccount(ctx, repo, parent)
	require.NoError(t, err)
	require.Equal(t, int64(100), got.ID)

	// 影子账号 + 合法 OpenAI OAuth 母账号 → 返回母账号
	shadow := &Account{ID: 200, ParentAccountID: &pid, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	got, err = resolveCredentialAccount(ctx, repo, shadow)
	require.NoError(t, err)
	require.Equal(t, int64(100), got.ID)

	// 影子账号 + 母账号非 OpenAI OAuth（API Key 类型）→ 返回 error
	badRepo := newStubCredRepo(&Account{ID: 100, Platform: PlatformOpenAI, Type: AccountTypeAPIKey})
	_, err = resolveCredentialAccount(ctx, badRepo, shadow)
	require.Error(t, err)
}

func TestInheritOpenAIShadowUpstreamProfile(t *testing.T) {
	parentID := int64(100)
	shadowCredentials := map[string]any{
		"access_token":          "stale-shadow-token",
		"model_mapping":         map[string]any{"gpt-5.3-codex-spark": "gpt-5.3-codex-spark"},
		"compact_model_mapping": map[string]any{"gpt-5.3-codex-spark": "gpt-5.3-codex-spark-compact"},
	}
	shadowExtra := map[string]any{
		openAILongContextBillingEnabledKey: false,
		"openai_device_id":                 "shadow-device",
		codexFingerprintSeedExtraKey:       "shadow-seed",
		"codex_fingerprint_mode":           "device",
		"openai_ws_force_http":             true,
		"codex_7d_used_percent":            25.0,
		UpstreamBillingProbeExtraKey:       map[string]any{"status": "shadow"},
		PlatformOpenAI:                     map[string]any{"codex_image_generation_bridge": false, "shadow_only": true},
	}
	shadow := &Account{
		ID:              200,
		Platform:        PlatformOpenAI,
		Type:            AccountTypeOAuth,
		Credentials:     shadowCredentials,
		Extra:           shadowExtra,
		ParentAccountID: &parentID,
		QuotaDimension:  QuotaDimensionSpark,
		Priority:        7,
		Concurrency:     3,
		Schedulable:     true,
	}
	parentCredentials := map[string]any{
		"access_token":          "parent-token",
		"base_url":              "https://parent.example.test",
		"model_mapping":         map[string]any{"parent": "mapping"},
		"compact_model_mapping": map[string]any{"parent": "compact"},
	}
	parentExtra := map[string]any{
		openAILongContextBillingEnabledKey:          true,
		"openai_device_id":                          "parent-device",
		codexFingerprintSeedExtraKey:                "parent-seed",
		"codex_fingerprint_mode":                    "session",
		"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModePassthrough,
		"openai_passthrough":                        true,
		"parent_only":                               true,
		PlatformOpenAI:                              map[string]any{"codex_image_generation_bridge": true, "parent_only": true},
	}
	parent := &Account{
		ID:          parentID,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Credentials: parentCredentials,
		Extra:       parentExtra,
	}

	effective := InheritOpenAIShadowUpstreamProfile(shadow, parent)

	require.Equal(t, shadow.ID, effective.ID)
	require.Equal(t, shadow.QuotaDimension, effective.QuotaDimension)
	require.Equal(t, shadow.Priority, effective.Priority)
	require.Equal(t, shadow.Concurrency, effective.Concurrency)
	require.Equal(t, "parent-token", effective.Credentials["access_token"])
	require.Equal(t, "https://parent.example.test", effective.Credentials["base_url"])
	require.Equal(t, shadowCredentials["model_mapping"], effective.Credentials["model_mapping"])
	require.Equal(t, shadowCredentials["compact_model_mapping"], effective.Credentials["compact_model_mapping"])
	require.Equal(t, true, effective.Extra[openAILongContextBillingEnabledKey])
	require.Equal(t, "parent-device", effective.Extra["openai_device_id"])
	require.Equal(t, "parent-seed", effective.Extra[codexFingerprintSeedExtraKey])
	require.Equal(t, "session", effective.Extra["codex_fingerprint_mode"])
	require.NotContains(t, effective.Extra, "openai_ws_force_http")
	require.Equal(t, 25.0, effective.Extra["codex_7d_used_percent"])
	require.Equal(t, map[string]any{"status": "shadow"}, effective.Extra[UpstreamBillingProbeExtraKey])
	require.NotContains(t, effective.Extra, "parent_only")
	require.Equal(t, map[string]any{
		"codex_image_generation_bridge": true,
		"shadow_only":                   true,
	}, effective.Extra[PlatformOpenAI])
	require.Equal(t, shadowCredentials, shadow.Credentials)
	require.Equal(t, shadowExtra, shadow.Extra)
	require.Equal(t, parentCredentials, parent.Credentials)
	require.Equal(t, parentExtra, parent.Extra)
}
