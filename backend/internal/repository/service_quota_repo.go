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

const serviceQuotaColumns = `id, enabled, platform, group_id, account_id, model_pattern, limiter_type, counter_mode, is_fallback, window_mode, limit_value, created_at, updated_at`

const serviceQuotaInsertSQL = `INSERT INTO service_quota_rules (enabled, platform, group_id, account_id, model_pattern, limiter_type, counter_mode, is_fallback, window_mode, limit_value) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING ` + serviceQuotaColumns

const serviceQuotaUpdateSQL = `UPDATE service_quota_rules SET enabled=$1, platform=$2, group_id=$3, account_id=$4, model_pattern=$5, limiter_type=$6, counter_mode=$7, is_fallback=$8, window_mode=$9, limit_value=$10, updated_at=now() WHERE id=$11 RETURNING ` + serviceQuotaColumns

func (r *serviceQuotaRuleRepository) List(ctx context.Context, filter service.ServiceQuotaListFilter) ([]*service.ServiceQuotaRule, error) {
	query := `SELECT ` + serviceQuotaColumns + ` FROM service_quota_rules WHERE 1=1`
	args := []any{}
	if filter.Enabled != nil {
		args = append(args, *filter.Enabled)
		query += ` AND enabled = $` + strconv.Itoa(len(args))
	}
	if strings.TrimSpace(filter.LimiterType) != "" {
		args = append(args, filter.LimiterType)
		query += ` AND limiter_type = $` + strconv.Itoa(len(args))
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

	row := tx.QueryRowContext(ctx, serviceQuotaInsertSQL,
		input.Enabled, input.Platform, input.GroupID, input.AccountID,
		input.ModelPattern, input.LimiterType, input.CounterMode, input.IsFallback,
		input.WindowMode, input.LimitValue)
	rule, err := scanServiceQuotaRule(row)
	if err != nil {
		return nil, err
	}
	if err := replaceRuleUsersTx(ctx, tx, rule.ID, input.TargetUserIDs); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if err := r.loadRuleUsers(ctx, []*service.ServiceQuotaRule{rule}); err != nil {
		return nil, err
	}
	return rule, nil
}

func (r *serviceQuotaRuleRepository) Update(ctx context.Context, id int64, input service.ServiceQuotaRuleInput) (*service.ServiceQuotaRule, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	row := tx.QueryRowContext(ctx, serviceQuotaUpdateSQL,
		input.Enabled, input.Platform, input.GroupID, input.AccountID,
		input.ModelPattern, input.LimiterType, input.CounterMode, input.IsFallback,
		input.WindowMode, input.LimitValue, id)
	rule, err := scanServiceQuotaRule(row)
	if err != nil {
		return nil, err
	}
	if err := replaceRuleUsersTx(ctx, tx, rule.ID, input.TargetUserIDs); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if err := r.loadRuleUsers(ctx, []*service.ServiceQuotaRule{rule}); err != nil {
		return nil, err
	}
	return rule, nil
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

// FetchAccountScope 返回账号所属的 platform 及其所属的 group ID 列表，用于 scope 一致性校验。
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

// FetchGroupScope 返回分组的 platform。
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

// loadRuleUsers 批量补齐每条规则的绑定用户列表。
func (r *serviceQuotaRuleRepository) loadRuleUsers(ctx context.Context, rules []*service.ServiceQuotaRule) error {
	ids := make([]int64, 0, len(rules))
	index := make(map[int64]*service.ServiceQuotaRule, len(rules))
	for _, rule := range rules {
		if rule.CounterMode != service.ServiceQuotaCounterModeUser {
			continue
		}
		ids = append(ids, rule.ID)
		index[rule.ID] = rule
	}
	if len(ids) == 0 {
		return nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	query := fmt.Sprintf(`SELECT ru.rule_id, ru.user_id, COALESCE(u.email, '') FROM service_quota_rule_users ru LEFT JOIN users u ON u.id = ru.user_id WHERE ru.rule_id IN (%s) ORDER BY ru.rule_id, ru.user_id`, strings.Join(placeholders, ","))
	rows, err := r.db.QueryContext(ctx, query, args...)
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
	var platform, model sql.NullString
	var groupID, accountID sql.NullInt64
	err := scanner.Scan(
		&rule.ID, &rule.Enabled, &platform, &groupID, &accountID, &model,
		&rule.LimiterType, &rule.CounterMode, &rule.IsFallback,
		&rule.WindowMode, &rule.LimitValue, &rule.CreatedAt, &rule.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrServiceQuotaRuleNotFound
		}
		return nil, err
	}
	if platform.Valid {
		rule.Platform = &platform.String
	}
	if groupID.Valid {
		rule.GroupID = &groupID.Int64
	}
	if accountID.Valid {
		rule.AccountID = &accountID.Int64
	}
	if model.Valid {
		rule.ModelPattern = &model.String
	}
	return &rule, nil
}
