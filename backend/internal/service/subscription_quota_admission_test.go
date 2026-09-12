package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSubscriptionQuotaAdmissionBypassesOldCacheAndFreezesCopy(t *testing.T) {
	start := time.Now().UTC()
	fresh := &UserSubscription{ID: 1, UserID: 2, GroupID: 3, Status: SubscriptionStatusActive, ExpiresAt: start.Add(time.Hour), DailyWindowStart: &start, Quota: &SubscriptionQuotaSnapshot{SubscriptionID: 1, DailyBucketID: 10, WeeklyBucketID: 11, MonthlyBucketID: 12, DailyWindowStart: &start}}
	// No Redis client is configured: an authoritative refresh must not consult
	// an obsolete cache entry after a group event has replenished the quota.
	s := &BillingCacheService{}
	s.SetSubscriptionQuotaAdmission(func(context.Context, int64, *Group) (*UserSubscription, error) { return fresh, nil })
	request := &UserSubscription{ID: 1, UserID: 2, GroupID: 3, DailyUsageUSD: 1000}
	require.NoError(t, s.checkSubscriptionEligibility(context.Background(), 2, &Group{ID: 3}, request))
	require.Zero(t, request.DailyUsageUSD)
	frozen := CloneSubscriptionForRequest(request)
	fresh.Quota.DailyBucketID = 20
	require.NoError(t, s.RefreshSubscriptionQuotaAdmission(context.Background(), 2, &Group{ID: 3}, request))
	require.Equal(t, int64(20), request.Quota.DailyBucketID)
	require.Equal(t, int64(10), frozen.Quota.DailyBucketID, "queued billing retains the earlier bucket")
	*request.Quota.DailyWindowStart = start.Add(time.Hour)
	require.True(t, start.Equal(*frozen.Quota.DailyWindowStart))
}

func TestSubscriptionQuotaAdmissionRejectsOwnershipMismatch(t *testing.T) {
	s := &BillingCacheService{}
	s.SetSubscriptionQuotaAdmission(func(context.Context, int64, *Group) (*UserSubscription, error) {
		return &UserSubscription{ID: 1, UserID: 99, GroupID: 3}, nil
	})
	request := &UserSubscription{ID: 1, UserID: 2, GroupID: 3}
	err := s.RefreshSubscriptionQuotaAdmission(context.Background(), 2, &Group{ID: 3}, request)
	require.ErrorIs(t, err, ErrSubscriptionInvalid)
	require.Equal(t, int64(2), request.UserID)
}

func TestSubscriptionQuotaAdmissionUsesCurrentLimitDecision(t *testing.T) {
	s := &BillingCacheService{}
	s.SetSubscriptionQuotaAdmission(func(context.Context, int64, *Group) (*UserSubscription, error) { return nil, ErrWeeklyLimitExceeded })
	request := &UserSubscription{ID: 1, UserID: 2, GroupID: 3}
	err := s.RefreshSubscriptionQuotaAdmission(context.Background(), 2, &Group{ID: 3}, request)
	require.ErrorIs(t, err, ErrWeeklyLimitExceeded, "an old zero-usage object cannot override a fresh rejection")
}
