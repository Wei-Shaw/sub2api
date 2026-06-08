// Package quotaview provides shared quota response helpers for user and admin handlers.
// Extracted to avoid import cycles between handler and handler/admin packages.
package quotaview

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// LazyZeroQuotaForResponse 按 D14 规则把过期档位归零（不写 DB）。
// includeWindowStart=true 时输出 *_window_start 字段（admin 视角调试用）
func LazyZeroQuotaForResponse(r service.UserPlatformQuotaRecord, now time.Time, includeWindowStart bool) map[string]any {
	daily := buildWindowSlice(r.DailyUsageUSD, r.DailyLimitUSD, r.DailyWindowStart, NeedsDailyReset(r.DailyWindowStart, now), nextDailyResetTime(now), includeWindowStart)
	weekly := buildWindowSlice(r.WeeklyUsageUSD, r.WeeklyLimitUSD, r.WeeklyWindowStart, NeedsWeeklyReset(r.WeeklyWindowStart, now), nextWeeklyResetTime(now), includeWindowStart)
	monthly := buildWindowSlice(r.MonthlyUsageUSD, r.MonthlyLimitUSD, r.MonthlyWindowStart, NeedsMonthlyReset(r.MonthlyWindowStart, now), NextMonthlyResetTimeFrom(r.MonthlyWindowStart, now), includeWindowStart)
	out := map[string]any{
		"platform":                 r.Platform,
		"daily_usage_usd":          daily.usage,
		"daily_limit_usd":          daily.limit,
		"daily_window_resets_at":   daily.resetsAt,
		"weekly_usage_usd":         weekly.usage,
		"weekly_limit_usd":         weekly.limit,
		"weekly_window_resets_at":  weekly.resetsAt,
		"monthly_usage_usd":        monthly.usage,
		"monthly_limit_usd":        monthly.limit,
		"monthly_window_resets_at": monthly.resetsAt,
	}
	if includeWindowStart {
		out["daily_window_start"] = daily.windowStart
		out["weekly_window_start"] = weekly.windowStart
		out["monthly_window_start"] = monthly.windowStart
	}
	return out
}

type windowSlice struct {
	usage       float64
	limit       *float64
	resetsAt    *string
	windowStart *string
}

func buildWindowSlice(usage float64, limit *float64, start *time.Time, expired bool, nextReset time.Time, includeStart bool) windowSlice {
	out := windowSlice{usage: usage, limit: limit}
	if expired {
		out.usage = 0
		out.resetsAt = nil
	} else if start != nil {
		s := nextReset.Format(time.RFC3339)
		out.resetsAt = &s
	}
	if includeStart && start != nil {
		s := start.Format(time.RFC3339)
		out.windowStart = &s
	}
	return out
}

// NeedsDailyReset 判断日窗口是否已过期：start 早于「全局时区当天 0 点」即过期。
// 时区跟随 timezone.Location()（全局服务器时区），与 billing / repo 写入的 window_start 同口径。
func NeedsDailyReset(start *time.Time, now time.Time) bool {
	if start == nil {
		return false
	}
	return start.Before(timezone.StartOfDay(now))
}

func NeedsWeeklyReset(start *time.Time, now time.Time) bool {
	if start == nil {
		return false
	}
	return start.Before(timezone.StartOfWeek(now))
}

// NeedsMonthlyReset 30 天滚动窗口语义（与订阅模式 NeedsMonthlyReset 一致）。
func NeedsMonthlyReset(start *time.Time, now time.Time) bool {
	if start == nil {
		return false
	}
	return now.Sub(*start) >= 30*24*time.Hour
}

func nextDailyResetTime(now time.Time) time.Time {
	return timezone.StartOfDay(now).AddDate(0, 0, 1)
}

func nextWeeklyResetTime(now time.Time) time.Time {
	return timezone.StartOfWeek(now).AddDate(0, 0, 7)
}

// NextMonthlyResetTimeFrom 计算 30 天滚动月度窗口的下次重置时间。
// 语义：
//   - start != nil → 返回 start + 30d（与 billing_cache_service.nextMonthlyResetFrom 一致）
//   - start == nil → 退化为 now + 30d（保留旧行为，避免 nil 崩溃）
//
// 导出（首字母大写）以允许测试直接调用。
func NextMonthlyResetTimeFrom(start *time.Time, now time.Time) time.Time {
	if start == nil {
		return now.Add(30 * 24 * time.Hour)
	}
	return start.Add(30 * 24 * time.Hour)
}
