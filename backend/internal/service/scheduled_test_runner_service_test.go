//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type scheduledPlanRepoStub struct {
	nextRuns      []time.Time
	afterRunTimes []time.Time
}

func (r *scheduledPlanRepoStub) Create(context.Context, *ScheduledTestPlan) (*ScheduledTestPlan, error) {
	panic("unexpected call")
}
func (r *scheduledPlanRepoStub) GetByID(context.Context, int64) (*ScheduledTestPlan, error) {
	panic("unexpected call")
}
func (r *scheduledPlanRepoStub) ListByAccountID(context.Context, int64) ([]*ScheduledTestPlan, error) {
	panic("unexpected call")
}
func (r *scheduledPlanRepoStub) ListDue(context.Context, time.Time) ([]*ScheduledTestPlan, error) {
	panic("unexpected call")
}
func (r *scheduledPlanRepoStub) Update(context.Context, *ScheduledTestPlan) (*ScheduledTestPlan, error) {
	panic("unexpected call")
}
func (r *scheduledPlanRepoStub) Delete(context.Context, int64) error { panic("unexpected call") }
func (r *scheduledPlanRepoStub) UpdateAfterRun(_ context.Context, _ int64, _ time.Time, next time.Time) error {
	r.afterRunTimes = append(r.afterRunTimes, next)
	return nil
}
func (r *scheduledPlanRepoStub) UpdateNextRun(_ context.Context, _ int64, next time.Time) error {
	r.nextRuns = append(r.nextRuns, next)
	return nil
}

type scheduledResultRepoStub struct {
	results        []*ScheduledTestResult
	createStatuses []string
	updateStatuses []string
}

func (r *scheduledResultRepoStub) Create(_ context.Context, result *ScheduledTestResult) (*ScheduledTestResult, error) {
	copy := *result
	copy.ID = int64(len(r.results) + 1)
	r.results = append(r.results, &copy)
	r.createStatuses = append(r.createStatuses, result.Status)
	return &copy, nil
}
func (r *scheduledResultRepoStub) Update(_ context.Context, result *ScheduledTestResult) error {
	r.updateStatuses = append(r.updateStatuses, result.Status)
	for _, existing := range r.results {
		if existing.ID == result.ID {
			*existing = *result
			return nil
		}
	}
	return nil
}
func (r *scheduledResultRepoStub) ListByPlanID(context.Context, int64, int) ([]*ScheduledTestResult, error) {
	return r.results, nil
}
func (r *scheduledResultRepoStub) PruneOldResults(context.Context, int64, string, int) error {
	return nil
}

type scheduledTesterStub struct {
	models          []string
	canonicalModels []string
	status          map[string]string
	err             map[string]error
	delay           time.Duration
	beforeReturn    func(string)
}

func (s *scheduledTesterStub) RunTestBackground(_ context.Context, _ int64, modelID string) (*ScheduledTestResult, error) {
	s.models = append(s.models, modelID)
	return s.run(modelID)
}

func (s *scheduledTesterStub) RunCanonicalTestBackground(_ context.Context, _ int64, modelID string) (*ScheduledTestResult, error) {
	s.canonicalModels = append(s.canonicalModels, modelID)
	return s.run(modelID)
}

func (s *scheduledTesterStub) run(modelID string) (*ScheduledTestResult, error) {
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	if s.beforeReturn != nil {
		s.beforeReturn(modelID)
	}
	if err := s.err[modelID]; err != nil {
		return nil, err
	}
	return &ScheduledTestResult{ModelID: modelID, Status: s.status[modelID]}, nil
}

func targetModelIDs(targets []ErrorRecoveryTarget) []string {
	models := make([]string, 0, len(targets))
	for _, target := range targets {
		models = append(models, target.ModelID)
	}
	return models
}

