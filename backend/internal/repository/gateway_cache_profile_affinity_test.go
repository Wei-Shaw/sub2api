package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestGatewayCacheProfileIndexMissLeavesLegacyStickyReadable(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	cache := NewGatewayCache(client)
	ctx := context.Background()
	const groupID int64 = 3
	const legacyKey = "openai:legacy-session-hash"
	const profileIndexKey = "codex-profile:index:missing"
	require.NoError(t, cache.SetSessionAccountID(ctx, groupID, legacyKey, 99, time.Hour))

	accountID, err := cache.GetSessionAccountID(ctx, groupID, profileIndexKey)
	require.Zero(t, accountID)
	require.True(t, errors.Is(err, service.ErrStickySessionNotFound))
	accountID, err = cache.GetSessionAccountID(ctx, groupID, legacyKey)
	require.NoError(t, err)
	require.Equal(t, int64(99), accountID)
}
