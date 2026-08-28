//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newConnectionSignalTestCache(t *testing.T) (*connectionSignalCache, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return &connectionSignalCache{rdb: rdb}, mr
}

func TestConnectionSignalCache_EmitAlwaysOnCmdBudget(t *testing.T) {
	cache, _ := newConnectionSignalTestCache(t)
	ctx := context.Background()
	now := time.Now().Unix()
	cmds, err := cache.EmitAlwaysOn(ctx, service.ConnectionSignal{
		UserID: 1, APIKeyID: 42, IP: "203.0.113.10", UAHash: "abcdef0123456789", NowUnix: now,
	}, 50000, 32, 1)
	require.NoError(t, err)
	// Base always-on ≈19 + owner SET (+ optional prefix SET).
	// seq=1 with N=32 → no prune.
	require.LessOrEqual(t, cmds, 22, "always-on pipeline should stay near design budget")
	require.GreaterOrEqual(t, cmds, 19)

	owner, err := cache.GetKeyOwner(ctx, 42)
	require.NoError(t, err)
	require.Equal(t, int64(1), owner)
}

func TestConnectionSignalCache_UASlidingWindow1h(t *testing.T) {
	cache, mr := newConnectionSignalTestCache(t)
	ctx := context.Background()

	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC).Unix()
	// Three UAs appear at t0
	for i, ua := range []string{"ua-a", "ua-b", "ua-c"} {
		_, err := cache.EmitAlwaysOn(ctx, service.ConnectionSignal{
			UserID: 7, APIKeyID: 99, IP: "198.51.100.1", UAHash: ua, NowUnix: base + int64(i),
		}, 50000, 9999, uint64(i+1))
		require.NoError(t, err)
	}

	// Fast-forward wall clock for miniredis TTL, but scoring uses NowUnix for window.
	mr.FastForward(2 * time.Hour)

	// At base+90min, old UAs (lastSeen ~base) must drop out of the 1h window.
	later := base + 90*60
	// Refresh only ua-c recently
	_, err := cache.EmitAlwaysOn(ctx, service.ConnectionSignal{
		UserID: 7, APIKeyID: 99, IP: "198.51.100.2", UAHash: "ua-c", NowUnix: later,
	}, 50000, 9999, 100)
	require.NoError(t, err)

	m, err := cache.ReadKeyWindowMetrics(ctx, 99, 7, later)
	require.NoError(t, err)
	require.Equal(t, 1, m.UACount1h, "only UA seen within last 1h should count (not cumulative lifetime)")
}

func TestConnectionSignalCache_BucketedMetricsAdvanceWithTime(t *testing.T) {
	cache, _ := newConnectionSignalTestCache(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC).Unix()

	emit := func(keyID int64, address string, now int64, seq uint64) {
		_, err := cache.EmitAlwaysOn(ctx, service.ConnectionSignal{
			UserID: 7, APIKeyID: keyID, IP: address, UAHash: "ua", NowUnix: now,
		}, 50000, 9999, seq)
		require.NoError(t, err)
	}
	emit(42, "198.51.100.1", base, 1)
	emit(42, "198.51.100.2", base+1, 2)
	emit(43, "198.51.100.3", base+2, 3)

	m, err := cache.ReadKeyWindowMetrics(ctx, 42, 7, base+2)
	require.NoError(t, err)
	require.EqualValues(t, 2, m.IPHll1h)
	require.EqualValues(t, 2, m.UserKeys1h)
	require.EqualValues(t, 3, m.UserIPHLL1h)

	// Old buckets still exist in Redis, but are not part of the bounded 1h union.
	twoHoursLater := base + 2*60*60
	emit(42, "203.0.113.10", twoHoursLater, 4)
	m, err = cache.ReadKeyWindowMetrics(ctx, 42, 7, twoHoursLater)
	require.NoError(t, err)
	require.EqualValues(t, 1, m.IPHll1h)
	require.EqualValues(t, 1, m.UserKeys1h)
	require.EqualValues(t, 1, m.UserIPHLL1h)
	require.EqualValues(t, 3, m.IPHll24h)

	// At +25h, the original bucket is outside 24h while the +2h sample remains.
	twentyFiveHoursLater := base + 25*60*60
	emit(42, "203.0.113.11", twentyFiveHoursLater, 5)
	m, err = cache.ReadKeyWindowMetrics(ctx, 42, 7, twentyFiveHoursLater)
	require.NoError(t, err)
	require.EqualValues(t, 1, m.IPHll1h)
	require.EqualValues(t, 2, m.IPHll24h)
}

