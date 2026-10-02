//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestModelDowngradeCounterCacheCountsPerAccountAndModel(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := NewModelDowngradeCounterCache(rdb)
	ctx := context.Background()

	for want := int64(1); want <= 3; want++ {
		got, err := cache.IncrementModelDowngradeCount(ctx, 224, "gpt-6-astra", 30)
		require.NoError(t, err)
		require.Equal(t, want, got)
	}
	// 模型名大小写和空格不拆出第二个计数器。
	got, err := cache.IncrementModelDowngradeCount(ctx, 224, " GPT-6-Astra ", 30)
	require.NoError(t, err)
	require.Equal(t, int64(4), got)

	// 同账号的其他模型、同模型的其他账号各自独立计数。
	got, err = cache.IncrementModelDowngradeCount(ctx, 224, "gpt-6-sol", 30)
	require.NoError(t, err)
	require.Equal(t, int64(1), got)
	got, err = cache.IncrementModelDowngradeCount(ctx, 225, "gpt-6-astra", 30)
	require.NoError(t, err)
	require.Equal(t, int64(1), got)

	// 窗口只在首次写入时设置，后续命中不会续期。
	require.Equal(t, 30*time.Minute, mr.TTL(modelDowngradeCounterKey(224, "gpt-6-astra")))

	require.NoError(t, cache.ResetModelDowngradeCount(ctx, 224, "GPT-6-ASTRA"))
	got, err = cache.IncrementModelDowngradeCount(ctx, 224, "gpt-6-astra", 30)
	require.NoError(t, err)
	require.Equal(t, int64(1), got)
}

func TestModelDowngradeCounterCacheWindowExpires(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := NewModelDowngradeCounterCache(rdb)
	ctx := context.Background()

	_, err := cache.IncrementModelDowngradeCount(ctx, 224, "gpt-6-astra", 1)
	require.NoError(t, err)
	mr.FastForward(61 * time.Second)

	got, err := cache.IncrementModelDowngradeCount(ctx, 224, "gpt-6-astra", 1)
	require.NoError(t, err)
	require.Equal(t, int64(1), got, "窗口过期后计数从 1 重新开始")
}
