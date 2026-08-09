//go:build unit

package repository

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func newTotpSecurityCache(t *testing.T) (*TotpCache, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	cache, ok := NewTotpCache(client).(*TotpCache)
	require.True(t, ok)
	return cache, server
}

func TestTotpCacheConsumeLoginSessionExactlyOnceConcurrently(t *testing.T) {
	cache, _ := newTotpSecurityCache(t)
	ctx := context.Background()
	const callers = 32
	const token = "fake-local-temp-token"

	require.NoError(t, cache.SetLoginSession(ctx, token, &service.TotpLoginSession{
		UserID:      42,
		Email:       "local@example.test",
		TokenExpiry: time.Now().Add(time.Minute),
	}, time.Minute))

	start := make(chan struct{})
	var winners atomic.Int32
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			session, err := cache.ConsumeLoginSession(ctx, token)
			if err != nil {
				errs <- err
				return
			}
			if session != nil {
				if session.UserID != 42 {
					errs <- fmt.Errorf("unexpected consumed user id: %d", session.UserID)
					return
				}
				winners.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	require.Equal(t, int32(1), winners.Load())
	remaining, err := cache.GetLoginSession(ctx, token)
	require.NoError(t, err)
	require.Nil(t, remaining)
}

func TestTotpCacheConsumeLoginSessionRedisFailureFailsClosed(t *testing.T) {
	cache, server := newTotpSecurityCache(t)
	ctx := context.Background()
	require.NoError(t, cache.SetLoginSession(ctx, "fake-local-temp-token", &service.TotpLoginSession{UserID: 7}, time.Minute))

	server.Close()
	session, err := cache.ConsumeLoginSession(ctx, "fake-local-temp-token")
	require.Error(t, err)
	require.Nil(t, session)
}

func TestTotpCacheStepUpGrantIsCredentialGenerationBound(t *testing.T) {
	cache, _ := newTotpSecurityCache(t)
	ctx := context.Background()

	require.NoError(t, cache.SetStepUpGrant(ctx, 11, "session-a", "generation-a", time.Minute))
	granted, err := cache.HasStepUpGrant(ctx, 11, "session-a", "generation-a")
	require.NoError(t, err)
	require.True(t, granted)

	granted, err = cache.HasStepUpGrant(ctx, 11, "session-a", "generation-b")
	require.NoError(t, err)
	require.False(t, granted)
}
