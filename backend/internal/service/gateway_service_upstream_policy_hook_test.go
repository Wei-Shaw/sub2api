package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestGatewayService_Forward_DoesNotAttachPolicyWhenFlagOn(t *testing.T) {
	resetUpstreamPassthroughDefaultsCache()
	resetUpstreamPassthroughGlobalOverrideCache()

	repo := &fakeSettingRepoForUpstream{values: map[string]string{
		SettingKeyUpstreamPolicyV1Enabled: "true",
	}}
	svc := NewSettingService(repo, &config.Config{})
	account := &Account{Type: AccountTypeAPIKey, Platform: PlatformAnthropic}

	ctx := ResolveAndStorePolicy(context.Background(), account, svc)
	_, ok := GetUpstreamPolicyFromContext(ctx)

	require.False(t, ok, "upstream passthrough policy runtime is disabled; flag must not affect gateway requests")
}
