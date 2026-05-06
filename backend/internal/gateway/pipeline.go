package gateway

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/config"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// Pipeline defaults.
const (
	defaultMaxFailovers = 10
)

// ParseRequestFunc extracts a ForwardRequest from the raw body.
// Injected by the handler; each protocol has its own parse logic.
type ParseRequestFunc func(body []byte) (*ForwardRequest, error)

// RecordUsageFunc persists usage after a successful forward.
// Injected by the handler; each protocol maps ForwardResult to its
// native usage struct before writing.
type RecordUsageFunc func(ctx context.Context, account *service.Account, result *ForwardResult) error

// GatewayPipeline orchestrates the common request lifecycle:
//
//	parse -> channel mapping -> user slot -> billing check ->
//	session hash -> LOOP { select account -> account slot ->
//	billing consume -> provider.Forward } -> record usage -> close
//
// Platform differences are expressed through GatewayProvider (Forward /
// ShouldFailover) and the two handler-injected callbacks (parse / record).
type GatewayPipeline struct {
	registry       *ProviderRegistry
	gatewayService *service.GatewayService
	billingCache   *service.BillingCacheService
	concurrency    *service.ConcurrencyService
	settings       *service.SettingService
	maxFailovers   int
}

// NewGatewayPipeline creates a pipeline with all required dependencies.
func NewGatewayPipeline(
	registry *ProviderRegistry,
	gw *service.GatewayService,
	billing *service.BillingCacheService,
	conc *service.ConcurrencyService,
	settings *service.SettingService,
	cfg *config.Config,
) *GatewayPipeline {
	maxFailovers := defaultMaxFailovers
	if cfg != nil && cfg.Gateway.MaxAccountSwitches > 0 {
		maxFailovers = cfg.Gateway.MaxAccountSwitches
	}
	return &GatewayPipeline{
		registry:       registry,
		gatewayService: gw,
		billingCache:   billing,
		concurrency:    conc,
		settings:       settings,
		maxFailovers:   maxFailovers,
	}
}

// Registry returns the provider registry for diagnostic / health
// inspection. Routes can use this to verify provider wiring without
// triggering a full Execute lifecycle.
func (p *GatewayPipeline) Registry() *ProviderRegistry { return p.registry }

// Execute runs the full gateway request lifecycle. It is the single
// entry point that replaces the per-platform if/else chains in the
// gateway handler.
//
// protocol: input wire protocol ("anthropic" / "openai" / "gemini").
// forcePlatform: empty = scheduler picks; non-empty = pin to platform.
// parse: protocol-specific body parser.
// record: protocol-specific usage recorder.
func (p *GatewayPipeline) Execute(
	c *gin.Context,
	protocol string,
	forcePlatform string,
	parse ParseRequestFunc,
	record RecordUsageFunc,
) error {
	req, err := p.readAndParse(c, protocol, forcePlatform, parse)
	if err != nil {
		return err
	}
	if err := p.resolveChannelMapping(c, req); err != nil {
		return err
	}
	cleanup, err := p.acquireUserSlot(c, req)
	if err != nil {
		return err
	}
	defer cleanup()

	if err := p.prepareBilling(c, req); err != nil {
		return err
	}
	defer req.BillingTicket.Close()

	p.resolveSessionHash(c, req)

	result, err := p.selectAndForward(c.Request.Context(), c.Writer, req)
	if err != nil {
		return err
	}
	return p.recordUsage(c.Request.Context(), req, result, record)
}

// readAndParse reads the raw body from the request and invokes the
// protocol-specific parser to build the ForwardRequest. It also
// extracts auth context from the gin context.
func (p *GatewayPipeline) readAndParse(
	c *gin.Context,
	protocol, forcePlatform string,
	parse ParseRequestFunc,
) (*ForwardRequest, error) {
	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		return nil, fmt.Errorf("gateway: read body: %w", err)
	}
	if len(body) == 0 {
		return nil, errors.New("gateway: empty request body")
	}
	req, err := parse(body)
	if err != nil {
		return nil, fmt.Errorf("gateway: parse request: %w", err)
	}
	req.Protocol = protocol
	req.ForcePlatform = forcePlatform
	req.RawBody = body
	p.extractAuthContext(c, req)
	return req, nil
}

