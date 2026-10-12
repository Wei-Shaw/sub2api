package service

// 本文件由 gateway_service.go 纯移动拆分而来：Anthropic APIKey 直通
// （passthrough）转发路径及其流式/非流式响应与 usage 解析。
// 原始业务字节保持不变；计量与审计仅旁路观察。

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/gin-gonic/gin"
)

type anthropicPassthroughForwardInput struct {
	Body          []byte
	Parsed        *ParsedRequest
	RequestModel  string
	OriginalModel string
	RequestStream bool
	StartTime     time.Time
}

func (s *GatewayService) forwardAnthropicAPIKeyPassthrough(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	reqModel string,
	originalModel string,
	reqStream bool,
	startTime time.Time,
) (*ForwardResult, error) {
	return s.forwardAnthropicAPIKeyPassthroughWithInput(ctx, c, account, anthropicPassthroughForwardInput{
		Body:          body,
		RequestModel:  reqModel,
		OriginalModel: originalModel,
		RequestStream: reqStream,
		StartTime:     startTime,
	})
}

func (s *GatewayService) forwardAnthropicAPIKeyPassthroughWithInput(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	input anthropicPassthroughForwardInput,
) (*ForwardResult, error) {
	token, tokenType, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}
	if tokenType != "apikey" {
		return nil, fmt.Errorf("anthropic api key passthrough requires apikey token, got: %s", tokenType)
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	logger.LegacyPrintf("service.gateway", "[Anthropic 自动透传] 命中 API Key 透传分支: account=%d name=%s model=%s stream=%v",
		account.ID, account.Name, input.RequestModel, input.RequestStream)

	if c != nil {
		c.Set("anthropic_passthrough", true)
	}

	upstreamCtx, release := detachStreamUpstreamContext(ctx, input.RequestStream)
	upstreamReq, wireBody, err := s.buildUpstreamRequestAnthropicAPIKeyPassthrough(upstreamCtx, c, account, input.Body, token)
	defer release()
	if err != nil {
		return nil, err
	}
	input.Body = wireBody
	if input.Parsed != nil {
		if err := input.Parsed.ReplaceBody(wireBody); err != nil {
			return nil, err
		}
	}
	resp, err := s.httpUpstream.DoWithTLS(upstreamReq, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return nil, s.handleUpstreamTransportError(ctx, c, account, err, OpsUpstreamErrorEvent{
			UpstreamURL: safeUpstreamURL(upstreamReq.URL.String()), Passthrough: true,
		})
	}
	if resp == nil || resp.Body == nil {
		return nil, errors.New("upstream request failed: empty response")
	}
	defer func() { _ = resp.Body.Close() }()
	// Native errors are part of the API contract. Do not rectify the body,
	// substitute an account, or replace the provider's error document.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		scheduleOllamaCloudUsageActivity(s.deferredService, account)
		body, readErr := ReadUpstreamResponseBody(resp.Body, s.cfg, c, anthropicTooLargeError)
		if readErr != nil {
			return nil, readErr
		}
		if s.rateLimitService != nil && resp.StatusCode >= 400 {
			s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, body, input.RequestModel)
		}
		setOpsUpstreamError(c, resp.StatusCode, sanitizeUpstreamErrorMessage(extractUpstreamErrorMessage(body)), "")
		c.Set(AnthropicPassthroughResponseContextKey, true)
		writeAnthropicPassthroughResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
		c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), body)
		return nil, fmt.Errorf("upstream returned HTTP %d", resp.StatusCode)
	}

	if input.Parsed != nil && input.Parsed.OnUpstreamAccepted != nil {
		input.Parsed.OnUpstreamAccepted()
	}

	var usage *ClaudeUsage
	var firstTokenMs *int
	var clientDisconnect bool
	if input.RequestStream {
		streamResult, err := s.handleStreamingResponseAnthropicAPIKeyPassthrough(ctx, resp, c, account, input.StartTime, input.RequestModel)
		if err != nil {
			// 流中断时保留已观测到的 usage 与错误一起返回，避免上游已计量的请求
			// 完全漏记漏计费（issue #5148）。
			if partial := partialStreamUsageResult(c, resp, streamResult, input.OriginalModel, input.RequestModel, input.StartTime, err); partial != nil {
				return partial, err
			}
			return nil, err
		}
		usage = streamResult.usage
		firstTokenMs = streamResult.firstTokenMs
		clientDisconnect = streamResult.clientDisconnect
	} else {
		usage, err = s.handleNonStreamingResponseAnthropicAPIKeyPassthrough(ctx, resp, c, account)
		if err != nil {
			return nil, err
		}
	}
	if usage == nil {
		usage = &ClaudeUsage{}
	}

	return &ForwardResult{
		RequestID:                     resp.Header.Get("x-request-id"),
		UpstreamHeaders:               resp.Header,
		Usage:                         *usage,
		Model:                         input.OriginalModel,
		UpstreamModel:                 input.RequestModel,
		UpstreamResponseModel:         observedUpstreamResponseModel(c),
		UpstreamResponseModelConflict: observedUpstreamResponseModelConflict(c),
		UpstreamResponseServiceTier:   observedUpstreamResponseServiceTier(c),
		Stream:                        input.RequestStream,
		Duration:                      time.Since(input.StartTime),
		FirstTokenMs:                  firstTokenMs,
		ClientDisconnect:              clientDisconnect,
	}, nil
}

