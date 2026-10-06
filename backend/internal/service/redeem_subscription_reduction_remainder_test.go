package service

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestRedeemReductionPreservesRemainingTime(t *testing.T) {
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name      string
		remaining time.Duration
		days      int
		expired   bool
	}{
		{"partial day", 36 * time.Hour, 1, false},
		{"one second left", 24*time.Hour + time.Second, 1, false},
		{"exact exhaustion", 24 * time.Hour, 1, true},
		{"excess deduction", 12 * time.Hour, 1, true},
		{"already expired", -time.Hour, 1, true},
		{"multiple days", 84 * time.Hour, 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sub := UserSubscription{ID: 7, ExpiresAt: now.Add(tc.remaining), Status: SubscriptionStatusActive, Notes: "original"}
			repo := &lockingRenewalRepo{stale: sub, current: sub}
			subscriptions := NewSubscriptionService(nil, repo, nil, nil, nil)
			subscriptions.now = func() time.Time { return now }
			svc := &RedeemService{subscriptionService: subscriptions}
			require.NoError(t, svc.reduceOrCancelSubscription(context.Background(), 11, 13, tc.days, "deduct"))
			want := sub.ExpiresAt.AddDate(0, 0, -tc.days)
			status := SubscriptionStatusActive
			if tc.expired {
				want = now
				status = SubscriptionStatusExpired
			}
			require.Equal(t, want, repo.current.ExpiresAt)
			require.Equal(t, status, repo.current.Status)
			require.Contains(t, repo.current.Notes, "original")
			require.Contains(t, repo.current.Notes, "deduct")
		})
	}
}
