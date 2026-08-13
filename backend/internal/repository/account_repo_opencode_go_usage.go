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

// ListOpenCodeGoUsageGroupAccounts resolves every sibling for all supplied
// identities with one ID query and one batch hydration. API keys are query
// parameters only; no derived shared key is persisted.
func (r *accountRepository) ListOpenCodeGoUsageGroupAccounts(ctx context.Context, accounts []*service.Account) ([]service.Account, error) {
	if r == nil || r.sql == nil {
		return nil, service.ErrOpenCodeGoUsageUnavailable
	}
	keys := make([]string, 0, len(accounts))
	seen := make(map[string]struct{}, len(accounts))
	for _, account := range accounts {
		if !service.IsOpenCodeGoUsageAccount(account) || account.Credentials == nil {
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
			AND `+opencodeGoUsageEligibleSQL+`
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

// SetOpenCodeGoUsageAutoRefresh persists the group-scoped auto-refresh switch.
func (r *accountRepository) SetOpenCodeGoUsageAutoRefresh(ctx context.Context, account *service.Account, enabled bool) error {
	if account == nil {
		return service.ErrAccountNilInput
	}
	if r == nil || r.client == nil || !service.IsOpenCodeGoUsageAccount(account) {
		return service.ErrOpenCodeGoUsageUnavailable
	}
	return r.updateOpenCodeGoUsageGroup(ctx, account, map[string]any{
		service.OpenCodeGoUsageAutoRefreshExtraKey: enabled,
	}, nil, true)
}

// UpdateOpenCodeGoUsageSnapshot persists the group-scoped usage snapshot.
func (r *accountRepository) UpdateOpenCodeGoUsageSnapshot(ctx context.Context, account *service.Account, snapshot *service.OpenCodeGoUsageSnapshot) error {
	if account == nil || snapshot == nil {
		return service.ErrAccountNilInput
	}
	if r == nil || r.client == nil || !service.IsOpenCodeGoUsageAccount(account) {
		return service.ErrOpenCodeGoUsageUnavailable
	}
	payload := openCodeGoUsageManagedPayload(account)
	payload[service.OpenCodeGoUsageSnapshotExtraKey] = snapshot
	return r.updateOpenCodeGoUsageGroup(ctx, account, payload, nil, true)
}

// DisableOpenCodeGoUsageAutoRefresh is group-scoped and retains the loaded
// identity CAS. It cannot disable a new group after the account changes key.
func (r *accountRepository) DisableOpenCodeGoUsageAutoRefresh(ctx context.Context, account *service.Account) error {
	if account == nil {
		return service.ErrAccountNilInput
	}
	if r == nil || r.client == nil || !service.IsOpenCodeGoUsageAccount(account) {
		return service.ErrOpenCodeGoUsageUnavailable
	}
	payload := openCodeGoUsageManagedPayload(account)
	payload[service.OpenCodeGoUsageAutoRefreshExtraKey] = false
	delete(payload, service.OpenCodeGoUsageSnapshotExtraKey)
	return r.updateOpenCodeGoUsageGroup(ctx, account, payload,
		[]string{service.OpenCodeGoUsageSnapshotExtraKey}, true)
}

func openCodeGoUsageManagedPayload(account *service.Account) map[string]any {
	payload := make(map[string]any, 2)
	if account == nil || account.Extra == nil {
		return payload
	}
	for _, key := range []string{
		service.OpenCodeGoUsageAutoRefreshExtraKey,
		service.OpenCodeGoUsageSnapshotExtraKey,
	} {
		if value, ok := account.Extra[key]; ok {
			payload[key] = value
		}
	}
	return payload
}

type lockedOpenCodeGoUsageMember struct {
	id            int64
	anchorMatches bool
	autoJSON      string
	snapshotJSON  string
}

// updateOpenCodeGoUsageGroup locks every member of the exact api_key group and
// merges the payload onto each member's extra. The merge is pure
// (COALESCE(extra,'{}') || payload) unless deleteKeys is non-empty, in which
// case those keys are removed first — used only by the group-level disable path
// to clear the stale snapshot. Managed keys are never removed on ordinary
// writes, so a snapshot write can never wipe the auto-refresh switch or vice versa.
func (r *accountRepository) updateOpenCodeGoUsageGroup(
	ctx context.Context,
	account *service.Account,
	payload map[string]any,
	deleteKeys []string,
	requireExpectedState bool,
) error {
	if account == nil {
		return service.ErrAccountNilInput
	}
	if r == nil || r.client == nil || !service.IsOpenCodeGoUsageAccount(account) {
		return service.ErrOpenCodeGoUsageUnavailable
	}
	apiKey, ok := account.Credentials["api_key"].(string)
	if !ok || apiKey == "" {
		return service.ErrOpenCodeGoUsageAccountInvalid
	}
	apply := func(txCtx context.Context, client *dbent.Client) error {
		matchesProxy, err := lockAndMatchProbeProxyIdentity(txCtx, client, account)
		if err != nil {
			return err
		}
		if !matchesProxy {
			return service.ErrOpenCodeGoUsageIdentityChanged
		}
		members, err := lockOpenCodeGoUsageGroup(txCtx, client, account, apiKey)
		if err != nil {
			return err
		}
		anchorMatches := false
		for _, member := range members {
			anchorMatches = anchorMatches || member.anchorMatches
		}
		if !anchorMatches {
			return service.ErrOpenCodeGoUsageIdentityChanged
		}
		if requireExpectedState {
			expectedAuto, err := canonicalAccountExtraJSON(account, service.OpenCodeGoUsageAutoRefreshExtraKey)
			if err != nil {
				return err
			}
			expectedSnapshot, err := canonicalAccountExtraJSON(account, service.OpenCodeGoUsageSnapshotExtraKey)
			if err != nil {
				return err
			}
			stateMatches := false
			for _, member := range members {
				if canonicalJSON(member.autoJSON) == expectedAuto &&
					canonicalJSON(member.snapshotJSON) == expectedSnapshot {
					stateMatches = true
					break
				}
			}
			if !stateMatches {
				return service.ErrOpenCodeGoUsageIdentityChanged
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
		mergeExpr := "COALESCE(extra, '{}'::jsonb)"
		for _, key := range deleteKeys {
			mergeExpr += " - " + pq.QuoteLiteral(key)
		}
		mergeExpr += " || $1::jsonb"
		result, err := client.ExecContext(txCtx, `
			UPDATE accounts
			SET extra = `+mergeExpr+`,
				updated_at = NOW()
			WHERE deleted_at IS NULL
				AND `+opencodeGoUsageEligibleSQL+`
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

func lockOpenCodeGoUsageGroup(
	ctx context.Context,
	client *dbent.Client,
	account *service.Account,
	apiKey string,
) ([]lockedOpenCodeGoUsageMember, error) {
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
			COALESCE((extra -> 'opencode_go_usage_auto_refresh')::text, 'null'),
			COALESCE((extra -> 'opencode_go_usage_snapshot')::text, 'null')
		FROM accounts
		WHERE deleted_at IS NULL
			AND `+opencodeGoUsageEligibleSQL+`
			AND credentials ->> 'api_key' = $1
		ORDER BY id
		FOR NO KEY UPDATE
	`, apiKey, account.ID, account.Platform, account.Type, string(credentials), proxyID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	members := make([]lockedOpenCodeGoUsageMember, 0, 1)
	for rows.Next() {
		var member lockedOpenCodeGoUsageMember
		if err := rows.Scan(&member.id, &member.anchorMatches, &member.autoJSON, &member.snapshotJSON); err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return nil, service.ErrOpenCodeGoUsageIdentityChanged
	}
	return members, nil
}

// ListDueOpenCodeGoUsageAccounts returns at most one truly-due activity-driven
// candidate per exact API key. Due timing (debounce, max-wait, failure backoff)
// is evaluated in SQL before LIMIT so non-due active groups cannot starve due ones.
// Account.LastUsedAt is stamped with the group MAX(last_used_at) for a service
// pure-function recheck against races between list and refresh.
//
// Rules mirror service.openCodeGoUsageAutoRefreshDueAt (keep both in sync):
//   - missing/invalid snapshot or times → fail-open first due
//   - success: activity after fetched_at;
//     due_at = GREATEST(LEAST(last_used+debounce, fetched+maxWait), fetched+minFetchInterval)
//   - failed/unauthorized: activity after last_attempt; activity_due = LEAST(...);
//     final due_at is not earlier than a valid next_refresh_at (invalid/missing fail-open)
func (r *accountRepository) ListDueOpenCodeGoUsageAccounts(
	ctx context.Context,
	now time.Time,
	debounce, maxWait time.Duration,
	limit int,
) ([]service.Account, error) {
	if limit <= 0 {
		return []service.Account{}, nil
	}
	if r == nil || r.sql == nil {
		return nil, errors.New("account repository SQL executor not configured")
	}
	if debounce <= 0 {
		debounce = time.Minute
	}
	if maxWait <= 0 {
		maxWait = 15 * time.Minute
	}
	debounceSeconds := debounce.Seconds()
	maxWaitSeconds := maxWait.Seconds()
	minFetchIntervalSeconds := service.OpenCodeGoUsageMinFetchInterval.Seconds()
	rows, err := r.sql.QueryContext(ctx, `
		WITH eligible AS (
			SELECT id,
				credentials ->> 'api_key' AS api_key,
				last_used_at,
				extra -> 'opencode_go_usage_snapshot' AS snapshot
			FROM accounts
			WHERE deleted_at IS NULL
				AND status = 'active'
				AND `+opencodeGoUsageEligibleSQL+`
				AND extra @> '{"opencode_go_usage_auto_refresh": true}'::jsonb
		), group_activity AS (
			SELECT credentials ->> 'api_key' AS api_key,
				MAX(last_used_at) AS group_last_used_at
			FROM accounts
			WHERE deleted_at IS NULL
				AND `+opencodeGoUsageEligibleSQL+`
				AND jsonb_typeof(credentials -> 'api_key') = 'string'
			GROUP BY credentials ->> 'api_key'
		), joined AS (
			SELECT e.id, e.api_key, e.snapshot, g.group_last_used_at,
				e.snapshot #>> '{status}' AS status,
				e.snapshot #>> '{fetched_at}' AS fetched_at,
				e.snapshot #>> '{last_attempt_at}' AS last_attempt_at,
				e.snapshot #>> '{next_refresh_at}' AS next_refresh_at
			FROM eligible e
			JOIN group_activity g ON g.api_key = e.api_key
		), parsed AS MATERIALIZED (
			SELECT id, api_key, snapshot, group_last_used_at, status,
				`+ollamaCloudUsageParseRFC3339SQL("fetched_at")+` AS parsed_fetched_at,
				`+ollamaCloudUsageParseRFC3339SQL("last_attempt_at")+` AS parsed_last_attempt_at,
				`+ollamaCloudUsageParseRFC3339SQL("next_refresh_at")+` AS parsed_next_refresh_at
			FROM joined
		), timed AS (
			SELECT *,
				CASE
					WHEN status = 'ok'
						AND parsed_fetched_at IS NOT NULL
						AND group_last_used_at IS NOT NULL
						AND group_last_used_at > parsed_fetched_at::timestamptz
					THEN GREATEST(
						LEAST(
							group_last_used_at + make_interval(secs => $2::double precision),
							parsed_fetched_at::timestamptz + make_interval(secs => $3::double precision)
						),
						parsed_fetched_at::timestamptz + make_interval(secs => $5::double precision)
					)
					WHEN status IN ('failed', 'unauthorized')
						AND parsed_last_attempt_at IS NOT NULL
						AND group_last_used_at IS NOT NULL
						AND group_last_used_at > parsed_last_attempt_at::timestamptz
					THEN GREATEST(
						LEAST(
							group_last_used_at + make_interval(secs => $2::double precision),
							parsed_last_attempt_at::timestamptz + make_interval(secs => $3::double precision)
						),
						COALESCE(parsed_next_refresh_at::timestamptz, '-infinity'::timestamptz)
					)
					ELSE NULL
				END AS activity_due_at
			FROM parsed
		), candidates AS (
			SELECT *,
				CASE
					WHEN snapshot IS NULL OR snapshot = 'null'::jsonb OR status IS NULL
						OR status NOT IN ('ok', 'failed', 'unauthorized') THEN 0
					WHEN status = 'ok' AND parsed_fetched_at IS NULL THEN 0
					WHEN status IN ('failed', 'unauthorized') AND parsed_last_attempt_at IS NULL THEN 0
					WHEN activity_due_at IS NOT NULL AND $1 >= activity_due_at THEN 1
					ELSE NULL
				END AS due_class,
				activity_due_at AS due_at
			FROM timed
		), ranked AS (
			SELECT id, api_key, group_last_used_at, due_class, due_at,
				row_number() OVER (
					PARTITION BY api_key
					ORDER BY due_class,
						due_at NULLS FIRST,
						id
				) AS group_rank
			FROM candidates
			WHERE due_class IS NOT NULL
		)
		SELECT id, group_last_used_at
		FROM ranked
		WHERE group_rank = 1
		ORDER BY due_class, due_at NULLS FIRST, id
		LIMIT $4
	`, now.UTC(), debounceSeconds, maxWaitSeconds, limit, minFetchIntervalSeconds)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	type dueRow struct {
		id            int64
		groupLastUsed *time.Time
	}
	rowsOut := make([]dueRow, 0, limit)
	ids := make([]int64, 0, limit)
	for rows.Next() {
		var row dueRow
		if err := rows.Scan(&row.id, &row.groupLastUsed); err != nil {
			return nil, err
		}
		rowsOut = append(rowsOut, row)
		ids = append(ids, row.id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	accounts, err := r.GetByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]*service.Account, len(accounts))
	for _, account := range accounts {
		if account != nil {
			byID[account.ID] = account
		}
	}
	result := make([]service.Account, 0, len(rowsOut))
	for _, row := range rowsOut {
		account := byID[row.id]
		if account == nil {
			continue
		}
		// Stamp group MAX(last_used_at) for service due evaluation.
		if row.groupLastUsed != nil {
			ts := row.groupLastUsed.UTC()
			account.LastUsedAt = &ts
		} else {
			account.LastUsedAt = nil
		}
		result = append(result, *account)
	}
	return result, nil
}
