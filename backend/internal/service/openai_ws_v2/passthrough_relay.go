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
}

type Usage struct {
	InputTokens              int
	OutputTokens             int
	CacheCreationInputTokens int
	CacheReadInputTokens     int
	ImageOutputTokens        int
}

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
}

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
}

type RelayExit struct {
	Stage           string
	Err             error
	Graceful        bool
	WroteDownstream bool
}

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
}

type RelayTraceEvent struct {
	Stage           string
	Direction       string
	MessageType     string
	PayloadBytes    int
	Graceful        bool
	WroteDownstream bool
	Error           string
}

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
}

type relayExitSignal struct {
	stage           string
	err             error
	graceful        bool
	wroteDownstream bool
}

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
}

type relayTurnTiming struct {
	startAt               time.Time
	firstTokenMs          *int
	firstResponseModel    string
	terminalResponseModel string
	responseModelConflict bool
	// terminalResponseServiceTier is only taken from terminal events: earlier
	// events echo the requested tier, not the one the upstream actually used.
	terminalResponseServiceTier string
}

func Relay(
	ctx context.Context,
	clientConn FrameConn,
	upstreamConn FrameConn,
	firstClientMessage []byte,
	options RelayOptions,
) (RelayResult, *RelayExit) {
	result := RelayResult{RequestModel: strings.TrimSpace(gjson.GetBytes(firstClientMessage, "model").String())}
	if clientConn == nil || upstreamConn == nil {
		return result, &RelayExit{Stage: "relay_init", Err: errors.New("relay connection is nil")}
	}
	if ctx == nil {
		ctx = context.Background()
	}

	nowFn := options.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	writeTimeout := options.WriteTimeout
	if writeTimeout <= 0 {
		writeTimeout = 2 * time.Minute
	}
	drainTimeout := options.UpstreamDrainTimeout
	if drainTimeout <= 0 {
		drainTimeout = 1200 * time.Millisecond
	}
	firstMessageType := options.FirstMessageType
	if firstMessageType != coderws.MessageBinary {
		firstMessageType = coderws.MessageText
	}
	startAt := nowFn()
	state := &relayState{requestModel: result.RequestModel}
	if isClientResponseCreateFrame(firstMessageType, firstClientMessage) {
		firstTurnStartedAt := options.FirstTurnStartedAt
		if firstTurnStartedAt.IsZero() {
			firstTurnStartedAt = startAt
		}
		state.setPendingTurnStartedAt(firstTurnStartedAt)
	}
	onTrace := options.OnTrace

	relayCtx, relayCancel := context.WithCancel(ctx)
	defer relayCancel()

	lastActivity := atomic.Int64{}
	lastActivity.Store(nowFn().UnixNano())
	markActivity := func() {
		lastActivity.Store(nowFn().UnixNano())
	}

	writeUpstream := func(msgType coderws.MessageType, payload []byte) error {
		writeCtx, cancel := context.WithTimeout(relayCtx, writeTimeout)
		defer cancel()
		return upstreamConn.WriteFrame(writeCtx, msgType, payload)
	}
	writeClientFrameUpstream := func(msgType coderws.MessageType, payload []byte) error {
		if isClientResponseCreateFrame(msgType, payload) {
			state.setRequestModel(strings.TrimSpace(gjson.GetBytes(payload, "model").String()))
			turnStartedAt := time.Time{}
			if options.TakeNextTurnStartedAt != nil {
				turnStartedAt = options.TakeNextTurnStartedAt()
			}
			if turnStartedAt.IsZero() {
				turnStartedAt = nowFn()
			}
			state.setPendingTurnStartedAt(turnStartedAt)
		}
		return writeUpstream(msgType, payload)
	}
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
	}

	clientToUpstreamFrames := &atomic.Int64{}
	upstreamToClientFrames := &atomic.Int64{}
	droppedDownstreamFrames := &atomic.Int64{}
	emitRelayTrace(onTrace, RelayTraceEvent{
		Stage:        "relay_start",
		PayloadBytes: len(firstClientMessage),
		MessageType:  relayMessageTypeString(firstMessageType),
	})

	if options.FirstMessageSent {
		emitRelayTrace(onTrace, RelayTraceEvent{
			Stage:        "write_first_message_skipped",
			Direction:    "client_to_upstream",
			MessageType:  relayMessageTypeString(firstMessageType),
			PayloadBytes: len(firstClientMessage),
		})
	} else {
		if err := writeUpstream(firstMessageType, firstClientMessage); err != nil {
			result.Duration = nowFn().Sub(startAt)
			emitRelayTrace(onTrace, RelayTraceEvent{
				Stage:        "write_first_message_failed",
				Direction:    "client_to_upstream",
				MessageType:  relayMessageTypeString(firstMessageType),
				PayloadBytes: len(firstClientMessage),
				Error:        err.Error(),
			})
			return result, &RelayExit{Stage: "write_upstream", Err: err}
		}
		emitRelayTrace(onTrace, RelayTraceEvent{
			Stage:        "write_first_message_ok",
			Direction:    "client_to_upstream",
			MessageType:  relayMessageTypeString(firstMessageType),
			PayloadBytes: len(firstClientMessage),
		})
	}
	clientToUpstreamFrames.Add(1)
	markActivity()

	exitCh := make(chan relayExitSignal, 3)
	dropDownstreamWrites := atomic.Bool{}
	clientReaderStarted := atomic.Bool{}
	startClientReader := func() {
		if !clientReaderStarted.CompareAndSwap(false, true) {
			return
		}
		go runClientToUpstream(relayCtx, clientConn, options.ReadClientFrame, writeClientFrameUpstream, markActivity, clientToUpstreamFrames, onTrace, exitCh)
	}
	if !options.StartClientAfterFirstDownstream {
		startClientReader()
	}
	upstreamDone := make(chan struct{})
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
				}
			},
			&dropDownstreamWrites,
			upstreamToClientFrames,
			droppedDownstreamFrames,
			markActivity,
			onTrace,
			exitCh,
		)
	}()
	go runIdleWatchdog(relayCtx, nowFn, options.IdleTimeout, &lastActivity, onTrace, exitCh)

	firstExit := <-exitCh
	// An outer ingress cancellation is a control-plane close, not a graceful
	// upstream disconnect. Leave the client connection open here so the
	// adapter can emit the precise lease/request close code. Internal
	// relayCancel does not cancel ctx and therefore does not take this path.
	if ctx.Err() != nil {
		firstExit.graceful = false
	}
	emitRelayTrace(onTrace, RelayTraceEvent{
		Stage:           "first_exit",
		Direction:       relayDirectionFromStage(firstExit.stage),
		Graceful:        firstExit.graceful,
		WroteDownstream: firstExit.wroteDownstream,
		Error:           relayErrorString(firstExit.err),
	})
	if options.BeforeRelayCancel != nil {
		options.BeforeRelayCancel(RelayExit{
			Stage:           firstExit.stage,
			Err:             firstExit.err,
			Graceful:        firstExit.graceful,
			WroteDownstream: firstExit.wroteDownstream,
		})
	}
	combinedWroteDownstream := firstExit.wroteDownstream
	secondExit := relayExitSignal{graceful: true}
	hasSecondExit := false

	// 客户端断开后尽力继续读取上游短窗口，捕获延迟 usage/terminal 事件用于计费。
	if firstExit.stage == "read_client" && firstExit.graceful {
		dropDownstreamWrites.Store(true)
		secondExit, hasSecondExit = waitRelayExit(exitCh, drainTimeout)
	} else {
		relayCancel()
		_ = upstreamConn.Close()
		if clientReaderStarted.Load() {
			secondExit, hasSecondExit = waitRelayExit(exitCh, 200*time.Millisecond)
		}
	}
	if hasSecondExit {
		combinedWroteDownstream = combinedWroteDownstream || secondExit.wroteDownstream
		emitRelayTrace(onTrace, RelayTraceEvent{
			Stage:           "second_exit",
			Direction:       relayDirectionFromStage(secondExit.stage),
			Graceful:        secondExit.graceful,
			WroteDownstream: secondExit.wroteDownstream,
			Error:           relayErrorString(secondExit.err),
		})
	}

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
		})
		return result, nil
	}
	if firstExit.stage == "read_client" && firstExit.graceful {
		stage := "client_disconnected"
		exitErr := firstExit.err
		if hasSecondExit && !secondExit.graceful {
			stage = secondExit.stage
			exitErr = secondExit.err
		}
		if exitErr == nil {
			exitErr = io.EOF
		}
		emitRelayTrace(onTrace, RelayTraceEvent{
			Stage:           "relay_exit",
			Direction:       relayDirectionFromStage(stage),
			Graceful:        false,
			WroteDownstream: combinedWroteDownstream,
			Error:           relayErrorString(exitErr),
		})
		return result, &RelayExit{
			Stage:           stage,
			Err:             exitErr,
			WroteDownstream: combinedWroteDownstream,
		}
	}
	if firstExit.graceful && (!hasSecondExit || secondExit.graceful) {
		emitRelayTrace(onTrace, RelayTraceEvent{
			Stage:           "relay_complete",
			Graceful:        true,
			WroteDownstream: combinedWroteDownstream,
		})
		_ = clientConn.Close()
		return result, nil
	}
	if !firstExit.graceful {
		emitRelayTrace(onTrace, RelayTraceEvent{
			Stage:           "relay_exit",
			Direction:       relayDirectionFromStage(firstExit.stage),
			Graceful:        false,
			WroteDownstream: combinedWroteDownstream,
			Error:           relayErrorString(firstExit.err),
		})
		return result, &RelayExit{
			Stage:           firstExit.stage,
			Err:             firstExit.err,
			WroteDownstream: combinedWroteDownstream,
		}
	}
	if hasSecondExit && !secondExit.graceful {
		emitRelayTrace(onTrace, RelayTraceEvent{
			Stage:           "relay_exit",
			Direction:       relayDirectionFromStage(secondExit.stage),
			Graceful:        false,
			WroteDownstream: combinedWroteDownstream,
			Error:           relayErrorString(secondExit.err),
		})
		return result, &RelayExit{
			Stage:           secondExit.stage,
			Err:             secondExit.err,
			WroteDownstream: combinedWroteDownstream,
		}
	}
	if options.FirstMessageSent {
		emitRelayTrace(onTrace, RelayTraceEvent{
			Stage:           "relay_client_closed",
			Graceful:        true,
			WroteDownstream: combinedWroteDownstream,
		})
		return result, nil
	}
	emitRelayTrace(onTrace, RelayTraceEvent{
		Stage:           "relay_complete",
		Graceful:        true,
		WroteDownstream: combinedWroteDownstream,
	})
	_ = clientConn.Close()
	return result, nil
}

