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
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

// ForwardAsResponses accepts an OpenAI Responses API request body, converts it
// to Anthropic Messages format, forwards to the Anthropic upstream, and converts
// the response back to Responses format. This enables OpenAI Responses API
// clients to access Anthropic models through Anthropic platform groups.
//
// The method follows the same pattern as OpenAIGatewayService.ForwardAsAnthropic
// but in reverse direction: Responses → Anthropic upstream → Responses.
func (s *GatewayService) ForwardAsResponses(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	parsed *ParsedRequest,
) (*ForwardResult, error) {
	startTime := time.Now()

	normalizedBody, normalized, err := normalizeOpenAIResponsesLegacyIngress(body)
	if err != nil {
		return nil, err
	}
	if normalized {
		body = normalizedBody
	}

	// 1. Lower Codex client-side tools to function tools understood by Anthropic.
	adaptedBody, clientToolMapping, err := adaptResponsesClientToolsForAnthropic(body)
	if err != nil {
		return nil, fmt.Errorf("adapt responses client tools: %w", err)
	}

	// 2. Parse Responses request
	var responsesReq apicompat.ResponsesRequest
	if err := json.Unmarshal(adaptedBody, &responsesReq); err != nil {
		return nil, fmt.Errorf("parse responses request: %w", err)
	}
	originalModel := responsesReq.Model
	clientStream := responsesReq.Stream

	// 3. Convert Responses → Anthropic
	anthropicReq, err := apicompat.ResponsesToAnthropicRequest(&responsesReq)
	if err != nil {
		return nil, fmt.Errorf("convert responses to anthropic: %w", err)
	}

	// 3. Force upstream streaming (Anthropic works best with streaming)
	anthropicReq.Stream = true
	reqStream := true

	// 4. Model mapping
	mappedModel := originalModel
	if account.Platform == PlatformKiro {
		if next := account.GetMappedModel(originalModel); next != "" {
			mappedModel = next
		}
	} else if account.Type == AccountTypeAPIKey || account.Type == AccountTypeServiceAccount {
		mappedModel = account.GetMappedModel(originalModel)
	}
	if mappedModel == originalModel && account.Platform == PlatformAnthropic && account.Type == AccountTypeServiceAccount {
		normalized := normalizeVertexAnthropicModelID(claude.NormalizeModelID(originalModel))
		if normalized != originalModel {
			mappedModel = normalized
		}
	} else if mappedModel == originalModel && account.Platform == PlatformAnthropic && account.Type != AccountTypeAPIKey {
		normalized := claude.NormalizeModelID(originalModel)
		if normalized != originalModel {
			mappedModel = normalized
		}
	}
	reasoningEffort := ExtractResponsesReasoningEffortFromBody(body, mappedModel, originalModel)
	// 国产模型默认 effort 补充：需要 mappedModel 判定，推迟到 mapping 完成之后。
	reasoningEffort = ApplyThinkingEnabledFallback(reasoningEffort, body, mappedModel)

	// 4b. Codex remote compaction v2：input 里带 compaction_trigger 的请求不是普通
	// 轮次，而是"把前文压缩成摘要"。Anthropic 协议族没有原生 compact 端点，转换器
	// 已把触发器降级成摘要指令（见 apicompat.CompactionSummaryPrompt），这里只需把
	// 请求参数调成适合产出摘要的形态。
	isCompact := apicompat.HasCompactionTrigger(&responsesReq)
	if isCompact {
		// 压缩专用模型映射：账号未配 compact_model_mapping 时沿用普通映射结果。
		if next, matched := account.ResolveCompactMappedModel(originalModel); matched {
			if trimmed := strings.TrimSpace(next); trimmed != "" {
				mappedModel = trimmed
			}
		}
		// 摘要轮次不允许调用工具。tools 必须保留：历史里的 tool_use 块引用了工具
		// 定义，删掉会让上游校验失败；tool_choice=none 已足够抑制调用。
		anthropicReq.ToolChoice = json.RawMessage(`{"type":"none"}`)
		// 摘要不需要思考块，省 token 也避免 thinking 与 tool_choice 的组合限制。
		anthropicReq.Thinking = nil
		// Codex 的 compact 请求不带 max_output_tokens，会落到转换器的 8192 默认值；
		// 对覆盖数十万 token 前文的结构化摘要偏紧，容易被截断。
		if anthropicReq.MaxTokens < compactionMinMaxTokens {
			anthropicReq.MaxTokens = compactionMinMaxTokens
		}
	}
	anthropicReq.Model = mappedModel

	logger.L().Debug("gateway forward_as_responses: model mapping applied",
		zap.Int64("account_id", account.ID),
		zap.String("original_model", originalModel),
		zap.String("mapped_model", mappedModel),
		zap.Bool("client_stream", clientStream),
		zap.Bool("compaction", isCompact),
	)

	// 5. Marshal Anthropic request body
	anthropicBody, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("marshal anthropic request: %w", err)
	}

	// 6. Apply Claude Code mimicry for OAuth accounts (non-Claude-Code endpoints).
	// OpenAI Responses 协议进来的请求永远不是 Claude Code 客户端，所以对 OAuth 账号
	// 必须完整执行 /v1/messages 主路径上的伪装链路（system 重写 + normalize + metadata 注入），
	// 否则会被 Anthropic 判为第三方应用并扣 extra usage。
	// 见 applyClaudeCodeOAuthMimicryToBody 与 shouldMimicClaudeCodeForAccount 的 godoc
	// （后者说明了 Kiro 为何必须排除）。
	isClaudeCode := false
	shouldMimicClaudeCode := shouldMimicClaudeCodeForAccount(account, isClaudeCode)

	if shouldMimicClaudeCode {
		anthropicBody = s.applyClaudeCodeOAuthMimicryToBody(ctx, c, account, anthropicBody, anthropicReq.System, mappedModel)
		clientToolMapping.CustomTools = responsesCustomToolsWithRewriteAliases(clientToolMapping.CustomTools, toolNameRewriteFromContext(c))
	}

	// 7. Enforce cache_control block limit
	anthropicBody = enforceCacheControlLimit(anthropicBody)

	var resp *http.Response
	if isKiroDirectModeAccount(account) {
		var group *Group
		if parsed != nil {
			group = parsed.Group
		}
		cachePlan := s.prepareKiroResponsesCacheEmulationUsage(ctx, account, group, body, mappedModel, estimateKiroInputTokens(ctx, anthropicBody))
		resp, _, err = s.openKiroAnthropicStreamResponse(ctx, account, parsed, anthropicBody, mappedModel, originalModel, c.Request.Header, group, cachePlan)
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
			writeResponsesError(c, http.StatusBadGateway, "server_error", "Upstream request failed")
			return nil, fmt.Errorf("upstream request failed: %s", safeErr)
		}
	} else {
		// 8. Get access token
		token, tokenType, err := s.GetAccessToken(ctx, account)
		if err != nil {
			return nil, fmt.Errorf("get access token: %w", err)
		}

		// 9. Get proxy URL
		proxyURL := ""
		if account.ProxyID != nil && account.Proxy != nil {
			proxyURL = account.Proxy.URL()
		}

		// 10. Build upstream request
		upstreamCtx, releaseUpstreamCtx := detachStreamUpstreamContext(ctx, reqStream)
		upstreamReq, _, err := s.buildUpstreamRequest(upstreamCtx, c, account, anthropicBody, token, tokenType, mappedModel, reqStream, shouldMimicClaudeCode)
		releaseUpstreamCtx()
		if err != nil {
			return nil, fmt.Errorf("build upstream request: %w", err)
		}

		// 11. Send request
		resp, err = s.httpUpstream.DoWithTLS(upstreamReq, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
		if err != nil {
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
			}
			return nil, s.handleUpstreamTransportError(ctx, c, account, err, OpsUpstreamErrorEvent{
				UpstreamURL: safeUpstreamURL(upstreamReq.URL.String()),
			})
		}
	}
	defer func() { _ = resp.Body.Close() }()

	// 12. Handle error response with failover
	if resp.StatusCode >= 400 {
		respBody, _ := s.readUpstreamErrorBody(resp)
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))

		upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
		upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)

		if s.shouldFailoverUpstreamError(resp.StatusCode) {
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				ProxyID:            opsUpstreamProxyID(account),
				ProxyName:          opsUpstreamProxyName(account),
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				Kind:               "failover",
				Message:            upstreamMsg,
			})
			shouldDisable := false
			if s.rateLimitService != nil {
				shouldDisable = s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody, mappedModel)
			}
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				RetryableOnSameAccount: !shouldDisable && account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
			}
		}

		// Non-failover error: return Responses-formatted error to client
		writeResponsesError(c, mapUpstreamStatusCode(resp.StatusCode), "server_error", upstreamMsg)
		return nil, fmt.Errorf("upstream error: %d %s", resp.StatusCode, upstreamMsg)
	}

	// 13. Handle normal response (convert Anthropic → Responses)
	var result *ForwardResult
	var handleErr error
	if isCompact {
		// compact 请求必须缓冲：要拿到完整摘要文本才能合成 Codex 要求的单个
		// compaction item，逐事件透传做不到。
		result, handleErr = s.handleResponsesCompactionResponse(resp, c, originalModel, mappedModel, reasoningEffort, startTime, clientStream)
	} else if clientStream {
		result, handleErr = s.handleResponsesStreamingResponse(resp, c, originalModel, mappedModel, reasoningEffort, startTime, clientToolMapping)
	} else {
		result, handleErr = s.handleResponsesBufferedStreamingResponse(resp, c, originalModel, mappedModel, reasoningEffort, startTime, clientToolMapping)
	}

	return result, handleErr
}

