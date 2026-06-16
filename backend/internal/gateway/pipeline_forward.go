package gateway

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
)

const (
	// maxSameAccountRetries is the per-account retry limit for
	// RetryableOnSameAccount errors before switching accounts.
	maxSameAccountRetries = 3
	// sameAccountRetryDelay is the pause between same-account retries.
	sameAccountRetryDelay = 500 * time.Millisecond
)

// pipelineFailoverState tracks cross-iteration failover state within
// a single request lifecycle. It mirrors the critical parts of the
// legacy handler.FailoverState without the platform-specific delays.
type pipelineFailoverState struct {
	switchCount           int
	maxSwitches           int
	excludedIDs           map[int64]struct{}
	sameAccountRetryCount map[int64]int
	lastFailoverErr       *service.UpstreamFailoverError
	forceCacheBilling     bool
}

func newPipelineFailoverState(maxSwitches int) *pipelineFailoverState {
	return &pipelineFailoverState{
		maxSwitches:           maxSwitches,
		excludedIDs:           make(map[int64]struct{}),
		sameAccountRetryCount: make(map[int64]int),
	}
}

func (p *GatewayPipeline) selectAndForward(
	ctx context.Context,
	w http.ResponseWriter,
	req *ForwardRequest,
) (*ForwardResult, error) {
	fs := newPipelineFailoverState(p.effectiveMaxFailovers(req.Protocol))

	for {
		result, done, err := p.tryOneAccount(ctx, w, req, fs)
		if done {
			return result, err
		}
		// Loop continues for retry/failover
	}
}

func (p *GatewayPipeline) tryOneAccount(
	ctx context.Context,
	w http.ResponseWriter,
	req *ForwardRequest,
	fs *pipelineFailoverState,
) (*ForwardResult, bool, error) {
	account, releaseFunc, err := p.selectAccount(ctx, req, fs.excludedIDs)
	if err != nil {
		return p.handleSelectionError(ctx, fs, err)
	}
	req.Account = account
	// For OpenAI pool-mode accounts without a session hash, generate a
	// one-time sticky key so same-account retries don't re-balance.
	req.SessionHash = ensurePoolModeSessionHash(req.SessionHash, account)
	if req.GinContext != nil {
		setOpsSelectedAccount(req.GinContext, account.ID, account.Platform)
	}
	releaseFunc = wrapReleaseOnDone(ctx, releaseFunc)
	defer func() {
		if releaseFunc != nil {
			releaseFunc()
		}
	}()

	if err := p.consumeBilling(ctx, req); err != nil {
		return nil, true, err
	}

	// Replace model in body if channel mapping is active
	p.applyModelReplacement(req)

	result, err := p.forwardToProvider(ctx, w, req)
	if err != nil {
		return p.handleForwardError(ctx, w, req, fs, err)
	}
	// Post-flight success hook: lets platform adapters reset per-account
	// failure state (e.g. Antigravity INTERNAL 500 penalty ladder). No-op
	// for platforms without such state.
	p.gatewayService.HandlePipelineForwardSuccess(ctx, req.Account)
	return result, true, nil
}

// applyModelReplacement replaces the model name in the request body
// when channel mapping is active. This must happen after channel
// mapping resolution and before forwarding to the upstream provider.
func (p *GatewayPipeline) applyModelReplacement(req *ForwardRequest) {
	if req.ChannelMapping == nil || !req.ChannelMapping.Mapped {
		return
	}
	req.RawBody = service.ReplaceModelInBody(req.RawBody, req.ChannelMapping.MappedModel)
}

func (p *GatewayPipeline) selectAccount(
	ctx context.Context,
	req *ForwardRequest,
	excludedIDs map[int64]struct{},
) (*service.Account, func(), error) {
	// Images protocol: loop until we find an account that supports the
	// required image capability, mirroring the legacy
	// SelectAccountWithSchedulerForImages filtering behaviour.
	if req.Protocol == ProtocolImages && req.ImagesRequest != nil {
		return p.selectAccountForImages(ctx, req, excludedIDs)
	}

	return p.selectAccountGeneric(ctx, req, excludedIDs)
}

