package service

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

type autoRouteCapacityGroupRepo struct {
	GroupRepository
	rows    []GroupAutoRouteAccountCapacity
	configs map[int64]GroupAutoRouteConfig
	groups  []Group
}

func (r *autoRouteCapacityGroupRepo) ListAutoRouteAccountCapacities(context.Context, []int64) ([]GroupAutoRouteAccountCapacity, error) {
	return append([]GroupAutoRouteAccountCapacity(nil), r.rows...), nil
}

func (r *autoRouteCapacityGroupRepo) GetAutoRouteConfigs(context.Context, []int64) (map[int64]GroupAutoRouteConfig, error) {
	return r.configs, nil
}

func (r *autoRouteCapacityGroupRepo) ListActiveByPlatform(context.Context, string) ([]Group, error) {
	return append([]Group(nil), r.groups...), nil
}

type autoRouteCapacityCache struct {
	ConcurrencyCache
	loads          map[int64]int
	pending        map[int64]int
	pendingMax     map[int64]int
	acquiredGroups []int64
	releaseCalls   int
}

type autoRouteAccountRepo struct {
	accounts map[int64][]Account
	errs     map[int64]error
}

func (r *autoRouteAccountRepo) ListSchedulableByGroupIDAndPlatform(_ context.Context, groupID int64, platform string) ([]Account, error) {
	if err := r.errs[groupID]; err != nil {
		return nil, err
	}
	accounts := r.accounts[groupID]
	result := make([]Account, 0, len(accounts))
	for i := range accounts {
		if accounts[i].Platform == platform {
			result = append(result, accounts[i])
		}
	}
	return result, nil
}

func autoRouteTestAccount(id int64, concurrency int, models ...string) Account {
	mapping := make(map[string]any, len(models))
	for _, model := range models {
		mapping[model] = model
	}
	return Account{
		ID:          id,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: concurrency,
		Credentials: map[string]any{"model_mapping": mapping},
	}
}

func (c *autoRouteCapacityCache) GetAccountConcurrencyBatch(_ context.Context, accountIDs []int64) (map[int64]int, error) {
	result := make(map[int64]int, len(accountIDs))
	for _, accountID := range accountIDs {
		result[accountID] = c.loads[accountID]
	}
	return result, nil
}

func (c *autoRouteCapacityCache) AcquireAutoRouteGroupSlot(_ context.Context, groupID int64, maxPending int, _ string) (bool, error) {
	if c.pending == nil {
		c.pending = make(map[int64]int)
	}
	if c.pendingMax == nil {
		c.pendingMax = make(map[int64]int)
	}
	c.pendingMax[groupID] = maxPending
	if c.pending[groupID] >= maxPending {
		return false, nil
	}
	c.pending[groupID]++
	c.acquiredGroups = append(c.acquiredGroups, groupID)
	return true, nil
}

func (c *autoRouteCapacityCache) ReleaseAutoRouteGroupSlot(_ context.Context, groupID int64, _ string) error {
	if c.pending[groupID] > 0 {
		c.pending[groupID]--
	}
	c.releaseCalls++
	return nil
}

func (c *autoRouteCapacityCache) AcquireAccountSlot(context.Context, int64, int, string) (bool, error) {
	return true, nil
}

func (c *autoRouteCapacityCache) ReleaseAccountSlot(context.Context, int64, string) error {
	return nil
}

func TestSelectAutoRouteTargetPrefersEffectiveUserRate(t *testing.T) {
	groups := []Group{
		{ID: 1, RateMultiplier: 0.5, SubscriptionType: SubscriptionTypeStandard, ActiveAccountCount: 1},
		{ID: 2, RateMultiplier: 0.8, SubscriptionType: SubscriptionTypeStandard, ActiveAccountCount: 1},
	}
	target := selectAutoRouteTarget(groups, map[int64]struct{}{1: {}, 2: {}}, map[int64]float64{1: 1.2, 2: 0.7}, false)
	require.NotNil(t, target)
	require.Equal(t, int64(2), target.ID)
}

