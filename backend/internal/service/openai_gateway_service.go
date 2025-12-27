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
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
)

const (
	// ChatGPT internal API for OAuth accounts
	chatgptCodexURL = "https://chatgpt.com/backend-api/codex/responses"
	// OpenAI Platform API for API Key accounts (fallback)
	openaiPlatformAPIURL   = "https://api.openai.com/v1/responses"
	openaiStickySessionTTL = time.Hour // 粘性会话TTL
)

// openaiSSEDataRe matches SSE data lines with optional whitespace after colon.
// Some upstream APIs return non-standard "data:" without space (should be "data: ").
var openaiSSEDataRe = regexp.MustCompile(`^data:\s*`)

// OpenAI allowed headers whitelist (for non-OAuth accounts)
var openaiAllowedHeaders = map[string]bool{
	"accept-language": true,
	"content-type":    true,
	"user-agent":      true,
	"originator":      true,
	"session_id":      true,
}

// OpenAICodexUsageSnapshot represents Codex API usage limits from response headers
type OpenAICodexUsageSnapshot struct {
	PrimaryUsedPercent          *float64 `json:"primary_used_percent,omitempty"`
	PrimaryResetAfterSeconds    *int     `json:"primary_reset_after_seconds,omitempty"`
	PrimaryWindowMinutes        *int     `json:"primary_window_minutes,omitempty"`
	SecondaryUsedPercent        *float64 `json:"secondary_used_percent,omitempty"`
	SecondaryResetAfterSeconds  *int     `json:"secondary_reset_after_seconds,omitempty"`
	SecondaryWindowMinutes      *int     `json:"secondary_window_minutes,omitempty"`
	PrimaryOverSecondaryPercent *float64 `json:"primary_over_secondary_percent,omitempty"`
	UpdatedAt                   string   `json:"updated_at,omitempty"`
}

// OpenAIUsage represents OpenAI API response usage
type OpenAIUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

// OpenAIForwardResult represents the result of forwarding
type OpenAIForwardResult struct {
	RequestID    string
	Usage        OpenAIUsage
	Model        string
	Stream       bool
	Duration     time.Duration
	FirstTokenMs *int
}

// OpenAIGatewayService handles OpenAI API gateway operations
type OpenAIGatewayService struct {
	accountRepo         AccountRepository
	usageLogRepo        UsageLogRepository
	userRepo            UserRepository
	userSubRepo         UserSubscriptionRepository
	cache               GatewayCache
	cfg                 *config.Config
	billingService      *BillingService
	rateLimitService    *RateLimitService
	billingCacheService *BillingCacheService
	httpUpstream        HTTPUpstream
}

// NewOpenAIGatewayService creates a new OpenAIGatewayService
func NewOpenAIGatewayService(
	accountRepo AccountRepository,
	usageLogRepo UsageLogRepository,
	userRepo UserRepository,
	userSubRepo UserSubscriptionRepository,
	cache GatewayCache,
	cfg *config.Config,
	billingService *BillingService,
	rateLimitService *RateLimitService,
	billingCacheService *BillingCacheService,
	httpUpstream HTTPUpstream,
) *OpenAIGatewayService {
	return &OpenAIGatewayService{
		accountRepo:         accountRepo,
		usageLogRepo:        usageLogRepo,
		userRepo:            userRepo,
		userSubRepo:         userSubRepo,
		cache:               cache,
		cfg:                 cfg,
		billingService:      billingService,
		rateLimitService:    rateLimitService,
		billingCacheService: billingCacheService,
		httpUpstream:        httpUpstream,
	}
}

// GenerateSessionHash generates session hash from header (OpenAI uses session_id header)
func (s *OpenAIGatewayService) GenerateSessionHash(c *gin.Context) string {
	sessionID := c.GetHeader("session_id")
	if sessionID == "" {
		return ""
	}
	hash := sha256.Sum256([]byte(sessionID))
	return hex.EncodeToString(hash[:])
}