// selectAccountGeneric performs the standard load-aware account selection
// used by all non-images protocols.
func (p *GatewayPipeline) selectAccountGeneric(
	ctx context.Context,
	req *ForwardRequest,
	excludedIDs map[int64]struct{},
) (*service.Account, func(), error) {
	selection, err := p.gatewayService.SelectAccountWithLoadAwareness(
		ctx, req.GroupID, req.SessionHash, req.Model,
		excludedIDs, req.MetadataUserID, req.UserID,
	)
	if err != nil {
		return nil, nil, err
	}
	return p.acquireSlotIfNeeded(ctx, req, selection)
}

// selectAccountForImages wraps selectAccountGeneric with image-capability
// filtering, matching the legacy SelectAccountWithSchedulerForImages logic:
// try with the required capability first, then fall back to Basic if the
// required capability was Native.
func (p *GatewayPipeline) selectAccountForImages(
	ctx context.Context,
	req *ForwardRequest,
	excludedIDs map[int64]struct{},
) (*service.Account, func(), error) {
	capability := req.ImagesRequest.RequiredCapability
	localExcluded := cloneExcludedIDs(excludedIDs)

	account, release, err := p.selectAccountWithImageCapability(ctx, req, localExcluded, capability)
	if err == nil {
		return account, release, nil
	}
	// Fall back from Native to Basic (OAuth accounts support both).
	if capability == service.OpenAIImagesCapabilityNative {
		return p.selectAccountWithImageCapability(ctx, req, excludedIDs, service.OpenAIImagesCapabilityBasic)
	}
	return nil, nil, err
}

// selectAccountWithImageCapability repeatedly selects accounts, skipping
// those that don't support the given capability.
func (p *GatewayPipeline) selectAccountWithImageCapability(
	ctx context.Context,
	req *ForwardRequest,
	excludedIDs map[int64]struct{},
	capability service.OpenAIImagesCapability,
) (*service.Account, func(), error) {
	localExcluded := cloneExcludedIDs(excludedIDs)
	for {
		account, release, err := p.selectAccountGeneric(ctx, req, localExcluded)
		if err != nil {
			return nil, nil, err
		}
		if account.SupportsOpenAIImageCapability(capability) {
			return account, release, nil
		}
		// Account doesn't support the capability; release and exclude.
		if release != nil {
			release()
		}
		if localExcluded == nil {
			localExcluded = make(map[int64]struct{})
		}
		if _, exists := localExcluded[account.ID]; exists {
			return nil, nil, service.ErrNoAvailableAccounts
		}
		localExcluded[account.ID] = struct{}{}
	}
}

// acquireSlotIfNeeded handles the wait-plan slot acquisition that was
// previously inlined in selectAccount. It increments the account wait
// queue counter before waiting and decrements it after (matching the
// legacy IncrementAccountWaitCount / DecrementAccountWaitCount flow).
func (p *GatewayPipeline) acquireSlotIfNeeded(
	ctx context.Context,
	req *ForwardRequest,
	selection *service.AccountSelectionResult,
) (*service.Account, func(), error) {
	account := selection.Account
	releaseFunc := selection.ReleaseFunc
	if !selection.Acquired && selection.WaitPlan != nil {
		// Increment account wait queue; reject with 429 if full.
		accountWaitCounted := false
		canWait, err := p.concurrency.IncrementAccountWaitCount(
			ctx, account.ID, selection.WaitPlan.MaxWaiting,
		)
		if err != nil {
			slog.Warn("pipeline.account_wait_counter_increment_failed",
				"account_id", account.ID, "error", err,
			)
			// On error, allow through (best-effort, matches legacy behaviour).
		} else if !canWait {
			slog.Info("pipeline.account_wait_queue_full",
				"account_id", account.ID,
				"max_waiting", selection.WaitPlan.MaxWaiting,
			)
			return nil, nil, fmt.Errorf("account wait queue full")
		}
		if err == nil && canWait {
			accountWaitCounted = true
		}

		slot, err := p.concurrency.AcquireAccountSlot(
			ctx, account.ID, selection.WaitPlan.MaxConcurrency,
		)
		// Always decrement wait count after acquisition attempt.
		if accountWaitCounted {
			p.concurrency.DecrementAccountWaitCount(ctx, account.ID)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("account slot: %w", err)
		}
		releaseFunc = slot.ReleaseFunc
		_ = p.gatewayService.BindStickySession(ctx, req.GroupID, req.SessionHash, account.ID)
	}
	return account, releaseFunc, nil
}

