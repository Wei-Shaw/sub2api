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

func TestDeepseekFileStoreOwnershipAndExpiry(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	mr := miniredis.RunT(t)
	mr.SetTime(now)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := NewGatewayCache(client).(service.DeepseekFileStore)
	ctx := context.Background()
	f := service.DeepseekFile{ID: "f", AccountID: 63, Filename: "a.png", Format: "openai", CreatedAt: now, Bytes: 100}
	require.NoError(t, cache.SaveDeepseekFile(ctx, 7, f))
	tmp := f
	tmp.ID, tmp.ExpiresAt, tmp.Format = "temp", now.Add(48*time.Hour).Unix(), "anthropic"
	require.NoError(t, cache.SaveDeepseekFile(ctx, 8, tmp))
	require.Equal(t, 72*time.Hour, mr.TTL(deepseekUserFileIndex(8)+":temp"))
	require.Equal(t, 72*time.Hour, mr.TTL(deepseekUserFileIndex(8)))
	require.NoError(t, cache.DeleteDeepseekFile(ctx, 8, f.AccountID, f.ID))
	require.Error(t, cache.SaveDeepseekFile(ctx, 8, f), "conflicting IDs cannot replace an existing owner")
	separate := f
	separate.AccountID = 64
	require.NoError(t, cache.SaveDeepseekFile(ctx, 8, separate))
	scoped, err := cache.DeepseekFileAccounts(ctx, 8, []string{f.ID})
	require.NoError(t, err)
	require.Equal(t, int64(64), scoped[f.ID])
	require.NoError(t, cache.DeleteDeepseekFile(ctx, 8, 64, f.ID))
	files, err := cache.ListDeepseekFiles(ctx, 7)
	require.NoError(t, err)
	require.Equal(t, []service.DeepseekFile{f}, files)
	owners, err := cache.DeepseekFileAccounts(ctx, 7, []string{"f", "temp", "missing"})
	require.NoError(t, err)
	require.Equal(t, map[string]int64{"f": 63}, owners)
	mr.FastForward(72 * time.Hour)
	owners, err = cache.DeepseekFileAccounts(ctx, 8, []string{"temp"})
	require.NoError(t, err)
	require.Empty(t, owners)
	require.False(t, mr.Exists(deepseekUserFileIndex(8)))
	mr.FastForward(365 * 24 * time.Hour)
	files, err = cache.ListDeepseekFiles(ctx, 7)
	require.NoError(t, err)
	require.Equal(t, []service.DeepseekFile{f}, files)
	owners, err = cache.DeepseekFileAccounts(ctx, 7, []string{"f"})
	require.NoError(t, err)
	require.Equal(t, int64(63), owners["f"])
	require.NoError(t, cache.DeleteDeepseekFile(ctx, 7, f.AccountID, f.ID))
	require.Empty(t, mr.Keys())
}

func TestDeepseekFileStoreReadsLegacyOwners(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := NewGatewayCache(client).(service.DeepseekFileStore)
	require.NoError(t, mr.Set(deepseekFileOwnerPrefix+"owned", "7|12|deepseek"))
	require.NoError(t, mr.Set(deepseekFileOwnerPrefix+"unowned", "12|deepseek"))
	require.NoError(t, mr.Set(deepseekFileOwnerPrefix+"foreign", "8|12|deepseek"))
	require.NoError(t, mr.Set(deepseekFileOwnerPrefix+"other", "7|12|kimi"))
	ids, err := cache.DeepseekFileAccounts(context.Background(), 7, []string{"owned", "unowned", "foreign", "other"})
	require.NoError(t, err)
	require.Equal(t, map[string]int64{"owned": 12}, ids)
}
