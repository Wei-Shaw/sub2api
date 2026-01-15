package service

import (
	"math"
	"time"
)

// computeDashboardHealthScore computes a 0-100 health score from the metrics returned by the dashboard overview.
//
// Design goals:
// - Backend-owned scoring (UI only displays).
// - Layered scoring: Business Health (80%) + Infrastructure Health (20%)
// - Avoids double-counting (e.g., DB failure affects both infra and business metrics)
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

	businessHealth := computeBusinessHealth(overview)
	infraHealth := computeInfraHealth(now, overview)

	// Weighted combination: 80% business + 20% infrastructure
	score := businessHealth*0.8 + infraHealth*0.2
	return int(math.Round(clampFloat64(score, 0, 100)))
REDACTED

// computeBusinessHealth calculates business health score (0-100)
// Components: Error Rate (50%) + TTFT (50%)
func computeBusinessHealth(overview *OpsDashboardOverview) float64 {
	// Error rate score: 1% → 100, 10% → 0 (linear)
	// Combines request errors and upstream errors
	errorScore := 100.0
	errorPct := clampFloat64(overview.ErrorRate*100, 0, 100)
	upstreamPct := clampFloat64(overview.UpstreamErrorRate*100, 0, 100)
	combinedErrorPct := math.Max(errorPct, upstreamPct) // Use worst case
	if combinedErrorPct > 1.0 {
		if combinedErrorPct <= 10.0 {
			errorScore = (10.0 - combinedErrorPct) / 9.0 * 100
	REDACTED else {
			errorScore = 0
	REDACTED
REDACTED

	// TTFT score: 1s → 100, 3s → 0 (linear)
	// Time to first token is critical for user experience
	ttftScore := 100.0
	if overview.TTFT.P99 != nil {
		p99 := float64(*overview.TTFT.P99)
		if p99 > 1000 {
			if p99 <= 3000 {
				ttftScore = (3000 - p99) / 2000 * 100
		REDACTED else {
				ttftScore = 0
		REDACTED
	REDACTED
REDACTED

	// Weighted combination: 50% error rate + 50% TTFT
	return errorScore*0.5 + ttftScore*0.5
REDACTED

// computeInfraHealth calculates infrastructure health score (0-100)
// Components: Storage (40%) + Compute Resources (30%) + Background Jobs (30%)
func computeInfraHealth(now time.Time, overview *OpsDashboardOverview) float64 {
	// Storage score: DB critical, Redis less critical
	storageScore := 100.0
	if overview.SystemMetrics != nil {
		if overview.SystemMetrics.DBOK != nil && !*overview.SystemMetrics.DBOK {
			storageScore = 0 // DB failure is critical
	REDACTED else if overview.SystemMetrics.RedisOK != nil && !*overview.SystemMetrics.RedisOK {
			storageScore = 50 // Redis failure is degraded but not critical
	REDACTED
REDACTED

	// Compute resources score: CPU + Memory
	computeScore := 100.0
	if overview.SystemMetrics != nil {
		cpuScore := 100.0
		if overview.SystemMetrics.CPUUsagePercent != nil {
			cpuPct := clampFloat64(*overview.SystemMetrics.CPUUsagePercent, 0, 100)
			if cpuPct > 80 {
				if cpuPct <= 100 {
					cpuScore = (100 - cpuPct) / 20 * 100
			REDACTED else {
					cpuScore = 0
			REDACTED
		REDACTED
	REDACTED

		memScore := 100.0
		if overview.SystemMetrics.MemoryUsagePercent != nil {
			memPct := clampFloat64(*overview.SystemMetrics.MemoryUsagePercent, 0, 100)
			if memPct > 85 {
				if memPct <= 100 {
					memScore = (100 - memPct) / 15 * 100
			REDACTED else {
					memScore = 0
			REDACTED
		REDACTED
	REDACTED

		computeScore = (cpuScore + memScore) / 2
REDACTED

	// Background jobs score
	jobScore := 100.0
	failedJobs := 0
	totalJobs := 0
	for _, hb := range overview.JobHeartbeats {
		if hb == nil {
			continue
	REDACTED
		totalJobs++
		if hb.LastErrorAt != nil && (hb.LastSuccessAt == nil || hb.LastErrorAt.After(*hb.LastSuccessAt)) {
			failedJobs++
	REDACTED else if hb.LastSuccessAt != nil && now.Sub(*hb.LastSuccessAt) > 15*time.Minute {
			failedJobs++
	REDACTED
REDACTED
	if totalJobs > 0 && failedJobs > 0 {
		jobScore = (1 - float64(failedJobs)/float64(totalJobs)) * 100
REDACTED

	// Weighted combination
	return storageScore*0.4 + computeScore*0.3 + jobScore*0.3
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
