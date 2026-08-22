package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type slotCleanupCache struct {
	ConcurrencyCache
	calls atomic.Int64
}

func (c *slotCleanupCache) CleanupExpiredAccountSlotKeys(context.Context) error {
	c.calls.Add(1)
	return nil
}

func TestStartSlotCleanupWorker_UsesCacheWideCleanupWithoutAccountRepo(t *testing.T) {
	cache := &slotCleanupCache{}
	svc := NewConcurrencyService(cache)

	svc.StartSlotCleanupWorker(nil, time.Hour)

	deadline := time.After(time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if cache.calls.Load() > 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatal("cleanup worker did not call cache-wide account slot cleanup")
		case <-ticker.C:
		}
	}
}
