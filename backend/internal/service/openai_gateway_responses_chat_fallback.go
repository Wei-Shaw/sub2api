package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// forwardResponsesViaRawChatCompletions serves /v1/responses clients through an
// upstream that only supports /v1/chat/completions.
func (s *OpenAIGatewayService) forwardResponsesViaRawChatCompletions(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()

	var responsesReq apicompat.ResponsesRequest
	if err := json.Unmarshal(body, &responsesReq); err != nil {
		writeOpenAIResponsesFallbackError(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return nil, fmt.Errorf("parse responses request: %w", err)
	}
	originalModel := strings.TrimSpace(responsesReq.Model)
	if originalModel == "" {
		writeOpenAIResponsesFallbackError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return nil, fmt.Errorf("missing model in request")
	}

	clientStream := responsesReq.Stream
	serviceTier := extractOpenAIServiceTierFromBody(body)
	// custom 工具（如 codex 的 exec）降级为 function 工具转发，回程需按名字还原为
	// custom_tool_call 项，先记下名字集合；tool_search 工具同理，回程还原为
	// tool_search_call 项；namespace 子工具（如 MCP 工具）摊平转发，回程按映射还原
	// 为带 namespace 字段的 function_call 项。
	effectiveTools, err := apicompat.EffectiveResponsesTools(&responsesReq)
	if err != nil {
		writeOpenAIResponsesFallbackError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return nil, fmt.Errorf("resolve responses tools: %w", err)
	}
	customTools := apicompat.CustomToolNames(effectiveTools)
	toolSearch := apicompat.HasToolSearchTool(effectiveTools)
	namespaceTools := apicompat.NamespaceToolNames(effectiveTools)

	// 自愈回写：历史里带明文 summary 的 reasoning item 刷新进缓存，覆盖 Redis
	// 被 flush / 跨实例漂移后同 id 的 encrypted-only 副本无法再取明文的情况。
	s.recacheReasoningItemsFromInput(responsesReq.Input)

	chatReq, err := apicompat.ResponsesToChatCompletionsRequestWithOptions(&responsesReq, &apicompat.ResponsesToChatOptions{
		ReasoningContentByID: s.reasoningContentByID,
	})
	if err != nil {
		writeOpenAIResponsesFallbackError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return nil, fmt.Errorf("convert responses to chat completions: %w", err)
	}

	billingModel := resolveOpenAIForwardModel(account, originalModel, "")
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)
	reasoningEffort := extractOpenAIReasoningEffortFromBody(body, upstreamModel, billingModel, originalModel)
	// 国产模型默认 effort 补充：需要 mappedModel 判定，推迟到 billingModel 算出之后。
	reasoningEffort = ApplyThinkingEnabledFallback(reasoningEffort, body, billingModel)
	chatReq.Model = upstreamModel
	if clientStream {
		chatReq.StreamOptions = &apicompat.ChatStreamOptions{IncludeUsage: true}
	}

	chatBody, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("marshal chat completions fallback request: %w", err)
	}
	chatBody, err = s.applyOpenAIFastPolicyToBody(ctx, account, upstreamModel, chatBody)
	if err != nil {
		var blocked *OpenAIFastBlockedError
		if errors.As(err, &blocked) {
			writeOpenAIFastPolicyBlockedResponse(c, blocked)
		}
		return nil, err
	}
	if serviceTier == nil {
		serviceTier = extractOpenAIServiceTierFromBody(chatBody)
	}

	logger.L().Debug("openai responses: forwarding via raw chat completions",
		zap.Int64("account_id", account.ID),
		zap.String("original_model", originalModel),
		zap.String("billing_model", billingModel),
		zap.String("upstream_model", upstreamModel),
		zap.Bool("stream", clientStream),
	)
	SetOpsUpstreamModel(c, upstreamModel)

	// Build and send upstream request via the shared CC pipeline
	apiKey, targetURL, err := s.resolveCCFallbackTarget(account)
	if err != nil {
		return nil, err
	}
	resp, err := s.sendCCUpstreamRequest(ctx, c, account, targetURL, chatBody, clientStream, apiKey, account.GetOpenAIUserAgent(), "")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody, upstreamMsg := s.readOpenAIUpstreamError(resp)
		if foErr := s.failoverOpenAIUpstreamHTTPError(ctx, c, account, resp, respBody, upstreamMsg, upstreamModel); foErr != nil {
			return nil, foErr
		}
		return s.handleErrorResponse(ctx, resp, c, account, chatBody, billingModel)
	}

	if clientStream {
		return s.streamChatCompletionsAsResponses(c, resp, originalModel, customTools, toolSearch, namespaceTools, billingModel, upstreamModel, reasoningEffort, serviceTier, startTime)
	}
	return s.bufferChatCompletionsAsResponses(c, resp, originalModel, customTools, toolSearch, namespaceTools, billingModel, upstreamModel, reasoningEffort, serviceTier, startTime)
}