func (s *GatewayService) buildUpstreamRequestAnthropicAPIKeyPassthrough(
	ctx context.Context, c *gin.Context, account *Account, body []byte, token string,
) (*http.Request, []byte, error) {
	req, err := s.buildAnthropicPassthroughRequest(ctx, c, account, http.MethodPost, "/messages", bytes.NewReader(body), token)
	return req, body, err
}

func (s *GatewayService) handleStreamingResponseAnthropicAPIKeyPassthrough(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
	startTime time.Time,
	model string,
) (*streamingResult, error) {
	observer := upstreamResponseModelObserverFromContext(c)
	if observer == nil {
		observer = beginUpstreamResponseModelObservation(c)
	}
	if s.rateLimitService != nil {
		s.rateLimitService.UpdateSessionWindow(ctx, account, resp.Header)
	}

	c.Set(AnthropicPassthroughResponseContextKey, true)
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

	c.Status(resp.StatusCode)
	w := c.Writer
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
	}

	usage := &ClaudeUsage{}
	var firstTokenMs *int
	clientDisconnected := false
	wroteFirstLine := false
	sawTerminalEvent := false

	scanner := bufio.NewScanner(resp.Body)
	scanner.Split(scanAnthropicWireLines)
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

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				if !clientDisconnected {
					// 兜底补刷，确保最后一个未以空行结尾的事件也能及时送达客户端。
					flusher.Flush()
				}
				if !sawTerminalEvent {
					if clientDisconnected && streamInterval > 0 {
						lastRead := time.Unix(0, atomic.LoadInt64(&lastReadAt))
						if time.Since(lastRead) >= streamInterval {
							return &streamingResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: true}, fmt.Errorf("stream usage incomplete after timeout")
						}
					}
					return &streamingResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: clientDisconnected}, fmt.Errorf("stream usage incomplete: missing terminal event")
				}
				return &streamingResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: clientDisconnected}, nil
			}
			if ev.err != nil {
				if sawTerminalEvent {
					return &streamingResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: clientDisconnected}, nil
				}
				if clientDisconnected {
					return &streamingResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: true}, fmt.Errorf("stream usage incomplete after disconnect: %w", ev.err)
				}
				if errors.Is(ev.err, context.Canceled) || errors.Is(ev.err, context.DeadlineExceeded) {
					return &streamingResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: true}, fmt.Errorf("stream usage incomplete: %w", ev.err)
				}
				if errors.Is(ev.err, bufio.ErrTooLong) {
					logger.LegacyPrintf("service.gateway", "[Anthropic passthrough] SSE line too long: account=%d max_size=%d error=%v", account.ID, maxLineSize, ev.err)
					return &streamingResult{usage: usage, firstTokenMs: firstTokenMs}, ev.err
				}
				return &streamingResult{usage: usage, firstTokenMs: firstTokenMs}, fmt.Errorf("stream read error: %w", ev.err)
			}

			line := strings.TrimSuffix(strings.TrimSuffix(ev.line, "\n"), "\r")
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
				if _, err := io.WriteString(w, ev.line); err != nil {
					clientDisconnected = true
					logger.LegacyPrintf("service.gateway", "[Anthropic passthrough] Client disconnected; draining upstream usage: account=%d", account.ID)
				} else if !wroteFirstLine || line == "" || strings.HasPrefix(line, ":") {
					// Flush first bytes, complete events and heartbeat comments, without rewriting SSE.
					flusher.Flush()
					wroteFirstLine = true
				}
			}

		case <-intervalCh:
			lastRead := time.Unix(0, atomic.LoadInt64(&lastReadAt))
			if time.Since(lastRead) < streamInterval {
				continue
			}
			if clientDisconnected {
				return &streamingResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: true}, fmt.Errorf("stream usage incomplete after timeout")
			}
			logger.LegacyPrintf("service.gateway", "[Anthropic passthrough] Stream data interval timeout: account=%d model=%s interval=%s", account.ID, model, streamInterval)
			if s.rateLimitService != nil {
				s.rateLimitService.HandleStreamTimeout(ctx, account, model)
			}
			return &streamingResult{usage: usage, firstTokenMs: firstTokenMs}, fmt.Errorf("stream data interval timeout")

		}
	}
}

