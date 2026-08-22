package repository

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

// Redis Key 模式（使用 hash tag 确保 Redis Cluster 下同一 accountID 的 key 落入同一 slot）
// 格式: umq:{accountID}:lock / umq:{accountID}:last
const (
	umqKeyPrefix  = "umq:"
	umqLockSuffix = ":lock" // STRING (requestID), PX lockTtlMs
	umqLastSuffix = ":last" // STRING (毫秒时间戳), EX 60s

	// 锁索引用来替代后台清理对 umq:*:lock 的全量 SCAN。
	// member 是 accountID，score 是锁预计过期的 Redis Unix 毫秒时间戳。
	umqLockIndexKey              = "umq:lock:index" // ZSET member=accountID, score=lockExpireAtUnixMs
	umqLockIndexCleanupBatchSize = 1000
)

// Lua 脚本：原子获取串行锁（SET NX PX + 重入安全）
// 返回 {是否获取成功, 锁预计过期时间毫秒}，让 Go 侧用同一 Redis 时间源更新索引。
// 获取失败（锁被他人持有）时也返回观测到的到期时间，供 Go 侧回填锁索引：
// 这让升级窗口遗留、索引写失败、释放竞态误删索引的存量锁在下一次被争用时自动重新入索引，
// 是替代旧 SCAN 兜底的自愈机制。PTTL == -1 的异常锁返回当前时间，使其立即成为 reconcile 候选。
var acquireLockScript = redis.NewScript(`
redis.replicate_commands()
local cur = redis.call('GET', KEYS[1])
local ttl = tonumber(ARGV[2])
if cur == ARGV[1] then
    redis.call('PEXPIRE', KEYS[1], ttl)
    local t = redis.call('TIME')
    local ms = tonumber(t[1])*1000 + math.floor(tonumber(t[2])/1000)
    return {1, ms + ttl}
end
if cur ~= false then
    local t = redis.call('TIME')
    local ms = tonumber(t[1])*1000 + math.floor(tonumber(t[2])/1000)
    local pttl = redis.call('PTTL', KEYS[1])
    if pttl and pttl > 0 then
        return {0, ms + pttl}
    end
    return {0, ms}
end
redis.call('SET', KEYS[1], ARGV[1], 'PX', ttl)
local t = redis.call('TIME')
local ms = tonumber(t[1])*1000 + math.floor(tonumber(t[2])/1000)
return {1, ms + ttl}
`)

// Lua 脚本：原子释放锁 + 记录完成时间（使用 Redis TIME 避免时钟偏差）
var releaseLockScript = redis.NewScript(`
-- Redis 3.2-4.x compat: opt into effects replication so redis.call('TIME')
-- replicates correctly. No-op on Redis 5.0+ (effects replication is default).
redis.replicate_commands()
local cur = redis.call('GET', KEYS[1])
if cur == ARGV[1] then
    redis.call('DEL', KEYS[1])
    local t = redis.call('TIME')
    local ms = tonumber(t[1])*1000 + math.floor(tonumber(t[2])/1000)
    redis.call('SET', KEYS[2], ms, 'EX', 60)
    return 1
end
return 0
`)

// Lua 脚本：校验锁 TTL 状态，PTTL == -1 时原子删除异常锁。
// 返回状态: -2=锁不存在，-1=无 TTL 的异常锁已删除，1=锁仍存活并返回剩余 PTTL。
var reconcileLockScript = redis.NewScript(`
local pttl = redis.call('PTTL', KEYS[1])
if pttl == -2 then
    return {-2, 0}
end
if pttl == -1 then
    redis.call('DEL', KEYS[1])
    return {-1, 0}
end
return {1, pttl}
`)

type userMsgQueueCache struct {
	rdb *redis.Client
}

// NewUserMsgQueueCache 创建用户消息队列缓存
func NewUserMsgQueueCache(rdb *redis.Client) service.UserMsgQueueCache {
	return &userMsgQueueCache{rdb: rdb}
}

func umqLockKey(accountID int64) string {
	// 格式: umq:{123}:lock — 花括号确保 Redis Cluster hash tag 生效
	return umqKeyPrefix + "{" + strconv.FormatInt(accountID, 10) + "}" + umqLockSuffix
}

func umqLastKey(accountID int64) string {
	// 格式: umq:{123}:last — 与 lockKey 同一 hash slot
	return umqKeyPrefix + "{" + strconv.FormatInt(accountID, 10) + "}" + umqLastSuffix
}

// AcquireLock 尝试获取账号级串行锁
// 无论成功与否都尽力写入锁索引：成功时登记自己的锁，失败时回填观测到的持有者锁，
// 保证任何被争用的锁都能被后台 reconcile 发现，无需扫描所有锁 key。
func (c *userMsgQueueCache) AcquireLock(ctx context.Context, accountID int64, requestID string, lockTtlMs int) (bool, error) {
	key := umqLockKey(accountID)
	result, err := acquireLockScript.Run(ctx, c.rdb, []string{key}, requestID, lockTtlMs).Result()
	if err != nil {
		return false, fmt.Errorf("umq acquire lock: %w", err)
	}
	acquired, err := redisScriptInt64At(result, 0)
	if err != nil {
		return false, fmt.Errorf("umq parse acquire lock result: %w", err)
	}
	expireAtMs, err := redisScriptInt64At(result, 1)
	if err != nil {
		return false, fmt.Errorf("umq parse acquire lock expire: %w", err)
	}
	if expireAtMs > 0 {
		if err := c.rdb.ZAdd(ctx, umqLockIndexKey, redis.Z{
			Score:  float64(expireAtMs),
			Member: strconv.FormatInt(accountID, 10),
		}).Err(); err != nil {
			logger.LegacyPrintf("repository.umq", "Warning: update lock index for account %d failed: %v", accountID, err)
		}
	}
	return acquired == 1, nil
}

