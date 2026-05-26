package repository

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

// 会话限制缓存常量定义
//
// 设计说明：
// 使用 Redis 有序集合（Sorted Set）跟踪每个账号的活跃会话：
// - Key: session_limit:account:{accountID}
// - Member: sessionUUID（从 metadata.user_id 中提取）
// - Score: Unix 时间戳（会话最后活跃时间）
//
// 通过 ZREMRANGEBYSCORE 自动清理过期会话，无需手动管理 TTL
const (
	// 会话限制键前缀
	// 格式: session_limit:account:{accountID}
	sessionLimitKeyPrefix = "session_limit:account:"

	// 窗口费用缓存键前缀
	// 格式: window_cost:account:{accountID}
	windowCostKeyPrefix = "window_cost:account:"

	// 窗口费用缓存 TTL（30秒）
	windowCostCacheTTL = 30 * time.Second
)

var (
	// registerSessionScript 注册会话活动
	// 使用 Redis TIME 命令获取服务器时间，避免多实例时钟不同步
	// KEYS[1] = session_limit:account:{accountID}
	// ARGV[1] = maxSessions
	// ARGV[2] = idleTimeout（秒）
	// ARGV[3] = sessionUUID
	// 返回: 1 = 允许, 0 = 拒绝
	registerSessionScript = redis.NewScript(`
		local key = KEYS[1]
		local maxSessions = tonumber(ARGV[1])
		local idleTimeout = tonumber(ARGV[2])
		local sessionUUID = ARGV[3]

		-- 使用 Redis 服务器时间，确保多实例时钟一致
		local timeResult = redis.call('TIME')
		local now = tonumber(timeResult[1])
		local expireBefore = now - idleTimeout

		-- 清理过期会话
		redis.call('ZREMRANGEBYSCORE', key, '-inf', expireBefore)

		-- 检查会话是否已存在（支持刷新时间戳）
		local exists = redis.call('ZSCORE', key, sessionUUID)
		if exists ~= false then
			-- 会话已存在，刷新时间戳
			redis.call('ZADD', key, now, sessionUUID)
			redis.call('EXPIRE', key, idleTimeout + 60)
			return 1
		end

		-- 检查是否达到会话数量上限
		local count = redis.call('ZCARD', key)
		if count < maxSessions then
			-- 未达上限，添加新会话
			redis.call('ZADD', key, now, sessionUUID)
			redis.call('EXPIRE', key, idleTimeout + 60)
			return 1
		end

		-- 达到上限，拒绝新会话
		return 0
	`)

	// refreshSessionScript 刷新会话时间戳
	// KEYS[1] = session_limit:account:{accountID}
	// ARGV[1] = idleTimeout（秒）
	// ARGV[2] = sessionUUID
	refreshSessionScript = redis.NewScript(`
		local key = KEYS[1]
		local idleTimeout = tonumber(ARGV[1])
		local sessionUUID = ARGV[2]

		local timeResult = redis.call('TIME')
		local now = tonumber(timeResult[1])

		-- 检查会话是否存在
		local exists = redis.call('ZSCORE', key, sessionUUID)
		if exists ~= false then
			redis.call('ZADD', key, now, sessionUUID)
			redis.call('EXPIRE', key, idleTimeout + 60)
		end
		return 1
	`)

	// getActiveSessionCountScript 获取活跃会话数
	// KEYS[1] = session_limit:account:{accountID}
	// ARGV[1] = idleTimeout（秒）
	getActiveSessionCountScript = redis.NewScript(`
		local key = KEYS[1]
		local idleTimeout = tonumber(ARGV[1])

		local timeResult = redis.call('TIME')
		local now = tonumber(timeResult[1])
		local expireBefore = now - idleTimeout

		-- 清理过期会话
		redis.call('ZREMRANGEBYSCORE', key, '-inf', expireBefore)

		return redis.call('ZCARD', key)
	`)

	// isSessionActiveScript 检查会话是否活跃
	// KEYS[1] = session_limit:account:{accountID}
	// ARGV[1] = idleTimeout（秒）
	// ARGV[2] = sessionUUID
	isSessionActiveScript = redis.NewScript(`
		local key = KEYS[1]
		local idleTimeout = tonumber(ARGV[1])
		local sessionUUID = ARGV[2]

		local timeResult = redis.call('TIME')
		local now = tonumber(timeResult[1])
		local expireBefore = now - idleTimeout

		-- 获取会话的时间戳
		local score = redis.call('ZSCORE', key, sessionUUID)
		if score == false then
			return 0
		end

		-- 检查是否过期
		if tonumber(score) <= expireBefore then
			return 0
		end

		return 1
	`)
)

