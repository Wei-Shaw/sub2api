package service

// 国产供应商 Anthropic 协议账号的 CC 入站反向路径。
//
// 客户端说 OpenAI Chat Completions、上游是供应商原生 Anthropic 端点
// （api_protocol=anthropic）时的交叉组合：请求 CC→Responses→Anthropic 转换，
// 响应 Anthropic→Responses→CC 转换。转换链与 Anthropic 平台的
// gateway_forward_as_chat_completions.go 完全一致（复用同一组 apicompat
// 状态机），仅上游发送/错误处理对齐 OpenAI 网关语义。

import (
	"bufio"
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

// forwardChatCompletionsViaNativeAnthropic serves OpenAI /v1/chat/completions
// clients through a CN provider's native Anthropic endpoint.
//
// Conversion chain:
//
//	Request:  Chat Completions → Responses → Anthropic (chained)
//	Response: Anthropic events → Responses events → CC chunks (chained state machines)
func (s *OpenAIGatewayService) forwardChatCompletionsViaNativeAnthropic(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	defaultMappedModel string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()

	// 1. Parse Chat Completions request
	var ccReq apicompat.ChatCompletionsRequest
	if err := json.Unmarshal(body, &ccReq); err != nil {
		writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return nil, fmt.Errorf("parse chat completions request: %w", err)
	}
	originalModel := ccReq.Model
	if strings.TrimSpace(originalModel) == "" {
		writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return nil, fmt.Errorf("missing model in request")
	}
	clientStream := ccReq.Stream
	includeUsage := ccReq.StreamOptions != nil && ccReq.StreamOptions.IncludeUsage

	// 2. Convert CC → Responses → Anthropic (chained conversion)
	responsesReq, err := apicompat.ChatCompletionsToResponses(&ccReq)
	if err != nil {
		writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", "Failed to convert request")
		return nil, fmt.Errorf("convert chat completions to responses: %w", err)
	}
	anthropicReq, err := apicompat.ResponsesToAnthropicRequest(responsesReq)
	if err != nil {
		writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", "Failed to convert request")
		return nil, fmt.Errorf("convert responses to anthropic: %w", err)
	}

	// 3. Model mapping（OpenAI 网关统一入口的映射语义）
	billingModel := resolveOpenAIForwardModel(account, originalModel, defaultMappedModel)
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)
	anthropicReq.Model = upstreamModel

	// 4. Force upstream streaming（客户端原始终决定响应格式；
	// 上游恒为流式，非流式由缓冲路径组装）。
	anthropicReq.Stream = true
	reqStream := true

	logger.L().Debug("openai chat_completions: forwarding via native anthropic endpoint",
		zap.Int64("account_id", account.ID),
		zap.String("original_model", originalModel),
		zap.String("billing_model", billingModel),
		zap.String("upstream_model", upstreamModel),
		zap.Bool("client_stream", clientStream),
	)

	anthropicBody, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("marshal anthropic request: %w", err)
	}

	// 与 /v1/messages 直通路径相同的 pre-filter。
	anthropicBody = StripEmptyTextBlocks(anthropicBody)
	anthropicBody = FilterWebSearchHistoryBlocks(anthropicBody, upstreamModel)
	anthropicBody = enforceCacheControlLimit(anthropicBody)

	apiKey := strings.TrimSpace(account.GetOpenAIProtocolAPIKey())
	if apiKey == "" {
		return nil, fmt.Errorf("account %d missing api_key", account.ID)
	}
	targetURL, err := s.nativeAnthropicTargetURL(account)
	if err != nil {
		return nil, err
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	upstreamCtx, releaseUpstreamCtx := detachStreamUpstreamContext(ctx, reqStream)
	upstreamReq, _, err := s.buildNativeAnthropicUpstreamRequest(upstreamCtx, c, account, anthropicBody, apiKey, targetURL)
	releaseUpstreamCtx()
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}

	resp, err := s.doOpenAIUpstream(upstreamReq, proxyURL, account)
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, true)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody, upstreamMsg := s.readOpenAIUpstreamError(resp)
		if foErr := s.failoverOpenAIUpstreamHTTPError(ctx, c, account, resp, respBody, upstreamMsg, upstreamModel); foErr != nil {
			return nil, foErr
		}
		writeChatCompletionsError(c, mapUpstreamStatusCode(resp.StatusCode), "server_error", upstreamMsg)
		return nil, fmt.Errorf("upstream error: %d %s", resp.StatusCode, upstreamMsg)
	}

	reasoningEffort := extractCCReasoningEffortFromBody(body, upstreamModel, billingModel, originalModel)
	reasoningEffort = ApplyThinkingEnabledFallback(reasoningEffort, body, billingModel)

	if clientStream {
		return s.handleCCStreamingFromNativeAnthropic(resp, c, originalModel, billingModel, upstreamModel, reasoningEffort, startTime, includeUsage)
	}
	return s.handleCCBufferedFromNativeAnthropic(resp, c, originalModel, billingModel, upstreamModel, reasoningEffort, startTime)
}

