//go:build unit

package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type monitorFakeRepo struct {
	rules []*ServiceQuotaRule
}

func (f *monitorFakeRepo) List(_ context.Context, _ ServiceQuotaListFilter) ([]*ServiceQuotaRule, error) {
	out := make([]*ServiceQuotaRule, len(f.rules))
	copy(out, f.rules)
	return out, nil
}

func (f *monitorFakeRepo) Create(_ context.Context, _ ServiceQuotaRuleInput) (*ServiceQuotaRule, error) {
	return nil, errors.New("not supported")
}
func (f *monitorFakeRepo) Update(_ context.Context, _ int64, _ ServiceQuotaRuleInput) (*ServiceQuotaRule, error) {
	return nil, errors.New("not supported")
}
func (f *monitorFakeRepo) Delete(_ context.Context, _ int64) error {
	return errors.New("not supported")
}
func (f *monitorFakeRepo) FetchAccountScope(_ context.Context, _ int64) (*AccountScopeInfo, error) {
	return nil, errors.New("not supported")
}
func (f *monitorFakeRepo) FetchGroupScope(_ context.Context, _ int64) (*GroupScopeInfo, error) {
	return nil, errors.New("not supported")
}
func (f *monitorFakeRepo) FetchChannelScope(_ context.Context, _ int64) (*ChannelScopeInfo, error) {
	return nil, errors.New("not supported")
}

type monitorFakeLimiter struct {
	mu        sync.Mutex
	snapshots map[string]LimiterSnapshot
	manyErr   error
	calls     [][]SnapshotKey
}

func newMonitorFakeLimiter() *monitorFakeLimiter {
	return &monitorFakeLimiter{snapshots: map[string]LimiterSnapshot{}}
}

func (f *monitorFakeLimiter) Current(_ context.Context, _ string, _ time.Duration, _ string) (float64, error) {
	return 0, nil
}
func (f *monitorFakeLimiter) Increment(_ context.Context, _ string, _ float64, _ time.Duration, _ string) (float64, error) {
	return 0, nil
}
func (f *monitorFakeLimiter) Acquire(_ context.Context, _, _ string, _ int64) (bool, error) {
	return true, nil
}
func (f *monitorFakeLimiter) Release(_ context.Context, _, _ string) error   { return nil }
func (f *monitorFakeLimiter) Reset(_ context.Context, _ string) error        { return nil }
func (f *monitorFakeLimiter) ResetPattern(_ context.Context, _ string) error { return nil }

func (f *monitorFakeLimiter) Snapshot(_ context.Context, key string, _ time.Duration, _ string) (LimiterSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.snapshots[key], nil
}

func (f *monitorFakeLimiter) SnapshotMany(_ context.Context, keys []SnapshotKey) ([]LimiterSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, append([]SnapshotKey(nil), keys...))
	if f.manyErr != nil {
		return nil, f.manyErr
	}
	out := make([]LimiterSnapshot, len(keys))
	for i, k := range keys {
		out[i] = f.snapshots[k.Key]
	}
	return out, nil
}

type monitorFakeCache struct{}

func (monitorFakeCache) GetRules(_ context.Context) ([]*ServiceQuotaRule, bool, error) {
	return nil, false, nil
}
func (monitorFakeCache) SetRules(_ context.Context, _ []*ServiceQuotaRule) error { return nil }
func (monitorFakeCache) InvalidateRules(_ context.Context) error                 { return nil }
func (monitorFakeCache) GetEnabled(_ context.Context) (*bool, error)             { return nil, nil }
func (monitorFakeCache) SetEnabled(_ context.Context, _ bool) error              { return nil }
func (monitorFakeCache) InvalidateEnabled(_ context.Context) error               { return nil }
func (monitorFakeCache) Invalidate(_ context.Context) error                      { return nil }

