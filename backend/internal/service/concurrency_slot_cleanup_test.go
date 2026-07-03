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
REDACTED

func (c *slotCleanupCache) CleanupExpiredAccountSlotKeys(context.Context) error {
	c.calls.Add(1)
	return nil
REDACTED

func TestStartSlotCleanupWorker_UsesCacheWideCleanupWithoutAccountRepo(t *testing.T) {
	cache := &slotCleanupCache{REDACTED
	svc := NewConcurrencyService(cache)

	svc.StartSlotCleanupWorker(nil, time.Hour)

	deadline := time.After(time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if cache.calls.Load() > 0 {
			return
	REDACTED
		select {
		case <-deadline:
			t.Fatal("cleanup worker did not call cache-wide account slot cleanup")
		case <-ticker.C:
	REDACTED
REDACTED
REDACTED
