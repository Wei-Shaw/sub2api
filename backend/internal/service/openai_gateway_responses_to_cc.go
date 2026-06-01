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
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

type responsesAsChatCompletionsForwardRequest struct {
	originalModel   string
	clientStream    bool
	reasoningEffort *string
	serviceTier     *string
	billingModel    string
	upstreamModel   string
	ccBody          []byte
	targetURL       string
	apiKey          string
}

func (s *OpenAIGatewayService) prepareResponsesAsChatCompletionsRequest(account *Account, body []byte, forceStream bool) (*responsesAsChatCompletionsForwardRequest, error) {
	var responsesReq apicompat.ResponsesRequest
	if err := json.Unmarshal(body, &responsesReq); err != nil {
		return nil, fmt.Errorf("parse responses request: %w", err)
	}
	originalModel := strings.TrimSpace(responsesReq.Model)
	if originalModel == "" {
		return nil, fmt.Errorf("model is required")
	}
	if forceStream {
		responsesReq.Stream = true
	}
	clientStream := responsesReq.Stream
	reasoningEffort := ExtractResponsesReasoningEffortFromBody(body)
	var serviceTier *string
	if responsesReq.ServiceTier != "" {
		st := responsesReq.ServiceTier
		serviceTier = &st
	}

	ccReq, err := apicompat.ResponsesToChatCompletionsRequestWithOptions(&responsesReq, apicompat.ConvertResponsesOptions{
		StripReasoningEffort: account.StripReasoningEffortOnCC,
	})
	if err != nil {
		return nil, fmt.Errorf("convert responses to chat completions: %w", err)
	}

	billingModel := resolveOpenAIForwardModel(account, originalModel, "")
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)
	ccReq.Model = upstreamModel

	ccBody, err := json.Marshal(ccReq)
	if err != nil {
		return nil, fmt.Errorf("marshal chat completions request: %w", err)
	}
	if clientStream {
		ccBody, err = ensureOpenAIChatStreamUsage(ccBody)
		if err != nil {
			return nil, fmt.Errorf("enable stream usage: %w", err)
		}
	}

	apiKey := account.GetUpstreamAPIKey()
	if apiKey == "" {
		return nil, fmt.Errorf("account %d missing api_key", account.ID)
	}
	targetURL := account.GetOpenAIChatCompletionsURL()
	if targetURL == "" {
		return nil, fmt.Errorf("account %d missing chat_completions_url", account.ID)
	}
	if _, err := s.validateUpstreamBaseURL(targetURL); err != nil {
		return nil, fmt.Errorf("invalid chat_completions_url: %w", err)
	}

	return &responsesAsChatCompletionsForwardRequest{
		originalModel:   originalModel,
		clientStream:    clientStream,
		reasoningEffort: reasoningEffort,
		serviceTier:     serviceTier,
		billingModel:    billingModel,
		upstreamModel:   upstreamModel,
		ccBody:          ccBody,
		targetURL:       targetURL,
		apiKey:          apiKey,
	}, nil
}

