package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	opsAggHourlyJobName   = "ops_preaggregation_hourly"
	opsAggDailyJobName    = "ops_preaggregation_daily"
	opsAggMinuteJobName   = "ops_preaggregation_minute"
	opsAggBackfillJobName = "ops_preaggregation_minute_backfill"

	opsAggHourlyInterval = 10 * time.Minute
	opsAggDailyInterval  = 1 * time.Hour
	opsAggMinuteInterval = 1 * time.Minute

	// Keep in sync with ops retention target (vNext default 30d).
	opsAggBackfillWindow = 1 * time.Hour

	// Recompute overlap to absorb late-arriving rows near boundaries.
	opsAggHourlyOverlap = 2 * time.Hour
	opsAggDailyOverlap  = 48 * time.Hour

	opsAggHourlyChunk = 24 * time.Hour
	opsAggDailyChunk  = 7 * 24 * time.Hour
	opsAggMinuteChunk = 1 * time.Hour

	// Delay around boundaries (e.g. 10:00..10:05) to avoid aggregating buckets
	// that may still receive late inserts.
	opsAggSafeDelay = 5 * time.Minute

	// Minute buckets are consumed by near-real-time trend charts, so the delay is
	// much shorter than the hourly one. Rows arriving after this point are picked
	// up by opsAggMinuteOverlap on a later pass, and the trend query stitches the
	// unaggregated tail from raw logs anyway.
	opsAggMinuteSafeDelay = 90 * time.Second
	opsAggMinuteOverlap   = 10 * time.Minute

	// Minute rollups are retained for MinuteMetricsRetentionDays (default 30);
	// backfill aims to cover the same window so trend queries stay off raw logs.
	opsAggMinuteBackfillChunk   = 6 * time.Hour
	opsAggMinuteBackfillMaxDays = 30

	opsAggMaxQueryTimeout = 5 * time.Second
	opsAggHourlyTimeout   = 5 * time.Minute
	opsAggDailyTimeout    = 2 * time.Minute
	opsAggMinuteTimeout   = 50 * time.Second
	opsAggBackfillTimeout = 30 * time.Minute

	opsAggHourlyLeaderLockKey   = "ops:aggregation:hourly:leader"
	opsAggDailyLeaderLockKey    = "ops:aggregation:daily:leader"
	opsAggMinuteLeaderLockKey   = "ops:aggregation:minute:leader"
	opsAggBackfillLeaderLockKey = "ops:aggregation:minute-backfill:leader"

	opsAggHourlyLeaderLockTTL   = 15 * time.Minute
	opsAggDailyLeaderLockTTL    = 10 * time.Minute
	opsAggMinuteLeaderLockTTL   = 2 * time.Minute
	opsAggBackfillLeaderLockTTL = opsAggBackfillTimeout + time.Minute
)

// OpsAggregationService periodically backfills ops_metrics_hourly / ops_metrics_daily
// for stable long-window dashboard queries.
//
// It is safe to run in multi-replica deployments when Redis is available (leader lock).
type OpsAggregationService struct {
	opsRepo     OpsRepository
	settingRepo SettingRepository
	cfg         *config.Config

	db          *sql.DB
	redisClient *redis.Client
	instanceID  string

	stopCh    chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once

	hourlyMu sync.Mutex
	dailyMu  sync.Mutex
	minuteMu sync.Mutex

	skipLogMu sync.Mutex
	skipLogAt time.Time
}

func NewOpsAggregationService(
	opsRepo OpsRepository,
	settingRepo SettingRepository,
	db *sql.DB,
	redisClient *redis.Client,
	cfg *config.Config,
) *OpsAggregationService {
	return &OpsAggregationService{
		opsRepo:     opsRepo,
		settingRepo: settingRepo,
		cfg:         cfg,
		db:          db,
		redisClient: redisClient,
		instanceID:  uuid.NewString(),
	}
}

func (s *OpsAggregationService) Start() {
	if s == nil {
		return
	}
	s.startOnce.Do(func() {
		if s.stopCh == nil {
			s.stopCh = make(chan struct{})
		}
		go s.hourlyLoop()
		go s.dailyLoop()
		go s.minuteLoop()
		go s.backfillMinuteHistory()
	})
}

func (s *OpsAggregationService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		if s.stopCh != nil {
			close(s.stopCh)
		}
	})
}