// extractAuthContext populates auth-related fields on ForwardRequest
// from the gin context set by auth middleware.
func (p *GatewayPipeline) extractAuthContext(c *gin.Context, req *ForwardRequest) {
	if apiKey, ok := middleware2.GetAPIKeyFromContext(c); ok {
		req.APIKey = apiKey
		req.User = apiKey.User
		if req.GroupID == nil {
			req.GroupID = apiKey.GroupID
		}
	}
	if subject, ok := middleware2.GetAuthSubjectFromContext(c); ok {
		req.UserID = subject.UserID
		req.Concurrency = subject.Concurrency
	}
	if sub, ok := middleware2.GetSubscriptionFromContext(c); ok {
		req.Subscription = sub
	}
}

// resolveChannelMapping applies channel-level model mapping and
// restriction rules. The result is stored on req.ChannelMapping.
func (p *GatewayPipeline) resolveChannelMapping(c *gin.Context, req *ForwardRequest) error {
	platform := ""
	if req.APIKey != nil {
		platform = req.APIKey.GroupPlatform()
	}
	mapping, _ := p.gatewayService.ResolveChannelMappingAndRestrict(
		c.Request.Context(), req.GroupID, platform, req.Model,
	)
	req.ChannelMapping = &mapping
	return nil
}

// acquireUserSlot increments the wait counter, acquires the user
// concurrency slot, and returns a cleanup func that releases both.
func (p *GatewayPipeline) acquireUserSlot(c *gin.Context, req *ForwardRequest) (func(), error) {
	ctx := c.Request.Context()
	waitCounted, err := p.incrementWaitCount(ctx, req)
	if err != nil {
		return nil, err
	}
	slot, err := p.concurrency.AcquireUserSlot(ctx, req.UserID, req.Concurrency)
	if err != nil {
		if waitCounted {
			p.concurrency.DecrementWaitCount(ctx, req.UserID)
		}
		return nil, fmt.Errorf("gateway: acquire user slot: %w", err)
	}
	// User slot acquired; no longer in wait queue.
	if waitCounted {
		p.concurrency.DecrementWaitCount(ctx, req.UserID)
	}
	release := slot.ReleaseFunc
	return func() {
		if release != nil {
			release()
		}
	}, nil
}

// incrementWaitCount increments the user wait counter. Returns true if
// the counter was incremented (caller must decrement later).
func (p *GatewayPipeline) incrementWaitCount(ctx context.Context, req *ForwardRequest) (bool, error) {
	maxWait := service.CalculateMaxWait(req.Concurrency)
	canWait, err := p.concurrency.IncrementWaitCount(ctx, req.UserID, maxWait)
	if err != nil {
		slog.Warn("pipeline.user_wait_counter_increment_failed", "error", err)
		return false, nil // on error, allow request to proceed
	}
	if !canWait {
		return false, errors.New("gateway: user wait queue full")
	}
	return true, nil
}

// prepareBilling runs the first phase of the two-phase billing check
// and stores the BillingTicket on req.
func (p *GatewayPipeline) prepareBilling(c *gin.Context, req *ForwardRequest) error {
	var group *service.Group
	if req.APIKey != nil {
		group = req.APIKey.Group
	}
	channelID := int64(0)
	if req.ChannelMapping != nil {
		channelID = req.ChannelMapping.ChannelID
	}
	ticket, err := p.billingCache.PrepareBillingCheckForRequest(
		c.Request.Context(), req.User, req.APIKey, group, req.Subscription,
		service.ServiceQuotaCheckRequest{Model: req.Model, ChannelID: channelID},
	)
	if err != nil {
		return fmt.Errorf("gateway: billing check: %w", err)
	}
	req.BillingTicket = ticket
	return nil
}

// resolveSessionHash generates the session hash for sticky routing
// and looks up any previously bound account.
func (p *GatewayPipeline) resolveSessionHash(c *gin.Context, req *ForwardRequest) {
	// TODO [M3]: GenerateSessionHash requires *service.ParsedRequest,
	// which differs from ForwardRequest. Phase 1 M3 needs a bridging
	// adapter or the Pipeline caller sets SessionHash before Execute.
	// For now, leave SessionHash as set by the parse callback.
	ctx := c.Request.Context()
	if req.SessionHash == "" {
		return
	}
	cachedID, err := p.gatewayService.GetCachedSessionAccountID(ctx, req.GroupID, req.SessionHash)
	if err != nil {
		slog.Debug("pipeline.sticky_session_lookup_failed",
			"session_hash", req.SessionHash,
			"error", err,
		)
	}
	if cachedID > 0 {
		slog.Debug("pipeline.sticky_session_hit",
			"session_hash", req.SessionHash,
			"bound_account_id", cachedID,
		)
	}
}

