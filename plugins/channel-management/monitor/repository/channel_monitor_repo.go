// Package monitorrepository implements the channel-monitor data access layer
// on top of the SDK-provided *sql.DB. The host plugin module does not ship
// its own ent client, so all queries are written against PostgreSQL via raw
// SQL (in line with V5-CURATE D5 — "no plugin ent module in V5").
//
// SQL bodies are ported from the upstream commit 09fd83ab implementation in
// backend/internal/repository/channel_monitor_repo.go. The CRUD functions
// (Create / GetByID / Update / Delete / List / ListEnabled / MarkChecked)
// re-express the ent builder calls as raw INSERT/SELECT/UPDATE statements;
// the history / aggregation / rollup queries are split into sibling files
// (monitor_history_repo.go, monitor_aggregation_repo.go,
// monitor_rollup_repo.go) — same package, same receiver — to keep each
// file focused on a single responsibility per T18.
package monitorrepository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/plugins/channel-management/monitor/internal/bodymode"
	monitorservice "github.com/Wei-Shaw/sub2api/plugins/channel-management/monitor/service"
)

// channelMonitorRepository is the channel-monitor data access implementation.
// db is the SDK-provided handle whose driver proxies queries through gRPC
// back to the host's connection pool.
type channelMonitorRepository struct {
	db *sql.DB
}

// NewChannelMonitorRepository wires the repo on top of the SDK DB handle.
func NewChannelMonitorRepository(db *sql.DB) monitorservice.ChannelMonitorRepository {
	return &channelMonitorRepository{db: db}
}

// channelMonitorColumns lists the columns selected when scanning a full
// monitor row. Centralised so SELECT and scanMonitorRow stay in sync.
const channelMonitorColumns = `id, name, provider, endpoint, api_key_encrypted, primary_model,
	extra_models, group_name, enabled, interval_seconds, last_checked_at,
	created_by, created_at, updated_at,
	template_id, extra_headers, body_override_mode, body_override`

// ---------- CRUD ----------

func (r *channelMonitorRepository) Create(ctx context.Context, m *monitorservice.ChannelMonitor) error {
	extras, err := marshalJSONBSlice(m.ExtraModels)
	if err != nil {
		return fmt.Errorf("marshal extra_models: %w", err)
	}
	headers, err := marshalJSONBStringMap(m.ExtraHeaders)
	if err != nil {
		return fmt.Errorf("marshal extra_headers: %w", err)
	}
	bodyOverride, err := marshalJSONBOptional(m.BodyOverride)
	if err != nil {
		return fmt.Errorf("marshal body_override: %w", err)
	}
	// body_override is JSONB NULLABLE. We send a NULL placeholder when the
	// service didn't provide a value to avoid PostgreSQL coercing the empty
	// string to invalid JSON; otherwise the JSON text is cast via $14::jsonb.
	const q = `
		INSERT INTO channel_monitors (
			name, provider, endpoint, api_key_encrypted, primary_model,
			extra_models, group_name, enabled, interval_seconds, created_by,
			template_id, extra_headers, body_override_mode, body_override
		) VALUES (
			$1, $2, $3, $4, $5,
			$6::jsonb, $7, $8, $9, $10,
			$11, $12::jsonb, $13, $14::jsonb
		)
		RETURNING id, created_at, updated_at
	`
	var bodyArg any
	if bodyOverride != nil {
		bodyArg = string(bodyOverride)
	}
	return r.db.QueryRowContext(ctx, q,
		m.Name, m.Provider, m.Endpoint, m.APIKey, m.PrimaryModel,
		string(extras), m.GroupName, m.Enabled, m.IntervalSeconds, m.CreatedBy,
		m.TemplateID, string(headers), bodymode.Normalize(m.BodyOverrideMode), bodyArg,
	).Scan(&m.ID, &m.CreatedAt, &m.UpdatedAt)
}

func (r *channelMonitorRepository) GetByID(ctx context.Context, id int64) (*monitorservice.ChannelMonitor, error) {
	const q = `SELECT ` + channelMonitorColumns + ` FROM channel_monitors WHERE id = $1`
	row := r.db.QueryRowContext(ctx, q, id)
	m, err := scanMonitorRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, monitorservice.ErrChannelMonitorNotFound
		}
		return nil, fmt.Errorf("get monitor by id: %w", err)
	}
	return m, nil
}

