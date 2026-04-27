//go:build unit

package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// ─── Mocks ───

// fakeServiceQuotaRepo 提供一个无 DB 依赖的规则仓库。规则用 List 返回，其他写入方法直接返回错误，
// 因为两阶段 PreCheck 测试不会触及写路径。
type fakeServiceQuotaRepo struct {
	rules []*ServiceQuotaRule
}

func (f *fakeServiceQuotaRepo) List(_ context.Context, _ ServiceQuotaListFilter) ([]*ServiceQuotaRule, error) {
	return append([]*ServiceQuotaRule(nil), f.rules...), nil
}

func (f *fakeServiceQuotaRepo) Create(_ context.Context, _ ServiceQuotaRuleInput) (*ServiceQuotaRule, error) {
	return nil, errors.New("fakeServiceQuotaRepo: Create not supported")
}

func (f *fakeServiceQuotaRepo) Update(_ context.Context, _ int64, _ ServiceQuotaRuleInput) (*ServiceQuotaRule, error) {
	return nil, errors.New("fakeServiceQuotaRepo: Update not supported")
}

func (f *fakeServiceQuotaRepo) Delete(_ context.Context, _ int64) error {
	return errors.New("fakeServiceQuotaRepo: Delete not supported")
}

func (f *fakeServiceQuotaRepo) FetchAccountScope(_ context.Context, _ int64) (*AccountScopeInfo, error) {
	return nil, errors.New("fakeServiceQuotaRepo: FetchAccountScope not supported")
}

func (f *fakeServiceQuotaRepo) FetchGroupScope(_ context.Context, _ int64) (*GroupScopeInfo, error) {
	return nil, errors.New("fakeServiceQuotaRepo: FetchGroupScope not supported")
}

// fakeServiceQuotaLimiter 是内存版限流器，用 map 存计数 / 并发槽位，方便观察 PreCheckAcquire
// 在 channel/account scope 是否真的命中。
type fakeServiceQuotaLimiter struct {
	mu          sync.Mutex
	counters    map[string]float64
	concurrency map[string]map[string]struct{} // key -> set of members
	// acquireCalls / incrementCalls 让测试能断言"PreCheckAcquire 是否真的按 channel/account scope 命中".
	acquireCalls   []string
	incrementCalls []string
}

func newFakeLimiter() *fakeServiceQuotaLimiter {
	return &fakeServiceQuotaLimiter{
		counters:    map[string]float64{},
		concurrency: map[string]map[string]struct{}{},
	}
}

func (f *fakeServiceQuotaLimiter) Current(_ context.Context, key string, _ time.Duration, _ string) (float64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.counters[key], nil
}

func (f *fakeServiceQuotaLimiter) Increment(_ context.Context, key string, delta float64, _ time.Duration, _ string) (float64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.counters[key] += delta
	f.incrementCalls = append(f.incrementCalls, key)
	return f.counters[key], nil
}

func (f *fakeServiceQuotaLimiter) Acquire(_ context.Context, key, member string, limit int64) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.acquireCalls = append(f.acquireCalls, key)
	set, ok := f.concurrency[key]
	if !ok {
		set = map[string]struct{}{}
		f.concurrency[key] = set
	}
	if int64(len(set)) >= limit {
		return false, nil
	}
	set[member] = struct{}{}
	return true, nil
}

func (f *fakeServiceQuotaLimiter) Release(_ context.Context, key, member string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if set, ok := f.concurrency[key]; ok {
		delete(set, member)
	}
	return nil
}

// Snapshot / SnapshotMany 是只读监控路径用的接口方法，两阶段 PreCheck 测试不依赖
// 真实快照语义。返回零值即可——只要保证 fakeServiceQuotaLimiter 仍然实现
// service.ServiceQuotaLimiter 接口，避免 wire 编译失败。
func (f *fakeServiceQuotaLimiter) Snapshot(_ context.Context, _ string, _ time.Duration, _ string) (LimiterSnapshot, error) {
	return LimiterSnapshot{}, nil
}

