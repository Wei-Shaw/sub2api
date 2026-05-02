//go:build unit

package repository

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestFixedWindowTTL_AlignsToMinuteBoundary 覆盖 P0 修复的核心断言：
// fixed window TTL 必须按 wall-clock 整分钟边界对齐，而不是从首次写入起算 60s。
//
// 0s / 30s / 59s 三个写入时刻分别期望 TTL = 60s / 30s / 1s
// （秒级精度足够；纳秒余数会让结果偏小一个纳秒，断言时按 second 截断）。
func TestFixedWindowTTL_AlignsToMinuteBoundary(t *testing.T) {
	t.Parallel()
	window := time.Minute

	cases := []struct {
		name    string
		nowSec  int64 // Unix 秒数，从 epoch 起
		wantSec int64 // 期望 TTL 秒数
	}{
		{"分钟边界（0s）→ 整 60s", 0, 60},
		{"半分钟（30s）→ 30s", 30, 30},
		{"分钟末尾（59s）→ 1s", 59, 1},
		{"任意分钟的 18s（用户截图场景）→ 42s", 60*123 + 18, 42},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			now := time.Unix(tc.nowSec, 0)
			ttl := fixedWindowTTL(now, window)
			require.Equal(t, tc.wantSec, int64(ttl/time.Second),
				"now=%v window=%v got TTL=%v want=%ds", now, window, ttl, tc.wantSec)
		})
	}
}

// TestFixedWindowTTL_AlignsToHourBoundary 验证小时窗口对齐到整点。
func TestFixedWindowTTL_AlignsToHourBoundary(t *testing.T) {
	t.Parallel()
	window := time.Hour

	cases := []struct {
		name    string
		nowSec  int64
		wantSec int64
	}{
		{"小时整点 → 3600s", 0, 3600},
		{"小时第 30 分 → 1800s", 30 * 60, 1800},
		{"小时最后 1s → 1s", 3599, 1},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			now := time.Unix(tc.nowSec, 0)
			ttl := fixedWindowTTL(now, window)
			require.Equal(t, tc.wantSec, int64(ttl/time.Second))
		})
	}
}

// TestFixedWindowTTL_AlignsToUTCDayBoundary 验证 24h 窗口对齐到 UTC 整天 00:00。
//
// 关键不变量：TPD/daily_usd 周期必须按 UTC 对齐（与上游 OpenAI/Anthropic billing 周期一致），
// 不受服务器本地时区影响。这里用 epoch（UTC 00:00:00）+ 已知秒数构造 now，
// 期望 TTL 严格等于 86400 - (now_unix % 86400)。
func TestFixedWindowTTL_AlignsToUTCDayBoundary(t *testing.T) {
	t.Parallel()
	window := 24 * time.Hour

	cases := []struct {
		name    string
		nowUTC  time.Time
		wantSec int64
	}{
		{"UTC 整天 → 24h", time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC), 86400},
		{"UTC 中午 12:00 → 12h", time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC), 12 * 3600},
		{"UTC 23:59:59 → 1s", time.Date(2026, 4, 27, 23, 59, 59, 0, time.UTC), 1},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ttl := fixedWindowTTL(tc.nowUTC, window)
			require.Equal(t, tc.wantSec, int64(ttl/time.Second))
		})
	}
}

// TestFixedWindowTTL_AlignsToUTC_RegardlessOfLocalTimezone 显式验证：
// 即使把 now 用本地 +08 时区表达，对齐结果仍按 UTC 整天计算。
//
// 实现细节依赖 UnixNano() 的语义：UnixNano 与 Location 无关，永远是相对 epoch (UTC) 的纳秒数。
// 这条测试是 P0 修复的"时区一致性"防回归。
func TestFixedWindowTTL_AlignsToUTC_RegardlessOfLocalTimezone(t *testing.T) {
	t.Parallel()
	window := 24 * time.Hour

	// 北京时间 8:00:00 = UTC 0:00:00 → 距离下一个 UTC 0 点剩 24h
	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	now := time.Date(2026, 4, 27, 8, 0, 0, 0, loc)
	ttl := fixedWindowTTL(now, window)
	require.Equal(t, int64(86400), int64(ttl/time.Second),
		"UTC 整天对齐与本地时区无关；now(local +08)=%v 对应的 UnixNano=%d", now, now.UnixNano())

	// 北京时间 16:00:00 = UTC 8:00:00 → 距离下一个 UTC 0 点剩 16h
	now2 := time.Date(2026, 4, 27, 16, 0, 0, 0, loc)
	ttl2 := fixedWindowTTL(now2, window)
	require.Equal(t, int64(16*3600), int64(ttl2/time.Second))
}

// TestFixedWindowTTL_BoundaryCases 覆盖容错分支。
func TestFixedWindowTTL_BoundaryCases(t *testing.T) {
	t.Parallel()

	// window <= 0 时降级返回 1s 防止写出 0/负 TTL（IncrByFloat 之后 ExpireNX 失败的次坏选择）。
	require.Equal(t, time.Second, fixedWindowTTL(time.Now(), 0))
	require.Equal(t, time.Second, fixedWindowTTL(time.Now(), -time.Minute))
}