func (s *OpsAggregationService) hourlyLoop() {
	// First run immediately.
	s.aggregateHourly()

	ticker := time.NewTicker(opsAggHourlyInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.aggregateHourly()
		case <-s.stopCh:
			return
		}
	}
}

func (s *OpsAggregationService) dailyLoop() {
	// First run immediately.
	s.aggregateDaily()

	ticker := time.NewTicker(opsAggDailyInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.aggregateDaily()
		case <-s.stopCh:
			return
		}
	}
}

func (s *OpsAggregationService) minuteLoop() {
	// First run immediately.
	s.aggregateMinute()

	ticker := time.NewTicker(opsAggMinuteInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.aggregateMinute()
		case <-s.stopCh:
			return
		}
	}
}

// aggregateMinute rolls up recent minute buckets for the trend charts.
// It recomputes a small overlapping window so late-arriving rows (a streaming
// request can land seconds after its bucket closed) are folded in on a later pass.
func (s *OpsAggregationService) aggregateMinute() {
	if s == nil || s.opsRepo == nil {
		return
	}
	if s.cfg != nil {
		if !s.cfg.Ops.Enabled {
			return
		}
		if !s.cfg.Ops.Aggregation.Enabled {
			return
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), opsAggMinuteTimeout)
	defer cancel()

	if !s.isMonitoringEnabled(ctx) {
		return
	}

	release, ok := s.tryAcquireLeaderLock(ctx, opsAggMinuteLeaderLockKey, opsAggMinuteLeaderLockTTL, "[OpsAggregation][minute]")
	if !ok {
		return
	}
	if release != nil {
		defer release()
	}

	s.minuteMu.Lock()
	defer s.minuteMu.Unlock()

	startedAt := time.Now().UTC()
	runAt := startedAt

	end := utcFloorToMinute(time.Now().UTC().Add(-opsAggMinuteSafeDelay))
	start := end.Add(-opsAggMinuteOverlap)

	// Resume from the latest aggregated bucket, re-covering the overlap window.
	{
		ctxMax, cancelMax := context.WithTimeout(context.Background(), opsAggMaxQueryTimeout)
		latest, ok, err := s.opsRepo.GetLatestMinuteBucketStart(ctxMax)
		cancelMax()
		if err != nil {
			logger.LegacyPrintf("service.ops_aggregation", "[OpsAggregation][minute] failed to read latest bucket: %v", err)
		} else if ok {
			candidate := latest.Add(-opsAggMinuteOverlap)
			if candidate.After(start) {
				start = candidate
			}
		}
	}

	start = utcFloorToMinute(start)
	if !start.Before(end) {
		return
	}

	var aggErr error
	for cursor := start; cursor.Before(end); cursor = cursor.Add(opsAggMinuteChunk) {
		chunkEnd := minTime(cursor.Add(opsAggMinuteChunk), end)
		if err := s.opsRepo.UpsertMinuteMetrics(ctx, cursor, chunkEnd); err != nil {
			aggErr = err
			logger.LegacyPrintf("service.ops_aggregation", "[OpsAggregation][minute] upsert failed (%s..%s): %v", cursor.Format(time.RFC3339), chunkEnd.Format(time.RFC3339), err)
			break
		}
	}

	s.reportAggregationHeartbeat(opsAggMinuteJobName, runAt, startedAt, start, end, aggErr)
}