func adaptResponsesClientToolsForAnthropic(body []byte) ([]byte, apicompat.ResponsesClientToolMapping, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var requestBody map[string]any
	if err := decoder.Decode(&requestBody); err != nil {
		return body, apicompat.ResponsesClientToolMapping{}, err
	}
	additionalToolsChanged, err := liftResponsesAdditionalTools(requestBody)
	if err != nil {
		return body, apicompat.ResponsesClientToolMapping{}, err
	}

	mapping, changed, err := apicompat.AdaptResponsesClientTools(requestBody)
	if err != nil {
		return body, apicompat.ResponsesClientToolMapping{}, err
	}
	changed = changed || additionalToolsChanged
	if !changed {
		return body, mapping, nil
	}
	rebuilt, err := json.Marshal(requestBody)
	if err != nil {
		return body, apicompat.ResponsesClientToolMapping{}, err
	}
	return rebuilt, mapping, nil
}

func liftResponsesAdditionalTools(requestBody map[string]any) (bool, error) {
	input, ok := requestBody["input"].([]any)
	if !ok {
		return false, nil
	}

	tools, _ := requestBody["tools"].([]any)
	kept := make([]any, 0, len(input))
	changed := false
	for _, raw := range input {
		item, ok := raw.(map[string]any)
		if !ok || strings.TrimSpace(fmt.Sprint(item["type"])) != "additional_tools" {
			kept = append(kept, raw)
			continue
		}
		additional, ok := item["tools"].([]any)
		if !ok {
			return false, fmt.Errorf("additional_tools.tools must be an array")
		}
		tools = append(tools, additional...)
		changed = true
	}
	if !changed {
		return false, nil
	}
	requestBody["tools"] = tools
	requestBody["input"] = kept
	return true, nil
}

