package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/gemini"
	"github.com/Wei-Shaw/sub2api/internal/pkg/googleapi"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// geminiCLITmpDirRegex 用于从 Gemini CLI 请求体中提取 tmp 目录的哈希值
// 匹配格式: /Users/xxx/.gemini/tmp/[64位十六进制哈希]
var geminiCLITmpDirRegex = regexp.MustCompile(`/\.gemini/tmp/([A-Fa-f0-9]{64REDACTED)`)

func isGeminiCLIRequest(c *gin.Context, body []byte) bool {
	if strings.TrimSpace(c.GetHeader("x-gemini-api-privileged-user-id")) != "" {
		return true
REDACTED
	return geminiCLITmpDirRegex.Match(body)
REDACTED

// GeminiV1BetaListModels proxies:
// GET /v1beta/models
func (h *GatewayHandler) GeminiV1BetaListModels(c *gin.Context) {
	apiKey, ok := middleware.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil {
		googleError(c, http.StatusUnauthorized, "Invalid API key")
		return
REDACTED
	// 检查平台：优先使用强制平台（/antigravity 路由），否则要求 gemini 分组
	forcePlatform, hasForcePlatform := middleware.GetForcePlatformFromContext(c)
	if !hasForcePlatform && (apiKey.Group == nil || apiKey.Group.Platform != service.PlatformGemini) {
		googleError(c, http.StatusBadRequest, "API key group platform is not gemini")
		return
REDACTED

	// 强制 antigravity 模式：返回 antigravity 支持的模型列表
	if forcePlatform == service.PlatformAntigravity {
		c.JSON(http.StatusOK, antigravity.FallbackGeminiModelsList())
		return
REDACTED

	account, err := h.geminiCompatService.SelectAccountForAIStudioEndpoints(c.Request.Context(), apiKey.GroupID)
	if err != nil {
		// 没有 gemini 账户，检查是否有 antigravity 账户可用
		hasAntigravity, _ := h.geminiCompatService.HasAntigravityAccounts(c.Request.Context(), apiKey.GroupID)
		if hasAntigravity {
			// antigravity 账户使用静态模型列表
			c.JSON(http.StatusOK, gemini.FallbackModelsList())
			return
	REDACTED
		googleError(c, http.StatusServiceUnavailable, "No available Gemini accounts: "+err.Error())
		return
REDACTED

	res, err := h.geminiCompatService.ForwardAIStudioGET(c.Request.Context(), account, "/v1beta/models")
	if err != nil {
		googleError(c, http.StatusBadGateway, err.Error())
		return
REDACTED
	if shouldFallbackGeminiModels(res) {
		c.JSON(http.StatusOK, gemini.FallbackModelsList())
		return
REDACTED
	writeUpstreamResponse(c, res)
REDACTED

// GeminiV1BetaGetModel proxies:
// GET /v1beta/models/{modelREDACTED
func (h *GatewayHandler) GeminiV1BetaGetModel(c *gin.Context) {
	apiKey, ok := middleware.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil {
		googleError(c, http.StatusUnauthorized, "Invalid API key")
		return
REDACTED
	// 检查平台：优先使用强制平台（/antigravity 路由），否则要求 gemini 分组
	forcePlatform, hasForcePlatform := middleware.GetForcePlatformFromContext(c)
	if !hasForcePlatform && (apiKey.Group == nil || apiKey.Group.Platform != service.PlatformGemini) {
		googleError(c, http.StatusBadRequest, "API key group platform is not gemini")
		return
REDACTED

	modelName := strings.TrimSpace(c.Param("model"))
	if modelName == "" {
		googleError(c, http.StatusBadRequest, "Missing model in URL")
		return
REDACTED

	// 强制 antigravity 模式：返回 antigravity 模型信息
	if forcePlatform == service.PlatformAntigravity {
		c.JSON(http.StatusOK, antigravity.FallbackGeminiModel(modelName))
		return
REDACTED

	account, err := h.geminiCompatService.SelectAccountForAIStudioEndpoints(c.Request.Context(), apiKey.GroupID)
	if err != nil {
		// 没有 gemini 账户，检查是否有 antigravity 账户可用
		hasAntigravity, _ := h.geminiCompatService.HasAntigravityAccounts(c.Request.Context(), apiKey.GroupID)
		if hasAntigravity {
			// antigravity 账户使用静态模型信息
			c.JSON(http.StatusOK, gemini.FallbackModel(modelName))
			return
	REDACTED
		googleError(c, http.StatusServiceUnavailable, "No available Gemini accounts: "+err.Error())
		return
REDACTED

	res, err := h.geminiCompatService.ForwardAIStudioGET(c.Request.Context(), account, "/v1beta/models/"+modelName)
	if err != nil {
		googleError(c, http.StatusBadGateway, err.Error())
		return
REDACTED
	if shouldFallbackGeminiModels(res) {
		c.JSON(http.StatusOK, gemini.FallbackModel(modelName))
		return
REDACTED
	writeUpstreamResponse(c, res)
REDACTED

// GeminiV1BetaModels proxies Gemini native REST endpoints like:
// POST /v1beta/models/{modelREDACTED:generateContent
// POST /v1beta/models/{modelREDACTED:streamGenerateContent?alt=sse
func (h *GatewayHandler) GeminiV1BetaModels(c *gin.Context) {
	apiKey, ok := middleware.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil {
		googleError(c, http.StatusUnauthorized, "Invalid API key")
		return
REDACTED
	authSubject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		googleError(c, http.StatusInternalServerError, "User context not found")
		return
REDACTED

	// 检查平台：优先使用强制平台（/antigravity 路由，中间件已设置 request.Context），否则要求 gemini 分组
	if !middleware.HasForcePlatform(c) {
		if apiKey.Group == nil || apiKey.Group.Platform != service.PlatformGemini {
			googleError(c, http.StatusBadRequest, "API key group platform is not gemini")
			return
	REDACTED
REDACTED

	modelName, action, err := parseGeminiModelAction(strings.TrimPrefix(c.Param("modelAction"), "/"))
	if err != nil {
		googleError(c, http.StatusNotFound, err.Error())
		return
REDACTED

	stream := action == "streamGenerateContent"

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			googleError(c, http.StatusRequestEntityTooLarge, buildBodyTooLargeMessage(maxErr.Limit))
			return
	REDACTED
		googleError(c, http.StatusBadRequest, "Failed to read request body")
		return
REDACTED
	if len(body) == 0 {
		googleError(c, http.StatusBadRequest, "Request body is empty")
		return
REDACTED

	setOpsRequestContext(c, modelName, stream, body)

	// Get subscription (may be nil)
	subscription, _ := middleware.GetSubscriptionFromContext(c)

	// For Gemini native API, do not send Claude-style ping frames.
	geminiConcurrency := NewConcurrencyHelper(h.concurrencyHelper.concurrencyService, SSEPingFormatNone, 0)

	// 0) wait queue check
	maxWait := service.CalculateMaxWait(authSubject.Concurrency)
	canWait, err := geminiConcurrency.IncrementWaitCount(c.Request.Context(), authSubject.UserID, maxWait)
	waitCounted := false
	if err != nil {
		log.Printf("Increment wait count failed: %v", err)
REDACTED else if !canWait {
		googleError(c, http.StatusTooManyRequests, "Too many pending requests, please retry later")
		return
REDACTED
	if err == nil && canWait {
		waitCounted = true
REDACTED
	defer func() {
		if waitCounted {
			geminiConcurrency.DecrementWaitCount(c.Request.Context(), authSubject.UserID)
	REDACTED
REDACTED()

	// 1) user concurrency slot
	streamStarted := false
	userReleaseFunc, err := geminiConcurrency.AcquireUserSlotWithWait(c, authSubject.UserID, authSubject.Concurrency, stream, &streamStarted)
	if err != nil {
		googleError(c, http.StatusTooManyRequests, err.Error())
		return
REDACTED
	if waitCounted {
		geminiConcurrency.DecrementWaitCount(c.Request.Context(), authSubject.UserID)
		waitCounted = false
REDACTED
	// 确保请求取消时也会释放槽位，避免长连接被动中断造成泄漏
	userReleaseFunc = wrapReleaseOnDone(c.Request.Context(), userReleaseFunc)
	if userReleaseFunc != nil {
		defer userReleaseFunc()
REDACTED

	// 2) billing eligibility check (after wait)
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription); err != nil {
		status, _, message := billingErrorDetails(err)
		googleError(c, status, message)
		return
REDACTED

	// 3) select account (sticky session based on request body)
	// 优先使用 Gemini CLI 的会话标识（privileged-user-id + tmp 目录哈希）
	sessionHash := extractGeminiCLISessionHash(c, body)
	if sessionHash == "" {
		// Fallback: 使用通用的会话哈希生成逻辑（适用于其他客户端）
		parsedReq, _ := service.ParseGatewayRequest(body)
		sessionHash = h.gatewayService.GenerateSessionHash(parsedReq)
REDACTED
	sessionKey := sessionHash
	if sessionHash != "" {
		sessionKey = "gemini:" + sessionHash
REDACTED

	// 查询粘性会话绑定的账号 ID（用于检测账号切换）
	var sessionBoundAccountID int64
	if sessionKey != "" {
		sessionBoundAccountID, _ = h.gatewayService.GetCachedSessionAccountID(c.Request.Context(), apiKey.GroupID, sessionKey)
REDACTED
	isCLI := isGeminiCLIRequest(c, body)
	cleanedForUnknownBinding := false

	maxAccountSwitches := h.maxAccountSwitchesGemini
	switchCount := 0
	failedAccountIDs := make(map[int64]struct{REDACTED)
	lastFailoverStatus := 0

	for {
		selection, err := h.gatewayService.SelectAccountWithLoadAwareness(c.Request.Context(), apiKey.GroupID, sessionKey, modelName, failedAccountIDs, "") // Gemini 不使用会话限制
		if err != nil {
			if len(failedAccountIDs) == 0 {
				googleError(c, http.StatusServiceUnavailable, "No available Gemini accounts: "+err.Error())
				return
		REDACTED
			handleGeminiFailoverExhausted(c, lastFailoverStatus)
			return
	REDACTED
		account := selection.Account
		setOpsSelectedAccount(c, account.ID)

		// 检测账号切换：如果粘性会话绑定的账号与当前选择的账号不同，清除 thoughtSignature
		// 注意：Gemini 原生 API 的 thoughtSignature 与具体上游账号强相关；跨账号透传会导致 400。
		if sessionBoundAccountID > 0 && sessionBoundAccountID != account.ID {
			log.Printf("[Gemini] Sticky session account switched: %d -> %d, cleaning thoughtSignature", sessionBoundAccountID, account.ID)
			body = service.CleanGeminiNativeThoughtSignatures(body)
			sessionBoundAccountID = account.ID
	REDACTED else if sessionKey != "" && sessionBoundAccountID == 0 && isCLI && !cleanedForUnknownBinding && bytes.Contains(body, []byte(`"thoughtSignature"`)) {
			// 无缓存绑定但请求里已有 thoughtSignature：常见于缓存丢失/TTL 过期后，CLI 继续携带旧签名。
			// 为避免第一次转发就 400，这里做一次确定性清理，让新账号重新生成签名链路。
			log.Printf("[Gemini] Sticky session binding missing for CLI request, cleaning thoughtSignature proactively")
			body = service.CleanGeminiNativeThoughtSignatures(body)
			cleanedForUnknownBinding = true
			sessionBoundAccountID = account.ID
	REDACTED else if sessionBoundAccountID == 0 {
			// 记录本次请求中首次选择到的账号，便于同一请求内 failover 时检测切换。
			sessionBoundAccountID = account.ID
	REDACTED

		// 4) account concurrency slot
		accountReleaseFunc := selection.ReleaseFunc
		if !selection.Acquired {
			if selection.WaitPlan == nil {
				googleError(c, http.StatusServiceUnavailable, "No available Gemini accounts")
				return
		REDACTED
			accountWaitCounted := false
			canWait, err := geminiConcurrency.IncrementAccountWaitCount(c.Request.Context(), account.ID, selection.WaitPlan.MaxWaiting)
			if err != nil {
				log.Printf("Increment account wait count failed: %v", err)
		REDACTED else if !canWait {
				log.Printf("Account wait queue full: account=%d", account.ID)
				googleError(c, http.StatusTooManyRequests, "Too many pending requests, please retry later")
				return
		REDACTED
			if err == nil && canWait {
				accountWaitCounted = true
		REDACTED
			defer func() {
				if accountWaitCounted {
					geminiConcurrency.DecrementAccountWaitCount(c.Request.Context(), account.ID)
			REDACTED
		REDACTED()

			accountReleaseFunc, err = geminiConcurrency.AcquireAccountSlotWithWaitTimeout(
				c,
				account.ID,
				selection.WaitPlan.MaxConcurrency,
				selection.WaitPlan.Timeout,
				stream,
				&streamStarted,
			)
			if err != nil {
				googleError(c, http.StatusTooManyRequests, err.Error())
				return
		REDACTED
			if accountWaitCounted {
				geminiConcurrency.DecrementAccountWaitCount(c.Request.Context(), account.ID)
				accountWaitCounted = false
		REDACTED
			if err := h.gatewayService.BindStickySession(c.Request.Context(), apiKey.GroupID, sessionKey, account.ID); err != nil {
				log.Printf("Bind sticky session failed: %v", err)
		REDACTED
	REDACTED
		// 账号槽位/等待计数需要在超时或断开时安全回收
		accountReleaseFunc = wrapReleaseOnDone(c.Request.Context(), accountReleaseFunc)

		// 5) forward (根据平台分流)
		var result *service.ForwardResult
		requestCtx := c.Request.Context()
		if switchCount > 0 {
			requestCtx = context.WithValue(requestCtx, ctxkey.AccountSwitchCount, switchCount)
	REDACTED
		if account.Platform == service.PlatformAntigravity {
			result, err = h.antigravityGatewayService.ForwardGemini(requestCtx, c, account, modelName, action, stream, body)
	REDACTED else {
			result, err = h.geminiCompatService.ForwardNative(requestCtx, c, account, modelName, action, stream, body)
	REDACTED
		if accountReleaseFunc != nil {
			accountReleaseFunc()
	REDACTED
		if err != nil {
			var failoverErr *service.UpstreamFailoverError
			if errors.As(err, &failoverErr) {
				failedAccountIDs[account.ID] = struct{REDACTED{REDACTED
				if switchCount >= maxAccountSwitches {
					lastFailoverStatus = failoverErr.StatusCode
					handleGeminiFailoverExhausted(c, lastFailoverStatus)
					return
			REDACTED
				lastFailoverStatus = failoverErr.StatusCode
				switchCount++
				log.Printf("Gemini account %d: upstream error %d, switching account %d/%d", account.ID, failoverErr.StatusCode, switchCount, maxAccountSwitches)
				continue
		REDACTED
			// ForwardNative already wrote the response
			log.Printf("Gemini native forward failed: %v", err)
			return
	REDACTED

		// 捕获请求信息（用于异步记录，避免在 goroutine 中访问 gin.Context）
		userAgent := c.GetHeader("User-Agent")
		clientIP := ip.GetClientIP(c)

		// 6) record usage async
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
				log.Printf("Record usage failed: %v", err)
		REDACTED
	REDACTED(result, account, userAgent, clientIP)
		return
REDACTED
REDACTED

func parseGeminiModelAction(rest string) (model string, action string, err error) {
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return "", "", &pathParseError{"missing path"REDACTED
REDACTED

	// Standard: {modelREDACTED:{actionREDACTED
	if i := strings.Index(rest, ":"); i > 0 && i < len(rest)-1 {
		return rest[:i], rest[i+1:], nil
REDACTED

	// Fallback: {modelREDACTED/{actionREDACTED
	if i := strings.Index(rest, "/"); i > 0 && i < len(rest)-1 {
		return rest[:i], rest[i+1:], nil
REDACTED

	return "", "", &pathParseError{"invalid model action path"REDACTED
REDACTED

func handleGeminiFailoverExhausted(c *gin.Context, statusCode int) {
	status, message := mapGeminiUpstreamError(statusCode)
	googleError(c, status, message)
REDACTED

func mapGeminiUpstreamError(statusCode int) (int, string) {
	switch statusCode {
	case 401:
		return http.StatusBadGateway, "Upstream authentication failed, please contact administrator"
	case 403:
		return http.StatusBadGateway, "Upstream access forbidden, please contact administrator"
	case 429:
		return http.StatusTooManyRequests, "Upstream rate limit exceeded, please retry later"
	case 529:
		return http.StatusServiceUnavailable, "Upstream service overloaded, please retry later"
	case 500, 502, 503, 504:
		return http.StatusBadGateway, "Upstream service temporarily unavailable"
	default:
		return http.StatusBadGateway, "Upstream request failed"
REDACTED
REDACTED

type pathParseError struct{ msg string REDACTED

func (e *pathParseError) Error() string { return e.msg REDACTED

func googleError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"code":    status,
			"message": message,
			"status":  googleapi.HTTPStatusToGoogleStatus(status),
	REDACTED,
REDACTED)
REDACTED

func writeUpstreamResponse(c *gin.Context, res *service.UpstreamHTTPResult) {
	if res == nil {
		googleError(c, http.StatusBadGateway, "Empty upstream response")
		return
REDACTED
	for k, vv := range res.Headers {
		// Avoid overriding content-length and hop-by-hop headers.
		if strings.EqualFold(k, "Content-Length") || strings.EqualFold(k, "Transfer-Encoding") || strings.EqualFold(k, "Connection") {
			continue
	REDACTED
		for _, v := range vv {
			c.Writer.Header().Add(k, v)
	REDACTED
REDACTED
	contentType := res.Headers.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
REDACTED
	c.Data(res.StatusCode, contentType, res.Body)
REDACTED

func shouldFallbackGeminiModels(res *service.UpstreamHTTPResult) bool {
	if res == nil {
		return true
REDACTED
	if res.StatusCode != http.StatusUnauthorized && res.StatusCode != http.StatusForbidden {
		return false
REDACTED
	if strings.Contains(strings.ToLower(res.Headers.Get("Www-Authenticate")), "insufficient_scope") {
		return true
REDACTED
	if strings.Contains(strings.ToLower(string(res.Body)), "insufficient authentication scopes") {
		return true
REDACTED
	if strings.Contains(strings.ToLower(string(res.Body)), "access_token_scope_insufficient") {
		return true
REDACTED
	return false
REDACTED

// extractGeminiCLISessionHash 从 Gemini CLI 请求中提取会话标识。
// 组合 x-gemini-api-privileged-user-id header 和请求体中的 tmp 目录哈希。
//
// 会话标识生成策略：
//  1. 从请求体中提取 tmp 目录哈希（64位十六进制）
//  2. 从 header 中提取 privileged-user-id（UUID）
//  3. 组合两者生成 SHA256 哈希作为最终的会话标识
//
// 如果找不到 tmp 目录哈希，返回空字符串（不使用粘性会话）。
//
// extractGeminiCLISessionHash extracts session identifier from Gemini CLI requests.
// Combines x-gemini-api-privileged-user-id header with tmp directory hash from request body.
func extractGeminiCLISessionHash(c *gin.Context, body []byte) string {
	// 1. 从请求体中提取 tmp 目录哈希
	match := geminiCLITmpDirRegex.FindSubmatch(body)
	if len(match) < 2 {
		return "" // 没有找到 tmp 目录，不使用粘性会话
REDACTED
	tmpDirHash := string(match[1])

	// 2. 提取 privileged-user-id
	privilegedUserID := strings.TrimSpace(c.GetHeader("x-gemini-api-privileged-user-id"))

	// 3. 组合生成最终的 session hash
	if privilegedUserID != "" {
		// 组合两个标识符：privileged-user-id + tmp 目录哈希
		combined := privilegedUserID + ":" + tmpDirHash
		hash := sha256.Sum256([]byte(combined))
		return hex.EncodeToString(hash[:])
REDACTED

	// 如果没有 privileged-user-id，直接使用 tmp 目录哈希
	return tmpDirHash
REDACTED
