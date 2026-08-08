package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func fallbackStickyTestAccounts(groupID int64) []Account {
	return []Account{
		{
			ID:                      91001,
			Platform:                PlatformOpenAI,
			Type:                    AccountTypeOAuth,
			Status:                  StatusActive,
			Schedulable:             true,
			Concurrency:             1,
			Priority:                200,
			GroupIDs:                []int64{groupID},
			Credentials:             map[string]any{"model_mapping": map[string]any{"gpt-ready": "gpt-ready"}},
			Extra:                   map[string]any{"openai_oauth_responses_websockets_v2_enabled": true},
			OpenAISessionStickyMode: OpenAISessionStickyModeFallbackOnly,
		},
		{
			ID:                      91002,
			Platform:                PlatformOpenAI,
			Type:                    AccountTypeOAuth,
			Status:                  StatusActive,
			Schedulable:             true,
			Concurrency:             1,
			Priority:                1,
			GroupIDs:                []int64{groupID},
			Credentials:             map[string]any{"model_mapping": map[string]any{"gpt-ready": "gpt-ready"}},
			OpenAISessionStickyMode: OpenAISessionStickyModeNormal,
		},
	}
}

func fallbackStickyTestAccountsWithTwoPrimaries(groupID int64) []Account {
	accounts := fallbackStickyTestAccounts(groupID)
	secondPrimary := accounts[1]
	secondPrimary.ID = 91003
	secondPrimary.Priority = 2
	return append(accounts, secondPrimary)
}

func TestFallbackOnlyStickyEscapesToImmediatelyAvailableHigherPriorityAccount(t *testing.T) {
	ctx := context.Background()
	groupID := int64(9101)
	accounts := fallbackStickyTestAccounts(groupID)
	cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:fallback-session": accounts[0].ID}}
	svc := &OpenAIGatewayService{
		accountRepo: schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: accounts}},
		cache:       cache,
	}

	selected, err := svc.SelectAccountForModel(ctx, &groupID, "fallback-session", "gpt-ready")

	require.NoError(t, err)
	require.Equal(t, accounts[1].ID, selected.ID)
	require.Equal(t, accounts[1].ID, cache.sessionBindings["openai:fallback-session"])
}

func TestNormalStickyKeepsExistingAccountEvenWhenHigherPriorityAccountIsAvailable(t *testing.T) {
	ctx := context.Background()
	groupID := int64(9102)
	accounts := fallbackStickyTestAccounts(groupID)
	accounts[0].OpenAISessionStickyMode = OpenAISessionStickyModeNormal
	cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:normal-session": accounts[0].ID}}
	svc := &OpenAIGatewayService{
		accountRepo: schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: accounts}},
		cache:       cache,
	}

	selected, err := svc.SelectAccountForModel(ctx, &groupID, "normal-session", "gpt-ready")

	require.NoError(t, err)
	require.Equal(t, accounts[0].ID, selected.ID)
}

func TestFallbackOnlyStickyKeepsAccountWhenHigherPriorityCandidateCannotServeRequest(t *testing.T) {
	ctx := context.Background()
	groupID := int64(9103)
	accounts := fallbackStickyTestAccounts(groupID)
	accounts[1].Credentials = map[string]any{"model_mapping": map[string]any{"other-model": "other-model"}}
	cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:unsupported-session": accounts[0].ID}}
	svc := &OpenAIGatewayService{
		accountRepo: schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: accounts}},
		cache:       cache,
	}

	selected, err := svc.SelectAccountForModel(ctx, &groupID, "unsupported-session", "gpt-ready")

	require.NoError(t, err)
	require.Equal(t, accounts[0].ID, selected.ID)
}

func TestFallbackOnlyStickyKeepsAccountWhenHigherPrioritySlotIsFull(t *testing.T) {
	ctx := context.Background()
	groupID := int64(9104)
	accounts := fallbackStickyTestAccounts(groupID)
	cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:full-session": accounts[0].ID}}
	concurrency := schedulerTestConcurrencyCache{loadMap: map[int64]*AccountLoadInfo{
		accounts[1].ID: {AccountID: accounts[1].ID, CurrentConcurrency: accounts[1].Concurrency, LoadRate: 100},
	}}
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: accounts}},
		cache:              cache,
		concurrencyService: NewConcurrencyService(concurrency),
	}

	selected, err := svc.SelectAccountForModel(ctx, &groupID, "full-session", "gpt-ready")

	require.NoError(t, err)
	require.Equal(t, accounts[0].ID, selected.ID)
}

