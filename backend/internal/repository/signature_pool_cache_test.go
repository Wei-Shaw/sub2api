//go:build unit

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service/signature"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func setupPoolTest(t *testing.T, softTTL time.Duration) (context.Context, signature.SignaturePool) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return context.Background(), NewSignaturePoolCache(rdb, softTTL)
}

func TestSignaturePool_AddAndTopN(t *testing.T) {
	ctx, pool := setupPoolTest(t, time.Hour)
	now := time.Now()

	require.NoError(t, pool.Add(ctx, "b", "A", now, 10))
	require.NoError(t, pool.Add(ctx, "b", "B", now.Add(time.Second), 10))
	require.NoError(t, pool.Add(ctx, "b", "C", now.Add(2*time.Second), 10))

	sigs, err := pool.TopN(ctx, "b", 3)
	require.NoError(t, err)
	require.Equal(t, []string{"C", "B", "A"}, sigs)
}

func TestSignaturePool_TopN_FewerThanN(t *testing.T) {
	ctx, pool := setupPoolTest(t, time.Hour)
	require.NoError(t, pool.Add(ctx, "b", "only", time.Now(), 10))

	sigs, err := pool.TopN(ctx, "b", 5)
	require.NoError(t, err)
	require.Equal(t, []string{"only"}, sigs)
}

func TestSignaturePool_TopN_EmptyBucket(t *testing.T) {
	ctx, pool := setupPoolTest(t, time.Hour)
	sigs, err := pool.TopN(ctx, "empty", 10)
	require.NoError(t, err)
	require.Empty(t, sigs)
}

func TestSignaturePool_CapacityTrim(t *testing.T) {
	ctx, pool := setupPoolTest(t, time.Hour)
	now := time.Now()

	for i := 0; i < 6; i++ {
		require.NoError(t, pool.Add(ctx, "b", fmt.Sprintf("s%d", i), now.Add(time.Duration(i)*time.Second), 3))
	}

	sigs, err := pool.TopN(ctx, "b", 10)
	require.NoError(t, err)
	require.Equal(t, []string{"s5", "s4", "s3"}, sigs)

	sz, err := pool.Size(ctx, "b")
	require.NoError(t, err)
	require.Equal(t, int64(3), sz)
}

func TestSignaturePool_LazyExpiry(t *testing.T) {
	ctx, pool := setupPoolTest(t, time.Hour)

	oldTime := time.Now().Add(-2 * time.Hour)
	require.NoError(t, pool.Add(ctx, "b", "old", oldTime, 100))

	sz, _ := pool.Size(ctx, "b")
	require.Equal(t, int64(1), sz, "old entry exists before cleanup")

	require.NoError(t, pool.Add(ctx, "b", "new", time.Now(), 100))

	sigs, err := pool.TopN(ctx, "b", 10)
	require.NoError(t, err)
	require.Equal(t, []string{"new"}, sigs)

	sz, _ = pool.Size(ctx, "b")
	require.Equal(t, int64(1), sz)
}

func TestSignaturePool_TopN_NoTTLFilter(t *testing.T) {
	ctx, pool := setupPoolTest(t, time.Hour)

	oldTime := time.Now().Add(-2 * time.Hour)
	require.NoError(t, pool.Add(ctx, "b", "stale", oldTime, 100))

	sigs, err := pool.TopN(ctx, "b", 10)
	require.NoError(t, err)
	require.Equal(t, []string{"stale"}, sigs, "TopN must not filter by TTL")
}

func TestSignaturePool_DuplicateUpdatesScore(t *testing.T) {
	ctx, pool := setupPoolTest(t, time.Hour)
	now := time.Now()

	require.NoError(t, pool.Add(ctx, "b", "A", now, 10))
	require.NoError(t, pool.Add(ctx, "b", "B", now.Add(time.Second), 10))
	require.NoError(t, pool.Add(ctx, "b", "A", now.Add(2*time.Second), 10))

	sigs, err := pool.TopN(ctx, "b", 10)
	require.NoError(t, err)
	require.Equal(t, []string{"A", "B"}, sigs)

	sz, _ := pool.Size(ctx, "b")
	require.Equal(t, int64(2), sz)
}

func TestSignaturePool_EmptyInputNoop(t *testing.T) {
	ctx, pool := setupPoolTest(t, time.Hour)
	require.NoError(t, pool.Add(ctx, "", "sig", time.Now(), 10))
	require.NoError(t, pool.Add(ctx, "b", "", time.Now(), 10))

	sigs, _ := pool.TopN(ctx, "", 10)
	require.Empty(t, sigs)
}

func TestSignaturePool_ZeroN(t *testing.T) {
	ctx, pool := setupPoolTest(t, time.Hour)
	require.NoError(t, pool.Add(ctx, "b", "sig", time.Now(), 10))

	sigs, err := pool.TopN(ctx, "b", 0)
	require.NoError(t, err)
	require.Empty(t, sigs)
}

func TestSignaturePool_Size(t *testing.T) {
	ctx, pool := setupPoolTest(t, time.Hour)
	now := time.Now()

	sz, _ := pool.Size(ctx, "b")
	require.Equal(t, int64(0), sz)

	for i := 0; i < 5; i++ {
		require.NoError(t, pool.Add(ctx, "b", fmt.Sprintf("s%d", i), now.Add(time.Duration(i)*time.Second), 100))
	}
	sz, _ = pool.Size(ctx, "b")
	require.Equal(t, int64(5), sz)
}
