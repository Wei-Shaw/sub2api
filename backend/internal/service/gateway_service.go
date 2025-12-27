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
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/gin-gonic/gin"
)

const (
	claudeAPIURL            = "https://api.anthropic.com/v1/messages?beta=true"
	claudeAPICountTokensURL = "https://api.anthropic.com/v1/messages/count_tokens?beta=true"
	stickySessionTTL        = time.Hour // 粘性会话TTL
)

// sseDataRe matches SSE data lines with optional whitespace after colon.
// Some upstream APIs return non-standard "data:" without space (should be "data: ").
var sseDataRe = regexp.MustCompile(`^data:\s*`)

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
	"user-agent":                                true,
	"content-type":                              true,
}

// GatewayCache defines cache operations for gateway service
type GatewayCache interface {
	GetSessionAccountID(ctx context.Context, sessionHash string) (int64, error)
	SetSessionAccountID(ctx context.Context, sessionHash string, accountID int64, ttl time.Duration) error
	RefreshSessionTTL(ctx context.Context, sessionHash string, ttl time.Duration) error
}

// ClaudeUsage 表示Claude API返回的usage信息
type ClaudeUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// ForwardResult 转发结果
type ForwardResult struct {
	RequestID    string
	Usage        ClaudeUsage
	Model        string
	Stream       bool
	Duration     time.Duration
	FirstTokenMs *int // 首字时间（流式请求）
}

// UpstreamFailoverError indicates an upstream error that should trigger account failover.
type UpstreamFailoverError struct {
	StatusCode int
}

func (e *UpstreamFailoverError) Error() string {
	return fmt.Sprintf("upstream error: %d (failover)", e.StatusCode)
}

// GatewayService handles API gateway operations
type GatewayService struct {
	accountRepo         AccountRepository
	usageLogRepo        UsageLogRepository
	userRepo            UserRepository
	userSubRepo         UserSubscriptionRepository
	cache               GatewayCache
	cfg                 *config.Config
	billingService      *BillingService
	rateLimitService    *RateLimitService
	billingCacheService *BillingCacheService
	identityService     *IdentityService
	httpUpstream        HTTPUpstream
}

// NewGatewayService creates a new GatewayService
func NewGatewayService(
	accountRepo AccountRepository,
	usageLogRepo UsageLogRepository,
	userRepo UserRepository,
	userSubRepo UserSubscriptionRepository,
	cache GatewayCache,
	cfg *config.Config,
	billingService *BillingService,
	rateLimitService *RateLimitService,
	billingCacheService *BillingCacheService,
	identityService *IdentityService,
	httpUpstream HTTPUpstream,
) *GatewayService {
	return &GatewayService{
		accountRepo:         accountRepo,
		usageLogRepo:        usageLogRepo,
		userRepo:            userRepo,
		userSubRepo:         userSubRepo,
		cache:               cache,
		cfg:                 cfg,
		billingService:      billingService,
		rateLimitService:    rateLimitService,
		billingCacheService: billingCacheService,
		identityService:     identityService,
		httpUpstream:        httpUpstream,
	}
}

// GenerateSessionHash 从请求体计算粘性会话hash
func (s *GatewayService) GenerateSessionHash(body []byte) string {
	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return ""
	}

	// 1. 最高优先级：从metadata.user_id提取session_xxx
	if metadata, ok := req["metadata"].(map[string]any); ok {
		if userID, ok := metadata["user_id"].(string); ok {
			re := regexp.MustCompile(`session_([a-f0-9-]{36})`)
			if match := re.FindStringSubmatch(userID); len(match) > 1 {
				return match[1]
			}
		}
	}

	// 2. 提取带cache_control: {type: "ephemeral"}的内容
	cacheableContent := s.extractCacheableContent(req)
	if cacheableContent != "" {
		return s.hashContent(cacheableContent)
	}

	// 3. Fallback: 使用system内容
	if system := req["system"]; system != nil {
		systemText := s.extractTextFromSystem(system)
		if systemText != "" {
			return s.hashContent(systemText)
		}
	}

	// 4. 最后fallback: 使用第一条消息
	if messages, ok := req["messages"].([]any); ok && len(messages) > 0 {
		if firstMsg, ok := messages[0].(map[string]any); ok {
			msgText := s.extractTextFromContent(firstMsg["content"])
			if msgText != "" {
				return s.hashContent(msgText)
			}
		}
	}

	return ""
}

