package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	deepSeekChatCompletionsEndpoint  = "/chat/completions"
	deepSeekResponsesEndpoint        = "/responses"
	deepSeekResponsesCompactEndpoint = "/responses/compact"
)

// buildDeepSeekEndpointURL appends a native DeepSeek endpoint to the configured
// API root. Unlike the generic OpenAI-compatible builder, it never inserts /v1.
func buildDeepSeekEndpointURL(root, endpoint string) string {
	normalized := strings.TrimSpace(root)
	endpoint = "/" + strings.TrimLeft(strings.TrimSpace(endpoint), "/")
	parsed, err := url.Parse(normalized)
	if err != nil {
		return strings.TrimRight(normalized, "/") + endpoint
	}

	path := strings.TrimRight(parsed.Path, "/")
	if !strings.HasSuffix(path, endpoint) {
		path += endpoint
	}
	parsed.Path = path
	parsed.RawPath = ""
	parsed.Fragment = ""
	return parsed.String()
}

func buildDeepSeekChatCompletionsURL(root string) string {
	return buildDeepSeekEndpointURL(root, deepSeekChatCompletionsEndpoint)
}

func buildDeepSeekResponsesURL(root string) string {
	return buildDeepSeekEndpointURL(root, deepSeekResponsesEndpoint)
}

func (s *OpenAIGatewayService) deepSeekEndpointURL(account *Account, endpoint string) (string, error) {
	if account == nil || !account.IsDeepSeekAPIKey() {
		return "", fmt.Errorf("deepseek endpoint requires a DeepSeek API key account")
	}
	root, err := normalizeDeepSeekBaseURL(account.GetDeepSeekBaseURL())
	if err != nil {
		return "", fmt.Errorf("invalid deepseek base_url: %w", err)
	}
	root, err = s.validateUpstreamBaseURL(root)
	if err != nil {
		return "", fmt.Errorf("invalid deepseek base_url: %w", err)
	}
	switch endpoint {
	case deepSeekChatCompletionsEndpoint:
		return buildDeepSeekChatCompletionsURL(root), nil
	case deepSeekResponsesEndpoint:
		return buildDeepSeekResponsesURL(root), nil
	case deepSeekResponsesCompactEndpoint:
		return buildDeepSeekEndpointURL(root, deepSeekResponsesCompactEndpoint), nil
	default:
		return "", fmt.Errorf("unsupported deepseek endpoint: %s", endpoint)
	}
}

type deepSeekResponsesRelayResult struct {
	usage            *OpenAIUsage
	firstTokenMs     *int
	responseID       string
	terminalEvent    string
	clientDisconnect bool
}

func deepSeekResponsesTerminalFromJSON(body []byte) string {
	if eventType := strings.TrimSpace(gjson.GetBytes(body, "type").String()); openAIStreamEventTypeIsTerminal(eventType) {
		return eventType
	}
	switch strings.TrimSpace(gjson.GetBytes(body, "status").String()) {
	case "completed":
		return "response.completed"
	case "incomplete":
		return "response.incomplete"
	case "failed":
		return "response.failed"
	case "cancelled", "canceled":
		return "response.cancelled"
	default:
		return ""
	}
}

func deepSeekResponsesRequiresUsage(terminalEvent string) bool {
	switch terminalEvent {
	case "response.completed", "response.done", "response.incomplete":
		return true
	default:
		return false
	}
}

func (s *OpenAIGatewayService) writeDeepSeekResponsesHeaders(c *gin.Context, resp *http.Response, stream bool) {
	writeOpenAIPassthroughResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	if requestID := openAICompatibleUpstreamRequestID(resp.Header); requestID != "" {
		c.Writer.Header().Set("x-request-id", requestID)
		if vendorID := strings.TrimSpace(resp.Header.Get("x-deepseek-request-id")); vendorID != "" {
			c.Writer.Header().Set("x-deepseek-request-id", vendorID)
		}
	}
	if stream {
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("Connection", "keep-alive")
		c.Writer.Header().Set("X-Accel-Buffering", "no")
	} else if strings.TrimSpace(c.Writer.Header().Get("Content-Type")) == "" {
		c.Writer.Header().Set("Content-Type", "application/json")
	}
}

