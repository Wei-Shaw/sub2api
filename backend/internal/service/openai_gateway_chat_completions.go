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
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"go.uber.org/zap"
)

// cursorResponsesUnsupportedFields are top-level Responses API parameters that
// Codex upstreams reject with "Unsupported parameter: ...". They must be
// stripped when forwarding a raw client body through the Responses-shape
// short-circuit in ForwardAsChatCompletions (see isResponsesShape branch).
// The normal Chat Completions → Responses conversion path is unaffected
// because ChatCompletionsRequest has no fields for these parameters — unknown
// fields are dropped naturally by json.Unmarshal. Kept semantically in sync
// with the list in openai_gateway_service.go:2034 used by the /v1/responses
// passthrough path.
var cursorResponsesUnsupportedFields = []string{
	"prompt_cache_retention",
	"safety_identifier",
	"metadata",
	"stream_options",
}

// ForwardAsChatCompletions accepts a Chat Completions request body, converts it
// to OpenAI Responses API format, forwards to the OpenAI upstream, and converts
// the response back to Chat Completions format.
//
// 历史背景：该函数原本对所有 OpenAI 账号无差别走 CC→Responses 转换 + /v1/responses
// 端点——这在 OAuth（ChatGPT 内部 API 仅支持 Responses）和官方 APIKey 账号上是
// 正确的，但 sub2api 接入 DeepSeek/Kimi/GLM 等第三方 OpenAI 兼容上游后假设破裂：
// 这些上游普遍只支持 /v1/chat/completions，无 /v1/responses 端点。
//
// 当前路由策略（基于账号覆盖模式/探测标记，详见 openai_compat.ShouldUseResponsesAPI）：
//   - APIKey 账号 + 强制或探测确认不支持 Responses → 走 forwardAsRawChatCompletions
//     直转上游 /v1/chat/completions，不做协议转换
//   - 其他所有情况（OAuth、APIKey 强制/探测确认支持、未探测）→ 走原有 CC→Responses
//     转换路径（保留旧行为，存量未探测账号零兼容破坏）
func (s *OpenAIGatewayService) ForwardAsChatCompletions(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	promptCacheKey string,
	defaultMappedModel string,
) (*OpenAIForwardResult, error) {
	// 入口分流：APIKey 账号 + 强制或已探测确认上游不支持 Responses，走 CC 直转。
	// 自动模式下标记缺失（未探测）按"现状即证据"原则继续走下方原 Responses 转换路径。
	if account.Type == AccountTypeAPIKey && !openai_compat.ShouldUseResponsesAPI(account.Extra) {
		return s.forwardAsRawChatCompletions(ctx, c, account, body, defaultMappedModel)
	}

	startTime := time.Now()

	// 1. Parse Chat Completions request
	var chatReq apicompat.ChatCompletionsRequest
	if err := json.Unmarshal(body, &chatReq); err != nil {
		return nil, fmt.Errorf("parse chat completions request: %w", err)
	}
	originalModel := chatReq.Model
	clientStream := chatReq.Stream

	// 2. Resolve model mapping early so compat prompt_cache_key injection can
	// derive a stable seed from the final upstream model family.
	billingModel := resolveOpenAIForwardModel(account, originalModel, defaultMappedModel)
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)
	chatCompletionsCompat := account != nil && account.IsOpenAIChatCompletionsCompatEnabled()

	promptCacheKey = strings.TrimSpace(promptCacheKey)
	compatPromptCacheInjected := false
	if promptCacheKey == "" && account.Type == AccountTypeOAuth && shouldAutoInjectPromptCacheKeyForCompat(upstreamModel) {
		promptCacheKey = deriveCompatPromptCacheKey(&chatReq, upstreamModel)
		compatPromptCacheInjected = promptCacheKey != ""
	}

	if chatCompletionsCompat {
		isResponsesShape := !gjson.GetBytes(body, "messages").Exists() && gjson.GetBytes(body, "input").Exists()
		if isResponsesShape {
			writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", "Responses-style body is not supported when chat completions compatibility mode is enabled")
			return nil, fmt.Errorf("responses-style body is not supported in openai_chat_completions_compat mode")
		}

		compatBody, err := sjson.SetBytes(body, "model", upstreamModel)
		if err != nil {
			return nil, fmt.Errorf("rewrite model in chat completions body: %w", err)
		}

		token, _, err := s.GetAccessToken(ctx, account)
		if err != nil {
			return nil, fmt.Errorf("get access token: %w", err)
		}

		upstreamReq, err := s.buildUpstreamRequestChatCompletionsCompat(ctx, c, account, compatBody, token, clientStream)
		if err != nil {
			return nil, fmt.Errorf("build upstream request: %w", err)
		}

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
			writeChatCompletionsError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed")
			return nil, fmt.Errorf("upstream request failed: %s", safeErr)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode >= 400 {
			return s.handleChatCompletionsErrorResponse(resp, c, account)
		}

		var (
			usage           *OpenAIUsage
			firstTokenMs    *int
			finalOutputText string
		)
		if clientStream {
			streamResult, err := s.handleChatCompletionsCompatStreamingResponse(resp, c, originalModel, upstreamModel, startTime)
			if err != nil {
				return nil, err
			}
			usage = streamResult.usage
			firstTokenMs = streamResult.firstTokenMs
			finalOutputText = streamResult.finalOutputText
		} else {
			var err error
			usage, finalOutputText, err = s.handleChatCompletionsCompatNonStreamingResponse(resp, c, originalModel, upstreamModel)
			if err != nil {
				return nil, err
			}
		}
		if usage == nil {
			usage = &OpenAIUsage{}
		}

		var serviceTier *string
		if st := strings.TrimSpace(chatReq.ServiceTier); st != "" {
			serviceTier = &st
		}
		var reasoningEffort *string
		if effort := strings.TrimSpace(chatReq.ReasoningEffort); effort != "" {
			reasoningEffort = &effort
		}

		return &OpenAIForwardResult{
			RequestID:       resp.Header.Get("x-request-id"),
			Usage:           *usage,
			Model:           originalModel,
			BillingModel:    billingModel,
			UpstreamModel:   upstreamModel,
			ServiceTier:     serviceTier,
			ReasoningEffort: reasoningEffort,
			Stream:          clientStream,
			Duration:        time.Since(startTime),
			FirstTokenMs:    firstTokenMs,
			FinalOutputText: finalOutputText,
		}, nil
	}

	// 3. Build the upstream (Responses API) body.
	//
	// Cursor compatibility: some clients (notably Cursor cloud) send Responses
	// API shaped bodies — `input: [...]` with no `messages` field — to the
	// /v1/chat/completions URL. Running those through ChatCompletionsToResponses
	// would silently drop Cursor's `input` array (the struct has no Input field)
	// and produce `input: null`, which Codex upstreams reject with
	// "Invalid type for 'input': expected a string, but got an object".
	//
	// Detect that shape and forward the raw body as-is, only rewriting `model`
	// to the resolved upstream model. The downstream codex OAuth transform will
	// still normalize store/stream/instructions/etc.
	isResponsesShape := !gjson.GetBytes(body, "messages").Exists() && gjson.GetBytes(body, "input").Exists()

	var (
		responsesReq  *apicompat.ResponsesRequest
		responsesBody []byte
		err           error
	)
	if isResponsesShape {
		responsesBody, err = sjson.SetBytes(body, "model", upstreamModel)
		if err != nil {
			return nil, fmt.Errorf("rewrite model in responses-shape body: %w", err)
		}
		// Strip Responses API parameters that no Codex upstream accepts.
		// Because this branch forwards the raw body (the normal path rebuilds
		// it from ChatCompletionsRequest and drops unknown fields naturally),
		// we must filter these fields explicitly here — otherwise the upstream
		// rejects the request with "Unsupported parameter: ...".
		for _, field := range cursorResponsesUnsupportedFields {
			if stripped, derr := sjson.DeleteBytes(responsesBody, field); derr == nil {
				responsesBody = stripped
			}
		}
		// Minimal stub populated from the raw body so downstream billing
		// propagation (ServiceTier, ReasoningEffort) keeps working.
		responsesReq = &apicompat.ResponsesRequest{
			Model:       upstreamModel,
			ServiceTier: gjson.GetBytes(responsesBody, "service_tier").String(),
		}
		if effort := gjson.GetBytes(responsesBody, "reasoning.effort").String(); effort != "" {
			responsesReq.Reasoning = &apicompat.ResponsesReasoning{Effort: effort}
		}
	} else {
		// Normal path: convert Chat Completions → Responses.
		// ChatCompletionsToResponses always sets Stream=true (upstream always streams).
		responsesReq, err = apicompat.ChatCompletionsToResponses(&chatReq)
		if err != nil {
			return nil, fmt.Errorf("convert chat completions to responses: %w", err)
		}
		responsesReq.Model = upstreamModel
		responsesBody, err = json.Marshal(responsesReq)
		if err != nil {
			return nil, fmt.Errorf("marshal responses request: %w", err)
		}
	}

	logFields := []zap.Field{
		zap.Int64("account_id", account.ID),
		zap.String("original_model", originalModel),
		zap.String("billing_model", billingModel),
		zap.String("upstream_model", upstreamModel),
		zap.Bool("stream", clientStream),
		zap.Bool("responses_shape", isResponsesShape),
	}
	if compatPromptCacheInjected {
		logFields = append(logFields,
			zap.Bool("compat_prompt_cache_key_injected", true),
			zap.String("compat_prompt_cache_key_sha256", hashSensitiveValueForLog(promptCacheKey)),
		)
	}
	logger.L().Debug("openai chat_completions: model mapping applied", logFields...)

	if account.Type == AccountTypeOAuth {
		var reqBody map[string]any
		if err := json.Unmarshal(responsesBody, &reqBody); err != nil {
			return nil, fmt.Errorf("unmarshal for codex transform: %w", err)
		}
		codexResult := applyCodexOAuthTransform(reqBody, false, false)
		if codexResult.NormalizedModel != "" {
			upstreamModel = codexResult.NormalizedModel
		}
		if codexResult.PromptCacheKey != "" {
			promptCacheKey = codexResult.PromptCacheKey
		} else if promptCacheKey != "" {
			reqBody["prompt_cache_key"] = promptCacheKey
		}
		responsesBody, err = json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("remarshal after codex transform: %w", err)
		}
	}

	// 4b. Apply OpenAI fast policy (may filter service_tier or block the request).
	updatedBody, policyErr := s.applyOpenAIFastPolicyToBody(ctx, account, upstreamModel, responsesBody)
	if policyErr != nil {
		var blocked *OpenAIFastBlockedError
		if errors.As(policyErr, &blocked) {
			MarkOpsClientBusinessLimited(c, OpsClientBusinessLimitedReasonLocalPolicyDenied)
			writeChatCompletionsError(c, http.StatusForbidden, "permission_error", blocked.Message)
		}
		return nil, policyErr
	}
	responsesBody = updatedBody

	// 5. Get access token
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("get access token: %w", err)
	}

	// 6. Build upstream request
	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	upstreamReq, err := s.buildUpstreamRequest(upstreamCtx, c, account, responsesBody, token, true, promptCacheKey, false)
	releaseUpstreamCtx()
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}

	if promptCacheKey != "" {
		upstreamReq.Header.Set("session_id", generateSessionUUID(promptCacheKey))
	}

	// 7. Send request
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
		writeChatCompletionsError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed")
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
	}
	defer func() { _ = resp.Body.Close() }()

	// 8. Handle error response with failover
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))

		upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
		upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
		if account.Type == AccountTypeAPIKey &&
			openai_compat.ResolveResponsesSupport(account.Extra) == openai_compat.ResponsesSupportUnknown &&
			!isResponsesEndpointSupportedByStatus(resp.StatusCode) {
			logger.L().Info("openai chat_completions: /responses unsupported, falling back to raw chat completions",
				zap.Int64("account_id", account.ID),
				zap.Int("upstream_status", resp.StatusCode),
				zap.String("upstream_message", upstreamMsg),
			)
			return s.forwardAsRawChatCompletions(ctx, c, account, body, defaultMappedModel)
		}
		if s.shouldFailoverOpenAIUpstreamResponse(resp.StatusCode, upstreamMsg, respBody) {
			upstreamDetail := ""
			if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
				maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
				if maxBytes <= 0 {
					maxBytes = 2048
				}
				upstreamDetail = truncateString(string(respBody), maxBytes)
			}
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				Kind:               "failover",
				Message:            upstreamMsg,
				Detail:             upstreamDetail,
			})
			s.handleOpenAIAccountUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody, upstreamModel)
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				RetryableOnSameAccount: account.IsPoolMode() && (account.IsPoolModeRetryableStatus(resp.StatusCode) || isOpenAITransientProcessingError(resp.StatusCode, upstreamMsg, respBody)),
			}
		}
		return s.handleChatCompletionsErrorResponse(resp, c, account, billingModel)
	}

	// 9. Handle normal response
	var result *OpenAIForwardResult
	var handleErr error
	if clientStream {
		result, handleErr = s.handleChatStreamingResponse(resp, c, account, originalModel, billingModel, upstreamModel, startTime, len(body))
	} else {
		result, handleErr = s.handleChatBufferedStreamingResponse(resp, c, originalModel, billingModel, upstreamModel, startTime)
	}

	// Propagate ServiceTier and ReasoningEffort to result for billing
	if handleErr == nil && result != nil {
		if responsesReq.ServiceTier != "" {
			st := responsesReq.ServiceTier
			result.ServiceTier = &st
		}
		if responsesReq.Reasoning != nil && responsesReq.Reasoning.Effort != "" {
			re := responsesReq.Reasoning.Effort
			result.ReasoningEffort = &re
		}
	}

	// Extract and save Codex usage snapshot from response headers (for OAuth accounts)
	if handleErr == nil && account.Type == AccountTypeOAuth {
		if snapshot := ParseCodexRateLimitHeaders(resp.Header); snapshot != nil {
			s.updateCodexUsageSnapshot(ctx, account.ID, snapshot)
		}
	}

	return result, handleErr
}

