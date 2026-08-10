package repository

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const imageTaskKeyPrefix = "image_task:"

const (
	imageTaskCleanupKeyPrefix   = "image_task_cleanup:"
	imageTaskCleanupSchedule    = "image_task_cleanup_schedule"
	imageTaskUserIndexPrefix    = "image_task_user:"
	imageTaskUserMarkerPrefix   = "image_task_user_indexed:"
	imageTaskUserBackfillTTL    = 8 * 24 * time.Hour
	imageTaskAdminRecordPrefix  = "image_task_admin:"
	imageTaskAdminIndex         = "image_task_admin_index"
	imageTaskAdminExpiry        = "image_task_admin_expiry"
	imageTaskAdminMarker        = "image_task_admin_indexed"
	imageTaskAdminStats         = "image_task_admin_storage_stats"
	imageTaskAdminObjectStats   = "image_task_admin_object_stats"
	imageTaskMutationLockPrefix = "image_task_lock:"
)

type imageTaskStore struct {
	rdb *redis.Client
}

func (s *imageTaskStore) ListForAdmin(ctx context.Context, offset int64, limit int) ([]*service.ImageTaskRecord, int, error) {
	if s == nil || s.rdb == nil {
		return []*service.ImageTaskRecord{}, 0, nil
	}
	if limit <= 0 {
		limit = 24
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	if err := s.ensureImageTaskAdminIndex(ctx); err != nil {
		return nil, 0, err
	}
	if err := s.pruneExpiredImageTaskAdminRecords(ctx, time.Now()); err != nil {
		return nil, 0, err
	}

	records := make([]*service.ImageTaskRecord, 0, limit)
	cursor := offset
	for len(records) < limit {
		ids, err := s.rdb.ZRevRange(ctx, imageTaskAdminIndex, cursor, cursor+int64(limit*2)-1).Result()
		if err != nil {
			return nil, 0, err
		}
		if len(ids) == 0 {
			break
		}
		keys := make([]string, len(ids))
		for i, id := range ids {
			keys[i] = imageTaskAdminRecordKey(id)
		}
		values, err := s.rdb.MGet(ctx, keys...).Result()
		if err != nil {
			return nil, 0, err
		}
		stale := make([]any, 0)
		for i, value := range values {
			text, ok := value.(string)
			if !ok {
				stale = append(stale, ids[i])
				continue
			}
			var task service.ImageTaskRecord
			if json.Unmarshal([]byte(text), &task) != nil || strings.TrimSpace(task.ID) == "" {
				stale = append(stale, ids[i])
				continue
			}
			records = append(records, &task)
			if len(records) == limit {
				break
			}
		}
		if len(stale) > 0 {
			_ = s.rdb.ZRem(ctx, imageTaskAdminIndex, stale...).Err()
		}
		cursor += int64(len(ids))
		if len(ids) < limit*2 {
			break
		}
	}
	total, err := s.rdb.ZCard(ctx, imageTaskAdminIndex).Result()
	return records, int(total), err
}

func (s *imageTaskStore) AdminStorageStats(ctx context.Context) (int, int64, error) {
	values, err := s.rdb.HMGet(ctx, imageTaskAdminStats, "images", "bytes").Result()
	if err != nil {
		return 0, 0, err
	}
	images := parseRedisInt64(values[0])
	bytes := parseRedisInt64(values[1])
	if images < 0 {
		images = 0
	}
	if bytes < 0 {
		bytes = 0
	}
	return int(images), bytes, nil
}

func NewImageTaskStore(rdb *redis.Client) service.ImageTaskStore {
	return &imageTaskStore{rdb: rdb}
}

func (s *imageTaskStore) Save(ctx context.Context, task *service.ImageTaskRecord, ttl time.Duration) error {
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}
	cleanupExists, err := s.rdb.Exists(ctx, imageTaskCleanupKey(task.ID)).Result()
	if err != nil {
		return err
	}
	err = s.guardedImageTaskTx(ctx, task.ID, func(pipe redis.Pipeliner) error {
		pipe.Set(ctx, imageTaskKey(task.ID), data, ttl)
		pipe.ZAdd(ctx, imageTaskAdminIndex, redis.Z{Score: imageTaskAdminScore(task), Member: task.ID})
		if cleanupExists > 0 {
			pipe.Set(ctx, imageTaskAdminRecordKey(task.ID), data, 0)
			pipe.ZRem(ctx, imageTaskAdminExpiry, task.ID)
		} else {
			pipe.Set(ctx, imageTaskAdminRecordKey(task.ID), data, ttl)
			pipe.ZAdd(ctx, imageTaskAdminExpiry, redis.Z{Score: float64(time.Now().Add(ttl).Unix()), Member: task.ID})
		}
		if task.UserID > 0 {
			pipe.ZAdd(ctx, imageTaskUserIndexKey(task.UserID), redis.Z{Score: float64(task.CreatedAt), Member: task.ID})
			pipe.Expire(ctx, imageTaskUserIndexKey(task.UserID), imageTaskUserBackfillTTL)
			pipe.Set(ctx, imageTaskUserMarkerKey(task.UserID), "1", imageTaskUserBackfillTTL)
		}
		return nil
	})
	return err
}