func isClientResponseCreateFrame(msgType coderws.MessageType, payload []byte) bool {
	if msgType != coderws.MessageText && msgType != coderws.MessageBinary {
		return false
	}
	return strings.TrimSpace(gjson.GetBytes(payload, "type").String()) == "response.create"
}

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
		}
	}
	for {
		msgType, payload, err := readClientFrame(ctx, clientConn)
		if err != nil {
			emitRelayTrace(onTrace, RelayTraceEvent{
				Stage:     "read_client_failed",
				Direction: "client_to_upstream",
				Error:     err.Error(),
				Graceful:  isDisconnectError(err),
			})
			exitCh <- relayExitSignal{stage: "read_client", err: err, graceful: isDisconnectError(err)}
			return
		}
		markActivity()
		if err := writeUpstream(msgType, payload); err != nil {
			emitRelayTrace(onTrace, RelayTraceEvent{
				Stage:        "write_upstream_failed",
				Direction:    "client_to_upstream",
				MessageType:  relayMessageTypeString(msgType),
				PayloadBytes: len(payload),
				Error:        err.Error(),
			})
			exitCh <- relayExitSignal{stage: "write_upstream", err: err}
			return
		}
		if forwardedFrames != nil {
			forwardedFrames.Add(1)
		}
		markActivity()
	}
}

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
			})
			exitCh <- relayExitSignal{
				stage:           "read_upstream",
				err:             err,
				graceful:        isDisconnectError(err),
				wroteDownstream: wroteDownstream,
			}
			return
		}
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
				})
				exitCh <- relayExitSignal{
					stage:           "upstream_message",
					err:             err,
					wroteDownstream: wroteDownstream,
				}
				return
			}
		}
		observedEvent := observedUpstreamEvent{}
		switch msgType {
		case coderws.MessageText:
			eventType := strings.TrimSpace(gjson.GetBytes(payload, "type").String())
			if shouldFinalizePendingBareError(state, payload, eventType) {
				emitTurnComplete(onTurnComplete, state, finalizePendingBareError(state, nowFn()))
			}
			observedEvent = observeUpstreamMessage(state, payload, startAt, nowFn, onUsageParseFailure)
		case coderws.MessageBinary:
			// binary frame 直接透传，不进入 JSON 观测路径（避免无效解析开销）。
		}
		emitTurnComplete(onTurnComplete, state, observedEvent)
		if dropDownstreamWrites != nil && dropDownstreamWrites.Load() {
			if droppedFrames != nil {
				droppedFrames.Add(1)
			}
			emitRelayTrace(onTrace, RelayTraceEvent{
				Stage:           "drop_downstream_frame",
				Direction:       "upstream_to_client",
				MessageType:     relayMessageTypeString(msgType),
				PayloadBytes:    len(payload),
				WroteDownstream: wroteDownstream,
			})
			if observedEvent.terminal {
				exitCh <- relayExitSignal{
					stage:           "drain_terminal",
					graceful:        true,
					wroteDownstream: wroteDownstream,
				}
				return
			}
			markActivity()
			continue
		}
		if beforeClientWrite != nil {
			beforeClientWrite(msgType, payload)
		}
		writeErr := writeClient(msgType, payload)
		if afterClientWrite != nil {
			afterClientWrite(msgType, payload, writeErr)
		}
		if writeErr != nil {
			emitRelayTrace(onTrace, RelayTraceEvent{
				Stage:           "write_client_failed",
				Direction:       "upstream_to_client",
				MessageType:     relayMessageTypeString(msgType),
				PayloadBytes:    len(payload),
				WroteDownstream: wroteDownstream,
				Error:           writeErr.Error(),
			})
			exitCh <- relayExitSignal{stage: "write_client", err: writeErr, wroteDownstream: wroteDownstream}
			return
		}
		wroteDownstream = true
		if afterWriteClient != nil {
			afterWriteClient(msgType, payload)
		}
		if forwardedFrames != nil {
			forwardedFrames.Add(1)
		}
		markActivity()
	}
}

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
	}
	checkInterval := minDuration(idleTimeout/4, 5*time.Second)
	if checkInterval < time.Second {
		checkInterval = time.Second
	}
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
			}
			emitRelayTrace(onTrace, RelayTraceEvent{
				Stage:     "idle_timeout_triggered",
				Direction: "watchdog",
				Error:     context.DeadlineExceeded.Error(),
			})
			exitCh <- relayExitSignal{stage: "idle_timeout", err: context.DeadlineExceeded}
			return
		}
	}
}

