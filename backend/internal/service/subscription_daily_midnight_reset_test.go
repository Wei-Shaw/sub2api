package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

// dailyMidnightResetRepo 记录 ResetDailyUsage 收到的新窗口起点。
type dailyMidnightResetRepo struct {
	userSubRepoNoop

	resetCalled    bool
	newWindowStart time.Time
}

func (r *dailyMidnightResetRepo) ResetDailyUsage(_ context.Context, _ int64, _ *time.Time, newWindowStart time.Time) error {
	r.resetCalled = true
	r.newWindowStart = newWindowStart
	return nil
}

// midnightTestBase 返回一个固定日期在配置时区的 0 点，测试用时刻均从它推导，
// 保证断言在任意本地时区下都与生产逻辑同构。
func midnightTestBase() time.Time {
	return timezone.StartOfDay(time.Date(2026, 8, 6, 12, 0, 0, 0, timezone.Location()))
}

func newMidnightTestSub(dailyWindowStart time.Time, base time.Time) *UserSubscription {
	start := dailyWindowStart
	return &UserSubscription{
		ID:               1,
		UserID:           10,
		GroupID:          20,
		StartsAt:         base.AddDate(0, 0, -3),
		ExpiresAt:        base.AddDate(0, 0, 30),
		DailyUsageUSD:    43.34,
		DailyWindowStart: &start,
	}
}

// 手动重置（或任何原因）留下的非 0 点锚点，跨 0 点后必须重置，
// 且新窗口起点是当天 0 点，而不是锚点+24h 的滚动时刻。
func TestCheckAndResetWindows_DailyResetsAtMidnightNotRollingAnchor(t *testing.T) {
	base := midnightTestBase()
	manualResetAt := base.Add(16*time.Hour + 49*time.Minute) // 昨日 16:49 手动重置
	now := base.AddDate(0, 0, 1).Add(5 * time.Minute)        // 次日 00:05

	repo := &dailyMidnightResetRepo{}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	svc.now = func() time.Time { return now }
	sub := newMidnightTestSub(manualResetAt, base)

	require.NoError(t, svc.CheckAndResetWindows(context.Background(), sub))

	require.True(t, repo.resetCalled, "跨 0 点后日窗口必须重置")
	require.Equal(t, base.AddDate(0, 0, 1), repo.newWindowStart, "新窗口起点应为当天 0 点")
	require.Zero(t, sub.DailyUsageUSD)
	require.Equal(t, base.AddDate(0, 0, 1), *sub.DailyWindowStart)
}

// 同一日历日内（即使已过锚点+若干小时）不得重置：手动重置当天剩余时间继续累计用量。
func TestCheckAndResetWindows_DailyNoResetWithinSameCalendarDay(t *testing.T) {
	base := midnightTestBase()
	manualResetAt := base.Add(16*time.Hour + 49*time.Minute)
	now := base.Add(23*time.Hour + 59*time.Minute)

	repo := &dailyMidnightResetRepo{}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	svc.now = func() time.Time { return now }
	sub := newMidnightTestSub(manualResetAt, base)

	require.NoError(t, svc.CheckAndResetWindows(context.Background(), sub))

	require.False(t, repo.resetCalled, "同一日历日内不应重置日窗口")
	require.Equal(t, 43.34, sub.DailyUsageUSD)
}

// 0.1.170/171 期间产生的滚动锚点（如多日前的 17:18）自愈：下一次维护即拉回今天 0 点。
func TestCheckAndResetWindows_LegacyRollingAnchorHealsToMidnight(t *testing.T) {
	base := midnightTestBase()
	staleAnchor := base.AddDate(0, 0, -3).Add(17*time.Hour + 18*time.Minute)
	now := base.Add(10 * time.Hour)

	repo := &dailyMidnightResetRepo{}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	svc.now = func() time.Time { return now }
	sub := newMidnightTestSub(staleAnchor, base)

	require.NoError(t, svc.CheckAndResetWindows(context.Background(), sub))

	require.True(t, repo.resetCalled)
	require.Equal(t, base, repo.newWindowStart, "滚动锚点应被拉回今天 0 点，而非按 17:18 步进")
}

// 手动重置写入的 0 点锚点不会改变刷新节奏：次日 0 点照常需要重置。
func TestNeedsDailyReset_MidnightScheduleSurvivesManualReset(t *testing.T) {
	base := midnightTestBase()
	sub := newMidnightTestSub(base, base) // 手动重置后锚点=当天 0 点

	require.False(t, sub.NeedsDailyResetAt(base.Add(23*time.Hour+54*time.Minute)), "当天 23:54 不应重置")
	require.True(t, sub.NeedsDailyResetAt(base.AddDate(0, 0, 1).Add(time.Minute)), "次日 00:01 应重置")
}

// 多日订阅的日窗口展示的下次刷新时间固定为次日 0 点（截图中「X 小时后重置」的数据源）。
func TestDailyResetTime_NextMidnightForMultiDaySubscription(t *testing.T) {
	base := midnightTestBase()

	// 0 点锚点 → 次日 0 点
	sub := newMidnightTestSub(base, base)
	resetAt := sub.DailyResetTime()
	require.NotNil(t, resetAt)
	require.Equal(t, base.AddDate(0, 0, 1), *resetAt)

	// 非 0 点滚动锚点 → 仍是其所在日的次日 0 点，而非锚点+24h
	rolling := newMidnightTestSub(base.Add(16*time.Hour+49*time.Minute), base)
	resetAt = rolling.DailyResetTime()
	require.NotNil(t, resetAt)
	require.Equal(t, base.AddDate(0, 0, 1), *resetAt)
}

// 截图场景：跨 0 点后（后台尚未收到请求推进窗口）列表展示即应清零日用量。
func TestNormalizeExpiredWindows_DailyUsageClearsAfterMidnight(t *testing.T) {
	base := midnightTestBase()
	manualResetAt := base.Add(16*time.Hour + 49*time.Minute)
	now := base.AddDate(0, 0, 1).Add(time.Minute) // 次日 00:01

	subs := []UserSubscription{*newMidnightTestSub(manualResetAt, base)}
	normalizeExpiredWindowsAt(subs, now)

	require.Zero(t, subs[0].DailyUsageUSD, "跨 0 点后展示的日用量应清零")
	require.Nil(t, subs[0].DailyWindowStart)
}

// 日卡（一次性日额度）不受 0 点语义影响：跨 0 点不重置。
func TestCheckAndResetWindows_OneTimeDailyCardStillExemptFromMidnightReset(t *testing.T) {
	base := midnightTestBase()
	startsAt := base.Add(17 * time.Hour)
	anchor := base
	now := base.AddDate(0, 0, 1).Add(2 * time.Hour)

	repo := &dailyMidnightResetRepo{}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	svc.now = func() time.Time { return now }
	sub := &UserSubscription{
		ID:               1,
		UserID:           10,
		GroupID:          20,
		StartsAt:         startsAt,
		ExpiresAt:        startsAt.AddDate(0, 0, 1),
		DailyUsageUSD:    10,
		DailyWindowStart: &anchor,
	}

	require.NoError(t, svc.CheckAndResetWindows(context.Background(), sub))

	require.False(t, repo.resetCalled, "日卡为一次性配额，跨 0 点不应重置")
	require.Equal(t, 10.0, sub.DailyUsageUSD)
}