func (s *imageTaskStore) Get(ctx context.Context, id string) (*service.ImageTaskRecord, error) {
	data, err := s.rdb.Get(ctx, imageTaskKey(id)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, service.ErrImageTaskNotFound
		}
		return nil, err
	}
	var task service.ImageTaskRecord
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

func (s *imageTaskStore) ListByUser(ctx context.Context, userID int64, limit int) ([]*service.ImageTaskRecord, error) {
	if userID <= 0 {
		return []*service.ImageTaskRecord{}, nil
	}
	if limit <= 0 {
		limit = 24
	}
	if limit > 100 {
		limit = 100
	}

	indexed, err := s.rdb.Exists(ctx, imageTaskUserMarkerKey(userID)).Result()
	if err != nil {
		return nil, err
	}
	if indexed == 0 {
		if err := s.backfillImageTaskUserIndex(ctx, userID); err != nil {
			return nil, err
		}
	}

	scanLimit := limit * 4
	if scanLimit < 100 {
		scanLimit = 100
	}
	ids, err := s.rdb.ZRevRange(ctx, imageTaskUserIndexKey(userID), 0, int64(scanLimit-1)).Result()
	if err != nil || len(ids) == 0 {
		return []*service.ImageTaskRecord{}, err
	}
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = imageTaskKey(id)
	}
	values, err := s.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}

	tasks := make([]*service.ImageTaskRecord, 0, limit)
	stale := make([]any, 0)
	for i, value := range values {
		text, ok := value.(string)
		if !ok {
			stale = append(stale, ids[i])
			continue
		}
		var task service.ImageTaskRecord
		if err := json.Unmarshal([]byte(text), &task); err != nil {
			stale = append(stale, ids[i])
			continue
		}
		if task.UserID != userID {
			stale = append(stale, ids[i])
			continue
		}
		tasks = append(tasks, &task)
		if len(tasks) == limit {
			break
		}
	}
	if len(stale) > 0 {
		_ = s.rdb.ZRem(ctx, imageTaskUserIndexKey(userID), stale...).Err()
	}
	return tasks, nil
}

func (s *imageTaskStore) backfillImageTaskUserIndex(ctx context.Context, userID int64) error {
	entries := make([]redis.Z, 0)
	iterator := s.rdb.Scan(ctx, 0, imageTaskKeyPrefix+"*", 100).Iterator()
	for iterator.Next(ctx) {
		data, err := s.rdb.Get(ctx, iterator.Val()).Bytes()
		if err == redis.Nil {
			continue
		}
		if err != nil {
			return err
		}
		var task service.ImageTaskRecord
		if err := json.Unmarshal(data, &task); err != nil {
			continue
		}
		if task.UserID == userID {
			entries = append(entries, redis.Z{Score: float64(task.CreatedAt), Member: task.ID})
		}
	}
	if err := iterator.Err(); err != nil {
		return err
	}
	_, err := s.rdb.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		if len(entries) > 0 {
			pipe.ZAdd(ctx, imageTaskUserIndexKey(userID), entries...)
			pipe.Expire(ctx, imageTaskUserIndexKey(userID), imageTaskUserBackfillTTL)
		}
		pipe.Set(ctx, imageTaskUserMarkerKey(userID), "1", imageTaskUserBackfillTTL)
		return nil
	})
	return err
}

