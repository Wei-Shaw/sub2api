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
	mathrand "math/rand"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	antigravityStickySessionTTL = time.Hour
	antigravityMaxRetries       = 3
	antigravityRetryBaseDelay   = 1 * time.Second
	antigravityRetryMaxDelay    = 16 * time.Second
)

// getSessionID 从 gin.Context 获取 session_id（用于日志追踪）
func getSessionID(c *gin.Context) string {
	if c == nil {
		return ""
REDACTED
	return c.GetHeader("session_id")
REDACTED

// logPrefix 生成统一的日志前缀
func logPrefix(sessionID, accountName string) string {
	if sessionID != "" {
		return fmt.Sprintf("[antigravity-Forward] session=%s account=%s", sessionID, accountName)
REDACTED
	return fmt.Sprintf("[antigravity-Forward] account=%s", accountName)
REDACTED

// Antigravity 直接支持的模型（精确匹配透传）
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
	"gemini-3-pro-image":         true,
REDACTED

// Antigravity 前缀映射表（按前缀长度降序排列，确保最长匹配优先）
// 用于处理模型版本号变化（如 -20251111, -thinking, -preview 等后缀）
var antigravityPrefixMapping = []struct {
	prefix string
	target string
REDACTED{
	// 长前缀优先
	{"gemini-2.5-flash-image", "gemini-3-pro-image"REDACTED, // gemini-2.5-flash-image → 3-pro-image
	{"gemini-3-pro-image", "gemini-3-pro-image"REDACTED,     // gemini-3-pro-image-preview 等
	{"claude-3-5-sonnet", "claude-sonnet-4-5"REDACTED,       // 旧版 claude-3-5-sonnet-xxx
	{"claude-sonnet-4-5", "claude-sonnet-4-5"REDACTED,       // claude-sonnet-4-5-xxx
	{"claude-haiku-4-5", "claude-sonnet-4-5"REDACTED,        // claude-haiku-4-5-xxx → sonnet
	{"claude-opus-4-5", "claude-opus-4-5-thinking"REDACTED,
	{"claude-3-haiku", "claude-sonnet-4-5"REDACTED, // 旧版 claude-3-haiku-xxx → sonnet
	{"claude-sonnet-4", "claude-sonnet-4-5"REDACTED,
	{"claude-haiku-4", "claude-sonnet-4-5"REDACTED, // → sonnet
	{"claude-opus-4", "claude-opus-4-5-thinking"REDACTED,
	{"gemini-3-pro", "gemini-3-pro-high"REDACTED, // gemini-3-pro, gemini-3-pro-preview 等
REDACTED

// AntigravityGatewayService 处理 Antigravity 平台的 API 转发
type AntigravityGatewayService struct {
	accountRepo      AccountRepository
	tokenProvider    *AntigravityTokenProvider
	rateLimitService *RateLimitService
	httpUpstream     HTTPUpstream
	settingService   *SettingService
REDACTED

func NewAntigravityGatewayService(
	accountRepo AccountRepository,
	_ GatewayCache,
	tokenProvider *AntigravityTokenProvider,
	rateLimitService *RateLimitService,
	httpUpstream HTTPUpstream,
	settingService *SettingService,
) *AntigravityGatewayService {
	return &AntigravityGatewayService{
		accountRepo:      accountRepo,
		tokenProvider:    tokenProvider,
		rateLimitService: rateLimitService,
		httpUpstream:     httpUpstream,
		settingService:   settingService,
REDACTED
REDACTED

// GetTokenProvider 返回 token provider
func (s *AntigravityGatewayService) GetTokenProvider() *AntigravityTokenProvider {
	return s.tokenProvider
REDACTED

// getMappedModel 获取映射后的模型名
// 逻辑：账户映射 → 直接支持透传 → 前缀映射 → gemini透传 → 默认值
func (s *AntigravityGatewayService) getMappedModel(account *Account, requestedModel string) string {
	// 1. 账户级映射（用户自定义优先）
	if mapped := account.GetMappedModel(requestedModel); mapped != requestedModel {
		return mapped
REDACTED

	// 2. 直接支持的模型透传
	if antigravitySupportedModels[requestedModel] {
		return requestedModel
REDACTED

	// 3. 前缀映射（处理版本号变化，如 -20251111, -thinking, -preview）
	for _, pm := range antigravityPrefixMapping {
		if strings.HasPrefix(requestedModel, pm.prefix) {
			return pm.target
	REDACTED
REDACTED

	// 4. Gemini 模型透传（未匹配到前缀的 gemini 模型）
	if strings.HasPrefix(requestedModel, "gemini-") {
		return requestedModel
REDACTED

	// 5. 默认值
	return "claude-sonnet-4-5"
REDACTED

// IsModelSupported 检查模型是否被支持
// 所有 claude- 和 gemini- 前缀的模型都能通过映射或透传支持
func (s *AntigravityGatewayService) IsModelSupported(requestedModel string) bool {
	return strings.HasPrefix(requestedModel, "claude-") ||
		strings.HasPrefix(requestedModel, "gemini-")
REDACTED

// TestConnectionResult 测试连接结果
type TestConnectionResult struct {
	Text        string // 响应文本
	MappedModel string // 实际使用的模型
REDACTED

// TestConnection 测试 Antigravity 账号连接（非流式，无重试、无计费）
// 支持 Claude 和 Gemini 两种协议，根据 modelID 前缀自动选择
func (s *AntigravityGatewayService) TestConnection(ctx context.Context, account *Account, modelID string) (*TestConnectionResult, error) {
	// 获取 token
	if s.tokenProvider == nil {
		return nil, errors.New("antigravity token provider not configured")
REDACTED
	accessToken, err := s.tokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("获取 access_token 失败: %w", err)
REDACTED

	// 获取 project_id（部分账户类型可能没有）
	projectID := strings.TrimSpace(account.GetCredential("project_id"))

	// 模型映射
	mappedModel := s.getMappedModel(account, modelID)

	// 构建请求体
	var requestBody []byte
	if strings.HasPrefix(modelID, "gemini-") {
		// Gemini 模型：直接使用 Gemini 格式
		requestBody, err = s.buildGeminiTestRequest(projectID, mappedModel)
REDACTED else {
		// Claude 模型：使用协议转换
		requestBody, err = s.buildClaudeTestRequest(projectID, mappedModel)
REDACTED
	if err != nil {
		return nil, fmt.Errorf("构建请求失败: %w", err)
REDACTED

	// 构建 HTTP 请求（非流式）
	req, err := antigravity.NewAPIRequest(ctx, "generateContent", accessToken, requestBody)
	if err != nil {
		return nil, err
REDACTED

	// 代理 URL
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED

	// 发送请求
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	// 读取响应
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
REDACTED

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API 返回 %d: %s", resp.StatusCode, string(respBody))
REDACTED

	// 解包 v1internal 响应
	unwrapped, err := s.unwrapV1InternalResponse(respBody)
	if err != nil {
		return nil, fmt.Errorf("解包响应失败: %w", err)
REDACTED

	// 提取响应文本
	text := extractGeminiResponseText(unwrapped)

	return &TestConnectionResult{
		Text:        text,
		MappedModel: mappedModel,
REDACTED, nil
REDACTED

// buildGeminiTestRequest 构建 Gemini 格式测试请求
func (s *AntigravityGatewayService) buildGeminiTestRequest(projectID, model string) ([]byte, error) {
	payload := map[string]any{
		"contents": []map[string]any{
			{
				"role": "user",
				"parts": []map[string]any{
					{"text": "hi"REDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED
	payloadBytes, _ := json.Marshal(payload)
	return s.wrapV1InternalRequest(projectID, model, payloadBytes)
REDACTED

// buildClaudeTestRequest 构建 Claude 格式测试请求并转换为 Gemini 格式
func (s *AntigravityGatewayService) buildClaudeTestRequest(projectID, mappedModel string) ([]byte, error) {
	claudeReq := &antigravity.ClaudeRequest{
		Model: mappedModel,
		Messages: []antigravity.ClaudeMessage{
			{
				Role:    "user",
				Content: json.RawMessage(`"hi"`),
		REDACTED,
	REDACTED,
		MaxTokens: 1024,
		Stream:    false,
REDACTED
	return antigravity.TransformClaudeToGemini(claudeReq, projectID, mappedModel)
REDACTED

func (s *AntigravityGatewayService) getClaudeTransformOptions(ctx context.Context) antigravity.TransformOptions {
	opts := antigravity.DefaultTransformOptions()
	if s.settingService == nil {
		return opts
REDACTED
	opts.EnableIdentityPatch = s.settingService.IsIdentityPatchEnabled(ctx)
	opts.IdentityPatch = s.settingService.GetIdentityPatchPrompt(ctx)
	return opts
REDACTED

// extractGeminiResponseText 从 Gemini 响应中提取文本
func extractGeminiResponseText(respBody []byte) string {
	var resp map[string]any
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return ""
REDACTED

	candidates, ok := resp["candidates"].([]any)
	if !ok || len(candidates) == 0 {
		return ""
REDACTED

	candidate, ok := candidates[0].(map[string]any)
	if !ok {
		return ""
REDACTED

	content, ok := candidate["content"].(map[string]any)
	if !ok {
		return ""
REDACTED

	parts, ok := content["parts"].([]any)
	if !ok {
		return ""
REDACTED

	var texts []string
	for _, part := range parts {
		if partMap, ok := part.(map[string]any); ok {
			if text, ok := partMap["text"].(string); ok && text != "" {
				texts = append(texts, text)
		REDACTED
	REDACTED
REDACTED

	return strings.Join(texts, "")
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

// isModelNotFoundError 检测是否为模型不存在的 404 错误
func isModelNotFoundError(statusCode int, body []byte) bool {
	if statusCode != 404 {
		return false
REDACTED

	bodyStr := strings.ToLower(string(body))
	keywords := []string{"model not found", "unknown model", "not found"REDACTED
	for _, keyword := range keywords {
		if strings.Contains(bodyStr, keyword) {
			return true
	REDACTED
REDACTED
	return true // 404 without specific message also treated as model not found
REDACTED

// Forward 转发 Claude 协议请求（Claude → Gemini 转换）
func (s *AntigravityGatewayService) Forward(ctx context.Context, c *gin.Context, account *Account, body []byte) (*ForwardResult, error) {
	startTime := time.Now()
	sessionID := getSessionID(c)
	prefix := logPrefix(sessionID, account.Name)

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

	// 获取 access_token
	if s.tokenProvider == nil {
		return nil, errors.New("antigravity token provider not configured")
REDACTED
	accessToken, err := s.tokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("获取 access_token 失败: %w", err)
REDACTED

	// 获取 project_id（部分账户类型可能没有）
	projectID := strings.TrimSpace(account.GetCredential("project_id"))

	// 代理 URL
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED

	// 转换 Claude 请求为 Gemini 格式
	geminiBody, err := antigravity.TransformClaudeToGeminiWithOptions(&claudeReq, projectID, mappedModel, s.getClaudeTransformOptions(ctx))
	if err != nil {
		return nil, fmt.Errorf("transform request: %w", err)
REDACTED

	// 构建上游 action
	action := "generateContent"
	if claudeReq.Stream {
		action = "streamGenerateContent?alt=sse"
REDACTED

	// 重试循环
	var resp *http.Response
	for attempt := 1; attempt <= antigravityMaxRetries; attempt++ {
		// 检查 context 是否已取消（客户端断开连接）
		select {
		case <-ctx.Done():
			log.Printf("%s status=context_canceled error=%v", prefix, ctx.Err())
			return nil, ctx.Err()
		default:
	REDACTED

		upstreamReq, err := antigravity.NewAPIRequest(ctx, action, accessToken, geminiBody)
		if err != nil {
			return nil, err
	REDACTED

		resp, err = s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
		if err != nil {
			if attempt < antigravityMaxRetries {
				log.Printf("%s status=request_failed retry=%d/%d error=%v", prefix, attempt, antigravityMaxRetries, err)
				if !sleepAntigravityBackoffWithContext(ctx, attempt) {
					log.Printf("%s status=context_canceled_during_backoff", prefix)
					return nil, ctx.Err()
			REDACTED
				continue
		REDACTED
			log.Printf("%s status=request_failed retries_exhausted error=%v", prefix, err)
			return nil, s.writeClaudeError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed after retries")
	REDACTED

		if resp.StatusCode >= 400 && s.shouldRetryUpstreamError(resp.StatusCode) {
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
			_ = resp.Body.Close()

			if attempt < antigravityMaxRetries {
				log.Printf("%s status=%d retry=%d/%d", prefix, resp.StatusCode, attempt, antigravityMaxRetries)
				if !sleepAntigravityBackoffWithContext(ctx, attempt) {
					log.Printf("%s status=context_canceled_during_backoff", prefix)
					return nil, ctx.Err()
			REDACTED
				continue
		REDACTED
			// 所有重试都失败，标记限流状态
			if resp.StatusCode == 429 {
				s.handleUpstreamError(ctx, prefix, account, resp.StatusCode, resp.Header, respBody)
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

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))

		// 优先检测 thinking block 的 signature 相关错误（400）并重试一次：
		// Antigravity /v1internal 链路在部分场景会对 thought/thinking signature 做严格校验，
		// 当历史消息携带的 signature 不合法时会直接 400；去除 thinking 后可继续完成请求。
		if resp.StatusCode == http.StatusBadRequest && isSignatureRelatedError(respBody) {
			// Conservative two-stage fallback:
			// 1) Disable top-level thinking + thinking->text
			// 2) Only if still signature-related 400: also downgrade tool_use/tool_result to text.

			retryStages := []struct {
				name  string
				strip func(*antigravity.ClaudeRequest) (bool, error)
		REDACTED{
				{name: "thinking-only", strip: stripThinkingFromClaudeRequestREDACTED,
				{name: "thinking+tools", strip: stripSignatureSensitiveBlocksFromClaudeRequestREDACTED,
		REDACTED

			for _, stage := range retryStages {
				retryClaudeReq := claudeReq
				retryClaudeReq.Messages = append([]antigravity.ClaudeMessage(nil), claudeReq.Messages...)

				stripped, stripErr := stage.strip(&retryClaudeReq)
				if stripErr != nil || !stripped {
					continue
			REDACTED

				log.Printf("Antigravity account %d: detected signature-related 400, retrying once (%s)", account.ID, stage.name)

				retryGeminiBody, txErr := antigravity.TransformClaudeToGeminiWithOptions(&retryClaudeReq, projectID, mappedModel, s.getClaudeTransformOptions(ctx))
				if txErr != nil {
					continue
			REDACTED
				retryReq, buildErr := antigravity.NewAPIRequest(ctx, action, accessToken, retryGeminiBody)
				if buildErr != nil {
					continue
			REDACTED
				retryResp, retryErr := s.httpUpstream.Do(retryReq, proxyURL, account.ID, account.Concurrency)
				if retryErr != nil {
					log.Printf("Antigravity account %d: signature retry request failed (%s): %v", account.ID, stage.name, retryErr)
					continue
			REDACTED

				if retryResp.StatusCode < 400 {
					_ = resp.Body.Close()
					resp = retryResp
					respBody = nil
					break
			REDACTED

				retryBody, _ := io.ReadAll(io.LimitReader(retryResp.Body, 2<<20))
				_ = retryResp.Body.Close()

				// If this stage fixed the signature issue, we stop; otherwise we may try the next stage.
				if retryResp.StatusCode != http.StatusBadRequest || !isSignatureRelatedError(retryBody) {
					respBody = retryBody
					resp = &http.Response{
						StatusCode: retryResp.StatusCode,
						Header:     retryResp.Header.Clone(),
						Body:       io.NopCloser(bytes.NewReader(retryBody)),
				REDACTED
					break
			REDACTED

				// Still signature-related; capture context and allow next stage.
				respBody = retryBody
				resp = &http.Response{
					StatusCode: retryResp.StatusCode,
					Header:     retryResp.Header.Clone(),
					Body:       io.NopCloser(bytes.NewReader(retryBody)),
			REDACTED
		REDACTED
	REDACTED

		// 处理错误响应（重试后仍失败或不触发重试）
		if resp.StatusCode >= 400 {
			s.handleUpstreamError(ctx, prefix, account, resp.StatusCode, resp.Header, respBody)

			if s.shouldFailoverUpstreamError(resp.StatusCode) {
				return nil, &UpstreamFailoverError{StatusCode: resp.StatusCodeREDACTED
		REDACTED

			return nil, s.writeMappedClaudeError(c, resp.StatusCode, respBody)
	REDACTED
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
			log.Printf("%s status=stream_error error=%v", prefix, err)
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

func isSignatureRelatedError(respBody []byte) bool {
	msg := strings.ToLower(strings.TrimSpace(extractAntigravityErrorMessage(respBody)))
	if msg == "" {
		// Fallback: best-effort scan of the raw payload.
		msg = strings.ToLower(string(respBody))
REDACTED

	// Keep this intentionally broad: different upstreams may use "signature" or "thought_signature".
	if strings.Contains(msg, "thought_signature") || strings.Contains(msg, "signature") {
		return true
REDACTED

	// Also detect thinking block structural errors:
	// "Expected `thinking` or `redacted_thinking`, but found `text`"
	if strings.Contains(msg, "expected") && (strings.Contains(msg, "thinking") || strings.Contains(msg, "redacted_thinking")) {
		return true
REDACTED

	return false
REDACTED

func extractAntigravityErrorMessage(body []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
REDACTED

	// Google-style: {"error": {"message": "..."REDACTEDREDACTED
	if errObj, ok := payload["error"].(map[string]any); ok {
		if msg, ok := errObj["message"].(string); ok && strings.TrimSpace(msg) != "" {
			return msg
	REDACTED
REDACTED

	// Fallback: top-level message
	if msg, ok := payload["message"].(string); ok && strings.TrimSpace(msg) != "" {
		return msg
REDACTED

	return ""
REDACTED

// stripThinkingFromClaudeRequest converts thinking blocks to text blocks in a Claude Messages request.
// This preserves the thinking content while avoiding signature validation errors.
// Note: redacted_thinking blocks are removed because they cannot be converted to text.
// It also disables top-level `thinking` to avoid upstream structural constraints for thinking mode.
func stripThinkingFromClaudeRequest(req *antigravity.ClaudeRequest) (bool, error) {
	if req == nil {
		return false, nil
REDACTED

	changed := false
	if req.Thinking != nil {
		req.Thinking = nil
		changed = true
REDACTED

	for i := range req.Messages {
		raw := req.Messages[i].Content
		if len(raw) == 0 {
			continue
	REDACTED

		// If content is a string, nothing to strip.
		var str string
		if json.Unmarshal(raw, &str) == nil {
			continue
	REDACTED

		// Otherwise treat as an array of blocks and convert thinking blocks to text.
		var blocks []map[string]any
		if err := json.Unmarshal(raw, &blocks); err != nil {
			continue
	REDACTED

		filtered := make([]map[string]any, 0, len(blocks))
		modifiedAny := false
		for _, block := range blocks {
			t, _ := block["type"].(string)
			switch t {
			case "thinking":
				thinkingText, _ := block["thinking"].(string)
				if thinkingText != "" {
					filtered = append(filtered, map[string]any{
						"type": "text",
						"text": thinkingText,
				REDACTED)
			REDACTED
				modifiedAny = true
			case "redacted_thinking":
				modifiedAny = true
			case "":
				if thinkingText, hasThinking := block["thinking"].(string); hasThinking {
					if thinkingText != "" {
						filtered = append(filtered, map[string]any{
							"type": "text",
							"text": thinkingText,
					REDACTED)
				REDACTED
					modifiedAny = true
			REDACTED else {
					filtered = append(filtered, block)
			REDACTED
			default:
				filtered = append(filtered, block)
		REDACTED
	REDACTED

		if !modifiedAny {
			continue
	REDACTED

		if len(filtered) == 0 {
			filtered = append(filtered, map[string]any{
				"type": "text",
				"text": "(content removed)",
		REDACTED)
	REDACTED

		newRaw, err := json.Marshal(filtered)
		if err != nil {
			return changed, err
	REDACTED
		req.Messages[i].Content = newRaw
		changed = true
REDACTED

	return changed, nil
REDACTED

// stripSignatureSensitiveBlocksFromClaudeRequest is a stronger retry degradation that additionally converts
// tool blocks to plain text. Use this only after a thinking-only retry still fails with signature errors.
func stripSignatureSensitiveBlocksFromClaudeRequest(req *antigravity.ClaudeRequest) (bool, error) {
	if req == nil {
		return false, nil
REDACTED

	changed := false
	if req.Thinking != nil {
		req.Thinking = nil
		changed = true
REDACTED

	for i := range req.Messages {
		raw := req.Messages[i].Content
		if len(raw) == 0 {
			continue
	REDACTED

		// If content is a string, nothing to strip.
		var str string
		if json.Unmarshal(raw, &str) == nil {
			continue
	REDACTED

		// Otherwise treat as an array of blocks and convert signature-sensitive blocks to text.
		var blocks []map[string]any
		if err := json.Unmarshal(raw, &blocks); err != nil {
			continue
	REDACTED

		filtered := make([]map[string]any, 0, len(blocks))
		modifiedAny := false
		for _, block := range blocks {
			t, _ := block["type"].(string)
			switch t {
			case "thinking":
				// Convert thinking to text, skip if empty
				thinkingText, _ := block["thinking"].(string)
				if thinkingText != "" {
					filtered = append(filtered, map[string]any{
						"type": "text",
						"text": thinkingText,
				REDACTED)
			REDACTED
				modifiedAny = true
			case "redacted_thinking":
				// Remove redacted_thinking (cannot convert encrypted content)
				modifiedAny = true
			case "tool_use":
				// Convert tool_use to text to avoid upstream signature/thought_signature validation errors.
				// This is a retry-only degradation path, so we prioritise request validity over tool semantics.
				name, _ := block["name"].(string)
				id, _ := block["id"].(string)
				input := block["input"]
				inputJSON, _ := json.Marshal(input)
				text := "(tool_use)"
				if name != "" {
					text += " name=" + name
			REDACTED
				if id != "" {
					text += " id=" + id
			REDACTED
				if len(inputJSON) > 0 && string(inputJSON) != "null" {
					text += " input=" + string(inputJSON)
			REDACTED
				filtered = append(filtered, map[string]any{
					"type": "text",
					"text": text,
			REDACTED)
				modifiedAny = true
			case "tool_result":
				// Convert tool_result to text so it stays consistent when tool_use is downgraded.
				toolUseID, _ := block["tool_use_id"].(string)
				isError, _ := block["is_error"].(bool)
				content := block["content"]
				contentJSON, _ := json.Marshal(content)
				text := "(tool_result)"
				if toolUseID != "" {
					text += " tool_use_id=" + toolUseID
			REDACTED
				if isError {
					text += " is_error=true"
			REDACTED
				if len(contentJSON) > 0 && string(contentJSON) != "null" {
					text += "\n" + string(contentJSON)
			REDACTED
				filtered = append(filtered, map[string]any{
					"type": "text",
					"text": text,
			REDACTED)
				modifiedAny = true
			case "":
				// Handle untyped block with "thinking" field
				if thinkingText, hasThinking := block["thinking"].(string); hasThinking {
					if thinkingText != "" {
						filtered = append(filtered, map[string]any{
							"type": "text",
							"text": thinkingText,
					REDACTED)
				REDACTED
					modifiedAny = true
			REDACTED else {
					filtered = append(filtered, block)
			REDACTED
			default:
				filtered = append(filtered, block)
		REDACTED
	REDACTED

		if !modifiedAny {
			continue
	REDACTED

		if len(filtered) == 0 {
			// Keep request valid: upstream rejects empty content arrays.
			filtered = append(filtered, map[string]any{
				"type": "text",
				"text": "(content removed)",
		REDACTED)
	REDACTED

		newRaw, err := json.Marshal(filtered)
		if err != nil {
			return changed, err
	REDACTED
		req.Messages[i].Content = newRaw
		changed = true
REDACTED

	return changed, nil
REDACTED

// ForwardGemini 转发 Gemini 协议请求
func (s *AntigravityGatewayService) ForwardGemini(ctx context.Context, c *gin.Context, account *Account, originalModel string, action string, stream bool, body []byte) (*ForwardResult, error) {
	startTime := time.Now()
	sessionID := getSessionID(c)
	prefix := logPrefix(sessionID, account.Name)

	if strings.TrimSpace(originalModel) == "" {
		return nil, s.writeGoogleError(c, http.StatusBadRequest, "Missing model in URL")
REDACTED
	if strings.TrimSpace(action) == "" {
		return nil, s.writeGoogleError(c, http.StatusBadRequest, "Missing action in URL")
REDACTED
	if len(body) == 0 {
		return nil, s.writeGoogleError(c, http.StatusBadRequest, "Request body is empty")
REDACTED

	// 解析请求以获取 image_size（用于图片计费）
	imageSize := s.extractImageSize(body)

	switch action {
	case "generateContent", "streamGenerateContent":
		// ok
	case "countTokens":
		// 直接返回空值，不透传上游
		c.JSON(http.StatusOK, map[string]any{"totalTokens": 0REDACTED)
		return &ForwardResult{
			RequestID:    "",
			Usage:        ClaudeUsage{REDACTED,
			Model:        originalModel,
			Stream:       false,
			Duration:     time.Since(time.Now()),
			FirstTokenMs: nil,
	REDACTED, nil
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

	// 获取 project_id（部分账户类型可能没有）
	projectID := strings.TrimSpace(account.GetCredential("project_id"))

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

	// 构建上游 action
	upstreamAction := action
	if action == "generateContent" && stream {
		upstreamAction = "streamGenerateContent"
REDACTED
	if stream || upstreamAction == "streamGenerateContent" {
		upstreamAction += "?alt=sse"
REDACTED

	// 重试循环
	var resp *http.Response
	for attempt := 1; attempt <= antigravityMaxRetries; attempt++ {
		// 检查 context 是否已取消（客户端断开连接）
		select {
		case <-ctx.Done():
			log.Printf("%s status=context_canceled error=%v", prefix, ctx.Err())
			return nil, ctx.Err()
		default:
	REDACTED

		upstreamReq, err := antigravity.NewAPIRequest(ctx, upstreamAction, accessToken, wrappedBody)
		if err != nil {
			return nil, err
	REDACTED

		resp, err = s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
		if err != nil {
			if attempt < antigravityMaxRetries {
				log.Printf("%s status=request_failed retry=%d/%d error=%v", prefix, attempt, antigravityMaxRetries, err)
				if !sleepAntigravityBackoffWithContext(ctx, attempt) {
					log.Printf("%s status=context_canceled_during_backoff", prefix)
					return nil, ctx.Err()
			REDACTED
				continue
		REDACTED
			log.Printf("%s status=request_failed retries_exhausted error=%v", prefix, err)
			return nil, s.writeGoogleError(c, http.StatusBadGateway, "Upstream request failed after retries")
	REDACTED

		if resp.StatusCode >= 400 && s.shouldRetryUpstreamError(resp.StatusCode) {
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
			_ = resp.Body.Close()

			if attempt < antigravityMaxRetries {
				log.Printf("%s status=%d retry=%d/%d", prefix, resp.StatusCode, attempt, antigravityMaxRetries)
				if !sleepAntigravityBackoffWithContext(ctx, attempt) {
					log.Printf("%s status=context_canceled_during_backoff", prefix)
					return nil, ctx.Err()
			REDACTED
				continue
		REDACTED
			// 所有重试都失败，标记限流状态
			if resp.StatusCode == 429 {
				s.handleUpstreamError(ctx, prefix, account, resp.StatusCode, resp.Header, respBody)
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
	defer func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
	REDACTED
REDACTED()

	// 处理错误响应
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		// 尽早关闭原始响应体，释放连接；后续逻辑仍可能需要读取 body，因此用内存副本重新包装。
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))

		// 模型兜底：模型不存在且开启 fallback 时，自动用 fallback 模型重试一次
		if s.settingService != nil && s.settingService.IsModelFallbackEnabled(ctx) &&
			isModelNotFoundError(resp.StatusCode, respBody) {
			fallbackModel := s.settingService.GetFallbackModel(ctx, PlatformAntigravity)
			if fallbackModel != "" && fallbackModel != mappedModel {
				log.Printf("[Antigravity] Model not found (%s), retrying with fallback model %s (account: %s)", mappedModel, fallbackModel, account.Name)

				fallbackWrapped, err := s.wrapV1InternalRequest(projectID, fallbackModel, body)
				if err == nil {
					fallbackReq, err := antigravity.NewAPIRequest(ctx, upstreamAction, accessToken, fallbackWrapped)
					if err == nil {
						fallbackResp, err := s.httpUpstream.Do(fallbackReq, proxyURL, account.ID, account.Concurrency)
						if err == nil && fallbackResp.StatusCode < 400 {
							_ = resp.Body.Close()
							resp = fallbackResp
					REDACTED else if fallbackResp != nil {
							_ = fallbackResp.Body.Close()
					REDACTED
				REDACTED
			REDACTED
		REDACTED
	REDACTED

		// fallback 成功：继续按正常响应处理
		if resp.StatusCode < 400 {
			goto handleSuccess
	REDACTED

		s.handleUpstreamError(ctx, prefix, account, resp.StatusCode, resp.Header, respBody)

		if s.shouldFailoverUpstreamError(resp.StatusCode) {
			return nil, &UpstreamFailoverError{StatusCode: resp.StatusCodeREDACTED
	REDACTED

		// 解包并返回错误
		requestID := resp.Header.Get("x-request-id")
		if requestID != "" {
			c.Header("x-request-id", requestID)
	REDACTED
		unwrapped, _ := s.unwrapV1InternalResponse(respBody)
		contentType := resp.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/json"
	REDACTED
		c.Data(resp.StatusCode, contentType, unwrapped)
		return nil, fmt.Errorf("antigravity upstream error: %d", resp.StatusCode)
REDACTED

handleSuccess:
	requestID := resp.Header.Get("x-request-id")
	if requestID != "" {
		c.Header("x-request-id", requestID)
REDACTED

	var usage *ClaudeUsage
	var firstTokenMs *int

	if stream || upstreamAction == "streamGenerateContent" {
		streamRes, err := s.handleGeminiStreamingResponse(c, resp, startTime)
		if err != nil {
			log.Printf("%s status=stream_error error=%v", prefix, err)
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

	// 判断是否为图片生成模型
	imageCount := 0
	if isImageGenerationModel(mappedModel) {
		// Gemini 图片生成 API 每次请求只生成一张图片（API 限制）
		imageCount = 1
REDACTED

	return &ForwardResult{
		RequestID:    requestID,
		Usage:        *usage,
		Model:        originalModel,
		Stream:       stream,
		Duration:     time.Since(startTime),
		FirstTokenMs: firstTokenMs,
		ImageCount:   imageCount,
		ImageSize:    imageSize,
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

// sleepAntigravityBackoffWithContext 带 context 取消检查的退避等待
// 返回 true 表示正常完成等待，false 表示 context 已取消
func sleepAntigravityBackoffWithContext(ctx context.Context, attempt int) bool {
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

	select {
	case <-ctx.Done():
		return false
	case <-time.After(sleepFor):
		return true
REDACTED
REDACTED

func (s *AntigravityGatewayService) handleUpstreamError(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte) {
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
			log.Printf("%s status=429 rate_limited reset_in=%v (fallback)", prefix, defaultDur)
			_ = s.accountRepo.SetRateLimited(ctx, account.ID, ra)
			return
	REDACTED
		resetTime := time.Unix(*resetAt, 0)
		log.Printf("%s status=429 rate_limited reset_at=%v reset_in=%v", prefix, resetTime.Format("15:04:05"), time.Until(resetTime).Truncate(time.Second))
		_ = s.accountRepo.SetRateLimited(ctx, account.ID, resetTime)
		return
REDACTED
	// 其他错误码继续使用 rateLimitService
	if s.rateLimitService == nil {
		return
REDACTED
	shouldDisable := s.rateLimitService.HandleUpstreamError(ctx, account, statusCode, headers, body)
	if shouldDisable {
		log.Printf("%s status=%d marked_error", prefix, statusCode)
REDACTED
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

	// 使用 Scanner 并限制单行大小，避免 ReadString 无上限导致 OOM
	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.settingService.cfg != nil && s.settingService.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.settingService.cfg.Gateway.MaxLineSize
REDACTED
	scanner.Buffer(make([]byte, 64*1024), maxLineSize)
	usage := &ClaudeUsage{REDACTED
	var firstTokenMs *int

	type scanEvent struct {
		line string
		err  error
REDACTED
	// 独立 goroutine 读取上游，避免读取阻塞影响超时处理
	events := make(chan scanEvent, 16)
	done := make(chan struct{REDACTED)
	sendEvent := func(ev scanEvent) bool {
		select {
		case events <- ev:
			return true
		case <-done:
			return false
	REDACTED
REDACTED
	var lastReadAt int64
	atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
	go func() {
		defer close(events)
		for scanner.Scan() {
			atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
			if !sendEvent(scanEvent{line: scanner.Text()REDACTED) {
				return
		REDACTED
	REDACTED
		if err := scanner.Err(); err != nil {
			_ = sendEvent(scanEvent{err: errREDACTED)
	REDACTED
REDACTED()
	defer close(done)

	// 上游数据间隔超时保护（防止上游挂起长期占用连接）
	streamInterval := time.Duration(0)
	if s.settingService.cfg != nil && s.settingService.cfg.Gateway.StreamDataIntervalTimeout > 0 {
		streamInterval = time.Duration(s.settingService.cfg.Gateway.StreamDataIntervalTimeout) * time.Second
REDACTED
	var intervalTicker *time.Ticker
	if streamInterval > 0 {
		intervalTicker = time.NewTicker(streamInterval)
		defer intervalTicker.Stop()
REDACTED
	var intervalCh <-chan time.Time
	if intervalTicker != nil {
		intervalCh = intervalTicker.C
REDACTED

	// 仅发送一次错误事件，避免多次写入导致协议混乱
	errorEventSent := false
	sendErrorEvent := func(reason string) {
		if errorEventSent {
			return
	REDACTED
		errorEventSent = true
		_, _ = fmt.Fprintf(c.Writer, "event: error\ndata: {\"error\":\"%s\"REDACTED\n\n", reason)
		flusher.Flush()
REDACTED

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return &antigravityStreamResult{usage: usage, firstTokenMs: firstTokenMsREDACTED, nil
		REDACTED
			if ev.err != nil {
				if errors.Is(ev.err, bufio.ErrTooLong) {
					log.Printf("SSE line too long (antigravity): max_size=%d error=%v", maxLineSize, ev.err)
					sendErrorEvent("response_too_large")
					return &antigravityStreamResult{usage: usage, firstTokenMs: firstTokenMsREDACTED, ev.err
			REDACTED
				sendErrorEvent("stream_read_error")
				return nil, ev.err
		REDACTED

			line := ev.line
			trimmed := strings.TrimRight(line, "\r\n")
			if strings.HasPrefix(trimmed, "data:") {
				payload := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
				if payload == "" || payload == "[DONE]" {
					if _, err := fmt.Fprintf(c.Writer, "%s\n", line); err != nil {
						sendErrorEvent("write_failed")
						return &antigravityStreamResult{usage: usage, firstTokenMs: firstTokenMsREDACTED, err
				REDACTED
					flusher.Flush()
					continue
			REDACTED

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

				if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", payload); err != nil {
					sendErrorEvent("write_failed")
					return &antigravityStreamResult{usage: usage, firstTokenMs: firstTokenMsREDACTED, err
			REDACTED
				flusher.Flush()
				continue
		REDACTED

			if _, err := fmt.Fprintf(c.Writer, "%s\n", line); err != nil {
				sendErrorEvent("write_failed")
				return &antigravityStreamResult{usage: usage, firstTokenMs: firstTokenMsREDACTED, err
		REDACTED
			flusher.Flush()

		case <-intervalCh:
			lastRead := time.Unix(0, atomic.LoadInt64(&lastReadAt))
			if time.Since(lastRead) < streamInterval {
				continue
		REDACTED
			log.Printf("Stream data interval timeout (antigravity)")
			sendErrorEvent("stream_timeout")
			return &antigravityStreamResult{usage: usage, firstTokenMs: firstTokenMsREDACTED, fmt.Errorf("stream data interval timeout")
	REDACTED
REDACTED
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
	log.Printf("[antigravity-Forward] upstream_error status=%d body=%s", upstreamStatus, string(body))

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
		log.Printf("[antigravity-Forward] transform_error error=%v body=%s", err, string(body))
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
	// 使用 Scanner 并限制单行大小，避免 ReadString 无上限导致 OOM
	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.settingService.cfg != nil && s.settingService.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.settingService.cfg.Gateway.MaxLineSize
REDACTED
	scanner.Buffer(make([]byte, 64*1024), maxLineSize)

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

	type scanEvent struct {
		line string
		err  error
REDACTED
	// 独立 goroutine 读取上游，避免读取阻塞影响超时处理
	events := make(chan scanEvent, 16)
	done := make(chan struct{REDACTED)
	sendEvent := func(ev scanEvent) bool {
		select {
		case events <- ev:
			return true
		case <-done:
			return false
	REDACTED
REDACTED
	var lastReadAt int64
	atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
	go func() {
		defer close(events)
		for scanner.Scan() {
			atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
			if !sendEvent(scanEvent{line: scanner.Text()REDACTED) {
				return
		REDACTED
	REDACTED
		if err := scanner.Err(); err != nil {
			_ = sendEvent(scanEvent{err: errREDACTED)
	REDACTED
REDACTED()
	defer close(done)

	streamInterval := time.Duration(0)
	if s.settingService.cfg != nil && s.settingService.cfg.Gateway.StreamDataIntervalTimeout > 0 {
		streamInterval = time.Duration(s.settingService.cfg.Gateway.StreamDataIntervalTimeout) * time.Second
REDACTED
	var intervalTicker *time.Ticker
	if streamInterval > 0 {
		intervalTicker = time.NewTicker(streamInterval)
		defer intervalTicker.Stop()
REDACTED
	var intervalCh <-chan time.Time
	if intervalTicker != nil {
		intervalCh = intervalTicker.C
REDACTED

	// 仅发送一次错误事件，避免多次写入导致协议混乱
	errorEventSent := false
	sendErrorEvent := func(reason string) {
		if errorEventSent {
			return
	REDACTED
		errorEventSent = true
		_, _ = fmt.Fprintf(c.Writer, "event: error\ndata: {\"error\":\"%s\"REDACTED\n\n", reason)
		flusher.Flush()
REDACTED

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				// 发送结束事件
				finalEvents, agUsage := processor.Finish()
				if len(finalEvents) > 0 {
					_, _ = c.Writer.Write(finalEvents)
					flusher.Flush()
			REDACTED
				return &antigravityStreamResult{usage: convertUsage(agUsage), firstTokenMs: firstTokenMsREDACTED, nil
		REDACTED
			if ev.err != nil {
				if errors.Is(ev.err, bufio.ErrTooLong) {
					log.Printf("SSE line too long (antigravity): max_size=%d error=%v", maxLineSize, ev.err)
					sendErrorEvent("response_too_large")
					return &antigravityStreamResult{usage: convertUsage(nil), firstTokenMs: firstTokenMsREDACTED, ev.err
			REDACTED
				sendErrorEvent("stream_read_error")
				return nil, fmt.Errorf("stream read error: %w", ev.err)
		REDACTED

			line := ev.line
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
					sendErrorEvent("write_failed")
					return &antigravityStreamResult{usage: convertUsage(agUsage), firstTokenMs: firstTokenMsREDACTED, writeErr
			REDACTED
				flusher.Flush()
		REDACTED

		case <-intervalCh:
			lastRead := time.Unix(0, atomic.LoadInt64(&lastReadAt))
			if time.Since(lastRead) < streamInterval {
				continue
		REDACTED
			log.Printf("Stream data interval timeout (antigravity)")
			sendErrorEvent("stream_timeout")
			return &antigravityStreamResult{usage: convertUsage(nil), firstTokenMs: firstTokenMsREDACTED, fmt.Errorf("stream data interval timeout")
	REDACTED
REDACTED

REDACTED

// extractImageSize 从 Gemini 请求中提取 image_size 参数
func (s *AntigravityGatewayService) extractImageSize(body []byte) string {
	var req antigravity.GeminiRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return "2K" // 默认 2K
REDACTED

	if req.GenerationConfig != nil && req.GenerationConfig.ImageConfig != nil {
		size := strings.ToUpper(strings.TrimSpace(req.GenerationConfig.ImageConfig.ImageSize))
		if size == "1K" || size == "2K" || size == "4K" {
			return size
	REDACTED
REDACTED

	return "2K" // 默认 2K
REDACTED

// isImageGenerationModel 判断模型是否为图片生成模型
// 支持的模型：gemini-3-pro-image, gemini-3-pro-image-preview, gemini-2.5-flash-image 等
func isImageGenerationModel(model string) bool {
	modelLower := strings.ToLower(model)
	// 移除 models/ 前缀
	modelLower = strings.TrimPrefix(modelLower, "models/")

	// 精确匹配或前缀匹配
	return modelLower == "gemini-3-pro-image" ||
		modelLower == "gemini-3-pro-image-preview" ||
		strings.HasPrefix(modelLower, "gemini-3-pro-image-") ||
		modelLower == "gemini-2.5-flash-image" ||
		modelLower == "gemini-2.5-flash-image-preview" ||
		strings.HasPrefix(modelLower, "gemini-2.5-flash-image-")
REDACTED
