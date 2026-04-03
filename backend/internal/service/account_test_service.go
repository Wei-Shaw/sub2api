package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/util/soraerror"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// sseDataPrefix matches SSE data lines with optional whitespace after colon.
// Some upstream APIs return non-standard "data:" without space (should be "data: ").
var sseDataPrefix = regexp.MustCompile(`^data:\s*`)

const (
	testClaudeAPIURL   = "https://api.anthropic.com/v1/messages?beta=true"
	chatgptCodexAPIURL = "https://chatgpt.com/backend-api/codex/responses"
	soraMeAPIURL       = "https://sora.chatgpt.com/backend/me" // Sora 用户信息接口，用于测试连接
	soraBillingAPIURL  = "https://sora.chatgpt.com/backend/billing/subscriptions"
	soraInviteMineURL  = "https://sora.chatgpt.com/backend/project_y/invite/mine"
	soraBootstrapURL   = "https://sora.chatgpt.com/backend/m/bootstrap"
	soraRemainingURL   = "https://sora.chatgpt.com/backend/nf/check"
)

// TestEvent represents a SSE event for account testing
type TestEvent struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Model    string `json:"model,omitempty"`
	Status   string `json:"status,omitempty"`
	Code     string `json:"code,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
	Data     any    `json:"data,omitempty"`
	Success  bool   `json:"success,omitempty"`
	Error    string `json:"error,omitempty"`
}

const (
	defaultGeminiTextTestPrompt  = "hi"
	defaultGeminiImageTestPrompt = "Generate a cute orange cat astronaut sticker on a clean pastel background."
)

// AccountTestService handles account testing operations
type AccountTestService struct {
	accountRepo               AccountRepository
	geminiTokenProvider       *GeminiTokenProvider
	antigravityGatewayService *AntigravityGatewayService
	httpUpstream              HTTPUpstream
	cfg                       *config.Config
	tlsFPProfileService       *TLSFingerprintProfileService
	soraTestGuardMu           sync.Mutex
	soraTestLastRun           map[int64]time.Time
	soraTestCooldown          time.Duration
}

const defaultSoraTestCooldown = 10 * time.Second

// NewAccountTestService creates a new AccountTestService
func NewAccountTestService(
	accountRepo AccountRepository,
	geminiTokenProvider *GeminiTokenProvider,
	antigravityGatewayService *AntigravityGatewayService,
	httpUpstream HTTPUpstream,
	cfg *config.Config,
	tlsFPProfileService *TLSFingerprintProfileService,
) *AccountTestService {
	return &AccountTestService{
		accountRepo:               accountRepo,
		geminiTokenProvider:       geminiTokenProvider,
		antigravityGatewayService: antigravityGatewayService,
		httpUpstream:              httpUpstream,
		cfg:                       cfg,
		tlsFPProfileService:       tlsFPProfileService,
		soraTestLastRun:           make(map[int64]time.Time),
		soraTestCooldown:          defaultSoraTestCooldown,
	}
}

func (s *AccountTestService) validateUpstreamBaseURL(raw string) (string, error) {
	if s.cfg == nil {
		return "", errors.New("config is not available")
	}
	if !s.cfg.Security.URLAllowlist.Enabled {
		return urlvalidator.ValidateURLFormat(raw, s.cfg.Security.URLAllowlist.AllowInsecureHTTP)
	}
	normalized, err := urlvalidator.ValidateHTTPSURL(raw, urlvalidator.ValidationOptions{
		AllowedHosts:     s.cfg.Security.URLAllowlist.UpstreamHosts,
		RequireAllowlist: true,
		AllowPrivate:     s.cfg.Security.URLAllowlist.AllowPrivateHosts,
	})
	if err != nil {
		return "", err
	}
	return normalized, nil
}

// generateSessionString generates a Claude Code style session string.
// The output format is determined by the UA version in claude.DefaultHeaders,
// ensuring consistency between the user_id format and the UA sent to upstream.
func generateSessionString() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	hex64 := hex.EncodeToString(b)
	sessionUUID := uuid.New().String()
	uaVersion := ExtractCLIVersion(claude.DefaultHeaders["User-Agent"])
	return FormatMetadataUserID(hex64, "", sessionUUID, uaVersion), nil
}

// createTestPayload creates a Claude Code style test request payload
func createTestPayload(modelID string) (map[string]any, error) {
	sessionID, err := generateSessionString()
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"model": modelID,
		"messages": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type": "text",
						"text": "hi",
						"cache_control": map[string]string{
							"type": "ephemeral",
						},
					},
				},
			},
		},
		"system": []map[string]any{
			{
				"type": "text",
				"text": claudeCodeSystemPrompt,
				"cache_control": map[string]string{
					"type": "ephemeral",
				},
			},
		},
		"metadata": map[string]string{
			"user_id": sessionID,
		},
		"max_tokens":  1024,
		"temperature": 1,
		"stream":      true,
	}, nil
}

// TestAccountConnection tests an account's connection by sending a test request
// All account types use full Claude Code client characteristics, only auth header differs
// modelID is optional - if empty, defaults to claude.DefaultTestModel
func (s *AccountTestService) TestAccountConnection(c *gin.Context, accountID int64, modelID string, prompt string) error {
	ctx := c.Request.Context()

	// Get account
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return s.sendErrorAndEnd(c, "Account not found")
	}

	// Route to platform-specific test method
	if account.IsOpenAI() {
		return s.testOpenAIAccountConnection(c, account, modelID)
	}

	if account.IsGemini() {
		return s.testGeminiAccountConnection(c, account, modelID, prompt)
	}

	if account.Platform == PlatformAntigravity {
		return s.routeAntigravityTest(c, account, modelID, prompt)
	}

	if account.Platform == PlatformSora {
		return s.testSoraAccountConnection(c, account)
	}

	return s.testClaudeAccountConnection(c, account, modelID)
}

// testClaudeAccountConnection tests an Anthropic Claude account's connection
func (s *AccountTestService) testClaudeAccountConnection(c *gin.Context, account *Account, modelID string) error {
	ctx := c.Request.Context()

	// Determine the model to use
	testModelID := modelID
	if testModelID == "" {
		testModelID = claude.DefaultTestModel
	}

	// API Key 账号测试连接时也需要应用通配符模型映射。
	if account.Type == "apikey" {
		testModelID = account.GetMappedModel(testModelID)
	}

	// Bedrock accounts use a separate test path
	if account.IsBedrock() {
		return s.testBedrockAccountConnection(c, ctx, account, testModelID)
	}

	// Determine authentication method and API URL
	var authToken string
	var useBearer bool
	var apiURL string

	if account.IsOAuth() {
		// OAuth or Setup Token - use Bearer token
		useBearer = true
		apiURL = testClaudeAPIURL
		authToken = account.GetCredential("access_token")
		if authToken == "" {
			return s.sendErrorAndEnd(c, "No access token available")
		}
	} else if account.Type == "apikey" {
		// API Key - use x-api-key header
		useBearer = false
		authToken = account.GetCredential("api_key")
		if authToken == "" {
			return s.sendErrorAndEnd(c, "No API key available")
		}

		baseURL := account.GetBaseURL()
		if baseURL == "" {
			baseURL = "https://api.anthropic.com"
		}
		normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
		if err != nil {
			return s.sendErrorAndEnd(c, fmt.Sprintf("Invalid base URL: %s", err.Error()))
		}
		apiURL = strings.TrimSuffix(normalizedBaseURL, "/") + "/v1/messages?beta=true"
	} else {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Unsupported account type: %s", account.Type))
	}

	// Set SSE headers
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	// Create Claude Code style payload (same for all account types)
	payload, err := createTestPayload(testModelID)
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create test payload")
	}
	payloadBytes, _ := json.Marshal(payload)

	// Send test_start event
	s.sendEvent(c, TestEvent{Type: "test_start", Model: testModelID})

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create request")
	}

	// Set common headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")

	// Apply Claude Code client headers
	for key, value := range claude.DefaultHeaders {
		req.Header.Set(key, value)
	}

	// Set authentication header
	if useBearer {
		req.Header.Set("anthropic-beta", claude.DefaultBetaHeader)
		req.Header.Set("Authorization", "Bearer "+authToken)
	} else {
		req.Header.Set("anthropic-beta", claude.APIKeyBetaHeader)
		req.Header.Set("x-api-key", authToken)
	}

	// Get proxy URL
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Request failed: %s", err.Error()))
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		errMsg := fmt.Sprintf("API returned %d: %s", resp.StatusCode, string(body))

		// 403 表示账号被上游封禁，标记为 error 状态
		if resp.StatusCode == http.StatusForbidden {
			_ = s.accountRepo.SetError(ctx, account.ID, errMsg)
		}

		return s.sendErrorAndEnd(c, errMsg)
	}

	// Process SSE stream
	return s.processClaudeStream(c, resp.Body)
}

// testBedrockAccountConnection tests a Bedrock (SigV4 or API Key) account using non-streaming invoke
func (s *AccountTestService) testBedrockAccountConnection(c *gin.Context, ctx context.Context, account *Account, testModelID string) error {
	region := bedrockRuntimeRegion(account)
	resolvedModelID, ok := ResolveBedrockModelID(account, testModelID)
	if !ok {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Unsupported Bedrock model: %s", testModelID))
	}
	testModelID = resolvedModelID

	// Set SSE headers (test UI expects SSE)
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	// Create a minimal Bedrock-compatible payload (no stream, no cache_control)
	bedrockPayload := map[string]any{
		"anthropic_version": "bedrock-2023-05-31",
		"messages": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type": "text",
						"text": "hi",
					},
				},
			},
		},
		"max_tokens":  256,
		"temperature": 1,
	}
	bedrockBody, _ := json.Marshal(bedrockPayload)

	// Use non-streaming endpoint (response is standard Claude JSON)
	apiURL := BuildBedrockURL(region, testModelID, false)

	s.sendEvent(c, TestEvent{Type: "test_start", Model: testModelID})

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(bedrockBody))
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create request")
	}
	req.Header.Set("Content-Type", "application/json")

	// Sign or set auth based on account type
	if account.IsBedrockAPIKey() {
		apiKey := account.GetCredential("api_key")
		if apiKey == "" {
			return s.sendErrorAndEnd(c, "No API key available")
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
	} else {
		signer, err := NewBedrockSignerFromAccount(account)
		if err != nil {
			return s.sendErrorAndEnd(c, fmt.Sprintf("Failed to create Bedrock signer: %s", err.Error()))
		}
		if err := signer.SignRequest(ctx, req, bedrockBody); err != nil {
			return s.sendErrorAndEnd(c, fmt.Sprintf("Failed to sign request: %s", err.Error()))
		}
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, nil)
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Request failed: %s", err.Error()))
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return s.sendErrorAndEnd(c, fmt.Sprintf("API returned %d: %s", resp.StatusCode, string(body)))
	}

	// Bedrock non-streaming response is standard Claude JSON, extract the text
	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Failed to parse response: %s", err.Error()))
	}

	text := ""
	if len(result.Content) > 0 {
		text = result.Content[0].Text
	}
	if text == "" {
		text = "(empty response)"
	}

	s.sendEvent(c, TestEvent{Type: "content", Text: text})
	s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
	return nil
}

// testOpenAIAccountConnection tests an OpenAI account's connection
func (s *AccountTestService) testOpenAIAccountConnection(c *gin.Context, account *Account, modelID string) error {
	ctx := c.Request.Context()

	// Default to openai.DefaultTestModel for OpenAI testing
	testModelID := modelID
	if testModelID == "" {
		testModelID = openai.DefaultTestModel
	}

	// For API Key accounts with model mapping, map the model
	if account.Type == "apikey" {
		mapping := account.GetModelMapping()
		if len(mapping) > 0 {
			if mappedModel, exists := mapping[testModelID]; exists {
				testModelID = mappedModel
			}
		}
	}

	// Determine authentication method and API URL
	var authToken string
	var apiURL string
	var isOAuth bool
	var chatgptAccountID string

	if account.IsOAuth() {
		isOAuth = true
		// OAuth - use Bearer token with ChatGPT internal API
		authToken = account.GetOpenAIAccessToken()
		if authToken == "" {
			return s.sendErrorAndEnd(c, "No access token available")
		}

		// OAuth uses ChatGPT internal API
		apiURL = chatgptCodexAPIURL
		chatgptAccountID = account.GetChatGPTAccountID()
	} else if account.Type == "apikey" {
		// API Key - use Platform API
		authToken = account.GetOpenAIApiKey()
		if authToken == "" {
			return s.sendErrorAndEnd(c, "No API key available")
		}

		baseURL := account.GetOpenAIBaseURL()
		if baseURL == "" {
			baseURL = "https://api.openai.com"
		}
		normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
		if err != nil {
			return s.sendErrorAndEnd(c, fmt.Sprintf("Invalid base URL: %s", err.Error()))
		}
		apiURL = strings.TrimSuffix(normalizedBaseURL, "/") + "/responses"
	} else {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Unsupported account type: %s", account.Type))
	}

	// Set SSE headers
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	// Create OpenAI Responses API payload
	payload := createOpenAITestPayload(testModelID, isOAuth)
	payloadBytes, _ := json.Marshal(payload)

	// Send test_start event
	s.sendEvent(c, TestEvent{Type: "test_start", Model: testModelID})

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create request")
	}

	// Set common headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+authToken)

	// Set OAuth-specific headers for ChatGPT internal API
	if isOAuth {
		req.Host = "chatgpt.com"
		req.Header.Set("accept", "text/event-stream")
		if chatgptAccountID != "" {
			req.Header.Set("chatgpt-account-id", chatgptAccountID)
		}
	}

	// Get proxy URL
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Request failed: %s", err.Error()))
	}
	defer func() { _ = resp.Body.Close() }()

	if isOAuth && s.accountRepo != nil {
		if updates, err := extractOpenAICodexProbeUpdates(resp); err == nil && len(updates) > 0 {
			_ = s.accountRepo.UpdateExtra(ctx, account.ID, updates)
			mergeAccountExtra(account, updates)
		}
		if snapshot := ParseCodexRateLimitHeaders(resp.Header); snapshot != nil {
			if resetAt := codexRateLimitResetAtFromSnapshot(snapshot, time.Now()); resetAt != nil {
				_ = s.accountRepo.SetRateLimited(ctx, account.ID, *resetAt)
				account.RateLimitResetAt = resetAt
			}
		}
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if isOAuth && s.accountRepo != nil {
			if resetAt := (&RateLimitService{}).calculateOpenAI429ResetTime(resp.Header); resetAt != nil {
				_ = s.accountRepo.SetRateLimited(ctx, account.ID, *resetAt)
				account.RateLimitResetAt = resetAt
			}
		}
		// 401 Unauthorized: 标记账号为永久错误
		if resp.StatusCode == http.StatusUnauthorized && s.accountRepo != nil {
			errMsg := fmt.Sprintf("Authentication failed (401): %s", string(body))
			_ = s.accountRepo.SetError(ctx, account.ID, errMsg)
		}
		return s.sendErrorAndEnd(c, fmt.Sprintf("API returned %d: %s", resp.StatusCode, string(body)))
	}

	// Process SSE stream
	return s.processOpenAIStream(c, resp.Body)
}

// testGeminiAccountConnection tests a Gemini account's connection
func (s *AccountTestService) testGeminiAccountConnection(c *gin.Context, account *Account, modelID string, prompt string) error {
	ctx := c.Request.Context()

	// Determine the model to use
	testModelID := modelID
	if testModelID == "" {
		testModelID = geminicli.DefaultTestModel
	}

	// For API Key accounts with model mapping, map the model
	if account.Type == AccountTypeAPIKey {
		mapping := account.GetModelMapping()
		if len(mapping) > 0 {
			if mappedModel, exists := mapping[testModelID]; exists {
				testModelID = mappedModel
			}
		}
	}

	// Set SSE headers
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	// Create test payload (Gemini format)
	payload := createGeminiTestPayload(testModelID, prompt)

	// Build request based on account type
	var req *http.Request
	var err error

	switch account.Type {
	case AccountTypeAPIKey:
		req, err = s.buildGeminiAPIKeyRequest(ctx, account, testModelID, payload)
	case AccountTypeOAuth:
		req, err = s.buildGeminiOAuthRequest(ctx, account, testModelID, payload)
	default:
		return s.sendErrorAndEnd(c, fmt.Sprintf("Unsupported account type: %s", account.Type))
	}

	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Failed to build request: %s", err.Error()))
	}

	// Send test_start event
	s.sendEvent(c, TestEvent{Type: "test_start", Model: testModelID})

	// Get proxy and execute request
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Request failed: %s", err.Error()))
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return s.sendErrorAndEnd(c, fmt.Sprintf("API returned %d: %s", resp.StatusCode, string(body)))
	}

	// Process SSE stream
	return s.processGeminiStream(c, resp.Body)
}

type soraProbeStep struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	HTTPStatus int    `json:"http_status,omitempty"`
	ErrorCode  string `json:"error_code,omitempty"`
	Message    string `json:"message,omitempty"`
}

type soraProbeSummary struct {
	Status string          `json:"status"`
	Steps  []soraProbeStep `json:"steps"`
}

type soraProbeRecorder struct {
	steps []soraProbeStep
}

func (r *soraProbeRecorder) addStep(name, status string, httpStatus int, errorCode, message string) {
	r.steps = append(r.steps, soraProbeStep{
		Name:       name,
		Status:     status,
		HTTPStatus: httpStatus,
		ErrorCode:  strings.TrimSpace(errorCode),
		Message:    strings.TrimSpace(message),
	})
}

func (r *soraProbeRecorder) finalize() soraProbeSummary {
	meSuccess := false
	partial := false
	for _, step := range r.steps {
		if step.Name == "me" {
			meSuccess = strings.EqualFold(step.Status, "success")
			continue
		}
		if strings.EqualFold(step.Status, "failed") {
			partial = true
		}
	}

	status := "success"
	if !meSuccess {
		status = "failed"
	} else if partial {
		status = "partial_success"
	}

	return soraProbeSummary{
		Status: status,
		Steps:  append([]soraProbeStep(nil), r.steps...),
	}
}

func (s *AccountTestService) emitSoraProbeSummary(c *gin.Context, rec *soraProbeRecorder) {
	if rec == nil {
		return
	}
	summary := rec.finalize()
	code := ""
	for _, step := range summary.Steps {
		if strings.EqualFold(step.Status, "failed") && strings.TrimSpace(step.ErrorCode) != "" {
			code = step.ErrorCode
			break
		}
	}
	s.sendEvent(c, TestEvent{
		Type:   "sora_test_result",
		Status: summary.Status,
		Code:   code,
		Data:   summary,
	})
}

func (s *AccountTestService) acquireSoraTestPermit(accountID int64) (time.Duration, bool) {
	if accountID <= 0 {
		return 0, true
	}
	s.soraTestGuardMu.Lock()
	defer s.soraTestGuardMu.Unlock()

	if s.soraTestLastRun == nil {
		s.soraTestLastRun = make(map[int64]time.Time)
	}
	cooldown := s.soraTestCooldown
	if cooldown <= 0 {
		cooldown = defaultSoraTestCooldown
	}

	now := time.Now()
	if lastRun, ok := s.soraTestLastRun[accountID]; ok {
		elapsed := now.Sub(lastRun)
		if elapsed < cooldown {
			return cooldown - elapsed, false
		}
	}
	s.soraTestLastRun[accountID] = now
	return 0, true
}

func ceilSeconds(d time.Duration) int {
	if d <= 0 {
		return 1
	}
	sec := int(d / time.Second)
	if d%time.Second != 0 {
		sec++
	}
	if sec < 1 {
		sec = 1
	}
	return sec
}

// testSoraAPIKeyAccountConnection 测试 Sora apikey 类型账号的连通性。
// 向上游 base_url 发送轻量级 prompt-enhance 请求验证连通性和 API Key 有效性。
func (s *AccountTestService) testSoraAPIKeyAccountConnection(c *gin.Context, account *Account) error {
	ctx := c.Request.Context()

	apiKey := account.GetCredential("api_key")
	if apiKey == "" {
		return s.sendErrorAndEnd(c, "Sora apikey 账号缺少 api_key 凭证")
	}

	baseURL := account.GetBaseURL()
	if baseURL == "" {
		return s.sendErrorAndEnd(c, "Sora apikey 账号缺少 base_url")
	}

	// 验证 base_url 格式
	normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("base_url 无效: %s", err.Error()))
	}
	upstreamURL := strings.TrimSuffix(normalizedBaseURL, "/") + "/sora/v1/chat/completions"

	// 设置 SSE 头
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	if wait, ok := s.acquireSoraTestPermit(account.ID); !ok {
		msg := fmt.Sprintf("Sora 账号测试过于频繁，请 %d 秒后重试", ceilSeconds(wait))
		return s.sendErrorAndEnd(c, msg)
	}

	s.sendEvent(c, TestEvent{Type: "test_start", Model: "sora-upstream"})

	// 构建轻量级 prompt-enhance 请求作为连通性测试
	testPayload := map[string]any{
		"model":    "prompt-enhance-short-10s",
		"messages": []map[string]string{{"role": "user", "content": "test"}},
		"stream":   false,
	}
	payloadBytes, _ := json.Marshal(testPayload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return s.sendErrorAndEnd(c, "构建测试请求失败")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	// 获取代理 URL
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("上游连接失败: %s", err.Error()))
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))

	if resp.StatusCode == http.StatusOK {
		s.sendEvent(c, TestEvent{Type: "content", Text: fmt.Sprintf("上游连接成功 (%s)", upstreamURL)})
		s.sendEvent(c, TestEvent{Type: "content", Text: fmt.Sprintf("API Key 有效 (HTTP %d)", resp.StatusCode)})
		s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
		return nil
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return s.sendErrorAndEnd(c, fmt.Sprintf("上游认证失败 (HTTP %d)，请检查 API Key 是否正确", resp.StatusCode))
	}

	// 其他错误但能连通（如 400 参数错误）也算连通性测试通过
	if resp.StatusCode == http.StatusBadRequest {
		s.sendEvent(c, TestEvent{Type: "content", Text: fmt.Sprintf("上游连接成功 (%s)", upstreamURL)})
		s.sendEvent(c, TestEvent{Type: "content", Text: fmt.Sprintf("API Key 有效（上游返回 %d，参数校验错误属正常）", resp.StatusCode)})
		s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
		return nil
	}

	return s.sendErrorAndEnd(c, fmt.Sprintf("上游返回异常 HTTP %d: %s", resp.StatusCode, truncateSoraErrorBody(respBody, 256)))
}

// testSoraAccountConnection 测试 Sora 账号的连接
// OAuth 类型：调用 /backend/me 接口验证 access_token 有效性
// APIKey 类型：向上游 base_url 发送轻量级 prompt-enhance 请求验证连通性
func (s *AccountTestService) testSoraAccountConnection(c *gin.Context, account *Account) error {
	// apikey 类型走独立测试流程
	if account.Type == AccountTypeAPIKey {
		return s.testSoraAPIKeyAccountConnection(c, account)
	}

	ctx := c.Request.Context()
	recorder := &soraProbeRecorder{}

	authToken := account.GetCredential("access_token")
	if authToken == "" {
		recorder.addStep("me", "failed", http.StatusUnauthorized, "missing_access_token", "No access token available")
		s.emitSoraProbeSummary(c, recorder)
		return s.sendErrorAndEnd(c, "No access token available")
	}

	// Set SSE headers
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	if wait, ok := s.acquireSoraTestPermit(account.ID); !ok {
		msg := fmt.Sprintf("Sora 账号测试过于频繁，请 %d 秒后重试", ceilSeconds(wait))
		recorder.addStep("rate_limit", "failed", http.StatusTooManyRequests, "test_rate_limited", msg)
		s.emitSoraProbeSummary(c, recorder)
		return s.sendErrorAndEnd(c, msg)
	}

	// Send test_start event
	s.sendEvent(c, TestEvent{Type: "test_start", Model: "sora"})

	req, err := http.NewRequestWithContext(ctx, "GET", soraMeAPIURL, nil)
	if err != nil {
		recorder.addStep("me", "failed", 0, "request_build_failed", err.Error())
		s.emitSoraProbeSummary(c, recorder)
		return s.sendErrorAndEnd(c, "Failed to create request")
	}

	// 使用 Sora 客户端标准请求头
	req.Header.Set("Authorization", "Bearer "+authToken)
	req.Header.Set("User-Agent", "Sora/1.2026.007 (Android 15; 24122RKC7C; build 2600700)")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Origin", "https://sora.chatgpt.com")
	req.Header.Set("Referer", "https://sora.chatgpt.com/")

	// Get proxy URL
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	soraTLSProfile := s.resolveSoraTLSProfile()

	resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, soraTLSProfile)
	if err != nil {
		recorder.addStep("me", "failed", 0, "network_error", err.Error())
		s.emitSoraProbeSummary(c, recorder)
		return s.sendErrorAndEnd(c, fmt.Sprintf("Request failed: %s", err.Error()))
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		if isCloudflareChallengeResponse(resp.StatusCode, resp.Header, body) {
			recorder.addStep("me", "failed", resp.StatusCode, "cf_challenge", "Cloudflare challenge detected")
			s.emitSoraProbeSummary(c, recorder)
			s.logSoraCloudflareChallenge(account, proxyURL, soraMeAPIURL, resp.Header, body)
			return s.sendErrorAndEnd(c, formatCloudflareChallengeMessage(fmt.Sprintf("Sora request blocked by Cloudflare challenge (HTTP %d). Please switch to a clean proxy/network and retry.", resp.StatusCode), resp.Header, body))
		}
		upstreamCode, upstreamMessage := soraerror.ExtractUpstreamErrorCodeAndMessage(body)
		switch {
		case resp.StatusCode == http.StatusUnauthorized && strings.EqualFold(upstreamCode, "token_invalidated"):
			recorder.addStep("me", "failed", resp.StatusCode, "token_invalidated", "Sora token invalidated")
			s.emitSoraProbeSummary(c, recorder)
			return s.sendErrorAndEnd(c, "Sora token 已失效（token_invalidated），请重新授权账号")
		case strings.EqualFold(upstreamCode, "unsupported_country_code"):
			recorder.addStep("me", "failed", resp.StatusCode, "unsupported_country_code", "Sora is unavailable in current egress region")
			s.emitSoraProbeSummary(c, recorder)
			return s.sendErrorAndEnd(c, "Sora 在当前网络出口地区不可用（unsupported_country_code），请切换到支持地区后重试")
		case strings.TrimSpace(upstreamMessage) != "":
			recorder.addStep("me", "failed", resp.StatusCode, upstreamCode, upstreamMessage)
			s.emitSoraProbeSummary(c, recorder)
			return s.sendErrorAndEnd(c, fmt.Sprintf("Sora API returned %d: %s", resp.StatusCode, upstreamMessage))
		default:
			recorder.addStep("me", "failed", resp.StatusCode, upstreamCode, "Sora me endpoint failed")
			s.emitSoraProbeSummary(c, recorder)
			return s.sendErrorAndEnd(c, fmt.Sprintf("Sora API returned %d: %s", resp.StatusCode, truncateSoraErrorBody(body, 512)))
		}
	}
	recorder.addStep("me", "success", resp.StatusCode, "", "me endpoint ok")

	// 解析 /me 响应，提取用户信息
	var meResp map[string]any
	if err := json.Unmarshal(body, &meResp); err != nil {
		// 能收到 200 就说明 token 有效
		s.sendEvent(c, TestEvent{Type: "content", Text: "Sora connection OK (token valid)"})
	} else {
		// 尝试提取用户名或邮箱信息
		info := "Sora connection OK"
		if name, ok := meResp["name"].(string); ok && name != "" {
			info = fmt.Sprintf("Sora connection OK - User: %s", name)
		} else if email, ok := meResp["email"].(string); ok && email != "" {
			info = fmt.Sprintf("Sora connection OK - Email: %s", email)
		}
		s.sendEvent(c, TestEvent{Type: "content", Text: info})
	}

	// 追加轻量能力检查：订阅信息查询（失败仅告警，不中断连接测试）
	subReq, err := http.NewRequestWithContext(ctx, "GET", soraBillingAPIURL, nil)
	if err == nil {
		subReq.Header.Set("Authorization", "Bearer "+authToken)
		subReq.Header.Set("User-Agent", "Sora/1.2026.007 (Android 15; 24122RKC7C; build 2600700)")
		subReq.Header.Set("Accept", "application/json")
		subReq.Header.Set("Accept-Language", "en-US,en;q=0.9")
		subReq.Header.Set("Origin", "https://sora.chatgpt.com")
		subReq.Header.Set("Referer", "https://sora.chatgpt.com/")

		subResp, subErr := s.httpUpstream.DoWithTLS(subReq, proxyURL, account.ID, account.Concurrency, soraTLSProfile)
		if subErr != nil {
			recorder.addStep("subscription", "failed", 0, "network_error", subErr.Error())
			s.sendEvent(c, TestEvent{Type: "content", Text: fmt.Sprintf("Subscription check skipped: %s", subErr.Error())})
		} else {
			subBody, _ := io.ReadAll(subResp.Body)
			_ = subResp.Body.Close()
			if subResp.StatusCode == http.StatusOK {
				recorder.addStep("subscription", "success", subResp.StatusCode, "", "subscription endpoint ok")
				if summary := parseSoraSubscriptionSummary(subBody); summary != "" {
					s.sendEvent(c, TestEvent{Type: "content", Text: summary})
				} else {
					s.sendEvent(c, TestEvent{Type: "content", Text: "Subscription check OK"})
				}
			} else {
				if isCloudflareChallengeResponse(subResp.StatusCode, subResp.Header, subBody) {
					recorder.addStep("subscription", "failed", subResp.StatusCode, "cf_challenge", "Cloudflare challenge detected")
					s.logSoraCloudflareChallenge(account, proxyURL, soraBillingAPIURL, subResp.Header, subBody)
					s.sendEvent(c, TestEvent{Type: "content", Text: formatCloudflareChallengeMessage(fmt.Sprintf("Subscription check blocked by Cloudflare challenge (HTTP %d)", subResp.StatusCode), subResp.Header, subBody)})
				} else {
					upstreamCode, upstreamMessage := soraerror.ExtractUpstreamErrorCodeAndMessage(subBody)
					recorder.addStep("subscription", "failed", subResp.StatusCode, upstreamCode, upstreamMessage)
					s.sendEvent(c, TestEvent{Type: "content", Text: fmt.Sprintf("Subscription check returned %d", subResp.StatusCode)})
				}
			}
		}
	}

	// 追加 Sora2 能力探测（对齐 sora2api 的测试思路）：邀请码 + 剩余额度。
	s.testSora2Capabilities(c, ctx, account, authToken, proxyURL, soraTLSProfile, recorder)

	s.emitSoraProbeSummary(c, recorder)
	s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
	return nil
}

func (s *AccountTestService) testSora2Capabilities(
	c *gin.Context,
	ctx context.Context,
	account *Account,
	authToken string,
	proxyURL string,
	tlsProfile *tlsfingerprint.Profile,
	recorder *soraProbeRecorder,
) {
	inviteStatus, inviteHeader, inviteBody, err := s.fetchSoraTestEndpoint(
		ctx,
		account,
		authToken,
		soraInviteMineURL,
		proxyURL,
		tlsProfile,
	)
	if err != nil {
		if recorder != nil {
			recorder.addStep("sora2_invite", "failed", 0, "network_error", err.Error())
		}
		s.sendEvent(c, TestEvent{Type: "content", Text: fmt.Sprintf("Sora2 invite check skipped: %s", err.Error())})
		return
	}

	if inviteStatus == http.StatusUnauthorized {
		bootstrapStatus, _, _, bootstrapErr := s.fetchSoraTestEndpoint(
			ctx,
			account,
			authToken,
			soraBootstrapURL,
			proxyURL,
			tlsProfile,
		)
		if bootstrapErr == nil && bootstrapStatus == http.StatusOK {
			if recorder != nil {
				recorder.addStep("sora2_bootstrap", "success", bootstrapStatus, "", "bootstrap endpoint ok")
			}
			s.sendEvent(c, TestEvent{Type: "content", Text: "Sora2 bootstrap OK, retry invite check"})
			inviteStatus, inviteHeader, inviteBody, err = s.fetchSoraTestEndpoint(
				ctx,
				account,
				authToken,
				soraInviteMineURL,
				proxyURL,
				tlsProfile,
			)
			if err != nil {
				if recorder != nil {
					recorder.addStep("sora2_invite", "failed", 0, "network_error", err.Error())
				}
				s.sendEvent(c, TestEvent{Type: "content", Text: fmt.Sprintf("Sora2 invite retry failed: %s", err.Error())})
				return
			}
		} else if recorder != nil {
			code := ""
			msg := ""
			if bootstrapErr != nil {
				code = "network_error"
				msg = bootstrapErr.Error()
			}
			recorder.addStep("sora2_bootstrap", "failed", bootstrapStatus, code, msg)
		}
	}

	if inviteStatus != http.StatusOK {
		if isCloudflareChallengeResponse(inviteStatus, inviteHeader, inviteBody) {
			if recorder != nil {
				recorder.addStep("sora2_invite", "failed", inviteStatus, "cf_challenge", "Cloudflare challenge detected")
			}
			s.logSoraCloudflareChallenge(account, proxyURL, soraInviteMineURL, inviteHeader, inviteBody)
			s.sendEvent(c, TestEvent{Type: "content", Text: formatCloudflareChallengeMessage(fmt.Sprintf("Sora2 invite check blocked by Cloudflare challenge (HTTP %d)", inviteStatus), inviteHeader, inviteBody)})
			return
		}
		upstreamCode, upstreamMessage := soraerror.ExtractUpstreamErrorCodeAndMessage(inviteBody)
		if recorder != nil {
			recorder.addStep("sora2_invite", "failed", inviteStatus, upstreamCode, upstreamMessage)
		}
		s.sendEvent(c, TestEvent{Type: "content", Text: fmt.Sprintf("Sora2 invite check returned %d", inviteStatus)})
		return
	}
	if recorder != nil {
		recorder.addStep("sora2_invite", "success", inviteStatus, "", "invite endpoint ok")
	}

	if summary := parseSoraInviteSummary(inviteBody); summary != "" {
		s.sendEvent(c, TestEvent{Type: "content", Text: summary})
	} else {
		s.sendEvent(c, TestEvent{Type: "content", Text: "Sora2 invite check OK"})
	}

	remainingStatus, remainingHeader, remainingBody, remainingErr := s.fetchSoraTestEndpoint(
		ctx,
		account,
		authToken,
		soraRemainingURL,
		proxyURL,
		tlsProfile,
	)
	if remainingErr != nil {
		if recorder != nil {
			recorder.addStep("sora2_remaining", "failed", 0, "network_error", remainingErr.Error())
		}
		s.sendEvent(c, TestEvent{Type: "content", Text: fmt.Sprintf("Sora2 remaining check skipped: %s", remainingErr.Error())})
		return
	}
	if remainingStatus != http.StatusOK {
		if isCloudflareChallengeResponse(remainingStatus, remainingHeader, remainingBody) {
			if recorder != nil {
				recorder.addStep("sora2_remaining", "failed", remainingStatus, "cf_challenge", "Cloudflare challenge detected")
			}
			s.logSoraCloudflareChallenge(account, proxyURL, soraRemainingURL, remainingHeader, remainingBody)
			s.sendEvent(c, TestEvent{Type: "content", Text: formatCloudflareChallengeMessage(fmt.Sprintf("Sora2 remaining check blocked by Cloudflare challenge (HTTP %d)", remainingStatus), remainingHeader, remainingBody)})
			return
		}
		upstreamCode, upstreamMessage := soraerror.ExtractUpstreamErrorCodeAndMessage(remainingBody)
		if recorder != nil {
			recorder.addStep("sora2_remaining", "failed", remainingStatus, upstreamCode, upstreamMessage)
		}
		s.sendEvent(c, TestEvent{Type: "content", Text: fmt.Sprintf("Sora2 remaining check returned %d", remainingStatus)})
		return
	}
	if recorder != nil {
		recorder.addStep("sora2_remaining", "success", remainingStatus, "", "remaining endpoint ok")
	}
	if summary := parseSoraRemainingSummary(remainingBody); summary != "" {
		s.sendEvent(c, TestEvent{Type: "content", Text: summary})
	} else {
		s.sendEvent(c, TestEvent{Type: "content", Text: "Sora2 remaining check OK"})
	}
}

func (s *AccountTestService) fetchSoraTestEndpoint(
	ctx context.Context,
	account *Account,
	authToken string,
	url string,
	proxyURL string,
	tlsProfile *tlsfingerprint.Profile,
) (int, http.Header, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+authToken)
	req.Header.Set("User-Agent", "Sora/1.2026.007 (Android 15; 24122RKC7C; build 2600700)")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Origin", "https://sora.chatgpt.com")
	req.Header.Set("Referer", "https://sora.chatgpt.com/")

	resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, tlsProfile)
	if err != nil {
		return 0, nil, nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return resp.StatusCode, resp.Header, nil, readErr
	}
	return resp.StatusCode, resp.Header, body, nil
}

func parseSoraSubscriptionSummary(body []byte) string {
	var subResp struct {
		Data []struct {
			Plan struct {
				ID    string `json:"id"`
				Title string `json:"title"`
			} `json:"plan"`
			EndTS string `json:"end_ts"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &subResp); err != nil {
		return ""
	}
	if len(subResp.Data) == 0 {
		return ""
	}

	first := subResp.Data[0]
	parts := make([]string, 0, 3)
	if first.Plan.Title != "" {
		parts = append(parts, first.Plan.Title)
	}
	if first.Plan.ID != "" {
		parts = append(parts, first.Plan.ID)
	}
	if first.EndTS != "" {
		parts = append(parts, "end="+first.EndTS)
	}
	if len(parts) == 0 {
		return ""
	}
	return "Subscription: " + strings.Join(parts, " | ")
}