func (s *OpenAIGatewayService) buildUpstreamRequestChatCompletionsCompat(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	token string,
	clientStream bool,
) (*http.Request, error) {
	baseURL := account.GetOpenAIBaseURL()
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	validatedURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	targetURL := buildOpenAIChatCompletionsURL(validatedURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("authorization", "Bearer "+token)
	req.Header.Set("content-type", "application/json")
	if clientStream {
		req.Header.Set("accept", "text/event-stream")
	} else {
		req.Header.Set("accept", "application/json")
	}
	for key, values := range c.Request.Header {
		lowerKey := strings.ToLower(key)
		if openaiAllowedHeaders[lowerKey] {
			for _, v := range values {
				req.Header.Add(key, v)
			}
		}
	}
	if customUA := account.GetOpenAIUserAgent(); customUA != "" {
		req.Header.Set("user-agent", customUA)
	}
	return req, nil
}

func extractOpenAIChatCompletionsUsageFromJSONBytes(body []byte) (OpenAIUsage, bool) {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return OpenAIUsage{}, false
	}
	values := gjson.GetManyBytes(
		body,
		"usage.prompt_tokens",
		"usage.completion_tokens",
		"usage.prompt_tokens_details.cached_tokens",
		"usage.input_tokens",
		"usage.output_tokens",
	)
	prompt := int(values[0].Int())
	completion := int(values[1].Int())
	cached := int(values[2].Int())
	if prompt == 0 && completion == 0 {
		prompt = int(values[3].Int())
		completion = int(values[4].Int())
	}
	return OpenAIUsage{
		InputTokens:          prompt,
		OutputTokens:         completion,
		CacheReadInputTokens: cached,
	}, prompt > 0 || completion > 0 || cached > 0
}

