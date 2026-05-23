//go:build unit

package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type billingDecisionLoaderStub struct {
	config *billingDecisionRuntimeConfig
	pool   *BillingPool
}

func (s billingDecisionLoaderStub) LoadAPIKeyBillingConfig(context.Context, int64) (*billingDecisionRuntimeConfig, error) {
	return s.config, nil
}

func (s billingDecisionLoaderStub) LoadBillingPool(context.Context, *int64, int64) (*BillingPool, error) {
	return s.pool, nil
}

type billingDecisionUserSubRepoStub struct {
	byGroup map[int64]*UserSubscription
}

func (s billingDecisionUserSubRepoStub) Create(context.Context, *UserSubscription) error {
	return errors.New("unexpected Create call")
}

func (s billingDecisionUserSubRepoStub) GetByID(context.Context, int64) (*UserSubscription, error) {
	return nil, errors.New("unexpected GetByID call")
}

func (s billingDecisionUserSubRepoStub) GetByUserIDAndGroupID(_ context.Context, _ int64, groupID int64) (*UserSubscription, error) {
	return s.GetActiveByUserIDAndGroupID(context.Background(), 0, groupID)
}

func (s billingDecisionUserSubRepoStub) GetActiveByUserIDAndGroupID(_ context.Context, _ int64, groupID int64) (*UserSubscription, error) {
	sub, ok := s.byGroup[groupID]
	if !ok || sub == nil {
		return nil, ErrSubscriptionNotFound
	}
	cp := *sub
	return &cp, nil
}

func (s billingDecisionUserSubRepoStub) Update(context.Context, *UserSubscription) error {
	return errors.New("unexpected Update call")
}

func (s billingDecisionUserSubRepoStub) Delete(context.Context, int64) error {
	return errors.New("unexpected Delete call")
}

func (s billingDecisionUserSubRepoStub) ListByUserID(context.Context, int64) ([]UserSubscription, error) {
	return nil, errors.New("unexpected ListByUserID call")
}

func (s billingDecisionUserSubRepoStub) ListActiveByUserID(context.Context, int64) ([]UserSubscription, error) {
	return nil, errors.New("unexpected ListActiveByUserID call")
}

func (s billingDecisionUserSubRepoStub) ListByGroupID(context.Context, int64, pagination.PaginationParams) ([]UserSubscription, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("unexpected ListByGroupID call")
}

func (s billingDecisionUserSubRepoStub) List(context.Context, pagination.PaginationParams, *int64, *int64, string, string, string, string) ([]UserSubscription, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("unexpected List call")
}

func (s billingDecisionUserSubRepoStub) ExistsByUserIDAndGroupID(context.Context, int64, int64) (bool, error) {
	return false, errors.New("unexpected ExistsByUserIDAndGroupID call")
}

func (s billingDecisionUserSubRepoStub) ExtendExpiry(context.Context, int64, time.Time) error {
	return errors.New("unexpected ExtendExpiry call")
}

func (s billingDecisionUserSubRepoStub) UpdateStatus(context.Context, int64, string) error {
	return errors.New("unexpected UpdateStatus call")
}

func (s billingDecisionUserSubRepoStub) UpdateNotes(context.Context, int64, string) error {
	return errors.New("unexpected UpdateNotes call")
}

func (s billingDecisionUserSubRepoStub) ActivateWindows(context.Context, int64, time.Time) error {
	return errors.New("unexpected ActivateWindows call")
}

func (s billingDecisionUserSubRepoStub) ResetDailyUsage(context.Context, int64, time.Time) error {
	return errors.New("unexpected ResetDailyUsage call")
}

func (s billingDecisionUserSubRepoStub) ResetWeeklyUsage(context.Context, int64, time.Time) error {
	return errors.New("unexpected ResetWeeklyUsage call")
}

func (s billingDecisionUserSubRepoStub) ResetMonthlyUsage(context.Context, int64, time.Time) error {
	return errors.New("unexpected ResetMonthlyUsage call")
}

func (s billingDecisionUserSubRepoStub) IncrementUsage(context.Context, int64, float64) error {
	return errors.New("unexpected IncrementUsage call")
}

func (s billingDecisionUserSubRepoStub) BatchUpdateExpiredStatus(context.Context) (int64, error) {
	return 0, errors.New("unexpected BatchUpdateExpiredStatus call")
}

func newBillingDecisionSubscription(id, userID, groupID int64, dailyUsage float64) *UserSubscription {
	return &UserSubscription{
		ID:            id,
		UserID:        userID,
		GroupID:       groupID,
		Status:        SubscriptionStatusActive,
		ExpiresAt:     time.Now().Add(24 * time.Hour),
		DailyUsageUSD: dailyUsage,
	}
}