func emitRelayTrace(onTrace func(event RelayTraceEvent), event RelayTraceEvent) {
	if onTrace == nil {
		return
	}
	onTrace(event)
}

func relayMessageTypeString(msgType coderws.MessageType) string {
	switch msgType {
	case coderws.MessageText:
		return "text"
	case coderws.MessageBinary:
		return "binary"
	default:
		return "unknown(" + strconv.Itoa(int(msgType)) + ")"
	}
}

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
	}
}

func relayErrorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func observeUpstreamMessage(
	state *relayState,
	message []byte,
	startAt time.Time,
	nowFn func() time.Time,
	onUsageParseFailure func(eventType string, usageRaw string),
) observedUpstreamEvent {
	if state == nil || len(message) == 0 {
		return observedUpstreamEvent{}
	}
	values := gjson.GetManyBytes(message, "type", "response.id", "response_id", "id")
	eventType := strings.TrimSpace(values[0].String())
	if eventType == "" {
		return observedUpstreamEvent{}
	}
	responseID := strings.TrimSpace(values[1].String())
	if responseID == "" {
		responseID = strings.TrimSpace(values[2].String())
	}
	// 仅 terminal 事件兜底读取顶层 id，避免把 event_id 当成 response_id 关联到 turn。
	if responseID == "" && isTerminalEvent(eventType) {
		responseID = strings.TrimSpace(values[3].String())
	}
	now := nowFn()

	if state.firstTokenMs == nil && isTokenEvent(eventType) {
		ms := int(now.Sub(startAt).Milliseconds())
		if ms >= 0 {
			state.firstTokenMs = &ms
		}
		if state.activeTurn != nil && state.activeTurn.firstTokenMs == nil {
			tms := int(now.Sub(state.activeTurn.startAt).Milliseconds())
			if tms >= 0 {
				state.activeTurn.firstTokenMs = &tms
			}
		}
	}
	parsedUsage := parseUsageAndAccumulate(state, message, eventType, onUsageParseFailure)
	observed := observedUpstreamEvent{
		eventType:  eventType,
		responseID: responseID,
		usage:      parsedUsage,
	}
	var turnTiming *relayTurnTiming
	if responseID != "" {
		turnTiming = openAIWSRelayGetOrInitTurnTiming(state, responseID, now)
		if turnTiming != nil && turnTiming.firstTokenMs == nil && isTokenEvent(eventType) {
			ms := int(now.Sub(turnTiming.startAt).Milliseconds())
			if ms >= 0 {
				turnTiming.firstTokenMs = &ms
			}
		}
	} else {
		turnTiming = state.activeTurn
	}
	observeRelayTurnResponseModel(turnTiming, firstRelayResponseModel(message), isTerminalEvent(eventType))
	if !isTerminalEvent(eventType) {
		return observed
	}
	observeRelayTurnResponseServiceTier(turnTiming, firstRelayResponseServiceTier(message))
	state.terminalEventType = eventType
	if eventType == "error" {
		// Some Responses servers emit error immediately before response.failed.
		// Defer turn settlement so the authoritative failed usage can replace
		// this fallback instead of billing both terminal frames.
		if observed.responseID == "" {
			observed.responseID = openAIWSRelayActiveTurnID(state)
		}
		pending := observed
		state.pendingBareError = &pending
		return observed
	}
	state.pendingBareError = nil
	return finalizeObservedRelayTerminal(state, observed, now)
}

