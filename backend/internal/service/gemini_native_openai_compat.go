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

// forwardNativeViaOpenAICompat 让 Gemini 原生协议（/v1beta/models/{model}:generateContent）
// 的客户端也能访问「OpenAI 兼容」上游（自定义平台、或 Gemini 平台 upstream_mode=openai_chat_completions）。
//
// 由于上游只接受 OpenAI Chat Completions 协议，这里做双向转换：
//  1. 入站 Gemini generateContent 请求体 → OpenAI Chat Completions 请求体
//  2. 转发到 {base_url}/chat/completions
//  3. 上游 OpenAI 响应 → Gemini generateContent 响应（流式 / 非流式）
//
// 注意：countTokens 动作上游无对应接口，返回基于输入长度的粗略估算。
func (s *GeminiMessagesCompatService) forwardNativeViaOpenAICompat(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	originalModel string,
	action string,
	stream bool,
	body []byte,
	startTime time.Time,
) (*ForwardResult, error) {
	if action == "countTokens" {
		return s.writeGeminiCountTokensEstimate(c, body, startTime)
	}

	apiKey := strings.TrimSpace(account.GetCredential("api_key"))
	if apiKey == "" {
		return nil, s.writeGoogleError(c, http.StatusBadGateway, "api_key not configured")
	}
	baseURL := strings.TrimSpace(account.GetCredential("base_url"))
	if baseURL == "" {
		return nil, s.writeGoogleError(c, http.StatusBadGateway, "base_url is required for OpenAI-compatible upstream")
	}
	normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, s.writeGoogleError(c, http.StatusBadGateway, "invalid base_url: "+err.Error())
	}

	upstreamModel := account.GetMappedModel(originalModel)
	if upstreamModel == "" {
		upstreamModel = originalModel
	}

	ccBody, err := geminiNativeRequestToChatCompletions(body, upstreamModel, stream)
	if err != nil {
		return nil, s.writeGoogleError(c, http.StatusBadRequest, "Failed to convert Gemini request: "+err.Error())
	}

	targetURL := buildOpenAIChatCompletionsURL(normalizedBaseURL)
	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	upstreamReq, err := http.NewRequestWithContext(upstreamCtx, http.MethodPost, targetURL, bytes.NewReader(ccBody))
	releaseUpstreamCtx()
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
		return nil, s.writeGoogleError(c, http.StatusBadGateway, "Upstream request failed")
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
		return nil, s.writeGoogleError(c, resp.StatusCode, msg)
	}

	if stream {
		return s.streamNativeViaOpenAICompat(c, resp, originalModel, upstreamModel, body, startTime)
	}
	return s.bufferNativeViaOpenAICompat(c, resp, originalModel, upstreamModel, startTime)
}

func (s *GeminiMessagesCompatService) bufferNativeViaOpenAICompat(
	c *gin.Context,
	resp *http.Response,
	originalModel string,
	upstreamModel string,
	startTime time.Time,
) (*ForwardResult, error) {
	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		if !errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
			return nil, s.writeGoogleError(c, http.StatusBadGateway, "Failed to read upstream response")
		}
		return nil, err
	}

	geminiBody, usage, err := chatCompletionToGeminiNativeResponse(respBody, originalModel)
	if err != nil {
		return nil, s.writeGoogleError(c, http.StatusBadGateway, "Failed to convert upstream response: "+err.Error())
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(http.StatusOK)
	_, _ = c.Writer.Write(geminiBody)

	return &ForwardResult{
		Usage:         *usage,
		Model:         originalModel,
		UpstreamModel: upstreamModel,
		Stream:        false,
		Duration:      time.Since(startTime),
	}, nil
}