func (s *GatewayService) extractCacheableContent(req map[string]any) string {
	var content string

	// 检查system中的cacheable内容
	if system, ok := req["system"].([]any); ok {
		for _, part := range system {
			if partMap, ok := part.(map[string]any); ok {
				if cc, ok := partMap["cache_control"].(map[string]any); ok {
					if cc["type"] == "ephemeral" {
						if text, ok := partMap["text"].(string); ok {
							content += text
						}
					}
				}
			}
		}
	}

	// 检查messages中的cacheable内容
	if messages, ok := req["messages"].([]any); ok {
		for _, msg := range messages {
			if msgMap, ok := msg.(map[string]any); ok {
				if msgContent, ok := msgMap["content"].([]any); ok {
					for _, part := range msgContent {
						if partMap, ok := part.(map[string]any); ok {
							if cc, ok := partMap["cache_control"].(map[string]any); ok {
								if cc["type"] == "ephemeral" {
									// 找到cacheable内容，提取第一条消息的文本
									return s.extractTextFromContent(msgMap["content"])
								}
							}
						}
					}
				}
			}
		}
	}

	return content
}

func (s *GatewayService) extractTextFromSystem(system any) string {
	switch v := system.(type) {
	case string:
		return v
	case []any:
		var texts []string
		for _, part := range v {
			if partMap, ok := part.(map[string]any); ok {
				if text, ok := partMap["text"].(string); ok {
					texts = append(texts, text)
				}
			}
		}
		return strings.Join(texts, "")
	}
	return ""
}

func (s *GatewayService) extractTextFromContent(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var texts []string
		for _, part := range v {
			if partMap, ok := part.(map[string]any); ok {
				if partMap["type"] == "text" {
					if text, ok := partMap["text"].(string); ok {
						texts = append(texts, text)
					}
				}
			}
		}
		return strings.Join(texts, "")
	}
	return ""
}

func (s *GatewayService) hashContent(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:16]) // 32字符
}

// replaceModelInBody 替换请求体中的model字段
func (s *GatewayService) replaceModelInBody(body []byte, newModel string) []byte {
	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return body
	}
	req["model"] = newModel
	newBody, err := json.Marshal(req)
	if err != nil {
		return body
	}
	return newBody
}

// SelectAccount 选择账号（粘性会话+优先级）
func (s *GatewayService) SelectAccount(ctx context.Context, groupID *int64, sessionHash string) (*Account, error) {
	return s.SelectAccountForModel(ctx, groupID, sessionHash, "")
}

// SelectAccountForModel 选择支持指定模型的账号（粘性会话+优先级+模型映射）
func (s *GatewayService) SelectAccountForModel(ctx context.Context, groupID *int64, sessionHash string, requestedModel string) (*Account, error) {
	return s.SelectAccountForModelWithExclusions(ctx, groupID, sessionHash, requestedModel, nil)
}

// SelectAccountForModelWithExclusions selects an account supporting the requested model while excluding specified accounts.
func (s *GatewayService) SelectAccountForModelWithExclusions(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{}) (*Account, error) {
	// 1. 查询粘性会话
	if sessionHash != "" {
		accountID, err := s.cache.GetSessionAccountID(ctx, sessionHash)
		if err == nil && accountID > 0 {
			if _, excluded := excludedIDs[accountID]; !excluded {
				account, err := s.accountRepo.GetByID(ctx, accountID)
				// 使用IsSchedulable代替IsActive，确保限流/过载账号不会被选中
				// 同时检查模型支持
				if err == nil && account.IsSchedulable() && (requestedModel == "" || account.IsModelSupported(requestedModel)) {
					// 续期粘性会话
					if err := s.cache.RefreshSessionTTL(ctx, sessionHash, stickySessionTTL); err != nil {
						log.Printf("refresh session ttl failed: session=%s err=%v", sessionHash, err)
					}
					return account, nil
				}
			}
		}
	}

	// 2. 获取可调度账号列表（排除限流和过载的账号，仅限 Anthropic 平台）
	var accounts []Account
	var err error
	if groupID != nil {
		accounts, err = s.accountRepo.ListSchedulableByGroupIDAndPlatform(ctx, *groupID, PlatformAnthropic)
	} else {
		accounts, err = s.accountRepo.ListSchedulableByPlatform(ctx, PlatformAnthropic)
	}
	if err != nil {
		return nil, fmt.Errorf("query accounts failed: %w", err)
	}

	// 3. 按优先级+最久未用选择（考虑模型支持）
	var selected *Account
	for i := range accounts {
		acc := &accounts[i]
		if _, excluded := excludedIDs[acc.ID]; excluded {
			continue
		}
		// 检查模型支持
		if requestedModel != "" && !acc.IsModelSupported(requestedModel) {
			continue
		}
		if selected == nil {
			selected = acc
			continue
		}
		// 优先选择priority值更小的（priority值越小优先级越高）
		if acc.Priority < selected.Priority {
			selected = acc
		} else if acc.Priority == selected.Priority {
			// 优先级相同时，选最久未用的
			switch {
			case acc.LastUsedAt == nil && selected.LastUsedAt != nil:
				selected = acc
			case acc.LastUsedAt != nil && selected.LastUsedAt == nil:
				// keep selected (never used is preferred)
			case acc.LastUsedAt == nil && selected.LastUsedAt == nil:
				// keep selected (both never used)
			default:
				if acc.LastUsedAt.Before(*selected.LastUsedAt) {
					selected = acc
				}
			}
		}
	}

	if selected == nil {
		if requestedModel != "" {
			return nil, fmt.Errorf("no available accounts supporting model: %s", requestedModel)
		}
		return nil, errors.New("no available accounts")
	}

	// 4. 建立粘性绑定
	if sessionHash != "" {
		if err := s.cache.SetSessionAccountID(ctx, sessionHash, selected.ID, stickySessionTTL); err != nil {
			log.Printf("set session account failed: session=%s account_id=%d err=%v", sessionHash, selected.ID, err)
		}
	}

	return selected, nil
}