func shouldFinalizePendingBareError(state *relayState, payload []byte, eventType string) bool {
	if state == nil || state.pendingBareError == nil {
		return false
	}
	eventType = strings.TrimSpace(eventType)
	if eventType == "" || eventType == "error" || eventType == "response.failed" {
		return false
	}
	if isTerminalEvent(eventType) || eventType == "response.created" {
		return true
	}
	// Auxiliary provider frames may be interleaved between error and its
	// authoritative response.failed. Only a response event identifying a
	// different turn closes the pending error.
	responseID := strings.TrimSpace(gjson.GetBytes(payload, "response.id").String())
	if responseID == "" || state.pendingBareError.responseID == "" {
		return false
	}
	return responseID != state.pendingBareError.responseID
}

func finalizePendingBareError(state *relayState, now time.Time) observedUpstreamEvent {
	if state == nil || state.pendingBareError == nil {
		return observedUpstreamEvent{}
	}
	observed := *state.pendingBareError
	state.pendingBareError = nil
	return finalizeObservedRelayTerminal(state, observed, now)
}

func finalizeObservedRelayTerminal(state *relayState, observed observedUpstreamEvent, now time.Time) observedUpstreamEvent {
	if state == nil || strings.TrimSpace(observed.eventType) == "" {
		return observedUpstreamEvent{}
	}
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
			}
			observed.startedAt = turnTiming.startAt
			observed.duration = duration
			observed.firstToken = openAIWSRelayCloneIntPtr(turnTiming.firstTokenMs)
		}
	} else {
		state.consumePendingTurnStartedAt()
		openAIWSRelayDiscardActiveTurnTiming(state)
	}
	return observed
}

