package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const openAIMessagesResponsesUpstreamEndpoint = "/v1/responses"

type openAIMessagesWSReadOutcome struct {
	responseID    string
	terminalEvent string
	usage         OpenAIUsage
	firstTokenMs  *int
	err           error
}

// forwardOpenAIMessagesSingleTurnWS performs one direct-dial Responses WSv2
// turn. Starting WriteJSON is the no-failover boundary: every error after that
// point is returned as a normal error so the handler cannot resend the request.
func (s *OpenAIGatewayService) forwardOpenAIMessagesSingleTurnWS(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	responsesBody []byte,
	token string,
	promptCacheKey string,
	clientStream bool,
	originalModel string,
	billingModel string,
	upstreamModel string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	SetActualOpenAIUpstreamEndpoint(c, openAIMessagesResponsesUpstreamEndpoint)

	if err := validateOpenAIWSBearerToken(account, token); err != nil && (account == nil || !account.IsShadow()) {
		return nil, openAIMessagesWSPrewriteFailover(err, http.StatusBadGateway, nil, nil)
	}
	var reqBody map[string]any
	if err := json.Unmarshal(responsesBody, &reqBody); err != nil {
		return nil, openAIMessagesWSPrewriteFailover(fmt.Errorf("decode response.create body: %w", err), http.StatusBadGateway, nil, nil)
	}
	if reqBody == nil {
		return nil, openAIMessagesWSPrewriteFailover(errors.New("response.create body must be an object"), http.StatusBadGateway, nil, nil)
	}
	// This bridge is intentionally single-turn and never participates in OAuth
	// response continuation, even if a caller supplied this field unexpectedly.
	delete(reqBody, "previous_response_id")
	payload := s.buildOpenAIWSCreatePayload(reqBody, account)
	payload["store"] = false

	wsURL, err := s.buildOpenAIResponsesWSURL(account)
	if err != nil {
		return nil, openAIMessagesWSPrewriteFailover(fmt.Errorf("build websocket url: %w", err), http.StatusBadGateway, nil, nil)
	}
	decision := OpenAIWSProtocolDecision{
		Transport: OpenAIUpstreamTransportResponsesWebsocketV2,
		Reason:    "messages_account_enabled",
	}
	headers, _, err := s.buildOpenAIWSHeaders(ctx, c, account, token, decision, false, "", "", promptCacheKey)
	if err != nil {
		return nil, openAIMessagesWSPrewriteFailover(fmt.Errorf("build websocket headers: %w", err), http.StatusBadGateway, nil, nil)
	}
	headers, err = s.refreshOpenAIAgentIdentityHeaders(ctx, account, headers)
	if err != nil {
		return nil, openAIMessagesWSPrewriteFailover(fmt.Errorf("refresh websocket authentication headers: %w", err), http.StatusBadGateway, nil, nil)
	}

	proxyURL := ""
	if account != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	dialer := s.getOpenAIWSPassthroughDialer()
	if dialer == nil {
		return nil, openAIMessagesWSPrewriteFailover(errors.New("openai websocket dialer is nil"), http.StatusBadGateway, nil, nil)
	}
	dialCtx, cancelDial := context.WithTimeout(ctx, s.openAIWSDialTimeout())
	conn, statusCode, handshakeHeaders, dialErr := dialer.Dial(dialCtx, wsURL, headers, proxyURL)
	cancelDial()
	if dialErr != nil {
		var handshakeErr *openAIWSHandshakeError
		var responseBody []byte
		if errors.As(dialErr, &handshakeErr) && handshakeErr != nil {
			responseBody = append([]byte(nil), handshakeErr.Body...)
		}
		wrapped := &openAIWSDialError{
			StatusCode:      statusCode,
			ResponseHeaders: cloneHeader(handshakeHeaders),
			ResponseBody:    responseBody,
			Err:             dialErr,
		}
		s.handleOpenAIWSDialTransientFailure(ctx, account, canonicalOpenAIAccountSchedulingModel(account, originalModel), wrapped)
		if errors.Is(dialErr, context.Canceled) {
			return nil, dialErr
		}
		if statusCode == 0 {
			return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, wrapped, false)
		}
		return nil, openAIMessagesWSPrewriteFailover(wrapped, statusCode, handshakeHeaders, responseBody)
	}
	if conn == nil {
		return nil, openAIMessagesWSPrewriteFailover(errors.New("openai websocket dial returned a nil connection"), http.StatusBadGateway, handshakeHeaders, nil)
	}
	defer func() { _ = conn.Close() }()

	// A cancellation observed before WriteJSON is invoked means no upstream frame
	// can have been sent. Return it directly rather than turning it into failover.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// The upstream may have begun consuming this frame as soon as WriteJSON is
	// called. Never wrap this or any later error as UpstreamFailoverError.
	writeCtx, cancelWrite := context.WithTimeout(ctx, s.openAIWSWriteTimeout())
	writeErr := conn.WriteJSON(writeCtx, payload)
	cancelWrite()
	if writeErr != nil {
		return nil, fmt.Errorf("write openai messages websocket request: %v", writeErr)
	}

	pipeReader, pipeWriter := io.Pipe()
	outcomes := make(chan openAIMessagesWSReadOutcome, 1)
	drain := make(chan struct{})
	go s.readOpenAIMessagesSingleTurnWS(ctx, conn, pipeWriter, account, originalModel, handshakeHeaders, startTime, drain, outcomes)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     cloneHeader(handshakeHeaders),
		Body:       pipeReader,
	}
	var result *OpenAIForwardResult
	var handleErr error
	if clientStream {
		result, handleErr = s.handleAnthropicStreamingResponse(resp, c, account, originalModel, billingModel, upstreamModel, startTime)
	} else {
		result, handleErr = s.handleAnthropicBufferedStreamingResponse(resp, c, account, originalModel, billingModel, upstreamModel, startTime)
	}
	_ = pipeReader.Close()
	close(drain)
	outcome := <-outcomes

	if result == nil {
		result = &OpenAIForwardResult{
			Model:         originalModel,
			BillingModel:  billingModel,
			UpstreamModel: upstreamModel,
		}
	}
	if result.RequestID == "" {
		result.RequestID = strings.TrimSpace(handshakeHeaders.Get("x-request-id"))
	}
	if outcome.responseID != "" {
		result.ResponseID = outcome.responseID
	}
	result.Usage = outcome.usage
	result.Stream = clientStream
	if ctx.Err() != nil {
		result.ClientDisconnect = true
	}
	if clientStream {
		result.RequestType = RequestTypeStream
	} else {
		result.RequestType = RequestTypeSync
	}
	result.OpenAIWSMode = true
	result.UpstreamEndpoint = openAIMessagesResponsesUpstreamEndpoint
	result.UpstreamTerminalEvent = outcome.terminalEvent
	result.ResponseHeaders = cloneHeader(handshakeHeaders)
	result.Duration = time.Since(startTime)
	// The SSE converter treats its first input chunk as TTFT. Only the WS reader
	// knows whether that chunk contained a real token event, so always overwrite.
	result.FirstTokenMs = outcome.firstTokenMs

	if handleErr != nil && !(result.ClientDisconnect && outcome.terminalEvent != "") {
		var failoverErr *UpstreamFailoverError
		if errors.As(handleErr, &failoverErr) {
			return result, fmt.Errorf("openai messages websocket response failed after request write: %v", failoverErr)
		}
		return result, handleErr
	}
	if outcome.err != nil {
		return result, fmt.Errorf("read openai messages websocket response: %v", outcome.err)
	}
	if GetOpsCyberPolicy(c) != nil {
		return nil, errOpenAICyberPolicyForwarded
	}
	return result, nil
}

