package service

import (
	"context"
	"errors"
	"sync"

	coderws "github.com/coder/websocket"
)

type openAIWSAccountWaitReaderKey struct{}

func OpenAIWSAccountWaitClientError(ctx context.Context) error {
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	r, _ := ctx.Value(openAIWSAccountWaitReaderKey{}).(*openAIWSAccountWaitReader)
	if r == nil {
		return nil
	}
	r.mu.Lock()
	err := r.err
	r.mu.Unlock()
	if err == nil {
		return nil
	}
	var closeErr *OpenAIWSClientCloseError
	if errors.As(err, &closeErr) {
		return err
	}
	return NewOpenAIWSClientCloseError(coderws.StatusGoingAway, "websocket client disconnected before account admission", err)
}

type openAIWSAccountWaitReader struct {
	conn    *coderws.Conn
	mu      sync.Mutex
	queue   []openAIWSClientReadResult
	err     error
	reading bool
	closed  bool
	notify  chan struct{}
	watch   *openAIWSAccountWaitWatch
	workers sync.WaitGroup
}

type openAIWSAccountWaitWatch struct {
	cancel context.CancelCauseFunc
}

// WithOpenAIWSAccountWaitReader installs a lazy, single reader shared by
// admission waiting and normal frame reads. Cleanup belongs to the handler.
func WithOpenAIWSAccountWaitReader(ctx context.Context, conn *coderws.Conn) (context.Context, func()) {
	r := &openAIWSAccountWaitReader{conn: conn, notify: make(chan struct{})}
	return context.WithValue(ctx, openAIWSAccountWaitReaderKey{}, r), r.close
}

// BeginOpenAIWSAccountWait is called between normal reads. Ending observation
// leaves any active read intact for the next normal reader to consume.
func BeginOpenAIWSAccountWait(ctx context.Context) (context.Context, func()) {
	r, _ := ctx.Value(openAIWSAccountWaitReaderKey{}).(*openAIWSAccountWaitReader)
	if r == nil {
		return ctx, func() {}
	}
	waitCtx, cancel := context.WithCancelCause(ctx)
	w := &openAIWSAccountWaitWatch{cancel: cancel}
	r.mu.Lock()
	r.watch = w
	if r.err != nil {
		cancel(r.err)
	} else {
		r.startLocked()
	}
	r.mu.Unlock()
	return waitCtx, func() {
		r.mu.Lock()
		if r.watch == w {
			r.watch = nil
		}
		r.mu.Unlock()
		cancel(context.Canceled)
	}
}

func (r *openAIWSAccountWaitReader) signalLocked() {
	close(r.notify)
	r.notify = make(chan struct{})
}

func (r *openAIWSAccountWaitReader) startLocked() {
	if r.reading || r.closed || r.err != nil {
		return
	}
	r.reading = true
	r.workers.Add(1)
	go r.readLoop()
}

func (r *openAIWSAccountWaitReader) readLoop() {
	defer r.workers.Done()
	for {
		kind, payload, err := r.conn.Read(context.Background())
		r.mu.Lock()
		if err != nil {
			r.err = err
		} else if r.watch != nil && len(r.queue) != 0 {
			r.err = NewOpenAIWSClientCloseError(coderws.StatusTryAgainLater, "too many pending websocket frames while waiting for account", nil)
		} else {
			r.queue = append(r.queue, openAIWSClientReadResult{messageType: kind, payload: payload})
		}
		if r.watch != nil && r.err != nil {
			r.watch.cancel(r.err)
		}
		again := r.watch != nil && r.err == nil && !r.closed
		if !again {
			r.reading = false
		}
		r.signalLocked()
		r.mu.Unlock()
		if !again {
			return
		}
	}
}

func (r *openAIWSAccountWaitReader) next() openAIWSClientReadResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Normal processing takes ownership of any read started while waiting.
	r.watch = nil
	for {
		if len(r.queue) > 0 {
			result := r.queue[0]
			r.queue[0] = openAIWSClientReadResult{}
			r.queue = r.queue[1:]
			return result
		}
		if r.err != nil {
			return openAIWSClientReadResult{err: r.err}
		}
		if r.closed {
			return openAIWSClientReadResult{err: context.Canceled}
		}
		r.startLocked()
		notify := r.notify
		r.mu.Unlock()
		<-notify
		r.mu.Lock()
	}
}

func (r *openAIWSAccountWaitReader) close() {
	r.mu.Lock()
	r.closed = true
	if r.watch != nil {
		r.watch.cancel(context.Canceled)
	}
	r.signalLocked()
	r.mu.Unlock()
	_ = r.conn.CloseNow()
	r.workers.Wait()
}