func cloneExcludedIDs(src map[int64]struct{}) map[int64]struct{} {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[int64]struct{}, len(src))
	for k := range src {
		dst[k] = struct{}{}
	}
	return dst
}

func (p *GatewayPipeline) consumeBilling(ctx context.Context, req *ForwardRequest) error {
	if req.BillingTicket == nil {
		return nil
	}
	channelID := int64(0)
	if req.ChannelMapping != nil {
		channelID = req.ChannelMapping.ChannelID
	}
	if err := req.BillingTicket.Consume(ctx, channelID, req.Account.ID); err != nil {
		return fmt.Errorf("gateway: billing consume: %w", err)
	}
	return nil
}

func (p *GatewayPipeline) forwardToProvider(
	ctx context.Context,
	w http.ResponseWriter,
	req *ForwardRequest,
) (*ForwardResult, error) {
	provider, ok := p.registry.Get(req.Account.Platform)
	if !ok {
		return nil, fmt.Errorf("gateway: no provider for platform %q", req.Account.Platform)
	}
	return provider.Forward(ctx, w, req)
}

// handleSelectionError handles account selection failures. When all
// candidates are excluded after a 503, it clears the exclusion list
// and retries (single-account backoff pattern from legacy).
func (p *GatewayPipeline) handleSelectionError(
	ctx context.Context,
	fs *pipelineFailoverState,
	err error,
) (*ForwardResult, bool, error) {
	if len(fs.excludedIDs) == 0 {
		return nil, true, fmt.Errorf("gateway: select account: %w", err)
	}
	// Single-account backoff: if the last error was 503 and we
	// haven't exhausted switches, clear exclusions and retry.
	if fs.lastFailoverErr != nil &&
		fs.lastFailoverErr.StatusCode == http.StatusServiceUnavailable &&
		fs.switchCount <= fs.maxSwitches {
		slog.Warn("pipeline.single_account_backoff",
			"switch_count", fs.switchCount,
			"max_switches", fs.maxSwitches,
		)
		if !sleepWithContext(ctx, 2*time.Second) {
			return nil, true, ctx.Err()
		}
		fs.excludedIDs = make(map[int64]struct{})
		return nil, false, nil // retry
	}
	return nil, true, fmt.Errorf("gateway: failover limit reached, no account succeeded")
}

// handleForwardError processes a forward error, deciding whether to
// retry on the same account, switch accounts, or abort.
func (p *GatewayPipeline) handleForwardError(
	ctx context.Context,
	w http.ResponseWriter,
	req *ForwardRequest,
	fs *pipelineFailoverState,
	err error,
) (*ForwardResult, bool, error) {
	// Non-failover: bytes already written or upstream accepted.
	if writerHasData(w) {
		return nil, true, fmt.Errorf("gateway: forward failed after data written: %w", err)
	}
	if req.UpstreamAccepted {
		return nil, true, fmt.Errorf("gateway: forward failed after upstream accepted: %w", err)
	}

	// Check if the error is a failover-eligible upstream error.
	var failoverErr *service.UpstreamFailoverError
	if !errors.As(err, &failoverErr) {
		return p.handleNonFailoverError(ctx, req, fs, err)
	}

	// Ensure RequestedModel is populated so HandlePipelineUpstreamError can
	// trigger per-model rate limiting (e.g. model-not-found cooldown).
	// Providers may or may not set it; the pipeline always knows the model.
	if failoverErr.RequestedModel == "" {
		failoverErr.RequestedModel = req.Model
	}

	fs.lastFailoverErr = failoverErr
	p.gatewayService.HandlePipelineUpstreamError(ctx, req.Account, failoverErr)
	if failoverErr.ForceCacheBilling {
		fs.forceCacheBilling = true
		req.ForceCacheBilling = true
	}

	// Same-account retry for transient errors.
	if failoverErr.RetryableOnSameAccount {
		if result, done, retryErr := p.handleSameAccountRetry(ctx, req, fs, failoverErr); done {
			return result, done, retryErr
		}
	}

	return p.handleAccountSwitch(req, fs, failoverErr)
}

