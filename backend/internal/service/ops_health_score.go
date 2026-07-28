package service

import (
	"math"
	"time"
)

const (
	opsHealthScoreModeFull      = "business_and_infrastructure"
	opsHealthScoreModeInfraOnly = "infrastructure_only"
	opsHealthScoreModeUnknown   = "unavailable"
)

const (
	opsHealthComponentBusinessQuality = "business_quality"
	opsHealthComponentBusinessLatency = "business_latency"
	opsHealthComponentStorage         = "infrastructure_storage"
	opsHealthComponentCompute         = "infrastructure_compute"
	opsHealthComponentJobs            = "infrastructure_jobs"
)

type opsHealthComponentCalculation struct {
	key     string
	score   float64
	reasons []*OpsHealthScoreReason
}

type opsHealthScoreResult struct {
	Score     int
	Breakdown *OpsHealthScoreBreakdown
}

// computeDashboardHealthScoreResult computes a 0-100 health score and the
// deductions that explain it from the dashboard overview.
//
// Design goals:
// - Backend-owned scoring (UI only displays).
// - Layered scoring: Business Health (70%) + Infrastructure Health (30%)
// - Avoids double-counting (e.g., DB failure affects both infra and business metrics)
// - Conservative + stable: penalize clear degradations; avoid overreacting to missing/idle data.
func computeDashboardHealthScoreResult(now time.Time, overview *OpsDashboardOverview, thresholds *OpsMetricThresholds) opsHealthScoreResult {
	if overview == nil {
		return opsHealthScoreResult{
			Score: 0,
			Breakdown: &OpsHealthScoreBreakdown{
				Mode:       opsHealthScoreModeUnknown,
				Score:      0,
				Components: []*OpsHealthScoreComponent{},
			},
		}
	}

	businessQuality, businessLatency := computeBusinessHealthComponents(overview, thresholds)
	storage, compute, jobs := computeInfraHealthComponents(now, overview)
	businessIncluded := overview.RequestCountSLA > 0 || overview.RequestCountTotal > 0 || overview.ErrorCountTotal > 0

	type weightedComponent struct {
		calculation opsHealthComponentCalculation
		weight      float64
	}
	weighted := make([]weightedComponent, 0, 5)
	mode := opsHealthScoreModeInfraOnly
	if businessIncluded {
		mode = opsHealthScoreModeFull
		weighted = append(weighted,
			weightedComponent{calculation: businessQuality, weight: 0.35},
			weightedComponent{calculation: businessLatency, weight: 0.35},
			weightedComponent{calculation: storage, weight: 0.12},
			weightedComponent{calculation: compute, weight: 0.09},
			weightedComponent{calculation: jobs, weight: 0.09},
		)
	} else {
		weighted = append(weighted,
			weightedComponent{calculation: storage, weight: 0.4},
			weightedComponent{calculation: compute, weight: 0.3},
			weightedComponent{calculation: jobs, weight: 0.3},
		)
	}

	rawScore := 0.0
	components := make([]*OpsHealthScoreComponent, 0, len(weighted))
	for _, item := range weighted {
		rawScore += item.calculation.score * item.weight
		maxPoints := item.weight * 100
		earnedPoints := item.calculation.score * item.weight
		reasons := item.calculation.reasons
		if reasons == nil {
			reasons = []*OpsHealthScoreReason{}
		}
		components = append(components, &OpsHealthScoreComponent{
			Key:             item.calculation.key,
			Score:           roundHealthScoreValue(item.calculation.score),
			Weight:          item.weight,
			MaxPoints:       roundHealthScoreValue(maxPoints),
			EarnedPoints:    roundHealthScoreValue(earnedPoints),
			DeductionPoints: roundHealthScoreValue(maxPoints - earnedPoints),
			Reasons:         reasons,
		})
	}
	score := int(math.Round(clampFloat64(rawScore, 0, 100)))

	// Idle/no-data removes business-health evidence, but it must never hide
	// infrastructure degradation such as a failed database or stale jobs.
	return opsHealthScoreResult{
		Score: score,
		Breakdown: &OpsHealthScoreBreakdown{
			Mode:             mode,
			BusinessIncluded: businessIncluded,
			Score:            score,
			Components:       components,
		},
	}
}

func roundHealthScoreValue(value float64) float64 {
	return math.Round(clampFloat64(value, 0, 100)*10) / 10
}

func healthScoreValuePtr(value float64) *float64 {
	return &value
}