func emitTurnComplete(
	onTurnComplete func(turn RelayTurnResult),
	state *relayState,
	observed observedUpstreamEvent,
) {
	if onTurnComplete == nil || !observed.terminal {
		return
	}
	responseID := strings.TrimSpace(observed.responseID)
	if responseID == "" && strings.TrimSpace(observed.eventType) != "error" {
		return
	}
	requestModel := ""
	if state != nil {
		requestModel = state.currentRequestModel()
	}
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
	})
}

func firstRelayResponseModel(message []byte) string {
	if len(message) == 0 {
		return ""
	}
	values := gjson.GetManyBytes(message, "response.model", "model")
	for _, value := range values {
		if value.Type != gjson.String {
			continue
		}
		if model := strings.TrimSpace(value.String()); model != "" {
			return model
		}
	}
	return ""
}

func observeRelayTurnResponseModel(turn *relayTurnTiming, model string, terminal bool) {
	if turn == nil {
		return
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return
	}
	current := relayTurnResponseModel(turn)
	if current != "" && !strings.EqualFold(current, model) {
		turn.responseModelConflict = true
	}
	if terminal {
		turn.terminalResponseModel = model
		return
	}
	if turn.firstResponseModel == "" {
		turn.firstResponseModel = model
	}
}

func relayTurnResponseModel(turn *relayTurnTiming) string {
	if turn == nil {
		return ""
	}
	if turn.terminalResponseModel != "" {
		return turn.terminalResponseModel
	}
	return turn.firstResponseModel
}

