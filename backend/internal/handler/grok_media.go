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
	noAccountCode, noAccountMessage := "grok_media_no_eligible_account", "No eligible Grok media accounts"
	if endpoint.IsSeedance() {
		noAccountCode, noAccountMessage = "seedance_no_eligible_account", "No eligible Seedance accounts"
	}
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
	requestPlatform := service.PlatformGrok
	if endpoint.IsSeedance() {
		requestPlatform = service.PlatformOpenAI
		if endpoint == service.SeedanceEndpointCreate {
			requestInfo, err = service.ParseSeedanceRequest(body)
			if err != nil {
				h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", err.Error())
				return
			}
		}
	} else if apiKey.Group != nil && apiKey.Group.Platform == service.PlatformOpenAI {
		requestPlatform = service.PlatformOpenAI
		requestInfo = service.ParseOpenAIVideoRequest(contentType, body)
	}
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
	var durableTask *service.VideoTask
	durableGrokCreate := requestPlatform == service.PlatformGrok && endpoint == service.GrokMediaEndpointVideosGenerations && h.gatewayService.GrokVideoDurableSettlementEnabled()
	if !endpoint.IsSeedance() && endpoint.IsVideoLookupRequest() {
		// Always check durable ownership, even on a Redis hit: otherwise cached
		// new tasks could bypass settlement through the legacy billing path.
		durableTask, err = h.gatewayService.LoadDurableVideoTask(c.Request.Context(), requestID, subject.UserID, apiKey.ID)
		// Older embedded deployments without the durable repository keep their
		// legacy path while enrollment is disabled. Actual database errors must
		// fail closed; they must never allow bypassing a saved settlement gate.
		if requestPlatform == service.PlatformGrok && !h.gatewayService.GrokVideoDurableSettlementEnabled() && errors.Is(err, service.ErrVideoTaskRepositoryUnavailable) {
			err = nil
		}
		if err != nil {
			h.errorResponse(c, http.StatusServiceUnavailable, "video_task_unavailable", "Video task tracking is temporarily unavailable")
			return
		}
	}

	reqLog = reqLog.With(zap.String("model", requestModel))
	setOpsRequestContext(c, requestModel, false)
	setOpsEndpointContext(c, "", int16(service.RequestTypeSync))

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

	// Accepted durable tasks settle using their original billing snapshot; a
	// new-request balance gate must not prevent retrieval after settlement.
	if durableTask == nil {
		if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
			reqLog.Info("grok_media.billing_eligibility_check_failed", zap.Error(err))
			status, code, message, retryAfter := billingErrorDetails(err)
			if retryAfter > 0 {
				c.Header("Retry-After", strconv.Itoa(retryAfter))
			}
			h.errorResponse(c, status, code, message)
			return
		}
	}

	// 余额模式在途预留（与计费同口径估算；计费任务扣减余额缓存后才释放）。
	inflightDone, inflightErr := reserveInflightBalance(c, h.billingCacheService, h.gatewayService, apiKey, subscription, grokMediaInflightEstimate(endpoint, routingModel, requestInfo, body))
	if inflightErr != nil {
		status, code, message, retryAfter := billingErrorDetails(inflightErr)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		h.errorResponse(c, status, code, message)
		return
	}
	defer inflightDone()

	if durableGrokCreate || (requestPlatform == service.PlatformOpenAI && !endpoint.IsSeedance() && isGrokVideoCreateEndpoint(endpoint)) {
		if !h.beginOpenAIVideoSubmission(c, apiKey, contentType, body) {
			return
		}
	}

	sessionSeed := body
	if len(sessionSeed) == 0 && strings.TrimSpace(requestID) != "" {
		sessionSeed = []byte(requestID)
	}
	sessionHash := h.gatewayService.GenerateExplicitSessionHash(c, sessionSeed)
	boundLookupAccountID := int64(0)
	if endpoint.IsVideoLookupRequest() {
		sessionHash = service.GrokMediaVideoRequestSessionHash(requestID, subject.UserID, apiKey.ID)
		if durableTask != nil {
			boundLookupAccountID = durableTask.AccountID
		} else {
			boundLookupAccountID, err = h.gatewayService.ResolveGrokMediaVideoRequestAccount(
				c.Request.Context(), apiKey.GroupID, requestID, subject.UserID, apiKey.ID,
			)
		}
		if err != nil || boundLookupAccountID <= 0 {
			reqLog.Info("grok_media.video_lookup_owner_binding_missing", zap.Error(err))
			h.errorResponse(c, http.StatusNotFound, "not_found_error", "Video request not found")
			return
		}
	}
	// Grok 媒体（图片/视频生成与视频查询）按媒体倍率计费，不在 token 利润门
	// 范围内：显式豁免，防止 service 层防御性装门按文本 D 误过滤媒体请求，
	// 也防止已计费的在途视频任务因绑定账号被门排除而查询返回伪 404。
	requestCtx := service.WithOpenAIProfitControlSuppressed(c.Request.Context())
	profitVetoCount := 0
	failedAccountIDs := make(map[int64]struct{})
	sameAccountRetryCount := make(map[int64]int)
	var lastFailoverErr *service.UpstreamFailoverError
	var oauth429FailoverState service.OpenAIOAuth429FailoverState
	mediaEligibilityRejected := false
	switchCount := 0
	videoCreateStartedAt := ""
	if isGrokVideoCreateEndpoint(endpoint) {
		videoCreateStartedAt = service.GrokVideoPendingCreatedAtNow()
	}
	maxAccountSwitches := h.maxAccountSwitches
	if maxAccountSwitches <= 0 {
		maxAccountSwitches = 3
	}
	routingStart := time.Now()
	requiredCapability := grokMediaRequiredCapability(endpoint)
	if requestPlatform == service.PlatformOpenAI && !endpoint.IsSeedance() && endpoint.IsGenerationRequest() {
		requiredCapability = service.OpenAIEndpointCapabilityVideos
	}
	var accountReleaseFunc func()
	releaseAccount := func() {
		if accountReleaseFunc != nil {
			accountReleaseFunc()
			accountReleaseFunc = nil
		}
	}
	defer releaseAccount()

	for {
		releaseAccount()
		if failoverClientGone(c) {
			return
		}
		var selection *service.AccountSelectionResult
		var scheduleDecision service.OpenAIAccountScheduleDecision
		var err error
		pinnedOpenAILookup := (durableTask != nil || requestPlatform == service.PlatformOpenAI) && !endpoint.IsSeedance() && endpoint.IsVideoLookupRequest()
		if pinnedOpenAILookup {
			var account *service.Account
			var lookupErr error
			if durableTask != nil {
				account, lookupErr = h.gatewayService.GetDurableVideoLookupAccount(requestCtx, boundLookupAccountID)
			} else {
				account, lookupErr = h.gatewayService.GetOpenAIVideoLookupAccount(requestCtx, boundLookupAccountID)
			}
			if lookupErr != nil {
				h.errorResponse(c, http.StatusServiceUnavailable, "video_account_unavailable", "Video task account is temporarily unavailable")
				return
			}
			if durableTask != nil && !allowDurableOpenAIVideoForward(c, h.gatewayService, durableTask, account, endpoint) {
				return
			}
			selection = &service.AccountSelectionResult{Account: account}
			scheduleDecision.Layer = "video_owner_binding"
		} else if boundLookupAccountID > 0 {
			selection, scheduleDecision, err = h.gatewayService.SelectMediaVideoRequestAccount(
				requestCtx, apiKey.GroupID, sessionHash, boundLookupAccountID, routingModel, requestPlatform,
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
				requestPlatform,
			)
		}
		// Retain the upstream once-only slot release on every rejection path.
		if selection != nil && selection.Acquired {
			selection.ReleaseFunc = wrapReleaseOnDone(requestCtx, selection.ReleaseFunc)
			accountReleaseFunc = selection.ReleaseFunc
		}
		if err != nil {
			if failoverClientGone(c) {
				reqLog.Info("grok_media.account_select_aborted_client_disconnected", zap.Error(err))
				return
			}
			if boundLookupAccountID > 0 && errors.Is(err, service.ErrNoAvailableAccounts) {
				h.errorResponse(c, http.StatusNotFound, "not_found_error", "Video request not found")
				return
			}
			reqLog.Warn("grok_media.account_select_failed",
				zap.Error(err),
				zap.Int("excluded_account_count", len(failedAccountIDs)),
			)
			if (requestPlatform == service.PlatformGrok || endpoint.IsSeedance()) && endpoint.IsGenerationRequest() && errors.Is(err, service.ErrNoAvailableAccounts) &&
				(len(failedAccountIDs) == 0 || (mediaEligibilityRejected && lastFailoverErr == nil)) {
				markOpsRoutingCapacityLimitedIfNoAvailable(c, err)
				h.errorResponse(c, http.StatusServiceUnavailable, noAccountCode, noAccountMessage)
				return
			}
			if len(failedAccountIDs) == 0 {
				cls := classifyNoAccountErrorFromGin(c, h.gatewayService, apiKey, requestModel, routingModel, requestPlatform)
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
			if (requestPlatform == service.PlatformGrok || endpoint.IsSeedance()) && endpoint.IsGenerationRequest() {
				markOpsRoutingCapacityLimited(c)
				h.errorResponse(c, http.StatusServiceUnavailable, noAccountCode, noAccountMessage)
				return
			}
			cls := classifyNoAccountErrorFromGin(c, h.gatewayService, apiKey, requestModel, routingModel, requestPlatform)
			if !cls.ModelNotFound {
				markOpsRoutingCapacityLimited(c)
			}
			h.errorResponse(c, cls.Status, cls.ErrType, cls.Message)
			return
		}
		if failoverClientGone(c) {
			return
		}
		if boundLookupAccountID > 0 && selection.Account.ID != boundLookupAccountID {
			reqLog.Warn("grok_media.video_lookup_bound_account_unavailable",
				zap.Int64("bound_account_id", boundLookupAccountID),
				zap.Int64("selected_account_id", selection.Account.ID),
			)
			h.errorResponse(c, http.StatusNotFound, "not_found_error", "Video request not found")
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
		if account.Platform == service.PlatformGrok && endpoint.IsGenerationRequest() {
			eligible, eligibilityReason, eligibilityErr := h.ensureGrokMediaAccountEligibility(requestCtx, account)
			if !eligible {
				releaseAccount()
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
		if failoverClientGone(c) {
			return
		}
		sessionHash = ensureOpenAIPoolModeSessionHash(sessionHash, account)
		setOpsSelectedAccount(c, account.ID, account.Platform)

		slotResult := openAISlotAcquireOK
		if pinnedOpenAILookup {
			// A lookup must not enter creation scheduling or rewrite its owner
			// binding as a short-lived sticky session. Still enforce concurrency.
			var acquired bool
			accountReleaseFunc, acquired, err = h.concurrencyHelper.TryAcquireAccountSlot(requestCtx, account.ID, account.Concurrency)
			if err != nil {
				h.errorResponse(c, http.StatusServiceUnavailable, "video_account_unavailable", "Video task account is temporarily unavailable")
				return
			}
			if !acquired {
				h.errorResponse(c, http.StatusTooManyRequests, "rate_limit_error", "Video task account is busy; retry this lookup later")
				return
			}
		} else {
			admissionSessionHash := sessionHash
			if boundLookupAccountID > 0 {
				admissionSessionHash = ""
			}
			accountReleaseFunc, slotResult = h.acquireResponsesAccountSlot(c, apiKey.GroupID, admissionSessionHash, selection, false, &streamStarted, reqLog)
		}
		if slotResult == openAISlotAcquireProfitVetoed {
			// 媒体路径已显式豁免利润门（suppress 标记），此分支仅防御性兜底，
			// 同样受否决上限约束。
			if !recordOpenAIProfitVeto(failedAccountIDs, account.ID, &profitVetoCount) {
				h.handleOpenAIProfitVetoExhausted(c, streamStarted, reqLog, profitVetoCount)
				return
			}
			continue
		}
		if slotResult != openAISlotAcquireOK {
			return
		}

		service.SetOpsLatencyMs(c, service.OpsRoutingLatencyMsKey, time.Since(routingStart).Milliseconds())
		forwardStart := time.Now()
		writerSizeBeforeForward := c.Writer.Size()
		result, err := forwardRecordedVideo(c, h.gatewayService, endpoint, apiKey, account, subscription,
			service.GrokVideoPendingBilling{Platform: account.Platform, Model: requestModel, BillingModel: requestModel,
				OriginalModel: clientRequestedModel(c, requestModel), VideoResolution: requestInfo.Resolution,
				VideoDurationSeconds: requestInfo.DurationSeconds, CreatedAt: videoCreateStartedAt}, requestID,
			func() (*service.OpenAIForwardResult, error) {
				defer releaseAccount()
				if endpoint.IsSeedance() {
					return h.gatewayService.ForwardSeedance(requestCtx, c, account, endpoint, requestID, body)
				}
				if durableGrokCreate && account.Platform == service.PlatformGrok {
					var quote *service.VideoTaskCostSnapshot
					return h.gatewayService.ForwardGrokVideoCreate(requestCtx, c, account, endpoint, "", body, contentType,
						func(created *service.OpenAIForwardResult) error {
							pending := service.GrokVideoPendingBilling{Platform: account.Platform, Model: requestModel, BillingModel: firstNonEmptyString(created.BillingModel, requestModel), UpstreamModel: created.UpstreamModel, VideoResolution: created.VideoResolution, VideoDurationSeconds: created.VideoDurationSeconds, OriginalModel: clientRequestedModel(c, requestModel), CreatedAt: videoCreateStartedAt}
							saveCtx, cancel := context.WithTimeout(context.WithoutCancel(requestCtx), 15*time.Second)
							defer cancel()
							if err := h.gatewayService.PersistDurableVideoTask(saveCtx, created.ResponseID, apiKey, account, subscription, pending, quote); err != nil {
								return err
							}
							return h.gatewayService.CompleteVideoSubmission(saveCtx, apiKey.ID, c.GetHeader("Idempotency-Key"), created.ResponseID)
						}, func(normalized *service.OpenAIForwardResult) error {
							pricingAt, err := time.Parse(time.RFC3339Nano, videoCreateStartedAt)
							if err != nil {
								return err
							}
							quote, err = h.gatewayService.ResolveVideoTaskCost(requestCtx, &service.OpenAIRecordUsageInput{RequireAtomicVideo: true, Result: normalized, APIKey: apiKey, User: apiKey.User, Account: account, Subscription: subscription, APIKeyService: h.apiKeyService, PricingAt: pricingAt, QuotaPlatform: service.PlatformGrok, InboundEndpoint: "/v1/videos", UpstreamEndpoint: normalized.UpstreamEndpoint, ChannelUsageFields: service.ChannelUsageFields{OriginalModel: clientRequestedModel(c, requestModel), ChannelMappedModel: requestModel}})
							return err
						})
				}
				if account.Platform == service.PlatformOpenAI && isGrokVideoCreateEndpoint(endpoint) {
					var quote *service.VideoTaskCostSnapshot
					return h.gatewayService.ForwardOpenAIVideoCreate(requestCtx, c, account, body, contentType,
						func(created *service.OpenAIForwardResult) error {
							pending := service.GrokVideoPendingBilling{Platform: account.Platform,
								Model: requestModel, BillingModel: firstNonEmptyString(created.BillingModel, requestModel),
								UpstreamModel: created.UpstreamModel, VideoResolution: created.VideoResolution,
								VideoDurationSeconds: created.VideoDurationSeconds,
								OriginalModel:        clientRequestedModel(c, requestModel), CreatedAt: videoCreateStartedAt,
							}
							// Acceptance may outlive the client connection. Persist and
							// complete local idempotency before publishing success.
							saveCtx, cancel := context.WithTimeout(context.WithoutCancel(requestCtx), 15*time.Second)
							defer cancel()
							if err := h.gatewayService.PersistOpenAIVideoTask(saveCtx, created.ResponseID, apiKey, account, subscription, pending, quote); err != nil {
								return err
							}
							if err := h.gatewayService.CompleteVideoSubmission(saveCtx, apiKey.ID, c.GetHeader("Idempotency-Key"), created.ResponseID); err != nil {
								return err
							}
							if err := persistOpenAIVideoTask(saveCtx, h.gatewayService, apiKey.GroupID,
								created.ResponseID, subject.UserID, apiKey.ID, account.ID, pending); err != nil {
								reqLog.Warn("openai_video.cache_save_failed", zap.String("task_id", created.ResponseID))
							}
							return nil
						}, func(normalized *service.OpenAIForwardResult) error {
							pricingAt, err := time.Parse(time.RFC3339Nano, videoCreateStartedAt)
							if err != nil {
								return err
							}
							quote, err = h.gatewayService.ResolveVideoTaskCost(requestCtx, &service.OpenAIRecordUsageInput{
								RequireAtomicVideo: true, Result: normalized,
								APIKey: apiKey, User: apiKey.User, Account: account, Subscription: subscription,
								APIKeyService: h.apiKeyService, PricingAt: pricingAt, QuotaPlatform: service.PlatformOpenAI,
								InboundEndpoint: "/v1/videos", UpstreamEndpoint: normalized.UpstreamEndpoint,
								ChannelUsageFields: service.ChannelUsageFields{
									OriginalModel: clientRequestedModel(c, requestModel), ChannelMappedModel: requestModel,
								},
							})
							return err
						})
				}
				return h.gatewayService.ForwardGrokMedia(requestCtx, c, account, endpoint, requestID, body, contentType)
			})

		forwardDurationMs := time.Since(forwardStart).Milliseconds()
		upstreamLatencyMs, _ := getContextInt64(c, service.OpsUpstreamLatencyMsKey)
		responseLatencyMs := forwardDurationMs
		if upstreamLatencyMs > 0 && forwardDurationMs > upstreamLatencyMs {
			responseLatencyMs = forwardDurationMs - upstreamLatencyMs
		}
		service.SetOpsLatencyMs(c, service.OpsResponseLatencyMsKey, responseLatencyMs)

		if err != nil {
			var unknown *service.OpenAIVideoCreateUnknownError
			if errors.As(err, &unknown) {
				if !service.IsResponseCommitted(c) && c.Writer.Size() == writerSizeBeforeForward {
					service.MarkResponseCommitted(c)
					payload := gin.H{"error": gin.H{
						"type": "upstream_error", "code": "video_create_unknown",
						"message": "Video creation outcome is unknown; do not submit again with a new idempotency key",
					}}
					if unknown.TaskID != "" {
						payload["id"] = unknown.TaskID
						payload["object"] = "video"
					}
					c.JSON(http.StatusBadGateway, payload)
				}
				reqLog.Warn("openai_video.create_unknown", zap.Int64("account_id", account.ID), zap.String("task_id", unknown.TaskID))
				return
			}
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
					h.gatewayService.ReportOpenAIAccountScheduleResult(account, grokMediaScheduleModel(account, routingModel, nil), false, nil)
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
					retryLimit := effectiveSameAccountRetryLimit(failoverErr, account)
					if sameAccountRetryAllowed(failoverErr, sameAccountRetryCount[account.ID], retryLimit) {
						sameAccountRetryCount[account.ID]++
						retryDelay := sameAccountRetryDelayFor(failoverErr, sameAccountRetryCount[account.ID])
						reqLog.Warn("grok_media.pool_mode_same_account_retry",
							zap.Int64("account_id", account.ID),
							zap.Int("upstream_status", failoverErr.StatusCode),
							zap.Int("retry_limit", retryLimit),
							zap.Int("retry_count", sameAccountRetryCount[account.ID]),
							zap.Duration("retry_delay", retryDelay),
						)
						select {
						case <-requestCtx.Done():
							return
						case <-time.After(retryDelay):
						}
						continue
					}
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
			h.gatewayService.ReportOpenAIAccountScheduleResult(account, grokMediaScheduleModel(account, routingModel, nil), false, nil)
			if !service.IsResponseCommitted(c) && c.Writer.Size() == writerSizeBeforeForward {
				h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Upstream request failed")
			}
			reqLog.Warn("grok_media.forward_failed",
				zap.Int64("account_id", account.ID),
				zap.Error(err),
			)
			return
		}

		h.gatewayService.ReportOpenAIAccountScheduleResult(account, grokMediaScheduleModel(account, routingModel, result), true, nil)
		shouldBindLegacyVideoTask := false
		if account.Platform != service.PlatformOpenAI && isGrokVideoCreateEndpoint(endpoint) && strings.TrimSpace(result.ResponseID) != "" {
			shouldBindLegacyVideoTask = !durableGrokCreate
		}
		if endpoint == service.SeedanceEndpointCreate && strings.TrimSpace(result.ResponseID) != "" {
			shouldBindLegacyVideoTask = true
		}
		if shouldBindLegacyVideoTask {
			if err := h.gatewayService.BindGrokMediaVideoRequestAccount(
				requestCtx, apiKey.GroupID, result.ResponseID, subject.UserID, apiKey.ID, account.ID,
			); err != nil {
				reqLog.Warn("grok_media.bind_video_request_account_failed",
					zap.Int64("account_id", account.ID),
					zap.String("request_id", result.ResponseID),
					zap.Error(err),
				)
			}
			// Defer billing until status polling observes video.url. Persist create-time
			// model/duration/resolution so status can still price if upstream omits them.
			// Retry once: missing pending causes silent underpricing (status omits resolution).
			pending := service.GrokVideoPendingBilling{Platform: account.Platform,
				Model:                requestModel,
				BillingModel:         firstNonEmptyString(result.BillingModel, requestModel),
				UpstreamModel:        result.UpstreamModel,
				VideoResolution:      result.VideoResolution,
				VideoDurationSeconds: result.VideoDurationSeconds,
				OriginalModel:        clientRequestedModel(c, requestModel),
				// Wall-clock start for usage duration_ms: create accepted → first done discovery.
				CreatedAt: videoCreateStartedAt,
			}
			if err := h.gatewayService.StoreGrokVideoPendingBilling(requestCtx, result.ResponseID, subject.UserID, apiKey.ID, pending); err != nil {
				reqLog.Warn("grok_media.store_video_pending_billing_failed_retrying",
					zap.Int64("account_id", account.ID),
					zap.String("request_id", result.ResponseID),
					zap.Error(err),
				)
				if err2 := h.gatewayService.StoreGrokVideoPendingBilling(requestCtx, result.ResponseID, subject.UserID, apiKey.ID, pending); err2 != nil {
					// Response body may already be committed; completion path will fail-closed
					// when pending is still missing and status cannot price duration.
					reqLog.Error("grok_media.store_video_pending_billing_failed",
						zap.Int64("account_id", account.ID),
						zap.String("request_id", result.ResponseID),
						zap.Error(err2),
					)
				}
			}
		}
		// Status poll OR content download can observe official done+video.url.
		// Both paths share the same claim key so the customer is charged once.
		if endpoint == service.SeedanceEndpointStatus {
			if billResult := prepareSeedanceCompletionBilling(requestCtx, h, apiKey, subject, requestID, result); billResult != nil {
				recordGrokMediaUsage(c, h, reqLog, apiKey, subject, subscription, account, billResult, billResult.Model, body, requestID)
			}
		} else if durableTask != nil {
			// The durable worker/gate already performed atomic settlement.
			// Never enter Redis claim/legacy billing for this task again.
		} else if endpoint == service.GrokMediaEndpointVideoStatus || endpoint == service.GrokMediaEndpointVideoContent {
			taskID := strings.TrimSpace(requestID)
			if billResult := prepareGrokVideoCompletionBilling(requestCtx, h, reqLog, apiKey, subject, taskID, result); billResult != nil {
				recordGrokMediaUsage(c, h, reqLog, apiKey, subject, subscription, account, billResult, billResult.Model, body, taskID)
			}
		} else if shouldRecordGrokMediaUsage(endpoint, requestModel, result) {
			recordGrokMediaUsage(c, h, reqLog, apiKey, subject, subscription, account, result, requestModel, body, requestID)
		}
		reqLog.Debug("grok_media.request_completed",
			zap.Int64("account_id", account.ID),
			zap.Int("switch_count", switchCount),
		)
		return
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
	if endpoint.IsSeedance() {
		return service.OpenAIEndpointCapabilitySeedance
	}
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

func isGrokVideoCreateEndpoint(endpoint service.GrokMediaEndpoint) bool {
	switch endpoint {
	case service.SeedanceEndpointCreate, service.GrokMediaEndpointVideosGenerations,
		service.GrokMediaEndpointVideosEdits,
		service.GrokMediaEndpointVideosExtensions:
		return true
	default:
		return false
	}
}

// shouldRecordGrokMediaUsage gates usage writes for immediate (image) generation.
// Async video create never bills here — status polling does on official
// status=done with video.url (docs.x.ai Video Generation).
// Status/content polls, empty model, and failed generations with zero billable
// image units never bill via this helper.
func shouldRecordGrokMediaUsage(endpoint service.GrokMediaEndpoint, requestModel string, result *service.OpenAIForwardResult) bool {
	if result == nil {
		return false
	}
	if isGrokVideoCreateEndpoint(endpoint) || endpoint.IsVideoLookupRequest() {
		return false
	}
	if !endpoint.IsGenerationRequest() || strings.TrimSpace(requestModel) == "" {
		return false
	}
	return result.ImageCount > 0
}

// prepareGrokVideoCompletionBilling claims one-shot billing for official done+video.url
// observations (status poll or content download). Duration/model prefer status body;
// resolution uses create-time request (status response does not document resolution).
func prepareGrokVideoCompletionBilling(
	ctx context.Context,
	h *OpenAIGatewayHandler,
	reqLog *zap.Logger,
	apiKey *service.APIKey,
	subject middleware2.AuthSubject,
	taskRequestID string,
	statusResult *service.OpenAIForwardResult,
) *service.OpenAIForwardResult {
	if h == nil || h.gatewayService == nil || apiKey == nil || statusResult == nil {
		return nil
	}
	// Forward already set VideoCount only when status=done && video.url (official).
	if statusResult.VideoCount <= 0 {
		return nil
	}
	taskRequestID = strings.TrimSpace(firstNonEmptyString(taskRequestID, statusResult.ResponseID))
	if taskRequestID == "" {
		return nil
	}
	// Load create-time snapshot before claim so we can fail-closed without burning the claim
	// when Redis lost pending and status cannot price the job.
	pending, loadErr := h.gatewayService.LoadGrokVideoPendingBilling(ctx, taskRequestID, subject.UserID, apiKey.ID)
	if loadErr != nil {
		reqLog.Warn("grok_media.video_pending_billing_load_failed", zap.String("request_id", taskRequestID), zap.Error(loadErr))
	}
	if pending == nil {
		if apiKey.Group != nil && apiKey.Group.Platform == service.PlatformOpenAI {
			reqLog.Error("openai_video.billing_skipped_missing_pending", zap.String("request_id", taskRequestID))
			return nil
		}
		// Status omits resolution; without pending we would silently default to 480p and underbill.
		// Allow billing only when official status carries duration (still may default resolution).
		if statusResult.VideoDurationSeconds <= 0 {
			reqLog.Error("grok_media.video_billing_skipped_missing_pending",
				zap.String("request_id", taskRequestID),
				zap.String("reason", "no create-time snapshot and status has no video.duration"),
			)
			return nil
		}
		reqLog.Error("grok_media.video_billing_without_pending",
			zap.String("request_id", taskRequestID),
			zap.Int("status_duration_seconds", statusResult.VideoDurationSeconds),
			zap.String("note", "resolution falls back to default 480p; investigate pending store failures"),
		)
	}
	if apiKey.Group != nil && apiKey.Group.Platform == service.PlatformOpenAI && pending != nil &&
		statusResult.VideoDurationSeconds <= 0 && pending.VideoDurationSeconds <= 0 {
		reqLog.Error("openai_video.billing_skipped_missing_duration", zap.String("request_id", taskRequestID))
		return nil
	}
	claimed, err := h.gatewayService.ClaimGrokVideoBilling(ctx, taskRequestID, subject.UserID, apiKey.ID)
	if err != nil {
		reqLog.Warn("grok_media.video_billing_claim_failed", zap.String("request_id", taskRequestID), zap.Error(err))
		return nil
	}
	if !claimed {
		reqLog.Debug("grok_media.video_billing_already_claimed", zap.String("request_id", taskRequestID))
		return nil
	}
	// Re-merge with pending: resolution is request-only; model/duration fill gaps.
	merged := *statusResult
	if pending != nil {
		if strings.TrimSpace(merged.Model) == "" {
			merged.Model = firstNonEmptyString(pending.BillingModel, pending.Model, pending.OriginalModel)
		}
		if strings.TrimSpace(merged.BillingModel) == "" {
			merged.BillingModel = firstNonEmptyString(pending.BillingModel, pending.Model, merged.Model)
		}
		if strings.TrimSpace(merged.UpstreamModel) == "" {
			merged.UpstreamModel = pending.UpstreamModel
		}
		// Official status omits resolution — always prefer create request.
		if strings.TrimSpace(pending.VideoResolution) != "" {
			merged.VideoResolution = pending.VideoResolution
		}
		if merged.VideoDurationSeconds <= 0 {
			merged.VideoDurationSeconds = pending.VideoDurationSeconds
		}
		if strings.TrimSpace(merged.ResponseID) == "" {
			merged.ResponseID = taskRequestID
		}
	}
	if strings.TrimSpace(merged.Model) == "" {
		merged.Model = "grok-imagine-video"
	}
	if strings.TrimSpace(merged.BillingModel) == "" {
		merged.BillingModel = merged.Model
	}
	// Always force durable task id so usage_billing_dedup survives multi-poll +
	// context-local request ids (do not prefer empty-only fill).
	merged.RequestID = service.StableGrokVideoBillingRequestID(firstNonEmptyString(merged.ResponseID, taskRequestID))
	merged.ResponseID = firstNonEmptyString(merged.ResponseID, taskRequestID)
	merged.VideoCount = 1
	// Pure video: do not keep legacy ImageCount (avoids image-path heuristics).
	merged.ImageCount = 0
	// Official default resolution is 480p when the create request omitted it.
	merged.VideoResolution = service.NormalizeVideoBillingResolutionOrDefault(merged.VideoResolution)
	// Official default duration is 8s when neither status nor create provided it.
	merged.VideoDurationSeconds = service.NormalizeVideoDurationForPlatform(apiKey.Group, merged.VideoDurationSeconds)
	// E2E latency for async video: create accept → this discovery of done+url.
	// Bill on discovery (status/content), not after further client polls; duration
	// must not be only the single discovery hop (~hundreds of ms).
	if pending != nil {
		if e2e := service.GrokVideoE2EDuration(pending.CreatedAt, time.Now()); e2e > 0 {
			merged.Duration = e2e
		}
	}
	return &merged
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
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
	// Async video: force durable task request id and release claim if billing fails.
	videoTaskID := ""
	if result != nil && result.VideoCount > 0 {
		service.SaveGrokVideoPreviewAsync(result.VideoURL, firstNonEmptyString(result.ResponseID, requestID), apiKey.ID)
	}
	if result != nil && (result.VideoCount > 0 || strings.HasPrefix(result.ResponseID, "seedance:")) {
		videoTaskID = strings.TrimSpace(firstNonEmptyString(requestID, result.ResponseID))
		if stable := service.StableGrokVideoBillingRequestID(firstNonEmptyString(result.ResponseID, requestID)); stable != "" {
			result.RequestID = stable
		}
		// Prefer task id hash for payload fingerprint stability across status/content.
		if len(body) == 0 && videoTaskID != "" {
			payloadForHash = []byte(videoTaskID)
		}
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
			if videoTaskID != "" {
				if releaseErr := h.gatewayService.ReleaseGrokVideoBilling(ctx, videoTaskID, subject.UserID, apiKey.ID); releaseErr != nil {
					reqLog.Warn("grok_media.video_billing_claim_release_failed",
						zap.String("request_id", videoTaskID),
						zap.Error(releaseErr),
					)
				}
			}
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
