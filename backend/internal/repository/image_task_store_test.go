package repository

import (
	"context"
	"encoding/json"
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

func TestImageTaskStoreListByUserIsOrderedAndIsolated(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewImageTaskStore(rdb)
	ctx := context.Background()
	for _, task := range []*service.ImageTaskRecord{
		{ID: "user-7-old", UserID: 7, CreatedAt: 100, Status: service.ImageTaskStatusCompleted},
		{ID: "user-8", UserID: 8, CreatedAt: 300, Status: service.ImageTaskStatusCompleted},
		{ID: "user-7-new", UserID: 7, CreatedAt: 200, Status: service.ImageTaskStatusFailed},
	} {
		require.NoError(t, store.Save(ctx, task, time.Hour))
	}

	tasks, err := store.ListByUser(ctx, 7, 24)
	require.NoError(t, err)
	require.Len(t, tasks, 2)
	require.Equal(t, "user-7-new", tasks[0].ID)
	require.Equal(t, "user-7-old", tasks[1].ID)
	for _, task := range tasks {
		require.Equal(t, int64(7), task.UserID)
	}
}

func TestImageTaskStoreListByUserBackfillsExistingTasksAndRejectsForeignIndexEntries(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewImageTaskStore(rdb)
	ctx := context.Background()

	for _, task := range []*service.ImageTaskRecord{
		{ID: "legacy-own", UserID: 7, CreatedAt: 200, Status: service.ImageTaskStatusCompleted},
		{ID: "legacy-foreign", UserID: 8, CreatedAt: 300, Status: service.ImageTaskStatusCompleted},
	} {
		data, err := json.Marshal(task)
		require.NoError(t, err)
		require.NoError(t, rdb.Set(ctx, imageTaskKey(task.ID), data, time.Hour).Err())
	}

	tasks, err := store.ListByUser(ctx, 7, 24)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.Equal(t, "legacy-own", tasks[0].ID)

	require.NoError(t, rdb.ZAdd(ctx, imageTaskUserIndexKey(7), redis.Z{Score: 400, Member: "legacy-foreign"}).Err())
	require.NoError(t, rdb.Set(ctx, imageTaskKey("corrupt"), "not-json", time.Hour).Err())
	require.NoError(t, rdb.ZAdd(ctx, imageTaskUserIndexKey(7), redis.Z{Score: 500, Member: "corrupt"}).Err())
	tasks, err = store.ListByUser(ctx, 7, 24)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.Equal(t, "legacy-own", tasks[0].ID)
	_, err = rdb.ZScore(ctx, imageTaskUserIndexKey(7), "legacy-foreign").Result()
	require.ErrorIs(t, err, redis.Nil)
	_, err = rdb.ZScore(ctx, imageTaskUserIndexKey(7), "corrupt").Result()
	require.ErrorIs(t, err, redis.Nil)
}
