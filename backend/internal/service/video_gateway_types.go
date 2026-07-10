package service

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	VideoProviderMock     = "mock"
	VideoProviderSeedance = "seedance"
	VideoProviderKling    = "kling"

	VideoTaskTypeTextToVideo      = "text_to_video"
	VideoTaskTypeImageToVideo     = "image_to_video"
	VideoTaskTypeReferenceToVideo = "reference_to_video"

	VideoStatusQueued    = "queued"
	VideoStatusSubmitted = "submitted"
	VideoStatusRunning   = "running"
	VideoStatusSucceeded = "succeeded"
	VideoStatusFailed    = "failed"
	VideoStatusCancelled = "cancelled"

	VideoDispatchStatePending      = "pending"
	VideoDispatchStateDispatching  = "dispatching"
	VideoDispatchStateAccepted     = "accepted"
	VideoDispatchStateUnknown      = "unknown"
	VideoDispatchStateNotRequired  = "not_required"
	VideoSettlementStatusPending   = "pending"
	VideoSettlementStatusSettled   = "settled"
	VideoSettlementStatusReleased  = "released"
	VideoSettlementStatusNotNeeded = "not_required"
	VideoSideEffectStatusPending   = "pending"
	VideoSideEffectStatusNotNeeded = "not_required"

	VideoRouteStrategyLeastInflight = "least_inflight"

	BillingReservationReapActionReleased       = "released"
	BillingReservationReapActionReviewRequired = "review_required"
)

var (
	ErrVideoProviderNotFound     = infraerrors.NotFound("VIDEO_PROVIDER_NOT_FOUND", "video provider account not found")
	ErrVideoTaskNotFound         = infraerrors.NotFound("VIDEO_TASK_NOT_FOUND", "video task not found")
	ErrVideoProviderDisabled     = infraerrors.BadRequest("VIDEO_PROVIDER_DISABLED", "video provider account is disabled")
	ErrVideoInvalidProvider      = infraerrors.BadRequest("VIDEO_INVALID_PROVIDER", "provider must be one of mock/seedance/kling")
	ErrVideoInvalidTaskType      = infraerrors.BadRequest("VIDEO_INVALID_TASK_TYPE", "task_type must be one of text_to_video/image_to_video/reference_to_video")
	ErrVideoInvalidStatus        = infraerrors.BadRequest("VIDEO_INVALID_STATUS", "invalid video task status")
	ErrVideoTaskTerminalConflict = infraerrors.Conflict(
		"VIDEO_TASK_TERMINAL_CONFLICT",
		"video task already has a conflicting terminal status",
	)
	ErrVideoDispatchUnknown    = infraerrors.ServiceUnavailable("VIDEO_DISPATCH_UNKNOWN", "provider dispatch outcome requires manual reconciliation")
	ErrVideoMissingPrompt      = infraerrors.BadRequest("VIDEO_MISSING_PROMPT", "prompt is required")
	ErrVideoMissingProvider    = infraerrors.BadRequest("VIDEO_MISSING_PROVIDER", "provider_account_id is required")
	ErrVideoKeyDecryptFailed   = infraerrors.InternalServer("VIDEO_KEY_DECRYPT_FAILED", "video provider key decryption failed; please reconfigure the provider")
	ErrVideoMockUnavailable    = infraerrors.ServiceUnavailable("VIDEO_MOCK_PROVIDER_UNAVAILABLE", "mock video provider is unavailable")
	ErrVideoTrialLimitExceeded = infraerrors.Forbidden("VIDEO_TRIAL_LIMIT_EXCEEDED", "seedance trial limited to 1 call per day per user")
)

type VideoKeyEncryptor interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

