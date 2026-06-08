//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var _ OpsRepository = (*stubOpsRepo)(nil)

type stubOpsRepo struct {
	OpsRepository
	overview *OpsDashboardOverview
	err      error
}

func (s *stubOpsRepo) GetDashboardOverview(ctx context.Context, filter *OpsDashboardFilter) (*OpsDashboardOverview, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.overview != nil {
		return s.overview, nil
	}
	return &OpsDashboardOverview{}, nil
}

func TestComputeGroupAvailableRatio(t *testing.T) {
	t.Parallel()

	t.Run("正常情况: 10个账号, 8个可用 = 80%", func(t *testing.T) {
		t.Parallel()

		got := computeGroupAvailableRatio(&GroupAvailability{
			TotalAccounts:  10,
			AvailableCount: 8,
		})
		require.InDelta(t, 80.0, got, 0.0001)
	})

	t.Run("边界情况: TotalAccounts = 0 应返回 0", func(t *testing.T) {
		t.Parallel()

		got := computeGroupAvailableRatio(&GroupAvailability{
			TotalAccounts:  0,
			AvailableCount: 8,
		})
		require.Equal(t, 0.0, got)
	})

	t.Run("边界情况: AvailableCount = 0 应返回 0%", func(t *testing.T) {
		t.Parallel()

		got := computeGroupAvailableRatio(&GroupAvailability{
			TotalAccounts:  10,
			AvailableCount: 0,
		})
		require.Equal(t, 0.0, got)
	})
}

func TestCountAccountsByCondition(t *testing.T) {
	t.Parallel()

	t.Run("测试限流账号统计: acc.IsRateLimited", func(t *testing.T) {
		t.Parallel()

		accounts := map[int64]*AccountAvailability{
			1: {IsRateLimited: true},
			2: {IsRateLimited: false},
			3: {IsRateLimited: true},
		}

		got := countAccountsByCondition(accounts, func(acc *AccountAvailability) bool {
			return acc.IsRateLimited
		})
		require.Equal(t, int64(2), got)
	})

	t.Run("测试错误账号统计（排除临时不可调度）: acc.HasError && acc.TempUnschedulableUntil == nil", func(t *testing.T) {
		t.Parallel()

		until := time.Now().UTC().Add(5 * time.Minute)
		accounts := map[int64]*AccountAvailability{
			1: {HasError: true},
			2: {HasError: true, TempUnschedulableUntil: &until},
			3: {HasError: false},
		}

		got := countAccountsByCondition(accounts, func(acc *AccountAvailability) bool {
			return acc.HasError && acc.TempUnschedulableUntil == nil
		})
		require.Equal(t, int64(1), got)
	})

	t.Run("边界情况: 空 map 应返回 0", func(t *testing.T) {
		t.Parallel()

		got := countAccountsByCondition(map[int64]*AccountAvailability{}, func(acc *AccountAvailability) bool {
			return acc.IsRateLimited
		})
		require.Equal(t, int64(0), got)
	})
}

// TestComputeRuleMetric_AccountTempUnscheduledCount verifies the new
// account_temp_unscheduled_count metric counts accounts currently in the
// temp-unscheduled window and ignores those whose window has expired or
// were never temp-unscheduled.
func TestComputeRuleMetric_AccountTempUnscheduledCount(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	futureUntil := now.Add(5 * time.Minute)
	pastUntil := now.Add(-1 * time.Minute)

	availability := &OpsAccountAvailability{
		Accounts: map[int64]*AccountAvailability{
			// currently temp-unscheduled (window active)
			1: {TempUnschedulableUntil: &futureUntil},
			2: {TempUnschedulableUntil: &futureUntil},
			// temp-unsched window already expired → should NOT count
			3: {TempUnschedulableUntil: &pastUntil},
			// never temp-unscheduled
			4: {HasError: true},
			5: {IsRateLimited: true},
		},
	}

	opsService := &OpsService{
		getAccountAvailability: func(_ context.Context, _ string, _ *int64) (*OpsAccountAvailability, error) {
			return availability, nil
		},
	}
	svc := &OpsAlertEvaluatorService{
		opsService: opsService,
		opsRepo:    &stubOpsRepo{},
	}

	rule := &OpsAlertRule{MetricType: "account_temp_unscheduled_count"}
	val, ok := svc.computeRuleMetric(context.Background(), rule, nil,
		now.Add(-5*time.Minute), now, "", nil)

	require.True(t, ok)
	require.InDelta(t, 2.0, val, 0.0001, "only 2 accounts have an active temp-unsched window")
}

