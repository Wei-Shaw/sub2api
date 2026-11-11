//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	gocache "github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/require"
)

func TestAccountModelMappingAllowUnlisted(t *testing.T) {
	mapping := map[string]any{"claude-opus-5": "claude-opus-5-5"}

	restrictive := &Account{Platform: PlatformAnthropic, Type: AccountTypeSetupToken, Credentials: map[string]any{
		"model_mapping": mapping,
	}}
	require.False(t, restrictive.ModelMappingAllowsUnlisted())
	require.True(t, restrictive.IsModelSupported("claude-opus-5"))
	require.False(t, restrictive.IsModelSupported("claude-sonnet-5"), "默认语义：非空映射仍是白名单")

	decoupled := &Account{Platform: PlatformAnthropic, Type: AccountTypeSetupToken, Credentials: map[string]any{
		"model_mapping":                        mapping,
		ModelMappingAllowUnlistedCredentialKey: true,
	}}
	require.True(t, decoupled.ModelMappingAllowsUnlisted())
	require.True(t, decoupled.IsModelSupported("claude-opus-5"))
	require.True(t, decoupled.IsModelSupported("claude-sonnet-5"))
	require.Equal(t, "claude-opus-5-5", decoupled.GetMappedModel("claude-opus-5"))
	require.Equal(t, "claude-sonnet-5", decoupled.GetMappedModel("claude-sonnet-5"), "未命中映射的模型原样转发")

	nonBool := &Account{Platform: PlatformAnthropic, Credentials: map[string]any{
		"model_mapping":                        mapping,
		ModelMappingAllowUnlistedCredentialKey: "true",
	}}
	require.False(t, nonBool.ModelMappingAllowsUnlisted(), "只认布尔 true")
}

func TestAccountModelMappingAllowUnlistedKeepsWhitelistEntries(t *testing.T) {
	account := &Account{Platform: PlatformAnthropic, Type: AccountTypeSetupToken, Credentials: map[string]any{
		"model_mapping": map[string]any{
			"claude-opus-5":   "claude-opus-5-5",
			"claude-sonnet-5": "claude-sonnet-5",
		},
		ModelMappingAllowUnlistedCredentialKey: true,
	}}
	require.True(t, account.IsModelSupported("claude-opus-5"), "映射源模型可用")
	require.True(t, account.IsModelSupported("claude-sonnet-5"), "白名单模型可用")
	require.False(t, account.IsModelSupported("claude-haiku-4-5"), "配置了白名单时，白名单之外的模型仍被拒绝")
}

func TestAccountModelMappingAllowUnlistedKeepsPlatformFallbackRules(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{
		"model_mapping":                        map[string]any{"my-alias": "gpt-6-sol"},
		ModelMappingAllowUnlistedCredentialKey: true,
	}}
	require.True(t, account.IsModelSupported("my-alias"), "映射别名不受无映射规则约束")
	require.True(t, account.IsModelSupported("gpt-6-luna"))
	require.False(t, account.IsModelSupported("glm-5.3"), "未命中映射时仍按 OpenAI OAuth 的无映射规则排除外家模型")
}

func TestGetAvailableModels_AllowUnlistedMappingIncludesPlatformDefaults(t *testing.T) {
	repo := &modelsListAccountRepoStub{all: []Account{{
		ID:       1,
		Platform: PlatformAnthropic,
		Credentials: map[string]any{
			"model_mapping":                        map[string]any{"claude-opus-5-alias": "claude-opus-5-5"},
			ModelMappingAllowUnlistedCredentialKey: true,
		},
	}}}
	svc := &GatewayService{
		accountRepo:        repo,
		modelsListCache:    gocache.New(time.Minute, time.Minute),
		modelsListCacheTTL: time.Minute,
	}
	models := svc.GetAvailableModels(context.Background(), nil, PlatformAnthropic)
	require.Contains(t, models, "claude-opus-5-alias")
	for _, id := range claude.DefaultModelIDs() {
		require.Contains(t, models, id)
	}
}
