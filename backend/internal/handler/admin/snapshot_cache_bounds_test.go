package admin

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSnapshotCacheBoundsAndReplacement(t *testing.T) {
	c := newSnapshotCache(time.Minute)
	c.maxEntries = 3
	c.maxCost = 32
	for i := 0; i < 100; i++ {
		c.Set(fmt.Sprint(i), "value")
		require.LessOrEqual(t, len(c.items), 3)
		require.LessOrEqual(t, c.cost, 32)
		require.Equal(t, len(c.items), c.order.Len())
	}
	_, ok := c.Get("0")
	require.False(t, ok)
	_, ok = c.Get("99")
	require.True(t, ok)
	for i := 0; i < 100; i++ {
		c.Set("99", "new")
	}
	require.Equal(t, len(c.items), len(c.positions))
	require.Equal(t, len(c.items), len(c.costs))
	require.Equal(t, len(c.items), c.order.Len())
	c.Set("large", strings.Repeat("x", 33))
	_, ok = c.Get("large")
	require.False(t, ok)
	c.Set("99", make(chan int))
	_, ok = c.Get("99")
	require.False(t, ok, "uncacheable replacement must not leave old data")
}

func TestSnapshotCachePayloadBudgetAndExpiry(t *testing.T) {
	c := newSnapshotCache(time.Minute)
	c.maxCost = 16
	c.Set("a", "123456") // 9 bytes including JSON quotes and key.
	c.Set("b", "123456")
	_, ok := c.Get("a")
	require.False(t, ok)
	require.Equal(t, 9, c.cost)
	c.mu.Lock()
	e := c.items["b"]
	e.ExpiresAt = time.Now().Add(-time.Second)
	c.items["b"] = e
	c.mu.Unlock()
	c.Set("c", "v")
	_, ok = c.Get("b")
	require.False(t, ok)
	require.Equal(t, 1, c.order.Len())
	require.Equal(t, 4, c.cost)
}

func TestSnapshotCacheConcurrentExpiredReplacement(t *testing.T) {
	c := newSnapshotCache(time.Hour)
	for i := 0; i < 100; i++ {
		c.Set("shared", "old")
		c.mu.Lock()
		e := c.items["shared"]
		e.ExpiresAt = time.Now().Add(-time.Second)
		c.items["shared"] = e
		c.mu.Unlock()
		var wg sync.WaitGroup
		start := make(chan struct{})
		for j := 0; j < 8; j++ {
			wg.Add(1)
			go func() { defer wg.Done(); <-start; c.Get("shared") }()
		}
		wg.Add(1)
		go func() { defer wg.Done(); <-start; c.Set("shared", "fresh") }()
		close(start)
		wg.Wait()
		e, ok := c.Get("shared")
		require.True(t, ok)
		require.Equal(t, "fresh", e.Payload)
	}
}
