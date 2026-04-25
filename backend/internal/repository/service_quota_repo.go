package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type serviceQuotaRuleRepository struct{ db *sql.DB }

func NewServiceQuotaRuleRepository(db *sql.DB) service.ServiceQuotaRuleRepository {
	return &serviceQuotaRuleRepository{db: db}
}

const serviceQuotaRuleColumns = `id, enabled, name, counter_mode, is_fallback, created_at, updated_at`

// 5 张维度关联表的元信息：每张表有 (rule_id, <col>) 两列。
type dimensionTable struct {
	table  string
	column string
}

var (
	dimPlatforms = dimensionTable{"service_quota_rule_platforms", "platform"}
	dimChannels  = dimensionTable{"service_quota_rule_channels", "channel_id"}
	dimGroups    = dimensionTable{"service_quota_rule_groups", "group_id"}
	dimAccounts  = dimensionTable{"service_quota_rule_accounts", "account_id"}
	dimModels    = dimensionTable{"service_quota_rule_models", "model_pattern"}
)

func (r *serviceQuotaRuleRepository) List(ctx context.Context, filter service.ServiceQuotaListFilter) ([]*service.ServiceQuotaRule, error) {
	query := `SELECT ` + serviceQuotaRuleColumns + ` FROM service_quota_rules WHERE 1=1`
	args := []any{}
	if filter.Enabled != nil {
		args = append(args, *filter.Enabled)
		query += ` AND enabled = $` + strconv.Itoa(len(args))
	}
	if strings.TrimSpace(filter.LimiterType) != "" {
		args = append(args, filter.LimiterType)
		query += ` AND EXISTS (SELECT 1 FROM service_quota_limiters l WHERE l.rule_id = service_quota_rules.id AND l.limiter_type = $` + strconv.Itoa(len(args)) + `)`
	}
	query += ` ORDER BY id DESC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []*service.ServiceQuotaRule{}
	for rows.Next() {
		rule, err := scanServiceQuotaRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := r.loadAll(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *serviceQuotaRuleRepository) Create(ctx context.Context, input service.ServiceQuotaRuleInput) (*service.ServiceQuotaRule, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	row := tx.QueryRowContext(ctx,
		`INSERT INTO service_quota_rules (enabled, name, counter_mode, is_fallback) VALUES ($1,$2,$3,$4) RETURNING `+serviceQuotaRuleColumns,
		enabled, input.Name, input.CounterMode, input.IsFallback,
	)
	rule, err := scanServiceQuotaRule(row)
	if err != nil {
		return nil, err
	}
	if err := writeRuleChildren(ctx, tx, rule.ID, input); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.fetchByID(ctx, rule.ID)
}

func (r *serviceQuotaRuleRepository) Update(ctx context.Context, id int64, input service.ServiceQuotaRuleInput) (*service.ServiceQuotaRule, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	row := tx.QueryRowContext(ctx,
		`UPDATE service_quota_rules SET enabled=$1, name=$2, counter_mode=$3, is_fallback=$4, updated_at=now() WHERE id=$5 RETURNING `+serviceQuotaRuleColumns,
		enabled, input.Name, input.CounterMode, input.IsFallback, id,
	)
	rule, err := scanServiceQuotaRule(row)
	if err != nil {
		return nil, err
	}

	if err := deleteRuleChildren(ctx, tx, rule.ID); err != nil {
		return nil, err
	}
	if err := writeRuleChildren(ctx, tx, rule.ID, input); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.fetchByID(ctx, rule.ID)
}

func (r *serviceQuotaRuleRepository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM service_quota_rules WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *serviceQuotaRuleRepository) FetchAccountScope(ctx context.Context, accountID int64) (*service.AccountScopeInfo, error) {
	info := &service.AccountScopeInfo{}
	err := r.db.QueryRowContext(ctx, `SELECT platform FROM accounts WHERE id = $1`, accountID).Scan(&info.Platform)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT group_id FROM account_groups WHERE account_id = $1`, accountID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var gid int64
		if err := rows.Scan(&gid); err != nil {
			return nil, err
		}
		info.GroupIDs = append(info.GroupIDs, gid)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return info, nil
}

func (r *serviceQuotaRuleRepository) FetchGroupScope(ctx context.Context, groupID int64) (*service.GroupScopeInfo, error) {
	info := &service.GroupScopeInfo{}
	err := r.db.QueryRowContext(ctx, `SELECT platform FROM groups WHERE id = $1`, groupID).Scan(&info.Platform)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return info, nil
}

