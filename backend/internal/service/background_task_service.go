package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
	"github.com/google/uuid"
)

const (
	backgroundTaskPollInterval  = 30 * time.Second
	backgroundTaskLeaseDuration = 2 * time.Minute
	backgroundTaskLeaseRenewal  = 30 * time.Second
	backgroundTaskConcurrency   = 4
	backgroundTaskRunnerJobName = "background_task_runner"
)

var openAIQuotaResetBackoffs = [...]time.Duration{
	15 * time.Second,
	30 * time.Second,
	60 * time.Second,
	120 * time.Second,
}

type BackgroundTaskPublic struct {
	ID               int64                `json:"id"`
	TaskType         string               `json:"task_type"`
	ResourceType     string               `json:"resource_type"`
	ResourceID       string               `json:"resource_id"`
	AccountID        *int64               `json:"account_id,omitempty"`
	AccountName      string               `json:"account_name,omitempty"`
	CreditExpiresAt  *time.Time           `json:"credit_expires_at,omitempty"`
	RunAt            time.Time            `json:"run_at"`
	Status           BackgroundTaskStatus `json:"status"`
	AttemptCount     int                  `json:"attempt_count"`
	DispatchCount    int                  `json:"dispatch_count"`
	FirstDispatchAt  *time.Time           `json:"first_dispatch_at,omitempty"`
	LastDispatchAt   *time.Time           `json:"last_dispatch_at,omitempty"`
	ResultCode       *string              `json:"result_code,omitempty"`
	Result           json.RawMessage      `json:"result,omitempty"`
	LastErrorCode    *string              `json:"last_error_code,omitempty"`
	LastErrorMessage *string              `json:"last_error_message,omitempty"`
	CanCancel        bool                 `json:"can_cancel"`
	CanRetry         bool                 `json:"can_retry"`
	CanceledAt       *time.Time           `json:"canceled_at,omitempty"`
	StartedAt        *time.Time           `json:"started_at,omitempty"`
	FinishedAt       *time.Time           `json:"finished_at,omitempty"`
	CreatedAt        time.Time            `json:"created_at"`
	UpdatedAt        time.Time            `json:"updated_at"`
}

type backgroundTaskDisplay struct {
	AccountID       int64  `json:"account_id"`
	AccountName     string `json:"account_name"`
	CreditExpiresAt string `json:"credit_expires_at"`
}

func PublicBackgroundTask(task *BackgroundTaskRun) *BackgroundTaskPublic {
	if task == nil {
		return nil
	}
	out := &BackgroundTaskPublic{
		ID:               task.ID,
		TaskType:         task.TaskType,
		ResourceType:     task.ResourceType,
		ResourceID:       task.ResourceID,
		RunAt:            task.RunAt,
		Status:           task.Status,
		AttemptCount:     task.AttemptCount,
		DispatchCount:    task.DispatchCount,
		FirstDispatchAt:  task.FirstDispatchAt,
		LastDispatchAt:   task.LastDispatchAt,
		ResultCode:       task.ResultCode,
		Result:           task.Result,
		LastErrorCode:    task.LastErrorCode,
		LastErrorMessage: task.LastErrorMessage,
		CanCancel: (task.Status == BackgroundTaskStatusPending ||
			task.Status == BackgroundTaskStatusRetryWait ||
			task.Status == BackgroundTaskStatusRunning) && task.FirstDispatchAt == nil,
		CanRetry:   task.Status == BackgroundTaskStatusIndeterminate,
		CanceledAt: task.CanceledAt,
		StartedAt:  task.StartedAt,
		FinishedAt: task.FinishedAt,
		CreatedAt:  task.CreatedAt,
		UpdatedAt:  task.UpdatedAt,
	}
	var display backgroundTaskDisplay
	if len(task.Display) > 0 && json.Unmarshal(task.Display, &display) == nil {
		if display.AccountID > 0 {
			value := display.AccountID
			out.AccountID = &value
		}
		out.AccountName = display.AccountName
		if expiresAt, err := time.Parse(time.RFC3339, display.CreditExpiresAt); err == nil {
			out.CreditExpiresAt = &expiresAt
		}
	}
	return out
}

type BackgroundTaskHandlerResult struct {
	Status            BackgroundTaskStatus
	RetryAt           *time.Time
	ResultCode        string
	Result            json.RawMessage
	ErrorCode         string
	ErrorMessage      string
	ReleaseDedupeLock bool
}