func (s *GeminiMessagesCompatService) streamNativeViaOpenAICompat(
	c *gin.Context,
	resp *http.Response,
	originalModel string,
	upstreamModel string,
	reqBody []byte,
	startTime time.Time,
) (*ForwardResult, error) {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	usage := &ClaudeUsage{}
	var firstTokenMs *int
	clientDisconnected := false
	var finishReason string
	var outputText strings.Builder

	writeGeminiChunk := func(text string, finish string, u *ClaudeUsage) {
		if clientDisconnected {
			return
		}
		chunk := buildGeminiStreamChunk(text, finish, u)
		if _, err := c.Writer.WriteString("data: " + chunk + "\n\n"); err != nil {
			clientDisconnected = true
			logger.LegacyPrintf("service.gemini_native_openai_compat", "client disconnected: %v", err)
			return
		}
		c.Writer.Flush()
	}

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
		if isOpenAIChatUsageOnlyStreamChunk(trimmed) {
			continue
		}
		delta := gjson.Get(trimmed, "choices.0.delta.content").String()
		if fr := gjson.Get(trimmed, "choices.0.finish_reason"); fr.Exists() && fr.String() != "" {
			finishReason = mapOpenAIFinishReasonToGemini(fr.String())
		}
		if delta == "" {
			continue
		}
		if firstTokenMs == nil {
			elapsed := int(time.Since(startTime).Milliseconds())
			firstTokenMs = &elapsed
		}
		_, _ = outputText.WriteString(delta)
		writeGeminiChunk(delta, "", nil)
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		logger.LegacyPrintf("service.gemini_native_openai_compat", "stream read error: %v", err)
	}

	// 部分 OpenAI 兼容上游在流式响应中不返回 usage（即使设置了 stream_options.include_usage），
	// 导致计费为 0。此时基于已累计的输出文本与入站请求体估算 token，保证流式输出仍被计费。
	if usage.OutputTokens == 0 && outputText.Len() > 0 {
		usage.OutputTokens = estimateTokensForText(outputText.String())
	}
	if usage.InputTokens == 0 {
		usage.InputTokens = estimateGeminiCountTokens(reqBody)
	}

	// 终结块：携带 finishReason 与 usageMetadata（Gemini 客户端依赖最后一块的元数据）
	if finishReason == "" {
		finishReason = "STOP"
	}
	writeGeminiChunk("", finishReason, usage)

	return &ForwardResult{
		Usage:            *usage,
		Model:            originalModel,
		UpstreamModel:    upstreamModel,
		Stream:           true,
		Duration:         time.Since(startTime),
		FirstTokenMs:     firstTokenMs,
		ClientDisconnect: clientDisconnected,
	}, nil
}

// writeGeminiCountTokensEstimate 为 countTokens 动作返回基于字符数的粗略估算
// （OpenAI 兼容上游无 countTokens 接口）。约 4 字符 ≈ 1 token。
func (s *GeminiMessagesCompatService) writeGeminiCountTokensEstimate(c *gin.Context, body []byte, startTime time.Time) (*ForwardResult, error) {
	chars := 0
	gjson.GetBytes(body, "contents").ForEach(func(_, content gjson.Result) bool {
		content.Get("parts").ForEach(func(_, part gjson.Result) bool {
			chars += len(part.Get("text").String())
			return true
		})
		return true
	})
	gjson.GetBytes(body, "systemInstruction.parts").ForEach(func(_, part gjson.Result) bool {
		chars += len(part.Get("text").String())
		return true
	})
	estimate := chars / 4
	if estimate == 0 && chars > 0 {
		estimate = 1
	}
	resp := map[string]any{"totalTokens": estimate}
	out, _ := json.Marshal(resp)
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(http.StatusOK)
	_, _ = c.Writer.Write(out)
	return &ForwardResult{Stream: false, Duration: time.Since(startTime)}, nil
}

// ───────────────────────── pure converters ─────────────────────────

