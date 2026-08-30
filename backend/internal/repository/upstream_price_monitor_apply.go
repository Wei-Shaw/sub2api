package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/shopspring/decimal"
)

type upstreamPriceChannelRollback struct {
	ID                   int64   `json:"id"`
	InputPrice           *string `json:"input_price,omitempty"`
	OutputPrice          *string `json:"output_price,omitempty"`
	CacheWritePrice      *string `json:"cache_write_price,omitempty"`
	CacheReadPrice       *string `json:"cache_read_price,omitempty"`
	PerRequestPrice      *string `json:"per_request_price,omitempty"`
	AfterInputPrice      *string `json:"after_input_price,omitempty"`
	AfterOutputPrice     *string `json:"after_output_price,omitempty"`
	AfterCacheWritePrice *string `json:"after_cache_write_price,omitempty"`
	AfterCacheReadPrice  *string `json:"after_cache_read_price,omitempty"`
	AfterPerRequestPrice *string `json:"after_per_request_price,omitempty"`
}

type upstreamPriceIntervalRollback struct {
	ID                   int64   `json:"id"`
	InputPrice           *string `json:"input_price,omitempty"`
	OutputPrice          *string `json:"output_price,omitempty"`
	CacheWritePrice      *string `json:"cache_write_price,omitempty"`
	CacheReadPrice       *string `json:"cache_read_price,omitempty"`
	PerRequestPrice      *string `json:"per_request_price,omitempty"`
	AfterInputPrice      *string `json:"after_input_price,omitempty"`
	AfterOutputPrice     *string `json:"after_output_price,omitempty"`
	AfterCacheWritePrice *string `json:"after_cache_write_price,omitempty"`
	AfterCacheReadPrice  *string `json:"after_cache_read_price,omitempty"`
	AfterPerRequestPrice *string `json:"after_per_request_price,omitempty"`
}

type upstreamPriceDisplayRollback struct {
	ID                      int64   `json:"id"`
	ModelMultiplier         *string `json:"model_multiplier,omitempty"`
	PerRequestLTE256K       *string `json:"per_request_lte_256k,omitempty"`
	PerRequest256K512K      *string `json:"per_request_256k_512k,omitempty"`
	PerRequestGT512K        *string `json:"per_request_gt_512k,omitempty"`
	AfterModelMultiplier    *string `json:"after_model_multiplier,omitempty"`
	AfterPerRequestLTE256K  *string `json:"after_per_request_lte_256k,omitempty"`
	AfterPerRequest256K512K *string `json:"after_per_request_256k_512k,omitempty"`
	AfterPerRequestGT512K   *string `json:"after_per_request_gt_512k,omitempty"`
}

type upstreamPriceRollbackSnapshot struct {
	Channels  []upstreamPriceChannelRollback  `json:"channels"`
	Displays  []upstreamPriceDisplayRollback  `json:"displays"`
	Intervals []upstreamPriceIntervalRollback `json:"intervals"`
}

type upstreamPriceApplyEvidence struct {
	Model       string
	Suggested   domain.UpstreamPriceVector
	BillingMode string
}

