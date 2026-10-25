package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// --- Plan Repository ---

type scheduledTestPlanRepository struct {
	db *sql.DB
}

func NewScheduledTestPlanRepository(db *sql.DB) service.ScheduledTestPlanRepository {
	return &scheduledTestPlanRepository{db: db}
}

func (r *scheduledTestPlanRepository) Create(ctx context.Context, plan *service.ScheduledTestPlan) (*service.ScheduledTestPlan, error) {
	if IsMySQL() {
		return r.createPlanMySQL(ctx, plan)
	}
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO scheduled_test_plans (account_id, model_id, cron_expression, enabled, max_results, auto_recover, next_run_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
		RETURNING id, account_id, model_id, cron_expression, enabled, max_results, auto_recover, last_run_at, next_run_at, created_at, updated_at
	`, plan.AccountID, plan.ModelID, plan.CronExpression, plan.Enabled, plan.MaxResults, plan.AutoRecover, plan.NextRunAt)
	return scanPlan(row)
}

func (r *scheduledTestPlanRepository) GetByID(ctx context.Context, id int64) (*service.ScheduledTestPlan, error) {
	if IsMySQL() {
		return r.getByIDMySQL(ctx, id)
	}
	row := r.db.QueryRowContext(ctx, `
		SELECT id, account_id, model_id, cron_expression, enabled, max_results, auto_recover, last_run_at, next_run_at, created_at, updated_at
		FROM scheduled_test_plans WHERE id = $1
	`, id)
	return scanPlan(row)
}

func (r *scheduledTestPlanRepository) ListByAccountID(ctx context.Context, accountID int64) ([]*service.ScheduledTestPlan, error) {
	if IsMySQL() {
		return r.listByAccountIDMySQL(ctx, accountID)
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, account_id, model_id, cron_expression, enabled, max_results, auto_recover, last_run_at, next_run_at, created_at, updated_at
		FROM scheduled_test_plans WHERE account_id = $1
		ORDER BY created_at DESC
	`, accountID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanPlans(rows)
}

func (r *scheduledTestPlanRepository) ListDue(ctx context.Context, now time.Time) ([]*service.ScheduledTestPlan, error) {
	if IsMySQL() {
		return r.listDueMySQL(ctx, now)
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, account_id, model_id, cron_expression, enabled, max_results, auto_recover, last_run_at, next_run_at, created_at, updated_at
		FROM scheduled_test_plans
		WHERE enabled = true AND next_run_at <= $1
		ORDER BY next_run_at ASC
	`, now)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanPlans(rows)
}

func (r *scheduledTestPlanRepository) Update(ctx context.Context, plan *service.ScheduledTestPlan) (*service.ScheduledTestPlan, error) {
	if IsMySQL() {
		return r.updatePlanMySQL(ctx, plan)
	}
	row := r.db.QueryRowContext(ctx, `
		UPDATE scheduled_test_plans
		SET model_id = $2, cron_expression = $3, enabled = $4, max_results = $5, auto_recover = $6, next_run_at = $7, updated_at = NOW()
		WHERE id = $1
		RETURNING id, account_id, model_id, cron_expression, enabled, max_results, auto_recover, last_run_at, next_run_at, created_at, updated_at
	`, plan.ID, plan.ModelID, plan.CronExpression, plan.Enabled, plan.MaxResults, plan.AutoRecover, plan.NextRunAt)
	return scanPlan(row)
}

func (r *scheduledTestPlanRepository) Delete(ctx context.Context, id int64) error {
	if IsMySQL() {
		return r.deletePlanMySQL(ctx, id)
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM scheduled_test_plans WHERE id = $1`, id)
	return err
}

func (r *scheduledTestPlanRepository) UpdateAfterRun(ctx context.Context, id int64, lastRunAt time.Time, nextRunAt time.Time) error {
	if IsMySQL() {
		return r.updateAfterRunMySQL(ctx, id, lastRunAt, nextRunAt)
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE scheduled_test_plans SET last_run_at = $2, next_run_at = $3, updated_at = NOW() WHERE id = $1
	`, id, lastRunAt, nextRunAt)
	return err
}

// --- Result Repository ---

type scheduledTestResultRepository struct {
	db *sql.DB
}

func NewScheduledTestResultRepository(db *sql.DB) service.ScheduledTestResultRepository {
	return &scheduledTestResultRepository{db: db}
}

func (r *scheduledTestResultRepository) Create(ctx context.Context, result *service.ScheduledTestResult) (*service.ScheduledTestResult, error) {
	if IsMySQL() {
		return r.createResultMySQL(ctx, result)
	}
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO scheduled_test_results (plan_id, status, response_text, error_message, latency_ms, started_at, finished_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		RETURNING id, plan_id, status, response_text, error_message, latency_ms, started_at, finished_at, created_at
	`, result.PlanID, result.Status, result.ResponseText, result.ErrorMessage, result.LatencyMs, result.StartedAt, result.FinishedAt)

	out := &service.ScheduledTestResult{}
	if err := row.Scan(
		&out.ID, &out.PlanID, &out.Status, &out.ResponseText, &out.ErrorMessage,
		&out.LatencyMs, &out.StartedAt, &out.FinishedAt, &out.CreatedAt,
	); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *scheduledTestResultRepository) ListByPlanID(ctx context.Context, planID int64, limit int) ([]*service.ScheduledTestResult, error) {
	if IsMySQL() {
		return r.listByPlanIDMySQL(ctx, planID, limit)
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, plan_id, status, response_text, error_message, latency_ms, started_at, finished_at, created_at
		FROM scheduled_test_results
		WHERE plan_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, planID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var results []*service.ScheduledTestResult
	for rows.Next() {
		r := &service.ScheduledTestResult{}
		if err := rows.Scan(
			&r.ID, &r.PlanID, &r.Status, &r.ResponseText, &r.ErrorMessage,
			&r.LatencyMs, &r.StartedAt, &r.FinishedAt, &r.CreatedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (r *scheduledTestResultRepository) PruneOldResults(ctx context.Context, planID int64, keepCount int) error {
	if IsMySQL() {
		return r.pruneOldResultsMySQL(ctx, planID, keepCount)
	}
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM scheduled_test_results
		WHERE id IN (
			SELECT id FROM (
				SELECT id, ROW_NUMBER() OVER (PARTITION BY plan_id ORDER BY created_at DESC) AS rn
				FROM scheduled_test_results
				WHERE plan_id = $1
			) ranked
			WHERE rn > $2
		)
	`, planID, keepCount)
	return err
}

// --- scan helpers ---

type scannable interface {
	Scan(dest ...any) error
}

func scanPlan(row scannable) (*service.ScheduledTestPlan, error) {
	p := &service.ScheduledTestPlan{}
	if err := row.Scan(
		&p.ID, &p.AccountID, &p.ModelID, &p.CronExpression, &p.Enabled, &p.MaxResults, &p.AutoRecover,
		&p.LastRunAt, &p.NextRunAt, &p.CreatedAt, &p.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return p, nil
}

func scanPlans(rows *sql.Rows) ([]*service.ScheduledTestPlan, error) {
	var plans []*service.ScheduledTestPlan
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		plans = append(plans, p)
	}
	return plans, rows.Err()
}

// ---------------------------------------------------------------------------
// MySQL 方言实现（见 DIALECT_MYSQL_GUIDE.md）。PG 分支行为与字节保持原样。
// ---------------------------------------------------------------------------

// scheduledTestPlanSelectMySQL 是 MySQL 分支的计划行查询前缀（列清单与 PG 版逐字一致）。
const scheduledTestPlanSelectMySQL = `
		SELECT id, account_id, model_id, cron_expression, enabled, max_results, auto_recover, last_run_at, next_run_at, created_at, updated_at
		FROM scheduled_test_plans`

// scheduledTestResultSelectMySQL 是 MySQL 分支的结果行查询前缀（列清单与 PG 版逐字一致）。
const scheduledTestResultSelectMySQL = `
		SELECT id, plan_id, status, response_text, error_message, latency_ms, started_at, finished_at, created_at
		FROM scheduled_test_results`

// createPlanMySQL 是 Create 的 MySQL 实现：PG 版 INSERT..RETURNING 全行 →
// 同一事务内 INSERT + LastInsertId + 按主键回查（created_at/updated_at 由数据库
// NOW(3) 生成，应用侧未知；last_run_at 建行时为 NULL）。
func (r *scheduledTestPlanRepository) createPlanMySQL(ctx context.Context, plan *service.ScheduledTestPlan) (*service.ScheduledTestPlan, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
		INSERT INTO scheduled_test_plans (account_id, model_id, cron_expression, enabled, max_results, auto_recover, next_run_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, NOW(3), NOW(3))
	`, plan.AccountID, plan.ModelID, plan.CronExpression, plan.Enabled, plan.MaxResults, plan.AutoRecover, plan.NextRunAt)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	created, err := scanPlan(tx.QueryRowContext(ctx, scheduledTestPlanSelectMySQL+` WHERE id = ?`, id))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return created, nil
}

// getByIDMySQL 是 GetByID 的 MySQL 实现（$1→?）。
func (r *scheduledTestPlanRepository) getByIDMySQL(ctx context.Context, id int64) (*service.ScheduledTestPlan, error) {
	return scanPlan(r.db.QueryRowContext(ctx, scheduledTestPlanSelectMySQL+` WHERE id = ?`, id))
}

// listByAccountIDMySQL 是 ListByAccountID 的 MySQL 实现（$1→?）。
func (r *scheduledTestPlanRepository) listByAccountIDMySQL(ctx context.Context, accountID int64) ([]*service.ScheduledTestPlan, error) {
	rows, err := r.db.QueryContext(ctx, scheduledTestPlanSelectMySQL+`
		WHERE account_id = ?
		ORDER BY created_at DESC
	`, accountID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanPlans(rows)
}

// listDueMySQL 是 ListDue 的 MySQL 实现（$1→?；enabled = true 两边通用）。
func (r *scheduledTestPlanRepository) listDueMySQL(ctx context.Context, now time.Time) ([]*service.ScheduledTestPlan, error) {
	rows, err := r.db.QueryContext(ctx, scheduledTestPlanSelectMySQL+`
		WHERE enabled = true AND next_run_at <= ?
		ORDER BY next_run_at ASC
	`, now)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanPlans(rows)
}

// updatePlanMySQL 是 Update 的 MySQL 实现：PG 版 UPDATE..RETURNING 全行（更新后值）→
// 同一事务内 UPDATE + 按主键回查。回查无行 → sql.ErrNoRows，与 PG 的 RETURNING 空行行为一致。
// REVIEW: 不依赖 RowsAffected 判定——MySQL RowsAffected 语义为"值变化的行"而 PG 为
// "匹配的行"，行匹配但写入值完全相同时两者不一致；恒回查在两种情况下都返回该行当前值。
func (r *scheduledTestPlanRepository) updatePlanMySQL(ctx context.Context, plan *service.ScheduledTestPlan) (*service.ScheduledTestPlan, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		UPDATE scheduled_test_plans
		SET model_id = ?, cron_expression = ?, enabled = ?, max_results = ?, auto_recover = ?, next_run_at = ?, updated_at = NOW(3)
		WHERE id = ?
	`, plan.ModelID, plan.CronExpression, plan.Enabled, plan.MaxResults, plan.AutoRecover, plan.NextRunAt, plan.ID); err != nil {
		return nil, err
	}
	updated, err := scanPlan(tx.QueryRowContext(ctx, scheduledTestPlanSelectMySQL+` WHERE id = ?`, plan.ID))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return updated, nil
}

// deletePlanMySQL 是 Delete 的 MySQL 实现（$1→?）。
func (r *scheduledTestPlanRepository) deletePlanMySQL(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM scheduled_test_plans WHERE id = ?`, id)
	return err
}

// updateAfterRunMySQL 是 UpdateAfterRun 的 MySQL 实现（$n→?，NOW()→NOW(3)）。
func (r *scheduledTestPlanRepository) updateAfterRunMySQL(ctx context.Context, id int64, lastRunAt time.Time, nextRunAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE scheduled_test_plans SET last_run_at = ?, next_run_at = ?, updated_at = NOW(3) WHERE id = ?
	`, lastRunAt, nextRunAt, id)
	return err
}

// createResultMySQL 是 Create 的 MySQL 实现：PG 版 INSERT..RETURNING 全行 →
// 同一事务内 INSERT + LastInsertId + 按主键回查（created_at 由数据库 NOW(3) 生成）。
func (r *scheduledTestResultRepository) createResultMySQL(ctx context.Context, result *service.ScheduledTestResult) (*service.ScheduledTestResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
		INSERT INTO scheduled_test_results (plan_id, status, response_text, error_message, latency_ms, started_at, finished_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, NOW(3))
	`, result.PlanID, result.Status, result.ResponseText, result.ErrorMessage, result.LatencyMs, result.StartedAt, result.FinishedAt)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	out := &service.ScheduledTestResult{}
	row := tx.QueryRowContext(ctx, scheduledTestResultSelectMySQL+` WHERE id = ?`, id)
	if err := row.Scan(
		&out.ID, &out.PlanID, &out.Status, &out.ResponseText, &out.ErrorMessage,
		&out.LatencyMs, &out.StartedAt, &out.FinishedAt, &out.CreatedAt,
	); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

// listByPlanIDMySQL 是 ListByPlanID 的 MySQL 实现（$n→?）。
func (r *scheduledTestResultRepository) listByPlanIDMySQL(ctx context.Context, planID int64, limit int) ([]*service.ScheduledTestResult, error) {
	rows, err := r.db.QueryContext(ctx, scheduledTestResultSelectMySQL+`
		WHERE plan_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, planID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var results []*service.ScheduledTestResult
	for rows.Next() {
		r := &service.ScheduledTestResult{}
		if err := rows.Scan(
			&r.ID, &r.PlanID, &r.Status, &r.ResponseText, &r.ErrorMessage,
			&r.LatencyMs, &r.StartedAt, &r.FinishedAt, &r.CreatedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// pruneOldResultsMySQL 是 PruneOldResults 的 MySQL 实现（$n→?）。
// 窗口函数与派生表包裹在 MySQL 8 原生支持（派生表正是绕开
// "DELETE 目标表不能直接出现在子查询中"限制的惯用写法）。
func (r *scheduledTestResultRepository) pruneOldResultsMySQL(ctx context.Context, planID int64, keepCount int) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM scheduled_test_results
		WHERE id IN (
			SELECT id FROM (
				SELECT id, ROW_NUMBER() OVER (PARTITION BY plan_id ORDER BY created_at DESC) AS rn
				FROM scheduled_test_results
				WHERE plan_id = ?
			) ranked
			WHERE rn > ?
		)
	`, planID, keepCount)
	return err
}
