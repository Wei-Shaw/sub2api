//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// makeQuotaSelectorSvc 构造一个 service_quota 调度联动测试用的 GatewayService。
// 使用 load-aware 路径（LoadBatchEnabled=true + ConcurrencyService）以触发 selector 内部的
// service_quota 预过滤接入点；调用方仅传入 accounts、quotaSvc 与 ticket 即可。
func makeQuotaSelectorSvc(t *testing.T, accounts []Account, quotaSvc ServiceQuotaService) *GatewayService {
	t.Helper()
	repo := &mockAccountRepoForPlatform{
		accounts:     accounts,
		accountsByID: map[int64]*Account{},
	}
	for i := range repo.accounts {
		repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	}
	cfg := testConfig()
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	bcs := &BillingCacheService{serviceQuota: quotaSvc}
	return &GatewayService{
		accountRepo:         repo,
		cache:               &mockGatewayCacheForPlatform{},
		cfg:                 cfg,
		concurrencyService:  NewConcurrencyService(&mockConcurrencyCache{}),
		billingCacheService: bcs,
	}
}

// TestSelectAccountWithLoadAwareness_ServiceQuotaPrefilter_RemovesBlocked 验证 selector
// 真接入后命中 account-scope 的候选会被 service_quota 预过滤剔除——剩余账号正常被选中。
func TestSelectAccountWithLoadAwareness_ServiceQuotaPrefilter_RemovesBlocked(t *testing.T) {
	accounts := []Account{
		{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 5},
		{ID: 2, Platform: PlatformAnthropic, Priority: 2, Status: StatusActive, Schedulable: true, Concurrency: 5},
	}
	quotaSvc := &fakeQuotaSvcForFilter{
		blocks: map[int64]*ServiceQuotaPredictedBlock{
			1: {RuleID: 11, LimiterType: ServiceQuotaLimiterRPM, ScopeKind: ServiceQuotaPredictedBlockScopeAccount},
		},
	}
	svc := makeQuotaSelectorSvc(t, accounts, quotaSvc)

	plan := &ServiceQuotaPreCheckPlan{Rules: []*ServiceQuotaRule{{ID: 11, Enabled: true}}}
	ticket := &BillingTicket{plan: plan}
	ctx := WithBillingTicket(context.Background(), ticket)

	result, err := svc.SelectAccountWithLoadAwareness(ctx, nil, "", "claude-3-5-sonnet-20241022", nil, "", int64(0))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Account)
	// account 1 被 quota 阻塞 → 应选 account 2
	require.Equal(t, int64(2), result.Account.ID)
}

// TestSelectAccountWithLoadAwareness_ServiceQuotaPrefilter_AllBlockedReturnsErr：
// 全候选被 service_quota 阻塞 → ErrNoAvailableAccounts（与原生限流耗尽走同一 sentinel error）。
func TestSelectAccountWithLoadAwareness_ServiceQuotaPrefilter_AllBlockedReturnsErr(t *testing.T) {
	accounts := []Account{
		{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 5},
		{ID: 2, Platform: PlatformAnthropic, Priority: 2, Status: StatusActive, Schedulable: true, Concurrency: 5},
	}
	quotaSvc := &fakeQuotaSvcForFilter{
		blocks: map[int64]*ServiceQuotaPredictedBlock{
			1: {RuleID: 1, LimiterType: ServiceQuotaLimiterRPM, ScopeKind: ServiceQuotaPredictedBlockScopeAccount},
			2: {RuleID: 1, LimiterType: ServiceQuotaLimiterRPM, ScopeKind: ServiceQuotaPredictedBlockScopeAccount},
		},
	}
	svc := makeQuotaSelectorSvc(t, accounts, quotaSvc)

	plan := &ServiceQuotaPreCheckPlan{Rules: []*ServiceQuotaRule{{ID: 1, Enabled: true}}}
	ticket := &BillingTicket{plan: plan}
	ctx := WithBillingTicket(context.Background(), ticket)

	_, err := svc.SelectAccountWithLoadAwareness(ctx, nil, "", "claude-3-5-sonnet-20241022", nil, "", int64(0))
	require.ErrorIs(t, err, ErrNoAvailableAccounts)
}