func newBillingDecisionGroup(id int64, platform string, dailyLimit float64) *Group {
	return &Group{
		ID:               id,
		Platform:         platform,
		Status:           StatusActive,
		SubscriptionType: SubscriptionTypeSubscription,
		DailyLimitUSD:    &dailyLimit,
	}
}

func TestResolveBillingDecision_PrimaryAvailableUsesPrimarySubscription(t *testing.T) {
	t.Parallel()

	user := &User{ID: 1}
	billingPoolID := int64(499)
	primaryGroup := newBillingDecisionGroup(9, PlatformOpenAI, 10)
	apiKey := &APIKey{ID: 99, User: user, Group: primaryGroup, GroupID: &primaryGroup.ID}

	svc := NewSubscriptionService(nil, billingDecisionUserSubRepoStub{byGroup: map[int64]*UserSubscription{
		primaryGroup.ID: newBillingDecisionSubscription(999, user.ID, primaryGroup.ID, 2),
	}}, nil, nil, &config.Config{})
	svc.billingDecisionLoader = billingDecisionLoaderStub{
		config: &billingDecisionRuntimeConfig{
			BillingPoolID:       &billingPoolID,
			BillingMode:         APIKeyBillingModePrimaryThenPoolThenBalance,
			UsePoolDefaultOrder: true,
			Loaded:              true,
		},
	}

	decision, err := svc.ResolveBillingDecision(context.Background(), apiKey, ResolveBillingDecisionOptions{})
	require.NoError(t, err)
	require.NotNil(t, decision)
	require.Equal(t, BillingSourceTypeSubscription, decision.SourceType)
	require.NotNil(t, decision.Subscription)
	require.NotNil(t, decision.SubscriptionID)
	require.Equal(t, int64(999), *decision.SubscriptionID)
	require.Equal(t, primaryGroup.ID, decision.Subscription.GroupID)
	require.Equal(t, primaryGroup.ID, *decision.BillingGroupID)
	require.Equal(t, billingPoolID, *decision.BillingPoolID)
	require.Zero(t, decision.ChainIndex)
}

func TestResolveBillingDecision_PrimaryExhaustedFallsBackToPoolSubscription(t *testing.T) {
	t.Parallel()

	user := &User{ID: 1}
	primaryGroup := newBillingDecisionGroup(10, PlatformOpenAI, 10)
	fallbackGroup := newBillingDecisionGroup(11, PlatformOpenAI, 10)
	apiKey := &APIKey{ID: 100, User: user, Group: primaryGroup, GroupID: &primaryGroup.ID}

	svc := NewSubscriptionService(nil, billingDecisionUserSubRepoStub{byGroup: map[int64]*UserSubscription{
		primaryGroup.ID:  newBillingDecisionSubscription(1000, user.ID, primaryGroup.ID, 12),
		fallbackGroup.ID: newBillingDecisionSubscription(1001, user.ID, fallbackGroup.ID, 2),
	}}, nil, nil, &config.Config{})
	svc.billingDecisionLoader = billingDecisionLoaderStub{
		config: &billingDecisionRuntimeConfig{BillingMode: APIKeyBillingModePrimaryThenPoolThenBalance, UsePoolDefaultOrder: true, Loaded: true},
		pool: &BillingPool{
			ID:                   500,
			Status:               StatusActive,
			PlatformScope:        string(BillingPoolPlatformScopeSamePlatform),
			AllowBalanceFallback: true,
			Members: []BillingPoolMember{
				{GroupID: primaryGroup.ID, CanBePrimary: true, Group: primaryGroup},
				{GroupID: fallbackGroup.ID, CanBeFallback: true, Group: fallbackGroup},
			},
		},
	}

	decision, err := svc.ResolveBillingDecision(context.Background(), apiKey, ResolveBillingDecisionOptions{})
	require.NoError(t, err)
	require.NotNil(t, decision)
	require.Equal(t, BillingSourceTypeSubscription, decision.SourceType)
	require.NotNil(t, decision.Subscription)
	require.Equal(t, fallbackGroup.ID, decision.Subscription.GroupID)
	require.Equal(t, fallbackGroup.ID, *decision.BillingGroupID)
	require.Equal(t, int64(500), *decision.BillingPoolID)
	require.Equal(t, 1, decision.ChainIndex)
}

