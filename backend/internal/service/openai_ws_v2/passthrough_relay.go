package openai_ws_v2

import (
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/tidwall/gjson"
)

type FrameConn interface {
	ReadFrame(ctx context.Context) (coderws.MessageType, []byte, error)
	WriteFrame(ctx context.Context, msgType coderws.MessageType, payload []byte) error
	Close() error
REDACTED

type Usage struct {
	InputTokens              int
	OutputTokens             int
	CacheCreationInputTokens int
	CacheReadInputTokens     int
	ImageOutputTokens        int
REDACTED

type RelayResult struct {
	RequestModel            string
	Usage                   Usage
	RequestID               string
	TerminalEventType       string
	FirstTokenMs            *int
	Duration                time.Duration
	ClientToUpstreamFrames  int64
	UpstreamToClientFrames  int64
	DroppedDownstreamFrames int64
REDACTED

type RelayTurnResult struct {
	RequestModel      string
	Usage             Usage
	RequestID         string
	TerminalEventType string
	Duration          time.Duration
	FirstTokenMs      *int
REDACTED

type RelayExit struct {
	Stage           string
	Err             error
	Graceful        bool
	WroteDownstream bool
REDACTED

type RelayOptions struct {
	WriteTimeout                    time.Duration
	IdleTimeout                     time.Duration
	UpstreamDrainTimeout            time.Duration
	FirstMessageType                coderws.MessageType
	FirstMessageSent                bool
	StartClientAfterFirstDownstream bool
	OnUsageParseFailure             func(eventType string, usageRaw string)
	OnTurnComplete                  func(turn RelayTurnResult)
	BeforeWriteClient               func(msgType coderws.MessageType, payload []byte, wroteDownstream bool) error
	BeforeClientWrite               func(msgType coderws.MessageType, payload []byte)
	AfterClientWrite                func(msgType coderws.MessageType, payload []byte, writeErr error)
	BeforeRelayCancel               func(exit RelayExit)
	ReadClientFrame                 func(ctx context.Context, clientConn FrameConn) (coderws.MessageType, []byte, error)
	OnTrace                         func(event RelayTraceEvent)
	Now                             func() time.Time
REDACTED

type RelayTraceEvent struct {
	Stage           string
	Direction       string
	MessageType     string
	PayloadBytes    int
	Graceful        bool
	WroteDownstream bool
	Error           string
REDACTED

type relayState struct {
	usage             Usage
	requestModel      string
	lastResponseID    string
	terminalEventType string
	firstTokenMs      *int
	turnTimingByID    map[string]*relayTurnTiming
	activeTurn        *relayTurnTiming
REDACTED

type relayExitSignal struct {
	stage           string
	err             error
	graceful        bool
	wroteDownstream bool
REDACTED

type observedUpstreamEvent struct {
	terminal   bool
	eventType  string
	responseID string
	usage      Usage
	duration   time.Duration
	firstToken *int
REDACTED

type relayTurnTiming struct {
	startAt      time.Time
	firstTokenMs *int
REDACTED

func Relay(
	ctx context.Context,
	clientConn FrameConn,
	upstreamConn FrameConn,
	firstClientMessage []byte,
	options RelayOptions,
) (RelayResult, *RelayExit) {
	result := RelayResult{RequestModel: strings.TrimSpace(gjson.GetBytes(firstClientMessage, "model").String())REDACTED
	if clientConn == nil || upstreamConn == nil {
		return result, &RelayExit{Stage: "relay_init", Err: errors.New("relay connection is nil")REDACTED
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED

	nowFn := options.Now
	if nowFn == nil {
		nowFn = time.Now
REDACTED
	writeTimeout := options.WriteTimeout
	if writeTimeout <= 0 {
		writeTimeout = 2 * time.Minute
REDACTED
	drainTimeout := options.UpstreamDrainTimeout
	if drainTimeout <= 0 {
		drainTimeout = 1200 * time.Millisecond
REDACTED
	firstMessageType := options.FirstMessageType
	if firstMessageType != coderws.MessageBinary {
		firstMessageType = coderws.MessageText
REDACTED
	startAt := nowFn()
	state := &relayState{requestModel: result.RequestModelREDACTED
	onTrace := options.OnTrace

	relayCtx, relayCancel := context.WithCancel(ctx)
	defer relayCancel()

	lastActivity := atomic.Int64{REDACTED
	lastActivity.Store(nowFn().UnixNano())
	markActivity := func() {
		lastActivity.Store(nowFn().UnixNano())
REDACTED

	writeUpstream := func(msgType coderws.MessageType, payload []byte) error {
		writeCtx, cancel := context.WithTimeout(relayCtx, writeTimeout)
		defer cancel()
		return upstreamConn.WriteFrame(writeCtx, msgType, payload)
REDACTED
	writeClient := func(msgType coderws.MessageType, payload []byte) error {
		writeCtx, cancel := context.WithTimeout(relayCtx, writeTimeout)
		defer cancel()
		return clientConn.WriteFrame(writeCtx, msgType, payload)
REDACTED

	clientToUpstreamFrames := &atomic.Int64{REDACTED
	upstreamToClientFrames := &atomic.Int64{REDACTED
	droppedDownstreamFrames := &atomic.Int64{REDACTED
	emitRelayTrace(onTrace, RelayTraceEvent{
		Stage:        "relay_start",
		PayloadBytes: len(firstClientMessage),
		MessageType:  relayMessageTypeString(firstMessageType),
REDACTED)

	if options.FirstMessageSent {
		emitRelayTrace(onTrace, RelayTraceEvent{
			Stage:        "write_first_message_skipped",
			Direction:    "client_to_upstream",
			MessageType:  relayMessageTypeString(firstMessageType),
			PayloadBytes: len(firstClientMessage),
	REDACTED)
REDACTED else {
		if err := writeUpstream(firstMessageType, firstClientMessage); err != nil {
			result.Duration = nowFn().Sub(startAt)
			emitRelayTrace(onTrace, RelayTraceEvent{
				Stage:        "write_first_message_failed",
				Direction:    "client_to_upstream",
				MessageType:  relayMessageTypeString(firstMessageType),
				PayloadBytes: len(firstClientMessage),
				Error:        err.Error(),
		REDACTED)
			return result, &RelayExit{Stage: "write_upstream", Err: errREDACTED
	REDACTED
		emitRelayTrace(onTrace, RelayTraceEvent{
			Stage:        "write_first_message_ok",
			Direction:    "client_to_upstream",
			MessageType:  relayMessageTypeString(firstMessageType),
			PayloadBytes: len(firstClientMessage),
	REDACTED)
REDACTED
	clientToUpstreamFrames.Add(1)
	markActivity()

	exitCh := make(chan relayExitSignal, 3)
	dropDownstreamWrites := atomic.Bool{REDACTED
	clientReaderStarted := atomic.Bool{REDACTED
	startClientReader := func() {
		if !clientReaderStarted.CompareAndSwap(false, true) {
			return
	REDACTED
		go runClientToUpstream(relayCtx, clientConn, options.ReadClientFrame, writeUpstream, markActivity, clientToUpstreamFrames, onTrace, exitCh)
REDACTED
	if !options.StartClientAfterFirstDownstream {
		startClientReader()
REDACTED
	go runUpstreamToClient(
		relayCtx,
		upstreamConn,
		writeClient,
		startAt,
		nowFn,
		state,
		options.OnUsageParseFailure,
		options.OnTurnComplete,
		options.BeforeWriteClient,
		options.BeforeClientWrite,
		options.AfterClientWrite,
		func(msgType coderws.MessageType, payload []byte) {
			if options.StartClientAfterFirstDownstream {
				startClientReader()
		REDACTED
	REDACTED,
		&dropDownstreamWrites,
		upstreamToClientFrames,
		droppedDownstreamFrames,
		markActivity,
		onTrace,
		exitCh,
	)
	go runIdleWatchdog(relayCtx, nowFn, options.IdleTimeout, &lastActivity, onTrace, exitCh)

	firstExit := <-exitCh
	// An outer ingress cancellation is a control-plane close, not a graceful
	// upstream disconnect. Leave the client connection open here so the
	// adapter can emit the precise lease/request close code. Internal
	// relayCancel does not cancel ctx and therefore does not take this path.
	if ctx.Err() != nil {
		firstExit.graceful = false
REDACTED
	emitRelayTrace(onTrace, RelayTraceEvent{
		Stage:           "first_exit",
		Direction:       relayDirectionFromStage(firstExit.stage),
		Graceful:        firstExit.graceful,
		WroteDownstream: firstExit.wroteDownstream,
		Error:           relayErrorString(firstExit.err),
REDACTED)
	if options.BeforeRelayCancel != nil {
		options.BeforeRelayCancel(RelayExit{
			Stage:           firstExit.stage,
			Err:             firstExit.err,
			Graceful:        firstExit.graceful,
			WroteDownstream: firstExit.wroteDownstream,
	REDACTED)
REDACTED
	combinedWroteDownstream := firstExit.wroteDownstream
	secondExit := relayExitSignal{graceful: trueREDACTED
	hasSecondExit := false

	// 客户端断开后尽力继续读取上游短窗口，捕获延迟 usage/terminal 事件用于计费。
	if firstExit.stage == "read_client" && firstExit.graceful {
		dropDownstreamWrites.Store(true)
		secondExit, hasSecondExit = waitRelayExit(exitCh, drainTimeout)
REDACTED else {
		relayCancel()
		_ = upstreamConn.Close()
		if clientReaderStarted.Load() {
			secondExit, hasSecondExit = waitRelayExit(exitCh, 200*time.Millisecond)
	REDACTED
REDACTED
	if hasSecondExit {
		combinedWroteDownstream = combinedWroteDownstream || secondExit.wroteDownstream
		emitRelayTrace(onTrace, RelayTraceEvent{
			Stage:           "second_exit",
			Direction:       relayDirectionFromStage(secondExit.stage),
			Graceful:        secondExit.graceful,
			WroteDownstream: secondExit.wroteDownstream,
			Error:           relayErrorString(secondExit.err),
	REDACTED)
REDACTED

	relayCancel()
	_ = upstreamConn.Close()

	enrichResult(&result, state, nowFn().Sub(startAt))
	result.ClientToUpstreamFrames = clientToUpstreamFrames.Load()
	result.UpstreamToClientFrames = upstreamToClientFrames.Load()
	result.DroppedDownstreamFrames = droppedDownstreamFrames.Load()
	if options.FirstMessageSent && firstExit.stage == "read_client" && firstExit.graceful {
		emitRelayTrace(onTrace, RelayTraceEvent{
			Stage:           "relay_client_closed",
			Graceful:        true,
			WroteDownstream: combinedWroteDownstream,
	REDACTED)
		return result, nil
REDACTED
	if firstExit.stage == "read_client" && firstExit.graceful {
		stage := "client_disconnected"
		exitErr := firstExit.err
		if hasSecondExit && !secondExit.graceful {
			stage = secondExit.stage
			exitErr = secondExit.err
	REDACTED
		if exitErr == nil {
			exitErr = io.EOF
	REDACTED
		emitRelayTrace(onTrace, RelayTraceEvent{
			Stage:           "relay_exit",
			Direction:       relayDirectionFromStage(stage),
			Graceful:        false,
			WroteDownstream: combinedWroteDownstream,
			Error:           relayErrorString(exitErr),
	REDACTED)
		return result, &RelayExit{
			Stage:           stage,
			Err:             exitErr,
			WroteDownstream: combinedWroteDownstream,
	REDACTED
REDACTED
	if firstExit.graceful && (!hasSecondExit || secondExit.graceful) {
		emitRelayTrace(onTrace, RelayTraceEvent{
			Stage:           "relay_complete",
			Graceful:        true,
			WroteDownstream: combinedWroteDownstream,
	REDACTED)
		_ = clientConn.Close()
		return result, nil
REDACTED
	if !firstExit.graceful {
		emitRelayTrace(onTrace, RelayTraceEvent{
			Stage:           "relay_exit",
			Direction:       relayDirectionFromStage(firstExit.stage),
			Graceful:        false,
			WroteDownstream: combinedWroteDownstream,
			Error:           relayErrorString(firstExit.err),
	REDACTED)
		return result, &RelayExit{
			Stage:           firstExit.stage,
			Err:             firstExit.err,
			WroteDownstream: combinedWroteDownstream,
	REDACTED
REDACTED
	if hasSecondExit && !secondExit.graceful {
		emitRelayTrace(onTrace, RelayTraceEvent{
			Stage:           "relay_exit",
			Direction:       relayDirectionFromStage(secondExit.stage),
			Graceful:        false,
			WroteDownstream: combinedWroteDownstream,
			Error:           relayErrorString(secondExit.err),
	REDACTED)
		return result, &RelayExit{
			Stage:           secondExit.stage,
			Err:             secondExit.err,
			WroteDownstream: combinedWroteDownstream,
	REDACTED
REDACTED
	if options.FirstMessageSent {
		emitRelayTrace(onTrace, RelayTraceEvent{
			Stage:           "relay_client_closed",
			Graceful:        true,
			WroteDownstream: combinedWroteDownstream,
	REDACTED)
		return result, nil
REDACTED
	emitRelayTrace(onTrace, RelayTraceEvent{
		Stage:           "relay_complete",
		Graceful:        true,
		WroteDownstream: combinedWroteDownstream,
REDACTED)
	_ = clientConn.Close()
	return result, nil
REDACTED

func runClientToUpstream(
	ctx context.Context,
	clientConn FrameConn,
	readClientFrame func(context.Context, FrameConn) (coderws.MessageType, []byte, error),
	writeUpstream func(msgType coderws.MessageType, payload []byte) error,
	markActivity func(),
	forwardedFrames *atomic.Int64,
	onTrace func(event RelayTraceEvent),
	exitCh chan<- relayExitSignal,
) {
	if readClientFrame == nil {
		readClientFrame = func(ctx context.Context, conn FrameConn) (coderws.MessageType, []byte, error) {
			return conn.ReadFrame(ctx)
	REDACTED
REDACTED
	for {
		msgType, payload, err := readClientFrame(ctx, clientConn)
		if err != nil {
			emitRelayTrace(onTrace, RelayTraceEvent{
				Stage:     "read_client_failed",
				Direction: "client_to_upstream",
				Error:     err.Error(),
				Graceful:  isDisconnectError(err),
		REDACTED)
			exitCh <- relayExitSignal{stage: "read_client", err: err, graceful: isDisconnectError(err)REDACTED
			return
	REDACTED
		markActivity()
		if err := writeUpstream(msgType, payload); err != nil {
			emitRelayTrace(onTrace, RelayTraceEvent{
				Stage:        "write_upstream_failed",
				Direction:    "client_to_upstream",
				MessageType:  relayMessageTypeString(msgType),
				PayloadBytes: len(payload),
				Error:        err.Error(),
		REDACTED)
			exitCh <- relayExitSignal{stage: "write_upstream", err: errREDACTED
			return
	REDACTED
		if forwardedFrames != nil {
			forwardedFrames.Add(1)
	REDACTED
		markActivity()
REDACTED
REDACTED

func runUpstreamToClient(
	ctx context.Context,
	upstreamConn FrameConn,
	writeClient func(msgType coderws.MessageType, payload []byte) error,
	startAt time.Time,
	nowFn func() time.Time,
	state *relayState,
	onUsageParseFailure func(eventType string, usageRaw string),
	onTurnComplete func(turn RelayTurnResult),
	beforeWriteClient func(msgType coderws.MessageType, payload []byte, wroteDownstream bool) error,
	beforeClientWrite func(msgType coderws.MessageType, payload []byte),
	afterClientWrite func(msgType coderws.MessageType, payload []byte, writeErr error),
	afterWriteClient func(msgType coderws.MessageType, payload []byte),
	dropDownstreamWrites *atomic.Bool,
	forwardedFrames *atomic.Int64,
	droppedFrames *atomic.Int64,
	markActivity func(),
	onTrace func(event RelayTraceEvent),
	exitCh chan<- relayExitSignal,
) {
	wroteDownstream := false
	for {
		msgType, payload, err := upstreamConn.ReadFrame(ctx)
		if err != nil {
			emitRelayTrace(onTrace, RelayTraceEvent{
				Stage:           "read_upstream_failed",
				Direction:       "upstream_to_client",
				Error:           err.Error(),
				Graceful:        isDisconnectError(err),
				WroteDownstream: wroteDownstream,
		REDACTED)
			exitCh <- relayExitSignal{
				stage:           "read_upstream",
				err:             err,
				graceful:        isDisconnectError(err),
				wroteDownstream: wroteDownstream,
		REDACTED
			return
	REDACTED
		markActivity()
		if beforeWriteClient != nil {
			if err := beforeWriteClient(msgType, payload, wroteDownstream); err != nil {
				emitRelayTrace(onTrace, RelayTraceEvent{
					Stage:           "upstream_message_rejected",
					Direction:       "upstream_to_client",
					MessageType:     relayMessageTypeString(msgType),
					PayloadBytes:    len(payload),
					WroteDownstream: wroteDownstream,
					Error:           err.Error(),
			REDACTED)
				exitCh <- relayExitSignal{
					stage:           "upstream_message",
					err:             err,
					wroteDownstream: wroteDownstream,
			REDACTED
				return
		REDACTED
	REDACTED
		observedEvent := observedUpstreamEvent{REDACTED
		switch msgType {
		case coderws.MessageText:
			observedEvent = observeUpstreamMessage(state, payload, startAt, nowFn, onUsageParseFailure)
		case coderws.MessageBinary:
			// binary frame 直接透传，不进入 JSON 观测路径（避免无效解析开销）。
	REDACTED
		emitTurnComplete(onTurnComplete, state, observedEvent)
		if dropDownstreamWrites != nil && dropDownstreamWrites.Load() {
			if droppedFrames != nil {
				droppedFrames.Add(1)
		REDACTED
			emitRelayTrace(onTrace, RelayTraceEvent{
				Stage:           "drop_downstream_frame",
				Direction:       "upstream_to_client",
				MessageType:     relayMessageTypeString(msgType),
				PayloadBytes:    len(payload),
				WroteDownstream: wroteDownstream,
		REDACTED)
			if observedEvent.terminal {
				exitCh <- relayExitSignal{
					stage:           "drain_terminal",
					graceful:        true,
					wroteDownstream: wroteDownstream,
			REDACTED
				return
		REDACTED
			markActivity()
			continue
	REDACTED
		if beforeClientWrite != nil {
			beforeClientWrite(msgType, payload)
	REDACTED
		writeErr := writeClient(msgType, payload)
		if afterClientWrite != nil {
			afterClientWrite(msgType, payload, writeErr)
	REDACTED
		if writeErr != nil {
			emitRelayTrace(onTrace, RelayTraceEvent{
				Stage:           "write_client_failed",
				Direction:       "upstream_to_client",
				MessageType:     relayMessageTypeString(msgType),
				PayloadBytes:    len(payload),
				WroteDownstream: wroteDownstream,
				Error:           writeErr.Error(),
		REDACTED)
			exitCh <- relayExitSignal{stage: "write_client", err: writeErr, wroteDownstream: wroteDownstreamREDACTED
			return
	REDACTED
		wroteDownstream = true
		if afterWriteClient != nil {
			afterWriteClient(msgType, payload)
	REDACTED
		if forwardedFrames != nil {
			forwardedFrames.Add(1)
	REDACTED
		markActivity()
REDACTED
REDACTED

func runIdleWatchdog(
	ctx context.Context,
	nowFn func() time.Time,
	idleTimeout time.Duration,
	lastActivity *atomic.Int64,
	onTrace func(event RelayTraceEvent),
	exitCh chan<- relayExitSignal,
) {
	if idleTimeout <= 0 {
		return
REDACTED
	checkInterval := minDuration(idleTimeout/4, 5*time.Second)
	if checkInterval < time.Second {
		checkInterval = time.Second
REDACTED
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			last := time.Unix(0, lastActivity.Load())
			if nowFn().Sub(last) < idleTimeout {
				continue
		REDACTED
			emitRelayTrace(onTrace, RelayTraceEvent{
				Stage:     "idle_timeout_triggered",
				Direction: "watchdog",
				Error:     context.DeadlineExceeded.Error(),
		REDACTED)
			exitCh <- relayExitSignal{stage: "idle_timeout", err: context.DeadlineExceededREDACTED
			return
	REDACTED
REDACTED
REDACTED

func emitRelayTrace(onTrace func(event RelayTraceEvent), event RelayTraceEvent) {
	if onTrace == nil {
		return
REDACTED
	onTrace(event)
REDACTED

func relayMessageTypeString(msgType coderws.MessageType) string {
	switch msgType {
	case coderws.MessageText:
		return "text"
	case coderws.MessageBinary:
		return "binary"
	default:
		return "unknown(" + strconv.Itoa(int(msgType)) + ")"
REDACTED
REDACTED

func relayDirectionFromStage(stage string) string {
	switch stage {
	case "read_client", "write_upstream":
		return "client_to_upstream"
	case "read_upstream", "write_client", "drain_terminal":
		return "upstream_to_client"
	case "idle_timeout":
		return "watchdog"
	default:
		return ""
REDACTED
REDACTED

func relayErrorString(err error) string {
	if err == nil {
		return ""
REDACTED
	return err.Error()
REDACTED

func observeUpstreamMessage(
	state *relayState,
	message []byte,
	startAt time.Time,
	nowFn func() time.Time,
	onUsageParseFailure func(eventType string, usageRaw string),
) observedUpstreamEvent {
	if state == nil || len(message) == 0 {
		return observedUpstreamEvent{REDACTED
REDACTED
	values := gjson.GetManyBytes(message, "type", "response.id", "response_id", "id")
	eventType := strings.TrimSpace(values[0].String())
	if eventType == "" {
		return observedUpstreamEvent{REDACTED
REDACTED
	responseID := strings.TrimSpace(values[1].String())
	if responseID == "" {
		responseID = strings.TrimSpace(values[2].String())
REDACTED
	// 仅 terminal 事件兜底读取顶层 id，避免把 event_id 当成 response_id 关联到 turn。
	if responseID == "" && isTerminalEvent(eventType) {
		responseID = strings.TrimSpace(values[3].String())
REDACTED
	now := nowFn()

	if state.firstTokenMs == nil && isTokenEvent(eventType) {
		ms := int(now.Sub(startAt).Milliseconds())
		if ms >= 0 {
			state.firstTokenMs = &ms
	REDACTED
		if state.activeTurn != nil && state.activeTurn.firstTokenMs == nil {
			tms := int(now.Sub(state.activeTurn.startAt).Milliseconds())
			if tms >= 0 {
				state.activeTurn.firstTokenMs = &tms
		REDACTED
	REDACTED
REDACTED
	parsedUsage := parseUsageAndAccumulate(state, message, eventType, onUsageParseFailure)
	observed := observedUpstreamEvent{
		eventType:  eventType,
		responseID: responseID,
		usage:      parsedUsage,
REDACTED
	if responseID != "" {
		turnTiming := openAIWSRelayGetOrInitTurnTiming(state, responseID, now)
		if turnTiming != nil && turnTiming.firstTokenMs == nil && isTokenEvent(eventType) {
			ms := int(now.Sub(turnTiming.startAt).Milliseconds())
			if ms >= 0 {
				turnTiming.firstTokenMs = &ms
		REDACTED
	REDACTED
REDACTED
	if !isTerminalEvent(eventType) {
		return observed
REDACTED
	observed.terminal = true
	state.terminalEventType = eventType
	if responseID != "" {
		state.lastResponseID = responseID
		if turnTiming, ok := openAIWSRelayDeleteTurnTiming(state, responseID); ok {
			duration := now.Sub(turnTiming.startAt)
			if duration < 0 {
				duration = 0
		REDACTED
			observed.duration = duration
			observed.firstToken = openAIWSRelayCloneIntPtr(turnTiming.firstTokenMs)
	REDACTED
REDACTED
	return observed
REDACTED

func emitTurnComplete(
	onTurnComplete func(turn RelayTurnResult),
	state *relayState,
	observed observedUpstreamEvent,
) {
	if onTurnComplete == nil || !observed.terminal {
		return
REDACTED
	responseID := strings.TrimSpace(observed.responseID)
	if responseID == "" {
		return
REDACTED
	requestModel := ""
	if state != nil {
		requestModel = state.requestModel
REDACTED
	onTurnComplete(RelayTurnResult{
		RequestModel:      requestModel,
		Usage:             observed.usage,
		RequestID:         responseID,
		TerminalEventType: observed.eventType,
		Duration:          observed.duration,
		FirstTokenMs:      openAIWSRelayCloneIntPtr(observed.firstToken),
REDACTED)
REDACTED

func openAIWSRelayGetOrInitTurnTiming(state *relayState, responseID string, now time.Time) *relayTurnTiming {
	if state == nil {
		return nil
REDACTED
	if state.turnTimingByID == nil {
		state.turnTimingByID = make(map[string]*relayTurnTiming, 8)
REDACTED
	timing, ok := state.turnTimingByID[responseID]
	if !ok || timing == nil || timing.startAt.IsZero() {
		timing = &relayTurnTiming{startAt: nowREDACTED
		state.turnTimingByID[responseID] = timing
		state.activeTurn = timing
		return timing
REDACTED
	return timing
REDACTED

func openAIWSRelayDeleteTurnTiming(state *relayState, responseID string) (relayTurnTiming, bool) {
	if state == nil || state.turnTimingByID == nil {
		return relayTurnTiming{REDACTED, false
REDACTED
	timing, ok := state.turnTimingByID[responseID]
	if !ok || timing == nil {
		return relayTurnTiming{REDACTED, false
REDACTED
	delete(state.turnTimingByID, responseID)
	if state.activeTurn == timing {
		state.activeTurn = nil
REDACTED
	return *timing, true
REDACTED

func openAIWSRelayCloneIntPtr(v *int) *int {
	if v == nil {
		return nil
REDACTED
	cloned := *v
	return &cloned
REDACTED

func parseUsageAndAccumulate(
	state *relayState,
	message []byte,
	eventType string,
	onParseFailure func(eventType string, usageRaw string),
) Usage {
	if state == nil || len(message) == 0 || !shouldParseUsage(eventType) {
		return Usage{REDACTED
REDACTED
	usageResult := gjson.GetBytes(message, "response.usage")
	if !usageResult.Exists() {
		return Usage{REDACTED
REDACTED
	usageRaw := strings.TrimSpace(usageResult.Raw)
	if usageRaw == "" || !strings.HasPrefix(usageRaw, "{") {
		recordUsageParseFailure()
		if onParseFailure != nil {
			onParseFailure(eventType, usageRaw)
	REDACTED
		return Usage{REDACTED
REDACTED

	inputResult := gjson.GetBytes(message, "response.usage.input_tokens")
	if !inputResult.Exists() {
		inputResult = gjson.GetBytes(message, "response.usage.prompt_tokens")
REDACTED
	outputResult := gjson.GetBytes(message, "response.usage.output_tokens")
	if !outputResult.Exists() {
		outputResult = gjson.GetBytes(message, "response.usage.completion_tokens")
REDACTED
	cachedResult := gjson.GetBytes(message, "response.usage.input_tokens_details.cached_tokens")
	if !cachedResult.Exists() {
		cachedResult = gjson.GetBytes(message, "response.usage.prompt_tokens_details.cached_tokens")
REDACTED
	imageTokens := usageResult.Get("output_tokens_details.image_tokens").Int()
	if imageTokens == 0 {
		imageTokens = usageResult.Get("completion_tokens_details.image_tokens").Int()
REDACTED

	inputTokens, inputOK := parseUsageIntField(inputResult, true)
	outputTokens, outputOK := parseUsageIntField(outputResult, true)
	cachedTokens, cachedOK := parseUsageIntField(cachedResult, false)
	if !inputOK || !outputOK || !cachedOK {
		recordUsageParseFailure()
		if onParseFailure != nil {
			onParseFailure(eventType, usageRaw)
	REDACTED
		// 解析失败时不做部分字段累加，避免计费 usage 出现“半有效”状态。
		return Usage{REDACTED
REDACTED
	parsedUsage := Usage{
		InputTokens:              inputTokens,
		OutputTokens:             outputTokens,
		CacheCreationInputTokens: openAICacheCreationTokensFromUsage(usageResult),
		CacheReadInputTokens:     cachedTokens,
		ImageOutputTokens:        int(imageTokens),
REDACTED

	state.usage.InputTokens += parsedUsage.InputTokens
	state.usage.OutputTokens += parsedUsage.OutputTokens
	state.usage.CacheCreationInputTokens += parsedUsage.CacheCreationInputTokens
	state.usage.CacheReadInputTokens += parsedUsage.CacheReadInputTokens
	state.usage.ImageOutputTokens += parsedUsage.ImageOutputTokens
	return parsedUsage
REDACTED

func parseUsageIntField(value gjson.Result, required bool) (int, bool) {
	if !value.Exists() {
		return 0, !required
REDACTED
	if value.Type != gjson.Number {
		return 0, false
REDACTED
	return int(value.Int()), true
REDACTED

func openAICacheCreationTokensFromUsage(value gjson.Result) int {
	for _, field := range []string{
		"input_tokens_details.cache_write_tokens",
		"prompt_tokens_details.cache_write_tokens",
		"input_tokens_details.cache_creation_tokens",
		"prompt_tokens_details.cache_creation_tokens",
REDACTED {
		result := value.Get(field)
		if result.Exists() {
			return max(int(result.Int()), 0)
	REDACTED
REDACTED
	for _, field := range []string{
		"cache_write_tokens",
		"cache_creation_input_tokens",
		"cache_write_input_tokens",
		"cache_creation_tokens",
REDACTED {
		if tokens := int(value.Get(field).Int()); tokens > 0 {
			return tokens
	REDACTED
REDACTED
	return 0
REDACTED

func enrichResult(result *RelayResult, state *relayState, duration time.Duration) {
	if result == nil {
		return
REDACTED
	result.Duration = duration
	if state == nil {
		return
REDACTED
	result.RequestModel = state.requestModel
	result.Usage = state.usage
	result.RequestID = state.lastResponseID
	result.TerminalEventType = state.terminalEventType
	result.FirstTokenMs = state.firstTokenMs
REDACTED

func isDisconnectError(err error) bool {
	if err == nil {
		return false
REDACTED
	if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) || errors.Is(err, context.Canceled) {
		return true
REDACTED
	switch coderws.CloseStatus(err) {
	case coderws.StatusNormalClosure, coderws.StatusGoingAway, coderws.StatusNoStatusRcvd, coderws.StatusAbnormalClosure:
		return true
REDACTED
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	if message == "" {
		return false
REDACTED
	return strings.Contains(message, "failed to read frame header: eof") ||
		strings.Contains(message, "unexpected eof") ||
		strings.Contains(message, "use of closed network connection") ||
		strings.Contains(message, "connection reset by peer") ||
		strings.Contains(message, "broken pipe")
REDACTED

func isTerminalEvent(eventType string) bool {
	switch eventType {
	case "response.completed", "response.done", "response.failed", "response.incomplete", "response.cancelled", "response.canceled":
		return true
	default:
		return false
REDACTED
REDACTED

func shouldParseUsage(eventType string) bool {
	switch eventType {
	case "response.completed", "response.done", "response.failed", "response.incomplete", "response.cancelled", "response.canceled":
		return true
	default:
		return false
REDACTED
REDACTED

func isTokenEvent(eventType string) bool {
	if eventType == "" {
		return false
REDACTED
	switch eventType {
	case "response.created", "response.in_progress", "response.output_item.added", "response.output_item.done":
		return false
REDACTED
	if strings.Contains(eventType, ".delta") {
		return true
REDACTED
	if strings.HasPrefix(eventType, "response.output_text") {
		return true
REDACTED
	if strings.HasPrefix(eventType, "response.output") {
		return true
REDACTED
	return eventType == "response.completed" || eventType == "response.done"
REDACTED

func minDuration(a, b time.Duration) time.Duration {
	if a <= 0 {
		return b
REDACTED
	if b <= 0 {
		return a
REDACTED
	if a < b {
		return a
REDACTED
	return b
REDACTED

func waitRelayExit(exitCh <-chan relayExitSignal, timeout time.Duration) (relayExitSignal, bool) {
	if timeout <= 0 {
		timeout = 200 * time.Millisecond
REDACTED
	select {
	case sig := <-exitCh:
		return sig, true
	case <-time.After(timeout):
		return relayExitSignal{REDACTED, false
REDACTED
REDACTED
