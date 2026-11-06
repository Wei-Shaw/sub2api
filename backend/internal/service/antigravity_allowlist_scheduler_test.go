//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func TestAntigravityAllowlistScheduler_StickyMixedAndFailover(t *testing.T) {
	for _, groupPlatform := range []string{PlatformAntigravity, PlatformGemini} {
		for _, loadAware := range []bool{false, true} {
			t.Run(groupPlatform+map[bool]string{true: "/load", false: "/legacy"}[loadAware], func(t *testing.T) {
				groupID := int64(10)
				repo := &mockAccountRepoForPlatform{accountsByID: map[int64]*Account{}}
				for i, model := range []string{"gemini-4-argon-high", "gemini-3.8-flash-high"} {
					repo.accounts = append(repo.accounts, Account{
						ID: int64(i + 1), Platform: PlatformAntigravity, Type: AccountTypeOAuth,
						Priority: i + 1, Status: StatusActive, Schedulable: true, Concurrency: 5,
						AccountGroups: []AccountGroup{{GroupID: groupID}},
						Credentials:   map[string]any{"model_mapping": map[string]any{model: model}},
						Extra:         map[string]any{"mixed_scheduling": true},
					})
				}
				for i := range repo.accounts {
					repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
				}
				group := &Group{ID: groupID, Platform: groupPlatform, Status: StatusActive, Hydrated: true}
				cfg := testConfig()
				cfg.Gateway.Scheduling.LoadBatchEnabled = loadAware
				cache := &mockGatewayCacheForPlatform{sessionBindings: map[string]int64{"sticky": 1}}
				svc := &GatewayService{
					accountRepo: repo, cache: cache, cfg: cfg,
					groupRepo:          &mockGroupRepoForGateway{groups: map[int64]*Group{groupID: group}},
					concurrencyService: NewConcurrencyService(&mockConcurrencyCache{}),
				}
				ctx := context.WithValue(context.Background(), ctxkey.Group, group)
				ctx = WithPrefetchedStickySession(ctx, 1, groupID, false)
				result, err := svc.SelectAccountWithLoadAwareness(ctx, &groupID, "sticky", "gemini-3.8-flash-high", nil, "", 0)
				require.NoError(t, err)
				require.Equal(t, int64(2), result.Account.ID, "disallowed sticky account must never be selected")
				if result.ReleaseFunc != nil {
					result.ReleaseFunc()
				}
				// The allowed candidate failed upstream. The only remaining account
				// permits Argon only and must not be used as a failover escape hatch.
				result, err = svc.SelectAccountWithLoadAwareness(ctx, &groupID, "sticky", "gemini-3.8-flash-high", map[int64]struct{}{2: {}}, "", 0)
				require.Error(t, err)
				require.Nil(t, result)
			})
		}
	}
}
