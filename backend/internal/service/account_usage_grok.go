package service

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

func (s *AccountUsageService) getGrokUsage(ctx context.Context, account *Account, force bool) (*UsageInfo, error) {
	if s.grokQuotaFetcher == nil {
		now := time.Now()
		return &UsageInfo{UpdatedAt: &now}, nil
	}
	var billingProbeResult *GrokQuotaProbeResult
	if account != nil && account.IsGrokOAuth() && s.grokQuotaService != nil && (force || grokBillingSnapshotNeedsRefresh(account, time.Now())) && s.shouldProbeGrokBilling(account.ID, time.Now(), force) {
		result, err := s.grokQuotaService.ProbeBilling(ctx, account.ID)
		if err == nil && result != nil && result.Billing != nil {
			billingProbeResult = result
			mergeAccountExtra(account, map[string]any{grokBillingExtraKey: result.Billing})
		} else if err != nil && force {
			return nil, err
		}
	}
	usage := s.grokQuotaFetcher.BuildUsageInfo(account)
	if usage.GrokQuotaSnapshotState == "" {
		if usage.ErrorCode == "quota_unknown" {
			usage.GrokQuotaSnapshotState = "unknown_until_first_response"
		} else {
			usage.GrokQuotaSnapshotState = "observed"
		}
	}

	if account != nil {
		if s.usageLogRepo != nil {
			if stats, err := s.usageLogRepo.GetAccountTodayStats(ctx, account.ID); err == nil && stats != nil {
				usage.GrokLocalUsage = windowStatsFromAccountStats(stats)
			}
		}
		if billingProbeResult != nil {
			usage.GrokLocalUsage24h = billingProbeResult.LocalUsage24h
			usage.GrokLocalUsage7d = billingProbeResult.LocalUsage7d
			usage.GrokLocalUsageMonthly = billingProbeResult.LocalUsageMonthly
		} else if s.usageLogRepo != nil {
			usage.GrokLocalUsage24h, usage.GrokLocalUsage7d, usage.GrokLocalUsageMonthly = grokLocalUsageForQuota(
				ctx, s.usageLogRepo, account.ID, usage.GrokBilling, time.Now().UTC(),
			)
		}
		if usage.SevenDay != nil && usage.GrokLocalUsage7d != nil {
			usage.SevenDay.WindowStats = usage.GrokLocalUsage7d
		}
		if usage.ThirtyDay != nil && usage.GrokLocalUsageMonthly != nil {
			usage.ThirtyDay.WindowStats = usage.GrokLocalUsageMonthly
		}
	}

	enrichUsageWithAccountError(usage, account)
	return usage, nil
}

func grokLocalUsageForQuota(
	ctx context.Context,
	repo UsageLogRepository,
	accountID int64,
	billing *xai.BillingSummary,
	now time.Time,
) (*WindowStats, *WindowStats, *WindowStats) {
	if grokBillingHasAuthoritativeQuota(billing) {
		weekly, monthly := grokLocalUsageForBilling(ctx, repo, accountID, billing, now)
		return nil, weekly, monthly
	}
	return grokLocalUsage24h(ctx, repo, accountID, now), nil, nil
}

func grokLocalUsage24h(ctx context.Context, repo UsageLogRepository, accountID int64, now time.Time) *WindowStats {
	if repo == nil || accountID <= 0 {
		return nil
	}
	start := now.UTC().Add(-grokFreeQuotaWindow)
	stats, err := repo.GetAccountWindowStats(ctx, accountID, start)
	if err != nil {
		slog.Warn("grok_rolling_24h_usage_query_failed", "account_id", accountID, "window_start", start, "error", err)
		return nil
	}
	return windowStatsFromAccountStats(stats)
}

func grokLocalUsageForBilling(
	ctx context.Context,
	repo UsageLogRepository,
	accountID int64,
	billing *xai.BillingSummary,
	now time.Time,
) (*WindowStats, *WindowStats) {
	var weekly *WindowStats
	var monthly *WindowStats
	if repo == nil || accountID <= 0 {
		return weekly, monthly
	}
	if start, ok := currentGrokBillingWindow(billing, true, now); ok {
		if stats, err := repo.GetAccountWindowStats(ctx, accountID, start); err == nil {
			weekly = windowStatsFromAccountStats(stats)
		} else {
			slog.Warn("grok_window_usage_query_failed", "account_id", accountID, "window_start", start, "error", err)
		}
	}
	if start, ok := currentGrokBillingWindow(billing, false, now); ok {
		if stats, err := repo.GetAccountWindowStats(ctx, accountID, start); err == nil {
			monthly = windowStatsFromAccountStats(stats)
		} else {
			slog.Warn("grok_monthly_usage_query_failed", "account_id", accountID, "window_start", start, "error", err)
		}
	}
	return weekly, monthly
}

func currentGrokBillingWindow(billing *xai.BillingSummary, weekly bool, now time.Time) (time.Time, bool) {
	if billing == nil {
		return time.Time{}, false
	}
	startRaw, endRaw := billing.BillingPeriodStart, billing.BillingPeriodEnd
	if weekly {
		if billing.PeriodType != "weekly" {
			return time.Time{}, false
		}
		startRaw, endRaw = billing.PeriodStart, billing.PeriodEnd
	}
	start, startErr := parseTime(strings.TrimSpace(startRaw))
	end, endErr := parseTime(strings.TrimSpace(endRaw))
	if startErr != nil || endErr != nil || now.Before(start) || !now.Before(end) {
		return time.Time{}, false
	}
	return start, true
}

func grokBillingSnapshotNeedsRefresh(account *Account, now time.Time) bool {
	if account == nil {
		return false
	}
	billing, err := grokBillingSnapshotFromExtra(account.Extra)
	if err != nil || billing == nil || billing.Partial || len(billing.FailedWindows) > 0 {
		return true
	}
	stamp := strings.TrimSpace(billing.UpdatedAt)
	if stamp == "" {
		stamp = strings.TrimSpace(billing.FetchedAt)
	}
	updatedAt, err := parseTime(stamp)
	return err != nil || now.Sub(updatedAt) >= openAIProbeCacheTTL
}

func (s *AccountUsageService) shouldProbeGrokBilling(accountID int64, now time.Time, force bool) bool {
	if force || s == nil || s.cache == nil || accountID <= 0 {
		return true
	}
	if cached, ok := s.cache.grokProbeCache.Load(accountID); ok {
		if ts, ok := cached.(time.Time); ok && now.Sub(ts) < grokProbeRetryTTL {
			return false
		}
	}
	s.cache.grokProbeCache.Store(accountID, now)
	return true
}
