package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

// CodexAnalyticsRedisCache adapts Redis to the Codex analytics service cache seam.
type CodexAnalyticsRedisCache struct {
	client *redis.Client
}

func NewCodexAnalyticsRedisCache(client *redis.Client) service.CodexAnalyticsCache {
	return &CodexAnalyticsRedisCache{client: client}
}

func (c *CodexAnalyticsRedisCache) Get(ctx context.Context, key string) ([]byte, time.Duration, error) {
	value, err := c.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, 0, service.ErrCodexAnalyticsCacheMiss
	}
	if err != nil {
		return nil, 0, err
	}
	ttl, err := c.client.PTTL(ctx, key).Result()
	if err != nil {
		return nil, 0, err
	}
	return value, ttl, nil
}

func (c *CodexAnalyticsRedisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return c.client.Set(ctx, key, value, ttl).Err()
}

func (c *CodexAnalyticsRedisCache) Delete(ctx context.Context, key string) error {
	return c.client.Del(ctx, key).Err()
}
