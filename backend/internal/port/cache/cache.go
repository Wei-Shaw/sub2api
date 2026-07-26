// Package cache contains the port interfaces (repository abstractions) for
// the Redis-backed cache stores owned by the service layer. These contracts
// are stdlib-only (context/time) so the repository layer can implement them
// without importing internal/service, breaking the repo→service inversion.
// The service package keeps a type alias to each interface so existing call
// sites and test stubs continue to satisfy the contract.
package cache

import (
	"context"
	"time"
)

// RPMCache RPM 计数器缓存接口
// 用于 Anthropic OAuth/SetupToken 账号的每分钟请求数限制
type RPMCache interface {
	// IncrementRPM 原子递增并返回当前分钟的计数
	// 使用 Redis 服务器时间确定 minute key，避免多实例时钟偏差
	IncrementRPM(ctx context.Context, accountID int64) (count int, err error)

	// GetRPM 获取当前分钟的 RPM 计数
	GetRPM(ctx context.Context, accountID int64) (count int, err error)

	// GetRPMBatch 批量获取多个账号的 RPM 计数（使用 Pipeline）
	GetRPMBatch(ctx context.Context, accountIDs []int64) (map[int64]int, error)
}

// SessionLimitCache 管理账号级别的活跃会话跟踪
// 用于 Anthropic OAuth/SetupToken 账号的会话数量限制
//
// Key 格式: session_limit:account:{accountID}
// 数据结构: Sorted Set (member=sessionUUID, score=timestamp)
//
// 会话在空闲超时后自动过期，无需手动清理
type SessionLimitCache interface {
	// RegisterSession 注册会话活动
	// - 如果会话已存在，刷新其时间戳并返回 true
	// - 如果会话不存在且活跃会话数 < maxSessions，添加新会话并返回 true
	// - 如果会话不存在且活跃会话数 >= maxSessions，返回 false（拒绝）
	//
	// 参数:
	//   accountID: 账号 ID
	//   sessionUUID: 从 metadata.user_id 中提取的会话 UUID
	//   maxSessions: 最大并发会话数限制
	//   idleTimeout: 会话空闲超时时间
	//
	// 返回:
	//   allowed: true 表示允许（在限制内或会话已存在），false 表示拒绝（超出限制且是新会话）
	//   error: 操作错误
	RegisterSession(ctx context.Context, accountID int64, sessionUUID string, maxSessions int, idleTimeout time.Duration) (allowed bool, err error)

	// RefreshSession 刷新现有会话的时间戳
	// 用于活跃会话保持活动状态
	RefreshSession(ctx context.Context, accountID int64, sessionUUID string, idleTimeout time.Duration) error

	// GetActiveSessionCount 获取当前活跃会话数
	// 返回未过期的会话数量
	GetActiveSessionCount(ctx context.Context, accountID int64) (int, error)

	// GetActiveSessionCountBatch 批量获取多个账号的活跃会话数
	// idleTimeouts: 每个账号的空闲超时时间配置，key 为 accountID；若为 nil 或某账号不在其中，则使用默认超时
	// 返回 map[accountID]count，查询失败的账号不在 map 中
	GetActiveSessionCountBatch(ctx context.Context, accountIDs []int64, idleTimeouts map[int64]time.Duration) (map[int64]int, error)

	// IsSessionActive 检查特定会话是否活跃（未过期）
	IsSessionActive(ctx context.Context, accountID int64, sessionUUID string) (bool, error)

	// ========== 5h窗口费用缓存 ==========
	// Key 格式: window_cost:account:{accountID}
	// 用于缓存账号在当前5h窗口内的标准费用，减少数据库聚合查询压力

	// GetWindowCost 获取缓存的窗口费用
	// 返回 (cost, true, nil) 如果缓存命中
	// 返回 (0, false, nil) 如果缓存未命中
	// 返回 (0, false, err) 如果发生错误
	GetWindowCost(ctx context.Context, accountID int64) (cost float64, hit bool, err error)

	// SetWindowCost 设置窗口费用缓存
	SetWindowCost(ctx context.Context, accountID int64, cost float64) error

	// GetWindowCostBatch 批量获取窗口费用缓存
	// 返回 map[accountID]cost，缓存未命中的账号不在 map 中
	GetWindowCostBatch(ctx context.Context, accountIDs []int64) (map[int64]float64, error)
}