// GetAccessToken 获取账号凭证
func (s *GatewayService) GetAccessToken(ctx context.Context, account *Account) (string, string, error) {
	switch account.Type {
	case AccountTypeOAuth, AccountTypeSetupToken:
		// Both oauth and setup-token use OAuth token flow
		return s.getOAuthToken(ctx, account)
	case AccountTypeApiKey:
		apiKey := account.GetCredential("api_key")
		if apiKey == "" {
			return "", "", errors.New("api_key not found in credentials")
		}
		return apiKey, "apikey", nil
	default:
		return "", "", fmt.Errorf("unsupported account type: %s", account.Type)
	}
}

func (s *GatewayService) getOAuthToken(ctx context.Context, account *Account) (string, string, error) {
	accessToken := account.GetCredential("access_token")
	if accessToken == "" {
		return "", "", errors.New("access_token not found in credentials")
	}
	// Token刷新由后台 TokenRefreshService 处理，此处只返回当前token
	return accessToken, "oauth", nil
}

// 重试相关常量
const (
	maxRetries = 10              // 最大重试次数
	retryDelay = 3 * time.Second // 重试等待时间
)

func (s *GatewayService) shouldRetryUpstreamError(account *Account, statusCode int) bool {
	// OAuth/Setup Token 账号：仅 403 重试
	if account.IsOAuth() {
		return statusCode == 403
	}

	// API Key 账号：未配置的错误码重试
	return !account.ShouldHandleErrorCode(statusCode)
}

// shouldFailoverUpstreamError determines whether an upstream error should trigger account failover.
func (s *GatewayService) shouldFailoverUpstreamError(statusCode int) bool {
	switch statusCode {
	case 401, 403, 429, 529:
		return true
	default:
		return statusCode >= 500
	}
}