type BackgroundTaskExecution struct {
	repo  BackgroundTaskRepository
	task  *BackgroundTaskRun
	owner string
	mu    sync.Mutex
	begun bool
}

func (e *BackgroundTaskExecution) BeginDispatch(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.begun {
		return errors.New("background task dispatch already started")
	}
	now := time.Now()
	if err := e.repo.BeginDispatch(ctx, e.task.ID, e.owner, e.task.ClaimVersion, now); err != nil {
		return err
	}
	e.begun = true
	e.task.DispatchCount++
	e.task.LastDispatchAt = &now
	if e.task.FirstDispatchAt == nil {
		e.task.FirstDispatchAt = &now
	}
	e.task.DedupeLocked = true
	return nil
}

type BackgroundTaskHandler interface {
	TaskType() string
	Execute(ctx context.Context, execution *BackgroundTaskExecution) BackgroundTaskHandlerResult
}

type BackgroundTaskService struct {
	repo         BackgroundTaskRepository
	accountRepo  AccountRepository
	quotaService openAIQuotaTaskAPI
	opsRepo      OpsRepository
	handlers     map[string]BackgroundTaskHandler
	owner        string
	sem          chan struct{}

	startOnce sync.Once
	stopOnce  sync.Once
	stop      context.CancelFunc
	loopWG    sync.WaitGroup
	jobWG     sync.WaitGroup
}

func NewBackgroundTaskService(repo BackgroundTaskRepository, accountRepo AccountRepository, quotaService *OpenAIQuotaService, opsRepo OpsRepository) *BackgroundTaskService {
	hostname, _ := os.Hostname()
	service := &BackgroundTaskService{
		repo:         repo,
		accountRepo:  accountRepo,
		quotaService: quotaService,
		opsRepo:      opsRepo,
		handlers:     make(map[string]BackgroundTaskHandler),
		owner:        fmt.Sprintf("%s:%s", hostname, uuid.NewString()),
		sem:          make(chan struct{}, backgroundTaskConcurrency),
	}
	service.RegisterHandler(&openAIQuotaResetTaskHandler{quotaService: quotaService})
	return service
}

func (s *BackgroundTaskService) RegisterHandler(handler BackgroundTaskHandler) {
	if s == nil || handler == nil || strings.TrimSpace(handler.TaskType()) == "" {
		return
	}
	s.handlers[handler.TaskType()] = handler
}

func (s *BackgroundTaskService) Start() {
	if s == nil || s.repo == nil {
		return
	}
	s.startOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		s.stop = cancel
		s.loopWG.Add(1)
		go s.run(ctx)
	})
}