func parseSoraInviteSummary(body []byte) string {
	var inviteResp struct {
		InviteCode    string `json:"invite_code"`
		RedeemedCount int64  `json:"redeemed_count"`
		TotalCount    int64  `json:"total_count"`
	}
	if err := json.Unmarshal(body, &inviteResp); err != nil {
		return ""
	}

	parts := []string{"Sora2: supported"}
	if inviteResp.InviteCode != "" {
		parts = append(parts, "invite="+inviteResp.InviteCode)
	}
	if inviteResp.TotalCount > 0 {
		parts = append(parts, fmt.Sprintf("used=%d/%d", inviteResp.RedeemedCount, inviteResp.TotalCount))
	}
	return strings.Join(parts, " | ")
}

func parseSoraRemainingSummary(body []byte) string {
	var remainingResp struct {
		RateLimitAndCreditBalance struct {
			EstimatedNumVideosRemaining int64 `json:"estimated_num_videos_remaining"`
			RateLimitReached            bool  `json:"rate_limit_reached"`
			AccessResetsInSeconds       int64 `json:"access_resets_in_seconds"`
		} `json:"rate_limit_and_credit_balance"`
	}
	if err := json.Unmarshal(body, &remainingResp); err != nil {
		return ""
	}
	info := remainingResp.RateLimitAndCreditBalance
	parts := []string{fmt.Sprintf("Sora2 remaining: %d", info.EstimatedNumVideosRemaining)}
	if info.RateLimitReached {
		parts = append(parts, "rate_limited=true")
	}
	if info.AccessResetsInSeconds > 0 {
		parts = append(parts, fmt.Sprintf("reset_in=%ds", info.AccessResetsInSeconds))
	}
	return strings.Join(parts, " | ")
}