func parseOpenAIChatCompletionsSSEUsageBytes(data []byte, usage *OpenAIUsage) {
	if usage == nil || len(data) == 0 || bytes.Equal(data, []byte("[DONE]")) {
		return
	}
	prompt := int(gjson.GetBytes(data, "usage.prompt_tokens").Int())
	completion := int(gjson.GetBytes(data, "usage.completion_tokens").Int())
	cached := int(gjson.GetBytes(data, "usage.prompt_tokens_details.cached_tokens").Int())
	if prompt == 0 && completion == 0 && cached == 0 {
		return
	}
	usage.InputTokens = prompt
	usage.OutputTokens = completion
	usage.CacheReadInputTokens = cached
}

func extractOpenAIChatCompletionsDeltaText(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	return strings.TrimSpace(gjson.GetBytes(data, "choices.0.delta.content").String())
}

func extractOpenAIChatCompletionsMessageText(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	return strings.TrimSpace(gjson.GetBytes(body, "choices.0.message.content").String())
}

func (s *OpenAIGatewayService) handleChatCompletionsCompatNonStreamingResponse(
	resp *http.Response,
	c *gin.Context,
	originalModel string,
	upstreamModel string,
) (*OpenAIUsage, string, error) {
	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, "", err
	}
	usageValue, usageOK := extractOpenAIChatCompletionsUsageFromJSONBytes(body)
	if !usageOK {
		return nil, "", fmt.Errorf("parse chat completions response: invalid json response")
	}
	if originalModel != upstreamModel {
		body = s.replaceModelInResponseBody(body, upstreamModel, originalModel)
	}
	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(resp.StatusCode, contentType, body)
	return &usageValue, extractOpenAIChatCompletionsMessageText(body), nil
}

