package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/gemini"
	"github.com/Wei-Shaw/sub2api/internal/pkg/googleapi"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"

	"github.com/gin-gonic/gin"
)

// geminiCLITmpDirRegex 用于从 Gemini CLI 请求体中提取 tmp 目录的哈希值
// 匹配格式: /Users/xxx/.gemini/tmp/[64位十六进制哈希]
var geminiCLITmpDirRegex = regexp.MustCompile(`/\.gemini/tmp/([A-Fa-f0-9]{64REDACTED)`)

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
	if h.errorPassthroughService != nil {
		service.BindErrorPassthroughService(c, h.errorPassthroughService)
REDACTED
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
		parsedReq, _ := service.ParseGatewayRequest(body, domain.PlatformGemini)
		if parsedReq != nil {
			parsedReq.SessionContext = &service.SessionContext{
				ClientIP:  ip.GetClientIP(c),
				UserAgent: c.GetHeader("User-Agent"),
				APIKeyID:  apiKey.ID,
		REDACTED
	REDACTED
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

	// === Gemini 内容摘要会话 Fallback 逻辑 ===
	// 当原有会话标识无效时（sessionBoundAccountID == 0），尝试基于内容摘要链匹配
	var geminiDigestChain string
	var geminiPrefixHash string
	var geminiSessionUUID string
	var matchedDigestChain string
	useDigestFallback := sessionBoundAccountID == 0

	if useDigestFallback {
		// 解析 Gemini 请求体
		var geminiReq antigravity.GeminiRequest
		if err := json.Unmarshal(body, &geminiReq); err == nil && len(geminiReq.Contents) > 0 {
			// 生成摘要链
			geminiDigestChain = service.BuildGeminiDigestChain(&geminiReq)
			if geminiDigestChain != "" {
				// 生成前缀 hash
				userAgent := c.GetHeader("User-Agent")
				clientIP := ip.GetClientIP(c)
				platform := ""
				if apiKey.Group != nil {
					platform = apiKey.Group.Platform
			REDACTED
				geminiPrefixHash = service.GenerateGeminiPrefixHash(
					authSubject.UserID,
					apiKey.ID,
					clientIP,
					userAgent,
					platform,
					modelName,
				)

				// 查找会话
				foundUUID, foundAccountID, foundMatchedChain, found := h.gatewayService.FindGeminiSession(
					c.Request.Context(),
					derefGroupID(apiKey.GroupID),
					geminiPrefixHash,
					geminiDigestChain,
				)
				if found {
					matchedDigestChain = foundMatchedChain
					sessionBoundAccountID = foundAccountID
					geminiSessionUUID = foundUUID
					log.Printf("[Gemini] Digest fallback matched: uuid=%s, accountID=%d, chain=%s",
						safeShortPrefix(foundUUID, 8), foundAccountID, truncateDigestChain(geminiDigestChain))

					// 关键：如果原 sessionKey 为空，使用 prefixHash + uuid 作为 sessionKey
					// 这样 SelectAccountWithLoadAwareness 的粘性会话逻辑会优先使用匹配到的账号
					if sessionKey == "" {
						sessionKey = service.GenerateGeminiDigestSessionKey(geminiPrefixHash, foundUUID)
				REDACTED
					_ = h.gatewayService.BindStickySession(c.Request.Context(), apiKey.GroupID, sessionKey, foundAccountID)
			REDACTED else {
					// 生成新的会话 UUID
					geminiSessionUUID = uuid.New().String()
					// 为新会话也生成 sessionKey（用于后续请求的粘性会话）
					if sessionKey == "" {
						sessionKey = service.GenerateGeminiDigestSessionKey(geminiPrefixHash, geminiSessionUUID)
				REDACTED
			REDACTED
		REDACTED
	REDACTED
REDACTED

	// 判断是否真的绑定了粘性会话：有 sessionKey 且已经绑定到某个账号
	hasBoundSession := sessionKey != "" && sessionBoundAccountID > 0
	cleanedForUnknownBinding := false

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
		selection, err := h.gatewayService.SelectAccountWithLoadAwareness(c.Request.Context(), apiKey.GroupID, sessionKey, modelName, failedAccountIDs, "") // Gemini 不使用会话限制
		if err != nil {
			if len(failedAccountIDs) == 0 {
				googleError(c, http.StatusServiceUnavailable, "No available Gemini accounts: "+err.Error())
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
			h.handleGeminiFailoverExhausted(c, lastFailoverErr)
			return
	REDACTED
		account := selection.Account
		setOpsSelectedAccount(c, account.ID, account.Platform)

		// 检测账号切换：如果粘性会话绑定的账号与当前选择的账号不同，清除 thoughtSignature
		// 注意：Gemini 原生 API 的 thoughtSignature 与具体上游账号强相关；跨账号透传会导致 400。
		if sessionBoundAccountID > 0 && sessionBoundAccountID != account.ID {
			log.Printf("[Gemini] Sticky session account switched: %d -> %d, cleaning thoughtSignature", sessionBoundAccountID, account.ID)
			body = service.CleanGeminiNativeThoughtSignatures(body)
			sessionBoundAccountID = account.ID
	REDACTED else if sessionKey != "" && sessionBoundAccountID == 0 && !cleanedForUnknownBinding && bytes.Contains(body, []byte(`"thoughtSignature"`)) {
			// 无缓存绑定但请求里已有 thoughtSignature：常见于缓存丢失/TTL 过期后，客户端继续携带旧签名。
			// 为避免第一次转发就 400，这里做一次确定性清理，让新账号重新生成签名链路。
			log.Printf("[Gemini] Sticky session binding missing, cleaning thoughtSignature proactively")
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
		if account.Platform == service.PlatformAntigravity && account.Type != service.AccountTypeAPIKey {
			result, err = h.antigravityGatewayService.ForwardGemini(requestCtx, c, account, modelName, action, stream, body, hasBoundSession)
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
				if needForceCacheBilling(hasBoundSession, failoverErr) {
					forceCacheBilling = true
			REDACTED
				if switchCount >= maxAccountSwitches {
					lastFailoverErr = failoverErr
					h.handleGeminiFailoverExhausted(c, lastFailoverErr)
					return
			REDACTED
				lastFailoverErr = failoverErr
				switchCount++
				log.Printf("Gemini account %d: upstream error %d, switching account %d/%d", account.ID, failoverErr.StatusCode, switchCount, maxAccountSwitches)
				if account.Platform == service.PlatformAntigravity {
					if !sleepFailoverDelay(c.Request.Context(), switchCount) {
						return
				REDACTED
			REDACTED
				continue
		REDACTED
			// ForwardNative already wrote the response
			log.Printf("Gemini native forward failed: %v", err)
			return
	REDACTED

		// 捕获请求信息（用于异步记录，避免在 goroutine 中访问 gin.Context）
		userAgent := c.GetHeader("User-Agent")
		clientIP := ip.GetClientIP(c)

		// 保存 Gemini 内容摘要会话（用于 Fallback 匹配）
		if useDigestFallback && geminiDigestChain != "" && geminiPrefixHash != "" {
			if err := h.gatewayService.SaveGeminiSession(
				c.Request.Context(),
				derefGroupID(apiKey.GroupID),
				geminiPrefixHash,
				geminiDigestChain,
				geminiSessionUUID,
				account.ID,
				matchedDigestChain,
			); err != nil {
				log.Printf("[Gemini] Failed to save digest session: %v", err)
		REDACTED
	REDACTED

		// 6) record usage async (Gemini 使用长上下文双倍计费)
		go func(result *service.ForwardResult, usedAccount *service.Account, ua, ip string, fcb bool) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := h.gatewayService.RecordUsageWithLongContext(ctx, &service.RecordUsageLongContextInput{
				Result:                result,
				APIKey:                apiKey,
				User:                  apiKey.User,
				Account:               usedAccount,
				Subscription:          subscription,
				UserAgent:             ua,
				IPAddress:             ip,
				LongContextThreshold:  200000, // Gemini 200K 阈值
				LongContextMultiplier: 2.0,    // 超出部分双倍计费
				ForceCacheBilling:     fcb,
				APIKeyService:         h.apiKeyService,
		REDACTED); err != nil {
				log.Printf("Record usage failed: %v", err)
		REDACTED
	REDACTED(result, account, userAgent, clientIP, forceCacheBilling)
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

func (h *GatewayHandler) handleGeminiFailoverExhausted(c *gin.Context, failoverErr *service.UpstreamFailoverError) {
	if failoverErr == nil {
		googleError(c, http.StatusBadGateway, "Upstream request failed")
		return
REDACTED

	statusCode := failoverErr.StatusCode
	responseBody := failoverErr.ResponseBody

	// 先检查透传规则
	if h.errorPassthroughService != nil && len(responseBody) > 0 {
		if rule := h.errorPassthroughService.MatchRule(service.PlatformGemini, statusCode, responseBody); rule != nil {
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

			googleError(c, respCode, msg)
			return
	REDACTED
REDACTED

	// 使用默认的错误映射
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

// truncateDigestChain 截断摘要链用于日志显示
func truncateDigestChain(chain string) string {
	if len(chain) <= 50 {
		return chain
REDACTED
	return chain[:50] + "..."
REDACTED

// safeShortPrefix 返回字符串前 n 个字符；长度不足时返回原字符串。
// 用于日志展示，避免切片越界。
func safeShortPrefix(value string, n int) string {
	if n <= 0 || len(value) <= n {
		return value
REDACTED
	return value[:n]
REDACTED

// derefGroupID 安全解引用 *int64，nil 返回 0
func derefGroupID(groupID *int64) int64 {
	if groupID == nil {
		return 0
REDACTED
	return *groupID
REDACTED