func (s *AccountTestService) resolveSoraTLSProfile() *tlsfingerprint.Profile {
	if s == nil || s.cfg == nil || !s.cfg.Sora.Client.DisableTLSFingerprint {
		// Sora TLS fingerprint enabled — use built-in default profile
		return &tlsfingerprint.Profile{Name: "Built-in Default (Sora)"}
	}
	return nil // disabled
}

func isCloudflareChallengeResponse(statusCode int, headers http.Header, body []byte) bool {
	return soraerror.IsCloudflareChallengeResponse(statusCode, headers, body)
}

func formatCloudflareChallengeMessage(base string, headers http.Header, body []byte) string {
	return soraerror.FormatCloudflareChallengeMessage(base, headers, body)
}

func extractCloudflareRayID(headers http.Header, body []byte) string {
	return soraerror.ExtractCloudflareRayID(headers, body)
}

func extractSoraEgressIPHint(headers http.Header) string {
	if headers == nil {
		return "unknown"
	}
	candidates := []string{
		"x-openai-public-ip",
		"x-envoy-external-address",
		"cf-connecting-ip",
		"x-forwarded-for",
	}
	for _, key := range candidates {
		if value := strings.TrimSpace(headers.Get(key)); value != "" {
			return value
		}
	}
	return "unknown"
}

