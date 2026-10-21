package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type openAILegacySuccessTestCache struct {
	schedulerTestGatewayCache
	bindings map[string]GatewayStickySuccessBinding
	writes   int
}

func (c *openAILegacySuccessTestCache) GetGatewayStickySuccess(_ context.Context, groupID int64, session, model string) (GatewayStickySuccessBinding, error) {
	return c.bindings[buildLegacySuccessTestKey(groupID, session, model)], nil
}

func buildLegacySuccessTestKey(groupID int64, session, model string) string {
	return fmt.Sprintf("%d:%s:%s", groupID, session, model)
}

func (c *openAILegacySuccessTestCache) CompareAndSwapGatewayStickySuccess(_ context.Context, groupID int64, session, model string, expected, next GatewayStickySuccessBinding, _ time.Duration) (bool, error) {
	key := buildLegacySuccessTestKey(groupID, session, model)
	if c.bindings[key] != expected {
		return false, nil
	}
	c.bindings[key] = next
	c.writes++
	return true, nil
}

func TestOpenAILegacyStickySuccessLifecycle(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
	cache := &openAILegacySuccessTestCache{bindings: map[string]GatewayStickySuccessBinding{}}
	svc := &OpenAIGatewayService{cache: cache}
	group := profitControlTestGroup(7, 0, 0)
	ctx, _ := svc.WithOpenAIRequestPricingContext(profitControlTestCtx(group), &group.ID)
	require.NoError(t, svc.BindStickySession(ctx, &group.ID, "session", 1))
	first := svc.BeginOpenAILegacyStickySuccess(ctx, &group.ID, "session", "alias-a")
	late := svc.BeginOpenAILegacyStickySuccess(ctx, &group.ID, "session", "alias-a")
	require.Same(t, first, svc.BeginOpenAILegacyStickySuccess(first, &group.ID, "session", "alias-a"))

	// Selection/admission and failed attempts must neither replace nor renew keys.
	require.NoError(t, svc.BindStickySessionAfterProfitAdmission(first, &group.ID, "session", 2))
	require.NoError(t, svc.refreshStickySessionTTL(first, &group.ID, "session", time.Hour))
	require.NoError(t, svc.deleteStickySessionAccountID(first, &group.ID, "session"))
	require.Zero(t, cache.writes)
	svc.CommitOpenAILegacyStickySuccess(first, &Account{ID: 2}, &OpenAIForwardResult{})
	svc.CommitOpenAILegacyStickySuccess(late, &Account{ID: 1}, &OpenAIForwardResult{})
	require.Equal(t, 1, cache.writes, "late completion must lose CAS")

	next := svc.BeginOpenAILegacyStickySuccess(ctx, &group.ID, "session", "alias-a")
	id, err := svc.getStickySessionAccountID(next, &group.ID, "session")
	require.NoError(t, err)
	require.EqualValues(t, 2, id)
	for _, result := range []*OpenAIForwardResult{nil, {ClientDisconnect: true}, {OpenAIWSMode: true, UpstreamTerminalEvent: "response.failed"}} {
		svc.CommitOpenAILegacyStickySuccess(next, &Account{ID: 1}, result)
	}
	canceled, cancel := context.WithCancel(next)
	cancel()
	svc.CommitOpenAILegacyStickySuccess(canceled, &Account{ID: 1}, &OpenAIForwardResult{})
	require.Equal(t, 1, cache.writes)
	svc.CommitOpenAILegacyStickySuccess(next, &Account{ID: 2}, &OpenAIForwardResult{})
	require.Equal(t, 2, cache.writes, "successful reuse renews the preference")

	alias := svc.BeginOpenAILegacyStickySuccess(ctx, &group.ID, "session", "alias-b")
	id, err = svc.getStickySessionAccountID(alias, &group.ID, "session")
	require.NoError(t, err)
	require.EqualValues(t, 1, id, "another client model keeps its own preference")
	id, err = svc.getStickySessionAccountID(ctx, &group.ID, "session")
	require.NoError(t, err)
	require.EqualValues(t, 1, id, "unmanaged protocols keep the original binding")
	_, managed := openAILegacyStickySuccessCandidate(next, nil, "session")
	require.False(t, managed, "another group must not inherit the preference")
}