func activeModelLimits(resetAt time.Time, models ...string) map[string]any {
	limits := make(map[string]any, len(models))
	for _, modelID := range models {
		limits[modelID] = map[string]any{
			"rate_limit_reset_at": resetAt.Format(time.RFC3339),
			"updated_at":          time.Now().UTC().Format(time.RFC3339Nano),
		}
	}
	return limits
}

func TestScheduledTestRunner_ErrorRecoveryTestsEachModelAndClearsOnlySuccess(t *testing.T) {
	interval := 5
	accountRepo := &rateLimitClearRepoStub{getByIDAccount: &Account{
		ID:          42,
		Status:      StatusActive,
		Schedulable: true,
		Extra: map[string]any{"model_rate_limits": activeModelLimits(
			time.Now().Add(time.Hour), "gpt-5.6-luna", "gpt-5.6-sol", "gpt-5.6-terra",
			creditsExhaustedKey, antigravityGeminiModelRateLimitKey, openAIImageGenerationRateLimitKey,
		)},
	}}
	planRepo := &scheduledPlanRepoStub{}
	resultRepo := &scheduledResultRepoStub{}
	tester := &scheduledTesterStub{status: map[string]string{
		"gpt-5.6-luna":  "failed",
		"gpt-5.6-sol":   "success",
		"gpt-5.6-terra": "failed",
	}}
	rateLimits := NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil)
	runner := &ScheduledTestRunnerService{
		planRepo:       planRepo,
		scheduledSvc:   NewScheduledTestService(planRepo, resultRepo),
		accountTestSvc: tester,
		rateLimitSvc:   rateLimits,
	}
	plan := &ScheduledTestPlan{
		ID: 7, AccountID: 42, ModelID: "gpt-5.6-luna", TriggerMode: ScheduledTestTriggerErrorRecovery,
		RetryIntervalMinutes: &interval, AutoRecover: true, MaxResults: 100,
	}

	runner.runOnePlan(context.Background(), plan)

	require.Equal(t, []string{"gpt-5.6-luna", "gpt-5.6-sol", "gpt-5.6-terra"}, tester.canonicalModels)
	require.Len(t, resultRepo.results, 3)
	require.Equal(t, []string{"running", "running", "running"}, resultRepo.createStatuses)
	require.Equal(t, []string{"failed", "success", "failed"}, resultRepo.updateStatuses)
	require.Equal(t, []string{"gpt-5.6-sol"}, accountRepo.clearModelRateLimitKeys)
	require.Len(t, planRepo.afterRunTimes, 1)
	require.WithinDuration(t, time.Now().Add(5*time.Minute), planRepo.afterRunTimes[0], 2*time.Second)
}

func TestScheduledTestRunner_ErrorRecoveryOnlyTestsSelectedFailedModels(t *testing.T) {
	interval := 5
	accountRepo := &rateLimitClearRepoStub{getByIDAccount: &Account{
		ID: 43, Status: StatusActive, Schedulable: true,
		Extra: map[string]any{"model_rate_limits": activeModelLimits(
			time.Now().Add(time.Hour), "gpt-5.6-luna", "gpt-5.6-sol", "gpt-5.6-terra",
		)},
	}}
	planRepo := &scheduledPlanRepoStub{}
	tester := &scheduledTesterStub{status: map[string]string{"gpt-5.6-sol": "failed"}}
	runner := &ScheduledTestRunnerService{
		planRepo: planRepo, scheduledSvc: NewScheduledTestService(planRepo, &scheduledResultRepoStub{}),
		accountTestSvc: tester, rateLimitSvc: NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil),
	}
	runner.runOnePlan(context.Background(), &ScheduledTestPlan{
		ID: 13, AccountID: 43, ModelID: "gpt-5.6-sol", ModelIDs: []string{"gpt-5.6-sol"},
		TriggerMode: ScheduledTestTriggerErrorRecovery, RetryIntervalMinutes: &interval, AutoRecover: true, MaxResults: 100,
	})

	require.Equal(t, []string{"gpt-5.6-sol"}, tester.canonicalModels)
}

