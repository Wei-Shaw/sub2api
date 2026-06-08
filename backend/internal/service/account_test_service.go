package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
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
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
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
)

// TestEvent represents a SSE event for account testing
type TestEvent struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	Model     string `json:"model,omitempty"`
	Status    string `json:"status,omitempty"`
	Code      string `json:"code,omitempty"`
	ImageURL  string `json:"image_url,omitempty"`
	VideoURL  string `json:"video_url,omitempty"`
	SourceURL string `json:"source_url,omitempty"`
	MimeType  string `json:"mime_type,omitempty"`
	Data      any    `json:"data,omitempty"`
	Success   bool   `json:"success,omitempty"`
	Error     string `json:"error,omitempty"`
}

const (
	defaultGeminiTextTestPrompt  = "hi"
	defaultGeminiImageTestPrompt = "Generate a cute orange cat astronaut sticker on a clean pastel background."
	defaultOpenAIImageTestPrompt = "Generate a cute orange cat astronaut sticker on a clean pastel background."
	defaultXAIVideoTestPrompt    = "A tiny blue square slowly moving left to right on a white background."
	defaultXAIImageToVideoPrompt = "Animate this source image with subtle natural motion and a gentle camera push-in. Preserve the subject and composition."
	defaultXAIImageToVideoURL    = "https://docs.x.ai/assets/api-examples/images/style-realistic.png"
	xaiVideoTestDurationSeconds  = 2
	xaiVideoTestResolution       = "480p"
	xaiVideoPollInterval         = 3 * time.Second
	xaiVideoPollTimeout          = 2 * time.Minute
	xaiVideoInlineMaxBytes       = 20 << 20
	xaiVideoInlineFetchTimeout   = 20 * time.Second
)

// isOpenAIImageModel checks if the model is an OpenAI image generation model (e.g. gpt-image-2).
func isOpenAIImageModel(model string) bool {
	return strings.HasPrefix(strings.ToLower(model), "gpt-image-")
}

func isXAIImageModel(model string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "grok-imagine-image")
}

func isXAIVideoModel(model string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "grok-imagine-video")
}

func isXAIImageToVideoOnlyModel(model string) bool {
	return strings.EqualFold(strings.TrimSpace(model), "grok-imagine-video-1.5-preview")
}

// AccountTestService handles account testing operations
type AccountTestService struct {
	accountRepo               AccountRepository
	geminiTokenProvider       *GeminiTokenProvider
	claudeTokenProvider       *ClaudeTokenProvider
	xaiTokenProvider          *XAITokenProvider
	antigravityGatewayService *AntigravityGatewayService
	httpUpstream              HTTPUpstream
	cfg                       *config.Config
	tlsFPProfileService       *TLSFingerprintProfileService
}

