package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/gin-gonic/gin"
)

// isClaudeCodeClient 判断请求是否来自真正的 Claude Code 客户端。
// 判定条件：
//  1. User-Agent 匹配 claude-cli/X.Y.Z（大小写不敏感）
//  2. metadata.user_id 符合 Claude Code 格式（legacy 或 JSON 格式）
//
// 只检查 metadata.user_id 非空不够严格：第三方工具（opencode 等）可能伪造 UA
// 并附带任意 metadata.user_id 字符串，从而绕过 mimicry。必须通过 ParseMetadataUserID
// 验证格式才能确认是真正的 Claude Code 客户端。
func isClaudeCodeClient(userAgent string, metadataUserID string) bool {
	if !claudeCliUserAgentRe.MatchString(userAgent) {
		return false
	}
	return ParseMetadataUserID(metadataUserID) != nil
}

func shouldUseClaudeCodeNoopDeltaKeepalive(userAgent string) bool {
	version := ExtractCLIVersion(userAgent)
	if version == "" {
		return false
	}
	return CompareVersions(version, claudeCodeNoopDeltaKeepaliveMinVersion) >= 0
}

func claudeCodeKeepaliveDeltaTypeForContentBlock(blockType string) string {
	switch blockType {
	case "text":
		return "text_delta"
	case "tool_use":
		return "input_json_delta"
	case "thinking":
		return "thinking_delta"
	default:
		return ""
	}
}

func claudeCodeKeepaliveFieldForDeltaType(deltaType string) string {
	switch deltaType {
	case "text_delta":
		return "text"
	case "input_json_delta":
		return "partial_json"
	case "thinking_delta":
		return "thinking"
	default:
		return ""
	}
}

func buildClaudeCodeNoopDeltaKeepalive(index int, deltaType string) (string, bool) {
	fieldName := claudeCodeKeepaliveFieldForDeltaType(deltaType)
	if fieldName == "" {
		return "", false
	}
	return fmt.Sprintf("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":%d,\"delta\":{\"type\":\"%s\",\"%s\":\"\"}}\n\n", index, deltaType, fieldName), true
}

func sseEventIndex(event map[string]any) (int, bool) {
	switch v := event["index"].(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	case int64:
		return int(v), true
	case json.Number:
		i, err := v.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	default:
		return 0, false
	}
}

// shouldRectifySignatureError 统一判断是否应触发签名整流（strip thinking blocks 并重试）。
// 根据账号类型检查对应的开关和匹配模式。
//
// mappedModel 用于按 thinking 协议族分流：passback-required (DeepSeek/Kimi/GLM 等) 上游
// 的 400 不是签名缺失问题，retry 任何 thinking 变形都会破坏「原样回传」契约——直接透传
// 错误给客户端。详见 thinking_protocol.go。
func (s *GatewayService) shouldRectifySignatureError(ctx context.Context, account *Account, respBody []byte, mappedModel string) bool {
	if !ShouldRectifyThinkingSignatureError(mappedModel) {
		return false
	}
	if account.Type == AccountTypeAPIKey {
		// API Key 账号：独立开关，一次读取配置
		settings, err := s.settingService.GetRectifierSettings(ctx)
		if err != nil || !settings.Enabled || !settings.APIKeySignatureEnabled {
			return false
		}
		// 先检查内置模式（同 OAuth），再检查自定义关键词
		if s.isThinkingBlockSignatureError(respBody) {
			return true
		}
		return matchSignaturePatterns(respBody, settings.APIKeySignaturePatterns)
	}
	// OAuth/SetupToken/Upstream/Bedrock 等：保持原有行为（内置模式 + 原开关）
	return s.isThinkingBlockSignatureError(respBody) && s.settingService.IsSignatureRectifierEnabled(ctx)
}

// isSignatureErrorPattern 仅做模式匹配，不检查开关。
// 用于已进入重试流程后的二阶段检测（此时开关已在首次调用时验证过）。
func (s *GatewayService) isSignatureErrorPattern(ctx context.Context, account *Account, respBody []byte) bool {
	if s.isThinkingBlockSignatureError(respBody) {
		return true
	}
	if account.Type == AccountTypeAPIKey {
		settings, err := s.settingService.GetRectifierSettings(ctx)
		if err != nil {
			return false
		}
		return matchSignaturePatterns(respBody, settings.APIKeySignaturePatterns)
	}
	return false
}

