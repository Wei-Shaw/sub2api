package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
)

const (
	opsMetricsCollectorJobName     = "ops_metrics_collector"
	opsMetricsCollectorMinInterval = 60 * time.Second
	opsMetricsCollectorMaxInterval = 1 * time.Hour

	opsMetricsCollectorTimeout = 10 * time.Second

	opsMetricsCollectorLeaderLockKey = "ops:metrics:collector:leader"
	opsMetricsCollectorLeaderLockTTL = 90 * time.Second

	opsMetricsCollectorHeartbeatTimeout = 2 * time.Second

	bytesPerMB = 1024 * 1024
)

var opsMetricsCollectorAdvisoryLockID = hashAdvisoryLockID(opsMetricsCollectorLeaderLockKey)

type opsSchedulableAccountLoadRepository interface {
	ListSchedulableAccountLoads(ctx context.Context) ([]AccountWithConcurrency, error)
REDACTED

type OpsMetricsCollector struct {
	opsRepo     OpsRepository
	settingRepo SettingRepository
	cfg         *config.Config

	accountRepo        AccountRepository
	concurrencyService *ConcurrencyService

	db          *sql.DB
	redisClient *redis.Client
	instanceID  string

	lastCgroupCPUUsageNanos uint64
	lastCgroupCPUSampleAt   time.Time

	stopCh    chan struct{REDACTED
	startOnce sync.Once
	stopOnce  sync.Once

	skipLogMu sync.Mutex
	skipLogAt time.Time
REDACTED

func NewOpsMetricsCollector(
	opsRepo OpsRepository,
	settingRepo SettingRepository,
	accountRepo AccountRepository,
	concurrencyService *ConcurrencyService,
	db *sql.DB,
	redisClient *redis.Client,
	cfg *config.Config,
) *OpsMetricsCollector {
	return &OpsMetricsCollector{
		opsRepo:            opsRepo,
		settingRepo:        settingRepo,
		cfg:                cfg,
		accountRepo:        accountRepo,
		concurrencyService: concurrencyService,
		db:                 db,
		redisClient:        redisClient,
		instanceID:         uuid.NewString(),
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
	// First run immediately so the dashboard has data soon after startup.
	c.collectOnce()

	for {
		interval := c.getInterval()
		timer := time.NewTimer(interval)
		select {
		case <-timer.C:
			c.collectOnce()
		case <-c.stopCh:
			timer.Stop()
			return
	REDACTED
REDACTED
REDACTED

func (c *OpsMetricsCollector) getInterval() time.Duration {
	interval := opsMetricsCollectorMinInterval

	if c.settingRepo == nil {
		return interval
REDACTED

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	raw, err := c.settingRepo.GetValue(ctx, SettingKeyOpsMetricsIntervalSeconds)
	if err != nil {
		return interval
REDACTED
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return interval
REDACTED

	seconds, err := strconv.Atoi(raw)
	if err != nil {
		return interval
REDACTED
	if seconds < int(opsMetricsCollectorMinInterval.Seconds()) {
		seconds = int(opsMetricsCollectorMinInterval.Seconds())
REDACTED
	if seconds > int(opsMetricsCollectorMaxInterval.Seconds()) {
		seconds = int(opsMetricsCollectorMaxInterval.Seconds())
REDACTED
	return time.Duration(seconds) * time.Second
REDACTED

func (c *OpsMetricsCollector) collectOnce() {
	if c == nil {
		return
REDACTED
	if c.cfg != nil && !c.cfg.Ops.Enabled {
		return
REDACTED
	if c.opsRepo == nil {
		return
REDACTED
	if c.db == nil {
		return
REDACTED

	ctx, cancel := context.WithTimeout(context.Background(), opsMetricsCollectorTimeout)
	defer cancel()

	if !c.isMonitoringEnabled(ctx) {
		return
REDACTED

	release, ok := c.tryAcquireLeaderLock(ctx)
	if !ok {
		return
REDACTED
	if release != nil {
		defer release()
REDACTED

	startedAt := time.Now().UTC()
	err := c.collectAndPersist(ctx)
	finishedAt := time.Now().UTC()

	durationMs := finishedAt.Sub(startedAt).Milliseconds()
	dur := durationMs
	runAt := startedAt

	if err != nil {
		msg := truncateString(err.Error(), 2048)
		errAt := finishedAt
		hbCtx, hbCancel := context.WithTimeout(context.Background(), opsMetricsCollectorHeartbeatTimeout)
		defer hbCancel()
		_ = c.opsRepo.UpsertJobHeartbeat(hbCtx, &OpsUpsertJobHeartbeatInput{
			JobName:        opsMetricsCollectorJobName,
			LastRunAt:      &runAt,
			LastErrorAt:    &errAt,
			LastError:      &msg,
			LastDurationMs: &dur,
	REDACTED)
		log.Printf("[OpsMetricsCollector] collect failed: %v", err)
		return
REDACTED

	successAt := finishedAt
	hbCtx, hbCancel := context.WithTimeout(context.Background(), opsMetricsCollectorHeartbeatTimeout)
	defer hbCancel()
	_ = c.opsRepo.UpsertJobHeartbeat(hbCtx, &OpsUpsertJobHeartbeatInput{
		JobName:        opsMetricsCollectorJobName,
		LastRunAt:      &runAt,
		LastSuccessAt:  &successAt,
		LastDurationMs: &dur,
REDACTED)
REDACTED

func (c *OpsMetricsCollector) isMonitoringEnabled(ctx context.Context) bool {
	if c == nil {
		return false
REDACTED
	if c.cfg != nil && !c.cfg.Ops.Enabled {
		return false
REDACTED
	if c.settingRepo == nil {
		return true
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED

	value, err := c.settingRepo.GetValue(ctx, SettingKeyOpsMonitoringEnabled)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return true
	REDACTED
		// Fail-open: collector should not become a hard dependency.
		return true
REDACTED
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "false", "0", "off", "disabled":
		return false
	default:
		return true
REDACTED
REDACTED

func (c *OpsMetricsCollector) collectAndPersist(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
REDACTED

	// Align to stable minute boundaries to avoid partial buckets and to maximize cache hits.
	now := time.Now().UTC()
	windowEnd := now.Truncate(time.Minute)
	windowStart := windowEnd.Add(-1 * time.Minute)

	sys, err := c.collectSystemStats(ctx)
	if err != nil {
		// Continue; system stats are best-effort.
		log.Printf("[OpsMetricsCollector] system stats error: %v", err)
REDACTED

	dbOK := c.checkDB(ctx)
	redisOK := c.checkRedis(ctx)
	active, idle := c.dbPoolStats()
	redisTotal, redisIdle, redisStatsOK := c.redisPoolStats()

	successCount, tokenConsumed, err := c.queryUsageCounts(ctx, windowStart, windowEnd)
	if err != nil {
		return fmt.Errorf("query usage counts: %w", err)
REDACTED

	duration, ttft, err := c.queryUsageLatency(ctx, windowStart, windowEnd)
	if err != nil {
		return fmt.Errorf("query usage latency: %w", err)
REDACTED

	errorTotal, businessLimited, errorSLA, upstreamExcl, upstream429, upstream529, err := c.queryErrorCounts(ctx, windowStart, windowEnd)
	if err != nil {
		return fmt.Errorf("query error counts: %w", err)
REDACTED

	accountSwitchCount, err := c.queryAccountSwitchCount(ctx, windowStart, windowEnd)
	if err != nil {
		return fmt.Errorf("query account switch counts: %w", err)
REDACTED

	windowSeconds := windowEnd.Sub(windowStart).Seconds()
	if windowSeconds <= 0 {
		windowSeconds = 60
REDACTED
	requestTotal := successCount + errorTotal
	qps := float64(requestTotal) / windowSeconds
	tps := float64(tokenConsumed) / windowSeconds

	goroutines := runtime.NumGoroutine()
	concurrencyQueueDepth := c.collectConcurrencyQueueDepth(ctx)

	input := &OpsInsertSystemMetricsInput{
		CreatedAt:     windowEnd,
		WindowMinutes: 1,

		SuccessCount:         successCount,
		ErrorCountTotal:      errorTotal,
		BusinessLimitedCount: businessLimited,
		ErrorCountSLA:        errorSLA,

		UpstreamErrorCountExcl429529: upstreamExcl,
		Upstream429Count:             upstream429,
		Upstream529Count:             upstream529,

		TokenConsumed:      tokenConsumed,
		AccountSwitchCount: accountSwitchCount,
		QPS:                float64Ptr(roundTo1DP(qps)),
		TPS:                float64Ptr(roundTo1DP(tps)),

		DurationP50Ms: duration.p50,
		DurationP90Ms: duration.p90,
		DurationP95Ms: duration.p95,
		DurationP99Ms: duration.p99,
		DurationAvgMs: duration.avg,
		DurationMaxMs: duration.max,

		TTFTP50Ms: ttft.p50,
		TTFTP90Ms: ttft.p90,
		TTFTP95Ms: ttft.p95,
		TTFTP99Ms: ttft.p99,
		TTFTAvgMs: ttft.avg,
		TTFTMaxMs: ttft.max,

		CPUUsagePercent:    sys.cpuUsagePercent,
		MemoryUsedMB:       sys.memoryUsedMB,
		MemoryTotalMB:      sys.memoryTotalMB,
		MemoryUsagePercent: sys.memoryUsagePercent,

		DBOK:    boolPtr(dbOK),
		RedisOK: boolPtr(redisOK),

		RedisConnTotal: func() *int {
			if !redisStatsOK {
				return nil
		REDACTED
			return intPtr(redisTotal)
	REDACTED(),
		RedisConnIdle: func() *int {
			if !redisStatsOK {
				return nil
		REDACTED
			return intPtr(redisIdle)
	REDACTED(),

		DBConnActive:          intPtr(active),
		DBConnIdle:            intPtr(idle),
		GoroutineCount:        intPtr(goroutines),
		ConcurrencyQueueDepth: concurrencyQueueDepth,
REDACTED

	return c.opsRepo.InsertSystemMetrics(ctx, input)
REDACTED

func (c *OpsMetricsCollector) collectConcurrencyQueueDepth(parentCtx context.Context) *int {
	if c == nil || c.accountRepo == nil || c.concurrencyService == nil {
		return nil
REDACTED
	if parentCtx == nil {
		parentCtx = context.Background()
REDACTED

	// Best-effort: never let concurrency sampling break the metrics collector.
	ctx, cancel := context.WithTimeout(parentCtx, 2*time.Second)
	defer cancel()

	accountLoads, err := c.listSchedulableAccountLoads(ctx)
	if err != nil {
		return nil
REDACTED
	if len(accountLoads) == 0 {
		zero := 0
		return &zero
REDACTED

	loadMap, err := c.concurrencyService.GetAccountsLoadBatch(ctx, accountLoads)
	if err != nil {
		return nil
REDACTED

	var total int64
	for _, info := range loadMap {
		if info == nil || info.WaitingCount <= 0 {
			continue
	REDACTED
		total += int64(info.WaitingCount)
REDACTED
	if total < 0 {
		total = 0
REDACTED

	maxInt := int64(^uint(0) >> 1)
	if total > maxInt {
		total = maxInt
REDACTED
	v := int(total)
	return &v
REDACTED

func (c *OpsMetricsCollector) listSchedulableAccountLoads(ctx context.Context) ([]AccountWithConcurrency, error) {
	if repo, ok := c.accountRepo.(opsSchedulableAccountLoadRepository); ok {
		return repo.ListSchedulableAccountLoads(ctx)
REDACTED

	accounts, err := c.accountRepo.ListSchedulable(ctx)
	if err != nil {
		return nil, err
REDACTED
	loads := make([]AccountWithConcurrency, 0, len(accounts))
	for _, account := range accounts {
		if account.ID <= 0 {
			continue
	REDACTED
		loads = append(loads, AccountWithConcurrency{
			ID:             account.ID,
			MaxConcurrency: account.EffectiveLoadFactor(),
	REDACTED)
REDACTED
	return loads, nil
REDACTED

type opsCollectedPercentiles struct {
	p50 *int
	p90 *int
	p95 *int
	p99 *int
	avg *float64
	max *int
REDACTED

func (c *OpsMetricsCollector) queryUsageCounts(ctx context.Context, start, end time.Time) (successCount int64, tokenConsumed int64, err error) {
	q := `
SELECT
  COALESCE(COUNT(*), 0) AS success_count,
  COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) AS token_consumed
FROM usage_logs
WHERE created_at >= $1 AND created_at < $2`

	var tokens sql.NullInt64
	if err := c.db.QueryRowContext(ctx, q, start, end).Scan(&successCount, &tokens); err != nil {
		return 0, 0, err
REDACTED
	if tokens.Valid {
		tokenConsumed = tokens.Int64
REDACTED
	return successCount, tokenConsumed, nil
REDACTED

func (c *OpsMetricsCollector) queryUsageLatency(ctx context.Context, start, end time.Time) (duration opsCollectedPercentiles, ttft opsCollectedPercentiles, err error) {
	{
		q := `
SELECT
  percentile_cont(0.50) WITHIN GROUP (ORDER BY duration_ms) AS p50,
  percentile_cont(0.90) WITHIN GROUP (ORDER BY duration_ms) AS p90,
  percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_ms) AS p95,
  percentile_cont(0.99) WITHIN GROUP (ORDER BY duration_ms) AS p99,
  AVG(duration_ms) AS avg_ms,
  MAX(duration_ms) AS max_ms
FROM usage_logs
WHERE created_at >= $1 AND created_at < $2
  AND duration_ms IS NOT NULL`

		var p50, p90, p95, p99 sql.NullFloat64
		var avg sql.NullFloat64
		var max sql.NullInt64
		if err := c.db.QueryRowContext(ctx, q, start, end).Scan(&p50, &p90, &p95, &p99, &avg, &max); err != nil {
			return opsCollectedPercentiles{REDACTED, opsCollectedPercentiles{REDACTED, err
	REDACTED
		duration.p50 = floatToIntPtr(p50)
		duration.p90 = floatToIntPtr(p90)
		duration.p95 = floatToIntPtr(p95)
		duration.p99 = floatToIntPtr(p99)
		if avg.Valid {
			v := roundTo1DP(avg.Float64)
			duration.avg = &v
	REDACTED
		if max.Valid {
			v := int(max.Int64)
			duration.max = &v
	REDACTED
REDACTED

	{
		q := `
SELECT
  percentile_cont(0.50) WITHIN GROUP (ORDER BY first_token_ms) AS p50,
  percentile_cont(0.90) WITHIN GROUP (ORDER BY first_token_ms) AS p90,
  percentile_cont(0.95) WITHIN GROUP (ORDER BY first_token_ms) AS p95,
  percentile_cont(0.99) WITHIN GROUP (ORDER BY first_token_ms) AS p99,
  AVG(first_token_ms) AS avg_ms,
  MAX(first_token_ms) AS max_ms
FROM usage_logs
WHERE created_at >= $1 AND created_at < $2
  AND first_token_ms IS NOT NULL`

		var p50, p90, p95, p99 sql.NullFloat64
		var avg sql.NullFloat64
		var max sql.NullInt64
		if err := c.db.QueryRowContext(ctx, q, start, end).Scan(&p50, &p90, &p95, &p99, &avg, &max); err != nil {
			return opsCollectedPercentiles{REDACTED, opsCollectedPercentiles{REDACTED, err
	REDACTED
		ttft.p50 = floatToIntPtr(p50)
		ttft.p90 = floatToIntPtr(p90)
		ttft.p95 = floatToIntPtr(p95)
		ttft.p99 = floatToIntPtr(p99)
		if avg.Valid {
			v := roundTo1DP(avg.Float64)
			ttft.avg = &v
	REDACTED
		if max.Valid {
			v := int(max.Int64)
			ttft.max = &v
	REDACTED
REDACTED

	return duration, ttft, nil
REDACTED

func (c *OpsMetricsCollector) queryErrorCounts(ctx context.Context, start, end time.Time) (
	errorTotal int64,
	businessLimited int64,
	errorSLA int64,
	upstreamExcl429529 int64,
	upstream429 int64,
	upstream529 int64,
	err error,
) {
	q := `
SELECT
  COALESCE(COUNT(*) FILTER (WHERE COALESCE(status_code, 0) >= 400), 0) AS error_total,
  COALESCE(COUNT(*) FILTER (WHERE COALESCE(status_code, 0) >= 400 AND is_business_limited), 0) AS business_limited,
  COALESCE(COUNT(*) FILTER (WHERE COALESCE(status_code, 0) >= 400 AND NOT is_business_limited), 0) AS error_sla,
  COALESCE(COUNT(*) FILTER (WHERE error_owner = 'provider' AND NOT is_business_limited AND COALESCE(upstream_status_code, status_code, 0) NOT IN (429, 529)), 0) AS upstream_excl,
  COALESCE(COUNT(*) FILTER (WHERE error_owner = 'provider' AND NOT is_business_limited AND COALESCE(upstream_status_code, status_code, 0) = 429), 0) AS upstream_429,
  COALESCE(COUNT(*) FILTER (WHERE error_owner = 'provider' AND NOT is_business_limited AND COALESCE(upstream_status_code, status_code, 0) = 529), 0) AS upstream_529
FROM ops_error_logs
WHERE created_at >= $1 AND created_at < $2
  AND is_count_tokens = FALSE`

	if err := c.db.QueryRowContext(ctx, q, start, end).Scan(
		&errorTotal,
		&businessLimited,
		&errorSLA,
		&upstreamExcl429529,
		&upstream429,
		&upstream529,
	); err != nil {
		return 0, 0, 0, 0, 0, 0, err
REDACTED
	return errorTotal, businessLimited, errorSLA, upstreamExcl429529, upstream429, upstream529, nil
REDACTED

func (c *OpsMetricsCollector) queryAccountSwitchCount(ctx context.Context, start, end time.Time) (int64, error) {
	q := `
SELECT
  COALESCE(SUM(CASE
    WHEN split_part(ev->>'kind', ':', 1) IN ('failover', 'retry_exhausted_failover', 'failover_on_400') THEN 1
    ELSE 0
  END), 0) AS switch_count
FROM ops_error_logs o
CROSS JOIN LATERAL jsonb_array_elements(
  COALESCE(NULLIF(o.upstream_errors, 'null'::jsonb), '[]'::jsonb)
) AS ev
WHERE o.created_at >= $1 AND o.created_at < $2
  AND o.is_count_tokens = FALSE`

	var count int64
	if err := c.db.QueryRowContext(ctx, q, start, end).Scan(&count); err != nil {
		return 0, err
REDACTED
	return count, nil
REDACTED

type opsCollectedSystemStats struct {
	cpuUsagePercent    *float64
	memoryUsedMB       *int64
	memoryTotalMB      *int64
	memoryUsagePercent *float64
REDACTED

func (c *OpsMetricsCollector) collectSystemStats(ctx context.Context) (*opsCollectedSystemStats, error) {
	out := &opsCollectedSystemStats{REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED

	sampleAt := time.Now().UTC()

	// Prefer cgroup (container) metrics when available.
	if cpuPct := c.tryCgroupCPUPercent(sampleAt); cpuPct != nil {
		out.cpuUsagePercent = cpuPct
REDACTED

	cgroupUsed, cgroupTotal, cgroupOK := readCgroupMemoryBytes()
	if cgroupOK {
		usedMB := int64(cgroupUsed / bytesPerMB)
		out.memoryUsedMB = &usedMB
		if cgroupTotal > 0 {
			totalMB := int64(cgroupTotal / bytesPerMB)
			out.memoryTotalMB = &totalMB
			pct := roundTo1DP(float64(cgroupUsed) / float64(cgroupTotal) * 100)
			out.memoryUsagePercent = &pct
	REDACTED
REDACTED

	// Fallback to host metrics if cgroup metrics are unavailable (or incomplete).
	if out.cpuUsagePercent == nil {
		if cpuPercents, err := cpu.PercentWithContext(ctx, 0, false); err == nil && len(cpuPercents) > 0 {
			v := roundTo1DP(cpuPercents[0])
			out.cpuUsagePercent = &v
	REDACTED
REDACTED

	// If total memory isn't available from cgroup (e.g. memory.max = "max"), fill total from host.
	if out.memoryUsedMB == nil || out.memoryTotalMB == nil || out.memoryUsagePercent == nil {
		if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil && vm != nil {
			if out.memoryUsedMB == nil {
				usedMB := int64(vm.Used / bytesPerMB)
				out.memoryUsedMB = &usedMB
		REDACTED
			if out.memoryTotalMB == nil {
				totalMB := int64(vm.Total / bytesPerMB)
				out.memoryTotalMB = &totalMB
		REDACTED
			if out.memoryUsagePercent == nil {
				if out.memoryUsedMB != nil && out.memoryTotalMB != nil && *out.memoryTotalMB > 0 {
					pct := roundTo1DP(float64(*out.memoryUsedMB) / float64(*out.memoryTotalMB) * 100)
					out.memoryUsagePercent = &pct
			REDACTED else {
					pct := roundTo1DP(vm.UsedPercent)
					out.memoryUsagePercent = &pct
			REDACTED
		REDACTED
	REDACTED
REDACTED

	return out, nil
REDACTED

func (c *OpsMetricsCollector) tryCgroupCPUPercent(now time.Time) *float64 {
	usageNanos, ok := readCgroupCPUUsageNanos()
	if !ok {
		return nil
REDACTED

	// Initialize baseline sample.
	if c.lastCgroupCPUSampleAt.IsZero() {
		c.lastCgroupCPUUsageNanos = usageNanos
		c.lastCgroupCPUSampleAt = now
		return nil
REDACTED

	elapsed := now.Sub(c.lastCgroupCPUSampleAt)
	if elapsed <= 0 {
		c.lastCgroupCPUUsageNanos = usageNanos
		c.lastCgroupCPUSampleAt = now
		return nil
REDACTED

	prev := c.lastCgroupCPUUsageNanos
	c.lastCgroupCPUUsageNanos = usageNanos
	c.lastCgroupCPUSampleAt = now

	if usageNanos < prev {
		// Counter reset (container restarted).
		return nil
REDACTED

	deltaUsageSec := float64(usageNanos-prev) / 1e9
	elapsedSec := elapsed.Seconds()
	if elapsedSec <= 0 {
		return nil
REDACTED

	cores := readCgroupCPULimitCores()
	if cores <= 0 {
		// Can't reliably normalize; skip and fall back to gopsutil.
		return nil
REDACTED

	pct := (deltaUsageSec / (elapsedSec * cores)) * 100
	if pct < 0 {
		pct = 0
REDACTED
	// Clamp to avoid noise/jitter showing impossible values.
	if pct > 100 {
		pct = 100
REDACTED
	v := roundTo1DP(pct)
	return &v
REDACTED

func readCgroupMemoryBytes() (usedBytes uint64, totalBytes uint64, ok bool) {
	// cgroup v2 (most common in modern containers)
	if used, ok1 := readUintFile("/sys/fs/cgroup/memory.current"); ok1 {
		usedBytes = used
		rawMax, err := os.ReadFile("/sys/fs/cgroup/memory.max")
		if err == nil {
			s := strings.TrimSpace(string(rawMax))
			if s != "" && s != "max" {
				if v, err := strconv.ParseUint(s, 10, 64); err == nil {
					totalBytes = v
			REDACTED
		REDACTED
	REDACTED
		return usedBytes, totalBytes, true
REDACTED

	// cgroup v1 fallback
	if used, ok1 := readUintFile("/sys/fs/cgroup/memory/memory.usage_in_bytes"); ok1 {
		usedBytes = used
		if limit, ok2 := readUintFile("/sys/fs/cgroup/memory/memory.limit_in_bytes"); ok2 {
			// Some environments report a very large number when unlimited.
			if limit > 0 && limit < (1<<60) {
				totalBytes = limit
		REDACTED
	REDACTED
		return usedBytes, totalBytes, true
REDACTED

	return 0, 0, false
REDACTED

func readCgroupCPUUsageNanos() (usageNanos uint64, ok bool) {
	// cgroup v2: cpu.stat has usage_usec
	if raw, err := os.ReadFile("/sys/fs/cgroup/cpu.stat"); err == nil {
		lines := strings.Split(string(raw), "\n")
		for _, line := range lines {
			fields := strings.Fields(line)
			if len(fields) != 2 {
				continue
		REDACTED
			if fields[0] != "usage_usec" {
				continue
		REDACTED
			v, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				continue
		REDACTED
			return v * 1000, true
	REDACTED
REDACTED

	// cgroup v1: cpuacct.usage is in nanoseconds
	if v, ok := readUintFile("/sys/fs/cgroup/cpuacct/cpuacct.usage"); ok {
		return v, true
REDACTED

	return 0, false
REDACTED

func readCgroupCPULimitCores() float64 {
	// cgroup v2: cpu.max => "<quota> <period>" or "max <period>"
	if raw, err := os.ReadFile("/sys/fs/cgroup/cpu.max"); err == nil {
		fields := strings.Fields(string(raw))
		if len(fields) >= 2 && fields[0] != "max" {
			quota, err1 := strconv.ParseFloat(fields[0], 64)
			period, err2 := strconv.ParseFloat(fields[1], 64)
			if err1 == nil && err2 == nil && quota > 0 && period > 0 {
				return quota / period
		REDACTED
	REDACTED
REDACTED

	// cgroup v1: cpu.cfs_quota_us / cpu.cfs_period_us
	quota, okQuota := readIntFile("/sys/fs/cgroup/cpu/cpu.cfs_quota_us")
	period, okPeriod := readIntFile("/sys/fs/cgroup/cpu/cpu.cfs_period_us")
	if okQuota && okPeriod && quota > 0 && period > 0 {
		return float64(quota) / float64(period)
REDACTED

	return 0
REDACTED

func readUintFile(path string) (uint64, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, false
REDACTED
	s := strings.TrimSpace(string(raw))
	if s == "" {
		return 0, false
REDACTED
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, false
REDACTED
	return v, true
REDACTED

func readIntFile(path string) (int64, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, false
REDACTED
	s := strings.TrimSpace(string(raw))
	if s == "" {
		return 0, false
REDACTED
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
REDACTED
	return v, true
REDACTED

func (c *OpsMetricsCollector) checkDB(ctx context.Context) bool {
	if c == nil || c.db == nil {
		return false
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED
	var one int
	if err := c.db.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil {
		return false
REDACTED
	return one == 1
REDACTED

func (c *OpsMetricsCollector) checkRedis(ctx context.Context) bool {
	if c == nil || c.redisClient == nil {
		return false
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED
	return c.redisClient.Ping(ctx).Err() == nil
REDACTED

func (c *OpsMetricsCollector) redisPoolStats() (total int, idle int, ok bool) {
	if c == nil || c.redisClient == nil {
		return 0, 0, false
REDACTED
	stats := c.redisClient.PoolStats()
	if stats == nil {
		return 0, 0, false
REDACTED
	return int(stats.TotalConns), int(stats.IdleConns), true
REDACTED

func (c *OpsMetricsCollector) dbPoolStats() (active int, idle int) {
	if c == nil || c.db == nil {
		return 0, 0
REDACTED
	stats := c.db.Stats()
	return stats.InUse, stats.Idle
REDACTED

var opsMetricsCollectorReleaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)

func (c *OpsMetricsCollector) tryAcquireLeaderLock(ctx context.Context) (func(), bool) {
	if c == nil || c.redisClient == nil {
		return nil, true
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED

	ok, err := c.redisClient.SetNX(ctx, opsMetricsCollectorLeaderLockKey, c.instanceID, opsMetricsCollectorLeaderLockTTL).Result()
	if err != nil {
		// Prefer fail-closed to avoid stampeding the database when Redis is flaky.
		// Fallback to a DB advisory lock when Redis is present but unavailable.
		release, ok := tryAcquireDBAdvisoryLock(ctx, c.db, opsMetricsCollectorAdvisoryLockID)
		if !ok {
			c.maybeLogSkip()
			return nil, false
	REDACTED
		return release, true
REDACTED
	if !ok {
		c.maybeLogSkip()
		return nil, false
REDACTED

	release := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = opsMetricsCollectorReleaseScript.Run(ctx, c.redisClient, []string{opsMetricsCollectorLeaderLockKeyREDACTED, c.instanceID).Result()
REDACTED
	return release, true
REDACTED

func (c *OpsMetricsCollector) maybeLogSkip() {
	c.skipLogMu.Lock()
	defer c.skipLogMu.Unlock()

	now := time.Now()
	if !c.skipLogAt.IsZero() && now.Sub(c.skipLogAt) < time.Minute {
		return
REDACTED
	c.skipLogAt = now
	log.Printf("[OpsMetricsCollector] leader lock held by another instance; skipping")
REDACTED

func floatToIntPtr(v sql.NullFloat64) *int {
	if !v.Valid {
		return nil
REDACTED
	n := int(math.Round(v.Float64))
	return &n
REDACTED

func roundTo1DP(v float64) float64 {
	return math.Round(v*10) / 10
REDACTED

func truncateString(s string, max int) string {
	if max <= 0 {
		return ""
REDACTED
	if len(s) <= max {
		return s
REDACTED
	cut := s[:max]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
REDACTED
	return cut
REDACTED

func boolPtr(v bool) *bool {
	out := v
	return &out
REDACTED

func intPtr(v int) *int {
	out := v
	return &out
REDACTED

func float64Ptr(v float64) *float64 {
	out := v
	return &out
REDACTED