func TestComputeRuleMetricNewIndicators(t *testing.T) {
	t.Parallel()

	groupID := int64(101)
	platform := "openai"

	availability := &OpsAccountAvailability{
		Group: &GroupAvailability{
			GroupID:        groupID,
			TotalAccounts:  10,
			AvailableCount: 8,
		},
		Accounts: map[int64]*AccountAvailability{
			1: {IsRateLimited: true},
			2: {IsRateLimited: true},
			3: {HasError: true},
			4: {HasError: true, TempUnschedulableUntil: timePtr(time.Now().UTC().Add(2 * time.Minute))},
			5: {HasError: false, IsRateLimited: false},
		},
	}

	opsService := &OpsService{
		getAccountAvailability: func(_ context.Context, _ string, _ *int64) (*OpsAccountAvailability, error) {
			return availability, nil
		},
	}

	svc := &OpsAlertEvaluatorService{
		opsService: opsService,
		opsRepo:    &stubOpsRepo{overview: &OpsDashboardOverview{}},
	}

	start := time.Now().UTC().Add(-5 * time.Minute)
	end := time.Now().UTC()
	ctx := context.Background()

	tests := []struct {
		name       string
		metricType string
		groupID    *int64
		wantValue  float64
		wantOK     bool
	}{
		{
			name:       "group_available_accounts",
			metricType: "group_available_accounts",
			groupID:    &groupID,
			wantValue:  8,
			wantOK:     true,
		},
		{
			name:       "group_available_ratio",
			metricType: "group_available_ratio",
			groupID:    &groupID,
			wantValue:  80.0,
			wantOK:     true,
		},
		{
			name:       "account_rate_limited_count",
			metricType: "account_rate_limited_count",
			groupID:    nil,
			wantValue:  2,
			wantOK:     true,
		},
		{
			name:       "account_error_count",
			metricType: "account_error_count",
			groupID:    nil,
			wantValue:  1,
			wantOK:     true,
		},
		{
			name:       "group_available_accounts without group_id returns false",
			metricType: "group_available_accounts",
			groupID:    nil,
			wantValue:  0,
			wantOK:     false,
		},
		{
			name:       "group_available_ratio without group_id returns false",
			metricType: "group_available_ratio",
			groupID:    nil,
			wantValue:  0,
			wantOK:     false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rule := &OpsAlertRule{
				MetricType: tt.metricType,
			}
			gotValue, gotOK := svc.computeRuleMetric(ctx, rule, nil, start, end, platform, tt.groupID)
			require.Equal(t, tt.wantOK, gotOK)
			if !tt.wantOK {
				return
			}
			require.InDelta(t, tt.wantValue, gotValue, 0.0001)
		})
	}
}

func TestOpsAlertI18NNormalizesLegacyChineseNames(t *testing.T) {
	t.Parallel()

	rules := normalizeDefaultOpsAlertRules([]*OpsAlertRule{
		{Name: "错误率过高", Description: "当错误率超过 5% 且持续 5 分钟时触发告警", Severity: "P1"},
		{Name: "成功率过低", Description: "", Severity: "P0"},
	})
	require.Equal(t, "Tỷ lệ lỗi cao", rules[0].Name)
	require.Equal(t, "Tỷ lệ thành công thấp", rules[1].Name)
	require.False(t, containsCJK(rules[0].Description))
	require.Equal(t, "P1: Tỷ lệ lỗi cao", buildOpsAlertTitle(rules[0]))

	events := normalizeDefaultOpsAlertEvents([]*OpsAlertEvent{
		{Title: "P0: 错误率极高", Description: "错误率极高"},
	})
	require.Equal(t, "P0: Tỷ lệ lỗi cực cao", events[0].Title)
	require.Equal(t, "Tỷ lệ lỗi cực cao", events[0].Description)
}

