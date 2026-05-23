package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/robfig/cron/v3"
)

const (
	AccountBatchTestSourceManual    = "manual"
	AccountBatchTestSourceScheduled = "scheduled"

	AccountBatchTestStatusPending   = "pending"
	AccountBatchTestStatusRunning   = "running"
	AccountBatchTestStatusCompleted = "completed"
	AccountBatchTestStatusFailed     = "failed"

	AccountBatchTestResultSuccess = "success"
	AccountBatchTestResultFailed   = "failed"
	AccountBatchTestResultSkipped  = "skipped"

	DefaultAccountHealthCheckModel       = "gpt-5.4-mini"
	DefaultAccountHealthCheckConcurrency = 5
	AccountHealthCheckFailThreshold      = 3
)

type accountHealthCheckOutcome string

const (
	accountHealthCheckOutcomeNormal       accountHealthCheckOutcome = "normal"
	accountHealthCheckOutcomeUnauthorized accountHealthCheckOutcome = "unauthorized"
	accountHealthCheckOutcomeRateLimited  accountHealthCheckOutcome = "rate_limited"
	accountHealthCheckOutcomeFailed       accountHealthCheckOutcome = "failed"
)

type AccountBatchTestTask struct {
	ID               int64      `json:"id"`
	Source           string     `json:"source"`
	Status           string     `json:"status"`
	ModelID          string     `json:"model_id"`
	Concurrency      int        `json:"concurrency"`
	AutoDisable      bool       `json:"auto_disable"`
	TotalCount       int        `json:"total_count"`
	CompletedCount   int        `json:"completed_count"`
	SuccessCount     int        `json:"success_count"`
	FailedCount      int        `json:"failed_count"`
	DeactivatedCount int        `json:"deactivated_count"`
	ErrorMessage     string     `json:"error_message"`
	StartedAt        *time.Time `json:"started_at"`
	FinishedAt       *time.Time `json:"finished_at"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type AccountBatchTestResult struct {
	ID                int64     `json:"id"`
	TaskID            int64     `json:"task_id"`
	AccountID         *int64    `json:"account_id"`
	AccountName       string    `json:"account_name"`
	Platform          string    `json:"platform"`
	AccountType       string    `json:"account_type"`
	Status            string    `json:"status"`
	ResponseText      string    `json:"response_text"`
	ErrorMessage      string    `json:"error_message"`
	LatencyMs         int64     `json:"latency_ms"`
	FailStreak        int       `json:"fail_streak"`
	TriggeredDisabled bool      `json:"triggered_disabled"`
	StartedAt         time.Time `json:"started_at"`
	FinishedAt        time.Time `json:"finished_at"`
	CreatedAt         time.Time `json:"created_at"`
}

type AccountHealthCheckSettings struct {
	Enabled   *bool `json:"health_check_enabled"`
	Protected *bool `json:"health_check_protected"`
}

type AccountBatchTestTaskRepository interface {
	Create(ctx context.Context, task *AccountBatchTestTask) (*AccountBatchTestTask, error)
	GetByID(ctx context.Context, id int64) (*AccountBatchTestTask, error)
	List(ctx context.Context, limit, offset int) ([]*AccountBatchTestTask, int64, error)
	MarkRunning(ctx context.Context, id int64, startedAt time.Time) error
	IncrementProgress(ctx context.Context, id int64, status string, triggeredDisabled bool) error
	Finish(ctx context.Context, id int64, status string, errMsg string, finishedAt time.Time) error
}

type AccountBatchTestResultRepository interface {
	Create(ctx context.Context, result *AccountBatchTestResult) (*AccountBatchTestResult, error)
	ListByTaskID(ctx context.Context, taskID int64, limit, offset int) ([]*AccountBatchTestResult, int64, error)
}

type AccountHealthCheckRepository interface {
	GetAccountsByIDs(ctx context.Context, ids []int64) ([]*Account, error)
	ListScheduledCandidates(ctx context.Context) ([]*Account, error)
	UpdateSettings(ctx context.Context, id int64, settings AccountHealthCheckSettings) (*Account, error)
	BulkUpdateSettings(ctx context.Context, ids []int64, settings AccountHealthCheckSettings) (int64, error)
	RecordResult(ctx context.Context, accountID int64, status string, errorMessage string, failStreak int, disable bool) (*Account, error)
}

type AccountBackgroundTestRunner interface {
	RunTestBackgroundObserveOnly(ctx context.Context, accountID int64, modelID string) (*ScheduledTestResult, error)
}

type AccountHealthCheckService struct {
	taskRepo    AccountBatchTestTaskRepository
	resultRepo  AccountBatchTestResultRepository
	accountRepo AccountHealthCheckRepository
	testRunner  AccountBackgroundTestRunner
}

func NewAccountHealthCheckService(
	taskRepo AccountBatchTestTaskRepository,
	resultRepo AccountBatchTestResultRepository,
	accountRepo AccountHealthCheckRepository,
	testRunner AccountBackgroundTestRunner,
) *AccountHealthCheckService {
	return &AccountHealthCheckService{
		taskRepo:    taskRepo,
		resultRepo:  resultRepo,
		accountRepo: accountRepo,
		testRunner:  testRunner,
	}
}

func (s *AccountHealthCheckService) CreateManualBatchTest(ctx context.Context, accountIDs []int64, modelID string, concurrency int, autoDisable bool) (*AccountBatchTestTask, error) {
	if len(accountIDs) == 0 {
		return nil, errors.New("account_ids is required")
	}
	accounts, err := s.accountRepo.GetAccountsByIDs(ctx, accountIDs)
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, errors.New("no accounts found")
	}
	return s.createAndStartTask(ctx, AccountBatchTestSourceManual, accounts, modelID, concurrency, autoDisable)
}

func (s *AccountHealthCheckService) RunScheduled(ctx context.Context) (*AccountBatchTestTask, error) {
	accounts, err := s.accountRepo.ListScheduledCandidates(ctx)
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, nil
	}
	return s.createAndStartTask(ctx, AccountBatchTestSourceScheduled, accounts, DefaultAccountHealthCheckModel, DefaultAccountHealthCheckConcurrency, false)
}

func (s *AccountHealthCheckService) ListTasks(ctx context.Context, limit, offset int) ([]*AccountBatchTestTask, int64, error) {
	limit = normalizeBatchListLimit(limit)
	if offset < 0 {
		offset = 0
	}
	return s.taskRepo.List(ctx, limit, offset)
}

func (s *AccountHealthCheckService) GetTask(ctx context.Context, id int64, limit, offset int) (*AccountBatchTestTask, []*AccountBatchTestResult, int64, error) {
	task, err := s.taskRepo.GetByID(ctx, id)
	if err != nil {
		return nil, nil, 0, err
	}
	limit = normalizeBatchListLimit(limit)
	if offset < 0 {
		offset = 0
	}
	results, total, err := s.resultRepo.ListByTaskID(ctx, id, limit, offset)
	if err != nil {
		return nil, nil, 0, err
	}
	return task, results, total, nil
}

func (s *AccountHealthCheckService) UpdateSettings(ctx context.Context, accountID int64, settings AccountHealthCheckSettings) (*Account, error) {
	if settings.Enabled == nil && settings.Protected == nil {
		return nil, errors.New("no settings to update")
	}
	return s.accountRepo.UpdateSettings(ctx, accountID, settings)
}

func (s *AccountHealthCheckService) BulkUpdateSettings(ctx context.Context, accountIDs []int64, settings AccountHealthCheckSettings) (int64, error) {
	if len(accountIDs) == 0 {
		return 0, errors.New("account_ids is required")
	}
	if settings.Enabled == nil && settings.Protected == nil {
		return 0, errors.New("no settings to update")
	}
	return s.accountRepo.BulkUpdateSettings(ctx, accountIDs, settings)
}

func (s *AccountHealthCheckService) createAndStartTask(ctx context.Context, source string, accounts []*Account, modelID string, concurrency int, autoDisable bool) (*AccountBatchTestTask, error) {
	if s == nil || s.taskRepo == nil || s.resultRepo == nil || s.accountRepo == nil || s.testRunner == nil {
		return nil, errors.New("account health check service is not configured")
	}
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		modelID = DefaultAccountHealthCheckModel
	}
	if concurrency <= 0 {
		concurrency = DefaultAccountHealthCheckConcurrency
	}
	if concurrency > 50 {
		concurrency = 50
	}

	task, err := s.taskRepo.Create(ctx, &AccountBatchTestTask{
		Source:      source,
		Status:      AccountBatchTestStatusPending,
		ModelID:     modelID,
		Concurrency: concurrency,
		AutoDisable: autoDisable,
		TotalCount:  len(accounts),
	})
	if err != nil {
		return nil, err
	}

	copiedAccounts := make([]*Account, 0, len(accounts))
	for _, account := range accounts {
		if account != nil {
			copied := *account
			copiedAccounts = append(copiedAccounts, &copied)
		}
	}
	go s.runTask(context.Background(), task.ID, copiedAccounts, modelID, concurrency, autoDisable)
	return task, nil
}

func (s *AccountHealthCheckService) runTask(ctx context.Context, taskID int64, accounts []*Account, modelID string, concurrency int, autoDisable bool) {
	if err := s.taskRepo.MarkRunning(ctx, taskID, time.Now()); err != nil {
		logger.LegacyPrintf("service.account_health_check", "[AccountHealthCheck] mark task running failed: task=%d err=%v", taskID, err)
		return
	}

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var firstErr error
	var mu sync.Mutex

	for _, account := range accounts {
		if account == nil {
			continue
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(acc *Account) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := s.runOne(ctx, taskID, acc, modelID, autoDisable); err != nil {
				logger.LegacyPrintf("service.account_health_check", "[AccountHealthCheck] task=%d account=%d failed: %v", taskID, acc.ID, err)
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}(account)
	}
	wg.Wait()

	status := AccountBatchTestStatusCompleted
	errMsg := ""
	if firstErr != nil {
		status = AccountBatchTestStatusFailed
		errMsg = firstErr.Error()
	}
	if err := s.taskRepo.Finish(ctx, taskID, status, errMsg, time.Now()); err != nil {
		logger.LegacyPrintf("service.account_health_check", "[AccountHealthCheck] finish task failed: task=%d err=%v", taskID, err)
	}
}

func (s *AccountHealthCheckService) runOne(ctx context.Context, taskID int64, account *Account, modelID string, autoDisable bool) error {
	result, err := s.testRunner.RunTestBackgroundObserveOnly(ctx, account.ID, modelID)
	if err != nil {
		now := time.Now()
		result = &ScheduledTestResult{
			Status:       AccountBatchTestResultFailed,
			ErrorMessage: err.Error(),
			StartedAt:    now,
			FinishedAt:   now,
		}
	}
	if result == nil {
		now := time.Now()
		result = &ScheduledTestResult{
			Status:       AccountBatchTestResultFailed,
			ErrorMessage: "empty test result",
			StartedAt:    now,
			FinishedAt:   now,
		}
	}

	status := result.Status
	if status != AccountBatchTestResultSuccess {
		status = AccountBatchTestResultFailed
	}

	failStreak := 0
	shouldDisable := false
	errMsg := result.ErrorMessage
	outcome := classifyAccountHealthCheckOutcome(result, status)
	switch outcome {
	case accountHealthCheckOutcomeNormal:
		errMsg = ""
		status = AccountBatchTestResultSuccess
	case accountHealthCheckOutcomeRateLimited:
		errMsg = ""
		status = AccountBatchTestResultSuccess
		result.ResponseText = strings.TrimSpace(result.ResponseText)
		if result.ResponseText == "" {
			result.ResponseText = "429/rate limit treated as healthy"
		}
	case accountHealthCheckOutcomeUnauthorized:
		failStreak = account.HealthCheckFailStreak + 1
		shouldDisable = true
	default:
		failStreak = account.HealthCheckFailStreak + 1
		if autoDisable && failStreak >= AccountHealthCheckFailThreshold && !account.HealthCheckProtected {
			shouldDisable = true
		}
	}

	updated, recordErr := s.accountRepo.RecordResult(ctx, account.ID, status, errMsg, failStreak, shouldDisable)
	if recordErr != nil {
		return fmt.Errorf("record account health result: %w", recordErr)
	}
	if updated != nil {
		failStreak = updated.HealthCheckFailStreak
	}

	accountID := account.ID
	created, createErr := s.resultRepo.Create(ctx, &AccountBatchTestResult{
		TaskID:            taskID,
		AccountID:         &accountID,
		AccountName:       account.Name,
		Platform:          account.Platform,
		AccountType:       account.Type,
		Status:            status,
		ResponseText:      result.ResponseText,
		ErrorMessage:      errMsg,
		LatencyMs:         result.LatencyMs,
		FailStreak:        failStreak,
		TriggeredDisabled: shouldDisable,
		StartedAt:         result.StartedAt,
		FinishedAt:        result.FinishedAt,
	})
	if createErr != nil {
		return fmt.Errorf("create batch test result: %w", createErr)
	}

	if err := s.taskRepo.IncrementProgress(ctx, taskID, created.Status, created.TriggeredDisabled); err != nil {
		return fmt.Errorf("increment task progress: %w", err)
	}
	return nil
}

func classifyAccountHealthCheckOutcome(result *ScheduledTestResult, status string) accountHealthCheckOutcome {
	if result == nil {
		return accountHealthCheckOutcomeFailed
	}
	text := strings.ToLower(strings.Join([]string{
		result.ErrorMessage,
		result.ResponseText,
	}, "\n"))
	if isHealthCheckUnauthorizedText(text) {
		return accountHealthCheckOutcomeUnauthorized
	}
	if status == AccountBatchTestResultSuccess {
		return accountHealthCheckOutcomeNormal
	}
	if isHealthCheckRateLimitText(text) {
		return accountHealthCheckOutcomeRateLimited
	}
	return accountHealthCheckOutcomeFailed
}

func isHealthCheckUnauthorizedText(text string) bool {
	text = strings.ToLower(text)
	return strings.Contains(text, "401") ||
		strings.Contains(text, "unauthorized") ||
		strings.Contains(text, "authentication failed") ||
		strings.Contains(text, "invalid_api_key") ||
		strings.Contains(text, "invalid api key")
}

func isHealthCheckRateLimitText(text string) bool {
	text = strings.ToLower(text)
	return strings.Contains(text, "429") ||
		strings.Contains(text, "rate limit") ||
		strings.Contains(text, "rate_limit") ||
		strings.Contains(text, "rate-limited") ||
		strings.Contains(text, "rate limited") ||
		strings.Contains(text, "too many requests")
}

func normalizeBatchListLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	if limit > 200 {
		return 200
	}
	return limit
}

type AccountHealthCheckRunnerService struct {
	healthSvc *AccountHealthCheckService
	cfg       *config.Config
	cron      *cron.Cron
	startOnce sync.Once
	stopOnce  sync.Once
}

func NewAccountHealthCheckRunnerService(healthSvc *AccountHealthCheckService, cfg *config.Config) *AccountHealthCheckRunnerService {
	return &AccountHealthCheckRunnerService{healthSvc: healthSvc, cfg: cfg}
}

func (s *AccountHealthCheckRunnerService) Start() {
	if s == nil || s.healthSvc == nil {
		return
	}
	s.startOnce.Do(func() {
		loc := time.Local
		if s.cfg != nil {
			if parsed, err := time.LoadLocation(s.cfg.Timezone); err == nil && parsed != nil {
				loc = parsed
			}
		}
		c := cron.New(cron.WithLocation(loc))
		_, err := c.AddFunc("17 */3 * * *", func() { s.runScheduled() })
		if err != nil {
			logger.LegacyPrintf("service.account_health_check", "[AccountHealthCheck] runner not started: %v", err)
			return
		}
		s.cron = c
		s.cron.Start()
		logger.LegacyPrintf("service.account_health_check", "[AccountHealthCheck] runner started (every 3h)")
	})
}

func (s *AccountHealthCheckRunnerService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		if s.cron != nil {
			ctx := s.cron.Stop()
			select {
			case <-ctx.Done():
			case <-time.After(3 * time.Second):
				logger.LegacyPrintf("service.account_health_check", "[AccountHealthCheck] runner stop timed out")
			}
		}
	})
}

func (s *AccountHealthCheckRunnerService) runScheduled() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	task, err := s.healthSvc.RunScheduled(ctx)
	if err != nil {
		logger.LegacyPrintf("service.account_health_check", "[AccountHealthCheck] scheduled run failed: %v", err)
		return
	}
	if task != nil {
		logger.LegacyPrintf("service.account_health_check", "[AccountHealthCheck] scheduled task created: task=%d total=%d", task.ID, task.TotalCount)
	}
}