func (s *imageTaskStore) Delete(ctx context.Context, id string) error {
	cleanupExists, err := s.rdb.Exists(ctx, imageTaskCleanupKey(id)).Result()
	if err != nil {
		return err
	}
	err = s.guardedImageTaskTx(ctx, id, func(pipe redis.Pipeliner) error {
		pipe.Del(ctx, imageTaskKey(id))
		if cleanupExists == 0 {
			pipe.Del(ctx, imageTaskAdminRecordKey(id))
			pipe.ZRem(ctx, imageTaskAdminIndex, id)
			pipe.ZRem(ctx, imageTaskAdminExpiry, id)
		}
		return nil
	})
	return err
}

func (s *imageTaskStore) ScheduleCleanup(ctx context.Context, cleanup service.ImageTaskCleanup) error {
	cleanup.TaskID = strings.TrimSpace(cleanup.TaskID)
	cleanup.Keys = append([]string(nil), cleanup.Keys...)
	cleanup.Sizes = append([]int64(nil), cleanup.Sizes...)
	if len(cleanup.Sizes) == 0 && cleanup.Record != nil {
		cleanup.Sizes = append([]int64(nil), cleanup.Record.StorageSizes...)
	}
	data, err := json.Marshal(cleanup)
	if err != nil {
		return err
	}
	oldImages, oldBytes, err := s.cleanupStats(ctx, cleanup.TaskID)
	if err != nil {
		return err
	}
	tracked, err := s.rdb.HExists(ctx, imageTaskAdminObjectStats, cleanup.TaskID).Result()
	if err != nil {
		return err
	}
	if !tracked {
		oldImages, oldBytes = 0, 0
	}
	newImages, newBytes := len(cleanup.Keys), sumPositiveInt64(cleanup.Sizes)
	var recordData []byte
	if cleanup.Record != nil {
		recordData, err = json.Marshal(cleanup.Record)
		if err != nil {
			return err
		}
	}
	err = s.guardedImageTaskTx(ctx, cleanup.TaskID, func(pipe redis.Pipeliner) error {
		// Cleanup metadata remains until object deletion succeeds. This keeps failed
		// deletions visible and manually recoverable instead of creating hidden orphans.
		pipe.Set(ctx, imageTaskCleanupKey(cleanup.TaskID), data, 0)
		pipe.ZAdd(ctx, imageTaskCleanupSchedule, redis.Z{Score: float64(cleanup.ExpiresAt), Member: cleanup.TaskID})
		pipe.HIncrBy(ctx, imageTaskAdminStats, "images", int64(newImages-oldImages))
		pipe.HIncrBy(ctx, imageTaskAdminStats, "bytes", newBytes-oldBytes)
		pipe.HSet(ctx, imageTaskAdminObjectStats, cleanup.TaskID, encodeCleanupStats(newImages, newBytes))
		pipe.ZRem(ctx, imageTaskAdminExpiry, cleanup.TaskID)
		if cleanup.Record != nil {
			pipe.Set(ctx, imageTaskAdminRecordKey(cleanup.TaskID), recordData, 0)
			pipe.ZAdd(ctx, imageTaskAdminIndex, redis.Z{Score: imageTaskAdminScore(cleanup.Record), Member: cleanup.TaskID})
		}
		return nil
	})
	return err
}

func (s *imageTaskStore) GetCleanup(ctx context.Context, id string) (*service.ImageTaskCleanup, error) {
	data, err := s.rdb.Get(ctx, imageTaskCleanupKey(id)).Bytes()
	if err == redis.Nil {
		return nil, service.ErrImageTaskNotFound
	}
	if err != nil {
		return nil, err
	}
	var cleanup service.ImageTaskCleanup
	if err := json.Unmarshal(data, &cleanup); err != nil {
		return nil, err
	}
	return &cleanup, nil
}

func (s *imageTaskStore) ListDueCleanup(ctx context.Context, now time.Time, limit int) ([]service.ImageTaskCleanup, error) {
	if limit <= 0 {
		limit = 100
	}
	ids, err := s.rdb.ZRangeByScore(ctx, imageTaskCleanupSchedule, &redis.ZRangeBy{
		Min: "-inf", Max: strconv.FormatInt(now.Unix(), 10), Offset: 0, Count: int64(limit),
	}).Result()
	if err != nil {
		return nil, err
	}
	out := make([]service.ImageTaskCleanup, 0, len(ids))
	for _, id := range ids {
		data, getErr := s.rdb.Get(ctx, imageTaskCleanupKey(id)).Bytes()
		if getErr == redis.Nil {
			_ = s.rdb.ZRem(ctx, imageTaskCleanupSchedule, id).Err()
			continue
		}
		if getErr != nil {
			return nil, getErr
		}
		var cleanup service.ImageTaskCleanup
		if err := json.Unmarshal(data, &cleanup); err != nil {
			return nil, err
		}
		out = append(out, cleanup)
	}
	return out, nil
}

