package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	redeemRateLimitKeyPrefix = "redeem:ratelimit:"
	redeemLockKeyPrefix      = "redeem:lock:"
	redeemRateLimitDuration  = 24 * time.Hour
)

// redeemRateLimitKey generates the Redis key for redeem attempt rate limiting.
func redeemRateLimitKey(userID int64) string {
	return fmt.Sprintf("%s%d", redeemRateLimitKeyPrefix, userID)
}

// redeemLockKey generates the Redis key for redeem code locking.
func redeemLockKey(code string) string {
	return redeemLockKeyPrefix + code
}

// redeemReleaseLockScript 比较并删除：只有持有者 token 匹配时才释放锁。
var redeemReleaseLockScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)

type redeemCache struct {
	rdb *redis.Client
}

func NewRedeemCache(rdb *redis.Client) service.RedeemCache {
	return &redeemCache{rdb: rdb}
}

func (c *redeemCache) GetRedeemAttemptCount(ctx context.Context, userID int64) (int, error) {
	key := redeemRateLimitKey(userID)
	count, err := c.rdb.Get(ctx, key).Int()
	if err == redis.Nil {
		return 0, nil
	}
	return count, err
}

func (c *redeemCache) IncrementRedeemAttemptCount(ctx context.Context, userID int64) error {
	key := redeemRateLimitKey(userID)
	pipe := c.rdb.Pipeline()
	pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, redeemRateLimitDuration)
	_, err := pipe.Exec(ctx)
	return err
}

// AcquireRedeemLock 以 token 为持有者标识抢占兑换码锁。
// token 必须由调用方随机生成，配合 ReleaseRedeemLock 的比较删除，
// 避免锁 TTL 到期后误删下一个持有者的锁。
func (c *redeemCache) AcquireRedeemLock(ctx context.Context, code, token string, ttl time.Duration) (bool, error) {
	key := redeemLockKey(code)
	return c.rdb.SetNX(ctx, key, token, ttl).Result()
}

// ReleaseRedeemLock 仅在锁仍属于本次持有者（值等于 token）时删除，
// 与 batch_image_queue 的 batchImageReleaseLockScript 采用同一套比较删除语义。
func (c *redeemCache) ReleaseRedeemLock(ctx context.Context, code, token string) error {
	if token == "" {
		return nil
	}
	key := redeemLockKey(code)
	return redeemReleaseLockScript.Run(ctx, c.rdb, []string{key}, token).Err()
}