// ForwardResponsesAsChatCompletions accepts an OpenAI Responses API request
// and forwards it to a Chat-Completions-only upstream by:
//
//  1. Parsing the body as ResponsesRequest
//  2. Converting to ChatCompletionsRequest via apicompat.ResponsesToChatCompletionsRequest
//  3. POSTing the CC body to account.GetOpenAIChatCompletionsURL()
//  4. Converting the upstream CC response back to Responses format:
//     - non-stream: ChatCompletionsResponse → ResponsesResponse JSON
//     - stream: per-chunk SSE conversion via apicompat.ConvertChatCompletionsSSEChunkToResponsesEvents
//
// Adapter for `apikey-chat-completions` accounts hit through the /v1/responses
// or /responses entry. Mirrors the inverse of ForwardAsChatCompletions which
// handles CC requests targeting Responses upstreams.
func (s *OpenAIGatewayService) ForwardResponsesAsChatCompletions(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()
	forwardReq, err := s.prepareResponsesAsChatCompletionsRequest(account, body, false)
	if err != nil {
		if c != nil {
			writeResponsesError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		}
		return nil, err
	}
	originalModel := forwardReq.originalModel
	clientStream := forwardReq.clientStream
	reasoningEffort := forwardReq.reasoningEffort
	serviceTier := forwardReq.serviceTier
	billingModel := forwardReq.billingModel
	upstreamModel := forwardReq.upstreamModel
	ccBody := forwardReq.ccBody
	targetURL := forwardReq.targetURL
	apiKey := forwardReq.apiKey

	logger.L().Debug("openai responses→cc: forwarding",
		zap.Int64("account_id", account.ID),
		zap.String("original_model", originalModel),
		zap.String("billing_model", billingModel),
		zap.String("upstream_model", upstreamModel),
		zap.String("upstream_url", targetURL),
		zap.Bool("stream", clientStream),
	)

	// 6. Build upstream request
	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	upstreamReq, err := http.NewRequestWithContext(upstreamCtx, http.MethodPost, targetURL, bytes.NewReader(ccBody))
	releaseUpstreamCtx()
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}
	upstreamReq.Header.Set("Content-Type", "application/json")
	applyOpenAICompatibleAPIKeyAuth(upstreamReq, account, apiKey)
	if clientStream {
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
	if customUA := account.GetOpenAIUserAgent(); customUA != "" {
		upstreamReq.Header.Set("user-agent", customUA)
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
		writeResponsesError(c, http.StatusBadGateway, "server_error", "Upstream request failed")
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
	}
	defer func() { _ = resp.Body.Close() }()

	// 8. Error handling
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))

		upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
		upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)

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
		writeResponsesError(c, mapUpstreamStatusCode(resp.StatusCode), "server_error", upstreamMsg)
		return nil, fmt.Errorf("upstream error: %d %s", resp.StatusCode, upstreamMsg)
	}

	// 9. Forward response
	if clientStream {
		return s.streamResponsesFromCC(c, resp, originalModel, billingModel, upstreamModel, reasoningEffort, serviceTier, startTime)
	}
	return s.bufferResponsesFromCC(c, resp, originalModel, billingModel, upstreamModel, reasoningEffort, serviceTier, startTime)
}

