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

// ChatCompletions handles OpenAI Chat Completions API endpoint for Anthropic platform groups.
// POST /v1/chat/completions
// This converts Chat Completions requests to Anthropic format (via Responses format chain),
// forwards to Anthropic upstream, and converts responses back to Chat Completions format.
func (h *GatewayHandler) ChatCompletions(c *gin.Context) {
	streamStarted := false

	requestStart := time.Now()

	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		h.chatCompletionsErrorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}

	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.chatCompletionsErrorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}
	reqLog := requestLogger(
		c,
		"handler.gateway.chat_completions",
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
	)

	// Read request body
	body, err := readLenientJSONRequestBodyWithPrealloc(c.Request, h.cfg)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			h.chatCompletionsErrorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
			return
		}
		h.chatCompletionsErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}

	if len(body) == 0 {
		h.chatCompletionsErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return
	}

	setOpsRequestContext(c, "", false)

	// Validate JSON
	if !gjson.ValidBytes(body) {
		logRequestBodyParseFailure(reqLog, body, nil)
		h.chatCompletionsErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
	}

	// Extract model and stream
	modelResult := gjson.GetBytes(body, "model")
	if !modelResult.Exists() || modelResult.Type != gjson.String || modelResult.String() == "" {
		h.chatCompletionsErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}
	reqModel := modelResult.String()
	reqStream, ok := parseOpenAICompatibleStream(body)
	if !ok {
		h.chatCompletionsErrorResponse(c, http.StatusBadRequest, "invalid_request_error", invalidStreamFieldTypeMessage)
		return
	}
	reqLog = reqLog.With(zap.String("model", reqModel), zap.Bool("stream", reqStream))

	setOpsRequestContext(c, reqModel, reqStream)
	setOpsEndpointContext(c, "", int16(service.RequestTypeFromLegacy(reqStream, false)))

	// 解析渠道级模型映射
	channelMapping, _ := h.gatewayService.ResolveChannelMappingAndRestrict(c.Request.Context(), apiKey.GroupID, reqModel)

	// Claude Code only restriction
	if apiKey.Group != nil && apiKey.Group.ClaudeCodeOnly {
		h.chatCompletionsErrorResponse(c, http.StatusForbidden, "permission_error",
			"This group is restricted to Claude Code clients (/v1/messages only)")
		return
	}

	if decision := h.checkContentModeration(c, reqLog, apiKey, subject, service.ContentModerationProtocolOpenAIChat, reqModel, body); decision != nil && decision.Blocked {
		h.chatCompletionsErrorResponse(c, contentModerationStatus(decision), contentModerationErrorCode(decision), decision.Message)
		return
	}

	// Error passthrough binding
	if h.errorPassthroughService != nil {
		service.BindErrorPassthroughService(c, h.errorPassthroughService)
	}

	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	groupPlatform := ""
	if forcePlatform, ok := middleware2.GetForcePlatformFromContext(c); ok {
		groupPlatform = forcePlatform
	} else if apiKey.Group != nil {
		groupPlatform = apiKey.Group.Platform
	}

	service.SetOpsLatencyMs(c, service.OpsAuthLatencyMsKey, time.Since(requestStart).Milliseconds())

	extraSettings := service.ExtraConcurrencyRuntimeSettings{}
	useExtraAdmission := false
	if (groupPlatform == service.PlatformAnthropic || groupPlatform == service.PlatformGemini || groupPlatform == service.PlatformAntigravity) && h.gatewayAdmission != nil && h.settingService != nil {
		extraSettings = h.settingService.GetExtraConcurrencyRuntimeSettings(c.Request.Context())
		useExtraAdmission = extraSettings.Enabled
	}

	var (
		admission      *RequestAdmission
		extraAdmission *service.GatewayAdmissionSession
	)
	if useExtraAdmission {
		extraAdmission, err = h.gatewayAdmission.Begin(c.Request.Context(), service.GatewayAdmissionRequest{
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
		reqLog.Warn("gateway.cc.user_slot_acquire_failed", zap.Error(err))
		h.handleCCConcurrencyError(c, err, "user", streamStarted)
		return
	}

	// 2. Re-check billing
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
		reqLog.Info("gateway.cc.billing_check_failed", zap.Error(err))
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		h.chatCompletionsErrorResponse(c, status, code, message)
		return
	}

	// Parse request for session hash
	bodyRef := service.NewRequestBodyRef(body)
	parsedReq, _ := service.ParseGatewayRequest(bodyRef, "chat_completions")
	if parsedReq == nil {
		parsedReq = &service.ParsedRequest{Model: reqModel, Stream: reqStream, Body: bodyRef}
	}
	parsedReq.SessionContext = &service.SessionContext{
		ClientIP:  ip.GetClientIP(c),
		UserAgent: c.GetHeader("User-Agent"),
		APIKeyID:  apiKey.ID,
	}
	sessionHash := h.gatewayService.GenerateSessionHash(parsedReq)
	selectionSessionHash := sessionHash
	if groupPlatform == service.PlatformGemini && selectionSessionHash != "" {
		selectionSessionHash = "gemini:" + selectionSessionHash
	}

	// 3. Account selection + failover loop
	fs := NewFailoverState(h.maxAccountSwitches, false)
	if groupPlatform == service.PlatformGemini {
		fs = NewFailoverState(h.maxAccountSwitchesGemini, false)
	}

	for {
		var (
			selection      *service.AccountSelectionResult
			admittedTarget *service.AdmittedTarget
		)
		if useExtraAdmission {
			admittedTarget, err = extraAdmission.NextTarget(c.Request.Context(), service.GatewayTargetRequest{
				GroupID:            apiKey.GroupID,
				SessionKey:         selectionSessionHash,
				Model:              reqModel,
				ExcludedAccountIDs: fs.FailedAccountIDs,
			})
		} else {
			selection, err = h.gatewayService.SelectAccountWithLoadAwareness(c.Request.Context(), apiKey.GroupID, selectionSessionHash, reqModel, fs.FailedAccountIDs, "", int64(0))
		}
		if err != nil {
			if useExtraAdmission && errors.Is(err, context.Canceled) {
				return
			}
			if useExtraAdmission && isGatewayAdmissionConcurrencyError(err) {
				reqLog.Warn("gateway.cc.extra_admission_failed", zap.Error(err))
				h.handleCCConcurrencyError(c, err, "account", streamStarted)
				return
			}
			if len(fs.FailedAccountIDs) == 0 {
				cls := classifyNoAccountErrorFromGin(c, h.gatewayService, apiKey, reqModel, reqModel, groupPlatform)
				if !cls.ModelNotFound {
					markOpsRoutingCapacityLimitedIfNoAvailable(c, err)
				}
				message := cls.Message
				if !cls.ModelNotFound {
					message = "No available accounts: " + err.Error()
				}
				h.chatCompletionsErrorResponse(c, cls.Status, cls.ErrType, message)
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
					h.chatCompletionsErrorResponse(c, http.StatusBadGateway, "server_error", "All available accounts exhausted")
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

		// 4. Acquire account concurrency slot
		if !useExtraAdmission {
			if err := admission.AdmitAccount(AccountAdmissionRequest{
				Selection:  selection,
				WaitPolicy: AccountWaitPolicyUntracked,
			}); err != nil {
				var unavailableErr *AccountUnavailableError
				if errors.As(err, &unavailableErr) {
					markOpsRoutingCapacityLimited(c)
					h.chatCompletionsErrorResponse(c, http.StatusServiceUnavailable, "api_error", "No available accounts")
					return
				}
				reqLog.Warn("gateway.cc.account_slot_acquire_failed", zap.Int64("account_id", account.ID), zap.Error(err))
				h.handleCCConcurrencyError(c, err, "account", streamStarted)
				return
			}
		}

		if !useExtraAdmission && groupPlatform == service.PlatformGemini && account.Platform != service.PlatformGemini {
			if useExtraAdmission {
				extraAdmission.ReleaseTarget()
			} else {
				admission.ReleaseAccount()
			}
			fs.FailedAccountIDs[account.ID] = struct{}{}
			continue
		}
		if !useExtraAdmission && account.Platform == service.PlatformGemini && h.geminiCompatService == nil {
			if useExtraAdmission {
				extraAdmission.ReleaseTarget()
			} else {
				admission.ReleaseAccount()
			}
			h.chatCompletionsErrorResponse(c, http.StatusBadGateway, "upstream_error", "Gemini compatibility service is not configured")
			return
		}

		// 5. Forward request
		writerSizeBeforeForward := c.Writer.Size()
		forwardBody := body
		if channelMapping.Mapped {
			forwardBody = h.gatewayService.ReplaceModelInBody(body, channelMapping.MappedModel)
		}
		var result *service.ForwardResult
		var billingRecheckErr error
		forward := func(ctx context.Context, targetAccount *service.Account) error {
			if targetAccount.Platform == service.PlatformGemini {
				result, err = h.geminiCompatService.ForwardAsChatCompletions(ctx, c, targetAccount, forwardBody)
			} else {
				result, err = h.gatewayService.ForwardAsChatCompletions(ctx, c, targetAccount, forwardBody, parsedReq)
			}
			return err
		}
		sameAccountRetryCanceled := false
		forwardSameAccount := func(ctx context.Context, targetAccount *service.Account) error {
			for {
				writerSizeBeforeAttempt := c.Writer.Size()
				attemptErr := forward(ctx, targetAccount)
				if attemptErr == nil {
					return nil
				}
				var failoverErr *service.UpstreamFailoverError
				if !errors.As(attemptErr, &failoverErr) || c.Writer.Size() != writerSizeBeforeAttempt {
					return attemptErr
				}
				retry, canceled := fs.TrySameAccountRetry(ctx, targetAccount.ID, failoverErr)
				if canceled {
					sameAccountRetryCanceled = true
					return ctx.Err()
				}
				if !retry {
					return attemptErr
				}
			}
		}
		preparationHandled := false
		preparationRetry := false
		if useExtraAdmission {
			preparationHandled, err = admittedTarget.DispatchPrepared(
				c.Request.Context(),
				func(_ context.Context, targetAccount *service.Account) (service.GatewayTargetPreparation, error) {
					if groupPlatform == service.PlatformGemini && targetAccount.Platform != service.PlatformGemini {
						fs.FailedAccountIDs[targetAccount.ID] = struct{}{}
						preparationRetry = true
						return service.GatewayTargetPreparation{Handled: true}, nil
					}
					if targetAccount.Platform == service.PlatformGemini && h.geminiCompatService == nil {
						h.chatCompletionsErrorResponse(c, http.StatusBadGateway, "upstream_error", "Gemini compatibility service is not configured")
						return service.GatewayTargetPreparation{Handled: true}, nil
					}
					return service.GatewayTargetPreparation{}, nil
				},
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
			err = forward(c.Request.Context(), account)
			admission.ReleaseAccount()
		}
		if preparationHandled {
			if preparationRetry {
				continue
			}
			return
		}
		if billingRecheckErr != nil {
			reqLog.Info("gateway.cc.billing_recheck_failed", zap.Error(billingRecheckErr))
			status, code, message, retryAfter := billingErrorDetails(billingRecheckErr)
			if retryAfter > 0 {
				c.Header("Retry-After", strconv.Itoa(retryAfter))
			}
			h.chatCompletionsErrorResponse(c, status, code, message)
			return
		}
		if useExtraAdmission && isGatewayAdmissionConcurrencyError(err) {
			reqLog.Warn("gateway.cc.extra_admission_dispatch_failed", zap.Error(err))
			h.handleCCConcurrencyError(c, err, "account", streamStarted)
			return
		}
		if sameAccountRetryCanceled || (useExtraAdmission && errors.Is(err, context.Canceled)) {
			return
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
			upstreamErrorAlreadyCommunicated := gatewayForwardErrorAlreadyCommunicated(c, writerSizeBeforeForward, err)
			wroteFallback := false
			if !upstreamErrorAlreadyCommunicated {
				wroteFallback = h.ensureForwardErrorResponse(c, streamStarted)
			}
			reqLog.Error("gateway.cc.forward_failed",
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
				reqLog.Error("gateway.cc.record_usage_failed",
					zap.Int64("account_id", account.ID),
					zap.Error(err),
				)
			}
		})
		return
	}
}

// chatCompletionsErrorResponse writes an error in OpenAI Chat Completions format.
func (h *GatewayHandler) chatCompletionsErrorResponse(c *gin.Context, status int, errType, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
}

func (h *GatewayHandler) handleCCConcurrencyError(c *gin.Context, err error, slotType string, streamStarted bool) {
	status, errType, message := concurrencyErrorResponse(err, slotType)
	if streamStarted {
		h.handleStreamingAwareError(c, status, errType, message, true)
		return
	}
	h.chatCompletionsErrorResponse(c, status, errType, message)
}

// handleCCFailoverExhausted writes a failover-exhausted error in CC format.
func (h *GatewayHandler) handleCCFailoverExhausted(c *gin.Context, lastErr *service.UpstreamFailoverError, streamStarted bool) {
	if streamStarted {
		return
	}
	statusCode := http.StatusBadGateway
	if lastErr != nil && lastErr.StatusCode > 0 {
		statusCode = lastErr.StatusCode
	}
	if lastErr != nil && service.IsOpenAISilentRefusalErrorBody(lastErr.ResponseBody) {
		service.SetOpsUpstreamError(c, statusCode, service.OpenAISilentRefusalClientMessage(), "")
		h.chatCompletionsErrorResponse(c, http.StatusBadGateway, "upstream_error", service.OpenAISilentRefusalClientMessage())
		return
	}
	h.chatCompletionsErrorResponse(c, statusCode, "server_error", "All available accounts exhausted")
}
