package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/go-sql-driver/mysql"
	"github.com/lib/pq"
)

const (
	affiliateCodeLength      = 12
	affiliateCodeMaxAttempts = 12
)

var affiliateCodeCharset = []byte("ABCDEFGHJKLMNPQRSTUVWXYZ23456789")

const affiliateUserOverviewSQL = `
SELECT ua.user_id,
       COALESCE(u.email, ''),
       COALESCE(u.username, ''),
       ua.aff_code,
       COALESCE(ua.aff_rebate_rate_percent, 0)::double precision,
       (ua.aff_rebate_rate_percent IS NOT NULL) AS has_custom_rate,
       ua.aff_count,
       COALESCE(rebated.rebated_invitee_count, 0),
       (ua.aff_quota + COALESCE(matured.matured_frozen_quota, 0))::double precision,
       ua.aff_history_quota::double precision
FROM user_affiliates ua
JOIN users u ON u.id = ua.user_id
LEFT JOIN (
    SELECT user_id, COUNT(DISTINCT source_user_id)::integer AS rebated_invitee_count
    FROM user_affiliate_ledger
    WHERE action = 'accrue' AND source_user_id IS NOT NULL
    GROUP BY user_id
) rebated ON rebated.user_id = ua.user_id
LEFT JOIN (
    SELECT user_id, COALESCE(SUM(amount), 0)::double precision AS matured_frozen_quota
    FROM user_affiliate_ledger
    WHERE action = 'accrue' AND frozen_until IS NOT NULL AND frozen_until <= NOW()
    GROUP BY user_id
) matured ON matured.user_id = ua.user_id
WHERE ua.user_id = $1
LIMIT 1`

// affiliateUserOverviewSQLMySQL 是 affiliateUserOverviewSQL 的 MySQL 版本：
// 去掉 ::double precision / ::integer 显式转换（DECIMAL/INT 结果列由 database/sql
// 自动转换到 float64/int）；NOW() → NOW(3) 与 DATETIME(3) 毫秒精度对齐。
const affiliateUserOverviewSQLMySQL = `
SELECT ua.user_id,
       COALESCE(u.email, ''),
       COALESCE(u.username, ''),
       ua.aff_code,
       COALESCE(ua.aff_rebate_rate_percent, 0),
       (ua.aff_rebate_rate_percent IS NOT NULL) AS has_custom_rate,
       ua.aff_count,
       COALESCE(rebated.rebated_invitee_count, 0),
       (ua.aff_quota + COALESCE(matured.matured_frozen_quota, 0)),
       ua.aff_history_quota
FROM user_affiliates ua
JOIN users u ON u.id = ua.user_id
LEFT JOIN (
    SELECT user_id, COUNT(DISTINCT source_user_id) AS rebated_invitee_count
    FROM user_affiliate_ledger
    WHERE action = 'accrue' AND source_user_id IS NOT NULL
    GROUP BY user_id
) rebated ON rebated.user_id = ua.user_id
LEFT JOIN (
    SELECT user_id, COALESCE(SUM(amount), 0) AS matured_frozen_quota
    FROM user_affiliate_ledger
    WHERE action = 'accrue' AND frozen_until IS NOT NULL AND frozen_until <= NOW(3)
    GROUP BY user_id
) matured ON matured.user_id = ua.user_id
WHERE ua.user_id = ?
LIMIT 1`

type affiliateQueryExecer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type affiliateRepository struct {
	client *dbent.Client
}

func NewAffiliateRepository(client *dbent.Client, _ *sql.DB) service.AffiliateRepository {
	return &affiliateRepository{client: client}
}

func (r *affiliateRepository) EnsureUserAffiliate(ctx context.Context, userID int64) (*service.AffiliateSummary, error) {
	if userID <= 0 {
		return nil, service.ErrUserNotFound
	}
	client := clientFromContext(ctx, r.client)
	return ensureUserAffiliateWithClient(ctx, client, userID)
}

func (r *affiliateRepository) GetAffiliateByCode(ctx context.Context, code string) (*service.AffiliateSummary, error) {
	client := clientFromContext(ctx, r.client)
	return queryAffiliateByCode(ctx, client, code)
}

func (r *affiliateRepository) BindInviter(ctx context.Context, userID, inviterID int64) (bool, error) {
	var bound bool
	err := r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		if _, err := ensureUserAffiliateWithClient(txCtx, txClient, userID); err != nil {
			return err
		}
		if _, err := ensureUserAffiliateWithClient(txCtx, txClient, inviterID); err != nil {
			return err
		}

		bindInviterSQL := "UPDATE user_affiliates SET inviter_id = $1, updated_at = NOW() WHERE user_id = $2 AND inviter_id IS NULL"
		if IsMySQL() {
			bindInviterSQL = "UPDATE user_affiliates SET inviter_id = ?, updated_at = NOW(3) WHERE user_id = ? AND inviter_id IS NULL"
		}
		res, err := txClient.ExecContext(txCtx, bindInviterSQL, inviterID, userID)
		if err != nil {
			return fmt.Errorf("bind inviter: %w", err)
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			bound = false
			return nil
		}

		incCountSQL := "UPDATE user_affiliates SET aff_count = aff_count + 1, updated_at = NOW() WHERE user_id = $1"
		if IsMySQL() {
			incCountSQL = "UPDATE user_affiliates SET aff_count = aff_count + 1, updated_at = NOW(3) WHERE user_id = ?"
		}
		if _, err = txClient.ExecContext(txCtx, incCountSQL, inviterID); err != nil {
			return fmt.Errorf("increment inviter aff_count: %w", err)
		}
		bound = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return bound, nil
}

func (r *affiliateRepository) AccrueQuota(ctx context.Context, inviterID, inviteeUserID int64, amount float64, freezeHours int, sourceOrderID *int64) (bool, error) {
	if amount <= 0 {
		return false, nil
	}

	var applied bool
	err := r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		// freezeHours > 0: add to frozen quota; == 0: add to available quota directly
		// 注意：PG 复用 $1 传 2 个参数；MySQL 的 ? 不支持复用，需传 3 个参数。
		var updateSQL string
		updateArgs := []any{amount, inviterID}
		if freezeHours > 0 {
			updateSQL = "UPDATE user_affiliates SET aff_frozen_quota = aff_frozen_quota + $1, aff_history_quota = aff_history_quota + $1, updated_at = NOW() WHERE user_id = $2"
			if IsMySQL() {
				updateSQL = "UPDATE user_affiliates SET aff_frozen_quota = aff_frozen_quota + ?, aff_history_quota = aff_history_quota + ?, updated_at = NOW(3) WHERE user_id = ?"
				updateArgs = []any{amount, amount, inviterID}
			}
		} else {
			updateSQL = "UPDATE user_affiliates SET aff_quota = aff_quota + $1, aff_history_quota = aff_history_quota + $1, updated_at = NOW() WHERE user_id = $2"
			if IsMySQL() {
				updateSQL = "UPDATE user_affiliates SET aff_quota = aff_quota + ?, aff_history_quota = aff_history_quota + ?, updated_at = NOW(3) WHERE user_id = ?"
				updateArgs = []any{amount, amount, inviterID}
			}
		}
		res, err := txClient.ExecContext(txCtx, updateSQL, updateArgs...)
		if err != nil {
			return err
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			applied = false
			return nil
		}

		if freezeHours > 0 {
			freezeLedgerSQL := `
INSERT INTO user_affiliate_ledger (user_id, action, amount, source_user_id, source_order_id, frozen_until, created_at, updated_at)
VALUES ($1, 'accrue', $2, $3, $4, NOW() + make_interval(hours => $5), NOW(), NOW())`
			if IsMySQL() {
				// make_interval(hours => $5) → DATE_ADD(NOW(3), INTERVAL ? HOUR)
				freezeLedgerSQL = `
INSERT INTO user_affiliate_ledger (user_id, action, amount, source_user_id, source_order_id, frozen_until, created_at, updated_at)
VALUES (?, 'accrue', ?, ?, ?, DATE_ADD(NOW(3), INTERVAL ? HOUR), NOW(3), NOW(3))`
			}
			if _, err = txClient.ExecContext(txCtx, freezeLedgerSQL,
				inviterID, amount, inviteeUserID, nullableInt64Arg(sourceOrderID), freezeHours); err != nil {
				return fmt.Errorf("insert affiliate accrue ledger: %w", err)
			}
		} else {
			accrueLedgerSQL := `
INSERT INTO user_affiliate_ledger (user_id, action, amount, source_user_id, source_order_id, created_at, updated_at)
VALUES ($1, 'accrue', $2, $3, $4, NOW(), NOW())`
			if IsMySQL() {
				accrueLedgerSQL = `
INSERT INTO user_affiliate_ledger (user_id, action, amount, source_user_id, source_order_id, created_at, updated_at)
VALUES (?, 'accrue', ?, ?, ?, NOW(3), NOW(3))`
			}
			if _, err = txClient.ExecContext(txCtx, accrueLedgerSQL, inviterID, amount, inviteeUserID, nullableInt64Arg(sourceOrderID)); err != nil {
				return fmt.Errorf("insert affiliate accrue ledger: %w", err)
			}
		}

		applied = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return applied, nil
}

