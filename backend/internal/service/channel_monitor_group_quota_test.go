//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

// --- 组级聚合的 stub ---

type stubMonitorGroupAccountSource struct {
	accounts map[int64][]Account
	err      error
	calls    int
}

func (s *stubMonitorGroupAccountSource) ListByGroup(_ context.Context, groupID int64) ([]Account, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.accounts[groupID], nil
}

// perAccountUsageSource 按账号 ID 返回不同用量，用于验证聚合口径。
type perAccountUsageSource struct {
	byAccount map[int64]*UsageInfo
	errFor    map[int64]error
}

func (s *perAccountUsageSource) GetUsageForAccount(_ context.Context, account *Account, _ ...bool) (*UsageInfo, error) {
	if err, ok := s.errFor[account.ID]; ok {
		return nil, err
	}
	return s.byAccount[account.ID], nil
}

// kiroUsageWithBonus 构造同时带 credits 和 bonus 的 kiro 用量。
func kiroUsageWithBonus(creditUsed, creditLimit, bonusUsed, bonusLimit float64) *UsageInfo {
	usage := kiroUsage(creditUsed, creditLimit)
	if bonusLimit > 0 {
		usage.KiroBonus = &KiroCreditProgress{
			CurrentUsage:   bonusUsed,
			UsageLimit:     bonusLimit,
			PercentageUsed: bonusUsed / bonusLimit * 100,
		}
	}
	return usage
}

// kiroUsage 构造一份 kiro 用量：credits 已用 used / 上限 limit。
func kiroUsage(used, limit float64) *UsageInfo {
	percent := 0.0
	if limit > 0 {
		percent = used / limit * 100
	}
	return &UsageInfo{
		KiroCredit: &KiroCreditProgress{
			CurrentUsage:   used,
			UsageLimit:     limit,
			PercentageUsed: percent,
		},
	}
}

func newGroupQuotaFetcher(t *testing.T, groupAccounts []Account, usage *perAccountUsageSource) *ChannelMonitorQuotaFetcher {
	t.Helper()
	accountsByID := make(map[int64]*Account, len(groupAccounts))
	for i := range groupAccounts {
		acc := groupAccounts[i]
		accountsByID[acc.ID] = &acc
	}
	return &ChannelMonitorQuotaFetcher{
		usage:            usage,
		accounts:         &stubMonitorAccountSource{accounts: accountsByID},
		groupAccounts:    &stubMonitorGroupAccountSource{accounts: map[int64][]Account{9: groupAccounts}},
		balanceThreshold: monitorBalanceThreshold(nil),
		cache:            make(map[int64]monitorQuotaCacheEntry),
	}
}

func kiroGroupAccounts(ids ...int64) []Account {
	accounts := make([]Account, 0, len(ids))
	for _, id := range ids {
		accounts = append(accounts, Account{ID: id, Platform: domain.PlatformKiro})
	}
	return accounts
}

// --- Phase 0：kiro 额度必须被映射成 tier ---
//
// 回归 bug：usageQuotaTiers 曾经没有 kiro 分支，导致 Tiers 为空、
// quotaDegradedHint 无从告警，额度耗尽的 kiro 账号也恒判 operational。

func TestUsageQuotaTiers_MapsKiroCreditsAndBonus(t *testing.T) {
	resetAt := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	usage := &UsageInfo{
		KiroResetAt: &resetAt,
		KiroCredit:  &KiroCreditProgress{CurrentUsage: 125, UsageLimit: 2000, PercentageUsed: 6.25},
		KiroBonus:   &KiroCreditProgress{CurrentUsage: 25, UsageLimit: 500, PercentageUsed: 5},
	}

	tiers := usageQuotaTiers(usage)

	require.Len(t, tiers, 2)
	require.Equal(t, "total", tiers[0].Window)
	require.Equal(t, "credits", tiers[0].Label)
	require.InDelta(t, 6.25, tiers[0].UsedPercent, 0.001)
	require.Equal(t, float64(125), tiers[0].Used)
	require.Equal(t, float64(2000), tiers[0].Limit)
	require.Equal(t, "2026-09-01T00:00:00Z", tiers[0].ResetAt)
	require.Equal(t, "bonus", tiers[1].Label)
}

// 上游没给额度上限时使用率无从计算，跳过而不是产出 0% 的假 tier。
func TestUsageQuotaTiers_SkipsKiroCreditsWithoutLimit(t *testing.T) {
	require.Nil(t, usageQuotaTiers(&UsageInfo{
		KiroCredit: &KiroCreditProgress{CurrentUsage: 10, UsageLimit: 0},
	}))
}

