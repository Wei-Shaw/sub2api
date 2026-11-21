package service

import (
	"context"

	dbent "github.com/Wei-Shaw/sub2api/ent"
)

// snapshotUserPlatformQuotaDefaults initializes configured limits for a new user.
// Callers keep failures best-effort and log them without failing user creation.
func snapshotUserPlatformQuotaDefaults(ctx context.Context, repo UserPlatformQuotaRepository, userID int64, quotas map[string]*DefaultPlatformQuotaSetting) error {
	if repo == nil || userID <= 0 || len(quotas) == 0 {
		return nil
	}
	// Missing rows mean unlimited; explicit zero is a configured limit.
	records := make([]UserPlatformQuotaRecord, 0, len(quotas))
	for platform, quota := range quotas {
		if !quota.HasAnyLimit() {
			continue
		}
		records = append(records, UserPlatformQuotaRecord{
			UserID:          userID,
			Platform:        platform,
			DailyLimitUSD:   quota.DailyLimitUSD,
			WeeklyLimitUSD:  quota.WeeklyLimitUSD,
			MonthlyLimitUSD: quota.MonthlyLimitUSD,
		})
	}
	if len(records) == 0 {
		return nil
	}
	// A failed best-effort write must not abort the caller's transaction.
	return repo.BulkInsertInitial(dbent.WithoutTx(ctx), records)
}