// matchSignaturePatterns 检查响应体是否匹配自定义关键词列表（不区分大小写）。
func matchSignaturePatterns(respBody []byte, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}
	bodyLower := strings.ToLower(string(respBody))
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.Contains(bodyLower, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

// isThinkingBlockSignatureError 检测是否是thinking block相关错误
// 这类错误可以通过过滤thinking blocks并重试来解决
func (s *GatewayService) isThinkingBlockSignatureError(respBody []byte) bool {
	msg := strings.ToLower(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
	if msg == "" {
		return false
	}

	// 检测signature相关的错误（更宽松的匹配）
	// 例如: "Invalid `signature` in `thinking` block", "***.signature" 等
	if strings.Contains(msg, "signature") {
		return true
	}

	// 检测 thinking block 顺序/类型错误
	// 例如: "Expected `thinking` or `redacted_thinking`, but found `text`"
	if strings.Contains(msg, "expected") && (strings.Contains(msg, "thinking") || strings.Contains(msg, "redacted_thinking")) {
		logger.LegacyPrintf("service.gateway", "[SignatureCheck] Detected thinking block type error")
		return true
	}

	// 检测 thinking block 被修改的错误
	// 例如: "thinking or redacted_thinking blocks in the latest assistant message cannot be modified"
	if strings.Contains(msg, "cannot be modified") && (strings.Contains(msg, "thinking") || strings.Contains(msg, "redacted_thinking")) {
		logger.LegacyPrintf("service.gateway", "[SignatureCheck] Detected thinking block modification error")
		return true
	}

	// 检测空消息内容错误（可能是过滤 thinking blocks 后导致的，或客户端发送了空 text block）
	// 例如: "all messages must have non-empty content"
	//       "messages: text content blocks must be non-empty"
	if strings.Contains(msg, "non-empty content") || strings.Contains(msg, "empty content") ||
		strings.Contains(msg, "content blocks must be non-empty") {
		logger.LegacyPrintf("service.gateway", "[SignatureCheck] Detected empty content error")
		return true
	}

	// 检测 thinking block 缺少 thinking 字段的错误（跨模型切换时常见：
	// 其他模型回过的 assistant 历史里有 type=thinking 但没有 thinking 文本，
	// 喂给开启 extended thinking 的 claude 时会被拒）
	// 例如: "messages.1.content.0.thinking: each thinking block must contain thinking"
	if strings.Contains(msg, "thinking block must contain") {
		logger.LegacyPrintf("service.gateway", "[SignatureCheck] Detected thinking block missing content error")
		return true
	}

	return false
}

func (s *GatewayService) shouldFailoverOn400(respBody []byte) bool {
	// 只对"可能是兼容性差异导致"的 400 允许切换，避免无意义重试。
	// 默认保守：无法识别则不切换。
	msg := strings.ToLower(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
	if msg == "" {
		return false
	}

	// 缺少/错误的 beta header：换账号/链路可能成功（尤其是混合调度时）。
	// 更精确匹配 beta 相关的兼容性问题，避免误触发切换。
	if strings.Contains(msg, "anthropic-beta") ||
		strings.Contains(msg, "beta feature") ||
		strings.Contains(msg, "requires beta") {
		return true
	}

	// thinking/tool streaming 等兼容性约束（常见于中间转换链路）
	if strings.Contains(msg, "thinking") || strings.Contains(msg, "thought_signature") || strings.Contains(msg, "signature") {
		return true
	}
	if strings.Contains(msg, "tool_use") || strings.Contains(msg, "tool_result") || strings.Contains(msg, "tools") {
		return true
	}

	return false
}

// sanitizeStreamError 返回不含网络地址的客户端可见错误描述。
// 默认 (*net.OpError).Error() 会拼接 Source/Addr 字段，泄露内部 IP/端口与上游
// 服务器地址（例如 "read tcp 10.0.0.1:54321->52.1.2.3:443: read: connection
// reset by peer"）。该函数只保留可识别的错误类别，原始 err 仍在调用点写入日志。
func sanitizeStreamError(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, io.ErrUnexpectedEOF):
		return "unexpected EOF"
	case errors.Is(err, io.EOF):
		return "EOF"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline exceeded"
	case errors.Is(err, syscall.ECONNRESET):
		return "connection reset by peer"
	case errors.Is(err, syscall.ECONNABORTED):
		return "connection aborted"
	case errors.Is(err, syscall.ETIMEDOUT):
		return "connection timed out"
	case errors.Is(err, syscall.EPIPE):
		return "broken pipe"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "connection refused"
	}
	var netErr *net.OpError
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			if netErr.Op != "" {
				return netErr.Op + " timeout"
			}
			return "i/o timeout"
		}
		if netErr.Op != "" {
			return netErr.Op + " network error"
		}
	}
	return "upstream connection error"
}

// ExtractUpstreamErrorMessage 从上游响应体中提取错误消息
// 支持 Claude 风格的错误格式：{"type":"error","error":{"type":"...","message":"..."}}
func ExtractUpstreamErrorMessage(body []byte) string {
	return extractUpstreamErrorMessage(body)
}

func extractUpstreamErrorMessage(body []byte) string {
	// Claude 风格：{"type":"error","error":{"type":"...","message":"..."}}
	if m := gjson.GetBytes(body, "error.message").String(); strings.TrimSpace(m) != "" {
		inner := strings.TrimSpace(m)
		// 有些上游会把完整 JSON 作为字符串塞进 message
		if strings.HasPrefix(inner, "{") {
			if innerMsg := gjson.Get(inner, "error.message").String(); strings.TrimSpace(innerMsg) != "" {
				return innerMsg
			}
		}
		return m
	}

	// ChatGPT 内部 API 风格：{"detail":"..."}
	if d := gjson.GetBytes(body, "detail").String(); strings.TrimSpace(d) != "" {
		return d
	}

	// 兜底：尝试顶层 message
	return gjson.GetBytes(body, "message").String()
}

func extractUpstreamErrorCode(body []byte) string {
	if code := strings.TrimSpace(gjson.GetBytes(body, "error.code").String()); code != "" {
		return code
	}

	inner := strings.TrimSpace(gjson.GetBytes(body, "error.message").String())
	if !strings.HasPrefix(inner, "{") {
		return ""
	}

	if code := strings.TrimSpace(gjson.Get(inner, "error.code").String()); code != "" {
		return code
	}

	if lastBrace := strings.LastIndex(inner, "}"); lastBrace >= 0 {
		if code := strings.TrimSpace(gjson.Get(inner[:lastBrace+1], "error.code").String()); code != "" {
			return code
		}
	}

	return ""
}

func isCountTokensUnsupported404(statusCode int, body []byte) bool {
	if statusCode != http.StatusNotFound {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(extractUpstreamErrorMessage(body)))
	if msg == "" {
		return false
	}
	if strings.Contains(msg, "/v1/messages/count_tokens") {
		return true
	}
	return strings.Contains(msg, "count_tokens") && strings.Contains(msg, "not found")
}

