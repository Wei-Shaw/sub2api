package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"go.uber.org/zap"
)

// openaiStreamingResult streaming response result
type openaiStreamingResult struct {
	usage            *OpenAIUsage
	firstTokenMs     *int
	responseID       string
	imageCount       int
	imageOutputSizes []string
	searchCount      int
REDACTED

type openaiNonStreamingResult struct {
	*OpenAIUsage
	usage            *OpenAIUsage
	responseID       string
	imageCount       int
	imageOutputSizes []string
	searchCount      int
REDACTED

func (s *OpenAIGatewayService) handleStreamingResponse(ctx context.Context, resp *http.Response, c *gin.Context, account *Account, startTime time.Time, originalModel, mappedModel string) (*openaiStreamingResult, error) {
	return s.handleStreamingResponseWithReasoning(ctx, resp, c, account, startTime, originalModel, mappedModel, "")
REDACTED

func (s *OpenAIGatewayService) handleStreamingResponseWithReasoning(ctx context.Context, resp *http.Response, c *gin.Context, account *Account, startTime time.Time, originalModel, mappedModel, reasoningEffort string) (*openaiStreamingResult, error) {
	observer := upstreamResponseModelObserverFromContext(c)
	if observer == nil {
		observer = beginUpstreamResponseModelObservation(c)
REDACTED
	firstOutputTimeout := time.Duration(0)
	if account != nil && account.Platform == PlatformOpenAI {
		firstOutputTimeout = s.openAIFirstOutputTimeout(reasoningEffort)
REDACTED
	guardFirstOutput := firstOutputTimeout > 0
	stageFirstOutput := account != nil && account.Platform == PlatformOpenAI
	var attemptResponseHeaders http.Header
	if stageFirstOutput {
		if s.responseHeaderFilter != nil {
			attemptResponseHeaders = responseheaders.FilterHeaders(resp.Header, s.responseHeaderFilter)
	REDACTED else if requestID := strings.TrimSpace(resp.Header.Get("x-request-id")); requestID != "" {
			attemptResponseHeaders = http.Header{"X-Request-Id": []string{requestIDREDACTEDREDACTED
	REDACTED
REDACTED else if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
REDACTED
	// x-codex-turn-state 不在通用响应头白名单内，按 Codex 协议显式回传：
	// 客户端会在同回合的后续请求中回带（openai_codex_turn_state.go）。
	// OpenAI 首个语义输出前只暂存，溯源在 applyAttemptResponseHeaders 真正提交时记录。
	if stageFirstOutput {
		stageOpenAICodexTurnState(&attemptResponseHeaders, resp.Header)
REDACTED else {
		s.relayOpenAICodexTurnState(c, account, resp.Header)
REDACTED

	// Set SSE response headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	// Pass through other headers
	if !stageFirstOutput && resp.Header.Get("x-request-id") != "" {
		v := resp.Header.Get("x-request-id")
		c.Header("x-request-id", v)
REDACTED
	applyAttemptResponseHeaders := func() {
		if !stageFirstOutput || len(attemptResponseHeaders) == 0 || c.Writer.Written() {
			return
	REDACTED
		for key, values := range attemptResponseHeaders {
			for _, value := range values {
				c.Writer.Header().Add(key, value)
		REDACTED
	REDACTED
		// 暂存头此刻才真正写给客户端：turn-state 溯源在这里记录（见
		// noteStagedOpenAICodexTurnStateCommitted 的 failover 说明）。
		s.noteStagedOpenAICodexTurnStateCommitted(c, account, attemptResponseHeaders)
		// These headers describe this gateway's SSE stream and are stable across
		// account attempts. Keep them authoritative over upstream values.
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
REDACTED

	w := c.Writer
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
REDACTED
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
REDACTED
	var firstTokenMs *int
	firstOutputProgressObserved := false
	bufferedWriter := bufio.NewWriterSize(w, 4*1024)
	var firstOutputStage *openAIFirstOutputStage
	if stageFirstOutput {
		firstOutputStage = newDefaultOpenAIFirstOutputStage()
		defer func() {
			if err := firstOutputStage.Close(); err != nil {
				logger.LegacyPrintf("service.openai_gateway", "OpenAI first-output staging cleanup failed: account=%d model=%s error=%v", account.ID, originalModel, err)
		REDACTED
	REDACTED()
REDACTED
	writePendingString := func(value string) (int, error) {
		if firstOutputStage != nil && !firstOutputStage.closed {
			return firstOutputStage.WriteString(value)
	REDACTED
		return bufferedWriter.WriteString(value)
REDACTED
	pendingBytes := func() int64 {
		if firstOutputStage != nil && !firstOutputStage.closed {
			return firstOutputStage.Buffered()
	REDACTED
		return int64(bufferedWriter.Buffered())
REDACTED
	flushBuffered := func() error {
		if firstOutputStage != nil && !firstOutputStage.closed {
			if err := firstOutputStage.CommitTo(w); err != nil {
				return err
		REDACTED
	REDACTED else {
			if err := bufferedWriter.Flush(); err != nil {
				return err
		REDACTED
	REDACTED
		flusher.Flush()
		return nil
REDACTED

	usage := &OpenAIUsage{REDACTED
	imageCounter := newOpenAIImageOutputCounter()
	responseID := ""
	var firstOutputScanGuard atomic.Bool
	firstOutputScanGuard.Store(stageFirstOutput)
	scanner := bufio.NewScanner(resp.Body)
	scanBuf := getSSEScannerBuf64K()
	scanner.Buffer(scanBuf[:0], maxLineSize)
	if stageFirstOutput {
		scanner.Split(openAIFirstOutputDynamicScanLines(&firstOutputScanGuard))
REDACTED
	documentScanner := newOpenAISSEJSONDocumentScanner(scanner)

	streamInterval := time.Duration(0)
	if s.cfg != nil && s.cfg.Gateway.StreamDataIntervalTimeout > 0 {
		streamInterval = time.Duration(s.cfg.Gateway.StreamDataIntervalTimeout) * time.Second
REDACTED
	// Grok: always enforce an upstream-read idle so hung SSE bodies fail over
	// instead of holding the OAuth slot until the client cancels. Prefer the
	// global gateway setting when set; otherwise apply a Grok-only default.
	if account != nil && account.Platform == PlatformGrok {
		cfgSec := 0
		if s.cfg != nil {
			cfgSec = s.cfg.Gateway.StreamDataIntervalTimeout
	REDACTED
		streamInterval = resolveGrokStreamIdleTimeout(cfgSec)
REDACTED
	// 仅监控上游数据间隔超时，不被下游写入阻塞影响
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
	// 下游 keepalive 仅用于防止代理空闲断开
	var keepaliveTicker *time.Ticker
	if keepaliveInterval > 0 {
		keepaliveTicker = time.NewTicker(keepaliveInterval)
		defer keepaliveTicker.Stop()
REDACTED
	var keepaliveCh <-chan time.Time
	if keepaliveTicker != nil {
		keepaliveCh = keepaliveTicker.C
REDACTED

	var firstOutputTimer *time.Timer
	var firstOutputCh <-chan time.Time
	if firstOutputTimeout > 0 {
		remaining := time.Until(startTime.Add(firstOutputTimeout))
		if remaining <= 0 {
			remaining = time.Nanosecond
	REDACTED
		firstOutputTimer = time.NewTimer(remaining)
		firstOutputCh = firstOutputTimer.C
		defer firstOutputTimer.Stop()
REDACTED
	stopFirstOutputTimer := func() {
		if firstOutputTimer == nil {
			return
	REDACTED
		if !firstOutputTimer.Stop() {
			select {
			case <-firstOutputTimer.C:
			default:
		REDACTED
	REDACTED
		firstOutputTimer = nil
		firstOutputCh = nil
REDACTED
	// Track downstream writes separately from upstream reads: pre-output failover
	// can buffer response.created / response.in_progress, so keepalive must be
	// based on downstream idle time.
	lastDownstreamWriteAt := time.Now()

	// 仅发送一次错误事件，避免多次写入导致协议混乱。
	// 注意：OpenAI `/v1/responses` streaming 事件必须符合 OpenAI Responses schema；
	// 否则下游 SDK（例如 OpenCode）会因为类型校验失败而报错。
	errorEventSent := false
	clientDisconnected := false // 客户端断开后继续 drain 上游以收集 usage
	sawTerminalEvent := false
	sawFailedEvent := false
	sawBareError := false
	sawResponseFailed := false
	terminalEventType := ""
	responsesSemanticOutputSeen := false
	capacityFailoverSuppressedLogged := false
	failedMessage := ""
	clientOutputStarted := false
	codexFailureTerminal := account != nil && account.IsOpenAIOAuthLike()
	upstreamRequestID := strings.TrimSpace(resp.Header.Get("x-request-id"))
	var streamEarlyErr error
	terminalFailurePending := false
	failureDelivered := false
	suppressCurrentEvent := false
	var bareErrorPayload []byte
	bareErrorAccountSideEffectsPending := false
	pendingSSEEventType := ""
	eventInProgress := false
	eventStartsClientOutput := false
	eventStartsVisibleOutput := false
	eventShouldFlush := false
	handlePendingWriteError := func(err error) {
		if firstOutputStage != nil && !firstOutputStage.closed {
			message := "OpenAI first-output staging failed"
			if errors.Is(err, errOpenAIFirstOutputStageLimit) {
				message = "OpenAI first-output staging limit exceeded"
		REDACTED
			logger.LegacyPrintf("service.openai_gateway", "%s: account=%d model=%s error=%v", message, account.ID, originalModel, err)
			failoverErr := s.newOpenAIStreamFailoverError(c, account, false, upstreamRequestID, nil, message)
			failoverErr.SafeToFailoverAfterWrite = true
			streamEarlyErr = failoverErr
			_ = resp.Body.Close()
			return
	REDACTED
		clientDisconnected = true
		logger.LegacyPrintf("service.openai_gateway", "Client disconnected during streaming, continuing to drain upstream for billing")
REDACTED
	completeGuardedEvent := func(queueDrained bool) {
		completedProgressEvent := eventStartsClientOutput
		completedVisibleEvent := eventStartsVisibleOutput
		shouldFlush := eventShouldFlush || (queueDrained && clientOutputStarted)
		eventInProgress = false
		if !clientDisconnected {
			if completedProgressEvent {
				applyAttemptResponseHeaders()
		REDACTED
			if shouldFlush {
				if err := flushBuffered(); err != nil {
					clientDisconnected = true
					logger.LegacyPrintf("service.openai_gateway", "Client disconnected during streaming flush, continuing to drain upstream for billing")
			REDACTED else {
					clientOutputStarted = true
					lastDownstreamWriteAt = time.Now()
			REDACTED
		REDACTED
	REDACTED
		if completedProgressEvent && !firstOutputProgressObserved {
			firstOutputScanGuard.Store(false)
			firstOutputProgressObserved = true
			stopFirstOutputTimer()
	REDACTED
		if completedVisibleEvent && firstTokenMs == nil {
			ms := int(time.Since(startTime).Milliseconds())
			firstTokenMs = &ms
	REDACTED
		eventStartsClientOutput = false
		eventStartsVisibleOutput = false
		eventShouldFlush = false
REDACTED
	sendErrorEvent := func(reason string) {
		if errorEventSent || clientDisconnected || failureDelivered {
			return
	REDACTED
		errorEventSent = true
		payload := `{"type":"error","sequence_number":0,"error":{"type":"upstream_error","message":` + strconv.Quote(reason) + `,"code":` + strconv.Quote(reason) + `REDACTEDREDACTED`
		if err := flushBuffered(); err != nil {
			clientDisconnected = true
			return
	REDACTED
		if _, err := writePendingString("data: " + payload + "\n\n"); err != nil {
			clientDisconnected = true
			return
	REDACTED
		if err := flushBuffered(); err != nil {
			clientDisconnected = true
			return
	REDACTED
		clientOutputStarted = true
		lastDownstreamWriteAt = time.Now()
REDACTED

	needModelReplace := originalModel != mappedModel
	streamOutputAccumulator := apicompat.NewBufferedResponseAccumulator()
	streamImageOutputs := make([]json.RawMessage, 0, 1)
	streamSeenImages := make(map[string]struct{REDACTED)
	searchCounter := 0
	// Dedup search tool calls across SSE events (item.done + response.completed
	// both list the same call_id — counting both would ~2× the surcharge).
	streamSearchSeen := make(map[string]struct{REDACTED)
	resultWithUsage := func() *openaiStreamingResult {
		return &openaiStreamingResult{
			usage:            usage,
			firstTokenMs:     firstTokenMs,
			responseID:       responseID,
			imageCount:       imageCounter.Count(),
			imageOutputSizes: imageCounter.Sizes(),
			searchCount:      searchCounter,
	REDACTED
REDACTED
	flushPending := func(disconnectMessage string) {
		if clientDisconnected || pendingBytes() == 0 {
			return
	REDACTED
		if err := flushBuffered(); err != nil {
			clientDisconnected = true
			logger.LegacyPrintf("service.openai_gateway", "%s", disconnectMessage)
			return
	REDACTED
		clientOutputStarted = true
		lastDownstreamWriteAt = time.Now()
REDACTED
	finalizeStream := func() (*openaiStreamingResult, error) {
		if stageFirstOutput && eventInProgress {
			// EOF dispatches the final SSE event even without a trailing blank line.
			completeGuardedEvent(true)
	REDACTED
		if codexFailureTerminal && sawBareError && !sawResponseFailed && bareErrorAccountSideEffectsPending {
			s.handleOpenAIStreamTerminalAccountSideEffects(c, account, bareErrorPayload, failedMessage, resp.Header)
			bareErrorAccountSideEffectsPending = false
	REDACTED
		if codexFailureTerminal && sawBareError && !sawResponseFailed && !clientDisconnected {
			applyAttemptResponseHeaders()
			if _, err := writePendingString(buildOpenAIResponseFailedSSE(responseID, originalModel, bareErrorPayload, failedMessage)); err != nil {
				handlePendingWriteError(err)
		REDACTED else {
				failureDelivered = true
		REDACTED
	REDACTED
		if sawTerminalEvent && !sawFailedEvent {
			s.clearOpenAIProxyStreamDisconnect(account)
	REDACTED
		if !sawTerminalEvent && !openAIStreamClientOutputStarted(c, clientOutputStarted) && !eventShouldFlush {
			return resultWithUsage(), s.newOpenAIStreamFailoverError(
				c,
				account,
				false,
				upstreamRequestID,
				nil,
				"OpenAI stream ended before a terminal event",
			)
	REDACTED
		flushPending("Client disconnected during final flush, returning collected usage")
		if !sawTerminalEvent {
			if openAIStreamClientOutputStarted(c, clientOutputStarted) && !clientDisconnected {
				s.recordOpenAIProxyStreamDisconnect(account, errors.New("stream ended before terminal event"), upstreamRequestID)
		REDACTED
			return resultWithUsage(), fmt.Errorf("stream usage incomplete: missing terminal event")
	REDACTED
		if sawFailedEvent {
			return resultWithUsage(), fmt.Errorf("upstream response failed: %s", failedMessage)
	REDACTED
		logOpenAISuccessMissingUsage(ctx, c, account, resp, usage, terminalEventType, clientDisconnected)
		return resultWithUsage(), nil
REDACTED
	handleScanErr := func(scanErr error) (*openaiStreamingResult, error, bool) {
		if scanErr == nil {
			return nil, nil, false
	REDACTED
		if errors.Is(scanErr, errOpenAIFirstOutputScannerLimit) && !firstOutputProgressObserved {
			logger.LegacyPrintf("service.openai_gateway", "SSE token exceeded guarded first-output limit: account=%d limit=%d error=%v", account.ID, openAIFirstOutputStageMaxBytes+openAIFirstOutputScannerFramingAllowance, scanErr)
			failoverErr := s.newOpenAIStreamFailoverError(
				c, account, false, upstreamRequestID, nil,
				"OpenAI SSE line exceeds guarded first-output limit",
			)
			failoverErr.SafeToFailoverAfterWrite = true
			return resultWithUsage(), failoverErr, true
	REDACTED
		if errors.Is(scanErr, bufio.ErrTooLong) && stageFirstOutput && !firstOutputProgressObserved {
			logger.LegacyPrintf("service.openai_gateway", "SSE line too long before first output: account=%d max_size=%d error=%v", account.ID, maxLineSize, scanErr)
			failoverErr := s.newOpenAIStreamFailoverError(
				c, account, false, upstreamRequestID, nil,
				"OpenAI SSE line exceeds guarded first-output limit",
			)
			failoverErr.SafeToFailoverAfterWrite = true
			return resultWithUsage(), failoverErr, true
	REDACTED
		if sawTerminalEvent {
			if !sawFailedEvent {
				s.clearOpenAIProxyStreamDisconnect(account)
				logger.LegacyPrintf("service.openai_gateway", "Upstream scan ended after terminal event: %v", scanErr)
		REDACTED
			result, err := finalizeStream()
			return result, err, true
	REDACTED
		// 客户端断开/取消请求时，上游读取往往会返回 context canceled。
		// /v1/responses 的 SSE 事件必须符合 OpenAI 协议；这里不注入自定义 error event，避免下游 SDK 解析失败。
		if errors.Is(scanErr, context.Canceled) || errors.Is(scanErr, context.DeadlineExceeded) {
			if eventShouldFlush {
				flushPending("Client disconnected during canceled stream flush, returning collected usage")
		REDACTED
			return resultWithUsage(), fmt.Errorf("stream usage incomplete: %w", scanErr), true
	REDACTED
		if errors.Is(scanErr, bufio.ErrTooLong) {
			logger.LegacyPrintf("service.openai_gateway", "SSE line too long: account=%d max_size=%d error=%v", account.ID, maxLineSize, scanErr)
			sendErrorEvent("response_too_large")
			return resultWithUsage(), scanErr, true
	REDACTED
		if !openAIStreamClientOutputStarted(c, clientOutputStarted) && !eventShouldFlush {
			msg := "OpenAI stream disconnected before completion"
			if errText := strings.TrimSpace(scanErr.Error()); errText != "" {
				msg += ": " + errText
		REDACTED
			return resultWithUsage(), s.newOpenAIStreamFailoverError(c, account, false, upstreamRequestID, nil, msg), true
	REDACTED
		// 客户端已断开时，上游出错仅影响体验，不影响计费；返回已收集 usage
		if clientDisconnected {
			return resultWithUsage(), fmt.Errorf("stream usage incomplete after disconnect: %w", scanErr), true
	REDACTED
		s.recordOpenAIProxyStreamDisconnect(account, scanErr, upstreamRequestID)
		sendErrorEvent("stream_read_error")
		return resultWithUsage(), fmt.Errorf("stream read error: %w", scanErr), true
REDACTED
	processSSELine := func(line string, queueDrained bool) {
		if streamEarlyErr != nil {
			return
	REDACTED
		if eventType, ok := extractOpenAISSEEventLine(line); ok {
			pendingSSEEventType = eventType
			eventType = strings.TrimSpace(eventType)
			suppressCurrentEvent = codexFailureTerminal && (eventType == "error" || (sawBareError && !sawResponseFailed && eventType != "response.failed"))
	REDACTED
		// Extract data from SSE line (supports both "data: " and "data:" formats)
		if data, ok := extractOpenAISSEDataLine(line); ok {
			dataBytes := []byte(data)
			eventType := effectiveOpenAISSEEventType(dataBytes, pendingSSEEventType)
			if codexFailureTerminal && sawBareError && !sawResponseFailed &&
				(eventType == "response.completed" || eventType == "response.done") {
				// A later successful terminal is authoritative over a pending bare
				// error. Keep its usage and terminal visible to the client.
				sawBareError = false
				sawFailedEvent = false
				terminalFailurePending = false
				suppressCurrentEvent = false
				bareErrorPayload = nil
				bareErrorAccountSideEffectsPending = false
				failedMessage = ""
		REDACTED
			if codexFailureTerminal && sawBareError && !sawResponseFailed && eventType != "response.failed" {
				suppressCurrentEvent = true
		REDACTED
			observer.ObserveOpenAI(dataBytes, eventType)
			// 初始上游 data 的 type 只解析一次：原始值保持终止事件的精确匹配，规范化值供后续分支复用。
			if openAIStreamEventIsTerminalWithType(data, eventType) {
				sawTerminalEvent = true
				terminalEventType = eventType
				if strings.TrimSpace(data) == "[DONE]" {
					terminalEventType = "[DONE]"
			REDACTED
		REDACTED
			if responseID == "" {
				responseID = extractOpenAIResponseIDFromJSONBytes(dataBytes)
		REDACTED
			forceFlushFailedEvent := false
			if !capacityFailoverSuppressedLogged && account != nil && account.Platform == PlatformOpenAI &&
				(eventType == "error" || eventType == "response.failed") &&
				openAIStreamClientOutputStarted(c, clientOutputStarted) &&
				isOpenAIUpstreamCapacityShedEvent(dataBytes) {
				logOpenAICapacityFailoverSuppressed(ctx, account, "native_sse", upstreamRequestID, eventType)
				capacityFailoverSuppressedLogged = true
		REDACTED
			cyberHit := false
			if eventType == "response.failed" || eventType == "error" {
				if codexFailureTerminal && eventType == "error" {
					sawBareError = true
					bareErrorPayload = append(bareErrorPayload[:0], dataBytes...)
					suppressCurrentEvent = true
			REDACTED else if codexFailureTerminal && eventType == "response.failed" {
					sawResponseFailed = true
			REDACTED
				failedMessage = extractOpenAISSEErrorMessage(dataBytes)
				if failedMessage == "" {
					failedMessage = "Upstream response failed"
			REDACTED
				// response.failed 自带上游已消耗的 usage（input token 通常已扣）；必须先解析
				// 再打 cyber 标记，否则 mark 记到的是解析前的 0，导致流式 cyber 按 0 token 计费
				// 而漏记真实用量。对齐 WS V2 / Chat 流式路径（均先解析 usage 再 Mark）。
				s.parseSSEUsageBytesWithType(dataBytes, eventType, usage)
				if hit, code, msg := detectOpenAICyberPolicy(dataBytes); hit {
					cyberHit = true
					MarkOpsCyberPolicy(c, CyberPolicyMark{
						Code:           code,
						Message:        msg,
						Body:           truncateString(string(dataBytes), 4096),
						UpstreamStatus: http.StatusOK,
						UpstreamInTok:  usage.InputTokens,
						UpstreamOutTok: usage.OutputTokens,
				REDACTED)
			REDACTED
				outputStarted := openAIStreamClientOutputStarted(c, clientOutputStarted)
				if !outputStarted && !cyberHit {
					if compactErr := newOpenAICompactFallbackSignal(c, dataBytes, failedMessage); compactErr != nil {
						sawFailedEvent = true
						streamEarlyErr = compactErr
						return
				REDACTED
			REDACTED
				if outputStarted && !cyberHit {
					if codexFailureTerminal && eventType == "error" {
						// OpenAI commonly follows a bare error with response.failed.
						// Defer account health updates so the pair is applied once.
						bareErrorAccountSideEffectsPending = true
				REDACTED else {
						s.handleOpenAIStreamTerminalAccountSideEffects(c, account, dataBytes, failedMessage, resp.Header)
						bareErrorAccountSideEffectsPending = false
				REDACTED
			REDACTED
				if !outputStarted {
					shouldFailover := false
					if !cyberHit {
						if eventType == "error" {
							shouldFailover = openAIStreamErrorEventShouldFailover(dataBytes, failedMessage)
					REDACTED else {
							shouldFailover = openAIStreamFailedEventShouldFailover(dataBytes, failedMessage)
					REDACTED
				REDACTED
					if shouldFailover {
						sawFailedEvent = true
						streamEarlyErr = s.newOpenAIStreamFailoverError(c, account, false, upstreamRequestID, dataBytes, failedMessage, resp.Header)
						return
				REDACTED
					if !cyberHit && !sawBareError {
						if status, errType, errMsg, matched := applyOpenAIStreamFailedErrorPassthroughRule(c, account.Platform, dataBytes, failedMessage); matched {
							sawFailedEvent = true
							// 命中透传规则也要记录 ops 上游错误事件（对齐 CC/Messages 与
							// antigravity 先例），否则透传命中的 failed 在监控中不可见。
							s.recordOpenAIStreamUpstreamError(c, account, false, upstreamRequestID, "http_error", dataBytes, failedMessage)
							MarkResponseCommitted(c)
							c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
							c.JSON(status, gin.H{
								"error": gin.H{
									"type":    errType,
									"message": errMsg,
							REDACTED,
						REDACTED)
							streamEarlyErr = fmt.Errorf("upstream response failed: passthrough rule matched message=%s", errMsg)
							return
					REDACTED
				REDACTED
			REDACTED
				forceFlushFailedEvent = true
				sawFailedEvent = true
				terminalFailurePending = !codexFailureTerminal || eventType == "response.failed"
		REDACTED
			if normalizedData, normalized := normalizeCompletedImageGenerationStatus(dataBytes); normalized {
				dataBytes = normalizedData
				data = string(normalizedData)
				line = "data: " + data
		REDACTED
			imageCounter.AddSSEData(dataBytes)
			searchCounter += countGrokNativeSearchCallsInSSEDataDedup(dataBytes, streamSearchSeen)

			// Correct Codex tool calls if needed (apply_patch -> edit, etc.)
			if correctedData, corrected := s.toolCorrector.CorrectToolCallsInSSEBytes(dataBytes); corrected {
				dataBytes = correctedData
				data = string(correctedData)
				line = "data: " + data
				eventType = effectiveOpenAISSEEventType(dataBytes, eventType)
		REDACTED
			if imageOutput, ok := extractImageGenerationOutputFromSSEData(dataBytes, streamSeenImages); ok {
				streamImageOutputs = append(streamImageOutputs, imageOutput)
		REDACTED
			if responsesStreamEventMayContributeToOutput(eventType) {
				var streamEvent apicompat.ResponsesStreamEvent
				if err := json.Unmarshal(dataBytes, &streamEvent); err == nil {
					streamOutputAccumulator.ProcessEvent(&streamEvent)
			REDACTED
		REDACTED
			if normalizedData, normalized := normalizeResponsesStreamingTerminalOutput(dataBytes, streamOutputAccumulator, streamImageOutputs); normalized {
				dataBytes = normalizedData
				data = string(normalizedData)
				line = "data: " + data
				eventType = effectiveOpenAISSEEventType(dataBytes, eventType)
		REDACTED
			restoredData, restoreErr := restoreGrokResponsesClientToolPayload(c, dataBytes)
			if restoreErr != nil {
				streamEarlyErr = fmt.Errorf("restore Grok Responses client tool response: %w", restoreErr)
				return
		REDACTED
			restoredData, restoreErr = restoreOpenAIResponsesNamespacePayload(c, restoredData)
			if restoreErr != nil {
				streamEarlyErr = fmt.Errorf("restore OpenAI namespace response: %w", restoreErr)
				return
		REDACTED
			restoredData = restoreCodexToolNamesFromSSEContext(c, restoredData, eventType)
			if !bytes.Equal(restoredData, dataBytes) {
				dataBytes = restoredData
				data = string(restoredData)
				line = "data: " + data
				eventType = effectiveOpenAISSEEventType(dataBytes, eventType)
		REDACTED
			if sanitizedData, sanitized := sanitizeOpenAIResponseFailedEventForClient(
				dataBytes,
				eventType,
				openAIStreamClientOutputStarted(c, clientOutputStarted),
			); sanitized {
				dataBytes = sanitizedData
				data = string(sanitizedData)
				line = "data: " + data
		REDACTED
			// Replace model in response if needed.
			// Fast path: most events do not contain model field values.
			if needModelReplace && mappedModel != "" && strings.Contains(line, mappedModel) {
				line = s.replaceModelInSSELine(line, mappedModel, originalModel)
		REDACTED
			startsClientOutput := forceFlushFailedEvent || openAIStreamDataStartsClientOutput(data, eventType)
			startsVisibleOutput := openAIStreamDataStartsVisibleOutput(data, eventType)
			if stageFirstOutput {
				eventStartsClientOutput = eventStartsClientOutput || startsClientOutput
				eventStartsVisibleOutput = eventStartsVisibleOutput || startsVisibleOutput
				if startsClientOutput {
					firstOutputScanGuard.Store(false)
			REDACTED
		REDACTED
			if startsClientOutput && !openAIStreamEventTypeIsTerminal(eventType) {
				responsesSemanticOutputSeen = true
		REDACTED
			// OpenAI Responses streams that terminate with an empty
			// response.completed (no output, no usage, no error, nothing sent
			// to the client) are silent upstream refusals: fail over instead of
			// recording a successful 0/0 usage turn (issue #5009).
			if account != nil && account.Platform == PlatformOpenAI &&
				(eventType == "response.completed" || eventType == "response.done") &&
				!sawFailedEvent && !responsesSemanticOutputSeen && !clientOutputStarted &&
				openAIResponsesCompletedEventIsEmpty(dataBytes, usage) {
				sawTerminalEvent = true
				streamEarlyErr = newOpenAIResponsesEmptyCompletedFailoverError(c, account, upstreamRequestID)
				return
		REDACTED

			// 写入客户端（客户端断开后继续 drain 上游）
			if !clientDisconnected && !failureDelivered && !suppressCurrentEvent {
				shouldFlush := queueDrained && (clientOutputStarted || startsClientOutput)
				if firstTokenMs == nil && startsVisibleOutput {
					// 保证首个 token 事件尽快出站，避免影响 TTFT。
					shouldFlush = true
			REDACTED
				eventShouldFlush = eventShouldFlush || shouldFlush
				if _, err := writePendingString(line); err != nil {
					handlePendingWriteError(err)
			REDACTED else if _, err := writePendingString("\n"); err != nil {
					handlePendingWriteError(err)
			REDACTED else {
					eventInProgress = true
			REDACTED
		REDACTED

			// Record first token time
			if !guardFirstOutput && firstTokenMs == nil && startsVisibleOutput {
				ms := int(time.Since(startTime).Milliseconds())
				firstTokenMs = &ms
				stopFirstOutputTimer()
		REDACTED
			s.parseSSEUsageBytesWithType(dataBytes, eventType, usage)
			return
	REDACTED

		// A blank line dispatches a guarded event from the attempt-local stage.
		if stageFirstOutput && line == "" {
			pendingSSEEventType = ""
			if suppressCurrentEvent {
				suppressCurrentEvent = false
				terminalFailurePending = false
				eventInProgress = false
				eventStartsClientOutput = false
				eventStartsVisibleOutput = false
				eventShouldFlush = false
				return
		REDACTED
			if failureDelivered {
				terminalFailurePending = false
				eventInProgress = false
				eventStartsClientOutput = false
				eventStartsVisibleOutput = false
				eventShouldFlush = false
				return
		REDACTED
			if !clientDisconnected {
				if _, err := writePendingString("\n"); err != nil {
					handlePendingWriteError(err)
			REDACTED
		REDACTED
			if streamEarlyErr == nil {
				completeGuardedEvent(queueDrained)
		REDACTED
			if terminalFailurePending && streamEarlyErr == nil {
				terminalFailurePending = false
				failureDelivered = true
		REDACTED
			return
	REDACTED
		// Non-guarded streams retain upstream's event-boundary flushing: a keepalive
		// or queue-drain flush must never split an open SSE event.
		shouldFlush := false
		if line == "" {
			pendingSSEEventType = ""
			if suppressCurrentEvent {
				suppressCurrentEvent = false
				terminalFailurePending = false
				eventInProgress = false
				eventShouldFlush = false
				return
		REDACTED
			shouldFlush = eventShouldFlush || (queueDrained && clientOutputStarted)
			eventShouldFlush = false
			if failureDelivered {
				terminalFailurePending = false
		REDACTED
	REDACTED
		if !clientDisconnected && !failureDelivered && !suppressCurrentEvent {
			if _, err := writePendingString(line); err != nil {
				handlePendingWriteError(err)
		REDACTED else if _, err := writePendingString("\n"); err != nil {
				handlePendingWriteError(err)
		REDACTED else {
				eventInProgress = line != ""
				if shouldFlush {
					if err := flushBuffered(); err != nil {
						clientDisconnected = true
						logger.LegacyPrintf("service.openai_gateway", "Client disconnected during streaming flush, continuing to drain upstream for billing")
				REDACTED else {
						clientOutputStarted = true
						lastDownstreamWriteAt = time.Now()
				REDACTED
			REDACTED
		REDACTED
			if line == "" && terminalFailurePending && streamEarlyErr == nil {
				terminalFailurePending = false
				failureDelivered = true
		REDACTED
	REDACTED
REDACTED

	// 无超时/无 keepalive 的常见路径走同步扫描，减少 goroutine 与 channel 开销。
	if streamInterval <= 0 && keepaliveInterval <= 0 && firstOutputTimeout <= 0 {
		defer putSSEScannerBuf64K(scanBuf)
		for documentScanner.Scan() {
			processSSELine(documentScanner.Text(), true)
			if streamEarlyErr != nil {
				return resultWithUsage(), streamEarlyErr
		REDACTED
	REDACTED
		if result, err, done := handleScanErr(documentScanner.Err()); done {
			return result, err
	REDACTED
		return finalizeStream()
REDACTED

	type scanEvent struct {
		line      string
		err       error
		processed chan struct{REDACTED
REDACTED
	// 独立 goroutine 读取上游，避免读取阻塞影响 keepalive/超时处理
	// Guard mode permits one queued token plus the token being processed. With
	// the guarded scanner cap this bounds scanner/channel retention near 16 MiB;
	// the timeout-disabled path preserves the legacy depth of 16.
	events := make(chan scanEvent, openAIFirstOutputEventQueueSize(guardFirstOutput))
	done := make(chan struct{REDACTED)
	sendEvent := func(ev scanEvent) bool {
		if firstOutputScanGuard.Load() {
			ev.processed = make(chan struct{REDACTED)
	REDACTED
		select {
		case events <- ev:
		case <-done:
			return false
	REDACTED
		if ev.processed == nil {
			return true
	REDACTED
		select {
		case <-ev.processed:
			return true
		case <-done:
			return false
	REDACTED
REDACTED
	markEventProcessed := func(ev scanEvent) {
		if ev.processed != nil {
			close(ev.processed)
	REDACTED
REDACTED
	var lastReadAt int64
	atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
	go func(scanBuf *sseScannerBuf64K) {
		defer putSSEScannerBuf64K(scanBuf)
		defer close(events)
		for documentScanner.Scan() {
			atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
			if !sendEvent(scanEvent{line: documentScanner.Text()REDACTED) {
				return
		REDACTED
	REDACTED
		if err := documentScanner.Err(); err != nil {
			_ = sendEvent(scanEvent{err: errREDACTED)
	REDACTED
REDACTED(scanBuf)
	defer close(done)

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				if stageFirstOutput && eventInProgress {
					// EOF dispatches the final SSE event even without a trailing blank
					// line. Do not synthesize extra bytes on the downstream wire.
					completeGuardedEvent(true)
			REDACTED
				return finalizeStream()
		REDACTED
			if result, err, done := handleScanErr(ev.err); done {
				markEventProcessed(ev)
				return result, err
		REDACTED
			processSSELine(ev.line, len(events) == 0)
			markEventProcessed(ev)
			if streamEarlyErr != nil {
				return resultWithUsage(), streamEarlyErr
		REDACTED

		case <-intervalCh:
			if failureDelivered {
				return resultWithUsage(), fmt.Errorf("upstream response failed: %s", failedMessage)
		REDACTED
			lastRead := time.Unix(0, atomic.LoadInt64(&lastReadAt))
			if time.Since(lastRead) < streamInterval {
				continue
		REDACTED
			if codexFailureTerminal && sawBareError && !sawResponseFailed {
				_ = resp.Body.Close()
				return finalizeStream()
		REDACTED
			if clientDisconnected {
				return resultWithUsage(), fmt.Errorf("stream usage incomplete after timeout")
		REDACTED
			logger.LegacyPrintf("service.openai_gateway", "Stream data interval timeout: account=%d model=%s interval=%s", account.ID, originalModel, streamInterval)
			// 处理流超时，可能标记账户为临时不可调度或错误状态
			if s.rateLimitService != nil {
				s.rateLimitService.HandleStreamTimeout(ctx, account, originalModel)
		REDACTED
			// Grok: short cool + account failover when no client-visible bytes
			// were committed yet (pre-commit). After output started we keep the
			// legacy stream_timeout path so partial SSE is not dual-written.
			if account != nil && account.Platform == PlatformGrok {
				s.tempUnscheduleGrok(ctx, account, grokStreamIdleCooldown, "grok stream idle timeout")
				if !openAIStreamClientOutputStarted(c, clientOutputStarted) && !eventShouldFlush {
					_ = resp.Body.Close()
					return resultWithUsage(), grokStreamIdleFailoverError(account, streamInterval)
			REDACTED
		REDACTED
			sendErrorEvent("stream_timeout")
			return resultWithUsage(), fmt.Errorf("stream data interval timeout")

		case <-firstOutputCh:
			if firstOutputProgressObserved {
				stopFirstOutputTimer()
				continue
		REDACTED
			if codexFailureTerminal && sawBareError && !sawResponseFailed && len(events) == 0 {
				_ = resp.Body.Close()
				return finalizeStream()
		REDACTED
			_ = resp.Body.Close()
			for ev := range events {
				markEventProcessed(ev)
		REDACTED
			return resultWithUsage(), s.newOpenAIFirstOutputTimeoutError(
				ctx, c, account, startTime, originalModel, reasoningEffort,
				firstOutputTimeout, "semantic_output", resp.Header,
			)

		case <-keepaliveCh:
			if clientDisconnected || failureDelivered {
				continue
		REDACTED
			if eventInProgress {
				continue
		REDACTED
			if time.Since(lastDownstreamWriteAt) < keepaliveInterval {
				continue
		REDACTED
			if stageFirstOutput {
				// Bypass attempt-local buffered frames. The stable SSE headers may be
				// committed here, but account headers remain private until semantic output.
				n, err := w.Write([]byte(":\n\n"))
				recordOpenAIStreamKeepaliveBytes(c, n)
				if err != nil {
					clientDisconnected = true
					logger.LegacyPrintf("service.openai_gateway", "Client disconnected during streaming, continuing to drain upstream for billing")
					continue
			REDACTED
				flusher.Flush()
				lastDownstreamWriteAt = time.Now()
				continue
		REDACTED
			if _, err := writePendingString(":\n\n"); err != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.openai_gateway", "Client disconnected during streaming, continuing to drain upstream for billing")
				continue
		REDACTED
			if err := flushBuffered(); err != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.openai_gateway", "Client disconnected during keepalive flush, continuing to drain upstream for billing")
		REDACTED else {
				lastDownstreamWriteAt = time.Now()
		REDACTED
	REDACTED
REDACTED

REDACTED

// extractOpenAISSEDataLine 低开销提取 SSE `data:` 行内容。
// 兼容 `data: xxx` 与 `data:xxx` 两种格式。
func extractOpenAISSEDataLine(line string) (string, bool) {
	if !strings.HasPrefix(line, "data:") {
		return "", false
REDACTED
	start := len("data:")
	for start < len(line) {
		if line[start] != ' ' && line[start] != '	' {
			break
	REDACTED
		start++
REDACTED
	return line[start:], true
REDACTED

func extractOpenAISSEEventLine(line string) (string, bool) {
	if !strings.HasPrefix(line, "event:") {
		return "", false
REDACTED
	start := len("event:")
	for start < len(line) {
		if line[start] != ' ' && line[start] != '	' {
			break
	REDACTED
		start++
REDACTED
	return strings.TrimSpace(line[start:]), true
REDACTED

type openAICompatSSEFrame struct {
	EventType string
	Data      string
REDACTED

type openAICompatSSEFrameParser struct {
	eventType string
	dataLines []string
REDACTED

func (p *openAICompatSSEFrameParser) AddLine(line string) (openAICompatSSEFrame, bool) {
	if line == "" {
		return p.dispatch()
REDACTED
	if strings.HasPrefix(line, ":") {
		return openAICompatSSEFrame{REDACTED, false
REDACTED
	if eventType, ok := extractOpenAISSEEventLine(line); ok {
		p.eventType = eventType
		return openAICompatSSEFrame{REDACTED, false
REDACTED
	if data, ok := extractOpenAISSEDataLine(line); ok {
		p.dataLines = append(p.dataLines, data)
REDACTED
	return openAICompatSSEFrame{REDACTED, false
REDACTED

func (p *openAICompatSSEFrameParser) Finish() (openAICompatSSEFrame, bool) {
	return p.dispatch()
REDACTED

func (p *openAICompatSSEFrameParser) dispatch() (openAICompatSSEFrame, bool) {
	frame := openAICompatSSEFrame{
		EventType: p.eventType,
		Data:      strings.Join(p.dataLines, "\n"),
REDACTED
	p.eventType = ""
	p.dataLines = nil
	return frame, frame.Data != ""
REDACTED

func openAICompatPayloadWithEventType(payload, eventType string) string {
	eventType = strings.TrimSpace(eventType)
	if eventType == "" || strings.TrimSpace(payload) == "" || strings.TrimSpace(payload) == "[DONE]" {
		return payload
REDACTED
	if strings.TrimSpace(gjson.Get(payload, "type").String()) != "" {
		return payload
REDACTED
	patched, err := sjson.Set(payload, "type", eventType)
	if err != nil {
		return payload
REDACTED
	return patched
REDACTED

func effectiveOpenAISSEEventType(payload []byte, eventType string) string {
	if payloadType := strings.TrimSpace(gjson.GetBytes(payload, "type").String()); payloadType != "" {
		return payloadType
REDACTED
	return strings.TrimSpace(eventType)
REDACTED

func (s *OpenAIGatewayService) replaceModelInSSELine(line, fromModel, toModel string) string {
	data, ok := extractOpenAISSEDataLine(line)
	if !ok {
		return line
REDACTED
	if data == "" || data == "[DONE]" {
		return line
REDACTED

	// 使用 gjson 精确检查 model 字段，避免全量 JSON 反序列化
	if m := gjson.Get(data, "model"); m.Exists() && m.Str == fromModel {
		newData, err := sjson.Set(data, "model", toModel)
		if err != nil {
			return line
	REDACTED
		return "data: " + newData
REDACTED

	// 检查嵌套的 response.model 字段
	if m := gjson.Get(data, "response.model"); m.Exists() && m.Str == fromModel {
		newData, err := sjson.Set(data, "response.model", toModel)
		if err != nil {
			return line
	REDACTED
		return "data: " + newData
REDACTED

	return line
REDACTED

// correctToolCallsInResponseBody 修正响应体中的工具调用
func (s *OpenAIGatewayService) correctToolCallsInResponseBody(body []byte) []byte {
	if len(body) == 0 {
		return body
REDACTED

	updated := body
	if s != nil && s.toolCorrector != nil {
		if corrected, changed := s.toolCorrector.CorrectToolCallsInSSEBytes(updated); changed {
			updated = corrected
	REDACTED
REDACTED
	if normalized, changed := normalizeOpenAIResponsesFunctionCallArguments(updated); changed {
		updated = normalized
REDACTED
	return updated
REDACTED

func normalizeOpenAIResponsesFunctionCallArguments(data []byte) ([]byte, bool) {
	if len(bytes.TrimSpace(data)) == 0 || !bytes.Contains(data, []byte(`"arguments"`)) {
		return data, false
REDACTED
	if !gjson.ValidBytes(data) {
		return data, false
REDACTED

	updated := data
	changed := false
	setDedupedArgument := func(path string) {
		arg := gjson.GetBytes(updated, path)
		if !arg.Exists() || arg.Type != gjson.String {
			return
	REDACTED
		deduped, ok := dedupeRepeatedJSONArgumentString(arg.Str)
		if !ok {
			return
	REDACTED
		next, err := sjson.SetBytes(updated, path, deduped)
		if err != nil {
			return
	REDACTED
		updated = next
		changed = true
REDACTED

	eventType := strings.TrimSpace(gjson.GetBytes(updated, "type").String())
	if eventType == "response.function_call_arguments.done" {
		setDedupedArgument("arguments")
REDACTED
	if itemType := strings.TrimSpace(gjson.GetBytes(updated, "item.type").String()); isResponsesFunctionCallItemType(itemType) {
		setDedupedArgument("item.arguments")
REDACTED
	dedupeResponsesFunctionCallOutputArguments(updated, "response.output", setDedupedArgument)
	dedupeResponsesFunctionCallOutputArguments(updated, "output", setDedupedArgument)

	return updated, changed
REDACTED

func dedupeResponsesFunctionCallOutputArguments(data []byte, outputPath string, setDedupedArgument func(string)) {
	output := gjson.GetBytes(data, outputPath)
	if !output.Exists() || !output.IsArray() {
		return
REDACTED
	for i, item := range output.Array() {
		if !isResponsesFunctionCallItemType(strings.TrimSpace(item.Get("type").String())) {
			continue
	REDACTED
		setDedupedArgument(outputPath + "." + strconv.Itoa(i) + ".arguments")
REDACTED
REDACTED

func isResponsesFunctionCallItemType(itemType string) bool {
	return itemType == "function_call" || itemType == "custom_tool_call"
REDACTED

func dedupeRepeatedJSONArgumentString(arguments string) (string, bool) {
	if len(arguments) == 0 || len(arguments)%2 != 0 {
		return "", false
REDACTED
	halfLen := len(arguments) / 2
	first := arguments[:halfLen]
	if first != arguments[halfLen:] {
		return "", false
REDACTED
	trimmed := strings.TrimSpace(first)
	if trimmed == "" || (!strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[")) {
		return "", false
REDACTED
	if !json.Valid([]byte(first)) {
		return "", false
REDACTED
	return first, true
REDACTED

func (s *OpenAIGatewayService) parseSSEUsage(data string, usage *OpenAIUsage) {
	s.parseSSEUsageBytes([]byte(data), usage)
REDACTED

func (s *OpenAIGatewayService) parseSSEUsageBytes(data []byte, usage *OpenAIUsage) {
	s.parseSSEUsageBytesWithType(data, "", usage)
REDACTED

func (s *OpenAIGatewayService) parseSSEUsageBytesWithType(data []byte, eventType string, usage *OpenAIUsage) {
	if usage == nil || len(data) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("[DONE]")) {
		return
REDACTED
	// Usage is absent from nearly every delta event. Avoid full JSON validation
	// and four gjson path scans on that hot path while retaining progressive
	// usage from compatible upstreams on any event type.
	if !bytes.Contains(data, []byte(`"usage"`)) {
		return
REDACTED
	parsedUsage, ok := extractOpenAIUsageFromJSONBytes(data)
	if !ok {
		return
REDACTED
	if openAIStreamEventTypeIsTerminal(effectiveOpenAISSEEventType(data, eventType)) {
		if !openAIUsageHasTokens(&parsedUsage) && openAIUsageHasTokens(usage) {
			return
	REDACTED
		*usage = parsedUsage
		return
REDACTED
	mergeOpenAIUsageNonZero(usage, parsedUsage)
REDACTED

// Compatible Responses upstreams may report usage before the terminal event.
// Retain those non-zero fields as a fallback. A terminal usage with any billed
// tokens is authoritative as a whole; an all-zero terminal does not erase a
// non-zero progressive observation from the same turn.
func mergeOpenAIUsageNonZero(dst *OpenAIUsage, src OpenAIUsage) {
	if dst == nil {
		return
REDACTED
	if src.InputTokens > 0 {
		dst.InputTokens = src.InputTokens
REDACTED
	if src.ImageInputTokens > 0 {
		dst.ImageInputTokens = src.ImageInputTokens
REDACTED
	if src.OutputTokens > 0 {
		dst.OutputTokens = src.OutputTokens
REDACTED
	if src.CacheCreationInputTokens > 0 {
		dst.CacheCreationInputTokens = src.CacheCreationInputTokens
REDACTED
	if src.CacheReadInputTokens > 0 {
		dst.CacheReadInputTokens = src.CacheReadInputTokens
REDACTED
	if src.ImageOutputTokens > 0 {
		dst.ImageOutputTokens = src.ImageOutputTokens
REDACTED
REDACTED

func openAIUsageHasTokens(usage *OpenAIUsage) bool {
	return usage != nil && (usage.InputTokens > 0 || usage.ImageInputTokens > 0 ||
		usage.OutputTokens > 0 || usage.CacheCreationInputTokens > 0 ||
		usage.CacheReadInputTokens > 0 || usage.ImageOutputTokens > 0)
REDACTED

const openAIMissingUsageLogInterval = time.Minute

type openAIMissingUsageLogSampler struct {
	total      atomic.Uint64
	suppressed atomic.Uint64
	lastLog    atomic.Int64
REDACTED

var openAIMissingUsageSampler openAIMissingUsageLogSampler

func (s *openAIMissingUsageLogSampler) sample(now time.Time) (logNow bool, total uint64, suppressed uint64) {
	total = s.total.Add(1)
	nowNanos := now.UnixNano()
	for {
		last := s.lastLog.Load()
		if last != 0 && nowNanos-last < int64(openAIMissingUsageLogInterval) {
			s.suppressed.Add(1)
			return false, total, 0
	REDACTED
		if s.lastLog.CompareAndSwap(last, nowNanos) {
			return true, total, s.suppressed.Swap(0)
	REDACTED
REDACTED
REDACTED

func logOpenAISuccessMissingUsage(ctx context.Context, c *gin.Context, account *Account, resp *http.Response, usage *OpenAIUsage, terminalEvent string, clientDisconnected bool) {
	if resp == nil || resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices || openAIUsageHasTokens(usage) {
		return
REDACTED
	terminalEvent = strings.TrimSpace(terminalEvent)
	if terminalEvent != "response.completed" && terminalEvent != "response.done" && terminalEvent != "[DONE]" && terminalEvent != "json" {
		return
REDACTED
	logNow, total, suppressed := openAIMissingUsageSampler.sample(time.Now())
	if !logNow {
		return
REDACTED
	accountID := int64(0)
	accountType := ""
	if account != nil {
		accountID = account.ID
		accountType = string(account.Type)
REDACTED
	inboundEndpoint := ""
	if c != nil && c.Request != nil && c.Request.URL != nil {
		inboundEndpoint = c.Request.URL.Path
REDACTED
	logger.FromContext(ctx).With(
		zap.String("component", "service.openai_gateway"),
		zap.Int64("account_id", accountID),
		zap.String("account_type", accountType),
		zap.String("inbound_endpoint", inboundEndpoint),
		zap.String("upstream_request_id", strings.TrimSpace(resp.Header.Get("x-request-id"))),
		zap.Int("upstream_status_code", resp.StatusCode),
		zap.String("terminal_event", terminalEvent),
		zap.Bool("client_disconnected", clientDisconnected),
		zap.Uint64("missing_usage_total", total),
		zap.Uint64("suppressed_since_last", suppressed),
	).Warn("openai_usage.success_missing_usage")
REDACTED

func extractOpenAIUsageFromJSONBytes(body []byte) (OpenAIUsage, bool) {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return OpenAIUsage{REDACTED, false
REDACTED
	// 部分 OpenAI 兼容上游（例如 Cline API）会将标准响应包在 data 字段中：
	// {"data":{"choices": [...], "usage": {...REDACTEDREDACTED, "success":trueREDACTED。
	// 按优先级先保留原有路径，再尝试兼容层 data 包装，
	// 避免同步请求能正常返回但用量被静默记录为 0。
	candidates := []struct {
		usagePath      string
		imageUsagePath string
REDACTED{
		{usagePath: "usage", imageUsagePath: "tool_usage.image_gen"REDACTED,
		{usagePath: "response.usage", imageUsagePath: "response.tool_usage.image_gen"REDACTED,
		{usagePath: "data.usage", imageUsagePath: "data.tool_usage.image_gen"REDACTED,
		{usagePath: "data.response.usage", imageUsagePath: "data.response.tool_usage.image_gen"REDACTED,
REDACTED
	for _, candidate := range candidates {
		if usage, ok := openAIUsageFromGJSON(gjson.GetBytes(body, candidate.usagePath)); ok {
			mergeHostedImageGenToolUsage(gjson.GetBytes(body, candidate.imageUsagePath), &usage)
			return usage, true
	REDACTED
REDACTED
	return OpenAIUsage{REDACTED, false
REDACTED

// openAIResponsesCompletedEventIsEmpty reports whether a response.completed /
// response.done SSE payload carries no usage, no error and no output items.
// The accumulated usage is consulted too, because OpenAI may deliver usage on
// an earlier event. An empty terminal event after a stream with no semantic
// output is treated as a silent upstream refusal (issue #5009).
func openAIResponsesCompletedEventIsEmpty(data []byte, usage *OpenAIUsage) bool {
	if len(data) == 0 || !gjson.ValidBytes(data) {
		return false
REDACTED
	if usage != nil && (usage.InputTokens > 0 || usage.OutputTokens > 0 ||
		usage.ImageInputTokens > 0 || usage.ImageOutputTokens > 0 ||
		usage.CacheCreationInputTokens > 0 || usage.CacheReadInputTokens > 0) {
		return false
REDACTED
	if gjson.GetBytes(data, "usage").Exists() || gjson.GetBytes(data, "response.usage").Exists() {
		return false
REDACTED
	if gjson.GetBytes(data, "error").Exists() || gjson.GetBytes(data, "response.error").Exists() {
		return false
REDACTED
	if output := gjson.GetBytes(data, "response.output"); output.Exists() && output.IsArray() && len(output.Array()) > 0 {
		return false
REDACTED
	return true
REDACTED

func mergeHostedImageGenToolUsage(imageGen gjson.Result, usage *OpenAIUsage) {
	if !imageGen.Exists() || !imageGen.IsObject() {
		return
REDACTED
	if usage.ImageOutputTokens == 0 {
		if v := imageGen.Get("output_tokens_details.image_tokens").Int(); v > 0 {
			usage.ImageOutputTokens = int(v)
	REDACTED
REDACTED
	if usage.ImageInputTokens == 0 {
		if v := imageGen.Get("input_tokens_details.image_tokens").Int(); v > 0 {
			usage.ImageInputTokens = int(v)
	REDACTED
REDACTED
REDACTED

func extractOpenAIResponseIDFromJSONBytes(body []byte) string {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return ""
REDACTED
	if id := strings.TrimSpace(gjson.GetBytes(body, "id").String()); id != "" {
		return id
REDACTED
	return strings.TrimSpace(gjson.GetBytes(body, "response.id").String())
REDACTED

const openAIHTTPResponseOwnerContextKey = "openai_http_response_owner"

type openAIHTTPResponseOwner struct {
	userID   int64
	apiKeyID int64
REDACTED

// SetOpenAIHTTPResponseOwner marks the authenticated downstream owner whose
// successful Responses IDs may be used for later HTTP continuations.
func SetOpenAIHTTPResponseOwner(c *gin.Context, userID, apiKeyID int64) {
	if c == nil || userID <= 0 || apiKeyID <= 0 {
		return
REDACTED
	c.Set(openAIHTTPResponseOwnerContextKey, openAIHTTPResponseOwner{userID: userID, apiKeyID: apiKeyIDREDACTED)
REDACTED

// ValidateOpenAIHTTPResponseOwner authorizes a continuation by downstream
// tenant. API key identity is retained in the binding, while keys owned by the
// same user remain interoperable.
func (s *OpenAIGatewayService) ValidateOpenAIHTTPResponseOwner(
	ctx context.Context,
	groupID int64,
	responseID string,
	userID, apiKeyID int64,
) (bool, error) {
	if s == nil || strings.TrimSpace(responseID) == "" || userID <= 0 || apiKeyID <= 0 {
		return false, nil
REDACTED
	ownerUserID, ownerAPIKeyID, found, err := s.getOpenAIWSStateStore().GetHTTPResponseOwner(ctx, groupID, responseID)
	if err != nil || !found {
		return false, err
REDACTED
	return ownerUserID == userID || (ownerUserID <= 0 && ownerAPIKeyID == apiKeyID), nil
REDACTED

// BindOpenAIHTTPResponseOwner records an HTTP continuation owner independently
// from the upstream account selected for that response.
func (s *OpenAIGatewayService) BindOpenAIHTTPResponseOwner(
	ctx context.Context,
	groupID int64,
	responseID string,
	userID, apiKeyID int64,
) error {
	if s == nil {
		return nil
REDACTED
	return s.getOpenAIWSStateStore().BindHTTPResponseOwner(
		ctx, groupID, responseID, userID, apiKeyID, s.openAIWSResponseStickyTTL(),
	)
REDACTED

func (s *OpenAIGatewayService) bindHTTPResponseAccount(ctx context.Context, c *gin.Context, account *Account, responseID string) {
	if s == nil || account == nil || account.ID <= 0 {
		return
REDACTED
	responseID = strings.TrimSpace(responseID)
	if responseID == "" {
		return
REDACTED
	store := s.getOpenAIWSStateStore()
	if store == nil {
		return
REDACTED
	groupID := getOpenAIGroupIDFromContext(c)
	ttl := s.openAIWSResponseStickyTTL()
	logOpenAIWSBindResponseAccountWarn(groupID, account.ID, responseID, store.BindResponseAccount(ctx, groupID, responseID, account.ID, ttl))
	if rawOwner, ok := c.Get(openAIHTTPResponseOwnerContextKey); ok {
		if owner, ok := rawOwner.(openAIHTTPResponseOwner); ok && owner.userID > 0 && owner.apiKeyID > 0 {
			if err := s.BindOpenAIHTTPResponseOwner(ctx, groupID, responseID, owner.userID, owner.apiKeyID); err != nil {
				logger.L().Warn(
					"openai.http_bind_response_owner_failed",
					zap.Int64("group_id", groupID),
					zap.Int64("account_id", account.ID),
					zap.Int64("user_id", owner.userID),
					zap.Int64("api_key_id", owner.apiKeyID),
					zap.String("response_id", truncateOpenAIWSLogValue(responseID, openAIWSIDValueMaxLen)),
					zap.Error(err),
				)
		REDACTED
	REDACTED
REDACTED
REDACTED

func openAIUsageFromGJSON(value gjson.Result) (OpenAIUsage, bool) {
	if !value.Exists() || !value.IsObject() {
		return OpenAIUsage{REDACTED, false
REDACTED
	inputTokens := value.Get("input_tokens").Int()
	if inputTokens == 0 {
		inputTokens = value.Get("prompt_tokens").Int()
REDACTED
	outputTokens := value.Get("output_tokens").Int()
	if outputTokens == 0 {
		outputTokens = value.Get("completion_tokens").Int()
REDACTED
	// xAI reports visible output separately from reasoning_tokens; OpenAI
	// folds reasoning into completion/output. Use total_tokens to tell them apart.
	reasoningTokens := max(int(firstPositiveGJSONInt(
		value.Get("completion_tokens_details.reasoning_tokens"),
		value.Get("output_tokens_details.reasoning_tokens"),
	)), 0)
	if reasoningTokens > 0 {
		outputTokens = xai.IncludeIndependentReasoningTokens(
			inputTokens, outputTokens, value.Get("total_tokens").Int(), int64(reasoningTokens),
		)
REDACTED
	cacheReadTokens := openAICacheReadTokensFromUsage(value)
	cacheCreationTokens := openAICacheCreationTokensFromUsage(value)
	imageOutputTokens := value.Get("output_tokens_details.image_tokens").Int()
	if imageOutputTokens == 0 {
		imageOutputTokens = value.Get("completion_tokens_details.image_tokens").Int()
REDACTED
	// 图片输入 token（如 gpt-image-2 的 /v1/images/edits 带图请求），
	// 上游在 input_tokens_details.image_tokens 单独回传，用于图/文输入分价计费。
	// 普通文本请求该字段为 0，走原路径行为不变。
	imageInputTokens := firstPositiveGJSONInt(
		value.Get("input_tokens_details.image_tokens"),
		value.Get("prompt_tokens_details.image_tokens"),
	)
	return OpenAIUsage{
		InputTokens:              int(inputTokens),
		ImageInputTokens:         imageInputTokens,
		OutputTokens:             int(outputTokens),
		CacheCreationInputTokens: cacheCreationTokens,
		CacheReadInputTokens:     cacheReadTokens,
		ImageOutputTokens:        int(imageOutputTokens),
REDACTED, true
REDACTED

func openAICacheReadTokensFromUsage(value gjson.Result) int {
	for _, nested := range []gjson.Result{
		value.Get("input_tokens_details.cached_tokens"),
		value.Get("prompt_tokens_details.cached_tokens"),
REDACTED {
		if nested.Exists() {
			return max(int(nested.Int()), 0)
	REDACTED
REDACTED

	return firstPositiveGJSONInt(
		value.Get("cache_read_input_tokens"),
		value.Get("cache_read_tokens"),
		value.Get("cached_tokens"),
	)
REDACTED

func openAICacheCreationTokensFromUsage(value gjson.Result) int {
	for _, nested := range []gjson.Result{
		value.Get("input_tokens_details.cache_write_tokens"),
		value.Get("prompt_tokens_details.cache_write_tokens"),
		value.Get("input_tokens_details.cache_creation_tokens"),
		value.Get("prompt_tokens_details.cache_creation_tokens"),
REDACTED {
		if nested.Exists() {
			return max(int(nested.Int()), 0)
	REDACTED
REDACTED

	return firstPositiveGJSONInt(
		value.Get("cache_write_tokens"),
		value.Get("cache_creation_input_tokens"),
		value.Get("cache_write_input_tokens"),
		value.Get("cache_creation_tokens"),
	)
REDACTED

func (s *OpenAIGatewayService) handleNonStreamingResponse(ctx context.Context, resp *http.Response, c *gin.Context, account *Account, originalModel, mappedModel string) (*openaiNonStreamingResult, error) {
	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
REDACTED
	observer := upstreamResponseModelObserverFromContext(c)
	if observer == nil {
		observer = beginUpstreamResponseModelObservation(c)
REDACTED
	if bodyHasSSEFraming(body) {
		observeOpenAISSEBody(observer, string(body))
REDACTED else {
		observer.ObserveOpenAI(body, strings.TrimSpace(gjson.GetBytes(body, "type").String()))
REDACTED

	// Detect SSE responses for ALL account types via Content-Type header.
	// Some OpenAI-compatible upstreams (including other sub2api instances)
	// may return SSE even when stream=false was requested.
	if isEventStreamResponse(resp.Header) {
		return s.handleSSEToJSON(resp, c, account, body, originalModel, mappedModel)
REDACTED
	// bodyLooksLikeSSE is a line-level heuristic: real SSE framing requires
	// "data:"/"event:" field names at the very start of a physical line. A
	// plain bytes.Contains scan would also match ordinary JSON responses
	// whose string content merely echoes the literal text "data:" or
	// "event:" (e.g. compact tool output), causing those JSON bodies to be
	// misrouted into handleSSEToJSON and lose their usage accounting.
	bodyLooksLikeSSE := bodyHasSSEFraming(body)

	// For OAuth accounts, also fall back to a body-content heuristic because
	// the upstream may omit the Content-Type header while still sending SSE.
	// This heuristic is NOT applied to API-key accounts to avoid false
	// positives on JSON responses that coincidentally contain "data:" or
	// "event:" in their text content.
	if account.Type == AccountTypeOAuth && bodyLooksLikeSSE {
		return s.handleSSEToJSON(resp, c, account, body, originalModel, mappedModel)
REDACTED
	if account != nil && account.IsGrok() && isOpenAIResponsesCompactPath(c) {
		body, err = convertGrokResponseToOpenAICompact(body)
		if err != nil {
			return nil, fmt.Errorf("convert Grok compact response: %w", err)
	REDACTED
REDACTED

	usageValue, usageOK := extractOpenAIUsageFromJSONBytes(body)
	if !usageOK {
		if bodyLooksLikeSSE {
			return s.handleSSEToJSON(resp, c, account, body, originalModel, mappedModel)
	REDACTED
		return nil, fmt.Errorf("parse response: invalid json response")
REDACTED
	usage := &usageValue
	logOpenAISuccessMissingUsage(ctx, c, account, resp, usage, "json", false)

	// Replace model in response if needed
	if originalModel != mappedModel {
		body = s.replaceModelInResponseBody(body, mappedModel, originalModel)
REDACTED
	body, err = restoreGrokResponsesClientToolPayload(c, body)
	if err != nil {
		return nil, fmt.Errorf("restore Grok Responses client tool response: %w", err)
REDACTED
	body, err = restoreOpenAIResponsesNamespacePayload(c, body)
	if err != nil {
		return nil, fmt.Errorf("restore OpenAI namespace response: %w", err)
REDACTED
	body = restoreCodexToolNamesFromContext(c, body)
	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	// Codex 协议要求 /responses/compact JSON 响应携带 x-codex-turn-state
	// （codex-api/src/endpoint/compact.rs 从响应头捕获），显式回传。
	s.relayOpenAICodexTurnState(c, account, resp.Header)

	contentType := "application/json"
	if s.cfg != nil && !s.cfg.Security.ResponseHeaders.Enabled {
		if upstreamType := resp.Header.Get("Content-Type"); upstreamType != "" {
			contentType = upstreamType
	REDACTED
REDACTED

	if !writeOpenAICompactSSEBridge(c, resp.StatusCode, body) {
		c.Data(resp.StatusCode, contentType, body)
REDACTED

	return &openaiNonStreamingResult{
		OpenAIUsage:      usage,
		usage:            usage,
		responseID:       extractOpenAIResponseIDFromJSONBytes(body),
		imageCount:       countOpenAIResponseImageOutputsFromJSONBytes(body),
		imageOutputSizes: collectOpenAIResponseImageOutputSizesFromJSONBytes(body),
		searchCount:      countGrokNativeSearchCallsFromJSONBytes(body),
REDACTED, nil
REDACTED

func isEventStreamResponse(header http.Header) bool {
	contentType := strings.ToLower(header.Get("Content-Type"))
	return strings.Contains(contentType, "text/event-stream")
REDACTED

// bodyHasSSEFraming reports whether body contains genuine SSE framing by
// scanning for physical lines that begin with the "data:" or "event:"
// field names, per the SSE spec. Unlike a raw substring scan, this does not
// match when those strings only appear embedded inside JSON string values
// (e.g. "data: foo" quoted as part of an assistant text field), since such
// occurrences never start a physical line in a valid JSON encoding.
func bodyHasSSEFraming(body []byte) bool {
	for _, line := range bytes.Split(body, []byte("\n")) {
		line = bytes.TrimRight(line, "\r")
		if bytes.HasPrefix(line, []byte("data:")) || bytes.HasPrefix(line, []byte("event:")) {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func (s *OpenAIGatewayService) handleSSEToJSON(resp *http.Response, c *gin.Context, account *Account, body []byte, originalModel, mappedModel string) (*openaiNonStreamingResult, error) {
	bodyText := string(body)
	terminalType, terminalPayload, terminalOK := extractOpenAISSETerminalEvent(bodyText)
	if terminalOK && (terminalType == "response.failed" || terminalType == "error") {
		msg := extractOpenAISSEErrorMessage(terminalPayload)
		if msg == "" {
			msg = "Upstream compact response failed"
	REDACTED
		if compactErr := newOpenAICompactFallbackSignal(c, terminalPayload, msg); compactErr != nil {
			return nil, compactErr
	REDACTED
		return nil, s.writeOpenAINonStreamingProtocolError(resp, c, msg)
REDACTED
	finalResponse, ok := extractCodexFinalResponse(bodyText)

	usage := s.parseSSEUsageFromBody(bodyText)
	if ok {
		if parsedUsage, parsed := extractOpenAIUsageFromJSONBytes(finalResponse); parsed {
			*usage = parsedUsage
	REDACTED
		// When the terminal event has an empty output array, reconstruct
		// output from accumulated delta events so the client gets full content.
		// gjson Array() returns empty slice for null, missing, or empty arrays.
		if len(gjson.GetBytes(finalResponse, "output").Array()) == 0 {
			if outputJSON, reconstructed := reconstructResponseOutputFromSSE(bodyText); reconstructed {
				if patched, err := sjson.SetRawBytes(finalResponse, "output", outputJSON); err == nil {
					finalResponse = patched
			REDACTED
		REDACTED
	REDACTED
		finalResponse = supplementCompactionItemFromSSE(c, finalResponse, bodyText)
		body = finalResponse
		if originalModel != mappedModel {
			body = s.replaceModelInResponseBody(body, mappedModel, originalModel)
	REDACTED
		// Correct tool calls in final response
		body = s.correctToolCallsInResponseBody(body)
		restoredBody, restoreErr := restoreGrokResponsesClientToolPayload(c, body)
		if restoreErr != nil {
			return nil, fmt.Errorf("restore Grok Responses client tool response: %w", restoreErr)
	REDACTED
		restoredBody, restoreErr = restoreOpenAIResponsesNamespacePayload(c, restoredBody)
		if restoreErr != nil {
			return nil, fmt.Errorf("restore OpenAI namespace response: %w", restoreErr)
	REDACTED
		restoredBody = restoreCodexToolNamesFromContext(c, restoredBody)
		body = restoredBody
REDACTED else {
		if originalModel != mappedModel {
			bodyText = s.replaceModelInSSEBody(bodyText, mappedModel, originalModel)
	REDACTED
		body = []byte(bodyText)
REDACTED

	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	logOpenAISuccessMissingUsage(c.Request.Context(), c, account, resp, usage, terminalType, false)
	s.relayOpenAICodexTurnState(c, account, resp.Header)

	contentType := "application/json; charset=utf-8"
	if !ok {
		contentType = resp.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "text/event-stream"
	REDACTED
REDACTED
	if !writeOpenAICompactSSEBridge(c, resp.StatusCode, body) {
		c.Data(resp.StatusCode, contentType, body)
REDACTED

	return &openaiNonStreamingResult{
		OpenAIUsage:      usage,
		usage:            usage,
		responseID:       extractOpenAIResponseIDFromJSONBytes(body),
		imageCount:       countOpenAIImageOutputsFromSSEBody(bodyText),
		imageOutputSizes: collectOpenAIImageOutputSizesFromSSEBody(bodyText),
		searchCount:      countGrokNativeSearchCallsFromSSEBody(bodyText),
REDACTED, nil
REDACTED

func extractOpenAISSETerminalEvent(body string) (string, []byte, bool) {
	var terminalType string
	var terminalPayload []byte
	forEachOpenAISSEFrame(body, func(eventType string, data []byte) {
		switch eventType {
		case "response.completed", "response.done", "response.failed", "response.incomplete", "response.cancelled", "response.canceled", "error":
			terminalType = eventType
			terminalPayload = append([]byte(nil), data...)
	REDACTED
REDACTED)
	if terminalPayload != nil {
		return terminalType, terminalPayload, true
REDACTED
	return "", nil, false
REDACTED

func extractOpenAISSEErrorMessage(payload []byte) string {
	if len(payload) == 0 {
		return ""
REDACTED
	for _, path := range []string{"response.error.message", "error.message", "message"REDACTED {
		if msg := strings.TrimSpace(gjson.GetBytes(payload, path).String()); msg != "" {
			return sanitizeUpstreamErrorMessage(msg)
	REDACTED
REDACTED
	return sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(payload)))
REDACTED

func buildOpenAIResponseFailedSSE(responseID, model string, source []byte, fallbackMessage string) string {
	responseID = strings.TrimSpace(responseID)
	if responseID == "" {
		responseID = "resp_" + strings.ReplaceAll(uuid.NewString(), "-", "")
REDACTED
	errorType := strings.TrimSpace(gjson.GetBytes(source, "error.type").String())
	if errorType == "" {
		errorType = strings.TrimSpace(gjson.GetBytes(source, "response.error.type").String())
REDACTED
	code := strings.TrimSpace(gjson.GetBytes(source, "error.code").String())
	if code == "" {
		code = strings.TrimSpace(gjson.GetBytes(source, "response.error.code").String())
REDACTED
	if code == "" {
		code = "upstream_error"
REDACTED
	message := extractOpenAISSEErrorMessage(source)
	if message == "" {
		message = strings.TrimSpace(fallbackMessage)
REDACTED
	if message == "" {
		message = "Upstream response failed"
REDACTED
	errorBody := gin.H{"code": code, "message": messageREDACTED
	if errorType != "" {
		errorBody["type"] = errorType
REDACTED
	response := gin.H{
		"id":     responseID,
		"object": "response",
		"status": "failed",
		"output": []any{REDACTED,
		"error":  errorBody,
REDACTED
	if model = strings.TrimSpace(model); model != "" {
		response["model"] = model
REDACTED
	payload, err := marshalOpenAIUpstreamJSON(gin.H{
		"type":     "response.failed",
		"response": response,
REDACTED)
	if err != nil {
		// All values above are JSON primitives, so this is only a defensive fallback.
		payload = []byte(`{"type":"response.failed","response":{"status":"failed","output":[],"error":{"code":"upstream_error","message":"Upstream response failed"REDACTEDREDACTEDREDACTED`)
REDACTED
	return "event: response.failed\ndata: " + string(payload) + "\n\n"
REDACTED

func sanitizeOpenAIResponseFailedEventForClient(payload []byte, eventType string, clientOutputStarted bool) ([]byte, bool) {
	eventType = strings.TrimSpace(eventType)
	isFailedEvent := eventType == "response.failed"
	if (!isFailedEvent && eventType != "error") || len(payload) == 0 || !gjson.ValidBytes(payload) {
		return payload, false
REDACTED
	updated := payload
	// 容量降载码对 Codex CLI 是致命错误；事件既然要写给客户端（failover 已不可用），
	// 就改写为客户端可重试的错误码。error 帧与 response.failed 都要改：上游降载
	// 总是先推 error 帧再收 failed，两帧携带同一个错误。
	if rewritten, changed := sanitizeOpenAICapacityShedErrorCodeForClient(updated); changed {
		updated = rewritten
REDACTED
	if !isFailedEvent {
		return updated, !bytes.Equal(updated, payload)
REDACTED
	if clientOutputStarted && isOpenAIContextWindowError(extractOpenAISSEErrorMessage(payload), payload) {
		errorPath := ""
		switch {
		case gjson.GetBytes(updated, "response.error").Exists():
			errorPath = "response.error"
		case gjson.GetBytes(updated, "error").Exists():
			errorPath = "error"
	REDACTED
		if errorPath != "" {
			next, err := sjson.SetBytes(updated, errorPath+".type", "invalid_request_error")
			if err != nil {
				return payload, false
		REDACTED
			updated = next
			next, err = sjson.SetBytes(updated, errorPath+".code", "context_length_exceeded")
			if err != nil {
				return payload, false
		REDACTED
			updated = next
	REDACTED
REDACTED
	if !gjson.GetBytes(updated, "response").Exists() {
		return updated, !bytes.Equal(updated, payload)
REDACTED
	for _, path := range []string{
		"response.instructions",
		"response.output",
		"response.usage",
		"response.metadata",
		"response.reasoning",
		"response.tools",
		"response.tool_choice",
		"response.parallel_tool_calls",
		"response.text",
		"response.truncation",
		"response.max_output_tokens",
		"response.incomplete_details",
REDACTED {
		next, err := sjson.DeleteBytes(updated, path)
		if err != nil {
			return payload, false
	REDACTED
		updated = next
REDACTED
	return updated, !bytes.Equal(updated, payload)
REDACTED

func (s *OpenAIGatewayService) writeOpenAINonStreamingProtocolError(resp *http.Response, c *gin.Context, message string) error {
	message = sanitizeUpstreamErrorMessage(strings.TrimSpace(message))
	if message == "" {
		message = "Upstream returned an invalid non-streaming response"
REDACTED
	setOpsUpstreamError(c, http.StatusBadGateway, message, "")
	// body-signal compact 心跳可能已把响应头提交为 200，此时只能以
	// response.failed 终止事件回传错误，不能再写 JSON+状态码。
	if openAICompactClientWantsStream(c) && StopOpenAICompactSSEKeepaliveCommitted(c) {
		writeOpenAICompactSSEFailureMessage(c, http.StatusBadGateway, "upstream_error", message)
		return fmt.Errorf("non-streaming openai protocol error: %s", message)
REDACTED
	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	c.JSON(http.StatusBadGateway, gin.H{
		"error": gin.H{
			"type":    "upstream_error",
			"message": message,
	REDACTED,
REDACTED)
	return fmt.Errorf("non-streaming openai protocol error: %s", message)
REDACTED

func extractCodexFinalResponse(body string) ([]byte, bool) {
	var finalResponse []byte
	forEachOpenAISSEFrame(body, func(eventType string, data []byte) {
		if finalResponse != nil {
			return
	REDACTED
		if normalized, changed := normalizeCompletedImageGenerationStatus(data); changed {
			data = normalized
	REDACTED
		if eventType == "response.done" || eventType == "response.completed" {
			if response := gjson.GetBytes(data, "response"); response.Exists() && response.Type == gjson.JSON && response.Raw != "" {
				finalResponse = []byte(response.Raw)
		REDACTED
	REDACTED
REDACTED)
	if finalResponse != nil {
		return finalResponse, true
REDACTED
	return nil, false
REDACTED

func normalizeCompletedImageGenerationStatus(data []byte) ([]byte, bool) {
	if len(data) == 0 || !gjson.ValidBytes(data) {
		return data, false
REDACTED

	shouldNormalize := func(item gjson.Result) bool {
		if !item.Exists() || !item.IsObject() ||
			strings.TrimSpace(item.Get("type").String()) != "image_generation_call" {
			return false
	REDACTED
		switch strings.TrimSpace(item.Get("status").String()) {
		case "generating", "in_progress":
			return strings.TrimSpace(item.Get("result").String()) != ""
		default:
			return false
	REDACTED
REDACTED

	eventType := strings.TrimSpace(gjson.GetBytes(data, "type").String())
	switch eventType {
	case "response.output_item.done":
		if !shouldNormalize(gjson.GetBytes(data, "item")) {
			return data, false
	REDACTED
		updated, err := sjson.SetBytes(data, "item.status", "completed")
		if err != nil {
			return data, false
	REDACTED
		return updated, true
	case "response.completed", "response.done":
		output := gjson.GetBytes(data, "response.output")
		if !output.Exists() || !output.IsArray() {
			return data, false
	REDACTED
		updated := data
		changed := false
		for i, item := range output.Array() {
			if !shouldNormalize(item) {
				continue
		REDACTED
			next, err := sjson.SetBytes(updated, "response.output."+strconv.Itoa(i)+".status", "completed")
			if err != nil {
				return data, false
		REDACTED
			updated = next
			changed = true
	REDACTED
		return updated, changed
	default:
		return data, false
REDACTED
REDACTED

func normalizeResponsesStreamingTerminalOutput(data []byte, acc *apicompat.BufferedResponseAccumulator, imageOutputs []json.RawMessage) ([]byte, bool) {
	eventType := strings.TrimSpace(gjson.GetBytes(data, "type").String())
	switch eventType {
	case "response.completed", "response.done", "response.incomplete", "response.cancelled", "response.canceled":
	default:
		return data, false
REDACTED

	output := gjson.GetBytes(data, "response.output")
	hasAccumulatedOutput := (acc != nil && acc.HasContent()) || len(imageOutputs) > 0
	if output.Exists() && output.IsArray() {
		if len(output.Array()) > 0 || !hasAccumulatedOutput {
			return data, false
	REDACTED
REDACTED

	outputJSON := []byte("[]")
	if reconstructed, ok := buildResponsesOutputJSON(acc, imageOutputs); ok {
		outputJSON = reconstructed
REDACTED
	updated, err := sjson.SetRawBytes(data, "response.output", outputJSON)
	if err != nil {
		return data, false
REDACTED
	return updated, true
REDACTED

func responsesStreamEventMayContributeToOutput(eventType string) bool {
	switch eventType {
	case "response.output_text.delta",
		"response.output_item.added",
		"response.function_call_arguments.delta",
		"response.reasoning_summary_text.delta":
		return true
	default:
		return false
REDACTED
REDACTED

// collectRawResponsesOutputItemsFromSSE 按到达顺序收集 SSE 流中
// response.output_item.done 携带的原始 item。除已产生结果但仍停留在进行中
// 的图片状态外，item 以 raw JSON 逐字节保留，
// 避免经窄结构体重建时丢弃 encrypted_content/summary/opaque 等 compact
// 专属或未来新增字段（#3777 问题 2）。若整条流没有任何 done 事件，退回
// 收集 output_item.added 中的 compaction 类 item——compaction 结果没有
// delta 事件，部分上游只在 added 事件中携带完整 item。
func collectRawResponsesOutputItemsFromSSE(bodyText string) ([]byte, bool) {
	var items []json.RawMessage
	seen := make(map[string]struct{REDACTED)
	hasCompactionItem := false
	appendItem := func(item gjson.Result) {
		if !item.Exists() || !item.IsObject() {
			return
	REDACTED
		key := strings.TrimSpace(item.Get("id").String())
		if key == "" {
			key = item.Raw
	REDACTED
		if _, dup := seen[key]; dup {
			return
	REDACTED
		seen[key] = struct{REDACTED{REDACTED
		if isResponsesCompactionItemType(item.Get("type").String()) {
			hasCompactionItem = true
	REDACTED
		items = append(items, json.RawMessage(item.Raw))
REDACTED
	forEachOpenAISSEFrame(bodyText, func(eventType string, data []byte) {
		data = []byte(openAICompatPayloadWithEventType(string(data), eventType))
		if normalized, changed := normalizeCompletedImageGenerationStatus(data); changed {
			data = normalized
	REDACTED
		if eventType != "response.output_item.done" {
			return
	REDACTED
		appendItem(gjson.GetBytes(data, "item"))
REDACTED)
	// done 事件未携带 compaction item 时再看 added：覆盖"其他 item 有 done、
	// compaction 只在 added 中"的混合形态；done 已含 compaction 时跳过，
	// 避免同一 item 在无 id 可去重时被收集两份（Codex 要求恰好一个）。
	if !hasCompactionItem {
		forEachOpenAISSEFrame(bodyText, func(eventType string, data []byte) {
			if eventType != "response.output_item.added" {
				return
		REDACTED
			item := gjson.GetBytes(data, "item")
			if !isResponsesCompactionItemType(item.Get("type").String()) {
				return
		REDACTED
			appendItem(item)
	REDACTED)
REDACTED
	if len(items) == 0 {
		return nil, false
REDACTED
	outputJSON, err := json.Marshal(items)
	if err != nil {
		return nil, false
REDACTED
	return outputJSON, true
REDACTED

// isResponsesCompactionItemType reports whether the item type is the Codex
// remote-compact result item ("compaction", upstream alias "compaction_summary").
func isResponsesCompactionItemType(itemType string) bool {
	switch strings.TrimSpace(itemType) {
	case "compaction", "compaction_summary":
		return true
	default:
		return false
REDACTED
REDACTED

// supplementCompactionItemFromSSE 保证 compact 请求的终态 output 携带
// compaction item：终态 output 非空但缺失 compaction、而原始事件流的
// output_item.done（或 added）中存在时（上游不一致形态），以 raw JSON 补入。
// Codex remote compact v2 只从 output_item.done 收集 item 且要求恰好一个
// compaction item——纯流式透传（v0.1.146）下客户端直接读事件流天然拿得到，
// SSE→JSON 提取链路必须给出等价结果。非 compact 请求原样返回。
func supplementCompactionItemFromSSE(c *gin.Context, finalResponse []byte, bodyText string) []byte {
	if !isOpenAIResponsesCompactPath(c) {
		return finalResponse
REDACTED
	if len(gjson.GetBytes(finalResponse, "output").Array()) == 0 {
		// 空 output 由 reconstructResponseOutputFromSSE 整体修补，不在此处理。
		return finalResponse
REDACTED
	if responsesOutputHasCompactionItem(finalResponse) {
		return finalResponse
REDACTED
	item, found := findRawCompactionItemFromSSE(bodyText)
	if !found {
		return finalResponse
REDACTED
	patched, err := sjson.SetRawBytes(finalResponse, "output.-1", item)
	if err != nil {
		return finalResponse
REDACTED
	return patched
REDACTED

// responsesOutputHasCompactionItem reports whether the response JSON already
// carries a compaction item in its output array.
func responsesOutputHasCompactionItem(response []byte) bool {
	for _, item := range gjson.GetBytes(response, "output").Array() {
		if isResponsesCompactionItemType(item.Get("type").String()) {
			return true
	REDACTED
REDACTED
	return false
REDACTED

// findRawCompactionItemFromSSE 从原始 SSE 事件流中提取第一个 compaction 类
// item 的 raw JSON：output_item.done 优先，output_item.added 兜底。
func findRawCompactionItemFromSSE(bodyText string) (json.RawMessage, bool) {
	var found json.RawMessage
	pick := func(eventType string) {
		forEachOpenAISSEFrame(bodyText, func(effectiveType string, data []byte) {
			if found != nil {
				return
		REDACTED
			if effectiveType != eventType {
				return
		REDACTED
			item := gjson.GetBytes(data, "item")
			if !item.IsObject() || !isResponsesCompactionItemType(item.Get("type").String()) {
				return
		REDACTED
			found = json.RawMessage(item.Raw)
	REDACTED)
REDACTED
	pick("response.output_item.done")
	if found == nil {
		pick("response.output_item.added")
REDACTED
	return found, found != nil
REDACTED

// reconstructResponseOutputFromSSE scans raw SSE body text and returns a
// JSON-encoded output array for a terminal event whose output is empty.
// Raw output_item.done items are preferred: per the Responses protocol they
// are the authoritative final form of each item. Delta accumulation only
// covers text/function_call/reasoning content and silently drops unknown
// item types such as compaction — Codex remote compact v2 then fails with
// "expected exactly one compaction output item, got 0" (#3887).
// Returns (nil, false) if nothing could be reconstructed.
func reconstructResponseOutputFromSSE(bodyText string) ([]byte, bool) {
	if outputJSON, ok := collectRawResponsesOutputItemsFromSSE(bodyText); ok {
		return outputJSON, true
REDACTED
	acc := apicompat.NewBufferedResponseAccumulator()
	imageOutputs := make([]json.RawMessage, 0, 1)
	seenImages := make(map[string]struct{REDACTED)
	forEachOpenAISSEFrame(bodyText, func(eventType string, data []byte) {
		data = []byte(openAICompatPayloadWithEventType(string(data), eventType))
		if imageOutput, ok := extractImageGenerationOutputFromSSEData(data, seenImages); ok {
			imageOutputs = append(imageOutputs, imageOutput)
	REDACTED
		if responsesStreamEventMayContributeToOutput(eventType) {
			var event apicompat.ResponsesStreamEvent
			if err := json.Unmarshal(data, &event); err == nil {
				acc.ProcessEvent(&event)
		REDACTED
	REDACTED
REDACTED)
	return buildResponsesOutputJSON(acc, imageOutputs)
REDACTED

func buildResponsesOutputJSON(acc *apicompat.BufferedResponseAccumulator, imageOutputs []json.RawMessage) ([]byte, bool) {
	if (acc == nil || !acc.HasContent()) && len(imageOutputs) == 0 {
		return nil, false
REDACTED
	var output []json.RawMessage
	if acc != nil && acc.HasContent() {
		outputJSON, err := json.Marshal(acc.BuildOutput())
		if err == nil {
			_ = json.Unmarshal(outputJSON, &output)
	REDACTED
REDACTED
	output = append(output, imageOutputs...)
	if len(output) == 0 {
		return nil, false
REDACTED

	outputJSON, err := json.Marshal(output)
	if err != nil {
		return nil, false
REDACTED
	return outputJSON, true
REDACTED

func extractImageGenerationOutputFromSSEData(data []byte, seen map[string]struct{REDACTED) (json.RawMessage, bool) {
	if len(data) == 0 || !gjson.ValidBytes(data) {
		return nil, false
REDACTED
	if gjson.GetBytes(data, "type").String() != "response.output_item.done" {
		return nil, false
REDACTED
	item := gjson.GetBytes(data, "item")
	if !item.Exists() || !item.IsObject() || item.Get("type").String() != "image_generation_call" {
		return nil, false
REDACTED
	if strings.TrimSpace(item.Get("result").String()) == "" {
		return nil, false
REDACTED
	key := strings.TrimSpace(item.Get("id").String())
	if key == "" {
		key = strings.TrimSpace(item.Get("output_format").String()) + "|" + strings.TrimSpace(item.Get("result").String())
REDACTED
	if key != "" && seen != nil {
		if _, exists := seen[key]; exists {
			return nil, false
	REDACTED
		seen[key] = struct{REDACTED{REDACTED
REDACTED
	return json.RawMessage(item.Raw), true
REDACTED

func (s *OpenAIGatewayService) parseSSEUsageFromBody(body string) *OpenAIUsage {
	usage := &OpenAIUsage{REDACTED
	forEachOpenAISSEFrame(body, func(eventType string, data []byte) {
		s.parseSSEUsageBytesWithType(data, eventType, usage)
REDACTED)
	return usage
REDACTED

func (s *OpenAIGatewayService) replaceModelInSSEBody(body, fromModel, toModel string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if _, ok := extractOpenAISSEDataLine(line); !ok {
			continue
	REDACTED
		lines[i] = s.replaceModelInSSELine(line, fromModel, toModel)
REDACTED
	return strings.Join(lines, "\n")
REDACTED