// SelectAccount selects an OpenAI account with sticky session support
func (s *OpenAIGatewayService) SelectAccount(ctx context.Context, groupID *int64, sessionHash string) (*Account, error) {
	return s.SelectAccountForModel(ctx, groupID, sessionHash, "")
}

// SelectAccountForModel selects an account supporting the requested model
func (s *OpenAIGatewayService) SelectAccountForModel(ctx context.Context, groupID *int64, sessionHash string, requestedModel string) (*Account, error) {
	return s.SelectAccountForModelWithExclusions(ctx, groupID, sessionHash, requestedModel, nil)
}

// SelectAccountForModelWithExclusions selects an account supporting the requested model while excluding specified accounts.
func (s *OpenAIGatewayService) SelectAccountForModelWithExclusions(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{}) (*Account, error) {
	// 1. Check sticky session
	if sessionHash != "" {
		accountID, err := s.cache.GetSessionAccountID(ctx, "openai:"+sessionHash)
		if err == nil && accountID > 0 {
			if _, excluded := excludedIDs[accountID]; !excluded {
				account, err := s.accountRepo.GetByID(ctx, accountID)
				if err == nil && account.IsSchedulable() && account.IsOpenAI() && (requestedModel == "" || account.IsModelSupported(requestedModel)) {
					// Refresh sticky session TTL
					_ = s.cache.RefreshSessionTTL(ctx, "openai:"+sessionHash, openaiStickySessionTTL)
					return account, nil
				}
			}
		}
	}

	// 2. Get schedulable OpenAI accounts
	var accounts []Account
	var err error
	if groupID != nil {
		accounts, err = s.accountRepo.ListSchedulableByGroupIDAndPlatform(ctx, *groupID, PlatformOpenAI)
	} else {
		accounts, err = s.accountRepo.ListSchedulableByPlatform(ctx, PlatformOpenAI)
	}
	if err != nil {
		return nil, fmt.Errorf("query accounts failed: %w", err)
	}

	// 3. Select by priority + LRU
	var selected *Account
	for i := range accounts {
		acc := &accounts[i]
		if _, excluded := excludedIDs[acc.ID]; excluded {
			continue
		}
		// Check model support
		if requestedModel != "" && !acc.IsModelSupported(requestedModel) {
			continue
		}
		if selected == nil {
			selected = acc
			continue
		}
		// Lower priority value means higher priority
		if acc.Priority < selected.Priority {
			selected = acc
		} else if acc.Priority == selected.Priority {
			switch {
			case acc.LastUsedAt == nil && selected.LastUsedAt != nil:
				selected = acc
			case acc.LastUsedAt != nil && selected.LastUsedAt == nil:
				// keep selected (never used is preferred)
			case acc.LastUsedAt == nil && selected.LastUsedAt == nil:
				// keep selected (both never used)
			default:
				// Same priority, select least recently used
				if acc.LastUsedAt.Before(*selected.LastUsedAt) {
					selected = acc
				}
			}
		}
	}

	if selected == nil {
		if requestedModel != "" {
			return nil, fmt.Errorf("no available OpenAI accounts supporting model: %s", requestedModel)
		}
		return nil, errors.New("no available OpenAI accounts")
	}

	// 4. Set sticky session
	if sessionHash != "" {
		_ = s.cache.SetSessionAccountID(ctx, "openai:"+sessionHash, selected.ID, openaiStickySessionTTL)
	}

	return selected, nil
}

// GetAccessToken gets the access token for an OpenAI account
func (s *OpenAIGatewayService) GetAccessToken(ctx context.Context, account *Account) (string, string, error) {
	switch account.Type {
	case AccountTypeOAuth:
		accessToken := account.GetOpenAIAccessToken()
		if accessToken == "" {
			return "", "", errors.New("access_token not found in credentials")
		}
		return accessToken, "oauth", nil
	case AccountTypeApiKey:
		apiKey := account.GetOpenAIApiKey()
		if apiKey == "" {
			return "", "", errors.New("api_key not found in credentials")
		}
		return apiKey, "apikey", nil
	default:
		return "", "", fmt.Errorf("unsupported account type: %s", account.Type)
	}
}