type VideoProviderAccount struct {
	ID                  int64
	Provider            string
	DisplayName         string
	Enabled             bool
	EncryptedAPIKey     string
	PlainAPIKey         string `json:"-"` // decrypted transient credential; never serialize
	APIKeyConfigured    bool
	APIKeyDecryptFailed bool
	MaskedKey           string
	BaseURL             string
	DefaultModel        string
	RateLimitPerMinute  int
	Metadata            map[string]any
	KeyStatus           string
	HealthStatus        string
	DiagnosticType      string
	SuggestedAction     string
	Priority            int
	CurrentInflight     int64
	TodayTasks          int64
	TodayFailures       int64
	LastError           string
	LastTestAt          *time.Time
	RouteAvailable      bool
	RouteSkipReason     string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// String implements fmt.Stringer so that the transient plaintext API key is
// never rendered by %v/%+v/%s formatting (defensive bottom: PlainAPIKey is the
// decrypted upstream credential and must not land in logs, panics or dumps).
func (a VideoProviderAccount) String() string {
	return fmt.Sprintf("VideoProviderAccount{ID:%d, Provider:%q, DisplayName:%q, Enabled:%t, APIKeyConfigured:%t, MaskedKey:%q, PlainAPIKey:[REDACTED]}",
		a.ID, a.Provider, a.DisplayName, a.Enabled, a.APIKeyConfigured, a.MaskedKey)
}

// GoString implements fmt.GoStringer so the %#v verb (Go-syntax dump) also masks
// the plaintext key instead of printing every struct field verbatim.
func (a VideoProviderAccount) GoString() string {
	return a.String()
}

// LogValue implements slog.LogValuer so that structured logging of the account
// (e.g. slog.Any("account", acc)) never exposes the plaintext API key.
func (a VideoProviderAccount) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Int64("id", a.ID),
		slog.String("provider", a.Provider),
		slog.String("display_name", a.DisplayName),
		slog.Bool("enabled", a.Enabled),
		slog.Bool("api_key_configured", a.APIKeyConfigured),
		slog.String("masked_key", a.MaskedKey),
		slog.String("plain_api_key", "[REDACTED]"),
	)
}

// VideoDispatchTransportError preserves the provider-facing infra error while
// carrying transport evidence about whether a create request may have crossed
// the network boundary. Callers must only treat the outcome as ambiguous when
// RequestMayHaveBeenSent is true.
type VideoDispatchTransportError struct {
	Err                    error
	RequestMayHaveBeenSent bool
}

func (e *VideoDispatchTransportError) Error() string {
	return "video dispatch transport error"
}