func TestSelectAutoRouteTargetFallsBackAndSwitchesBack(t *testing.T) {
	groups := []Group{
		{ID: 1, RateMultiplier: 0.5, SubscriptionType: SubscriptionTypeStandard, ActiveAccountCount: 0},
		{ID: 2, RateMultiplier: 0.8, SubscriptionType: SubscriptionTypeStandard, ActiveAccountCount: 1},
	}
	allowed := map[int64]struct{}{1: {}, 2: {}}

	target := selectAutoRouteTarget(groups, allowed, nil, false)
	require.NotNil(t, target)
	require.Equal(t, int64(2), target.ID)

	groups[0].ActiveAccountCount = 1
	target = selectAutoRouteTarget(groups, allowed, nil, false)
	require.NotNil(t, target)
	require.Equal(t, int64(1), target.ID)
}

func TestSelectAutoRouteTargetUsesStableTieBreakAndIgnoresInvalidOverride(t *testing.T) {
	groups := []Group{
		{ID: 5, RateMultiplier: 0.5, SortOrder: 20, SubscriptionType: SubscriptionTypeStandard, ActiveAccountCount: 1},
		{ID: 3, RateMultiplier: 0.5, SortOrder: 10, SubscriptionType: SubscriptionTypeStandard, ActiveAccountCount: 1},
	}
	target := selectAutoRouteTarget(groups, map[int64]struct{}{3: {}, 5: {}}, map[int64]float64{3: math.NaN()}, false)
	require.NotNil(t, target)
	require.Equal(t, int64(3), target.ID)
}

func TestSelectAutoRouteTargetDoesNotExposeExclusiveTargetFromPublicEntry(t *testing.T) {
	groups := []Group{
		{ID: 1, RateMultiplier: 0.2, IsExclusive: true, SubscriptionType: SubscriptionTypeStandard, ActiveAccountCount: 1},
		{ID: 2, RateMultiplier: 0.8, SubscriptionType: SubscriptionTypeStandard, ActiveAccountCount: 1},
	}
	allowed := map[int64]struct{}{1: {}, 2: {}}

	publicTarget := selectAutoRouteTarget(groups, allowed, nil, false)
	require.NotNil(t, publicTarget)
	require.Equal(t, int64(2), publicTarget.ID)

	exclusiveTarget := selectAutoRouteTarget(groups, allowed, nil, true)
	require.NotNil(t, exclusiveTarget)
	require.Equal(t, int64(1), exclusiveTarget.ID)
}

func TestNormalizeAutoRouteGroupIDs(t *testing.T) {
	require.Equal(t, []int64{2, 3}, NormalizeAutoRouteGroupIDs(1, []int64{0, 2, 1, 2, -1, 3}))
}

func TestLoadAutoRouteGroupCapacitiesAggregatesRemainingSlots(t *testing.T) {
	repo := &autoRouteCapacityGroupRepo{rows: []GroupAutoRouteAccountCapacity{
		{GroupID: 1, AccountID: 11, MaxConcurrency: 2},
		{GroupID: 1, AccountID: 12, MaxConcurrency: 3},
		{GroupID: 1, AccountID: 12, MaxConcurrency: 3},
		{GroupID: 2, AccountID: 13, MaxConcurrency: 1},
		{GroupID: 3, AccountID: 14, MaxConcurrency: 0},
	}}
	cache := &autoRouteCapacityCache{loads: map[int64]int{11: 99, 12: 1, 13: 1, 14: 99}}
	svc := NewAPIKeyService(nil, nil, repo, nil, nil, nil, nil)
	svc.SetConcurrencyService(NewConcurrencyService(cache))

	capacities, enabled := svc.loadAutoRouteGroupCapacities(context.Background(), []int64{1, 2, 3})
	require.True(t, enabled)
	require.Equal(t, 2, capacities[1].remaining())
	require.Equal(t, 0, capacities[2].remaining())
	require.True(t, capacities[3].Unlimited)
	require.Greater(t, capacities[3].remaining(), 1_000_000)
}

