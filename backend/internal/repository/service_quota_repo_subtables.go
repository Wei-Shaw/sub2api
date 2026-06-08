package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/lib/pq"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *serviceQuotaRuleRepository) loadLimiters(ctx context.Context, rules []*service.ServiceQuotaRule) error {
	if len(rules) == 0 {
		return nil
	}
	ids, index := indexRules(rules)
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(
		`SELECT id, rule_id, limiter_type, window_mode, limit_value, token_components, count_on_arrival FROM service_quota_limiters WHERE rule_id IN (%s) ORDER BY rule_id, limiter_type`,
		joinPlaceholders(len(ids))), ids...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var def service.ServiceQuotaLimiterDef
		var components pq.StringArray
		var countOnArrival bool
		if err := rows.Scan(&def.ID, &def.RuleID, &def.LimiterType, &def.WindowMode, &def.LimitValue, &components, &countOnArrival); err != nil {
			return err
		}
		// 仅 TPM/TPD 暴露 token_components；其他类型忽略 DB 默认值，对外保持 nil。
		if service.ServiceQuotaLimiterTypeUsesTokenComponents(def.LimiterType) {
			def.TokenComponents = []string(components)
		}
		// 仅 RPM 使用 count_on_arrival；非 RPM 行 DB 维持默认 false，这里也强制清零避免外泄。
		if service.ServiceQuotaLimiterTypeUsesCountOnArrival(def.LimiterType) {
			def.CountOnArrival = countOnArrival
		}
		if rule, ok := index[def.RuleID]; ok {
			rule.Limiters = append(rule.Limiters, def)
		}
	}
	return rows.Err()
}

func (r *serviceQuotaRuleRepository) loadPaths(ctx context.Context, rules []*service.ServiceQuotaRule) error {
	if len(rules) == 0 {
		return nil
	}
	ids, index := indexRules(rules)
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(
		`SELECT id, rule_id, platform, channel_id, group_id, account_id, model_pattern FROM service_quota_paths WHERE rule_id IN (%s) ORDER BY rule_id, id`,
		joinPlaceholders(len(ids))), ids...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var def service.ServiceQuotaPathDef
		var platform, model sql.NullString
		var channelID, groupID, accountID sql.NullInt64
		if err := rows.Scan(&def.ID, &def.RuleID, &platform, &channelID, &groupID, &accountID, &model); err != nil {
			return err
		}
		if platform.Valid {
			def.Platform = &platform.String
		}
		if channelID.Valid {
			def.ChannelID = &channelID.Int64
		}
		if groupID.Valid {
			def.GroupID = &groupID.Int64
		}
		if accountID.Valid {
			def.AccountID = &accountID.Int64
		}
		if model.Valid {
			def.ModelPattern = &model.String
		}
		if rule, ok := index[def.RuleID]; ok {
			rule.Paths = append(rule.Paths, def)
		}
	}
	return rows.Err()
}

func (r *serviceQuotaRuleRepository) loadRuleUsers(ctx context.Context, rules []*service.ServiceQuotaRule) error {
	relevant := make([]*service.ServiceQuotaRule, 0, len(rules))
	for _, rule := range rules {
		if rule.CounterMode == service.ServiceQuotaCounterModeUser {
			relevant = append(relevant, rule)
		}
	}
	if len(relevant) == 0 {
		return nil
	}
	ids, index := indexRules(relevant)
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(
		`SELECT ru.rule_id, ru.user_id, COALESCE(u.email, '') FROM service_quota_rule_users ru LEFT JOIN users u ON u.id = ru.user_id WHERE ru.rule_id IN (%s) ORDER BY ru.rule_id, ru.user_id`,
		joinPlaceholders(len(ids))), ids...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var ruleID, userID int64
		var email string
		if err := rows.Scan(&ruleID, &userID, &email); err != nil {
			return err
		}
		if rule, ok := index[ruleID]; ok {
			rule.TargetUserIDs = append(rule.TargetUserIDs, userID)
			rule.TargetUsers = append(rule.TargetUsers, service.ServiceQuotaRuleUserRef{ID: userID, Email: email})
		}
	}
	return rows.Err()
}

