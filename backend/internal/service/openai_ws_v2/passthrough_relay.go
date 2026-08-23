package openai_ws_v2

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/tidwall/gjson"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
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
	RequestModel          string
	ResponseModel         string
	ResponseModelConflict bool
	// ResponseServiceTier is the raw service_tier declared by the last terminal
	// response event; "" when the upstream never declared one.
	ResponseServiceTier     string
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
	RequestModel          string
	ResponseModel         string
	ResponseModelConflict bool
	ResponseServiceTier   string
	Usage                 Usage
	RequestID             string
	TerminalEventType     string
	StartedAt             time.Time
	Duration              time.Duration
	FirstTokenMs          *int
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
	FirstTurnStartedAt              time.Time
	TakeNextTurnStartedAt           func() time.Time
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
	usage                   Usage
	turnUsage               Usage
	requestModelMu          sync.RWMutex
	requestModel            string
	pendingTurnStart        atomic.Pointer[time.Time]
	lastResponseID          string
	lastResponseModel       string
	lastResponseServiceTier string
	responseConflict        bool
	terminalEventType       string
	firstTokenMs            *int
	turnTimingByID          map[string]*relayTurnTiming
	activeTurn              *relayTurnTiming
	pendingBareError        *observedUpstreamEvent
REDACTED