// selectAndForward runs the select-account -> forward -> failover loop.
// It iterates up to maxFailovers times, selecting a new account on each
// failover.
func (p *GatewayPipeline) selectAndForward(
	ctx context.Context,
	w http.ResponseWriter,
	req *ForwardRequest,
) (*ForwardResult, error) {
	excludedIDs := make(map[int64]struct{})

	for i := 0; i < p.maxFailovers; i++ {
		result, done, err := p.tryOneAccount(ctx, w, req, excludedIDs)
		if done {
			return result, err
		}
		// Provider signaled failover; the account ID was already
		// added to excludedIDs by tryOneAccount.
		req.SwitchCount++
	}
	return nil, errors.New("gateway: failover limit reached, no account succeeded")
}

// tryOneAccount performs a single select -> slot -> consume -> forward
// attempt. Returns (result, true, nil) on success, (nil, true, err) on
// terminal failure, (nil, false, nil) when the caller should retry with
// the next account.
func (p *GatewayPipeline) tryOneAccount(
	ctx context.Context,
	w http.ResponseWriter,
	req *ForwardRequest,
	excludedIDs map[int64]struct{},
) (*ForwardResult, bool, error) {
	account, releaseFunc, err := p.selectAccount(ctx, req, excludedIDs)
	if err != nil {
		return nil, true, fmt.Errorf("gateway: select account: %w", err)
	}
	req.Account = account

	// Ensure the account slot is released on any exit path.
	defer func() {
		if releaseFunc != nil {
			releaseFunc()
		}
	}()

	if err := p.consumeBilling(ctx, req); err != nil {
		return nil, true, err
	}

	result, err := p.forwardToProvider(ctx, w, req)
	if err != nil {
		return p.handleForwardError(ctx, req, excludedIDs, err)
	}
	return result, true, nil
}

// selectAccount calls GatewayService.SelectAccountWithLoadAwareness
// and returns the selected account plus its release func.
func (p *GatewayPipeline) selectAccount(
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
	account := selection.Account

	// If slot not yet acquired and a wait plan exists, acquire it.
	releaseFunc := selection.ReleaseFunc
	if !selection.Acquired && selection.WaitPlan != nil {
		slot, err := p.concurrency.AcquireAccountSlot(
			ctx, account.ID, selection.WaitPlan.MaxConcurrency,
		)
		if err != nil {
			return nil, nil, fmt.Errorf("account slot: %w", err)
		}
		releaseFunc = slot.ReleaseFunc

		// Bind sticky session after wait-based acquire.
		_ = p.gatewayService.BindStickySession(ctx, req.GroupID, req.SessionHash, account.ID)
	}
	return account, releaseFunc, nil
}

// consumeBilling runs the second phase of the billing ticket (Consume).
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

// forwardToProvider looks up the registered GatewayProvider for the
// account's platform and calls Forward.
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

// handleForwardError inspects the forward error and decides whether to
// failover or return a terminal error.
func (p *GatewayPipeline) handleForwardError(
	ctx context.Context,
	req *ForwardRequest,
	excludedIDs map[int64]struct{},
	err error,
) (*ForwardResult, bool, error) {
	// If upstream already started writing (UpstreamAccepted), we must
	// not failover because partial response is already on the wire.
	if req.UpstreamAccepted {
		return nil, true, fmt.Errorf("gateway: forward failed after upstream accepted: %w", err)
	}

	provider, ok := p.registry.Get(req.Account.Platform)
	if !ok {
		return nil, true, err
	}

	if provider.ShouldFailover(ctx, req, err) {
		excludedIDs[req.Account.ID] = struct{}{}
		slog.Info("pipeline.failover",
			"account_id", req.Account.ID,
			"platform", req.Account.Platform,
			"switch_count", req.SwitchCount+1,
			"error", err,
		)
		return nil, false, nil // signal retry
	}
	return nil, true, err
}

// recordUsage invokes the handler-injected RecordUsageFunc.
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
