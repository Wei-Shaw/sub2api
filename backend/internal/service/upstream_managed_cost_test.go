package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestBuildManagedCostTiersSortsByEffectiveRate(t *testing.T) {
	t.Parallel()

	accounts := []*Account{
		{ID: 1, Extra: map[string]any{managedUpstreamOwnerKey: managedUpstreamOwner, managedUpstreamEffectiveRateKey: 0.8}},
		{ID: 2, Extra: map[string]any{managedUpstreamOwnerKey: managedUpstreamOwner, managedUpstreamEffectiveRateKey: "0.4"}},
		{ID: 3, Extra: map[string]any{managedUpstreamOwnerKey: managedUpstreamOwner, managedUpstreamEffectiveRateKey: 0.4}},
	}

	tiers, ok := buildManagedCostTiers(accounts)
	require.True(t, ok)
	require.Len(t, tiers, 2)
	require.Equal(t, 0.4, tiers[0].rate)
	require.Equal(t, []int64{2, 3}, managedTierAccountIDs(tiers[0]))
	require.Equal(t, 0.8, tiers[1].rate)
	require.Equal(t, []int64{1}, managedTierAccountIDs(tiers[1]))
	require.Equal(t, 2*time.Second, managedCostTierWaitTimeout)
}

func TestManagedOpenAICostTiersWaitThenFallBack(t *testing.T) {
	low := &Account{
		ID: 101, Name: "low", Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-low", "model_mapping": map[string]any{"gpt-5": "gpt-5"}},
		Extra:       map[string]any{managedUpstreamOwnerKey: managedUpstreamOwner, managedUpstreamEffectiveRateKey: 0.4},
	}
	high := &Account{
		ID: 102, Name: "high", Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-high", "model_mapping": map[string]any{"gpt-5": "gpt-5"}},
		Extra:       map[string]any{managedUpstreamOwnerKey: managedUpstreamOwner, managedUpstreamEffectiveRateKey: 0.8},
	}
	tiers, ok := buildManagedCostTiers([]*Account{high, low})
	require.True(t, ok)

	cache := schedulerTestConcurrencyCache{acquireResults: map[int64]bool{low.ID: false, high.ID: true}}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 1
	gateway := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: []Account{*low, *high}},
		concurrencyService: NewConcurrencyService(cache),
		cfg:                cfg,
	}
	scheduler := &defaultOpenAIAccountScheduler{service: gateway, stats: newOpenAIAccountRuntimeStats()}

	started := time.Now()
	selection, _, _, _, err := scheduler.selectManagedOpenAICostTiers(context.Background(), OpenAIAccountScheduleRequest{
		Platform: PlatformOpenAI, RequestedModel: "gpt-5",
	}, tiers, map[int64]*AccountLoadInfo{})
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, high.ID, selection.Account.ID)
	require.GreaterOrEqual(t, time.Since(started), managedCostTierWaitTimeout)
}

func TestManagedGatewayCostTiersWaitThenFallBack(t *testing.T) {
	low := &Account{
		ID: 201, Name: "low", Platform: PlatformAnthropic, Status: StatusActive,
		Schedulable: true, Concurrency: 1,
		Extra: map[string]any{managedUpstreamOwnerKey: managedUpstreamOwner, managedUpstreamEffectiveRateKey: 0.4},
	}
	high := &Account{
		ID: 202, Name: "high", Platform: PlatformAnthropic, Status: StatusActive,
		Schedulable: true, Concurrency: 1,
		Extra: map[string]any{managedUpstreamOwnerKey: managedUpstreamOwner, managedUpstreamEffectiveRateKey: 0.8},
	}
	tiers, ok := buildManagedCostTiers([]*Account{high, low})
	require.True(t, ok)

	cache := schedulerTestConcurrencyCache{acquireResults: map[int64]bool{low.ID: false, high.ID: true}}
	gateway := &GatewayService{concurrencyService: NewConcurrencyService(cache)}

	started := time.Now()
	selection, _, err := gateway.selectManagedCostTiers(
		context.Background(), tiers, nil, "", false, gatewaySchedulingView{},
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, high.ID, selection.Account.ID)
	require.GreaterOrEqual(t, time.Since(started), managedCostTierWaitTimeout)
}

func TestBuildManagedCostTiersLeavesMixedManualPoolUntouched(t *testing.T) {
	t.Parallel()

	accounts := []*Account{
		{ID: 1, Extra: map[string]any{managedUpstreamOwnerKey: managedUpstreamOwner, managedUpstreamEffectiveRateKey: 0.4}},
		{ID: 2, Extra: map[string]any{"owner": "manual"}},
	}

	tiers, ok := buildManagedCostTiers(accounts)
	require.False(t, ok)
	require.Nil(t, tiers)
}

func managedTierAccountIDs(tier managedCostTier) []int64 {
	ids := make([]int64, 0, len(tier.accounts))
	for _, account := range tier.accounts {
		ids = append(ids, account.ID)
	}
	return ids
}
