package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestCodexDeviceConversationCapacityConcurrentAdmissions(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache, ok := NewConcurrencyCache(client, 15, 900).(service.CodexDeviceConversationCapacityCache)
	require.True(t, ok)
	ctx := context.Background()
	const workers = 40
	results := make(chan bool, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ok, err := cache.AcquireCodexDeviceConversationLeaseWithLimit(ctx, "atomic-slot", fmt.Sprint(i), 5)
			results <- ok
			errs <- err
		}(i)
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	admitted := 0
	for ok := range results {
		if ok {
			admitted++
		}
	}
	require.Equal(t, 5, admitted, "atomic slot capacity cannot be oversold")
}

func TestCodexDeviceConversationCapacityExpiresMembersIndependently(t *testing.T) {
	server := miniredis.RunT(t)
	server.SetTime(time.Now())
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache, ok := NewConcurrencyCache(client, 15, 900).(service.CodexDeviceConversationCapacityCache)
	require.True(t, ok)
	ctx := context.Background()
	start := time.Now()
	server.SetTime(start)
	for _, id := range []string{"stale", "live"} {
		ok, err := cache.AcquireCodexDeviceConversationLeaseWithLimit(ctx, "slot", id, 2)
		require.NoError(t, err)
		require.True(t, ok)
	}
	server.SetTime(start.Add(40 * time.Second))
	ok, err := cache.RefreshCodexDeviceConversationLease(ctx, "slot", "live")
	require.NoError(t, err)
	require.True(t, ok)
	server.SetTime(start.Add(61 * time.Second))
	ok, err = cache.AcquireCodexDeviceConversationLeaseWithLimit(ctx, "slot", "new", 2)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = cache.RefreshCodexDeviceConversationLease(ctx, "slot", "stale")
	require.NoError(t, err)
	require.False(t, ok)
	ok, err = cache.RefreshCodexDeviceConversationLease(ctx, "slot", "live")
	require.NoError(t, err)
	require.True(t, ok)
}

func TestCodexDeviceConversationCapacity(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache, ok := NewConcurrencyCache(client, 15, 900).(service.CodexDeviceConversationCapacityCache)
	require.True(t, ok)
	ctx := context.Background()
	for _, owner := range []string{"one", "two", "three"} {
		ok, err := cache.AcquireCodexDeviceConversationLeaseWithLimit(ctx, "slot", owner, 3)
		require.NoError(t, err)
		require.True(t, ok)
	}
	ok, err := cache.AcquireCodexDeviceConversationLeaseWithLimit(ctx, "slot", "four", 3)
	require.NoError(t, err)
	require.False(t, ok)
	require.NoError(t, cache.ReleaseCodexDeviceConversationLease(ctx, "slot", "one"))
	ok, err = cache.RefreshCodexDeviceConversationLease(ctx, "slot", "two")
	require.NoError(t, err)
	require.True(t, ok, "releasing one request must not cancel another")
	ok, err = cache.AcquireCodexDeviceConversationLeaseWithLimit(ctx, "slot", "four", 3)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = cache.AcquireCodexDeviceConversationLeaseWithLimit(ctx, "slot", "five", 1)
	require.NoError(t, err)
	require.False(t, ok, "lowering capacity must stop new admissions without evicting running requests")
	for _, owner := range []string{"five", "six", "seven"} {
		ok, err = cache.AcquireCodexDeviceConversationLeaseWithLimit(ctx, "slot", owner, 0)
		require.NoError(t, err)
		require.True(t, ok, "zero adds no slot-specific cap")
	}
}

func TestCodexDeviceConversationCapacityPreservesLegacyLease(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := NewConcurrencyCache(client, 15, 900).(service.CodexDeviceConversationCapacityCache)
	ctx := context.Background()
	require.NoError(t, client.Set(ctx, codexDeviceConversationLeaseKey("slot"), "legacy", time.Minute).Err())
	ok, err := cache.AcquireCodexDeviceConversationLeaseWithLimit(ctx, "slot", "new", 0)
	require.NoError(t, err)
	require.False(t, ok)
	ok, err = cache.RefreshCodexDeviceConversationLease(ctx, "slot", "legacy")
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, cache.ReleaseCodexDeviceConversationLease(ctx, "slot", "stranger"))
	require.Equal(t, "legacy", client.Get(ctx, codexDeviceConversationLeaseKey("slot")).Val())
	require.NoError(t, cache.ReleaseCodexDeviceConversationLease(ctx, "slot", "legacy"))
	ok, err = cache.AcquireCodexDeviceConversationLeaseWithLimit(ctx, "slot", "new", 3)
	require.NoError(t, err)
	require.True(t, ok)
}

func TestCodexDeviceConversationLeaseRedisOwnershipAndExpiry(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	regular := NewConcurrencyCache(client, 15, 900)
	leases, ok := regular.(service.CodexDeviceConversationLeaseCache)
	require.True(t, ok)
	ctx := context.Background()
	const slotKey = "account:91|profile:windows/desktop/x86_64|slot:0|epoch:4"

	acquired, err := leases.AcquireCodexDeviceConversationLease(ctx, slotKey, "owner-a")
	require.NoError(t, err)
	require.True(t, acquired)
	acquired, err = leases.AcquireCodexDeviceConversationLease(ctx, slotKey, "owner-b")
	require.NoError(t, err)
	require.False(t, acquired, "a second process cannot acquire the same device slot")

	refreshed, err := leases.RefreshCodexDeviceConversationLease(ctx, slotKey, "owner-b")
	require.NoError(t, err)
	require.False(t, refreshed, "a non-owner cannot refresh the lease")
	require.NoError(t, leases.ReleaseCodexDeviceConversationLease(ctx, slotKey, "owner-b"))
	acquired, err = leases.AcquireCodexDeviceConversationLease(ctx, slotKey, "owner-b")
	require.NoError(t, err)
	require.False(t, acquired, "a non-owner release cannot delete the lease")

	refreshed, err = leases.RefreshCodexDeviceConversationLease(ctx, slotKey, "owner-a")
	require.NoError(t, err)
	require.True(t, refreshed)
	require.NoError(t, leases.ReleaseCodexDeviceConversationLease(ctx, slotKey, "owner-a"))
	acquired, err = leases.AcquireCodexDeviceConversationLease(ctx, slotKey, "owner-b")
	require.NoError(t, err)
	require.True(t, acquired, "capacity must return immediately after owner release")

	redisServer.FastForward((codexDeviceConversationLeaseTTLSeconds + 1) * time.Second)
	refreshed, err = leases.RefreshCodexDeviceConversationLease(ctx, slotKey, "owner-b")
	require.NoError(t, err)
	require.False(t, refreshed)
	acquired, err = leases.AcquireCodexDeviceConversationLease(ctx, slotKey, "owner-c")
	require.NoError(t, err)
	require.True(t, acquired, "expired leases must not strand a device slot")
}
