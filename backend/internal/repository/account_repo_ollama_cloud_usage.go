package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

const (
	ollamaCloudBaseURLRegexSQL       = `^[hH][tT][tT][pP][sS]://([wW][wW][wW]\.)?[oO][lL][lL][aA][mM][aA]\.[cC][oO][mM](:443)?(/v1)?$`
	ollamaCloudBaseURLMatchSQLPrefix = "btrim("
	ollamaCloudBaseURLMatchSQLSuffix = ") ~ '" + ollamaCloudBaseURLRegexSQL + "'"
	ollamaCloudUsageEligibleSQL      = `
	platform IN ('openai', 'anthropic')
	AND type = 'apikey'
	AND ` + ollamaCloudBaseURLMatchSQLPrefix + `credentials ->> 'base_url'` + ollamaCloudBaseURLMatchSQLSuffix + `
	AND jsonb_typeof(credentials -> 'api_key') = 'string'
`
)

func ollamaCloudBaseURLMatchesSQL(expression string) string {
	return ollamaCloudBaseURLMatchSQLPrefix + expression + ollamaCloudBaseURLMatchSQLSuffix
}

// ListOllamaCloudUsageGroupAccounts resolves every sibling for all supplied
// identities with one ID query and one batch hydration. API keys are query
// parameters only; no derived shared key is persisted.
func (r *accountRepository) ListOllamaCloudUsageGroupAccounts(ctx context.Context, accounts []*service.Account) ([]service.Account, error) {
	if r == nil || r.sql == nil {
		return nil, service.ErrOllamaCloudUsageUnavailable
	}
	keys := make([]string, 0, len(accounts))
	seen := make(map[string]struct{}, len(accounts))
	for _, account := range accounts {
		if !service.IsOllamaCloudUsageAccount(account) || account.Credentials == nil {
			continue
		}
		apiKey, ok := account.Credentials["api_key"].(string)
		if !ok || apiKey == "" {
			continue
		}
		if _, duplicate := seen[apiKey]; duplicate {
			continue
		}
		seen[apiKey] = struct{}{}
		keys = append(keys, apiKey)
	}
	if len(keys) == 0 {
		return []service.Account{}, nil
	}
	rows, err := r.sql.QueryContext(ctx, `
		SELECT id
		FROM accounts
		WHERE deleted_at IS NULL
			AND `+ollamaCloudUsageEligibleSQL+`
			AND credentials ->> 'api_key' = ANY($1)
		ORDER BY id
	`, pq.Array(keys))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	ids := make([]int64, 0, len(keys))
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

func (r *accountRepository) SaveOllamaCloudUsageSession(ctx context.Context, account *service.Account, ciphertext string, autoRefresh bool) error {
	return r.updateOllamaCloudUsageGroup(ctx, account, map[string]any{
		service.OllamaCloudUsageSessionExtraKey:     ciphertext,
		service.OllamaCloudUsageAutoRefreshExtraKey: autoRefresh,
	}, false)
}

func (r *accountRepository) DeleteOllamaCloudUsageSession(ctx context.Context, account *service.Account) error {
	return r.updateOllamaCloudUsageGroup(ctx, account, map[string]any{}, false)
}

func (r *accountRepository) SetOllamaCloudUsageAutoRefresh(ctx context.Context, account *service.Account, enabled bool) error {
	if !ollamaCloudUsageAccountHasSession(account) {
		return service.ErrOllamaCloudUsageSessionRequired
	}
	payload := ollamaCloudUsageManagedPayload(account)
	payload[service.OllamaCloudUsageAutoRefreshExtraKey] = enabled
	return r.updateOllamaCloudUsageGroup(ctx, account, payload, true)
}

func (r *accountRepository) UpdateOllamaCloudUsageSnapshot(ctx context.Context, account *service.Account, snapshot *service.OllamaCloudUsageSnapshot) error {
	if account == nil || snapshot == nil {
		return service.ErrAccountNilInput
	}
	if !ollamaCloudUsageAccountHasSession(account) {
		return service.ErrOllamaCloudUsageSessionRequired
	}
	payload := ollamaCloudUsageManagedPayload(account)
	payload[service.OllamaCloudUsageSnapshotExtraKey] = snapshot
	return r.updateOllamaCloudUsageGroup(ctx, account, payload, true)
}

// DisableOllamaCloudUsageAutoRefresh is group-scoped and retains the loaded
// identity CAS. It cannot disable a new group after the account changes key.
func (r *accountRepository) DisableOllamaCloudUsageAutoRefresh(ctx context.Context, account *service.Account) error {
	if !ollamaCloudUsageAccountHasSession(account) {
		return service.ErrOllamaCloudUsageSessionRequired
	}
	payload := ollamaCloudUsageManagedPayload(account)
	payload[service.OllamaCloudUsageAutoRefreshExtraKey] = false
	delete(payload, service.OllamaCloudUsageSnapshotExtraKey)
	return r.updateOllamaCloudUsageGroup(ctx, account, payload, true)
}

func ollamaCloudUsageManagedPayload(account *service.Account) map[string]any {
	payload := make(map[string]any, 3)
	if account == nil || account.Extra == nil {
		return payload
	}
	for _, key := range []string{
		service.OllamaCloudUsageSessionExtraKey,
		service.OllamaCloudUsageAutoRefreshExtraKey,
		service.OllamaCloudUsageSnapshotExtraKey,
	} {
		if value, ok := account.Extra[key]; ok {
			payload[key] = value
		}
	}
	return payload
}

func ollamaCloudUsageAccountHasSession(account *service.Account) bool {
	if account == nil || account.Extra == nil {
		return false
	}
	value, ok := account.Extra[service.OllamaCloudUsageSessionExtraKey].(string)
	return ok && value != ""
}

type lockedOllamaCloudUsageMember struct {
	id            int64
	anchorMatches bool
	sessionJSON   string
	autoJSON      string
	snapshotJSON  string
}

func (r *accountRepository) updateOllamaCloudUsageGroup(
	ctx context.Context,
	account *service.Account,
	payload map[string]any,
	requireExpectedState bool,
) error {
	if account == nil {
		return service.ErrAccountNilInput
	}
	if r == nil || r.client == nil || !service.IsOllamaCloudUsageAccount(account) {
		return service.ErrOllamaCloudUsageUnavailable
	}
	apiKey, ok := account.Credentials["api_key"].(string)
	if !ok || apiKey == "" {
		return service.ErrOllamaCloudUsageAccountInvalid
	}
	apply := func(txCtx context.Context, client *dbent.Client) error {
		matchesProxy, err := lockAndMatchProbeProxyIdentity(txCtx, client, account)
		if err != nil {
			return err
		}
		if !matchesProxy {
			return service.ErrOllamaCloudUsageIdentityChanged
		}
		members, err := lockOllamaCloudUsageGroup(txCtx, client, account, apiKey)
		if err != nil {
			return err
		}
		anchorMatches := false
		for _, member := range members {
			anchorMatches = anchorMatches || member.anchorMatches
		}
		if !anchorMatches {
			return service.ErrOllamaCloudUsageIdentityChanged
		}
		if requireExpectedState {
			expectedSession, err := canonicalAccountExtraJSON(account, service.OllamaCloudUsageSessionExtraKey)
			if err != nil {
				return err
			}
			expectedAuto, err := canonicalAccountExtraJSON(account, service.OllamaCloudUsageAutoRefreshExtraKey)
			if err != nil {
				return err
			}
			expectedSnapshot, err := canonicalAccountExtraJSON(account, service.OllamaCloudUsageSnapshotExtraKey)
			if err != nil {
				return err
			}
			stateMatches := false
			for _, member := range members {
				if canonicalJSON(member.sessionJSON) == expectedSession &&
					canonicalJSON(member.autoJSON) == expectedAuto &&
					canonicalJSON(member.snapshotJSON) == expectedSnapshot {
					stateMatches = true
					break
				}
			}
			if !stateMatches {
				return service.ErrOllamaCloudUsageIdentityChanged
			}
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		memberIDs := make([]int64, len(members))
		for index := range members {
			memberIDs[index] = members[index].id
		}
		result, err := client.ExecContext(txCtx, `
			UPDATE accounts
			SET extra = (COALESCE(extra, '{}'::jsonb)
					- 'ollama_cloud_usage_session'
					- 'ollama_cloud_usage_auto_refresh'
					- 'ollama_cloud_usage_snapshot') || $1::jsonb,
				updated_at = NOW()
			WHERE deleted_at IS NULL
				AND `+ollamaCloudUsageEligibleSQL+`
				AND credentials ->> 'api_key' = $2
				AND id = ANY($3)
		`, string(encoded), apiKey, pq.Array(memberIDs))
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected != int64(len(members)) {
			return service.ErrOllamaCloudUsageIdentityChanged
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

func lockOllamaCloudUsageGroup(
	ctx context.Context,
	client *dbent.Client,
	account *service.Account,
	apiKey string,
) ([]lockedOllamaCloudUsageMember, error) {
	credentials, err := json.Marshal(normalizeJSONMap(account.Credentials))
	if err != nil {
		return nil, err
	}
	var proxyID any
	if account.ProxyID != nil {
		proxyID = *account.ProxyID
	}
	rows, err := client.QueryContext(ctx, `
		SELECT
			id,
			id = $2
				AND platform = $3
				AND type = $4
				AND credentials = $5::jsonb
				AND proxy_id IS NOT DISTINCT FROM $6,
			COALESCE((extra -> 'ollama_cloud_usage_session')::text, 'null'),
			COALESCE((extra -> 'ollama_cloud_usage_auto_refresh')::text, 'null'),
			COALESCE((extra -> 'ollama_cloud_usage_snapshot')::text, 'null')
		FROM accounts
		WHERE deleted_at IS NULL
			AND `+ollamaCloudUsageEligibleSQL+`
			AND credentials ->> 'api_key' = $1
		ORDER BY id
		FOR NO KEY UPDATE
	`, apiKey, account.ID, account.Platform, account.Type, string(credentials), proxyID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	members := make([]lockedOllamaCloudUsageMember, 0, 1)
	for rows.Next() {
		var member lockedOllamaCloudUsageMember
		if err := rows.Scan(&member.id, &member.anchorMatches, &member.sessionJSON, &member.autoJSON, &member.snapshotJSON); err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return nil, service.ErrOllamaCloudUsageIdentityChanged
	}
	return members, nil
}

func canonicalAccountExtraJSON(account *service.Account, key string) (string, error) {
	var value any
	if account != nil && account.Extra != nil {
		value = account.Extra[key]
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return canonicalJSON(string(raw)), nil
}

func canonicalJSON(raw string) string {
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return ""
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(encoded)
}

// ListDueOllamaCloudUsageAccounts returns at most one due representative per
// exact API key before hydration, preventing one shared group from consuming a
// whole runner cycle.
func (r *accountRepository) ListDueOllamaCloudUsageAccounts(ctx context.Context, now time.Time, limit int) ([]service.Account, error) {
	if limit <= 0 {
		return []service.Account{}, nil
	}
	if r == nil || r.sql == nil {
		return nil, errors.New("account repository SQL executor not configured")
	}
	rows, err := r.sql.QueryContext(ctx, `
		WITH candidates AS (
			SELECT id, credentials ->> 'api_key' AS api_key,
				extra #>> '{ollama_cloud_usage_snapshot,next_refresh_at}' AS next_refresh_at
			FROM accounts
			WHERE deleted_at IS NULL
				AND status = 'active'
				AND `+ollamaCloudUsageEligibleSQL+`
				AND jsonb_typeof(extra -> 'ollama_cloud_usage_session') = 'string'
				AND extra @> '{"ollama_cloud_usage_auto_refresh": true}'::jsonb
		), parsed AS MATERIALIZED (
			SELECT id, api_key, next_refresh_at,
				next_refresh_at ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})$' AS rfc3339_shape,
				jsonb_path_query_first_tz(
					to_jsonb(regexp_replace(
						next_refresh_at,
						'(\.[0-9]{6})[0-9]+(Z|[+-][0-9]{2}:[0-9]{2})$',
						'\1\2'
					)),
					'$.datetime()', '{}'::jsonb, true
				) #>> '{}' AS parsed_next_refresh_at
			FROM candidates
		), due AS (
			SELECT *,
				CASE WHEN next_refresh_at IS NULL OR NOT rfc3339_shape OR parsed_next_refresh_at IS NULL THEN 0 ELSE 1 END AS due_class
			FROM parsed
			WHERE next_refresh_at IS NULL
				OR NOT rfc3339_shape
				OR parsed_next_refresh_at IS NULL
				OR parsed_next_refresh_at::timestamptz <= $1
		), ranked AS (
			SELECT *, row_number() OVER (
				PARTITION BY api_key
				ORDER BY due_class,
					CASE WHEN rfc3339_shape AND parsed_next_refresh_at IS NOT NULL THEN parsed_next_refresh_at::timestamptz END NULLS FIRST,
					id
			) AS group_rank
			FROM due
		)
		SELECT id
		FROM ranked
		WHERE group_rank = 1
		ORDER BY due_class,
			CASE WHEN rfc3339_shape AND parsed_next_refresh_at IS NOT NULL THEN parsed_next_refresh_at::timestamptz END NULLS FIRST,
			id
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
	accounts, err := r.GetByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	result := make([]service.Account, 0, len(accounts))
	for _, account := range accounts {
		if account != nil {
			result = append(result, *account)
		}
	}
	return result, nil
}
