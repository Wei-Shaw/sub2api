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
	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
)

// ForwardAsChatCompletions serves OpenAI Chat Completions clients through
// Gemini accounts. It keeps the client-facing response in Chat Completions
// format while routing the upstream call through Gemini native endpoints.
func (s *GeminiMessagesCompatService) ForwardAsChatCompletions(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
) (*ForwardResult, error) {
	startTime := time.Now()

	var ccReq apicompat.ChatCompletionsRequest
	if err := json.Unmarshal(body, &ccReq); err != nil {
		return nil, s.writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
REDACTED
	if strings.TrimSpace(ccReq.Model) == "" {
		return nil, s.writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
REDACTED

	originalModel := ccReq.Model
	clientStream := ccReq.Stream
	includeUsage := ccReq.StreamOptions != nil && ccReq.StreamOptions.IncludeUsage

	responsesReq, err := apicompat.ChatCompletionsToResponses(&ccReq)
	if err != nil {
		return nil, s.writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
REDACTED

	anthropicReq, err := apicompat.ResponsesToAnthropicRequest(responsesReq)
	if err != nil {
		return nil, s.writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
REDACTED
	anthropicReq.Stream = clientStream

	claudeBody, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("marshal chat completions compat request: %w", err)
REDACTED

	return s.forwardClaudeBodyAsChatCompletions(ctx, c, account, claudeBody, originalModel, clientStream, includeUsage, startTime, body)
REDACTED

func (s *GeminiMessagesCompatService) forwardClaudeBodyAsChatCompletions(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	claudeBody []byte,
	originalModel string,
	clientStream bool,
	includeUsage bool,
	startTime time.Time,
	originalChatBody []byte,
) (*ForwardResult, error) {
	var req struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
REDACTED
	if err := json.Unmarshal(claudeBody, &req); err != nil {
		return nil, s.writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
REDACTED
	if strings.TrimSpace(req.Model) == "" {
		return nil, s.writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
REDACTED

	mappedModel := req.Model
	if account.Type == AccountTypeAPIKey || account.Type == AccountTypeServiceAccount {
		mappedModel = account.GetMappedModel(req.Model)
REDACTED

	geminiReq, err := convertClaudeMessagesToGeminiGenerateContent(claudeBody)
	if err != nil {
		return nil, s.writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
REDACTED
	geminiReq = ensureGeminiFunctionCallThoughtSignatures(geminiReq)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED

	useUpstreamStream := clientStream
	if account.Type == AccountTypeOAuth && !clientStream && strings.TrimSpace(account.GetCredential("project_id")) != "" {
		useUpstreamStream = true
REDACTED

	buildReq, requestIDHeader := s.buildGeminiChatCompletionsUpstreamRequestFunc(
		account,
		mappedModel,
		geminiReq,
		clientStream,
		useUpstreamStream,
	)

	var resp *http.Response
	for attempt := 1; attempt <= geminiMaxRetries; attempt++ {
		upstreamReq, idHeader, err := buildReq(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, err
		REDACTED
			return nil, s.writeChatCompletionsError(c, http.StatusBadGateway, "upstream_error", err.Error())
	REDACTED
		requestIDHeader = idHeader

		resp, err = s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
		if err != nil {
			safeErr := sanitizeUpstreamErrorMessage(err.Error())
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: 0,
				Kind:               "request_error",
				Message:            safeErr,
		REDACTED)
			if attempt < geminiMaxRetries {
				logger.LegacyPrintf("service.gemini_chat_completions", "Gemini account %d: upstream request failed, retry %d/%d: %v", account.ID, attempt, geminiMaxRetries, err)
				sleepGeminiBackoff(attempt)
				continue
		REDACTED
			setOpsUpstreamError(c, 0, safeErr, "")
			return nil, s.writeChatCompletionsError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed after retries: "+safeErr)
	REDACTED

		if matched, rebuilt := s.checkErrorPolicyInLoop(ctx, account, resp); matched {
			resp = rebuilt
			break
	REDACTED else {
			resp = rebuilt
	REDACTED

		if resp.StatusCode >= 400 && s.shouldRetryGeminiUpstreamError(account, resp.StatusCode) {
			respBody := s.readUpstreamErrorBody(resp)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusForbidden && isGeminiInsufficientScope(resp.Header, respBody) {
				resp = &http.Response{
					StatusCode: resp.StatusCode,
					Header:     resp.Header.Clone(),
					Body:       io.NopCloser(bytes.NewReader(respBody)),
			REDACTED
				break
		REDACTED
			if resp.StatusCode == http.StatusTooManyRequests {
				s.handleGeminiUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
		REDACTED
			if attempt < geminiMaxRetries {
				upstreamReqID := resp.Header.Get(requestIDHeader)
				if upstreamReqID == "" {
					upstreamReqID = resp.Header.Get("x-goog-request-id")
			REDACTED
				upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
				upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
				appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
					Platform:           account.Platform,
					AccountID:          account.ID,
					AccountName:        account.Name,
					UpstreamStatusCode: resp.StatusCode,
					UpstreamRequestID:  upstreamReqID,
					Kind:               "retry",
					Message:            upstreamMsg,
			REDACTED)
				logger.LegacyPrintf("service.gemini_chat_completions", "Gemini account %d: upstream status %d, retry %d/%d", account.ID, resp.StatusCode, attempt, geminiMaxRetries)
				sleepGeminiBackoff(attempt)
				continue
		REDACTED
			resp = &http.Response{
				StatusCode: resp.StatusCode,
				Header:     resp.Header.Clone(),
				Body:       io.NopCloser(bytes.NewReader(respBody)),
		REDACTED
			break
	REDACTED

		break
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	requestID := resp.Header.Get(requestIDHeader)
	if requestID == "" {
		requestID = resp.Header.Get("x-goog-request-id")
REDACTED
	if requestID != "" {
		c.Header("x-request-id", requestID)
REDACTED

	reasoningEffort := extractCCReasoningEffortFromBody(originalChatBody)
	// 国产模型默认 effort 补充（本路径上游是 Gemini，不会命中 passback-required）。
	// 保持与 OpenAI 网关路径调用模式一致，便于未来上游变异时语义一致。
	reasoningEffort = ApplyThinkingEnabledFallback(reasoningEffort, originalChatBody, mappedModel)

	if resp.StatusCode >= 400 {
		respBody := s.readUpstreamErrorBody(resp)
		s.handleGeminiUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
		evBody := unwrapIfNeeded(account.Type == AccountTypeOAuth, respBody)

		if s.shouldFailoverGeminiUpstreamError(resp.StatusCode) {
			upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(evBody)))
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  requestID,
				Kind:               "failover",
				Message:            upstreamMsg,
		REDACTED)
			return nil, &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: evBodyREDACTED
	REDACTED

		return nil, s.writeGeminiChatCompletionsMappedError(c, account, resp.StatusCode, requestID, evBody)
REDACTED

	var usage *ClaudeUsage
	var firstTokenMs *int
	if clientStream {
		streamRes, err := s.handleChatCompletionsStreamingResponseFromGemini(c, resp, startTime, originalModel, account.Type == AccountTypeOAuth, includeUsage)
		if err != nil {
			return nil, err
	REDACTED
		usage = streamRes.usage
		firstTokenMs = streamRes.firstTokenMs
REDACTED else if useUpstreamStream {
		collected, usageObj, err := collectGeminiSSE(resp.Body, account.Type == AccountTypeOAuth)
		if err != nil {
			return nil, s.writeChatCompletionsError(c, http.StatusBadGateway, "upstream_error", "Failed to read upstream stream")
	REDACTED
		collectedBytes, _ := json.Marshal(collected)
		chatResp, usageObj2, err := geminiResponseToChatCompletions(collected, originalModel, collectedBytes, usageObj)
		if err != nil {
			return nil, s.writeChatCompletionsError(c, http.StatusBadGateway, "upstream_error", "Failed to parse upstream response")
	REDACTED
		c.JSON(http.StatusOK, chatResp)
		usage = usageObj2
REDACTED else {
		usageResp, err := s.handleChatCompletionsNonStreamingResponseFromGemini(c, resp, originalModel, account.Type == AccountTypeOAuth)
		if err != nil {
			return nil, err
	REDACTED
		usage = usageResp
REDACTED

	if usage == nil {
		usage = &ClaudeUsage{REDACTED
REDACTED

	imageCount := 0
	imageInputSize := s.extractImageInputSize(claudeBody)
	imageSize := normalizeOpenAIImageSizeTier(imageInputSize)
	if isImageGenerationModel(originalModel) {
		imageCount = 1
REDACTED

	return &ForwardResult{
		RequestID:        requestID,
		Usage:            *usage,
		Model:            originalModel,
		UpstreamModel:    mappedModel,
		Stream:           clientStream,
		Duration:         time.Since(startTime),
		FirstTokenMs:     firstTokenMs,
		ReasoningEffort:  reasoningEffort,
		ImageCount:       imageCount,
		ImageSize:        imageSize,
		ImageInputSize:   imageInputSize,
		ClientDisconnect: false,
REDACTED, nil
REDACTED

func (s *GeminiMessagesCompatService) buildGeminiChatCompletionsUpstreamRequestFunc(
	account *Account,
	mappedModel string,
	geminiReq []byte,
	clientStream bool,
	useUpstreamStream bool,
) (func(context.Context) (*http.Request, string, error), string) {
	switch account.Type {
	case AccountTypeAPIKey:
		return func(ctx context.Context) (*http.Request, string, error) {
			apiKey := account.GetCredential("api_key")
			if strings.TrimSpace(apiKey) == "" {
				return nil, "", errors.New("gemini api_key not configured")
		REDACTED

			baseURL := account.GetGeminiBaseURL(geminicli.AIStudioBaseURL)
			normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
			if err != nil {
				return nil, "", err
		REDACTED

			action := "generateContent"
			if clientStream {
				action = "streamGenerateContent"
		REDACTED
			fullURL := fmt.Sprintf("%s/v1beta/models/%s:%s", strings.TrimRight(normalizedBaseURL, "/"), mappedModel, action)
			if clientStream {
				fullURL += "?alt=sse"
		REDACTED

			restGeminiReq := normalizeGeminiRequestForAIStudio(geminiReq)
			upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(restGeminiReq))
			if err != nil {
				return nil, "", err
		REDACTED
			upstreamReq.Header.Set("Content-Type", "application/json")
			upstreamReq.Header.Set("x-goog-api-key", apiKey)
			return upstreamReq, "x-request-id", nil
	REDACTED, "x-request-id"

	case AccountTypeOAuth:
		return func(ctx context.Context) (*http.Request, string, error) {
			if s.tokenProvider == nil {
				return nil, "", errors.New("gemini token provider not configured")
		REDACTED
			accessToken, err := s.tokenProvider.GetAccessToken(ctx, account)
			if err != nil {
				return nil, "", err
		REDACTED

			projectID := strings.TrimSpace(account.GetCredential("project_id"))
			action := "generateContent"
			if useUpstreamStream {
				action = "streamGenerateContent"
		REDACTED

			if projectID != "" {
				baseURL, err := s.validateUpstreamBaseURL(geminicli.GeminiCliBaseURL)
				if err != nil {
					return nil, "", err
			REDACTED
				fullURL := fmt.Sprintf("%s/v1internal:%s", strings.TrimRight(baseURL, "/"), action)
				if useUpstreamStream {
					fullURL += "?alt=sse"
			REDACTED

				var inner any
				if err := json.Unmarshal(geminiReq, &inner); err != nil {
					return nil, "", fmt.Errorf("failed to parse gemini request: %w", err)
			REDACTED
				wrappedBytes, _ := json.Marshal(map[string]any{
					"model":   mappedModel,
					"project": projectID,
					"request": inner,
			REDACTED)

				upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(wrappedBytes))
				if err != nil {
					return nil, "", err
			REDACTED
				upstreamReq.Header.Set("Content-Type", "application/json")
				upstreamReq.Header.Set("Authorization", "Bearer "+accessToken)
				upstreamReq.Header.Set("User-Agent", geminicli.GeminiCLIUserAgent)
				return upstreamReq, "x-request-id", nil
		REDACTED

			baseURL := account.GetGeminiBaseURL(geminicli.AIStudioBaseURL)
			normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
			if err != nil {
				return nil, "", err
		REDACTED

			fullURL := fmt.Sprintf("%s/v1beta/models/%s:%s", strings.TrimRight(normalizedBaseURL, "/"), mappedModel, action)
			if useUpstreamStream {
				fullURL += "?alt=sse"
		REDACTED

			restGeminiReq := normalizeGeminiRequestForAIStudio(geminiReq)
			upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(restGeminiReq))
			if err != nil {
				return nil, "", err
		REDACTED
			upstreamReq.Header.Set("Content-Type", "application/json")
			upstreamReq.Header.Set("Authorization", "Bearer "+accessToken)
			return upstreamReq, "x-request-id", nil
	REDACTED, "x-request-id"

	case AccountTypeServiceAccount:
		return func(ctx context.Context) (*http.Request, string, error) {
			if s.tokenProvider == nil {
				return nil, "", errors.New("gemini token provider not configured")
		REDACTED
			accessToken, err := s.tokenProvider.GetAccessToken(ctx, account)
			if err != nil {
				return nil, "", err
		REDACTED

			action := "generateContent"
			if clientStream {
				action = "streamGenerateContent"
		REDACTED
			fullURL, err := buildVertexGeminiURL(account.VertexProjectID(), account.VertexLocation(mappedModel), mappedModel, action, clientStream)
			if err != nil {
				return nil, "", err
		REDACTED

			restGeminiReq := normalizeGeminiRequestForAIStudio(geminiReq)
			upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(restGeminiReq))
			if err != nil {
				return nil, "", err
		REDACTED
			upstreamReq.Header.Set("Content-Type", "application/json")
			upstreamReq.Header.Set("Authorization", "Bearer "+accessToken)
			return upstreamReq, "x-request-id", nil
	REDACTED, "x-request-id"

	default:
		return func(context.Context) (*http.Request, string, error) {
			return nil, "", fmt.Errorf("unsupported account type: %s", account.Type)
	REDACTED, "x-request-id"
REDACTED
REDACTED

func (s *GeminiMessagesCompatService) handleChatCompletionsNonStreamingResponseFromGemini(
	c *gin.Context,
	resp *http.Response,
	originalModel string,
	isOAuth bool,
) (*ClaudeUsage, error) {
	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
REDACTED
	if isOAuth {
		if unwrappedBody, uwErr := unwrapGeminiResponse(respBody); uwErr == nil {
			respBody = unwrappedBody
	REDACTED
REDACTED

	var geminiResp map[string]any
	if err := json.Unmarshal(respBody, &geminiResp); err != nil {
		return nil, s.writeChatCompletionsError(c, http.StatusBadGateway, "upstream_error", "Failed to parse upstream response")
REDACTED

	chatResp, usage, err := geminiResponseToChatCompletions(geminiResp, originalModel, respBody, nil)
	if err != nil {
		return nil, s.writeChatCompletionsError(c, http.StatusBadGateway, "upstream_error", "Failed to parse upstream response")
REDACTED

	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	c.JSON(http.StatusOK, chatResp)
	return usage, nil
REDACTED

func geminiResponseToChatCompletions(
	geminiResp map[string]any,
	originalModel string,
	rawData []byte,
	usageOverride *ClaudeUsage,
) (*apicompat.ChatCompletionsResponse, *ClaudeUsage, error) {
	claudeRespMap, usage := convertGeminiToClaudeMessage(geminiResp, originalModel, rawData)
	if usageOverride != nil && (usageOverride.InputTokens > 0 || usageOverride.OutputTokens > 0 || usageOverride.CacheReadInputTokens > 0) {
		usage = usageOverride
		if usageMap, ok := claudeRespMap["usage"].(map[string]any); ok {
			usageMap["input_tokens"] = usage.InputTokens
			usageMap["output_tokens"] = usage.OutputTokens
			usageMap["cache_read_input_tokens"] = usage.CacheReadInputTokens
	REDACTED
REDACTED

	claudeBytes, err := json.Marshal(claudeRespMap)
	if err != nil {
		return nil, nil, err
REDACTED
	var anthropicResp apicompat.AnthropicResponse
	if err := json.Unmarshal(claudeBytes, &anthropicResp); err != nil {
		return nil, nil, err
REDACTED
	responsesResp := apicompat.AnthropicToResponsesResponse(&anthropicResp)
	return apicompat.ResponsesToChatCompletions(responsesResp, originalModel), usage, nil
REDACTED

func (s *GeminiMessagesCompatService) handleChatCompletionsStreamingResponseFromGemini(
	c *gin.Context,
	resp *http.Response,
	startTime time.Time,
	originalModel string,
	isOAuth bool,
	includeUsage bool,
) (*geminiStreamResult, error) {
	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
REDACTED
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
REDACTED

	anthState := apicompat.NewAnthropicEventToResponsesState()
	anthState.Model = originalModel
	ccState := apicompat.NewResponsesEventToChatState()
	ccState.Model = originalModel
	ccState.IncludeUsage = includeUsage

	var usage ClaudeUsage
	var firstTokenMs *int
	firstChunk := true

	writeChatChunk := func(chunk apicompat.ChatCompletionsChunk) bool {
		sse, err := apicompat.ChatChunkToSSE(chunk)
		if err != nil {
			return false
	REDACTED
		if _, err := io.WriteString(c.Writer, sse); err != nil {
			return true
	REDACTED
		return false
REDACTED

	emitAnthropicEvent := func(evt *apicompat.AnthropicStreamEvent) bool {
		responsesEvents := apicompat.AnthropicEventToResponsesEvents(evt, anthState)
		for _, resEvt := range responsesEvents {
			chunks := apicompat.ResponsesEventToChatChunks(&resEvt, ccState)
			for _, chunk := range chunks {
				if disconnected := writeChatChunk(chunk); disconnected {
					return true
			REDACTED
		REDACTED
	REDACTED
		flusher.Flush()
		return false
REDACTED

	messageID := "msg_" + randomHex(12)
	if emitAnthropicEvent(&apicompat.AnthropicStreamEvent{
		Type: "message_start",
		Message: &apicompat.AnthropicResponse{
			ID:      messageID,
			Type:    "message",
			Role:    "assistant",
			Model:   originalModel,
			Content: []apicompat.AnthropicContentBlock{REDACTED,
			Usage:   apicompat.AnthropicUsage{REDACTED,
	REDACTED,
REDACTED) {
		return &geminiStreamResult{usage: &usage, firstTokenMs: firstTokenMsREDACTED, nil
REDACTED

	finishReason := ""
	sawToolUse := false
	nextBlockIndex := 0
	openBlockIndex := -1
	openBlockType := ""
	seenText := ""
	openToolIndex := -1
	openToolName := ""
	seenToolJSON := ""

	closeOpenBlock := func() bool {
		if openBlockIndex < 0 {
			return false
	REDACTED
		disconnected := emitAnthropicEvent(&apicompat.AnthropicStreamEvent{Type: "content_block_stop"REDACTED)
		openBlockIndex = -1
		openBlockType = ""
		return disconnected
REDACTED
	closeOpenTool := func() bool {
		if openToolIndex < 0 {
			return false
	REDACTED
		disconnected := emitAnthropicEvent(&apicompat.AnthropicStreamEvent{Type: "content_block_stop"REDACTED)
		openToolIndex = -1
		openToolName = ""
		seenToolJSON = ""
		return disconnected
REDACTED

	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			trimmed := strings.TrimRight(line, "\r\n")
			if strings.HasPrefix(trimmed, "data:") {
				payload := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
				if payload != "" && payload != "[DONE]" {
					rawBytes := []byte(payload)
					if isOAuth {
						if innerBytes, uwErr := unwrapGeminiResponse(rawBytes); uwErr == nil {
							rawBytes = innerBytes
					REDACTED
				REDACTED

					var geminiResp map[string]any
					if err := json.Unmarshal(rawBytes, &geminiResp); err == nil {
						if firstChunk {
							firstChunk = false
							ms := int(time.Since(startTime).Milliseconds())
							firstTokenMs = &ms
					REDACTED
						if fr := extractGeminiFinishReason(geminiResp); fr != "" {
							finishReason = fr
					REDACTED
						if u := extractGeminiUsage(rawBytes); u != nil {
							usage = *u
					REDACTED

						for _, part := range extractGeminiParts(geminiResp) {
							if text, ok := part["text"].(string); ok && text != "" {
								if openToolIndex >= 0 {
									if closeOpenTool() {
										return &geminiStreamResult{usage: &usage, firstTokenMs: firstTokenMsREDACTED, nil
								REDACTED
							REDACTED
								delta, newSeen := computeGeminiTextDelta(seenText, text)
								seenText = newSeen
								if delta == "" {
									continue
							REDACTED
								if openBlockType != "text" {
									if closeOpenBlock() {
										return &geminiStreamResult{usage: &usage, firstTokenMs: firstTokenMsREDACTED, nil
								REDACTED
									idx := nextBlockIndex
									nextBlockIndex++
									openBlockIndex = idx
									openBlockType = "text"
									if emitAnthropicEvent(&apicompat.AnthropicStreamEvent{
										Type:  "content_block_start",
										Index: &idx,
										ContentBlock: &apicompat.AnthropicContentBlock{
											Type: "text",
											Text: "",
									REDACTED,
								REDACTED) {
										return &geminiStreamResult{usage: &usage, firstTokenMs: firstTokenMsREDACTED, nil
								REDACTED
							REDACTED
								if emitAnthropicEvent(&apicompat.AnthropicStreamEvent{
									Type: "content_block_delta",
									Delta: &apicompat.AnthropicDelta{
										Type: "text_delta",
										Text: delta,
								REDACTED,
							REDACTED) {
									return &geminiStreamResult{usage: &usage, firstTokenMs: firstTokenMsREDACTED, nil
							REDACTED
								continue
						REDACTED

							if fc, ok := part["functionCall"].(map[string]any); ok && fc != nil {
								name, _ := fc["name"].(string)
								if strings.TrimSpace(name) == "" {
									name = "tool"
							REDACTED
								if closeOpenBlock() {
									return &geminiStreamResult{usage: &usage, firstTokenMs: firstTokenMsREDACTED, nil
							REDACTED
								if openToolIndex >= 0 && openToolName != name {
									if closeOpenTool() {
										return &geminiStreamResult{usage: &usage, firstTokenMs: firstTokenMsREDACTED, nil
								REDACTED
							REDACTED
								if openToolIndex < 0 {
									idx := nextBlockIndex
									nextBlockIndex++
									openToolIndex = idx
									openToolName = name
									sawToolUse = true
									if emitAnthropicEvent(&apicompat.AnthropicStreamEvent{
										Type:  "content_block_start",
										Index: &idx,
										ContentBlock: &apicompat.AnthropicContentBlock{
											Type:  "tool_use",
											ID:    "toolu_" + randomHex(8),
											Name:  name,
											Input: json.RawMessage(`{REDACTED`),
									REDACTED,
								REDACTED) {
										return &geminiStreamResult{usage: &usage, firstTokenMs: firstTokenMsREDACTED, nil
								REDACTED
							REDACTED

								argsJSONText := "{REDACTED"
								switch v := fc["args"].(type) {
								case nil:
								case string:
									if strings.TrimSpace(v) != "" {
										argsJSONText = v
								REDACTED
								default:
									if b, err := json.Marshal(v); err == nil && len(b) > 0 {
										argsJSONText = string(b)
								REDACTED
							REDACTED
								delta, newSeen := computeGeminiTextDelta(seenToolJSON, argsJSONText)
								seenToolJSON = newSeen
								if delta != "" {
									if emitAnthropicEvent(&apicompat.AnthropicStreamEvent{
										Type: "content_block_delta",
										Delta: &apicompat.AnthropicDelta{
											Type:        "input_json_delta",
											PartialJSON: delta,
									REDACTED,
								REDACTED) {
										return &geminiStreamResult{usage: &usage, firstTokenMs: firstTokenMsREDACTED, nil
								REDACTED
							REDACTED
						REDACTED
					REDACTED
				REDACTED
			REDACTED
		REDACTED
	REDACTED

		if errors.Is(err, io.EOF) {
			break
	REDACTED
		if err != nil {
			return nil, fmt.Errorf("stream read error: %w", err)
	REDACTED
REDACTED

	if closeOpenBlock() {
		return &geminiStreamResult{usage: &usage, firstTokenMs: firstTokenMsREDACTED, nil
REDACTED
	if closeOpenTool() {
		return &geminiStreamResult{usage: &usage, firstTokenMs: firstTokenMsREDACTED, nil
REDACTED

	stopReason := mapGeminiFinishReasonToClaudeStopReason(finishReason)
	if sawToolUse {
		stopReason = "tool_use"
REDACTED
	anthState.InputTokens = usage.InputTokens
	anthState.CacheReadInputTokens = usage.CacheReadInputTokens
	if emitAnthropicEvent(&apicompat.AnthropicStreamEvent{
		Type: "message_delta",
		Delta: &apicompat.AnthropicDelta{
			Type:       "message_delta",
			StopReason: stopReason,
	REDACTED,
		Usage: &apicompat.AnthropicUsage{
			InputTokens:          usage.InputTokens,
			OutputTokens:         usage.OutputTokens,
			CacheReadInputTokens: usage.CacheReadInputTokens,
	REDACTED,
REDACTED) {
		return &geminiStreamResult{usage: &usage, firstTokenMs: firstTokenMsREDACTED, nil
REDACTED
	if emitAnthropicEvent(&apicompat.AnthropicStreamEvent{Type: "message_stop"REDACTED) {
		return &geminiStreamResult{usage: &usage, firstTokenMs: firstTokenMsREDACTED, nil
REDACTED

	for _, resEvt := range apicompat.FinalizeAnthropicResponsesStream(anthState) {
		chunks := apicompat.ResponsesEventToChatChunks(&resEvt, ccState)
		for _, chunk := range chunks {
			if disconnected := writeChatChunk(chunk); disconnected {
				return &geminiStreamResult{usage: &usage, firstTokenMs: firstTokenMsREDACTED, nil
		REDACTED
	REDACTED
REDACTED
	for _, chunk := range apicompat.FinalizeResponsesChatStream(ccState) {
		if disconnected := writeChatChunk(chunk); disconnected {
			return &geminiStreamResult{usage: &usage, firstTokenMs: firstTokenMsREDACTED, nil
	REDACTED
REDACTED

	_, _ = io.WriteString(c.Writer, "data: [DONE]\n\n")
	flusher.Flush()

	return &geminiStreamResult{usage: &usage, firstTokenMs: firstTokenMsREDACTED, nil
REDACTED

func (s *GeminiMessagesCompatService) writeGeminiChatCompletionsMappedError(
	c *gin.Context,
	account *Account,
	upstreamStatus int,
	upstreamRequestID string,
	body []byte,
) error {
	upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(body)))
	setOpsUpstreamError(c, upstreamStatus, upstreamMsg, "")
	if account != nil {
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: upstreamStatus,
			UpstreamRequestID:  upstreamRequestID,
			Kind:               "http_error",
			Message:            upstreamMsg,
	REDACTED)
REDACTED

	if status, errType, errMsg, matched := applyErrorPassthroughRule(
		c,
		PlatformGemini,
		upstreamStatus,
		body,
		http.StatusBadGateway,
		"upstream_error",
		"Upstream request failed",
	); matched {
		return s.writeChatCompletionsError(c, status, errType, errMsg)
REDACTED

	statusCode := http.StatusBadGateway
	errType := "upstream_error"
	errMsg := "Upstream request failed"
	if mapped := mapGeminiErrorBodyToClaudeError(body); mapped != nil {
		if mapped.Type != "" {
			errType = mapped.Type
	REDACTED
		if mapped.Message != "" {
			errMsg = mapped.Message
	REDACTED
		if mapped.StatusCode > 0 {
			statusCode = mapped.StatusCode
	REDACTED
REDACTED

	switch upstreamStatus {
	case http.StatusBadRequest:
		if statusCode == http.StatusBadGateway {
			statusCode = http.StatusBadRequest
	REDACTED
		if errType == "upstream_error" {
			errType = "invalid_request_error"
	REDACTED
		if errMsg == "Upstream request failed" {
			errMsg = "Invalid request"
	REDACTED
	case http.StatusNotFound:
		statusCode = http.StatusNotFound
		if errType == "upstream_error" {
			errType = "not_found_error"
	REDACTED
		if errMsg == "Upstream request failed" {
			errMsg = "Resource not found"
	REDACTED
	case http.StatusTooManyRequests:
		statusCode = http.StatusTooManyRequests
		if errType == "upstream_error" {
			errType = "rate_limit_error"
	REDACTED
		if errMsg == "Upstream request failed" {
			errMsg = "Upstream rate limit exceeded, please retry later"
	REDACTED
	case 529:
		statusCode = http.StatusServiceUnavailable
		if errType == "upstream_error" {
			errType = "overloaded_error"
	REDACTED
		if errMsg == "Upstream request failed" {
			errMsg = "Upstream service overloaded, please retry later"
	REDACTED
REDACTED

	if upstreamMsg != "" && errMsg == "Upstream request failed" {
		errMsg = upstreamMsg
REDACTED
	return s.writeChatCompletionsError(c, statusCode, errType, errMsg)
REDACTED

func (s *GeminiMessagesCompatService) writeChatCompletionsError(c *gin.Context, status int, errType, message string) error {
	c.JSON(status, gin.H{
		"error": gin.H{
			"type":    errType,
			"message": message,
	REDACTED,
REDACTED)
	return fmt.Errorf("%s", message)
REDACTED