func (s *OpenAIGatewayService) bufferChatCompletionsAsResponses(
	c *gin.Context,
	resp *http.Response,
	originalModel string,
	customTools map[string]bool,
	toolSearch bool,
	namespaceTools map[string]apicompat.NamespacedToolName,
	billingModel string,
	upstreamModel string,
	reasoningEffort *string,
	serviceTier *string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")
	ccResp, usage, err := s.readCCUpstreamJSONResponse(c, resp, writeOpenAIResponsesFallbackError)
	if err != nil {
		return nil, err
	}
	responsesResp := apicompat.ChatCompletionsResponseToResponses(ccResp, originalModel, customTools, toolSearch, namespaceTools)
	s.cacheReasoningItemsFromOutput(responsesResp.Output)

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}
	c.JSON(http.StatusOK, responsesResp)

	return &OpenAIForwardResult{
		RequestID:       requestID,
		Usage:           usage,
		Model:           originalModel,
		BillingModel:    billingModel,
		UpstreamModel:   upstreamModel,
		ReasoningEffort: reasoningEffort,
		ServiceTier:     serviceTier,
		Stream:          false,
		Duration:        time.Since(startTime),
	}, nil
}

func (s *OpenAIGatewayService) streamChatCompletionsAsResponses(
	c *gin.Context,
	resp *http.Response,
	originalModel string,
	customTools map[string]bool,
	toolSearch bool,
	namespaceTools map[string]apicompat.NamespacedToolName,
	billingModel string,
	upstreamModel string,
	reasoningEffort *string,
	serviceTier *string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")
	writeStreamHeaders := s.newStreamHeaderWriter(c, resp.Header)

	state := apicompat.NewChatCompletionsToResponsesStreamState(originalModel)
	state.CustomTools = customTools
	state.ToolSearchDeclared = toolSearch
	state.NamespaceTools = namespaceTools
	clientDisconnected := false

	writeEvents := func(events []apicompat.ResponsesStreamEvent) {
		if clientDisconnected || len(events) == 0 {
			return
		}
		writeStreamHeaders()
		for _, event := range events {
			sse, err := apicompat.ResponsesEventToSSE(event)
			if err != nil {
				logger.L().Warn("openai responses chat fallback: failed to marshal stream event",
					zap.Error(err),
					zap.String("request_id", requestID),
				)
				continue
			}
			if _, err := fmt.Fprint(c.Writer, sse); err != nil {
				clientDisconnected = true
				logger.L().Debug("openai responses chat fallback: client disconnected, continuing to drain upstream for billing",
					zap.Error(err),
					zap.String("request_id", requestID),
				)
				return
			}
		}
		c.Writer.Flush()
	}

	scan := s.scanCCStream(resp, "openai responses chat fallback", requestID, startTime, func(chunk *apicompat.ChatCompletionsChunk) {
		events := apicompat.ChatCompletionsChunkToResponsesEvents(chunk, state)
		s.cacheReasoningItemsFromEvents(events)
		writeEvents(events)
	})

	if scan.Err != nil {
		return &OpenAIForwardResult{
			RequestID:       requestID,
			Usage:           scan.Usage,
			Model:           originalModel,
			BillingModel:    billingModel,
			UpstreamModel:   upstreamModel,
			ReasoningEffort: reasoningEffort,
			ServiceTier:     serviceTier,
			Stream:          true,
			Duration:        time.Since(startTime),
			FirstTokenMs:    scan.FirstTokenMs,
		}, fmt.Errorf("stream usage incomplete: %w", scan.Err)
	}

	finalEvents := apicompat.FinalizeChatCompletionsResponsesStream(state)
	s.cacheReasoningItemsFromEvents(finalEvents)
	writeEvents(finalEvents)
	if !clientDisconnected {
		writeStreamHeaders()
		if _, err := fmt.Fprint(c.Writer, "data: [DONE]\n\n"); err != nil {
			clientDisconnected = true
		}
		if !clientDisconnected {
			c.Writer.Flush()
		}
	}
	if !scan.SawDone {
		logCCStreamMissingDoneSentinel("openai responses chat fallback", requestID)
	}

	return &OpenAIForwardResult{
		RequestID:       requestID,
		Usage:           scan.Usage,
		Model:           originalModel,
		BillingModel:    billingModel,
		UpstreamModel:   upstreamModel,
		ReasoningEffort: reasoningEffort,
		ServiceTier:     serviceTier,
		Stream:          true,
		Duration:        time.Since(startTime),
		FirstTokenMs:    scan.FirstTokenMs,
	}, nil
}