func newMonitorService(t *testing.T, enabled bool, rules []*ServiceQuotaRule, limiter *monitorFakeLimiter) ServiceQuotaMonitorService {
	t.Helper()
	settingRepo := newMockSettingRepo()
	if enabled {
		settingRepo.data[SettingKeyServiceQuotaEnabled] = "true"
	} else {
		settingRepo.data[SettingKeyServiceQuotaEnabled] = "false"
	}
	settings := NewSettingService(settingRepo, &config.Config{})
	repo := &monitorFakeRepo{rules: rules}
	if limiter == nil {
		limiter = newMonitorFakeLimiter()
	}
	return NewServiceQuotaMonitorService(repo, limiter, monitorFakeCache{}, settings)
}

func ptrInt64Monitor(v int64) *int64    { return &v }
func ptrStringMonitor(v string) *string { return &v }

func ruleSimple(id int64, mode string, limiterType string, limit float64, targetUsers []int64) *ServiceQuotaRule {
	return &ServiceQuotaRule{
		ID:            id,
		Enabled:       true,
		CounterMode:   mode,
		TargetUserIDs: targetUsers,
		Limiters: []ServiceQuotaLimiterDef{
			{ID: id*100 + 1, RuleID: id, LimiterType: limiterType, WindowMode: ServiceQuotaWindowFixed, LimitValue: limit},
		},
		Paths: []ServiceQuotaPathDef{
			{ID: id*1000 + 1, RuleID: id},
		},
	}
}

func TestSnapshot_Disabled_ReturnsEmpty(t *testing.T) {
	svc := newMonitorService(t, false, nil, nil)
	snap, err := svc.Snapshot(context.Background(), MonitorSnapshotFilter{})
	require.NoError(t, err)
	require.NotNil(t, snap)
	require.False(t, snap.Enabled)
	require.Empty(t, snap.Items)
}

func TestSnapshot_NoRules_ReturnsEmpty(t *testing.T) {
	svc := newMonitorService(t, true, nil, nil)
	snap, err := svc.Snapshot(context.Background(), MonitorSnapshotFilter{})
	require.NoError(t, err)
	require.True(t, snap.Enabled)
	require.Empty(t, snap.Items)
	require.False(t, snap.Truncated)
}

func TestSnapshot_CartesianExpansion(t *testing.T) {
	rule := &ServiceQuotaRule{
		ID:            42,
		Enabled:       true,
		CounterMode:   ServiceQuotaCounterModeUser,
		TargetUserIDs: []int64{7, 9},
		Limiters: []ServiceQuotaLimiterDef{
			{ID: 1, RuleID: 42, LimiterType: ServiceQuotaLimiterRPM, WindowMode: ServiceQuotaWindowFixed, LimitValue: 100},
			{ID: 2, RuleID: 42, LimiterType: ServiceQuotaLimiterConcurrency, WindowMode: ServiceQuotaWindowFixed, LimitValue: 5},
		},
		Paths: []ServiceQuotaPathDef{
			{ID: 100, RuleID: 42, Platform: ptrStringMonitor("antigravity")},
			{ID: 101, RuleID: 42, Platform: ptrStringMonitor("openai")},
		},
	}
	svc := newMonitorService(t, true, []*ServiceQuotaRule{rule}, nil)
	snap, err := svc.Snapshot(context.Background(), MonitorSnapshotFilter{})
	require.NoError(t, err)
	require.Len(t, snap.Items, 8)
	idxSeen := map[int]int{}
	for _, item := range snap.Items {
		idxSeen[item.PathIndex]++
	}
	require.Equal(t, 4, idxSeen[1])
	require.Equal(t, 4, idxSeen[2])
}

