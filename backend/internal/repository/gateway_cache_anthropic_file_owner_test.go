package repository

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestGatewayCacheAnthropicFileOwner(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache := NewGatewayCache(client)
	ctx := context.Background()

	// 未命中返回 ok=false 且不是错误。
	accountID, platform, ok, err := cache.GetAnthropicFileOwner(ctx, "file_missing")
	require.NoError(t, err)
	require.False(t, ok)
	require.Zero(t, accountID)
	require.Empty(t, platform)

	// 写入后可读回账号与平台。
	require.NoError(t, cache.SetAnthropicFileOwner(ctx, "file_abc", 63, "deepseek", 7*24*time.Hour))
	accountID, platform, ok, err = cache.GetAnthropicFileOwner(ctx, "file_abc")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, int64(63), accountID)
	require.Equal(t, "deepseek", platform)

	// key 前缀与 TTL 符合预期。
	require.True(t, mr.Exists(anthropicFileOwnerPrefix+"file_abc"))
	require.Equal(t, 7*24*time.Hour, mr.TTL(anthropicFileOwnerPrefix+"file_abc"))

	// 空 fileID / 空 platform / 非正账号是 no-op，不写 key。
	require.NoError(t, cache.SetAnthropicFileOwner(ctx, "", 63, "deepseek", time.Minute))
	require.NoError(t, cache.SetAnthropicFileOwner(ctx, "file_noplatform", 63, "", time.Minute))
	require.NoError(t, cache.SetAnthropicFileOwner(ctx, "file_noaccount", 0, "deepseek", time.Minute))
	require.False(t, mr.Exists(anthropicFileOwnerPrefix+"file_noplatform"))
	require.False(t, mr.Exists(anthropicFileOwnerPrefix+"file_noaccount"))
}
