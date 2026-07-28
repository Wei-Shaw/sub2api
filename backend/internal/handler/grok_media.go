package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// GrokImages handles xAI image generation/editing through Grok groups.
func (h *OpenAIGatewayHandler) GrokImages(c *gin.Context) {
	endpoint := service.GrokMediaEndpointImagesGenerations
	if strings.Contains(c.Request.URL.Path, "/images/edits") {
		endpoint = service.GrokMediaEndpointImagesEdits
	}
	h.handleGrokMedia(c, endpoint, "")
}

// GrokVideoGeneration handles xAI video generation through Grok groups.
func (h *OpenAIGatewayHandler) GrokVideoGeneration(c *gin.Context) {
	h.handleGrokMedia(c, service.GrokMediaEndpointVideosGenerations, "")
}

// GrokVideoEdit handles asynchronous xAI video edits through Grok groups.
func (h *OpenAIGatewayHandler) GrokVideoEdit(c *gin.Context) {
	h.handleGrokMedia(c, service.GrokMediaEndpointVideosEdits, "")
}

// GrokVideoExtension handles asynchronous xAI video extensions through Grok groups.
func (h *OpenAIGatewayHandler) GrokVideoExtension(c *gin.Context) {
	h.handleGrokMedia(c, service.GrokMediaEndpointVideosExtensions, "")
}

// GrokVideoStatus handles xAI video status retrieval through Grok groups.
func (h *OpenAIGatewayHandler) GrokVideoStatus(c *gin.Context) {
	h.handleGrokMedia(c, service.GrokMediaEndpointVideoStatus, c.Param("request_id"))
}

// GrokVideoContent proxies downloadable video content through the task's upstream account.
func (h *OpenAIGatewayHandler) GrokVideoContent(c *gin.Context) {
	h.handleGrokMedia(c, service.GrokMediaEndpointVideoContent, c.Param("request_id"))
}

