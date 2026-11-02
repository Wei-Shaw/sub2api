//go:build unit

package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type admissionRoutingCache struct {
	schedulerTestConcurrencyCache
	classes []AccountWaitClass
	limits  []int
	legacy  int
	allow   bool
}

func (c *admissionRoutingCache) AcquireAccountSlot(_ context.Context, _ int64, _ int, _ string) (bool, error) {
	c.legacy++
	return c.allow, nil
}

func (c *admissionRoutingCache) AcquireAccountSlotForClass(_ context.Context, _ int64, _ int, _ string, class AccountWaitClass, limit int) (bool, error) {
	c.classes = append(c.classes, class)
	c.limits = append(c.limits, limit)
	return c.allow, nil
}

func (c *admissionRoutingCache) ReuseAccountSlot(context.Context, int64, string, string, int) (bool, error) {
	return true, nil
}

func (c *admissionRoutingCache) GetAccountAdmissionState(context.Context, int64) (*AccountAdmissionState, error) {
	return &AccountAdmissionState{}, nil
}

func TestContinuationBurstSchedulerEntryPoints(t *testing.T) {
	for _, platform := range []string{PlatformOpenAI, PlatformGrok, PlatformKimi, PlatformZhipu, PlatformDeepseek, PlatformMiniMax, PlatformOpenCodeGo} {
		for _, scheduler := range []string{"advanced", "legacy_load", "legacy_priority"} {
			for _, route := range []string{"new", "sticky_continuation", "sticky_new"} {
				t.Run(platform+"/"+scheduler+"/"+route, func(t *testing.T) {
					resetOpenAIAdvancedSchedulerSettingCacheForTest()
					t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
					groupID, accounts, cfg := newStickyBusySchedulerFixture(t)
					accounts = accounts[:1]
					accounts[0].Platform = platform
					model := "gpt-5.1"
					switch platform {
					case PlatformGrok:
						model = "grok-4.3"
					case PlatformDeepseek:
						model = "deepseek-v4-pro"
					}
					cfg.Gateway.Scheduling.ContinuationBurstLimit = 2
					cfg.Gateway.Scheduling.LoadBatchEnabled = scheduler != "legacy_priority"
					cache := &admissionRoutingCache{allow: true, schedulerTestConcurrencyCache: schedulerTestConcurrencyCache{
						contWaitCounts: map[int64]int{21001: 1},
						loadMap:        map[int64]*AccountLoadInfo{21001: {AccountID: 21001, ContinuationWaiting: 1}},
					}}
					svc := &OpenAIGatewayService{
						accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
						cache:              &schedulerTestGatewayCache{},
						cfg:                cfg,
						concurrencyService: NewConcurrencyService(cache),
					}
					if scheduler == "advanced" {
						svc.rateLimitService = newOpenAIAdvancedSchedulerRateLimitService("true")
					}
					session := ""
					want := AccountWaitClassNewSession
					if route != "new" {
						session = "bound"
						require.NoError(t, svc.setStickySessionAccountID(context.Background(), &groupID, session, 21001, time.Hour))
						if route == "sticky_continuation" {
							want = AccountWaitClassContinuation
						}
					}
					ctx := WithOpenAIAdmissionOptions(context.Background(), OpenAIAdmissionOptions{ContinuationEligible: want == AccountWaitClassContinuation, StickyFullWaits: true})
					selectAccount := func() *AccountSelectionResult {
						result, _, err := svc.SelectAccountWithSchedulerForCapability(ctx, &groupID, "", session, model, nil, OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityChatCompletions, false, false, true, platform)
						require.NoError(t, err)
						require.NotNil(t, result)
						return result
					}
					result := selectAccount()
					require.True(t, result.Acquired, "旧续聊等待快照不能挡住原子准入的放行")
					if platform != PlatformOpenAI {
						result.ReleaseFunc()
						cache.allow = false
						result = selectAccount()
						require.False(t, result.Acquired)
						require.NotNil(t, result.WaitPlan)
						require.Equal(t, AccountWaitClassLegacy, result.WaitPlan.Class, "非 openai 平台不分续聊/新会话类")
						wantWaiting := cfg.Gateway.Scheduling.StickySessionMaxWaiting
						if route == "new" {
							wantWaiting = cfg.Gateway.Scheduling.FallbackMaxWaiting
						}
						require.Equal(t, wantWaiting, result.WaitPlan.MaxWaiting, "沿用上游的等待上限")
						require.Empty(t, cache.classes, "非 openai 平台不走连续准入原子仲裁")
						require.Positive(t, cache.legacy)
						return
					}
					require.NotNil(t, result.ReuseFunc, "已获槽结果携带下一轮准入句柄")
					result.ReleaseFunc()
					cache.allow = false
					result = selectAccount()
					require.False(t, result.Acquired, "原子准入拒绝后必须等待，不能绕过")
					require.NotNil(t, result.WaitPlan)
					require.Equal(t, want, result.WaitPlan.Class)
					require.Equal(t, 100, result.WaitPlan.MaxWaiting)
					require.NotEmpty(t, cache.classes)
					require.Zero(t, cache.legacy, "openai 分级准入不得绕过原子仲裁")
					for i, class := range cache.classes {
						require.Equal(t, want, class)
						require.Equal(t, 2, cache.limits[i])
					}
				})
			}
		}
	}
}