func TestSnapshot_AdminFilter_ByPlatform(t *testing.T) {
	rule := &ServiceQuotaRule{
		ID:          1,
		Enabled:     true,
		CounterMode: ServiceQuotaCounterModeShared,
		Limiters: []ServiceQuotaLimiterDef{
			{ID: 1, RuleID: 1, LimiterType: ServiceQuotaLimiterRPM, WindowMode: ServiceQuotaWindowFixed, LimitValue: 100},
		},
		Paths: []ServiceQuotaPathDef{
			{ID: 1, RuleID: 1, Platform: nil},
			{ID: 2, RuleID: 1, Platform: ptrStringMonitor("openai")},
			{ID: 3, RuleID: 1, Platform: ptrStringMonitor("antigravity")},
		},
	}
	svc := newMonitorService(t, true, []*ServiceQuotaRule{rule}, nil)
	snap, err := svc.Snapshot(context.Background(), MonitorSnapshotFilter{Platform: ptrStringMonitor("openai")})
	require.NoError(t, err)
	require.Len(t, snap.Items, 2)
}

func TestSnapshot_AdminFilter_ByUserID(t *testing.T) {
	rule := &ServiceQuotaRule{
		ID:            1,
		Enabled:       true,
		CounterMode:   ServiceQuotaCounterModeUser,
		TargetUserIDs: []int64{5, 6},
		Limiters: []ServiceQuotaLimiterDef{
			{ID: 1, RuleID: 1, LimiterType: ServiceQuotaLimiterRPM, WindowMode: ServiceQuotaWindowFixed, LimitValue: 100},
		},
		Paths: []ServiceQuotaPathDef{{ID: 1, RuleID: 1}},
	}
	svc := newMonitorService(t, true, []*ServiceQuotaRule{rule}, nil)
	snap, err := svc.Snapshot(context.Background(), MonitorSnapshotFilter{UserID: ptrInt64Monitor(6)})
	require.NoError(t, err)
	require.Len(t, snap.Items, 1)
	require.NotNil(t, snap.Items[0].ScopeUserID)
	require.Equal(t, int64(6), *snap.Items[0].ScopeUserID)
}

// 用户视角下 PathSummary 不再被全局抹空：仍暴露 platform / model_pattern
// 给用户看到自己被限流的"业务路径"，但 channel/group/account 内部拓扑必须为 nil。
// CounterMode 与 ScopeUserID 仍抹空（admin 专属字段）。
func TestSnapshot_UserScope_PathSummaryHidesInternalTopologyOnly(t *testing.T) {
	rule := ruleSimple(1, ServiceQuotaCounterModeUser, ServiceQuotaLimiterRPM, 100, []int64{7})
	rule.Paths[0].Platform = ptrStringMonitor("openai")
	model := "gpt-*"
	rule.Paths[0].ModelPattern = &model
	channelID := int64(42)
	rule.Paths[0].ChannelID = &channelID
	groupID := int64(11)
	rule.Paths[0].GroupID = &groupID
	accountID := int64(99)
	rule.Paths[0].AccountID = &accountID

	svc := newMonitorService(t, true, []*ServiceQuotaRule{rule}, nil)
	snap, err := svc.Snapshot(context.Background(), MonitorSnapshotFilter{
		UserScope: &MonitorUserScope{UserID: 7},
	})
	require.NoError(t, err)
	require.Len(t, snap.Items, 1)
	ps := snap.Items[0].PathSummary
	require.NotNil(t, ps)
	require.NotNil(t, ps.Platform)
	require.Equal(t, "openai", *ps.Platform)
	require.NotNil(t, ps.ModelPattern)
	require.Equal(t, "gpt-*", *ps.ModelPattern)
	require.Nil(t, ps.ChannelID, "channel_id 不能暴露给用户")
	require.Nil(t, ps.GroupID, "group_id 不能暴露给用户")
	require.Nil(t, ps.AccountID, "account_id 不能暴露给用户")
	require.Equal(t, "", snap.Items[0].CounterMode)
	require.Nil(t, snap.Items[0].ScopeUserID)
}

