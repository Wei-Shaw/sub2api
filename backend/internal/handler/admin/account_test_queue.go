package admin

import (
	"context"
	"sync"
	"time"
)

type accountTestQueue struct {
	mu              sync.Mutex
	nextAvailableAt time.Time
	cooldown        time.Duration
}

func newAccountTestQueue(cooldown time.Duration) *accountTestQueue {
	return &accountTestQueue{cooldown: cooldown}
}

func (q *accountTestQueue) Run(ctx context.Context, fn func() error) error {
	if q == nil {
		return fn()
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	if wait := time.Until(q.nextAvailableAt); wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}

	err := fn()
	if q.cooldown > 0 {
		q.nextAvailableAt = time.Now().Add(q.cooldown)
	}
	return err
}
