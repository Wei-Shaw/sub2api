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
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// forwardMessagesViaOpenAICompat 让 Anthropic Messages 协议（POST /v1/messages）的客户端
// 访问「OpenAI 兼容」上游（自定义平台、或 Gemini upstream_mode=openai_chat_completions）。
//
// 入站 Anthropic 请求 → OpenAI Chat Completions（apicompat: Anthropic→Responses→CC），
// 转发到 {base_url}/chat/completions，再把上游 OpenAI 响应 → Anthropic Messages 响应
// （非流式直接构造；流式经 apicompat CC chunk→Responses→Anthropic 事件链转换为 Anthropic SSE）。
func (s *GeminiMessagesCompatService) forwardMessagesViaOpenAICompat(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	originalModel string,
	stream bool,
	startTime time.Time,
) (*ForwardResult, error) {
	apiKey := strings.TrimSpace(account.GetCredential("api_key"))
	if apiKey == "" {
		return nil, s.writeClaudeError(c, http.StatusBadGateway, "api_error", "api_key not configured")
	}
	baseURL := strings.TrimSpace(account.GetCredential("base_url"))
	if baseURL == "" {
		return nil, s.writeClaudeError(c, http.StatusBadGateway, "api_error", "base_url is required for OpenAI-compatible upstream")
	}
	normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, s.writeClaudeError(c, http.StatusBadGateway, "api_error", "invalid base_url: "+err.Error())
	}

	upstreamModel := account.GetMappedModel(originalModel)
	if upstreamModel == "" {
		upstreamModel = originalModel
	}

	ccBody, err := anthropicMessagesToChatCompletions(body, upstreamModel, stream)
	if err != nil {
		return nil, s.writeClaudeError(c, http.StatusBadRequest, "invalid_request_error", "Failed to convert request: "+err.Error())
	}

	targetURL := buildOpenAIChatCompletionsURL(normalizedBaseURL)
	upstreamCtx, release := detachUpstreamContext(ctx)
	upstreamReq, err := http.NewRequestWithContext(upstreamCtx, http.MethodPost, targetURL, bytes.NewReader(ccBody))
	release()
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}
	upstreamReq = upstreamReq.WithContext(WithHTTPUpstreamProfile(upstreamReq.Context(), HTTPUpstreamProfileOpenAI))
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Authorization", "Bearer "+apiKey)
	if stream {
		upstreamReq.Header.Set("Accept", "text/event-stream")
	} else {
		upstreamReq.Header.Set("Accept", "application/json")
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:    account.Platform,
			AccountID:   account.ID,
			AccountName: account.Name,
			Kind:        "request_error",
			Message:     safeErr,
		})
		return nil, s.writeClaudeError(c, http.StatusBadGateway, "api_error", "Upstream request failed")
	}
	defer func() { _ = resp.Body.Close() }()

	requestID := rawOpenAIChatRequestID(resp.Header)
	if requestID != "" {
		c.Header("x-request-id", requestID)
	}

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		s.handleGeminiUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
		if s.shouldFailoverGeminiUpstreamError(resp.StatusCode) {
			upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  requestID,
				Kind:               "failover",
				Message:            upstreamMsg,
			})
			return nil, &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: respBody}
		}
		msg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
		if msg == "" {
			msg = "upstream error"
		}
		return nil, s.writeClaudeError(c, resp.StatusCode, "api_error", msg)
	}

	if stream {
		return s.streamMessagesViaOpenAICompat(c, resp, originalModel, upstreamModel, requestID, body, startTime)
	}
	return s.bufferMessagesViaOpenAICompat(c, resp, originalModel, upstreamModel, requestID, startTime)
}

