//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// nextFixedDailyReset
// ---------------------------------------------------------------------------

func TestNextFixedDailyReset_BeforeResetHour(t *testing.T) {
	tz := time.UTC
	// 2026-03-14 06:00 UTC, reset hour = 9
	after := time.Date(2026, 3, 14, 6, 0, 0, 0, tz)
	got := nextFixedDailyReset(9, tz, after)
	want := time.Date(2026, 3, 14, 9, 0, 0, 0, tz)
	assert.Equal(t, want, got)
}

func TestNextFixedDailyReset_AtResetHour(t *testing.T) {
	tz := time.UTC
	// Exactly at reset hour → should return tomorrow
	after := time.Date(2026, 3, 14, 9, 0, 0, 0, tz)
	got := nextFixedDailyReset(9, tz, after)
	want := time.Date(2026, 3, 15, 9, 0, 0, 0, tz)
	assert.Equal(t, want, got)
}

func TestNextFixedDailyReset_AfterResetHour(t *testing.T) {
	tz := time.UTC
	// After reset hour → should return tomorrow
	after := time.Date(2026, 3, 14, 15, 30, 0, 0, tz)
	got := nextFixedDailyReset(9, tz, after)
	want := time.Date(2026, 3, 15, 9, 0, 0, 0, tz)
	assert.Equal(t, want, got)
}

func TestNextFixedDailyReset_MidnightReset(t *testing.T) {
	tz := time.UTC
	// Reset at hour 0 (midnight), currently 23:59
	after := time.Date(2026, 3, 14, 23, 59, 0, 0, tz)
	got := nextFixedDailyReset(0, tz, after)
	want := time.Date(2026, 3, 15, 0, 0, 0, 0, tz)
	assert.Equal(t, want, got)
}

func TestNextFixedDailyReset_NonUTCTimezone(t *testing.T) {
	tz, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)

	// 2026-03-14 07:00 UTC = 2026-03-14 15:00 CST, reset hour = 9 (CST)
	after := time.Date(2026, 3, 14, 7, 0, 0, 0, time.UTC)
	got := nextFixedDailyReset(9, tz, after)
	// Already past 9:00 CST today → tomorrow 9:00 CST = 2026-03-15 01:00 UTC
	want := time.Date(2026, 3, 15, 9, 0, 0, 0, tz)
	assert.Equal(t, want, got)
}

// ---------------------------------------------------------------------------
// lastFixedDailyReset
// ---------------------------------------------------------------------------

func TestLastFixedDailyReset_BeforeResetHour(t *testing.T) {
	tz := time.UTC
	now := time.Date(2026, 3, 14, 6, 0, 0, 0, tz)
	got := lastFixedDailyReset(9, tz, now)
	// Before today's 9:00 → yesterday 9:00
	want := time.Date(2026, 3, 13, 9, 0, 0, 0, tz)
	assert.Equal(t, want, got)
}

func TestLastFixedDailyReset_AtResetHour(t *testing.T) {
	tz := time.UTC
	now := time.Date(2026, 3, 14, 9, 0, 0, 0, tz)
	got := lastFixedDailyReset(9, tz, now)
	// At exactly 9:00 → today 9:00
	want := time.Date(2026, 3, 14, 9, 0, 0, 0, tz)
	assert.Equal(t, want, got)
}

func TestLastFixedDailyReset_AfterResetHour(t *testing.T) {
	tz := time.UTC
	now := time.Date(2026, 3, 14, 15, 0, 0, 0, tz)
	got := lastFixedDailyReset(9, tz, now)
	// After 9:00 → today 9:00
	want := time.Date(2026, 3, 14, 9, 0, 0, 0, tz)
	assert.Equal(t, want, got)
}

// ---------------------------------------------------------------------------
// nextFixedWeeklyReset
// ---------------------------------------------------------------------------

func TestNextFixedWeeklyReset_TargetDayAhead(t *testing.T) {
	tz := time.UTC
	// 2026-03-14 is Saturday (day=6), target = Monday (day=1), hour = 9
	after := time.Date(2026, 3, 14, 10, 0, 0, 0, tz)
	got := nextFixedWeeklyReset(1, 9, tz, after)
	// Next Monday = 2026-03-16
	want := time.Date(2026, 3, 16, 9, 0, 0, 0, tz)
	assert.Equal(t, want, got)
}

