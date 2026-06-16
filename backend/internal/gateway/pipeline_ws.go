package gateway

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// WSForwardFunc is called after pre-flight completes. It owns the WS relay
// lifecycle. account and token are the selected account and its access token.
// hooks must be called at turn boundaries by the relay.
type WSForwardFunc func(ctx context.Context, account *service.Account, token string, hooks *WSSessionHooks) error

// WSSessionHooks are pipeline-managed turn lifecycle callbacks.
type WSSessionHooks struct {
	// BeforeTurn is called before each turn (turn >= 2 for re-acquisition).
	// For turn 1, pre-flight already acquired slots.
	BeforeTurn func(turn int) error
	// BeforeTurnContent runs content interception against a subsequent turn's
	// payload (turn >= 2). Turn 1 is already moderated during pre-flight, so
	// the relay only invokes this for later turns, supplying the per-turn
	// payload the BeforeTurn(turn int) signature cannot carry. A non-nil error
	// (a *ContentBlockedError) aborts the turn.
	BeforeTurnContent func(turn int, payload []byte) error
	// AfterTurn is called after each turn completes (usage recording, slot release).
	AfterTurn func(turn int, result *ForwardResult, turnErr error)
}

// WSParseFunc extracts request info from the first WS frame.
type WSParseFunc func(firstFrame []byte) (*ForwardRequest, error)

// AccountInfo carries what the WSForwardFunc needs about the selected account.
type AccountInfo struct {
	Account     *service.Account
	AccessToken string
}

// ExecuteWS runs the WebSocket gateway request lifecycle. Unlike Execute
// (which loops with failover), ExecuteWS selects one account, acquires all
// slots, then delegates the full WS session to the forward callback. Turn
// re-acquisition is handled via WSSessionHooks.
func (p *GatewayPipeline) ExecuteWS(
	c *gin.Context,
	protocol string,
	forcePlatform string,
	parse WSParseFunc,
	forward WSForwardFunc,
	record RecordUsageFunc,
) error {
	// --- Phase 1: pre-flight (auth, channel mapping, user slot, billing) ---
	req, userSlotRelease, err := p.executeWSPreFlight(c, protocol, forcePlatform, parse)
	if err != nil {
		return err
	}

	// Track current slot releases for turn lifecycle management.
	var currentUserRelease func()
	var currentAccountRelease func()
	currentUserRelease = userSlotRelease

	releaseTurnSlots := func() {
		if currentAccountRelease != nil {
			currentAccountRelease()
			currentAccountRelease = nil
		}
		if currentUserRelease != nil {
			currentUserRelease()
			currentUserRelease = nil
		}
	}
	defer releaseTurnSlots()
	defer req.BillingTicket.Close()

	// --- Phases 2-6: account selection through WS relay ---
	return p.wsSelectAndForward(
		c, req, protocol,
		&currentUserRelease, &currentAccountRelease,
		releaseTurnSlots, forward, record,
	)
}

// wsSelectAndForward runs phases 2-6 of the WS lifecycle: account selection,
// billing consume, access token retrieval, session hook setup, and relay
// delegation. The release pointers are shared with the caller's deferred
// cleanup so slot ownership transfers correctly across WS turns.
func (p *GatewayPipeline) wsSelectAndForward(
	c *gin.Context,
	req *ForwardRequest,
	protocol string,
	currentUserRelease *func(),
	currentAccountRelease *func(),
	releaseTurnSlots func(),
	forward WSForwardFunc,
	record RecordUsageFunc,
) error {
	ctx := c.Request.Context()

	// --- Phase 2: account selection (single, no failover loop) ---
	account, accountRelease, err := p.selectAccountGeneric(ctx, req, nil)
	if err != nil {
		return fmt.Errorf("gateway/ws: select account: %w", err)
	}
	req.Account = account
	*currentAccountRelease = wrapReleaseOnDone(ctx, accountRelease)
	setOpsSelectedAccount(c, account.ID, account.Platform)

	// --- Phase 3: billing consume ---
	if err := p.consumeBilling(ctx, req); err != nil {
		return err
	}

	// --- Phase 4: get access token ---
	token, _, err := p.gatewayService.GetAccessToken(ctx, account)
	if err != nil {
		return fmt.Errorf("gateway/ws: get access token: %w", err)
	}

	// --- Phase 5: build session hooks ---
	hooks := p.buildWSSessionHooks(
		c, ctx, req, account, p.resolveAccountMaxConcurrency(account),
		currentUserRelease, currentAccountRelease,
		releaseTurnSlots, record,
	)

	// --- Phase 6: delegate to WS relay ---
	slog.Debug("pipeline.ws_session_starting",
		"account_id", account.ID,
		"model", req.Model,
		"protocol", protocol,
	)
	return forward(ctx, account, token, hooks)
}

