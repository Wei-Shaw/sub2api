package service

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"sub2api/internal/pkg/claude"
	"sub2api/internal/repository"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	testClaudeAPIURL = "https://api.anthropic.com/v1/messages"
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
	repos        *repository.Repositories
	oauthService *OAuthService
	httpClient   *http.Client
REDACTED

// NewAccountTestService creates a new AccountTestService
func NewAccountTestService(repos *repository.Repositories, oauthService *OAuthService) *AccountTestService {
	return &AccountTestService{
		repos:        repos,
		oauthService: oauthService,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
	REDACTED,
REDACTED
REDACTED

// generateSessionString generates a Claude Code style session string
func generateSessionString() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	hex64 := hex.EncodeToString(bytes)
	sessionUUID := uuid.New().String()
	return fmt.Sprintf("user_%s_account__session_%s", hex64, sessionUUID)
REDACTED

// createTestPayload creates a Claude Code style test request payload
func createTestPayload(modelID string) map[string]interface{REDACTED {
	return map[string]interface{REDACTED{
		"model": modelID,
		"messages": []map[string]interface{REDACTED{
			{
				"role": "user",
				"content": []map[string]interface{REDACTED{
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
		"system": []map[string]interface{REDACTED{
			{
				"type": "text",
				"text": "You are Claude Code, Anthropic's official CLI for Claude.",
				"cache_control": map[string]string{
					"type": "ephemeral",
			REDACTED,
		REDACTED,
	REDACTED,
		"metadata": map[string]string{
			"user_id": generateSessionString(),
	REDACTED,
		"max_tokens":  1024,
		"temperature": 1,
		"stream":      true,
REDACTED
REDACTED

// TestAccountConnection tests an account's connection by sending a test request
// All account types use full Claude Code client characteristics, only auth header differs
// modelID is optional - if empty, defaults to claude.DefaultTestModel
func (s *AccountTestService) TestAccountConnection(c *gin.Context, accountID int64, modelID string) error {
	ctx := c.Request.Context()

	// Get account
	account, err := s.repos.Account.GetByID(ctx, accountID)
	if err != nil {
		return s.sendErrorAndEnd(c, "Account not found")
REDACTED

	// Determine the model to use
	testModelID := modelID
	if testModelID == "" {
		testModelID = claude.DefaultTestModel
REDACTED

	// For API Key accounts with model mapping, map the model
	if account.Type == "apikey" {
		mapping := account.GetModelMapping()
		if mapping != nil && len(mapping) > 0 {
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

		// Check if token needs refresh
		needRefresh := false
		if expiresAtStr := account.GetCredential("expires_at"); expiresAtStr != "" {
			expiresAt, err := strconv.ParseInt(expiresAtStr, 10, 64)
			if err == nil && time.Now().Unix()+300 > expiresAt {
				needRefresh = true
		REDACTED
	REDACTED

		if needRefresh && s.oauthService != nil {
			tokenInfo, err := s.oauthService.RefreshAccountToken(ctx, account)
			if err != nil {
				return s.sendErrorAndEnd(c, fmt.Sprintf("Failed to refresh token: %s", err.Error()))
		REDACTED
			authToken = tokenInfo.AccessToken
	REDACTED
REDACTED else if account.Type == "apikey" {
		// API Key - use x-api-key header
		useBearer = false
		authToken = account.GetCredential("api_key")
		if authToken == "" {
			return s.sendErrorAndEnd(c, "No API key available")
	REDACTED

		apiURL = account.GetBaseURL()
		if apiURL == "" {
			apiURL = "https://api.anthropic.com"
	REDACTED
		apiURL = strings.TrimSuffix(apiURL, "/") + "/v1/messages"
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
	payload := createTestPayload(testModelID)
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
	req.Header.Set("anthropic-beta", claude.DefaultBetaHeader)

	// Apply Claude Code client headers
	for key, value := range claude.DefaultHeaders {
		req.Header.Set(key, value)
REDACTED

	// Set authentication header
	if useBearer {
		req.Header.Set("Authorization", "Bearer "+authToken)
REDACTED else {
		req.Header.Set("x-api-key", authToken)
REDACTED

	// Configure proxy if account has one
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL := account.Proxy.URL()
		if proxyURL != "" {
			if parsedURL, err := url.Parse(proxyURL); err == nil {
				transport.Proxy = http.ProxyURL(parsedURL)
		REDACTED
	REDACTED
REDACTED

	client := &http.Client{
		Transport: transport,
		Timeout:   60 * time.Second,
REDACTED

	resp, err := client.Do(req)
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Request failed: %s", err.Error()))
REDACTED
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return s.sendErrorAndEnd(c, fmt.Sprintf("API returned %d: %s", resp.StatusCode, string(body)))
REDACTED

	// Process SSE stream
	return s.processStream(c, resp.Body)
REDACTED

// processStream processes the SSE stream from Claude API
func (s *AccountTestService) processStream(c *gin.Context, body io.Reader) error {
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

		var data map[string]interface{REDACTED
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			continue
	REDACTED

		eventType, _ := data["type"].(string)

		switch eventType {
		case "content_block_delta":
			if delta, ok := data["delta"].(map[string]interface{REDACTED); ok {
				if text, ok := delta["text"].(string); ok {
					s.sendEvent(c, TestEvent{Type: "content", Text: textREDACTED)
			REDACTED
		REDACTED
		case "message_stop":
			s.sendEvent(c, TestEvent{Type: "test_complete", Success: trueREDACTED)
			return nil
		case "error":
			errorMsg := "Unknown error"
			if errData, ok := data["error"].(map[string]interface{REDACTED); ok {
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
	fmt.Fprintf(c.Writer, "data: %s\n\n", eventJSON)
	c.Writer.Flush()
REDACTED

// sendErrorAndEnd sends an error event and ends the stream
func (s *AccountTestService) sendErrorAndEnd(c *gin.Context, errorMsg string) error {
	log.Printf("Account test error: %s", errorMsg)
	s.sendEvent(c, TestEvent{Type: "error", Error: errorMsgREDACTED)
	return fmt.Errorf("%s", errorMsg)
REDACTED
