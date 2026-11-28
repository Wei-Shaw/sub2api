//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func enableOpenAIAdvancedSchedulerForTest(t *testing.T) {
	t.Helper()
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
	openAIAdvancedSchedulerSettingCache.Store(&cachedOpenAIAdvancedSchedulerSetting{
		enabled:   true,
		expiresAt: time.Now().Add(time.Hour).UnixNano(),
	})
}

func openAIModelRoutingSnapshot(group *Group, accounts []Account) *SchedulerSnapshotService {
	ptrs := make([]*Account, 0, len(accounts))
	byID := make(map[int64]*Account, len(accounts))
	for i := range accounts {
		acc := accounts[i]
		cloned := acc
		ptrs = append(ptrs, &cloned)
		byID[acc.ID] = &cloned
	}
	return &SchedulerSnapshotService{
		cache: &openAISnapshotCacheStub{
			snapshotAccounts: ptrs,
			accountsByID:     byID,
		},
		groupRepo: profitControlGroupRepo{group: group},
	}
}

func TestOpenAIScheduler_ModelRoutingPrefersRulePriorityOverAccountPriority(t *testing.T) {
	enableOpenAIAdvancedSchedulerForTest(t)

	ctx := context.Background()
	groupID := int64(11001)
	accounts := []Account{
		{
			ID: 6, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
			Status: StatusActive, Schedulable: true, Concurrency: 5, Priority: 1, // would win without routing
			GroupIDs:    []int64{groupID},
			Credentials: map[string]any{"openai_capabilities": []any{"chat_completions"}},
		},
		{
			ID: 13, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
			Status: StatusActive, Schedulable: true, Concurrency: 5, Priority: 99, // worse global priority
			GroupIDs:    []int64{groupID},
			Credentials: map[string]any{"openai_capabilities": []any{"chat_completions"}},
		},
	}
	group := &Group{
		ID:                  groupID,
		Platform:            PlatformOpenAI,
		Status:              StatusActive,
		Hydrated:            true,
		ModelRoutingEnabled: true,
		ModelRouting: domain.ModelRoutingMap{
			"gpt-6-astra": {
				Accounts: []domain.ModelRoutingAccountRef{
					{ID: 13, Priority: 10, Concurrency: 0},
					{ID: 6, Priority: 50, Concurrency: 0},
				},
			},
		},
	}
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:              &schedulerTestGatewayCache{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
		schedulerSnapshot:  openAIModelRoutingSnapshot(group, accounts),
	}

	selection, decision, err := svc.SelectAccountWithSchedulerForCapability(
		ctx, &groupID, "", "", "gpt-6-astra", nil,
		OpenAIUpstreamTransportHTTPSSE, OpenAIEndpointCapabilityChatCompletions,
		false, false, true,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(13), selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerModelRouting, decision.Layer)
	require.True(t, selection.Acquired)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIScheduler_ModelRoutingFallsBackWhenPreferredExcluded(t *testing.T) {
	enableOpenAIAdvancedSchedulerForTest(t)

	ctx := context.Background()
	groupID := int64(11002)
	accounts := []Account{
		{
			ID: 6, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
			Status: StatusActive, Schedulable: true, Concurrency: 5, Priority: 1,
			GroupIDs:    []int64{groupID},
			Credentials: map[string]any{"openai_capabilities": []any{"chat_completions"}},
		},
		{
			ID: 13, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
			Status: StatusActive, Schedulable: true, Concurrency: 5, Priority: 99,
			GroupIDs:    []int64{groupID},
			Credentials: map[string]any{"openai_capabilities": []any{"chat_completions"}},
		},
	}
	group := &Group{
		ID:                  groupID,
		Platform:            PlatformOpenAI,
		Status:              StatusActive,
		Hydrated:            true,
		ModelRoutingEnabled: true,
		ModelRouting: domain.ModelRoutingMap{
			"gpt-6-*": {
				Accounts: []domain.ModelRoutingAccountRef{
					{ID: 13, Priority: 10},
				},
			},
		},
	}
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:              &schedulerTestGatewayCache{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
		schedulerSnapshot:  openAIModelRoutingSnapshot(group, accounts),
	}

	excluded := map[int64]struct{}{13: {}}
	selection, decision, err := svc.SelectAccountWithSchedulerForCapability(
		ctx, &groupID, "", "", "gpt-6-astra", excluded,
		OpenAIUpstreamTransportHTTPSSE, OpenAIEndpointCapabilityChatCompletions,
		false, false, true,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(6), selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIScheduler_ModelRoutingAccountConcurrencySkipsToNextPreferred(t *testing.T) {
	enableOpenAIAdvancedSchedulerForTest(t)

	ctx := context.Background()
	groupID := int64(11003)
	accounts := []Account{
		{
			ID: 13, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
			Status: StatusActive, Schedulable: true, Concurrency: 5, Priority: 50,
			GroupIDs:    []int64{groupID},
			Credentials: map[string]any{"openai_capabilities": []any{"chat_completions"}},
		},
		{
			ID: 6, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
			Status: StatusActive, Schedulable: true, Concurrency: 5, Priority: 50,
			GroupIDs:    []int64{groupID},
			Credentials: map[string]any{"openai_capabilities": []any{"chat_completions"}},
		},
	}
	group := &Group{
		ID:                  groupID,
		Platform:            PlatformOpenAI,
		Status:              StatusActive,
		Hydrated:            true,
		ModelRoutingEnabled: true,
		ModelRouting: domain.ModelRoutingMap{
			"gpt-6.1-sol": {
				Accounts: []domain.ModelRoutingAccountRef{
					{ID: 13, Priority: 10, Concurrency: 1},
					{ID: 6, Priority: 20, Concurrency: 0},
				},
			},
		},
	}

	routeCache := &stubModelRouteConcurrencyCache{
		stubConcurrencyCacheForTest: stubConcurrencyCacheForTest{acquireResult: true},
		total:                       0,
		account:                     map[int64]int{13: 1}, // already at route concurrency
	}
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:              &schedulerTestGatewayCache{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(routeCache),
		schedulerSnapshot:  openAIModelRoutingSnapshot(group, accounts),
	}

	selection, decision, err := svc.SelectAccountWithSchedulerForCapability(
		ctx, &groupID, "", "", "gpt-6.1-sol", nil,
		OpenAIUpstreamTransportHTTPSSE, OpenAIEndpointCapabilityChatCompletions,
		false, false, true,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(6), selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerModelRouting, decision.Layer)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIScheduler_LegacyPathAlsoHonorsModelRouting(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
	// Ensure advanced scheduler stays off.
	openAIAdvancedSchedulerSettingCache.Store(&cachedOpenAIAdvancedSchedulerSetting{
		enabled:   false,
		expiresAt: time.Now().Add(time.Hour).UnixNano(),
	})

	ctx := context.Background()
	groupID := int64(11004)
	accounts := []Account{
		{
			ID: 6, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
			Status: StatusActive, Schedulable: true, Concurrency: 5, Priority: 1,
			GroupIDs:    []int64{groupID},
			Credentials: map[string]any{"openai_capabilities": []any{"chat_completions"}},
		},
		{
			ID: 13, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
			Status: StatusActive, Schedulable: true, Concurrency: 5, Priority: 99,
			GroupIDs:    []int64{groupID},
			Credentials: map[string]any{"openai_capabilities": []any{"chat_completions"}},
		},
	}
	group := &Group{
		ID:                  groupID,
		Platform:            PlatformOpenAI,
		Status:              StatusActive,
		Hydrated:            true,
		ModelRoutingEnabled: true,
		ModelRouting: domain.ModelRoutingMap{
			"gpt-6-astra": {
				Accounts: []domain.ModelRoutingAccountRef{
					{ID: 13, Priority: 5},
				},
			},
		},
	}
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = false
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:              &schedulerTestGatewayCache{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
		schedulerSnapshot:  openAIModelRoutingSnapshot(group, accounts),
	}

	selection, decision, err := svc.SelectAccountWithSchedulerForCapability(
		ctx, &groupID, "", "", "gpt-6-astra", nil,
		OpenAIUpstreamTransportHTTPSSE, OpenAIEndpointCapabilityChatCompletions,
		false, false, true,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(13), selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerModelRouting, decision.Layer)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}