// 耗尽的 kiro 账号现在能被 quotaDegradedHint 认出来（此前恒为 operational）。
func TestQuotaFetcher_ExhaustedKiroAccountIsNotOperational(t *testing.T) {
	fetcher := newGroupQuotaFetcher(t, kiroGroupAccounts(1), &perAccountUsageSource{
		byAccount: map[int64]*UsageInfo{1: kiroUsage(2000, 2000)},
	})

	snapshot := fetcher.Fetch(context.Background(), 1)
	require.True(t, snapshot.Success)
	require.Len(t, snapshot.Tiers, 1)

	res := deriveQuotaCheckResult(snapshot, "quota", time.Now())
	require.Equal(t, MonitorStatusDegraded, res.Status)
	require.Contains(t, res.Message, "quota high")
}

// --- FetchGroup 聚合 ---

func TestFetchGroup_AggregatesCountsAndSumsTiers(t *testing.T) {
	fetcher := newGroupQuotaFetcher(t, kiroGroupAccounts(1, 2, 3), &perAccountUsageSource{
		byAccount: map[int64]*UsageInfo{
			1: kiroUsage(500, 1000),  // 健康
			2: kiroUsage(1000, 1000), // 耗尽
			3: kiroUsage(0, 1000),    // 健康
		},
	})

	snapshot := fetcher.FetchGroup(context.Background(), 9)

	require.True(t, snapshot.Success)
	require.Equal(t, 3, snapshot.AccountsTotal)
	require.Equal(t, 2, snapshot.AccountsHealthy)
	require.Equal(t, 1, snapshot.AccountsExhausted)
	// 三个账号都给了绝对值，按 sum(used)/sum(limit) 算真实占用率：1500/3000。
	require.Len(t, snapshot.Tiers, 1)
	require.Equal(t, "credits", snapshot.Tiers[0].Label)
	require.Equal(t, float64(1500), snapshot.Tiers[0].Used)
	require.Equal(t, float64(3000), snapshot.Tiers[0].Limit)
	require.InDelta(t, 50, snapshot.Tiers[0].UsedPercent, 0.001)
}

// credits 打满但 bonus 还有余的号必须算健康，否则组里「还有余额」会被误判耗尽。
func TestFetchGroup_KiroBonusRemainingIsHealthy(t *testing.T) {
	fetcher := newGroupQuotaFetcher(t, kiroGroupAccounts(1, 2), &perAccountUsageSource{
		byAccount: map[int64]*UsageInfo{
			1: kiroUsageWithBonus(500, 500, 20, 200),
			2: kiroUsage(0, 500),
		},
	})

	snapshot := fetcher.FetchGroup(context.Background(), 9)
	require.Equal(t, 2, snapshot.AccountsTotal)
	require.Equal(t, 2, snapshot.AccountsHealthy)
	require.Equal(t, 0, snapshot.AccountsExhausted)
	require.Equal(t, MonitorStatusOperational, deriveQuotaCheckResult(snapshot, "quota", time.Now()).Status)
}

// 部分号耗尽时聚合占用率会被空号拉高，不能再拿 90% 阈值把整渠打成降级。
func TestDeriveGroupQuotaStatus_PartialExhaustionIgnoresBlendedHighUsage(t *testing.T) {
	status, _ := deriveGroupQuotaStatus(&domain.MonitorQuotaSnapshot{
		Success:           true,
		AccountsTotal:     10,
		AccountsHealthy:   2,
		AccountsExhausted: 8,
		Tiers: []domain.MonitorQuotaTier{
			{Window: "total", Label: "credits", UsedPercent: 96},
		},
	})
	require.Equal(t, MonitorStatusOperational, status)
}

// 抓取失败的账号计为「未知」而非「耗尽」：不能把查不到当成没额度。
func TestFetchGroup_FetchFailuresCountAsUnknown(t *testing.T) {
	fetcher := newGroupQuotaFetcher(t, kiroGroupAccounts(1, 2), &perAccountUsageSource{
		byAccount: map[int64]*UsageInfo{1: kiroUsage(100, 1000)},
		errFor:    map[int64]error{2: errors.New("upstream 500")},
	})

	snapshot := fetcher.FetchGroup(context.Background(), 9)

	require.Equal(t, 2, snapshot.AccountsTotal)
	require.Equal(t, 1, snapshot.AccountsHealthy)
	require.Equal(t, 0, snapshot.AccountsExhausted)

	res := deriveQuotaCheckResult(snapshot, "quota", time.Now())
	require.Equal(t, MonitorStatusOperational, res.Status)
}