func (h *OpenAIGatewayHandler) handleGrokMedia(c *gin.Context, endpoint service.GrokMediaEndpoint, requestID string) {
	streamStarted := false
	defer h.recoverResponsesPanic(c, &streamStarted)

	requestStart := time.Now()
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}

	reqLog := requestLogger(
		c,
		"handler.openai_gateway.grok_media",
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
		zap.String("endpoint", string(endpoint)),
	)
	if !h.ensureResponsesDependencies(c, reqLog) {
		return
	}

	var body []byte
	var err error
	if endpoint.RequiresRequestBody() {
		body, err = pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
		if err != nil {
			if maxErr, ok := extractMaxBytesError(err); ok {
				h.errorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
				return
			}
			h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
			return
		}
		if len(body) == 0 {
			h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
			return
		}
	}

	contentType := c.GetHeader("Content-Type")
	requestInfo := service.ParseGrokMediaRequest(contentType, body)
	requestModel := requestInfo.Model
	routingModel := service.NormalizeGrokMediaModelForEndpoint(endpoint, requestModel, requestInfo.HasInputImage())
	if endpoint.IsGenerationRequest() && strings.TrimSpace(requestModel) == "" {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}
	if endpoint.IsVideoLookupRequest() && strings.TrimSpace(requestID) == "" {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "request_id is required")
		return
	}

	reqLog = reqLog.With(zap.String("model", requestModel))
	setOpsRequestContext(c, requestModel, false)
	setOpsEndpointContext(c, "", int16(service.RequestTypeSync))

	var videoCreateRecord *service.GrokMediaVideoCreateRecord
	var imageCreateRecord *service.GrokMediaImageCreateRecord
	if endpoint.IsGenerationRequest() {
		if !service.GroupAllowsImageGeneration(apiKey.Group) {
			h.errorResponse(c, http.StatusForbidden, "permission_error", service.ImageGenerationPermissionMessage())
			return
		}
		if moderationBody := requestInfo.ModerationBody(); len(moderationBody) > 0 {
			decision := h.checkSecurityAudit(c, reqLog, apiKey, subject, service.ContentModerationProtocolOpenAIImages, requestModel, moderationBody)
			if decision != nil && !decision.AllowNextStage {
				h.openAISecurityAuditError(c, decision)
				return
			}
		}
		if endpoint.IsImageGenerationRequest() {
			imageCreateRecord, err = h.gatewayService.ClaimGrokMediaImageCreate(
				c.Request.Context(), apiKey.GroupID, endpoint, c.GetHeader("Idempotency-Key"), contentType,
				body, subject.UserID, apiKey.ID,
			)
			switch {
			case errors.Is(err, service.ErrGrokMediaImageIdempotencyKeyInvalid):
				h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Idempotency-Key is invalid")
				return
			case errors.Is(err, service.ErrGrokMediaImageIdempotencyConflict):
				h.errorResponse(c, http.StatusConflict, "idempotency_conflict", "Idempotency-Key was already used with a different image request")
				return
			case err != nil:
				reqLog.Warn("grok_media.image_create_idempotency_claim_failed", zap.Error(err))
				if !errors.Is(err, service.ErrGrokMediaImageIdempotencyUnavailable) {
					h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Image request cannot be canonicalized")
				} else {
					h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Image idempotency store temporarily unavailable")
				}
				return
			}
			if imageCreateRecord != nil && imageCreateRecord.Completed() {
				if replayErr := service.WriteGrokMediaImageCreateReplay(c, imageCreateRecord); replayErr != nil {
					reqLog.Warn("grok_media.image_create_idempotency_replay_failed", zap.Error(replayErr))
					h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Image idempotency replay temporarily unavailable")
				}
				return
			}
			if imageCreateRecord != nil {
				service.SetGrokMediaUpstreamIdempotencyKey(c, imageCreateRecord.UpstreamIdempotencyKey)
			}
		} else if endpoint.IsVideoGenerationRequest() {
			videoCreateRecord, err = h.gatewayService.ClaimGrokMediaVideoCreate(
				c.Request.Context(), apiKey.GroupID, endpoint, c.GetHeader("Idempotency-Key"), contentType,
				body, subject.UserID, apiKey.ID,
			)
			switch {
			case errors.Is(err, service.ErrGrokMediaVideoIdempotencyKeyRequired):
				h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Idempotency-Key is required for video creation")
				return
			case errors.Is(err, service.ErrGrokMediaVideoIdempotencyKeyInvalid):
				h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Idempotency-Key is invalid")
				return
			case errors.Is(err, service.ErrGrokMediaVideoIdempotencyConflict):
				h.errorResponse(c, http.StatusConflict, "idempotency_conflict", "Idempotency-Key was already used with a different video request")
				return
			case err != nil:
				reqLog.Warn("grok_media.video_create_idempotency_claim_failed", zap.Error(err))
				if !errors.Is(err, service.ErrGrokMediaVideoIdempotencyUnavailable) {
					h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Video request cannot be canonicalized")
				} else {
					h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Video idempotency store temporarily unavailable")
				}
				return
			}
			if videoCreateRecord.Completed() {
				replayBindingCtx, replayBindingCancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 5*time.Second)
				replayBindingErr := h.gatewayService.BindGrokMediaVideoRequestAccount(
					replayBindingCtx, apiKey.GroupID, videoCreateRecord.RequestID,
					subject.UserID, apiKey.ID, videoCreateRecord.AccountID,
				)
				replayBindingCancel()
				if replayBindingErr != nil {
					reqLog.Warn("grok_media.video_create_idempotency_replay_owner_refresh_failed", zap.Error(replayBindingErr))
					h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Video idempotency owner refresh temporarily unavailable")
					return
				}
				if replayErr := service.WriteGrokMediaVideoCreateReplay(c, videoCreateRecord); replayErr != nil {
					reqLog.Warn("grok_media.video_create_idempotency_replay_failed", zap.Error(replayErr))
					h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Video idempotency replay temporarily unavailable")
				}
				return
			}
			service.SetGrokMediaUpstreamIdempotencyKey(c, videoCreateRecord.UpstreamIdempotencyKey)
		}
		imageReleaseFunc, acquired := h.acquireImageGenerationSlot(c, streamStarted)
		if !acquired {
			return
		}
		if imageReleaseFunc != nil {
			defer imageReleaseFunc()
		}
	}

	if h.errorPassthroughService != nil {
		service.BindErrorPassthroughService(c, h.errorPassthroughService)
	}

	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	service.SetOpsLatencyMs(c, service.OpsAuthLatencyMsKey, time.Since(requestStart).Milliseconds())

	userReleaseFunc, acquired := h.acquireResponsesUserSlot(c, subject.UserID, subject.Concurrency, false, &streamStarted, reqLog)
	if !acquired {
		return
	}
	if userReleaseFunc != nil {
		defer userReleaseFunc()
	}

	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
		reqLog.Info("grok_media.billing_eligibility_check_failed", zap.Error(err))
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		h.errorResponse(c, status, code, message)
		return
	}

	sessionSeed := body
	if len(sessionSeed) == 0 && strings.TrimSpace(requestID) != "" {
		sessionSeed = []byte(requestID)
	}
	sessionHash := h.gatewayService.GenerateExplicitSessionHash(c, sessionSeed)
	boundLookupAccountID := int64(0)
	boundCreateAccountID := int64(0)
	if videoCreateRecord != nil {
		boundCreateAccountID = videoCreateRecord.AccountID
	} else if imageCreateRecord != nil {
		boundCreateAccountID = imageCreateRecord.AccountID
	}
	if endpoint.IsVideoLookupRequest() {
		sessionHash = ""
		boundLookupAccountID, err = h.gatewayService.ResolveGrokMediaVideoRequestAccount(
			c.Request.Context(), apiKey.GroupID, requestID, subject.UserID, apiKey.ID,
		)
		if errors.Is(err, service.ErrGrokMediaVideoRequestOwnerNotFound) || (err == nil && boundLookupAccountID <= 0) {
			reqLog.Info("grok_media.video_lookup_owner_binding_missing", zap.Error(err))
			h.errorResponse(c, http.StatusNotFound, "not_found_error", "Video request not found")
			return
		}
		if err != nil {
			reqLog.Warn("grok_media.video_lookup_owner_binding_unavailable", zap.Error(err))
			h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Video request owner lookup temporarily unavailable")
			return
		}
	}
	requestCtx := c.Request.Context()
	failedAccountIDs := make(map[int64]struct{})
	sameAccountRetryCount := make(map[int64]int)
	var lastFailoverErr *service.UpstreamFailoverError
	var oauth429FailoverState service.OpenAIOAuth429FailoverState
	mediaEligibilityRejected := false
	switchCount := 0
	maxAccountSwitches := h.maxAccountSwitches
	if maxAccountSwitches <= 0 {
		maxAccountSwitches = 3
	}
	routingStart := time.Now()
	requiredCapability := grokMediaRequiredCapability(endpoint)

	for {
		if failoverClientGone(c) {
			return
		}
		var selection *service.AccountSelectionResult
		var scheduleDecision service.OpenAIAccountScheduleDecision
		requiresBoundAccount := endpoint.IsVideoLookupRequest() || boundCreateAccountID > 0
		if requiresBoundAccount {
			boundAccountID := boundLookupAccountID
			if boundCreateAccountID > 0 {
				boundAccountID = boundCreateAccountID
			}
			selection, err = h.gatewayService.SelectGrokMediaVideoRequestOwner(
				requestCtx,
				apiKey.GroupID,
				boundAccountID,
			)
		} else {
			selection, scheduleDecision, err = h.gatewayService.SelectAccountWithSchedulerForCapability(
				requestCtx,
				apiKey.GroupID,
				"",
				sessionHash,
				routingModel,
				failedAccountIDs,
				service.OpenAIUpstreamTransportHTTPSSE,
				requiredCapability,
				false,
				false,
				false,
				service.PlatformGrok,
			)
		}
		if err != nil && requiresBoundAccount {
			reqLog.Warn("grok_media.bound_account_unavailable", zap.Error(err))
			h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Persisted media account temporarily unavailable")
			return
		}
		if err != nil {
			if failoverClientGone(c) {
				reqLog.Info("grok_media.account_select_aborted_client_disconnected", zap.Error(err))
				return
			}
			reqLog.Warn("grok_media.account_select_failed",
				zap.Error(err),
				zap.Int("excluded_account_count", len(failedAccountIDs)),
			)
			if endpoint.IsGenerationRequest() && errors.Is(err, service.ErrNoAvailableAccounts) &&
				(len(failedAccountIDs) == 0 || (mediaEligibilityRejected && lastFailoverErr == nil)) {
				markOpsRoutingCapacityLimitedIfNoAvailable(c, err)
				h.errorResponse(c, http.StatusServiceUnavailable, "grok_media_no_eligible_account", "No eligible Grok media accounts")
				return
			}
			if len(failedAccountIDs) == 0 {
				cls := classifyNoAccountErrorFromGin(c, h.gatewayService, apiKey, requestModel, routingModel, service.PlatformGrok)
				if !cls.ModelNotFound {
					markOpsRoutingCapacityLimitedIfNoAvailable(c, err)
				}
				h.errorResponse(c, cls.Status, cls.ErrType, cls.Message)
				return
			}
			if lastFailoverErr != nil {
				h.handleFailoverExhausted(c, lastFailoverErr, false)
			} else {
				h.errorResponse(c, http.StatusBadGateway, "api_error", "Upstream request failed")
			}
			return
		}
		if selection == nil || selection.Account == nil {
			if endpoint.IsGenerationRequest() {
				markOpsRoutingCapacityLimited(c)
				h.errorResponse(c, http.StatusServiceUnavailable, "grok_media_no_eligible_account", "No eligible Grok media accounts")
				return
			}
			cls := classifyNoAccountErrorFromGin(c, h.gatewayService, apiKey, requestModel, routingModel, service.PlatformGrok)
			if !cls.ModelNotFound {
				markOpsRoutingCapacityLimited(c)
			}
			h.errorResponse(c, cls.Status, cls.ErrType, cls.Message)
			return
		}
		expectedBoundAccountID := boundLookupAccountID
		if boundCreateAccountID > 0 {
			expectedBoundAccountID = boundCreateAccountID
		}
		if releaseMismatchedGrokMediaVideoOwner(selection, expectedBoundAccountID) {
			reqLog.Warn("grok_media.video_lookup_bound_account_unavailable",
				zap.Int64("bound_account_id", expectedBoundAccountID),
				zap.Int64("selected_account_id", selection.Account.ID),
			)
			h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Video request owner account mismatch")
			return
		}

		reqLog.Debug("grok_media.account_schedule_decision",
			zap.String("layer", scheduleDecision.Layer),
			zap.Bool("sticky_session_hit", scheduleDecision.StickySessionHit),
			zap.Int("candidate_count", scheduleDecision.CandidateCount),
			zap.Int("top_k", scheduleDecision.TopK),
			zap.Int64("latency_ms", scheduleDecision.LatencyMs),
			zap.Float64("load_skew", scheduleDecision.LoadSkew),
		)

		account := selection.Account
		if endpoint.IsGenerationRequest() && boundCreateAccountID == 0 {
			eligible, eligibilityReason, eligibilityErr := h.ensureGrokMediaAccountEligibility(requestCtx, account)
			if !eligible {
				mediaEligibilityRejected = true
				failedAccountIDs[account.ID] = struct{}{}
				reqLog.Warn("grok_media.account_eligibility_rejected",
					zap.Int64("account_id", account.ID),
					zap.String("reason", eligibilityReason),
					zap.Bool("probe_failed", eligibilityErr != nil),
				)
				if switchCount >= maxAccountSwitches {
					markOpsRoutingCapacityLimited(c)
					h.errorResponse(c, http.StatusServiceUnavailable, "grok_media_no_eligible_account", "No eligible Grok media accounts")
					return
				}
				switchCount++
				continue
			}
		}
		sessionHash = ensureOpenAIPoolModeSessionHash(sessionHash, account)
		setOpsSelectedAccount(c, account.ID, account.Platform)

		accountReleaseFunc, accountAcquired := h.acquireResponsesAccountSlot(c, apiKey.GroupID, sessionHash, selection, false, &streamStarted, reqLog)
		if !accountAcquired {
			return
		}
		if videoCreateRecord != nil {
			persistedAccountID, bindErr := h.gatewayService.BindGrokMediaVideoCreateAccount(requestCtx, videoCreateRecord, account.ID)
			if bindErr != nil {
				if accountReleaseFunc != nil {
					accountReleaseFunc()
				}
				reqLog.Warn("grok_media.bind_video_create_account_failed", zap.Error(bindErr))
				h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Video idempotency account persistence temporarily unavailable")
				return
			}
			boundCreateAccountID = persistedAccountID
			if persistedAccountID != account.ID {
				if accountReleaseFunc != nil {
					accountReleaseFunc()
				}
				reqLog.Info("grok_media.video_create_account_claim_already_bound",
					zap.Int64("selected_account_id", account.ID),
					zap.Int64("bound_account_id", persistedAccountID),
				)
				continue
			}
		} else if imageCreateRecord != nil {
			persistedAccountID, bindErr := h.gatewayService.BindGrokMediaImageCreateAccount(requestCtx, imageCreateRecord, account.ID)
			if bindErr != nil {
				if accountReleaseFunc != nil {
					accountReleaseFunc()
				}
				reqLog.Warn("grok_media.bind_image_create_account_failed", zap.Error(bindErr))
				h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Image idempotency account persistence temporarily unavailable")
				return
			}
			boundCreateAccountID = persistedAccountID
			if persistedAccountID != account.ID {
				if accountReleaseFunc != nil {
					accountReleaseFunc()
				}
				reqLog.Info("grok_media.image_create_account_claim_already_bound",
					zap.Int64("selected_account_id", account.ID),
					zap.Int64("bound_account_id", persistedAccountID),
				)
				continue
			}
		}

		service.SetOpsLatencyMs(c, service.OpsRoutingLatencyMsKey, time.Since(routingStart).Milliseconds())
		forwardStart := time.Now()
		writerSizeBeforeForward := c.Writer.Size()
		result, err := func() (*service.OpenAIForwardResult, error) {
			defer func() {
				if accountReleaseFunc != nil {
					accountReleaseFunc()
				}
			}()
			return h.gatewayService.ForwardGrokMedia(requestCtx, c, account, endpoint, requestID, body, contentType)
		}()

		forwardDurationMs := time.Since(forwardStart).Milliseconds()
		upstreamLatencyMs, _ := getContextInt64(c, service.OpsUpstreamLatencyMsKey)
		responseLatencyMs := forwardDurationMs
		if upstreamLatencyMs > 0 && forwardDurationMs > upstreamLatencyMs {
			responseLatencyMs = forwardDurationMs - upstreamLatencyMs
		}
		service.SetOpsLatencyMs(c, service.OpsResponseLatencyMsKey, responseLatencyMs)

		if err != nil {
			var failoverErr *service.UpstreamFailoverError
			if errors.As(err, &failoverErr) {
				if failoverClientGone(c) {
					reqLog.Info("grok_media.failover_aborted_client_disconnected",
						zap.Int64("account_id", account.ID),
						zap.Int("upstream_status", failoverErr.StatusCode),
					)
					return
				}
				if failoverErr.ShouldReportAccountScheduleFailure() {
					h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, grokMediaScheduleModel(account, routingModel, nil), false, nil)
				}
				if c.Writer.Size() != writerSizeBeforeForward {
					h.handleFailoverExhausted(c, failoverErr, true)
					return
				}
				if !failoverErr.ShouldRetryNextAccount() {
					h.handleFailoverExhausted(c, failoverErr, false)
					return
				}
				if endpoint.IsVideoLookupRequest() {
					h.handleFailoverExhausted(c, failoverErr, false)
					return
				}
				if failoverErr.RetryableOnSameAccount {
					retryLimit := account.GetPoolModeRetryCount()
					if sameAccountRetryCount[account.ID] < retryLimit {
						sameAccountRetryCount[account.ID]++
						reqLog.Warn("grok_media.pool_mode_same_account_retry",
							zap.Int64("account_id", account.ID),
							zap.Int("upstream_status", failoverErr.StatusCode),
							zap.Int("retry_limit", retryLimit),
							zap.Int("retry_count", sameAccountRetryCount[account.ID]),
						)
						select {
						case <-requestCtx.Done():
							return
						case <-time.After(sameAccountRetryDelay):
						}
						continue
					}
				}
				if boundCreateAccountID > 0 {
					if !grokVideoCreateAccountRebindSafe(failoverErr) {
						h.handleFailoverExhausted(c, failoverErr, false)
						return
					}
					var releaseErr error
					if videoCreateRecord != nil {
						releaseErr = h.gatewayService.ReleaseGrokMediaVideoCreateAccount(requestCtx, videoCreateRecord, account.ID)
					} else if imageCreateRecord != nil {
						releaseErr = h.gatewayService.ReleaseGrokMediaImageCreateAccount(requestCtx, imageCreateRecord, account.ID)
					}
					if releaseErr != nil {
						reqLog.Warn("grok_media.release_rejected_create_account_failed", zap.Error(releaseErr))
						h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Media idempotency account recovery temporarily unavailable")
						return
					}
					boundCreateAccountID = 0
				}
				h.gatewayService.RecordOpenAIAccountSwitch()
				failedAccountIDs[account.ID] = struct{}{}
				lastFailoverErr = failoverErr
				if switchCount >= maxAccountSwitches {
					h.handleFailoverExhausted(c, failoverErr, false)
					return
				}
				switchCount++
				if h.gatewayService.ShouldStopOpenAIOAuth429Failover(account, failoverErr.StatusCode, switchCount, &oauth429FailoverState) {
					h.handleFailoverExhausted(c, failoverErr, false)
					return
				}
				reqLog.Warn("grok_media.upstream_failover_switching",
					zap.Int64("account_id", account.ID),
					zap.Int("upstream_status", failoverErr.StatusCode),
					zap.Int("switch_count", switchCount),
					zap.Int("max_switches", maxAccountSwitches),
				)
				continue
			}
			h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, grokMediaScheduleModel(account, routingModel, nil), false, nil)
			if !service.IsResponseCommitted(c) && c.Writer.Size() == writerSizeBeforeForward {
				h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Upstream request failed")
			}
			reqLog.Warn("grok_media.forward_failed",
				zap.Int64("account_id", account.ID),
				zap.Error(err),
			)
			return
		}

		h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, grokMediaScheduleModel(account, routingModel, result), true, nil)
		if endpoint.IsVideoGenerationRequest() {
			statusCode, responseContentType, responseBody, bufferedErr := service.BufferedGrokMediaResponse(c)
			if bufferedErr != nil {
				reqLog.Warn("grok_media.read_buffered_video_create_response_failed", zap.Error(bufferedErr))
				h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Video create response persistence temporarily unavailable")
				return
			}
			bindingCtx, bindingCancel := context.WithTimeout(context.WithoutCancel(requestCtx), 5*time.Second)
			bindErr := h.gatewayService.CompleteGrokMediaVideoCreate(
				bindingCtx, videoCreateRecord, result.ResponseID, account.ID,
				statusCode, responseContentType, responseBody,
			)
			bindingCancel()
			if bindErr != nil {
				reqLog.Warn("grok_media.bind_video_request_account_failed",
					zap.Int64("account_id", account.ID),
					zap.String("request_id", result.ResponseID),
					zap.Error(bindErr),
				)
				h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Video request owner persistence temporarily unavailable")
				return
			}
			if commitErr := service.CommitBufferedGrokMediaResponse(c); commitErr != nil {
				reqLog.Warn("grok_media.commit_buffered_response_failed", zap.Error(commitErr))
				h.errorResponse(c, http.StatusInternalServerError, "api_error", "Failed to write video create response")
				return
			}
		} else if imageCreateRecord != nil {
			statusCode, responseContentType, responseBody, bufferedErr := service.BufferedGrokMediaResponse(c)
			if bufferedErr != nil {
				reqLog.Warn("grok_media.read_buffered_image_create_response_failed", zap.Error(bufferedErr))
				h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Image create response persistence temporarily unavailable")
				return
			}
			completionCtx, completionCancel := context.WithTimeout(context.WithoutCancel(requestCtx), 5*time.Second)
			completionErr := h.gatewayService.CompleteGrokMediaImageCreate(
				completionCtx, imageCreateRecord, account.ID, statusCode, responseContentType, responseBody,
			)
			completionCancel()
			if completionErr != nil {
				reqLog.Warn("grok_media.complete_image_create_idempotency_failed", zap.Int64("account_id", account.ID), zap.Error(completionErr))
				h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Image idempotency response persistence temporarily unavailable")
				return
			}
			if commitErr := service.CommitBufferedGrokMediaResponse(c); commitErr != nil {
				reqLog.Warn("grok_media.commit_buffered_image_response_failed", zap.Error(commitErr))
				h.errorResponse(c, http.StatusInternalServerError, "api_error", "Failed to write image create response")
				return
			}
		}
		if endpoint.IsVideoLookupRequest() && result.GrokMediaVideoTerminal {
			if terminalErr := h.retainGrokMediaVideoTerminalOwner(
				requestCtx, endpoint, result, apiKey.GroupID, requestID, subject.UserID, apiKey.ID,
			); terminalErr != nil {
				// The upstream response has already been served. Retention metadata is
				// best-effort here and must never trigger account fallback or replace it.
				reqLog.Warn("grok_media.video_terminal_owner_retention_failed", zap.Error(terminalErr))
			}
		}
		if shouldRecordGrokMediaUsage(endpoint, requestModel) {
			recordGrokMediaUsage(c, h, reqLog, apiKey, subject, subscription, account, result, requestModel, body, requestID)
		}
		reqLog.Debug("grok_media.request_completed",
			zap.Int64("account_id", account.ID),
			zap.Int("switch_count", switchCount),
		)
		return
	}
}