func firstRelayResponseServiceTier(message []byte) string {
	if len(message) == 0 {
		return ""
	}
	values := gjson.GetManyBytes(message, "response.service_tier", "service_tier")
	for _, value := range values {
		if value.Type != gjson.String {
			continue
		}
		if tier := strings.TrimSpace(value.String()); tier != "" {
			return tier
		}
	}
	return ""
}

func observeRelayTurnResponseServiceTier(turn *relayTurnTiming, tier string) {
	if turn == nil {
		return
	}
	if tier = strings.TrimSpace(tier); tier != "" {
		turn.terminalResponseServiceTier = tier
	}
}

func openAIWSRelayGetOrInitTurnTiming(state *relayState, responseID string, now time.Time) *relayTurnTiming {
	if state == nil {
		return nil
	}
	if state.turnTimingByID == nil {
		state.turnTimingByID = make(map[string]*relayTurnTiming, 8)
	}
	timing, ok := state.turnTimingByID[responseID]
	if !ok || timing == nil || timing.startAt.IsZero() {
		startAt := state.consumePendingTurnStartedAt()
		if startAt.IsZero() {
			startAt = now
		}
		timing = &relayTurnTiming{startAt: startAt}
		state.turnTimingByID[responseID] = timing
		state.activeTurn = timing
		return timing
	}
	return timing
}

func (s *relayState) setPendingTurnStartedAt(startedAt time.Time) {
	if s == nil || startedAt.IsZero() {
		return
	}
	startedAtCopy := startedAt
	s.pendingTurnStart.Store(&startedAtCopy)
}

func (s *relayState) consumePendingTurnStartedAt() time.Time {
	if s == nil {
		return time.Time{}
	}
	startedAt := s.pendingTurnStart.Swap(nil)
	if startedAt == nil {
		return time.Time{}
	}
	return *startedAt
}

func openAIWSRelayDeleteTurnTiming(state *relayState, responseID string) (relayTurnTiming, bool) {
	if state == nil || state.turnTimingByID == nil {
		return relayTurnTiming{}, false
	}
	timing, ok := state.turnTimingByID[responseID]
	if !ok || timing == nil {
		return relayTurnTiming{}, false
	}
	delete(state.turnTimingByID, responseID)
	if state.activeTurn == timing {
		state.activeTurn = nil
	}
	return *timing, true
}

func openAIWSRelayDiscardActiveTurnTiming(state *relayState) {
	if state == nil || state.activeTurn == nil {
		return
	}
	active := state.activeTurn
	for responseID, timing := range state.turnTimingByID {
		if timing == active {
			delete(state.turnTimingByID, responseID)
		}
	}
	state.activeTurn = nil
}

func openAIWSRelayActiveTurnID(state *relayState) string {
	if state == nil || state.activeTurn == nil {
		return ""
	}
	for responseID, timing := range state.turnTimingByID {
		if timing == state.activeTurn {
			return responseID
		}
	}
	return ""
}

func openAIWSRelayCloneIntPtr(v *int) *int {
	if v == nil {
		return nil
	}
	cloned := *v
	return &cloned
}