// 凭据失效的号确实用不了，归入耗尽而不是未知。
func TestFetchGroup_CredentialInvalidCountsAsExhausted(t *testing.T) {
	fetcher := newGroupQuotaFetcher(t, kiroGroupAccounts(1), &perAccountUsageSource{
		errFor: map[int64]error{1: errors.New("401 unauthorized")},
	})

	snapshot := fetcher.FetchGroup(context.Background(), 9)

	require.Equal(t, 1, snapshot.AccountsTotal)
	require.Equal(t, 0, snapshot.AccountsHealthy)
	require.Equal(t, 1, snapshot.AccountsExhausted)
	require.Equal(t, MonitorStatusFailed, deriveQuotaCheckResult(snapshot, "quota", time.Now()).Status)
}

// 空分组 / 列取失败都是配置类问题，推导 degraded 而不是渠道故障。
func TestFetchGroup_EmptyGroupAndListErrorAreConfigProblems(t *testing.T) {
	empty := newGroupQuotaFetcher(t, nil, &perAccountUsageSource{})
	snapshot := empty.FetchGroup(context.Background(), 9)
	require.False(t, snapshot.Success)
	require.Contains(t, snapshot.Error, monitorQuotaErrGroupNoAccounts)
	require.Equal(t, MonitorStatusDegraded, deriveQuotaCheckResult(snapshot, "quota", time.Now()).Status)

	broken := newGroupQuotaFetcher(t, kiroGroupAccounts(1), &perAccountUsageSource{})
	broken.groupAccounts = &stubMonitorGroupAccountSource{err: errors.New("db down")}
	snapshot = broken.FetchGroup(context.Background(), 9)
	require.False(t, snapshot.Success)
	require.Contains(t, snapshot.Error, monitorQuotaErrGroupNotFound)
	require.Equal(t, MonitorStatusDegraded, deriveQuotaCheckResult(snapshot, "quota", time.Now()).Status)
}

func TestFetchGroup_NilFetcherAndMissingSourceDegradeGracefully(t *testing.T) {
	var nilFetcher *ChannelMonitorQuotaFetcher
	require.False(t, nilFetcher.FetchGroup(context.Background(), 9).Success)

	noSource := &ChannelMonitorQuotaFetcher{cache: make(map[int64]monitorQuotaCacheEntry)}
	require.False(t, noSource.FetchGroup(context.Background(), 9).Success)
}

// --- 状态推导：六条聚合分支 ---

func TestDeriveGroupQuotaStatus_Branches(t *testing.T) {
	cases := []struct {
		name          string
		total         int
		healthy       int
		exhausted     int
		snapshotError string
		wantStatus    string
		wantMessage   string
	}{
		{
			name: "全部耗尽记为 failed", total: 3, healthy: 0, exhausted: 3,
			wantStatus: MonitorStatusFailed, wantMessage: "3/3 accounts exhausted",
		},
		{
			name: "无健康号但有耗尽号也记为 failed", total: 3, healthy: 0, exhausted: 1,
			wantStatus: MonitorStatusFailed, wantMessage: "1/3 accounts exhausted",
		},
		{
			// 关键分支：冷启动首轮可能全部超预算，那是抓取问题而非额度耗尽，
			// 报 failed 会白白污染可用率曲线。
			name: "全部未知记为 error 而非 failed", total: 3, healthy: 0, exhausted: 0,
			snapshotError: "context canceled",
			wantStatus:    MonitorStatusError, wantMessage: "context canceled",
		},
		{
			name: "全部未知且无错误摘要时给出兜底文案", total: 2, healthy: 0, exhausted: 0,
			wantStatus: MonitorStatusError, wantMessage: "quota unavailable for all 2 accounts",
		},
		{
			name: "部分耗尽但还有健康号记为 operational", total: 5, healthy: 3, exhausted: 2,
			wantStatus: MonitorStatusOperational,
		},
		{
			name: "部分未知但还有健康号记为 operational", total: 4, healthy: 1, exhausted: 0,
			wantStatus: MonitorStatusOperational,
		},
		{
			name: "全部健康记为 operational", total: 2, healthy: 2, exhausted: 0,
			wantStatus: MonitorStatusOperational,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := &domain.MonitorQuotaSnapshot{
				Success:           true,
				AccountsTotal:     tc.total,
				AccountsHealthy:   tc.healthy,
				AccountsExhausted: tc.exhausted,
				Error:             tc.snapshotError,
			}
			status, message := deriveGroupQuotaStatus(snapshot)
			require.Equal(t, tc.wantStatus, status)
			if tc.wantMessage == "" {
				require.Empty(t, message)
			} else {
				require.Contains(t, message, tc.wantMessage)
			}
		})
	}
}

