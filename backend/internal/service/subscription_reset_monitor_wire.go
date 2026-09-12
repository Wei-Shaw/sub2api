package service

import "database/sql"

func ProvideSubscriptionResetMonitor(repo SubscriptionResetObserverRepository, accounts AccountRepository, quota *OpenAIQuotaService, leader LeaderLockCache, db *sql.DB) *SubscriptionResetMonitor {
	monitor := NewSubscriptionResetMonitor(repo, accounts, quota, leader, db)
	monitor.Start()
	return monitor
}
