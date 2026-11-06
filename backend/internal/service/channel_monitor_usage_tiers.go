package service

import (
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"sort"
	"time"
)

// usageQuotaTiers 把 UsageInfo 的各平台窗口归一为 tier 列表（无数据的窗口跳过）。
func usageQuotaTiers(usage *UsageInfo) []domain.MonitorQuotaTier {
	if usage == nil {
		return nil
	}
	tiers := make([]domain.MonitorQuotaTier, 0, 8)
	if usage.AntigravityQuotaSource == "" && len(usage.AntigravityQuotaGroups) == 0 {
		appendProgressTier(&tiers, "5h", "", usage.FiveHour)
		appendProgressTier(&tiers, "7d", "", usage.SevenDay)
	}
	appendProgressTier(&tiers, "7d-sonnet", "", usage.SevenDaySonnet)
	appendProgressTier(&tiers, "7d-fable", "", usage.SevenDayFable)
	appendProgressTier(&tiers, "30d", "", usage.ThirtyDay)
	// Gemini 多档日配额：同 Window 不同 Label。
	appendProgressTier(&tiers, "daily", "shared", usage.GeminiSharedDaily)
	appendProgressTier(&tiers, "daily", "pro", usage.GeminiProDaily)
	appendProgressTier(&tiers, "daily", "flash", usage.GeminiFlashDaily)
	// Grok requests/tokens 两个日窗口 + 月度计费窗口。
	appendQuotaWindowTier(&tiers, "daily", "requests", usage.GrokRequestQuota)
	appendQuotaWindowTier(&tiers, "daily", "tokens", usage.GrokTokenQuota)
	if len(usage.AntigravityQuotaGroups) > 0 {
		for _, group := range usage.AntigravityQuotaGroups {
			for _, bucket := range group.Buckets {
				if bucket.Disabled || bucket.Utilization == nil {
					continue
				}
				tier := domain.MonitorQuotaTier{Window: bucket.Window, Label: group.Name, UsedPercent: *bucket.Utilization}
				if bucket.ResetsAt != nil {
					tier.ResetAt = bucket.ResetsAt.UTC().Format(time.RFC3339)
				}
				tiers = append(tiers, tier)
			}
		}
		return tiers
	}
	// Antigravity per-model 总量额度，Label = 模型名（按名排序保证输出稳定）。
	for _, model := range sortedQuotaModelNames(usage.AntigravityQuota) {
		q := usage.AntigravityQuota[model]
		if q == nil {
			continue
		}
		tiers = append(tiers, domain.MonitorQuotaTier{
			Window:      "total",
			Label:       model,
			UsedPercent: float64(q.Utilization),
			ResetAt:     q.ResetTime,
		})
	}
	if len(tiers) == 0 {
		return nil
	}
	return tiers
}

func appendProgressTier(tiers *[]domain.MonitorQuotaTier, window, label string, p *UsageProgress) {
	if p == nil {
		return
	}
	tier := domain.MonitorQuotaTier{
		Window:      window,
		Label:       label,
		UsedPercent: p.Utilization,
	}
	if p.ResetsAt != nil {
		tier.ResetAt = p.ResetsAt.UTC().Format(time.RFC3339)
	}
	if p.LimitRequests > 0 {
		tier.Used = float64(p.UsedRequests)
		tier.Limit = float64(p.LimitRequests)
	}
	*tiers = append(*tiers, tier)
}

func appendQuotaWindowTier(tiers *[]domain.MonitorQuotaTier, window, label string, q *xai.QuotaWindow) {
	if q == nil || q.Limit == nil || *q.Limit <= 0 {
		return
	}
	used := *q.Limit
	if q.Remaining != nil {
		used = *q.Limit - *q.Remaining
		if used < 0 {
			used = 0
		}
	}
	tier := domain.MonitorQuotaTier{
		Window:      window,
		Label:       label,
		Used:        float64(used),
		Limit:       float64(*q.Limit),
		UsedPercent: float64(used) / float64(*q.Limit) * 100,
	}
	if q.ResetAt != "" {
		tier.ResetAt = q.ResetAt
	} else if q.ResetUnix != nil && *q.ResetUnix > 0 {
		tier.ResetAt = time.Unix(*q.ResetUnix, 0).UTC().Format(time.RFC3339)
	}
	*tiers = append(*tiers, tier)
}

func sortedQuotaModelNames(quotas map[string]*AntigravityModelQuota) []string {
	names := make([]string, 0, len(quotas))
	for name := range quotas {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
