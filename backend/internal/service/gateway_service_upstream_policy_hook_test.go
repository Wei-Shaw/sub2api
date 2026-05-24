package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// This test verifies the gateway-entry seam MECHANICALLY: when ResolveAndStorePolicy
// is called with FeatureFlag ON, the returned ctx has the policy attached.
// We don't run the full GatewayService.Forward pipeline (it requires deep HTTP/Account
// setup); instead we test the helper that Forward calls. Phase B-2 will add
// behavioral integration tests at downstream call sites.

func TestGatewayService_Forward_AttachesPolicyWhenFlagOn(t *testing.T) {
	resetUpstreamPassthroughDefaultsCache()
	resetUpstreamPassthroughGlobalOverrideCache()

	repo := &fakeSettingRepoForUpstream{values: map[string]string{
		SettingKeyUpstreamPolicyV1Enabled: "true",
	}}
	svc := NewSettingService(repo, &config.Config{})

	a := &Account{Type: AccountTypeAPIKey, Platform: PlatformAnthropic}
	ctx := ResolveAndStorePolicy(context.Background(), a, svc)

	policy, ok := GetUpstreamPolicyFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, CategoryOfficial, policy.Category)
}

func TestGatewayService_Forward_NoAttachWhenFlagOff(t *testing.T) {
	resetUpstreamPassthroughDefaultsCache()
	resetUpstreamPassthroughGlobalOverrideCache()

	repo := &fakeSettingRepoForUpstream{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})

	a := &Account{Type: AccountTypeAPIKey, Platform: PlatformAnthropic}
	ctx := ResolveAndStorePolicy(context.Background(), a, svc)

	_, ok := GetUpstreamPolicyFromContext(ctx)
	require.False(t, ok)
}