// 用户视角下 path 完全 wildcard（platform/model 都 nil）→ PathSummary 仍抹成 nil，
// 让前端走"所有请求"分支显示。
func TestSnapshot_UserScope_AllWildcardPathReturnsNilSummary(t *testing.T) {
	rule := ruleSimple(1, ServiceQuotaCounterModeUser, ServiceQuotaLimiterRPM, 100, []int64{7})
	// rule.Paths[0] 默认所有字段 nil（全 wildcard）
	svc := newMonitorService(t, true, []*ServiceQuotaRule{rule}, nil)
	snap, err := svc.Snapshot(context.Background(), MonitorSnapshotFilter{
		UserScope: &MonitorUserScope{UserID: 7},
	})
	require.NoError(t, err)
	require.Len(t, snap.Items, 1)
	require.Nil(t, snap.Items[0].PathSummary, "全 wildcard path 应返回 nil 让前端走 allRequests 分支")
}

func TestSnapshot_UserScope_KeepsShared(t *testing.T) {
	rule := ruleSimple(1, ServiceQuotaCounterModeShared, ServiceQuotaLimiterRPM, 100, nil)
	svc := newMonitorService(t, true, []*ServiceQuotaRule{rule}, nil)
	snap, err := svc.Snapshot(context.Background(), MonitorSnapshotFilter{
		UserScope: &MonitorUserScope{UserID: 999},
	})
	require.NoError(t, err)
	require.Len(t, snap.Items, 1)
}

func TestSnapshot_UserScope_FiltersToTargetUsers(t *testing.T) {
	rule := ruleSimple(1, ServiceQuotaCounterModeUser, ServiceQuotaLimiterRPM, 100, []int64{5, 6})
	svc := newMonitorService(t, true, []*ServiceQuotaRule{rule}, nil)
	snap, err := svc.Snapshot(context.Background(), MonitorSnapshotFilter{
		UserScope: &MonitorUserScope{UserID: 8},
	})
	require.NoError(t, err)
	require.Empty(t, snap.Items)

	snap2, err := svc.Snapshot(context.Background(), MonitorSnapshotFilter{
		UserScope: &MonitorUserScope{UserID: 5},
	})
	require.NoError(t, err)
	require.Len(t, snap2.Items, 1)
}

// TestSnapshot_AdminNoFilter_PerUserSkipped 验证新语义：admin 无 user filter 时
// per_user 规则完全不展开（不再产生占位行）。fake limiter 预置 shared 风格的 key，
// 验证 SnapshotMany 不会因为有遗留计数器而误命中。
func TestSnapshot_AdminNoFilter_PerUserSkipped(t *testing.T) {
	rule := ruleSimple(1, ServiceQuotaCounterModePerUser, ServiceQuotaLimiterRPM, 100, nil)
	limiter := newMonitorFakeLimiter()
	sharedKey := BuildServiceQuotaCounterKey(rule.ID, rule.Paths[0].ID, ServiceQuotaLimiterRPM, nil)
	limiter.snapshots[sharedKey] = LimiterSnapshot{Current: 999, Exists: true}
	svc := newMonitorService(t, true, []*ServiceQuotaRule{rule}, limiter)

	snap, err := svc.Snapshot(context.Background(), MonitorSnapshotFilter{})
	require.NoError(t, err)
	require.Empty(t, snap.Items, "per_user 规则在 admin 无 user filter 时不应展开任何行")
}

// TestSnapshot_AdminNoFilter_UserModeExpandedToTargets 验证新语义：admin 无 user filter
// 时 user 模式按 TargetUserIDs 展开 N 行（每个目标用户 1 行）。
func TestSnapshot_AdminNoFilter_UserModeExpandedToTargets(t *testing.T) {
	rule := ruleSimple(1, ServiceQuotaCounterModeUser, ServiceQuotaLimiterRPM, 100, []int64{5, 7})
	svc := newMonitorService(t, true, []*ServiceQuotaRule{rule}, nil)
	snap, err := svc.Snapshot(context.Background(), MonitorSnapshotFilter{})
	require.NoError(t, err)
	require.Len(t, snap.Items, 2)
	scopeUsers := []int64{}
	for _, item := range snap.Items {
		require.NotNil(t, item.ScopeUserID)
		scopeUsers = append(scopeUsers, *item.ScopeUserID)
	}
	require.ElementsMatch(t, []int64{5, 7}, scopeUsers)
}

