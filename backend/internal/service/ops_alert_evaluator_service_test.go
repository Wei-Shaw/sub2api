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
	overview            *OpsDashboardOverview
	classificationStats *OpsErrorClassificationStats
	err                 error
}

func (s *stubOpsRepo) GetErrorClassificationStats(ctx context.Context, filter *OpsDashboardFilter) (*OpsErrorClassificationStats, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.classificationStats != nil {
		return s.classificationStats, nil
	}
	return &OpsErrorClassificationStats{}, nil
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

func TestOpsAlertEmailUsesDurableDispatcherAndTransitionDedup(t *testing.T) {
	ctx := context.Background()
	settings := newNotificationEmailMemorySettingRepo()
	opsService := &OpsService{settingRepo: settings}
	_, err := opsService.UpdateEmailNotificationConfig(ctx, &OpsEmailNotificationConfigUpdateRequest{
		Alert: &OpsEmailAlertConfig{
			Enabled: true, Recipients: []string{"ops@example.com"}, MinSeverity: "warning",
			IncludeResolvedAlerts: true,
		},
	})
	require.NoError(t, err)

	deliveryRepo := newFakeNotificationEmailDeliveryRepository()
	dispatcher := NewNotificationEmailDispatcher(deliveryRepo, NewNotificationEmailService(settings, nil))
	svc := &OpsAlertEvaluatorService{opsService: opsService, notificationDispatcher: dispatcher}
	rule := &OpsAlertRule{ID: 9, Name: "availability", Severity: "P1", NotifyEmail: true, MetricType: "error_rate", Operator: ">", Threshold: 5}
	event := &OpsAlertEvent{ID: 42, RuleID: rule.ID, Status: OpsAlertStatusFiring, FiredAt: time.Now().UTC()}

	require.Equal(t, 1, svc.maybeEnqueueAlertEmail(ctx, nil, opsAlertEmailTransitionFiring, rule, event))
	require.Equal(t, 1, svc.maybeEnqueueAlertEmail(ctx, nil, opsAlertEmailTransitionFiring, rule, event), "existing deduplicated delivery still proves the transition is queued")

	event.Status = OpsAlertStatusResolved
	resolvedAt := time.Now().UTC()
	event.ResolvedAt = &resolvedAt
	require.Equal(t, 1, svc.maybeEnqueueAlertEmail(ctx, nil, opsAlertEmailTransitionResolved, rule, event))
	require.Len(t, deliveryRepo.items, 2)
	require.Equal(t, opsAlertEmailTransitionFiring, deliveryRepo.items[0].ReminderKey)
	require.Equal(t, opsAlertEmailTransitionResolved, deliveryRepo.items[1].ReminderKey)
	require.Equal(t, "ops_alert_event", deliveryRepo.items[0].SourceType)
}

func TestOpsAlertResolvedEmailRequiresFineGrainedSwitch(t *testing.T) {
	ctx := context.Background()
	settings := newNotificationEmailMemorySettingRepo()
	opsService := &OpsService{settingRepo: settings}
	_, err := opsService.UpdateEmailNotificationConfig(ctx, &OpsEmailNotificationConfigUpdateRequest{
		Alert: &OpsEmailAlertConfig{Enabled: true, Recipients: []string{"ops@example.com"}},
	})
	require.NoError(t, err)

	deliveryRepo := newFakeNotificationEmailDeliveryRepository()
	svc := &OpsAlertEvaluatorService{
		opsService: opsService,
		notificationDispatcher: NewNotificationEmailDispatcher(
			deliveryRepo,
			NewNotificationEmailService(settings, nil),
		),
	}
	rule := &OpsAlertRule{ID: 9, Severity: "P0", NotifyEmail: true}
	event := &OpsAlertEvent{ID: 42, RuleID: rule.ID, Status: OpsAlertStatusResolved}

	require.Zero(t, svc.maybeEnqueueAlertEmail(ctx, nil, opsAlertEmailTransitionResolved, rule, event))
	require.Empty(t, deliveryRepo.items)
}

func TestOpsAlertMetricRequiresMinimumSamplesAndBadCount(t *testing.T) {
	now := time.Now().UTC()
	rule := &OpsAlertRule{
		MetricType: "error_rate", Operator: ">=", Threshold: 20,
		MinimumSamples: 30, MinimumBadCount: 10,
	}
	svc := &OpsAlertEvaluatorService{opsRepo: &stubOpsRepo{overview: &OpsDashboardOverview{
		RequestCountSLA: 4, ErrorCountSLA: 1, ErrorRate: 0.25,
	}}}

	result := svc.evaluateRuleMetric(context.Background(), rule, nil, now.Add(-5*time.Minute), now, "", nil, now)
	require.Equal(t, OpsAlertEvaluationStatusOK, result.Status)
	require.False(t, result.Breached)
	require.EqualValues(t, 4, result.SampleCount)
	require.EqualValues(t, 1, result.BadCount)

	svc.opsRepo = &stubOpsRepo{overview: &OpsDashboardOverview{
		RequestCountSLA: 50, ErrorCountSLA: 12, ErrorRate: 0.24,
	}}
	result = svc.evaluateRuleMetric(context.Background(), rule, nil, now.Add(-5*time.Minute), now, "", nil, now)
	require.Equal(t, OpsAlertEvaluationStatusBreached, result.Status)
	require.True(t, result.Breached)
}

func TestOpsAlertMetricDistinguishesStaleAndUnsupported(t *testing.T) {
	now := time.Now().UTC()
	cpu := 90.0
	svc := &OpsAlertEvaluatorService{opsRepo: &stubOpsRepo{}}

	stale := svc.evaluateRuleMetric(context.Background(), &OpsAlertRule{
		MetricType: "cpu_usage_percent", Operator: ">", Threshold: 85,
	}, &OpsSystemMetricsSnapshot{CreatedAt: now.Add(-10 * time.Minute), CPUUsagePercent: &cpu}, now.Add(-5*time.Minute), now, "", nil, now)
	require.Equal(t, OpsAlertEvaluationStatusStale, stale.Status)
	require.False(t, stale.Breached)

	unsupported := svc.evaluateRuleMetric(context.Background(), &OpsAlertRule{
		MetricType: "p99_latency_ms", Operator: ">", Threshold: 3000,
	}, nil, now.Add(-5*time.Minute), now, "", nil, now)
	require.Equal(t, OpsAlertEvaluationStatusUnsupported, unsupported.Status)
	require.Equal(t, "unsupported_metric", unsupported.ErrorCode)
}

func TestResetAlertRuleStateAfterEvaluationGap(t *testing.T) {
	now := time.Now().UTC()
	last := now.Add(-3 * time.Minute)
	state := &OpsAlertRuleState{
		RuleID: 1, LastEvaluatedAt: &last, ConsecutiveBreaches: 3, ConsecutiveRecoveries: 2,
	}
	resetAlertRuleStateAfterGap(state, now, time.Minute)
	require.Zero(t, state.ConsecutiveBreaches)
	require.Zero(t, state.ConsecutiveRecoveries)
}
