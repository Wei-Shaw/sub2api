package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/gemini"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

type openAICompatModelList struct {
	Object string                    `json:"object"`
	Data   []openAICompatModelObject `json:"data"`
}

type openAICompatModelObject struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type geminiOpenAICompatibleUnaryOptions struct {
	Component     string
	RequestType   int16
	BeforeForward func(*gin.Context, *service.APIKey, servermiddleware.AuthSubject, string, []byte) bool
	Forward       func(context.Context, *gin.Context, *service.Account, []byte) (*service.ForwardResult, error)
}

func ensureGeminiOpenAICompatibleGroup(c *gin.Context) bool {
	apiKey, ok := servermiddleware.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil || apiKey.Group == nil || apiKey.Group.Platform != service.PlatformGemini {
		geminiOpenAICompatError(c, http.StatusBadRequest, "invalid_request_error", "The /v1beta/openai compatibility endpoint requires a Gemini group")
		return false
	}
	return true
}

func geminiOpenAICompatError(c *gin.Context, status int, errType string, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
	c.Abort()
}

func geminiModelNameToOpenAIID(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "models/")
	return name
}

func geminiModelsToOpenAIModelList(src gemini.ModelsListResponse) openAICompatModelList {
	out := openAICompatModelList{
		Object: "list",
		Data:   make([]openAICompatModelObject, 0, len(src.Models)),
	}
	for _, model := range src.Models {
		id := geminiModelNameToOpenAIID(model.Name)
		if id == "" {
			continue
		}
		out.Data = append(out.Data, openAICompatModelObject{
			ID:      id,
			Object:  "model",
			Created: 0,
			OwnedBy: "google",
		})
	}
	return out
}

func geminiModelToOpenAIModelObject(model string) openAICompatModelObject {
	return openAICompatModelObject{
		ID:      geminiModelNameToOpenAIID(model),
		Object:  "model",
		Created: 0,
		OwnedBy: "google",
	}
}

func (h *GatewayHandler) GeminiOpenAICompatibleModels(c *gin.Context) {
	if !ensureGeminiOpenAICompatibleGroup(c) {
		return
	}
	c.JSON(http.StatusOK, geminiModelsToOpenAIModelList(gemini.FallbackModelsList()))
}

func (h *GatewayHandler) GeminiOpenAICompatibleGetModel(c *gin.Context) {
	if !ensureGeminiOpenAICompatibleGroup(c) {
		return
	}
	model := strings.TrimSpace(c.Param("model"))
	if model == "" {
		geminiOpenAICompatError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}
	c.JSON(http.StatusOK, geminiModelToOpenAIModelObject(model))
}

func (h *GatewayHandler) GeminiOpenAICompatibleEmbeddings(c *gin.Context) {
	h.handleGeminiOpenAICompatibleUnary(c, geminiOpenAICompatibleUnaryOptions{
		Component:   "handler.gemini_openai.embeddings",
		RequestType: int16(service.RequestTypeSync),
		Forward: func(ctx context.Context, c *gin.Context, account *service.Account, body []byte) (*service.ForwardResult, error) {
			return h.geminiCompatService.ForwardOpenAICompatibleEmbeddings(ctx, c, account, body)
		},
	})
}

func (h *GatewayHandler) GeminiOpenAICompatibleImagesGenerations(c *gin.Context) {
	h.handleGeminiOpenAICompatibleUnary(c, geminiOpenAICompatibleUnaryOptions{
		Component:   "handler.gemini_openai.images",
		RequestType: int16(service.RequestTypeSync),
		BeforeForward: func(c *gin.Context, apiKey *service.APIKey, subject servermiddleware.AuthSubject, model string, body []byte) bool {
			if !service.GroupAllowsImageGeneration(apiKey.Group) {
				geminiOpenAICompatError(c, http.StatusForbidden, "permission_error", service.ImageGenerationPermissionMessage())
				return false
			}
			reqLog := requestLogger(
				c,
				"handler.gemini_openai.images",
				zap.Int64("user_id", subject.UserID),
				zap.Int64("api_key_id", apiKey.ID),
				zap.Any("group_id", apiKey.GroupID),
				zap.String("model", model),
			)
			if decision := h.checkContentModeration(c, reqLog, apiKey, subject, service.ContentModerationProtocolOpenAIImages, model, body); decision != nil && decision.Blocked {
				geminiOpenAICompatError(c, contentModerationStatus(decision), contentModerationErrorCode(decision), decision.Message)
				return false
			}
			return true
		},
		Forward: func(ctx context.Context, c *gin.Context, account *service.Account, body []byte) (*service.ForwardResult, error) {
			return h.geminiCompatService.ForwardOpenAICompatibleImagesGenerations(ctx, c, account, body)
		},
	})
}

