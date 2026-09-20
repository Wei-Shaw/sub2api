package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/typesafe"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// SystemOne proxies TypeSafe's native, non-streaming System One protocol.
func (h *GatewayHandler) SystemOne(c *gin.Context) {
	requestStart := time.Now()
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil || apiKey.Group == nil {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}
	reqLog := requestLogger(c, "handler.gateway.systemone",
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
	)

	body, err := readLenientJSONRequestBodyWithPrealloc(c.Request, h.cfg)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			h.errorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
			return
		}
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}
	model, err := typesafe.ValidateSystemOneRequest(body)
	if err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	ensureCompositeTargetPlatform(c, apiKey, model)
	if apiKey.Group.Platform != service.PlatformTypeSafe &&
		(apiKey.Group.Platform != service.PlatformComposite || !compositeTargetPlatformAllowed(c, apiKey, model, service.PlatformTypeSafe)) {
		h.errorResponse(c, http.StatusNotFound, "not_found_error", "System One is only available for TypeSafe and compatible Composite groups")
		return
	}
	if decision := h.checkSecurityAudit(c, reqLog, apiKey, subject, service.ContentModerationProtocolTypeSafeSystemOne, model, body); decision != nil && !decision.AllowNextStage {
		h.anthropicSecurityAuditError(c, decision)
		return
	}

	setOpsRequestContext(c, model, false)
	setOpsEndpointContext(c, "", int16(service.RequestTypeSync))
	service.SetOpsLatencyMs(c, service.OpsAuthLatencyMsKey, time.Since(requestStart).Milliseconds())
	pricingCtx, pricingAt := service.WithGatewayTokenRequestPricing(c.Request.Context())
	c.Request = c.Request.WithContext(pricingCtx)
	channelMapping, _ := h.gatewayService.ResolveChannelMappingAndRestrict(c.Request.Context(), apiKey.GroupID, model)
	subscription, _ := middleware2.GetSubscriptionFromContext(c)

	streamStarted := false
	userRelease, err := h.concurrencyHelper.AcquireUserSlotWithWait(c, subject.UserID, subject.Concurrency, false, &streamStarted)
	if err != nil {
		h.handleConcurrencyError(c, err, "user", false)
		return
	}
	if userRelease != nil {
		defer userRelease()
	}
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		h.errorResponse(c, status, code, message)
		return
	}

	failedAccountIDs := make(map[int64]struct{})
	var lastFailoverErr *service.UpstreamFailoverError
	maxSwitches := h.maxAccountSwitches
	if maxSwitches <= 0 {
		maxSwitches = 3
	}
	for switchCount := 0; ; {
		selection, err := h.gatewayService.SelectAccountWithLoadAwareness(c.Request.Context(), apiKey.GroupID, "", model, failedAccountIDs, "", subject.UserID)
		if err != nil || selection == nil || selection.Account == nil {
			if failoverClientGone(c) {
				return
			}
			if lastFailoverErr != nil {
				h.handleFailoverExhausted(c, lastFailoverErr, service.PlatformTypeSafe, false)
				return
			}
			cls := classifyNoAccountErrorFromGin(c, h.gatewayService, apiKey, model, model, service.PlatformTypeSafe)
			h.errorResponse(c, cls.Status, cls.ErrType, cls.Message)
			return
		}
		account := selection.Account
		setOpsSelectedAccount(c, account.ID, account.Platform)
		service.SetOpsUpstreamModel(c, model)

		accountRelease, err := h.acquireSystemOneAccountSlot(c, selection)
		if err != nil {
			h.handleConcurrencyError(c, err, "account", false)
			return
		}
		forwardStart := time.Now()
		result, forwardErr := func() (*service.SystemOneForwardResult, error) {
			if accountRelease != nil {
				defer accountRelease()
			}
			return h.gatewayService.ForwardSystemOne(c.Request.Context(), c, account, body)
		}()
		service.SetOpsLatencyMs(c, service.OpsResponseLatencyMsKey, time.Since(forwardStart).Milliseconds())

		if forwardErr != nil {
			var failoverErr *service.UpstreamFailoverError
			if errors.As(forwardErr, &failoverErr) {
				if failoverClientGone(c) {
					return
				}
				failedAccountIDs[account.ID] = struct{}{}
				lastFailoverErr = failoverErr
				if switchCount >= maxSwitches {
					h.handleFailoverExhausted(c, failoverErr, service.PlatformTypeSafe, false)
					return
				}
				switchCount++
				reqLog.Warn("systemone.upstream_failover_switching",
					zap.Int64("account_id", account.ID),
					zap.Int("upstream_status", failoverErr.StatusCode),
					zap.Int("switch_count", switchCount),
				)
				continue
			}
			var upstreamErr *service.SystemOneUpstreamError
			if errors.As(forwardErr, &upstreamErr) {
				status := upstreamErr.StatusCode
				if status != http.StatusBadRequest && status != http.StatusUnprocessableEntity {
					status = http.StatusBadGateway
				}
				h.errorResponse(c, status, "upstream_error", "TypeSafe rejected the request")
				return
			}
			reqLog.Warn("systemone.forward_failed", zap.Int64("account_id", account.ID), zap.Error(forwardErr))
			h.errorResponse(c, http.StatusBadGateway, "upstream_error", "TypeSafe upstream request failed")
			return
		}

		c.Data(result.StatusCode, result.ContentType, result.Body)
		h.recordSystemOneUsage(c, apiKey, account, subscription, channelMapping, model, body, result, subject.UserID, pricingAt)
		return
	}
}

