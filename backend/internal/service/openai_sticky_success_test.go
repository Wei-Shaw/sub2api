package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type stickySuccessTestCache struct {
	schedulerTestGatewayCache
	bindings map[string]OpenAIStickySuccessBinding
}

func (c *stickySuccessTestCache) GetOpenAIStickySuccess(_ context.Context, group int64, session, model string) (OpenAIStickySuccessBinding, error) {
	binding, ok := c.bindings[fmt.Sprintf("%d/%s/%s", group, session, model)]
	if !ok {
		return binding, ErrStickySessionNotFound
	}
	return binding, nil
}

func (c *stickySuccessTestCache) CompareAndSwapOpenAIStickySuccess(_ context.Context, group int64, session, model string, expected, next OpenAIStickySuccessBinding, _ time.Duration) (bool, error) {
	key := fmt.Sprintf("%d/%s/%s", group, session, model)
	if c.bindings[key] != expected {
		return false, nil
	}
	c.bindings[key] = next
	return true, nil
}

func TestOpenAIStickySuccessRebindAndPolicy(t *testing.T) {
	for _, policyVeto := range []bool{false, true} {
		t.Run(fmt.Sprintf("policy_veto_%t", policyVeto), func(t *testing.T) {
			cache := &stickySuccessTestCache{
				schedulerTestGatewayCache: schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:session": 1}},
				bindings:                  map[string]OpenAIStickySuccessBinding{},
			}
			rate := 0.2
			accounts := []Account{
				{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, GroupIDs: []int64{81}, RateMultiplier: &rate},
				{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, GroupIDs: []int64{81}, RateMultiplier: &rate},
			}
			if policyVeto {
				expensive := 0.9
				accounts[0].RateMultiplier = &expensive
			}
			svc := &OpenAIGatewayService{
				cache: cache, accountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts},
				rateLimitService: newOpenAIAdvancedSchedulerRateLimitService("true", "true"),
			}
			t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
			group := profitControlTestGroup(81, 0.5, 0)
			base, _ := svc.WithOpenAIRequestPricingContext(profitControlTestCtx(group), &group.ID)
			first := svc.BeginOpenAIStickySuccess(base, &group.ID, "session", "gpt-test")
			require.NotNil(t, openAIStickySuccessFromContext(first))
			selected, _, err := svc.SelectAccountWithScheduler(first, &group.ID, "", "session", "gpt-test", map[int64]struct{}{1: {}}, OpenAIUpstreamTransportHTTPSSE, false)
			require.NoError(t, err)
			require.Equal(t, int64(2), selected.Account.ID)
			svc.CommitOpenAIStickySuccess(first, selected.Account, &OpenAIForwardResult{})
			require.Equal(t, int64(1), cache.sessionBindings["openai:session"], "legacy policy binding stays intact")
			next := svc.BeginOpenAIStickySuccess(base, &group.ID, "session", "gpt-test")
			state := openAIStickySuccessFromContext(next)
			if policyVeto {
				require.Zero(t, state.expected.AccountID, "policy-only fallback must not acquire success preference")
				return
			}
			require.Equal(t, int64(2), state.originalID)
			selected, _, err = svc.SelectAccountWithScheduler(next, &group.ID, "", "session", "gpt-test", nil, OpenAIUpstreamTransportHTTPSSE, false)
			require.NoError(t, err)
			require.Equal(t, int64(2), selected.Account.ID, "healthy success must be preferred on the next request")
			accounts[1].Schedulable = false
			selected, _, err = svc.SelectAccountWithScheduler(next, &group.ID, "", "session", "gpt-test", nil, OpenAIUpstreamTransportHTTPSSE, false)
			require.NoError(t, err)
			require.Equal(t, int64(1), selected.Account.ID, "success preference cannot bypass administrative suspension")
			other := svc.BeginOpenAIStickySuccess(base, &group.ID, "session", "other-model")
			require.Equal(t, int64(1), openAIStickySuccessFromContext(other).originalID)
		})
	}
}

func TestOpenAIStickySuccessOnlyCommitsSuccessfulConnectedResults(t *testing.T) {
	cache := &stickySuccessTestCache{bindings: map[string]OpenAIStickySuccessBinding{}}
	svc := &OpenAIGatewayService{cache: cache}
	state := &openAIStickySuccessState{groupID: 1, sessionHash: "s", model: "m"}
	ctx := context.WithValue(context.Background(), openAIStickySuccessKey{}, state)
	account := &Account{ID: 2}
	for _, result := range []*OpenAIForwardResult{nil, {ClientDisconnect: true}, {OpenAIWSMode: true, UpstreamTerminalEvent: "response.failed"}} {
		svc.CommitOpenAIStickySuccess(ctx, account, result)
		require.Empty(t, cache.bindings)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	svc.CommitOpenAIStickySuccess(canceled, account, &OpenAIForwardResult{})
	require.Empty(t, cache.bindings)
	svc.CommitOpenAIStickySuccess(ctx, account, &OpenAIForwardResult{})
	current := cache.bindings["1/s/m"]
	require.Equal(t, int64(2), current.AccountID)
	svc.CommitOpenAIStickySuccess(ctx, &Account{ID: 3}, &OpenAIForwardResult{})
	require.Equal(t, current, cache.bindings["1/s/m"], "late completion cannot overwrite a newer success")
}
