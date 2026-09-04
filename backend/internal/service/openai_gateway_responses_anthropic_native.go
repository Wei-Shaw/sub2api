package service

// 国产供应商 Anthropic 协议账号的 Responses 入站反向路径。
//
// 客户端说 OpenAI Responses（/v1/responses，Codex 等）、上游是供应商原生
// Anthropic 端点（api_protocol=anthropic）时的交叉组合：请求 Responses→Anthropic
// 单次转换，响应 Anthropic 事件→Responses 事件转换。转换链与 Anthropic 平台的
// gateway_forward_as_responses.go 完全一致（复用同一组 apicompat 状态机），仅上游
// 发送/错误处理对齐 OpenAI 网关语义（模型映射、failover、transport error）。

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
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

// forwardResponsesViaNativeAnthropic serves OpenAI /v1/responses clients through
// a CN provider's native Anthropic endpoint.
//
// Conversion chain:
//
//	Request:  Responses → Anthropic (single conversion)
//	Response: Anthropic events → Responses events (stream state machine)
func (s *OpenAIGatewayService) forwardResponsesViaNativeAnthropic(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	defaultMappedModel string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()

	// 1. Lower Codex client-side tools to function tools understood by Anthropic.
	adaptedBody, clientToolMapping, err := adaptResponsesClientToolsForAnthropic(body)
	if err != nil {
		writeResponsesError(c, http.StatusBadRequest, "invalid_request_error", "Failed to adapt request tools")
		return nil, fmt.Errorf("adapt responses client tools: %w", err)
	}

	// 2. Parse Responses request
	var responsesReq apicompat.ResponsesRequest
	if err := json.Unmarshal(adaptedBody, &responsesReq); err != nil {
		writeResponsesError(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return nil, fmt.Errorf("parse responses request: %w", err)
	}
	originalModel := responsesReq.Model
	if strings.TrimSpace(originalModel) == "" {
		writeResponsesError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return nil, fmt.Errorf("missing model in request")
	}
	clientStream := responsesReq.Stream

	// 3. Convert Responses → Anthropic
	anthropicReq, err := apicompat.ResponsesToAnthropicRequest(&responsesReq)
	if err != nil {
		writeResponsesError(c, http.StatusBadRequest, "invalid_request_error", "Failed to convert request")
		return nil, fmt.Errorf("convert responses to anthropic: %w", err)
	}

	// 4. Model mapping（OpenAI 网关统一入口的映射语义）
	billingModel := resolveOpenAIForwardModel(account, originalModel, defaultMappedModel)
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)
	anthropicReq.Model = upstreamModel

	reasoningEffort := ExtractResponsesReasoningEffortFromBody(body, upstreamModel, billingModel, originalModel)
	reasoningEffort = ApplyThinkingEnabledFallback(reasoningEffort, body, billingModel)

	// 5. Force upstream streaming（客户端原始终决定响应格式；
	// 上游恒为流式，非流式由缓冲路径组装）。
	anthropicReq.Stream = true
	reqStream := true

	logger.L().Debug("openai responses: forwarding via native anthropic endpoint",
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
		writeResponsesError(c, mapUpstreamStatusCode(resp.StatusCode), "server_error", upstreamMsg)
		return nil, fmt.Errorf("upstream error: %d %s", resp.StatusCode, upstreamMsg)
	}

	if clientStream {
		return s.handleResponsesStreamingFromNativeAnthropic(resp, c, originalModel, billingModel, upstreamModel, reasoningEffort, startTime, clientToolMapping)
	}
	return s.handleResponsesBufferedFromNativeAnthropic(resp, c, originalModel, billingModel, upstreamModel, reasoningEffort, startTime, clientToolMapping)
}

// handleResponsesBufferedFromNativeAnthropic reads Anthropic SSE events, assembles
// the full response, then converts Anthropic → Responses.
func (s *OpenAIGatewayService) handleResponsesBufferedFromNativeAnthropic(
	resp *http.Response,
	c *gin.Context,
	originalModel string,
	billingModel string,
	upstreamModel string,
	reasoningEffort *string,
	startTime time.Time,
	clientToolMapping apicompat.ResponsesClientToolMapping,
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
			logger.L().Warn("openai responses via native anthropic buffered: read error",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
		}
	}
	onIdle := func() (*OpenAIForwardResult, error) {
		_ = resp.Body.Close()
		logger.L().Warn("openai responses via native anthropic buffered: data interval timeout",
			zap.String("request_id", requestID),
			zap.Duration("interval", streamInterval),
		)
		writeResponsesError(c, http.StatusBadGateway, "server_error", "Upstream stream data interval timeout")
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
		// SSE 规范允许 `event:xxx`（冒号后无空格）：Kimi 等上游返回紧凑格式。
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
		writeResponsesError(c, http.StatusBadGateway, "server_error", "Upstream stream ended without a response")
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
	responsesResp.Model = originalModel

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}
	// 非流式响应必须是 application/json（上游被强制流式，透传头会污染）。
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

