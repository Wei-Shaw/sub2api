//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// 快照盲区：5h 重置已过期、只剩 weekly（3 天后）时，响应式冷却必须截断到 2h。
func TestCNProviderQuotaSnapshotReset_CapsBlindSpotAtTwoHours(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	account := &Account{
		Platform:    PlatformZhipu,
		Credentials: map[string]any{"account_mode": AccountModeCoding},
		Extra: map[string]any{
			cnExtraKey(PlatformZhipu, cnExtraSuffix5hReset):     now.Add(-time.Hour).Format(time.RFC3339),
			cnExtraKey(PlatformZhipu, cnExtraSuffixWeeklyReset): now.Add(3 * 24 * time.Hour).Format(time.RFC3339),
		},
	}

	got := cnProviderQuotaSnapshotReset(account, now)

	require.NotNil(t, got)
	require.True(t, now.Add(cnQuotaSnapshotCooldownCap).Equal(*got),
		"weekly-only snapshot must be capped at %s, got %s", cnQuotaSnapshotCooldownCap, got.Sub(now))
}

// 上限内的真实窗口重置点保持原值（2h 上限只截断盲区与长周期）。
func TestCNProviderQuotaSnapshotReset_KeepsResetWithinCap(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	withinCap := now.Add(90 * time.Minute)
	account := &Account{
		Platform:    PlatformZhipu,
		Credentials: map[string]any{"account_mode": AccountModeCoding},
		Extra: map[string]any{
			cnExtraKey(PlatformZhipu, cnExtraSuffix5hReset):     withinCap.Format(time.RFC3339),
			cnExtraKey(PlatformZhipu, cnExtraSuffixWeeklyReset): now.Add(3 * 24 * time.Hour).Format(time.RFC3339),
		},
	}

	got := cnProviderQuotaSnapshotReset(account, now)

	require.NotNil(t, got)
	require.True(t, withinCap.Equal(*got))
}

// 5h 窗口真实重置点在 3 小时后：必须按真实重置点冷却，不能被 2h 上限截断，
// 否则账号会提前 1 小时回到调度、继续撞上游限流（review P2 回归）。
func TestCNProviderQuotaSnapshotReset_KeepsGenuine5hResetBeyondCap(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	fiveHourReset := now.Add(3 * time.Hour)
	account := &Account{
		Platform:    PlatformZhipu,
		Credentials: map[string]any{"account_mode": AccountModeCoding},
		Extra: map[string]any{
			cnExtraKey(PlatformZhipu, cnExtraSuffix5hReset):     fiveHourReset.Format(time.RFC3339),
			cnExtraKey(PlatformZhipu, cnExtraSuffixWeeklyReset): now.Add(3 * 24 * time.Hour).Format(time.RFC3339),
		},
	}

	got := cnProviderQuotaSnapshotReset(account, now)

	require.NotNil(t, got)
	require.True(t, fiveHourReset.Equal(*got),
		"genuine 5h reset must not be capped, got %s", got.Sub(now))
}

// 选中的是 weekly（比 5h 更早、且超过 2h）：仍按 2h 截断，避免数天停调。
func TestCNProviderQuotaSnapshotReset_CapsSelectedWeeklyBeyondCap(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	account := &Account{
		Platform:    PlatformZhipu,
		Credentials: map[string]any{"account_mode": AccountModeCoding},
		Extra: map[string]any{
			cnExtraKey(PlatformZhipu, cnExtraSuffix5hReset):     now.Add(4 * time.Hour).Format(time.RFC3339),
			cnExtraKey(PlatformZhipu, cnExtraSuffixWeeklyReset): now.Add(3 * time.Hour).Format(time.RFC3339),
		},
	}

	got := cnProviderQuotaSnapshotReset(account, now)

	require.NotNil(t, got)
	require.True(t, now.Add(cnQuotaSnapshotCooldownCap).Equal(*got),
		"weekly-selected cooldown must be capped, got %s", got.Sub(now))
}
