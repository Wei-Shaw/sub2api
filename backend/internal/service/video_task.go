package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrVideoTaskRepositoryUnavailable = errors.New("durable video task repository unavailable")
	ErrVideoTaskBusy                  = errors.New("video task is leased or awaiting retry")
	ErrVideoTaskLeaseLost             = errors.New("video task lease lost")
	ErrVideoTaskConflict              = errors.New("video task owner or create snapshot conflicts")
)

const (
	VideoTaskPending   = "pending"
	VideoTaskReady     = "ready"
	VideoTaskCompleted = "completed"
	VideoTaskFailed    = "failed"
)

// VideoTask contains only task identity and billing metadata, never credentials,
// prompts, response headers, or signed download URLs. TrackingOnly records share
// the task list while retaining their provider's existing billing lifecycle.
type VideoTask struct {
	TaskID             string
	UserID             int64
	APIKeyID           int64
	GroupID            *int64
	AccountID          int64
	SubscriptionID     *int64
	Pending            GrokVideoPendingBilling
	CostSnapshot       *VideoTaskCostSnapshot
	Completion         *VideoTaskCompletion
	State              string
	LastError          string
	Attempts           int
	NextAttemptAt      time.Time
	LeaseToken         string
	RoutingFingerprint string
}

// VideoTaskCompletion freezes the first billable observation before charging,
// so retries can settle even after the upstream task or its URL expires.
type VideoTaskCompletion struct {
	Model                 string        `json:"model"`
	BillingModel          string        `json:"billing_model"`
	UpstreamModel         string        `json:"upstream_model,omitempty"`
	UpstreamResponseModel string        `json:"upstream_response_model,omitempty"`
	UpstreamEndpoint      string        `json:"upstream_endpoint,omitempty"`
	VideoResolution       string        `json:"video_resolution"`
	VideoDurationSeconds  int           `json:"video_duration_seconds"`
	Usage                 OpenAIUsage   `json:"usage"`
	Duration              time.Duration `json:"duration"`
}

// VideoTaskRepository is optional on UsageBillingRepository, avoiding a second
// DB dependency in the gateway constructor. All mutations after creation are
// fenced by a unique lease token. Leases and due times use the database clock.
type VideoTaskRepository interface {
	CreateVideoTask(context.Context, *VideoTask) error
	GetVideoTask(context.Context, string, int64, int64) (*VideoTask, error)
	ClaimDueVideoTask(context.Context, time.Duration) (*VideoTask, error)
	ClaimOwnedVideoTask(context.Context, string, int64, int64, time.Duration) (*VideoTask, error)
	SaveVideoTaskCompletion(context.Context, *VideoTask, *VideoTaskCompletion) error
	FinishVideoTaskAttempt(context.Context, *VideoTask, string, time.Duration, string) error
}

func (s *OpenAIGatewayService) videoTaskRepository() (VideoTaskRepository, error) {
	if s != nil {
		if repo, ok := s.usageBillingRepo.(VideoTaskRepository); ok && repo != nil {
			return repo, nil
		}
	}
	return nil, ErrVideoTaskRepositoryUnavailable
}

// PersistOpenAIVideoTask is a synchronous create-success callback. The forwarder
// must call it after obtaining the task ID but BEFORE writing success downstream.
// ResolveVideoTaskCost must run BEFORE sending the create request; pass that
// frozen quote here. A non-nil quote is required and no live repricing is allowed.
// A failed save must not acknowledge an untracked task to the client.
func (s *OpenAIGatewayService) PersistOpenAIVideoTask(ctx context.Context, taskID string, apiKey *APIKey, account *Account, subscription *UserSubscription, pending GrokVideoPendingBilling, quote *VideoTaskCostSnapshot) error {
	if account == nil || account.Platform != PlatformOpenAI {
		return errors.New("durable video task requires an owned OpenAI account task")
	}
	return s.PersistDurableVideoTask(ctx, taskID, apiKey, account, subscription, pending, quote)
}

