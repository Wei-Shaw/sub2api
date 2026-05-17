package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

type entitlementUserRepoStub struct {
	users map[int64]*User
}

func (s *entitlementUserRepoStub) GetByID(_ context.Context, id int64) (*User, error) {
	return s.users[id], nil
}

type entitlementGroupRepoStub struct {
	groups map[int64]*Group
}

func (s *entitlementGroupRepoStub) GetByID(_ context.Context, id int64) (*Group, error) {
	return s.groups[id], nil
}

type entitlementAPIKeyRepoStub struct {
	keys []APIKey
}

func (s *entitlementAPIKeyRepoStub) ListByUserID(_ context.Context, userID int64, _ pagination.PaginationParams, _ APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
	out := make([]APIKey, 0, len(s.keys))
	for _, key := range s.keys {
		if key.UserID == userID {
			out = append(out, key)
		}
	}
	return out, &pagination.PaginationResult{Total: int64(len(out))}, nil
}

type entitlementAPIKeyUpdaterStub struct {
	updatedID     int64
	updatedUserID int64
	updatedGroup  *int64
	updatedKey    *APIKey
}

func (s *entitlementAPIKeyUpdaterStub) Update(_ context.Context, id, userID int64, req UpdateAPIKeyRequest) (*APIKey, error) {
	s.updatedID = id
	s.updatedUserID = userID
	s.updatedGroup = req.GroupID
	key := &APIKey{ID: id, UserID: userID, Status: StatusActive, GroupID: req.GroupID}
	s.updatedKey = key
	return key, nil
}

type entitlementUserSubRepoStub struct {
	subs []UserSubscription
}

func (s *entitlementUserSubRepoStub) ListByUserID(_ context.Context, userID int64) ([]UserSubscription, error) {
	out := make([]UserSubscription, 0, len(s.subs))
	for _, sub := range s.subs {
		if sub.UserID == userID {
			out = append(out, sub)
		}
	}
	return out, nil
}

type entitlementUsageRepoStub struct {
	summary *usagestats.CreditUsageSummary
	err     error
	calls   int
}

func (s *entitlementUsageRepoStub) GetUserCreditUsageSummary(_ context.Context, _ int64) (*usagestats.CreditUsageSummary, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.summary, nil
}

func TestEntitlementService_GetUserEntitlements_IncludesSubscriptionAndBalanceFallback(t *testing.T) {
	now := time.Now()
	later := now.Add(30 * 24 * time.Hour)
	monthlyLimit := 5.0
	groupID := int64(8)
	fallbackGroupID := int64(2)
	rateMultiplier := 0.0463
	modelScopes := []string{"openai"}
	svc := NewEntitlementService(
		&entitlementUserRepoStub{users: map[int64]*User{42: {ID: 42, Balance: 12.5}}},
		&entitlementGroupRepoStub{groups: map[int64]*Group{8: {ID: 8, Name: "OpenAI-Subscription", Platform: PlatformOpenAI, SubscriptionType: SubscriptionTypeSubscription, MonthlyLimitUSD: &monthlyLimit, RateMultiplier: rateMultiplier, SupportedModelScopes: modelScopes, FallbackGroupID: &fallbackGroupID}}},
		&entitlementAPIKeyUpdaterStub{},
		&entitlementAPIKeyRepoStub{keys: []APIKey{{ID: 100, UserID: 42, Status: StatusActive, GroupID: &groupID}}},
		&entitlementUserSubRepoStub{subs: []UserSubscription{{ID: 7, UserID: 42, GroupID: 8, Status: StatusActive, StartsAt: now, ExpiresAt: later, MonthlyUsageUSD: 1.25}}},
	)

	state, err := svc.GetUserEntitlements(context.Background(), 42)
	require.NoError(t, err)
	require.NotNil(t, state.Current)
	require.Equal(t, int64(100), state.Current.APIKeyID)
	require.Equal(t, groupID, *state.Current.GroupID)
	require.Equal(t, EntitlementModeSubscription, state.Current.Mode)
	require.Equal(t, PlatformOpenAI, state.Current.GroupPlatform)
	require.Equal(t, rateMultiplier, state.Current.RateMultiplier)
	require.Equal(t, modelScopes, state.Current.SupportedModelScopes)
	require.Equal(t, monthlyLimit, *state.Current.MonthlyLimitUSD)
	require.Equal(t, 1.25, *state.Current.MonthlyUsageUSD)
	require.True(t, state.Fallback.Available)
	require.Equal(t, 12.5, state.Fallback.BalanceUSD)
	require.Len(t, state.Entitlements, 1)
	require.True(t, state.Entitlements[0].Switchable)
	require.True(t, state.Entitlements[0].Current)
	require.Equal(t, PlatformOpenAI, state.Entitlements[0].GroupPlatform)
	require.Equal(t, rateMultiplier, state.Entitlements[0].RateMultiplier)
	require.Equal(t, modelScopes, state.Entitlements[0].SupportedModelScopes)
	require.Equal(t, monthlyLimit, *state.Entitlements[0].MonthlyLimitUSD)
	require.Equal(t, fallbackGroupID, *state.Entitlements[0].FallbackGroupID)
}