func (h *OpenAIGatewayHandler) retainGrokMediaVideoTerminalOwner(
	ctx context.Context,
	endpoint service.GrokMediaEndpoint,
	result *service.OpenAIForwardResult,
	groupID *int64,
	requestID string,
	userID, apiKeyID int64,
) error {
	if !endpoint.IsVideoLookupRequest() || result == nil || !result.GrokMediaVideoTerminal {
		return nil
	}
	if h == nil || h.gatewayService == nil {
		return errors.New("grok media video terminal owner service is unavailable")
	}
	terminalCtx, terminalCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer terminalCancel()
	return h.gatewayService.MarkGrokMediaVideoRequestTerminal(
		terminalCtx, groupID, requestID, userID, apiKeyID,
	)
}

func releaseMismatchedGrokMediaVideoOwner(selection *service.AccountSelectionResult, boundAccountID int64) bool {
	if boundAccountID <= 0 || selection == nil || selection.Account == nil || selection.Account.ID == boundAccountID {
		return false
	}
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
		selection.ReleaseFunc = nil
	}
	return true
}

func grokVideoCreateAccountRebindSafe(err *service.UpstreamFailoverError) bool {
	if err == nil {
		return false
	}
	if err.IsCredentialFailure() {
		return true
	}
	switch err.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests:
		return true
	default:
		return false
	}
}

