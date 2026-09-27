//go:build unit

package repository

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newRTTConcurrencyCache(tb testing.TB, delay time.Duration) (*concurrencyCache, *redis.Client, *rttCountingHook) {
	tb.Helper()
	rdb, hook := newRTTRedisClient(tb, delay)
	cache, ok := NewConcurrencyCache(rdb, 15, 1800).(*concurrencyCache)
	require.True(tb, ok)
	return cache, rdb, hook
}

func TestConcurrencyCacheSlotRoundTrips(t *testing.T) {
	ctx := context.Background()
	cache, _, hook := newRTTConcurrencyCache(t, 0)

	// Warm up so script SHAs are cached server side (steady-state count).
	_, err := cache.AcquireAccountSlot(ctx, 1, 5, "warm")
	require.NoError(t, err)
	require.NoError(t, cache.ReleaseAccountSlot(ctx, 1, "warm"))
	require.NoError(t, cache.TrackAPIKeySlot(ctx, 1, "warm"))

	hook.n.Store(0)
	ok, err := cache.AcquireAccountSlot(ctx, 11, 5, "req")
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, cache.ReleaseAccountSlot(ctx, 11, "req"))
	require.Equal(t, int64(2), hook.n.Load(), "account acquire+release")

	hook.n.Store(0)
	ok, err = cache.AcquireUserSlot(ctx, 21, 5, "req")
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, cache.ReleaseUserSlot(ctx, 21, "req"))
	require.Equal(t, int64(2), hook.n.Load(), "user acquire+release")

	hook.n.Store(0)
	require.NoError(t, cache.TrackAPIKeySlot(ctx, 31, "req"))
	require.NoError(t, cache.ReleaseAPIKeySlot(ctx, 31, "req"))
	require.Equal(t, int64(2), hook.n.Load(), "api key track+release")
}

func TestConcurrencyCacheSlotActiveIndexLifecycle(t *testing.T) {
	ctx := context.Background()
	cache, rdb, _ := newRTTConcurrencyCache(t, 0)
	member := "41"

	ok, err := cache.AcquireAccountSlot(ctx, 41, 5, "a")
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = cache.AcquireAccountSlot(ctx, 41, 5, "b")
	require.NoError(t, err)
	require.True(t, ok)
	now, err := rdb.Time(ctx).Result()
	require.NoError(t, err)
	score, err := rdb.ZScore(ctx, accountActiveIndexKey, member).Result()
	require.NoError(t, err)
	require.InDelta(t, float64(now.Unix()+int64(cache.slotTTLSeconds)), score, 2)

	// One slot still held: index keeps the member with slot TTL.
	require.NoError(t, cache.ReleaseAccountSlot(ctx, 41, "a"))
	score, err = rdb.ZScore(ctx, accountActiveIndexKey, member).Result()
	require.NoError(t, err)
	require.InDelta(t, float64(now.Unix()+int64(cache.slotTTLSeconds)), score, 2)

	// No slot but waiters: index keeps the member with the (longer) wait TTL.
	ok, err = cache.IncrementAccountWaitCount(ctx, 41, 10)
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, cache.ReleaseAccountSlot(ctx, 41, "b"))
	score, err = rdb.ZScore(ctx, accountActiveIndexKey, member).Result()
	require.NoError(t, err)
	require.InDelta(t, float64(now.Unix()+int64(cache.waitQueueTTLSeconds)), score, 2)
	require.EqualValues(t, 0, rdb.ZCard(ctx, accountSlotKey(41)).Val())

	// Nothing left: index member removed.
	require.NoError(t, cache.DecrementAccountWaitCount(ctx, 41))
	_, err = rdb.ZScore(ctx, accountActiveIndexKey, member).Result()
	require.ErrorIs(t, err, redis.Nil)

	// Same lifecycle on the user index; expired slots are reaped on release.
	ok, err = cache.AcquireUserSlot(ctx, 42, 5, "u")
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, rdb.ZAdd(ctx, userSlotKey(42), redis.Z{Score: float64(now.Unix() - int64(cache.slotTTLSeconds) - 10), Member: "stale"}).Err())
	_, err = rdb.ZScore(ctx, userActiveIndexKey, "42").Result()
	require.NoError(t, err)
	require.NoError(t, cache.ReleaseUserSlot(ctx, 42, "u"))
	_, err = rdb.ZScore(ctx, userActiveIndexKey, "42").Result()
	require.ErrorIs(t, err, redis.Nil)
	require.EqualValues(t, 0, rdb.ZCard(ctx, userSlotKey(42)).Val())
}