func (s *OpenAIGatewayService) handleChatCompletionsCompatStreamingResponse(
	resp *http.Response,
	c *gin.Context,
	originalModel string,
	upstreamModel string,
	startTime time.Time,
) (*openaiStreamingResult, error) {
	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	w := c.Writer
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
	}
	bufferedWriter := bufio.NewWriterSize(w, 4*1024)
	flushBuffered := func() error {
		if err := bufferedWriter.Flush(); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	usage := &OpenAIUsage{}
	var firstTokenMs *int
	var finalTextBuilder strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	sawDone := false
	for scanner.Scan() {
		line := scanner.Text()
		if data, ok := extractOpenAISSEDataLine(line); ok {
			if originalModel != upstreamModel && upstreamModel != "" && strings.Contains(data, upstreamModel) {
				line = s.replaceModelInSSELine(line, upstreamModel, originalModel)
				data, _ = extractOpenAISSEDataLine(line)
			}
			if _, err := bufferedWriter.WriteString(line + "\n"); err != nil {
				return &openaiStreamingResult{
					usage:           usage,
					firstTokenMs:    firstTokenMs,
					finalOutputText: strings.TrimSpace(finalTextBuilder.String()),
				}, fmt.Errorf("stream write error: %w", err)
			}
			if err := flushBuffered(); err != nil {
				return &openaiStreamingResult{
					usage:           usage,
					firstTokenMs:    firstTokenMs,
					finalOutputText: strings.TrimSpace(finalTextBuilder.String()),
				}, fmt.Errorf("stream flush error: %w", err)
			}
			if data == "[DONE]" {
				sawDone = true
				continue
			}
			dataBytes := []byte(data)
			if firstTokenMs == nil {
				ms := int(time.Since(startTime).Milliseconds())
				firstTokenMs = &ms
			}
			parseOpenAIChatCompletionsSSEUsageBytes(dataBytes, usage)
			if delta := extractOpenAIChatCompletionsDeltaText(dataBytes); delta != "" {
				finalTextBuilder.WriteString(delta)
			}
			continue
		}
		if _, err := bufferedWriter.WriteString(line + "\n"); err != nil {
			return &openaiStreamingResult{
				usage:           usage,
				firstTokenMs:    firstTokenMs,
				finalOutputText: strings.TrimSpace(finalTextBuilder.String()),
			}, fmt.Errorf("stream write error: %w", err)
		}
		if err := flushBuffered(); err != nil {
			return &openaiStreamingResult{
				usage:           usage,
				firstTokenMs:    firstTokenMs,
				finalOutputText: strings.TrimSpace(finalTextBuilder.String()),
			}, fmt.Errorf("stream flush error: %w", err)
		}
	}
	if err := scanner.Err(); err != nil {
		return &openaiStreamingResult{
			usage:           usage,
			firstTokenMs:    firstTokenMs,
			finalOutputText: strings.TrimSpace(finalTextBuilder.String()),
		}, fmt.Errorf("stream read error: %w", err)
	}
	if !sawDone {
		return &openaiStreamingResult{
			usage:           usage,
			firstTokenMs:    firstTokenMs,
			finalOutputText: strings.TrimSpace(finalTextBuilder.String()),
		}, fmt.Errorf("stream usage incomplete: missing terminal event")
	}
	return &openaiStreamingResult{
		usage:           usage,
		firstTokenMs:    firstTokenMs,
		finalOutputText: strings.TrimSpace(finalTextBuilder.String()),
	}, nil
}

