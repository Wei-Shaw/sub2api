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

func TestImageTaskStoreRoundTripAndTTL(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewImageTaskStore(rdb)
	task := &service.ImageTaskRecord{
		ID:        "imgtask_123",
		UserID:    7,
		APIKeyID:  9,
		Status:    service.ImageTaskStatusProcessing,
		CreatedAt: 100,
		ExpiresAt: 200,
	}

	require.NoError(t, store.Save(context.Background(), task, 24*time.Hour))
	got, err := store.Get(context.Background(), task.ID)
	require.NoError(t, err)
	require.Equal(t, task, got)
	require.Equal(t, 24*time.Hour, mr.TTL(imageTaskKey(task.ID)))
}

func TestImageTaskStoreMissing(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewImageTaskStore(rdb)

	_, err := store.Get(context.Background(), "imgtask_missing")
	require.ErrorIs(t, err, service.ErrImageTaskNotFound)
}

func TestImageTaskStoreCleanupScheduleAndDelete(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewImageTaskStore(rdb)
	now := time.Now().Truncate(time.Second)
	cleanup := service.ImageTaskCleanup{
		TaskID:    "imgtask_cleanup",
		Keys:      []string{"images/imgtask_cleanup-0.png"},
		ExpiresAt: now.Add(time.Hour).Unix(),
	}

	require.NoError(t, store.ScheduleCleanup(context.Background(), cleanup))
	due, err := store.ListDueCleanup(context.Background(), now, 10)
	require.NoError(t, err)
	require.Empty(t, due)
	due, err = store.ListDueCleanup(context.Background(), now.Add(time.Hour), 10)
	require.NoError(t, err)
	require.Equal(t, []service.ImageTaskCleanup{cleanup}, due)

	require.NoError(t, store.DeleteCleanup(context.Background(), cleanup.TaskID))
	due, err = store.ListDueCleanup(context.Background(), now.Add(2*time.Hour), 10)
	require.NoError(t, err)
	require.Empty(t, due)
}

func TestImageTaskStoreDeleteTask(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewImageTaskStore(rdb)
	task := &service.ImageTaskRecord{ID: "imgtask_delete", UserID: 7, Status: service.ImageTaskStatusCompleted}
	require.NoError(t, store.Save(context.Background(), task, time.Hour))
	require.NoError(t, store.Delete(context.Background(), task.ID))
	_, err := store.Get(context.Background(), task.ID)
	require.ErrorIs(t, err, service.ErrImageTaskNotFound)
}