// handleResponsesStreamingFromNativeAnthropic reads Anthropic SSE events, converts
// each to Responses SSE events, and writes them to the client.
func (s *OpenAIGatewayService) handleResponsesStreamingFromNativeAnthropic(
	resp *http.Response,
	c *gin.Context,
	originalModel string,
	billingModel string,
	upstreamModel string,
	reasoningEffort *string,
	startTime time.Time,
	clientToolMapping apicompat.ResponsesClientToolMapping,
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

	state := apicompat.NewAnthropicEventToResponsesState()
	state.Model = originalModel
	clientToolRestorer := apicompat.NewResponsesClientToolStreamRestorer(clientToolMapping)

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

	// 读间隔上限：上游挂住 SSE（不发数据也不断连）时结束转换循环。上游 ctx 为
	// WithoutCancel 且 http.Client 无整体 Timeout，无此界限则 scanner.Scan()
	// 永久阻塞（见 anthropic native pump 文件注释）。
	streamInterval := s.anthropicNativeStreamInterval()
	pump := newAnthropicNativeLinePump(scanner, streamInterval)
	defer pump.stop()

	logReadErr := func(err error) {
		if !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			logger.L().Warn("openai responses via native anthropic stream: read error",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
		}
	}
	onIdle := func() (*OpenAIForwardResult, error) {
		_ = resp.Body.Close()
		logger.L().Warn("openai responses via native anthropic stream: data interval timeout",
			zap.String("request_id", requestID),
			zap.Duration("interval", streamInterval),
		)
		return resultWithUsage(), fmt.Errorf("stream data interval timeout")
	}

	// 与 CC 姊妹路径（handleCCStreamingFromNativeAnthropic.writeChunk）同语义：
	// 客户端断开后不再写出，但继续排水上游至流自然结束——Anthropic 的最终
	// output_tokens 只在末尾 message_delta 携带，提前退出会把整段生成记成 ~1
	// token，payg 上游照常计费而平台漏记。状态机照常推进以保证 finalize 一致。
	processAnthropicEvent := func(event *apicompat.AnthropicStreamEvent) {
		if firstChunk {
			firstChunk = false
			ms := int(time.Since(startTime).Milliseconds())
			firstTokenMs = &ms
		}

		if event.Type == "message_delta" && event.Usage != nil {
			mergeAnthropicUsage(&usage, *event.Usage)
		}
		if event.Type == "message_start" && event.Message != nil {
			mergeAnthropicUsage(&usage, event.Message.Usage)
		}

		events := apicompat.AnthropicEventToResponsesEvents(event, state)
		if clientDisconnected {
			return
		}
		for _, evt := range events {
			payload, err := json.Marshal(evt)
			if err != nil {
				continue
			}
			payload = reverseToolNamesIfPresent(c, payload)
			payloads, _, err := clientToolRestorer.RestoreEvent(payload)
			if err != nil {
				continue
			}
			for _, restored := range payloads {
				eventType := gjson.GetBytes(restored, "type").String()
				if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", eventType, restored); err != nil {
					clientDisconnected = true
					return
				}
			}
		}
		if len(events) > 0 {
			c.Writer.Flush()
		}
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

		processAnthropicEvent(&event)
	}

	// Finalize state machine（客户端已断开时仍推进，保证 usage 汇总完整；仅在
	// 客户端仍连接时写出）。终态帧与逐事件路径一致过工具名反转与客户端工具还原，
	// 避免流截断时终态帧携带改写后的工具名。
	if finalEvents := apicompat.FinalizeAnthropicResponsesStream(state); len(finalEvents) > 0 && !clientDisconnected {
		wrote := false
		for _, evt := range finalEvents {
			payload, err := json.Marshal(evt)
			if err != nil {
				continue
			}
			payload = reverseToolNamesIfPresent(c, payload)
			payloads, _, err := clientToolRestorer.RestoreEvent(payload)
			if err != nil {
				continue
			}
			for _, restored := range payloads {
				eventType := gjson.GetBytes(restored, "type").String()
				if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", eventType, restored); err != nil {
					clientDisconnected = true
					break
				}
				wrote = true
			}
			if clientDisconnected {
				break
			}
		}
		if wrote {
			c.Writer.Flush()
		}
	}

	return resultWithUsage(), nil
}
