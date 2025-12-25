package repository

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service/ports"

	"github.com/redis/go-redis/v9"
)

const stickySessionPrefix = "sticky_session:"

type gatewayCache struct {
	rdb *redis.Client
}

func NewGatewayCache(rdb *redis.Client) ports.GatewayCache {
	return &gatewayCache{rdb: rdb}
}

func (c *gatewayCache) GetSessionAccountID(ctx context.Context, sessionHash string) (int64, error) {
	key := stickySessionPrefix + sessionHash
	return c.rdb.Get(ctx, key).Int64()
}

func (c *gatewayCache) SetSessionAccountID(ctx context.Context, sessionHash string, accountID int64, ttl time.Duration) error {
	key := stickySessionPrefix + sessionHash
	return c.rdb.Set(ctx, key, accountID, ttl).Err()
}

func (c *gatewayCache) RefreshSessionTTL(ctx context.Context, sessionHash string, ttl time.Duration) error {
	key := stickySessionPrefix + sessionHash
	return c.rdb.Expire(ctx, key, ttl).Err()
}
