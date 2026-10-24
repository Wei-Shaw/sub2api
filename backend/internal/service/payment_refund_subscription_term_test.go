package service

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
)

func TestRefundSubscriptionAdjustmentPreservesReplacementTerm(t *testing.T) {
	now := time.Date(2026, 10, 22, 13, 24, 0, 0, time.UTC)
	oldStart := now.AddDate(0, 0, -10)
	for _, days := range []int{-30, 30} {
		current := UserSubscription{ID: 7, UserID: 11, GroupID: 13, Status: SubscriptionStatusActive,
			StartsAt: now, ExpiresAt: now.AddDate(0, 0, 30), MonthlyUsageUSD: 42}
		repo := &lockingRenewalRepo{current: current}
		subSvc := NewSubscriptionService(nil, repo, nil, nil, nil)
		subSvc.now = func() time.Time { return now }
		svc := &PaymentService{subscriptionSvc: subSvc}
		plan := &RefundPlan{SubscriptionID: current.ID, SubscriptionStartsAt: &oldStart, SubDaysToDeduct: 30}
		_, err := svc.adjustRefundSubscription(context.Background(), plan, days)
		require.NoError(t, err)
		require.Equal(t, current, repo.current, "deduction and rollback must both ignore a newer purchase")
	}
}

func TestLegacyPendingRefundDoesNotChangeReplacementTerm(t *testing.T) {
	now := time.Date(2026, 10, 22, 13, 24, 0, 0, time.UTC)
	completed := now.AddDate(0, 0, -10)
	current := UserSubscription{ID: 7, UserID: 11, GroupID: 13, Status: SubscriptionStatusActive,
		StartsAt: now, ExpiresAt: now.AddDate(0, 0, 30), MonthlyUsageUSD: 42}
	repo := &lockingRenewalRepo{current: current}
	svc := &PaymentService{subscriptionSvc: NewSubscriptionService(nil, repo, nil, nil, nil)}
	for _, days := range []int{-30, 30} {
		plan := &RefundPlan{SubscriptionID: current.ID, SubDaysToDeduct: 30,
			Order: &dbent.PaymentOrder{CompletedAt: &completed}}
		_, err := svc.adjustRefundSubscription(context.Background(), plan, days)
		require.NoError(t, err)
		require.Equal(t, current, repo.current)
	}
}

func TestRefundSubscriptionAdjustmentCanRestoreExactTerm(t *testing.T) {
	now := time.Date(2026, 10, 22, 13, 24, 0, 0, time.UTC)
	start := now.AddDate(0, 0, -10)
	current := UserSubscription{ID: 7, UserID: 11, GroupID: 13, Status: SubscriptionStatusActive,
		StartsAt: start, ExpiresAt: start.AddDate(0, 0, 30), MonthlyUsageUSD: 42}
	repo := &lockingRenewalRepo{current: current}
	subSvc := NewSubscriptionService(nil, repo, nil, nil, nil)
	subSvc.now = func() time.Time { return now }
	svc := &PaymentService{subscriptionSvc: subSvc}
	plan := &RefundPlan{SubscriptionID: current.ID, SubscriptionStartsAt: &start, SubDaysToDeduct: 30}
	_, err := svc.adjustRefundSubscription(context.Background(), plan, -30)
	require.NoError(t, err)
	require.Equal(t, SubscriptionStatusExpired, repo.current.Status)
	require.Equal(t, start, repo.current.ExpiresAt)
	// Refund failure restores the original expiry, not now + 30 days.
	_, err = svc.adjustRefundSubscription(context.Background(), plan, 30)
	require.NoError(t, err)
	require.Equal(t, current, repo.current)
}