func responsesCustomToolsWithRewriteAliases(customTools map[string]bool, rw *ToolNameRewrite) map[string]bool {
	if len(customTools) == 0 || rw == nil || len(rw.Forward) == 0 {
		return customTools
	}

	var out map[string]bool
	for original, rewritten := range rw.Forward {
		if !customTools[original] || strings.TrimSpace(rewritten) == "" {
			continue
		}
		if out == nil {
			out = make(map[string]bool, len(customTools)+1)
			for name, ok := range customTools {
				out[name] = ok
			}
		}
		out[rewritten] = true
	}
	if out == nil {
		return customTools
	}
	return out
}

// ExtractResponsesReasoningEffortFromBody reads Responses API reasoning.effort
// and normalizes it for usage logging.
func ExtractResponsesReasoningEffortFromBody(body []byte, modelCandidates ...string) *string {
	raw := strings.TrimSpace(gjson.GetBytes(body, "reasoning.effort").String())
	if raw == "" {
		return nil
	}
	model := firstNonEmpty(modelCandidates...)
	if model == "" {
		model = strings.TrimSpace(gjson.GetBytes(body, "model").String())
	}
	normalized := normalizeOpenAIReasoningEffortForModel(raw, model)
	if normalized == "" {
		return nil
	}
	return &normalized
}

