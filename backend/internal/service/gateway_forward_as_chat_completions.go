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
	"go.uber.org/zap"
)

// ForwardAsChatCompletions accepts an OpenAI Chat Completions API request body,
// converts it to Anthropic Messages format (chained via Responses format),
// forwards to the Anthropic upstream, and converts the response back to Chat
// Completions format. This enables Chat Completions clients to access Anthropic
// models through Anthropic platform groups.
func (s *GatewayService) ForwardAsChatCompletions(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	parsed *ParsedRequest,
) (*ForwardResult, error) {
	startTime := time.Now()

	// 1. Parse Chat Completions request
	var ccReq apicompat.ChatCompletionsRequest
	if err := json.Unmarshal(body, &ccReq); err != nil {
		return nil, fmt.Errorf("parse chat completions request: %w", err)
REDACTED
	originalModel := ccReq.Model
	clientStream := ccReq.Stream
	includeUsage := ccReq.StreamOptions != nil && ccReq.StreamOptions.IncludeUsage

	// 2. Convert CC → Responses → Anthropic (chained conversion)
	responsesReq, err := apicompat.ChatCompletionsToResponses(&ccReq)
	if err != nil {
		return nil, fmt.Errorf("convert chat completions to responses: %w", err)
REDACTED

	anthropicReq, err := apicompat.ResponsesToAnthropicRequest(responsesReq)
	if err != nil {
		return nil, fmt.Errorf("convert responses to anthropic: %w", err)
REDACTED

	// 3. Force upstream streaming
	anthropicReq.Stream = true
	reqStream := true

	// 4. Model mapping
	mappedModel := originalModel
	if account.Type == AccountTypeAPIKey {
		mappedModel = account.GetMappedModel(originalModel)
REDACTED
	if mappedModel == originalModel && account.Platform == PlatformAnthropic && account.Type != AccountTypeAPIKey {
		normalized := claude.NormalizeModelID(originalModel)
		if normalized != originalModel {
			mappedModel = normalized
	REDACTED
REDACTED
	anthropicReq.Model = mappedModel

	logger.L().Debug("gateway forward_as_chat_completions: model mapping applied",
		zap.Int64("account_id", account.ID),
		zap.String("original_model", originalModel),
		zap.String("mapped_model", mappedModel),
		zap.Bool("client_stream", clientStream),
	)

	// 5. Marshal Anthropic request body
	anthropicBody, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("marshal anthropic request: %w", err)
REDACTED

	// 6. Apply Claude Code mimicry for OAuth accounts
	isClaudeCode := false // CC API is never Claude Code
	shouldMimicClaudeCode := account.IsOAuth() && !isClaudeCode

	if shouldMimicClaudeCode {
		if !strings.Contains(strings.ToLower(mappedModel), "haiku") &&
			!systemIncludesClaudeCodePrompt(anthropicReq.System) {
			anthropicBody = injectClaudeCodePrompt(anthropicBody, anthropicReq.System)
	REDACTED
REDACTED

	// 7. Enforce cache_control block limit
	anthropicBody = enforceCacheControlLimit(anthropicBody)

	// 8. Get access token
	token, tokenType, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("get access token: %w", err)
REDACTED

	// 9. Get proxy URL
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED

	// 10. Build upstream request
	upstreamCtx, releaseUpstreamCtx := detachStreamUpstreamContext(ctx, reqStream)
	upstreamReq, err := s.buildUpstreamRequest(upstreamCtx, c, account, anthropicBody, token, tokenType, mappedModel, reqStream, shouldMimicClaudeCode)
	releaseUpstreamCtx()
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
REDACTED

	// 11. Send request
	resp, err := s.httpUpstream.DoWithTLS(upstreamReq, proxyURL, account.ID, account.Concurrency, account.IsTLSFingerprintEnabled())
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
	REDACTED
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: 0,
			Kind:               "request_error",
			Message:            safeErr,
	REDACTED)
		writeGatewayCCError(c, http.StatusBadGateway, "server_error", "Upstream request failed")
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	// 12. Handle error response with failover
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))

		upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
		upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)

		if s.shouldFailoverUpstreamError(resp.StatusCode) {
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				Kind:               "failover",
				Message:            upstreamMsg,
		REDACTED)
			if s.rateLimitService != nil {
				s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
		REDACTED
			return nil, &UpstreamFailoverError{
				StatusCode:   resp.StatusCode,
				ResponseBody: respBody,
		REDACTED
	REDACTED

		writeGatewayCCError(c, mapUpstreamStatusCode(resp.StatusCode), "server_error", upstreamMsg)
		return nil, fmt.Errorf("upstream error: %d %s", resp.StatusCode, upstreamMsg)
REDACTED

	// 13. Handle normal response
	// Read Anthropic SSE → convert to Responses events → convert to CC format
	var result *ForwardResult
	var handleErr error
	if clientStream {
		result, handleErr = s.handleCCStreamingFromAnthropic(resp, c, originalModel, mappedModel, startTime, includeUsage)
REDACTED else {
		result, handleErr = s.handleCCBufferedFromAnthropic(resp, c, originalModel, mappedModel, startTime)
REDACTED

	return result, handleErr
REDACTED

// handleCCBufferedFromAnthropic reads Anthropic SSE events, assembles the full
// response, then converts Anthropic → Responses → Chat Completions.
func (s *GatewayService) handleCCBufferedFromAnthropic(
	resp *http.Response,
	c *gin.Context,
	originalModel string,
	mappedModel string,
	startTime time.Time,
) (*ForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")

	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
REDACTED
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	var finalResp *apicompat.AnthropicResponse
	var usage ClaudeUsage

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "event: ") {
			continue
	REDACTED

		if !scanner.Scan() {
			break
	REDACTED
		dataLine := scanner.Text()
		if !strings.HasPrefix(dataLine, "data: ") {
			continue
	REDACTED
		payload := dataLine[6:]

		var event apicompat.AnthropicStreamEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			continue
	REDACTED

		if event.Type == "message_start" && event.Message != nil {
			finalResp = event.Message
	REDACTED
		if event.Type == "message_delta" {
			if event.Usage != nil {
				usage = ClaudeUsage{
					InputTokens:  event.Usage.InputTokens,
					OutputTokens: event.Usage.OutputTokens,
			REDACTED
				if event.Usage.CacheReadInputTokens > 0 {
					usage.CacheReadInputTokens = event.Usage.CacheReadInputTokens
			REDACTED
		REDACTED
			if event.Delta != nil && event.Delta.StopReason != "" && finalResp != nil {
				finalResp.StopReason = event.Delta.StopReason
		REDACTED
	REDACTED
		if event.Type == "content_block_start" && event.ContentBlock != nil && finalResp != nil {
			finalResp.Content = append(finalResp.Content, *event.ContentBlock)
	REDACTED
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
			REDACTED
		REDACTED
	REDACTED
REDACTED

	if err := scanner.Err(); err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			logger.L().Warn("forward_as_cc buffered: read error",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
	REDACTED
REDACTED

	if finalResp == nil {
		writeGatewayCCError(c, http.StatusBadGateway, "server_error", "Upstream stream ended without a response")
		return nil, fmt.Errorf("upstream stream ended without response")
REDACTED

	if usage.InputTokens > 0 || usage.OutputTokens > 0 {
		finalResp.Usage = apicompat.AnthropicUsage{
			InputTokens:  usage.InputTokens,
			OutputTokens: usage.OutputTokens,
	REDACTED
REDACTED

	// Chain: Anthropic → Responses → Chat Completions
	responsesResp := apicompat.AnthropicToResponsesResponse(finalResp)
	ccResp := apicompat.ResponsesToChatCompletions(responsesResp, originalModel)

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
REDACTED
	c.JSON(http.StatusOK, ccResp)

	return &ForwardResult{
		RequestID:     requestID,
		Usage:         usage,
		Model:         originalModel,
		UpstreamModel: mappedModel,
		Stream:        false,
		Duration:      time.Since(startTime),
REDACTED, nil
REDACTED

// handleCCStreamingFromAnthropic reads Anthropic SSE events, converts each
// to Responses events, then to Chat Completions chunks, and writes them.
func (s *GatewayService) handleCCStreamingFromAnthropic(
	resp *http.Response,
	c *gin.Context,
	originalModel string,
	mappedModel string,
	startTime time.Time,
	includeUsage bool,
) (*ForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
REDACTED
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	// Use Anthropic→Responses state machine, then convert Responses→CC
	anthState := apicompat.NewAnthropicEventToResponsesState()
	anthState.Model = originalModel
	ccState := apicompat.NewResponsesEventToChatState()
	ccState.Model = originalModel
	ccState.IncludeUsage = includeUsage

	var usage ClaudeUsage
	var firstTokenMs *int
	firstChunk := true

	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
REDACTED
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	resultWithUsage := func() *ForwardResult {
		return &ForwardResult{
			RequestID:     requestID,
			Usage:         usage,
			Model:         originalModel,
			UpstreamModel: mappedModel,
			Stream:        true,
			Duration:      time.Since(startTime),
			FirstTokenMs:  firstTokenMs,
	REDACTED
REDACTED

	writeChunk := func(chunk apicompat.ChatCompletionsChunk) bool {
		sse, err := apicompat.ChatChunkToSSE(chunk)
		if err != nil {
			return false
	REDACTED
		if _, err := fmt.Fprint(c.Writer, sse); err != nil {
			return true // client disconnected
	REDACTED
		return false
REDACTED

	processAnthropicEvent := func(event *apicompat.AnthropicStreamEvent) bool {
		if firstChunk {
			firstChunk = false
			ms := int(time.Since(startTime).Milliseconds())
			firstTokenMs = &ms
	REDACTED

		// Extract usage
		if event.Type == "message_delta" && event.Usage != nil {
			usage = ClaudeUsage{
				InputTokens:  event.Usage.InputTokens,
				OutputTokens: event.Usage.OutputTokens,
		REDACTED
			if event.Usage.CacheReadInputTokens > 0 {
				usage.CacheReadInputTokens = event.Usage.CacheReadInputTokens
		REDACTED
	REDACTED
		if event.Type == "message_start" && event.Message != nil && event.Message.Usage.InputTokens > 0 {
			usage.InputTokens = event.Message.Usage.InputTokens
	REDACTED

		// Chain: Anthropic event → Responses events → CC chunks
		responsesEvents := apicompat.AnthropicEventToResponsesEvents(event, anthState)
		for _, resEvt := range responsesEvents {
			ccChunks := apicompat.ResponsesEventToChatChunks(&resEvt, ccState)
			for _, chunk := range ccChunks {
				if disconnected := writeChunk(chunk); disconnected {
					return true
			REDACTED
		REDACTED
	REDACTED
		c.Writer.Flush()
		return false
REDACTED

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "event: ") {
			continue
	REDACTED

		if !scanner.Scan() {
			break
	REDACTED
		dataLine := scanner.Text()
		if !strings.HasPrefix(dataLine, "data: ") {
			continue
	REDACTED
		payload := dataLine[6:]

		var event apicompat.AnthropicStreamEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			continue
	REDACTED

		if processAnthropicEvent(&event) {
			return resultWithUsage(), nil
	REDACTED
REDACTED

	if err := scanner.Err(); err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			logger.L().Warn("forward_as_cc stream: read error",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
	REDACTED
REDACTED

	// Finalize both state machines
	finalResEvents := apicompat.FinalizeAnthropicResponsesStream(anthState)
	for _, resEvt := range finalResEvents {
		ccChunks := apicompat.ResponsesEventToChatChunks(&resEvt, ccState)
		for _, chunk := range ccChunks {
			writeChunk(chunk) //nolint:errcheck
	REDACTED
REDACTED
	finalCCChunks := apicompat.FinalizeResponsesChatStream(ccState)
	for _, chunk := range finalCCChunks {
		writeChunk(chunk) //nolint:errcheck
REDACTED

	// Write [DONE] marker
	fmt.Fprint(c.Writer, "data: [DONE]\n\n") //nolint:errcheck
	c.Writer.Flush()

	return resultWithUsage(), nil
REDACTED

// writeGatewayCCError writes an error in OpenAI Chat Completions format for
// the Anthropic-upstream CC forwarding path.
func writeGatewayCCError(c *gin.Context, statusCode int, errType, message string) {
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"type":    errType,
			"message": message,
	REDACTED,
REDACTED)
REDACTED
