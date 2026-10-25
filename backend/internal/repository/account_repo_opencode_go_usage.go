package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

const (
	// 与 service.isOpenCodeGoBaseURL 对齐：Go 侧接受显式默认端口 :443（parsed.Host ==
	// hostname+":443"）与两个官方基址变体（CC/Responses 的 /zen/go/v1 与 Anthropic
	// 的 /zen/go），SQL 正则必须同样接受，否则同一 base_url 在 Go 判定 eligible 而
	// SQL 判定不 eligible，身份清理与组查询会漏行。只做 parity，不扩域名/路径；
	// Zen 基址（/zen、/zen/v1）必须拒绝。
	opencodeGoBaseURLRegexSQL       = `^[hH][tT][tT][pP][sS]://[oO][pP][eE][nN][cC][oO][dD][eE]\.[aA][iI](:[4][4][3])?/[zZ][eE][nN]/[gG][oO](/[vV]1)?/?$`
	opencodeGoBaseURLMatchSQLPrefix = "btrim("
	opencodeGoBaseURLMatchSQLSuffix = ") ~ '" + opencodeGoBaseURLRegexSQL + "'"
	// opencodeGoUsageMountPlatformsSQL 是 service.isOpenCodeGoUsageMountPlatform 的
	// SQL 镜像：OpenCode Go key 允许挂在 openai/anthropic 与国产 OpenAI 兼容平台
	// 下复用，与 ollama 的 ollamaCloudUsagePlatformsSQL 完全一致。opencode_go
	// 平台本身不在名单内——平台账号走下方 account_mode 分支。所有平台白名单 SQL
	// 只允许引用本常量，不得各处重写字面量，防止漂移。
	opencodeGoUsageMountPlatformsSQL = "'openai', 'anthropic', 'kimi', 'zhipu', 'deepseek', 'minimax'"
	// opencodeGoUsageEligibleSQL 与 service.IsOpenCodeGoUsageAccount 互为镜像：
	//   - opencode_go 平台：account_mode 存储于 credentials（domain/constants.go），
	//     未设置/为 null/非 "zen" 一律视为 Go 订阅，与 GetOpenCodeAccountMode 的
	//     默认兼容逻辑一致；COALESCE 把 <> 对 NULL 的结果归一为 true。不校验
	//     base_url（平台字段已是权威来源）。
	//   - 挂载白名单平台：base_url 必须匹配官方 OpenCode Go 基址正则。
	//
	// 已知且可接受的差异：btrim 只去空格，Go 侧 strings.TrimSpace 还会去
	// \t\n\v\f\r 等空白；account_mode 等凭证字段实际不会出现这类空白，且改用
	// btrim(x, E' \t\n\r\f\v') 会在 parity 关键的 SQL 字符串里引入转义脆弱性，
	// 故保持现状不改行为。
	opencodeGoUsageEligibleSQL = `
	(
		(platform = 'opencode_go'
			AND COALESCE(btrim(credentials ->> 'account_mode') <> 'zen', true))
		OR (platform IN (` + opencodeGoUsageMountPlatformsSQL + `)
			AND ` + opencodeGoBaseURLMatchSQLPrefix + `credentials ->> 'base_url'` + opencodeGoBaseURLMatchSQLSuffix + `)
	)
	AND type = 'apikey'
	AND jsonb_typeof(credentials -> 'api_key') = 'string'
`
)

// opencodeGoUsageEligibleSQLMySQL 是 opencodeGoUsageEligibleSQL 的 MySQL 版，语义逐条
// 对应（见 DIALECT_MYSQL_GUIDE.md）：btrim(x) ~ 正则 → mysqlOpenCodeBaseURLMatchesSQL
// （REGEXP_LIKE(TRIM(x), 正则, 'c')，正则反斜杠已按 MySQL 字面量规则翻倍）；
// credentials ->> 'key' → mysqlJSONTextOf（缺失路径/JSON null 归一为 SQL NULL）；
// jsonb_typeof(credentials -> 'api_key') = 'string' → JSON_TYPE(...) = 'STRING'（大写）。
// btrim 只去空格的既知差异与 PG 版保持一致（account_mode 等凭证字段不出现空白）。
var opencodeGoUsageEligibleSQLMySQL = `
	(
		(platform = 'opencode_go'
			AND COALESCE(TRIM(` + mysqlJSONTextOf("credentials", "account_mode") + `) <> 'zen', TRUE))
		OR (platform IN (` + opencodeGoUsageMountPlatformsSQL + `)
			AND ` + mysqlOpenCodeBaseURLMatchesSQL(mysqlJSONTextOf("credentials", "base_url")) + `)
	)
	AND type = 'apikey'
	AND JSON_TYPE(JSON_EXTRACT(credentials, '$.api_key')) = 'STRING'
`

