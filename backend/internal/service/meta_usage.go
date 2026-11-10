package service

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/meta"
)

const (
	metaUsageWindowUtilizationKey = "meta_usage_window_utilization"
	metaUsageWindowResetKey       = "meta_usage_window_reset"
	metaUsageWeeklyUtilizationKey = "meta_usage_weekly_utilization"
	metaUsageWeeklyResetKey       = "meta_usage_weekly_reset"
	metaUsageSampledAtKey         = "meta_usage_sampled_at"
	metaUsageTierKey              = "meta_usage_tier"
)

func (s *OpenAIGatewayService) sampleMetaSubscriptionUsage(ctx context.Context, account *Account, usage *meta.SubscriptionUsage) {
	if s == nil || s.accountRepo == nil || account == nil || account.ID <= 0 || !account.IsMeta() || usage == nil {
		return
	}
	updates := metaSubscriptionUsageExtraUpdates(usage, time.Now())
	if len(updates) == 0 {
		return
	}
	updates[metaUsageSampledAtKey] = time.Now().UTC().Format(time.RFC3339)
	if err := s.accountRepo.UpdateExtra(ctx, account.ID, updates); err != nil {
		slog.Warn("meta_usage_sample_failed", "account_id", account.ID, "error", err)
	}
}

func metaSubscriptionUsageExtraUpdates(usage *meta.SubscriptionUsage, now time.Time) map[string]any {
	updates := make(map[string]any, 5)
	if window := metaUsageWindowIfSane(usage.Window, now); window != nil {
		updates[metaUsageWindowUtilizationKey] = window.UsedPercent / 100
		updates[metaUsageWindowResetKey] = window.ResetsAt
	}
	if window := metaUsageWindowIfSane(usage.Weekly, now); window != nil {
		updates[metaUsageWeeklyUtilizationKey] = window.UsedPercent / 100
		updates[metaUsageWeeklyResetKey] = window.ResetsAt
	}
	if len(updates) == 0 {
		return nil
	}
	if tier := strings.TrimSpace(usage.Tier); tier != "" {
		updates[metaUsageTierKey] = tier
	}
	return updates
}

func metaUsageWindowIfSane(window *meta.SubscriptionUsageWindow, now time.Time) *meta.SubscriptionUsageWindow {
	if window == nil {
		return nil
	}
	resetAt := time.Unix(window.ResetsAt, 0)
	if resetAt.Before(now.Add(-24*time.Hour)) || resetAt.After(now.Add(8*24*time.Hour)) {
		return nil
	}
	return window
}

func (s *AccountUsageService) getMetaUsage(ctx context.Context, account *Account) (*UsageInfo, error) {
	info := &UsageInfo{Source: "passive"}
	if window := buildPassiveUsageWindow(account.Extra, metaUsageWindowUtilizationKey, metaUsageWindowResetKey); window != nil {
		info.FiveHour = window
	} else {
		info.FiveHour = &UsageProgress{}
	}
	info.SevenDay = buildPassiveUsageWindow(account.Extra, metaUsageWeeklyUtilizationKey, metaUsageWeeklyResetKey)
	if tier, ok := account.Extra[metaUsageTierKey].(string); ok {
		info.SubscriptionTierRaw = strings.TrimSpace(tier)
	}
	if raw, ok := account.Extra[metaUsageSampledAtKey].(string); ok {
		if sampledAt, err := time.Parse(time.RFC3339, raw); err == nil {
			info.UpdatedAt = &sampledAt
		}
	}
	s.addWindowStats(ctx, account, info)
	return info, nil
}