func sanitizeProxyURLForLog(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "<invalid_proxy_url>"
	}
	if u.User != nil {
		u.User = nil
	}
	return u.String()
}

func endpointPathForLog(endpoint string) string {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || parsed.Path == "" {
		return endpoint
	}
	return parsed.Path
}

func (s *AccountTestService) logSoraCloudflareChallenge(account *Account, proxyURL, endpoint string, headers http.Header, body []byte) {
	accountID := int64(0)
	platform := ""
	proxyID := "none"
	if account != nil {
		accountID = account.ID
		platform = account.Platform
		if account.ProxyID != nil {
			proxyID = fmt.Sprintf("%d", *account.ProxyID)
		}
	}
	cfRay := extractCloudflareRayID(headers, body)
	if cfRay == "" {
		cfRay = "unknown"
	}
	log.Printf(
		"[SoraCFChallenge] account_id=%d platform=%s endpoint=%s path=%s proxy_id=%s proxy_url=%s cf_ray=%s egress_ip_hint=%s",
		accountID,
		platform,
		endpoint,
		endpointPathForLog(endpoint),
		proxyID,
		sanitizeProxyURLForLog(proxyURL),
		cfRay,
		extractSoraEgressIPHint(headers),
	)
}

func truncateSoraErrorBody(body []byte, max int) string {
	return soraerror.TruncateBody(body, max)
}