func TestAutoRoutePendingCapacitySpillsAndReleasesOnRealAccountSlot(t *testing.T) {
	cache := &autoRouteCapacityCache{}
	svc := NewConcurrencyService(cache)

	reservation, acquired, err := svc.AcquireAutoRouteGroupSlot(context.Background(), 1, 1)
	require.NoError(t, err)
	require.True(t, acquired)
	require.NotNil(t, reservation)

	_, acquired, err = svc.AcquireAutoRouteGroupSlot(context.Background(), 1, 1)
	require.NoError(t, err)
	require.False(t, acquired)

	ctx := WithAutoRouteCapacityReservation(context.Background(), reservation)
	accountSlot, err := svc.AcquireAccountSlot(ctx, 11, 2)
	require.NoError(t, err)
	require.True(t, accountSlot.Acquired)
	require.Equal(t, 1, cache.releaseCalls)
	require.Equal(t, 0, cache.pending[int64(1)])

	_, acquired, err = svc.AcquireAutoRouteGroupSlot(context.Background(), 1, 1)
	require.NoError(t, err)
	require.True(t, acquired)
}

func TestResolveAutoRouteGroupCapacitySpillsConcurrentRequestsAndSwitchesBack(t *testing.T) {
	repo := &autoRouteCapacityGroupRepo{
		configs: map[int64]GroupAutoRouteConfig{99: {Enabled: true, GroupIDs: []int64{1, 2}}},
		groups: []Group{
			{ID: 1, Platform: PlatformOpenAI, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 0.5, ActiveAccountCount: 1},
			{ID: 2, Platform: PlatformOpenAI, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 0.8, ActiveAccountCount: 1},
		},
		rows: []GroupAutoRouteAccountCapacity{
			{GroupID: 1, AccountID: 11, MaxConcurrency: 1},
			{GroupID: 2, AccountID: 22, MaxConcurrency: 1},
		},
	}
	cache := &autoRouteCapacityCache{loads: map[int64]int{11: 0, 22: 0}}
	svc := NewAPIKeyService(nil, nil, repo, nil, nil, nil, nil)
	svc.SetConcurrencyService(NewConcurrencyService(cache))
	entryID := int64(99)
	apiKey := &APIKey{
		UserID:  7,
		GroupID: &entryID,
		Group: &Group{
			ID:               entryID,
			Platform:         PlatformOpenAI,
			SubscriptionType: SubscriptionTypeStandard,
		},
	}

	first, firstReservation, err := svc.ResolveAutoRouteGroupWithCapacity(context.Background(), apiKey)
	require.NoError(t, err)
	require.Equal(t, int64(1), first.Group.ID)
	require.NotNil(t, firstReservation)

	second, secondReservation, err := svc.ResolveAutoRouteGroupWithCapacity(context.Background(), apiKey)
	require.NoError(t, err)
	require.Equal(t, int64(2), second.Group.ID)
	require.NotNil(t, secondReservation)

	firstReservation.Release()
	third, thirdReservation, err := svc.ResolveAutoRouteGroupWithCapacity(context.Background(), apiKey)
	require.NoError(t, err)
	require.Equal(t, int64(1), third.Group.ID)
	thirdReservation.Release()
	secondReservation.Release()
}

