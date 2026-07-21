package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

const upstreamBillingRateHistoryColumns = `
	id,
	detected_at,
	group_rate_multiplier,
	user_rate_multiplier,
	peak_rate_enabled,
	peak_start,
	peak_end,
	peak_timezone,
	peak_rate_multiplier,
	resolved_rate_multiplier,
	effective_rate_multiplier`

func appendUpstreamBillingRateHistoryEvent(
	ctx context.Context,
	exec sqlExecutor,
	accountID int64,
	snapshot *service.UpstreamBillingProbeSnapshot,
) error {
	candidate, err := service.BuildUpstreamBillingRateHistoryEvent(snapshot)
	if err != nil {
		return err
	}

	latest, err := latestUpstreamBillingRateHistoryEvent(ctx, exec, accountID, true)
	if err != nil {
		return err
	}
	if service.SameUpstreamBillingRateHistoryValues(latest, candidate) {
		return nil
	}

	_, err = exec.ExecContext(ctx, `
		INSERT INTO account_upstream_billing_rate_events (
			account_id,
			detected_at,
			group_rate_multiplier,
			user_rate_multiplier,
			peak_rate_enabled,
			peak_start,
			peak_end,
			peak_timezone,
			peak_rate_multiplier,
			resolved_rate_multiplier,
			effective_rate_multiplier
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	`, accountID, candidate.DetectedAt, candidate.GroupRateMultiplier,
		candidate.UserRateMultiplier, candidate.PeakRateEnabled, candidate.PeakStart,
		candidate.PeakEnd, candidate.PeakTimezone, candidate.PeakRateMultiplier,
		candidate.ResolvedRateMultiplier, candidate.EffectiveRateMultiplier)
	if err != nil {
		return fmt.Errorf("insert upstream billing rate history: %w", err)
	}
	return nil
}