func (r *upstreamPriceMonitorRepository) ApplyRun(
	ctx context.Context,
	runID int64,
	snapshotHash string,
	channelIDs []int64,
	accountIDs []int64,
	decimals int,
	maxAgeMinutes int,
	expectedConfigUpdatedAt time.Time,
	expectedModelCatalogRevision int64,
) error {
	if r == nil || r.db == nil || runID <= 0 || strings.TrimSpace(snapshotHash) == "" || len(channelIDs) == 0 || len(accountIDs) == 0 ||
		maxAgeMinutes <= 0 || expectedConfigUpdatedAt.IsZero() || expectedModelCatalogRevision <= 0 {
		return service.ErrUpstreamPriceRunNotApplicable
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(742193847561)`); err != nil {
		return err
	}

	var trigger, status, storedHash string
	var appliedAt, finishedAt sql.NullTime
	if err := tx.QueryRowContext(ctx, `SELECT trigger,status,snapshot_hash,applied_at,finished_at
		FROM upstream_price_monitor_runs WHERE id=$1 FOR UPDATE`, runID).
		Scan(&trigger, &status, &storedHash, &appliedAt, &finishedAt); err != nil {
		return service.ErrUpstreamPriceRunNotApplicable
	}
	if status != string(domain.UpstreamPriceMonitorRunStatusCompleted) || appliedAt.Valid || !finishedAt.Valid ||
		finishedAt.Time.Before(time.Now().Add(-time.Duration(maxAgeMinutes)*time.Minute)) {
		return service.ErrUpstreamPriceRunNotApplicable
	}
	if storedHash != strings.TrimSpace(snapshotHash) {
		return service.ErrUpstreamPriceSnapshotMismatch
	}
	var newerAppliedSnapshot bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM upstream_price_monitor_runs
		WHERE applied_at IS NOT NULL AND (finished_at > $1 OR (finished_at=$1 AND id>$2)))`,
		finishedAt.Time, runID).Scan(&newerAppliedSnapshot); err != nil {
		return err
	}
	if newerAppliedSnapshot {
		return service.ErrUpstreamPriceRunNotApplicable
	}
	var enabled bool
	var mode string
	var currentConfigUpdatedAt time.Time
	if err := tx.QueryRowContext(ctx, `SELECT enabled,mode,updated_at FROM upstream_price_monitor_config
		WHERE id=1 FOR UPDATE`).Scan(&enabled, &mode, &currentConfigUpdatedAt); err != nil {
		return err
	}
	if !currentConfigUpdatedAt.Equal(expectedConfigUpdatedAt) {
		return service.ErrUpstreamPriceRunNotApplicable
	}
	if trigger == string(domain.UpstreamPriceMonitorRunTriggerScheduled) &&
		(!enabled || mode != string(domain.UpstreamPriceMonitorModeAutoApply)) {
		return service.ErrUpstreamPriceRunNotApplicable
	}
	var currentCatalogRevision int64
	if err := tx.QueryRowContext(ctx, `SELECT revision FROM upstream_price_monitor_model_scan_state
		WHERE id=1 FOR UPDATE`).Scan(&currentCatalogRevision); err != nil {
		return err
	}
	if currentCatalogRevision != expectedModelCatalogRevision {
		return service.ErrUpstreamPriceRunNotApplicable
	}

	evidence, err := loadTrustedApplyEvidence(ctx, tx, runID, accountIDs)
	if err != nil {
		return err
	}
	if len(evidence) == 0 {
		return service.ErrUpstreamPriceRunNotApplicable
	}

	snapshot := upstreamPriceRollbackSnapshot{}
	matchedChannels := 0
	appliedModels := 0
	changedAny := false
	for _, item := range evidence {
		if item.BillingMode == service.DisplayBillingModePerRequest {
			count, changed, applyErr := applyUpstreamPerRequestEvidence(ctx, tx, item, channelIDs, &snapshot)
			if applyErr != nil {
				return applyErr
			}
			if count > 0 {
				matchedChannels += count
			}
			if changed {
				appliedModels++
				changedAny = true
			}
			continue
		}
		input := perMillionToPerToken(item.Suggested.InputPerMillion)
		output := perMillionToPerToken(item.Suggested.OutputPerMillion)
		cacheWrite := perMillionToPerToken(item.Suggested.CacheWritePerMillion)
		cacheRead := perMillionToPerToken(item.Suggested.CacheReadPerMillion)
		itemMatchedChannels := 0
		itemChanged := false
		for _, channelID := range channelIDs {
			if err := rejectSharedUpstreamPricingRow(ctx, tx, channelID, service.DisplayBillingModeToken, item.Model); err != nil {
				return err
			}
			rows, queryErr := tx.QueryContext(ctx, `SELECT id,input_price::text,output_price::text,
				cache_write_price::text,cache_read_price::text,per_request_price::text
				FROM channel_model_pricing
				WHERE channel_id=$1 AND billing_mode='token' AND jsonb_array_length(models)=1
				  AND LOWER(models->>0)=LOWER($2) FOR UPDATE`, channelID, item.Model)
			if queryErr != nil {
				return queryErr
			}
			var lockedRows []upstreamPriceChannelRollback
			for rows.Next() {
				var before upstreamPriceChannelRollback
				var in, out, cw, cr, request sql.NullString
				if scanErr := rows.Scan(&before.ID, &in, &out, &cw, &cr, &request); scanErr != nil {
					_ = rows.Close()
					return scanErr
				}
				before.InputPrice = nullStringPtr(in)
				before.OutputPrice = nullStringPtr(out)
				before.CacheWritePrice = nullStringPtr(cw)
				before.CacheReadPrice = nullStringPtr(cr)
				before.PerRequestPrice = nullStringPtr(request)
				lockedRows = append(lockedRows, before)
			}
			if rowsErr := rows.Err(); rowsErr != nil {
				_ = rows.Close()
				return rowsErr
			}
			if closeErr := rows.Close(); closeErr != nil {
				return closeErr
			}
			for _, before := range lockedRows {
				result, execErr := tx.ExecContext(ctx, `UPDATE channel_model_pricing SET
					input_price=COALESCE($2::numeric,input_price),output_price=COALESCE($3::numeric,output_price),
					cache_write_price=COALESCE($4::numeric,cache_write_price),cache_read_price=COALESCE($5::numeric,cache_read_price),
					updated_at=NOW() WHERE id=$1 AND (
						($2::numeric IS NOT NULL AND input_price IS DISTINCT FROM $2::numeric) OR
						($3::numeric IS NOT NULL AND output_price IS DISTINCT FROM $3::numeric) OR
						($4::numeric IS NOT NULL AND cache_write_price IS DISTINCT FROM $4::numeric) OR
						($5::numeric IS NOT NULL AND cache_read_price IS DISTINCT FROM $5::numeric))`,
					before.ID, input, output, cacheWrite, cacheRead)
				if execErr != nil {
					return execErr
				}
				if affected, _ := result.RowsAffected(); affected > 0 {
					before = channelRollbackWithAfter(before, input, output, cacheWrite, cacheRead, nil)
					snapshot.Channels = append(snapshot.Channels, before)
					itemChanged = true
				}
				intervalChanged, intervalErr := applyUpstreamTokenBaseIntervals(ctx, tx, before.ID, input, output, cacheWrite, cacheRead, &snapshot)
				if intervalErr != nil {
					return intervalErr
				}
				itemChanged = itemChanged || intervalChanged
				itemMatchedChannels++
			}
		}
		if itemMatchedChannels == 0 {
			continue
		}
		matchedChannels += itemMatchedChannels

		multiplier, multiplierOK, multiplierErr := resolveDisplayMultiplier(ctx, tx, item, decimals)
		if multiplierErr != nil {
			return multiplierErr
		}
		requiresDisplay := item.Suggested.InputPerMillion != nil || item.Suggested.OutputPerMillion != nil
		if requiresDisplay && !multiplierOK {
			return fmt.Errorf("%w: model %s input/output price cannot be represented by one display multiplier",
				service.ErrUpstreamPriceRunNotApplicable, item.Model)
		}
		if requiresDisplay {
			displayRows, queryErr := tx.QueryContext(ctx, `SELECT id,model_multiplier::text FROM display_model_prices
				WHERE billing_mode='token' AND LOWER(model_name)=LOWER($1) FOR UPDATE`, item.Model)
			if queryErr != nil {
				return queryErr
			}
			var lockedDisplays []upstreamPriceDisplayRollback
			for displayRows.Next() {
				var before upstreamPriceDisplayRollback
				var old sql.NullString
				if scanErr := displayRows.Scan(&before.ID, &old); scanErr != nil {
					_ = displayRows.Close()
					return scanErr
				}
				before.ModelMultiplier = nullStringPtr(old)
				lockedDisplays = append(lockedDisplays, before)
			}
			if rowsErr := displayRows.Err(); rowsErr != nil {
				_ = displayRows.Close()
				return rowsErr
			}
			if closeErr := displayRows.Close(); closeErr != nil {
				return closeErr
			}
			if len(lockedDisplays) == 0 {
				return fmt.Errorf("%w: model %s has no token display-price row", service.ErrUpstreamPriceRunNotApplicable, item.Model)
			}
			for _, before := range lockedDisplays {
				result, execErr := tx.ExecContext(ctx, `UPDATE display_model_prices SET model_multiplier=$2::numeric,updated_at=NOW()
					WHERE id=$1 AND model_multiplier IS DISTINCT FROM $2::numeric`, before.ID, multiplier.String())
				if execErr != nil {
					return execErr
				}
				if affected, _ := result.RowsAffected(); affected > 0 {
					before = displayRollbackWithAfter(before, multiplier.String(), nil, nil, nil)
					snapshot.Displays = append(snapshot.Displays, before)
					itemChanged = true
				}
			}
		}
		if itemChanged {
			appliedModels++
			changedAny = true
		}
	}
	if matchedChannels == 0 {
		return fmt.Errorf("%w: no configured channel price rows matched trusted evidence", service.ErrUpstreamPriceRunNotApplicable)
	}

	rawSnapshot, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	// Only the most recent monitor apply may be rolled back. This prevents an
	// older snapshot from overwriting a newer successful monitor run.
	if changedAny {
		if _, err := tx.ExecContext(ctx, `UPDATE upstream_price_monitor_runs SET rollback_available=FALSE
			WHERE id<>$1 AND rollback_available=TRUE`, runID); err != nil {
			return err
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE upstream_price_monitor_runs SET applied_at=NOW(),rollback_available=$4,
		rollback_snapshot=CASE WHEN $4 THEN $2 ELSE NULL END,
		summary=jsonb_set(summary,'{applied_models}',to_jsonb($3::int),true)
		WHERE id=$1 AND applied_at IS NULL`, runID, rawSnapshot, appliedModels, changedAny)
	if err != nil {
		return err
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected != 1 {
		return service.ErrUpstreamPriceRunNotApplicable
	}
	return tx.Commit()
}

// applyUpstreamTokenBaseIntervals keeps the normal-context interval aligned
// with the channel row. The resolver gives an explicit interval precedence
// over the flat row, so updating only channel_model_pricing would leave live
// billing on the old value. Higher-context intervals are deliberately left
// untouched because passive default-tier evidence cannot infer their pricing.
func applyUpstreamTokenBaseIntervals(
	ctx context.Context,
	tx *sql.Tx,
	pricingID int64,
	input, output, cacheWrite, cacheRead any,
	snapshot *upstreamPriceRollbackSnapshot,
) (bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id,input_price::text,output_price::text,
		cache_write_price::text,cache_read_price::text,per_request_price::text,
		input_multiplier IS NOT NULL,output_multiplier IS NOT NULL,
		cache_write_multiplier IS NOT NULL,cache_read_multiplier IS NOT NULL
		FROM channel_pricing_intervals WHERE pricing_id=$1 AND min_tokens=0
		ORDER BY sort_order,id FOR UPDATE`, pricingID)
	if err != nil {
		return false, err
	}
	type lockedTokenInterval struct {
		rollback                                        upstreamPriceIntervalRollback
		hasInputMultiplier, hasOutputMultiplier         bool
		hasCacheWriteMultiplier, hasCacheReadMultiplier bool
	}
	var locked []lockedTokenInterval
	for rows.Next() {
		var item lockedTokenInterval
		var in, out, cw, cr, request sql.NullString
		if err := rows.Scan(&item.rollback.ID, &in, &out, &cw, &cr, &request,
			&item.hasInputMultiplier, &item.hasOutputMultiplier,
			&item.hasCacheWriteMultiplier, &item.hasCacheReadMultiplier); err != nil {
			_ = rows.Close()
			return false, err
		}
		item.rollback.InputPrice = nullStringPtr(in)
		item.rollback.OutputPrice = nullStringPtr(out)
		item.rollback.CacheWritePrice = nullStringPtr(cw)
		item.rollback.CacheReadPrice = nullStringPtr(cr)
		item.rollback.PerRequestPrice = nullStringPtr(request)
		locked = append(locked, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return false, err
	}
	if err := rows.Close(); err != nil {
		return false, err
	}
	changed := false
	for _, item := range locked {
		before := item.rollback
		intervalInput, intervalOutput := input, output
		intervalCacheWrite, intervalCacheRead := cacheWrite, cacheRead
		if before.InputPrice == nil && item.hasInputMultiplier {
			intervalInput = nil
		}
		if before.OutputPrice == nil && item.hasOutputMultiplier {
			intervalOutput = nil
		}
		if before.CacheWritePrice == nil && item.hasCacheWriteMultiplier {
			intervalCacheWrite = nil
		}
		if before.CacheReadPrice == nil && item.hasCacheReadMultiplier {
			intervalCacheRead = nil
		}
		result, err := tx.ExecContext(ctx, `UPDATE channel_pricing_intervals SET
			input_price=COALESCE($2::numeric,input_price),output_price=COALESCE($3::numeric,output_price),
			cache_write_price=COALESCE($4::numeric,cache_write_price),cache_read_price=COALESCE($5::numeric,cache_read_price),
			updated_at=NOW() WHERE id=$1 AND (
				($2::numeric IS NOT NULL AND input_price IS DISTINCT FROM $2::numeric) OR
				($3::numeric IS NOT NULL AND output_price IS DISTINCT FROM $3::numeric) OR
				($4::numeric IS NOT NULL AND cache_write_price IS DISTINCT FROM $4::numeric) OR
				($5::numeric IS NOT NULL AND cache_read_price IS DISTINCT FROM $5::numeric))`,
			before.ID, intervalInput, intervalOutput, intervalCacheWrite, intervalCacheRead)
		if err != nil {
			return false, err
		}
		if affected, _ := result.RowsAffected(); affected > 0 {
			before = intervalRollbackWithAfter(before, intervalInput, intervalOutput, intervalCacheWrite, intervalCacheRead, nil)
			snapshot.Intervals = append(snapshot.Intervals, before)
			changed = true
		}
	}
	return changed, nil
}

func rejectSharedUpstreamPricingRow(ctx context.Context, tx *sql.Tx, channelID int64, billingMode, model string) error {
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM channel_model_pricing cmp
		WHERE cmp.channel_id=$1 AND cmp.billing_mode=$2 AND jsonb_array_length(cmp.models)<>1
		  AND EXISTS (SELECT 1 FROM jsonb_array_elements_text(cmp.models) AS model_name(value)
		              WHERE LOWER(value)=LOWER($3)))`, channelID, billingMode, model).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("%w: model %s is inside a shared price row on channel %d; split it into a single-model row before auto-apply",
			service.ErrUpstreamPriceRunNotApplicable, model, channelID)
	}
	return nil
}

func (r *upstreamPriceMonitorRepository) RollbackRun(ctx context.Context, runID int64, snapshotHash string) error {
	if r == nil || r.db == nil || runID <= 0 || strings.TrimSpace(snapshotHash) == "" {
		return service.ErrUpstreamPriceRunNotApplicable
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(742193847561)`); err != nil {
		return err
	}
	var storedHash string
	var appliedAt sql.NullTime
	var rollbackAvailable bool
	var raw []byte
	if err := tx.QueryRowContext(ctx, `SELECT snapshot_hash,applied_at,rollback_available,rollback_snapshot
		FROM upstream_price_monitor_runs WHERE id=$1 FOR UPDATE`, runID).
		Scan(&storedHash, &appliedAt, &rollbackAvailable, &raw); err != nil {
		return service.ErrUpstreamPriceRunNotApplicable
	}
	if storedHash != strings.TrimSpace(snapshotHash) {
		return service.ErrUpstreamPriceSnapshotMismatch
	}
	if !appliedAt.Valid || !rollbackAvailable || len(raw) == 0 {
		return service.ErrUpstreamPriceRunNotApplicable
	}
	var newerApplyExists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM upstream_price_monitor_runs
		WHERE rollback_snapshot IS NOT NULL AND applied_at > $1)`, appliedAt.Time).Scan(&newerApplyExists); err != nil {
		return err
	}
	if newerApplyExists {
		return service.ErrUpstreamPriceRunNotApplicable
	}
	var snapshot upstreamPriceRollbackSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return err
	}
	for _, row := range snapshot.Channels {
		result, err := tx.ExecContext(ctx, `UPDATE channel_model_pricing SET input_price=$2::numeric,output_price=$3::numeric,
			cache_write_price=$4::numeric,cache_read_price=$5::numeric,per_request_price=$6::numeric,updated_at=NOW()
			WHERE id=$1 AND input_price IS NOT DISTINCT FROM $7::numeric AND output_price IS NOT DISTINCT FROM $8::numeric
			  AND cache_write_price IS NOT DISTINCT FROM $9::numeric AND cache_read_price IS NOT DISTINCT FROM $10::numeric
			  AND per_request_price IS NOT DISTINCT FROM $11::numeric`, row.ID, row.InputPrice, row.OutputPrice,
			row.CacheWritePrice, row.CacheReadPrice, row.PerRequestPrice, row.AfterInputPrice, row.AfterOutputPrice,
			row.AfterCacheWritePrice, row.AfterCacheReadPrice, row.AfterPerRequestPrice)
		if err != nil {
			return err
		}
		if affected, _ := result.RowsAffected(); affected != 1 {
			return service.ErrUpstreamPriceRunNotApplicable
		}
	}
	for _, row := range snapshot.Intervals {
		result, err := tx.ExecContext(ctx, `UPDATE channel_pricing_intervals SET input_price=$2::numeric,
			output_price=$3::numeric,cache_write_price=$4::numeric,cache_read_price=$5::numeric,
			per_request_price=$6::numeric,updated_at=NOW()
			WHERE id=$1 AND input_price IS NOT DISTINCT FROM $7::numeric AND output_price IS NOT DISTINCT FROM $8::numeric
			  AND cache_write_price IS NOT DISTINCT FROM $9::numeric AND cache_read_price IS NOT DISTINCT FROM $10::numeric
			  AND per_request_price IS NOT DISTINCT FROM $11::numeric`, row.ID, row.InputPrice, row.OutputPrice,
			row.CacheWritePrice, row.CacheReadPrice, row.PerRequestPrice, row.AfterInputPrice, row.AfterOutputPrice,
			row.AfterCacheWritePrice, row.AfterCacheReadPrice, row.AfterPerRequestPrice)
		if err != nil {
			return err
		}
		if affected, _ := result.RowsAffected(); affected != 1 {
			return service.ErrUpstreamPriceRunNotApplicable
		}
	}
	for _, row := range snapshot.Displays {
		result, err := tx.ExecContext(ctx, `UPDATE display_model_prices SET model_multiplier=$2::numeric,
			per_request_lte_256k=$3::numeric,per_request_256k_512k_override=$4::numeric,
			per_request_gt_512k_override=$5::numeric,updated_at=NOW()
			WHERE id=$1 AND model_multiplier IS NOT DISTINCT FROM $6::numeric
			  AND per_request_lte_256k IS NOT DISTINCT FROM $7::numeric
			  AND per_request_256k_512k_override IS NOT DISTINCT FROM $8::numeric
			  AND per_request_gt_512k_override IS NOT DISTINCT FROM $9::numeric`, row.ID, row.ModelMultiplier,
			row.PerRequestLTE256K, row.PerRequest256K512K, row.PerRequestGT512K, row.AfterModelMultiplier,
			row.AfterPerRequestLTE256K, row.AfterPerRequest256K512K, row.AfterPerRequestGT512K)
		if err != nil {
			return err
		}
		if affected, _ := result.RowsAffected(); affected != 1 {
			return service.ErrUpstreamPriceRunNotApplicable
		}
	}
	rolledBackAt := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := tx.ExecContext(ctx, `UPDATE upstream_price_monitor_runs SET rollback_available=FALSE,
		summary=jsonb_set(summary,'{rolled_back_at}',to_jsonb($2::text),true)
		WHERE id=$1 AND rollback_available=TRUE`, runID, rolledBackAt)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return service.ErrUpstreamPriceRunNotApplicable
	}
	return tx.Commit()
}

func loadTrustedApplyEvidence(ctx context.Context, tx *sql.Tx, runID int64, accountIDs []int64) ([]upstreamPriceApplyEvidence, error) {
	rows, err := tx.QueryContext(ctx, `SELECT account_id,model_name,billing_mode,status,suggested_prices
		FROM upstream_price_monitor_evidence WHERE run_id=$1
		ORDER BY LOWER(model_name),account_id,observed_at DESC,id DESC FOR UPDATE`, runID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	type tokenCandidate struct {
		model     string
		byAccount map[int64]domain.UpstreamPriceVector
	}
	tokenCandidates := make(map[string]*tokenCandidate)
	perRequest := make(map[string]upstreamPriceApplyEvidence)
	expectedAccounts := make(map[int64]struct{}, len(accountIDs))
	for _, accountID := range accountIDs {
		expectedAccounts[accountID] = struct{}{}
	}
	for rows.Next() {
		var accountID int64
		var model, billingMode, status string
		var raw []byte
		if err := rows.Scan(&accountID, &model, &billingMode, &status, &raw); err != nil {
			return nil, err
		}
		if status != string(domain.UpstreamPriceEvidenceStatusTrusted) {
			continue
		}
		var prices domain.UpstreamPriceVector
		if err := json.Unmarshal(raw, &prices); err != nil {
			return nil, err
		}
		key := strings.ToLower(strings.TrimSpace(model)) + "\x00" + billingMode
		if billingMode == service.DisplayBillingModePerRequest {
			if previous, exists := perRequest[key]; exists {
				merged, compatible := mergeCompatibleUpstreamPriceVectors(previous.Suggested, prices)
				if !compatible {
					return nil, fmt.Errorf("%w: duplicate per-request declarations disagree for model %s",
						service.ErrUpstreamPriceRunNotApplicable, model)
				}
				prices = merged
			}
			perRequest[key] = upstreamPriceApplyEvidence{Model: model, Suggested: prices, BillingMode: billingMode}
			continue
		}
		if _, selected := expectedAccounts[accountID]; !selected {
			continue
		}
		candidate := tokenCandidates[key]
		if candidate == nil {
			candidate = &tokenCandidate{model: model, byAccount: make(map[int64]domain.UpstreamPriceVector)}
			tokenCandidates[key] = candidate
		}
		if previous, exists := candidate.byAccount[accountID]; exists {
			merged, compatible := mergeCompatibleUpstreamPriceVectors(previous, prices)
			if !compatible {
				return nil, fmt.Errorf("%w: duplicate trusted evidence disagrees for account %d model %s",
					service.ErrUpstreamPriceRunNotApplicable, accountID, model)
			}
			prices = merged
		}
		candidate.byAccount[accountID] = prices
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]upstreamPriceApplyEvidence, 0, len(tokenCandidates)+len(perRequest))
	for _, item := range perRequest {
		out = append(out, item)
	}
	for _, candidate := range tokenCandidates {
		if len(candidate.byAccount) != len(expectedAccounts) {
			continue
		}
		vectors := make([]domain.UpstreamPriceVector, 0, len(accountIDs))
		complete := true
		for _, accountID := range accountIDs {
			value, exists := candidate.byAccount[accountID]
			if !exists {
				complete = false
				break
			}
			vectors = append(vectors, value)
		}
		if !complete {
			continue
		}
		merged := mergeMaximumCommonUpstreamPriceVector(vectors)
		if upstreamPriceVectorEmpty(merged) {
			continue
		}
		out = append(out, upstreamPriceApplyEvidence{
			Model: candidate.model, Suggested: merged, BillingMode: service.DisplayBillingModeToken,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		left, right := strings.ToLower(out[i].Model), strings.ToLower(out[j].Model)
		if left != right {
			return left < right
		}
		return out[i].BillingMode < out[j].BillingMode
	})
	return out, nil
}

func mergeMaximumCommonUpstreamPriceVector(values []domain.UpstreamPriceVector) domain.UpstreamPriceVector {
	return domain.UpstreamPriceVector{
		InputPerMillion:      maximumCommonFloat(values, func(v domain.UpstreamPriceVector) *float64 { return v.InputPerMillion }),
		OutputPerMillion:     maximumCommonFloat(values, func(v domain.UpstreamPriceVector) *float64 { return v.OutputPerMillion }),
		CacheWritePerMillion: maximumCommonFloat(values, func(v domain.UpstreamPriceVector) *float64 { return v.CacheWritePerMillion }),
		CacheReadPerMillion:  maximumCommonFloat(values, func(v domain.UpstreamPriceVector) *float64 { return v.CacheReadPerMillion }),
	}
}

func mergeCompatibleUpstreamPriceVectors(a, b domain.UpstreamPriceVector) (domain.UpstreamPriceVector, bool) {
	var out domain.UpstreamPriceVector
	fields := []struct {
		a, b *float64
		set  func(*float64)
	}{
		{a.InputPerMillion, b.InputPerMillion, func(v *float64) { out.InputPerMillion = v }},
		{a.OutputPerMillion, b.OutputPerMillion, func(v *float64) { out.OutputPerMillion = v }},
		{a.CacheWritePerMillion, b.CacheWritePerMillion, func(v *float64) { out.CacheWritePerMillion = v }},
		{a.CacheReadPerMillion, b.CacheReadPerMillion, func(v *float64) { out.CacheReadPerMillion = v }},
		{a.PerRequestLTE256K, b.PerRequestLTE256K, func(v *float64) { out.PerRequestLTE256K = v }},
		{a.PerRequest256K512K, b.PerRequest256K512K, func(v *float64) { out.PerRequest256K512K = v }},
		{a.PerRequestGT512K, b.PerRequestGT512K, func(v *float64) { out.PerRequestGT512K = v }},
	}
	for _, field := range fields {
		merged, compatible := mergeCompatiblePrice(field.a, field.b)
		if !compatible {
			return domain.UpstreamPriceVector{}, false
		}
		field.set(merged)
	}
	return out, true
}

func mergeCompatiblePrice(a, b *float64) (*float64, bool) {
	if a == nil {
		return b, true
	}
	if b == nil {
		return a, true
	}
	tolerance := math.Max(1e-8, math.Max(math.Abs(*a), math.Abs(*b))*.005)
	if math.Abs(*a-*b) > tolerance {
		return nil, false
	}
	value := math.Max(*a, *b)
	return &value, true
}

func maximumCommonFloat(values []domain.UpstreamPriceVector, pick func(domain.UpstreamPriceVector) *float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	var maximum float64
	for index, value := range values {
		selected := pick(value)
		if selected == nil {
			return nil
		}
		if index == 0 || *selected > maximum {
			maximum = *selected
		}
	}
	return &maximum
}

func upstreamPriceVectorEmpty(value domain.UpstreamPriceVector) bool {
	return value.InputPerMillion == nil && value.OutputPerMillion == nil &&
		value.CacheWritePerMillion == nil && value.CacheReadPerMillion == nil &&
		value.PerRequestLTE256K == nil && value.PerRequest256K512K == nil && value.PerRequestGT512K == nil
}

func applyUpstreamPerRequestEvidence(
	ctx context.Context,
	tx *sql.Tx,
	evidence upstreamPriceApplyEvidence,
	channelIDs []int64,
	snapshot *upstreamPriceRollbackSnapshot,
) (int, bool, error) {
	tierPrices := []*float64{
		evidence.Suggested.PerRequestLTE256K,
		evidence.Suggested.PerRequest256K512K,
		evidence.Suggested.PerRequestGT512K,
	}
	for _, price := range tierPrices {
		if price == nil {
			return 0, false, fmt.Errorf("%w: incomplete per-request tiers for model %s", service.ErrUpstreamPriceRunNotApplicable, evidence.Model)
		}
	}

	matched := 0
	changed := false
	for _, channelID := range channelIDs {
		if err := rejectSharedUpstreamPricingRow(ctx, tx, channelID, service.DisplayBillingModePerRequest, evidence.Model); err != nil {
			return matched, false, err
		}
		rows, err := tx.QueryContext(ctx, `SELECT id,input_price::text,output_price::text,
			cache_write_price::text,cache_read_price::text,per_request_price::text FROM channel_model_pricing
			WHERE channel_id=$1 AND billing_mode='per_request' AND jsonb_array_length(models)=1
			  AND LOWER(models->>0)=LOWER($2) FOR UPDATE`, channelID, evidence.Model)
		if err != nil {
			return matched, false, err
		}
		var lockedRows []upstreamPriceChannelRollback
		for rows.Next() {
			var row upstreamPriceChannelRollback
			var in, out, cw, cr, request sql.NullString
			if err := rows.Scan(&row.ID, &in, &out, &cw, &cr, &request); err != nil {
				_ = rows.Close()
				return matched, false, err
			}
			row.InputPrice = nullStringPtr(in)
			row.OutputPrice = nullStringPtr(out)
			row.CacheWritePrice = nullStringPtr(cw)
			row.CacheReadPrice = nullStringPtr(cr)
			row.PerRequestPrice = nullStringPtr(request)
			lockedRows = append(lockedRows, row)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return matched, false, err
		}
		if err := rows.Close(); err != nil {
			return matched, false, err
		}
		for _, row := range lockedRows {
			result, err := tx.ExecContext(ctx, `UPDATE channel_model_pricing SET
				per_request_price=$2::numeric,updated_at=NOW()
				WHERE id=$1 AND per_request_price IS DISTINCT FROM $2::numeric`,
				row.ID, decimalString(evidence.Suggested.PerRequestLTE256K))
			if err != nil {
				return matched, false, err
			}
			if affected, _ := result.RowsAffected(); affected > 0 {
				row = channelRollbackWithAfter(row, nil, nil, nil, nil, decimalString(evidence.Suggested.PerRequestLTE256K))
				snapshot.Channels = append(snapshot.Channels, row)
				changed = true
			}

			intervals, err := tx.QueryContext(ctx, `SELECT id,min_tokens,max_tokens,input_price::text,
				output_price::text,cache_write_price::text,cache_read_price::text,per_request_price::text
				FROM channel_pricing_intervals WHERE pricing_id=$1 ORDER BY min_tokens,sort_order,id FOR UPDATE`, row.ID)
			if err != nil {
				return matched, false, err
			}
			type lockedInterval struct {
				rollback  upstreamPriceIntervalRollback
				minTokens int64
				maxTokens sql.NullInt64
			}
			var lockedIntervals []lockedInterval
			for intervals.Next() {
				var item lockedInterval
				var in, out, cw, cr, request sql.NullString
				if err := intervals.Scan(&item.rollback.ID, &item.minTokens, &item.maxTokens,
					&in, &out, &cw, &cr, &request); err != nil {
					_ = intervals.Close()
					return matched, false, err
				}
				item.rollback.InputPrice = nullStringPtr(in)
				item.rollback.OutputPrice = nullStringPtr(out)
				item.rollback.CacheWritePrice = nullStringPtr(cw)
				item.rollback.CacheReadPrice = nullStringPtr(cr)
				item.rollback.PerRequestPrice = nullStringPtr(request)
				lockedIntervals = append(lockedIntervals, item)
			}
			if err := intervals.Err(); err != nil {
				_ = intervals.Close()
				return matched, false, err
			}
			if err := intervals.Close(); err != nil {
				return matched, false, err
			}
			if len(lockedIntervals) != 3 || lockedIntervals[0].minTokens != 0 ||
				!lockedIntervals[0].maxTokens.Valid || lockedIntervals[0].maxTokens.Int64 != 256000 ||
				lockedIntervals[1].minTokens != 256000 || !lockedIntervals[1].maxTokens.Valid || lockedIntervals[1].maxTokens.Int64 != 512000 ||
				lockedIntervals[2].minTokens != 512000 || lockedIntervals[2].maxTokens.Valid {
				return matched, false, fmt.Errorf("%w: channel price row %d does not have the exact 0/256K/512K request tiers",
					service.ErrUpstreamPriceRunNotApplicable, row.ID)
			}
			for index, item := range lockedIntervals {
				result, err := tx.ExecContext(ctx, `UPDATE channel_pricing_intervals SET per_request_price=$2::numeric,updated_at=NOW()
					WHERE id=$1 AND per_request_price IS DISTINCT FROM $2::numeric`, item.rollback.ID, decimalString(tierPrices[index]))
				if err != nil {
					return matched, false, err
				}
				if affected, _ := result.RowsAffected(); affected > 0 {
					before := intervalRollbackWithAfter(item.rollback, nil, nil, nil, nil, decimalString(tierPrices[index]))
					snapshot.Intervals = append(snapshot.Intervals, before)
					changed = true
				}
			}
			matched++
		}
	}
	if matched == 0 {
		return 0, false, nil
	}

	displays, err := tx.QueryContext(ctx, `SELECT id,per_request_lte_256k::text,
		per_request_256k_512k_override::text,per_request_gt_512k_override::text
		FROM display_model_prices WHERE billing_mode='per_request' AND LOWER(model_name)=LOWER($1) FOR UPDATE`, evidence.Model)
	if err != nil {
		return matched, false, err
	}
	var lockedDisplays []upstreamPriceDisplayRollback
	for displays.Next() {
		var row upstreamPriceDisplayRollback
		var small, middle, large sql.NullString
		if err := displays.Scan(&row.ID, &small, &middle, &large); err != nil {
			_ = displays.Close()
			return matched, false, err
		}
		row.PerRequestLTE256K = nullStringPtr(small)
		row.PerRequest256K512K = nullStringPtr(middle)
		row.PerRequestGT512K = nullStringPtr(large)
		lockedDisplays = append(lockedDisplays, row)
	}
	if err := displays.Err(); err != nil {
		_ = displays.Close()
		return matched, false, err
	}
	if err := displays.Close(); err != nil {
		return matched, false, err
	}
	if len(lockedDisplays) == 0 {
		return matched, false, fmt.Errorf("%w: model %s has no per-request display-price row",
			service.ErrUpstreamPriceRunNotApplicable, evidence.Model)
	}
	for _, row := range lockedDisplays {
		result, err := tx.ExecContext(ctx, `UPDATE display_model_prices SET per_request_lte_256k=$2::numeric,
			per_request_256k_512k_override=$3::numeric,per_request_gt_512k_override=$4::numeric,updated_at=NOW()
			WHERE id=$1 AND (per_request_lte_256k IS DISTINCT FROM $2::numeric OR
				per_request_256k_512k_override IS DISTINCT FROM $3::numeric OR
				per_request_gt_512k_override IS DISTINCT FROM $4::numeric)`, row.ID,
			decimalString(evidence.Suggested.PerRequestLTE256K), decimalString(evidence.Suggested.PerRequest256K512K),
			decimalString(evidence.Suggested.PerRequestGT512K))
		if err != nil {
			return matched, false, err
		}
		if affected, _ := result.RowsAffected(); affected > 0 {
			row = displayRollbackWithAfter(row, nil, decimalString(evidence.Suggested.PerRequestLTE256K),
				decimalString(evidence.Suggested.PerRequest256K512K), decimalString(evidence.Suggested.PerRequestGT512K))
			snapshot.Displays = append(snapshot.Displays, row)
			changed = true
		}
	}
	return matched, changed, nil
}

func decimalString(value *float64) any {
	if value == nil {
		return nil
	}
	return decimal.NewFromFloat(*value).String()
}

func numericAfter(before *string, next any) *string {
	if next == nil {
		return before
	}
	switch value := next.(type) {
	case string:
		out := value
		return &out
	case *string:
		return value
	default:
		out := fmt.Sprint(value)
		return &out
	}
}

func channelRollbackWithAfter(before upstreamPriceChannelRollback, input, output, cacheWrite, cacheRead, request any) upstreamPriceChannelRollback {
	before.AfterInputPrice = numericAfter(before.InputPrice, input)
	before.AfterOutputPrice = numericAfter(before.OutputPrice, output)
	before.AfterCacheWritePrice = numericAfter(before.CacheWritePrice, cacheWrite)
	before.AfterCacheReadPrice = numericAfter(before.CacheReadPrice, cacheRead)
	before.AfterPerRequestPrice = numericAfter(before.PerRequestPrice, request)
	return before
}

func intervalRollbackWithAfter(before upstreamPriceIntervalRollback, input, output, cacheWrite, cacheRead, request any) upstreamPriceIntervalRollback {
	before.AfterInputPrice = numericAfter(before.InputPrice, input)
	before.AfterOutputPrice = numericAfter(before.OutputPrice, output)
	before.AfterCacheWritePrice = numericAfter(before.CacheWritePrice, cacheWrite)
	before.AfterCacheReadPrice = numericAfter(before.CacheReadPrice, cacheRead)
	before.AfterPerRequestPrice = numericAfter(before.PerRequestPrice, request)
	return before
}

func displayRollbackWithAfter(before upstreamPriceDisplayRollback, multiplier, low, middle, high any) upstreamPriceDisplayRollback {
	before.AfterModelMultiplier = numericAfter(before.ModelMultiplier, multiplier)
	before.AfterPerRequestLTE256K = numericAfter(before.PerRequestLTE256K, low)
	before.AfterPerRequest256K512K = numericAfter(before.PerRequest256K512K, middle)
	before.AfterPerRequestGT512K = numericAfter(before.PerRequestGT512K, high)
	return before
}

func resolveDisplayMultiplier(ctx context.Context, tx *sql.Tx, evidence upstreamPriceApplyEvidence, decimals int) (decimal.Decimal, bool, error) {
	var officialInput, officialOutput sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT official_input_per_million::text,official_output_per_million::text
		FROM display_model_prices WHERE billing_mode='token' AND LOWER(model_name)=LOWER($1)
		ORDER BY id LIMIT 1 FOR UPDATE`, evidence.Model).Scan(&officialInput, &officialOutput)
	if errors.Is(err, sql.ErrNoRows) {
		return decimal.Zero, false, nil
	}
	if err != nil {
		return decimal.Zero, false, err
	}
	return calculateDisplayMultiplier(evidence, officialInput, officialOutput, decimals)
}

