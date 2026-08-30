package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type upstreamPriceMonitorRepository struct{ db *sql.DB }

func NewUpstreamPriceMonitorRepository(db *sql.DB) service.UpstreamPriceMonitorRepository {
	return &upstreamPriceMonitorRepository{db: db}
}

func (r *upstreamPriceMonitorRepository) GetConfig(ctx context.Context) (*domain.UpstreamPriceMonitorConfig, error) {
	if r == nil || r.db == nil {
		cfg := domain.DefaultUpstreamPriceMonitorConfig()
		return &cfg, nil
	}
	var cfg domain.UpstreamPriceMonitorConfig
	err := r.db.QueryRowContext(ctx, `SELECT enabled, mode, interval_minutes, markup,
		display_multiplier_decimals, account_ids, channel_ids, domestic_models,
		passive_sample_max_age_minutes, active_probe_enabled, updated_at
		FROM upstream_price_monitor_config WHERE id=1`).Scan(
		&cfg.Enabled, &cfg.Mode, &cfg.IntervalMinutes, &cfg.Markup,
		&cfg.DisplayMultiplierDecimals, pq.Array(&cfg.AccountIDs), pq.Array(&cfg.ChannelIDs), pq.Array(&cfg.DomesticModels),
		&cfg.PassiveSampleMaxAgeMinutes, &cfg.ActiveProbeEnabled, &cfg.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		defaults := domain.DefaultUpstreamPriceMonitorConfig()
		return &defaults, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get upstream price monitor config: %w", err)
	}
	return &cfg, nil
}

func (r *upstreamPriceMonitorRepository) UpdateConfig(ctx context.Context, cfg *domain.UpstreamPriceMonitorConfig) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	modelRows, err := tx.QueryContext(ctx, `SELECT model_name FROM upstream_price_monitor_models
		WHERE status='managed' ORDER BY LOWER(model_name)`)
	if err != nil {
		return err
	}
	cfg.DomesticModels = cfg.DomesticModels[:0]
	for modelRows.Next() {
		var model string
		if err := modelRows.Scan(&model); err != nil {
			_ = modelRows.Close()
			return err
		}
		cfg.DomesticModels = append(cfg.DomesticModels, model)
	}
	if err := modelRows.Err(); err != nil {
		_ = modelRows.Close()
		return err
	}
	if err := modelRows.Close(); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, `INSERT INTO upstream_price_monitor_config
		(id,enabled,mode,interval_minutes,markup,display_multiplier_decimals,account_ids,channel_ids,domestic_models,
		 passive_sample_max_age_minutes,active_probe_enabled,updated_at)
		VALUES (1,$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NOW())
		ON CONFLICT (id) DO UPDATE SET enabled=EXCLUDED.enabled,mode=EXCLUDED.mode,
		interval_minutes=EXCLUDED.interval_minutes,markup=EXCLUDED.markup,
		display_multiplier_decimals=EXCLUDED.display_multiplier_decimals,account_ids=EXCLUDED.account_ids,
		channel_ids=EXCLUDED.channel_ids,domestic_models=EXCLUDED.domestic_models,
		passive_sample_max_age_minutes=EXCLUDED.passive_sample_max_age_minutes,
		active_probe_enabled=EXCLUDED.active_probe_enabled,updated_at=NOW()
		RETURNING updated_at`, cfg.Enabled, cfg.Mode, cfg.IntervalMinutes, cfg.Markup,
		cfg.DisplayMultiplierDecimals, pq.Array(cfg.AccountIDs), pq.Array(cfg.ChannelIDs), pq.Array(cfg.DomesticModels),
		cfg.PassiveSampleMaxAgeMinutes, cfg.ActiveProbeEnabled).Scan(&cfg.UpdatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *upstreamPriceMonitorRepository) CreateRun(ctx context.Context, run *domain.UpstreamPriceMonitorRun) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// A process may die after inserting a running row. Treat a 30-minute row as
	// an expired lease before relying on the partial unique index.
	if _, err := tx.ExecContext(ctx, `UPDATE upstream_price_monitor_runs
		SET status='failed',finished_at=NOW(),error=CASE WHEN error='' THEN 'stale running lease recovered' ELSE error END
		WHERE status='running' AND started_at < NOW() - INTERVAL '30 minutes'`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM upstream_price_monitor_runs
		WHERE status<>'running' AND rollback_available=FALSE AND started_at < NOW() - INTERVAL '90 days'`); err != nil {
		return err
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO upstream_price_monitor_runs
		(trigger,status,mode,dry_run,started_at) VALUES ($1,'running',$2,$3,$4) RETURNING id`,
		run.Trigger, run.Mode, run.DryRun, run.StartedAt).Scan(&run.ID)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return service.ErrUpstreamPriceMonitorRunConflict
		}
		return err
	}
	return tx.Commit()
}

