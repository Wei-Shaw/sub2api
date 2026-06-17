package repository

import (
	"context"
	"database/sql"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type balancePackageRepository struct {
	client *dbent.Client
}

func NewBalancePackageRepository(client *dbent.Client) service.BalancePackageRepository {
	return &balancePackageRepository{client: client}
}

func (r *balancePackageRepository) CreateBalancePackage(ctx context.Context, pkg *service.UserBalancePackage) error {
	client := clientFromContext(ctx, r.client)
	now := time.Now().UTC()
	if pkg.Status == "" {
		pkg.Status = service.BalancePackageStatusActive
	}
	if pkg.CreatedAt.IsZero() {
		pkg.CreatedAt = now
	}
	if pkg.UpdatedAt.IsZero() {
		pkg.UpdatedAt = now
	}
	if pkg.RemainingAmount == 0 && pkg.Amount > 0 {
		pkg.RemainingAmount = pkg.Amount
	}
	rows, err := client.QueryContext(ctx, `
INSERT INTO user_balance_packages
	(user_id, redeem_code_id, amount, remaining_amount, expires_at, status, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, created_at, updated_at
`, pkg.UserID, pkg.RedeemCodeID, pkg.Amount, pkg.RemainingAmount, pkg.ExpiresAt, pkg.Status, pkg.CreatedAt, pkg.UpdatedAt)
	if err != nil {
		return err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return err
		}
		return service.ErrBalancePackageNotFound
	}
	return rows.Scan(&pkg.ID, &pkg.CreatedAt, &pkg.UpdatedAt)
}

func (r *balancePackageRepository) ListActiveBalancePackagesForUpdate(ctx context.Context, userID int64, now time.Time) ([]service.UserBalancePackage, error) {
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx, `
SELECT id, user_id, redeem_code_id, amount, remaining_amount, expires_at, status, created_at, updated_at
FROM user_balance_packages
WHERE user_id = $1
  AND status = $2
  AND remaining_amount > 0
  AND expires_at > $3
ORDER BY expires_at ASC, id ASC
FOR UPDATE
`, userID, service.BalancePackageStatusActive, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]service.UserBalancePackage, 0)
	for rows.Next() {
		var pkg service.UserBalancePackage
		var redeemCodeID sql.NullInt64
		if err := rows.Scan(
			&pkg.ID,
			&pkg.UserID,
			&redeemCodeID,
			&pkg.Amount,
			&pkg.RemainingAmount,
			&pkg.ExpiresAt,
			&pkg.Status,
			&pkg.CreatedAt,
			&pkg.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if redeemCodeID.Valid {
			pkg.RedeemCodeID = &redeemCodeID.Int64
		}
		out = append(out, pkg)
	}
	return out, rows.Err()
}

func (r *balancePackageRepository) ListUserVisibleBalancePackages(ctx context.Context, userID int64, now time.Time) ([]service.UserBalancePackage, error) {
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx, `
SELECT p.id, p.user_id, p.redeem_code_id, COALESCE(rc.code, ''), p.amount, p.remaining_amount, p.expires_at, p.status, p.created_at, p.updated_at
FROM user_balance_packages p
LEFT JOIN redeem_codes rc ON rc.id = p.redeem_code_id
WHERE p.user_id = $1
  AND p.status = $2
  AND p.remaining_amount > 0
  AND p.expires_at > $3
ORDER BY p.expires_at ASC, p.id ASC
`, userID, service.BalancePackageStatusActive, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]service.UserBalancePackage, 0)
	for rows.Next() {
		var pkg service.UserBalancePackage
		var redeemCodeID sql.NullInt64
		if err := rows.Scan(
			&pkg.ID,
			&pkg.UserID,
			&redeemCodeID,
			&pkg.RedeemCode,
			&pkg.Amount,
			&pkg.RemainingAmount,
			&pkg.ExpiresAt,
			&pkg.Status,
			&pkg.CreatedAt,
			&pkg.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if redeemCodeID.Valid {
			pkg.RedeemCodeID = &redeemCodeID.Int64
		}
		out = append(out, pkg)
	}
	return out, rows.Err()
}

func (r *balancePackageRepository) UpdateBalancePackageRemaining(ctx context.Context, id int64, remaining float64, status string) error {
	client := clientFromContext(ctx, r.client)
	result, err := client.ExecContext(ctx, `
UPDATE user_balance_packages
SET remaining_amount = $2, status = $3, updated_at = NOW()
WHERE id = $1
`, id, remaining, status)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err == nil && n == 0 {
		return service.ErrBalancePackageNotFound
	}
	return nil
}

func (r *balancePackageRepository) GetUserBaseBalance(ctx context.Context, userID int64) (float64, error) {
	client := clientFromContext(ctx, r.client)
	var balance float64
	rows, err := client.QueryContext(ctx, `SELECT balance FROM users WHERE id = $1`, userID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return 0, err
		}
		return 0, service.ErrUserNotFound
	}
	if err := rows.Scan(&balance); err != nil {
		return 0, err
	}
	return balance, nil
}

func (r *balancePackageRepository) UpdateUserBalance(ctx context.Context, userID int64, amount float64) error {
	client := clientFromContext(ctx, r.client)
	result, err := client.ExecContext(ctx, `
UPDATE users
SET balance = balance + $2, updated_at = NOW()
WHERE id = $1
`, userID, amount)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err == nil && n == 0 {
		return service.ErrUserNotFound
	}
	return nil
}

func (r *balancePackageRepository) SumActiveBalancePackages(ctx context.Context, userID int64, now time.Time) (float64, error) {
	client := clientFromContext(ctx, r.client)
	var sum float64
	rows, err := client.QueryContext(ctx, `
SELECT COALESCE(SUM(remaining_amount), 0)
FROM user_balance_packages
WHERE user_id = $1
  AND status = $2
  AND remaining_amount > 0
  AND expires_at > $3
`, userID, service.BalancePackageStatusActive, now)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	if rows.Next() {
		if err := rows.Scan(&sum); err != nil {
			return 0, err
		}
	}
	return sum, rows.Err()
}