type sessionLimitCache struct {
	rdb                *redis.Client
	defaultIdleTimeout time.Duration // 默认空闲超时（用于 GetActiveSessionCount）
}

// NewSessionLimitCache 创建会话限制缓存
// defaultIdleTimeoutMinutes: 默认空闲超时时间（分钟），用于无参数查询
func NewSessionLimitCache(rdb *redis.Client, defaultIdleTimeoutMinutes int) service.SessionLimitCache {
	if defaultIdleTimeoutMinutes <= 0 {
		defaultIdleTimeoutMinutes = 5 // 默认 5 分钟
	}

	// 预加载 Lua 脚本到 Redis，避免 Pipeline 中出现 NOSCRIPT 错误
	ctx := context.Background()
	scripts := []*redis.Script{
		registerSessionScript,
		refreshSessionScript,
		getActiveSessionCountScript,
		isSessionActiveScript,
	}
	for _, script := range scripts {
		if err := script.Load(ctx, rdb).Err(); err != nil {
			log.Printf("[SessionLimitCache] Failed to preload Lua script: %v", err)
		}
	}

	return &sessionLimitCache{
		rdb:                rdb,
		defaultIdleTimeout: time.Duration(defaultIdleTimeoutMinutes) * time.Minute,
	}
}

// sessionLimitKey 生成会话限制的 Redis 键
func sessionLimitKey(accountID int64) string {
	return fmt.Sprintf("%s%d", sessionLimitKeyPrefix, accountID)
}

// windowCostKey 生成窗口费用缓存的 Redis 键
func windowCostKey(accountID int64) string {
	return fmt.Sprintf("%s%d", windowCostKeyPrefix, accountID)
}

// RegisterSession 注册会话活动
func (c *sessionLimitCache) RegisterSession(ctx context.Context, accountID int64, sessionUUID string, maxSessions int, idleTimeout time.Duration) (bool, error) {
	if sessionUUID == "" || maxSessions <= 0 {
		return true, nil // 无效参数，默认允许
	}

	key := sessionLimitKey(accountID)
	idleTimeoutSeconds := int(idleTimeout.Seconds())
	if idleTimeoutSeconds <= 0 {
		idleTimeoutSeconds = int(c.defaultIdleTimeout.Seconds())
	}

	result, err := registerSessionScript.Run(ctx, c.rdb, []string{key}, maxSessions, idleTimeoutSeconds, sessionUUID).Int()
	if err != nil {
		return true, err // 失败开放：缓存错误时允许请求通过
	}
	return result == 1, nil
}

// RefreshSession 刷新会话时间戳
func (c *sessionLimitCache) RefreshSession(ctx context.Context, accountID int64, sessionUUID string, idleTimeout time.Duration) error {
	if sessionUUID == "" {
		return nil
	}

	key := sessionLimitKey(accountID)
	idleTimeoutSeconds := int(idleTimeout.Seconds())
	if idleTimeoutSeconds <= 0 {
		idleTimeoutSeconds = int(c.defaultIdleTimeout.Seconds())
	}

	_, err := refreshSessionScript.Run(ctx, c.rdb, []string{key}, idleTimeoutSeconds, sessionUUID).Result()
	return err
}

