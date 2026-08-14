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

func (r *balanceLedgerRepository) FindDeduct(ctx context.Context, cmd *service.LedgerDeductCommand) (*service.LedgerDeductResult, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("balance ledger repository db is nil")
	}
	var (
		userID          int64
		amount          float64
		balanceAfter    sql.NullFloat64
		organizationID  sql.NullInt64
		payerUserID     int64
		balanceSource   string
		authzGeneration sql.NullInt64
	)
	err := r.db.QueryRowContext(ctx, `
		SELECT user_id,amount,balance_after,organization_id,COALESCE(payer_user_id,user_id),
		       COALESCE(balance_source,'self'),authz_generation
		FROM balance_ledger WHERE app_id=$1 AND request_id=$2 AND kind=1`, cmd.AppID, cmd.RequestID).
		Scan(&userID, &amount, &balanceAfter, &organizationID, &payerUserID, &balanceSource, &authzGeneration)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if userID != cmd.UserID || !floatEqual(amount, cmd.Amount) {
		return nil, service.ErrLedgerRequestConflict
	}
	var orgID *int64
	if organizationID.Valid {
		orgID = &organizationID.Int64
	}
	return &service.LedgerDeductResult{
		Applied: false, BalanceAfter: balanceAfter.Float64, OrganizationID: orgID,
		PayerUserID: payerUserID, BalanceSource: balanceSource, AuthzGeneration: authzGeneration.Int64,
	}, nil
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
	payerUserID := cmd.PayerUserID
	if payerUserID == 0 {
		payerUserID = cmd.UserID
	}
	balanceSource := cmd.BalanceSource
	if balanceSource == "" {
		balanceSource = service.BalanceSourceSelf
	}
	if balanceSource == service.BalanceSourceCompany && (cmd.OrganizationID == nil || *cmd.OrganizationID <= 0) {
		return nil, service.ErrCompanyNotFound
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
		INSERT INTO balance_ledger (request_id, app_id, user_id, kind, amount, refunded_amount, description, extra,
			organization_id,payer_user_id,balance_source,authz_generation)
		VALUES ($1, $2, $3, 1, $4, 0, $5, $6::jsonb,$7,$8,$9,$10)
		ON CONFLICT (app_id, request_id) DO NOTHING
		RETURNING id
	`, cmd.RequestID, cmd.AppID, cmd.UserID, cmd.Amount, cmd.Description, normalizeLedgerExtra(cmd.Extra), cmd.OrganizationID, payerUserID, balanceSource, cmd.AuthzGeneration).Scan(&ledgerID)
	if errors.Is(err, sql.ErrNoRows) {
		// 幂等重放：返回首次结果。
		return r.replayDeduct(ctx, tx, cmd)
	}
	if err != nil {
		return nil, err
	}

	// 2. 扣减余额。企业钱包允许一次性透支；普通用户和 IAM 划拨余额必须
	// 保持不透支，避免账本扣款绕过余额预检。IAM 划拨余额不足时由上层
	// BillingContextResolver 选择企业钱包作为扣款来源。
	var newBalance float64
	if balanceSource == service.BalanceSourceCompany && cmd.OrganizationID != nil && *cmd.OrganizationID > 0 {
		err = tx.QueryRowContext(ctx, `
			UPDATE organizations SET balance = balance - $1, updated_at = NOW()
			WHERE id = $2
			RETURNING balance
		`, cmd.Amount, *cmd.OrganizationID).Scan(&newBalance)
	} else {
		err = tx.QueryRowContext(ctx, `
			UPDATE users SET balance = balance - $1, updated_at = NOW()
			WHERE id = $2 AND deleted_at IS NULL
			  AND balance >= $1
			RETURNING balance
		`, cmd.Amount, payerUserID).Scan(&newBalance)
	}
	if errors.Is(err, sql.ErrNoRows) {
		// IAM 划拨余额不足时，将整笔扣款切换到企业钱包。这里必须在
		// 同一事务内完成，避免预检看到正余额后结算阶段丢失扣款。
		if balanceSource == service.BalanceSourceAllocated && cmd.OrganizationID != nil && *cmd.OrganizationID > 0 {
			var companyBalance float64
			var ownerUserID int64
			companyErr := tx.QueryRowContext(ctx, `
				UPDATE organizations
				SET balance = balance - $1, updated_at = NOW()
				WHERE id = $2
				RETURNING balance, owner_user_id
			`, cmd.Amount, *cmd.OrganizationID).Scan(&companyBalance, &ownerUserID)
			if companyErr == nil {
				if _, companyErr = tx.ExecContext(ctx, `
					UPDATE balance_ledger
					SET payer_user_id=$1, balance_source=$2, balance_after=$3
					WHERE id=$4
				`, ownerUserID, service.BalanceSourceCompany, companyBalance, ledgerID); companyErr != nil {
					return nil, companyErr
				}
				if companyErr = tx.Commit(); companyErr != nil {
					return nil, companyErr
				}
				tx = nil
				orgID := *cmd.OrganizationID
				return &service.LedgerDeductResult{
					Applied: true, BalanceAfter: companyBalance, OrganizationID: &orgID,
					PayerUserID: ownerUserID, BalanceSource: service.BalanceSourceCompany,
					AuthzGeneration: cmd.AuthzGeneration,
				}, nil
			}
			if !errors.Is(companyErr, sql.ErrNoRows) {
				return nil, companyErr
			}
			return nil, r.classifyOrganizationDeductFailure(ctx, tx, *cmd.OrganizationID)
		}
		if balanceSource == service.BalanceSourceCompany && cmd.OrganizationID != nil && *cmd.OrganizationID > 0 {
			return nil, r.classifyOrganizationDeductFailure(ctx, tx, *cmd.OrganizationID)
		}
		// 记录不存在（用户被删除等异常）。
		return nil, r.classifyDeductFailure(ctx, tx, payerUserID)
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
	return &service.LedgerDeductResult{Applied: true, BalanceAfter: newBalance, OrganizationID: cmd.OrganizationID, PayerUserID: payerUserID, BalanceSource: balanceSource, AuthzGeneration: cmd.AuthzGeneration}, nil
}

func (r *balanceLedgerRepository) replayDeduct(ctx context.Context, tx *sql.Tx, cmd *service.LedgerDeductCommand) (*service.LedgerDeductResult, error) {
	var (
		userID          int64
		amount          float64
		balanceAfter    sql.NullFloat64
		organizationID  sql.NullInt64
		payerUserID     int64
		balanceSource   string
		authzGeneration sql.NullInt64
	)
	if err := tx.QueryRowContext(ctx, `
		SELECT user_id, amount, balance_after, organization_id, COALESCE(payer_user_id,user_id), COALESCE(balance_source,'self'), authz_generation FROM balance_ledger
		WHERE app_id = $1 AND request_id = $2
	`, cmd.AppID, cmd.RequestID).Scan(&userID, &amount, &balanceAfter, &organizationID, &payerUserID, &balanceSource, &authzGeneration); err != nil {
		return nil, err
	}
	// 同一 request_id 必须对应一致的参数，否则视为冲突。
	if userID != cmd.UserID || !floatEqual(amount, cmd.Amount) {
		return nil, service.ErrLedgerRequestConflict
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	var orgID *int64
	if organizationID.Valid {
		orgID = &organizationID.Int64
	}
	return &service.LedgerDeductResult{Applied: false, BalanceAfter: balanceAfter.Float64, OrganizationID: orgID, PayerUserID: payerUserID, BalanceSource: balanceSource, AuthzGeneration: authzGeneration.Int64}, nil
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

func (r *balanceLedgerRepository) classifyOrganizationDeductFailure(ctx context.Context, tx *sql.Tx, organizationID int64) error {
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM organizations WHERE id = $1)`, organizationID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return service.ErrCompanyNotFound
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
		origID              int64
		origUserID          int64
		origAmount          float64
		origRefunded        float64
		origOrganizationID  sql.NullInt64
		origPayerUserID     int64
		origBalanceSource   string
		origAuthzGeneration sql.NullInt64
	)
	err = tx.QueryRowContext(ctx, `
		SELECT id, user_id, amount, refunded_amount, organization_id, COALESCE(payer_user_id,user_id), COALESCE(balance_source,'self'), authz_generation FROM balance_ledger
		WHERE app_id = $1 AND request_id = $2 AND kind = 1
		FOR UPDATE
	`, cmd.AppID, cmd.OriginalRequestID).Scan(&origID, &origUserID, &origAmount, &origRefunded, &origOrganizationID, &origPayerUserID, &origBalanceSource, &origAuthzGeneration)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrOriginalDeductNotFound
	}
	if err != nil {
		return nil, err
	}

	// 2. 退款幂等占位。
	var refundID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO balance_ledger (request_id, app_id, user_id, kind, amount, refunded_amount, refund_of, description, extra,
			organization_id,payer_user_id,balance_source,authz_generation)
		VALUES ($1, $2, $3, 2, $4, 0, $5, $6, $7::jsonb,$8,$9,$10,$11)
		ON CONFLICT (app_id, request_id) DO NOTHING
		RETURNING id
	`, cmd.RefundRequestID, cmd.AppID, origUserID, cmd.Amount, cmd.OriginalRequestID, cmd.Description, normalizeLedgerExtra(cmd.Extra), nullInt64Value(origOrganizationID), origPayerUserID, origBalanceSource, nullInt64Value(origAuthzGeneration)).Scan(&refundID)
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
	if origBalanceSource == service.BalanceSourceCompany && !origOrganizationID.Valid {
		return nil, service.ErrCompanyNotFound
	}
	if origBalanceSource == service.BalanceSourceCompany {
		err = tx.QueryRowContext(ctx, `
			UPDATE organizations SET balance = balance + $1, updated_at = NOW()
			WHERE id = $2
			RETURNING balance
		`, cmd.Amount, origOrganizationID.Int64).Scan(&newBalance)
	} else {
		err = tx.QueryRowContext(ctx, `
			UPDATE users SET balance = balance + $1, updated_at = NOW()
			WHERE id = $2 AND deleted_at IS NULL
			RETURNING balance
		`, cmd.Amount, origPayerUserID).Scan(&newBalance)
	}
	if errors.Is(err, sql.ErrNoRows) {
		if origBalanceSource == service.BalanceSourceCompany {
			return nil, service.ErrCompanyNotFound
		}
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
	var organizationID *int64
	if origOrganizationID.Valid {
		organizationID = &origOrganizationID.Int64
	}
	return &service.LedgerRefundResult{
		Applied:         true,
		UserID:          origPayerUserID,
		PayerUserID:     origPayerUserID,
		BalanceAfter:    newBalance,
		RefundedTotal:   origRefunded + cmd.Amount,
		OrganizationID:  organizationID,
		BalanceSource:   origBalanceSource,
		AuthzGeneration: origAuthzGeneration.Int64,
	}, nil
}

func nullInt64Value(value sql.NullInt64) any {
	if value.Valid {
		return value.Int64
	}
	return nil
}

func (r *balanceLedgerRepository) replayRefund(ctx context.Context, tx *sql.Tx, cmd *service.LedgerRefundCommand, origRefunded float64) (*service.LedgerRefundResult, error) {
	var (
		amount          float64
		balanceAfter    sql.NullFloat64
		userID          int64
		organizationID  sql.NullInt64
		payerUserID     int64
		balanceSource   string
		authzGeneration sql.NullInt64
	)
	if err := tx.QueryRowContext(ctx, `
		SELECT user_id, amount, balance_after, organization_id, COALESCE(payer_user_id,user_id), COALESCE(balance_source,'self'), authz_generation FROM balance_ledger
		WHERE app_id = $1 AND request_id = $2 AND kind = 2
	`, cmd.AppID, cmd.RefundRequestID).Scan(&userID, &amount, &balanceAfter, &organizationID, &payerUserID, &balanceSource, &authzGeneration); err != nil {
		return nil, err
	}
	if !floatEqual(amount, cmd.Amount) {
		return nil, service.ErrLedgerRequestConflict
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	var orgID *int64
	if organizationID.Valid {
		orgID = &organizationID.Int64
	}
	return &service.LedgerRefundResult{
		Applied:         false,
		UserID:          payerUserID,
		PayerUserID:     payerUserID,
		BalanceAfter:    balanceAfter.Float64,
		RefundedTotal:   origRefunded,
		OrganizationID:  orgID,
		BalanceSource:   balanceSource,
		AuthzGeneration: authzGeneration.Int64,
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