// Forward 转发请求到Claude API
func (s *GatewayService) Forward(ctx context.Context, c *gin.Context, account *Account, body []byte) (*ForwardResult, error) {
	startTime := time.Now()

	// 解析请求获取model和stream
	var req struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("parse request: %w", err)
	}

	if !gjson.GetBytes(body, "system").Exists() {
		body, _ = sjson.SetBytes(body, "system", []any{
			map[string]any{
				"type": "text",
				"text": "You are Claude Code, Anthropic's official CLI for Claude.",
				"cache_control": map[string]string{
					"type": "ephemeral",
				},
			},
		})
	}

	// 应用模型映射（仅对apikey类型账号）
	originalModel := req.Model
	if account.Type == AccountTypeApiKey {
		mappedModel := account.GetMappedModel(req.Model)
		if mappedModel != req.Model {
			// 替换请求体中的模型名
			body = s.replaceModelInBody(body, mappedModel)
			req.Model = mappedModel
			log.Printf("Model mapping applied: %s -> %s (account: %s)", originalModel, mappedModel, account.Name)
		}
	}

	// 获取凭证
	token, tokenType, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}

	// 获取代理URL
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	// 重试循环
	var resp *http.Response
	for attempt := 1; attempt <= maxRetries; attempt++ {
		// 构建上游请求（每次重试需要重新构建，因为请求体需要重新读取）
		upstreamReq, err := s.buildUpstreamRequest(ctx, c, account, body, token, tokenType)
		if err != nil {
			return nil, err
		}

		// 发送请求
		resp, err = s.httpUpstream.Do(upstreamReq, proxyURL)
		if err != nil {
			return nil, fmt.Errorf("upstream request failed: %w", err)
		}

		// 检查是否需要重试
		if resp.StatusCode >= 400 && s.shouldRetryUpstreamError(account, resp.StatusCode) {
			if attempt < maxRetries {
				log.Printf("Account %d: upstream error %d, retry %d/%d after %v",
					account.ID, resp.StatusCode, attempt, maxRetries, retryDelay)
				_ = resp.Body.Close()
				time.Sleep(retryDelay)
				continue
			}
			// 最后一次尝试也失败，跳出循环处理重试耗尽
			break
		}

		// 不需要重试（成功或不可重试的错误），跳出循环
		break
	}
	defer func() { _ = resp.Body.Close() }()

	// 处理重试耗尽的情况
	if resp.StatusCode >= 400 && s.shouldRetryUpstreamError(account, resp.StatusCode) {
		if s.shouldFailoverUpstreamError(resp.StatusCode) {
			s.handleRetryExhaustedSideEffects(ctx, resp, account)
			return nil, &UpstreamFailoverError{StatusCode: resp.StatusCode}
		}
		return s.handleRetryExhaustedError(ctx, resp, c, account)
	}

	// 处理可切换账号的错误
	if resp.StatusCode >= 400 && s.shouldFailoverUpstreamError(resp.StatusCode) {
		s.handleFailoverSideEffects(ctx, resp, account)
		return nil, &UpstreamFailoverError{StatusCode: resp.StatusCode}
	}

	// 处理错误响应（不可重试的错误）
	if resp.StatusCode >= 400 {
		return s.handleErrorResponse(ctx, resp, c, account)
	}

	// 处理正常响应
	var usage *ClaudeUsage
	var firstTokenMs *int
	if req.Stream {
		streamResult, err := s.handleStreamingResponse(ctx, resp, c, account, startTime, originalModel, req.Model)
		if err != nil {
			return nil, err
		}
		usage = streamResult.usage
		firstTokenMs = streamResult.firstTokenMs
	} else {
		usage, err = s.handleNonStreamingResponse(ctx, resp, c, account, originalModel, req.Model)
		if err != nil {
			return nil, err
		}
	}

	return &ForwardResult{
		RequestID:    resp.Header.Get("x-request-id"),
		Usage:        *usage,
		Model:        originalModel, // 使用原始模型用于计费和日志
		Stream:       req.Stream,
		Duration:     time.Since(startTime),
		FirstTokenMs: firstTokenMs,
	}, nil
}

func (s *GatewayService) buildUpstreamRequest(ctx context.Context, c *gin.Context, account *Account, body []byte, token, tokenType string) (*http.Request, error) {
	// 确定目标URL
	targetURL := claudeAPIURL
	if account.Type == AccountTypeApiKey {
		baseURL := account.GetBaseURL()
		targetURL = baseURL + "/v1/messages"
	}

	// OAuth账号：应用统一指纹
	var fingerprint *Fingerprint
	if account.IsOAuth() && s.identityService != nil {
		// 1. 获取或创建指纹（包含随机生成的ClientID）
		fp, err := s.identityService.GetOrCreateFingerprint(ctx, account.ID, c.Request.Header)
		if err != nil {
			log.Printf("Warning: failed to get fingerprint for account %d: %v", account.ID, err)
			// 失败时降级为透传原始headers
		} else {
			fingerprint = fp

			// 2. 重写metadata.user_id（需要指纹中的ClientID和账号的account_uuid）
			accountUUID := account.GetExtraString("account_uuid")
			if accountUUID != "" && fp.ClientID != "" {
				if newBody, err := s.identityService.RewriteUserID(body, account.ID, accountUUID, fp.ClientID); err == nil && len(newBody) > 0 {
					body = newBody
				}
			}
		}
	}

	req, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	// 设置认证头
	if tokenType == "oauth" {
		req.Header.Set("authorization", "Bearer "+token)
	} else {
		req.Header.Set("x-api-key", token)
	}

	// 白名单透传headers
	for key, values := range c.Request.Header {
		lowerKey := strings.ToLower(key)
		if allowedHeaders[lowerKey] {
			for _, v := range values {
				req.Header.Add(key, v)
			}
		}
	}

	// OAuth账号：应用缓存的指纹到请求头（覆盖白名单透传的头）
	if fingerprint != nil {
		s.identityService.ApplyFingerprint(req, fingerprint)
	}

	// 确保必要的headers存在
	if req.Header.Get("content-type") == "" {
		req.Header.Set("content-type", "application/json")
	}
	if req.Header.Get("anthropic-version") == "" {
		req.Header.Set("anthropic-version", "2023-06-01")
	}

	// 处理anthropic-beta header（OAuth账号需要特殊处理）
	if tokenType == "oauth" {
		req.Header.Set("anthropic-beta", s.getBetaHeader(body, c.GetHeader("anthropic-beta")))
	}

	return req, nil
}