func parseUsageAndAccumulate(
	state *relayState,
	message []byte,
	eventType string,
	onParseFailure func(eventType string, usageRaw string),
) Usage {
	if state == nil || len(message) == 0 || !shouldParseUsage(eventType) || !bytes.Contains(message, []byte(`"usage"`)) {
		return Usage{}
	}
	usageResult := gjson.GetBytes(message, "response.usage")
	if !usageResult.Exists() {
		usageResult = gjson.GetBytes(message, "usage")
	}
	if !usageResult.Exists() {
		return Usage{}
	}
	usageRaw := strings.TrimSpace(usageResult.Raw)
	if usageRaw == "" || !strings.HasPrefix(usageRaw, "{") {
		recordUsageParseFailure()
		if onParseFailure != nil {
			onParseFailure(eventType, usageRaw)
		}
		return Usage{}
	}

	inputResult := usageResult.Get("input_tokens")
	if !inputResult.Exists() {
		inputResult = usageResult.Get("prompt_tokens")
	}
	outputResult := usageResult.Get("output_tokens")
	if !outputResult.Exists() {
		outputResult = usageResult.Get("completion_tokens")
	}
	cachedResult := usageResult.Get("input_tokens_details.cached_tokens")
	if !cachedResult.Exists() {
		cachedResult = usageResult.Get("prompt_tokens_details.cached_tokens")
	}
	imageTokens := usageResult.Get("output_tokens_details.image_tokens").Int()
	if imageTokens == 0 {
		imageTokens = usageResult.Get("completion_tokens_details.image_tokens").Int()
	}

	requireTotals := isTerminalEvent(strings.TrimSpace(eventType))
	inputTokens, inputOK := parseUsageIntField(inputResult, requireTotals)
	outputTokens, outputOK := parseUsageIntField(outputResult, requireTotals)
	cachedTokens, cachedOK := parseUsageIntField(cachedResult, false)
	if !inputOK || !outputOK || !cachedOK {
		recordUsageParseFailure()
		if onParseFailure != nil {
			onParseFailure(eventType, usageRaw)
		}
		// 解析失败时不做部分字段累加，避免计费 usage 出现“半有效”状态。
		return Usage{}
	}
	reasoningTokens := usageResult.Get("output_tokens_details.reasoning_tokens").Int()
	if reasoningTokens == 0 {
		reasoningTokens = usageResult.Get("completion_tokens_details.reasoning_tokens").Int()
	}
	if reasoningTokens > 0 {
		outputTokens = int(xai.IncludeIndependentReasoningTokens(
			int64(inputTokens), int64(outputTokens), usageResult.Get("total_tokens").Int(), reasoningTokens,
		))
	}
	parsedUsage := Usage{
		InputTokens:              inputTokens,
		OutputTokens:             outputTokens,
		CacheCreationInputTokens: openAICacheCreationTokensFromUsage(usageResult),
		CacheReadInputTokens:     cachedTokens,
		ImageOutputTokens:        int(imageTokens),
	}

	if isTerminalEvent(strings.TrimSpace(eventType)) {
		if relayUsageHasTokens(parsedUsage) || !relayUsageHasTokens(state.turnUsage) {
			state.turnUsage = parsedUsage
		}
	} else {
		mergeRelayUsageNonZero(&state.turnUsage, parsedUsage)
		return Usage{}
	}
	return parsedUsage
}

func relayUsageHasTokens(usage Usage) bool {
	return usage.InputTokens > 0 || usage.OutputTokens > 0 ||
		usage.CacheCreationInputTokens > 0 || usage.CacheReadInputTokens > 0 ||
		usage.ImageOutputTokens > 0
}

func mergeRelayUsageNonZero(dst *Usage, src Usage) {
	if dst == nil {
		return
	}
	if src.InputTokens > 0 {
		dst.InputTokens = src.InputTokens
	}
	if src.OutputTokens > 0 {
		dst.OutputTokens = src.OutputTokens
	}
	if src.CacheCreationInputTokens > 0 {
		dst.CacheCreationInputTokens = src.CacheCreationInputTokens
	}
	if src.CacheReadInputTokens > 0 {
		dst.CacheReadInputTokens = src.CacheReadInputTokens
	}
	if src.ImageOutputTokens > 0 {
		dst.ImageOutputTokens = src.ImageOutputTokens
	}
}

