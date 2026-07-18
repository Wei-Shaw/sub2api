package repository

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

const grokRecoveryRFC3339Pattern = `^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})$`

// ClaimDueGrokFreeRecoveryCandidates atomically claims one bounded page of
// overdue pending accounts. Row locking prevents two instances from probing
// the same account, while the outbox insert is committed with the latch lease.
func (r *accountRepository) ClaimDueGrokFreeRecoveryCandidates(
	ctx context.Context,
	now, nextProbeAt, leaseUntil time.Time,
	limit int,
) ([]service.Account, error) {
	if limit <= 0 {
		return []service.Account{}, nil
	}
	rows, err := r.sql.QueryContext(ctx, `
		WITH eligible AS (
			SELECT
				a.id,
				CASE
					WHEN a.extra ->> $2 ~ $12 THEN (a.extra ->> $2)::timestamptz
					ELSE NULL
				END AS parsed_next_probe_at
			FROM accounts a
			WHERE a.deleted_at IS NULL
				AND a.platform = 'grok'
				AND a.type = 'oauth'
				AND a.status = 'active'
				AND a.schedulable = TRUE
				AND (NOT a.auto_pause_on_expired OR a.expires_at IS NULL OR a.expires_at > $5)
				AND COALESCE(a.extra ->> $1, 'false') = 'true'
		), candidates AS (
			SELECT a.id
			FROM accounts a
			JOIN eligible e ON e.id = a.id
			WHERE e.parsed_next_probe_at IS NULL OR e.parsed_next_probe_at <= $5
			ORDER BY e.parsed_next_probe_at ASC NULLS FIRST, a.id ASC
			FOR UPDATE OF a SKIP LOCKED
			LIMIT $10
		), claimed AS (
			UPDATE accounts a
			SET rate_limited_at = COALESCE(a.rate_limited_at, $5),
				rate_limit_reset_at = GREATEST(COALESCE(a.rate_limit_reset_at, $9), $9),
				extra = COALESCE(a.extra, '{}'::jsonb) || jsonb_build_object(
					$1::text, TRUE,
					$2::text, $8::text,
					$3::text, $6::text,
					$4::text, 'running'
				),
				updated_at = NOW()
			FROM candidates c
			WHERE a.id = c.id
			RETURNING a.id
		), outboxed AS (
			INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)
			SELECT $11, c.id, NULL, NULL FROM claimed c
			RETURNING account_id
		)
		SELECT id FROM claimed ORDER BY id
	`,
		service.GrokFreeRecoveryPendingExtraKey,
		service.GrokFreeRecoveryNextProbeAtExtraKey,
		service.GrokFreeRecoveryLastProbeAtExtraKey,
		service.GrokFreeRecoveryLastProbeResultExtraKey,
		now.UTC(),
		now.UTC().Format(time.RFC3339Nano),
		nextProbeAt.UTC(),
		nextProbeAt.UTC().Format(time.RFC3339Nano),
		leaseUntil.UTC(),
		limit,
		service.SchedulerOutboxEventAccountChanged,
		grokRecoveryRFC3339Pattern,
	)
	if err != nil {
		return nil, fmt.Errorf("claim due Grok Free recovery candidates: %w", err)
	}
	ids, err := scanInt64Rows(rows)
	if err != nil {
		return nil, err
	}
	r.syncSchedulerAccountSnapshots(ctx, ids)
	return r.loadOrderedGrokRecoveryAccounts(ctx, ids)
}

// ListGrokFreeProactiveCandidates returns a keyset-paginated page. The caller
// applies the rolling-token threshold before atomically claiming selected IDs.
func (r *accountRepository) ListGrokFreeProactiveCandidates(
	ctx context.Context,
	now time.Time,
	afterID int64,
	limit int,
) ([]service.Account, error) {
	if limit <= 0 {
		return []service.Account{}, nil
	}
	rows, err := r.sql.QueryContext(ctx, `
		SELECT a.id
		FROM accounts a
		WHERE a.deleted_at IS NULL
			AND a.id > $1
			AND a.platform = 'grok'
			AND a.type = 'oauth'
			AND a.status = 'active'
			AND a.schedulable = TRUE
			AND (NOT a.auto_pause_on_expired OR a.expires_at IS NULL OR a.expires_at > $2)
			AND COALESCE(a.extra ->> $3, 'false') <> 'true'
			AND CASE
				WHEN a.extra ->> $4 ~ $6 THEN (a.extra ->> $4)::timestamptz <= $2
				ELSE TRUE
			END
		ORDER BY a.id ASC
		LIMIT $5
	`,
		afterID,
		now.UTC(),
		service.GrokFreeRecoveryPendingExtraKey,
		service.GrokFreeProactiveNextProbeAtExtraKey,
		limit,
		grokRecoveryRFC3339Pattern,
	)
	if err != nil {
		return nil, fmt.Errorf("list proactive Grok Free recovery candidates: %w", err)
	}
	ids, err := scanInt64Rows(rows)
	if err != nil {
		return nil, err
	}
	return r.loadOrderedGrokRecoveryAccounts(ctx, ids)
}

