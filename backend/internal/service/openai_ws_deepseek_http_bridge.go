package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

var ErrDeepSeekWSTurnCancelled = errors.New("DeepSeek Responses WebSocket turn cancelled")

var errDeepSeekWSSensitiveDelta = errors.New("DeepSeek Responses WebSocket upstream split a sensitive value across events")
var errDeepSeekWSNativeTerminal = errors.New("DeepSeek Responses WebSocket upstream returned a non-success terminal")
var errDeepSeekWSReplayLimit = errors.New("DeepSeek Responses WebSocket replay context limit exceeded")

const (
	deepSeekWSSensitiveHoldMaxEvents = 256
	deepSeekWSSensitiveHoldMaxBytes  = 1024 * 1024
	deepSeekWSReplayMaxItems         = 4096
	deepSeekWSReplayDefaultMaxBytes  = int64(256 * 1024 * 1024)
)

type deepSeekWSAccountNeutralError struct {
	err error
}

func (e *deepSeekWSAccountNeutralError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *deepSeekWSAccountNeutralError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

// NewDeepSeekWSAccountNeutralError marks a request-scoped DeepSeek WS failure.
func NewDeepSeekWSAccountNeutralError(err error) error {
	if err == nil {
		return nil
	}
	return &deepSeekWSAccountNeutralError{err: err}
}

// IsDeepSeekWSAccountNeutralError reports request-scoped upstream rejections
// that must not affect the selected account's scheduling health.
func IsDeepSeekWSAccountNeutralError(err error) bool {
	var neutralErr *deepSeekWSAccountNeutralError
	return errors.As(err, &neutralErr)
}

func newDeepSeekWSClientRequestError(err error) error {
	closeErr := NewOpenAIWSClientCloseError(
		coderws.StatusPolicyViolation,
		"DeepSeek Responses request was rejected",
		err,
	)
	return NewDeepSeekWSAccountNeutralError(closeErr)
}

func newDeepSeekWSClientWriteError(err error) error {
	if err == nil {
		return nil
	}
	var closeErr *OpenAIWSClientCloseError
	if errors.As(err, &closeErr) {
		return err
	}
	return NewOpenAIWSClientCloseError(
		coderws.StatusGoingAway,
		"DeepSeek Responses WebSocket client write failed",
		err,
	)
}

func isDeepSeekWSDeterministicClientStatus(statusCode int) bool {
	return statusCode == http.StatusBadRequest || statusCode == http.StatusUnprocessableEntity
}

func ensureDeepSeekWSUsageRequestID(c *gin.Context, result *OpenAIForwardResult, turn int) {
	if result == nil || strings.TrimSpace(result.RequestID) != "" {
		return
	}
	domain := "normal"
	if result.RequestKind.Normalize() == UsageRequestKindCompact {
		domain = "compact"
	}
	if GetOpsCyberPolicy(c) != nil {
		domain = "cyber"
	}
	if turn < 1 {
		turn = 1
	}
	identity := strings.TrimSpace(result.ResponseID)
	if identity == "" {
		identity = strings.ReplaceAll(uuid.NewString(), "-", "")
	}
	result.RequestID = fmt.Sprintf("deepseek-ws:%s:turn:%d:%s", domain, turn, identity)
}

// DeepSeekWSIngressHooks keeps connection framing and stateless replay in the
// service while allowing the handler to perform security review, scheduling,
// per-turn concurrency admission and billing at the correct lifecycle points.
type DeepSeekWSIngressHooks struct {
	ClientLifecycleContext context.Context
	BeforeRequest          func(turn int, payload []byte, originalModel string) ([]byte, error)
	ExecuteTurn            func(turnCtx context.Context, turn int, payload []byte, write func([]byte) error) (*OpenAIForwardResult, error)
	AfterTurn              func(turn int, result *OpenAIForwardResult, turnErr error)
}

type deepSeekWSClientFrame struct {
	messageType coderws.MessageType
	payload     []byte
	err         error
}

type deepSeekWSTurnWireState struct {
	mu            sync.Mutex
	responseID    string
	terminalEvent string
}

func (s *deepSeekWSTurnWireState) observe(message []byte) {
	eventType, responseID, _ := parseOpenAIWSEventEnvelope(message)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.responseID == "" {
		s.responseID = strings.TrimSpace(responseID)
	}
	if openAIStreamEventTypeIsTerminal(eventType) {
		s.terminalEvent = eventType
	}
}

func (s *deepSeekWSTurnWireState) snapshot() (string, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.responseID, s.terminalEvent
}

func parseDeepSeekWSCreateFrame(payload []byte, fallbackModel string) ([]byte, string, string, error) {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 || !gjson.ValidBytes(trimmed) {
		return nil, "", "", errors.New("invalid websocket request payload")
	}
	if err := ValidateDeepSeekUserIdentityRequest(trimmed, DeepSeekUserIdentityResponses); err != nil {
		return nil, "", "", err
	}
	if _, err := probeDeepSeekResponsesCompactionState(trimmed); err != nil {
		return nil, "", "", err
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &object); err != nil || object == nil {
		return nil, "", "", errors.New("response.create payload must be a JSON object")
	}
	eventType := strings.TrimSpace(gjson.GetBytes(trimmed, "type").String())
	if eventType == "" {
		eventType = "response.create"
		encodedType, _ := json.Marshal(eventType)
		object["type"] = encodedType
	}
	if eventType != "response.create" {
		return nil, "", "", fmt.Errorf("unsupported websocket request type: %s", eventType)
	}
	model := strings.TrimSpace(gjson.GetBytes(trimmed, "model").String())
	if model == "" {
		model = strings.TrimSpace(fallbackModel)
		if model == "" {
			return nil, "", "", errors.New("model is required in response.create payload")
		}
		encodedModel, _ := json.Marshal(model)
		object["model"] = encodedModel
	}
	normalized, err := json.Marshal(object)
	if err != nil {
		return nil, "", "", err
	}
	return normalized, model, strings.TrimSpace(gjson.GetBytes(trimmed, "previous_response_id").String()), nil
}

func deepSeekWSInputSequence(payload []byte) ([]json.RawMessage, bool, error) {
	input := gjson.GetBytes(payload, "input")
	if !input.Exists() {
		return nil, false, nil
	}
	switch {
	case input.IsArray():
		var items []json.RawMessage
		if err := json.Unmarshal([]byte(input.Raw), &items); err != nil {
			return nil, true, err
		}
		return items, true, nil
	case input.Type == gjson.JSON:
		trimmed := strings.TrimSpace(input.Raw)
		if !strings.HasPrefix(trimmed, "{") {
			return nil, true, errors.New("responses input must be a string, object, or array")
		}
		return []json.RawMessage{json.RawMessage(append([]byte(nil), trimmed...))}, true, nil
	case input.Type == gjson.String:
		message, err := json.Marshal(map[string]any{
			"type": "message",
			"role": "user",
			"content": []map[string]any{{
				"type": "input_text",
				"text": input.String(),
			}},
		})
		if err != nil {
			return nil, true, err
		}
		return []json.RawMessage{message}, true, nil
	default:
		return nil, true, errors.New("responses input must be a string, object, or array")
	}
}

func buildDeepSeekWSFullInput(previous []json.RawMessage, currentPayload []byte) ([]json.RawMessage, bool, error) {
	current, currentExists, err := deepSeekWSInputSequence(currentPayload)
	if err != nil {
		return nil, false, err
	}
	if len(previous) == 0 {
		return append([]json.RawMessage(nil), current...), currentExists, nil
	}
	if !currentExists || len(current) == 0 {
		return append([]json.RawMessage(nil), previous...), true, nil
	}
	if openAIWSRawItemsHasPrefix(current, previous) {
		return append([]json.RawMessage(nil), current...), true, nil
	}
	return combineOpenAIWSReplayItems(previous, current), true, nil
}

func validateDeepSeekWSReplayToolPairs(items []json.RawMessage) error {
	calls := make(map[string]struct{})
	outputs := make(map[string]struct{})
	for _, item := range items {
		itemType := strings.TrimSpace(gjson.GetBytes(item, "type").String())
		callID := strings.TrimSpace(gjson.GetBytes(item, "call_id").String())
		switch {
		case isCodexToolCallContextItemType(itemType):
			if callID == "" {
				return errors.New("DeepSeek WebSocket replay contains a tool call without call_id")
			}
			if _, exists := calls[callID]; exists {
				return errors.New("DeepSeek WebSocket replay contains duplicate tool call ids")
			}
			calls[callID] = struct{}{}
		case isCodexToolCallOutputItemType(itemType):
			if callID == "" {
				return errors.New("DeepSeek WebSocket replay contains a tool result without call_id")
			}
			if _, exists := calls[callID]; !exists {
				return errors.New("DeepSeek WebSocket replay contains an unpaired tool result")
			}
			if _, exists := outputs[callID]; exists {
				return errors.New("DeepSeek WebSocket replay contains duplicate tool results")
			}
			outputs[callID] = struct{}{}
		}
	}
	return nil
}

func prepareDeepSeekWSHTTPBridgeBody(payload []byte) ([]byte, error) {
	if err := ValidateDeepSeekUserIdentityRequest(payload, DeepSeekUserIdentityResponses); err != nil {
		return nil, err
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(payload, &body); err != nil || body == nil {
		return nil, errors.New("response.create payload must be a JSON object")
	}
	delete(body, "type")
	delete(body, "generate")
	delete(body, "previous_response_id")
	for _, field := range openAIResponsesOptionalFields {
		delete(body, field)
	}
	body["stream"] = json.RawMessage("true")
	if strings.TrimSpace(gjson.GetBytes(payload, "model").String()) == "" {
		return nil, errors.New("model is required in response.create payload")
	}
	return json.Marshal(body)
}

func buildDeepSeekWSCancelledEvent(responseID string) []byte {
	responseID = strings.TrimSpace(responseID)
	if responseID == "" {
		responseID = "resp_cancelled_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	}
	body, _ := json.Marshal(map[string]any{
		"type": "response.cancelled",
		"response": map[string]any{
			"id":     responseID,
			"object": "response",
			"status": "cancelled",
		},
	})
	return body
}

func deepSeekWSProtocolError(message string) []byte {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "invalid websocket request"
	}
	body, _ := json.Marshal(map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    "invalid_request_error",
			"message": message,
		},
	})
	return body
}