// upsertLimitersTx 用"按 (rule_id, limiter_type) 内容稳定 id"语义写入 limiters。
//
// 流程：
//  1. 删除新清单中不再出现的 limiter_type（保留输入中存在的 type 的 id）
//  2. ON CONFLICT (rule_id, limiter_type) DO UPDATE 把 limit_value/window_mode 更新到位
//
// 这样 user 改 RPM=60 → RPM=600 时 limiter id 不变（虽然 counterKey 公式不含 limiter_id，
// 但保留 id 仍然是 reuse 原则的体现，避免无意义的 id 自增 burn）。
//
// 输入空 limiters 时清空规则下所有 limiter（统一在 deleteRemovedLimitersTx 里处理）。
func upsertLimitersTx(ctx context.Context, tx *sql.Tx, ruleID int64, limiters []service.ServiceQuotaLimiterInput) error {
	if err := deleteRemovedLimitersTx(ctx, tx, ruleID, limiters); err != nil {
		return err
	}
	if len(limiters) == 0 {
		return nil
	}
	values := make([]string, len(limiters))
	args := make([]any, 0, len(limiters)*6)
	for i, l := range limiters {
		base := i * 6
		values[i] = fmt.Sprintf("($%d,$%d,$%d,$%d,$%d,$%d)", base+1, base+2, base+3, base+4, base+5, base+6)
		// 非 TPM/TPD 的 limiter 强制 token_components 为默认值（DB 层 NOT NULL，
		// 入口侧传任意值都会被这里覆盖成默认；service 层暴露给前端时再清空）。
		components := l.TokenComponents
		if !service.ServiceQuotaLimiterTypeUsesTokenComponents(l.LimiterType) || len(components) == 0 {
			components = service.ServiceQuotaTokenComponentsDefault
		}
		// 非 RPM 的 limiter count_on_arrival 恒置 false（DB 默认即 false，仍显式覆盖避免管理员误填）。
		countOnArrival := false
		if service.ServiceQuotaLimiterTypeUsesCountOnArrival(l.LimiterType) && l.CountOnArrival != nil {
			countOnArrival = *l.CountOnArrival
		}
		args = append(args, ruleID, l.LimiterType, l.WindowMode, l.LimitValue, pq.StringArray(components), countOnArrival)
	}
	query := `INSERT INTO service_quota_limiters (rule_id, limiter_type, window_mode, limit_value, token_components, count_on_arrival) VALUES ` +
		strings.Join(values, ",") +
		` ON CONFLICT (rule_id, limiter_type) DO UPDATE SET window_mode = EXCLUDED.window_mode, limit_value = EXCLUDED.limit_value, token_components = EXCLUDED.token_components, count_on_arrival = EXCLUDED.count_on_arrival`
	_, err := tx.ExecContext(ctx, query, args...)
	return err
}

// deleteRemovedLimitersTx 把"新输入清单里没有的 limiter_type"从 DB 删除。
// 拆出来便于 upsertLimitersTx 保持 ≤30 行。
func deleteRemovedLimitersTx(ctx context.Context, tx *sql.Tx, ruleID int64, limiters []service.ServiceQuotaLimiterInput) error {
	if len(limiters) == 0 {
		_, err := tx.ExecContext(ctx, `DELETE FROM service_quota_limiters WHERE rule_id = $1`, ruleID)
		return err
	}
	args := []any{ruleID}
	placeholders := make([]string, len(limiters))
	for i, l := range limiters {
		args = append(args, l.LimiterType)
		placeholders[i] = fmt.Sprintf("$%d", i+2)
	}
	query := `DELETE FROM service_quota_limiters WHERE rule_id = $1 AND limiter_type NOT IN (` + strings.Join(placeholders, ",") + `)`
	_, err := tx.ExecContext(ctx, query, args...)
	return err
}

