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
	ownerUserID, accountID, platform, ok, err := cache.GetAnthropicFileOwner(ctx, "file_missing")
	require.NoError(t, err)
	require.False(t, ok)
	require.Zero(t, ownerUserID)
	require.Zero(t, accountID)
	require.Empty(t, platform)

	// 写入后可读回归属用户、账号与平台。
	require.NoError(t, cache.SetAnthropicFileOwner(ctx, "file_abc", 7, 63, "deepseek", 7*24*time.Hour))
	ownerUserID, accountID, platform, ok, err = cache.GetAnthropicFileOwner(ctx, "file_abc")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, int64(7), ownerUserID)
	require.Equal(t, int64(63), accountID)
	require.Equal(t, "deepseek", platform)

	// key 前缀、值编码与 TTL 符合预期：归属用户存在同一全局键的值里，
	// 是否跨用户由 service 层按 ownerUserID 判定。
	require.True(t, mr.Exists(anthropicFileOwnerPrefix+"file_abc"))
	ownerValue, getErr := mr.Get(anthropicFileOwnerPrefix + "file_abc")
	require.NoError(t, getErr)
	require.Equal(t, "7|63|deepseek", ownerValue)
	require.Equal(t, 7*24*time.Hour, mr.TTL(anthropicFileOwnerPrefix+"file_abc"))

	// 空 fileID / 空 platform / 非正账号 / 非正归属用户是 no-op，不写 key。
	require.NoError(t, cache.SetAnthropicFileOwner(ctx, "", 7, 63, "deepseek", time.Minute))
	require.NoError(t, cache.SetAnthropicFileOwner(ctx, "file_noplatform", 7, 63, "", time.Minute))
	require.NoError(t, cache.SetAnthropicFileOwner(ctx, "file_noaccount", 7, 0, "deepseek", time.Minute))
	require.NoError(t, cache.SetAnthropicFileOwner(ctx, "file_noowner", 0, 63, "deepseek", time.Minute))
	require.False(t, mr.Exists(anthropicFileOwnerPrefix+"file_noplatform"))
	require.False(t, mr.Exists(anthropicFileOwnerPrefix+"file_noaccount"))
	require.False(t, mr.Exists(anthropicFileOwnerPrefix+"file_noowner"))

	// 旧格式（只有 accountID|platform）按未知归属处理，不返回可硬钉结果。
	require.NoError(t, mr.Set(anthropicFileOwnerPrefix+"file_legacy", "63|deepseek"))
	_, _, _, ok, err = cache.GetAnthropicFileOwner(ctx, "file_legacy")
	require.NoError(t, err)
	require.False(t, ok)
}