func (r *affiliateRepository) GetAccruedRebateFromInvitee(ctx context.Context, inviterID, inviteeUserID int64) (float64, error) {
	client := clientFromContext(ctx, r.client)
	// PG 的 SUM(amount)::double precision 在 MySQL 侧无需显式转换：DECIMAL 结果列
	// 由 database/sql 自动转换为 float64（见 DIALECT_MYSQL_GUIDE.md）。
	query := `SELECT COALESCE(SUM(amount), 0)::double precision FROM user_affiliate_ledger WHERE user_id = $1 AND source_user_id = $2 AND action = 'accrue'`
	if IsMySQL() {
		query = `SELECT COALESCE(SUM(amount), 0) FROM user_affiliate_ledger WHERE user_id = ? AND source_user_id = ? AND action = 'accrue'`
	}
	rows, err := client.QueryContext(ctx, query, inviterID, inviteeUserID)
	if err != nil {
		return 0, fmt.Errorf("query accrued rebate from invitee: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var total float64
	if rows.Next() {
		if err := rows.Scan(&total); err != nil {
			return 0, err
		}
	}
	return total, rows.Close()
}

func (r *affiliateRepository) ThawFrozenQuota(ctx context.Context, userID int64) (float64, error) {
	var thawed float64
	err := r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		var err error
		thawed, err = thawFrozenQuotaTx(txCtx, txClient, userID)
		return err
	})
	return thawed, err
}

// thawFrozenQuotaTx moves matured frozen quota to available quota within an existing tx.
func thawFrozenQuotaTx(txCtx context.Context, txClient *dbent.Client, userID int64) (float64, error) {
	var thawed float64
	if IsMySQL() {
		// PG 版是可写 CTE（UPDATE..RETURNING amount 后 SUM）；MySQL 无可写 CTE，
		// 按 DIALECT_MYSQL_GUIDE.md 用 sqlAtomicScope 拆为同事务多语句：
		// 先 FOR UPDATE 锁定并读取到期流水的 amount 集合，再以相同条件 UPDATE 清冻结。
		var err error
		thawed, err = thawMaturedFrozenLedgerMySQL(txCtx, txClient, userID)
		if err != nil {
			return 0, err
		}
	} else {
		rows, err := txClient.QueryContext(txCtx, `
WITH matured AS (
    UPDATE user_affiliate_ledger
    SET frozen_until = NULL, updated_at = NOW()
    WHERE user_id = $1
      AND frozen_until IS NOT NULL
      AND frozen_until <= NOW()
    RETURNING amount
)
SELECT COALESCE(SUM(amount), 0) FROM matured`, userID)
		if err != nil {
			return 0, fmt.Errorf("thaw frozen quota: %w", err)
		}
		defer func() { _ = rows.Close() }()

		if rows.Next() {
			if err := rows.Scan(&thawed); err != nil {
				return 0, err
			}
		}
		if err := rows.Close(); err != nil {
			return 0, err
		}
	}
	if thawed <= 0 {
		return 0, nil
	}

	// 注意：PG 复用 $1 传 2 个参数；MySQL 的 ? 不支持复用，需传 3 个参数。
	moveQuotaSQL := `
UPDATE user_affiliates
SET aff_quota = aff_quota + $1,
    aff_frozen_quota = GREATEST(aff_frozen_quota - $1, 0),
    updated_at = NOW()
WHERE user_id = $2`
	moveQuotaArgs := []any{thawed, userID}
	if IsMySQL() {
		moveQuotaSQL = `
UPDATE user_affiliates
SET aff_quota = aff_quota + ?,
    aff_frozen_quota = GREATEST(aff_frozen_quota - ?, 0),
    updated_at = NOW(3)
WHERE user_id = ?`
		moveQuotaArgs = []any{thawed, thawed, userID}
	}
	_, err := txClient.ExecContext(txCtx, moveQuotaSQL, moveQuotaArgs...)
	if err != nil {
		return 0, fmt.Errorf("move thawed quota: %w", err)
	}
	return thawed, nil
}

// thawMaturedFrozenLedgerMySQL 对应 PG 版 thawFrozenQuotaTx 中可写 CTE 的语义：
// SELECT ... FOR UPDATE 读取到期流水的 amount（求和），再以相同条件 UPDATE 清除
// frozen_until。行集合先锁定后写，UPDATE 命中的行与求和的行严格一致。
func thawMaturedFrozenLedgerMySQL(txCtx context.Context, txClient *dbent.Client, userID int64) (float64, error) {
	scope, err := beginSQLAtomicScope(txCtx, txClient, nil)
	if err != nil {
		return 0, err
	}
	defer scope.rollback()

	rows, err := scope.query(txCtx, `
SELECT amount
FROM user_affiliate_ledger
WHERE user_id = ?
  AND frozen_until IS NOT NULL
  AND frozen_until <= NOW(3)
FOR UPDATE`, userID)
	if err != nil {
		return 0, fmt.Errorf("thaw frozen quota: %w", err)
	}
	var thawed float64
	matured := false
	for rows.Next() {
		var amount float64
		if err := rows.Scan(&amount); err != nil {
			_ = rows.Close()
			return 0, err
		}
		thawed += amount
		matured = true
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if !matured {
		// PG 版 COALESCE(SUM(...), 0)=0 时 UPDATE 不命中任何行（也无后续额度搬移）。
		return 0, nil
	}
	// PG 版单条语句即使 SUM(amount)=0 也会清除命中行的冻结标记，这里保持一致。
	if _, err := scope.exec(txCtx, `
UPDATE user_affiliate_ledger
SET frozen_until = NULL, updated_at = NOW(3)
WHERE user_id = ?
  AND frozen_until IS NOT NULL
  AND frozen_until <= NOW(3)`, userID); err != nil {
		return 0, fmt.Errorf("thaw frozen quota: %w", err)
	}
	return thawed, nil
}

func (r *affiliateRepository) TransferQuotaToBalance(ctx context.Context, userID int64) (float64, float64, error) {
	var transferred float64
	var newBalance float64

	err := r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		if _, err := ensureUserAffiliateWithClient(txCtx, txClient, userID); err != nil {
			return err
		}

		// Thaw any matured frozen quota before transfer.
		if _, err := thawFrozenQuotaTx(txCtx, txClient, userID); err != nil {
			return fmt.Errorf("thaw before transfer: %w", err)
		}

		if IsMySQL() {
			// PG 版是可写 CTE（claimed FOR UPDATE + cleared UPDATE..RETURNING c.amount）；
			// MySQL 拆为同事务「FOR UPDATE 锁定读 aff_quota → 置 0」，先取值即为转移额。
			var err error
			transferred, err = claimAffiliateQuotaMySQL(txCtx, txClient, userID)
			if err != nil {
				return err
			}
		} else {
			rows, err := txClient.QueryContext(txCtx, `
WITH claimed AS (
	SELECT aff_quota::double precision AS amount
	FROM user_affiliates
	WHERE user_id = $1
	  AND aff_quota > 0
	FOR UPDATE
),
cleared AS (
	UPDATE user_affiliates ua
	SET aff_quota = 0,
	    updated_at = NOW()
	FROM claimed c
	WHERE ua.user_id = $1
	RETURNING c.amount
)
SELECT amount
FROM cleared`, userID)
			if err != nil {
				return fmt.Errorf("claim affiliate quota: %w", err)
			}

			if !rows.Next() {
				_ = rows.Close()
				if err := rows.Err(); err != nil {
					return err
				}
				return service.ErrAffiliateQuotaEmpty
			}
			if err := rows.Scan(&transferred); err != nil {
				_ = rows.Close()
				return err
			}
			if err := rows.Close(); err != nil {
				return err
			}
		}
		if transferred <= 0 {
			return service.ErrAffiliateQuotaEmpty
		}

		affected, err := txClient.User.Update().
			Where(user.IDEQ(userID)).
			AddBalance(transferred).
			AddTotalRecharged(transferred).
			Save(txCtx)
		if err != nil {
			return fmt.Errorf("credit user balance by affiliate quota: %w", err)
		}
		if affected == 0 {
			return service.ErrUserNotFound
		}

		newBalance, err = queryUserBalance(txCtx, txClient, userID)
		if err != nil {
			return err
		}

		snapshot, err := queryAffiliateTransferSnapshot(txCtx, txClient, userID)
		if err != nil {
			return err
		}

		transferLedgerSQL := `
INSERT INTO user_affiliate_ledger (
    user_id,
    action,
    amount,
    source_user_id,
    balance_after,
    aff_quota_after,
    aff_frozen_quota_after,
    aff_history_quota_after,
    created_at,
    updated_at
)
VALUES ($1, 'transfer', $2, NULL, $3, $4, $5, $6, NOW(), NOW())`
		if IsMySQL() {
			transferLedgerSQL = `
INSERT INTO user_affiliate_ledger (
    user_id,
    action,
    amount,
    source_user_id,
    balance_after,
    aff_quota_after,
    aff_frozen_quota_after,
    aff_history_quota_after,
    created_at,
    updated_at
)
VALUES (?, 'transfer', ?, NULL, ?, ?, ?, ?, NOW(3), NOW(3))`
		}
		if _, err = txClient.ExecContext(txCtx, transferLedgerSQL,
			userID,
			transferred,
			snapshot.BalanceAfter,
			snapshot.AvailableQuotaAfter,
			snapshot.FrozenQuotaAfter,
			snapshot.HistoryQuotaAfter,
		); err != nil {
			return fmt.Errorf("insert affiliate transfer ledger: %w", err)
		}

		return nil
	})
	if err != nil {
		return 0, 0, err
	}

	return transferred, newBalance, nil
}