func (s *imageTaskStore) DeleteCleanup(ctx context.Context, id string) error {
	images, bytes, err := s.cleanupStats(ctx, id)
	if err != nil {
		return err
	}
	tracked, err := s.rdb.HExists(ctx, imageTaskAdminObjectStats, id).Result()
	if err != nil {
		return err
	}
	if !tracked {
		images, bytes = 0, 0
	}
	err = s.guardedImageTaskTx(ctx, id, func(pipe redis.Pipeliner) error {
		pipe.Del(ctx, imageTaskCleanupKey(id))
		pipe.ZRem(ctx, imageTaskCleanupSchedule, id)
		pipe.Del(ctx, imageTaskAdminRecordKey(id))
		pipe.ZRem(ctx, imageTaskAdminIndex, id)
		pipe.ZRem(ctx, imageTaskAdminExpiry, id)
		pipe.HIncrBy(ctx, imageTaskAdminStats, "images", -int64(images))
		pipe.HIncrBy(ctx, imageTaskAdminStats, "bytes", -bytes)
		pipe.HDel(ctx, imageTaskAdminObjectStats, id)
		return nil
	})
	return err
}

func (s *imageTaskStore) TryLock(ctx context.Context, id, token string, ttl time.Duration) (bool, error) {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return s.rdb.SetNX(ctx, imageTaskMutationLockKey(id), token, ttl).Result()
}

var imageTaskUnlockScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)

var imageTaskPruneAdminScript = redis.NewScript(`
redis.call("ZREM", KEYS[1], ARGV[1])
if redis.call("EXISTS", KEYS[2]) == 0 then
  redis.call("DEL", KEYS[3])
  redis.call("ZREM", KEYS[4], ARGV[1])
end
return 1
`)

func (s *imageTaskStore) Unlock(ctx context.Context, id, token string) error {
	return imageTaskUnlockScript.Run(ctx, s.rdb, []string{imageTaskMutationLockKey(id)}, token).Err()
}

func (s *imageTaskStore) guardedImageTaskTx(ctx context.Context, id string, fn func(redis.Pipeliner) error) error {
	guardID, token, guarded := service.ImageTaskMutationGuardFromContext(ctx)
	if !guarded {
		_, err := s.rdb.TxPipelined(ctx, fn)
		return err
	}
	id = strings.TrimSpace(id)
	if guardID != id {
		return service.ErrImageTaskBusy
	}
	lockKey := imageTaskMutationLockKey(id)
	err := s.rdb.Watch(ctx, func(tx *redis.Tx) error {
		current, err := tx.Get(ctx, lockKey).Result()
		if err == redis.Nil || current != token {
			return service.ErrImageTaskBusy
		}
		if err != nil {
			return err
		}
		_, err = tx.TxPipelined(ctx, fn)
		return err
	}, lockKey)
	if errors.Is(err, redis.TxFailedErr) {
		return service.ErrImageTaskBusy
	}
	return err
}

func (s *imageTaskStore) cleanupStats(ctx context.Context, id string) (int, int64, error) {
	cleanup, err := s.GetCleanup(ctx, id)
	if err != nil {
		if err == service.ErrImageTaskNotFound {
			return 0, 0, nil
		}
		return 0, 0, err
	}
	sizes := cleanup.Sizes
	if len(sizes) == 0 && cleanup.Record != nil {
		sizes = cleanup.Record.StorageSizes
	}
	return len(cleanup.Keys), sumPositiveInt64(sizes), nil
}

