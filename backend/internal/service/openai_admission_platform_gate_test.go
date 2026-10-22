//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

var nonOpenAICompatiblePlatforms = []string{PlatformGrok, PlatformKimi, PlatformZhipu, PlatformDeepseek, PlatformMiniMax, PlatformOpenCodeGo}

func openAICompatibleTestModel(platform string) string {
	switch platform {
	case PlatformGrok:
		return "grok-4.3"
	case PlatformDeepseek:
		return "deepseek-v4-pro"
	}
	return "gpt-5.1"
}

func TestTieredAdmissionPlansOnlyForOpenAI(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.Scheduling = config.GatewaySchedulingConfig{
		ContinuationMaxWaiting: 100, StickySessionMaxWaiting: 3, StickySessionWaitTimeout: 120 * time.Second,
		FallbackMaxWaiting: 20, FallbackWaitTimeout: 30 * time.Second,
	}
	cfg.Gateway.OpenAIWS.TurnSlotHoldSeconds = 10
	svc := &OpenAIGatewayService{cfg: cfg}
	require.Equal(t, 10*time.Second, svc.OpenAIWSTurnSlotHold(PlatformOpenAI))
	for _, platform := range nonOpenAICompatiblePlatforms {
		t.Run(platform, func(t *testing.T) {
			account := &Account{ID: 31, Platform: platform, Concurrency: 3}
			for _, plan := range []*AccountWaitPlan{stickyWaitPlanFor(cfg.Gateway.Scheduling, account, true), stickyWaitPlanFor(cfg.Gateway.Scheduling, account, false), svc.OpenAIWSAccountWaitPlan(account)} {
				require.Equal(t, AccountWaitClassLegacy, plan.Class)
				require.Equal(t, 120*time.Second, plan.Timeout)
				require.Equal(t, 3, plan.MaxWaiting, "沿用上游的粘性等待上限")
				require.Equal(t, 3, plan.MaxConcurrency)
			}
			require.Zero(t, svc.OpenAIWSTurnSlotHold(platform), "轮间保槽只作用于 openai")
		})
	}
}

func TestTieredAdmissionYieldOnlyForOpenAI(t *testing.T) {
	for _, platform := range append([]string{PlatformOpenAI}, nonOpenAICompatiblePlatforms...) {
		for _, scheduler := range []string{"advanced", "legacy_load", "legacy_priority"} {
			for _, route := range []string{"new", "sticky"} {
				t.Run(platform+"/"+scheduler+"/"+route, func(t *testing.T) {
					resetOpenAIAdvancedSchedulerSettingCacheForTest()
					t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
					groupID, accounts, cfg := newStickyBusySchedulerFixture(t)
					accounts = accounts[:1]
					accounts[0].Platform = platform
					cfg.Gateway.Scheduling.LoadBatchEnabled = scheduler != "legacy_priority"
					svc := &OpenAIGatewayService{
						accountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts},
						cache:       &schedulerTestGatewayCache{},
						cfg:         cfg,
						concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{
							contWaitCounts: map[int64]int{21001: 1},
							loadMap:        map[int64]*AccountLoadInfo{21001: {AccountID: 21001, ContinuationWaiting: 1}},
						}),
					}
					if scheduler == "advanced" {
						svc.rateLimitService = newOpenAIAdvancedSchedulerRateLimitService("true")
					}
					session := ""
					if route == "sticky" {
						session = "bound"
						require.NoError(t, svc.setStickySessionAccountID(context.Background(), &groupID, session, 21001, time.Hour))
					}
					ctx := WithOpenAIAdmissionOptions(context.Background(), OpenAIAdmissionOptions{ContinuationEligible: false})
					result, _, err := svc.SelectAccountWithSchedulerForCapability(ctx, &groupID, "", session, openAICompatibleTestModel(platform), nil, OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityChatCompletions, false, false, true, platform)
					require.NoError(t, err)
					require.NotNil(t, result)
					if platform == PlatformOpenAI {
						require.False(t, result.Acquired, "openai 新会话让位给续聊等待者")
						return
					}
					require.True(t, result.Acquired, "非 openai 平台不感知续聊等待者，沿用上游直接抢槽")
					result.ReleaseFunc()
				})
			}
		}
	}
}

func TestTieredAdmissionStickyFullWaitsOnlyForOpenAI(t *testing.T) {
	for _, platform := range append([]string{PlatformOpenAI}, nonOpenAICompatiblePlatforms...) {
		t.Run(platform, func(t *testing.T) {
			resetOpenAIAdvancedSchedulerSettingCacheForTest()
			t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
			groupID, accounts, cfg := newStickyBusySchedulerFixture(t)
			for i := range accounts {
				accounts[i].Platform = platform
			}
			svc := newStickyFullWaitsService(t, accounts, cfg, &schedulerTestGatewayCache{}, schedulerTestConcurrencyCache{
				acquireResults: map[int64]bool{21001: false, 21002: true},
			})
			require.NoError(t, svc.setStickySessionAccountID(context.Background(), &groupID, "bound", 21001, time.Hour))
			ctx := WithOpenAIAdmissionOptions(context.Background(), OpenAIAdmissionOptions{ContinuationEligible: true, StickyFullWaits: true})
			selection, _, err := svc.SelectAccountWithSchedulerForCapability(ctx, &groupID, "", "bound", openAICompatibleTestModel(platform), nil, OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityChatCompletions, false, false, true, platform)
			require.NoError(t, err)
			require.NotNil(t, selection)
			if platform == PlatformOpenAI {
				require.False(t, selection.Acquired)
				require.Equal(t, int64(21001), selection.WaitPlan.AccountID, "openai WS 续聊满槽不逃逸")
				return
			}
			require.True(t, selection.Acquired, "非 openai 平台满槽照上游逃逸")
			require.Equal(t, int64(21002), selection.Account.ID)
			selection.ReleaseFunc()
		})
	}
}
