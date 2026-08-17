package service

// 国产供应商（kimi/zhipu/deepseek）原生 Anthropic 端点直通路径。
//
// 当账号 credentials["api_protocol"] = "anthropic" 时，入站 /v1/messages 请求
// 不再做 Anthropic→CC→Anthropic 双重转换，而是零转换直通供应商的官方
// Anthropic 兼容端点（如 https://open.bigmodel.cn/api/anthropic/v1/messages），
// 适配 Claude Code 等原生 Anthropic 客户端。转发骨架以
// gateway_anthropic_passthrough.go 的 APIKey 透传为模板（字节级 SSE 中继 +
// usage 解析），错误/failover 语义对齐 OpenAI 网关其他路径
// （failoverOpenAIUpstreamHTTPError / handleAnthropicErrorResponse）。

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// forwardAnthropicViaNativeAnthropicEndpoint 将 Anthropic Messages 请求零转换
// 直通到国产供应商的原生 Anthropic 端点。仅做模型名映射与少量 body 清洗
// （空文本块 / web-search 历史块），协议本身不转换。
func (s *OpenAIGatewayService) forwardAnthropicViaNativeAnthropicEndpoint(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	defaultMappedModel string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()

	originalModel := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if originalModel == "" {
		writeAnthropicError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return nil, fmt.Errorf("missing model in request")
	}
	clientStream := gjson.GetBytes(body, "stream").Bool()

	billingModel := resolveOpenAIForwardModel(account, originalModel, defaultMappedModel)
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)
	if upstreamModel != originalModel {
		rewritten, err := sjson.SetBytes(body, "model", upstreamModel)
		if err != nil {
			return nil, fmt.Errorf("rewrite model: %w", err)
		}
		body = rewritten
	}

	// 与 Anthropic 平台 passthrough 相同的 pre-filter：剥离空文本块与上游
	// 无法接受的 web-search 历史块（GLM/Kimi/DeepSeek 对 server_tool_use 400）。
	body = StripEmptyTextBlocks(body)
	body = FilterWebSearchHistoryBlocks(body, upstreamModel)

	logger.LegacyPrintf("service.gateway", "[CN Anthropic 直通] account=%d(%s) platform=%s model=%s upstream=%s stream=%v",
		account.ID, account.Name, account.Platform, originalModel, upstreamModel, clientStream)

	apiKey := strings.TrimSpace(account.GetOpenAIProtocolAPIKey())
	if apiKey == "" {
		return nil, fmt.Errorf("account %d missing api_key", account.ID)
	}
	targetURL, err := s.nativeAnthropicTargetURL(account)
	if err != nil {
		return nil, err
	}

	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	upstreamCtx, releaseUpstreamCtx := detachStreamUpstreamContext(ctx, clientStream)
	upstreamReq, _, err := s.buildNativeAnthropicUpstreamRequest(upstreamCtx, c, account, body, apiKey, targetURL)
	releaseUpstreamCtx()
	if err != nil {
		return nil, err
	}

	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, true)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody, upstreamMsg := s.readOpenAIUpstreamError(resp)
		if foErr := s.failoverOpenAIUpstreamHTTPError(ctx, c, account, resp, respBody, upstreamMsg, upstreamModel); foErr != nil {
			return nil, foErr
		}
		// 非 failover 错误：经共享 compat handler 以 Anthropic 格式回写
		// （透传规则、ops 记录、cyber_policy 与 CC 回退路径一致）。
		return s.handleAnthropicErrorResponse(resp, c, account, billingModel)
	}

	if clientStream {
		return s.handleNativeAnthropicStreamingResponse(ctx, resp, c, account, originalModel, billingModel, upstreamModel, startTime)
	}
	return s.handleNativeAnthropicBufferedResponse(ctx, resp, c, account, originalModel, billingModel, upstreamModel, startTime)
}

