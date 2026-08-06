package service

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

type grokFreeQuotaUsageContextKeyType struct{}

const grokFreeLocalUsageCooldown = 24 * time.Hour

type grokFreeQuotaUsageEntry struct {
	tokens int64
	known  bool
}

// grokOAuthUsesFreeRollingQuota treats an unclassified Grok OAuth account as
// Free. Explicit paid evidence always wins, so upgraded accounts are exempt
// even when an older Free quota snapshot is still present.
func grokOAuthUsesFreeRollingQuota(account *Account) bool {
	return account != nil && account.IsGrokOAuth() && !grokOAuthHasExplicitPaidQuota(account)
}

func grokOAuthHasExplicitPaidQuota(account *Account) bool {
	if account == nil || !account.IsGrokOAuth() {
		return false
	}

	if billing, err := grokBillingSnapshotFromExtra(account.Extra); err == nil && billing != nil {
		if tier := strings.TrimSpace(billing.Plan); tier != "" &&
			!isGrokFreeSubscriptionTier(tier) && !isGrokUnknownSubscriptionTier(tier) {
			return true
		}
		if billing.UsagePercent != nil || billing.UsedPercent != nil ||
			(billing.MonthlyLimitCents != nil && *billing.MonthlyLimitCents > 0) {
			return true
		}
	}

	if snapshot, err := grokQuotaSnapshotFromExtra(account.Extra); err == nil && snapshot != nil {
		if tier := strings.TrimSpace(snapshot.SubscriptionTier); tier != "" &&
			!isGrokFreeSubscriptionTier(tier) && !isGrokUnknownSubscriptionTier(tier) {
			return true
		}
	}

	if tier := strings.TrimSpace(account.GetCredential("subscription_tier")); tier != "" &&
		!isGrokFreeSubscriptionTier(tier) && !isGrokUnknownSubscriptionTier(tier) {
		return true
	}
	return false
}

// withGrokFreeQuotaUsagePrefetch loads one rolling-window aggregate for all
// relevant candidates. Unknown entries are retained after a failed query so
// downstream checks fail open without turning the failure into N+1 queries.
func withGrokFreeQuotaUsagePrefetch(ctx context.Context, repo UsageLogRepository, accounts []Account, now time.Time) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if repo == nil || len(accounts) == 0 {
		return ctx
	}

	entries := make(map[int64]grokFreeQuotaUsageEntry)
	if existing, ok := ctx.Value(grokFreeQuotaUsageContextKeyType{}).(map[int64]grokFreeQuotaUsageEntry); ok {
		for accountID, entry := range existing {
			entries[accountID] = entry
		}
	}

	accountIDs := make([]int64, 0, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		if account.ID <= 0 || !grokOAuthUsesFreeRollingQuota(account) {
			continue
		}
		if _, exists := entries[account.ID]; exists {
			continue
		}
		entries[account.ID] = grokFreeQuotaUsageEntry{}
		accountIDs = append(accountIDs, account.ID)
	}
	if len(accountIDs) == 0 {
		return ctx
	}

	startTime := now.UTC().Add(-grokFreeQuotaWindow)
	if batchReader, ok := repo.(accountWindowStatsBatchReader); ok {
		statsByAccount, err := batchReader.GetAccountWindowStatsBatch(ctx, accountIDs, startTime)
		if err != nil {
			slog.Warn("grok_free_quota_batch_query_failed", "accounts", len(accountIDs), "window_start", startTime, "error", err)
			return context.WithValue(ctx, grokFreeQuotaUsageContextKeyType{}, entries)
		}
		for _, accountID := range accountIDs {
			tokens := int64(0)
			if stats := statsByAccount[accountID]; stats != nil {
				tokens = stats.Tokens
			}
			entries[accountID] = grokFreeQuotaUsageEntry{tokens: tokens, known: true}
		}
		return context.WithValue(ctx, grokFreeQuotaUsageContextKeyType{}, entries)
	}

	for _, accountID := range accountIDs {
		stats, err := repo.GetAccountWindowStats(ctx, accountID, startTime)
		if err != nil {
			slog.Warn("grok_free_quota_query_failed", "account_id", accountID, "window_start", startTime, "error", err)
			continue
		}
		tokens := int64(0)
		if stats != nil {
			tokens = stats.Tokens
		}
		entries[accountID] = grokFreeQuotaUsageEntry{tokens: tokens, known: true}
	}
	return context.WithValue(ctx, grokFreeQuotaUsageContextKeyType{}, entries)
}