func TestScheduledTestRunner_ErrorRecoveryMapsPlanModelOnceAndProbesCanonicalTarget(t *testing.T) {
	interval := 5
	account := &Account{
		ID:          143,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{"model_mapping": map[string]any{
			"public-alias": "upstream-a",
			"upstream-a":   "upstream-b",
		}},
		Extra: map[string]any{modelRateLimitsKey: activeModelLimits(time.Now().Add(time.Hour), "upstream-a")},
	}
	accountRepo := &rateLimitClearRepoStub{getByIDAccount: account}
	planRepo := &scheduledPlanRepoStub{}
	tester := &scheduledTesterStub{status: map[string]string{"upstream-a": "success"}}
	runner := &ScheduledTestRunnerService{
		planRepo: planRepo, scheduledSvc: NewScheduledTestService(planRepo, &scheduledResultRepoStub{}),
		accountTestSvc: tester, rateLimitSvc: NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil),
	}

	runner.runOnePlan(context.Background(), &ScheduledTestPlan{
		ID: 113, AccountID: account.ID, ModelID: "public-alias", ModelIDs: []string{"public-alias"},
		TriggerMode: ScheduledTestTriggerErrorRecovery, RetryIntervalMinutes: &interval, AutoRecover: true, MaxResults: 100,
	})

	require.Empty(t, tester.models)
	require.Equal(t, []string{"upstream-a"}, tester.canonicalModels)
	require.Equal(t, []string{"upstream-a"}, accountRepo.clearModelRateLimitKeys)
}

func TestScheduledTestRunner_ErrorRecoveryExactTargetWinsOverMappedSibling(t *testing.T) {
	interval := 5
	account := &Account{
		ID: 144, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{"model_mapping": map[string]any{"upstream-a": "upstream-b"}},
		Extra: map[string]any{modelRateLimitsKey: activeModelLimits(
			time.Now().Add(time.Hour), "upstream-a", "upstream-b",
		)},
	}
	accountRepo := &rateLimitClearRepoStub{getByIDAccount: account}
	planRepo := &scheduledPlanRepoStub{}
	tester := &scheduledTesterStub{status: map[string]string{"upstream-a": "success", "upstream-b": "success"}}
	runner := &ScheduledTestRunnerService{
		planRepo: planRepo, scheduledSvc: NewScheduledTestService(planRepo, &scheduledResultRepoStub{}),
		accountTestSvc: tester, rateLimitSvc: NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil),
	}

	runner.runOnePlan(context.Background(), &ScheduledTestPlan{
		ID: 114, AccountID: account.ID, ModelID: "upstream-a", ModelIDs: []string{"upstream-a"},
		TriggerMode: ScheduledTestTriggerErrorRecovery, RetryIntervalMinutes: &interval, AutoRecover: true, MaxResults: 100,
	})

	require.Equal(t, []string{"upstream-a"}, tester.canonicalModels)
	require.Equal(t, []string{"upstream-a"}, accountRepo.clearModelRateLimitKeys)
}

