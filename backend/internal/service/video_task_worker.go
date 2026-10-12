package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	videoTaskPollInterval   = 15 * time.Second
	videoTaskAttemptTimeout = 45 * time.Second
	videoTaskLeaseDuration  = 2 * time.Minute
)

type videoTaskProcessor struct {
	repo VideoTaskRepository
	poll func(context.Context, *VideoTask) (*VideoTaskCompletion, string, error)
	bill func(context.Context, *VideoTask) error
}

// StartVideoTaskWorker is idempotent and scans durable records immediately on
// startup. A nil/legacy repository opts out; it never discovers or bills tasks
// from Redis, account enumeration, or upstream task listings.
func (s *OpenAIGatewayService) StartVideoTaskWorker(apiKeyService *APIKeyService) {
	if s == nil || apiKeyService == nil {
		return
	}
	s.videoPreviewCache.start()
	repo, err := s.videoTaskRepository()
	if err != nil {
		return
	}
	s.videoTaskMu.Lock()
	defer s.videoTaskMu.Unlock()
	if s.videoTaskCancel != nil || s.videoTaskStopped {
		return
	}
	p := &videoTaskProcessor{repo: repo, poll: s.pollDurableVideoTask, bill: func(ctx context.Context, t *VideoTask) error { return s.billDurableVideoTask(ctx, t, apiKeyService) }}
	var monitor *videoTaskMonitor
	if monitorRepo, ok := s.usageBillingRepo.(VideoTaskMonitorRepository); ok {
		monitor = &videoTaskMonitor{repo: monitorRepo, poll: s.pollVideoTaskMonitor}
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.videoTaskProcessor = p
	s.videoTaskCancel = cancel
	s.videoTaskDone = make(chan struct{})
	go func() {
		defer close(s.videoTaskDone)
		if monitor != nil {
			monitorDone := make(chan struct{})
			go func() { defer close(monitorDone); monitor.run(ctx) }()
			defer func() { <-monitorDone }()
		}
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			// Acquire each lease immediately before processing, not an entire
			// batch whose leases could expire while waiting on earlier jobs.
			for i := 0; i < 8 && ctx.Err() == nil; i++ {
				claimCtx, claimCancel := context.WithTimeout(ctx, 5*time.Second)
				t, err := repo.ClaimDueVideoTask(claimCtx, videoTaskLeaseDuration)
				claimCancel()
				if err != nil {
					if ctx.Err() == nil {
						logger.LegacyPrintf("service.video_task", "durable video task claim failed")
					}
					break
				}
				if t == nil {
					break
				}
				if err := p.process(ctx, t); err != nil && ctx.Err() == nil {
					// Avoid logging upstream response/error bodies or signed URLs.
					logger.LegacyPrintf("service.video_task", "video settlement deferred user=%d key=%d account=%d", t.UserID, t.APIKeyID, t.AccountID)
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// StopVideoTaskWorker cancels in-flight network/DB calls, releases the current
// lease when possible, and waits for the worker before shared DB cleanup.
func (s *OpenAIGatewayService) StopVideoTaskWorker() {
	if s == nil {
		return
	}
	s.videoPreviewCache.stop()
	s.videoTaskMu.Lock()
	s.videoTaskStopped = true
	cancel, done := s.videoTaskCancel, s.videoTaskDone
	if cancel != nil {
		cancel()
	}
	s.videoTaskMu.Unlock()
	if done != nil {
		<-done
	}
}

// ReconcileOwnedVideoTask is the synchronous status/content gate. Call only for
// durable tasks and skip the legacy Redis claim path. Pending upstream status
// returns nil; billing errors and a busy/backoff lease fail closed. No task match
// returns nil without polling, so this cannot discover or bill legacy tasks.
func (s *OpenAIGatewayService) ReconcileOwnedVideoTask(ctx context.Context, taskID string, userID, keyID int64) error {
	t, err := s.LoadDurableVideoTask(ctx, taskID, userID, keyID)
	if err != nil || t == nil {
		return err
	}
	if t.State == VideoTaskCompleted || t.State == VideoTaskFailed {
		return nil
	}
	s.videoTaskMu.Lock()
	p, stopped := s.videoTaskProcessor, s.videoTaskStopped
	s.videoTaskMu.Unlock()
	if p == nil || stopped {
		return ErrVideoTaskRepositoryUnavailable
	}
	t, err = p.repo.ClaimOwnedVideoTask(ctx, taskID, userID, keyID, videoTaskLeaseDuration)
	if err != nil {
		return err
	}
	if t == nil {
		// Another process may have settled between lookup and claim.
		latest, err := s.LoadDurableVideoTask(ctx, taskID, userID, keyID)
		if err != nil {
			return err
		}
		if latest != nil && (latest.State == VideoTaskCompleted || latest.State == VideoTaskFailed) {
			return nil
		}
		return ErrVideoTaskBusy
	}
	return p.process(ctx, t)
}

func (p *videoTaskProcessor) process(parent context.Context, t *VideoTask) error {
	if t.Pending.TrackingOnly {
		return errors.New("tracking-only video tasks cannot enter settlement")
	}
	ctx, cancel := context.WithTimeout(parent, videoTaskAttemptTimeout)
	defer cancel()
	if t.State == VideoTaskCompleted || t.State == VideoTaskFailed {
		return nil
	}
	if t.Completion == nil {
		completion, outcome, err := p.poll(ctx, t)
		if err != nil {
			code := "poll_failed"
			var timeout net.Error
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) || (errors.As(err, &timeout) && timeout.Timeout()) {
				code = "poll_timeout"
			}
			return p.retry(t, code, err)
		}
		switch outcome {
		case "upstream_failed", "upstream_cancelled":
			return p.repo.FinishVideoTaskAttempt(ctx, t, VideoTaskFailed, 0, outcome)
		case "upstream_expired":
			// Expiration can describe result retention, not generation failure.
			// Keep reconciling the original task; never refund or resubmit here.
			return p.retry(t, outcome, errors.New("video outcome is unknown after upstream expiry"))
		}
		if completion == nil {
			return p.repo.FinishVideoTaskAttempt(ctx, t, VideoTaskPending, videoTaskPollInterval, "")
		}
		if err := p.repo.SaveVideoTaskCompletion(ctx, t, completion); err != nil {
			return p.retry(t, "completion_save_failed", err)
		}
		t.Completion, t.State = completion, VideoTaskReady
	}
	if err := p.bill(ctx, t); err != nil {
		return p.retry(t, "billing_failed", err)
	}
	// Only atomic RecordUsage success (including verified dedup) reaches here.
	// A crash here retries the same saved completion, never creates a new video.
	return p.repo.FinishVideoTaskAttempt(ctx, t, VideoTaskCompleted, 0, "")
}

func (p *videoTaskProcessor) retry(t *VideoTask, code string, cause error) error {
	// Independent bounded cleanup even when a client disconnects or shutdown
	// cancels the attempt. If this fails, the lease expires and is reclaimed.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	delay := videoTaskPollInterval * time.Duration(1<<min(max(t.Attempts-1, 0), 4))
	if !errors.Is(cause, context.Canceled) && (code == "poll_failed" || code == "poll_timeout" || code == "upstream_expired") && videoTaskPollingExhausted(t, time.Now()) {
		code = "poll_exhausted"
	}
	err := p.repo.FinishVideoTaskAttempt(ctx, t, t.State, delay, code)
	return errors.Join(cause, err)
}

func (s *OpenAIGatewayService) pollDurableVideoTask(ctx context.Context, t *VideoTask) (*VideoTaskCompletion, string, error) {
	if err := validateVideoTaskID(t.TaskID); err != nil {
		return nil, "", err
	}
	if s.accountRepo == nil {
		return nil, "", errors.New("video account repository unavailable")
	}
	account, err := s.accountRepo.GetByID(ctx, t.AccountID)
	if err != nil {
		return nil, "", err
	}
	if account == nil || account.ID != t.AccountID || !durableVideoPlatform(account.Platform) {
		return nil, "", errors.New("pinned OpenAI video account unavailable")
	}
	if err := ValidateDurableVideoTaskAccount(t, account); err != nil {
		return nil, "", err
	}
	// Local recorder only. ForwardGrokMedia enforces existing account URL,
	// credential, proxy, and provider protocol rules. Completed media is queued
	// separately for read-only caching and cannot delay settlement.
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set(videoPreviewCacheOwnerKey, t.APIKeyID)
	c.Request, err = http.NewRequestWithContext(ctx, http.MethodGet, "/v1/videos/"+url.PathEscape(t.TaskID), nil)
	if err != nil {
		return nil, "", err
	}
	result, err := s.ForwardGrokMedia(ctx, c, account, GrokMediaEndpointVideoStatus, t.TaskID, nil, "")
	if err != nil {
		return nil, "", err
	}
	if recorder.Code < 200 || recorder.Code >= 300 {
		return nil, "", fmt.Errorf("video lookup HTTP %d", recorder.Code)
	}
	if !gjson.ValidBytes(recorder.Body.Bytes()) || !gjson.ParseBytes(recorder.Body.Bytes()).IsObject() {
		return nil, "", errors.New("invalid video lookup response")
	}
	if result == nil || result.VideoCount <= 0 {
		status := strings.ToLower(strings.TrimSpace(gjson.GetBytes(recorder.Body.Bytes(), "status").String()))
		switch status {
		case "failed":
			return nil, "upstream_failed", nil
		case "cancelled", "canceled":
			return nil, "upstream_cancelled", nil
		case "expired":
			return nil, "upstream_expired", nil
		case "queued", "pending", "in_progress", "processing", "running":
			return nil, "", nil
		default:
			return nil, "", errors.New("video lookup outcome is unknown")
		}
	}
	completion, err := durableVideoCompletion(t, result, time.Now())
	if err == nil && account.Platform == PlatformGrok {
		// Grok status often omits model; its legacy parser supplies a family
		// default. Preserve the actually accepted model instead of that default.
		completion.Model = t.Pending.Model
		completion.BillingModel = firstNonEmpty(t.Pending.BillingModel, t.Pending.Model)
	}
	return completion, "", err
}

func durableVideoCompletion(t *VideoTask, result *OpenAIForwardResult, now time.Time) (*VideoTaskCompletion, error) {
	p := t.Pending
	model := firstNonEmpty(result.Model, p.Model, p.OriginalModel)
	billingModel := firstNonEmpty(result.BillingModel, p.BillingModel, p.Model, model)
	resolution := firstNonEmpty(p.VideoResolution, result.VideoResolution)
	seconds := result.VideoDurationSeconds
	if seconds <= 0 {
		seconds = p.VideoDurationSeconds
	}
	if model == "" || billingModel == "" || resolution == "" || seconds <= 0 {
		return nil, errors.New("completed video lacks pricing metadata")
	}
	// OpenAI/Seedance durations and resolution tiers must not be normalized to
	// xAI's 15-second limit/default. Preserve the forwarding layer's metadata.
	return &VideoTaskCompletion{Model: model, BillingModel: billingModel, UpstreamModel: firstNonEmpty(result.UpstreamModel, p.UpstreamModel), UpstreamResponseModel: result.UpstreamResponseModel, UpstreamEndpoint: result.UpstreamEndpoint, VideoResolution: resolution, VideoDurationSeconds: seconds, Usage: result.Usage, Duration: GrokVideoE2EDuration(p.CreatedAt, now)}, nil
}

func (s *OpenAIGatewayService) billDurableVideoTask(ctx context.Context, t *VideoTask, keys *APIKeyService) error {
	key, err := keys.GetByID(ctx, t.APIKeyID)
	if err != nil {
		return err
	}
	if key == nil || key.ID != t.APIKeyID || key.UserID != t.UserID {
		return errors.New("video task API key owner mismatch")
	}
	// Never bill a newly assigned group/subscription. Resolve the original IDs,
	// even if the key's group changed after acceptance. Do not authenticate the
	// key again: expiration/revocation must not erase accepted usage.
	copyKey := *key
	key = &copyKey
	key.GroupID, key.Group = t.GroupID, nil
	// The immutable quote carries the original group/billing type and quota
	// participation. Live group prices and subscription type are never consulted.
	if key.User == nil {
		if s.userRepo == nil {
			return errors.New("video user repository unavailable")
		}
		key.User, err = s.userRepo.GetByID(ctx, t.UserID)
		if err != nil {
			return err
		}
	}
	if key.User == nil || key.User.ID != t.UserID {
		return errors.New("video task user missing")
	}
	if s.accountRepo == nil {
		return errors.New("video account repository unavailable")
	}
	account, err := s.accountRepo.GetByID(ctx, t.AccountID)
	if err != nil {
		return err
	}
	if account == nil || account.ID != t.AccountID || !durableVideoPlatform(account.Platform) {
		return errors.New("video task account mismatch")
	}
	if err := ValidateDurableVideoTaskAccount(t, account); err != nil {
		return err
	}
	var subscription *UserSubscription
	if t.SubscriptionID != nil {
		if s.userSubRepo == nil {
			return errors.New("video subscription repository unavailable")
		}
		subscription, err = s.userSubRepo.GetByID(ctx, *t.SubscriptionID)
		if err != nil {
			return err
		}
		if subscription == nil || subscription.ID != *t.SubscriptionID || subscription.UserID != t.UserID || t.GroupID == nil || subscription.GroupID != *t.GroupID {
			return errors.New("video task subscription mismatch")
		}
	}
	input, err := durableVideoUsageInput(t)
	if err != nil {
		return err
	}
	input.APIKey, input.User, input.Account, input.Subscription, input.APIKeyService = key, key.User, account, subscription, keys
	// RecordUsage uses the captured command and log, bypassing live repricing.
	// Fingerprint conflicts still propagate instead of marking the task complete.
	return s.RecordUsage(ctx, input)
}

func durableVideoUsageInput(t *VideoTask) (*OpenAIRecordUsageInput, error) {
	c := t.Completion
	if c == nil {
		return nil, errors.New("video completion missing")
	}
	if t.CostSnapshot == nil {
		return nil, errors.New("video task cost snapshot missing")
	}
	pricingAt, err := time.Parse(time.RFC3339Nano, t.Pending.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &OpenAIRecordUsageInput{
		RequireAtomicVideo: true, VideoTaskCost: t.CostSnapshot,
		Result:    &OpenAIForwardResult{RequestID: StableGrokVideoBillingRequestID(t.TaskID), ResponseID: t.TaskID, Model: c.Model, BillingModel: c.BillingModel, UpstreamModel: c.UpstreamModel, UpstreamResponseModel: c.UpstreamResponseModel, UpstreamEndpoint: c.UpstreamEndpoint, VideoCount: 1, VideoResolution: c.VideoResolution, VideoDurationSeconds: c.VideoDurationSeconds, Usage: c.Usage, Duration: c.Duration},
		PricingAt: pricingAt, RequestPayloadHash: HashUsageRequestPayload([]byte(t.TaskID)), QuotaPlatform: t.CostSnapshot.QuotaPlatform,
		InboundEndpoint: "/v1/videos", UpstreamEndpoint: c.UpstreamEndpoint,
		ChannelUsageFields: ChannelUsageFields{OriginalModel: t.Pending.OriginalModel, ChannelMappedModel: t.Pending.Model},
	}, nil
}
