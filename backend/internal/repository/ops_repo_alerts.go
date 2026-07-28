package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func normalizeOpsAlertIncidentFamily(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "custom"
	}
	return value
}

func normalizeOpsAlertRecoverySustainedMinutes(value int) int {
	if value < 1 {
		return 1
	}
	return value
}

func matchesLegacyOpsAlertRuleDefault(
	rule *service.OpsAlertRule,
	name, description, metricType, operator, severity string,
	threshold float64,
	windowMinutes, sustainedMinutes, cooldownMinutes int,
) bool {
	return rule != nil &&
		rule.Name == name &&
		rule.Description == description &&
		rule.Enabled &&
		rule.NotifyEmail &&
		rule.MetricType == metricType &&
		rule.Operator == operator &&
		rule.Severity == severity &&
		rule.Threshold == threshold &&
		rule.WindowMinutes == windowMinutes &&
		rule.SustainedMinutes == sustainedMinutes &&
		rule.CooldownMinutes == cooldownMinutes &&
		rule.IncidentFamily == "custom" &&
		rule.MinimumSamples == 0 &&
		rule.MinimumBadCount == 0 &&
		rule.RecoveryOperator == "" &&
		rule.RecoveryThreshold == nil &&
		rule.RecoverySustainedMinutes == 1 &&
		!rule.ShadowMode &&
		len(rule.Filters) == 0
}

func applyLegacyOpsAlertRuleCompatibility(rule *service.OpsAlertRule) {
	if rule == nil {
		return
	}

	switch {
	case matchesLegacyOpsAlertRuleDefault(
		rule, "成功率过低", "当成功率低于 95% 且持续 5 分钟时触发告警（服务可用性下降）",
		"success_rate", "<", "P0", 95, 5, 5, 15,
	):
		rule.Enabled = false
		rule.Description += " [disabled: duplicate of error_rate]"
	case matchesLegacyOpsAlertRuleDefault(
		rule, "P95延迟过高", "当 P95 延迟超过 2000ms 且持续 10 分钟时触发告警",
		"p95_latency_ms", ">", "P2", 2000, 5, 10, 30,
	), matchesLegacyOpsAlertRuleDefault(
		rule, "P99延迟过高", "当 P99 延迟超过 3000ms 且持续 10 分钟时触发告警",
		"p99_latency_ms", ">", "P2", 3000, 5, 10, 30,
	):
		rule.Enabled = false
		rule.Description += " [disabled: unsupported legacy latency metric]"
	case matchesLegacyOpsAlertRuleDefault(
		rule, "错误率过高", "当错误率超过 5% 且持续 5 分钟时触发告警",
		"error_rate", ">", "P1", 5, 5, 5, 20,
	):
		rule.Name = "基础设施可用性缓慢下降"
		rule.Description = "30 分钟 SLA 合格请求失败率达到 5%，失败至少 10 次且样本至少 100；持续 10 分钟后触发"
		rule.MetricType = "availability_failure_rate"
		rule.Operator = ">="
		rule.WindowMinutes = 30
		rule.SustainedMinutes = 10
		rule.IncidentFamily = "availability"
		rule.MinimumSamples = 100
		rule.MinimumBadCount = 10
		rule.RecoveryOperator = "<"
		rule.RecoveryThreshold = float64PtrRepository(2.5)
		rule.RecoverySustainedMinutes = 10
	case matchesLegacyOpsAlertRuleDefault(
		rule, "错误率极高", "当错误率超过 20% 且持续 1 分钟时触发告警（服务严重异常）",
		"error_rate", ">", "P0", 20, 1, 1, 15,
	):
		rule.Name = "基础设施可用性快速下降"
		rule.Description = "5 分钟 SLA 合格请求失败率达到 20%，失败至少 10 次且样本至少 30；持续 3 分钟后触发"
		rule.MetricType = "availability_failure_rate"
		rule.Operator = ">="
		rule.WindowMinutes = 5
		rule.SustainedMinutes = 3
		rule.IncidentFamily = "availability"
		rule.MinimumSamples = 30
		rule.MinimumBadCount = 10
		rule.RecoveryOperator = "<"
		rule.RecoveryThreshold = float64PtrRepository(10)
		rule.RecoverySustainedMinutes = 5
	case matchesLegacyOpsAlertRuleDefault(
		rule, "CPU使用率过高", "当 CPU 使用率超过 85% 且持续 10 分钟时触发告警",
		"cpu_usage_percent", ">", "P2", 85, 5, 10, 30,
	):
		applyLegacyOpsAlertRecovery(rule, "resource_capacity", 75)
	case matchesLegacyOpsAlertRuleDefault(
		rule, "内存使用率过高", "当内存使用率超过 90% 且持续 10 分钟时触发告警（可能导致 OOM）",
		"memory_usage_percent", ">", "P1", 90, 5, 10, 20,
	):
		applyLegacyOpsAlertRecovery(rule, "resource_capacity", 85)
	case matchesLegacyOpsAlertRuleDefault(
		rule, "并发队列积压", "当并发队列深度超过 100 且持续 5 分钟时触发告警（系统处理能力不足）",
		"concurrency_queue_depth", ">", "P1", 100, 5, 5, 20,
	):
		applyLegacyOpsAlertRecovery(rule, "request_queue", 50)
	}
}

