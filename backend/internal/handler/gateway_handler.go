package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"sub2api/internal/middleware"
	"sub2api/internal/model"
	"sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

const (
	// Maximum wait time for concurrency slot
	maxConcurrencyWait = 60 * time.Second
	// Ping interval during wait
	pingInterval = 5 * time.Second
)

// GatewayHandler handles API gateway requests
type GatewayHandler struct {
	gatewayService      *service.GatewayService
	userService         *service.UserService
	concurrencyService  *service.ConcurrencyService
	billingCacheService *service.BillingCacheService
REDACTED

// NewGatewayHandler creates a new GatewayHandler
func NewGatewayHandler(gatewayService *service.GatewayService, userService *service.UserService, concurrencyService *service.ConcurrencyService, billingCacheService *service.BillingCacheService) *GatewayHandler {
	return &GatewayHandler{
		gatewayService:      gatewayService,
		userService:         userService,
		concurrencyService:  concurrencyService,
		billingCacheService: billingCacheService,
REDACTED
REDACTED

// Messages handles Claude API compatible messages endpoint
// POST /v1/messages
func (h *GatewayHandler) Messages(c *gin.Context) {
	// 从context获取apiKey和user（ApiKeyAuth中间件已设置）
	apiKey, ok := middleware.GetApiKeyFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
REDACTED

	user, ok := middleware.GetUserFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
REDACTED

	// 读取请求体
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
REDACTED

	if len(body) == 0 {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return
REDACTED

	// 解析请求获取模型名和stream
	var req struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
REDACTED
	if err := json.Unmarshal(body, &req); err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
REDACTED

	// Track if we've started streaming (for error handling)
	streamStarted := false

	// 获取订阅信息（可能为nil）- 提前获取用于后续检查
	subscription, _ := middleware.GetSubscriptionFromContext(c)

	// 0. 检查wait队列是否已满
	maxWait := service.CalculateMaxWait(user.Concurrency)
	canWait, err := h.concurrencyService.IncrementWaitCount(c.Request.Context(), user.ID, maxWait)
	if err != nil {
		log.Printf("Increment wait count failed: %v", err)
		// On error, allow request to proceed
REDACTED else if !canWait {
		h.errorResponse(c, http.StatusTooManyRequests, "rate_limit_error", "Too many pending requests, please retry later")
		return
REDACTED
	// 确保在函数退出时减少wait计数
	defer h.concurrencyService.DecrementWaitCount(c.Request.Context(), user.ID)

	// 1. 首先获取用户并发槽位
	userReleaseFunc, err := h.acquireUserSlotWithWait(c, user, req.Stream, &streamStarted)
	if err != nil {
		log.Printf("User concurrency acquire failed: %v", err)
		h.handleConcurrencyError(c, err, "user", streamStarted)
		return
REDACTED
	if userReleaseFunc != nil {
		defer userReleaseFunc()
REDACTED

	// 2. 【新增】Wait后二次检查余额/订阅
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), user, apiKey, apiKey.Group, subscription); err != nil {
		log.Printf("Billing eligibility check failed after wait: %v", err)
		h.handleStreamingAwareError(c, http.StatusForbidden, "billing_error", err.Error(), streamStarted)
		return
REDACTED

	// 计算粘性会话hash
	sessionHash := h.gatewayService.GenerateSessionHash(body)

	// 选择支持该模型的账号
	account, err := h.gatewayService.SelectAccountForModel(c.Request.Context(), apiKey.GroupID, sessionHash, req.Model)
	if err != nil {
		h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "No available accounts: "+err.Error(), streamStarted)
		return
REDACTED

	// 3. 获取账号并发槽位
	accountReleaseFunc, err := h.acquireAccountSlotWithWait(c, account, req.Stream, &streamStarted)
	if err != nil {
		log.Printf("Account concurrency acquire failed: %v", err)
		h.handleConcurrencyError(c, err, "account", streamStarted)
		return
REDACTED
	if accountReleaseFunc != nil {
		defer accountReleaseFunc()
REDACTED

	// 转发请求
	result, err := h.gatewayService.Forward(c.Request.Context(), c, account, body)
	if err != nil {
		// 错误响应已在Forward中处理，这里只记录日志
		log.Printf("Forward request failed: %v", err)
		return
REDACTED

	// 异步记录使用量（subscription已在函数开头获取）
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := h.gatewayService.RecordUsage(ctx, &service.RecordUsageInput{
			Result:       result,
			ApiKey:       apiKey,
			User:         user,
			Account:      account,
			Subscription: subscription,
	REDACTED); err != nil {
			log.Printf("Record usage failed: %v", err)
	REDACTED
REDACTED()
REDACTED

// acquireUserSlotWithWait acquires a user concurrency slot, waiting if necessary
// For streaming requests, sends ping events during the wait
// streamStarted is updated if streaming response has begun
func (h *GatewayHandler) acquireUserSlotWithWait(c *gin.Context, user *model.User, isStream bool, streamStarted *bool) (func(), error) {
	ctx := c.Request.Context()

	// Try to acquire immediately
	result, err := h.concurrencyService.AcquireUserSlot(ctx, user.ID, user.Concurrency)
	if err != nil {
		return nil, err
REDACTED

	if result.Acquired {
		return result.ReleaseFunc, nil
REDACTED

	// Need to wait - handle streaming ping if needed
	return h.waitForSlotWithPing(c, "user", user.ID, user.Concurrency, isStream, streamStarted)
REDACTED

// acquireAccountSlotWithWait acquires an account concurrency slot, waiting if necessary
// For streaming requests, sends ping events during the wait
// streamStarted is updated if streaming response has begun
func (h *GatewayHandler) acquireAccountSlotWithWait(c *gin.Context, account *model.Account, isStream bool, streamStarted *bool) (func(), error) {
	ctx := c.Request.Context()

	// Try to acquire immediately
	result, err := h.concurrencyService.AcquireAccountSlot(ctx, account.ID, account.Concurrency)
	if err != nil {
		return nil, err
REDACTED

	if result.Acquired {
		return result.ReleaseFunc, nil
REDACTED

	// Need to wait - handle streaming ping if needed
	return h.waitForSlotWithPing(c, "account", account.ID, account.Concurrency, isStream, streamStarted)
REDACTED

// concurrencyError represents a concurrency limit error with context
type concurrencyError struct {
	SlotType  string
	IsTimeout bool
REDACTED

func (e *concurrencyError) Error() string {
	if e.IsTimeout {
		return fmt.Sprintf("timeout waiting for %s concurrency slot", e.SlotType)
REDACTED
	return fmt.Sprintf("%s concurrency limit reached", e.SlotType)
REDACTED

// waitForSlotWithPing waits for a concurrency slot, sending ping events for streaming requests
// Note: For streaming requests, we send ping to keep the connection alive.
// streamStarted pointer is updated when streaming begins (for proper error handling by caller)
func (h *GatewayHandler) waitForSlotWithPing(c *gin.Context, slotType string, id int64, maxConcurrency int, isStream bool, streamStarted *bool) (func(), error) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), maxConcurrencyWait)
	defer cancel()

	// For streaming requests, set up SSE headers for ping
	var flusher http.Flusher
	if isStream {
		var ok bool
		flusher, ok = c.Writer.(http.Flusher)
		if !ok {
			return nil, fmt.Errorf("streaming not supported")
	REDACTED
REDACTED

	pingTicker := time.NewTicker(pingInterval)
	defer pingTicker.Stop()

	pollTicker := time.NewTicker(100 * time.Millisecond)
	defer pollTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, &concurrencyError{
				SlotType:  slotType,
				IsTimeout: true,
		REDACTED

		case <-pingTicker.C:
			// Send ping for streaming requests to keep connection alive
			if isStream && flusher != nil {
				// Set headers on first ping (lazy initialization)
				if !*streamStarted {
					c.Header("Content-Type", "text/event-stream")
					c.Header("Cache-Control", "no-cache")
					c.Header("Connection", "keep-alive")
					c.Header("X-Accel-Buffering", "no")
					*streamStarted = true
			REDACTED
				fmt.Fprintf(c.Writer, "data: {\"type\": \"ping\"REDACTED\n\n")
				flusher.Flush()
		REDACTED

		case <-pollTicker.C:
			// Try to acquire slot
			var result *service.AcquireResult
			var err error

			if slotType == "user" {
				result, err = h.concurrencyService.AcquireUserSlot(ctx, id, maxConcurrency)
		REDACTED else {
				result, err = h.concurrencyService.AcquireAccountSlot(ctx, id, maxConcurrency)
		REDACTED

			if err != nil {
				return nil, err
		REDACTED

			if result.Acquired {
				return result.ReleaseFunc, nil
		REDACTED
	REDACTED
REDACTED
REDACTED

// Models handles listing available models
// GET /v1/models
func (h *GatewayHandler) Models(c *gin.Context) {
	models := []gin.H{
		{
			"id":           "claude-opus-4-5-20251101",
			"type":         "model",
			"display_name": "Claude Opus 4.5",
			"created_at":   "2025-11-01T00:00:00Z",
	REDACTED,
		{
			"id":           "claude-sonnet-4-5-20250929",
			"type":         "model",
			"display_name": "Claude Sonnet 4.5",
			"created_at":   "2025-09-29T00:00:00Z",
	REDACTED,
		{
			"id":           "REDACTED",
			"type":         "model",
			"display_name": "Claude Haiku 4.5",
			"created_at":   "2025-10-01T00:00:00Z",
	REDACTED,
REDACTED

	c.JSON(http.StatusOK, gin.H{
		"data":   models,
		"object": "list",
REDACTED)
REDACTED

// Usage handles getting account balance for CC Switch integration
// GET /v1/usage
func (h *GatewayHandler) Usage(c *gin.Context) {
	apiKey, ok := middleware.GetApiKeyFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
REDACTED

	user, ok := middleware.GetUserFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
REDACTED

	// 订阅模式：返回订阅限额信息
	if apiKey.Group != nil && apiKey.Group.IsSubscriptionType() {
		subscription, ok := middleware.GetSubscriptionFromContext(c)
		if !ok {
			h.errorResponse(c, http.StatusForbidden, "subscription_error", "No active subscription")
			return
	REDACTED

		remaining := h.calculateSubscriptionRemaining(apiKey.Group, subscription)
		c.JSON(http.StatusOK, gin.H{
			"isValid":   true,
			"planName":  apiKey.Group.Name,
			"remaining": remaining,
			"unit":      "USD",
	REDACTED)
		return
REDACTED

	// 余额模式：返回钱包余额
	latestUser, err := h.userService.GetByID(c.Request.Context(), user.ID)
	if err != nil {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "Failed to get user info")
		return
REDACTED

	c.JSON(http.StatusOK, gin.H{
		"isValid":   true,
		"planName":  "钱包余额",
		"remaining": latestUser.Balance,
		"unit":      "USD",
REDACTED)
REDACTED

// calculateSubscriptionRemaining 计算订阅剩余可用额度
// 逻辑：
// 1. 如果日/周/月任一限额达到100%，返回0
// 2. 否则返回所有已配置周期中剩余额度的最小值
func (h *GatewayHandler) calculateSubscriptionRemaining(group *model.Group, sub *model.UserSubscription) float64 {
	var remainingValues []float64

	// 检查日限额
	if group.HasDailyLimit() {
		remaining := *group.DailyLimitUSD - sub.DailyUsageUSD
		if remaining <= 0 {
			return 0
	REDACTED
		remainingValues = append(remainingValues, remaining)
REDACTED

	// 检查周限额
	if group.HasWeeklyLimit() {
		remaining := *group.WeeklyLimitUSD - sub.WeeklyUsageUSD
		if remaining <= 0 {
			return 0
	REDACTED
		remainingValues = append(remainingValues, remaining)
REDACTED

	// 检查月限额
	if group.HasMonthlyLimit() {
		remaining := *group.MonthlyLimitUSD - sub.MonthlyUsageUSD
		if remaining <= 0 {
			return 0
	REDACTED
		remainingValues = append(remainingValues, remaining)
REDACTED

	// 如果没有配置任何限额，返回-1表示无限制
	if len(remainingValues) == 0 {
		return -1
REDACTED

	// 返回最小值
	min := remainingValues[0]
	for _, v := range remainingValues[1:] {
		if v < min {
			min = v
	REDACTED
REDACTED
	return min
REDACTED

// handleConcurrencyError handles concurrency-related errors with proper 429 response
func (h *GatewayHandler) handleConcurrencyError(c *gin.Context, err error, slotType string, streamStarted bool) {
	h.handleStreamingAwareError(c, http.StatusTooManyRequests, "rate_limit_error",
		fmt.Sprintf("Concurrency limit exceeded for %s, please retry later", slotType), streamStarted)
REDACTED

// handleStreamingAwareError handles errors that may occur after streaming has started
func (h *GatewayHandler) handleStreamingAwareError(c *gin.Context, status int, errType, message string, streamStarted bool) {
	if streamStarted {
		// Stream already started, send error as SSE event then close
		flusher, ok := c.Writer.(http.Flusher)
		if ok {
			// Send error event in SSE format
			errorEvent := fmt.Sprintf(`data: {"type": "error", "error": {"type": "%s", "message": "%s"REDACTEDREDACTED`+"\n\n", errType, message)
			fmt.Fprint(c.Writer, errorEvent)
			flusher.Flush()
	REDACTED
		return
REDACTED

	// Normal case: return JSON response with proper status code
	h.errorResponse(c, status, errType, message)
REDACTED

// errorResponse 返回Claude API格式的错误响应
func (h *GatewayHandler) errorResponse(c *gin.Context, status int, errType, message string) {
	c.JSON(status, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    errType,
			"message": message,
	REDACTED,
REDACTED)
REDACTED

// CountTokens handles token counting endpoint
// POST /v1/messages/count_tokens
// 特点：校验订阅/余额，但不计算并发、不记录使用量
func (h *GatewayHandler) CountTokens(c *gin.Context) {
	// 从context获取apiKey和user（ApiKeyAuth中间件已设置）
	apiKey, ok := middleware.GetApiKeyFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
REDACTED

	user, ok := middleware.GetUserFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
REDACTED

	// 读取请求体
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
REDACTED

	if len(body) == 0 {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return
REDACTED

	// 解析请求获取模型名
	var req struct {
		Model string `json:"model"`
REDACTED
	if err := json.Unmarshal(body, &req); err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
REDACTED

	// 获取订阅信息（可能为nil）
	subscription, _ := middleware.GetSubscriptionFromContext(c)

	// 校验 billing eligibility（订阅/余额）
	// 【注意】不计算并发，但需要校验订阅/余额
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), user, apiKey, apiKey.Group, subscription); err != nil {
		h.errorResponse(c, http.StatusForbidden, "billing_error", err.Error())
		return
REDACTED

	// 计算粘性会话 hash
	sessionHash := h.gatewayService.GenerateSessionHash(body)

	// 选择支持该模型的账号
	account, err := h.gatewayService.SelectAccountForModel(c.Request.Context(), apiKey.GroupID, sessionHash, req.Model)
	if err != nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "No available accounts: "+err.Error())
		return
REDACTED

	// 转发请求（不记录使用量）
	if err := h.gatewayService.ForwardCountTokens(c.Request.Context(), c, account, body); err != nil {
		log.Printf("Forward count_tokens request failed: %v", err)
		// 错误响应已在 ForwardCountTokens 中处理
		return
REDACTED
REDACTED
