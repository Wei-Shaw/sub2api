package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/robfig/cron/v3"
)

const scheduledTestDefaultMaxWorkers = 10

type scheduledAccountTester interface {
	RunTestBackground(ctx context.Context, accountID int64, modelID string) (*ScheduledTestResult, error)
}

// ScheduledTestRunnerService periodically scans due test plans and executes them.
type ScheduledTestRunnerService struct {
	planRepo       ScheduledTestPlanRepository
	scheduledSvc   *ScheduledTestService
	accountTestSvc scheduledAccountTester
	rateLimitSvc   *RateLimitService
	cfg            *config.Config

	cron      *cron.Cron
	startOnce sync.Once
	stopOnce  sync.Once
	running   sync.Map
}

// NewScheduledTestRunnerService creates a new runner.
func NewScheduledTestRunnerService(
	planRepo ScheduledTestPlanRepository,
	scheduledSvc *ScheduledTestService,
	accountTestSvc *AccountTestService,
	rateLimitSvc *RateLimitService,
	cfg *config.Config,
) *ScheduledTestRunnerService {
	return &ScheduledTestRunnerService{
		planRepo:       planRepo,
		scheduledSvc:   scheduledSvc,
		accountTestSvc: accountTestSvc,
		rateLimitSvc:   rateLimitSvc,
		cfg:            cfg,
	}
}

// Start begins the cron ticker (every minute).
func (s *ScheduledTestRunnerService) Start() {
	if s == nil {
		return
	}
	s.startOnce.Do(func() {
		loc := time.Local
		if s.cfg != nil {
			if parsed, err := time.LoadLocation(s.cfg.Timezone); err == nil && parsed != nil {
				loc = parsed
			}
		}

		c := cron.New(cron.WithParser(scheduledTestCronParser), cron.WithLocation(loc))
		_, err := c.AddFunc("* * * * *", func() { s.runScheduled() })
		if err != nil {
			logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] not started (invalid schedule): %v", err)
			return
		}
		s.cron = c
		s.cron.Start()
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] started (tick=every minute)")
	})
}

// Stop gracefully shuts down the cron scheduler.
func (s *ScheduledTestRunnerService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		if s.cron != nil {
			ctx := s.cron.Stop()
			select {
			case <-ctx.Done():
			case <-time.After(3 * time.Second):
				logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] cron stop timed out")
			}
		}
	})
}

func (s *ScheduledTestRunnerService) runScheduled() {
	// Delay 10s so execution lands at ~:10 of each minute instead of :00.
	time.Sleep(10 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	now := time.Now()
	plans, err := s.planRepo.ListDue(ctx, now)
	if err != nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] ListDue error: %v", err)
		return
	}
	if len(plans) == 0 {
		return
	}

	logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] found %d due plans", len(plans))

	sem := make(chan struct{}, scheduledTestDefaultMaxWorkers)
	var wg sync.WaitGroup

	for _, plan := range plans {
		sem <- struct{}{}
		wg.Add(1)
		go func(p *ScheduledTestPlan) {
			defer wg.Done()
			defer func() { <-sem }()
			s.runOnePlan(ctx, p)
		}(plan)
	}

	wg.Wait()
}

func (s *ScheduledTestRunnerService) runOnePlan(ctx context.Context, plan *ScheduledTestPlan) {
	if _, loaded := s.running.LoadOrStore(plan.ID, struct{}{}); loaded {
		return
	}
	defer s.running.Delete(plan.ID)

	if plan.TriggerMode == ScheduledTestTriggerErrorRecovery {
		s.runErrorRecoveryPlan(ctx, plan)
		return
	}

	s.runModelTest(ctx, plan, plan.ModelID)
	nextRun, err := computeNextRun(plan.CronExpression, time.Now())
	if err != nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d computeNextRun error: %v", plan.ID, err)
		return
	}
	if err := s.planRepo.UpdateAfterRun(ctx, plan.ID, time.Now(), nextRun); err != nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d UpdateAfterRun error: %v", plan.ID, err)
	}
}

func (s *ScheduledTestRunnerService) runErrorRecoveryPlan(ctx context.Context, plan *ScheduledTestPlan) {
	if s.rateLimitSvc == nil {
		return
	}
	startedAt := time.Now().Truncate(time.Second)
	targets, err := s.rateLimitSvc.ErrorRecoveryTargets(ctx, plan.AccountID, plan.ModelID)
	if err != nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d recovery targets error: %v", plan.ID, err)
		return
	}
	if len(targets) == 0 {
		if err := s.planRepo.UpdateNextRun(ctx, plan.ID, startedAt.Add(time.Minute)); err != nil {
			logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d UpdateNextRun error: %v", plan.ID, err)
		}
		return
	}

	for _, target := range targets {
		if len(plan.ModelIDs) > 0 && !containsModelID(plan.ModelIDs, target.ModelID) && !target.RecoverAccountState {
			continue
		}
		s.runModelTest(ctx, plan, target.ModelID, &target)
	}
	nextRun, err := computeRecoveryNextRun(plan, startedAt)
	if err != nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d compute recovery next run error: %v", plan.ID, err)
		return
	}
	if err := s.planRepo.UpdateAfterRun(ctx, plan.ID, time.Now(), nextRun); err != nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d UpdateAfterRun error: %v", plan.ID, err)
	}
}

