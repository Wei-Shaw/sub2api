package service

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
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// anthropicAsCCAllowedHeaders 是 Anthropic→CC 转换路径的客户端 header 透传白名单。
//
// 故意不包含 anthropic-* 系列 header（anthropic-version / anthropic-beta / x-api-key
// 等）——上游是 OpenAI 兼容的 Chat Completions 端点，对这些 header 一无所知，
// 透传只会被忽略或触发上游报错。授权头由本路径显式设置 Authorization: Bearer。
var anthropicAsCCAllowedHeaders = map[string]bool{
	"accept-language": true,
	"user-agent":      true,
}

// ForwardAnthropicAsChatCompletions accepts an Anthropic Messages API request
// and forwards it to a Chat-Completions-only upstream by:
//
//  1. Parsing the body as AnthropicRequest
//  2. Converting Anthropic → Responses → Chat Completions (chained via apicompat)
//  3. POSTing the CC body to account.GetOpenAIChatCompletionsURL() with stream=true
//  4. Converting the upstream CC SSE chunks back to Anthropic Messages format:
//     - stream: per-chunk CC→Responses→Anthropic SSE conversion
//     - non-stream: buffer SSE → assemble Responses → AnthropicResponse JSON
//
// Adapter for `apikey-chat-completions` accounts hit through the /v1/messages
// entry on Anthropic-platform groups. Mirrors OpenAIGatewayService.ForwardResponsesAsChatCompletions
// but with Anthropic as the client-facing protocol.
func (s *GatewayService) ForwardAnthropicAsChatCompletions(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	parsed *ParsedRequest,
) (*ForwardResult, error) {
	startTime := time.Now()
	if parsed == nil {
		return nil, fmt.Errorf("parse request: empty parsed request")
	}
	body := parsed.Body.Bytes()

	// 1. Parse Anthropic request
	var anthropicReq apicompat.AnthropicRequest
	if err := json.Unmarshal(body, &anthropicReq); err != nil {
		writeAnthropicCCError(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return nil, fmt.Errorf("parse anthropic request: %w", err)
	}
	originalModel := strings.TrimSpace(anthropicReq.Model)
	if originalModel == "" {
		writeAnthropicCCError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return nil, fmt.Errorf("model is required")
	}
	clientStream := anthropicReq.Stream

	// 2. Model mapping (apikey-chat-completions: 由用户自定义上游模型 ID, 不做 Anthropic 规范化)
	mappedModel := account.GetMappedModel(originalModel)
	if mappedModel != originalModel {
		anthropicReq.Model = mappedModel
	}

	// 3. Chain conversion: Anthropic → Responses → CC
	responsesReq, err := apicompat.AnthropicToResponses(&anthropicReq)
	if err != nil {
		writeAnthropicCCError(c, http.StatusBadRequest, "invalid_request_error", "Failed to convert request: "+err.Error())
		return nil, fmt.Errorf("convert anthropic to responses: %w", err)
	}
	ccReq, err := apicompat.ResponsesToChatCompletionsRequest(responsesReq)
	if err != nil {
		writeAnthropicCCError(c, http.StatusBadRequest, "invalid_request_error", "Failed to convert request: "+err.Error())
		return nil, fmt.Errorf("convert responses to chat completions: %w", err)
	}
	// 4. Force upstream stream so we can reuse the CC-SSE chain regardless of client.
	ccReq.Stream = true
	ccReq.Model = mappedModel
	ccBody, err := json.Marshal(ccReq)
	if err != nil {
		return nil, fmt.Errorf("marshal chat completions request: %w", err)
	}
	ccBody, err = ensureOpenAIChatStreamUsage(ccBody)
	if err != nil {
		return nil, fmt.Errorf("enable stream usage: %w", err)
	}

	// 5. Upstream URL + API key
	apiKey := account.GetUpstreamAPIKey()
	if apiKey == "" {
		writeAnthropicCCError(c, http.StatusBadGateway, "api_error", fmt.Sprintf("account %d missing api_key", account.ID))
		return nil, fmt.Errorf("account %d missing api_key", account.ID)
	}
	targetURL := account.GetOpenAIChatCompletionsURL()
	if targetURL == "" {
		writeAnthropicCCError(c, http.StatusBadGateway, "api_error", fmt.Sprintf("account %d missing chat_completions_url", account.ID))
		return nil, fmt.Errorf("account %d missing chat_completions_url", account.ID)
	}
	if _, err := s.validateUpstreamBaseURL(targetURL); err != nil {
		writeAnthropicCCError(c, http.StatusBadGateway, "api_error", "Invalid upstream URL")
		return nil, fmt.Errorf("invalid chat_completions_url: %w", err)
	}

	logger.L().Debug("gateway anthropic→cc: forwarding",
		zap.Int64("account_id", account.ID),
		zap.String("original_model", originalModel),
		zap.String("upstream_model", mappedModel),
		zap.String("upstream_url", targetURL),
		zap.Bool("client_stream", clientStream),
	)

	// 6. Build upstream request
	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	upstreamReq, err := http.NewRequestWithContext(upstreamCtx, http.MethodPost, targetURL, bytes.NewReader(ccBody))
	releaseUpstreamCtx()
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Accept", "text/event-stream")
	applyOpenAICompatibleAPIKeyAuth(upstreamReq, account, apiKey)
	if c != nil && c.Request != nil {
		for key, values := range c.Request.Header {
			if anthropicAsCCAllowedHeaders[strings.ToLower(key)] {
				for _, v := range values {
					upstreamReq.Header.Add(key, v)
				}
			}
		}
	}

	// 7. Send
	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: 0,
			Kind:               "request_error",
			Message:            safeErr,
		})
		writeAnthropicCCError(c, http.StatusBadGateway, "api_error", "Upstream request failed")
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
	}
	defer func() { _ = resp.Body.Close() }()

	// 8. Error handling — translate to Anthropic error envelope
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))

		upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
		upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
		if upstreamMsg == "" {
			upstreamMsg = http.StatusText(resp.StatusCode)
		}

		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  resp.Header.Get("x-request-id"),
			Kind:               "upstream_error",
			Message:            upstreamMsg,
		})
		if s.rateLimitService != nil {
			s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
		}
		// 可重试 / 可切账号的状态码（401/403/429/529/5xx）走 failover loop —— 不在此处写客户端响应，
		// 让 handler 端根据 stream-written guard 决定是否切换到下一个账号。
		if s.shouldFailoverUpstreamError(resp.StatusCode) {
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				ResponseHeaders:        resp.Header,
				RetryableOnSameAccount: account.IsPoolMode() && isPoolModeRetryableStatus(resp.StatusCode),
			}
		}
		// 不可 failover 的错误（如 400 / 404 / 422）：原样写 Anthropic 错误信封并返回普通错误
		writeAnthropicCCError(c, mapUpstreamStatusCode(resp.StatusCode), anthropicErrorTypeForStatus(resp.StatusCode), upstreamMsg)
		return nil, fmt.Errorf("upstream error: %d %s", resp.StatusCode, upstreamMsg)
	}

	// 上游已接受请求（2xx），提前释放用户串行锁——不等流完成
	if parsed.OnUpstreamAccepted != nil {
		parsed.OnUpstreamAccepted()
	}

	// 9. Forward response
	if clientStream {
		return s.streamAnthropicFromCC(c, resp, originalModel, mappedModel, startTime)
	}
	return s.bufferAnthropicFromCC(c, resp, originalModel, mappedModel, startTime)
}

