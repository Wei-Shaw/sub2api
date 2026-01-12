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
REDACTED

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
REDACTED
	defer func() { _ = rows.Close() REDACTED()

	out := []*service.OpsAlertRule{REDACTED
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
	REDACTED
		if lastTriggeredAt.Valid {
			v := lastTriggeredAt.Time
			rule.LastTriggeredAt = &v
	REDACTED
		if len(filtersRaw) > 0 && string(filtersRaw) != "null" {
			var decoded map[string]any
			if err := json.Unmarshal(filtersRaw, &decoded); err == nil {
				rule.Filters = decoded
		REDACTED
	REDACTED
		out = append(out, &rule)
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED
	return out, nil
REDACTED

func (r *opsRepository) CreateAlertRule(ctx context.Context, input *service.OpsAlertRule) (*service.OpsAlertRule, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
REDACTED
	if input == nil {
		return nil, fmt.Errorf("nil input")
REDACTED

	filtersArg, err := opsNullJSONMap(input.Filters)
	if err != nil {
		return nil, err
REDACTED

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
REDACTED
	if lastTriggeredAt.Valid {
		v := lastTriggeredAt.Time
		out.LastTriggeredAt = &v
REDACTED
	if len(filtersRaw) > 0 && string(filtersRaw) != "null" {
		var decoded map[string]any
		if err := json.Unmarshal(filtersRaw, &decoded); err == nil {
			out.Filters = decoded
	REDACTED
REDACTED

	return &out, nil
REDACTED

func (r *opsRepository) UpdateAlertRule(ctx context.Context, input *service.OpsAlertRule) (*service.OpsAlertRule, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
REDACTED
	if input == nil {
		return nil, fmt.Errorf("nil input")
REDACTED
	if input.ID <= 0 {
		return nil, fmt.Errorf("invalid id")
REDACTED

	filtersArg, err := opsNullJSONMap(input.Filters)
	if err != nil {
		return nil, err
REDACTED

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
REDACTED

	if lastTriggeredAt.Valid {
		v := lastTriggeredAt.Time
		out.LastTriggeredAt = &v
REDACTED
	if len(filtersRaw) > 0 && string(filtersRaw) != "null" {
		var decoded map[string]any
		if err := json.Unmarshal(filtersRaw, &decoded); err == nil {
			out.Filters = decoded
	REDACTED
REDACTED

	return &out, nil
REDACTED

func (r *opsRepository) DeleteAlertRule(ctx context.Context, id int64) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil ops repository")
REDACTED
	if id <= 0 {
		return fmt.Errorf("invalid id")
REDACTED

	res, err := r.db.ExecContext(ctx, "DELETE FROM ops_alert_rules WHERE id = $1", id)
	if err != nil {
		return err
REDACTED
	affected, err := res.RowsAffected()
	if err != nil {
		return err
REDACTED
	if affected == 0 {
		return sql.ErrNoRows
REDACTED
	return nil
REDACTED

func (r *opsRepository) ListAlertEvents(ctx context.Context, filter *service.OpsAlertEventFilter) ([]*service.OpsAlertEvent, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
REDACTED
	if filter == nil {
		filter = &service.OpsAlertEventFilter{REDACTED
REDACTED

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
REDACTED
	if limit > 500 {
		limit = 500
REDACTED

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
ORDER BY fired_at DESC
LIMIT ` + limitArg

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()

	out := []*service.OpsAlertEvent{REDACTED
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
	REDACTED
		if metricValue.Valid {
			v := metricValue.Float64
			ev.MetricValue = &v
	REDACTED
		if thresholdValue.Valid {
			v := thresholdValue.Float64
			ev.ThresholdValue = &v
	REDACTED
		if resolvedAt.Valid {
			v := resolvedAt.Time
			ev.ResolvedAt = &v
	REDACTED
		if len(dimensionsRaw) > 0 && string(dimensionsRaw) != "null" {
			var decoded map[string]any
			if err := json.Unmarshal(dimensionsRaw, &decoded); err == nil {
				ev.Dimensions = decoded
		REDACTED
	REDACTED
		out = append(out, &ev)
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED
	return out, nil
REDACTED

func (r *opsRepository) GetActiveAlertEvent(ctx context.Context, ruleID int64) (*service.OpsAlertEvent, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
REDACTED
	if ruleID <= 0 {
		return nil, fmt.Errorf("invalid rule id")
REDACTED

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
	REDACTED
		return nil, err
REDACTED
	return ev, nil
REDACTED

func (r *opsRepository) GetLatestAlertEvent(ctx context.Context, ruleID int64) (*service.OpsAlertEvent, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
REDACTED
	if ruleID <= 0 {
		return nil, fmt.Errorf("invalid rule id")
REDACTED

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
	REDACTED
		return nil, err
REDACTED
	return ev, nil
REDACTED

func (r *opsRepository) CreateAlertEvent(ctx context.Context, event *service.OpsAlertEvent) (*service.OpsAlertEvent, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
REDACTED
	if event == nil {
		return nil, fmt.Errorf("nil event")
REDACTED

	dimensionsArg, err := opsNullJSONMap(event.Dimensions)
	if err != nil {
		return nil, err
REDACTED

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
REDACTED

func (r *opsRepository) UpdateAlertEventStatus(ctx context.Context, eventID int64, status string, resolvedAt *time.Time) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil ops repository")
REDACTED
	if eventID <= 0 {
		return fmt.Errorf("invalid event id")
REDACTED
	if strings.TrimSpace(status) == "" {
		return fmt.Errorf("invalid status")
REDACTED

	q := `
UPDATE ops_alert_events
SET status = $2,
    resolved_at = $3
WHERE id = $1`

	_, err := r.db.ExecContext(ctx, q, eventID, strings.TrimSpace(status), opsNullTime(resolvedAt))
	return err
REDACTED

func (r *opsRepository) UpdateAlertEventEmailSent(ctx context.Context, eventID int64, emailSent bool) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil ops repository")
REDACTED
	if eventID <= 0 {
		return fmt.Errorf("invalid event id")
REDACTED

	_, err := r.db.ExecContext(ctx, "UPDATE ops_alert_events SET email_sent = $2 WHERE id = $1", eventID, emailSent)
	return err
REDACTED

type opsAlertEventRow interface {
	Scan(dest ...any) error
REDACTED

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
REDACTED
	if metricValue.Valid {
		v := metricValue.Float64
		ev.MetricValue = &v
REDACTED
	if thresholdValue.Valid {
		v := thresholdValue.Float64
		ev.ThresholdValue = &v
REDACTED
	if resolvedAt.Valid {
		v := resolvedAt.Time
		ev.ResolvedAt = &v
REDACTED
	if len(dimensionsRaw) > 0 && string(dimensionsRaw) != "null" {
		var decoded map[string]any
		if err := json.Unmarshal(dimensionsRaw, &decoded); err == nil {
			ev.Dimensions = decoded
	REDACTED
REDACTED
	return &ev, nil
REDACTED

func buildOpsAlertEventsWhere(filter *service.OpsAlertEventFilter) (string, []any) {
	clauses := []string{"1=1"REDACTED
	args := []any{REDACTED

	if filter == nil {
		return "WHERE " + strings.Join(clauses, " AND "), args
REDACTED

	if status := strings.TrimSpace(filter.Status); status != "" {
		args = append(args, status)
		clauses = append(clauses, "status = $"+itoa(len(args)))
REDACTED
	if severity := strings.TrimSpace(filter.Severity); severity != "" {
		args = append(args, severity)
		clauses = append(clauses, "severity = $"+itoa(len(args)))
REDACTED
	if filter.StartTime != nil && !filter.StartTime.IsZero() {
		args = append(args, *filter.StartTime)
		clauses = append(clauses, "fired_at >= $"+itoa(len(args)))
REDACTED
	if filter.EndTime != nil && !filter.EndTime.IsZero() {
		args = append(args, *filter.EndTime)
		clauses = append(clauses, "fired_at < $"+itoa(len(args)))
REDACTED

	// Dimensions are stored in JSONB. We filter best-effort without requiring GIN indexes.
	if platform := strings.TrimSpace(filter.Platform); platform != "" {
		args = append(args, platform)
		clauses = append(clauses, "(dimensions->>'platform') = $"+itoa(len(args)))
REDACTED
	if filter.GroupID != nil && *filter.GroupID > 0 {
		args = append(args, fmt.Sprintf("%d", *filter.GroupID))
		clauses = append(clauses, "(dimensions->>'group_id') = $"+itoa(len(args)))
REDACTED

	return "WHERE " + strings.Join(clauses, " AND "), args
REDACTED

func opsNullJSONMap(v map[string]any) (any, error) {
	if v == nil {
		return sql.NullString{REDACTED, nil
REDACTED
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
REDACTED
	if len(b) == 0 {
		return sql.NullString{REDACTED, nil
REDACTED
	return sql.NullString{String: string(b), Valid: trueREDACTED, nil
REDACTED