func (s *imageTaskStore) ensureImageTaskAdminIndex(ctx context.Context) error {
	indexed, err := s.rdb.Exists(ctx, imageTaskAdminMarker).Result()
	if err != nil || indexed > 0 {
		return err
	}

	iterator := s.rdb.Scan(ctx, 0, imageTaskKeyPrefix+"imgtask_*", 200).Iterator()
	for iterator.Next(ctx) {
		data, getErr := s.rdb.Get(ctx, iterator.Val()).Bytes()
		if getErr == redis.Nil {
			continue
		}
		if getErr != nil {
			return getErr
		}
		var task service.ImageTaskRecord
		if json.Unmarshal(data, &task) != nil || strings.TrimSpace(task.ID) == "" {
			continue
		}
		if err := s.backfillImageTaskAdminRecord(ctx, task.ID); err != nil {
			return err
		}
	}
	if err := iterator.Err(); err != nil {
		return err
	}

	cleanupIDs, err := s.rdb.ZRange(ctx, imageTaskCleanupSchedule, 0, -1).Result()
	if err != nil {
		return err
	}
	for _, id := range cleanupIDs {
		if err := s.backfillImageTaskCleanup(ctx, id); err != nil {
			return err
		}
	}
	_, err = s.rdb.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Set(ctx, imageTaskAdminMarker, "1", 0)
		return nil
	})
	return err
}

func (s *imageTaskStore) withImageTaskBackfillLock(ctx context.Context, id string, fn func(context.Context) error) error {
	token := uuid.NewString()
	locked, err := s.TryLock(ctx, id, token, 30*time.Second)
	if err != nil {
		return err
	}
	if !locked {
		return service.ErrImageTaskBusy
	}
	defer func() { _ = s.Unlock(context.Background(), id, token) }()
	return fn(service.WithImageTaskMutationGuard(ctx, id, token))
}

func (s *imageTaskStore) backfillImageTaskAdminRecord(ctx context.Context, id string) error {
	return s.withImageTaskBackfillLock(ctx, id, func(guardedCtx context.Context) error {
		data, err := s.rdb.Get(guardedCtx, imageTaskKey(id)).Bytes()
		if err == redis.Nil {
			return nil
		}
		if err != nil {
			return err
		}
		var task service.ImageTaskRecord
		if json.Unmarshal(data, &task) != nil || strings.TrimSpace(task.ID) == "" {
			return nil
		}
		ttl, err := s.rdb.PTTL(guardedCtx, imageTaskKey(id)).Result()
		if err != nil {
			return err
		}
		cleanupExists, err := s.rdb.Exists(guardedCtx, imageTaskCleanupKey(id)).Result()
		if err != nil {
			return err
		}
		if cleanupExists == 0 && ttl <= 0 {
			return nil
		}
		return s.guardedImageTaskTx(guardedCtx, id, func(pipe redis.Pipeliner) error {
			pipe.ZAdd(guardedCtx, imageTaskAdminIndex, redis.Z{Score: imageTaskAdminScore(&task), Member: id})
			if cleanupExists > 0 {
				pipe.Set(guardedCtx, imageTaskAdminRecordKey(id), data, 0)
				pipe.ZRem(guardedCtx, imageTaskAdminExpiry, id)
			} else {
				pipe.Set(guardedCtx, imageTaskAdminRecordKey(id), data, ttl)
				pipe.ZAdd(guardedCtx, imageTaskAdminExpiry, redis.Z{Score: float64(time.Now().Add(ttl).Unix()), Member: id})
			}
			return nil
		})
	})
}

