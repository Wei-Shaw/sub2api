//go:build unit

package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestContinuationMaxWaitingPlans(t *testing.T) {
	account := &Account{ID: 31, Concurrency: 3}
	for _, limit := range []int{7, 100} {
		for _, burst := range []int{0, 2} {
			t.Run(fmt.Sprintf("limit=%d/burst=%d", limit, burst), func(t *testing.T) {
				cfg := config.GatewaySchedulingConfig{
					ContinuationMaxWaiting: limit, ContinuationBurstLimit: burst,
					StickySessionMaxWaiting: 3, StickySessionWaitTimeout: 120 * time.Second,
					FallbackMaxWaiting: 20, FallbackWaitTimeout: 30 * time.Second,
				}
				svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{Scheduling: cfg}}}
				for _, plan := range []*AccountWaitPlan{stickyWaitPlanFor(cfg, account, true), svc.OpenAIWSAccountWaitPlan(account)} {
					require.Equal(t, limit, plan.MaxWaiting)
					require.Equal(t, AccountWaitClassContinuation, plan.Class)
					require.Equal(t, 120*time.Second, plan.Timeout)
					require.Equal(t, account.Concurrency, plan.MaxConcurrency)
				}
				plan := stickyWaitPlanFor(cfg, account, false)
				require.Equal(t, 20, plan.MaxWaiting)
				require.Equal(t, AccountWaitClassNewSession, plan.Class)
				require.Equal(t, 30*time.Second, plan.Timeout)
			})
		}
	}
	require.Equal(t, 100, (&OpenAIGatewayService{}).OpenAIWSAccountWaitPlan(account).MaxWaiting)
}

func TestContinuationMaxWaitingStickySpillover(t *testing.T) {
	for _, advanced := range []bool{false, true} {
		for _, tc := range []struct {
			name           string
			limit, waiting int
			spill          bool
		}{
			{"below sticky threshold", 100, 2, false},
			{"sticky threshold reached", 100, 3, true},
			{"continuation capacity reached first", 1, 1, true},
		} {
			t.Run(fmt.Sprintf("advanced=%v/%s", advanced, tc.name), func(t *testing.T) {
				resetOpenAIAdvancedSchedulerSettingCacheForTest()
				t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
				groupID, accounts, cfg := newStickyBusySchedulerFixture(t)
				cfg.Gateway.Scheduling.LoadBatchEnabled = true
				cfg.Gateway.Scheduling.StickySessionMaxWaiting = 3
				cfg.Gateway.Scheduling.ContinuationMaxWaiting = tc.limit
				cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:continuation-capacity": 21001}}
				svc := &OpenAIGatewayService{
					accountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts}, cfg: cfg, cache: cache,
					concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{
						acquireResults: map[int64]bool{21001: false, 21002: true},
						contWaitCounts: map[int64]int{21001: tc.waiting},
						loadMap:        map[int64]*AccountLoadInfo{21001: {AccountID: 21001, LoadRate: 100}, 21002: {AccountID: 21002}},
					}),
				}
				if advanced {
					svc.rateLimitService = newOpenAIAdvancedSchedulerRateLimitService("true")
				}
				ctx := WithOpenAIAdmissionOptions(context.Background(), OpenAIAdmissionOptions{ContinuationEligible: true, StickyFullWaits: true})
				selection, _, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "continuation-capacity", "gpt-5.1", nil, OpenAIUpstreamTransportAny, false)
				require.NoError(t, err)
				require.NotNil(t, selection)
				if tc.spill {
					require.True(t, selection.Acquired)
					require.Equal(t, int64(21002), selection.Account.ID)
					selection.ReleaseFunc()
				} else {
					require.False(t, selection.Acquired)
					require.Equal(t, int64(21001), selection.Account.ID)
					require.NotNil(t, selection.WaitPlan)
					require.Equal(t, tc.limit, selection.WaitPlan.MaxWaiting)
				}
				require.Equal(t, int64(21001), cache.sessionBindings["openai:continuation-capacity"])
			})
		}
	}
}
