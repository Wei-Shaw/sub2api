package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func legacyOpsAlertRule(
	name, description, metricType, operator, severity string,
	threshold float64,
	windowMinutes, sustainedMinutes, cooldownMinutes int,
) *service.OpsAlertRule {
	return &service.OpsAlertRule{
		Name: name, Description: description, Enabled: true, NotifyEmail: true,
		MetricType: metricType, Operator: operator, Severity: severity, Threshold: threshold,
		WindowMinutes: windowMinutes, SustainedMinutes: sustainedMinutes, CooldownMinutes: cooldownMinutes,
		IncidentFamily: "custom", RecoverySustainedMinutes: 1,
	}
}

func TestApplyLegacyOpsAlertRuleCompatibility(t *testing.T) {
	tests := []struct {
		name            string
		rule            *service.OpsAlertRule
		wantEnabled     bool
		wantName        string
		wantMetric      string
		wantFamily      string
		wantSamples     int
		wantBadCount    int
		wantRecovery    *float64
		wantRecoveryFor int
	}{
		{
			name: "duplicate success rate is disabled",
			rule: legacyOpsAlertRule("成功率过低", "当成功率低于 95% 且持续 5 分钟时触发告警（服务可用性下降）",
				"success_rate", "<", "P0", 95, 5, 5, 15),
			wantEnabled: false, wantName: "成功率过低", wantMetric: "success_rate", wantFamily: "custom", wantRecoveryFor: 1,
		},
		{
			name: "unsupported p95 latency is disabled",
			rule: legacyOpsAlertRule("P95延迟过高", "当 P95 延迟超过 2000ms 且持续 10 分钟时触发告警",
				"p95_latency_ms", ">", "P2", 2000, 5, 10, 30),
			wantEnabled: false, wantName: "P95延迟过高", wantMetric: "p95_latency_ms", wantFamily: "custom", wantRecoveryFor: 1,
		},
		{
			name: "unsupported p99 latency is disabled",
			rule: legacyOpsAlertRule("P99延迟过高", "当 P99 延迟超过 3000ms 且持续 10 分钟时触发告警",
				"p99_latency_ms", ">", "P2", 3000, 5, 10, 30),
			wantEnabled: false, wantName: "P99延迟过高", wantMetric: "p99_latency_ms", wantFamily: "custom", wantRecoveryFor: 1,
		},
		{
			name: "legacy slow error rule is disabled",
			rule: legacyOpsAlertRule("错误率过高", "当错误率超过 5% 且持续 5 分钟时触发告警",
				"error_rate", ">", "P1", 5, 5, 5, 20),
			wantEnabled: false, wantName: "错误率过高", wantMetric: "error_rate", wantFamily: "custom", wantRecoveryFor: 1,
		},
		{
			name: "legacy fast error rule is disabled",
			rule: legacyOpsAlertRule("错误率极高", "当错误率超过 20% 且持续 1 分钟时触发告警（服务严重异常）",
				"error_rate", ">", "P0", 20, 1, 1, 15),
			wantEnabled: false, wantName: "错误率极高", wantMetric: "error_rate", wantFamily: "custom", wantRecoveryFor: 1,
		},
		{
			name: "cpu rule gains recovery semantics",
			rule: legacyOpsAlertRule("CPU使用率过高", "当 CPU 使用率超过 85% 且持续 10 分钟时触发告警",
				"cpu_usage_percent", ">", "P2", 85, 5, 10, 30),
			wantEnabled: true, wantName: "CPU使用率过高", wantMetric: "cpu_usage_percent",
			wantFamily: "resource_capacity", wantRecovery: float64PtrRepository(75), wantRecoveryFor: 5,
		},
		{
			name: "memory rule gains recovery semantics",
			rule: legacyOpsAlertRule("内存使用率过高", "当内存使用率超过 90% 且持续 10 分钟时触发告警（可能导致 OOM）",
				"memory_usage_percent", ">", "P1", 90, 5, 10, 20),
			wantEnabled: true, wantName: "内存使用率过高", wantMetric: "memory_usage_percent",
			wantFamily: "resource_capacity", wantRecovery: float64PtrRepository(85), wantRecoveryFor: 5,
		},
		{
			name: "queue rule gains recovery semantics",
			rule: legacyOpsAlertRule("并发队列积压", "当并发队列深度超过 100 且持续 5 分钟时触发告警（系统处理能力不足）",
				"concurrency_queue_depth", ">", "P1", 100, 5, 5, 20),
			wantEnabled: true, wantName: "并发队列积压", wantMetric: "concurrency_queue_depth",
			wantFamily: "request_queue", wantRecovery: float64PtrRepository(50), wantRecoveryFor: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applyLegacyOpsAlertRuleCompatibility(tt.rule)
			require.Equal(t, tt.wantEnabled, tt.rule.Enabled)
			require.Equal(t, tt.wantName, tt.rule.Name)
			require.Equal(t, tt.wantMetric, tt.rule.MetricType)
			require.Equal(t, tt.wantFamily, tt.rule.IncidentFamily)
			require.Equal(t, tt.wantSamples, tt.rule.MinimumSamples)
			require.Equal(t, tt.wantBadCount, tt.rule.MinimumBadCount)
			require.Equal(t, tt.wantRecovery, tt.rule.RecoveryThreshold)
			require.Equal(t, tt.wantRecoveryFor, tt.rule.RecoverySustainedMinutes)
		})
	}
}

