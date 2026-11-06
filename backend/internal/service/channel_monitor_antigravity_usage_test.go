package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMonitorAntigravityQuota_UsesGroupsInsteadOfLocalStats(t *testing.T) {
	used := 95.0
	usage := &UsageInfo{
		AntigravityQuotaSource: "summary",
		FiveHour:               &UsageProgress{WindowStats: &WindowStats{Requests: 3}},
		SevenDay:               &UsageProgress{WindowStats: &WindowStats{Requests: 8}},
		AntigravityQuotaGroups: []AntigravityQuotaGroup{
			{Name: "Gemini Models", Buckets: []AntigravityQuotaBucket{{ID: "gemini-5h", Window: "5h", Disabled: true}}},
			{Name: "Gemini 4 Argon", Buckets: []AntigravityQuotaBucket{{ID: "argon", Window: "weekly", Utilization: &used}}},
		},
		AntigravityQuota: map[string]*AntigravityModelQuota{"gemini-4-argon": {Utilization: 0}},
	}
	tiers := usageQuotaTiers(usage)
	require.Len(t, tiers, 1)
	require.Equal(t, "Gemini 4 Argon", tiers[0].Label)
	require.Equal(t, "weekly", tiers[0].Window)
	require.Equal(t, used, tiers[0].UsedPercent)
	usage.AntigravityQuotaGroups = nil
	usage.AntigravityQuota = nil
	usage.AntigravityQuotaSource = "unavailable"
	require.Empty(t, usageQuotaTiers(usage), "local statistics cannot stand in for provider quota")
}

func TestMonitorAntigravityQuota_StaleAndAuthenticationFailures(t *testing.T) {
	usage := &UsageInfo{AntigravityQuotaError: "quota_summary_unavailable", AntigravityQuotaStale: true}
	failed, invalid, _ := usageFailureInfo(usage)
	require.True(t, failed)
	require.False(t, invalid)
	usage.IsForbidden = true
	_, invalid, _ = usageFailureInfo(usage)
	require.True(t, invalid, "a stale snapshot must not hide current authentication failures")
}