// nativeAnthropicTargetURL 组装国产供应商原生 Anthropic messages 端点。
// 第三方端点保持朴素路径，不附加 ?beta=true。
func (s *OpenAIGatewayService) nativeAnthropicTargetURL(account *Account) (string, error) {
	baseURL := strings.TrimSpace(account.GetAnthropicProtocolBaseURL())
	if baseURL == "" {
		return "", fmt.Errorf("account %d has no anthropic protocol base url", account.ID)
	}
	validatedURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base_url: %w", err)
	}
	return strings.TrimRight(validatedURL, "/") + "/v1/messages", nil
}

func (s *OpenAIGatewayService) buildNativeAnthropicUpstreamRequest(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	apiKey string,
	targetURL string,
) (*http.Request, []byte, error) {
	// 能力维度 body sanitize：与 Anthropic 平台 passthrough 相同，按 beta
	// header 决定是否保留 body 中的 beta 能力字段，避免客户端"body 带字段但
	// header 忘带 token"的 bug 让第三方上游 400。
	clientBeta := ""
	if c != nil && c.Request != nil {
		clientBeta = getHeaderRaw(c.Request.Header, "anthropic-beta")
	}
	if beta, ok := account.HeaderOverrideValue("anthropic-beta"); ok {
		clientBeta = beta
	}
	if sanitized, changed := sanitizeAnthropicBodyForBetaTokens(body, clientBeta); changed {
		body = sanitized
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}

	if c != nil && c.Request != nil {
		for key, values := range c.Request.Header {
			lowerKey := strings.ToLower(strings.TrimSpace(key))
			if !allowedHeaders[lowerKey] {
				continue
			}
			wireKey := resolveWireCasing(key)
			for _, v := range values {
				addHeaderRaw(req.Header, wireKey, v)
			}
		}
	}

	// 覆盖入站鉴权残留，注入上游认证（默认 x-api-key；可经 extra
	// anthropic_apikey_auth_scheme 切换 Authorization: Bearer）。
	req.Header.Del("authorization")
	req.Header.Del("x-api-key")
	req.Header.Del("x-goog-api-key")
	req.Header.Del("cookie")
	setAnthropicAPIKeyAuthHeader(req.Header, account, apiKey)

	if getHeaderRaw(req.Header, "content-type") == "" {
		setHeaderRaw(req.Header, "content-type", "application/json")
	}
	if getHeaderRaw(req.Header, "anthropic-version") == "" {
		setHeaderRaw(req.Header, "anthropic-version", "2023-06-01")
	}

	// 账号级请求头覆写（最终生效，覆盖上面所有来源的同名头）
	account.ApplyHeaderOverrides(req.Header)

	return req, body, nil
}

// handleNativeAnthropicBufferedResponse 处理非流式原生 Anthropic 响应：
// 校验 JSON、解析 usage、透传响应头后原样回写（仅工具名反向还原）。
func (s *OpenAIGatewayService) handleNativeAnthropicBufferedResponse(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
	originalModel string,
	billingModel string,
	upstreamModel string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	if s.rateLimitService != nil {
		s.rateLimitService.UpdateSessionWindow(ctx, account, resp.Header)
	}

	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, anthropicTooLargeError)
	if err != nil {
		return nil, err
	}
	observer := upstreamResponseModelObserverFromContext(c)
	if observer == nil {
		observer = beginUpstreamResponseModelObservation(c)
	}
	observer.ObserveAnthropic(body)

	var raw json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, invalidNonStreamingJSONFailoverError(ctx, s.rateLimitService, resp, account, body, err, billingModel)
	}

	usage := parseClaudeUsageFromResponseBody(body)
	if IsForceCacheBilling(ctx) && usage.InputTokens > 0 {
		body, err = classifyAnthropicResponseInputAsCacheRead(body, usage)
		if err != nil {
			return nil, err
		}
	}

	writeAnthropicPassthroughResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/json"
	}
	body = reverseToolNamesIfPresent(c, body)
	c.Data(resp.StatusCode, contentType, body)

	return &OpenAIForwardResult{
		RequestID:        resp.Header.Get("x-request-id"),
		Usage:            claudeUsageToOpenAIUsage(usage),
		Model:            originalModel,
		BillingModel:     billingModel,
		UpstreamModel:    upstreamModel,
		UpstreamEndpoint: "/v1/messages",
		Stream:           false,
		Duration:         time.Since(startTime),
	}, nil
}