// GetActiveSessionCount 获取活跃会话数
func (c *sessionLimitCache) GetActiveSessionCount(ctx context.Context, accountID int64) (int, error) {
	key := sessionLimitKey(accountID)
	idleTimeoutSeconds := int(c.defaultIdleTimeout.Seconds())

	result, err := getActiveSessionCountScript.Run(ctx, c.rdb, []string{key}, idleTimeoutSeconds).Int()
	if err != nil {
		return 0, err
	}
	return result, nil
}

// GetActiveSessionCountBatch 批量获取多个账号的活跃会话数
func (c *sessionLimitCache) GetActiveSessionCountBatch(ctx context.Context, accountIDs []int64, idleTimeouts map[int64]time.Duration) (map[int64]int, error) {
	if len(accountIDs) == 0 {
		return make(map[int64]int), nil
	}

	results := make(map[int64]int, len(accountIDs))

	// 使用 pipeline 批量执行
	pipe := c.rdb.Pipeline()

	cmds := make(map[int64]*redis.Cmd, len(accountIDs))
	for _, accountID := range accountIDs {
		key := sessionLimitKey(accountID)
		// 使用各账号自己的 idleTimeout，如果没有则用默认值
		idleTimeout := c.defaultIdleTimeout
		if idleTimeouts != nil {
			if t, ok := idleTimeouts[accountID]; ok && t > 0 {
				idleTimeout = t
			}
		}
		idleTimeoutSeconds := int(idleTimeout.Seconds())
		cmds[accountID] = getActiveSessionCountScript.Run(ctx, pipe, []string{key}, idleTimeoutSeconds)
	}

	// 执行 pipeline，即使部分失败也尝试获取成功的结果
	_, _ = pipe.Exec(ctx)

	for accountID, cmd := range cmds {
		if result, err := cmd.Int(); err == nil {
			results[accountID] = result
		}
	}

	return results, nil
}

// IsSessionActive 检查会话是否活跃
func (c *sessionLimitCache) IsSessionActive(ctx context.Context, accountID int64, sessionUUID string) (bool, error) {
	if sessionUUID == "" {
		return false, nil
	}

	key := sessionLimitKey(accountID)
	idleTimeoutSeconds := int(c.defaultIdleTimeout.Seconds())

	result, err := isSessionActiveScript.Run(ctx, c.rdb, []string{key}, idleTimeoutSeconds, sessionUUID).Int()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

// ========== 5h窗口费用缓存实现 ==========

// GetWindowCost 获取缓存的窗口费用
func (c *sessionLimitCache) GetWindowCost(ctx context.Context, accountID int64) (float64, bool, error) {
	key := windowCostKey(accountID)
	val, err := c.rdb.Get(ctx, key).Float64()
	if err == redis.Nil {
		return 0, false, nil // 缓存未命中
	}
	if err != nil {
		return 0, false, err
	}
	return val, true, nil
}

// SetWindowCost 设置窗口费用缓存
func (c *sessionLimitCache) SetWindowCost(ctx context.Context, accountID int64, cost float64) error {
	key := windowCostKey(accountID)
	return c.rdb.Set(ctx, key, cost, windowCostCacheTTL).Err()
}

// GetWindowCostBatch 批量获取窗口费用缓存
func (c *sessionLimitCache) GetWindowCostBatch(ctx context.Context, accountIDs []int64) (map[int64]float64, error) {
	if len(accountIDs) == 0 {
		return make(map[int64]float64), nil
	}

	// 构建批量查询的 keys
	keys := make([]string, len(accountIDs))
	for i, accountID := range accountIDs {
		keys[i] = windowCostKey(accountID)
	}

	// 使用 MGET 批量获取
	vals, err := c.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}

	results := make(map[int64]float64, len(accountIDs))
	for i, val := range vals {
		if val == nil {
			continue // 缓存未命中
		}
		// 尝试解析为 float64
		switch v := val.(type) {
		case string:
			if cost, err := strconv.ParseFloat(v, 64); err == nil {
				results[accountIDs[i]] = cost
			}
		case float64:
			results[accountIDs[i]] = v
		}
	}

	return results, nil
}