func (s *BackgroundTaskService) Stop(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.stopOnce.Do(func() {
		if s.stop != nil {
			s.stop()
		}
	})
	if ctx == nil {
		ctx = context.Background()
	}
	done := make(chan struct{})
	go func() {
		// The scan loop must exit before waiting on jobs because it owns jobWG.Add.
		s.loopWG.Wait()
		s.jobWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *BackgroundTaskService) run(ctx context.Context) {
	defer s.loopWG.Done()
	ticker := time.NewTicker(backgroundTaskPollInterval)
	defer ticker.Stop()
	for {
		s.scanOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *BackgroundTaskService) scanOnce(ctx context.Context) {
	started := time.Now()
	available := cap(s.sem) - len(s.sem)
	if available <= 0 {
		s.recordRunnerHeartbeat(started, 0, nil)
		return
	}
	tasks, err := s.repo.ClaimDue(ctx, s.owner, started, started.Add(backgroundTaskLeaseDuration), available)
	if err != nil {
		s.recordRunnerHeartbeat(started, 0, err)
		return
	}
	for _, task := range tasks {
		if task == nil {
			continue
		}
		s.sem <- struct{}{}
		s.jobWG.Add(1)
		go func(task *BackgroundTaskRun) {
			defer func() {
				<-s.sem
				s.jobWG.Done()
			}()
			if processErr := s.processTask(ctx, task); processErr != nil && !errors.Is(processErr, ErrBackgroundTaskLeaseLost) {
				slog.Error("background_task_failed", "task_id", task.ID, "task_type", task.TaskType, "error", processErr)
				s.recordRunnerHeartbeat(time.Now(), 0, processErr)
			}
		}(task)
	}
	s.recordRunnerHeartbeat(started, len(tasks), nil)
}

func (s *BackgroundTaskService) processTask(ctx context.Context, task *BackgroundTaskRun) error {
	handler := s.handlers[task.TaskType]
	if handler == nil {
		status := BackgroundTaskStatusFailed
		if task.FirstDispatchAt != nil {
			status = BackgroundTaskStatusIndeterminate
		}
		return s.repo.Finish(context.Background(), task.ID, s.owner, task.ClaimVersion, BackgroundTaskFinishInput{
			Status: status, ErrorCode: "handler_not_found",
			ErrorMessage: "no handler registered for task type", ReleaseDedupeLock: task.FirstDispatchAt == nil,
		})
	}

	renewStop := make(chan struct{})
	renewDone := make(chan struct{})
	go s.renewLease(task, renewStop, renewDone)
	result := executeBackgroundTaskHandler(handler, ctx, &BackgroundTaskExecution{repo: s.repo, task: task, owner: s.owner})
	close(renewStop)
	<-renewDone

	if result.RetryAt != nil {
		return s.repo.ScheduleRetry(context.Background(), task.ID, s.owner, task.ClaimVersion, *result.RetryAt, result.ErrorCode, result.ErrorMessage)
	}
	return s.repo.Finish(context.Background(), task.ID, s.owner, task.ClaimVersion, BackgroundTaskFinishInput{
		Status: result.Status, ResultCode: result.ResultCode, Result: result.Result,
		ErrorCode: result.ErrorCode, ErrorMessage: result.ErrorMessage,
		ReleaseDedupeLock: result.ReleaseDedupeLock,
	})
}

func executeBackgroundTaskHandler(handler BackgroundTaskHandler, ctx context.Context, execution *BackgroundTaskExecution) (result BackgroundTaskHandlerResult) {
	defer func() {
		if recovered := recover(); recovered != nil {
			status := BackgroundTaskStatusFailed
			if execution.task.FirstDispatchAt != nil {
				status = BackgroundTaskStatusIndeterminate
			}
			result = BackgroundTaskHandlerResult{
				Status: status, ErrorCode: "handler_panic",
				ErrorMessage:      "background task handler panicked",
				ReleaseDedupeLock: execution.task.FirstDispatchAt == nil,
			}
		}
	}()
	return handler.Execute(ctx, execution)
}

func (s *BackgroundTaskService) renewLease(task *BackgroundTaskRun, stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(backgroundTaskLeaseRenewal)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case now := <-ticker.C:
			if err := s.repo.RenewLease(context.Background(), task.ID, s.owner, task.ClaimVersion, now.Add(backgroundTaskLeaseDuration)); err != nil {
				return
			}
		}
	}
}

func (s *BackgroundTaskService) recordRunnerHeartbeat(started time.Time, claimed int, runErr error) {
	if s.opsRepo == nil {
		return
	}
	now := time.Now()
	duration := now.Sub(started).Milliseconds()
	input := &OpsUpsertJobHeartbeatInput{JobName: backgroundTaskRunnerJobName, LastRunAt: &now, LastDurationMs: &duration}
	if runErr != nil {
		message := runErr.Error()
		input.LastErrorAt = &now
		input.LastError = &message
	} else {
		backlog, err := s.repo.CountBacklog(context.Background(), now)
		if err != nil {
			message := err.Error()
			input.LastErrorAt = &now
			input.LastError = &message
		} else {
			input.LastSuccessAt = &now
			result := fmt.Sprintf("claimed=%d backlog=%d active=%d", claimed, backlog, len(s.sem))
			input.LastResult = &result
		}
	}
	_ = s.opsRepo.UpsertJobHeartbeat(context.Background(), input)
}

