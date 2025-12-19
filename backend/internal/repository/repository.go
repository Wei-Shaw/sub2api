package repository

// Repositories 所有仓库的集合
type Repositories struct {
	User             *UserRepository
	ApiKey           *ApiKeyRepository
	Group            *GroupRepository
	Account          *AccountRepository
	Proxy            *ProxyRepository
	RedeemCode       *RedeemCodeRepository
	UsageLog         *UsageLogRepository
	Setting          *SettingRepository
	UserSubscription *UserSubscriptionRepository
REDACTED