func applyLegacyOpsAlertRecovery(rule *service.OpsAlertRule, family string, threshold float64) {
	rule.IncidentFamily = family
	rule.RecoveryOperator = "<"
	rule.RecoveryThreshold = float64PtrRepository(threshold)
	rule.RecoverySustainedMinutes = 5
}

func float64PtrRepository(value float64) *float64 {
	return &value
}

func (r *opsRepository) ListAlertRules(ctx context.Context) ([]*service.OpsAlertRule, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}

	q := `
SELECT
  id,
  name,
  COALESCE(description, ''),
  enabled,
  COALESCE(severity, ''),
  metric_type,
  operator,
  threshold,
	  window_minutes,
	  sustained_minutes,
	  cooldown_minutes,
	  COALESCE(incident_family, 'custom'),
	  COALESCE(minimum_samples, 0),
	  COALESCE(minimum_bad_count, 0),
	  COALESCE(recovery_operator, ''),
	  recovery_threshold,
	  COALESCE(recovery_sustained_minutes, 1),
	  COALESCE(shadow_mode, false),
	  COALESCE(notify_email, true),
  filters,
  last_triggered_at,
  created_at,
  updated_at
FROM ops_alert_rules
ORDER BY id DESC`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []*service.OpsAlertRule{}
	for rows.Next() {
		var rule service.OpsAlertRule
		var filtersRaw []byte
		var lastTriggeredAt sql.NullTime
		var recoveryThreshold sql.NullFloat64
		if err := rows.Scan(
			&rule.ID,
			&rule.Name,
			&rule.Description,
			&rule.Enabled,
			&rule.Severity,
			&rule.MetricType,
			&rule.Operator,
			&rule.Threshold,
			&rule.WindowMinutes,
			&rule.SustainedMinutes,
			&rule.CooldownMinutes,
			&rule.IncidentFamily,
			&rule.MinimumSamples,
			&rule.MinimumBadCount,
			&rule.RecoveryOperator,
			&recoveryThreshold,
			&rule.RecoverySustainedMinutes,
			&rule.ShadowMode,
			&rule.NotifyEmail,
			&filtersRaw,
			&lastTriggeredAt,
			&rule.CreatedAt,
			&rule.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if lastTriggeredAt.Valid {
			v := lastTriggeredAt.Time
			rule.LastTriggeredAt = &v
		}
		if recoveryThreshold.Valid {
			value := recoveryThreshold.Float64
			rule.RecoveryThreshold = &value
		}
		if len(filtersRaw) > 0 && string(filtersRaw) != "null" {
			var decoded map[string]any
			if err := json.Unmarshal(filtersRaw, &decoded); err == nil {
				rule.Filters = decoded
			}
		}
		applyLegacyOpsAlertRuleCompatibility(&rule)
		out = append(out, &rule)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *opsRepository) CreateAlertRule(ctx context.Context, input *service.OpsAlertRule) (*service.OpsAlertRule, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if input == nil {
		return nil, fmt.Errorf("nil input")
	}

	filtersArg, err := opsNullJSONMap(input.Filters)
	if err != nil {
		return nil, err
	}

	q := `
INSERT INTO ops_alert_rules (
  name,
  description,
  enabled,
  severity,
  metric_type,
  operator,
  threshold,
	  window_minutes,
	  sustained_minutes,
	  cooldown_minutes,
	  incident_family,
	  minimum_samples,
	  minimum_bad_count,
	  recovery_operator,
	  recovery_threshold,
	  recovery_sustained_minutes,
	  shadow_mode,
	  notify_email,
  filters,
  created_at,
  updated_at
) VALUES (
	  $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,NOW(),NOW()
)
RETURNING
  id,
  name,
  COALESCE(description, ''),
  enabled,
  COALESCE(severity, ''),
  metric_type,
  operator,
  threshold,
	  window_minutes,
	  sustained_minutes,
	  cooldown_minutes,
	  COALESCE(incident_family, 'custom'),
	  COALESCE(minimum_samples, 0),
	  COALESCE(minimum_bad_count, 0),
	  COALESCE(recovery_operator, ''),
	  recovery_threshold,
	  COALESCE(recovery_sustained_minutes, 1),
	  COALESCE(shadow_mode, false),
	  COALESCE(notify_email, true),
  filters,
  last_triggered_at,
  created_at,
  updated_at`

	var out service.OpsAlertRule
	var filtersRaw []byte
	var lastTriggeredAt sql.NullTime
	var recoveryThreshold sql.NullFloat64

	if err := r.db.QueryRowContext(
		ctx,
		q,
		strings.TrimSpace(input.Name),
		strings.TrimSpace(input.Description),
		input.Enabled,
		strings.TrimSpace(input.Severity),
		strings.TrimSpace(input.MetricType),
		strings.TrimSpace(input.Operator),
		input.Threshold,
		input.WindowMinutes,
		input.SustainedMinutes,
		input.CooldownMinutes,
		normalizeOpsAlertIncidentFamily(input.IncidentFamily),
		input.MinimumSamples,
		input.MinimumBadCount,
		opsNullString(input.RecoveryOperator),
		opsNullFloat64(input.RecoveryThreshold),
		normalizeOpsAlertRecoverySustainedMinutes(input.RecoverySustainedMinutes),
		input.ShadowMode,
		input.NotifyEmail,
		filtersArg,
	).Scan(
		&out.ID,
		&out.Name,
		&out.Description,
		&out.Enabled,
		&out.Severity,
		&out.MetricType,
		&out.Operator,
		&out.Threshold,
		&out.WindowMinutes,
		&out.SustainedMinutes,
		&out.CooldownMinutes,
		&out.IncidentFamily,
		&out.MinimumSamples,
		&out.MinimumBadCount,
		&out.RecoveryOperator,
		&recoveryThreshold,
		&out.RecoverySustainedMinutes,
		&out.ShadowMode,
		&out.NotifyEmail,
		&filtersRaw,
		&lastTriggeredAt,
		&out.CreatedAt,
		&out.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if lastTriggeredAt.Valid {
		v := lastTriggeredAt.Time
		out.LastTriggeredAt = &v
	}
	if recoveryThreshold.Valid {
		value := recoveryThreshold.Float64
		out.RecoveryThreshold = &value
	}
	if len(filtersRaw) > 0 && string(filtersRaw) != "null" {
		var decoded map[string]any
		if err := json.Unmarshal(filtersRaw, &decoded); err == nil {
			out.Filters = decoded
		}
	}

	return &out, nil
}

func (r *opsRepository) UpdateAlertRule(ctx context.Context, input *service.OpsAlertRule) (*service.OpsAlertRule, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if input == nil {
		return nil, fmt.Errorf("nil input")
	}
	if input.ID <= 0 {
		return nil, fmt.Errorf("invalid id")
	}

	filtersArg, err := opsNullJSONMap(input.Filters)
	if err != nil {
		return nil, err
	}

	q := `
UPDATE ops_alert_rules
SET
  name = $2,
  description = $3,
  enabled = $4,
  severity = $5,
  metric_type = $6,
  operator = $7,
  threshold = $8,
	  window_minutes = $9,
	  sustained_minutes = $10,
	  cooldown_minutes = $11,
	  incident_family = $12,
	  minimum_samples = $13,
	  minimum_bad_count = $14,
	  recovery_operator = $15,
	  recovery_threshold = $16,
	  recovery_sustained_minutes = $17,
	  shadow_mode = $18,
	  notify_email = $19,
	  filters = $20,
  updated_at = NOW()
WHERE id = $1
RETURNING
  id,
  name,
  COALESCE(description, ''),
  enabled,
  COALESCE(severity, ''),
  metric_type,
  operator,
  threshold,
	  window_minutes,
	  sustained_minutes,
	  cooldown_minutes,
	  COALESCE(incident_family, 'custom'),
	  COALESCE(minimum_samples, 0),
	  COALESCE(minimum_bad_count, 0),
	  COALESCE(recovery_operator, ''),
	  recovery_threshold,
	  COALESCE(recovery_sustained_minutes, 1),
	  COALESCE(shadow_mode, false),
	  COALESCE(notify_email, true),
  filters,
  last_triggered_at,
  created_at,
  updated_at`

	var out service.OpsAlertRule
	var filtersRaw []byte
	var lastTriggeredAt sql.NullTime
	var recoveryThreshold sql.NullFloat64

	if err := r.db.QueryRowContext(
		ctx,
		q,
		input.ID,
		strings.TrimSpace(input.Name),
		strings.TrimSpace(input.Description),
		input.Enabled,
		strings.TrimSpace(input.Severity),
		strings.TrimSpace(input.MetricType),
		strings.TrimSpace(input.Operator),
		input.Threshold,
		input.WindowMinutes,
		input.SustainedMinutes,
		input.CooldownMinutes,
		normalizeOpsAlertIncidentFamily(input.IncidentFamily),
		input.MinimumSamples,
		input.MinimumBadCount,
		opsNullString(input.RecoveryOperator),
		opsNullFloat64(input.RecoveryThreshold),
		normalizeOpsAlertRecoverySustainedMinutes(input.RecoverySustainedMinutes),
		input.ShadowMode,
		input.NotifyEmail,
		filtersArg,
	).Scan(
		&out.ID,
		&out.Name,
		&out.Description,
		&out.Enabled,
		&out.Severity,
		&out.MetricType,
		&out.Operator,
		&out.Threshold,
		&out.WindowMinutes,
		&out.SustainedMinutes,
		&out.CooldownMinutes,
		&out.IncidentFamily,
		&out.MinimumSamples,
		&out.MinimumBadCount,
		&out.RecoveryOperator,
		&recoveryThreshold,
		&out.RecoverySustainedMinutes,
		&out.ShadowMode,
		&out.NotifyEmail,
		&filtersRaw,
		&lastTriggeredAt,
		&out.CreatedAt,
		&out.UpdatedAt,
	); err != nil {
		return nil, err
	}

	if lastTriggeredAt.Valid {
		v := lastTriggeredAt.Time
		out.LastTriggeredAt = &v
	}
	if recoveryThreshold.Valid {
		value := recoveryThreshold.Float64
		out.RecoveryThreshold = &value
	}
	if len(filtersRaw) > 0 && string(filtersRaw) != "null" {
		var decoded map[string]any
		if err := json.Unmarshal(filtersRaw, &decoded); err == nil {
			out.Filters = decoded
		}
	}

	return &out, nil
}

func (r *opsRepository) DeleteAlertRule(ctx context.Context, id int64) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil ops repository")
	}
	if id <= 0 {
		return fmt.Errorf("invalid id")
	}

	res, err := r.db.ExecContext(ctx, "DELETE FROM ops_alert_rules WHERE id = $1", id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *opsRepository) ListAlertEvents(ctx context.Context, filter *service.OpsAlertEventFilter) ([]*service.OpsAlertEvent, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if filter == nil {
		filter = &service.OpsAlertEventFilter{}
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	where, args := buildOpsAlertEventsWhere(filter)
	args = append(args, limit)
	limitArg := "$" + itoa(len(args))

	q := `
SELECT
  id,
  COALESCE(rule_id, 0),
  COALESCE(severity, ''),
  COALESCE(status, ''),
  COALESCE(title, ''),
  COALESCE(description, ''),
  metric_value,
  threshold_value,
  dimensions,
  fired_at,
  resolved_at,
  email_sent,
  COALESCE(email_queued, false),
  created_at
FROM ops_alert_events
` + where + `
ORDER BY fired_at DESC, id DESC
LIMIT ` + limitArg

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []*service.OpsAlertEvent{}
	for rows.Next() {
		var ev service.OpsAlertEvent
		var metricValue sql.NullFloat64
		var thresholdValue sql.NullFloat64
		var dimensionsRaw []byte
		var resolvedAt sql.NullTime
		if err := rows.Scan(
			&ev.ID,
			&ev.RuleID,
			&ev.Severity,
			&ev.Status,
			&ev.Title,
			&ev.Description,
			&metricValue,
			&thresholdValue,
			&dimensionsRaw,
			&ev.FiredAt,
			&resolvedAt,
			&ev.EmailSent,
			&ev.EmailQueued,
			&ev.CreatedAt,
		); err != nil {
			return nil, err
		}
		if metricValue.Valid {
			v := metricValue.Float64
			ev.MetricValue = &v
		}
		if thresholdValue.Valid {
			v := thresholdValue.Float64
			ev.ThresholdValue = &v
		}
		if resolvedAt.Valid {
			v := resolvedAt.Time
			ev.ResolvedAt = &v
		}
		if len(dimensionsRaw) > 0 && string(dimensionsRaw) != "null" {
			var decoded map[string]any
			if err := json.Unmarshal(dimensionsRaw, &decoded); err == nil {
				ev.Dimensions = decoded
			}
		}
		out = append(out, &ev)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *opsRepository) GetAlertEventByID(ctx context.Context, eventID int64) (*service.OpsAlertEvent, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if eventID <= 0 {
		return nil, fmt.Errorf("invalid event id")
	}

	q := `
SELECT
  id,
  COALESCE(rule_id, 0),
  COALESCE(severity, ''),
  COALESCE(status, ''),
  COALESCE(title, ''),
  COALESCE(description, ''),
  metric_value,
  threshold_value,
  dimensions,
  fired_at,
  resolved_at,
  email_sent,
  COALESCE(email_queued, false),
  created_at
FROM ops_alert_events
WHERE id = $1`

	row := r.db.QueryRowContext(ctx, q, eventID)
	ev, err := scanOpsAlertEvent(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return ev, nil
}

func (r *opsRepository) GetActiveAlertEvent(ctx context.Context, ruleID int64) (*service.OpsAlertEvent, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if ruleID <= 0 {
		return nil, fmt.Errorf("invalid rule id")
	}

	q := `
SELECT
  id,
  COALESCE(rule_id, 0),
  COALESCE(severity, ''),
  COALESCE(status, ''),
  COALESCE(title, ''),
  COALESCE(description, ''),
  metric_value,
  threshold_value,
  dimensions,
  fired_at,
  resolved_at,
  email_sent,
  COALESCE(email_queued, false),
  created_at
FROM ops_alert_events
WHERE rule_id = $1 AND status = $2
ORDER BY fired_at DESC
LIMIT 1`

	row := r.db.QueryRowContext(ctx, q, ruleID, service.OpsAlertStatusFiring)
	ev, err := scanOpsAlertEvent(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return ev, nil
}

func (r *opsRepository) GetLatestAlertEvent(ctx context.Context, ruleID int64) (*service.OpsAlertEvent, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if ruleID <= 0 {
		return nil, fmt.Errorf("invalid rule id")
	}

	q := `
SELECT
  id,
  COALESCE(rule_id, 0),
  COALESCE(severity, ''),
  COALESCE(status, ''),
  COALESCE(title, ''),
  COALESCE(description, ''),
  metric_value,
  threshold_value,
  dimensions,
  fired_at,
  resolved_at,
  email_sent,
  COALESCE(email_queued, false),
  created_at
FROM ops_alert_events
WHERE rule_id = $1
ORDER BY fired_at DESC
LIMIT 1`

	row := r.db.QueryRowContext(ctx, q, ruleID)
	ev, err := scanOpsAlertEvent(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return ev, nil
}

func (r *opsRepository) CreateAlertEvent(ctx context.Context, event *service.OpsAlertEvent) (*service.OpsAlertEvent, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if event == nil {
		return nil, fmt.Errorf("nil event")
	}

	dimensionsArg, err := opsNullJSONMap(event.Dimensions)
	if err != nil {
		return nil, err
	}

	q := `
INSERT INTO ops_alert_events (
  rule_id,
  severity,
  status,
  title,
  description,
  metric_value,
  threshold_value,
  dimensions,
  fired_at,
  resolved_at,
  email_sent,
  email_queued,
  created_at
) VALUES (
  $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,NOW()
)
RETURNING
  id,
  COALESCE(rule_id, 0),
  COALESCE(severity, ''),
  COALESCE(status, ''),
  COALESCE(title, ''),
  COALESCE(description, ''),
  metric_value,
  threshold_value,
  dimensions,
  fired_at,
  resolved_at,
  email_sent,
  COALESCE(email_queued, false),
  created_at`

	row := r.db.QueryRowContext(
		ctx,
		q,
		opsNullInt64(&event.RuleID),
		opsNullString(event.Severity),
		opsNullString(event.Status),
		opsNullString(event.Title),
		opsNullString(event.Description),
		opsNullFloat64(event.MetricValue),
		opsNullFloat64(event.ThresholdValue),
		dimensionsArg,
		event.FiredAt,
		opsNullTime(event.ResolvedAt),
		event.EmailSent,
		event.EmailQueued,
	)
	created, err := scanOpsAlertEvent(row)
	if err != nil {
		return nil, err
	}
	if created != nil && created.RuleID > 0 {
		if _, updateErr := r.db.ExecContext(ctx, `UPDATE ops_alert_rules SET last_triggered_at = $2, updated_at = NOW() WHERE id = $1`, created.RuleID, created.FiredAt); updateErr != nil {
			return nil, updateErr
		}
	}
	return created, nil
}

func (r *opsRepository) UpdateAlertEventStatus(ctx context.Context, eventID int64, status string, resolvedAt *time.Time) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil ops repository")
	}
	if eventID <= 0 {
		return fmt.Errorf("invalid event id")
	}
	if strings.TrimSpace(status) == "" {
		return fmt.Errorf("invalid status")
	}

	q := `
UPDATE ops_alert_events
SET status = $2,
    resolved_at = $3
WHERE id = $1`

	_, err := r.db.ExecContext(ctx, q, eventID, strings.TrimSpace(status), opsNullTime(resolvedAt))
	return err
}

func (r *opsRepository) UpdateAlertEventEmailSent(ctx context.Context, eventID int64, emailSent bool) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil ops repository")
	}
	if eventID <= 0 {
		return fmt.Errorf("invalid event id")
	}

	_, err := r.db.ExecContext(ctx, "UPDATE ops_alert_events SET email_sent = $2 WHERE id = $1", eventID, emailSent)
	return err
}

func (r *opsRepository) UpdateAlertEventEmailQueued(ctx context.Context, eventID int64, emailQueued bool) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil ops repository")
	}
	if eventID <= 0 {
		return fmt.Errorf("invalid event id")
	}
	_, err := r.db.ExecContext(ctx, "UPDATE ops_alert_events SET email_queued = $2 WHERE id = $1", eventID, emailQueued)
	return err
}

