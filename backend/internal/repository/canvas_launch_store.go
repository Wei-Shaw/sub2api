package repository

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const canvasLaunchStorePrefix = "canvas:launch:"

type canvasLaunchStore struct{ rdb *redis.Client }

func NewCanvasLaunchStore(rdb *redis.Client) service.CanvasLaunchStore {
	return &canvasLaunchStore{rdb: rdb}
}

func (s *canvasLaunchStore) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	return s.rdb.SetNX(ctx, canvasLaunchStorePrefix+key, value, ttl).Result()
}

func (s *canvasLaunchStore) GetDel(ctx context.Context, key string) (string, error) {
	value, err := s.rdb.GetDel(ctx, canvasLaunchStorePrefix+key).Result()
	if err == redis.Nil {
		return "", service.ErrCanvasLaunchNotFound
	}
	return value, err
}
