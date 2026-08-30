package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

func (r *upstreamPriceMonitorRepository) ReconcileModelCatalog(
	ctx context.Context,
	models []domain.UpstreamPriceDiscoveredModel,
	expectedAccounts int,
	complete bool,
) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var scanRevision int64
	if err := tx.QueryRowContext(ctx, `UPDATE upstream_price_monitor_model_scan_state SET
		revision=revision+1,discovery_complete=$1,last_scan_at=NOW(),
		last_complete_scan_at=CASE WHEN $1 THEN NOW() ELSE last_complete_scan_at END
		WHERE id=1 RETURNING revision`, complete).Scan(&scanRevision); err != nil {
		return 0, err
	}
	observedKeys := make([]string, 0, len(models))
	for _, model := range models {
		name := strings.TrimSpace(model.ModelName)
		if name == "" || len(name) > 255 {
			continue
		}
		key := strings.ToLower(name)
		observedKeys = append(observedKeys, key)
		if _, err := tx.ExecContext(ctx, `INSERT INTO upstream_price_monitor_models
			(model_key,model_name,status,domestic_candidate,seen_account_count,expected_account_count,
			 missing_runs,last_complete_revision,first_seen_at,last_seen_at,updated_at)
			VALUES ($1,$2,'discovered',$3,$4,$5,0,CASE WHEN $6 THEN $7 ELSE 0 END,NOW(),NOW(),NOW())
			ON CONFLICT (model_key) DO UPDATE SET model_name=EXCLUDED.model_name,
			 domestic_candidate=upstream_price_monitor_models.domestic_candidate OR EXCLUDED.domestic_candidate,
			 seen_account_count=EXCLUDED.seen_account_count,expected_account_count=EXCLUDED.expected_account_count,
			 missing_runs=0,last_seen_at=NOW(),updated_at=NOW(),
			 last_complete_revision=CASE WHEN $6 THEN $7 ELSE upstream_price_monitor_models.last_complete_revision END,
			 status=CASE WHEN upstream_price_monitor_models.status='suspected_retired' THEN 'discovered'
			             ELSE upstream_price_monitor_models.status END`,
			key, name, model.DomesticCandidate, model.SeenAccountCount, expectedAccounts, complete, scanRevision); err != nil {
			return 0, err
		}
	}
	if complete && expectedAccounts > 0 {
		if _, err := tx.ExecContext(ctx, `UPDATE upstream_price_monitor_models SET
			missing_runs=CASE WHEN status='managed' THEN missing_runs+1 ELSE missing_runs END,
			seen_account_count=0,expected_account_count=$2,
			last_missing_at=NOW(),updated_at=NOW()
			WHERE NOT (model_key=ANY($1::text[]))`, pq.Array(observedKeys), expectedAccounts); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE upstream_price_monitor_models SET
			status='suspected_retired',updated_at=NOW()
			WHERE status='managed' AND missing_runs>=3
			  AND (last_seen_at IS NULL OR last_seen_at < NOW() - INTERVAL '24 hours')`); err != nil {
			return 0, err
		}
	}
	if err := syncUpstreamPriceManagedModels(ctx, tx); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return scanRevision, nil
}

func (r *upstreamPriceMonitorRepository) ListModelCatalog(ctx context.Context) ([]domain.UpstreamPriceModelCatalogEntry, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT m.model_name,m.status,m.domestic_candidate,m.seen_account_count,
		m.expected_account_count,m.missing_runs,
		(state.discovery_complete AND m.last_complete_revision=state.revision),
		m.first_seen_at,m.last_seen_at,m.last_missing_at,m.updated_at
		FROM upstream_price_monitor_models m CROSS JOIN upstream_price_monitor_model_scan_state state
		WHERE state.id=1
		ORDER BY CASE status WHEN 'managed' THEN 0 WHEN 'discovered' THEN 1
		 WHEN 'suspected_retired' THEN 2 WHEN 'ignored' THEN 3 ELSE 4 END,
		 domestic_candidate DESC,LOWER(model_name)`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []domain.UpstreamPriceModelCatalogEntry
	for rows.Next() {
		var item domain.UpstreamPriceModelCatalogEntry
		var firstSeen, lastSeen, lastMissing sql.NullTime
		if err := rows.Scan(&item.ModelName, &item.Status, &item.DomesticCandidate,
			&item.SeenAccountCount, &item.ExpectedAccountCount, &item.MissingRuns, &item.DiscoveryComplete,
			&firstSeen, &lastSeen, &lastMissing, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.FirstSeenAt = nullTimePtr(firstSeen)
		item.LastSeenAt = nullTimePtr(lastSeen)
		item.LastMissingAt = nullTimePtr(lastMissing)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *upstreamPriceMonitorRepository) GetModelCatalogRevision(ctx context.Context) (int64, bool, error) {
	var revision int64
	var complete bool
	err := r.db.QueryRowContext(ctx, `SELECT revision,discovery_complete
		FROM upstream_price_monitor_model_scan_state WHERE id=1`).Scan(&revision, &complete)
	return revision, complete, err
}

func (r *upstreamPriceMonitorRepository) SetModelCatalogStatus(
	ctx context.Context,
	model string,
	status domain.UpstreamPriceModelStatus,
) (*domain.UpstreamPriceModelCatalogEntry, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, service.ErrUpstreamPriceMonitorInvalidConfig
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if status == domain.UpstreamPriceModelStatusManaged {
		var seen, expected int
		var currentComplete bool
		if err := tx.QueryRowContext(ctx, `SELECT m.seen_account_count,m.expected_account_count,
			(state.discovery_complete AND m.last_complete_revision=state.revision)
			FROM upstream_price_monitor_models m CROSS JOIN upstream_price_monitor_model_scan_state state
			WHERE m.model_key=LOWER($1) AND state.id=1 FOR UPDATE OF m,state`, model).Scan(&seen, &expected, &currentComplete); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("%w: model is not in the discovered catalogue", service.ErrUpstreamPriceMonitorInvalidConfig)
			}
			return nil, err
		}
		if !currentComplete || expected <= 0 || seen != expected {
			return nil, fmt.Errorf("%w: model is not visible on every selected production account", service.ErrUpstreamPriceMonitorInvalidConfig)
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE upstream_price_monitor_models SET status=$2,
		missing_runs=CASE WHEN $2='managed' THEN 0 ELSE missing_runs END,updated_at=NOW()
		WHERE model_key=LOWER($1)`, model, status)
	if err != nil {
		return nil, err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return nil, fmt.Errorf("%w: model is not in the discovered catalogue", service.ErrUpstreamPriceMonitorInvalidConfig)
	}
	if err := syncUpstreamPriceManagedModels(ctx, tx); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	items, err := r.ListModelCatalog(ctx)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if strings.EqualFold(items[i].ModelName, model) {
			return &items[i], nil
		}
	}
	return nil, errors.New("updated model catalogue row could not be reloaded")
}

func syncUpstreamPriceManagedModels(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `WITH managed AS (
		SELECT COALESCE(ARRAY_AGG(model_name ORDER BY LOWER(model_name)),'{}'::text[]) AS models
		FROM upstream_price_monitor_models WHERE status='managed'
	)
	UPDATE upstream_price_monitor_config SET domestic_models=managed.models,updated_at=NOW()
	FROM managed WHERE id=1 AND domestic_models IS DISTINCT FROM managed.models`)
	return err
}

func nullTimePtr(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	out := value.Time
	return &out
}
