package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
)

type chatStreamingResult struct {
	usage        *OpenAIUsage
	firstTokenMs *int
REDACTED

func (s *OpenAIGatewayService) forwardChatCompletions(ctx context.Context, c *gin.Context, account *Account, body []byte, includeUsage bool, startTime time.Time) (*OpenAIForwardResult, error) {
	// Parse request body once (avoid multiple parse/serialize cycles)
	var reqBody map[string]any
	if err := json.Unmarshal(body, &reqBody); err != nil {
		return nil, fmt.Errorf("parse request: %w", err)
REDACTED

	reqModel, _ := reqBody["model"].(string)
	reqStream, _ := reqBody["stream"].(bool)
	originalModel := reqModel

	bodyModified := false
	mappedModel := account.GetMappedModel(reqModel)
	if mappedModel != reqModel {
		log.Printf("[OpenAI Chat] Model mapping applied: %s -> %s (account: %s)", reqModel, mappedModel, account.Name)
		reqBody["model"] = mappedModel
		bodyModified = true
REDACTED

	if reqStream && includeUsage {
		streamOptions, _ := reqBody["stream_options"].(map[string]any)
		if streamOptions == nil {
			streamOptions = map[string]any{REDACTED
	REDACTED
		if _, ok := streamOptions["include_usage"]; !ok {
			streamOptions["include_usage"] = true
			reqBody["stream_options"] = streamOptions
			bodyModified = true
	REDACTED
REDACTED

	if bodyModified {
		var err error
		body, err = json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("serialize request body: %w", err)
	REDACTED
REDACTED

	// Get access token
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
REDACTED

	upstreamReq, err := s.buildChatCompletionsRequest(ctx, c, account, body, token)
	if err != nil {
		return nil, err
REDACTED

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED

	if c != nil {
		c.Set(OpsUpstreamRequestBodyKey, string(body))
REDACTED

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
	REDACTED)
		c.JSON(http.StatusBadGateway, gin.H{
			"error": gin.H{
				"type":    "upstream_error",
				"message": "Upstream request failed",
		REDACTED,
	REDACTED)
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	if resp.StatusCode >= 400 {
		if s.shouldFailoverUpstreamError(resp.StatusCode) {
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
			_ = resp.Body.Close()
			resp.Body = io.NopCloser(bytes.NewReader(respBody))

			upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
			upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
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

			s.handleFailoverSideEffects(ctx, resp, account)
			return nil, &UpstreamFailoverError{StatusCode: resp.StatusCodeREDACTED
	REDACTED
		return s.handleErrorResponse(ctx, resp, c, account, body)
REDACTED

	var usage *OpenAIUsage
	var firstTokenMs *int
	if reqStream {
		streamResult, err := s.handleChatCompletionsStreamingResponse(ctx, resp, c, account, startTime, originalModel, mappedModel)
		if err != nil {
			return nil, err
	REDACTED
		usage = streamResult.usage
		firstTokenMs = streamResult.firstTokenMs
REDACTED else {
		usage, err = s.handleChatCompletionsNonStreamingResponse(resp, c, originalModel, mappedModel)
		if err != nil {
			return nil, err
	REDACTED
REDACTED

	if usage == nil {
		usage = &OpenAIUsage{REDACTED
REDACTED

	return &OpenAIForwardResult{
		RequestID:    resp.Header.Get("x-request-id"),
		Usage:        *usage,
		Model:        originalModel,
		Stream:       reqStream,
		Duration:     time.Since(startTime),
		FirstTokenMs: firstTokenMs,
REDACTED, nil
REDACTED

func (s *OpenAIGatewayService) buildChatCompletionsRequest(ctx context.Context, c *gin.Context, account *Account, body []byte, token string) (*http.Request, error) {
	var targetURL string
	baseURL := account.GetOpenAIBaseURL()
	if baseURL == "" {
		targetURL = openaiChatAPIURL
REDACTED else {
		validatedURL, err := s.validateUpstreamBaseURL(baseURL)
		if err != nil {
			return nil, err
	REDACTED
		targetURL = validatedURL + "/chat/completions"
REDACTED

	req, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
REDACTED

	req.Header.Set("authorization", "Bearer "+token)

	for key, values := range c.Request.Header {
		lowerKey := strings.ToLower(key)
		if openaiChatAllowedHeaders[lowerKey] {
			for _, v := range values {
				req.Header.Add(key, v)
		REDACTED
	REDACTED
REDACTED

	customUA := account.GetOpenAIUserAgent()
	if customUA != "" {
		req.Header.Set("user-agent", customUA)
REDACTED

	if req.Header.Get("content-type") == "" {
		req.Header.Set("content-type", "application/json")
REDACTED

	return req, nil
REDACTED

func (s *OpenAIGatewayService) handleChatCompletionsStreamingResponse(ctx context.Context, resp *http.Response, c *gin.Context, account *Account, startTime time.Time, originalModel, mappedModel string) (*chatStreamingResult, error) {
	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
REDACTED

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	if v := resp.Header.Get("x-request-id"); v != "" {
		c.Header("x-request-id", v)
REDACTED

	w := c.Writer
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
REDACTED

	usage := &OpenAIUsage{REDACTED
	var firstTokenMs *int

	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
REDACTED
	scanner.Buffer(make([]byte, 64*1024), maxLineSize)

	type scanEvent struct {
		line string
		err  error
REDACTED
	events := make(chan scanEvent, 16)
	done := make(chan struct{REDACTED)
	sendEvent := func(ev scanEvent) bool {
		select {
		case events <- ev:
			return true
		case <-done:
			return false
	REDACTED
REDACTED
	var lastReadAt int64
	atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
	go func() {
		defer close(events)
		for scanner.Scan() {
			atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
			if !sendEvent(scanEvent{line: scanner.Text()REDACTED) {
				return
		REDACTED
	REDACTED
		if err := scanner.Err(); err != nil {
			_ = sendEvent(scanEvent{err: errREDACTED)
	REDACTED
REDACTED()
	defer close(done)

	streamInterval := time.Duration(0)
	if s.cfg != nil && s.cfg.Gateway.StreamDataIntervalTimeout > 0 {
		streamInterval = time.Duration(s.cfg.Gateway.StreamDataIntervalTimeout) * time.Second
REDACTED
	var intervalTicker *time.Ticker
	if streamInterval > 0 {
		intervalTicker = time.NewTicker(streamInterval)
		defer intervalTicker.Stop()
REDACTED
	var intervalCh <-chan time.Time
	if intervalTicker != nil {
		intervalCh = intervalTicker.C
REDACTED

	keepaliveInterval := time.Duration(0)
	if s.cfg != nil && s.cfg.Gateway.StreamKeepaliveInterval > 0 {
		keepaliveInterval = time.Duration(s.cfg.Gateway.StreamKeepaliveInterval) * time.Second
REDACTED
	var keepaliveTicker *time.Ticker
	if keepaliveInterval > 0 {
		keepaliveTicker = time.NewTicker(keepaliveInterval)
		defer keepaliveTicker.Stop()
REDACTED
	var keepaliveCh <-chan time.Time
	if keepaliveTicker != nil {
		keepaliveCh = keepaliveTicker.C
REDACTED
	lastDataAt := time.Now()

	errorEventSent := false
	sendErrorEvent := func(reason string) {
		if errorEventSent {
			return
	REDACTED
		errorEventSent = true
		_, _ = fmt.Fprintf(w, "event: error\ndata: {\"error\":\"%s\"REDACTED\n\n", reason)
		flusher.Flush()
REDACTED

	needModelReplace := originalModel != mappedModel

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return &chatStreamingResult{usage: usage, firstTokenMs: firstTokenMsREDACTED, nil
		REDACTED
			if ev.err != nil {
				if errors.Is(ev.err, bufio.ErrTooLong) {
					log.Printf("SSE line too long: account=%d max_size=%d error=%v", account.ID, maxLineSize, ev.err)
					sendErrorEvent("response_too_large")
					return &chatStreamingResult{usage: usage, firstTokenMs: firstTokenMsREDACTED, ev.err
			REDACTED
				sendErrorEvent("stream_read_error")
				return &chatStreamingResult{usage: usage, firstTokenMs: firstTokenMsREDACTED, fmt.Errorf("stream read error: %w", ev.err)
		REDACTED

			line := ev.line
			lastDataAt = time.Now()

			if openaiSSEDataRe.MatchString(line) {
				data := openaiSSEDataRe.ReplaceAllString(line, "")

				if needModelReplace {
					line = s.replaceModelInSSELine(line, mappedModel, originalModel)
			REDACTED

				if correctedData, corrected := s.toolCorrector.CorrectToolCallsInSSEData(data); corrected {
					line = "data: " + correctedData
			REDACTED

				if _, err := fmt.Fprintf(w, "%s\n", line); err != nil {
					sendErrorEvent("write_failed")
					return &chatStreamingResult{usage: usage, firstTokenMs: firstTokenMsREDACTED, err
			REDACTED
				flusher.Flush()

				if firstTokenMs == nil {
					if event := parseChatStreamEvent(data); event != nil {
						if chatChunkHasDelta(event) {
							ms := int(time.Since(startTime).Milliseconds())
							firstTokenMs = &ms
					REDACTED
						applyChatUsageFromEvent(event, usage)
				REDACTED
			REDACTED else {
					if event := parseChatStreamEvent(data); event != nil {
						applyChatUsageFromEvent(event, usage)
				REDACTED
			REDACTED
		REDACTED else {
				if _, err := fmt.Fprintf(w, "%s\n", line); err != nil {
					sendErrorEvent("write_failed")
					return &chatStreamingResult{usage: usage, firstTokenMs: firstTokenMsREDACTED, err
			REDACTED
				flusher.Flush()
		REDACTED

		case <-intervalCh:
			lastRead := time.Unix(0, atomic.LoadInt64(&lastReadAt))
			if time.Since(lastRead) < streamInterval {
				continue
		REDACTED
			log.Printf("Stream data interval timeout: account=%d model=%s interval=%s", account.ID, originalModel, streamInterval)
			if s.rateLimitService != nil {
				s.rateLimitService.HandleStreamTimeout(ctx, account, originalModel)
		REDACTED
			sendErrorEvent("stream_timeout")
			return &chatStreamingResult{usage: usage, firstTokenMs: firstTokenMsREDACTED, fmt.Errorf("stream data interval timeout")

		case <-keepaliveCh:
			if time.Since(lastDataAt) < keepaliveInterval {
				continue
		REDACTED
			if _, err := fmt.Fprint(w, ":\n\n"); err != nil {
				return &chatStreamingResult{usage: usage, firstTokenMs: firstTokenMsREDACTED, err
		REDACTED
			flusher.Flush()
	REDACTED
REDACTED
REDACTED

func (s *OpenAIGatewayService) handleChatCompletionsNonStreamingResponse(resp *http.Response, c *gin.Context, originalModel, mappedModel string) (*OpenAIUsage, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
REDACTED

	usage := &OpenAIUsage{REDACTED
	var parsed map[string]any
	if json.Unmarshal(body, &parsed) == nil {
		if usageMap, ok := parsed["usage"].(map[string]any); ok {
			applyChatUsageFromMap(usageMap, usage)
	REDACTED
REDACTED

	if originalModel != mappedModel {
		body = s.replaceModelInResponseBody(body, mappedModel, originalModel)
REDACTED
	body = s.correctToolCallsInResponseBody(body)

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
REDACTED

	contentType := "application/json"
	if s.cfg != nil && !s.cfg.Security.ResponseHeaders.Enabled {
		if upstreamType := resp.Header.Get("Content-Type"); upstreamType != "" {
			contentType = upstreamType
	REDACTED
REDACTED

	c.Data(resp.StatusCode, contentType, body)
	return usage, nil
REDACTED

func parseChatStreamEvent(data string) map[string]any {
	if data == "" || data == "[DONE]" {
		return nil
REDACTED
	var event map[string]any
	if json.Unmarshal([]byte(data), &event) != nil {
		return nil
REDACTED
	return event
REDACTED

func chatChunkHasDelta(event map[string]any) bool {
	choices, ok := event["choices"].([]any)
	if !ok {
		return false
REDACTED
	for _, choice := range choices {
		choiceMap, ok := choice.(map[string]any)
		if !ok {
			continue
	REDACTED
		delta, ok := choiceMap["delta"].(map[string]any)
		if !ok {
			continue
	REDACTED
		if content, ok := delta["content"].(string); ok && strings.TrimSpace(content) != "" {
			return true
	REDACTED
		if toolCalls, ok := delta["tool_calls"].([]any); ok && len(toolCalls) > 0 {
			return true
	REDACTED
		if functionCall, ok := delta["function_call"].(map[string]any); ok && len(functionCall) > 0 {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func applyChatUsageFromEvent(event map[string]any, usage *OpenAIUsage) {
	if event == nil || usage == nil {
		return
REDACTED
	usageMap, ok := event["usage"].(map[string]any)
	if !ok {
		return
REDACTED
	applyChatUsageFromMap(usageMap, usage)
REDACTED

func applyChatUsageFromMap(usageMap map[string]any, usage *OpenAIUsage) {
	if usageMap == nil || usage == nil {
		return
REDACTED
	promptTokens := int(getNumber(usageMap["prompt_tokens"]))
	completionTokens := int(getNumber(usageMap["completion_tokens"]))
	if promptTokens > 0 {
		usage.InputTokens = promptTokens
REDACTED
	if completionTokens > 0 {
		usage.OutputTokens = completionTokens
REDACTED
REDACTED
