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
	"math"
	mathrand "math/rand"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
	"github.com/Wei-Shaw/sub2api/internal/pkg/googleapi"

	"github.com/gin-gonic/gin"
)

const geminiStickySessionTTL = time.Hour

const (
	geminiMaxRetries     = 5
	geminiRetryBaseDelay = 1 * time.Second
	geminiRetryMaxDelay  = 16 * time.Second
)

type GeminiMessagesCompatService struct {
	accountRepo      AccountRepository
	cache            GatewayCache
	tokenProvider    *GeminiTokenProvider
	rateLimitService *RateLimitService
	httpUpstream     HTTPUpstream
REDACTED

func NewGeminiMessagesCompatService(
	accountRepo AccountRepository,
	cache GatewayCache,
	tokenProvider *GeminiTokenProvider,
	rateLimitService *RateLimitService,
	httpUpstream HTTPUpstream,
) *GeminiMessagesCompatService {
	return &GeminiMessagesCompatService{
		accountRepo:      accountRepo,
		cache:            cache,
		tokenProvider:    tokenProvider,
		rateLimitService: rateLimitService,
		httpUpstream:     httpUpstream,
REDACTED
REDACTED

// GetTokenProvider returns the token provider for OAuth accounts
func (s *GeminiMessagesCompatService) GetTokenProvider() *GeminiTokenProvider {
	return s.tokenProvider
REDACTED

func (s *GeminiMessagesCompatService) SelectAccountForModel(ctx context.Context, groupID *int64, sessionHash string, requestedModel string) (*model.Account, error) {
	cacheKey := "gemini:" + sessionHash
	if sessionHash != "" {
		accountID, err := s.cache.GetSessionAccountID(ctx, cacheKey)
		if err == nil && accountID > 0 {
			account, err := s.accountRepo.GetByID(ctx, accountID)
			if err == nil && account.IsSchedulable() && account.Platform == model.PlatformGemini && (requestedModel == "" || account.IsModelSupported(requestedModel)) {
				_ = s.cache.RefreshSessionTTL(ctx, cacheKey, geminiStickySessionTTL)
				return account, nil
		REDACTED
	REDACTED
REDACTED

	var accounts []model.Account
	var err error
	if groupID != nil {
		accounts, err = s.accountRepo.ListSchedulableByGroupIDAndPlatform(ctx, *groupID, model.PlatformGemini)
REDACTED else {
		accounts, err = s.accountRepo.ListSchedulableByPlatform(ctx, model.PlatformGemini)
REDACTED
	if err != nil {
		return nil, fmt.Errorf("query accounts failed: %w", err)
REDACTED

	var selected *model.Account
	for i := range accounts {
		acc := &accounts[i]
		if requestedModel != "" && !acc.IsModelSupported(requestedModel) {
			continue
	REDACTED
		if selected == nil {
			selected = acc
			continue
	REDACTED
		if acc.Priority < selected.Priority {
			selected = acc
	REDACTED else if acc.Priority == selected.Priority {
			switch {
			case acc.LastUsedAt == nil && selected.LastUsedAt != nil:
				selected = acc
			case acc.LastUsedAt != nil && selected.LastUsedAt == nil:
				// keep selected (never used is preferred)
			case acc.LastUsedAt == nil && selected.LastUsedAt == nil:
				// Prefer OAuth accounts when both are unused (more compatible for Code Assist flows).
				if acc.Type == model.AccountTypeOAuth && selected.Type != model.AccountTypeOAuth {
					selected = acc
			REDACTED
			default:
				if acc.LastUsedAt.Before(*selected.LastUsedAt) {
					selected = acc
			REDACTED
		REDACTED
	REDACTED
REDACTED

	if selected == nil {
		if requestedModel != "" {
			return nil, fmt.Errorf("no available Gemini accounts supporting model: %s", requestedModel)
	REDACTED
		return nil, errors.New("no available Gemini accounts")
REDACTED

	if sessionHash != "" {
		_ = s.cache.SetSessionAccountID(ctx, cacheKey, selected.ID, geminiStickySessionTTL)
REDACTED

	return selected, nil
REDACTED

// SelectAccountForAIStudioEndpoints selects an account that is likely to succeed against
// generativelanguage.googleapis.com (e.g. GET /v1beta/models).
//
// Preference order:
// 1) API key accounts (AI Studio)
// 2) OAuth accounts without project_id (AI Studio OAuth)
// 3) OAuth accounts explicitly marked as ai_studio
// 4) Any remaining Gemini accounts (fallback)
func (s *GeminiMessagesCompatService) SelectAccountForAIStudioEndpoints(ctx context.Context, groupID *int64) (*model.Account, error) {
	var accounts []model.Account
	var err error
	if groupID != nil {
		accounts, err = s.accountRepo.ListSchedulableByGroupIDAndPlatform(ctx, *groupID, model.PlatformGemini)
REDACTED else {
		accounts, err = s.accountRepo.ListSchedulableByPlatform(ctx, model.PlatformGemini)
REDACTED
	if err != nil {
		return nil, fmt.Errorf("query accounts failed: %w", err)
REDACTED
	if len(accounts) == 0 {
		return nil, errors.New("no available Gemini accounts")
REDACTED

	rank := func(a *model.Account) int {
		if a == nil {
			return 999
	REDACTED
		switch a.Type {
		case model.AccountTypeApiKey:
			if strings.TrimSpace(a.GetCredential("api_key")) != "" {
				return 0
		REDACTED
			return 9
		case model.AccountTypeOAuth:
			if strings.TrimSpace(a.GetCredential("project_id")) == "" {
				return 1
		REDACTED
			if strings.TrimSpace(a.GetCredential("oauth_type")) == "ai_studio" {
				return 2
		REDACTED
			// Code Assist OAuth tokens often lack AI Studio scopes for models listing.
			return 3
		default:
			return 10
	REDACTED
REDACTED

	var selected *model.Account
	for i := range accounts {
		acc := &accounts[i]
		if selected == nil {
			selected = acc
			continue
	REDACTED

		r1, r2 := rank(acc), rank(selected)
		if r1 < r2 {
			selected = acc
			continue
	REDACTED
		if r1 > r2 {
			continue
	REDACTED

		if acc.Priority < selected.Priority {
			selected = acc
	REDACTED else if acc.Priority == selected.Priority {
			switch {
			case acc.LastUsedAt == nil && selected.LastUsedAt != nil:
				selected = acc
			case acc.LastUsedAt != nil && selected.LastUsedAt == nil:
				// keep selected
			case acc.LastUsedAt == nil && selected.LastUsedAt == nil:
				if acc.Type == model.AccountTypeOAuth && selected.Type != model.AccountTypeOAuth {
					selected = acc
			REDACTED
			default:
				if acc.LastUsedAt.Before(*selected.LastUsedAt) {
					selected = acc
			REDACTED
		REDACTED
	REDACTED
REDACTED

	if selected == nil {
		return nil, errors.New("no available Gemini accounts")
REDACTED
	return selected, nil
REDACTED

func (s *GeminiMessagesCompatService) Forward(ctx context.Context, c *gin.Context, account *model.Account, body []byte) (*ForwardResult, error) {
	startTime := time.Now()

	var req struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
REDACTED
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("parse request: %w", err)
REDACTED
	if strings.TrimSpace(req.Model) == "" {
		return nil, fmt.Errorf("missing model")
REDACTED

	originalModel := req.Model
	mappedModel := req.Model
	if account.Type == model.AccountTypeApiKey {
		mappedModel = account.GetMappedModel(req.Model)
REDACTED

	geminiReq, err := convertClaudeMessagesToGeminiGenerateContent(body)
	if err != nil {
		return nil, s.writeClaudeError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
REDACTED

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED

	var requestIDHeader string
	var buildReq func(ctx context.Context) (*http.Request, string, error)
	useUpstreamStream := req.Stream
	if account.Type == model.AccountTypeOAuth && !req.Stream && strings.TrimSpace(account.GetCredential("project_id")) != "" {
		// Code Assist's non-streaming generateContent may return no content; use streaming upstream and aggregate.
		useUpstreamStream = true
REDACTED

	switch account.Type {
	case model.AccountTypeApiKey:
		buildReq = func(ctx context.Context) (*http.Request, string, error) {
			apiKey := account.GetCredential("api_key")
			if strings.TrimSpace(apiKey) == "" {
				return nil, "", errors.New("gemini api_key not configured")
		REDACTED

			baseURL := strings.TrimRight(account.GetCredential("base_url"), "/")
			if baseURL == "" {
				baseURL = geminicli.AIStudioBaseURL
		REDACTED

			action := "generateContent"
			if req.Stream {
				action = "streamGenerateContent"
		REDACTED
			fullURL := fmt.Sprintf("%s/v1beta/models/%s:%s", strings.TrimRight(baseURL, "/"), mappedModel, action)
			if req.Stream {
				fullURL += "?alt=sse"
		REDACTED

			upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(geminiReq))
			if err != nil {
				return nil, "", err
		REDACTED
			upstreamReq.Header.Set("Content-Type", "application/json")
			upstreamReq.Header.Set("x-goog-api-key", apiKey)
			return upstreamReq, "x-request-id", nil
	REDACTED
		requestIDHeader = "x-request-id"

	case model.AccountTypeOAuth:
		buildReq = func(ctx context.Context) (*http.Request, string, error) {
			if s.tokenProvider == nil {
				return nil, "", errors.New("gemini token provider not configured")
		REDACTED
			accessToken, err := s.tokenProvider.GetAccessToken(ctx, account)
			if err != nil {
				return nil, "", err
		REDACTED

			projectID := strings.TrimSpace(account.GetCredential("project_id"))

			action := "generateContent"
			if useUpstreamStream {
				action = "streamGenerateContent"
		REDACTED

			// Two modes for OAuth:
			// 1. With project_id -> Code Assist API (wrapped request)
			// 2. Without project_id -> AI Studio API (direct OAuth, like API key but with Bearer token)
			if projectID != "" {
				// Mode 1: Code Assist API
				fullURL := fmt.Sprintf("%s/v1internal:%s", geminicli.GeminiCliBaseURL, action)
				if useUpstreamStream {
					fullURL += "?alt=sse"
			REDACTED

				wrapped := map[string]any{
					"model":   mappedModel,
					"project": projectID,
			REDACTED
				var inner any
				if err := json.Unmarshal(geminiReq, &inner); err != nil {
					return nil, "", fmt.Errorf("failed to parse gemini request: %w", err)
			REDACTED
				wrapped["request"] = inner
				wrappedBytes, _ := json.Marshal(wrapped)

				upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(wrappedBytes))
				if err != nil {
					return nil, "", err
			REDACTED
				upstreamReq.Header.Set("Content-Type", "application/json")
				upstreamReq.Header.Set("Authorization", "Bearer "+accessToken)
				upstreamReq.Header.Set("User-Agent", geminicli.GeminiCLIUserAgent)
				return upstreamReq, "x-request-id", nil
		REDACTED else {
				// Mode 2: AI Studio API with OAuth (like API key mode, but using Bearer token)
				baseURL := strings.TrimRight(account.GetCredential("base_url"), "/")
				if baseURL == "" {
					baseURL = geminicli.AIStudioBaseURL
			REDACTED

				fullURL := fmt.Sprintf("%s/v1beta/models/%s:%s", baseURL, mappedModel, action)
				if useUpstreamStream {
					fullURL += "?alt=sse"
			REDACTED

				upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(geminiReq))
				if err != nil {
					return nil, "", err
			REDACTED
				upstreamReq.Header.Set("Content-Type", "application/json")
				upstreamReq.Header.Set("Authorization", "Bearer "+accessToken)
				return upstreamReq, "x-request-id", nil
		REDACTED
	REDACTED
		requestIDHeader = "x-request-id"

	default:
		return nil, fmt.Errorf("unsupported account type: %s", account.Type)
REDACTED

	var resp *http.Response
	for attempt := 1; attempt <= geminiMaxRetries; attempt++ {
		upstreamReq, idHeader, err := buildReq(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, err
		REDACTED
			// Local build error: don't retry.
			if strings.Contains(err.Error(), "missing project_id") {
				return nil, s.writeClaudeError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		REDACTED
			return nil, s.writeClaudeError(c, http.StatusBadGateway, "upstream_error", err.Error())
	REDACTED
		requestIDHeader = idHeader

		resp, err = s.httpUpstream.Do(upstreamReq, proxyURL)
		if err != nil {
			if attempt < geminiMaxRetries {
				log.Printf("Gemini account %d: upstream request failed, retry %d/%d: %v", account.ID, attempt, geminiMaxRetries, err)
				sleepGeminiBackoff(attempt)
				continue
		REDACTED
			return nil, s.writeClaudeError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed after retries")
	REDACTED

		if resp.StatusCode >= 400 && s.shouldRetryGeminiUpstreamError(account, resp.StatusCode) {
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
			_ = resp.Body.Close()
			if resp.StatusCode == 429 {
				// Mark as rate-limited early so concurrent requests avoid this account.
				s.handleGeminiUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
		REDACTED
			if attempt < geminiMaxRetries {
				log.Printf("Gemini account %d: upstream status %d, retry %d/%d", account.ID, resp.StatusCode, attempt, geminiMaxRetries)
				sleepGeminiBackoff(attempt)
				continue
		REDACTED
			return nil, s.writeClaudeError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed after retries")
	REDACTED

		break
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		s.handleGeminiUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
		return nil, s.writeGeminiMappedError(c, resp.StatusCode, respBody)
REDACTED

	requestID := resp.Header.Get(requestIDHeader)
	if requestID == "" {
		requestID = resp.Header.Get("x-goog-request-id")
REDACTED
	if requestID != "" {
		c.Header("x-request-id", requestID)
REDACTED

	var usage *ClaudeUsage
	var firstTokenMs *int
	if req.Stream {
		streamRes, err := s.handleStreamingResponse(c, resp, startTime, originalModel)
		if err != nil {
			return nil, err
	REDACTED
		usage = streamRes.usage
		firstTokenMs = streamRes.firstTokenMs
REDACTED else {
		if useUpstreamStream {
			collected, usageObj, err := collectGeminiSSE(resp.Body, true)
			if err != nil {
				return nil, s.writeClaudeError(c, http.StatusBadGateway, "upstream_error", "Failed to read upstream stream")
		REDACTED
			claudeResp, usageObj2 := convertGeminiToClaudeMessage(collected, originalModel)
			c.JSON(http.StatusOK, claudeResp)
			usage = usageObj2
			if usageObj != nil && (usageObj.InputTokens > 0 || usageObj.OutputTokens > 0) {
				usage = usageObj
		REDACTED
	REDACTED else {
			usage, err = s.handleNonStreamingResponse(c, resp, originalModel)
			if err != nil {
				return nil, err
		REDACTED
	REDACTED
REDACTED

	return &ForwardResult{
		RequestID:    requestID,
		Usage:        *usage,
		Model:        originalModel,
		Stream:       req.Stream,
		Duration:     time.Since(startTime),
		FirstTokenMs: firstTokenMs,
REDACTED, nil
REDACTED

func (s *GeminiMessagesCompatService) ForwardNative(ctx context.Context, c *gin.Context, account *model.Account, originalModel string, action string, stream bool, body []byte) (*ForwardResult, error) {
	startTime := time.Now()

	if strings.TrimSpace(originalModel) == "" {
		return nil, s.writeGoogleError(c, http.StatusBadRequest, "Missing model in URL")
REDACTED
	if strings.TrimSpace(action) == "" {
		return nil, s.writeGoogleError(c, http.StatusBadRequest, "Missing action in URL")
REDACTED
	if len(body) == 0 {
		return nil, s.writeGoogleError(c, http.StatusBadRequest, "Request body is empty")
REDACTED

	switch action {
	case "generateContent", "streamGenerateContent", "countTokens":
		// ok
	default:
		return nil, s.writeGoogleError(c, http.StatusNotFound, "Unsupported action: "+action)
REDACTED

	mappedModel := originalModel
	if account.Type == model.AccountTypeApiKey {
		mappedModel = account.GetMappedModel(originalModel)
REDACTED

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED

	useUpstreamStream := stream
	upstreamAction := action
	if account.Type == model.AccountTypeOAuth && !stream && action == "generateContent" && strings.TrimSpace(account.GetCredential("project_id")) != "" {
		// Code Assist's non-streaming generateContent may return no content; use streaming upstream and aggregate.
		useUpstreamStream = true
		upstreamAction = "streamGenerateContent"
REDACTED
	forceAIStudio := action == "countTokens"

	var requestIDHeader string
	var buildReq func(ctx context.Context) (*http.Request, string, error)

	switch account.Type {
	case model.AccountTypeApiKey:
		buildReq = func(ctx context.Context) (*http.Request, string, error) {
			apiKey := account.GetCredential("api_key")
			if strings.TrimSpace(apiKey) == "" {
				return nil, "", errors.New("gemini api_key not configured")
		REDACTED

			baseURL := strings.TrimRight(account.GetCredential("base_url"), "/")
			if baseURL == "" {
				baseURL = geminicli.AIStudioBaseURL
		REDACTED

			fullURL := fmt.Sprintf("%s/v1beta/models/%s:%s", strings.TrimRight(baseURL, "/"), mappedModel, upstreamAction)
			if useUpstreamStream {
				fullURL += "?alt=sse"
		REDACTED

			upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(body))
			if err != nil {
				return nil, "", err
		REDACTED
			upstreamReq.Header.Set("Content-Type", "application/json")
			upstreamReq.Header.Set("x-goog-api-key", apiKey)
			return upstreamReq, "x-request-id", nil
	REDACTED
		requestIDHeader = "x-request-id"

	case model.AccountTypeOAuth:
		buildReq = func(ctx context.Context) (*http.Request, string, error) {
			if s.tokenProvider == nil {
				return nil, "", errors.New("gemini token provider not configured")
		REDACTED
			accessToken, err := s.tokenProvider.GetAccessToken(ctx, account)
			if err != nil {
				return nil, "", err
		REDACTED

			projectID := strings.TrimSpace(account.GetCredential("project_id"))

			// Two modes for OAuth:
			// 1. With project_id -> Code Assist API (wrapped request)
			// 2. Without project_id -> AI Studio API (direct OAuth, like API key but with Bearer token)
			if projectID != "" && !forceAIStudio {
				// Mode 1: Code Assist API
				fullURL := fmt.Sprintf("%s/v1internal:%s", geminicli.GeminiCliBaseURL, upstreamAction)
				if useUpstreamStream {
					fullURL += "?alt=sse"
			REDACTED

				wrapped := map[string]any{
					"model":   mappedModel,
					"project": projectID,
			REDACTED
				var inner any
				if err := json.Unmarshal(body, &inner); err != nil {
					return nil, "", fmt.Errorf("failed to parse gemini request: %w", err)
			REDACTED
				wrapped["request"] = inner
				wrappedBytes, _ := json.Marshal(wrapped)

				upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(wrappedBytes))
				if err != nil {
					return nil, "", err
			REDACTED
				upstreamReq.Header.Set("Content-Type", "application/json")
				upstreamReq.Header.Set("Authorization", "Bearer "+accessToken)
				upstreamReq.Header.Set("User-Agent", geminicli.GeminiCLIUserAgent)
				return upstreamReq, "x-request-id", nil
		REDACTED else {
				// Mode 2: AI Studio API with OAuth (like API key mode, but using Bearer token)
				baseURL := strings.TrimRight(account.GetCredential("base_url"), "/")
				if baseURL == "" {
					baseURL = geminicli.AIStudioBaseURL
			REDACTED

				fullURL := fmt.Sprintf("%s/v1beta/models/%s:%s", baseURL, mappedModel, upstreamAction)
				if useUpstreamStream {
					fullURL += "?alt=sse"
			REDACTED

				upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(body))
				if err != nil {
					return nil, "", err
			REDACTED
				upstreamReq.Header.Set("Content-Type", "application/json")
				upstreamReq.Header.Set("Authorization", "Bearer "+accessToken)
				return upstreamReq, "x-request-id", nil
		REDACTED
	REDACTED
		requestIDHeader = "x-request-id"

	default:
		return nil, s.writeGoogleError(c, http.StatusBadGateway, "Unsupported account type: "+account.Type)
REDACTED

	var resp *http.Response
	for attempt := 1; attempt <= geminiMaxRetries; attempt++ {
		upstreamReq, idHeader, err := buildReq(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, err
		REDACTED
			// Local build error: don't retry.
			if strings.Contains(err.Error(), "missing project_id") {
				return nil, s.writeGoogleError(c, http.StatusBadRequest, err.Error())
		REDACTED
			return nil, s.writeGoogleError(c, http.StatusBadGateway, err.Error())
	REDACTED
		requestIDHeader = idHeader

		resp, err = s.httpUpstream.Do(upstreamReq, proxyURL)
		if err != nil {
			if attempt < geminiMaxRetries {
				log.Printf("Gemini account %d: upstream request failed, retry %d/%d: %v", account.ID, attempt, geminiMaxRetries, err)
				sleepGeminiBackoff(attempt)
				continue
		REDACTED
			if action == "countTokens" {
				estimated := estimateGeminiCountTokens(body)
				c.JSON(http.StatusOK, map[string]any{"totalTokens": estimatedREDACTED)
				return &ForwardResult{
					RequestID:    "",
					Usage:        ClaudeUsage{REDACTED,
					Model:        originalModel,
					Stream:       false,
					Duration:     time.Since(startTime),
					FirstTokenMs: nil,
			REDACTED, nil
		REDACTED
			return nil, s.writeGoogleError(c, http.StatusBadGateway, "Upstream request failed after retries")
	REDACTED

		if resp.StatusCode >= 400 && s.shouldRetryGeminiUpstreamError(account, resp.StatusCode) {
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
			_ = resp.Body.Close()
			if resp.StatusCode == 429 {
				s.handleGeminiUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
		REDACTED
			if attempt < geminiMaxRetries {
				log.Printf("Gemini account %d: upstream status %d, retry %d/%d", account.ID, resp.StatusCode, attempt, geminiMaxRetries)
				sleepGeminiBackoff(attempt)
				continue
		REDACTED
			if action == "countTokens" {
				estimated := estimateGeminiCountTokens(body)
				c.JSON(http.StatusOK, map[string]any{"totalTokens": estimatedREDACTED)
				return &ForwardResult{
					RequestID:    "",
					Usage:        ClaudeUsage{REDACTED,
					Model:        originalModel,
					Stream:       false,
					Duration:     time.Since(startTime),
					FirstTokenMs: nil,
			REDACTED, nil
		REDACTED
			return nil, s.writeGoogleError(c, http.StatusBadGateway, "Upstream request failed after retries")
	REDACTED

		break
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	requestID := resp.Header.Get(requestIDHeader)
	if requestID == "" {
		requestID = resp.Header.Get("x-goog-request-id")
REDACTED
	if requestID != "" {
		c.Header("x-request-id", requestID)
REDACTED

	isOAuth := account.Type == model.AccountTypeOAuth

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		s.handleGeminiUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)

		// Best-effort fallback for OAuth tokens missing AI Studio scopes when calling countTokens.
		// This avoids Gemini SDKs failing hard during preflight token counting.
		if action == "countTokens" && isOAuth && isGeminiInsufficientScope(resp.Header, respBody) {
			estimated := estimateGeminiCountTokens(body)
			c.JSON(http.StatusOK, map[string]any{"totalTokens": estimatedREDACTED)
			return &ForwardResult{
				RequestID:    requestID,
				Usage:        ClaudeUsage{REDACTED,
				Model:        originalModel,
				Stream:       false,
				Duration:     time.Since(startTime),
				FirstTokenMs: nil,
		REDACTED, nil
	REDACTED

		respBody = unwrapIfNeeded(isOAuth, respBody)
		contentType := resp.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/json"
	REDACTED
		c.Data(resp.StatusCode, contentType, respBody)
		return nil, fmt.Errorf("gemini upstream error: %d", resp.StatusCode)
REDACTED

	var usage *ClaudeUsage
	var firstTokenMs *int

	if stream {
		streamRes, err := s.handleNativeStreamingResponse(c, resp, startTime, isOAuth)
		if err != nil {
			return nil, err
	REDACTED
		usage = streamRes.usage
		firstTokenMs = streamRes.firstTokenMs
REDACTED else {
		if useUpstreamStream {
			collected, usageObj, err := collectGeminiSSE(resp.Body, isOAuth)
			if err != nil {
				return nil, s.writeGoogleError(c, http.StatusBadGateway, "Failed to read upstream stream")
		REDACTED
			b, _ := json.Marshal(collected)
			c.Data(http.StatusOK, "application/json", b)
			usage = usageObj
	REDACTED else {
			usageResp, err := s.handleNativeNonStreamingResponse(c, resp, isOAuth)
			if err != nil {
				return nil, err
		REDACTED
			usage = usageResp
	REDACTED
REDACTED

	if usage == nil {
		usage = &ClaudeUsage{REDACTED
REDACTED

	return &ForwardResult{
		RequestID:    requestID,
		Usage:        *usage,
		Model:        originalModel,
		Stream:       stream,
		Duration:     time.Since(startTime),
		FirstTokenMs: firstTokenMs,
REDACTED, nil
REDACTED

func (s *GeminiMessagesCompatService) shouldRetryGeminiUpstreamError(account *model.Account, statusCode int) bool {
	switch statusCode {
	case 429, 500, 502, 503, 504, 529:
		return true
	case 403:
		// GeminiCli OAuth occasionally returns 403 transiently (activation/quota propagation); allow retry.
		return account != nil && account.Type == model.AccountTypeOAuth
	default:
		return false
REDACTED
REDACTED

func sleepGeminiBackoff(attempt int) {
	delay := geminiRetryBaseDelay * time.Duration(1<<uint(attempt-1))
	if delay > geminiRetryMaxDelay {
		delay = geminiRetryMaxDelay
REDACTED

	// +/- 20% jitter
	r := mathrand.New(mathrand.NewSource(time.Now().UnixNano()))
	jitter := time.Duration(float64(delay) * 0.2 * (r.Float64()*2 - 1))
	sleepFor := delay + jitter
	if sleepFor < 0 {
		sleepFor = 0
REDACTED
	time.Sleep(sleepFor)
REDACTED

func (s *GeminiMessagesCompatService) writeGeminiMappedError(c *gin.Context, upstreamStatus int, body []byte) error {
	var statusCode int
	var errType, errMsg string

	if mapped := mapGeminiErrorBodyToClaudeError(body); mapped != nil {
		errType = mapped.Type
		if mapped.Message != "" {
			errMsg = mapped.Message
	REDACTED
		if mapped.StatusCode > 0 {
			statusCode = mapped.StatusCode
	REDACTED
REDACTED

	switch upstreamStatus {
	case 400:
		if statusCode == 0 {
			statusCode = http.StatusBadRequest
	REDACTED
		if errType == "" {
			errType = "invalid_request_error"
	REDACTED
		if errMsg == "" {
			errMsg = "Invalid request"
	REDACTED
	case 401:
		if statusCode == 0 {
			statusCode = http.StatusBadGateway
	REDACTED
		if errType == "" {
			errType = "authentication_error"
	REDACTED
		if errMsg == "" {
			errMsg = "Upstream authentication failed, please contact administrator"
	REDACTED
	case 403:
		if statusCode == 0 {
			statusCode = http.StatusBadGateway
	REDACTED
		if errType == "" {
			errType = "permission_error"
	REDACTED
		if errMsg == "" {
			errMsg = "Upstream access forbidden, please contact administrator"
	REDACTED
	case 404:
		if statusCode == 0 {
			statusCode = http.StatusNotFound
	REDACTED
		if errType == "" {
			errType = "not_found_error"
	REDACTED
		if errMsg == "" {
			errMsg = "Resource not found"
	REDACTED
	case 429:
		if statusCode == 0 {
			statusCode = http.StatusTooManyRequests
	REDACTED
		if errType == "" {
			errType = "rate_limit_error"
	REDACTED
		if errMsg == "" {
			errMsg = "Upstream rate limit exceeded, please retry later"
	REDACTED
	case 529:
		if statusCode == 0 {
			statusCode = http.StatusServiceUnavailable
	REDACTED
		if errType == "" {
			errType = "overloaded_error"
	REDACTED
		if errMsg == "" {
			errMsg = "Upstream service overloaded, please retry later"
	REDACTED
	case 500, 502, 503, 504:
		if statusCode == 0 {
			statusCode = http.StatusBadGateway
	REDACTED
		if errType == "" {
			switch upstreamStatus {
			case 504:
				errType = "timeout_error"
			case 503:
				errType = "overloaded_error"
			default:
				errType = "api_error"
		REDACTED
	REDACTED
		if errMsg == "" {
			errMsg = "Upstream service temporarily unavailable"
	REDACTED
	default:
		if statusCode == 0 {
			statusCode = http.StatusBadGateway
	REDACTED
		if errType == "" {
			errType = "upstream_error"
	REDACTED
		if errMsg == "" {
			errMsg = "Upstream request failed"
	REDACTED
REDACTED

	c.JSON(statusCode, gin.H{
		"type":  "error",
		"error": gin.H{"type": errType, "message": errMsgREDACTED,
REDACTED)
	return fmt.Errorf("upstream error: %d", upstreamStatus)
REDACTED

type claudeErrorMapping struct {
	Type       string
	Message    string
	StatusCode int
REDACTED

func mapGeminiErrorBodyToClaudeError(body []byte) *claudeErrorMapping {
	if len(body) == 0 {
		return nil
REDACTED

	var parsed struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Status  string `json:"status"`
	REDACTED `json:"error"`
REDACTED
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil
REDACTED
	if strings.TrimSpace(parsed.Error.Status) == "" && parsed.Error.Code == 0 && strings.TrimSpace(parsed.Error.Message) == "" {
		return nil
REDACTED

	mapped := &claudeErrorMapping{
		Type:    mapGeminiStatusToClaudeErrorType(parsed.Error.Status),
		Message: "",
REDACTED
	if mapped.Type == "" {
		mapped.Type = "upstream_error"
REDACTED

	switch strings.ToUpper(strings.TrimSpace(parsed.Error.Status)) {
	case "INVALID_ARGUMENT":
		mapped.StatusCode = http.StatusBadRequest
	case "NOT_FOUND":
		mapped.StatusCode = http.StatusNotFound
	case "RESOURCE_EXHAUSTED":
		mapped.StatusCode = http.StatusTooManyRequests
	default:
		// Keep StatusCode unset and let HTTP status mapping decide.
REDACTED

	// Keep messages generic by default; upstream error message can be long or include sensitive fragments.
	return mapped
REDACTED

func mapGeminiStatusToClaudeErrorType(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "INVALID_ARGUMENT":
		return "invalid_request_error"
	case "PERMISSION_DENIED":
		return "permission_error"
	case "NOT_FOUND":
		return "not_found_error"
	case "RESOURCE_EXHAUSTED":
		return "rate_limit_error"
	case "UNAUTHENTICATED":
		return "authentication_error"
	case "UNAVAILABLE":
		return "overloaded_error"
	case "INTERNAL":
		return "api_error"
	case "DEADLINE_EXCEEDED":
		return "timeout_error"
	default:
		return ""
REDACTED
REDACTED

type geminiStreamResult struct {
	usage        *ClaudeUsage
	firstTokenMs *int
REDACTED

func (s *GeminiMessagesCompatService) handleNonStreamingResponse(c *gin.Context, resp *http.Response, originalModel string) (*ClaudeUsage, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, s.writeClaudeError(c, http.StatusBadGateway, "upstream_error", "Failed to read upstream response")
REDACTED

	geminiResp, err := unwrapGeminiResponse(body)
	if err != nil {
		return nil, s.writeClaudeError(c, http.StatusBadGateway, "upstream_error", "Failed to parse upstream response")
REDACTED

	claudeResp, usage := convertGeminiToClaudeMessage(geminiResp, originalModel)
	c.JSON(http.StatusOK, claudeResp)

	return usage, nil
REDACTED

func (s *GeminiMessagesCompatService) handleStreamingResponse(c *gin.Context, resp *http.Response, startTime time.Time, originalModel string) (*geminiStreamResult, error) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
REDACTED

	messageID := "msg_" + randomHex(12)
	messageStart := map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":            messageID,
			"type":          "message",
			"role":          "assistant",
			"model":         originalModel,
			"content":       []any{REDACTED,
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": map[string]any{
				"input_tokens":  0,
				"output_tokens": 0,
		REDACTED,
	REDACTED,
REDACTED
	writeSSE(c.Writer, "message_start", messageStart)
	flusher.Flush()

	var firstTokenMs *int
	var usage ClaudeUsage
	finishReason := ""
	sawToolUse := false

	nextBlockIndex := 0
	openBlockIndex := -1
	openBlockType := ""
	seenText := ""
	openToolIndex := -1
	openToolID := ""
	openToolName := ""
	seenToolJSON := ""

	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("stream read error: %w", err)
	REDACTED

		if !strings.HasPrefix(line, "data:") {
			if errors.Is(err, io.EOF) {
				break
		REDACTED
			continue
	REDACTED
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			if errors.Is(err, io.EOF) {
				break
		REDACTED
			continue
	REDACTED

		geminiResp, err := unwrapGeminiResponse([]byte(payload))
		if err != nil {
			continue
	REDACTED

		if fr := extractGeminiFinishReason(geminiResp); fr != "" {
			finishReason = fr
	REDACTED

		parts := extractGeminiParts(geminiResp)
		for _, part := range parts {
			if text, ok := part["text"].(string); ok && text != "" {
				delta, newSeen := computeGeminiTextDelta(seenText, text)
				seenText = newSeen
				if delta == "" {
					continue
			REDACTED

				if openBlockType != "text" {
					if openBlockIndex >= 0 {
						writeSSE(c.Writer, "content_block_stop", map[string]any{
							"type":  "content_block_stop",
							"index": openBlockIndex,
					REDACTED)
				REDACTED
					openBlockType = "text"
					openBlockIndex = nextBlockIndex
					nextBlockIndex++
					writeSSE(c.Writer, "content_block_start", map[string]any{
						"type":  "content_block_start",
						"index": openBlockIndex,
						"content_block": map[string]any{
							"type": "text",
							"text": "",
					REDACTED,
				REDACTED)
			REDACTED

				if firstTokenMs == nil {
					ms := int(time.Since(startTime).Milliseconds())
					firstTokenMs = &ms
			REDACTED
				writeSSE(c.Writer, "content_block_delta", map[string]any{
					"type":  "content_block_delta",
					"index": openBlockIndex,
					"delta": map[string]any{
						"type": "text_delta",
						"text": delta,
				REDACTED,
			REDACTED)
				flusher.Flush()
				continue
		REDACTED

			if fc, ok := part["functionCall"].(map[string]any); ok && fc != nil {
				name, _ := fc["name"].(string)
				args := fc["args"]
				if strings.TrimSpace(name) == "" {
					name = "tool"
			REDACTED

				// Close any open text block before tool_use.
				if openBlockIndex >= 0 {
					writeSSE(c.Writer, "content_block_stop", map[string]any{
						"type":  "content_block_stop",
						"index": openBlockIndex,
				REDACTED)
					openBlockIndex = -1
					openBlockType = ""
			REDACTED

				// If we receive streamed tool args in pieces, keep a single tool block open and emit deltas.
				if openToolIndex >= 0 && openToolName != name {
					writeSSE(c.Writer, "content_block_stop", map[string]any{
						"type":  "content_block_stop",
						"index": openToolIndex,
				REDACTED)
					openToolIndex = -1
					openToolName = ""
					seenToolJSON = ""
			REDACTED

				if openToolIndex < 0 {
					openToolID = "toolu_" + randomHex(8)
					openToolIndex = nextBlockIndex
					openToolName = name
					nextBlockIndex++
					sawToolUse = true

					writeSSE(c.Writer, "content_block_start", map[string]any{
						"type":  "content_block_start",
						"index": openToolIndex,
						"content_block": map[string]any{
							"type":  "tool_use",
							"id":    openToolID,
							"name":  name,
							"input": map[string]any{REDACTED,
					REDACTED,
				REDACTED)
			REDACTED

				argsJSONText := "{REDACTED"
				switch v := args.(type) {
				case nil:
					// keep default "{REDACTED"
				case string:
					if strings.TrimSpace(v) != "" {
						argsJSONText = v
				REDACTED
				default:
					if b, err := json.Marshal(args); err == nil && len(b) > 0 {
						argsJSONText = string(b)
				REDACTED
			REDACTED

				delta, newSeen := computeGeminiTextDelta(seenToolJSON, argsJSONText)
				seenToolJSON = newSeen
				if delta != "" {
					writeSSE(c.Writer, "content_block_delta", map[string]any{
						"type":  "content_block_delta",
						"index": openToolIndex,
						"delta": map[string]any{
							"type":         "input_json_delta",
							"partial_json": delta,
					REDACTED,
				REDACTED)
			REDACTED
				flusher.Flush()
		REDACTED
	REDACTED

		if u := extractGeminiUsage(geminiResp); u != nil {
			usage = *u
	REDACTED

		// Process the final unterminated line at EOF as well.
		if errors.Is(err, io.EOF) {
			break
	REDACTED
REDACTED

	if openBlockIndex >= 0 {
		writeSSE(c.Writer, "content_block_stop", map[string]any{
			"type":  "content_block_stop",
			"index": openBlockIndex,
	REDACTED)
REDACTED
	if openToolIndex >= 0 {
		writeSSE(c.Writer, "content_block_stop", map[string]any{
			"type":  "content_block_stop",
			"index": openToolIndex,
	REDACTED)
REDACTED

	stopReason := mapGeminiFinishReasonToClaudeStopReason(finishReason)
	if sawToolUse {
		stopReason = "tool_use"
REDACTED

	usageObj := map[string]any{
		"output_tokens": usage.OutputTokens,
REDACTED
	if usage.InputTokens > 0 {
		usageObj["input_tokens"] = usage.InputTokens
REDACTED
	writeSSE(c.Writer, "message_delta", map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason":   stopReason,
			"stop_sequence": nil,
	REDACTED,
		"usage": usageObj,
REDACTED)
	writeSSE(c.Writer, "message_stop", map[string]any{
		"type": "message_stop",
REDACTED)
	flusher.Flush()

	return &geminiStreamResult{usage: &usage, firstTokenMs: firstTokenMsREDACTED, nil
REDACTED

func writeSSE(w io.Writer, event string, data any) {
	if event != "" {
		_, _ = fmt.Fprintf(w, "event: %s\n", event)
REDACTED
	b, _ := json.Marshal(data)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", string(b))
REDACTED

func randomHex(nBytes int) string {
	b := make([]byte, nBytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
REDACTED

func (s *GeminiMessagesCompatService) writeClaudeError(c *gin.Context, status int, errType, message string) error {
	c.JSON(status, gin.H{
		"type":  "error",
		"error": gin.H{"type": errType, "message": messageREDACTED,
REDACTED)
	return fmt.Errorf("%s", message)
REDACTED

func (s *GeminiMessagesCompatService) writeGoogleError(c *gin.Context, status int, message string) error {
	c.JSON(status, gin.H{
		"error": gin.H{
			"code":    status,
			"message": message,
			"status":  googleapi.HTTPStatusToGoogleStatus(status),
	REDACTED,
REDACTED)
	return fmt.Errorf("%s", message)
REDACTED

func unwrapIfNeeded(isOAuth bool, raw []byte) []byte {
	if !isOAuth {
		return raw
REDACTED
	inner, err := unwrapGeminiResponse(raw)
	if err != nil {
		return raw
REDACTED
	b, err := json.Marshal(inner)
	if err != nil {
		return raw
REDACTED
	return b
REDACTED

func collectGeminiSSE(body io.Reader, isOAuth bool) (map[string]any, *ClaudeUsage, error) {
	reader := bufio.NewReader(body)

	var last map[string]any
	var lastWithParts map[string]any
	usage := &ClaudeUsage{REDACTED

	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			trimmed := strings.TrimRight(line, "\r\n")
			if strings.HasPrefix(trimmed, "data:") {
				payload := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
				switch payload {
				case "", "[DONE]":
					if payload == "[DONE]" {
						return pickGeminiCollectResult(last, lastWithParts), usage, nil
				REDACTED
				default:
					var parsed map[string]any
					if isOAuth {
						inner, err := unwrapGeminiResponse([]byte(payload))
						if err == nil && inner != nil {
							parsed = inner
					REDACTED
				REDACTED else {
						_ = json.Unmarshal([]byte(payload), &parsed)
				REDACTED
					if parsed != nil {
						last = parsed
						if u := extractGeminiUsage(parsed); u != nil {
							usage = u
					REDACTED
						if parts := extractGeminiParts(parsed); len(parts) > 0 {
							lastWithParts = parsed
					REDACTED
				REDACTED
			REDACTED
		REDACTED
	REDACTED

		if errors.Is(err, io.EOF) {
			break
	REDACTED
		if err != nil {
			return nil, nil, err
	REDACTED
REDACTED

	return pickGeminiCollectResult(last, lastWithParts), usage, nil
REDACTED

func pickGeminiCollectResult(last map[string]any, lastWithParts map[string]any) map[string]any {
	if lastWithParts != nil {
		return lastWithParts
REDACTED
	if last != nil {
		return last
REDACTED
	return map[string]any{REDACTED
REDACTED

type geminiNativeStreamResult struct {
	usage        *ClaudeUsage
	firstTokenMs *int
REDACTED

func isGeminiInsufficientScope(headers http.Header, body []byte) bool {
	if strings.Contains(strings.ToLower(headers.Get("Www-Authenticate")), "insufficient_scope") {
		return true
REDACTED
	lower := strings.ToLower(string(body))
	return strings.Contains(lower, "insufficient authentication scopes") || strings.Contains(lower, "access_token_scope_insufficient")
REDACTED

func estimateGeminiCountTokens(reqBody []byte) int {
	var obj map[string]any
	if err := json.Unmarshal(reqBody, &obj); err != nil {
		return 0
REDACTED

	var texts []string

	// systemInstruction.parts[].text
	if si, ok := obj["systemInstruction"].(map[string]any); ok {
		if parts, ok := si["parts"].([]any); ok {
			for _, p := range parts {
				if pm, ok := p.(map[string]any); ok {
					if t, ok := pm["text"].(string); ok && strings.TrimSpace(t) != "" {
						texts = append(texts, t)
				REDACTED
			REDACTED
		REDACTED
	REDACTED
REDACTED

	// contents[].parts[].text
	if contents, ok := obj["contents"].([]any); ok {
		for _, c := range contents {
			cm, ok := c.(map[string]any)
			if !ok {
				continue
		REDACTED
			parts, ok := cm["parts"].([]any)
			if !ok {
				continue
		REDACTED
			for _, p := range parts {
				pm, ok := p.(map[string]any)
				if !ok {
					continue
			REDACTED
				if t, ok := pm["text"].(string); ok && strings.TrimSpace(t) != "" {
					texts = append(texts, t)
			REDACTED
		REDACTED
	REDACTED
REDACTED

	total := 0
	for _, t := range texts {
		total += estimateTokensForText(t)
REDACTED
	if total < 0 {
		return 0
REDACTED
	return total
REDACTED

func estimateTokensForText(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
REDACTED
	runes := []rune(s)
	if len(runes) == 0 {
		return 0
REDACTED
	ascii := 0
	for _, r := range runes {
		if r <= 0x7f {
			ascii++
	REDACTED
REDACTED
	asciiRatio := float64(ascii) / float64(len(runes))
	if asciiRatio >= 0.8 {
		// Roughly 4 chars per token for English-like text.
		return (len(runes) + 3) / 4
REDACTED
	// For CJK-heavy text, approximate 1 rune per token.
	return len(runes)
REDACTED

type UpstreamHTTPResult struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
REDACTED

func (s *GeminiMessagesCompatService) handleNativeNonStreamingResponse(c *gin.Context, resp *http.Response, isOAuth bool) (*ClaudeUsage, error) {
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
REDACTED

	var parsed map[string]any
	if isOAuth {
		parsed, err = unwrapGeminiResponse(respBody)
		if err == nil && parsed != nil {
			respBody, _ = json.Marshal(parsed)
	REDACTED
REDACTED else {
		_ = json.Unmarshal(respBody, &parsed)
REDACTED

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
REDACTED
	c.Data(resp.StatusCode, contentType, respBody)

	if parsed != nil {
		if u := extractGeminiUsage(parsed); u != nil {
			return u, nil
	REDACTED
REDACTED
	return &ClaudeUsage{REDACTED, nil
REDACTED

func (s *GeminiMessagesCompatService) handleNativeStreamingResponse(c *gin.Context, resp *http.Response, startTime time.Time, isOAuth bool) (*geminiNativeStreamResult, error) {
	c.Status(resp.StatusCode)
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "text/event-stream; charset=utf-8"
REDACTED
	c.Header("Content-Type", contentType)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
REDACTED

	reader := bufio.NewReader(resp.Body)
	usage := &ClaudeUsage{REDACTED
	var firstTokenMs *int

	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			trimmed := strings.TrimRight(line, "\r\n")
			if strings.HasPrefix(trimmed, "data:") {
				payload := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
				// Keepalive / done markers
				if payload == "" || payload == "[DONE]" {
					_, _ = io.WriteString(c.Writer, line)
					flusher.Flush()
			REDACTED else {
					var rawToWrite string
					rawToWrite = payload

					var parsed map[string]any
					if isOAuth {
						inner, err := unwrapGeminiResponse([]byte(payload))
						if err == nil && inner != nil {
							parsed = inner
							if b, err := json.Marshal(inner); err == nil {
								rawToWrite = string(b)
						REDACTED
					REDACTED
				REDACTED else {
						_ = json.Unmarshal([]byte(payload), &parsed)
				REDACTED

					if parsed != nil {
						if u := extractGeminiUsage(parsed); u != nil {
							usage = u
					REDACTED
				REDACTED

					if firstTokenMs == nil {
						ms := int(time.Since(startTime).Milliseconds())
						firstTokenMs = &ms
				REDACTED

					if isOAuth {
						// SSE format requires double newline (\n\n) to separate events
						_, _ = fmt.Fprintf(c.Writer, "data: %s\n\n", rawToWrite)
				REDACTED else {
						// Pass-through for AI Studio responses.
						_, _ = io.WriteString(c.Writer, line)
				REDACTED
					flusher.Flush()
			REDACTED
		REDACTED else {
				_, _ = io.WriteString(c.Writer, line)
				flusher.Flush()
		REDACTED
	REDACTED

		if errors.Is(err, io.EOF) {
			break
	REDACTED
		if err != nil {
			return nil, err
	REDACTED
REDACTED

	return &geminiNativeStreamResult{usage: usage, firstTokenMs: firstTokenMsREDACTED, nil
REDACTED

// ForwardAIStudioGET forwards a GET request to AI Studio (generativelanguage.googleapis.com) for
// endpoints like /v1beta/models and /v1beta/models/{modelREDACTED.
//
// This is used to support Gemini SDKs that call models listing endpoints before generation.
func (s *GeminiMessagesCompatService) ForwardAIStudioGET(ctx context.Context, account *model.Account, path string) (*UpstreamHTTPResult, error) {
	if account == nil {
		return nil, errors.New("account is nil")
REDACTED
	path = strings.TrimSpace(path)
	if path == "" || !strings.HasPrefix(path, "/") {
		return nil, errors.New("invalid path")
REDACTED

	baseURL := strings.TrimRight(account.GetCredential("base_url"), "/")
	if baseURL == "" {
		baseURL = geminicli.AIStudioBaseURL
REDACTED
	fullURL := strings.TrimRight(baseURL, "/") + path

	var proxyURL string
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, err
REDACTED

	switch account.Type {
	case model.AccountTypeApiKey:
		apiKey := strings.TrimSpace(account.GetCredential("api_key"))
		if apiKey == "" {
			return nil, errors.New("gemini api_key not configured")
	REDACTED
		req.Header.Set("x-goog-api-key", apiKey)
	case model.AccountTypeOAuth:
		if s.tokenProvider == nil {
			return nil, errors.New("gemini token provider not configured")
	REDACTED
		accessToken, err := s.tokenProvider.GetAccessToken(ctx, account)
		if err != nil {
			return nil, err
	REDACTED
		req.Header.Set("Authorization", "Bearer "+accessToken)
	default:
		return nil, fmt.Errorf("unsupported account type: %s", account.Type)
REDACTED

	resp, err := s.httpUpstream.Do(req, proxyURL)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	return &UpstreamHTTPResult{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header.Clone(),
		Body:       body,
REDACTED, nil
REDACTED

func unwrapGeminiResponse(raw []byte) (map[string]any, error) {
	var outer map[string]any
	if err := json.Unmarshal(raw, &outer); err != nil {
		return nil, err
REDACTED
	if resp, ok := outer["response"].(map[string]any); ok && resp != nil {
		return resp, nil
REDACTED
	return outer, nil
REDACTED

func convertGeminiToClaudeMessage(geminiResp map[string]any, originalModel string) (map[string]any, *ClaudeUsage) {
	usage := extractGeminiUsage(geminiResp)
	if usage == nil {
		usage = &ClaudeUsage{REDACTED
REDACTED

	contentBlocks := make([]any, 0)
	sawToolUse := false
	if candidates, ok := geminiResp["candidates"].([]any); ok && len(candidates) > 0 {
		if cand, ok := candidates[0].(map[string]any); ok {
			if content, ok := cand["content"].(map[string]any); ok {
				if parts, ok := content["parts"].([]any); ok {
					for _, part := range parts {
						pm, ok := part.(map[string]any)
						if !ok {
							continue
					REDACTED
						if text, ok := pm["text"].(string); ok && text != "" {
							contentBlocks = append(contentBlocks, map[string]any{
								"type": "text",
								"text": text,
						REDACTED)
					REDACTED
						if fc, ok := pm["functionCall"].(map[string]any); ok {
							name, _ := fc["name"].(string)
							if strings.TrimSpace(name) == "" {
								name = "tool"
						REDACTED
							args := fc["args"]
							sawToolUse = true
							contentBlocks = append(contentBlocks, map[string]any{
								"type":  "tool_use",
								"id":    "toolu_" + randomHex(8),
								"name":  name,
								"input": args,
						REDACTED)
					REDACTED
				REDACTED
			REDACTED
		REDACTED
	REDACTED
REDACTED

	stopReason := mapGeminiFinishReasonToClaudeStopReason(extractGeminiFinishReason(geminiResp))
	if sawToolUse {
		stopReason = "tool_use"
REDACTED

	resp := map[string]any{
		"id":            "msg_" + randomHex(12),
		"type":          "message",
		"role":          "assistant",
		"model":         originalModel,
		"content":       contentBlocks,
		"stop_reason":   stopReason,
		"stop_sequence": nil,
		"usage": map[string]any{
			"input_tokens":  usage.InputTokens,
			"output_tokens": usage.OutputTokens,
	REDACTED,
REDACTED

	return resp, usage
REDACTED

func extractGeminiUsage(geminiResp map[string]any) *ClaudeUsage {
	usageMeta, ok := geminiResp["usageMetadata"].(map[string]any)
	if !ok || usageMeta == nil {
		return nil
REDACTED
	prompt, _ := asInt(usageMeta["promptTokenCount"])
	cand, _ := asInt(usageMeta["candidatesTokenCount"])
	return &ClaudeUsage{
		InputTokens:  prompt,
		OutputTokens: cand,
REDACTED
REDACTED

func asInt(v any) (int, bool) {
	switch t := v.(type) {
	case float64:
		return int(t), true
	case int:
		return t, true
	case int64:
		return int(t), true
	case json.Number:
		i, err := t.Int64()
		if err != nil {
			return 0, false
	REDACTED
		return int(i), true
	default:
		return 0, false
REDACTED
REDACTED

func (s *GeminiMessagesCompatService) handleGeminiUpstreamError(ctx context.Context, account *model.Account, statusCode int, headers http.Header, body []byte) {
	if s.rateLimitService != nil && (statusCode == 401 || statusCode == 403 || statusCode == 529) {
		s.rateLimitService.HandleUpstreamError(ctx, account, statusCode, headers, body)
		return
REDACTED
	if statusCode != 429 {
		return
REDACTED
	resetAt := parseGeminiRateLimitResetTime(body)
	if resetAt == nil {
		ra := time.Now().Add(5 * time.Minute)
		_ = s.accountRepo.SetRateLimited(ctx, account.ID, ra)
		return
REDACTED
	_ = s.accountRepo.SetRateLimited(ctx, account.ID, time.Unix(*resetAt, 0))
REDACTED

func parseGeminiRateLimitResetTime(body []byte) *int64 {
	// Try to parse metadata.quotaResetDelay like "12.345s"
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err == nil {
		if errObj, ok := parsed["error"].(map[string]any); ok {
			if msg, ok := errObj["message"].(string); ok {
				if looksLikeGeminiDailyQuota(msg) {
					if ts := nextGeminiDailyResetUnix(); ts != nil {
						return ts
				REDACTED
			REDACTED
		REDACTED
			if details, ok := errObj["details"].([]any); ok {
				for _, d := range details {
					dm, ok := d.(map[string]any)
					if !ok {
						continue
				REDACTED
					if meta, ok := dm["metadata"].(map[string]any); ok {
						if v, ok := meta["quotaResetDelay"].(string); ok {
							if dur, err := time.ParseDuration(v); err == nil {
								ts := time.Now().Unix() + int64(dur.Seconds())
								return &ts
						REDACTED
					REDACTED
				REDACTED
			REDACTED
		REDACTED
	REDACTED
REDACTED

	// Match "Please retry in Xs"
	retryInRegex := regexp.MustCompile(`Please retry in ([0-9.]+)s`)
	matches := retryInRegex.FindStringSubmatch(string(body))
	if len(matches) == 2 {
		if dur, err := time.ParseDuration(matches[1] + "s"); err == nil {
			ts := time.Now().Unix() + int64(math.Ceil(dur.Seconds()))
			return &ts
	REDACTED
REDACTED

	return nil
REDACTED

func looksLikeGeminiDailyQuota(message string) bool {
	m := strings.ToLower(message)
	if strings.Contains(m, "per day") || strings.Contains(m, "requests per day") || strings.Contains(m, "quota") && strings.Contains(m, "per day") {
		return true
REDACTED
	return false
REDACTED

func nextGeminiDailyResetUnix() *int64 {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		// Fallback: PST without DST.
		loc = time.FixedZone("PST", -8*3600)
REDACTED
	now := time.Now().In(loc)
	reset := time.Date(now.Year(), now.Month(), now.Day(), 0, 5, 0, 0, loc)
	if !reset.After(now) {
		reset = reset.Add(24 * time.Hour)
REDACTED
	ts := reset.Unix()
	return &ts
REDACTED

func extractGeminiFinishReason(geminiResp map[string]any) string {
	if candidates, ok := geminiResp["candidates"].([]any); ok && len(candidates) > 0 {
		if cand, ok := candidates[0].(map[string]any); ok {
			if fr, ok := cand["finishReason"].(string); ok {
				return fr
		REDACTED
	REDACTED
REDACTED
	return ""
REDACTED

func extractGeminiParts(geminiResp map[string]any) []map[string]any {
	if candidates, ok := geminiResp["candidates"].([]any); ok && len(candidates) > 0 {
		if cand, ok := candidates[0].(map[string]any); ok {
			if content, ok := cand["content"].(map[string]any); ok {
				if partsAny, ok := content["parts"].([]any); ok && len(partsAny) > 0 {
					out := make([]map[string]any, 0, len(partsAny))
					for _, p := range partsAny {
						pm, ok := p.(map[string]any)
						if !ok {
							continue
					REDACTED
						out = append(out, pm)
				REDACTED
					return out
			REDACTED
		REDACTED
	REDACTED
REDACTED
	return nil
REDACTED

func computeGeminiTextDelta(seen, incoming string) (delta, newSeen string) {
	incoming = strings.TrimSuffix(incoming, "\u0000")
	if incoming == "" {
		return "", seen
REDACTED

	// Cumulative mode: incoming contains full text so far.
	if strings.HasPrefix(incoming, seen) {
		return strings.TrimPrefix(incoming, seen), incoming
REDACTED
	// Duplicate/rewind: ignore.
	if strings.HasPrefix(seen, incoming) {
		return "", seen
REDACTED
	// Delta mode: treat incoming as incremental chunk.
	return incoming, seen + incoming
REDACTED

func mapGeminiFinishReasonToClaudeStopReason(finishReason string) string {
	switch strings.ToUpper(strings.TrimSpace(finishReason)) {
	case "MAX_TOKENS":
		return "max_tokens"
	case "STOP":
		return "end_turn"
	default:
		return "end_turn"
REDACTED
REDACTED

func convertClaudeMessagesToGeminiGenerateContent(body []byte) ([]byte, error) {
	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
REDACTED

	toolUseIDToName := make(map[string]string)

	systemText := extractClaudeSystemText(req["system"])
	contents, err := convertClaudeMessagesToGeminiContents(req["messages"], toolUseIDToName)
	if err != nil {
		return nil, err
REDACTED

	out := make(map[string]any)
	if systemText != "" {
		out["systemInstruction"] = map[string]any{
			"parts": []any{map[string]any{"text": systemTextREDACTEDREDACTED,
	REDACTED
REDACTED
	out["contents"] = contents

	if tools := convertClaudeToolsToGeminiTools(req["tools"]); tools != nil {
		out["tools"] = tools
REDACTED

	generationConfig := convertClaudeGenerationConfig(req)
	if generationConfig != nil {
		out["generationConfig"] = generationConfig
REDACTED

	stripGeminiFunctionIDs(out)
	return json.Marshal(out)
REDACTED

func stripGeminiFunctionIDs(req map[string]any) {
	// Defensive cleanup: some upstreams reject unexpected `id` fields in functionCall/functionResponse.
	contents, ok := req["contents"].([]any)
	if !ok {
		return
REDACTED
	for _, c := range contents {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
	REDACTED
		contentParts, ok := cm["parts"].([]any)
		if !ok {
			continue
	REDACTED
		for _, p := range contentParts {
			pm, ok := p.(map[string]any)
			if !ok {
				continue
		REDACTED
			if fc, ok := pm["functionCall"].(map[string]any); ok && fc != nil {
				delete(fc, "id")
		REDACTED
			if fr, ok := pm["functionResponse"].(map[string]any); ok && fr != nil {
				delete(fr, "id")
		REDACTED
	REDACTED
REDACTED
REDACTED

func extractClaudeSystemText(system any) string {
	switch v := system.(type) {
	case string:
		return strings.TrimSpace(v)
	case []any:
		var parts []string
		for _, p := range v {
			pm, ok := p.(map[string]any)
			if !ok {
				continue
		REDACTED
			if t, _ := pm["type"].(string); t != "text" {
				continue
		REDACTED
			if text, ok := pm["text"].(string); ok && strings.TrimSpace(text) != "" {
				parts = append(parts, text)
		REDACTED
	REDACTED
		return strings.TrimSpace(strings.Join(parts, "\n"))
	default:
		return ""
REDACTED
REDACTED

func convertClaudeMessagesToGeminiContents(messages any, toolUseIDToName map[string]string) ([]any, error) {
	arr, ok := messages.([]any)
	if !ok {
		return nil, errors.New("messages must be an array")
REDACTED

	out := make([]any, 0, len(arr))
	for _, m := range arr {
		mm, ok := m.(map[string]any)
		if !ok {
			continue
	REDACTED
		role, _ := mm["role"].(string)
		role = strings.ToLower(strings.TrimSpace(role))
		gRole := "user"
		if role == "assistant" {
			gRole = "model"
	REDACTED

		parts := make([]any, 0)
		switch content := mm["content"].(type) {
		case string:
			if strings.TrimSpace(content) != "" {
				parts = append(parts, map[string]any{"text": contentREDACTED)
		REDACTED
		case []any:
			for _, block := range content {
				bm, ok := block.(map[string]any)
				if !ok {
					continue
			REDACTED
				bt, _ := bm["type"].(string)
				switch bt {
				case "text":
					if text, ok := bm["text"].(string); ok && strings.TrimSpace(text) != "" {
						parts = append(parts, map[string]any{"text": textREDACTED)
				REDACTED
				case "tool_use":
					id, _ := bm["id"].(string)
					name, _ := bm["name"].(string)
					if strings.TrimSpace(id) != "" && strings.TrimSpace(name) != "" {
						toolUseIDToName[id] = name
				REDACTED
					parts = append(parts, map[string]any{
						"functionCall": map[string]any{
							"name": name,
							"args": bm["input"],
					REDACTED,
				REDACTED)
				case "tool_result":
					toolUseID, _ := bm["tool_use_id"].(string)
					name := toolUseIDToName[toolUseID]
					if name == "" {
						name = "tool"
				REDACTED
					parts = append(parts, map[string]any{
						"functionResponse": map[string]any{
							"name": name,
							"response": map[string]any{
								"content": extractClaudeContentText(bm["content"]),
						REDACTED,
					REDACTED,
				REDACTED)
				case "image":
					if src, ok := bm["source"].(map[string]any); ok {
						if srcType, _ := src["type"].(string); srcType == "base64" {
							mediaType, _ := src["media_type"].(string)
							data, _ := src["data"].(string)
							if mediaType != "" && data != "" {
								parts = append(parts, map[string]any{
									"inlineData": map[string]any{
										"mimeType": mediaType,
										"data":     data,
								REDACTED,
							REDACTED)
						REDACTED
					REDACTED
				REDACTED
				default:
					// best-effort: preserve unknown blocks as text
					if b, err := json.Marshal(bm); err == nil {
						parts = append(parts, map[string]any{"text": string(b)REDACTED)
				REDACTED
			REDACTED
		REDACTED
		default:
			// ignore
	REDACTED

		out = append(out, map[string]any{
			"role":  gRole,
			"parts": parts,
	REDACTED)
REDACTED
	return out, nil
REDACTED

func extractClaudeContentText(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		var sb strings.Builder
		for _, part := range t {
			pm, ok := part.(map[string]any)
			if !ok {
				continue
		REDACTED
			if pm["type"] == "text" {
				if text, ok := pm["text"].(string); ok {
					_, _ = sb.WriteString(text)
			REDACTED
		REDACTED
	REDACTED
		return sb.String()
	default:
		b, _ := json.Marshal(t)
		return string(b)
REDACTED
REDACTED

func convertClaudeToolsToGeminiTools(tools any) []any {
	arr, ok := tools.([]any)
	if !ok || len(arr) == 0 {
		return nil
REDACTED

	funcDecls := make([]any, 0, len(arr))
	for _, t := range arr {
		tm, ok := t.(map[string]any)
		if !ok {
			continue
	REDACTED
		name, _ := tm["name"].(string)
		desc, _ := tm["description"].(string)
		params := tm["input_schema"]
		if name == "" {
			continue
	REDACTED
		funcDecls = append(funcDecls, map[string]any{
			"name":        name,
			"description": desc,
			"parameters":  params,
	REDACTED)
REDACTED

	if len(funcDecls) == 0 {
		return nil
REDACTED
	return []any{
		map[string]any{
			"functionDeclarations": funcDecls,
	REDACTED,
REDACTED
REDACTED

func convertClaudeGenerationConfig(req map[string]any) map[string]any {
	out := make(map[string]any)
	if mt, ok := asInt(req["max_tokens"]); ok && mt > 0 {
		out["maxOutputTokens"] = mt
REDACTED
	if temp, ok := req["temperature"].(float64); ok {
		out["temperature"] = temp
REDACTED
	if topP, ok := req["top_p"].(float64); ok {
		out["topP"] = topP
REDACTED
	if stopSeq, ok := req["stop_sequences"].([]any); ok && len(stopSeq) > 0 {
		out["stopSequences"] = stopSeq
REDACTED
	if len(out) == 0 {
		return nil
REDACTED
	return out
REDACTED
