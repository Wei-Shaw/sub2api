package quotaview

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// TestNextMonthlyResetTimeFrom_FromStart 验证：start 已知时返回 start+30d，不随 now 漂移。
func TestNextMonthlyResetTimeFrom_FromStart(t *testing.T) {
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	now := t0.Add(15 * 24 * time.Hour)  // t0 + 15d
	want := t0.Add(30 * 24 * time.Hour) // t0 + 30d

	got := NextMonthlyResetTimeFrom(&t0, now)
	if !got.Equal(want) {
		t.Errorf("NextMonthlyResetTimeFrom: want %v, got %v", want, got)
	}
}

// TestNextMonthlyResetTimeFrom_NilStart 验证：start=nil 时退化为 now+30d（不 panic）。
func TestNextMonthlyResetTimeFrom_NilStart(t *testing.T) {
	now := time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC)
	want := now.Add(30 * 24 * time.Hour)

	got := NextMonthlyResetTimeFrom(nil, now)
	if !got.Equal(want) {
		t.Errorf("NextMonthlyResetTimeFrom(nil): want %v, got %v", want, got)
	}
}

// TestLazyZeroQuotaForResponse_MonthlyResetsAt_NotDrifting 验证：
// 连续两次以不同 now 调用、但 MonthlyWindowStart 相同的 record，
// monthly_window_resets_at 始终等于 windowStart+30d，不随 now 漂移。
func TestLazyZeroQuotaForResponse_MonthlyResetsAt_NotDrifting(t *testing.T) {
	windowStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	wantResetsAt := windowStart.Add(30 * 24 * time.Hour).Format(time.RFC3339)

	r := service.UserPlatformQuotaRecord{
		Platform:           "openai",
		MonthlyUsageUSD:    5.0,
		MonthlyWindowStart: &windowStart,
	}

	// 第一次调用：now = windowStart + 5d
	now1 := windowStart.Add(5 * 24 * time.Hour)
	out1 := LazyZeroQuotaForResponse(r, now1, false)
	resetsAt1, ok1 := out1["monthly_window_resets_at"]
	if !ok1 || resetsAt1 == nil {
		t.Fatal("first call: monthly_window_resets_at should be set for active window")
	}
	s1, ok := resetsAt1.(*string)
	if !ok || s1 == nil {
		t.Fatalf("first call: monthly_window_resets_at should be *string, got %T", resetsAt1)
	}
	if *s1 != wantResetsAt {
		t.Errorf("first call: want %s, got %s", wantResetsAt, *s1)
	}

	// 第二次调用：now = windowStart + 10d（不同 now，但 resetsAt 应不变）
	now2 := windowStart.Add(10 * 24 * time.Hour)
	out2 := LazyZeroQuotaForResponse(r, now2, false)
	resetsAt2, ok2 := out2["monthly_window_resets_at"]
	if !ok2 || resetsAt2 == nil {
		t.Fatal("second call: monthly_window_resets_at should be set for active window")
	}
	s2, ok := resetsAt2.(*string)
	if !ok || s2 == nil {
		t.Fatalf("second call: monthly_window_resets_at should be *string, got %T", resetsAt2)
	}
	if *s2 != wantResetsAt {
		t.Errorf("second call: want %s, got %s", wantResetsAt, *s2)
	}

	// 两次结果必须相等
	if *s1 != *s2 {
		t.Errorf("resetsAt drifted between calls: %s vs %s", *s1, *s2)
	}
}

// TestNeedsDailyReset_FollowsServerTimezone 验证日窗口过期判断按全局时区（北京 0 点）而非 UTC。
func TestNeedsDailyReset_FollowsServerTimezone(t *testing.T) {
	if err := timezone.Init("Asia/Shanghai"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { _ = timezone.Init("UTC") })

	// now = 2026-05-25 23:00 UTC = 2026-05-26 07:00 +08（北京 5/26）
	now := time.Date(2026, 5, 25, 23, 0, 0, 0, time.UTC)

	// start = 2026-05-25 10:00 UTC = 2026-05-25 18:00 +08（北京 5/25）→ 应判定为过期
	startPrevBeijingDay := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	if !NeedsDailyReset(&startPrevBeijingDay, now) {
		t.Error("上一个北京日的窗口应判定为过期")
	}

	// start = 2026-05-25 20:00 UTC = 2026-05-26 04:00 +08（北京 5/26 同日）→ 不应过期
	startSameBeijingDay := time.Date(2026, 5, 25, 20, 0, 0, 0, time.UTC)
	if NeedsDailyReset(&startSameBeijingDay, now) {
		t.Error("同一北京日的窗口不应判定为过期")
	}
}

// TestNextDailyResetTime_FollowsServerTimezone 验证下次日重置 = 次日北京 0 点。
func TestNextDailyResetTime_FollowsServerTimezone(t *testing.T) {
	if err := timezone.Init("Asia/Shanghai"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { _ = timezone.Init("UTC") })

	now := time.Date(2026, 5, 25, 23, 0, 0, 0, time.UTC)            // 北京 5/26 07:00
	want := time.Date(2026, 5, 27, 0, 0, 0, 0, timezone.Location()) // 北京 5/27 00:00
	if got := nextDailyResetTime(now); !got.Equal(want) {
		t.Errorf("nextDailyResetTime = %v, want %v", got, want)
	}
}

// TestNextWeeklyResetTime_FollowsServerTimezone 验证下次周重置 = 下周一北京 0 点。
func TestNextWeeklyResetTime_FollowsServerTimezone(t *testing.T) {
	if err := timezone.Init("Asia/Shanghai"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { _ = timezone.Init("UTC") })

	// 北京 2026-05-26（周二）→ 下周一是 2026-06-01
	now := time.Date(2026, 5, 25, 23, 0, 0, 0, time.UTC) // 北京 5/26 07:00 周二
	want := time.Date(2026, 6, 1, 0, 0, 0, 0, timezone.Location())
	if got := nextWeeklyResetTime(now); !got.Equal(want) {
		t.Errorf("nextWeeklyResetTime = %v, want %v", got, want)
	}
}