// handleNonFailoverError handles errors that are not UpstreamFailoverError.
// Falls back to the provider's ShouldFailover check and, if approved,
// switches to a different account.
func (p *GatewayPipeline) handleNonFailoverError(
	ctx context.Context,
	req *ForwardRequest,
	fs *pipelineFailoverState,
	err error,
) (*ForwardResult, bool, error) {
	provider, ok := p.registry.Get(req.Account.Platform)
	if !ok || !provider.ShouldFailover(ctx, req, err) {
		return nil, true, err
	}
	// Provider says failover but no UpstreamFailoverError detail;
	// treat as a simple account switch.
	fs.excludedIDs[req.Account.ID] = struct{}{}
	fs.switchCount++
	if fs.switchCount > fs.maxSwitches {
		return nil, true, fmt.Errorf("gateway: failover limit reached: %w", err)
	}
	req.SwitchCount = fs.switchCount
	return nil, false, nil
}

// handleSameAccountRetry attempts to retry on the same account for
// transient errors. Returns (nil, false, nil) when retries are exhausted
// so the caller falls through to account switch.
func (p *GatewayPipeline) handleSameAccountRetry(
	ctx context.Context,
	req *ForwardRequest,
	fs *pipelineFailoverState,
	failoverErr *service.UpstreamFailoverError,
) (*ForwardResult, bool, error) {
	retries := fs.sameAccountRetryCount[req.Account.ID]
	if retries < maxSameAccountRetries {
		fs.sameAccountRetryCount[req.Account.ID]++
		slog.Warn("pipeline.same_account_retry",
			"account_id", req.Account.ID,
			"retry_count", retries+1,
			"max_retries", maxSameAccountRetries,
			"status_code", failoverErr.StatusCode,
		)
		if !sleepWithContext(ctx, sameAccountRetryDelay) {
			return nil, true, ctx.Err()
		}
		return nil, false, nil
	}
	// Same-account retries exhausted: temp-unschedule the account.
	p.gatewayService.TempUnscheduleRetryableError(ctx, req.Account.ID, failoverErr)
	return nil, false, nil
}

// handleAccountSwitch excludes the current account and increments the
// switch counter for failover to a different account.
func (p *GatewayPipeline) handleAccountSwitch(
	req *ForwardRequest,
	fs *pipelineFailoverState,
	failoverErr *service.UpstreamFailoverError,
) (*ForwardResult, bool, error) {
	fs.excludedIDs[req.Account.ID] = struct{}{}
	fs.switchCount++
	if fs.switchCount > fs.maxSwitches {
		return nil, true, fmt.Errorf("gateway: failover limit reached: %w", failoverErr)
	}
	req.SwitchCount = fs.switchCount
	slog.Info("pipeline.failover_switch",
		"account_id", req.Account.ID,
		"platform", req.Account.Platform,
		"switch_count", fs.switchCount,
		"status_code", failoverErr.StatusCode,
	)
	return nil, false, nil
}

func (p *GatewayPipeline) recordUsage(
	ctx context.Context,
	req *ForwardRequest,
	result *ForwardResult,
	record RecordUsageFunc,
) error {
	if record == nil || result == nil || req.Account == nil {
		return nil
	}
	return record(ctx, req.Account, result)
}

// writerHasData checks if the http.ResponseWriter has already
// written data to the client. Supports gin.ResponseWriter and
// the standard interface.
func writerHasData(w http.ResponseWriter) bool {
	type sizer interface {
		Size() int
	}
	if sw, ok := w.(sizer); ok {
		return sw.Size() > 0
	}
	return false
}

// wrapReleaseOnDone wraps a slot release function so it fires
// automatically when the request context is done (client disconnect).
// This ensures slots are released even if the handler returns
// without explicitly calling the release function.
func wrapReleaseOnDone(ctx context.Context, releaseFunc func()) func() {
	if releaseFunc == nil {
		return nil
	}
	var once sync.Once
	var stop func() bool

	release := func() {
		once.Do(func() {
			if stop != nil {
				_ = stop()
			}
			releaseFunc()
		})
	}
	stop = context.AfterFunc(ctx, release)
	return release
}

// sleepWithContext pauses for the given duration, returning false
// if the context is cancelled before the sleep completes.
func sleepWithContext(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

// ensurePoolModeSessionHash generates a one-time sticky session key for
// OpenAI pool-mode accounts that have no session hash. This prevents
// same-account retries from re-balancing to a different account.
func ensurePoolModeSessionHash(sessionHash string, account *service.Account) string {
	if sessionHash != "" || account == nil || !account.IsPoolMode() {
		return sessionHash
	}
	return "openai-pool-retry-" + uuid.NewString()
}