func TestNextFixedWeeklyReset_TargetDayToday_BeforeHour(t *testing.T) {
	tz := time.UTC
	// 2026-03-16 is Monday (day=1), target = Monday, hour = 9, before 9:00
	after := time.Date(2026, 3, 16, 6, 0, 0, 0, tz)
	got := nextFixedWeeklyReset(1, 9, tz, after)
	// Today at 9:00
	want := time.Date(2026, 3, 16, 9, 0, 0, 0, tz)
	assert.Equal(t, want, got)
}

func TestNextFixedWeeklyReset_TargetDayToday_AtHour(t *testing.T) {
	tz := time.UTC
	// 2026-03-16 is Monday, target = Monday, hour = 9, exactly at 9:00
	after := time.Date(2026, 3, 16, 9, 0, 0, 0, tz)
	got := nextFixedWeeklyReset(1, 9, tz, after)
	// Next Monday at 9:00
	want := time.Date(2026, 3, 23, 9, 0, 0, 0, tz)
	assert.Equal(t, want, got)
}

func TestNextFixedWeeklyReset_TargetDayToday_AfterHour(t *testing.T) {
	tz := time.UTC
	// 2026-03-16 is Monday, target = Monday, hour = 9, after 9:00
	after := time.Date(2026, 3, 16, 15, 0, 0, 0, tz)
	got := nextFixedWeeklyReset(1, 9, tz, after)
	// Next Monday at 9:00
	want := time.Date(2026, 3, 23, 9, 0, 0, 0, tz)
	assert.Equal(t, want, got)
}

func TestNextFixedWeeklyReset_TargetDayPast(t *testing.T) {
	tz := time.UTC
	// 2026-03-18 is Wednesday (day=3), target = Monday (day=1)
	after := time.Date(2026, 3, 18, 10, 0, 0, 0, tz)
	got := nextFixedWeeklyReset(1, 9, tz, after)
	// Next Monday = 2026-03-23
	want := time.Date(2026, 3, 23, 9, 0, 0, 0, tz)
	assert.Equal(t, want, got)
}

func TestNextFixedWeeklyReset_Sunday(t *testing.T) {
	tz := time.UTC
	// 2026-03-14 is Saturday (day=6), target = Sunday (day=0)
	after := time.Date(2026, 3, 14, 10, 0, 0, 0, tz)
	got := nextFixedWeeklyReset(0, 0, tz, after)
	// Next Sunday = 2026-03-15
	want := time.Date(2026, 3, 15, 0, 0, 0, 0, tz)
	assert.Equal(t, want, got)
}

// ---------------------------------------------------------------------------
// lastFixedWeeklyReset
// ---------------------------------------------------------------------------

func TestLastFixedWeeklyReset_SameDay_AfterHour(t *testing.T) {
	tz := time.UTC
	// 2026-03-16 is Monday (day=1), target = Monday, hour = 9, now = 15:00
	now := time.Date(2026, 3, 16, 15, 0, 0, 0, tz)
	got := lastFixedWeeklyReset(1, 9, tz, now)
	// Today at 9:00
	want := time.Date(2026, 3, 16, 9, 0, 0, 0, tz)
	assert.Equal(t, want, got)
}

func TestLastFixedWeeklyReset_SameDay_BeforeHour(t *testing.T) {
	tz := time.UTC
	// 2026-03-16 is Monday, target = Monday, hour = 9, now = 06:00
	now := time.Date(2026, 3, 16, 6, 0, 0, 0, tz)
	got := lastFixedWeeklyReset(1, 9, tz, now)
	// Last Monday at 9:00 = 2026-03-09
	want := time.Date(2026, 3, 9, 9, 0, 0, 0, tz)
	assert.Equal(t, want, got)
}

func TestLastFixedWeeklyReset_DifferentDay(t *testing.T) {
	tz := time.UTC
	// 2026-03-18 is Wednesday (day=3), target = Monday (day=1)
	now := time.Date(2026, 3, 18, 10, 0, 0, 0, tz)
	got := lastFixedWeeklyReset(1, 9, tz, now)
	// Last Monday = 2026-03-16
	want := time.Date(2026, 3, 16, 9, 0, 0, 0, tz)
	assert.Equal(t, want, got)
}

// ---------------------------------------------------------------------------
// isFixedDailyPeriodExpired
// ---------------------------------------------------------------------------