func (s *BackgroundTaskService) CreateOpenAIQuotaReset(ctx context.Context, accountID int64, expectedExpiresAt time.Time, leadTimeMinutes int, creationRequestKey string, createdBy int64) (*BackgroundTaskRun, bool, error) {
	if s == nil || s.repo == nil {
		return nil, false, infraerrors.New(500, "BACKGROUND_TASK_NOT_CONFIGURED", "background task service is not configured")
	}
	creationRequestKey = strings.TrimSpace(creationRequestKey)
	if creationRequestKey == "" {
		return nil, false, infraerrors.BadRequest("IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key header is required")
	}
	if len(creationRequestKey) > BackgroundTaskCreationRequestKeyMaxLength {
		return nil, false, infraerrors.BadRequest("IDEMPOTENCY_KEY_TOO_LONG", "Idempotency-Key header must not exceed 128 bytes")
	}
	resourceID := strconv.FormatInt(accountID, 10)
	existing, err := s.repo.GetByCreationRequestKey(ctx, creationRequestKey)
	if err == nil {
		if err := validateOpenAIQuotaResetCreationReplay(existing, resourceID); err != nil {
			return nil, false, err
		}
		return existing, false, nil
	}
	if !errors.Is(err, ErrBackgroundTaskNotFound) {
		return nil, false, err
	}
	if s.quotaService == nil || s.accountRepo == nil {
		return nil, false, infraerrors.New(500, "BACKGROUND_TASK_NOT_CONFIGURED", "background task service is not configured")
	}
	if leadTimeMinutes != 10 && leadTimeMinutes != 30 && leadTimeMinutes != 60 {
		return nil, false, infraerrors.BadRequest("INVALID_LEAD_TIME", "lead_time_minutes must be one of 10, 30, or 60")
	}
	candidates, err := s.quotaService.ListResetCredits(ctx, accountID)
	if err != nil {
		return nil, false, err
	}
	now := time.Now()
	var nearest *OpenAIQuotaResetCreditCandidate
	for i := range candidates {
		if candidates[i].ExpiresAt.After(now) && (nearest == nil ||
			candidates[i].ExpiresAt.Before(nearest.ExpiresAt) ||
			(candidates[i].ExpiresAt.Equal(nearest.ExpiresAt) && candidates[i].ID < nearest.ID)) {
			nearest = &candidates[i]
		}
	}
	if nearest == nil {
		return nil, false, infraerrors.Conflict("OPENAI_QUOTA_NO_SCHEDULABLE_CREDIT", "no unexpired reset credit with details is available")
	}
	if !nearest.ExpiresAt.Equal(expectedExpiresAt) {
		return nil, false, infraerrors.Conflict("OPENAI_QUOTA_CREDIT_CHANGED", "reset credit changed; refresh quota details and try again")
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil || account == nil {
		return nil, false, infraerrors.NotFound("OPENAI_QUOTA_ACCOUNT_NOT_FOUND", "account not found")
	}

	payload, err := json.Marshal(openAIQuotaResetTaskPayload{
		AccountID: accountID, CreditID: nearest.ID, CreditExpiresAt: nearest.ExpiresAt.Format(time.RFC3339),
	})
	if err != nil {
		return nil, false, err
	}
	display, err := json.Marshal(backgroundTaskDisplay{
		AccountID: accountID, AccountName: account.Name, CreditExpiresAt: nearest.ExpiresAt.Format(time.RFC3339),
	})
	if err != nil {
		return nil, false, err
	}
	runAt := nearest.ExpiresAt.Add(-time.Duration(leadTimeMinutes) * time.Minute)
	if runAt.Before(now) {
		runAt = now
	}
	task, created, err := s.repo.Create(ctx, &CreateBackgroundTaskInput{
		TaskType: BackgroundTaskTypeOpenAIQuotaReset, ResourceType: "openai_account",
		ResourceID: resourceID, Payload: payload, Display: display,
		RunAt: runAt, DedupeKey: fmt.Sprintf("account:%d:credit:%s", accountID, nearest.ID),
		GenerateIdempotencyKey: true, CreationRequestKey: creationRequestKey,
		MaxAttempts: 5, CreatedBy: createdBy,
	})
	if err != nil {
		return nil, false, err
	}
	if err := validateOpenAIQuotaResetCreationReplay(task, resourceID); err != nil {
		return nil, false, err
	}
	return task, created, nil
}

func validateOpenAIQuotaResetCreationReplay(task *BackgroundTaskRun, resourceID string) error {
	if task == nil {
		return infraerrors.New(500, "BACKGROUND_TASK_INVALID_CREATION_REPLAY", "background task creation replay returned no task")
	}
	if task.TaskType != BackgroundTaskTypeOpenAIQuotaReset || task.ResourceType != "openai_account" || task.ResourceID != resourceID {
		return infraerrors.Conflict("BACKGROUND_TASK_CREATION_KEY_CONFLICT", "Idempotency-Key was already used for a different background task")
	}
	return nil
}

func (s *BackgroundTaskService) Get(ctx context.Context, id int64) (*BackgroundTaskRun, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *BackgroundTaskService) List(ctx context.Context, filter BackgroundTaskListFilter) (*BackgroundTaskListResult, error) {
	return s.repo.List(ctx, filter)
}

func (s *BackgroundTaskService) Cancel(ctx context.Context, id, canceledBy int64) (*BackgroundTaskRun, error) {
	return s.repo.Cancel(ctx, id, canceledBy, time.Now())
}

func (s *BackgroundTaskService) RetryIndeterminate(ctx context.Context, id int64) (*BackgroundTaskRun, error) {
	return s.repo.RequeueIndeterminate(ctx, id, time.Now())
}

type openAIQuotaResetTaskPayload struct {
	AccountID       int64  `json:"account_id"`
	CreditID        string `json:"credit_id"`
	CreditExpiresAt string `json:"credit_expires_at"`
}

type openAIQuotaResetExecutor interface {
	ResetCreditByID(ctx context.Context, accountID int64, redeemRequestID, creditID string) (*OpenAIQuotaResetResult, error)
}

type openAIQuotaTaskAPI interface {
	openAIQuotaResetExecutor
	ListResetCredits(ctx context.Context, accountID int64) ([]OpenAIQuotaResetCreditCandidate, error)
}

type openAIQuotaResetTaskHandler struct {
	quotaService openAIQuotaResetExecutor
}

func (h *openAIQuotaResetTaskHandler) TaskType() string {
	return BackgroundTaskTypeOpenAIQuotaReset
}

func (h *openAIQuotaResetTaskHandler) Execute(ctx context.Context, execution *BackgroundTaskExecution) BackgroundTaskHandlerResult {
	task := execution.task
	if h == nil || h.quotaService == nil || task == nil || task.IdempotencyKey == nil || strings.TrimSpace(*task.IdempotencyKey) == "" {
		return backgroundTaskFailure("invalid_task_payload", "quota reset task payload is incomplete", task == nil || task.FirstDispatchAt == nil)
	}
	var payload openAIQuotaResetTaskPayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil || payload.AccountID <= 0 || strings.TrimSpace(payload.CreditID) == "" {
		return backgroundTaskFailure("invalid_task_payload", "quota reset task payload is invalid", task.FirstDispatchAt == nil)
	}
	expectedResourceID := strconv.FormatInt(payload.AccountID, 10)
	expectedDedupeKey := fmt.Sprintf("account:%d:credit:%s", payload.AccountID, payload.CreditID)
	if task.ResourceType != "openai_account" || task.ResourceID != expectedResourceID || task.DedupeKey != expectedDedupeKey {
		return backgroundTaskFailure("invalid_task_binding", "quota reset task resource binding is invalid", task.FirstDispatchAt == nil)
	}
	expiresAt, err := time.Parse(time.RFC3339, payload.CreditExpiresAt)
	if err != nil {
		return backgroundTaskFailure("invalid_credit_expiry", "quota reset credit expiry is invalid", task.FirstDispatchAt == nil)
	}
	if task.MaxAttempts <= 0 {
		return backgroundTaskFailure("invalid_retry_budget", "quota reset task retry budget is invalid", task.FirstDispatchAt == nil)
	}
	if task.DispatchCount >= task.MaxAttempts {
		if task.FirstDispatchAt != nil || task.DispatchCount > 0 {
			return backgroundTaskIndeterminate("retry_budget_exhausted", "quota reset retry budget was exhausted before a definitive response")
		}
		return backgroundTaskFailure("retry_budget_exhausted", "quota reset task has no dispatch budget", true)
	}
	now := time.Now()
	manualRecoveryAfterExpiry := task.FirstDispatchAt != nil && task.RunAt.After(expiresAt)
	if !now.Before(expiresAt) && !manualRecoveryAfterExpiry {
		if task.FirstDispatchAt != nil {
			return backgroundTaskIndeterminate("credit_expired_after_dispatch", "credit expired before an ambiguous reset could be confirmed")
		}
		return BackgroundTaskHandlerResult{
			Status: BackgroundTaskStatusSkipped, ResultCode: "credit_expired",
			ReleaseDedupeLock: true,
		}
	}
	hadPriorDispatch := task.DispatchCount > 0
	if err := execution.BeginDispatch(ctx); err != nil {
		retryAt := time.Now()
		return BackgroundTaskHandlerResult{
			RetryAt: &retryAt, ErrorCode: "begin_dispatch_failed",
			ErrorMessage: "quota reset dispatch did not start; the task will be reclaimed",
		}
	}
	result, resetErr := h.quotaService.ResetCreditByID(ctx, payload.AccountID, *task.IdempotencyKey, payload.CreditID)
	if resetErr != nil {
		if ctx.Err() != nil {
			return retryOrIndeterminate(task, expiresAt, "worker_shutdown", "quota reset request was interrupted by worker shutdown")
		}
		var attemptErr *OpenAIQuotaResetAttemptError
		if errors.As(resetErr, &attemptErr) {
			safeMessage := safeOpenAIQuotaResetTaskError(resetErr, attemptErr)
			if attemptErr.Retryable {
				return retryOrIndeterminate(task, expiresAt, string(attemptErr.Kind), safeMessage)
			}
			if hadPriorDispatch {
				return backgroundTaskIndeterminate(string(attemptErr.Kind), safeMessage)
			}
			if attemptErr.Kind == OpenAIQuotaResetErrorRequest {
				return backgroundTaskIndeterminate(string(attemptErr.Kind), safeMessage)
			}
			if !attemptErr.DefinitiveNoConsumption {
				return backgroundTaskIndeterminate(string(attemptErr.Kind), safeMessage)
			}
			return backgroundTaskFailure("quota_reset_failed", safeMessage, true)
		}
		if hadPriorDispatch {
			return backgroundTaskIndeterminate("quota_reset_failed", safeOpenAIQuotaResetTaskError(resetErr, nil))
		}
		return backgroundTaskFailure("quota_reset_failed", safeOpenAIQuotaResetTaskError(resetErr, nil), true)
	}
	if result == nil {
		return retryOrIndeterminate(task, expiresAt, "empty_response", "upstream returned an empty reset response")
	}
	resultJSON, _ := json.Marshal(map[string]any{"code": result.Code, "windows_reset": result.WindowsReset})
	code := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(result.Code), "-", "_"))
	switch code {
	case "reset", "already_redeemed", "alreadyredeemed":
		return BackgroundTaskHandlerResult{Status: BackgroundTaskStatusSucceeded, ResultCode: code, Result: resultJSON}
	case "nothing_to_reset", "nothingtoreset", "no_credit", "nocredit":
		if task.DispatchCount > 1 {
			return backgroundTaskIndeterminate(code, "upstream returned a no-op after an earlier ambiguous dispatch")
		}
		return BackgroundTaskHandlerResult{
			Status: BackgroundTaskStatusSkipped, ResultCode: code, Result: resultJSON,
			ReleaseDedupeLock: true,
		}
	default:
		return retryOrIndeterminate(task, expiresAt, "unknown_response", "upstream returned an unknown reset outcome")
	}
}

