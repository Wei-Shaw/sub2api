//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUserSubscriptionDaysRemainingAt(t *testing.T) {
	now := time.Date(2026, time.July, 19, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		expiresAt time.Time
		want      int
	}{
		{name: "expired", expiresAt: now.Add(-time.Nanosecond), want: 0},
		{name: "expires now", expiresAt: now, want: 0},
		{name: "less than one day", expiresAt: now.Add(subscriptionDayDuration - time.Nanosecond), want: 1},
		{name: "exactly one day", expiresAt: now.Add(subscriptionDayDuration), want: 1},
		{name: "over one day", expiresAt: now.Add(subscriptionDayDuration + time.Nanosecond), want: 2},
		{name: "less than two days", expiresAt: now.Add(2*subscriptionDayDuration - time.Nanosecond), want: 2},
		{name: "exactly two days", expiresAt: now.Add(2 * subscriptionDayDuration), want: 2},
		{name: "over two days", expiresAt: now.Add(2*subscriptionDayDuration + time.Nanosecond), want: 3},
		{name: "exactly seven days", expiresAt: now.Add(7 * subscriptionDayDuration), want: 7},
		{name: "over seven days", expiresAt: now.Add(7*subscriptionDayDuration + time.Nanosecond), want: 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub := &UserSubscription{ExpiresAt: tt.expiresAt}
			require.Equal(t, tt.want, sub.daysRemainingAt(now))
		})
	}
}
