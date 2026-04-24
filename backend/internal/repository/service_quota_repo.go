package repository

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type serviceQuotaRuleRepository struct{ db *sql.DB }

func NewServiceQuotaRuleRepository(db *sql.DB) service.ServiceQuotaRuleRepository {
	return &serviceQuotaRuleRepository{db: db}
}

func (r *serviceQuotaRuleRepository) List(ctx context.Context, filter service.ServiceQuotaListFilter) ([]*service.ServiceQuotaRule, error) {
	query := `SELECT id, enabled, scope_level, platform, group_id, account_id, model_pattern, limiter_type, target_mode, target_user_id, window_mode, limit_value, created_at, updated_at FROM service_quota_rules WHERE 1=1`
	args := []any{}
	if filter.Enabled != nil {
		args = append(args, *filter.Enabled)
		query += ` AND enabled = $` + serviceQuotaItoa(len(args))
	}
	if strings.TrimSpace(filter.ScopeLevel) != "" {
		args = append(args, filter.ScopeLevel)
		query += ` AND scope_level = $` + serviceQuotaItoa(len(args))
	}
	if strings.TrimSpace(filter.LimiterType) != "" {
		args = append(args, filter.LimiterType)
		query += ` AND limiter_type = $` + serviceQuotaItoa(len(args))
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
	return out, rows.Err()
}

func (r *serviceQuotaRuleRepository) Create(ctx context.Context, input service.ServiceQuotaRuleInput) (*service.ServiceQuotaRule, error) {
	row := r.db.QueryRowContext(ctx, serviceQuotaInsertSQL, input.Enabled, input.ScopeLevel, input.Platform, input.GroupID, input.AccountID, input.ModelPattern, input.LimiterType, input.TargetMode, input.TargetUserID, input.WindowMode, input.LimitValue)
	return scanServiceQuotaRule(row)
}

func (r *serviceQuotaRuleRepository) Update(ctx context.Context, id int64, input service.ServiceQuotaRuleInput) (*service.ServiceQuotaRule, error) {
	row := r.db.QueryRowContext(ctx, serviceQuotaUpdateSQL, input.Enabled, input.ScopeLevel, input.Platform, input.GroupID, input.AccountID, input.ModelPattern, input.LimiterType, input.TargetMode, input.TargetUserID, input.WindowMode, input.LimitValue, id)
	return scanServiceQuotaRule(row)
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

const serviceQuotaColumns = `id, enabled, scope_level, platform, group_id, account_id, model_pattern, limiter_type, target_mode, target_user_id, window_mode, limit_value, created_at, updated_at`

const serviceQuotaInsertSQL = `INSERT INTO service_quota_rules (enabled, scope_level, platform, group_id, account_id, model_pattern, limiter_type, target_mode, target_user_id, window_mode, limit_value) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING ` + serviceQuotaColumns

const serviceQuotaUpdateSQL = `UPDATE service_quota_rules SET enabled=$1, scope_level=$2, platform=$3, group_id=$4, account_id=$5, model_pattern=$6, limiter_type=$7, target_mode=$8, target_user_id=$9, window_mode=$10, limit_value=$11, updated_at=now() WHERE id=$12 RETURNING ` + serviceQuotaColumns

type serviceQuotaScanner interface{ Scan(dest ...any) error }

func scanServiceQuotaRule(scanner serviceQuotaScanner) (*service.ServiceQuotaRule, error) {
	var rule service.ServiceQuotaRule
	var platform, model sql.NullString
	var groupID, accountID, targetUserID sql.NullInt64
	err := scanner.Scan(&rule.ID, &rule.Enabled, &rule.ScopeLevel, &platform, &groupID, &accountID, &model, &rule.LimiterType, &rule.TargetMode, &targetUserID, &rule.WindowMode, &rule.LimitValue, &rule.CreatedAt, &rule.UpdatedAt)
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
	if targetUserID.Valid {
		rule.TargetUserID = &targetUserID.Int64
	}
	return &rule, nil
}

func serviceQuotaItoa(v int) string { return strconv.Itoa(v) }