// PersistDurableVideoTask enrolls only newly accepted, pre-quoted tasks.
// Historical tracking-only records must never be promoted to settlement.
func (s *OpenAIGatewayService) PersistDurableVideoTask(ctx context.Context, taskID string, apiKey *APIKey, account *Account, subscription *UserSubscription, pending GrokVideoPendingBilling, quote *VideoTaskCostSnapshot) error {
	if apiKey == nil || apiKey.ID <= 0 || apiKey.UserID <= 0 || account == nil || account.ID <= 0 || !durableVideoPlatform(account.Platform) || pending.TrackingOnly {
		return errors.New("durable video task requires an owned OpenAI account task")
	}
	if err := validateVideoTaskID(taskID); err != nil {
		return err
	}
	if apiKey.GroupID != nil && *apiKey.GroupID <= 0 {
		return errors.New("invalid video task group")
	}
	if subscription != nil && (subscription.ID <= 0 || subscription.UserID != apiKey.UserID || apiKey.GroupID == nil || subscription.GroupID != *apiKey.GroupID) {
		return errors.New("video task subscription does not match owner and group")
	}
	repo, err := s.videoTaskRepository()
	if err != nil {
		return err
	}
	if pending.CreatedAt == "" {
		pending.CreatedAt = GrokVideoPendingCreatedAtNow()
	}
	_, err = time.Parse(time.RFC3339Nano, pending.CreatedAt)
	if err != nil {
		return fmt.Errorf("invalid video create time: %w", err)
	}
	// This path must never invent the Grok 8s/480p defaults, even if the quote
	// calculator supports them for older callers. New tasks need accepted units.
	if pending.VideoDurationSeconds <= 0 || strings.TrimSpace(pending.VideoResolution) == "" {
		return errors.New("video task requires accepted duration and resolution")
	}
	if quote == nil {
		return errors.New("video task requires a pre-send cost snapshot")
	}
	if err := quote.validate(); err != nil {
		return err
	}
	subscriptionID := int64(0)
	if subscription != nil {
		subscriptionID = subscription.ID
	}
	if quote.Platform != account.Platform || quote.Command.UserID != apiKey.UserID || quote.Command.APIKeyID != apiKey.ID || quote.Command.AccountID != account.ID ||
		valueOrZero(quote.Log.GroupID) != valueOrZero(apiKey.GroupID) || valueOrZero(quote.Command.SubscriptionID) != subscriptionID ||
		quote.Log.VideoCount != 1 || *quote.Log.VideoDurationSeconds != pending.VideoDurationSeconds ||
		!strings.EqualFold(strings.TrimSpace(*quote.Log.VideoResolution), strings.TrimSpace(pending.VideoResolution)) {
		return errors.New("video task pre-send quote does not match accepted owner or units")
	}
	// Deep-copy before binding task identity. Never mutate the caller's frozen
	// quote, and never let a later mutation change a repository's saved snapshot.
	payload, err := json.Marshal(quote)
	if err != nil {
		return err
	}
	var frozen VideoTaskCostSnapshot
	if err := json.Unmarshal(payload, &frozen); err != nil {
		return err
	}
	frozen.Command.RequestID = ""
	frozen.Command.RequestFingerprint = ""
	frozen.Command.RequestPayloadHash = HashUsageRequestPayload([]byte(taskID))
	task := &VideoTask{TaskID: taskID, UserID: apiKey.UserID, APIKeyID: apiKey.ID, GroupID: apiKey.GroupID, AccountID: account.ID, Pending: pending, RoutingFingerprint: videoTaskRoutingFingerprint(account)}
	task.CostSnapshot = &frozen
	if subscription != nil {
		task.SubscriptionID = &subscription.ID
	}
	return repo.CreateVideoTask(ctx, task)
}

// LoadDurableVideoTask is the Redis-loss fallback for both pinned account and
// pending pricing metadata. Owner/key are part of the query, never post-filtered.
// A missing task returns nil, nil, allowing the caller to retain legacy behavior.
func (s *OpenAIGatewayService) LoadDurableVideoTask(ctx context.Context, taskID string, userID, apiKeyID int64) (*VideoTask, error) {
	repo, err := s.videoTaskRepository()
	if err != nil {
		return nil, err
	}
	if err := validateVideoTaskID(taskID); err != nil {
		return nil, err
	}
	if userID <= 0 || apiKeyID <= 0 {
		return nil, errors.New("video task owner required")
	}
	t, err := repo.GetVideoTask(ctx, taskID, userID, apiKeyID)
	if t != nil && t.Pending.TrackingOnly {
		// Lifecycle records do not opt legacy providers into worker settlement.
		return nil, err
	}
	return t, err
}

func validateVideoTaskID(id string) error {
	if id == "" || len(id) > 512 || strings.TrimSpace(id) != id || id == "." || id == ".." || strings.ContainsAny(id, "/\\?#%") {
		return errors.New("invalid video task id")
	}
	for _, r := range id {
		if r < 33 || r == 127 {
			return errors.New("invalid video task id")
		}
	}
	return nil
}

func videoTaskRoutingFingerprint(account *Account) string {
	protocol, _ := account.Extra["openai_video_protocol"].(string)
	baseURL := account.GetOpenAIBaseURL()
	if account.Platform == PlatformGrok {
		baseURL = account.GetGrokMediaBaseURL()
	}
	// Hash rather than store the base URL: provider base URLs may themselves
	// contain access material. Parent identity prevents swapping credential owner.
	body, _ := json.Marshal([]any{account.Platform, account.Type, baseURL, protocol, account.ParentAccountID})
	return fmt.Sprintf("%x", sha256.Sum256(body))
}

// ValidateDurableVideoTaskAccount must also guard the handler's subsequent
// status/content forward, including already-completed durable tasks.
func ValidateDurableVideoTaskAccount(task *VideoTask, account *Account) error {
	if task == nil || account == nil || account.ID != task.AccountID || !durableVideoPlatform(account.Platform) || task.RoutingFingerprint == "" || task.RoutingFingerprint != videoTaskRoutingFingerprint(account) {
		return errors.New("durable video task account route changed")
	}
	return nil
}

func durableVideoPlatform(platform string) bool {
	return platform == PlatformOpenAI || platform == PlatformGrok
}