func TestConcurrencyCacheParallelAcquireReleaseDoesNotLeakSlots(t *testing.T) {
	ctx := context.Background()
	cache, rdb, _ := newRTTConcurrencyCache(t, 0)
	const (
		workers   = 100
		maxSlots  = 10
		accountID = 51
		userID    = 52
	)
	var inFlight, peak atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			requestID := fmt.Sprintf("req-%d", i)
			for {
				ok, err := cache.AcquireAccountSlot(ctx, accountID, maxSlots, requestID)
				if err != nil {
					t.Errorf("acquire account: %v", err)
					return
				}
				if ok {
					break
				}
				time.Sleep(time.Millisecond)
			}
			if _, err := cache.AcquireUserSlot(ctx, userID, workers, requestID); err != nil {
				t.Errorf("acquire user: %v", err)
			}
			cur := inFlight.Add(1)
			for {
				old := peak.Load()
				if cur <= old || peak.CompareAndSwap(old, cur) {
					break
				}
			}
			inFlight.Add(-1)
			if err := cache.ReleaseUserSlot(ctx, userID, requestID); err != nil {
				t.Errorf("release user: %v", err)
			}
			if err := cache.ReleaseAccountSlot(ctx, accountID, requestID); err != nil {
				t.Errorf("release account: %v", err)
			}
		}(i)
	}
	wg.Wait()

	require.LessOrEqual(t, peak.Load(), int64(maxSlots))
	require.EqualValues(t, 0, rdb.ZCard(ctx, accountSlotKey(accountID)).Val())
	require.EqualValues(t, 0, rdb.ZCard(ctx, userSlotKey(userID)).Val())
	_, err := rdb.ZScore(ctx, accountActiveIndexKey, strconv.Itoa(accountID)).Result()
	require.ErrorIs(t, err, redis.Nil)
	_, err = rdb.ZScore(ctx, userActiveIndexKey, strconv.Itoa(userID)).Result()
	require.ErrorIs(t, err, redis.Nil)
}

// BenchmarkAcquireRelease runs one request's slot lifecycle (account + user + API key)
// with 200µs simulated network latency per round trip.
func BenchmarkAcquireRelease(b *testing.B) {
	ctx := context.Background()
	cache, _, _ := newRTTConcurrencyCache(b, 200*time.Microsecond)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		requestID := strconv.Itoa(i)
		if ok, err := cache.AcquireAccountSlot(ctx, 61, 10, requestID); err != nil || !ok {
			b.Fatalf("acquire account ok=%v err=%v", ok, err)
		}
		if ok, err := cache.AcquireUserSlot(ctx, 62, 10, requestID); err != nil || !ok {
			b.Fatalf("acquire user ok=%v err=%v", ok, err)
		}
		if err := cache.TrackAPIKeySlot(ctx, 63, requestID); err != nil {
			b.Fatal(err)
		}
		if err := cache.ReleaseAPIKeySlot(ctx, 63, requestID); err != nil {
			b.Fatal(err)
		}
		if err := cache.ReleaseUserSlot(ctx, 62, requestID); err != nil {
			b.Fatal(err)
		}
		if err := cache.ReleaseAccountSlot(ctx, 61, requestID); err != nil {
			b.Fatal(err)
		}
	}
}