type relayExitSignal struct {
	stage           string
	err             error
	graceful        bool
	wroteDownstream bool
REDACTED

type observedUpstreamEvent struct {
	terminal            bool
	eventType           string
	responseID          string
	usage               Usage
	startedAt           time.Time
	responseModel       string
	responseConflict    bool
	responseServiceTier string
	duration            time.Duration
	firstToken          *int
REDACTED

type relayTurnTiming struct {
	startAt               time.Time
	firstTokenMs          *int
	firstResponseModel    string
	terminalResponseModel string
	responseModelConflict bool
	// terminalResponseServiceTier is only taken from terminal events: earlier
	// events echo the requested tier, not the one the upstream actually used.
	terminalResponseServiceTier string
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
	if isClientResponseCreateFrame(firstMessageType, firstClientMessage) {
		firstTurnStartedAt := options.FirstTurnStartedAt
		if firstTurnStartedAt.IsZero() {
			firstTurnStartedAt = startAt
	REDACTED
		state.setPendingTurnStartedAt(firstTurnStartedAt)
REDACTED
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
	writeClientFrameUpstream := func(msgType coderws.MessageType, payload []byte) error {
		if isClientResponseCreateFrame(msgType, payload) {
			state.setRequestModel(strings.TrimSpace(gjson.GetBytes(payload, "model").String()))
			turnStartedAt := time.Time{REDACTED
			if options.TakeNextTurnStartedAt != nil {
				turnStartedAt = options.TakeNextTurnStartedAt()
		REDACTED
			if turnStartedAt.IsZero() {
				turnStartedAt = nowFn()
		REDACTED
			state.setPendingTurnStartedAt(turnStartedAt)
	REDACTED
		return writeUpstream(msgType, payload)
REDACTED
	writeClient := func(msgType coderws.MessageType, payload []byte) error {
		// 下行写超时故意不挂在 relayCtx 上：coder/websocket 在已武装的 write
		// ctx 被取消时会直接硬关连接（context.AfterFunc 的 stop 不等待执行中
		// 的回调），外部取消若落在一次已成功写入的解除武装窗口内，会连同尚未
		// 发出的 close 帧一起冲掉，客户端只能看到裸 EOF 而收不到关闭码。与读
		// 侧 conn.Read(context.Background()) 同理，取消路径的连接回收由各退出
		// 分支的显式 Close/CloseNow 兜底。
		writeCtx, cancel := context.WithTimeout(context.Background(), writeTimeout)
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
		go runClientToUpstream(relayCtx, clientConn, options.ReadClientFrame, writeClientFrameUpstream, markActivity, clientToUpstreamFrames, onTrace, exitCh)
REDACTED
	if !options.StartClientAfterFirstDownstream {
		startClientReader()
REDACTED
	upstreamDone := make(chan struct{REDACTED)
	go func() {
		defer close(upstreamDone)
		runUpstreamToClient(
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
REDACTED()
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
	// ReadFrame observes relayCtx cancellation and Close is the transport-level
	// fallback. Join the reader before touching relayState or firing the final
	// turn callback; otherwise a late read can race Relay's result settlement.
	<-upstreamDone

	emitTurnComplete(options.OnTurnComplete, state, finalizePendingBareError(state, nowFn()))
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

func isClientResponseCreateFrame(msgType coderws.MessageType, payload []byte) bool {
	if msgType != coderws.MessageText && msgType != coderws.MessageBinary {
		return false
REDACTED
	return strings.TrimSpace(gjson.GetBytes(payload, "type").String()) == "response.create"
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
			emitTurnComplete(onTurnComplete, state, finalizePendingBareError(state, nowFn()))
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
			eventType := strings.TrimSpace(gjson.GetBytes(payload, "type").String())
			if shouldFinalizePendingBareError(state, payload, eventType) {
				emitTurnComplete(onTurnComplete, state, finalizePendingBareError(state, nowFn()))
		REDACTED
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
	var turnTiming *relayTurnTiming
	if responseID != "" {
		turnTiming = openAIWSRelayGetOrInitTurnTiming(state, responseID, now)
		if turnTiming != nil && turnTiming.firstTokenMs == nil && isTokenEvent(eventType) {
			ms := int(now.Sub(turnTiming.startAt).Milliseconds())
			if ms >= 0 {
				turnTiming.firstTokenMs = &ms
		REDACTED
	REDACTED
REDACTED else {
		turnTiming = state.activeTurn
REDACTED
	observeRelayTurnResponseModel(turnTiming, firstRelayResponseModel(message), isTerminalEvent(eventType))
	if !isTerminalEvent(eventType) {
		return observed
REDACTED
	observeRelayTurnResponseServiceTier(turnTiming, firstRelayResponseServiceTier(message))
	state.terminalEventType = eventType
	if eventType == "error" {
		// Some Responses servers emit error immediately before response.failed.
		// Defer turn settlement so the authoritative failed usage can replace
		// this fallback instead of billing both terminal frames.
		if observed.responseID == "" {
			observed.responseID = openAIWSRelayActiveTurnID(state)
	REDACTED
		pending := observed
		state.pendingBareError = &pending
		return observed
REDACTED
	state.pendingBareError = nil
	return finalizeObservedRelayTerminal(state, observed, now)
REDACTED

func shouldFinalizePendingBareError(state *relayState, payload []byte, eventType string) bool {
	if state == nil || state.pendingBareError == nil {
		return false
REDACTED
	eventType = strings.TrimSpace(eventType)
	if eventType == "" || eventType == "error" || eventType == "response.failed" {
		return false
REDACTED
	if isTerminalEvent(eventType) || eventType == "response.created" {
		return true
REDACTED
	// Auxiliary provider frames may be interleaved between error and its
	// authoritative response.failed. Only a response event identifying a
	// different turn closes the pending error.
	responseID := strings.TrimSpace(gjson.GetBytes(payload, "response.id").String())
	if responseID == "" || state.pendingBareError.responseID == "" {
		return false
REDACTED
	return responseID != state.pendingBareError.responseID
REDACTED

func finalizePendingBareError(state *relayState, now time.Time) observedUpstreamEvent {
	if state == nil || state.pendingBareError == nil {
		return observedUpstreamEvent{REDACTED
REDACTED
	observed := *state.pendingBareError
	state.pendingBareError = nil
	return finalizeObservedRelayTerminal(state, observed, now)
REDACTED

func finalizeObservedRelayTerminal(state *relayState, observed observedUpstreamEvent, now time.Time) observedUpstreamEvent {
	if state == nil || strings.TrimSpace(observed.eventType) == "" {
		return observedUpstreamEvent{REDACTED
REDACTED
	observed.usage = finalizeRelayTurnUsage(state)
	observed.terminal = true
	responseID := strings.TrimSpace(observed.responseID)
	if responseID != "" {
		state.lastResponseID = responseID
		if turnTiming, ok := openAIWSRelayDeleteTurnTiming(state, responseID); ok {
			observed.responseModel = relayTurnResponseModel(&turnTiming)
			observed.responseConflict = turnTiming.responseModelConflict
			observed.responseServiceTier = turnTiming.terminalResponseServiceTier
			state.lastResponseModel = observed.responseModel
			state.responseConflict = observed.responseConflict
			state.lastResponseServiceTier = observed.responseServiceTier
			duration := now.Sub(turnTiming.startAt)
			if duration < 0 {
				duration = 0
		REDACTED
			observed.startedAt = turnTiming.startAt
			observed.duration = duration
			observed.firstToken = openAIWSRelayCloneIntPtr(turnTiming.firstTokenMs)
	REDACTED
REDACTED else {
		state.consumePendingTurnStartedAt()
		openAIWSRelayDiscardActiveTurnTiming(state)
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
	if responseID == "" && strings.TrimSpace(observed.eventType) != "error" {
		return
REDACTED
	requestModel := ""
	if state != nil {
		requestModel = state.currentRequestModel()
REDACTED
	onTurnComplete(RelayTurnResult{
		RequestModel:          requestModel,
		ResponseModel:         observed.responseModel,
		ResponseModelConflict: observed.responseConflict,
		ResponseServiceTier:   observed.responseServiceTier,
		Usage:                 observed.usage,
		RequestID:             responseID,
		TerminalEventType:     observed.eventType,
		StartedAt:             observed.startedAt,
		Duration:              observed.duration,
		FirstTokenMs:          openAIWSRelayCloneIntPtr(observed.firstToken),
REDACTED)
REDACTED

func firstRelayResponseModel(message []byte) string {
	if len(message) == 0 {
		return ""
REDACTED
	values := gjson.GetManyBytes(message, "response.model", "model")
	for _, value := range values {
		if value.Type != gjson.String {
			continue
	REDACTED
		if model := strings.TrimSpace(value.String()); model != "" {
			return model
	REDACTED
REDACTED
	return ""
REDACTED

func observeRelayTurnResponseModel(turn *relayTurnTiming, model string, terminal bool) {
	if turn == nil {
		return
REDACTED
	model = strings.TrimSpace(model)
	if model == "" {
		return
REDACTED
	current := relayTurnResponseModel(turn)
	if current != "" && !strings.EqualFold(current, model) {
		turn.responseModelConflict = true
REDACTED
	if terminal {
		turn.terminalResponseModel = model
		return
REDACTED
	if turn.firstResponseModel == "" {
		turn.firstResponseModel = model
REDACTED
REDACTED

func relayTurnResponseModel(turn *relayTurnTiming) string {
	if turn == nil {
		return ""
REDACTED
	if turn.terminalResponseModel != "" {
		return turn.terminalResponseModel
REDACTED
	return turn.firstResponseModel
REDACTED

func firstRelayResponseServiceTier(message []byte) string {
	if len(message) == 0 {
		return ""
REDACTED
	values := gjson.GetManyBytes(message, "response.service_tier", "service_tier")
	for _, value := range values {
		if value.Type != gjson.String {
			continue
	REDACTED
		if tier := strings.TrimSpace(value.String()); tier != "" {
			return tier
	REDACTED
REDACTED
	return ""
REDACTED

func observeRelayTurnResponseServiceTier(turn *relayTurnTiming, tier string) {
	if turn == nil {
		return
REDACTED
	if tier = strings.TrimSpace(tier); tier != "" {
		turn.terminalResponseServiceTier = tier
REDACTED
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
		startAt := state.consumePendingTurnStartedAt()
		if startAt.IsZero() {
			startAt = now
	REDACTED
		timing = &relayTurnTiming{startAt: startAtREDACTED
		state.turnTimingByID[responseID] = timing
		state.activeTurn = timing
		return timing
REDACTED
	return timing
REDACTED

func (s *relayState) setPendingTurnStartedAt(startedAt time.Time) {
	if s == nil || startedAt.IsZero() {
		return
REDACTED
	startedAtCopy := startedAt
	s.pendingTurnStart.Store(&startedAtCopy)
REDACTED

func (s *relayState) consumePendingTurnStartedAt() time.Time {
	if s == nil {
		return time.Time{REDACTED
REDACTED
	startedAt := s.pendingTurnStart.Swap(nil)
	if startedAt == nil {
		return time.Time{REDACTED
REDACTED
	return *startedAt
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

func openAIWSRelayDiscardActiveTurnTiming(state *relayState) {
	if state == nil || state.activeTurn == nil {
		return
REDACTED
	active := state.activeTurn
	for responseID, timing := range state.turnTimingByID {
		if timing == active {
			delete(state.turnTimingByID, responseID)
	REDACTED
REDACTED
	state.activeTurn = nil
REDACTED

func openAIWSRelayActiveTurnID(state *relayState) string {
	if state == nil || state.activeTurn == nil {
		return ""
REDACTED
	for responseID, timing := range state.turnTimingByID {
		if timing == state.activeTurn {
			return responseID
	REDACTED
REDACTED
	return ""
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
	if state == nil || len(message) == 0 || !shouldParseUsage(eventType) || !bytes.Contains(message, []byte(`"usage"`)) {
		return Usage{REDACTED
REDACTED
	usageResult := gjson.GetBytes(message, "response.usage")
	if !usageResult.Exists() {
		usageResult = gjson.GetBytes(message, "usage")
REDACTED
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

	inputResult := usageResult.Get("input_tokens")
	if !inputResult.Exists() {
		inputResult = usageResult.Get("prompt_tokens")
REDACTED
	outputResult := usageResult.Get("output_tokens")
	if !outputResult.Exists() {
		outputResult = usageResult.Get("completion_tokens")
REDACTED
	cachedResult := usageResult.Get("input_tokens_details.cached_tokens")
	if !cachedResult.Exists() {
		cachedResult = usageResult.Get("prompt_tokens_details.cached_tokens")
REDACTED
	imageTokens := usageResult.Get("output_tokens_details.image_tokens").Int()
	if imageTokens == 0 {
		imageTokens = usageResult.Get("completion_tokens_details.image_tokens").Int()
REDACTED

	requireTotals := isTerminalEvent(strings.TrimSpace(eventType))
	inputTokens, inputOK := parseUsageIntField(inputResult, requireTotals)
	outputTokens, outputOK := parseUsageIntField(outputResult, requireTotals)
	cachedTokens, cachedOK := parseUsageIntField(cachedResult, false)
	if !inputOK || !outputOK || !cachedOK {
		recordUsageParseFailure()
		if onParseFailure != nil {
			onParseFailure(eventType, usageRaw)
	REDACTED
		// 解析失败时不做部分字段累加，避免计费 usage 出现“半有效”状态。
		return Usage{REDACTED
REDACTED
	reasoningTokens := usageResult.Get("output_tokens_details.reasoning_tokens").Int()
	if reasoningTokens == 0 {
		reasoningTokens = usageResult.Get("completion_tokens_details.reasoning_tokens").Int()
REDACTED
	if reasoningTokens > 0 {
		outputTokens = int(xai.IncludeIndependentReasoningTokens(
			int64(inputTokens), int64(outputTokens), usageResult.Get("total_tokens").Int(), reasoningTokens,
		))
REDACTED
	parsedUsage := Usage{
		InputTokens:              inputTokens,
		OutputTokens:             outputTokens,
		CacheCreationInputTokens: openAICacheCreationTokensFromUsage(usageResult),
		CacheReadInputTokens:     cachedTokens,
		ImageOutputTokens:        int(imageTokens),
REDACTED

	if isTerminalEvent(strings.TrimSpace(eventType)) {
		if relayUsageHasTokens(parsedUsage) || !relayUsageHasTokens(state.turnUsage) {
			state.turnUsage = parsedUsage
	REDACTED
REDACTED else {
		mergeRelayUsageNonZero(&state.turnUsage, parsedUsage)
		return Usage{REDACTED
REDACTED
	return parsedUsage
REDACTED

func relayUsageHasTokens(usage Usage) bool {
	return usage.InputTokens > 0 || usage.OutputTokens > 0 ||
		usage.CacheCreationInputTokens > 0 || usage.CacheReadInputTokens > 0 ||
		usage.ImageOutputTokens > 0
REDACTED

func mergeRelayUsageNonZero(dst *Usage, src Usage) {
	if dst == nil {
		return
REDACTED
	if src.InputTokens > 0 {
		dst.InputTokens = src.InputTokens
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

func finalizeRelayTurnUsage(state *relayState) Usage {
	if state == nil {
		return Usage{REDACTED
REDACTED
	turnUsage := state.turnUsage
	state.usage.InputTokens += turnUsage.InputTokens
	state.usage.OutputTokens += turnUsage.OutputTokens
	state.usage.CacheCreationInputTokens += turnUsage.CacheCreationInputTokens
	state.usage.CacheReadInputTokens += turnUsage.CacheReadInputTokens
	state.usage.ImageOutputTokens += turnUsage.ImageOutputTokens
	state.turnUsage = Usage{REDACTED
	return turnUsage
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
	result.RequestModel = state.currentRequestModel()
	result.ResponseModel = state.lastResponseModel
	result.ResponseModelConflict = state.responseConflict
	result.ResponseServiceTier = state.lastResponseServiceTier
	result.Usage = state.usage
	result.RequestID = state.lastResponseID
	result.TerminalEventType = state.terminalEventType
	result.FirstTokenMs = state.firstTokenMs
REDACTED

func (s *relayState) setRequestModel(model string) {
	if s == nil || model == "" {
		return
REDACTED
	s.requestModelMu.Lock()
	s.requestModel = model
	s.requestModelMu.Unlock()
REDACTED

func (s *relayState) currentRequestModel() string {
	if s == nil {
		return ""
REDACTED
	s.requestModelMu.RLock()
	defer s.requestModelMu.RUnlock()
	return s.requestModel
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
	switch strings.TrimSpace(eventType) {
	case "response.completed", "response.done", "response.failed", "response.incomplete", "response.cancelled", "response.canceled", "error":
		return true
	default:
		return false
REDACTED
REDACTED

func shouldParseUsage(eventType string) bool {
	eventType = strings.TrimSpace(eventType)
	if eventType == "error" || isTerminalEvent(eventType) {
		return true
REDACTED
	return strings.HasPrefix(eventType, "response.") && !strings.HasSuffix(eventType, ".delta")
REDACTED

func isTokenEvent(eventType string) bool {
	eventType = strings.TrimSpace(eventType)
	return strings.HasSuffix(eventType, ".delta") ||
		eventType == "response.output_text.done" ||
		eventType == "response.function_call_arguments.done"
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