func (r *opsRepository) InsertAlertRuleEvaluation(ctx context.Context, evaluation *service.OpsAlertRuleEvaluation) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil ops repository")
	}
	if evaluation == nil || evaluation.RuleID <= 0 {
		return fmt.Errorf("invalid alert rule evaluation")
	}
	evaluatedAt := evaluation.EvaluatedAt.UTC()
	if evaluatedAt.IsZero() {
		evaluatedAt = time.Now().UTC()
	}
	version := strings.TrimSpace(evaluation.EvaluatorVersion)
	if version == "" {
		version = "v2"
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO ops_alert_rule_evaluations (
			rule_id, evaluated_at, window_start, window_end, status, breached,
			metric_value, threshold_value, sample_count, bad_count, data_as_of,
			error_code, error_message, evaluator_version
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
	`, evaluation.RuleID, evaluatedAt, evaluation.WindowStart.UTC(), evaluation.WindowEnd.UTC(),
		strings.TrimSpace(evaluation.Status), evaluation.Breached,
		opsNullFloat64(evaluation.MetricValue), opsNullFloat64(evaluation.ThresholdValue),
		evaluation.SampleCount, evaluation.BadCount, opsNullTime(evaluation.DataAsOf),
		opsNullString(evaluation.ErrorCode), opsNullString(evaluation.ErrorMessage), version)
	return err
}

func (r *opsRepository) ListLatestAlertRuleEvaluations(ctx context.Context) ([]*service.OpsAlertRuleEvaluation, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, rule_id, evaluated_at, window_start, window_end, status, breached,
			metric_value, threshold_value, sample_count, bad_count, data_as_of,
			COALESCE(error_code, ''), COALESCE(error_message, ''), evaluator_version, created_at
		FROM (
			SELECT DISTINCT ON (rule_id) *
			FROM ops_alert_rule_evaluations
			ORDER BY rule_id, evaluated_at DESC, id DESC
		) latest
		ORDER BY rule_id
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]*service.OpsAlertRuleEvaluation, 0)
	for rows.Next() {
		evaluation, scanErr := scanOpsAlertRuleEvaluation(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, evaluation)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *opsRepository) GetAlertRuleState(ctx context.Context, ruleID int64) (*service.OpsAlertRuleState, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if ruleID <= 0 {
		return nil, fmt.Errorf("invalid rule id")
	}
	var state service.OpsAlertRuleState
	var lastEvaluatedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, `
		SELECT rule_id, last_evaluated_at, consecutive_breaches, consecutive_recoveries, updated_at
		FROM ops_alert_rule_states WHERE rule_id = $1
	`, ruleID).Scan(&state.RuleID, &lastEvaluatedAt, &state.ConsecutiveBreaches, &state.ConsecutiveRecoveries, &state.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if lastEvaluatedAt.Valid {
		value := lastEvaluatedAt.Time
		state.LastEvaluatedAt = &value
	}
	return &state, nil
}

func (r *opsRepository) UpsertAlertRuleState(ctx context.Context, state *service.OpsAlertRuleState) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil ops repository")
	}
	if state == nil || state.RuleID <= 0 {
		return fmt.Errorf("invalid alert rule state")
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO ops_alert_rule_states (
			rule_id, last_evaluated_at, consecutive_breaches, consecutive_recoveries, updated_at
		) VALUES ($1,$2,$3,$4,NOW())
		ON CONFLICT (rule_id) DO UPDATE SET
			last_evaluated_at = EXCLUDED.last_evaluated_at,
			consecutive_breaches = EXCLUDED.consecutive_breaches,
			consecutive_recoveries = EXCLUDED.consecutive_recoveries,
			updated_at = NOW()
	`, state.RuleID, opsNullTime(state.LastEvaluatedAt), state.ConsecutiveBreaches, state.ConsecutiveRecoveries)
	return err
}

