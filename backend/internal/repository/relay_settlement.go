package repository

import (
	"context"
	"database/sql"
	"errors"
	"math"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/shopspring/decimal"
)

func lockRelaySettlementUsers(ctx context.Context, tx *sql.Tx, cmd *service.UsageBillingCommand) error {
	if cmd.SupplierUserID == nil {
		return nil
	}
	if cmd.GroupID == nil || cmd.SupplierRate < 0 || cmd.SupplierRate > 1 || math.IsNaN(cmd.SupplierRate) || math.IsInf(cmd.SupplierRate, 0) {
		return errors.New("invalid relay settlement")
	}
	var valid bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM accounts a JOIN groups g ON g.id = $3
		WHERE a.id = $1 AND a.owner_user_id = $2 AND g.owner_user_id IS NULL
	)`, cmd.AccountID, *cmd.SupplierUserID, *cmd.GroupID).Scan(&valid); err != nil {
		return err
	}
	if !valid {
		return errors.New("relay settlement ownership mismatch")
	}
	// Opposite-direction trades lock both wallets in the same order, including self-use.
	rows, err := tx.QueryContext(ctx, `SELECT id FROM users WHERE id = ANY($1) ORDER BY id FOR UPDATE`,
		pq.Array([]int64{cmd.UserID, *cmd.SupplierUserID}))
	if err != nil {
		return err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	expected := 2
	if cmd.UserID == *cmd.SupplierUserID {
		expected = 1
	}
	if count != expected {
		return service.ErrUserNotFound
	}
	return nil
}

func creditRelaySupplier(ctx context.Context, tx *sql.Tx, cmd *service.UsageBillingCommand, result *service.UsageBillingApplyResult) error {
	if cmd.SupplierUserID == nil || cmd.BalanceCost <= 0 {
		return nil
	}
	amount, _ := decimal.NewFromFloat(cmd.BalanceCost).Mul(decimal.NewFromFloat(cmd.SupplierRate)).Round(service.UsageBillingMonetaryScale).Float64()
	if _, err := tx.ExecContext(ctx, `INSERT INTO relay_earnings
		(request_id, api_key_id, consumer_user_id, supplier_user_id, account_id, group_id, charged_amount, supplier_rate, amount, credited_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,CASE WHEN $10 THEN NOW() ELSE NULL END)`,
		cmd.RequestID, cmd.APIKeyID, cmd.UserID, *cmd.SupplierUserID, cmd.AccountID, *cmd.GroupID,
		cmd.BalanceCost, cmd.SupplierRate, amount, !result.BalanceOverdrafted); err != nil {
		return err
	}
	if result.BalanceOverdrafted {
		return nil
	}
	var balance float64
	if err := tx.QueryRowContext(ctx, `UPDATE users SET balance = balance + $1, updated_at = NOW()
		WHERE id = $2 RETURNING balance`, amount, *cmd.SupplierUserID).Scan(&balance); err != nil {
		return err
	}
	result.SupplierUserID = cmd.SupplierUserID
	result.SupplierAmount = amount
	if cmd.UserID == *cmd.SupplierUserID {
		result.NewBalance = &balance
	}
	return nil
}
