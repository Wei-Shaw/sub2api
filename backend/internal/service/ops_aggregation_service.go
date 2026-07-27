package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	opsAggHourlyJobName = "ops_preaggregation_hourly"
	opsAggDailyJobName  = "ops_preaggregation_daily"

	opsAggHourlyInterval = 10 * time.Minute
	opsAggDailyInterval  = 1 * time.Hour

	// Normal runs only refresh the recent edge. A fresh metric-definition version
	// gets a bounded bootstrap window large enough to cover one complete UTC day
	// regardless of the deployment time.
	opsAggBackfillWindow        = 1 * time.Hour
	opsAggInitialBackfillWindow = 48 * time.Hour

	// Recompute overlap to absorb late-arriving rows near boundaries.
	opsAggHourlyOverlap = 2 * time.Hour
	opsAggDailyOverlap  = 48 * time.Hour

	opsAggHourlyChunk = 24 * time.Hour
	opsAggDailyChunk  = 7 * 24 * time.Hour

	// Delay around boundaries (e.g. 10:00..10:05) to avoid aggregating buckets
	// that may still receive late inserts.
	opsAggSafeDelay = 5 * time.Minute

	opsAggMaxQueryTimeout = 5 * time.Second
	opsAggHourlyTimeout   = 5 * time.Minute
	opsAggDailyTimeout    = 2 * time.Minute

	opsAggHourlyLeaderLockKey = "ops:aggregation:hourly:leader"
	opsAggDailyLeaderLockKey  = "ops:aggregation:daily:leader"

	opsAggHourlyLeaderLockTTL = 15 * time.Minute
	opsAggDailyLeaderLockTTL  = 10 * time.Minute
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
	wg        sync.WaitGroup
	runCtx    context.Context
	runCancel context.CancelFunc

	hourlyMu sync.Mutex
	dailyMu  sync.Mutex

	hourlyDefinitionReady atomic.Bool
	dailyDefinitionReady  atomic.Bool

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
	runCtx, runCancel := context.WithCancel(context.Background())
	return &OpsAggregationService{
		opsRepo:     opsRepo,
		settingRepo: settingRepo,
		cfg:         cfg,
		db:          db,
		redisClient: redisClient,
		instanceID:  uuid.NewString(),
		runCtx:      runCtx,
		runCancel:   runCancel,
	}
}

func (s *OpsAggregationService) Start() {
	if s == nil {
		return
	}
	s.startOnce.Do(func() {
		if s.runCtx == nil {
			s.runCtx, s.runCancel = context.WithCancel(context.Background())
		}
		if s.stopCh == nil {
			s.stopCh = make(chan struct{})
		}
		s.wg.Add(2)
		go s.hourlyLoop()
		go s.dailyLoop()
	})
}

func (s *OpsAggregationService) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), defaultBackgroundStopTimeout)
	defer cancel()
	if err := s.StopContext(ctx); err != nil {
		logger.LegacyPrintf("service.ops_aggregation", "[OpsAggregation] shutdown incomplete: %v", err)
	}
}

func (s *OpsAggregationService) StopContext(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.stopOnce.Do(func() {
		if s.runCancel != nil {
			s.runCancel()
		}
		if s.stopCh != nil {
			close(s.stopCh)
		}
	})
	return waitForBackgroundWorkers(ctx, &s.wg)
}

func (s *OpsAggregationService) runtimeContext() context.Context {
	if s != nil && s.runCtx != nil {
		return s.runCtx
	}
	return context.Background()
}

