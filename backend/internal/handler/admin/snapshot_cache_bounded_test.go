package admin

import (
	"fmt"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

func stopSnapshotCacheCleanup(c *snapshotCache) {
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.items)
	if c.cleanupTimer != nil {
		c.cleanupTimer.Stop()
		c.cleanupTimer = nil
	}
}

func TestBoundedSnapshotCacheCapacityAndRefresh(t *testing.T) {
	c := newBoundedSnapshotCache(time.Hour, 2)
	t.Cleanup(func() { stopSnapshotCacheCleanup(c) })
	c.Set("a", "old")
	c.Set("b", "keep")
	c.Set("a", "refreshed")

	require.Len(t, c.items, 2)
	_, ok := c.Get("b")
	require.True(t, ok, "refreshing an existing key must not evict another key")

	c.Set("c", "new")
	require.Len(t, c.items, 2)
	_, ok = c.Get("b")
	require.False(t, ok, "the oldest snapshot should be evicted")
	refreshed, ok := c.Get("a")
	require.True(t, ok)
	require.Equal(t, "refreshed", refreshed.Payload)
}

func TestBoundedSnapshotCacheExpiresWithoutReads(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newBoundedSnapshotCache(30*time.Second, 2)
		defer stopSnapshotCacheCleanup(c)
		c.Set("unvisited", "payload")

		time.Sleep(30 * time.Second)
		synctest.Wait()

		c.mu.RLock()
		defer c.mu.RUnlock()
		require.Empty(t, c.items)
		require.Nil(t, c.cleanupTimer, "an empty cache must not retain a cleanup timer")
	})
}

func TestBoundedSnapshotCacheRefreshSurvivesEarlierCleanup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newBoundedSnapshotCache(30*time.Second, 2)
		defer stopSnapshotCacheCleanup(c)
		c.Set("key", "old")
		time.Sleep(15 * time.Second)
		c.Set("key", "new")

		time.Sleep(15 * time.Second)
		synctest.Wait()
		entry, ok := c.Get("key")
		require.True(t, ok)
		require.Equal(t, "new", entry.Payload)

		time.Sleep(15 * time.Second)
		synctest.Wait()
		c.mu.RLock()
		defer c.mu.RUnlock()
		require.Empty(t, c.items)
		require.Nil(t, c.cleanupTimer)
	})
}

func TestBoundedSnapshotCachePurgesExpiredOnSet(t *testing.T) {
	c := newBoundedSnapshotCache(time.Hour, 2)
	t.Cleanup(func() { stopSnapshotCacheCleanup(c) })
	c.Set("expired", "old")
	c.mu.Lock()
	entry := c.items["expired"]
	entry.ExpiresAt = time.Now().Add(-time.Second)
	c.items["expired"] = entry
	c.mu.Unlock()

	c.Set("unrelated", "new")

	c.mu.RLock()
	defer c.mu.RUnlock()
	require.Len(t, c.items, 1)
	require.Contains(t, c.items, "unrelated")
}

func TestBoundedSnapshotCacheConcurrentCapacity(t *testing.T) {
	c := newBoundedSnapshotCache(time.Hour, maxAccountPerformanceSnapshots)
	t.Cleanup(func() { stopSnapshotCacheCleanup(c) })
	var wg sync.WaitGroup
	for worker := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 20 {
				key := fmt.Sprintf("%d:%d", worker, i)
				c.Set(key, i)
				c.Get(key)
			}
		}()
	}
	wg.Wait()
	c.mu.RLock()
	defer c.mu.RUnlock()
	require.Len(t, c.items, maxAccountPerformanceSnapshots)
}

func TestBoundedSnapshotCacheRejectsInvalidCapacity(t *testing.T) {
	for _, capacity := range []int{0, -1} {
		require.Panics(t, func() { newBoundedSnapshotCache(time.Second, capacity) })
	}
}

func TestSnapshotCacheUnboundedConstructorDoesNotStartCleanup(t *testing.T) {
	c := newSnapshotCache(time.Hour)
	c.Set("key", "value")
	require.Zero(t, c.maxEntries)
	require.Nil(t, c.cleanupTimer)
}
