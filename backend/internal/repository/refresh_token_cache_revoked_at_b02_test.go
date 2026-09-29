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

func TestB02RefreshTokenCacheUserTokensRevokedAtRoundTrip(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	store, ok := NewRefreshTokenCache(rdb).(service.UserTokenRevocationStore)
	require.True(t, ok, "refresh token cache must implement UserTokenRevocationStore")

	ctx := context.Background()
	got, err := store.GetUserTokensRevokedAt(ctx, 42)
	require.NoError(t, err)
	require.Zero(t, got)

	require.NoError(t, store.SetUserTokensRevokedAt(ctx, 42, 1700000000, 2*time.Hour))
	got, err = store.GetUserTokensRevokedAt(ctx, 42)
	require.NoError(t, err)
	require.Equal(t, int64(1700000000), got)
	require.Equal(t, 2*time.Hour, mr.TTL(userTokensRevokedAtKey(42)))

	mr.FastForward(2*time.Hour + time.Second)
	got, err = store.GetUserTokensRevokedAt(ctx, 42)
	require.NoError(t, err)
	require.Zero(t, got)
}
