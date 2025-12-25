package repository

import "github.com/Wei-Shaw/sub2api/internal/service"

// Repositories 所有仓库的集合
type Repositories struct {
	User             service.UserRepository
	ApiKey           service.ApiKeyRepository
	Group            service.GroupRepository
	Account          service.AccountRepository
	Proxy            service.ProxyRepository
	RedeemCode       service.RedeemCodeRepository
	UsageLog         service.UsageLogRepository
	Setting          service.SettingRepository
	UserSubscription service.UserSubscriptionRepository
REDACTED
