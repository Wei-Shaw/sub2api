package gateway

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// executePreFlight runs parse 鈫?channel mapping 鈫?user slot 鈫?billing
// check 鈫?session hash. Returns the request, a slot-release func, and
// any error. Caller must defer slotRelease() on success.
func (p *GatewayPipeline) executePreFlight(
	c *gin.Context,
	protocol, forcePlatform string,
	parse ParseRequestFunc,
) (*ForwardRequest, func(), error) {
	req, err := p.readAndParse(c, protocol, forcePlatform, parse)
	if err != nil {
		return nil, nil, err
	}
	if err := p.resolveChannelMapping(c, req); err != nil {
		return nil, nil, err
	}
	slotRelease, err := p.acquireUserSlot(c, req)
	if err != nil {
		return nil, nil, err
	}
	if err := p.prepareBilling(c, req); err != nil {
		slotRelease()
		return nil, nil, err
	}
	p.resolveSessionHash(c, req)
	p.setOpsContext(c, req)
	if err := p.checkContentIntercept(c, req); err != nil {
		slotRelease()
		return nil, nil, err
	}
	return req, slotRelease, nil
}

func (p *GatewayPipeline) readAndParse(
	c *gin.Context,
	protocol, forcePlatform string,
	parse ParseRequestFunc,
) (*ForwardRequest, error) {
	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		return nil, fmt.Errorf("gateway: read body: %w", err)
	}
	// Body may be empty when the handler pre-read it (e.g. Messages,
	// CountTokens). In that case the ParseRequestFunc supplies its own
	// copy via the closure; we pass whatever we got and let it decide.
	req, err := parse(body)
	if err != nil {
		return nil, fmt.Errorf("gateway: parse request: %w", err)
	}
	req.Protocol = protocol
	req.ForcePlatform = forcePlatform
	// Only overwrite RawBody when the pipeline actually read data.
	// Pre-read handlers set RawBody in the ParseRequestFunc; clobbering
	// it with an empty slice would break forwarding.
	if len(body) > 0 {
		req.RawBody = body
	}
	p.extractAuthContext(c, req)
	return req, nil
}

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

func (p *GatewayPipeline) resolveChannelMapping(c *gin.Context, req *ForwardRequest) error {
	platform := ""
	if req.APIKey != nil {
		platform = req.APIKey.GroupPlatform()
	}
	mapping, _ := p.gatewayService.ResolveChannelMappingAndRestrict(
		c.Request.Context(), req.GroupID, platform, req.Model,
	)
	req.ChannelMapping = &mapping
	if mapping.Mapped {
		req.ChannelMappedModel = mapping.MappedModel
	}
	return nil
}

// Slot acquisition constants matching handler/gateway_helper.go.
const (
	slotMaxWait        = 30 * time.Second
	slotPingInterval   = 10 * time.Second
	slotInitialBackoff = 100 * time.Millisecond
	slotBackoffMult    = 1.5
	slotMaxBackoff     = 2 * time.Second
	ssePingClaude      = "data: {\"type\": \"ping\"}\n\n"
	ssePingComment     = ":\n\n"
)

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

	if slot.Acquired {
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

	// Slot not immediately available 鈥?wait with SSE ping for streaming requests
	release, err := p.waitForUserSlotWithPing(c, req)
	if waitCounted {
		p.concurrency.DecrementWaitCount(ctx, req.UserID)
	}
	if err != nil {
		return nil, fmt.Errorf("gateway: acquire user slot: %w", err)
	}
	return func() {
		if release != nil {
			release()
		}
	}, nil
}

// waitForUserSlotWithPing retries slot acquisition with exponential backoff,
// sending SSE ping events for streaming requests to keep the connection alive.
func (p *GatewayPipeline) waitForUserSlotWithPing(c *gin.Context, req *ForwardRequest) (func(), error) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), slotMaxWait)
	defer cancel()

	pingFormat := p.pingFormatForProtocol(req.Protocol)
	needPing := req.Stream && pingFormat != ""

	var flusher http.Flusher
	if needPing {
		var ok bool
		flusher, ok = c.Writer.(http.Flusher)
		if !ok {
			needPing = false
		}
	}

	var pingCh <-chan time.Time
	if needPing {
		pingTicker := time.NewTicker(slotPingInterval)
		defer pingTicker.Stop()
		pingCh = pingTicker.C
	}

	streamStarted := false
	backoff := slotInitialBackoff
	timer := time.NewTimer(backoff)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, errors.New("timeout waiting for user concurrency slot")

		case <-pingCh:
			if !streamStarted {
				c.Header("Content-Type", "text/event-stream")
				c.Header("Cache-Control", "no-cache")
				c.Header("Connection", "keep-alive")
				c.Header("X-Accel-Buffering", "no")
				streamStarted = true
			}
			if _, err := fmt.Fprint(c.Writer, pingFormat); err != nil {
				return nil, err
			}
			flusher.Flush()

		case <-timer.C:
			slot, err := p.concurrency.AcquireUserSlot(ctx, req.UserID, req.Concurrency)
			if err != nil {
				return nil, err
			}
			if slot.Acquired {
				return slot.ReleaseFunc, nil
			}
			backoff = nextSlotBackoff(backoff)
			timer.Reset(backoff)
		}
	}
}