func (s *OpenAIGatewayService) shouldFailoverUpstreamError(statusCode int) bool {
	switch statusCode {
	case 401, 403, 429, 529:
		return true
	default:
		return statusCode >= 500
	}
}

func (s *OpenAIGatewayService) handleFailoverSideEffects(ctx context.Context, resp *http.Response, account *Account) {
	body, _ := io.ReadAll(resp.Body)
	s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, body)
}

// Forward forwards request to OpenAI API
func (s *OpenAIGatewayService) Forward(ctx context.Context, c *gin.Context, account *Account, body []byte) (*OpenAIForwardResult, error) {
	startTime := time.Now()

	// Parse request body once (avoid multiple parse/serialize cycles)
	var reqBody map[string]any
	if err := json.Unmarshal(body, &reqBody); err != nil {
		return nil, fmt.Errorf("parse request: %w", err)
	}

	// Extract model and stream from parsed body
	reqModel, _ := reqBody["model"].(string)
	reqStream, _ := reqBody["stream"].(bool)

	// Track if body needs re-serialization
	bodyModified := false
	originalModel := reqModel

	// Apply model mapping
	mappedModel := account.GetMappedModel(reqModel)
	if mappedModel != reqModel {
		reqBody["model"] = mappedModel
		bodyModified = true
	}

	// For OAuth accounts using ChatGPT internal API, add store: false
	if account.Type == AccountTypeOAuth {
		reqBody["store"] = false
		bodyModified = true
	}

	// Re-serialize body only if modified
	if bodyModified {
		var err error
		body, err = json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("serialize request body: %w", err)
		}
	}

	// Get access token
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}

	// Build upstream request
	upstreamReq, err := s.buildUpstreamRequest(ctx, c, account, body, token, reqStream)
	if err != nil {
		return nil, err
	}

	// Get proxy URL
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	// Send request
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL)
	if err != nil {
		return nil, fmt.Errorf("upstream request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Handle error response
	if resp.StatusCode >= 400 {
		if s.shouldFailoverUpstreamError(resp.StatusCode) {
			s.handleFailoverSideEffects(ctx, resp, account)
			return nil, &UpstreamFailoverError{StatusCode: resp.StatusCode}
		}
		return s.handleErrorResponse(ctx, resp, c, account)
	}

	// Handle normal response
	var usage *OpenAIUsage
	var firstTokenMs *int
	if reqStream {
		streamResult, err := s.handleStreamingResponse(ctx, resp, c, account, startTime, originalModel, mappedModel)
		if err != nil {
			return nil, err
		}
		usage = streamResult.usage
		firstTokenMs = streamResult.firstTokenMs
	} else {
		usage, err = s.handleNonStreamingResponse(ctx, resp, c, account, originalModel, mappedModel)
		if err != nil {
			return nil, err
		}
	}

	// Extract and save Codex usage snapshot from response headers (for OAuth accounts)
	if account.Type == AccountTypeOAuth {
		if snapshot := extractCodexUsageHeaders(resp.Header); snapshot != nil {
			s.updateCodexUsageSnapshot(ctx, account.ID, snapshot)
		}
	}

	return &OpenAIForwardResult{
		RequestID:    resp.Header.Get("x-request-id"),
		Usage:        *usage,
		Model:        originalModel,
		Stream:       reqStream,
		Duration:     time.Since(startTime),
		FirstTokenMs: firstTokenMs,
	}, nil
}

func (s *OpenAIGatewayService) buildUpstreamRequest(ctx context.Context, c *gin.Context, account *Account, body []byte, token string, isStream bool) (*http.Request, error) {
	// Determine target URL based on account type
	var targetURL string
	switch account.Type {
	case AccountTypeOAuth:
		// OAuth accounts use ChatGPT internal API
		targetURL = chatgptCodexURL
	case AccountTypeApiKey:
		// API Key accounts use Platform API or custom base URL
		baseURL := account.GetOpenAIBaseURL()
		if baseURL != "" {
			targetURL = baseURL + "/v1/responses"
		} else {
			targetURL = openaiPlatformAPIURL
		}
	default:
		targetURL = openaiPlatformAPIURL
	}

	req, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	// Set authentication header
	req.Header.Set("authorization", "Bearer "+token)

	// Set headers specific to OAuth accounts (ChatGPT internal API)
	if account.Type == AccountTypeOAuth {
		// Required: set Host for ChatGPT API (must use req.Host, not Header.Set)
		req.Host = "chatgpt.com"
		// Required: set chatgpt-account-id header
		chatgptAccountID := account.GetChatGPTAccountID()
		if chatgptAccountID != "" {
			req.Header.Set("chatgpt-account-id", chatgptAccountID)
		}
		// Set accept header based on stream mode
		if isStream {
			req.Header.Set("accept", "text/event-stream")
		} else {
			req.Header.Set("accept", "application/json")
		}
	}

	// Whitelist passthrough headers
	for key, values := range c.Request.Header {
		lowerKey := strings.ToLower(key)
		if openaiAllowedHeaders[lowerKey] {
			for _, v := range values {
				req.Header.Add(key, v)
			}
		}
	}

	// Apply custom User-Agent if configured
	customUA := account.GetOpenAIUserAgent()
	if customUA != "" {
		req.Header.Set("user-agent", customUA)
	}

	// Ensure required headers exist
	if req.Header.Get("content-type") == "" {
		req.Header.Set("content-type", "application/json")
	}

	return req, nil
}

func (s *OpenAIGatewayService) handleErrorResponse(ctx context.Context, resp *http.Response, c *gin.Context, account *Account) (*OpenAIForwardResult, error) {
	body, _ := io.ReadAll(resp.Body)

	// Check custom error codes
	if !account.ShouldHandleErrorCode(resp.StatusCode) {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":    "upstream_error",
				"message": "Upstream gateway error",
			},
		})
		return nil, fmt.Errorf("upstream error: %d (not in custom error codes)", resp.StatusCode)
	}

	// Handle upstream error (mark account status)
	s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, body)

	// Return appropriate error response
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
	default:
		statusCode = http.StatusBadGateway
		errType = "upstream_error"
		errMsg = "Upstream request failed"
	}

	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"type":    errType,
			"message": errMsg,
		},
	})

	return nil, fmt.Errorf("upstream error: %d", resp.StatusCode)
}