func TestAccountQuotaUsagePercentHelpers(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	accounts := []Account{
		{
			ID:       1,
			Name:     "safe",
			Platform: PlatformAnthropic,
			Type:     AccountTypeAPIKey,
			Status:   StatusActive,
			Extra: map[string]any{
				"quota_limit": 1000.0,
				"quota_used":  650.0,
			},
		},
		{
			ID:       2,
			Name:     "near-empty",
			Platform: PlatformAnthropic,
			Type:     AccountTypeAPIKey,
			Status:   StatusActive,
			Extra: map[string]any{
				"quota_daily_limit": 1000.0,
				"quota_daily_used":  800.0,
				"quota_daily_start": now.Format(time.RFC3339),
			},
		},
		{
			ID:       3,
			Name:     "oauth-no-quota",
			Platform: PlatformAnthropic,
			Type:     AccountTypeOAuth,
			Status:   StatusActive,
			Extra: map[string]any{
				"quota_limit": 1000.0,
				"quota_used":  1000.0,
			},
		},
	}

	got := maxAccountQuotaUsagePercent(accounts)
	require.InDelta(t, 80.0, got, 0.0001)

	percent, ok := accountMaxQuotaUsagePercent(accounts[0])
	require.True(t, ok)
	require.InDelta(t, 65.0, percent, 0.0001)
}

func TestComputeRuleMetricAccountInventoryAndQuota(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	groupID := int64(42)
	accounts := []Account{
		{
			ID:          1,
			Name:        "available-quota-70",
			Platform:    PlatformAnthropic,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			GroupIDs:    []int64{groupID},
			Extra: map[string]any{
				"quota_limit": 1000.0,
				"quota_used":  700.0,
			},
		},
		{
			ID:          2,
			Name:        "exhausted-daily",
			Platform:    PlatformAnthropic,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			GroupIDs:    []int64{groupID},
			Extra: map[string]any{
				"quota_daily_limit": 1000.0,
				"quota_daily_used":  1000.0,
				"quota_daily_start": now.Format(time.RFC3339),
			},
		},
		{
			ID:          3,
			Name:        "disabled-other-group",
			Platform:    PlatformAnthropic,
			Type:        AccountTypeAPIKey,
			Status:      StatusDisabled,
			Schedulable: true,
			GroupIDs:    []int64{99},
			Extra: map[string]any{
				"quota_limit": 1000.0,
				"quota_used":  950.0,
			},
		},
	}

	svc := &OpsAlertEvaluatorService{
		opsService: &OpsService{
			listAccountsForOpsHook: func(_ context.Context, platformFilter string, groupIDFilter *int64) ([]Account, error) {
				return accounts, nil
			},
		},
		opsRepo: &stubOpsRepo{overview: &OpsDashboardOverview{}},
	}
	ctx := context.Background()
	start := now.Add(-5 * time.Minute)

	tests := []struct {
		metric string
		group  *int64
		want   float64
	}{
		{metric: "account_available_count", group: &groupID, want: 1},
		{metric: "account_available_ratio", group: &groupID, want: 50},
		{metric: "account_quota_usage_ratio", group: &groupID, want: 100},
		{metric: "account_quota_exhausted_count", group: &groupID, want: 1},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.metric, func(t *testing.T) {
			t.Parallel()
			got, ok := svc.computeRuleMetric(ctx, &OpsAlertRule{MetricType: tt.metric}, nil, start, now, PlatformAnthropic, tt.group)
			require.True(t, ok)
			require.InDelta(t, tt.want, got, 0.0001)
		})
	}
}

func TestComputeRuleMetricLatencyPercentiles(t *testing.T) {
	t.Parallel()

	p95 := 2100
	p99 := 3200
	svc := &OpsAlertEvaluatorService{
		opsRepo: &stubOpsRepo{overview: &OpsDashboardOverview{
			RequestCountSLA: 10,
			Duration: OpsPercentiles{
				P95: &p95,
				P99: &p99,
			},
		}},
	}
	ctx := context.Background()
	now := time.Now().UTC()

	gotP95, ok := svc.computeRuleMetric(ctx, &OpsAlertRule{MetricType: "p95_latency_ms"}, nil, now.Add(-5*time.Minute), now, "", nil)
	require.True(t, ok)
	require.Equal(t, float64(2100), gotP95)

	gotP99, ok := svc.computeRuleMetric(ctx, &OpsAlertRule{MetricType: "p99_latency_ms"}, nil, now.Add(-5*time.Minute), now, "", nil)
	require.True(t, ok)
	require.Equal(t, float64(3200), gotP99)
}