func (r *channelMonitorRepository) Update(ctx context.Context, m *monitorservice.ChannelMonitor) error {
	extras, err := marshalJSONBSlice(m.ExtraModels)
	if err != nil {
		return fmt.Errorf("marshal extra_models: %w", err)
	}
	headers, err := marshalJSONBStringMap(m.ExtraHeaders)
	if err != nil {
		return fmt.Errorf("marshal extra_headers: %w", err)
	}
	bodyOverride, err := marshalJSONBOptional(m.BodyOverride)
	if err != nil {
		return fmt.Errorf("marshal body_override: %w", err)
	}
	const q = `
		UPDATE channel_monitors SET
			name=$2, provider=$3, endpoint=$4, api_key_encrypted=$5, primary_model=$6,
			extra_models=$7::jsonb, group_name=$8, enabled=$9, interval_seconds=$10,
			template_id=$11, extra_headers=$12::jsonb, body_override_mode=$13,
			body_override=$14::jsonb, updated_at=NOW()
		WHERE id=$1
		RETURNING updated_at
	`
	var bodyArg any
	if bodyOverride != nil {
		bodyArg = string(bodyOverride)
	}
	if err := r.db.QueryRowContext(ctx, q,
		m.ID, m.Name, m.Provider, m.Endpoint, m.APIKey, m.PrimaryModel,
		string(extras), m.GroupName, m.Enabled, m.IntervalSeconds,
		m.TemplateID, string(headers), bodymode.Normalize(m.BodyOverrideMode), bodyArg,
	).Scan(&m.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return monitorservice.ErrChannelMonitorNotFound
		}
		return fmt.Errorf("update monitor: %w", err)
	}
	return nil
}

func (r *channelMonitorRepository) Delete(ctx context.Context, id int64) error {
	const q = `DELETE FROM channel_monitors WHERE id = $1`
	res, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return fmt.Errorf("delete monitor: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete monitor rows affected: %w", err)
	}
	if n == 0 {
		return monitorservice.ErrChannelMonitorNotFound
	}
	return nil
}