func classifyDeepSeekWSControlFrame(payload []byte) (string, string, error) {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 || !gjson.ValidBytes(trimmed) {
		return "", "", errors.New("invalid websocket request payload")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &object); err != nil || object == nil {
		return "", "", errors.New("websocket payload must be a JSON object")
	}
	eventType := strings.TrimSpace(gjson.GetBytes(trimmed, "type").String())
	if eventType == "" {
		eventType = "response.create"
	}
	return eventType, strings.TrimSpace(gjson.GetBytes(trimmed, "response_id").String()), nil
}

// ProxyDeepSeekResponsesWebSocket owns client framing and stateless context
// replay. It deliberately owns no account lease; ExecuteTurn acquires and
// releases all per-turn resources so an idle socket consumes no model capacity.
func (s *OpenAIGatewayService) ProxyDeepSeekResponsesWebSocket(
	ctx context.Context,
	c *gin.Context,
	clientConn *coderws.Conn,
	firstClientMessage []byte,
	hooks *DeepSeekWSIngressHooks,
) (returnErr error) {
	if s == nil {
		return errors.New("service is nil")
	}
	if c == nil {
		return errors.New("gin context is nil")
	}
	if clientConn == nil {
		return errors.New("client websocket is nil")
	}
	if hooks == nil || hooks.ExecuteTurn == nil {
		return errors.New("DeepSeek WebSocket turn executor is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	lifecycleCtx := ctx
	if hooks.ClientLifecycleContext != nil {
		lifecycleCtx = hooks.ClientLifecycleContext
	}
	sessionCtx, cancelSession := context.WithCancelCause(ctx)
	defer func() {
		var closeErr *OpenAIWSClientCloseError
		if errors.As(returnErr, &closeErr) {
			_ = clientConn.Close(closeErr.StatusCode(), closeErr.Reason())
		}
		cancelSession(context.Canceled)
	}()
	stopLifecycle := context.AfterFunc(lifecycleCtx, func() { cancelSession(context.Cause(lifecycleCtx)) })
	defer stopLifecycle()

	frames := make(chan deepSeekWSClientFrame, 2)
	go func() {
		for {
			messageType, payload, err := clientConn.Read(sessionCtx)
			select {
			case frames <- deepSeekWSClientFrame{messageType: messageType, payload: payload, err: err}:
			case <-sessionCtx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()

	var writeMu sync.Mutex
	writeClient := func(payload []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		writeCtx, cancel := context.WithTimeout(lifecycleCtx, s.openAIWSWriteTimeout())
		defer cancel()
		return clientConn.Write(writeCtx, coderws.MessageText, payload)
	}
	closeWithProtocolError := func(message string, cause error) error {
		_ = writeClient(deepSeekWSProtocolError(message))
		return NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, message, cause)
	}

	current := append([]byte(nil), firstClientMessage...)
	model := ""
	lastResponseID := ""
	ledger := []json.RawMessage(nil)
	var pending *deepSeekWSClientFrame

	for turn := 1; ; turn++ {
		payload, originalModel, previousResponseID, err := parseDeepSeekWSCreateFrame(current, model)
		if err != nil {
			return closeWithProtocolError(err.Error(), err)
		}
		model = originalModel
		if previousResponseID != "" {
			if lastResponseID == "" {
				return closeWithProtocolError("previous_response_id cannot be recovered on this DeepSeek WebSocket connection", nil)
			}
			if previousResponseID != lastResponseID {
				return closeWithProtocolError("previous_response_id does not match the previous DeepSeek WebSocket turn", nil)
			}
		}
		replayInput := ledger
		if previousResponseID == "" {
			replayInput = nil
		}
		fullInput, fullInputExists, err := buildDeepSeekWSFullInput(replayInput, payload)
		if err != nil {
			return closeWithProtocolError("unable to recover DeepSeek WebSocket context", err)
		}
		payload, err = setOpenAIWSPayloadInputSequence(payload, fullInput, fullInputExists)
		if err != nil {
			return closeWithProtocolError("unable to encode DeepSeek WebSocket context", err)
		}
		if s.deepSeekCompactRequestTooLarge(payload) {
			_ = writeClient(deepSeekWSProtocolError("DeepSeek WebSocket request exceeds gateway max_body_size"))
			return NewOpenAIWSClientCloseError(coderws.StatusMessageTooBig, "DeepSeek WebSocket request exceeds gateway max_body_size", ErrDeepSeekCompactRequestTooLarge)
		}
		if hooks.BeforeRequest != nil {
			payload, err = hooks.BeforeRequest(turn, payload, originalModel)
			if err != nil {
				return err
			}
		}
		if s.deepSeekCompactRequestTooLarge(payload) {
			_ = writeClient(deepSeekWSProtocolError("DeepSeek WebSocket request exceeds gateway max_body_size"))
			return NewOpenAIWSClientCloseError(coderws.StatusMessageTooBig, "DeepSeek WebSocket request exceeds gateway max_body_size", ErrDeepSeekCompactRequestTooLarge)
		}
		payload, err = prepareDeepSeekWSHTTPBridgeBody(payload)
		if err != nil {
			return closeWithProtocolError(err.Error(), err)
		}
		turnInput, turnInputExists, err := deepSeekWSInputSequence(payload)
		if err != nil || !turnInputExists {
			if err == nil {
				err = errors.New("stateless DeepSeek WebSocket turn is missing input")
			}
			return closeWithProtocolError("unable to recover DeepSeek WebSocket context", err)
		}
		if err := validateDeepSeekWSReplayToolPairs(turnInput); err != nil {
			return closeWithProtocolError(err.Error(), err)
		}

		turnCtx, cancelTurn := context.WithCancelCause(sessionCtx)
		wireState := &deepSeekWSTurnWireState{}
		turnWrite := func(message []byte) error {
			wireState.observe(message)
			return writeClient(message)
		}
		type turnOutcome struct {
			result *OpenAIForwardResult
			err    error
		}
		outcomes := make(chan turnOutcome, 1)
		go func() {
			result, executeErr := hooks.ExecuteTurn(turnCtx, turn, payload, turnWrite)
			outcomes <- turnOutcome{result: result, err: executeErr}
		}()

		cancelRequested := false
		cancelResponseID := ""
		var outcome turnOutcome
	turnWait:
		for {
			select {
			case outcome = <-outcomes:
				break turnWait
			case frame := <-frames:
				if frame.err != nil {
					cancelTurn(frame.err)
					outcome = <-outcomes
					if hooks.AfterTurn != nil {
						hooks.AfterTurn(turn, outcome.result, outcome.err)
					}
					cancelTurn(context.Canceled)
					if isOpenAIWSClientDisconnectError(frame.err) {
						return nil
					}
					return frame.err
				}
				if frame.messageType != coderws.MessageText && frame.messageType != coderws.MessageBinary {
					cancelTurn(errors.New("unsupported websocket client message type"))
					outcome = <-outcomes
					if hooks.AfterTurn != nil {
						hooks.AfterTurn(turn, outcome.result, outcome.err)
					}
					return closeWithProtocolError("unsupported websocket client message type", nil)
				}
				eventType, responseID, frameErr := classifyDeepSeekWSControlFrame(frame.payload)
				if frameErr != nil {
					cancelTurn(frameErr)
					outcome = <-outcomes
					if hooks.AfterTurn != nil {
						hooks.AfterTurn(turn, outcome.result, outcome.err)
					}
					return closeWithProtocolError(frameErr.Error(), frameErr)
				}
				switch eventType {
				case "response.cancel":
					activeID, terminalEvent := wireState.snapshot()
					if terminalEvent != "" {
						cancelTurn(errors.New("response.cancel arrived after terminal event"))
						outcome = <-outcomes
						if hooks.AfterTurn != nil {
							hooks.AfterTurn(turn, outcome.result, outcome.err)
						}
						return closeWithProtocolError("response.cancel arrived after the active turn completed", nil)
					}
					if cancelRequested {
						cancelTurn(errors.New("duplicate response.cancel"))
						outcome = <-outcomes
						if hooks.AfterTurn != nil {
							hooks.AfterTurn(turn, outcome.result, outcome.err)
						}
						return closeWithProtocolError("duplicate response.cancel", nil)
					}
					if responseID != "" && activeID != "" && responseID != activeID {
						cancelTurn(errors.New("response.cancel response_id mismatch"))
						outcome = <-outcomes
						if hooks.AfterTurn != nil {
							hooks.AfterTurn(turn, outcome.result, outcome.err)
						}
						return closeWithProtocolError("response.cancel does not match the active turn", nil)
					}
					cancelRequested = true
					cancelResponseID = responseID
					cancelTurn(ErrDeepSeekWSTurnCancelled)
				case "response.create":
					_, terminalEvent := wireState.snapshot()
					if terminalEvent == "" {
						cancelTurn(errors.New("overlapping response.create"))
						outcome = <-outcomes
						if hooks.AfterTurn != nil {
							hooks.AfterTurn(turn, outcome.result, outcome.err)
						}
						return closeWithProtocolError("overlapping response.create is not supported", nil)
					}
					if pending != nil {
						cancelTurn(errors.New("multiple queued response.create frames"))
						outcome = <-outcomes
						if hooks.AfterTurn != nil {
							hooks.AfterTurn(turn, outcome.result, outcome.err)
						}
						return closeWithProtocolError("only one response.create may be queued", nil)
					}
					frameCopy := frame
					pending = &frameCopy
				default:
					cancelTurn(fmt.Errorf("unsupported websocket request type: %s", eventType))
					outcome = <-outcomes
					if hooks.AfterTurn != nil {
						hooks.AfterTurn(turn, outcome.result, outcome.err)
					}
					return closeWithProtocolError("unsupported websocket request type: "+eventType, nil)
				}
			case <-sessionCtx.Done():
				cancelTurn(context.Cause(sessionCtx))
				outcome = <-outcomes
				if hooks.AfterTurn != nil {
					hooks.AfterTurn(turn, outcome.result, outcome.err)
				}
				return context.Cause(sessionCtx)
			}
		}
		cancelTurn(context.Canceled)

		responseID, terminalEvent := wireState.snapshot()
		var outputItems []json.RawMessage
		if outcome.result != nil {
			if responseID == "" {
				responseID = strings.TrimSpace(outcome.result.ResponseID)
			}
			if terminalEvent == "" {
				terminalEvent = strings.TrimSpace(outcome.result.UpstreamTerminalEvent)
			}
			if outcome.result.wsReplayInputExists {
				if limitErr := s.validateDeepSeekWSReplayInput(outcome.result.wsReplayInput); limitErr != nil {
					outcome.err = newDeepSeekWSReplayLimitCloseError(limitErr)
				} else {
					outputItems = outcome.result.wsReplayInput
				}
			}
		}
		if cancelRequested {
			if terminalEvent == "" {
				if responseID == "" {
					responseID = cancelResponseID
				}
				if err := writeClient(buildDeepSeekWSCancelledEvent(responseID)); err != nil {
					if hooks.AfterTurn != nil {
						hooks.AfterTurn(turn, outcome.result, err)
					}
					return err
				}
				terminalEvent = "response.cancelled"
			}
			outcome.err = ErrDeepSeekWSTurnCancelled
		}
		if hooks.AfterTurn != nil {
			hooks.AfterTurn(turn, outcome.result, outcome.err)
		}
		if outcome.err != nil && !cancelRequested {
			if !errors.Is(outcome.err, errDeepSeekWSNativeTerminal) {
				return outcome.err
			}
			// Native non-success terminals complete the turn without making it
			// replayable on another account. The connection itself remains valid.
		}
		ledger = append([]json.RawMessage(nil), turnInput...)
		if outcome.result != nil && outcome.result.deepSeekWSCompaction {
			ledger = nil
		}
		if len(outputItems) > 0 && terminalEvent != "response.cancelled" && terminalEvent != "response.canceled" {
			ledger = append(ledger, outputItems...)
		}
		if responseID != "" {
			lastResponseID = responseID
		}

		if pending != nil {
			current = append(current[:0], pending.payload...)
			pending = nil
			continue
		}
		idleTimeout := s.openAIWSIngressInterTurnIdleTimeout()
		var idleTimer *time.Timer
		var idle <-chan time.Time
		if idleTimeout > 0 {
			idleTimer = time.NewTimer(idleTimeout)
			idle = idleTimer.C
		}
		select {
		case frame := <-frames:
			if idleTimer != nil {
				idleTimer.Stop()
			}
			if frame.err != nil {
				if isOpenAIWSClientDisconnectError(frame.err) {
					return nil
				}
				return frame.err
			}
			if frame.messageType != coderws.MessageText && frame.messageType != coderws.MessageBinary {
				return closeWithProtocolError("unsupported websocket client message type", nil)
			}
			eventType, _, frameErr := classifyDeepSeekWSControlFrame(frame.payload)
			if frameErr != nil {
				return closeWithProtocolError(frameErr.Error(), frameErr)
			}
			if eventType != "response.create" {
				return closeWithProtocolError("unsupported websocket request type: "+eventType, nil)
			}
			current = append(current[:0], frame.payload...)
		case <-idle:
			return NewOpenAIWSClientCloseError(coderws.StatusNormalClosure, "websocket inter-turn idle timeout", context.DeadlineExceeded)
		case <-sessionCtx.Done():
			if idleTimer != nil {
				idleTimer.Stop()
			}
			return context.Cause(sessionCtx)
		}
	}
}

func (s *OpenAIGatewayService) buildDeepSeekWSHTTPBridgeRequest(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	token string,
	body []byte,
) (*http.Request, []byte, error) {
	if account == nil || !account.IsDeepSeekAPIKey() {
		return nil, nil, errors.New("DeepSeek WebSocket bridge requires a DeepSeek API key account")
	}
	if strings.TrimSpace(token) == "" {
		return nil, nil, fmt.Errorf("account %d missing api_key", account.ID)
	}
	prepared, err := prepareDeepSeekWSHTTPBridgeBody(body)
	if err != nil {
		return nil, nil, err
	}
	prepared = normalizeDeepSeekResponsesRequestBody(account, prepared)
	prepared, err = applyDeepSeekAuthenticatedUserID(ctx, s.cfg, account, DeepSeekUserIdentityResponses, prepared)
	if err != nil {
		return nil, nil, err
	}
	targetURL, err := s.deepSeekEndpointURL(account, deepSeekResponsesEndpoint)
	if err != nil {
		return nil, nil, err
	}
	requestCtx := WithHTTPUpstreamProfile(withDeepSeekRedirectsDisabled(ctx, account), HTTPUpstreamProfileOpenAI)
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, targetURL, bytes.NewReader(prepared))
	if err != nil {
		return nil, nil, fmt.Errorf("build DeepSeek WebSocket HTTP bridge request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)
	if c != nil && c.Request != nil {
		for key, values := range c.Request.Header {
			if !openaiCCRawAllowedHeaders[strings.ToLower(key)] {
				continue
			}
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}
	}
	account.ApplyHeaderOverrides(req.Header)
	req.Header.Del("Authorization")
	req.Header.Del("X-Api-Key")
	req.Header.Del("X-Goog-Api-Key")
	req.Header.Set("Authorization", "Bearer "+token)
	return req, prepared, nil
}

type deepSeekWSSSEState struct {
	responseID       string
	terminalEvent    string
	usage            OpenAIUsage
	firstTokenMs     *int
	wroteEvent       bool
	replay           deepSeekWSReplayCollector
	upstreamModel    string
	modelConflict    bool
	terminalData     []byte
	eventAfterFinish bool
	securityBlocked  bool
}

type deepSeekWSReplayCollector struct {
	items    []json.RawMessage
	bytes    int64
	maxBytes int64
}

func (s *OpenAIGatewayService) deepSeekWSReplayMaxBytes() int64 {
	if s != nil && s.cfg != nil && s.cfg.Gateway.MaxBodySize > 0 {
		return s.cfg.Gateway.MaxBodySize
	}
	return deepSeekWSReplayDefaultMaxBytes
}

func newDeepSeekWSReplayLimitCloseError(cause error) error {
	return NewOpenAIWSClientCloseError(
		coderws.StatusMessageTooBig,
		"DeepSeek WebSocket replay context limit exceeded",
		cause,
	)
}

func (s *OpenAIGatewayService) validateDeepSeekWSReplayInput(items []json.RawMessage) error {
	if len(items) > deepSeekWSReplayMaxItems {
		return fmt.Errorf("%w: more than %d output items", errDeepSeekWSReplayLimit, deepSeekWSReplayMaxItems)
	}
	maxBytes := s.deepSeekWSReplayMaxBytes()
	var total int64
	for _, item := range items {
		itemBytes := int64(len(item))
		if itemBytes > maxBytes-total {
			return fmt.Errorf("%w: output items exceed gateway max_body_size (%d bytes)", errDeepSeekWSReplayLimit, maxBytes)
		}
		total += itemBytes
	}
	return nil
}

func (c *deepSeekWSReplayCollector) add(item gjson.Result) error {
	if !item.Exists() || item.Type != gjson.JSON {
		return nil
	}
	if len(c.items) >= deepSeekWSReplayMaxItems {
		return fmt.Errorf("%w: more than %d output items", errDeepSeekWSReplayLimit, deepSeekWSReplayMaxItems)
	}
	itemBytes := int64(len(item.Raw))
	if itemBytes > c.maxBytes-c.bytes {
		return fmt.Errorf("%w: output items exceed gateway max_body_size (%d bytes)", errDeepSeekWSReplayLimit, c.maxBytes)
	}
	c.items = append(c.items, json.RawMessage(append([]byte(nil), item.Raw...)))
	c.bytes += itemBytes
	return nil
}

func (c *deepSeekWSReplayCollector) replace(output gjson.Result) error {
	if !output.IsArray() {
		return nil
	}
	maxBytes := c.maxBytes
	*c = deepSeekWSReplayCollector{maxBytes: maxBytes}
	var collectErr error
	output.ForEach(func(_, item gjson.Result) bool {
		collectErr = c.add(item)
		return collectErr == nil
	})
	if collectErr != nil {
		*c = deepSeekWSReplayCollector{maxBytes: maxBytes}
		return collectErr
	}
	return nil
}

func (c *deepSeekWSReplayCollector) observe(eventType string, message []byte) error {
	switch eventType {
	case "response.output_item.done":
		return c.add(gjson.GetBytes(message, "item"))
	case "response.completed", "response.done", "response.incomplete", "response.failed":
		return c.replace(gjson.GetBytes(message, "response.output"))
	case "response.cancelled", "response.canceled":
		c.items = nil
		c.bytes = 0
	}
	return nil
}

type deepSeekWSSensitiveDeltaGuard struct {
	secrets        []string
	pending        [][]byte
	pendingBytes   int
	prefix         string
	prefixStream   string
	streamPrefixes map[string]string
}

func (s *OpenAIGatewayService) newDeepSeekWSSensitiveDeltaGuard(ctx context.Context, account *Account) *deepSeekWSSensitiveDeltaGuard {
	guard := &deepSeekWSSensitiveDeltaGuard{}
	if account == nil {
		return guard
	}
	if apiKey := account.GetDeepSeekAPIKey(); apiKey != "" {
		guard.secrets = append(guard.secrets, apiKey)
	}
	if account.ResolveDeepSeekUserIsolationMode() == DeepSeekUserIsolationModeAuthenticatedUser {
		if derivedID, err := DeriveDeepSeekAuthenticatedUserID(ctx, s.cfg); err == nil && derivedID != "" {
			guard.secrets = append(guard.secrets, derivedID)
		}
	}
	return guard
}

func (g *deepSeekWSSensitiveDeltaGuard) longestSecretPrefixSuffix(value string) string {
	longest := ""
	for _, secret := range g.secrets {
		maxPrefix := min(len(value), len(secret)-1)
		for size := maxPrefix; size > len(longest); size-- {
			if strings.HasSuffix(value, secret[:size]) {
				longest = secret[:size]
				break
			}
		}
	}
	if derivedPrefix := deepSeekWSLongestDerivedIDPrefixSuffix(value); len(derivedPrefix) > len(longest) {
		longest = derivedPrefix
	}
	return longest
}

func deepSeekWSDerivedIDPrefixPossible(value string) bool {
	if value == "" {
		return false
	}
	if len(value) <= len(deepSeekUserIDPrefix) {
		return strings.HasPrefix(deepSeekUserIDPrefix, value)
	}
	if !strings.HasPrefix(value, deepSeekUserIDPrefix) || len(value) >= len(deepSeekUserIDPrefix)+deepSeekUserIDEncodedDigestBytes {
		return false
	}
	for _, char := range value[len(deepSeekUserIDPrefix):] {
		if !isDeepSeekDerivedUserIDByte(char) {
			return false
		}
	}
	return true
}

func deepSeekWSLongestDerivedIDPrefixSuffix(value string) string {
	maxLength := len(deepSeekUserIDPrefix) + deepSeekUserIDEncodedDigestBytes - 1
	if len(value) > maxLength {
		value = value[len(value)-maxLength:]
	}
	for start := 0; start < len(value); start++ {
		candidate := value[start:]
		if deepSeekWSDerivedIDPrefixPossible(candidate) {
			return candidate
		}
	}
	return ""
}

func deepSeekWSSensitiveStreamKey(eventType string, message []byte) string {
	return deepSeekResponsesSSESensitiveStreamKey(eventType, message)
}

func (g *deepSeekWSSensitiveDeltaGuard) advancePrefix(prefix, delta string) (string, error) {
	candidate := delta
	if prefix != "" {
		combined := prefix + delta
		candidate = combined
		if redactDeepSeekDerivedUserIDsString(combined) != combined {
			return "", errDeepSeekWSSensitiveDelta
		}
		for _, secret := range g.secrets {
			if strings.Contains(combined, secret) {
				return "", errDeepSeekWSSensitiveDelta
			}
		}
		for _, secret := range g.secrets {
			if strings.HasPrefix(secret, combined) {
				return combined, nil
			}
		}
		if deepSeekWSDerivedIDPrefixPossible(combined) {
			return combined, nil
		}
	}
	return g.longestSecretPrefixSuffix(candidate), nil
}

func (g *deepSeekWSSensitiveDeltaGuard) hasPendingPrefix() bool {
	return g.prefix != "" || len(g.streamPrefixes) > 0
}

func (g *deepSeekWSSensitiveDeltaGuard) appendPending(message []byte) error {
	if len(g.pending) >= deepSeekWSSensitiveHoldMaxEvents || g.pendingBytes+len(message) > deepSeekWSSensitiveHoldMaxBytes {
		return fmt.Errorf("%w: pending event limit exceeded", errDeepSeekWSSensitiveDelta)
	}
	g.pending = append(g.pending, append([]byte(nil), message...))
	g.pendingBytes += len(message)
	return nil
}

func (g *deepSeekWSSensitiveDeltaGuard) flush(write func([]byte) error) error {
	for _, message := range g.pending {
		if err := write(message); err != nil {
			return err
		}
	}
	g.pending = nil
	g.pendingBytes = 0
	g.prefix = ""
	g.prefixStream = ""
	g.streamPrefixes = nil
	return nil
}

func (g *deepSeekWSSensitiveDeltaGuard) writeOrHold(message []byte, eventType string, write func([]byte) error) error {
	delta := gjson.GetBytes(message, "delta")
	isDelta := strings.HasSuffix(eventType, ".delta") && delta.Type == gjson.String
	streamKey := deepSeekWSSensitiveStreamKey(eventType, message)

	if len(g.pending) == 0 {
		if !isDelta {
			return write(message)
		}
		prefix, err := g.advancePrefix("", delta.String())
		if err != nil {
			return err
		}
		if prefix == "" {
			return write(message)
		}
		if err := g.appendPending(message); err != nil {
			return err
		}
		g.prefix = prefix
		g.prefixStream = streamKey
		g.streamPrefixes = map[string]string{streamKey: prefix}
		return nil
	}

	if isDelta {
		globalPrefix, err := g.advancePrefix(g.prefix, delta.String())
		if err != nil {
			return err
		}
		streamPrefix, err := g.advancePrefix(g.streamPrefixes[streamKey], delta.String())
		if err != nil {
			return err
		}
		g.prefix = globalPrefix
		if globalPrefix == "" {
			g.prefixStream = ""
		} else {
			g.prefixStream = streamKey
		}
		if streamPrefix == "" {
			delete(g.streamPrefixes, streamKey)
		} else {
			if g.streamPrefixes == nil {
				g.streamPrefixes = make(map[string]string)
			}
			g.streamPrefixes[streamKey] = streamPrefix
		}
		if err := g.appendPending(message); err != nil {
			return err
		}
		if !g.hasPendingPrefix() {
			return g.flush(write)
		}
		return nil
	}

	if err := g.appendPending(message); err != nil {
		return err
	}
	if strings.HasSuffix(eventType, ".done") {
		delete(g.streamPrefixes, streamKey)
		if g.prefixStream == streamKey {
			g.prefix = ""
			g.prefixStream = ""
		}
	}
	if openAIStreamEventTypeIsTerminal(eventType) {
		return g.flush(write)
	}
	if !g.hasPendingPrefix() {
		return g.flush(write)
	}
	return nil
}

func deepSeekWSTerminalStatusMatches(eventType, status string) bool {
	status = strings.ToLower(strings.TrimSpace(status))
	switch eventType {
	case "response.completed", "response.done":
		return status == "completed"
	case "response.incomplete":
		return status == "incomplete"
	case "response.failed":
		return status == "failed"
	case "response.cancelled", "response.canceled":
		return status == "cancelled" || status == "canceled"
	default:
		return false
	}
}

func (s *OpenAIGatewayService) readDeepSeekWSResponsesSSE(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	resp *http.Response,
	startTime time.Time,
	write func([]byte) error,
) (deepSeekWSSSEState, error) {
	state := deepSeekWSSSEState{
		replay: deepSeekWSReplayCollector{maxBytes: s.deepSeekWSReplayMaxBytes()},
	}
	var protocolErr error
	var cyberMark *CyberPolicyMark
	defer func() {
		if cyberMark != nil {
			cyberMark.UpstreamInTok = state.usage.InputTokens
			cyberMark.UpstreamOutTok = state.usage.OutputTokens
			MarkOpsCyberPolicy(c, *cyberMark)
		}
	}()
	setProtocolErr := func(err error) {
		if protocolErr == nil {
			protocolErr = err
		}
	}
	sensitiveGuard := s.newDeepSeekWSSensitiveDeltaGuard(ctx, account)
	emit := func(message []byte) error {
		state.wroteEvent = true
		if err := write(message); err != nil {
			return newDeepSeekWSClientWriteError(err)
		}
		return nil
	}
	requestID := openAICompatibleUpstreamRequestID(resp.Header)
	observer := &upstreamResponseModelObserver{}
	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	scanBuf := getSSEScannerBuf64K()
	scanner.Buffer(scanBuf[:0], maxLineSize)
	defer putSSEScannerBuf64K(scanBuf)

	currentEvent := ""
	dataLines := make([]string, 0, 1)
	dataBytes := 0
	processEvent := func() error {
		if len(dataLines) == 0 {
			currentEvent = ""
			dataBytes = 0
			return nil
		}
		data := strings.TrimSpace(strings.Join(dataLines, "\n"))
		dataLines = dataLines[:0]
		defer func() {
			currentEvent = ""
			dataBytes = 0
		}()
		if data == "" {
			return nil
		}
		if data == "[DONE]" {
			return errors.New("DeepSeek Responses WebSocket bridge received an invalid [DONE] sentinel")
		}
		message := redactDeepSeekAPIKey(account, []byte(data))
		if !gjson.ValidBytes(message) {
			return errors.New("DeepSeek Responses WebSocket bridge received malformed SSE JSON")
		}
		eventType := strings.TrimSpace(gjson.GetBytes(message, "type").String())
		if eventType == "" {
			return errors.New("DeepSeek Responses WebSocket bridge received an event without a JSON type")
		}
		if declaredType := strings.TrimSpace(currentEvent); declaredType != "" && declaredType != eventType {
			return errors.New("DeepSeek Responses WebSocket bridge received mismatched SSE event and JSON types")
		}
		if state.terminalEvent != "" {
			state.eventAfterFinish = true
			return errors.New("DeepSeek Responses WebSocket bridge received data after the terminal event")
		}
		if hit, code, cyberMessage := detectOpenAICyberPolicy(message); hit {
			cyberMark = &CyberPolicyMark{
				Code: code, Message: cyberMessage, Body: truncateString(string(message), 4096), UpstreamStatus: http.StatusOK,
			}
		}
		errorPayload := gjson.GetBytes(message, "error")
		responseError := gjson.GetBytes(message, "response.error")
		nativeNonSuccessTerminal := eventType == "response.failed" || eventType == "response.incomplete" ||
			eventType == "response.cancelled" || eventType == "response.canceled"
		if eventType == "error" ||
			(!nativeNonSuccessTerminal && ((errorPayload.Exists() && errorPayload.Type != gjson.Null) ||
				(responseError.Exists() && responseError.Type != gjson.Null))) {
			setProtocolErr(errors.New("DeepSeek Responses WebSocket bridge received an upstream error event"))
		}
		observer.ObserveOpenAI(message, eventType)
		if responseID := extractOpenAIResponseIDFromJSONBytes(message); responseID != "" {
			if state.responseID == "" {
				state.responseID = responseID
			} else if state.responseID != responseID {
				setProtocolErr(errors.New("DeepSeek Responses WebSocket bridge received conflicting response ids"))
			}
		}
		if state.firstTokenMs == nil && openAIStreamDataStartsVisibleOutput(string(message), eventType) {
			ms := int(time.Since(startTime).Milliseconds())
			state.firstTokenMs = &ms
		}
		if openAIWSEventShouldParseUsage(eventType) {
			parseOpenAIWSResponseUsageFromCompletedEvent(message, &state.usage)
		}
		if eventType != "error" && openAIStreamEventTypeIsTerminal(eventType) {
			state.terminalEvent = eventType
			state.terminalData = append([]byte(nil), message...)
			if !deepSeekWSTerminalStatusMatches(eventType, gjson.GetBytes(message, "response.status").String()) {
				setProtocolErr(errors.New("DeepSeek Responses WebSocket bridge terminal type and status do not match"))
			}
		}
		if err := state.replay.observe(eventType, message); err != nil {
			return newDeepSeekWSReplayLimitCloseError(err)
		}
		if err := sensitiveGuard.writeOrHold(message, eventType, emit); err != nil {
			if errors.Is(err, errDeepSeekWSSensitiveDelta) {
				state.securityBlocked = true
			}
			return err
		}
		return nil
	}

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			if err := processEvent(); err != nil {
				return state, err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "event:") {
			currentEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if data, ok := extractOpenAISSEDataLine(line); ok {
			dataBytes += len(data)
			if dataBytes > maxLineSize {
				return state, errors.New("DeepSeek Responses WebSocket bridge SSE event exceeds max_line_size")
			}
			dataLines = append(dataLines, data)
		}
	}
	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return state, context.Cause(ctx)
		}
		return state, fmt.Errorf("read DeepSeek Responses WebSocket bridge stream: %w", err)
	}
	if err := processEvent(); err != nil {
		return state, err
	}
	if err := sensitiveGuard.flush(emit); err != nil {
		return state, err
	}
	state.upstreamModel = observer.Model()
	state.modelConflict = observer.Conflict()
	if state.terminalEvent == "" {
		if protocolErr != nil {
			return state, protocolErr
		}
		return state, errors.New("DeepSeek Responses WebSocket bridge ended before a terminal event")
	}
	if deepSeekResponsesRequiresUsage(state.terminalEvent) && !hasBillableOpenAIUsage(state.usage) {
		return state, newDeepSeekMissingUsageFailoverError(c, account, requestID)
	}
	if protocolErr != nil {
		return state, protocolErr
	}
	if state.terminalEvent == "response.failed" || state.terminalEvent == "response.incomplete" ||
		state.terminalEvent == "response.cancelled" || state.terminalEvent == "response.canceled" {
		return state, fmt.Errorf("%w: %s", errDeepSeekWSNativeTerminal, state.terminalEvent)
	}
	return state, nil
}

func buildDeepSeekWSCompactEvents(finalJSON []byte) ([][]byte, []json.RawMessage, error) {
	if !gjson.ValidBytes(finalJSON) || !gjson.ParseBytes(finalJSON).IsObject() {
		return nil, nil, errors.New("invalid synthesized DeepSeek compact response")
	}
	output := gjson.GetBytes(finalJSON, "output")
	if !output.IsArray() || len(output.Array()) != 1 {
		return nil, nil, errors.New("DeepSeek compact response must contain exactly one output item")
	}
	item := output.Array()[0]
	if item.Type != gjson.JSON || strings.TrimSpace(item.Get("type").String()) != "compaction" || strings.TrimSpace(item.Get("encrypted_content").String()) == "" {
		return nil, nil, errors.New("DeepSeek compact response contains an invalid compaction item")
	}
	if strings.TrimSpace(gjson.GetBytes(finalJSON, "id").String()) == "" || strings.TrimSpace(gjson.GetBytes(finalJSON, "status").String()) != "completed" {
		return nil, nil, errors.New("DeepSeek compact response is missing its completed response state")
	}
	itemRaw := json.RawMessage(append([]byte(nil), item.Raw...))
	itemEvent, err := json.Marshal(map[string]any{
		"type":         "response.output_item.done",
		"output_index": 0,
		"item":         itemRaw,
	})
	if err != nil {
		return nil, nil, err
	}
	responseRaw := json.RawMessage(append([]byte(nil), finalJSON...))
	terminalEvent, err := json.Marshal(map[string]any{
		"type":     "response.completed",
		"response": responseRaw,
	})
	if err != nil {
		return nil, nil, err
	}
	return [][]byte{itemEvent, terminalEvent}, []json.RawMessage{itemRaw}, nil
}

func (s *OpenAIGatewayService) recordDeepSeekWSHTTPError(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	upstreamModel string,
	err *deepSeekCompactUpstreamHTTPError,
) bool {
	if err == nil {
		return false
	}
	message := redactDeepSeekAPIKeyString(account, err.Message)
	setOpsUpstreamError(c, err.StatusCode, message, "")
	cyberBlocked := false
	if hit, code, cyberMessage := detectOpenAICyberPolicy(err.Body); hit {
		cyberBlocked = true
		MarkOpsCyberPolicy(c, CyberPolicyMark{
			Code: code, Message: cyberMessage, Body: truncateString(string(err.Body), 4096), UpstreamStatus: err.StatusCode,
		})
	} else if !isDeepSeekWSDeterministicClientStatus(err.StatusCode) {
		s.handleOpenAIAccountUpstreamError(ctx, account, err.StatusCode, err.Headers, err.Body, upstreamModel)
	}
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:           account.Platform,
		AccountID:          account.ID,
		AccountName:        account.Name,
		UpstreamStatusCode: err.StatusCode,
		UpstreamRequestID:  openAICompatibleUpstreamRequestID(err.Headers),
		Kind:               "http_error",
		Message:            message,
	})
	return cyberBlocked
}

func (s *OpenAIGatewayService) forwardDeepSeekResponsesWebSocketCompactionTurn(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	token string,
	payload []byte,
	write func([]byte) error,
) (*OpenAIForwardResult, error) {
	originalModel := strings.TrimSpace(gjson.GetBytes(payload, "model").String())
	if originalModel == "" {
		return nil, errors.New("DeepSeek WebSocket compaction requires a model")
	}
	billingModel := resolveOpenAIForwardModel(account, originalModel, "")
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)
	execution, err := s.executeDeepSeekRemoteCompaction(
		ctx,
		c,
		account,
		payload,
		originalModel,
		billingModel,
		upstreamModel,
		true,
		func(responsesBody []byte) (*http.Response, error) {
			req, _, buildErr := s.buildDeepSeekWSHTTPBridgeRequest(ctx, c, account, token, responsesBody)
			if buildErr != nil {
				return nil, buildErr
			}
			req.Header.Set("X-DeepSeek-Harness-Compact", "1")
			proxyURL := ""
			if account.Proxy != nil {
				proxyURL = account.Proxy.URL()
			}
			resp, requestErr := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
			if requestErr != nil {
				return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, requestErr, false)
			}
			return resp, nil
		},
	)
	if err != nil {
		var upstreamHTTPError *deepSeekCompactUpstreamHTTPError
		if !errors.As(err, &upstreamHTTPError) {
			return execution.Result, err
		}
		cyberBlocked := s.recordDeepSeekWSHTTPError(ctx, c, account, upstreamModel, upstreamHTTPError)
		message := strings.TrimSpace(upstreamHTTPError.Message)
		if message == "" {
			message = http.StatusText(upstreamHTTPError.StatusCode)
		}
		result := execution.Result
		if result == nil && cyberBlocked {
			result = &OpenAIForwardResult{
				RequestID:        openAICompatibleUpstreamRequestID(upstreamHTTPError.Headers),
				Model:            originalModel,
				BillingModel:     billingModel,
				UpstreamModel:    upstreamModel,
				UpstreamEndpoint: deepSeekResponsesEndpoint,
				ReasoningEffort:  optionalTrimmedStringPtr(gjson.GetBytes(upstreamHTTPError.RequestBody, "reasoning.effort").String()),
				RequestKind:      UsageRequestKindCompact,
				Stream:           true,
				OpenAIWSMode:     true,
				ResponseHeaders:  upstreamHTTPError.Headers.Clone(),
			}
		}
		if writeErr := write(buildOpenAIWSHTTPBridgeErrorEvent(upstreamHTTPError.StatusCode, message)); writeErr != nil {
			return result, newDeepSeekWSClientWriteError(writeErr)
		}
		if cyberBlocked || isDeepSeekWSDeterministicClientStatus(upstreamHTTPError.StatusCode) {
			return result, newDeepSeekWSClientRequestError(upstreamHTTPError)
		}
		return result, upstreamHTTPError
	}
	if execution.Result == nil {
		return nil, errors.New("DeepSeek WebSocket compaction result is nil")
	}
	events, replayItems, err := buildDeepSeekWSCompactEvents(execution.FinalJSON)
	if err != nil {
		return execution.Result, err
	}
	execution.Result.OpenAIWSMode = true
	execution.Result.Stream = true
	execution.Result.RequestKind = UsageRequestKindCompact
	execution.Result.UpstreamEndpoint = deepSeekResponsesEndpoint
	execution.Result.UpstreamTerminalEvent = "response.completed"
	for _, event := range events {
		if err := write(event); err != nil {
			return execution.Result, newDeepSeekWSClientWriteError(err)
		}
	}
	execution.Result.wsReplayInput = replayItems
	execution.Result.wsReplayInputExists = true
	execution.Result.deepSeekWSCompaction = true
	return execution.Result, nil
}

