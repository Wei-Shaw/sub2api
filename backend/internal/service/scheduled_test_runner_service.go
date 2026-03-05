package service

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/robfig/cron/v3"
)

const (
	scheduledTestLeaderLockKey     = "scheduled_test:runner:leader"
	scheduledTestLeaderLockTTL     = 2 * time.Minute
	scheduledTestDefaultMaxWorkers = 10
)

var scheduledTestReleaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)

// ScheduledTestRunnerService periodically scans due test plans and executes them.
type ScheduledTestRunnerService struct {
	planRepo       ScheduledTestPlanRepository
	scheduledSvc   *ScheduledTestService
	accountTestSvc *AccountTestService
	db             *sql.DB
	redisClient    *redis.Client
	cfg            *config.Config

	instanceID string
	cron       *cron.Cron
	startOnce  sync.Once
	stopOnce   sync.Once

	warnNoRedisOnce sync.Once
REDACTED

// NewScheduledTestRunnerService creates a new runner.
func NewScheduledTestRunnerService(
	planRepo ScheduledTestPlanRepository,
	scheduledSvc *ScheduledTestService,
	accountTestSvc *AccountTestService,
	db *sql.DB,
	redisClient *redis.Client,
	cfg *config.Config,
) *ScheduledTestRunnerService {
	return &ScheduledTestRunnerService{
		planRepo:       planRepo,
		scheduledSvc:   scheduledSvc,
		accountTestSvc: accountTestSvc,
		db:             db,
		redisClient:    redisClient,
		cfg:            cfg,
		instanceID:     uuid.NewString(),
REDACTED
REDACTED

// Start begins the cron ticker (every minute).
func (s *ScheduledTestRunnerService) Start() {
	if s == nil {
		return
REDACTED
	s.startOnce.Do(func() {
		loc := time.Local
		if s.cfg != nil {
			if parsed, err := time.LoadLocation(s.cfg.Timezone); err == nil && parsed != nil {
				loc = parsed
		REDACTED
	REDACTED

		c := cron.New(cron.WithParser(scheduledTestCronParser), cron.WithLocation(loc))
		_, err := c.AddFunc("* * * * *", func() { s.runScheduled() REDACTED)
		if err != nil {
			logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] not started (invalid schedule): %v", err)
			return
	REDACTED
		s.cron = c
		s.cron.Start()
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] started (tick=every minute)")
REDACTED)
REDACTED

// Stop gracefully shuts down the cron scheduler.
func (s *ScheduledTestRunnerService) Stop() {
	if s == nil {
		return
REDACTED
	s.stopOnce.Do(func() {
		if s.cron != nil {
			ctx := s.cron.Stop()
			select {
			case <-ctx.Done():
			case <-time.After(3 * time.Second):
				logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] cron stop timed out")
		REDACTED
	REDACTED
REDACTED)
REDACTED

func (s *ScheduledTestRunnerService) runScheduled() {
	// Delay 10s so execution lands at ~:10 of each minute instead of :00.
	time.Sleep(10 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	release, ok := s.tryAcquireLeaderLock(ctx)
	if !ok {
		return
REDACTED
	if release != nil {
		defer release()
REDACTED

	now := time.Now()
	plans, err := s.planRepo.ListDue(ctx, now)
	if err != nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] ListDue error: %v", err)
		return
REDACTED
	if len(plans) == 0 {
		return
REDACTED

	logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] found %d due plans", len(plans))

	maxWorkers := scheduledTestDefaultMaxWorkers
	sem := make(chan struct{REDACTED, maxWorkers)
	var wg sync.WaitGroup

	for _, plan := range plans {
		sem <- struct{REDACTED{REDACTED
		wg.Add(1)
		go func(p *ScheduledTestPlan) {
			defer wg.Done()
			defer func() { <-sem REDACTED()
			s.runOnePlan(ctx, p)
	REDACTED(plan)
REDACTED

	wg.Wait()
REDACTED

func (s *ScheduledTestRunnerService) runOnePlan(ctx context.Context, plan *ScheduledTestPlan) {
	outcome, err := s.accountTestSvc.RunTestBackground(ctx, plan.AccountID, plan.ModelID)
	if err != nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d RunTestBackground error: %v", plan.ID, err)
		return
REDACTED

	if err := s.scheduledSvc.SaveResult(ctx, plan.ID, plan.MaxResults, outcome); err != nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d SaveResult error: %v", plan.ID, err)
REDACTED

	// Compute next run
	nextRun, err := computeNextRun(plan.CronExpression, time.Now())
	if err != nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d computeNextRun error: %v", plan.ID, err)
		return
REDACTED

	if err := s.planRepo.UpdateAfterRun(ctx, plan.ID, time.Now(), nextRun); err != nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d UpdateAfterRun error: %v", plan.ID, err)
REDACTED
REDACTED

func (s *ScheduledTestRunnerService) tryAcquireLeaderLock(ctx context.Context) (func(), bool) {
	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		return nil, true
REDACTED

	key := scheduledTestLeaderLockKey
	ttl := scheduledTestLeaderLockTTL

	if s.redisClient != nil {
		ok, err := s.redisClient.SetNX(ctx, key, s.instanceID, ttl).Result()
		if err == nil {
			if !ok {
				return nil, false
		REDACTED
			return func() {
				_, _ = scheduledTestReleaseScript.Run(ctx, s.redisClient, []string{keyREDACTED, s.instanceID).Result()
		REDACTED, true
	REDACTED
		s.warnNoRedisOnce.Do(func() {
			logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] Redis SetNX failed; falling back to DB advisory lock: %v", err)
	REDACTED)
REDACTED else {
		s.warnNoRedisOnce.Do(func() {
			logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] Redis not configured; using DB advisory lock")
	REDACTED)
REDACTED

	release, ok := tryAcquireDBAdvisoryLock(ctx, s.db, hashAdvisoryLockID(key))
	if !ok {
		return nil, false
REDACTED
	return release, true
REDACTED