// backfillMinuteHistory fills the minute table back to the retention window on startup,
// so trend queries over historical windows do not fall back to scanning raw logs.
// Runs once per process under its own leader lock; chunked so no single statement is long.
func (s *OpsAggregationService) backfillMinuteHistory() {
	if s == nil || s.opsRepo == nil {
		return
	}
	if s.cfg != nil {
		if !s.cfg.Ops.Enabled || !s.cfg.Ops.Aggregation.Enabled {
			return
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), opsAggBackfillTimeout)
	defer cancel()

	if !s.isMonitoringEnabled(ctx) {
		return
	}

	release, ok := s.tryAcquireLeaderLock(ctx, opsAggBackfillLeaderLockKey, opsAggBackfillLeaderLockTTL, "[OpsAggregation][minute-backfill]")
	if !ok {
		return
	}
	if release != nil {
		defer release()
	}

	retentionDays := opsAggMinuteBackfillMaxDays
	if s.cfg != nil && s.cfg.Ops.Cleanup.MinuteMetricsRetentionDays > 0 {
		retentionDays = s.cfg.Ops.Cleanup.MinuteMetricsRetentionDays
	}

	target := utcFloorToMinute(time.Now().UTC().AddDate(0, 0, -retentionDays))
	safeEnd := utcFloorToMinute(time.Now().UTC().Add(-opsAggMinuteSafeDelay))

	ranges, err := s.minuteBackfillRanges(ctx, target, safeEnd)
	if err != nil {
		logger.LegacyPrintf("service.ops_aggregation", "[OpsAggregation][minute-backfill] failed to inspect coverage: %v", err)
		return
	}
	if len(ranges) == 0 {
		return
	}

	startedAt := time.Now().UTC()
	runAt := startedAt
	windowStart, windowEnd := ranges[0][0], ranges[len(ranges)-1][1]
	logger.LegacyPrintf("service.ops_aggregation", "[OpsAggregation][minute-backfill] started (%d range(s), %s..%s)", len(ranges), windowStart.Format(time.RFC3339), windowEnd.Format(time.RFC3339))

	var aggErr error
backfill:
	for _, rng := range ranges {
		for cursor := rng[0]; cursor.Before(rng[1]); cursor = cursor.Add(opsAggMinuteBackfillChunk) {
			if ctx.Err() != nil {
				// Timed out: whatever was committed stays; the next process start resumes the gap.
				logger.LegacyPrintf("service.ops_aggregation", "[OpsAggregation][minute-backfill] interrupted at %s, remaining window resumes on next start", cursor.Format(time.RFC3339))
				return
			}
			chunkEnd := minTime(cursor.Add(opsAggMinuteBackfillChunk), rng[1])
			if err := s.opsRepo.UpsertMinuteMetrics(ctx, cursor, chunkEnd); err != nil {
				aggErr = err
				logger.LegacyPrintf("service.ops_aggregation", "[OpsAggregation][minute-backfill] upsert failed (%s..%s): %v", cursor.Format(time.RFC3339), chunkEnd.Format(time.RFC3339), err)
				break backfill
			}
		}
	}

	if aggErr == nil {
		logger.LegacyPrintf("service.ops_aggregation", "[OpsAggregation][minute-backfill] done (%s..%s duration=%s)", windowStart.Format(time.RFC3339), windowEnd.Format(time.RFC3339), time.Since(startedAt))
	}
	s.reportAggregationHeartbeat(opsAggBackfillJobName, runAt, startedAt, windowStart, windowEnd, aggErr)
}

// minuteBackfillRanges 返回分钟表在 [target, safeEnd) 内还缺的时间段。
//
// 分两头补：
//   - 低端：保留窗口起点到已覆盖的最早桶——首次上线或延长 retention 后的缺口。
//   - 高端：已覆盖的最新桶到当前安全线——停机留下的缺口。增量循环每轮只回看
//     opsAggMinuteOverlap，停机时间超过这个跨度就再也追不回来，会在 trend 上留下永久空洞。
//
// 中段空洞（回填自身被打断留下的）不在这里处理：分块提交后下次启动会从新的
// earliest/latest 重新算缺口，正常情况下补得回来。
func (s *OpsAggregationService) minuteBackfillRanges(ctx context.Context, target, safeEnd time.Time) ([][2]time.Time, error) {
	if !target.Before(safeEnd) {
		return nil, nil
	}

	earliest, hasEarliest, err := s.opsRepo.GetEarliestMinuteBucketStart(ctx)
	if err != nil {
		return nil, err
	}
	if !hasEarliest {
		// 表是空的：一次覆盖整个保留窗口。
		return [][2]time.Time{{target, safeEnd}}, nil
	}

	var ranges [][2]time.Time
	if target.Before(earliest) {
		ranges = append(ranges, [2]time.Time{target, minTime(earliest, safeEnd)})
	}

	latest, hasLatest, err := s.opsRepo.GetLatestMinuteBucketStart(ctx)
	if err != nil {
		return nil, err
	}
	if hasLatest {
		// 只补增量循环够不到的部分，避免每次启动都重跑最近十分钟。
		gapStart := latest.Add(time.Minute)
		if gapStart.Before(safeEnd.Add(-opsAggMinuteOverlap)) {
			ranges = append(ranges, [2]time.Time{gapStart, safeEnd})
		}
	}
	return ranges, nil
}

