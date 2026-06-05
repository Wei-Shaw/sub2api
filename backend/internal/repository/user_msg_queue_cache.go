package repository

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

// Redis Key 模式（使用 hash tag 确保 Redis Cluster 下同一 accountID 的 key 落入同一 slot）
// 格式: umq:{accountID}:lock / umq:{accountID}:last
const (
	umqKeyPrefix  = "umq:"
	umqLockSuffix = ":lock" // STRING (requestID), PX lockTtlMs
	umqLastSuffix = ":last" // STRING (毫秒时间戳), EX 60s
)

// Lua 脚本：原子获取串行锁（SET NX PX + 重入安全）
var acquireLockScript = redis.NewScript(`
local cur = redis.call('GET', KEYS[1])
if cur == ARGV[1] then
    redis.call('PEXPIRE', KEYS[1], tonumber(ARGV[2]))
    return 1
end
if cur ~= false then return 0 end
redis.call('SET', KEYS[1], ARGV[1], 'PX', tonumber(ARGV[2]))
return 1
`)

// Lua 脚本：原子释放锁 + 记录完成时间（使用 Redis TIME 避免时钟偏差）
var releaseLockScript = redis.NewScript(`
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

// Lua 脚本：原子清理孤儿锁（仅在 PTTL == -1 时删除，避免 TOCTOU 竞态误删合法锁）
var forceReleaseLockScript = redis.NewScript(`
local pttl = redis.call('PTTL', KEYS[1])
if pttl == -1 then
    redis.call('DEL', KEYS[1])
    return 1
end
return 0
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

// umqScanPattern 用于 SCAN 扫描锁 key
func umqScanPattern() string {
	return umqKeyPrefix + "{*}" + umqLockSuffix
}

// AcquireLock 尝试获取账号级串行锁
func (c *userMsgQueueCache) AcquireLock(ctx context.Context, accountID int64, requestID string, lockTtlMs int) (bool, error) {
	key := umqLockKey(accountID)
	result, err := acquireLockScript.Run(ctx, c.rdb, []string{key}, requestID, lockTtlMs).Int()
	if err != nil {
		return false, fmt.Errorf("umq acquire lock: %w", err)
	}
	return result == 1, nil
}

// ReleaseLock 释放锁并记录完成时间
func (c *userMsgQueueCache) ReleaseLock(ctx context.Context, accountID int64, requestID string) (bool, error) {
	lockKey := umqLockKey(accountID)
	lastKey := umqLastKey(accountID)
	result, err := releaseLockScript.Run(ctx, c.rdb, []string{lockKey, lastKey}, requestID).Int()
	if err != nil {
		return false, fmt.Errorf("umq release lock: %w", err)
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

// ForceReleaseLock 原子清理孤儿锁（仅在 PTTL == -1 时删除，防止 TOCTOU 竞态误删合法锁）
func (c *userMsgQueueCache) ForceReleaseLock(ctx context.Context, accountID int64) error {
	key := umqLockKey(accountID)
	_, err := forceReleaseLockScript.Run(ctx, c.rdb, []string{key}).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return fmt.Errorf("umq force release lock: %w", err)
	}
	return nil
}

// ScanLockKeys 扫描所有锁 key，仅返回 PTTL == -1（无过期时间）的孤儿锁 accountID 列表
// 正常的锁都有 PX 过期时间，PTTL == -1 表示异常状态（如 Redis 故障恢复后丢失 TTL）
func (c *userMsgQueueCache) ScanLockKeys(ctx context.Context, maxCount int) ([]int64, error) {
	var accountIDs []int64
	var cursor uint64
	pattern := umqScanPattern()

	for {
		keys, nextCursor, err := c.rdb.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return nil, fmt.Errorf("umq scan lock keys: %w", err)
		}
		for _, key := range keys {
			// 检查 PTTL：只清理 PTTL == -1（无过期时间）的异常锁
			pttl, err := c.rdb.PTTL(ctx, key).Result()
			if err != nil {
				continue
			}
			// PTTL 返回值：-2 = key 不存在，-1 = 无过期时间，>0 = 剩余毫秒
			// go-redis 对哨兵值 -1/-2 不乘精度系数，直接返回 time.Duration(-1)/-2
			// 只删除 -1（无过期时间的异常锁），跳过正常持有的锁
			if pttl != time.Duration(-1) {
				continue
			}

			// 从 key 中提取 accountID: umq:{123}:lock → 提取 {} 内的数字
			openBrace := strings.IndexByte(key, '{')
			closeBrace := strings.IndexByte(key, '}')
			if openBrace < 0 || closeBrace <= openBrace+1 {
				continue
			}
			idStr := key[openBrace+1 : closeBrace]
			id, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil {
				continue
			}
			accountIDs = append(accountIDs, id)
			if len(accountIDs) >= maxCount {
				return accountIDs, nil
			}
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return accountIDs, nil
}

// GetCurrentTimeMs 通过 Redis TIME 命令获取当前服务器时间（毫秒），确保与锁记录的时间源一致
func (c *userMsgQueueCache) GetCurrentTimeMs(ctx context.Context) (int64, error) {
	t, err := c.rdb.Time(ctx).Result()
	if err != nil {
		return 0, fmt.Errorf("umq get redis time: %w", err)
	}
	return t.UnixMilli(), nil
}