func mergeAnthropicUsage(dst *ClaudeUsage, src apicompat.AnthropicUsage) {
	if dst == nil {
		return
	}

	// Some Anthropic-compatible providers retain OpenAI-style prompt/cache
	// fields. Prefer those authoritative totals or hit/miss buckets over the
	// overloaded input_tokens field. This covers Kimi's changing stream
	// semantics as well as GLM/DeepSeek cache aliases.
	if src.PromptTokens > 0 || src.PromptCacheHitTokens != nil || src.PromptCacheMissTokens != nil {
		cacheReadTokens := src.CacheReadInputTokens
		if cacheReadTokens == 0 && src.CachedTokens > 0 {
			cacheReadTokens = src.CachedTokens
		}
		if cacheReadTokens == 0 && src.PromptTokensDetails != nil && src.PromptTokensDetails.CachedTokens > 0 {
			cacheReadTokens = src.PromptTokensDetails.CachedTokens
		}
		if cacheReadTokens == 0 && src.PromptCacheHitTokens != nil {
			cacheReadTokens = max(*src.PromptCacheHitTokens, 0)
		}

		if src.PromptCacheMissTokens != nil {
			dst.InputTokens = max(*src.PromptCacheMissTokens, 0)
		} else {
			dst.InputTokens = max(src.PromptTokens-cacheReadTokens-src.CacheCreationInputTokens, 0)
		}
		dst.CacheReadInputTokens = cacheReadTokens
		dst.CacheCreationInputTokens = src.CacheCreationInputTokens
	} else {
		if src.InputTokens > 0 {
			dst.InputTokens = src.InputTokens
		}
		if src.CacheReadInputTokens > 0 {
			dst.CacheReadInputTokens = src.CacheReadInputTokens
		} else if src.CachedTokens > 0 {
			dst.CacheReadInputTokens = src.CachedTokens
		}
		if src.CacheCreationInputTokens > 0 {
			dst.CacheCreationInputTokens = src.CacheCreationInputTokens
		}
	}
	if src.OutputTokens > 0 {
		dst.OutputTokens = src.OutputTokens
	}
}

func mergeKiroCreditsFromAnthropicPayload(dst *ClaudeUsage, payload string) {
	if dst == nil || payload == "" || !gjson.Valid(payload) {
		return
	}
	if credits := kiroCreditsFromUsageGJSON(gjson.Get(payload, "usage")); credits > 0 {
		dst.KiroCredits = credits
		return
	}
	if credits := kiroCreditsFromUsageGJSON(gjson.Get(payload, "message.usage")); credits > 0 {
		dst.KiroCredits = credits
	}
}

// parseAnthropicSSEField parses an SSE field line in the form "field:value" or "field: value".
// According to the SSE spec (https://html.spec.whatwg.org/multipage/server-sent-events.html#event-stream-interpretation),
// the space after the colon is optional. This function handles both formats.
func parseAnthropicSSEField(line, field string) (string, bool) {
	prefix := field + ":"
	if !strings.HasPrefix(line, prefix) {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(line, prefix)), true
}

// handleResponsesBufferedStreamingResponse reads all Anthropic SSE events from
// the upstream streaming response, assembles them into a complete Anthropic
// response, converts to Responses API JSON format, and writes it to the client.
func (s *GatewayService) handleResponsesBufferedStreamingResponse(
	resp *http.Response,
	c *gin.Context,
	originalModel string,
	mappedModel string,
	reasoningEffort *string,
	startTime time.Time,
	clientToolMapping apicompat.ResponsesClientToolMapping,
) (*ForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")
	finalResp, usage := s.collectAnthropicResponseFromSSE(resp.Body, requestID, nil)

	if finalResp == nil {
		writeResponsesError(c, http.StatusBadGateway, "server_error", "Upstream stream ended without a response")
		return nil, fmt.Errorf("upstream stream ended without response")
	}
	return s.writeResponsesBufferedResult(
		c, resp, finalResp, usage, requestID,
		originalModel, mappedModel, reasoningEffort, startTime, clientToolMapping,
	)
}

