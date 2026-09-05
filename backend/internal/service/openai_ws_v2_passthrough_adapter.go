package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	openaiwsv2 "github.com/Wei-Shaw/sub2api/internal/service/openai_ws_v2"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

type openAIWSClientFrameConn struct {
	conn                 *coderws.Conn
	controlCtx           context.Context
	interTurnIdleTimeout time.Duration
	interTurnStarted     chan struct{}
	waitingForNextTurn   atomic.Bool
	// The relay observes upstream payloads, while clients must keep seeing the
	// model identifier they supplied for the current turn.
	restoreResponseModel func([]byte) []byte
	restoreToolNames     func([]byte) []byte
}

// openAIWSPolicyEnforcingFrameConn wraps a client-side FrameConn and runs
// every client→upstream frame through the OpenAI Fast Policy. It is the
// passthrough-relay equivalent of the parseClientPayload integration in the
// ingress session path. filter returns:
//   - newPayload, nil, nil: forward the (possibly mutated) payload
//   - _, *OpenAIFastBlockedError, nil: block — the wrapper sends an error
//     event via onBlock and surfaces a transport-level error so the relay
//     stops reading from the client.
//   - _, _, err: a transport error other than block.
type openAIWSPolicyEnforcingFrameConn struct {
	inner   openaiwsv2.FrameConn
	filter  func(msgType coderws.MessageType, payload []byte) ([]byte, *OpenAIFastBlockedError, error)
	onBlock func(blocked *OpenAIFastBlockedError)
}

var _ openaiwsv2.FrameConn = (*openAIWSPolicyEnforcingFrameConn)(nil)

func (c *openAIWSPolicyEnforcingFrameConn) ReadFrame(ctx context.Context) (coderws.MessageType, []byte, error) {
	if c == nil || c.inner == nil {
		return coderws.MessageText, nil, errOpenAIWSConnClosed
	}
	msgType, payload, err := c.inner.ReadFrame(ctx)
	if err != nil {
		return msgType, payload, err
	}
	if c.filter == nil {
		return msgType, payload, nil
	}
	updated, blocked, filterErr := c.filter(msgType, payload)
	if filterErr != nil {
		return msgType, payload, filterErr
	}
	if blocked != nil {
		if c.onBlock != nil {
			c.onBlock(blocked)
		}
		return msgType, nil, NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, blocked.Message, blocked)
	}
	return msgType, updated, nil
}

func (c *openAIWSPolicyEnforcingFrameConn) WriteFrame(ctx context.Context, msgType coderws.MessageType, payload []byte) error {
	if c == nil || c.inner == nil {
		return errOpenAIWSConnClosed
	}
	return c.inner.WriteFrame(ctx, msgType, payload)
}

func (c *openAIWSPolicyEnforcingFrameConn) Close() error {
	if c == nil || c.inner == nil {
		return nil
	}
	return c.inner.Close()
}

// openAIWSPassthroughPolicyModelForFrame returns the model actually forwarded
// by a passthrough WS frame. Passthrough only replaces authentication, so an
// account's ordinary model_mapping and Codex aliases must not rewrite it.
func openAIWSPassthroughPolicyModelForFrame(account *Account, payload []byte) string {
	if account == nil || len(payload) == 0 {
		return ""
	}
	original := strings.TrimSpace(gjson.GetBytes(payload, "model").String())
	if original == "" {
		return ""
	}
	return original
}

// openAIWSPassthroughPolicyModelFromSessionFrame returns the upstream model
// derived from a session.update frame's session.model field. Returns "" when
// the frame is not a session.update event or carries no session.model. Used
// by the per-frame policy filter (client→upstream direction) to keep
// capturedSessionModel in sync with the session-level model the client may
// rotate mid-session.
//
// Realtime / Responses WS lets the client change the session model after
// the WS handshake via:
//
//	{"type":"session.update","session":{"model":"gpt-5.5", ...}}
//
// If we only capture the model from the very first frame, a client can ship
// gpt-4o on the first response.create (whitelisted as pass), then
// session.update to gpt-5.5, then send response.create without "model" so
// the per-frame resolver returns "" and the stale capturedSessionModel falls
// back to gpt-4o — defeating the gpt-5.5 fast-policy filter.
func openAIWSPassthroughPolicyModelFromSessionFrame(account *Account, payload []byte) string {
	if account == nil || len(payload) == 0 {
		return ""
	}
	frameType := strings.TrimSpace(gjson.GetBytes(payload, "type").String())
	if frameType != "session.update" {
		return ""
	}
	original := strings.TrimSpace(gjson.GetBytes(payload, "session.model").String())
	if original == "" {
		return ""
	}
	return original
}

type openAIWSPassthroughUsageMeta struct {
	serviceTier              atomic.Pointer[string]
	reasoningEffort          atomic.Pointer[string]
	requestedReasoningEffort atomic.Pointer[string]
	requestModel             atomic.Pointer[string]
	upstreamModel            atomic.Pointer[string]

	// 仅在 client->upstream filter goroutine 中读写；Load 侧通过上方原子指针同步。
	sessionRequestModel string
}

func newOpenAIWSPassthroughUsageMeta(initialRequestModel string, firstFrame []byte) *openAIWSPassthroughUsageMeta {
	meta := &openAIWSPassthroughUsageMeta{
		sessionRequestModel: strings.TrimSpace(initialRequestModel),
	}
	if meta.sessionRequestModel == "" {
		meta.sessionRequestModel = openAIWSPassthroughRequestModelForFrame(firstFrame)
	}
	return meta
}

func (m *openAIWSPassthroughUsageMeta) initFromFirstFrame(policyOutput []byte, mappedModel string) {
	if m == nil {
		return
	}
	m.serviceTier.Store(extractOpenAIServiceTierFromBody(policyOutput))
	m.reasoningEffort.Store(extractOpenAIReasoningEffortFromBody(policyOutput, mappedModel, m.sessionRequestModel))
	m.storeTurnModels(m.sessionRequestModel, policyOutput)
}

func (m *openAIWSPassthroughUsageMeta) captureRequestedReasoningEffort(originalBody []byte, modelCandidates ...string) {
	if m == nil {
		return
	}
	candidates := append([]string{m.sessionRequestModel}, modelCandidates...)
	m.requestedReasoningEffort.Store(CanonicalRequestedReasoningEffort(originalBody, candidates...))
}

func (m *openAIWSPassthroughUsageMeta) updateSessionRequestModel(payload []byte) {
	if m == nil {
		return
	}
	if model := openAIWSPassthroughRequestModelFromSessionFrame(payload); model != "" {
		m.sessionRequestModel = model
	}
}

func (m *openAIWSPassthroughUsageMeta) requestModelForFrame(payload []byte) string {
	if m == nil {
		return openAIWSPassthroughRequestModelForFrame(payload)
	}
	if model := openAIWSPassthroughRequestModelForFrame(payload); model != "" {
		return model
	}
	return m.sessionRequestModel
}

func (m *openAIWSPassthroughUsageMeta) updateFromResponseCreate(policyOutput []byte, mappedModel string, requestModelForFrame string) {
	if m == nil {
		return
	}
	m.serviceTier.Store(extractOpenAIServiceTierFromBody(policyOutput))
	m.reasoningEffort.Store(extractOpenAIReasoningEffortFromBody(policyOutput, mappedModel, requestModelForFrame))
	m.storeTurnModels(requestModelForFrame, policyOutput)
}

func (m *openAIWSPassthroughUsageMeta) storeTurnModels(requestModel string, upstreamPayload []byte) {
	if m == nil {
		return
	}
	requestModel = strings.TrimSpace(requestModel)
	upstreamModel := strings.TrimSpace(gjson.GetBytes(upstreamPayload, "model").String())
	if upstreamModel == "" {
		upstreamModel = requestModel
	}
	m.requestModel.Store(openAIWSTrimmedStringPtr(requestModel))
	m.upstreamModel.Store(openAIWSTrimmedStringPtr(upstreamModel))
}

