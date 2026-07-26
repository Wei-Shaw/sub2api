// Package billing contains the port interfaces for the Billing bounded context:
// the BillingCache contract (cache operations for subscription / API-key rate
// limit / user×platform quota data) and the PricingRemoteClient contract
// (remote price JSON fetcher). All method signatures reference pure-scalar
// DTOs defined in internal/domain, so the repository layer can implement
// these contracts without importing internal/service. The service package
// keeps type aliases to each interface so existing call sites and test stubs
// continue to satisfy them.
package billing

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// BillingCache defines cache operations for billing service
type BillingCache interface {
	// Balance operations
	GetUserBalance(ctx context.Context, userID int64) (float64, error)
	SetUserBalance(ctx context.Context, userID int64, balance float64) error
	DeductUserBalance(ctx context.Context, userID int64, amount float64) error
	InvalidateUserBalance(ctx context.Context, userID int64) error

	// Subscription operations
	GetSubscriptionCache(ctx context.Context, userID, groupID int64) (*domain.SubscriptionCacheData, error)
	SetSubscriptionCache(ctx context.Context, userID, groupID int64, data *domain.SubscriptionCacheData) error
	UpdateSubscriptionUsage(ctx context.Context, userID, groupID int64, cost float64) error
	InvalidateSubscriptionCache(ctx context.Context, userID, groupID int64) error

	// API Key rate limit operations
	GetAPIKeyRateLimit(ctx context.Context, keyID int64) (*domain.APIKeyRateLimitCacheData, error)
	SetAPIKeyRateLimit(ctx context.Context, keyID int64, data *domain.APIKeyRateLimitCacheData) error
	UpdateAPIKeyRateLimitUsage(ctx context.Context, keyID int64, cost float64) error
	InvalidateAPIKeyRateLimit(ctx context.Context, keyID int64) error

	// user × platform quota 缓存
	GetUserPlatformQuotaCache(ctx context.Context, userID int64, platform string) (*domain.UserPlatformQuotaCacheEntry, bool, error)
	SetUserPlatformQuotaCache(ctx context.Context, userID int64, platform string, entry *domain.UserPlatformQuotaCacheEntry, ttl time.Duration) error
	DeleteUserPlatformQuotaCache(ctx context.Context, userID int64, platform string) error
	// IncrUserPlatformQuotaUsageCache 在缓存命中时累加用量；缓存未命中（key 不存在）静默返回 nil。
	// markDirty=true 时将该 key 的 member 写入 Redis 脏集，供 flusher 批量回写 DB。
	IncrUserPlatformQuotaUsageCache(ctx context.Context, userID int64, platform string, cost float64, ttl time.Duration, markDirty bool) error

	// 脏集读写，供 flusher 使用。
	PopDirtyUserPlatformQuotaKeys(ctx context.Context, n int) ([]domain.UserPlatformQuotaKey, error)
	ReaddDirtyUserPlatformQuotaKeys(ctx context.Context, keys []domain.UserPlatformQuotaKey) error
	BatchGetUserPlatformQuotaCache(ctx context.Context, keys []domain.UserPlatformQuotaKey) ([]*domain.UserPlatformQuotaCacheEntry, error)
}

// PricingRemoteClient 远程价格数据获取接口
type PricingRemoteClient interface {
	FetchPricingJSON(ctx context.Context, url string) ([]byte, error)
	FetchHashText(ctx context.Context, url string) (string, error)
}

// UsageBillingRepository is the write-path contract for applying idempotent
// usage billing events and batch image balance hold / capture / release
// operations. Method signatures reference pure-scalar DTOs in internal/domain
// so the repository implementation does not need to import internal/service;
// the service package re-exports this interface as a type alias.
type UsageBillingRepository interface {
	Apply(ctx context.Context, cmd *domain.UsageBillingCommand) (*domain.UsageBillingApplyResult, error)
	ReserveBatchImageBalance(ctx context.Context, cmd *domain.BatchImageBalanceHoldCommand) (*domain.BatchImageBalanceHoldResult, error)
	CaptureBatchImageBalance(ctx context.Context, cmd *domain.BatchImageBalanceHoldCommand) (*domain.BatchImageBalanceHoldResult, error)
	ReleaseBatchImageBalance(ctx context.Context, cmd *domain.BatchImageBalanceHoldCommand) (*domain.BatchImageBalanceHoldResult, error)
}

// UserPlatformQuotaRepository 定义 service 层所需的 user × platform quota 数据访问端口。
// repository 包的 userPlatformQuotaRepository 实现此接口。
type UserPlatformQuotaRepository interface {
	// GetByUserPlatform 查询单条配额记录，未找到时返回 (nil, nil)。
	GetByUserPlatform(ctx context.Context, userID int64, platform string) (*domain.UserPlatformQuotaRecord, error)
	// BulkInsertInitial 幂等批量插入初始配额记录（ON CONFLICT DO NOTHING）。
	BulkInsertInitial(ctx context.Context, records []domain.UserPlatformQuotaRecord) error
	// IncrementUsageWithReset 原子地累加用量，若窗口已过期则先重置再累加。
	IncrementUsageWithReset(ctx context.Context, userID int64, platform string, cost float64, now time.Time) error
	// ListByUser 查询用户的所有平台配额记录。
	ListByUser(ctx context.Context, userID int64) ([]domain.UserPlatformQuotaRecord, error)
	// UpsertForUser 全量替换该用户所有平台限额配置（事务内）：
	//   1. 软删除未在 records 中出现的所有 active 行
	//   2. 对 records 中每条：UPDATE 已存在的（含重新激活软删行）；UPDATE 未命中时 INSERT
	//      仅改 *_limit_usd + deleted_at + updated_at，保留 *_usage_usd / *_window_start。
	// records 为空时仅执行步骤 1。
	UpsertForUser(ctx context.Context, userID int64, records []domain.UserPlatformQuotaRecord) error
	// ResetExpiredWindow 重置指定窗口（"daily"|"weekly"|"monthly"）的用量与起始时间。
	// 未命中活跃记录时返回（service-side wrapper of repository.ErrUserPlatformQuotaNotFound）。
	ResetExpiredWindow(ctx context.Context, userID int64, platform string, window string, newStart time.Time) error
	// BatchSnapshotUsage 绝对值覆盖写入整批 usage 快照。FK 违反返回 ErrUserPlatformQuotaFKViolation。
	BatchSnapshotUsage(ctx context.Context, snapshots []domain.UserPlatformQuotaSnapshot, now time.Time) error
}