// handleCCBufferedFromNativeAnthropic reads Anthropic SSE events, assembles the
// full response, then converts Anthropic → Responses → Chat Completions.
func (s *OpenAIGatewayService) handleCCBufferedFromNativeAnthropic(
	resp *http.Response,
	c *gin.Context,
	originalModel string,
	billingModel string,
	upstreamModel string,
	reasoningEffort *string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")

	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	var finalResp *apicompat.AnthropicResponse
	var usage ClaudeUsage

	// 读间隔上限：上游挂住 SSE 时中止组装（缓冲路径尚未提交响应头，可回 502）。
	streamInterval := s.anthropicNativeStreamInterval()
	pump := newAnthropicNativeLinePump(scanner, streamInterval)
	defer pump.stop()

	logReadErr := func(err error) {
		if !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			logger.L().Warn("openai cc via native anthropic buffered: read error",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
		}
	}
	onIdle := func() (*OpenAIForwardResult, error) {
		_ = resp.Body.Close()
		logger.L().Warn("openai cc via native anthropic buffered: data interval timeout",
			zap.String("request_id", requestID),
			zap.Duration("interval", streamInterval),
		)
		writeChatCompletionsError(c, http.StatusBadGateway, "server_error", "Upstream stream data interval timeout")
		return nil, fmt.Errorf("stream data interval timeout")
	}

	for {
		line, rerr := pump.next()
		if rerr != nil {
			if errors.Is(rerr, errAnthropicNativeStreamIdle) {
				return onIdle()
			}
			logReadErr(rerr)
			break
		}
		// SSE 规范允许 `event:xxx`（冒号后无空格）：Kimi 等 Anthropic 兼容上游
		// 返回紧凑格式，严格匹配 "event: " 会丢弃全部事件（#4653 同根因）。
		if _, ok := extractOpenAISSEEventLine(line); !ok {
			continue
		}

		dataLine, rerr := pump.next()
		if rerr != nil {
			if errors.Is(rerr, errAnthropicNativeStreamIdle) {
				return onIdle()
			}
			logReadErr(rerr)
			break
		}
		payload, ok := extractOpenAISSEDataLine(dataLine)
		if !ok {
			continue
		}

		var event apicompat.AnthropicStreamEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			continue
		}

		if event.Type == "message_start" && event.Message != nil {
			finalResp = event.Message
			mergeAnthropicUsage(&usage, event.Message.Usage)
		}
		if event.Type == "message_delta" {
			if event.Usage != nil {
				mergeAnthropicUsage(&usage, *event.Usage)
			}
			if event.Delta != nil && event.Delta.StopReason != "" && finalResp != nil {
				finalResp.StopReason = apicompat.AnthropicStopReasonPtr(event.Delta.StopReason)
			}
		}
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

	if finalResp == nil {
		writeChatCompletionsError(c, http.StatusBadGateway, "server_error", "Upstream stream ended without a response")
		return nil, fmt.Errorf("upstream stream ended without response")
	}

	if usage.InputTokens > 0 || usage.OutputTokens > 0 {
		finalResp.Usage = apicompat.AnthropicUsage{
			InputTokens:              usage.InputTokens,
			OutputTokens:             usage.OutputTokens,
			CacheCreationInputTokens: usage.CacheCreationInputTokens,
			CacheReadInputTokens:     usage.CacheReadInputTokens,
		}
	}

	responsesResp := apicompat.AnthropicToResponsesResponse(finalResp)
	ccResp := apicompat.ResponsesToChatCompletions(responsesResp, originalModel)

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}
	// 非流式响应必须是 application/json（上游被强制流式，透传头会污染）。
	c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	if respBytes, err := json.Marshal(ccResp); err == nil {
		respBytes = reverseToolNamesIfPresent(c, respBytes)
		c.Data(http.StatusOK, "application/json; charset=utf-8", respBytes)
	} else {
		c.JSON(http.StatusOK, ccResp)
	}

	return &OpenAIForwardResult{
		RequestID:        requestID,
		Usage:            claudeUsageToOpenAIUsage(&usage),
		Model:            originalModel,
		BillingModel:     billingModel,
		UpstreamModel:    upstreamModel,
		UpstreamEndpoint: "/v1/messages",
		ReasoningEffort:  reasoningEffort,
		Stream:           false,
		Duration:         time.Since(startTime),
	}, nil
}

