package service

import (
	"math"
	"time"
)

// computeDashboardHealthScore computes a 0-100 health score from the metrics returned by the dashboard overview.
//
// Design goals:
// - Backend-owned scoring (UI only displays).
// - Uses "overall" business indicators (SLA/error/latency) plus infra indicators (db/redis/cpu/mem/jobs).
// - Conservative + stable: penalize clear degradations; avoid overreacting to missing/idle data.
func computeDashboardHealthScore(now time.Time, overview *OpsDashboardOverview) int {
	if overview == nil {
		return 0
REDACTED

	// Idle/no-data: avoid showing a "bad" score when there is no traffic.
	// UI can still render a gray/idle state based on QPS + error rate.
	if overview.RequestCountSLA <= 0 && overview.RequestCountTotal <= 0 && overview.ErrorCountTotal <= 0 {
		return 100
REDACTED

	score := 100.0

	// --- SLA (primary signal) ---
	// SLA is a ratio (0..1). Target is intentionally modest for LLM gateways; it can be tuned later.
	slaPct := clampFloat64(overview.SLA*100, 0, 100)
	if slaPct < 99.5 {
		// Up to -45 points as SLA drops.
		score -= math.Min(45, (99.5-slaPct)*12)
REDACTED

	// --- Error rates (secondary signal) ---
	errorPct := clampFloat64(overview.ErrorRate*100, 0, 100)
	if errorPct > 1 {
		// Cap at -20 points by 6% error rate.
		score -= math.Min(20, (errorPct-1)*4)
REDACTED

	upstreamPct := clampFloat64(overview.UpstreamErrorRate*100, 0, 100)
	if upstreamPct > 1 {
		// Upstream instability deserves extra weight, but keep it smaller than SLA/error.
		score -= math.Min(15, (upstreamPct-1)*3)
REDACTED

	// --- Latency (tail-focused) ---
	// Use p99 of duration + TTFT. Penalize only when clearly elevated.
	if overview.Duration.P99 != nil {
		p99 := float64(*overview.Duration.P99)
		if p99 > 2000 {
			// From 2s upward, gradually penalize up to -20.
			score -= math.Min(20, (p99-2000)/900) // ~20s => ~-20
	REDACTED
REDACTED
	if overview.TTFT.P99 != nil {
		p99 := float64(*overview.TTFT.P99)
		if p99 > 500 {
			// TTFT > 500ms starts hurting; cap at -10.
			score -= math.Min(10, (p99-500)/200) // 2.5s => -10
	REDACTED
REDACTED

	// --- System metrics snapshot (best-effort) ---
	if overview.SystemMetrics != nil {
		if overview.SystemMetrics.DBOK != nil && !*overview.SystemMetrics.DBOK {
			score -= 20
	REDACTED
		if overview.SystemMetrics.RedisOK != nil && !*overview.SystemMetrics.RedisOK {
			score -= 15
	REDACTED

		if overview.SystemMetrics.CPUUsagePercent != nil {
			cpuPct := clampFloat64(*overview.SystemMetrics.CPUUsagePercent, 0, 100)
			if cpuPct > 85 {
				score -= math.Min(10, (cpuPct-85)*1.5)
		REDACTED
	REDACTED
		if overview.SystemMetrics.MemoryUsagePercent != nil {
			memPct := clampFloat64(*overview.SystemMetrics.MemoryUsagePercent, 0, 100)
			if memPct > 90 {
				score -= math.Min(10, (memPct-90)*1.0)
		REDACTED
	REDACTED

		if overview.SystemMetrics.DBConnWaiting != nil && *overview.SystemMetrics.DBConnWaiting > 0 {
			waiting := float64(*overview.SystemMetrics.DBConnWaiting)
			score -= math.Min(10, waiting*2)
	REDACTED
		if overview.SystemMetrics.ConcurrencyQueueDepth != nil && *overview.SystemMetrics.ConcurrencyQueueDepth > 0 {
			depth := float64(*overview.SystemMetrics.ConcurrencyQueueDepth)
			score -= math.Min(10, depth*0.5)
	REDACTED
REDACTED

	// --- Job heartbeats (best-effort) ---
	// Penalize only clear "error after last success" signals, and cap the impact.
	jobPenalty := 0.0
	for _, hb := range overview.JobHeartbeats {
		if hb == nil {
			continue
	REDACTED
		if hb.LastErrorAt != nil && (hb.LastSuccessAt == nil || hb.LastErrorAt.After(*hb.LastSuccessAt)) {
			jobPenalty += 5
			continue
	REDACTED
		if hb.LastSuccessAt != nil && now.Sub(*hb.LastSuccessAt) > 15*time.Minute {
			jobPenalty += 2
	REDACTED
REDACTED
	score -= math.Min(15, jobPenalty)

	score = clampFloat64(score, 0, 100)
	return int(math.Round(score))
REDACTED

func clampFloat64(v float64, min float64, max float64) float64 {
	if v < min {
		return min
REDACTED
	if v > max {
		return max
REDACTED
	return v
REDACTED