// handleChatCompletionsErrorResponse reads an upstream error and returns it in
// OpenAI Chat Completions error format.
func (s *OpenAIGatewayService) handleChatCompletionsErrorResponse(
	resp *http.Response,
	c *gin.Context,
	account *Account,
	requestedModel ...string,
) (*OpenAIForwardResult, error) {
	return s.handleCompatErrorResponse(resp, c, account, writeChatCompletionsError, requestedModel...)
}

// handleChatBufferedStreamingResponse reads all Responses SSE events from the
// upstream, finds the terminal event, converts to a Chat Completions JSON
// response, and writes it to the client.
func (s *OpenAIGatewayService) handleChatBufferedStreamingResponse(
	resp *http.Response,
	c *gin.Context,
	originalModel string,
	billingModel string,
	upstreamModel string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")

	finalResponse, usage, acc, err := s.readOpenAICompatBufferedTerminal(resp, "openai chat_completions buffered", requestID)
	if err != nil {
		return nil, err
	}

	if finalResponse == nil {
		writeChatCompletionsError(c, http.StatusBadGateway, "api_error", "Upstream stream ended without a terminal response event")
		return nil, fmt.Errorf("upstream stream ended without terminal event")
	}

	// When the terminal event has an empty output array, reconstruct from
	// accumulated delta events so the client receives the full content.
	acc.SupplementResponseOutput(finalResponse)

	chatResp := apicompat.ResponsesToChatCompletions(finalResponse, originalModel)
	finalOutputText := extractChatCompletionsAssistantText(chatResp)
	finalOutputJSON, _ := json.Marshal(chatResp)

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}
	c.JSON(http.StatusOK, chatResp)

	return &OpenAIForwardResult{
		RequestID:       requestID,
		Usage:           usage,
		Model:           originalModel,
		BillingModel:    billingModel,
		UpstreamModel:   upstreamModel,
		Stream:          false,
		Duration:        time.Since(startTime),
		FinalOutputText: finalOutputText,
		FinalOutputJSON: cloneJSONRawMessage(finalOutputJSON),
	}, nil
}

