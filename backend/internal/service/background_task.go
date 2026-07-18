package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type BackgroundTaskStatus string

const (
	BackgroundTaskStatusPending       BackgroundTaskStatus = "pending"
	BackgroundTaskStatusRunning       BackgroundTaskStatus = "running"
	BackgroundTaskStatusRetryWait     BackgroundTaskStatus = "retry_wait"
	BackgroundTaskStatusSucceeded     BackgroundTaskStatus = "succeeded"
	BackgroundTaskStatusSkipped       BackgroundTaskStatus = "skipped"
	BackgroundTaskStatusFailed        BackgroundTaskStatus = "failed"
	BackgroundTaskStatusCanceled      BackgroundTaskStatus = "canceled"
	BackgroundTaskStatusIndeterminate BackgroundTaskStatus = "indeterminate"
)

const (
	BackgroundTaskTypeOpenAIQuotaReset        = "openai_quota_reset"
	BackgroundTaskCreationRequestKeyMaxLength = 128
)

var (
	ErrBackgroundTaskNotFound       = errors.New("background task not found")
	ErrBackgroundTaskConflict       = errors.New("background task state conflict")
	ErrBackgroundTaskLeaseLost      = errors.New("background task lease lost")
	ErrBackgroundTaskCannotCancel   = errors.New("background task cannot be canceled")
	ErrBackgroundTaskCannotRetry    = errors.New("background task cannot be retried")
	ErrBackgroundTaskInvalidStatus  = errors.New("invalid background task status")
	ErrBackgroundTaskInvalidPayload = errors.New("invalid background task payload")
)

func (s BackgroundTaskStatus) Valid() bool {
	switch s {
	case BackgroundTaskStatusPending,
		BackgroundTaskStatusRunning,
		BackgroundTaskStatusRetryWait,
		BackgroundTaskStatusSucceeded,
		BackgroundTaskStatusSkipped,
		BackgroundTaskStatusFailed,
		BackgroundTaskStatusCanceled,
		BackgroundTaskStatusIndeterminate:
		return true
	default:
		return false
	}
}

func (s BackgroundTaskStatus) Active() bool {
	return s == BackgroundTaskStatusPending || s == BackgroundTaskStatusRunning || s == BackgroundTaskStatusRetryWait
}

type BackgroundTaskRun struct {
	ID                 int64                `json:"id"`
	TaskType           string               `json:"task_type"`
	ResourceType       string               `json:"resource_type"`
	ResourceID         string               `json:"resource_id"`
	Payload            json.RawMessage      `json:"-"`
	Display            json.RawMessage      `json:"display"`
	RunAt              time.Time            `json:"run_at"`
	Status             BackgroundTaskStatus `json:"status"`
	AttemptCount       int                  `json:"attempt_count"`
	DispatchCount      int                  `json:"dispatch_count"`
	MaxAttempts        int                  `json:"-"`
	DedupeKey          string               `json:"-"`
	IdempotencyKey     *string              `json:"-"`
	CreationRequestKey *string              `json:"-"`
	DedupeLocked       bool                 `json:"-"`
	ClaimOwner         *string              `json:"-"`
	ClaimVersion       int64                `json:"-"`
	LeaseUntil         *time.Time           `json:"-"`
	FirstDispatchAt    *time.Time           `json:"first_dispatch_at,omitempty"`
	LastDispatchAt     *time.Time           `json:"last_dispatch_at,omitempty"`
	ResultCode         *string              `json:"result_code,omitempty"`
	Result             json.RawMessage      `json:"result,omitempty"`
	LastErrorCode      *string              `json:"last_error_code,omitempty"`
	LastErrorMessage   *string              `json:"last_error_message,omitempty"`
	CreatedBy          int64                `json:"created_by"`
	CanceledBy         *int64               `json:"canceled_by,omitempty"`
	CanceledAt         *time.Time           `json:"canceled_at,omitempty"`
	StartedAt          *time.Time           `json:"started_at,omitempty"`
	FinishedAt         *time.Time           `json:"finished_at,omitempty"`
	CreatedAt          time.Time            `json:"created_at"`
	UpdatedAt          time.Time            `json:"updated_at"`
}

