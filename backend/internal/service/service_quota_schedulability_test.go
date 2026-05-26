//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// rulesByPathScope 是 SnapshotForAccounts 系列测试的小型工厂集合，构造常见的
// (account / account+model / account+channel / channel-only / group-only) path 组合。
//
// 集中放在测试 file 顶端避免每个测试局部 inline 重复结构体字面量，符合
// CLAUDE.md「测试 fixture 也要复用」原则。
func ruleAccountRPM(id, accountID int64, limit float64) *ServiceQuotaRule {
	acc := accountID
	return &ServiceQuotaRule{
		ID:          id,
		Enabled:     true,
		CounterMode: ServiceQuotaCounterModeShared,
		Limiters: []ServiceQuotaLimiterDef{
			{ID: id*100 + 1, RuleID: id, LimiterType: ServiceQuotaLimiterRPM, WindowMode: ServiceQuotaWindowFixed, LimitValue: limit, CountOnArrival: false},
		},
		Paths: []ServiceQuotaPathDef{
			{ID: id*100 + 11, RuleID: id, AccountID: &acc},
		},
	}
}

func ruleAccountModelRPM(id, accountID int64, modelPattern string, limit float64) *ServiceQuotaRule {
	acc := accountID
	mp := modelPattern
	return &ServiceQuotaRule{
		ID:          id,
		Enabled:     true,
		CounterMode: ServiceQuotaCounterModeShared,
		Limiters: []ServiceQuotaLimiterDef{
			{ID: id*100 + 1, RuleID: id, LimiterType: ServiceQuotaLimiterRPM, WindowMode: ServiceQuotaWindowFixed, LimitValue: limit, CountOnArrival: false},
		},
		Paths: []ServiceQuotaPathDef{
			{ID: id*100 + 11, RuleID: id, AccountID: &acc, ModelPattern: &mp},
		},
	}
}

func ruleAccountChannelRPM(id, accountID, channelID int64, limit float64) *ServiceQuotaRule {
	acc := accountID
	ch := channelID
	return &ServiceQuotaRule{
		ID:          id,
		Enabled:     true,
		CounterMode: ServiceQuotaCounterModeShared,
		Limiters: []ServiceQuotaLimiterDef{
			{ID: id*100 + 1, RuleID: id, LimiterType: ServiceQuotaLimiterRPM, WindowMode: ServiceQuotaWindowFixed, LimitValue: limit, CountOnArrival: false},
		},
		Paths: []ServiceQuotaPathDef{
			{ID: id*100 + 11, RuleID: id, AccountID: &acc, ChannelID: &ch},
		},
	}
}

func ruleChannelOnlyRPM(id, channelID int64, limit float64) *ServiceQuotaRule {
	ch := channelID
	return &ServiceQuotaRule{
		ID:          id,
		Enabled:     true,
		CounterMode: ServiceQuotaCounterModeShared,
		Limiters: []ServiceQuotaLimiterDef{
			{ID: id*100 + 1, RuleID: id, LimiterType: ServiceQuotaLimiterRPM, WindowMode: ServiceQuotaWindowFixed, LimitValue: limit, CountOnArrival: false},
		},
		Paths: []ServiceQuotaPathDef{
			{ID: id*100 + 11, RuleID: id, ChannelID: &ch},
		},
	}
}

func ruleGroupOnlyRPM(id, groupID int64, limit float64) *ServiceQuotaRule {
	g := groupID
	return &ServiceQuotaRule{
		ID:          id,
		Enabled:     true,
		CounterMode: ServiceQuotaCounterModeShared,
		Limiters: []ServiceQuotaLimiterDef{
			{ID: id*100 + 1, RuleID: id, LimiterType: ServiceQuotaLimiterRPM, WindowMode: ServiceQuotaWindowFixed, LimitValue: limit, CountOnArrival: false},
		},
		Paths: []ServiceQuotaPathDef{
			{ID: id*100 + 11, RuleID: id, GroupID: &g},
		},
	}
}

func makePlanForRules(req ServiceQuotaCheckRequest, rules []*ServiceQuotaRule) *ServiceQuotaPreCheckPlan {
	return &ServiceQuotaPreCheckPlan{Rules: rules, Req: req}
}

func newQuotaServiceForSchedTest(t *testing.T, rules []*ServiceQuotaRule, limiter *fakeServiceQuotaLimiter, enabled bool) *serviceQuotaService {
	t.Helper()
	repo := &fakeServiceQuotaRepo{rules: rules}
	settingRepo := newMockSettingRepo()
	if enabled {
		settingRepo.data[SettingKeyServiceQuotaEnabled] = "true"
	}
	settings := NewSettingService(settingRepo, &config.Config{})
	return &serviceQuotaService{
		repo:     repo,
		settings: settings,
		limiter:  limiter,
	}
}