func (s *GeminiMessagesCompatService) bufferMessagesViaOpenAICompat(
	c *gin.Context,
	resp *http.Response,
	originalModel string,
	upstreamModel string,
	requestID string,
	startTime time.Time,
) (*ForwardResult, error) {
	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		if !errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
			return nil, s.writeClaudeError(c, http.StatusBadGateway, "api_error", "Failed to read upstream response")
		}
		return nil, err
	}

	anthBody, usage, err := chatCompletionToAnthropicMessage(respBody, originalModel)
	if err != nil {
		return nil, s.writeClaudeError(c, http.StatusBadGateway, "api_error", "Failed to convert upstream response: "+err.Error())
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(http.StatusOK)
	_, _ = c.Writer.Write(anthBody)

	return &ForwardResult{
		RequestID:     requestID,
		Usage:         *usage,
		Model:         originalModel,
		UpstreamModel: upstreamModel,
		Stream:        false,
		Duration:      time.Since(startTime),
	}, nil
}

func (s *GeminiMessagesCompatService) streamMessagesViaOpenAICompat(
	c *gin.Context,
	resp *http.Response,
	originalModel string,
	upstreamModel string,
	requestID string,
	reqBody []byte,
	startTime time.Time,
) (*ForwardResult, error) {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
	}

	ccToRes := apicompat.NewChatCompletionsToResponsesStreamState(originalModel)
	resToAnth := apicompat.NewResponsesEventToAnthropicState()
	resToAnth.Model = originalModel

	usage := &ClaudeUsage{}
	var firstTokenMs *int
	clientDisconnected := false
	var outputText strings.Builder

	writeEvt := func(evt apicompat.AnthropicStreamEvent) {
		if clientDisconnected {
			return
		}
		sse, err := apicompat.ResponsesAnthropicEventToSSE(evt)
		if err != nil {
			return
		}
		if _, werr := io.WriteString(c.Writer, sse); werr != nil {
			clientDisconnected = true
			logger.LegacyPrintf("service.gemini_messages_openai_compat", "client disconnected: %v", werr)
			return
		}
		flusher.Flush()
	}

	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	for scanner.Scan() {
		line := scanner.Text()
		payload, ok := extractOpenAISSEDataLine(line)
		if !ok {
			continue
		}
		trimmed := strings.TrimSpace(payload)
		if trimmed == "" || trimmed == "[DONE]" {
			continue
		}
		if u := extractCCStreamUsage(trimmed); u != nil {
			usage = openAIUsageToClaudeUsage(*u)
		}
		if d := gjson.Get(trimmed, "choices.0.delta.content").String(); d != "" {
			_, _ = outputText.WriteString(d)
			if firstTokenMs == nil {
				elapsed := int(time.Since(startTime).Milliseconds())
				firstTokenMs = &elapsed
			}
		}
		var chunk apicompat.ChatCompletionsChunk
		if err := json.Unmarshal([]byte(trimmed), &chunk); err != nil {
			continue
		}
		for _, resEvt := range apicompat.ChatCompletionsChunkToResponsesEvents(&chunk, ccToRes) {
			for _, anthEvt := range apicompat.ResponsesEventToAnthropicEvents(&resEvt, resToAnth) {
				writeEvt(anthEvt)
			}
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		logger.LegacyPrintf("service.gemini_messages_openai_compat", "stream read error: %v", err)
	}

	// 补齐终止事件（message_delta + message_stop）
	for _, anthEvt := range apicompat.FinalizeResponsesAnthropicStream(resToAnth) {
		writeEvt(anthEvt)
	}

	// 上游未返回 usage 时按累计输出文本与入站请求估算，避免流式计费为 0。
	if usage.OutputTokens == 0 && outputText.Len() > 0 {
		usage.OutputTokens = estimateTokensForText(outputText.String())
	}
	if usage.InputTokens == 0 {
		usage.InputTokens = estimateAnthropicInputTokens(reqBody)
	}

	return &ForwardResult{
		RequestID:        requestID,
		Usage:            *usage,
		Model:            originalModel,
		UpstreamModel:    upstreamModel,
		Stream:           true,
		Duration:         time.Since(startTime),
		FirstTokenMs:     firstTokenMs,
		ClientDisconnect: clientDisconnected,
	}, nil
}

// ───────────────────────── pure converters ─────────────────────────