// WithdrawQuota 登记一笔线下提现。operationID 标识一次登记：同一事务内先以
// operation_id 唯一约束写入占位流水，同标识的流水已存在时不重复扣减，用户与金额
// 一致则返回该流水记录的结果（Replayed=true），不一致返回
// ErrIdempotencyKeyConflict。占位成功后先解冻已到期的冻结额度，再以
// aff_quota >= amount 为条件原子扣减并补写额度快照。可提取额度不足或用户没有
// 返利档案时返回 ErrAffiliateQuotaInsufficient，占位随事务回滚，不占用该标识。
func (r *affiliateRepository) WithdrawQuota(ctx context.Context, userID int64, amount float64, operationID string) (*service.AffiliateWithdrawResult, error) {
	if userID <= 0 {
		return nil, service.ErrUserNotFound
	}
	if amount <= 0 {
		return nil, service.ErrAffiliateWithdrawAmountInvalid
	}
	if operationID == "" {
		return nil, service.ErrIdempotencyKeyRequired
	}

	var result *service.AffiliateWithdrawResult
	err := r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		ledgerID, claimed, err := claimAffiliateWithdrawLedger(txCtx, txClient, userID, amount, operationID)
		if err != nil {
			return err
		}
		if !claimed {
			result, err = findAffiliateWithdrawByOperation(txCtx, txClient, userID, amount, operationID)
			return err
		}

		if _, err := thawFrozenQuotaTx(txCtx, txClient, userID); err != nil {
			return fmt.Errorf("thaw before withdraw: %w", err)
		}

		// 注意：PG 复用 $1 传 2 个参数；MySQL 的 ? 不支持复用，需传 3 个参数。
		deductSQL := `
UPDATE user_affiliates
SET aff_quota = aff_quota - $1,
    updated_at = NOW()
WHERE user_id = $2
  AND aff_quota >= $1`
		deductArgs := []any{amount, userID}
		if IsMySQL() {
			deductSQL = `
UPDATE user_affiliates
SET aff_quota = aff_quota - ?,
    updated_at = NOW(3)
WHERE user_id = ?
  AND aff_quota >= ?`
			deductArgs = []any{amount, userID, amount}
		}
		res, err := txClient.ExecContext(txCtx, deductSQL, deductArgs...)
		if err != nil {
			return fmt.Errorf("deduct affiliate quota: %w", err)
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("deduct affiliate quota: %w", err)
		}
		if affected == 0 {
			return service.ErrAffiliateQuotaInsufficient
		}

		snapshot, err := queryAffiliateTransferSnapshot(txCtx, txClient, userID)
		if err != nil {
			return err
		}

		recordSnapshotSQL := `
UPDATE user_affiliate_ledger
SET balance_after = $1,
    aff_quota_after = $2,
    aff_frozen_quota_after = $3,
    aff_history_quota_after = $4,
    updated_at = NOW()
WHERE id = $5`
		if IsMySQL() {
			recordSnapshotSQL = `
UPDATE user_affiliate_ledger
SET balance_after = ?,
    aff_quota_after = ?,
    aff_frozen_quota_after = ?,
    aff_history_quota_after = ?,
    updated_at = NOW(3)
WHERE id = ?`
		}
		if _, err := txClient.ExecContext(txCtx, recordSnapshotSQL,
			snapshot.BalanceAfter,
			snapshot.AvailableQuotaAfter,
			snapshot.FrozenQuotaAfter,
			snapshot.HistoryQuotaAfter,
			ledgerID,
		); err != nil {
			return fmt.Errorf("record affiliate withdraw snapshot: %w", err)
		}

		result = &service.AffiliateWithdrawResult{
			LedgerID:            ledgerID,
			UserID:              userID,
			Amount:              amount,
			AvailableQuotaAfter: snapshot.AvailableQuotaAfter,
			FrozenQuotaAfter:    snapshot.FrozenQuotaAfter,
			HistoryQuotaAfter:   snapshot.HistoryQuotaAfter,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// claimAffiliateWithdrawLedger 为有返利档案的用户写入一条带 operation_id 的
// withdraw 占位流水。同标识的流水已存在或用户没有返利档案时 claimed 为 false。
// 同标识的并发事务在唯一索引上等待先写入者结束：先写入者提交后返回 claimed=false，
// 回滚后本事务继续写入。
func claimAffiliateWithdrawLedger(ctx context.Context, client affiliateQueryExecer, userID int64, amount float64, operationID string) (int64, bool, error) {
	if IsMySQL() {
		return claimAffiliateWithdrawLedgerMySQL(ctx, client, userID, amount, operationID)
	}
	rows, err := client.QueryContext(ctx, `
INSERT INTO user_affiliate_ledger (user_id, action, amount, operation_id, created_at, updated_at)
SELECT ua.user_id, 'withdraw', $2, $3, NOW(), NOW()
FROM user_affiliates ua
WHERE ua.user_id = $1
ON CONFLICT (operation_id) WHERE operation_id IS NOT NULL DO NOTHING
RETURNING id`, userID, amount, operationID)
	if err != nil {
		return 0, false, fmt.Errorf("claim affiliate withdraw ledger: %w", err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return 0, false, fmt.Errorf("claim affiliate withdraw ledger: %w", err)
		}
		return 0, false, nil
	}
	var ledgerID int64
	if err := rows.Scan(&ledgerID); err != nil {
		return 0, false, err
	}
	return ledgerID, true, rows.Close()
}

// findAffiliateWithdrawByOperation 读取 operation_id 已对应的线下提现流水，
// 用户与金额一致时返回该流水记录的登记结果（Replayed=true），不一致返回
// ErrIdempotencyKeyConflict；没有对应流水说明用户没有返利档案，返回
// ErrAffiliateQuotaInsufficient。
func findAffiliateWithdrawByOperation(ctx context.Context, client affiliateQueryExecer, userID int64, amount float64, operationID string) (*service.AffiliateWithdrawResult, error) {
	query := `
SELECT id,
       user_id = $2 AND action = 'withdraw' AND amount = CAST($3 AS DECIMAL(20,8)),
       user_id,
       amount::double precision,
       COALESCE(aff_quota_after, 0)::double precision,
       COALESCE(aff_frozen_quota_after, 0)::double precision,
       COALESCE(aff_history_quota_after, 0)::double precision
FROM user_affiliate_ledger
WHERE operation_id = $1`
	args := []any{operationID, userID, amount}
	if IsMySQL() {
		// 尾部 FOR UPDATE 是语义必需：MySQL 默认 REPEATABLE READ 下，本事务的
		// 一致性快照在 claim 的存在性 SELECT 处已建立，ODKU 在唯一索引上等待
		// 先写入者提交后，普通快照读仍看不到那条新提交的流水（会误报
		// ErrAffiliateQuotaInsufficient）。锁定读读取最新已提交版本，与 PG
		// READ COMMITTED 下后续普通 SELECT 可见的语义一致；本事务随即返回并
		// 提交，锁持有窗口极短。
		query = `
SELECT id,
       user_id = ? AND action = 'withdraw' AND amount = CAST(? AS DECIMAL(20,8)),
       user_id,
       amount,
       COALESCE(aff_quota_after, 0),
       COALESCE(aff_frozen_quota_after, 0),
       COALESCE(aff_history_quota_after, 0)
FROM user_affiliate_ledger
WHERE operation_id = ?
FOR UPDATE`
		args = []any{userID, amount, operationID}
	}
	rows, err := client.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query affiliate withdraw by operation: %w", err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("query affiliate withdraw by operation: %w", err)
		}
		return nil, service.ErrAffiliateQuotaInsufficient
	}
	var sameRequest bool
	result := &service.AffiliateWithdrawResult{Replayed: true}
	if err := rows.Scan(
		&result.LedgerID,
		&sameRequest,
		&result.UserID,
		&result.Amount,
		&result.AvailableQuotaAfter,
		&result.FrozenQuotaAfter,
		&result.HistoryQuotaAfter,
	); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if !sameRequest {
		return nil, service.ErrIdempotencyKeyConflict
	}
	return result, nil
}