// Arming only depends on the legacy scheduler being in charge. The profit gate
// no longer decides whether the state exists, only whether the legacy sticky
// key writes are taken over along with it.
func TestOpenAILegacyStickySuccessRequiresLegacySchedulerAndGatesLegacyWrites(t *testing.T) {
	for _, advanced := range []bool{false, true} {
		resetOpenAIAdvancedSchedulerSettingCacheForTest()
		svc := &OpenAIGatewayService{cache: &openAILegacySuccessTestCache{bindings: map[string]GatewayStickySuccessBinding{}}}
		if advanced {
			svc.rateLimitService = &RateLimitService{settingService: NewSettingService(&openAIAdvancedSchedulerSettingRepoStub{
				values: map[string]string{openAIAdvancedSchedulerSettingKey: "true"},
			}, nil)}
		}

		// No profit gate: the legacy scheduler still arms, candidate source only.
		ungated := openAILegacyStickySuccessFromContext(
			svc.BeginOpenAILegacyStickySuccess(context.Background(), nil, "session", "model"))
		if advanced {
			require.Nil(t, ungated, "the advanced scheduler owns its own preference")
		} else {
			require.NotNil(t, ungated, "groups without profit control must arm too")
			require.False(t, ungated.legacyWritesManaged, "legacy key writes stay untouched there")
			require.False(t, openAILegacyStickySuccessWritesManaged(
				context.WithValue(context.Background(), openAILegacyStickySuccessKey{}, ungated), nil, "session"))
		}

		group := profitControlTestGroup(7, 0, 0)
		ctx, _ := svc.WithOpenAIRequestPricingContext(profitControlTestCtx(group), &group.ID)
		state := openAILegacyStickySuccessFromContext(svc.BeginOpenAILegacyStickySuccess(ctx, &group.ID, "session", "model"))
		if advanced {
			require.Nil(t, state)
		} else {
			require.NotNil(t, state)
			require.True(t, state.legacyWritesManaged, "profit-controlled groups keep the managed legacy writes")
		}
	}
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
}

func TestOpenAILegacyStickySuccessSelectionRespectsEligibility(t *testing.T) {
	for _, loadBatch := range []bool{false, true} {
		t.Run(fmt.Sprintf("load_batch_%t", loadBatch), func(t *testing.T) {
			resetOpenAIAdvancedSchedulerSettingCacheForTest()
			t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
			cache := &openAILegacySuccessTestCache{bindings: map[string]GatewayStickySuccessBinding{}}
			accounts := make([]Account, 2)
			for i, rate := range []float64{0.8, 1} {
				a := upstreamCostTestAccount(int64(i+1), UpstreamBillingProbeStatusOK, rate, time.Now().Add(-time.Minute), time.Hour)
				a.RateMultiplier = &rate
				a.GroupIDs = []int64{7}
				a.Status, a.Schedulable, a.Concurrency, a.Priority = StatusActive, true, 4, 10-i
				accounts[i] = *a
			}
			cfg := &config.Config{}
			cfg.Gateway.Scheduling.LoadBatchEnabled = loadBatch
			svc := &OpenAIGatewayService{cache: cache, cfg: cfg,
				accountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts},
				rateLimitService: &RateLimitService{settingService: NewSettingService(&openAIAdvancedSchedulerSettingRepoStub{
					values: map[string]string{SettingKeyOpenAILowUpstreamRatePriorityEnabled: "true"},
				}, cfg)},
				concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
			}
			group := profitControlTestGroup(7, 0, 0)
			ctx, _ := svc.WithOpenAIRequestPricingContext(profitControlTestCtx(group), &group.ID)
			first := svc.BeginOpenAILegacyStickySuccess(ctx, &group.ID, "session", "model")
			selectID := func(ctx context.Context, excluded map[int64]struct{}) int64 {
				sel, _, err := svc.SelectAccountWithScheduler(ctx, &group.ID, "", "session", "model", excluded, OpenAIUpstreamTransportAny, false)
				require.NoError(t, err)
				if sel.ReleaseFunc != nil {
					sel.ReleaseFunc()
				}
				return sel.Account.ID
			}
			require.EqualValues(t, 1, selectID(first, nil))
			require.EqualValues(t, 2, selectID(first, map[int64]struct{}{1: {}}))
			svc.CommitOpenAILegacyStickySuccess(first, &accounts[1], &OpenAIForwardResult{})
			next := svc.BeginOpenAILegacyStickySuccess(ctx, &group.ID, "session", "model")
			require.EqualValues(t, 2, selectID(next, nil))
			tooExpensive := 1.2
			accounts[1].RateMultiplier = &tooExpensive
			require.EqualValues(t, 1, selectID(next, nil), "profit eligibility beats success preference")
			accounts[1].RateMultiplier = accounts[0].RateMultiplier
			accounts[1].Schedulable = false
			require.EqualValues(t, 1, selectID(next, nil), "administrative pause beats success preference")
		})
	}
}
