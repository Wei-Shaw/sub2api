//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// tokenResetNoopRepo 让 CheckAndResetWindows 的 Reset* 调用不触碰 DB，
// 仅用于验证服务层在重置后会清零内存中的 token 用量。
type tokenResetNoopRepo struct {
	userSubRepoNoop
}

func (tokenResetNoopRepo) ResetDailyUsage(context.Context, int64, *time.Time, time.Time) error {
	return nil
}
func (tokenResetNoopRepo) ResetWeeklyUsage(context.Context, int64, *time.Time, time.Time) error {
	return nil
}
func (tokenResetNoopRepo) ResetMonthlyUsage(context.Context, int64, *time.Time, time.Time) error {
	return nil
}

// TestNormalizeExpiredWindows_ZeroesTokenUsage 校正展示层在窗口过期时
// 必须把 token 用量一并清零，避免前端显示历史 token 用量。
func TestNormalizeExpiredWindows_ZeroesTokenUsage(t *testing.T) {
	now := time.Now()
	expiredDaily := now.Add(-25 * time.Hour)
	expiredWeekly := now.Add(-8 * 24 * time.Hour)
	expiredMonthly := now.Add(-31 * 24 * time.Hour)
	startsAt := now.AddDate(0, 0, -60) // 多日订阅，避免被识别为一次性日卡

	subs := []UserSubscription{{
		ID:                 1,
		StartsAt:           startsAt,
		ExpiresAt:          startsAt.AddDate(0, 0, 90),
		DailyWindowStart:   &expiredDaily,
		WeeklyWindowStart:  &expiredWeekly,
		MonthlyWindowStart: &expiredMonthly,
		DailyUsageUSD:      10,
		WeeklyUsageUSD:     20,
		MonthlyUsageUSD:    30,
		DailyUsageTokens:   100,
		WeeklyUsageTokens:  200,
		MonthlyUsageTokens: 300,
	}}

	normalizeExpiredWindows(subs)

	require.Nil(t, subs[0].DailyWindowStart, "expired daily window start should be cleared")
	require.Equal(t, int64(0), subs[0].DailyUsageTokens, "daily token usage should be zeroed")
	require.Equal(t, int64(0), subs[0].WeeklyUsageTokens, "weekly token usage should be zeroed")
	require.Equal(t, int64(0), subs[0].MonthlyUsageTokens, "monthly token usage should be zeroed")
}

// TestCheckAndResetWindows_ZeroesTokenUsage 校正窗口自动重置后，
// 内存中的 token 用量必须与 USD 用量一起清零。
func TestCheckAndResetWindows_ZeroesTokenUsage(t *testing.T) {
	now := time.Now()
	startsAt := now.AddDate(0, 0, -60) // 多日订阅，避免被识别为一次性日卡
	expiredDaily := now.Add(-25 * time.Hour)
	expiredWeekly := now.Add(-8 * 24 * time.Hour)
	expiredMonthly := now.Add(-31 * 24 * time.Hour)

	repo := &tokenResetNoopRepo{}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	sub := &UserSubscription{
		ID:                 1,
		UserID:             10,
		GroupID:            20,
		StartsAt:           startsAt,
		ExpiresAt:          startsAt.AddDate(0, 0, 90),
		DailyWindowStart:   &expiredDaily,
		WeeklyWindowStart:  &expiredWeekly,
		MonthlyWindowStart: &expiredMonthly,
		DailyUsageUSD:      10,
		WeeklyUsageUSD:     20,
		MonthlyUsageUSD:    30,
		DailyUsageTokens:   100,
		WeeklyUsageTokens:  200,
		MonthlyUsageTokens: 300,
	}

	err := svc.CheckAndResetWindows(context.Background(), sub)

	require.NoError(t, err)
	require.Equal(t, 0.0, sub.DailyUsageUSD, "daily USD usage should be zeroed")
	require.Equal(t, int64(0), sub.DailyUsageTokens, "daily token usage should be zeroed")
	require.Equal(t, int64(0), sub.WeeklyUsageTokens, "weekly token usage should be zeroed")
	require.Equal(t, int64(0), sub.MonthlyUsageTokens, "monthly token usage should be zeroed")
}

// TestRenewedSubscriptionTerm_ZeroesTokenUsage 续费（已过期订阅开启新周期）必须把
// token 用量连同 USD 用量一起清零，否则上一周期累计的 token 会让用户续费后立即触顶。
func TestRenewedSubscriptionTerm_ZeroesTokenUsage(t *testing.T) {
	now := time.Now()
	existing := &UserSubscription{
		StartsAt:           now.AddDate(0, 0, -60),
		ExpiresAt:          now.AddDate(0, 0, -1),
		DailyUsageUSD:      10,
		WeeklyUsageUSD:     20,
		MonthlyUsageUSD:    30,
		DailyUsageTokens:   100,
		WeeklyUsageTokens:  200,
		MonthlyUsageTokens: 300,
	}

	renewed := renewedSubscriptionTerm(existing, "续费", now, now.AddDate(0, 0, 30))

	require.Equal(t, 0.0, renewed.DailyUsageUSD)
	require.Equal(t, int64(0), renewed.DailyUsageTokens, "daily token usage should be zeroed on renew")
	require.Equal(t, int64(0), renewed.WeeklyUsageTokens, "weekly token usage should be zeroed on renew")
	require.Equal(t, int64(0), renewed.MonthlyUsageTokens, "monthly token usage should be zeroed on renew")
}
