package service

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestAccountSchedulingLiveSelectionSkipsGovernedBeforeFailover(t *testing.T) {
	for _, advanced := range []bool{false, true} {
		for _, enabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("advanced=%t/policy=%t", advanced, enabled), func(t *testing.T) {
				defer resetOpenAIAdvancedSchedulerSettingCacheForTest()
				accounts := make([]Account, 0, 5)
				allowed := map[int64]bool{}
				for id := int64(1); id <= 4; id++ {
					accounts = append(accounts, Account{
						ID: id, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
						Status: StatusActive, Schedulable: true, Concurrency: 1,
						Credentials: map[string]any{"plan_type": "free"},
					})
					allowed[id] = true
				}
				const paidID int64 = 5
				accounts = append(accounts, Account{
					ID: paidID, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
					Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 100,
					Credentials: map[string]any{"plan_type": "plus"},
				})
				poolRepo := &accountSchedulingTestRepository{Decision: &AccountSchedulingDecision{
					Enabled: enabled, Allowed: allowed,
				}}
				poolSettings := &accountSchedulingTestSettings{Raw: fmt.Sprintf(`{"enabled":%t,"max_active_accounts":4}`, enabled)}
				var acquiredIDs []int64
				gateway := &OpenAIGatewayService{
					accountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts},
					cache:       &schedulerTestGatewayCache{},
					cfg:         &config.Config{},
					rateLimitService: newOpenAIAdvancedSchedulerRateLimitService(
						strconv.FormatBool(advanced), "false", "false",
					),
					concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{acquiredIDs: &acquiredIDs}),
					accountSchedulingPools: NewAccountSchedulingPoolService(
						poolRepo, nil, nil, poolSettings,
					),
				}
				ctx := WithOpenAIProfitControlSuppressed(context.Background())
				selection, _, err := gateway.SelectAccountWithSchedulerForCapability(
					ctx, nil, "", "", "", nil,
					OpenAIUpstreamTransportHTTPSSE, OpenAIEndpointCapabilityLive,
					false, false, false,
				)
				require.NoError(t, err)
				require.NotNil(t, selection)
				require.True(t, selection.Acquired)
				t.Cleanup(selection.ReleaseFunc)
				if enabled {
					require.Equal(t, paidID, selection.Account.ID, "four higher-priority Free accounts must not consume all Live failover attempts")
					require.Equal(t, []int64{paidID}, acquiredIDs, "unsupported Live candidates must be filtered before acquiring concurrency")
				} else {
					// Advanced scheduling may legitimately choose either tier. The
					// disabled policy must preserve the complete candidate set rather
					// than impose a deterministic winner on its weighted selection.
					liveCtx := context.WithValue(ctx, accountSchedulingSelectionCapabilityKey{}, OpenAIEndpointCapabilityLive)
					eligible, filterErr := gateway.accountSchedulingPools.EligibleAccounts(liveCtx, accounts)
					require.NoError(t, filterErr)
					require.Equal(t, accounts, eligible, "disabling the policy preserves all existing Live candidates")
					require.Empty(t, poolRepo.Requests, "disabled policy must not query pool admission")
				}
			})
		}
	}
}