// ListOpenCodeGoUsageGroupAccounts resolves every sibling for all supplied
// identities with one ID query and one batch hydration. API keys are query
// parameters only; no derived shared key is persisted.
func (r *accountRepository) ListOpenCodeGoUsageGroupAccounts(ctx context.Context, accounts []*service.Account) ([]service.Account, error) {
	if r == nil || r.sql == nil {
		return nil, service.ErrOpenCodeGoUsageUnavailable
	}
	if IsMySQL() {
		return r.listOpenCodeGoUsageGroupAccountsMySQL(ctx, accounts)
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
	return r.updateOpenCodeGoUsageGroup(ctx, account, map[string]any{
		service.OpenCodeGoUsageSnapshotExtraKey: snapshot,
	}, nil, true)
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
		var query string
		var args []any
		if IsMySQL() {
			query, args = openCodeGoUsageGroupUpdateSQLMySQL(deleteKeys, memberIDs, string(encoded), apiKey)
		} else {
			mergeExpr := "COALESCE(extra, '{}'::jsonb)"
			for _, key := range deleteKeys {
				mergeExpr += " - " + pq.QuoteLiteral(key)
			}
			mergeExpr += " || $1::jsonb"
			query = `
				UPDATE accounts
				SET extra = ` + mergeExpr + `,
					updated_at = NOW()
				WHERE deleted_at IS NULL
					AND ` + opencodeGoUsageEligibleSQL + `
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
	// anchorMatches 布尔列：MySQL 用 <=> 对齐 PG 的 IS NOT DISTINCT FROM；
	// (extra -> 'key')::text → CAST(JSON_EXTRACT(...) AS CHAR)。FOR NO KEY UPDATE → FOR UPDATE。
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
				COALESCE(CAST(JSON_EXTRACT(extra, '$.opencode_go_usage_auto_refresh') AS CHAR), 'null'),
				COALESCE(CAST(JSON_EXTRACT(extra, '$.opencode_go_usage_snapshot') AS CHAR), 'null')
			FROM accounts
			WHERE deleted_at IS NULL
				AND `+opencodeGoUsageEligibleSQLMySQL+`
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
				COALESCE((extra -> 'opencode_go_usage_auto_refresh')::text, 'null'),
				COALESCE((extra -> 'opencode_go_usage_snapshot')::text, 'null')
			FROM accounts
			WHERE deleted_at IS NULL
				AND `+opencodeGoUsageEligibleSQL+`
				AND credentials ->> 'api_key' = $1
			ORDER BY id
			FOR NO KEY UPDATE
		`, apiKey, account.ID, account.Platform, account.Type, string(credentials), proxyID)
	}
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
	// debounce/maxWait 的兜底默认值置于方言分流之前，保证 MySQL 早退分支与
	// PG 分支拿到同样的兜底语义（服务层已归一为正值，此处仅为防御性兜底）。
	if debounce <= 0 {
		debounce = time.Minute
	}
	if maxWait <= 0 {
		maxWait = 15 * time.Minute
	}
	if IsMySQL() {
		return r.listDueOpenCodeGoUsageAccountsMySQL(ctx, now, debounce, maxWait, limit)
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

// ---------------------------------------------------------------------------
// MySQL 方言实现（见 DIALECT_MYSQL_GUIDE.md）。语义与各 PG 版本逐条对应：
// `->>`/#>> → mysqlJSONTextOf / JSON_UNQUOTE(JSON_EXTRACT(...))；`= ANY($1)` → IN 展开；
// `@>` → JSON_CONTAINS；`FOR NO KEY UPDATE` → `FOR UPDATE`；NOW() → NOW(3)。
// ---------------------------------------------------------------------------

// openCodeGoUsageGroupUpdateSQLMySQL 构造 opencode go 组内成员 extra 合并的 MySQL UPDATE，
// 对应 PG 的 `(COALESCE(extra,'{}'::jsonb) - key1 - key2 ...) || $1::jsonb`：
// JSON_MERGE_PATCH(JSON_REMOVE(COALESCE(extra, CAST('{}' AS JSON)), ?, ...), CAST(? AS JSON))。
// 删除路径以绑定参数传入（对齐 mysqlJSONSetMergeSQL 的 "$.key" 路径参数风格，避免拼接转义）；
// patch 键先被 JSON_REMOVE 移除、与 base 无交集，JSON_MERGE_PATCH 的递归对象合并不会触发，
// 与 base 无交集，JSON_MERGE_PATCH 的递归对象合并不会触发，
// 与 PG `||` 的整键替换语义等价。deleteKeys 为空时跳过 JSON_REMOVE（MySQL 的 JSON_REMOVE
// 至少需要一个 path 参数）。返回语句与按序参数（删除路径 → payload → apiKey → memberIDs）。
// REVIEW: payload 值若为 JSON null（理论上仅当 account.Extra 中 managed key 本身为 null
// 时经 openCodeGoUsageManagedPayload 复制产生），JSON_MERGE_PATCH 会删除该键，而 PG `||`
// 会将其置为 JSON null；两种状态在下游判定（due 列表的 snapshot IS NULL / = 'null'、
// CAS 的 canonicalJSON('null')）行为一致，故接受该表示差异。
func openCodeGoUsageGroupUpdateSQLMySQL(deleteKeys []string, memberIDs []int64, payloadJSON, apiKey string) (string, []any) {
	base := "COALESCE(extra, CAST('{}' AS JSON))"
	args := make([]any, 0, len(deleteKeys)+2+len(memberIDs))
	if len(deleteKeys) > 0 {
		placeholders := make([]string, 0, len(deleteKeys))
		for _, key := range deleteKeys {
			placeholders = append(placeholders, "?")
			args = append(args, "$."+mysqlJSONPathMember(key))
		}
		base = "JSON_REMOVE(" + base + ", " + strings.Join(placeholders, ", ") + ")"
	}
	query := `
		UPDATE accounts
		SET extra = JSON_MERGE_PATCH(` + base + `, CAST(? AS JSON)),
			updated_at = NOW(3)
		WHERE deleted_at IS NULL
			AND ` + opencodeGoUsageEligibleSQLMySQL + `
			AND ` + mysqlJSONTextOf("credentials", "api_key") + ` = ?
			AND id IN (` + Placeholders(len(memberIDs)) + `)
	`
	args = append(args, payloadJSON, apiKey)
	for _, id := range memberIDs {
		args = append(args, id)
	}
	return query, args
}

// listOpenCodeGoUsageGroupAccountsMySQL 是 ListOpenCodeGoUsageGroupAccounts 的
// MySQL 实现：`credentials ->> 'api_key' = ANY($1)` → IN 展开；组员水合与
// PG 版共用 GetByIDs。
func (r *accountRepository) listOpenCodeGoUsageGroupAccountsMySQL(ctx context.Context, accounts []*service.Account) ([]service.Account, error) {
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
	args := make([]any, 0, len(keys))
	for _, key := range keys {
		args = append(args, key)
	}
	rows, err := r.sql.QueryContext(ctx, `
		SELECT id
		FROM accounts
		WHERE deleted_at IS NULL
			AND `+opencodeGoUsageEligibleSQLMySQL+`
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

// listDueOpenCodeGoUsageAccountsMySQL 是 ListDueOpenCodeGoUsageAccounts 的 MySQL 实现。
// 语义对应与 listDueOllamaCloudUsageAccountsMySQL 相同（只读 CTE 去 MATERIALIZED 提示、
// @> → JSON_CONTAINS、make_interval → INTERVAL ? MICROSECOND、'-infinity' →
// '1970-01-01 00:00:00'、去 NULLS FIRST 依赖 MySQL ASC 的 NULL 排最前）。
// 参数按 SQL 文本顺序：debounce、maxWait、minFetchInterval、debounce、maxWait、now、limit。
func (r *accountRepository) listDueOpenCodeGoUsageAccountsMySQL(
	ctx context.Context,
	now time.Time,
	debounce, maxWait time.Duration,
	limit int,
) ([]service.Account, error) {
	debounceMicros := debounce.Microseconds()
	maxWaitMicros := maxWait.Microseconds()
	minFetchIntervalMicros := service.OpenCodeGoUsageMinFetchInterval.Microseconds()
	rows, err := r.sql.QueryContext(ctx, `
		WITH eligible AS (
			SELECT id,
				`+mysqlJSONTextOf("credentials", "api_key")+` AS api_key,
				last_used_at,
				JSON_EXTRACT(extra, '$.opencode_go_usage_snapshot') AS snapshot
			FROM accounts
			WHERE deleted_at IS NULL
				AND status = 'active'
				AND `+opencodeGoUsageEligibleSQLMySQL+`
				AND JSON_CONTAINS(extra, CAST('{"opencode_go_usage_auto_refresh": true}' AS JSON))
		), group_activity AS (
			SELECT `+mysqlJSONTextOf("credentials", "api_key")+` AS api_key,
				MAX(last_used_at) AS group_last_used_at
			FROM accounts
			WHERE deleted_at IS NULL
				AND `+opencodeGoUsageEligibleSQLMySQL+`
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