func TestResolveBillingDecision_SamePlatformExcludesCrossPlatformFallback(t *testing.T) {
	t.Parallel()

	user := &User{ID: 1, Balance: 8}
	primaryGroup := newBillingDecisionGroup(20, PlatformOpenAI, 10)
	crossPlatformGroup := newBillingDecisionGroup(21, PlatformAnthropic, 10)
	apiKey := &APIKey{ID: 200, User: user, Group: primaryGroup, GroupID: &primaryGroup.ID}

	svc := NewSubscriptionService(nil, billingDecisionUserSubRepoStub{byGroup: map[int64]*UserSubscription{
		primaryGroup.ID:       newBillingDecisionSubscription(2000, user.ID, primaryGroup.ID, 20),
		crossPlatformGroup.ID: newBillingDecisionSubscription(2001, user.ID, crossPlatformGroup.ID, 1),
	}}, nil, nil, &config.Config{})
	svc.billingDecisionLoader = billingDecisionLoaderStub{
		config: &billingDecisionRuntimeConfig{BillingMode: APIKeyBillingModePrimaryThenPoolThenBalance, UsePoolDefaultOrder: true, Loaded: true},
		pool: &BillingPool{
			ID:                   501,
			Status:               StatusActive,
			PlatformScope:        string(BillingPoolPlatformScopeSamePlatform),
			AllowBalanceFallback: true,
			Members: []BillingPoolMember{
				{GroupID: primaryGroup.ID, CanBePrimary: true, Group: primaryGroup},
				{GroupID: crossPlatformGroup.ID, CanBeFallback: true, Group: crossPlatformGroup},
			},
		},
	}

	decision, err := svc.ResolveBillingDecision(context.Background(), apiKey, ResolveBillingDecisionOptions{})
	require.NoError(t, err)
	require.NotNil(t, decision)
	require.Equal(t, BillingSourceTypeBalance, decision.SourceType)
	require.Nil(t, decision.Subscription)
	require.Equal(t, int64(501), *decision.BillingPoolID)
	require.Equal(t, 1, decision.ChainIndex)
}

func TestResolveBillingDecision_PoolExhaustedFallsBackToBalance(t *testing.T) {
	t.Parallel()

	user := &User{ID: 1, Balance: 6}
	primaryGroup := newBillingDecisionGroup(25, PlatformOpenAI, 10)
	fallbackGroup := newBillingDecisionGroup(26, PlatformOpenAI, 10)
	apiKey := &APIKey{ID: 250, User: user, Group: primaryGroup, GroupID: &primaryGroup.ID}

	svc := NewSubscriptionService(nil, billingDecisionUserSubRepoStub{byGroup: map[int64]*UserSubscription{
		primaryGroup.ID:  newBillingDecisionSubscription(2500, user.ID, primaryGroup.ID, 12),
		fallbackGroup.ID: newBillingDecisionSubscription(2501, user.ID, fallbackGroup.ID, 11),
	}}, nil, nil, &config.Config{})
	svc.billingDecisionLoader = billingDecisionLoaderStub{
		config: &billingDecisionRuntimeConfig{BillingMode: APIKeyBillingModePrimaryThenPoolThenBalance, UsePoolDefaultOrder: true, Loaded: true},
		pool: &BillingPool{
			ID:                   503,
			Status:               StatusActive,
			PlatformScope:        string(BillingPoolPlatformScopeSamePlatform),
			AllowBalanceFallback: true,
			Members: []BillingPoolMember{
				{GroupID: primaryGroup.ID, CanBePrimary: true, Group: primaryGroup},
				{GroupID: fallbackGroup.ID, CanBeFallback: true, Group: fallbackGroup},
			},
		},
	}

	decision, err := svc.ResolveBillingDecision(context.Background(), apiKey, ResolveBillingDecisionOptions{})
	require.NoError(t, err)
	require.NotNil(t, decision)
	require.Equal(t, BillingSourceTypeBalance, decision.SourceType)
	require.Nil(t, decision.Subscription)
	require.Nil(t, decision.SubscriptionID)
	require.Nil(t, decision.BillingGroupID)
	require.Equal(t, int64(503), *decision.BillingPoolID)
	require.Equal(t, 2, decision.ChainIndex)
}

