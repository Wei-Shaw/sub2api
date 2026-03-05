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
	"github.com/Wei-Shaw/sub2api/internal/util/soraerror"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// sseDataPrefix matches SSE data lines with optional whitespace after colon.
// Some upstream APIs return non-standard "data:" without space (should be "data: ").
var sseDataPrefix = regexp.MustCompile(`^data:\s*`)

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
	Status  string `json:"status,omitempty"`
	Code    string `json:"code,omitempty"`
	Data    any    `json:"data,omitempty"`
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
	soraTestGuardMu           sync.Mutex
	soraTestLastRun           map[int64]time.Time
	soraTestCooldown          time.Duration
REDACTED

const defaultSoraTestCooldown = 10 * time.Second

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
		soraTestLastRun:           make(map[int64]time.Time),
		soraTestCooldown:          defaultSoraTestCooldown,
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

type soraProbeStep struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	HTTPStatus int    `json:"http_status,omitempty"`
	ErrorCode  string `json:"error_code,omitempty"`
	Message    string `json:"message,omitempty"`
REDACTED

type soraProbeSummary struct {
	Status string          `json:"status"`
	Steps  []soraProbeStep `json:"steps"`
REDACTED

type soraProbeRecorder struct {
	steps []soraProbeStep
REDACTED

func (r *soraProbeRecorder) addStep(name, status string, httpStatus int, errorCode, message string) {
	r.steps = append(r.steps, soraProbeStep{
		Name:       name,
		Status:     status,
		HTTPStatus: httpStatus,
		ErrorCode:  strings.TrimSpace(errorCode),
		Message:    strings.TrimSpace(message),
REDACTED)
REDACTED

func (r *soraProbeRecorder) finalize() soraProbeSummary {
	meSuccess := false
	partial := false
	for _, step := range r.steps {
		if step.Name == "me" {
			meSuccess = strings.EqualFold(step.Status, "success")
			continue
	REDACTED
		if strings.EqualFold(step.Status, "failed") {
			partial = true
	REDACTED
REDACTED

	status := "success"
	if !meSuccess {
		status = "failed"
REDACTED else if partial {
		status = "partial_success"
REDACTED

	return soraProbeSummary{
		Status: status,
		Steps:  append([]soraProbeStep(nil), r.steps...),
REDACTED
REDACTED

func (s *AccountTestService) emitSoraProbeSummary(c *gin.Context, rec *soraProbeRecorder) {
	if rec == nil {
		return
REDACTED
	summary := rec.finalize()
	code := ""
	for _, step := range summary.Steps {
		if strings.EqualFold(step.Status, "failed") && strings.TrimSpace(step.ErrorCode) != "" {
			code = step.ErrorCode
			break
	REDACTED
REDACTED
	s.sendEvent(c, TestEvent{
		Type:   "sora_test_result",
		Status: summary.Status,
		Code:   code,
		Data:   summary,
REDACTED)
REDACTED

func (s *AccountTestService) acquireSoraTestPermit(accountID int64) (time.Duration, bool) {
	if accountID <= 0 {
		return 0, true
REDACTED
	s.soraTestGuardMu.Lock()
	defer s.soraTestGuardMu.Unlock()

	if s.soraTestLastRun == nil {
		s.soraTestLastRun = make(map[int64]time.Time)
REDACTED
	cooldown := s.soraTestCooldown
	if cooldown <= 0 {
		cooldown = defaultSoraTestCooldown
REDACTED

	now := time.Now()
	if lastRun, ok := s.soraTestLastRun[accountID]; ok {
		elapsed := now.Sub(lastRun)
		if elapsed < cooldown {
			return cooldown - elapsed, false
	REDACTED
REDACTED
	s.soraTestLastRun[accountID] = now
	return 0, true
REDACTED

func ceilSeconds(d time.Duration) int {
	if d <= 0 {
		return 1
REDACTED
	sec := int(d / time.Second)
	if d%time.Second != 0 {
		sec++
REDACTED
	if sec < 1 {
		sec = 1
REDACTED
	return sec
REDACTED

// testSoraAPIKeyAccountConnection 测试 Sora apikey 类型账号的连通性。
// 向上游 base_url 发送轻量级 prompt-enhance 请求验证连通性和 API Key 有效性。
func (s *AccountTestService) testSoraAPIKeyAccountConnection(c *gin.Context, account *Account) error {
	ctx := c.Request.Context()

	apiKey := account.GetCredential("api_key")
	if apiKey == "" {
		return s.sendErrorAndEnd(c, "Sora apikey 账号缺少 api_key 凭证")
REDACTED

	baseURL := account.GetBaseURL()
	if baseURL == "" {
		return s.sendErrorAndEnd(c, "Sora apikey 账号缺少 base_url")
REDACTED

	// 验证 base_url 格式
	normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("base_url 无效: %s", err.Error()))
REDACTED
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
REDACTED

	s.sendEvent(c, TestEvent{Type: "test_start", Model: "sora-upstream"REDACTED)

	// 构建轻量级 prompt-enhance 请求作为连通性测试
	testPayload := map[string]any{
		"model":    "prompt-enhance-short-10s",
		"messages": []map[string]string{{"role": "user", "content": "test"REDACTEDREDACTED,
		"stream":   false,
REDACTED
	payloadBytes, _ := json.Marshal(testPayload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return s.sendErrorAndEnd(c, "构建测试请求失败")
REDACTED
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	// 获取代理 URL
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED

	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("上游连接失败: %s", err.Error()))
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))

	if resp.StatusCode == http.StatusOK {
		s.sendEvent(c, TestEvent{Type: "content", Text: fmt.Sprintf("上游连接成功 (%s)", upstreamURL)REDACTED)
		s.sendEvent(c, TestEvent{Type: "content", Text: fmt.Sprintf("API Key 有效 (HTTP %d)", resp.StatusCode)REDACTED)
		s.sendEvent(c, TestEvent{Type: "test_complete", Success: trueREDACTED)
		return nil
REDACTED

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return s.sendErrorAndEnd(c, fmt.Sprintf("上游认证失败 (HTTP %d)，请检查 API Key 是否正确", resp.StatusCode))
REDACTED

	// 其他错误但能连通（如 400 参数错误）也算连通性测试通过
	if resp.StatusCode == http.StatusBadRequest {
		s.sendEvent(c, TestEvent{Type: "content", Text: fmt.Sprintf("上游连接成功 (%s)", upstreamURL)REDACTED)
		s.sendEvent(c, TestEvent{Type: "content", Text: fmt.Sprintf("API Key 有效（上游返回 %d，参数校验错误属正常）", resp.StatusCode)REDACTED)
		s.sendEvent(c, TestEvent{Type: "test_complete", Success: trueREDACTED)
		return nil
REDACTED

	return s.sendErrorAndEnd(c, fmt.Sprintf("上游返回异常 HTTP %d: %s", resp.StatusCode, truncateSoraErrorBody(respBody, 256)))
REDACTED

// testSoraAccountConnection 测试 Sora 账号的连接
// OAuth 类型：调用 /backend/me 接口验证 access_token 有效性
// APIKey 类型：向上游 base_url 发送轻量级 prompt-enhance 请求验证连通性
func (s *AccountTestService) testSoraAccountConnection(c *gin.Context, account *Account) error {
	// apikey 类型走独立测试流程
	if account.Type == AccountTypeAPIKey {
		return s.testSoraAPIKeyAccountConnection(c, account)
REDACTED

	ctx := c.Request.Context()
	recorder := &soraProbeRecorder{REDACTED

	authToken := account.GetCredential("access_token")
	if authToken == "" {
		recorder.addStep("me", "failed", http.StatusUnauthorized, "missing_access_token", "No access token available")
		s.emitSoraProbeSummary(c, recorder)
		return s.sendErrorAndEnd(c, "No access token available")
REDACTED

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
REDACTED

	// Send test_start event
	s.sendEvent(c, TestEvent{Type: "test_start", Model: "sora"REDACTED)

	req, err := http.NewRequestWithContext(ctx, "GET", soraMeAPIURL, nil)
	if err != nil {
		recorder.addStep("me", "failed", 0, "request_build_failed", err.Error())
		s.emitSoraProbeSummary(c, recorder)
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
		recorder.addStep("me", "failed", 0, "network_error", err.Error())
		s.emitSoraProbeSummary(c, recorder)
		return s.sendErrorAndEnd(c, fmt.Sprintf("Request failed: %s", err.Error()))
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		if isCloudflareChallengeResponse(resp.StatusCode, resp.Header, body) {
			recorder.addStep("me", "failed", resp.StatusCode, "cf_challenge", "Cloudflare challenge detected")
			s.emitSoraProbeSummary(c, recorder)
			s.logSoraCloudflareChallenge(account, proxyURL, soraMeAPIURL, resp.Header, body)
			return s.sendErrorAndEnd(c, formatCloudflareChallengeMessage(fmt.Sprintf("Sora request blocked by Cloudflare challenge (HTTP %d). Please switch to a clean proxy/network and retry.", resp.StatusCode), resp.Header, body))
	REDACTED
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
	REDACTED
REDACTED
	recorder.addStep("me", "success", resp.StatusCode, "", "me endpoint ok")

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
			recorder.addStep("subscription", "failed", 0, "network_error", subErr.Error())
			s.sendEvent(c, TestEvent{Type: "content", Text: fmt.Sprintf("Subscription check skipped: %s", subErr.Error())REDACTED)
	REDACTED else {
			subBody, _ := io.ReadAll(subResp.Body)
			_ = subResp.Body.Close()
			if subResp.StatusCode == http.StatusOK {
				recorder.addStep("subscription", "success", subResp.StatusCode, "", "subscription endpoint ok")
				if summary := parseSoraSubscriptionSummary(subBody); summary != "" {
					s.sendEvent(c, TestEvent{Type: "content", Text: summaryREDACTED)
			REDACTED else {
					s.sendEvent(c, TestEvent{Type: "content", Text: "Subscription check OK"REDACTED)
			REDACTED
		REDACTED else {
				if isCloudflareChallengeResponse(subResp.StatusCode, subResp.Header, subBody) {
					recorder.addStep("subscription", "failed", subResp.StatusCode, "cf_challenge", "Cloudflare challenge detected")
					s.logSoraCloudflareChallenge(account, proxyURL, soraBillingAPIURL, subResp.Header, subBody)
					s.sendEvent(c, TestEvent{Type: "content", Text: formatCloudflareChallengeMessage(fmt.Sprintf("Subscription check blocked by Cloudflare challenge (HTTP %d)", subResp.StatusCode), subResp.Header, subBody)REDACTED)
			REDACTED else {
					upstreamCode, upstreamMessage := soraerror.ExtractUpstreamErrorCodeAndMessage(subBody)
					recorder.addStep("subscription", "failed", subResp.StatusCode, upstreamCode, upstreamMessage)
					s.sendEvent(c, TestEvent{Type: "content", Text: fmt.Sprintf("Subscription check returned %d", subResp.StatusCode)REDACTED)
			REDACTED
		REDACTED
	REDACTED
REDACTED

	// 追加 Sora2 能力探测（对齐 sora2api 的测试思路）：邀请码 + 剩余额度。
	s.testSora2Capabilities(c, ctx, account, authToken, proxyURL, enableSoraTLSFingerprint, recorder)

	s.emitSoraProbeSummary(c, recorder)
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
	recorder *soraProbeRecorder,
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
		if recorder != nil {
			recorder.addStep("sora2_invite", "failed", 0, "network_error", err.Error())
	REDACTED
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
			if recorder != nil {
				recorder.addStep("sora2_bootstrap", "success", bootstrapStatus, "", "bootstrap endpoint ok")
		REDACTED
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
				if recorder != nil {
					recorder.addStep("sora2_invite", "failed", 0, "network_error", err.Error())
			REDACTED
				s.sendEvent(c, TestEvent{Type: "content", Text: fmt.Sprintf("Sora2 invite retry failed: %s", err.Error())REDACTED)
				return
		REDACTED
	REDACTED else if recorder != nil {
			code := ""
			msg := ""
			if bootstrapErr != nil {
				code = "network_error"
				msg = bootstrapErr.Error()
		REDACTED
			recorder.addStep("sora2_bootstrap", "failed", bootstrapStatus, code, msg)
	REDACTED
REDACTED

	if inviteStatus != http.StatusOK {
		if isCloudflareChallengeResponse(inviteStatus, inviteHeader, inviteBody) {
			if recorder != nil {
				recorder.addStep("sora2_invite", "failed", inviteStatus, "cf_challenge", "Cloudflare challenge detected")
		REDACTED
			s.logSoraCloudflareChallenge(account, proxyURL, soraInviteMineURL, inviteHeader, inviteBody)
			s.sendEvent(c, TestEvent{Type: "content", Text: formatCloudflareChallengeMessage(fmt.Sprintf("Sora2 invite check blocked by Cloudflare challenge (HTTP %d)", inviteStatus), inviteHeader, inviteBody)REDACTED)
			return
	REDACTED
		upstreamCode, upstreamMessage := soraerror.ExtractUpstreamErrorCodeAndMessage(inviteBody)
		if recorder != nil {
			recorder.addStep("sora2_invite", "failed", inviteStatus, upstreamCode, upstreamMessage)
	REDACTED
		s.sendEvent(c, TestEvent{Type: "content", Text: fmt.Sprintf("Sora2 invite check returned %d", inviteStatus)REDACTED)
		return
REDACTED
	if recorder != nil {
		recorder.addStep("sora2_invite", "success", inviteStatus, "", "invite endpoint ok")
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
		if recorder != nil {
			recorder.addStep("sora2_remaining", "failed", 0, "network_error", remainingErr.Error())
	REDACTED
		s.sendEvent(c, TestEvent{Type: "content", Text: fmt.Sprintf("Sora2 remaining check skipped: %s", remainingErr.Error())REDACTED)
		return
REDACTED
	if remainingStatus != http.StatusOK {
		if isCloudflareChallengeResponse(remainingStatus, remainingHeader, remainingBody) {
			if recorder != nil {
				recorder.addStep("sora2_remaining", "failed", remainingStatus, "cf_challenge", "Cloudflare challenge detected")
		REDACTED
			s.logSoraCloudflareChallenge(account, proxyURL, soraRemainingURL, remainingHeader, remainingBody)
			s.sendEvent(c, TestEvent{Type: "content", Text: formatCloudflareChallengeMessage(fmt.Sprintf("Sora2 remaining check blocked by Cloudflare challenge (HTTP %d)", remainingStatus), remainingHeader, remainingBody)REDACTED)
			return
	REDACTED
		upstreamCode, upstreamMessage := soraerror.ExtractUpstreamErrorCodeAndMessage(remainingBody)
		if recorder != nil {
			recorder.addStep("sora2_remaining", "failed", remainingStatus, upstreamCode, upstreamMessage)
	REDACTED
		s.sendEvent(c, TestEvent{Type: "content", Text: fmt.Sprintf("Sora2 remaining check returned %d", remainingStatus)REDACTED)
		return
REDACTED
	if recorder != nil {
		recorder.addStep("sora2_remaining", "success", remainingStatus, "", "remaining endpoint ok")
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
		return true
REDACTED
	return !s.cfg.Sora.Client.DisableTLSFingerprint
REDACTED

func isCloudflareChallengeResponse(statusCode int, headers http.Header, body []byte) bool {
	return soraerror.IsCloudflareChallengeResponse(statusCode, headers, body)
REDACTED

func formatCloudflareChallengeMessage(base string, headers http.Header, body []byte) string {
	return soraerror.FormatCloudflareChallengeMessage(base, headers, body)
REDACTED

func extractCloudflareRayID(headers http.Header, body []byte) string {
	return soraerror.ExtractCloudflareRayID(headers, body)
REDACTED

func extractSoraEgressIPHint(headers http.Header) string {
	if headers == nil {
		return "unknown"
REDACTED
	candidates := []string{
		"x-openai-public-ip",
		"x-envoy-external-address",
		"cf-connecting-ip",
		"x-forwarded-for",
REDACTED
	for _, key := range candidates {
		if value := strings.TrimSpace(headers.Get(key)); value != "" {
			return value
	REDACTED
REDACTED
	return "unknown"
REDACTED

func sanitizeProxyURLForLog(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
REDACTED
	u, err := url.Parse(raw)
	if err != nil {
		return "<invalid_proxy_url>"
REDACTED
	if u.User != nil {
		u.User = nil
REDACTED
	return u.String()
REDACTED

func endpointPathForLog(endpoint string) string {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || parsed.Path == "" {
		return endpoint
REDACTED
	return parsed.Path
REDACTED

func (s *AccountTestService) logSoraCloudflareChallenge(account *Account, proxyURL, endpoint string, headers http.Header, body []byte) {
	accountID := int64(0)
	platform := ""
	proxyID := "none"
	if account != nil {
		accountID = account.ID
		platform = account.Platform
		if account.ProxyID != nil {
			proxyID = fmt.Sprintf("%d", *account.ProxyID)
	REDACTED
REDACTED
	cfRay := extractCloudflareRayID(headers, body)
	if cfRay == "" {
		cfRay = "unknown"
REDACTED
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
REDACTED

func truncateSoraErrorBody(body []byte, max int) string {
	return soraerror.TruncateBody(body, max)
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

// RunTestBackground executes an account test in-memory (no real HTTP client),
// capturing SSE output via httptest.NewRecorder, then parses the result.
func (s *AccountTestService) RunTestBackground(ctx context.Context, accountID int64, modelID string) (*ScheduledTestResult, error) {
	startedAt := time.Now()

	w := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(w)
	ginCtx.Request = (&http.Request{REDACTED).WithContext(ctx)

	testErr := s.TestAccountConnection(ginCtx, accountID, modelID)

	finishedAt := time.Now()
	body := w.Body.String()
	responseText, errMsg := parseTestSSEOutput(body)

	status := "success"
	if testErr != nil || errMsg != "" {
		status = "failed"
		if errMsg == "" && testErr != nil {
			errMsg = testErr.Error()
	REDACTED
REDACTED

	return &ScheduledTestResult{
		Status:       status,
		ResponseText: responseText,
		ErrorMessage: errMsg,
		LatencyMs:    finishedAt.Sub(startedAt).Milliseconds(),
		StartedAt:    startedAt,
		FinishedAt:   finishedAt,
REDACTED, nil
REDACTED

// parseTestSSEOutput extracts response text and error message from captured SSE output.
func parseTestSSEOutput(body string) (responseText, errMsg string) {
	var texts []string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
	REDACTED
		jsonStr := strings.TrimPrefix(line, "data: ")
		var event TestEvent
		if err := json.Unmarshal([]byte(jsonStr), &event); err != nil {
			continue
	REDACTED
		switch event.Type {
		case "content":
			if event.Text != "" {
				texts = append(texts, event.Text)
		REDACTED
		case "error":
			errMsg = event.Error
	REDACTED
REDACTED
	responseText = strings.Join(texts, "")
	return
REDACTED
