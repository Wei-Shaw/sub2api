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
	opsAggHourlyJobName = "ops_preaggregation_hourly"
	opsAggDailyJobName  = "ops_preaggregation_daily"

	opsAggHourlyInterval = 10 * time.Minute
	opsAggDailyInterval  = 1 * time.Hour

	// Keep in sync with ops retention target (vNext default 30d).
	opsAggBackfillWindow = 1 * time.Hour

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

	stopCh    chan struct{REDACTED
	startOnce sync.Once
	stopOnce  sync.Once

	hourlyMu sync.Mutex
	dailyMu  sync.Mutex

	skipLogMu sync.Mutex
	skipLogAt time.Time
REDACTED

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
REDACTED
REDACTED

func (s *OpsAggregationService) Start() {
	if s == nil {
		return
REDACTED
	s.startOnce.Do(func() {
		if s.stopCh == nil {
			s.stopCh = make(chan struct{REDACTED)
	REDACTED
		go s.hourlyLoop()
		go s.dailyLoop()
REDACTED)
REDACTED

func (s *OpsAggregationService) Stop() {
	if s == nil {
		return
REDACTED
	s.stopOnce.Do(func() {
		if s.stopCh != nil {
			close(s.stopCh)
	REDACTED
REDACTED)
REDACTED

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
	REDACTED
REDACTED
REDACTED

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
	REDACTED
REDACTED
REDACTED

func (s *OpsAggregationService) aggregateHourly() {
	if s == nil || s.opsRepo == nil {
		return
REDACTED
	if s.cfg != nil {
		if !s.cfg.Ops.Enabled {
			return
	REDACTED
		if !s.cfg.Ops.Aggregation.Enabled {
			return
	REDACTED
REDACTED

	ctx, cancel := context.WithTimeout(context.Background(), opsAggHourlyTimeout)
	defer cancel()

	if !s.isMonitoringEnabled(ctx) {
		return
REDACTED

	release, ok := s.tryAcquireLeaderLock(ctx, opsAggHourlyLeaderLockKey, opsAggHourlyLeaderLockTTL, "[OpsAggregation][hourly]")
	if !ok {
		return
REDACTED
	if release != nil {
		defer release()
REDACTED

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
	REDACTED else if ok {
			candidate := latest.Add(-opsAggHourlyOverlap)
			if candidate.After(start) {
				start = candidate
		REDACTED
	REDACTED
REDACTED

	start = utcFloorToHour(start)
	if !start.Before(end) {
		return
REDACTED

	var aggErr error
	for cursor := start; cursor.Before(end); cursor = cursor.Add(opsAggHourlyChunk) {
		chunkEnd := minTime(cursor.Add(opsAggHourlyChunk), end)
		if err := s.opsRepo.UpsertHourlyMetrics(ctx, cursor, chunkEnd); err != nil {
			aggErr = err
			logger.LegacyPrintf("service.ops_aggregation", "[OpsAggregation][hourly] upsert failed (%s..%s): %v", cursor.Format(time.RFC3339), chunkEnd.Format(time.RFC3339), err)
			break
	REDACTED
REDACTED

	finishedAt := time.Now().UTC()
	durationMs := finishedAt.Sub(startedAt).Milliseconds()
	dur := durationMs

	if aggErr != nil {
		msg := truncateString(aggErr.Error(), 2048)
		errAt := finishedAt
		hbCtx, hbCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer hbCancel()
		_ = s.opsRepo.UpsertJobHeartbeat(hbCtx, &OpsUpsertJobHeartbeatInput{
			JobName:        opsAggHourlyJobName,
			LastRunAt:      &runAt,
			LastErrorAt:    &errAt,
			LastError:      &msg,
			LastDurationMs: &dur,
	REDACTED)
		return
REDACTED

	successAt := finishedAt
	hbCtx, hbCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer hbCancel()
	result := truncateString(fmt.Sprintf("window=%s..%s", start.Format(time.RFC3339), end.Format(time.RFC3339)), 2048)
	_ = s.opsRepo.UpsertJobHeartbeat(hbCtx, &OpsUpsertJobHeartbeatInput{
		JobName:        opsAggHourlyJobName,
		LastRunAt:      &runAt,
		LastSuccessAt:  &successAt,
		LastDurationMs: &dur,
		LastResult:     &result,
REDACTED)
REDACTED

func (s *OpsAggregationService) aggregateDaily() {
	if s == nil || s.opsRepo == nil {
		return
REDACTED
	if s.cfg != nil {
		if !s.cfg.Ops.Enabled {
			return
	REDACTED
		if !s.cfg.Ops.Aggregation.Enabled {
			return
	REDACTED
REDACTED

	ctx, cancel := context.WithTimeout(context.Background(), opsAggDailyTimeout)
	defer cancel()

	if !s.isMonitoringEnabled(ctx) {
		return
REDACTED

	release, ok := s.tryAcquireLeaderLock(ctx, opsAggDailyLeaderLockKey, opsAggDailyLeaderLockTTL, "[OpsAggregation][daily]")
	if !ok {
		return
REDACTED
	if release != nil {
		defer release()
REDACTED

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
	REDACTED else if ok {
			candidate := latest.Add(-opsAggDailyOverlap)
			if candidate.After(start) {
				start = candidate
		REDACTED
	REDACTED
REDACTED

	start = utcFloorToDay(start)
	if !start.Before(end) {
		return
REDACTED

	var aggErr error
	for cursor := start; cursor.Before(end); cursor = cursor.Add(opsAggDailyChunk) {
		chunkEnd := minTime(cursor.Add(opsAggDailyChunk), end)
		if err := s.opsRepo.UpsertDailyMetrics(ctx, cursor, chunkEnd); err != nil {
			aggErr = err
			logger.LegacyPrintf("service.ops_aggregation", "[OpsAggregation][daily] upsert failed (%s..%s): %v", cursor.Format("2006-01-02"), chunkEnd.Format("2006-01-02"), err)
			break
	REDACTED
REDACTED

	finishedAt := time.Now().UTC()
	durationMs := finishedAt.Sub(startedAt).Milliseconds()
	dur := durationMs

	if aggErr != nil {
		msg := truncateString(aggErr.Error(), 2048)
		errAt := finishedAt
		hbCtx, hbCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer hbCancel()
		_ = s.opsRepo.UpsertJobHeartbeat(hbCtx, &OpsUpsertJobHeartbeatInput{
			JobName:        opsAggDailyJobName,
			LastRunAt:      &runAt,
			LastErrorAt:    &errAt,
			LastError:      &msg,
			LastDurationMs: &dur,
	REDACTED)
		return
REDACTED

	successAt := finishedAt
	hbCtx, hbCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer hbCancel()
	result := truncateString(fmt.Sprintf("window=%s..%s", start.Format(time.RFC3339), end.Format(time.RFC3339)), 2048)
	_ = s.opsRepo.UpsertJobHeartbeat(hbCtx, &OpsUpsertJobHeartbeatInput{
		JobName:        opsAggDailyJobName,
		LastRunAt:      &runAt,
		LastSuccessAt:  &successAt,
		LastDurationMs: &dur,
		LastResult:     &result,
REDACTED)
REDACTED

func (s *OpsAggregationService) isMonitoringEnabled(ctx context.Context) bool {
	if s == nil {
		return false
REDACTED
	if s.cfg != nil && !s.cfg.Ops.Enabled {
		return false
REDACTED
	if s.settingRepo == nil {
		return true
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED

	value, err := s.settingRepo.GetValue(ctx, SettingKeyOpsMonitoringEnabled)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return true
	REDACTED
		return true
REDACTED
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "false", "0", "off", "disabled":
		return false
	default:
		return true
REDACTED
REDACTED

var opsAggReleaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)

func (s *OpsAggregationService) tryAcquireLeaderLock(ctx context.Context, key string, ttl time.Duration, logPrefix string) (func(), bool) {
	if s == nil {
		return nil, false
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED

	// Prefer Redis leader lock when available (multi-instance), but avoid stampeding
	// the DB when Redis is flaky by falling back to a DB advisory lock.
	if s.redisClient != nil {
		ok, err := s.redisClient.SetNX(ctx, key, s.instanceID, ttl).Result()
		if err == nil {
			if !ok {
				s.maybeLogSkip(logPrefix)
				return nil, false
		REDACTED
			release := func() {
				ctx2, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				_, _ = opsAggReleaseScript.Run(ctx2, s.redisClient, []string{keyREDACTED, s.instanceID).Result()
		REDACTED
			return release, true
	REDACTED
		// Redis error: fall through to DB advisory lock.
REDACTED

	release, ok := tryAcquireDBAdvisoryLock(ctx, s.db, hashAdvisoryLockID(key))
	if !ok {
		s.maybeLogSkip(logPrefix)
		return nil, false
REDACTED
	return release, true
REDACTED

func (s *OpsAggregationService) maybeLogSkip(prefix string) {
	s.skipLogMu.Lock()
	defer s.skipLogMu.Unlock()

	now := time.Now()
	if !s.skipLogAt.IsZero() && now.Sub(s.skipLogAt) < time.Minute {
		return
REDACTED
	s.skipLogAt = now
	if prefix == "" {
		prefix = "[OpsAggregation]"
REDACTED
	logger.LegacyPrintf("service.ops_aggregation", "%s leader lock held by another instance; skipping", prefix)
REDACTED

func utcFloorToHour(t time.Time) time.Time {
	return t.UTC().Truncate(time.Hour)
REDACTED

func utcFloorToDay(t time.Time) time.Time {
	u := t.UTC()
	y, m, d := u.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
REDACTED

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
REDACTED
	return b
REDACTED
