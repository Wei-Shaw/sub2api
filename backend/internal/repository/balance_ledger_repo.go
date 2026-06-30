package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// balanceLedgerEpsilon 用于 decimal(20,8) ↔ float64 比较的容差，避免恰好全额退被浮点误差误判超额。
const balanceLedgerEpsilon = 1e-9

type balanceLedgerRepository struct {
	db *sql.DB
}

// NewBalanceLedgerRepository 创建余额账本仓储（原生 SQL + 事务）。
func NewBalanceLedgerRepository(sqlDB *sql.DB) service.BalanceLedgerRepository {
	return &balanceLedgerRepository{db: sqlDB}
}

func normalizeLedgerExtra(extra string) string {
	if extra == "" {
		return "{}"
	}
	return extra
}

// Deduct 不透支扣费 + (app_id, request_id) 幂等，单事务。
func (r *balanceLedgerRepository) Deduct(ctx context.Context, cmd *service.LedgerDeductCommand) (_ *service.LedgerDeductResult, err error) {
	if r == nil || r.db == nil {
		return nil, errors.New("balance ledger repository db is nil")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	// 1. 幂等占位：插入 deduct 流水；冲突即已处理过。
	var ledgerID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO balance_ledger (request_id, app_id, user_id, kind, amount, refunded_amount, description, extra)
		VALUES ($1, $2, $3, 1, $4, 0, $5, $6::jsonb)
		ON CONFLICT (app_id, request_id) DO NOTHING
		RETURNING id
	`, cmd.RequestID, cmd.AppID, cmd.UserID, cmd.Amount, cmd.Description, normalizeLedgerExtra(cmd.Extra)).Scan(&ledgerID)
	if errors.Is(err, sql.ErrNoRows) {
		// 幂等重放：返回首次结果。
		return r.replayDeduct(ctx, tx, cmd)
	}
	if err != nil {
		return nil, err
	}

	// 2. 不透支扣减。
	var newBalance float64
	err = tx.QueryRowContext(ctx, `
		UPDATE users SET balance = balance - $1, updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL AND balance >= $1
		RETURNING balance
	`, cmd.Amount, cmd.UserID).Scan(&newBalance)
	if errors.Is(err, sql.ErrNoRows) {
		// 区分「用户不存在」与「余额不足」。
		return nil, r.classifyDeductFailure(ctx, tx, cmd.UserID)
	}
	if err != nil {
		return nil, err
	}

	// 3. 回填余额快照。
	if _, err = tx.ExecContext(ctx, `UPDATE balance_ledger SET balance_after = $1 WHERE id = $2`, newBalance, ledgerID); err != nil {
		return nil, err
	}

	if err = tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil
	return &service.LedgerDeductResult{Applied: true, BalanceAfter: newBalance}, nil
}

func (r *balanceLedgerRepository) replayDeduct(ctx context.Context, tx *sql.Tx, cmd *service.LedgerDeductCommand) (*service.LedgerDeductResult, error) {
	var (
		userID       int64
		amount       float64
		balanceAfter sql.NullFloat64
	)
	if err := tx.QueryRowContext(ctx, `
		SELECT user_id, amount, balance_after FROM balance_ledger
		WHERE app_id = $1 AND request_id = $2
	`, cmd.AppID, cmd.RequestID).Scan(&userID, &amount, &balanceAfter); err != nil {
		return nil, err
	}
	// 同一 request_id 必须对应一致的参数，否则视为冲突。
	if userID != cmd.UserID || !floatEqual(amount, cmd.Amount) {
		return nil, service.ErrLedgerRequestConflict
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &service.LedgerDeductResult{Applied: false, BalanceAfter: balanceAfter.Float64}, nil
}

func (r *balanceLedgerRepository) classifyDeductFailure(ctx context.Context, tx *sql.Tx, userID int64) error {
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id = $1 AND deleted_at IS NULL)`, userID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return service.ErrUserNotFound
	}
	return service.ErrBalanceInsufficient
}