// ForwardDeepSeekResponsesWebSocketTurn performs one native DeepSeek HTTP
// /responses request and relays each SSE data JSON document as one WS message.
// It never detaches ctx, so response.cancel and client disconnect abort the
// actual HTTP request.
func (s *OpenAIGatewayService) ForwardDeepSeekResponsesWebSocketTurn(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	token string,
	payload []byte,
	turn int,
	write func([]byte) error,
) (*OpenAIForwardResult, error) {
	if s == nil || s.httpUpstream == nil {
		return nil, errors.New("DeepSeek WebSocket HTTP upstream is unavailable")
	}
	if write == nil {
		return nil, errors.New("DeepSeek WebSocket client writer is required")
	}
	if account == nil || !account.IsDeepSeekAPIKey() {
		return nil, errors.New("DeepSeek WebSocket bridge requires a DeepSeek API key account")
	}
	if account.ResolveDeepSeekResponsesWebSocketMode(DeepSeekResponsesWSHTTPBridgeEnabled(s.cfg)) != DeepSeekResponsesWebSocketModeHTTPBridge {
		return nil, NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "DeepSeek Responses WebSocket mode is disabled for this account", nil)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if s.deepSeekCompactRequestTooLarge(payload) {
		return nil, ErrDeepSeekCompactRequestTooLarge
	}
	if IsDeepSeekCompactionMarked(c) && HasCompactionTriggerInInput(payload) {
		result, err := s.forwardDeepSeekResponsesWebSocketCompactionTurn(ctx, c, account, token, payload, write)
		ensureDeepSeekWSUsageRequestID(c, result, turn)
		if err != nil && GetOpsCyberPolicy(c) != nil && !IsDeepSeekWSAccountNeutralError(err) {
			err = NewDeepSeekWSAccountNeutralError(err)
		}
		return result, err
	}
	startTime := time.Now()
	originalModel := strings.TrimSpace(gjson.GetBytes(payload, "model").String())
	if originalModel == "" {
		return nil, errors.New("DeepSeek WebSocket request requires a model")
	}
	billingModel := resolveOpenAIForwardModel(account, originalModel, "")
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)
	upstreamPayload := payload
	if upstreamModel != originalModel {
		upstreamPayload = ReplaceModelInBody(payload, upstreamModel)
	}
	req, body, err := s.buildDeepSeekWSHTTPBridgeRequest(ctx, c, account, token, upstreamPayload)
	if err != nil {
		return nil, err
	}
	SetActualOpenAIUpstreamEndpoint(c, deepSeekResponsesEndpoint)
	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	defer func() { _ = resp.Body.Close() }()
	sanitizeDeepSeekResponseHeadersInPlace(account, resp.Header)
	if resp.StatusCode >= http.StatusBadRequest {
		respBody, upstreamMsg := s.readOpenAIUpstreamError(resp)
		respBody = redactDeepSeekAPIKey(account, respBody)
		upstreamMsg = redactDeepSeekAPIKeyString(account, upstreamMsg)
		if hit, _, _ := detectOpenAICyberPolicy(respBody); hit {
			result := &OpenAIForwardResult{
				RequestID:        openAICompatibleUpstreamRequestID(resp.Header),
				Model:            originalModel,
				BillingModel:     billingModel,
				UpstreamModel:    upstreamModel,
				UpstreamEndpoint: deepSeekResponsesEndpoint,
				ServiceTier:      extractOpenAIServiceTierFromBody(body),
				ReasoningEffort:  optionalTrimmedStringPtr(gjson.GetBytes(body, "reasoning.effort").String()),
				Stream:           true,
				OpenAIWSMode:     true,
				ResponseHeaders:  resp.Header.Clone(),
				Duration:         time.Since(startTime),
			}
			s.recordDeepSeekWSHTTPError(ctx, c, account, upstreamModel, &deepSeekCompactUpstreamHTTPError{
				StatusCode: resp.StatusCode,
				Headers:    resp.Header.Clone(),
				Body:       respBody,
				Message:    upstreamMsg,
			})
			if writeErr := write(buildOpenAIWSHTTPBridgeErrorEvent(resp.StatusCode, upstreamMsg)); writeErr != nil {
				return result, newDeepSeekWSClientWriteError(writeErr)
			}
			ensureDeepSeekWSUsageRequestID(c, result, turn)
			return result, newDeepSeekWSClientRequestError(fmt.Errorf("DeepSeek Responses WebSocket upstream HTTP %d: %s", resp.StatusCode, upstreamMsg))
		}
		if failoverErr := s.failoverOpenAIUpstreamHTTPError(ctx, c, account, resp, respBody, upstreamMsg, upstreamModel); failoverErr != nil {
			return nil, failoverErr
		}
		if writeErr := write(buildOpenAIWSHTTPBridgeErrorEvent(resp.StatusCode, upstreamMsg)); writeErr != nil {
			return nil, newDeepSeekWSClientWriteError(writeErr)
		}
		upstreamErr := fmt.Errorf("DeepSeek Responses WebSocket upstream HTTP %d: %s", resp.StatusCode, upstreamMsg)
		if isDeepSeekWSDeterministicClientStatus(resp.StatusCode) {
			return nil, newDeepSeekWSClientRequestError(upstreamErr)
		}
		return nil, upstreamErr
	}

	state, streamErr := s.readDeepSeekWSResponsesSSE(ctx, c, account, resp, startTime, write)
	result := &OpenAIForwardResult{
		RequestID:                     openAICompatibleUpstreamRequestID(resp.Header),
		ResponseID:                    state.responseID,
		Usage:                         state.usage,
		Model:                         originalModel,
		BillingModel:                  billingModel,
		UpstreamModel:                 upstreamModel,
		UpstreamResponseModel:         state.upstreamModel,
		UpstreamResponseModelConflict: state.modelConflict,
		UpstreamEndpoint:              deepSeekResponsesEndpoint,
		ServiceTier:                   extractOpenAIServiceTierFromBody(body),
		ReasoningEffort:               optionalTrimmedStringPtr(gjson.GetBytes(body, "reasoning.effort").String()),
		Stream:                        true,
		OpenAIWSMode:                  true,
		UpstreamTerminalEvent:         state.terminalEvent,
		ResponseHeaders:               resp.Header.Clone(),
		Duration:                      time.Since(startTime),
		FirstTokenMs:                  state.firstTokenMs,
	}
	ensureDeepSeekWSUsageRequestID(c, result, turn)
	if len(state.replay.items) > 0 {
		result.wsReplayInput = state.replay.items
		result.wsReplayInputExists = true
	}
	if streamErr == nil {
		return result, nil
	}
	if GetOpsCyberPolicy(c) != nil {
		return result, NewDeepSeekWSAccountNeutralError(streamErr)
	}
	if state.securityBlocked {
		return result, NewDeepSeekWSAccountNeutralError(streamErr)
	}
	if errors.Is(streamErr, context.Canceled) || errors.Is(streamErr, ErrDeepSeekWSTurnCancelled) {
		return result, streamErr
	}
	if result.HasBillableTokenUsage() || state.wroteEvent || state.securityBlocked {
		// A failover-typed integrity error is only retryable before any client
		// semantic event and before billable usage. Strip that type once either
		// boundary has been crossed so the handler cannot replay this turn.
		var clientCloseErr *OpenAIWSClientCloseError
		if errors.As(streamErr, &clientCloseErr) {
			return result, streamErr
		}
		if errors.Is(streamErr, errDeepSeekWSSensitiveDelta) {
			return result, streamErr
		}
		if errors.Is(streamErr, errDeepSeekWSNativeTerminal) {
			return result, streamErr
		}
		return result, errors.New(streamErr.Error())
	}
	var failoverErr *UpstreamFailoverError
	if errors.As(streamErr, &failoverErr) {
		return nil, failoverErr
	}
	return nil, s.newOpenAIStreamFailoverError(
		c,
		account,
		false,
		openAICompatibleUpstreamRequestID(resp.Header),
		state.terminalData,
		streamErr.Error(),
		resp.Header,
	)
}