func (r *serviceQuotaRuleRepository) fetchByID(ctx context.Context, id int64) (*service.ServiceQuotaRule, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+serviceQuotaRuleColumns+` FROM service_quota_rules WHERE id = $1`, id)
	rule, err := scanServiceQuotaRule(row)
	if err != nil {
		return nil, err
	}
	rules := []*service.ServiceQuotaRule{rule}
	if err := r.loadAll(ctx, rules); err != nil {
		return nil, err
	}
	return rule, nil
}

func (r *serviceQuotaRuleRepository) loadAll(ctx context.Context, rules []*service.ServiceQuotaRule) error {
	if len(rules) == 0 {
		return nil
	}
	if err := r.loadLimiters(ctx, rules); err != nil {
		return err
	}
	if err := r.loadStringDim(ctx, rules, dimPlatforms, func(rule *service.ServiceQuotaRule, v string) {
		rule.Platforms = append(rule.Platforms, v)
	}); err != nil {
		return err
	}
	if err := r.loadStringDim(ctx, rules, dimModels, func(rule *service.ServiceQuotaRule, v string) {
		rule.ModelPatterns = append(rule.ModelPatterns, v)
	}); err != nil {
		return err
	}
	if err := r.loadInt64Dim(ctx, rules, dimChannels, func(rule *service.ServiceQuotaRule, v int64) {
		rule.ChannelIDs = append(rule.ChannelIDs, v)
	}); err != nil {
		return err
	}
	if err := r.loadInt64Dim(ctx, rules, dimGroups, func(rule *service.ServiceQuotaRule, v int64) {
		rule.GroupIDs = append(rule.GroupIDs, v)
	}); err != nil {
		return err
	}
	if err := r.loadInt64Dim(ctx, rules, dimAccounts, func(rule *service.ServiceQuotaRule, v int64) {
		rule.AccountIDs = append(rule.AccountIDs, v)
	}); err != nil {
		return err
	}
	return r.loadRuleUsers(ctx, rules)
}

func (r *serviceQuotaRuleRepository) loadLimiters(ctx context.Context, rules []*service.ServiceQuotaRule) error {
	ids, index := indexRules(rules)
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(
		`SELECT id, rule_id, limiter_type, window_mode, limit_value FROM service_quota_limiters WHERE rule_id IN (%s) ORDER BY rule_id, limiter_type`,
		joinPlaceholders(len(ids))), ids...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var def service.ServiceQuotaLimiterDef
		if err := rows.Scan(&def.ID, &def.RuleID, &def.LimiterType, &def.WindowMode, &def.LimitValue); err != nil {
			return err
		}
		if rule, ok := index[def.RuleID]; ok {
			rule.Limiters = append(rule.Limiters, def)
		}
	}
	return rows.Err()
}

// loadStringDim 通用加载 (rule_id, <string col>) 形态的维度表。
func (r *serviceQuotaRuleRepository) loadStringDim(ctx context.Context, rules []*service.ServiceQuotaRule, dim dimensionTable, assign func(*service.ServiceQuotaRule, string)) error {
	ids, index := indexRules(rules)
	query := fmt.Sprintf(`SELECT rule_id, %s FROM %s WHERE rule_id IN (%s) ORDER BY rule_id, %s`,
		dim.column, dim.table, joinPlaceholders(len(ids)), dim.column)
	rows, err := r.db.QueryContext(ctx, query, ids...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var ruleID int64
		var value string
		if err := rows.Scan(&ruleID, &value); err != nil {
			return err
		}
		if rule, ok := index[ruleID]; ok {
			assign(rule, value)
		}
	}
	return rows.Err()
}