func (s *GatewayService) readUpstreamErrorBody(resp *http.Response) ([]byte, error) {
	if resp == nil || resp.Body == nil {
		return nil, nil
	}
	limit := gatewayUpstreamErrorBodyReadLimit
	if s != nil && s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody && s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes > int(limit) {
		limit = int64(s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}

func (s *GatewayService) handleErrorResponse(ctx context.Context, resp *http.Response, c *gin.Context, account *Account, requestedModel ...string) (*ForwardResult, error) {
	body, _ := s.readUpstreamErrorBody(resp)

	// 调试日志：打印上游错误响应
	logger.LegacyPrintf("service.gateway", "[Forward] Upstream error (non-retryable): Account=%d(%s) Status=%d RequestID=%s Body=%s",
		account.ID, account.Name, resp.StatusCode, resp.Header.Get("x-request-id"), truncateString(string(body), 1000))

	upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(body))
	upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)

	// Print a compact upstream request fingerprint when we hit the Claude Code OAuth
	// credential scope error. This avoids requiring env-var tweaks in a fixed deploy.
	if isClaudeCodeCredentialScopeError(upstreamMsg) && c != nil {
		if v, ok := c.Get(claudeMimicDebugInfoKey); ok {
			if line, ok := v.(string); ok && strings.TrimSpace(line) != "" {
				logger.LegacyPrintf("service.gateway", "[ClaudeMimicDebugOnError] status=%d request_id=%s %s",
					resp.StatusCode,
					resp.Header.Get("x-request-id"),
					line,
				)
			}
		}
	}

	// Enrich Ops error logs with upstream status + message, and optionally a truncated body snippet.
	upstreamDetail := ""
	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
		}
		upstreamDetail = truncateString(string(body), maxBytes)
	}
	setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, upstreamDetail)
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:           account.Platform,
		AccountID:          account.ID,
		UpstreamStatusCode: resp.StatusCode,
		UpstreamRequestID:  resp.Header.Get("x-request-id"),
		Kind:               "http_error",
		Message:            upstreamMsg,
		Detail:             upstreamDetail,
	})

	// 处理上游错误，标记账号状态
	shouldDisable := false
	if s.rateLimitService != nil {
		if len(requestedModel) > 0 {
			shouldDisable = s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, body, requestedModel[0])
		} else {
			shouldDisable = s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, body)
		}
	}
	if shouldDisable {
		return nil, &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: body}
	}

	MarkResponseCommitted(c)

	// 记录上游错误响应体摘要便于排障（可选：由配置控制；不回显到客户端）
	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		logger.LegacyPrintf("service.gateway",
			"Upstream error %d (account=%d platform=%s type=%s): %s",
			resp.StatusCode,
			account.ID,
			account.Platform,
			account.Type,
			truncateForLog(body, s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes),
		)
	}

	// 非 failover 错误也支持错误透传规则匹配。
	if status, errType, errMsg, matched := applyErrorPassthroughRule(
		c,
		account.Platform,
		resp.StatusCode,
		body,
		http.StatusBadGateway,
		"upstream_error",
		"Upstream request failed",
	); matched {
		c.JSON(status, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    errType,
				"message": errMsg,
			},
		})

		summary := upstreamMsg
		if summary == "" {
			summary = errMsg
		}
		if summary == "" {
			return nil, fmt.Errorf("upstream error: %d (passthrough rule matched)", resp.StatusCode)
		}
		return nil, fmt.Errorf("upstream error: %d (passthrough rule matched) message=%s", resp.StatusCode, summary)
	}

	// 根据状态码返回适当的自定义错误响应（不透传上游详细信息）
	var errType, errMsg string
	var statusCode int

	switch resp.StatusCode {
	case 400:
		c.Data(http.StatusBadRequest, "application/json", body)
		summary := upstreamMsg
		if summary == "" {
			summary = truncateForLog(body, 512)
		}
		if summary == "" {
			return nil, fmt.Errorf("upstream error: %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("upstream error: %d message=%s", resp.StatusCode, summary)
	case 401:
		statusCode = http.StatusBadGateway
		errType = "upstream_error"
		errMsg = "Upstream authentication failed, please contact administrator"
	case 403:
		statusCode = http.StatusBadGateway
		errType = "upstream_error"
		errMsg = "Upstream access forbidden, please contact administrator"
	case 429:
		statusCode = http.StatusTooManyRequests
		errType = "rate_limit_error"
		errMsg = "Upstream rate limit exceeded, please retry later"
	case 529:
		statusCode = http.StatusServiceUnavailable
		errType = "overloaded_error"
		errMsg = "Upstream service overloaded, please retry later"
	case 500, 502, 503, 504:
		statusCode = http.StatusBadGateway
		errType = "upstream_error"
		errMsg = "Upstream service temporarily unavailable"
	default:
		statusCode = http.StatusBadGateway
		errType = "upstream_error"
		errMsg = "Upstream request failed"
	}

	// 返回自定义错误响应
	c.JSON(statusCode, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    errType,
			"message": errMsg,
		},
	})

	if upstreamMsg == "" {
		return nil, fmt.Errorf("upstream error: %d", resp.StatusCode)
	}
	return nil, fmt.Errorf("upstream error: %d message=%s", resp.StatusCode, upstreamMsg)
}

func (s *GatewayService) handleRetryExhaustedSideEffects(ctx context.Context, resp *http.Response, account *Account) {
	body, _ := s.readUpstreamErrorBody(resp)
	statusCode := resp.StatusCode

	// OAuth/Setup Token 账号的 403：标记账号异常
	if account.IsOAuth() && statusCode == 403 {
		s.rateLimitService.HandleUpstreamError(ctx, account, statusCode, resp.Header, body)
		logger.LegacyPrintf("service.gateway", "Account %d: marked as error after %d retries for status %d", account.ID, maxRetryAttempts, statusCode)
	} else {
		// API Key 未配置错误码：不标记账号状态
		logger.LegacyPrintf("service.gateway", "Account %d: upstream error %d after %d retries (not marking account)", account.ID, statusCode, maxRetryAttempts)
	}
}

func (s *GatewayService) handleFailoverSideEffects(ctx context.Context, resp *http.Response, account *Account, requestedModel ...string) {
	body, _ := s.readUpstreamErrorBody(resp)
	if len(requestedModel) > 0 {
		s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, body, requestedModel[0])
		return
	}
	s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, body)
}

