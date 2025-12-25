package repository

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/wire"
)

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
	wire.Struct(new(Repositories), "*"),

	// Cache implementations
	NewGatewayCache,
	NewBillingCache,
	NewApiKeyCache,
	NewConcurrencyCache,
	NewEmailCache,
	NewIdentityCache,
	NewRedeemCache,
	NewUpdateCache,

	// HTTP service ports (DI Strategy A: return interface directly)
	NewTurnstileVerifier,
	NewPricingRemoteClient,
	NewGitHubReleaseClient,
	NewProxyExitInfoProber,
	NewClaudeUsageFetcher,
	NewClaudeOAuthClient,
	NewHTTPUpstream,
	NewOpenAIOAuthClient,

	// Bind concrete repositories to service port interfaces
	wire.Bind(new(service.UserRepository), new(*UserRepository)),
	wire.Bind(new(service.ApiKeyRepository), new(*ApiKeyRepository)),
	wire.Bind(new(service.GroupRepository), new(*GroupRepository)),
	wire.Bind(new(service.AccountRepository), new(*AccountRepository)),
	wire.Bind(new(service.ProxyRepository), new(*ProxyRepository)),
	wire.Bind(new(service.RedeemCodeRepository), new(*RedeemCodeRepository)),
	wire.Bind(new(service.UsageLogRepository), new(*UsageLogRepository)),
	wire.Bind(new(service.SettingRepository), new(*SettingRepository)),
	wire.Bind(new(service.UserSubscriptionRepository), new(*UserSubscriptionRepository)),
)