// ReleaseLock 释放锁并记录完成时间
// 只有 requestID 匹配时才删除锁索引，避免误删其他请求重入后写入的新锁。
func (c *userMsgQueueCache) ReleaseLock(ctx context.Context, accountID int64, requestID string) (bool, error) {
	lockKey := umqLockKey(accountID)
	lastKey := umqLastKey(accountID)
	result, err := releaseLockScript.Run(ctx, c.rdb, []string{lockKey, lastKey}, requestID).Int()
	if err != nil {
		return false, fmt.Errorf("umq release lock: %w", err)
	}
	if result == 1 {
		// 与下一个 AcquireLock 的 ZAdd 存在竞态：可能误删新持有者刚写入的索引项。
		// 该锁下次被争用时 AcquireLock 的回填路径会重新登记，无需在此加锁。
		if err := c.rdb.ZRem(ctx, umqLockIndexKey, strconv.FormatInt(accountID, 10)).Err(); err != nil {
			logger.LegacyPrintf("repository.umq", "Warning: remove lock index for account %d failed: %v", accountID, err)
		}
	}
	return result == 1, nil
}

// GetLastCompletedMs 获取上次完成时间（毫秒时间戳）
func (c *userMsgQueueCache) GetLastCompletedMs(ctx context.Context, accountID int64) (int64, error) {
	key := umqLastKey(accountID)
	val, err := c.rdb.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("umq get last completed: %w", err)
	}
	ms, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("umq parse last completed: %w", err)
	}
	return ms, nil
}

// GetCurrentTimeMs 通过 Redis TIME 命令获取当前服务器时间（毫秒），确保与锁记录的时间源一致
func (c *userMsgQueueCache) GetCurrentTimeMs(ctx context.Context) (int64, error) {
	t, err := c.rdb.Time(ctx).Result()
	if err != nil {
		return 0, fmt.Errorf("umq get redis time: %w", err)
	}
	return t.UnixMilli(), nil
}

// ReconcileExpiredLockCandidates 只处理索引里已经到期的候选锁。
// 候选到期不等于锁一定失效：可能是续租后索引滞后，所以必须再用 PTTL 二次确认。
func (c *userMsgQueueCache) ReconcileExpiredLockCandidates(ctx context.Context, maxCount int) (int, error) {
	if maxCount <= 0 {
		maxCount = umqLockIndexCleanupBatchSize
	}
	nowMs, err := c.GetCurrentTimeMs(ctx)
	if err != nil {
		return 0, err
	}
	members, err := c.rdb.ZRangeByScore(ctx, umqLockIndexKey, &redis.ZRangeBy{
		Min:   "-inf",
		Max:   strconv.FormatInt(nowMs, 10),
		Count: int64(maxCount),
	}).Result()
	if err != nil {
		return 0, fmt.Errorf("umq read lock index: %w", err)
	}

	cleaned := 0
	for _, member := range members {
		accountID, err := strconv.ParseInt(member, 10, 64)
		if err != nil || accountID <= 0 {
			c.removeLockIndexMember(ctx, member)
			continue
		}

		result, err := reconcileLockScript.Run(ctx, c.rdb, []string{umqLockKey(accountID)}).Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			return cleaned, fmt.Errorf("umq reconcile lock: %w", err)
		}
		status, err := redisScriptInt64At(result, 0)
		if err != nil {
			return cleaned, fmt.Errorf("umq parse reconcile status: %w", err)
		}
		pttl, err := redisScriptInt64At(result, 1)
		if err != nil {
			return cleaned, fmt.Errorf("umq parse reconcile pttl: %w", err)
		}

		switch status {
		case -2:
			// 锁自然过期或已释放，只需移除索引残留。
			c.removeLockIndexMember(ctx, member)
		case -1:
			// 无 TTL 的锁会永久阻塞队列，Lua 已原子删除它，这里统计一次清理。
			c.removeLockIndexMember(ctx, member)
			cleaned++
		case 1:
			// 锁仍存活，说明索引过期时间滞后；按剩余 PTTL 重新排期。
			if err := c.rdb.ZAdd(ctx, umqLockIndexKey, redis.Z{
				Score:  float64(nowMs + pttl),
				Member: member,
			}).Err(); err != nil {
				logger.LegacyPrintf("repository.umq", "Warning: reschedule lock index member %s failed: %v", member, err)
			}
		}
	}
	return cleaned, nil
}

// removeLockIndexMember 移除锁索引残留；索引维护是 best-effort，失败只记日志。
func (c *userMsgQueueCache) removeLockIndexMember(ctx context.Context, member string) {
	if err := c.rdb.ZRem(ctx, umqLockIndexKey, member).Err(); err != nil {
		logger.LegacyPrintf("repository.umq", "Warning: remove lock index member %s failed: %v", member, err)
	}
}

// redisScriptInt64At 兼容 go-redis 对 Lua 数组元素的不同返回类型。
func redisScriptInt64At(result any, index int) (int64, error) {
	values, ok := result.([]any)
	if !ok {
		return 0, fmt.Errorf("expected redis script array, got %T", result)
	}
	if index < 0 || index >= len(values) {
		return 0, fmt.Errorf("redis script array missing index %d", index)
	}
	switch v := values[index].(type) {
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	case string:
		return strconv.ParseInt(v, 10, 64)
	case []byte:
		return strconv.ParseInt(string(v), 10, 64)
	default:
		return 0, fmt.Errorf("unexpected redis script value %T", v)
	}
}