// bufferResponsesFromCC reads the upstream non-streaming Chat Completions JSON
// response, converts it to a Responses API JSON response, and writes it to the
// client.
func (s *OpenAIGatewayService) bufferResponsesFromCC(
	c *gin.Context,
	resp *http.Response,
	originalModel string,
	billingModel string,
	upstreamModel string,
	reasoningEffort *string,
	serviceTier *string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")

	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		if !errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
			writeResponsesError(c, http.StatusBadGateway, "server_error", "Failed to read upstream response")
		}
		return nil, fmt.Errorf("read upstream body: %w", err)
	}

	var ccResp apicompat.ChatCompletionsResponse
	if err := json.Unmarshal(respBody, &ccResp); err != nil {
		writeResponsesError(c, http.StatusBadGateway, "server_error", "Invalid upstream response")
		return nil, fmt.Errorf("decode chat completions response: %w", err)
	}

	var usage OpenAIUsage
	if ccResp.Usage != nil {
		usage = OpenAIUsage{
			InputTokens:  ccResp.Usage.PromptTokens,
			OutputTokens: ccResp.Usage.CompletionTokens,
		}
		if ccResp.Usage.PromptTokensDetails != nil {
			usage.CacheReadInputTokens = ccResp.Usage.PromptTokensDetails.CachedTokens
		}
	}

	responsesResp := apicompat.ChatCompletionsToResponsesResponse(&ccResp, originalModel)

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}
	out, err := json.Marshal(responsesResp)
	if err != nil {
		writeResponsesError(c, http.StatusBadGateway, "server_error", "Failed to marshal response")
		return nil, fmt.Errorf("marshal responses response: %w", err)
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", out)

	return &OpenAIForwardResult{
		RequestID:       requestID,
		ResponseID:      responsesResp.ID,
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

// streamResponsesFromCC reads upstream Chat Completions SSE chunks line-by-line,
// converts each to Responses API SSE events, and writes them to the client.
func (s *OpenAIGatewayService) streamResponsesFromCC(
	c *gin.Context,
	resp *http.Response,
	originalModel string,
	billingModel string,
	upstreamModel string,
	reasoningEffort *string,
	serviceTier *string,
	startTime time.Time,
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

	state := apicompat.NewCCStreamState()
	state.Model = originalModel

	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	var firstTokenMs *int
	clientDisconnected := false

	flushEvents := func(events [][]byte) {
		if clientDisconnected || len(events) == 0 {
			return
		}
		for _, evt := range events {
			if _, err := c.Writer.Write(evt); err != nil {
				clientDisconnected = true
				logger.L().Debug("openai responses→cc stream: client disconnected",
					zap.Error(err),
					zap.String("request_id", requestID),
				)
				return
			}
		}
		c.Writer.Flush()
	}

	for scanner.Scan() {
		line := scanner.Bytes()
		// Skip blank separator lines.
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			continue
		}
		events, err := apicompat.ConvertChatCompletionsSSEChunkToResponsesEvents(line, state)
		if err != nil {
			logger.L().Warn("openai responses→cc stream: failed to convert chunk",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
		}
		if firstTokenMs == nil && len(events) > 0 {
			ms := int(time.Since(startTime).Milliseconds())
			firstTokenMs = &ms
		}
		flushEvents(events)
	}

	if err := scanner.Err(); err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			logger.L().Warn("openai responses→cc stream: read error",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
		}
	}

	// Ensure stream is finalised even if upstream stopped without [DONE].
	if !state.CompletedSent {
		flushEvents(apicompat.FinalizeCCStream(state))
	}

	usage := OpenAIUsage{}
	if state.Usage != nil {
		usage.InputTokens = state.Usage.InputTokens
		usage.OutputTokens = state.Usage.OutputTokens
		if state.Usage.InputTokensDetails != nil {
			usage.CacheReadInputTokens = state.Usage.InputTokensDetails.CachedTokens
		}
	}

	return &OpenAIForwardResult{
		RequestID:       requestID,
		ResponseID:      state.ResponseID,
		Usage:           usage,
		Model:           originalModel,
		BillingModel:    billingModel,
		UpstreamModel:   upstreamModel,
		ReasoningEffort: reasoningEffort,
		ServiceTier:     serviceTier,
		Stream:          true,
		Duration:        time.Since(startTime),
		FirstTokenMs:    firstTokenMs,
	}, nil
}

func (s *OpenAIGatewayService) ProxyResponsesWebSocketAsChatCompletions(
	ctx context.Context,
	c *gin.Context,
	clientConn *coderws.Conn,
	account *Account,
	firstClientMessage []byte,
	hooks *OpenAIWSIngressHooks,
) error {
	if clientConn == nil {
		return errors.New("client websocket is nil")
	}
	if account == nil {
		return errors.New("account is nil")
	}

	turn := 1
	nextMessage := firstClientMessage
	for {
		if hooks != nil && hooks.BeforeTurn != nil {
			if err := hooks.BeforeTurn(turn); err != nil {
				return err
			}
		}

		result, turnErr := s.proxyResponsesWebSocketTurnAsChatCompletions(ctx, c, clientConn, account, nextMessage)
		if hooks != nil && hooks.AfterTurn != nil {
			hooks.AfterTurn(turn, result, turnErr)
		}
		if turnErr != nil {
			return turnErr
		}

		msgType, message, err := clientConn.Read(ctx)
		if err != nil {
			if isOpenAIWSClientDisconnectError(err) {
				return nil
			}
			return fmt.Errorf("read client websocket message: %w", err)
		}
		if msgType != coderws.MessageText && msgType != coderws.MessageBinary {
			return NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "unsupported websocket message type", nil)
		}
		if !gjson.ValidBytes(message) {
			return NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "invalid JSON payload", nil)
		}
		nextMessage = message
		turn++
	}
}