func (e *VideoDispatchTransportError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type VideoTask struct {
	ID                  int64
	APIKeyID            *int64
	ProviderAccountID   int64
	ProviderAccountName string
	Provider            string
	Model               string
	TaskType            string
	Prompt              string
	NegativePrompt      string
	ReferenceImageURL   string
	ReferenceVideoURL   string
	Content             []VideoTaskContentItem
	HasVideoInput       bool
	AspectRatio         string
	Duration            int
	Resolution          string
	GenerateAudio       *bool
	Watermark           *bool
	CameraFixed         *bool
	ReturnLastFrame     *bool
	Status              string
	UpstreamTaskID      string
	ResultURL           string
	UsageTotalTokens    *int64
	ActualResolution    string
	ActualDuration      *int
	LastFrameURL        string
	ErrorMessage        string
	CostEstimate        float64
	Currency            string
	PricingSource       string
	PricingVersion      string
	PollCount           int
	LocalAssetPath      string
	LocalAssetSavedAt   *time.Time
	CreationKey         string
	CreationFingerprint string
	CreationReplayed    bool
	ReservationID       *int64
	Version             int64
	DispatchState       string
	SettlementStatus    string
	ArchiveStatus       string
	CaptureStatus       string
	BalanceChargedAt    *time.Time
	WorkerClaimedAt     *time.Time
	WorkerClaimedUntil  *time.Time
	CreatedBy           int64
	CreatedByEmail      string
	CreatedByName       string
	CreatedAt           time.Time
	UpdatedAt           time.Time
	CompletedAt         *time.Time
}

type VideoTaskContentItem struct {
	Type string `json:"type"`
	Role string `json:"role,omitempty"`
	URL  string `json:"url,omitempty"`
	Text string `json:"text,omitempty"`
}

type VideoTaskEvent struct {
	ID          int64
	VideoTaskID int64
	EventType   string
	Message     string
	Payload     map[string]any
	CreatedAt   time.Time
}

type VideoUsageSummary struct {
	Provider     string
	Model        string
	Status       string
	Count        int64
	CostEstimate float64
	Duration     int64
}

type VideoProviderStatus struct {
	Provider         string
	DisplayName      string
	Enabled          bool
	APIKeyConfigured bool
	MaskedKey        string
	DefaultModel     string
	KeyStatus        string
	HealthStatus     string
	DiagnosticType   string
	SuggestedAction  string
	RouteAvailable   bool
	RouteSkipReason  string
	Priority         int
	CurrentInflight  int64
	LastError        string
	LastTestAt       *time.Time
	UpdatedAt        time.Time
	TodayTasks       int64
	RunningTasks     int64
	FailedTasks      int64
}

type VideoHealthDiagnostic struct {
	Provider        string
	DisplayName     string
	RouteAccount    string
	KeyStatus       string
	LastTestAt      *time.Time
	ExceptionType   string
	ImpactTasks     int64
	RecentError     string
	SuggestedAction string
	Status          string
}

type VideoDashboard struct {
	TodayTasks        int64
	SuccessRate       float64
	FailedTasks       int64
	QueuedTasks       int64
	RunningTasks      int64
	ProviderStatus    []VideoProviderStatus
	HealthDiagnostics []VideoHealthDiagnostic
	RecentFailures    []*VideoTask
	RecentSuccesses   []*VideoTask
	UsageOverview     []VideoUsageSummary
}

type VideoProviderCreateParams struct {
	Provider           string
	DisplayName        string
	Enabled            bool
	APIKey             string
	BaseURL            string
	DefaultModel       string
	RateLimitPerMinute int
	Metadata           map[string]any
}

type VideoProviderUpdateParams struct {
	DisplayName        *string
	Enabled            *bool
	APIKey             *string
	BaseURL            *string
	DefaultModel       *string
	RateLimitPerMinute *int
	Metadata           *map[string]any
}

type VideoProviderTestResult struct {
	Provider         string         `json:"provider"`
	Configured       bool           `json:"configured"`
	Reachable        bool           `json:"reachable"`
	Message          string         `json:"message"`
	NormalizedStatus string         `json:"normalized_status"`
	PayloadPreview   map[string]any `json:"payload_preview,omitempty"`
}

type VideoTaskCreateParams struct {
	APIKeyID                               *int64
	ProviderAccountID                      int64
	TaskType                               string
	Model                                  string
	Prompt                                 string
	NegativePrompt                         string
	ReferenceImageURL                      string
	ReferenceVideoURL                      string
	Content                                []VideoTaskContentItem
	AspectRatio                            string
	Duration                               int
	Resolution                             string
	GenerateAudio                          *bool
	Watermark                              *bool
	CameraFixed                            *bool
	ReturnLastFrame                        *bool
	CreatedBy                              int64
	CreationKey                            string
	CreationFingerprint                    string
	EnforceRealProviderTrial               bool // JWT user paths: seedance requires daily trial + smoke gate
	RequireSeedanceProductionAuthorization bool // Admin production path: seedance requires provider metadata production_authorized=true
	SafeDemoOnly                           bool // Drama safe demo: route only to mock provider
}

// PricingSnapshot freezes the non-float pricing identity used for both creation
// reservations and terminal settlement. AmountOriginal retains the provider's
// billing currency; the separately returned Money is always USD.
type PricingSnapshot struct {
	AmountOriginal Money
	ExchangeRate   string
	PricingSource  string
	PricingVersion string
}

// ProjectVideoMoneyToLegacyFloat is the single compatibility boundary for
// legacy task/adapter fields that still expose float64. New financial inputs
// remain Money; callers must not use this projection for ledger arithmetic.
func ProjectVideoMoneyToLegacyFloat(amount Money) (float64, error) {
	value, _ := amount.Decimal().Float64()
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("video money cannot be represented by legacy float64")
	}
	return value, nil
}