func TestResolveAutoRouteFinalSelectionFiltersModelAfterRatePreselection(t *testing.T) {
	repo := &autoRouteCapacityGroupRepo{
		configs: map[int64]GroupAutoRouteConfig{99: {Enabled: true, GroupIDs: []int64{1, 2}}},
		groups: []Group{
			{ID: 1, Platform: PlatformOpenAI, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 0.5, ActiveAccountCount: 1},
			{ID: 2, Platform: PlatformOpenAI, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 0.8, ActiveAccountCount: 1},
		},
	}
	cache := &autoRouteCapacityCache{}
	svc := NewAPIKeyService(nil, nil, repo, nil, nil, nil, nil)
	svc.SetConcurrencyService(NewConcurrencyService(cache))
	svc.SetAccountRepository(&autoRouteAccountRepo{accounts: map[int64][]Account{
		1: {autoRouteTestAccount(11, 1, "gpt-low")},
		2: {autoRouteTestAccount(22, 1, "gpt-high")},
	}})
	entryID := int64(99)
	apiKey := &APIKey{UserID: 7, GroupID: &entryID, Group: &Group{
		ID: entryID, Platform: PlatformOpenAI, SubscriptionType: SubscriptionTypeStandard,
	}}

	preselected, err := svc.ResolveAutoRouteGroup(context.Background(), apiKey)
	require.NoError(t, err)
	require.Equal(t, int64(1), preselected.Group.ID)
	require.Empty(t, cache.acquiredGroups, "倍率初选不得预占容量")

	routed, reservation, err := svc.ResolveAutoRouteGroupWithCapacityForRequest(context.Background(), preselected, AutoRouteRequirements{
		RequestedModel: "gpt-high",
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), routed.Group.ID)
	require.NotNil(t, reservation)
	reservation.Release()
}

func TestResolveAutoRouteModelFilterSurvivesCandidateRepositoryFailure(t *testing.T) {
	repo := &autoRouteCapacityGroupRepo{
		configs: map[int64]GroupAutoRouteConfig{99: {Enabled: true, GroupIDs: []int64{1, 2}}},
		groups: []Group{
			{ID: 1, Platform: PlatformOpenAI, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 0.5, ActiveAccountCount: 1},
			{ID: 2, Platform: PlatformOpenAI, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 0.8, ActiveAccountCount: 1},
		},
	}
	svc := NewAPIKeyService(nil, nil, repo, nil, nil, nil, nil)
	svc.SetAccountRepository(&autoRouteAccountRepo{
		accounts: map[int64][]Account{2: {autoRouteTestAccount(22, 1, "gpt-high")}},
		errs:     map[int64]error{1: errors.New("temporary repository failure")},
	})
	entryID := int64(99)
	apiKey := &APIKey{UserID: 7, GroupID: &entryID, Group: &Group{
		ID: entryID, Platform: PlatformOpenAI, SubscriptionType: SubscriptionTypeStandard,
	}}

	routed, reservation, err := svc.ResolveAutoRouteGroupWithCapacityForRequest(context.Background(), apiKey, AutoRouteRequirements{
		RequestedModel: "gpt-high",
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), routed.Group.ID)
	require.Nil(t, reservation)
}

func TestLoadAutoRouteGroupCapacitiesForRequestDoesNotDoubleCountSharedAccount(t *testing.T) {
	cache := &autoRouteCapacityCache{}
	svc := NewAPIKeyService(nil, nil, nil, nil, nil, nil, nil)
	svc.SetConcurrencyService(NewConcurrencyService(cache))
	shared := autoRouteTestAccount(11, 2, "gpt-5")
	svc.SetAccountRepository(&autoRouteAccountRepo{accounts: map[int64][]Account{
		1: {shared},
		2: {shared, autoRouteTestAccount(22, 1, "gpt-5")},
	}})
	candidates := []Group{
		{ID: 1, RateMultiplier: 0.5},
		{ID: 2, RateMultiplier: 0.8},
	}

	capacities, enabled := svc.loadAutoRouteGroupCapacitiesForRequest(context.Background(), candidates, AutoRouteRequirements{RequestedModel: "gpt-5"})
	require.True(t, enabled)
	require.Equal(t, 2, capacities[1].Max)
	require.Equal(t, 0, capacities[2].Max, "含共享账号的后续分组必须整体跳过，避免调度器再次选中同一账号")
}