// handleRetryExhaustedError 处理重试耗尽后的错误
// OAuth 403：标记账号异常
// API Key 未配置错误码：仅返回错误，不标记账号
func (s *GatewayService) handleRetryExhaustedError(ctx context.Context, resp *http.Response, c *gin.Context, account *Account) (*ForwardResult, error) {
	MarkResponseCommitted(c)
	// Capture upstream error body before side-effects consume the stream.
	respBody, _ := s.readUpstreamErrorBody(resp)
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(respBody))

	s.handleRetryExhaustedSideEffects(ctx, resp, account)

	upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
	upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)

	if isClaudeCodeCredentialScopeError(upstreamMsg) && c != nil {
		if v, ok := c.Get(claudeMimicDebugInfoKey); ok {
			if line, ok := v.(string); ok && strings.TrimSpace(line) != "" {
				logger.LegacyPrintf("service.gateway", "[ClaudeMimicDebugOnError] status=%d request_id=%s %s",
					resp.StatusCode,
					resp.Header.Get("x-request-id"),
					line,
				)
			}
		}
	}

	upstreamDetail := ""
	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
		}
		upstreamDetail = truncateString(string(respBody), maxBytes)
	}
	setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, upstreamDetail)
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:           account.Platform,
		AccountID:          account.ID,
		UpstreamStatusCode: resp.StatusCode,
		UpstreamRequestID:  resp.Header.Get("x-request-id"),
		Kind:               "retry_exhausted",
		Message:            upstreamMsg,
		Detail:             upstreamDetail,
	})

	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		logger.LegacyPrintf("service.gateway",
			"Upstream error %d retries_exhausted (account=%d platform=%s type=%s): %s",
			resp.StatusCode,
			account.ID,
			account.Platform,
			account.Type,
			truncateForLog(respBody, s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes),
		)
	}

	if status, errType, errMsg, matched := applyErrorPassthroughRule(
		c,
		account.Platform,
		resp.StatusCode,
		respBody,
		http.StatusBadGateway,
		"upstream_error",
		"Upstream request failed after retries",
	); matched {
		c.JSON(status, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    errType,
				"message": errMsg,
			},
		})

		summary := upstreamMsg
		if summary == "" {
			summary = errMsg
		}
		if summary == "" {
			return nil, fmt.Errorf("upstream error: %d (retries exhausted, passthrough rule matched)", resp.StatusCode)
		}
		return nil, fmt.Errorf("upstream error: %d (retries exhausted, passthrough rule matched) message=%s", resp.StatusCode, summary)
	}

	// 返回统一的重试耗尽错误响应
	c.JSON(http.StatusBadGateway, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    "upstream_error",
			"message": "Upstream request failed after retries",
		},
	})

	if upstreamMsg == "" {
		return nil, fmt.Errorf("upstream error: %d (retries exhausted)", resp.StatusCode)
	}
	return nil, fmt.Errorf("upstream error: %d (retries exhausted) message=%s", resp.StatusCode, upstreamMsg)
}

// streamingResult 流式响应结果
type streamingResult struct {
	usage            *ClaudeUsage
	firstTokenMs     *int
	clientDisconnect bool // 客户端是否在流式传输过程中断开
}

