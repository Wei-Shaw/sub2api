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
	"regexp"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// sseDataPrefix matches SSE data lines with optional whitespace after colon.
// Some upstream APIs return non-standard "data:" without space (should be "data: ").
var sseDataPrefix = regexp.MustCompile(`^data:\s*`)
var cloudflareRayPattern = regexp.MustCompile(`(?i)cRay:\s*'([a-z0-9-]+)'`)

const (
	testClaudeAPIURL   = "https://api.anthropic.com/v1/messages"
	chatgptCodexAPIURL = "https://chatgpt.com/backend-api/codex/responses"
	soraMeAPIURL       = "https://sora.chatgpt.com/backend/me" // Sora 用户信息接口，用于测试连接
	soraBillingAPIURL  = "https://sora.chatgpt.com/backend/billing/subscriptions"
	soraInviteMineURL  = "https://sora.chatgpt.com/backend/project_y/invite/mine"
	soraBootstrapURL   = "https://sora.chatgpt.com/backend/m/bootstrap"
	soraRemainingURL   = "https://sora.chatgpt.com/backend/nf/check"
)

// TestEvent represents a SSE event for account testing
type TestEvent struct {
	Type    string `json:"type"`
	Text    string `json:"text,omitempty"`
	Model   string `json:"model,omitempty"`
	Success bool   `json:"success,omitempty"`
	Error   string `json:"error,omitempty"`
REDACTED

// AccountTestService handles account testing operations
type AccountTestService struct {
	accountRepo               AccountRepository
	geminiTokenProvider       *GeminiTokenProvider
	antigravityGatewayService *AntigravityGatewayService
	httpUpstream              HTTPUpstream
	cfg                       *config.Config
REDACTED

// NewAccountTestService creates a new AccountTestService
func NewAccountTestService(
	accountRepo AccountRepository,
	geminiTokenProvider *GeminiTokenProvider,
	antigravityGatewayService *AntigravityGatewayService,
	httpUpstream HTTPUpstream,
	cfg *config.Config,
) *AccountTestService {
	return &AccountTestService{
		accountRepo:               accountRepo,
		geminiTokenProvider:       geminiTokenProvider,
		antigravityGatewayService: antigravityGatewayService,
		httpUpstream:              httpUpstream,
		cfg:                       cfg,
REDACTED
REDACTED

func (s *AccountTestService) validateUpstreamBaseURL(raw string) (string, error) {
	if s.cfg == nil {
		return "", errors.New("config is not available")
REDACTED
	if !s.cfg.Security.URLAllowlist.Enabled {
		return urlvalidator.ValidateURLFormat(raw, s.cfg.Security.URLAllowlist.AllowInsecureHTTP)
REDACTED
	normalized, err := urlvalidator.ValidateHTTPSURL(raw, urlvalidator.ValidationOptions{
		AllowedHosts:     s.cfg.Security.URLAllowlist.UpstreamHosts,
		RequireAllowlist: true,
		AllowPrivate:     s.cfg.Security.URLAllowlist.AllowPrivateHosts,
REDACTED)
	if err != nil {
		return "", err
REDACTED
	return normalized, nil
REDACTED

// generateSessionString generates a Claude Code style session string
func generateSessionString() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
REDACTED
	hex64 := hex.EncodeToString(bytes)
	sessionUUID := uuid.New().String()
	return fmt.Sprintf("user_%s_account__session_%s", hex64, sessionUUID), nil
REDACTED

// createTestPayload creates a Claude Code style test request payload
func createTestPayload(modelID string) (map[string]any, error) {
	sessionID, err := generateSessionString()
	if err != nil {
		return nil, err
REDACTED

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
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
		"system": []map[string]any{
			{
				"type": "text",
				"text": claudeCodeSystemPrompt,
				"cache_control": map[string]string{
					"type": "ephemeral",
			REDACTED,
		REDACTED,
	REDACTED,
		"metadata": map[string]string{
			"user_id": sessionID,
	REDACTED,
		"max_tokens":  1024,
		"temperature": 1,
		"stream":      true,
REDACTED, nil
REDACTED

// TestAccountConnection tests an account's connection by sending a test request
// All account types use full Claude Code client characteristics, only auth header differs
// modelID is optional - if empty, defaults to claude.DefaultTestModel
func (s *AccountTestService) TestAccountConnection(c *gin.Context, accountID int64, modelID string) error {
	ctx := c.Request.Context()

	// Get account
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return s.sendErrorAndEnd(c, "Account not found")
REDACTED

	// Route to platform-specific test method
	if account.IsOpenAI() {
		return s.testOpenAIAccountConnection(c, account, modelID)
REDACTED

	if account.IsGemini() {
		return s.testGeminiAccountConnection(c, account, modelID)
REDACTED

	if account.Platform == PlatformAntigravity {
		return s.testAntigravityAccountConnection(c, account, modelID)
REDACTED

	if account.Platform == PlatformSora {
		return s.testSoraAccountConnection(c, account)
REDACTED

	return s.testClaudeAccountConnection(c, account, modelID)
REDACTED

// testClaudeAccountConnection tests an Anthropic Claude account's connection
func (s *AccountTestService) testClaudeAccountConnection(c *gin.Context, account *Account, modelID string) error {
	ctx := c.Request.Context()

	// Determine the model to use
	testModelID := modelID
	if testModelID == "" {
		testModelID = claude.DefaultTestModel
REDACTED

	// For API Key accounts with model mapping, map the model
	if account.Type == "apikey" {
		mapping := account.GetModelMapping()
		if len(mapping) > 0 {
			if mappedModel, exists := mapping[testModelID]; exists {
				testModelID = mappedModel
		REDACTED
	REDACTED
REDACTED

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
	REDACTED
REDACTED else if account.Type == "apikey" {
		// API Key - use x-api-key header
		useBearer = false
		authToken = account.GetCredential("api_key")
		if authToken == "" {
			return s.sendErrorAndEnd(c, "No API key available")
	REDACTED

		baseURL := account.GetBaseURL()
		if baseURL == "" {
			baseURL = "https://api.anthropic.com"
	REDACTED
		normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
		if err != nil {
			return s.sendErrorAndEnd(c, fmt.Sprintf("Invalid base URL: %s", err.Error()))
	REDACTED
		apiURL = strings.TrimSuffix(normalizedBaseURL, "/") + "/v1/messages"
REDACTED else {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Unsupported account type: %s", account.Type))
REDACTED

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
REDACTED
	payloadBytes, _ := json.Marshal(payload)

	// Send test_start event
	s.sendEvent(c, TestEvent{Type: "test_start", Model: testModelIDREDACTED)

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create request")
REDACTED

	// Set common headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")

	// Apply Claude Code client headers
	for key, value := range claude.DefaultHeaders {
		req.Header.Set(key, value)
REDACTED

	// Set authentication header
	if useBearer {
		req.Header.Set("anthropic-beta", claude.DefaultBetaHeader)
		req.Header.Set("Authorization", "Bearer "+authToken)
REDACTED else {
		req.Header.Set("anthropic-beta", claude.APIKeyBetaHeader)
		req.Header.Set("x-api-key", authToken)
REDACTED

	// Get proxy URL
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED

	resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, account.IsTLSFingerprintEnabled())
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Request failed: %s", err.Error()))
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return s.sendErrorAndEnd(c, fmt.Sprintf("API returned %d: %s", resp.StatusCode, string(body)))
REDACTED

	// Process SSE stream
	return s.processClaudeStream(c, resp.Body)
REDACTED

// testOpenAIAccountConnection tests an OpenAI account's connection
func (s *AccountTestService) testOpenAIAccountConnection(c *gin.Context, account *Account, modelID string) error {
	ctx := c.Request.Context()

	// Default to openai.DefaultTestModel for OpenAI testing
	testModelID := modelID
	if testModelID == "" {
		testModelID = openai.DefaultTestModel
REDACTED

	// For API Key accounts with model mapping, map the model
	if account.Type == "apikey" {
		mapping := account.GetModelMapping()
		if len(mapping) > 0 {
			if mappedModel, exists := mapping[testModelID]; exists {
				testModelID = mappedModel
		REDACTED
	REDACTED
REDACTED

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
	REDACTED

		// OAuth uses ChatGPT internal API
		apiURL = chatgptCodexAPIURL
		chatgptAccountID = account.GetChatGPTAccountID()
REDACTED else if account.Type == "apikey" {
		// API Key - use Platform API
		authToken = account.GetOpenAIApiKey()
		if authToken == "" {
			return s.sendErrorAndEnd(c, "No API key available")
	REDACTED

		baseURL := account.GetOpenAIBaseURL()
		if baseURL == "" {
			baseURL = "https://api.openai.com"
	REDACTED
		normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
		if err != nil {
			return s.sendErrorAndEnd(c, fmt.Sprintf("Invalid base URL: %s", err.Error()))
	REDACTED
		apiURL = strings.TrimSuffix(normalizedBaseURL, "/") + "/responses"
REDACTED else {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Unsupported account type: %s", account.Type))
REDACTED

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
	s.sendEvent(c, TestEvent{Type: "test_start", Model: testModelIDREDACTED)

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create request")
REDACTED

	// Set common headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+authToken)

	// Set OAuth-specific headers for ChatGPT internal API
	if isOAuth {
		req.Host = "chatgpt.com"
		req.Header.Set("accept", "text/event-stream")
		if chatgptAccountID != "" {
			req.Header.Set("chatgpt-account-id", chatgptAccountID)
	REDACTED
REDACTED

	// Get proxy URL
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED

	resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, account.IsTLSFingerprintEnabled())
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Request failed: %s", err.Error()))
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return s.sendErrorAndEnd(c, fmt.Sprintf("API returned %d: %s", resp.StatusCode, string(body)))
REDACTED

	// Process SSE stream
	return s.processOpenAIStream(c, resp.Body)
REDACTED

// testGeminiAccountConnection tests a Gemini account's connection
func (s *AccountTestService) testGeminiAccountConnection(c *gin.Context, account *Account, modelID string) error {
	ctx := c.Request.Context()

	// Determine the model to use
	testModelID := modelID
	if testModelID == "" {
		testModelID = geminicli.DefaultTestModel
REDACTED

	// For API Key accounts with model mapping, map the model
	if account.Type == AccountTypeAPIKey {
		mapping := account.GetModelMapping()
		if len(mapping) > 0 {
			if mappedModel, exists := mapping[testModelID]; exists {
				testModelID = mappedModel
		REDACTED
	REDACTED
REDACTED

	// Set SSE headers
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	// Create test payload (Gemini format)
	payload := createGeminiTestPayload()

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
REDACTED

	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Failed to build request: %s", err.Error()))
REDACTED

	// Send test_start event
	s.sendEvent(c, TestEvent{Type: "test_start", Model: testModelIDREDACTED)

	// Get proxy and execute request
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED

	resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, account.IsTLSFingerprintEnabled())
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Request failed: %s", err.Error()))
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return s.sendErrorAndEnd(c, fmt.Sprintf("API returned %d: %s", resp.StatusCode, string(body)))
REDACTED

	// Process SSE stream
	return s.processGeminiStream(c, resp.Body)
REDACTED

// testSoraAccountConnection 测试 Sora 账号的连接
// 调用 /backend/me 接口验证 access_token 有效性（不需要 Sentinel Token）
func (s *AccountTestService) testSoraAccountConnection(c *gin.Context, account *Account) error {
	ctx := c.Request.Context()

	authToken := account.GetCredential("access_token")
	if authToken == "" {
		return s.sendErrorAndEnd(c, "No access token available")
REDACTED

	// Set SSE headers
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	// Send test_start event
	s.sendEvent(c, TestEvent{Type: "test_start", Model: "sora"REDACTED)

	req, err := http.NewRequestWithContext(ctx, "GET", soraMeAPIURL, nil)
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create request")
REDACTED

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
REDACTED
	enableSoraTLSFingerprint := s.shouldEnableSoraTLSFingerprint()

	resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, enableSoraTLSFingerprint)
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Request failed: %s", err.Error()))
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		if isCloudflareChallengeResponse(resp.StatusCode, body) {
			return s.sendErrorAndEnd(c, formatCloudflareChallengeMessage("Sora request blocked by Cloudflare challenge (HTTP 403). Please switch to a clean proxy/network and retry.", resp.Header, body))
	REDACTED
		return s.sendErrorAndEnd(c, fmt.Sprintf("Sora API returned %d: %s", resp.StatusCode, truncateSoraErrorBody(body, 512)))
REDACTED

	// 解析 /me 响应，提取用户信息
	var meResp map[string]any
	if err := json.Unmarshal(body, &meResp); err != nil {
		// 能收到 200 就说明 token 有效
		s.sendEvent(c, TestEvent{Type: "content", Text: "Sora connection OK (token valid)"REDACTED)
REDACTED else {
		// 尝试提取用户名或邮箱信息
		info := "Sora connection OK"
		if name, ok := meResp["name"].(string); ok && name != "" {
			info = fmt.Sprintf("Sora connection OK - User: %s", name)
	REDACTED else if email, ok := meResp["email"].(string); ok && email != "" {
			info = fmt.Sprintf("Sora connection OK - Email: %s", email)
	REDACTED
		s.sendEvent(c, TestEvent{Type: "content", Text: infoREDACTED)
REDACTED

	// 追加轻量能力检查：订阅信息查询（失败仅告警，不中断连接测试）
	subReq, err := http.NewRequestWithContext(ctx, "GET", soraBillingAPIURL, nil)
	if err == nil {
		subReq.Header.Set("Authorization", "Bearer "+authToken)
		subReq.Header.Set("User-Agent", "Sora/1.2026.007 (Android 15; 24122RKC7C; build 2600700)")
		subReq.Header.Set("Accept", "application/json")
		subReq.Header.Set("Accept-Language", "en-US,en;q=0.9")
		subReq.Header.Set("Origin", "https://sora.chatgpt.com")
		subReq.Header.Set("Referer", "https://sora.chatgpt.com/")

		subResp, subErr := s.httpUpstream.DoWithTLS(subReq, proxyURL, account.ID, account.Concurrency, enableSoraTLSFingerprint)
		if subErr != nil {
			s.sendEvent(c, TestEvent{Type: "content", Text: fmt.Sprintf("Subscription check skipped: %s", subErr.Error())REDACTED)
	REDACTED else {
			subBody, _ := io.ReadAll(subResp.Body)
			_ = subResp.Body.Close()
			if subResp.StatusCode == http.StatusOK {
				if summary := parseSoraSubscriptionSummary(subBody); summary != "" {
					s.sendEvent(c, TestEvent{Type: "content", Text: summaryREDACTED)
			REDACTED else {
					s.sendEvent(c, TestEvent{Type: "content", Text: "Subscription check OK"REDACTED)
			REDACTED
		REDACTED else {
				if isCloudflareChallengeResponse(subResp.StatusCode, subBody) {
					s.sendEvent(c, TestEvent{Type: "content", Text: formatCloudflareChallengeMessage("Subscription check blocked by Cloudflare challenge (HTTP 403)", subResp.Header, subBody)REDACTED)
			REDACTED else {
					s.sendEvent(c, TestEvent{Type: "content", Text: fmt.Sprintf("Subscription check returned %d", subResp.StatusCode)REDACTED)
			REDACTED
		REDACTED
	REDACTED
REDACTED

	// 追加 Sora2 能力探测（对齐 sora2api 的测试思路）：邀请码 + 剩余额度。
	s.testSora2Capabilities(c, ctx, account, authToken, proxyURL, enableSoraTLSFingerprint)

	s.sendEvent(c, TestEvent{Type: "test_complete", Success: trueREDACTED)
	return nil
REDACTED

func (s *AccountTestService) testSora2Capabilities(
	c *gin.Context,
	ctx context.Context,
	account *Account,
	authToken string,
	proxyURL string,
	enableTLSFingerprint bool,
) {
	inviteStatus, inviteHeader, inviteBody, err := s.fetchSoraTestEndpoint(
		ctx,
		account,
		authToken,
		soraInviteMineURL,
		proxyURL,
		enableTLSFingerprint,
	)
	if err != nil {
		s.sendEvent(c, TestEvent{Type: "content", Text: fmt.Sprintf("Sora2 invite check skipped: %s", err.Error())REDACTED)
		return
REDACTED

	if inviteStatus == http.StatusUnauthorized {
		bootstrapStatus, _, _, bootstrapErr := s.fetchSoraTestEndpoint(
			ctx,
			account,
			authToken,
			soraBootstrapURL,
			proxyURL,
			enableTLSFingerprint,
		)
		if bootstrapErr == nil && bootstrapStatus == http.StatusOK {
			s.sendEvent(c, TestEvent{Type: "content", Text: "Sora2 bootstrap OK, retry invite check"REDACTED)
			inviteStatus, inviteHeader, inviteBody, err = s.fetchSoraTestEndpoint(
				ctx,
				account,
				authToken,
				soraInviteMineURL,
				proxyURL,
				enableTLSFingerprint,
			)
			if err != nil {
				s.sendEvent(c, TestEvent{Type: "content", Text: fmt.Sprintf("Sora2 invite retry failed: %s", err.Error())REDACTED)
				return
		REDACTED
	REDACTED
REDACTED

	if inviteStatus != http.StatusOK {
		if isCloudflareChallengeResponse(inviteStatus, inviteBody) {
			s.sendEvent(c, TestEvent{Type: "content", Text: formatCloudflareChallengeMessage("Sora2 invite check blocked by Cloudflare challenge (HTTP 403)", inviteHeader, inviteBody)REDACTED)
			return
	REDACTED
		s.sendEvent(c, TestEvent{Type: "content", Text: fmt.Sprintf("Sora2 invite check returned %d", inviteStatus)REDACTED)
		return
REDACTED

	if summary := parseSoraInviteSummary(inviteBody); summary != "" {
		s.sendEvent(c, TestEvent{Type: "content", Text: summaryREDACTED)
REDACTED else {
		s.sendEvent(c, TestEvent{Type: "content", Text: "Sora2 invite check OK"REDACTED)
REDACTED

	remainingStatus, remainingHeader, remainingBody, remainingErr := s.fetchSoraTestEndpoint(
		ctx,
		account,
		authToken,
		soraRemainingURL,
		proxyURL,
		enableTLSFingerprint,
	)
	if remainingErr != nil {
		s.sendEvent(c, TestEvent{Type: "content", Text: fmt.Sprintf("Sora2 remaining check skipped: %s", remainingErr.Error())REDACTED)
		return
REDACTED
	if remainingStatus != http.StatusOK {
		if isCloudflareChallengeResponse(remainingStatus, remainingBody) {
			s.sendEvent(c, TestEvent{Type: "content", Text: formatCloudflareChallengeMessage("Sora2 remaining check blocked by Cloudflare challenge (HTTP 403)", remainingHeader, remainingBody)REDACTED)
			return
	REDACTED
		s.sendEvent(c, TestEvent{Type: "content", Text: fmt.Sprintf("Sora2 remaining check returned %d", remainingStatus)REDACTED)
		return
REDACTED
	if summary := parseSoraRemainingSummary(remainingBody); summary != "" {
		s.sendEvent(c, TestEvent{Type: "content", Text: summaryREDACTED)
REDACTED else {
		s.sendEvent(c, TestEvent{Type: "content", Text: "Sora2 remaining check OK"REDACTED)
REDACTED
REDACTED

func (s *AccountTestService) fetchSoraTestEndpoint(
	ctx context.Context,
	account *Account,
	authToken string,
	url string,
	proxyURL string,
	enableTLSFingerprint bool,
) (int, http.Header, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, nil, nil, err
REDACTED
	req.Header.Set("Authorization", "Bearer "+authToken)
	req.Header.Set("User-Agent", "Sora/1.2026.007 (Android 15; 24122RKC7C; build 2600700)")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Origin", "https://sora.chatgpt.com")
	req.Header.Set("Referer", "https://sora.chatgpt.com/")

	resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, enableTLSFingerprint)
	if err != nil {
		return 0, nil, nil, err
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return resp.StatusCode, resp.Header, nil, readErr
REDACTED
	return resp.StatusCode, resp.Header, body, nil
REDACTED

func parseSoraSubscriptionSummary(body []byte) string {
	var subResp struct {
		Data []struct {
			Plan struct {
				ID    string `json:"id"`
				Title string `json:"title"`
		REDACTED `json:"plan"`
			EndTS string `json:"end_ts"`
	REDACTED `json:"data"`
REDACTED
	if err := json.Unmarshal(body, &subResp); err != nil {
		return ""
REDACTED
	if len(subResp.Data) == 0 {
		return ""
REDACTED

	first := subResp.Data[0]
	parts := make([]string, 0, 3)
	if first.Plan.Title != "" {
		parts = append(parts, first.Plan.Title)
REDACTED
	if first.Plan.ID != "" {
		parts = append(parts, first.Plan.ID)
REDACTED
	if first.EndTS != "" {
		parts = append(parts, "end="+first.EndTS)
REDACTED
	if len(parts) == 0 {
		return ""
REDACTED
	return "Subscription: " + strings.Join(parts, " | ")
REDACTED

func parseSoraInviteSummary(body []byte) string {
	var inviteResp struct {
		InviteCode    string `json:"invite_code"`
		RedeemedCount int64  `json:"redeemed_count"`
		TotalCount    int64  `json:"total_count"`
REDACTED
	if err := json.Unmarshal(body, &inviteResp); err != nil {
		return ""
REDACTED

	parts := []string{"Sora2: supported"REDACTED
	if inviteResp.InviteCode != "" {
		parts = append(parts, "invite="+inviteResp.InviteCode)
REDACTED
	if inviteResp.TotalCount > 0 {
		parts = append(parts, fmt.Sprintf("used=%d/%d", inviteResp.RedeemedCount, inviteResp.TotalCount))
REDACTED
	return strings.Join(parts, " | ")
REDACTED

func parseSoraRemainingSummary(body []byte) string {
	var remainingResp struct {
		RateLimitAndCreditBalance struct {
			EstimatedNumVideosRemaining int64 `json:"estimated_num_videos_remaining"`
			RateLimitReached            bool  `json:"rate_limit_reached"`
			AccessResetsInSeconds       int64 `json:"access_resets_in_seconds"`
	REDACTED `json:"rate_limit_and_credit_balance"`
REDACTED
	if err := json.Unmarshal(body, &remainingResp); err != nil {
		return ""
REDACTED
	info := remainingResp.RateLimitAndCreditBalance
	parts := []string{fmt.Sprintf("Sora2 remaining: %d", info.EstimatedNumVideosRemaining)REDACTED
	if info.RateLimitReached {
		parts = append(parts, "rate_limited=true")
REDACTED
	if info.AccessResetsInSeconds > 0 {
		parts = append(parts, fmt.Sprintf("reset_in=%ds", info.AccessResetsInSeconds))
REDACTED
	return strings.Join(parts, " | ")
REDACTED

func (s *AccountTestService) shouldEnableSoraTLSFingerprint() bool {
	if s == nil || s.cfg == nil {
		return false
REDACTED
	return s.cfg.Gateway.TLSFingerprint.Enabled && !s.cfg.Sora.Client.DisableTLSFingerprint
REDACTED

func isCloudflareChallengeResponse(statusCode int, body []byte) bool {
	if statusCode != http.StatusForbidden {
		return false
REDACTED
	preview := strings.ToLower(truncateSoraErrorBody(body, 4096))
	return strings.Contains(preview, "window._cf_chl_opt") ||
		strings.Contains(preview, "just a moment") ||
		strings.Contains(preview, "enable javascript and cookies to continue")
REDACTED

func formatCloudflareChallengeMessage(base string, headers http.Header, body []byte) string {
	rayID := extractCloudflareRayID(headers, body)
	if rayID == "" {
		return base
REDACTED
	return fmt.Sprintf("%s (cf-ray: %s)", base, rayID)
REDACTED

func extractCloudflareRayID(headers http.Header, body []byte) string {
	if headers != nil {
		rayID := strings.TrimSpace(headers.Get("cf-ray"))
		if rayID != "" {
			return rayID
	REDACTED
		rayID = strings.TrimSpace(headers.Get("Cf-Ray"))
		if rayID != "" {
			return rayID
	REDACTED
REDACTED

	preview := truncateSoraErrorBody(body, 8192)
	matches := cloudflareRayPattern.FindStringSubmatch(preview)
	if len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
REDACTED
	return ""
REDACTED

func truncateSoraErrorBody(body []byte, max int) string {
	if max <= 0 {
		max = 512
REDACTED
	raw := strings.TrimSpace(string(body))
	if len(raw) <= max {
		return raw
REDACTED
	return raw[:max] + "...(truncated)"
REDACTED

// testAntigravityAccountConnection tests an Antigravity account's connection
// 支持 Claude 和 Gemini 两种协议，使用非流式请求
func (s *AccountTestService) testAntigravityAccountConnection(c *gin.Context, account *Account, modelID string) error {
	ctx := c.Request.Context()

	// 默认模型：Claude 使用 claude-sonnet-4-5，Gemini 使用 gemini-3-pro-preview
	testModelID := modelID
	if testModelID == "" {
		testModelID = "claude-sonnet-4-5"
REDACTED

	if s.antigravityGatewayService == nil {
		return s.sendErrorAndEnd(c, "Antigravity gateway service not configured")
REDACTED

	// Set SSE headers
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	// Send test_start event
	s.sendEvent(c, TestEvent{Type: "test_start", Model: testModelIDREDACTED)

	// 调用 AntigravityGatewayService.TestConnection（复用协议转换逻辑）
	result, err := s.antigravityGatewayService.TestConnection(ctx, account, testModelID)
	if err != nil {
		return s.sendErrorAndEnd(c, err.Error())
REDACTED

	// 发送响应内容
	if result.Text != "" {
		s.sendEvent(c, TestEvent{Type: "content", Text: result.TextREDACTED)
REDACTED

	s.sendEvent(c, TestEvent{Type: "test_complete", Success: trueREDACTED)
	return nil
REDACTED

// buildGeminiAPIKeyRequest builds request for Gemini API Key accounts
func (s *AccountTestService) buildGeminiAPIKeyRequest(ctx context.Context, account *Account, modelID string, payload []byte) (*http.Request, error) {
	apiKey := account.GetCredential("api_key")
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("no API key available")
REDACTED

	baseURL := account.GetCredential("base_url")
	if baseURL == "" {
		baseURL = geminicli.AIStudioBaseURL
REDACTED
	normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, err
REDACTED

	// Use streamGenerateContent for real-time feedback
	fullURL := fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent?alt=sse",
		strings.TrimRight(normalizedBaseURL, "/"), modelID)

	req, err := http.NewRequestWithContext(ctx, "POST", fullURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
REDACTED

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", apiKey)

	return req, nil
REDACTED

// buildGeminiOAuthRequest builds request for Gemini OAuth accounts
func (s *AccountTestService) buildGeminiOAuthRequest(ctx context.Context, account *Account, modelID string, payload []byte) (*http.Request, error) {
	if s.geminiTokenProvider == nil {
		return nil, fmt.Errorf("gemini token provider not configured")
REDACTED

	// Get access token (auto-refreshes if needed)
	accessToken, err := s.geminiTokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
REDACTED

	projectID := strings.TrimSpace(account.GetCredential("project_id"))
	if projectID == "" {
		// AI Studio OAuth mode (no project_id): call generativelanguage API directly with Bearer token.
		baseURL := account.GetCredential("base_url")
		if strings.TrimSpace(baseURL) == "" {
			baseURL = geminicli.AIStudioBaseURL
	REDACTED
		normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
		if err != nil {
			return nil, err
	REDACTED
		fullURL := fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent?alt=sse", strings.TrimRight(normalizedBaseURL, "/"), modelID)

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(payload))
		if err != nil {
			return nil, err
	REDACTED
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+accessToken)
		return req, nil
REDACTED

	// Code Assist mode (with project_id)
	return s.buildCodeAssistRequest(ctx, accessToken, projectID, modelID, payload)
REDACTED

// buildCodeAssistRequest builds request for Google Code Assist API (used by Gemini CLI and Antigravity)
func (s *AccountTestService) buildCodeAssistRequest(ctx context.Context, accessToken, projectID, modelID string, payload []byte) (*http.Request, error) {
	var inner map[string]any
	if err := json.Unmarshal(payload, &inner); err != nil {
		return nil, err
REDACTED

	wrapped := map[string]any{
		"model":   modelID,
		"project": projectID,
		"request": inner,
REDACTED
	wrappedBytes, _ := json.Marshal(wrapped)

	normalizedBaseURL, err := s.validateUpstreamBaseURL(geminicli.GeminiCliBaseURL)
	if err != nil {
		return nil, err
REDACTED
	fullURL := fmt.Sprintf("%s/v1internal:streamGenerateContent?alt=sse", normalizedBaseURL)

	req, err := http.NewRequestWithContext(ctx, "POST", fullURL, bytes.NewReader(wrappedBytes))
	if err != nil {
		return nil, err
REDACTED

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", geminicli.GeminiCLIUserAgent)

	return req, nil
REDACTED

// createGeminiTestPayload creates a minimal test payload for Gemini API
func createGeminiTestPayload() []byte {
	payload := map[string]any{
		"contents": []map[string]any{
			{
				"role": "user",
				"parts": []map[string]any{
					{"text": "hi"REDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
		"systemInstruction": map[string]any{
			"parts": []map[string]any{
				{"text": "You are a helpful AI assistant."REDACTED,
		REDACTED,
	REDACTED,
REDACTED
	bytes, _ := json.Marshal(payload)
	return bytes
REDACTED

// processGeminiStream processes SSE stream from Gemini API
func (s *AccountTestService) processGeminiStream(c *gin.Context, body io.Reader) error {
	reader := bufio.NewReader(body)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				s.sendEvent(c, TestEvent{Type: "test_complete", Success: trueREDACTED)
				return nil
		REDACTED
			return s.sendErrorAndEnd(c, fmt.Sprintf("Stream read error: %s", err.Error()))
	REDACTED

		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "data: ") {
			continue
	REDACTED

		jsonStr := strings.TrimPrefix(line, "data: ")
		if jsonStr == "[DONE]" {
			s.sendEvent(c, TestEvent{Type: "test_complete", Success: trueREDACTED)
			return nil
	REDACTED

		var data map[string]any
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			continue
	REDACTED

		// Support two Gemini response formats:
		// - AI Studio: {"candidates": [...]REDACTED
		// - Gemini CLI: {"response": {"candidates": [...]REDACTEDREDACTED
		if resp, ok := data["response"].(map[string]any); ok && resp != nil {
			data = resp
	REDACTED
		if candidates, ok := data["candidates"].([]any); ok && len(candidates) > 0 {
			if candidate, ok := candidates[0].(map[string]any); ok {
				// Extract content first (before checking completion)
				if content, ok := candidate["content"].(map[string]any); ok {
					if parts, ok := content["parts"].([]any); ok {
						for _, part := range parts {
							if partMap, ok := part.(map[string]any); ok {
								if text, ok := partMap["text"].(string); ok && text != "" {
									s.sendEvent(c, TestEvent{Type: "content", Text: textREDACTED)
							REDACTED
						REDACTED
					REDACTED
				REDACTED
			REDACTED

				// Check for completion after extracting content
				if finishReason, ok := candidate["finishReason"].(string); ok && finishReason != "" {
					s.sendEvent(c, TestEvent{Type: "test_complete", Success: trueREDACTED)
					return nil
			REDACTED
		REDACTED
	REDACTED

		// Handle errors
		if errData, ok := data["error"].(map[string]any); ok {
			errorMsg := "Unknown error"
			if msg, ok := errData["message"].(string); ok {
				errorMsg = msg
		REDACTED
			return s.sendErrorAndEnd(c, errorMsg)
	REDACTED
REDACTED
REDACTED

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
				REDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
		"stream": true,
REDACTED

	// OAuth accounts using ChatGPT internal API require store: false
	if isOAuth {
		payload["store"] = false
REDACTED

	// All accounts require instructions for Responses API
	payload["instructions"] = openai.DefaultInstructions

	return payload
REDACTED

// processClaudeStream processes the SSE stream from Claude API
func (s *AccountTestService) processClaudeStream(c *gin.Context, body io.Reader) error {
	reader := bufio.NewReader(body)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				s.sendEvent(c, TestEvent{Type: "test_complete", Success: trueREDACTED)
				return nil
		REDACTED
			return s.sendErrorAndEnd(c, fmt.Sprintf("Stream read error: %s", err.Error()))
	REDACTED

		line = strings.TrimSpace(line)
		if line == "" || !sseDataPrefix.MatchString(line) {
			continue
	REDACTED

		jsonStr := sseDataPrefix.ReplaceAllString(line, "")
		if jsonStr == "[DONE]" {
			s.sendEvent(c, TestEvent{Type: "test_complete", Success: trueREDACTED)
			return nil
	REDACTED

		var data map[string]any
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			continue
	REDACTED

		eventType, _ := data["type"].(string)

		switch eventType {
		case "content_block_delta":
			if delta, ok := data["delta"].(map[string]any); ok {
				if text, ok := delta["text"].(string); ok {
					s.sendEvent(c, TestEvent{Type: "content", Text: textREDACTED)
			REDACTED
		REDACTED
		case "message_stop":
			s.sendEvent(c, TestEvent{Type: "test_complete", Success: trueREDACTED)
			return nil
		case "error":
			errorMsg := "Unknown error"
			if errData, ok := data["error"].(map[string]any); ok {
				if msg, ok := errData["message"].(string); ok {
					errorMsg = msg
			REDACTED
		REDACTED
			return s.sendErrorAndEnd(c, errorMsg)
	REDACTED
REDACTED
REDACTED

// processOpenAIStream processes the SSE stream from OpenAI Responses API
func (s *AccountTestService) processOpenAIStream(c *gin.Context, body io.Reader) error {
	reader := bufio.NewReader(body)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				s.sendEvent(c, TestEvent{Type: "test_complete", Success: trueREDACTED)
				return nil
		REDACTED
			return s.sendErrorAndEnd(c, fmt.Sprintf("Stream read error: %s", err.Error()))
	REDACTED

		line = strings.TrimSpace(line)
		if line == "" || !sseDataPrefix.MatchString(line) {
			continue
	REDACTED

		jsonStr := sseDataPrefix.ReplaceAllString(line, "")
		if jsonStr == "[DONE]" {
			s.sendEvent(c, TestEvent{Type: "test_complete", Success: trueREDACTED)
			return nil
	REDACTED

		var data map[string]any
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			continue
	REDACTED

		eventType, _ := data["type"].(string)

		switch eventType {
		case "response.output_text.delta":
			// OpenAI Responses API uses "delta" field for text content
			if delta, ok := data["delta"].(string); ok && delta != "" {
				s.sendEvent(c, TestEvent{Type: "content", Text: deltaREDACTED)
		REDACTED
		case "response.completed":
			s.sendEvent(c, TestEvent{Type: "test_complete", Success: trueREDACTED)
			return nil
		case "error":
			errorMsg := "Unknown error"
			if errData, ok := data["error"].(map[string]any); ok {
				if msg, ok := errData["message"].(string); ok {
					errorMsg = msg
			REDACTED
		REDACTED
			return s.sendErrorAndEnd(c, errorMsg)
	REDACTED
REDACTED
REDACTED

// sendEvent sends a SSE event to the client
func (s *AccountTestService) sendEvent(c *gin.Context, event TestEvent) {
	eventJSON, _ := json.Marshal(event)
	if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", eventJSON); err != nil {
		log.Printf("failed to write SSE event: %v", err)
		return
REDACTED
	c.Writer.Flush()
REDACTED

// sendErrorAndEnd sends an error event and ends the stream
func (s *AccountTestService) sendErrorAndEnd(c *gin.Context, errorMsg string) error {
	log.Printf("Account test error: %s", errorMsg)
	s.sendEvent(c, TestEvent{Type: "error", Error: errorMsgREDACTED)
	return fmt.Errorf("%s", errorMsg)
REDACTED
