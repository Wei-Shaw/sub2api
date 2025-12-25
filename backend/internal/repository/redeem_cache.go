package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service/ports"

	"github.com/redis/go-redis/v9"
)

const (
	redeemRateLimitKeyPrefix = "redeem:ratelimit:"
	redeemLockKeyPrefix      = "redeem:lock:"
	redeemRateLimitDuration  = 24 * time.Hour
)

type redeemCache struct {
	rdb *redis.Client
}

func NewRedeemCache(rdb *redis.Client) ports.RedeemCache {
	return &redeemCache{rdb: rdb}
}

func (c *redeemCache) GetRedeemAttemptCount(ctx context.Context, userID int64) (int, error) {
	key := fmt.Sprintf("%s%d", redeemRateLimitKeyPrefix, userID)
	return c.rdb.Get(ctx, key).Int()
}

func (c *redeemCache) IncrementRedeemAttemptCount(ctx context.Context, userID int64) error {
	key := fmt.Sprintf("%s%d", redeemRateLimitKeyPrefix, userID)
	pipe := c.rdb.Pipeline()
	pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, redeemRateLimitDuration)
	_, err := pipe.Exec(ctx)
	return err
}

func (c *redeemCache) AcquireRedeemLock(ctx context.Context, code string, ttl time.Duration) (bool, error) {
	key := redeemLockKeyPrefix + code
	return c.rdb.SetNX(ctx, key, 1, ttl).Result()
}

func (c *redeemCache) ReleaseRedeemLock(ctx context.Context, code string) error {
	key := redeemLockKeyPrefix + code
	return c.rdb.Del(ctx, key).Err()
}