func (m *openAIWSPassthroughUsageMeta) turnModels(fallback string) (string, string) {
	requestModel := strings.TrimSpace(fallback)
	upstreamModel := requestModel
	if m == nil {
		return requestModel, upstreamModel
	}
	if current := m.requestModel.Load(); current != nil && strings.TrimSpace(*current) != "" {
		requestModel = strings.TrimSpace(*current)
	}
	if current := m.upstreamModel.Load(); current != nil && strings.TrimSpace(*current) != "" {
		upstreamModel = strings.TrimSpace(*current)
	}
	return requestModel, upstreamModel
}

func openAIWSTrimmedStringPtr(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func openAIWSDifferentModel(requestModel, upstreamModel string) string {
	upstreamModel = strings.TrimSpace(upstreamModel)
	if upstreamModel == "" || upstreamModel == strings.TrimSpace(requestModel) {
		return ""
	}
	return upstreamModel
}

func openAIWSPassthroughRequestModelForFrame(payload []byte) string {
	if len(payload) == 0 || strings.TrimSpace(gjson.GetBytes(payload, "type").String()) != "response.create" {
		return ""
	}
	return strings.TrimSpace(gjson.GetBytes(payload, "model").String())
}

func openAIWSPassthroughRequestModelFromSessionFrame(payload []byte) string {
	if len(payload) == 0 || strings.TrimSpace(gjson.GetBytes(payload, "type").String()) != "session.update" {
		return ""
	}
	return strings.TrimSpace(gjson.GetBytes(payload, "session.model").String())
}

const openaiWSV2PassthroughModeFields = "ws_mode=passthrough ws_router=v2"

var errOpenAIWSPassthroughFirstOutputTimeout = errors.New("openai websocket passthrough first output timeout")
var errOpenAIWSPassthroughActiveTurnTimeout = errors.New("openai websocket passthrough active turn read timeout")

type openAIWSPassthroughDeadlinePhase uint8

const (
	openAIWSPassthroughDeadlinePhaseFirstSemantic openAIWSPassthroughDeadlinePhase = iota + 1
	openAIWSPassthroughDeadlinePhaseActiveRead
)

type openAIWSPassthroughFirstOutputDeadline struct {
	timeout         time.Duration
	startedAt       time.Time
	requestModel    string
	reasoningEffort string
	phase           openAIWSPassthroughDeadlinePhase
}

type openAIWSPassthroughFirstOutputTimeoutError struct {
	deadline openAIWSPassthroughFirstOutputDeadline
}

func (e *openAIWSPassthroughFirstOutputTimeoutError) Error() string {
	return errOpenAIWSPassthroughFirstOutputTimeout.Error()
}

func (e *openAIWSPassthroughFirstOutputTimeoutError) Unwrap() error {
	return errOpenAIWSPassthroughFirstOutputTimeout
}

type openAIWSPassthroughActiveTurnTimeoutError struct{}

func (e *openAIWSPassthroughActiveTurnTimeoutError) Error() string {
	return errOpenAIWSPassthroughActiveTurnTimeout.Error()
}

func (e *openAIWSPassthroughActiveTurnTimeoutError) Unwrap() error {
	return errOpenAIWSPassthroughActiveTurnTimeout
}

type openAIWSPassthroughFirstOutputDeadlineState struct {
	armed      bool
	generation uint64
	deadline   openAIWSPassthroughFirstOutputDeadline
}

type openAIWSPassthroughTurnLifecycle struct {
	mu       sync.Mutex
	inFlight bool
}

func newOpenAIWSPassthroughTurnLifecycle(inFlight bool) *openAIWSPassthroughTurnLifecycle {
	return &openAIWSPassthroughTurnLifecycle{inFlight: inFlight}
}

func (l *openAIWSPassthroughTurnLifecycle) beginResponseCreate(onAccepted func()) bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inFlight {
		return false
	}
	l.inFlight = true
	if onAccepted != nil {
		onAccepted()
	}
	return true
}

func (l *openAIWSPassthroughTurnLifecycle) cancelResponseCreate() {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.inFlight = false
	l.mu.Unlock()
}

func (l *openAIWSPassthroughTurnLifecycle) beginTerminalWrite() {
	if l != nil {
		l.mu.Lock()
	}
}

func (l *openAIWSPassthroughTurnLifecycle) finishTerminalWrite(succeeded bool, onSucceeded func()) {
	if l == nil {
		return
	}
	if succeeded {
		if onSucceeded != nil {
			onSucceeded()
		}
		l.inFlight = false
	}
	l.mu.Unlock()
}

type openAIWSPassthroughFirstOutputFrameConn struct {
	inner             openaiwsv2.FrameConn
	resolveDeadline   func(payload []byte) openAIWSPassthroughFirstOutputDeadline
	activeReadTimeout time.Duration

	mu              sync.Mutex
	state           openAIWSPassthroughFirstOutputDeadlineState
	deadlineChanged chan struct{}
}

func (c *openAIWSPassthroughFirstOutputFrameConn) ReadFrame(ctx context.Context) (coderws.MessageType, []byte, error) {
	if c == nil || c.inner == nil {
		return coderws.MessageText, nil, errOpenAIWSConnClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}

	type readResult struct {
		msgType coderws.MessageType
		payload []byte
		err     error
	}
	readCtx, cancelRead := context.WithCancel(ctx)
	readResultCh := make(chan readResult, 1)
	go func() {
		msgType, payload, err := c.inner.ReadFrame(readCtx)
		readResultCh <- readResult{msgType: msgType, payload: payload, err: err}
	}()

	var timer *time.Timer
	var timerCh <-chan time.Time
	resetTimer := func() {
		state := c.deadlineState()
		if timer != nil {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		}
		if !state.armed || state.deadline.timeout <= 0 {
			timerCh = nil
			return
		}
		remaining := time.Until(state.deadline.startedAt.Add(state.deadline.timeout))
		if remaining < 0 {
			remaining = 0
		}
		if timer == nil {
			timer = time.NewTimer(remaining)
		} else {
			timer.Reset(remaining)
		}
		timerCh = timer.C
	}
	resetTimer()

	defer func() {
		cancelRead()
		if timer != nil {
			timer.Stop()
		}
	}()
	for {
		select {
		case result := <-readResultCh:
			if result.err == nil {
				c.observeUpstreamActivity(result.msgType, result.payload)
			}
			return result.msgType, result.payload, result.err
		case <-c.deadlineChanged:
			resetTimer()
		case <-timerCh:
			state := c.deadlineState()
			if !state.armed || state.deadline.timeout <= 0 || time.Now().Before(state.deadline.startedAt.Add(state.deadline.timeout)) {
				resetTimer()
				continue
			}
			if ctx.Err() != nil {
				cancelRead()
				<-readResultCh
				return coderws.MessageText, nil, ctx.Err()
			}
			cancelRead()
			<-readResultCh
			if state.deadline.phase == openAIWSPassthroughDeadlinePhaseActiveRead {
				return coderws.MessageText, nil, &openAIWSPassthroughActiveTurnTimeoutError{}
			}
			return coderws.MessageText, nil, &openAIWSPassthroughFirstOutputTimeoutError{deadline: state.deadline}
		case <-ctx.Done():
			cancelRead()
			<-readResultCh
			return coderws.MessageText, nil, ctx.Err()
		}
	}
}

func (c *openAIWSPassthroughFirstOutputFrameConn) WriteFrame(ctx context.Context, msgType coderws.MessageType, payload []byte) error {
	if c == nil || c.inner == nil {
		return errOpenAIWSConnClosed
	}
	generation := uint64(0)
	if msgType == coderws.MessageText && strings.TrimSpace(gjson.GetBytes(payload, "type").String()) == "response.create" {
		generation = c.armDeadline(payload)
	}
	if err := c.inner.WriteFrame(ctx, msgType, payload); err != nil {
		c.disarmDeadline(generation)
		return err
	}
	return nil
}

func (c *openAIWSPassthroughFirstOutputFrameConn) Close() error {
	if c == nil || c.inner == nil {
		return nil
	}
	return c.inner.Close()
}