func TestScheduledTestRunner_ErrorRecoveryConstrainedPlanWithNoMatchRunsNothing(t *testing.T) {
	tests := []struct {
		name        string
		modelIDs    []string
		mapping     map[string]any
		getByIDFail bool
	}{
		{name: "unknown model", modelIDs: []string{"unknown"}},
		{name: "mapped target missing", modelIDs: []string{"public-alias"}, mapping: map[string]any{"public-alias": "missing-upstream"}},
		{name: "mapping lookup fails", modelIDs: []string{"public-alias"}, mapping: map[string]any{"public-alias": "upstream-a"}, getByIDFail: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interval := 5
			account := &Account{
				ID: 145, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true,
				Credentials: map[string]any{"model_mapping": tt.mapping},
				Extra:       map[string]any{modelRateLimitsKey: activeModelLimits(time.Now().Add(time.Hour), "upstream-a")},
			}
			repo := &rateLimitClearRepoStub{getByIDAccount: account}
			if tt.getByIDFail {
				repo.getByIDErr = context.DeadlineExceeded
				repo.getByIDErrAfter = 1
			}
			planRepo := &scheduledPlanRepoStub{}
			tester := &scheduledTesterStub{status: map[string]string{"upstream-a": "success"}}
			runner := &ScheduledTestRunnerService{
				planRepo: planRepo, scheduledSvc: NewScheduledTestService(planRepo, &scheduledResultRepoStub{}),
				accountTestSvc: tester, rateLimitSvc: NewRateLimitService(repo, nil, &config.Config{}, nil, nil),
			}

			runner.runOnePlan(context.Background(), &ScheduledTestPlan{
				ID: 115, AccountID: account.ID, ModelID: tt.modelIDs[0], ModelIDs: tt.modelIDs,
				TriggerMode: ScheduledTestTriggerErrorRecovery, RetryIntervalMinutes: &interval, AutoRecover: true, MaxResults: 100,
			})

			require.Empty(t, tester.canonicalModels)
			require.Empty(t, repo.clearModelRateLimitKeys)
		})
	}
}

func TestScheduledTestRunner_ErrorRecoveryDoesNotRecoverWhenDisabled(t *testing.T) {
	interval := 5
	accountRepo := &rateLimitClearRepoStub{getByIDAccount: &Account{
		ID: 44, Status: StatusActive, Schedulable: true,
		Extra: map[string]any{"model_rate_limits": activeModelLimits(time.Now().Add(time.Hour), "gpt-5.6-sol")},
	}}
	planRepo := &scheduledPlanRepoStub{}
	tester := &scheduledTesterStub{status: map[string]string{"gpt-5.6-sol": "success"}}
	runner := &ScheduledTestRunnerService{
		planRepo: planRepo, scheduledSvc: NewScheduledTestService(planRepo, &scheduledResultRepoStub{}),
		accountTestSvc: tester, rateLimitSvc: NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil),
	}
	runner.runOnePlan(context.Background(), &ScheduledTestPlan{
		ID: 14, AccountID: 44, ModelID: "gpt-5.6-sol", TriggerMode: ScheduledTestTriggerErrorRecovery,
		RetryIntervalMinutes: &interval, AutoRecover: false, MaxResults: 100,
	})

	require.Equal(t, []string{"gpt-5.6-sol"}, tester.canonicalModels)
	require.Empty(t, accountRepo.clearModelRateLimitKeys)
}

func TestScheduledTestRunner_ErrorRecoverySkipsHealthyAndRunningPlans(t *testing.T) {
	interval := 5
	accountRepo := &rateLimitClearRepoStub{getByIDAccount: &Account{
		ID: 8, Status: StatusActive, Schedulable: true, Extra: map[string]any{},
	}}
	planRepo := &scheduledPlanRepoStub{}
	tester := &scheduledTesterStub{status: map[string]string{}}
	runner := &ScheduledTestRunnerService{
		planRepo:       planRepo,
		scheduledSvc:   NewScheduledTestService(planRepo, &scheduledResultRepoStub{}),
		accountTestSvc: tester,
		rateLimitSvc:   NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil),
	}
	plan := &ScheduledTestPlan{
		ID: 9, AccountID: 8, ModelID: "gpt-5.6-luna", TriggerMode: ScheduledTestTriggerErrorRecovery,
		RetryIntervalMinutes: &interval, AutoRecover: true, MaxResults: 100,
	}

	startedAt := time.Now()
	runner.runOnePlan(context.Background(), plan)
	require.Empty(t, tester.models)
	require.Len(t, planRepo.nextRuns, 1)
	require.Empty(t, planRepo.afterRunTimes)
	require.WithinDuration(t, startedAt.Add(time.Minute), planRepo.nextRuns[0], time.Second)
	require.False(t, planRepo.nextRuns[0].After(startedAt.Add(time.Minute)))
	require.Zero(t, planRepo.nextRuns[0].Nanosecond())

	runner.running.Store(plan.ID, struct{}{})
	runner.runOnePlan(context.Background(), plan)
	require.Empty(t, tester.models)
	require.Len(t, planRepo.nextRuns, 1)
}