// handleNativeAnthropicStreamingResponse 处理流式原生 Anthropic 响应：
// 字节级 SSE 中继（逐行透传、按事件边界 flush），同时解析 usage。
// 骨架与 handleStreamingResponseAnthropicAPIKeyPassthrough 一致。
func (s *OpenAIGatewayService) handleNativeAnthropicStreamingResponse(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
	originalModel string,
	billingModel string,
	upstreamModel string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	observer := upstreamResponseModelObserverFromContext(c)
	if observer == nil {
		observer = beginUpstreamResponseModelObservation(c)
	}
	if s.rateLimitService != nil {
		s.rateLimitService.UpdateSessionWindow(ctx, account, resp.Header)
	}

	writeAnthropicPassthroughResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)

	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "text/event-stream"
	}
	c.Header("Content-Type", contentType)
	if c.Writer.Header().Get("Cache-Control") == "" {
		c.Header("Cache-Control", "no-cache")
	}
	if c.Writer.Header().Get("Connection") == "" {
		c.Header("Connection", "keep-alive")
	}
	c.Header("X-Accel-Buffering", "no")
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
	clientDisconnected := false
	sawTerminalEvent := false

	scanner := bufio.NewScanner(resp.Body)
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
	var intervalTicker *time.Ticker
	if streamInterval > 0 {
		intervalTicker = time.NewTicker(streamInterval)
		defer intervalTicker.Stop()
	}
	var intervalCh <-chan time.Time
	if intervalTicker != nil {
		intervalCh = intervalTicker.C
	}

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
	inPartialEvent := false

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				if !clientDisconnected {
					flusher.Flush()
				}
				if !sawTerminalEvent {
					return s.nativeAnthropicStreamResult(c, resp, usage, firstTokenMs, clientDisconnected, originalModel, billingModel, upstreamModel, startTime),
						fmt.Errorf("stream usage incomplete: missing terminal event")
				}
				return s.nativeAnthropicStreamResult(c, resp, usage, firstTokenMs, clientDisconnected, originalModel, billingModel, upstreamModel, startTime), nil
			}
			if ev.err != nil {
				if sawTerminalEvent {
					return s.nativeAnthropicStreamResult(c, resp, usage, firstTokenMs, clientDisconnected, originalModel, billingModel, upstreamModel, startTime), nil
				}
				if clientDisconnected {
					return s.nativeAnthropicStreamResult(c, resp, usage, firstTokenMs, clientDisconnected, originalModel, billingModel, upstreamModel, startTime),
						fmt.Errorf("stream usage incomplete after disconnect: %w", ev.err)
				}
				if errors.Is(ev.err, context.Canceled) || errors.Is(ev.err, context.DeadlineExceeded) {
					return s.nativeAnthropicStreamResult(c, resp, usage, firstTokenMs, clientDisconnected, originalModel, billingModel, upstreamModel, startTime),
						fmt.Errorf("stream usage incomplete: %w", ev.err)
				}
				if errors.Is(ev.err, bufio.ErrTooLong) {
					logger.LegacyPrintf("service.gateway", "[CN Anthropic 直通] SSE line too long: account=%d max_size=%d error=%v", account.ID, maxLineSize, ev.err)
					return s.nativeAnthropicStreamResult(c, resp, usage, firstTokenMs, clientDisconnected, originalModel, billingModel, upstreamModel, startTime), ev.err
				}
				return s.nativeAnthropicStreamResult(c, resp, usage, firstTokenMs, clientDisconnected, originalModel, billingModel, upstreamModel, startTime),
					fmt.Errorf("stream read error: %w", ev.err)
			}

			line := ev.line
			if data, ok := extractAnthropicSSEDataLine(line); ok {
				trimmed := strings.TrimSpace(data)
				observer.ObserveAnthropic([]byte(trimmed))
				if anthropicStreamEventIsTerminal("", trimmed) {
					sawTerminalEvent = true
				}
				if firstTokenMs == nil && trimmed != "" && trimmed != "[DONE]" {
					ms := int(time.Since(startTime).Milliseconds())
					firstTokenMs = &ms
				}
				parseSSEUsagePassthrough(data, usage)
			} else {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "event:") && anthropicStreamEventIsTerminal(strings.TrimSpace(strings.TrimPrefix(trimmed, "event:")), "") {
					sawTerminalEvent = true
				}
			}

			if !clientDisconnected {
				restored := string(reverseToolNamesIfPresent(c, []byte(line)))
				if _, err := io.WriteString(w, restored); err != nil {
					clientDisconnected = true
					logger.LegacyPrintf("service.gateway", "[CN Anthropic 直通] Client disconnected during streaming, continue draining upstream for usage: account=%d", account.ID)
				} else if _, err := io.WriteString(w, "\n"); err != nil {
					clientDisconnected = true
					logger.LegacyPrintf("service.gateway", "[CN Anthropic 直通] Client disconnected during streaming, continue draining upstream for usage: account=%d", account.ID)
				} else if line == "" {
					// 按 SSE 事件边界刷出，减少每行 flush 带来的 syscall 开销。
					flusher.Flush()
					lastDataAt = time.Now()
					resetKeepaliveTimer()
					inPartialEvent = false
				} else {
					inPartialEvent = true
				}
			}

		case <-intervalCh:
			lastRead := time.Unix(0, atomic.LoadInt64(&lastReadAt))
			if time.Since(lastRead) < streamInterval {
				continue
			}
			if clientDisconnected {
				return s.nativeAnthropicStreamResult(c, resp, usage, firstTokenMs, clientDisconnected, originalModel, billingModel, upstreamModel, startTime),
					fmt.Errorf("stream usage incomplete after timeout")
			}
			logger.LegacyPrintf("service.gateway", "[CN Anthropic 直通] Stream data interval timeout: account=%d model=%s interval=%s", account.ID, upstreamModel, streamInterval)
			if s.rateLimitService != nil {
				s.rateLimitService.HandleStreamTimeout(ctx, account, upstreamModel)
			}
			return s.nativeAnthropicStreamResult(c, resp, usage, firstTokenMs, clientDisconnected, originalModel, billingModel, upstreamModel, startTime),
				fmt.Errorf("stream data interval timeout")

		case <-keepaliveCh:
			if clientDisconnected {
				continue
			}
			if inPartialEvent {
				resetKeepaliveTimer()
				continue
			}
			if time.Since(lastDataAt) < keepaliveInterval {
				resetKeepaliveTimer()
				continue
			}
			if _, err := fmt.Fprint(w, "event: ping\ndata: {\"type\": \"ping\"}\n\n"); err != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.gateway", "[CN Anthropic 直通] Client disconnected during keepalive ping, continue draining upstream for usage: account=%d", account.ID)
				continue
			}
			flusher.Flush()
			lastDataAt = time.Now()
			resetKeepaliveTimer()
		}
	}
}