func (r *upstreamPriceMonitorRepository) FinishRun(ctx context.Context, run *domain.UpstreamPriceMonitorRun) error {
	summary, err := json.Marshal(run.Summary)
	if err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, `UPDATE upstream_price_monitor_runs SET
		status=$2,finished_at=$3,matched_models=$4,mismatched_models=$5,probed_models=$6,probe_cost=$7,
		snapshot_hash=$8,summary=$9,error=$10,applied_at=$11,rollback_available=$12
		WHERE id=$1 AND status='running'`, run.ID, run.Status, run.FinishedAt, run.MatchedModels,
		run.MismatchedModels, run.ProbedModels, run.ProbeCost, run.SnapshotHash, summary, run.Error,
		run.AppliedAt, run.RollbackAvailable)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		return service.ErrUpstreamPriceMonitorRunConflict
	}
	return nil
}

func (r *upstreamPriceMonitorRepository) MarkApplyFailure(ctx context.Context, runID int64, message string) error {
	if len(message) > 4000 {
		message = message[:4000]
	}
	_, err := r.db.ExecContext(ctx, `UPDATE upstream_price_monitor_runs SET status='partial',error=$2
		WHERE id=$1 AND status='completed' AND applied_at IS NULL`, runID, message)
	return err
}

const upstreamPriceRunSelect = `SELECT id,trigger,status,mode,dry_run,started_at,finished_at,
	matched_models,mismatched_models,probed_models,probe_cost,snapshot_hash,summary,error,applied_at,rollback_available
	FROM upstream_price_monitor_runs`

func (r *upstreamPriceMonitorRepository) GetRun(ctx context.Context, id int64) (*domain.UpstreamPriceMonitorRun, error) {
	run, err := scanUpstreamPriceRun(r.db.QueryRowContext(ctx, upstreamPriceRunSelect+` WHERE id=$1`, id))
	if err != nil {
		return nil, fmt.Errorf("get upstream price monitor run: %w", err)
	}
	return run, nil
}

func (r *upstreamPriceMonitorRepository) ListRuns(ctx context.Context, limit, offset int, statuses ...domain.UpstreamPriceMonitorRunStatus) (*domain.UpstreamPriceMonitorRunPage, error) {
	page := &domain.UpstreamPriceMonitorRunPage{}
	var status domain.UpstreamPriceMonitorRunStatus
	if len(statuses) > 0 {
		status = statuses[0]
	}
	countQuery := `SELECT COUNT(*) FROM upstream_price_monitor_runs`
	var countArgs []any
	if status != "" {
		countQuery += ` WHERE status=$1`
		countArgs = append(countArgs, status)
	}
	if err := r.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&page.Total); err != nil {
		return nil, err
	}
	listQuery := upstreamPriceRunSelect
	listArgs := []any{limit, offset}
	if status == "" {
		listQuery += ` ORDER BY started_at DESC,id DESC LIMIT $1 OFFSET $2`
	} else {
		listQuery += ` WHERE status=$1 ORDER BY started_at DESC,id DESC LIMIT $2 OFFSET $3`
		listArgs = []any{status, limit, offset}
	}
	rows, err := r.db.QueryContext(ctx, listQuery, listArgs...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		run, err := scanUpstreamPriceRun(rows)
		if err != nil {
			return nil, err
		}
		page.Items = append(page.Items, *run)
	}
	return page, rows.Err()
}

