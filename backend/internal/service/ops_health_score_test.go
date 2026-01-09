//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestComputeDashboardHealthScore_IdleReturns100(t *testing.T) {
	t.Parallel()

	score := computeDashboardHealthScore(time.Now().UTC(), &OpsDashboardOverview{REDACTED)
	require.Equal(t, 100, score)
REDACTED

func TestComputeDashboardHealthScore_DegradesOnBadSignals(t *testing.T) {
	t.Parallel()

	ov := &OpsDashboardOverview{
		RequestCountTotal: 100,
		RequestCountSLA:   100,
		SuccessCount:      90,
		ErrorCountTotal:   10,
		ErrorCountSLA:     10,

		SLA:               0.90,
		ErrorRate:         0.10,
		UpstreamErrorRate: 0.08,

		Duration: OpsPercentiles{P99: intPtr(20_000)REDACTED,
		TTFT:     OpsPercentiles{P99: intPtr(2_000)REDACTED,

		SystemMetrics: &OpsSystemMetricsSnapshot{
			DBOK:                  boolPtr(false),
			RedisOK:               boolPtr(false),
			CPUUsagePercent:       float64Ptr(98.0),
			MemoryUsagePercent:    float64Ptr(97.0),
			DBConnWaiting:         intPtr(3),
			ConcurrencyQueueDepth: intPtr(10),
	REDACTED,
		JobHeartbeats: []*OpsJobHeartbeat{
			{
				JobName:     "job-a",
				LastErrorAt: timePtr(time.Now().UTC().Add(-1 * time.Minute)),
				LastError:   stringPtr("boom"),
		REDACTED,
	REDACTED,
REDACTED

	score := computeDashboardHealthScore(time.Now().UTC(), ov)
	require.Less(t, score, 80)
	require.GreaterOrEqual(t, score, 0)
REDACTED

func timePtr(v time.Time) *time.Time { return &v REDACTED

func stringPtr(v string) *string { return &v REDACTED