func (s *imageTaskStore) backfillImageTaskCleanup(ctx context.Context, id string) error {
	return s.withImageTaskBackfillLock(ctx, id, func(guardedCtx context.Context) error {
		cleanup, err := s.GetCleanup(guardedCtx, id)
		if errors.Is(err, service.ErrImageTaskNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		cleanup.TaskID = id
		if cleanup.Record == nil {
			if taskData, taskErr := s.rdb.Get(guardedCtx, imageTaskKey(id)).Bytes(); taskErr == nil {
				var task service.ImageTaskRecord
				if json.Unmarshal(taskData, &task) == nil {
					cleanup.Record = &task
				}
			}
		}
		if cleanup.Record == nil {
			cleanup.Record = &service.ImageTaskRecord{
				ID: id, Status: service.ImageTaskStatusCompleted,
				CreatedAt: cleanup.ExpiresAt, ExpiresAt: cleanup.ExpiresAt,
				StorageKeys:     append([]string(nil), cleanup.Keys...),
				StorageSizes:    append([]int64(nil), cleanup.Sizes...),
				StorageIdentity: cleanup.StorageIdentity,
			}
		}
		cleanup.Record.ID = id
		if len(cleanup.Keys) == 0 {
			cleanup.Keys = append([]string(nil), cleanup.Record.StorageKeys...)
		}
		if len(cleanup.Sizes) == 0 {
			cleanup.Sizes = append([]int64(nil), cleanup.Record.StorageSizes...)
		}
		if cleanup.ExpiresAt == 0 {
			cleanup.ExpiresAt = cleanup.Record.ExpiresAt
		}
		if cleanup.StorageIdentity == "" {
			cleanup.StorageIdentity = cleanup.Record.StorageIdentity
		}
		cleanup.Record.StorageKeys = append([]string(nil), cleanup.Keys...)
		cleanup.Record.StorageSizes = append([]int64(nil), cleanup.Sizes...)
		cleanup.Record.StorageIdentity = cleanup.StorageIdentity
		images := len(cleanup.Keys)
		bytes := sumPositiveInt64(cleanup.Sizes)
		tracked, err := s.rdb.HExists(guardedCtx, imageTaskAdminObjectStats, id).Result()
		if err != nil {
			return err
		}
		cleanupData, err := json.Marshal(cleanup)
		if err != nil {
			return err
		}
		recordData, err := json.Marshal(cleanup.Record)
		if err != nil {
			return err
		}
		return s.guardedImageTaskTx(guardedCtx, id, func(pipe redis.Pipeliner) error {
			if !tracked {
				pipe.HIncrBy(guardedCtx, imageTaskAdminStats, "images", int64(images))
				pipe.HIncrBy(guardedCtx, imageTaskAdminStats, "bytes", bytes)
				pipe.HSet(guardedCtx, imageTaskAdminObjectStats, id, encodeCleanupStats(images, bytes))
			}
			pipe.Set(guardedCtx, imageTaskCleanupKey(id), cleanupData, 0)
			pipe.Set(guardedCtx, imageTaskAdminRecordKey(id), recordData, 0)
			pipe.ZAdd(guardedCtx, imageTaskAdminIndex, redis.Z{Score: imageTaskAdminScore(cleanup.Record), Member: id})
			pipe.ZRem(guardedCtx, imageTaskAdminExpiry, id)
			return nil
		})
	})
}

func (s *imageTaskStore) pruneExpiredImageTaskAdminRecords(ctx context.Context, now time.Time) error {
	ids, err := s.rdb.ZRangeByScore(ctx, imageTaskAdminExpiry, &redis.ZRangeBy{
		Min: "-inf", Max: strconv.FormatInt(now.Unix(), 10), Offset: 0, Count: 500,
	}).Result()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := imageTaskPruneAdminScript.Run(ctx, s.rdb, []string{
			imageTaskAdminExpiry,
			imageTaskCleanupKey(id),
			imageTaskAdminRecordKey(id),
			imageTaskAdminIndex,
		}, id).Err(); err != nil {
			return err
		}
	}
	return nil
}

func parseRedisInt64(value any) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case string:
		parsed, _ := strconv.ParseInt(typed, 10, 64)
		return parsed
	case []byte:
		parsed, _ := strconv.ParseInt(string(typed), 10, 64)
		return parsed
	default:
		return 0
	}
}

func sumPositiveInt64(values []int64) int64 {
	var total int64
	for _, value := range values {
		if value > 0 {
			total += value
		}
	}
	return total
}

func encodeCleanupStats(images int, bytes int64) string {
	return strconv.Itoa(images) + ":" + strconv.FormatInt(bytes, 10)
}

func imageTaskAdminScore(task *service.ImageTaskRecord) float64 {
	if task == nil {
		return 0
	}
	return float64(task.CreatedAt)
}

func imageTaskKey(id string) string {
	return imageTaskKeyPrefix + strings.TrimSpace(id)
}

func imageTaskCleanupKey(id string) string {
	return imageTaskCleanupKeyPrefix + strings.TrimSpace(id)
}

func imageTaskAdminRecordKey(id string) string {
	return imageTaskAdminRecordPrefix + strings.TrimSpace(id)
}

func imageTaskMutationLockKey(id string) string {
	return imageTaskMutationLockPrefix + strings.TrimSpace(id)
}

func imageTaskUserIndexKey(userID int64) string {
	return imageTaskUserIndexPrefix + strconv.FormatInt(userID, 10)
}

func imageTaskUserMarkerKey(userID int64) string {
	return imageTaskUserMarkerPrefix + strconv.FormatInt(userID, 10)
}
