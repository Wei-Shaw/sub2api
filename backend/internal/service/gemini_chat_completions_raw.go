package service

import (
	"bufio"
	"bytes"
	"context"
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
	"github.com/tidwall/gjson"
)

// forwardRawOpenAIChatCompletions 以 OpenAI 兼容 Chat Completions 协议把请求原样透传到上游
// {base_url}/chat/completions。两类账号会走到这里（见 Account.UsesOpenAICompatRawForward）：
//   - 自定义平台（platform=custom，apikey 类型）：管理员自填 base_url + api_key，
//     其支持的模型通过同步上游 {base_url}/models 获取（见 buildCustomUpstreamModelsRequest）
//   - Gemini 平台显式选择 openai_chat_completions 上游模式的账号
func (s *GeminiMessagesCompatService) forwardRawOpenAIChatCompletions(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	ccReq *apicompat.ChatCompletionsRequest,
	startTime time.Time,
) (*ForwardResult, error) {
	originalModel := strings.TrimSpace(ccReq.Model)
	if originalModel == "" {
		return nil, s.writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
	}

	apiKey := strings.TrimSpace(account.GetCredential("api_key"))
	if apiKey == "" {
		return nil, s.writeChatCompletionsError(c, http.StatusBadGateway, "upstream_error", "api_key not configured")
	}

	upstreamModel := account.GetMappedModel(originalModel)
	if upstreamModel == "" {
		upstreamModel = originalModel
	}
	upstreamBody := body
	if upstreamModel != originalModel {
		upstreamBody = ReplaceModelInBody(body, upstreamModel)
	}
	if ccReq.Stream {
		updated, err := ensureOpenAIChatStreamUsage(upstreamBody)
		if err != nil {
			return nil, fmt.Errorf("enable stream usage: %w", err)
		}
		upstreamBody = updated
	}

	baseURL := strings.TrimSpace(account.GetCredential("base_url"))
	if baseURL == "" {
		return nil, s.writeChatCompletionsError(c, http.StatusBadGateway, "upstream_error", "base_url is required for OpenAI-compatible upstream")
	}
	normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, s.writeChatCompletionsError(c, http.StatusBadGateway, "upstream_error", "invalid base_url: "+err.Error())
	}
	targetURL := buildOpenAIChatCompletionsURL(normalizedBaseURL)

	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	upstreamReq, err := http.NewRequestWithContext(upstreamCtx, http.MethodPost, targetURL, bytes.NewReader(upstreamBody))
	releaseUpstreamCtx()
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}
	upstreamReq = upstreamReq.WithContext(WithHTTPUpstreamProfile(upstreamReq.Context(), HTTPUpstreamProfileOpenAI))
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Authorization", "Bearer "+apiKey)
	if ccReq.Stream {
		upstreamReq.Header.Set("Accept", "text/event-stream")
	} else {
		upstreamReq.Header.Set("Accept", "application/json")
	}
	for key, values := range c.Request.Header {
		if openaiCCRawAllowedHeaders[strings.ToLower(key)] {
			for _, v := range values {
				upstreamReq.Header.Add(key, v)
			}
		}
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
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: 0,
			Kind:               "request_error",
			Message:            safeErr,
		})
		return nil, s.writeChatCompletionsError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed")
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
		return nil, s.writeGeminiChatCompletionsMappedError(c, account, resp.StatusCode, requestID, respBody)
	}

	reasoningEffort := extractCCReasoningEffortFromBody(body)
	if ccReq.Stream {
		return s.streamRawOpenAIChatCompletions(c, resp, account, originalModel, upstreamModel, reasoningEffort, startTime)
	}
	return s.bufferRawOpenAIChatCompletions(c, resp, originalModel, upstreamModel, reasoningEffort, startTime)
}

