//go:build unit

package repository

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestMaybeReset(t *testing.T) {
	start := time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC)
	other := start.AddDate(0, 0, -1)
	cases := []struct {
		name      string
		prevUsage float64
		prevStart *time.Time
		currStart time.Time
		cost      float64
		want      float64
	}{
		{"nil prev start resets", 10, nil, start, 1.5, 1.5},
		{"different start resets", 10, &other, start, 1.5, 1.5},
		{"same start accumulates", 10, &start, start, 1.5, 11.5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := maybeReset(c.prevUsage, c.prevStart, c.currStart, c.cost); got != c.want {
				t.Errorf("maybeReset = %v, want %v", got, c.want)
			}
		})
	}
}

// TestMonthlyMaybeReset_NilStart 验证 prevStart=nil 时重置。
func TestMonthlyMaybeReset_NilStart(t *testing.T) {
	now := time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
	usage, start := monthlyMaybeReset(10.0, nil, 1.5, now)
	if usage != 1.5 {
		t.Errorf("usage = %v, want 1.5", usage)
	}
	if !start.Equal(now) {
		t.Errorf("start = %v, want %v", start, now)
	}
}

// TestMonthlyMaybeReset_Expired 验证窗口满 30 天时重置（30 天恰好到期）。
func TestMonthlyMaybeReset_Expired(t *testing.T) {
	windowStart := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)
	// now = windowStart + 30d（刚好到期）
	now := windowStart.Add(30 * 24 * time.Hour)
	usage, start := monthlyMaybeReset(8.0, &windowStart, 2.0, now)
	if usage != 2.0 {
		t.Errorf("usage = %v, want 2.0 (reset)", usage)
	}
	if !start.Equal(now) {
		t.Errorf("start = %v, want %v (new window)", start, now)
	}
}

// TestMonthlyMaybeReset_CrossMonthBoundary 验证跨自然月时也使用 30 天滚动（不提前重置）。
// 旧行为：5 月 1 日跨月立即重置；新行为：窗口起始 4 月 20 日，5 月 1 日仅过了 11 天，应累加。
func TestMonthlyMaybeReset_CrossMonthBoundary(t *testing.T) {
	windowStart := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	// 5 月 1 日：距起始 11 天，不足 30 天，应累加
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	usage, start := monthlyMaybeReset(5.0, &windowStart, 1.0, now)
	if usage != 6.0 {
		t.Errorf("usage = %v, want 6.0 (accumulate, not reset at month boundary)", usage)
	}
	if !start.Equal(windowStart) {
		t.Errorf("start = %v, want %v (preserved)", start, windowStart)
	}
}

// TestMonthlyMaybeReset_Active 验证窗口内正常累加。
func TestMonthlyMaybeReset_Active(t *testing.T) {
	windowStart := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	// 15 天内，窗口有效
	now := windowStart.Add(15 * 24 * time.Hour)
	usage, start := monthlyMaybeReset(3.0, &windowStart, 0.5, now)
	if usage != 3.5 {
		t.Errorf("usage = %v, want 3.5", usage)
	}
	if !start.Equal(windowStart) {
		t.Errorf("start = %v, want %v", start, windowStart)
	}
}

// TestUpdateLimitsRowQuery_HasDeletedAtGuard 通过读取源文件验证 updateLimitsRow
// 的 SQL WHERE 子句包含 deleted_at IS NULL 守卫（I-NEW-1）。
// 此防回归测试可在无 DB 的 CI 环境中运行，防止意外删除该守卫。
func TestUpdateLimitsRowQuery_HasDeletedAtGuard(t *testing.T) {
	src, err := os.ReadFile("user_platform_quota_repo.go")
	if err != nil {
		t.Fatalf("failed to read source file: %v", err)
	}
	const guard = "AND deleted_at IS NULL"
	if !strings.Contains(string(src), guard) {
		t.Errorf("updateLimitsRow SQL must contain %q to prevent bulk reactivation of soft-deleted rows (I-NEW-1)", guard)
	}
}
