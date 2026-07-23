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
	interTurnStarted     chan struct{REDACTED
	waitingForNextTurn   atomic.Bool
	// The relay observes upstream payloads, while clients must keep seeing the
	// model identifier they supplied for the current turn.
	restoreResponseModel func([]byte) []byte
REDACTED

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
REDACTED

var _ openaiwsv2.FrameConn = (*openAIWSPolicyEnforcingFrameConn)(nil)

func (c *openAIWSPolicyEnforcingFrameConn) ReadFrame(ctx context.Context) (coderws.MessageType, []byte, error) {
	if c == nil || c.inner == nil {
		return coderws.MessageText, nil, errOpenAIWSConnClosed
REDACTED
	msgType, payload, err := c.inner.ReadFrame(ctx)
	if err != nil {
		return msgType, payload, err
REDACTED
	if c.filter == nil {
		return msgType, payload, nil
REDACTED
	updated, blocked, filterErr := c.filter(msgType, payload)
	if filterErr != nil {
		return msgType, payload, filterErr
REDACTED
	if blocked != nil {
		if c.onBlock != nil {
			c.onBlock(blocked)
	REDACTED
		return msgType, nil, NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, blocked.Message, blocked)
REDACTED
	return msgType, updated, nil
REDACTED

func (c *openAIWSPolicyEnforcingFrameConn) WriteFrame(ctx context.Context, msgType coderws.MessageType, payload []byte) error {
	if c == nil || c.inner == nil {
		return errOpenAIWSConnClosed
REDACTED
	return c.inner.WriteFrame(ctx, msgType, payload)
REDACTED

func (c *openAIWSPolicyEnforcingFrameConn) Close() error {
	if c == nil || c.inner == nil {
		return nil
REDACTED
	return c.inner.Close()
REDACTED

// openAIWSPassthroughPolicyModelForFrame returns the upstream-perspective
// model name that should be passed to evaluateOpenAIFastPolicy for a single
// passthrough WS frame. Mirrors the HTTP-side normalization
// (account.GetMappedModel + normalizeOpenAIModelForUpstream) so the WS path
// matches model whitelists identically.
func openAIWSPassthroughPolicyModelForFrame(account *Account, payload []byte) string {
	if account == nil || len(payload) == 0 {
		return ""
REDACTED
	original := strings.TrimSpace(gjson.GetBytes(payload, "model").String())
	if original == "" {
		return ""
REDACTED
	return normalizeOpenAIModelForUpstream(account, account.GetMappedModel(original))
REDACTED

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
//	{"type":"session.update","session":{"model":"gpt-5.5", ...REDACTEDREDACTED
//
// If we only capture the model from the very first frame, a client can ship
// gpt-4o on the first response.create (whitelisted as pass), then
// session.update to gpt-5.5, then send response.create without "model" so
// the per-frame resolver returns "" and the stale capturedSessionModel falls
// back to gpt-4o — defeating the gpt-5.5 fast-policy filter.
func openAIWSPassthroughPolicyModelFromSessionFrame(account *Account, payload []byte) string {
	if account == nil || len(payload) == 0 {
		return ""
REDACTED
	frameType := strings.TrimSpace(gjson.GetBytes(payload, "type").String())
	if frameType != "session.update" {
		return ""
REDACTED
	original := strings.TrimSpace(gjson.GetBytes(payload, "session.model").String())
	if original == "" {
		return ""
REDACTED
	return normalizeOpenAIModelForUpstream(account, account.GetMappedModel(original))
REDACTED

type openAIWSPassthroughUsageMeta struct {
	serviceTier     atomic.Pointer[string]
	reasoningEffort atomic.Pointer[string]
	requestModel    atomic.Pointer[string]
	upstreamModel   atomic.Pointer[string]

	// 仅在 client->upstream filter goroutine 中读写；Load 侧通过上方原子指针同步。
	sessionRequestModel string
REDACTED

func newOpenAIWSPassthroughUsageMeta(initialRequestModel string, firstFrame []byte) *openAIWSPassthroughUsageMeta {
	meta := &openAIWSPassthroughUsageMeta{
		sessionRequestModel: strings.TrimSpace(initialRequestModel),
REDACTED
	if meta.sessionRequestModel == "" {
		meta.sessionRequestModel = openAIWSPassthroughRequestModelForFrame(firstFrame)
REDACTED
	return meta
REDACTED

func (m *openAIWSPassthroughUsageMeta) initFromFirstFrame(policyOutput []byte, mappedModel string) {
	if m == nil {
		return
REDACTED
	m.serviceTier.Store(extractOpenAIServiceTierFromBody(policyOutput))
	m.reasoningEffort.Store(extractOpenAIReasoningEffortFromBody(policyOutput, mappedModel, m.sessionRequestModel))
	m.storeTurnModels(m.sessionRequestModel, policyOutput)
REDACTED

func (m *openAIWSPassthroughUsageMeta) updateSessionRequestModel(payload []byte) {
	if m == nil {
		return
REDACTED
	if model := openAIWSPassthroughRequestModelFromSessionFrame(payload); model != "" {
		m.sessionRequestModel = model
REDACTED
REDACTED

func (m *openAIWSPassthroughUsageMeta) requestModelForFrame(payload []byte) string {
	if m == nil {
		return openAIWSPassthroughRequestModelForFrame(payload)
REDACTED
	if model := openAIWSPassthroughRequestModelForFrame(payload); model != "" {
		return model
REDACTED
	return m.sessionRequestModel
REDACTED

func (m *openAIWSPassthroughUsageMeta) updateFromResponseCreate(policyOutput []byte, mappedModel string, requestModelForFrame string) {
	if m == nil {
		return
REDACTED
	m.serviceTier.Store(extractOpenAIServiceTierFromBody(policyOutput))
	m.reasoningEffort.Store(extractOpenAIReasoningEffortFromBody(policyOutput, mappedModel, requestModelForFrame))
	m.storeTurnModels(requestModelForFrame, policyOutput)
REDACTED

func (m *openAIWSPassthroughUsageMeta) storeTurnModels(requestModel string, upstreamPayload []byte) {
	if m == nil {
		return
REDACTED
	requestModel = strings.TrimSpace(requestModel)
	upstreamModel := strings.TrimSpace(gjson.GetBytes(upstreamPayload, "model").String())
	if upstreamModel == "" {
		upstreamModel = requestModel
REDACTED
	m.requestModel.Store(openAIWSTrimmedStringPtr(requestModel))
	m.upstreamModel.Store(openAIWSTrimmedStringPtr(upstreamModel))
REDACTED

func (m *openAIWSPassthroughUsageMeta) turnModels(fallback string) (string, string) {
	requestModel := strings.TrimSpace(fallback)
	upstreamModel := requestModel
	if m == nil {
		return requestModel, upstreamModel
REDACTED
	if current := m.requestModel.Load(); current != nil && strings.TrimSpace(*current) != "" {
		requestModel = strings.TrimSpace(*current)
REDACTED
	if current := m.upstreamModel.Load(); current != nil && strings.TrimSpace(*current) != "" {
		upstreamModel = strings.TrimSpace(*current)
REDACTED
	return requestModel, upstreamModel
REDACTED

func openAIWSTrimmedStringPtr(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
REDACTED
	return &value
REDACTED

func openAIWSDifferentModel(requestModel, upstreamModel string) string {
	upstreamModel = strings.TrimSpace(upstreamModel)
	if upstreamModel == "" || upstreamModel == strings.TrimSpace(requestModel) {
		return ""
REDACTED
	return upstreamModel
REDACTED

func openAIWSPassthroughRequestModelForFrame(payload []byte) string {
	if len(payload) == 0 || strings.TrimSpace(gjson.GetBytes(payload, "type").String()) != "response.create" {
		return ""
REDACTED
	return strings.TrimSpace(gjson.GetBytes(payload, "model").String())
REDACTED

func openAIWSPassthroughRequestModelFromSessionFrame(payload []byte) string {
	if len(payload) == 0 || strings.TrimSpace(gjson.GetBytes(payload, "type").String()) != "session.update" {
		return ""
REDACTED
	return strings.TrimSpace(gjson.GetBytes(payload, "session.model").String())
REDACTED

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
REDACTED

type openAIWSPassthroughFirstOutputTimeoutError struct {
	deadline openAIWSPassthroughFirstOutputDeadline
REDACTED

func (e *openAIWSPassthroughFirstOutputTimeoutError) Error() string {
	return errOpenAIWSPassthroughFirstOutputTimeout.Error()
REDACTED

func (e *openAIWSPassthroughFirstOutputTimeoutError) Unwrap() error {
	return errOpenAIWSPassthroughFirstOutputTimeout
REDACTED

type openAIWSPassthroughActiveTurnTimeoutError struct{REDACTED

func (e *openAIWSPassthroughActiveTurnTimeoutError) Error() string {
	return errOpenAIWSPassthroughActiveTurnTimeout.Error()
REDACTED

func (e *openAIWSPassthroughActiveTurnTimeoutError) Unwrap() error {
	return errOpenAIWSPassthroughActiveTurnTimeout
REDACTED

type openAIWSPassthroughFirstOutputDeadlineState struct {
	armed      bool
	generation uint64
	deadline   openAIWSPassthroughFirstOutputDeadline
REDACTED

type openAIWSPassthroughTurnLifecycle struct {
	mu       sync.Mutex
	inFlight bool
REDACTED

func newOpenAIWSPassthroughTurnLifecycle(inFlight bool) *openAIWSPassthroughTurnLifecycle {
	return &openAIWSPassthroughTurnLifecycle{inFlight: inFlightREDACTED
REDACTED

func (l *openAIWSPassthroughTurnLifecycle) beginResponseCreate(onAccepted func()) bool {
	if l == nil {
		return false
REDACTED
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inFlight {
		return false
REDACTED
	l.inFlight = true
	if onAccepted != nil {
		onAccepted()
REDACTED
	return true
REDACTED

func (l *openAIWSPassthroughTurnLifecycle) cancelResponseCreate() {
	if l == nil {
		return
REDACTED
	l.mu.Lock()
	l.inFlight = false
	l.mu.Unlock()
REDACTED

func (l *openAIWSPassthroughTurnLifecycle) beginTerminalWrite() {
	if l != nil {
		l.mu.Lock()
REDACTED
REDACTED

func (l *openAIWSPassthroughTurnLifecycle) finishTerminalWrite(succeeded bool, onSucceeded func()) {
	if l == nil {
		return
REDACTED
	if succeeded {
		if onSucceeded != nil {
			onSucceeded()
	REDACTED
		l.inFlight = false
REDACTED
	l.mu.Unlock()
REDACTED

type openAIWSPassthroughFirstOutputFrameConn struct {
	inner             openaiwsv2.FrameConn
	resolveDeadline   func(payload []byte) openAIWSPassthroughFirstOutputDeadline
	activeReadTimeout time.Duration

	mu              sync.Mutex
	state           openAIWSPassthroughFirstOutputDeadlineState
	deadlineChanged chan struct{REDACTED
REDACTED

func (c *openAIWSPassthroughFirstOutputFrameConn) ReadFrame(ctx context.Context) (coderws.MessageType, []byte, error) {
	if c == nil || c.inner == nil {
		return coderws.MessageText, nil, errOpenAIWSConnClosed
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED

	type readResult struct {
		msgType coderws.MessageType
		payload []byte
		err     error
REDACTED
	readCtx, cancelRead := context.WithCancel(ctx)
	readResultCh := make(chan readResult, 1)
	go func() {
		msgType, payload, err := c.inner.ReadFrame(readCtx)
		readResultCh <- readResult{msgType: msgType, payload: payload, err: errREDACTED
REDACTED()

	var timer *time.Timer
	var timerCh <-chan time.Time
	resetTimer := func() {
		state := c.deadlineState()
		if timer != nil {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
			REDACTED
		REDACTED
	REDACTED
		if !state.armed || state.deadline.timeout <= 0 {
			timerCh = nil
			return
	REDACTED
		remaining := time.Until(state.deadline.startedAt.Add(state.deadline.timeout))
		if remaining < 0 {
			remaining = 0
	REDACTED
		if timer == nil {
			timer = time.NewTimer(remaining)
	REDACTED else {
			timer.Reset(remaining)
	REDACTED
		timerCh = timer.C
REDACTED
	resetTimer()

	defer func() {
		cancelRead()
		if timer != nil {
			timer.Stop()
	REDACTED
REDACTED()
	for {
		select {
		case result := <-readResultCh:
			if result.err == nil {
				c.observeUpstreamActivity(result.msgType, result.payload)
		REDACTED
			return result.msgType, result.payload, result.err
		case <-c.deadlineChanged:
			resetTimer()
		case <-timerCh:
			state := c.deadlineState()
			if !state.armed || state.deadline.timeout <= 0 || time.Now().Before(state.deadline.startedAt.Add(state.deadline.timeout)) {
				resetTimer()
				continue
		REDACTED
			if ctx.Err() != nil {
				cancelRead()
				<-readResultCh
				return coderws.MessageText, nil, ctx.Err()
		REDACTED
			cancelRead()
			<-readResultCh
			if state.deadline.phase == openAIWSPassthroughDeadlinePhaseActiveRead {
				return coderws.MessageText, nil, &openAIWSPassthroughActiveTurnTimeoutError{REDACTED
		REDACTED
			return coderws.MessageText, nil, &openAIWSPassthroughFirstOutputTimeoutError{deadline: state.deadlineREDACTED
		case <-ctx.Done():
			cancelRead()
			<-readResultCh
			return coderws.MessageText, nil, ctx.Err()
	REDACTED
REDACTED
REDACTED

func (c *openAIWSPassthroughFirstOutputFrameConn) WriteFrame(ctx context.Context, msgType coderws.MessageType, payload []byte) error {
	if c == nil || c.inner == nil {
		return errOpenAIWSConnClosed
REDACTED
	generation := uint64(0)
	if msgType == coderws.MessageText && strings.TrimSpace(gjson.GetBytes(payload, "type").String()) == "response.create" {
		generation = c.armDeadline(payload)
REDACTED
	if err := c.inner.WriteFrame(ctx, msgType, payload); err != nil {
		c.disarmDeadline(generation)
		return err
REDACTED
	return nil
REDACTED

func (c *openAIWSPassthroughFirstOutputFrameConn) Close() error {
	if c == nil || c.inner == nil {
		return nil
REDACTED
	return c.inner.Close()
REDACTED

func (c *openAIWSPassthroughFirstOutputFrameConn) armDeadline(payload []byte) uint64 {
	if c == nil || c.resolveDeadline == nil {
		return 0
REDACTED
	deadline := c.resolveDeadline(payload)
	if deadline.timeout <= 0 {
		return 0
REDACTED
	if deadline.startedAt.IsZero() {
		deadline.startedAt = time.Now()
REDACTED
	deadline.phase = openAIWSPassthroughDeadlinePhaseFirstSemantic
	c.mu.Lock()
	c.state.generation++
	generation := c.state.generation
	c.state.armed = true
	c.state.deadline = deadline
	c.mu.Unlock()
	c.notifyDeadlineChanged()
	return generation
REDACTED

func (c *openAIWSPassthroughFirstOutputFrameConn) observeUpstreamActivity(msgType coderws.MessageType, payload []byte) {
	if c == nil {
		return
REDACTED
	if msgType == coderws.MessageText && openAIWSPassthroughIsTerminalOutput(payload) {
		c.disarmDeadline(0)
		return
REDACTED
	state := c.deadlineState()
	if state.armed && state.deadline.phase == openAIWSPassthroughDeadlinePhaseActiveRead {
		c.armActiveReadDeadline()
		return
REDACTED
	if msgType == coderws.MessageText && openAIWSPassthroughStartsSemanticOutput(payload) {
		c.armActiveReadDeadline()
REDACTED
REDACTED

func (c *openAIWSPassthroughFirstOutputFrameConn) armActiveReadDeadline() {
	if c == nil {
		return
REDACTED
	if c.activeReadTimeout <= 0 {
		c.disarmDeadline(0)
		return
REDACTED
	c.mu.Lock()
	c.state.generation++
	c.state.armed = true
	c.state.deadline = openAIWSPassthroughFirstOutputDeadline{
		timeout:   c.activeReadTimeout,
		startedAt: time.Now(),
		phase:     openAIWSPassthroughDeadlinePhaseActiveRead,
REDACTED
	c.mu.Unlock()
	c.notifyDeadlineChanged()
REDACTED

func (c *openAIWSPassthroughFirstOutputFrameConn) disarmDeadline(generation uint64) {
	if c == nil {
		return
REDACTED
	c.mu.Lock()
	if !c.state.armed || (generation != 0 && generation != c.state.generation) {
		c.mu.Unlock()
		return
REDACTED
	c.state.armed = false
	c.mu.Unlock()
	c.notifyDeadlineChanged()
REDACTED

func (c *openAIWSPassthroughFirstOutputFrameConn) deadlineState() openAIWSPassthroughFirstOutputDeadlineState {
	if c == nil {
		return openAIWSPassthroughFirstOutputDeadlineState{REDACTED
REDACTED
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
REDACTED

func (c *openAIWSPassthroughFirstOutputFrameConn) notifyDeadlineChanged() {
	if c == nil || c.deadlineChanged == nil {
		return
REDACTED
	select {
	case c.deadlineChanged <- struct{REDACTED{REDACTED:
	default:
REDACTED
REDACTED

func openAIWSPassthroughStartsSemanticOutput(payload []byte) bool {
	eventType := strings.TrimSpace(gjson.GetBytes(payload, "type").String())
	switch eventType {
	case "response.completed", "response.done", "response.failed", "response.incomplete", "response.cancelled", "response.canceled":
		return true
	case "", "response.created", "response.in_progress", "response.output_item.added", "response.output_item.done":
		return false
REDACTED
	return strings.Contains(eventType, ".delta") ||
		strings.HasPrefix(eventType, "response.output_text") ||
		strings.HasPrefix(eventType, "response.output")
REDACTED

func openAIWSPassthroughIsTerminalOutput(payload []byte) bool {
	switch strings.TrimSpace(gjson.GetBytes(payload, "type").String()) {
	case "response.completed", "response.done", "response.failed", "response.incomplete", "response.cancelled", "response.canceled":
		return true
	default:
		return false
REDACTED
REDACTED

var _ openaiwsv2.FrameConn = (*openAIWSClientFrameConn)(nil)
var _ openaiwsv2.FrameConn = (*openAIWSPassthroughFirstOutputFrameConn)(nil)

func (c *openAIWSClientFrameConn) ReadFrame(ctx context.Context) (coderws.MessageType, []byte, error) {
	if c == nil || c.conn == nil {
		return coderws.MessageText, nil, errOpenAIWSConnClosed
REDACTED
	controlCtx := ctx
	if c.controlCtx != nil {
		controlCtx = c.controlCtx
REDACTED
	msgType, payload, err := readOpenAIWSClientMessageWithTimeoutStart(
		controlCtx,
		c.conn,
		c.interTurnIdleTimeout,
		coderws.StatusNormalClosure,
		"websocket idle timeout",
		c.interTurnStarted,
		func() bool { return c.waitingForNextTurn.Load() REDACTED,
	)
	return msgType, payload, err
REDACTED

func (c *openAIWSClientFrameConn) markTurnStarted() {
	if c != nil {
		c.waitingForNextTurn.Store(false)
REDACTED
REDACTED

func (c *openAIWSClientFrameConn) markTurnCompleted() {
	if c == nil {
		return
REDACTED
	c.waitingForNextTurn.Store(true)
	select {
	case c.interTurnStarted <- struct{REDACTED{REDACTED:
	default:
REDACTED
REDACTED

func (c *openAIWSClientFrameConn) WriteFrame(ctx context.Context, msgType coderws.MessageType, payload []byte) error {
	if c == nil || c.conn == nil {
		return errOpenAIWSConnClosed
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED
	if msgType == coderws.MessageText {
		if normalized, changed := normalizeCompletedImageGenerationStatus(payload); changed {
			payload = normalized
	REDACTED
		if c.restoreResponseModel != nil {
			payload = c.restoreResponseModel(payload)
	REDACTED
REDACTED
	return c.conn.Write(ctx, msgType, payload)
REDACTED

func (c *openAIWSClientFrameConn) Close() error {
	if c == nil || c.conn == nil {
		return nil
REDACTED
	_ = c.conn.Close(coderws.StatusNormalClosure, "")
	_ = c.conn.CloseNow()
	return nil
REDACTED

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
	if s == nil {
		return errors.New("service is nil")
REDACTED
	if clientConn == nil {
		return errors.New("client websocket is nil")
REDACTED
	if account == nil {
		return errors.New("account is nil")
REDACTED
	if err := validateOpenAIWSBearerToken(account, token); err != nil {
		return err
REDACTED
	if account.IsOpenAIOAuth() && isOpenAIResponsesLiteWebSocketPayload(firstClientMessage) {
		liteFirstMessage, _, liteErr := normalizeOpenAIResponsesLiteToolsPayload(firstClientMessage)
		if liteErr != nil {
			return NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, liteErr.Error(), liteErr)
	REDACTED
		firstClientMessage = liteFirstMessage
REDACTED
	if hooks != nil && (hooks.MaxReasoningEffort != "" || len(hooks.ReasoningEffortMappings) > 0) {
		if capped, changed := ApplyOpenAIReasoningEffortPolicy(firstClientMessage, hooks.MaxReasoningEffort, hooks.ReasoningEffortMappings); changed {
			firstClientMessage = capped
	REDACTED
REDACTED
	requestModel := strings.TrimSpace(gjson.GetBytes(firstClientMessage, "model").String())
	requestPreviousResponseID := strings.TrimSpace(gjson.GetBytes(firstClientMessage, "previous_response_id").String())
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
REDACTED
	if initialRequestModel == "" {
		initialRequestModel = openAIWSPassthroughRequestModelForFrame(firstClientMessage)
REDACTED
	if hooks != nil && hooks.MapRequestModel != nil {
		mappedModel, mapErr := hooks.MapRequestModel(1, initialRequestModel)
		if mapErr != nil {
			return mapErr
	REDACTED
		if mappedModel = strings.TrimSpace(mappedModel); mappedModel != "" {
			firstClientMessage = s.ReplaceModelInBody(firstClientMessage, mappedModel)
	REDACTED
REDACTED
	capturedSessionModel := openAIWSPassthroughPolicyModelForFrame(account, firstClientMessage)
	if capturedSessionModel != "" && capturedSessionModel != strings.TrimSpace(gjson.GetBytes(firstClientMessage, "model").String()) {
		firstClientMessage = s.ReplaceModelInBody(firstClientMessage, capturedSessionModel)
REDACTED
	usageMeta := newOpenAIWSPassthroughUsageMeta(initialRequestModel, firstClientMessage)
	updatedFirst, blocked, policyErr := s.applyOpenAIFastPolicyToWSResponseCreate(ctx, account, capturedSessionModel, firstClientMessage)
	if policyErr != nil {
		return fmt.Errorf("apply openai fast policy on first ws frame: %w", policyErr)
REDACTED
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
	REDACTED
		return NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, blocked.Message, blocked)
REDACTED
	firstClientMessage = updatedFirst

	// 在 policy filter 之后再提取 service_tier / reasoning_effort 用于
	// usage 上报：filter
	// 命中时 service_tier 已经从 firstClientMessage 中删除，billing 应当
	// 反映上游实际处理的 tier（nil = default），而不是用户最初请求的
	// "priority"。HTTP 入口（line ~2728 extractOpenAIServiceTier(reqBody)）
	// 与 WS ingress（openai_ws_forwarder.go:2991 取自 payload）的语义一致。
	//
	// 多轮 passthrough：OpenAI Realtime / Responses WS 协议允许客户端在
	// 同一连接的不同 response.create 帧上发送不同 service_tier（参考
	// codex-rs/core/src/client.rs build_responses_request 每次重新填值）。
	// 因此使用 atomic.Pointer[string] 在 filter（runClientToUpstream
	// goroutine）和 OnTurnComplete / final result（runUpstreamToClient
	// goroutine）之间同步当前 turn 的 usage metadata。
	usageMeta.initFromFirstFrame(firstClientMessage, capturedSessionModel)
	promptCacheKey := strings.TrimSpace(gjson.GetBytes(firstClientMessage, "prompt_cache_key").String())

	wsURL, err := s.buildOpenAIResponsesWSURL(account)
	if err != nil {
		return fmt.Errorf("build ws url: %w", err)
REDACTED
	wsHost := "-"
	wsPath := "-"
	if parsedURL, parseErr := url.Parse(wsURL); parseErr == nil && parsedURL != nil {
		wsHost = normalizeOpenAIWSLogValue(parsedURL.Host)
		wsPath = normalizeOpenAIWSLogValue(parsedURL.Path)
REDACTED
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
REDACTED
	if s.cfg != nil && s.cfg.Gateway.ForceCodexCLI {
		isCodexCLI = true
REDACTED
	turnState := ""
	turnMetadata := ""
	if c != nil {
		turnState = strings.TrimSpace(c.GetHeader(openAIWSTurnStateHeader))
		turnMetadata = strings.TrimSpace(c.GetHeader(openAIWSTurnMetadataHeader))
REDACTED
	headers, _, buildHdrErr := s.buildOpenAIWSHeaders(ctx, c, account, token, wsDecision, isCodexCLI, turnState, turnMetadata, promptCacheKey)
	if buildHdrErr != nil {
		return fmt.Errorf("build ws headers: %w", buildHdrErr)
REDACTED
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED

	dialer := s.getOpenAIWSPassthroughDialer()
	if dialer == nil {
		return errors.New("openai ws passthrough dialer is nil")
REDACTED

	agentTaskRecoveryTried := false
	var upstreamConn openAIWSClientConn
	statusCode := 0
	var handshakeHeaders http.Header
	for {
		headers, err = s.refreshOpenAIAgentIdentityHeaders(ctx, account, headers)
		if err != nil {
			return fmt.Errorf("refresh ws authentication headers: %w", err)
	REDACTED
		dialCtx, cancelDial := context.WithTimeout(ctx, s.openAIWSDialTimeout())
		upstreamConn, statusCode, handshakeHeaders, err = dialer.Dial(dialCtx, wsURL, headers, proxyURL)
		cancelDial()
		if err == nil {
			break
	REDACTED
		var handshakeErr *openAIWSHandshakeError
		responseBody := []byte(nil)
		if errors.As(err, &handshakeErr) && handshakeErr != nil {
			responseBody = handshakeErr.Body
	REDACTED
		dialErr := &openAIWSDialError{StatusCode: statusCode, ResponseHeaders: cloneHeader(handshakeHeaders), ResponseBody: responseBody, Err: errREDACTED
		if s.isAgentIdentityAccount(ctx, account) && isAgentIdentityTaskInvalidWSDialError(dialErr) && !agentTaskRecoveryTried {
			agentTaskRecoveryTried = true
			if recoveryErr := s.recoverAgentIdentityTask(ctx, account, account.GetCredential("task_id")); recoveryErr != nil {
				return fmt.Errorf("agent identity task recovery failed: %w", recoveryErr)
		REDACTED
			continue
	REDACTED
		logOpenAIWSV2Passthrough(
			"relay_dial_failed account_id=%d status_code=%d err=%s",
			account.ID,
			statusCode,
			truncateOpenAIWSLogValue(err.Error(), openAIWSLogValueMaxLen),
		)
		s.handleOpenAIWSDialTransientFailure(ctx, account, capturedSessionModel, dialErr)
		if statusCode == http.StatusTooManyRequests {
			s.persistOpenAIWSRateLimitSignal(ctx, account, handshakeHeaders, nil, "rate_limit_exceeded", "rate_limit_error", strings.TrimSpace(err.Error()))
			return &UpstreamFailoverError{
				StatusCode:      http.StatusTooManyRequests,
				ResponseHeaders: cloneHeader(handshakeHeaders),
		REDACTED
	REDACTED
		return s.mapOpenAIWSPassthroughDialError(err, statusCode, handshakeHeaders)
REDACTED
	defer func() {
		_ = upstreamConn.Close()
REDACTED()
	logOpenAIWSV2Passthrough(
		"relay_dial_ok account_id=%d status_code=%d upstream_request_id=%s",
		account.ID,
		statusCode,
		openAIWSHeaderValueForLog(handshakeHeaders, "x-request-id"),
	)

	upstreamFrameConn, ok := upstreamConn.(openaiwsv2.FrameConn)
	if !ok {
		return errors.New("openai ws passthrough upstream connection does not support frame relay")
REDACTED
	relayUpstreamFrameConn := &openAIWSPassthroughFirstOutputFrameConn{
		inner:             upstreamFrameConn,
		activeReadTimeout: s.openAIWSPassthroughIdleTimeout(),
		deadlineChanged:   make(chan struct{REDACTED, 1),
		resolveDeadline: func(payload []byte) openAIWSPassthroughFirstOutputDeadline {
			reasoningEffort := ""
			if current := usageMeta.reasoningEffort.Load(); current != nil {
				reasoningEffort = *current
		REDACTED
			timeout := s.openAIFirstOutputTimeout(reasoningEffort)
			if timeout <= 0 {
				timeout = s.openAIWSPassthroughIdleTimeout()
		REDACTED
			model := openAIWSPassthroughRequestModelForFrame(payload)
			if model == "" {
				model = usageMeta.requestModelForFrame(payload)
		REDACTED
			if model == "" {
				model = requestModel
		REDACTED
			return openAIWSPassthroughFirstOutputDeadline{
				timeout:         timeout,
				startedAt:       time.Now(),
				requestModel:    model,
				reasoningEffort: reasoningEffort,
		REDACTED
	REDACTED,
REDACTED

	completedTurns := atomic.Int32{REDACTED
	turnLifecycle := newOpenAIWSPassthroughTurnLifecycle(true)
	clientFrameConn := &openAIWSClientFrameConn{
		conn:                 clientConn,
		controlCtx:           ctx,
		interTurnIdleTimeout: s.openAIWSIngressInterTurnIdleTimeout(),
		interTurnStarted:     make(chan struct{REDACTED, 1),
		restoreResponseModel: func(payload []byte) []byte {
			eventType := strings.TrimSpace(gjson.GetBytes(payload, "type").String())
			if !openAIWSEventMayContainModel(eventType) {
				return payload
		REDACTED
			requestModel, upstreamModel := usageMeta.turnModels("")
			return replaceOpenAIWSMessageModel(payload, upstreamModel, requestModel)
	REDACTED,
REDACTED
	policyClientConn := &openAIWSPolicyEnforcingFrameConn{
		inner: clientFrameConn,
		// 注意线程安全：filter 仅在 runClientToUpstream 这一条
		// goroutine 中被调用（passthrough_relay.go: ReadFrame loop），
		// capturedSessionModel 的读写都发生在该 goroutine 内，因此无需
		// 加锁/原子化。
		filter: func(msgType coderws.MessageType, payload []byte) (out []byte, blocked *OpenAIFastBlockedError, filterErr error) {
			if msgType != coderws.MessageText {
				return payload, nil, nil
		REDACTED
			eventType := strings.TrimSpace(gjson.GetBytes(payload, "type").String())
			isResponseCreate := eventType == "response.create"
			acceptedTurn := false
			if isResponseCreate {
				if !turnLifecycle.beginResponseCreate(clientFrameConn.markTurnStarted) {
					err := errors.New("overlapping response.create is not supported")
					return payload, nil, NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, err.Error(), err)
			REDACTED
				defer func() {
					if !acceptedTurn {
						turnLifecycle.cancelResponseCreate()
				REDACTED
			REDACTED()
		REDACTED
			if isResponseCreate {
				if account.IsOpenAIOAuth() && isOpenAIResponsesLiteWebSocketPayload(payload) {
					litePayload, _, liteErr := normalizeOpenAIResponsesLiteToolsPayload(payload)
					if liteErr != nil {
						return payload, nil, NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, liteErr.Error(), liteErr)
				REDACTED
					payload = litePayload
			REDACTED
				if hooks != nil && (hooks.MaxReasoningEffort != "" || len(hooks.ReasoningEffortMappings) > 0) {
					if capped, changed := ApplyOpenAIReasoningEffortPolicy(payload, hooks.MaxReasoningEffort, hooks.ReasoningEffortMappings); changed {
						payload = capped
				REDACTED
			REDACTED
		REDACTED
			turnNo := int(completedTurns.Load()) + 1
			if turnNo < 2 {
				turnNo = 2
		REDACTED
			requestModelForThisFrame := ""
			if isResponseCreate {
				requestModelForThisFrame = usageMeta.requestModelForFrame(payload)
				if requestModelForThisFrame == "" {
					requestModelForThisFrame = capturedSessionModel
			REDACTED
				if hooks != nil && hooks.BeforeRequest != nil {
					if err := hooks.BeforeRequest(turnNo, payload, requestModelForThisFrame); err != nil {
						return payload, nil, err
				REDACTED
			REDACTED
				if hooks != nil && hooks.MapRequestModel != nil {
					upstreamModel, err := hooks.MapRequestModel(turnNo, requestModelForThisFrame)
					if err != nil {
						return payload, nil, err
				REDACTED
					if upstreamModel = strings.TrimSpace(upstreamModel); upstreamModel != "" {
						payload = s.ReplaceModelInBody(payload, upstreamModel)
				REDACTED
			REDACTED
		REDACTED
			// 在评估策略前先刷新 capturedSessionModel：客户端可能通过
			// session.update 修改 session-level model（Realtime /
			// Responses WS 协议允许），如果不刷新就会出现
			// "首帧 model=gpt-4o（pass）→ session.update 改成 gpt-5.5
			// → 不带 model 的 response.create fallback 到 gpt-4o" 的
			// 绕过路径。这里只看 session.update 事件中的 session.model
			// 字段，response.create 自己的 model 仍然由其本帧字段决定。
			if updated := openAIWSPassthroughPolicyModelFromSessionFrame(account, payload); updated != "" {
				capturedSessionModel = updated
		REDACTED
			usageMeta.updateSessionRequestModel(payload)
			if requestModelForThisFrame == "" {
				requestModelForThisFrame = usageMeta.requestModelForFrame(payload)
		REDACTED
			// Per-frame model first; if the client omits "model" on a
			// follow-up frame (legal in Realtime), fall back to the
			// session-level model captured from the first frame so the
			// model whitelist still resolves. An empty model would miss
			// any whitelist and silently fall back to pass.
			model := openAIWSPassthroughPolicyModelForFrame(account, payload)
			if model == "" {
				model = capturedSessionModel
		REDACTED
			if isResponseCreate && model != "" && model != strings.TrimSpace(gjson.GetBytes(payload, "model").String()) {
				payload = s.ReplaceModelInBody(payload, model)
		REDACTED
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
				acceptedTurn = true
		REDACTED
			return out, blocked, policyErr
	REDACTED,
		onBlock: func(blocked *OpenAIFastBlockedError) {
			MarkOpsClientBusinessLimited(c, OpsClientBusinessLimitedReasonLocalPolicyDenied)
			// See note above on Conn.Write being synchronous w.r.t. flush;
			// no explicit flush is required to ensure the error event lands
			// before the close frame.
			eventBytes := buildOpenAIFastPolicyBlockedWSEvent(blocked)
			if eventBytes == nil {
				return
		REDACTED
			writeCtx, cancel := context.WithTimeout(ctx, s.openAIWSWriteTimeout())
			_ = clientConn.Write(writeCtx, coderws.MessageText, eventBytes)
			cancel()
	REDACTED,
REDACTED
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
REDACTED
	upstreamFirstMessageSent = true

	readNextClientFrame := func(readCtx context.Context, conn openaiwsv2.FrameConn) (coderws.MessageType, []byte, error) {
		for {
			msgType, payload, readErr := conn.ReadFrame(readCtx)
			if readErr != nil {
				return msgType, payload, readErr
		REDACTED
			if msgType == coderws.MessageText && strings.TrimSpace(gjson.GetBytes(payload, "type").String()) == "response.create" {
				return msgType, payload, nil
		REDACTED
			if writeErr := upstreamFrameConn.WriteFrame(readCtx, msgType, payload); writeErr != nil {
				return msgType, payload, writeErr
		REDACTED
	REDACTED
REDACTED

	relayResult, relayExit := openaiwsv2.RunEntry(openaiwsv2.EntryInput{
		Ctx:                ctx,
		ClientConn:         policyClientConn,
		UpstreamConn:       relayUpstreamFrameConn,
		FirstClientMessage: firstClientMessage,
		Options: openaiwsv2.RelayOptions{
			WriteTimeout: s.openAIWSWriteTimeout(),
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
		REDACTED,
			OnTurnComplete: func(turn openaiwsv2.RelayTurnResult) {
				turnNo := int(completedTurns.Add(1))
				turnRequestModel, turnUpstreamModel := usageMeta.turnModels(turn.RequestModel)
				turnResult := &OpenAIForwardResult{
					RequestID: turn.RequestID,
					Usage: OpenAIUsage{
						InputTokens:              turn.Usage.InputTokens,
						OutputTokens:             turn.Usage.OutputTokens,
						CacheCreationInputTokens: turn.Usage.CacheCreationInputTokens,
						CacheReadInputTokens:     turn.Usage.CacheReadInputTokens,
						ImageOutputTokens:        turn.Usage.ImageOutputTokens,
				REDACTED,
					Model:                 turnRequestModel,
					UpstreamModel:         openAIWSDifferentModel(turnRequestModel, turnUpstreamModel),
					ServiceTier:           usageMeta.serviceTier.Load(),
					ReasoningEffort:       usageMeta.reasoningEffort.Load(),
					Stream:                true,
					OpenAIWSMode:          true,
					UpstreamTerminalEvent: normalizeOpenAIWSTerminalEvent(turn.TerminalEventType),
					ResponseHeaders:       cloneHeader(handshakeHeaders),
					Duration:              turn.Duration,
					FirstTokenMs:          turn.FirstTokenMs,
			REDACTED
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
			REDACTED
		REDACTED,
			BeforeClientWrite: func(msgType coderws.MessageType, payload []byte) {
				if msgType == coderws.MessageText && openAIWSPassthroughIsTerminalOutput(payload) {
					turnLifecycle.beginTerminalWrite()
			REDACTED
		REDACTED,
			AfterClientWrite: func(msgType coderws.MessageType, payload []byte, writeErr error) {
				if msgType == coderws.MessageText && openAIWSPassthroughIsTerminalOutput(payload) {
					turnLifecycle.finishTerminalWrite(writeErr == nil, clientFrameConn.markTurnCompleted)
			REDACTED
		REDACTED,
			BeforeRelayCancel: func(exit openaiwsv2.RelayExit) {
				if context.Cause(ctx) != nil {
					return
			REDACTED
				status, reason, ok := openAIWSPassthroughRelayClientClose(exit, int(completedTurns.Load()))
				if !ok {
					return
			REDACTED
				_ = clientConn.Close(status, reason)
				_ = clientConn.CloseNow()
		REDACTED,
			BeforeWriteClient: func(msgType coderws.MessageType, payload []byte, wroteDownstream bool) error {
				if msgType != coderws.MessageText {
					return nil
			REDACTED
				eventType, _, _ := parseOpenAIWSEventEnvelope(payload)
				if isOpenAIWSTerminalEvent(eventType) {
					s.handleOpenAIWSTerminalTransientFailure(ctx, account, capturedSessionModel, handshakeHeaders, payload)
			REDACTED
				if eventType == "error" {
					s.handleOpenAIWSErrorEventTransientFailure(ctx, account, capturedSessionModel, handshakeHeaders, payload)
			REDACTED
				if wroteDownstream || eventType != "error" {
					return nil
			REDACTED
				errCodeRaw, errTypeRaw, errMsgRaw := parseOpenAIWSErrorEventFields(payload)
				if !isOpenAIWSRateLimitError(errCodeRaw, errTypeRaw, errMsgRaw) {
					return nil
			REDACTED
				s.persistOpenAIWSRateLimitSignal(ctx, account, handshakeHeaders, payload, errCodeRaw, errTypeRaw, errMsgRaw)
				logOpenAIWSV2Passthrough(
					"relay_rate_limit_failover account_id=%d err_code=%s err_type=%s err_message=%s",
					account.ID,
					truncateOpenAIWSLogValue(errCodeRaw, openAIWSLogValueMaxLen),
					truncateOpenAIWSLogValue(errTypeRaw, openAIWSLogValueMaxLen),
					truncateOpenAIWSLogValue(errMsgRaw, openAIWSLogValueMaxLen),
				)
				return &UpstreamFailoverError{
					StatusCode:      http.StatusTooManyRequests,
					ResponseBody:    append([]byte(nil), payload...),
					ResponseHeaders: cloneHeader(handshakeHeaders),
			REDACTED
		REDACTED,
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
		REDACTED,
	REDACTED,
REDACTED)
	if cause := context.Cause(ctx); cause != nil {
		status := coderws.StatusGoingAway
		reason := "websocket request canceled"
		if errors.Is(cause, ErrOpenAIWSIngressLeaseLost) {
			status = coderws.StatusTryAgainLater
			reason = "websocket ingress capacity lease lost; please reconnect"
	REDACTED
		_ = clientConn.Close(status, reason)
		_ = clientConn.CloseNow()
		return NewOpenAIWSClientCloseError(status, reason, cause)
REDACTED

	resultRequestModel, resultUpstreamModel := usageMeta.turnModels(relayResult.RequestModel)
	result := &OpenAIForwardResult{
		RequestID: relayResult.RequestID,
		Usage: OpenAIUsage{
			InputTokens:              relayResult.Usage.InputTokens,
			OutputTokens:             relayResult.Usage.OutputTokens,
			CacheCreationInputTokens: relayResult.Usage.CacheCreationInputTokens,
			CacheReadInputTokens:     relayResult.Usage.CacheReadInputTokens,
			ImageOutputTokens:        relayResult.Usage.ImageOutputTokens,
	REDACTED,
		Model:                 resultRequestModel,
		UpstreamModel:         openAIWSDifferentModel(resultRequestModel, resultUpstreamModel),
		ServiceTier:           usageMeta.serviceTier.Load(),
		ReasoningEffort:       usageMeta.reasoningEffort.Load(),
		Stream:                true,
		OpenAIWSMode:          true,
		UpstreamTerminalEvent: normalizeOpenAIWSTerminalEvent(relayResult.TerminalEventType),
		ResponseHeaders:       cloneHeader(handshakeHeaders),
		Duration:              relayResult.Duration,
		FirstTokenMs:          relayResult.FirstTokenMs,
REDACTED

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
			hooks.AfterTurn(1, result, nil)
	REDACTED
		return nil
REDACTED
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
		failoverErr := s.newOpenAIFirstOutputTimeoutError(
			ctx,
			c,
			account,
			deadline.startedAt,
			deadline.requestModel,
			deadline.reasoningEffort,
			deadline.timeout,
			"websocket_first_semantic_output",
			handshakeHeaders,
		)
		if turnCount == 0 && !relayExit.WroteDownstream {
			relayErr = failoverErr
	REDACTED else {
			// The handler only retains the initial response.create across
			// account attempts. Replaying it after a later-turn timeout would
			// duplicate the first turn, so later turns end the client session.
			relayErr = NewOpenAIWSClientCloseError(
				coderws.StatusGoingAway,
				"upstream produced no semantic output; please reconnect",
				firstOutputTimeoutErr,
			)
	REDACTED
REDACTED
	var activeTurnTimeoutErr *openAIWSPassthroughActiveTurnTimeoutError
	if errors.As(relayErr, &activeTurnTimeoutErr) {
		relayErr = NewOpenAIWSClientCloseError(
			coderws.StatusGoingAway,
			"upstream websocket read timeout; please reconnect",
			activeTurnTimeoutErr,
		)
REDACTED
	if relayExit.Stage == "idle_timeout" {
		relayErr = NewOpenAIWSClientCloseError(
			coderws.StatusPolicyViolation,
			"client websocket idle timeout",
			relayErr,
		)
REDACTED
	turnErr := wrapOpenAIWSIngressTurnError(
		relayExit.Stage,
		relayErr,
		relayExit.WroteDownstream,
	)
	if hooks != nil && hooks.AfterTurn != nil {
		hooks.AfterTurn(turnCount+1, nil, turnErr)
REDACTED
	return turnErr
REDACTED

func openAIWSPassthroughRelayClientClose(exit openaiwsv2.RelayExit, completedTurns int) (coderws.StatusCode, string, bool) {
	var closeErr *OpenAIWSClientCloseError
	if errors.As(exit.Err, &closeErr) {
		return closeErr.StatusCode(), closeErr.Reason(), true
REDACTED
	var activeTurnTimeoutErr *openAIWSPassthroughActiveTurnTimeoutError
	if errors.As(exit.Err, &activeTurnTimeoutErr) {
		return coderws.StatusGoingAway, "upstream websocket read timeout; please reconnect", true
REDACTED
	var firstOutputTimeoutErr *openAIWSPassthroughFirstOutputTimeoutError
	if errors.As(exit.Err, &firstOutputTimeoutErr) {
		if completedTurns > 0 || exit.WroteDownstream {
			return coderws.StatusGoingAway, "upstream produced no semantic output; please reconnect", true
	REDACTED
		return 0, "", false
REDACTED
	if !exit.Graceful && exit.Stage == "read_upstream" {
		return coderws.StatusInternalError, "upstream websocket proxy failed", true
REDACTED
	return 0, "", false
REDACTED

func (s *OpenAIGatewayService) mapOpenAIWSPassthroughDialError(
	err error,
	statusCode int,
	handshakeHeaders http.Header,
) error {
	if err == nil {
		return nil
REDACTED
	wrappedErr := err
	var dialErr *openAIWSDialError
	if !errors.As(err, &dialErr) {
		var handshakeErr *openAIWSHandshakeError
		var responseBody []byte
		if errors.As(err, &handshakeErr) && handshakeErr != nil {
			responseBody = append([]byte(nil), handshakeErr.Body...)
	REDACTED
		wrappedErr = &openAIWSDialError{
			StatusCode:      statusCode,
			ResponseHeaders: cloneHeader(handshakeHeaders),
			ResponseBody:    responseBody,
			Err:             err,
	REDACTED
REDACTED

	if errors.Is(err, context.Canceled) {
		return err
REDACTED
	if errors.Is(err, context.DeadlineExceeded) {
		return NewOpenAIWSClientCloseError(
			coderws.StatusTryAgainLater,
			"upstream websocket connect timeout",
			wrappedErr,
		)
REDACTED
	if statusCode == http.StatusTooManyRequests {
		return NewOpenAIWSClientCloseError(
			coderws.StatusTryAgainLater,
			"upstream websocket is busy, please retry later",
			wrappedErr,
		)
REDACTED
	if statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
		return NewOpenAIWSClientCloseError(
			coderws.StatusPolicyViolation,
			"upstream websocket authentication failed",
			wrappedErr,
		)
REDACTED
	if statusCode >= http.StatusBadRequest && statusCode < http.StatusInternalServerError {
		return NewOpenAIWSClientCloseError(
			coderws.StatusPolicyViolation,
			"upstream websocket handshake rejected",
			wrappedErr,
		)
REDACTED
	return fmt.Errorf("openai ws passthrough dial: %w", wrappedErr)
REDACTED

func openaiwsv2RelayMessageTypeName(msgType coderws.MessageType) string {
	switch msgType {
	case coderws.MessageText:
		return "text"
	case coderws.MessageBinary:
		return "binary"
	default:
		return fmt.Sprintf("unknown(%d)", msgType)
REDACTED
REDACTED

func relayErrorText(err error) string {
	if err == nil {
		return ""
REDACTED
	return err.Error()
REDACTED

func openAIWSFirstTokenMsForLog(firstTokenMs *int) int {
	if firstTokenMs == nil {
		return -1
REDACTED
	return *firstTokenMs
REDACTED

func logOpenAIWSV2Passthrough(format string, args ...any) {
	logger.LegacyPrintf(
		"service.openai_ws_v2",
		"[OpenAI WS v2 passthrough] %s "+format,
		append([]any{openaiWSV2PassthroughModeFieldsREDACTED, args...)...,
	)
REDACTED
