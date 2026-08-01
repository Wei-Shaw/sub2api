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
	return s.rdb.Set(ctx, imageTaskKey(task.ID), data, ttl).Err()
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