// collectAnthropicResponseFromSSE 读完上游 Anthropic SSE 流并聚合成一个完整的
// Anthropic 响应。onEvent 在每个事件解析成功后被调用（可为 nil），供调用方挂载
// 心跳一类的副作用——compact 请求会借此在漫长的摘要生成期间向下游发送保活字节。
func (s *GatewayService) collectAnthropicResponseFromSSE(
	body io.Reader,
	requestID string,
	onEvent func(),
) (*apicompat.AnthropicResponse, ClaudeUsage) {
	scanner := bufio.NewScanner(body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	// Accumulate the final Anthropic response from streaming events
	var finalResp *apicompat.AnthropicResponse
	var usage ClaudeUsage

	for scanner.Scan() {
		line := scanner.Text()
		eventType, ok := parseAnthropicSSEField(line, "event")
		if !ok {
			continue
		}

		// Read the data line
		if !scanner.Scan() {
			break
		}
		dataLine := scanner.Text()
		payload, ok := parseAnthropicSSEField(dataLine, "data")
		if !ok {
			continue
		}

		var event apicompat.AnthropicStreamEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			logger.L().Warn("forward_as_responses buffered: failed to parse event",
				zap.Error(err),
				zap.String("request_id", requestID),
				zap.String("event_type", eventType),
			)
			continue
		}
		if onEvent != nil {
			onEvent()
		}

		// message_start carries the initial response structure
		if event.Type == "message_start" && event.Message != nil {
			finalResp = event.Message
			mergeAnthropicUsage(&usage, event.Message.Usage)
		}

		// message_delta carries final usage and stop_reason
		if event.Type == "message_delta" {
			if event.Usage != nil {
				mergeAnthropicUsage(&usage, *event.Usage)
			}
			mergeKiroCreditsFromAnthropicPayload(&usage, payload)
			if event.Delta != nil && event.Delta.StopReason != "" && finalResp != nil {
				finalResp.StopReason = apicompat.AnthropicStopReasonPtr(event.Delta.StopReason)
			}
		}

		// Accumulate content blocks
		if event.Type == "content_block_start" && event.ContentBlock != nil && finalResp != nil {
			finalResp.Content = append(finalResp.Content, *event.ContentBlock)
		}
		if event.Type == "content_block_delta" && event.Delta != nil && finalResp != nil && event.Index != nil {
			idx := *event.Index
			if idx < len(finalResp.Content) {
				switch event.Delta.Type {
				case "text_delta":
					finalResp.Content[idx].Text += event.Delta.Text
				case "thinking_delta":
					finalResp.Content[idx].Thinking += event.Delta.Thinking
				case "input_json_delta":
					finalResp.Content[idx].Input = appendRawJSON(finalResp.Content[idx].Input, event.Delta.PartialJSON)
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			logger.L().Warn("forward_as_responses buffered: read error",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
		}
	}
	return finalResp, usage
}

// writeResponsesBufferedResult 把聚合好的 Anthropic 响应转成 Responses JSON 写回。
func (s *GatewayService) writeResponsesBufferedResult(
	c *gin.Context,
	resp *http.Response,
	finalResp *apicompat.AnthropicResponse,
	usage ClaudeUsage,
	requestID string,
	originalModel string,
	mappedModel string,
	reasoningEffort *string,
	startTime time.Time,
	clientToolMapping apicompat.ResponsesClientToolMapping,
) (*ForwardResult, error) {
	// Update usage from accumulated delta. 无条件赋值：纯缓存命中的响应
	// （input/output 均为 0 但 cache read/write 非 0）不能被整体丢弃。
	finalResp.Usage = apicompat.AnthropicUsage{
		InputTokens:              usage.InputTokens,
		OutputTokens:             usage.OutputTokens,
		CacheCreationInputTokens: usage.CacheCreationInputTokens,
		CacheReadInputTokens:     usage.CacheReadInputTokens,
	}

	// Convert to Responses format
	responsesResp := apicompat.AnthropicToResponsesResponseWithCustomTools(finalResp, clientToolMapping.CustomTools)
	responsesResp.Model = originalModel // Use original model name

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}
	// 非流式响应必须是 application/json。上游被强制流式后会返回
	// Content-Type: text/event-stream，经 WriteFilteredHeaders 透传后会污染
	// 响应头；而 c.Data/c.JSON 走 Gin 的 writeContentType（仅当头不存在时才设置），
	// 无法覆盖已存在的 SSE 头。这里显式 Set 强制改回 JSON，避免下游中间层
	// （如 new-api）按 Content-Type 误判为流式。
	c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	if respBytes, err := json.Marshal(responsesResp); err == nil {
		respBytes = reverseToolNamesIfPresent(c, respBytes)
		respBytes, _, err = apicompat.RestoreResponsesClientToolPayload(respBytes, clientToolMapping)
		if err != nil {
			return nil, fmt.Errorf("restore responses client tools: %w", err)
		}
		c.Data(http.StatusOK, "application/json; charset=utf-8", respBytes)
	} else {
		c.JSON(http.StatusOK, responsesResp)
	}

	return &ForwardResult{
		RequestID:       requestID,
		UpstreamHeaders: resp.Header,
		Usage:           usage,
		Model:           originalModel,
		UpstreamModel:   mappedModel,
		ReasoningEffort: reasoningEffort,
		Stream:          false,
		Duration:        time.Since(startTime),
	}, nil
}

// handleResponsesStreamingResponse reads Anthropic SSE events from upstream,
// converts each to Responses SSE events, and writes them to the client.
func (s *GatewayService) handleResponsesStreamingResponse(
	resp *http.Response,
	c *gin.Context,
	originalModel string,
	mappedModel string,
	reasoningEffort *string,
	startTime time.Time,
	clientToolMapping apicompat.ResponsesClientToolMapping,
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

	state := apicompat.NewAnthropicEventToResponsesState()
	state.Model = originalModel
	state.CustomTools = clientToolMapping.CustomTools
	clientToolRestorer := apicompat.NewResponsesClientToolStreamRestorer(clientToolMapping)
	var usage ClaudeUsage
	var firstTokenMs *int
	firstChunk := true

	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	resultWithUsage := func() *ForwardResult {
		return &ForwardResult{
			RequestID:       requestID,
			UpstreamHeaders: resp.Header,
			Usage:           usage,
			Model:           originalModel,
			UpstreamModel:   mappedModel,
			ReasoningEffort: reasoningEffort,
			Stream:          true,
			Duration:        time.Since(startTime),
			FirstTokenMs:    firstTokenMs,
		}
	}

	// processEvent handles a single parsed Anthropic SSE event.
	processEvent := func(event *apicompat.AnthropicStreamEvent) bool {
		if firstChunk {
			firstChunk = false
			ms := int(time.Since(startTime).Milliseconds())
			firstTokenMs = &ms
		}

		// Extract usage from message_delta
		if event.Type == "message_delta" && event.Usage != nil {
			mergeAnthropicUsage(&usage, *event.Usage)
		}
		// Also capture usage from message_start
		if event.Type == "message_start" && event.Message != nil {
			mergeAnthropicUsage(&usage, event.Message.Usage)
		}

		// Convert to Responses events
		events := apicompat.AnthropicEventToResponsesEvents(event, state)
		for _, evt := range events {
			payload, err := json.Marshal(evt)
			if err != nil {
				logger.L().Warn("forward_as_responses stream: failed to marshal event",
					zap.Error(err),
					zap.String("request_id", requestID),
				)
				continue
			}
			payload = reverseToolNamesIfPresent(c, payload)
			payloads, _, err := clientToolRestorer.RestoreEvent(payload)
			if err != nil {
				logger.L().Warn("forward_as_responses stream: failed to restore client tools",
					zap.Error(err),
					zap.String("request_id", requestID),
				)
				continue
			}
			for _, restored := range payloads {
				eventType := gjson.GetBytes(restored, "type").String()
				if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", eventType, restored); err != nil {
					logger.L().Info("forward_as_responses stream: client disconnected",
						zap.String("request_id", requestID),
					)
					return true // client disconnected
				}
			}
		}
		if len(events) > 0 {
			c.Writer.Flush()
		}
		return false
	}

	finalizeStream := func() (*ForwardResult, error) {
		if finalEvents := apicompat.FinalizeAnthropicResponsesStream(state); len(finalEvents) > 0 {
			for _, evt := range finalEvents {
				sse, err := apicompat.ResponsesEventToSSE(evt)
				if err != nil {
					continue
				}
				out := string(reverseToolNamesIfPresent(c, []byte(sse)))
				fmt.Fprint(c.Writer, out) //nolint:errcheck
			}
			c.Writer.Flush()
		}
		return resultWithUsage(), nil
	}

	// Read Anthropic SSE events
	for scanner.Scan() {
		line := scanner.Text()
		eventType, ok := parseAnthropicSSEField(line, "event")
		if !ok {
			continue
		}

		// Read data line
		if !scanner.Scan() {
			break
		}
		dataLine := scanner.Text()
		payload, ok := parseAnthropicSSEField(dataLine, "data")
		if !ok {
			continue
		}

		var event apicompat.AnthropicStreamEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			logger.L().Warn("forward_as_responses stream: failed to parse event",
				zap.Error(err),
				zap.String("request_id", requestID),
				zap.String("event_type", eventType),
			)
			continue
		}

		mergeKiroCreditsFromAnthropicPayload(&usage, payload)

		if processEvent(&event) {
			return resultWithUsage(), nil
		}
	}

	if err := scanner.Err(); err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			logger.L().Warn("forward_as_responses stream: read error",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
		}
	}

	return finalizeStream()
}