func (h *OpenAIGatewayHandler) ensureGrokMediaAccountEligibility(ctx context.Context, account *service.Account) (bool, string, error) {
	if account == nil {
		return false, "missing_account", errors.New("grok media account is required")
	}
	eligible, reason := account.GrokMediaGenerationEligibility()
	if eligible || reason != "billing_unobserved" {
		return eligible, reason, nil
	}
	if h == nil || h.grokMediaEligibilityProber == nil {
		return false, "billing_probe_unavailable", errors.New("grok media eligibility probe is not configured")
	}
	return h.grokMediaEligibilityProber.ProbeMediaEligibility(ctx, account.ID)
}

func grokMediaRequiredCapability(endpoint service.GrokMediaEndpoint) service.OpenAIEndpointCapability {
	if endpoint.IsGenerationRequest() {
		return service.OpenAIEndpointCapabilityGrokMediaGeneration
	}
	return ""
}

func grokMediaScheduleModel(account *service.Account, routingModel string, result *service.OpenAIForwardResult) string {
	if result != nil && strings.TrimSpace(result.UpstreamModel) != "" {
		return result.UpstreamModel
	}
	if account == nil {
		return strings.TrimSpace(routingModel)
	}
	return account.GetMappedModel(routingModel)
}

func shouldRecordGrokMediaUsage(endpoint service.GrokMediaEndpoint, requestModel string) bool {
	return endpoint.IsGenerationRequest() && strings.TrimSpace(requestModel) != ""
}

