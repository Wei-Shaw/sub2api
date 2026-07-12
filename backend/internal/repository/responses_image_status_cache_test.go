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

func newResponsesImageStatusTestCache(t *testing.T) (*responsesImageStatusCache, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = rdb.Close()
	})
	store, ok := NewResponsesImageStatusStore(rdb).(*responsesImageStatusCache)
	require.True(t, ok, "NewResponsesImageStatusStore should return *responsesImageStatusCache")
	return store, mr
}

func TestResponsesImageStatusKey(t *testing.T) {
	require.Equal(t, "gen_img:req-1", ResponsesImageStatusKey(" req-1 "))
}

func TestResponsesImageStatusCacheSetGetTTLAndOverwrite(t *testing.T) {
	cache, mr := newResponsesImageStatusTestCache(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)

	status := &service.ResponsesImageStatus{
		RequestID: "img-1",
		Status:    service.ResponsesImageStatusRunning,
		Progress:  25,
		URLs:      []string{"https://upstream.example/image.png"},
		CreatedAt: now,
		UpdatedAt: now,
	}
	require.NoError(t, cache.SetResponsesImageStatus(ctx, status, service.ResponsesImageStatusTTL))

	ttl := mr.TTL(ResponsesImageStatusKey("img-1"))
	require.Greater(t, ttl, service.ResponsesImageStatusTTL-time.Second)
	require.LessOrEqual(t, ttl, service.ResponsesImageStatusTTL)

	got, err := cache.GetResponsesImageStatus(ctx, "img-1")
	require.NoError(t, err)
	require.Equal(t, status.RequestID, got.RequestID)
	require.Equal(t, status.Status, got.Status)
	require.Equal(t, status.Progress, got.Progress)
	require.Equal(t, status.URLs, got.URLs)

	updated := &service.ResponsesImageStatus{
		RequestID: "img-1",
		Status:    service.ResponsesImageStatusSucceeded,
		Progress:  100,
		COSURLs:   []string{"https://cos.example/image.png"},
		CreatedAt: now,
		UpdatedAt: now.Add(time.Minute),
	}
	require.NoError(t, cache.SetResponsesImageStatus(ctx, updated, service.ResponsesImageStatusTTL))

	got, err = cache.GetResponsesImageStatus(ctx, "img-1")
	require.NoError(t, err)
	require.Equal(t, service.ResponsesImageStatusSucceeded, got.Status)
	require.Equal(t, []string{"https://cos.example/image.png"}, got.COSURLs)
}

func TestResponsesImageStatusCacheBatchGet(t *testing.T) {
	cache, _ := newResponsesImageStatusTestCache(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)

	require.NoError(t, cache.SetResponsesImageStatus(ctx, &service.ResponsesImageStatus{
		RequestID: "img-1",
		Status:    service.ResponsesImageStatusSucceeded,
		Progress:  100,
		CreatedAt: now,
		UpdatedAt: now,
	}, service.ResponsesImageStatusTTL))
	require.NoError(t, cache.SetResponsesImageStatus(ctx, &service.ResponsesImageStatus{
		RequestID: "img-2",
		Status:    service.ResponsesImageStatusRunning,
		Progress:  25,
		CreatedAt: now,
		UpdatedAt: now,
	}, service.ResponsesImageStatusTTL))

	got, err := cache.GetResponsesImageStatuses(ctx, []string{"img-1", "missing", "img-2", "img-1"})
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, service.ResponsesImageStatusSucceeded, got["img-1"].Status)
	require.Equal(t, service.ResponsesImageStatusRunning, got["img-2"].Status)
	require.Nil(t, got["missing"])
}

func TestResponsesImageStatusCacheMissingAndInvalidJSON(t *testing.T) {
	cache, mr := newResponsesImageStatusTestCache(t)
	ctx := context.Background()

	_, err := cache.GetResponsesImageStatus(ctx, "missing")
	require.ErrorIs(t, err, service.ErrResponsesImageStatusNotFound)

	require.NoError(t, mr.Set(ResponsesImageStatusKey("bad"), "{"))
	_, err = cache.GetResponsesImageStatus(ctx, "bad")
	require.Error(t, err)
	require.False(t, errors.Is(err, service.ErrResponsesImageStatusNotFound))
}