func extractAnthropicSSEDataLine(line string) (string, bool) {
	if !strings.HasPrefix(line, "data:") {
		return "", false
	}
	start := len("data:")
	for start < len(line) {
		if line[start] != ' ' && line[start] != '\t' {
			break
		}
		start++
	}
	return line[start:], true
}

// parseSSEUsagePassthrough 从 Anthropic SSE data 行提取 usage（包级函数：
// Anthropic 平台 passthrough 与国产供应商原生 Anthropic 直通共用）。
func parseSSEUsagePassthrough(data string, usage *ClaudeUsage) {
	if usage == nil || data == "" || data == "[DONE]" {
		return
	}

	parsed := gjson.Parse(data)
	switch parsed.Get("type").String() {
	case "message_start":
		msgUsage := parsed.Get("message.usage")
		if msgUsage.Exists() {
			usage.InputTokens = int(msgUsage.Get("input_tokens").Int())
			usage.CacheCreationInputTokens = int(msgUsage.Get("cache_creation_input_tokens").Int())
			usage.CacheReadInputTokens = int(msgUsage.Get("cache_read_input_tokens").Int())

			// 保持与通用解析一致：message_start 允许覆盖 5m/1h 明细（包括 0）。
			cc5m := msgUsage.Get("cache_creation.ephemeral_5m_input_tokens")
			cc1h := msgUsage.Get("cache_creation.ephemeral_1h_input_tokens")
			if cc5m.Exists() || cc1h.Exists() {
				usage.CacheCreation5mTokens = int(cc5m.Int())
				usage.CacheCreation1hTokens = int(cc1h.Int())
			}
		}
	case "message_delta":
		deltaUsage := parsed.Get("usage")
		if deltaUsage.Exists() {
			if v := deltaUsage.Get("input_tokens").Int(); v > 0 {
				usage.InputTokens = int(v)
			}
			if v := deltaUsage.Get("output_tokens").Int(); v > 0 {
				usage.OutputTokens = int(v)
			}
			if v := deltaUsage.Get("cache_creation_input_tokens").Int(); v > 0 {
				usage.CacheCreationInputTokens = int(v)
			}
			if v := deltaUsage.Get("cache_read_input_tokens").Int(); v > 0 {
				usage.CacheReadInputTokens = int(v)
			}

			cc5m := deltaUsage.Get("cache_creation.ephemeral_5m_input_tokens")
			cc1h := deltaUsage.Get("cache_creation.ephemeral_1h_input_tokens")
			if cc5m.Exists() {
				usage.CacheCreation5mTokens = int(cc5m.Int())
			}
			if cc1h.Exists() {
				usage.CacheCreation1hTokens = int(cc1h.Int())
			}
		}
	}

	if usage.CacheReadInputTokens == 0 {
		if cached := parsed.Get("message.usage.cached_tokens").Int(); cached > 0 {
			usage.CacheReadInputTokens = int(cached)
		}
		if cached := parsed.Get("usage.cached_tokens").Int(); usage.CacheReadInputTokens == 0 && cached > 0 {
			usage.CacheReadInputTokens = int(cached)
		}
	}
	if usage.CacheCreationInputTokens == 0 {
		cc5m := parsed.Get("message.usage.cache_creation.ephemeral_5m_input_tokens").Int()
		cc1h := parsed.Get("message.usage.cache_creation.ephemeral_1h_input_tokens").Int()
		if cc5m == 0 && cc1h == 0 {
			cc5m = parsed.Get("usage.cache_creation.ephemeral_5m_input_tokens").Int()
			cc1h = parsed.Get("usage.cache_creation.ephemeral_1h_input_tokens").Int()
		}
		total := cc5m + cc1h
		if total > 0 {
			usage.CacheCreationInputTokens = int(total)
		}
	}

	// Kimi's Anthropic-compatible stream uses input_tokens with two meanings:
	// message_start reports total prompt input, while message_delta reports only
	// uncached input. prompt_tokens remains the total in both events. Normalize
	// to ClaudeUsage's mutually-exclusive buckets so downstream billing does not
	// subtract cache tokens from an already-uncached value.
	usageNode := parsed.Get("usage")
	if parsed.Get("type").String() == "message_start" {
		usageNode = parsed.Get("message.usage")
	}
	normalizeAnthropicCompatiblePromptUsage(usageNode, usage)
}