func TestIsFixedDailyPeriodExpired_ZeroPeriodStart(t *testing.T) {
	a := &Account{Extra: map[string]any{
		"quota_daily_reset_mode": "fixed",
		"quota_daily_reset_hour": float64(9),
		"quota_reset_timezone":   "UTC",
	}}
	assert.True(t, a.isFixedDailyPeriodExpired(time.Time{}))
}

func TestIsFixedDailyPeriodExpired_NotExpired(t *testing.T) {
	a := &Account{Extra: map[string]any{
		"quota_daily_reset_mode": "fixed",
		"quota_daily_reset_hour": float64(9),
		"quota_reset_timezone":   "UTC",
	}}
	// Period started after the most recent reset → not expired
	// (This test uses a time very close to "now", which is after the last reset)
	periodStart := time.Now().Add(-1 * time.Minute)
	assert.False(t, a.isFixedDailyPeriodExpired(periodStart))
}

func TestIsFixedDailyPeriodExpired_Expired(t *testing.T) {
	a := &Account{Extra: map[string]any{
		"quota_daily_reset_mode": "fixed",
		"quota_daily_reset_hour": float64(9),
		"quota_reset_timezone":   "UTC",
	}}
	// Period started 3 days ago → definitely expired
	periodStart := time.Now().Add(-72 * time.Hour)
	assert.True(t, a.isFixedDailyPeriodExpired(periodStart))
}

func TestIsFixedDailyPeriodExpired_InvalidTimezone(t *testing.T) {
	a := &Account{Extra: map[string]any{
		"quota_daily_reset_mode": "fixed",
		"quota_daily_reset_hour": float64(9),
		"quota_reset_timezone":   "Invalid/Timezone",
	}}
	// Invalid timezone falls back to UTC
	periodStart := time.Now().Add(-72 * time.Hour)
	assert.True(t, a.isFixedDailyPeriodExpired(periodStart))
}

// ---------------------------------------------------------------------------
// isFixedWeeklyPeriodExpired
// ---------------------------------------------------------------------------

func TestIsFixedWeeklyPeriodExpired_ZeroPeriodStart(t *testing.T) {
	a := &Account{Extra: map[string]any{
		"quota_weekly_reset_mode": "fixed",
		"quota_weekly_reset_day":  float64(1),
		"quota_weekly_reset_hour": float64(9),
		"quota_reset_timezone":    "UTC",
	}}
	assert.True(t, a.isFixedWeeklyPeriodExpired(time.Time{}))
}

func TestIsFixedWeeklyPeriodExpired_NotExpired(t *testing.T) {
	a := &Account{Extra: map[string]any{
		"quota_weekly_reset_mode": "fixed",
		"quota_weekly_reset_day":  float64(1),
		"quota_weekly_reset_hour": float64(9),
		"quota_reset_timezone":    "UTC",
	}}
	// Period started 1 minute ago → not expired
	periodStart := time.Now().Add(-1 * time.Minute)
	assert.False(t, a.isFixedWeeklyPeriodExpired(periodStart))
}

func TestIsFixedWeeklyPeriodExpired_Expired(t *testing.T) {
	a := &Account{Extra: map[string]any{
		"quota_weekly_reset_mode": "fixed",
		"quota_weekly_reset_day":  float64(1),
		"quota_weekly_reset_hour": float64(9),
		"quota_reset_timezone":    "UTC",
	}}
	// Period started 10 days ago → definitely expired
	periodStart := time.Now().Add(-240 * time.Hour)
	assert.True(t, a.isFixedWeeklyPeriodExpired(periodStart))
}

// ---------------------------------------------------------------------------
// ValidateQuotaResetConfig
// ---------------------------------------------------------------------------

func TestValidateQuotaResetConfig_NilExtra(t *testing.T) {
	assert.NoError(t, ValidateQuotaResetConfig(nil))
}

func TestValidateQuotaResetConfig_EmptyExtra(t *testing.T) {
	assert.NoError(t, ValidateQuotaResetConfig(map[string]any{}))
}

func TestValidateQuotaResetConfig_ValidFixed(t *testing.T) {
	extra := map[string]any{
		"quota_daily_reset_mode":  "fixed",
		"quota_daily_reset_hour":  float64(9),
		"quota_weekly_reset_mode": "fixed",
		"quota_weekly_reset_day":  float64(1),
		"quota_weekly_reset_hour": float64(0),
		"quota_reset_timezone":    "Asia/Shanghai",
	}
	assert.NoError(t, ValidateQuotaResetConfig(extra))
}