func ApplyVideoPricingSnapshotToTask(task *VideoTask, snapshot PricingSnapshot) error {
	if task == nil {
		return fmt.Errorf("video task is required")
	}
	if len(snapshot.AmountOriginal.Currency()) != 3 {
		return fmt.Errorf("video pricing snapshot original currency is required")
	}
	legacyCost, err := ProjectVideoMoneyToLegacyFloat(snapshot.AmountOriginal)
	if err != nil {
		return err
	}
	task.CostEstimate = legacyCost
	task.Currency = string(snapshot.AmountOriginal.Currency())
	task.PricingSource = snapshot.PricingSource
	task.PricingVersion = snapshot.PricingVersion
	return nil
}

// VideoTaskPricing is the narrow financial boundary shared by creation and
// finalization. It deliberately exposes Money rather than float64.
type VideoTaskPricing interface {
	EstimatePrice(ctx context.Context, task *VideoTask) (Money, PricingSnapshot, error)
	ActualPrice(ctx context.Context, task *VideoTask) (Money, PricingSnapshot, error)
}

type VideoTaskCreationInput struct {
	Task                 *VideoTask
	ReservedAmountUSD    Money
	PricingSnapshot      PricingSnapshot
	ReservationExpiresAt time.Time
	DailyTrialProvider   string
	DailyTrialDate       time.Time
}

type VideoTaskCreationResult struct {
	Task        *VideoTask
	Reservation *BillingReservation
	Replayed    bool
}

type VideoTaskCreationReplayInput struct {
	CreationKey         string
	CreationFingerprint string
	CreatedBy           int64
	APIKeyID            *int64
}

type VideoTaskCreationRepository interface {
	CreateWithReservation(ctx context.Context, input VideoTaskCreationInput) (*VideoTaskCreationResult, error)
}

type VideoTaskCreationReplayRepository interface {
	ReplayExisting(ctx context.Context, input VideoTaskCreationReplayInput) (*VideoTaskCreationResult, bool, error)
}

// VideoTaskFinalizationRepository owns the single transaction that persists a
// terminal task, its financial settlement and its durable side effects.
type VideoTaskFinalizationRepository interface {
	FinalizeVideoTask(ctx context.Context, input VideoTaskFinalizationInput) (VideoTaskFinalizationResult, error)
}

// VideoTaskPollUpdateResult is the authoritative task state observed by the
// repository after a version-guarded nonterminal poll attempt.
type VideoTaskPollUpdateResult struct {
	Applied            bool
	Status             string
	Version            int64
	ResultURL          string
	ErrorMessage       string
	CostEstimate       float64
	PollCount          int
	UsageTotalTokens   *int64
	ActualResolution   string
	ActualDuration     *int
	LastFrameURL       string
	SettlementStatus   string
	BalanceChargedAt   *time.Time
	WorkerClaimedAt    *time.Time
	WorkerClaimedUntil *time.Time
	ArchiveStatus      string
	CaptureStatus      string
	CompletedAt        *time.Time
}

// VideoTaskPollRepository owns the atomic nonterminal task update and its poll
// event so stale provider responses cannot overwrite a concurrently committed
// terminal state.
type VideoTaskPollRepository interface {
	UpdatePolledTaskCAS(ctx context.Context, expectedVersion int64, candidate *VideoTask, event *VideoTaskEvent) (VideoTaskPollUpdateResult, error)
}

