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
	"claude-haiku-4":             "claude-sonnet-4-5",
	"claude-3-haiku-20240307":    "claude-sonnet-4-5",
	"REDACTED":  "claude-sonnet-4-5",
REDACTED

// AntigravityGatewayService 处理 Antigravity 平台的 API 转发
type AntigravityGatewayService struct {
	accountRepo      AccountRepository
	cache            GatewayCache
	tokenProvider    *AntigravityTokenProvider
	rateLimitService *RateLimitService
	httpUpstream     HTTPUpstream
REDACTED

func NewAntigravityGatewayService(
	accountRepo AccountRepository,
	cache GatewayCache,
	tokenProvider *AntigravityTokenProvider,
	rateLimitService *RateLimitService,
	httpUpstream HTTPUpstream,
) *AntigravityGatewayService {
	return &AntigravityGatewayService{
		accountRepo:      accountRepo,
		cache:            cache,
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

// unwrapSSELine 解包 SSE 行中的 v1internal 响应
func (s *AntigravityGatewayService) unwrapSSELine(line string) string {
	if !strings.HasPrefix(line, "data: ") {
		return line
REDACTED

	data := strings.TrimPrefix(line, "data: ")
	if data == "" || data == "[DONE]" {
		return line
REDACTED

	var outer map[string]any
	if err := json.Unmarshal([]byte(data), &outer); err != nil {
		return line
REDACTED

	if resp, ok := outer["response"]; ok {
		unwrapped, err := json.Marshal(resp)
		if err != nil {
			return line
	REDACTED
		return "data: " + string(unwrapped)
REDACTED

	return line
REDACTED

// Forward 转发 Claude 协议请求
func (s *AntigravityGatewayService) Forward(ctx context.Context, c *gin.Context, account *Account, body []byte) (*ForwardResult, error) {
	startTime := time.Now()

	// 解析请求获取 model 和 stream
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
	mappedModel := s.getMappedModel(account, req.Model)
	if mappedModel != req.Model {
		log.Printf("Antigravity model mapping: %s -> %s (account: %s)", req.Model, mappedModel, account.Name)
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

	// 包装请求
	wrappedBody, err := s.wrapV1InternalRequest(projectID, mappedModel, body)
	if err != nil {
		return nil, err
REDACTED

	// 构建上游 URL
	action := "generateContent"
	if req.Stream {
		action = "streamGenerateContent"
REDACTED
	fullURL := fmt.Sprintf("%s/v1internal:%s", antigravity.BaseURL, action)
	if req.Stream {
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
			return nil, s.writeClaudeError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed after retries")
	REDACTED

		if resp.StatusCode >= 400 && s.shouldRetryUpstreamError(resp.StatusCode) {
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
			_ = resp.Body.Close()

			if resp.StatusCode == 429 {
				s.handleUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
		REDACTED
			if attempt < antigravityMaxRetries {
				log.Printf("Antigravity account %d: upstream status %d, retry %d/%d", account.ID, resp.StatusCode, attempt, antigravityMaxRetries)
				sleepAntigravityBackoff(attempt)
				continue
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
	if req.Stream {
		streamRes, err := s.handleStreamingResponse(c, resp, startTime, originalModel)
		if err != nil {
			return nil, err
	REDACTED
		usage = streamRes.usage
		firstTokenMs = streamRes.firstTokenMs
REDACTED else {
		usage, err = s.handleNonStreamingResponse(c, resp, originalModel)
		if err != nil {
			return nil, err
	REDACTED
REDACTED

	return &ForwardResult{
		RequestID:    requestID,
		Usage:        *usage,
		Model:        originalModel, // 使用原始模型用于计费和日志
		Stream:       req.Stream,
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

			if resp.StatusCode == 429 {
				s.handleUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
		REDACTED
			if attempt < antigravityMaxRetries {
				log.Printf("Antigravity account %d: upstream status %d, retry %d/%d", account.ID, resp.StatusCode, attempt, antigravityMaxRetries)
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
	if s.rateLimitService == nil {
		return
REDACTED
	s.rateLimitService.HandleUpstreamError(ctx, account, statusCode, headers, body)
REDACTED

type antigravityStreamResult struct {
	usage        *ClaudeUsage
	firstTokenMs *int
REDACTED

func (s *AntigravityGatewayService) handleStreamingResponse(c *gin.Context, resp *http.Response, startTime time.Time, originalModel string) (*antigravityStreamResult, error) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
REDACTED

	usage := &ClaudeUsage{REDACTED
	var firstTokenMs *int
	reader := bufio.NewReader(resp.Body)

	for {
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("stream read error: %w", err)
	REDACTED

		if len(line) > 0 {
			// 解包 v1internal 响应
			unwrapped := s.unwrapSSELine(strings.TrimRight(line, "\r\n"))

			// 解析 usage
			if strings.HasPrefix(unwrapped, "data: ") {
				data := strings.TrimPrefix(unwrapped, "data: ")
				if data != "" && data != "[DONE]" {
					if firstTokenMs == nil {
						ms := int(time.Since(startTime).Milliseconds())
						firstTokenMs = &ms
				REDACTED
					s.parseClaudeSSEUsage(data, usage)
			REDACTED
		REDACTED

			// 写入响应
			if _, writeErr := fmt.Fprintf(c.Writer, "%s\n", unwrapped); writeErr != nil {
				return &antigravityStreamResult{usage: usage, firstTokenMs: firstTokenMsREDACTED, writeErr
		REDACTED
			flusher.Flush()
	REDACTED

		if errors.Is(err, io.EOF) {
			break
	REDACTED
REDACTED

	return &antigravityStreamResult{usage: usage, firstTokenMs: firstTokenMsREDACTED, nil
REDACTED

func (s *AntigravityGatewayService) handleNonStreamingResponse(c *gin.Context, resp *http.Response, originalModel string) (*ClaudeUsage, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, s.writeClaudeError(c, http.StatusBadGateway, "upstream_error", "Failed to read upstream response")
REDACTED

	// 解包 v1internal 响应
	unwrapped, err := s.unwrapV1InternalResponse(body)
	if err != nil {
		return nil, s.writeClaudeError(c, http.StatusBadGateway, "upstream_error", "Failed to parse upstream response")
REDACTED

	// 解析 usage
	var respObj struct {
		Usage ClaudeUsage `json:"usage"`
REDACTED
	_ = json.Unmarshal(unwrapped, &respObj)

	c.Data(http.StatusOK, "application/json", unwrapped)
	return &respObj.Usage, nil
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

func (s *AntigravityGatewayService) parseClaudeSSEUsage(data string, usage *ClaudeUsage) {
	// 解析 message_start 获取 input tokens
	var msgStart struct {
		Type    string `json:"type"`
		Message struct {
			Usage ClaudeUsage `json:"usage"`
	REDACTED `json:"message"`
REDACTED
	if json.Unmarshal([]byte(data), &msgStart) == nil && msgStart.Type == "message_start" {
		usage.InputTokens = msgStart.Message.Usage.InputTokens
		usage.CacheCreationInputTokens = msgStart.Message.Usage.CacheCreationInputTokens
		usage.CacheReadInputTokens = msgStart.Message.Usage.CacheReadInputTokens
REDACTED

	// 解析 message_delta 获取 output tokens
	var msgDelta struct {
		Type  string `json:"type"`
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	REDACTED `json:"usage"`
REDACTED
	if json.Unmarshal([]byte(data), &msgDelta) == nil && msgDelta.Type == "message_delta" {
		usage.OutputTokens = msgDelta.Usage.OutputTokens
		if usage.InputTokens == 0 {
			usage.InputTokens = msgDelta.Usage.InputTokens
	REDACTED
		if usage.CacheCreationInputTokens == 0 {
			usage.CacheCreationInputTokens = msgDelta.Usage.CacheCreationInputTokens
	REDACTED
		if usage.CacheReadInputTokens == 0 {
			usage.CacheReadInputTokens = msgDelta.Usage.CacheReadInputTokens
	REDACTED
REDACTED
REDACTED

func (s *AntigravityGatewayService) writeClaudeError(c *gin.Context, status int, errType, message string) error {
	c.JSON(status, gin.H{
		"type":  "error",
		"error": gin.H{"type": errType, "message": messageREDACTED,
REDACTED)
	return fmt.Errorf("%s", message)
REDACTED

func (s *AntigravityGatewayService) writeMappedClaudeError(c *gin.Context, upstreamStatus int, body []byte) error {
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
