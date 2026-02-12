package handler

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	pkgerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// GatewayHandler handles API gateway requests
type GatewayHandler struct {
	gatewayService            *service.GatewayService
	geminiCompatService       *service.GeminiMessagesCompatService
	antigravityGatewayService *service.AntigravityGatewayService
	userService               *service.UserService
	billingCacheService       *service.BillingCacheService
	usageService              *service.UsageService
	apiKeyService             *service.APIKeyService
	errorPassthroughService   *service.ErrorPassthroughService
	concurrencyHelper         *ConcurrencyHelper
	maxAccountSwitches        int
	maxAccountSwitchesGemini  int
	cfg                       *config.Config
REDACTED

// NewGatewayHandler creates a new GatewayHandler
func NewGatewayHandler(
	gatewayService *service.GatewayService,
	geminiCompatService *service.GeminiMessagesCompatService,
	antigravityGatewayService *service.AntigravityGatewayService,
	userService *service.UserService,
	concurrencyService *service.ConcurrencyService,
	billingCacheService *service.BillingCacheService,
	usageService *service.UsageService,
	apiKeyService *service.APIKeyService,
	errorPassthroughService *service.ErrorPassthroughService,
	cfg *config.Config,
) *GatewayHandler {
	pingInterval := time.Duration(0)
	maxAccountSwitches := 10
	maxAccountSwitchesGemini := 3
	if cfg != nil {
		pingInterval = time.Duration(cfg.Concurrency.PingInterval) * time.Second
		if cfg.Gateway.MaxAccountSwitches > 0 {
			maxAccountSwitches = cfg.Gateway.MaxAccountSwitches
	REDACTED
		if cfg.Gateway.MaxAccountSwitchesGemini > 0 {
			maxAccountSwitchesGemini = cfg.Gateway.MaxAccountSwitchesGemini
	REDACTED
REDACTED
	return &GatewayHandler{
		gatewayService:            gatewayService,
		geminiCompatService:       geminiCompatService,
		antigravityGatewayService: antigravityGatewayService,
		userService:               userService,
		billingCacheService:       billingCacheService,
		usageService:              usageService,
		apiKeyService:             apiKeyService,
		errorPassthroughService:   errorPassthroughService,
		concurrencyHelper:         NewConcurrencyHelper(concurrencyService, SSEPingFormatClaude, pingInterval),
		maxAccountSwitches:        maxAccountSwitches,
		maxAccountSwitchesGemini:  maxAccountSwitchesGemini,
		cfg:                       cfg,
REDACTED
REDACTED

// Messages handles Claude API compatible messages endpoint
// POST /v1/messages
func (h *GatewayHandler) Messages(c *gin.Context) {
	// 从context获取apiKey和user（ApiKeyAuth中间件已设置）
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

	// 读取请求体
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

	parsedReq, err := service.ParseGatewayRequest(body, domain.PlatformAnthropic)
	if err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
REDACTED
	reqModel := parsedReq.Model
	reqStream := parsedReq.Stream

	// 设置 max_tokens=1 + haiku 探测请求标识到 context 中
	// 必须在 SetClaudeCodeClientContext 之前设置，因为 ClaudeCodeValidator 需要读取此标识进行绕过判断
	if isMaxTokensOneHaikuRequest(reqModel, parsedReq.MaxTokens, reqStream) {
		ctx := context.WithValue(c.Request.Context(), ctxkey.IsMaxTokensOneHaikuRequest, true)
		c.Request = c.Request.WithContext(ctx)
REDACTED

	// 检查是否为 Claude Code 客户端，设置到 context 中
	SetClaudeCodeClientContext(c, body)
	isClaudeCodeClient := service.IsClaudeCodeClient(c.Request.Context())

	// 在请求上下文中记录 thinking 状态，供 Antigravity 最终模型 key 推导/模型维度限流使用
	c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ctxkey.ThinkingEnabled, parsedReq.ThinkingEnabled))

	setOpsRequestContext(c, reqModel, reqStream, body)

	// 验证 model 必填
	if reqModel == "" {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
REDACTED

	// Track if we've started streaming (for error handling)
	streamStarted := false

	// 绑定错误透传服务，允许 service 层在非 failover 错误场景复用规则。
	if h.errorPassthroughService != nil {
		service.BindErrorPassthroughService(c, h.errorPassthroughService)
REDACTED

	// 获取订阅信息（可能为nil）- 提前获取用于后续检查
	subscription, _ := middleware2.GetSubscriptionFromContext(c)

	// 0. 检查wait队列是否已满
	maxWait := service.CalculateMaxWait(subject.Concurrency)
	canWait, err := h.concurrencyHelper.IncrementWaitCount(c.Request.Context(), subject.UserID, maxWait)
	waitCounted := false
	if err != nil {
		log.Printf("Increment wait count failed: %v", err)
		// On error, allow request to proceed
REDACTED else if !canWait {
		h.errorResponse(c, http.StatusTooManyRequests, "rate_limit_error", "Too many pending requests, please retry later")
		return
REDACTED
	if err == nil && canWait {
		waitCounted = true
REDACTED
	// Ensure we decrement if we exit before acquiring the user slot.
	defer func() {
		if waitCounted {
			h.concurrencyHelper.DecrementWaitCount(c.Request.Context(), subject.UserID)
	REDACTED
REDACTED()

	// 1. 首先获取用户并发槽位
	userReleaseFunc, err := h.concurrencyHelper.AcquireUserSlotWithWait(c, subject.UserID, subject.Concurrency, reqStream, &streamStarted)
	if err != nil {
		log.Printf("User concurrency acquire failed: %v", err)
		h.handleConcurrencyError(c, err, "user", streamStarted)
		return
REDACTED
	// User slot acquired: no longer waiting in the queue.
	if waitCounted {
		h.concurrencyHelper.DecrementWaitCount(c.Request.Context(), subject.UserID)
		waitCounted = false
REDACTED
	// 在请求结束或 Context 取消时确保释放槽位，避免客户端断开造成泄漏
	userReleaseFunc = wrapReleaseOnDone(c.Request.Context(), userReleaseFunc)
	if userReleaseFunc != nil {
		defer userReleaseFunc()
REDACTED

	// 2. 【新增】Wait后二次检查余额/订阅
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription); err != nil {
		log.Printf("Billing eligibility check failed after wait: %v", err)
		status, code, message := billingErrorDetails(err)
		h.handleStreamingAwareError(c, status, code, message, streamStarted)
		return
REDACTED

	// 计算粘性会话hash
	parsedReq.SessionContext = &service.SessionContext{
		ClientIP:  ip.GetClientIP(c),
		UserAgent: c.GetHeader("User-Agent"),
		APIKeyID:  apiKey.ID,
REDACTED
	sessionHash := h.gatewayService.GenerateSessionHash(parsedReq)

	// 获取平台：优先使用强制平台（/antigravity 路由，中间件已设置 request.Context），否则使用分组平台
	platform := ""
	if forcePlatform, ok := middleware2.GetForcePlatformFromContext(c); ok {
		platform = forcePlatform
REDACTED else if apiKey.Group != nil {
		platform = apiKey.Group.Platform
REDACTED
	sessionKey := sessionHash
	if platform == service.PlatformGemini && sessionHash != "" {
		sessionKey = "gemini:" + sessionHash
REDACTED

	// 查询粘性会话绑定的账号 ID
	var sessionBoundAccountID int64
	if sessionKey != "" {
		sessionBoundAccountID, _ = h.gatewayService.GetCachedSessionAccountID(c.Request.Context(), apiKey.GroupID, sessionKey)
REDACTED
	// 判断是否真的绑定了粘性会话：有 sessionKey 且已经绑定到某个账号
	hasBoundSession := sessionKey != "" && sessionBoundAccountID > 0

	if platform == service.PlatformGemini {
		maxAccountSwitches := h.maxAccountSwitchesGemini
		switchCount := 0
		failedAccountIDs := make(map[int64]struct{REDACTED)
		var lastFailoverErr *service.UpstreamFailoverError
		var forceCacheBilling bool // 粘性会话切换时的缓存计费标记

		// 单账号分组提前设置 SingleAccountRetry 标记，让 Service 层首次 503 就不设模型限流标记。
		// 避免单账号分组收到 503 (MODEL_CAPACITY_EXHAUSTED) 时设 29s 限流，导致后续请求连续快速失败。
		if h.gatewayService.IsSingleAntigravityAccountGroup(c.Request.Context(), apiKey.GroupID) {
			ctx := context.WithValue(c.Request.Context(), ctxkey.SingleAccountRetry, true)
			c.Request = c.Request.WithContext(ctx)
	REDACTED

		for {
			selection, err := h.gatewayService.SelectAccountWithLoadAwareness(c.Request.Context(), apiKey.GroupID, sessionKey, reqModel, failedAccountIDs, "") // Gemini 不使用会话限制
			if err != nil {
				if len(failedAccountIDs) == 0 {
					log.Printf("[Gateway] SelectAccount failed: %v", err)
					h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "Service temporarily unavailable", streamStarted)
					return
			REDACTED
				// Antigravity 单账号退避重试：分组内没有其他可用账号时，
				// 对 503 错误不直接返回，而是清除排除列表、等待退避后重试同一个账号。
				// 谷歌上游 503 (MODEL_CAPACITY_EXHAUSTED) 通常是暂时性的，等几秒就能恢复。
				if lastFailoverErr != nil && lastFailoverErr.StatusCode == http.StatusServiceUnavailable && switchCount <= maxAccountSwitches {
					if sleepAntigravitySingleAccountBackoff(c.Request.Context(), switchCount) {
						log.Printf("Antigravity single-account 503 retry: clearing failed accounts, retry %d/%d", switchCount, maxAccountSwitches)
						failedAccountIDs = make(map[int64]struct{REDACTED)
						// 设置 context 标记，让 Service 层预检查等待限流过期而非直接切换
						ctx := context.WithValue(c.Request.Context(), ctxkey.SingleAccountRetry, true)
						c.Request = c.Request.WithContext(ctx)
						continue
				REDACTED
			REDACTED
				if lastFailoverErr != nil {
					h.handleFailoverExhausted(c, lastFailoverErr, service.PlatformGemini, streamStarted)
			REDACTED else {
					h.handleFailoverExhaustedSimple(c, 502, streamStarted)
			REDACTED
				return
		REDACTED
			account := selection.Account
			setOpsSelectedAccount(c, account.ID, account.Platform)

			// 检查请求拦截（预热请求、SUGGESTION MODE等）
			if account.IsInterceptWarmupEnabled() {
				interceptType := detectInterceptType(body, reqModel, parsedReq.MaxTokens, reqStream, isClaudeCodeClient)
				if interceptType != InterceptTypeNone {
					if selection.Acquired && selection.ReleaseFunc != nil {
						selection.ReleaseFunc()
				REDACTED
					if reqStream {
						sendMockInterceptStream(c, reqModel, interceptType)
				REDACTED else {
						sendMockInterceptResponse(c, reqModel, interceptType)
				REDACTED
					return
			REDACTED
		REDACTED

			// 3. 获取账号并发槽位
			accountReleaseFunc := selection.ReleaseFunc
			if !selection.Acquired {
				if selection.WaitPlan == nil {
					h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "No available accounts", streamStarted)
					return
			REDACTED
				accountWaitCounted := false
				canWait, err := h.concurrencyHelper.IncrementAccountWaitCount(c.Request.Context(), account.ID, selection.WaitPlan.MaxWaiting)
				if err != nil {
					log.Printf("Increment account wait count failed: %v", err)
			REDACTED else if !canWait {
					log.Printf("Account wait queue full: account=%d", account.ID)
					h.handleStreamingAwareError(c, http.StatusTooManyRequests, "rate_limit_error", "Too many pending requests, please retry later", streamStarted)
					return
			REDACTED
				if err == nil && canWait {
					accountWaitCounted = true
			REDACTED
				releaseWait := func() {
					if accountWaitCounted {
						h.concurrencyHelper.DecrementAccountWaitCount(c.Request.Context(), account.ID)
						accountWaitCounted = false
				REDACTED
			REDACTED

				accountReleaseFunc, err = h.concurrencyHelper.AcquireAccountSlotWithWaitTimeout(
					c,
					account.ID,
					selection.WaitPlan.MaxConcurrency,
					selection.WaitPlan.Timeout,
					reqStream,
					&streamStarted,
				)
				if err != nil {
					log.Printf("Account concurrency acquire failed: %v", err)
					releaseWait()
					h.handleConcurrencyError(c, err, "account", streamStarted)
					return
			REDACTED
				// Slot acquired: no longer waiting in queue.
				releaseWait()
				if err := h.gatewayService.BindStickySession(c.Request.Context(), apiKey.GroupID, sessionKey, account.ID); err != nil {
					log.Printf("Bind sticky session failed: %v", err)
			REDACTED
		REDACTED
			// 账号槽位/等待计数需要在超时或断开时安全回收
			accountReleaseFunc = wrapReleaseOnDone(c.Request.Context(), accountReleaseFunc)

			// 转发请求 - 根据账号平台分流
			var result *service.ForwardResult
			requestCtx := c.Request.Context()
			if switchCount > 0 {
				requestCtx = context.WithValue(requestCtx, ctxkey.AccountSwitchCount, switchCount)
		REDACTED
			if account.Platform == service.PlatformAntigravity {
				result, err = h.antigravityGatewayService.ForwardGemini(requestCtx, c, account, reqModel, "generateContent", reqStream, body, hasBoundSession)
		REDACTED else {
				result, err = h.geminiCompatService.Forward(requestCtx, c, account, body)
		REDACTED
			if accountReleaseFunc != nil {
				accountReleaseFunc()
		REDACTED
			if err != nil {
				var failoverErr *service.UpstreamFailoverError
				if errors.As(err, &failoverErr) {
					failedAccountIDs[account.ID] = struct{REDACTED{REDACTED
					lastFailoverErr = failoverErr
					if needForceCacheBilling(hasBoundSession, failoverErr) {
						forceCacheBilling = true
				REDACTED
					if switchCount >= maxAccountSwitches {
						h.handleFailoverExhausted(c, failoverErr, service.PlatformGemini, streamStarted)
						return
				REDACTED
					switchCount++
					log.Printf("Account %d: upstream error %d, switching account %d/%d", account.ID, failoverErr.StatusCode, switchCount, maxAccountSwitches)
					if account.Platform == service.PlatformAntigravity {
						if !sleepFailoverDelay(c.Request.Context(), switchCount) {
							return
					REDACTED
				REDACTED
					continue
			REDACTED
				// 错误响应已在Forward中处理，这里只记录日志
				log.Printf("Forward request failed: %v", err)
				return
		REDACTED

			// 捕获请求信息（用于异步记录，避免在 goroutine 中访问 gin.Context）
			userAgent := c.GetHeader("User-Agent")
			clientIP := ip.GetClientIP(c)

			// 异步记录使用量（subscription已在函数开头获取）
			go func(result *service.ForwardResult, usedAccount *service.Account, ua, clientIP string, fcb bool) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := h.gatewayService.RecordUsage(ctx, &service.RecordUsageInput{
					Result:            result,
					APIKey:            apiKey,
					User:              apiKey.User,
					Account:           usedAccount,
					Subscription:      subscription,
					UserAgent:         ua,
					IPAddress:         clientIP,
					ForceCacheBilling: fcb,
					APIKeyService:     h.apiKeyService,
			REDACTED); err != nil {
					log.Printf("Record usage failed: %v", err)
			REDACTED
		REDACTED(result, account, userAgent, clientIP, forceCacheBilling)
			return
	REDACTED
REDACTED

	currentAPIKey := apiKey
	currentSubscription := subscription
	var fallbackGroupID *int64
	if apiKey.Group != nil {
		fallbackGroupID = apiKey.Group.FallbackGroupIDOnInvalidRequest
REDACTED
	fallbackUsed := false

	// 单账号分组提前设置 SingleAccountRetry 标记，让 Service 层首次 503 就不设模型限流标记。
	// 避免单账号分组收到 503 (MODEL_CAPACITY_EXHAUSTED) 时设 29s 限流，导致后续请求连续快速失败。
	if h.gatewayService.IsSingleAntigravityAccountGroup(c.Request.Context(), currentAPIKey.GroupID) {
		ctx := context.WithValue(c.Request.Context(), ctxkey.SingleAccountRetry, true)
		c.Request = c.Request.WithContext(ctx)
REDACTED

	for {
		maxAccountSwitches := h.maxAccountSwitches
		switchCount := 0
		failedAccountIDs := make(map[int64]struct{REDACTED)
		var lastFailoverErr *service.UpstreamFailoverError
		retryWithFallback := false
		var forceCacheBilling bool // 粘性会话切换时的缓存计费标记

		for {
			// 选择支持该模型的账号
			selection, err := h.gatewayService.SelectAccountWithLoadAwareness(c.Request.Context(), currentAPIKey.GroupID, sessionKey, reqModel, failedAccountIDs, parsedReq.MetadataUserID)
			if err != nil {
				if len(failedAccountIDs) == 0 {
					log.Printf("[Gateway] SelectAccount failed: %v", err)
					h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "Service temporarily unavailable", streamStarted)
					return
			REDACTED
				// Antigravity 单账号退避重试：分组内没有其他可用账号时，
				// 对 503 错误不直接返回，而是清除排除列表、等待退避后重试同一个账号。
				// 谷歌上游 503 (MODEL_CAPACITY_EXHAUSTED) 通常是暂时性的，等几秒就能恢复。
				if lastFailoverErr != nil && lastFailoverErr.StatusCode == http.StatusServiceUnavailable && switchCount <= maxAccountSwitches {
					if sleepAntigravitySingleAccountBackoff(c.Request.Context(), switchCount) {
						log.Printf("Antigravity single-account 503 retry: clearing failed accounts, retry %d/%d", switchCount, maxAccountSwitches)
						failedAccountIDs = make(map[int64]struct{REDACTED)
						// 设置 context 标记，让 Service 层预检查等待限流过期而非直接切换
						ctx := context.WithValue(c.Request.Context(), ctxkey.SingleAccountRetry, true)
						c.Request = c.Request.WithContext(ctx)
						continue
				REDACTED
			REDACTED
				if lastFailoverErr != nil {
					h.handleFailoverExhausted(c, lastFailoverErr, platform, streamStarted)
			REDACTED else {
					h.handleFailoverExhaustedSimple(c, 502, streamStarted)
			REDACTED
				return
		REDACTED
			account := selection.Account
			setOpsSelectedAccount(c, account.ID, account.Platform)

			// 检查请求拦截（预热请求、SUGGESTION MODE等）
			if account.IsInterceptWarmupEnabled() {
				interceptType := detectInterceptType(body, reqModel, parsedReq.MaxTokens, reqStream, isClaudeCodeClient)
				if interceptType != InterceptTypeNone {
					if selection.Acquired && selection.ReleaseFunc != nil {
						selection.ReleaseFunc()
				REDACTED
					if reqStream {
						sendMockInterceptStream(c, reqModel, interceptType)
				REDACTED else {
						sendMockInterceptResponse(c, reqModel, interceptType)
				REDACTED
					return
			REDACTED
		REDACTED

			// 3. 获取账号并发槽位
			accountReleaseFunc := selection.ReleaseFunc
			if !selection.Acquired {
				if selection.WaitPlan == nil {
					h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "No available accounts", streamStarted)
					return
			REDACTED
				accountWaitCounted := false
				canWait, err := h.concurrencyHelper.IncrementAccountWaitCount(c.Request.Context(), account.ID, selection.WaitPlan.MaxWaiting)
				if err != nil {
					log.Printf("Increment account wait count failed: %v", err)
			REDACTED else if !canWait {
					log.Printf("Account wait queue full: account=%d", account.ID)
					h.handleStreamingAwareError(c, http.StatusTooManyRequests, "rate_limit_error", "Too many pending requests, please retry later", streamStarted)
					return
			REDACTED
				if err == nil && canWait {
					accountWaitCounted = true
			REDACTED
				releaseWait := func() {
					if accountWaitCounted {
						h.concurrencyHelper.DecrementAccountWaitCount(c.Request.Context(), account.ID)
						accountWaitCounted = false
				REDACTED
			REDACTED

				accountReleaseFunc, err = h.concurrencyHelper.AcquireAccountSlotWithWaitTimeout(
					c,
					account.ID,
					selection.WaitPlan.MaxConcurrency,
					selection.WaitPlan.Timeout,
					reqStream,
					&streamStarted,
				)
				if err != nil {
					log.Printf("Account concurrency acquire failed: %v", err)
					releaseWait()
					h.handleConcurrencyError(c, err, "account", streamStarted)
					return
			REDACTED
				// Slot acquired: no longer waiting in queue.
				releaseWait()
				if err := h.gatewayService.BindStickySession(c.Request.Context(), currentAPIKey.GroupID, sessionKey, account.ID); err != nil {
					log.Printf("Bind sticky session failed: %v", err)
			REDACTED
		REDACTED
			// 账号槽位/等待计数需要在超时或断开时安全回收
			accountReleaseFunc = wrapReleaseOnDone(c.Request.Context(), accountReleaseFunc)

			// 转发请求 - 根据账号平台分流
			var result *service.ForwardResult
			requestCtx := c.Request.Context()
			if switchCount > 0 {
				requestCtx = context.WithValue(requestCtx, ctxkey.AccountSwitchCount, switchCount)
		REDACTED
			if account.Platform == service.PlatformAntigravity && account.Type != service.AccountTypeAPIKey {
				result, err = h.antigravityGatewayService.Forward(requestCtx, c, account, body, hasBoundSession)
		REDACTED else {
				result, err = h.gatewayService.Forward(requestCtx, c, account, parsedReq)
		REDACTED
			if accountReleaseFunc != nil {
				accountReleaseFunc()
		REDACTED
			if err != nil {
				var promptTooLongErr *service.PromptTooLongError
				if errors.As(err, &promptTooLongErr) {
					log.Printf("Prompt too long from antigravity: group=%d fallback_group_id=%v fallback_used=%v", currentAPIKey.GroupID, fallbackGroupID, fallbackUsed)
					if !fallbackUsed && fallbackGroupID != nil && *fallbackGroupID > 0 {
						fallbackGroup, err := h.gatewayService.ResolveGroupByID(c.Request.Context(), *fallbackGroupID)
						if err != nil {
							log.Printf("Resolve fallback group failed: %v", err)
							_ = h.antigravityGatewayService.WriteMappedClaudeError(c, account, promptTooLongErr.StatusCode, promptTooLongErr.RequestID, promptTooLongErr.Body)
							return
					REDACTED
						if fallbackGroup.Platform != service.PlatformAnthropic ||
							fallbackGroup.SubscriptionType == service.SubscriptionTypeSubscription ||
							fallbackGroup.FallbackGroupIDOnInvalidRequest != nil {
							log.Printf("Fallback group invalid: group=%d platform=%s subscription=%s", fallbackGroup.ID, fallbackGroup.Platform, fallbackGroup.SubscriptionType)
							_ = h.antigravityGatewayService.WriteMappedClaudeError(c, account, promptTooLongErr.StatusCode, promptTooLongErr.RequestID, promptTooLongErr.Body)
							return
					REDACTED
						fallbackAPIKey := cloneAPIKeyWithGroup(apiKey, fallbackGroup)
						if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), fallbackAPIKey.User, fallbackAPIKey, fallbackGroup, nil); err != nil {
							status, code, message := billingErrorDetails(err)
							h.handleStreamingAwareError(c, status, code, message, streamStarted)
							return
					REDACTED
						// 兜底重试按“直接请求兜底分组”处理：清除强制平台，允许按分组平台调度
						ctx := context.WithValue(c.Request.Context(), ctxkey.ForcePlatform, "")
						c.Request = c.Request.WithContext(ctx)
						currentAPIKey = fallbackAPIKey
						currentSubscription = nil
						fallbackUsed = true
						retryWithFallback = true
						break
				REDACTED
					_ = h.antigravityGatewayService.WriteMappedClaudeError(c, account, promptTooLongErr.StatusCode, promptTooLongErr.RequestID, promptTooLongErr.Body)
					return
			REDACTED
				var failoverErr *service.UpstreamFailoverError
				if errors.As(err, &failoverErr) {
					failedAccountIDs[account.ID] = struct{REDACTED{REDACTED
					lastFailoverErr = failoverErr
					if needForceCacheBilling(hasBoundSession, failoverErr) {
						forceCacheBilling = true
				REDACTED
					if switchCount >= maxAccountSwitches {
						h.handleFailoverExhausted(c, failoverErr, account.Platform, streamStarted)
						return
				REDACTED
					switchCount++
					log.Printf("Account %d: upstream error %d, switching account %d/%d", account.ID, failoverErr.StatusCode, switchCount, maxAccountSwitches)
					if account.Platform == service.PlatformAntigravity {
						if !sleepFailoverDelay(c.Request.Context(), switchCount) {
							return
					REDACTED
				REDACTED
					continue
			REDACTED
				// 错误响应已在Forward中处理，这里只记录日志
				log.Printf("Account %d: Forward request failed: %v", account.ID, err)
				return
		REDACTED

			// 捕获请求信息（用于异步记录，避免在 goroutine 中访问 gin.Context）
			userAgent := c.GetHeader("User-Agent")
			clientIP := ip.GetClientIP(c)

			// 异步记录使用量（subscription已在函数开头获取）
			go func(result *service.ForwardResult, usedAccount *service.Account, ua, clientIP string, fcb bool) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := h.gatewayService.RecordUsage(ctx, &service.RecordUsageInput{
					Result:            result,
					APIKey:            currentAPIKey,
					User:              currentAPIKey.User,
					Account:           usedAccount,
					Subscription:      currentSubscription,
					UserAgent:         ua,
					IPAddress:         clientIP,
					ForceCacheBilling: fcb,
					APIKeyService:     h.apiKeyService,
			REDACTED); err != nil {
					log.Printf("Record usage failed: %v", err)
			REDACTED
		REDACTED(result, account, userAgent, clientIP, forceCacheBilling)
			return
	REDACTED
		if !retryWithFallback {
			return
	REDACTED
REDACTED
REDACTED

// Models handles listing available models
// GET /v1/models
// Returns models based on account configurations (model_mapping whitelist)
// Falls back to default models if no whitelist is configured
func (h *GatewayHandler) Models(c *gin.Context) {
	apiKey, _ := middleware2.GetAPIKeyFromContext(c)

	var groupID *int64
	var platform string

	if apiKey != nil && apiKey.Group != nil {
		groupID = &apiKey.Group.ID
		platform = apiKey.Group.Platform
REDACTED
	if forcedPlatform, ok := middleware2.GetForcePlatformFromContext(c); ok && strings.TrimSpace(forcedPlatform) != "" {
		platform = forcedPlatform
REDACTED

	if platform == service.PlatformSora {
		c.JSON(http.StatusOK, gin.H{
			"object": "list",
			"data":   service.DefaultSoraModels(h.cfg),
	REDACTED)
		return
REDACTED

	// Get available models from account configurations (without platform filter)
	availableModels := h.gatewayService.GetAvailableModels(c.Request.Context(), groupID, "")

	if len(availableModels) > 0 {
		// Build model list from whitelist
		models := make([]claude.Model, 0, len(availableModels))
		for _, modelID := range availableModels {
			models = append(models, claude.Model{
				ID:          modelID,
				Type:        "model",
				DisplayName: modelID,
				CreatedAt:   "2024-01-01T00:00:00Z",
		REDACTED)
	REDACTED
		c.JSON(http.StatusOK, gin.H{
			"object": "list",
			"data":   models,
	REDACTED)
		return
REDACTED

	// Fallback to default models
	if platform == "openai" {
		c.JSON(http.StatusOK, gin.H{
			"object": "list",
			"data":   openai.DefaultModels,
	REDACTED)
		return
REDACTED

	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   claude.DefaultModels,
REDACTED)
REDACTED

// AntigravityModels 返回 Antigravity 支持的全部模型
// GET /antigravity/models
func (h *GatewayHandler) AntigravityModels(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   antigravity.DefaultModels(),
REDACTED)
REDACTED

func cloneAPIKeyWithGroup(apiKey *service.APIKey, group *service.Group) *service.APIKey {
	if apiKey == nil || group == nil {
		return apiKey
REDACTED
	cloned := *apiKey
	groupID := group.ID
	cloned.GroupID = &groupID
	cloned.Group = group
	return &cloned
REDACTED

// Usage handles getting account balance and usage statistics for CC Switch integration
// GET /v1/usage
func (h *GatewayHandler) Usage(c *gin.Context) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
REDACTED

	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
REDACTED

	// Best-effort: 获取用量统计（按当前 API Key 过滤），失败不影响基础响应
	var usageData gin.H
	if h.usageService != nil {
		dashStats, err := h.usageService.GetAPIKeyDashboardStats(c.Request.Context(), apiKey.ID)
		if err == nil && dashStats != nil {
			usageData = gin.H{
				"today": gin.H{
					"requests":              dashStats.TodayRequests,
					"input_tokens":          dashStats.TodayInputTokens,
					"output_tokens":         dashStats.TodayOutputTokens,
					"cache_creation_tokens": dashStats.TodayCacheCreationTokens,
					"cache_read_tokens":     dashStats.TodayCacheReadTokens,
					"total_tokens":          dashStats.TodayTokens,
					"cost":                  dashStats.TodayCost,
					"actual_cost":           dashStats.TodayActualCost,
			REDACTED,
				"total": gin.H{
					"requests":              dashStats.TotalRequests,
					"input_tokens":          dashStats.TotalInputTokens,
					"output_tokens":         dashStats.TotalOutputTokens,
					"cache_creation_tokens": dashStats.TotalCacheCreationTokens,
					"cache_read_tokens":     dashStats.TotalCacheReadTokens,
					"total_tokens":          dashStats.TotalTokens,
					"cost":                  dashStats.TotalCost,
					"actual_cost":           dashStats.TotalActualCost,
			REDACTED,
				"average_duration_ms": dashStats.AverageDurationMs,
				"rpm":                 dashStats.Rpm,
				"tpm":                 dashStats.Tpm,
		REDACTED
	REDACTED
REDACTED

	// 订阅模式：返回订阅限额信息 + 用量统计
	if apiKey.Group != nil && apiKey.Group.IsSubscriptionType() {
		subscription, ok := middleware2.GetSubscriptionFromContext(c)
		if !ok {
			h.errorResponse(c, http.StatusForbidden, "subscription_error", "No active subscription")
			return
	REDACTED

		remaining := h.calculateSubscriptionRemaining(apiKey.Group, subscription)
		resp := gin.H{
			"isValid":   true,
			"planName":  apiKey.Group.Name,
			"remaining": remaining,
			"unit":      "USD",
			"subscription": gin.H{
				"daily_usage_usd":   subscription.DailyUsageUSD,
				"weekly_usage_usd":  subscription.WeeklyUsageUSD,
				"monthly_usage_usd": subscription.MonthlyUsageUSD,
				"daily_limit_usd":   apiKey.Group.DailyLimitUSD,
				"weekly_limit_usd":  apiKey.Group.WeeklyLimitUSD,
				"monthly_limit_usd": apiKey.Group.MonthlyLimitUSD,
				"expires_at":        subscription.ExpiresAt,
		REDACTED,
	REDACTED
		if usageData != nil {
			resp["usage"] = usageData
	REDACTED
		c.JSON(http.StatusOK, resp)
		return
REDACTED

	// 余额模式：返回钱包余额 + 用量统计
	latestUser, err := h.userService.GetByID(c.Request.Context(), subject.UserID)
	if err != nil {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "Failed to get user info")
		return
REDACTED

	resp := gin.H{
		"isValid":   true,
		"planName":  "钱包余额",
		"remaining": latestUser.Balance,
		"unit":      "USD",
		"balance":   latestUser.Balance,
REDACTED
	if usageData != nil {
		resp["usage"] = usageData
REDACTED
	c.JSON(http.StatusOK, resp)
REDACTED

// calculateSubscriptionRemaining 计算订阅剩余可用额度
// 逻辑：
// 1. 如果日/周/月任一限额达到100%，返回0
// 2. 否则返回所有已配置周期中剩余额度的最小值
func (h *GatewayHandler) calculateSubscriptionRemaining(group *service.Group, sub *service.UserSubscription) float64 {
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

// needForceCacheBilling 判断 failover 时是否需要强制缓存计费
// 粘性会话切换账号、或上游明确标记时，将 input_tokens 转为 cache_read 计费
func needForceCacheBilling(hasBoundSession bool, failoverErr *service.UpstreamFailoverError) bool {
	return hasBoundSession || (failoverErr != nil && failoverErr.ForceCacheBilling)
REDACTED

// sleepFailoverDelay 账号切换线性递增延时：第1次0s、第2次1s、第3次2s…
// 返回 false 表示 context 已取消。
func sleepFailoverDelay(ctx context.Context, switchCount int) bool {
	delay := time.Duration(switchCount-1) * time.Second
	if delay <= 0 {
		return true
REDACTED
	select {
	case <-ctx.Done():
		return false
	case <-time.After(delay):
		return true
REDACTED
REDACTED

// sleepAntigravitySingleAccountBackoff Antigravity 平台单账号分组的 503 退避重试延时。
// 当分组内只有一个可用账号且上游返回 503（MODEL_CAPACITY_EXHAUSTED）时使用，
// 采用短固定延时策略。Service 层在 SingleAccountRetry 模式下已经做了充分的原地重试
// （最多 3 次、总等待 30s），所以 Handler 层的退避只需短暂等待即可。
// 返回 false 表示 context 已取消。
func sleepAntigravitySingleAccountBackoff(ctx context.Context, retryCount int) bool {
	// 固定短延时：2s
	// Service 层已经在原地等待了足够长的时间（retryDelay × 重试次数），
	// Handler 层只需短暂间隔后重新进入 Service 层即可。
	const delay = 2 * time.Second

	log.Printf("Antigravity single-account 503 backoff: waiting %v before retry (attempt %d)", delay, retryCount)

	select {
	case <-ctx.Done():
		return false
	case <-time.After(delay):
		return true
REDACTED
REDACTED

func (h *GatewayHandler) handleFailoverExhausted(c *gin.Context, failoverErr *service.UpstreamFailoverError, platform string, streamStarted bool) {
	statusCode := failoverErr.StatusCode
	responseBody := failoverErr.ResponseBody

	// 先检查透传规则
	if h.errorPassthroughService != nil && len(responseBody) > 0 {
		if rule := h.errorPassthroughService.MatchRule(platform, statusCode, responseBody); rule != nil {
			// 确定响应状态码
			respCode := statusCode
			if !rule.PassthroughCode && rule.ResponseCode != nil {
				respCode = *rule.ResponseCode
		REDACTED

			// 确定响应消息
			msg := service.ExtractUpstreamErrorMessage(responseBody)
			if !rule.PassthroughBody && rule.CustomMessage != nil {
				msg = *rule.CustomMessage
		REDACTED

			h.handleStreamingAwareError(c, respCode, "upstream_error", msg, streamStarted)
			return
	REDACTED
REDACTED

	// 使用默认的错误映射
	status, errType, errMsg := h.mapUpstreamError(statusCode)
	h.handleStreamingAwareError(c, status, errType, errMsg, streamStarted)
REDACTED

// handleFailoverExhaustedSimple 简化版本，用于没有响应体的情况
func (h *GatewayHandler) handleFailoverExhaustedSimple(c *gin.Context, statusCode int, streamStarted bool) {
	status, errType, errMsg := h.mapUpstreamError(statusCode)
	h.handleStreamingAwareError(c, status, errType, errMsg, streamStarted)
REDACTED

func (h *GatewayHandler) mapUpstreamError(statusCode int) (int, string, string) {
	switch statusCode {
	case 401:
		return http.StatusBadGateway, "upstream_error", "Upstream authentication failed, please contact administrator"
	case 403:
		return http.StatusBadGateway, "upstream_error", "Upstream access forbidden, please contact administrator"
	case 429:
		return http.StatusTooManyRequests, "rate_limit_error", "Upstream rate limit exceeded, please retry later"
	case 529:
		return http.StatusServiceUnavailable, "overloaded_error", "Upstream service overloaded, please retry later"
	case 500, 502, 503, 504:
		return http.StatusBadGateway, "upstream_error", "Upstream service temporarily unavailable"
	default:
		return http.StatusBadGateway, "upstream_error", "Upstream request failed"
REDACTED
REDACTED

// handleStreamingAwareError handles errors that may occur after streaming has started
func (h *GatewayHandler) handleStreamingAwareError(c *gin.Context, status int, errType, message string, streamStarted bool) {
	if streamStarted {
		// Stream already started, send error as SSE event then close
		flusher, ok := c.Writer.(http.Flusher)
		if ok {
			// Send error event in SSE format with proper JSON marshaling
			errorData := map[string]any{
				"type": "error",
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
			errorEvent := fmt.Sprintf("data: %s\n\n", string(jsonBytes))
			if _, err := fmt.Fprint(c.Writer, errorEvent); err != nil {
				_ = c.Error(err)
		REDACTED
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
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
REDACTED

	_, ok = middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
REDACTED

	// 读取请求体
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

	// 检查是否为 Claude Code 客户端，设置到 context 中
	SetClaudeCodeClientContext(c, body)

	setOpsRequestContext(c, "", false, body)

	parsedReq, err := service.ParseGatewayRequest(body, domain.PlatformAnthropic)
	if err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
REDACTED
	// 在请求上下文中记录 thinking 状态，供 Antigravity 最终模型 key 推导/模型维度限流使用
	c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ctxkey.ThinkingEnabled, parsedReq.ThinkingEnabled))

	// 验证 model 必填
	if parsedReq.Model == "" {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
REDACTED

	setOpsRequestContext(c, parsedReq.Model, parsedReq.Stream, body)

	// 获取订阅信息（可能为nil）
	subscription, _ := middleware2.GetSubscriptionFromContext(c)

	// 校验 billing eligibility（订阅/余额）
	// 【注意】不计算并发，但需要校验订阅/余额
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription); err != nil {
		status, code, message := billingErrorDetails(err)
		h.errorResponse(c, status, code, message)
		return
REDACTED

	// 计算粘性会话 hash
	parsedReq.SessionContext = &service.SessionContext{
		ClientIP:  ip.GetClientIP(c),
		UserAgent: c.GetHeader("User-Agent"),
		APIKeyID:  apiKey.ID,
REDACTED
	sessionHash := h.gatewayService.GenerateSessionHash(parsedReq)

	// 选择支持该模型的账号
	account, err := h.gatewayService.SelectAccountForModel(c.Request.Context(), apiKey.GroupID, sessionHash, parsedReq.Model)
	if err != nil {
		log.Printf("[Gateway] SelectAccountForModel failed: %v", err)
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Service temporarily unavailable")
		return
REDACTED
	setOpsSelectedAccount(c, account.ID, account.Platform)

	// 转发请求（不记录使用量）
	if err := h.gatewayService.ForwardCountTokens(c.Request.Context(), c, account, parsedReq); err != nil {
		log.Printf("Forward count_tokens request failed: %v", err)
		// 错误响应已在 ForwardCountTokens 中处理
		return
REDACTED
REDACTED

// InterceptType 表示请求拦截类型
type InterceptType int

const (
	InterceptTypeNone              InterceptType = iota
	InterceptTypeWarmup                          // 预热请求（返回 "New Conversation"）
	InterceptTypeSuggestionMode                  // SUGGESTION MODE（返回空字符串）
	InterceptTypeMaxTokensOneHaiku               // max_tokens=1 + haiku 探测请求（返回 "#"）
)

// isHaikuModel 检查模型名称是否包含 "haiku"（大小写不敏感）
func isHaikuModel(model string) bool {
	return strings.Contains(strings.ToLower(model), "haiku")
REDACTED

// isMaxTokensOneHaikuRequest 检查是否为 max_tokens=1 + haiku 模型的探测请求
// 这类请求用于 Claude Code 验证 API 连通性
// 条件：max_tokens == 1 且 model 包含 "haiku" 且非流式请求
func isMaxTokensOneHaikuRequest(model string, maxTokens int, isStream bool) bool {
	return maxTokens == 1 && isHaikuModel(model) && !isStream
REDACTED

// detectInterceptType 检测请求是否需要拦截，返回拦截类型
// 参数说明：
//   - body: 请求体字节
//   - model: 请求的模型名称
//   - maxTokens: max_tokens 值
//   - isStream: 是否为流式请求
//   - isClaudeCodeClient: 是否已通过 Claude Code 客户端校验
func detectInterceptType(body []byte, model string, maxTokens int, isStream bool, isClaudeCodeClient bool) InterceptType {
	// 优先检查 max_tokens=1 + haiku 探测请求（仅非流式）
	if isClaudeCodeClient && isMaxTokensOneHaikuRequest(model, maxTokens, isStream) {
		return InterceptTypeMaxTokensOneHaiku
REDACTED

	// 快速检查：如果不包含任何关键字，直接返回
	bodyStr := string(body)
	hasSuggestionMode := strings.Contains(bodyStr, "[SUGGESTION MODE:")
	hasWarmupKeyword := strings.Contains(bodyStr, "title") || strings.Contains(bodyStr, "Warmup")

	if !hasSuggestionMode && !hasWarmupKeyword {
		return InterceptTypeNone
REDACTED

	// 解析请求（只解析一次）
	var req struct {
		Messages []struct {
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
		REDACTED `json:"content"`
	REDACTED `json:"messages"`
		System []struct {
			Text string `json:"text"`
	REDACTED `json:"system"`
REDACTED
	if err := json.Unmarshal(body, &req); err != nil {
		return InterceptTypeNone
REDACTED

	// 检查 SUGGESTION MODE（最后一条 user 消息）
	if hasSuggestionMode && len(req.Messages) > 0 {
		lastMsg := req.Messages[len(req.Messages)-1]
		if lastMsg.Role == "user" && len(lastMsg.Content) > 0 &&
			lastMsg.Content[0].Type == "text" &&
			strings.HasPrefix(lastMsg.Content[0].Text, "[SUGGESTION MODE:") {
			return InterceptTypeSuggestionMode
	REDACTED
REDACTED

	// 检查 Warmup 请求
	if hasWarmupKeyword {
		// 检查 messages 中的标题提示模式
		for _, msg := range req.Messages {
			for _, content := range msg.Content {
				if content.Type == "text" {
					if strings.Contains(content.Text, "Please write a 5-10 word title for the following conversation:") ||
						content.Text == "Warmup" {
						return InterceptTypeWarmup
				REDACTED
			REDACTED
		REDACTED
	REDACTED
		// 检查 system 中的标题提取模式
		for _, sys := range req.System {
			if strings.Contains(sys.Text, "nalyze if this message indicates a new conversation topic. If it does, extract a 2-3 word title") {
				return InterceptTypeWarmup
		REDACTED
	REDACTED
REDACTED

	return InterceptTypeNone
REDACTED

// sendMockInterceptStream 发送流式 mock 响应（用于请求拦截）
func sendMockInterceptStream(c *gin.Context, model string, interceptType InterceptType) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	// 根据拦截类型决定响应内容
	var msgID string
	var outputTokens int
	var textDeltas []string

	switch interceptType {
	case InterceptTypeSuggestionMode:
		msgID = "msg_mock_suggestion"
		outputTokens = 1
		textDeltas = []string{""REDACTED // 空内容
	default: // InterceptTypeWarmup
		msgID = "msg_mock_warmup"
		outputTokens = 2
		textDeltas = []string{"New", " Conversation"REDACTED
REDACTED

	// Build message_start event with proper JSON marshaling
	messageStart := map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":            msgID,
			"type":          "message",
			"role":          "assistant",
			"model":         model,
			"content":       []any{REDACTED,
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": map[string]int{
				"input_tokens":  10,
				"output_tokens": 0,
		REDACTED,
	REDACTED,
REDACTED
	messageStartJSON, _ := json.Marshal(messageStart)

	// Build events
	events := []string{
		`event: message_start` + "\n" + `data: ` + string(messageStartJSON),
		`event: content_block_start` + "\n" + `data: {"content_block":{"text":"","type":"text"REDACTED,"index":0,"type":"content_block_start"REDACTED`,
REDACTED

	// Add text deltas
	for _, text := range textDeltas {
		delta := map[string]any{
			"type":  "content_block_delta",
			"index": 0,
			"delta": map[string]string{
				"type": "text_delta",
				"text": text,
		REDACTED,
	REDACTED
		deltaJSON, _ := json.Marshal(delta)
		events = append(events, `event: content_block_delta`+"\n"+`data: `+string(deltaJSON))
REDACTED

	// Add final events
	messageDelta := map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason":   "end_turn",
			"stop_sequence": nil,
	REDACTED,
		"usage": map[string]int{
			"input_tokens":  10,
			"output_tokens": outputTokens,
	REDACTED,
REDACTED
	messageDeltaJSON, _ := json.Marshal(messageDelta)

	events = append(events,
		`event: content_block_stop`+"\n"+`data: {"index":0,"type":"content_block_stop"REDACTED`,
		`event: message_delta`+"\n"+`data: `+string(messageDeltaJSON),
		`event: message_stop`+"\n"+`data: {"type":"message_stop"REDACTED`,
	)

	for _, event := range events {
		_, _ = c.Writer.WriteString(event + "\n\n")
		c.Writer.Flush()
		time.Sleep(20 * time.Millisecond)
REDACTED
REDACTED

// generateRealisticMsgID 生成仿真的消息 ID（msg_bdrk_XXXXXXX 格式）
// 格式与 Claude API 真实响应一致，24 位随机字母数字
func generateRealisticMsgID() string {
	const charset = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	const idLen = 24
	randomBytes := make([]byte, idLen)
	if _, err := rand.Read(randomBytes); err != nil {
		return fmt.Sprintf("msg_bdrk_%d", time.Now().UnixNano())
REDACTED
	b := make([]byte, idLen)
	for i := range b {
		b[i] = charset[int(randomBytes[i])%len(charset)]
REDACTED
	return "msg_bdrk_" + string(b)
REDACTED

// sendMockInterceptResponse 发送非流式 mock 响应（用于请求拦截）
func sendMockInterceptResponse(c *gin.Context, model string, interceptType InterceptType) {
	var msgID, text, stopReason string
	var outputTokens int

	switch interceptType {
	case InterceptTypeSuggestionMode:
		msgID = "msg_mock_suggestion"
		text = ""
		outputTokens = 1
		stopReason = "end_turn"
	case InterceptTypeMaxTokensOneHaiku:
		msgID = generateRealisticMsgID()
		text = "#"
		outputTokens = 1
		stopReason = "max_tokens" // max_tokens=1 探测请求的 stop_reason 应为 max_tokens
	default: // InterceptTypeWarmup
		msgID = "msg_mock_warmup"
		text = "New Conversation"
		outputTokens = 2
		stopReason = "end_turn"
REDACTED

	// 构建完整的响应格式（与 Claude API 响应格式一致）
	response := gin.H{
		"model":         model,
		"id":            msgID,
		"type":          "message",
		"role":          "assistant",
		"content":       []gin.H{{"type": "text", "text": textREDACTEDREDACTED,
		"stop_reason":   stopReason,
		"stop_sequence": nil,
		"usage": gin.H{
			"input_tokens":                10,
			"cache_creation_input_tokens": 0,
			"cache_read_input_tokens":     0,
			"cache_creation": gin.H{
				"ephemeral_5m_input_tokens": 0,
				"ephemeral_1h_input_tokens": 0,
		REDACTED,
			"output_tokens": outputTokens,
			"total_tokens":  10 + outputTokens,
	REDACTED,
REDACTED

	c.JSON(http.StatusOK, response)
REDACTED

func billingErrorDetails(err error) (status int, code, message string) {
	if errors.Is(err, service.ErrBillingServiceUnavailable) {
		msg := pkgerrors.Message(err)
		if msg == "" {
			msg = "Billing service temporarily unavailable. Please retry later."
	REDACTED
		return http.StatusServiceUnavailable, "billing_service_error", msg
REDACTED
	msg := pkgerrors.Message(err)
	if msg == "" {
		log.Printf("[Gateway] billing error details: %v", err)
		msg = "Billing error"
REDACTED
	return http.StatusForbidden, "billing_error", msg
REDACTED