// openaiStreamingResult streaming response result
type openaiStreamingResult struct {
	usage        *OpenAIUsage
	firstTokenMs *int
}

func (s *OpenAIGatewayService) handleStreamingResponse(ctx context.Context, resp *http.Response, c *gin.Context, account *Account, startTime time.Time, originalModel, mappedModel string) (*openaiStreamingResult, error) {
	// Set SSE response headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	// Pass through other headers
	if v := resp.Header.Get("x-request-id"); v != "" {
		c.Header("x-request-id", v)
	}

	w := c.Writer
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
	}

	usage := &OpenAIUsage{}
	var firstTokenMs *int
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	needModelReplace := originalModel != mappedModel

	for scanner.Scan() {
		line := scanner.Text()

		// Extract data from SSE line (supports both "data: " and "data:" formats)
		if openaiSSEDataRe.MatchString(line) {
			data := openaiSSEDataRe.ReplaceAllString(line, "")

			// Replace model in response if needed
			if needModelReplace {
				line = s.replaceModelInSSELine(line, mappedModel, originalModel)
			}

			// Forward line
			if _, err := fmt.Fprintf(w, "%s\n", line); err != nil {
				return &openaiStreamingResult{usage: usage, firstTokenMs: firstTokenMs}, err
			}
			flusher.Flush()

			// Record first token time
			if firstTokenMs == nil && data != "" && data != "[DONE]" {
				ms := int(time.Since(startTime).Milliseconds())
				firstTokenMs = &ms
			}
			s.parseSSEUsage(data, usage)
		} else {
			// Forward non-data lines as-is
			if _, err := fmt.Fprintf(w, "%s\n", line); err != nil {
				return &openaiStreamingResult{usage: usage, firstTokenMs: firstTokenMs}, err
			}
			flusher.Flush()
		}
	}

	if err := scanner.Err(); err != nil {
		return &openaiStreamingResult{usage: usage, firstTokenMs: firstTokenMs}, fmt.Errorf("stream read error: %w", err)
	}

	return &openaiStreamingResult{usage: usage, firstTokenMs: firstTokenMs}, nil
}