// geminiNativeRequestToChatCompletions 把 Gemini generateContent 请求体转换为
// OpenAI Chat Completions 请求体。
func geminiNativeRequestToChatCompletions(body []byte, model string, stream bool) ([]byte, error) {
	root := gjson.ParseBytes(body)
	ccReq := apicompat.ChatCompletionsRequest{
		Model:  model,
		Stream: stream,
	}
	if stream {
		ccReq.StreamOptions = &apicompat.ChatStreamOptions{IncludeUsage: true}
	}

	// systemInstruction → system message
	if sysText := joinGeminiPartsText(root.Get("systemInstruction.parts")); sysText != "" {
		ccReq.Messages = append(ccReq.Messages, apicompat.ChatMessage{
			Role:    "system",
			Content: jsonString(sysText),
		})
	}

	// contents[] → messages[]
	root.Get("contents").ForEach(func(_, content gjson.Result) bool {
		msg, ok := geminiContentToChatMessage(content)
		if ok {
			ccReq.Messages = append(ccReq.Messages, msg...)
		}
		return true
	})

	// generationConfig
	gc := root.Get("generationConfig")
	if gc.Exists() {
		if v := gc.Get("temperature"); v.Exists() {
			t := v.Float()
			ccReq.Temperature = &t
		}
		if v := gc.Get("topP"); v.Exists() {
			t := v.Float()
			ccReq.TopP = &t
		}
		if v := gc.Get("maxOutputTokens"); v.Exists() {
			t := int(v.Int())
			ccReq.MaxTokens = &t
		}
		if v := gc.Get("stopSequences"); v.Exists() && v.IsArray() {
			stops := make([]string, 0)
			v.ForEach(func(_, s gjson.Result) bool {
				stops = append(stops, s.String())
				return true
			})
			if len(stops) > 0 {
				if raw, err := json.Marshal(stops); err == nil {
					ccReq.Stop = raw
				}
			}
		}
	}

	// tools[].functionDeclarations → tools[]
	root.Get("tools").ForEach(func(_, tool gjson.Result) bool {
		tool.Get("functionDeclarations").ForEach(func(_, fn gjson.Result) bool {
			ct := apicompat.ChatTool{Type: "function", Function: &apicompat.ChatFunction{
				Name:        fn.Get("name").String(),
				Description: fn.Get("description").String(),
			}}
			if params := fn.Get("parameters"); params.Exists() {
				ct.Function.Parameters = json.RawMessage(params.Raw)
			}
			ccReq.Tools = append(ccReq.Tools, ct)
			return true
		})
		return true
	})

	return json.Marshal(ccReq)
}

// geminiContentToChatMessage 把单条 Gemini content 转换为一个或多个 CC 消息。
// functionResponse part 会生成 role=tool 的消息；其余 part 合并为一条 user/assistant 消息。
func geminiContentToChatMessage(content gjson.Result) ([]apicompat.ChatMessage, bool) {
	role := content.Get("role").String()
	ccRole := "user"
	if role == "model" {
		ccRole = "assistant"
	}

	var out []apicompat.ChatMessage
	var textParts []string
	var contentParts []apicompat.ChatContentPart
	var toolCalls []apicompat.ChatToolCall
	hasImage := false

	content.Get("parts").ForEach(func(_, part gjson.Result) bool {
		switch {
		case part.Get("text").Exists():
			txt := part.Get("text").String()
			textParts = append(textParts, txt)
			contentParts = append(contentParts, apicompat.ChatContentPart{Type: "text", Text: txt})
		case part.Get("inlineData").Exists():
			mime := part.Get("inlineData.mimeType").String()
			data := part.Get("inlineData.data").String()
			if mime != "" && data != "" {
				hasImage = true
				contentParts = append(contentParts, apicompat.ChatContentPart{
					Type:     "image_url",
					ImageURL: &apicompat.ChatImageURL{URL: "data:" + mime + ";base64," + data},
				})
			}
		case part.Get("functionCall").Exists():
			fc := part.Get("functionCall")
			args := fc.Get("args")
			argStr := "{}"
			if args.Exists() {
				argStr = args.Raw
			}
			toolCalls = append(toolCalls, apicompat.ChatToolCall{
				Type:     "function",
				Function: apicompat.ChatFunctionCall{Name: fc.Get("name").String(), Arguments: argStr},
			})
		case part.Get("functionResponse").Exists():
			fr := part.Get("functionResponse")
			respRaw := fr.Get("response").Raw
			if respRaw == "" {
				respRaw = "{}"
			}
			out = append(out, apicompat.ChatMessage{
				Role:    "tool",
				Name:    fr.Get("name").String(),
				Content: jsonString(respRaw),
			})
		}
		return true
	})

	// assistant 带 functionCall
	if ccRole == "assistant" && len(toolCalls) > 0 {
		msg := apicompat.ChatMessage{Role: "assistant", ToolCalls: toolCalls}
		if len(textParts) > 0 {
			msg.Content = jsonString(strings.Join(textParts, ""))
		}
		out = append(out, msg)
		return out, len(out) > 0
	}

	// 普通文本 / 多模态消息
	if hasImage && len(contentParts) > 0 {
		if raw, err := json.Marshal(contentParts); err == nil {
			out = append(out, apicompat.ChatMessage{Role: ccRole, Content: raw})
		}
	} else if len(textParts) > 0 {
		out = append(out, apicompat.ChatMessage{Role: ccRole, Content: jsonString(strings.Join(textParts, ""))})
	}

	return out, len(out) > 0
}