func (s *OpenAIGatewayService) handleDeepSeekResponsesJSON(
	resp *http.Response,
	c *gin.Context,
	account *Account,
) (*deepSeekResponsesRelayResult, error) {
	sanitizeDeepSeekResponseHeadersInPlace(account, resp.Header)
	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}
	body = redactDeepSeekAPIKey(account, body)
	if !gjson.ValidBytes(body) {
		return nil, s.newOpenAIStreamFailoverError(
			c,
			account,
			true,
			openAICompatibleUpstreamRequestID(resp.Header),
			body,
			"DeepSeek Responses returned invalid JSON",
			resp.Header,
		)
	}
	observer := upstreamResponseModelObserverFromContext(c)
	if observer == nil {
		observer = beginUpstreamResponseModelObservation(c)
	}
	observer.ObserveOpenAI(body, strings.TrimSpace(gjson.GetBytes(body, "type").String()))

	usage := &OpenAIUsage{}
	if parsed, ok := extractOpenAIUsageFromJSONBytes(body); ok {
		*usage = parsed
	}
	if hit, code, message := detectOpenAICyberPolicy(body); hit {
		MarkOpsCyberPolicy(c, CyberPolicyMark{
			Code:           code,
			Message:        message,
			Body:           truncateString(string(body), 4096),
			UpstreamStatus: http.StatusOK,
			UpstreamInTok:  usage.InputTokens,
			UpstreamOutTok: usage.OutputTokens,
		})
	}
	terminalEvent := deepSeekResponsesTerminalFromJSON(body)
	if terminalEvent == "" {
		return nil, s.newOpenAIStreamFailoverError(
			c,
			account,
			true,
			openAICompatibleUpstreamRequestID(resp.Header),
			body,
			"DeepSeek Responses returned JSON without a terminal status or type",
			resp.Header,
		)
	}
	if deepSeekResponsesRequiresUsage(terminalEvent) && !hasBillableOpenAIUsage(*usage) {
		return nil, newDeepSeekMissingUsageFailoverError(c, account, openAICompatibleUpstreamRequestID(resp.Header))
	}

	writeBody := body
	if restored, restoreErr := restoreOpenAIResponsesClientToolPayload(c, body); restoreErr != nil {
		return nil, fmt.Errorf("restore DeepSeek Responses client tools: %w", restoreErr)
	} else {
		writeBody = restored
	}

	s.writeDeepSeekResponsesHeaders(c, resp, false)
	c.Data(resp.StatusCode, c.Writer.Header().Get("Content-Type"), writeBody)
	result := &deepSeekResponsesRelayResult{
		usage:         usage,
		responseID:    extractOpenAIResponseIDFromJSONBytes(body),
		terminalEvent: terminalEvent,
	}
	if terminalEvent == "response.failed" {
		return result, errors.New("DeepSeek Responses upstream returned response.failed")
	}
	return result, nil
}