func (f *fakeServiceQuotaLimiter) SnapshotMany(_ context.Context, keys []SnapshotKey) ([]LimiterSnapshot, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	return make([]LimiterSnapshot, len(keys)), nil
}

// Reset 模拟 DEL：从内存 map 中清掉对应 key 的计数与并发集合。
// 测试目前不直接断言 Reset 行为，但接口必须实现以让编译通过。
func (f *fakeServiceQuotaLimiter) Reset(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.counters, key)
	delete(f.concurrency, key)
	return nil
}

// ─── Helpers ───

// newQuotaServiceForTest 拼装一个 *serviceQuotaService，带最小能跑两阶段路径的依赖。
// settingRepo 由 caller 控制，方便切换 feature flag。
func newQuotaServiceForTest(t *testing.T, rules []*ServiceQuotaRule, settingRepo *mockSettingRepo, limiter *fakeServiceQuotaLimiter) *serviceQuotaService {
	t.Helper()
	if settingRepo == nil {
		settingRepo = newMockSettingRepo()
	}
	settingRepo.data[SettingKeyServiceQuotaEnabled] = "true"
	settings := NewSettingService(settingRepo, &config.Config{})
	repo := &fakeServiceQuotaRepo{rules: rules}
	return &serviceQuotaService{
		repo:     repo,
		settings: settings,
		limiter:  limiter,
		// cache=nil 时 service 会回退走 repo.List + singleflight，足够覆盖测试路径。
	}
}

// ruleConcurrencyAccount 构造一个限定 account_id 的 concurrency 限流规则。
func ruleConcurrencyAccount(id, accountID int64, limit float64) *ServiceQuotaRule {
	acc := accountID
	return &ServiceQuotaRule{
		ID:          id,
		Enabled:     true,
		CounterMode: ServiceQuotaCounterModePerUser,
		Limiters: []ServiceQuotaLimiterDef{
			{ID: id*100 + 1, RuleID: id, LimiterType: ServiceQuotaLimiterConcurrency, WindowMode: ServiceQuotaWindowFixed, LimitValue: limit},
		},
		Paths: []ServiceQuotaPathDef{
			{ID: id*100 + 11, RuleID: id, AccountID: &acc},
		},
	}
}

// ruleRPMChannel 构造一个限定 channel_id 的 RPM 规则。
func ruleRPMChannel(id, channelID int64, limit float64) *ServiceQuotaRule {
	ch := channelID
	return &ServiceQuotaRule{
		ID:          id,
		Enabled:     true,
		CounterMode: ServiceQuotaCounterModePerUser,
		Limiters: []ServiceQuotaLimiterDef{
			{ID: id*100 + 1, RuleID: id, LimiterType: ServiceQuotaLimiterRPM, WindowMode: ServiceQuotaWindowFixed, LimitValue: limit},
		},
		Paths: []ServiceQuotaPathDef{
			{ID: id*100 + 11, RuleID: id, ChannelID: &ch},
		},
	}
}

// ─── Tests ───

func TestPreCheckSelect_ReturnsCandidatesWithoutAcquire(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()
	rule := ruleConcurrencyAccount(1, 7, 1)
	svc := newQuotaServiceForTest(t, []*ServiceQuotaRule{rule}, nil, limiter)

	plan, err := svc.PreCheckSelect(context.Background(), ServiceQuotaCheckRequest{UserID: 42, Platform: "anthropic"})
	require.NoError(t, err)
	require.NotNil(t, plan)
	require.Len(t, plan.Rules, 1)
	require.Equal(t, int64(1), plan.Rules[0].ID)
	require.False(t, plan.PreparedAt.IsZero())

	// 关键：阶段 A 不应触发任何 Acquire / Increment。
	require.Empty(t, limiter.acquireCalls)
	require.Empty(t, limiter.incrementCalls)
}