// NewAccountTestService creates a new AccountTestService
func NewAccountTestService(
	accountRepo AccountRepository,
	geminiTokenProvider *GeminiTokenProvider,
	claudeTokenProvider *ClaudeTokenProvider,
	xaiTokenProvider *XAITokenProvider,
	antigravityGatewayService *AntigravityGatewayService,
	httpUpstream HTTPUpstream,
	cfg *config.Config,
	tlsFPProfileService *TLSFingerprintProfileService,
) *AccountTestService {
	return &AccountTestService{
		accountRepo:               accountRepo,
		geminiTokenProvider:       geminiTokenProvider,
		claudeTokenProvider:       claudeTokenProvider,
		xaiTokenProvider:          xaiTokenProvider,
		antigravityGatewayService: antigravityGatewayService,
		httpUpstream:              httpUpstream,
		cfg:                       cfg,
		tlsFPProfileService:       tlsFPProfileService,
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
// mode is optional - "compact" routes OpenAI accounts to the /responses/compact probe path
func (s *AccountTestService) TestAccountConnection(c *gin.Context, accountID int64, modelID string, prompt string, mode string) error {
	ctx := c.Request.Context()

	// Get account
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return s.sendErrorAndEnd(c, "Account not found")
	}

	// Route to platform-specific test method
	if account.IsOpenAI() {
		return s.testOpenAIAccountConnection(c, account, modelID, prompt, normalizeAccountTestMode(mode))
	}

	if account.IsXAI() {
		return s.testXAIAccountConnection(c, account, modelID, prompt)
	}

	if account.IsGemini() {
		return s.testGeminiAccountConnection(c, account, modelID, prompt)
	}

	if account.Platform == PlatformAntigravity {
		return s.routeAntigravityTest(c, account, modelID, prompt)
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
	if account.Type == AccountTypeServiceAccount {
		return s.testClaudeVertexServiceAccountConnection(c, ctx, account, testModelID)
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

func (s *AccountTestService) testClaudeVertexServiceAccountConnection(c *gin.Context, ctx context.Context, account *Account, testModelID string) error {
	if mappedModel, matched := account.ResolveMappedModel(testModelID); matched {
		testModelID = mappedModel
	} else {
		testModelID = normalizeVertexAnthropicModelID(claude.NormalizeModelID(testModelID))
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	payload, err := createTestPayload(testModelID)
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create test payload")
	}
	payloadBytes, _ := json.Marshal(payload)
	vertexBody, err := buildVertexAnthropicRequestBody(payloadBytes)
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Failed to create Vertex request body: %s", err.Error()))
	}

	if s.claudeTokenProvider == nil {
		return s.sendErrorAndEnd(c, "Claude token provider not configured")
	}
	accessToken, err := s.claudeTokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Failed to get service account access token: %s", err.Error()))
	}

	fullURL, err := buildVertexAnthropicURL(account.VertexProjectID(), account.VertexLocation(testModelID), testModelID, true)
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Failed to build Vertex URL: %s", err.Error()))
	}

	s.sendEvent(c, TestEvent{Type: "test_start", Model: testModelID})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(vertexBody))
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

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
		if resp.StatusCode == http.StatusForbidden {
			_ = s.accountRepo.SetError(ctx, account.ID, errMsg)
		}
		return s.sendErrorAndEnd(c, errMsg)
	}

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
func (s *AccountTestService) testOpenAIAccountConnection(c *gin.Context, account *Account, modelID string, prompt string, mode string) error {
	ctx := c.Request.Context()
	mode = normalizeAccountTestMode(mode)

	// Default to openai.DefaultTestModel for OpenAI testing
	testModelID := modelID
	if testModelID == "" {
		testModelID = openai.DefaultTestModel
	}

	// Align test routing with gateway behavior: OpenAI accounts apply normal
	// account model mapping, and compact mode applies compact-only mapping on top.
	testModelID = account.GetMappedModel(testModelID)
	if mode == AccountTestModeCompact {
		testModelID = resolveOpenAICompactForwardModel(account, testModelID)
		return s.testOpenAICompactConnection(c, account, testModelID)
	}

	// Route to image generation test if an image model is selected
	if isOpenAIImageModel(testModelID) {
		imagePrompt := strings.TrimSpace(prompt)
		if imagePrompt == "" {
			imagePrompt = defaultOpenAIImageTestPrompt
		}
		if account.Type == "apikey" {
			return s.testOpenAIImageAPIKey(c, ctx, account, testModelID, imagePrompt)
		}
		return s.testOpenAIImageOAuth(c, ctx, account, testModelID, imagePrompt)
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
		if !openai_compat.ShouldUseResponsesAPI(account.Extra) {
			return s.testOpenAIChatCompletionsConnection(c, account, testModelID, prompt, normalizedBaseURL, authToken)
		}
		apiURL = buildOpenAIResponsesURL(normalizedBaseURL)
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
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))

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
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusTooManyRequests {
			s.reconcileOpenAI429State(ctx, account, resp.Header, body)
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

// testOpenAIChatCompletionsConnection tests an OpenAI-compatible APIKey account
// through the raw /v1/chat/completions endpoint.
func (s *AccountTestService) testOpenAIChatCompletionsConnection(
	c *gin.Context,
	account *Account,
	testModelID string,
	prompt string,
	normalizedBaseURL string,
	authToken string,
) error {
	ctx := c.Request.Context()
	apiURL := buildOpenAIChatCompletionsURL(normalizedBaseURL)

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	payload := createOpenAIChatCompletionsTestPayload(testModelID, prompt)
	payloadBytes, _ := json.Marshal(payload)

	s.sendEvent(c, TestEvent{Type: "test_start", Model: testModelID})
	s.sendEvent(c, TestEvent{Type: "status", Text: "正在通过 /v1/chat/completions 测试连接"})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create Chat Completions request")
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+authToken)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Chat Completions API (/v1/chat/completions) request failed: %s", err.Error()))
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusTooManyRequests {
			s.reconcileOpenAI429State(ctx, account, resp.Header, body)
		}
		if resp.StatusCode == http.StatusUnauthorized && s.accountRepo != nil {
			errMsg := fmt.Sprintf("Chat Completions authentication failed (401): %s", string(body))
			_ = s.accountRepo.SetError(ctx, account.ID, errMsg)
		}
		return s.sendErrorAndEnd(c, fmt.Sprintf("Chat Completions API (/v1/chat/completions) returned %d: %s", resp.StatusCode, string(body)))
	}

	return s.processOpenAIChatCompletionsStream(c, resp.Body)
}

// testOpenAICompactConnection probes /responses/compact and persists the
// resulting capability state on the account.
func (s *AccountTestService) testOpenAICompactConnection(c *gin.Context, account *Account, testModelID string) error {
	ctx := c.Request.Context()

	authToken := ""
	apiURL := ""
	isOAuth := false
	chatgptAccountID := ""

	switch {
	case account.IsOAuth():
		isOAuth = true
		authToken = account.GetOpenAIAccessToken()
		if authToken == "" {
			return s.sendErrorAndEnd(c, "No access token available")
		}
		apiURL = chatgptCodexAPIURL + "/compact"
		chatgptAccountID = account.GetChatGPTAccountID()
	case account.Type == AccountTypeAPIKey:
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
		apiURL = appendOpenAIResponsesRequestPathSuffix(buildOpenAIResponsesURL(normalizedBaseURL), "/compact")
	default:
		return s.sendErrorAndEnd(c, fmt.Sprintf("Unsupported account type: %s", account.Type))
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	payloadBytes, _ := json.Marshal(createOpenAICompactProbePayload(testModelID))
	s.sendEvent(c, TestEvent{Type: "test_start", Model: testModelID})

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create request")
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+authToken)
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	req.Header.Set("Originator", "codex_cli_rs")
	req.Header.Set("User-Agent", codexCLIUserAgent)
	req.Header.Set("Version", codexCLIVersion)
	probeSessionID := compactProbeSessionID(account.ID)
	req.Header.Set("Session_ID", probeSessionID)
	req.Header.Set("Conversation_ID", probeSessionID)

	if isOAuth {
		req.Host = "chatgpt.com"
		if chatgptAccountID != "" {
			req.Header.Set("chatgpt-account-id", chatgptAccountID)
		}
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
	if err != nil {
		if s.accountRepo != nil {
			updates := buildOpenAICompactProbeExtraUpdates(nil, nil, err, time.Now())
			_ = s.accountRepo.UpdateExtra(ctx, account.ID, updates)
			mergeAccountExtra(account, updates)
		}
		return s.sendErrorAndEnd(c, fmt.Sprintf("Request failed: %s", err.Error()))
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))

	if s.accountRepo != nil {
		updates := buildOpenAICompactProbeExtraUpdates(resp, body, nil, time.Now())
		if codexUpdates, err := extractOpenAICodexProbeUpdates(resp); err == nil && len(codexUpdates) > 0 {
			updates = mergeExtraUpdates(updates, codexUpdates)
		}
		if len(updates) > 0 {
			_ = s.accountRepo.UpdateExtra(ctx, account.ID, updates)
			mergeAccountExtra(account, updates)
		}
		// 探测如返回 429,主动同步限流状态,避免后续短时间内继续选中。
		if resp.StatusCode == http.StatusTooManyRequests {
			s.reconcileOpenAI429State(ctx, account, resp.Header, body)
		}
	}

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusUnauthorized && s.accountRepo != nil {
			errMsg := fmt.Sprintf("Authentication failed (401): %s", string(body))
			_ = s.accountRepo.SetError(ctx, account.ID, errMsg)
		}
		return s.sendErrorAndEnd(c, fmt.Sprintf("API returned %d: %s", resp.StatusCode, string(body)))
	}

	s.sendEvent(c, TestEvent{Type: "content", Text: "Compact probe succeeded"})
	s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
	return nil
}