func (s *GatewayService) handleStreamingResponse(ctx context.Context, resp *http.Response, c *gin.Context, account *Account, startTime time.Time, originalModel, mappedModel string, mimicClaudeCode bool) (*streamingResult, error) {
	// 更新5h窗口状态
	s.rateLimitService.UpdateSessionWindow(ctx, account, resp.Header)

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}

	// 设置SSE响应头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	// 透传其他响应头
	if v := resp.Header.Get("x-request-id"); v != "" {
		c.Header("x-request-id", v)
	}

	w := c.Writer
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
	}

	usage := &ClaudeUsage{}
	var firstTokenMs *int
	scanner := bufio.NewScanner(resp.Body)
	// 设置更大的buffer以处理长行
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	scanBuf := getSSEScannerBuf64K()
	scanner.Buffer(scanBuf[:0], maxLineSize)

	type scanEvent struct {
		line string
		err  error
	}
	// 独立 goroutine 读取上游，避免读取阻塞导致超时/keepalive无法处理
	events := make(chan scanEvent, 16)
	done := make(chan struct{})
	sendEvent := func(ev scanEvent) bool {
		select {
		case events <- ev:
			return true
		case <-done:
			return false
		}
	}
	var lastReadAt int64
	atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
	go func(scanBuf *sseScannerBuf64K) {
		defer putSSEScannerBuf64K(scanBuf)
		defer close(events)
		for scanner.Scan() {
			atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
			if !sendEvent(scanEvent{line: scanner.Text()}) {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			_ = sendEvent(scanEvent{err: err})
		}
	}(scanBuf)
	defer close(done)

	streamInterval := time.Duration(0)
	if s.cfg != nil && s.cfg.Gateway.StreamDataIntervalTimeout > 0 {
		streamInterval = time.Duration(s.cfg.Gateway.StreamDataIntervalTimeout) * time.Second
	}
	// 仅监控上游数据间隔超时，避免下游写入阻塞导致误判
	var intervalTicker *time.Ticker
	if streamInterval > 0 {
		intervalTicker = time.NewTicker(streamInterval)
		defer intervalTicker.Stop()
	}
	var intervalCh <-chan time.Time
	if intervalTicker != nil {
		intervalCh = intervalTicker.C
	}

	// 下游 keepalive：防止代理/Cloudflare Tunnel 因连接空闲而断开
	keepaliveInterval := time.Duration(0)
	if s.cfg != nil && s.cfg.Gateway.StreamKeepaliveInterval > 0 {
		keepaliveInterval = time.Duration(s.cfg.Gateway.StreamKeepaliveInterval) * time.Second
	}
	var keepaliveTimer *time.Timer
	if keepaliveInterval > 0 {
		keepaliveTimer = time.NewTimer(keepaliveInterval)
		defer keepaliveTimer.Stop()
	}
	var keepaliveCh <-chan time.Time
	if keepaliveTimer != nil {
		keepaliveCh = keepaliveTimer.C
	}
	lastDataAt := time.Now()
	resetKeepaliveTimer := func() {
		if keepaliveTimer == nil {
			return
		}
		if !keepaliveTimer.Stop() {
			select {
			case <-keepaliveTimer.C:
			default:
			}
		}
		keepaliveTimer.Reset(keepaliveInterval)
	}

	// 仅发送一次错误事件，避免多次写入导致协议混乱（写失败时尽力通知客户端）。
	// 事件格式遵循 Anthropic SSE 标准：{"type":"error","error":{"type":<reason>,"message":<message>}}
	// 这样 Anthropic SDK / Claude Code 等客户端能按标准 error 类型解析，UI 能显示具体错误文案，
	// 服务端 ExtractUpstreamErrorMessage 也能从透传的 body 中提取 message。
	errorEventSent := false
	sendErrorEvent := func(reason, message string) {
		if errorEventSent {
			return
		}
		errorEventSent = true
		if message == "" {
			message = reason
		}
		body, err := json.Marshal(map[string]any{
			"type": "error",
			"error": map[string]string{
				"type":    reason,
				"message": message,
			},
		})
		if err != nil {
			// json.Marshal 不可能在已知 string-only 输入上失败，保守 fallback
			body = []byte(fmt.Sprintf(`{"type":"error","error":{"type":%q,"message":%q}}`, reason, message))
		}
		_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", body)
		flusher.Flush()
	}

	needModelReplace := originalModel != mappedModel
	clientDisconnected := false // 客户端断开标志，断开后继续读取上游以获取完整usage
	sawTerminalEvent := false
	useNoopDeltaKeepalive := c != nil && c.Request != nil && shouldUseClaudeCodeNoopDeltaKeepalive(c.GetHeader("User-Agent"))
	noopDeltaKeepaliveBlockIndex := -1
	noopDeltaKeepaliveDeltaType := ""

	pendingEventLines := make([]string, 0, 4)

	processSSEEvent := func(lines []string) ([]string, string, *sseUsagePatch, error) {
		if len(lines) == 0 {
			return nil, "", nil, nil
		}

		eventName := ""
		dataLine := ""
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "event:") {
				eventName = strings.TrimSpace(strings.TrimPrefix(trimmed, "event:"))
				continue
			}
			if dataLine == "" && sseDataRe.MatchString(trimmed) {
				dataLine = sseDataRe.ReplaceAllString(trimmed, "")
			}
		}

		if eventName == "error" {
			return nil, dataLine, nil, &sseStreamErrorEventError{RawData: dataLine}
		}

		if dataLine == "" {
			return []string{strings.Join(lines, "\n") + "\n\n"}, "", nil, nil
		}

		if dataLine == "[DONE]" {
			sawTerminalEvent = true
			block := ""
			if eventName != "" {
				block = "event: " + eventName + "\n"
			}
			block += "data: " + dataLine + "\n\n"
			return []string{block}, dataLine, nil, nil
		}

		var event map[string]any
		if err := json.Unmarshal([]byte(dataLine), &event); err != nil {
			// JSON 解析失败，直接透传原始数据
			block := ""
			if eventName != "" {
				block = "event: " + eventName + "\n"
			}
			block += "data: " + dataLine + "\n\n"
			return []string{block}, dataLine, nil, nil
		}

		eventType, _ := event["type"].(string)
		if eventName == "" {
			eventName = eventType
		}
		eventChanged := false

		if useNoopDeltaKeepalive {
			switch eventType {
			case "content_block_start":
				if idx, ok := sseEventIndex(event); ok {
					noopDeltaKeepaliveBlockIndex = -1
					noopDeltaKeepaliveDeltaType = ""
					if contentBlock, ok := event["content_block"].(map[string]any); ok {
						blockType, _ := contentBlock["type"].(string)
						if deltaType := claudeCodeKeepaliveDeltaTypeForContentBlock(blockType); deltaType != "" {
							noopDeltaKeepaliveBlockIndex = idx
							noopDeltaKeepaliveDeltaType = deltaType
						}
					}
				}
			case "content_block_delta":
				if idx, ok := sseEventIndex(event); ok {
					if delta, ok := event["delta"].(map[string]any); ok {
						deltaType, _ := delta["type"].(string)
						if claudeCodeKeepaliveFieldForDeltaType(deltaType) != "" {
							noopDeltaKeepaliveBlockIndex = idx
							noopDeltaKeepaliveDeltaType = deltaType
						}
					}
				}
			case "content_block_stop":
				if idx, ok := sseEventIndex(event); ok && idx == noopDeltaKeepaliveBlockIndex {
					noopDeltaKeepaliveBlockIndex = -1
					noopDeltaKeepaliveDeltaType = ""
				}
			case "message_stop":
				noopDeltaKeepaliveBlockIndex = -1
				noopDeltaKeepaliveDeltaType = ""
			}
		}

		// 兼容 Kimi cached_tokens → cache_read_input_tokens
		if eventType == "message_start" {
			if msg, ok := event["message"].(map[string]any); ok {
				if u, ok := msg["usage"].(map[string]any); ok {
					eventChanged = reconcileCachedTokens(u) || eventChanged
				}
			}
		}
		if eventType == "message_delta" {
			if u, ok := event["usage"].(map[string]any); ok {
				eventChanged = reconcileCachedTokens(u) || eventChanged
			}
		}

		// Cache TTL Override: 重写 SSE 事件中的 cache_creation 分类。
		// 账号级设置优先；全局 1h 请求注入开启时，默认把 usage 计费归回 5m。
		if overrideTarget, ok := s.resolveCacheTTLUsageOverrideTarget(ctx, account); ok {
			if eventType == "message_start" {
				if msg, ok := event["message"].(map[string]any); ok {
					if u, ok := msg["usage"].(map[string]any); ok {
						eventChanged = rewriteCacheCreationJSON(u, overrideTarget) || eventChanged
					}
				}
			}
			if eventType == "message_delta" {
				if u, ok := event["usage"].(map[string]any); ok {
					eventChanged = rewriteCacheCreationJSON(u, overrideTarget) || eventChanged
				}
			}
		}

		if needModelReplace {
			if msg, ok := event["message"].(map[string]any); ok {
				if model, ok := msg["model"].(string); ok && model == mappedModel {
					msg["model"] = originalModel
					eventChanged = true
				}
			}
		}

		usagePatch := s.extractSSEUsagePatch(event)
		if anthropicStreamEventIsTerminal(eventName, dataLine) {
			sawTerminalEvent = true
		}
		if !eventChanged {
			block := ""
			if eventName != "" {
				block = "event: " + eventName + "\n"
			}
			block += "data: " + dataLine + "\n\n"
			return []string{block}, dataLine, usagePatch, nil
		}

		newData, err := json.Marshal(event)
		if err != nil {
			// 序列化失败，直接透传原始数据
			block := ""
			if eventName != "" {
				block = "event: " + eventName + "\n"
			}
			block += "data: " + dataLine + "\n\n"
			return []string{block}, dataLine, usagePatch, nil
		}

		block := ""
		if eventName != "" {
			block = "event: " + eventName + "\n"
		}
		block += "data: " + string(newData) + "\n\n"
		return []string{block}, string(newData), usagePatch, nil
	}

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				// 上游完成，返回结果
				if !sawTerminalEvent {
					return &streamingResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: clientDisconnected}, fmt.Errorf("stream usage incomplete: missing terminal event")
				}
				return &streamingResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: clientDisconnected}, nil
			}
			if ev.err != nil {
				if sawTerminalEvent {
					return &streamingResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: clientDisconnected}, nil
				}
				// 检测 context 取消（客户端断开会导致 context 取消，进而影响上游读取）
				if errors.Is(ev.err, context.Canceled) || errors.Is(ev.err, context.DeadlineExceeded) {
					return &streamingResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: true}, fmt.Errorf("stream usage incomplete: %w", ev.err)
				}
				// 客户端已通过写入失败检测到断开，上游也出错了，返回已收集的 usage
				if clientDisconnected {
					return &streamingResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: true}, fmt.Errorf("stream usage incomplete after disconnect: %w", ev.err)
				}
				// 客户端未断开，正常的错误处理
				if errors.Is(ev.err, bufio.ErrTooLong) {
					logger.LegacyPrintf("service.gateway", "SSE line too long: account=%d max_size=%d error=%v", account.ID, maxLineSize, ev.err)
					sendErrorEvent("response_too_large", fmt.Sprintf("upstream SSE line exceeded %d bytes", maxLineSize))
					return &streamingResult{usage: usage, firstTokenMs: firstTokenMs}, ev.err
				}
				// 上游中途读错误（unexpected EOF / connection reset 等，常见于 HTTP/2 GOAWAY）：
				// 若尚未向客户端写过任何字节，包成 UpstreamFailoverError 让 handler 层走 failover/重试。
				// 已经开始写流时 SSE 协议无 resume，只能透传错误事件给客户端。
				// 注意:面向客户端的 disconnectMsg 必须用 sanitizeStreamError 剥离地址,
				// 默认 *net.OpError 的 Error() 会泄露内部 IP/端口和上游地址。完整 ev.err
				// 仅在下方 LegacyPrintf 内部日志中保留供运维诊断。
				disconnectMsg := "upstream stream disconnected: " + sanitizeStreamError(ev.err)
				if !c.Writer.Written() {
					logger.LegacyPrintf("service.gateway", "Upstream stream read error before any client output (account=%d), failing over: %v", account.ID, ev.err)
					body, _ := json.Marshal(map[string]any{
						"type": "error",
						"error": map[string]string{
							"type":    "upstream_disconnected",
							"message": disconnectMsg,
						},
					})
					return nil, &UpstreamFailoverError{
						StatusCode:             http.StatusBadGateway,
						ResponseBody:           body,
						RetryableOnSameAccount: true,
					}
				}
				sendErrorEvent("stream_read_error", disconnectMsg)
				return &streamingResult{usage: usage, firstTokenMs: firstTokenMs}, fmt.Errorf("stream read error: %w", ev.err)
			}
			line := ev.line
			trimmed := strings.TrimSpace(line)

			if trimmed == "" {
				if len(pendingEventLines) == 0 {
					continue
				}

				outputBlocks, data, usagePatch, err := processSSEEvent(pendingEventLines)
				pendingEventLines = pendingEventLines[:0]
				if err != nil {
					if clientDisconnected {
						return &streamingResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: true}, nil
					}
					return nil, err
				}

				for _, block := range outputBlocks {
					if !clientDisconnected {
						restored := reverseToolNamesIfPresent(c, []byte(block))
						if _, werr := fmt.Fprint(w, string(restored)); werr != nil {
							clientDisconnected = true
							logger.LegacyPrintf("service.gateway", "Client disconnected during streaming, continuing to drain upstream for billing")
							break
						}
						flusher.Flush()
						lastDataAt = time.Now()
						resetKeepaliveTimer()
					}
					if data != "" {
						if firstTokenMs == nil && data != "[DONE]" {
							ms := int(time.Since(startTime).Milliseconds())
							firstTokenMs = &ms
						}
						if usagePatch != nil {
							mergeSSEUsagePatch(usage, usagePatch)
						}
					}
				}
				continue
			}

			pendingEventLines = append(pendingEventLines, line)

		case <-intervalCh:
			lastRead := time.Unix(0, atomic.LoadInt64(&lastReadAt))
			if time.Since(lastRead) < streamInterval {
				continue
			}
			if clientDisconnected {
				return &streamingResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: true}, fmt.Errorf("stream usage incomplete after timeout")
			}
			logger.LegacyPrintf("service.gateway", "Stream data interval timeout: account=%d model=%s interval=%s", account.ID, originalModel, streamInterval)
			// 处理流超时，可能标记账户为临时不可调度或错误状态
			if s.rateLimitService != nil {
				s.rateLimitService.HandleStreamTimeout(ctx, account, originalModel)
			}
			sendErrorEvent("stream_timeout", fmt.Sprintf("upstream stream idle for %s", streamInterval))
			return &streamingResult{usage: usage, firstTokenMs: firstTokenMs}, fmt.Errorf("stream data interval timeout")

		case <-keepaliveCh:
			if clientDisconnected {
				continue
			}
			if time.Since(lastDataAt) < keepaliveInterval {
				resetKeepaliveTimer()
				continue
			}
			keepaliveBlock := "event: ping\ndata: {\"type\": \"ping\"}\n\n"
			if useNoopDeltaKeepalive && noopDeltaKeepaliveBlockIndex >= 0 {
				if block, ok := buildClaudeCodeNoopDeltaKeepalive(noopDeltaKeepaliveBlockIndex, noopDeltaKeepaliveDeltaType); ok {
					keepaliveBlock = block
				}
			}
			if _, werr := fmt.Fprint(w, keepaliveBlock); werr != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.gateway", "Client disconnected during keepalive ping, continuing to drain upstream for billing")
				continue
			}
			flusher.Flush()
			lastDataAt = time.Now()
			resetKeepaliveTimer()
		}
	}

}