// Refund 部分退 + 凭原流水冲销 + (app_id, refund_request_id) 幂等，单事务。
func (r *balanceLedgerRepository) Refund(ctx context.Context, cmd *service.LedgerRefundCommand) (_ *service.LedgerRefundResult, err error) {
	if r == nil || r.db == nil {
		return nil, errors.New("balance ledger repository db is nil")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	// 1. 锁原扣流水（同时串行化并发部分退，防止超额）。
	var (
		origID       int64
		origUserID   int64
		origAmount   float64
		origRefunded float64
	)
	err = tx.QueryRowContext(ctx, `
		SELECT id, user_id, amount, refunded_amount FROM balance_ledger
		WHERE app_id = $1 AND request_id = $2 AND kind = 1
		FOR UPDATE
	`, cmd.AppID, cmd.OriginalRequestID).Scan(&origID, &origUserID, &origAmount, &origRefunded)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrOriginalDeductNotFound
	}
	if err != nil {
		return nil, err
	}

	// 2. 退款幂等占位。
	var refundID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO balance_ledger (request_id, app_id, user_id, kind, amount, refunded_amount, refund_of, description, extra)
		VALUES ($1, $2, $3, 2, $4, 0, $5, $6, $7::jsonb)
		ON CONFLICT (app_id, request_id) DO NOTHING
		RETURNING id
	`, cmd.RefundRequestID, cmd.AppID, origUserID, cmd.Amount, cmd.OriginalRequestID, cmd.Description, normalizeLedgerExtra(cmd.Extra)).Scan(&refundID)
	if errors.Is(err, sql.ErrNoRows) {
		return r.replayRefund(ctx, tx, cmd, origRefunded)
	}
	if err != nil {
		return nil, err
	}

	// 3. 累计退款不得超过原扣金额。
	if origRefunded+cmd.Amount > origAmount+balanceLedgerEpsilon {
		return nil, service.ErrOverRefund
	}

	// 4. 累加原扣已退 + 退回余额。
	if _, err = tx.ExecContext(ctx, `UPDATE balance_ledger SET refunded_amount = refunded_amount + $1, updated_at = NOW() WHERE id = $2`, cmd.Amount, origID); err != nil {
		return nil, err
	}
	var newBalance float64
	err = tx.QueryRowContext(ctx, `
		UPDATE users SET balance = balance + $1, updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
		RETURNING balance
	`, cmd.Amount, origUserID).Scan(&newBalance)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE balance_ledger SET balance_after = $1 WHERE id = $2`, newBalance, refundID); err != nil {
		return nil, err
	}

	if err = tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil
	return &service.LedgerRefundResult{
		Applied:       true,
		UserID:        origUserID,
		BalanceAfter:  newBalance,
		RefundedTotal: origRefunded + cmd.Amount,
	}, nil
}

func (r *balanceLedgerRepository) replayRefund(ctx context.Context, tx *sql.Tx, cmd *service.LedgerRefundCommand, origRefunded float64) (*service.LedgerRefundResult, error) {
	var (
		amount       float64
		balanceAfter sql.NullFloat64
		userID       int64
	)
	if err := tx.QueryRowContext(ctx, `
		SELECT user_id, amount, balance_after FROM balance_ledger
		WHERE app_id = $1 AND request_id = $2 AND kind = 2
	`, cmd.AppID, cmd.RefundRequestID).Scan(&userID, &amount, &balanceAfter); err != nil {
		return nil, err
	}
	if !floatEqual(amount, cmd.Amount) {
		return nil, service.ErrLedgerRequestConflict
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &service.LedgerRefundResult{
		Applied:       false,
		UserID:        userID,
		BalanceAfter:  balanceAfter.Float64,
		RefundedTotal: origRefunded,
	}, nil
}

func floatEqual(a, b float64) bool {
	d := a - b
	return d < balanceLedgerEpsilon && d > -balanceLedgerEpsilon
}

// AppStats 汇总某接入方的累计扣/退（一次 GROUP BY 查询）。
func (r *balanceLedgerRepository) AppStats(ctx context.Context, appID string) (*service.AppLedgerStats, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("balance ledger repository db is nil")
	}
	stats := &service.AppLedgerStats{AppID: appID}
	rows, err := r.db.QueryContext(ctx, `
		SELECT kind, COALESCE(SUM(amount), 0), COUNT(*)
		FROM balance_ledger
		WHERE app_id = $1
		GROUP BY kind
	`, appID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var (
			kind  int8
			sum   float64
			count int64
		)
		if err := rows.Scan(&kind, &sum, &count); err != nil {
			return nil, err
		}
		switch kind {
		case 1: // deduct
			stats.TotalDeducted = sum
			stats.DeductCount = count
		case 2: // refund
			stats.TotalRefunded = sum
			stats.RefundCount = count
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	stats.NetDeducted = stats.TotalDeducted - stats.TotalRefunded
	return stats, nil
}