type CreateBackgroundTaskInput struct {
	TaskType       string
	ResourceType   string
	ResourceID     string
	Payload        json.RawMessage
	Display        json.RawMessage
	RunAt          time.Time
	DedupeKey      string
	IdempotencyKey *string
	// CreationRequestKey identifies the admin request that created this task.
	// It is distinct from IdempotencyKey, which identifies the upstream reset.
	CreationRequestKey string
	// GenerateIdempotencyKey asks the repository to create the immutable key
	// only after it has established that no dedupe owner already exists.
	GenerateIdempotencyKey bool
	MaxAttempts            int
	CreatedBy              int64
}

func (i *CreateBackgroundTaskInput) Validate() error {
	if i == nil {
		return ErrBackgroundTaskInvalidPayload
	}
	if i.TaskType == "" || i.ResourceType == "" || i.ResourceID == "" || i.DedupeKey == "" {
		return fmt.Errorf("%w: task type, resource and dedupe key are required", ErrBackgroundTaskInvalidPayload)
	}
	if i.RunAt.IsZero() {
		return fmt.Errorf("%w: run_at is required", ErrBackgroundTaskInvalidPayload)
	}
	if i.GenerateIdempotencyKey && i.IdempotencyKey != nil {
		return fmt.Errorf("%w: idempotency key must be supplied or generated, not both", ErrBackgroundTaskInvalidPayload)
	}
	if len(i.CreationRequestKey) > BackgroundTaskCreationRequestKeyMaxLength {
		return fmt.Errorf("%w: creation request key must not exceed 128 bytes", ErrBackgroundTaskInvalidPayload)
	}
	if i.MaxAttempts <= 0 {
		i.MaxAttempts = 5
	}
	if len(i.Payload) == 0 {
		i.Payload = json.RawMessage(`{}`)
	}
	if len(i.Display) == 0 {
		i.Display = json.RawMessage(`{}`)
	}
	if !json.Valid(i.Payload) || !json.Valid(i.Display) {
		return fmt.Errorf("%w: payload and display must be JSON", ErrBackgroundTaskInvalidPayload)
	}
	return nil
}

type BackgroundTaskListFilter struct {
	TaskType     string
	Status       BackgroundTaskStatus
	ResourceType string
	ResourceID   string
	Page         int
	PageSize     int
}

type BackgroundTaskListResult struct {
	Items    []*BackgroundTaskRun `json:"items"`
	Total    int64                `json:"total"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"page_size"`
}

type BackgroundTaskFinishInput struct {
	Status            BackgroundTaskStatus
	ResultCode        string
	Result            json.RawMessage
	ErrorCode         string
	ErrorMessage      string
	ReleaseDedupeLock bool
	FinishedAt        time.Time
}

type BackgroundTaskRepository interface {
	Create(ctx context.Context, input *CreateBackgroundTaskInput) (*BackgroundTaskRun, bool, error)
	GetByID(ctx context.Context, id int64) (*BackgroundTaskRun, error)
	GetByCreationRequestKey(ctx context.Context, key string) (*BackgroundTaskRun, error)
	List(ctx context.Context, filter BackgroundTaskListFilter) (*BackgroundTaskListResult, error)
	CountBacklog(ctx context.Context, now time.Time) (int64, error)
	ClaimDue(ctx context.Context, owner string, now, leaseUntil time.Time, limit int) ([]*BackgroundTaskRun, error)
	RenewLease(ctx context.Context, id int64, owner string, claimVersion int64, leaseUntil time.Time) error
	BeginDispatch(ctx context.Context, id int64, owner string, claimVersion int64, now time.Time) error
	ScheduleRetry(ctx context.Context, id int64, owner string, claimVersion int64, runAt time.Time, errorCode, errorMessage string) error
	Finish(ctx context.Context, id int64, owner string, claimVersion int64, input BackgroundTaskFinishInput) error
	Cancel(ctx context.Context, id, canceledBy int64, now time.Time) (*BackgroundTaskRun, error)
	RequeueIndeterminate(ctx context.Context, id int64, now time.Time) (*BackgroundTaskRun, error)
}