func finalizeRelayTurnUsage(state *relayState) Usage {
	if state == nil {
		return Usage{}
	}
	turnUsage := state.turnUsage
	state.usage.InputTokens += turnUsage.InputTokens
	state.usage.OutputTokens += turnUsage.OutputTokens
	state.usage.CacheCreationInputTokens += turnUsage.CacheCreationInputTokens
	state.usage.CacheReadInputTokens += turnUsage.CacheReadInputTokens
	state.usage.ImageOutputTokens += turnUsage.ImageOutputTokens
	state.turnUsage = Usage{}
	return turnUsage
}

func parseUsageIntField(value gjson.Result, required bool) (int, bool) {
	if !value.Exists() {
		return 0, !required
	}
	if value.Type != gjson.Number {
		return 0, false
	}
	return int(value.Int()), true
}

func openAICacheCreationTokensFromUsage(value gjson.Result) int {
	for _, field := range []string{
		"input_tokens_details.cache_write_tokens",
		"prompt_tokens_details.cache_write_tokens",
		"input_tokens_details.cache_creation_tokens",
		"prompt_tokens_details.cache_creation_tokens",
	} {
		result := value.Get(field)
		if result.Exists() {
			return max(int(result.Int()), 0)
		}
	}
	for _, field := range []string{
		"cache_write_tokens",
		"cache_creation_input_tokens",
		"cache_write_input_tokens",
		"cache_creation_tokens",
	} {
		if tokens := int(value.Get(field).Int()); tokens > 0 {
			return tokens
		}
	}
	return 0
}

func enrichResult(result *RelayResult, state *relayState, duration time.Duration) {
	if result == nil {
		return
	}
	result.Duration = duration
	if state == nil {
		return
	}
	result.RequestModel = state.currentRequestModel()
	result.ResponseModel = state.lastResponseModel
	result.ResponseModelConflict = state.responseConflict
	result.ResponseServiceTier = state.lastResponseServiceTier
	result.Usage = state.usage
	result.RequestID = state.lastResponseID
	result.TerminalEventType = state.terminalEventType
	result.FirstTokenMs = state.firstTokenMs
}

func (s *relayState) setRequestModel(model string) {
	if s == nil || model == "" {
		return
	}
	s.requestModelMu.Lock()
	s.requestModel = model
	s.requestModelMu.Unlock()
}

func (s *relayState) currentRequestModel() string {
	if s == nil {
		return ""
	}
	s.requestModelMu.RLock()
	defer s.requestModelMu.RUnlock()
	return s.requestModel
}

func isDisconnectError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) || errors.Is(err, context.Canceled) {
		return true
	}
	switch coderws.CloseStatus(err) {
	case coderws.StatusNormalClosure, coderws.StatusGoingAway, coderws.StatusNoStatusRcvd, coderws.StatusAbnormalClosure:
		return true
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	if message == "" {
		return false
	}
	return strings.Contains(message, "failed to read frame header: eof") ||
		strings.Contains(message, "unexpected eof") ||
		strings.Contains(message, "use of closed network connection") ||
		strings.Contains(message, "connection reset by peer") ||
		strings.Contains(message, "broken pipe")
}

func isTerminalEvent(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "response.completed", "response.done", "response.failed", "response.incomplete", "response.cancelled", "response.canceled", "error":
		return true
	default:
		return false
	}
}

func shouldParseUsage(eventType string) bool {
	eventType = strings.TrimSpace(eventType)
	if eventType == "error" || isTerminalEvent(eventType) {
		return true
	}
	return strings.HasPrefix(eventType, "response.") && !strings.HasSuffix(eventType, ".delta")
}

func isTokenEvent(eventType string) bool {
	eventType = strings.TrimSpace(eventType)
	return strings.HasSuffix(eventType, ".delta") ||
		eventType == "response.output_text.done" ||
		eventType == "response.function_call_arguments.done"
}

func minDuration(a, b time.Duration) time.Duration {
	if a <= 0 {
		return b
	}
	if b <= 0 {
		return a
	}
	if a < b {
		return a
	}
	return b
}

func waitRelayExit(exitCh <-chan relayExitSignal, timeout time.Duration) (relayExitSignal, bool) {
	if timeout <= 0 {
		timeout = 200 * time.Millisecond
	}
	select {
	case sig := <-exitCh:
		return sig, true
	case <-time.After(timeout):
		return relayExitSignal{}, false
	}
}
