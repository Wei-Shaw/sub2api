package service

import (
	"context"
	"errors"
	"sync"
	"time"
)

const defaultGeminiDisconnectDrainTimeout = 2 * time.Minute

// ErrGeminiClientDisconnected identifies a stream read that ended only after
// the downstream client had already gone away.
var ErrGeminiClientDisconnected = errors.New("gemini downstream client disconnected")

// newGeminiUpstreamAttemptContext keeps request-scoped values while delaying
// cancellation of an accepted upstream attempt until its bounded drain window
// expires. Callers must release it when the response body is closed.
func newGeminiUpstreamAttemptContext(parent context.Context, drainTimeout time.Duration, onDrainStart func()) (context.Context, func(), context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	if drainTimeout <= 0 {
		drainTimeout = defaultGeminiDisconnectDrainTimeout
	}

	upstreamCtx, cancel := context.WithCancel(context.WithoutCancel(parent))
	var mu sync.Mutex
	var timer *time.Timer
	finished := false
	startDrain := func() {
		mu.Lock()
		defer mu.Unlock()
		if finished || timer != nil {
			return
		}
		timer = time.AfterFunc(drainTimeout, cancel)
		if onDrainStart != nil {
			onDrainStart()
		}
	}
	stopParent := context.AfterFunc(parent, startDrain)

	var once sync.Once
	release := func() {
		once.Do(func() {
			_ = stopParent()
			mu.Lock()
			finished = true
			if timer != nil {
				timer.Stop()
			}
			mu.Unlock()
			cancel()
		})
	}
	return upstreamCtx, startDrain, release
}

func (s *GeminiMessagesCompatService) geminiDisconnectDrainTimeout() time.Duration {
	if s != nil && s.cfg != nil && s.cfg.Gateway.GeminiDisconnectDrainTimeoutSeconds > 0 {
		return time.Duration(s.cfg.Gateway.GeminiDisconnectDrainTimeoutSeconds) * time.Second
	}
	return defaultGeminiDisconnectDrainTimeout
}

func sleepGeminiBackoffWithContext(ctx context.Context, attempt int) bool {
	timer := time.NewTimer(geminiBackoffDelay(attempt))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