func (s *GatewayService) parseSSEUsage(data string, usage *ClaudeUsage) {
	if usage == nil {
		return
	}

	var event map[string]any
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return
	}

	if patch := s.extractSSEUsagePatch(event); patch != nil {
		mergeSSEUsagePatch(usage, patch)
	}
}

type sseUsagePatch struct {
	inputTokens              int
	hasInputTokens           bool
	outputTokens             int
	hasOutputTokens          bool
	cacheCreationInputTokens int
	hasCacheCreationInput    bool
	cacheReadInputTokens     int
	hasCacheReadInput        bool
	cacheCreation5mTokens    int
	hasCacheCreation5m       bool
	cacheCreation1hTokens    int
	hasCacheCreation1h       bool
}

func (s *GatewayService) extractSSEUsagePatch(event map[string]any) *sseUsagePatch {
	if len(event) == 0 {
		return nil
	}

	eventType, _ := event["type"].(string)
	switch eventType {
	case "message_start":
		msg, _ := event["message"].(map[string]any)
		usageObj, _ := msg["usage"].(map[string]any)
		if len(usageObj) == 0 {
			return nil
		}

		patch := &sseUsagePatch{}
		patch.hasInputTokens = true
		if v, ok := parseSSEUsageInt(usageObj["input_tokens"]); ok {
			patch.inputTokens = v
		}
		patch.hasCacheCreationInput = true
		if v, ok := parseSSEUsageInt(usageObj["cache_creation_input_tokens"]); ok {
			patch.cacheCreationInputTokens = v
		}
		patch.hasCacheReadInput = true
		if v, ok := parseSSEUsageInt(usageObj["cache_read_input_tokens"]); ok {
			patch.cacheReadInputTokens = v
		}
		if cc, ok := usageObj["cache_creation"].(map[string]any); ok {
			if v, exists := parseSSEUsageInt(cc["ephemeral_5m_input_tokens"]); exists {
				patch.cacheCreation5mTokens = v
				patch.hasCacheCreation5m = true
			}
			if v, exists := parseSSEUsageInt(cc["ephemeral_1h_input_tokens"]); exists {
				patch.cacheCreation1hTokens = v
				patch.hasCacheCreation1h = true
			}
		}
		return patch

	case "message_delta":
		usageObj, _ := event["usage"].(map[string]any)
		if len(usageObj) == 0 {
			return nil
		}

		patch := &sseUsagePatch{}
		if v, ok := parseSSEUsageInt(usageObj["input_tokens"]); ok && v > 0 {
			patch.inputTokens = v
			patch.hasInputTokens = true
		}
		if v, ok := parseSSEUsageInt(usageObj["output_tokens"]); ok && v > 0 {
			patch.outputTokens = v
			patch.hasOutputTokens = true
		}
		if v, ok := parseSSEUsageInt(usageObj["cache_creation_input_tokens"]); ok && v > 0 {
			patch.cacheCreationInputTokens = v
			patch.hasCacheCreationInput = true
		}
		if v, ok := parseSSEUsageInt(usageObj["cache_read_input_tokens"]); ok && v > 0 {
			patch.cacheReadInputTokens = v
			patch.hasCacheReadInput = true
		}
		if cc, ok := usageObj["cache_creation"].(map[string]any); ok {
			if v, exists := parseSSEUsageInt(cc["ephemeral_5m_input_tokens"]); exists && v > 0 {
				patch.cacheCreation5mTokens = v
				patch.hasCacheCreation5m = true
			}
			if v, exists := parseSSEUsageInt(cc["ephemeral_1h_input_tokens"]); exists && v > 0 {
				patch.cacheCreation1hTokens = v
				patch.hasCacheCreation1h = true
			}
		}
		return patch
	}

	return nil
}

