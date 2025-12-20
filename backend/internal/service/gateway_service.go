package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"sub2api/internal/config"
	"sub2api/internal/model"
	"sub2api/internal/pkg/claude"
	"sub2api/internal/service/ports"

	"github.com/gin-gonic/gin"
)

// ClaudeUpstream handles HTTP requests to Claude API
type ClaudeUpstream interface {
	Do(req *http.Request, proxyURL string) (*http.Response, error)
REDACTED

const (
	claudeAPIURL            = "https://api.anthropic.com/v1/messages?beta=true"
	claudeAPICountTokensURL = "https://api.anthropic.com/v1/messages/count_tokens?beta=true"
	stickySessionTTL        = time.Hour // 粘性会话TTL
	tokenRefreshBuffer      = 5 * 60    // 提前5分钟刷新token
)

// allowedHeaders 白名单headers（参考CRS项目）
var allowedHeaders = map[string]bool{
	"accept":                                    true,
	"x-stainless-retry-count":                   true,
	"x-stainless-timeout":                       true,
	"x-stainless-lang":                          true,
	"x-stainless-package-version":               true,
	"x-stainless-os":                            true,
	"x-stainless-arch":                          true,
	"x-stainless-runtime":                       true,
	"x-stainless-runtime-version":               true,
	"x-stainless-helper-method":                 true,
	"anthropic-dangerous-direct-browser-access": true,
	"anthropic-version":                         true,
	"x-app":                                     true,
	"anthropic-beta":                            true,
	"accept-language":                           true,
	"sec-fetch-mode":                            true,
	"accept-encoding":                           true,
	"user-agent":                                true,
	"content-type":                              true,
REDACTED

// ClaudeUsage 表示Claude API返回的usage信息
type ClaudeUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
REDACTED

// ForwardResult 转发结果
type ForwardResult struct {
	RequestID    string
	Usage        ClaudeUsage
	Model        string
	Stream       bool
	Duration     time.Duration
	FirstTokenMs *int // 首字时间（流式请求）
REDACTED

// GatewayService handles API gateway operations
type GatewayService struct {
	accountRepo         ports.AccountRepository
	usageLogRepo        ports.UsageLogRepository
	userRepo            ports.UserRepository
	userSubRepo         ports.UserSubscriptionRepository
	cache               ports.GatewayCache
	cfg                 *config.Config
	oauthService        *OAuthService
	billingService      *BillingService
	rateLimitService    *RateLimitService
	billingCacheService *BillingCacheService
	identityService     *IdentityService
	claudeUpstream      ClaudeUpstream
REDACTED

// NewGatewayService creates a new GatewayService
func NewGatewayService(
	accountRepo ports.AccountRepository,
	usageLogRepo ports.UsageLogRepository,
	userRepo ports.UserRepository,
	userSubRepo ports.UserSubscriptionRepository,
	cache ports.GatewayCache,
	cfg *config.Config,
	oauthService *OAuthService,
	billingService *BillingService,
	rateLimitService *RateLimitService,
	billingCacheService *BillingCacheService,
	identityService *IdentityService,
	claudeUpstream ClaudeUpstream,
) *GatewayService {
	return &GatewayService{
		accountRepo:         accountRepo,
		usageLogRepo:        usageLogRepo,
		userRepo:            userRepo,
		userSubRepo:         userSubRepo,
		cache:               cache,
		cfg:                 cfg,
		oauthService:        oauthService,
		billingService:      billingService,
		rateLimitService:    rateLimitService,
		billingCacheService: billingCacheService,
		identityService:     identityService,
		claudeUpstream:      claudeUpstream,
REDACTED
REDACTED

// GenerateSessionHash 从请求体计算粘性会话hash
func (s *GatewayService) GenerateSessionHash(body []byte) string {
	var req map[string]interface{REDACTED
	if err := json.Unmarshal(body, &req); err != nil {
		return ""
REDACTED

	// 1. 最高优先级：从metadata.user_id提取session_xxx
	if metadata, ok := req["metadata"].(map[string]interface{REDACTED); ok {
		if userID, ok := metadata["user_id"].(string); ok {
			re := regexp.MustCompile(`session_([a-f0-9-]{36REDACTED)`)
			if match := re.FindStringSubmatch(userID); len(match) > 1 {
				return match[1]
		REDACTED
	REDACTED
REDACTED

	// 2. 提取带cache_control: {type: "ephemeral"REDACTED的内容
	cacheableContent := s.extractCacheableContent(req)
	if cacheableContent != "" {
		return s.hashContent(cacheableContent)
REDACTED

	// 3. Fallback: 使用system内容
	if system := req["system"]; system != nil {
		systemText := s.extractTextFromSystem(system)
		if systemText != "" {
			return s.hashContent(systemText)
	REDACTED
REDACTED

	// 4. 最后fallback: 使用第一条消息
	if messages, ok := req["messages"].([]interface{REDACTED); ok && len(messages) > 0 {
		if firstMsg, ok := messages[0].(map[string]interface{REDACTED); ok {
			msgText := s.extractTextFromContent(firstMsg["content"])
			if msgText != "" {
				return s.hashContent(msgText)
		REDACTED
	REDACTED
REDACTED

	return ""
REDACTED

func (s *GatewayService) extractCacheableContent(req map[string]interface{REDACTED) string {
	var content string

	// 检查system中的cacheable内容
	if system, ok := req["system"].([]interface{REDACTED); ok {
		for _, part := range system {
			if partMap, ok := part.(map[string]interface{REDACTED); ok {
				if cc, ok := partMap["cache_control"].(map[string]interface{REDACTED); ok {
					if cc["type"] == "ephemeral" {
						if text, ok := partMap["text"].(string); ok {
							content += text
					REDACTED
				REDACTED
			REDACTED
		REDACTED
	REDACTED
REDACTED

	// 检查messages中的cacheable内容
	if messages, ok := req["messages"].([]interface{REDACTED); ok {
		for _, msg := range messages {
			if msgMap, ok := msg.(map[string]interface{REDACTED); ok {
				if msgContent, ok := msgMap["content"].([]interface{REDACTED); ok {
					for _, part := range msgContent {
						if partMap, ok := part.(map[string]interface{REDACTED); ok {
							if cc, ok := partMap["cache_control"].(map[string]interface{REDACTED); ok {
								if cc["type"] == "ephemeral" {
									// 找到cacheable内容，提取第一条消息的文本
									return s.extractTextFromContent(msgMap["content"])
							REDACTED
						REDACTED
					REDACTED
				REDACTED
			REDACTED
		REDACTED
	REDACTED
REDACTED

	return content
REDACTED

func (s *GatewayService) extractTextFromSystem(system interface{REDACTED) string {
	switch v := system.(type) {
	case string:
		return v
	case []interface{REDACTED:
		var texts []string
		for _, part := range v {
			if partMap, ok := part.(map[string]interface{REDACTED); ok {
				if text, ok := partMap["text"].(string); ok {
					texts = append(texts, text)
			REDACTED
		REDACTED
	REDACTED
		return strings.Join(texts, "")
REDACTED
	return ""
REDACTED

func (s *GatewayService) extractTextFromContent(content interface{REDACTED) string {
	switch v := content.(type) {
	case string:
		return v
	case []interface{REDACTED:
		var texts []string
		for _, part := range v {
			if partMap, ok := part.(map[string]interface{REDACTED); ok {
				if partMap["type"] == "text" {
					if text, ok := partMap["text"].(string); ok {
						texts = append(texts, text)
				REDACTED
			REDACTED
		REDACTED
	REDACTED
		return strings.Join(texts, "")
REDACTED
	return ""
REDACTED

func (s *GatewayService) hashContent(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:16]) // 32字符
REDACTED

// replaceModelInBody 替换请求体中的model字段
func (s *GatewayService) replaceModelInBody(body []byte, newModel string) []byte {
	var req map[string]interface{REDACTED
	if err := json.Unmarshal(body, &req); err != nil {
		return body
REDACTED
	req["model"] = newModel
	newBody, err := json.Marshal(req)
	if err != nil {
		return body
REDACTED
	return newBody
REDACTED

// SelectAccount 选择账号（粘性会话+优先级）
func (s *GatewayService) SelectAccount(ctx context.Context, groupID *int64, sessionHash string) (*model.Account, error) {
	return s.SelectAccountForModel(ctx, groupID, sessionHash, "")
REDACTED

// SelectAccountForModel 选择支持指定模型的账号（粘性会话+优先级+模型映射）
func (s *GatewayService) SelectAccountForModel(ctx context.Context, groupID *int64, sessionHash string, requestedModel string) (*model.Account, error) {
	// 1. 查询粘性会话
	if sessionHash != "" {
		accountID, err := s.cache.GetSessionAccountID(ctx, sessionHash)
		if err == nil && accountID > 0 {
			account, err := s.accountRepo.GetByID(ctx, accountID)
			// 使用IsSchedulable代替IsActive，确保限流/过载账号不会被选中
			// 同时检查模型支持
			if err == nil && account.IsSchedulable() && (requestedModel == "" || account.IsModelSupported(requestedModel)) {
				// 续期粘性会话
				s.cache.RefreshSessionTTL(ctx, sessionHash, stickySessionTTL)
				return account, nil
		REDACTED
	REDACTED
REDACTED

	// 2. 获取可调度账号列表（排除限流和过载的账号）
	var accounts []model.Account
	var err error
	if groupID != nil {
		accounts, err = s.accountRepo.ListSchedulableByGroupID(ctx, *groupID)
REDACTED else {
		accounts, err = s.accountRepo.ListSchedulable(ctx)
REDACTED
	if err != nil {
		return nil, fmt.Errorf("query accounts failed: %w", err)
REDACTED

	// 3. 按优先级+最久未用选择（考虑模型支持）
	var selected *model.Account
	for i := range accounts {
		acc := &accounts[i]
		// 检查模型支持
		if requestedModel != "" && !acc.IsModelSupported(requestedModel) {
			continue
	REDACTED
		if selected == nil {
			selected = acc
			continue
	REDACTED
		// 优先选择priority值更小的（priority值越小优先级越高）
		if acc.Priority < selected.Priority {
			selected = acc
	REDACTED else if acc.Priority == selected.Priority {
			// 优先级相同时，选最久未用的
			if acc.LastUsedAt == nil || (selected.LastUsedAt != nil && acc.LastUsedAt.Before(*selected.LastUsedAt)) {
				selected = acc
		REDACTED
	REDACTED
REDACTED

	if selected == nil {
		if requestedModel != "" {
			return nil, fmt.Errorf("no available accounts supporting model: %s", requestedModel)
	REDACTED
		return nil, errors.New("no available accounts")
REDACTED

	// 4. 建立粘性绑定
	if sessionHash != "" {
		s.cache.SetSessionAccountID(ctx, sessionHash, selected.ID, stickySessionTTL)
REDACTED

	return selected, nil
REDACTED

// GetAccessToken 获取账号凭证
func (s *GatewayService) GetAccessToken(ctx context.Context, account *model.Account) (string, string, error) {
	switch account.Type {
	case model.AccountTypeOAuth, model.AccountTypeSetupToken:
		// Both oauth and setup-token use OAuth token flow
		return s.getOAuthToken(ctx, account)
	case model.AccountTypeApiKey:
		apiKey := account.GetCredential("api_key")
		if apiKey == "" {
			return "", "", errors.New("api_key not found in credentials")
	REDACTED
		return apiKey, "apikey", nil
	default:
		return "", "", fmt.Errorf("unsupported account type: %s", account.Type)
REDACTED
REDACTED

func (s *GatewayService) getOAuthToken(ctx context.Context, account *model.Account) (string, string, error) {
	accessToken := account.GetCredential("access_token")
	expiresAtStr := account.GetCredential("expires_at")

	// 检查是否需要刷新
	needRefresh := false
	if expiresAtStr != "" {
		expiresAt, err := strconv.ParseInt(expiresAtStr, 10, 64)
		if err == nil && time.Now().Unix()+tokenRefreshBuffer > expiresAt {
			needRefresh = true
	REDACTED
REDACTED

	if needRefresh || accessToken == "" {
		tokenInfo, err := s.oauthService.RefreshAccountToken(ctx, account)
		if err != nil {
			return "", "", fmt.Errorf("refresh token failed: %w", err)
	REDACTED

		// 更新账号凭证
		account.Credentials["access_token"] = tokenInfo.AccessToken
		account.Credentials["expires_at"] = strconv.FormatInt(tokenInfo.ExpiresAt, 10)
		if tokenInfo.RefreshToken != "" {
			account.Credentials["refresh_token"] = tokenInfo.RefreshToken
	REDACTED

		if err := s.accountRepo.Update(ctx, account); err != nil {
			log.Printf("Failed to update account credentials: %v", err)
	REDACTED

		return tokenInfo.AccessToken, "oauth", nil
REDACTED

	return accessToken, "oauth", nil
REDACTED

// Forward 转发请求到Claude API
func (s *GatewayService) Forward(ctx context.Context, c *gin.Context, account *model.Account, body []byte) (*ForwardResult, error) {
	startTime := time.Now()

	// 解析请求获取model和stream
	var req struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
REDACTED
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("parse request: %w", err)
REDACTED

	// 应用模型映射（仅对apikey类型账号）
	originalModel := req.Model
	if account.Type == model.AccountTypeApiKey {
		mappedModel := account.GetMappedModel(req.Model)
		if mappedModel != req.Model {
			// 替换请求体中的模型名
			body = s.replaceModelInBody(body, mappedModel)
			req.Model = mappedModel
			log.Printf("Model mapping applied: %s -> %s (account: %s)", originalModel, mappedModel, account.Name)
	REDACTED
REDACTED

	// 获取凭证
	token, tokenType, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
REDACTED

	// 构建上游请求
	upstreamReq, err := s.buildUpstreamRequest(ctx, c, account, body, token, tokenType)
	if err != nil {
		return nil, err
REDACTED

	// 获取代理URL
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED

	// 发送请求
	resp, err := s.claudeUpstream.Do(upstreamReq, proxyURL)
	if err != nil {
		return nil, fmt.Errorf("upstream request failed: %w", err)
REDACTED
	defer resp.Body.Close()

	// 处理401错误：刷新token重试
	if resp.StatusCode == http.StatusUnauthorized && tokenType == "oauth" {
		resp.Body.Close()
		token, tokenType, err = s.forceRefreshToken(ctx, account)
		if err != nil {
			return nil, fmt.Errorf("token refresh failed: %w", err)
	REDACTED
		upstreamReq, err = s.buildUpstreamRequest(ctx, c, account, body, token, tokenType)
		if err != nil {
			return nil, err
	REDACTED
		resp, err = s.claudeUpstream.Do(upstreamReq, proxyURL)
		if err != nil {
			return nil, fmt.Errorf("retry request failed: %w", err)
	REDACTED
		defer resp.Body.Close()
REDACTED

	// 处理错误响应
	if resp.StatusCode >= 400 {
		return s.handleErrorResponse(ctx, resp, c, account)
REDACTED

	// 处理正常响应
	var usage *ClaudeUsage
	var firstTokenMs *int
	if req.Stream {
		streamResult, err := s.handleStreamingResponse(ctx, resp, c, account, startTime, originalModel, req.Model)
		if err != nil {
			return nil, err
	REDACTED
		usage = streamResult.usage
		firstTokenMs = streamResult.firstTokenMs
REDACTED else {
		usage, err = s.handleNonStreamingResponse(ctx, resp, c, account, originalModel, req.Model)
		if err != nil {
			return nil, err
	REDACTED
REDACTED

	return &ForwardResult{
		RequestID:    resp.Header.Get("x-request-id"),
		Usage:        *usage,
		Model:        originalModel, // 使用原始模型用于计费和日志
		Stream:       req.Stream,
		Duration:     time.Since(startTime),
		FirstTokenMs: firstTokenMs,
REDACTED, nil
REDACTED

func (s *GatewayService) buildUpstreamRequest(ctx context.Context, c *gin.Context, account *model.Account, body []byte, token, tokenType string) (*http.Request, error) {
	// 确定目标URL
	targetURL := claudeAPIURL
	if account.Type == model.AccountTypeApiKey {
		baseURL := account.GetBaseURL()
		targetURL = baseURL + "/v1/messages"
REDACTED

	// OAuth账号：应用统一指纹
	var fingerprint *ports.Fingerprint
	if account.IsOAuth() && s.identityService != nil {
		// 1. 获取或创建指纹（包含随机生成的ClientID）
		fp, err := s.identityService.GetOrCreateFingerprint(ctx, account.ID, c.Request.Header)
		if err != nil {
			log.Printf("Warning: failed to get fingerprint for account %d: %v", account.ID, err)
			// 失败时降级为透传原始headers
	REDACTED else {
			fingerprint = fp

			// 2. 重写metadata.user_id（需要指纹中的ClientID和账号的account_uuid）
			accountUUID := account.GetExtraString("account_uuid")
			if accountUUID != "" && fp.ClientID != "" {
				if newBody, err := s.identityService.RewriteUserID(body, account.ID, accountUUID, fp.ClientID); err == nil && len(newBody) > 0 {
					body = newBody
			REDACTED
		REDACTED
	REDACTED
REDACTED

	req, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
REDACTED

	// 设置认证头
	if tokenType == "oauth" {
		req.Header.Set("Authorization", "Bearer "+token)
REDACTED else {
		req.Header.Set("x-api-key", token)
REDACTED

	// 白名单透传headers
	for key, values := range c.Request.Header {
		lowerKey := strings.ToLower(key)
		if allowedHeaders[lowerKey] {
			for _, v := range values {
				req.Header.Add(key, v)
		REDACTED
	REDACTED
REDACTED

	// OAuth账号：应用缓存的指纹到请求头（覆盖白名单透传的头）
	if fingerprint != nil {
		s.identityService.ApplyFingerprint(req, fingerprint)
REDACTED

	// 确保必要的headers存在
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
REDACTED
	if req.Header.Get("anthropic-version") == "" {
		req.Header.Set("anthropic-version", "2023-06-01")
REDACTED

	// 处理anthropic-beta header（OAuth账号需要特殊处理）
	if tokenType == "oauth" {
		req.Header.Set("anthropic-beta", s.getBetaHeader(body, c.GetHeader("anthropic-beta")))
REDACTED

	return req, nil
REDACTED

// getBetaHeader 处理anthropic-beta header
// 对于OAuth账号，需要确保包含oauth-2025-04-20
func (s *GatewayService) getBetaHeader(body []byte, clientBetaHeader string) string {
	// 如果客户端传了anthropic-beta
	if clientBetaHeader != "" {
		// 已包含oauth beta则直接返回
		if strings.Contains(clientBetaHeader, claude.BetaOAuth) {
			return clientBetaHeader
	REDACTED

		// 需要添加oauth beta
		parts := strings.Split(clientBetaHeader, ",")
		for i, p := range parts {
			parts[i] = strings.TrimSpace(p)
	REDACTED

		// 在claude-code-20250219后面插入oauth beta
		claudeCodeIdx := -1
		for i, p := range parts {
			if p == claude.BetaClaudeCode {
				claudeCodeIdx = i
				break
		REDACTED
	REDACTED

		if claudeCodeIdx >= 0 {
			// 在claude-code后面插入
			newParts := make([]string, 0, len(parts)+1)
			newParts = append(newParts, parts[:claudeCodeIdx+1]...)
			newParts = append(newParts, claude.BetaOAuth)
			newParts = append(newParts, parts[claudeCodeIdx+1:]...)
			return strings.Join(newParts, ",")
	REDACTED

		// 没有claude-code，放在第一位
		return claude.BetaOAuth + "," + clientBetaHeader
REDACTED

	// 客户端没传，根据模型生成
	var modelID string
	var reqMap map[string]interface{REDACTED
	if json.Unmarshal(body, &reqMap) == nil {
		if m, ok := reqMap["model"].(string); ok {
			modelID = m
	REDACTED
REDACTED

	// haiku模型不需要claude-code beta
	if strings.Contains(strings.ToLower(modelID), "haiku") {
		return claude.HaikuBetaHeader
REDACTED

	return claude.DefaultBetaHeader
REDACTED

func (s *GatewayService) forceRefreshToken(ctx context.Context, account *model.Account) (string, string, error) {
	tokenInfo, err := s.oauthService.RefreshAccountToken(ctx, account)
	if err != nil {
		return "", "", err
REDACTED

	account.Credentials["access_token"] = tokenInfo.AccessToken
	account.Credentials["expires_at"] = strconv.FormatInt(tokenInfo.ExpiresAt, 10)
	if tokenInfo.RefreshToken != "" {
		account.Credentials["refresh_token"] = tokenInfo.RefreshToken
REDACTED

	if err := s.accountRepo.Update(ctx, account); err != nil {
		log.Printf("Failed to update account credentials: %v", err)
REDACTED

	return tokenInfo.AccessToken, "oauth", nil
REDACTED

func (s *GatewayService) handleErrorResponse(ctx context.Context, resp *http.Response, c *gin.Context, account *model.Account) (*ForwardResult, error) {
	body, _ := io.ReadAll(resp.Body)

	// apikey 类型账号：检查自定义错误码配置
	// 如果启用且错误码不在列表中，返回通用 500 错误（不做任何账号状态处理）
	if !account.ShouldHandleErrorCode(resp.StatusCode) {
		c.JSON(http.StatusInternalServerError, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    "upstream_error",
				"message": "Upstream gateway error",
		REDACTED,
	REDACTED)
		return nil, fmt.Errorf("upstream error: %d (not in custom error codes)", resp.StatusCode)
REDACTED

	// 处理上游错误，标记账号状态
	s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, body)

	// 根据状态码返回适当的自定义错误响应（不透传上游详细信息）
	var errType, errMsg string
	var statusCode int

	switch resp.StatusCode {
	case 401:
		statusCode = http.StatusBadGateway
		errType = "upstream_error"
		errMsg = "Upstream authentication failed, please contact administrator"
	case 403:
		statusCode = http.StatusBadGateway
		errType = "upstream_error"
		errMsg = "Upstream access forbidden, please contact administrator"
	case 429:
		statusCode = http.StatusTooManyRequests
		errType = "rate_limit_error"
		errMsg = "Upstream rate limit exceeded, please retry later"
	case 529:
		statusCode = http.StatusServiceUnavailable
		errType = "overloaded_error"
		errMsg = "Upstream service overloaded, please retry later"
	case 500, 502, 503, 504:
		statusCode = http.StatusBadGateway
		errType = "upstream_error"
		errMsg = "Upstream service temporarily unavailable"
	default:
		statusCode = http.StatusBadGateway
		errType = "upstream_error"
		errMsg = "Upstream request failed"
REDACTED

	// 返回自定义错误响应
	c.JSON(statusCode, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    errType,
			"message": errMsg,
	REDACTED,
REDACTED)

	return nil, fmt.Errorf("upstream error: %d", resp.StatusCode)
REDACTED

// streamingResult 流式响应结果
type streamingResult struct {
	usage        *ClaudeUsage
	firstTokenMs *int
REDACTED

func (s *GatewayService) handleStreamingResponse(ctx context.Context, resp *http.Response, c *gin.Context, account *model.Account, startTime time.Time, originalModel, mappedModel string) (*streamingResult, error) {
	// 更新5h窗口状态
	s.rateLimitService.UpdateSessionWindow(ctx, account, resp.Header)

	// 设置SSE响应头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	// 透传其他响应头
	if v := resp.Header.Get("x-request-id"); v != "" {
		c.Header("x-request-id", v)
REDACTED

	w := c.Writer
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
REDACTED

	usage := &ClaudeUsage{REDACTED
	var firstTokenMs *int
	scanner := bufio.NewScanner(resp.Body)
	// 设置更大的buffer以处理长行
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	needModelReplace := originalModel != mappedModel

	for scanner.Scan() {
		line := scanner.Text()

		// 如果有模型映射，替换响应中的model字段
		if needModelReplace && strings.HasPrefix(line, "data: ") {
			line = s.replaceModelInSSELine(line, mappedModel, originalModel)
	REDACTED

		// 转发行
		fmt.Fprintf(w, "%s\n", line)
		flusher.Flush()

		// 解析usage数据
		if strings.HasPrefix(line, "data: ") {
			data := line[6:]
			// 记录首字时间：第一个有效的 content_block_delta 或 message_start
			if firstTokenMs == nil && data != "" && data != "[DONE]" {
				ms := int(time.Since(startTime).Milliseconds())
				firstTokenMs = &ms
		REDACTED
			s.parseSSEUsage(data, usage)
	REDACTED
REDACTED

	if err := scanner.Err(); err != nil {
		return &streamingResult{usage: usage, firstTokenMs: firstTokenMsREDACTED, fmt.Errorf("stream read error: %w", err)
REDACTED

	return &streamingResult{usage: usage, firstTokenMs: firstTokenMsREDACTED, nil
REDACTED

// replaceModelInSSELine 替换SSE数据行中的model字段
func (s *GatewayService) replaceModelInSSELine(line, fromModel, toModel string) string {
	data := line[6:] // 去掉 "data: " 前缀
	if data == "" || data == "[DONE]" {
		return line
REDACTED

	var event map[string]interface{REDACTED
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return line
REDACTED

	// 只替换 message_start 事件中的 message.model
	if event["type"] != "message_start" {
		return line
REDACTED

	msg, ok := event["message"].(map[string]interface{REDACTED)
	if !ok {
		return line
REDACTED

	model, ok := msg["model"].(string)
	if !ok || model != fromModel {
		return line
REDACTED

	msg["model"] = toModel
	newData, err := json.Marshal(event)
	if err != nil {
		return line
REDACTED

	return "data: " + string(newData)
REDACTED

func (s *GatewayService) parseSSEUsage(data string, usage *ClaudeUsage) {
	// 解析message_start获取input tokens
	var msgStart struct {
		Type    string `json:"type"`
		Message struct {
			Usage ClaudeUsage `json:"usage"`
	REDACTED `json:"message"`
REDACTED
	if json.Unmarshal([]byte(data), &msgStart) == nil && msgStart.Type == "message_start" {
		usage.InputTokens = msgStart.Message.Usage.InputTokens
		usage.CacheCreationInputTokens = msgStart.Message.Usage.CacheCreationInputTokens
		usage.CacheReadInputTokens = msgStart.Message.Usage.CacheReadInputTokens
REDACTED

	// 解析message_delta获取output tokens
	var msgDelta struct {
		Type  string `json:"type"`
		Usage struct {
			OutputTokens int `json:"output_tokens"`
	REDACTED `json:"usage"`
REDACTED
	if json.Unmarshal([]byte(data), &msgDelta) == nil && msgDelta.Type == "message_delta" {
		usage.OutputTokens = msgDelta.Usage.OutputTokens
REDACTED
REDACTED

func (s *GatewayService) handleNonStreamingResponse(ctx context.Context, resp *http.Response, c *gin.Context, account *model.Account, originalModel, mappedModel string) (*ClaudeUsage, error) {
	// 更新5h窗口状态
	s.rateLimitService.UpdateSessionWindow(ctx, account, resp.Header)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
REDACTED

	// 解析usage
	var response struct {
		Usage ClaudeUsage `json:"usage"`
REDACTED
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
REDACTED

	// 如果有模型映射，替换响应中的model字段
	if originalModel != mappedModel {
		body = s.replaceModelInResponseBody(body, mappedModel, originalModel)
REDACTED

	// 透传响应头
	for key, values := range resp.Header {
		for _, value := range values {
			c.Header(key, value)
	REDACTED
REDACTED

	// 写入响应
	c.Data(resp.StatusCode, "application/json", body)

	return &response.Usage, nil
REDACTED

// replaceModelInResponseBody 替换响应体中的model字段
func (s *GatewayService) replaceModelInResponseBody(body []byte, fromModel, toModel string) []byte {
	var resp map[string]interface{REDACTED
	if err := json.Unmarshal(body, &resp); err != nil {
		return body
REDACTED

	model, ok := resp["model"].(string)
	if !ok || model != fromModel {
		return body
REDACTED

	resp["model"] = toModel
	newBody, err := json.Marshal(resp)
	if err != nil {
		return body
REDACTED

	return newBody
REDACTED

// RecordUsageInput 记录使用量的输入参数
type RecordUsageInput struct {
	Result       *ForwardResult
	ApiKey       *model.ApiKey
	User         *model.User
	Account      *model.Account
	Subscription *model.UserSubscription // 可选：订阅信息
REDACTED

// RecordUsage 记录使用量并扣费（或更新订阅用量）
func (s *GatewayService) RecordUsage(ctx context.Context, input *RecordUsageInput) error {
	result := input.Result
	apiKey := input.ApiKey
	user := input.User
	account := input.Account
	subscription := input.Subscription

	// 计算费用
	tokens := UsageTokens{
		InputTokens:         result.Usage.InputTokens,
		OutputTokens:        result.Usage.OutputTokens,
		CacheCreationTokens: result.Usage.CacheCreationInputTokens,
		CacheReadTokens:     result.Usage.CacheReadInputTokens,
REDACTED

	// 获取费率倍数
	multiplier := s.cfg.Default.RateMultiplier
	if apiKey.GroupID != nil && apiKey.Group != nil {
		multiplier = apiKey.Group.RateMultiplier
REDACTED

	cost, err := s.billingService.CalculateCost(result.Model, tokens, multiplier)
	if err != nil {
		log.Printf("Calculate cost failed: %v", err)
		// 使用默认费用继续
		cost = &CostBreakdown{ActualCost: 0REDACTED
REDACTED

	// 判断计费方式：订阅模式 vs 余额模式
	isSubscriptionBilling := subscription != nil && apiKey.Group != nil && apiKey.Group.IsSubscriptionType()
	billingType := model.BillingTypeBalance
	if isSubscriptionBilling {
		billingType = model.BillingTypeSubscription
REDACTED

	// 创建使用日志
	durationMs := int(result.Duration.Milliseconds())
	usageLog := &model.UsageLog{
		UserID:              user.ID,
		ApiKeyID:            apiKey.ID,
		AccountID:           account.ID,
		RequestID:           result.RequestID,
		Model:               result.Model,
		InputTokens:         result.Usage.InputTokens,
		OutputTokens:        result.Usage.OutputTokens,
		CacheCreationTokens: result.Usage.CacheCreationInputTokens,
		CacheReadTokens:     result.Usage.CacheReadInputTokens,
		InputCost:           cost.InputCost,
		OutputCost:          cost.OutputCost,
		CacheCreationCost:   cost.CacheCreationCost,
		CacheReadCost:       cost.CacheReadCost,
		TotalCost:           cost.TotalCost,
		ActualCost:          cost.ActualCost,
		RateMultiplier:      multiplier,
		BillingType:         billingType,
		Stream:              result.Stream,
		DurationMs:          &durationMs,
		FirstTokenMs:        result.FirstTokenMs,
		CreatedAt:           time.Now(),
REDACTED

	// 添加分组和订阅关联
	if apiKey.GroupID != nil {
		usageLog.GroupID = apiKey.GroupID
REDACTED
	if subscription != nil {
		usageLog.SubscriptionID = &subscription.ID
REDACTED

	if err := s.usageLogRepo.Create(ctx, usageLog); err != nil {
		log.Printf("Create usage log failed: %v", err)
REDACTED

	// 根据计费类型执行扣费
	if isSubscriptionBilling {
		// 订阅模式：更新订阅用量（使用 TotalCost 原始费用，不考虑倍率）
		if cost.TotalCost > 0 {
			if err := s.userSubRepo.IncrementUsage(ctx, subscription.ID, cost.TotalCost); err != nil {
				log.Printf("Increment subscription usage failed: %v", err)
		REDACTED
			// 异步更新订阅缓存
			go func() {
				cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := s.billingCacheService.UpdateSubscriptionUsage(cacheCtx, user.ID, *apiKey.GroupID, cost.TotalCost); err != nil {
					log.Printf("Update subscription cache failed: %v", err)
			REDACTED
		REDACTED()
	REDACTED
REDACTED else {
		// 余额模式：扣除用户余额（使用 ActualCost 考虑倍率后的费用）
		if cost.ActualCost > 0 {
			if err := s.userRepo.DeductBalance(ctx, user.ID, cost.ActualCost); err != nil {
				log.Printf("Deduct balance failed: %v", err)
		REDACTED
			// 异步更新余额缓存
			go func() {
				cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := s.billingCacheService.DeductBalanceCache(cacheCtx, user.ID, cost.ActualCost); err != nil {
					log.Printf("Update balance cache failed: %v", err)
			REDACTED
		REDACTED()
	REDACTED
REDACTED

	// 更新账号最后使用时间
	if err := s.accountRepo.UpdateLastUsed(ctx, account.ID); err != nil {
		log.Printf("Update last used failed: %v", err)
REDACTED

	return nil
REDACTED

// ForwardCountTokens 转发 count_tokens 请求到上游 API
// 特点：不记录使用量、仅支持非流式响应
func (s *GatewayService) ForwardCountTokens(ctx context.Context, c *gin.Context, account *model.Account, body []byte) error {
	// 应用模型映射（仅对 apikey 类型账号）
	if account.Type == model.AccountTypeApiKey {
		var req struct {
			Model string `json:"model"`
	REDACTED
		if err := json.Unmarshal(body, &req); err == nil && req.Model != "" {
			mappedModel := account.GetMappedModel(req.Model)
			if mappedModel != req.Model {
				body = s.replaceModelInBody(body, mappedModel)
				log.Printf("CountTokens model mapping applied: %s -> %s (account: %s)", req.Model, mappedModel, account.Name)
		REDACTED
	REDACTED
REDACTED

	// 获取凭证
	token, tokenType, err := s.GetAccessToken(ctx, account)
	if err != nil {
		s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Failed to get access token")
		return err
REDACTED

	// 构建上游请求
	upstreamReq, err := s.buildCountTokensRequest(ctx, c, account, body, token, tokenType)
	if err != nil {
		s.countTokensError(c, http.StatusInternalServerError, "api_error", "Failed to build request")
		return err
REDACTED

	// 获取代理URL
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED

	// 发送请求
	resp, err := s.claudeUpstream.Do(upstreamReq, proxyURL)
	if err != nil {
		s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Request failed")
		return fmt.Errorf("upstream request failed: %w", err)
REDACTED
	defer resp.Body.Close()

	// 处理 401 错误：刷新 token 重试（仅 OAuth）
	if resp.StatusCode == http.StatusUnauthorized && tokenType == "oauth" {
		resp.Body.Close()
		token, tokenType, err = s.forceRefreshToken(ctx, account)
		if err != nil {
			s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Token refresh failed")
			return fmt.Errorf("token refresh failed: %w", err)
	REDACTED
		upstreamReq, err = s.buildCountTokensRequest(ctx, c, account, body, token, tokenType)
		if err != nil {
			return err
	REDACTED
		resp, err = s.claudeUpstream.Do(upstreamReq, proxyURL)
		if err != nil {
			s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Retry failed")
			return fmt.Errorf("retry request failed: %w", err)
	REDACTED
		defer resp.Body.Close()
REDACTED

	// 读取响应体
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Failed to read response")
		return err
REDACTED

	// 处理错误响应
	if resp.StatusCode >= 400 {
		// 标记账号状态（429/529等）
		s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)

		// 返回简化的错误响应
		errMsg := "Upstream request failed"
		switch resp.StatusCode {
		case 429:
			errMsg = "Rate limit exceeded"
		case 529:
			errMsg = "Service overloaded"
	REDACTED
		s.countTokensError(c, resp.StatusCode, "upstream_error", errMsg)
		return fmt.Errorf("upstream error: %d", resp.StatusCode)
REDACTED

	// 透传成功响应
	c.Data(resp.StatusCode, "application/json", respBody)
	return nil
REDACTED

// buildCountTokensRequest 构建 count_tokens 上游请求
func (s *GatewayService) buildCountTokensRequest(ctx context.Context, c *gin.Context, account *model.Account, body []byte, token, tokenType string) (*http.Request, error) {
	// 确定目标 URL
	targetURL := claudeAPICountTokensURL
	if account.Type == model.AccountTypeApiKey {
		baseURL := account.GetBaseURL()
		targetURL = baseURL + "/v1/messages/count_tokens"
REDACTED

	// OAuth 账号：应用统一指纹和重写 userID
	if account.IsOAuth() && s.identityService != nil {
		fp, err := s.identityService.GetOrCreateFingerprint(ctx, account.ID, c.Request.Header)
		if err == nil {
			accountUUID := account.GetExtraString("account_uuid")
			if accountUUID != "" && fp.ClientID != "" {
				if newBody, err := s.identityService.RewriteUserID(body, account.ID, accountUUID, fp.ClientID); err == nil && len(newBody) > 0 {
					body = newBody
			REDACTED
		REDACTED
	REDACTED
REDACTED

	req, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
REDACTED

	// 设置认证头
	if tokenType == "oauth" {
		req.Header.Set("Authorization", "Bearer "+token)
REDACTED else {
		req.Header.Set("x-api-key", token)
REDACTED

	// 白名单透传 headers
	for key, values := range c.Request.Header {
		lowerKey := strings.ToLower(key)
		if allowedHeaders[lowerKey] {
			for _, v := range values {
				req.Header.Add(key, v)
		REDACTED
	REDACTED
REDACTED

	// OAuth 账号：应用指纹到请求头
	if account.IsOAuth() && s.identityService != nil {
		fp, _ := s.identityService.GetOrCreateFingerprint(ctx, account.ID, c.Request.Header)
		if fp != nil {
			s.identityService.ApplyFingerprint(req, fp)
	REDACTED
REDACTED

	// 确保必要的 headers 存在
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
REDACTED
	if req.Header.Get("anthropic-version") == "" {
		req.Header.Set("anthropic-version", "2023-06-01")
REDACTED

	// OAuth 账号：处理 anthropic-beta header
	if tokenType == "oauth" {
		req.Header.Set("anthropic-beta", s.getBetaHeader(body, c.GetHeader("anthropic-beta")))
REDACTED

	return req, nil
REDACTED

// countTokensError 返回 count_tokens 错误响应
func (s *GatewayService) countTokensError(c *gin.Context, status int, errType, message string) {
	c.JSON(status, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    errType,
			"message": message,
	REDACTED,
REDACTED)
REDACTED
