package repository

import (
	"context"
	"encoding/json"
	"io"
	"strconv"
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

func TestImageTaskStoreAdminPaginationRetainsFailedCleanupInventory(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewImageTaskStore(rdb)
	ctx := context.Background()

	for index, task := range []*service.ImageTaskRecord{
		{ID: "old", UserID: 1, CreatedAt: 100, Status: service.ImageTaskStatusCompleted, StorageKeys: []string{"old-0"}, StorageSizes: []int64{100}},
		{ID: "middle", UserID: 2, CreatedAt: 200, Status: service.ImageTaskStatusCompleted, StorageKeys: []string{"middle-0", "middle-1"}, StorageSizes: []int64{200, 300}},
		{ID: "new", UserID: 3, CreatedAt: 300, Status: service.ImageTaskStatusCompleted, StorageKeys: []string{"new-0"}, StorageSizes: []int64{400}},
	} {
		task.ExpiresAt = time.Now().Add(time.Hour).Unix()
		require.NoError(t, store.Save(ctx, task, time.Hour))
		require.NoError(t, store.ScheduleCleanup(ctx, service.ImageTaskCleanup{
			TaskID: task.ID, Keys: task.StorageKeys, Sizes: task.StorageSizes,
			ExpiresAt: task.ExpiresAt, Record: task,
		}))
		cleanupCount, err := rdb.ZCard(ctx, imageTaskCleanupSchedule).Result()
		require.NoError(t, err)
		require.Equal(t, index+1, int(cleanupCount))
	}

	page, total, err := store.ListForAdmin(ctx, 1, 1)
	require.NoError(t, err)
	require.Equal(t, 3, total)
	require.Len(t, page, 1)
	require.Equal(t, "middle", page[0].ID)
	images, bytes, err := store.AdminStorageStats(ctx)
	require.NoError(t, err)
	require.Equal(t, 4, images)
	require.Equal(t, int64(1000), bytes)

	// Active task keys expire, but failed object cleanup remains visible and
	// manually manageable until deletion actually succeeds.
	mr.FastForward(2 * time.Hour)
	page, total, err = store.ListForAdmin(ctx, 0, 10)
	require.NoError(t, err)
	require.Equal(t, 3, total)
	require.Len(t, page, 3)
	cleanup, err := store.GetCleanup(ctx, "middle")
	require.NoError(t, err)
	require.Equal(t, []string{"middle-0", "middle-1"}, cleanup.Keys)

	require.NoError(t, store.DeleteCleanup(ctx, "middle"))
	page, total, err = store.ListForAdmin(ctx, 0, 10)
	require.NoError(t, err)
	require.Equal(t, 2, total)
	require.Len(t, page, 2)
	images, bytes, err = store.AdminStorageStats(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, images)
	require.Equal(t, int64(500), bytes)
}

func TestImageTaskStoreMutationLockUsesOwnerToken(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewImageTaskStore(rdb)
	ctx := context.Background()

	locked, err := store.TryLock(ctx, "task", "owner-a", time.Minute)
	require.NoError(t, err)
	require.True(t, locked)
	locked, err = store.TryLock(ctx, "task", "owner-b", time.Minute)
	require.NoError(t, err)
	require.False(t, locked)
	require.NoError(t, store.Unlock(ctx, "task", "owner-b"))
	locked, err = store.TryLock(ctx, "task", "owner-b", time.Minute)
	require.NoError(t, err)
	require.False(t, locked)
	require.NoError(t, store.Unlock(ctx, "task", "owner-a"))
	locked, err = store.TryLock(ctx, "task", "owner-b", time.Minute)
	require.NoError(t, err)
	require.True(t, locked)
}

func TestImageTaskStoreExpiredLockHolderCannotOverwriteNewOwner(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewImageTaskStore(rdb)
	ctx := context.Background()
	taskID := "imgtask_fenced"

	locked, err := store.TryLock(ctx, taskID, "owner-a", time.Second)
	require.NoError(t, err)
	require.True(t, locked)
	ownerACtx := service.WithImageTaskMutationGuard(ctx, taskID, "owner-a")

	mr.FastForward(2 * time.Second)
	locked, err = store.TryLock(ctx, taskID, "owner-b", time.Minute)
	require.NoError(t, err)
	require.True(t, locked)
	ownerBCtx := service.WithImageTaskMutationGuard(ctx, taskID, "owner-b")
	require.NoError(t, store.Save(ownerBCtx, &service.ImageTaskRecord{
		ID: taskID, Status: service.ImageTaskStatusCompleted, StorageKeys: []string{"new-owner"},
	}, time.Hour))

	err = store.Save(ownerACtx, &service.ImageTaskRecord{
		ID: taskID, Status: service.ImageTaskStatusCompleted, StorageKeys: []string{"stale-owner"},
	}, time.Hour)
	require.ErrorIs(t, err, service.ErrImageTaskBusy)
	got, err := store.Get(ctx, taskID)
	require.NoError(t, err)
	require.Equal(t, []string{"new-owner"}, got.StorageKeys)
}

func TestImageTaskStoreAdminBackfillDoesNotRaceCleanupDeletion(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewImageTaskStore(rdb)
	ctx := context.Background()
	taskID := "imgtask_cleanup_race"
	expiresAt := time.Now().Add(time.Hour).Unix()
	legacy := `{"task_id":"` + taskID + `","keys":["race-0"],"expires_at":` + strconv.FormatInt(expiresAt, 10) + `}`
	require.NoError(t, rdb.Set(ctx, imageTaskCleanupKey(taskID), legacy, 0).Err())
	require.NoError(t, rdb.ZAdd(ctx, imageTaskCleanupSchedule, redis.Z{Score: float64(expiresAt), Member: taskID}).Err())

	locked, err := store.TryLock(ctx, taskID, "delete-owner", time.Minute)
	require.NoError(t, err)
	require.True(t, locked)
	deleteCtx := service.WithImageTaskMutationGuard(ctx, taskID, "delete-owner")
	_, _, err = store.ListForAdmin(ctx, 0, 10)
	require.ErrorIs(t, err, service.ErrImageTaskBusy)
	require.NoError(t, store.DeleteCleanup(deleteCtx, taskID))
	require.NoError(t, store.Unlock(ctx, taskID, "delete-owner"))

	records, total, err := store.ListForAdmin(ctx, 0, 10)
	require.NoError(t, err)
	require.Empty(t, records)
	require.Zero(t, total)
	images, bytes, err := store.AdminStorageStats(ctx)
	require.NoError(t, err)
	require.Zero(t, images)
	require.Zero(t, bytes)
	require.False(t, mr.Exists(imageTaskCleanupKey(taskID)))
	require.False(t, mr.Exists(imageTaskAdminRecordKey(taskID)))
}

type legacyCleanupImageStorage struct {
	sizes   map[string]int64
	deleted []string
}

func (s *legacyCleanupImageStorage) Save(context.Context, string, string, []byte) error { return nil }
func (s *legacyCleanupImageStorage) Load(context.Context, string, int64) ([]byte, string, error) {
	return nil, "", io.EOF
}
func (s *legacyCleanupImageStorage) Size(_ context.Context, key string) (int64, error) {
	return s.sizes[key], nil
}
func (s *legacyCleanupImageStorage) Delete(_ context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	return nil
}

func TestImageTaskStoreBackfillsLegacyCleanupForAdminManagement(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewImageTaskStore(rdb)
	ctx := context.Background()
	taskID := "imgtask_legacy_cleanup"
	expiresAt := time.Now().Add(time.Hour).Unix()
	legacy := `{"task_id":"` + taskID + `","keys":["legacy-0","legacy-1"],"expires_at":` + strconv.FormatInt(expiresAt, 10) + `}`
	require.NoError(t, rdb.Set(ctx, imageTaskCleanupKey(taskID), legacy, 0).Err())
	require.NoError(t, rdb.ZAdd(ctx, imageTaskCleanupSchedule, redis.Z{Score: float64(expiresAt), Member: taskID}).Err())

	storage := &legacyCleanupImageStorage{sizes: map[string]int64{"legacy-0": 100, "legacy-1": 200}}
	svc := service.NewImageTaskServiceWithUploader(
		store, service.NewImageResultUploader(storage, "", 0, nil), time.Hour, time.Minute,
	)
	page, err := svc.ListForAdmin(ctx, 1, 24)
	require.NoError(t, err)
	require.Equal(t, 1, page.Total)
	require.Equal(t, 2, page.TotalImages)
	require.Equal(t, int64(300), page.StorageBytes)
	require.Len(t, page.Tasks, 1)
	require.Equal(t, taskID, page.Tasks[0].Task.ID)
	require.Equal(t, 2, page.Tasks[0].Task.ImageCount)
	cleanup, err := store.GetCleanup(ctx, taskID)
	require.NoError(t, err)
	require.NotNil(t, cleanup.Record)
	require.Equal(t, []int64{100, 200}, cleanup.Sizes)

	require.NoError(t, svc.DeleteForAdmin(ctx, taskID))
	require.ElementsMatch(t, []string{"legacy-0", "legacy-1"}, storage.deleted)
	page, err = svc.ListForAdmin(ctx, 1, 24)
	require.NoError(t, err)
	require.Zero(t, page.Total)
	require.Zero(t, page.TotalImages)
	require.Zero(t, page.StorageBytes)
}