// getBetaHeader 处理anthropic-beta header
// 对于OAuth账号，需要确保包含oauth-2025-04-20
func (s *GatewayService) getBetaHeader(body []byte, clientBetaHeader string) string {
	// 如果客户端传了anthropic-beta
	if clientBetaHeader != "" {
		// 已包含oauth beta则直接返回
		if strings.Contains(clientBetaHeader, claude.BetaOAuth) {
			return clientBetaHeader
		}

		// 需要添加oauth beta
		parts := strings.Split(clientBetaHeader, ",")
		for i, p := range parts {
			parts[i] = strings.TrimSpace(p)
		}

		// 在claude-code-20250219后面插入oauth beta
		claudeCodeIdx := -1
		for i, p := range parts {
			if p == claude.BetaClaudeCode {
				claudeCodeIdx = i
				break
			}
		}

		if claudeCodeIdx >= 0 {
			// 在claude-code后面插入
			newParts := make([]string, 0, len(parts)+1)
			newParts = append(newParts, parts[:claudeCodeIdx+1]...)
			newParts = append(newParts, claude.BetaOAuth)
			newParts = append(newParts, parts[claudeCodeIdx+1:]...)
			return strings.Join(newParts, ",")
		}

		// 没有claude-code，放在第一位
		return claude.BetaOAuth + "," + clientBetaHeader
	}

	// 客户端没传，根据模型生成
	var modelID string
	var reqMap map[string]any
	if json.Unmarshal(body, &reqMap) == nil {
		if m, ok := reqMap["model"].(string); ok {
			modelID = m
		}
	}

	// haiku模型不需要claude-code beta
	if strings.Contains(strings.ToLower(modelID), "haiku") {
		return claude.HaikuBetaHeader
	}

	return claude.DefaultBetaHeader
}

func (s *GatewayService) handleErrorResponse(ctx context.Context, resp *http.Response, c *gin.Context, account *Account) (*ForwardResult, error) {
	body, _ := io.ReadAll(resp.Body)

	// 处理上游错误，标记账号状态
	s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, body)

	// 根据状态码返回适当的自定义错误响应（不透传上游详细信息）
	var errType, errMsg string
	var statusCode int

	switch resp.StatusCode {
	case 400:
		c.Data(http.StatusBadRequest, "application/json", body)
		return nil, fmt.Errorf("upstream error: %d", resp.StatusCode)
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
	}

	// 返回自定义错误响应
	c.JSON(statusCode, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    errType,
			"message": errMsg,
		},
	})

	return nil, fmt.Errorf("upstream error: %d", resp.StatusCode)
}

func (s *GatewayService) handleRetryExhaustedSideEffects(ctx context.Context, resp *http.Response, account *Account) {
	body, _ := io.ReadAll(resp.Body)
	statusCode := resp.StatusCode

	// OAuth/Setup Token 账号的 403：标记账号异常
	if account.IsOAuth() && statusCode == 403 {
		s.rateLimitService.HandleUpstreamError(ctx, account, statusCode, resp.Header, body)
		log.Printf("Account %d: marked as error after %d retries for status %d", account.ID, maxRetries, statusCode)
	} else {
		// API Key 未配置错误码：不标记账号状态
		log.Printf("Account %d: upstream error %d after %d retries (not marking account)", account.ID, statusCode, maxRetries)
	}
}

func (s *GatewayService) handleFailoverSideEffects(ctx context.Context, resp *http.Response, account *Account) {
	body, _ := io.ReadAll(resp.Body)
	s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, body)
}

