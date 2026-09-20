package service

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	coderws "github.com/coder/websocket"
)

type openAIWSClientReadResult struct {
	messageType coderws.MessageType
	payload     []byte
	err         error
}

type openAIWSClientReadAheadKey struct{}

type openAIWSClientReadAhead struct {
	conn    *coderws.Conn
	claimed atomic.Bool
	result  chan openAIWSClientReadResult
	done    chan struct{}
}

// BeginOpenAIWSClientReadAhead observes disconnects during handshake recovery.
// The next regular reader takes over this read, preserving an early client frame.
// Call only while no regular reader is active, and clean up when the handler exits.
func BeginOpenAIWSClientReadAhead(ctx context.Context, conn *coderws.Conn) (context.Context, func()) {
	if reader, _ := ctx.Value(openAIWSAccountWaitReaderKey{}).(*openAIWSAccountWaitReader); reader != nil && reader.conn == conn {
		return BeginOpenAIWSAccountWait(ctx)
	}
	if pending, _ := ctx.Value(openAIWSClientReadAheadKey{}).(*openAIWSClientReadAhead); pending != nil && pending.conn == conn && !pending.claimed.Load() {
		return ctx, func() {}
	}
	pending := &openAIWSClientReadAhead{
		conn: conn, result: make(chan openAIWSClientReadResult, 1), done: make(chan struct{}),
	}
	ctx, cancel := context.WithCancel(ctx)
	ctx = context.WithValue(ctx, openAIWSClientReadAheadKey{}, pending)
	go func() {
		defer close(pending.done)
		messageType, payload, err := conn.Read(context.Background())
		pending.result <- openAIWSClientReadResult{messageType: messageType, payload: payload, err: err}
		if err != nil && !pending.claimed.Load() {
			cancel()
		}
	}()
	return ctx, func() {
		cancel()
		select {
		case <-pending.done:
		default:
			_ = conn.CloseNow()
			<-pending.done
		}
	}
}

// ReadOpenAIWSClientMessage keeps one reader alive while control events send
// their close frame, then closes the transport and joins that reader.
func ReadOpenAIWSClientMessage(
	controlCtx context.Context,
	conn *coderws.Conn,
	timeout time.Duration,
	timeoutStatus coderws.StatusCode,
	timeoutReason string,
) (coderws.MessageType, []byte, error) {
	return readOpenAIWSClientMessageWithTimeoutStart(
		controlCtx,
		conn,
		timeout,
		timeoutStatus,
		timeoutReason,
		nil,
		nil,
	)
}

// readOpenAIWSClientMessageWithTimeoutStart supports readers whose timeout
// starts after a state transition, such as a completed passthrough turn. When
// timeoutActive is nil, a positive timeout starts immediately.
func readOpenAIWSClientMessageWithTimeoutStart(
	controlCtx context.Context,
	conn *coderws.Conn,
	timeout time.Duration,
	timeoutStatus coderws.StatusCode,
	timeoutReason string,
	timeoutStart <-chan struct{},
	timeoutActive func() bool,
) (coderws.MessageType, []byte, error) {
	if conn == nil {
		return 0, nil, errors.New("openai websocket client connection is nil")
	}
	if controlCtx == nil {
		controlCtx = context.Background()
	}

	readDone := make(chan openAIWSClientReadResult, 1)
	if reader, _ := controlCtx.Value(openAIWSAccountWaitReaderKey{}).(*openAIWSAccountWaitReader); reader != nil && reader.conn == conn {
		go func() { readDone <- reader.next() }()
	} else if pending, _ := controlCtx.Value(openAIWSClientReadAheadKey{}).(*openAIWSClientReadAhead); pending != nil && pending.conn == conn && pending.claimed.CompareAndSwap(false, true) {
		readDone = pending.result
	} else {
		go func() {
			messageType, payload, err := conn.Read(context.Background())
			readDone <- openAIWSClientReadResult{messageType: messageType, payload: payload, err: err}
		}()
	}

	var timer *time.Timer
	var timeoutCh <-chan time.Time
	startTimeout := func() {
		if timeout <= 0 || (timeoutActive != nil && !timeoutActive()) {
			return
		}
		if timer == nil {
			timer = time.NewTimer(timeout)
		} else {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(timeout)
		}
		timeoutCh = timer.C
	}
	if timeoutActive == nil || timeoutActive() {
		startTimeout()
	}
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()

	closeAndJoin := func(status coderws.StatusCode, reason string, cause error) (coderws.MessageType, []byte, error) {
		_ = conn.Close(status, reason)
		_ = conn.CloseNow()
		<-readDone
		return 0, nil, NewOpenAIWSClientCloseError(status, reason, cause)
	}

	for {
		select {
		case result := <-readDone:
			return result.messageType, result.payload, result.err
		case <-timeoutStart:
			startTimeout()
		case <-timeoutCh:
			return closeAndJoin(timeoutStatus, timeoutReason, context.DeadlineExceeded)
		case <-controlCtx.Done():
			cause := context.Cause(controlCtx)
			if errors.Is(cause, ErrOpenAIWSIngressLeaseLost) {
				return closeAndJoin(
					coderws.StatusTryAgainLater,
					"websocket ingress capacity lease lost; please reconnect",
					cause,
				)
			}
			return closeAndJoin(coderws.StatusGoingAway, "websocket request canceled", cause)
		}
	}
}
