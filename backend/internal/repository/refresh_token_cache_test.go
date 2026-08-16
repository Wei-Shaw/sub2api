//go:build unit

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

func newRefreshTokenCacheTest(t *testing.T) (*refreshTokenCache, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { require.NoError(t, rdb.Close()) })
	return &refreshTokenCache{rdb: rdb}, mr
}

func refreshTokenTestData(userID int64, familyID string) *service.RefreshTokenData {
	now := time.Now().UTC()
	return &service.RefreshTokenData{
		UserID:            userID,
		TokenVersion:      7,
		TokenVersionEpoch: 1,
		FamilyID:          familyID,
		CreatedAt:         now,
		ExpiresAt:         now.Add(time.Hour),
	}
}

func TestRefreshTokenCacheConsumeAllowsExactlyOneOf32ConcurrentCallers(t *testing.T) {
	cache, _ := newRefreshTokenCacheTest(t)
	ctx := context.Background()
	const (
		callers   = 32
		tokenHash = "concurrent-parent"
	)
	require.NoError(t, cache.StoreRefreshToken(ctx, tokenHash, refreshTokenTestData(42, "concurrent-family"), time.Hour))

	start := make(chan struct{})
	results := make(chan error, callers)
	for i := 0; i < callers; i++ {
		go func() {
			<-start
			data, err := cache.ConsumeRefreshToken(ctx, tokenHash)
			if data == nil || data.UserID != 42 || data.FamilyID != "concurrent-family" {
				results <- errors.New("consume did not return the authenticated tombstone data")
				return
			}
			results <- err
		}()
	}
	close(start)

	successes := 0
	replays := 0
	for i := 0; i < callers; i++ {
		err := <-results
		switch {
		case err == nil:
			successes++
		case errors.Is(err, service.ErrRefreshTokenReused):
			replays++
		default:
			require.NoError(t, err)
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, callers-1, replays)
}

func TestRefreshTokenCacheReplayRevokesWinnerChildAndBlocksFamily(t *testing.T) {
	cache, mr := newRefreshTokenCacheTest(t)
	ctx := context.Background()
	const (
		userID     = int64(43)
		familyID   = "replayed-family"
		parentHash = "replayed-parent"
		childHash  = "winner-child"
	)
	data := refreshTokenTestData(userID, familyID)
	require.NoError(t, cache.StoreRefreshToken(ctx, parentHash, data, time.Hour))

	consumed, err := cache.ConsumeRefreshToken(ctx, parentHash)
	require.NoError(t, err)
	require.Equal(t, data.UserID, consumed.UserID)
	require.True(t, mr.Exists(consumedRefreshTokenKey(parentHash)))
	require.Greater(t, mr.TTL(consumedRefreshTokenKey(parentHash)), time.Duration(0))

	child := *data
	child.CreatedAt = time.Now().UTC()
	child.ExpiresAt = child.CreatedAt.Add(time.Hour)
	require.NoError(t, cache.StoreRefreshToken(ctx, childHash, &child, time.Hour))

	replayed, err := cache.ConsumeRefreshToken(ctx, parentHash)
	require.ErrorIs(t, err, service.ErrRefreshTokenReused)
	require.Equal(t, data.UserID, replayed.UserID)
	require.True(t, mr.Exists(revokedTokenFamilyKey(familyID)))
	require.Greater(t, mr.TTL(revokedTokenFamilyKey(familyID)), time.Duration(0))

	_, err = cache.GetRefreshToken(ctx, childHash)
	require.ErrorIs(t, err, service.ErrRefreshTokenNotFound)
	require.False(t, mr.Exists(userRefreshTokensKey(userID)))
	require.False(t, mr.Exists(tokenFamilyKey(familyID)))
	require.ErrorIs(t,
		cache.StoreRefreshToken(ctx, "blocked-grandchild", &child, time.Hour),
		service.ErrRefreshTokenReused,
	)
	require.NoError(t, cache.AcknowledgeRefreshTokenReplay(ctx, parentHash))
	require.False(t, mr.Exists(consumedRefreshTokenKey(parentHash)))
	require.True(t, mr.Exists(revokedTokenFamilyKey(familyID)), "acknowledging the old token must retain family revocation")
	_, err = cache.ConsumeRefreshToken(ctx, parentHash)
	require.ErrorIs(t, err, service.ErrRefreshTokenNotFound)
}

func TestRefreshTokenCacheReplayBeforeChildStoreBlocksWinner(t *testing.T) {
	cache, mr := newRefreshTokenCacheTest(t)
	ctx := context.Background()
	data := refreshTokenTestData(44, "early-replay-family")
	require.NoError(t, cache.StoreRefreshToken(ctx, "early-parent", data, time.Millisecond))
	_, err := cache.ConsumeRefreshToken(ctx, "early-parent")
	require.NoError(t, err)
	require.GreaterOrEqual(t, mr.TTL(consumedRefreshTokenKey("early-parent")), refreshTombstoneFallbackTTL)
	_, err = cache.ConsumeRefreshToken(ctx, "early-parent")
	require.ErrorIs(t, err, service.ErrRefreshTokenReused)
	require.GreaterOrEqual(t, mr.TTL(revokedTokenFamilyKey(data.FamilyID)), refreshTombstoneFallbackTTL)
	require.ErrorIs(t,
		cache.StoreRefreshToken(ctx, "late-child", data, time.Hour),
		service.ErrRefreshTokenReused,
	)
}