// handleRetryExhaustedError 处理重试耗尽后的错误
// OAuth 403：标记账号异常
// API Key 未配置错误码：仅返回错误，不标记账号
func (s *GatewayService) handleRetryExhaustedError(ctx context.Context, resp *http.Response, c *gin.Context, account *Account) (*ForwardResult, error) {
	s.handleRetryExhaustedSideEffects(ctx, resp, account)

	// 返回统一的重试耗尽错误响应
	c.JSON(http.StatusBadGateway, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    "upstream_error",
			"message": "Upstream request failed after retries",
		},
	})

	return nil, fmt.Errorf("upstream error: %d (retries exhausted)", resp.StatusCode)
}

// streamingResult 流式响应结果
type streamingResult struct {
	usage        *ClaudeUsage
	firstTokenMs *int
}

func (s *GatewayService) handleStreamingResponse(ctx context.Context, resp *http.Response, c *gin.Context, account *Account, startTime time.Time, originalModel, mappedModel string) (*streamingResult, error) {
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
	}

	w := c.Writer
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
	}

	usage := &ClaudeUsage{}
	var firstTokenMs *int
	scanner := bufio.NewScanner(resp.Body)
	// 设置更大的buffer以处理长行
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	needModelReplace := originalModel != mappedModel

	for scanner.Scan() {
		line := scanner.Text()

		// Extract data from SSE line (supports both "data: " and "data:" formats)
		if sseDataRe.MatchString(line) {
			data := sseDataRe.ReplaceAllString(line, "")

			// 如果有模型映射，替换响应中的model字段
			if needModelReplace {
				line = s.replaceModelInSSELine(line, mappedModel, originalModel)
			}

			// 转发行
			if _, err := fmt.Fprintf(w, "%s\n", line); err != nil {
				return &streamingResult{usage: usage, firstTokenMs: firstTokenMs}, err
			}
			flusher.Flush()

			// 记录首字时间：第一个有效的 content_block_delta 或 message_start
			if firstTokenMs == nil && data != "" && data != "[DONE]" {
				ms := int(time.Since(startTime).Milliseconds())
				firstTokenMs = &ms
			}
			s.parseSSEUsage(data, usage)
		} else {
			// 非 data 行直接转发
			if _, err := fmt.Fprintf(w, "%s\n", line); err != nil {
				return &streamingResult{usage: usage, firstTokenMs: firstTokenMs}, err
			}
			flusher.Flush()
		}
	}

	if err := scanner.Err(); err != nil {
		return &streamingResult{usage: usage, firstTokenMs: firstTokenMs}, fmt.Errorf("stream read error: %w", err)
	}

	return &streamingResult{usage: usage, firstTokenMs: firstTokenMs}, nil
}

// replaceModelInSSELine 替换SSE数据行中的model字段
func (s *GatewayService) replaceModelInSSELine(line, fromModel, toModel string) string {
	if !sseDataRe.MatchString(line) {
		return line
	}
	data := sseDataRe.ReplaceAllString(line, "")
	if data == "" || data == "[DONE]" {
		return line
	}

	var event map[string]any
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return line
	}

	// 只替换 message_start 事件中的 message.model
	if event["type"] != "message_start" {
		return line
	}

	msg, ok := event["message"].(map[string]any)
	if !ok {
		return line
	}

	model, ok := msg["model"].(string)
	if !ok || model != fromModel {
		return line
	}

	msg["model"] = toModel
	newData, err := json.Marshal(event)
	if err != nil {
		return line
	}

	return "data: " + string(newData)
}

