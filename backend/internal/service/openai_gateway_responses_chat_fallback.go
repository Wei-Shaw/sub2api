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
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"type":    "invalid_request_error",
				"message": "Failed to parse request body",
		REDACTED,
	REDACTED)
		return nil, fmt.Errorf("parse responses request: %w", err)
REDACTED
	originalModel := strings.TrimSpace(responsesReq.Model)
	if originalModel == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"type":    "invalid_request_error",
				"message": "model is required",
		REDACTED,
	REDACTED)
		return nil, fmt.Errorf("missing model in request")
REDACTED

	clientStream := responsesReq.Stream
	reasoningEffort := extractOpenAIReasoningEffortFromBody(body, originalModel)
	serviceTier := extractOpenAIServiceTierFromBody(body)

	chatReq, err := apicompat.ResponsesToChatCompletionsRequest(&responsesReq)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"type":    "invalid_request_error",
				"message": err.Error(),
		REDACTED,
	REDACTED)
		return nil, fmt.Errorf("convert responses to chat completions: %w", err)
REDACTED

	billingModel := resolveOpenAIForwardModel(account, originalModel, "")
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)
	chatReq.Model = upstreamModel
	if clientStream {
		chatReq.StreamOptions = &apicompat.ChatStreamOptions{IncludeUsage: trueREDACTED
REDACTED

	chatBody, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("marshal chat completions fallback request: %w", err)
REDACTED
	chatBody, err = s.applyOpenAIFastPolicyToBody(ctx, account, upstreamModel, chatBody)
	if err != nil {
		var blocked *OpenAIFastBlockedError
		if errors.As(err, &blocked) {
			writeOpenAIFastPolicyBlockedResponse(c, blocked)
	REDACTED
		return nil, err
REDACTED
	if serviceTier == nil {
		serviceTier = extractOpenAIServiceTierFromBody(chatBody)
REDACTED

	logger.L().Debug("openai responses: forwarding via raw chat completions",
		zap.Int64("account_id", account.ID),
		zap.String("original_model", originalModel),
		zap.String("billing_model", billingModel),
		zap.String("upstream_model", upstreamModel),
		zap.Bool("stream", clientStream),
	)

	apiKey := account.GetOpenAIApiKey()
	if apiKey == "" {
		return nil, fmt.Errorf("account %d missing api_key", account.ID)
REDACTED
	baseURL := account.GetOpenAIBaseURL()
	if baseURL == "" {
		baseURL = "https://api.openai.com"
REDACTED
	validatedURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base_url: %w", err)
REDACTED
	targetURL := buildOpenAIChatCompletionsURL(validatedURL)

	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	upstreamReq, err := http.NewRequestWithContext(upstreamCtx, http.MethodPost, targetURL, bytes.NewReader(chatBody))
	releaseUpstreamCtx()
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
REDACTED
	upstreamReq = upstreamReq.WithContext(WithHTTPUpstreamProfile(upstreamReq.Context(), HTTPUpstreamProfileOpenAI))
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Authorization", "Bearer "+apiKey)
	if clientStream {
		upstreamReq.Header.Set("Accept", "text/event-stream")
REDACTED else {
		upstreamReq.Header.Set("Accept", "application/json")
REDACTED
	for key, values := range c.Request.Header {
		lowerKey := strings.ToLower(key)
		if openaiCCRawAllowedHeaders[lowerKey] {
			for _, v := range values {
				upstreamReq.Header.Add(key, v)
		REDACTED
	REDACTED
REDACTED
	if customUA := account.GetOpenAIUserAgent(); customUA != "" {
		upstreamReq.Header.Set("user-agent", customUA)
REDACTED

	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		// Transport-level failure (proxy/DNS/TCP/TLS — no HTTP response). Convert to
		// a failover so the handler switches to a healthy account, and temporarily
		// unschedule the account on durable faults (e.g. rejected proxy credentials).
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	if resp.StatusCode >= 400 {
		respBody := s.readUpstreamErrorBody(resp)
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))

		upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
		upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
		if s.shouldFailoverOpenAIUpstreamResponse(resp.StatusCode, upstreamMsg, respBody) {
			upstreamDetail := ""
			if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
				maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
				if maxBytes <= 0 {
					maxBytes = 2048
			REDACTED
				upstreamDetail = truncateString(string(respBody), maxBytes)
		REDACTED
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				Kind:               "failover",
				Message:            upstreamMsg,
				Detail:             upstreamDetail,
		REDACTED)
			s.handleOpenAIAccountUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody, upstreamModel)
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				RetryableOnSameAccount: account.IsPoolMode() && (account.IsPoolModeRetryableStatus(resp.StatusCode) || isOpenAITransientProcessingError(resp.StatusCode, upstreamMsg, respBody)),
		REDACTED
	REDACTED
		return s.handleErrorResponse(ctx, resp, c, account, chatBody, billingModel)
REDACTED

	if clientStream {
		return s.streamChatCompletionsAsResponses(c, resp, originalModel, billingModel, upstreamModel, reasoningEffort, serviceTier, startTime)
REDACTED
	return s.bufferChatCompletionsAsResponses(c, resp, originalModel, billingModel, upstreamModel, reasoningEffort, serviceTier, startTime)
REDACTED

func (s *OpenAIGatewayService) bufferChatCompletionsAsResponses(
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
			c.JSON(http.StatusBadGateway, gin.H{
				"error": gin.H{
					"type":    "api_error",
					"message": "Failed to read upstream response",
			REDACTED,
		REDACTED)
	REDACTED
		return nil, fmt.Errorf("read upstream body: %w", err)
REDACTED

	var ccResp apicompat.ChatCompletionsResponse
	if err := json.Unmarshal(respBody, &ccResp); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": gin.H{
				"type":    "api_error",
				"message": "Failed to parse upstream response",
		REDACTED,
	REDACTED)
		return nil, fmt.Errorf("parse chat completions response: %w", err)
REDACTED
	responsesResp := apicompat.ChatCompletionsResponseToResponses(&ccResp, originalModel)

	usage := OpenAIUsage{REDACTED
	if parsed, ok := extractOpenAIUsageFromJSONBytes(respBody); ok {
		usage = parsed
REDACTED

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
REDACTED
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
REDACTED, nil
REDACTED

func (s *OpenAIGatewayService) streamChatCompletionsAsResponses(
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
	headersWritten := false
	writeStreamHeaders := func() {
		if headersWritten {
			return
	REDACTED
		headersWritten = true
		if s.responseHeaderFilter != nil {
			responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	REDACTED
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("Connection", "keep-alive")
		c.Writer.Header().Set("X-Accel-Buffering", "no")
		c.Writer.WriteHeader(http.StatusOK)
REDACTED

	state := apicompat.NewChatCompletionsToResponsesStreamState(originalModel)
	var usage OpenAIUsage
	var firstTokenMs *int
	clientDisconnected := false
	sawDone := false

	writeEvents := func(events []apicompat.ResponsesStreamEvent) {
		if clientDisconnected || len(events) == 0 {
			return
	REDACTED
		writeStreamHeaders()
		for _, event := range events {
			sse, err := apicompat.ResponsesEventToSSE(event)
			if err != nil {
				logger.L().Warn("openai responses chat fallback: failed to marshal stream event",
					zap.Error(err),
					zap.String("request_id", requestID),
				)
				continue
		REDACTED
			if _, err := fmt.Fprint(c.Writer, sse); err != nil {
				clientDisconnected = true
				logger.L().Debug("openai responses chat fallback: client disconnected, continuing to drain upstream for billing",
					zap.Error(err),
					zap.String("request_id", requestID),
				)
				return
		REDACTED
	REDACTED
		c.Writer.Flush()
REDACTED

	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
REDACTED
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	for scanner.Scan() {
		line := scanner.Text()
		payload, ok := extractOpenAISSEDataLine(line)
		if !ok {
			continue
	REDACTED
		payload = strings.TrimSpace(payload)
		if payload == "" {
			continue
	REDACTED
		if payload == "[DONE]" {
			sawDone = true
			break
	REDACTED

		if u := extractCCStreamUsage(payload); u != nil {
			usage = *u
	REDACTED

		var chunk apicompat.ChatCompletionsChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			logger.L().Warn("openai responses chat fallback: failed to parse chat stream chunk",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
			continue
	REDACTED
		if firstTokenMs == nil && !isOpenAIChatUsageOnlyStreamChunk(payload) && chatChunkStartsResponsesOutput(&chunk) {
			ms := int(time.Since(startTime).Milliseconds())
			firstTokenMs = &ms
	REDACTED
		writeEvents(apicompat.ChatCompletionsChunkToResponsesEvents(&chunk, state))
REDACTED

	if err := scanner.Err(); err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			logger.L().Warn("openai responses chat fallback: stream read error",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
	REDACTED
		return &OpenAIForwardResult{
			RequestID:       requestID,
			Usage:           usage,
			Model:           originalModel,
			BillingModel:    billingModel,
			UpstreamModel:   upstreamModel,
			ReasoningEffort: reasoningEffort,
			ServiceTier:     serviceTier,
			Stream:          true,
			Duration:        time.Since(startTime),
			FirstTokenMs:    firstTokenMs,
	REDACTED, fmt.Errorf("stream usage incomplete: %w", err)
REDACTED

	writeEvents(apicompat.FinalizeChatCompletionsResponsesStream(state))
	if !clientDisconnected {
		writeStreamHeaders()
		if _, err := fmt.Fprint(c.Writer, "data: [DONE]\n\n"); err != nil {
			clientDisconnected = true
	REDACTED
		if !clientDisconnected {
			c.Writer.Flush()
	REDACTED
REDACTED
	if !sawDone {
		logger.L().Debug("openai responses chat fallback: upstream stream ended without done sentinel",
			zap.String("request_id", requestID),
		)
REDACTED

	return &OpenAIForwardResult{
		RequestID:       requestID,
		Usage:           usage,
		Model:           originalModel,
		BillingModel:    billingModel,
		UpstreamModel:   upstreamModel,
		ReasoningEffort: reasoningEffort,
		ServiceTier:     serviceTier,
		Stream:          true,
		Duration:        time.Since(startTime),
		FirstTokenMs:    firstTokenMs,
REDACTED, nil
REDACTED

func chatChunkStartsResponsesOutput(chunk *apicompat.ChatCompletionsChunk) bool {
	if chunk == nil {
		return false
REDACTED
	for _, choice := range chunk.Choices {
		if choice.Delta.Content != nil || choice.Delta.ReasoningContent != nil || len(choice.Delta.ToolCalls) > 0 {
			return true
	REDACTED
REDACTED
	return false
REDACTED