// reportAggregationHeartbeat records the outcome of one aggregation pass.
// Extracted so the minute/backfill jobs don't duplicate the hourly/daily boilerplate.
func (s *OpsAggregationService) reportAggregationHeartbeat(jobName string, runAt, startedAt time.Time, windowStart, windowEnd time.Time, aggErr error) {
	finishedAt := time.Now().UTC()
	dur := finishedAt.Sub(startedAt).Milliseconds()

	hbCtx, hbCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer hbCancel()

	if aggErr != nil {
		msg := truncateString(aggErr.Error(), 2048)
		errAt := finishedAt
		_ = s.opsRepo.UpsertJobHeartbeat(hbCtx, &OpsUpsertJobHeartbeatInput{
			JobName:        jobName,
			LastRunAt:      &runAt,
			LastErrorAt:    &errAt,
			LastError:      &msg,
			LastDurationMs: &dur,
		})
		return
	}

	successAt := finishedAt
	result := truncateString(fmt.Sprintf("window=%s..%s", windowStart.Format(time.RFC3339), windowEnd.Format(time.RFC3339)), 2048)
	_ = s.opsRepo.UpsertJobHeartbeat(hbCtx, &OpsUpsertJobHeartbeatInput{
		JobName:        jobName,
		LastRunAt:      &runAt,
		LastSuccessAt:  &successAt,
		LastDurationMs: &dur,
		LastResult:     &result,
	})
}

func (s *OpsAggregationService) aggregateHourly() {
	if s == nil || s.opsRepo == nil {
		return
	}
	if s.cfg != nil {
		if !s.cfg.Ops.Enabled {
			return
		}
		if !s.cfg.Ops.Aggregation.Enabled {
			return
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), opsAggHourlyTimeout)
	defer cancel()

	if !s.isMonitoringEnabled(ctx) {
		return
	}

	release, ok := s.tryAcquireLeaderLock(ctx, opsAggHourlyLeaderLockKey, opsAggHourlyLeaderLockTTL, "[OpsAggregation][hourly]")
	if !ok {
		return
	}
	if release != nil {
		defer release()
	}

	s.hourlyMu.Lock()
	defer s.hourlyMu.Unlock()

	startedAt := time.Now().UTC()
	runAt := startedAt

	// Aggregate stable full hours only.
	end := utcFloorToHour(time.Now().UTC().Add(-opsAggSafeDelay))
	start := end.Add(-opsAggBackfillWindow)

	// Resume from the latest bucket with overlap.
	{
		ctxMax, cancelMax := context.WithTimeout(context.Background(), opsAggMaxQueryTimeout)
		latest, ok, err := s.opsRepo.GetLatestHourlyBucketStart(ctxMax)
		cancelMax()
		if err != nil {
			logger.LegacyPrintf("service.ops_aggregation", "[OpsAggregation][hourly] failed to read latest bucket: %v", err)
		} else if ok {
			candidate := latest.Add(-opsAggHourlyOverlap)
			if candidate.After(start) {
				start = candidate
			}
		}
	}

	start = utcFloorToHour(start)
	if !start.Before(end) {
		return
	}

	var aggErr error
	for cursor := start; cursor.Before(end); cursor = cursor.Add(opsAggHourlyChunk) {
		chunkEnd := minTime(cursor.Add(opsAggHourlyChunk), end)
		if err := s.opsRepo.UpsertHourlyMetrics(ctx, cursor, chunkEnd); err != nil {
			aggErr = err
			logger.LegacyPrintf("service.ops_aggregation", "[OpsAggregation][hourly] upsert failed (%s..%s): %v", cursor.Format(time.RFC3339), chunkEnd.Format(time.RFC3339), err)
			break
		}
	}

	s.reportAggregationHeartbeat(opsAggHourlyJobName, runAt, startedAt, start, end, aggErr)
}

