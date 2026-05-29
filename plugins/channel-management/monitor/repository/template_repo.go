package monitorrepository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/plugins/channel-management/monitor/internal/bodymode"
	monitorservice "github.com/Wei-Shaw/sub2api/plugins/channel-management/monitor/service"
)

// channelMonitorTemplateRepository implements the request-template data
// access interface. SQL bodies are ported from upstream commit 09fd83ab
// backend/internal/repository/channel_monitor_template_repo.go: the ent
// builder calls become raw INSERT/SELECT/UPDATE statements against the
// SDK-provided *sql.DB.
type channelMonitorTemplateRepository struct {
	db *sql.DB
}

// NewChannelMonitorTemplateRepository wires the template repo on top of the
// SDK DB handle.
func NewChannelMonitorTemplateRepository(db *sql.DB) monitorservice.ChannelMonitorRequestTemplateRepository {
	return &channelMonitorTemplateRepository{db: db}
}

const channelMonitorTemplateColumns = `id, name, provider, description, extra_headers,
	body_override_mode, body_override, created_at, updated_at`

func (r *channelMonitorTemplateRepository) Create(ctx context.Context, t *monitorservice.ChannelMonitorRequestTemplate) error {
	headers, err := marshalJSONBStringMap(t.ExtraHeaders)
	if err != nil {
		return fmt.Errorf("marshal extra_headers: %w", err)
	}
	bodyOverride, err := marshalJSONBOptional(t.BodyOverride)
	if err != nil {
		return fmt.Errorf("marshal body_override: %w", err)
	}
	const q = `
		INSERT INTO channel_monitor_request_templates
			(name, provider, description, extra_headers, body_override_mode, body_override)
		VALUES
			($1, $2, $3, $4::jsonb, $5, $6::jsonb)
		RETURNING id, created_at, updated_at
	`
	var bodyArg any
	if bodyOverride != nil {
		bodyArg = string(bodyOverride)
	}
	return r.db.QueryRowContext(ctx, q,
		t.Name, t.Provider, t.Description, string(headers), bodymode.Normalize(t.BodyOverrideMode), bodyArg,
	).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
}

func (r *channelMonitorTemplateRepository) GetByID(ctx context.Context, id int64) (*monitorservice.ChannelMonitorRequestTemplate, error) {
	q := `SELECT ` + channelMonitorTemplateColumns + ` FROM channel_monitor_request_templates WHERE id = $1`
	row := r.db.QueryRowContext(ctx, q, id)
	t, err := scanTemplateRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, monitorservice.ErrChannelMonitorTemplateNotFound
		}
		return nil, fmt.Errorf("get template by id: %w", err)
	}
	return t, nil
}

func (r *channelMonitorTemplateRepository) Update(ctx context.Context, t *monitorservice.ChannelMonitorRequestTemplate) error {
	headers, err := marshalJSONBStringMap(t.ExtraHeaders)
	if err != nil {
		return fmt.Errorf("marshal extra_headers: %w", err)
	}
	bodyOverride, err := marshalJSONBOptional(t.BodyOverride)
	if err != nil {
		return fmt.Errorf("marshal body_override: %w", err)
	}
	const q = `
		UPDATE channel_monitor_request_templates SET
			name=$2, description=$3, extra_headers=$4::jsonb,
			body_override_mode=$5, body_override=$6::jsonb, updated_at=NOW()
		WHERE id=$1
		RETURNING updated_at
	`
	var bodyArg any
	if bodyOverride != nil {
		bodyArg = string(bodyOverride)
	}
	if err := r.db.QueryRowContext(ctx, q,
		t.ID, t.Name, t.Description, string(headers), bodymode.Normalize(t.BodyOverrideMode), bodyArg,
	).Scan(&t.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return monitorservice.ErrChannelMonitorTemplateNotFound
		}
		return fmt.Errorf("update template: %w", err)
	}
	return nil
}

func (r *channelMonitorTemplateRepository) Delete(ctx context.Context, id int64) error {
	const q = `DELETE FROM channel_monitor_request_templates WHERE id = $1`
	res, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return fmt.Errorf("delete template: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete template rows affected: %w", err)
	}
	if n == 0 {
		return monitorservice.ErrChannelMonitorTemplateNotFound
	}
	return nil
}