// nativeAnthropicStreamResult 组装流式直通结果；流中断时同样返回已观测到的
// usage 与错误一起带出，避免上游已计量的请求漏记漏计费（对齐 issue #5148 语义）。
func (s *OpenAIGatewayService) nativeAnthropicStreamResult(
	c *gin.Context,
	resp *http.Response,
	usage *ClaudeUsage,
	firstTokenMs *int,
	clientDisconnect bool,
	originalModel string,
	billingModel string,
	upstreamModel string,
	startTime time.Time,
) *OpenAIForwardResult {
	if usage == nil {
		usage = &ClaudeUsage{}
	}
	return &OpenAIForwardResult{
		RequestID:        resp.Header.Get("x-request-id"),
		Usage:            claudeUsageToOpenAIUsage(usage),
		Model:            originalModel,
		BillingModel:     billingModel,
		UpstreamModel:    upstreamModel,
		UpstreamEndpoint: "/v1/messages",
		Stream:           true,
		Duration:         time.Since(startTime),
		FirstTokenMs:     firstTokenMs,
		ClientDisconnect: clientDisconnect,
	}
}

// forwardCountTokensViaNativeAnthropic 把 Anthropic count_tokens 请求透传到
// 国产供应商原生 Anthropic 端点（{base}/v1/messages/count_tokens），
// 仅做模型名映射，不做协议转换。
func (s *OpenAIGatewayService) forwardCountTokensViaNativeAnthropic(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	defaultMappedModel string,
) error {
	originalModel := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if originalModel == "" {
		writeAnthropicCountTokensError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return fmt.Errorf("count_tokens: missing model in request")
	}
	billingModel := resolveOpenAIForwardModel(account, originalModel, strings.TrimSpace(defaultMappedModel))
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)
	if upstreamModel != originalModel {
		rewritten, err := sjson.SetBytes(body, "model", upstreamModel)
		if err != nil {
			return fmt.Errorf("count_tokens: rewrite model: %w", err)
		}
		body = rewritten
	}

	apiKey := strings.TrimSpace(account.GetOpenAIProtocolAPIKey())
	if apiKey == "" {
		writeAnthropicCountTokensError(c, http.StatusBadGateway, "upstream_error", "Account api_key is missing")
		return fmt.Errorf("count_tokens: account %d missing api_key", account.ID)
	}
	targetURL, err := s.nativeAnthropicTargetURL(account)
	if err != nil {
		return fmt.Errorf("count_tokens: %w", err)
	}
	targetURL = strings.TrimSuffix(targetURL, "/v1/messages") + "/v1/messages/count_tokens"

	upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		writeAnthropicCountTokensError(c, http.StatusInternalServerError, "api_error", "Failed to build request")
		return fmt.Errorf("count_tokens: build request: %w", err)
	}
	reqHeader := upstreamReq.Header
	reqHeader.Del("authorization")
	reqHeader.Del("x-api-key")
	setAnthropicAPIKeyAuthHeader(reqHeader, account, apiKey)
	reqHeader.Set("content-type", "application/json")
	reqHeader.Set("accept", "application/json")
	account.ApplyHeaderOverrides(reqHeader)

	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		writeAnthropicCountTokensError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed")
		return fmt.Errorf("count_tokens: upstream request failed: %s", safeErr)
	}
	defer func() { _ = resp.Body.Close() }()

	// count_tokens 响应体极小；与其他探测路径一致加 256KB 上限防异常上游放大内存。
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, cnQuotaMaxBodyBytes))
	if err != nil {
		writeAnthropicCountTokensError(c, http.StatusBadGateway, "upstream_error", "Failed to read response")
		return fmt.Errorf("count_tokens: read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		if s.rateLimitService != nil {
			s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
		}
		upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
		setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, "")
		writeAnthropicCountTokensError(c, resp.StatusCode, "upstream_error", "Upstream request failed")
		return fmt.Errorf("count_tokens: upstream error: %d", resp.StatusCode)
	}

	inputTokens := gjson.GetBytes(respBody, "input_tokens")
	if !inputTokens.Exists() {
		writeAnthropicCountTokensError(c, http.StatusBadGateway, "upstream_error", "Upstream response missing input_tokens")
		return fmt.Errorf("count_tokens: response missing input_tokens field")
	}
	c.JSON(http.StatusOK, gin.H{
		"input_tokens": int(inputTokens.Int()),
	})
	return nil
}

// claudeUsageToOpenAIUsage 把 Anthropic 格式 usage 映射到 OpenAI 网关统一的
// 用量结构（字段一一对应）。
func claudeUsageToOpenAIUsage(u *ClaudeUsage) OpenAIUsage {
	if u == nil {
		return OpenAIUsage{}
	}
	return OpenAIUsage{
		InputTokens:              u.InputTokens,
		OutputTokens:             u.OutputTokens,
		CacheCreationInputTokens: u.CacheCreationInputTokens,
		CacheReadInputTokens:     u.CacheReadInputTokens,
	}
}