func (s *OpenAIGatewayService) handleDeepSeekResponsesStream(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
	startTime time.Time,
) (*deepSeekResponsesRelayResult, error) {
	sanitizeDeepSeekResponseHeadersInPlace(account, resp.Header)
	observer := upstreamResponseModelObserverFromContext(c)
	if observer == nil {
		observer = beginUpstreamResponseModelObservation(c)
	}
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
	}

	usage := &OpenAIUsage{}
	var firstTokenMs *int
	responseID := ""
	terminalEvent := ""
	currentEventType := ""
	currentEventData := make([]string, 0, 1)
	clientDisconnected := false
	clientOutputStarted := false
	pendingLine := make([]byte, 0, 64*1024)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	requestID := openAICompatibleUpstreamRequestID(resp.Header)
	sensitiveGuard := newDeepSeekSSESensitiveEventGuard(account, deepSeekSSESensitiveProtocolResponses)
	var cyberMark *CyberPolicyMark
	defer func() {
		if cyberMark == nil {
			return
		}
		cyberMark.UpstreamInTok = usage.InputTokens
		cyberMark.UpstreamOutTok = usage.OutputTokens
		MarkOpsCyberPolicy(c, *cyberMark)
	}()

	result := func() *deepSeekResponsesRelayResult {
		return &deepSeekResponsesRelayResult{
			usage:            usage,
			firstTokenMs:     firstTokenMs,
			responseID:       responseID,
			terminalEvent:    terminalEvent,
			clientDisconnect: clientDisconnected,
		}
	}
	processEvent := func(commitTerminal bool) (bool, error) {
		hadData := len(currentEventData) > 0
		data := strings.Join(currentEventData, "\n")
		currentEventData = currentEventData[:0]
		eventType := currentEventType
		currentEventType = ""
		payload := strings.TrimSpace(data)
		if payload == "" {
			if hadData {
				return false, errors.New("DeepSeek Responses stream returned empty SSE data")
			}
			return false, nil
		}
		if payload == "[DONE]" {
			return false, errors.New("DeepSeek Responses stream must not contain [DONE]")
		}
		dataBytes := redactDeepSeekAPIKey(account, []byte(data))
		if !gjson.ValidBytes(dataBytes) {
			return false, errors.New("DeepSeek Responses stream returned malformed JSON data")
		}
		payloadObject := gjson.ParseBytes(dataBytes)
		if !payloadObject.IsObject() {
			return false, errors.New("DeepSeek Responses stream data must be a JSON object")
		}
		payloadType := strings.TrimSpace(payloadObject.Get("type").String())
		if payloadType == "" {
			return false, errors.New("DeepSeek Responses stream payload has no type")
		}
		if eventType != "" && eventType != payloadType {
			return false, errors.New("DeepSeek Responses stream event type does not match its payload")
		}
		eventType = payloadType
		observer.ObserveOpenAI(dataBytes, eventType)
		s.parseSSEUsageBytes(dataBytes, usage)
		if hit, code, message := detectOpenAICyberPolicy(dataBytes); hit {
			if cyberMark == nil {
				cyberMark = &CyberPolicyMark{
					Code:           code,
					Message:        message,
					Body:           truncateString(string(dataBytes), 4096),
					UpstreamStatus: http.StatusOK,
				}
			}
		}
		if responseID == "" {
			responseID = extractOpenAIResponseIDFromJSONBytes(dataBytes)
		}
		if firstTokenMs == nil && openAIStreamDataStartsVisibleOutput(strings.TrimSpace(string(dataBytes)), eventType) {
			ms := int(time.Since(startTime).Milliseconds())
			firstTokenMs = &ms
		}
		if openAIStreamEventTypeIsTerminal(eventType) {
			if commitTerminal {
				terminalEvent = eventType
				return true, nil
			}
		}
		return false, nil
	}
	collectLine := func(rawLine []byte) bool {
		line := bytes.TrimSuffix(rawLine, []byte{'\r'})
		trimmed := strings.TrimSpace(string(line))
		if trimmed == "" {
			return true
		}
		if strings.HasPrefix(trimmed, "event:") {
			currentEventType = strings.TrimSpace(strings.TrimPrefix(trimmed, "event:"))
			return false
		}
		data, ok := extractOpenAISSEDataLine(string(line))
		if !ok {
			return false
		}
		currentEventData = append(currentEventData, data)
		return false
	}
	missingTerminal := func(readErr error) (*deepSeekResponsesRelayResult, error) {
		message := "DeepSeek Responses stream ended before a terminal event"
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			message = "DeepSeek Responses stream read failed before a terminal event: " + readErr.Error()
		}
		if !clientOutputStarted {
			return nil, s.newOpenAIStreamFailoverError(c, account, true, requestID, nil, message, resp.Header)
		}
		return result(), errors.New(message)
	}
	protocolFailure := func(protocolErr error) (*deepSeekResponsesRelayResult, error) {
		if protocolErr == nil {
			return result(), nil
		}
		if !clientOutputStarted {
			return nil, s.newOpenAIStreamFailoverError(
				c,
				account,
				true,
				requestID,
				nil,
				protocolErr.Error(),
				resp.Header,
			)
		}
		return result(), protocolErr
	}
	finishTerminal := func() (*deepSeekResponsesRelayResult, error) {
		if deepSeekResponsesRequiresUsage(terminalEvent) && !hasBillableOpenAIUsage(*usage) {
			_ = newDeepSeekMissingUsageFailoverError(c, account, requestID)
			return result(), errors.New(deepSeekMissingUsageMsg)
		}
		if terminalEvent == "response.failed" {
			return result(), errors.New("DeepSeek Responses upstream returned response.failed")
		}
		return result(), nil
	}
	writeWire := func(wire []byte) {
		if clientDisconnected || len(wire) == 0 {
			return
		}
		if _, writeErr := c.Writer.Write(wire); writeErr != nil {
			clientDisconnected = true
			return
		}
		clientOutputStarted = true
		flusher.Flush()
	}

	s.writeDeepSeekResponsesHeaders(c, resp, true)
	type readResult struct {
		data []byte
		err  error
	}
	readResults := make(chan readResult, 1)
	stopReading := make(chan struct{})
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		defer close(readResults)
		for {
			buf := make([]byte, 32*1024)
			n, readErr := resp.Body.Read(buf)
			read := readResult{err: readErr}
			if n > 0 {
				read.data = buf[:n]
			}
			select {
			case readResults <- read:
			case <-stopReading:
				return
			}
			if readErr != nil {
				return
			}
		}
	}()
	defer func() {
		close(stopReading)
		_ = resp.Body.Close()
		<-readerDone
	}()

	streamInterval := time.Duration(0)
	if s.cfg != nil && s.cfg.Gateway.StreamDataIntervalTimeout > 0 {
		streamInterval = time.Duration(s.cfg.Gateway.StreamDataIntervalTimeout) * time.Second
	}
	var timeoutTimer *time.Timer
	var timeoutCh <-chan time.Time
	resetTimeout := func() {
		if streamInterval <= 0 {
			return
		}
		if timeoutTimer == nil {
			timeoutTimer = time.NewTimer(streamInterval)
			timeoutCh = timeoutTimer.C
			return
		}
		if !timeoutTimer.Stop() {
			select {
			case <-timeoutTimer.C:
			default:
			}
		}
		timeoutTimer.Reset(streamInterval)
	}
	stopTimeout := func() {
		if timeoutTimer == nil {
			return
		}
		if !timeoutTimer.Stop() {
			select {
			case <-timeoutTimer.C:
			default:
			}
		}
	}
	resetTimeout()
	defer stopTimeout()

	for {
		var read readResult
		var ok bool
		select {
		case read, ok = <-readResults:
			if !ok {
				return missingTerminal(io.EOF)
			}
			if len(read.data) > 0 {
				resetTimeout()
			}
		case <-timeoutCh:
			_ = resp.Body.Close()
			return protocolFailure(fmt.Errorf("%w after %s", errDeepSeekSSEDataIntervalTimeout, streamInterval))
		}
		if len(read.data) > 0 {
			chunk := read.data
			pendingLine = append(pendingLine, chunk...)
			for {
				newline := bytes.IndexByte(pendingLine, '\n')
				if newline < 0 {
					break
				}
				wireLine := pendingLine[:newline+1]
				line := wireLine[:newline]
				pendingLine = pendingLine[newline+1:]
				atEventBoundary := collectLine(line)
				terminal := false
				if atEventBoundary {
					var processErr error
					terminal, processErr = processEvent(true)
					if processErr != nil {
						return protocolFailure(processErr)
					}
				}
				if guardErr := sensitiveGuard.PushWireLine(wireLine, func(safeWire []byte) error {
					writeWire(safeWire)
					return nil
				}); guardErr != nil {
					return result(), guardErr
				}
				if terminal {
					return finishTerminal()
				}
			}
			if len(pendingLine) > maxLineSize {
				return result(), fmt.Errorf("DeepSeek Responses SSE line exceeds max size %d", maxLineSize)
			}
		}
		if read.err != nil {
			if len(pendingLine) > 0 {
				_ = collectLine(pendingLine)
			}
			if len(currentEventData) > 0 {
				if _, processErr := processEvent(false); processErr != nil {
					return protocolFailure(processErr)
				}
			}
			if len(pendingLine) > 0 {
				if guardErr := sensitiveGuard.PushWireLine(pendingLine, func(safeWire []byte) error {
					writeWire(safeWire)
					return nil
				}); guardErr != nil {
					return result(), guardErr
				}
			}
			if guardErr := sensitiveGuard.Finish(func(safeWire []byte) error {
				writeWire(safeWire)
				return nil
			}); guardErr != nil {
				return result(), guardErr
			}
			if terminalEvent != "" {
				return finishTerminal()
			}
			if ctx.Err() != nil {
				return result(), fmt.Errorf("DeepSeek Responses stream interrupted: %w", ctx.Err())
			}
			return missingTerminal(read.err)
		}
	}
}