func (c *openAIWSPassthroughFirstOutputFrameConn) armDeadline(payload []byte) uint64 {
	if c == nil || c.resolveDeadline == nil {
		return 0
	}
	deadline := c.resolveDeadline(payload)
	if deadline.timeout <= 0 {
		return 0
	}
	if deadline.startedAt.IsZero() {
		deadline.startedAt = time.Now()
	}
	deadline.phase = openAIWSPassthroughDeadlinePhaseFirstSemantic
	c.mu.Lock()
	c.state.generation++
	generation := c.state.generation
	c.state.armed = true
	c.state.deadline = deadline
	c.mu.Unlock()
	c.notifyDeadlineChanged()
	return generation
}

func (c *openAIWSPassthroughFirstOutputFrameConn) observeUpstreamActivity(msgType coderws.MessageType, payload []byte) {
	if c == nil {
		return
	}
	if msgType == coderws.MessageText && openAIWSPassthroughIsTerminalOutput(payload) {
		c.disarmDeadline(0)
		return
	}
	state := c.deadlineState()
	if state.armed && state.deadline.phase == openAIWSPassthroughDeadlinePhaseActiveRead {
		c.armActiveReadDeadline()
		return
	}
	if msgType == coderws.MessageText && openAIWSPassthroughStartsSemanticOutput(payload) {
		c.armActiveReadDeadline()
	}
}

func (c *openAIWSPassthroughFirstOutputFrameConn) armActiveReadDeadline() {
	if c == nil {
		return
	}
	if c.activeReadTimeout <= 0 {
		c.disarmDeadline(0)
		return
	}
	c.mu.Lock()
	c.state.generation++
	c.state.armed = true
	c.state.deadline = openAIWSPassthroughFirstOutputDeadline{
		timeout:   c.activeReadTimeout,
		startedAt: time.Now(),
		phase:     openAIWSPassthroughDeadlinePhaseActiveRead,
	}
	c.mu.Unlock()
	c.notifyDeadlineChanged()
}

func (c *openAIWSPassthroughFirstOutputFrameConn) disarmDeadline(generation uint64) {
	if c == nil {
		return
	}
	c.mu.Lock()
	if !c.state.armed || (generation != 0 && generation != c.state.generation) {
		c.mu.Unlock()
		return
	}
	c.state.armed = false
	c.mu.Unlock()
	c.notifyDeadlineChanged()
}

