package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

var scheduledTestCronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

const (
	ScheduledTestTriggerScheduled     = "scheduled"
	ScheduledTestTriggerErrorRecovery = "error_recovery"
	scheduledTestDefaultCron          = "*/30 * * * *"
)

// ScheduledTestService provides CRUD operations for scheduled test plans and results.
type ScheduledTestService struct {
	planRepo   ScheduledTestPlanRepository
	resultRepo ScheduledTestResultRepository
}

// NewScheduledTestService creates a new ScheduledTestService.
func NewScheduledTestService(
	planRepo ScheduledTestPlanRepository,
	resultRepo ScheduledTestResultRepository,
) *ScheduledTestService {
	return &ScheduledTestService{
		planRepo:   planRepo,
		resultRepo: resultRepo,
	}
}

// CreatePlan validates the cron expression, computes next_run_at, and persists the plan.
func (s *ScheduledTestService) CreatePlan(ctx context.Context, plan *ScheduledTestPlan) (*ScheduledTestPlan, error) {
	nextRun, err := validateAndComputePlanNextRun(plan, time.Now())
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression: %w", err)
	}
	plan.NextRunAt = &nextRun

	if plan.MaxResults <= 0 {
		plan.MaxResults = 50
	}

	return s.planRepo.Create(ctx, plan)
}

// GetPlan retrieves a plan by ID.
func (s *ScheduledTestService) GetPlan(ctx context.Context, id int64) (*ScheduledTestPlan, error) {
	return s.planRepo.GetByID(ctx, id)
}

// ListPlansByAccount returns all plans for a given account.
func (s *ScheduledTestService) ListPlansByAccount(ctx context.Context, accountID int64) ([]*ScheduledTestPlan, error) {
	return s.planRepo.ListByAccountID(ctx, accountID)
}

// UpdatePlan validates cron and updates the plan.
func (s *ScheduledTestService) UpdatePlan(ctx context.Context, plan *ScheduledTestPlan) (*ScheduledTestPlan, error) {
	nextRun, err := validateAndComputePlanNextRun(plan, time.Now())
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression: %w", err)
	}
	plan.NextRunAt = &nextRun

	return s.planRepo.Update(ctx, plan)
}

// DeletePlan removes a plan and its results (via CASCADE).
func (s *ScheduledTestService) DeletePlan(ctx context.Context, id int64) error {
	return s.planRepo.Delete(ctx, id)
}

// ListResults returns the most recent results for a plan.
func (s *ScheduledTestService) ListResults(ctx context.Context, planID int64, limit int) ([]*ScheduledTestResult, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.resultRepo.ListByPlanID(ctx, planID, limit)
}

// SaveResult inserts a result and prunes old entries beyond maxResults.
func (s *ScheduledTestService) SaveResult(ctx context.Context, planID int64, maxResults int, result *ScheduledTestResult) error {
	result.PlanID = planID
	if _, err := s.resultRepo.Create(ctx, result); err != nil {
		return err
	}
	return s.resultRepo.PruneOldResults(ctx, planID, result.ModelID, maxResults)
}

func (s *ScheduledTestService) StartResult(ctx context.Context, planID int64, result *ScheduledTestResult) (*ScheduledTestResult, error) {
	result.PlanID = planID
	return s.resultRepo.Create(ctx, result)
}

func (s *ScheduledTestService) UpdateResult(ctx context.Context, planID int64, maxResults int, result *ScheduledTestResult) error {
	result.PlanID = planID
	if err := s.resultRepo.Update(ctx, result); err != nil {
		return err
	}
	return s.resultRepo.PruneOldResults(ctx, planID, result.ModelID, maxResults)
}

func validateAndComputePlanNextRun(plan *ScheduledTestPlan, from time.Time) (time.Time, error) {
	plan.ModelID = strings.TrimSpace(plan.ModelID)
	plan.ModelIDs = normalizeModelIDs(plan.ModelIDs)
	if plan.TriggerMode == "" {
		plan.TriggerMode = ScheduledTestTriggerScheduled
	}
	if plan.TriggerMode == ScheduledTestTriggerScheduled {
		plan.RetryIntervalMinutes = nil
		plan.RetryCronExpression = nil
		return computeNextRun(plan.CronExpression, from)
	}
	if plan.TriggerMode != ScheduledTestTriggerErrorRecovery {
		return time.Time{}, fmt.Errorf("invalid trigger mode: %s", plan.TriggerMode)
	}
	if isInternalModelRateLimitScope(plan.ModelID) {
		return time.Time{}, fmt.Errorf("internal rate-limit scope cannot be used as a probe model: %s", plan.ModelID)
	}
	for _, modelID := range plan.ModelIDs {
		if isInternalModelRateLimitScope(modelID) {
			return time.Time{}, fmt.Errorf("internal rate-limit scope cannot be used as a probe model: %s", modelID)
		}
	}
	if len(plan.ModelIDs) > 0 {
		plan.ModelID = plan.ModelIDs[0]
	}
	if plan.ModelID == "" {
		return time.Time{}, fmt.Errorf("error recovery requires an account probe model")
	}

	if plan.CronExpression == "" {
		plan.CronExpression = scheduledTestDefaultCron
	}
	if plan.RetryIntervalMinutes != nil {
		if *plan.RetryIntervalMinutes < 1 || *plan.RetryIntervalMinutes > 1440 || plan.RetryCronExpression != nil {
			return time.Time{}, fmt.Errorf("retry interval must be 1..1440 minutes and cannot be combined with cron")
		}
		return from, nil
	}
	if plan.RetryCronExpression == nil || *plan.RetryCronExpression == "" {
		return time.Time{}, fmt.Errorf("error recovery requires retry interval or cron expression")
	}
	if _, err := computeNextRun(*plan.RetryCronExpression, from); err != nil {
		return time.Time{}, fmt.Errorf("invalid retry cron expression: %w", err)
	}
	return from, nil
}

func normalizeModelIDs(modelIDs []string) []string {
	if len(modelIDs) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(modelIDs))
	result := make([]string, 0, len(modelIDs))
	for _, modelID := range modelIDs {
		modelID = strings.TrimSpace(modelID)
		if modelID == "" {
			continue
		}
		if _, ok := seen[modelID]; ok {
			continue
		}
		seen[modelID] = struct{}{}
		result = append(result, modelID)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func computeRecoveryNextRun(plan *ScheduledTestPlan, from time.Time) (time.Time, error) {
	if plan.RetryIntervalMinutes != nil {
		return from.Add(time.Duration(*plan.RetryIntervalMinutes) * time.Minute), nil
	}
	if plan.RetryCronExpression == nil {
		return time.Time{}, fmt.Errorf("missing retry schedule")
	}
	return computeNextRun(*plan.RetryCronExpression, from)
}

func computeNextRun(cronExpr string, from time.Time) (time.Time, error) {
	sched, err := scheduledTestCronParser.Parse(cronExpr)
	if err != nil {
		return time.Time{}, err
	}
	return sched.Next(from), nil
}