func computeBusinessHealthComponents(overview *OpsDashboardOverview, thresholds *OpsMetricThresholds) (opsHealthComponentCalculation, opsHealthComponentCalculation) {
	quality := opsHealthComponentCalculation{
		key:     opsHealthComponentBusinessQuality,
		score:   100,
		reasons: []*OpsHealthScoreReason{},
	}
	latency := opsHealthComponentCalculation{
		key:     opsHealthComponentBusinessLatency,
		score:   100,
		reasons: []*OpsHealthScoreReason{},
	}

	errorPct := clampFloat64(overview.ErrorRate*100, 0, 100)
	upstreamPct := clampFloat64(overview.UpstreamErrorRate*100, 0, 100)
	if thresholds == nil {
		combinedErrorPct := math.Max(errorPct, upstreamPct)
		if combinedErrorPct > 1 {
			if combinedErrorPct <= 10 {
				quality.score = (10 - combinedErrorPct) / 9 * 100
			} else {
				quality.score = 0
			}
		}
		if errorPct > 1 {
			quality.reasons = append(quality.reasons, &OpsHealthScoreReason{
				Code:      "request_error_rate_high",
				Value:     healthScoreValuePtr(errorPct),
				Threshold: healthScoreValuePtr(1),
			})
		}
		if upstreamPct > 1 {
			quality.reasons = append(quality.reasons, &OpsHealthScoreReason{
				Code:      "upstream_error_rate_high",
				Value:     healthScoreValuePtr(upstreamPct),
				Threshold: healthScoreValuePtr(1),
			})
		}

		if overview.TTFT.P99 != nil {
			p99 := float64(*overview.TTFT.P99)
			if p99 > 1000 {
				if p99 <= 3000 {
					latency.score = (3000 - p99) / 2000 * 100
				} else {
					latency.score = 0
				}
				latency.reasons = append(latency.reasons, &OpsHealthScoreReason{
					Code:      "ttft_p99_high",
					Value:     healthScoreValuePtr(p99),
					Threshold: healthScoreValuePtr(1000),
				})
			}
		}
		return quality, latency
	}

	errorLimit := 5.0
	if thresholds.RequestErrorRatePercentMax != nil && *thresholds.RequestErrorRatePercentMax > 0 {
		errorLimit = *thresholds.RequestErrorRatePercentMax
	}
	upstreamLimit := errorLimit
	if thresholds.UpstreamErrorRatePercentMax != nil && *thresholds.UpstreamErrorRatePercentMax > 0 {
		upstreamLimit = *thresholds.UpstreamErrorRatePercentMax
	}
	errorScore := thresholdHealthScore(errorPct, errorLimit)
	upstreamScore := thresholdHealthScore(upstreamPct, upstreamLimit)
	quality.score = math.Min(errorScore, upstreamScore)
	if errorScore < 100 {
		quality.reasons = append(quality.reasons, &OpsHealthScoreReason{
			Code:      "request_error_rate_high",
			Value:     healthScoreValuePtr(errorPct),
			Threshold: healthScoreValuePtr(errorLimit * 0.8),
		})
	}
	if upstreamScore < 100 {
		quality.reasons = append(quality.reasons, &OpsHealthScoreReason{
			Code:      "upstream_error_rate_high",
			Value:     healthScoreValuePtr(upstreamPct),
			Threshold: healthScoreValuePtr(upstreamLimit * 0.8),
		})
	}

	if overview.TTFT.P99 != nil {
		ttftLimit := 500.0
		if thresholds.TTFTp99MsMax != nil && *thresholds.TTFTp99MsMax > 0 {
			ttftLimit = *thresholds.TTFTp99MsMax
		}
		p99 := float64(*overview.TTFT.P99)
		latency.score = thresholdHealthScore(p99, ttftLimit)
		if latency.score < 100 {
			latency.reasons = append(latency.reasons, &OpsHealthScoreReason{
				Code:      "ttft_p99_high",
				Value:     healthScoreValuePtr(p99),
				Threshold: healthScoreValuePtr(ttftLimit * 0.8),
			})
		}
	}
	return quality, latency
}

func thresholdHealthScore(value, criticalThreshold float64) float64 {
	if criticalThreshold <= 0 {
		return 100
	}
	warningThreshold := criticalThreshold * 0.8
	if value <= warningThreshold {
		return 100
	}
	zeroThreshold := criticalThreshold * 2
	if value >= zeroThreshold {
		return 0
	}
	return (zeroThreshold - value) / (zeroThreshold - warningThreshold) * 100
}