func (r *channelMonitorTemplateRepository) List(ctx context.Context, params monitorservice.ChannelMonitorRequestTemplateListParams) ([]*monitorservice.ChannelMonitorRequestTemplate, error) {
	conds := []string{}
	args := []any{}
	if params.Provider != "" {
		args = append(args, params.Provider)
		conds = append(conds, fmt.Sprintf("provider = $%d", len(args)))
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}
	q := `SELECT ` + channelMonitorTemplateColumns +
		` FROM channel_monitor_request_templates` + where +
		` ORDER BY provider ASC, name ASC`

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list monitor templates: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]*monitorservice.ChannelMonitorRequestTemplate, 0)
	for rows.Next() {
		t, err := scanTemplateRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan template list row: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ApplyToMonitors copies the template's headers / body override snapshot to
// every monitor whose template_id matches the template id AND whose id is in
// the provided whitelist. The double WHERE filter mirrors the upstream ent
// implementation: callers cannot accidentally splat a template onto monitors
// that aren't currently associated with it.
func (r *channelMonitorTemplateRepository) ApplyToMonitors(ctx context.Context, id int64, monitorIDs []int64) (int64, error) {
	if len(monitorIDs) == 0 {
		return 0, nil
	}
	tpl, err := r.GetByID(ctx, id)
	if err != nil {
		return 0, err
	}
	headers, err := marshalJSONBStringMap(tpl.ExtraHeaders)
	if err != nil {
		return 0, fmt.Errorf("marshal extra_headers: %w", err)
	}
	bodyOverride, err := marshalJSONBOptional(tpl.BodyOverride)
	if err != nil {
		return 0, fmt.Errorf("marshal body_override: %w", err)
	}
	// Build a positional id list so PG can plan an IN (...) — the SDK driver
	// does not support int64 array arguments directly through database/sql,
	// so we expand the slice into ($N, $N+1, ...) placeholders and append
	// each id as a stand-alone arg.
	idArgs := make([]any, 0, len(monitorIDs))
	idPlaceholders := make([]string, 0, len(monitorIDs))
	for _, mid := range monitorIDs {
		idArgs = append(idArgs, mid)
		idPlaceholders = append(idPlaceholders, fmt.Sprintf("$%d", len(idArgs)+3))
	}
	q := fmt.Sprintf(`
		UPDATE channel_monitors SET
			extra_headers=$1::jsonb,
			body_override_mode=$2,
			body_override=$3::jsonb,
			updated_at=NOW()
		WHERE template_id=%d
		  AND id IN (%s)
	`, id, strings.Join(idPlaceholders, ","))

	var bodyArg any
	if bodyOverride != nil {
		bodyArg = string(bodyOverride)
	}
	args := append([]any{string(headers), bodymode.Normalize(tpl.BodyOverrideMode), bodyArg}, idArgs...)

	res, err := r.db.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, fmt.Errorf("apply template to monitors: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("apply rows affected: %w", err)
	}
	return n, nil
}

func (r *channelMonitorTemplateRepository) CountAssociatedMonitors(ctx context.Context, id int64) (int64, error) {
	const q = `SELECT COUNT(*) FROM channel_monitors WHERE template_id = $1`
	var n int64
	if err := r.db.QueryRowContext(ctx, q, id).Scan(&n); err != nil {
		return 0, fmt.Errorf("count monitors for template %d: %w", id, err)
	}
	return n, nil
}

func (r *channelMonitorTemplateRepository) ListAssociatedMonitors(ctx context.Context, id int64) ([]*monitorservice.AssociatedMonitorBrief, error) {
	const q = `
		SELECT id, name, provider, COALESCE(api_mode, ''), enabled
		FROM channel_monitors
		WHERE template_id = $1
		ORDER BY name ASC
	`
	rows, err := r.db.QueryContext(ctx, q, id)
	if err != nil {
		return nil, fmt.Errorf("list associated monitors for template %d: %w", id, err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]*monitorservice.AssociatedMonitorBrief, 0)
	for rows.Next() {
		brief := &monitorservice.AssociatedMonitorBrief{}
		if err := rows.Scan(&brief.ID, &brief.Name, &brief.Provider, &brief.APIMode, &brief.Enabled); err != nil {
			return nil, fmt.Errorf("scan associated monitor row: %w", err)
		}
		out = append(out, brief)
	}
	return out, rows.Err()
}

// scanTemplateRow projects a row in channelMonitorTemplateColumns order
// into a service model.
func scanTemplateRow(r scanRow) (*monitorservice.ChannelMonitorRequestTemplate, error) {
	t := &monitorservice.ChannelMonitorRequestTemplate{}
	var (
		headersRaw []byte
		bodyRaw    sql.NullString
		bodyMode   sql.NullString
	)
	if err := r.Scan(
		&t.ID, &t.Name, &t.Provider, &t.Description,
		&headersRaw, &bodyMode, &bodyRaw, &t.CreatedAt, &t.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if bodyMode.Valid {
		t.BodyOverrideMode = bodyMode.String
	}
	if err := unmarshalJSONBStringMap(headersRaw, &t.ExtraHeaders); err != nil {
		return nil, fmt.Errorf("decode extra_headers: %w", err)
	}
	if t.ExtraHeaders == nil {
		t.ExtraHeaders = map[string]string{}
	}
	if bodyRaw.Valid && strings.TrimSpace(bodyRaw.String) != "" {
		var body map[string]any
		if err := json.Unmarshal([]byte(bodyRaw.String), &body); err != nil {
			return nil, fmt.Errorf("decode body_override: %w", err)
		}
		t.BodyOverride = body
	}
	return t, nil
}
