package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

// Responses handles OpenAI Responses API endpoint for Anthropic platform groups.
// POST /v1/responses
// This converts Responses API requests to Anthropic format, forwards to Anthropic
// upstream, and converts responses back to Responses format.
func (h *GatewayHandler) Responses(c *gin.Context) {
	streamStarted := false

	requestStart := time.Now()

	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		h.responsesErrorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}

	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.responsesErrorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}
	reqLog := requestLogger(
		c,
		"handler.gateway.responses",
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
	)

	// Read request body
	body, err := readLenientJSONRequestBodyWithPrealloc(c.Request, h.cfg)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			h.responsesErrorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
			return
		}
		h.responsesErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}

	if len(body) == 0 {
		h.responsesErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return
	}

	setOpsRequestContext(c, "", false)

	// Validate JSON
	if !gjson.ValidBytes(body) {
		logRequestBodyParseFailure(reqLog, body, nil)
		h.responsesErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
	}

	// Extract model and stream using gjson (like OpenAI handler)
	modelResult := gjson.GetBytes(body, "model")
	if !modelResult.Exists() || modelResult.Type != gjson.String || modelResult.String() == "" {
		h.responsesErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}
	reqModel := modelResult.String()
	reqStream, ok := parseOpenAICompatibleStream(body)
	if !ok {
		h.responsesErrorResponse(c, http.StatusBadRequest, "invalid_request_error", invalidStreamFieldTypeMessage)
		return
	}
	reqLog = reqLog.With(zap.String("model", reqModel), zap.Bool("stream", reqStream))

	setOpsRequestContext(c, reqModel, reqStream)
	setOpsEndpointContext(c, "", int16(service.RequestTypeFromLegacy(reqStream, false)))
	requestCtx := c.Request.Context()
	if service.IsImageGenerationIntent("/v1/responses", reqModel, body) {
		requestCtx = service.WithOpenAIImageGenerationIntent(requestCtx)
	}

	// 解析渠道级模型映射
	channelMapping, _ := h.gatewayService.ResolveChannelMappingAndRestrict(requestCtx, apiKey.GroupID, reqModel)

	// Claude Code only restriction:
	// /v1/responses is never a Claude Code endpoint.
	// When claude_code_only is enabled, this endpoint is rejected.
	// The existing service-layer checkClaudeCodeRestriction handles degradation
	// to fallback groups when the Forward path calls SelectAccountForModelWithExclusions.
	// Here we just reject at handler level since /v1/responses clients can't be Claude Code.
	if apiKey.Group != nil && apiKey.Group.ClaudeCodeOnly {
		h.responsesErrorResponse(c, http.StatusForbidden, "permission_error",
			"This group is restricted to Claude Code clients (/v1/messages only)")
		return
	}

	if decision := h.checkContentModeration(c, reqLog, apiKey, subject, service.ContentModerationProtocolOpenAIResponses, reqModel, body); decision != nil && decision.Blocked {
		h.responsesErrorResponse(c, contentModerationStatus(decision), contentModerationErrorCode(decision), decision.Message)
		return
	}

	// Error passthrough binding
	if h.errorPassthroughService != nil {
		service.BindErrorPassthroughService(c, h.errorPassthroughService)
	}

	subscription, _ := middleware2.GetSubscriptionFromContext(c)

	service.SetOpsLatencyMs(c, service.OpsAuthLatencyMsKey, time.Since(requestStart).Milliseconds())

	platform := ""
	if forcePlatform, ok := middleware2.GetForcePlatformFromContext(c); ok {
		platform = forcePlatform
	} else if apiKey.Group != nil {
		platform = apiKey.Group.Platform
	}

	extraSettings := service.ExtraConcurrencyRuntimeSettings{}
	useExtraAdmission := false
	if (platform == service.PlatformAnthropic || platform == service.PlatformGemini || platform == service.PlatformAntigravity) && h.gatewayAdmission != nil && h.settingService != nil {
		extraSettings = h.settingService.GetExtraConcurrencyRuntimeSettings(requestCtx)
		useExtraAdmission = extraSettings.Enabled
	}

	var (
		admission      *RequestAdmission
		extraAdmission *service.GatewayAdmissionSession
	)
	if useExtraAdmission {
		extraAdmission, err = h.gatewayAdmission.Begin(requestCtx, service.GatewayAdmissionRequest{
			UserID:        subject.UserID,
			StandardLimit: subject.Concurrency,
			ExtraLimit:    subject.ExtraConcurrency,
			Settings:      extraSettings,
		})
		if errors.Is(err, service.ErrGatewayAdmissionDraining) {
			useExtraAdmission = false
			err = nil
		}
		if useExtraAdmission && err == nil {
			release := h.concurrencyHelper.withAPIKeySlotFromGin(c, extraAdmission.Close)
			release = wrapReleaseOnDone(c.Request.Context(), release)
			defer release()
		}
	}
	if !useExtraAdmission {
		// Feature-off compatibility is structural: retain the legacy helper path.
		admission, err = h.concurrencyHelper.Begin(c, UserAdmissionRequest{
			UserID:         subject.UserID,
			MaxConcurrency: subject.Concurrency,
			Mode:           AdmissionModeWait,
			Stream:         reqStream,
			StreamStarted:  &streamStarted,
		})
		if err == nil {
			defer admission.Close()
		}
	}
	if err != nil {
		reqLog.Warn("gateway.responses.user_slot_acquire_failed", zap.Error(err))
		h.handleResponsesConcurrencyError(c, err, "user", streamStarted)
		return
	}

	// 2. Re-check billing
	if err := h.billingCacheService.CheckBillingEligibility(requestCtx, apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(requestCtx, apiKey)); err != nil {
		reqLog.Info("gateway.responses.billing_check_failed", zap.Error(err))
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		h.responsesErrorResponse(c, status, code, message)
		return
	}

	// Parse request for session hash
	bodyRef := service.NewRequestBodyRef(body)
	parsedReq, _ := service.ParseGatewayRequest(bodyRef, "responses")
	if parsedReq == nil {
		parsedReq = &service.ParsedRequest{Model: reqModel, Stream: reqStream, Body: bodyRef}
	}
	parsedReq.SessionContext = &service.SessionContext{
		ClientIP:  ip.GetClientIP(c),
		UserAgent: c.GetHeader("User-Agent"),
		APIKeyID:  apiKey.ID,
	}
	sessionHash := h.gatewayService.GenerateSessionHash(parsedReq)

	// 3. Account selection + failover loop
	fs := NewFailoverState(h.maxAccountSwitches, false)

	for {
		var (
			selection      *service.AccountSelectionResult
			admittedTarget *service.AdmittedTarget
		)
		if useExtraAdmission {
			admittedTarget, err = extraAdmission.NextTarget(requestCtx, service.GatewayTargetRequest{
				GroupID:            apiKey.GroupID,
				SessionKey:         sessionHash,
				Model:              reqModel,
				MetadataUserID:     parsedReq.MetadataUserID,
				ExcludedAccountIDs: fs.FailedAccountIDs,
			})
		} else {
			selection, err = h.gatewayService.SelectAccountWithLoadAwareness(requestCtx, apiKey.GroupID, sessionHash, reqModel, fs.FailedAccountIDs, "", int64(0))
		}
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			if useExtraAdmission && isGatewayAdmissionConcurrencyError(err) {
				reqLog.Warn("gateway.responses.extra_admission_failed", zap.Error(err))
				h.handleResponsesConcurrencyError(c, err, "account", streamStarted)
				return
			}
			if len(fs.FailedAccountIDs) == 0 {
				cls := classifyNoAccountErrorFromGin(c, h.gatewayService, apiKey, reqModel, reqModel, service.PlatformAnthropic)
				if !cls.ModelNotFound {
					markOpsRoutingCapacityLimitedIfNoAvailable(c, err)
				}
				message := cls.Message
				if !cls.ModelNotFound {
					message = "No available accounts: " + err.Error()
				}
				h.responsesErrorResponse(c, cls.Status, cls.ErrType, message)
				return
			}
			action := fs.HandleSelectionExhausted(requestCtx)
			switch action {
			case FailoverContinue:
				continue
			case FailoverCanceled:
				return
			default:
				if fs.LastFailoverErr != nil {
					h.handleResponsesFailoverExhausted(c, fs.LastFailoverErr, streamStarted)
				} else {
					h.responsesErrorResponse(c, http.StatusBadGateway, "server_error", "All available accounts exhausted")
				}
				return
			}
		}
		var account *service.Account
		if useExtraAdmission {
			account = admittedTarget.Account
		} else {
			account = selection.Account
		}
		if !useExtraAdmission {
			setOpsSelectedAccount(c, account.ID, account.Platform)
		}

		// 4. Acquire account concurrency slot. NextTarget owns it on the new path.
		if !useExtraAdmission {
			if err := admission.AdmitAccount(AccountAdmissionRequest{
				Selection:  selection,
				WaitPolicy: AccountWaitPolicyUntracked,
			}); err != nil {
				var unavailableErr *AccountUnavailableError
				if errors.As(err, &unavailableErr) {
					markOpsRoutingCapacityLimited(c)
					h.responsesErrorResponse(c, http.StatusServiceUnavailable, "api_error", "No available accounts")
					return
				}
				reqLog.Warn("gateway.responses.account_slot_acquire_failed", zap.Int64("account_id", account.ID), zap.Error(err))
				h.handleResponsesConcurrencyError(c, err, "account", streamStarted)
				return
			}
		}

		// 5. Forward request
		writerSizeBeforeForward := c.Writer.Size()
		forwardBody := body
		if channelMapping.Mapped {
			forwardBody = h.gatewayService.ReplaceModelInBody(body, channelMapping.MappedModel)
		}
		var result *service.ForwardResult
		sameAccountRetryCanceled := false
		forwardSameAccount := func(ctx context.Context, targetAccount *service.Account) error {
			for {
				writerSizeBeforeAttempt := c.Writer.Size()
				result, err = h.gatewayService.ForwardAsResponses(ctx, c, targetAccount, forwardBody, parsedReq)
				if err == nil {
					return nil
				}
				var failoverErr *service.UpstreamFailoverError
				if !errors.As(err, &failoverErr) || c.Writer.Size() != writerSizeBeforeAttempt {
					return err
				}
				retry, canceled := fs.TrySameAccountRetry(ctx, targetAccount.ID, failoverErr)
				if canceled {
					sameAccountRetryCanceled = true
					return ctx.Err()
				}
				if !retry {
					return err
				}
			}
		}
		var billingRecheckErr error
		if useExtraAdmission {
			_, err = admittedTarget.DispatchPrepared(
				requestCtx,
				nil,
				func(ctx context.Context) error {
					billingRecheckErr = h.billingCacheService.RecheckBillingEligibility(
						ctx,
						apiKey.User,
						apiKey,
						apiKey.Group,
						subscription,
						service.QuotaPlatform(ctx, apiKey),
					)
					return billingRecheckErr
				},
				func(ctx context.Context, targetAccount *service.Account) error {
					account = targetAccount
					ctx = setOpsSelectedAccountContext(c, ctx, targetAccount.ID, targetAccount.Platform)
					return forwardSameAccount(ctx, targetAccount)
				},
			)
			if admittedTarget.Account != nil {
				account = admittedTarget.Account
			}
		} else {
			err = func() error {
				defer admission.ReleaseAccount()
				return forwardSameAccount(requestCtx, account)
			}()
		}
		if sameAccountRetryCanceled || requestCtx.Err() != nil {
			return
		}
		if billingRecheckErr != nil {
			reqLog.Info("gateway.responses.billing_eligibility_recheck_failed", zap.Error(billingRecheckErr))
			status, code, message, retryAfter := billingErrorDetails(billingRecheckErr)
			if retryAfter > 0 {
				c.Header("Retry-After", strconv.Itoa(retryAfter))
			}
			h.responsesErrorResponse(c, status, code, message)
			return
		}
		if useExtraAdmission && isGatewayAdmissionConcurrencyError(err) {
			reqLog.Warn("gateway.responses.extra_admission_dispatch_failed", zap.Error(err))
			h.handleResponsesConcurrencyError(c, err, "account", streamStarted)
			return
		}

		if err != nil {
			var failoverErr *service.UpstreamFailoverError
			if errors.As(err, &failoverErr) {
				// Can't failover if streaming content already sent
				if c.Writer.Size() != writerSizeBeforeForward {
					h.handleResponsesFailoverExhausted(c, failoverErr, true)
					return
				}
				action := fs.HandleFailoverError(requestCtx, h.gatewayService, account.ID, account.Platform, failoverErr)
				switch action {
				case FailoverContinue:
					continue
				case FailoverExhausted:
					h.handleResponsesFailoverExhausted(c, fs.LastFailoverErr, streamStarted)
					return
				case FailoverCanceled:
					return
				}
			}
			upstreamErrorAlreadyCommunicated := gatewayForwardErrorAlreadyCommunicated(c, writerSizeBeforeForward, err)
			wroteFallback := false
			if !upstreamErrorAlreadyCommunicated {
				wroteFallback = h.ensureForwardErrorResponse(c, streamStarted)
			}
			reqLog.Error("gateway.responses.forward_failed",
				zap.Int64("account_id", account.ID),
				zap.Bool("fallback_error_response_written", wroteFallback),
				zap.Bool("upstream_error_response_already_written", upstreamErrorAlreadyCommunicated),
				zap.Error(err),
			)
			return
		}

		// 6. Record usage
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
				reqLog.Error("gateway.responses.record_usage_failed",
					zap.Int64("account_id", account.ID),
					zap.Error(err),
				)
			}
		})
		return
	}
}

