package service

import (
	"context"
	"database/sql"
	"errors"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const DingTalkManagerPageSize = 20

func (s *DingTalkOrganizationService) ManagerPage(ctx context.Context, page int) ([]DingTalkManager, int64, error) {
	if page < 1 || page > 1000000 {
		return nil, 0, infraerrors.BadRequest("INVALID_PAGE", "Invalid page")
	}
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM dingtalk_manager_budgets b JOIN users u ON u.id=b.user_id WHERE u.deleted_at IS NULL`).Scan(&total); err != nil {
		return nil, 0, err
	}
	managers, err := s.listManagers(ctx, 0, true, ` LIMIT $3 OFFSET $4`, []any{DingTalkManagerPageSize, (page - 1) * DingTalkManagerPageSize})
	return managers, total, err
}

type DingTalkManagerPatch struct {
	Enabled            *bool                        `json:"enabled"`
	LimitCents         *int64                       `json:"limit_cents"`
	ExpectedLimitCents *int64                       `json:"expected_limit_cents"`
	Departments        *[]DingTalkManagerDepartment `json:"departments"`
}

// Patching permissions never writes a budget from an older browser snapshot.
func (s *DingTalkOrganizationService) PatchManager(ctx context.Context, id int64, p DingTalkManagerPatch) error {
	if id <= 0 || (p.Enabled == nil && p.LimitCents == nil && p.Departments == nil) {
		return infraerrors.BadRequest("INVALID_MANAGER", "No manager changes supplied")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var limit, used int64
	err = tx.QueryRowContext(ctx, `SELECT limit_cents,used_cents FROM dingtalk_manager_budgets WHERE user_id=$1 FOR UPDATE`, id).Scan(&limit, &used)
	if errors.Is(err, sql.ErrNoRows) {
		return infraerrors.NotFound("MANAGER_NOT_FOUND", "Manager not found")
	}
	if err != nil {
		return err
	}
	if p.LimitCents != nil {
		if p.ExpectedLimitCents == nil || *p.ExpectedLimitCents != limit {
			return infraerrors.Conflict("MANAGER_BUDGET_CHANGED", "Budget changed; refresh the manager before editing")
		}
		if *p.LimitCents < used || *p.LimitCents > maxDingTalkBudgetCents {
			return infraerrors.BadRequest("INVALID_QUOTA", "Budget must be at least the amount already allocated and at most 1,000,000,000")
		}
		if _, err = tx.ExecContext(ctx, `UPDATE dingtalk_manager_budgets SET limit_cents=$2,updated_at=NOW() WHERE user_id=$1`, id, *p.LimitCents); err != nil {
			return err
		}
	}
	if p.Enabled != nil {
		if _, err = tx.ExecContext(ctx, `UPDATE dingtalk_manager_budgets SET enabled=$2,updated_at=NOW() WHERE user_id=$1`, id, *p.Enabled); err != nil {
			return err
		}
	}
	if p.Departments != nil {
		if err = replaceDingTalkManagerDepartments(ctx, tx, id, *p.Departments); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type DingTalkBudgetIncrease struct {
	ID              int64     `json:"id"`
	ManagerID       int64     `json:"manager_id"`
	AmountCents     int64     `json:"amount_cents"`
	LimitCentsAfter int64     `json:"limit_cents_after"`
	CreatedAt       time.Time `json:"created_at"`
}

// Serialize increments and persist their request IDs, so concurrent administrators
// and network retries cannot lose or duplicate budget increases.
func (s *DingTalkOrganizationService) IncreaseManagerBudget(ctx context.Context, actor, id, amount int64, request string) (*DingTalkBudgetIncrease, error) {
	if actor <= 0 || id <= 0 || amount <= 0 || amount > maxDingTalkBudgetCents || len(request) < 16 || len(request) > 64 {
		return nil, infraerrors.BadRequest("INVALID_QUOTA", "Invalid amount, manager or request ID")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('dingtalk-budget:' || $1))`, actor); err != nil {
		return nil, err
	}
	var result DingTalkBudgetIncrease
	err = tx.QueryRowContext(ctx, `SELECT id,manager_id,amount_cents,limit_cents_after,created_at FROM dingtalk_manager_budget_increases WHERE actor_id=$1 AND request_id=$2`, actor, request).Scan(&result.ID, &result.ManagerID, &result.AmountCents, &result.LimitCentsAfter, &result.CreatedAt)
	if err == nil {
		if result.ManagerID != id || result.AmountCents != amount {
			return nil, infraerrors.Conflict("BUDGET_REQUEST_CONFLICT", "Request ID was already used for another increase")
		}
		return &result, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	result.ManagerID, result.AmountCents = id, amount
	// PostgreSQL resolves the WHERE expression before the assignment, so the
	// subtraction needs explicit types even though limit_cents is a BIGINT.
	err = tx.QueryRowContext(ctx, `UPDATE dingtalk_manager_budgets b SET limit_cents=limit_cents+$2::bigint,updated_at=NOW() WHERE user_id=$1 AND limit_cents<=$3::bigint-$2::bigint AND EXISTS(SELECT 1 FROM users u WHERE u.id=b.user_id AND u.deleted_at IS NULL AND u.status='active' AND u.role<>'admin') RETURNING limit_cents`, id, amount, maxDingTalkBudgetCents).Scan(&result.LimitCentsAfter)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, infraerrors.BadRequest("INVALID_MANAGER_BUDGET", "Manager is unavailable or the resulting budget exceeds 1,000,000,000")
	}
	if err != nil {
		return nil, err
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO dingtalk_manager_budget_increases(actor_id,manager_id,amount_cents,limit_cents_after,request_id) VALUES($1,$2,$3,$4,$5) RETURNING id,created_at`, actor, id, amount, result.LimitCentsAfter, request).Scan(&result.ID, &result.CreatedAt)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *DingTalkOrganizationService) ManagerGrants(ctx context.Context, id int64, page int) ([]DingTalkQuotaGrant, int64, error) {
	if id <= 0 || page < 1 || page > 1000000 {
		return nil, 0, infraerrors.BadRequest("INVALID_PAGE", "Invalid manager or page")
	}
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM dingtalk_quota_grants WHERE actor_id=$1`, id).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,actor_id,target_id,app_id,department_id,amount_cents,request_id,created_at FROM dingtalk_quota_grants WHERE actor_id=$1 ORDER BY id DESC LIMIT $2 OFFSET $3`, id, DingTalkManagerPageSize, (page-1)*DingTalkManagerPageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := []DingTalkQuotaGrant{}
	for rows.Next() {
		var g DingTalkQuotaGrant
		if err = rows.Scan(&g.ID, &g.ActorID, &g.TargetID, &g.AppID, &g.DepartmentID, &g.AmountCents, &g.RequestID, &g.CreatedAt); err != nil {
			return nil, 0, err
		}
		result = append(result, g)
	}
	return result, total, rows.Err()
}