// routeAntigravityTest 路由 Antigravity 账号的测试请求。
// APIKey 类型走原生协议（与 gateway_handler 路由一致），OAuth/Upstream 走 CRS 中转。
func (s *AccountTestService) routeAntigravityTest(c *gin.Context, account *Account, modelID string, prompt string) error {
	if account.Type == AccountTypeAPIKey {
		if strings.HasPrefix(modelID, "gemini-") {
			return s.testGeminiAccountConnection(c, account, modelID, prompt)
		}
		return s.testClaudeAccountConnection(c, account, modelID)
	}
	return s.testAntigravityAccountConnection(c, account, modelID)
}

// testAntigravityAccountConnection tests an Antigravity account's connection
// 支持 Claude 和 Gemini 两种协议，使用非流式请求
func (s *AccountTestService) testAntigravityAccountConnection(c *gin.Context, account *Account, modelID string) error {
	ctx := c.Request.Context()

	// 默认模型：Claude 使用 claude-sonnet-4-5，Gemini 使用 gemini-3-pro-preview
	testModelID := modelID
	if testModelID == "" {
		testModelID = "claude-sonnet-4-5"
	}

	if s.antigravityGatewayService == nil {
		return s.sendErrorAndEnd(c, "Antigravity gateway service not configured")
	}

	// Set SSE headers
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	// Send test_start event
	s.sendEvent(c, TestEvent{Type: "test_start", Model: testModelID})

	// 调用 AntigravityGatewayService.TestConnection（复用协议转换逻辑）
	result, err := s.antigravityGatewayService.TestConnection(ctx, account, testModelID)
	if err != nil {
		return s.sendErrorAndEnd(c, err.Error())
	}

	// 发送响应内容
	if result.Text != "" {
		s.sendEvent(c, TestEvent{Type: "content", Text: result.Text})
	}

	s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
	return nil
}