func computeInfraHealthComponents(now time.Time, overview *OpsDashboardOverview) (opsHealthComponentCalculation, opsHealthComponentCalculation, opsHealthComponentCalculation) {
	// Storage score: DB critical, Redis less critical
	storage := opsHealthComponentCalculation{
		key:     opsHealthComponentStorage,
		score:   100,
		reasons: []*OpsHealthScoreReason{},
	}
	if overview.SystemMetrics != nil {
		if overview.SystemMetrics.DBOK != nil && !*overview.SystemMetrics.DBOK {
			storage.score = 0 // DB failure is critical
			storage.reasons = append(storage.reasons, &OpsHealthScoreReason{Code: "database_unavailable"})
		} else if overview.SystemMetrics.RedisOK != nil && !*overview.SystemMetrics.RedisOK {
			storage.score = 50 // Redis failure is degraded but not critical
			storage.reasons = append(storage.reasons, &OpsHealthScoreReason{Code: "redis_unavailable"})
		}
	}

	// Compute resources score: CPU + Memory
	compute := opsHealthComponentCalculation{
		key:     opsHealthComponentCompute,
		score:   100,
		reasons: []*OpsHealthScoreReason{},
	}
	if overview.SystemMetrics != nil {
		cpuScore := 100.0
		if overview.SystemMetrics.CPUUsagePercent != nil {
			cpuPct := clampFloat64(*overview.SystemMetrics.CPUUsagePercent, 0, 100)
			if cpuPct > 80 {
				cpuScore = (100 - cpuPct) / 20 * 100
				compute.reasons = append(compute.reasons, &OpsHealthScoreReason{
					Code:      "cpu_usage_high",
					Value:     healthScoreValuePtr(cpuPct),
					Threshold: healthScoreValuePtr(80),
				})
			}
		}

		memScore := 100.0
		if overview.SystemMetrics.MemoryUsagePercent != nil {
			memPct := clampFloat64(*overview.SystemMetrics.MemoryUsagePercent, 0, 100)
			if memPct > 85 {
				memScore = (100 - memPct) / 15 * 100
				compute.reasons = append(compute.reasons, &OpsHealthScoreReason{
					Code:      "memory_usage_high",
					Value:     healthScoreValuePtr(memPct),
					Threshold: healthScoreValuePtr(85),
				})
			}
		}

		compute.score = (cpuScore + memScore) / 2
	}

	// Background jobs score
	jobs := opsHealthComponentCalculation{
		key:     opsHealthComponentJobs,
		score:   100,
		reasons: []*OpsHealthScoreReason{},
	}
	failedJobs := 0
	totalJobs := 0
	for _, hb := range overview.JobHeartbeats {
		if hb == nil {
			continue
		}
		totalJobs++
		if hb.LastErrorAt != nil && (hb.LastSuccessAt == nil || hb.LastErrorAt.After(*hb.LastSuccessAt)) {
			failedJobs++
			jobs.reasons = append(jobs.reasons, &OpsHealthScoreReason{
				Code:    "job_last_run_failed",
				JobName: hb.JobName,
			})
		} else if hb.LastSuccessAt != nil && now.Sub(*hb.LastSuccessAt) > opsJobHeartbeatMaxAge(hb.JobName) {
			failedJobs++
			age := now.Sub(*hb.LastSuccessAt)
			maxAge := opsJobHeartbeatMaxAge(hb.JobName)
			jobs.reasons = append(jobs.reasons, &OpsHealthScoreReason{
				Code:          "job_heartbeat_stale",
				JobName:       hb.JobName,
				AgeSeconds:    healthScoreValuePtr(age.Seconds()),
				MaxAgeSeconds: healthScoreValuePtr(maxAge.Seconds()),
			})
		}
	}
	if totalJobs > 0 && failedJobs > 0 {
		jobs.score = (1 - float64(failedJobs)/float64(totalJobs)) * 100
	}

	return storage, compute, jobs
}

func opsJobHeartbeatMaxAge(jobName string) time.Duration {
	switch jobName {
	case opsAlertEvaluatorJobName:
		return 3 * time.Minute
	case opsMetricsCollectorJobName:
		// The collector interval is configurable up to one hour.
		return 2 * time.Hour
	case opsAggHourlyJobName:
		return 30 * time.Minute
	case opsAggDailyJobName:
		return 2 * time.Hour
	case opsScheduledReportJobName, opsCleanupJobName:
		// These cron-driven jobs may intentionally run only once per day.
		return 36 * time.Hour
	default:
		return 30 * time.Minute
	}
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
