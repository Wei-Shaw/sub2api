// Package cache contains the port interfaces (repository abstractions) for
// the Redis-backed cache stores owned by the service layer. These contracts
// are stdlib-only (context/time) so the repository layer can implement them
// without importing internal/service, breaking the repo→service inversion.
// The service package keeps a type alias to each interface so existing call
// sites and test stubs continue to satisfy the contract.
package cache

import (
	"context"
	"errors"
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

// ========== Email cache ==========

// EmailCache defines cache operations for email service
type EmailCache interface {
	GetVerificationCode(ctx context.Context, email string) (*VerificationCodeData, error)
	SetVerificationCode(ctx context.Context, email string, data *VerificationCodeData, ttl time.Duration) error
	DeleteVerificationCode(ctx context.Context, email string) error

	// Notify email verification code methods
	GetNotifyVerifyCode(ctx context.Context, email string) (*VerificationCodeData, error)
	SetNotifyVerifyCode(ctx context.Context, email string, data *VerificationCodeData, ttl time.Duration) error
	DeleteNotifyVerifyCode(ctx context.Context, email string) error

	// Password reset token methods
	GetPasswordResetToken(ctx context.Context, email string) (*PasswordResetTokenData, error)
	SetPasswordResetToken(ctx context.Context, email string, data *PasswordResetTokenData, ttl time.Duration) error
	DeletePasswordResetToken(ctx context.Context, email string) error

	// Password reset email cooldown methods
	// Returns true if in cooldown period (email was sent recently)
	IsPasswordResetEmailInCooldown(ctx context.Context, email string) bool
	SetPasswordResetEmailCooldown(ctx context.Context, email string, ttl time.Duration) error

	// Notify code rate limiting per user
	IncrNotifyCodeUserRate(ctx context.Context, userID int64, window time.Duration) (int64, error)
	GetNotifyCodeUserRate(ctx context.Context, userID int64) (int64, error)
}

// VerificationCodeData represents verification code data
type VerificationCodeData struct {
	Code      string
	Attempts  int
	CreatedAt time.Time
	ExpiresAt time.Time // absolute expiry; used to preserve remaining TTL when updating attempts
}

// PasswordResetTokenData represents password reset token data
type PasswordResetTokenData struct {
	Token     string
	CreatedAt time.Time
}

// ========== Refresh token cache ==========

// ErrRefreshTokenNotFound is returned when a refresh token is not found in cache.
// This is used to abstract away the underlying cache implementation (e.g., redis.Nil).
var ErrRefreshTokenNotFound = errors.New("refresh token not found")

// RefreshTokenData 存储在Redis中的Refresh Token数据
type RefreshTokenData struct {
	UserID       int64     `json:"user_id"`
	TokenVersion int64     `json:"token_version"`          // 用于检测密码更改后的Token失效
	FamilyID     string    `json:"family_id"`              // Token家族ID，用于防重放攻击
	BindingHash  string    `json:"binding_hash,omitempty"` // 会话指纹哈希（IP+UA），会话绑定开启时校验
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// RefreshTokenCache 管理Refresh Token的Redis缓存
// 用于JWT Token刷新机制，支持Token轮转和防重放攻击
//
// Key 格式:
//   - refresh_token:{token_hash}     -> RefreshTokenData (JSON)
//   - user_refresh_tokens:{user_id}  -> Set<token_hash>
//   - token_family:{family_id}       -> Set<token_hash>
type RefreshTokenCache interface {
	// StoreRefreshToken 存储Refresh Token
	// tokenHash: Token的SHA256哈希值（不存储原始Token）
	// data: Token关联的数据
	// ttl: Token过期时间
	StoreRefreshToken(ctx context.Context, tokenHash string, data *RefreshTokenData, ttl time.Duration) error

	// GetRefreshToken 获取Refresh Token数据
	// 返回 (data, nil) 如果Token存在
	// 返回 (nil, ErrRefreshTokenNotFound) 如果Token不存在
	// 返回 (nil, err) 如果发生其他错误
	GetRefreshToken(ctx context.Context, tokenHash string) (*RefreshTokenData, error)

	// DeleteRefreshToken 删除单个Refresh Token
	// 用于Token轮转时使旧Token失效
	DeleteRefreshToken(ctx context.Context, tokenHash string) error

	// DeleteUserRefreshTokens 删除用户的所有Refresh Token
	// 用于密码更改或用户主动登出所有设备
	DeleteUserRefreshTokens(ctx context.Context, userID int64) error

	// DeleteTokenFamily 删除整个Token家族
	// 用于检测到Token重放攻击时，撤销整个会话链
	DeleteTokenFamily(ctx context.Context, familyID string) error

	// AddToUserTokenSet 将Token添加到用户的Token集合
	// 用于跟踪用户的所有活跃Refresh Token
	AddToUserTokenSet(ctx context.Context, userID int64, tokenHash string, ttl time.Duration) error

	// AddToFamilyTokenSet 将Token添加到家族Token集合
	// 用于跟踪同一登录会话的所有Token
	AddToFamilyTokenSet(ctx context.Context, familyID string, tokenHash string, ttl time.Duration) error

	// GetUserTokenHashes 获取用户的所有Token哈希
	// 用于批量删除用户Token
	GetUserTokenHashes(ctx context.Context, userID int64) ([]string, error)

	// GetFamilyTokenHashes 获取家族的所有Token哈希
	// 用于批量删除家族Token
	GetFamilyTokenHashes(ctx context.Context, familyID string) ([]string, error)

	// IsTokenInFamily 检查Token是否属于指定家族
	// 用于验证Token家族关系
	IsTokenInFamily(ctx context.Context, familyID string, tokenHash string) (bool, error)
}

// ========== Identity cache ==========

// Fingerprint represents account fingerprint data
type Fingerprint struct {
	ClientID                string
	UserAgent               string
	StainlessLang           string
	StainlessPackageVersion string
	StainlessOS             string
	StainlessArch           string
	StainlessRuntime        string
	StainlessRuntimeVersion string
	UpdatedAt               int64 `json:",omitempty"` // Unix timestamp，用于判断是否需要续期TTL
}

// IdentityCache defines cache operations for identity service
type IdentityCache interface {
	GetFingerprint(ctx context.Context, accountID int64) (*Fingerprint, error)
	SetFingerprint(ctx context.Context, accountID int64, fp *Fingerprint) error
	// GetMaskedSessionID 获取固定的会话ID（用于会话ID伪装功能）
	// 返回的 sessionID 是一个 UUID 格式的字符串
	// 如果不存在或已过期（15分钟无请求），返回空字符串
	GetMaskedSessionID(ctx context.Context, accountID int64) (string, error)
	// SetMaskedSessionID 设置固定的会话ID，TTL 为 15 分钟
	// 每次调用都会刷新 TTL
	SetMaskedSessionID(ctx context.Context, accountID int64, sessionID string) error
}

// ========== TOTP cache ==========

// TotpCache defines cache operations for TOTP service
type TotpCache interface {
	// Setup session methods
	GetSetupSession(ctx context.Context, userID int64) (*TotpSetupSession, error)
	SetSetupSession(ctx context.Context, userID int64, session *TotpSetupSession, ttl time.Duration) error
	DeleteSetupSession(ctx context.Context, userID int64) error

	// Login session methods (for 2FA login flow)
	GetLoginSession(ctx context.Context, tempToken string) (*TotpLoginSession, error)
	SetLoginSession(ctx context.Context, tempToken string, session *TotpLoginSession, ttl time.Duration) error
	DeleteLoginSession(ctx context.Context, tempToken string) error

	// Rate limiting
	IncrementVerifyAttempts(ctx context.Context, userID int64) (int, error)
	GetVerifyAttempts(ctx context.Context, userID int64) (int, error)
	ClearVerifyAttempts(ctx context.Context, userID int64) error

	// Step-up grant methods (敏感操作 sudo 窗口)
	SetStepUpGrant(ctx context.Context, userID int64, sessionKey string, ttl time.Duration) error
	HasStepUpGrant(ctx context.Context, userID int64, sessionKey string) (bool, error)
}

// TotpSetupSession represents a TOTP setup session
type TotpSetupSession struct {
	Secret     string // Plain text TOTP secret (not encrypted yet)
	SetupToken string // Random token to verify setup request
	CreatedAt  time.Time
}

// TotpLoginSession represents a pending 2FA login session
type TotpLoginSession struct {
	UserID           int64
	Email            string
	TokenExpiry      time.Time
	PendingOAuthBind *PendingOAuthBindLoginSession `json:"pending_oauth_bind,omitempty"`
}

type PendingOAuthBindLoginSession struct {
	PendingSessionToken string `json:"pending_session_token,omitempty"`
	BrowserSessionKey   string `json:"browser_session_key,omitempty"`
}
