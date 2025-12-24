package repository

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service/ports"

	"github.com/redis/go-redis/v9"
)

const updateCacheKey = "update:latest"

type updateCache struct {
	rdb *redis.Client
REDACTED

func NewUpdateCache(rdb *redis.Client) ports.UpdateCache {
	return &updateCache{rdb: rdbREDACTED
REDACTED

func (c *updateCache) GetUpdateInfo(ctx context.Context) (string, error) {
	return c.rdb.Get(ctx, updateCacheKey).Result()
REDACTED

func (c *updateCache) SetUpdateInfo(ctx context.Context, data string, ttl time.Duration) error {
	return c.rdb.Set(ctx, updateCacheKey, data, ttl).Err()
REDACTED