// appendRawJSON 累积 tool_use 块的 input_json_delta 分片。
//
// Anthropic 协议里 content_block_start 会带一个空对象占位（"input": {}），真实参数
// 随后由 input_json_delta.partial_json 分片流出，消费方必须忽略这个占位。buffered
// 路径把整个 ContentBlock 直接追加进 finalResp.Content，Input 因此已是 "{}"，若按
// 普通分片往后拼就会得到 `{}{"command":"ls"}` 这种非法 JSON——Input 是
// json.RawMessage，不做校验，会原样出到客户端。
//
// 因此把"占位空对象"与"空值"同等对待，让第一个真实分片直接替换它。真流式路径靠
// content_block_start 时 CurrentArgs.Reset() 达到同一效果（见
// anthropic_to_responses_response.go 的状态机），这里是它在 buffered 路径上的对应。
//
// 上游只发占位、不发任何 delta 时（无参工具），existing 保持 "{}" 不变，正是应有的
// 语义——不能退化成空串，否则 Input 的 omitempty 会让 input 字段整个消失。
func appendRawJSON(existing json.RawMessage, fragment string) json.RawMessage {
	if len(existing) == 0 || isEmptyJSONObject(existing) {
		return json.RawMessage(fragment)
	}
	return json.RawMessage(string(existing) + fragment)
}

