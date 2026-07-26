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
