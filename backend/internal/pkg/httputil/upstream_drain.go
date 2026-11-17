package httputil

import (
	"context"
	"sync"
	"time"
)

const DefaultUpstreamDrainTimeout = 180 * time.Second

type upstreamDrainKey struct{}
type upstreamDrainPolicy struct {
	client  context.Context
	timeout time.Duration
}

// DetachUpstreamContext keeps billing reads alive after the client leaves. It
// records the original cancellation signal without starting a timer or changing
// existing construction-time release semantics. The transport owns the actual
// attempt lifetime and installs the bounded drain when sending the request.
func DetachUpstreamContext(ctx context.Context, timeout ...time.Duration) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	policy := upstreamDrainPolicy{client: ctx}
	if previous, ok := ctx.Value(upstreamDrainKey{}).(upstreamDrainPolicy); ok {
		policy = previous
	}
	if len(timeout) > 0 && timeout[0] > 0 {
		policy.timeout = timeout[0]
	}
	return context.WithValue(context.WithoutCancel(ctx), upstreamDrainKey{}, policy)
}

// NewUpstreamAttemptContext starts a total drain budget when the original client
// context is cancelled. Incoming bytes do not extend this budget. Until then the
// request may stream for arbitrarily long, and late usage remains readable
// throughout the drain window. release must run on error or response-body close.
func NewUpstreamAttemptContext(parent context.Context, defaultTimeout time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	policy, ok := parent.Value(upstreamDrainKey{}).(upstreamDrainPolicy)
	if !ok || policy.client.Done() == nil {
		return ctx, cancel
	}
	timeout := policy.timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	if timeout <= 0 {
		timeout = DefaultUpstreamDrainTimeout
	}
	var mu sync.Mutex
	var timer *time.Timer
	closed := false
	startDrain := func() {
		mu.Lock()
		defer mu.Unlock()
		if !closed {
			timer = time.AfterFunc(timeout, cancel)
		}
	}
	stop := context.AfterFunc(policy.client, startDrain)
	return ctx, func() {
		stop()
		mu.Lock()
		closed = true
		if timer != nil {
			timer.Stop()
		}
		mu.Unlock()
		cancel()
	}
}
