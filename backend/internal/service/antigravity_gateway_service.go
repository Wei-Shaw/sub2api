package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	antigravityStickySessionTTL = time.Hour
	antigravityMaxRetries       = 5
	antigravityRetryBaseDelay   = 1 * time.Second
	antigravityRetryMaxDelay    = 16 * time.Second
)

// Antigravity 直接支持的模型
var antigravitySupportedModels = map[string]bool{
	"claude-opus-4-5-thinking":   true,
	"claude-sonnet-4-5":          true,
	"claude-sonnet-4-5-thinking": true,
	"gemini-2.5-flash":           true,
	"gemini-2.5-flash-lite":      true,
	"gemini-2.5-flash-thinking":  true,
	"gemini-3-flash":             true,
	"gemini-3-pro-low":           true,
	"gemini-3-pro-high":          true,
	"gemini-3-pro-preview":       true,
	"gemini-3-pro-image":         true,
REDACTED

// Antigravity 系统默认模型映射表（不支持 → 支持）
var antigravityModelMapping = map[string]string{
	"claude-3-5-sonnet-20241022": "claude-sonnet-4-5",
	"claude-3-5-sonnet-20240620": "claude-sonnet-4-5",
	"claude-sonnet-4-5-20250929": "claude-sonnet-4-5-thinking",
	"claude-opus-4":              "claude-opus-4-5-thinking",
	"claude-opus-4-5-20251101":   "claude-opus-4-5-thinking",
	"claude-haiku-4":             "gemini-3-flash",
	"claude-haiku-4-5":           "gemini-3-flash",
	"claude-3-haiku-20240307":    "gemini-3-flash",
	"REDACTED":  "gemini-3-flash",
	// 生图模型：官方名 → Antigravity 内部名
	"gemini-3-pro-image-preview": "gemini-3-pro-image",
REDACTED

// AntigravityGatewayService 处理 Antigravity 平台的 API 转发
type AntigravityGatewayService struct {
	accountRepo      AccountRepository
	tokenProvider    *AntigravityTokenProvider
	rateLimitService *RateLimitService
	httpUpstream     HTTPUpstream
REDACTED

func NewAntigravityGatewayService(
	accountRepo AccountRepository,
	_ GatewayCache,
	tokenProvider *AntigravityTokenProvider,
	rateLimitService *RateLimitService,
	httpUpstream HTTPUpstream,
) *AntigravityGatewayService {
	return &AntigravityGatewayService{
		accountRepo:      accountRepo,
		tokenProvider:    tokenProvider,
		rateLimitService: rateLimitService,
		httpUpstream:     httpUpstream,
REDACTED
REDACTED

// GetTokenProvider 返回 token provider
func (s *AntigravityGatewayService) GetTokenProvider() *AntigravityTokenProvider {
	return s.tokenProvider
REDACTED

// getMappedModel 获取映射后的模型名
func (s *AntigravityGatewayService) getMappedModel(account *Account, requestedModel string) string {
	// 1. 优先使用账户级映射（复用现有方法）
	if mapped := account.GetMappedModel(requestedModel); mapped != requestedModel {
		return mapped
REDACTED

	// 2. 系统默认映射
	if mapped, ok := antigravityModelMapping[requestedModel]; ok {
		return mapped
REDACTED

	// 3. Gemini 模型透传
	if strings.HasPrefix(requestedModel, "gemini-") {
		return requestedModel
REDACTED

	// 4. Claude 前缀透传直接支持的模型
	if antigravitySupportedModels[requestedModel] {
		return requestedModel
REDACTED

	// 5. 默认值
	return "claude-sonnet-4-5"
REDACTED

// IsModelSupported 检查模型是否被支持
func (s *AntigravityGatewayService) IsModelSupported(requestedModel string) bool {
	// 直接支持的模型
	if antigravitySupportedModels[requestedModel] {
		return true
REDACTED
	// 可映射的模型
	if _, ok := antigravityModelMapping[requestedModel]; ok {
		return true
REDACTED
	// Gemini 前缀透传
	if strings.HasPrefix(requestedModel, "gemini-") {
		return true
REDACTED
	// Claude 模型支持（通过默认映射）
	if strings.HasPrefix(requestedModel, "claude-") {
		return true
REDACTED
	return false
REDACTED

// wrapV1InternalRequest 包装请求为 v1internal 格式
func (s *AntigravityGatewayService) wrapV1InternalRequest(projectID, model string, originalBody []byte) ([]byte, error) {
	var request any
	if err := json.Unmarshal(originalBody, &request); err != nil {
		return nil, fmt.Errorf("解析请求体失败: %w", err)
REDACTED

	wrapped := map[string]any{
		"project":     projectID,
		"requestId":   "agent-" + uuid.New().String(),
		"userAgent":   "sub2api",
		"requestType": "agent",
		"model":       model,
		"request":     request,
REDACTED

	return json.Marshal(wrapped)
REDACTED

// unwrapV1InternalResponse 解包 v1internal 响应
func (s *AntigravityGatewayService) unwrapV1InternalResponse(body []byte) ([]byte, error) {
	var outer map[string]any
	if err := json.Unmarshal(body, &outer); err != nil {
		return nil, err
REDACTED

	if resp, ok := outer["response"]; ok {
		return json.Marshal(resp)
REDACTED

	return body, nil
REDACTED

// Forward 转发 Claude 协议请求（Claude → Gemini 转换）
func (s *AntigravityGatewayService) Forward(ctx context.Context, c *gin.Context, account *Account, body []byte) (*ForwardResult, error) {
	startTime := time.Now()

	// 解析 Claude 请求
	var claudeReq antigravity.ClaudeRequest
	if err := json.Unmarshal(body, &claudeReq); err != nil {
		return nil, fmt.Errorf("parse claude request: %w", err)
REDACTED
	if strings.TrimSpace(claudeReq.Model) == "" {
		return nil, fmt.Errorf("missing model")
REDACTED

	originalModel := claudeReq.Model
	mappedModel := s.getMappedModel(account, claudeReq.Model)
	if mappedModel != claudeReq.Model {
		log.Printf("Antigravity model mapping: %s -> %s (account: %s)", claudeReq.Model, mappedModel, account.Name)
REDACTED

	// 获取 access_token
	if s.tokenProvider == nil {
		return nil, errors.New("antigravity token provider not configured")
REDACTED
	accessToken, err := s.tokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("获取 access_token 失败: %w", err)
REDACTED

	// 获取 project_id
	projectID := strings.TrimSpace(account.GetCredential("project_id"))
	if projectID == "" {
		return nil, errors.New("project_id not found in credentials")
REDACTED

	// 代理 URL
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED

	// 转换 Claude 请求为 Gemini 格式
	geminiBody, err := antigravity.TransformClaudeToGemini(&claudeReq, projectID, mappedModel)
	if err != nil {
		return nil, fmt.Errorf("transform request: %w", err)
REDACTED

	// 构建上游 URL
	action := "generateContent"
	if claudeReq.Stream {
		action = "streamGenerateContent"
REDACTED
	fullURL := fmt.Sprintf("%s/v1internal:%s", antigravity.BaseURL, action)
	if claudeReq.Stream {
		fullURL += "?alt=sse"
REDACTED

	// 重试循环
	var resp *http.Response
	for attempt := 1; attempt <= antigravityMaxRetries; attempt++ {
		upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(geminiBody))
		if err != nil {
			return nil, err
	REDACTED
		upstreamReq.Header.Set("Content-Type", "application/json")
		upstreamReq.Header.Set("Authorization", "Bearer "+accessToken)
		upstreamReq.Header.Set("User-Agent", antigravity.UserAgent)

		resp, err = s.httpUpstream.Do(upstreamReq, proxyURL)
		if err != nil {
			if attempt < antigravityMaxRetries {
				log.Printf("Antigravity account %d: upstream request failed, retry %d/%d: %v", account.ID, attempt, antigravityMaxRetries, err)
				sleepAntigravityBackoff(attempt)
				continue
		REDACTED
			return nil, s.writeClaudeError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed after retries")
	REDACTED

		if resp.StatusCode >= 400 && s.shouldRetryUpstreamError(resp.StatusCode) {
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
			_ = resp.Body.Close()

			if attempt < antigravityMaxRetries {
				log.Printf("Antigravity account %d: upstream status %d, retry %d/%d", account.ID, resp.StatusCode, attempt, antigravityMaxRetries)
				sleepAntigravityBackoff(attempt)
				continue
		REDACTED
			// 所有重试都失败，标记限流状态
			if resp.StatusCode == 429 {
				s.handleUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
		REDACTED
			// 最后一次尝试也失败
			resp = &http.Response{
				StatusCode: resp.StatusCode,
				Header:     resp.Header.Clone(),
				Body:       io.NopCloser(bytes.NewReader(respBody)),
		REDACTED
			break
	REDACTED

		break
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	// 处理错误响应
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		s.handleUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)

		if s.shouldFailoverUpstreamError(resp.StatusCode) {
			return nil, &UpstreamFailoverError{StatusCode: resp.StatusCodeREDACTED
	REDACTED

		return nil, s.writeMappedClaudeError(c, resp.StatusCode, respBody)
REDACTED

	requestID := resp.Header.Get("x-request-id")
	if requestID != "" {
		c.Header("x-request-id", requestID)
REDACTED

	var usage *ClaudeUsage
	var firstTokenMs *int
	if claudeReq.Stream {
		streamRes, err := s.handleClaudeStreamingResponse(c, resp, startTime, originalModel)
		if err != nil {
			return nil, err
	REDACTED
		usage = streamRes.usage
		firstTokenMs = streamRes.firstTokenMs
REDACTED else {
		usage, err = s.handleClaudeNonStreamingResponse(c, resp, originalModel)
		if err != nil {
			return nil, err
	REDACTED
REDACTED

	return &ForwardResult{
		RequestID:    requestID,
		Usage:        *usage,
		Model:        originalModel, // 使用原始模型用于计费和日志
		Stream:       claudeReq.Stream,
		Duration:     time.Since(startTime),
		FirstTokenMs: firstTokenMs,
REDACTED, nil
REDACTED

// ForwardGemini 转发 Gemini 协议请求
func (s *AntigravityGatewayService) ForwardGemini(ctx context.Context, c *gin.Context, account *Account, originalModel string, action string, stream bool, body []byte) (*ForwardResult, error) {
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

	mappedModel := s.getMappedModel(account, originalModel)

	// 获取 access_token
	if s.tokenProvider == nil {
		return nil, errors.New("antigravity token provider not configured")
REDACTED
	accessToken, err := s.tokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("获取 access_token 失败: %w", err)
REDACTED

	// 获取 project_id
	projectID := strings.TrimSpace(account.GetCredential("project_id"))
	if projectID == "" {
		return nil, errors.New("project_id not found in credentials")
REDACTED

	// 代理 URL
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED

	// 包装请求
	wrappedBody, err := s.wrapV1InternalRequest(projectID, mappedModel, body)
	if err != nil {
		return nil, err
REDACTED

	// 构建上游 URL
	upstreamAction := action
	if action == "generateContent" && stream {
		upstreamAction = "streamGenerateContent"
REDACTED
	fullURL := fmt.Sprintf("%s/v1internal:%s", antigravity.BaseURL, upstreamAction)
	if stream || upstreamAction == "streamGenerateContent" {
		fullURL += "?alt=sse"
REDACTED

	// 重试循环
	var resp *http.Response
	for attempt := 1; attempt <= antigravityMaxRetries; attempt++ {
		upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(wrappedBody))
		if err != nil {
			return nil, err
	REDACTED
		upstreamReq.Header.Set("Content-Type", "application/json")
		upstreamReq.Header.Set("Authorization", "Bearer "+accessToken)
		upstreamReq.Header.Set("User-Agent", antigravity.UserAgent)

		resp, err = s.httpUpstream.Do(upstreamReq, proxyURL)
		if err != nil {
			if attempt < antigravityMaxRetries {
				log.Printf("Antigravity account %d: upstream request failed, retry %d/%d: %v", account.ID, attempt, antigravityMaxRetries, err)
				sleepAntigravityBackoff(attempt)
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

		if resp.StatusCode >= 400 && s.shouldRetryUpstreamError(resp.StatusCode) {
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
			_ = resp.Body.Close()

			if attempt < antigravityMaxRetries {
				log.Printf("Antigravity account %d: upstream status %d, retry %d/%d", account.ID, resp.StatusCode, attempt, antigravityMaxRetries)
				sleepAntigravityBackoff(attempt)
				continue
		REDACTED
			// 所有重试都失败，标记限流状态
			if resp.StatusCode == 429 {
				s.handleUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
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
			resp = &http.Response{
				StatusCode: resp.StatusCode,
				Header:     resp.Header.Clone(),
				Body:       io.NopCloser(bytes.NewReader(respBody)),
		REDACTED
			break
	REDACTED

		break
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	requestID := resp.Header.Get("x-request-id")
	if requestID != "" {
		c.Header("x-request-id", requestID)
REDACTED

	// 处理错误响应
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		s.handleUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)

		if action == "countTokens" {
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

		if s.shouldFailoverUpstreamError(resp.StatusCode) {
			return nil, &UpstreamFailoverError{StatusCode: resp.StatusCodeREDACTED
	REDACTED

		// 解包并返回错误
		unwrapped, _ := s.unwrapV1InternalResponse(respBody)
		contentType := resp.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/json"
	REDACTED
		c.Data(resp.StatusCode, contentType, unwrapped)
		return nil, fmt.Errorf("antigravity upstream error: %d", resp.StatusCode)
REDACTED

	var usage *ClaudeUsage
	var firstTokenMs *int

	if stream || upstreamAction == "streamGenerateContent" {
		streamRes, err := s.handleGeminiStreamingResponse(c, resp, startTime)
		if err != nil {
			return nil, err
	REDACTED
		usage = streamRes.usage
		firstTokenMs = streamRes.firstTokenMs
REDACTED else {
		usageResp, err := s.handleGeminiNonStreamingResponse(c, resp)
		if err != nil {
			return nil, err
	REDACTED
		usage = usageResp
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

func (s *AntigravityGatewayService) shouldRetryUpstreamError(statusCode int) bool {
	switch statusCode {
	case 429, 500, 502, 503, 504, 529:
		return true
	default:
		return false
REDACTED
REDACTED

func (s *AntigravityGatewayService) shouldFailoverUpstreamError(statusCode int) bool {
	switch statusCode {
	case 401, 403, 429, 529:
		return true
	default:
		return statusCode >= 500
REDACTED
REDACTED

func sleepAntigravityBackoff(attempt int) {
	sleepGeminiBackoff(attempt) // 复用 Gemini 的退避逻辑
REDACTED

func (s *AntigravityGatewayService) handleUpstreamError(ctx context.Context, account *Account, statusCode int, headers http.Header, body []byte) {
	// 429 使用 Gemini 格式解析（从 body 解析重置时间）
	if statusCode == 429 {
		resetAt := ParseGeminiRateLimitResetTime(body)
		if resetAt == nil {
			// 解析失败：Gemini 有重试时间用 5 分钟，Claude 没有用 1 分钟
			defaultDur := 1 * time.Minute
			if bytes.Contains(body, []byte("Please retry in")) || bytes.Contains(body, []byte("retryDelay")) {
				defaultDur = 5 * time.Minute
		REDACTED
			ra := time.Now().Add(defaultDur)
			_ = s.accountRepo.SetRateLimited(ctx, account.ID, ra)
			return
	REDACTED
		_ = s.accountRepo.SetRateLimited(ctx, account.ID, time.Unix(*resetAt, 0))
		return
REDACTED
	// 其他错误码继续使用 rateLimitService
	if s.rateLimitService == nil {
		return
REDACTED
	s.rateLimitService.HandleUpstreamError(ctx, account, statusCode, headers, body)
REDACTED

type antigravityStreamResult struct {
	usage        *ClaudeUsage
	firstTokenMs *int
REDACTED

func (s *AntigravityGatewayService) handleGeminiStreamingResponse(c *gin.Context, resp *http.Response, startTime time.Time) (*antigravityStreamResult, error) {
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
				if payload == "" || payload == "[DONE]" {
					_, _ = io.WriteString(c.Writer, line)
					flusher.Flush()
			REDACTED else {
					// 解包 v1internal 响应
					inner, parseErr := s.unwrapV1InternalResponse([]byte(payload))
					if parseErr == nil && inner != nil {
						payload = string(inner)
				REDACTED

					// 解析 usage
					var parsed map[string]any
					if json.Unmarshal(inner, &parsed) == nil {
						if u := extractGeminiUsage(parsed); u != nil {
							usage = u
					REDACTED
				REDACTED

					if firstTokenMs == nil {
						ms := int(time.Since(startTime).Milliseconds())
						firstTokenMs = &ms
				REDACTED

					_, _ = fmt.Fprintf(c.Writer, "data: %s\n\n", payload)
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

	return &antigravityStreamResult{usage: usage, firstTokenMs: firstTokenMsREDACTED, nil
REDACTED

func (s *AntigravityGatewayService) handleGeminiNonStreamingResponse(c *gin.Context, resp *http.Response) (*ClaudeUsage, error) {
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
REDACTED

	// 解包 v1internal 响应
	unwrapped, _ := s.unwrapV1InternalResponse(respBody)

	var parsed map[string]any
	if json.Unmarshal(unwrapped, &parsed) == nil {
		if u := extractGeminiUsage(parsed); u != nil {
			c.Data(resp.StatusCode, "application/json", unwrapped)
			return u, nil
	REDACTED
REDACTED

	c.Data(resp.StatusCode, "application/json", unwrapped)
	return &ClaudeUsage{REDACTED, nil
REDACTED

func (s *AntigravityGatewayService) writeClaudeError(c *gin.Context, status int, errType, message string) error {
	c.JSON(status, gin.H{
		"type":  "error",
		"error": gin.H{"type": errType, "message": messageREDACTED,
REDACTED)
	return fmt.Errorf("%s", message)
REDACTED

func (s *AntigravityGatewayService) writeMappedClaudeError(c *gin.Context, upstreamStatus int, body []byte) error {
	// 记录上游错误详情便于调试
	log.Printf("Antigravity upstream error %d: %s", upstreamStatus, string(body))

	var statusCode int
	var errType, errMsg string

	switch upstreamStatus {
	case 400:
		statusCode = http.StatusBadRequest
		errType = "invalid_request_error"
		errMsg = "Invalid request"
	case 401:
		statusCode = http.StatusBadGateway
		errType = "authentication_error"
		errMsg = "Upstream authentication failed"
	case 403:
		statusCode = http.StatusBadGateway
		errType = "permission_error"
		errMsg = "Upstream access forbidden"
	case 429:
		statusCode = http.StatusTooManyRequests
		errType = "rate_limit_error"
		errMsg = "Upstream rate limit exceeded"
	case 529:
		statusCode = http.StatusServiceUnavailable
		errType = "overloaded_error"
		errMsg = "Upstream service overloaded"
	default:
		statusCode = http.StatusBadGateway
		errType = "upstream_error"
		errMsg = "Upstream request failed"
REDACTED

	c.JSON(statusCode, gin.H{
		"type":  "error",
		"error": gin.H{"type": errType, "message": errMsgREDACTED,
REDACTED)
	return fmt.Errorf("upstream error: %d", upstreamStatus)
REDACTED

func (s *AntigravityGatewayService) writeGoogleError(c *gin.Context, status int, message string) error {
	statusStr := "UNKNOWN"
	switch status {
	case 400:
		statusStr = "INVALID_ARGUMENT"
	case 404:
		statusStr = "NOT_FOUND"
	case 429:
		statusStr = "RESOURCE_EXHAUSTED"
	case 500:
		statusStr = "INTERNAL"
	case 502, 503:
		statusStr = "UNAVAILABLE"
REDACTED

	c.JSON(status, gin.H{
		"error": gin.H{
			"code":    status,
			"message": message,
			"status":  statusStr,
	REDACTED,
REDACTED)
	return fmt.Errorf("%s", message)
REDACTED

// handleClaudeNonStreamingResponse 处理 Claude 非流式响应（Gemini → Claude 转换）
func (s *AntigravityGatewayService) handleClaudeNonStreamingResponse(c *gin.Context, resp *http.Response, originalModel string) (*ClaudeUsage, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, s.writeClaudeError(c, http.StatusBadGateway, "upstream_error", "Failed to read upstream response")
REDACTED

	// 转换 Gemini 响应为 Claude 格式
	claudeResp, agUsage, err := antigravity.TransformGeminiToClaude(body, originalModel)
	if err != nil {
		log.Printf("Transform Gemini to Claude failed: %v, body: %s", err, string(body))
		return nil, s.writeClaudeError(c, http.StatusBadGateway, "upstream_error", "Failed to parse upstream response")
REDACTED

	c.Data(http.StatusOK, "application/json", claudeResp)

	// 转换为 service.ClaudeUsage
	usage := &ClaudeUsage{
		InputTokens:              agUsage.InputTokens,
		OutputTokens:             agUsage.OutputTokens,
		CacheCreationInputTokens: agUsage.CacheCreationInputTokens,
		CacheReadInputTokens:     agUsage.CacheReadInputTokens,
REDACTED
	return usage, nil
REDACTED

// handleClaudeStreamingResponse 处理 Claude 流式响应（Gemini SSE → Claude SSE 转换）
func (s *AntigravityGatewayService) handleClaudeStreamingResponse(c *gin.Context, resp *http.Response, startTime time.Time, originalModel string) (*antigravityStreamResult, error) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
REDACTED

	processor := antigravity.NewStreamingProcessor(originalModel)
	var firstTokenMs *int
	reader := bufio.NewReader(resp.Body)

	// 辅助函数：转换 antigravity.ClaudeUsage 到 service.ClaudeUsage
	convertUsage := func(agUsage *antigravity.ClaudeUsage) *ClaudeUsage {
		if agUsage == nil {
			return &ClaudeUsage{REDACTED
	REDACTED
		return &ClaudeUsage{
			InputTokens:              agUsage.InputTokens,
			OutputTokens:             agUsage.OutputTokens,
			CacheCreationInputTokens: agUsage.CacheCreationInputTokens,
			CacheReadInputTokens:     agUsage.CacheReadInputTokens,
	REDACTED
REDACTED

	for {
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("stream read error: %w", err)
	REDACTED

		if len(line) > 0 {
			// 处理 SSE 行，转换为 Claude 格式
			claudeEvents := processor.ProcessLine(strings.TrimRight(line, "\r\n"))

			if len(claudeEvents) > 0 {
				if firstTokenMs == nil {
					ms := int(time.Since(startTime).Milliseconds())
					firstTokenMs = &ms
			REDACTED

				if _, writeErr := c.Writer.Write(claudeEvents); writeErr != nil {
					finalEvents, agUsage := processor.Finish()
					if len(finalEvents) > 0 {
						_, _ = c.Writer.Write(finalEvents)
				REDACTED
					return &antigravityStreamResult{usage: convertUsage(agUsage), firstTokenMs: firstTokenMsREDACTED, writeErr
			REDACTED
				flusher.Flush()
		REDACTED
	REDACTED

		if errors.Is(err, io.EOF) {
			break
	REDACTED
REDACTED

	// 发送结束事件
	finalEvents, agUsage := processor.Finish()
	if len(finalEvents) > 0 {
		_, _ = c.Writer.Write(finalEvents)
		flusher.Flush()
REDACTED

	return &antigravityStreamResult{usage: convertUsage(agUsage), firstTokenMs: firstTokenMsREDACTED, nil
REDACTED
