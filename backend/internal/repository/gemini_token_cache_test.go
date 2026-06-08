//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

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