func (s *AccountTestService) reconcileOpenAI429State(ctx context.Context, account *Account, headers http.Header, body []byte) {
	if s == nil || s.accountRepo == nil || account == nil {
		return
	}

	persistOpenAI429PlanType(ctx, s.accountRepo, account, body)

	var resetAt *time.Time
	if calculated := calculateOpenAI429ResetTime(headers); calculated != nil {
		resetAt = calculated
	} else if unixTs := parseOpenAIRateLimitResetTime(body); unixTs != nil {
		t := time.Unix(*unixTs, 0)
		resetAt = &t
	}
	if resetAt == nil {
		return
	}

	if err := s.accountRepo.SetRateLimited(ctx, account.ID, *resetAt); err != nil {
		return
	}

	now := time.Now()
	account.RateLimitedAt = &now
	account.RateLimitResetAt = resetAt

	if account.Status == StatusError {
		if err := s.accountRepo.ClearError(ctx, account.ID); err != nil {
			return
		}
		account.Status = StatusActive
		account.ErrorMessage = ""
	}
}

// testXAIAccountConnection tests an xAI/Grok account via the compatible endpoint for the selected model.
func (s *AccountTestService) testXAIAccountConnection(c *gin.Context, account *Account, modelID string, prompt string) error {
	ctx := c.Request.Context()

	testModelID := modelID
	if testModelID == "" {
		testModelID = xai.DefaultTestModel
	}
	if account.Type == AccountTypeAPIKey {
		testModelID = account.GetMappedModel(testModelID)
	}

	if isXAIVideoModel(testModelID) {
		videoPrompt := strings.TrimSpace(prompt)
		if videoPrompt == "" {
			if isXAIImageToVideoOnlyModel(testModelID) {
				videoPrompt = defaultXAIImageToVideoPrompt
			} else {
				videoPrompt = defaultXAIVideoTestPrompt
			}
		}
		return s.testXAIVideoGeneration(c, ctx, account, testModelID, videoPrompt)
	}

	if isXAIImageModel(testModelID) {
		imagePrompt := strings.TrimSpace(prompt)
		if imagePrompt == "" {
			imagePrompt = defaultOpenAIImageTestPrompt
		}
		return s.testXAIImageGeneration(c, ctx, account, testModelID, imagePrompt)
	}

	authToken, err := s.resolveXAIAuthToken(ctx, account)
	if err != nil {
		return s.sendErrorAndEnd(c, err.Error())
	}

	normalizedBaseURL, err := s.validateUpstreamBaseURL(account.GetXAIBaseURL())
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Invalid base URL: %s", err.Error()))
	}

	payload := createOpenAITestPayload(testModelID, false)
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create test payload")
	}
	payloadBytes, err = xai.NormalizeResponsesBody(payloadBytes, testModelID, true, "")
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Failed to normalize request: %s", err.Error()))
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	s.sendEvent(c, TestEvent{Type: "test_start", Model: testModelID})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, xai.BuildResponsesURL(normalizedBaseURL), bytes.NewReader(payloadBytes))
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Connection", "Keep-Alive")
	req.Header.Set("Authorization", "Bearer "+authToken)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	var resp *http.Response
	if s.tlsFPProfileService != nil {
		resp, err = s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
	} else {
		resp, err = s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	}
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Request failed: %s", err.Error()))
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusUnauthorized && s.accountRepo != nil {
			errMsg := fmt.Sprintf("Authentication failed (401): %s", string(body))
			_ = s.accountRepo.SetError(ctx, account.ID, errMsg)
		}
		return s.sendErrorAndEnd(c, fmt.Sprintf("API returned %d: %s", resp.StatusCode, string(body)))
	}

	return s.processOpenAIStream(c, resp.Body)
}

