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

func TestOpenAIStickySuccessCacheIsolationAndCAS(t *testing.T) {
	r := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: r.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache, ok := NewGatewayCache(client).(service.OpenAIStickySuccessCache)
	require.True(t, ok)
	ctx := context.Background()
	empty := service.OpenAIStickySuccessBinding{}
	one := service.OpenAIStickySuccessBinding{AccountID: 1, Revision: "first"}
	two := service.OpenAIStickySuccessBinding{AccountID: 1, Revision: "new-success-same-account"}
	changed, err := cache.CompareAndSwapOpenAIStickySuccess(ctx, 7, "session", "model", empty, one, time.Minute)
	require.NoError(t, err)
	require.True(t, changed)
	changed, err = cache.CompareAndSwapOpenAIStickySuccess(ctx, 7, "session", "model", one, two, time.Minute)
	require.NoError(t, err)
	require.True(t, changed)
	changed, err = cache.CompareAndSwapOpenAIStickySuccess(ctx, 7, "session", "model", one, service.OpenAIStickySuccessBinding{AccountID: 2, Revision: "stale"}, time.Minute)
	require.NoError(t, err)
	require.False(t, changed)
	got, err := cache.GetOpenAIStickySuccess(ctx, 7, "session", "model")
	require.NoError(t, err)
	require.Equal(t, two, got)
	for _, scope := range []struct {
		group          int64
		session, model string
	}{{8, "session", "model"}, {7, "other", "model"}, {7, "session", "other"}} {
		_, err := cache.GetOpenAIStickySuccess(ctx, scope.group, scope.session, scope.model)
		require.ErrorIs(t, err, service.ErrStickySessionNotFound)
	}
	r.FastForward(time.Minute)
	_, err = cache.GetOpenAIStickySuccess(ctx, 7, "session", "model")
	require.ErrorIs(t, err, service.ErrStickySessionNotFound)
}
