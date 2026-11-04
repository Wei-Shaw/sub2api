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
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/shared/claude"
	"github.com/Wei-Shaw/sub2api/internal/shared/geminicli"
	"github.com/Wei-Shaw/sub2api/internal/shared/openai"
	"github.com/Wei-Shaw/sub2api/internal/shared/openai_compat"
	"github.com/Wei-Shaw/sub2api/internal/shared/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/shared/urlvalidator"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// sseDataPrefix matches SSE data lines with optional whitespace after colon.
// Some upstream APIs return non-standard "data:" without space (should be "data: ").
var sseDataPrefix = regexp.MustCompile(`^data:\s*`)

const (
	testClaudeAPIURL            = "https://api.anthropic.com/v1/messages?beta=true"
	chatgptCodexAPIURL          = "https://chatgpt.com/backend-api/codex/responses"
	defaultAntigravityTestModel = "claude-sonnet-4-6"
)

type accountTestEffortContextKey struct{}

func withAccountTestEffort(ctx context.Context, effort string) context.Context {
	return context.WithValue(ctx, accountTestEffortContextKey{}, strings.ToLower(strings.TrimSpace(effort)))
}

func accountTestEffort(ctx context.Context) string {
	value, _ := ctx.Value(accountTestEffortContextKey{}).(string)
	return value
}

// TestEvent represents a SSE event for account testing
type TestEvent struct {
	Type            string `json:"type"`
	Text            string `json:"text,omitempty"`
	Model           string `json:"model,omitempty"`
	Status          string `json:"status,omitempty"`
	Code            string `json:"code,omitempty"`
	ImageURL        string `json:"image_url,omitempty"`
	MimeType        string `json:"mime_type,omitempty"`
	Data            any    `json:"data,omitempty"`
	Success         bool   `json:"success,omitempty"`
	Error           string `json:"error,omitempty"`
	ConversationID  string `json:"conversation_id,omitempty"`
	ResponseID      string `json:"response_id,omitempty"`
	ReasoningTokens *int64 `json:"reasoning_tokens,omitempty"`
}

const (
	defaultGeminiTextTestPrompt  = "hi"
	defaultGeminiImageTestPrompt = "Generate a cute orange cat astronaut sticker on a clean pastel background."
	defaultOpenAIImageTestPrompt = "Generate a cute orange cat astronaut sticker on a clean pastel background."
)

// isOpenAIImageModel checks if the model is an OpenAI image generation model (e.g. gpt-image-2).
func isOpenAIImageModel(model string) bool {
	return strings.HasPrefix(strings.ToLower(model), "gpt-image-")
}

// AccountTestService handles account testing operations
type AccountTestService struct {
	accountRepo               AccountRepository
	geminiTokenProvider       *GeminiTokenProvider
	claudeTokenProvider       *ClaudeTokenProvider
	grokTokenProvider         *GrokTokenProvider
	antigravityGatewayService *AntigravityGatewayService
	httpUpstream              HTTPUpstream
	cfg                       *config.Config
	tlsFPProfileService       *TLSFingerprintProfileService
	openAIModelsManifest      openAIModelsManifestFetcher
	upstreamModelsCache       OAuthModelSyncModelsCacheInvalidator
	customModelCapabilities   CustomModelCapabilityResolver
	agentIdentityTaskMu       sync.Mutex
	agentIdentityWS           agentIdentityWSConnectionInvalidator
	oauthGatewayLimiter       *OpenAIGatewayService
	settingService            *SettingService
}

// SetSettingService wires the persisted gateway settings used by OpenAI OAuth
// account probes. Kept as a setter so focused test constructors remain stable.
func (s *AccountTestService) SetSettingService(settingService *SettingService) {
	if s != nil {
		s.settingService = settingService
	}
}

func (s *AccountTestService) doAccountTestHTTPUpstreamWithTLS(req *http.Request, proxyURL string, account *Account, profile *tlsfingerprint.Profile) (*http.Response, error) {
	if s.oauthGatewayLimiter != nil && isOpenAIOAuthGatewayModelRequest(req, account) {
		if err := s.oauthGatewayLimiter.admitOpenAIOAuthGatewayModelRequest(req.Context(), account); err != nil {
			return nil, err
		}
	}
	return doAccountHTTPUpstreamWithTLS(s.httpUpstream, req, proxyURL, account, profile)
}

func (s *AccountTestService) doAccountTestHTTPUpstream(req *http.Request, proxyURL string, account *Account) (*http.Response, error) {
	if s.oauthGatewayLimiter != nil && isOpenAIOAuthGatewayModelRequest(req, account) {
		if err := s.oauthGatewayLimiter.admitOpenAIOAuthGatewayModelRequest(req.Context(), account); err != nil {
			return nil, err
		}
	}
	return doAccountHTTPUpstream(s.httpUpstream, req, proxyURL, account)
}