func TestPreCheckAcquire_ChannelScopeRPMHitsOnlyMatchingChannel(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()
	rule := ruleRPMChannel(2, 99, 100)
	svc := newQuotaServiceForTest(t, []*ServiceQuotaRule{rule}, nil, limiter)

	plan, err := svc.PreCheckSelect(context.Background(), ServiceQuotaCheckRequest{UserID: 42})
	require.NoError(t, err)
	require.NotNil(t, plan)

	// channelID=0 不命中 channel=99 的 path，应走"无规则匹配"分支。
	lease, err := svc.PreCheckAcquire(context.Background(), plan, 0, 0)
	require.NoError(t, err)
	require.Nil(t, lease)
	require.Empty(t, limiter.incrementCalls, "channel=0 不应触发 RPM Increment")

	// 选定正确 channel=99 后才命中 path，触发 Increment。
	// RPM 规则没有 concurrency 限流器，lease.Release 应为 nil（非 lease 本身 nil）。
	lease, err = svc.PreCheckAcquire(context.Background(), plan, 99, 0)
	require.NoError(t, err)
	require.NotNil(t, lease)
	require.Nil(t, lease.Release)
	require.Len(t, limiter.incrementCalls, 1)
}

func TestPreCheckAcquire_AccountConcurrencyEnforced(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()
	rule := ruleConcurrencyAccount(3, 7, 1)
	svc := newQuotaServiceForTest(t, []*ServiceQuotaRule{rule}, nil, limiter)

	plan, err := svc.PreCheckSelect(context.Background(), ServiceQuotaCheckRequest{UserID: 42})
	require.NoError(t, err)
	require.NotNil(t, plan)

	// 第一次抢应成功并返回 lease。
	lease1, err := svc.PreCheckAcquire(context.Background(), plan, 0, 7)
	require.NoError(t, err)
	require.NotNil(t, lease1)
	require.NotNil(t, lease1.Release)

	// 第二次抢同 account 应被拒（limit=1 已占满）。
	lease2, err := svc.PreCheckAcquire(context.Background(), plan, 0, 7)
	require.Error(t, err)
	require.Nil(t, lease2)
	require.True(t, errors.Is(err, ErrServiceQuotaExceeded) || err.Error() != "")

	// 释放后第三次抢应成功，证明 lease.Release 真的清理了 concurrency 槽位。
	lease1.Release()
	lease3, err := svc.PreCheckAcquire(context.Background(), plan, 0, 7)
	require.NoError(t, err)
	require.NotNil(t, lease3)
	lease3.Release()
}

func TestPreCheckAcquire_DifferentAccountSamRulePathMissesBecauseOfPathFilter(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()
	rule := ruleConcurrencyAccount(4, 7, 1)
	svc := newQuotaServiceForTest(t, []*ServiceQuotaRule{rule}, nil, limiter)

	plan, err := svc.PreCheckSelect(context.Background(), ServiceQuotaCheckRequest{UserID: 42})
	require.NoError(t, err)

	// account=8 与规则限定的 account=7 不匹配，PreCheckAcquire 应不触及限流器。
	lease, err := svc.PreCheckAcquire(context.Background(), plan, 0, 8)
	require.NoError(t, err)
	require.Nil(t, lease)
	require.Empty(t, limiter.acquireCalls)
}

func TestIsPreCheckTwoPhase_DefaultFalse(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()
	repo := newMockSettingRepo()
	svc := newQuotaServiceForTest(t, nil, repo, limiter)
	require.False(t, svc.IsPreCheckTwoPhase(context.Background()))
}

func TestIsPreCheckTwoPhase_ReadsSetting(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()
	repo := newMockSettingRepo()
	repo.data[SettingKeyServiceQuotaPreCheckTwoPhase] = "true"
	svc := newQuotaServiceForTest(t, nil, repo, limiter)
	require.True(t, svc.IsPreCheckTwoPhase(context.Background()))
}

