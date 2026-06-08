package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const timeoutCounterPrefix = "timeout_count:account:"

// timeoutCounterIncrScript 使用 Lua 脚本原子性地增加计数并返回当前值
// 如果 key 不存在，则创建并设置过期时间
var timeoutCounterIncrScript = redis.NewScript(`
	local key = KEYS[1]
	local ttl = tonumber(ARGV[1])

	local count = redis.call('INCR', key)
	if count == 1 then
		redis.call('EXPIRE', key, ttl)
	end

	return count
`)

type timeoutCounterCache struct {
	rdb *redis.Client
}

// NewTimeoutCounterCache 创建超时计数器缓存实例
func NewTimeoutCounterCache(rdb *redis.Client) service.TimeoutCounterCache {
	return &timeoutCounterCache{rdb: rdb}
}

// IncrementTimeoutCount 增加账户的超时计数，返回当前计数值
// windowMinutes 是计数窗口时间（分钟），超过此时间计数器会自动重置
func (c *timeoutCounterCache) IncrementTimeoutCount(ctx context.Context, accountID int64, windowMinutes int) (int64, error) {
	key := fmt.Sprintf("%s%d", timeoutCounterPrefix, accountID)

	ttlSeconds := windowMinutes * 60
	if ttlSeconds < 60 {
		ttlSeconds = 60 // 最小1分钟
	}

	result, err := timeoutCounterIncrScript.Run(ctx, c.rdb, []string{key}, ttlSeconds).Int64()
	if err != nil {
		return 0, fmt.Errorf("increment timeout count: %w", err)
	}

	return result, nil
}

// GetTimeoutCount 获取账户当前的超时计数
func (c *timeoutCounterCache) GetTimeoutCount(ctx context.Context, accountID int64) (int64, error) {
	key := fmt.Sprintf("%s%d", timeoutCounterPrefix, accountID)

	val, err := c.rdb.Get(ctx, key).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("get timeout count: %w", err)
	}

	return val, nil
}

// ResetTimeoutCount 重置账户的超时计数
func (c *timeoutCounterCache) ResetTimeoutCount(ctx context.Context, accountID int64) error {
	key := fmt.Sprintf("%s%d", timeoutCounterPrefix, accountID)
	return c.rdb.Del(ctx, key).Err()
}

// GetTimeoutCountTTL 获取计数器剩余过期时间
func (c *timeoutCounterCache) GetTimeoutCountTTL(ctx context.Context, accountID int64) (time.Duration, error) {
	key := fmt.Sprintf("%s%d", timeoutCounterPrefix, accountID)
	return c.rdb.TTL(ctx, key).Result()
}