func TestValidateQuotaResetConfig_ValidRolling(t *testing.T) {
	extra := map[string]any{
		"quota_daily_reset_mode":  "rolling",
		"quota_weekly_reset_mode": "rolling",
	}
	assert.NoError(t, ValidateQuotaResetConfig(extra))
}

func TestValidateQuotaResetConfig_InvalidTimezone(t *testing.T) {
	extra := map[string]any{
		"quota_reset_timezone": "Not/A/Timezone",
	}
	err := ValidateQuotaResetConfig(extra)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "quota_reset_timezone")
}

func TestValidateQuotaResetConfig_InvalidDailyMode(t *testing.T) {
	extra := map[string]any{
		"quota_daily_reset_mode": "invalid",
	}
	err := ValidateQuotaResetConfig(extra)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "quota_daily_reset_mode")
}

func TestValidateQuotaResetConfig_InvalidDailyHour_TooHigh(t *testing.T) {
	extra := map[string]any{
		"quota_daily_reset_hour": float64(24),
	}
	err := ValidateQuotaResetConfig(extra)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "quota_daily_reset_hour")
}

func TestValidateQuotaResetConfig_InvalidDailyHour_Negative(t *testing.T) {
	extra := map[string]any{
		"quota_daily_reset_hour": float64(-1),
	}
	err := ValidateQuotaResetConfig(extra)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "quota_daily_reset_hour")
}

func TestValidateQuotaResetConfig_InvalidWeeklyMode(t *testing.T) {
	extra := map[string]any{
		"quota_weekly_reset_mode": "unknown",
	}
	err := ValidateQuotaResetConfig(extra)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "quota_weekly_reset_mode")
}

func TestValidateQuotaResetConfig_InvalidWeeklyDay_TooHigh(t *testing.T) {
	extra := map[string]any{
		"quota_weekly_reset_day": float64(7),
	}
	err := ValidateQuotaResetConfig(extra)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "quota_weekly_reset_day")
}

func TestValidateQuotaResetConfig_InvalidWeeklyDay_Negative(t *testing.T) {
	extra := map[string]any{
		"quota_weekly_reset_day": float64(-1),
	}
	err := ValidateQuotaResetConfig(extra)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "quota_weekly_reset_day")
}

func TestValidateQuotaResetConfig_InvalidWeeklyHour(t *testing.T) {
	extra := map[string]any{
		"quota_weekly_reset_hour": float64(25),
	}
	err := ValidateQuotaResetConfig(extra)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "quota_weekly_reset_hour")
}

func TestValidateQuotaResetConfig_BoundaryValues(t *testing.T) {
	// All boundary values should be valid
	extra := map[string]any{
		"quota_daily_reset_hour":  float64(23),
		"quota_weekly_reset_day":  float64(0), // Sunday
		"quota_weekly_reset_hour": float64(0),
		"quota_reset_timezone":    "UTC",
	}
	assert.NoError(t, ValidateQuotaResetConfig(extra))

	extra2 := map[string]any{
		"quota_daily_reset_hour":  float64(0),
		"quota_weekly_reset_day":  float64(6), // Saturday
		"quota_weekly_reset_hour": float64(23),
	}
	assert.NoError(t, ValidateQuotaResetConfig(extra2))
}

// ---------------------------------------------------------------------------
// ComputeQuotaResetAt
// ---------------------------------------------------------------------------

func TestComputeQuotaResetAt_RollingMode_NoResetAt(t *testing.T) {
	extra := map[string]any{
		"quota_daily_reset_mode":  "rolling",
		"quota_weekly_reset_mode": "rolling",
	}
	ComputeQuotaResetAt(extra)
	_, hasDailyResetAt := extra["quota_daily_reset_at"]
	_, hasWeeklyResetAt := extra["quota_weekly_reset_at"]
	assert.False(t, hasDailyResetAt, "rolling mode should not set quota_daily_reset_at")
	assert.False(t, hasWeeklyResetAt, "rolling mode should not set quota_weekly_reset_at")
}