// responsesErrorResponse writes an error in OpenAI Responses API format.
func (h *GatewayHandler) responsesErrorResponse(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	})
}

func (h *GatewayHandler) handleResponsesConcurrencyError(c *gin.Context, err error, slotType string, streamStarted bool) {
	status, code, message := concurrencyErrorResponse(err, slotType)
	if streamStarted {
		h.handleStreamingAwareError(c, status, code, message, true)
		return
	}
	h.responsesErrorResponse(c, status, code, message)
}

// handleResponsesFailoverExhausted writes a failover-exhausted error in Responses format.
func (h *GatewayHandler) handleResponsesFailoverExhausted(c *gin.Context, lastErr *service.UpstreamFailoverError, streamStarted bool) {
	if streamStarted {
		return // Can't write error after stream started
	}
	statusCode := http.StatusBadGateway
	if lastErr != nil && lastErr.StatusCode > 0 {
		statusCode = lastErr.StatusCode
	}
	if lastErr != nil && service.IsOpenAISilentRefusalErrorBody(lastErr.ResponseBody) {
		service.SetOpsUpstreamError(c, statusCode, service.OpenAISilentRefusalClientMessage(), "")
		h.responsesErrorResponse(c, http.StatusBadGateway, "upstream_error", service.OpenAISilentRefusalClientMessage())
		return
	}
	h.responsesErrorResponse(c, statusCode, "server_error", "All available accounts exhausted")
}
