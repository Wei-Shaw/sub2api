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
REDACTED{
		{name: "expired", expiresAt: now.Add(-time.Nanosecond), want: 0REDACTED,
		{name: "expires now", expiresAt: now, want: 0REDACTED,
		{name: "less than one day", expiresAt: now.Add(subscriptionDayDuration - time.Nanosecond), want: 1REDACTED,
		{name: "exactly one day", expiresAt: now.Add(subscriptionDayDuration), want: 1REDACTED,
		{name: "over one day", expiresAt: now.Add(subscriptionDayDuration + time.Nanosecond), want: 2REDACTED,
		{name: "less than two days", expiresAt: now.Add(2*subscriptionDayDuration - time.Nanosecond), want: 2REDACTED,
		{name: "exactly two days", expiresAt: now.Add(2 * subscriptionDayDuration), want: 2REDACTED,
		{name: "over two days", expiresAt: now.Add(2*subscriptionDayDuration + time.Nanosecond), want: 3REDACTED,
		{name: "exactly seven days", expiresAt: now.Add(7 * subscriptionDayDuration), want: 7REDACTED,
		{name: "over seven days", expiresAt: now.Add(7*subscriptionDayDuration + time.Nanosecond), want: 8REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub := &UserSubscription{ExpiresAt: tt.expiresAtREDACTED
			require.Equal(t, tt.want, sub.daysRemainingAt(now))
	REDACTED)
REDACTED
REDACTED