// VideoDispatchRepository is deliberately narrower than VideoGatewayRepository:
// dispatch state transitions must be CAS-protected and append their audit event
// in the same repository-owned transaction.
type VideoDispatchRepository interface {
	MarkDispatchingCAS(ctx context.Context, task *VideoTask, event *VideoTaskEvent) (bool, error)
	MarkDispatchAcceptedCAS(ctx context.Context, task *VideoTask, event *VideoTaskEvent) (bool, error)
	MarkDispatchUnknownCAS(ctx context.Context, task *VideoTask, event *VideoTaskEvent) (bool, error)
	ListDispatchUnknownTasks(ctx context.Context, limit int) ([]*VideoTask, error)
}

type BillingReservationReapResult struct {
	ReservationID int64
	TaskID        int64
	Action        string
}

type BillingReservationReaperRepository interface {
	ReapExpiredVideoReservations(ctx context.Context, now time.Time, limit int) ([]BillingReservationReapResult, error)
}

type VideoTaskListParams struct {
	Page      int
	PageSize  int
	Status    string
	Provider  string
	CreatedBy int64
	IsAdmin   bool
}

type VideoGatewayRepository interface {
	VideoTaskCreationRepository
	VideoTaskCreationReplayRepository

	CreateProviderAccount(ctx context.Context, account *VideoProviderAccount) error
	GetProviderAccount(ctx context.Context, id int64) (*VideoProviderAccount, error)
	ListProviderAccounts(ctx context.Context) ([]*VideoProviderAccount, error)
	UpdateProviderAccount(ctx context.Context, account *VideoProviderAccount) error

	CreateTask(ctx context.Context, task *VideoTask) error
	CreateDailyTrialTask(ctx context.Context, task *VideoTask, provider string, createdBy int64, trialDate time.Time) (bool, error)
	GetTask(ctx context.Context, id int64) (*VideoTask, error)
	ListTasks(ctx context.Context, params VideoTaskListParams) ([]*VideoTask, int64, error)
	ListDramaTasks(ctx context.Context, params VideoTaskListParams, filters map[string]string) ([]*VideoTask, int64, error)
	ListRunnableTasks(ctx context.Context, limit int) ([]*VideoTask, error)
	ListUnchargedSucceededVideoTasks(ctx context.Context, limit int) ([]*VideoTask, error)
	ClaimTaskForSubmit(ctx context.Context, taskID int64) (bool, error)
	UpdateTask(ctx context.Context, task *VideoTask) error
	SetTaskLocalAsset(ctx context.Context, taskID int64, path string, savedAt time.Time) error
	ClearTaskLocalAsset(ctx context.Context, taskID int64) error
	ListExpiredLocalAssets(ctx context.Context, olderThan time.Time, limit int) ([]*VideoTask, error)

	AddTaskEvent(ctx context.Context, event *VideoTaskEvent) error
	ListTaskEvents(ctx context.Context, taskID int64, limit int) ([]*VideoTaskEvent, error)
	InsertUsageLog(ctx context.Context, task *VideoTask) error
	ClaimVideoBalanceCharge(ctx context.Context, taskID int64) (time.Time, bool, error)
	ClearVideoBalanceChargeIfClaimedAt(ctx context.Context, taskID int64, claimedAt time.Time) (bool, error)

	CountTasksSince(ctx context.Context, since time.Time) (map[string]int64, error)
	CountProviderTasksSince(ctx context.Context, since time.Time) (map[string]map[string]int64, error)
	ProviderAccountTaskStatsSince(ctx context.Context, since time.Time) (map[int64]VideoProviderRuntimeStats, error)
	ListRecentTasksByStatus(ctx context.Context, status string, limit int) ([]*VideoTask, error)
	UsageSummarySince(ctx context.Context, since time.Time) ([]VideoUsageSummary, error)
}

type VideoProviderRuntimeStats struct {
	TodayTasks      int64
	CurrentInflight int64
	TodayFailures   int64
	LastError       string
	LastErrorAt     *time.Time
}

func IsTerminalVideoStatus(status string) bool {
	switch status {
	case VideoStatusSucceeded, VideoStatusFailed, VideoStatusCancelled:
		return true
	default:
		return false
	}
}
