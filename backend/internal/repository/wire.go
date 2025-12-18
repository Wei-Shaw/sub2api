package repository

import (
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
)