type upstreamPriceRowScanner interface{ Scan(...any) error }

func scanUpstreamPriceRun(row upstreamPriceRowScanner) (*domain.UpstreamPriceMonitorRun, error) {
	var run domain.UpstreamPriceMonitorRun
	var finished, applied sql.NullTime
	var summary []byte
	if err := row.Scan(&run.ID, &run.Trigger, &run.Status, &run.Mode, &run.DryRun, &run.StartedAt, &finished,
		&run.MatchedModels, &run.MismatchedModels, &run.ProbedModels, &run.ProbeCost, &run.SnapshotHash,
		&summary, &run.Error, &applied, &run.RollbackAvailable); err != nil {
		return nil, err
	}
	if finished.Valid {
		run.FinishedAt = &finished.Time
	}
	if applied.Valid {
		run.AppliedAt = &applied.Time
	}
	if len(summary) > 0 {
		if err := json.Unmarshal(summary, &run.Summary); err != nil {
			return nil, err
		}
	}
	return &run, nil
}

func (r *upstreamPriceMonitorRepository) GetRuntime(ctx context.Context) (*domain.UpstreamPriceMonitorRuntime, error) {
	cfg, err := r.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	runtime := &domain.UpstreamPriceMonitorRuntime{Status: "idle"}
	if !cfg.Enabled {
		runtime.Status = "disabled"
	}
	var lastStatus, lastError string
	var lastAt sql.NullTime
	err = r.db.QueryRowContext(ctx, `SELECT status,started_at,error FROM upstream_price_monitor_runs ORDER BY started_at DESC,id DESC LIMIT 1`).
		Scan(&lastStatus, &lastAt, &lastError)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if lastAt.Valid {
		runtime.LastRunAt = &lastAt.Time
		switch lastStatus {
		case string(domain.UpstreamPriceMonitorRunStatusRunning):
			runtime.Status = "running"
		case string(domain.UpstreamPriceMonitorRunStatusFailed):
			runtime.Status = "failed"
		case string(domain.UpstreamPriceMonitorRunStatusPartial):
			runtime.Status = "degraded"
		default:
			if cfg.Enabled {
				runtime.Status = "idle"
			}
		}
		runtime.LastError = lastError
		if cfg.Enabled && lastStatus != string(domain.UpstreamPriceMonitorRunStatusRunning) {
			next := lastAt.Time.Add(time.Duration(cfg.IntervalMinutes) * time.Minute)
			runtime.NextRunAt = &next
		}
	}
	rows, err := r.db.QueryContext(ctx, `SELECT status FROM upstream_price_monitor_runs ORDER BY started_at DESC,id DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var status string
		if err := rows.Scan(&status); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if status != string(domain.UpstreamPriceMonitorRunStatusFailed) && status != string(domain.UpstreamPriceMonitorRunStatusPartial) {
			break
		}
		runtime.ConsecutiveFailures++
	}
	_ = rows.Close()
	if err := r.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(COALESCE((remote_delta->>'actual_cost')::numeric,0)),0)
		FROM upstream_price_monitor_evidence WHERE source='active_probe' AND created_at >= CURRENT_DATE`).Scan(&runtime.TodayProbeCost); err != nil {
		return nil, err
	}
	domesticModelKeys := make([]string, 0, len(cfg.DomesticModels))
	for _, model := range cfg.DomesticModels {
		domesticModelKeys = append(domesticModelKeys, strings.ToLower(model))
	}
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT LOWER(model_name)) FROM upstream_price_monitor_evidence
		WHERE status='trusted' AND billing_mode='token' AND account_id=ANY($2)
		  AND LOWER(model_name)=ANY($3) AND observed_at >= NOW() - ($1 * INTERVAL '1 minute')`,
		cfg.PassiveSampleMaxAgeMinutes, pq.Array(cfg.AccountIDs), pq.Array(domesticModelKeys)).
		Scan(&runtime.Coverage.Trusted); err != nil {
		return nil, err
	}
	runtime.Coverage.Total = len(cfg.DomesticModels)
	return runtime, nil
}

func (r *upstreamPriceMonitorRepository) GetCheckpoints(ctx context.Context, accountID int64, models []string) (map[string]domain.UpstreamPriceUsageCheckpoint, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT account_id,model_name,account_identity_hash,remote_snapshot,ledger_date,
		billing_context_hash,local_usage_log_id,captured_at,active_probe_pending,active_probe_started_at,revision
		FROM upstream_price_monitor_usage_checkpoints WHERE account_id=$1 AND model_name=ANY($2)`, accountID, pq.Array(models))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make(map[string]domain.UpstreamPriceUsageCheckpoint, len(models))
	for rows.Next() {
		var cp domain.UpstreamPriceUsageCheckpoint
		var raw []byte
		var ledgerDate time.Time
		var probeStarted sql.NullTime
		if err := rows.Scan(&cp.AccountID, &cp.ModelName, &cp.AccountIdentityHash, &raw, &ledgerDate,
			&cp.BillingContextHash, &cp.LocalUsageLogID, &cp.CapturedAt, &cp.ActiveProbePending, &probeStarted, &cp.Revision); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &cp.Remote); err != nil {
			return nil, err
		}
		cp.LedgerDate = ledgerDate.Format("2006-01-02")
		if probeStarted.Valid {
			cp.ActiveProbeStartedAt = &probeStarted.Time
		}
		out[strings.ToLower(cp.ModelName)] = cp
	}
	return out, rows.Err()
}

