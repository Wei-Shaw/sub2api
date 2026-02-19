package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/util/soraerror"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"go.uber.org/zap"
)

// SoraGatewayHandler handles Sora chat completions requests
type SoraGatewayHandler struct {
	gatewayService      *service.GatewayService
	soraGatewayService  *service.SoraGatewayService
	billingCacheService *service.BillingCacheService
	concurrencyHelper   *ConcurrencyHelper
	maxAccountSwitches  int
	streamMode          string
	soraMediaSigningKey string
	soraMediaRoot       string
REDACTED

// NewSoraGatewayHandler creates a new SoraGatewayHandler
func NewSoraGatewayHandler(
	gatewayService *service.GatewayService,
	soraGatewayService *service.SoraGatewayService,
	concurrencyService *service.ConcurrencyService,
	billingCacheService *service.BillingCacheService,
	cfg *config.Config,
) *SoraGatewayHandler {
	pingInterval := time.Duration(0)
	maxAccountSwitches := 3
	streamMode := "force"
	signKey := ""
	mediaRoot := "/app/data/sora"
	if cfg != nil {
		pingInterval = time.Duration(cfg.Concurrency.PingInterval) * time.Second
		if cfg.Gateway.MaxAccountSwitches > 0 {
			maxAccountSwitches = cfg.Gateway.MaxAccountSwitches
	REDACTED
		if mode := strings.TrimSpace(cfg.Gateway.SoraStreamMode); mode != "" {
			streamMode = mode
	REDACTED
		signKey = strings.TrimSpace(cfg.Gateway.SoraMediaSigningKey)
		if root := strings.TrimSpace(cfg.Sora.Storage.LocalPath); root != "" {
			mediaRoot = root
	REDACTED
REDACTED
	return &SoraGatewayHandler{
		gatewayService:      gatewayService,
		soraGatewayService:  soraGatewayService,
		billingCacheService: billingCacheService,
		concurrencyHelper:   NewConcurrencyHelper(concurrencyService, SSEPingFormatComment, pingInterval),
		maxAccountSwitches:  maxAccountSwitches,
		streamMode:          strings.ToLower(streamMode),
		soraMediaSigningKey: signKey,
		soraMediaRoot:       mediaRoot,
REDACTED
REDACTED

// ChatCompletions handles Sora /v1/chat/completions endpoint
func (h *SoraGatewayHandler) ChatCompletions(c *gin.Context) {
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
		"handler.sora_gateway.chat_completions",
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
	)

	body, err := io.ReadAll(c.Request.Body)
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

	setOpsRequestContext(c, "", false, body)

	// 校验请求体 JSON 合法性
	if !gjson.ValidBytes(body) {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
REDACTED

	// 使用 gjson 只读提取字段做校验，避免完整 Unmarshal
	modelResult := gjson.GetBytes(body, "model")
	if !modelResult.Exists() || modelResult.Type != gjson.String || modelResult.String() == "" {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
REDACTED
	reqModel := modelResult.String()

	msgsResult := gjson.GetBytes(body, "messages")
	if !msgsResult.IsArray() || len(msgsResult.Array()) == 0 {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "messages is required")
		return
REDACTED

	clientStream := gjson.GetBytes(body, "stream").Bool()
	reqLog = reqLog.With(zap.String("model", reqModel), zap.Bool("stream", clientStream))
	if !clientStream {
		if h.streamMode == "error" {
			h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Sora requires stream=true")
			return
	REDACTED
		var err error
		body, err = sjson.SetBytes(body, "stream", true)
		if err != nil {
			h.errorResponse(c, http.StatusInternalServerError, "api_error", "Failed to process request")
			return
	REDACTED
REDACTED

	setOpsRequestContext(c, reqModel, clientStream, body)

	platform := ""
	if forced, ok := middleware2.GetForcePlatformFromContext(c); ok {
		platform = forced
REDACTED else if apiKey.Group != nil {
		platform = apiKey.Group.Platform
REDACTED
	if platform != service.PlatformSora {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "This endpoint only supports Sora platform")
		return
REDACTED

	streamStarted := false
	subscription, _ := middleware2.GetSubscriptionFromContext(c)

	maxWait := service.CalculateMaxWait(subject.Concurrency)
	canWait, err := h.concurrencyHelper.IncrementWaitCount(c.Request.Context(), subject.UserID, maxWait)
	waitCounted := false
	if err != nil {
		reqLog.Warn("sora.user_wait_counter_increment_failed", zap.Error(err))
REDACTED else if !canWait {
		reqLog.Info("sora.user_wait_queue_full", zap.Int("max_wait", maxWait))
		h.errorResponse(c, http.StatusTooManyRequests, "rate_limit_error", "Too many pending requests, please retry later")
		return
REDACTED
	if err == nil && canWait {
		waitCounted = true
REDACTED
	defer func() {
		if waitCounted {
			h.concurrencyHelper.DecrementWaitCount(c.Request.Context(), subject.UserID)
	REDACTED
REDACTED()

	userReleaseFunc, err := h.concurrencyHelper.AcquireUserSlotWithWait(c, subject.UserID, subject.Concurrency, clientStream, &streamStarted)
	if err != nil {
		reqLog.Warn("sora.user_slot_acquire_failed", zap.Error(err))
		h.handleConcurrencyError(c, err, "user", streamStarted)
		return
REDACTED
	if waitCounted {
		h.concurrencyHelper.DecrementWaitCount(c.Request.Context(), subject.UserID)
		waitCounted = false
REDACTED
	userReleaseFunc = wrapReleaseOnDone(c.Request.Context(), userReleaseFunc)
	if userReleaseFunc != nil {
		defer userReleaseFunc()
REDACTED

	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription); err != nil {
		reqLog.Info("sora.billing_eligibility_check_failed", zap.Error(err))
		status, code, message := billingErrorDetails(err)
		h.handleStreamingAwareError(c, status, code, message, streamStarted)
		return
REDACTED

	sessionHash := generateOpenAISessionHash(c, body)

	maxAccountSwitches := h.maxAccountSwitches
	switchCount := 0
	failedAccountIDs := make(map[int64]struct{REDACTED)
	lastFailoverStatus := 0
	var lastFailoverBody []byte
	var lastFailoverHeaders http.Header

	for {
		selection, err := h.gatewayService.SelectAccountWithLoadAwareness(c.Request.Context(), apiKey.GroupID, sessionHash, reqModel, failedAccountIDs, "")
		if err != nil {
			reqLog.Warn("sora.account_select_failed",
				zap.Error(err),
				zap.Int("excluded_account_count", len(failedAccountIDs)),
			)
			if len(failedAccountIDs) == 0 {
				h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "No available accounts: "+err.Error(), streamStarted)
				return
		REDACTED
			h.handleFailoverExhausted(c, lastFailoverStatus, lastFailoverHeaders, lastFailoverBody, streamStarted)
			return
	REDACTED
		account := selection.Account
		setOpsSelectedAccount(c, account.ID, account.Platform)

		accountReleaseFunc := selection.ReleaseFunc
		if !selection.Acquired {
			if selection.WaitPlan == nil {
				h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "No available accounts", streamStarted)
				return
		REDACTED
			accountWaitCounted := false
			canWait, err := h.concurrencyHelper.IncrementAccountWaitCount(c.Request.Context(), account.ID, selection.WaitPlan.MaxWaiting)
			if err != nil {
				reqLog.Warn("sora.account_wait_counter_increment_failed", zap.Int64("account_id", account.ID), zap.Error(err))
		REDACTED else if !canWait {
				reqLog.Info("sora.account_wait_queue_full",
					zap.Int64("account_id", account.ID),
					zap.Int("max_waiting", selection.WaitPlan.MaxWaiting),
				)
				h.handleStreamingAwareError(c, http.StatusTooManyRequests, "rate_limit_error", "Too many pending requests, please retry later", streamStarted)
				return
		REDACTED
			if err == nil && canWait {
				accountWaitCounted = true
		REDACTED
			defer func() {
				if accountWaitCounted {
					h.concurrencyHelper.DecrementAccountWaitCount(c.Request.Context(), account.ID)
			REDACTED
		REDACTED()

			accountReleaseFunc, err = h.concurrencyHelper.AcquireAccountSlotWithWaitTimeout(
				c,
				account.ID,
				selection.WaitPlan.MaxConcurrency,
				selection.WaitPlan.Timeout,
				clientStream,
				&streamStarted,
			)
			if err != nil {
				reqLog.Warn("sora.account_slot_acquire_failed", zap.Int64("account_id", account.ID), zap.Error(err))
				h.handleConcurrencyError(c, err, "account", streamStarted)
				return
		REDACTED
			if accountWaitCounted {
				h.concurrencyHelper.DecrementAccountWaitCount(c.Request.Context(), account.ID)
				accountWaitCounted = false
		REDACTED
	REDACTED
		accountReleaseFunc = wrapReleaseOnDone(c.Request.Context(), accountReleaseFunc)

		result, err := h.soraGatewayService.Forward(c.Request.Context(), c, account, body, clientStream)
		if accountReleaseFunc != nil {
			accountReleaseFunc()
	REDACTED
		if err != nil {
			var failoverErr *service.UpstreamFailoverError
			if errors.As(err, &failoverErr) {
				failedAccountIDs[account.ID] = struct{REDACTED{REDACTED
				if switchCount >= maxAccountSwitches {
					lastFailoverStatus = failoverErr.StatusCode
					lastFailoverHeaders = failoverErr.ResponseHeaders
					lastFailoverBody = failoverErr.ResponseBody
					h.handleFailoverExhausted(c, lastFailoverStatus, lastFailoverHeaders, lastFailoverBody, streamStarted)
					return
			REDACTED
				lastFailoverStatus = failoverErr.StatusCode
				lastFailoverHeaders = failoverErr.ResponseHeaders
				lastFailoverBody = failoverErr.ResponseBody
				switchCount++
				upstreamErrCode, upstreamErrMsg := extractUpstreamErrorCodeAndMessage(lastFailoverBody)
				reqLog.Warn("sora.upstream_failover_switching",
					zap.Int64("account_id", account.ID),
					zap.Int("upstream_status", failoverErr.StatusCode),
					zap.String("upstream_error_code", upstreamErrCode),
					zap.String("upstream_error_message", upstreamErrMsg),
					zap.Int("switch_count", switchCount),
					zap.Int("max_switches", maxAccountSwitches),
				)
				continue
		REDACTED
			reqLog.Error("sora.forward_failed", zap.Int64("account_id", account.ID), zap.Error(err))
			return
	REDACTED

		userAgent := c.GetHeader("User-Agent")
		clientIP := ip.GetClientIP(c)

		go func(result *service.ForwardResult, usedAccount *service.Account, ua, ip string) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := h.gatewayService.RecordUsage(ctx, &service.RecordUsageInput{
				Result:       result,
				APIKey:       apiKey,
				User:         apiKey.User,
				Account:      usedAccount,
				Subscription: subscription,
				UserAgent:    ua,
				IPAddress:    ip,
		REDACTED); err != nil {
				logger.L().With(
					zap.String("component", "handler.sora_gateway.chat_completions"),
					zap.Int64("user_id", subject.UserID),
					zap.Int64("api_key_id", apiKey.ID),
					zap.Any("group_id", apiKey.GroupID),
					zap.String("model", reqModel),
					zap.Int64("account_id", usedAccount.ID),
				).Error("sora.record_usage_failed", zap.Error(err))
		REDACTED
	REDACTED(result, account, userAgent, clientIP)
		reqLog.Debug("sora.request_completed",
			zap.Int64("account_id", account.ID),
			zap.Int("switch_count", switchCount),
		)
		return
REDACTED
REDACTED

func generateOpenAISessionHash(c *gin.Context, body []byte) string {
	if c == nil {
		return ""
REDACTED
	sessionID := strings.TrimSpace(c.GetHeader("session_id"))
	if sessionID == "" {
		sessionID = strings.TrimSpace(c.GetHeader("conversation_id"))
REDACTED
	if sessionID == "" && len(body) > 0 {
		sessionID = strings.TrimSpace(gjson.GetBytes(body, "prompt_cache_key").String())
REDACTED
	if sessionID == "" {
		return ""
REDACTED
	hash := sha256.Sum256([]byte(sessionID))
	return hex.EncodeToString(hash[:])
REDACTED

func (h *SoraGatewayHandler) handleConcurrencyError(c *gin.Context, err error, slotType string, streamStarted bool) {
	h.handleStreamingAwareError(c, http.StatusTooManyRequests, "rate_limit_error",
		fmt.Sprintf("Concurrency limit exceeded for %s, please retry later", slotType), streamStarted)
REDACTED

func (h *SoraGatewayHandler) handleFailoverExhausted(c *gin.Context, statusCode int, responseHeaders http.Header, responseBody []byte, streamStarted bool) {
	status, errType, errMsg := h.mapUpstreamError(statusCode, responseHeaders, responseBody)
	h.handleStreamingAwareError(c, status, errType, errMsg, streamStarted)
REDACTED

func (h *SoraGatewayHandler) mapUpstreamError(statusCode int, responseHeaders http.Header, responseBody []byte) (int, string, string) {
	if isSoraCloudflareChallengeResponse(statusCode, responseHeaders, responseBody) {
		baseMsg := fmt.Sprintf("Sora request blocked by Cloudflare challenge (HTTP %d). Please switch to a clean proxy/network and retry.", statusCode)
		return http.StatusBadGateway, "upstream_error", formatSoraCloudflareChallengeMessage(baseMsg, responseHeaders, responseBody)
REDACTED

	upstreamCode, upstreamMessage := extractUpstreamErrorCodeAndMessage(responseBody)
	if strings.EqualFold(upstreamCode, "cf_shield_429") {
		baseMsg := "Sora request blocked by Cloudflare shield (429). Please switch to a clean proxy/network and retry."
		return http.StatusTooManyRequests, "rate_limit_error", formatSoraCloudflareChallengeMessage(baseMsg, responseHeaders, responseBody)
REDACTED
	if shouldPassthroughSoraUpstreamMessage(statusCode, upstreamMessage) {
		switch statusCode {
		case 401, 403, 404, 500, 502, 503, 504:
			return http.StatusBadGateway, "upstream_error", upstreamMessage
		case 429:
			return http.StatusTooManyRequests, "rate_limit_error", upstreamMessage
	REDACTED
REDACTED

	switch statusCode {
	case 401:
		return http.StatusBadGateway, "upstream_error", "Upstream authentication failed, please contact administrator"
	case 403:
		return http.StatusBadGateway, "upstream_error", "Upstream access forbidden, please contact administrator"
	case 404:
		if strings.EqualFold(upstreamCode, "unsupported_country_code") {
			return http.StatusBadGateway, "upstream_error", "Upstream region capability unavailable for this account, please contact administrator"
	REDACTED
		return http.StatusBadGateway, "upstream_error", "Upstream capability unavailable for this account, please contact administrator"
	case 429:
		return http.StatusTooManyRequests, "rate_limit_error", "Upstream rate limit exceeded, please retry later"
	case 529:
		return http.StatusServiceUnavailable, "upstream_error", "Upstream service overloaded, please retry later"
	case 500, 502, 503, 504:
		return http.StatusBadGateway, "upstream_error", "Upstream service temporarily unavailable"
	default:
		return http.StatusBadGateway, "upstream_error", "Upstream request failed"
REDACTED
REDACTED

func isSoraCloudflareChallengeResponse(statusCode int, headers http.Header, body []byte) bool {
	return soraerror.IsCloudflareChallengeResponse(statusCode, headers, body)
REDACTED

func shouldPassthroughSoraUpstreamMessage(statusCode int, message string) bool {
	message = strings.TrimSpace(message)
	if message == "" {
		return false
REDACTED
	if statusCode == http.StatusForbidden || statusCode == http.StatusTooManyRequests {
		lower := strings.ToLower(message)
		if strings.Contains(lower, "<html") || strings.Contains(lower, "<!doctype html") || strings.Contains(lower, "window._cf_chl_opt") {
			return false
	REDACTED
REDACTED
	return true
REDACTED

func formatSoraCloudflareChallengeMessage(base string, headers http.Header, body []byte) string {
	return soraerror.FormatCloudflareChallengeMessage(base, headers, body)
REDACTED

func extractUpstreamErrorCodeAndMessage(body []byte) (string, string) {
	return soraerror.ExtractUpstreamErrorCodeAndMessage(body)
REDACTED

func (h *SoraGatewayHandler) handleStreamingAwareError(c *gin.Context, status int, errType, message string, streamStarted bool) {
	if streamStarted {
		flusher, ok := c.Writer.(http.Flusher)
		if ok {
			errorData := map[string]any{
				"error": map[string]string{
					"type":    errType,
					"message": message,
			REDACTED,
		REDACTED
			jsonBytes, err := json.Marshal(errorData)
			if err != nil {
				_ = c.Error(err)
				return
		REDACTED
			errorEvent := fmt.Sprintf("event: error\ndata: %s\n\n", string(jsonBytes))
			if _, err := fmt.Fprint(c.Writer, errorEvent); err != nil {
				_ = c.Error(err)
		REDACTED
			flusher.Flush()
	REDACTED
		return
REDACTED
	h.errorResponse(c, status, errType, message)
REDACTED

func (h *SoraGatewayHandler) errorResponse(c *gin.Context, status int, errType, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"type":    errType,
			"message": message,
	REDACTED,
REDACTED)
REDACTED

// MediaProxy serves local Sora media files.
func (h *SoraGatewayHandler) MediaProxy(c *gin.Context) {
	h.proxySoraMedia(c, false)
REDACTED

// MediaProxySigned serves local Sora media files with signature verification.
func (h *SoraGatewayHandler) MediaProxySigned(c *gin.Context) {
	h.proxySoraMedia(c, true)
REDACTED

func (h *SoraGatewayHandler) proxySoraMedia(c *gin.Context, requireSignature bool) {
	rawPath := c.Param("filepath")
	if rawPath == "" {
		c.Status(http.StatusNotFound)
		return
REDACTED
	cleaned := path.Clean(rawPath)
	if !strings.HasPrefix(cleaned, "/image/") && !strings.HasPrefix(cleaned, "/video/") {
		c.Status(http.StatusNotFound)
		return
REDACTED

	query := c.Request.URL.Query()
	if requireSignature {
		if h.soraMediaSigningKey == "" {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": gin.H{
					"type":    "api_error",
					"message": "Sora 媒体签名未配置",
			REDACTED,
		REDACTED)
			return
	REDACTED
		expiresStr := strings.TrimSpace(query.Get("expires"))
		signature := strings.TrimSpace(query.Get("sig"))
		expires, err := strconv.ParseInt(expiresStr, 10, 64)
		if err != nil || expires <= time.Now().Unix() {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"type":    "authentication_error",
					"message": "Sora 媒体签名已过期",
			REDACTED,
		REDACTED)
			return
	REDACTED
		query.Del("sig")
		query.Del("expires")
		signingQuery := query.Encode()
		if !service.VerifySoraMediaURL(cleaned, signingQuery, expires, signature, h.soraMediaSigningKey) {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"type":    "authentication_error",
					"message": "Sora 媒体签名无效",
			REDACTED,
		REDACTED)
			return
	REDACTED
REDACTED
	if strings.TrimSpace(h.soraMediaRoot) == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": gin.H{
				"type":    "api_error",
				"message": "Sora 媒体目录未配置",
		REDACTED,
	REDACTED)
		return
REDACTED

	relative := strings.TrimPrefix(cleaned, "/")
	localPath := filepath.Join(h.soraMediaRoot, filepath.FromSlash(relative))
	if _, err := os.Stat(localPath); err != nil {
		if os.IsNotExist(err) {
			c.Status(http.StatusNotFound)
			return
	REDACTED
		c.Status(http.StatusInternalServerError)
		return
REDACTED
	c.File(localPath)
REDACTED
