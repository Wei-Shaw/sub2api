package repository

import (
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
)

// ProvideConcurrencyCache 创建并发控制缓存，从配置读取 TTL 参数
// 性能优化：TTL 可配置，支持长时间运行的 LLM 请求场景
func ProvideConcurrencyCache(rdb *redis.Client, cfg *config.Config) service.ConcurrencyCache {
	waitTTLSeconds := int(cfg.Gateway.Scheduling.StickySessionWaitTimeout.Seconds())
	if cfg.Gateway.Scheduling.FallbackWaitTimeout > cfg.Gateway.Scheduling.StickySessionWaitTimeout {
		waitTTLSeconds = int(cfg.Gateway.Scheduling.FallbackWaitTimeout.Seconds())
REDACTED
	if waitTTLSeconds <= 0 {
		waitTTLSeconds = cfg.Gateway.ConcurrencySlotTTLMinutes * 60
REDACTED
	return NewConcurrencyCache(rdb, cfg.Gateway.ConcurrencySlotTTLMinutes, waitTTLSeconds)
REDACTED

// ProviderSet is the Wire provider set for all repositories
var ProviderSet = wire.NewSet(
	NewUserRepository,
	NewApiKeyRepository,
	NewGroupRepository,
	NewAccountRepository,
	NewProxyRepository,
	NewRedeemCodeRepository,
	NewUsageLogRepository,
	NewSettingRepository,
	NewUserSubscriptionRepository,

	// Cache implementations
	NewGatewayCache,
	NewBillingCache,
	NewApiKeyCache,
	ProvideConcurrencyCache,
	NewEmailCache,
	NewIdentityCache,
	NewRedeemCache,
	NewUpdateCache,
	NewGeminiTokenCache,

	// HTTP service ports (DI Strategy A: return interface directly)
	NewTurnstileVerifier,
	NewPricingRemoteClient,
	NewGitHubReleaseClient,
	NewProxyExitInfoProber,
	NewClaudeUsageFetcher,
	NewClaudeOAuthClient,
	NewHTTPUpstream,
	NewOpenAIOAuthClient,
	NewGeminiOAuthClient,
	NewGeminiCliCodeAssistClient,
)
