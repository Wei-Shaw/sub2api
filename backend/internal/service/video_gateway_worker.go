package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const (
	// videoProviderCallTimeout mirrors the adapter's http.Client timeout: a single
	// provider create/poll round-trip can take this long.
	videoProviderCallTimeout = 30 * time.Second
	// videoTaskProcessBudget is the per-task processing budget (one provider call
	// plus the follow-up DB writes). It is derived from a cancel-detached root so a
	// poll that outlives the worker's tick cadence still has time to PERSIST its
	// result; without it a slow poll's UpdateTask runs on an already-expired tick
	// context and the result (even a 'succeeded') is silently dropped. (VA2.)
	videoTaskProcessBudget = videoProviderCallTimeout + 10*time.Second
	// videoDefaultMaxPollAttempts is the per-task poll cap when config is absent.
	// 72 × 5s poll interval = 360s window ≥ 2× the ~170s Seedance 720p generation time.
	// It is the 720p tier default (see videoPollAttempts720p).
	videoDefaultMaxPollAttempts = 72
	// videoDefaultPollInterval is the worker tick / poll cadence when config is absent.
	videoDefaultPollInterval = 5 * time.Second

	// Per-task poll budgets scale with resolution (B2): higher resolutions generate
	// far slower — 1080p/5s observed ~19min real vs ~170s for 720p — so they need a
	// longer poll window, while low resolutions finish fast and should fail faster.
	// These are the DEFAULT caps used when max_poll_attempts is NOT pinned in config;
	// a configured max_poll_attempts overrides all tiers. At the 5s default interval:
	//   480p →  48 × 5s =  4min,  720p → 72 × 5s = 6min,  1080p → 300 × 5s = 25min.
	videoPollAttempts480p  = 48
	videoPollAttempts720p  = videoDefaultMaxPollAttempts // 72; unchanged default tier
	videoPollAttempts1080p = 300
	// videoTaskTimeoutMargin keeps the wall-clock backstop strictly OUTSIDE the
	// resolution-scaled poll window: the poll-count cap is the primary bound and the
	// wall-clock timeout is the outer net. effectiveTaskTimeout = max(configured
	// task_timeout_minutes, pollWindow + this margin), so raising 1080p's poll window
	// also raises its wall-clock backstop instead of letting a stale 15min timeout
	// kill a 25min 1080p render. (Enforces the config-comment invariant automatically.)
	videoTaskTimeoutMargin = 5 * time.Minute
)

type VideoGatewayWorker struct {
	service         *VideoGatewayService
	finalizer       *VideoTaskFinalizer
	interval        time.Duration
	timeout         time.Duration
	batch           int
	providerEnabled bool
	reaper          *BillingReservationReaper
	stopCh          chan struct{}
	doneCh          chan struct{}
	startOnce       sync.Once
	stopOnce        sync.Once
	cancel          context.CancelFunc
}

func NewVideoGatewayWorker(service *VideoGatewayService, cfg *config.Config, finalizers ...*VideoTaskFinalizer) *VideoGatewayWorker {
	interval := videoDefaultPollInterval
	timeout := 15 * time.Minute
	batch := videoDefaultBatchSize
	providerEnabled := cfg == nil || cfg.VideoGateway.WorkerEnabled
	if cfg != nil {
		if cfg.VideoGateway.PollIntervalSeconds > 0 {
			interval = time.Duration(cfg.VideoGateway.PollIntervalSeconds) * time.Second
		}
		if cfg.VideoGateway.TaskTimeoutMinutes > 0 {
			timeout = time.Duration(cfg.VideoGateway.TaskTimeoutMinutes) * time.Minute
		}
		if cfg.VideoGateway.WorkerBatchSize > 0 {
			batch = cfg.VideoGateway.WorkerBatchSize
		}
	}
	var finalizer *VideoTaskFinalizer
	if len(finalizers) > 0 {
		finalizer = finalizers[0]
	} else if service != nil {
		finalizer = provideVideoTaskFinalizer(service.repo)
	}
	worker := &VideoGatewayWorker{
		service:         service,
		finalizer:       finalizer,
		interval:        interval,
		timeout:         timeout,
		batch:           batch,
		providerEnabled: providerEnabled,
		stopCh:          make(chan struct{}),
		doneCh:          make(chan struct{}),
	}
	if service != nil && service.repo != nil {
		if repo, ok := service.repo.(BillingReservationReaperRepository); ok {
			worker.reaper = NewBillingReservationReaper(repo, cfg)
		}
	}
	return worker
}