type opsAlertRuleEvaluationRow interface {
	Scan(dest ...any) error
}

func scanOpsAlertRuleEvaluation(row opsAlertRuleEvaluationRow) (*service.OpsAlertRuleEvaluation, error) {
	var evaluation service.OpsAlertRuleEvaluation
	var metricValue sql.NullFloat64
	var thresholdValue sql.NullFloat64
	var dataAsOf sql.NullTime
	err := row.Scan(
		&evaluation.ID, &evaluation.RuleID, &evaluation.EvaluatedAt,
		&evaluation.WindowStart, &evaluation.WindowEnd, &evaluation.Status, &evaluation.Breached,
		&metricValue, &thresholdValue, &evaluation.SampleCount, &evaluation.BadCount,
		&dataAsOf, &evaluation.ErrorCode, &evaluation.ErrorMessage,
		&evaluation.EvaluatorVersion, &evaluation.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if metricValue.Valid {
		value := metricValue.Float64
		evaluation.MetricValue = &value
	}
	if thresholdValue.Valid {
		value := thresholdValue.Float64
		evaluation.ThresholdValue = &value
	}
	if dataAsOf.Valid {
		value := dataAsOf.Time
		evaluation.DataAsOf = &value
	}
	return &evaluation, nil
}

type opsAlertEventRow interface {
	Scan(dest ...any) error
}

func (r *opsRepository) CreateAlertSilence(ctx context.Context, input *service.OpsAlertSilence) (*service.OpsAlertSilence, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if input == nil {
		return nil, fmt.Errorf("nil input")
	}
	if input.RuleID <= 0 {
		return nil, fmt.Errorf("invalid rule_id")
	}
	platform := strings.TrimSpace(input.Platform)
	if platform == "" {
		return nil, fmt.Errorf("invalid platform")
	}
	if input.Until.IsZero() {
		return nil, fmt.Errorf("invalid until")
	}

	q := `
INSERT INTO ops_alert_silences (
  rule_id,
  platform,
  group_id,
  region,
  until,
  reason,
  created_by,
  created_at
) VALUES (
  $1,$2,$3,$4,$5,$6,$7,NOW()
)
RETURNING id, rule_id, platform, group_id, region, until, COALESCE(reason,''), created_by, created_at`

	row := r.db.QueryRowContext(
		ctx,
		q,
		input.RuleID,
		platform,
		opsNullInt64(input.GroupID),
		opsNullString(input.Region),
		input.Until,
		opsNullString(input.Reason),
		opsNullInt64(input.CreatedBy),
	)

	var out service.OpsAlertSilence
	var groupID sql.NullInt64
	var region sql.NullString
	var createdBy sql.NullInt64
	if err := row.Scan(
		&out.ID,
		&out.RuleID,
		&out.Platform,
		&groupID,
		&region,
		&out.Until,
		&out.Reason,
		&createdBy,
		&out.CreatedAt,
	); err != nil {
		return nil, err
	}
	if groupID.Valid {
		v := groupID.Int64
		out.GroupID = &v
	}
	if region.Valid {
		v := strings.TrimSpace(region.String)
		if v != "" {
			out.Region = &v
		}
	}
	if createdBy.Valid {
		v := createdBy.Int64
		out.CreatedBy = &v
	}
	return &out, nil
}

func (r *opsRepository) IsAlertSilenced(ctx context.Context, ruleID int64, platform string, groupID *int64, region *string, now time.Time) (bool, error) {
	if r == nil || r.db == nil {
		return false, fmt.Errorf("nil ops repository")
	}
	if ruleID <= 0 {
		return false, fmt.Errorf("invalid rule id")
	}
	platform = strings.TrimSpace(platform)
	if platform == "" {
		return false, nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	q := `
SELECT 1
FROM ops_alert_silences
WHERE rule_id = $1
  AND platform = $2
  AND (group_id IS NOT DISTINCT FROM $3)
  AND (region IS NOT DISTINCT FROM $4)
  AND until > $5
LIMIT 1`

	var dummy int
	err := r.db.QueryRowContext(ctx, q, ruleID, platform, opsNullInt64(groupID), opsNullString(region), now).Scan(&dummy)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func scanOpsAlertEvent(row opsAlertEventRow) (*service.OpsAlertEvent, error) {
	var ev service.OpsAlertEvent
	var metricValue sql.NullFloat64
	var thresholdValue sql.NullFloat64
	var dimensionsRaw []byte
	var resolvedAt sql.NullTime

	if err := row.Scan(
		&ev.ID,
		&ev.RuleID,
		&ev.Severity,
		&ev.Status,
		&ev.Title,
		&ev.Description,
		&metricValue,
		&thresholdValue,
		&dimensionsRaw,
		&ev.FiredAt,
		&resolvedAt,
		&ev.EmailSent,
		&ev.EmailQueued,
		&ev.CreatedAt,
	); err != nil {
		return nil, err
	}
	if metricValue.Valid {
		v := metricValue.Float64
		ev.MetricValue = &v
	}
	if thresholdValue.Valid {
		v := thresholdValue.Float64
		ev.ThresholdValue = &v
	}
	if resolvedAt.Valid {
		v := resolvedAt.Time
		ev.ResolvedAt = &v
	}
	if len(dimensionsRaw) > 0 && string(dimensionsRaw) != "null" {
		var decoded map[string]any
		if err := json.Unmarshal(dimensionsRaw, &decoded); err == nil {
			ev.Dimensions = decoded
		}
	}
	return &ev, nil
}

func buildOpsAlertEventsWhere(filter *service.OpsAlertEventFilter) (string, []any) {
	clauses := []string{"1=1"}
	args := []any{}

	if filter == nil {
		return "WHERE " + strings.Join(clauses, " AND "), args
	}

	if status := strings.TrimSpace(filter.Status); status != "" {
		args = append(args, status)
		clauses = append(clauses, "status = $"+itoa(len(args)))
	}
	if severity := strings.TrimSpace(filter.Severity); severity != "" {
		args = append(args, severity)
		clauses = append(clauses, "severity = $"+itoa(len(args)))
	}
	if filter.EmailSent != nil {
		args = append(args, *filter.EmailSent)
		clauses = append(clauses, "email_sent = $"+itoa(len(args)))
	}
	if filter.StartTime != nil && !filter.StartTime.IsZero() {
		args = append(args, *filter.StartTime)
		clauses = append(clauses, "fired_at >= $"+itoa(len(args)))
	}
	if filter.EndTime != nil && !filter.EndTime.IsZero() {
		args = append(args, *filter.EndTime)
		clauses = append(clauses, "fired_at < $"+itoa(len(args)))
	}

	// Cursor pagination (descending by fired_at, then id)
	if filter.BeforeFiredAt != nil && !filter.BeforeFiredAt.IsZero() && filter.BeforeID != nil && *filter.BeforeID > 0 {
		args = append(args, *filter.BeforeFiredAt)
		tsArg := "$" + itoa(len(args))
		args = append(args, *filter.BeforeID)
		idArg := "$" + itoa(len(args))
		clauses = append(clauses, fmt.Sprintf("(fired_at < %s OR (fired_at = %s AND id < %s))", tsArg, tsArg, idArg))
	}
	// Dimensions are stored in JSONB. We filter best-effort without requiring GIN indexes.
	if platform := strings.TrimSpace(filter.Platform); platform != "" {
		args = append(args, platform)
		clauses = append(clauses, "(dimensions->>'platform') = $"+itoa(len(args)))
	}
	if filter.GroupID != nil && *filter.GroupID > 0 {
		args = append(args, fmt.Sprintf("%d", *filter.GroupID))
		clauses = append(clauses, "(dimensions->>'group_id') = $"+itoa(len(args)))
	}

	return "WHERE " + strings.Join(clauses, " AND "), args
}

func opsNullJSONMap(v map[string]any) (any, error) {
	if v == nil {
		return sql.NullString{}, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		return sql.NullString{}, nil
	}
	return sql.NullString{String: string(b), Valid: true}, nil
}
