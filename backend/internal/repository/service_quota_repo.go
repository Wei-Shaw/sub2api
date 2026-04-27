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
	// Create 与 Update 共用同一组 upsert helper（复用原则）：Create 时空表无冲突，
	// upsert 退化为普通 INSERT；Update 时 ON CONFLICT 保留旧 id 让计数延续。
	if err := upsertLimitersTx(ctx, tx, rule.ID, input.Limiters); err != nil {
		return nil, err
	}
	if err := upsertPathsTx(ctx, tx, rule.ID, input.Paths); err != nil {
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

	// 走 upsert 而非 DELETE+INSERT，让"调整 limit_value / 字段未变的 path"保留原 id。
	// 计数 key 公式 svcquota:v2:<rule>:<path>:<limiter_type>:<target> 含 path_id，
	// 旧实现会重置已有 Redis 计数；upsert 后内容相同的 path/limiter id 稳定，计数延续。
	if err := upsertLimitersTx(ctx, tx, rule.ID, input.Limiters); err != nil {
		return nil, err
	}
	if err := upsertPathsTx(ctx, tx, rule.ID, input.Paths); err != nil {
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

// indexRules / joinPlaceholders / queryStringList / queryInt64List 是 service_quota_repo*.go
// 共享的 SQL 片段拼装与扫描 helper，集中放在主文件里供 scope / subtables 使用。

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

// queryStringList 跑一条 "SELECT 单列 string" 的查询，把所有行收集成 []string。
// 空结果返回 nil 而非空切片（与原 append-style 行为一致），调用方按是否 nil 判定"无数据"。
//
// 抽出公共扫描辅助是为了让多个 repo 方法共享，避免 rows.Next/Scan/append 三段重复代码。
func queryStringList(ctx context.Context, db *sql.DB, query string, args ...any) ([]string, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// queryInt64List 与 queryStringList 同款，目标列为 int64（如 group_id / user_id 等）。
func queryInt64List(ctx context.Context, db *sql.DB, query string, args ...any) ([]int64, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []int64
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
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
