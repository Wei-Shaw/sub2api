package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

func TestResetQuotaOnRenewalRestartsCurrentTerm(t *testing.T) {
	now := time.Date(2026, 10, 22, 13, 24, 0, 0, time.UTC)
	for _, status := range []string{SubscriptionStatusActive, SubscriptionStatusSuspended, SubscriptionStatusExpired} {
		t.Run(status, func(t *testing.T) {
			start := now.AddDate(0, 0, -20)
			current := UserSubscription{
				ID: 7, UserID: 11, GroupID: 13, Status: status,
				StartsAt: start, ExpiresAt: start.AddDate(0, 0, 30),
				DailyWindowStart: &start, WeeklyWindowStart: &start, MonthlyWindowStart: &start,
				DailyUsageUSD: 10, WeeklyUsageUSD: 50, MonthlyUsageUSD: 100,
				AssignedAt: start, Notes: "original purchase",
			}
			if status == SubscriptionStatusExpired {
				current.ExpiresAt = now
			}
			repo := &lockingRenewalRepo{stale: current, current: current}
			svc := NewSubscriptionService(&subscriptionGroupRepoStub{group: &Group{
				ID: 13, SubscriptionType: SubscriptionTypeSubscription, ResetQuotaOnRenewal: true,
			}}, repo, nil, nil, nil)
			svc.now = func() time.Time { return now }

			sub, reused, err := svc.AssignOrExtendSubscription(context.Background(), &AssignSubscriptionInput{
				UserID: 11, GroupID: 13, ValidityDays: 30, ApplyRenewalPolicy: true, Notes: "new purchase",
			})
			require.NoError(t, err)
			require.True(t, reused)
			require.Equal(t, 1, repo.lockReads)
			require.Equal(t, current.ID, sub.ID)
			require.Equal(t, current.AssignedAt, sub.AssignedAt)
			require.Equal(t, "original purchase\nnew purchase", sub.Notes)
			require.Equal(t, SubscriptionStatusActive, sub.Status)
			require.Equal(t, now, sub.StartsAt)
			require.Equal(t, now.AddDate(0, 0, 30), sub.ExpiresAt)
			require.Equal(t, timezone.StartOfDay(now), *sub.DailyWindowStart)
			require.Equal(t, now, *sub.WeeklyWindowStart)
			require.Equal(t, now, *sub.MonthlyWindowStart)
			require.Zero(t, sub.DailyUsageUSD)
			require.Zero(t, sub.WeeklyUsageUSD)
			require.Zero(t, sub.MonthlyUsageUSD)
			// Neither the old day-30 boundary nor the new exact expiry can
			// grant an extra monthly allowance for this 30-day purchase.
			require.False(t, sub.canAutomaticallyResetMonthlyAt(start.AddDate(0, 0, 30)))
			require.False(t, sub.canAutomaticallyResetMonthlyAt(sub.ExpiresAt))
		})
	}
}

func TestResetQuotaOnRenewalDoesNotAccumulateRepeatedPurchases(t *testing.T) {
	now := time.Date(2026, 10, 22, 13, 24, 0, 0, time.UTC)
	current := UserSubscription{ID: 7, UserID: 11, GroupID: 13, Status: SubscriptionStatusActive,
		StartsAt: now.AddDate(0, 0, -20), ExpiresAt: now.AddDate(0, 0, 10)}
	repo := &lockingRenewalRepo{stale: current, current: current}
	svc := NewSubscriptionService(&subscriptionGroupRepoStub{group: &Group{
		ID: 13, SubscriptionType: SubscriptionTypeSubscription, ResetQuotaOnRenewal: true,
	}}, repo, nil, nil, nil)
	svc.now = func() time.Time { return now }
	input := &AssignSubscriptionInput{UserID: 11, GroupID: 13, ValidityDays: 30, ApplyRenewalPolicy: true}
	_, _, err := svc.AssignOrExtendSubscription(context.Background(), input)
	require.NoError(t, err)
	now = now.Add(time.Hour)
	sub, _, err := svc.AssignOrExtendSubscription(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, now, sub.StartsAt)
	require.Equal(t, now.AddDate(0, 0, 30), sub.ExpiresAt)
	// Explicit administrative adjustments retain their existing semantics.
	sub, err = svc.ExtendSubscription(context.Background(), sub.ID, 5)
	require.NoError(t, err)
	require.Equal(t, now, sub.StartsAt)
	require.Equal(t, now.AddDate(0, 0, 35), sub.ExpiresAt)
}

func TestResetQuotaOnRenewalDoesNotChangeDefaultGrants(t *testing.T) {
	now := time.Date(2026, 10, 22, 13, 24, 0, 0, time.UTC)
	current := UserSubscription{ID: 7, UserID: 11, GroupID: 13, Status: SubscriptionStatusActive,
		StartsAt: now.AddDate(0, 0, -20), ExpiresAt: now.AddDate(0, 0, 10), MonthlyUsageUSD: 42}
	repo := &lockingRenewalRepo{stale: current, current: current}
	svc := NewSubscriptionService(&subscriptionGroupRepoStub{group: &Group{
		ID: 13, SubscriptionType: SubscriptionTypeSubscription, ResetQuotaOnRenewal: true,
	}}, repo, nil, nil, nil)
	svc.now = func() time.Time { return now }
	sub, _, err := svc.AssignOrExtendSubscription(context.Background(), &AssignSubscriptionInput{
		UserID: 11, GroupID: 13, ValidityDays: 5,
	})
	require.NoError(t, err)
	require.Equal(t, current.StartsAt, sub.StartsAt)
	require.Equal(t, current.ExpiresAt.AddDate(0, 0, 5), sub.ExpiresAt)
	require.Equal(t, current.MonthlyUsageUSD, sub.MonthlyUsageUSD)
}

func TestRenewalPolicyDefaultsToExtendingTime(t *testing.T) {
	now := time.Date(2026, 10, 22, 13, 24, 0, 0, time.UTC)
	current := UserSubscription{ID: 7, UserID: 11, GroupID: 13, Status: SubscriptionStatusActive,
		StartsAt: now.AddDate(0, 0, -20), ExpiresAt: now.AddDate(0, 0, 10), MonthlyUsageUSD: 42}
	repo := &lockingRenewalRepo{stale: current, current: current}
	svc := NewSubscriptionService(&subscriptionGroupRepoStub{group: &Group{
		ID: 13, SubscriptionType: SubscriptionTypeSubscription,
	}}, repo, nil, nil, nil)
	svc.now = func() time.Time { return now }
	sub, _, err := svc.AssignOrExtendSubscription(context.Background(), &AssignSubscriptionInput{
		UserID: 11, GroupID: 13, ValidityDays: 30, ApplyRenewalPolicy: true,
	})
	require.NoError(t, err)
	require.Equal(t, current.StartsAt, sub.StartsAt)
	require.Equal(t, current.ExpiresAt.AddDate(0, 0, 30), sub.ExpiresAt)
	require.Equal(t, current.MonthlyUsageUSD, sub.MonthlyUsageUSD)
}