func (r *channelMonitorRepository) List(ctx context.Context, params monitorservice.ChannelMonitorListParams) ([]*monitorservice.ChannelMonitor, int64, error) {
	where, args := buildListWhere(params)

	var total int64
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM channel_monitors `+where, args...,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count monitors: %w", err)
	}

	pageSize := params.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	page := params.Page
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * pageSize

	q := `SELECT ` + channelMonitorColumns + ` FROM channel_monitors ` + where +
		fmt.Sprintf(` ORDER BY id DESC LIMIT %d OFFSET %d`, pageSize, offset)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list monitors: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]*monitorservice.ChannelMonitor, 0)
	for rows.Next() {
		m, err := scanMonitorRow(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan monitor list row: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// buildListWhere builds the WHERE clause + positional args for List filters.
func buildListWhere(params monitorservice.ChannelMonitorListParams) (string, []any) {
	conds := []string{}
	args := []any{}
	if params.Provider != "" {
		args = append(args, params.Provider)
		conds = append(conds, fmt.Sprintf("provider = $%d", len(args)))
	}
	if params.Enabled != nil {
		args = append(args, *params.Enabled)
		conds = append(conds, fmt.Sprintf("enabled = $%d", len(args)))
	}
	if s := strings.TrimSpace(params.Search); s != "" {
		args = append(args, "%"+s+"%")
		idx := len(args)
		conds = append(conds, fmt.Sprintf(
			"(name ILIKE $%d OR group_name ILIKE $%d OR primary_model ILIKE $%d)",
			idx, idx, idx,
		))
	}
	if len(conds) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(conds, " AND "), args
}

// ---------- Scheduler helpers ----------

func (r *channelMonitorRepository) ListEnabled(ctx context.Context) ([]*monitorservice.ChannelMonitor, error) {
	q := `SELECT ` + channelMonitorColumns + ` FROM channel_monitors WHERE enabled = TRUE`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list enabled monitors: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]*monitorservice.ChannelMonitor, 0)
	for rows.Next() {
		m, err := scanMonitorRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan enabled monitor row: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *channelMonitorRepository) MarkChecked(ctx context.Context, id int64, checkedAt time.Time) error {
	const q = `UPDATE channel_monitors SET last_checked_at = $2 WHERE id = $1`
	res, err := r.db.ExecContext(ctx, q, id, checkedAt)
	if err != nil {
		return fmt.Errorf("mark checked: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark checked rows affected: %w", err)
	}
	if n == 0 {
		return monitorservice.ErrChannelMonitorNotFound
	}
	return nil
}

// ---------- 通用 helpers ----------
//
// 以下 scanMonitorRow / nullableInt / assignNullInt / JSONB helpers /
// deleteChannelMonitorBatched 被本文件以及 sibling files
// (monitor_history_repo.go / monitor_aggregation_repo.go /
// monitor_rollup_repo.go) 共用，因此保留在主文件中。

// scanRow is the minimal interface needed by scanMonitorRow so the helper
// can be shared between *sql.Row (Get) and *sql.Rows (List).
type scanRow interface {
	Scan(dest ...any) error
}

// scanMonitorRow projects a row in channelMonitorColumns order into a service
// model. Centralised so SELECT and Scan call sites stay in sync.
func scanMonitorRow(r scanRow) (*monitorservice.ChannelMonitor, error) {
	m := &monitorservice.ChannelMonitor{}
	var (
		extrasRaw   []byte
		headersRaw  []byte
		bodyRaw     sql.NullString
		lastChecked sql.NullTime
		bodyMode    sql.NullString
		templateID  sql.NullInt64
		groupName   sql.NullString
	)
	if err := r.Scan(
		&m.ID, &m.Name, &m.Provider, &m.Endpoint, &m.APIKey, &m.PrimaryModel,
		&extrasRaw, &groupName, &m.Enabled, &m.IntervalSeconds, &lastChecked,
		&m.CreatedBy, &m.CreatedAt, &m.UpdatedAt,
		&templateID, &headersRaw, &bodyMode, &bodyRaw,
	); err != nil {
		return nil, err
	}
	m.GroupName = groupName.String
	if lastChecked.Valid {
		t := lastChecked.Time
		m.LastCheckedAt = &t
	}
	if templateID.Valid {
		id := templateID.Int64
		m.TemplateID = &id
	}
	if bodyMode.Valid {
		m.BodyOverrideMode = bodyMode.String
	}

	if err := unmarshalJSONBSlice(extrasRaw, &m.ExtraModels); err != nil {
		return nil, fmt.Errorf("decode extra_models: %w", err)
	}
	if m.ExtraModels == nil {
		m.ExtraModels = []string{}
	}
	if err := unmarshalJSONBStringMap(headersRaw, &m.ExtraHeaders); err != nil {
		return nil, fmt.Errorf("decode extra_headers: %w", err)
	}
	if m.ExtraHeaders == nil {
		m.ExtraHeaders = map[string]string{}
	}
	if bodyRaw.Valid && strings.TrimSpace(bodyRaw.String) != "" {
		var body map[string]any
		if err := json.Unmarshal([]byte(bodyRaw.String), &body); err != nil {
			return nil, fmt.Errorf("decode body_override: %w", err)
		}
		m.BodyOverride = body
	}
	return m, nil
}

// nullableInt converts *int into a value driver-friendly representation.
// Nil becomes nil (NULL), else the int is sent as int64.
func nullableInt(v *int) any {
	if v == nil {
		return nil
	}
	return int64(*v)
}

// assignNullInt unpacks sql.NullInt64 into a *int target.
func assignNullInt(dst **int, n sql.NullInt64) {
	if !n.Valid {
		return
	}
	v := int(n.Int64)
	*dst = &v
}

// channelMonitorPruneBatchSize is the upstream batch size for prune helpers.
// Shared by monitor_history_repo.go and monitor_rollup_repo.go.
const channelMonitorPruneBatchSize = 5000

// deleteChannelMonitorBatched runs the supplied prune SQL repeatedly with a
// fixed batch size until no more rows match. Used by both the history and
// rollup prune entry points.
func deleteChannelMonitorBatched(ctx context.Context, db *sql.DB, query string, cutoff time.Time) (int64, error) {
	var total int64
	for {
		res, err := db.ExecContext(ctx, query, cutoff, channelMonitorPruneBatchSize)
		if err != nil {
			return total, fmt.Errorf("channel_monitor prune batch: %w", err)
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return total, fmt.Errorf("channel_monitor prune rows affected: %w", err)
		}
		total += affected
		if affected == 0 {
			break
		}
	}
	return total, nil
}

// ---------- JSONB helpers ----------

func marshalJSONBSlice(in []string) ([]byte, error) {
	if in == nil {
		in = []string{}
	}
	return json.Marshal(in)
}

func marshalJSONBStringMap(in map[string]string) ([]byte, error) {
	if in == nil {
		in = map[string]string{}
	}
	return json.Marshal(in)
}

func marshalJSONBOptional(in map[string]any) ([]byte, error) {
	if in == nil {
		return nil, nil
	}
	return json.Marshal(in)
}

func unmarshalJSONBSlice(raw []byte, dst *[]string) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, dst)
}

func unmarshalJSONBStringMap(raw []byte, dst *map[string]string) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, dst)
}