func (s *GeminiMessagesCompatService) streamRawOpenAIChatCompletions(
	c *gin.Context,
	resp *http.Response,
	account *Account,
	originalModel string,
	upstreamModel string,
	reasoningEffort *string,
	startTime time.Time,
) (*ForwardResult, error) {
	requestID := rawOpenAIChatRequestID(resp.Header)
	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
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
	var outputText strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if payload, ok := extractOpenAISSEDataLine(line); ok {
			trimmedPayload := strings.TrimSpace(payload)
			if trimmedPayload != "" && trimmedPayload != "[DONE]" {
				usageOnlyChunk := isOpenAIChatUsageOnlyStreamChunk(trimmedPayload)
				if u := extractCCStreamUsage(trimmedPayload); u != nil {
					usage = openAIUsageToClaudeUsage(*u)
				}
				if delta := gjson.Get(trimmedPayload, "choices.0.delta.content").String(); delta != "" {
					_, _ = outputText.WriteString(delta)
				}
				if firstTokenMs == nil && !usageOnlyChunk {
					elapsed := int(time.Since(startTime).Milliseconds())
					firstTokenMs = &elapsed
				}
			}
		}
		if !clientDisconnected {
			if _, err := c.Writer.WriteString(line + "\n"); err != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.gemini_chat_completions", "Gemini raw OpenAI-compatible stream client disconnected: %v", err)
			}
			if line == "" {
				c.Writer.Flush()
			}
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		logger.LegacyPrintf("service.gemini_chat_completions", "Gemini raw OpenAI-compatible stream read error: %v", err)
	}

	// 上游流式响应缺少 usage 时按累计输出文本估算，避免流式输出计费为 0。
	if usage.OutputTokens == 0 && outputText.Len() > 0 {
		usage.OutputTokens = estimateTokensForText(outputText.String())
	}

	return &ForwardResult{
		RequestID:        requestID,
		Usage:            *usage,
		Model:            originalModel,
		UpstreamModel:    upstreamModel,
		Stream:           true,
		Duration:         time.Since(startTime),
		FirstTokenMs:     firstTokenMs,
		ReasoningEffort:  reasoningEffort,
		ClientDisconnect: clientDisconnected,
	}, nil
}

func (s *GeminiMessagesCompatService) bufferRawOpenAIChatCompletions(
	c *gin.Context,
	resp *http.Response,
	originalModel string,
	upstreamModel string,
	reasoningEffort *string,
	startTime time.Time,
) (*ForwardResult, error) {
	requestID := rawOpenAIChatRequestID(resp.Header)
	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		if !errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
			return nil, s.writeChatCompletionsError(c, http.StatusBadGateway, "api_error", "Failed to read upstream response")
		}
		return nil, err
	}

	usage := &ClaudeUsage{}
	if u, ok := openAIUsageFromGJSON(gjson.GetBytes(respBody, "usage")); ok {
		usage = openAIUsageToClaudeUsage(u)
	}

	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		c.Writer.Header().Set("Content-Type", ct)
	} else {
		c.Writer.Header().Set("Content-Type", "application/json")
	}
	c.Writer.WriteHeader(http.StatusOK)
	_, _ = c.Writer.Write(respBody)

	return &ForwardResult{
		RequestID:       requestID,
		Usage:           *usage,
		Model:           originalModel,
		UpstreamModel:   upstreamModel,
		Stream:          false,
		Duration:        time.Since(startTime),
		ReasoningEffort: reasoningEffort,
	}, nil
}

func openAIUsageToClaudeUsage(u OpenAIUsage) *ClaudeUsage {
	return &ClaudeUsage{
		InputTokens:              u.InputTokens,
		OutputTokens:             u.OutputTokens,
		CacheCreationInputTokens: u.CacheCreationInputTokens,
		CacheReadInputTokens:     u.CacheReadInputTokens,
		ImageOutputTokens:        u.ImageOutputTokens,
	}
}

func rawOpenAIChatRequestID(header http.Header) string {
	for _, key := range []string{"x-request-id", "x-goog-request-id", "request-id"} {
		if v := strings.TrimSpace(header.Get(key)); v != "" {
			return v
		}
		for _, v := range header[key] {
			if trimmed := strings.TrimSpace(v); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}
