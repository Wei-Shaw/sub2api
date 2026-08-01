package repository

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const imageTaskKeyPrefix = "image_task:"

const (
	imageTaskCleanupKeyPrefix = "image_task_cleanup:"
	imageTaskCleanupSchedule  = "image_task_cleanup_schedule"
	imageTaskCleanupGrace     = 7 * 24 * time.Hour
	imageTaskUserIndexPrefix  = "image_task_user:"
	imageTaskUserMarkerPrefix = "image_task_user_indexed:"
	imageTaskUserBackfillTTL  = 8 * 24 * time.Hour
)

type imageTaskStore struct {
	rdb *redis.Client
}

func NewImageTaskStore(rdb *redis.Client) service.ImageTaskStore {
	return &imageTaskStore{rdb: rdb}
}

func (s *imageTaskStore) Save(ctx context.Context, task *service.ImageTaskRecord, ttl time.Duration) error {
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}
	_, err = s.rdb.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Set(ctx, imageTaskKey(task.ID), data, ttl)
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
	return s.rdb.Del(ctx, imageTaskKey(id)).Err()
}

func (s *imageTaskStore) ScheduleCleanup(ctx context.Context, cleanup service.ImageTaskCleanup) error {
	data, err := json.Marshal(cleanup)
	if err != nil {
		return err
	}
	ttl := time.Until(time.Unix(cleanup.ExpiresAt, 0)) + imageTaskCleanupGrace
	if ttl < imageTaskCleanupGrace {
		ttl = imageTaskCleanupGrace
	}
	_, err = s.rdb.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Set(ctx, imageTaskCleanupKey(cleanup.TaskID), data, ttl)
		pipe.ZAdd(ctx, imageTaskCleanupSchedule, redis.Z{Score: float64(cleanup.ExpiresAt), Member: cleanup.TaskID})
		return nil
	})
	return err
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
	_, err := s.rdb.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Del(ctx, imageTaskCleanupKey(id))
		pipe.ZRem(ctx, imageTaskCleanupSchedule, id)
		return nil
	})
	return err
}

func imageTaskKey(id string) string {
	return imageTaskKeyPrefix + strings.TrimSpace(id)
}

func imageTaskCleanupKey(id string) string {
	return imageTaskCleanupKeyPrefix + strings.TrimSpace(id)
}

func imageTaskUserIndexKey(userID int64) string {
	return imageTaskUserIndexPrefix + strconv.FormatInt(userID, 10)
}

func imageTaskUserMarkerKey(userID int64) string {
	return imageTaskUserMarkerPrefix + strconv.FormatInt(userID, 10)
}
