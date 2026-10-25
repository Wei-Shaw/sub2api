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
		if len(filtersRaw) > 0 && string(filtersRaw) != "null" {
			var decoded map[string]any
			if err := json.Unmarshal(filtersRaw, &decoded); err == nil {
				rule.Filters = decoded
			}
		}
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

	if IsMySQL() {
		return r.createAlertRuleMySQL(ctx, input, filtersArg)
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
  notify_email,
  filters,
  created_at,
  updated_at
) VALUES (
  $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,NOW(),NOW()
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
  COALESCE(notify_email, true),
  filters,
  last_triggered_at,
  created_at,
  updated_at`

	var out service.OpsAlertRule
	var filtersRaw []byte
	var lastTriggeredAt sql.NullTime

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

	if IsMySQL() {
		return r.updateAlertRuleMySQL(ctx, input, filtersArg)
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
  notify_email = $12,
  filters = $13,
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
  COALESCE(notify_email, true),
  filters,
  last_triggered_at,
  created_at,
  updated_at`

	var out service.OpsAlertRule
	var filtersRaw []byte
	var lastTriggeredAt sql.NullTime

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

	if IsMySQL() {
		res, err := r.db.ExecContext(ctx, "DELETE FROM ops_alert_rules WHERE id = ?", id)
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

	if IsMySQL() {
		return r.listAlertEventsMySQL(ctx, filter, limit)
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
	if IsMySQL() {
		row := r.db.QueryRowContext(ctx, `SELECT `+opsAlertEventSelectListMySQL+` FROM ops_alert_events WHERE id = ?`, eventID)
		ev, err := scanOpsAlertEvent(row)
		if err != nil {
			if err == sql.ErrNoRows {
				return nil, nil
			}
			return nil, err
		}
		return ev, nil
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
	if IsMySQL() {
		row := r.db.QueryRowContext(ctx, `SELECT `+opsAlertEventSelectListMySQL+` FROM ops_alert_events WHERE rule_id = ? AND status = ? ORDER BY fired_at DESC LIMIT 1`, ruleID, service.OpsAlertStatusFiring)
		ev, err := scanOpsAlertEvent(row)
		if err != nil {
			if err == sql.ErrNoRows {
				return nil, nil
			}
			return nil, err
		}
		return ev, nil
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
	if IsMySQL() {
		row := r.db.QueryRowContext(ctx, `SELECT `+opsAlertEventSelectListMySQL+` FROM ops_alert_events WHERE rule_id = ? ORDER BY fired_at DESC LIMIT 1`, ruleID)
		ev, err := scanOpsAlertEvent(row)
		if err != nil {
			if err == sql.ErrNoRows {
				return nil, nil
			}
			return nil, err
		}
		return ev, nil
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

	if IsMySQL() {
		return r.createAlertEventMySQL(ctx, event, dimensionsArg)
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
  created_at
) VALUES (
  $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NOW()
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
	)
	return scanOpsAlertEvent(row)
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

	if IsMySQL() {
		_, err := r.db.ExecContext(ctx, `
UPDATE ops_alert_events
SET status = ?,
    resolved_at = ?
WHERE id = ?`, strings.TrimSpace(status), opsNullTime(resolvedAt), eventID)
		return err
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

	if IsMySQL() {
		_, err := r.db.ExecContext(ctx, "UPDATE ops_alert_events SET email_sent = ? WHERE id = ?", emailSent, eventID)
		return err
	}
	_, err := r.db.ExecContext(ctx, "UPDATE ops_alert_events SET email_sent = $2 WHERE id = $1", eventID, emailSent)
	return err
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

	if IsMySQL() {
		return r.createAlertSilenceMySQL(ctx, input, platform)
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

	if IsMySQL() {
		// IS NOT DISTINCT FROM → NOT (a <=> b) 取反的等价正向形式：(a <=> b)。
		q := `
SELECT 1
FROM ops_alert_silences
WHERE rule_id = ?
  AND platform = ?
  AND (group_id <=> ?)
  AND (region <=> ?)
  AND until > ?
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

// ---------------------------------------------------------------------------
// MySQL 方言实现（见 DIALECT_MYSQL_GUIDE.md）。
// ---------------------------------------------------------------------------

// opsAlertEventSelectListMySQL 是 ops_alert_events 行的 SELECT 列清单，
// 与 PG 版各查询的列清单逐列一致，供 MySQL 分支共用（避免多处重复）。
const opsAlertEventSelectListMySQL = `
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
  created_at`

// opsAlertRuleSelectListMySQL 是 ops_alert_rules 行的 SELECT 列清单（MySQL 分支回查用），
// 与 PG 版 RETURNING/列表列清单逐列一致。
const opsAlertRuleSelectListMySQL = `
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
  COALESCE(notify_email, true),
  filters,
  last_triggered_at,
  created_at,
  updated_at
FROM ops_alert_rules`

// scanOpsAlertRule 扫描 opsAlertRuleSelectListMySQL 对应的行，后处理与 PG 分支一致
// （last_triggered_at 装箱、filters JSON 反序列化）。
func scanOpsAlertRule(row opsAlertEventRow) (*service.OpsAlertRule, error) {
	var out service.OpsAlertRule
	var filtersRaw []byte
	var lastTriggeredAt sql.NullTime
	if err := row.Scan(
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
	if len(filtersRaw) > 0 && string(filtersRaw) != "null" {
		var decoded map[string]any
		if err := json.Unmarshal(filtersRaw, &decoded); err == nil {
			out.Filters = decoded
		}
	}
	return &out, nil
}

// createAlertRuleMySQL 是 CreateAlertRule 的 MySQL 实现（规范 RETURNING 改写第 2 类）：
// INSERT（NOW()→NOW(3)，$n→?）+ LastInsertId + 回查全部列，对齐 PG
// INSERT..RETURNING（created_at/updated_at 为 DB 生成的 NOW(3)）。
func (r *opsRepository) createAlertRuleMySQL(ctx context.Context, input *service.OpsAlertRule, filtersArg any) (*service.OpsAlertRule, error) {
	res, err := r.db.ExecContext(ctx, `
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
  notify_email,
  filters,
  created_at,
  updated_at
) VALUES (
  ?,?,?,?,?,?,?,?,?,?,?,?,NOW(3),NOW(3)
)`,
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
		input.NotifyEmail,
		filtersArg,
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	row := r.db.QueryRowContext(ctx, `SELECT `+opsAlertRuleSelectListMySQL+` WHERE id = ?`, id)
	return scanOpsAlertRule(row)
}

// updateAlertRuleMySQL 是 UpdateAlertRule 的 MySQL 实现（规范 RETURNING 改写第 4 类单行版）：
// UPDATE（NOW()→NOW(3)，$n→?）+ 回查该行，对齐 PG UPDATE..RETURNING（返回更新后的行）；
// id 不存在时回查返回 ErrNoRows，与 PG QueryRow().Scan 的行为一致。
func (r *opsRepository) updateAlertRuleMySQL(ctx context.Context, input *service.OpsAlertRule, filtersArg any) (*service.OpsAlertRule, error) {
	_, err := r.db.ExecContext(ctx, `
UPDATE ops_alert_rules
SET
  name = ?,
  description = ?,
  enabled = ?,
  severity = ?,
  metric_type = ?,
  operator = ?,
  threshold = ?,
  window_minutes = ?,
  sustained_minutes = ?,
  cooldown_minutes = ?,
  notify_email = ?,
  filters = ?,
  updated_at = NOW(3)
WHERE id = ?`,
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
		input.NotifyEmail,
		filtersArg,
		input.ID,
	)
	if err != nil {
		return nil, err
	}
	row := r.db.QueryRowContext(ctx, `SELECT `+opsAlertRuleSelectListMySQL+` WHERE id = ?`, input.ID)
	return scanOpsAlertRule(row)
}

// listAlertEventsMySQL 是 ListAlertEvents 的 MySQL 实现：
// buildOpsAlertEventsWhereMySQL（$n→?、dimensions->>'k' → JSON_UNQUOTE(JSON_EXTRACT(...))）；
// LIMIT $n → LIMIT ?；扫描逻辑与 PG 版一致。
func (r *opsRepository) listAlertEventsMySQL(ctx context.Context, filter *service.OpsAlertEventFilter, limit int) ([]*service.OpsAlertEvent, error) {
	where, args := buildOpsAlertEventsWhereMySQL(filter)
	args = append(args, limit)

	q := `
SELECT ` + opsAlertEventSelectListMySQL + `
FROM ops_alert_events
` + where + `
ORDER BY fired_at DESC, id DESC
LIMIT ?`

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

// createAlertEventMySQL 是 CreateAlertEvent 的 MySQL 实现（规范 RETURNING 改写第 2 类）：
// INSERT（created_at 由 DB 生成 NOW(3)，$n→?）+ LastInsertId + 回查全部列，
// 对齐 PG INSERT..RETURNING（含 DB 默认 created_at）。
func (r *opsRepository) createAlertEventMySQL(ctx context.Context, event *service.OpsAlertEvent, dimensionsArg any) (*service.OpsAlertEvent, error) {
	res, err := r.db.ExecContext(ctx, `
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
  created_at
) VALUES (
  ?,?,?,?,?,?,?,?,?,?,?,?,NOW(3)
)`,
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
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	row := r.db.QueryRowContext(ctx, `SELECT `+opsAlertEventSelectListMySQL+` FROM ops_alert_events WHERE id = ?`, id)
	return scanOpsAlertEvent(row)
}

// createAlertSilenceMySQL 是 CreateAlertSilence 的 MySQL 实现（规范 RETURNING 改写第 2 类）：
// INSERT（created_at 由 DB 生成 NOW(3)，$n→?）+ LastInsertId + 回查全部列，对齐 PG
// INSERT..RETURNING。
func (r *opsRepository) createAlertSilenceMySQL(ctx context.Context, input *service.OpsAlertSilence, platform string) (*service.OpsAlertSilence, error) {
	res, err := r.db.ExecContext(ctx, `
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
  ?,?,?,?,?,?,?,NOW(3)
)`,
		input.RuleID,
		platform,
		opsNullInt64(input.GroupID),
		opsNullString(input.Region),
		input.Until,
		opsNullString(input.Reason),
		opsNullInt64(input.CreatedBy),
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	row := r.db.QueryRowContext(ctx, `
SELECT id, rule_id, platform, group_id, region, until, COALESCE(reason,''), created_by, created_at
FROM ops_alert_silences
WHERE id = ?`, id)

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

// buildOpsAlertEventsWhereMySQL 是 buildOpsAlertEventsWhere 的 MySQL 版（$n→?）：
// dimensions->>'k' → JSON_UNQUOTE(JSON_EXTRACT(dimensions, '$.k'))
// （两侧对缺失键均得 NULL，等值比较语义一致）；
// 游标条件在 PG 版复用同一参数两次，MySQL 需按占位符个数重复追加参数。
func buildOpsAlertEventsWhereMySQL(filter *service.OpsAlertEventFilter) (string, []any) {
	clauses := []string{"1=1"}
	args := []any{}

	if filter == nil {
		return "WHERE " + strings.Join(clauses, " AND "), args
	}

	if status := strings.TrimSpace(filter.Status); status != "" {
		args = append(args, status)
		clauses = append(clauses, "status = ?")
	}
	if severity := strings.TrimSpace(filter.Severity); severity != "" {
		args = append(args, severity)
		clauses = append(clauses, "severity = ?")
	}
	if filter.EmailSent != nil {
		args = append(args, *filter.EmailSent)
		clauses = append(clauses, "email_sent = ?")
	}
	if filter.StartTime != nil && !filter.StartTime.IsZero() {
		args = append(args, *filter.StartTime)
		clauses = append(clauses, "fired_at >= ?")
	}
	if filter.EndTime != nil && !filter.EndTime.IsZero() {
		args = append(args, *filter.EndTime)
		clauses = append(clauses, "fired_at < ?")
	}

	// Cursor pagination (descending by fired_at, then id)
	if filter.BeforeFiredAt != nil && !filter.BeforeFiredAt.IsZero() && filter.BeforeID != nil && *filter.BeforeID > 0 {
		clauses = append(clauses, "(fired_at < ? OR (fired_at = ? AND id < ?))")
		args = append(args, *filter.BeforeFiredAt, *filter.BeforeFiredAt, *filter.BeforeID)
	}
	// Dimensions are stored in JSON. We filter best-effort without requiring functional indexes.
	if platform := strings.TrimSpace(filter.Platform); platform != "" {
		args = append(args, platform)
		clauses = append(clauses, "(JSON_UNQUOTE(JSON_EXTRACT(dimensions, '$.platform'))) = ?")
	}
	if filter.GroupID != nil && *filter.GroupID > 0 {
		args = append(args, fmt.Sprintf("%d", *filter.GroupID))
		clauses = append(clauses, "(JSON_UNQUOTE(JSON_EXTRACT(dimensions, '$.group_id'))) = ?")
	}

	return "WHERE " + strings.Join(clauses, " AND "), args
}