func (r *affiliateRepository) ListInvitees(ctx context.Context, inviterID int64, limit int) ([]service.AffiliateInvitee, error) {
	if limit <= 0 {
		limit = 100
	}
	client := clientFromContext(ctx, r.client)
	listInviteesSQL := `
SELECT ua.user_id,
       COALESCE(u.email, ''),
       COALESCE(u.username, ''),
       ua.created_at,
       COALESCE(SUM(ual.amount), 0)::double precision AS total_rebate
FROM user_affiliates ua
LEFT JOIN users u ON u.id = ua.user_id
LEFT JOIN user_affiliate_ledger ual
       ON ual.user_id = $1
      AND ual.source_user_id = ua.user_id
      AND ual.action = 'accrue'
WHERE ua.inviter_id = $1
GROUP BY ua.user_id, u.email, u.username, ua.created_at
ORDER BY ua.created_at DESC
LIMIT $2`
	if IsMySQL() {
		listInviteesSQL = `
SELECT ua.user_id,
       COALESCE(u.email, ''),
       COALESCE(u.username, ''),
       ua.created_at,
       COALESCE(SUM(ual.amount), 0) AS total_rebate
FROM user_affiliates ua
LEFT JOIN users u ON u.id = ua.user_id
LEFT JOIN user_affiliate_ledger ual
       ON ual.user_id = ?
      AND ual.source_user_id = ua.user_id
      AND ual.action = 'accrue'
WHERE ua.inviter_id = ?
GROUP BY ua.user_id, u.email, u.username, ua.created_at
ORDER BY ua.created_at DESC
LIMIT ?`
	}
	rows, err := client.QueryContext(ctx, listInviteesSQL, inviterID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	invitees := make([]service.AffiliateInvitee, 0)
	for rows.Next() {
		var item service.AffiliateInvitee
		var createdAt time.Time
		if err := rows.Scan(&item.UserID, &item.Email, &item.Username, &createdAt, &item.TotalRebate); err != nil {
			return nil, err
		}
		item.CreatedAt = &createdAt
		invitees = append(invitees, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return invitees, nil
}

func (r *affiliateRepository) ListAffiliateInviteRecords(ctx context.Context, filter service.AffiliateRecordFilter) ([]service.AffiliateInviteRecord, int64, error) {
	client := clientFromContext(ctx, r.client)
	searchColumns := []string{
		"inviter.email", "inviter.username", "invitee.email", "invitee.username",
		"ua.inviter_id::text", "ua.user_id::text", "inviter_aff.aff_code",
	}
	if IsMySQL() {
		// x::text → CAST(x AS CHAR)：管理端按 ID 字符串搜索的口径不变。
		searchColumns = []string{
			"inviter.email", "inviter.username", "invitee.email", "invitee.username",
			"CAST(ua.inviter_id AS CHAR)", "CAST(ua.user_id AS CHAR)", "inviter_aff.aff_code",
		}
	}
	where, args := buildAffiliateRecordWhere(filter, "ua.created_at", searchColumns)

	total, err := queryAffiliateRecordCount(ctx, client, `
SELECT COUNT(*)
FROM user_affiliates ua
JOIN users invitee ON invitee.id = ua.user_id
JOIN users inviter ON inviter.id = ua.inviter_id
JOIN user_affiliates inviter_aff ON inviter_aff.user_id = ua.inviter_id
`+where, args...)
	if err != nil {
		return nil, 0, err
	}

	orderBy := buildAffiliateRecordOrderBy(filter, map[string]string{
		"inviter":      "inviter.email",
		"invitee":      "invitee.email",
		"aff_code":     "inviter_aff.aff_code",
		"total_rebate": "total_rebate",
		"created_at":   "ua.created_at",
	}, "ua.created_at")
	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)
	listQuery := `
SELECT ua.inviter_id,
       COALESCE(inviter.email, ''),
       COALESCE(inviter.username, ''),
       ua.user_id,
       COALESCE(invitee.email, ''),
       COALESCE(invitee.username, ''),
       COALESCE(inviter_aff.aff_code, ''),
       COALESCE(SUM(ual.amount), 0)::double precision AS total_rebate,
       ua.created_at
FROM user_affiliates ua
JOIN users invitee ON invitee.id = ua.user_id
JOIN users inviter ON inviter.id = ua.inviter_id
JOIN user_affiliates inviter_aff ON inviter_aff.user_id = ua.inviter_id
LEFT JOIN user_affiliate_ledger ual
       ON ual.user_id = ua.inviter_id
      AND ual.source_user_id = ua.user_id
      AND ual.action = 'accrue'
` + where + `
GROUP BY ua.inviter_id, inviter.email, inviter.username, ua.user_id, invitee.email, invitee.username, inviter_aff.aff_code, ua.created_at
` + orderBy + `
LIMIT $` + fmt.Sprint(len(args)-1) + ` OFFSET $` + fmt.Sprint(len(args))
	if IsMySQL() {
		listQuery = `
SELECT ua.inviter_id,
       COALESCE(inviter.email, ''),
       COALESCE(inviter.username, ''),
       ua.user_id,
       COALESCE(invitee.email, ''),
       COALESCE(invitee.username, ''),
       COALESCE(inviter_aff.aff_code, ''),
       COALESCE(SUM(ual.amount), 0) AS total_rebate,
       ua.created_at
FROM user_affiliates ua
JOIN users invitee ON invitee.id = ua.user_id
JOIN users inviter ON inviter.id = ua.inviter_id
JOIN user_affiliates inviter_aff ON inviter_aff.user_id = ua.inviter_id
LEFT JOIN user_affiliate_ledger ual
       ON ual.user_id = ua.inviter_id
      AND ual.source_user_id = ua.user_id
      AND ual.action = 'accrue'
` + where + `
GROUP BY ua.inviter_id, inviter.email, inviter.username, ua.user_id, invitee.email, invitee.username, inviter_aff.aff_code, ua.created_at
` + orderBy + `
LIMIT ? OFFSET ?`
	}
	rows, err := client.QueryContext(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.AffiliateInviteRecord, 0)
	for rows.Next() {
		var item service.AffiliateInviteRecord
		if err := rows.Scan(
			&item.InviterID,
			&item.InviterEmail,
			&item.InviterUsername,
			&item.InviteeID,
			&item.InviteeEmail,
			&item.InviteeUsername,
			&item.AffCode,
			&item.TotalRebate,
			&item.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *affiliateRepository) ListAffiliateRebateRecords(ctx context.Context, filter service.AffiliateRecordFilter) ([]service.AffiliateRebateRecord, int64, error) {
	client := clientFromContext(ctx, r.client)
	searchColumns := []string{
		"inviter.email", "inviter.username", "invitee.email", "invitee.username",
		"po.id::text", "po.out_trade_no", "po.payment_type", "po.status",
	}
	if IsMySQL() {
		searchColumns = []string{
			"inviter.email", "inviter.username", "invitee.email", "invitee.username",
			"CAST(po.id AS CHAR)", "po.out_trade_no", "po.payment_type", "po.status",
		}
	}
	where, args := buildAffiliateRecordWhere(filter, "ual.created_at", searchColumns)
	// 返利记录列出全部 accrue 流水：非订单来源的流水没有订单，被邀请人账号
	// 删除后 source_user_id 为 NULL，这两类行都保留。
	baseJoin := `
FROM user_affiliate_ledger ual
JOIN users inviter ON inviter.id = ual.user_id
LEFT JOIN users invitee ON invitee.id = ual.source_user_id
LEFT JOIN payment_orders po ON po.id = ual.source_order_id
WHERE ual.action = 'accrue'`
	if where != "" {
		where = strings.Replace(where, "WHERE ", " AND ", 1)
	}

	total, err := queryAffiliateRecordCount(ctx, client, "SELECT COUNT(*) "+baseJoin+where, args...)
	if err != nil {
		return nil, 0, err
	}

	orderBy := buildAffiliateRecordOrderBy(filter, map[string]string{
		"order":         "po.id",
		"inviter":       "inviter.email",
		"invitee":       "invitee.email",
		"order_amount":  "po.amount",
		"pay_amount":    "po.pay_amount",
		"rebate_amount": "ual.amount",
		"payment_type":  "po.payment_type",
		"order_status":  "po.status",
		"created_at":    "ual.created_at",
	}, "ual.created_at")
	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)
	listQuery := `
SELECT po.id,
       COALESCE(po.out_trade_no, ''),
       ual.user_id,
       COALESCE(inviter.email, ''),
       COALESCE(inviter.username, ''),
       ual.source_user_id,
       COALESCE(invitee.email, ''),
       COALESCE(invitee.username, ''),
       po.amount::double precision,
       po.pay_amount::double precision,
       ual.amount::double precision,
       COALESCE(po.payment_type, ''),
       COALESCE(po.status, ''),
       ual.created_at
` + baseJoin + where + `
` + orderBy + `
LIMIT $` + fmt.Sprint(len(args)-1) + ` OFFSET $` + fmt.Sprint(len(args))
	if IsMySQL() {
		listQuery = `
SELECT po.id,
       COALESCE(po.out_trade_no, ''),
       ual.user_id,
       COALESCE(inviter.email, ''),
       COALESCE(inviter.username, ''),
       ual.source_user_id,
       COALESCE(invitee.email, ''),
       COALESCE(invitee.username, ''),
       po.amount,
       po.pay_amount,
       ual.amount,
       COALESCE(po.payment_type, ''),
       COALESCE(po.status, ''),
       ual.created_at
` + baseJoin + where + `
` + orderBy + `
LIMIT ? OFFSET ?`
	}
	rows, err := client.QueryContext(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.AffiliateRebateRecord, 0)
	for rows.Next() {
		var item service.AffiliateRebateRecord
		var orderID sql.NullInt64
		var inviteeID sql.NullInt64
		var orderAmount sql.NullFloat64
		var payAmount sql.NullFloat64
		if err := rows.Scan(
			&orderID,
			&item.OutTradeNo,
			&item.InviterID,
			&item.InviterEmail,
			&item.InviterUsername,
			&inviteeID,
			&item.InviteeEmail,
			&item.InviteeUsername,
			&orderAmount,
			&payAmount,
			&item.RebateAmount,
			&item.PaymentType,
			&item.OrderStatus,
			&item.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		item.OrderID = nullableInt64Ptr(orderID)
		item.InviteeID = nullableInt64Ptr(inviteeID)
		item.OrderAmount = nullableFloat64Ptr(orderAmount)
		item.PayAmount = nullableFloat64Ptr(payAmount)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *affiliateRepository) ListAffiliateTransferRecords(ctx context.Context, filter service.AffiliateRecordFilter) ([]service.AffiliateTransferRecord, int64, error) {
	client := clientFromContext(ctx, r.client)
	searchColumns := []string{
		"u.email", "u.username", "u.id::text",
	}
	if IsMySQL() {
		searchColumns = []string{
			"u.email", "u.username", "CAST(u.id AS CHAR)",
		}
	}
	where, args := buildAffiliateRecordWhere(filter, "ual.created_at", searchColumns)
	baseJoin := `
FROM user_affiliate_ledger ual
JOIN users u ON u.id = ual.user_id
WHERE ual.action IN ('transfer', 'withdraw')`
	if where != "" {
		where = strings.Replace(where, "WHERE ", " AND ", 1)
	}

	total, err := queryAffiliateRecordCount(ctx, client, "SELECT COUNT(*) "+baseJoin+where, args...)
	if err != nil {
		return nil, 0, err
	}

	orderBy := buildAffiliateRecordOrderBy(filter, map[string]string{
		"user":                  "u.email",
		"action":                "ual.action",
		"amount":                "ual.amount",
		"balance_after":         "ual.balance_after",
		"available_quota_after": "ual.aff_quota_after",
		"frozen_quota_after":    "ual.aff_frozen_quota_after",
		"history_quota_after":   "ual.aff_history_quota_after",
		"created_at":            "ual.created_at",
	}, "ual.created_at")
	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)
	listQuery := `
SELECT ual.id,
       ual.action,
       ual.user_id,
       COALESCE(u.email, ''),
       COALESCE(u.username, ''),
       ual.amount::double precision,
       ual.balance_after::double precision,
       ual.aff_quota_after::double precision,
       ual.aff_frozen_quota_after::double precision,
       ual.aff_history_quota_after::double precision,
       ual.created_at
` + baseJoin + where + `
` + orderBy + `
LIMIT $` + fmt.Sprint(len(args)-1) + ` OFFSET $` + fmt.Sprint(len(args))
	if IsMySQL() {
		listQuery = `
SELECT ual.id,
       ual.action,
       ual.user_id,
       COALESCE(u.email, ''),
       COALESCE(u.username, ''),
       ual.amount,
       ual.balance_after,
       ual.aff_quota_after,
       ual.aff_frozen_quota_after,
       ual.aff_history_quota_after,
       ual.created_at
` + baseJoin + where + `
` + orderBy + `
LIMIT ? OFFSET ?`
	}
	rows, err := client.QueryContext(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.AffiliateTransferRecord, 0)
	for rows.Next() {
		var item service.AffiliateTransferRecord
		var balanceAfter sql.NullFloat64
		var availableQuotaAfter sql.NullFloat64
		var frozenQuotaAfter sql.NullFloat64
		var historyQuotaAfter sql.NullFloat64
		if err := rows.Scan(
			&item.LedgerID,
			&item.Action,
			&item.UserID,
			&item.UserEmail,
			&item.Username,
			&item.Amount,
			&balanceAfter,
			&availableQuotaAfter,
			&frozenQuotaAfter,
			&historyQuotaAfter,
			&item.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		item.BalanceAfter = nullableFloat64Ptr(balanceAfter)
		item.AvailableQuotaAfter = nullableFloat64Ptr(availableQuotaAfter)
		item.FrozenQuotaAfter = nullableFloat64Ptr(frozenQuotaAfter)
		item.HistoryQuotaAfter = nullableFloat64Ptr(historyQuotaAfter)
		item.SnapshotAvailable = balanceAfter.Valid &&
			availableQuotaAfter.Valid &&
			frozenQuotaAfter.Valid &&
			historyQuotaAfter.Valid
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *affiliateRepository) GetAffiliateUserOverview(ctx context.Context, userID int64) (*service.AffiliateUserOverview, error) {
	if userID <= 0 {
		return nil, service.ErrUserNotFound
	}
	client := clientFromContext(ctx, r.client)
	query := affiliateUserOverviewSQL
	if IsMySQL() {
		query = affiliateUserOverviewSQLMySQL
	}
	rows, err := client.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrUserNotFound
	}

	var overview service.AffiliateUserOverview
	var customRate float64
	var hasCustomRate bool
	if err := rows.Scan(
		&overview.UserID,
		&overview.Email,
		&overview.Username,
		&overview.AffCode,
		&customRate,
		&hasCustomRate,
		&overview.InvitedCount,
		&overview.RebatedInviteeCount,
		&overview.AvailableQuota,
		&overview.HistoryQuota,
	); err != nil {
		return nil, err
	}
	if hasCustomRate {
		overview.RebateRatePercent = customRate
		overview.RebateRateCustom = true
	}
	return &overview, rows.Err()
}

func buildAffiliateRecordWhere(filter service.AffiliateRecordFilter, timeColumn string, searchColumns []string) (string, []any) {
	clauses := make([]string, 0, 3)
	args := make([]any, 0, 3)
	mysql := IsMySQL()
	if filter.StartAt != nil {
		args = append(args, *filter.StartAt)
		if mysql {
			clauses = append(clauses, fmt.Sprintf("%s >= ?", timeColumn))
		} else {
			clauses = append(clauses, fmt.Sprintf("%s >= $%d", timeColumn, len(args)))
		}
	}
	if filter.EndAt != nil {
		args = append(args, *filter.EndAt)
		if mysql {
			clauses = append(clauses, fmt.Sprintf("%s <= ?", timeColumn))
		} else {
			clauses = append(clauses, fmt.Sprintf("%s <= $%d", timeColumn, len(args)))
		}
	}
	search := strings.TrimSpace(filter.Search)
	if search != "" && len(searchColumns) > 0 {
		pattern := "%" + strings.ToLower(search) + "%"
		if !mysql {
			// PG 占位符 $n 可复用：追加一次参数、多个列引用同一个 $n。
			args = append(args, pattern)
		}
		parts := make([]string, 0, len(searchColumns))
		for _, col := range searchColumns {
			if mysql {
				// utf8mb4_0900_ai_ci 大小写不敏感，LOWER(...) LIKE 与 PG 的 LOWER LIKE 语义一致。
				// MySQL 的 ? 不可复用：每个 LIKE 占位符都要追加独立参数（同一模式重复）。
				args = append(args, pattern)
				parts = append(parts, fmt.Sprintf("LOWER(%s) LIKE ?", col))
			} else {
				parts = append(parts, fmt.Sprintf("LOWER(%s) LIKE $%d", col, len(args)))
			}
		}
		clauses = append(clauses, "("+strings.Join(parts, " OR ")+")")
	}
	if len(clauses) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

func buildAffiliateRecordOrderBy(filter service.AffiliateRecordFilter, sortColumns map[string]string, fallbackColumn string) string {
	column := sortColumns[filter.SortBy]
	if column == "" {
		column = fallbackColumn
	}
	direction := "DESC"
	if !filter.SortDesc {
		direction = "ASC"
	}
	if IsMySQL() {
		// MySQL 无 NULLS FIRST/LAST：DESC 默认 NULL 在末尾（等价 DESC NULLS LAST）；
		// ASC 用「col IS NULL」前置键把 NULL 压到末尾，模拟 PG 的 ASC NULLS LAST。
		if direction == "DESC" {
			return "ORDER BY " + column + " DESC"
		}
		return "ORDER BY " + column + " IS NULL, " + column + " ASC"
	}
	return "ORDER BY " + column + " " + direction + " NULLS LAST"
}

func queryAffiliateRecordCount(ctx context.Context, client affiliateQueryExecer, query string, args ...any) (int64, error) {
	rows, err := client.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return 0, rows.Err()
	}
	var total int64
	if err := rows.Scan(&total); err != nil {
		return 0, err
	}
	return total, rows.Err()
}

func (r *affiliateRepository) withTx(ctx context.Context, fn func(txCtx context.Context, txClient *dbent.Client) error) error {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return fn(ctx, tx.Client())
	}

	tx, err := r.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin affiliate transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	txCtx := dbent.NewTxContext(ctx, tx)
	if err := fn(txCtx, tx.Client()); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit affiliate transaction: %w", err)
	}
	return nil
}

func ensureUserAffiliateWithClient(ctx context.Context, client affiliateQueryExecer, userID int64) (*service.AffiliateSummary, error) {
	summary, err := queryAffiliateByUserID(ctx, client, userID)
	if err == nil {
		return summary, nil
	}
	if !errors.Is(err, service.ErrAffiliateProfileNotFound) {
		return nil, err
	}

	for i := 0; i < affiliateCodeMaxAttempts; i++ {
		code, codeErr := generateAffiliateCode()
		if codeErr != nil {
			return nil, codeErr
		}
		if IsMySQL() {
			// PG 的 ON CONFLICT (user_id) DO NOTHING 在 MySQL 拆为裸 INSERT + 冲突判定：
			//   - user_id 主键冲突 → 档案已存在，等同 DO NOTHING（成功路径）；
			//   - aff_code 唯一冲突 → 换码重试（对应 PG 的 23505 → continue）。
			// REVIEW: 主键冲突依赖 InnoDB 1062 错误消息中的 ".PRIMARY" 键名判定。
			_, insertErr := client.ExecContext(ctx, `
INSERT INTO user_affiliates (user_id, aff_code, created_at, updated_at)
VALUES (?, ?, NOW(3), NOW(3))`, userID, code)
			if insertErr == nil {
				break
			}
			if isAffiliateUniqueViolation(insertErr) {
				if isMySQLPrimaryKeyDuplicate(insertErr) {
					break
				}
				continue
			}
			return nil, insertErr
		}
		_, insertErr := client.ExecContext(ctx, `
INSERT INTO user_affiliates (user_id, aff_code, created_at, updated_at)
VALUES ($1, $2, NOW(), NOW())
ON CONFLICT (user_id) DO NOTHING`, userID, code)
		if insertErr == nil {
			break
		}
		if isAffiliateUniqueViolation(insertErr) {
			continue
		}
		return nil, insertErr
	}

	return queryAffiliateByUserID(ctx, client, userID)
}

func queryAffiliateByUserID(ctx context.Context, client affiliateQueryExecer, userID int64) (*service.AffiliateSummary, error) {
	query := `
SELECT user_id,
       aff_code,
       aff_code_custom,
       aff_rebate_rate_percent,
       inviter_id,
       aff_count,
       aff_quota::double precision,
       aff_frozen_quota::double precision,
       aff_history_quota::double precision,
       created_at,
       updated_at
FROM user_affiliates
WHERE user_id = $1`
	if IsMySQL() {
		query = `
SELECT user_id,
       aff_code,
       aff_code_custom,
       aff_rebate_rate_percent,
       inviter_id,
       aff_count,
       aff_quota,
       aff_frozen_quota,
       aff_history_quota,
       created_at,
       updated_at
FROM user_affiliates
WHERE user_id = ?`
	}
	rows, err := client.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrAffiliateProfileNotFound
	}

	var out service.AffiliateSummary
	var inviterID sql.NullInt64
	var rebateRate sql.NullFloat64
	if err := rows.Scan(
		&out.UserID,
		&out.AffCode,
		&out.AffCodeCustom,
		&rebateRate,
		&inviterID,
		&out.AffCount,
		&out.AffQuota,
		&out.AffFrozenQuota,
		&out.AffHistoryQuota,
		&out.CreatedAt,
		&out.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if inviterID.Valid {
		out.InviterID = &inviterID.Int64
	}
	if rebateRate.Valid {
		v := rebateRate.Float64
		out.AffRebateRatePercent = &v
	}
	return &out, nil
}

func queryAffiliateByCode(ctx context.Context, client affiliateQueryExecer, code string) (*service.AffiliateSummary, error) {
	query := `
SELECT user_id,
       aff_code,
       aff_code_custom,
       aff_rebate_rate_percent,
       inviter_id,
       aff_count,
       aff_quota::double precision,
       aff_frozen_quota::double precision,
       aff_history_quota::double precision,
       created_at,
       updated_at
FROM user_affiliates
WHERE aff_code = $1
LIMIT 1`
	if IsMySQL() {
		query = `
SELECT user_id,
       aff_code,
       aff_code_custom,
       aff_rebate_rate_percent,
       inviter_id,
       aff_count,
       aff_quota,
       aff_frozen_quota,
       aff_history_quota,
       created_at,
       updated_at
FROM user_affiliates
WHERE aff_code = ?
LIMIT 1`
	}
	rows, err := client.QueryContext(ctx, query, strings.ToUpper(strings.TrimSpace(code)))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrAffiliateProfileNotFound
	}

	var out service.AffiliateSummary
	var inviterID sql.NullInt64
	var rebateRate sql.NullFloat64
	if err := rows.Scan(
		&out.UserID,
		&out.AffCode,
		&out.AffCodeCustom,
		&rebateRate,
		&inviterID,
		&out.AffCount,
		&out.AffQuota,
		&out.AffFrozenQuota,
		&out.AffHistoryQuota,
		&out.CreatedAt,
		&out.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if inviterID.Valid {
		out.InviterID = &inviterID.Int64
	}
	if rebateRate.Valid {
		v := rebateRate.Float64
		out.AffRebateRatePercent = &v
	}
	return &out, nil
}

func queryUserBalance(ctx context.Context, client affiliateQueryExecer, userID int64) (float64, error) {
	query := "SELECT balance::double precision FROM users WHERE id = $1 LIMIT 1"
	if IsMySQL() {
		query = "SELECT balance FROM users WHERE id = ? LIMIT 1"
	}
	rows, err := client.QueryContext(ctx, query, userID)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return 0, err
		}
		return 0, service.ErrUserNotFound
	}
	var balance float64
	if err := rows.Scan(&balance); err != nil {
		return 0, err
	}
	return balance, nil
}

type affiliateTransferSnapshot struct {
	BalanceAfter        float64
	AvailableQuotaAfter float64
	FrozenQuotaAfter    float64
	HistoryQuotaAfter   float64
}

func queryAffiliateTransferSnapshot(ctx context.Context, client affiliateQueryExecer, userID int64) (*affiliateTransferSnapshot, error) {
	query := `
SELECT u.balance::double precision,
       ua.aff_quota::double precision,
       ua.aff_frozen_quota::double precision,
       ua.aff_history_quota::double precision
FROM users u
JOIN user_affiliates ua ON ua.user_id = u.id
WHERE u.id = $1
LIMIT 1`
	if IsMySQL() {
		query = `
SELECT u.balance,
       ua.aff_quota,
       ua.aff_frozen_quota,
       ua.aff_history_quota
FROM users u
JOIN user_affiliates ua ON ua.user_id = u.id
WHERE u.id = ?
LIMIT 1`
	}
	rows, err := client.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("query affiliate transfer snapshot: %w", err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrUserNotFound
	}

	var snapshot affiliateTransferSnapshot
	if err := rows.Scan(
		&snapshot.BalanceAfter,
		&snapshot.AvailableQuotaAfter,
		&snapshot.FrozenQuotaAfter,
		&snapshot.HistoryQuotaAfter,
	); err != nil {
		return nil, err
	}
	return &snapshot, rows.Err()
}

func nullableFloat64Ptr(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	return &v.Float64
}

func nullableInt64Ptr(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	return &v.Int64
}

func generateAffiliateCode() (string, error) {
	buf := make([]byte, affiliateCodeLength)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate affiliate code: %w", err)
	}
	for i := range buf {
		buf[i] = affiliateCodeCharset[int(buf[i])%len(affiliateCodeCharset)]
	}
	return string(buf), nil
}