// TestSnapshot_AdminNoFilter_SharedKeptSingle 验证新语义：shared 模式 1 行，
// scope_user_id=nil（target=shared）。
func TestSnapshot_AdminNoFilter_SharedKeptSingle(t *testing.T) {
	rule := ruleSimple(1, ServiceQuotaCounterModeShared, ServiceQuotaLimiterRPM, 100, nil)
	svc := newMonitorService(t, true, []*ServiceQuotaRule{rule}, nil)
	snap, err := svc.Snapshot(context.Background(), MonitorSnapshotFilter{})
	require.NoError(t, err)
	require.Len(t, snap.Items, 1)
	require.Nil(t, snap.Items[0].ScopeUserID)
}

// TestSnapshot_AdminWithUserFilter_PerUserExpandedToFilterUser 验证：admin 提供
// user_id filter 时 per_user 规则按 filter user 展开 1 行。
func TestSnapshot_AdminWithUserFilter_PerUserExpandedToFilterUser(t *testing.T) {
	rule := ruleSimple(1, ServiceQuotaCounterModePerUser, ServiceQuotaLimiterRPM, 100, nil)
	svc := newMonitorService(t, true, []*ServiceQuotaRule{rule}, nil)
	snap, err := svc.Snapshot(context.Background(), MonitorSnapshotFilter{UserID: ptrInt64Monitor(42)})
	require.NoError(t, err)
	require.Len(t, snap.Items, 1)
	require.NotNil(t, snap.Items[0].ScopeUserID)
	require.Equal(t, int64(42), *snap.Items[0].ScopeUserID)
}

// TestSnapshot_AdminWithUserFilter_UserModeOnlyIfTargeted 验证：admin 带 user filter
// 时 user 模式只在 filter user 是 TargetUserIDs 之一才返回 1 行；否则不返回。
func TestSnapshot_AdminWithUserFilter_UserModeOnlyIfTargeted(t *testing.T) {
	t.Run("filter user not in target", func(t *testing.T) {
		rule := ruleSimple(1, ServiceQuotaCounterModeUser, ServiceQuotaLimiterRPM, 100, []int64{5, 7})
		svc := newMonitorService(t, true, []*ServiceQuotaRule{rule}, nil)
		snap, err := svc.Snapshot(context.Background(), MonitorSnapshotFilter{UserID: ptrInt64Monitor(42)})
		require.NoError(t, err)
		require.Empty(t, snap.Items)
	})
	t.Run("filter user in target", func(t *testing.T) {
		rule := ruleSimple(1, ServiceQuotaCounterModeUser, ServiceQuotaLimiterRPM, 100, []int64{5, 42})
		svc := newMonitorService(t, true, []*ServiceQuotaRule{rule}, nil)
		snap, err := svc.Snapshot(context.Background(), MonitorSnapshotFilter{UserID: ptrInt64Monitor(42)})
		require.NoError(t, err)
		require.Len(t, snap.Items, 1)
		require.NotNil(t, snap.Items[0].ScopeUserID)
		require.Equal(t, int64(42), *snap.Items[0].ScopeUserID)
	})
}

// TestSnapshot_AdminWithUserFilter_SharedAlwaysIncluded 验证：shared 规则不受 user filter
// 影响，admin 带 user filter 时仍返回 1 行（让 admin 知道全局共享池占用）。
func TestSnapshot_AdminWithUserFilter_SharedAlwaysIncluded(t *testing.T) {
	rule := ruleSimple(1, ServiceQuotaCounterModeShared, ServiceQuotaLimiterRPM, 100, nil)
	svc := newMonitorService(t, true, []*ServiceQuotaRule{rule}, nil)
	snap, err := svc.Snapshot(context.Background(), MonitorSnapshotFilter{UserID: ptrInt64Monitor(42)})
	require.NoError(t, err)
	require.Len(t, snap.Items, 1)
	require.Nil(t, snap.Items[0].ScopeUserID)
}

