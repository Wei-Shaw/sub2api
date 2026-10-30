package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// stripOpenAIModelSyncState prevents ordinary writes from supplying ownership.
// Only CommitOpenAIModelSync may replace this server-managed state.
func stripOpenAIModelSyncState(extra map[string]any) map[string]any {
	if _, present := extra[service.OpenAIModelAutoSyncStateExtraKey]; !present {
		return extra
	}
	extra = copyJSONMap(extra)
	delete(extra, service.OpenAIModelAutoSyncStateExtraKey)
	return extra
}

// ListDueOpenAIModelSyncAccounts bounds hydration to the oldest due accounts.
// JSONB numbers are already valid PostgreSQL numerics; check integral value and
// int64 bounds before converting to the epoch representation used by the state.
func (r *accountRepository) ListDueOpenAIModelSyncAccounts(ctx context.Context, now time.Time, limit int) ([]service.Account, error) {
	if limit <= 0 {
		return []service.Account{}, nil
	}
	if scoped := r.accountTransactionScope(ctx); scoped != r {
		return scoped.ListDueOpenAIModelSyncAccounts(ctx, now, limit)
	}
	if r.sql == nil {
		return nil, errors.New("account repository SQL executor not configured")
	}
	rows, err := r.sql.QueryContext(ctx, `
		WITH candidates AS (
			SELECT id, extra #> '{openai_model_auto_sync,next_sync_unix}' AS next_sync_unix
			FROM accounts
			WHERE deleted_at IS NULL
				AND status = 'active'
				AND platform = 'openai'
				AND type IN ('oauth', 'apikey')
				AND parent_account_id IS NULL
				AND (expires_at IS NULL OR expires_at > $1)
				AND extra @> '{"openai_model_auto_sync_enabled": true}'::jsonb
		), numeric_times AS (
			SELECT id, CASE WHEN jsonb_typeof(next_sync_unix) = 'number'
				THEN (next_sync_unix #>> '{}')::numeric END AS next_sync_unix
			FROM candidates
		), due_times AS MATERIALIZED (
			SELECT id, CASE
				WHEN next_sync_unix BETWEEN -9223372036854775808::numeric AND 9223372036854775807::numeric
					AND next_sync_unix = trunc(next_sync_unix)
				THEN next_sync_unix::bigint END AS next_sync_unix
			FROM numeric_times
		)
		SELECT id FROM due_times
		WHERE next_sync_unix IS NULL OR next_sync_unix <= $2
		ORDER BY next_sync_unix ASC NULLS FIRST, id ASC
		LIMIT $3
	`, now, now.Unix(), limit)
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
	if len(ids) == 0 {
		return []service.Account{}, nil
	}
	accounts, err := r.GetByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]service.Account, 0, len(accounts))
	for _, account := range accounts {
		if account != nil {
			out = append(out, *account)
		}
	}
	return out, nil
}

// CommitOpenAIModelSync is called after the network observation completes. The
// transaction only checks the observation's identity and publishes its result.
func (r *accountRepository) CommitOpenAIModelSync(ctx context.Context, expected *service.Account, mapping map[string]string, state *service.OpenAIModelAutoSyncState) error {
	if expected == nil || state == nil {
		return service.ErrAccountNilInput
	}
	enabled, _ := expected.Extra[service.OpenAIModelAutoSyncEnabledExtraKey].(bool)
	if !enabled || expected.Platform != service.PlatformOpenAI ||
		(expected.Type != service.AccountTypeOAuth && expected.Type != service.AccountTypeAPIKey) || expected.IsShadow() {
		return service.ErrOpenAIModelAutoSyncChanged
	}
	if dbent.TxFromContext(ctx) != nil {
		return r.commitOpenAIModelSyncInTx(ctx, expected, mapping, state)
	}
	tx, err := r.client.Tx(ctx)
	if errors.Is(err, dbent.ErrTxStarted) {
		return r.commitOpenAIModelSyncInTx(ctx, expected, mapping, state)
	}
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.commitOpenAIModelSyncInTx(dbent.NewTxContext(ctx, tx), expected, mapping, state); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// Cache propagation happens only after commit. The durable outbox also
	// carries the change if this best-effort write fails.
	r.syncSchedulerAccountSnapshot(ctx, expected.ID)
	return nil
}

func (r *accountRepository) commitOpenAIModelSyncInTx(ctx context.Context, expected *service.Account, mapping map[string]string, state *service.OpenAIModelAutoSyncState) error {
	payload, err := json.Marshal(map[string]any{service.OpenAIModelAutoSyncStateExtraKey: state})
	if err != nil {
		return err
	}
	credentials, err := json.Marshal(expected.Credentials)
	if err != nil {
		return err
	}
	priorState, err := json.Marshal(expected.Extra[service.OpenAIModelAutoSyncStateExtraKey])
	if err != nil {
		return err
	}
	enabled, err := json.Marshal(expected.Extra[service.OpenAIModelAutoSyncEnabledExtraKey])
	if err != nil {
		return err
	}
	var mappingJSON any
	if mapping != nil {
		encoded, err := json.Marshal(mapping)
		if err != nil {
			return err
		}
		mappingJSON = string(encoded)
	}
	client := clientFromContext(ctx, r.client)
	proxyMatches, err := lockAndMatchProbeProxyIdentity(ctx, client, expected)
	if err != nil {
		return err
	}
	if !proxyMatches {
		return service.ErrOpenAIModelAutoSyncChanged
	}
	var proxyID any
	if expected.ProxyID != nil {
		proxyID = *expected.ProxyID
	}
	result, err := client.ExecContext(ctx, `
		UPDATE accounts
		SET credentials = CASE WHEN $9::jsonb IS NULL THEN credentials
				ELSE COALESCE(credentials, '{}'::jsonb) || jsonb_build_object('model_mapping', $9::jsonb) END,
			extra = COALESCE(extra, '{}'::jsonb) || $1::jsonb,
			updated_at = NOW()
		WHERE id = $2
			AND platform = $3
			AND type = $4
			AND credentials = $5::jsonb
			AND proxy_id IS NOT DISTINCT FROM $6
			AND COALESCE(extra -> 'openai_model_auto_sync', 'null'::jsonb) = $7::jsonb
			AND COALESCE(extra -> 'openai_model_auto_sync_enabled', 'null'::jsonb) = $8::jsonb
			AND extra @> '{"openai_model_auto_sync_enabled": true}'::jsonb
			AND status = 'active'
			AND (expires_at IS NULL OR expires_at > clock_timestamp())
			AND parent_account_id IS NULL
			AND deleted_at IS NULL
	`, string(payload), expected.ID, expected.Platform, expected.Type, string(credentials), proxyID, string(priorState), string(enabled), mappingJSON)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrOpenAIModelAutoSyncChanged
	}
	return enqueueSchedulerOutbox(ctx, client, service.SchedulerOutboxEventAccountChanged, &expected.ID, nil, nil)
}