// upsertPathsTx 用"按 (rule_id, platform, channel_id, group_id, account_id, model_pattern)
// 唯一索引 idx_service_quota_paths_unique 内容稳定 id"语义写入 paths。
// 该索引内部用 COALESCE 把 NULL 折叠为零值参与去重，5 字段完全一致即视作"同一条 path"。
//
// 流程：
//  1. 删除新清单中不再出现的 path（按所有 5 个字段比对，NULL 折叠为零值）
//  2. ON CONFLICT 命中已有 unique index 时不更新任何字段（path 5 个字段就是身份本身，
//     保留旧 id 即可——这是"保留计数"的关键，因为 counterKey 公式含 path_id）
//
// 注意 unique index 用 COALESCE 把 NULL 折叠为零值参与去重，所以 ON CONFLICT 子句要写
// "ON CONSTRAINT idx_service_quota_paths_unique"——索引本身是唯一约束。
func upsertPathsTx(ctx context.Context, tx *sql.Tx, ruleID int64, paths []service.ServiceQuotaPathInput) error {
	if err := deleteRemovedPathsTx(ctx, tx, ruleID, paths); err != nil {
		return err
	}
	if len(paths) == 0 {
		return nil
	}
	values := make([]string, len(paths))
	args := make([]any, 0, len(paths)*6)
	for i, p := range paths {
		base := i * 6
		values[i] = fmt.Sprintf("($%d,$%d,$%d,$%d,$%d,$%d)", base+1, base+2, base+3, base+4, base+5, base+6)
		args = append(args, ruleID, p.Platform, p.ChannelID, p.GroupID, p.AccountID, p.ModelPattern)
	}
	// ON CONFLICT 用列+表达式形式匹配唯一索引 idx_service_quota_paths_unique（含 COALESCE 折叠 NULL）做冲突推断，
	// 命中时 DO NOTHING：5 字段身份完全一致 → 保留旧行 + 旧 id，让 Redis 计数延续。
	// 注：unique index（不是 constraint）只能用列+表达式形式推断；ON CONFLICT ON CONSTRAINT <index_name> 在 PG 里会失败。
	query := `INSERT INTO service_quota_paths (rule_id, platform, channel_id, group_id, account_id, model_pattern) VALUES ` +
		strings.Join(values, ",") +
		` ON CONFLICT (rule_id, COALESCE(platform, ''), COALESCE(channel_id, 0), COALESCE(group_id, 0), COALESCE(account_id, 0), COALESCE(model_pattern, '')) DO NOTHING`
	_, err := tx.ExecContext(ctx, query, args...)
	return err
}

// deleteRemovedPathsTx 把"新输入清单里没有的 path 组合"从 DB 删除。
// NULL 字段统一用 COALESCE 折叠成零值，与 idx_service_quota_paths_unique 的语义一致。
//
// 没有合适的 NOT IN 多列写法时用 NOT EXISTS + 临时 VALUES 子查询：可读性比连串 OR 高，
// 也不会被 SQL 注入（参数走占位符）。
func deleteRemovedPathsTx(ctx context.Context, tx *sql.Tx, ruleID int64, paths []service.ServiceQuotaPathInput) error {
	if len(paths) == 0 {
		_, err := tx.ExecContext(ctx, `DELETE FROM service_quota_paths WHERE rule_id = $1`, ruleID)
		return err
	}
	values := make([]string, len(paths))
	args := []any{ruleID}
	for i, p := range paths {
		base := i*5 + 2
		values[i] = fmt.Sprintf("($%d::varchar, $%d::bigint, $%d::bigint, $%d::bigint, $%d::varchar)",
			base, base+1, base+2, base+3, base+4)
		args = append(args, p.Platform, p.ChannelID, p.GroupID, p.AccountID, p.ModelPattern)
	}
	query := `DELETE FROM service_quota_paths
		WHERE rule_id = $1
		AND NOT EXISTS (
			SELECT 1 FROM (VALUES ` + strings.Join(values, ",") + `) AS keep(platform, channel_id, group_id, account_id, model_pattern)
			WHERE COALESCE(service_quota_paths.platform, '')      = COALESCE(keep.platform, '')
			  AND COALESCE(service_quota_paths.channel_id, 0)     = COALESCE(keep.channel_id, 0)
			  AND COALESCE(service_quota_paths.group_id, 0)       = COALESCE(keep.group_id, 0)
			  AND COALESCE(service_quota_paths.account_id, 0)     = COALESCE(keep.account_id, 0)
			  AND COALESCE(service_quota_paths.model_pattern, '') = COALESCE(keep.model_pattern, '')
		)`
	_, err := tx.ExecContext(ctx, query, args...)
	return err
}

func replaceRuleUsersTx(ctx context.Context, tx *sql.Tx, ruleID int64, userIDs []int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM service_quota_rule_users WHERE rule_id = $1`, ruleID); err != nil {
		return err
	}
	unique := dedupUserIDs(userIDs)
	if len(unique) == 0 {
		return nil
	}
	values := make([]string, len(unique))
	args := make([]any, 0, len(unique)*2)
	for i, uid := range unique {
		values[i] = fmt.Sprintf("($%d,$%d)", i*2+1, i*2+2)
		args = append(args, ruleID, uid)
	}
	query := `INSERT INTO service_quota_rule_users (rule_id, user_id) VALUES ` + strings.Join(values, ",")
	_, err := tx.ExecContext(ctx, query, args...)
	return err
}

func dedupUserIDs(ids []int64) []int64 {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