func TestScheduledTestRunner_ErrorRecoverySchedulesFromStartAfterSlowProbe(t *testing.T) {
	interval := 1
	accountRepo := &rateLimitClearRepoStub{getByIDAccount: &Account{
		ID: 15, Status: StatusActive, Schedulable: true,
		Extra: map[string]any{"model_rate_limits": activeModelLimits(time.Now().Add(time.Hour), "gpt-5.6-sol")},
	}}
	planRepo := &scheduledPlanRepoStub{}
	tester := &scheduledTesterStub{
		status: map[string]string{"gpt-5.6-sol": "failed"},
		delay:  200 * time.Millisecond,
	}
	runner := &ScheduledTestRunnerService{
		planRepo: planRepo, scheduledSvc: NewScheduledTestService(planRepo, &scheduledResultRepoStub{}),
		accountTestSvc: tester, rateLimitSvc: NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil),
	}
	startedAt := time.Now()
	runner.runOnePlan(context.Background(), &ScheduledTestPlan{
		ID: 15, AccountID: 15, ModelID: "gpt-5.6-sol", TriggerMode: ScheduledTestTriggerErrorRecovery,
		RetryIntervalMinutes: &interval, AutoRecover: false, MaxResults: 100,
	})

	require.Len(t, planRepo.afterRunTimes, 1)
	require.WithinDuration(t, startedAt.Add(time.Minute), planRepo.afterRunTimes[0], time.Second)
	require.False(t, planRepo.afterRunTimes[0].After(startedAt.Add(time.Minute)))
	require.Zero(t, planRepo.afterRunTimes[0].Nanosecond(), "next run must not retain sub-second drift past the fixed scheduler tick")
}

func TestScheduledTestRunner_SavesFailedResultWhenBackgroundTestErrors(t *testing.T) {
	interval := 5
	accountRepo := &rateLimitClearRepoStub{getByIDAccount: &Account{
		ID: 16, Status: StatusActive, Schedulable: true,
		Extra: map[string]any{"model_rate_limits": activeModelLimits(time.Now().Add(time.Hour), "gpt-5.6-sol")},
	}}
	planRepo := &scheduledPlanRepoStub{}
	resultRepo := &scheduledResultRepoStub{}
	tester := &scheduledTesterStub{err: map[string]error{"gpt-5.6-sol": context.DeadlineExceeded}}
	runner := &ScheduledTestRunnerService{
		planRepo: planRepo, scheduledSvc: NewScheduledTestService(planRepo, resultRepo),
		accountTestSvc: tester, rateLimitSvc: NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil),
	}
	runner.runOnePlan(context.Background(), &ScheduledTestPlan{
		ID: 16, AccountID: 16, ModelID: "gpt-5.6-sol", TriggerMode: ScheduledTestTriggerErrorRecovery,
		RetryIntervalMinutes: &interval, AutoRecover: false, MaxResults: 100,
	})

	require.Len(t, resultRepo.results, 1)
	require.Equal(t, []string{"running"}, resultRepo.createStatuses)
	require.Equal(t, []string{"failed"}, resultRepo.updateStatuses)
	require.Equal(t, "failed", resultRepo.results[0].Status)
	require.Equal(t, context.DeadlineExceeded.Error(), resultRepo.results[0].ErrorMessage)
}

