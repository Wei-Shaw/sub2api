package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

// modelDowngradeCounterPrefix 与流超时计数器（timeout_count:account:）刻意分开：
// 两种熔断的窗口和阈值不同，共用 key 会互相污染。
const modelDowngradeCounterPrefix = "model_downgrade_count:account:"

// modelDowngradeCounterIncrScript 原子递增并在首次写入时设置窗口过期时间。
var modelDowngradeCounterIncrScript = redis.NewScript(`
	local key = KEYS[1]
	local ttl = tonumber(ARGV[1])

	local count = redis.call('INCR', key)
	if count == 1 then
		redis.call('EXPIRE', key, ttl)
	end

	return count
`)

type modelDowngradeCounterCache struct {
	rdb *redis.Client
}

// NewModelDowngradeCounterCache 创建模型降级命中计数器实例
func NewModelDowngradeCounterCache(rdb *redis.Client) service.ModelDowngradeCounterCache {
	return &modelDowngradeCounterCache{rdb: rdb}
}

// modelDowngradeCounterKey 计数维度是 account_id + sent_model；模型名统一小写，
// 避免同一模型的大小写写法被拆成两个计数器。
func modelDowngradeCounterKey(accountID int64, sentModel string) string {
	return fmt.Sprintf("%s%d:model:%s", modelDowngradeCounterPrefix, accountID, strings.ToLower(strings.TrimSpace(sentModel)))
}

// IncrementModelDowngradeCount 增加一次命中计数，返回窗口内当前值
func (c *modelDowngradeCounterCache) IncrementModelDowngradeCount(ctx context.Context, accountID int64, sentModel string, windowMinutes int) (int64, error) {
	ttlSeconds := windowMinutes * 60
	if ttlSeconds < 60 {
		ttlSeconds = 60 // 最小1分钟
	}

	result, err := modelDowngradeCounterIncrScript.Run(ctx, c.rdb, []string{modelDowngradeCounterKey(accountID, sentModel)}, ttlSeconds).Int64()
	if err != nil {
		return 0, fmt.Errorf("increment model downgrade count: %w", err)
	}
	return result, nil
}

// ResetModelDowngradeCount 清零该账号 + 模型的命中计数
func (c *modelDowngradeCounterCache) ResetModelDowngradeCount(ctx context.Context, accountID int64, sentModel string) error {
	return c.rdb.Del(ctx, modelDowngradeCounterKey(accountID, sentModel)).Err()
}