// buildGeminiAPIKeyRequest builds request for Gemini API Key accounts
func (s *AccountTestService) buildGeminiAPIKeyRequest(ctx context.Context, account *Account, modelID string, payload []byte) (*http.Request, error) {
	apiKey := account.GetCredential("api_key")
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("no API key available")
	}

	baseURL := account.GetCredential("base_url")
	if baseURL == "" {
		baseURL = geminicli.AIStudioBaseURL
	}
	normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, err
	}

	// Use streamGenerateContent for real-time feedback
	fullURL := fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent?alt=sse",
		strings.TrimRight(normalizedBaseURL, "/"), modelID)

	req, err := http.NewRequestWithContext(ctx, "POST", fullURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", apiKey)

	return req, nil
}

// buildGeminiOAuthRequest builds request for Gemini OAuth accounts
func (s *AccountTestService) buildGeminiOAuthRequest(ctx context.Context, account *Account, modelID string, payload []byte) (*http.Request, error) {
	if s.geminiTokenProvider == nil {
		return nil, fmt.Errorf("gemini token provider not configured")
	}

	// Get access token (auto-refreshes if needed)
	accessToken, err := s.geminiTokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	projectID := strings.TrimSpace(account.GetCredential("project_id"))
	if projectID == "" {
		// AI Studio OAuth mode (no project_id): call generativelanguage API directly with Bearer token.
		baseURL := account.GetCredential("base_url")
		if strings.TrimSpace(baseURL) == "" {
			baseURL = geminicli.AIStudioBaseURL
		}
		normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
		if err != nil {
			return nil, err
		}
		fullURL := fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent?alt=sse", strings.TrimRight(normalizedBaseURL, "/"), modelID)

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+accessToken)
		return req, nil
	}

	// Code Assist mode (with project_id)
	return s.buildCodeAssistRequest(ctx, accessToken, projectID, modelID, payload)
}