func (c *openAIWSPassthroughFirstOutputFrameConn) deadlineState() openAIWSPassthroughFirstOutputDeadlineState {
	if c == nil {
		return openAIWSPassthroughFirstOutputDeadlineState{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

func (c *openAIWSPassthroughFirstOutputFrameConn) notifyDeadlineChanged() {
	if c == nil || c.deadlineChanged == nil {
		return
	}
	select {
	case c.deadlineChanged <- struct{}{}:
	default:
	}
}

func openAIWSPassthroughStartsSemanticOutput(payload []byte) bool {
	eventType := strings.TrimSpace(gjson.GetBytes(payload, "type").String())
	switch eventType {
	case "response.completed", "response.done", "response.failed", "response.incomplete", "response.cancelled", "response.canceled":
		return true
	case "", "response.created", "response.in_progress", "response.output_item.added", "response.output_item.done":
		return false
	}
	return strings.Contains(eventType, ".delta") ||
		strings.HasPrefix(eventType, "response.output_text") ||
		strings.HasPrefix(eventType, "response.output")
}

func openAIWSPassthroughIsTerminalOutput(payload []byte) bool {
	switch strings.TrimSpace(gjson.GetBytes(payload, "type").String()) {
	case "response.completed", "response.done", "response.failed", "response.incomplete", "response.cancelled", "response.canceled":
		return true
	default:
		return false
	}
}

var _ openaiwsv2.FrameConn = (*openAIWSClientFrameConn)(nil)
var _ openaiwsv2.FrameConn = (*openAIWSPassthroughFirstOutputFrameConn)(nil)

func (c *openAIWSClientFrameConn) ReadFrame(ctx context.Context) (coderws.MessageType, []byte, error) {
	if c == nil || c.conn == nil {
		return coderws.MessageText, nil, errOpenAIWSConnClosed
	}
	controlCtx := ctx
	if c.controlCtx != nil {
		controlCtx = c.controlCtx
	}
	msgType, payload, err := readOpenAIWSClientMessageWithTimeoutStart(
		controlCtx,
		c.conn,
		c.interTurnIdleTimeout,
		coderws.StatusNormalClosure,
		"websocket idle timeout",
		c.interTurnStarted,
		func() bool { return c.waitingForNextTurn.Load() },
	)
	return msgType, payload, err
}

func (c *openAIWSClientFrameConn) markTurnStarted() {
	if c != nil {
		c.waitingForNextTurn.Store(false)
	}
}

func (c *openAIWSClientFrameConn) markTurnCompleted() {
	if c == nil {
		return
	}
	c.waitingForNextTurn.Store(true)
	select {
	case c.interTurnStarted <- struct{}{}:
	default:
	}
}

func (c *openAIWSClientFrameConn) WriteFrame(ctx context.Context, msgType coderws.MessageType, payload []byte) error {
	if c == nil || c.conn == nil {
		return errOpenAIWSConnClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if msgType == coderws.MessageText {
		if normalized, changed := normalizeCompletedImageGenerationStatus(payload); changed {
			payload = normalized
		}
		if c.restoreResponseModel != nil {
			payload = c.restoreResponseModel(payload)
		}
		if c.restoreToolNames != nil {
			payload = c.restoreToolNames(payload)
		}
	}
	return c.conn.Write(ctx, msgType, payload)
}

func (c *openAIWSClientFrameConn) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	_ = c.conn.Close(coderws.StatusNormalClosure, "")
	_ = c.conn.CloseNow()
	return nil
}

func (s *OpenAIGatewayService) proxyResponsesWebSocketV2Passthrough(
	ctx context.Context,
	c *gin.Context,
	clientConn *coderws.Conn,
	account *Account,
	token string,
	firstClientMessage []byte,
	hooks *OpenAIWSIngressHooks,
	wsDecision OpenAIWSProtocolDecision,
) error {
	// The same request context survives scheduler failover attempts. Clear any
	// reverse mapping installed by the prior account before processing frames.
	setCodexToolNameReverse(c, nil)
	if s == nil {
		return errors.New("service is nil")
	}
	if clientConn == nil {
		return errors.New("client websocket is nil")
	}
	if account == nil {
		return errors.New("account is nil")
	}
	if err := validateOpenAIWSBearerToken(account, token); err != nil {
		return err
	}
	if isOpenAIResponsesLiteWebSocketPayload(firstClientMessage) {
		liteFirstMessage, _, liteErr := normalizeOpenAIResponsesLitePayloadForAccount(firstClientMessage, account)
		if liteErr != nil {
			return NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, liteErr.Error(), liteErr)
		}
		firstClientMessage = liteFirstMessage
	}
	originalFirstClientMessage := firstClientMessage
	if next, policyErr := applyOpenAIWSReasoningEffortPolicy(firstClientMessage, hooks); policyErr != nil {
		return NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, policyErr.Error(), policyErr)
	} else {
		firstClientMessage = next
	}
	requestModel := strings.TrimSpace(gjson.GetBytes(firstClientMessage, "model").String())
	requestPreviousResponseID := strings.TrimSpace(gjson.GetBytes(firstClientMessage, "previous_response_id").String())
	promptCacheKey := strings.TrimSpace(gjson.GetBytes(firstClientMessage, "prompt_cache_key").String())
	logOpenAIWSV2Passthrough(
		"relay_start account_id=%d model=%s previous_response_id=%s first_message_type=%s first_message_bytes=%d",
		account.ID,
		truncateOpenAIWSLogValue(requestModel, openAIWSLogValueMaxLen),
		truncateOpenAIWSLogValue(requestPreviousResponseID, openAIWSIDValueMaxLen),
		openaiwsv2RelayMessageTypeName(coderws.MessageText),
		len(firstClientMessage),
	)

	// Apply OpenAI Fast Policy on the first response.create frame. Subsequent
	// frames are filtered via a wrapping FrameConn below so every client→
	// upstream frame goes through the same policy evaluator/normalize/scope as
	// HTTP entrypoints.
	//
	// We capture the session-level model from the first frame here so the
	// per-frame filter (below) can fall back to it when a follow-up frame
	// omits "model" — Realtime clients are allowed to send response.create
	// without re-stating the model, in which case the upstream uses the model
	// negotiated at session.update time. Without this fallback, an empty
	// model would miss any admin-configured model whitelist and be silently
	// passed through, defeating that policy on every frame after the first.
	initialRequestModel := ""
	if hooks != nil {
		initialRequestModel = strings.TrimSpace(hooks.InitialRequestModel)
	}
	if initialRequestModel == "" {
		initialRequestModel = openAIWSPassthroughRequestModelForFrame(firstClientMessage)
	}
	if hooks != nil && hooks.MapRequestModel != nil {
		mappedModel, mapErr := hooks.MapRequestModel(1, initialRequestModel)
		if mapErr != nil {
			return mapErr
		}
		if mappedModel = strings.TrimSpace(mappedModel); mappedModel != "" {
			firstClientMessage = s.ReplaceModelInBody(firstClientMessage, mappedModel)
		}
	}
	capturedSessionModel := openAIWSPassthroughPolicyModelForFrame(account, firstClientMessage)
	if capturedSessionModel != "" && capturedSessionModel != strings.TrimSpace(gjson.GetBytes(firstClientMessage, "model").String()) {
		firstClientMessage = s.ReplaceModelInBody(firstClientMessage, capturedSessionModel)
	}
	firstMessageResponsesLite := isOpenAIResponsesLiteWebSocketPayload(firstClientMessage)
	if normalized, compatibilityChanged, normalizeErr := normalizeOpenAIResponsesWebSocketCompatibilityBody(firstClientMessage, account, firstMessageResponsesLite); normalizeErr != nil {
		return fmt.Errorf("normalize first websocket response.create: %w", normalizeErr)
	} else if compatibilityChanged {
		firstClientMessage = normalized
	}
	if account.IsOpenAIOAuthLike() {
		aliasedBody, reverse, aliased, aliasErr := aliasOpenAIOAuthReservedToolNamesBody(firstClientMessage)
		if aliasErr != nil {
			return aliasErr
		}
		updateCodexToolNameReverseForWSFrame(c, firstClientMessage, reverse)
		if aliased {
			firstClientMessage = aliasedBody
		}
	}
	accountScopedFirst, accountScoped, scopeErr := applyCodexAccountIdentityClientMetadataRaw(firstClientMessage, codexAccountIdentitySource(c, account), getAPIKeyIDFromContext(c))
	if scopeErr != nil {
		return NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "invalid websocket identity metadata", scopeErr)
	}
	if accountScoped {
		firstClientMessage = accountScopedFirst
	}
	usageMeta := newOpenAIWSPassthroughUsageMeta(initialRequestModel, firstClientMessage)
	updatedFirst, blocked, policyErr := s.applyOpenAIFastPolicyToWSResponseCreate(ctx, account, capturedSessionModel, firstClientMessage)
	if policyErr != nil {
		return fmt.Errorf("apply openai fast policy on first ws frame: %w", policyErr)
	}
	if blocked != nil {
		MarkOpsClientBusinessLimited(c, OpsClientBusinessLimitedReasonLocalPolicyDenied)
		// coder/websocket@v1.8.14 Conn.Write is synchronous: it acquires
		// writeFrameMu, writes the entire frame, and Flushes the underlying
		// bufio writer before returning (write.go:42 → write.go:307-311).
		// The subsequent close handshake re-acquires the same writeFrameMu
		// to send the close frame, so the error event is guaranteed to
		// reach the kernel send buffer before any close frame is queued.
		// No explicit flush hop is required here.
		eventBytes := buildOpenAIFastPolicyBlockedWSEvent(blocked)
		if eventBytes != nil {
			writeCtx, cancelWrite := context.WithTimeout(ctx, s.openAIWSWriteTimeout())
			_ = clientConn.Write(writeCtx, coderws.MessageText, eventBytes)
			cancelWrite()
		}
		return NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, blocked.Message, blocked)
	}
	firstClientMessage = updatedFirst

	// 在 policy filter 之后再提取 service_tier / reasoning_effort 用于
	// usage 上报：filter 命中时 service_tier 已经从 firstClientMessage 中删除，
	// 最终出站 tier 应为 nil，而不是用户最初请求的 "priority"。观察到的回包
	// tier 单独保存在 UpstreamResponseServiceTier，由 usage 阶段统一决策。
	// HTTP 入口（line ~2728 extractOpenAIServiceTier(reqBody)）
	// 与 WS ingress（openai_ws_forwarder.go:2991 取自 payload）的语义一致。
	//
	// 多轮 passthrough：OpenAI Realtime / Responses WS 协议允许客户端在
	// 同一连接的不同 response.create 帧上发送不同 service_tier（参考
	// codex-rs/core/src/client.rs build_responses_request 每次重新填值）。
	// 因此使用 atomic.Pointer[string] 在 filter（runClientToUpstream
	// goroutine）和 OnTurnComplete / final result（runUpstreamToClient
	// goroutine）之间同步当前 turn 的 usage metadata。
	usageMeta.initFromFirstFrame(firstClientMessage, capturedSessionModel)
	usageMeta.captureRequestedReasoningEffort(originalFirstClientMessage, capturedSessionModel)
	_, initialUpstreamModel := usageMeta.turnModels(initialRequestModel)
	SetOpsUpstreamModel(c, initialUpstreamModel)
	wsURL, err := s.buildOpenAIResponsesWSURL(account)
	if err != nil {
		return fmt.Errorf("build ws url: %w", err)
	}
	wsHost := "-"
	wsPath := "-"
	if parsedURL, parseErr := url.Parse(wsURL); parseErr == nil && parsedURL != nil {
		wsHost = normalizeOpenAIWSLogValue(parsedURL.Host)
		wsPath = normalizeOpenAIWSLogValue(parsedURL.Path)
	}
	logOpenAIWSV2Passthrough(
		"relay_dial_start account_id=%d ws_host=%s ws_path=%s proxy_enabled=%v",
		account.ID,
		wsHost,
		wsPath,
		account.ProxyID != nil && account.Proxy != nil,
	)

	isCodexCLI := false
	if c != nil {
		isCodexCLI = openai.IsCodexOfficialClientByHeaders(c.GetHeader("User-Agent"), c.GetHeader("originator"))
	}
	if s.cfg != nil && s.cfg.Gateway.ForceCodexCLI {
		isCodexCLI = true
	}
	turnState := ""
	turnMetadata := ""
	if c != nil {
		turnState = strings.TrimSpace(c.GetHeader(openAIWSTurnStateHeader))
		turnMetadata = strings.TrimSpace(c.GetHeader(openAIWSTurnMetadataHeader))
	}
	headers, _, buildHdrErr := s.buildOpenAIWSHeaders(
		ctx,
		c,
		account,
		token,
		wsDecision,
		isCodexCLI,
		turnState,
		turnMetadata,
		promptCacheKey,
		gjson.GetBytes(firstClientMessage, "model").String(),
		gjson.GetBytes(firstClientMessage, "service_tier").String(),
	)
	if buildHdrErr != nil {
		return fmt.Errorf("build ws headers: %w", buildHdrErr)
	}
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	dialer := s.getOpenAIWSPassthroughDialer()
	if dialer == nil {
		return errors.New("openai ws passthrough dialer is nil")
	}

	agentTaskRecoveryTried := false
	var upstreamConn openAIWSClientConn
	statusCode := 0
	var handshakeHeaders http.Header
	for {
		headers, err = s.refreshOpenAIAgentIdentityHeaders(ctx, account, headers)
		if err != nil {
			return fmt.Errorf("refresh ws authentication headers: %w", err)
		}
		dialCtx, cancelDial := context.WithTimeout(ctx, s.openAIWSDialTimeout())
		upstreamConn, statusCode, handshakeHeaders, err = dialer.Dial(dialCtx, wsURL, headers, proxyURL)
		cancelDial()
		if err == nil {
			break
		}
		var handshakeErr *openAIWSHandshakeError
		responseBody := []byte(nil)
		if errors.As(err, &handshakeErr) && handshakeErr != nil {
			responseBody = handshakeErr.Body
		}
		dialErr := &openAIWSDialError{StatusCode: statusCode, ResponseHeaders: cloneHeader(handshakeHeaders), ResponseBody: responseBody, Err: err}
		if s.isAgentIdentityAccount(ctx, account) && isAgentIdentityTaskInvalidWSDialError(dialErr) && !agentTaskRecoveryTried {
			agentTaskRecoveryTried = true
			if recoveryErr := s.recoverAgentIdentityTask(ctx, account, account.GetCredential("task_id")); recoveryErr != nil {
				return fmt.Errorf("agent identity task recovery failed: %w", recoveryErr)
			}
			continue
		}
		logOpenAIWSV2Passthrough(
			"relay_dial_failed account_id=%d status_code=%d err=%s",
			account.ID,
			statusCode,
			truncateOpenAIWSLogValue(err.Error(), openAIWSLogValueMaxLen),
		)
		s.handleOpenAIWSDialTransientFailure(ctx, account, capturedSessionModel, dialErr)
		if statusCode == http.StatusTooManyRequests {
			s.persistOpenAIWSRateLimitSignal(ctx, account, handshakeHeaders, nil, "rate_limit_exceeded", "rate_limit_error", strings.TrimSpace(err.Error()), capturedSessionModel)
			return s.newOpenAIWSRateLimitFailoverError(account, handshakeHeaders, nil, err.Error())
		}
		return s.mapOpenAIWSPassthroughDialError(err, statusCode, handshakeHeaders)
	}
	defer func() {
		_ = upstreamConn.Close()
	}()
	logOpenAIWSV2Passthrough(
		"relay_dial_ok account_id=%d status_code=%d upstream_request_id=%s",
		account.ID,
		statusCode,
		openAIWSHeaderValueForLog(handshakeHeaders, "x-request-id"),
	)

	upstreamFrameConn, ok := upstreamConn.(openaiwsv2.FrameConn)
	if !ok {
		return errors.New("openai ws passthrough upstream connection does not support frame relay")
	}
	relayUpstreamFrameConn := &openAIWSPassthroughFirstOutputFrameConn{
		inner:             upstreamFrameConn,
		activeReadTimeout: s.openAIWSPassthroughIdleTimeout(),
		deadlineChanged:   make(chan struct{}, 1),
		resolveDeadline: func(payload []byte) openAIWSPassthroughFirstOutputDeadline {
			reasoningEffort := ""
			if current := usageMeta.reasoningEffort.Load(); current != nil {
				reasoningEffort = *current
			}
			timeout := s.openAIFirstOutputTimeout(reasoningEffort)
			if timeout <= 0 {
				timeout = s.openAIWSPassthroughIdleTimeout()
			}
			model := openAIWSPassthroughRequestModelForFrame(payload)
			if model == "" {
				model = usageMeta.requestModelForFrame(payload)
			}
			if model == "" {
				model = requestModel
			}
			return openAIWSPassthroughFirstOutputDeadline{
				timeout:         timeout,
				startedAt:       time.Now(),
				requestModel:    model,
				reasoningEffort: reasoningEffort,
			}
		},
	}

	completedTurns := atomic.Int32{}
	turnLifecycle := newOpenAIWSPassthroughTurnLifecycle(true)
	var acceptedTurnStartedAt atomic.Pointer[time.Time]
	clientFrameConn := &openAIWSClientFrameConn{
		conn:                 clientConn,
		controlCtx:           ctx,
		interTurnIdleTimeout: s.openAIWSIngressInterTurnIdleTimeout(),
		interTurnStarted:     make(chan struct{}, 1),
		restoreResponseModel: func(payload []byte) []byte {
			eventType := strings.TrimSpace(gjson.GetBytes(payload, "type").String())
			if !openAIWSEventMayContainModel(eventType) {
				return payload
			}
			requestModel, upstreamModel := usageMeta.turnModels("")
			return replaceOpenAIWSMessageModel(payload, upstreamModel, requestModel)
		},
		restoreToolNames: func(payload []byte) []byte {
			return restoreCodexToolNamesFromContext(c, payload)
		},
	}
	policyClientConn := &openAIWSPolicyEnforcingFrameConn{
		inner: clientFrameConn,
		// 注意线程安全：filter 仅在 runClientToUpstream 这一条
		// goroutine 中被调用（passthrough_relay.go: ReadFrame loop），
		// capturedSessionModel 的读写都发生在该 goroutine 内，因此无需
		// 加锁/原子化。
		filter: func(msgType coderws.MessageType, payload []byte) (out []byte, blocked *OpenAIFastBlockedError, filterErr error) {
			if msgType != coderws.MessageText && msgType != coderws.MessageBinary {
				return payload, nil, nil
			}
			eventType := strings.TrimSpace(gjson.GetBytes(payload, "type").String())
			isResponseCreate := eventType == "response.create"
			responseCreateAt := time.Time{}
			acceptedTurn := false
			if isResponseCreate {
				responseCreateAt = time.Now()
				if !turnLifecycle.beginResponseCreate(clientFrameConn.markTurnStarted) {
					err := errors.New("overlapping response.create is not supported")
					return payload, nil, NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, err.Error(), err)
				}
				defer func() {
					if !acceptedTurn {
						turnLifecycle.cancelResponseCreate()
					}
				}()
			}
			responsesLite := isResponseCreate && isOpenAIResponsesLiteWebSocketPayload(payload)
			if isResponseCreate {
				if normalized, compatibilityChanged, normalizeErr := normalizeOpenAIResponsesWebSocketCompatibilityBody(payload, account, responsesLite); normalizeErr != nil {
					return payload, nil, NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "invalid websocket request payload", normalizeErr)
				} else if compatibilityChanged {
					payload = normalized
				}
			}
			if account.IsOpenAIOAuthLike() && (isResponseCreate || eventType == "session.update") {
				aliasedBody, reverse, aliased, aliasErr := aliasOpenAIOAuthReservedToolNamesBody(payload)
				if aliasErr != nil {
					return payload, nil, NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, aliasErr.Error(), aliasErr)
				}
				updateCodexToolNameReverseForWSFrame(c, payload, reverse)
				if aliased {
					payload = aliasedBody
				}
			}
			if isResponseCreate || eventType == "session.update" {
				accountScopedPayload, accountScoped, scopeErr := applyCodexAccountIdentityClientMetadataRaw(payload, codexAccountIdentitySource(c, account), getAPIKeyIDFromContext(c))
				if scopeErr != nil {
					return payload, nil, NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "invalid websocket identity metadata", scopeErr)
				}
				if accountScoped {
					payload = accountScopedPayload
				}
			}
			if isResponseCreate {
				if responsesLite {
					litePayload, _, liteErr := normalizeOpenAIResponsesLitePayloadForAccount(payload, account)
					if liteErr != nil {
						return payload, nil, NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, liteErr.Error(), liteErr)
					}
					payload = litePayload
				}
				originalResponseCreate := payload
				if next, policyErr := applyOpenAIWSReasoningEffortPolicy(payload, hooks); policyErr != nil {
					return payload, nil, NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, policyErr.Error(), policyErr)
				} else {
					payload = next
				}
				usageMeta.captureRequestedReasoningEffort(originalResponseCreate)
			}
			turnNo := int(completedTurns.Load()) + 1
			if turnNo < 2 {
				turnNo = 2
			}
			requestModelForThisFrame := ""
			if isResponseCreate {
				requestModelForThisFrame = usageMeta.requestModelForFrame(payload)
				if requestModelForThisFrame == "" {
					requestModelForThisFrame = capturedSessionModel
				}
				if hooks != nil && hooks.BeforeRequest != nil {
					if err := hooks.BeforeRequest(turnNo, payload, requestModelForThisFrame); err != nil {
						return payload, nil, err
					}
				}
				if hooks != nil && hooks.MapRequestModel != nil {
					upstreamModel, err := hooks.MapRequestModel(turnNo, requestModelForThisFrame)
					if err != nil {
						return payload, nil, err
					}
					if upstreamModel = strings.TrimSpace(upstreamModel); upstreamModel != "" {
						payload = s.ReplaceModelInBody(payload, upstreamModel)
					}
				}
			}
			// 在评估策略前先刷新 capturedSessionModel：客户端可能通过
			// session.update 修改 session-level model（Realtime /
			// Responses WS 协议允许），如果不刷新就会出现
			// "首帧 model=gpt-4o（pass）→ session.update 改成 gpt-5.5
			// → 不带 model 的 response.create fallback 到 gpt-4o" 的
			// 绕过路径。这里只看 session.update 事件中的 session.model
			// 字段，response.create 自己的 model 仍然由其本帧字段决定。
			if updated := openAIWSPassthroughPolicyModelFromSessionFrame(account, payload); updated != "" {
				capturedSessionModel = updated
			}
			usageMeta.updateSessionRequestModel(payload)
			if requestModelForThisFrame == "" {
				requestModelForThisFrame = usageMeta.requestModelForFrame(payload)
			}
			// Per-frame model first; if the client omits "model" on a
			// follow-up frame (legal in Realtime), fall back to the
			// session-level model captured from the first frame so the
			// model whitelist still resolves. An empty model would miss
			// any whitelist and silently fall back to pass.
			model := openAIWSPassthroughPolicyModelForFrame(account, payload)
			if model == "" {
				model = capturedSessionModel
			}
			if isResponseCreate && model != "" && model != strings.TrimSpace(gjson.GetBytes(payload, "model").String()) {
				payload = s.ReplaceModelInBody(payload, model)
			}
			out, blocked, policyErr := s.applyOpenAIFastPolicyToWSResponseCreate(ctx, account, model, payload)
			// 多轮 passthrough usage：仅在成功（non-block / non-err）
			// 的 response.create 帧上更新 usageMeta，使用
			// filter 处理后的 payload，与首帧 policy-after-extract 语义
			// 保持一致（参见上方 extractOpenAIServiceTierFromBody 注释）。
			//   - 非 response.create 帧（response.cancel /
			//     conversation.item.create / session.update 等）不携带
			//     per-response metadata，不应覆盖前一轮值。
			//   - blocked != nil：该帧不会发送上游，usage metadata 应保持
			//     上一轮值。
			//   - policyErr != nil：异常路径，保持上一轮值。
			//   - 不带 service_tier 的 response.create 会让
			//     extractOpenAIServiceTierFromBody 返回 nil；这里有意
			//     覆盖（Store(nil)），因为 OpenAI 上游对该帧实际不传
			//     service_tier 时按 default 处理，billing 应如实反映。
			if policyErr == nil && blocked == nil && isResponseCreate {
				usageMeta.updateFromResponseCreate(out, model, requestModelForThisFrame)
				_, actualModel := usageMeta.turnModels(requestModelForThisFrame)
				SetOpsUpstreamModel(c, actualModel)
				responseCreateAtCopy := responseCreateAt
				acceptedTurnStartedAt.Store(&responseCreateAtCopy)
				acceptedTurn = true
			}
			return out, blocked, policyErr
		},
		onBlock: func(blocked *OpenAIFastBlockedError) {
			MarkOpsClientBusinessLimited(c, OpsClientBusinessLimitedReasonLocalPolicyDenied)
			// See note above on Conn.Write being synchronous w.r.t. flush;
			// no explicit flush is required to ensure the error event lands
			// before the close frame.
			eventBytes := buildOpenAIFastPolicyBlockedWSEvent(blocked)
			if eventBytes == nil {
				return
			}
			writeCtx, cancel := context.WithTimeout(ctx, s.openAIWSWriteTimeout())
			_ = clientConn.Write(writeCtx, coderws.MessageText, eventBytes)
			cancel()
		},
	}
	upstreamFirstMessageSent := false
	firstWriteCtx, cancelFirstWrite := context.WithTimeout(ctx, s.openAIWSWriteTimeout())
	firstWriteErr := relayUpstreamFrameConn.WriteFrame(firstWriteCtx, coderws.MessageText, firstClientMessage)
	cancelFirstWrite()
	if firstWriteErr != nil {
		return wrapOpenAIWSIngressTurnError(
			"write_upstream",
			fmt.Errorf("write first upstream websocket request: %w", firstWriteErr),
			false,
		)
	}
	upstreamFirstMessageSent = true

	readNextClientFrame := func(readCtx context.Context, conn openaiwsv2.FrameConn) (coderws.MessageType, []byte, error) {
		for {
			msgType, payload, readErr := conn.ReadFrame(readCtx)
			if readErr != nil {
				return msgType, payload, readErr
			}
			if (msgType == coderws.MessageText || msgType == coderws.MessageBinary) && strings.TrimSpace(gjson.GetBytes(payload, "type").String()) == "response.create" {
				return msgType, payload, nil
			}
			if writeErr := upstreamFrameConn.WriteFrame(readCtx, msgType, payload); writeErr != nil {
				return msgType, payload, writeErr
			}
		}
	}

	firstTurnStartedAt := time.Time{}
	if hooks != nil {
		firstTurnStartedAt = hooks.InitialTurnStartedAt
	}
	failureAccountSideEffectsApplied := false
	relayResult, relayExit := openaiwsv2.RunEntry(openaiwsv2.EntryInput{
		Ctx:                ctx,
		ClientConn:         policyClientConn,
		UpstreamConn:       relayUpstreamFrameConn,
		FirstClientMessage: firstClientMessage,
		Options: openaiwsv2.RelayOptions{
			WriteTimeout:       s.openAIWSWriteTimeout(),
			FirstTurnStartedAt: firstTurnStartedAt,
			TakeNextTurnStartedAt: func() time.Time {
				startedAt := acceptedTurnStartedAt.Swap(nil)
				if startedAt == nil {
					return time.Time{}
				}
				return *startedAt
			},
			// Passthrough idle is enforced only after a completed turn by
			// clientFrameConn. The relay-wide activity watchdog would also
			// terminate a healthy active upstream turn.
			IdleTimeout:                     0,
			FirstMessageType:                coderws.MessageText,
			FirstMessageSent:                upstreamFirstMessageSent,
			StartClientAfterFirstDownstream: true,
			ReadClientFrame:                 readNextClientFrame,
			OnUsageParseFailure: func(eventType string, usageRaw string) {
				logOpenAIWSV2Passthrough(
					"usage_parse_failed event_type=%s usage_raw=%s",
					truncateOpenAIWSLogValue(eventType, openAIWSLogValueMaxLen),
					truncateOpenAIWSLogValue(usageRaw, openAIWSLogValueMaxLen),
				)
			},
			OnTurnComplete: func(turn openaiwsv2.RelayTurnResult) {
				turnNo := int(completedTurns.Add(1))
				if hooks != nil && hooks.TurnStarted != nil && !turn.StartedAt.IsZero() {
					hooks.TurnStarted(turnNo, turn.StartedAt)
				}
				turnRequestModel, turnUpstreamModel := usageMeta.turnModels(turn.RequestModel)
				turnResult := &OpenAIForwardResult{
					RequestID: turn.RequestID,
					Usage: OpenAIUsage{
						InputTokens:              turn.Usage.InputTokens,
						OutputTokens:             turn.Usage.OutputTokens,
						CacheCreationInputTokens: turn.Usage.CacheCreationInputTokens,
						CacheReadInputTokens:     turn.Usage.CacheReadInputTokens,
						ImageOutputTokens:        turn.Usage.ImageOutputTokens,
					},
					Model:                         turnRequestModel,
					UpstreamModel:                 openAIWSDifferentModel(turnRequestModel, turnUpstreamModel),
					UpstreamResponseModel:         turn.ResponseModel,
					UpstreamResponseModelConflict: turn.ResponseModelConflict,
					UpstreamResponseServiceTier:   normalizeObservedOpenAIServiceTier(turn.ResponseServiceTier),
					ServiceTier:                   usageMeta.serviceTier.Load(),
					ReasoningEffort:               usageMeta.reasoningEffort.Load(),
					RequestedReasoningEffort:      usageMeta.requestedReasoningEffort.Load(),
					Stream:                        true,
					OpenAIWSMode:                  true,
					UpstreamTerminalEvent:         normalizeOpenAIWSTerminalEvent(turn.TerminalEventType),
					ResponseHeaders:               cloneHeader(handshakeHeaders),
					Duration:                      turn.Duration,
					FirstTokenMs:                  turn.FirstTokenMs,
				}
				logOpenAIWSV2Passthrough(
					"relay_turn_completed account_id=%d turn=%d request_id=%s terminal_event=%s turn_requested_model=%s turn_upstream_model=%s duration_ms=%d first_token_ms=%d input_tokens=%d output_tokens=%d cache_read_tokens=%d",
					account.ID,
					turnNo,
					truncateOpenAIWSLogValue(turnResult.RequestID, openAIWSIDValueMaxLen),
					truncateOpenAIWSLogValue(turn.TerminalEventType, openAIWSLogValueMaxLen),
					truncateOpenAIWSLogValue(turnRequestModel, openAIWSLogValueMaxLen),
					truncateOpenAIWSLogValue(turnUpstreamModel, openAIWSLogValueMaxLen),
					turnResult.Duration.Milliseconds(),
					openAIWSFirstTokenMsForLog(turnResult.FirstTokenMs),
					turnResult.Usage.InputTokens,
					turnResult.Usage.OutputTokens,
					turnResult.Usage.CacheReadInputTokens,
				)
				if hooks != nil && hooks.AfterTurn != nil {
					hooks.AfterTurn(turnNo, turnResult, nil)
				}
			},
			BeforeClientWrite: func(msgType coderws.MessageType, payload []byte) {
				if msgType == coderws.MessageText && openAIWSPassthroughIsTerminalOutput(payload) {
					turnLifecycle.beginTerminalWrite()
				}
			},
			AfterClientWrite: func(msgType coderws.MessageType, payload []byte, writeErr error) {
				if msgType == coderws.MessageText && writeErr == nil {
					eventType, _, _ := parseOpenAIWSEventEnvelope(payload)
					markOpenAIWSClientVisibleFailure(c, eventType, payload)
				}
				if msgType == coderws.MessageText && openAIWSPassthroughIsTerminalOutput(payload) {
					turnLifecycle.finishTerminalWrite(writeErr == nil, clientFrameConn.markTurnCompleted)
				}
			},
			BeforeRelayCancel: func(exit openaiwsv2.RelayExit) {
				if context.Cause(ctx) != nil {
					return
				}
				status, reason, ok := openAIWSPassthroughRelayClientClose(exit, int(completedTurns.Load()))
				if !ok {
					return
				}
				// Match the handler close path and stay within the WebSocket control
				// frame limit; an oversized reason makes coder/websocket skip the
				// close frame, leaving the client with EOF instead of the status code.
				reason = truncateString(reason, 120)
				_ = clientConn.Close(status, reason)
				_ = clientConn.CloseNow()
			},
			BeforeWriteClient: func(msgType coderws.MessageType, payload []byte, wroteDownstream bool) error {
				if msgType != coderws.MessageText {
					return nil
				}
				eventType, _, _ := parseOpenAIWSEventEnvelope(payload)
				if eventType == "response.created" {
					failureAccountSideEffectsApplied = false
				}
				if (eventType == "error" || eventType == "response.failed") && markOpenAIWSV2PassthroughCyberPolicy(c, payload) {
					return nil
				}
				errCodeRaw, errTypeRaw, errMsgRaw := parseOpenAIWSErrorEventFields(payload)
				isPreOutputRateLimit := eventType == "error" && !wroteDownstream && isOpenAIWSRateLimitError(errCodeRaw, errTypeRaw, errMsgRaw)
				if (eventType == "error" || eventType == "response.failed") && !failureAccountSideEffectsApplied && !isPreOutputRateLimit {
					failureAccountSideEffectsApplied = s.handleOpenAIWSFailureAccountSideEffects(ctx, account, capturedSessionModel, handshakeHeaders, payload)
				}
				if eventType != "error" {
					return nil
				}
				if wroteDownstream || !isOpenAIWSRateLimitError(errCodeRaw, errTypeRaw, errMsgRaw) {
					return nil
				}
				s.persistOpenAIWSRateLimitSignal(ctx, account, handshakeHeaders, payload, errCodeRaw, errTypeRaw, errMsgRaw, capturedSessionModel)
				logOpenAIWSV2Passthrough(
					"relay_rate_limit_failover account_id=%d err_code=%s err_type=%s err_message=%s",
					account.ID,
					truncateOpenAIWSLogValue(errCodeRaw, openAIWSLogValueMaxLen),
					truncateOpenAIWSLogValue(errTypeRaw, openAIWSLogValueMaxLen),
					truncateOpenAIWSLogValue(errMsgRaw, openAIWSLogValueMaxLen),
				)
				return s.newOpenAIWSRateLimitFailoverError(account, handshakeHeaders, payload, errMsgRaw)
			},
			OnTrace: func(event openaiwsv2.RelayTraceEvent) {
				logOpenAIWSV2Passthrough(
					"relay_trace account_id=%d stage=%s direction=%s msg_type=%s bytes=%d graceful=%v wrote_downstream=%v err=%s",
					account.ID,
					truncateOpenAIWSLogValue(event.Stage, openAIWSLogValueMaxLen),
					truncateOpenAIWSLogValue(event.Direction, openAIWSLogValueMaxLen),
					truncateOpenAIWSLogValue(event.MessageType, openAIWSLogValueMaxLen),
					event.PayloadBytes,
					event.Graceful,
					event.WroteDownstream,
					truncateOpenAIWSLogValue(event.Error, openAIWSLogValueMaxLen),
				)
			},
		},
	})
	if cause := context.Cause(ctx); cause != nil {
		if isOpenAIWSSessionPreempted(ctx) {
			return errOpenAIWSSessionPreempted
		}
		status := coderws.StatusGoingAway
		reason := "websocket request canceled"
		if errors.Is(cause, ErrOpenAIWSIngressLeaseLost) {
			status = coderws.StatusTryAgainLater
			reason = "websocket ingress capacity lease lost; please reconnect"
		}
		_ = clientConn.Close(status, reason)
		_ = clientConn.CloseNow()
		return NewOpenAIWSClientCloseError(status, reason, cause)
	}

	resultRequestModel, resultUpstreamModel := usageMeta.turnModels(relayResult.RequestModel)
	result := &OpenAIForwardResult{
		RequestID: relayResult.RequestID,
		Usage: OpenAIUsage{
			InputTokens:              relayResult.Usage.InputTokens,
			OutputTokens:             relayResult.Usage.OutputTokens,
			CacheCreationInputTokens: relayResult.Usage.CacheCreationInputTokens,
			CacheReadInputTokens:     relayResult.Usage.CacheReadInputTokens,
			ImageOutputTokens:        relayResult.Usage.ImageOutputTokens,
		},
		Model:                         resultRequestModel,
		UpstreamModel:                 openAIWSDifferentModel(resultRequestModel, resultUpstreamModel),
		UpstreamResponseModel:         relayResult.ResponseModel,
		UpstreamResponseModelConflict: relayResult.ResponseModelConflict,
		UpstreamResponseServiceTier:   normalizeObservedOpenAIServiceTier(relayResult.ResponseServiceTier),
		ServiceTier:                   usageMeta.serviceTier.Load(),
		ReasoningEffort:               usageMeta.reasoningEffort.Load(),
		RequestedReasoningEffort:      usageMeta.requestedReasoningEffort.Load(),
		Stream:                        true,
		OpenAIWSMode:                  true,
		UpstreamTerminalEvent:         normalizeOpenAIWSTerminalEvent(relayResult.TerminalEventType),
		ResponseHeaders:               cloneHeader(handshakeHeaders),
		Duration:                      relayResult.Duration,
		FirstTokenMs:                  relayResult.FirstTokenMs,
	}

	turnCount := int(completedTurns.Load())
	if relayExit == nil {
		logOpenAIWSV2Passthrough(
			"relay_completed account_id=%d request_id=%s terminal_event=%s duration_ms=%d c2u_frames=%d u2c_frames=%d dropped_frames=%d turns=%d",
			account.ID,
			truncateOpenAIWSLogValue(result.RequestID, openAIWSIDValueMaxLen),
			truncateOpenAIWSLogValue(relayResult.TerminalEventType, openAIWSLogValueMaxLen),
			result.Duration.Milliseconds(),
			relayResult.ClientToUpstreamFrames,
			relayResult.UpstreamToClientFrames,
			relayResult.DroppedDownstreamFrames,
			turnCount,
		)
		// 正常路径按 terminal 事件逐 turn 已回调；仅在零 turn 场景兜底回调一次。
		if turnCount == 0 && hooks != nil && hooks.AfterTurn != nil {
			if hooks.TurnStarted != nil {
				hooks.TurnStarted(1, time.Now().Add(-result.Duration))
			}
			hooks.AfterTurn(1, result, nil)
		}
		return nil
	}
	logOpenAIWSV2Passthrough(
		"relay_failed account_id=%d stage=%s wrote_downstream=%v err=%s duration_ms=%d c2u_frames=%d u2c_frames=%d dropped_frames=%d turns=%d",
		account.ID,
		truncateOpenAIWSLogValue(relayExit.Stage, openAIWSLogValueMaxLen),
		relayExit.WroteDownstream,
		truncateOpenAIWSLogValue(relayErrorText(relayExit.Err), openAIWSLogValueMaxLen),
		result.Duration.Milliseconds(),
		relayResult.ClientToUpstreamFrames,
		relayResult.UpstreamToClientFrames,
		relayResult.DroppedDownstreamFrames,
		turnCount,
	)

	relayErr := relayExit.Err
	var firstOutputTimeoutErr *openAIWSPassthroughFirstOutputTimeoutError
	if errors.As(relayErr, &firstOutputTimeoutErr) {
		deadline := firstOutputTimeoutErr.deadline
		// The relay ran over the WebSocket transport, so a missing managed
		// proxy is an unknown route (http.DefaultClient), not a direct one.
		wsProxyID, wsProxyName := opsUpstreamWSProxyAttribution(account)
		failoverErr := s.newOpenAIFirstOutputTimeoutError(
			ctx,
			c,
			account,
			wsProxyID,
			wsProxyName,
			deadline.startedAt,
			deadline.requestModel,
			deadline.reasoningEffort,
			deadline.timeout,
			"websocket_first_semantic_output",
			handshakeHeaders,
		)
		if turnCount == 0 && !relayExit.WroteDownstream {
			relayErr = failoverErr
		} else {
			// The handler only retains the initial response.create across
			// account attempts. Replaying it after a later-turn timeout would
			// duplicate the first turn, so later turns end the client session.
			relayErr = NewOpenAIWSClientCloseError(
				coderws.StatusGoingAway,
				"upstream produced no semantic output; please reconnect",
				firstOutputTimeoutErr,
			)
		}
	}
	var activeTurnTimeoutErr *openAIWSPassthroughActiveTurnTimeoutError
	if errors.As(relayErr, &activeTurnTimeoutErr) {
		relayErr = NewOpenAIWSClientCloseError(
			coderws.StatusGoingAway,
			"upstream websocket read timeout; please reconnect",
			activeTurnTimeoutErr,
		)
	}
	if relayExit.Stage == "idle_timeout" {
		relayErr = NewOpenAIWSClientCloseError(
			coderws.StatusPolicyViolation,
			"client websocket idle timeout",
			relayErr,
		)
	}
	turnErr := wrapOpenAIWSIngressTurnError(
		relayExit.Stage,
		relayErr,
		relayExit.WroteDownstream,
	)
	if hooks != nil && hooks.AfterTurn != nil {
		if hooks.TurnStarted != nil {
			hooks.TurnStarted(turnCount+1, time.Now().Add(-result.Duration))
		}
		hooks.AfterTurn(turnCount+1, nil, turnErr)
	}
	return turnErr
}

