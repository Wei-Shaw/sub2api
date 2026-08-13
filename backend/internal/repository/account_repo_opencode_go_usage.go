package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

const (
	opencodeGoBaseURLRegexSQL       = `^[hH][tT][tT][pP][sS]://[oO][pP][eE][nN][cC][oO][dD][eE]\.[aA][iI]/[zZ][eE][nN]/[gG][oO]/[vV]1/?$`
	opencodeGoBaseURLMatchSQLPrefix = "btrim("
	opencodeGoBaseURLMatchSQLSuffix = ") ~ '" + opencodeGoBaseURLRegexSQL + "'"
	opencodeGoUsageEligibleSQL      = `
	platform = 'openai'
	AND type = 'apikey'
	AND ` + opencodeGoBaseURLMatchSQLPrefix + `credentials ->> 'base_url'` + opencodeGoBaseURLMatchSQLSuffix + `
	AND jsonb_typeof(credentials -> 'api_key') = 'string'
`
)

// SetOpenCodeGoUsageAutoRefresh persists the per-account auto-refresh switch.
func (r *accountRepository) SetOpenCodeGoUsageAutoRefresh(ctx context.Context, account *service.Account, enabled bool) error {
	if account == nil {
		return service.ErrAccountNilInput
	}
	if r == nil || r.client == nil || !service.IsOpenCodeGoUsageAccount(account) {
		return service.ErrOpenCodeGoUsageUnavailable
	}
	return r.updateOpenCodeGoUsageExtra(ctx, account, map[string]any{
		service.OpenCodeGoUsageAutoRefreshExtraKey: enabled,
	})
}

// UpdateOpenCodeGoUsageSnapshot persists the per-account usage snapshot.
func (r *accountRepository) UpdateOpenCodeGoUsageSnapshot(ctx context.Context, account *service.Account, snapshot *service.OpenCodeGoUsageSnapshot) error {
	if account == nil || snapshot == nil {
		return service.ErrAccountNilInput
	}
	if r == nil || r.client == nil || !service.IsOpenCodeGoUsageAccount(account) {
		return service.ErrOpenCodeGoUsageUnavailable
	}
	return r.updateOpenCodeGoUsageExtra(ctx, account, map[string]any{
		service.OpenCodeGoUsageSnapshotExtraKey: snapshot,
	})
}

// updateOpenCodeGoUsageExtra atomically merges managed extra keys onto the
// account row. Snapshots are written per account (no api_key group CAS).
func (r *accountRepository) updateOpenCodeGoUsageExtra(ctx context.Context, account *service.Account, payload map[string]any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	apply := func(txCtx context.Context, client *dbent.Client) error {
		result, err := client.ExecContext(txCtx, `
			UPDATE accounts
			SET extra = COALESCE(extra, '{}'::jsonb) || $1::jsonb,
				updated_at = NOW()
			WHERE deleted_at IS NULL
				AND `+opencodeGoUsageEligibleSQL+`
				AND id = $2
		`, string(encoded), account.ID)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected != 1 {
			return service.ErrOpenCodeGoUsageIdentityChanged
		}
		return nil
	}
	if dbent.TxFromContext(ctx) != nil {
		return apply(ctx, clientFromContext(ctx, r.client))
	}
	tx, err := r.client.Tx(ctx)
	if errors.Is(err, dbent.ErrTxStarted) {
		return apply(ctx, r.client)
	}
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	if err := apply(txCtx, tx.Client()); err != nil {
		return err
	}
	return tx.Commit()
}

// ListDueOpenCodeGoUsageAccounts returns at most limit eligible accounts whose
// auto-refresh is enabled and whose snapshot is missing or due (next_refresh_at
// at or before now). Invalid/missing next_refresh_at values fail open to due.
func (r *accountRepository) ListDueOpenCodeGoUsageAccounts(ctx context.Context, now time.Time, limit int) ([]service.Account, error) {
	if limit <= 0 {
		return []service.Account{}, nil
	}
	if r == nil || r.sql == nil {
		return nil, errors.New("account repository SQL executor not configured")
	}
	nextRefreshExpr := "extra -> 'opencode_go_usage_snapshot' #>> '{next_refresh_at}'"
	rows, err := r.sql.QueryContext(ctx, `
		SELECT id
		FROM accounts
		WHERE deleted_at IS NULL
			AND status = 'active'
			AND `+opencodeGoUsageEligibleSQL+`
			AND extra @> '{"opencode_go_usage_auto_refresh": true}'::jsonb
			AND (
				extra -> 'opencode_go_usage_snapshot' IS NULL
				OR extra -> 'opencode_go_usage_snapshot' = 'null'::jsonb
				OR `+ollamaCloudUsageParseRFC3339SQL(nextRefreshExpr)+` IS NULL
				OR `+ollamaCloudUsageParseRFC3339SQL(nextRefreshExpr)+`::timestamptz <= $1
			)
		ORDER BY id
		LIMIT $2
	`, now.UTC(), limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	ids := make([]int64, 0, limit)
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
	hydrated, err := r.GetByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	result := make([]service.Account, 0, len(hydrated))
	for _, account := range hydrated {
		if account != nil {
			result = append(result, *account)
		}
	}
	return result, nil
}
