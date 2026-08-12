//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/web3deposit"
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

func TestComputeRuleMetricWeb3DepositIndicators(t *testing.T) {
	restore := web3deposit.SetRuntimeMetricsForTest(web3deposit.RuntimeMetricsSnapshot{
		RPCHealthy:         false,
		ScannerLagBlocks:   123,
		FinalizerLagBlocks: 45,
		CreditFailures:     2,
	})
	defer restore()

	statusCounter := &web3DepositStatusCounterStub{
		counts: map[web3deposit.DepositStatus]int64{
			web3deposit.DepositStatusManualReview: 11,
		},
	}
	svc := &OpsAlertEvaluatorService{
		opsRepo:      &stubOpsRepo{overview: &OpsDashboardOverview{}},
		web3Deposits: statusCounter,
		cfg:          web3AlertTestConfig(true),
	}

	now := time.Now().UTC()
	tests := []struct {
		metricType string
		want       float64
	}{
		{metricType: "web3_rpc_unhealthy", want: 1},
		{metricType: "web3_scanner_lag_blocks", want: 123},
		{metricType: "web3_finalizer_lag_blocks", want: 45},
		{metricType: "web3_credit_failures_total", want: 0},
		{metricType: "web3_manual_review_count", want: 11},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.metricType, func(t *testing.T) {
			got, ok := svc.computeRuleMetric(context.Background(), &OpsAlertRule{MetricType: tt.metricType}, nil, now.Add(-time.Minute), now, "", nil)
			require.True(t, ok)
			require.InDelta(t, tt.want, got, 0.0001)
		})
	}
}

func TestComputeRuleMetricUsesLiveWeb3RPCHealth(t *testing.T) {
	restore := web3deposit.SetRuntimeMetricsForTest(web3deposit.RuntimeMetricsSnapshot{RPCHealthy: true})
	defer restore()
	svc := &OpsAlertEvaluatorService{
		cfg:               web3AlertTestConfig(true),
		web3RuntimeHealth: web3RuntimeHealthStub(false),
	}
	now := time.Now().UTC()

	got, ok := svc.computeRuleMetric(context.Background(), &OpsAlertRule{MetricType: "web3_rpc_unhealthy"}, nil, now.Add(-time.Minute), now, "", nil)

	require.True(t, ok)
	require.Equal(t, float64(1), got)
}

func TestComputeRuleMetricUsesWindowedWeb3CreditFailureDelta(t *testing.T) {
	restoreInitial := web3deposit.SetRuntimeMetricsForTest(web3deposit.RuntimeMetricsSnapshot{CreditFailures: 2})
	defer restoreInitial()
	svc := &OpsAlertEvaluatorService{cfg: web3AlertTestConfig(true)}
	rule := &OpsAlertRule{MetricType: "web3_credit_failures_total"}
	base := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)

	first, ok := svc.computeRuleMetric(context.Background(), rule, nil, base.Add(-time.Minute), base, "", nil)
	require.True(t, ok)
	require.Zero(t, first)

	restoreIncrement := web3deposit.SetRuntimeMetricsForTest(web3deposit.RuntimeMetricsSnapshot{CreditFailures: 5})
	defer restoreIncrement()
	second, ok := svc.computeRuleMetric(context.Background(), rule, nil, base, base.Add(time.Minute), "", nil)
	require.True(t, ok)
	require.Equal(t, float64(3), second)

	third, ok := svc.computeRuleMetric(context.Background(), rule, nil, base.Add(time.Minute), base.Add(2*time.Minute), "", nil)
	require.True(t, ok)
	require.Zero(t, third)
}

func TestComputeRuleMetricWeb3DepositIndicatorsDisabled(t *testing.T) {
	t.Parallel()

	svc := &OpsAlertEvaluatorService{
		opsRepo: &stubOpsRepo{overview: &OpsDashboardOverview{}},
		cfg:     web3AlertTestConfig(false),
	}

	now := time.Now().UTC()
	got, ok := svc.computeRuleMetric(context.Background(), &OpsAlertRule{MetricType: "web3_rpc_unhealthy"}, nil, now.Add(-time.Minute), now, "", nil)
	require.False(t, ok)
	require.Zero(t, got)
}

func web3AlertTestConfig(enabled bool) *config.Config {
	return &config.Config{Web3Deposit: config.Web3DepositConfig{
		Enabled: enabled,
		Networks: map[string]config.Web3DepositNetworkConfig{
			"conflux": {
				Enabled: true,
				Assets: map[string]config.Web3DepositAssetConfig{
					"usdt0": {ContractAddress: "0xaf37e8b6c9ed7f6318979f56fc287d76c30847ff"},
				},
			},
		},
	}}
}

type web3DepositStatusCounterStub struct {
	counts map[web3deposit.DepositStatus]int64
	err    error
}

type web3RuntimeHealthStub bool

func (s web3RuntimeHealthStub) AllReady() bool { return bool(s) }

func (s *web3DepositStatusCounterStub) ListAdminDeposits(context.Context, web3deposit.AdminDepositFilter) ([]web3deposit.Deposit, int64, error) {
	return nil, 0, nil
}

func (s *web3DepositStatusCounterStub) GetAdminDeposit(context.Context, int64) (web3deposit.Deposit, error) {
	return web3deposit.Deposit{}, nil
}

func (s *web3DepositStatusCounterStub) CountAdminDepositsByStatus(context.Context) (map[web3deposit.DepositStatus]int64, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.counts, nil
}

func (s *web3DepositStatusCounterStub) CountAdminDepositsByStatusForTarget(context.Context, uint64, string) (map[web3deposit.DepositStatus]int64, error) {
	return s.CountAdminDepositsByStatus(context.Background())
}