// TimeoutCounterCache 超时计数器缓存接口
type TimeoutCounterCache interface {
	// IncrementTimeoutCount 增加账户的超时计数，返回当前计数值
	// windowMinutes 是计数窗口时间（分钟），超过此时间计数器会自动重置
	IncrementTimeoutCount(ctx context.Context, accountID int64, windowMinutes int) (int64, error)
	// GetTimeoutCount 获取账户当前的超时计数
	GetTimeoutCount(ctx context.Context, accountID int64) (int64, error)
	// ResetTimeoutCount 重置账户的超时计数
	ResetTimeoutCount(ctx context.Context, accountID int64) error
	// GetTimeoutCountTTL 获取计数器剩余过期时间
	GetTimeoutCountTTL(ctx context.Context, accountID int64) (time.Duration, error)
}

// LeaderLockCache provides cross-instance mutual exclusion for periodic background
// jobs. It is implemented in the repository layer (Redis-backed) so the service
// layer never depends on Redis directly. Release is a compare-and-delete keyed by
// owner so a stale holder can never delete a peer's lock.
type LeaderLockCache interface {
	// TryAcquireLeaderLock sets key=owner with the given TTL iff key is absent.
	// It returns true when the caller becomes the owner.
	TryAcquireLeaderLock(ctx context.Context, key, owner string, ttl time.Duration) (bool, error)
	// ReleaseLeaderLock deletes key iff it is still owned by owner.
	ReleaseLeaderLock(ctx context.Context, key, owner string) error
}

// UpdateCache defines cache operations for update service
type UpdateCache interface {
	GetUpdateInfo(ctx context.Context) (string, error)
	SetUpdateInfo(ctx context.Context, data string, ttl time.Duration) error
}

// UserMsgQueueCache 用户消息串行队列 Redis 缓存接口
type UserMsgQueueCache interface {
	// AcquireLock 尝试获取账号级串行锁
	AcquireLock(ctx context.Context, accountID int64, requestID string, lockTtlMs int) (acquired bool, err error)
	// ReleaseLock 释放锁并记录完成时间
	ReleaseLock(ctx context.Context, accountID int64, requestID string) (released bool, err error)
	// GetLastCompletedMs 获取上次完成时间（毫秒时间戳，Redis TIME 源）
	GetLastCompletedMs(ctx context.Context, accountID int64) (int64, error)
	// GetCurrentTimeMs 获取 Redis 服务器当前时间（毫秒），与 ReleaseLock 记录的时间源一致
	GetCurrentTimeMs(ctx context.Context) (int64, error)
	// ReconcileExpiredLockCandidates 处理锁索引中的到期候选，按真实 PTTL 清理或刷新索引
	ReconcileExpiredLockCandidates(ctx context.Context, maxCount int) (cleaned int, err error)
}

// ContentModerationHashCache defines the contract for the Redis-backed store of
// flagged input hashes used by the content moderation pre-block path.
type ContentModerationHashCache interface {
	RecordFlaggedInputHash(ctx context.Context, inputHash string) error
	HasFlaggedInputHash(ctx context.Context, inputHash string) (bool, error)
	DeleteFlaggedInputHash(ctx context.Context, inputHash string) (bool, error)
	ClearFlaggedInputHashes(ctx context.Context) (int64, error)
	CountFlaggedInputHashes(ctx context.Context) (int64, error)
}

// Internal500CounterCache 追踪 Antigravity 账号连续 INTERNAL 500 失败轮数
type Internal500CounterCache interface {
	// IncrementInternal500Count 原子递增计数并返回当前值
	IncrementInternal500Count(ctx context.Context, accountID int64) (int64, error)
	// ResetInternal500Count 清零计数器（成功响应时调用）
	ResetInternal500Count(ctx context.Context, accountID int64) error
}

// OpenAI403CounterCache 追踪 OpenAI 账号连续 403 失败次数。
type OpenAI403CounterCache interface {
	// IncrementOpenAI403Count 原子递增 403 计数并返回当前值。
	IncrementOpenAI403Count(ctx context.Context, accountID int64, windowMinutes int) (int64, error)
	// ResetOpenAI403Count 成功后清零计数器。
	ResetOpenAI403Count(ctx context.Context, accountID int64) error
}

// GeminiTokenCache stores short-lived access tokens and coordinates refresh to avoid stampedes.
type GeminiTokenCache interface {
	// cacheKey should be stable for the token scope; for GeminiCli OAuth we primarily use project_id.
	GetAccessToken(ctx context.Context, cacheKey string) (string, error)
	SetAccessToken(ctx context.Context, cacheKey string, token string, ttl time.Duration) error
	DeleteAccessToken(ctx context.Context, cacheKey string) error

	AcquireRefreshLock(ctx context.Context, cacheKey string, ttl time.Duration) (bool, error)
	ReleaseRefreshLock(ctx context.Context, cacheKey string) error
}
