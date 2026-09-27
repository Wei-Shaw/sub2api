//go:build unit

package repository

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// rttCountingHook counts client->Redis round trips: one per command, one per pipeline.
// delay simulates network latency per round trip (busy-wait for sub-ms precision).
type rttCountingHook struct {
	n     atomic.Int64
	delay time.Duration
}

func (h *rttCountingHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (h *rttCountingHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		h.roundTrip()
		return next(ctx, cmd)
	}
}

func (h *rttCountingHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		h.roundTrip()
		return next(ctx, cmds)
	}
}

func (h *rttCountingHook) roundTrip() {
	h.n.Add(1)
	for start := time.Now(); time.Since(start) < h.delay; {
	}
}

func newRTTRedisClient(tb testing.TB, delay time.Duration) (*redis.Client, *rttCountingHook) {
	tb.Helper()
	mr := miniredis.RunT(tb)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	tb.Cleanup(func() { _ = rdb.Close() })
	hook := &rttCountingHook{delay: delay}
	rdb.AddHook(hook)
	return rdb, hook
}

func seedSchedulerSnapshotForRTT(tb testing.TB, cache *schedulerCache, bucket service.SchedulerBucket, n int) {
	tb.Helper()
	ctx := context.Background()
	accounts := make([]service.Account, 0, n)
	lastUsed := make(map[int64]time.Time, n)
	base := time.Now().UTC().Truncate(time.Millisecond)
	for i := 1; i <= n; i++ {
		id := int64(7000 + i)
		accounts = append(accounts, service.Account{
			ID:          id,
			Name:        "rtt",
			Platform:    service.PlatformAnthropic,
			Type:        service.AccountTypeAPIKey,
			Status:      service.StatusActive,
			Schedulable: true,
			Priority:    i,
		})
		lastUsed[id] = base.Add(time.Duration(i) * time.Second)
	}
	token, err := cache.CaptureBucketWriteToken(ctx, bucket)
	require.NoError(tb, err)
	require.NoError(tb, cache.SetSnapshot(ctx, bucket, token, accounts))
	require.NoError(tb, cache.UpdateLastUsed(ctx, lastUsed))
}

func TestSchedulerCacheGetSnapshotRoundTrips(t *testing.T) {
	ctx := context.Background()
	rdb, hook := newRTTRedisClient(t, 0)
	// Small chunk size so the bucket spans several MGET chunks per key family.
	cache, ok := newSchedulerCacheWithChunkSizes(rdb, 128, defaultSchedulerSnapshotWriteChunkSize).(*schedulerCache)
	require.True(t, ok)
	bucket := service.SchedulerBucket{GroupID: 31, Platform: service.PlatformAnthropic, Mode: service.SchedulerModeSingle}
	const n = 300
	seedSchedulerSnapshotForRTT(t, cache, bucket, n)

	// Warm up once (steady-state count).
	_, hit, err := cache.GetSnapshot(ctx, bucket)
	require.NoError(t, err)
	require.True(t, hit)

	hook.n.Store(0)
	accounts, hit, err := cache.GetSnapshot(ctx, bucket)
	require.NoError(t, err)
	require.True(t, hit)
	require.Equal(t, int64(3), hook.n.Load(), "GetSnapshot must cost 3 round trips (MGET state, ZRANGE, pipelined MGETs) regardless of chunk count")
	require.Len(t, accounts, n)
	for i, account := range accounts {
		require.Equal(t, int64(7001+i), account.ID)
		require.NotNil(t, account.LastUsedAt)
	}

	// A bucket that is not ready is a miss after a single round trip.
	hook.n.Store(0)
	missing := service.SchedulerBucket{GroupID: 32, Platform: service.PlatformAnthropic, Mode: service.SchedulerModeSingle}
	accounts, hit, err = cache.GetSnapshot(ctx, missing)
	require.NoError(t, err)
	require.False(t, hit)
	require.Nil(t, accounts)
	require.Equal(t, int64(1), hook.n.Load())
}

func TestSchedulerCacheGetSnapshotMissesWhenActiveOrMetaMissing(t *testing.T) {
	ctx := context.Background()
	rdb, _ := newRTTRedisClient(t, 0)
	cache, ok := newSchedulerCacheWithChunkSizes(rdb, 2, defaultSchedulerSnapshotWriteChunkSize).(*schedulerCache)
	require.True(t, ok)
	bucket := service.SchedulerBucket{GroupID: 33, Platform: service.PlatformAnthropic, Mode: service.SchedulerModeSingle}
	seedSchedulerSnapshotForRTT(t, cache, bucket, 5)

	// Missing meta payload for one member invalidates the whole snapshot.
	require.NoError(t, rdb.Del(ctx, schedulerAccountMetaKey("7004")).Err())
	accounts, hit, err := cache.GetSnapshot(ctx, bucket)
	require.NoError(t, err)
	require.False(t, hit)
	require.Nil(t, accounts)

	// Ready flag without an active version is a miss.
	require.NoError(t, rdb.Del(ctx, schedulerBucketKey(schedulerActivePrefix, bucket)).Err())
	accounts, hit, err = cache.GetSnapshot(ctx, bucket)
	require.NoError(t, err)
	require.False(t, hit)
	require.Nil(t, accounts)
}

// BenchmarkGetSnapshot simulates 200µs network latency per round trip.
func BenchmarkGetSnapshot(b *testing.B) {
	ctx := context.Background()
	rdb, _ := newRTTRedisClient(b, 200*time.Microsecond)
	cache, ok := newSchedulerCacheWithChunkSizes(rdb, defaultSchedulerSnapshotMGetChunkSize, defaultSchedulerSnapshotWriteChunkSize).(*schedulerCache)
	require.True(b, ok)
	bucket := service.SchedulerBucket{GroupID: 34, Platform: service.PlatformAnthropic, Mode: service.SchedulerModeSingle}
	seedSchedulerSnapshotForRTT(b, cache, bucket, 50)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, hit, err := cache.GetSnapshot(ctx, bucket); err != nil || !hit {
			b.Fatalf("GetSnapshot hit=%v err=%v", hit, err)
		}
	}
}