func TestScheduledTestRunner_ErrorRecoveryClearsTwoSuccessfulModelsInOneRun(t *testing.T) {
	interval := 5
	accountRepo := &rateLimitClearRepoStub{getByIDAccount: &Account{
		ID: 12, Status: StatusActive, Schedulable: true, UpdatedAt: time.Now(),
		Extra: map[string]any{"model_rate_limits": activeModelLimits(time.Now().Add(time.Hour), "gpt-5.6-luna", "gpt-5.6-sol")},
	}}
	planRepo := &scheduledPlanRepoStub{}
	tester := &scheduledTesterStub{status: map[string]string{"gpt-5.6-luna": "success", "gpt-5.6-sol": "success"}}
	runner := &ScheduledTestRunnerService{
		planRepo: planRepo, scheduledSvc: NewScheduledTestService(planRepo, &scheduledResultRepoStub{}),
		accountTestSvc: tester, rateLimitSvc: NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil),
	}
	runner.runOnePlan(context.Background(), &ScheduledTestPlan{
		ID: 12, AccountID: 12, ModelID: "gpt-5.6-luna", TriggerMode: ScheduledTestTriggerErrorRecovery,
		RetryIntervalMinutes: &interval, AutoRecover: true, MaxResults: 100,
	})

	require.Equal(t, []string{"gpt-5.6-luna", "gpt-5.6-sol"}, accountRepo.clearModelRateLimitKeys)
}

func TestScheduledTestRunner_ErrorRecoveryDoesNotClearNewerModelError(t *testing.T) {
	interval := 5
	oldReset := time.Now().Add(time.Hour)
	accountRepo := &rateLimitClearRepoStub{getByIDAccount: &Account{
		ID: 10, Status: StatusActive, Schedulable: true, UpdatedAt: time.Now(),
		Extra: map[string]any{"model_rate_limits": activeModelLimits(oldReset, "gpt-5.6-sol")},
	}}
	tester := &scheduledTesterStub{status: map[string]string{"gpt-5.6-sol": "success"}}
	tester.beforeReturn = func(modelID string) {
		limits := accountRepo.getByIDAccount.Extra["model_rate_limits"].(map[string]any)
		limits[modelID].(map[string]any)["updated_at"] = time.Now().Add(time.Second).UTC().Format(time.RFC3339Nano)
	}
	planRepo := &scheduledPlanRepoStub{}
	runner := &ScheduledTestRunnerService{
		planRepo: planRepo, scheduledSvc: NewScheduledTestService(planRepo, &scheduledResultRepoStub{}),
		accountTestSvc: tester, rateLimitSvc: NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil),
	}
	runner.runOnePlan(context.Background(), &ScheduledTestPlan{
		ID: 10, AccountID: 10, ModelID: "gpt-5.6-luna", TriggerMode: ScheduledTestTriggerErrorRecovery,
		RetryIntervalMinutes: &interval, AutoRecover: true, MaxResults: 100,
	})

	require.Empty(t, accountRepo.clearModelRateLimitKeys)
	require.True(t, accountRepo.getByIDAccount.isRateLimitActiveForKey("gpt-5.6-sol"))
}

