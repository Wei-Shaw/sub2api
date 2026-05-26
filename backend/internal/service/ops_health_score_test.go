//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestComputeDashboardHealthScore_IdleReturns100(t *testing.T) {
	t.Parallel()

	score := computeDashboardHealthScore(time.Now().UTC(), &OpsDashboardOverview{})
	require.Equal(t, 100, score)
}

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

		Duration: OpsPercentiles{P99: intPtr(20_000)},
		TTFT:     OpsPercentiles{P99: intPtr(2_000)},

		SystemMetrics: &OpsSystemMetricsSnapshot{
			DBOK:                  boolPtr(false),
			RedisOK:               boolPtr(false),
			CPUUsagePercent:       float64Ptr(98.0),
			MemoryUsagePercent:    float64Ptr(97.0),
			DBConnWaiting:         intPtr(3),
			ConcurrencyQueueDepth: intPtr(10),
		},
		JobHeartbeats: []*OpsJobHeartbeat{
			{
				JobName:     "job-a",
				LastErrorAt: timePtr(time.Now().UTC().Add(-1 * time.Minute)),
				LastError:   stringPtr("boom"),
			},
		},
	}

	score := computeDashboardHealthScore(time.Now().UTC(), ov)
	require.Less(t, score, 80)
	require.GreaterOrEqual(t, score, 0)
}

func TestComputeDashboardHealthScore_Comprehensive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		overview *OpsDashboardOverview
		wantMin  int
		wantMax  int
	}{
		{
			name:     "nil overview returns 0",
			overview: nil,
			wantMin:  0,
			wantMax:  0,
		},
		{
			name: "perfect health",
			overview: &OpsDashboardOverview{
				RequestCountTotal: 1000,
				RequestCountSLA:   1000,
				SLA:               1.0,
				ErrorRate:         0,
				UpstreamErrorRate: 0,
				Duration:          OpsPercentiles{P99: intPtr(500)},
				TTFT:              OpsPercentiles{P99: intPtr(100)},
				SystemMetrics: &OpsSystemMetricsSnapshot{
					DBOK:               boolPtr(true),
					RedisOK:            boolPtr(true),
					CPUUsagePercent:    float64Ptr(30),
					MemoryUsagePercent: float64Ptr(40),
				},
			},
			wantMin: 100,
			wantMax: 100,
		},
		{
			name: "good health - SLA 99.8%",
			overview: &OpsDashboardOverview{
				RequestCountTotal: 1000,
				RequestCountSLA:   1000,
				SLA:               0.998,
				ErrorRate:         0.003,
				UpstreamErrorRate: 0.001,
				Duration:          OpsPercentiles{P99: intPtr(800)},
				TTFT:              OpsPercentiles{P99: intPtr(200)},
				SystemMetrics: &OpsSystemMetricsSnapshot{
					DBOK:               boolPtr(true),
					RedisOK:            boolPtr(true),
					CPUUsagePercent:    float64Ptr(50),
					MemoryUsagePercent: float64Ptr(60),
				},
			},
			wantMin: 95,
			wantMax: 100,
		},
		{
			name: "medium health - SLA 96%",
			overview: &OpsDashboardOverview{
				RequestCountTotal: 1000,
				RequestCountSLA:   1000,
				SLA:               0.96,
				ErrorRate:         0.02,
				UpstreamErrorRate: 0.01,
				Duration:          OpsPercentiles{P99: intPtr(3000)},
				TTFT:              OpsPercentiles{P99: intPtr(600)},
				SystemMetrics: &OpsSystemMetricsSnapshot{
					DBOK:               boolPtr(true),
					RedisOK:            boolPtr(true),
					CPUUsagePercent:    float64Ptr(70),
					MemoryUsagePercent: float64Ptr(75),
				},
			},
			wantMin: 96,
			wantMax: 97,
		},
		{
			name: "DB failure",
			overview: &OpsDashboardOverview{
				RequestCountTotal: 1000,
				RequestCountSLA:   1000,
				SLA:               0.995,
				ErrorRate:         0,
				UpstreamErrorRate: 0,
				Duration:          OpsPercentiles{P99: intPtr(500)},
				SystemMetrics: &OpsSystemMetricsSnapshot{
					DBOK:               boolPtr(false),
					RedisOK:            boolPtr(true),
					CPUUsagePercent:    float64Ptr(30),
					MemoryUsagePercent: float64Ptr(40),
				},
			},
			wantMin: 70,
			wantMax: 90,
		},
		{
			name: "Redis failure",
			overview: &OpsDashboardOverview{
				RequestCountTotal: 1000,
				RequestCountSLA:   1000,
				SLA:               0.995,
				ErrorRate:         0,
				UpstreamErrorRate: 0,
				Duration:          OpsPercentiles{P99: intPtr(500)},
				SystemMetrics: &OpsSystemMetricsSnapshot{
					DBOK:               boolPtr(true),
					RedisOK:            boolPtr(false),
					CPUUsagePercent:    float64Ptr(30),
					MemoryUsagePercent: float64Ptr(40),
				},
			},
			wantMin: 85,
			wantMax: 95,
		},
		{
			name: "high CPU usage",
			overview: &OpsDashboardOverview{
				RequestCountTotal: 1000,
				RequestCountSLA:   1000,
				SLA:               0.995,
				ErrorRate:         0,
				UpstreamErrorRate: 0,
				Duration:          OpsPercentiles{P99: intPtr(500)},
				SystemMetrics: &OpsSystemMetricsSnapshot{
					DBOK:               boolPtr(true),
					RedisOK:            boolPtr(true),
					CPUUsagePercent:    float64Ptr(95),
					MemoryUsagePercent: float64Ptr(40),
				},
			},
			wantMin: 85,
			wantMax: 100,
		},
		{
			name: "combined failures - business degraded + infra healthy",
			overview: &OpsDashboardOverview{
				RequestCountTotal: 1000,
				RequestCountSLA:   1000,
				SLA:               0.90,
				ErrorRate:         0.05,
				UpstreamErrorRate: 0.02,
				Duration:          OpsPercentiles{P99: intPtr(10000)},
				SystemMetrics: &OpsSystemMetricsSnapshot{
					DBOK:               boolPtr(true),
					RedisOK:            boolPtr(true),
					CPUUsagePercent:    float64Ptr(20),
					MemoryUsagePercent: float64Ptr(30),
				},
			},
			wantMin: 84,
			wantMax: 85,
		},
		{
			name: "combined failures - business healthy + infra degraded",
			overview: &OpsDashboardOverview{
				RequestCountTotal: 1000,
				RequestCountSLA:   1000,
				SLA:               0.998,
				ErrorRate:         0.001,
				UpstreamErrorRate: 0,
				Duration:          OpsPercentiles{P99: intPtr(600)},
				SystemMetrics: &OpsSystemMetricsSnapshot{
					DBOK:               boolPtr(false),
					RedisOK:            boolPtr(false),
					CPUUsagePercent:    float64Ptr(95),
					MemoryUsagePercent: float64Ptr(95),
				},
			},
			wantMin: 70,
			wantMax: 90,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := computeDashboardHealthScore(time.Now().UTC(), tt.overview)
			require.GreaterOrEqual(t, score, tt.wantMin, "score should be >= %d", tt.wantMin)
			require.LessOrEqual(t, score, tt.wantMax, "score should be <= %d", tt.wantMax)
			require.GreaterOrEqual(t, score, 0, "score must be >= 0")
			require.LessOrEqual(t, score, 100, "score must be <= 100")
		})
	}
}