func TestFallbackOnlyStickyRecoversWhenHigherPrioritySlotIsTakenAfterProbe(t *testing.T) {
	ctx := context.Background()
	groupID := int64(9105)
	accounts := fallbackStickyTestAccounts(groupID)
	cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:racy-session": accounts[0].ID}}
	var acquiredIDs []int64
	concurrency := schedulerTestConcurrencyCache{
		acquiredIDs: &acquiredIDs,
		loadMap: map[int64]*AccountLoadInfo{
			accounts[1].ID: {AccountID: accounts[1].ID, CurrentConcurrency: 0},
		},
		acquireResults: map[int64]bool{
			accounts[1].ID: false,
			accounts[0].ID: true,
		},
	}
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = false
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: accounts}},
		cache:              cache,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(concurrency),
	}

	selection, err := svc.SelectAccountWithLoadAwareness(ctx, &groupID, "racy-session", "gpt-ready", nil)

	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, accounts[0].ID, selection.Account.ID)
	require.True(t, selection.Acquired)
	require.Equal(t, []int64{accounts[1].ID, accounts[0].ID}, acquiredIDs)
	require.Equal(t, accounts[0].ID, cache.sessionBindings["openai:racy-session"])
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestFallbackOnlyStickyRecoversAfterAllHigherPrioritySlotsRaceFull(t *testing.T) {
	ctx := context.Background()
	groupID := int64(9109)
	accounts := fallbackStickyTestAccountsWithTwoPrimaries(groupID)
	cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:multi-racy-session": accounts[0].ID}}
	var acquiredIDs []int64
	concurrency := schedulerTestConcurrencyCache{
		acquiredIDs: &acquiredIDs,
		loadMap: map[int64]*AccountLoadInfo{
			accounts[1].ID: {AccountID: accounts[1].ID, CurrentConcurrency: 0},
			accounts[2].ID: {AccountID: accounts[2].ID, CurrentConcurrency: 0},
		},
		acquireResults: map[int64]bool{
			accounts[1].ID: false,
			accounts[2].ID: false,
			accounts[0].ID: true,
		},
	}
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = false
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: accounts}},
		cache:              cache,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(concurrency),
	}

	selection, err := svc.SelectAccountWithLoadAwareness(ctx, &groupID, "multi-racy-session", "gpt-ready", nil)

	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, accounts[0].ID, selection.Account.ID)
	require.True(t, selection.Acquired)
	require.Equal(t, []int64{accounts[1].ID, accounts[2].ID, accounts[0].ID}, acquiredIDs)
	require.Equal(t, accounts[0].ID, cache.sessionBindings["openai:multi-racy-session"])
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestFallbackOnlyStickyEscapesInAdvancedScheduler(t *testing.T) {
	ctx := context.Background()
	groupID := int64(9106)
	accounts := fallbackStickyTestAccounts(groupID)
	cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:advanced-session": accounts[0].ID}}
	cfg := &config.Config{}
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: accounts}},
		cache:              cache,
		cfg:                cfg,
		rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("true"),
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}

	selection, decision, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "advanced-session", "gpt-ready", nil, OpenAIUpstreamTransportAny, false)

	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, accounts[1].ID, selection.Account.ID)
	require.False(t, decision.StickySessionHit)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestFallbackOnlyStickyAdvancedSchedulerRecoversWhenHigherPrioritySlotIsTakenAfterProbe(t *testing.T) {
	ctx := context.Background()
	groupID := int64(9107)
	accounts := fallbackStickyTestAccounts(groupID)
	cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:advanced-racy-session": accounts[0].ID}}
	concurrency := schedulerTestConcurrencyCache{
		loadMap: map[int64]*AccountLoadInfo{
			accounts[1].ID: {AccountID: accounts[1].ID, CurrentConcurrency: 0},
		},
		acquireResults: map[int64]bool{
			accounts[1].ID: false,
			accounts[0].ID: true,
		},
	}
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: accounts}},
		cache:              cache,
		cfg:                &config.Config{},
		rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("true"),
		concurrencyService: NewConcurrencyService(concurrency),
	}

	selection, _, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "advanced-racy-session", "gpt-ready", nil, OpenAIUpstreamTransportAny, false)

	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, accounts[0].ID, selection.Account.ID)
	require.True(t, selection.Acquired)
	require.Equal(t, accounts[0].ID, cache.sessionBindings["openai:advanced-racy-session"])
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestFallbackOnlyStickyAdvancedSchedulerRecoversAfterAllHigherPrioritySlotsRaceFull(t *testing.T) {
	ctx := context.Background()
	groupID := int64(9110)
	accounts := fallbackStickyTestAccountsWithTwoPrimaries(groupID)
	cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:advanced-multi-racy-session": accounts[0].ID}}
	var acquiredIDs []int64
	concurrency := schedulerTestConcurrencyCache{
		acquiredIDs: &acquiredIDs,
		loadMap: map[int64]*AccountLoadInfo{
			accounts[1].ID: {AccountID: accounts[1].ID, CurrentConcurrency: 0},
			accounts[2].ID: {AccountID: accounts[2].ID, CurrentConcurrency: 0},
		},
		acquireResults: map[int64]bool{
			accounts[1].ID: false,
			accounts[2].ID: false,
			accounts[0].ID: true,
		},
	}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 2
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: accounts}},
		cache:              cache,
		cfg:                cfg,
		rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("true"),
		concurrencyService: NewConcurrencyService(concurrency),
	}

	selection, _, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "advanced-multi-racy-session", "gpt-ready", nil, OpenAIUpstreamTransportAny, false)

	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, accounts[0].ID, selection.Account.ID)
	require.True(t, selection.Acquired)
	require.Equal(t, []int64{accounts[1].ID, accounts[2].ID, accounts[0].ID}, acquiredIDs)
	require.Equal(t, accounts[0].ID, cache.sessionBindings["openai:advanced-multi-racy-session"])
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestFallbackOnlyStickyAdvancedSchedulerSkipsImageIncompatibleHigherPriorityAccount(t *testing.T) {
	ctx := context.Background()
	groupID := int64(9111)
	accounts := fallbackStickyTestAccounts(groupID)
	accounts[1].Type = AccountTypeSetupToken
	cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:image-capability-session": accounts[0].ID}}
	var acquiredIDs []int64
	svc := &OpenAIGatewayService{
		accountRepo:      schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: accounts}},
		cache:            cache,
		cfg:              &config.Config{},
		rateLimitService: newOpenAIAdvancedSchedulerRateLimitService("true"),
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{
			acquiredIDs: &acquiredIDs,
		}),
	}

	selection, _, err := svc.SelectAccountWithSchedulerForImages(ctx, &groupID, "image-capability-session", "gpt-ready", nil, OpenAIImagesCapabilityBasic)

	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, accounts[0].ID, selection.Account.ID)
	require.Equal(t, []int64{accounts[0].ID}, acquiredIDs)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestFallbackOnlyStickyAdvancedSchedulerContinuesAfterHigherPriorityAcquireError(t *testing.T) {
	ctx := context.Background()
	groupID := int64(9112)
	accounts := fallbackStickyTestAccountsWithTwoPrimaries(groupID)
	cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:acquire-error-session": accounts[0].ID}}
	var acquiredIDs []int64
	concurrency := schedulerTestConcurrencyCache{
		acquiredIDs: &acquiredIDs,
		acquireErrors: map[int64]error{
			accounts[1].ID: errors.New("temporary acquire failure"),
		},
		acquireResults: map[int64]bool{
			accounts[2].ID: false,
			accounts[0].ID: true,
		},
	}
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: accounts}},
		cache:              cache,
		cfg:                &config.Config{},
		rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("true"),
		concurrencyService: NewConcurrencyService(concurrency),
	}

	selection, _, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "acquire-error-session", "gpt-ready", nil, OpenAIUpstreamTransportAny, false)

	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, accounts[0].ID, selection.Account.ID)
	require.True(t, selection.Acquired)
	require.Equal(t, []int64{accounts[1].ID, accounts[2].ID, accounts[0].ID}, acquiredIDs)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestFallbackOnlyStickyEscapesWeightedAdvancedScheduler(t *testing.T) {
	ctx := context.Background()
	groupID := int64(9108)
	accounts := fallbackStickyTestAccounts(groupID)
	cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:weighted-session": accounts[0].ID}}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 2
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Priority = 1
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: accounts}},
		cache:              cache,
		cfg:                cfg,
		rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("true", "true"),
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}

	selection, decision, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "weighted-session", "gpt-ready", nil, OpenAIUpstreamTransportAny, false)

	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, accounts[1].ID, selection.Account.ID)
	require.False(t, decision.StickySessionHit)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}
