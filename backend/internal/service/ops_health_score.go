package service

import (
	"math"
	"time"
)

// computeDashboardHealthScoreWithThresholds computes a 0-100 health score from
// the metrics returned by the dashboard overview.
//
// Design goals:
// - Backend-owned scoring (UI only displays).
// - Layered scoring: Business Health (70%) + Infrastructure Health (30%)
// - Avoids double-counting (e.g., DB failure affects both infra and business metrics)
// - Conservative + stable: penalize clear degradations; avoid overreacting to missing/idle data.
func computeDashboardHealthScoreWithThresholds(now time.Time, overview *OpsDashboardOverview, thresholds *OpsMetricThresholds) int {
	if overview == nil {
		return 0
	}
	if thresholds == nil {
		thresholds = defaultOpsMetricThresholds()
	}

	// Idle/no-data: avoid showing a "bad" score when there is no traffic.
	// UI can still render a gray/idle state based on QPS + error rate.
	if overview.RequestCountSLA <= 0 && overview.RequestCountTotal <= 0 && overview.ErrorCountTotal <= 0 {
		return 100
	}

	businessHealth := computeBusinessHealth(overview, thresholds)
	infraHealth := computeInfraHealth(now, overview)

	// Weighted combination: 70% business + 30% infrastructure
	score := businessHealth*0.7 + infraHealth*0.3
	return int(math.Round(clampFloat64(score, 0, 100)))
}

// computeBusinessHealth calculates business health score (0-100)
// Components: Error Rate (40%) + TTFT (40%) + SLA (20%)
func computeBusinessHealth(overview *OpsDashboardOverview, thresholds *OpsMetricThresholds) float64 {
	if thresholds == nil {
		thresholds = defaultOpsMetricThresholds()
	}
	defaults := defaultOpsMetricThresholds()

	// Error rate score scales with configured alert thresholds.
	// With defaults (5%), this preserves the previous 1% → 100, 10% → 0 curve.
	errorPct := clampFloat64(overview.ErrorRate*100, 0, 100)
	upstreamPct := clampFloat64(overview.UpstreamErrorRate*100, 0, 100)
	requestErrorThreshold := clampFloat64(
		thresholdOrDefault(thresholds.RequestErrorRatePercentMax, defaults.RequestErrorRatePercentMax),
		0,
		100,
	)
	upstreamErrorThreshold := clampFloat64(
		thresholdOrDefault(thresholds.UpstreamErrorRatePercentMax, defaults.UpstreamErrorRatePercentMax),
		0,
		100,
	)
	errorScore := math.Min(
		scoreHighIsBad(errorPct, requestErrorThreshold*0.2, requestErrorThreshold*2),
		scoreHighIsBad(upstreamPct, upstreamErrorThreshold*0.2, upstreamErrorThreshold*2),
	)

	// TTFT score scales with configured max threshold.
	// With default (500ms), this preserves the previous 1s → 100, 3s → 0 curve.
	ttftScore := 100.0
	if overview.TTFT.P99 != nil {
		p99 := float64(*overview.TTFT.P99)
		ttftThreshold := math.Max(0, thresholdOrDefault(thresholds.TTFTp99MsMax, defaults.TTFTp99MsMax))
		ttftScore = scoreHighIsBad(p99, ttftThreshold*2, ttftThreshold*6)
	}

	slaScore := 100.0
	hasSLASample := overview.RequestCountSLA > 0
	if hasSLASample {
		slaPercent := clampFloat64(overview.SLA*100, 0, 100)
		slaThreshold := clampFloat64(
			thresholdOrDefault(thresholds.SLAPercentMin, defaults.SLAPercentMin),
			0,
			100,
		)
		// Keep the configured boundary authoritative, while allowing the score
		// to decay over a small deficit rather than dropping discontinuously.
		slaZeroDeficit := math.Max(0.1, slaThreshold*0.02)
		slaScore = scoreHighIsBad(slaThreshold-slaPercent, 0, slaZeroDeficit)
	}

	if !hasSLASample {
		// Preserve the historical error/TTFT split when this window has no SLA
		// sample; otherwise the mere absence of SLA data would rescale both.
		return errorScore*0.5 + ttftScore*0.5
	}
	return errorScore*0.4 + ttftScore*0.4 + slaScore*0.2
}

func scoreHighIsBad(v float64, goodAtOrBelow float64, zeroAtOrAbove float64) float64 {
	if zeroAtOrAbove <= goodAtOrBelow {
		if v >= zeroAtOrAbove {
			return 0
		}
		return 100
	}
	if v <= goodAtOrBelow {
		return 100
	}
	if v >= zeroAtOrAbove {
		return 0
	}
	return (zeroAtOrAbove - v) / (zeroAtOrAbove - goodAtOrBelow) * 100
}

func thresholdOrDefault(value *float64, fallback *float64) float64 {
	if value != nil {
		return *value
	}
	if fallback != nil {
		return *fallback
	}
	return 0
}

// computeInfraHealth calculates infrastructure health score (0-100)
// Components: Storage (40%) + Compute Resources (30%) + Background Jobs (30%)
func computeInfraHealth(now time.Time, overview *OpsDashboardOverview) float64 {
	// Storage score: DB critical, Redis less critical
	storageScore := 100.0
	if overview.SystemMetrics != nil {
		if overview.SystemMetrics.DBOK != nil && !*overview.SystemMetrics.DBOK {
			storageScore = 0 // DB failure is critical
		} else if overview.SystemMetrics.RedisOK != nil && !*overview.SystemMetrics.RedisOK {
			storageScore = 50 // Redis failure is degraded but not critical
		}
	}

	// Compute resources score: CPU + Memory
	computeScore := 100.0
	if overview.SystemMetrics != nil {
		cpuScore := 100.0
		if overview.SystemMetrics.CPUUsagePercent != nil {
			cpuPct := clampFloat64(*overview.SystemMetrics.CPUUsagePercent, 0, 100)
			if cpuPct > 80 {
				if cpuPct <= 100 {
					cpuScore = (100 - cpuPct) / 20 * 100
				} else {
					cpuScore = 0
				}
			}
		}

		memScore := 100.0
		if overview.SystemMetrics.MemoryUsagePercent != nil {
			memPct := clampFloat64(*overview.SystemMetrics.MemoryUsagePercent, 0, 100)
			if memPct > 85 {
				if memPct <= 100 {
					memScore = (100 - memPct) / 15 * 100
				} else {
					memScore = 0
				}
			}
		}

		computeScore = (cpuScore + memScore) / 2
	}

	// Background jobs score
	jobScore := 100.0
	failedJobs := 0
	totalJobs := 0
	for _, hb := range overview.JobHeartbeats {
		if hb == nil {
			continue
		}
		totalJobs++
		if hb.LastErrorAt != nil && (hb.LastSuccessAt == nil || hb.LastErrorAt.After(*hb.LastSuccessAt)) {
			failedJobs++
		} else if hb.LastSuccessAt != nil && now.Sub(*hb.LastSuccessAt) > 15*time.Minute {
			failedJobs++
		}
	}
	if totalJobs > 0 && failedJobs > 0 {
		jobScore = (1 - float64(failedJobs)/float64(totalJobs)) * 100
	}

	// Weighted combination
	return storageScore*0.4 + computeScore*0.3 + jobScore*0.3
}

func clampFloat64(v float64, min float64, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
