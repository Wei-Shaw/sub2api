package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"

	"github.com/stretchr/testify/require"
)

// TestResolveCacheTTLUsageOverride_UserPrefOutranksAccountOverride ensures the new
// user-level preference (C2) sits at the top of the priority chain. Without this
// behaviour, customers selecting "cost_priority" would be silently overridden by an
// operator-set account TTL — defeating the self-service promise of Workstream C.
func TestResolveCacheTTLUsageOverride_UserPrefOutranksAccountOverride(t *testing.T) {
	svc := &GatewayService{settingService: NewSettingService(&gatewayTTLSettingRepo{}, &config.Config{})}
	account := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"cache_ttl_override_enabled": true,
			"cache_ttl_override_target":  "1h", // account says 1h
		},
	}
	apiKey := &APIKey{CacheStrategy: CacheStrategyCostPriority} // user says 5m

	target, ok, trace := svc.resolveCacheTTLUsageOverride(context.Background(), account, apiKey)
	require.True(t, ok)
	require.Equal(t, "5m", target, "user pref must win over account override")
	require.Equal(t, "user_pref:5m", trace)
}

func TestResolveCacheTTLUsageOverride_UserPrefLatencyForces1h(t *testing.T) {
	svc := &GatewayService{settingService: NewSettingService(&gatewayTTLSettingRepo{}, &config.Config{})}
	account := &Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	apiKey := &APIKey{CacheStrategy: CacheStrategyLatencyPriority}

	target, ok, trace := svc.resolveCacheTTLUsageOverride(context.Background(), account, apiKey)
	require.True(t, ok)
	require.Equal(t, "1h", target)
	require.Equal(t, "user_pref:1h", trace)
}

// TestResolveCacheTTLUsageOverride_UserPrefIgnoredOnUnsupportedAccount documents the
// capability gate: a user selecting a strategy bound to an OpenAI account must NOT
// silently take effect (would mis-classify usage for a platform that has no
// Anthropic cache semantics). The trace value is the smoking gun for ops debugging
// "why didn't my setting work?" tickets.
func TestResolveCacheTTLUsageOverride_UserPrefIgnoredOnUnsupportedAccount(t *testing.T) {
	svc := &GatewayService{settingService: NewSettingService(&gatewayTTLSettingRepo{}, &config.Config{})}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	apiKey := &APIKey{CacheStrategy: CacheStrategyCostPriority}

	target, ok, trace := svc.resolveCacheTTLUsageOverride(context.Background(), account, apiKey)
	require.False(t, ok)
	require.Empty(t, target)
	require.Equal(t, "user_pref_ignored:account_unsupported", trace)
}

func TestResolveCacheTTLUsageOverride_AutoFallsThroughToAccount(t *testing.T) {
	svc := &GatewayService{settingService: NewSettingService(&gatewayTTLSettingRepo{}, &config.Config{})}
	account := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"cache_ttl_override_enabled": true,
			"cache_ttl_override_target":  "1h",
		},
	}
	apiKey := &APIKey{CacheStrategy: CacheStrategyAuto}

	target, ok, trace := svc.resolveCacheTTLUsageOverride(context.Background(), account, apiKey)
	require.True(t, ok)
	require.Equal(t, "1h", target)
	require.Equal(t, "acct_override:1h", trace,
		"with apiKey.cache_strategy=auto, account override must surface unchanged")
}

func TestResolveCacheTTLUsageOverride_NilAPIKeyMatchesPreC2Behaviour(t *testing.T) {
	svc := &GatewayService{settingService: NewSettingService(&gatewayTTLSettingRepo{}, &config.Config{})}
	account := &Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth, Extra: map[string]any{
		"cache_ttl_override_enabled": true,
		"cache_ttl_override_target":  "5m",
	}}

	target, ok, trace := svc.resolveCacheTTLUsageOverride(context.Background(), account, nil)
	require.True(t, ok)
	require.Equal(t, "5m", target)
	require.Equal(t, "acct_override:5m", trace)
}

// TestResolveCacheTTLRequestRewriteTarget_MirrorsUsageTarget locks in the
// "consistent by default" rule: until we deliberately introduce divergence, the
// request-rewrite target must equal the usage-classification target. This guards
// against accidental drift that would create a compliance gap ("upstream produced
// 1h cache but we billed 5m").
func TestResolveCacheTTLRequestRewriteTarget_MirrorsUsageTarget(t *testing.T) {
	svc := &GatewayService{settingService: NewSettingService(&gatewayTTLSettingRepo{}, &config.Config{})}
	account := &Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	apiKey := &APIKey{CacheStrategy: CacheStrategyLatencyPriority}

	usageTarget, usageOK := svc.resolveCacheTTLUsageOverrideTarget(context.Background(), account, apiKey)
	rewriteTarget, rewriteOK := svc.resolveCacheTTLRequestRewriteTarget(context.Background(), account, apiKey)

	require.Equal(t, usageOK, rewriteOK)
	require.Equal(t, usageTarget, rewriteTarget)
}