// TestSnapshotForAccounts_FeatureDisabled_ReturnsNil 覆盖 short-circuit 三层之一：feature 关闭。
func TestSnapshotForAccounts_FeatureDisabled_ReturnsNil(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()
	svc := newQuotaServiceForSchedTest(t, []*ServiceQuotaRule{ruleAccountRPM(1, 7, 5)}, limiter, false)

	plan := makePlanForRules(ServiceQuotaCheckRequest{UserID: 1, Model: "claude"}, []*ServiceQuotaRule{ruleAccountRPM(1, 7, 5)})
	got, err := svc.SnapshotForAccounts(context.Background(), ServiceQuotaCheckRequest{UserID: 1, Model: "claude"}, plan, []int64{7})
	require.NoError(t, err)
	require.Nil(t, got)
	require.Empty(t, limiter.snapshotManyCalls)
}

// TestSnapshotForAccounts_NilPlan_ReturnsNil 覆盖 short-circuit 第二层：plan 为 nil。
func TestSnapshotForAccounts_NilPlan_ReturnsNil(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()
	svc := newQuotaServiceForSchedTest(t, nil, limiter, true)
	got, err := svc.SnapshotForAccounts(context.Background(), ServiceQuotaCheckRequest{UserID: 1}, nil, []int64{7})
	require.NoError(t, err)
	require.Nil(t, got)
	require.Empty(t, limiter.snapshotManyCalls)
}

// TestSnapshotForAccounts_EmptyCandidates_ReturnsNil 覆盖 short-circuit 第三层：候选为空。
func TestSnapshotForAccounts_EmptyCandidates_ReturnsNil(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()
	rule := ruleAccountRPM(1, 7, 5)
	svc := newQuotaServiceForSchedTest(t, []*ServiceQuotaRule{rule}, limiter, true)
	plan := makePlanForRules(ServiceQuotaCheckRequest{UserID: 1}, []*ServiceQuotaRule{rule})
	got, err := svc.SnapshotForAccounts(context.Background(), ServiceQuotaCheckRequest{UserID: 1}, plan, nil)
	require.NoError(t, err)
	require.Nil(t, got)
	require.Empty(t, limiter.snapshotManyCalls)
}

// TestSnapshotForAccounts_AccountOnly_BlocksAccount：path 仅含 account_id，已用 >= limit → 命中。
func TestSnapshotForAccounts_AccountOnly_BlocksAccount(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()
	rule := ruleAccountRPM(1, 7, 5)
	svc := newQuotaServiceForSchedTest(t, []*ServiceQuotaRule{rule}, limiter, true)

	// 预填 counter 使 account 7 已耗尽
	req := ServiceQuotaCheckRequest{UserID: 1, AccountID: 7, Model: "claude-3-7"}
	key := svc.counterKey(req, rule, rule.Paths[0], rule.Limiters[0])
	limiter.counters[key] = 5

	plan := makePlanForRules(ServiceQuotaCheckRequest{UserID: 1, Model: "claude-3-7"}, []*ServiceQuotaRule{rule})
	got, err := svc.SnapshotForAccounts(context.Background(), ServiceQuotaCheckRequest{UserID: 1, Model: "claude-3-7"}, plan, []int64{7, 8})
	require.NoError(t, err)
	require.Len(t, got, 1)
	block := got[7]
	require.NotNil(t, block)
	require.Equal(t, ServiceQuotaPredictedBlockScopeAccount, block.ScopeKind)
	require.Equal(t, int64(1), block.RuleID)
	require.Equal(t, ServiceQuotaLimiterRPM, block.LimiterType)
	require.Equal(t, float64(5), block.Used)
	require.Equal(t, float64(5), block.Limit)
}

// TestSnapshotForAccounts_AccountAndModelPattern_BlocksOnlyMatchingAccount：
// path = account+model_pattern；req.Model 匹配时命中、不匹配时不命中。
func TestSnapshotForAccounts_AccountAndModelPattern_BlocksOnlyMatchingAccount(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()
	rule := ruleAccountModelRPM(1, 7, "claude-3-7-*", 5)
	svc := newQuotaServiceForSchedTest(t, []*ServiceQuotaRule{rule}, limiter, true)

	matchReq := ServiceQuotaCheckRequest{UserID: 1, AccountID: 7, Model: "claude-3-7-sonnet"}
	matchKey := svc.counterKey(matchReq, rule, rule.Paths[0], rule.Limiters[0])
	limiter.counters[matchKey] = 9

	t.Run("model matches → blocked", func(t *testing.T) {
		base := ServiceQuotaCheckRequest{UserID: 1, Model: "claude-3-7-sonnet"}
		plan := makePlanForRules(base, []*ServiceQuotaRule{rule})
		got, err := svc.SnapshotForAccounts(context.Background(), base, plan, []int64{7})
		require.NoError(t, err)
		require.NotNil(t, got[7])
	})

	t.Run("model does not match → not blocked", func(t *testing.T) {
		base := ServiceQuotaCheckRequest{UserID: 1, Model: "gpt-5"}
		plan := makePlanForRules(base, []*ServiceQuotaRule{rule})
		got, err := svc.SnapshotForAccounts(context.Background(), base, plan, []int64{7})
		require.NoError(t, err)
		require.Empty(t, got)
	})
}

