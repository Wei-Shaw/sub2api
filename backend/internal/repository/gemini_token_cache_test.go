//go:build unit

package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestGeminiTokenCacheEncryptsAccessTokensAndRejectsTamperAndWrongAAD(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cipher := testCredentialCipher(t, "8", "primary")
	cache := newGeminiTokenCache(rdb, cipher)
	cacheKey := "openai:account:42"
	token := "provider-access-token-secret"

	require.NoError(t, cache.SetAccessToken(ctx, cacheKey, token, time.Hour))
	redisKey := encryptedOAuthTokenKeyPrefix + cacheKey
	firstStorage, err := mr.Get(redisKey)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(firstStorage, credentialCacheSecretPrefix))
	require.NotContains(t, firstStorage, token)

	got, err := cache.GetAccessToken(ctx, cacheKey)
	require.NoError(t, err)
	require.Equal(t, token, got)

	require.NoError(t, cache.SetAccessToken(ctx, cacheKey, token, time.Hour))
	secondStorage, err := mr.Get(redisKey)
	require.NoError(t, err)
	require.NotEqual(t, firstStorage, secondStorage, "each cache write must use a fresh nonce")
	_, _, err = cipher.DecryptSchedulerProxyPassword(42, "openai", 9, secondStorage)
	require.Error(t, err, "OAuth tokens and scheduler proxy passwords use domain-separated AEAD keys")

	mr.Set(redisKey, secondStorage+"A")
	_, err = cache.GetAccessToken(ctx, cacheKey)
	require.Error(t, err)

	mr.Set(encryptedOAuthTokenKeyPrefix+"openai:account:43", secondStorage)
	_, err = cache.GetAccessToken(ctx, "openai:account:43")
	require.Error(t, err, "cache key is authenticated as AEAD context")
}

func TestGeminiTokenCacheLegacyPlaintextIsDeletedAndTreatedAsMiss(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := newGeminiTokenCache(rdb, testCredentialCipher(t, "9", "primary"))
	cacheKey := "grok:account:77"

	mr.Set(encryptedOAuthTokenKeyPrefix+cacheKey, "legacy-plaintext-token")
	_, err := cache.GetAccessToken(ctx, cacheKey)
	require.ErrorIs(t, err, redis.Nil)
	require.False(t, mr.Exists(encryptedOAuthTokenKeyPrefix+cacheKey))

	mr.Set(legacyOAuthTokenKeyPrefix+cacheKey, "rolling-upgrade-plaintext-token")
	_, err = cache.GetAccessToken(ctx, cacheKey)
	require.ErrorIs(t, err, redis.Nil)
	require.False(t, mr.Exists(legacyOAuthTokenKeyPrefix+cacheKey))
}

func TestPurgeLegacyOAuthTokenCachePreservesEncryptedValuesAndRefreshLocks(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := newGeminiTokenCache(rdb, testCredentialCipher(t, "7", "primary"))

	require.NoError(t, cache.SetAccessToken(ctx, "openai:account:5", "encrypted-token", time.Hour))
	mr.Set(legacyOAuthTokenKeyPrefix+"openai:account:6", "legacy-plaintext-token")
	mr.Set(oauthRefreshLockKeyPrefix+"openai:account:6", "lock-owner")

	require.NoError(t, purgeLegacyOAuthTokenCache(ctx, rdb))
	require.True(t, mr.Exists(encryptedOAuthTokenKeyPrefix+"openai:account:5"))
	require.False(t, mr.Exists(legacyOAuthTokenKeyPrefix+"openai:account:6"))
	require.True(t, mr.Exists(oauthRefreshLockKeyPrefix+"openai:account:6"))
}

func TestGeminiTokenCache_DeleteAccessToken_RedisError(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         "127.0.0.1:1",
		DialTimeout:  50 * time.Millisecond,
		ReadTimeout:  50 * time.Millisecond,
		WriteTimeout: 50 * time.Millisecond,
	})
	t.Cleanup(func() {
		_ = rdb.Close()
	})

	cache := NewGeminiTokenCache(rdb)
	err := cache.DeleteAccessToken(context.Background(), "broken")
	require.Error(t, err)
}