// executeWSPreFlight is the WS-specific pre-flight that accepts a WSParseFunc
// instead of ParseRequestFunc. It adapts the WS parse function to the
// pipeline's standard executePreFlight.
func (p *GatewayPipeline) executeWSPreFlight(
	c *gin.Context,
	protocol, forcePlatform string,
	parse WSParseFunc,
) (*ForwardRequest, func(), error) {
	// Adapt WSParseFunc to ParseRequestFunc. The WS handler has already
	// read the first frame and passes it via closure; the pipeline's
	// readAndParse will pass whatever body it reads (possibly empty for
	// WS since the handler pre-read it).
	adapted := func(body []byte) (*ForwardRequest, error) {
		return parse(body)
	}
	return p.executePreFlight(c, protocol, forcePlatform, adapted)
}

// resolveAccountMaxConcurrency determines the effective max concurrency for
// an account, matching the logic in the handler.
func (p *GatewayPipeline) resolveAccountMaxConcurrency(account *service.Account) int {
	if account == nil {
		return 0
	}
	return account.Concurrency
}

// buildWSSessionHooks creates WSSessionHooks that manage slot re-acquisition
// and usage recording across WS turns.
func (p *GatewayPipeline) buildWSSessionHooks(
	c *gin.Context,
	ctx context.Context,
	req *ForwardRequest,
	account *service.Account,
	accountMaxConcurrency int,
	currentUserRelease *func(),
	currentAccountRelease *func(),
	releaseTurnSlots func(),
	record RecordUsageFunc,
) *WSSessionHooks {
	return &WSSessionHooks{
		BeforeTurn: func(turn int) error {
			if turn <= 1 {
				return nil
			}
			return p.wsReacquireSlots(
				ctx, req, account, accountMaxConcurrency,
				currentUserRelease, currentAccountRelease,
				releaseTurnSlots,
			)
		},
		BeforeTurnContent: func(turn int, payload []byte) error {
			if turn <= 1 {
				return nil
			}
			return p.CheckContentForPayload(c, req, payload)
		},
		AfterTurn: func(turn int, result *ForwardResult, turnErr error) {
			releaseTurnSlots()
			if turnErr != nil || result == nil || account == nil {
				return
			}
			p.wsRecordTurnUsage(ctx, req, account, result, record)
		},
	}
}

// wsReacquireSlots releases old slots and re-acquires user + account slots
// for subsequent WS turns (turn >= 2).
func (p *GatewayPipeline) wsReacquireSlots(
	ctx context.Context,
	req *ForwardRequest,
	account *service.Account,
	accountMaxConcurrency int,
	currentUserRelease *func(),
	currentAccountRelease *func(),
	releaseTurnSlots func(),
) error {
	// Defensive cleanup: release any lingering slots from previous turn.
	releaseTurnSlots()

	// Re-acquire user slot.
	userSlot, err := p.concurrency.AcquireUserSlot(ctx, req.UserID, req.Concurrency)
	if err != nil {
		return fmt.Errorf("gateway/ws: re-acquire user slot: %w", err)
	}
	if !userSlot.Acquired {
		return fmt.Errorf("gateway/ws: user concurrency limit reached")
	}
	*currentUserRelease = wrapReleaseOnDone(ctx, userSlot.ReleaseFunc)

	// Re-acquire account slot.
	accountSlot, err := p.concurrency.AcquireAccountSlot(
		ctx, account.ID, accountMaxConcurrency,
	)
	if err != nil {
		// Release user slot on failure.
		if *currentUserRelease != nil {
			(*currentUserRelease)()
			*currentUserRelease = nil
		}
		return fmt.Errorf("gateway/ws: re-acquire account slot: %w", err)
	}
	if !accountSlot.Acquired {
		if *currentUserRelease != nil {
			(*currentUserRelease)()
			*currentUserRelease = nil
		}
		return fmt.Errorf("gateway/ws: account concurrency limit reached")
	}
	*currentAccountRelease = wrapReleaseOnDone(ctx, accountSlot.ReleaseFunc)

	return nil
}

// wsRecordTurnUsage submits usage recording for a completed WS turn.
func (p *GatewayPipeline) wsRecordTurnUsage(
	ctx context.Context,
	req *ForwardRequest,
	account *service.Account,
	result *ForwardResult,
	record RecordUsageFunc,
) {
	if record == nil || result == nil || account == nil {
		return
	}
	// Record usage asynchronously; errors are logged but not propagated
	// since the WS session should continue.
	go func() {
		if err := record(ctx, account, result); err != nil {
			slog.Warn("pipeline.ws_record_usage_failed",
				"account_id", account.ID,
				"error", err,
			)
		}
	}()
}
