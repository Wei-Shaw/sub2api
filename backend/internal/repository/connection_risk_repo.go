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

type connectionRiskRepository struct {
	db *sql.DB
}

// NewConnectionRiskEventRepository creates the raw-SQL risk event store.
func NewConnectionRiskEventRepository(db *sql.DB) service.ConnectionRiskEventRepository {
	return &connectionRiskRepository{db: db}
}

const connectionRiskSelectColumns = `
 id, created_at, updated_at, subject_type, user_id, api_key_id, api_key_prefix,
 rules_fired, severity, score, status, title, summary, evidence, metrics,
 dedupe_key, action_taken, resolver_id, resolved_at,
 first_seen_at, last_seen_at, window_start, window_end`

// UpsertOpen inserts a new open event or refreshes last_seen on conflict of open dedupe_key.
func (r *connectionRiskRepository) UpsertOpen(ctx context.Context, event *service.ConnectionRiskEvent) (*service.ConnectionRiskEvent, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil connection risk repository")
	}
	if event == nil {
		return nil, fmt.Errorf("nil event")
	}
	now := time.Now().UTC()
	if event.FirstSeenAt.IsZero() {
		event.FirstSeenAt = now
	}
	if event.LastSeenAt.IsZero() {
		event.LastSeenAt = now
	}
	if event.Status == "" {
		event.Status = service.ConnectionRiskStatusOpen
	}
	if event.ActionTaken == "" {
		event.ActionTaken = service.ConnectionRiskActionNone
	}
	rulesJSON, _ := json.Marshal(event.RulesFired)
	if rulesJSON == nil {
		rulesJSON = []byte("[]")
	}
	evidenceJSON, _ := json.Marshal(event.Evidence)
	if evidenceJSON == nil {
		evidenceJSON = []byte("{}")
	}
	metricsJSON, _ := json.Marshal(event.Metrics)
	if metricsJSON == nil {
		metricsJSON = []byte("{}")
	}

	// Try update existing open row by dedupe_key first.
	if event.DedupeKey != "" {
		const upd = `
UPDATE connection_risk_events
SET last_seen_at = $1,
    updated_at = $1,
    score = GREATEST(score, $2),
    severity = CASE
      WHEN $3 = 'critical' THEN 'critical'
      WHEN severity = 'critical' THEN severity
      WHEN $3 = 'high' THEN 'high'
      WHEN severity = 'high' THEN severity
      WHEN $3 = 'medium' THEN 'medium'
      ELSE severity
    END,
    rules_fired = $4,
    evidence = $5,
    metrics = $6,
    summary = $7,
    title = $8
WHERE dedupe_key = $9 AND status = 'open'
RETURNING` + connectionRiskSelectColumns
		row := r.db.QueryRowContext(ctx, upd,
			event.LastSeenAt.UTC(),
			event.Score,
			event.Severity,
			rulesJSON,
			evidenceJSON,
			metricsJSON,
			event.Summary,
			event.Title,
			event.DedupeKey,
		)
		existing, err := scanConnectionRiskEvent(row.Scan)
		if err == nil {
			return existing, nil
		}
		if err != sql.ErrNoRows {
			return nil, err
		}
		// No open row for this dedupe_key — insert below.
	}

	const ins = `
INSERT INTO connection_risk_events (
  subject_type, user_id, api_key_id, api_key_prefix,
  rules_fired, severity, score, status, title, summary, evidence, metrics,
  dedupe_key, action_taken, first_seen_at, last_seen_at, window_start, window_end,
  created_at, updated_at
) VALUES (
  $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$19
)
RETURNING` + connectionRiskSelectColumns

	row := r.db.QueryRowContext(ctx, ins,
		event.SubjectType,
		nullInt64Ptr(event.UserID),
		nullInt64Ptr(event.APIKeyID),
		truncateString(event.APIKeyPrefix, 32),
		rulesJSON,
		truncateString(event.Severity, 16),
		event.Score,
		truncateString(event.Status, 16),
		truncateString(event.Title, 256),
		event.Summary,
		evidenceJSON,
		metricsJSON,
		truncateString(event.DedupeKey, 191),
		truncateString(event.ActionTaken, 64),
		event.FirstSeenAt.UTC(),
		event.LastSeenAt.UTC(),
		nullTimePtr(event.WindowStart),
		nullTimePtr(event.WindowEnd),
		now,
	)
	return scanConnectionRiskEvent(row.Scan)
}

func (r *connectionRiskRepository) GetByID(ctx context.Context, id int64) (*service.ConnectionRiskEvent, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil connection risk repository")
	}
	row := r.db.QueryRowContext(ctx, `SELECT`+connectionRiskSelectColumns+` FROM connection_risk_events WHERE id = $1`, id)
	ev, err := scanConnectionRiskEvent(row.Scan)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return ev, err
}