// NewAccountTestService creates a new AccountTestService
func NewAccountTestService(
	accountRepo AccountRepository,
	geminiTokenProvider *GeminiTokenProvider,
	claudeTokenProvider *ClaudeTokenProvider,
	grokTokenProvider *GrokTokenProvider,
	antigravityGatewayService *AntigravityGatewayService,
	httpUpstream HTTPUpstream,
	cfg *config.Config,
	tlsFPProfileService *TLSFingerprintProfileService,
) *AccountTestService {
	return &AccountTestService{
		accountRepo:               accountRepo,
		geminiTokenProvider:       geminiTokenProvider,
		claudeTokenProvider:       claudeTokenProvider,
		grokTokenProvider:         grokTokenProvider,
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
// mode is optional. OpenAI tests can follow account routing or explicitly probe
// /responses, /chat/completions, or /responses/compact.
func (s *AccountTestService) TestAccountConnection(c *gin.Context, accountID int64, modelID string, prompt string, mode string) error {
	initAccountTestLogger(c, accountID, modelID, mode)
	ctx := c.Request.Context()

	// Get account
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return s.sendErrorAndEnd(c, "Account not found")
	}
	bindAccountTestPlatform(c, account)

	// Synthetic UI load-test accounts exercise the real SSE parsing and modal
	// interactions, but intentionally do not send their placeholder credentials
	// to an upstream provider.
	if account.IsSyntheticUITest() {
		testModelID := modelID
		if testModelID == "" {
			testModelID = claude.DefaultTestModel
		}
		s.sendEvent(c, TestEvent{Type: "test_start", Model: testModelID})
		s.sendEvent(c, TestEvent{Type: "content", Text: "Synthetic Anthropic OAuth account is healthy and interactive."})
		s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
		return nil
	}

	// Route to platform-specific test method
	if account.IsOpenAI() || account.IsCNProvider() {
		return s.testOpenAIAccountConnection(c, account, modelID, prompt, normalizeAccountTestMode(mode))
	}

	if account.IsGemini() {
		return s.testGeminiAccountConnection(c, account, modelID, prompt)
	}

	if account.Platform == PlatformGrok {
		return s.testGrokAccountConnection(c, account, modelID)
	}

	if account.Platform == PlatformAntigravity {
		return s.routeAntigravityTest(c, account, modelID, prompt)
	}

	if account.IsTypeSafe() {
		return s.testTypeSafeAccountConnection(c, account, prompt)
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
	var apiURL string

	if account.IsOAuth() {
		apiURL = testClaudeAPIURL
		authToken = account.GetCredential("access_token")
		if authToken == "" {
			return s.sendErrorAndEnd(c, "No access token available")
		}
	} else if account.Type == "apikey" {
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
	if account.IsOAuth() {
		req.Header.Set("anthropic-beta", claude.DefaultBetaHeader)
		req.Header.Set("Authorization", "Bearer "+authToken)
	} else {
		req.Header.Set("anthropic-beta", claude.APIKeyBetaHeader)
		setAnthropicAPIKeyAuthHeader(req.Header, account, authToken)
	}

	// 账号级请求头覆写：测试请求与真实转发保持一致的最终头
	account.ApplyHeaderOverrides(req.Header)

	// Get proxy URL
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	resp, err := s.doAccountTestHTTPUpstreamWithTLS(req, proxyURL, account, s.tlsFPProfileService.ResolveTLSProfile(account))
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

	resp, err := s.doAccountTestHTTPUpstreamWithTLS(req, proxyURL, account, s.tlsFPProfileService.ResolveTLSProfile(account))
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

	resp, err := s.doAccountTestHTTPUpstreamWithTLS(req, proxyURL, account, nil)
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

	// Align test routing with gateway behavior. Native remote compaction v2 rides
	// the ordinary /responses endpoint, so legacy compact-only model mappings do
	// not apply to this probe.
	testModelID = account.GetMappedModel(testModelID)
	if account.IsOpenCodeGo() && IsOpenCodeUnsupportedModel(testModelID) {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Model %q is not supported on OpenCode standard gateway; use its native Gemini or System One endpoint", testModelID))
	}
	if mode == AccountTestModeCompact {
		return s.testOpenAICompactConnection(c, account, testModelID)
	}

	isImageModel := isOpenAIImageModel(testModelID)
	if !isImageModel && s.customModelCapabilities != nil {
		configured, err := s.customModelCapabilities.HasCapability(ctx, testModelID, "image")
		if err != nil {
			return s.sendErrorAndEnd(c, fmt.Sprintf("Failed to resolve custom model capabilities: %s", err.Error()))
		}
		isImageModel = configured
	}

	// Route to image generation test if a built-in or configured image model is selected.
	if isImageModel {
		imagePrompt := strings.TrimSpace(prompt)
		if imagePrompt == "" {
			imagePrompt = defaultOpenAIImageTestPrompt
		}
		if account.Type == "apikey" {
			return s.testOpenAIImageAPIKey(c, ctx, account, testModelID, imagePrompt)
		}
		return s.testOpenAIImageOAuth(c, ctx, account, testModelID, imagePrompt)
	}

	credentialAccount := account
	if account.IsCredentialShadow() {
		resolved, err := resolveCredentialAccount(ctx, s.accountRepo, account)
		if err != nil {
			return s.sendErrorAndEnd(c, err.Error())
		}
		credentialAccount = resolved
	}

	// Determine authentication method and API URL
	var authToken string
	var apiURL string
	var isOAuth bool
	useOfficialOAuthEndpoint := false

	if credentialAccount.IsOAuth() {
		if mode == AccountTestModeChatCompletions {
			return s.sendErrorAndEnd(c, "Chat Completions testing is only supported for OpenAI API key accounts")
		}
		isOAuth = true
		// Agent Identity signs each request and does not retain the OAuth token.
		if !credentialAccount.IsOpenAIAgentIdentity() {
			authToken = credentialAccount.GetOpenAIAccessToken()
		}
		if authToken == "" && !credentialAccount.IsOpenAIAgentIdentity() {
			return s.sendErrorAndEnd(c, "No access token available")
		}

		baseURL, official, relayErr := resolveOpenAIOAuthCodexBaseURL(ctx, s.settingService, s.cfg, credentialAccount)
		if relayErr != nil {
			return s.sendErrorAndEnd(c, fmt.Sprintf("Invalid OpenAI OAuth Codex relay URL: %s", relayErr.Error()))
		}
		apiURL = buildOpenAIOAuthCodexResponsesURL(baseURL)
		useOfficialOAuthEndpoint = official
	} else if credentialAccount.Type == "apikey" {
		// API Key - use Platform API
		authToken = credentialAccount.GetOpenAIProtocolAPIKey()
		if authToken == "" {
			return s.sendErrorAndEnd(c, "No API key available")
		}

		baseURL := credentialAccount.GetOpenAIBaseURL()
		if baseURL == "" {
			baseURL = "https://api.openai.com"
		}
		normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
		if err != nil {
			return s.sendErrorAndEnd(c, fmt.Sprintf("Invalid base URL: %s", err.Error()))
		}
		if account.IsCNProvider() || mode == AccountTestModeChatCompletions ||
			(mode == AccountTestModeDefault && !openai_compat.ShouldUseResponsesAPI(account.Extra)) {
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

	// Create OpenAI Responses API payload. OAuth accounts use ChatGPT Codex
	// upstream and must apply the same model normalization as real forwarding.
	upstreamTestModelID := testModelID
	if isOAuth {
		upstreamTestModelID = normalizeOpenAIModelForUpstream(credentialAccount, testModelID)
	}
	payload := createOpenAITestPayload(upstreamTestModelID, isOAuth, prompt)
	if effort := accountTestEffort(ctx); effort != "" {
		if isOAuth || mode != AccountTestModeChatCompletions {
			payload["reasoning"] = map[string]any{"effort": effort}
		}
		if mode == AccountTestModeChatCompletions {
			payload["reasoning_effort"] = effort
		}
	}
	payloadBytes, _ := json.Marshal(payload)

	// Send test_start event once. A task-invalid Agent Identity response may
	// restart this probe after registering a replacement task.
	if !agentIdentityTaskRecoveryWasTried(ctx) {
		s.sendEvent(c, TestEvent{Type: "test_start", Model: testModelID})
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create request")
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))

	// Set common headers
	req.Header.Set("Content-Type", "application/json")
	if !isOAuth {
		applyOpenAICodexProbeHeaders(req.Header)
	}
	if credentialAccount.IsOpenAIAgentIdentity() {
		authHeaders, authErr := buildAgentIdentityAuthenticationHeaders(ctx, s.accountRepo, s.agentIdentityWS, &s.agentIdentityTaskMu, credentialAccount, s.cfg)
		if authErr != nil {
			_ = setOpenAIAgentIdentityAccountError(ctx, s.accountRepo, s.agentIdentityWS, credentialAccount, "Agent Identity authentication failed: "+authErr.Error())
			return s.sendErrorAndEnd(c, "Failed to build Agent Identity authentication")
		}
		for key, values := range authHeaders {
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}
	} else {
		req.Header.Set("Authorization", "Bearer "+authToken)
	}

	// Set OAuth-specific headers for ChatGPT internal API
	if isOAuth {
		if useOfficialOAuthEndpoint {
			req.Host = "chatgpt.com"
		}
		req.Header.Set("accept", "text/event-stream")
		req.Header.Set("OpenAI-Beta", "responses=experimental")
		req.Header.Set("Originator", "codex_cli_rs")
		if customUA := strings.TrimSpace(credentialAccount.GetOpenAIUserAgent()); customUA != "" {
			req.Header.Set("User-Agent", customUA)
		} else {
			req.Header.Set("User-Agent", codexCLIUserAgent)
		}
		setOpenAIChatGPTAccountHeaders(req.Header, credentialAccount)
		// 与真实转发一致：originator 与最终 User-Agent 首段配套，否则上游 404（issue #3901）。
		enforceCodexIdentityHeadersWithUA(req.Header, credentialAccount.GetOpenAIUserAgent())
	}

	// 账号级请求头覆写：测试请求与真实转发保持一致的最终头
	credentialAccount.ApplyHeaderOverrides(req.Header)
	if state := accountTestTurnState(ctx); isOAuth && state != "" {
		req.Header.Set(openAICodexTurnStateHeader, state)
	}

	// Get proxy URL
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	resp, err := s.doAccountTestHTTPUpstreamWithTLS(req, proxyURL, account, s.tlsFPProfileService.ResolveTLSProfile(account))
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Request failed: %s", err.Error()))
	}
	defer func() { _ = resp.Body.Close() }()
	captureAccountTestTurnState(ctx, resp.Header)

	if isOAuth && s.accountRepo != nil {
		if updates, err := extractOpenAICodexProbeUpdates(resp); err == nil && len(updates) > 0 {
			_ = s.accountRepo.UpdateExtra(ctx, account.ID, updates)
			mergeAccountExtra(account, updates)
		}
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		body = redactAgentIdentitySensitiveBodyForAccount(ctx, s.accountRepo, credentialAccount, body)
		if !agentIdentityTaskRecoveryWasTried(ctx) && credentialAccount.IsOpenAIAgentIdentity() && isAgentIdentityTaskInvalidHTTPResponse(resp.StatusCode, body) {
			expectedTaskID := credentialAccount.GetCredential("task_id")
			if err := ensureAgentIdentityTaskForAccount(ctx, s.accountRepo, s.agentIdentityWS, &s.agentIdentityTaskMu, credentialAccount, expectedTaskID, s.cfg); err != nil {
				_ = setOpenAIAgentIdentityAccountError(ctx, s.accountRepo, s.agentIdentityWS, credentialAccount, "Agent Identity task recovery failed: "+err.Error())
				return s.sendErrorAndEnd(c, fmt.Sprintf("Agent Identity task recovery failed: %s", err.Error()))
			}
			c.Request = c.Request.WithContext(markAgentIdentityTaskRecoveryTried(ctx))
			return s.testOpenAIAccountConnection(c, account, modelID, prompt, mode)
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			s.reconcileOpenAI429State(ctx, account, resp.Header, body)
		}
		if resp.StatusCode == http.StatusForbidden && credentialAccount.IsOpenAIAgentIdentity() {
			_ = setOpenAIAgentIdentityAccountError(ctx, s.accountRepo, s.agentIdentityWS, credentialAccount, buildForbiddenErrorMessage(
				"Agent Identity access forbidden (403):", extractUpstreamErrorMessage(body), body, "credentials require account action",
			))
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
	if collect, _ := ctx.Value(accountTestUsageContextKey{}).(bool); collect {
		payload["stream_options"] = map[string]any{"include_usage": true}
	}
	if effort := accountTestEffort(ctx); effort != "" {
		payload["reasoning_effort"] = effort
	}
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

	// 账号级请求头覆写：测试请求与真实转发保持一致的最终头
	account.ApplyHeaderOverrides(req.Header)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	resp, err := s.doAccountTestHTTPUpstreamWithTLS(req, proxyURL, account, s.tlsFPProfileService.ResolveTLSProfile(account))
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

// testOpenAICompactConnection probes native remote compaction v2 (streaming
// /responses with a compaction_trigger input item) and persists the capability
// state on the account. The legacy unary /responses/compact endpoint is no
// longer used.
func (s *AccountTestService) testOpenAICompactConnection(c *gin.Context, account *Account, testModelID string) error {
	ctx := c.Request.Context()
	credentialAccount := account
	if account.IsShadow() {
		resolved, err := resolveCredentialAccount(ctx, s.accountRepo, account)
		if err != nil {
			return s.sendErrorAndEnd(c, "Failed to resolve account credentials")
		}
		credentialAccount = resolved
	}

	authToken := ""
	apiURL := ""
	isOAuth := false
	useOfficialOAuthEndpoint := false

	switch {
	case credentialAccount.IsOAuth():
		isOAuth = true
		if !credentialAccount.IsOpenAIAgentIdentity() {
			authToken = credentialAccount.GetOpenAIAccessToken()
		}
		if authToken == "" && !credentialAccount.IsOpenAIAgentIdentity() {
			return s.sendErrorAndEnd(c, "No access token available")
		}
		baseURL, official, relayErr := resolveOpenAIOAuthCodexBaseURL(ctx, s.settingService, s.cfg, credentialAccount)
		if relayErr != nil {
			return s.sendErrorAndEnd(c, fmt.Sprintf("Invalid OpenAI OAuth Codex relay URL: %s", relayErr.Error()))
		}
		apiURL = buildOpenAIOAuthCodexResponsesURL(baseURL)
		useOfficialOAuthEndpoint = official
	case account.Type == AccountTypeAPIKey:
		authToken = account.GetOpenAIProtocolAPIKey()
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
		apiURL = buildOpenAIResponsesURL(normalizedBaseURL)
	default:
		return s.sendErrorAndEnd(c, fmt.Sprintf("Unsupported account type: %s", account.Type))
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	if isOAuth {
		testModelID = normalizeOpenAIModelForUpstream(credentialAccount, testModelID)
	}
	payloadBytes, _ := json.Marshal(createOpenAICompactProbePayload(testModelID, isOAuth))
	if !agentIdentityTaskRecoveryWasTried(ctx) {
		s.sendEvent(c, TestEvent{Type: "test_start", Model: testModelID})
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create request")
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	ensureOpenAIRemoteCompactionV2BetaFeature(req.Header)
	if credentialAccount.IsOpenAIAgentIdentity() {
		authHeaders, authErr := buildAgentIdentityAuthenticationHeaders(ctx, s.accountRepo, s.agentIdentityWS, &s.agentIdentityTaskMu, credentialAccount, s.cfg)
		if authErr != nil {
			_ = setOpenAIAgentIdentityAccountError(ctx, s.accountRepo, s.agentIdentityWS, credentialAccount, "Agent Identity authentication failed: "+authErr.Error())
			return s.sendErrorAndEnd(c, "Failed to build Agent Identity authentication")
		}
		for key, values := range authHeaders {
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}
	} else {
		req.Header.Set("Authorization", "Bearer "+authToken)
	}
	applyOpenAICodexProbeHeaders(req.Header)
	probeSessionID := compactProbeSessionID(account.ID)
	req.Header.Set("Session_ID", probeSessionID)
	req.Header.Set("Conversation_ID", probeSessionID)

	if isOAuth {
		if useOfficialOAuthEndpoint {
			req.Host = "chatgpt.com"
		}
		setOpenAIChatGPTAccountHeaders(req.Header, credentialAccount)
		if fpIDs := resolveCodexFingerprintIDsFromRequest(account, req.Header); fpIDs != nil {
			applyCodexFingerprintHeaders(req.Header, fpIDs)
		}
	}

	// 账号级请求头覆写：测试请求与真实转发保持一致的最终头
	account.ApplyHeaderOverrides(req.Header)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	resp, err := s.doAccountTestHTTPUpstreamWithTLS(req, proxyURL, account, s.tlsFPProfileService.ResolveTLSProfile(account))
	if err != nil {
		if s.accountRepo != nil {
			updates := buildOpenAICompactProbeExtraUpdates(nil, nil, err, false, time.Now())
			_ = s.accountRepo.UpdateExtra(ctx, account.ID, updates)
			mergeAccountExtra(account, updates)
		}
		return s.sendErrorAndEnd(c, fmt.Sprintf("Request failed: %s", err.Error()))
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	body = redactAgentIdentitySensitiveBodyForAccount(ctx, s.accountRepo, credentialAccount, body)
	compactionFound := openAICompactProbeFoundCompactionItem(body)
	if !agentIdentityTaskRecoveryWasTried(ctx) && credentialAccount.IsOpenAIAgentIdentity() && isAgentIdentityTaskInvalidHTTPResponse(resp.StatusCode, body) {
		expectedTaskID := credentialAccount.GetCredential("task_id")
		if err := ensureAgentIdentityTaskForAccount(ctx, s.accountRepo, s.agentIdentityWS, &s.agentIdentityTaskMu, credentialAccount, expectedTaskID, s.cfg); err != nil {
			_ = setOpenAIAgentIdentityAccountError(ctx, s.accountRepo, s.agentIdentityWS, credentialAccount, "Agent Identity task recovery failed: "+err.Error())
			return s.sendErrorAndEnd(c, fmt.Sprintf("Agent Identity task recovery failed: %s", err.Error()))
		}
		c.Request = c.Request.WithContext(markAgentIdentityTaskRecoveryTried(ctx))
		return s.testOpenAICompactConnection(c, account, testModelID)
	}

	if s.accountRepo != nil {
		updates := buildOpenAICompactProbeExtraUpdates(resp, body, nil, compactionFound, time.Now())
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

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		if resp.StatusCode == http.StatusForbidden && credentialAccount.IsOpenAIAgentIdentity() {
			_ = setOpenAIAgentIdentityAccountError(ctx, s.accountRepo, s.agentIdentityWS, credentialAccount, buildForbiddenErrorMessage(
				"Agent Identity access forbidden (403):", extractUpstreamErrorMessage(body), body, "credentials require account action",
			))
		}
		if resp.StatusCode == http.StatusUnauthorized && s.accountRepo != nil {
			errMsg := fmt.Sprintf("Authentication failed (401): %s", string(body))
			_ = s.accountRepo.SetError(ctx, account.ID, errMsg)
		}
		return s.sendErrorAndEnd(c, fmt.Sprintf("API returned %d: %s", resp.StatusCode, string(body)))
	}

	if !compactionFound {
		return s.sendErrorAndEnd(c, "Upstream returned 2xx without a compaction output item (native remote compaction v2 unsupported on this chain)")
	}

	s.sendEvent(c, TestEvent{Type: "content", Text: "Compact probe succeeded (native remote compaction v2)"})
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

// geminiDefaultTestModel preserves the legacy Google One probe model.
func geminiDefaultTestModel(account *Account) string {
	if account != nil && account.Platform == PlatformGemini && account.Type == AccountTypeOAuth &&
		strings.EqualFold(strings.TrimSpace(account.GetCredential("oauth_type")), "google_one") {
		// Keep the existing consumer-account probe compatible during upgrade.
		return "gemini-2.0-flash"
	}
	return geminicli.DefaultTestModel
}

func (s *AccountTestService) testGeminiAccountConnection(c *gin.Context, account *Account, modelID string, prompt string) error {
	ctx := c.Request.Context()

	// Determine the model to use
	testModelID := modelID
	if testModelID == "" {
		testModelID = geminiDefaultTestModel(account)
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

	resp, err := s.doAccountTestHTTPUpstreamWithTLS(req, proxyURL, account, s.tlsFPProfileService.ResolveTLSProfile(account))
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
	return s.testAntigravityAccountConnection(c, account, modelID, prompt)
}

// testAntigravityAccountConnection tests an Antigravity account's connection
// 支持 Claude 和 Gemini 两种协议，使用非流式请求
func (s *AccountTestService) testAntigravityAccountConnection(c *gin.Context, account *Account, modelID string, prompt string) error {
	ctx := c.Request.Context()

	// 默认模型：Claude 使用当前可用的 Sonnet 4.6，Gemini 使用 gemini-3-pro-preview
	testModelID := antigravityConnectionTestModel(modelID)

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

	// 调用 AntigravityGatewayService.TestConnectionWithPrompt（复用协议转换逻辑）
	result, err := s.antigravityGatewayService.TestConnectionWithPrompt(ctx, account, testModelID, prompt)
	if err != nil {
		return s.sendErrorAndEnd(c, err.Error())
	}

	// 发送响应内容
	if result.Text != "" {
		s.sendEvent(c, TestEvent{Type: "content", Text: result.Text})
	}
	for _, img := range result.Images {
		s.sendEvent(c, TestEvent{
			Type:     "image",
			ImageURL: img.ImageURL,
			MimeType: img.MimeType,
		})
	}

	s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
	return nil
}

func antigravityConnectionTestModel(modelID string) string {
	if strings.TrimSpace(modelID) == "" {
		return defaultAntigravityTestModel
	}
	return modelID
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
	fullURL, err := buildGeminiAIStudioModelActionURL(normalizedBaseURL, modelID, "streamGenerateContent", true)
	if err != nil {
		return nil, err
	}

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
		fullURL, err := buildGeminiAIStudioModelActionURL(normalizedBaseURL, modelID, "streamGenerateContent", true)
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
	var reasoningTokens *int64
	finished := false

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				s.sendEvent(c, TestEvent{Type: "test_complete", Success: true, ReasoningTokens: reasoningTokens})
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
			s.sendEvent(c, TestEvent{Type: "test_complete", Success: true, ReasoningTokens: reasoningTokens})
			return nil
		}

		var data map[string]any
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			continue
		}
		s.captureQualityResponseIdentity(c, data)
		if tokens := reasoningTokensFromPayload(data); tokens != nil {
			reasoningTokens = tokens
		}

		// Support two Gemini response formats:
		// - AI Studio: {"candidates": [...]}
		// - Gemini CLI: {"response": {"candidates": [...]}}
		if resp, ok := data["response"].(map[string]any); ok && resp != nil {
			data = resp
		}
		if candidates, ok := data["candidates"].([]any); ok && len(candidates) > 0 && !finished {
			if candidate, ok := candidates[0].(map[string]any); ok {
				// Extract content first (before checking completion)
				if content, ok := candidate["content"].(map[string]any); ok {
					if parts, ok := content["parts"].([]any); ok {
						for _, part := range parts {
							if partMap, ok := part.(map[string]any); ok {
								if text, ok := partMap["text"].(string); ok && text != "" {
									markQualityProbeOutputForContext(c)
								}
								if thought, _ := partMap["thought"].(bool); thought {
									continue
								}
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
					// Some Gemini transports emit usageMetadata in a trailing
					// event after the candidate's finishReason. Keep consuming
					// the stream so the quality probe can retain that count.
					finished = true
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

// createOpenAITestPayload creates a test payload for OpenAI Responses API.
func createOpenAITestPayload(modelID string, isOAuth bool, prompt string) map[string]any {
	testPrompt := strings.TrimSpace(prompt)
	if testPrompt == "" {
		testPrompt = "hi"
	}

	payload := map[string]any{
		"model": modelID,
		"input": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type": "input_text",
						"text": testPrompt,
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

	// Codex OAuth routes select the backend persona from the model-specific
	// client instructions. Reusing the generic fallback can produce a valid
	// response on the low-quality route and a 312-character turn state.
	if isOAuth {
		payload["instructions"] = openai.CodexBaseInstructionsForModel(modelID)
	} else {
		payload["instructions"] = openai.DefaultInstructions
	}

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
	var reasoningTokens *int64

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				if seenFinish {
					s.sendEvent(c, TestEvent{Type: "status", Text: "已通过 /v1/chat/completions 验证"})
					s.sendEvent(c, TestEvent{Type: "test_complete", Success: true, ReasoningTokens: reasoningTokens})
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
			s.sendEvent(c, TestEvent{Type: "test_complete", Success: true, ReasoningTokens: reasoningTokens})
			return nil
		}

		var data map[string]any
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			return s.sendErrorAndEnd(c, "Invalid Chat Completions response from /v1/chat/completions: expected JSON data")
		}
		seenJSON = true
		s.captureQualityResponseIdentity(c, data)
		if tokens := reasoningTokensFromPayload(data); tokens != nil {
			reasoningTokens = tokens
		}

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
				if len(delta) > 0 {
					for _, value := range delta {
						if text, ok := value.(string); ok && text != "" {
							markQualityProbeOutputForContext(c)
							break
						}
					}
				}
				if text, ok := delta["content"].(string); ok && text != "" {
					s.sendEvent(c, TestEvent{Type: "content", Text: text})
				}
			}
			if message, ok := choice["message"].(map[string]any); ok {
				if text, ok := message["content"].(string); ok && text != "" {
					markQualityProbeOutputForContext(c)
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
	var reasoningTokens *int64

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				if seenCompleted {
					s.sendEvent(c, TestEvent{Type: "test_complete", Success: true, ReasoningTokens: reasoningTokens})
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
				s.sendEvent(c, TestEvent{Type: "test_complete", Success: true, ReasoningTokens: reasoningTokens})
				return nil
			}
			return s.sendErrorAndEnd(c, "Stream ended before response.completed")
		}

		var data map[string]any
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			continue
		}
		s.captureQualityResponseIdentity(c, data)
		if tokens := reasoningTokensFromPayload(data); tokens != nil {
			reasoningTokens = tokens
		}

		eventType, _ := data["type"].(string)
		if strings.HasSuffix(eventType, ".delta") {
			markQualityProbeOutputForContext(c)
		}

		switch eventType {
		case "response.output_text.delta":
			// OpenAI Responses API uses "delta" field for text content
			if delta, ok := data["delta"].(string); ok && delta != "" {
				s.sendEvent(c, TestEvent{Type: "content", Text: delta})
			}
		case "response.completed", "response.done":
			s.sendEvent(c, TestEvent{Type: "test_complete", Success: true, ReasoningTokens: reasoningTokens})
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
		"model":  modelID,
		"prompt": prompt,
		"size":   "1024x1024",
		"n":      1,
	}
	if size := account.OpenAIImageTestSize(); size != "" {
		payload["size"] = size
	}
	payloadBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create request")
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+authToken)

	// 账号级请求头覆写：测试请求与真实转发保持一致的最终头
	account.ApplyHeaderOverrides(req.Header)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	resp, err := s.doAccountTestHTTPUpstreamWithTLS(req, proxyURL, account, s.tlsFPProfileService.ResolveTLSProfile(account))
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
			URL           string `json:"url"`
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
		} else if item.URL != "" {
			s.sendEvent(c, TestEvent{
				Type:     "image",
				ImageURL: item.URL,
				MimeType: "image/*",
			})
		}
	}

	s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
	return nil
}

// testOpenAIImageOAuth tests OpenAI image generation using an OAuth account via Codex /responses API.
func (s *AccountTestService) testOpenAIImageOAuth(c *gin.Context, ctx context.Context, account *Account, modelID, prompt string) error {
	credentialAccount := account
	if account.IsShadow() {
		resolved, err := resolveCredentialAccount(ctx, s.accountRepo, account)
		if err != nil {
			return s.sendErrorAndEnd(c, "Failed to resolve account credentials")
		}
		credentialAccount = resolved
	}
	authToken := ""
	if !credentialAccount.IsOpenAIAgentIdentity() {
		authToken = credentialAccount.GetOpenAIAccessToken()
	}
	if authToken == "" && !credentialAccount.IsOpenAIAgentIdentity() {
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
		Size:     account.OpenAIImageTestSize(),
	}
	parsed.ExplicitSize = parsed.Size != ""
	applyOpenAIImagesDefaults(parsed)
	s.sendEvent(c, TestEvent{Type: "content", Text: fmt.Sprintf("Responses driver: %s; image model: %s\n", openAIImagesResponsesMainModelValue(), parsed.Model)})

	responsesBody, err := buildOpenAIImagesResponsesRequest(parsed, parsed.Model)
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Failed to build image request: %s", err.Error()))
	}

	baseURL, useOfficialOAuthEndpoint, relayErr := resolveOpenAIOAuthCodexBaseURL(ctx, s.settingService, s.cfg, credentialAccount)
	if relayErr != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Invalid OpenAI OAuth Codex relay URL: %s", relayErr.Error()))
	}
	targetURL := buildOpenAIOAuthCodexResponsesURL(baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(responsesBody))
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create request")
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	if useOfficialOAuthEndpoint {
		req.Host = "chatgpt.com"
	}
	if credentialAccount.IsOpenAIAgentIdentity() {
		authHeaders, authErr := buildAgentIdentityAuthenticationHeaders(ctx, s.accountRepo, s.agentIdentityWS, &s.agentIdentityTaskMu, credentialAccount, s.cfg)
		if authErr != nil {
			_ = setOpenAIAgentIdentityAccountError(ctx, s.accountRepo, s.agentIdentityWS, credentialAccount, "Agent Identity authentication failed: "+authErr.Error())
			return s.sendErrorAndEnd(c, "Failed to build Agent Identity authentication")
		}
		for key, values := range authHeaders {
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}
	} else {
		req.Header.Set("Authorization", "Bearer "+authToken)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	req.Header.Set("originator", "codex_cli_rs")
	if customUA := strings.TrimSpace(credentialAccount.GetOpenAIUserAgent()); customUA != "" {
		req.Header.Set("User-Agent", customUA)
	} else {
		req.Header.Set("User-Agent", codexCLIUserAgent)
	}
	setOpenAIChatGPTAccountHeaders(req.Header, credentialAccount)
	// 与真实转发一致：originator 与最终 User-Agent 首段配套（原 opencode 与 Codex UA 错配会 404，issue #3901）。
	enforceCodexIdentityHeadersWithUA(req.Header, credentialAccount.GetOpenAIUserAgent())

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.doAccountTestHTTPUpstream(req, proxyURL, account)
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
		body = redactAgentIdentitySensitiveBodyForAccount(ctx, s.accountRepo, credentialAccount, body)
		if resp.StatusCode == http.StatusForbidden && credentialAccount.IsOpenAIAgentIdentity() {
			_ = setOpenAIAgentIdentityAccountError(ctx, s.accountRepo, s.agentIdentityWS, credentialAccount, buildForbiddenErrorMessage(
				"Agent Identity access forbidden (403):", extractUpstreamErrorMessage(body), body, "credentials require account action",
			))
		}
		message := strings.TrimSpace(extractUpstreamErrorMessage(body))
		if message == "" {
			message = fmt.Sprintf("Responses API returned %d", resp.StatusCode)
		}
		return s.sendErrorAndEnd(c, message)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Failed to read image response: %s", err.Error()))
	}
	body = redactAgentIdentitySensitiveBodyForAccount(ctx, s.accountRepo, credentialAccount, body)

	results, _, _, _, _, err := collectOpenAIImagesFromResponsesBody(body)
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Failed to parse image response: %s", err.Error()))
	}
	if len(results) == 0 {
		if upstreamErr := extractOpenAIImagesUpstreamError(body); upstreamErr != nil {
			return s.sendErrorAndEnd(c, upstreamErr.clientMessage())
		}
		if textErr := openAIImagesTextFallbackError(body); textErr != nil {
			return s.sendErrorAndEnd(c, textErr.clientMessage())
		}
		return s.sendErrorAndEnd(c, "No images returned from responses API")
	}

	for _, item := range results {
		if item.RevisedPrompt != "" {
			s.sendEvent(c, TestEvent{Type: "content", Text: item.RevisedPrompt})
		}
		mimeType := openAIImageOutputMIMEType(item.OutputFormat)
		s.sendEvent(c, TestEvent{
			Type:     "image",
			ImageURL: "data:" + mimeType + ";base64," + item.Result,
			MimeType: mimeType,
		})
	}

	s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
	return nil
}

func (s *AccountTestService) sendEvent(c *gin.Context, event TestEvent) {
	if c != nil && c.Request != nil && (event.Type == "content" || event.Type == "image") {
		markQualityProbeOutput(c.Request.Context())
	}
	eventJSON, _ := json.Marshal(event)
	if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", eventJSON); err != nil {
		log.Printf("failed to write SSE event: %v", err)
		return
	}
	c.Writer.Flush()
}

// sendErrorAndEnd sends an error event and ends the stream
func (s *AccountTestService) sendErrorAndEnd(c *gin.Context, errorMsg string) error {
	logAccountTestError(c, errorMsg)
	s.sendEvent(c, TestEvent{Type: "error", Error: errorMsg})
	return fmt.Errorf("%s", errorMsg)
}

// RunTestBackground executes an account test in-memory (no real HTTP client),
// capturing SSE output via httptest.NewRecorder, then parses the result.
func (s *AccountTestService) RunTestBackground(ctx context.Context, accountID int64, modelID string) (*ScheduledTestResult, error) {
	return s.runTestBackground(ctx, accountID, modelID, "", "")
}

// RunQualityTestBackground executes the same account-specific transport probe
// used by the admin test endpoint, but supplies a deterministic quality prompt.
// The caller owns the grader; this method only captures the upstream response
// and preserves the existing SSE/error semantics.
func (s *AccountTestService) RunQualityTestBackground(ctx context.Context, accountID int64, modelID, prompt, effort string) (*ScheduledTestResult, error) {
	ctx = withAccountTestUsage(ctx)
	return s.runTestBackground(withAccountTestEffort(ctx, effort), accountID, modelID, prompt, effort)
}

func (s *AccountTestService) runTestBackground(ctx context.Context, accountID int64, modelID, prompt, _ string) (*ScheduledTestResult, error) {
	startedAt := time.Now()
	ctx, turnStateCapture := withAccountTestTurnStateCapture(ctx)

	w := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(w)
	ginCtx.Request = (&http.Request{}).WithContext(ctx)
	ginCtx.Set(accountTestBackgroundKey, true)

	testErr := s.TestAccountConnection(ginCtx, accountID, modelID, prompt, AccountTestModeDefault)

	finishedAt := time.Now()
	body := w.Body.String()
	responseText, errMsg, reasoningTokens := parseTestSSEOutputWithReasoning(body)
	conversationID, responseID := parseQualityResponseIdentity(body)

	status := "success"
	if testErr != nil || errMsg != "" {
		status = "failed"
		if errMsg == "" && testErr != nil {
			errMsg = testErr.Error()
		}
	}

	return &ScheduledTestResult{
		Status:            status,
		ResponseText:      responseText,
		ErrorMessage:      errMsg,
		ReasoningTokens:   reasoningTokens,
		TurnState:         turnStateCapture.value(),
		InjectedTurnState: accountTestTurnState(ctx),
		ConversationID:    conversationID, ResponseID: responseID,
		LatencyMs:  finishedAt.Sub(startedAt).Milliseconds(),
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
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