func TestEntitlementService_SwitchEntitlement_UsesSelectedAPIKeyAndRefreshesState(t *testing.T) {
	now := time.Now()
	later := now.Add(30 * 24 * time.Hour)
	oldGroupID := int64(2)
	newGroupID := int64(8)
	updater := &entitlementAPIKeyUpdaterStub{}
	svc := NewEntitlementService(
		&entitlementUserRepoStub{users: map[int64]*User{42: {ID: 42}}},
		&entitlementGroupRepoStub{groups: map[int64]*Group{8: {ID: 8, Name: "OpenAI-Subscription", SubscriptionType: SubscriptionTypeSubscription}}},
		updater,
		&entitlementAPIKeyRepoStub{keys: []APIKey{{ID: 100, UserID: 42, Status: StatusActive, GroupID: &oldGroupID}}},
		&entitlementUserSubRepoStub{subs: []UserSubscription{{ID: 7, UserID: 42, GroupID: 8, Status: StatusActive, StartsAt: now, ExpiresAt: later}}},
	)

	result, err := svc.SwitchEntitlement(context.Background(), 42, SwitchEntitlementRequest{GroupID: newGroupID})
	require.NoError(t, err)
	require.NotNil(t, result.APIKey)
	require.Equal(t, int64(100), updater.updatedID)
	require.Equal(t, int64(42), updater.updatedUserID)
	require.Equal(t, newGroupID, *updater.updatedGroup)
	require.NotNil(t, result.State)
}

func TestEntitlementService_SwitchEntitlement_NoAPIKeyReturnsActionableError(t *testing.T) {
	svc := NewEntitlementService(
		&entitlementUserRepoStub{users: map[int64]*User{42: {ID: 42}}},
		&entitlementGroupRepoStub{groups: map[int64]*Group{}},
		&entitlementAPIKeyUpdaterStub{},
		&entitlementAPIKeyRepoStub{},
		&entitlementUserSubRepoStub{},
	)

	_, err := svc.SwitchEntitlement(context.Background(), 42, SwitchEntitlementRequest{GroupID: 8})
	require.ErrorIs(t, err, ErrEntitlementAPIKeyRequired)
}

func TestEntitlementService_GetUserEntitlements_AttachesCreditQuotaForBalanceGroup(t *testing.T) {
	now := time.Now()
	later := now.Add(30 * 24 * time.Hour)
	balanceGroupID := int64(2)
	svc := NewEntitlementService(
		&entitlementUserRepoStub{users: map[int64]*User{42: {ID: 42, Balance: 50}}},
		&entitlementGroupRepoStub{groups: map[int64]*Group{2: {ID: 2, Name: "OpenAI Plus", SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 0.0463}}},
		&entitlementAPIKeyUpdaterStub{},
		&entitlementAPIKeyRepoStub{keys: []APIKey{{ID: 100, UserID: 42, Status: StatusActive, GroupID: &balanceGroupID}}},
		&entitlementUserSubRepoStub{subs: []UserSubscription{{ID: 9, UserID: 42, GroupID: balanceGroupID, Status: StatusActive, StartsAt: now, ExpiresAt: later}}},
	)
	svc.SetUsageRepository(&entitlementUsageRepoStub{summary: &usagestats.CreditUsageSummary{
		UserID:                     42,
		CreditUnitScale:            1,
		BalanceLedgerAmount:        50,
		TotalPurchasedLedgerAmount: 100,
		TotalPurchasedCredits:      27000000,
		TotalUsedLedgerAmount:      50,
		TotalUsedCredits:           13500000,
		Accuracy:                   "balance_derived",
		GroupEstimates: []usagestats.CreditUsageGroupEstimate{
			{
				GroupID:               balanceGroupID,
				GroupName:             "OpenAI Plus",
				RateMultiplier:        0.0463,
				PurchasedLedgerAmount: 100,
				PurchasedCredits:      27000000,
			},
		},
	}})

	state, err := svc.GetUserEntitlements(context.Background(), 42)
	require.NoError(t, err)
	require.NotNil(t, state.CreditUsage)
	require.Equal(t, 27000000.0, state.CreditUsage.TotalPurchasedCredits)
	require.Equal(t, 13500000.0, state.CreditUsage.TotalUsedCredits)

	require.Len(t, state.Entitlements, 1)
	item := state.Entitlements[0]
	require.Equal(t, EntitlementModeBalance, item.Mode)
	require.NotNil(t, item.CreditQuota)
	require.Equal(t, 27000000.0, item.CreditQuota.PurchasedCredits)
	// Single-group case: used share equals total used.
	require.InDelta(t, 13500000.0, item.CreditQuota.UsedCredits, 0.001)
	require.InDelta(t, 13500000.0, item.CreditQuota.RemainingCredits, 0.001)
	require.InDelta(t, 50.0, item.CreditQuota.UsedPercent, 0.001)
	require.False(t, item.CreditQuota.NearLimit)
	require.Equal(t, "balance_derived", item.CreditQuota.Accuracy)
	require.NotEmpty(t, item.CreditQuota.AccuracyNotes)
}

