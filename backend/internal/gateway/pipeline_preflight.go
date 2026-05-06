package gateway

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// executePreFlight runs parse → channel mapping → user slot → billing
// check → session hash. Returns the request, a slot-release func, and
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
	return nil
}

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
	}
}