func (s *OpenAIGatewayService) proxyResponsesWebSocketTurnAsChatCompletions(
	ctx context.Context,
	c *gin.Context,
	clientConn *coderws.Conn,
	account *Account,
	body []byte,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()
	forwardReq, err := s.prepareResponsesAsChatCompletionsRequest(account, body, true)
	if err != nil {
		return nil, NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, err.Error(), err)
	}

	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	upstreamReq, err := http.NewRequestWithContext(upstreamCtx, http.MethodPost, forwardReq.targetURL, bytes.NewReader(forwardReq.ccBody))
	releaseUpstreamCtx()
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Authorization", "Bearer "+forwardReq.apiKey)
	upstreamReq.Header.Set("Accept", "text/event-stream")
	for key, values := range c.Request.Header {
		if openaiCCRawAllowedHeaders[strings.ToLower(key)] {
			for _, v := range values {
				upstreamReq.Header.Add(key, v)
			}
		}
	}
	if customUA := account.GetOpenAIUserAgent(); customUA != "" {
		upstreamReq.Header.Set("user-agent", customUA)
	}

	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return nil, fmt.Errorf("upstream request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
		upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
		if upstreamMsg == "" {
			upstreamMsg = http.StatusText(resp.StatusCode)
		}
		return nil, fmt.Errorf("upstream error: %d %s", resp.StatusCode, upstreamMsg)
	}

	requestID := resp.Header.Get("x-request-id")
	state := apicompat.NewCCStreamState()
	state.Model = forwardReq.originalModel
	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	var firstTokenMs *int
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		events, err := apicompat.ConvertChatCompletionsSSEChunkToResponsesEvents(line, state)
		if err != nil {
			logger.L().Warn("openai responses→cc websocket: failed to convert chunk",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
		}
		if firstTokenMs == nil && len(events) > 0 {
			ms := int(time.Since(startTime).Milliseconds())
			firstTokenMs = &ms
		}
		for _, eventFrame := range events {
			payloads := extractResponsesSSEDataPayloads(eventFrame)
			for _, payload := range payloads {
				if len(payload) == 0 || bytes.Equal(bytes.TrimSpace(payload), []byte("[DONE]")) {
					continue
				}
				writeCtx, cancel := context.WithTimeout(ctx, s.openAIWSWriteTimeout())
				err := clientConn.Write(writeCtx, coderws.MessageText, payload)
				cancel()
				if err != nil {
					return nil, fmt.Errorf("write client websocket event: %w", err)
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read upstream stream: %w", err)
	}
	if !state.CompletedSent {
		for _, eventFrame := range apicompat.FinalizeCCStream(state) {
			for _, payload := range extractResponsesSSEDataPayloads(eventFrame) {
				if len(payload) == 0 || bytes.Equal(bytes.TrimSpace(payload), []byte("[DONE]")) {
					continue
				}
				writeCtx, cancel := context.WithTimeout(ctx, s.openAIWSWriteTimeout())
				err := clientConn.Write(writeCtx, coderws.MessageText, payload)
				cancel()
				if err != nil {
					return nil, fmt.Errorf("write client websocket final event: %w", err)
				}
			}
		}
	}

	usage := OpenAIUsage{}
	if state.Usage != nil {
		usage.InputTokens = state.Usage.InputTokens
		usage.OutputTokens = state.Usage.OutputTokens
		if state.Usage.InputTokensDetails != nil {
			usage.CacheReadInputTokens = state.Usage.InputTokensDetails.CachedTokens
		}
	}

	return &OpenAIForwardResult{
		RequestID:       requestID,
		ResponseID:      state.ResponseID,
		Usage:           usage,
		Model:           forwardReq.originalModel,
		BillingModel:    forwardReq.billingModel,
		UpstreamModel:   forwardReq.upstreamModel,
		ReasoningEffort: forwardReq.reasoningEffort,
		ServiceTier:     forwardReq.serviceTier,
		Stream:          true,
		OpenAIWSMode:    true,
		Duration:        time.Since(startTime),
		FirstTokenMs:    firstTokenMs,
	}, nil
}

func extractResponsesSSEDataPayloads(frame []byte) [][]byte {
	var payloads [][]byte
	for _, rawLine := range bytes.Split(frame, []byte("\n")) {
		line := bytes.TrimSpace(rawLine)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(line[len("data:"):])
		if len(payload) == 0 {
			continue
		}
		payloadCopy := append([]byte(nil), payload...)
		payloads = append(payloads, payloadCopy)
	}
	return payloads
}
