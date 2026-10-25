package repository

import (
	"context"
	"database/sql"
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
	// ollamaCloudUsagePlatformsSQL 是 service.isOllamaCloudUsagePlatform 的 SQL
	// 镜像：Ollama Cloud key 允许挂在 openai/anthropic 与国产 OpenAI 兼容平台
	// 下复用。所有平台白名单 SQL 只允许引用本常量，不得各处重写字面量，防止漂移。
	ollamaCloudUsagePlatformsSQL = "'openai', 'anthropic', 'kimi', 'zhipu', 'deepseek', 'minimax'"
	ollamaCloudUsageEligibleSQL  = `
	platform IN (` + ollamaCloudUsagePlatformsSQL + `)
	AND type = 'apikey'
	AND ` + ollamaCloudBaseURLMatchSQLPrefix + `credentials ->> 'base_url'` + ollamaCloudBaseURLMatchSQLSuffix + `
	AND jsonb_typeof(credentials -> 'api_key') = 'string'
`
)

func ollamaCloudBaseURLMatchesSQL(expression string) string {
	return ollamaCloudBaseURLMatchSQLPrefix + expression + ollamaCloudBaseURLMatchSQLSuffix
}

// ollamaCloudUsageEligibleSQLMySQL 是 ollamaCloudUsageEligibleSQL 的 MySQL 版，语义逐条
// 对应（见 DIALECT_MYSQL_GUIDE.md）：btrim(x) ~ 正则 → mysqlOllamaBaseURLMatchesSQL
// （REGEXP_LIKE(TRIM(x), 正则, 'c')，正则反斜杠已按 MySQL 字面量规则翻倍）；
// credentials ->> 'key' → mysqlJSONTextOf（缺失路径/JSON null 归一为 SQL NULL）；
// jsonb_typeof(credentials -> 'api_key') = 'string' → JSON_TYPE(...) = 'STRING'（大写）。
// 平台白名单 ollamaCloudUsagePlatformsSQL 仅含普通字面量，两侧共用。
var ollamaCloudUsageEligibleSQLMySQL = `
	platform IN (` + ollamaCloudUsagePlatformsSQL + `)
	AND type = 'apikey'
	AND ` + mysqlOllamaBaseURLMatchesSQL(mysqlJSONTextOf("credentials", "base_url")) + `
	AND JSON_TYPE(JSON_EXTRACT(credentials, '$.api_key')) = 'STRING'
`

