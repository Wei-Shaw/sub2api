package service

import (
	"database/sql"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
)

func ProvideSubscriptionServiceWithQuota(groups GroupRepository, subscriptions UserSubscriptionRepository, billing *BillingCacheService, client *dbent.Client, cfg *config.Config, quota SubscriptionQuotaRepository) *SubscriptionService {
	s := NewSubscriptionService(groups, subscriptions, billing, client, cfg)
	s.SetSubscriptionQuotaRepository(quota)
	return s
}

func ProvideSubscriptionResetExecutor(repo SubscriptionResetExecutionRepository, leader LeaderLockCache, db *sql.DB) *SubscriptionResetExecutor {
	e := NewSubscriptionResetExecutor(repo, leader, db)
	e.Start()
	return e
}
