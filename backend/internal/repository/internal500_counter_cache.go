package repository

import (
	"context"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	internal500CounterPrefix    = "internal500_count:account:"
	internal500CounterTTLSeconds = 86400 // 24 小时兜底
)

// internal500CounterIncrScript 使用 Lua 脚本原子性地增加计数并返回当前值
// 如果 key 不存在，则创建并设置过期时间
var internal500CounterIncrScript = redis.NewScript(`
	local key = KEYS[1]
	local ttl = tonumber(ARGV[1])

	local count = redis.call('INCR', key)
	if count == 1 then
		redis.call('EXPIRE', key, ttl)
	end

	return count
`)

type internal500CounterCache struct {
	rdb *redis.Client
}

// NewInternal500CounterCache 创建 INTERNAL 500 连续失败计数器缓存实例
func NewInternal500CounterCache(rdb *redis.Client) service.Internal500CounterCache {
	return &internal500CounterCache{rdb: rdb}
}

// IncrementInternal500Count 原子递增计数并返回当前值
func (c *internal500CounterCache) IncrementInternal500Count(ctx context.Context, accountID int64) (int64, error) {
	key := fmt.Sprintf("%s%d", internal500CounterPrefix, accountID)

	result, err := internal500CounterIncrScript.Run(ctx, c.rdb, []string{key}, internal500CounterTTLSeconds).Int64()
	if err != nil {
		return 0, fmt.Errorf("increment internal500 count: %w", err)
	}

	return result, nil
}

// ResetInternal500Count 清零计数器（成功响应时调用）
func (c *internal500CounterCache) ResetInternal500Count(ctx context.Context, accountID int64) error {
	key := fmt.Sprintf("%s%d", internal500CounterPrefix, accountID)
	return c.rdb.Del(ctx, key).Err()
}