func ProvideVideoGatewayWorker(service *VideoGatewayService, cfg *config.Config) *VideoGatewayWorker {
	worker := NewVideoGatewayWorker(service, cfg)
	worker.Start()
	return worker
}

func (w *VideoGatewayWorker) Start() {
	if w == nil {
		return
	}
	w.startOnce.Do(func() {
		if w.service == nil || (!w.providerEnabled && (w.reaper == nil || !w.reaper.enabled)) {
			close(w.doneCh)
			return
		}
		ctx, cancel := context.WithCancel(context.Background())
		w.cancel = cancel
		go w.loop(ctx)
	})
}

func (w *VideoGatewayWorker) Stop() {
	if w == nil {
		return
	}
	w.Start()
	w.stopOnce.Do(func() {
		if w.cancel != nil {
			w.cancel()
		}
		close(w.stopCh)
		<-w.doneCh
	})
}

func (w *VideoGatewayWorker) loop(ctx context.Context) {
	defer close(w.doneCh)
	// A long-lived cancelable context tied to Stop. Ticks intentionally do NOT
	// impose the poll interval as a deadline: a single provider poll can take up to
	// videoProviderCallTimeout (~30s), far longer than the 5s tick cadence, and its
	// result must still be persisted. Per-task budgets are enforced in processTask;
	// shutdown cancels any in-flight batch via this context.
	var providerTicker, reaperTicker *time.Ticker
	var providerC, reaperC <-chan time.Time
	if w.providerEnabled {
		providerTicker = time.NewTicker(w.interval)
		providerC = providerTicker.C
		defer providerTicker.Stop()
	}
	if w.reaper != nil && w.reaper.enabled {
		reaperTicker = time.NewTicker(w.reaper.interval)
		reaperC = reaperTicker.C
		defer reaperTicker.Stop()
	}
	for {
		select {
		case <-providerC:
			if err := w.ProcessOnce(ctx); err != nil {
				slog.Warn("video_gateway: worker tick failed", "error", err)
			}
		case <-reaperC:
			if _, err := w.reaper.RunOnce(ctx); err != nil && ctx.Err() == nil {
				slog.Warn("video_gateway: reservation reaper tick failed", "error", err)
			}
		case <-w.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (w *VideoGatewayWorker) ProcessOnce(ctx context.Context) error {
	if w == nil || w.service == nil {
		return nil
	}
	return w.service.processRunnableTasks(ctx, w.batch, w.timeout, w.finalizer)
}

func (s *VideoGatewayService) ProcessRunnableTasks(ctx context.Context, batch int, timeout time.Duration) error {
	var finalizer *VideoTaskFinalizer
	if s != nil {
		finalizer = provideVideoTaskFinalizer(s.repo)
	}
	return s.processRunnableTasks(ctx, batch, timeout, finalizer)
}

func (s *VideoGatewayService) processRunnableTasks(ctx context.Context, batch int, timeout time.Duration, finalizer *VideoTaskFinalizer) error {
	if batch <= 0 {
		batch = videoDefaultBatchSize
	}
	tasks, err := s.repo.ListRunnableTasks(ctx, batch)
	if err != nil {
		return fmt.Errorf("list runnable video tasks: %w", err)
	}
	for _, task := range tasks {
		if err := s.processTaskWithFinalizer(ctx, task, timeout, finalizer); err != nil {
			slog.Warn("video_gateway: process task failed", "task_id", task.ID, "provider", task.Provider, "error", err)
		}
	}
	if s.videoReliabilityCoreEnabled() {
		return nil
	}
	if err := s.retryUnchargedSucceededVideoBilling(ctx, batch); err != nil {
		return err
	}
	return nil
}

func (s *VideoGatewayService) retryUnchargedSucceededVideoBilling(ctx context.Context, batch int) error {
	if s == nil || s.repo == nil {
		return nil
	}
	if batch <= 0 {
		batch = videoDefaultBatchSize
	}
	tasks, err := s.repo.ListUnchargedSucceededVideoTasks(ctx, batch)
	if err != nil {
		return fmt.Errorf("list uncharged succeeded video tasks: %w", err)
	}
	for _, task := range tasks {
		if task == nil || task.Status != VideoStatusSucceeded {
			continue
		}
		s.applyVideoBillingMetadata(task)
		s.chargeForVideo(ctx, task)
	}
	return nil
}

// normalizeResolutionTier folds a free-text resolution into one of the three poll
// budget tiers. Unknown / empty resolutions fall back to the safe 720p middle tier.
// Anything ≥1080p (incl. 2K/4K) maps to the long tier so it is never under-polled.
func normalizeResolutionTier(resolution string) string {
	r := strings.ToLower(strings.TrimSpace(resolution))
	switch {
	case strings.Contains(r, "2160"), strings.Contains(r, "4k"), strings.Contains(r, "1440"), strings.Contains(r, "2k"), strings.Contains(r, "1080"):
		return "1080p"
	case strings.Contains(r, "480"), strings.Contains(r, "360"), strings.Contains(r, "240"):
		return "480p"
	default:
		return "720p"
	}
}

// scaleBudget returns ceil(baseline × num / den), clamped to ≥1, so a scaled poll
// budget never rounds down to 0 and a higher tier always gets at least 1 poll.
func scaleBudget(baseline, num, den int) int {
	if den <= 0 {
		return baseline
	}
	scaled := (baseline*num + den - 1) / den // ceil division
	if scaled < 1 {
		return 1
	}
	return scaled
}

// scalePollBudgetForResolution scales a 720p BASELINE poll budget to the task's
// resolution tier, preserving the 480p:720p:1080p = 48:72:300 ratio (B2). It ALWAYS
// runs — max_poll_attempts is the 720p baseline, never a flat per-resolution cap — so
// a 1080p task gets its long window even under the pinned production default (72),
// which is the bug a config-pinned early-return would re-introduce.
func scalePollBudgetForResolution(baseline720p int, resolution string) int {
	if baseline720p <= 0 {
		baseline720p = videoPollAttempts720p
	}
	switch normalizeResolutionTier(resolution) {
	case "480p":
		return scaleBudget(baseline720p, videoPollAttempts480p, videoPollAttempts720p)
	case "1080p":
		return scaleBudget(baseline720p, videoPollAttempts1080p, videoPollAttempts720p)
	default:
		return baseline720p
	}
}

// maxPollAttemptsForTask is the per-task VA2 poll cap, ALWAYS scaled by the task's
// resolution (B2). The configured max_poll_attempts is the 720p BASELINE budget
// (production default 72, viper-set + validated >0); 480p scales down and 1080p
// scales up from it. Scaling runs even when config pins the baseline, so a 1080p
// render (~19min real) actually gets its longer window in production rather than the
// flat ~6min the baseline alone would yield — the defect a config early-return hid.
func (s *VideoGatewayService) maxPollAttemptsForTask(task *VideoTask) int {
	baseline := videoPollAttempts720p
	if s.cfg != nil && s.cfg.VideoGateway.MaxPollAttempts > 0 {
		baseline = s.cfg.VideoGateway.MaxPollAttempts
	}
	resolution := ""
	if task != nil {
		resolution = task.Resolution
	}
	return scalePollBudgetForResolution(baseline, resolution)
}

// pollInterval is the configured worker tick / poll cadence (default 5s), used to
// size the poll window for the wall-clock backstop.
func (s *VideoGatewayService) pollInterval() time.Duration {
	if s.cfg != nil && s.cfg.VideoGateway.PollIntervalSeconds > 0 {
		return time.Duration(s.cfg.VideoGateway.PollIntervalSeconds) * time.Second
	}
	return videoDefaultPollInterval
}

// effectiveTaskTimeout is the per-task wall-clock backstop. It keeps the configured
// task_timeout_minutes as a floor but guarantees the wall-clock sits strictly OUTSIDE
// the resolution-scaled poll window (pollWindow + margin), so raising 1080p's poll
// budget does not let a stale 15min timeout kill a 25min 1080p render (B2).
func (s *VideoGatewayService) effectiveTaskTimeout(task *VideoTask, base time.Duration) time.Duration {
	needed := time.Duration(s.maxPollAttemptsForTask(task))*s.pollInterval() + videoTaskTimeoutMargin
	if base >= needed {
		return base
	}
	return needed
}

func (s *VideoGatewayService) processTask(ctx context.Context, task *VideoTask, timeout time.Duration) error {
	var finalizer *VideoTaskFinalizer
	if s != nil {
		finalizer = provideVideoTaskFinalizer(s.repo)
	}
	return s.processTaskWithFinalizer(ctx, task, timeout, finalizer)
}

func (s *VideoGatewayService) processTaskWithFinalizer(ctx context.Context, task *VideoTask, timeout time.Duration, finalizer *VideoTaskFinalizer) error {
	if task == nil || IsTerminalVideoStatus(task.Status) {
		return nil
	}
	// Honor shutdown / cancellation between tasks before starting provider work.
	if err := ctx.Err(); err != nil {
		return err
	}
	// Give each task a bounded budget for one provider round-trip plus the follow-up
	// DB writes. The loop no longer imposes the short poll interval as a deadline, so
	// this budget — deliberately larger than the provider call timeout — is what lets
	// a poll slower than the tick cadence still persist its result (VA2). Cancellation
	// still propagates from the parent, so shutdown promptly aborts in-flight work.
	taskCtx, cancel := context.WithTimeout(ctx, videoTaskProcessBudget)
	defer cancel()
	if (task.Status == VideoStatusSubmitted || task.Status == VideoStatusRunning) &&
		task.DispatchState == VideoDispatchStateDispatching && strings.TrimSpace(task.UpstreamTaskID) == "" {
		dispatchRepo, ok := s.repo.(VideoDispatchRepository)
		if !ok {
			return fmt.Errorf("video dispatch CAS repository is unavailable")
		}
		return s.markVideoDispatchUnknown(taskCtx, dispatchRepo, task, "provider dispatch was interrupted before the upstream task id was persisted", nil, "")
	}

	// B2: the wall-clock backstop scales with resolution so a slow 1080p render is not
	// killed before its (longer) poll window completes. effectiveTaskTimeout keeps the
	// configured timeout as a floor and lifts it above pollWindow+margin when needed.
	effectiveTimeout := s.effectiveTaskTimeout(task, timeout)
	if effectiveTimeout > 0 && time.Since(task.CreatedAt) > effectiveTimeout {
		return s.terminateVideoTaskWithFinalizer(taskCtx, task, "video task timed out", map[string]any{
			"timeout_minutes": int(effectiveTimeout.Minutes()),
		}, finalizer)
	}
	account, err := s.repo.GetProviderAccount(taskCtx, task.ProviderAccountID)
	if err != nil {
		return err
	}
	s.decryptProviderKey(account)
	if account.APIKeyDecryptFailed {
		return ErrVideoKeyDecryptFailed
	}
	adapter, err := s.adapterFor(task.Provider)
	if err != nil {
		return err
	}
	switch task.Status {
	case VideoStatusQueued:
		if err := s.submitTask(taskCtx, adapter, account, task); err != nil {
			if errors.Is(err, ErrVideoDispatchUnknown) {
				return err
			}
			return s.failTaskWithFinalizer(taskCtx, task, "video provider submit failed: "+err.Error(), map[string]any{"stage": "submit"}, finalizer)
		}
	case VideoStatusSubmitted, VideoStatusRunning:
		// VA2 poll cap: once a task has used its poll budget without reaching a
		// terminal status, fail it deterministically instead of polling forever.
		// B2: the budget scales with the task's resolution.
		maxPolls := s.maxPollAttemptsForTask(task)
		if task.PollCount >= maxPolls {
			return s.terminateVideoTaskWithFinalizer(taskCtx, task, fmt.Sprintf("video task exceeded max poll attempts (%d)", maxPolls), map[string]any{
				"max_poll_attempts": maxPolls,
				"poll_count":        task.PollCount,
			}, finalizer)
		}
		if err := s.pollTaskWithFinalizer(taskCtx, adapter, account, task, finalizer); err != nil {
			var finalizationErr *videoTaskFinalizationAttemptError
			if errors.As(err, &finalizationErr) {
				return err
			}
			var pollPersistenceErr *videoTaskPollPersistenceAttemptError
			if errors.As(err, &pollPersistenceErr) {
				return err
			}
			return s.failTaskWithFinalizer(taskCtx, task, "video provider poll failed: "+err.Error(), map[string]any{"stage": "poll"}, finalizer)
		}
	default:
		return nil
	}
	return nil
}

// terminateVideoTask closes out a non-terminal task as failed with a reason
// (wall-clock timeout or VA2 poll-cap exhaustion), recording the terminal event
// and a usage log so it is always reconciled exactly once.
func (s *VideoGatewayService) terminateVideoTaskWithFinalizer(ctx context.Context, task *VideoTask, message string, payload map[string]any, finalizer *VideoTaskFinalizer) error {
	candidate := cloneVideoTaskForFinalization(task)
	candidate.Status = VideoStatusFailed
	candidate.ErrorMessage = message
	now := time.Now().UTC()
	candidate.CompletedAt = &now
	if s.videoReliabilityCoreEnabled() {
		return s.finalizeWorkerTerminal(ctx, task, candidate, payload, MustUSD("0"), PricingSnapshot{}, finalizer)
	}
	*task = *candidate
	if err := s.repo.UpdateTask(ctx, task); err != nil {
		return err
	}
	_ = s.repo.AddTaskEvent(ctx, &VideoTaskEvent{
		VideoTaskID: task.ID,
		EventType:   "failed",
		Message:     message,
		Payload:     payload,
	})
	s.applyVideoBillingMetadata(task)
	_ = s.repo.InsertUsageLog(ctx, task)
	return nil
}

func (s *VideoGatewayService) submitTask(ctx context.Context, adapter VideoAdapter, account *VideoProviderAccount, task *VideoTask) error {
	dispatchRepo, ok := s.repo.(VideoDispatchRepository)
	if !ok {
		// Several offline route harnesses intentionally use a legacy in-memory
		// repository. Preserve that compatibility only for the deterministic,
		// non-billable mock adapter; real providers remain fail-closed without CAS.
		if task.Provider == VideoProviderMock {
			return s.submitLegacyMockTask(ctx, adapter, account, task)
		}
		return fmt.Errorf("video dispatch CAS repository is unavailable")
	}
	dispatchingTask := *task
	claimed, err := dispatchRepo.MarkDispatchingCAS(ctx, &dispatchingTask, &VideoTaskEvent{
		VideoTaskID: task.ID,
		EventType:   VideoDispatchStateDispatching,
		Message:     "video task dispatch claimed",
		Payload:     map[string]any{"provider": task.Provider},
	})
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}
	*task = dispatchingTask
	result, err := adapter.CreateTask(ctx, account, task)
	if err != nil {
		if isAmbiguousVideoDispatchError(err) {
			return s.markVideoDispatchUnknown(ctx, dispatchRepo, task, "provider dispatch outcome requires review", err, "")
		}
		return err
	}
	if result == nil || strings.TrimSpace(result.UpstreamTaskID) == "" {
		return s.markVideoDispatchUnknown(ctx, dispatchRepo, task, "provider accepted request without a reconcilable task id", nil, "")
	}
	status := firstNonEmptyVideo(result.Status, VideoStatusSubmitted)
	// A SUCCESSFUL create means the task now lives upstream and must NEVER be re-created:
	// the only next action is to POLL it. Some providers (e.g. Ark — exact create-response
	// status token UNVERIFIED) return a create status that normalizes back to "queued"; if
	// we persisted that, processTask's `case VideoStatusQueued` would re-enter submitTask on
	// the next tick and issue a SECOND billed create. Advance any post-create "queued" to
	// "submitted" so the next tick polls instead. (No double-submit regardless of the token.)
	if status == VideoStatusQueued {
		status = VideoStatusSubmitted
	}
	acceptedTask := *task
	acceptedTask.Status = status
	acceptedTask.UpstreamTaskID = firstNonEmptyVideo(result.UpstreamTaskID, task.UpstreamTaskID)
	applied, err := dispatchRepo.MarkDispatchAcceptedCAS(ctx, &acceptedTask, &VideoTaskEvent{
		VideoTaskID: task.ID,
		EventType:   VideoStatusSubmitted,
		Message:     "video task submitted to provider",
		Payload:     map[string]any{"provider": task.Provider},
	})
	if err != nil {
		return s.markVideoDispatchUnknown(ctx, dispatchRepo, task, "provider accepted request but local task id persistence failed", err, acceptedTask.UpstreamTaskID)
	}
	if !applied {
		return nil
	}
	*task = acceptedTask
	return nil
}

func (s *VideoGatewayService) submitLegacyMockTask(ctx context.Context, adapter VideoAdapter, account *VideoProviderAccount, task *VideoTask) error {
	claimed, err := s.repo.ClaimTaskForSubmit(ctx, task.ID)
	if err != nil || !claimed {
		return err
	}
	task.Status = VideoStatusSubmitted
	result, err := adapter.CreateTask(ctx, account, task)
	if err != nil {
		return err
	}
	status := firstNonEmptyVideo(result.Status, VideoStatusSubmitted)
	if status == VideoStatusQueued {
		status = VideoStatusSubmitted
	}
	task.Status = status
	task.UpstreamTaskID = firstNonEmptyVideo(result.UpstreamTaskID, task.UpstreamTaskID)
	if err := s.repo.UpdateTask(ctx, task); err != nil {
		return err
	}
	return s.repo.AddTaskEvent(ctx, &VideoTaskEvent{
		VideoTaskID: task.ID,
		EventType:   VideoStatusSubmitted,
		Message:     "video mock task submitted",
		Payload:     map[string]any{"provider": VideoProviderMock},
	})
}

func (s *VideoGatewayService) markVideoDispatchUnknown(ctx context.Context, repo VideoDispatchRepository, task *VideoTask, message string, _ error, knownUpstreamTaskID string) error {
	unknownTask := *task
	unknownTask.Status = VideoStatusSubmitted
	unknownTask.UpstreamTaskID = ""
	unknownTask.ErrorMessage = message
	payload := map[string]any{"provider": task.Provider, "review_required": true}
	if known := strings.TrimSpace(knownUpstreamTaskID); known != "" {
		// Keep the column empty so workers never auto-poll/recreate, but retain the
		// known upstream id on the durable event for operator reconciliation.
		payload["known_upstream_task_id"] = known
	}
	applied, _ := repo.MarkDispatchUnknownCAS(ctx, &unknownTask, &VideoTaskEvent{
		VideoTaskID: task.ID,
		EventType:   "dispatch_unknown",
		Message:     message,
		Payload:     payload,
	})
	if applied {
		*task = unknownTask
		RecordReliabilityMetricAdd("video_dispatch_unknown_total", 1, map[string]string{"provider": task.Provider})
	}
	// Never return the raw provider/persistence error after an ambiguous send:
	// ProcessRunnableTasks logs returned errors, and provider errors may contain
	// sensitive URLs or response fragments. The fixed sentinel is sufficient for
	// callers; the durable dispatch_unknown event is the reconciliation evidence.
	return ErrVideoDispatchUnknown
}

func isAmbiguousVideoDispatchError(err error) bool {
	var transportErr *VideoDispatchTransportError
	return errors.As(err, &transportErr) && transportErr.RequestMayHaveBeenSent
}

func (s *VideoGatewayService) pollTask(ctx context.Context, adapter VideoAdapter, account *VideoProviderAccount, task *VideoTask) error {
	var finalizer *VideoTaskFinalizer
	if s != nil {
		finalizer = provideVideoTaskFinalizer(s.repo)
	}
	return s.pollTaskWithFinalizer(ctx, adapter, account, task, finalizer)
}

func (s *VideoGatewayService) pollTaskWithFinalizer(ctx context.Context, adapter VideoAdapter, account *VideoProviderAccount, task *VideoTask, finalizer *VideoTaskFinalizer) error {
	result, err := adapter.PollTask(ctx, account, task)
	if err != nil {
		return err
	}
	candidate := cloneVideoTaskForFinalization(task)
	// VA2: count every completed poll so processTask can enforce the per-task cap.
	candidate.PollCount++
	status := adapter.NormalizeStatus(result.Status)
	candidate.Status = status
	if result.ResultURL != "" {
		candidate.ResultURL = result.ResultURL
	}
	if result.UsageTotalTokens != nil {
		v := *result.UsageTotalTokens
		candidate.UsageTotalTokens = &v
	}
	if result.ActualResolution != "" {
		candidate.ActualResolution = result.ActualResolution
	}
	if result.ActualDuration != nil {
		v := *result.ActualDuration
		candidate.ActualDuration = &v
	}
	if result.LastFrameURL != "" {
		candidate.LastFrameURL = result.LastFrameURL
	}
	if result.ErrorMessage != "" {
		candidate.ErrorMessage = result.ErrorMessage
	}
	if result.CostEstimate > 0 {
		candidate.CostEstimate = result.CostEstimate
	}
	actualAmountUSD := MustUSD("0")
	actualPricingSnapshot := PricingSnapshot{}
	if IsTerminalVideoStatus(status) {
		if status == VideoStatusSucceeded {
			actualAmountUSD, actualPricingSnapshot, err = s.videoTaskPricing().ActualPrice(ctx, candidate)
			if err != nil {
				wrapped := fmt.Errorf("price terminal video task: %w", err)
				if s.videoReliabilityCoreEnabled() {
					return &videoTaskFinalizationAttemptError{err: wrapped}
				}
				return wrapped
			}
			if err := ApplyVideoPricingSnapshotToTask(candidate, actualPricingSnapshot); err != nil {
				wrapped := fmt.Errorf("project terminal video pricing: %w", err)
				if s.videoReliabilityCoreEnabled() {
					return &videoTaskFinalizationAttemptError{err: wrapped}
				}
				return wrapped
			}
		} else {
			candidate.CostEstimate = 0
		}
		now := time.Now().UTC()
		candidate.CompletedAt = &now
		if s.videoReliabilityCoreEnabled() {
			return s.finalizeWorkerTerminal(ctx, task, candidate, result.Payload, actualAmountUSD, actualPricingSnapshot, finalizer)
		}
	}
	eventType := status
	message := "video task status updated"
	switch status {
	case VideoStatusSucceeded:
		message = "video task succeeded"
	case VideoStatusFailed:
		message = "video task failed"
	}
	event := &VideoTaskEvent{
		VideoTaskID: task.ID,
		EventType:   eventType,
		Message:     message,
		Payload:     result.Payload,
	}
	if s.videoReliabilityCoreEnabled() {
		pollRepo, ok := s.repo.(VideoTaskPollRepository)
		if !ok {
			return &videoTaskPollPersistenceAttemptError{err: fmt.Errorf("video task poll repository is unavailable")}
		}
		persisted, err := pollRepo.UpdatePolledTaskCAS(ctx, task.Version, candidate, event)
		if err != nil {
			return &videoTaskPollPersistenceAttemptError{err: err}
		}
		applyVideoTaskPollUpdateResult(task, persisted)
		return nil
	}
	*task = *candidate
	if err := s.repo.UpdateTask(ctx, task); err != nil {
		return err
	}
	if err := s.repo.AddTaskEvent(ctx, event); err != nil {
		return err
	}
	if IsTerminalVideoStatus(status) {
		s.applyVideoBillingMetadata(task)
		_ = s.repo.InsertUsageLog(ctx, task)
		// VA1 billing: deduct only on a delivered (succeeded) generation.
		if status == VideoStatusSucceeded {
			s.chargeForVideoMoney(ctx, task, actualAmountUSD, actualPricingSnapshot)
			if captureErr := s.CollectVideoTaskGenerationContent(ctx, task); captureErr != nil {
				// Capture is deliberately fail-open: the delivered terminal task,
				// immutable usage record, and billing result must remain intact.
				slog.Warn("video_gateway: video content capture failed", "task_id", task.ID)
			}
			s.ArchiveSucceededVideoResult(ctx, task)
		}
	}
	return nil
}

func (s *VideoGatewayService) failTaskWithFinalizer(ctx context.Context, task *VideoTask, message string, payload map[string]any, finalizer *VideoTaskFinalizer) error {
	candidate := cloneVideoTaskForFinalization(task)
	candidate.Status = VideoStatusFailed
	candidate.ErrorMessage = message
	now := time.Now().UTC()
	candidate.CompletedAt = &now
	if s.videoReliabilityCoreEnabled() {
		return s.finalizeWorkerTerminal(ctx, task, candidate, payload, MustUSD("0"), PricingSnapshot{}, finalizer)
	}
	*task = *candidate
	if err := s.repo.UpdateTask(ctx, task); err != nil {
		return err
	}
	if err := s.repo.AddTaskEvent(ctx, &VideoTaskEvent{
		VideoTaskID: task.ID,
		EventType:   VideoStatusFailed,
		Message:     message,
		Payload:     payload,
	}); err != nil {
		return err
	}
	s.applyVideoBillingMetadata(task)
	_ = s.repo.InsertUsageLog(ctx, task)
	return nil
}

func (s *VideoGatewayService) finalizeWorkerTerminal(
	ctx context.Context,
	current *VideoTask,
	candidate *VideoTask,
	payload map[string]any,
	actualAmountUSD Money,
	pricingSnapshot PricingSnapshot,
	finalizer *VideoTaskFinalizer,
) error {
	if finalizer == nil {
		return &videoTaskFinalizationAttemptError{err: fmt.Errorf("video task finalizer is unavailable")}
	}
	if current == nil || candidate == nil || candidate.CompletedAt == nil {
		return &videoTaskFinalizationAttemptError{err: fmt.Errorf("terminal video task candidate is incomplete")}
	}
	result, err := finalizer.Finalize(ctx, VideoTaskFinalizationInput{
		TaskID:               current.ID,
		ExpectedVersion:      current.Version,
		TerminalStatus:       candidate.Status,
		ProviderResultURL:    candidate.ResultURL,
		ProviderErrorMessage: candidate.ErrorMessage,
		ProviderPayload:      payload,
		ActualDuration:       candidate.ActualDuration,
		ActualResolution:     candidate.ActualResolution,
		ActualTokens:         candidate.UsageTotalTokens,
		LastFrameURL:         candidate.LastFrameURL,
		PollCount:            candidate.PollCount,
		ActualCostUSD:        actualAmountUSD,
		PricingSnapshot:      pricingSnapshot,
		CompletedAt:          *candidate.CompletedAt,
	})
	if err != nil {
		return &videoTaskFinalizationAttemptError{err: err}
	}
	applyVideoTaskFinalizationResult(candidate, result)
	*current = *candidate
	return nil
}

type videoTaskFinalizationAttemptError struct {
	err error
}

// videoTaskPollPersistenceAttemptError marks a provider poll result that was
// obtained successfully but could not be persisted. The worker must retry it
// on a later pass instead of converting the storage failure into task failure.
type videoTaskPollPersistenceAttemptError struct {
	err error
}

func (e *videoTaskPollPersistenceAttemptError) Error() string {
	if e == nil || e.err == nil {
		return "video task poll persistence failed"
	}
	return "video task poll persistence failed: " + e.err.Error()
}

func (e *videoTaskPollPersistenceAttemptError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func (e *videoTaskFinalizationAttemptError) Error() string {
	if e == nil || e.err == nil {
		return "video task finalization failed"
	}
	return "video task finalization failed: " + e.err.Error()
}

func (e *videoTaskFinalizationAttemptError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func cloneVideoTaskForFinalization(task *VideoTask) *VideoTask {
	if task == nil {
		return nil
	}
	cloned := *task
	cloned.Content = append([]VideoTaskContentItem(nil), task.Content...)
	if task.UsageTotalTokens != nil {
		value := *task.UsageTotalTokens
		cloned.UsageTotalTokens = &value
	}
	if task.ActualDuration != nil {
		value := *task.ActualDuration
		cloned.ActualDuration = &value
	}
	if task.CompletedAt != nil {
		value := *task.CompletedAt
		cloned.CompletedAt = &value
	}
	return &cloned
}
