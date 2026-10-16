package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMinutesInWindow(t *testing.T) {
	t.Parallel()

	// same-day [09:00, 18:00)
	require.True(t, minutesInWindow(9*60, 9*60, 18*60))
	require.True(t, minutesInWindow(12*60, 9*60, 18*60))
	require.False(t, minutesInWindow(18*60, 9*60, 18*60))
	require.False(t, minutesInWindow(8*60+59, 9*60, 18*60))

	// overnight [22:00, 06:00)
	require.True(t, minutesInWindow(22*60, 22*60, 6*60))
	require.True(t, minutesInWindow(23*60, 22*60, 6*60))
	require.True(t, minutesInWindow(0, 22*60, 6*60))
	require.True(t, minutesInWindow(5*60+59, 22*60, 6*60))
	require.False(t, minutesInWindow(6*60, 22*60, 6*60))
	require.False(t, minutesInWindow(12*60, 22*60, 6*60))

	// full day
	require.True(t, minutesInWindow(0, 8*60, 8*60))
	require.True(t, minutesInWindow(23*60+59, 8*60, 8*60))
}

func TestAvailabilitySchedule_DailyDisable(t *testing.T) {
	t.Parallel()

	loc := time.FixedZone("CST", 8*3600)
	account := &Account{
		Status:      StatusActive,
		Schedulable: true,
		Extra: map[string]any{
			"availability_schedule": map[string]any{
				"enabled":  true,
				"timezone": "Asia/Shanghai",
				"rules": []any{
					map[string]any{
						"kind":   "daily",
						"start":  "00:00",
						"end":    "08:00",
						"action": "disable",
					},
				},
			},
		},
	}

	// 03:00 Shanghai → disabled
	now := time.Date(2026, 8, 16, 3, 0, 0, 0, loc)
	forced, ok := account.appliesAvailabilitySchedule(now)
	require.True(t, ok)
	require.False(t, forced)
	require.False(t, accountWithNowSchedulable(account, now))

	// 10:00 Shanghai → no match, stays schedulable
	now = time.Date(2026, 8, 16, 10, 0, 0, 0, loc)
	_, ok = account.appliesAvailabilitySchedule(now)
	require.False(t, ok)
	require.True(t, accountWithNowSchedulable(account, now))
}

func TestAvailabilitySchedule_WeeklyAndFirstMatchWins(t *testing.T) {
	t.Parallel()

	loc := time.FixedZone("CST", 8*3600)
	// 2026-08-17 is Monday
	account := &Account{
		Status:      StatusActive,
		Schedulable: true,
		Extra: map[string]any{
			"availability_schedule": map[string]any{
				"enabled":  true,
				"timezone": "Asia/Shanghai",
				"rules": []any{
					map[string]any{
						"kind":     "weekly",
						"weekdays": []any{1, 2, 3, 4, 5},
						"start":    "00:00",
						"end":      "08:00",
						"action":   "disable",
					},
					map[string]any{
						"kind":   "daily",
						"start":  "00:00",
						"end":    "23:59",
						"action": "enable",
					},
				},
			},
		},
	}

	mondayMorning := time.Date(2026, 8, 17, 3, 0, 0, 0, loc)
	forced, ok := account.appliesAvailabilitySchedule(mondayMorning)
	require.True(t, ok)
	require.False(t, forced, "weekly disable should win over later daily enable")

	saturdayMorning := time.Date(2026, 8, 15, 3, 0, 0, 0, loc) // Saturday
	forced, ok = account.appliesAvailabilitySchedule(saturdayMorning)
	require.True(t, ok)
	require.True(t, forced, "weekly miss → daily enable matches")
}

func TestAvailabilitySchedule_EnableDoesNotBypassManualOff(t *testing.T) {
	t.Parallel()

	loc := time.FixedZone("CST", 8*3600)
	account := &Account{
		Status:      StatusActive,
		Schedulable: false,
		Extra: map[string]any{
			"availability_schedule": map[string]any{
				"enabled": true,
				"rules": []any{
					map[string]any{
						"kind":   "daily",
						"start":  "00:00",
						"end":    "23:59",
						"action": "enable",
					},
				},
			},
		},
	}
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, loc)
	require.False(t, accountWithNowSchedulable(account, now))
}