func (s *OpenAIGatewayService) replaceModelInSSELine(line, fromModel, toModel string) string {
	if !openaiSSEDataRe.MatchString(line) {
		return line
	}
	data := openaiSSEDataRe.ReplaceAllString(line, "")
	if data == "" || data == "[DONE]" {
		return line
	}

	var event map[string]any
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return line
	}

	// Replace model in response
	if m, ok := event["model"].(string); ok && m == fromModel {
		event["model"] = toModel
		newData, err := json.Marshal(event)
		if err != nil {
			return line
		}
		return "data: " + string(newData)
	}

	// Check nested response
	if response, ok := event["response"].(map[string]any); ok {
		if m, ok := response["model"].(string); ok && m == fromModel {
			response["model"] = toModel
			newData, err := json.Marshal(event)
			if err != nil {
				return line
			}
			return "data: " + string(newData)
		}
	}

	return line
}

func (s *OpenAIGatewayService) parseSSEUsage(data string, usage *OpenAIUsage) {
	// Parse response.completed event for usage (OpenAI Responses format)
	var event struct {
		Type     string `json:"type"`
		Response struct {
			Usage struct {
				InputTokens       int `json:"input_tokens"`
				OutputTokens      int `json:"output_tokens"`
				InputTokenDetails struct {
					CachedTokens int `json:"cached_tokens"`
				} `json:"input_tokens_details"`
			} `json:"usage"`
		} `json:"response"`
	}

	if json.Unmarshal([]byte(data), &event) == nil && event.Type == "response.completed" {
		usage.InputTokens = event.Response.Usage.InputTokens
		usage.OutputTokens = event.Response.Usage.OutputTokens
		usage.CacheReadInputTokens = event.Response.Usage.InputTokenDetails.CachedTokens
	}
}

