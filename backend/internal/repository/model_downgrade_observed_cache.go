package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

// modelDowngradeObservedPrefix 是观察记录的 key 前缀。
// 与计数器（model_downgrade_count:account:）分开：计数器按窗口过期、值是数字，
// 观察记录按处理时长过期、值是整条 JSON，两者生命周期完全不同。
const modelDowngradeObservedPrefix = "model_downgrade_guard:observed:"

// modelDowngradeObservedScanCount 是 SCAN 的每轮建议条数。
// 观察记录条数很少（受阈值和 TTL 限制），SCAN 足够，不需要维护额外的索引集合。
const modelDowngradeObservedScanCount = 100

type modelDowngradeObservedCache struct {
	rdb *redis.Client
}

// NewModelDowngradeObservedCache 创建降级守卫观察记录存储实例
func NewModelDowngradeObservedCache(rdb *redis.Client) service.ModelDowngradeObservedCache {
	return &modelDowngradeObservedCache{rdb: rdb}
}

// modelDowngradeObservedKey 维度是 account_id + sent_model；模型名统一小写，
// 与计数器的 key 口径保持一致，避免同一模型的大小写写法写出两条记录。
func modelDowngradeObservedKey(accountID int64, sentModel string) string {
	return fmt.Sprintf("%s%d:%s", modelDowngradeObservedPrefix, accountID, strings.ToLower(strings.TrimSpace(sentModel)))
}

// RecordObserved 写入（或覆盖）一条观察记录。
// 同一账号 + 模型再次命中时直接覆盖旧值并重置 TTL，页面上永远只留最新的一条。
func (c *modelDowngradeObservedCache) RecordObserved(ctx context.Context, entry service.ModelDowngradeObservedEntry, ttl time.Duration) error {
	if ttl <= 0 {
		return fmt.Errorf("record model downgrade observed: ttl must be positive")
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("record model downgrade observed: %w", err)
	}
	key := modelDowngradeObservedKey(entry.AccountID, entry.SentModel)
	if err := c.rdb.Set(ctx, key, raw, ttl).Err(); err != nil {
		return fmt.Errorf("record model downgrade observed: %w", err)
	}
	return nil
}

// ListObserved 返回所有未过期的观察记录。
//
// 单条解析失败只跳过并打 warn：一条脏数据不应该让整张表打不开。
// ExpiresAt 在写入时就算好了，这里不再回查 TTL。
func (c *modelDowngradeObservedCache) ListObserved(ctx context.Context) ([]service.ModelDowngradeObservedEntry, error) {
	keys, err := c.scanKeys(ctx, modelDowngradeObservedPrefix+"*")
	if err != nil {
		return nil, fmt.Errorf("list model downgrade observed: %w", err)
	}
	if len(keys) == 0 {
		return nil, nil
	}

	values, err := c.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("list model downgrade observed: %w", err)
	}

	entries := make([]service.ModelDowngradeObservedEntry, 0, len(values))
	for i, value := range values {
		// SCAN 到 MGET 之间 key 可能已经过期，nil 不是异常。
		if value == nil {
			continue
		}
		raw, ok := value.(string)
		if !ok {
			slog.Warn("model_downgrade_observed_unexpected_value_type", "key", keys[i])
			continue
		}
		var entry service.ModelDowngradeObservedEntry
		if err := json.Unmarshal([]byte(raw), &entry); err != nil {
			slog.Warn("model_downgrade_observed_decode_failed", "key", keys[i], "error", err)
			continue
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// DeleteObserved 删除一条观察记录，返回是否真的删掉了（调用方据此区分 404）。
func (c *modelDowngradeObservedCache) DeleteObserved(ctx context.Context, accountID int64, sentModel string) (bool, error) {
	removed, err := c.rdb.Del(ctx, modelDowngradeObservedKey(accountID, sentModel)).Result()
	if err != nil {
		return false, fmt.Errorf("delete model downgrade observed: %w", err)
	}
	return removed > 0, nil
}

// scanKeys 用 SCAN 循环到游标归零，避免 KEYS 在生产实例上阻塞。
// SCAN 允许同一个 key 在多轮里重复出现，这里去重，否则列表会出现重复行。
func (c *modelDowngradeObservedCache) scanKeys(ctx context.Context, pattern string) ([]string, error) {
	var (
		keys   []string
		cursor uint64
	)
	seen := make(map[string]struct{})
	for {
		batch, next, err := c.rdb.Scan(ctx, cursor, pattern, modelDowngradeObservedScanCount).Result()
		if err != nil {
			return nil, err
		}
		for _, key := range batch {
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			keys = append(keys, key)
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return keys, nil
}