// handleCCStreamingFromNativeAnthropic reads Anthropic SSE events, converts each
// to Responses events, then to Chat Completions chunks, and writes them.
func (s *OpenAIGatewayService) handleCCStreamingFromNativeAnthropic(
	resp *http.Response,
	c *gin.Context,
	originalModel string,
	billingModel string,
	upstreamModel string,
	reasoningEffort *string,
	startTime time.Time,
	includeUsage bool,
) (*OpenAIForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	anthState := apicompat.NewAnthropicEventToResponsesState()
	anthState.Model = originalModel
	ccState := apicompat.NewResponsesEventToChatState()
	ccState.Model = originalModel
	ccState.IncludeUsage = includeUsage

	var usage ClaudeUsage
	var firstTokenMs *int
	firstChunk := true
	clientDisconnected := false

	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	resultWithUsage := func() *OpenAIForwardResult {
		return &OpenAIForwardResult{
			RequestID:        requestID,
			Usage:            claudeUsageToOpenAIUsage(&usage),
			Model:            originalModel,
			BillingModel:     billingModel,
			UpstreamModel:    upstreamModel,
			UpstreamEndpoint: "/v1/messages",
			ReasoningEffort:  reasoningEffort,
			Stream:           true,
			Duration:         time.Since(startTime),
			FirstTokenMs:     firstTokenMs,
			ClientDisconnect: clientDisconnected,
		}
	}

	// 读间隔上限：上游挂住 SSE（不发数据也不断连）时结束排水。上游 ctx 为
	// WithoutCancel 且 http.Client 无整体 Timeout，无此界限则客户端断开后
	// scanner.Scan() 永久阻塞（见 anthropic native pump 文件注释）。
	streamInterval := s.anthropicNativeStreamInterval()
	pump := newAnthropicNativeLinePump(scanner, streamInterval)
	defer pump.stop()

	logReadErr := func(err error) {
		if !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			logger.L().Warn("openai cc via native anthropic stream: read error",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
		}
	}
	// onIdle 关闭上游连接（解除阻塞的读、归还连接池位），并按已累计 usage
	// 返回——与 messages 主路径 "stream usage incomplete after timeout" 同语义。
	onIdle := func() (*OpenAIForwardResult, error) {
		_ = resp.Body.Close()
		if !clientDisconnected {
			logger.L().Warn("openai cc via native anthropic stream: data interval timeout",
				zap.String("request_id", requestID),
				zap.Duration("interval", streamInterval),
			)
		}
		return resultWithUsage(), fmt.Errorf("stream data interval timeout")
	}

	writeChunk := func(chunk apicompat.ChatCompletionsChunk) bool {
		if clientDisconnected {
			return false // 已断开：不再写客户端，只排水上游累计 usage
		}
		sse, err := apicompat.ChatChunkToSSE(chunk)
		if err != nil {
			return false
		}
		out := string(reverseToolNamesIfPresent(c, []byte(sse)))
		if _, err := fmt.Fprint(c.Writer, out); err != nil {
			clientDisconnected = true
			return false
		}
		return false
	}

	processAnthropicEvent := func(event *apicompat.AnthropicStreamEvent) bool {
		if firstChunk {
			firstChunk = false
			ms := int(time.Since(startTime).Milliseconds())
			firstTokenMs = &ms
		}

		// usage 恒累计（含客户端断开后的排水阶段，payg 上游照常计费）。
		if event.Type == "message_delta" && event.Usage != nil {
			mergeAnthropicUsage(&usage, *event.Usage)
		}
		if event.Type == "message_start" && event.Message != nil {
			mergeAnthropicUsage(&usage, event.Message.Usage)
		}

		// 客户端已断开：跳过转换与写出，继续读上游直到流结束（usage 完整、
		// 连接及时归还），不再提前 return。
		if clientDisconnected {
			return false
		}

		responsesEvents := apicompat.AnthropicEventToResponsesEvents(event, anthState)
		for _, resEvt := range responsesEvents {
			ccChunks := apicompat.ResponsesEventToChatChunks(&resEvt, ccState)
			for _, chunk := range ccChunks {
				writeChunk(chunk)
			}
		}
		if len(responsesEvents) > 0 {
			c.Writer.Flush()
		}
		return false
	}

	for {
		line, rerr := pump.next()
		if rerr != nil {
			if errors.Is(rerr, errAnthropicNativeStreamIdle) {
				return onIdle()
			}
			logReadErr(rerr)
			break
		}
		if _, ok := extractOpenAISSEEventLine(line); !ok {
			continue
		}

		dataLine, rerr := pump.next()
		if rerr != nil {
			if errors.Is(rerr, errAnthropicNativeStreamIdle) {
				return onIdle()
			}
			// EOF / 读错误：事件行后流终止，进入 finalize。
			logReadErr(rerr)
			break
		}
		payload, ok := extractOpenAISSEDataLine(dataLine)
		if !ok {
			continue
		}

		var event apicompat.AnthropicStreamEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			continue
		}

		if processAnthropicEvent(&event) {
			return resultWithUsage(), nil
		}
	}

	// Finalize both state machines（客户端已断开时仍执行，保证 usage 汇总完整）。
	finalResEvents := apicompat.FinalizeAnthropicResponsesStream(anthState)
	for _, resEvt := range finalResEvents {
		ccChunks := apicompat.ResponsesEventToChatChunks(&resEvt, ccState)
		for _, chunk := range ccChunks {
			writeChunk(chunk) //nolint:errcheck
		}
	}
	finalCCChunks := apicompat.FinalizeResponsesChatStream(ccState)
	for _, chunk := range finalCCChunks {
		writeChunk(chunk) //nolint:errcheck
	}

	if !clientDisconnected {
		fmt.Fprint(c.Writer, "data: [DONE]\n\n") //nolint:errcheck
		c.Writer.Flush()
	}

	return resultWithUsage(), nil
}
