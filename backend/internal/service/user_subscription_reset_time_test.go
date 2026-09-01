package service

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 周窗口初始化在开通日零点（legacy anchor）时，automaticWindowStartAt 会把实际
// 推进锚点修正为 StartsAt。展示层的 WeeklyResetTime 必须应用同一修正，
// 否则用户看到的重置时间会比实际重置时间早（差值 = StartsAt 的时分秒）。
func TestWeeklyResetTime_LegacyMidnightAnchor_UsesStartsAt(t *testing.T) {
	startsAt := time.Date(2026, 7, 31, 13, 37, 6, 0, time.FixedZone("UTC+8", 8*3600))
	windowStart := startOfDay(startsAt)

	sub := &UserSubscription{
		StartsAt:          startsAt,
		ExpiresAt:         startsAt.AddDate(0, 0, 30),
		WeeklyWindowStart: ptrTime(windowStart),
	}

	got := sub.WeeklyResetTime()
	require.NotNil(t, got)
	assert.True(t, startsAt.Add(7*24*time.Hour).Equal(*got),
		"legacy 午夜锚点应按 StartsAt+7d 计算重置时间，而不是窗口起点+7d")
}

// 非 legacy 锚点（手动重置或已推进过的窗口）保持权威，不做修正。
func TestWeeklyResetTime_RegularAnchor_UsesWindowStart(t *testing.T) {
	startsAt := time.Date(2026, 7, 31, 13, 37, 6, 0, time.FixedZone("UTC+8", 8*3600))
	windowStart := startsAt.Add(7 * 24 * time.Hour) // 已推进过一轮，非开通日零点

	sub := &UserSubscription{
		StartsAt:          startsAt,
		ExpiresAt:         startsAt.AddDate(0, 0, 30),
		WeeklyWindowStart: ptrTime(windowStart),
	}

	got := sub.WeeklyResetTime()
	require.NotNil(t, got)
	assert.True(t, windowStart.Add(7*24*time.Hour).Equal(*got),
		"非 legacy 锚点应按窗口起点+7d 计算重置时间")
}

// 展示与执行一致性：WeeklyResetTime 前一秒窗口不应推进，到点后应推进。
func TestWeeklyResetTime_MatchesAutomaticWindowStart(t *testing.T) {
	startsAt := time.Date(2026, 7, 31, 13, 37, 6, 0, time.FixedZone("UTC+8", 8*3600))
	windowStart := startOfDay(startsAt)

	sub := &UserSubscription{
		StartsAt:          startsAt,
		ExpiresAt:         startsAt.AddDate(0, 0, 30),
		WeeklyWindowStart: ptrTime(windowStart),
	}

	resetAt := *sub.WeeklyResetTime()

	_, ok := sub.automaticWindowStartAt(sub.WeeklyWindowStart, 7*24*time.Hour, resetAt.Add(-time.Second))
	assert.False(t, ok, "展示的重置时间之前窗口不应可推进")

	newStart, ok := sub.automaticWindowStartAt(sub.WeeklyWindowStart, 7*24*time.Hour, resetAt)
	assert.True(t, ok, "到达展示的重置时间后窗口应可推进")
	assert.True(t, resetAt.Equal(newStart), "推进后的新窗口起点应等于展示的重置时间")
}

// 月窗口与周窗口同属期限对齐滚动窗口，应用同一 legacy 锚点修正。
// 日窗口按日历日对齐（见 automaticDailyWindowStartAt），不走此修正，故不在此覆盖。
func TestMonthlyResetTime_LegacyMidnightAnchor_UsesStartsAt(t *testing.T) {
	startsAt := time.Date(2026, 7, 31, 13, 37, 6, 0, time.FixedZone("UTC+8", 8*3600))
	windowStart := startOfDay(startsAt)

	sub := &UserSubscription{
		StartsAt:           startsAt,
		ExpiresAt:          startsAt.AddDate(0, 0, 60),
		MonthlyWindowStart: ptrTime(windowStart),
	}

	monthly := sub.MonthlyResetTime()
	require.NotNil(t, monthly)
	assert.True(t, startsAt.Add(30*24*time.Hour).Equal(*monthly),
		"legacy 午夜锚点应按 StartsAt+30d 计算重置时间，而不是窗口起点+30d")
}

// 日窗口不受本修正影响：DailyResetTime 保持 #5380 的日历日对齐语义。
// 基准取配置时区的 0 点（与 subscription_daily_midnight_reset_test.go 同构），
// 保证断言在任意本地时区下都成立。
func TestDailyResetTime_UnaffectedByWindowResetAnchor(t *testing.T) {
	base := timezone.StartOfDay(time.Date(2026, 7, 31, 12, 0, 0, 0, timezone.Location()))
	startsAt := base.Add(13*time.Hour + 37*time.Minute + 6*time.Second)
	windowStart := base // legacy 锚点：开通日 0 点

	sub := &UserSubscription{
		StartsAt:         startsAt,
		ExpiresAt:        startsAt.AddDate(0, 0, 30),
		DailyWindowStart: ptrTime(windowStart),
	}

	got := sub.DailyResetTime()
	require.NotNil(t, got)
	assert.True(t, got.Equal(base.AddDate(0, 0, 1)),
		"日窗口应保持日历日对齐（窗口起点所在日的次日 0 点）")
	assert.False(t, got.Equal(startsAt.Add(24*time.Hour)),
		"日窗口不应被 windowResetAnchor 修正成 StartsAt+24h（那是周/月的语义）")
}
