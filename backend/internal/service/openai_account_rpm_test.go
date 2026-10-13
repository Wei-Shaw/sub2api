package service

import (
	"context"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// openAIRPMCacheStub is an in-memory RPMCache: counts[accountID] is the current-minute count.
type openAIRPMCacheStub struct {
	mu     sync.Mutex
	counts map[int64]int
}

func (c *openAIRPMCacheStub) IncrementRPM(_ context.Context, accountID int64) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.counts == nil {
		c.counts = map[int64]int{}
	}
	c.counts[accountID]++
	return c.counts[accountID], nil
}

func (c *openAIRPMCacheStub) GetRPM(_ context.Context, accountID int64) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.counts[accountID], nil
}

func (c *openAIRPMCacheStub) GetRPMBatch(_ context.Context, accountIDs []int64) (map[int64]int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[int64]int, len(accountIDs))
	for _, id := range accountIDs {
		out[id] = c.counts[id]
	}
	return out, nil
}

func (c *openAIRPMCacheStub) count(accountID int64) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.counts[accountID]
}

func rpmTestAPIKeyAccount(id int64, platform string, priority int, extra map[string]any) Account {
	return Account{
		ID:          id,
		Platform:    platform,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 4,
		Priority:    priority,
		Extra:       extra,
	}
}

func TestOpenAIAccountSchedulableForRPM(t *testing.T) {
	ctx := context.Background()
	limited := rpmTestAPIKeyAccount(1, PlatformMiniMax, 0, map[string]any{"base_rpm": 10, "rpm_sticky_buffer": 2})
	exempt := rpmTestAPIKeyAccount(2, PlatformMiniMax, 0, map[string]any{"base_rpm": 10, "rpm_strategy": "sticky_exempt"})
	unlimited := rpmTestAPIKeyAccount(3, PlatformMiniMax, 0, nil)

	cases := []struct {
		name    string
		account Account
		current int
		sticky  bool
		want    bool
	}{
		{"no limit", unlimited, 1000, false, true},
		{"below limit", limited, 9, false, true},
		{"at limit, new request", limited, 10, false, false},
		{"at limit, sticky within buffer", limited, 11, true, true},
		{"beyond buffer, sticky", limited, 12, true, false},
		{"sticky_exempt at limit, sticky", exempt, 50, true, true},
		{"sticky_exempt at limit, new request", exempt, 10, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &OpenAIGatewayService{rpmCache: &openAIRPMCacheStub{counts: map[int64]int{tc.account.ID: tc.current}}}
			require.Equal(t, tc.want, svc.isOpenAIAccountSchedulableForRPM(ctx, &tc.account, tc.sticky, nil))
			prefetched := svc.openAIRPMCounts(ctx, []Account{tc.account})
			require.Equal(t, tc.want, svc.isOpenAIAccountSchedulableForRPM(ctx, &tc.account, tc.sticky, prefetched))
		})
	}

	// Without a cache the gate fails open.
	require.True(t, (&OpenAIGatewayService{}).isOpenAIAccountSchedulableForRPM(ctx, &limited, false, nil))
}

// An API-key account of an OpenAI-compatible platform that used up its RPM is
// skipped by the legacy load-aware selector, and the chosen account is counted.
func TestOpenAIGatewayService_SelectAccountWithScheduler_LegacySkipsRPMFullAccount(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	ctx := context.Background()
	groupID := int64(42001)
	accounts := []Account{
		rpmTestAPIKeyAccount(42011, PlatformOpenAI, 0, map[string]any{"base_rpm": 5}), // preferred, but full
		rpmTestAPIKeyAccount(42012, PlatformOpenAI, 5, map[string]any{"base_rpm": 5}),
	}
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = false
	rpm := &openAIRPMCacheStub{counts: map[int64]int{42011: 5}}
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:              &schedulerTestGatewayCache{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}
	svc.SetRPMCache(rpm)

	selection, _, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "", "gpt-5.1", nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, int64(42012), selection.Account.ID)
	require.Equal(t, 1, rpm.count(42012))
	require.Equal(t, 5, rpm.count(42011))
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

// Same for the advanced scheduler's load-balance layer.
func TestOpenAIGatewayService_SelectAccountWithScheduler_AdvancedSkipsRPMFullAccount(t *testing.T) {
	ctx := context.Background()
	groupID := int64(42002)
	accounts := []Account{
		rpmTestAPIKeyAccount(42021, PlatformOpenAI, 0, map[string]any{"base_rpm": 3}),
		rpmTestAPIKeyAccount(42022, PlatformOpenAI, 0, map[string]any{"base_rpm": 3}),
	}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 2
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Priority = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	concurrencyCache := schedulerTestConcurrencyCache{
		loadMap: map[int64]*AccountLoadInfo{
			42021: {AccountID: 42021, LoadRate: 0},  // idle, would win on load
			42022: {AccountID: 42022, LoadRate: 50}, // busier
		},
		acquireResults: map[int64]bool{42021: true, 42022: true},
	}
	rpm := &openAIRPMCacheStub{counts: map[int64]int{42021: 3}}
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:              &schedulerTestGatewayCache{},
		cfg:                cfg,
		rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("true"),
		concurrencyService: NewConcurrencyService(concurrencyCache),
	}
	svc.SetRPMCache(rpm)

	selection, decision, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "", "gpt-5.1", nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, int64(42022), selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
	require.Equal(t, 1, rpm.count(42022))
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}

	// When every account is at its limit, selection reports the RPM exclusion.
	rpm.counts[42022] = 3
	_, _, err = svc.SelectAccountWithScheduler(ctx, &groupID, "", "", "gpt-5.1", nil, OpenAIUpstreamTransportAny, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "rpm_limited")
}
