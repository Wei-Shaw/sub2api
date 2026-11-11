//go:build unit

package repository

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newRefreshTokenCacheForTest(t *testing.T) *refreshTokenCache {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return &refreshTokenCache{rdb: rdb}
}

type consumeResult struct {
	data *service.RefreshTokenData
	err  error
}

// 同一个 refresh token 被并发消费时，只能有一个调用成功。
func TestRefreshTokenCacheConsumeIsAtomic(t *testing.T) {
	cache := newRefreshTokenCacheForTest(t)
	ctx := context.Background()

	require.NoError(t, cache.StoreRefreshToken(ctx, "hash-1", &service.RefreshTokenData{
		UserID:   7,
		FamilyID: "family-1",
	}, time.Minute))

	const workers = 16
	results := make(chan consumeResult, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			data, err := cache.ConsumeRefreshToken(ctx, "hash-1")
			results <- consumeResult{data: data, err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	successes, notFound := 0, 0
	for result := range results {
		switch {
		case result.err == nil:
			successes++
			require.Equal(t, int64(7), result.data.UserID)
		case errors.Is(result.err, service.ErrRefreshTokenNotFound):
			notFound++
		default:
			t.Fatalf("unexpected consume error: %v", result.err)
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, workers-1, notFound)

	_, err := cache.GetRefreshToken(ctx, "hash-1")
	require.ErrorIs(t, err, service.ErrRefreshTokenNotFound)
}