func isAffiliateUniqueViolation(err error) bool {
	// MySQL 分支：1062 = 唯一约束冲突（isMySQLUniqueViolation，dialect.go）。
	if isMySQLUniqueViolation(err) {
		return true
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return string(pqErr.Code) == "23505"
	}
	return false
}

// isMySQLPrimaryKeyDuplicate 判定 MySQL 1062 冲突是否发生在主键上：
// InnoDB 错误消息形如 "Duplicate entry '...' for key 'user_affiliates.PRIMARY'"。
// REVIEW: 依赖驱动错误消息文本判定冲突键名（InnoDB 消息格式稳定，但非协议保证）。
func isMySQLPrimaryKeyDuplicate(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && strings.Contains(mysqlErr.Message, ".PRIMARY")
}

// UpdateUserAffCode 改写用户的邀请码（自定义专属邀请码）。
// 唯一性冲突返回 ErrAffiliateCodeTaken。
func (r *affiliateRepository) UpdateUserAffCode(ctx context.Context, userID int64, newCode string) error {
	if userID <= 0 {
		return service.ErrUserNotFound
	}
	code := strings.ToUpper(strings.TrimSpace(newCode))
	if code == "" {
		return service.ErrAffiliateCodeInvalid
	}

	return r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		if _, err := ensureUserAffiliateWithClient(txCtx, txClient, userID); err != nil {
			return err
		}
		updateAffCodeSQL := `
UPDATE user_affiliates
SET aff_code = $1,
    aff_code_custom = true,
    updated_at = NOW()
WHERE user_id = $2`
		if IsMySQL() {
			updateAffCodeSQL = `
UPDATE user_affiliates
SET aff_code = ?,
    aff_code_custom = TRUE,
    updated_at = NOW(3)
WHERE user_id = ?`
		}
		res, err := txClient.ExecContext(txCtx, updateAffCodeSQL, code, userID)
		if err != nil {
			if isAffiliateUniqueViolation(err) {
				return service.ErrAffiliateCodeTaken
			}
			return fmt.Errorf("update aff_code: %w", err)
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			return service.ErrUserNotFound
		}
		return nil
	})
}