// forwardDeepSeekResponses forwards the native Responses wire without passing
// through OpenAI OAuth/Codex transforms, fast policy, image bridging, or the
// Chat Completions compatibility bridge.
func (s *OpenAIGatewayService) forwardDeepSeekResponses(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()
	if account == nil || !account.IsDeepSeekAPIKey() {
		return nil, fmt.Errorf("deepseek responses requires a DeepSeek API key account")
	}
	if !gjson.ValidBytes(body) {
		return nil, fmt.Errorf("parse DeepSeek Responses request: invalid JSON")
	}
	if !IsDeepSeekResponsesInputValidated(c) {
		restoredBody, _, err := s.RestoreDeepSeekCompactInput(ctx, body)
		if err != nil {
			return nil, err
		}
		body = restoredBody
	}

	clearOpenAIResponsesClientToolMapping(c)
	compactPath := isOpenAIResponsesCompactPath(c)
	if shouldAdaptDeepSeekResponsesClientTools(account, body, compactPath) {
		adaptedBody, mapping, adaptErr := adaptOpenAIResponsesClientTools(body)
		if adaptErr != nil {
			return nil, fmt.Errorf("adapt DeepSeek Responses client tools: %w", adaptErr)
		}
		body = adaptedBody
		setOpenAIResponsesClientToolMapping(c, mapping)
	}
	// 上游 PR #7286：DeepSeek /responses 接受 input[].additional_tools 但静默忽略
	// 其中的工具声明（Codex Responses Lite 形状），工具会退化为 DSML 文本。原生
	// DeepSeek 账号在本 fork 经此函数早返、到不了通用 Forward 的改道路由，这里补上
	// Lite→chat 改道。只用 Lite 判定：chat 协议账号保持既有行为。
	if shouldForwardDeepSeekResponsesLiteViaChatCompletions(account, body) {
		return s.forwardResponsesViaRawChatCompletions(ctx, c, account, body)
	}

	// 上游 PR #7283：DeepSeek 原生 Responses 在携带 tools 时严格校验历史——assistant
	// 消息缺 reasoning 明文会被 400 拒绝（"reasoning_text ... must be passed back"），
	// 被说明消息打断的调用块报找不到 tool 输出。先重排调用块再补 reasoning 占位
	// （顺序不能反：补齐要基于规整后的历史）。本 fork 经 forwardDeepSeekResponses
	// 早返，钩子放这里而非通用 Forward。
	if !compactPath &&
		(account.GetAPIProtocol() == APIProtocolResponses || account.IsAdaptiveAPIProtocol()) {
		if reorderedBody, reordered := normalizeDeepSeekResponsesToolCallBlocks(body); reordered {
			body = reorderedBody
		}
		if replayedBody, replayed := ensureDeepSeekResponsesReasoningPlaceholders(body, openAIRequestBodyHasTools(body)); replayed {
			body = replayedBody
		}
	}

	// 原生路径早返绕过通用 buildUpstreamRequest。图片 url 别名与工具输出
	// 提升仍要在这里做，否则 Codex input_image.image_url 会被 DeepSeek 422。
	body = normalizeDeepSeekResponsesRequestBody(account, body)

	originalModel := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if originalModel == "" {
		return nil, fmt.Errorf("parse DeepSeek Responses request: model is required")
	}
	clientStream := gjson.GetBytes(body, "stream").Bool()
	billingModel := resolveOpenAIForwardModel(account, originalModel, "")
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)
	if IsDeepSeekCompactionMarked(c) && HasCompactionTriggerInInput(body) {
		return s.forwardDeepSeekRemoteCompactionV2(ctx, c, account, body, originalModel, billingModel, upstreamModel)
	}
	upstreamBody := body
	if upstreamModel != originalModel {
		upstreamBody = ReplaceModelInBody(body, upstreamModel)
	}

	token := account.GetDeepSeekAPIKey()
	if token == "" {
		return nil, fmt.Errorf("account %d missing api_key", account.ID)
	}
	endpoint := deepSeekResponsesEndpoint
	if compactPath {
		endpoint = deepSeekResponsesCompactEndpoint
	}
	targetURL, err := s.deepSeekEndpointURL(account, endpoint)
	if err != nil {
		return nil, err
	}
	SetActualOpenAIUpstreamEndpoint(c, endpoint)

	upstreamStart := time.Now()
	resp, err := s.sendCCUpstreamRequest(ctx, c, account, targetURL, upstreamBody, clientStream, token, "", "")
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	sanitizeDeepSeekResponseHeadersInPlace(account, resp.Header)
	if mapping, ok := openAIResponsesClientToolMapping(c); ok && isEventStreamResponse(resp.Header) {
		maxLineSize := defaultMaxLineSize
		if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
			maxLineSize = s.cfg.Gateway.MaxLineSize
		}
		resp.Body = newGrokResponsesClientToolStreamBody(resp.Body, mapping, maxLineSize)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		respBody, upstreamMsg := s.readOpenAIUpstreamError(resp)
		if failoverErr := s.failoverOpenAIUpstreamHTTPError(ctx, c, account, resp, respBody, upstreamMsg, upstreamModel); failoverErr != nil {
			return nil, failoverErr
		}
		return s.handleErrorResponse(ctx, resp, c, account, upstreamBody, billingModel)
	}

	reasoningEffort := extractOpenAIReasoningEffortFromBodyForAccount(account, upstreamBody, upstreamModel, billingModel, originalModel)
	serviceTier := extractOpenAIServiceTierFromBody(upstreamBody)
	var relayResult *deepSeekResponsesRelayResult
	if clientStream {
		relayResult, err = s.handleDeepSeekResponsesStream(ctx, resp, c, account, startTime)
	} else {
		relayResult, err = s.handleDeepSeekResponsesJSON(resp, c, account)
	}
	if relayResult == nil {
		return nil, err
	}
	usage := relayResult.usage
	responseID := strings.TrimSpace(relayResult.responseID)
	s.bindHTTPResponseAccount(ctx, c, account, responseID)
	if usage == nil {
		usage = &OpenAIUsage{}
	}

	result := &OpenAIForwardResult{
		RequestID:                     openAICompatibleUpstreamRequestID(resp.Header),
		ResponseID:                    responseID,
		Usage:                         *usage,
		Model:                         originalModel,
		BillingModel:                  billingModel,
		UpstreamModel:                 upstreamModel,
		UpstreamResponseModel:         observedUpstreamResponseModel(c),
		UpstreamResponseModelConflict: observedUpstreamResponseModelConflict(c),
		UpstreamEndpoint:              deepSeekResponsesEndpoint,
		UpstreamTerminalEvent:         relayResult.terminalEvent,
		ServiceTier:                   serviceTier,
		ReasoningEffort:               reasoningEffort,
		Stream:                        clientStream,
		Duration:                      time.Since(startTime),
		FirstTokenMs:                  relayResult.firstTokenMs,
		ClientDisconnect:              relayResult.clientDisconnect,
		ResponseHeaders:               resp.Header.Clone(),
	}
	return result, err
}