func (s *GatewayService) parseSSEUsage(data string, usage *ClaudeUsage) {
	// 解析message_start获取input tokens（标准Claude API格式）
	var msgStart struct {
		Type    string `json:"type"`
		Message struct {
			Usage ClaudeUsage `json:"usage"`
		} `json:"message"`
	}
	if json.Unmarshal([]byte(data), &msgStart) == nil && msgStart.Type == "message_start" {
		usage.InputTokens = msgStart.Message.Usage.InputTokens
		usage.CacheCreationInputTokens = msgStart.Message.Usage.CacheCreationInputTokens
		usage.CacheReadInputTokens = msgStart.Message.Usage.CacheReadInputTokens
	}

	// 解析message_delta获取tokens（兼容GLM等把所有usage放在delta中的API）
	var msgDelta struct {
		Type  string `json:"type"`
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal([]byte(data), &msgDelta) == nil && msgDelta.Type == "message_delta" {
		// output_tokens 总是从 message_delta 获取
		usage.OutputTokens = msgDelta.Usage.OutputTokens

		// 如果 message_start 中没有值，则从 message_delta 获取（兼容GLM等API）
		if usage.InputTokens == 0 {
			usage.InputTokens = msgDelta.Usage.InputTokens
		}
		if usage.CacheCreationInputTokens == 0 {
			usage.CacheCreationInputTokens = msgDelta.Usage.CacheCreationInputTokens
		}
		if usage.CacheReadInputTokens == 0 {
			usage.CacheReadInputTokens = msgDelta.Usage.CacheReadInputTokens
		}
	}
}

func (s *GatewayService) handleNonStreamingResponse(ctx context.Context, resp *http.Response, c *gin.Context, account *Account, originalModel, mappedModel string) (*ClaudeUsage, error) {
	// 更新5h窗口状态
	s.rateLimitService.UpdateSessionWindow(ctx, account, resp.Header)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// 解析usage
	var response struct {
		Usage ClaudeUsage `json:"usage"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	// 如果有模型映射，替换响应中的model字段
	if originalModel != mappedModel {
		body = s.replaceModelInResponseBody(body, mappedModel, originalModel)
	}

	// 透传响应头
	for key, values := range resp.Header {
		for _, value := range values {
			c.Header(key, value)
		}
	}

	// 写入响应
	c.Data(resp.StatusCode, "application/json", body)

	return &response.Usage, nil
}

// replaceModelInResponseBody 替换响应体中的model字段
func (s *GatewayService) replaceModelInResponseBody(body []byte, fromModel, toModel string) []byte {
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		return body
	}

	model, ok := resp["model"].(string)
	if !ok || model != fromModel {
		return body
	}

	resp["model"] = toModel
	newBody, err := json.Marshal(resp)
	if err != nil {
		return body
	}

	return newBody
}

// RecordUsageInput 记录使用量的输入参数
type RecordUsageInput struct {
	Result       *ForwardResult
	ApiKey       *ApiKey
	User         *User
	Account      *Account
	Subscription *UserSubscription // 可选：订阅信息
}

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
	}

	// 获取费率倍数
	multiplier := s.cfg.Default.RateMultiplier
	if apiKey.GroupID != nil && apiKey.Group != nil {
		multiplier = apiKey.Group.RateMultiplier
	}

	cost, err := s.billingService.CalculateCost(result.Model, tokens, multiplier)
	if err != nil {
		log.Printf("Calculate cost failed: %v", err)
		// 使用默认费用继续
		cost = &CostBreakdown{ActualCost: 0}
	}

	// 判断计费方式：订阅模式 vs 余额模式
	isSubscriptionBilling := subscription != nil && apiKey.Group != nil && apiKey.Group.IsSubscriptionType()
	billingType := BillingTypeBalance
	if isSubscriptionBilling {
		billingType = BillingTypeSubscription
	}

	// 创建使用日志
	durationMs := int(result.Duration.Milliseconds())
	usageLog := &UsageLog{
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
	}

	// 添加分组和订阅关联
	if apiKey.GroupID != nil {
		usageLog.GroupID = apiKey.GroupID
	}
	if subscription != nil {
		usageLog.SubscriptionID = &subscription.ID
	}

	if err := s.usageLogRepo.Create(ctx, usageLog); err != nil {
		log.Printf("Create usage log failed: %v", err)
	}

	// 根据计费类型执行扣费
	if isSubscriptionBilling {
		// 订阅模式：更新订阅用量（使用 TotalCost 原始费用，不考虑倍率）
		if cost.TotalCost > 0 {
			if err := s.userSubRepo.IncrementUsage(ctx, subscription.ID, cost.TotalCost); err != nil {
				log.Printf("Increment subscription usage failed: %v", err)
			}
			// 异步更新订阅缓存
			go func() {
				cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := s.billingCacheService.UpdateSubscriptionUsage(cacheCtx, user.ID, *apiKey.GroupID, cost.TotalCost); err != nil {
					log.Printf("Update subscription cache failed: %v", err)
				}
			}()
		}
	} else {
		// 余额模式：扣除用户余额（使用 ActualCost 考虑倍率后的费用）
		if cost.ActualCost > 0 {
			if err := s.userRepo.DeductBalance(ctx, user.ID, cost.ActualCost); err != nil {
				log.Printf("Deduct balance failed: %v", err)
			}
			// 异步更新余额缓存
			go func() {
				cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := s.billingCacheService.DeductBalanceCache(cacheCtx, user.ID, cost.ActualCost); err != nil {
					log.Printf("Update balance cache failed: %v", err)
				}
			}()
		}
	}

	// 更新账号最后使用时间
	if err := s.accountRepo.UpdateLastUsed(ctx, account.ID); err != nil {
		log.Printf("Update last used failed: %v", err)
	}

	return nil
}

// ForwardCountTokens 转发 count_tokens 请求到上游 API
// 特点：不记录使用量、仅支持非流式响应
func (s *GatewayService) ForwardCountTokens(ctx context.Context, c *gin.Context, account *Account, body []byte) error {
	// 应用模型映射（仅对 apikey 类型账号）
	if account.Type == AccountTypeApiKey {
		var req struct {
			Model string `json:"model"`
		}
		if err := json.Unmarshal(body, &req); err == nil && req.Model != "" {
			mappedModel := account.GetMappedModel(req.Model)
			if mappedModel != req.Model {
				body = s.replaceModelInBody(body, mappedModel)
				log.Printf("CountTokens model mapping applied: %s -> %s (account: %s)", req.Model, mappedModel, account.Name)
			}
		}
	}

	// 获取凭证
	token, tokenType, err := s.GetAccessToken(ctx, account)
	if err != nil {
		s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Failed to get access token")
		return err
	}

	// 构建上游请求
	upstreamReq, err := s.buildCountTokensRequest(ctx, c, account, body, token, tokenType)
	if err != nil {
		s.countTokensError(c, http.StatusInternalServerError, "api_error", "Failed to build request")
		return err
	}

	// 获取代理URL
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	// 发送请求
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL)
	if err != nil {
		s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Request failed")
		return fmt.Errorf("upstream request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	// 读取响应体
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Failed to read response")
		return err
	}

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
		}
		s.countTokensError(c, resp.StatusCode, "upstream_error", errMsg)
		return fmt.Errorf("upstream error: %d", resp.StatusCode)
	}

	// 透传成功响应
	c.Data(resp.StatusCode, "application/json", respBody)
	return nil
}

// buildCountTokensRequest 构建 count_tokens 上游请求
func (s *GatewayService) buildCountTokensRequest(ctx context.Context, c *gin.Context, account *Account, body []byte, token, tokenType string) (*http.Request, error) {
	// 确定目标 URL
	targetURL := claudeAPICountTokensURL
	if account.Type == AccountTypeApiKey {
		baseURL := account.GetBaseURL()
		targetURL = baseURL + "/v1/messages/count_tokens"
	}

	// OAuth 账号：应用统一指纹和重写 userID
	if account.IsOAuth() && s.identityService != nil {
		fp, err := s.identityService.GetOrCreateFingerprint(ctx, account.ID, c.Request.Header)
		if err == nil {
			accountUUID := account.GetExtraString("account_uuid")
			if accountUUID != "" && fp.ClientID != "" {
				if newBody, err := s.identityService.RewriteUserID(body, account.ID, accountUUID, fp.ClientID); err == nil && len(newBody) > 0 {
					body = newBody
				}
			}
		}
	}

	req, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	// 设置认证头
	if tokenType == "oauth" {
		req.Header.Set("authorization", "Bearer "+token)
	} else {
		req.Header.Set("x-api-key", token)
	}

	// 白名单透传 headers
	for key, values := range c.Request.Header {
		lowerKey := strings.ToLower(key)
		if allowedHeaders[lowerKey] {
			for _, v := range values {
				req.Header.Add(key, v)
			}
		}
	}

	// OAuth 账号：应用指纹到请求头
	if account.IsOAuth() && s.identityService != nil {
		fp, _ := s.identityService.GetOrCreateFingerprint(ctx, account.ID, c.Request.Header)
		if fp != nil {
			s.identityService.ApplyFingerprint(req, fp)
		}
	}

	// 确保必要的 headers 存在
	if req.Header.Get("content-type") == "" {
		req.Header.Set("content-type", "application/json")
	}
	if req.Header.Get("anthropic-version") == "" {
		req.Header.Set("anthropic-version", "2023-06-01")
	}

	// OAuth 账号：处理 anthropic-beta header
	if tokenType == "oauth" {
		req.Header.Set("anthropic-beta", s.getBetaHeader(body, c.GetHeader("anthropic-beta")))
	}

	return req, nil
}

// countTokensError 返回 count_tokens 错误响应
func (s *GatewayService) countTokensError(c *gin.Context, status int, errType, message string) {
	c.JSON(status, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
}
