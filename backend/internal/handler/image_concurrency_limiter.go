package handler

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// sharedImageConcurrencyLimiter 是进程内唯一的出图并发限流器。
// OpenAI/Grok 出图（OpenAIGatewayHandler）与 Adobe 出图（GatewayHandler）共用它，
// 使 gateway.image_concurrency 的上限对所有出图入口整体生效。
var sharedImageConcurrencyLimiter = &imageConcurrencyLimiter{}

// acquireImageConcurrencySlot 按 gateway.image_concurrency 配置占用出图槽，不写响应。
// cfg 或 limiter 为空时视为不限流。
func acquireImageConcurrencySlot(ctx context.Context, cfg *config.Config, limiter *imageConcurrencyLimiter) (func(), bool) {
	if cfg == nil || limiter == nil {
		return nil, true
	}
	imageConcurrency := cfg.Gateway.ImageConcurrency
	wait := strings.TrimSpace(imageConcurrency.OverflowMode) == config.ImageConcurrencyOverflowModeWait
	return limiter.Acquire(
		ctx,
		imageConcurrency.Enabled,
		imageConcurrency.MaxConcurrentRequests,
		wait,
		time.Duration(imageConcurrency.WaitTimeoutSeconds)*time.Second,
		imageConcurrency.MaxWaitingRequests,
	)
}

type imageConcurrencyLimiter struct {
	mu      sync.Mutex
	notify  chan struct{}
	limit   int
	active  int
	waiting int
	enabled bool
}

func (l *imageConcurrencyLimiter) TryAcquire(enabled bool, limit int) (func(), bool) {
	return l.acquire(context.Background(), enabled, limit, false, 0, 0)
}

func (l *imageConcurrencyLimiter) Acquire(ctx context.Context, enabled bool, limit int, wait bool, timeout time.Duration, maxWaiting int) (func(), bool) {
	return l.acquire(ctx, enabled, limit, wait, timeout, maxWaiting)
}

func (l *imageConcurrencyLimiter) acquire(ctx context.Context, enabled bool, limit int, wait bool, timeout time.Duration, maxWaiting int) (func(), bool) {
	if !enabled || limit <= 0 {
		return nil, true
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if wait {
		if timeout <= 0 {
			return nil, false
		}
		waitCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		ctx = waitCtx
	}
	if maxWaiting < 0 {
		maxWaiting = 0
	}
	for {
		release, acquired, waitRelease, notify := l.tryAcquireLocked(enabled, limit, wait, maxWaiting)
		if acquired {
			return release, acquired
		}
		if !wait || notify == nil {
			return nil, false
		}
		if !l.waitForSlot(ctx, notify) {
			if waitRelease != nil {
				waitRelease()
			}
			return nil, false
		}
		if waitRelease != nil {
			waitRelease()
		}
	}
}

func (l *imageConcurrencyLimiter) tryAcquireLocked(enabled bool, limit int, wait bool, maxWaiting int) (func(), bool, func(), <-chan struct{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.notify == nil {
		l.notify = make(chan struct{})
	}
	if l.enabled != enabled || l.limit != limit {
		l.enabled = enabled
		l.limit = limit
	}
	if l.active < l.limit {
		l.active++
		return l.releaseFunc(), true, nil, nil
	}
	if !wait {
		return nil, false, nil, nil
	}
	if maxWaiting > 0 && l.waiting >= maxWaiting {
		return nil, false, nil, nil
	}
	l.waiting++
	return nil, false, l.waiterReleaseFunc(), l.notify
}

func (l *imageConcurrencyLimiter) waitForSlot(ctx context.Context, notify <-chan struct{}) bool {
	select {
	case <-notify:
		return true
	case <-ctx.Done():
		return false
	}
}

func (l *imageConcurrencyLimiter) releaseFunc() func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			if l.active > 0 {
				l.active--
			}
			if l.notify != nil {
				close(l.notify)
				l.notify = make(chan struct{})
			}
			l.mu.Unlock()
		})
	}
}

func (l *imageConcurrencyLimiter) waiterReleaseFunc() func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			if l.waiting > 0 {
				l.waiting--
			}
			l.mu.Unlock()
		})
	}
}
