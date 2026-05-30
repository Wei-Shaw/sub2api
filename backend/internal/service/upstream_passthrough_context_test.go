package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestUpstreamPassthroughPolicyRuntimeDisabled_NoPolicyAttached(t *testing.T) {
	resetUpstreamPassthroughDefaultsCache()
	resetUpstreamPassthroughGlobalOverrideCache()

	repo := &fakeSettingRepoForUpstream{values: map[string]string{
		SettingKeyUpstreamPolicyV1Enabled: "true",
	}}
	svc := NewSettingService(repo, &config.Config{})
	account := &Account{Type: AccountTypeAPIKey, Platform: PlatformAnthropic}

	ctx := ResolveAndStorePolicy(context.Background(), account, svc)
	policy, ok := GetUpstreamPolicyFromContext(ctx)

	require.False(t, ok)
	require.Nil(t, policy)
}

func TestUpstreamPassthroughPolicyRuntimeDisabled_HelperLegacyBehavior(t *testing.T) {
	ctx := SetUpstreamPolicyInContext(context.Background(), &EffectiveUpstreamPolicy{
		ProfileApplied:         ProfileTransparent,
		ForwardClientHeaders:   true,
		ForwardUserNetworkInfo: true,
		SkipBodyScrub:          true,
		SkipSystemPromptInject: true,
		ForwardClientUA:        true,
		ForwardBetaFlags:       true,
		SkipModelRewrite:       true,
	})

	require.True(t, ShouldScrubBody(ctx))
	require.True(t, ShouldInjectSystemPrompt(ctx))
	require.False(t, ShouldForwardClientHeaders(ctx))
	require.False(t, ShouldForwardUserNetworkInfo(ctx))
	require.False(t, ShouldSkipModelRewrite(ctx))
	require.False(t, ShouldForwardBetaFlags(ctx))
	require.False(t, ShouldForwardClientUA(ctx))

	whitelist := func(key string) bool { return key == "user-agent" }
	require.True(t, ShouldCopyClientHeader(ctx, "user-agent", whitelist))
	require.False(t, ShouldCopyClientHeader(ctx, "x-custom", whitelist))

	upstreamHeaders := http.Header{}
	clientHeaders := http.Header{"X-Forwarded-For": []string{"1.2.3.4"}}
	ForwardUserNetworkInfoHeaders(ctx, upstreamHeaders, clientHeaders)
	require.Empty(t, upstreamHeaders)
}

func TestUpstreamPassthroughPolicyRuntimeDisabled_PassthroughUsesLegacyFields(t *testing.T) {
	ctx := SetUpstreamPolicyInContext(context.Background(), &EffectiveUpstreamPolicy{ProfileApplied: ProfileTransparent})

	anthropic := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{"anthropic_passthrough": false},
	}
	require.False(t, anthropic.IsAnthropicAPIKeyPassthroughEnabledWithContext(ctx))
	anthropic.Extra["anthropic_passthrough"] = true
	require.True(t, anthropic.IsAnthropicAPIKeyPassthroughEnabledWithContext(ctx))

	openai := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{"openai_passthrough": false},
	}
	require.False(t, openai.IsOpenAIPassthroughEnabledWithContext(ctx))
	openai.Extra["openai_passthrough"] = true
	require.True(t, openai.IsOpenAIPassthroughEnabledWithContext(ctx))
}

func TestUpstreamPassthroughPolicyRuntimeDisabled_ModelMappingStillApplies(t *testing.T) {
	account := &Account{
		Credentials: map[string]any{"model_mapping": map[string]any{"claude-3-7-sonnet-20250219": "claude-3-haiku-20240307"}},
	}
	ctx := SetUpstreamPolicyInContext(context.Background(), &EffectiveUpstreamPolicy{SkipModelRewrite: true})

	require.Equal(t, "claude-3-haiku-20240307", account.GetMappedModelForUpstream(ctx, "claude-3-7-sonnet-20250219"))
}