// 全员健康时仍要沿用单账号的 90% 阈值，否则「组里每个号都 95%」会被判正常。
func TestDeriveGroupQuotaStatus_AllHealthyStillHonoursHighUsage(t *testing.T) {
	status, message := deriveGroupQuotaStatus(&domain.MonitorQuotaSnapshot{
		Success:         true,
		AccountsTotal:   2,
		AccountsHealthy: 2,
		Tiers: []domain.MonitorQuotaTier{
			{Window: "total", Label: "credits", UsedPercent: 95},
		},
	})
	require.Equal(t, MonitorStatusDegraded, status)
	require.Contains(t, message, "quota high")
}

// 聚合快照必须走计数口径，不能被单账号分支（!Success → error）截胡。
func TestDeriveQuotaCheckResult_AggregateTakesPrecedenceOverSuccessFlag(t *testing.T) {
	res := deriveQuotaCheckResult(&domain.MonitorQuotaSnapshot{
		Success:           false,
		Error:             "partial failure",
		AccountsTotal:     3,
		AccountsHealthy:   2,
		AccountsExhausted: 1,
	}, "quota", time.Now())
	require.Equal(t, MonitorStatusOperational, res.Status)
}

// --- 单账号耗尽判定 ---

func TestMonitorAccountQuotaExhausted(t *testing.T) {
	require.False(t, monitorAccountQuotaExhausted(nil))
	// 抓取失败 ≠ 耗尽（未知）。
	require.False(t, monitorAccountQuotaExhausted(&domain.MonitorQuotaSnapshot{Success: false}))
	require.True(t, monitorAccountQuotaExhausted(&domain.MonitorQuotaSnapshot{Success: true, CredentialInvalid: true}))
	require.True(t, monitorAccountQuotaExhausted(&domain.MonitorQuotaSnapshot{Success: true, BalanceLow: true}))
	require.True(t, monitorAccountQuotaExhausted(&domain.MonitorQuotaSnapshot{
		Success: true,
		Tiers:   []domain.MonitorQuotaTier{{Used: 1000, Limit: 1000}},
	}))
	require.True(t, monitorAccountQuotaExhausted(&domain.MonitorQuotaSnapshot{
		Success: true,
		Tiers:   []domain.MonitorQuotaTier{{UsedPercent: 100}},
	}))
	// 99% 是「快满了」而不是「用不了了」，仍算健康（由 90% 阈值判 degraded）。
	require.False(t, monitorAccountQuotaExhausted(&domain.MonitorQuotaSnapshot{
		Success: true,
		Tiers:   []domain.MonitorQuotaTier{{Used: 990, Limit: 1000, UsedPercent: 99}},
	}))
	// kiro credits 打满但 bonus 还有余：这个号还能打，不能整号算耗尽。
	require.False(t, monitorAccountQuotaExhausted(&domain.MonitorQuotaSnapshot{
		Success: true,
		Tiers: []domain.MonitorQuotaTier{
			{Window: "total", Label: "credits", Used: 500, Limit: 500, UsedPercent: 100},
			{Window: "total", Label: "bonus", Used: 10, Limit: 200, UsedPercent: 5},
		},
	}))
	require.True(t, monitorAccountQuotaExhausted(&domain.MonitorQuotaSnapshot{
		Success: true,
		Tiers: []domain.MonitorQuotaTier{
			{Window: "total", Label: "credits", Used: 500, Limit: 500, UsedPercent: 100},
			{Window: "total", Label: "bonus", Used: 200, Limit: 200, UsedPercent: 100},
		},
	}))
	// claude 5h 打满就发不出去，即使 7d 还有余。
	require.True(t, monitorAccountQuotaExhausted(&domain.MonitorQuotaSnapshot{
		Success: true,
		Tiers: []domain.MonitorQuotaTier{
			{Window: "5h", UsedPercent: 100},
			{Window: "7d", UsedPercent: 40},
		},
	}))
}

// --- 数据源绑定：账号与分组二选一 ---