// ResetUserAffCode 把 aff_code 还原为系统随机码，并清除 aff_code_custom 标记。
func (r *affiliateRepository) ResetUserAffCode(ctx context.Context, userID int64) (string, error) {
	if userID <= 0 {
		return "", service.ErrUserNotFound
	}
	var newCode string
	err := r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		if _, err := ensureUserAffiliateWithClient(txCtx, txClient, userID); err != nil {
			return err
		}
		for i := 0; i < affiliateCodeMaxAttempts; i++ {
			candidate, codeErr := generateAffiliateCode()
			if codeErr != nil {
				return codeErr
			}
			resetAffCodeSQL := `
UPDATE user_affiliates
SET aff_code = $1,
    aff_code_custom = false,
    updated_at = NOW()
WHERE user_id = $2`
			if IsMySQL() {
				resetAffCodeSQL = `
UPDATE user_affiliates
SET aff_code = ?,
    aff_code_custom = FALSE,
    updated_at = NOW(3)
WHERE user_id = ?`
			}
			res, err := txClient.ExecContext(txCtx, resetAffCodeSQL, candidate, userID)
			if err != nil {
				if isAffiliateUniqueViolation(err) {
					continue
				}
				return fmt.Errorf("reset aff_code: %w", err)
			}
			affected, _ := res.RowsAffected()
			if affected == 0 {
				return service.ErrUserNotFound
			}
			newCode = candidate
			return nil
		}
		return fmt.Errorf("reset aff_code: exhausted attempts")
	})
	if err != nil {
		return "", err
	}
	return newCode, nil
}