// normalizeAnthropicCompatiblePromptUsage converts provider-native OpenAI-style
// prompt/cache fields into Claude's mutually-exclusive usage buckets. Native
// Anthropic responses do not expose these aliases and are left alone.
func normalizeAnthropicCompatiblePromptUsage(usageNode gjson.Result, usage *ClaudeUsage) bool {
	if usage == nil || !usageNode.Exists() {
		return false
	}
	promptTokens := usageNode.Get("prompt_tokens")
	promptCacheHitTokens := usageNode.Get("prompt_cache_hit_tokens")
	promptCacheMissTokens := usageNode.Get("prompt_cache_miss_tokens")
	if (!promptTokens.Exists() || promptTokens.Int() <= 0) &&
		!promptCacheHitTokens.Exists() && !promptCacheMissTokens.Exists() {
		return false
	}

	cacheReadTokens := usage.CacheReadInputTokens
	if v := usageNode.Get("cache_read_input_tokens"); v.Exists() {
		cacheReadTokens = int(v.Int())
	}
	if cacheReadTokens == 0 {
		if v := usageNode.Get("cached_tokens"); v.Exists() {
			cacheReadTokens = int(v.Int())
		}
	}
	if cacheReadTokens == 0 {
		if v := usageNode.Get("prompt_tokens_details.cached_tokens"); v.Exists() {
			cacheReadTokens = int(v.Int())
		}
	}
	if cacheReadTokens == 0 && promptCacheHitTokens.Exists() {
		cacheReadTokens = max(int(promptCacheHitTokens.Int()), 0)
	}

	cacheCreationTokens := usage.CacheCreationInputTokens
	if v := usageNode.Get("cache_creation_input_tokens"); v.Exists() {
		cacheCreationTokens = int(v.Int())
	}
	if cacheCreationTokens == 0 {
		cc5m := usageNode.Get("cache_creation.ephemeral_5m_input_tokens").Int()
		cc1h := usageNode.Get("cache_creation.ephemeral_1h_input_tokens").Int()
		if cc5m > 0 || cc1h > 0 {
			cacheCreationTokens = int(cc5m + cc1h)
		}
	}

	if promptCacheMissTokens.Exists() {
		usage.InputTokens = max(int(promptCacheMissTokens.Int()), 0)
	} else {
		usage.InputTokens = max(int(promptTokens.Int())-cacheReadTokens-cacheCreationTokens, 0)
	}
	usage.CacheReadInputTokens = cacheReadTokens
	usage.CacheCreationInputTokens = cacheCreationTokens
	return true
}