func openAIWSPassthroughRelayClientClose(exit openaiwsv2.RelayExit, completedTurns int) (coderws.StatusCode, string, bool) {
	var closeErr *OpenAIWSClientCloseError
	if errors.As(exit.Err, &closeErr) {
		return closeErr.StatusCode(), closeErr.Reason(), true
	}
	var activeTurnTimeoutErr *openAIWSPassthroughActiveTurnTimeoutError
	if errors.As(exit.Err, &activeTurnTimeoutErr) {
		return coderws.StatusGoingAway, "upstream websocket read timeout; please reconnect", true
	}
	var firstOutputTimeoutErr *openAIWSPassthroughFirstOutputTimeoutError
	if errors.As(exit.Err, &firstOutputTimeoutErr) {
		if completedTurns > 0 || exit.WroteDownstream {
			return coderws.StatusGoingAway, "upstream produced no semantic output; please reconnect", true
		}
		return 0, "", false
	}
	if !exit.Graceful && exit.Stage == "read_upstream" {
		return coderws.StatusInternalError, "upstream websocket proxy failed", true
	}
	return 0, "", false
}

func markOpenAIWSV2PassthroughCyberPolicy(c *gin.Context, payload []byte) bool {
	usage := OpenAIUsage{}
	parseOpenAIWSResponseUsageFromCompletedEvent(payload, &usage)
	return markOpenAICyberPolicyEvent(c, payload, http.StatusOK, &usage)
}