func TestContinuationBurstPreviousResponseEntry(t *testing.T) {
	for _, advanced := range []bool{true, false} {
		t.Run(fmt.Sprint(advanced), func(t *testing.T) {
			resetOpenAIAdvancedSchedulerSettingCacheForTest()
			t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
			groupID, account, cfg := newPreviousResponseSchedulerFixture(t)
			account.GroupIDs = []int64{groupID}
			cfg.Gateway.Scheduling.ContinuationBurstLimit = 2
			cache := &admissionRoutingCache{allow: false}
			svc := &OpenAIGatewayService{
				accountRepo:        schedulerTestOpenAIAccountRepo{accounts: []Account{account}},
				cache:              &schedulerTestGatewayCache{},
				cfg:                cfg,
				concurrencyService: NewConcurrencyService(cache),
			}
			if advanced {
				svc.rateLimitService = newOpenAIAdvancedSchedulerRateLimitService("true")
			}
			ctx := WithOpenAIAdmissionOptions(context.Background(), OpenAIAdmissionOptions{ContinuationEligible: true, StickyFullWaits: true})
			require.NoError(t, svc.getOpenAIWSStateStore().BindResponseAccount(ctx, groupID, "resp_burst", account.ID, time.Hour))
			selection, _, err := svc.SelectAccountWithScheduler(ctx, &groupID, "resp_burst", "session", "gpt-5.1", nil, OpenAIUpstreamTransportAny, false)
			require.NoError(t, err)
			require.False(t, selection.Acquired)
			require.Equal(t, AccountWaitClassContinuation, selection.WaitPlan.Class)
			require.Equal(t, 100, selection.WaitPlan.MaxWaiting)
			require.Equal(t, []AccountWaitClass{AccountWaitClassContinuation}, cache.classes)
			require.Equal(t, []int{2}, cache.limits)
		})
	}
}

func TestContinuationBurstWeightedStickyEntry(t *testing.T) {
	for _, continuation := range []bool{true, false} {
		t.Run(fmt.Sprint(continuation), func(t *testing.T) {
			groupID, accounts, cfg := newStickyBusySchedulerFixture(t)
			cfg.Gateway.Scheduling.ContinuationBurstLimit = 2
			cache := &admissionRoutingCache{allow: !continuation, schedulerTestConcurrencyCache: schedulerTestConcurrencyCache{contWaitCounts: map[int64]int{21001: 1}}}
			svc := &OpenAIGatewayService{
				accountRepo:        schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: accounts[:1]}},
				cache:              &schedulerTestGatewayCache{},
				cfg:                cfg,
				concurrencyService: NewConcurrencyService(cache),
			}
			scheduler := &defaultOpenAIAccountScheduler{service: svc, stats: newOpenAIAccountRuntimeStats()}
			selection, err := scheduler.tryFallbackToWeightedSticky(context.Background(), OpenAIAccountScheduleRequest{
				GroupID: &groupID, Platform: PlatformOpenAI, StickyAccountID: 21001, StickyWeighted: true,
				RequestedModel: "gpt-5.1", RequiredTransport: OpenAIUpstreamTransportAny,
				RequiredCapability: OpenAIEndpointCapabilityChatCompletions, ContinuationEligible: continuation,
			})
			require.NoError(t, err)
			require.NotNil(t, selection)
			require.Equal(t, !continuation, selection.Acquired)
			want := AccountWaitClassNewSession
			if continuation {
				want = AccountWaitClassContinuation
			}
			require.Equal(t, []AccountWaitClass{want}, cache.classes)
			require.Equal(t, []int{2}, cache.limits)
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
		})
	}
}