// buildCodeAssistRequest builds request for Google Code Assist API (used by Gemini CLI and Antigravity)
func (s *AccountTestService) buildCodeAssistRequest(ctx context.Context, accessToken, projectID, modelID string, payload []byte) (*http.Request, error) {
	var inner map[string]any
	if err := json.Unmarshal(payload, &inner); err != nil {
		return nil, err
	}

	wrapped := map[string]any{
		"model":   modelID,
		"project": projectID,
		"request": inner,
	}
	wrappedBytes, _ := json.Marshal(wrapped)

	normalizedBaseURL, err := s.validateUpstreamBaseURL(geminicli.GeminiCliBaseURL)
	if err != nil {
		return nil, err
	}
	fullURL := fmt.Sprintf("%s/v1internal:streamGenerateContent?alt=sse", normalizedBaseURL)

	req, err := http.NewRequestWithContext(ctx, "POST", fullURL, bytes.NewReader(wrappedBytes))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", geminicli.GeminiCLIUserAgent)

	return req, nil
}

// createGeminiTestPayload creates a minimal test payload for Gemini API.
// Image models use the image-generation path so the frontend can preview the returned image.
func createGeminiTestPayload(modelID string, prompt string) []byte {
	if isImageGenerationModel(modelID) {
		imagePrompt := strings.TrimSpace(prompt)
		if imagePrompt == "" {
			imagePrompt = defaultGeminiImageTestPrompt
		}

		payload := map[string]any{
			"contents": []map[string]any{
				{
					"role": "user",
					"parts": []map[string]any{
						{"text": imagePrompt},
					},
				},
			},
			"generationConfig": map[string]any{
				"responseModalities": []string{"TEXT", "IMAGE"},
				"imageConfig": map[string]any{
					"aspectRatio": "1:1",
				},
			},
		}
		bytes, _ := json.Marshal(payload)
		return bytes
	}

	textPrompt := strings.TrimSpace(prompt)
	if textPrompt == "" {
		textPrompt = defaultGeminiTextTestPrompt
	}

	payload := map[string]any{
		"contents": []map[string]any{
			{
				"role": "user",
				"parts": []map[string]any{
					{"text": textPrompt},
				},
			},
		},
		"systemInstruction": map[string]any{
			"parts": []map[string]any{
				{"text": "You are a helpful AI assistant."},
			},
		},
	}
	bytes, _ := json.Marshal(payload)
	return bytes
}