func (s *AccountTestService) testXAIImageGeneration(c *gin.Context, ctx context.Context, account *Account, modelID, prompt string) error {
	authToken, err := s.resolveXAIAuthToken(ctx, account)
	if err != nil {
		return s.sendErrorAndEnd(c, err.Error())
	}

	normalizedBaseURL, err := s.validateUpstreamBaseURL(account.GetXAIBaseURL())
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Invalid base URL: %s", err.Error()))
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	s.sendEvent(c, TestEvent{Type: "test_start", Model: modelID})

	payload := map[string]any{
		"model":           modelID,
		"prompt":          prompt,
		"n":               1,
		"response_format": "b64_json",
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create xAI image generation payload")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, xai.BuildImagesGenerationsURL(normalizedBaseURL), bytes.NewReader(payloadBytes))
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create xAI image generation request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+authToken)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	var resp *http.Response
	if s.tlsFPProfileService != nil {
		resp, err = s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
	} else {
		resp, err = s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	}
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("xAI image generation request failed: %s", err.Error()))
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Failed to read xAI image response: %s", err.Error()))
	}

	if resp.StatusCode != http.StatusOK {
		return s.sendErrorAndEnd(c, formatXAIImageHTTPError(resp.StatusCode, body))
	}

	var result struct {
		Data []struct {
			URL           string `json:"url"`
			B64JSON       string `json:"b64_json"`
			RevisedPrompt string `json:"revised_prompt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Failed to parse xAI image response: %s", err.Error()))
	}
	if len(result.Data) == 0 {
		return s.sendErrorAndEnd(c, "No images returned from xAI image generation API")
	}

	sentImages := 0
	for _, item := range result.Data {
		if item.RevisedPrompt != "" {
			s.sendEvent(c, TestEvent{Type: "content", Text: item.RevisedPrompt})
		}
		switch {
		case strings.TrimSpace(item.B64JSON) != "":
			s.sendEvent(c, TestEvent{
				Type:     "image",
				ImageURL: "data:image/png;base64," + strings.TrimSpace(item.B64JSON),
				MimeType: "image/png",
			})
			sentImages++
		case strings.TrimSpace(item.URL) != "":
			s.sendEvent(c, TestEvent{
				Type:     "image",
				ImageURL: strings.TrimSpace(item.URL),
				MimeType: "image/*",
			})
			sentImages++
		}
	}
	if sentImages == 0 {
		return s.sendErrorAndEnd(c, "xAI image generation response did not include image data")
	}

	s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
	return nil
}

type xaiVideoPollResult struct {
	Status       string
	Progress     string
	VideoURL     string
	ErrorMessage string
	Body         string
}

func (s *AccountTestService) testXAIVideoGeneration(c *gin.Context, ctx context.Context, account *Account, modelID, prompt string) error {
	authToken, err := s.resolveXAIAuthToken(ctx, account)
	if err != nil {
		return s.sendErrorAndEnd(c, err.Error())
	}

	normalizedBaseURL, err := s.validateUpstreamBaseURL(account.GetXAIBaseURL())
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Invalid base URL: %s", err.Error()))
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	s.sendEvent(c, TestEvent{Type: "test_start", Model: modelID})

	payload := map[string]any{
		"model":    modelID,
		"prompt":   prompt,
		"duration": xaiVideoTestDurationSeconds,
	}
	if isXAIImageToVideoOnlyModel(modelID) {
		payload["image"] = map[string]any{
			"url":  defaultXAIImageToVideoURL,
			"type": "image_url",
		}
		payload["resolution"] = xaiVideoTestResolution
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create xAI video generation payload")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, xai.BuildVideosGenerationsURL(normalizedBaseURL), bytes.NewReader(payloadBytes))
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create xAI video generation request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+authToken)

	resp, err := s.doXAIAccountTestRequest(req, account)
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("xAI video generation request failed: %s", err.Error()))
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Failed to read xAI video generation response: %s", err.Error()))
	}
	if resp.StatusCode != http.StatusOK {
		return s.sendErrorAndEnd(c, formatXAIVideoHTTPError(resp.StatusCode, body))
	}

	var createResult any
	if err := json.Unmarshal(body, &createResult); err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Failed to parse xAI video generation response: %s", err.Error()))
	}
	requestID := extractJSONStringByKeys(createResult, "request_id", "id")
	if requestID == "" {
		if upstreamError := extractJSONErrorMessage(createResult); upstreamError != "" {
			return s.sendErrorAndEnd(c, fmt.Sprintf("xAI video generation failed: %s", upstreamError))
		}
		return s.sendErrorAndEnd(c, "xAI video generation response did not include request_id")
	}

	s.sendEvent(c, TestEvent{Type: "status", Text: fmt.Sprintf("xAI video task created: %s", requestID)})

	timer := time.NewTimer(xaiVideoPollTimeout)
	defer timer.Stop()

	lastStatus := "created"
	for {
		result, err := s.pollXAIVideoTask(ctx, account, authToken, normalizedBaseURL, requestID)
		if err != nil {
			return s.sendErrorAndEnd(c, err.Error())
		}

		if result.Status != "" {
			lastStatus = result.Status
		}
		if result.VideoURL != "" {
			videoURL, mimeType := s.prepareXAIVideoPlaybackURL(ctx, account, result.VideoURL)
			s.sendEvent(c, TestEvent{
				Type:      "video",
				VideoURL:  videoURL,
				SourceURL: result.VideoURL,
				MimeType:  mimeType,
			})
			s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
			return nil
		}
		if isXAIVideoCompletedStatus(result.Status) {
			return s.sendErrorAndEnd(c, fmt.Sprintf("xAI video task completed but response did not include video URL: %s", truncateString(result.Body, 1024)))
		}
		if isXAIVideoFailedStatus(result.Status) {
			if result.ErrorMessage != "" {
				return s.sendErrorAndEnd(c, fmt.Sprintf("xAI video task failed: %s", result.ErrorMessage))
			}
			return s.sendErrorAndEnd(c, fmt.Sprintf("xAI video task failed with status: %s", result.Status))
		}

		s.sendEvent(c, TestEvent{Type: "status", Text: formatXAIVideoPollStatus(result)})

		select {
		case <-ctx.Done():
			return s.sendErrorAndEnd(c, "xAI video generation cancelled")
		case <-timer.C:
			return s.sendErrorAndEnd(c, fmt.Sprintf("xAI video generation timed out; last status: %s", lastStatus))
		case <-time.After(xaiVideoPollInterval):
		}
	}
}

func (s *AccountTestService) prepareXAIVideoPlaybackURL(ctx context.Context, account *Account, rawVideoURL string) (string, string) {
	videoURL := strings.TrimSpace(rawVideoURL)
	if videoURL == "" {
		return "", "video/mp4"
	}
	if strings.HasPrefix(strings.ToLower(videoURL), "data:") || s.httpUpstream == nil {
		return videoURL, "video/mp4"
	}

	parsed, err := url.Parse(videoURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return videoURL, "video/mp4"
	}

	fetchCtx, cancel := context.WithTimeout(ctx, xaiVideoInlineFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, videoURL, nil)
	if err != nil {
		return videoURL, "video/mp4"
	}
	req.Header.Set("Accept", "video/*,application/octet-stream;q=0.9,*/*;q=0.1")

	resp, err := s.doXAIAccountTestRequest(req, account)
	if err != nil || resp == nil {
		return videoURL, "video/mp4"
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return videoURL, "video/mp4"
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, xaiVideoInlineMaxBytes+1))
	if err != nil || len(body) == 0 || len(body) > xaiVideoInlineMaxBytes {
		return videoURL, "video/mp4"
	}

	mimeType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if index := strings.Index(mimeType, ";"); index >= 0 {
		mimeType = strings.TrimSpace(mimeType[:index])
	}
	if !strings.HasPrefix(strings.ToLower(mimeType), "video/") {
		mimeType = "video/mp4"
	}

	return fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(body)), mimeType
}

func (s *AccountTestService) pollXAIVideoTask(ctx context.Context, account *Account, authToken, normalizedBaseURL, requestID string) (xaiVideoPollResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, xai.BuildVideoPollURL(normalizedBaseURL, requestID), nil)
	if err != nil {
		return xaiVideoPollResult{}, fmt.Errorf("failed to create xAI video poll request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+authToken)

	resp, err := s.doXAIAccountTestRequest(req, account)
	if err != nil {
		return xaiVideoPollResult{}, fmt.Errorf("xAI video poll request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return xaiVideoPollResult{}, fmt.Errorf("failed to read xAI video poll response: %w", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return xaiVideoPollResult{}, fmt.Errorf("xAI video poll returned %d: %s", resp.StatusCode, string(body))
	}

	var raw any
	if err := json.Unmarshal(body, &raw); err != nil {
		return xaiVideoPollResult{}, fmt.Errorf("failed to parse xAI video poll response: %w", err)
	}

	return xaiVideoPollResult{
		Status:       extractJSONStringByKeys(raw, "status", "state"),
		Progress:     extractJSONStringByKeys(raw, "progress", "percent", "percentage"),
		VideoURL:     extractXAIVideoURL(raw),
		ErrorMessage: extractJSONErrorMessage(raw),
		Body:         string(body),
	}, nil
}

func (s *AccountTestService) doXAIAccountTestRequest(req *http.Request, account *Account) (*http.Response, error) {
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	if s.tlsFPProfileService != nil {
		return s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
	}
	return s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
}

func formatXAIVideoPollStatus(result xaiVideoPollResult) string {
	status := strings.TrimSpace(result.Status)
	if status == "" {
		status = "pending"
	}
	if result.Progress != "" {
		return fmt.Sprintf("xAI video task status: %s (%s%%)", status, strings.TrimSuffix(result.Progress, "%"))
	}
	return fmt.Sprintf("xAI video task status: %s", status)
}

func formatXAIImageHTTPError(statusCode int, body []byte) string {
	return formatXAIMediaHTTPError(statusCode, body, "图片生成", "image generation")
}

func formatXAIVideoHTTPError(statusCode int, body []byte) string {
	return formatXAIMediaHTTPError(statusCode, body, "视频生成", "video generation")
}

func formatXAIMediaHTTPError(statusCode int, body []byte, zhOperation, enOperation string) string {
	message := strings.TrimSpace(string(body))
	var raw any
	if len(body) > 0 && json.Unmarshal(body, &raw) == nil {
		parts := make([]string, 0, 2)
		if code := extractJSONStringByKeys(raw, "code"); code != "" {
			parts = append(parts, code)
		}
		if upstreamError := extractJSONErrorMessage(raw); upstreamError != "" {
			parts = append(parts, upstreamError)
		}
		if len(parts) > 0 {
			message = strings.Join(parts, ": ")
		}
	}
	if message == "" {
		message = http.StatusText(statusCode)
	}

	lowerMessage := strings.ToLower(message)
	if statusCode == http.StatusForbidden &&
		(strings.Contains(lowerMessage, "out of credits") ||
			strings.Contains(lowerMessage, "grok subscription") ||
			strings.Contains(lowerMessage, "spending-limit") ||
			strings.Contains(lowerMessage, "does not have permission")) {
		return fmt.Sprintf("xAI %s权限不足或额度用尽（上游 403）：%s。请给该 Grok 账号增加 credits，或升级/开通 SuperGrok 后再测试。", zhOperation, message)
	}

	return fmt.Sprintf("xAI %s returned %d: %s", enOperation, statusCode, message)
}

func isXAIVideoFailedStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "failure", "error", "errored", "cancelled", "canceled", "rejected":
		return true
	default:
		return false
	}
}

func isXAIVideoCompletedStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "complete", "succeeded", "success", "done":
		return true
	default:
		return false
	}
}

func extractXAIVideoURL(value any) string {
	for _, key := range []string{"video_url", "download_url", "mp4_url", "asset_url"} {
		if videoURL := extractJSONStringByKeys(value, key); videoURL != "" {
			return videoURL
		}
	}
	if videoURL := extractNestedJSONURL(value, "video"); videoURL != "" {
		return videoURL
	}
	for _, key := range []string{"videos", "data", "output", "outputs"} {
		if videoURL := extractNestedJSONURL(value, key); videoURL != "" {
			return videoURL
		}
	}
	return extractJSONStringByKeys(value, "url")
}

func extractNestedJSONURL(value any, key string) string {
	if object, ok := value.(map[string]any); ok {
		if nested, exists := object[key]; exists {
			if videoURL := extractJSONStringByKeys(nested, "url", "video_url", "download_url", "mp4_url", "asset_url"); videoURL != "" {
				return videoURL
			}
		}
	}
	return ""
}

func extractJSONErrorMessage(value any) string {
	if object, ok := value.(map[string]any); ok {
		if rawError, exists := object["error"]; exists {
			if message := extractJSONStringByKeys(rawError, "message", "error_description", "error", "code"); message != "" {
				return message
			}
			if text := jsonScalarToString(rawError); text != "" {
				return text
			}
		}
	}
	return extractJSONStringByKeys(value, "error_description", "message")
}

func extractJSONStringByKeys(value any, keys ...string) string {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range keys {
			if raw, exists := typed[key]; exists {
				if text := jsonScalarToString(raw); text != "" {
					return text
				}
			}
		}
		for _, raw := range typed {
			if text := extractJSONStringByKeys(raw, keys...); text != "" {
				return text
			}
		}
	case []any:
		for _, raw := range typed {
			if text := extractJSONStringByKeys(raw, keys...); text != "" {
				return text
			}
		}
	}
	return ""
}

func jsonScalarToString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64, bool:
		return strings.TrimSpace(fmt.Sprintf("%v", typed))
	default:
		return ""
	}
}

func (s *AccountTestService) FetchXAIAvailableModels(ctx context.Context, account *Account) ([]xai.Model, error) {
	if account == nil || !account.IsXAI() {
		return nil, errors.New("account is not an xAI account")
	}
	if s.httpUpstream == nil {
		return nil, errors.New("http upstream is not available")
	}

	authToken, err := s.resolveXAIAuthToken(ctx, account)
	if err != nil {
		return nil, err
	}
	normalizedBaseURL, err := s.validateUpstreamBaseURL(account.GetXAIBaseURL())
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, xai.BuildModelsURL(normalizedBaseURL), nil)
	if err != nil {
		return nil, fmt.Errorf("create xAI models request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+authToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Connection", "Keep-Alive")

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	var resp *http.Response
	if s.tlsFPProfileService != nil {
		resp, err = s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
	} else {
		resp, err = s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	}
	if err != nil {
		return nil, fmt.Errorf("request xAI models: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if readErr != nil {
		return nil, fmt.Errorf("read xAI models response: %w", readErr)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		if resp.StatusCode == http.StatusUnauthorized && s.accountRepo != nil {
			errMsg := fmt.Sprintf("Authentication failed (401): %s", string(body))
			_ = s.accountRepo.SetError(ctx, account.ID, errMsg)
		}
		return nil, fmt.Errorf("xAI models returned %d: %s", resp.StatusCode, string(body))
	}

	models, err := xai.ParseModelList(body)
	if err != nil {
		return nil, err
	}
	if len(models) == 0 {
		return nil, errors.New("xAI models response is empty")
	}
	return models, nil
}

func (s *AccountTestService) resolveXAIAuthToken(ctx context.Context, account *Account) (string, error) {
	if account == nil || !account.IsXAI() {
		return "", errors.New("account is not an xAI account")
	}
	switch account.Type {
	case AccountTypeOAuth:
		if s.xaiTokenProvider != nil {
			token, err := s.xaiTokenProvider.GetAccessToken(ctx, account)
			if err != nil {
				return "", fmt.Errorf("failed to get access token: %w", err)
			}
			if strings.TrimSpace(token) != "" {
				return strings.TrimSpace(token), nil
			}
		}
		authToken := strings.TrimSpace(account.GetXAIAccessToken())
		if authToken == "" {
			return "", errors.New("no access token available")
		}
		return authToken, nil
	case AccountTypeAPIKey:
		authToken := strings.TrimSpace(account.GetXAIApiKey())
		if authToken == "" {
			return "", errors.New("no API key available")
		}
		return authToken, nil
	default:
		return "", fmt.Errorf("unsupported account type: %s", account.Type)
	}
}

// testGeminiAccountConnection tests a Gemini account's connection
func (s *AccountTestService) testGeminiAccountConnection(c *gin.Context, account *Account, modelID string, prompt string) error {
	ctx := c.Request.Context()

	// Determine the model to use
	testModelID := modelID
	if testModelID == "" {
		testModelID = geminicli.DefaultTestModel
	}

	// For static upstream credentials with model mapping, map the model
	if account.Type == AccountTypeAPIKey || account.Type == AccountTypeServiceAccount {
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
	case AccountTypeServiceAccount:
		req, err = s.buildGeminiServiceAccountRequest(ctx, account, testModelID, payload)
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

func (s *AccountTestService) buildGeminiServiceAccountRequest(ctx context.Context, account *Account, modelID string, payload []byte) (*http.Request, error) {
	if s.geminiTokenProvider == nil {
		return nil, fmt.Errorf("gemini token provider not configured")
	}
	accessToken, err := s.geminiTokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("failed to get service account access token: %w", err)
	}
	fullURL, err := buildVertexGeminiURL(account.VertexProjectID(), account.VertexLocation(modelID), modelID, "streamGenerateContent", true)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	return req, nil
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

func createOpenAIChatCompletionsTestPayload(modelID string, prompt string) map[string]any {
	testPrompt := strings.TrimSpace(prompt)
	if testPrompt == "" {
		testPrompt = "hi"
	}

	return map[string]any{
		"model": modelID,
		"messages": []map[string]any{
			{
				"role":    "user",
				"content": testPrompt,
			},
		},
		"stream": true,
	}
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

// processOpenAIChatCompletionsStream processes SSE chunks from the
// OpenAI-compatible Chat Completions API.
func (s *AccountTestService) processOpenAIChatCompletionsStream(c *gin.Context, body io.Reader) error {
	reader := bufio.NewReader(body)
	seenJSON := false
	seenFinish := false

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				if seenFinish {
					s.sendEvent(c, TestEvent{Type: "status", Text: "已通过 /v1/chat/completions 验证"})
					s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
					return nil
				}
				if seenJSON {
					return s.sendErrorAndEnd(c, "Chat Completions stream from /v1/chat/completions ended before [DONE]")
				}
				return s.sendErrorAndEnd(c, "Invalid Chat Completions response from /v1/chat/completions: expected SSE JSON data")
			}
			return s.sendErrorAndEnd(c, fmt.Sprintf("Chat Completions stream read error from /v1/chat/completions: %s", err.Error()))
		}

		line = strings.TrimSpace(line)
		if line == "" || !sseDataPrefix.MatchString(line) {
			continue
		}

		jsonStr := sseDataPrefix.ReplaceAllString(line, "")
		if jsonStr == "[DONE]" {
			s.sendEvent(c, TestEvent{Type: "status", Text: "已通过 /v1/chat/completions 验证"})
			s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
			return nil
		}

		var data map[string]any
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			return s.sendErrorAndEnd(c, "Invalid Chat Completions response from /v1/chat/completions: expected JSON data")
		}
		seenJSON = true

		if errData, ok := data["error"].(map[string]any); ok {
			errorMsg := "Chat Completions API (/v1/chat/completions) returned an error"
			if msg, ok := errData["message"].(string); ok && msg != "" {
				errorMsg = msg
			}
			return s.sendErrorAndEnd(c, fmt.Sprintf("Chat Completions API (/v1/chat/completions) error: %s", errorMsg))
		}

		choices, ok := data["choices"].([]any)
		if !ok {
			continue
		}
		for _, choiceValue := range choices {
			choice, ok := choiceValue.(map[string]any)
			if !ok {
				continue
			}
			if delta, ok := choice["delta"].(map[string]any); ok {
				if text, ok := delta["content"].(string); ok && text != "" {
					s.sendEvent(c, TestEvent{Type: "content", Text: text})
				}
			}
			if message, ok := choice["message"].(map[string]any); ok {
				if text, ok := message["content"].(string); ok && text != "" {
					s.sendEvent(c, TestEvent{Type: "content", Text: text})
				}
			}
			if finishReason, ok := choice["finish_reason"].(string); ok && finishReason != "" {
				seenFinish = true
			}
		}
	}
}

// processOpenAIStream processes the SSE stream from OpenAI Responses API
func (s *AccountTestService) processOpenAIStream(c *gin.Context, body io.Reader) error {
	reader := bufio.NewReader(body)
	seenCompleted := false

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				if seenCompleted {
					s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
					return nil
				}
				return s.sendErrorAndEnd(c, "Stream ended before response.completed")
			}
			return s.sendErrorAndEnd(c, fmt.Sprintf("Stream read error: %s", err.Error()))
		}

		line = strings.TrimSpace(line)
		if line == "" || !sseDataPrefix.MatchString(line) {
			continue
		}

		jsonStr := sseDataPrefix.ReplaceAllString(line, "")
		if jsonStr == "[DONE]" {
			if seenCompleted {
				s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
				return nil
			}
			return s.sendErrorAndEnd(c, "Stream ended before response.completed")
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
		case "response.completed", "response.done":
			s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
			return nil
		case "response.failed":
			errorMsg := "OpenAI response failed"
			if responseData, ok := data["response"].(map[string]any); ok {
				if errData, ok := responseData["error"].(map[string]any); ok {
					if msg, ok := errData["message"].(string); ok && msg != "" {
						errorMsg = msg
					}
				}
			}
			return s.sendErrorAndEnd(c, errorMsg)
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

// testOpenAIImageAPIKey tests OpenAI image generation using an API Key account.
func (s *AccountTestService) testOpenAIImageAPIKey(c *gin.Context, ctx context.Context, account *Account, modelID, prompt string) error {
	authToken := account.GetOpenAIApiKey()
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
	apiURL := buildOpenAIImagesURL(normalizedBaseURL, openAIImagesGenerationsEndpoint)

	// Set SSE headers
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	s.sendEvent(c, TestEvent{Type: "test_start", Model: modelID})

	payload := map[string]any{
		"model":           modelID,
		"prompt":          prompt,
		"n":               1,
		"response_format": "b64_json",
	}
	payloadBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create request")
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+authToken)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Request failed: %s", err.Error()))
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Failed to read response: %s", err.Error()))
	}

	if resp.StatusCode != http.StatusOK {
		return s.sendErrorAndEnd(c, fmt.Sprintf("API returned %d: %s", resp.StatusCode, string(body)))
	}

	// Parse {"data": [{"b64_json": "...", "revised_prompt": "..."}]}
	var result struct {
		Data []struct {
			B64JSON       string `json:"b64_json"`
			RevisedPrompt string `json:"revised_prompt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Failed to parse response: %s", err.Error()))
	}

	if len(result.Data) == 0 {
		return s.sendErrorAndEnd(c, "No images returned from API")
	}

	for _, item := range result.Data {
		if item.RevisedPrompt != "" {
			s.sendEvent(c, TestEvent{Type: "content", Text: item.RevisedPrompt})
		}
		if item.B64JSON != "" {
			s.sendEvent(c, TestEvent{
				Type:     "image",
				ImageURL: "data:image/png;base64," + item.B64JSON,
				MimeType: "image/png",
			})
		}
	}

	s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
	return nil
}