func (s *OpenAIGatewayService) mapOpenAIWSPassthroughDialError(
	err error,
	statusCode int,
	handshakeHeaders http.Header,
) error {
	if err == nil {
		return nil
	}
	wrappedErr := err
	var dialErr *openAIWSDialError
	if !errors.As(err, &dialErr) {
		var handshakeErr *openAIWSHandshakeError
		var responseBody []byte
		if errors.As(err, &handshakeErr) && handshakeErr != nil {
			responseBody = append([]byte(nil), handshakeErr.Body...)
		}
		wrappedErr = &openAIWSDialError{
			StatusCode:      statusCode,
			ResponseHeaders: cloneHeader(handshakeHeaders),
			ResponseBody:    responseBody,
			Err:             err,
		}
	}

	if errors.Is(err, context.Canceled) {
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return NewOpenAIWSClientCloseError(
			coderws.StatusTryAgainLater,
			"upstream websocket connect timeout",
			wrappedErr,
		)
	}
	if statusCode == http.StatusTooManyRequests {
		return NewOpenAIWSClientCloseError(
			coderws.StatusTryAgainLater,
			"upstream websocket is busy, please retry later",
			wrappedErr,
		)
	}
	if statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
		return NewOpenAIWSClientCloseError(
			coderws.StatusPolicyViolation,
			"upstream websocket authentication failed",
			wrappedErr,
		)
	}
	if statusCode >= http.StatusBadRequest && statusCode < http.StatusInternalServerError {
		return NewOpenAIWSClientCloseError(
			coderws.StatusPolicyViolation,
			"upstream websocket handshake rejected",
			wrappedErr,
		)
	}
	return fmt.Errorf("openai ws passthrough dial: %w", wrappedErr)
}

func openaiwsv2RelayMessageTypeName(msgType coderws.MessageType) string {
	switch msgType {
	case coderws.MessageText:
		return "text"
	case coderws.MessageBinary:
		return "binary"
	default:
		return fmt.Sprintf("unknown(%d)", msgType)
	}
}

func relayErrorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func openAIWSFirstTokenMsForLog(firstTokenMs *int) int {
	if firstTokenMs == nil {
		return -1
	}
	return *firstTokenMs
}

func logOpenAIWSV2Passthrough(format string, args ...any) {
	logger.LegacyPrintf(
		"service.openai_ws_v2",
		"[OpenAI WS v2 passthrough] %s "+format,
		append([]any{openaiWSV2PassthroughModeFields}, args...)...,
	)
}
