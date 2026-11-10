package service

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/meta"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	metaUsageWindowUtilizationKey = "meta_usage_window_utilization"
	metaUsageWindowResetKey       = "meta_usage_window_reset"
	metaUsageWeeklyUtilizationKey = "meta_usage_weekly_utilization"
	metaUsageWeeklyResetKey       = "meta_usage_weekly_reset"
	metaUsageSampledAtKey         = "meta_usage_sampled_at"
	metaUsageTierKey              = "meta_usage_tier"
	metaUsageSampleTimeout        = 5 * time.Second
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

func (s *OpenAIGatewayService) sampleMetaSubscriptionUsageWithTimeout(ctx context.Context, account *Account, usage *meta.SubscriptionUsage) {
	if usage == nil {
		return
	}
	sampleCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), metaUsageSampleTimeout)
	defer cancel()
	s.sampleMetaSubscriptionUsage(sampleCtx, account, usage)
}

func stripMetaSubscriptionUsage(body []byte) ([]byte, *meta.SubscriptionUsage) {
	if len(body) == 0 || !gjson.GetBytes(body, "subscription").IsObject() {
		return body, nil
	}
	usage, ok := meta.ParseSubscriptionUsage(body)
	if !ok {
		usage = nil
	}
	stripped, err := sjson.DeleteBytes(body, "subscription")
	if err != nil {
		return body, usage
	}
	return stripped, usage
}

func filterMetaSubscriptionUsageFromSSE(body []byte, account *Account) ([]byte, *meta.SubscriptionUsage) {
	if account == nil || !account.IsMeta() || len(body) == 0 {
		return body, nil
	}
	var snapshot *meta.SubscriptionUsage
	frames := bytes.Split(body, []byte("\n\n"))
	kept := make([][]byte, 0, len(frames))
	for _, frame := range frames {
		eventType, payload := parseSSEFrameIdentity(frame)
		if meta.IsSubscriptionUsageEvent(eventType, payload) {
			if usage, ok := meta.ParseSubscriptionUsage(payload); ok {
				snapshot = usage
			}
			continue
		}
		kept = append(kept, frame)
	}
	if len(kept) == len(frames) {
		return body, snapshot
	}
	return bytes.Join(kept, []byte("\n\n")), snapshot
}

func parseSSEFrameIdentity(frame []byte) (string, []byte) {
	var eventType string
	var data []byte
	for _, line := range bytes.Split(frame, []byte("\n")) {
		trimmed := bytes.TrimSpace(line)
		if rest, ok := bytes.CutPrefix(trimmed, []byte("event:")); ok {
			eventType = string(bytes.TrimSpace(rest))
			continue
		}
		if rest, ok := bytes.CutPrefix(trimmed, []byte("data:")); ok {
			if len(data) > 0 {
				data = append(data, '\n')
			}
			data = append(data, bytes.TrimSpace(rest)...)
		}
	}
	return eventType, data
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
