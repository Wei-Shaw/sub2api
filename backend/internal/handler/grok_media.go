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
REDACTED
	h.handleGrokMedia(c, endpoint, "")
REDACTED

// GrokVideoGeneration handles xAI video generation through Grok groups.
func (h *OpenAIGatewayHandler) GrokVideoGeneration(c *gin.Context) {
	h.handleGrokMedia(c, service.GrokMediaEndpointVideosGenerations, "")
REDACTED

// GrokVideoEdit handles asynchronous xAI video edits through Grok groups.
func (h *OpenAIGatewayHandler) GrokVideoEdit(c *gin.Context) {
	h.handleGrokMedia(c, service.GrokMediaEndpointVideosEdits, "")
REDACTED

// GrokVideoExtension handles asynchronous xAI video extensions through Grok groups.
func (h *OpenAIGatewayHandler) GrokVideoExtension(c *gin.Context) {
	h.handleGrokMedia(c, service.GrokMediaEndpointVideosExtensions, "")
REDACTED

// GrokVideoStatus handles xAI video status retrieval through Grok groups.
func (h *OpenAIGatewayHandler) GrokVideoStatus(c *gin.Context) {
	h.handleGrokMedia(c, service.GrokMediaEndpointVideoStatus, c.Param("request_id"))
REDACTED

// GrokVideoContent proxies downloadable video content through the task's upstream account.
func (h *OpenAIGatewayHandler) GrokVideoContent(c *gin.Context) {
	h.handleGrokMedia(c, service.GrokMediaEndpointVideoContent, c.Param("request_id"))
REDACTED

func (h *OpenAIGatewayHandler) handleGrokMedia(c *gin.Context, endpoint service.GrokMediaEndpoint, requestID string) {
	streamStarted := false
	defer h.recoverResponsesPanic(c, &streamStarted)

	requestStart := time.Now()
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
REDACTED
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
REDACTED

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
REDACTED

	var body []byte
	var err error
	if endpoint.RequiresRequestBody() {
		body, err = pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
		if err != nil {
			if maxErr, ok := extractMaxBytesError(err); ok {
				h.errorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
				return
		REDACTED
			h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
			return
	REDACTED
		if len(body) == 0 {
			h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
			return
	REDACTED
REDACTED

	contentType := c.GetHeader("Content-Type")
	requestInfo := service.ParseGrokMediaRequest(contentType, body)
	requestModel := requestInfo.Model
	if endpoint.IsGenerationRequest() && strings.TrimSpace(requestModel) == "" {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
REDACTED
	if endpoint.IsVideoLookupRequest() && strings.TrimSpace(requestID) == "" {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "request_id is required")
		return
REDACTED

	reqLog = reqLog.With(zap.String("model", requestModel))
	setOpsRequestContext(c, requestModel, false)
	setOpsEndpointContext(c, "", int16(service.RequestTypeSync))

	if endpoint.IsGenerationRequest() {
		if !service.GroupAllowsImageGeneration(apiKey.Group) {
			h.errorResponse(c, http.StatusForbidden, "permission_error", service.ImageGenerationPermissionMessage())
			return
	REDACTED
		if moderationBody := requestInfo.ModerationBody(); len(moderationBody) > 0 {
			decision := h.checkSecurityAudit(c, reqLog, apiKey, subject, service.ContentModerationProtocolOpenAIImages, requestModel, moderationBody)
			if decision != nil && !decision.AllowNextStage {
				h.openAISecurityAuditError(c, decision)
				return
		REDACTED
	REDACTED
		imageReleaseFunc, acquired := h.acquireImageGenerationSlot(c, streamStarted)
		if !acquired {
			return
	REDACTED
		if imageReleaseFunc != nil {
			defer imageReleaseFunc()
	REDACTED
REDACTED

	if h.errorPassthroughService != nil {
		service.BindErrorPassthroughService(c, h.errorPassthroughService)
REDACTED

	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	service.SetOpsLatencyMs(c, service.OpsAuthLatencyMsKey, time.Since(requestStart).Milliseconds())

	userReleaseFunc, acquired := h.acquireResponsesUserSlot(c, subject.UserID, subject.Concurrency, false, &streamStarted, reqLog)
	if !acquired {
		return
REDACTED
	if userReleaseFunc != nil {
		defer userReleaseFunc()
REDACTED

	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
		reqLog.Info("grok_media.billing_eligibility_check_failed", zap.Error(err))
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
	REDACTED
		h.errorResponse(c, status, code, message)
		return
REDACTED

	sessionSeed := body
	if len(sessionSeed) == 0 && strings.TrimSpace(requestID) != "" {
		sessionSeed = []byte(requestID)
REDACTED
	sessionHash := h.gatewayService.GenerateExplicitSessionHash(c, sessionSeed)
	boundLookupAccountID := int64(0)
	if endpoint.IsVideoLookupRequest() {
		sessionHash = service.GrokMediaVideoRequestSessionHash(requestID, subject.UserID, apiKey.ID)
		boundLookupAccountID, err = h.gatewayService.ResolveGrokMediaVideoRequestAccount(
			c.Request.Context(), apiKey.GroupID, requestID, subject.UserID, apiKey.ID,
		)
		if err != nil || boundLookupAccountID <= 0 {
			reqLog.Info("grok_media.video_lookup_owner_binding_missing", zap.Error(err))
			h.errorResponse(c, http.StatusNotFound, "not_found_error", "Video request not found")
			return
	REDACTED
REDACTED
	requestCtx := c.Request.Context()
	failedAccountIDs := make(map[int64]struct{REDACTED)
	sameAccountRetryCount := make(map[int64]int)
	var lastFailoverErr *service.UpstreamFailoverError
	var oauth429FailoverState service.OpenAIOAuth429FailoverState
	mediaEligibilityRejected := false
	switchCount := 0
	maxAccountSwitches := h.maxAccountSwitches
	if maxAccountSwitches <= 0 {
		maxAccountSwitches = 3
REDACTED
	routingStart := time.Now()
	requiredCapability := grokMediaRequiredCapability(endpoint)

	for {
		if failoverClientGone(c) {
			return
	REDACTED
		selection, scheduleDecision, err := h.gatewayService.SelectAccountWithSchedulerForCapability(
			requestCtx,
			apiKey.GroupID,
			"",
			sessionHash,
			requestModel,
			failedAccountIDs,
			service.OpenAIUpstreamTransportHTTPSSE,
			requiredCapability,
			false,
			false,
			false,
			service.PlatformGrok,
		)
		if err != nil {
			if failoverClientGone(c) {
				reqLog.Info("grok_media.account_select_aborted_client_disconnected", zap.Error(err))
				return
		REDACTED
			reqLog.Warn("grok_media.account_select_failed",
				zap.Error(err),
				zap.Int("excluded_account_count", len(failedAccountIDs)),
			)
			if endpoint.IsGenerationRequest() && errors.Is(err, service.ErrNoAvailableAccounts) &&
				(len(failedAccountIDs) == 0 || (mediaEligibilityRejected && lastFailoverErr == nil)) {
				markOpsRoutingCapacityLimitedIfNoAvailable(c, err)
				h.errorResponse(c, http.StatusServiceUnavailable, "grok_media_no_eligible_account", "No eligible Grok media accounts")
				return
		REDACTED
			if len(failedAccountIDs) == 0 {
				cls := classifyNoAccountErrorFromGin(c, h.gatewayService, apiKey, requestModel, requestModel, service.PlatformGrok)
				if !cls.ModelNotFound {
					markOpsRoutingCapacityLimitedIfNoAvailable(c, err)
			REDACTED
				h.errorResponse(c, cls.Status, cls.ErrType, cls.Message)
				return
		REDACTED
			if lastFailoverErr != nil {
				h.handleFailoverExhausted(c, lastFailoverErr, false)
		REDACTED else {
				h.errorResponse(c, http.StatusBadGateway, "api_error", "Upstream request failed")
		REDACTED
			return
	REDACTED
		if selection == nil || selection.Account == nil {
			if endpoint.IsGenerationRequest() {
				markOpsRoutingCapacityLimited(c)
				h.errorResponse(c, http.StatusServiceUnavailable, "grok_media_no_eligible_account", "No eligible Grok media accounts")
				return
		REDACTED
			cls := classifyNoAccountErrorFromGin(c, h.gatewayService, apiKey, requestModel, requestModel, service.PlatformGrok)
			if !cls.ModelNotFound {
				markOpsRoutingCapacityLimited(c)
		REDACTED
			h.errorResponse(c, cls.Status, cls.ErrType, cls.Message)
			return
	REDACTED
		if boundLookupAccountID > 0 && selection.Account.ID != boundLookupAccountID {
			reqLog.Warn("grok_media.video_lookup_bound_account_unavailable",
				zap.Int64("bound_account_id", boundLookupAccountID),
				zap.Int64("selected_account_id", selection.Account.ID),
			)
			h.errorResponse(c, http.StatusNotFound, "not_found_error", "Video request not found")
			return
	REDACTED

		reqLog.Debug("grok_media.account_schedule_decision",
			zap.String("layer", scheduleDecision.Layer),
			zap.Bool("sticky_session_hit", scheduleDecision.StickySessionHit),
			zap.Int("candidate_count", scheduleDecision.CandidateCount),
			zap.Int("top_k", scheduleDecision.TopK),
			zap.Int64("latency_ms", scheduleDecision.LatencyMs),
			zap.Float64("load_skew", scheduleDecision.LoadSkew),
		)

		account := selection.Account
		if endpoint.IsGenerationRequest() {
			eligible, eligibilityReason, eligibilityErr := h.ensureGrokMediaAccountEligibility(requestCtx, account)
			if !eligible {
				mediaEligibilityRejected = true
				failedAccountIDs[account.ID] = struct{REDACTED{REDACTED
				reqLog.Warn("grok_media.account_eligibility_rejected",
					zap.Int64("account_id", account.ID),
					zap.String("reason", eligibilityReason),
					zap.Bool("probe_failed", eligibilityErr != nil),
				)
				if switchCount >= maxAccountSwitches {
					markOpsRoutingCapacityLimited(c)
					h.errorResponse(c, http.StatusServiceUnavailable, "grok_media_no_eligible_account", "No eligible Grok media accounts")
					return
			REDACTED
				switchCount++
				continue
		REDACTED
	REDACTED
		sessionHash = ensureOpenAIPoolModeSessionHash(sessionHash, account)
		setOpsSelectedAccount(c, account.ID, account.Platform)

		accountReleaseFunc, accountAcquired := h.acquireResponsesAccountSlot(c, apiKey.GroupID, sessionHash, selection, false, &streamStarted, reqLog)
		if !accountAcquired {
			return
	REDACTED

		service.SetOpsLatencyMs(c, service.OpsRoutingLatencyMsKey, time.Since(routingStart).Milliseconds())
		forwardStart := time.Now()
		writerSizeBeforeForward := c.Writer.Size()
		result, err := func() (*service.OpenAIForwardResult, error) {
			defer func() {
				if accountReleaseFunc != nil {
					accountReleaseFunc()
			REDACTED
		REDACTED()
			return h.gatewayService.ForwardGrokMedia(requestCtx, c, account, endpoint, requestID, body, contentType)
	REDACTED()

		forwardDurationMs := time.Since(forwardStart).Milliseconds()
		upstreamLatencyMs, _ := getContextInt64(c, service.OpsUpstreamLatencyMsKey)
		responseLatencyMs := forwardDurationMs
		if upstreamLatencyMs > 0 && forwardDurationMs > upstreamLatencyMs {
			responseLatencyMs = forwardDurationMs - upstreamLatencyMs
	REDACTED
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
			REDACTED
				if failoverErr.ShouldReportAccountScheduleFailure() {
					h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, account.GetMappedModel(requestModel), false, nil)
			REDACTED
				if c.Writer.Size() != writerSizeBeforeForward {
					h.handleFailoverExhausted(c, failoverErr, true)
					return
			REDACTED
				if !failoverErr.ShouldRetryNextAccount() {
					h.handleFailoverExhausted(c, failoverErr, false)
					return
			REDACTED
				if endpoint.IsVideoLookupRequest() {
					h.handleFailoverExhausted(c, failoverErr, false)
					return
			REDACTED
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
					REDACTED
						continue
				REDACTED
			REDACTED
				h.gatewayService.RecordOpenAIAccountSwitch()
				failedAccountIDs[account.ID] = struct{REDACTED{REDACTED
				lastFailoverErr = failoverErr
				if switchCount >= maxAccountSwitches {
					h.handleFailoverExhausted(c, failoverErr, false)
					return
			REDACTED
				switchCount++
				if h.gatewayService.ShouldStopOpenAIOAuth429Failover(account, failoverErr.StatusCode, switchCount, &oauth429FailoverState) {
					h.handleFailoverExhausted(c, failoverErr, false)
					return
			REDACTED
				reqLog.Warn("grok_media.upstream_failover_switching",
					zap.Int64("account_id", account.ID),
					zap.Int("upstream_status", failoverErr.StatusCode),
					zap.Int("switch_count", switchCount),
					zap.Int("max_switches", maxAccountSwitches),
				)
				continue
		REDACTED
			h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, account.GetMappedModel(requestModel), false, nil)
			if !service.IsResponseCommitted(c) && c.Writer.Size() == writerSizeBeforeForward {
				h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Upstream request failed")
		REDACTED
			reqLog.Warn("grok_media.forward_failed",
				zap.Int64("account_id", account.ID),
				zap.Error(err),
			)
			return
	REDACTED

		h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, account.GetMappedModel(requestModel), true, nil)
		if endpoint.IsGenerationRequest() && strings.TrimSpace(result.ResponseID) != "" {
			if err := h.gatewayService.BindGrokMediaVideoRequestAccount(
				requestCtx, apiKey.GroupID, result.ResponseID, subject.UserID, apiKey.ID, account.ID,
			); err != nil {
				reqLog.Warn("grok_media.bind_video_request_account_failed",
					zap.Int64("account_id", account.ID),
					zap.String("request_id", result.ResponseID),
					zap.Error(err),
				)
		REDACTED
	REDACTED
		if shouldRecordGrokMediaUsage(endpoint, requestModel) {
			recordGrokMediaUsage(c, h, reqLog, apiKey, subject, subscription, account, result, requestModel, body, requestID)
	REDACTED
		reqLog.Debug("grok_media.request_completed",
			zap.Int64("account_id", account.ID),
			zap.Int("switch_count", switchCount),
		)
		return
REDACTED
REDACTED

func (h *OpenAIGatewayHandler) ensureGrokMediaAccountEligibility(ctx context.Context, account *service.Account) (bool, string, error) {
	if account == nil {
		return false, "missing_account", errors.New("grok media account is required")
REDACTED
	eligible, reason := account.GrokMediaGenerationEligibility()
	if eligible || reason != "billing_unobserved" {
		return eligible, reason, nil
REDACTED
	if h == nil || h.grokMediaEligibilityProber == nil {
		return false, "billing_probe_unavailable", errors.New("grok media eligibility probe is not configured")
REDACTED
	return h.grokMediaEligibilityProber.ProbeMediaEligibility(ctx, account.ID)
REDACTED

func grokMediaRequiredCapability(endpoint service.GrokMediaEndpoint) service.OpenAIEndpointCapability {
	if endpoint.IsGenerationRequest() {
		return service.OpenAIEndpointCapabilityGrokMediaGeneration
REDACTED
	return ""
REDACTED

func shouldRecordGrokMediaUsage(endpoint service.GrokMediaEndpoint, requestModel string) bool {
	return endpoint.IsGenerationRequest() && strings.TrimSpace(requestModel) != ""
REDACTED

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
	payloadForHash := body
	if len(payloadForHash) == 0 && strings.TrimSpace(requestID) != "" {
		payloadForHash = []byte(requestID)
REDACTED
	inboundEndpoint := GetInboundEndpoint(c)
	upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)
	quotaPlatform := service.QuotaPlatform(c.Request.Context(), apiKey)
	channelUsageFields := service.ChannelUsageFields{
		OriginalModel:      requestModel,
		ChannelMappedModel: requestModel,
REDACTED
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
			ChannelUsageFields: channelUsageFields,
	REDACTED); err != nil {
			logger.L().With(
				zap.String("component", "handler.openai_gateway.grok_media"),
				zap.Int64("user_id", subject.UserID),
				zap.Int64("api_key_id", apiKey.ID),
				zap.Any("group_id", apiKey.GroupID),
				zap.String("model", requestModel),
				zap.Int64("account_id", account.ID),
			).Error("grok_media.record_usage_failed", zap.Error(err))
			reqLog.Debug("grok_media.record_usage_failed", zap.Error(err))
	REDACTED
REDACTED)
REDACTED
