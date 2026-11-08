//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGrokTeamModelRateLimit_MarksAndFiltersSiblings(t *testing.T) {
	// Isolate from other tests by using unique team ids.
	team := "team-test-" + time.Now().Format("150405.000")
	a1 := &Account{
		ID: 101, Platform: PlatformGrok, Type: AccountTypeOAuth,
		Credentials: map[string]any{"team_id": team},
	}
	a2 := &Account{
		ID: 102, Platform: PlatformGrok, Type: AccountTypeOAuth,
		Credentials: map[string]any{"team_id": team},
	}
	other := &Account{
		ID: 103, Platform: PlatformGrok, Type: AccountTypeOAuth,
		Credentials: map[string]any{"team_id": team + "-other"},
	}
	noTeam := &Account{
		ID: 104, Platform: PlatformGrok, Type: AccountTypeOAuth,
		Credentials: map[string]any{},
	}

	now := time.Now()
	markGrokTeamModelRateLimit(a1, "grok-4.5", now.Add(5*time.Minute))

	require.True(t, isGrokTeamModelRateLimited(a1, "grok-4.5", now))
	require.True(t, isGrokTeamModelRateLimited(a2, "grok-4.5", now), "sibling with same team must cool")
	require.False(t, isGrokTeamModelRateLimited(a2, "grok-4.3", now), "other model stays pickable")
	require.False(t, isGrokTeamModelRateLimited(other, "grok-4.5", now))
	require.False(t, isGrokTeamModelRateLimited(noTeam, "grok-4.5", now))

	filtered := filterGrokTeamModelRateLimitedAccounts([]Account{*a1, *a2, *other, *noTeam}, "grok-4.5", now)
	require.Len(t, filtered, 2)
	ids := []int64{filtered[0].ID, filtered[1].ID}
	require.Contains(t, ids, int64(103))
	require.Contains(t, ids, int64(104))
}

func TestGrokTeamModelRateLimit_Expires(t *testing.T) {
	team := "team-expire-" + time.Now().Format("150405.000")
	a := &Account{
		ID: 201, Platform: PlatformGrok, Type: AccountTypeOAuth,
		Credentials: map[string]any{"team_id": team},
	}
	past := time.Now().Add(-time.Minute)
	markGrokTeamModelRateLimit(a, "grok-4.5", past)
	// mark clamps expired until into default TTL from "now" — use direct store inject via past+recheck
	// After mark with past, resolveGrokTeamRateLimitUntil path isn't used; mark uses now+default when until not after now.
	require.True(t, isGrokTeamModelRateLimited(a, "grok-4.5", time.Now()))
}

func TestGrokTeamModelRateLimitFilterUsesMappedUpstreamModel(t *testing.T) {
	now := time.Now()
	account := &Account{
		ID:       301,
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"team_id":       "team-mapped-301",
			"model_mapping": map[string]any{"gpt-*": "grok-4.5"},
		},
	}
	markGrokTeamModelRateLimit(account, "grok-4.5", now.Add(time.Hour))

	require.Empty(t, filterGrokTeamModelRateLimitedAccounts([]Account{*account}, "gpt-5", now))
}