// ListOllamaCloudUsageGroupAccounts resolves every sibling for all supplied
// identities with one ID query and one batch hydration. API keys are query
// parameters only; no derived shared key is persisted.
func (r *accountRepository) ListOllamaCloudUsageGroupAccounts(ctx context.Context, accounts []*service.Account) ([]service.Account, error) {
	if r == nil || r.sql == nil {
		return nil, service.ErrOllamaCloudUsageUnavailable
	}
	if IsMySQL() {
		return r.listOllamaCloudUsageGroupAccountsMySQL(ctx, accounts)
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
		var query string
		var args []any
		if IsMySQL() {
			query = ollamaCloudUsageGroupUpdateSQLMySQL(memberIDs)
			args = make([]any, 0, len(memberIDs)+2)
			args = append(args, string(encoded), apiKey)
			for _, id := range memberIDs {
				args = append(args, id)
			}
		} else {
			query = `
				UPDATE accounts
				SET extra = (COALESCE(extra, '{}'::jsonb)
						- 'ollama_cloud_usage_session'
						- 'ollama_cloud_usage_auto_refresh'
						- 'ollama_cloud_usage_snapshot') || $1::jsonb,
					updated_at = NOW()
				WHERE deleted_at IS NULL
					AND ` + ollamaCloudUsageEligibleSQL + `
					AND credentials ->> 'api_key' = $2
					AND id = ANY($3)
			`
			args = []any{string(encoded), apiKey, pq.Array(memberIDs)}
		}
		result, err := client.ExecContext(txCtx, query, args...)
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
	// anchorMatches 布尔列：MySQL 用 <=> 对齐 PG 的 IS NOT DISTINCT FROM；
	// (extra -> 'key')::text → CAST(JSON_EXTRACT(...) AS CHAR)（JSON 文本含引号，
	// 交由调用方 canonicalJSON 归一，两侧语义一致）。FOR NO KEY UPDATE → FOR UPDATE。
	var rows *sql.Rows
	if IsMySQL() {
		rows, err = client.QueryContext(ctx, `
			SELECT
				id,
				id = ?
					AND platform = ?
					AND type = ?
					AND credentials = CAST(? AS JSON)
					AND (proxy_id <=> ?),
				COALESCE(CAST(JSON_EXTRACT(extra, '$.ollama_cloud_usage_session') AS CHAR), 'null'),
				COALESCE(CAST(JSON_EXTRACT(extra, '$.ollama_cloud_usage_auto_refresh') AS CHAR), 'null'),
				COALESCE(CAST(JSON_EXTRACT(extra, '$.ollama_cloud_usage_snapshot') AS CHAR), 'null')
			FROM accounts
			WHERE deleted_at IS NULL
				AND `+ollamaCloudUsageEligibleSQLMySQL+`
				AND `+mysqlJSONTextOf("credentials", "api_key")+` = ?
			ORDER BY id
			FOR UPDATE
		`, account.ID, account.Platform, account.Type, string(credentials), proxyID, apiKey)
	} else {
		rows, err = client.QueryContext(ctx, `
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
	}
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

// ollamaCloudUsageParseRFC3339SQL reuses the verified RFC3339(/Nano) parse path
// for a snapshot timestamp expression. Invalid or missing values fail open to NULL.
//
// The value is rewritten twice before it reaches jsonpath:
//  1. Sub-second precision beyond 6 digits is truncated, because .datetime()
//     rejects more than microsecond resolution while Go emits 9 digits.
//  2. A trailing "Z" is rewritten to "+00:00". jsonpath .datetime() only learned
//     to accept the ISO-8601 "Z" designator in PostgreSQL 17, and every timestamp
//     this service writes is UTC (hence "Z"). Without this rewrite the parse
//     silently yields NULL on PostgreSQL <= 16, which makes every due column NULL
//     and collapses ListDueOllamaCloudUsageAccounts into its fail-open branch.
//
// jsonpath (rather than a direct ::timestamptz cast) is required so that values
// passing the shape regex but naming an impossible date (e.g. 2026-02-30) fail
// open to NULL instead of aborting the whole query.
func ollamaCloudUsageParseRFC3339SQL(expression string) string {
	return `CASE
		WHEN ` + expression + ` IS NULL THEN NULL
		WHEN ` + expression + ` ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})$'
			THEN jsonb_path_query_first_tz(
				to_jsonb(regexp_replace(
					regexp_replace(
						` + expression + `,
						'(\.[0-9]{6})[0-9]+(Z|[+-][0-9]{2}:[0-9]{2})$',
						'\1\2'
					),
					'Z$',
					'+00:00'
				)),
				'$.datetime()', '{}'::jsonb, true
			) #>> '{}'
		ELSE NULL
	END`
}

// ListDueOllamaCloudUsageAccounts returns at most one truly-due activity-driven
// candidate per exact API key. Due timing (debounce, max-wait, failure backoff)
// is evaluated in SQL before LIMIT so non-due active groups cannot starve due ones.
// Account.LastUsedAt is stamped with the group MAX(last_used_at) for a service
// pure-function recheck against races between list and refresh.
//
// Rules mirror service.ollamaCloudUsageAutoRefreshDueAt (keep both in sync):
//   - missing/invalid snapshot or times → fail-open first due
//   - success: activity after fetched_at;
//     due_at = GREATEST(LEAST(last_used+debounce, fetched+maxWait), fetched+minFetchInterval)
//   - failed/unauthorized: activity after last_attempt; activity_due = LEAST(...);
//     final due_at is not earlier than a valid next_refresh_at (invalid/missing fail-open)
func (r *accountRepository) ListDueOllamaCloudUsageAccounts(
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
	// debounce/maxWait 的兜底默认值置于方言分流之前，保证 MySQL 早退分支与
	// PG 分支拿到同样的兜底语义（服务层已归一为正值，此处仅为防御性兜底）。
	if debounce <= 0 {
		debounce = time.Minute
	}
	if maxWait <= 0 {
		maxWait = time.Hour
	}
	if IsMySQL() {
		return r.listDueOllamaCloudUsageAccountsMySQL(ctx, now, debounce, maxWait, limit)
	}
	debounceSeconds := debounce.Seconds()
	maxWaitSeconds := maxWait.Seconds()
	minFetchIntervalSeconds := service.OllamaCloudUsageMinFetchInterval.Seconds()
	rows, err := r.sql.QueryContext(ctx, `
		WITH eligible AS (
			SELECT id,
				credentials ->> 'api_key' AS api_key,
				last_used_at,
				extra -> 'ollama_cloud_usage_snapshot' AS snapshot
			FROM accounts
			WHERE deleted_at IS NULL
				AND status = 'active'
				AND `+ollamaCloudUsageEligibleSQL+`
				AND jsonb_typeof(extra -> 'ollama_cloud_usage_session') = 'string'
				AND extra @> '{"ollama_cloud_usage_auto_refresh": true}'::jsonb
		), group_activity AS (
			SELECT credentials ->> 'api_key' AS api_key,
				MAX(last_used_at) AS group_last_used_at
			FROM accounts
			WHERE deleted_at IS NULL
				AND `+ollamaCloudUsageEligibleSQL+`
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

// ---------------------------------------------------------------------------
// MySQL 方言实现（见 DIALECT_MYSQL_GUIDE.md）。语义与各 PG 版本逐条对应：
// `->>`/#>> → mysqlJSONTextOf / JSON_UNQUOTE(JSON_EXTRACT(...))；`= ANY($1)` → IN 展开；
// `@>` → JSON_CONTAINS；`FOR NO KEY UPDATE` → `FOR UPDATE`；NOW() → NOW(3)。
// ---------------------------------------------------------------------------

// ollamaCloudUsageGroupUpdateSQLMySQL 构造 ollama 组内成员 extra 合并的 MySQL UPDATE，
// 对应 PG 的 `(COALESCE(extra,'{}'::jsonb) - k1 - k2 - k3) || $1::jsonb`：
// JSON_MERGE_PATCH(JSON_REMOVE(COALESCE(extra, CAST('{}' AS JSON)), 路径...), CAST(? AS JSON))。
// 三个 managed key 先被 JSON_REMOVE 移除，patch 键与 base 无交集，JSON_MERGE_PATCH 的
// 递归对象合并不会触发，与 PG `||` 的整键替换语义等价（payload 值均为标量/整对象覆盖）。
// id = ANY($3) → IN 展开（占位符数 = len(memberIDs)，由调用方按序绑定）。
// REVIEW: payload 值若为 JSON null（理论上仅当 account.Extra 中 managed key 本身为 null
// 时经 managedPayload 复制产生），JSON_MERGE_PATCH 会删除该键，而 PG `||` 会将其置为
// JSON null；两种状态在下游判定（due 列表的 snapshot IS NULL / = 'null'、CAS 的
// canonicalJSON('null')）行为一致，故接受该表示差异。
func ollamaCloudUsageGroupUpdateSQLMySQL(memberIDs []int64) string {
	return `
		UPDATE accounts
		SET extra = JSON_MERGE_PATCH(
				JSON_REMOVE(
					COALESCE(extra, CAST('{}' AS JSON)),
					'$.ollama_cloud_usage_session',
					'$.ollama_cloud_usage_auto_refresh',
					'$.ollama_cloud_usage_snapshot'
				),
				CAST(? AS JSON)
			),
			updated_at = NOW(3)
		WHERE deleted_at IS NULL
			AND ` + ollamaCloudUsageEligibleSQLMySQL + `
			AND ` + mysqlJSONTextOf("credentials", "api_key") + ` = ?
			AND id IN (` + Placeholders(len(memberIDs)) + `)
	`
}

// listOllamaCloudUsageGroupAccountsMySQL 是 ListOllamaCloudUsageGroupAccounts 的
// MySQL 实现：`credentials ->> 'api_key' = ANY($1)` → IN 展开；组员水合与
// PG 版共用 GetByIDs。
func (r *accountRepository) listOllamaCloudUsageGroupAccountsMySQL(ctx context.Context, accounts []*service.Account) ([]service.Account, error) {
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
	args := make([]any, 0, len(keys))
	for _, key := range keys {
		args = append(args, key)
	}
	rows, err := r.sql.QueryContext(ctx, `
		SELECT id
		FROM accounts
		WHERE deleted_at IS NULL
			AND `+ollamaCloudUsageEligibleSQLMySQL+`
			AND `+mysqlJSONTextOf("credentials", "api_key")+` IN (`+Placeholders(len(keys))+`)
		ORDER BY id
	`, args...)
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

// ollamaCloudUsageParseRFC3339SQLMySQL 是 ollamaCloudUsageParseRFC3339SQL 的 MySQL 版。
// PG 的 jsonb_path_query_first_tz(... '$.datetime()') 宽松 RFC3339 解析改为：
// 形状正则预检（失败/缺失 fail-open 为 NULL）→ 截断第 7 位起的小数 → 去掉末尾 Z/偏移 →
// 'T' 换空格 → CAST AS DATETIME(6) → CONVERT_TZ(自身偏移, 会话时区)。
// 产出会话时区墙钟 DATETIME，可与 last_used_at（会话墙钟）及绑定的 now 参数直接比较，
// 对齐 PG 版 timestamptz 的「时间点比较」语义。CAST 对不存在的日期（如 2026-02-30）
// 返回 NULL，与 PG jsonpath 的 fail-open 一致。表达式在 SQL 中出现 4 次，由调用方
// 保证其为纯列引用（无占位符）。
// REVIEW: PG 的 jsonpath .datetime() 可解析 'Z' 与数值偏移（PG17 起支持 'Z'，旧版本
// 靠 Z→+00:00 改写）；MySQL 版经 CONVERT_TZ 显式换算，等价成立依赖会话 time_zone 为
// internal/config.MySQLConfig 写入的固定偏移（CONVERT_TZ 对命名时区需服务器时区表）。
func ollamaCloudUsageParseRFC3339SQLMySQL(expression string) string {
	return `CASE
		WHEN ` + expression + ` IS NULL THEN NULL
		WHEN ` + expression + ` REGEXP '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\\.[0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})$'
			THEN CONVERT_TZ(
				CAST(REPLACE(
					REGEXP_REPLACE(REGEXP_REPLACE(
						` + expression + `,
						'(\\.[0-9]{6})[0-9]+(Z|[+-][0-9]{2}:[0-9]{2})$',
						'$1$2'
					),
					'(Z|[+-][0-9]{2}:[0-9]{2})$',
					''
				), 'T', ' ') AS DATETIME(6)),
				COALESCE(REGEXP_SUBSTR(` + expression + `, '[+-][0-9]{2}:[0-9]{2}$'), '+00:00'),
				@@session.time_zone
			)
		ELSE NULL
	END`
}

// listDueOllamaCloudUsageAccountsMySQL 是 ListDueOllamaCloudUsageAccounts 的 MySQL 实现。
// 与 PG 版的语义对应：
//   - 只读 CTE 保留派生表结构（去掉 MATERIALIZED 提示，MySQL 8 无此提示，语义不变）；
//   - `extra @> '{"...": true}'::jsonb` → JSON_CONTAINS；
//   - make_interval(secs => $n::double precision) → INTERVAL ? MICROSECOND
//     （按微秒整数绑定，避免小数秒 INTERVAL 的方言行为差异）；
//   - '-infinity'::timestamptz → CAST('1970-01-01 00:00:00' AS DATETIME(6))
//     （充当「无下界」，同样小于任何真实时间）；
//   - `due_at NULLS FIRST` 去掉：MySQL ASC 排序 NULL 天然排最前，与 PG NULLS FIRST 一致；
//   - GREATEST/LEAST 仅在非空分支求值（CASE 守卫与 PG 一致），不受 MySQL
//     GREATEST/LEAST 遇 NULL 返回 NULL 的影响。
//
// 参数按 SQL 文本顺序：debounce、maxWait、minFetchInterval、debounce、maxWait、now、limit。
func (r *accountRepository) listDueOllamaCloudUsageAccountsMySQL(
	ctx context.Context,
	now time.Time,
	debounce, maxWait time.Duration,
	limit int,
) ([]service.Account, error) {
	debounceMicros := debounce.Microseconds()
	maxWaitMicros := maxWait.Microseconds()
	minFetchIntervalMicros := service.OllamaCloudUsageMinFetchInterval.Microseconds()
	rows, err := r.sql.QueryContext(ctx, `
		WITH eligible AS (
			SELECT id,
				`+mysqlJSONTextOf("credentials", "api_key")+` AS api_key,
				last_used_at,
				JSON_EXTRACT(extra, '$.ollama_cloud_usage_snapshot') AS snapshot
			FROM accounts
			WHERE deleted_at IS NULL
				AND status = 'active'
				AND `+ollamaCloudUsageEligibleSQLMySQL+`
				AND JSON_TYPE(JSON_EXTRACT(extra, '$.ollama_cloud_usage_session')) = 'STRING'
				AND JSON_CONTAINS(extra, CAST('{"ollama_cloud_usage_auto_refresh": true}' AS JSON))
		), group_activity AS (
			SELECT `+mysqlJSONTextOf("credentials", "api_key")+` AS api_key,
				MAX(last_used_at) AS group_last_used_at
			FROM accounts
			WHERE deleted_at IS NULL
				AND `+ollamaCloudUsageEligibleSQLMySQL+`
				AND JSON_TYPE(JSON_EXTRACT(credentials, '$.api_key')) = 'STRING'
			GROUP BY `+mysqlJSONTextOf("credentials", "api_key")+`
		), joined AS (
			SELECT e.id, e.api_key, e.snapshot, g.group_last_used_at,
				`+mysqlJSONTextOf("e.snapshot", "status")+` AS status,
				`+mysqlJSONTextOf("e.snapshot", "fetched_at")+` AS fetched_at,
				`+mysqlJSONTextOf("e.snapshot", "last_attempt_at")+` AS last_attempt_at,
				`+mysqlJSONTextOf("e.snapshot", "next_refresh_at")+` AS next_refresh_at
			FROM eligible e
			JOIN group_activity g ON g.api_key = e.api_key
		), parsed AS (
			SELECT id, api_key, snapshot, group_last_used_at, status,
				`+ollamaCloudUsageParseRFC3339SQLMySQL("fetched_at")+` AS parsed_fetched_at,
				`+ollamaCloudUsageParseRFC3339SQLMySQL("last_attempt_at")+` AS parsed_last_attempt_at,
				`+ollamaCloudUsageParseRFC3339SQLMySQL("next_refresh_at")+` AS parsed_next_refresh_at
			FROM joined
		), timed AS (
			SELECT *,
				CASE
					WHEN status = 'ok'
						AND parsed_fetched_at IS NOT NULL
						AND group_last_used_at IS NOT NULL
						AND group_last_used_at > parsed_fetched_at
					THEN GREATEST(
						LEAST(
							group_last_used_at + INTERVAL ? MICROSECOND,
							parsed_fetched_at + INTERVAL ? MICROSECOND
						),
						parsed_fetched_at + INTERVAL ? MICROSECOND
					)
					WHEN status IN ('failed', 'unauthorized')
						AND parsed_last_attempt_at IS NOT NULL
						AND group_last_used_at IS NOT NULL
						AND group_last_used_at > parsed_last_attempt_at
					THEN GREATEST(
						LEAST(
							group_last_used_at + INTERVAL ? MICROSECOND,
							parsed_last_attempt_at + INTERVAL ? MICROSECOND
						),
						COALESCE(parsed_next_refresh_at, CAST('1970-01-01 00:00:00' AS DATETIME(6)))
					)
					ELSE NULL
				END AS activity_due_at
			FROM parsed
		), candidates AS (
			SELECT *,
				CASE
					WHEN snapshot IS NULL OR JSON_TYPE(snapshot) = 'NULL' OR status IS NULL
						OR status NOT IN ('ok', 'failed', 'unauthorized') THEN 0
					WHEN status = 'ok' AND parsed_fetched_at IS NULL THEN 0
					WHEN status IN ('failed', 'unauthorized') AND parsed_last_attempt_at IS NULL THEN 0
					WHEN activity_due_at IS NOT NULL AND ? >= activity_due_at THEN 1
					ELSE NULL
				END AS due_class,
				activity_due_at AS due_at
			FROM timed
		), ranked AS (
			SELECT id, api_key, group_last_used_at, due_class, due_at,
				ROW_NUMBER() OVER (
					PARTITION BY api_key
					ORDER BY due_class,
						due_at,
						id
				) AS group_rank
			FROM candidates
			WHERE due_class IS NOT NULL
		)
		SELECT id, group_last_used_at
		FROM ranked
		WHERE group_rank = 1
		ORDER BY due_class, due_at, id
		LIMIT ?
	`, debounceMicros, maxWaitMicros, minFetchIntervalMicros, debounceMicros, maxWaitMicros, now, limit)
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