func TestApplyLegacyOpsAlertRuleCompatibilityPreservesOperatorChanges(t *testing.T) {
	mutations := []struct {
		name   string
		mutate func(*service.OpsAlertRule)
	}{
		{name: "name", mutate: func(rule *service.OpsAlertRule) { rule.Name += " custom" }},
		{name: "description", mutate: func(rule *service.OpsAlertRule) { rule.Description += " custom" }},
		{name: "enabled", mutate: func(rule *service.OpsAlertRule) { rule.Enabled = false }},
		{name: "notify email", mutate: func(rule *service.OpsAlertRule) { rule.NotifyEmail = false }},
		{name: "metric", mutate: func(rule *service.OpsAlertRule) { rule.MetricType = "provider_failure_rate" }},
		{name: "operator", mutate: func(rule *service.OpsAlertRule) { rule.Operator = ">=" }},
		{name: "severity", mutate: func(rule *service.OpsAlertRule) { rule.Severity = "P2" }},
		{name: "threshold", mutate: func(rule *service.OpsAlertRule) { rule.Threshold = 6 }},
		{name: "window", mutate: func(rule *service.OpsAlertRule) { rule.WindowMinutes = 10 }},
		{name: "sustained", mutate: func(rule *service.OpsAlertRule) { rule.SustainedMinutes = 10 }},
		{name: "cooldown", mutate: func(rule *service.OpsAlertRule) { rule.CooldownMinutes = 30 }},
		{name: "incident family", mutate: func(rule *service.OpsAlertRule) { rule.IncidentFamily = "customized" }},
		{name: "minimum samples", mutate: func(rule *service.OpsAlertRule) { rule.MinimumSamples = 10 }},
		{name: "minimum bad count", mutate: func(rule *service.OpsAlertRule) { rule.MinimumBadCount = 2 }},
		{name: "recovery operator", mutate: func(rule *service.OpsAlertRule) { rule.RecoveryOperator = "<" }},
		{name: "recovery threshold", mutate: func(rule *service.OpsAlertRule) { rule.RecoveryThreshold = float64PtrRepository(2.5) }},
		{name: "recovery sustained", mutate: func(rule *service.OpsAlertRule) { rule.RecoverySustainedMinutes = 5 }},
		{name: "shadow mode", mutate: func(rule *service.OpsAlertRule) { rule.ShadowMode = true }},
		{name: "filters", mutate: func(rule *service.OpsAlertRule) { rule.Filters = map[string]any{"group_id": float64(42)} }},
	}

	for _, tt := range mutations {
		t.Run(tt.name, func(t *testing.T) {
			rule := legacyOpsAlertRule("错误率过高", "当错误率超过 5% 且持续 5 分钟时触发告警",
				"error_rate", ">", "P1", 5, 5, 5, 20)
			tt.mutate(rule)
			want := *rule

			applyLegacyOpsAlertRuleCompatibility(rule)

			require.Equal(t, want, *rule)
		})
	}
}

func TestListAlertRulesAppliesLegacyCompatibility(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	rows := sqlmock.NewRows([]string{
		"id", "name", "description", "enabled", "severity", "metric_type", "operator", "threshold",
		"window_minutes", "sustained_minutes", "cooldown_minutes", "incident_family", "minimum_samples",
		"minimum_bad_count", "recovery_operator", "recovery_threshold", "recovery_sustained_minutes",
		"shadow_mode", "notify_email", "filters", "last_triggered_at", "created_at", "updated_at",
	}).AddRow(
		int64(42), "错误率过高", "当错误率超过 5% 且持续 5 分钟时触发告警", true, "P1", "error_rate", ">", 5.0,
		5, 5, 20, "custom", 0, 0, "", nil, 1, false, true, []byte("null"),
		sql.NullTime{}, time.Now(), time.Now(),
	)
	mock.ExpectQuery(`SELECT\s+id,`).WillReturnRows(rows)

	rules, err := repo.ListAlertRules(context.Background())
	require.NoError(t, err)
	require.Len(t, rules, 1)
	require.False(t, rules[0].Enabled)
	require.Contains(t, rules[0].Description, "[disabled: replaced by availability failure rules]")
	require.NoError(t, mock.ExpectationsWereMet())
}