func (r *upstreamPriceMonitorRepository) CurrentLocalUsageLogID(ctx context.Context, accountIDs []int64) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(id),0) FROM usage_logs WHERE account_id=ANY($1)`, pq.Array(accountIDs)).Scan(&id)
	return id, err
}

func (r *upstreamPriceMonitorRepository) AggregateLocalUsage(
	ctx context.Context,
	accountIDs []int64,
	afterByModel map[string]int64,
	throughID int64,
) (map[string]domain.UpstreamPriceLocalAggregate, error) {
	models := make([]string, 0, len(afterByModel))
	afterIDs := make([]int64, 0, len(afterByModel))
	for model, id := range afterByModel {
		models = append(models, model)
		afterIDs = append(afterIDs, id)
	}
	rows, err := r.db.QueryContext(ctx, `WITH bounds AS (
		SELECT model_name,LOWER(model_name) AS model_key,after_id
		FROM UNNEST($2::text[],$3::bigint[]) AS b(model_name,after_id)
	)
	SELECT b.model_name,
		COUNT(ul.id),COALESCE(SUM(ul.input_tokens),0),COALESCE(SUM(ul.output_tokens),0),
		COALESCE(SUM(ul.cache_creation_tokens),0),COALESCE(SUM(ul.cache_read_tokens),0),
		MIN(ul.created_at),MAX(ul.created_at),
		COUNT(DISTINCT COALESCE(NULLIF(BTRIM(ul.service_tier),''),'default')) FILTER (WHERE ul.id IS NOT NULL),
		COALESCE(BOOL_OR(ul.long_context_billing_applied OR
			COALESCE(NULLIF(BTRIM(ul.service_tier),''),'default') NOT IN ('default','standard','auto')) FILTER (WHERE ul.id IS NOT NULL),FALSE)
	FROM bounds b LEFT JOIN usage_logs ul ON ul.account_id=ANY($1) AND ul.id>b.after_id AND ul.id<=$4
		AND LOWER(COALESCE(NULLIF(BTRIM(ul.upstream_model),''),ul.model))=b.model_key
		AND ul.actual_cost>0 AND COALESCE(NULLIF(BTRIM(ul.billing_mode),''),'token')='token'
	GROUP BY b.model_name,b.model_key ORDER BY b.model_key`, pq.Array(accountIDs), pq.Array(models), pq.Array(afterIDs), throughID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make(map[string]domain.UpstreamPriceLocalAggregate, len(models))
	for rows.Next() {
		var aggregate domain.UpstreamPriceLocalAggregate
		var first, last sql.NullTime
		if err := rows.Scan(&aggregate.ModelName, &aggregate.Counters.Requests, &aggregate.Counters.InputTokens,
			&aggregate.Counters.OutputTokens, &aggregate.Counters.CacheCreationTokens, &aggregate.Counters.CacheReadTokens,
			&first, &last, &aggregate.DistinctServiceTiers, &aggregate.HasSpecialContext); err != nil {
			return nil, err
		}
		if first.Valid {
			aggregate.FirstUsageAt = &first.Time
		}
		if last.Valid {
			aggregate.LastUsageAt = &last.Time
		}
		out[strings.ToLower(aggregate.ModelName)] = aggregate
	}
	return out, rows.Err()
}

func (r *upstreamPriceMonitorRepository) ListMatchedObservations(ctx context.Context, accountID int64, model, contextKey string, since time.Time, limit int) ([]domain.UpstreamPriceObservation, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT
		COALESCE((remote_delta->>'input_tokens')::bigint,0),COALESCE((remote_delta->>'output_tokens')::bigint,0),
		COALESCE((remote_delta->>'cache_creation_tokens')::bigint,0),COALESCE((remote_delta->>'cache_read_tokens')::bigint,0),
		COALESCE((remote_delta->>'actual_cost')::numeric,0)
		FROM upstream_price_monitor_evidence
		WHERE account_id=$1 AND LOWER(model_name)=LOWER($2) AND context_key=$3 AND reconciliation_status='matched'
		  AND observed_at >= $4
		ORDER BY observed_at DESC,id DESC LIMIT $5`, accountID, model, contextKey, since, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []domain.UpstreamPriceObservation
	for rows.Next() {
		var row domain.UpstreamPriceObservation
		if err := rows.Scan(&row.InputTokens, &row.OutputTokens, &row.CacheCreationTokens, &row.CacheReadTokens, &row.ActualCost); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *upstreamPriceMonitorRepository) SaveReconciliation(
	ctx context.Context,
	checkpoint *domain.UpstreamPriceUsageCheckpoint,
	expectedRevision *int64,
	evidence *domain.UpstreamPriceEvidence,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if checkpoint != nil {
		remoteJSON, err := json.Marshal(checkpoint.Remote)
		if err != nil {
			return err
		}
		if expectedRevision == nil {
			result, err := tx.ExecContext(ctx, `INSERT INTO upstream_price_monitor_usage_checkpoints
				(account_id,model_name,account_identity_hash,remote_snapshot,ledger_date,billing_context_hash,
				 local_usage_log_id,captured_at,active_probe_pending,active_probe_started_at,revision,updated_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,1,NOW()) ON CONFLICT (account_id,model_name) DO NOTHING`,
				checkpoint.AccountID, checkpoint.ModelName, checkpoint.AccountIdentityHash, remoteJSON, checkpoint.LedgerDate,
				checkpoint.BillingContextHash, checkpoint.LocalUsageLogID, checkpoint.CapturedAt,
				checkpoint.ActiveProbePending, checkpoint.ActiveProbeStartedAt)
			if err != nil {
				return err
			}
			rows, _ := result.RowsAffected()
			if rows != 1 {
				return service.ErrUpstreamPriceCheckpointConflict
			}
			checkpoint.Revision = 1
		} else {
			result, err := tx.ExecContext(ctx, `UPDATE upstream_price_monitor_usage_checkpoints SET
				account_identity_hash=$3,remote_snapshot=$4,ledger_date=$5,billing_context_hash=$6,
				local_usage_log_id=$7,captured_at=$8,active_probe_pending=$9,active_probe_started_at=$10,
				revision=revision+1,updated_at=NOW()
				WHERE account_id=$1 AND model_name=$2 AND revision=$11`, checkpoint.AccountID, checkpoint.ModelName,
				checkpoint.AccountIdentityHash, remoteJSON, checkpoint.LedgerDate, checkpoint.BillingContextHash,
				checkpoint.LocalUsageLogID, checkpoint.CapturedAt, checkpoint.ActiveProbePending,
				checkpoint.ActiveProbeStartedAt, *expectedRevision)
			if err != nil {
				return err
			}
			rows, _ := result.RowsAffected()
			if rows != 1 {
				return service.ErrUpstreamPriceCheckpointConflict
			}
			checkpoint.Revision = *expectedRevision + 1
		}
	}
	if evidence != nil {
		if err := insertUpstreamPriceEvidence(ctx, tx, evidence); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func insertUpstreamPriceEvidence(ctx context.Context, tx *sql.Tx, evidence *domain.UpstreamPriceEvidence) error {
	localJSON, err := json.Marshal(evidence.LocalDelta)
	if err != nil {
		return err
	}
	remoteJSON, _ := json.Marshal(evidence.RemoteDelta)
	pricesJSON, _ := json.Marshal(evidence.Prices)
	currentJSON, _ := json.Marshal(evidence.CurrentPrices)
	suggestedJSON, _ := json.Marshal(evidence.SuggestedPrices)
	return tx.QueryRowContext(ctx, `INSERT INTO upstream_price_monitor_evidence
		(run_id,account_id,model_name,billing_mode,status,source,reconciliation_status,context_key,observed_at,
		 sample_count,local_delta,remote_delta,prices,current_prices,suggested_prices,
		 display_multiplier_current,display_multiplier_suggested,last_error)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
		RETURNING id,created_at`, evidence.RunID, evidence.AccountID, evidence.ModelName, evidence.BillingMode,
		evidence.Status, evidence.Source, evidence.ReconciliationStatus, evidence.ContextKey, evidence.ObservedAt,
		evidence.SampleCount, localJSON, remoteJSON, pricesJSON, currentJSON, suggestedJSON,
		evidence.DisplayMultiplierCurrent, evidence.DisplayMultiplierSuggested, evidence.LastError).
		Scan(&evidence.ID, &evidence.CreatedAt)
}

func (r *upstreamPriceMonitorRepository) ListEvidenceByRun(ctx context.Context, runID int64) ([]domain.UpstreamPriceEvidence, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,run_id,account_id,model_name,billing_mode,status,source,
		reconciliation_status,context_key,observed_at,sample_count,local_delta,remote_delta,prices,current_prices,
		suggested_prices,display_multiplier_current,display_multiplier_suggested,last_error,created_at
		FROM upstream_price_monitor_evidence WHERE run_id=$1 ORDER BY account_id,LOWER(model_name),context_key,id`, runID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []domain.UpstreamPriceEvidence
	for rows.Next() {
		var evidence domain.UpstreamPriceEvidence
		var localJSON, remoteJSON, pricesJSON, currentJSON, suggestedJSON []byte
		var currentMultiplier, suggestedMultiplier sql.NullFloat64
		if err := rows.Scan(&evidence.ID, &evidence.RunID, &evidence.AccountID, &evidence.ModelName,
			&evidence.BillingMode, &evidence.Status, &evidence.Source, &evidence.ReconciliationStatus,
			&evidence.ContextKey, &evidence.ObservedAt, &evidence.SampleCount, &localJSON, &remoteJSON,
			&pricesJSON, &currentJSON, &suggestedJSON, &currentMultiplier, &suggestedMultiplier,
			&evidence.LastError, &evidence.CreatedAt); err != nil {
			return nil, err
		}
		for _, target := range []struct {
			raw   []byte
			value any
		}{{localJSON, &evidence.LocalDelta}, {remoteJSON, &evidence.RemoteDelta}, {pricesJSON, &evidence.Prices},
			{currentJSON, &evidence.CurrentPrices}, {suggestedJSON, &evidence.SuggestedPrices}} {
			if len(target.raw) > 0 {
				if err := json.Unmarshal(target.raw, target.value); err != nil {
					return nil, err
				}
			}
		}
		if currentMultiplier.Valid {
			evidence.DisplayMultiplierCurrent = &currentMultiplier.Float64
		}
		if suggestedMultiplier.Valid {
			evidence.DisplayMultiplierSuggested = &suggestedMultiplier.Float64
		}
		out = append(out, evidence)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	run, err := r.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	scope, scopeOK := service.ReadUpstreamPriceRunApplyScope(run)
	if scopeOK {
		if err := r.enrichUpstreamPriceEvidenceBatch(ctx, out, scope.ChannelIDs, scope.DisplayMultiplierDecimals); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (r *upstreamPriceMonitorRepository) enrichUpstreamPriceEvidenceBatch(
	ctx context.Context,
	evidence []domain.UpstreamPriceEvidence,
	channelIDs []int64,
	decimals int,
) error {
	if len(evidence) == 0 || len(channelIDs) == 0 {
		return nil
	}
	modelSet := make(map[string]struct{}, len(evidence))
	for i := range evidence {
		modelSet[strings.ToLower(evidence[i].ModelName)] = struct{}{}
	}
	models := make([]string, 0, len(modelSet))
	for model := range modelSet {
		models = append(models, model)
	}

	rows, err := r.db.QueryContext(ctx, `SELECT LOWER(cmp.models->>0),cmp.billing_mode,
		(COALESCE(base_iv.input_price,cmp.input_price*COALESCE(base_iv.input_multiplier,1))*1000000)::float8,
		(COALESCE(base_iv.output_price,cmp.output_price*COALESCE(base_iv.output_multiplier,1))*1000000)::float8,
		(COALESCE(base_iv.cache_write_price,cmp.cache_write_price*COALESCE(base_iv.cache_write_multiplier,1))*1000000)::float8,
		(COALESCE(base_iv.cache_read_price,cmp.cache_read_price*COALESCE(base_iv.cache_read_multiplier,1))*1000000)::float8,
		COALESCE(request_low.per_request_price,cmp.per_request_price)::float8,
		request_middle.per_request_price::float8,request_high.per_request_price::float8
		FROM channel_model_pricing cmp
		LEFT JOIN LATERAL (SELECT input_price,output_price,cache_write_price,cache_read_price,
			input_multiplier,output_multiplier,cache_write_multiplier,cache_read_multiplier
			FROM channel_pricing_intervals WHERE pricing_id=cmp.id AND min_tokens=0 ORDER BY sort_order,id LIMIT 1) base_iv ON TRUE
		LEFT JOIN LATERAL (SELECT per_request_price FROM channel_pricing_intervals
			WHERE pricing_id=cmp.id AND min_tokens=0 AND max_tokens=256000 ORDER BY sort_order,id LIMIT 1) request_low ON TRUE
		LEFT JOIN LATERAL (SELECT per_request_price FROM channel_pricing_intervals
			WHERE pricing_id=cmp.id AND min_tokens=256000 AND max_tokens=512000 ORDER BY sort_order,id LIMIT 1) request_middle ON TRUE
		LEFT JOIN LATERAL (SELECT per_request_price FROM channel_pricing_intervals
			WHERE pricing_id=cmp.id AND min_tokens=512000 AND max_tokens IS NULL ORDER BY sort_order,id LIMIT 1) request_high ON TRUE
		WHERE cmp.channel_id=ANY($1) AND jsonb_array_length(cmp.models)=1 AND LOWER(cmp.models->>0)=ANY($2)
		ORDER BY array_position($1::bigint[],cmp.channel_id),cmp.id`, pq.Array(channelIDs), pq.Array(models))
	if err != nil {
		return err
	}
	current := make(map[string]domain.UpstreamPriceVector)
	mixed := make(map[string]bool)
	for rows.Next() {
		var model, billingMode string
		var input, output, cacheWrite, cacheRead, low, middle, high sql.NullFloat64
		if err := rows.Scan(&model, &billingMode, &input, &output, &cacheWrite, &cacheRead, &low, &middle, &high); err != nil {
			_ = rows.Close()
			return err
		}
		value := domain.UpstreamPriceVector{
			InputPerMillion: nullFloat64Ptr(input), OutputPerMillion: nullFloat64Ptr(output),
			CacheWritePerMillion: nullFloat64Ptr(cacheWrite), CacheReadPerMillion: nullFloat64Ptr(cacheRead),
			PerRequestLTE256K: nullFloat64Ptr(low), PerRequest256K512K: nullFloat64Ptr(middle), PerRequestGT512K: nullFloat64Ptr(high),
		}
		key := model + "\x00" + billingMode
		if previous, exists := current[key]; exists && !sameUpstreamPriceVector(previous, value) {
			mixed[key] = true
		} else if !exists {
			current[key] = value
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	type displayInfo struct {
		current *float64
		input   sql.NullString
		output  sql.NullString
	}
	displays := make(map[string]displayInfo)
	displayRows, err := r.db.QueryContext(ctx, `SELECT LOWER(d.model_name),
		COALESCE(d.model_multiplier,s.global_multiplier*COALESCE(p.multiplier,1))::float8,
		d.official_input_per_million::text,d.official_output_per_million::text
		FROM display_model_prices d JOIN display_pricing_providers p ON p.provider=d.provider
		CROSS JOIN display_pricing_settings s
		WHERE d.billing_mode='token' AND LOWER(d.model_name)=ANY($1) ORDER BY d.id`, pq.Array(models))
	if err != nil {
		return err
	}
	for displayRows.Next() {
		var model string
		var multiplier sql.NullFloat64
		var info displayInfo
		if err := displayRows.Scan(&model, &multiplier, &info.input, &info.output); err != nil {
			_ = displayRows.Close()
			return err
		}
		info.current = nullFloat64Ptr(multiplier)
		displays[model] = info
	}
	if err := displayRows.Err(); err != nil {
		_ = displayRows.Close()
		return err
	}
	if err := displayRows.Close(); err != nil {
		return err
	}

	for i := range evidence {
		key := strings.ToLower(evidence[i].ModelName) + "\x00" + evidence[i].BillingMode
		if mixed[key] {
			if evidence[i].LastError != "" {
				evidence[i].LastError += "; "
			}
			evidence[i].LastError += "configured target channels currently disagree on this model price"
		} else if value, ok := current[key]; ok {
			evidence[i].CurrentPrices = value
		}
		if evidence[i].BillingMode != service.DisplayBillingModeToken {
			continue
		}
		info, ok := displays[strings.ToLower(evidence[i].ModelName)]
		if !ok {
			continue
		}
		evidence[i].DisplayMultiplierCurrent = info.current
		multiplier, representable, err := calculateDisplayMultiplier(upstreamPriceApplyEvidence{
			Model: evidence[i].ModelName, Suggested: evidence[i].SuggestedPrices, BillingMode: evidence[i].BillingMode,
		}, info.input, info.output, decimals)
		if err != nil {
			return err
		}
		if representable {
			value := multiplier.InexactFloat64()
			evidence[i].DisplayMultiplierSuggested = &value
		}
	}
	return nil
}