func (s *OpsAggregationService) aggregateDaily() {
	if s == nil || s.opsRepo == nil {
		return
	}
	if s.cfg != nil {
		if !s.cfg.Ops.Enabled {
			return
		}
		if !s.cfg.Ops.Aggregation.Enabled {
			return
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), opsAggDailyTimeout)
	defer cancel()

	if !s.isMonitoringEnabled(ctx) {
		return
	}

	release, ok := s.tryAcquireLeaderLock(ctx, opsAggDailyLeaderLockKey, opsAggDailyLeaderLockTTL, "[OpsAggregation][daily]")
	if !ok {
		return
	}
	if release != nil {
		defer release()
	}

	s.dailyMu.Lock()
	defer s.dailyMu.Unlock()

	startedAt := time.Now().UTC()
	runAt := startedAt

	end := utcFloorToDay(time.Now().UTC())
	start := end.Add(-opsAggBackfillWindow)

	{
		ctxMax, cancelMax := context.WithTimeout(context.Background(), opsAggMaxQueryTimeout)
		latest, ok, err := s.opsRepo.GetLatestDailyBucketDate(ctxMax)
		cancelMax()
		if err != nil {
			logger.LegacyPrintf("service.ops_aggregation", "[OpsAggregation][daily] failed to read latest bucket: %v", err)
		} else if ok {
			candidate := latest.Add(-opsAggDailyOverlap)
			if candidate.After(start) {
				start = candidate
			}
		}
	}

	start = utcFloorToDay(start)
	if !start.Before(end) {
		return
	}

	var aggErr error
	for cursor := start; cursor.Before(end); cursor = cursor.Add(opsAggDailyChunk) {
		chunkEnd := minTime(cursor.Add(opsAggDailyChunk), end)
		if err := s.opsRepo.UpsertDailyMetrics(ctx, cursor, chunkEnd); err != nil {
			aggErr = err
			logger.LegacyPrintf("service.ops_aggregation", "[OpsAggregation][daily] upsert failed (%s..%s): %v", cursor.Format("2006-01-02"), chunkEnd.Format("2006-01-02"), err)
			break
		}
	}

	s.reportAggregationHeartbeat(opsAggDailyJobName, runAt, startedAt, start, end, aggErr)
}

func (s *OpsAggregationService) isMonitoringEnabled(ctx context.Context) bool {
	if s == nil {
		return false
	}
	if s.cfg != nil && !s.cfg.Ops.Enabled {
		return false
	}
	if s.settingRepo == nil {
		return true
	}
	if ctx == nil {
		ctx = context.Background()
	}

	value, err := s.settingRepo.GetValue(ctx, SettingKeyOpsMonitoringEnabled)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return true
		}
		return true
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "false", "0", "off", "disabled":
		return false
	default:
		return true
	}
}

var opsAggReleaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)

func (s *OpsAggregationService) tryAcquireLeaderLock(ctx context.Context, key string, ttl time.Duration, logPrefix string) (func(), bool) {
	if s == nil {
		return nil, false
	}
	if ctx == nil {
		ctx = context.Background()
	}

	// Prefer Redis leader lock when available (multi-instance), but avoid stampeding
	// the DB when Redis is flaky by falling back to a DB advisory lock.
	if s.redisClient != nil {
		ok, err := s.redisClient.SetNX(ctx, key, s.instanceID, ttl).Result()
		if err == nil {
			if !ok {
				s.maybeLogSkip(logPrefix)
				return nil, false
			}
			release := func() {
				ctx2, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				_, _ = opsAggReleaseScript.Run(ctx2, s.redisClient, []string{key}, s.instanceID).Result()
			}
			return release, true
		}
		// Redis error: fall through to DB advisory lock.
	}

	release, ok := tryAcquireDBAdvisoryLock(ctx, s.db, hashAdvisoryLockID(key))
	if !ok {
		s.maybeLogSkip(logPrefix)
		return nil, false
	}
	return release, true
}

func (s *OpsAggregationService) maybeLogSkip(prefix string) {
	s.skipLogMu.Lock()
	defer s.skipLogMu.Unlock()

	now := time.Now()
	if !s.skipLogAt.IsZero() && now.Sub(s.skipLogAt) < time.Minute {
		return
	}
	s.skipLogAt = now
	if prefix == "" {
		prefix = "[OpsAggregation]"
	}
	logger.LegacyPrintf("service.ops_aggregation", "%s leader lock held by another instance; skipping", prefix)
}

func utcFloorToMinute(t time.Time) time.Time {
	return t.UTC().Truncate(time.Minute)
}

func utcFloorToHour(t time.Time) time.Time {
	return t.UTC().Truncate(time.Hour)
}

func utcFloorToDay(t time.Time) time.Time {
	u := t.UTC()
	y, m, d := u.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
