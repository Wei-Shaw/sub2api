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
	ttft                *OpsTTFTSummary
	classificationStats *OpsErrorClassificationStats
	err                 error
	ttftErr             error
	ttftCalls           int
}

func (s *stubOpsRepo) GetTTFTPercentiles(ctx context.Context, filter *OpsDashboardFilter) (*OpsTTFTSummary, error) {
	s.ttftCalls++
	if s.ttftErr != nil {
		return nil, s.ttftErr
	}
	if s.ttft != nil {
		return s.ttft, nil
	}
	return &OpsTTFTSummary{}, nil
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
	require.Zero(t, svc.maybeEnqueueAlertEmail(ctx, nil, opsAlertEmailTransitionFiring, rule, event), "an existing deduplicated delivery is not newly queued")

	event.Status = OpsAlertStatusResolved
	resolvedAt := time.Now().UTC()
	event.ResolvedAt = &resolvedAt
	require.Equal(t, 1, svc.maybeEnqueueAlertEmail(ctx, nil, opsAlertEmailTransitionResolved, rule, event))
	require.Len(t, deliveryRepo.items, 2)
	require.Contains(t, deliveryRepo.items[0].ReminderKey, opsAlertEmailTransitionFiring)
	require.Contains(t, deliveryRepo.items[1].ReminderKey, opsAlertEmailTransitionResolved)
	require.Equal(t, "ops_alert_event", deliveryRepo.items[0].SourceType)
}

func TestOpsAlertEmailEnforcesPerRecipientHourlyLimit(t *testing.T) {
	ctx := context.Background()
	settings := newNotificationEmailMemorySettingRepo()
	opsService := &OpsService{settingRepo: settings}
	_, err := opsService.UpdateEmailNotificationConfig(ctx, &OpsEmailNotificationConfigUpdateRequest{
		Alert: &OpsEmailAlertConfig{
			Enabled: true, Recipients: []string{"ops@example.com"},
			RateLimitPerHour: 1,
		},
	})
	require.NoError(t, err)

	deliveryRepo := newFakeNotificationEmailDeliveryRepository()
	svc := &OpsAlertEvaluatorService{
		opsService:             opsService,
		notificationDispatcher: NewNotificationEmailDispatcher(deliveryRepo, NewNotificationEmailService(settings, nil)),
	}
	rule := &OpsAlertRule{ID: 9, Name: "availability", Severity: "P0", NotifyEmail: true}
	first := &OpsAlertEvent{ID: 41, RuleID: rule.ID, Status: OpsAlertStatusFiring, FiredAt: time.Now().UTC()}
	second := &OpsAlertEvent{ID: 42, RuleID: rule.ID, Status: OpsAlertStatusFiring, FiredAt: time.Now().UTC()}

	require.Equal(t, 1, svc.maybeEnqueueAlertEmail(ctx, nil, opsAlertEmailTransitionFiring, rule, first))
	require.Zero(t, svc.maybeEnqueueAlertEmail(ctx, nil, opsAlertEmailTransitionFiring, rule, second))
	require.Len(t, deliveryRepo.items, 1)
}

func TestOpsAlertEmailMergesSameIncidentWithinConfiguredWindow(t *testing.T) {
	ctx := context.Background()
	settings := newNotificationEmailMemorySettingRepo()
	opsService := &OpsService{settingRepo: settings}
	_, err := opsService.UpdateEmailNotificationConfig(ctx, &OpsEmailNotificationConfigUpdateRequest{
		Alert: &OpsEmailAlertConfig{
			Enabled: true, Recipients: []string{"ops@example.com"},
			BatchingWindowSeconds: 300,
		},
	})
	require.NoError(t, err)

	deliveryRepo := newFakeNotificationEmailDeliveryRepository()
	svc := &OpsAlertEvaluatorService{
		opsService:             opsService,
		notificationDispatcher: NewNotificationEmailDispatcher(deliveryRepo, NewNotificationEmailService(settings, nil)),
	}
	rule := &OpsAlertRule{ID: 9, IncidentFamily: "availability", Severity: "P0", NotifyEmail: true}
	first := &OpsAlertEvent{ID: 41, RuleID: rule.ID, Status: OpsAlertStatusFiring, FiredAt: time.Now().UTC()}
	second := &OpsAlertEvent{ID: 42, RuleID: rule.ID, Status: OpsAlertStatusFiring, FiredAt: time.Now().UTC()}

	require.Equal(t, 1, svc.maybeEnqueueAlertEmail(ctx, nil, opsAlertEmailTransitionFiring, rule, first))
	require.Zero(t, svc.maybeEnqueueAlertEmail(ctx, nil, opsAlertEmailTransitionFiring, rule, second))
	require.Len(t, deliveryRepo.items, 1)
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

	result := svc.evaluateRuleMetric(context.Background(), rule, nil, now.Add(-5*time.Minute), now, "", nil, now, nil)
	require.Equal(t, OpsAlertEvaluationStatusOK, result.Status)
	require.False(t, result.Breached)
	require.EqualValues(t, 4, result.SampleCount)
	require.EqualValues(t, 1, result.BadCount)

	svc.opsRepo = &stubOpsRepo{overview: &OpsDashboardOverview{
		RequestCountSLA: 50, ErrorCountSLA: 12, ErrorRate: 0.24,
	}}
	result = svc.evaluateRuleMetric(context.Background(), rule, nil, now.Add(-5*time.Minute), now, "", nil, now, nil)
	require.Equal(t, OpsAlertEvaluationStatusBreached, result.Status)
	require.True(t, result.Breached)
}