// ClaimGrokFreeProactiveCandidates conditionally claims only the IDs whose
// token usage crossed the proactive threshold. Stale pages become harmless
// no-ops when another instance or an admin changed the account first.
func (r *accountRepository) ClaimGrokFreeProactiveCandidates(
	ctx context.Context,
	ids []int64,
	now, nextProbeAt, leaseUntil time.Time,
) ([]service.Account, error) {
	if len(ids) == 0 {
		return []service.Account{}, nil
	}
	rows, err := r.sql.QueryContext(ctx, `
		WITH candidates AS (
			SELECT a.id
			FROM accounts a
			WHERE a.id = ANY($1)
				AND a.deleted_at IS NULL
				AND a.platform = 'grok'
				AND a.type = 'oauth'
				AND a.status = 'active'
				AND a.schedulable = TRUE
				AND (NOT a.auto_pause_on_expired OR a.expires_at IS NULL OR a.expires_at > $7)
				AND COALESCE(a.extra ->> $2, 'false') <> 'true'
				AND CASE
					WHEN a.extra ->> $6 ~ $12 THEN (a.extra ->> $6)::timestamptz <= $7
					ELSE TRUE
				END
			ORDER BY a.id ASC
			FOR UPDATE OF a SKIP LOCKED
		), claimed AS (
			UPDATE accounts a
			SET rate_limited_at = COALESCE(a.rate_limited_at, $7),
				rate_limit_reset_at = GREATEST(COALESCE(a.rate_limit_reset_at, $10), $10),
				extra = COALESCE(a.extra, '{}'::jsonb) || jsonb_build_object(
					$2::text, TRUE,
					$3::text, $9::text,
					$4::text, $8::text,
					$5::text, 'running',
					$6::text, $9::text
				),
				updated_at = NOW()
			FROM candidates c
			WHERE a.id = c.id
			RETURNING a.id
		), outboxed AS (
			INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)
			SELECT $11, c.id, NULL, NULL FROM claimed c
			RETURNING account_id
		)
		SELECT id FROM claimed ORDER BY id
	`,
		pq.Array(ids),
		service.GrokFreeRecoveryPendingExtraKey,
		service.GrokFreeRecoveryNextProbeAtExtraKey,
		service.GrokFreeRecoveryLastProbeAtExtraKey,
		service.GrokFreeRecoveryLastProbeResultExtraKey,
		service.GrokFreeProactiveNextProbeAtExtraKey,
		now.UTC(),
		now.UTC().Format(time.RFC3339Nano),
		nextProbeAt.UTC().Format(time.RFC3339Nano),
		leaseUntil.UTC(),
		service.SchedulerOutboxEventAccountChanged,
		grokRecoveryRFC3339Pattern,
	)
	if err != nil {
		return nil, fmt.Errorf("claim proactive Grok Free recovery candidates: %w", err)
	}
	claimedIDs, err := scanInt64Rows(rows)
	if err != nil {
		return nil, err
	}
	r.syncSchedulerAccountSnapshots(ctx, claimedIDs)
	return r.loadOrderedGrokRecoveryAccounts(ctx, claimedIDs)
}

// RecordGrokFreeRecoveryProbeResult records telemetry only for the currently
// claimed generation. A newer 429/claim changes next_probe_at and rejects a
// stale probe result.
func (r *accountRepository) RecordGrokFreeRecoveryProbeResult(
	ctx context.Context,
	id int64,
	expectedNextProbeAt time.Time,
	result string,
	completedAt time.Time,
) (bool, error) {
	res, err := r.sql.ExecContext(ctx, `
		UPDATE accounts
		SET extra = COALESCE(extra, '{}'::jsonb) || jsonb_build_object(
			$3::text, $5::text,
			$4::text, $6::text
		),
			updated_at = NOW()
		WHERE id = $1
			AND deleted_at IS NULL
			AND COALESCE(extra ->> $2, 'false') = 'true'
			AND extra ->> $7 = $8
	`,
		id,
		service.GrokFreeRecoveryPendingExtraKey,
		service.GrokFreeRecoveryLastProbeResultExtraKey,
		service.GrokFreeRecoveryLastResultAtExtraKey,
		result,
		completedAt.UTC().Format(time.RFC3339Nano),
		service.GrokFreeRecoveryNextProbeAtExtraKey,
		expectedNextProbeAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return false, fmt.Errorf("record Grok Free recovery probe result: %w", err)
	}
	affected, err := res.RowsAffected()
	return affected > 0, err
}

func scanInt64Rows(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
	Close() error
}) ([]int64, error) {
	defer func() { _ = rows.Close() }()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *accountRepository) loadOrderedGrokRecoveryAccounts(ctx context.Context, ids []int64) ([]service.Account, error) {
	if len(ids) == 0 {
		return []service.Account{}, nil
	}
	accounts, err := r.GetByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]service.Account, len(accounts))
	for _, account := range accounts {
		if account != nil {
			byID[account.ID] = *account
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := make([]service.Account, 0, len(ids))
	for _, id := range ids {
		if account, ok := byID[id]; ok {
			out = append(out, account)
		}
	}
	return out, nil
}