func parseClaudeUsageFromResponseBody(body []byte) *ClaudeUsage {
	usage := &ClaudeUsage{}
	if len(body) == 0 {
		return usage
	}

	parsed := gjson.ParseBytes(body)
	usageNode := parsed.Get("usage")
	if !usageNode.Exists() {
		return usage
	}

	usage.InputTokens = int(usageNode.Get("input_tokens").Int())
	usage.OutputTokens = int(usageNode.Get("output_tokens").Int())
	usage.CacheCreationInputTokens = int(usageNode.Get("cache_creation_input_tokens").Int())
	usage.CacheReadInputTokens = int(usageNode.Get("cache_read_input_tokens").Int())

	cc5m := usageNode.Get("cache_creation.ephemeral_5m_input_tokens").Int()
	cc1h := usageNode.Get("cache_creation.ephemeral_1h_input_tokens").Int()
	if cc5m > 0 || cc1h > 0 {
		usage.CacheCreation5mTokens = int(cc5m)
		usage.CacheCreation1hTokens = int(cc1h)
	}
	if usage.CacheCreationInputTokens == 0 && (cc5m > 0 || cc1h > 0) {
		usage.CacheCreationInputTokens = int(cc5m + cc1h)
	}
	if usage.CacheReadInputTokens == 0 {
		if cached := usageNode.Get("cached_tokens").Int(); cached > 0 {
			usage.CacheReadInputTokens = int(cached)
		}
	}
	normalizeAnthropicCompatiblePromptUsage(usageNode, usage)
	return usage
}

// invalidNonStreamingJSONFailoverError 把"上游 2xx 返回非 JSON body"归一为
// failover 错误（包级函数：Anthropic 平台 passthrough 与国产供应商原生
// Anthropic 直通共用）。
func invalidNonStreamingJSONFailoverError(
	ctx context.Context,
	rateLimitService *RateLimitService,
	resp *http.Response,
	account *Account,
	body []byte,
	parseErr error,
	requestedModel ...string,
) error {
	const statusCode = http.StatusBadGateway

	accountID := int64(0)
	accountName := ""
	retryableOnSameAccount := false
	if account != nil {
		accountID = account.ID
		accountName = account.Name
		retryableOnSameAccount = account.IsPoolMode() && account.IsPoolModeRetryableStatus(statusCode)
	}

	logger.LegacyPrintf(
		"service.gateway",
		"Account %d(%s): upstream returned non-JSON 2xx response, attempting failover: status=%d request_id=%s error=%v",
		accountID,
		accountName,
		resp.StatusCode,
		resp.Header.Get("x-request-id"),
		parseErr,
	)

	if rateLimitService != nil && account != nil {
		if len(requestedModel) > 0 {
			rateLimitService.HandleUpstreamError(ctx, account, statusCode, resp.Header, body, requestedModel[0])
		} else {
			rateLimitService.HandleUpstreamError(ctx, account, statusCode, resp.Header, body)
		}
	}

	return &UpstreamFailoverError{
		StatusCode:             statusCode,
		ResponseBody:           body,
		ResponseHeaders:        resp.Header,
		RetryableOnSameAccount: retryableOnSameAccount,
	}
}

func (s *GatewayService) handleNonStreamingResponseAnthropicAPIKeyPassthrough(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
) (*ClaudeUsage, error) {
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

	usage := parseClaudeUsageFromResponseBody(body)

	c.Set(AnthropicPassthroughResponseContextKey, true)
	writeAnthropicPassthroughResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(resp.StatusCode, contentType, body)
	return usage, nil
}

func classifyAnthropicResponseInputAsCacheRead(body []byte, usage *ClaudeUsage) ([]byte, error) {
	classified, err := sjson.SetBytes(body, "usage.input_tokens", 0)
	if err != nil {
		return nil, fmt.Errorf("classify forced cache billing input tokens: %w", err)
	}
	classified, err = sjson.SetBytes(classified, "usage.cache_read_input_tokens", usage.CacheReadInputTokens+usage.InputTokens)
	if err != nil {
		return nil, fmt.Errorf("classify forced cache billing cache read tokens: %w", err)
	}
	return classified, nil
}

func writeAnthropicPassthroughResponseHeaders(dst http.Header, src http.Header, _ *responseheaders.CompiledHeaderFilter) {
	copyAnthropicEndToEndHeaders(dst, src, false)
}
