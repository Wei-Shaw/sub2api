package dto

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUserSubscriptionFromServiceMonthlyResetAt(t *testing.T) {
	startsAt := time.Date(2026, 9, 1, 12, 0, 0, 0, timezone.Location())
	legacyWindow := timezone.StartOfDay(startsAt)
	sub := &service.UserSubscription{
		StartsAt: startsAt, ExpiresAt: startsAt.Add(60 * 24 * time.Hour),
		MonthlyWindowStart: &legacyWindow,
	}

	out := UserSubscriptionFromService(sub)
	require.NotNil(t, out.MonthlyResetAt)
	require.True(t, startsAt.Add(30*24*time.Hour).Equal(*out.MonthlyResetAt))
	require.True(t, legacyWindow.Equal(*out.MonthlyWindowStart), "keep the stored anchor unchanged")

	encoded, err := json.Marshal(out)
	require.NoError(t, err)
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &fields))
	var resetAt time.Time
	require.NoError(t, json.Unmarshal(fields["monthly_reset_at"], &resetAt))
	require.True(t, resetAt.Equal(*out.MonthlyResetAt))

	// Later midnight anchors may be intentional manual resets; only the
	// legacy first anchor is corrected to the subscription opening time.
	manualWindow := legacyWindow.Add(5 * 24 * time.Hour)
	sub.MonthlyWindowStart = &manualWindow
	out = UserSubscriptionFromService(sub)
	require.True(t, manualWindow.Add(30*24*time.Hour).Equal(*out.MonthlyResetAt))

	sub.MonthlyWindowStart = nil
	require.Nil(t, UserSubscriptionFromService(sub).MonthlyResetAt)
}