func TestComputeBusinessHealth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		overview *OpsDashboardOverview
		wantMin  float64
		wantMax  float64
	}{
		{
			name: "perfect metrics",
			overview: &OpsDashboardOverview{
				SLA:               1.0,
				ErrorRate:         0,
				UpstreamErrorRate: 0,
				Duration:          OpsPercentiles{P99: intPtr(500)},
			},
			wantMin: 100,
			wantMax: 100,
		},
		{
			name: "SLA boundary 99.5%",
			overview: &OpsDashboardOverview{
				SLA:               0.995,
				ErrorRate:         0,
				UpstreamErrorRate: 0,
				Duration:          OpsPercentiles{P99: intPtr(500)},
			},
			wantMin: 100,
			wantMax: 100,
		},
		{
			name: "SLA boundary 95%",
			overview: &OpsDashboardOverview{
				SLA:               0.95,
				ErrorRate:         0,
				UpstreamErrorRate: 0,
				Duration:          OpsPercentiles{P99: intPtr(500)},
			},
			wantMin: 100,
			wantMax: 100,
		},
		{
			name: "error rate boundary 1%",
			overview: &OpsDashboardOverview{
				SLA:               0.99,
				ErrorRate:         0.01,
				UpstreamErrorRate: 0,
				Duration:          OpsPercentiles{P99: intPtr(500)},
			},
			wantMin: 100,
			wantMax: 100,
		},
		{
			name: "error rate 5%",
			overview: &OpsDashboardOverview{
				SLA:               0.95,
				ErrorRate:         0.05,
				UpstreamErrorRate: 0,
				Duration:          OpsPercentiles{P99: intPtr(500)},
			},
			wantMin: 77,
			wantMax: 78,
		},
		{
			name: "TTFT boundary 2s",
			overview: &OpsDashboardOverview{
				SLA:               0.99,
				ErrorRate:         0,
				UpstreamErrorRate: 0,
				TTFT:              OpsPercentiles{P99: intPtr(2000)},
			},
			wantMin: 75,
			wantMax: 75,
		},
		{
			name: "upstream error dominates",
			overview: &OpsDashboardOverview{
				SLA:               0.995,
				ErrorRate:         0.001,
				UpstreamErrorRate: 0.03,
				Duration:          OpsPercentiles{P99: intPtr(500)},
			},
			wantMin: 88,
			wantMax: 90,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := computeBusinessHealth(tt.overview)
			require.GreaterOrEqual(t, score, tt.wantMin, "score should be >= %.1f", tt.wantMin)
			require.LessOrEqual(t, score, tt.wantMax, "score should be <= %.1f", tt.wantMax)
			require.GreaterOrEqual(t, score, 0.0, "score must be >= 0")
			require.LessOrEqual(t, score, 100.0, "score must be <= 100")
		})
	}
}