func (h *GatewayHandler) acquireSystemOneAccountSlot(c *gin.Context, selection *service.AccountSelectionResult) (func(), error) {
	if selection.Acquired {
		return selection.ReleaseFunc, nil
	}
	if selection.WaitPlan == nil {
		return nil, errors.New("account concurrency unavailable")
	}
	return h.concurrencyHelper.AcquireAccountSlotWithWaitTimeout(c, selection.Account.ID, selection.WaitPlan.MaxConcurrency, selection.WaitPlan.Timeout, false, new(bool))
}

func (h *GatewayHandler) recordSystemOneUsage(c *gin.Context, apiKey *service.APIKey, account *service.Account, subscription *service.UserSubscription, mapping service.ChannelMappingResult, model string, body []byte, result *service.SystemOneForwardResult, userID int64, pricingAt time.Time) {
	userAgent := c.GetHeader("User-Agent")
	clientIP := ip.GetClientIP(c)
	inboundEndpoint := GetInboundEndpoint(c)
	upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)
	quotaPlatform := service.QuotaPlatform(c.Request.Context(), apiKey)
	sessionID := service.ExtractClientSessionID(c)
	requestPayloadHash := service.HashUsageRequestPayload(body)

	h.submitMandatoryUsageRecordTask(c.Request.Context(), func(ctx context.Context) {
		if err := h.gatewayService.RecordUsage(ctx, &service.RecordUsageInput{
			Result:             &result.ForwardResult,
			APIKey:             apiKey,
			User:               apiKey.User,
			Account:            account,
			Subscription:       subscription,
			PricingAt:          pricingAt,
			InboundEndpoint:    inboundEndpoint,
			UpstreamEndpoint:   upstreamEndpoint,
			UserAgent:          userAgent,
			IPAddress:          clientIP,
			SessionID:          sessionID,
			RequestPayloadHash: requestPayloadHash,
			APIKeyService:      h.apiKeyService,
			QuotaPlatform:      quotaPlatform,
			ChannelUsageFields: clientRequestedUsageFields(c, mapping, model, result.UpstreamModel),
		}); err != nil {
			logger.L().With(
				zap.String("component", "handler.gateway.systemone"),
				zap.Int64("user_id", userID),
				zap.Int64("api_key_id", apiKey.ID),
				zap.Int64("account_id", account.ID),
			).Error("systemone.record_usage_failed", zap.Error(err))
		}
	})
}
