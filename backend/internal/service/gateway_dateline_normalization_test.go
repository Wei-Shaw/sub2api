package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/anthropicfp"
	"github.com/stretchr/testify/require"
)

// TestGatewayClientDatelineNormalization_Scope covers the account/switch matrix
// for the shouldNormalizeClientDateline gate: Anthropic OAuth/SetupToken pass
// only when the switch is on; API-Key and non-Anthropic platforms are excluded
// unconditionally.
func TestGatewayClientDatelineNormalization_Scope(t *testing.T) {
	repo := &gatewayTTLSettingRepo{data: map[string]string{}}
	gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{})
	svc := &GatewayService{
		settingService: NewSettingService(repo, &config.Config{}),
	}
	ctx := context.Background()

	// Default (missing key): fallback in parseSettings/cache loader is true.
	require.True(t, svc.shouldNormalizeClientDateline(ctx, &Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth}))
	require.True(t, svc.shouldNormalizeClientDateline(ctx, &Account{Platform: PlatformAnthropic, Type: AccountTypeSetupToken}))
	require.False(t, svc.shouldNormalizeClientDateline(ctx, &Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey}))
	require.False(t, svc.shouldNormalizeClientDateline(ctx, &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}))

	// Switch off: no account qualifies.
	repo.data[SettingKeyEnableClientDatelineNormalization] = "false"
	gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{})
	require.False(t, svc.shouldNormalizeClientDateline(ctx, &Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth}))
	require.False(t, svc.shouldNormalizeClientDateline(ctx, &Account{Platform: PlatformAnthropic, Type: AccountTypeSetupToken}))

	// Switch back on: OAuth qualifies again.
	repo.data[SettingKeyEnableClientDatelineNormalization] = "true"
	gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{})
	require.True(t, svc.shouldNormalizeClientDateline(ctx, &Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth}))
}

// TestGatewayClientDatelineNormalization_HelperNoRewrite exercises the code
// path used by Forward: the helper must return ok=false when the switch is
// off, when the account is API-Key, when the account is nil, and when the
// body carries no fingerprinted dateline. It must return ok=true and a
// rewritten body when both the switch is on and the account is Anthropic
// OAuth/SetupToken and a rewrite actually happened.
func TestGatewayClientDatelineNormalization_HelperNoRewrite(t *testing.T) {
	repo := &gatewayTTLSettingRepo{data: map[string]string{
		SettingKeyEnableClientDatelineNormalization: "true",
	}}
	gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{})
	svc := &GatewayService{
		settingService: NewSettingService(repo, &config.Config{}),
	}
	ctx := context.Background()

	dirty := []byte(`{"messages":[{"role":"user","content":"<system-reminder>\nToday’s date is 2026/07/01.\n</system-reminder>"}]}`)
	clean := []byte(`{"messages":[{"role":"user","content":"just hello"}]}`)

	// API-Key account: never rewrites, even with dirty payload.
	next, ok := svc.normalizeClientDatelineIfEnabled(ctx, &Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey}, dirty)
	require.False(t, ok)
	require.Nil(t, next)

	// Nil account: safe no-op.
	next, ok = svc.normalizeClientDatelineIfEnabled(ctx, nil, dirty)
	require.False(t, ok)
	require.Nil(t, next)

	// OAuth account + clean body: no changes, ok=false.
	next, ok = svc.normalizeClientDatelineIfEnabled(ctx, &Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth}, clean)
	require.False(t, ok)
	require.Nil(t, next)

	// OAuth account + dirty body: rewritten, ok=true.
	next, ok = svc.normalizeClientDatelineIfEnabled(ctx, &Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth}, dirty)
	require.True(t, ok)
	require.NotNil(t, next)
	require.Contains(t, string(next), "Today's date is 2026-07-01.")
	require.NotContains(t, string(next), "2026/07/01")
	require.NotContains(t, string(next), "Today’s date is")

	// SetupToken account + dirty body: rewritten, ok=true.
	next, ok = svc.normalizeClientDatelineIfEnabled(ctx, &Account{Platform: PlatformAnthropic, Type: AccountTypeSetupToken}, dirty)
	require.True(t, ok)
	require.Contains(t, string(next), "Today's date is 2026-07-01.")

	// Switch off: even OAuth account is not rewritten.
	repo.data[SettingKeyEnableClientDatelineNormalization] = "false"
	gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{})
	next, ok = svc.normalizeClientDatelineIfEnabled(ctx, &Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth}, dirty)
	require.False(t, ok)
	require.Nil(t, next)
}

// TestGatewayClientDatelineNormalization_LeavesUserProseUntouched double-checks
// that the pure normalizer never touches content outside <system-reminder>
// blocks. This is an integration guard between the switch-gated helper and
// the pkg/anthropicfp scope contract, tripped by anyone who broadens scope.
func TestGatewayClientDatelineNormalization_LeavesUserProseUntouched(t *testing.T) {
	repo := &gatewayTTLSettingRepo{data: map[string]string{
		SettingKeyEnableClientDatelineNormalization: "true",
	}}
	gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{})
	svc := &GatewayService{
		settingService: NewSettingService(repo, &config.Config{}),
	}
	ctx := context.Background()

	// User prose that happens to include a fingerprint-looking sentence
	// (outside <system-reminder>) must be preserved byte-for-byte.
	body := []byte(`{"messages":[{"role":"user","content":"I wrote: Today’s date is 2026/07/01. What do you think?"}]}`)
	next, ok := svc.normalizeClientDatelineIfEnabled(ctx, &Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth}, body)
	require.False(t, ok, "must not rewrite user prose outside <system-reminder>")
	require.Nil(t, next)

	// Direct pure-fn check for redundancy.
	out, hits, changed := anthropicfp.NormalizeDateline(body)
	require.False(t, changed)
	require.Empty(t, hits)
	require.Equal(t, body, out)
}
