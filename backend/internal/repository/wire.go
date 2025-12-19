package repository

import (
	"sub2api/internal/service/ports"

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

	// Bind concrete repositories to service port interfaces
	wire.Bind(new(ports.UserRepository), new(*UserRepository)),
	wire.Bind(new(ports.ApiKeyRepository), new(*ApiKeyRepository)),
	wire.Bind(new(ports.GroupRepository), new(*GroupRepository)),
	wire.Bind(new(ports.AccountRepository), new(*AccountRepository)),
	wire.Bind(new(ports.ProxyRepository), new(*ProxyRepository)),
	wire.Bind(new(ports.RedeemCodeRepository), new(*RedeemCodeRepository)),
	wire.Bind(new(ports.UsageLogRepository), new(*UsageLogRepository)),
	wire.Bind(new(ports.SettingRepository), new(*SettingRepository)),
	wire.Bind(new(ports.UserSubscriptionRepository), new(*UserSubscriptionRepository)),
)