// pingFormatForProtocol returns the SSE ping format string for the given protocol.
func (p *GatewayPipeline) pingFormatForProtocol(protocol string) string {
	switch protocol {
	case ProtocolAnthropic, ProtocolAnthropicViaOpenAI:
		return ssePingClaude
	case ProtocolChatCompletions, ProtocolResponses, ProtocolOpenAI, ProtocolImages:
		return ssePingComment
	default:
		return ""
	}
}

// nextSlotBackoff computes the next backoff duration with jitter.
func nextSlotBackoff(current time.Duration) time.Duration {
	next := time.Duration(float64(current) * slotBackoffMult)
	if next > slotMaxBackoff {
		next = slotMaxBackoff
	}
	jitter := 0.8 + rand.Float64()*0.4
	jittered := time.Duration(float64(next) * jitter)
	if jittered < slotInitialBackoff {
		return slotInitialBackoff
	}
	if jittered > slotMaxBackoff {
		return slotMaxBackoff
	}
	return jittered
}

func (p *GatewayPipeline) incrementWaitCount(ctx context.Context, req *ForwardRequest) (bool, error) {
	maxWait := service.CalculateMaxWait(req.Concurrency)
	canWait, err := p.concurrency.IncrementWaitCount(ctx, req.UserID, maxWait)
	if err != nil {
		slog.Warn("pipeline.user_wait_counter_increment_failed", "error", err)
		return false, nil
	}
	if !canWait {
		return false, errors.New("gateway: user wait queue full")
	}
	return true, nil
}

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

func (p *GatewayPipeline) resolveSessionHash(c *gin.Context, req *ForwardRequest) {
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
		req.IsStickySession = true
		groupID := int64(0)
		if req.GroupID != nil {
			groupID = *req.GroupID
		}
		ctx = service.WithPrefetchedStickySession(ctx, cachedID, groupID, false)
		c.Request = c.Request.WithContext(ctx)
	}
}

// Ops context key constants matching handler/ops_error_logger.go.
// Defined here to avoid importing the handler package.
const (
	opsModelKey       = "ops_model"
	opsStreamKey      = "ops_stream"
	opsRequestBodyKey = "ops_request_body"
	opsAccountIDKey   = "ops_account_id"
	opsRequestTypeKey = "ops_request_type"
)

// setOpsContext populates gin.Context keys required by the ops error
// logging middleware. These values are normally set by the legacy
// handler functions (setOpsRequestContext / setOpsEndpointContext);
// the pipeline sets them in a single pass after pre-flight completes.
func (p *GatewayPipeline) setOpsContext(c *gin.Context, req *ForwardRequest) {
	if c == nil || req == nil {
		return
	}
	model := strings.TrimSpace(req.Model)
	c.Set(opsModelKey, model)
	c.Set(opsStreamKey, req.Stream)
	if len(req.RawBody) > 0 {
		c.Set(opsRequestBodyKey, req.RawBody)
	}
	reqType := int16(service.RequestTypeFromLegacy(req.Stream, false))
	c.Set(opsRequestTypeKey, reqType)
	// Set model in request context for structured logging
	if c.Request != nil && model != "" {
		ctx := context.WithValue(c.Request.Context(), ctxkey.Model, model)
		c.Request = c.Request.WithContext(ctx)
	}
}

// setOpsSelectedAccount populates gin.Context keys for the selected
// account, matching the legacy setOpsSelectedAccount function.
// Called from pipeline_forward.go after account selection.
func setOpsSelectedAccount(c *gin.Context, accountID int64, platform string) {
	if c == nil || accountID <= 0 {
		return
	}
	c.Set(opsAccountIDKey, accountID)
	if c.Request != nil {
		ctx := context.WithValue(c.Request.Context(), ctxkey.AccountID, accountID)
		p := strings.TrimSpace(platform)
		if p != "" {
			ctx = context.WithValue(ctx, ctxkey.Platform, p)
		}
		c.Request = c.Request.WithContext(ctx)
	}
}