func mergeSSEUsagePatch(usage *ClaudeUsage, patch *sseUsagePatch) {
	if usage == nil || patch == nil {
		return
	}

	if patch.hasInputTokens {
		usage.InputTokens = patch.inputTokens
	}
	if patch.hasCacheCreationInput {
		usage.CacheCreationInputTokens = patch.cacheCreationInputTokens
	}
	if patch.hasCacheReadInput {
		usage.CacheReadInputTokens = patch.cacheReadInputTokens
	}
	if patch.hasOutputTokens {
		usage.OutputTokens = patch.outputTokens
	}
	if patch.hasCacheCreation5m {
		usage.CacheCreation5mTokens = patch.cacheCreation5mTokens
	}
	if patch.hasCacheCreation1h {
		usage.CacheCreation1hTokens = patch.cacheCreation1hTokens
	}
}

func parseSSEUsageInt(value any) (int, bool) {
	switch v := value.(type) {
	case float64:
		return int(v), true
	case float32:
		return int(v), true
	case int:
		return v, true
	case int64:
		return int(v), true
	case int32:
		return int(v), true
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return int(i), true
		}
		if f, err := v.Float64(); err == nil {
			return int(f), true
		}
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return parsed, true
		}
	}
	return 0, false
}

// applyCacheTTLOverride 将所有 cache creation tokens 归入指定的 TTL 类型。
// target 为 "5m" 或 "1h"。返回 true 表示发生了变更。
func applyCacheTTLOverride(usage *ClaudeUsage, target string) bool {
	// Fallback: 如果只有聚合字段但无 5m/1h 明细，将聚合字段归入 5m 默认类别
	if usage.CacheCreation5mTokens == 0 && usage.CacheCreation1hTokens == 0 && usage.CacheCreationInputTokens > 0 {
		usage.CacheCreation5mTokens = usage.CacheCreationInputTokens
	}

	total := usage.CacheCreation5mTokens + usage.CacheCreation1hTokens
	if total == 0 {
		return false
	}
	switch target {
	case "1h":
		if usage.CacheCreation1hTokens == total {
			return false // 已经全是 1h
		}
		usage.CacheCreation1hTokens = total
		usage.CacheCreation5mTokens = 0
	default: // "5m"
		if usage.CacheCreation5mTokens == total {
			return false // 已经全是 5m
		}
		usage.CacheCreation5mTokens = total
		usage.CacheCreation1hTokens = 0
	}
	return true
}