// TestSnapshotForAccounts_AccountWithChannel_BlocksAccount：path 含 account+channel，scope 仍为 account（铁律：account 优先级最高）。
func TestSnapshotForAccounts_AccountWithChannel_BlocksAccount(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()
	rule := ruleAccountChannelRPM(1, 7, 99, 5)
	svc := newQuotaServiceForSchedTest(t, []*ServiceQuotaRule{rule}, limiter, true)

	base := ServiceQuotaCheckRequest{UserID: 1, ChannelID: 99, Model: "claude"}
	req := base
	req.AccountID = 7
	key := svc.counterKey(req, rule, rule.Paths[0], rule.Limiters[0])
	limiter.counters[key] = 6

	plan := makePlanForRules(base, []*ServiceQuotaRule{rule})
	got, err := svc.SnapshotForAccounts(context.Background(), base, plan, []int64{7})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, ServiceQuotaPredictedBlockScopeAccount, got[7].ScopeKind)
}

// TestSnapshotForAccounts_ChannelOnly_BlocksAccount：path 仅含 channel_id（不含 account_id），scope=channel 仍入 map。
func TestSnapshotForAccounts_ChannelOnly_BlocksAccount(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()
	rule := ruleChannelOnlyRPM(1, 99, 5)
	svc := newQuotaServiceForSchedTest(t, []*ServiceQuotaRule{rule}, limiter, true)

	base := ServiceQuotaCheckRequest{UserID: 1, ChannelID: 99, Model: "claude"}
	// counter key 与 accountID 无关（path.AccountID==nil → counter shared by channel）。
	req := base
	req.AccountID = 7
	key := svc.counterKey(req, rule, rule.Paths[0], rule.Limiters[0])
	limiter.counters[key] = 5

	plan := makePlanForRules(base, []*ServiceQuotaRule{rule})
	got, err := svc.SnapshotForAccounts(context.Background(), base, plan, []int64{7, 8})
	require.NoError(t, err)
	// channel scope 命中 → 同 channel 的全部候选都应被剔除。
	require.Equal(t, ServiceQuotaPredictedBlockScopeChannel, got[7].ScopeKind)
	require.Equal(t, ServiceQuotaPredictedBlockScopeChannel, got[8].ScopeKind)
}

// TestSnapshotForAccounts_GroupOnly_NotBlocked：path 只含 group → 不进 map（切号无解，留给 Consume 429）。
func TestSnapshotForAccounts_GroupOnly_NotBlocked(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()
	rule := ruleGroupOnlyRPM(1, 5, 5)
	svc := newQuotaServiceForSchedTest(t, []*ServiceQuotaRule{rule}, limiter, true)

	base := ServiceQuotaCheckRequest{UserID: 1, GroupID: 5, Model: "claude"}
	req := base
	req.AccountID = 7
	key := svc.counterKey(req, rule, rule.Paths[0], rule.Limiters[0])
	limiter.counters[key] = 100

	plan := makePlanForRules(base, []*ServiceQuotaRule{rule})
	got, err := svc.SnapshotForAccounts(context.Background(), base, plan, []int64{7})
	require.NoError(t, err)
	require.Empty(t, got)
}

// TestSnapshotForAccounts_RedisFailure_FailOpen：SnapshotMany 错误 → 空 map + nil error。
func TestSnapshotForAccounts_RedisFailure_FailOpen(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()
	limiter.manyErr = errors.New("redis down")
	rule := ruleAccountRPM(1, 7, 5)
	svc := newQuotaServiceForSchedTest(t, []*ServiceQuotaRule{rule}, limiter, true)

	base := ServiceQuotaCheckRequest{UserID: 1, Model: "claude"}
	plan := makePlanForRules(base, []*ServiceQuotaRule{rule})
	got, err := svc.SnapshotForAccounts(context.Background(), base, plan, []int64{7})
	require.NoError(t, err)
	require.Nil(t, got)
}

// TestSnapshotForAccounts_BelowLimit_NotBlocked：用量未达 limit 不入 map。
func TestSnapshotForAccounts_BelowLimit_NotBlocked(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()
	rule := ruleAccountRPM(1, 7, 10)
	svc := newQuotaServiceForSchedTest(t, []*ServiceQuotaRule{rule}, limiter, true)

	base := ServiceQuotaCheckRequest{UserID: 1, Model: "claude"}
	req := base
	req.AccountID = 7
	key := svc.counterKey(req, rule, rule.Paths[0], rule.Limiters[0])
	limiter.counters[key] = 9

	plan := makePlanForRules(base, []*ServiceQuotaRule{rule})
	got, err := svc.SnapshotForAccounts(context.Background(), base, plan, []int64{7})
	require.NoError(t, err)
	require.Empty(t, got)
}