// TestSelectAccountWithLoadAwareness_ServiceQuotaPrefilter_NoTicket_BehaviorUnchanged：
// ctx 无 ticket（feature off / 旧路径）→ 调度行为与现状字节级一致（不触碰 quota 过滤）。
func TestSelectAccountWithLoadAwareness_ServiceQuotaPrefilter_NoTicket_BehaviorUnchanged(t *testing.T) {
	accounts := []Account{
		{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 5},
		{ID: 2, Platform: PlatformAnthropic, Priority: 2, Status: StatusActive, Schedulable: true, Concurrency: 5},
	}
	// quotaSvc 不会被调到（ticket nil → fast-path 跳过 facade）
	quotaSvc := &fakeQuotaSvcForFilter{
		blocks: map[int64]*ServiceQuotaPredictedBlock{
			1: {RuleID: 1, LimiterType: ServiceQuotaLimiterRPM, ScopeKind: ServiceQuotaPredictedBlockScopeAccount},
		},
	}
	svc := makeQuotaSelectorSvc(t, accounts, quotaSvc)

	result, err := svc.SelectAccountWithLoadAwareness(context.Background(), nil, "", "claude-3-5-sonnet-20241022", nil, "", int64(0))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, int64(1), result.Account.ID, "ticket nil 时调度应保持现状选最高优先级账号")
}

// TestSelectAccountWithLoadAwareness_ServiceQuotaPrefilter_NilPlan_BehaviorUnchanged：
// ticket 存在但 PreCheckPlan 为 nil（service_quota 全局 disabled / 无规则匹配）→ 调度不变。
func TestSelectAccountWithLoadAwareness_ServiceQuotaPrefilter_NilPlan_BehaviorUnchanged(t *testing.T) {
	accounts := []Account{
		{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 5},
		{ID: 2, Platform: PlatformAnthropic, Priority: 2, Status: StatusActive, Schedulable: true, Concurrency: 5},
	}
	quotaSvc := &fakeQuotaSvcForFilter{
		blocks: map[int64]*ServiceQuotaPredictedBlock{
			1: {RuleID: 1, LimiterType: ServiceQuotaLimiterRPM, ScopeKind: ServiceQuotaPredictedBlockScopeAccount},
		},
	}
	svc := makeQuotaSelectorSvc(t, accounts, quotaSvc)

	ticket := &BillingTicket{} // plan == nil
	ctx := WithBillingTicket(context.Background(), ticket)

	result, err := svc.SelectAccountWithLoadAwareness(ctx, nil, "", "claude-3-5-sonnet-20241022", nil, "", int64(0))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, int64(1), result.Account.ID, "plan nil 时调度应保持现状")
}

// TestSelectAccountWithLoadAwareness_ServiceQuotaPrefilter_SnapshotError_FailOpen：
// SnapshotForAccounts 返回错误 → 调度照常（fail-open，不阻塞热路径）。
func TestSelectAccountWithLoadAwareness_ServiceQuotaPrefilter_SnapshotError_FailOpen(t *testing.T) {
	accounts := []Account{
		{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 5},
		{ID: 2, Platform: PlatformAnthropic, Priority: 2, Status: StatusActive, Schedulable: true, Concurrency: 5},
	}
	quotaSvc := &fakeQuotaSvcForFilter{err: errors.New("redis down")}
	svc := makeQuotaSelectorSvc(t, accounts, quotaSvc)

	plan := &ServiceQuotaPreCheckPlan{Rules: []*ServiceQuotaRule{{ID: 1, Enabled: true}}}
	ticket := &BillingTicket{plan: plan}
	ctx := WithBillingTicket(context.Background(), ticket)

	result, err := svc.SelectAccountWithLoadAwareness(ctx, nil, "", "claude-3-5-sonnet-20241022", nil, "", int64(0))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, int64(1), result.Account.ID, "snapshot 失败时调度回退到最高优先级账号")
}
