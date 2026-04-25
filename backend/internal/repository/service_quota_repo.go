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
	if err := r.loadLimiters(ctx, out); err != nil {
		return nil, err
	}
	if err := r.loadPaths(ctx, out); err != nil {
		return nil, err
	}
	if err := r.loadRuleUsers(ctx, out); err != nil {
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
	if err := insertLimitersTx(ctx, tx, rule.ID, input.Limiters); err != nil {
		return nil, err
	}
	if err := insertPathsTx(ctx, tx, rule.ID, input.Paths); err != nil {
		return nil, err
	}
	if err := replaceRuleUsersTx(ctx, tx, rule.ID, input.TargetUserIDs); err != nil {
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

	if _, err := tx.ExecContext(ctx, `DELETE FROM service_quota_limiters WHERE rule_id = $1`, rule.ID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM service_quota_paths WHERE rule_id = $1`, rule.ID); err != nil {
		return nil, err
	}
	if err := insertLimitersTx(ctx, tx, rule.ID, input.Limiters); err != nil {
		return nil, err
	}
	if err := insertPathsTx(ctx, tx, rule.ID, input.Paths); err != nil {
		return nil, err
	}
	if err := replaceRuleUsersTx(ctx, tx, rule.ID, input.TargetUserIDs); err != nil {
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
	if err := r.loadLimiters(ctx, rules); err != nil {
		return nil, err
	}
	if err := r.loadPaths(ctx, rules); err != nil {
		return nil, err
	}
	if err := r.loadRuleUsers(ctx, rules); err != nil {
		return nil, err
	}
	return rule, nil
}

func (r *serviceQuotaRuleRepository) loadLimiters(ctx context.Context, rules []*service.ServiceQuotaRule) error {
	if len(rules) == 0 {
		return nil
	}
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

func insertPathsTx(ctx context.Context, tx *sql.Tx, ruleID int64, paths []service.ServiceQuotaPathInput) error {
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
	query := `INSERT INTO service_quota_paths (rule_id, platform, channel_id, group_id, account_id, model_pattern) VALUES ` + strings.Join(values, ",")
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