func TestRateLimitService_ErrorRecoveryTargetsExcludeManualAndCreditsStates(t *testing.T) {
	resetAt := time.Now().Add(time.Hour)
	account := &Account{
		ID: 11, Status: StatusActive, Schedulable: true,
		Extra: map[string]any{"model_rate_limits": activeModelLimits(resetAt, "gpt-5.6-sol", creditsExhaustedKey)},
	}
	repo := &rateLimitClearRepoStub{getByIDAccount: account}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)

	targets, err := svc.ErrorRecoveryTargets(context.Background(), 11, "gpt-5.6-luna")
	require.NoError(t, err)
	require.Equal(t, []string{"gpt-5.6-sol"}, targetModelIDs(targets))

	account.Schedulable = false
	targets, err = svc.ErrorRecoveryTargets(context.Background(), 11, "gpt-5.6-luna")
	require.NoError(t, err)
	require.Empty(t, targets)

	account.Status = StatusError
	account.Extra = map[string]any{"model_rate_limits": activeModelLimits(resetAt, "gpt-5.6-sol")}
	targets, err = svc.ErrorRecoveryTargets(context.Background(), 11, "gpt-5.6-luna")
	require.NoError(t, err)
	require.Empty(t, targets)

	account.UpdatedAt = time.Now()
	futureWindow := time.Now().Add(time.Hour)
	account.TempUnschedulableUntil = &futureWindow
	for _, internalScope := range []string{antigravityGeminiModelRateLimitKey, openAIImageGenerationRateLimitKey} {
		targets, err = svc.ErrorRecoveryTargets(context.Background(), 11, internalScope)
		require.NoError(t, err)
		require.Equal(t, []string{"gpt-5.6-sol"}, targetModelIDs(targets))
	}
	targets, err = svc.ErrorRecoveryTargets(context.Background(), 11, "gpt-5.6-luna")
	require.NoError(t, err)
	require.Equal(t, []string{"gpt-5.6-luna", "gpt-5.6-sol"}, targetModelIDs(targets))
	require.True(t, targets[0].RecoverAccountState)

	account.Status = StatusActive
	account.Schedulable = true
	account.TempUnschedulableUntil = nil
	account.Extra = map[string]any{"model_rate_limits": activeModelLimits(resetAt, "gpt-5.6-sol")}
	expiredAt := time.Now().Add(-time.Minute)
	account.ExpiresAt = &expiredAt
	targets, err = svc.ErrorRecoveryTargets(context.Background(), 11, "gpt-5.6-luna")
	require.NoError(t, err)
	require.Empty(t, targets)

	account.ExpiresAt = nil
	account.Extra = map[string]any{}
	expiredWindow := time.Now().Add(-time.Minute)
	account.RateLimitedAt = &expiredWindow
	account.RateLimitResetAt = &expiredWindow
	account.OverloadUntil = &expiredWindow
	account.TempUnschedulableUntil = &expiredWindow
	targets, err = svc.ErrorRecoveryTargets(context.Background(), 11, "gpt-5.6-luna")
	require.NoError(t, err)
	require.Empty(t, targets)
}

func TestScheduledTestService_ErrorRecoveryScheduleValidation(t *testing.T) {
	from := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	interval := 5
	plan := &ScheduledTestPlan{
		ModelID: "gpt-5.6-luna", TriggerMode: ScheduledTestTriggerErrorRecovery,
		RetryIntervalMinutes: &interval, AutoRecover: true,
	}
	next, err := validateAndComputePlanNextRun(plan, from)
	require.NoError(t, err)
	require.Equal(t, from, next)
	require.True(t, plan.AutoRecover)

	plan.AutoRecover = false
	next, err = validateAndComputePlanNextRun(plan, from)
	require.NoError(t, err)
	require.Equal(t, from, next)
	require.False(t, plan.AutoRecover)

	cronExpr := "*/7 * * * *"
	plan.RetryIntervalMinutes = nil
	plan.RetryCronExpression = &cronExpr
	_, err = validateAndComputePlanNextRun(plan, from)
	require.NoError(t, err)
	next, err = computeRecoveryNextRun(plan, from)
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 8, 1, 10, 7, 0, 0, time.UTC), next)

	plan.RetryIntervalMinutes = &interval
	_, err = validateAndComputePlanNextRun(plan, from)
	require.ErrorContains(t, err, "cannot be combined")

	for _, internalScope := range []string{antigravityGeminiModelRateLimitKey, openAIImageGenerationRateLimitKey} {
		plan.RetryCronExpression = nil
		plan.ModelID = internalScope
		plan.ModelIDs = nil
		_, err = validateAndComputePlanNextRun(plan, from)
		require.ErrorContains(t, err, "internal rate-limit scope")

		plan.ModelID = "gpt-5.6-luna"
		plan.ModelIDs = []string{internalScope}
		_, err = validateAndComputePlanNextRun(plan, from)
		require.ErrorContains(t, err, "internal rate-limit scope")
	}
}