func TestOpsAlertMetricDistinguishesStaleAndUnsupported(t *testing.T) {
	now := time.Now().UTC()
	cpu := 90.0
	svc := &OpsAlertEvaluatorService{opsRepo: &stubOpsRepo{}}

	stale := svc.evaluateRuleMetric(context.Background(), &OpsAlertRule{
		MetricType: "cpu_usage_percent", Operator: ">", Threshold: 85,
	}, &OpsSystemMetricsSnapshot{CreatedAt: now.Add(-10 * time.Minute), CPUUsagePercent: &cpu}, now.Add(-5*time.Minute), now, "", nil, now, nil)
	require.Equal(t, OpsAlertEvaluationStatusStale, stale.Status)
	require.False(t, stale.Breached)

	unsupported := svc.evaluateRuleMetric(context.Background(), &OpsAlertRule{
		MetricType: "p99_latency_ms", Operator: ">", Threshold: 3000,
	}, nil, now.Add(-5*time.Minute), now, "", nil, now, nil)
	require.Equal(t, OpsAlertEvaluationStatusUnsupported, unsupported.Status)
	require.Equal(t, "unsupported_metric", unsupported.ErrorCode)
}

func TestOpsAlertMetricEvaluatesTTFTPercentilesAndMaxInSeconds(t *testing.T) {
	now := time.Now().UTC()
	p95, p99, max := 2400, 5100, 12750
	svc := &OpsAlertEvaluatorService{opsRepo: &stubOpsRepo{ttft: &OpsTTFTSummary{
		SampleCount: 37,
		TTFT:        OpsPercentiles{P95: &p95, P99: &p99, Max: &max},
	}}}

	for _, tt := range []struct {
		metric    string
		threshold float64
		wantValue float64
		breached  bool
	}{
		{metric: "ttft_p95_seconds", threshold: 3, wantValue: 2.4, breached: false},
		{metric: "ttft_p99_seconds", threshold: 5, wantValue: 5.1, breached: true},
		{metric: "ttft_max_seconds", threshold: 10, wantValue: 12.75, breached: true},
	} {
		t.Run(tt.metric, func(t *testing.T) {
			result := svc.evaluateRuleMetric(context.Background(), &OpsAlertRule{
				MetricType: tt.metric, Operator: ">", Threshold: tt.threshold, MinimumSamples: 20,
			}, nil, now.Add(-5*time.Minute), now, "", nil, now, nil)
			require.Equal(t, tt.breached, result.Breached)
			require.NotNil(t, result.Value)
			require.InDelta(t, tt.wantValue, *result.Value, 0.0001)
			require.Equal(t, int64(37), result.SampleCount)
		})
	}
}

func TestOpsAlertMetricTTFTRequiresRealTTFTSamples(t *testing.T) {
	now := time.Now().UTC()
	p99 := 5000
	svc := &OpsAlertEvaluatorService{opsRepo: &stubOpsRepo{ttft: &OpsTTFTSummary{
		SampleCount: 0, TTFT: OpsPercentiles{P99: &p99},
	}}}
	result := svc.evaluateRuleMetric(context.Background(), &OpsAlertRule{
		MetricType: "ttft_p99_seconds", Operator: ">", Threshold: 3,
	}, nil, now.Add(-5*time.Minute), now, "", nil, now, nil)
	require.Equal(t, OpsAlertEvaluationStatusNoData, result.Status)
	require.Equal(t, "empty_ttft_window", result.ErrorCode)
}

func TestOpsAlertMetricTTFTUsesDedicatedQueryAndCachesMatchingWindows(t *testing.T) {
	now := time.Now().UTC()
	p99 := 4200
	repo := &stubOpsRepo{
		ttft: &OpsTTFTSummary{SampleCount: 12, TTFT: OpsPercentiles{P99: &p99}},
		// A dashboard-wide query failure must not make the dedicated TTFT metric fail.
		err: context.DeadlineExceeded,
	}
	svc := &OpsAlertEvaluatorService{opsRepo: repo}
	cache := make(map[opsAlertTTFTCacheKey]opsAlertTTFTCacheEntry)
	rule := &OpsAlertRule{MetricType: "ttft_p99_seconds", Operator: ">", Threshold: 3}

	first := svc.evaluateRuleMetric(
		context.Background(), rule, nil, now.Add(-5*time.Minute), now, "openai", nil, now, cache,
	)
	second := svc.evaluateRuleMetric(
		context.Background(), rule, nil, now.Add(-5*time.Minute), now, "openai", nil, now, cache,
	)

	require.Equal(t, OpsAlertEvaluationStatusBreached, first.Status)
	require.Equal(t, OpsAlertEvaluationStatusBreached, second.Status)
	require.Equal(t, 1, repo.ttftCalls)
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
