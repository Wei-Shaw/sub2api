package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAccountNeedsUpstreamBalanceProbe(t *testing.T) {
	require.False(t, accountNeedsUpstreamBalanceProbe(nil))
	require.False(t, accountNeedsUpstreamBalanceProbe(&Account{
		Type:   AccountTypeAPIKey,
		Status: StatusActive,
	}))
	require.True(t, accountNeedsUpstreamBalanceProbe(&Account{
		Type:   AccountTypeAPIKey,
		Status: StatusActive,
		Credentials: map[string]any{
			BalanceProbeSourceCredentialKey: BalanceProbeSourceNewAPI,
		},
	}))
	require.False(t, accountNeedsUpstreamBalanceProbe(&Account{
		Type:   AccountTypeOAuth,
		Status: StatusActive,
		Credentials: map[string]any{
			BalanceProbeSourceCredentialKey: BalanceProbeSourceSub2API,
		},
	}))
}

func TestUpstreamBalanceProbeDue(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	interval := 30 * time.Minute

	neverProbed := &Account{Extra: map[string]any{}}
	require.True(t, upstreamBalanceProbeDue(neverProbed, now, interval))

	fresh := &Account{Extra: map[string]any{
		upstreamBalanceExtraUpdated: now.Add(-10 * time.Minute).Format(time.RFC3339),
	}}
	require.False(t, upstreamBalanceProbeDue(fresh, now, interval))

	stale := &Account{Extra: map[string]any{
		upstreamBalanceExtraUpdated: now.Add(-31 * time.Minute).Format(time.RFC3339),
	}}
	require.True(t, upstreamBalanceProbeDue(stale, now, interval))
}