// isEmptyJSONObject 判断 raw 是否只是一个空 JSON 对象（允许任意位置的空白）。
//
// 只匹配闭合的空对象：分片过程中的中间态是 "{" 或 "{\"cmd" 这类不闭合前缀，不会
// 命中；真正把 {} 拆成 "{" + "}" 两片发的上游，第一片后 existing 是 "{"，同样不
// 命中。所以命中场景只有两种——content_block_start 的占位，或一个完整的空对象
// 分片，两者都应被后续分片替换而非拼接。
//
// 不走 json.Unmarshal：该判断在每个分片上都要做一次，长参数的工具会被调用很多次。
// 这里对真实载荷是 O(1) 退出——内层 TrimSpace 从左侧遇到第一个非空白字符就停。
func isEmptyJSONObject(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) < 2 || trimmed[0] != '{' || trimmed[len(trimmed)-1] != '}' {
		return false
	}
	return len(bytes.TrimSpace(trimmed[1:len(trimmed)-1])) == 0
}

// writeResponsesError writes an error response in OpenAI Responses API format.
func writeResponsesError(c *gin.Context, statusCode int, code, message string) {
	MarkResponseCommitted(c)
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	})
}

// mapUpstreamStatusCode maps upstream HTTP status codes to appropriate client-facing codes.
func mapUpstreamStatusCode(code int) int {
	if code >= 500 {
		return http.StatusBadGateway
	}
	return code
}
