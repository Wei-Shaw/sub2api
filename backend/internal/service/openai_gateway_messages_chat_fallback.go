package service

import (
	"context"
	"encoding/json"
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

// forwardAnthropicViaRawChatCompletions serves /v1/messages clients through
// an OpenAI-compatible upstream that only supports /v1/chat/completions.
//
// Conversion chain (direct, no Responses intermediary):
//
//	Request:  Anthropic Messages → Chat Completions (AnthropicToChatCompletionsRequest)
//	Response: CC chunk/response → Anthropic events/response (direct bridge)
//
// This is the /v1/messages counterpart of forwardResponsesViaRawChatCompletions
// (which serves /v1/responses clients). Unlike the Responses path, the direct
// bridge skips the Responses API intermediate representation entirely — every
// streaming token runs through a single state machine instead of two.
func (s *OpenAIGatewayService) forwardAnthropicViaRawChatCompletions(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	defaultMappedModel string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()

	// 1. Parse Anthropic request
	var anthropicReq apicompat.AnthropicRequest
	if err := json.Unmarshal(body, &anthropicReq); err != nil {
		writeAnthropicError(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return nil, fmt.Errorf("parse anthropic request: %w", err)
REDACTED
	originalModel := anthropicReq.Model
	if strings.TrimSpace(originalModel) == "" {
		writeAnthropicError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return nil, fmt.Errorf("missing model in request")
REDACTED
	applyOpenAICompatModelNormalization(&anthropicReq)
	clientStream := anthropicReq.Stream

	// 2. Anthropic → Chat Completions (direct, no Responses intermediary)
	chatReq, err := apicompat.AnthropicToChatCompletionsRequest(&anthropicReq)
	if err != nil {
		writeAnthropicError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return nil, fmt.Errorf("convert anthropic to chat completions: %w", err)
REDACTED

	billingModel := resolveOpenAIForwardModel(account, anthropicReq.Model, defaultMappedModel)
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)
	chatReq.Model = upstreamModel
	chatReq.Stream = clientStream
	if clientStream {
		chatReq.StreamOptions = &apicompat.ChatStreamOptions{IncludeUsage: trueREDACTED
REDACTED

	reasoningEffort := extractOpenAIReasoningEffortFromBody(body, upstreamModel, billingModel, originalModel)
	reasoningEffort = ApplyThinkingEnabledFallback(reasoningEffort, body, billingModel)
	serviceTier := extractOpenAIServiceTierFromBody(body)

	chatBody, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("marshal chat completions request: %w", err)
REDACTED
	if normalizedBody, normalized := NormalizeGLMOpenAIReasoningEffort(chatBody, upstreamModel); normalized {
		chatBody = normalizedBody
REDACTED
	// Unlike forwardResponsesViaRawChatCompletions, applyOpenAIFastPolicyToBody
	// is intentionally skipped: Anthropic Messages bodies carry no service_tier,
	// so the converted Chat Completions body never contains one and the policy
	// would always be a no-op on this path.

	logger.L().Debug("openai messages: forwarding via raw chat completions",
		zap.Int64("account_id", account.ID),
		zap.String("original_model", originalModel),
		zap.String("billing_model", billingModel),
		zap.String("upstream_model", upstreamModel),
		zap.Bool("stream", clientStream),
	)

	// 3. Build and send upstream request via the shared CC pipeline
	apiKey, targetURL, err := s.resolveCCFallbackTarget(account)
	if err != nil {
		return nil, err
REDACTED
	resp, err := s.sendCCUpstreamRequest(ctx, c, account, targetURL, chatBody, clientStream, apiKey, account.GetOpenAIUserAgent(), "")
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	// 4. Handle error responses
	if resp.StatusCode >= 400 {
		respBody, upstreamMsg := s.readOpenAIUpstreamError(resp)
		if foErr := s.failoverOpenAIUpstreamHTTPError(ctx, c, account, resp, respBody, upstreamMsg, upstreamModel); foErr != nil {
			return nil, foErr
	REDACTED
		// Non-failover error: return Anthropic-formatted error to client via the
		// shared compat handler (passthrough rules, ops recording, cyber_policy).
		return s.handleAnthropicErrorResponse(resp, c, account, billingModel)
REDACTED

	// 5. Convert response
	if clientStream {
		return s.streamChatCompletionsAsAnthropic(c, resp, originalModel, billingModel, upstreamModel, reasoningEffort, serviceTier, startTime)
REDACTED
	return s.bufferChatCompletionsAsAnthropic(c, resp, originalModel, billingModel, upstreamModel, reasoningEffort, serviceTier, startTime)
REDACTED

func (s *OpenAIGatewayService) bufferChatCompletionsAsAnthropic(
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
	ccResp, usage, err := s.readCCUpstreamJSONResponse(c, resp, writeAnthropicError)
	if err != nil {
		return nil, err
REDACTED
	anthropicResp := apicompat.ChatCompletionsResponseToAnthropic(ccResp, originalModel)

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
REDACTED
	c.JSON(http.StatusOK, anthropicResp)

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

func (s *OpenAIGatewayService) streamChatCompletionsAsAnthropic(
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
	writeStreamHeaders := s.newStreamHeaderWriter(c, resp.Header)

	anthropicState := apicompat.NewChatCompletionsToAnthropicStreamState(originalModel)
	clientDisconnected := false

	// 与 responses 兄弟不同：客户端断开后仍继续做事件转换（喂 anthropicState），
	// 仅跳过写出，保证 finalize 阶段的 usage 汇总不受断开影响。
	emitChunk := func(chunk *apicompat.ChatCompletionsChunk) {
		// CC chunk → Anthropic events (direct, single state machine)
		anthropicEvents := apicompat.ChatCompletionsChunkToAnthropicEvents(chunk, anthropicState)
		if clientDisconnected {
			return
	REDACTED
		for _, aEvt := range anthropicEvents {
			sse, err := apicompat.ResponsesAnthropicEventToSSE(aEvt)
			if err != nil {
				continue
		REDACTED
			writeStreamHeaders()
			if _, err := fmt.Fprint(c.Writer, sse); err != nil {
				clientDisconnected = true
				break
		REDACTED
	REDACTED
		if !clientDisconnected && len(anthropicEvents) > 0 {
			c.Writer.Flush()
	REDACTED
REDACTED

	scan := s.scanCCStream(resp, "openai messages chat fallback", requestID, startTime, emitChunk)
	usage := scan.Usage

	if scan.Err != nil {
		// Broken upstream read: skip finalization so no synthetic message_stop
		// masks the truncation, and surface the error to flag usage incomplete
		// (mirrors forwardResponsesViaRawChatCompletions).
		return &OpenAIForwardResult{
			RequestID:        requestID,
			Usage:            usage,
			Model:            originalModel,
			BillingModel:     billingModel,
			UpstreamModel:    upstreamModel,
			ReasoningEffort:  reasoningEffort,
			ServiceTier:      serviceTier,
			Stream:           true,
			Duration:         time.Since(startTime),
			FirstTokenMs:     scan.FirstTokenMs,
			ClientDisconnect: clientDisconnected,
	REDACTED, fmt.Errorf("stream usage incomplete: %w", scan.Err)
REDACTED

	// Finalize: close open blocks + emit message_delta/message_stop.
	finalEvents := apicompat.FinalizeChatCompletionsAnthropicStream(anthropicState)
	if !clientDisconnected {
		for _, aEvt := range finalEvents {
			sse, err := apicompat.ResponsesAnthropicEventToSSE(aEvt)
			if err != nil {
				continue
		REDACTED
			writeStreamHeaders()
			if _, err := fmt.Fprint(c.Writer, sse); err != nil {
				clientDisconnected = true
				break
		REDACTED
	REDACTED
		c.Writer.Flush()
REDACTED
	if !scan.SawDone {
		logCCStreamMissingDoneSentinel("openai messages chat fallback", requestID)
REDACTED

	return &OpenAIForwardResult{
		RequestID:        requestID,
		Usage:            usage,
		Model:            originalModel,
		BillingModel:     billingModel,
		UpstreamModel:    upstreamModel,
		ReasoningEffort:  reasoningEffort,
		ServiceTier:      serviceTier,
		Stream:           true,
		Duration:         time.Since(startTime),
		FirstTokenMs:     scan.FirstTokenMs,
		ClientDisconnect: clientDisconnected,
REDACTED, nil
REDACTED
