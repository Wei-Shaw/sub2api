package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type accountWindowUsageRepository struct {
	client *dbent.Client
	db     *sql.DB
}

func NewAccountWindowUsageRepository(client *dbent.Client, db *sql.DB) service.AccountWindowUsageRepository {
	return &accountWindowUsageRepository{client, db}
}

type windowTxKey struct{}
type windowSQL interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (r *accountWindowUsageRepository) executor(ctx context.Context) windowSQL {
	if tx, ok := ctx.Value(windowTxKey{}).(*sql.Tx); ok {
		return tx
	}
	return r.db
}

// The account lock serializes observation processing and window reconciliation.
// NOT EXISTS prevents another replica from processing the next observation for
// an account while its earlier observation is locked/in flight. The durable id
// handles arbitrarily many observations with the same timestamp.
func (r *accountWindowUsageRepository) ConsumeNextObservation(ctx context.Context, readyBefore time.Time, apply func(context.Context, *service.AccountQuotaObservation) error) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	var obs service.AccountQuotaObservation
	err = tx.QueryRowContext(ctx, `SELECT o.id,o.account_id,o.observed_at,o.payload
 FROM account_quota_observations o JOIN accounts a ON a.id=o.account_id
 WHERE o.processed_at IS NULL AND o.observed_at <= $1
 AND NOT EXISTS(SELECT 1 FROM account_quota_observations prior WHERE prior.account_id=o.account_id AND prior.processed_at IS NULL AND prior.id<o.id)
 ORDER BY o.id FOR UPDATE OF a,o SKIP LOCKED LIMIT 1`, readyBefore).Scan(&obs.ID, &obs.AccountID, &obs.ObservedAt, &obs.Payload)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claim quota observation: %w", err)
	}
	// Eligibility is rechecked under the same account lock. Discarding an event
	// after deletion/type change must not leave a poison item in the journal.
	var eligible bool
	err = tx.QueryRowContext(ctx, `SELECT platform='openai' AND type='oauth' AND parent_account_id IS NULL AND deleted_at IS NULL
 AND LOWER(TRIM(COALESCE(quota_dimension,''))) IN ('','global')
 AND LOWER(TRIM(COALESCE(credentials->>'auth_mode',''))) NOT IN ('agent_identity','personal_access_token','personalaccesstoken')
 AND LOWER(TRIM(COALESCE(credentials->>'openai_auth_mode',''))) NOT IN ('agent_identity','personal_access_token','personalaccesstoken')
 FROM accounts WHERE id=$1`, obs.AccountID).Scan(&eligible)
	if err != nil {
		return false, err
	}
	tctx := context.WithValue(ctx, windowTxKey{}, tx)
	if eligible {
		if err := apply(tctx, &obs); err != nil {
			return false, err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE account_quota_observations SET processed_at=NOW() WHERE id=$1`, obs.ID); err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (r *accountWindowUsageRepository) readOne(ctx context.Context, query string, args ...any) (*service.AccountWindowUsageRecord, error) {
	var raw []byte
	if err := r.executor(ctx).QueryRowContext(ctx, query, args...).Scan(&raw); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var row service.AccountWindowUsageRecord
	if err := json.Unmarshal(raw, &row); err != nil {
		return nil, err
	}
	return &row, nil
}
func (r *accountWindowUsageRepository) GetOpenWindow(ctx context.Context, accountID int64, kind string) (*service.AccountWindowUsageRecord, error) {
	return r.readOne(ctx, `SELECT row_to_json(w) FROM account_window_usage_histories w WHERE account_id=$1 AND window_type=$2 AND finalized_at IS NULL`, accountID, kind)
}
func (r *accountWindowUsageRepository) GetLatestWindow(ctx context.Context, accountID int64, kind string) (*service.AccountWindowUsageRecord, error) {
	return r.readOne(ctx, `SELECT row_to_json(w) FROM account_window_usage_histories w WHERE account_id=$1 AND window_type=$2 ORDER BY id DESC LIMIT 1`, accountID, kind)
}

func (r *accountWindowUsageRepository) GetClosedWindow(ctx context.Context, accountID int64, kind string, reset, observedAt time.Time) (*service.AccountWindowUsageRecord, error) {
	return r.readOne(ctx, `SELECT row_to_json(w) FROM account_window_usage_histories w
 WHERE account_id=$1 AND window_type=$2 AND finalized_at IS NOT NULL
 AND reset_at BETWEEN $3::timestamptz-INTERVAL '2 seconds' AND $3::timestamptz+INTERVAL '2 seconds'
 AND window_start <= $4::timestamptz+INTERVAL '2 seconds' AND window_end >= $4::timestamptz-INTERVAL '2 seconds'
 ORDER BY window_start DESC,id DESC LIMIT 1`, accountID, kind, reset, observedAt)
}

var accountWindowWriteColumns = []string{"account_id", "window_type", "window_start", "window_end", "reset_at", "duration_minutes", "first_observed_at", "last_sample_at", "last_observation_id", "peak_used_percent", "last_used_percent", "sample_count", "requests", "tokens_total", "api_reference_cost", "priced_requests", "missing_pricing_requests", "estimated_reference_limit", "estimate_reference_cost", "estimate_used_percent", "estimate_observed_at", "quality_flags", "end_reason", "finalized_at", "stats_finalized_at"}

func (r *accountWindowUsageRepository) SaveWindow(ctx context.Context, row *service.AccountWindowUsageRecord) error {
	if _, ok := ctx.Value(windowTxKey{}).(*sql.Tx); !ok {
		return fmt.Errorf("window writes require account transaction lock")
	}
	if row.QualityFlags == nil {
		row.QualityFlags = []string{}
	}
	payload, err := json.Marshal(row)
	if err != nil {
		return err
	}
	columns := strings.Join(accountWindowWriteColumns, ",")
	if row.ID == 0 {
		query := `INSERT INTO account_window_usage_histories (` + columns + `) SELECT ` + columns + ` FROM jsonb_populate_record(NULL::account_window_usage_histories,$1::jsonb) RETURNING id`
		return r.executor(ctx).QueryRowContext(ctx, query, string(payload)).Scan(&row.ID)
	}
	sets := make([]string, 0, len(accountWindowWriteColumns))
	for _, c := range accountWindowWriteColumns {
		sets = append(sets, c+"=v."+c)
	}
	query := `UPDATE account_window_usage_histories w SET ` + strings.Join(sets, ",") + `,updated_at=NOW() FROM jsonb_populate_record(NULL::account_window_usage_histories,$1::jsonb) v WHERE w.id=$2`
	_, err = r.executor(ctx).ExecContext(ctx, query, string(payload), row.ID)
	return err
}
func (r *accountWindowUsageRepository) LatestResetMarker(ctx context.Context, accountID int64, since, until time.Time) (*time.Time, error) {
	var at sql.NullTime
	err := r.executor(ctx).QueryRowContext(ctx, `SELECT MAX(observed_at) FROM account_quota_observations WHERE account_id=$1 AND observed_at>$2 AND observed_at<=$3 AND payload ? 'codex_history_reset_at'`, accountID, since, until).Scan(&at)
	if err != nil {
		return nil, err
	}
	if !at.Valid {
		return nil, nil
	}
	return &at.Time, nil
}

// pricing_at is the recorded price-selection timestamp, not a provider quota
// meter timestamp. Missing snapshots fall back to created_at only for counting
// missing prices. No legacy amount is recomputed with today's pricing table.
// Upper bounds are half-open so adjacent windows do not double count requests.
func (r *accountWindowUsageRepository) AggregateReferenceUsage(ctx context.Context, accountID int64, start, end time.Time) (*service.AccountWindowReferenceStats, error) {
	stats := &service.AccountWindowReferenceStats{}
	if !end.After(start) {
		return stats, nil
	}
	var cost sql.NullFloat64
	err := r.executor(ctx).QueryRowContext(ctx, `WITH window_usage AS (
 SELECT input_tokens,output_tokens,cache_creation_tokens,cache_read_tokens,api_reference_cost,api_reference_pricing,
 CASE WHEN api_reference_pricing->>'pricing_at' IS NOT NULL THEN (api_reference_pricing->>'pricing_at')::timestamptz ELSE created_at END AS reference_at
 FROM usage_logs WHERE account_id=$1 AND created_at >= $2
 ) SELECT COUNT(*),COALESCE(SUM(input_tokens+output_tokens+cache_creation_tokens+cache_read_tokens),0),
 SUM(api_reference_cost) FILTER (WHERE api_reference_cost IS NOT NULL AND api_reference_pricing->>'pricing_at' IS NOT NULL),
 COUNT(*) FILTER (WHERE api_reference_cost IS NOT NULL AND api_reference_pricing->>'pricing_at' IS NOT NULL),
 COUNT(*) FILTER (WHERE api_reference_cost IS NULL OR api_reference_pricing->>'pricing_at' IS NULL)
 FROM window_usage WHERE reference_at >= $2 AND reference_at < $3`, accountID, start, end).Scan(&stats.Requests, &stats.TokensTotal, &cost, &stats.PricedRequests, &stats.MissingPricingRequests)
	if err != nil {
		return nil, fmt.Errorf("aggregate window reference costs: %w", err)
	}
	if cost.Valid {
		stats.ReferenceCost = &cost.Float64
	}
	return stats, nil
}

func (r *accountWindowUsageRepository) ReconcileNextWindow(ctx context.Context, cutoff time.Time, apply func(context.Context, *service.AccountWindowUsageRecord) error) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	var raw []byte
	err = tx.QueryRowContext(ctx, `SELECT row_to_json(w) FROM account_window_usage_histories w JOIN accounts a ON a.id=w.account_id
 WHERE w.stats_finalized_at IS NULL AND w.window_end<$1
 AND NOT EXISTS(SELECT 1 FROM account_quota_observations o WHERE o.account_id=w.account_id AND o.processed_at IS NULL AND o.observed_at<=w.window_end+INTERVAL '5 minutes')
 ORDER BY w.window_end,w.id FOR UPDATE OF a,w SKIP LOCKED LIMIT 1`, cutoff).Scan(&raw)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var row service.AccountWindowUsageRecord
	if err = json.Unmarshal(raw, &row); err != nil {
		return false, err
	}
	if err = apply(context.WithValue(ctx, windowTxKey{}, tx), &row); err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}
func (r *accountWindowUsageRepository) ListHistorySince(ctx context.Context, accountID int64, since time.Time) ([]*service.AccountWindowUsageRecord, error) {
	rows, err := r.executor(ctx).QueryContext(ctx, `SELECT row_to_json(w) FROM account_window_usage_histories w WHERE account_id=$1 AND window_end>=$2 ORDER BY window_type,window_start,id`, accountID, since)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := make([]*service.AccountWindowUsageRecord, 0)
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var row service.AccountWindowUsageRecord
		if err := json.Unmarshal(raw, &row); err != nil {
			return nil, err
		}
		result = append(result, &row)
	}
	return result, rows.Err()
}
func (r *accountWindowUsageRepository) PruneHistory(ctx context.Context, finalizedCutoff, staleCutoff time.Time) error {
	// Bounded deletes avoid holding a single transaction over a large journal.
	if err := r.deleteWindowBatches(ctx, `DELETE FROM account_window_usage_histories WHERE id IN (
  SELECT w.id FROM account_window_usage_histories w
  WHERE (stats_finalized_at IS NOT NULL AND window_end<$1) OR
   (finalized_at IS NULL AND window_end<$2 AND NOT EXISTS(SELECT 1 FROM account_quota_observations o WHERE o.account_id=w.account_id AND processed_at IS NULL))
  ORDER BY w.id LIMIT 1000)`, finalizedCutoff, staleCutoff); err != nil {
		return err
	}
	// Final window rows already retain the estimate and its basis. Ordinary
	// processed observations only need a short operational retention (2 days).
	// Reset markers live for 14 days, or longer while that account has pending
	// observations, so a delayed replay can still locate its true reset cut point.
	processedCutoff := staleCutoff.AddDate(0, 0, 12)
	return r.deleteWindowBatches(ctx, `DELETE FROM account_quota_observations WHERE id IN (
  SELECT o.id FROM account_quota_observations o WHERE o.processed_at IS NOT NULL AND
  ((NOT(o.payload ? 'codex_history_reset_at') AND o.observed_at<$1) OR
   (o.payload ? 'codex_history_reset_at' AND o.observed_at<$2 AND NOT EXISTS(
    SELECT 1 FROM account_quota_observations pending WHERE pending.account_id=o.account_id AND pending.processed_at IS NULL)))
  ORDER BY o.id LIMIT 1000)`, processedCutoff, staleCutoff)
}
func (r *accountWindowUsageRepository) deleteWindowBatches(ctx context.Context, query string, args ...any) error {
	for {
		res, err := r.db.ExecContext(ctx, query, args...)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n < 1000 {
			return nil
		}
	}
}
