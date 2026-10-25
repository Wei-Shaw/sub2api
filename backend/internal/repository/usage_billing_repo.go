package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type usageBillingRepository struct {
	db *sql.DB
}

func NewUsageBillingRepository(_ *dbent.Client, sqlDB *sql.DB) service.UsageBillingRepository {
	return &usageBillingRepository{db: sqlDB}
}

func (r *usageBillingRepository) Apply(ctx context.Context, cmd *service.UsageBillingCommand) (_ *service.UsageBillingApplyResult, err error) {
	if cmd == nil {
		return &service.UsageBillingApplyResult{}, nil
	}
	if r == nil || r.db == nil {
		return nil, errors.New("usage billing repository db is nil")
	}

	cmd.Normalize()
	if cmd.RequestID == "" {
		return nil, service.ErrUsageBillingRequestIDRequired
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

	applied, err := r.claimUsageBillingKey(ctx, tx, cmd)
	if err != nil {
		return nil, err
	}
	if !applied {
		return &service.UsageBillingApplyResult{Applied: false}, nil
	}

	result := &service.UsageBillingApplyResult{Applied: true}
	if err := r.applyUsageBillingEffects(ctx, tx, cmd, result); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil
	return result, nil
}

func (r *usageBillingRepository) claimUsageBillingKey(ctx context.Context, tx *sql.Tx, cmd *service.UsageBillingCommand) (bool, error) {
	return r.claimUsageBillingRequest(ctx, tx, cmd.RequestID, cmd.APIKeyID, cmd.RequestFingerprint)
}

func (r *usageBillingRepository) claimUsageBillingRequest(ctx context.Context, tx *sql.Tx, requestID string, apiKeyID int64, requestFingerprint string) (bool, error) {
	if IsMySQL() {
		return r.claimUsageBillingRequestMySQL(ctx, tx, requestID, apiKeyID, requestFingerprint)
	}
	var id int64
	err := tx.QueryRowContext(ctx, `
		INSERT INTO usage_billing_dedup (request_id, api_key_id, request_fingerprint)
		VALUES ($1, $2, $3)
		ON CONFLICT (request_id, api_key_id) DO NOTHING
		RETURNING id
	`, requestID, apiKeyID, requestFingerprint).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		var existingFingerprint string
		if err := tx.QueryRowContext(ctx, `
			SELECT request_fingerprint
			FROM usage_billing_dedup
			WHERE request_id = $1 AND api_key_id = $2
		`, requestID, apiKeyID).Scan(&existingFingerprint); err != nil {
			return false, err
		}
		if strings.TrimSpace(existingFingerprint) != strings.TrimSpace(requestFingerprint) {
			return false, service.ErrUsageBillingRequestConflict
		}
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var archivedFingerprint string
	err = tx.QueryRowContext(ctx, `
		SELECT request_fingerprint
		FROM usage_billing_dedup_archive
		WHERE request_id = $1 AND api_key_id = $2
	`, requestID, apiKeyID).Scan(&archivedFingerprint)
	if err == nil {
		if strings.TrimSpace(archivedFingerprint) != strings.TrimSpace(requestFingerprint) {
			return false, service.ErrUsageBillingRequestConflict
		}
		return false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	return true, nil
}

func (r *usageBillingRepository) ReserveBatchImageBalance(ctx context.Context, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	return r.applyBatchImageBalanceHold(ctx, cmd, reserveUsageBillingBatchImageBalance)
}

func (r *usageBillingRepository) CaptureBatchImageBalance(ctx context.Context, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	return r.applyBatchImageBalanceHold(ctx, cmd, captureUsageBillingBatchImageBalance)
}

func (r *usageBillingRepository) ReleaseBatchImageBalance(ctx context.Context, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	return r.applyBatchImageBalanceHold(ctx, cmd, releaseUsageBillingBatchImageBalance)
}

func (r *usageBillingRepository) applyBatchImageBalanceHold(
	ctx context.Context,
	cmd *service.BatchImageBalanceHoldCommand,
	apply func(context.Context, *sql.Tx, *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error),
) (_ *service.BatchImageBalanceHoldResult, err error) {
	if cmd == nil {
		return &service.BatchImageBalanceHoldResult{}, nil
	}
	if r == nil || r.db == nil {
		return nil, errors.New("usage billing repository db is nil")
	}
	cmd.Normalize()
	if cmd.RequestID == "" {
		return nil, service.ErrUsageBillingRequestIDRequired
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

	applied, err := r.claimUsageBillingRequest(ctx, tx, cmd.RequestID, cmd.APIKeyID, cmd.RequestFingerprint)
	if err != nil {
		return nil, err
	}
	if !applied {
		return &service.BatchImageBalanceHoldResult{Applied: false}, nil
	}

	result, err := apply(ctx, tx, cmd)
	if err != nil {
		return nil, err
	}
	if result == nil {
		result = &service.BatchImageBalanceHoldResult{}
	}
	result.Applied = true

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil
	return result, nil
}

func (r *usageBillingRepository) applyUsageBillingEffects(ctx context.Context, tx *sql.Tx, cmd *service.UsageBillingCommand, result *service.UsageBillingApplyResult) error {
	if cmd.SubscriptionCost > 0 && cmd.SubscriptionID != nil {
		if err := incrementUsageBillingSubscription(ctx, tx, *cmd.SubscriptionID, cmd.SubscriptionCost); err != nil {
			return err
		}
	}

	if cmd.BalanceCost > 0 {
		newBalance, sufficient, err := deductUsageBillingBalance(ctx, tx, cmd.UserID, cmd.BalanceCost)
		if err != nil {
			return err
		}
		result.NewBalance = &newBalance
		result.BalanceOverdrafted = !sufficient
	}

	// Key 已不存在时跳过其自身的额度/限速计数，其余结算项不受影响。
	if cmd.APIKeyQuotaCost > 0 {
		exhausted, err := incrementUsageBillingAPIKeyQuota(ctx, tx, cmd.APIKeyID, cmd.APIKeyQuotaCost)
		if err != nil && !errors.Is(err, service.ErrAPIKeyNotFound) {
			return err
		}
		result.APIKeyQuotaExhausted = exhausted
	}

	if cmd.APIKeyRateLimitCost > 0 {
		if err := incrementUsageBillingAPIKeyRateLimit(ctx, tx, cmd.APIKeyID, cmd.APIKeyRateLimitCost); err != nil && !errors.Is(err, service.ErrAPIKeyNotFound) {
			return err
		}
	}

	// 账号已不存在时跳过其配额计数，其余结算项不受影响。
	if cmd.AccountQuotaCost > 0 && service.AccountTypeSupportsQuota(cmd.AccountType) {
		quotaState, err := incrementUsageBillingAccountQuota(ctx, tx, cmd.AccountID, cmd.AccountQuotaCost, cmd.AccountQuotaHasDailyLimit, cmd.AccountQuotaHasWeeklyLimit)
		if err != nil && !errors.Is(err, service.ErrAccountNotFound) {
			return err
		}
		result.QuotaState = quotaState
	}

	return nil
}

func incrementUsageBillingSubscription(ctx context.Context, tx *sql.Tx, subscriptionID int64, costUSD float64) error {
	if IsMySQL() {
		return incrementUsageBillingSubscriptionMySQL(ctx, tx, subscriptionID, costUSD)
	}
	const updateSQL = `
		UPDATE user_subscriptions us
		SET
			daily_usage_usd = us.daily_usage_usd + $1,
			weekly_usage_usd = us.weekly_usage_usd + $1,
			monthly_usage_usd = us.monthly_usage_usd + $1,
			updated_at = NOW()
		FROM groups g
		WHERE us.id = $2
			AND us.deleted_at IS NULL
			AND us.group_id = g.id
			AND g.deleted_at IS NULL
	`
	res, err := tx.ExecContext(ctx, updateSQL, costUSD, subscriptionID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected > 0 {
		return nil
	}
	return service.ErrSubscriptionNotFound
}

func deductUsageBillingBalance(ctx context.Context, tx *sql.Tx, userID int64, amount float64) (float64, bool, error) {
	if IsMySQL() {
		return deductUsageBillingBalanceMySQL(ctx, tx, userID, amount)
	}
	var newBalance float64
	err := tx.QueryRowContext(ctx, `
		UPDATE users
		SET balance = balance - $1,
			updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL AND balance >= $1
		RETURNING balance
	`, amount, userID).Scan(&newBalance)
	if err == nil {
		return newBalance, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, err
	}

	err = tx.QueryRowContext(ctx, `
		UPDATE users
		SET balance = balance - $1,
			updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
		RETURNING balance
	`, amount, userID).Scan(&newBalance)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, service.ErrUserNotFound
	}
	if err != nil {
		return 0, false, err
	}
	return newBalance, false, nil
}

func reserveUsageBillingBatchImageBalance(ctx context.Context, tx *sql.Tx, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	if IsMySQL() {
		return reserveUsageBillingBatchImageBalanceMySQL(ctx, tx, cmd)
	}
	if cmd.HoldAmount <= 0 {
		return &service.BatchImageBalanceHoldResult{}, nil
	}
	var balance, frozen float64
	err := tx.QueryRowContext(ctx, `
		UPDATE users
		SET balance = balance - $1,
			frozen_balance = COALESCE(frozen_balance, 0) + $1,
			updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL AND balance >= $1
		RETURNING balance, frozen_balance
	`, cmd.HoldAmount, cmd.UserID).Scan(&balance, &frozen)
	if err == nil {
		return &service.BatchImageBalanceHoldResult{NewBalance: &balance, FrozenBalance: &frozen}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if exists, existsErr := userExistsForBilling(ctx, tx, cmd.UserID); existsErr != nil {
		return nil, existsErr
	} else if !exists {
		return nil, service.ErrUserNotFound
	}
	return nil, service.ErrBatchImageInsufficientBalance
}

func captureUsageBillingBatchImageBalance(ctx context.Context, tx *sql.Tx, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	if IsMySQL() {
		return captureUsageBillingBatchImageBalanceMySQL(ctx, tx, cmd)
	}
	if cmd.HoldAmount <= 0 && cmd.ActualAmount <= 0 {
		return &service.BatchImageBalanceHoldResult{}, nil
	}
	if cmd.ActualAmount-cmd.HoldAmount > 0.00000001 {
		return nil, service.ErrBatchImageSettlementCostExceedsHold
	}
	var balance, frozen float64
	err := tx.QueryRowContext(ctx, `
		UPDATE users
		SET balance = balance
				+ CASE WHEN $1 > $2 THEN $1 - $2 ELSE 0 END
				- CASE WHEN $2 > $1 THEN $2 - $1 ELSE 0 END,
			frozen_balance = COALESCE(frozen_balance, 0) - $1,
			updated_at = NOW()
		WHERE id = $3 AND deleted_at IS NULL AND COALESCE(frozen_balance, 0) >= $1
		RETURNING balance, frozen_balance
	`, cmd.HoldAmount, cmd.ActualAmount, cmd.UserID).Scan(&balance, &frozen)
	if err == nil {
		return &service.BatchImageBalanceHoldResult{NewBalance: &balance, FrozenBalance: &frozen}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if exists, existsErr := userExistsForBilling(ctx, tx, cmd.UserID); existsErr != nil {
		return nil, existsErr
	} else if !exists {
		return nil, service.ErrUserNotFound
	}
	return nil, errors.New("batch image frozen balance is insufficient")
}

func releaseUsageBillingBatchImageBalance(ctx context.Context, tx *sql.Tx, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	if IsMySQL() {
		return releaseUsageBillingBatchImageBalanceMySQL(ctx, tx, cmd)
	}
	if cmd.HoldAmount <= 0 {
		return &service.BatchImageBalanceHoldResult{}, nil
	}
	// 释放前校验该 job 确实预留过 hold（hold request id 已被 claim），
	// 防止从未成功冻结的 job 触发"幻影释放"，从其他用户的冻结资金池中凭空生成余额。
	held, heldErr := batchImageHoldClaimExists(ctx, tx, service.BatchImageHoldRequestID(cmd.BatchID), cmd.APIKeyID)
	if heldErr != nil {
		return nil, heldErr
	}
	if !held {
		logger.LegacyPrintf("repository.usage_billing", "[BatchImage] release skipped, hold was never reserved: batch=%s", cmd.BatchID)
		return &service.BatchImageBalanceHoldResult{}, nil
	}
	var balance, frozen float64
	err := tx.QueryRowContext(ctx, `
		UPDATE users
		SET balance = balance + $1,
			frozen_balance = COALESCE(frozen_balance, 0) - $1,
			updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL AND COALESCE(frozen_balance, 0) >= $1
		RETURNING balance, frozen_balance
	`, cmd.HoldAmount, cmd.UserID).Scan(&balance, &frozen)
	if err == nil {
		return &service.BatchImageBalanceHoldResult{NewBalance: &balance, FrozenBalance: &frozen}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if exists, existsErr := userExistsForBilling(ctx, tx, cmd.UserID); existsErr != nil {
		return nil, existsErr
	} else if !exists {
		return nil, service.ErrUserNotFound
	}
	return nil, errors.New("batch image frozen balance is insufficient")
}

// batchImageHoldClaimExists 检查 hold request id 是否已在 dedup（或归档）表中被 claim，
// 即该 batch 的冻结操作确实成功提交过。
func batchImageHoldClaimExists(ctx context.Context, tx *sql.Tx, holdRequestID string, apiKeyID int64) (bool, error) {
	if IsMySQL() {
		return batchImageHoldClaimExistsMySQL(ctx, tx, holdRequestID, apiKeyID)
	}
	var exists int
	err := tx.QueryRowContext(ctx, `
		SELECT 1
		FROM usage_billing_dedup
		WHERE request_id = $1 AND api_key_id = $2
	`, holdRequestID, apiKeyID).Scan(&exists)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	err = tx.QueryRowContext(ctx, `
		SELECT 1
		FROM usage_billing_dedup_archive
		WHERE request_id = $1 AND api_key_id = $2
	`, holdRequestID, apiKeyID).Scan(&exists)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return false, err
}

func userExistsForBilling(ctx context.Context, tx *sql.Tx, userID int64) (bool, error) {
	if IsMySQL() {
		return userExistsForBillingMySQL(ctx, tx, userID)
	}
	var exists int
	err := tx.QueryRowContext(ctx, `
		SELECT 1
		FROM users
		WHERE id = $1 AND deleted_at IS NULL
	`, userID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func incrementUsageBillingAPIKeyQuota(ctx context.Context, tx *sql.Tx, apiKeyID int64, amount float64) (bool, error) {
	if IsMySQL() {
		return incrementUsageBillingAPIKeyQuotaMySQL(ctx, tx, apiKeyID, amount)
	}
	var exhausted bool
	err := tx.QueryRowContext(ctx, `
		UPDATE api_keys
		SET quota_used = quota_used + $1,
			status = CASE
				WHEN quota > 0
					AND status = $3
					AND quota_used < quota
					AND quota_used + $1 >= quota
				THEN $4
				ELSE status
			END,
			updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
		RETURNING quota > 0 AND quota_used >= quota AND quota_used - $1 < quota
	`, amount, apiKeyID, service.StatusAPIKeyActive, service.StatusAPIKeyQuotaExhausted).Scan(&exhausted)
	if errors.Is(err, sql.ErrNoRows) {
		return false, service.ErrAPIKeyNotFound
	}
	if err != nil {
		return false, err
	}
	return exhausted, nil
}

func incrementUsageBillingAPIKeyRateLimit(ctx context.Context, tx *sql.Tx, apiKeyID int64, cost float64) error {
	if IsMySQL() {
		return incrementUsageBillingAPIKeyRateLimitMySQL(ctx, tx, apiKeyID, cost)
	}
	res, err := tx.ExecContext(ctx, `
		UPDATE api_keys SET
			usage_5h = CASE WHEN window_5h_start IS NOT NULL AND window_5h_start + INTERVAL '5 hours' <= NOW() THEN $1 ELSE usage_5h + $1 END,
			usage_1d = CASE WHEN window_1d_start IS NOT NULL AND window_1d_start + INTERVAL '24 hours' <= NOW() THEN $1 ELSE usage_1d + $1 END,
			usage_7d = CASE WHEN window_7d_start IS NOT NULL AND window_7d_start + INTERVAL '7 days' <= NOW() THEN $1 ELSE usage_7d + $1 END,
			window_5h_start = CASE WHEN window_5h_start IS NULL OR window_5h_start + INTERVAL '5 hours' <= NOW() THEN NOW() ELSE window_5h_start END,
			window_1d_start = CASE WHEN window_1d_start IS NULL OR window_1d_start + INTERVAL '24 hours' <= NOW() THEN date_trunc('day', NOW()) ELSE window_1d_start END,
			window_7d_start = CASE WHEN window_7d_start IS NULL OR window_7d_start + INTERVAL '7 days' <= NOW() THEN date_trunc('day', NOW()) ELSE window_7d_start END,
			updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
	`, cost, apiKeyID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrAPIKeyNotFound
	}
	return nil
}

// accountQuotaTotalIncrementExpr 累加总额度已用量。
const accountQuotaTotalIncrementExpr = `COALESCE(extra, '{}'::jsonb)
			|| jsonb_build_object('quota_used', COALESCE((extra->>'quota_used')::numeric, 0) + $1)`

// accountQuotaDailyIncrementExpr 在库中设置了日上限时累加日用量，周期过期时从本次金额重新计数。
const accountQuotaDailyIncrementExpr = `
			|| CASE WHEN COALESCE((extra->>'quota_daily_limit')::numeric, 0) > 0 THEN
				jsonb_build_object(
					'quota_daily_used',
					CASE WHEN ` + dailyExpiredExpr + `
					THEN $1
					ELSE COALESCE((extra->>'quota_daily_used')::numeric, 0) + $1 END,
					'quota_daily_start',
					CASE WHEN ` + dailyExpiredExpr + `
					THEN ` + nowUTC + `
					ELSE COALESCE(extra->>'quota_daily_start', ` + nowUTC + `) END
				)
				|| CASE WHEN ` + dailyExpiredExpr + ` AND ` + nextDailyResetAtExpr + ` IS NOT NULL
				   THEN jsonb_build_object('quota_daily_reset_at', ` + nextDailyResetAtExpr + `)
				   ELSE '{}'::jsonb END
			ELSE '{}'::jsonb END`

// accountQuotaWeeklyIncrementExpr 在库中设置了周上限时累加周用量，周期过期时从本次金额重新计数。
const accountQuotaWeeklyIncrementExpr = `
			|| CASE WHEN COALESCE((extra->>'quota_weekly_limit')::numeric, 0) > 0 THEN
				jsonb_build_object(
					'quota_weekly_used',
					CASE WHEN ` + weeklyExpiredExpr + `
					THEN $1
					ELSE COALESCE((extra->>'quota_weekly_used')::numeric, 0) + $1 END,
					'quota_weekly_start',
					CASE WHEN ` + weeklyExpiredExpr + `
					THEN ` + nowUTC + `
					ELSE COALESCE(extra->>'quota_weekly_start', ` + nowUTC + `) END
				)
				|| CASE WHEN ` + weeklyExpiredExpr + ` AND ` + nextWeeklyResetAtExpr + ` IS NOT NULL
				   THEN jsonb_build_object('quota_weekly_reset_at', ` + nextWeeklyResetAtExpr + `)
				   ELSE '{}'::jsonb END
			ELSE '{}'::jsonb END`

// accountQuotaExtraIsObjectCond 要求 extra 为 JSON 对象：省略某个维度的语句只在对象上与完整语句等价。
//
// accountQuotaDailyLimitUnsetCond / accountQuotaWeeklyLimitUnsetCond 在库中未设置日/周上限时为真：
// 键不存在、值为 JSON null 或不大于 0 的数字。其他取值一律为假，交给包含该维度的语句处理。
// 三个条件都只使用 ->、jsonb_typeof 与 jsonb 比较，对任何行求值都不会报错。
const (
	accountQuotaExtraIsObjectCond   = `jsonb_typeof(extra) = 'object'`
	accountQuotaDailyLimitUnsetCond = `(extra->'quota_daily_limit' IS NULL
			OR jsonb_typeof(extra->'quota_daily_limit') = 'null'
			OR (jsonb_typeof(extra->'quota_daily_limit') = 'number' AND extra->'quota_daily_limit' <= '0'::jsonb))`
	accountQuotaWeeklyLimitUnsetCond = `(extra->'quota_weekly_limit' IS NULL
			OR jsonb_typeof(extra->'quota_weekly_limit') = 'null'
			OR (jsonb_typeof(extra->'quota_weekly_limit') = 'number' AND extra->'quota_weekly_limit' <= '0'::jsonb))`
)

const accountQuotaIncrementReturning = `
		RETURNING
			COALESCE((extra->>'quota_used')::numeric, 0),
			COALESCE((extra->>'quota_limit')::numeric, 0),
			COALESCE((extra->>'quota_daily_used')::numeric, 0),
			COALESCE((extra->>'quota_daily_limit')::numeric, 0),
			COALESCE((extra->>'quota_weekly_used')::numeric, 0),
			COALESCE((extra->>'quota_weekly_limit')::numeric, 0)`

// accountQuotaIncrementSQLs 按 accountQuotaIncrementSQLIndex 存放账号配额累加语句。省略日或周维度的语句
// 在 WHERE 中附加 accountQuotaExtraIsObjectCond 与被省略维度的未设置条件，条件不成立时命中 0 行；
// 同时包含日、周维度的语句不附加这些条件，对任何账号配置都给出完整的累加结果。
var accountQuotaIncrementSQLs = buildAccountQuotaIncrementSQLs()

func accountQuotaIncrementSQLIndex(daily, weekly bool) int {
	index := 0
	if daily {
		index |= 1
	}
	if weekly {
		index |= 2
	}
	return index
}

func buildAccountQuotaIncrementSQLs() [4]string {
	var sqls [4]string
	for _, daily := range []bool{false, true} {
		for _, weekly := range []bool{false, true} {
			set := accountQuotaTotalIncrementExpr
			where := "id = $2 AND deleted_at IS NULL"
			if !daily || !weekly {
				where += "\n\t\t\tAND " + accountQuotaExtraIsObjectCond
			}
			if daily {
				set += accountQuotaDailyIncrementExpr
			} else {
				where += "\n\t\t\tAND " + accountQuotaDailyLimitUnsetCond
			}
			if weekly {
				set += accountQuotaWeeklyIncrementExpr
			} else {
				where += "\n\t\t\tAND " + accountQuotaWeeklyLimitUnsetCond
			}
			sqls[accountQuotaIncrementSQLIndex(daily, weekly)] = "UPDATE accounts SET extra = (\n\t\t\t" + set +
				"\n\t\t), updated_at = NOW()\n\t\tWHERE " + where + accountQuotaIncrementReturning
		}
	}
	return sqls
}

// incrementUsageBillingAccountQuota 累加账号配额并返回累加后的状态。hasDaily/hasWeekly 是调用方所见的
// 账号日/周上限设置，只决定先执行哪条语句；该语句的附加条件不成立而命中 0 行时，再执行同时包含日、周维度的语句。
func incrementUsageBillingAccountQuota(ctx context.Context, tx *sql.Tx, accountID int64, amount float64, hasDaily, hasWeekly bool) (*service.AccountQuotaState, error) {
	if IsMySQL() {
		return incrementUsageBillingAccountQuotaMySQL(ctx, tx, accountID, amount, hasDaily, hasWeekly)
	}
	index := accountQuotaIncrementSQLIndex(hasDaily, hasWeekly)
	fullIndex := accountQuotaIncrementSQLIndex(true, true)
	state, err := execAccountQuotaIncrement(ctx, tx, accountQuotaIncrementSQLs[index], accountID, amount)
	if err != nil {
		return nil, err
	}
	if state == nil && index != fullIndex {
		state, err = execAccountQuotaIncrement(ctx, tx, accountQuotaIncrementSQLs[fullIndex], accountID, amount)
		if err != nil {
			return nil, err
		}
	}
	if state == nil {
		return nil, service.ErrAccountNotFound
	}
	// 任意维度额度在本次递增中从"未超"跨越到"已超"时，必须刷新调度快照，
	// 否则 Redis 中缓存的 Account 仍显示旧的 used 值，后续请求会继续选中本账号，
	// 最终观察到 daily_used / weekly_used 大幅超过配置的 limit。
	// 对于日/周额度，即使本次触发了周期重置（pre=0、post=amount），
	// 判定式 (post-amount) < limit 同样成立，逻辑与总额度保持一致。
	crossedTotal := state.TotalLimit > 0 && state.TotalUsed >= state.TotalLimit && (state.TotalUsed-amount) < state.TotalLimit
	crossedDaily := state.DailyLimit > 0 && state.DailyUsed >= state.DailyLimit && (state.DailyUsed-amount) < state.DailyLimit
	crossedWeekly := state.WeeklyLimit > 0 && state.WeeklyUsed >= state.WeeklyLimit && (state.WeeklyUsed-amount) < state.WeeklyLimit
	if crossedTotal || crossedDaily || crossedWeekly {
		if err := enqueueSchedulerOutbox(ctx, tx, service.SchedulerOutboxEventAccountChanged, &accountID, nil, nil); err != nil {
			logger.LegacyPrintf("repository.usage_billing", "[SchedulerOutbox] enqueue quota exceeded failed: account=%d err=%v", accountID, err)
			return nil, err
		}
	}
	return state, nil
}

// execAccountQuotaIncrement 执行一条账号配额累加语句，未命中任何行时返回 nil 状态。
func execAccountQuotaIncrement(ctx context.Context, tx *sql.Tx, query string, accountID int64, amount float64) (*service.AccountQuotaState, error) {
	rows, err := tx.QueryContext(ctx, query, amount, accountID)
	if err != nil {
		return nil, err
	}

	var state *service.AccountQuotaState
	if rows.Next() {
		state = &service.AccountQuotaState{}
		if err := rows.Scan(
			&state.TotalUsed, &state.TotalLimit,
			&state.DailyUsed, &state.DailyLimit,
			&state.WeeklyUsed, &state.WeeklyLimit,
		); err != nil {
			_ = rows.Close()
			return nil, err
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	// 必须在执行下一条 SQL 前显式关闭 rows：pq 驱动在同一连接上
	// 不允许前一条查询的结果集未耗尽时启动新查询，否则会返回
	// "unexpected Parse response" 错误。
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return state, nil
}

// ---------------------------------------------------------------------------
// MySQL 方言实现。所有语句都在调用方传入的同一 *sql.Tx 上执行（单连接单事务），
// 与 PG 单条 RETURNING 语句的原子语义对齐。
// ---------------------------------------------------------------------------

// claimUsageBillingRequestMySQL 是 claimUsageBillingRequest 的 MySQL 实现。
// PG 版用 ON CONFLICT (request_id, api_key_id) DO NOTHING RETURNING id 判定是否新插入；
// MySQL 版改用 ON DUPLICATE KEY UPDATE id = id（幂等 no-op）+ RowsAffected 判定：
//   - RowsAffected >= 1：新插入，随后与 PG 相同地校验归档表指纹；
//   - RowsAffected == 0：命中唯一键 (request_id, api_key_id) 的既有行，走冲突分支。
func (r *usageBillingRepository) claimUsageBillingRequestMySQL(ctx context.Context, tx *sql.Tx, requestID string, apiKeyID int64, requestFingerprint string) (bool, error) {
	res, err := tx.ExecContext(ctx, `
		INSERT INTO usage_billing_dedup (request_id, api_key_id, request_fingerprint)
		VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE id = id
	`, requestID, apiKeyID, requestFingerprint)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected == 0 {
		var existingFingerprint string
		if err := tx.QueryRowContext(ctx, `
			SELECT request_fingerprint
			FROM usage_billing_dedup
			WHERE request_id = ? AND api_key_id = ?
		`, requestID, apiKeyID).Scan(&existingFingerprint); err != nil {
			return false, err
		}
		if strings.TrimSpace(existingFingerprint) != strings.TrimSpace(requestFingerprint) {
			return false, service.ErrUsageBillingRequestConflict
		}
		return false, nil
	}
	var archivedFingerprint string
	err = tx.QueryRowContext(ctx, `
		SELECT request_fingerprint
		FROM usage_billing_dedup_archive
		WHERE request_id = ? AND api_key_id = ?
	`, requestID, apiKeyID).Scan(&archivedFingerprint)
	if err == nil {
		if strings.TrimSpace(archivedFingerprint) != strings.TrimSpace(requestFingerprint) {
			return false, service.ErrUsageBillingRequestConflict
		}
		return false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	return true, nil
}

// incrementUsageBillingSubscriptionMySQL 是 incrementUsageBillingSubscription 的 MySQL 实现：
// PG 的 UPDATE..FROM groups 改写为多表 UPDATE JOIN；`groups` 在 MySQL 8 是保留字，
// 必须反引号。额度三列都严格增加（costUSD > 0），RowsAffected 可靠区分命中与否。
func incrementUsageBillingSubscriptionMySQL(ctx context.Context, tx *sql.Tx, subscriptionID int64, costUSD float64) error {
	updateSQL := `
		UPDATE user_subscriptions us
		JOIN ` + "`groups`" + ` g
			ON g.id = us.group_id AND g.deleted_at IS NULL
		SET
			us.daily_usage_usd = us.daily_usage_usd + ?,
			us.weekly_usage_usd = us.weekly_usage_usd + ?,
			us.monthly_usage_usd = us.monthly_usage_usd + ?,
			us.updated_at = NOW(3)
		WHERE us.id = ? AND us.deleted_at IS NULL
	`
	res, err := tx.ExecContext(ctx, updateSQL, costUSD, costUSD, costUSD, subscriptionID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected > 0 {
		return nil
	}
	return service.ErrSubscriptionNotFound
}

// deductUsageBillingBalanceMySQL 是 deductUsageBillingBalance 的 MySQL 实现：
// 与 PG 版相同地先走「余额充足」条件 UPDATE，未命中再走无条件 UPDATE（允许透支），
// 最后回查扣减后的余额（对齐 RETURNING 的更新后语义）。仍无命中行说明用户不存在。
func deductUsageBillingBalanceMySQL(ctx context.Context, tx *sql.Tx, userID int64, amount float64) (float64, bool, error) {
	res, err := tx.ExecContext(ctx, `
		UPDATE users
		SET balance = balance - ?,
			updated_at = NOW(3)
		WHERE id = ? AND deleted_at IS NULL AND balance >= ?
	`, amount, userID, amount)
	if err != nil {
		return 0, false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, false, err
	}
	sufficient := affected > 0
	if !sufficient {
		res, err = tx.ExecContext(ctx, `
			UPDATE users
			SET balance = balance - ?,
				updated_at = NOW(3)
			WHERE id = ? AND deleted_at IS NULL
		`, amount, userID)
		if err != nil {
			return 0, false, err
		}
		affected, err = res.RowsAffected()
		if err != nil {
			return 0, false, err
		}
		if affected == 0 {
			return 0, false, service.ErrUserNotFound
		}
	}
	var newBalance float64
	if err := tx.QueryRowContext(ctx, `SELECT balance FROM users WHERE id = ?`, userID).Scan(&newBalance); err != nil {
		return 0, false, err
	}
	return newBalance, sufficient, nil
}

// reserveUsageBillingBatchImageBalanceMySQL 是 reserveUsageBillingBatchImageBalance 的 MySQL 实现：
// 条件 UPDATE（余额充足）+ 回查 (balance, frozen_balance)，未命中行的错误分支与 PG 一致。
func reserveUsageBillingBatchImageBalanceMySQL(ctx context.Context, tx *sql.Tx, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	if cmd.HoldAmount <= 0 {
		return &service.BatchImageBalanceHoldResult{}, nil
	}
	res, err := tx.ExecContext(ctx, `
		UPDATE users
		SET balance = balance - ?,
			frozen_balance = COALESCE(frozen_balance, 0) + ?,
			updated_at = NOW(3)
		WHERE id = ? AND deleted_at IS NULL AND balance >= ?
	`, cmd.HoldAmount, cmd.HoldAmount, cmd.UserID, cmd.HoldAmount)
	if err != nil {
		return nil, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		if exists, existsErr := userExistsForBilling(ctx, tx, cmd.UserID); existsErr != nil {
			return nil, existsErr
		} else if !exists {
			return nil, service.ErrUserNotFound
		}
		return nil, service.ErrBatchImageInsufficientBalance
	}
	var balance, frozen float64
	if err := tx.QueryRowContext(ctx, `SELECT balance, frozen_balance FROM users WHERE id = ?`, cmd.UserID).Scan(&balance, &frozen); err != nil {
		return nil, err
	}
	return &service.BatchImageBalanceHoldResult{NewBalance: &balance, FrozenBalance: &frozen}, nil
}

// captureUsageBillingBatchImageBalanceMySQL 是 captureUsageBillingBatchImageBalance 的 MySQL 实现：
// SET 内的多退少补 CASE 表达式与冻结扣减逐字对齐 PG 版，仅占位符与 NOW(3) 不同；
// 回查 (balance, frozen_balance) 对齐 RETURNING 的更新后语义。
// REVIEW: MySQL 默认 RowsAffected 返回"实际变更行数"，当 HoldAmount=0 且
// ActualAmount ∈ (0, 0.00000001] 时余额列可能按 DECIMAL(20,8) 舍入后不变，
// 会被误判为冻结不足（PG 会成功返回）；该金额在业务上不可达，仅作记录。
func captureUsageBillingBatchImageBalanceMySQL(ctx context.Context, tx *sql.Tx, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	if cmd.HoldAmount <= 0 && cmd.ActualAmount <= 0 {
		return &service.BatchImageBalanceHoldResult{}, nil
	}
	if cmd.ActualAmount-cmd.HoldAmount > 0.00000001 {
		return nil, service.ErrBatchImageSettlementCostExceedsHold
	}
	res, err := tx.ExecContext(ctx, `
		UPDATE users
		SET balance = balance
				+ CASE WHEN ? > ? THEN ? - ? ELSE 0 END
				- CASE WHEN ? > ? THEN ? - ? ELSE 0 END,
			frozen_balance = COALESCE(frozen_balance, 0) - ?,
			updated_at = NOW(3)
		WHERE id = ? AND deleted_at IS NULL AND COALESCE(frozen_balance, 0) >= ?
	`,
		cmd.HoldAmount, cmd.ActualAmount, cmd.HoldAmount, cmd.ActualAmount,
		cmd.ActualAmount, cmd.HoldAmount, cmd.ActualAmount, cmd.HoldAmount,
		cmd.HoldAmount, cmd.UserID, cmd.HoldAmount)
	if err != nil {
		return nil, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		if exists, existsErr := userExistsForBilling(ctx, tx, cmd.UserID); existsErr != nil {
			return nil, existsErr
		} else if !exists {
			return nil, service.ErrUserNotFound
		}
		return nil, errors.New("batch image frozen balance is insufficient")
	}
	var balance, frozen float64
	if err := tx.QueryRowContext(ctx, `SELECT balance, frozen_balance FROM users WHERE id = ?`, cmd.UserID).Scan(&balance, &frozen); err != nil {
		return nil, err
	}
	return &service.BatchImageBalanceHoldResult{NewBalance: &balance, FrozenBalance: &frozen}, nil
}

// releaseUsageBillingBatchImageBalanceMySQL 是 releaseUsageBillingBatchImageBalance 的 MySQL 实现：
// hold claim 校验、条件 UPDATE（冻结充足）与错误分支均与 PG 版一致。
func releaseUsageBillingBatchImageBalanceMySQL(ctx context.Context, tx *sql.Tx, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	if cmd.HoldAmount <= 0 {
		return &service.BatchImageBalanceHoldResult{}, nil
	}
	// 释放前校验该 job 确实预留过 hold（hold request id 已被 claim），
	// 防止从未成功冻结的 job 触发"幻影释放"，从其他用户的冻结资金池中凭空生成余额。
	held, heldErr := batchImageHoldClaimExists(ctx, tx, service.BatchImageHoldRequestID(cmd.BatchID), cmd.APIKeyID)
	if heldErr != nil {
		return nil, heldErr
	}
	if !held {
		logger.LegacyPrintf("repository.usage_billing", "[BatchImage] release skipped, hold was never reserved: batch=%s", cmd.BatchID)
		return &service.BatchImageBalanceHoldResult{}, nil
	}
	res, err := tx.ExecContext(ctx, `
		UPDATE users
		SET balance = balance + ?,
			frozen_balance = COALESCE(frozen_balance, 0) - ?,
			updated_at = NOW(3)
		WHERE id = ? AND deleted_at IS NULL AND COALESCE(frozen_balance, 0) >= ?
	`, cmd.HoldAmount, cmd.HoldAmount, cmd.UserID, cmd.HoldAmount)
	if err != nil {
		return nil, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		if exists, existsErr := userExistsForBilling(ctx, tx, cmd.UserID); existsErr != nil {
			return nil, existsErr
		} else if !exists {
			return nil, service.ErrUserNotFound
		}
		return nil, errors.New("batch image frozen balance is insufficient")
	}
	var balance, frozen float64
	if err := tx.QueryRowContext(ctx, `SELECT balance, frozen_balance FROM users WHERE id = ?`, cmd.UserID).Scan(&balance, &frozen); err != nil {
		return nil, err
	}
	return &service.BatchImageBalanceHoldResult{NewBalance: &balance, FrozenBalance: &frozen}, nil
}

// batchImageHoldClaimExistsMySQL 是 batchImageHoldClaimExists 的 MySQL 实现（? 占位符）。
func batchImageHoldClaimExistsMySQL(ctx context.Context, tx *sql.Tx, holdRequestID string, apiKeyID int64) (bool, error) {
	var exists int
	err := tx.QueryRowContext(ctx, `
		SELECT 1
		FROM usage_billing_dedup
		WHERE request_id = ? AND api_key_id = ?
	`, holdRequestID, apiKeyID).Scan(&exists)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	err = tx.QueryRowContext(ctx, `
		SELECT 1
		FROM usage_billing_dedup_archive
		WHERE request_id = ? AND api_key_id = ?
	`, holdRequestID, apiKeyID).Scan(&exists)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return false, err
}

// userExistsForBillingMySQL 是 userExistsForBilling 的 MySQL 实现（? 占位符）。
func userExistsForBillingMySQL(ctx context.Context, tx *sql.Tx, userID int64) (bool, error) {
	var exists int
	err := tx.QueryRowContext(ctx, `
		SELECT 1
		FROM users
		WHERE id = ? AND deleted_at IS NULL
	`, userID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// incrementUsageBillingAPIKeyQuotaMySQL 是 incrementUsageBillingAPIKeyQuota 的 MySQL 实现。
// PG 版 RETURNING 的布尔表达式以「更新后的行」求值（quota_used 已包含本次增量，
// `quota_used - $1` 还原递增前的值）；UPDATE 命中后在同一事务上用同一表达式回查，语义一致。
func incrementUsageBillingAPIKeyQuotaMySQL(ctx context.Context, tx *sql.Tx, apiKeyID int64, amount float64) (bool, error) {
	// MySQL 的 UPDATE SET 赋值按出现顺序从左到右求值（后面的赋值读到前面赋值后的新值），
	// 与 PG「所有赋值表达式一律读旧行值」不同。因此把 status 的 CASE 放在
	// quota_used 赋值之前，保证其读到的 quota_used 是递增前的旧值，与 PG 语义一致。
	res, err := tx.ExecContext(ctx, `
		UPDATE api_keys
		SET status = CASE
				WHEN quota > 0
					AND status = ?
					AND quota_used < quota
					AND quota_used + ? >= quota
				THEN ?
				ELSE status
			END,
			quota_used = quota_used + ?,
			updated_at = NOW(3)
		WHERE id = ? AND deleted_at IS NULL
	`, service.StatusAPIKeyActive, amount, service.StatusAPIKeyQuotaExhausted, amount, apiKeyID)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected == 0 {
		return false, service.ErrAPIKeyNotFound
	}
	var exhausted bool
	if err := tx.QueryRowContext(ctx, `
		SELECT quota > 0 AND quota_used >= quota AND quota_used - ? < quota
		FROM api_keys
		WHERE id = ?
	`, amount, apiKeyID).Scan(&exhausted); err != nil {
		return false, err
	}
	return exhausted, nil
}

// incrementUsageBillingAPIKeyRateLimitMySQL 是 incrementUsageBillingAPIKeyRateLimit 的 MySQL 实现：
// INTERVAL '5 hours' → INTERVAL 5 HOUR（数字无引号、单位单数），
// date_trunc('day', NOW()) → DATE(NOW(3))，NOW() → NOW(3)。
// 时间窗重置 CASE 的判定与赋值逐条对齐 PG 版（5h 用当前时刻，1d/7d 用当日零点）。
func incrementUsageBillingAPIKeyRateLimitMySQL(ctx context.Context, tx *sql.Tx, apiKeyID int64, cost float64) error {
	res, err := tx.ExecContext(ctx, `
		UPDATE api_keys SET
			usage_5h = CASE WHEN window_5h_start IS NOT NULL AND window_5h_start + INTERVAL 5 HOUR <= NOW(3) THEN ? ELSE usage_5h + ? END,
			usage_1d = CASE WHEN window_1d_start IS NOT NULL AND window_1d_start + INTERVAL 24 HOUR <= NOW(3) THEN ? ELSE usage_1d + ? END,
			usage_7d = CASE WHEN window_7d_start IS NOT NULL AND window_7d_start + INTERVAL 7 DAY <= NOW(3) THEN ? ELSE usage_7d + ? END,
			window_5h_start = CASE WHEN window_5h_start IS NULL OR window_5h_start + INTERVAL 5 HOUR <= NOW(3) THEN NOW(3) ELSE window_5h_start END,
			window_1d_start = CASE WHEN window_1d_start IS NULL OR window_1d_start + INTERVAL 24 HOUR <= NOW(3) THEN DATE(NOW(3)) ELSE window_1d_start END,
			window_7d_start = CASE WHEN window_7d_start IS NULL OR window_7d_start + INTERVAL 7 DAY <= NOW(3) THEN DATE(NOW(3)) ELSE window_7d_start END,
			updated_at = NOW(3)
		WHERE id = ? AND deleted_at IS NULL
	`, cost, cost, cost, cost, cost, cost, apiKeyID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrAPIKeyNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// MySQL 版账号配额累加表达式（对应上方 PG 常量 accountQuotaTotalIncrementExpr 等）。
//
// 对应关系与语义约定：
//   - PG jsonb `||`（浅合并）→ JSON_MERGE_PATCH。本文件所有合并的右侧值均为标量，
//     RFC 7396 的递归合并此时与 `||` 等价；JSON null 值会导致键被删除，
//     因此 reset_at 仅在计算结果非 NULL 时才进入 JSON_OBJECT（与 PG 的 IS NOT NULL 守卫一致）。
//   - extra->>'key' → JSON_UNQUOTE(JSON_EXTRACT(extra, '$.key'))；::numeric → CAST AS DECIMAL(20,10)；
//     jsonb_typeof 'null'/'number' → JSON_TYPE 'NULL'/数字类型集合（大写）。
//   - quota_daily_start / quota_*_reset_at 均为 UTC RFC3339 字符串（PG nowUTC 与
//     Go 层 resetAt.UTC().Format(time.RFC3339) 写入）。MySQL 侧比较前先 REPLACE 掉
//     'T' 与 'Z' 再 CAST AS DATETIME(6)，避免依赖服务端对带时区偏移字面量的解析行为，
//     直接按 UTC 墙钟与 UTC_TIMESTAMP(3) 比较。
//   - 固定重置模式的时区换算用 CONVERT_TZ：'UTC'（默认值）归一为偏移 '+00:00'，
//     不依赖服务器时区表；REVIEW: 其他 IANA 时区（如 'Asia/Shanghai'）要求服务器已通过
//     mysql_tzinfo_to_sql 加载时区表，否则 CONVERT_TZ 返回 NULL，表现为 fixed 模式
//     不写 reset_at（滚动模式与 total/daily/weekly 计数不受影响）。
// ---------------------------------------------------------------------------

const accountQuotaEpochDTMySQL = `'1970-01-01 00:00:00'`

// accountQuotaNowUTCMySQL 对应 PG nowUTC：UTC RFC3339 字符串（微秒位对齐 'US' 格式）。
const accountQuotaNowUTCMySQL = `DATE_FORMAT(UTC_TIMESTAMP(3), '%Y-%m-%dT%H:%i:%s.%fZ')`

// accountQuotaExtraTextMySQL 是 MySQL 版 extra->>'path'。
func accountQuotaExtraTextMySQL(path string) string {
	return `JSON_UNQUOTE(JSON_EXTRACT(extra, '$.` + path + `'))`
}

// accountQuotaExtraNumberMySQL 是 MySQL 版 (extra->>'path')::numeric。
func accountQuotaExtraNumberMySQL(path string) string {
	return `CAST(` + accountQuotaExtraTextMySQL(path) + ` AS DECIMAL(20,10))`
}

// accountQuotaExtraDTMySQL 把 extra 中的 UTC RFC3339 字符串转成 UTC DATETIME(6)。
func accountQuotaExtraDTMySQL(path string) string {
	return `CAST(REPLACE(REPLACE(` + accountQuotaExtraTextMySQL(path) + `, 'T', ' '), 'Z', '') AS DATETIME(6))`
}

// accountQuotaTZCaseMySQL 对应 PG 的 COALESCE(extra->>'quota_reset_timezone', 'UTC')：
// 'UTC' 与未设置统一归一为偏移 '+00:00'（CONVERT_TZ 用偏移不依赖时区表）。
var accountQuotaTZCaseMySQL = `(CASE WHEN COALESCE(` + accountQuotaExtraTextMySQL("quota_reset_timezone") + `, 'UTC') = 'UTC' THEN '+00:00' ELSE ` + accountQuotaExtraTextMySQL("quota_reset_timezone") + ` END)`

// accountQuotaNowLocalDTMySQL 对应 PG 的 NOW() AT TIME ZONE tz：tz 时区墙钟。
var accountQuotaNowLocalDTMySQL = `CONVERT_TZ(UTC_TIMESTAMP(3), '+00:00', ` + accountQuotaTZCaseMySQL + `)`

// accountQuotaDailyExpiredExprMySQL 对应 PG dailyExpiredExpr：
// fixed 模式比较 reset_at，rolling 模式比较 start + 24h，均按 UTC 墙钟。
var accountQuotaDailyExpiredExprMySQL = `(
		CASE WHEN COALESCE(` + accountQuotaExtraTextMySQL("quota_daily_reset_mode") + `, 'rolling') = 'fixed'
		THEN UTC_TIMESTAMP(3) >= COALESCE(` + accountQuotaExtraDTMySQL("quota_daily_reset_at") + `, ` + accountQuotaEpochDTMySQL + `)
		ELSE COALESCE(` + accountQuotaExtraDTMySQL("quota_daily_start") + `, ` + accountQuotaEpochDTMySQL + `) + INTERVAL 24 HOUR <= UTC_TIMESTAMP(3)
		END
	)`

// accountQuotaWeeklyExpiredExprMySQL 对应 PG weeklyExpiredExpr（rolling 周期为 168 小时）。
var accountQuotaWeeklyExpiredExprMySQL = `(
		CASE WHEN COALESCE(` + accountQuotaExtraTextMySQL("quota_weekly_reset_mode") + `, 'rolling') = 'fixed'
		THEN UTC_TIMESTAMP(3) >= COALESCE(` + accountQuotaExtraDTMySQL("quota_weekly_reset_at") + `, ` + accountQuotaEpochDTMySQL + `)
		ELSE COALESCE(` + accountQuotaExtraDTMySQL("quota_weekly_start") + `, ` + accountQuotaEpochDTMySQL + `) + INTERVAL 168 HOUR <= UTC_TIMESTAMP(3)
		END
	)`

// accountQuotaDailyResetLocalMySQL / accountQuotaDailyResetUTCMySQL：
// 今日重置点（tz 墙钟 / 换算回 UTC），对应 PG 的
// date_trunc('day', NOW() AT TIME ZONE tz) + reset_hour hours 及其 AT TIME ZONE tz。
var accountQuotaDailyResetLocalMySQL = `(DATE(` + accountQuotaNowLocalDTMySQL + `) + INTERVAL COALESCE(CAST(` + accountQuotaExtraTextMySQL("quota_daily_reset_hour") + ` AS SIGNED), 0) HOUR)`
var accountQuotaDailyResetUTCMySQL = `CONVERT_TZ(` + accountQuotaDailyResetLocalMySQL + `, ` + accountQuotaTZCaseMySQL + `, '+00:00')`

// accountQuotaNextDailyResetAtExprMySQL 对应 PG nextDailyResetAtExpr：
// fixed 模式返回下一个重置点的 UTC RFC3339 字符串（不带小数秒），rolling 模式为 NULL。
var accountQuotaNextDailyResetAtExprMySQL = `(
	CASE WHEN COALESCE(` + accountQuotaExtraTextMySQL("quota_daily_reset_mode") + `, 'rolling') = 'fixed'
	THEN DATE_FORMAT(CONVERT_TZ((
			CASE WHEN UTC_TIMESTAMP(3) >= ` + accountQuotaDailyResetUTCMySQL + `
			THEN ` + accountQuotaDailyResetLocalMySQL + ` + INTERVAL 1 DAY
			ELSE ` + accountQuotaDailyResetLocalMySQL + `
			END
		), ` + accountQuotaTZCaseMySQL + `, '+00:00'), '%Y-%m-%dT%H:%i:%SZ')
	ELSE NULL END
)`

// accountQuotaWeeklyResetLocalMySQL / accountQuotaWeeklyResetUTCMySQL：本周重置点。
var accountQuotaWeeklyResetLocalMySQL = `(DATE(` + accountQuotaNowLocalDTMySQL + `) + INTERVAL COALESCE(CAST(` + accountQuotaExtraTextMySQL("quota_weekly_reset_hour") + ` AS SIGNED), 0) HOUR)`
var accountQuotaWeeklyResetUTCMySQL = `CONVERT_TZ(` + accountQuotaWeeklyResetLocalMySQL + `, ` + accountQuotaTZCaseMySQL + `, '+00:00')`

// accountQuotaWeeklyDaysForwardMySQL：目标星期（PG DOW 语义，周日=0）与今日的间隔天数。
// MySQL DAYOFWEEK 周日=1，比 PG DOW 大 1，故减 1 对齐。
var accountQuotaWeeklyDaysForwardMySQL = `((COALESCE(CAST(` + accountQuotaExtraTextMySQL("quota_weekly_reset_day") + ` AS SIGNED), 1) - (DAYOFWEEK(` + accountQuotaNowLocalDTMySQL + `) - 1) + 7) % 7)`

// accountQuotaNextWeeklyResetAtExprMySQL 对应 PG nextWeeklyResetAtExpr：
// 同星期且已过重置点时顺延一周，否则前进 days_forward 天，输出 UTC RFC3339 字符串。
var accountQuotaNextWeeklyResetAtExprMySQL = `(
	CASE WHEN COALESCE(` + accountQuotaExtraTextMySQL("quota_weekly_reset_mode") + `, 'rolling') = 'fixed'
	THEN DATE_FORMAT(CONVERT_TZ((
			CASE
			WHEN ` + accountQuotaWeeklyDaysForwardMySQL + ` = 0 AND UTC_TIMESTAMP(3) >= ` + accountQuotaWeeklyResetUTCMySQL + `
			THEN ` + accountQuotaWeeklyResetLocalMySQL + ` + INTERVAL 7 DAY
			ELSE ` + accountQuotaWeeklyResetLocalMySQL + ` + INTERVAL ` + accountQuotaWeeklyDaysForwardMySQL + ` DAY
			END
		), ` + accountQuotaTZCaseMySQL + `, '+00:00'), '%Y-%m-%dT%H:%i:%SZ')
	ELSE NULL END
)`

// 三个条件对应 PG accountQuotaExtraIsObjectCond / accountQuotaDailyLimitUnsetCond /
// accountQuotaWeeklyLimitUnsetCond。JSON_EXTRACT 对缺失路径返回 SQL NULL、对 JSON null
// 返回 JSON null 字面量，与 PG `->` 的区分一致；JSON 与 JSON 的比较遵循 PG 式的类型
// 排序（字符串不会与数字比较），`<= CAST('0' AS JSON)` 仅对数字类型生效。
var accountQuotaExtraIsObjectCondMySQL = `JSON_TYPE(extra) = 'OBJECT'`

var accountQuotaDailyLimitUnsetCondMySQL = `(JSON_EXTRACT(extra, '$.quota_daily_limit') IS NULL
			OR JSON_TYPE(JSON_EXTRACT(extra, '$.quota_daily_limit')) = 'NULL'
			OR (JSON_TYPE(JSON_EXTRACT(extra, '$.quota_daily_limit')) IN ('INTEGER', 'DOUBLE', 'DECIMAL', 'UNSIGNED')
				AND JSON_EXTRACT(extra, '$.quota_daily_limit') <= CAST('0' AS JSON)))`

var accountQuotaWeeklyLimitUnsetCondMySQL = `(JSON_EXTRACT(extra, '$.quota_weekly_limit') IS NULL
			OR JSON_TYPE(JSON_EXTRACT(extra, '$.quota_weekly_limit')) = 'NULL'
			OR (JSON_TYPE(JSON_EXTRACT(extra, '$.quota_weekly_limit')) IN ('INTEGER', 'DOUBLE', 'DECIMAL', 'UNSIGNED')
				AND JSON_EXTRACT(extra, '$.quota_weekly_limit') <= CAST('0' AS JSON)))`

// accountQuotaTotalIncrementPatchArgMySQL 对应 accountQuotaTotalIncrementExpr：
// 累加总额度已用量（PATCH 参数列表的第一项）。
var accountQuotaTotalIncrementPatchArgMySQL = `JSON_OBJECT('quota_used', COALESCE(` + accountQuotaExtraNumberMySQL("quota_used") + `, 0) + ?)`

// accountQuotaDailyIncrementPatchArgMySQL 对应 accountQuotaDailyIncrementExpr：
// 设置了日上限时累加日用量并按需重置周期（PATCH 参数列表的一项，未设置时整项为空对象）。
var accountQuotaDailyIncrementPatchArgMySQL = `CASE WHEN COALESCE(` + accountQuotaExtraNumberMySQL("quota_daily_limit") + `, 0) > 0 THEN
					JSON_MERGE_PATCH(
						JSON_OBJECT(
							'quota_daily_used',
							CASE WHEN ` + accountQuotaDailyExpiredExprMySQL + `
							THEN ? ELSE COALESCE(` + accountQuotaExtraNumberMySQL("quota_daily_used") + `, 0) + ? END,
							'quota_daily_start',
							CASE WHEN ` + accountQuotaDailyExpiredExprMySQL + `
							THEN ` + accountQuotaNowUTCMySQL + `
							ELSE COALESCE(` + accountQuotaExtraTextMySQL("quota_daily_start") + `, ` + accountQuotaNowUTCMySQL + `) END
						),
						CASE WHEN ` + accountQuotaDailyExpiredExprMySQL + ` AND ` + accountQuotaNextDailyResetAtExprMySQL + ` IS NOT NULL
						THEN JSON_OBJECT('quota_daily_reset_at', ` + accountQuotaNextDailyResetAtExprMySQL + `)
						ELSE CAST('{}' AS JSON) END
					)
				ELSE CAST('{}' AS JSON) END`

// accountQuotaWeeklyIncrementPatchArgMySQL 对应 accountQuotaWeeklyIncrementExpr（周维度同上）。
var accountQuotaWeeklyIncrementPatchArgMySQL = `CASE WHEN COALESCE(` + accountQuotaExtraNumberMySQL("quota_weekly_limit") + `, 0) > 0 THEN
					JSON_MERGE_PATCH(
						JSON_OBJECT(
							'quota_weekly_used',
							CASE WHEN ` + accountQuotaWeeklyExpiredExprMySQL + `
							THEN ? ELSE COALESCE(` + accountQuotaExtraNumberMySQL("quota_weekly_used") + `, 0) + ? END,
							'quota_weekly_start',
							CASE WHEN ` + accountQuotaWeeklyExpiredExprMySQL + `
							THEN ` + accountQuotaNowUTCMySQL + `
							ELSE COALESCE(` + accountQuotaExtraTextMySQL("quota_weekly_start") + `, ` + accountQuotaNowUTCMySQL + `) END
						),
						CASE WHEN ` + accountQuotaWeeklyExpiredExprMySQL + ` AND ` + accountQuotaNextWeeklyResetAtExprMySQL + ` IS NOT NULL
						THEN JSON_OBJECT('quota_weekly_reset_at', ` + accountQuotaNextWeeklyResetAtExprMySQL + `)
						ELSE CAST('{}' AS JSON) END
					)
				ELSE CAST('{}' AS JSON) END`

// accountQuotaIncrementSelectMySQL 是 UPDATE 后的回查语句，六列顺序与
// PG accountQuotaIncrementReturning 的 RETURNING 列表一致（更新后的 extra 值）。
var accountQuotaIncrementSelectMySQL = `
			SELECT
				COALESCE(` + accountQuotaExtraNumberMySQL("quota_used") + `, 0),
				COALESCE(` + accountQuotaExtraNumberMySQL("quota_limit") + `, 0),
				COALESCE(` + accountQuotaExtraNumberMySQL("quota_daily_used") + `, 0),
				COALESCE(` + accountQuotaExtraNumberMySQL("quota_daily_limit") + `, 0),
				COALESCE(` + accountQuotaExtraNumberMySQL("quota_weekly_used") + `, 0),
				COALESCE(` + accountQuotaExtraNumberMySQL("quota_weekly_limit") + `, 0)
			FROM accounts WHERE id = ?`

// accountQuotaIncrementSQLsMySQL 按 accountQuotaIncrementSQLIndex 存放 MySQL 版账号配额
// 累加语句。语句结构与 PG 版一致：省略日或周维度时在 WHERE 附加对象/未设置条件，
// 命中 0 行时由调用方回退到完整语句；extra 的合并改为 JSON_MERGE_PATCH 的扁平参数列表
// （base, total, [daily], [weekly]），各参数无重复键，与 PG 的左结合 `||` 链等价。
var accountQuotaIncrementSQLsMySQL = buildAccountQuotaIncrementSQLsMySQL()

func buildAccountQuotaIncrementSQLsMySQL() [4]string {
	var sqls [4]string
	for _, daily := range []bool{false, true} {
		for _, weekly := range []bool{false, true} {
			patchArgs := []string{
				"\n\t\t\t\t" + accountQuotaTotalIncrementPatchArgMySQL,
			}
			where := "id = ? AND deleted_at IS NULL"
			if !daily || !weekly {
				where += "\n\t\t\tAND " + accountQuotaExtraIsObjectCondMySQL
			}
			if daily {
				patchArgs = append(patchArgs, "\n\t\t\t\t"+accountQuotaDailyIncrementPatchArgMySQL)
			} else {
				where += "\n\t\t\tAND " + accountQuotaDailyLimitUnsetCondMySQL
			}
			if weekly {
				patchArgs = append(patchArgs, "\n\t\t\t\t"+accountQuotaWeeklyIncrementPatchArgMySQL)
			} else {
				where += "\n\t\t\tAND " + accountQuotaWeeklyLimitUnsetCondMySQL
			}
			set := "JSON_MERGE_PATCH(\n\t\t\t\tCOALESCE(extra, CAST('{}' AS JSON))," + strings.Join(patchArgs, ",") + "\n\t\t\t)"
			sqls[accountQuotaIncrementSQLIndex(daily, weekly)] = "UPDATE accounts SET extra = (\n\t\t\t" + set +
				"\n\t\t), updated_at = NOW(3)\n\t\tWHERE " + where
		}
	}
	return sqls
}

// incrementUsageBillingAccountQuotaMySQL 是 incrementUsageBillingAccountQuota 的 MySQL 实现：
// 语句选择/回退/超限刷调度快照逻辑与 PG 版一致，仅执行方式改为
// 「条件 UPDATE + 同事务回查更新后的 extra 配额字段」。
func incrementUsageBillingAccountQuotaMySQL(ctx context.Context, tx *sql.Tx, accountID int64, amount float64, hasDaily, hasWeekly bool) (*service.AccountQuotaState, error) {
	index := accountQuotaIncrementSQLIndex(hasDaily, hasWeekly)
	fullIndex := accountQuotaIncrementSQLIndex(true, true)
	state, err := execAccountQuotaIncrementMySQL(ctx, tx, accountQuotaIncrementSQLsMySQL[index], hasDaily, hasWeekly, accountID, amount)
	if err != nil {
		return nil, err
	}
	if state == nil && index != fullIndex {
		state, err = execAccountQuotaIncrementMySQL(ctx, tx, accountQuotaIncrementSQLsMySQL[fullIndex], true, true, accountID, amount)
		if err != nil {
			return nil, err
		}
	}
	if state == nil {
		return nil, service.ErrAccountNotFound
	}
	// 与 PG 版相同的超限跨越判定与调度快照刷新。
	crossedTotal := state.TotalLimit > 0 && state.TotalUsed >= state.TotalLimit && (state.TotalUsed-amount) < state.TotalLimit
	crossedDaily := state.DailyLimit > 0 && state.DailyUsed >= state.DailyLimit && (state.DailyUsed-amount) < state.DailyLimit
	crossedWeekly := state.WeeklyLimit > 0 && state.WeeklyUsed >= state.WeeklyLimit && (state.WeeklyUsed-amount) < state.WeeklyLimit
	if crossedTotal || crossedDaily || crossedWeekly {
		// REVIEW: enqueueSchedulerOutbox 当前仍为 PG 专用语句（$n 占位符 +
		// ON CONFLICT），MySQL 方言化在其自身文件中完成；此处调用在 MySQL 上会报错，
		// 与 PG 版一致地作为错误上抛（计费结果仍已在本事务内生效/回滚）。
		if err := enqueueSchedulerOutbox(ctx, tx, service.SchedulerOutboxEventAccountChanged, &accountID, nil, nil); err != nil {
			logger.LegacyPrintf("repository.usage_billing", "[SchedulerOutbox] enqueue quota exceeded failed: account=%d err=%v", accountID, err)
			return nil, err
		}
	}
	return state, nil
}

// execAccountQuotaIncrementMySQL 是 execAccountQuotaIncrement 的 MySQL 实现：
// UPDATE 命中后回查更新后的 extra 配额字段（对齐 PG RETURNING 的更新后行语义）；
// 未命中任何行（账号不存在或省略维度的附加条件不成立）返回 nil 状态。
// amount 占位符数量：total 1 个 + daily/weekly 各 2 个 + WHERE id 1 个。
func execAccountQuotaIncrementMySQL(ctx context.Context, tx *sql.Tx, query string, daily, weekly bool, accountID int64, amount float64) (*service.AccountQuotaState, error) {
	args := make([]any, 0, 7)
	args = append(args, amount)
	if daily {
		args = append(args, amount, amount)
	}
	if weekly {
		args = append(args, amount, amount)
	}
	args = append(args, accountID)

	res, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, nil
	}
	rows, err := tx.QueryContext(ctx, accountQuotaIncrementSelectMySQL, accountID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var state *service.AccountQuotaState
	if rows.Next() {
		state = &service.AccountQuotaState{}
		if err := rows.Scan(
			&state.TotalUsed, &state.TotalLimit,
			&state.DailyUsed, &state.DailyLimit,
			&state.WeeklyUsed, &state.WeeklyLimit,
		); err != nil {
			return nil, err
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return state, nil
}
