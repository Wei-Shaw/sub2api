package service

import (
	"context"
	"log"
	"runtime"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
)

const (
	opsMetricsInterval       = 1 * time.Minute
	opsMetricsCollectTimeout = 10 * time.Second

	opsMetricsWindowShortMinutes = 1
	opsMetricsWindowLongMinutes  = 5

	bytesPerMB             = 1024 * 1024
	cpuUsageSampleInterval = 0 * time.Second

	percentScale = 100
)

type OpsMetricsCollector struct {
	opsService         *OpsService
	concurrencyService *ConcurrencyService
	interval           time.Duration
	lastGCPauseTotal   uint64
	lastGCPauseMu      sync.Mutex
	stopCh             chan struct{REDACTED
	startOnce          sync.Once
	stopOnce           sync.Once
REDACTED

func NewOpsMetricsCollector(opsService *OpsService, concurrencyService *ConcurrencyService) *OpsMetricsCollector {
	return &OpsMetricsCollector{
		opsService:         opsService,
		concurrencyService: concurrencyService,
		interval:           opsMetricsInterval,
REDACTED
REDACTED

func (c *OpsMetricsCollector) Start() {
	if c == nil {
		return
REDACTED
	c.startOnce.Do(func() {
		if c.stopCh == nil {
			c.stopCh = make(chan struct{REDACTED)
	REDACTED
		go c.run()
REDACTED)
REDACTED

func (c *OpsMetricsCollector) Stop() {
	if c == nil {
		return
REDACTED
	c.stopOnce.Do(func() {
		if c.stopCh != nil {
			close(c.stopCh)
	REDACTED
REDACTED)
REDACTED

func (c *OpsMetricsCollector) run() {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	c.collectOnce()
	for {
		select {
		case <-ticker.C:
			c.collectOnce()
		case <-c.stopCh:
			return
	REDACTED
REDACTED
REDACTED

func (c *OpsMetricsCollector) collectOnce() {
	if c.opsService == nil {
		return
REDACTED

	ctx, cancel := context.WithTimeout(context.Background(), opsMetricsCollectTimeout)
	defer cancel()

	now := time.Now()
	systemStats := c.collectSystemStats(ctx)
	queueDepth := c.collectQueueDepth(ctx)
	activeAlerts := c.collectActiveAlerts(ctx)

	for _, window := range []int{opsMetricsWindowShortMinutes, opsMetricsWindowLongMinutesREDACTED {
		startTime := now.Add(-time.Duration(window) * time.Minute)
		windowStats, err := c.opsService.GetWindowStats(ctx, startTime, now)
		if err != nil {
			log.Printf("[OpsMetrics] failed to get window stats (%dm): %v", window, err)
			continue
	REDACTED

		successRate, errorRate := computeRates(windowStats.SuccessCount, windowStats.ErrorCount)
		requestCount := windowStats.SuccessCount + windowStats.ErrorCount
		metric := &OpsMetrics{
			WindowMinutes:         window,
			RequestCount:          requestCount,
			SuccessCount:          windowStats.SuccessCount,
			ErrorCount:            windowStats.ErrorCount,
			SuccessRate:           successRate,
			ErrorRate:             errorRate,
			P95LatencyMs:          windowStats.P95LatencyMs,
			P99LatencyMs:          windowStats.P99LatencyMs,
			HTTP2Errors:           windowStats.HTTP2Errors,
			ActiveAlerts:          activeAlerts,
			CPUUsagePercent:       systemStats.cpuUsage,
			MemoryUsedMB:          systemStats.memoryUsedMB,
			MemoryTotalMB:         systemStats.memoryTotalMB,
			MemoryUsagePercent:    systemStats.memoryUsagePercent,
			HeapAllocMB:           systemStats.heapAllocMB,
			GCPauseMs:             systemStats.gcPauseMs,
			ConcurrencyQueueDepth: queueDepth,
			UpdatedAt:             now,
	REDACTED

		if err := c.opsService.RecordMetrics(ctx, metric); err != nil {
			log.Printf("[OpsMetrics] failed to record metrics (%dm): %v", window, err)
	REDACTED
REDACTED

REDACTED

func computeRates(successCount, errorCount int64) (float64, float64) {
	total := successCount + errorCount
	if total == 0 {
		// No traffic => no data. Rates are kept at 0 and request_count will be 0.
		// The UI should render this as N/A instead of "100% success".
		return 0, 0
REDACTED
	successRate := float64(successCount) / float64(total) * percentScale
	errorRate := float64(errorCount) / float64(total) * percentScale
	return successRate, errorRate
REDACTED

type opsSystemStats struct {
	cpuUsage           float64
	memoryUsedMB       int64
	memoryTotalMB      int64
	memoryUsagePercent float64
	heapAllocMB        int64
	gcPauseMs          float64
REDACTED

func (c *OpsMetricsCollector) collectSystemStats(ctx context.Context) opsSystemStats {
	stats := opsSystemStats{REDACTED

	if percents, err := cpu.PercentWithContext(ctx, cpuUsageSampleInterval, false); err == nil && len(percents) > 0 {
		stats.cpuUsage = percents[0]
REDACTED

	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		stats.memoryUsedMB = int64(vm.Used / bytesPerMB)
		stats.memoryTotalMB = int64(vm.Total / bytesPerMB)
		stats.memoryUsagePercent = vm.UsedPercent
REDACTED

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	stats.heapAllocMB = int64(memStats.HeapAlloc / bytesPerMB)
	c.lastGCPauseMu.Lock()
	if c.lastGCPauseTotal != 0 && memStats.PauseTotalNs >= c.lastGCPauseTotal {
		stats.gcPauseMs = float64(memStats.PauseTotalNs-c.lastGCPauseTotal) / float64(time.Millisecond)
REDACTED
	c.lastGCPauseTotal = memStats.PauseTotalNs
	c.lastGCPauseMu.Unlock()

	return stats
REDACTED

func (c *OpsMetricsCollector) collectQueueDepth(ctx context.Context) int {
	if c.concurrencyService == nil {
		return 0
REDACTED
	depth, err := c.concurrencyService.GetTotalWaitCount(ctx)
	if err != nil {
		log.Printf("[OpsMetrics] failed to get queue depth: %v", err)
		return 0
REDACTED
	return depth
REDACTED

func (c *OpsMetricsCollector) collectActiveAlerts(ctx context.Context) int {
	if c.opsService == nil {
		return 0
REDACTED
	count, err := c.opsService.CountActiveAlerts(ctx)
	if err != nil {
		return 0
REDACTED
	return count
REDACTED
