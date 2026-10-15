//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSubscriptionRefundAdjustmentRoundTrip(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name          string
		start         UserSubscription
		days          int
		between       func(*ledgerSubRepo)
		wantDeducted  time.Time
		wantDedStatus string
		wantRestored  time.Time
		wantStatus    string
	}{
		{
			name:          "partial deduction stays active",
			start:         UserSubscription{ID: 1, ExpiresAt: now.AddDate(0, 0, 40), Status: SubscriptionStatusActive},
			days:          30,
			wantDeducted:  now.AddDate(0, 0, 10),
			wantDedStatus: SubscriptionStatusActive,
			wantRestored:  now.AddDate(0, 0, 40),
			wantStatus:    SubscriptionStatusActive,
		},
		{
			name:          "deduction past the remaining time ends the subscription now",
			start:         UserSubscription{ID: 2, ExpiresAt: now.AddDate(0, 0, 10), Status: SubscriptionStatusActive},
			days:          30,
			wantDeducted:  now,
			wantDedStatus: SubscriptionStatusExpired,
			wantRestored:  now.AddDate(0, 0, 10),
			wantStatus:    SubscriptionStatusActive,
		},
		{
			name:          "a suspended subscription returns suspended",
			start:         UserSubscription{ID: 3, ExpiresAt: now.AddDate(0, 0, 5), Status: SubscriptionStatusSuspended},
			days:          30,
			wantDeducted:  now,
			wantDedStatus: SubscriptionStatusExpired,
			wantRestored:  now.AddDate(0, 0, 5),
			wantStatus:    SubscriptionStatusSuspended,
		},
		{
			name:  "a renewal bought in between is kept",
			start: UserSubscription{ID: 4, ExpiresAt: now.AddDate(0, 0, 10), Status: SubscriptionStatusActive},
			days:  30,
			between: func(r *ledgerSubRepo) {
				sub := r.rows[4]
				sub.ExpiresAt = now.AddDate(0, 0, 30)
				sub.Status = SubscriptionStatusActive
				r.rows[4] = sub
			},
			wantDeducted:  now,
			wantDedStatus: SubscriptionStatusExpired,
			wantRestored:  now.AddDate(0, 0, 40),
			wantStatus:    SubscriptionStatusActive,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			repo := newLedgerSubRepo(tc.start)
			svc := &SubscriptionService{userSubRepo: repo, now: func() time.Time { return now }}

			adjustment, err := svc.deductRefundSubscriptionDays(ctx, tc.start.ID, tc.days)
			require.NoError(t, err)
			require.NotNil(t, adjustment)
			deducted := repo.current(tc.start.ID)
			require.True(t, deducted.ExpiresAt.Equal(tc.wantDeducted), "deducted expiry %s", deducted.ExpiresAt)
			require.Equal(t, tc.wantDedStatus, deducted.Status)
			require.False(t, repo.wasDeleted(tc.start.ID))

			if tc.between != nil {
				tc.between(repo)
			}
			require.NoError(t, svc.restoreRefundSubscription(ctx, adjustment))
			restored := repo.current(tc.start.ID)
			require.True(t, restored.ExpiresAt.Equal(tc.wantRestored), "restored expiry %s", restored.ExpiresAt)
			require.Equal(t, tc.wantStatus, restored.Status)
		})
	}
}

func TestSubscriptionRefundAdjustmentEdges(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

	t.Run("an ended subscription has nothing to deduct", func(t *testing.T) {
		repo := newLedgerSubRepo(UserSubscription{ID: 5, ExpiresAt: now.Add(-time.Hour), Status: SubscriptionStatusExpired})
		svc := &SubscriptionService{userSubRepo: repo, now: func() time.Time { return now }}
		adjustment, err := svc.deductRefundSubscriptionDays(ctx, 5, 30)
		require.NoError(t, err)
		require.Nil(t, adjustment)
		require.True(t, repo.current(5).ExpiresAt.Equal(now.Add(-time.Hour)))
	})

	t.Run("a subscription revoked in between is not brought back", func(t *testing.T) {
		repo := newLedgerSubRepo(UserSubscription{ID: 6, ExpiresAt: now.AddDate(0, 0, 10), Status: SubscriptionStatusActive})
		svc := &SubscriptionService{userSubRepo: repo, now: func() time.Time { return now }}
		adjustment, err := svc.deductRefundSubscriptionDays(ctx, 6, 3)
		require.NoError(t, err)
		require.NoError(t, repo.Delete(ctx, 6))
		require.ErrorIs(t, svc.restoreRefundSubscription(ctx, adjustment), ErrSubscriptionNotFound)
		require.True(t, repo.wasDeleted(6))
	})

	t.Run("confirmation removes the interval again on top of a renewal", func(t *testing.T) {
		repo := newLedgerSubRepo(UserSubscription{ID: 7, ExpiresAt: now.AddDate(0, 0, 10), Status: SubscriptionStatusActive})
		svc := &SubscriptionService{userSubRepo: repo, now: func() time.Time { return now }}
		adjustment, err := svc.deductRefundSubscriptionDays(ctx, 7, 30)
		require.NoError(t, err)
		require.NoError(t, svc.restoreRefundSubscription(ctx, adjustment))
		require.NoError(t, repo.ExtendExpiry(ctx, 7, now.AddDate(0, 0, 40)))

		again, err := svc.reapplyRefundSubscription(ctx, adjustment)
		require.NoError(t, err)
		require.NotNil(t, again)
		sub := repo.current(7)
		require.True(t, sub.ExpiresAt.Equal(now.AddDate(0, 0, 30)), "only the refunded 10 days are removed again: %s", sub.ExpiresAt)
		require.Equal(t, SubscriptionStatusActive, sub.Status)
	})
}