func calculateDisplayMultiplier(evidence upstreamPriceApplyEvidence, officialInput, officialOutput sql.NullString, decimals int) (decimal.Decimal, bool, error) {
	var multiplier decimal.Decimal
	set := false
	if evidence.Suggested.InputPerMillion != nil && officialInput.Valid {
		base, err := decimal.NewFromString(officialInput.String)
		if err != nil || base.Sign() <= 0 {
			return decimal.Zero, false, nil
		}
		multiplier = decimal.NewFromFloat(*evidence.Suggested.InputPerMillion).Div(base)
		set = true
	}
	if evidence.Suggested.OutputPerMillion != nil && officialOutput.Valid {
		base, err := decimal.NewFromString(officialOutput.String)
		if err == nil && base.Sign() > 0 {
			candidate := decimal.NewFromFloat(*evidence.Suggested.OutputPerMillion).Div(base)
			if set && candidate.Sub(multiplier).Abs().GreaterThan(decimal.NewFromFloat(0.001)) {
				return decimal.Zero, false, nil
			}
			if !set {
				multiplier, set = candidate, true
			}
		}
	}
	if !set {
		return decimal.Zero, false, nil
	}
	return multiplier.Round(int32(decimals)), true, nil
}

func perMillionToPerToken(value *float64) any {
	if value == nil {
		return nil
	}
	return decimal.NewFromFloat(*value).Div(decimal.NewFromInt(1_000_000)).StringFixed(12)
}

func nullStringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	out := value.String
	return &out
}

func sameUpstreamPriceVector(a, b domain.UpstreamPriceVector) bool {
	return sameFloatPtr(a.InputPerMillion, b.InputPerMillion) && sameFloatPtr(a.OutputPerMillion, b.OutputPerMillion) &&
		sameFloatPtr(a.CacheWritePerMillion, b.CacheWritePerMillion) && sameFloatPtr(a.CacheReadPerMillion, b.CacheReadPerMillion) &&
		sameFloatPtr(a.PerRequestLTE256K, b.PerRequestLTE256K) && sameFloatPtr(a.PerRequest256K512K, b.PerRequest256K512K) &&
		sameFloatPtr(a.PerRequestGT512K, b.PerRequestGT512K)
}

func sameFloatPtr(a, b *float64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return decimal.NewFromFloat(*a).Sub(decimal.NewFromFloat(*b)).Abs().LessThanOrEqual(decimal.NewFromFloat(1e-9))
}