// processGeminiStream processes SSE stream from Gemini API
func (s *AccountTestService) processGeminiStream(c *gin.Context, body io.Reader) error {
	reader := bufio.NewReader(body)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
				return nil
			}
			return s.sendErrorAndEnd(c, fmt.Sprintf("Stream read error: %s", err.Error()))
		}

		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "data: ") {
			continue
		}

		jsonStr := strings.TrimPrefix(line, "data: ")
		if jsonStr == "[DONE]" {
			s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
			return nil
		}

		var data map[string]any
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			continue
		}

		// Support two Gemini response formats:
		// - AI Studio: {"candidates": [...]}
		// - Gemini CLI: {"response": {"candidates": [...]}}
		if resp, ok := data["response"].(map[string]any); ok && resp != nil {
			data = resp
		}
		if candidates, ok := data["candidates"].([]any); ok && len(candidates) > 0 {
			if candidate, ok := candidates[0].(map[string]any); ok {
				// Extract content first (before checking completion)
				if content, ok := candidate["content"].(map[string]any); ok {
					if parts, ok := content["parts"].([]any); ok {
						for _, part := range parts {
							if partMap, ok := part.(map[string]any); ok {
								if text, ok := partMap["text"].(string); ok && text != "" {
									s.sendEvent(c, TestEvent{Type: "content", Text: text})
								}
								if inlineData, ok := partMap["inlineData"].(map[string]any); ok {
									mimeType, _ := inlineData["mimeType"].(string)
									data, _ := inlineData["data"].(string)
									if strings.HasPrefix(strings.ToLower(mimeType), "image/") && data != "" {
										s.sendEvent(c, TestEvent{
											Type:     "image",
											ImageURL: fmt.Sprintf("data:%s;base64,%s", mimeType, data),
											MimeType: mimeType,
										})
									}
								}
							}
						}
					}
				}

				// Check for completion after extracting content
				if finishReason, ok := candidate["finishReason"].(string); ok && finishReason != "" {
					s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
					return nil
				}
			}
		}

		// Handle errors
		if errData, ok := data["error"].(map[string]any); ok {
			errorMsg := "Unknown error"
			if msg, ok := errData["message"].(string); ok {
				errorMsg = msg
			}
			return s.sendErrorAndEnd(c, errorMsg)
		}
	}
}

// createOpenAITestPayload creates a test payload for OpenAI Responses API
func createOpenAITestPayload(modelID string, isOAuth bool) map[string]any {
	payload := map[string]any{
		"model": modelID,
		"input": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type": "input_text",
						"text": "hi",
					},
				},
			},
		},
		"stream": true,
	}

	// OAuth accounts using ChatGPT internal API require store: false
	if isOAuth {
		payload["store"] = false
	}

	// All accounts require instructions for Responses API
	payload["instructions"] = openai.DefaultInstructions

	return payload
}

// processClaudeStream processes the SSE stream from Claude API
func (s *AccountTestService) processClaudeStream(c *gin.Context, body io.Reader) error {
	reader := bufio.NewReader(body)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
				return nil
			}
			return s.sendErrorAndEnd(c, fmt.Sprintf("Stream read error: %s", err.Error()))
		}

		line = strings.TrimSpace(line)
		if line == "" || !sseDataPrefix.MatchString(line) {
			continue
		}

		jsonStr := sseDataPrefix.ReplaceAllString(line, "")
		if jsonStr == "[DONE]" {
			s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
			return nil
		}

		var data map[string]any
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			continue
		}

		eventType, _ := data["type"].(string)

		switch eventType {
		case "content_block_delta":
			if delta, ok := data["delta"].(map[string]any); ok {
				if text, ok := delta["text"].(string); ok {
					s.sendEvent(c, TestEvent{Type: "content", Text: text})
				}
			}
		case "message_stop":
			s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
			return nil
		case "error":
			errorMsg := "Unknown error"
			if errData, ok := data["error"].(map[string]any); ok {
				if msg, ok := errData["message"].(string); ok {
					errorMsg = msg
				}
			}
			return s.sendErrorAndEnd(c, errorMsg)
		}
	}
}

// processOpenAIStream processes the SSE stream from OpenAI Responses API
func (s *AccountTestService) processOpenAIStream(c *gin.Context, body io.Reader) error {
	reader := bufio.NewReader(body)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
				return nil
			}
			return s.sendErrorAndEnd(c, fmt.Sprintf("Stream read error: %s", err.Error()))
		}

		line = strings.TrimSpace(line)
		if line == "" || !sseDataPrefix.MatchString(line) {
			continue
		}

		jsonStr := sseDataPrefix.ReplaceAllString(line, "")
		if jsonStr == "[DONE]" {
			s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
			return nil
		}

		var data map[string]any
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			continue
		}

		eventType, _ := data["type"].(string)

		switch eventType {
		case "response.output_text.delta":
			// OpenAI Responses API uses "delta" field for text content
			if delta, ok := data["delta"].(string); ok && delta != "" {
				s.sendEvent(c, TestEvent{Type: "content", Text: delta})
			}
		case "response.completed":
			s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
			return nil
		case "error":
			errorMsg := "Unknown error"
			if errData, ok := data["error"].(map[string]any); ok {
				if msg, ok := errData["message"].(string); ok {
					errorMsg = msg
				}
			}
			return s.sendErrorAndEnd(c, errorMsg)
		}
	}
}

// sendEvent sends a SSE event to the client
func (s *AccountTestService) sendEvent(c *gin.Context, event TestEvent) {
	eventJSON, _ := json.Marshal(event)
	if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", eventJSON); err != nil {
		log.Printf("failed to write SSE event: %v", err)
		return
	}
	c.Writer.Flush()
}

// sendErrorAndEnd sends an error event and ends the stream
func (s *AccountTestService) sendErrorAndEnd(c *gin.Context, errorMsg string) error {
	log.Printf("Account test error: %s", errorMsg)
	s.sendEvent(c, TestEvent{Type: "error", Error: errorMsg})
	return fmt.Errorf("%s", errorMsg)
}

// RunTestBackground executes an account test in-memory (no real HTTP client),
// capturing SSE output via httptest.NewRecorder, then parses the result.
func (s *AccountTestService) RunTestBackground(ctx context.Context, accountID int64, modelID string) (*ScheduledTestResult, error) {
	startedAt := time.Now()

	w := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(w)
	ginCtx.Request = (&http.Request{}).WithContext(ctx)

	testErr := s.TestAccountConnection(ginCtx, accountID, modelID, "")

	finishedAt := time.Now()
	body := w.Body.String()
	responseText, errMsg := parseTestSSEOutput(body)

	status := "success"
	if testErr != nil || errMsg != "" {
		status = "failed"
		if errMsg == "" && testErr != nil {
			errMsg = testErr.Error()
		}
	}

	return &ScheduledTestResult{
		Status:       status,
		ResponseText: responseText,
		ErrorMessage: errMsg,
		LatencyMs:    finishedAt.Sub(startedAt).Milliseconds(),
		StartedAt:    startedAt,
		FinishedAt:   finishedAt,
	}, nil
}

// parseTestSSEOutput extracts response text and error message from captured SSE output.
func parseTestSSEOutput(body string) (responseText, errMsg string) {
	var texts []string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		jsonStr := strings.TrimPrefix(line, "data: ")
		var event TestEvent
		if err := json.Unmarshal([]byte(jsonStr), &event); err != nil {
			continue
		}
		switch event.Type {
		case "content":
			if event.Text != "" {
				texts = append(texts, event.Text)
			}
		case "error":
			errMsg = event.Error
		}
	}
	responseText = strings.Join(texts, "")
	return
}