// loadInt64Dim 通用加载 (rule_id, <int64 col>) 形态的维度表。
func (r *serviceQuotaRuleRepository) loadInt64Dim(ctx context.Context, rules []*service.ServiceQuotaRule, dim dimensionTable, assign func(*service.ServiceQuotaRule, int64)) error {
	ids, index := indexRules(rules)
	query := fmt.Sprintf(`SELECT rule_id, %s FROM %s WHERE rule_id IN (%s) ORDER BY rule_id, %s`,
		dim.column, dim.table, joinPlaceholders(len(ids)), dim.column)
	rows, err := r.db.QueryContext(ctx, query, ids...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var ruleID, value int64
		if err := rows.Scan(&ruleID, &value); err != nil {
			return err
		}
		if rule, ok := index[ruleID]; ok {
			assign(rule, value)
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

func indexRules(rules []*service.ServiceQuotaRule) ([]any, map[int64]*service.ServiceQuotaRule) {
	ids := make([]any, len(rules))
	index := make(map[int64]*service.ServiceQuotaRule, len(rules))
	for i, rule := range rules {
		ids[i] = rule.ID
		index[rule.ID] = rule
	}
	return ids, index
}

func joinPlaceholders(n int) string {
	parts := make([]string, n)
	for i := 0; i < n; i++ {
		parts[i] = fmt.Sprintf("$%d", i+1)
	}
	return strings.Join(parts, ",")
}

// writeRuleChildren 一次性写入限流器、5 个维度集合、用户绑定（事务内调用）。
func writeRuleChildren(ctx context.Context, tx *sql.Tx, ruleID int64, input service.ServiceQuotaRuleInput) error {
	if err := insertLimitersTx(ctx, tx, ruleID, input.Limiters); err != nil {
		return err
	}
	if err := insertStringDimensionTx(ctx, tx, dimPlatforms, ruleID, input.Platforms); err != nil {
		return err
	}
	if err := insertStringDimensionTx(ctx, tx, dimModels, ruleID, input.ModelPatterns); err != nil {
		return err
	}
	if err := insertInt64DimensionTx(ctx, tx, dimChannels, ruleID, input.ChannelIDs); err != nil {
		return err
	}
	if err := insertInt64DimensionTx(ctx, tx, dimGroups, ruleID, input.GroupIDs); err != nil {
		return err
	}
	if err := insertInt64DimensionTx(ctx, tx, dimAccounts, ruleID, input.AccountIDs); err != nil {
		return err
	}
	return replaceRuleUsersTx(ctx, tx, ruleID, input.TargetUserIDs)
}

// deleteRuleChildren 在 Update 前清空限流器和 5 张维度表（rule_users 由 replace 内部 DELETE+INSERT 处理）。
func deleteRuleChildren(ctx context.Context, tx *sql.Tx, ruleID int64) error {
	tables := []string{
		"service_quota_limiters",
		dimPlatforms.table,
		dimModels.table,
		dimChannels.table,
		dimGroups.table,
		dimAccounts.table,
	}
	for _, t := range tables {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE rule_id = $1`, t), ruleID); err != nil {
			return err
		}
	}
	return nil
}

func insertLimitersTx(ctx context.Context, tx *sql.Tx, ruleID int64, limiters []service.ServiceQuotaLimiterInput) error {
	if len(limiters) == 0 {
		return nil
	}
	values := make([]string, len(limiters))
	args := make([]any, 0, len(limiters)*4)
	for i, l := range limiters {
		base := i * 4
		values[i] = fmt.Sprintf("($%d,$%d,$%d,$%d)", base+1, base+2, base+3, base+4)
		args = append(args, ruleID, l.LimiterType, l.WindowMode, l.LimitValue)
	}
	query := `INSERT INTO service_quota_limiters (rule_id, limiter_type, window_mode, limit_value) VALUES ` + strings.Join(values, ",")
	_, err := tx.ExecContext(ctx, query, args...)
	return err
}

func insertStringDimensionTx(ctx context.Context, tx *sql.Tx, dim dimensionTable, ruleID int64, values []string) error {
	if len(values) == 0 {
		return nil
	}
	rows := make([]string, len(values))
	args := make([]any, 0, len(values)*2)
	for i, v := range values {
		rows[i] = fmt.Sprintf("($%d,$%d)", i*2+1, i*2+2)
		args = append(args, ruleID, v)
	}
	query := fmt.Sprintf(`INSERT INTO %s (rule_id, %s) VALUES %s`, dim.table, dim.column, strings.Join(rows, ","))
	_, err := tx.ExecContext(ctx, query, args...)
	return err
}

func insertInt64DimensionTx(ctx context.Context, tx *sql.Tx, dim dimensionTable, ruleID int64, values []int64) error {
	if len(values) == 0 {
		return nil
	}
	rows := make([]string, len(values))
	args := make([]any, 0, len(values)*2)
	for i, v := range values {
		rows[i] = fmt.Sprintf("($%d,$%d)", i*2+1, i*2+2)
		args = append(args, ruleID, v)
	}
	query := fmt.Sprintf(`INSERT INTO %s (rule_id, %s) VALUES %s`, dim.table, dim.column, strings.Join(rows, ","))
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

type serviceQuotaScanner interface{ Scan(dest ...any) error }

func scanServiceQuotaRule(scanner serviceQuotaScanner) (*service.ServiceQuotaRule, error) {
	var rule service.ServiceQuotaRule
	var name sql.NullString
	err := scanner.Scan(&rule.ID, &rule.Enabled, &name, &rule.CounterMode, &rule.IsFallback, &rule.CreatedAt, &rule.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrServiceQuotaRuleNotFound
		}
		return nil, err
	}
	if name.Valid {
		rule.Name = &name.String
	}
	return &rule, nil
}