func (s *OpenAIGatewayService) handleNonStreamingResponse(ctx context.Context, resp *http.Response, c *gin.Context, account *Account, originalModel, mappedModel string) (*OpenAIUsage, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Parse usage
	var response struct {
		Usage struct {
			InputTokens       int `json:"input_tokens"`
			OutputTokens      int `json:"output_tokens"`
			InputTokenDetails struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"input_tokens_details"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	usage := &OpenAIUsage{
		InputTokens:          response.Usage.InputTokens,
		OutputTokens:         response.Usage.OutputTokens,
		CacheReadInputTokens: response.Usage.InputTokenDetails.CachedTokens,
	}

	// Replace model in response if needed
	if originalModel != mappedModel {
		body = s.replaceModelInResponseBody(body, mappedModel, originalModel)
	}

	// Pass through headers
	for key, values := range resp.Header {
		for _, value := range values {
			c.Header(key, value)
		}
	}

	c.Data(resp.StatusCode, "application/json", body)

	return usage, nil
}

func (s *OpenAIGatewayService) replaceModelInResponseBody(body []byte, fromModel, toModel string) []byte {
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

// OpenAIRecordUsageInput input for recording usage
type OpenAIRecordUsageInput struct {
	Result       *OpenAIForwardResult
	ApiKey       *ApiKey
	User         *User
	Account      *Account
	Subscription *UserSubscription
}

// RecordUsage records usage and deducts balance
func (s *OpenAIGatewayService) RecordUsage(ctx context.Context, input *OpenAIRecordUsageInput) error {
	result := input.Result
	apiKey := input.ApiKey
	user := input.User
	account := input.Account
	subscription := input.Subscription

	// 计算实际的新输入token（减去缓存读取的token）
	// 因为 input_tokens 包含了 cache_read_tokens，而缓存读取的token不应按输入价格计费
	actualInputTokens := result.Usage.InputTokens - result.Usage.CacheReadInputTokens
	if actualInputTokens < 0 {
		actualInputTokens = 0
	}

	// Calculate cost
	tokens := UsageTokens{
		InputTokens:         actualInputTokens,
		OutputTokens:        result.Usage.OutputTokens,
		CacheCreationTokens: result.Usage.CacheCreationInputTokens,
		CacheReadTokens:     result.Usage.CacheReadInputTokens,
	}

	// Get rate multiplier
	multiplier := s.cfg.Default.RateMultiplier
	if apiKey.GroupID != nil && apiKey.Group != nil {
		multiplier = apiKey.Group.RateMultiplier
	}

	cost, err := s.billingService.CalculateCost(result.Model, tokens, multiplier)
	if err != nil {
		cost = &CostBreakdown{ActualCost: 0}
	}

	// Determine billing type
	isSubscriptionBilling := subscription != nil && apiKey.Group != nil && apiKey.Group.IsSubscriptionType()
	billingType := BillingTypeBalance
	if isSubscriptionBilling {
		billingType = BillingTypeSubscription
	}

	// Create usage log
	durationMs := int(result.Duration.Milliseconds())
	usageLog := &UsageLog{
		UserID:              user.ID,
		ApiKeyID:            apiKey.ID,
		AccountID:           account.ID,
		RequestID:           result.RequestID,
		Model:               result.Model,
		InputTokens:         actualInputTokens,
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

	if apiKey.GroupID != nil {
		usageLog.GroupID = apiKey.GroupID
	}
	if subscription != nil {
		usageLog.SubscriptionID = &subscription.ID
	}

	_ = s.usageLogRepo.Create(ctx, usageLog)

	// Deduct based on billing type
	if isSubscriptionBilling {
		if cost.TotalCost > 0 {
			_ = s.userSubRepo.IncrementUsage(ctx, subscription.ID, cost.TotalCost)
			go func() {
				cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = s.billingCacheService.UpdateSubscriptionUsage(cacheCtx, user.ID, *apiKey.GroupID, cost.TotalCost)
			}()
		}
	} else {
		if cost.ActualCost > 0 {
			_ = s.userRepo.DeductBalance(ctx, user.ID, cost.ActualCost)
			go func() {
				cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = s.billingCacheService.DeductBalanceCache(cacheCtx, user.ID, cost.ActualCost)
			}()
		}
	}

	// Update account last used
	_ = s.accountRepo.UpdateLastUsed(ctx, account.ID)

	return nil
}

// extractCodexUsageHeaders extracts Codex usage limits from response headers
func extractCodexUsageHeaders(headers http.Header) *OpenAICodexUsageSnapshot {
	snapshot := &OpenAICodexUsageSnapshot{}
	hasData := false

	// Helper to parse float64 from header
	parseFloat := func(key string) *float64 {
		if v := headers.Get(key); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				return &f
			}
		}
		return nil
	}

	// Helper to parse int from header
	parseInt := func(key string) *int {
		if v := headers.Get(key); v != "" {
			if i, err := strconv.Atoi(v); err == nil {
				return &i
			}
		}
		return nil
	}

	// Primary (weekly) limits
	if v := parseFloat("x-codex-primary-used-percent"); v != nil {
		snapshot.PrimaryUsedPercent = v
		hasData = true
	}
	if v := parseInt("x-codex-primary-reset-after-seconds"); v != nil {
		snapshot.PrimaryResetAfterSeconds = v
		hasData = true
	}
	if v := parseInt("x-codex-primary-window-minutes"); v != nil {
		snapshot.PrimaryWindowMinutes = v
		hasData = true
	}

	// Secondary (5h) limits
	if v := parseFloat("x-codex-secondary-used-percent"); v != nil {
		snapshot.SecondaryUsedPercent = v
		hasData = true
	}
	if v := parseInt("x-codex-secondary-reset-after-seconds"); v != nil {
		snapshot.SecondaryResetAfterSeconds = v
		hasData = true
	}
	if v := parseInt("x-codex-secondary-window-minutes"); v != nil {
		snapshot.SecondaryWindowMinutes = v
		hasData = true
	}

	// Overflow ratio
	if v := parseFloat("x-codex-primary-over-secondary-limit-percent"); v != nil {
		snapshot.PrimaryOverSecondaryPercent = v
		hasData = true
	}

	if !hasData {
		return nil
	}

	snapshot.UpdatedAt = time.Now().Format(time.RFC3339)
	return snapshot
}

// updateCodexUsageSnapshot saves the Codex usage snapshot to account's Extra field
func (s *OpenAIGatewayService) updateCodexUsageSnapshot(ctx context.Context, accountID int64, snapshot *OpenAICodexUsageSnapshot) {
	if snapshot == nil {
		return
	}

	// Convert snapshot to map for merging into Extra
	updates := make(map[string]any)
	if snapshot.PrimaryUsedPercent != nil {
		updates["codex_primary_used_percent"] = *snapshot.PrimaryUsedPercent
	}
	if snapshot.PrimaryResetAfterSeconds != nil {
		updates["codex_primary_reset_after_seconds"] = *snapshot.PrimaryResetAfterSeconds
	}
	if snapshot.PrimaryWindowMinutes != nil {
		updates["codex_primary_window_minutes"] = *snapshot.PrimaryWindowMinutes
	}
	if snapshot.SecondaryUsedPercent != nil {
		updates["codex_secondary_used_percent"] = *snapshot.SecondaryUsedPercent
	}
	if snapshot.SecondaryResetAfterSeconds != nil {
		updates["codex_secondary_reset_after_seconds"] = *snapshot.SecondaryResetAfterSeconds
	}
	if snapshot.SecondaryWindowMinutes != nil {
		updates["codex_secondary_window_minutes"] = *snapshot.SecondaryWindowMinutes
	}
	if snapshot.PrimaryOverSecondaryPercent != nil {
		updates["codex_primary_over_secondary_percent"] = *snapshot.PrimaryOverSecondaryPercent
	}
	updates["codex_usage_updated_at"] = snapshot.UpdatedAt

	// Normalize to canonical 5h/7d fields based on window_minutes
	// This fixes the issue where OpenAI's primary/secondary naming is reversed
	// Strategy: Compare the two windows and assign the smaller one to 5h, larger one to 7d

	// IMPORTANT: We can only reliably determine window type from window_minutes field
	// The reset_after_seconds is remaining time, not window size, so it cannot be used for comparison

	var primaryWindowMins, secondaryWindowMins int
	var hasPrimaryWindow, hasSecondaryWindow bool

	// Only use window_minutes for reliable window size comparison
	if snapshot.PrimaryWindowMinutes != nil {
		primaryWindowMins = *snapshot.PrimaryWindowMinutes
		hasPrimaryWindow = true
	}

	if snapshot.SecondaryWindowMinutes != nil {
		secondaryWindowMins = *snapshot.SecondaryWindowMinutes
		hasSecondaryWindow = true
	}

	// Determine which is 5h and which is 7d
	var use5hFromPrimary, use7dFromPrimary bool
	var use5hFromSecondary, use7dFromSecondary bool

	if hasPrimaryWindow && hasSecondaryWindow {
		// Both window sizes known: compare and assign smaller to 5h, larger to 7d
		if primaryWindowMins < secondaryWindowMins {
			use5hFromPrimary = true
			use7dFromSecondary = true
		} else {
			use5hFromSecondary = true
			use7dFromPrimary = true
		}
	} else if hasPrimaryWindow {
		// Only primary window size known: classify by absolute threshold
		if primaryWindowMins <= 360 {
			use5hFromPrimary = true
		} else {
			use7dFromPrimary = true
		}
	} else if hasSecondaryWindow {
		// Only secondary window size known: classify by absolute threshold
		if secondaryWindowMins <= 360 {
			use5hFromSecondary = true
		} else {
			use7dFromSecondary = true
		}
	} else {
		// No window_minutes available: cannot reliably determine window types
		// Fall back to legacy assumption (may be incorrect)
		// Assume primary=7d, secondary=5h based on historical observation
		if snapshot.SecondaryUsedPercent != nil || snapshot.SecondaryResetAfterSeconds != nil || snapshot.SecondaryWindowMinutes != nil {
			use5hFromSecondary = true
		}
		if snapshot.PrimaryUsedPercent != nil || snapshot.PrimaryResetAfterSeconds != nil || snapshot.PrimaryWindowMinutes != nil {
			use7dFromPrimary = true
		}
	}

	// Write canonical 5h fields
	if use5hFromPrimary {
		if snapshot.PrimaryUsedPercent != nil {
			updates["codex_5h_used_percent"] = *snapshot.PrimaryUsedPercent
		}
		if snapshot.PrimaryResetAfterSeconds != nil {
			updates["codex_5h_reset_after_seconds"] = *snapshot.PrimaryResetAfterSeconds
		}
		if snapshot.PrimaryWindowMinutes != nil {
			updates["codex_5h_window_minutes"] = *snapshot.PrimaryWindowMinutes
		}
	} else if use5hFromSecondary {
		if snapshot.SecondaryUsedPercent != nil {
			updates["codex_5h_used_percent"] = *snapshot.SecondaryUsedPercent
		}
		if snapshot.SecondaryResetAfterSeconds != nil {
			updates["codex_5h_reset_after_seconds"] = *snapshot.SecondaryResetAfterSeconds
		}
		if snapshot.SecondaryWindowMinutes != nil {
			updates["codex_5h_window_minutes"] = *snapshot.SecondaryWindowMinutes
		}
	}

	// Write canonical 7d fields
	if use7dFromPrimary {
		if snapshot.PrimaryUsedPercent != nil {
			updates["codex_7d_used_percent"] = *snapshot.PrimaryUsedPercent
		}
		if snapshot.PrimaryResetAfterSeconds != nil {
			updates["codex_7d_reset_after_seconds"] = *snapshot.PrimaryResetAfterSeconds
		}
		if snapshot.PrimaryWindowMinutes != nil {
			updates["codex_7d_window_minutes"] = *snapshot.PrimaryWindowMinutes
		}
	} else if use7dFromSecondary {
		if snapshot.SecondaryUsedPercent != nil {
			updates["codex_7d_used_percent"] = *snapshot.SecondaryUsedPercent
		}
		if snapshot.SecondaryResetAfterSeconds != nil {
			updates["codex_7d_reset_after_seconds"] = *snapshot.SecondaryResetAfterSeconds
		}
		if snapshot.SecondaryWindowMinutes != nil {
			updates["codex_7d_window_minutes"] = *snapshot.SecondaryWindowMinutes
		}
	}

	// Update account's Extra field asynchronously
	go func() {
		updateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.accountRepo.UpdateExtra(updateCtx, accountID, updates)
	}()
}