func safeOpenAIQuotaResetTaskError(err error, attemptErr *OpenAIQuotaResetAttemptError) string {
	if attemptErr != nil {
		switch attemptErr.Kind {
		case OpenAIQuotaResetErrorRequest:
			return "upstream reset request failed before a definitive response"
		case OpenAIQuotaResetErrorAuth:
			return "upstream reset authentication failed"
		case OpenAIQuotaResetErrorUpstream:
			if attemptErr.UpstreamStatus > 0 {
				return fmt.Sprintf("upstream reset returned HTTP %d", attemptErr.UpstreamStatus)
			}
			return "upstream reset returned an error response"
		}
	}
	if err == nil {
		return "quota reset failed"
	}
	return logredact.RedactText(err.Error(), "authorization", "credit_id", "redeem_request_id", "proxy_url")
}

func retryOrIndeterminate(task *BackgroundTaskRun, expiresAt time.Time, errorCode, errorMessage string) BackgroundTaskHandlerResult {
	if task.DispatchCount < task.MaxAttempts {
		index := task.DispatchCount - 1
		if index >= 0 && index < len(openAIQuotaResetBackoffs) {
			retryAt := time.Now().Add(openAIQuotaResetBackoffs[index])
			if retryAt.Before(expiresAt) {
				return BackgroundTaskHandlerResult{RetryAt: &retryAt, ErrorCode: errorCode, ErrorMessage: errorMessage}
			}
		}
	}
	return backgroundTaskIndeterminate(errorCode, errorMessage)
}

func backgroundTaskFailure(code, message string, releaseDedupe bool) BackgroundTaskHandlerResult {
	return BackgroundTaskHandlerResult{
		Status: BackgroundTaskStatusFailed, ErrorCode: code, ErrorMessage: message,
		ReleaseDedupeLock: releaseDedupe,
	}
}

func backgroundTaskIndeterminate(code, message string) BackgroundTaskHandlerResult {
	return BackgroundTaskHandlerResult{
		Status: BackgroundTaskStatusIndeterminate, ErrorCode: code, ErrorMessage: message,
	}
}
