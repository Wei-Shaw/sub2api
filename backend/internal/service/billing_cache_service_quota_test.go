//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// quotaCacheStub 带状态的 BillingCache mock，用于模拟 quota 读取返回值
//
// 通过嵌入 billingCacheWorkerStub（定义在同 package 的 billing_cache_service_test.go）
// 复用所有非 quota 相关的 BillingCache 接口方法默认实现，仅覆盖本测试关心的 Get*Used 方法。
type quotaCacheStub struct {
	billingCacheWorkerStub
	totalUsed   float64
	ruleUsed    map[int64]float64
	getTotalErr error
	getRuleErr  error
}

func (q *quotaCacheStub) GetQuotaUsedTotal(context.Context, int64, string) (float64, error) {
	if q.getTotalErr != nil {
		return 0, q.getTotalErr
	}
	return q.totalUsed, nil
}

func (q *quotaCacheStub) GetQuotaUsedRule(_ context.Context, _, ruleID int64, _ string) (float64, error) {
	if q.getRuleErr != nil {
		return 0, q.getRuleErr
	}
	if q.ruleUsed == nil {
		return 0, nil
	}
	return q.ruleUsed[ruleID], nil
}

// quotaServiceFake 用于 checkQuotaEligibility 测试：直接返回预置 ResolvedQuota
type quotaServiceFake struct {
	resolved *ResolvedQuota
	err      error
}

func (f *quotaServiceFake) Resolve(context.Context, int64) (*ResolvedQuota, error) {
	return f.resolved, f.err
}
func (f *quotaServiceFake) MatchRule(r *ResolvedQuota, groupID int64) *QuotaRule {
	if r == nil {
		return nil
	}
	for i := range r.Rules {
		for _, g := range r.Rules[i].GroupIDs {
			if g == groupID {
				return &r.Rules[i]
			}
		}
	}
	return nil
}
func (f *quotaServiceFake) GetUserQuota(context.Context, int64) (*UserQuotaView, error) {
	return nil, nil
}
func (f *quotaServiceFake) UpdateUserQuota(context.Context, int64, UpdateUserQuotaRequest) error {
	return nil
}
func (f *quotaServiceFake) ListRules(context.Context, int64) ([]*QuotaRule, error) { return nil, nil }
func (f *quotaServiceFake) CreateRule(context.Context, int64, CreateRuleRequest) (*QuotaRule, error) {
	return nil, nil
}
func (f *quotaServiceFake) UpdateRule(context.Context, int64, int64, UpdateRuleRequest) (*QuotaRule, error) {
	return nil, nil
}
func (f *quotaServiceFake) DeleteRule(context.Context, int64, int64) error { return nil }
func (f *quotaServiceFake) ReplaceUserRules(context.Context, int64, []CreateRuleRequest) ([]*QuotaRule, error) {
	return nil, nil
}
func (f *quotaServiceFake) GetTodayUsage(context.Context, int64, *ResolvedQuota) (*QuotaUsageSnapshot, error) {
	return nil, nil
}

func newQuotaBillingCacheService(t *testing.T, cache BillingCache, qs QuotaService) *BillingCacheService {
	t.Helper()
	svc := NewBillingCacheService(cache, nil, nil, nil, &config.Config{})
	svc.SetQuotaService(qs)
	t.Cleanup(svc.Stop)
	return svc
}

func TestCheckQuotaEligibility_NotEnabledShortCircuits(t *testing.T) {
	cache := &quotaCacheStub{}
	qs := &quotaServiceFake{resolved: &ResolvedQuota{Enabled: false}}
	svc := newQuotaBillingCacheService(t, cache, qs)
	err := svc.checkQuotaEligibility(context.Background(), 1, nil)
	assert.NoError(t, err)
}

func TestCheckQuotaEligibility_TotalExceeded(t *testing.T) {
	limit := 10.0
	cache := &quotaCacheStub{totalUsed: 11.0}
	qs := &quotaServiceFake{resolved: &ResolvedQuota{Enabled: true, DailyLimit: &limit}}
	svc := newQuotaBillingCacheService(t, cache, qs)
	err := svc.checkQuotaEligibility(context.Background(), 1, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrQuotaExceeded))
}

func TestCheckQuotaEligibility_RuleExceeded(t *testing.T) {
	cache := &quotaCacheStub{ruleUsed: map[int64]float64{5: 3}}
	qs := &quotaServiceFake{resolved: &ResolvedQuota{
		Enabled: true,
		Rules: []QuotaRule{
			{ID: 5, GroupIDs: []int64{100}, DailyLimitUSD: 1.0},
		},
	}}
	svc := newQuotaBillingCacheService(t, cache, qs)
	err := svc.checkQuotaEligibility(context.Background(), 1, &Group{ID: 100})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrQuotaExceeded))
}

func TestCheckQuotaEligibility_FailOpenOnRedisError(t *testing.T) {
	limit := 10.0
	cache := &quotaCacheStub{getTotalErr: errors.New("boom")}
	qs := &quotaServiceFake{resolved: &ResolvedQuota{Enabled: true, DailyLimit: &limit}}
	svc := newQuotaBillingCacheService(t, cache, qs)
	err := svc.checkQuotaEligibility(context.Background(), 1, nil)
	// fail open：Redis 错误必须放行
	assert.NoError(t, err)
}

func TestCheckQuotaEligibility_FailOpenOnResolveError(t *testing.T) {
	qs := &quotaServiceFake{err: errors.New("db down")}
	svc := newQuotaBillingCacheService(t, &quotaCacheStub{}, qs)
	err := svc.checkQuotaEligibility(context.Background(), 1, nil)
	assert.NoError(t, err)
}

func TestCheckQuotaEligibility_UnderLimit(t *testing.T) {
	limit := 10.0
	cache := &quotaCacheStub{totalUsed: 5.0}
	qs := &quotaServiceFake{resolved: &ResolvedQuota{Enabled: true, DailyLimit: &limit}}
	svc := newQuotaBillingCacheService(t, cache, qs)
	err := svc.checkQuotaEligibility(context.Background(), 1, nil)
	assert.NoError(t, err)
}

func TestCheckQuotaEligibility_QuotaServiceNilShortCircuits(t *testing.T) {
	svc := NewBillingCacheService(&quotaCacheStub{}, nil, nil, nil, &config.Config{})
	t.Cleanup(svc.Stop)
	err := svc.checkQuotaEligibility(context.Background(), 1, nil)
	assert.NoError(t, err)
}