func recordGrokMediaUsage(
	c *gin.Context,
	h *OpenAIGatewayHandler,
	reqLog *zap.Logger,
	apiKey *service.APIKey,
	subject middleware2.AuthSubject,
	subscription *service.UserSubscription,
	account *service.Account,
	result *service.OpenAIForwardResult,
	requestModel string,
	body []byte,
	requestID string,
) {
	userAgent := c.GetHeader("User-Agent")
	clientIP := ip.GetClientIP(c)
	sessionID := service.ExtractClientSessionID(c)
	payloadForHash := body
	if len(payloadForHash) == 0 && strings.TrimSpace(requestID) != "" {
		payloadForHash = []byte(requestID)
	}
	inboundEndpoint := GetInboundEndpoint(c)
	upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)
	quotaPlatform := service.QuotaPlatform(c.Request.Context(), apiKey)
	// OriginalModel 记录客户端请求的模型：composite 分组下 body 已被改写为具体模型，
	// 公开别名需从 context 取回，与其他端点的用量归因口径一致（计费不受影响：
	// BillingModelSource 为空不会触发来源覆盖）。
	channelUsageFields := service.ChannelUsageFields{
		OriginalModel:      clientRequestedModel(c, requestModel),
		ChannelMappedModel: requestModel,
	}
	h.submitOpenAIUsageRecordTask(c.Request.Context(), result, func(ctx context.Context) {
		if err := h.gatewayService.RecordUsage(ctx, &service.OpenAIRecordUsageInput{
			Result:             result,
			APIKey:             apiKey,
			User:               apiKey.User,
			Account:            account,
			Subscription:       subscription,
			InboundEndpoint:    inboundEndpoint,
			UpstreamEndpoint:   upstreamEndpoint,
			UserAgent:          userAgent,
			IPAddress:          clientIP,
			RequestPayloadHash: service.HashUsageRequestPayload(payloadForHash),
			APIKeyService:      h.apiKeyService,
			QuotaPlatform:      quotaPlatform,
			SessionID:          sessionID,
			ChannelUsageFields: channelUsageFields,
		}); err != nil {
			logger.L().With(
				zap.String("component", "handler.openai_gateway.grok_media"),
				zap.Int64("user_id", subject.UserID),
				zap.Int64("api_key_id", apiKey.ID),
				zap.Any("group_id", apiKey.GroupID),
				zap.String("model", requestModel),
				zap.Int64("account_id", account.ID),
			).Error("grok_media.record_usage_failed", zap.Error(err))
			reqLog.Debug("grok_media.record_usage_failed", zap.Error(err))
		}
	})
}