func containsModelID(modelIDs []string, modelID string) bool {
	for _, candidate := range modelIDs {
		if candidate == modelID {
			return true
		}
	}
	return false
}

func (s *ScheduledTestRunnerService) runModelTest(ctx context.Context, plan *ScheduledTestPlan, modelID string, recoveryTarget ...*ErrorRecoveryTarget) {
	startedAt := time.Now()
	runningResult := &ScheduledTestResult{
		ModelID:    modelID,
		Status:     "running",
		StartedAt:  startedAt,
		FinishedAt: startedAt,
	}
	savedResult, err := s.scheduledSvc.StartResult(ctx, plan.ID, runningResult)
	if err != nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d model=%s StartResult error: %v", plan.ID, modelID, err)
		return
	}

	result, err := s.accountTestSvc.RunTestBackground(ctx, plan.AccountID, modelID)
	if err != nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d model=%s RunTestBackground error: %v", plan.ID, modelID, err)
		runningResult.ID = savedResult.ID
		runningResult.Status = "failed"
		runningResult.ErrorMessage = err.Error()
		runningResult.FinishedAt = time.Now()
		runningResult.LatencyMs = runningResult.FinishedAt.Sub(startedAt).Milliseconds()
		if updateErr := s.scheduledSvc.UpdateResult(ctx, plan.ID, plan.MaxResults, runningResult); updateErr != nil {
			logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d model=%s failed result update error: %v", plan.ID, modelID, updateErr)
		}
		return
	}
	if result == nil {
		runningResult.ID = savedResult.ID
		runningResult.Status = "failed"
		runningResult.ErrorMessage = fmt.Sprintf("background test returned no result for model %s", modelID)
		runningResult.FinishedAt = time.Now()
		runningResult.LatencyMs = runningResult.FinishedAt.Sub(startedAt).Milliseconds()
		if updateErr := s.scheduledSvc.UpdateResult(ctx, plan.ID, plan.MaxResults, runningResult); updateErr != nil {
			logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d model=%s nil result update error: %v", plan.ID, modelID, updateErr)
		}
		return
	}
	result.ModelID = modelID
	result.ID = savedResult.ID
	if result.StartedAt.IsZero() {
		result.StartedAt = startedAt
	}
	if result.FinishedAt.IsZero() {
		result.FinishedAt = time.Now()
	}
	if err := s.scheduledSvc.UpdateResult(ctx, plan.ID, plan.MaxResults, result); err != nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d SaveResult error: %v", plan.ID, err)
	}

	// Auto-recover account if test succeeded and auto_recover is enabled.
	if result.Status == "success" && plan.AutoRecover {
		if len(recoveryTarget) > 0 && recoveryTarget[0] != nil {
			s.tryRecoverErrorTarget(ctx, plan.AccountID, plan.ID, *recoveryTarget[0])
			return
		}
		s.tryRecoverAccount(ctx, plan.AccountID, plan.ID, modelID)
	}
}

func (s *ScheduledTestRunnerService) tryRecoverErrorTarget(ctx context.Context, accountID int64, planID int64, target ErrorRecoveryTarget) {
	recovery, err := s.rateLimitSvc.RecoverErrorTargetAfterSuccessfulTest(ctx, accountID, target)
	if err != nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d conditional auto-recover failed: %v", planID, err)
		return
	}
	if recovery.ClearedModelRateLimit {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d auto-recover: account=%d model=%s recovered", planID, accountID, target.ModelID)
	}
}

// tryRecoverAccount attempts to recover an account from recoverable runtime state.
func (s *ScheduledTestRunnerService) tryRecoverAccount(ctx context.Context, accountID int64, planID int64, modelID string) {
	if s.rateLimitSvc == nil {
		return
	}

	recovery, err := s.rateLimitSvc.RecoverAccountAfterSuccessfulTest(ctx, accountID, modelID)
	if err != nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d auto-recover failed: %v", planID, err)
		return
	}
	if recovery == nil {
		return
	}

	if recovery.ClearedError {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d auto-recover: account=%d recovered from error status", planID, accountID)
	}
	if recovery.ClearedRateLimit {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d auto-recover: account=%d cleared rate-limit/runtime state", planID, accountID)
	}
	if recovery.ClearedModelRateLimit {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d auto-recover: account=%d model=%s recovered", planID, accountID, modelID)
	}
}