func (h *GatewayHandler) GeminiOpenAICompatibleUnsupported(c *gin.Context) {
	geminiOpenAICompatError(c, http.StatusNotFound, "invalid_request_error", "Unsupported endpoint for Gemini OpenAI compatibility")
}

func (h *GatewayHandler) handleGeminiOpenAICompatibleUnary(c *gin.Context, opts geminiOpenAICompatibleUnaryOptions) {
	requestStart := time.Now()
	if !ensureGeminiOpenAICompatibleGroup(c) {
		return
	}

	apiKey, ok := servermiddleware.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil {
		geminiOpenAICompatError(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	subject, ok := servermiddleware.GetAuthSubjectFromContext(c)
	if !ok {
		geminiOpenAICompatError(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}
	reqLog := requestLogger(
		c,
		opts.Component,
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
	)

	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			geminiOpenAICompatError(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
			return
		}
		geminiOpenAICompatError(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}
	if len(body) == 0 {
		geminiOpenAICompatError(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return
	}
	if !gjson.ValidBytes(body) {
		geminiOpenAICompatError(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
	}

	modelResult := gjson.GetBytes(body, "model")
	if !modelResult.Exists() || modelResult.Type != gjson.String || strings.TrimSpace(modelResult.String()) == "" {
		geminiOpenAICompatError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}
	reqModel := modelResult.String()
	reqLog = reqLog.With(zap.String("model", reqModel))
	setOpsRequestContext(c, reqModel, false)
	setOpsEndpointContext(c, "", opts.RequestType)

	if opts.BeforeForward != nil && !opts.BeforeForward(c, apiKey, subject, reqModel, body) {
		return
	}

	if h.geminiCompatService == nil {
		geminiOpenAICompatError(c, http.StatusBadGateway, "upstream_error", "Gemini compatibility service is not configured")
		return
	}

	channelMapping, _ := h.gatewayService.ResolveChannelMappingAndRestrict(c.Request.Context(), apiKey.GroupID, reqModel)

	if h.errorPassthroughService != nil {
		service.BindErrorPassthroughService(c, h.errorPassthroughService)
	}
	subscription, _ := servermiddleware.GetSubscriptionFromContext(c)
	service.SetOpsLatencyMs(c, service.OpsAuthLatencyMsKey, time.Since(requestStart).Milliseconds())

	streamStarted := false
	maxWait := service.CalculateMaxWait(subject.Concurrency)
	canWait, err := h.concurrencyHelper.IncrementWaitCount(c.Request.Context(), subject.UserID, maxWait)
	waitCounted := false
	if err != nil {
		reqLog.Warn("gemini_openai.user_wait_counter_increment_failed", zap.Error(err))
	} else if !canWait {
		geminiOpenAICompatError(c, http.StatusTooManyRequests, "rate_limit_error", "Too many pending requests, please retry later")
		return
	}
	if err == nil && canWait {
		waitCounted = true
	}
	defer func() {
		if waitCounted {
			h.concurrencyHelper.DecrementWaitCount(c.Request.Context(), subject.UserID)
		}
	}()

	userReleaseFunc, err := h.concurrencyHelper.AcquireUserSlotWithWait(c, subject.UserID, subject.Concurrency, false, &streamStarted)
	if err != nil {
		reqLog.Warn("gemini_openai.user_slot_acquire_failed", zap.Error(err))
		h.handleConcurrencyError(c, err, "user", streamStarted)
		return
	}
	if waitCounted {
		h.concurrencyHelper.DecrementWaitCount(c.Request.Context(), subject.UserID)
		waitCounted = false
	}
	userReleaseFunc = wrapReleaseOnDone(c.Request.Context(), userReleaseFunc)
	if userReleaseFunc != nil {
		defer userReleaseFunc()
	}

	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
		reqLog.Info("gemini_openai.billing_check_failed", zap.Error(err))
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		geminiOpenAICompatError(c, status, code, message)
		return
	}

	fs := NewFailoverState(h.maxAccountSwitchesGemini, false)
	routingStart := time.Now()

	for {
		selection, err := h.gatewayService.SelectAccountWithLoadAwareness(c.Request.Context(), apiKey.GroupID, "", reqModel, fs.FailedAccountIDs, "", int64(0))
		if err != nil {
			if len(fs.FailedAccountIDs) == 0 {
				markOpsRoutingCapacityLimitedIfNoAvailable(c, err)
				geminiOpenAICompatError(c, http.StatusServiceUnavailable, "api_error", "No available Gemini accounts: "+err.Error())
				return
			}
			action := fs.HandleSelectionExhausted(c.Request.Context())
			switch action {
			case FailoverContinue:
				continue
			case FailoverCanceled:
				return
			default:
				if fs.LastFailoverErr != nil {
					h.handleCCFailoverExhausted(c, fs.LastFailoverErr, streamStarted)
				} else {
					geminiOpenAICompatError(c, http.StatusBadGateway, "server_error", "All available Gemini accounts exhausted")
				}
				return
			}
		}
		account := selection.Account
		setOpsSelectedAccount(c, account.ID, account.Platform)

		accountReleaseFunc := selection.ReleaseFunc
		if !selection.Acquired {
			if selection.WaitPlan == nil {
				markOpsRoutingCapacityLimited(c)
				geminiOpenAICompatError(c, http.StatusServiceUnavailable, "api_error", "No available Gemini accounts")
				return
			}
			accountReleaseFunc, err = h.concurrencyHelper.AcquireAccountSlotWithWaitTimeout(
				c,
				account.ID,
				selection.WaitPlan.MaxConcurrency,
				selection.WaitPlan.Timeout,
				false,
				&streamStarted,
			)
			if err != nil {
				reqLog.Warn("gemini_openai.account_slot_acquire_failed", zap.Int64("account_id", account.ID), zap.Error(err))
				h.handleConcurrencyError(c, err, "account", streamStarted)
				return
			}
		}
		accountReleaseFunc = wrapReleaseOnDone(c.Request.Context(), accountReleaseFunc)

		if account.Platform != service.PlatformGemini {
			if accountReleaseFunc != nil {
				accountReleaseFunc()
			}
			fs.FailedAccountIDs[account.ID] = struct{}{}
			continue
		}

		service.SetOpsLatencyMs(c, service.OpsRoutingLatencyMsKey, time.Since(routingStart).Milliseconds())
		forwardBody := body
		if channelMapping.Mapped {
			forwardBody = h.gatewayService.ReplaceModelInBody(body, channelMapping.MappedModel)
		}
		writerSizeBeforeForward := c.Writer.Size()
		forwardStart := time.Now()
		result, err := opts.Forward(c.Request.Context(), c, account, forwardBody)
		forwardDurationMs := time.Since(forwardStart).Milliseconds()
		upstreamLatencyMs, _ := getContextInt64(c, service.OpsUpstreamLatencyMsKey)
		responseLatencyMs := forwardDurationMs
		if upstreamLatencyMs > 0 && forwardDurationMs > upstreamLatencyMs {
			responseLatencyMs = forwardDurationMs - upstreamLatencyMs
		}
		service.SetOpsLatencyMs(c, service.OpsResponseLatencyMsKey, responseLatencyMs)
		if accountReleaseFunc != nil {
			accountReleaseFunc()
		}

		if err != nil {
			var failoverErr *service.UpstreamFailoverError
			if errors.As(err, &failoverErr) {
				if c.Writer.Size() != writerSizeBeforeForward {
					h.handleCCFailoverExhausted(c, failoverErr, true)
					return
				}
				action := fs.HandleFailoverError(c.Request.Context(), h.gatewayService, account.ID, account.Platform, failoverErr)
				switch action {
				case FailoverContinue:
					continue
				case FailoverExhausted:
					h.handleCCFailoverExhausted(c, fs.LastFailoverErr, streamStarted)
					return
				case FailoverCanceled:
					return
				}
			}
			h.ensureForwardErrorResponse(c, streamStarted)
			reqLog.Error("gemini_openai.forward_failed", zap.Int64("account_id", account.ID), zap.Error(err))
			return
		}

		userAgent := c.GetHeader("User-Agent")
		clientIP := ip.GetClientIP(c)
		requestPayloadHash := service.HashUsageRequestPayload(body)
		inboundEndpoint := GetInboundEndpoint(c)
		upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)
		quotaPlatform := service.QuotaPlatform(c.Request.Context(), apiKey)
		h.submitUsageRecordTask(c.Request.Context(), func(ctx context.Context) {
			if err := h.gatewayService.RecordUsage(ctx, &service.RecordUsageInput{
				Result:             result,
				QuotaPlatform:      quotaPlatform,
				APIKey:             apiKey,
				User:               apiKey.User,
				Account:            account,
				Subscription:       subscription,
				InboundEndpoint:    inboundEndpoint,
				UpstreamEndpoint:   upstreamEndpoint,
				UserAgent:          userAgent,
				IPAddress:          clientIP,
				RequestPayloadHash: requestPayloadHash,
				APIKeyService:      h.apiKeyService,
				ChannelUsageFields: channelMapping.ToUsageFields(reqModel, result.UpstreamModel),
			}); err != nil {
				logger.L().With(
					zap.String("component", opts.Component),
					zap.Int64("user_id", subject.UserID),
					zap.Int64("api_key_id", apiKey.ID),
					zap.Any("group_id", apiKey.GroupID),
					zap.String("model", reqModel),
					zap.Int64("account_id", account.ID),
				).Error("gemini_openai.record_usage_failed", zap.Error(err))
			}
		})
		return
	}
}