func TestValidateCreateParams_QuotaTargetBinding(t *testing.T) {
	accountID := int64(9)
	groupID := int64(11)
	base := func() ChannelMonitorCreateParams {
		return ChannelMonitorCreateParams{
			Provider:        MonitorProviderKiro,
			CheckMode:       MonitorCheckModeQuota,
			IntervalSeconds: 60,
		}
	}

	// 绑定分组同样满足配额模式的数据源要求。
	withGroup := base()
	withGroup.GroupID = &groupID
	require.NoError(t, validateCreateParams(withGroup))

	// 两个都绑：数据源歧义。
	conflict := base()
	conflict.AccountID = &accountID
	conflict.GroupID = &groupID
	require.ErrorIs(t, validateCreateParams(conflict), ErrChannelMonitorQuotaTargetConflict)

	// 一个都不绑。
	require.ErrorIs(t, validateCreateParams(base()), ErrChannelMonitorAccountRequired)
}

func TestValidateMonitorModeFields_QuotaTargetBinding(t *testing.T) {
	accountID := int64(9)
	groupID := int64(11)

	require.NoError(t, validateMonitorModeFields(&ChannelMonitor{
		Provider: MonitorProviderKiro, CheckMode: MonitorCheckModeQuota, GroupID: &groupID,
	}))
	require.ErrorIs(t, validateMonitorModeFields(&ChannelMonitor{
		Provider: MonitorProviderKiro, CheckMode: MonitorCheckModeQuota,
		AccountID: &accountID, GroupID: &groupID,
	}), ErrChannelMonitorQuotaTargetConflict)
	require.ErrorIs(t, validateMonitorModeFields(&ChannelMonitor{
		Provider: MonitorProviderKiro, CheckMode: MonitorCheckModeQuota,
	}), ErrChannelMonitorAccountRequired)
}

// 换绑数据源时必须清掉另一侧，否则存量单账号监控改成分组后会留着旧 account_id
// （库层 CHECK 会直接拒绝写入）。
func TestApplyMonitorUpdate_QuotaTargetsAreMutuallyExclusive(t *testing.T) {
	accountID := int64(9)
	groupID := int64(11)
	zero := int64(0)

	toGroup := &ChannelMonitor{
		Provider: MonitorProviderKiro, CheckMode: MonitorCheckModeQuota,
		Endpoint: "https://example.com", AccountID: &accountID,
	}
	require.NoError(t, applyMonitorUpdate(toGroup, ChannelMonitorUpdateParams{GroupID: &groupID}))
	require.Nil(t, toGroup.AccountID)
	require.Equal(t, groupID, *toGroup.GroupID)

	toAccount := &ChannelMonitor{
		Provider: MonitorProviderKiro, CheckMode: MonitorCheckModeQuota,
		Endpoint: "https://example.com", GroupID: &groupID,
	}
	require.NoError(t, applyMonitorUpdate(toAccount, ChannelMonitorUpdateParams{AccountID: &accountID}))
	require.Nil(t, toAccount.GroupID)
	require.Equal(t, accountID, *toAccount.AccountID)

	// 0 = 解绑本项；配额模式下解绑到无数据源要报错。
	unbind := &ChannelMonitor{
		Provider: MonitorProviderKiro, CheckMode: MonitorCheckModeQuota,
		Endpoint: "https://example.com", GroupID: &groupID,
	}
	require.ErrorIs(t,
		applyMonitorUpdate(unbind, ChannelMonitorUpdateParams{GroupID: &zero}),
		ErrChannelMonitorAccountRequired,
	)
}

// 只有部分账号给了绝对值时退化为百分比均值，而不是拿残缺的绝对值算出假占用率。
func TestFlattenGroupTiers_FallsBackToPercentAverage(t *testing.T) {
	accumulators := map[groupTierKey]*groupTierAccumulator{}
	order := []groupTierKey{}
	accumulateGroupTiers(accumulators, &order, []domain.MonitorQuotaTier{
		{Window: "total", Label: "credits", UsedPercent: 80, Used: 800, Limit: 1000},
	})
	accumulateGroupTiers(accumulators, &order, []domain.MonitorQuotaTier{
		{Window: "total", Label: "credits", UsedPercent: 20},
	})

	tiers := flattenGroupTiers(accumulators, order)
	require.Len(t, tiers, 1)
	require.InDelta(t, 50, tiers[0].UsedPercent, 0.001)
	require.Zero(t, tiers[0].Used)
	require.Zero(t, tiers[0].Limit)
}