// chatCompletionToGeminiNativeResponse 把上游 OpenAI 非流式响应转换为 Gemini generateContent 响应。
func chatCompletionToGeminiNativeResponse(ccBody []byte, model string) ([]byte, *ClaudeUsage, error) {
	root := gjson.ParseBytes(ccBody)
	choice := root.Get("choices.0")

	parts := make([]map[string]any, 0, 2)
	if text := choice.Get("message.content").String(); text != "" {
		parts = append(parts, map[string]any{"text": text})
	}
	choice.Get("message.tool_calls").ForEach(func(_, tc gjson.Result) bool {
		name := tc.Get("function.name").String()
		argsStr := tc.Get("function.arguments").String()
		var args any = map[string]any{}
		if strings.TrimSpace(argsStr) != "" {
			_ = json.Unmarshal([]byte(argsStr), &args)
		}
		parts = append(parts, map[string]any{"functionCall": map[string]any{"name": name, "args": args}})
		return true
	})
	if len(parts) == 0 {
		parts = append(parts, map[string]any{"text": ""})
	}

	finishReason := mapOpenAIFinishReasonToGemini(choice.Get("finish_reason").String())

	usage := &ClaudeUsage{}
	if u, ok := openAIUsageFromGJSON(root.Get("usage")); ok {
		usage = openAIUsageToClaudeUsage(u)
	}

	resp := map[string]any{
		"candidates": []map[string]any{{
			"content":      map[string]any{"role": "model", "parts": parts},
			"finishReason": finishReason,
			"index":        0,
		}},
		"usageMetadata": geminiUsageMetadata(usage),
		"modelVersion":  model,
	}
	out, err := json.Marshal(resp)
	return out, usage, err
}

// buildGeminiStreamChunk 构造单个 Gemini streamGenerateContent SSE data 负载（JSON 字符串）。
func buildGeminiStreamChunk(text string, finishReason string, usage *ClaudeUsage) string {
	parts := []map[string]any{}
	if text != "" {
		parts = append(parts, map[string]any{"text": text})
	}
	candidate := map[string]any{
		"content": map[string]any{"role": "model", "parts": parts},
		"index":   0,
	}
	if finishReason != "" {
		candidate["finishReason"] = finishReason
	}
	chunk := map[string]any{"candidates": []map[string]any{candidate}}
	if usage != nil && (usage.InputTokens > 0 || usage.OutputTokens > 0) {
		chunk["usageMetadata"] = geminiUsageMetadata(usage)
	}
	out, _ := json.Marshal(chunk)
	return string(out)
}

func geminiUsageMetadata(usage *ClaudeUsage) map[string]any {
	if usage == nil {
		usage = &ClaudeUsage{}
	}
	prompt := usage.InputTokens + usage.CacheReadInputTokens
	return map[string]any{
		"promptTokenCount":     prompt,
		"candidatesTokenCount": usage.OutputTokens,
		"totalTokenCount":      prompt + usage.OutputTokens,
	}
}

func mapOpenAIFinishReasonToGemini(reason string) string {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "stop", "tool_calls", "function_call":
		return "STOP"
	case "length":
		return "MAX_TOKENS"
	case "content_filter":
		return "SAFETY"
	case "":
		return ""
	default:
		return "STOP"
	}
}

func joinGeminiPartsText(parts gjson.Result) string {
	var b strings.Builder
	parts.ForEach(func(_, p gjson.Result) bool {
		_, _ = b.WriteString(p.Get("text").String())
		return true
	})
	return b.String()
}

func jsonString(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}