// grokFreeRollingQuotaExhausted returns known=false when local usage cannot be
// read. Scheduling deliberately fails open in that case.
func grokFreeRollingQuotaExhausted(ctx context.Context, repo UsageLogRepository, account *Account, now time.Time) (exhausted bool, tokens int64, known bool) {
	if !grokOAuthUsesFreeRollingQuota(account) {
		return false, 0, false
	}
	if ctx == nil {
		ctx = context.Background()
	}

	if entries, ok := ctx.Value(grokFreeQuotaUsageContextKeyType{}).(map[int64]grokFreeQuotaUsageEntry); ok {
		if entry, prefetched := entries[account.ID]; prefetched {
			if !entry.known {
				return false, 0, false
			}
			return entry.tokens >= xai.GrokFreeRolling24hTokenLimit, entry.tokens, true
		}
	}

	if repo == nil || account.ID <= 0 {
		return false, 0, false
	}
	startTime := now.UTC().Add(-grokFreeQuotaWindow)
	stats, err := repo.GetAccountWindowStats(ctx, account.ID, startTime)
	if err != nil {
		slog.Warn("grok_free_quota_query_failed", "account_id", account.ID, "window_start", startTime, "error", err)
		return false, 0, false
	}
	if stats != nil {
		tokens = stats.Tokens
	}
	return tokens >= xai.GrokFreeRolling24hTokenLimit, tokens, true
}

func (s *OpenAIGatewayService) withGrokFreeQuotaUsagePrefetch(ctx context.Context, accounts []Account) context.Context {
	if s == nil {
		return ctx
	}
	return withGrokFreeQuotaUsagePrefetch(ctx, s.usageLogRepo, accounts, time.Now())
}

func (s *OpenAIGatewayService) grokFreeRollingQuotaExhausted(ctx context.Context, account *Account) (bool, int64, bool) {
	if s == nil {
		return false, 0, false
	}
	now := time.Now()
	exhausted, tokens, known := grokFreeRollingQuotaExhausted(ctx, s.usageLogRepo, account, now)
	persistGrokFreeLocalUsageRateLimit(ctx, s.accountRepo, account, now, exhausted, known)
	return exhausted, tokens, known
}

func (s *GatewayService) withGrokFreeQuotaUsagePrefetch(ctx context.Context, accounts []Account) context.Context {
	if s == nil {
		return ctx
	}
	return withGrokFreeQuotaUsagePrefetch(ctx, s.usageLogRepo, accounts, time.Now())
}

func (s *GatewayService) grokFreeRollingQuotaExhausted(ctx context.Context, account *Account) (bool, int64, bool) {
	if s == nil {
		return false, 0, false
	}
	now := time.Now()
	exhausted, tokens, known := grokFreeRollingQuotaExhausted(ctx, s.usageLogRepo, account, now)
	persistGrokFreeLocalUsageRateLimit(ctx, s.accountRepo, account, now, exhausted, known)
	return exhausted, tokens, known
}

func persistGrokFreeLocalUsageRateLimit(
	ctx context.Context,
	repo AccountRepository,
	account *Account,
	now time.Time,
	exhausted bool,
	known bool,
) {
	if !known || !exhausted || repo == nil || account == nil || account.ID <= 0 {
		return
	}
	if account.RateLimitResetAt != nil && now.Before(*account.RateLimitResetAt) {
		return
	}
	persistGrokRateLimit(ctx, repo, account, now.Add(grokFreeLocalUsageCooldown))
}