// SetUserRebateRate 设置或清除用户专属返利比例。ratePercent==nil 表示清除（沿用全局）。
func (r *affiliateRepository) SetUserRebateRate(ctx context.Context, userID int64, ratePercent *float64) error {
	if userID <= 0 {
		return service.ErrUserNotFound
	}
	return r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		if _, err := ensureUserAffiliateWithClient(txCtx, txClient, userID); err != nil {
			return err
		}
		// nullableArg lets us use a single UPDATE for both "set value" and
		// "clear" cases — database/sql converts nil interface{} to SQL NULL.
		setRebateRateSQL := `
UPDATE user_affiliates
SET aff_rebate_rate_percent = $1,
    updated_at = NOW()
WHERE user_id = $2`
		if IsMySQL() {
			setRebateRateSQL = `
UPDATE user_affiliates
SET aff_rebate_rate_percent = ?,
    updated_at = NOW(3)
WHERE user_id = ?`
		}
		res, err := txClient.ExecContext(txCtx, setRebateRateSQL, nullableArg(ratePercent), userID)
		if err != nil {
			return fmt.Errorf("set aff_rebate_rate_percent: %w", err)
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			return service.ErrUserNotFound
		}
		return nil
	})
}

// BatchSetUserRebateRate 批量为多个用户设置专属比例（nil 清除）。
func (r *affiliateRepository) BatchSetUserRebateRate(ctx context.Context, userIDs []int64, ratePercent *float64) error {
	if len(userIDs) == 0 {
		return nil
	}
	return r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		for _, uid := range userIDs {
			if uid <= 0 {
				continue
			}
			if _, err := ensureUserAffiliateWithClient(txCtx, txClient, uid); err != nil {
				return err
			}
		}
		batchSetRebateRateSQL := `
UPDATE user_affiliates
SET aff_rebate_rate_percent = $1,
    updated_at = NOW()
WHERE user_id = ANY($2)`
		batchArgs := []any{nullableArg(ratePercent), pq.Array(userIDs)}
		if IsMySQL() {
			// ANY($2) + pq.Array → IN (?,?,...) 展开（Placeholders，dialect.go）。
			batchSetRebateRateSQL = `
UPDATE user_affiliates
SET aff_rebate_rate_percent = ?,
    updated_at = NOW(3)
WHERE user_id IN (` + Placeholders(len(userIDs)) + `)`
			batchArgs = make([]any, 0, len(userIDs)+1)
			batchArgs = append(batchArgs, nullableArg(ratePercent))
			for _, uid := range userIDs {
				batchArgs = append(batchArgs, uid)
			}
		}
		_, err := txClient.ExecContext(txCtx, batchSetRebateRateSQL, batchArgs...)
		if err != nil {
			return fmt.Errorf("batch set aff_rebate_rate_percent: %w", err)
		}
		return nil
	})
}

