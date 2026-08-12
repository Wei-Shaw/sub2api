package service

import (
	"testing"
	"time"
)

func cpPtrFloat(f float64) *float64 { return &f }
func cpPtrTime(t time.Time) *time.Time { return &t }

func TestEarliestFutureReset(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	future1 := now.Add(1 * time.Hour)
	future2 := now.Add(2 * time.Hour)
	past := now.Add(-1 * time.Hour)

	// nil base + future candidate → candidate
	got := earliestFutureReset(nil, &future1, now)
	if got == nil || !got.Equal(future1) {
		t.Fatalf("nil base: want %v, got %v", future1, got)
	}
	// future candidate earlier than base → candidate wins
	got = earliestFutureReset(&future2, &future1, now)
	if got == nil || !got.Equal(future1) {
		t.Fatalf("earlier wins: want %v, got %v", future1, got)
	}
	// base earlier than candidate → base stays
	got = earliestFutureReset(&future1, &future2, now)
	if got == nil || !got.Equal(future1) {
		t.Fatalf("base stays: want %v, got %v", future1, got)
	}
	// past candidate ignored → base unchanged
	got = earliestFutureReset(&future1, &past, now)
	if got == nil || !got.Equal(future1) {
		t.Fatalf("past ignored: want %v, got %v", future1, got)
	}
	// nil candidate → base unchanged
	got = earliestFutureReset(&future1, nil, now)
	if got == nil || !got.Equal(future1) {
		t.Fatalf("nil candidate: want %v, got %v", future1, got)
	}
}

func TestCodingPlanExtraUpdates_GLMWeeklyExhausted(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	fiveHourReset := now.Add(2 * time.Hour)
	weeklyReset := now.Add(3 * time.Hour)
	snap := &MonitorQuotaSnapshot{
		PlanLevel: "max",
		Tiers: []MonitorQuotaTier{
			{Name: QuotaTierFiveHour, Utilization: cpPtrFloat(72), ResetsAt: cpPtrTime(fiveHourReset)},
			{Name: QuotaTierWeekly, Utilization: cpPtrFloat(100), ResetsAt: cpPtrTime(weeklyReset)},
		},
	}

	updates, resetAt := codingPlanExtraUpdates(MonitorProviderGLM, snap, now)
	if updates["coding_plan_provider"] != MonitorProviderGLM {
		t.Fatalf("provider: got %v", updates["coding_plan_provider"])
	}
	if updates["coding_plan_plan_level"] != "max" {
		t.Fatalf("plan_level: got %v", updates["coding_plan_plan_level"])
	}
	if updates["coding_plan_5h_used_percent"] != float64(72) {
		t.Fatalf("5h pct: got %v", updates["coding_plan_5h_used_percent"])
	}
	if updates["coding_plan_weekly_used_percent"] != float64(100) {
		t.Fatalf("weekly pct: got %v", updates["coding_plan_weekly_used_percent"])
	}
	if updates["coding_plan_5h_reset_at"] != fiveHourReset.UTC().Format(time.RFC3339) {
		t.Fatalf("5h reset: got %v", updates["coding_plan_5h_reset_at"])
	}
	if resetAt == nil || !resetAt.Equal(weeklyReset) {
		t.Fatalf("exhausted reset (only weekly at 100%%): want %v, got %v", weeklyReset, resetAt)
	}
	if updates["coding_plan_exhausted"] != true {
		t.Fatalf("exhausted flag should be true")
	}
}

func TestCodingPlanExtraUpdates_Kimi5hExhausted(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	fiveHourReset := now.Add(1 * time.Hour)
	weeklyReset := now.Add(6 * time.Hour)
	snap := &MonitorQuotaSnapshot{
		Tiers: []MonitorQuotaTier{
			{Name: QuotaTierFiveHour, Utilization: cpPtrFloat(100), ResetsAt: cpPtrTime(fiveHourReset)},
			{Name: QuotaTierWeekly, Utilization: cpPtrFloat(40), ResetsAt: cpPtrTime(weeklyReset)},
		},
	}
	updates, resetAt := codingPlanExtraUpdates(MonitorProviderKimi, snap, now)
	if resetAt == nil || !resetAt.Equal(fiveHourReset) {
		t.Fatalf("exhausted reset (5h at 100%%): want %v, got %v", fiveHourReset, resetAt)
	}
	if updates["coding_plan_exhausted"] != true {
		t.Fatalf("exhausted flag should be true")
	}
}

func TestCodingPlanExtraUpdates_NotExhausted(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	snap := &MonitorQuotaSnapshot{
		Tiers: []MonitorQuotaTier{
			{Name: QuotaTierFiveHour, Utilization: cpPtrFloat(50), ResetsAt: cpPtrTime(now.Add(1 * time.Hour))},
			{Name: QuotaTierWeekly, Utilization: cpPtrFloat(30), ResetsAt: cpPtrTime(now.Add(2 * time.Hour))},
		},
	}
	updates, resetAt := codingPlanExtraUpdates(MonitorProviderGLM, snap, now)
	if resetAt != nil {
		t.Fatalf("no exhausted window: want nil reset, got %v", resetAt)
	}
	if updates["coding_plan_exhausted"] != false {
		t.Fatalf("exhausted flag should be false")
	}
}

func TestCodingPlanExtraUpdates_DeepSeekBalanceDepleted(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	avail := false
	bal := "0.00"
	cur := "CNY"
	snap := &MonitorQuotaSnapshot{
		Available: &avail,
		Tiers: []MonitorQuotaTier{
			{Name: QuotaTierBalance, Balance: &bal, Currency: &cur},
		},
	}
	updates, resetAt := codingPlanExtraUpdates(MonitorProviderDeepseek, snap, now)
	if updates["coding_plan_available"] != false {
		t.Fatalf("available: got %v", updates["coding_plan_available"])
	}
	if updates["coding_plan_balance"] != "0.00" {
		t.Fatalf("balance: got %v", updates["coding_plan_balance"])
	}
	if updates["coding_plan_currency"] != "CNY" {
		t.Fatalf("currency: got %v", updates["coding_plan_currency"])
	}
	// DeepSeek 余额耗尽不产生时间窗口 reset（需充值，不自动冷却）。
	if resetAt != nil {
		t.Fatalf("deepseek depleted: want nil reset, got %v", resetAt)
	}
	if updates["coding_plan_exhausted"] != true {
		t.Fatalf("exhausted flag should be true for depleted balance")
	}
}