// handleChatStreamingResponse reads Responses SSE events from upstream,
// converts each to Chat Completions SSE chunks, and writes them to the client.
func (s *OpenAIGatewayService) handleChatStreamingResponse(
	resp *http.Response,
	c *gin.Context,
	account *Account,
	originalModel string,
	billingModel string,
	upstreamModel string,
	startTime time.Time,
	requestBodyLen int,
) (*OpenAIForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")

	headersWritten := false
	writeStreamHeaders := func() {
		if headersWritten {
			return
		}
		headersWritten = true
		if s.responseHeaderFilter != nil {
			responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
		}
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("Connection", "keep-alive")
		c.Writer.Header().Set("X-Accel-Buffering", "no")
		c.Writer.WriteHeader(http.StatusOK)
	}

	state := apicompat.NewResponsesEventToChatState()
	state.Model = originalModel
	// 网关作为计费链路的一环，不能把下游 usage 输出绑定到客户端是否显式请求。
	// raw Chat Completions 直转路径已经强制透出 usage，这里保持同样行为，避免级联代理计费为 0。
	state.IncludeUsage = true

	var usage OpenAIUsage
	var firstTokenMs *int
	firstChunk := true
	clientDisconnected := false
	clientOutputStarted := false
	pendingSSE := make([]string, 0, 4)
	refusalDetector := newOpenAIChatSilentRefusalDetector(requestBodyLen)

	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

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

	resultWithUsage := func() *OpenAIForwardResult {
		return &OpenAIForwardResult{
			RequestID:     requestID,
			Usage:         usage,
			Model:         originalModel,
			BillingModel:  billingModel,
			UpstreamModel: upstreamModel,
			Stream:        true,
			Duration:      time.Since(startTime),
			FirstTokenMs:  firstTokenMs,
		}
	}

	processDataLine := func(payload string) bool {
		if firstChunk {
			firstChunk = false
			ms := int(time.Since(startTime).Milliseconds())
			firstTokenMs = &ms
		}

		var event apicompat.ResponsesStreamEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			logger.L().Warn("openai chat_completions stream: failed to parse event",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
			return false
		}
		refusalDetector.ObservePayload([]byte(payload))

		isTerminalEvent := isOpenAICompatResponsesTerminalEvent(event.Type)
		if isTerminalEvent {
			if event.Usage != nil {
				usage = copyOpenAIUsageFromResponsesUsage(event.Usage)
			}
			if event.Response != nil && event.Response.Usage != nil {
				usage = copyOpenAIUsageFromResponsesUsage(event.Response.Usage)
			}
		}

		chunks := apicompat.ResponsesEventToChatChunks(&event, state)
		if !clientDisconnected {
			for _, chunk := range chunks {
				refusalDetector.ObserveChatChunk(chunk)
				sse, err := apicompat.ChatChunkToSSE(chunk)
				if err != nil {
					logger.L().Warn("openai chat_completions stream: failed to marshal chunk",
						zap.Error(err),
						zap.String("request_id", requestID),
					)
					continue
				}
				if !clientOutputStarted && !refusalDetector.ShouldReleaseClientOutput() {
					pendingSSE = append(pendingSSE, sse)
					continue
				}
				if !clientOutputStarted {
					writeStreamHeaders()
					for _, pending := range pendingSSE {
						if _, err := fmt.Fprint(c.Writer, pending); err != nil {
							clientDisconnected = true
							logger.L().Info("openai chat_completions stream: client disconnected while flushing pending chunks",
								zap.String("request_id", requestID),
							)
							break
						}
					}
					pendingSSE = pendingSSE[:0]
					clientOutputStarted = !clientDisconnected
					if clientDisconnected {
						break
					}
				}
				if _, err := fmt.Fprint(c.Writer, sse); err != nil {
					clientDisconnected = true
					logger.L().Info("openai chat_completions stream: client disconnected, continuing to drain upstream for billing",
						zap.String("request_id", requestID),
					)
					break
				}
			}
		}
		if len(chunks) > 0 && !clientDisconnected && clientOutputStarted {
			c.Writer.Flush()
		}
		return isTerminalEvent
	}

	finalizeStream := func() (*OpenAIForwardResult, error) {
		if finalChunks := apicompat.FinalizeResponsesChatStream(state); len(finalChunks) > 0 && !clientDisconnected {
			for _, chunk := range finalChunks {
				refusalDetector.ObserveChatChunk(chunk)
				sse, err := apicompat.ChatChunkToSSE(chunk)
				if err != nil {
					continue
				}
				if !clientOutputStarted && !refusalDetector.ShouldReleaseClientOutput() {
					pendingSSE = append(pendingSSE, sse)
					continue
				}
				if !clientOutputStarted {
					writeStreamHeaders()
					for _, pending := range pendingSSE {
						if _, err := fmt.Fprint(c.Writer, pending); err != nil {
							clientDisconnected = true
							logger.L().Info("openai chat_completions stream: client disconnected during pending final flush",
								zap.String("request_id", requestID),
							)
							break
						}
					}
					pendingSSE = pendingSSE[:0]
					clientOutputStarted = !clientDisconnected
					if clientDisconnected {
						break
					}
				}
				if _, err := fmt.Fprint(c.Writer, sse); err != nil {
					clientDisconnected = true
					logger.L().Info("openai chat_completions stream: client disconnected during final flush",
						zap.String("request_id", requestID),
					)
					break
				}
			}
		}
		if !clientDisconnected && !clientOutputStarted {
			if refusalDetector.IsSilentRefusal() {
				return nil, newOpenAISilentRefusalFailoverError(c, account, requestID)
			}
			if len(pendingSSE) > 0 {
				writeStreamHeaders()
				for _, pending := range pendingSSE {
					if _, err := fmt.Fprint(c.Writer, pending); err != nil {
						clientDisconnected = true
						logger.L().Info("openai chat_completions stream: client disconnected during final pending flush",
							zap.String("request_id", requestID),
						)
						break
					}
				}
				pendingSSE = pendingSSE[:0]
				clientOutputStarted = !clientDisconnected
			}
		}
		// Send [DONE] sentinel
		if !clientDisconnected {
			writeStreamHeaders()
			if _, err := fmt.Fprint(c.Writer, "data: [DONE]\n\n"); err != nil {
				clientDisconnected = true
				logger.L().Info("openai chat_completions stream: client disconnected during done flush",
					zap.String("request_id", requestID),
				)
			}
			clientOutputStarted = !clientDisconnected
		}
		if !clientDisconnected {
			c.Writer.Flush()
		}
		return resultWithUsage(), nil
	}

	handleScanErr := func(err error) {
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			logger.L().Warn("openai chat_completions stream: read error",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
		}
	}
	missingTerminalErr := func() (*OpenAIForwardResult, error) {
		return resultWithUsage(), fmt.Errorf("stream usage incomplete: missing terminal event")
	}
	processFrame := func(frame openAICompatSSEFrame) bool {
		payload := openAICompatPayloadWithEventType(frame.Data, frame.EventType)
		if strings.TrimSpace(payload) == "[DONE]" {
			return false
		}
		return processDataLine(payload)
	}

	// Determine keepalive interval
	keepaliveInterval := time.Duration(0)
	if s.cfg != nil && s.cfg.Gateway.StreamKeepaliveInterval > 0 {
		keepaliveInterval = time.Duration(s.cfg.Gateway.StreamKeepaliveInterval) * time.Second
	}

	// No keepalive: fast synchronous path
	if streamInterval <= 0 && keepaliveInterval <= 0 {
		var parser openAICompatSSEFrameParser
		for scanner.Scan() {
			line := scanner.Text()
			frame, ok := parser.AddLine(line)
			if !ok {
				continue
			}
			if strings.TrimSpace(frame.Data) == "[DONE]" {
				return missingTerminalErr()
			}
			if processFrame(frame) {
				return finalizeStream()
			}
		}
		if err := scanner.Err(); err != nil {
			handleScanErr(err)
			return resultWithUsage(), fmt.Errorf("stream usage incomplete: %w", err)
		}
		if frame, ok := parser.Finish(); ok {
			if strings.TrimSpace(frame.Data) == "[DONE]" {
				return missingTerminalErr()
			}
			if processFrame(frame) {
				return finalizeStream()
			}
		}
		return missingTerminalErr()
	}

	// With keepalive: goroutine + channel + select
	type scanEvent struct {
		line string
		err  error
	}
	events := make(chan scanEvent, 16)
	done := make(chan struct{})
	var lastReadAt int64
	atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
	sendEvent := func(ev scanEvent) bool {
		select {
		case events <- ev:
			return true
		case <-done:
			return false
		}
	}
	go func() {
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
	}()
	defer close(done)

	var keepaliveTicker *time.Ticker
	if keepaliveInterval > 0 {
		keepaliveTicker = time.NewTicker(keepaliveInterval)
		defer keepaliveTicker.Stop()
	}
	var keepaliveCh <-chan time.Time
	if keepaliveTicker != nil {
		keepaliveCh = keepaliveTicker.C
	}
	lastDataAt := time.Now()
	var parser openAICompatSSEFrameParser

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				if frame, ok := parser.Finish(); ok {
					if strings.TrimSpace(frame.Data) == "[DONE]" {
						return missingTerminalErr()
					}
					if processFrame(frame) {
						return finalizeStream()
					}
				}
				return missingTerminalErr()
			}
			if ev.err != nil {
				handleScanErr(ev.err)
				return resultWithUsage(), fmt.Errorf("stream usage incomplete: %w", ev.err)
			}
			lastDataAt = time.Now()
			line := ev.line
			frame, ok := parser.AddLine(line)
			if !ok {
				continue
			}
			if strings.TrimSpace(frame.Data) == "[DONE]" {
				return missingTerminalErr()
			}
			if processFrame(frame) {
				return finalizeStream()
			}

		case <-intervalCh:
			lastRead := time.Unix(0, atomic.LoadInt64(&lastReadAt))
			if time.Since(lastRead) < streamInterval {
				continue
			}
			if clientDisconnected {
				return resultWithUsage(), fmt.Errorf("stream usage incomplete after timeout")
			}
			logger.L().Warn("openai chat_completions stream: data interval timeout",
				zap.String("request_id", requestID),
				zap.String("model", originalModel),
				zap.Duration("interval", streamInterval),
			)
			return resultWithUsage(), fmt.Errorf("stream data interval timeout")

		case <-keepaliveCh:
			if clientDisconnected {
				continue
			}
			if refusalDetector.Enabled() && !clientOutputStarted {
				continue
			}
			if time.Since(lastDataAt) < keepaliveInterval {
				continue
			}
			// Send SSE comment as keepalive
			writeStreamHeaders()
			if _, err := fmt.Fprint(c.Writer, ":\n\n"); err != nil {
				logger.L().Info("openai chat_completions stream: client disconnected during keepalive",
					zap.String("request_id", requestID),
				)
				clientDisconnected = true
				continue
			}
			c.Writer.Flush()
		}
	}
}

// writeChatCompletionsError writes an error response in OpenAI Chat Completions format.
func writeChatCompletionsError(c *gin.Context, statusCode int, errType, message string) {
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
}