// streamAnthropicFromCC reads upstream Chat Completions SSE chunks line-by-line,
// converts each through CC→Responses→Anthropic, and writes Anthropic SSE to client.
func (s *GatewayService) streamAnthropicFromCC(
	c *gin.Context,
	resp *http.Response,
	originalModel string,
	upstreamModel string,
	startTime time.Time,
) (*ForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	ccState := apicompat.NewCCStreamState()
	ccState.Model = originalModel
	anthState := apicompat.NewResponsesEventToAnthropicState()

	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	var firstTokenMs *int
	var usage ClaudeUsage
	clientDisconnected := false

	writeAnthropicEvent := func(evt apicompat.AnthropicStreamEvent) bool {
		if clientDisconnected {
			return true
		}
		sse, err := apicompat.ResponsesAnthropicEventToSSE(evt)
		if err != nil {
			return false
		}
		if _, err := c.Writer.WriteString(sse); err != nil {
			clientDisconnected = true
			logger.L().Debug("gateway anthropic→cc stream: client disconnected",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
			return true
		}
		c.Writer.Flush()
		return false
	}

	processResponsesEvents := func(events [][]byte) bool {
		for _, frame := range events {
			// Each frame is "event: <type>\ndata: <json>\n\n" or "data: [DONE]\n\n".
			payloads := extractResponsesSSEDataPayloads(frame)
			for _, payload := range payloads {
				if bytes.Equal(bytes.TrimSpace(payload), []byte("[DONE]")) {
					continue
				}
				var resEvt apicompat.ResponsesStreamEvent
				if err := json.Unmarshal(payload, &resEvt); err != nil {
					continue
				}
				anthEvents := apicompat.ResponsesEventToAnthropicEvents(&resEvt, anthState)
				for _, anthEvt := range anthEvents {
					if firstTokenMs == nil {
						ms := int(time.Since(startTime).Milliseconds())
						firstTokenMs = &ms
					}
					mergeAnthropicUsageFromEvent(&usage, anthEvt)
					if disconnected := writeAnthropicEvent(anthEvt); disconnected {
						return true
					}
				}
			}
		}
		return false
	}

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		events, err := apicompat.ConvertChatCompletionsSSEChunkToResponsesEvents(line, ccState)
		if err != nil {
			logger.L().Warn("gateway anthropic→cc stream: failed to convert chunk",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
		}
		if processResponsesEvents(events) {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			logger.L().Warn("gateway anthropic→cc stream: read error",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
		}
	}

	// Drain CC final events if upstream did not send [DONE].
	if !ccState.CompletedSent {
		processResponsesEvents(apicompat.FinalizeCCStream(ccState))
	}
	// Flush trailing Anthropic events from the Responses→Anthropic state machine.
	for _, evt := range apicompat.FinalizeResponsesAnthropicStream(anthState) {
		mergeAnthropicUsageFromEvent(&usage, evt)
		if writeAnthropicEvent(evt) {
			break
		}
	}

	// CC final usage may only be present in the closing chunk; pull from CC state if event-derived
	// usage is empty.
	if usage.InputTokens == 0 && usage.OutputTokens == 0 && ccState.Usage != nil {
		usage.InputTokens = ccState.Usage.InputTokens
		usage.OutputTokens = ccState.Usage.OutputTokens
		if ccState.Usage.InputTokensDetails != nil {
			usage.CacheReadInputTokens = ccState.Usage.InputTokensDetails.CachedTokens
		}
	}

	return &ForwardResult{
		RequestID:        requestID,
		Usage:            usage,
		Model:            originalModel,
		UpstreamModel:    upstreamModel,
		Stream:           true,
		Duration:         time.Since(startTime),
		FirstTokenMs:     firstTokenMs,
		ClientDisconnect: clientDisconnected,
	}, nil
}

// bufferAnthropicFromCC reads upstream CC SSE (we forced stream=true), assembles
// a complete ChatCompletionsResponse from the chunks, then converts to AnthropicResponse JSON.
func (s *GatewayService) bufferAnthropicFromCC(
	c *gin.Context,
	resp *http.Response,
	originalModel string,
	upstreamModel string,
	startTime time.Time,
) (*ForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")

	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	ccResp := apicompat.ChatCompletionsResponse{
		Object: "chat.completion",
		Model:  originalModel,
	}
	var contentBuf strings.Builder
	var reasoningBuf strings.Builder
	finishReason := ""
	type toolCallAcc struct {
		ID        string
		Name      string
		Arguments strings.Builder
	}
	toolCalls := map[int]*toolCallAcc{}
	var toolCallOrder []int

	for scanner.Scan() {
		line := bytes.TrimRight(scanner.Bytes(), "\r\n")
		if len(line) == 0 {
			continue
		}
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(line[len("data:"):])
		if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
			continue
		}
		var chk apicompat.ChatCompletionsChunk
		if err := json.Unmarshal(payload, &chk); err != nil {
			continue
		}
		if ccResp.ID == "" && chk.ID != "" {
			ccResp.ID = chk.ID
		}
		if chk.Model != "" {
			ccResp.Model = chk.Model
		}
		if chk.Usage != nil {
			ccResp.Usage = chk.Usage
		}
		for _, ch := range chk.Choices {
			if ch.Delta.Content != nil {
				contentBuf.WriteString(*ch.Delta.Content)
			}
			if ch.Delta.ReasoningContent != nil {
				reasoningBuf.WriteString(*ch.Delta.ReasoningContent)
			}
			for _, tc := range ch.Delta.ToolCalls {
				idx := 0
				if tc.Index != nil {
					idx = *tc.Index
				}
				st, ok := toolCalls[idx]
				if !ok {
					st = &toolCallAcc{}
					toolCalls[idx] = st
					toolCallOrder = append(toolCallOrder, idx)
				}
				if tc.ID != "" {
					st.ID = tc.ID
				}
				if tc.Function.Name != "" {
					st.Name = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					st.Arguments.WriteString(tc.Function.Arguments)
				}
			}
			if ch.FinishReason != nil && *ch.FinishReason != "" {
				finishReason = *ch.FinishReason
			}
		}
	}
	if err := scanner.Err(); err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			logger.L().Warn("gateway anthropic→cc buffered: read error",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
		}
	}

	msg := apicompat.ChatMessage{Role: "assistant"}
	contentText := contentBuf.String()
	if contentText != "" {
		raw, _ := json.Marshal(contentText)
		msg.Content = raw
	} else {
		msg.Content = json.RawMessage(`""`)
	}
	if reasoningBuf.Len() > 0 {
		msg.ReasoningContent = reasoningBuf.String()
	}
	for _, idx := range toolCallOrder {
		st := toolCalls[idx]
		args := st.Arguments.String()
		if args == "" {
			args = "{}"
		}
		msg.ToolCalls = append(msg.ToolCalls, apicompat.ChatToolCall{
			ID:   st.ID,
			Type: "function",
			Function: apicompat.ChatFunctionCall{
				Name:      st.Name,
				Arguments: args,
			},
		})
	}
	if finishReason == "" {
		if len(msg.ToolCalls) > 0 {
			finishReason = "tool_calls"
		} else {
			finishReason = "stop"
		}
	}
	ccResp.Choices = []apicompat.ChatChoice{{Index: 0, Message: msg, FinishReason: finishReason}}

	responsesResp := apicompat.ChatCompletionsToResponsesResponse(&ccResp, originalModel)
	anthResp := apicompat.ResponsesToAnthropic(responsesResp, originalModel)

	usage := ClaudeUsage{
		InputTokens:          anthResp.Usage.InputTokens,
		OutputTokens:         anthResp.Usage.OutputTokens,
		CacheReadInputTokens: anthResp.Usage.CacheReadInputTokens,
	}

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}
	out, err := json.Marshal(anthResp)
	if err != nil {
		writeAnthropicCCError(c, http.StatusBadGateway, "api_error", "Failed to marshal response")
		return nil, fmt.Errorf("marshal anthropic response: %w", err)
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", out)

	return &ForwardResult{
		RequestID:     requestID,
		Usage:         usage,
		Model:         originalModel,
		UpstreamModel: upstreamModel,
		Stream:        false,
		Duration:      time.Since(startTime),
	}, nil
}

// mergeAnthropicUsageFromEvent inspects an Anthropic stream event and merges any
// usage information found into the running ClaudeUsage tally.
func mergeAnthropicUsageFromEvent(target *ClaudeUsage, evt apicompat.AnthropicStreamEvent) {
	if evt.Type == "message_start" && evt.Message != nil {
		mergeAnthropicUsage(target, evt.Message.Usage)
	}
	if evt.Type == "message_delta" && evt.Usage != nil {
		mergeAnthropicUsage(target, *evt.Usage)
	}
}

// writeAnthropicCCError writes an Anthropic-style error envelope. Used by the
// Anthropic→CC forwarding path so Claude SDK clients can parse failures.
func writeAnthropicCCError(c *gin.Context, statusCode int, errType, message string) {
	if c == nil {
		return
	}
	c.JSON(statusCode, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
}

// anthropicErrorTypeForStatus maps an upstream HTTP status code to an Anthropic
// error envelope `error.type` string.
func anthropicErrorTypeForStatus(status int) string {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return "authentication_error"
	case status == http.StatusTooManyRequests:
		return "rate_limit_error"
	case status >= 400 && status < 500:
		return "invalid_request_error"
	default:
		return "api_error"
	}
}