func latestUpstreamBillingRateHistoryEvent(
	ctx context.Context,
	exec sqlExecutor,
	accountID int64,
	lock bool,
) (*service.UpstreamBillingRateHistoryEvent, error) {
	query := `SELECT ` + upstreamBillingRateHistoryColumns + `
		FROM account_upstream_billing_rate_events
		WHERE account_id = $1
		ORDER BY detected_at DESC, id DESC
		LIMIT 1`
	if lock {
		query += " FOR UPDATE"
	}
	rows, err := exec.QueryContext(ctx, query, accountID)
	if err != nil {
		return nil, fmt.Errorf("query latest upstream billing rate history: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, nil
	}
	event, err := scanUpstreamBillingRateHistoryEvent(rows.Scan)
	if err != nil {
		return nil, fmt.Errorf("scan latest upstream billing rate history: %w", err)
	}
	return event, rows.Err()
}

func scanUpstreamBillingRateHistoryEvent(scan func(...any) error) (*service.UpstreamBillingRateHistoryEvent, error) {
	event := &service.UpstreamBillingRateHistoryEvent{}
	var userRate, peakRate sql.NullFloat64
	var peakStart, peakEnd, peakTimezone sql.NullString
	if err := scan(
		&event.ID,
		&event.DetectedAt,
		&event.GroupRateMultiplier,
		&userRate,
		&event.PeakRateEnabled,
		&peakStart,
		&peakEnd,
		&peakTimezone,
		&peakRate,
		&event.ResolvedRateMultiplier,
		&event.EffectiveRateMultiplier,
	); err != nil {
		return nil, err
	}
	event.DetectedAt = event.DetectedAt.UTC()
	if userRate.Valid {
		value := userRate.Float64
		event.UserRateMultiplier = &value
	}
	if peakStart.Valid {
		value := peakStart.String
		event.PeakStart = &value
	}
	if peakEnd.Valid {
		value := peakEnd.String
		event.PeakEnd = &value
	}
	if peakTimezone.Valid {
		value := peakTimezone.String
		event.PeakTimezone = &value
	}
	if peakRate.Valid {
		value := peakRate.Float64
		event.PeakRateMultiplier = &value
	}
	return event, nil
}

func (r *accountRepository) ListUpstreamBillingRateHistory(
	ctx context.Context,
	accountID int64,
	since time.Time,
	limit int,
) ([]service.UpstreamBillingRateHistoryEvent, bool, error) {
	if r == nil || r.sql == nil {
		return nil, false, fmt.Errorf("account repository is unavailable")
	}
	if limit < 1 || limit > service.UpstreamBillingRateHistoryMaxEvents {
		return nil, false, service.ErrUpstreamBillingRateHistoryRangeInvalid
	}

	rows, err := r.sql.QueryContext(ctx, `SELECT `+upstreamBillingRateHistoryColumns+`
		FROM account_upstream_billing_rate_events
		WHERE account_id = $1 AND detected_at >= $2
		ORDER BY detected_at DESC, id DESC
		LIMIT $3`, accountID, since.UTC(), limit+1)
	if err != nil {
		return nil, false, fmt.Errorf("list upstream billing rate history: %w", err)
	}
	events := make([]service.UpstreamBillingRateHistoryEvent, 0, limit)
	for rows.Next() {
		event, scanErr := scanUpstreamBillingRateHistoryEvent(rows.Scan)
		if scanErr != nil {
			_ = rows.Close()
			return nil, false, fmt.Errorf("scan upstream billing rate history: %w", scanErr)
		}
		events = append(events, *event)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, false, err
	}
	_ = rows.Close()

	truncated := len(events) > limit
	if truncated {
		events = events[:limit]
	} else if len(events) < limit {
		predecessor, err := upstreamBillingRateHistoryPredecessor(ctx, r.sql, accountID, since)
		if err != nil {
			return nil, false, err
		}
		if predecessor != nil {
			events = append(events, *predecessor)
		}
	}

	for left, right := 0, len(events)-1; left < right; left, right = left+1, right-1 {
		events[left], events[right] = events[right], events[left]
	}
	return events, truncated, nil
}

func upstreamBillingRateHistoryPredecessor(
	ctx context.Context,
	exec sqlExecutor,
	accountID int64,
	since time.Time,
) (*service.UpstreamBillingRateHistoryEvent, error) {
	rows, err := exec.QueryContext(ctx, `SELECT `+upstreamBillingRateHistoryColumns+`
		FROM account_upstream_billing_rate_events
		WHERE account_id = $1 AND detected_at < $2
		ORDER BY detected_at DESC, id DESC
		LIMIT 1`, accountID, since.UTC())
	if err != nil {
		return nil, fmt.Errorf("query upstream billing rate history predecessor: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, nil
	}
	event, err := scanUpstreamBillingRateHistoryEvent(rows.Scan)
	if err != nil {
		return nil, fmt.Errorf("scan upstream billing rate history predecessor: %w", err)
	}
	return event, rows.Err()
}

func (r *accountRepository) DeleteUpstreamBillingRateHistoryBefore(
	ctx context.Context,
	cutoff time.Time,
	batchSize int,
) (int64, error) {
	if r == nil || r.sql == nil {
		return 0, fmt.Errorf("account repository is unavailable")
	}
	if batchSize <= 0 {
		batchSize = 5000
	}
	result, err := r.sql.ExecContext(ctx, `
		WITH batch AS (
			SELECT id
			FROM account_upstream_billing_rate_events
			WHERE detected_at < $1
			ORDER BY detected_at, id
			LIMIT $2
		)
		DELETE FROM account_upstream_billing_rate_events
		WHERE id IN (SELECT id FROM batch)
	`, cutoff.UTC(), batchSize)
	if err != nil {
		return 0, fmt.Errorf("delete expired upstream billing rate history: %w", err)
	}
	return result.RowsAffected()
}

func deleteUpstreamBillingRateHistory(ctx context.Context, exec sqlExecutor, accountIDs []int64) error {
	if len(accountIDs) == 0 {
		return nil
	}
	_, err := exec.ExecContext(ctx, `
		DELETE FROM account_upstream_billing_rate_events
		WHERE account_id = ANY($1)
	`, pq.Array(accountIDs))
	if err != nil {
		return fmt.Errorf("delete account upstream billing rate history: %w", err)
	}
	return nil
}

func lockAccountBaseURLChanged(ctx context.Context, exec sqlExecutor, accountID int64, credentialsJSON string) (bool, error) {
	rows, err := exec.QueryContext(ctx, `
		SELECT COALESCE(credentials ->> 'base_url', '') IS DISTINCT FROM
			COALESCE($2::jsonb ->> 'base_url', '')
		FROM accounts
		WHERE id = $1 AND deleted_at IS NULL
		FOR NO KEY UPDATE
	`, accountID, credentialsJSON)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return false, err
		}
		return false, service.ErrAccountNotFound
	}
	var changed bool
	if err := rows.Scan(&changed); err != nil {
		return false, err
	}
	return changed, rows.Err()
}

func lockBulkBaseURLChanges(
	ctx context.Context,
	exec sqlExecutor,
	accountIDs []int64,
	credentialsPatch []byte,
) ([]int64, error) {
	rows, err := exec.QueryContext(ctx, `
		SELECT id,
			COALESCE(credentials ->> 'base_url', '') IS DISTINCT FROM
				COALESCE((COALESCE(credentials, '{}'::jsonb) || $2::jsonb) ->> 'base_url', '')
		FROM accounts
		WHERE id = ANY($1) AND deleted_at IS NULL
		ORDER BY id
		FOR NO KEY UPDATE
	`, pq.Array(accountIDs), credentialsPatch)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	changedIDs := make([]int64, 0)
	for rows.Next() {
		var id int64
		var changed bool
		if err := rows.Scan(&id, &changed); err != nil {
			return nil, err
		}
		if changed {
			changedIDs = append(changedIDs, id)
		}
	}
	return changedIDs, rows.Err()
}