// TestSnapshot_BuildLimiterRuntime_TransparentResetAt 验证 LimiterRuntime 透传
// LimiterSnapshot.ResetAtUnixMs（来自 repo 层 PTTL / now+window 推算）。
func TestSnapshot_BuildLimiterRuntime_TransparentResetAt(t *testing.T) {
	rule := ruleSimple(1, ServiceQuotaCounterModeShared, ServiceQuotaLimiterRPM, 100, nil)
	limiter := newMonitorFakeLimiter()
	key := BuildServiceQuotaCounterKey(rule.ID, rule.Paths[0].ID, ServiceQuotaLimiterRPM, nil)
	expectedReset := time.Now().Add(60 * time.Second).UnixMilli()
	limiter.snapshots[key] = LimiterSnapshot{Current: 5, Exists: true, ResetAtUnixMs: expectedReset}
	svc := newMonitorService(t, true, []*ServiceQuotaRule{rule}, limiter)
	snap, err := svc.Snapshot(context.Background(), MonitorSnapshotFilter{})
	require.NoError(t, err)
	require.Len(t, snap.Items, 1)
	require.Equal(t, expectedReset, snap.Items[0].ResetAtUnixMs)
}

func TestSnapshot_HardCap_Truncated(t *testing.T) {
	limiters := make([]ServiceQuotaLimiterDef, 0, 6000)
	for i := 1; i <= 6000; i++ {
		limiters = append(limiters, ServiceQuotaLimiterDef{
			ID: int64(i), RuleID: 1, LimiterType: ServiceQuotaLimiterRPM,
			WindowMode: ServiceQuotaWindowFixed, LimitValue: 100,
		})
	}
	rule := &ServiceQuotaRule{
		ID: 1, Enabled: true, CounterMode: ServiceQuotaCounterModeShared,
		Limiters: limiters,
		Paths:    []ServiceQuotaPathDef{{ID: 1, RuleID: 1}},
	}
	svc := newMonitorService(t, true, []*ServiceQuotaRule{rule}, nil)
	snap, err := svc.Snapshot(context.Background(), MonitorSnapshotFilter{})
	require.NoError(t, err)
	require.Len(t, snap.Items, monitorMaxRows)
	require.True(t, snap.Truncated)
}

func TestSnapshot_BuildCounterKey_MatchesPreCheck(t *testing.T) {
	rule := ruleSimple(42, ServiceQuotaCounterModeUser, ServiceQuotaLimiterRPM, 100, []int64{7})
	limiter := newMonitorFakeLimiter()
	expectedKey := BuildServiceQuotaCounterKey(42, rule.Paths[0].ID, ServiceQuotaLimiterRPM, ptrInt64Monitor(7))
	limiter.snapshots[expectedKey] = LimiterSnapshot{Current: 33, Exists: true}
	svc := newMonitorService(t, true, []*ServiceQuotaRule{rule}, limiter)
	snap, err := svc.Snapshot(context.Background(), MonitorSnapshotFilter{})
	require.NoError(t, err)
	require.Len(t, snap.Items, 1)
	require.True(t, snap.Items[0].Exists)
	require.InDelta(t, 33.0, snap.Items[0].Current, 1e-9)
	require.InDelta(t, 33.0, snap.Items[0].UtilizationPct, 1e-9)
	require.Len(t, limiter.calls, 1)
	require.Equal(t, expectedKey, limiter.calls[0][0].Key)
}

func TestSnapshot_LimiterError_FailSoft(t *testing.T) {
	rule := ruleSimple(1, ServiceQuotaCounterModeShared, ServiceQuotaLimiterRPM, 100, nil)
	limiter := newMonitorFakeLimiter()
	limiter.manyErr = errors.New("redis down")
	svc := newMonitorService(t, true, []*ServiceQuotaRule{rule}, limiter)
	snap, err := svc.Snapshot(context.Background(), MonitorSnapshotFilter{})
	require.NoError(t, err)
	require.Len(t, snap.Items, 1)
	require.False(t, snap.Items[0].Exists)
	require.Equal(t, 0.0, snap.Items[0].Current)
}