// testOpenAIImageOAuth tests OpenAI image generation using an OAuth account via Codex /responses API.
func (s *AccountTestService) testOpenAIImageOAuth(c *gin.Context, ctx context.Context, account *Account, modelID, prompt string) error {
	authToken := account.GetOpenAIAccessToken()
	if authToken == "" {
		return s.sendErrorAndEnd(c, "No access token available")
	}

	// Set SSE headers
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	s.sendEvent(c, TestEvent{Type: "test_start", Model: modelID})
	s.sendEvent(c, TestEvent{Type: "content", Text: "Calling Codex /responses image tool...\n"})

	parsed := &OpenAIImagesRequest{
		Endpoint: openAIImagesGenerationsEndpoint,
		Model:    strings.TrimSpace(modelID),
		Prompt:   prompt,
	}
	applyOpenAIImagesDefaults(parsed)

	responsesBody, err := buildOpenAIImagesResponsesRequest(parsed, parsed.Model)
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Failed to build image request: %s", err.Error()))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, chatgptCodexAPIURL, bytes.NewReader(responsesBody))
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create request")
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	req.Host = "chatgpt.com"
	req.Header.Set("Authorization", "Bearer "+authToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	req.Header.Set("originator", "opencode")
	if customUA := strings.TrimSpace(account.GetOpenAIUserAgent()); customUA != "" {
		req.Header.Set("User-Agent", customUA)
	} else {
		req.Header.Set("User-Agent", codexCLIUserAgent)
	}
	if chatgptAccountID := strings.TrimSpace(account.GetChatGPTAccountID()); chatgptAccountID != "" {
		req.Header.Set("chatgpt-account-id", chatgptAccountID)
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Responses API request failed: %s", err.Error()))
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		message := strings.TrimSpace(extractUpstreamErrorMessage(body))
		if message == "" {
			message = fmt.Sprintf("Responses API returned %d", resp.StatusCode)
		}
		return s.sendErrorAndEnd(c, message)
	}

	return s.streamOpenAIImageOAuthTestResponse(c, resp.Body)
}

func (s *AccountTestService) streamOpenAIImageOAuthTestResponse(c *gin.Context, body io.Reader) error {
	type readEvent struct {
		line string
		err  error
	}

	events := make(chan readEvent, 16)
	done := make(chan struct{})
	go func() {
		defer close(events)
		reader := bufio.NewReader(body)
		for {
			line, err := reader.ReadString('\n')
			if line != "" {
				select {
				case events <- readEvent{line: line}:
				case <-done:
					return
				}
			}
			if err == io.EOF {
				return
			}
			if err != nil {
				select {
				case events <- readEvent{err: err}:
				case <-done:
				}
				return
			}
		}
	}()
	defer close(done)

	var (
		acc          openAISSEDataAccumulator
		responseMeta openAIResponsesImageResult
		emitted      = make(map[string]struct{})
		streamErr    error
		completed    bool
		imageCount   int
	)

	emitImage := func(item openAIResponsesImageResult) {
		key := openAIResponsesImageResultKey("", item)
		if key != "" {
			if _, exists := emitted[key]; exists {
				return
			}
			emitted[key] = struct{}{}
		}
		if item.RevisedPrompt != "" {
			s.sendEvent(c, TestEvent{Type: "content", Text: item.RevisedPrompt})
		}
		mimeType := openAIImageOutputMIMEType(item.OutputFormat)
		s.sendEvent(c, TestEvent{
			Type:     "image",
			ImageURL: "data:" + mimeType + ";base64," + item.Result,
			MimeType: mimeType,
		})
		imageCount++
	}

	processPayload := func(payload []byte) {
		if streamErr != nil || completed || !json.Valid(payload) {
			return
		}
		var event struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(payload, &event); err != nil {
			return
		}

		if meta, _, ok := extractOpenAIResponsesImageMetaFromLifecycleEvent(payload); ok {
			mergeOpenAIResponsesImageMeta(&responseMeta, meta)
		}

		switch event.Type {
		case "response.output_item.done":
			item, _, ok, err := extractOpenAIImageFromResponsesOutputItemDone(payload)
			if err != nil {
				streamErr = err
				return
			}
			if !ok {
				return
			}
			mergeOpenAIResponsesImageMeta(&item, responseMeta)
			emitImage(item)
		case "response.completed":
			results, _, _, firstMeta, err := extractOpenAIImagesFromResponsesCompleted(payload)
			if err != nil {
				streamErr = err
				return
			}
			completed = true
			mergeOpenAIResponsesImageMeta(&responseMeta, firstMeta)
			for _, item := range results {
				mergeOpenAIResponsesImageMeta(&item, responseMeta)
				emitImage(item)
			}
		case "error", "response.failed":
			if upstreamErr := openAIImagesUpstreamErrorFromSSEPayload(payload); upstreamErr != nil {
				streamErr = errors.New(upstreamErr.clientMessage())
			}
		}
	}

	keepalive := time.NewTicker(20 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				acc.Flush(processPayload)
				if streamErr != nil {
					return s.sendErrorAndEnd(c, streamErr.Error())
				}
				if imageCount == 0 {
					return s.sendErrorAndEnd(c, "No images returned from responses API")
				}
				s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
				return nil
			}
			if ev.err != nil {
				acc.Flush(processPayload)
				if streamErr != nil {
					return s.sendErrorAndEnd(c, streamErr.Error())
				}
				return s.sendErrorAndEnd(c, fmt.Sprintf("Failed to read image response: %s", ev.err.Error()))
			}
			acc.AddLine(ev.line, processPayload)
			if streamErr != nil {
				return s.sendErrorAndEnd(c, streamErr.Error())
			}
			if completed && imageCount > 0 {
				s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
				return nil
			}
		case <-keepalive.C:
			s.sendEvent(c, TestEvent{Type: "content", Text: "等待 OpenAI 生图结果...\n"})
		case <-c.Request.Context().Done():
			return c.Request.Context().Err()
		}
	}
}

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

	testErr := s.TestAccountConnection(ginCtx, accountID, modelID, "", AccountTestModeDefault)

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