func TestConnectionSignalCache_BucketedMetricsBoundaryCoverage(t *testing.T) {
	cache, _ := newConnectionSignalTestCache(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC).Unix()

	_, err := cache.EmitAlwaysOn(ctx, service.ConnectionSignal{
		UserID: 7, APIKeyID: 42, IP: "198.51.100.1", UAHash: "ua", NowUnix: base,
	}, 50000, 9999, 1)
	require.NoError(t, err)

	tests := []struct {
		name    string
		at      int64
		want1h  int64
		want24h int64
	}{
		{name: "just before one hour", at: base + int64(59*time.Minute+59*time.Second)/int64(time.Second), want1h: 1, want24h: 1},
		{name: "just after one hour remains in overlap bucket", at: base + int64(60*time.Minute+time.Second)/int64(time.Second), want1h: 1, want24h: 1},
		{name: "after one hour overlap expires", at: base + int64(65*time.Minute+time.Second)/int64(time.Second), want1h: 0, want24h: 1},
		{name: "just before twenty four hours", at: base + int64(23*time.Hour+59*time.Minute)/int64(time.Second), want1h: 0, want24h: 1},
		{name: "just after twenty four hours remains in overlap bucket", at: base + int64(24*time.Hour+time.Second)/int64(time.Second), want1h: 0, want24h: 1},
		{name: "after twenty four hour overlap expires", at: base + int64(25*time.Hour+time.Second)/int64(time.Second), want1h: 0, want24h: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics, err := cache.ReadKeyWindowMetrics(ctx, 42, 7, tt.at)
			require.NoError(t, err)
			require.EqualValues(t, tt.want1h, metrics.IPHll1h)
			require.EqualValues(t, tt.want24h, metrics.IPHll24h)
		})
	}
}

func TestConnectionSignalCache_ReadMetricsReturnsRedisCommandError(t *testing.T) {
	cache, mr := newConnectionSignalTestCache(t)
	ctx := context.Background()
	now := time.Now().Unix()

	// A wrong Redis type on a pipeline metric must fail the snapshot instead of
	// being silently interpreted as a zero count.
	mr.Set(crKeyIPs1h(42, now/crOneHourBucket), "not-a-hyperloglog")
	metrics, err := cache.ReadKeyWindowMetrics(ctx, 42, 7, now)
	require.Error(t, err)
	require.Nil(t, metrics)
}

func TestConnectionSignalCache_ReadMetricsAllowsMissingCounters(t *testing.T) {
	cache, _ := newConnectionSignalTestCache(t)

	metrics, err := cache.ReadKeyWindowMetrics(context.Background(), 42, 7, time.Now().Unix())
	require.NoError(t, err)
	require.NotNil(t, metrics)
	require.Zero(t, metrics.ReqCount5m)
	require.Zero(t, metrics.SBMismatch15m)
}

func TestConnectionSignalCache_SessionMismatchAndExempt(t *testing.T) {
	cache, _ := newConnectionSignalTestCache(t)
	ctx := context.Background()
	require.NoError(t, cache.IncrSessionMismatch(ctx, 5))
	require.NoError(t, cache.IncrSessionMismatch(ctx, 5))

	require.NoError(t, cache.SetExempt(ctx, "k", 9, "test", time.Hour))
	ok, err := cache.IsExempt(ctx, "k", 9)
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, cache.ClearExempt(ctx, "k", 9))
	ok, err = cache.IsExempt(ctx, "k", 9)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestConnectionSignalCache_Dedupe(t *testing.T) {
	cache, _ := newConnectionSignalTestCache(t)
	ctx := context.Background()
	ok, err := cache.TryDedupe(ctx, "k:1:R1:1", time.Minute)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = cache.TryDedupe(ctx, "k:1:R1:1", time.Minute)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestConnectionSignalCache_ActivePruneCap(t *testing.T) {
	cache, _ := newConnectionSignalTestCache(t)
	ctx := context.Background()
	now := time.Now().Unix()
	for i := 1; i <= 5; i++ {
		_, err := cache.EmitAlwaysOn(ctx, service.ConnectionSignal{
			UserID: int64(i), APIKeyID: int64(100 + i), IP: "203.0.113.1", UAHash: "x", NowUnix: now + int64(i),
		}, 3, 1, uint64(i)) // prune every emit, max 3
		require.NoError(t, err)
	}
	require.NoError(t, cache.PruneActive(ctx, 3, time.Hour))
	keys, users, err := cache.ActiveCards(ctx)
	require.NoError(t, err)
	require.LessOrEqual(t, keys, int64(3))
	require.LessOrEqual(t, users, int64(3))
}