func TestComputeInfraHealth(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()

	tests := []struct {
		name     string
		overview *OpsDashboardOverview
		wantMin  float64
		wantMax  float64
	}{
		{
			name: "all infrastructure healthy",
			overview: &OpsDashboardOverview{
				RequestCountTotal: 1000,
				SystemMetrics: &OpsSystemMetricsSnapshot{
					DBOK:               boolPtr(true),
					RedisOK:            boolPtr(true),
					CPUUsagePercent:    float64Ptr(30),
					MemoryUsagePercent: float64Ptr(40),
				},
			},
			wantMin: 100,
			wantMax: 100,
		},
		{
			name: "DB down",
			overview: &OpsDashboardOverview{
				RequestCountTotal: 1000,
				SystemMetrics: &OpsSystemMetricsSnapshot{
					DBOK:               boolPtr(false),
					RedisOK:            boolPtr(true),
					CPUUsagePercent:    float64Ptr(30),
					MemoryUsagePercent: float64Ptr(40),
				},
			},
			wantMin: 50,
			wantMax: 70,
		},
		{
			name: "Redis down",
			overview: &OpsDashboardOverview{
				RequestCountTotal: 1000,
				SystemMetrics: &OpsSystemMetricsSnapshot{
					DBOK:               boolPtr(true),
					RedisOK:            boolPtr(false),
					CPUUsagePercent:    float64Ptr(30),
					MemoryUsagePercent: float64Ptr(40),
				},
			},
			wantMin: 80,
			wantMax: 95,
		},
		{
			name: "CPU at 90%",
			overview: &OpsDashboardOverview{
				RequestCountTotal: 1000,
				SystemMetrics: &OpsSystemMetricsSnapshot{
					DBOK:               boolPtr(true),
					RedisOK:            boolPtr(true),
					CPUUsagePercent:    float64Ptr(90),
					MemoryUsagePercent: float64Ptr(40),
				},
			},
			wantMin: 85,
			wantMax: 95,
		},
		{
			name: "failed background job",
			overview: &OpsDashboardOverview{
				RequestCountTotal: 1000,
				SystemMetrics: &OpsSystemMetricsSnapshot{
					DBOK:               boolPtr(true),
					RedisOK:            boolPtr(true),
					CPUUsagePercent:    float64Ptr(30),
					MemoryUsagePercent: float64Ptr(40),
				},
				JobHeartbeats: []*OpsJobHeartbeat{
					{
						JobName:     "test-job",
						LastErrorAt: &now,
					},
				},
			},
			wantMin: 70,
			wantMax: 90,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := computeInfraHealth(now, tt.overview)
			require.GreaterOrEqual(t, score, tt.wantMin, "score should be >= %.1f", tt.wantMin)
			require.LessOrEqual(t, score, tt.wantMax, "score should be <= %.1f", tt.wantMax)
			require.GreaterOrEqual(t, score, 0.0, "score must be >= 0")
			require.LessOrEqual(t, score, 100.0, "score must be <= 100")
		})
	}
}

func timePtr(v time.Time) *time.Time { return &v }

func stringPtr(v string) *string { return &v }