func chatChunkStartsResponsesOutput(chunk *apicompat.ChatCompletionsChunk) bool {
	if chunk == nil {
		return false
	}
	for _, choice := range chunk.Choices {
		if choice.Delta.Content != nil || choice.Delta.ReasoningContent != nil || len(choice.Delta.ToolCalls) > 0 {
			return true
		}
	}
	return false
}

// responsesReasoningCacheTTL 是 reasoning 缓存（按 reasoning item id）的过期时间。
// Codex 会话可能跨多天恢复历史，取 7 天。
const responsesReasoningCacheTTL = 7 * 24 * time.Hour

// reasoningContentByID 按 reasoning item id 回查缓存的 reasoning 全文，供
// Responses→CC 桥接在客户端不回传明文 summary（encrypted-only reasoning
// item）时回注 reasoning_content。任何失败都 fail-open 返回 ""（维持桥接原
// 行为），因为缓存只是优化而非正确性前提。
func (s *OpenAIGatewayService) reasoningContentByID(itemID string) string {
	if s == nil || s.cache == nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	content, err := s.cache.GetReasoningContent(ctx, itemID)
	if err != nil {
		return ""
	}
	return content
}

// recacheReasoningItemsFromInput 把请求历史里带明文 summary 的 reasoning item
// 重新写入缓存（best-effort）。Codex 多数时候会原样回传明文 summary，借机
// 刷新 TTL 并自愈 Redis 被 flush / 跨实例漂移造成的缓存缺失。
func (s *OpenAIGatewayService) recacheReasoningItemsFromInput(inputRaw json.RawMessage) {
	if s == nil || s.cache == nil {
		return
	}
	inputRaw = bytes.TrimSpace(inputRaw)
	if len(inputRaw) == 0 || inputRaw[0] != '[' {
		return
	}
	var items []json.RawMessage
	if err := json.Unmarshal(inputRaw, &items); err != nil {
		return
	}
	for _, raw := range items {
		id, text, ok := apicompat.ExtractResponsesReasoningItem(raw)
		if !ok || id == "" || text == "" {
			continue
		}
		s.setReasoningContent(id, text)
	}
}

// cacheReasoningItemsFromEvents 从 Responses 流事件里提取完成的 reasoning
// item 写入缓存（覆盖一个流中的多个 reasoning item）。
func (s *OpenAIGatewayService) cacheReasoningItemsFromEvents(events []apicompat.ResponsesStreamEvent) {
	for _, event := range events {
		if event.Type != "response.output_item.done" || event.Item == nil {
			continue
		}
		s.cacheReasoningItem(event.Item)
	}
}

// cacheReasoningItemsFromOutput 从非流式 Responses 响应的 output 里提取
// reasoning item 写入缓存。
func (s *OpenAIGatewayService) cacheReasoningItemsFromOutput(output []apicompat.ResponsesOutput) {
	for i := range output {
		s.cacheReasoningItem(&output[i])
	}
}

func (s *OpenAIGatewayService) cacheReasoningItem(item *apicompat.ResponsesOutput) {
	if item == nil || item.Type != "reasoning" || item.ID == "" {
		return
	}
	var parts []string
	for _, sum := range item.Summary {
		if t := strings.TrimSpace(sum.Text); t != "" {
			parts = append(parts, t)
		}
	}
	if len(parts) == 0 {
		return
	}
	s.setReasoningContent(item.ID, strings.Join(parts, "\n"))
}

// setReasoningContent 写入缓存，使用 detached ctx：客户端断连后仍在 drain
// 上游流（计费需要），此时的 reasoning 也是后续轮次回注所依赖的，不能随
// 请求 ctx 一起取消。失败仅记日志，不影响转发。
func (s *OpenAIGatewayService) setReasoningContent(itemID, content string) {
	if s == nil || s.cache == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.cache.SetReasoningContent(ctx, itemID, content, responsesReasoningCacheTTL); err != nil {
		logger.L().Warn("openai responses chat fallback: cache reasoning content failed",
			zap.Error(err),
			zap.String("item_id", itemID),
		)
	}
}