func (s *OpenAIGatewayService) readOpenAIMessagesSingleTurnWS(
	requestCtx context.Context,
	conn openAIWSClientConn,
	writer *io.PipeWriter,
	account *Account,
	originalModel string,
	handshakeHeaders http.Header,
	startTime time.Time,
	drain <-chan struct{},
	outcomes chan<- openAIMessagesWSReadOutcome,
) {
	outcome := openAIMessagesWSReadOutcome{}
	deliveryOpen := true
	requestDetached := false
	draining := false
	var drainCtx context.Context
	var cancelDrain context.CancelFunc
	normalReadCtx, cancelNormalReads := context.WithCancel(requestCtx)
	readerDone := make(chan struct{})
	go func() {
		select {
		case <-drain:
			cancelNormalReads()
		case <-readerDone:
		}
	}()
	defer func() {
		close(readerDone)
		cancelNormalReads()
		if cancelDrain != nil {
			cancelDrain()
		}
		_ = writer.CloseWithError(outcome.err)
		outcomes <- outcome
	}()

	startDrain := func() {
		if draining {
			return
		}
		draining = true
		deliveryOpen = false
		drainCtx, cancelDrain = context.WithTimeout(context.Background(), s.openAIMessagesWSDrainTimeout())
	}

	for {
		if !draining {
			select {
			case <-requestCtx.Done():
				requestDetached = true
				startDrain()
			case <-drain:
				startDrain()
			default:
			}
		}

		baseCtx := normalReadCtx
		if draining {
			baseCtx = drainCtx
		}
		readCtx, cancelRead := context.WithTimeout(baseCtx, s.openAIWSReadTimeout())
		message, err := conn.ReadMessage(readCtx)
		cancelRead()
		if err != nil {
			if !draining {
				select {
				case <-requestCtx.Done():
					requestDetached = true
					startDrain()
					continue
				case <-drain:
					startDrain()
					continue
				default:
				}
			}
			outcome.err = err
			return
		}

		documents := [][]byte{message}
		if repaired, ok := splitOpenAIConcatenatedJSONDocuments(message); ok {
			documents = repaired
		}
		for _, document := range documents {
			eventType, responseID, _ := parseOpenAIWSEventEnvelope(document)
			if outcome.responseID == "" && responseID != "" {
				outcome.responseID = responseID
			}
			if outcome.firstTokenMs == nil && isOpenAIWSTokenEvent(eventType) {
				ms := int(time.Since(startTime).Milliseconds())
				outcome.firstTokenMs = &ms
			}
			if openAIWSEventShouldParseUsage(eventType) {
				parseOpenAIWSResponseUsageFromCompletedEvent(document, &outcome.usage)
			}

			if deliveryOpen {
				if _, err := fmt.Fprintf(writer, "data: %s\n\n", document); err != nil {
					startDrain()
				}
			}
			if isOpenAIWSTerminalEvent(eventType) {
				failureCtx := requestCtx
				if requestDetached {
					failureCtx = context.Background()
				}
				outcome.terminalEvent = s.handleOpenAIWSTerminalTransientFailure(failureCtx, account, canonicalOpenAIAccountSchedulingModel(account, originalModel), handshakeHeaders, document)
				return
			}
			if eventType == "error" {
				failureCtx := requestCtx
				if requestDetached {
					failureCtx = context.Background()
				}
				s.handleOpenAIWSErrorEventTransientFailure(failureCtx, account, canonicalOpenAIAccountSchedulingModel(account, originalModel), handshakeHeaders, document)
				_, _, messageText := parseOpenAIWSErrorEventFields(document)
				if strings.TrimSpace(messageText) == "" {
					messageText = "upstream websocket error event"
				}
				outcome.err = errors.New(messageText)
				return
			}
		}
	}
}

func openAIMessagesWSPrewriteFailover(err error, statusCode int, headers http.Header, body []byte) error {
	if err == nil {
		return nil
	}
	var failoverErr *UpstreamFailoverError
	if errors.As(err, &failoverErr) {
		return err
	}
	if statusCode <= 0 {
		statusCode = http.StatusBadGateway
	}
	if len(body) == 0 {
		body, _ = json.Marshal(gin.H{"error": gin.H{"type": "upstream_error", "message": sanitizeUpstreamErrorMessage(err.Error())}})
	}
	return newOpenAIUpstreamFailoverError(statusCode, headers, body, err.Error(), false)
}