func TestEntitlementService_GetUserEntitlements_DoesNotAttachCreditQuotaForSubscriptionGroup(t *testing.T) {
	now := time.Now()
	later := now.Add(30 * 24 * time.Hour)
	subscriptionGroupID := int64(8)
	svc := NewEntitlementService(
		&entitlementUserRepoStub{users: map[int64]*User{42: {ID: 42, Balance: 0}}},
		&entitlementGroupRepoStub{groups: map[int64]*Group{8: {ID: 8, Name: "OpenAI-Subscription", SubscriptionType: SubscriptionTypeSubscription, RateMultiplier: 1}}},
		&entitlementAPIKeyUpdaterStub{},
		&entitlementAPIKeyRepoStub{keys: []APIKey{{ID: 100, UserID: 42, Status: StatusActive, GroupID: &subscriptionGroupID}}},
		&entitlementUserSubRepoStub{subs: []UserSubscription{{ID: 9, UserID: 42, GroupID: subscriptionGroupID, Status: StatusActive, StartsAt: now, ExpiresAt: later}}},
	)
	svc.SetUsageRepository(&entitlementUsageRepoStub{summary: &usagestats.CreditUsageSummary{
		UserID:          42,
		CreditUnitScale: 1,
		GroupEstimates: []usagestats.CreditUsageGroupEstimate{
			{GroupID: subscriptionGroupID, PurchasedCredits: 999, RateMultiplier: 1},
		},
	}})

	state, err := svc.GetUserEntitlements(context.Background(), 42)
	require.NoError(t, err)
	require.Len(t, state.Entitlements, 1)
	require.Equal(t, EntitlementModeSubscription, state.Entitlements[0].Mode)
	require.Nil(t, state.Entitlements[0].CreditQuota, "subscription groups must not carry credit_quota; they use USD counters")
}

func TestEntitlementService_GetUserEntitlements_UsageRepoErrorDoesNotFailResponse(t *testing.T) {
	now := time.Now()
	later := now.Add(30 * 24 * time.Hour)
	balanceGroupID := int64(2)
	svc := NewEntitlementService(
		&entitlementUserRepoStub{users: map[int64]*User{42: {ID: 42, Balance: 50}}},
		&entitlementGroupRepoStub{groups: map[int64]*Group{2: {ID: 2, Name: "OpenAI Plus", SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 0.0463}}},
		&entitlementAPIKeyUpdaterStub{},
		&entitlementAPIKeyRepoStub{keys: []APIKey{{ID: 100, UserID: 42, Status: StatusActive, GroupID: &balanceGroupID}}},
		&entitlementUserSubRepoStub{subs: []UserSubscription{{ID: 9, UserID: 42, GroupID: balanceGroupID, Status: StatusActive, StartsAt: now, ExpiresAt: later}}},
	)
	stub := &entitlementUsageRepoStub{err: errors.New("transient sql failure")}
	svc.SetUsageRepository(stub)

	state, err := svc.GetUserEntitlements(context.Background(), 42)
	require.NoError(t, err, "usage repo failure must not break entitlement response")
	require.Equal(t, 1, stub.calls)
	require.Nil(t, state.CreditUsage, "credit_usage must be empty when repo errs")
	require.Len(t, state.Entitlements, 1)
	require.Nil(t, state.Entitlements[0].CreditQuota)
}