func TestResolveBillingDecision_MissingPrimaryRejectsEvenWithFallback(t *testing.T) {
	t.Parallel()

	user := &User{ID: 1, Balance: 20}
	primaryGroup := newBillingDecisionGroup(30, PlatformOpenAI, 10)
	fallbackGroup := newBillingDecisionGroup(31, PlatformOpenAI, 10)
	apiKey := &APIKey{ID: 300, User: user, Group: primaryGroup, GroupID: &primaryGroup.ID}

	svc := NewSubscriptionService(nil, billingDecisionUserSubRepoStub{byGroup: map[int64]*UserSubscription{
		fallbackGroup.ID: newBillingDecisionSubscription(3001, user.ID, fallbackGroup.ID, 1),
	}}, nil, nil, &config.Config{})
	svc.billingDecisionLoader = billingDecisionLoaderStub{
		config: &billingDecisionRuntimeConfig{BillingMode: APIKeyBillingModePrimaryThenPoolThenBalance, UsePoolDefaultOrder: true, Loaded: true},
		pool: &BillingPool{
			ID:                   502,
			Status:               StatusActive,
			PlatformScope:        string(BillingPoolPlatformScopeMixedPlatform),
			AllowBalanceFallback: true,
			Members: []BillingPoolMember{
				{GroupID: primaryGroup.ID, CanBePrimary: true, Group: primaryGroup},
				{GroupID: fallbackGroup.ID, CanBeFallback: true, Group: fallbackGroup},
			},
		},
	}

	decision, err := svc.ResolveBillingDecision(context.Background(), apiKey, ResolveBillingDecisionOptions{})
	require.ErrorIs(t, err, ErrPrimarySubscriptionRequired)
	require.Nil(t, decision)
}

type billingDecisionCacheRecorder struct {
	mu          sync.Mutex
	userID      int64
	groupID     int64
	updatedCost float64
}

func (r *billingDecisionCacheRecorder) GetUserBalance(context.Context, int64) (float64, error) {
	return 0, errors.New("unexpected GetUserBalance call")
}

func (r *billingDecisionCacheRecorder) SetUserBalance(context.Context, int64, float64) error {
	return nil
}

func (r *billingDecisionCacheRecorder) DeductUserBalance(context.Context, int64, float64) error {
	return nil
}

func (r *billingDecisionCacheRecorder) InvalidateUserBalance(context.Context, int64) error {
	return nil
}

func (r *billingDecisionCacheRecorder) GetSubscriptionCache(context.Context, int64, int64) (*SubscriptionCacheData, error) {
	return nil, errors.New("unexpected GetSubscriptionCache call")
}

func (r *billingDecisionCacheRecorder) SetSubscriptionCache(context.Context, int64, int64, *SubscriptionCacheData) error {
	return nil
}

func (r *billingDecisionCacheRecorder) UpdateSubscriptionUsage(_ context.Context, userID, groupID int64, cost float64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.userID = userID
	r.groupID = groupID
	r.updatedCost = cost
	return nil
}

func (r *billingDecisionCacheRecorder) InvalidateSubscriptionCache(context.Context, int64, int64) error {
	return nil
}

func (r *billingDecisionCacheRecorder) GetAPIKeyRateLimit(context.Context, int64) (*APIKeyRateLimitCacheData, error) {
	return nil, errors.New("unexpected GetAPIKeyRateLimit call")
}

func (r *billingDecisionCacheRecorder) SetAPIKeyRateLimit(context.Context, int64, *APIKeyRateLimitCacheData) error {
	return nil
}

func (r *billingDecisionCacheRecorder) UpdateAPIKeyRateLimitUsage(context.Context, int64, float64) error {
	return nil
}

func (r *billingDecisionCacheRecorder) InvalidateAPIKeyRateLimit(context.Context, int64) error {
	return nil
}

func TestFinalizePostUsageBilling_UsesActualSubscriptionGroupID(t *testing.T) {
	t.Parallel()

	cache := &billingDecisionCacheRecorder{}
	billingCacheService := NewBillingCacheService(cache, nil, nil, nil, nil, nil, &config.Config{})
	t.Cleanup(billingCacheService.Stop)

	apiGroupID := int64(40)
	actualGroupID := int64(41)
	params := &postUsageBillingParams{
		Cost:               &CostBreakdown{ActualCost: 2.5},
		User:               &User{ID: 9},
		APIKey:             &APIKey{ID: 10, GroupID: &apiGroupID},
		Account:            &Account{ID: 11},
		Subscription:       &UserSubscription{ID: 12, GroupID: actualGroupID},
		IsSubscriptionBill: true,
	}

	finalizePostUsageBilling(params, &billingDeps{
		billingCacheService: billingCacheService,
		deferredService:     &DeferredService{},
	}, &UsageBillingApplyResult{Applied: true})

	require.Eventually(t, func() bool {
		cache.mu.Lock()
		defer cache.mu.Unlock()
		return cache.groupID == actualGroupID && cache.updatedCost == 2.5 && cache.userID == 9
	}, 2*time.Second, 10*time.Millisecond)
}
