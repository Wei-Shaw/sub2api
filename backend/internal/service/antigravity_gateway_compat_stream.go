package service

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
)

type antigravityCompatStreamAdapter interface {
	Emit(*apicompat.AnthropicStreamEvent, *antigravityClientWriter)
	Finalize(*antigravityClientWriter)
	WriteError(*antigravityClientWriter, string)
REDACTED

type antigravityChatStreamAdapter struct {
	anthropicState *apicompat.AnthropicEventToResponsesState
	chatState      *apicompat.ResponsesEventToChatState
REDACTED

func newAntigravityChatStreamAdapter(model string, includeUsage bool) *antigravityChatStreamAdapter {
	anthropicState := apicompat.NewAnthropicEventToResponsesState()
	anthropicState.Model = model
	chatState := apicompat.NewResponsesEventToChatState()
	chatState.Model = model
	chatState.IncludeUsage = includeUsage
	return &antigravityChatStreamAdapter{
		anthropicState: anthropicState,
		chatState:      chatState,
REDACTED
REDACTED

func (a *antigravityChatStreamAdapter) Emit(event *apicompat.AnthropicStreamEvent, writer *antigravityClientWriter) {
	for _, responseEvent := range apicompat.AnthropicEventToResponsesEvents(event, a.anthropicState) {
		a.emitResponseEvent(&responseEvent, writer)
REDACTED
REDACTED

func (a *antigravityChatStreamAdapter) Finalize(writer *antigravityClientWriter) {
	for _, responseEvent := range apicompat.FinalizeAnthropicResponsesStream(a.anthropicState) {
		a.emitResponseEvent(&responseEvent, writer)
REDACTED
	for _, chunk := range apicompat.FinalizeResponsesChatStream(a.chatState) {
		if data, err := apicompat.ChatChunkToSSE(chunk); err == nil {
			writer.Write([]byte(data))
	REDACTED
REDACTED
	writer.Write([]byte("data: [DONE]\n\n"))
REDACTED

func (a *antigravityChatStreamAdapter) WriteError(writer *antigravityClientWriter, reason string) {
	writer.Fprintf("data: {\"error\":{\"message\":%q,\"type\":\"upstream_error\"REDACTEDREDACTED\n\n", reason)
REDACTED

func (a *antigravityChatStreamAdapter) emitResponseEvent(event *apicompat.ResponsesStreamEvent, writer *antigravityClientWriter) {
	for _, chunk := range apicompat.ResponsesEventToChatChunks(event, a.chatState) {
		if data, err := apicompat.ChatChunkToSSE(chunk); err == nil {
			writer.Write([]byte(data))
	REDACTED
REDACTED
REDACTED

type antigravityResponsesStreamAdapter struct {
	anthropicState *apicompat.AnthropicEventToResponsesState
REDACTED

func newAntigravityResponsesStreamAdapter(model string) *antigravityResponsesStreamAdapter {
	state := apicompat.NewAnthropicEventToResponsesState()
	state.Model = model
	return &antigravityResponsesStreamAdapter{anthropicState: stateREDACTED
REDACTED

func (a *antigravityResponsesStreamAdapter) Emit(event *apicompat.AnthropicStreamEvent, writer *antigravityClientWriter) {
	for _, responseEvent := range apicompat.AnthropicEventToResponsesEvents(event, a.anthropicState) {
		a.emitResponseEvent(responseEvent, writer)
REDACTED
REDACTED

func (a *antigravityResponsesStreamAdapter) Finalize(writer *antigravityClientWriter) {
	for _, responseEvent := range apicompat.FinalizeAnthropicResponsesStream(a.anthropicState) {
		a.emitResponseEvent(responseEvent, writer)
REDACTED
REDACTED

func (a *antigravityResponsesStreamAdapter) WriteError(writer *antigravityClientWriter, reason string) {
	writer.Fprintf("event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"upstream_error\",\"message\":%qREDACTEDREDACTED\n\n", reason)
REDACTED

func (a *antigravityResponsesStreamAdapter) emitResponseEvent(event apicompat.ResponsesStreamEvent, writer *antigravityClientWriter) {
	if data, err := apicompat.ResponsesEventToSSE(event); err == nil {
		writer.Write([]byte(data))
REDACTED
REDACTED

type antigravityCompatScanEvent struct {
	line string
	err  error
REDACTED

type antigravityCompatStreamSession struct {
	processor      *antigravity.StreamingProcessor
	adapter        antigravityCompatStreamAdapter
	writer         *antigravityClientWriter
	usage          *ClaudeUsage
	pendingEvents  []apicompat.AnthropicStreamEvent
	firstTokenMs   *int
	startTime      time.Time
	meaningfulData bool
REDACTED

func newAntigravityCompatStreamSession(
	model string,
	startTime time.Time,
	adapter antigravityCompatStreamAdapter,
	writer *antigravityClientWriter,
) *antigravityCompatStreamSession {
	return &antigravityCompatStreamSession{
		processor: antigravity.NewStreamingProcessor(model),
		adapter:   adapter,
		writer:    writer,
		usage:     &ClaudeUsage{REDACTED,
		startTime: startTime,
REDACTED
REDACTED

func (s *antigravityCompatStreamSession) consume(line string) {
	claudeEvents := s.processor.ProcessLine(strings.TrimRight(line, "\r\n"))
	if len(claudeEvents) == 0 {
		return
REDACTED
	s.consumeClaudeEvents(claudeEvents)
REDACTED

func (s *antigravityCompatStreamSession) hasMeaningfulData() bool {
	return s.meaningfulData
REDACTED

func (s *antigravityCompatStreamSession) finish() *antigravityStreamResult {
	finalEvents, usage := s.processor.Finish()
	mergeAntigravityCompatUsage(s.usage, usage)
	s.consumeClaudeEvents(finalEvents)
	s.adapter.Finalize(s.writer)
	return s.result(s.writer.Disconnected())
REDACTED

func (s *antigravityCompatStreamSession) collectResult(clientDisconnect bool) *antigravityStreamResult {
	_, usage := s.processor.Finish()
	mergeAntigravityCompatUsage(s.usage, usage)
	return s.result(clientDisconnect)
REDACTED

func (s *antigravityCompatStreamSession) result(clientDisconnect bool) *antigravityStreamResult {
	return &antigravityStreamResult{
		usage:            s.usage,
		firstTokenMs:     s.firstTokenMs,
		clientDisconnect: clientDisconnect,
REDACTED
REDACTED

func (s *antigravityCompatStreamSession) consumeClaudeEvents(data []byte) {
	var eventType string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "event:"):
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			s.consumeClaudeData(eventType, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
	REDACTED
REDACTED
REDACTED

func (s *antigravityCompatStreamSession) consumeClaudeData(eventType, payload string) {
	var event apicompat.AnthropicStreamEvent
	if json.Unmarshal([]byte(payload), &event) != nil {
		return
REDACTED
	if event.Type == "" {
		event.Type = eventType
REDACTED
	if event.Usage != nil {
		mergeAnthropicUsage(s.usage, *event.Usage)
REDACTED
	if event.Message != nil {
		mergeAnthropicUsage(s.usage, event.Message.Usage)
REDACTED
	s.emitOrBuffer(event)
REDACTED

func (s *antigravityCompatStreamSession) emitOrBuffer(event apicompat.AnthropicStreamEvent) {
	if s.meaningfulData {
		s.adapter.Emit(&event, s.writer)
		return
REDACTED

	s.pendingEvents = append(s.pendingEvents, event)
	if !isMeaningfulAntigravityCompatEvent(&event) {
		return
REDACTED

	s.meaningfulData = true
	ms := int(time.Since(s.startTime).Milliseconds())
	s.firstTokenMs = &ms
	for i := range s.pendingEvents {
		s.adapter.Emit(&s.pendingEvents[i], s.writer)
REDACTED
	s.pendingEvents = nil
REDACTED

func isMeaningfulAntigravityCompatEvent(event *apicompat.AnthropicStreamEvent) bool {
	if event == nil {
		return false
REDACTED
	if event.Type == "message_stop" {
		return true
REDACTED
	if event.ContentBlock != nil {
		block := event.ContentBlock
		return block.Type == "tool_use" ||
			block.Text != "" ||
			block.Thinking != "" ||
			block.Signature != "" ||
			block.Source != nil
REDACTED
	if event.Delta != nil {
		delta := event.Delta
		return delta.Text != "" ||
			delta.PartialJSON != "" ||
			delta.Thinking != "" ||
			delta.Signature != "" ||
			delta.StopReason != ""
REDACTED
	return false
REDACTED

func mergeAntigravityCompatUsage(dst *ClaudeUsage, src *antigravity.ClaudeUsage) {
	if dst == nil || src == nil {
		return
REDACTED
	dst.InputTokens = src.InputTokens
	dst.OutputTokens = src.OutputTokens
	dst.CacheCreationInputTokens = src.CacheCreationInputTokens
	dst.CacheReadInputTokens = src.CacheReadInputTokens
	dst.ImageOutputTokens = src.ImageOutputTokens
REDACTED

func (s *AntigravityGatewayService) handleAntigravityCompatStream(
	c *gin.Context,
	resp *http.Response,
	startTime time.Time,
	originalModel string,
	adapter antigravityCompatStreamAdapter,
	prefix string,
) (*antigravityStreamResult, error) {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
REDACTED

	writer := newAntigravityClientWriter(c.Writer, flusher, prefix)
	writer.beforeFirstWrite = func() {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
		c.Status(http.StatusOK)
REDACTED
	session := newAntigravityCompatStreamSession(originalModel, startTime, adapter, writer)
	events, stopScanner, maxLineSize := s.startAntigravityCompatScanner(resp.Body)
	defer stopScanner()

	timeout := s.antigravityCompatStreamTimeout()
	timeoutTimer, timeoutCh := newAntigravityCompatTimer(timeout)
	if timeoutTimer != nil {
		defer timeoutTimer.Stop()
REDACTED
	keepaliveTicker, keepaliveCh := s.newAntigravityCompatKeepaliveTicker()
	if keepaliveTicker != nil {
		defer keepaliveTicker.Stop()
REDACTED

	for {
		select {
		case event, open := <-events:
			if !open {
				if !session.hasMeaningfulData() && !writer.Disconnected() {
					return nil, antigravityCompatEmptyStreamError()
			REDACTED
				return session.finish(), nil
		REDACTED
			if event.err != nil {
				return s.handleAntigravityCompatReadError(c, session, event.err, maxLineSize, prefix)
		REDACTED
			resetAntigravityCompatTimer(timeoutTimer, timeout)
			session.consume(event.line)

		case <-timeoutCh:
			if writer.Disconnected() {
				return session.collectResult(true), nil
		REDACTED
			if !session.hasMeaningfulData() {
				return nil, antigravityCompatEmptyStreamError()
		REDACTED
			logger.LegacyPrintf("service.antigravity_gateway", "Stream data interval timeout (%s)", prefix)
			writeAntigravityCompatStreamError(c, adapter, writer, "stream_timeout")
			return session.collectResult(false), fmt.Errorf("stream data interval timeout")

		case <-keepaliveCh:
			if session.hasMeaningfulData() && !writer.Disconnected() {
				writer.Write([]byte(": ping\n\n"))
		REDACTED
	REDACTED
REDACTED
REDACTED

func (s *AntigravityGatewayService) startAntigravityCompatScanner(
	body io.Reader,
) (<-chan antigravityCompatScanEvent, func(), int) {
	maxLineSize := defaultMaxLineSize
	if s.settingService != nil && s.settingService.cfg != nil && s.settingService.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.settingService.cfg.Gateway.MaxLineSize
REDACTED
	scanner := bufio.NewScanner(body)
	scanBuf := getSSEScannerBuf64K()
	scanner.Buffer(scanBuf[:0], maxLineSize)

	events := make(chan antigravityCompatScanEvent, 16)
	done := make(chan struct{REDACTED)
	go func() {
		defer putSSEScannerBuf64K(scanBuf)
		defer close(events)
		send := func(event antigravityCompatScanEvent) bool {
			select {
			case events <- event:
				return true
			case <-done:
				return false
		REDACTED
	REDACTED
		for scanner.Scan() {
			if !send(antigravityCompatScanEvent{line: scanner.Text()REDACTED) {
				return
		REDACTED
	REDACTED
		if err := scanner.Err(); err != nil {
			send(antigravityCompatScanEvent{err: errREDACTED)
	REDACTED
REDACTED()
	return events, func() { close(done) REDACTED, maxLineSize
REDACTED

func (s *AntigravityGatewayService) antigravityCompatStreamTimeout() time.Duration {
	if s.settingService == nil || s.settingService.cfg == nil {
		return 0
REDACTED
	return time.Duration(s.settingService.cfg.Gateway.StreamDataIntervalTimeout) * time.Second
REDACTED

func (s *AntigravityGatewayService) newAntigravityCompatKeepaliveTicker() (*time.Ticker, <-chan time.Time) {
	if s.settingService == nil || s.settingService.cfg == nil {
		return nil, nil
REDACTED
	interval := time.Duration(s.settingService.cfg.Gateway.StreamKeepaliveInterval) * time.Second
	if interval <= 0 {
		return nil, nil
REDACTED
	ticker := time.NewTicker(interval)
	return ticker, ticker.C
REDACTED

func newAntigravityCompatTimer(timeout time.Duration) (*time.Timer, <-chan time.Time) {
	if timeout <= 0 {
		return nil, nil
REDACTED
	timer := time.NewTimer(timeout)
	return timer, timer.C
REDACTED

func resetAntigravityCompatTimer(timer *time.Timer, timeout time.Duration) {
	if timer == nil {
		return
REDACTED
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
	REDACTED
REDACTED
	timer.Reset(timeout)
REDACTED

func (s *AntigravityGatewayService) handleAntigravityCompatReadError(
	c *gin.Context,
	session *antigravityCompatStreamSession,
	err error,
	maxLineSize int,
	prefix string,
) (*antigravityStreamResult, error) {
	if !session.hasMeaningfulData() && !session.writer.Disconnected() {
		return nil, antigravityCompatEmptyStreamError()
REDACTED
	if disconnect, handled := handleStreamReadError(err, session.writer.Disconnected(), prefix); handled {
		return session.collectResult(disconnect), nil
REDACTED
	if errors.Is(err, bufio.ErrTooLong) {
		logger.LegacyPrintf("service.antigravity_gateway", "SSE line too long (%s): max_size=%d error=%v", prefix, maxLineSize, err)
		writeAntigravityCompatStreamError(c, session.adapter, session.writer, "response_too_large")
		return session.result(false), err
REDACTED
	writeAntigravityCompatStreamError(c, session.adapter, session.writer, "stream_read_error")
	return nil, fmt.Errorf("stream read error: %w", err)
REDACTED

func writeAntigravityCompatStreamError(
	c *gin.Context,
	adapter antigravityCompatStreamAdapter,
	writer *antigravityClientWriter,
	reason string,
) {
	adapter.WriteError(writer, reason)
	MarkResponseCommitted(c)
REDACTED

func antigravityCompatEmptyStreamError() error {
	logger.LegacyPrintf("service.antigravity_gateway", "Empty Antigravity compatibility stream, triggering failover")
	return &UpstreamFailoverError{
		StatusCode:             http.StatusBadGateway,
		ResponseBody:           []byte(`{"error":"empty stream response from upstream"REDACTED`),
		RetryableOnSameAccount: true,
REDACTED
REDACTED

func (s *AntigravityGatewayService) handleChatCompletionsStreamingFromAntigravity(
	c *gin.Context,
	resp *http.Response,
	startTime time.Time,
	originalModel string,
	includeUsage bool,
) (*antigravityStreamResult, error) {
	return s.handleAntigravityCompatStream(
		c,
		resp,
		startTime,
		originalModel,
		newAntigravityChatStreamAdapter(originalModel, includeUsage),
		"antigravity chat completions stream",
	)
REDACTED

func (s *AntigravityGatewayService) handleResponsesStreamingFromAntigravity(
	c *gin.Context,
	resp *http.Response,
	startTime time.Time,
	originalModel string,
) (*antigravityStreamResult, error) {
	return s.handleAntigravityCompatStream(
		c,
		resp,
		startTime,
		originalModel,
		newAntigravityResponsesStreamAdapter(originalModel),
		"antigravity responses stream",
	)
REDACTED
