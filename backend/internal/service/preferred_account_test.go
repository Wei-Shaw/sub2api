//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestGatewayPreferredAccountsAcrossPlatforms(t *testing.T) {
	for _, platform := range []string{PlatformAnthropic, PlatformGemini, PlatformAntigravity} {
		for _, mode := range []string{"batch", "legacy"} {
			t.Run(platform+"/"+mode, func(t *testing.T) {
				group := &Group{ID: 12, Platform: platform, Status: StatusActive, Hydrated: true, PreferUserAccounts: true}
				aliceID, bobID := int64(101), int64(102)
				account := func(id int64, priority int, userID *int64) Account {
					return Account{ID: id, Platform: platform, Type: AccountTypeAPIKey, Status: StatusActive,
						Schedulable: true, Concurrency: 2, Priority: priority, GroupIDs: []int64{group.ID}, PreferredUserID: userID}
				}
				accounts := []Account{account(1, 90, &aliceID), account(2, 80, &bobID), account(3, 1, nil)}
				slots := map[int64]bool{}
				model := ""
				cfg := &config.Config{}
				cfg.Gateway.Scheduling.LoadBatchEnabled = mode == "batch"
				gateway := &GatewayService{
					cfg: cfg,
					accountRepo: &mockAccountRepoForPlatform{accounts: accounts, accountsByID: map[int64]*Account{
						1: &accounts[0], 2: &accounts[1], 3: &accounts[2],
					}},
					groupRepo:          &mockGroupRepoForGateway{groups: map[int64]*Group{group.ID: group}},
					cache:              &schedulerTestGatewayCache{},
					concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{acquireResults: slots}),
				}
				pick := func(userID int64, session string, excluded map[int64]struct{}, want int64) {
					t.Helper()
					ctx := WithPreferredAccountUser(context.Background(), &APIKey{UserID: userID, Group: group})
					result, err := gateway.SelectAccountWithLoadAwareness(ctx, &group.ID, session, model, excluded, "", userID)
					require.NoError(t, err)
					require.NotNil(t, result)
					require.True(t, result.Acquired)
					defer result.ReleaseFunc()
					require.Equal(t, want, result.Account.ID)
				}
				pick(aliceID, "alice-first", nil, 1)
				pick(bobID, "bob-first", nil, 2)
				pick(aliceID, "alice-excluded", map[int64]struct{}{1: {}}, 3)
				slots[1] = false
				pick(aliceID, "alice-busy", nil, 3)
				delete(slots, 1)
				accounts[0].Extra = map[string]any{"quota_limit": 10.0, "quota_used": 10.0}
				pick(aliceID, "alice-quota", nil, 3)
				accounts[0].Extra = nil
				pick(aliceID, "alice-quota", nil, 3)
				pick(aliceID, "alice-recovered", nil, 1)
				if platform == PlatformAnthropic {
					model = "claude-sonnet-4-5"
					group.ModelRoutingEnabled = true
					group.ModelRouting = map[string][]int64{model: {2}}
					pick(aliceID, "alice-explicit-route", nil, 2)
					group.ModelRoutingEnabled = false
					model = ""
				}
				group.PreferUserAccounts = false
				pick(aliceID, "alice-disabled", nil, 3)
			})
		}
	}
}

func TestOpenAICompatiblePreferredAccount(t *testing.T) {
	t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
	cfg := newSchedulerTestOpenAIWSV2Config()
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	cfg.Gateway.OpenAIWS.LBTopK = 1
	group := &Group{ID: 12, Platform: PlatformKimi, PreferUserAccounts: true}
	userID := int64(101)
	accounts := []Account{
		{ID: 1, Platform: PlatformKimi, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 2, Priority: 90, GroupIDs: []int64{group.ID}, PreferredUserID: &userID},
		{ID: 2, Platform: PlatformKimi, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 2, Priority: 1, GroupIDs: []int64{group.ID}},
	}
	gateway := &OpenAIGatewayService{
		cfg: cfg, cache: &schedulerTestGatewayCache{},
		accountRepo:        schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: accounts}},
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
		rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("true"),
	}
	ctx := WithPreferredAccountUser(context.Background(), &APIKey{UserID: userID, Group: group})
	pick := func(session string, want int64) {
		t.Helper()
		result, _, err := gateway.SelectAccountWithSchedulerForCapability(ctx, &group.ID, "", session, "kimi-k2.5", nil, OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityChatCompletions, false, false, false, PlatformKimi)
		require.NoError(t, err)
		require.True(t, result.Acquired)
		defer result.ReleaseFunc()
		require.Equal(t, want, result.Account.ID)
	}
	pick("new-kimi", 1)
	accounts[0].Schedulable = false
	pick("fallback-kimi", 2)
	accounts[0].Schedulable = true
	pick("fallback-kimi", 2)
}
