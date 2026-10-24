//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompositeScopeSourcePrivacyFiltersBeforeGatewayRanking(t *testing.T) {
	for _, engine := range []string{"gateway-priority", "gateway-load"} {
		t.Run(engine, func(t *testing.T) {
			repo, groups := scopeIsolationFixtures()
			groups.groups[10].RequirePrivacySet = true
			groups.groups[20].Platform = PlatformAntigravity
			groups.groups[20].RequirePrivacySet = false // destination policy never replaces source policy
			for _, id := range []int64{1, 2} {
				account := repo.accountsByID[id]
				account.Platform = PlatformAntigravity
				account.Type = AccountTypeAPIKey
				account.GroupIDs = []int64{20}
				account.AccountGroups = []AccountGroup{{GroupID: 20}}
			}
			repo.accountsByID[1].Priority = 0
			repo.accountsByID[2].Priority = 20
			repo.accountsByID[2].Extra = map[string]any{"privacy_mode": AntigravityPrivacySet}
			target := int64(20)
			decision := scopeIsolationDecision(&target, nil)
			decision.TargetPlatform = PlatformAntigravity
			decision.UpstreamModel = "claude-sonnet-4-5"
			ctx := WithCompositeRouteDecision(context.Background(), decision)
			result, err := scopeIsolationScheduler(t, engine, repo, groups, &schedulerTestGatewayCache{})(ctx, "privacy-session", "", nil)
			require.NoError(t, err)
			require.NotNil(t, result)
			defer releaseLegacySchedulerDecisionSelection(result)
			require.Equal(t, int64(2), result.Account.ID, "higher-priority privacy-unset candidate must be filtered before ranking, not abort after slot acquisition")
			require.True(t, result.Account.IsPrivacySet())
		})
	}
}