func (r *connectionRiskRepository) List(ctx context.Context, filter *service.ConnectionRiskEventFilter) (*service.ConnectionRiskEventList, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil connection risk repository")
	}
	if filter == nil {
		filter = &service.ConnectionRiskEventFilter{}
	}
	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	where, args := buildConnectionRiskWhere(filter)
	countSQL := "SELECT COUNT(*) FROM connection_risk_events e " + where
	var total int64
	if err := r.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, err
	}

	offset := (page - 1) * pageSize
	args = append(args, pageSize, offset)
	query := `SELECT` + connectionRiskSelectColumns + `
FROM connection_risk_events e
` + where + `
ORDER BY e.last_seen_at DESC, e.id DESC
LIMIT $` + fmt.Sprintf("%d", len(args)-1) + ` OFFSET $` + fmt.Sprintf("%d", len(args))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]*service.ConnectionRiskEvent, 0, pageSize)
	for rows.Next() {
		ev, err := scanConnectionRiskEvent(rows.Scan)
		if err != nil {
			return nil, err
		}
		items = append(items, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &service.ConnectionRiskEventList{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (r *connectionRiskRepository) UpdateStatus(ctx context.Context, id int64, status string, resolverID *int64) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil connection risk repository")
	}
	now := time.Now().UTC()
	var resolvedAt any
	if status == service.ConnectionRiskStatusResolved || status == service.ConnectionRiskStatusSuppressed {
		resolvedAt = now
	} else {
		resolvedAt = nil
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE connection_risk_events
SET status = $1, resolver_id = $2, resolved_at = $3, updated_at = $4
WHERE id = $5`, status, nullInt64Ptr(resolverID), resolvedAt, now, id)
	return err
}

func (r *connectionRiskRepository) Delete(ctx context.Context, id int64) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil connection risk repository")
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM connection_risk_events WHERE id = $1`, id)
	return err
}

func (r *connectionRiskRepository) DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	if r == nil || r.db == nil {
		return 0, fmt.Errorf("nil connection risk repository")
	}
	res, err := r.db.ExecContext(ctx, `DELETE FROM connection_risk_events WHERE last_seen_at < $1`, cutoff.UTC())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func buildConnectionRiskWhere(filter *service.ConnectionRiskEventFilter) (string, []any) {
	var parts []string
	var args []any
	idx := 1
	if s := strings.TrimSpace(filter.Status); s != "" {
		parts = append(parts, fmt.Sprintf("e.status = $%d", idx))
		args = append(args, s)
		idx++
	}
	if s := strings.TrimSpace(filter.Severity); s != "" {
		parts = append(parts, fmt.Sprintf("e.severity = $%d", idx))
		args = append(args, s)
		idx++
	}
	if filter.UserID != nil {
		parts = append(parts, fmt.Sprintf("e.user_id = $%d", idx))
		args = append(args, *filter.UserID)
		idx++
	}
	if filter.APIKeyID != nil {
		parts = append(parts, fmt.Sprintf("e.api_key_id = $%d", idx))
		args = append(args, *filter.APIKeyID)
		idx++
	}
	if rule := strings.TrimSpace(filter.Rule); rule != "" {
		parts = append(parts, fmt.Sprintf("e.rules_fired @> $%d::jsonb", idx))
		// Match any hit with this rule_id
		payload, _ := json.Marshal([]map[string]string{{"rule_id": rule}})
		args = append(args, string(payload))
		idx++
	}
	_ = idx
	if len(parts) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(parts, " AND "), args
}

func scanConnectionRiskEvent(scan func(dest ...any) error) (*service.ConnectionRiskEvent, error) {
	var (
		ev                                 service.ConnectionRiskEvent
		userID, apiKeyID, resolverID       sql.NullInt64
		rulesRaw, evidenceRaw, metricsRaw  []byte
		resolvedAt, windowStart, windowEnd sql.NullTime
	)
	err := scan(
		&ev.ID,
		&ev.CreatedAt,
		&ev.UpdatedAt,
		&ev.SubjectType,
		&userID,
		&apiKeyID,
		&ev.APIKeyPrefix,
		&rulesRaw,
		&ev.Severity,
		&ev.Score,
		&ev.Status,
		&ev.Title,
		&ev.Summary,
		&evidenceRaw,
		&metricsRaw,
		&ev.DedupeKey,
		&ev.ActionTaken,
		&resolverID,
		&resolvedAt,
		&ev.FirstSeenAt,
		&ev.LastSeenAt,
		&windowStart,
		&windowEnd,
	)
	if err != nil {
		return nil, err
	}
	if userID.Valid {
		v := userID.Int64
		ev.UserID = &v
	}
	if apiKeyID.Valid {
		v := apiKeyID.Int64
		ev.APIKeyID = &v
	}
	if resolverID.Valid {
		v := resolverID.Int64
		ev.ResolverID = &v
	}
	if resolvedAt.Valid {
		v := resolvedAt.Time
		ev.ResolvedAt = &v
	}
	if windowStart.Valid {
		v := windowStart.Time
		ev.WindowStart = &v
	}
	if windowEnd.Valid {
		v := windowEnd.Time
		ev.WindowEnd = &v
	}
	if len(rulesRaw) > 0 {
		_ = json.Unmarshal(rulesRaw, &ev.RulesFired)
	}
	if len(evidenceRaw) > 0 {
		_ = json.Unmarshal(evidenceRaw, &ev.Evidence)
	}
	if len(metricsRaw) > 0 {
		_ = json.Unmarshal(metricsRaw, &ev.Metrics)
	}
	if ev.Evidence == nil {
		ev.Evidence = map[string]any{}
	}
	if ev.Metrics == nil {
		ev.Metrics = map[string]any{}
	}
	return &ev, nil
}

func nullTimePtr(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return t.UTC()
}