func TestComputeQuotaResetAt_RollingMode_ClearsExistingResetAt(t *testing.T) {
	extra := map[string]any{
		"quota_daily_reset_mode":  "rolling",
		"quota_weekly_reset_mode": "rolling",
		"quota_daily_reset_at":    "2026-03-14T09:00:00Z",
		"quota_weekly_reset_at":   "2026-03-16T09:00:00Z",
	}
	ComputeQuotaResetAt(extra)
	_, hasDailyResetAt := extra["quota_daily_reset_at"]
	_, hasWeeklyResetAt := extra["quota_weekly_reset_at"]
	assert.False(t, hasDailyResetAt, "rolling mode should remove quota_daily_reset_at")
	assert.False(t, hasWeeklyResetAt, "rolling mode should remove quota_weekly_reset_at")
}

func TestComputeQuotaResetAt_FixedDaily_SetsResetAt(t *testing.T) {
	extra := map[string]any{
		"quota_daily_reset_mode": "fixed",
		"quota_daily_reset_hour": float64(9),
		"quota_reset_timezone":   "UTC",
	}
	ComputeQuotaResetAt(extra)
	resetAtStr, ok := extra["quota_daily_reset_at"].(string)
	require.True(t, ok, "quota_daily_reset_at should be set")

	resetAt, err := time.Parse(time.RFC3339, resetAtStr)
	require.NoError(t, err)
	// Reset time should be in the future
	assert.True(t, resetAt.After(time.Now()), "reset_at should be in the future")
	// Reset hour should be 9 UTC
	assert.Equal(t, 9, resetAt.UTC().Hour())
}

func TestComputeQuotaResetAt_FixedWeekly_SetsResetAt(t *testing.T) {
	extra := map[string]any{
		"quota_weekly_reset_mode": "fixed",
		"quota_weekly_reset_day":  float64(1), // Monday
		"quota_weekly_reset_hour": float64(0),
		"quota_reset_timezone":    "UTC",
	}
	ComputeQuotaResetAt(extra)
	resetAtStr, ok := extra["quota_weekly_reset_at"].(string)
	require.True(t, ok, "quota_weekly_reset_at should be set")

	resetAt, err := time.Parse(time.RFC3339, resetAtStr)
	require.NoError(t, err)
	// Reset time should be in the future
	assert.True(t, resetAt.After(time.Now()), "reset_at should be in the future")
	// Reset day should be Monday
	assert.Equal(t, time.Monday, resetAt.UTC().Weekday())
}

func TestComputeQuotaResetAt_FixedDaily_WithTimezone(t *testing.T) {
	tz, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)

	extra := map[string]any{
		"quota_daily_reset_mode": "fixed",
		"quota_daily_reset_hour": float64(9),
		"quota_reset_timezone":   "Asia/Shanghai",
	}
	ComputeQuotaResetAt(extra)
	resetAtStr, ok := extra["quota_daily_reset_at"].(string)
	require.True(t, ok)

	resetAt, err := time.Parse(time.RFC3339, resetAtStr)
	require.NoError(t, err)
	// In Shanghai timezone, the hour should be 9
	assert.Equal(t, 9, resetAt.In(tz).Hour())
}

func TestComputeQuotaResetAt_DefaultTimezone(t *testing.T) {
	extra := map[string]any{
		"quota_daily_reset_mode": "fixed",
		"quota_daily_reset_hour": float64(12),
	}
	ComputeQuotaResetAt(extra)
	resetAtStr, ok := extra["quota_daily_reset_at"].(string)
	require.True(t, ok)

	resetAt, err := time.Parse(time.RFC3339, resetAtStr)
	require.NoError(t, err)
	// Default timezone is UTC
	assert.Equal(t, 12, resetAt.UTC().Hour())
}

func TestComputeQuotaResetAt_InvalidHour_ClampedToZero(t *testing.T) {
	extra := map[string]any{
		"quota_daily_reset_mode": "fixed",
		"quota_daily_reset_hour": float64(99),
		"quota_reset_timezone":   "UTC",
	}
	ComputeQuotaResetAt(extra)
	resetAtStr, ok := extra["quota_daily_reset_at"].(string)
	require.True(t, ok)

	resetAt, err := time.Parse(time.RFC3339, resetAtStr)
	require.NoError(t, err)
	// Invalid hour → clamped to 0
	assert.Equal(t, 0, resetAt.UTC().Hour())
}