func TestPreCheck_LegacySinglePhase_StillWorks(t *testing.T) {
	t.Parallel()
	// 旧 PreCheck 必须仍然能在传入完整 ChannelID/AccountID 时正确命中。
	limiter := newFakeLimiter()
	rule := ruleConcurrencyAccount(5, 7, 1)
	svc := newQuotaServiceForTest(t, []*ServiceQuotaRule{rule}, nil, limiter)

	lease, err := svc.PreCheck(context.Background(), ServiceQuotaCheckRequest{UserID: 42, AccountID: 7})
	require.NoError(t, err)
	require.NotNil(t, lease)
	require.Len(t, limiter.acquireCalls, 1)
	lease.Release()
}

func TestBillingTicket_Consume_FlagOff_NoOp(t *testing.T) {
	t.Parallel()
	// flag off 路径：Prepare 已经完成 PreCheck，Consume 应是 noop（不再触发 Acquire）。
	limiter := newFakeLimiter()
	rule := ruleConcurrencyAccount(6, 7, 5)
	repo := newMockSettingRepo()
	// flag 默认 false，无需显式设置。
	svc := newQuotaServiceForTest(t, []*ServiceQuotaRule{rule}, repo, limiter)

	ticket := &BillingTicket{
		svc:          nil, // 走旧路径：lease 已存在，Consume 直接 noop
		quotaReq:     ServiceQuotaCheckRequest{UserID: 42, AccountID: 7},
		twoPhase:     false,
		legacyOneOff: true,
	}
	// 模拟 Prepare 阶段已经 acquire：直接调一次旧 PreCheck 拿 lease 塞进去。
	lease, err := svc.PreCheck(context.Background(), ServiceQuotaCheckRequest{UserID: 42, AccountID: 7})
	require.NoError(t, err)
	require.NotNil(t, lease)
	ticket.lease = wrapServiceQuotaLeaseOnce(lease)

	acquireCallsBefore := len(limiter.acquireCalls)
	require.NoError(t, ticket.Consume(context.Background(), 0, 7))
	require.Equal(t, acquireCallsBefore, len(limiter.acquireCalls), "flag off 时 Consume 不应再 Acquire")

	ticket.Close()
	// Close 后再 Consume 应返回错误。
	require.Error(t, ticket.Consume(context.Background(), 0, 7))
}

func TestBillingTicket_Consume_FlagOn_AcquiresLazily(t *testing.T) {
	t.Parallel()
	// flag on 路径：Prepare 阶段不 acquire，Consume 才真正 acquire。
	limiter := newFakeLimiter()
	rule := ruleConcurrencyAccount(7, 7, 5)
	repo := newMockSettingRepo()
	repo.data[SettingKeyServiceQuotaPreCheckTwoPhase] = "true"
	svc := newQuotaServiceForTest(t, []*ServiceQuotaRule{rule}, repo, limiter)

	plan, err := svc.PreCheckSelect(context.Background(), ServiceQuotaCheckRequest{UserID: 42})
	require.NoError(t, err)
	require.NotNil(t, plan)

	ticket := &BillingTicket{
		svc:      &BillingCacheService{serviceQuota: svc},
		quotaReq: ServiceQuotaCheckRequest{UserID: 42},
		plan:     plan,
		twoPhase: true,
	}

	require.Empty(t, limiter.acquireCalls, "Prepare 阶段不应 Acquire")
	require.NoError(t, ticket.Consume(context.Background(), 0, 7))
	require.Len(t, limiter.acquireCalls, 1, "Consume 阶段应触发 Acquire")
	require.NotNil(t, ticket.lease)

	// 二次 Consume 切换 account：应释放旧 lease，再尝试新 path（account=8 不匹配规则）。
	require.NoError(t, ticket.Consume(context.Background(), 0, 8))
	require.Nil(t, ticket.lease, "切换到不命中的 account 后 lease 应为 nil")

	ticket.Close()
}
