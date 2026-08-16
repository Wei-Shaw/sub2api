package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type slotCleanupCache struct {
	ConcurrencyCache
	calls        atomic.Int64
	registers    atomic.Int64
	refreshes    atomic.Int64
	deadCleanups atomic.Int64
	cleanupErr   error
	registerErr  error
}

func (c *slotCleanupCache) CleanupExpiredAccountSlotKeys(context.Context) error {
	c.calls.Add(1)
	return nil
}

func (c *slotCleanupCache) RegisterProcessLease(context.Context, string, time.Duration) error {
	c.registers.Add(1)
	return c.registerErr
}

func (c *slotCleanupCache) RefreshProcessLease(context.Context, string, time.Duration) (bool, error) {
	c.refreshes.Add(1)
	return true, nil
}

func (c *slotCleanupCache) CleanupDeadAPIKeySlots(context.Context) error {
	c.deadCleanups.Add(1)
	return c.cleanupErr
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

func TestStartSlotCleanupWorker_APIKeyCleanupFailureDoesNotBlockAccountCleanup(t *testing.T) {
	cache := &slotCleanupCache{cleanupErr: errors.New("redis cleanup unavailable")}
	svc := NewConcurrencyService(cache)

	svc.StartSlotCleanupWorker(nil, time.Hour)

	deadline := time.After(time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if cache.deadCleanups.Load() > 0 && cache.calls.Load() > 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatalf(
				"cleanup calls = %d, dead API key cleanups = %d; account cleanup must continue after API key cleanup failure",
				cache.calls.Load(),
				cache.deadCleanups.Load(),
			)
		case <-ticker.C:
		}
	}
}

func TestAPIKeyProcessLiveness_RegistersAndStopsWithoutEarlyExpiry(t *testing.T) {
	cache := &slotCleanupCache{}
	svc := NewConcurrencyService(cache)

	if err := svc.StartAPIKeyProcessLiveness(); err != nil {
		t.Fatalf("StartAPIKeyProcessLiveness() error = %v", err)
	}
	if got := cache.registers.Load(); got != 1 {
		t.Fatalf("registers = %d, want 1", got)
	}
	if got := cache.deadCleanups.Load(); got != 1 {
		t.Fatalf("dead cleanups = %d, want 1", got)
	}
	if err := svc.StopAPIKeyProcessLiveness(); err != nil {
		t.Fatalf("StopAPIKeyProcessLiveness() error = %v", err)
	}
}

func TestAPIKeyProcessLiveness_InitialCleanupFailureKeepsRegisteredLease(t *testing.T) {
	cache := &slotCleanupCache{cleanupErr: errors.New("redis cleanup unavailable")}
	svc := NewConcurrencyService(cache)

	if err := svc.StartAPIKeyProcessLiveness(); err != nil {
		t.Fatalf("StartAPIKeyProcessLiveness() error = %v", err)
	}
	if got := cache.registers.Load(); got != 1 {
		t.Fatalf("registers = %d, want 1", got)
	}
	if err := svc.StopAPIKeyProcessLiveness(); err != nil {
		t.Fatalf("StopAPIKeyProcessLiveness() error = %v", err)
	}
}

func TestAPIKeyProcessLiveness_InitialRegistrationFailureKeepsRetryWorker(t *testing.T) {
	cache := &slotCleanupCache{registerErr: errors.New("redis registration unavailable")}
	svc := NewConcurrencyService(cache)

	if err := svc.StartAPIKeyProcessLiveness(); err != nil {
		t.Fatalf("StartAPIKeyProcessLiveness() error = %v", err)
	}
	if got := cache.registers.Load(); got != 1 {
		t.Fatalf("registers = %d, want 1", got)
	}
	if got := cache.deadCleanups.Load(); got != 0 {
		t.Fatalf("dead cleanups = %d, want 0 before this process has a lease", got)
	}
	if svc.apiKeyLeaseStop == nil || svc.apiKeyLeaseDone == nil {
		t.Fatal("process liveness worker was not kept for a registration retry")
	}
	if err := svc.StopAPIKeyProcessLiveness(); err != nil {
		t.Fatalf("StopAPIKeyProcessLiveness() error = %v", err)
	}
}
