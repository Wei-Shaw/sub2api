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
func (f *monitorFakeLimiter) Release(_ context.Context, _, _ string) error { return nil }

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

func TestSnapshot_UserScope_HidesPathSummary(t *testing.T) {
	rule := ruleSimple(1, ServiceQuotaCounterModeUser, ServiceQuotaLimiterRPM, 100, []int64{7})
	rule.Paths[0].Platform = ptrStringMonitor("openai")
	svc := newMonitorService(t, true, []*ServiceQuotaRule{rule}, nil)
	snap, err := svc.Snapshot(context.Background(), MonitorSnapshotFilter{
		UserScope: &MonitorUserScope{UserID: 7},
	})
	require.NoError(t, err)
	require.Len(t, snap.Items, 1)
	require.Nil(t, snap.Items[0].PathSummary)
	require.Equal(t, "", snap.Items[0].CounterMode)
	require.Nil(t, snap.Items[0].ScopeUserID)
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

// TestSnapshot_PerUserMode_NoUserFilter_EmitsPlaceholder 验证新行为：
// admin 不带 user filter 也会 emit 一条占位行（PerUserUnbound=true、Exists=false、
// ScopeUserID=nil），让前端能展示规则并提示用户选择具体用户查看实时计数。
//
// 同时验证 SnapshotMany 不会误命中已有的 shared 计数器：fake limiter 预置一个
// shared key 的高用量值，占位行返回的 Current 仍必须是 0。
func TestSnapshot_PerUserMode_NoUserFilter_EmitsPlaceholder(t *testing.T) {
	rule := ruleSimple(1, ServiceQuotaCounterModePerUser, ServiceQuotaLimiterRPM, 100, nil)
	limiter := newMonitorFakeLimiter()
	// 预置 shared 风格的 key，验证哨兵 key 不会与之冲突
	sharedKey := BuildServiceQuotaCounterKey(rule.ID, rule.Paths[0].ID, ServiceQuotaLimiterRPM, nil)
	limiter.snapshots[sharedKey] = LimiterSnapshot{Current: 999, Exists: true}
	svc := newMonitorService(t, true, []*ServiceQuotaRule{rule}, limiter)

	snap, err := svc.Snapshot(context.Background(), MonitorSnapshotFilter{})
	require.NoError(t, err)
	require.Len(t, snap.Items, 1)
	require.True(t, snap.Items[0].PerUserUnbound)
	require.False(t, snap.Items[0].Exists)
	require.Equal(t, 0.0, snap.Items[0].Current)
	require.Nil(t, snap.Items[0].ScopeUserID)
}

// TestSnapshot_PerUserMode_WithUserFilter_NotPlaceholder 验证：
// admin 提供 user_id filter 时仍按真实 user 展开，不打 PerUserUnbound 标志。
func TestSnapshot_PerUserMode_WithUserFilter_NotPlaceholder(t *testing.T) {
	rule := ruleSimple(1, ServiceQuotaCounterModePerUser, ServiceQuotaLimiterRPM, 100, nil)
	svc := newMonitorService(t, true, []*ServiceQuotaRule{rule}, nil)
	snap, err := svc.Snapshot(context.Background(), MonitorSnapshotFilter{UserID: ptrInt64Monitor(7)})
	require.NoError(t, err)
	require.Len(t, snap.Items, 1)
	require.False(t, snap.Items[0].PerUserUnbound)
	require.NotNil(t, snap.Items[0].ScopeUserID)
	require.Equal(t, int64(7), *snap.Items[0].ScopeUserID)
}

// TestBuildSnapshotKeys_PerUserUnbound_UsesSentinelKey 验证占位行生成的 key
// 包含 _per_user_unbound 后缀，从而与真实计数 key 不冲突。
func TestBuildSnapshotKeys_PerUserUnbound_UsesSentinelKey(t *testing.T) {
	rule := ruleSimple(42, ServiceQuotaCounterModePerUser, ServiceQuotaLimiterRPM, 100, nil)
	rows := []plannedRow{{
		rule:           rule,
		path:           rule.Paths[0],
		pathIndex:      1,
		limiter:        rule.Limiters[0],
		scopeUserID:    nil,
		perUserUnbound: true,
	}}
	keys := buildSnapshotKeys(rows)
	require.Len(t, keys, 1)
	require.Contains(t, keys[0].Key, "_per_user_unbound")
	// 与真实 shared key 不一致
	require.NotEqual(t, BuildServiceQuotaCounterKey(rule.ID, rule.Paths[0].ID, ServiceQuotaLimiterRPM, nil), keys[0].Key)
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