// anthropicMessagesToChatCompletions 把 Anthropic Messages 请求体转换为 OpenAI Chat Completions 请求体。
func anthropicMessagesToChatCompletions(body []byte, model string, stream bool) ([]byte, error) {
	var anthReq apicompat.AnthropicRequest
	if err := json.Unmarshal(body, &anthReq); err != nil {
		return nil, err
	}
	resReq, err := apicompat.AnthropicToResponses(&anthReq)
	if err != nil {
		return nil, err
	}
	ccReq, err := apicompat.ResponsesToChatCompletionsRequest(resReq)
	if err != nil {
		return nil, err
	}
	ccReq.Model = model
	ccReq.Stream = stream
	if stream {
		ccReq.StreamOptions = &apicompat.ChatStreamOptions{IncludeUsage: true}
	}
	return json.Marshal(ccReq)
}

// chatCompletionToAnthropicMessage 把上游 OpenAI 非流式响应转换为 Anthropic Messages 响应。
func chatCompletionToAnthropicMessage(ccBody []byte, model string) ([]byte, *ClaudeUsage, error) {
	root := gjson.ParseBytes(ccBody)
	choice := root.Get("choices.0")

	content := make([]map[string]any, 0, 2)
	if text := choice.Get("message.content").String(); text != "" {
		content = append(content, map[string]any{"type": "text", "text": text})
	}
	choice.Get("message.tool_calls").ForEach(func(_, tc gjson.Result) bool {
		var input any = map[string]any{}
		if args := tc.Get("function.arguments").String(); strings.TrimSpace(args) != "" {
			_ = json.Unmarshal([]byte(args), &input)
		}
		id := tc.Get("id").String()
		if id == "" {
			id = "toolu_" + randomHex(12)
		}
		content = append(content, map[string]any{
			"type":  "tool_use",
			"id":    id,
			"name":  tc.Get("function.name").String(),
			"input": input,
		})
		return true
	})
	if len(content) == 0 {
		content = append(content, map[string]any{"type": "text", "text": ""})
	}

	usage := &ClaudeUsage{}
	if u, ok := openAIUsageFromGJSON(root.Get("usage")); ok {
		usage = openAIUsageToClaudeUsage(u)
	}

	resp := map[string]any{
		"id":            "msg_" + randomHex(12),
		"type":          "message",
		"role":          "assistant",
		"model":         model,
		"content":       content,
		"stop_reason":   mapOpenAIFinishReasonToAnthropic(choice.Get("finish_reason").String()),
		"stop_sequence": nil,
		"usage": map[string]any{
			"input_tokens":            usage.InputTokens,
			"output_tokens":           usage.OutputTokens,
			"cache_read_input_tokens": usage.CacheReadInputTokens,
		},
	}
	out, err := json.Marshal(resp)
	return out, usage, err
}

func mapOpenAIFinishReasonToAnthropic(reason string) string {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "tool_calls", "function_call":
		return "tool_use"
	case "length":
		return "max_tokens"
	case "stop", "":
		return "end_turn"
	default:
		return "end_turn"
	}
}

// estimateAnthropicInputTokens 基于 system 与 messages 的文本粗略估算输入 token（约 4 字符 ≈ 1 token）。
func estimateAnthropicInputTokens(body []byte) int {
	total := 0
	root := gjson.ParseBytes(body)

	sys := root.Get("system")
	if sys.Type == gjson.String {
		total += estimateTokensForText(sys.String())
	} else {
		sys.ForEach(func(_, p gjson.Result) bool {
			total += estimateTokensForText(p.Get("text").String())
			return true
		})
	}

	root.Get("messages").ForEach(func(_, m gjson.Result) bool {
		content := m.Get("content")
		if content.Type == gjson.String {
			total += estimateTokensForText(content.String())
		} else {
			content.ForEach(func(_, p gjson.Result) bool {
				total += estimateTokensForText(p.Get("text").String())
				return true
			})
		}
		return true
	})
	return total
}