func (s *OpsAggregationService) hourlyLoop() {
	defer s.wg.Done()
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
	defer s.wg.Done()
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

	ctx, cancel := context.WithTimeout(s.runtimeContext(), opsAggHourlyTimeout)
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
	start := end.Add(-opsAggInitialBackfillWindow)
	bootstrapDefinition := !s.hourlyDefinitionReady.Load()

	// Resume from the latest bucket with overlap.
	if !bootstrapDefinition {
		ctxMax, cancelMax := context.WithTimeout(ctx, opsAggMaxQueryTimeout)
		latest, ok, err := s.opsRepo.GetLatestHourlyBucketStart(ctxMax)
		cancelMax()
		if err != nil {
			logger.LegacyPrintf("service.ops_aggregation", "[OpsAggregation][hourly] failed to read latest bucket: %v", err)
		} else if ok {
			start = aggregationWindowStart(end, latest, opsAggBackfillWindow, opsAggHourlyOverlap, opsAggInitialBackfillWindow)
		}
	}

	start = utcFloorToHour(start)
	if !start.Before(end) {
		return
	}

	var aggErr error
	if bootstrapDefinition {
		aggErr = s.opsRepo.InvalidateHourlyMetricsVersion(ctx, start, end, OpsMetricDefinitionVersion)
		if aggErr != nil {
			logger.LegacyPrintf("service.ops_aggregation", "[OpsAggregation][hourly] failed to invalidate stale metric definition: %v", aggErr)
		}
	}
	if aggErr == nil {
		for cursor := start; cursor.Before(end); cursor = cursor.Add(opsAggHourlyChunk) {
			chunkEnd := minTime(cursor.Add(opsAggHourlyChunk), end)
			if err := s.opsRepo.UpsertHourlyMetrics(ctx, cursor, chunkEnd); err != nil {
				aggErr = err
				logger.LegacyPrintf("service.ops_aggregation", "[OpsAggregation][hourly] upsert failed (%s..%s): %v", cursor.Format(time.RFC3339), chunkEnd.Format(time.RFC3339), err)
				break
			}
		}
	}

	finishedAt := time.Now().UTC()
	durationMs := finishedAt.Sub(startedAt).Milliseconds()
	dur := durationMs

	if aggErr != nil {
		msg := truncateString(aggErr.Error(), 2048)
		errAt := finishedAt
		hbCtx, hbCancel := context.WithTimeout(ctx, 2*time.Second)
		defer hbCancel()
		_ = s.opsRepo.UpsertJobHeartbeat(hbCtx, &OpsUpsertJobHeartbeatInput{
			JobName:        opsAggHourlyJobName,
			LastRunAt:      &runAt,
			LastErrorAt:    &errAt,
			LastError:      &msg,
			LastDurationMs: &dur,
		})
		return
	}
	if bootstrapDefinition {
		s.hourlyDefinitionReady.Store(true)
	}

	successAt := finishedAt
	hbCtx, hbCancel := context.WithTimeout(ctx, 2*time.Second)
	defer hbCancel()
	result := truncateString(fmt.Sprintf("window=%s..%s", start.Format(time.RFC3339), end.Format(time.RFC3339)), 2048)
	_ = s.opsRepo.UpsertJobHeartbeat(hbCtx, &OpsUpsertJobHeartbeatInput{
		JobName:        opsAggHourlyJobName,
		LastRunAt:      &runAt,
		LastSuccessAt:  &successAt,
		LastDurationMs: &dur,
		LastResult:     &result,
	})
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

	ctx, cancel := context.WithTimeout(s.runtimeContext(), opsAggDailyTimeout)
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
	if !s.hourlyDefinitionReady.Load() {
		return
	}

	startedAt := time.Now().UTC()
	runAt := startedAt

	end := utcFloorToDay(time.Now().UTC())

	// Never roll up a UTC day until the V2 hourly series covers its final hour.
	// The hourly and daily workers start concurrently, so this also prevents a
	// first-start race from permanently writing a partial daily bucket.
	{
		ctxMax, cancelMax := context.WithTimeout(ctx, opsAggMaxQueryTimeout)
		latestHourly, ok, err := s.opsRepo.GetLatestHourlyBucketStart(ctxMax)
		cancelMax()
		if err != nil {
			logger.LegacyPrintf("service.ops_aggregation", "[OpsAggregation][daily] failed to read hourly coverage: %v", err)
			return
		}
		if !ok {
			return
		}
		hourlyCoveredThrough := latestHourly.Add(time.Hour)
		if hourlyCoveredThrough.Before(end) {
			end = utcFloorToDay(hourlyCoveredThrough)
		}
	}

	start := end.Add(-opsAggInitialBackfillWindow)
	bootstrapDefinition := !s.dailyDefinitionReady.Load()

	if !bootstrapDefinition {
		ctxMax, cancelMax := context.WithTimeout(ctx, opsAggMaxQueryTimeout)
		latest, ok, err := s.opsRepo.GetLatestDailyBucketDate(ctxMax)
		cancelMax()
		if err != nil {
			logger.LegacyPrintf("service.ops_aggregation", "[OpsAggregation][daily] failed to read latest bucket: %v", err)
		} else if ok {
			start = aggregationWindowStart(end, latest, opsAggBackfillWindow, opsAggDailyOverlap, opsAggInitialBackfillWindow)
		}
	}

	start = utcFloorToDay(start)
	if !start.Before(end) {
		return
	}

	var aggErr error
	if bootstrapDefinition {
		aggErr = s.opsRepo.InvalidateDailyMetricsVersion(ctx, start, end, OpsMetricDefinitionVersion)
		if aggErr != nil {
			logger.LegacyPrintf("service.ops_aggregation", "[OpsAggregation][daily] failed to invalidate stale metric definition: %v", aggErr)
		}
	}
	if aggErr == nil {
		for cursor := start; cursor.Before(end); cursor = cursor.Add(opsAggDailyChunk) {
			chunkEnd := minTime(cursor.Add(opsAggDailyChunk), end)
			if err := s.opsRepo.UpsertDailyMetrics(ctx, cursor, chunkEnd); err != nil {
				aggErr = err
				logger.LegacyPrintf("service.ops_aggregation", "[OpsAggregation][daily] upsert failed (%s..%s): %v", cursor.Format("2006-01-02"), chunkEnd.Format("2006-01-02"), err)
				break
			}
		}
	}

	finishedAt := time.Now().UTC()
	durationMs := finishedAt.Sub(startedAt).Milliseconds()
	dur := durationMs

	if aggErr != nil {
		msg := truncateString(aggErr.Error(), 2048)
		errAt := finishedAt
		hbCtx, hbCancel := context.WithTimeout(ctx, 2*time.Second)
		defer hbCancel()
		_ = s.opsRepo.UpsertJobHeartbeat(hbCtx, &OpsUpsertJobHeartbeatInput{
			JobName:        opsAggDailyJobName,
			LastRunAt:      &runAt,
			LastErrorAt:    &errAt,
			LastError:      &msg,
			LastDurationMs: &dur,
		})
		return
	}
	if bootstrapDefinition {
		s.dailyDefinitionReady.Store(true)
	}

	successAt := finishedAt
	hbCtx, hbCancel := context.WithTimeout(ctx, 2*time.Second)
	defer hbCancel()
	result := truncateString(fmt.Sprintf("window=%s..%s", start.Format(time.RFC3339), end.Format(time.RFC3339)), 2048)
	_ = s.opsRepo.UpsertJobHeartbeat(hbCtx, &OpsUpsertJobHeartbeatInput{
		JobName:        opsAggDailyJobName,
		LastRunAt:      &runAt,
		LastSuccessAt:  &successAt,
		LastDurationMs: &dur,
		LastResult:     &result,
	})
}

func aggregationWindowStart(end, latest time.Time, backfill, overlap, maxLookback time.Duration) time.Time {
	start := end.Add(-backfill)
	candidate := latest.Add(-overlap)
	if candidate.Before(start) {
		start = candidate
	}
	lowerBound := end.Add(-maxLookback)
	if start.Before(lowerBound) {
		return lowerBound
	}
	return start
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
	if s.db == nil {
		return func() {}, true
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