// rewriteCacheCreationJSON 在 JSON usage 对象中重写 cache_creation 嵌套对象的 TTL 分类。
// usageObj 是 usage JSON 对象（map[string]any）。
func rewriteCacheCreationJSON(usageObj map[string]any, target string) bool {
	ccObj, ok := usageObj["cache_creation"].(map[string]any)
	if !ok {
		return false
	}
	v5m, _ := parseSSEUsageInt(ccObj["ephemeral_5m_input_tokens"])
	v1h, _ := parseSSEUsageInt(ccObj["ephemeral_1h_input_tokens"])
	total := v5m + v1h
	if total == 0 {
		return false
	}
	switch target {
	case "1h":
		if v1h == total {
			return false
		}
		ccObj["ephemeral_1h_input_tokens"] = float64(total)
		ccObj["ephemeral_5m_input_tokens"] = float64(0)
	default: // "5m"
		if v5m == total {
			return false
		}
		ccObj["ephemeral_5m_input_tokens"] = float64(total)
		ccObj["ephemeral_1h_input_tokens"] = float64(0)
	}
	return true
}

func (s *GatewayService) resolveCacheTTLUsageOverrideTarget(ctx context.Context, account *Account) (string, bool) {
	if account == nil {
		return "", false
	}
	if account.IsCacheTTLOverrideEnabled() {
		return account.GetCacheTTLOverrideTarget(), true
	}
	if account.IsAnthropicOAuthOrSetupToken() && s != nil && s.settingService != nil && s.settingService.IsAnthropicCacheTTL1hInjectionEnabled(ctx) {
		return cacheTTLTarget5m, true
	}
	return "", false
}

func (s *GatewayService) handleNonStreamingResponse(ctx context.Context, resp *http.Response, c *gin.Context, account *Account, originalModel, mappedModel string) (*ClaudeUsage, error) {
	// 更新5h窗口状态
	s.rateLimitService.UpdateSessionWindow(ctx, account, resp.Header)

	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, anthropicTooLargeError)
	if err != nil {
		return nil, err
	}

	// 解析usage
	var response struct {
		Usage ClaudeUsage `json:"usage"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
			return nil, s.invalidNonStreamingJSONFailoverError(ctx, resp, account, body, err, mappedModel)
		}
		return nil, fmt.Errorf("parse response: %w", err)
	}

	// 解析嵌套的 cache_creation 对象中的 5m/1h 明细
	cc5m := gjson.GetBytes(body, "usage.cache_creation.ephemeral_5m_input_tokens")
	cc1h := gjson.GetBytes(body, "usage.cache_creation.ephemeral_1h_input_tokens")
	if cc5m.Exists() || cc1h.Exists() {
		response.Usage.CacheCreation5mTokens = int(cc5m.Int())
		response.Usage.CacheCreation1hTokens = int(cc1h.Int())
	}

	// 兼容 Kimi cached_tokens → cache_read_input_tokens
	if response.Usage.CacheReadInputTokens == 0 {
		cachedTokens := gjson.GetBytes(body, "usage.cached_tokens").Int()
		if cachedTokens > 0 {
			response.Usage.CacheReadInputTokens = int(cachedTokens)
			if newBody, err := sjson.SetBytes(body, "usage.cache_read_input_tokens", cachedTokens); err == nil {
				body = newBody
			}
		}
	}

	// Cache TTL Override: 重写 non-streaming 响应中的 cache_creation 分类。
	// 账号级设置优先；全局 1h 请求注入开启时，默认把 usage 计费归回 5m。
	if overrideTarget, ok := s.resolveCacheTTLUsageOverrideTarget(ctx, account); ok {
		if applyCacheTTLOverride(&response.Usage, overrideTarget) {
			// 同步更新 body JSON 中的嵌套 cache_creation 对象
			if newBody, err := sjson.SetBytes(body, "usage.cache_creation.ephemeral_5m_input_tokens", response.Usage.CacheCreation5mTokens); err == nil {
				body = newBody
			}
			if newBody, err := sjson.SetBytes(body, "usage.cache_creation.ephemeral_1h_input_tokens", response.Usage.CacheCreation1hTokens); err == nil {
				body = newBody
			}
		}
	}

	// 如果有模型映射，替换响应中的model字段
	if originalModel != mappedModel {
		body = s.replaceModelInResponseBody(body, mappedModel, originalModel)
	}

	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)

	contentType := "application/json"
	if s.cfg != nil && !s.cfg.Security.ResponseHeaders.Enabled {
		if upstreamType := resp.Header.Get("Content-Type"); upstreamType != "" {
			contentType = upstreamType
		}
	}

	body = reverseToolNamesIfPresent(c, body)

	// 写入响应
	c.Data(resp.StatusCode, contentType, body)

	return &response.Usage, nil
}

// replaceModelInResponseBody 替换响应体中的model字段
// 使用 gjson/sjson 精确替换，避免全量 JSON 反序列化
func (s *GatewayService) replaceModelInResponseBody(body []byte, fromModel, toModel string) []byte {
	if m := gjson.GetBytes(body, "model"); m.Exists() && m.Str == fromModel {
		newBody, err := sjson.SetBytes(body, "model", toModel)
		if err != nil {
			return body
		}
		return newBody
	}
	return body
}

// reconcileCachedTokens 兼容 Kimi 等上游：
// 将 OpenAI 风格的 cached_tokens 映射到 Claude 标准的 cache_read_input_tokens
func reconcileCachedTokens(usage map[string]any) bool {
	if usage == nil {
		return false
	}
	cacheRead, _ := usage["cache_read_input_tokens"].(float64)
	if cacheRead > 0 {
		return false // 已有标准字段，无需处理
	}
	cached, _ := usage["cached_tokens"].(float64)
	if cached <= 0 {
		return false
	}
	usage["cache_read_input_tokens"] = cached
	return true
}