// nullableArg unwraps a *float64 into an interface{} suitable for SQL parameter
// binding: nil pointer → SQL NULL, non-nil → the float value.
func nullableArg(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullableInt64Arg(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

// claimAffiliateQuotaMySQL 是 TransferQuotaToBalance 中「可写 CTE（claimed FOR UPDATE +
// cleared UPDATE..RETURNING c.amount）」的 MySQL 拆解：
// 同事务内先 FOR UPDATE 锁定读取 aff_quota（>0 才有行），再置 0；先取值即为转移额。
// 无行（aff_quota <= 0）时返回 0，由调用方转换为 ErrAffiliateQuotaEmpty。
func claimAffiliateQuotaMySQL(txCtx context.Context, txClient *dbent.Client, userID int64) (float64, error) {
	scope, err := beginSQLAtomicScope(txCtx, txClient, nil)
	if err != nil {
		return 0, err
	}
	defer scope.rollback()

	rows, err := scope.query(txCtx,
		"SELECT aff_quota FROM user_affiliates WHERE user_id = ? AND aff_quota > 0 FOR UPDATE", userID)
	if err != nil {
		return 0, fmt.Errorf("claim affiliate quota: %w", err)
	}
	var transferred float64
	hasRow := false
	if rows.Next() {
		if err := rows.Scan(&transferred); err != nil {
			_ = rows.Close()
			return 0, err
		}
		hasRow = true
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if !hasRow {
		return 0, nil
	}
	if _, err := scope.exec(txCtx,
		"UPDATE user_affiliates SET aff_quota = 0, updated_at = NOW(3) WHERE user_id = ?", userID); err != nil {
		return 0, fmt.Errorf("claim affiliate quota: %w", err)
	}
	return transferred, nil
}

// claimAffiliateWithdrawLedgerMySQL 是 claimAffiliateWithdrawLedger 的 MySQL 实现。
// PG 版 INSERT..SELECT..ON CONFLICT (operation_id) WHERE operation_id IS NOT NULL
// DO NOTHING RETURNING id 在 MySQL 拆为：
//  1. 普通 SELECT 确认用户存在返利档案（快照读，不加锁：MySQL 的 INSERT..SELECT 会对
//     源表 user_affiliates 行加共享 next-key 锁，与并发扣减 quota 的 UPDATE 互斥，
//     同 operation_id 并发登记时会形成 S/X 死锁（Error 1213），见规范错误分类节）；
//  2. INSERT..VALUES + ON DUPLICATE KEY UPDATE（no-op）：MySQL 唯一索引天然允许多个
//     NULL（migrations/mysql/240），与 PG 部分唯一索引等价；同键并发插入在唯一索引上
//     等待先写入者提交/回滚（与 PG ON CONFLICT 等待语义一致）；
//  3. RowsAffected：1=新插入（LastInsertId 取主键），0=冲突或无返利档案（claimed=false）。
func claimAffiliateWithdrawLedgerMySQL(ctx context.Context, client affiliateQueryExecer, userID int64, amount float64, operationID string) (int64, bool, error) {
	// 对用户的返利档案行取排它锁：同一用户的并发登记（含同 operation_id 重放）
	// 在这把行锁上串行化。间隙锁方案不可行——InnoDB 间隙锁彼此兼容，
	// 两个并发事务仍会在同键 INSERT 上死锁（Error 1213）。
	var exists int64
	rows, err := client.QueryContext(ctx,
		"SELECT 1 FROM user_affiliates WHERE user_id = ? FOR UPDATE", userID)
	if err != nil {
		return 0, false, fmt.Errorf("claim affiliate withdraw ledger: %w", err)
	}
	if rows.Next() {
		if err := rows.Scan(&exists); err != nil {
			_ = rows.Close()
			return 0, false, fmt.Errorf("claim affiliate withdraw ledger: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, false, fmt.Errorf("claim affiliate withdraw ledger: %w", err)
	}
	if err := rows.Close(); err != nil {
		return 0, false, err
	}
	if exists == 0 {
		return 0, false, nil
	}
	res, err := client.ExecContext(ctx, `
INSERT INTO user_affiliate_ledger (user_id, action, amount, operation_id, created_at, updated_at)
VALUES (?, 'withdraw', ?, ?, NOW(3), NOW(3))
ON DUPLICATE KEY UPDATE user_affiliate_ledger.id = user_affiliate_ledger.id`,
		userID, amount, operationID)
	if err != nil {
		return 0, false, fmt.Errorf("claim affiliate withdraw ledger: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, false, err
	}
	if affected == 0 {
		return 0, false, nil
	}
	ledgerID, err := res.LastInsertId()
	if err != nil {
		return 0, false, err
	}
	return ledgerID, true, nil
}

// ListUsersWithCustomSettings 列出有专属配置（自定义码或专属比例）的用户。
//
// 单一查询同时处理"无搜索"与"按邮箱/用户名模糊搜索"：
// 空 search 时拼接出的 LIKE 模式为 "%%"，匹配所有行；非空时按 ILIKE 子串匹配。
// 这避免了为两种情况维护两份 SQL 模板。
func (r *affiliateRepository) ListUsersWithCustomSettings(ctx context.Context, filter service.AffiliateAdminFilter) ([]service.AffiliateAdminEntry, int64, error) {
	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize
	likePattern := "%" + strings.TrimSpace(filter.Search) + "%"

	baseFrom := `
FROM user_affiliates ua
JOIN users u ON u.id = ua.user_id
WHERE (ua.aff_code_custom = true OR ua.aff_rebate_rate_percent IS NOT NULL)
  AND (u.email ILIKE $1 OR u.username ILIKE $1)`
	listQuery := `
SELECT ua.user_id,
       COALESCE(u.email, ''),
       COALESCE(u.username, ''),
       ua.aff_code,
       ua.aff_code_custom,
       ua.aff_rebate_rate_percent,
       ua.aff_count` + baseFrom + `
ORDER BY ua.updated_at DESC
LIMIT $2 OFFSET $3`
	countArgs := []any{likePattern}
	listArgs := []any{likePattern, pageSize, offset}
	if IsMySQL() {
		// ILIKE → LIKE（utf8mb4_0900_ai_ci 大小写不敏感）；
		// $1 被引用两次，MySQL 分支需绑定两份同参数。
		baseFrom = `
FROM user_affiliates ua
JOIN users u ON u.id = ua.user_id
WHERE (ua.aff_code_custom = TRUE OR ua.aff_rebate_rate_percent IS NOT NULL)
  AND (u.email LIKE ? OR u.username LIKE ?)`
		listQuery = `
SELECT ua.user_id,
       COALESCE(u.email, ''),
       COALESCE(u.username, ''),
       ua.aff_code,
       ua.aff_code_custom,
       ua.aff_rebate_rate_percent,
       ua.aff_count` + baseFrom + `
ORDER BY ua.updated_at DESC
LIMIT ? OFFSET ?`
		countArgs = []any{likePattern, likePattern}
		listArgs = []any{likePattern, likePattern, pageSize, offset}
	}

	client := clientFromContext(ctx, r.client)

	total, err := scanInt64(ctx, client, "SELECT COUNT(*)"+baseFrom, countArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("count affiliate admin entries: %w", err)
	}

	rows, err := client.QueryContext(ctx, listQuery, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list affiliate admin entries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	entries := make([]service.AffiliateAdminEntry, 0)
	for rows.Next() {
		var e service.AffiliateAdminEntry
		var rebate sql.NullFloat64
		if err := rows.Scan(&e.UserID, &e.Email, &e.Username, &e.AffCode,
			&e.AffCodeCustom, &rebate, &e.AffCount); err != nil {
			return nil, 0, err
		}
		if rebate.Valid {
			v := rebate.Float64
			e.AffRebateRatePercent = &v
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return entries, total, nil
}

// scanInt64 runs a query expected to return a single int64 column (e.g. COUNT).
func scanInt64(ctx context.Context, client affiliateQueryExecer, query string, args ...any) (int64, error) {
	rows, err := client.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return 0, err
		}
		return 0, nil
	}
	var v int64
	if err := rows.Scan(&v); err != nil {
		return 0, err
	}
	return v, nil
}
