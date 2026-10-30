package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCodexUsageUsesActualMonthlyWindow(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	monthlyMinutes, shortMinutes := 30*24*60, 5*60
	monthlyReset, shortReset := 20*24*60*60, 60*60
	monthlyUsed, shortUsed := 100.0, 25.0
	for _, tc := range []struct {
		name     string
		snapshot OpenAICodexUsageSnapshot
	}{
		{"single monthly", OpenAICodexUsageSnapshot{PrimaryUsedPercent: &monthlyUsed, PrimaryWindowMinutes: &monthlyMinutes, PrimaryResetAfterSeconds: &monthlyReset}},
		{"monthly primary", OpenAICodexUsageSnapshot{PrimaryUsedPercent: &monthlyUsed, PrimaryWindowMinutes: &monthlyMinutes, PrimaryResetAfterSeconds: &monthlyReset, SecondaryUsedPercent: &shortUsed, SecondaryWindowMinutes: &shortMinutes, SecondaryResetAfterSeconds: &shortReset}},
		{"monthly secondary", OpenAICodexUsageSnapshot{SecondaryUsedPercent: &monthlyUsed, SecondaryWindowMinutes: &monthlyMinutes, SecondaryResetAfterSeconds: &monthlyReset, PrimaryUsedPercent: &shortUsed, PrimaryWindowMinutes: &shortMinutes, PrimaryResetAfterSeconds: &shortReset}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			extra := buildCodexUsageExtraUpdates(&tc.snapshot, now)
			usage := &UsageInfo{}
			applyExtraToUsage(usage, extra, now)
			require.NotNil(t, usage.SevenDay)
			require.Equal(t, monthlyMinutes, usage.SevenDay.WindowMinutes)
			require.Equal(t, monthlyUsed, usage.SevenDay.Utilization)
			require.Equal(t, monthlyReset, usage.SevenDay.RemainingSeconds)
			require.Equal(t, now.Add(-10*24*time.Hour), codexWindowStatsStart(usage.SevenDay, 7*24*time.Hour, now))
			if tc.name != "single monthly" {
				require.Equal(t, shortMinutes, usage.FiveHour.WindowMinutes)
				require.Equal(t, now.Add(-4*time.Hour), codexWindowStatsStart(usage.FiveHour, 5*time.Hour, now))
			}
			applyExtraToUsage(usage, extra, now.Add(21*24*time.Hour))
			require.Zero(t, usage.SevenDay.Utilization)
			require.Equal(t, monthlyMinutes, usage.SevenDay.WindowMinutes)
		})
	}
}

func TestCodexWindowStatsNeverStartsInFuture(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	reset := now.Add(20 * 24 * time.Hour)
	for _, minutes := range []int{0, -1, 367 * 24 * 60} {
		progress := &UsageProgress{WindowMinutes: minutes, ResetsAt: &reset}
		require.Equal(t, now.Add(-7*24*time.Hour), codexWindowStatsStart(progress, 7*24*time.Hour, now))
	}
	progress := &UsageProgress{WindowMinutes: 30 * 24 * 60}
	require.Equal(t, now.Add(-30*24*time.Hour), codexWindowStatsStart(progress, 7*24*time.Hour, now))
}