func TestAvailabilitySchedule_OvernightWindow(t *testing.T) {
	t.Parallel()

	loc := time.FixedZone("CST", 8*3600)
	account := &Account{
		Status:      StatusActive,
		Schedulable: true,
		Extra: map[string]any{
			"availability_schedule": map[string]any{
				"enabled":  true,
				"timezone": "Asia/Shanghai",
				"rules": []any{
					map[string]any{
						"kind":   "daily",
						"start":  "22:00",
						"end":    "06:00",
						"action": "disable",
					},
				},
			},
		},
	}

	require.False(t, accountWithNowSchedulable(account, time.Date(2026, 8, 16, 23, 30, 0, 0, loc)))
	require.False(t, accountWithNowSchedulable(account, time.Date(2026, 8, 16, 1, 0, 0, 0, loc)))
	require.True(t, accountWithNowSchedulable(account, time.Date(2026, 8, 16, 12, 0, 0, 0, loc)))
}

func TestAvailabilitySchedule_DisabledConfigIgnored(t *testing.T) {
	t.Parallel()

	loc := time.FixedZone("CST", 8*3600)
	account := &Account{
		Status:      StatusActive,
		Schedulable: true,
		Extra: map[string]any{
			"availability_schedule": map[string]any{
				"enabled": false,
				"rules": []any{
					map[string]any{
						"kind":   "daily",
						"start":  "00:00",
						"end":    "23:59",
						"action": "disable",
					},
				},
			},
		},
	}
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, loc)
	_, ok := account.appliesAvailabilitySchedule(now)
	require.False(t, ok)
	require.True(t, accountWithNowSchedulable(account, now))
}

func TestValidateAvailabilityScheduleExtra(t *testing.T) {
	t.Parallel()

	require.NoError(t, ValidateAvailabilityScheduleExtra(nil))
	require.NoError(t, ValidateAvailabilityScheduleExtra(map[string]any{}))

	err := ValidateAvailabilityScheduleExtra(map[string]any{
		"availability_schedule": map[string]any{
			"enabled": true,
			"rules": []any{
				map[string]any{
					"kind":   "weekly",
					"start":  "09:00",
					"end":    "18:00",
					"action": "disable",
				},
			},
		},
	})
	require.Error(t, err)

	err = ValidateAvailabilityScheduleExtra(map[string]any{
		"availability_schedule": map[string]any{
			"enabled": true,
			"rules": []any{
				map[string]any{
					"kind":     "weekly",
					"weekdays": []any{1, 8},
					"start":    "09:00",
					"end":      "18:00",
					"action":   "disable",
				},
			},
		},
	})
	require.Error(t, err)

	require.NoError(t, ValidateAvailabilityScheduleExtra(map[string]any{
		"availability_schedule": map[string]any{
			"enabled":  true,
			"timezone": "Asia/Shanghai",
			"rules": []any{
				map[string]any{
					"kind":     "weekly",
					"weekdays": []any{float64(1), float64(7)},
					"start":    "09:00",
					"end":      "18:00",
					"action":   "disable",
				},
			},
		},
	}))
}

func TestNormalizeAvailabilityScheduleExtra(t *testing.T) {
	t.Parallel()

	extra := map[string]any{
		"availability_schedule": map[string]any{
			"enabled": true,
			"rules": []any{
				map[string]any{
					"id":       "r1",
					"kind":     "weekly",
					"weekdays": []any{float64(1), float64(2)},
					"start":    "00:00",
					"end":      "08:00",
					"action":   "disable",
				},
			},
		},
	}
	out, err := NormalizeAvailabilityScheduleExtra(extra)
	require.NoError(t, err)
	schedule := out["availability_schedule"].(map[string]any)
	require.Equal(t, true, schedule["enabled"])
	rules := schedule["rules"].([]any)
	require.Len(t, rules, 1)
	rule := rules[0].(map[string]any)
	require.Equal(t, "weekly", rule["kind"])
	require.Equal(t, "r1", rule["id"])
}

// accountWithNowSchedulable mirrors IsSchedulable but injects `now` into the
// availability schedule branch for deterministic tests.
func accountWithNowSchedulable(a *Account, now time.Time) bool {
	if !a.IsActive() || !a.Schedulable {
		return false
	}
	if a.AutoPauseOnExpired && a.ExpiresAt != nil && !now.Before(*a.ExpiresAt) {
		return false
	}
	if a.OverloadUntil != nil && now.Before(*a.OverloadUntil) {
		return false
	}
	if a.RateLimitResetAt != nil && now.Before(*a.RateLimitResetAt) {
		return false
	}
	if a.TempUnschedulableUntil != nil && now.Before(*a.TempUnschedulableUntil) {
		return false
	}
	if forced, ok := a.appliesAvailabilitySchedule(now); ok {
		return forced
	}
	return true
}
