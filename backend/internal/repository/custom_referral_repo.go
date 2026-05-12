package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/moneyx"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/shopspring/decimal"
)

const (
	customInviteCodeLength      = 10
	customInviteCodeMaxAttempts = 12
)

var customInviteCodeCharset = []byte("ABCDEFGHJKLMNPQRSTUVWXYZ23456789")

func loadTableColumns(ctx context.Context, exec sqlQueryExecutor, table string) (map[string]struct{}, error) {
	rows, err := exec.QueryContext(ctx, fmt.Sprintf("SELECT * FROM %s LIMIT 0", table))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	names, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	columns := make(map[string]struct{}, len(names))
	for _, name := range names {
		columns[strings.TrimSpace(name)] = struct{}{}
	}
	return columns, nil
}

func hasTableColumn(columns map[string]struct{}, name string) bool {
	_, ok := columns[strings.TrimSpace(name)]
	return ok
}

func customReferralCommissionOrderFilter(columns map[string]struct{}, alias, placeholder string) string {
	prefix := ""
	if alias = strings.TrimSpace(alias); alias != "" {
		prefix = alias + "."
	}
	placeholder = strings.TrimSpace(placeholder)
	if placeholder == "" {
		placeholder = "$1"
	}
	hasOrderID := hasTableColumn(columns, "order_id")
	hasSourceOrderID := hasTableColumn(columns, "source_order_id")
	switch {
	case hasOrderID && hasSourceOrderID:
		return fmt.Sprintf("(%sorder_id = %s OR %ssource_order_id = %s)", prefix, placeholder, prefix, placeholder)
	case hasSourceOrderID:
		return fmt.Sprintf("%ssource_order_id = %s", prefix, placeholder)
	default:
		return fmt.Sprintf("%sorder_id = %s", prefix, placeholder)
	}
}

func customReferralCommissionOrderSelectExpr(columns map[string]struct{}, alias string) string {
	prefix := ""
	if alias = strings.TrimSpace(alias); alias != "" {
		prefix = alias + "."
	}
	hasOrderID := hasTableColumn(columns, "order_id")
	hasSourceOrderID := hasTableColumn(columns, "source_order_id")
	switch {
	case hasOrderID && hasSourceOrderID:
		return fmt.Sprintf("COALESCE(%sorder_id, %ssource_order_id)", prefix, prefix)
	case hasSourceOrderID:
		return fmt.Sprintf("%ssource_order_id", prefix)
	default:
		return fmt.Sprintf("%sorder_id", prefix)
	}
}

type customCommissionLedgerInsert struct {
	UserID              int64
	AffiliateID         int64
	CommissionID        int64
	RelatedCommissionID int64
	WithdrawalID        int64
	Type                string
	BizType             string
	RefType             string
	RefID               string
	ExternalRefID       string
	DeltaPending        float64
	DeltaAvailable      float64
	DeltaFrozen         float64
	DeltaWithdrawn      float64
	DeltaReversed       float64
	DeltaDebt           float64
	Remark              string
	Operator            string
	OperatorType        string
	OperatorID          int64
}

func normalizeLedgerOperatorType(operatorType, operator string) string {
	if operatorType = strings.TrimSpace(operatorType); operatorType != "" {
		return operatorType
	}
	operator = strings.TrimSpace(operator)
	switch {
	case strings.HasPrefix(operator, "admin"):
		return "admin"
	case strings.HasPrefix(operator, "user"):
		return "user"
	default:
		return "system"
	}
}

func (r *customReferralRepository) insertCommissionLedger(ctx context.Context, exec sqlQueryExecutor, entry customCommissionLedgerInsert) error {
	columns, err := loadTableColumns(ctx, exec, "custom_commission_ledger")
	if err != nil {
		return err
	}
	insertColumns := make([]string, 0, 18)
	insertValues := make([]string, 0, 18)
	insertArgs := make([]any, 0, 18)
	appendInsert := func(column string, value any) {
		if !hasTableColumn(columns, column) {
			return
		}
		insertColumns = append(insertColumns, column)
		insertArgs = append(insertArgs, value)
		insertValues = append(insertValues, fmt.Sprintf("$%d", len(insertArgs)))
	}
	if hasTableColumn(columns, "user_id") {
		if entry.UserID <= 0 {
			return fmt.Errorf("missing ledger user id for ledger type %s", strings.TrimSpace(entry.Type))
		}
		appendInsert("user_id", entry.UserID)
	}
	if entry.AffiliateID > 0 {
		appendInsert("affiliate_id", entry.AffiliateID)
	}
	if entry.CommissionID > 0 {
		appendInsert("commission_id", entry.CommissionID)
	}
	switch {
	case entry.RelatedCommissionID > 0:
		appendInsert("related_commission_id", entry.RelatedCommissionID)
	case entry.CommissionID > 0:
		appendInsert("related_commission_id", entry.CommissionID)
	}
	if entry.WithdrawalID > 0 {
		appendInsert("withdrawal_id", entry.WithdrawalID)
	}
	appendInsert("biz_type", firstNonEmpty(strings.TrimSpace(entry.BizType), strings.TrimSpace(entry.Type)))
	appendInsert("type", strings.TrimSpace(entry.Type))
	appendInsert("ref_type", strings.TrimSpace(entry.RefType))
	appendInsert("ref_id", strings.TrimSpace(entry.RefID))
	appendInsert("external_ref_id", strings.TrimSpace(entry.ExternalRefID))
	appendInsert("delta_pending", entry.DeltaPending)
	appendInsert("delta_available", entry.DeltaAvailable)
	appendInsert("delta_frozen", entry.DeltaFrozen)
	appendInsert("delta_withdrawn", entry.DeltaWithdrawn)
	appendInsert("delta_reversed", entry.DeltaReversed)
	appendInsert("delta_debt", entry.DeltaDebt)
	appendInsert("remark", strings.TrimSpace(entry.Remark))
	appendInsert("operator", firstNonEmpty(strings.TrimSpace(entry.Operator), "system"))
	appendInsert("operator_type", normalizeLedgerOperatorType(entry.OperatorType, entry.Operator))
	appendInsert("operator_id", nilIfZeroInt64(entry.OperatorID))

	_, err = exec.ExecContext(ctx, fmt.Sprintf(`
INSERT INTO custom_commission_ledger (
    %s, created_at
)
VALUES (%s, NOW())`, strings.Join(insertColumns, ", "), strings.Join(insertValues, ", ")), insertArgs...)
	return err
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func nilIfZeroInt64(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

type customReferralRepository struct {
	client *dbent.Client
	sql    *sql.DB
}

func NewCustomReferralRepository(client *dbent.Client, sqlDB *sql.DB) service.CustomReferralRepository {
	return &customReferralRepository{client: client, sql: sqlDB}
}

func (r *customReferralRepository) UpsertApprovedAffiliate(ctx context.Context, userID, adminID int64, rateOverride *float64) (*service.CustomAffiliate, error) {
	var out *service.CustomAffiliate
	err := r.withTx(ctx, func(txCtx context.Context, exec sqlQueryExecutor) error {
		existing, err := r.getAffiliateByUserIDWithExecutor(txCtx, exec, userID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}

		now := time.Now()
		if existing == nil {
			code, codeErr := r.generateUniqueInviteCode(txCtx, exec)
			if codeErr != nil {
				return codeErr
			}
			var rate any
			if rateOverride != nil {
				rate = *rateOverride
			}
			if _, err := exec.ExecContext(txCtx, `
INSERT INTO custom_affiliates (
    user_id, invite_code, status, source_type, rate_override,
    acquisition_enabled, settlement_enabled, withdrawal_enabled,
    approved_by, approved_at, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, TRUE, TRUE, TRUE, $6, $7, NOW(), NOW())`,
				userID,
				code,
				service.CustomAffiliateStatusApproved,
				service.CustomAffiliateSourceAdminCreated,
				rate,
				adminIDOrNil(adminID),
				now,
			); err != nil {
				return err
			}
		} else {
			var rate any
			if rateOverride != nil {
				rate = *rateOverride
			}
			if _, err := exec.ExecContext(txCtx, `
UPDATE custom_affiliates
SET status = $2,
    rate_override = $3,
    acquisition_enabled = TRUE,
    settlement_enabled = TRUE,
    withdrawal_enabled = TRUE,
    risk_reason = '',
    approved_by = $4,
    approved_at = $5,
    disabled_by = NULL,
    disabled_at = NULL,
    updated_at = NOW()
WHERE user_id = $1`,
				userID,
				service.CustomAffiliateStatusApproved,
				rate,
				adminIDOrNil(adminID),
				now,
			); err != nil {
				return err
			}
		}

		if _, err := exec.ExecContext(txCtx, `
INSERT INTO custom_commission_accounts (affiliate_id, user_id, created_at, updated_at)
SELECT a.id, a.user_id, NOW(), NOW()
FROM custom_affiliates a
WHERE a.user_id = $1
  AND NOT EXISTS (
    SELECT 1
    FROM custom_commission_accounts ca
    WHERE ca.affiliate_id = a.id
  )
  AND NOT EXISTS (
    SELECT 1
    FROM custom_commission_accounts ca
    WHERE ca.user_id = a.user_id
  )
ON CONFLICT DO NOTHING`, userID); err != nil {
			return err
		}

		item, err := r.getAffiliateByUserIDWithExecutor(txCtx, exec, userID)
		if err != nil {
			return err
		}
		out = item
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *customReferralRepository) SetAffiliateRateOverride(ctx context.Context, userID int64, rateOverride *float64, audit service.CustomReferralAdminAuditContext) (*service.CustomAffiliate, error) {
	var out *service.CustomAffiliate
	err := r.withTx(ctx, func(txCtx context.Context, exec sqlQueryExecutor) error {
		oldItem, err := r.getAffiliateByUserIDWithExecutor(txCtx, exec, userID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return service.ErrCustomReferralAffiliateNotFound
			}
			return err
		}
		var rate any
		if rateOverride != nil {
			rate = *rateOverride
		}
		res, err := exec.ExecContext(txCtx, `
UPDATE custom_affiliates
SET rate_override = $2,
    updated_at = NOW()
WHERE user_id = $1`,
			userID,
			rate,
		)
		if err != nil {
			return err
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			return service.ErrCustomReferralAffiliateNotFound
		}
		item, err := r.getAffiliateByUserIDWithExecutor(txCtx, exec, userID)
		if err != nil {
			return err
		}
		out = item
		if audit.OldValue == nil {
			audit.OldValue = map[string]any{"rate_override": oldItem.RateOverride}
		}
		if audit.NewValue == nil {
			audit.NewValue = map[string]any{"rate_override": item.RateOverride}
		}
		if strings.TrimSpace(audit.Action) == "" {
			audit.Action = "affiliate_rate_override"
		}
		if err := r.recordAdminAuditWithExecutor(txCtx, exec, userID, item.ID, audit); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *customReferralRepository) UpsertAffiliateApplication(ctx context.Context, userID int64, note string) (*service.CustomAffiliate, error) {
	var out *service.CustomAffiliate
	err := r.withTx(ctx, func(txCtx context.Context, exec sqlQueryExecutor) error {
		existing, err := r.getAffiliateByUserIDWithExecutor(txCtx, exec, userID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}

		if existing == nil {
			code, codeErr := r.generateUniqueInviteCode(txCtx, exec)
			if codeErr != nil {
				return codeErr
			}
			if _, err := exec.ExecContext(txCtx, `
INSERT INTO custom_affiliates (
    user_id, invite_code, status, source_type,
    acquisition_enabled, settlement_enabled, withdrawal_enabled,
    risk_reason, risk_note, created_at, updated_at
) VALUES ($1, $2, $3, $4, FALSE, FALSE, FALSE, '', $5, NOW(), NOW())`,
				userID,
				code,
				service.CustomAffiliateStatusPending,
				service.CustomAffiliateSourceUserApplied,
				note,
			); err != nil {
				return err
			}
		} else {
			if existing.Status == service.CustomAffiliateStatusApproved {
				return service.ErrCustomReferralAlreadyApproved
			}
			if existing.Status == service.CustomAffiliateStatusDisabled {
				return service.ErrCustomReferralAffiliateDisabled
			}
			if _, err := exec.ExecContext(txCtx, `
UPDATE custom_affiliates
SET status = $2,
    source_type = $3,
    acquisition_enabled = FALSE,
    settlement_enabled = FALSE,
    withdrawal_enabled = FALSE,
    risk_reason = '',
    risk_note = $4,
    approved_by = NULL,
    approved_at = NULL,
    disabled_by = NULL,
    disabled_at = NULL,
    updated_at = NOW()
WHERE user_id = $1`,
				userID,
				service.CustomAffiliateStatusPending,
				service.CustomAffiliateSourceUserApplied,
				note,
			); err != nil {
				return err
			}
		}

		item, err := r.getAffiliateByUserIDWithExecutor(txCtx, exec, userID)
		if err != nil {
			return err
		}
		out = item
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *customReferralRepository) SetAffiliateStatus(ctx context.Context, userID, adminID int64, status string, acquisitionEnabled, settlementEnabled, withdrawalEnabled bool, reason string, audit service.CustomReferralAdminAuditContext) (*service.CustomAffiliate, error) {
	var out *service.CustomAffiliate
	err := r.withTx(ctx, func(txCtx context.Context, exec sqlQueryExecutor) error {
		oldItem, err := r.getAffiliateByUserIDWithExecutor(txCtx, exec, userID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return service.ErrCustomReferralAffiliateNotFound
			}
			return err
		}
		var disabledBy any
		var disabledAt any
		if status == service.CustomAffiliateStatusDisabled {
			disabledBy = adminIDOrNil(adminID)
			disabledAt = time.Now()
		}
		res, err := exec.ExecContext(txCtx, `
UPDATE custom_affiliates
SET status = $2,
    acquisition_enabled = $3,
    settlement_enabled = $4,
    withdrawal_enabled = $5,
    risk_reason = $6,
    disabled_by = $7,
    disabled_at = $8,
    updated_at = NOW()
WHERE user_id = $1`,
			userID,
			status,
			acquisitionEnabled,
			settlementEnabled,
			withdrawalEnabled,
			strings.TrimSpace(reason),
			disabledBy,
			disabledAt,
		)
		if err != nil {
			return err
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			return service.ErrCustomReferralAffiliateNotFound
		}
		item, err := r.getAffiliateByUserIDWithExecutor(txCtx, exec, userID)
		if err != nil {
			return err
		}
		out = item
		if audit.OldValue == nil {
			audit.OldValue = customAffiliateStatusAuditValue(oldItem)
		}
		if audit.NewValue == nil {
			audit.NewValue = customAffiliateStatusAuditValue(item)
		}
		if strings.TrimSpace(audit.Action) == "" {
			audit.Action = "affiliate_status_update"
		}
		if audit.AdminUserID <= 0 {
			audit.AdminUserID = adminID
		}
		if audit.Reason == "" {
			audit.Reason = strings.TrimSpace(reason)
		}
		if err := r.recordAdminAuditWithExecutor(txCtx, exec, userID, item.ID, audit); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *customReferralRepository) AdjustAffiliateCommission(ctx context.Context, input service.CustomReferralAdjustInput) (*service.CustomAffiliate, error) {
	if strings.TrimSpace(input.IdempotencyKey) == "" {
		return nil, service.ErrCustomReferralInvalidIdempotency
	}
	var out *service.CustomAffiliate
	err := r.withTx(ctx, func(txCtx context.Context, exec sqlQueryExecutor) error {
		affiliate, err := r.getAffiliateByUserIDWithExecutor(txCtx, exec, input.UserID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return service.ErrCustomReferralAffiliateNotFound
			}
			return err
		}
		externalRefID := "admin_adjust:" + strings.TrimSpace(input.IdempotencyKey)
		existingRows, err := exec.QueryContext(txCtx, `
SELECT 1
FROM custom_commission_ledger
WHERE external_ref_id = $1
  AND type IN ('commission_adjust_increase', 'commission_adjust_decrease')
LIMIT 1
FOR UPDATE`, externalRefID)
		if err != nil {
			return err
		}
		if existingRows.Next() {
			_ = existingRows.Close()
			out, err = r.getAffiliateByUserIDWithExecutor(txCtx, exec, input.UserID)
			return err
		}
		if err := existingRows.Close(); err != nil {
			return err
		}

		if _, err := exec.ExecContext(txCtx, `
INSERT INTO custom_commission_accounts (affiliate_id, user_id, created_at, updated_at)
SELECT $1, $2, NOW(), NOW()
WHERE NOT EXISTS (
    SELECT 1
    FROM custom_commission_accounts ca
    WHERE ca.affiliate_id = $1
)
  AND NOT EXISTS (
    SELECT 1
    FROM custom_commission_accounts ca
    WHERE ca.user_id = $2
)
ON CONFLICT DO NOTHING`, affiliate.ID, affiliate.UserID); err != nil {
			return err
		}

		accountRows, err := exec.QueryContext(txCtx, `
SELECT pending_amount::double precision,
       available_amount::double precision,
       frozen_amount::double precision,
       debt_amount::double precision
FROM custom_commission_accounts
WHERE affiliate_id = $1
FOR UPDATE`, affiliate.ID)
		if err != nil {
			return err
		}
		pendingAmount := 0.0
		availableAmount := 0.0
		frozenAmount := 0.0
		debtAmount := 0.0
		if accountRows.Next() {
			if err := accountRows.Scan(&pendingAmount, &availableAmount, &frozenAmount, &debtAmount); err != nil {
				_ = accountRows.Close()
				return err
			}
		}
		if err := accountRows.Close(); err != nil {
			return err
		}

		deltaDec := input.DeltaDecimal
		if deltaDec.IsZero() {
			deltaDec = moneyx.Commission(input.Delta)
		}
		deltaDec = deltaDec.Round(moneyx.ScaleCommission)
		var availableDeltaDec decimal.Decimal
		var debtDeltaDec decimal.Decimal
		if deltaDec.GreaterThan(moneyx.Commission(0)) {
			debtAmountDec := moneyx.NonNegative(moneyx.Commission(debtAmount))
			debtRepaidDec := moneyx.Min(deltaDec, debtAmountDec)
			availableDeltaDec = deltaDec.Sub(debtRepaidDec).Round(moneyx.ScaleCommission)
			debtDeltaDec = debtRepaidDec.Neg().Round(moneyx.ScaleCommission)
		} else {
			requestedDec := deltaDec.Neg().Round(moneyx.ScaleCommission)
			availableAmountDec := moneyx.NonNegative(moneyx.Commission(availableAmount))
			availableDecreaseDec := moneyx.Min(requestedDec, availableAmountDec)
			availableDeltaDec = availableDecreaseDec.Neg().Round(moneyx.ScaleCommission)
			debtDeltaDec = requestedDec.Sub(availableDecreaseDec).Round(moneyx.ScaleCommission)
		}

		res, err := exec.ExecContext(txCtx, `
UPDATE custom_commission_accounts
SET available_amount = available_amount + $2,
    debt_amount = debt_amount + $3,
    updated_at = NOW()
WHERE affiliate_id = $1
  AND available_amount + $2 >= -0.00000001
  AND debt_amount + $3 >= -0.00000001`, affiliate.ID, availableDeltaDec.InexactFloat64(), debtDeltaDec.InexactFloat64())
		if err != nil {
			return err
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			return service.ErrCustomReferralAdjustInsufficient
		}

		ledgerType := "commission_adjust_increase"
		if deltaDec.IsNegative() {
			ledgerType = "commission_adjust_decrease"
		}
		if _, err := exec.ExecContext(txCtx, `
INSERT INTO custom_commission_ledger (
    affiliate_id, type, ref_type, ref_id, external_ref_id,
    delta_available, delta_debt, remark, operator, created_at
) VALUES ($1, $2, 'affiliate', $3, $4, $5, $6, $7, $8, NOW())`,
			affiliate.ID,
			ledgerType,
			fmt.Sprintf("%d", affiliate.ID),
			externalRefID,
			availableDeltaDec.InexactFloat64(),
			debtDeltaDec.InexactFloat64(),
			strings.TrimSpace(input.Remark),
			fmt.Sprintf("admin:%d", input.AdminUserID),
		); err != nil {
			if isPQUniqueViolation(err) {
				out, err = r.getAffiliateByUserIDWithExecutor(txCtx, exec, input.UserID)
				return err
			}
			return err
		}

		newAccountRows, err := exec.QueryContext(txCtx, `
SELECT pending_amount::double precision,
       available_amount::double precision,
       frozen_amount::double precision,
       debt_amount::double precision
FROM custom_commission_accounts
WHERE affiliate_id = $1`, affiliate.ID)
		if err != nil {
			return err
		}
		newPendingAmount := pendingAmount
		newAvailableAmount := availableAmount
		newFrozenAmount := frozenAmount
		newDebtAmount := debtAmount
		if newAccountRows.Next() {
			if err := newAccountRows.Scan(&newPendingAmount, &newAvailableAmount, &newFrozenAmount, &newDebtAmount); err != nil {
				_ = newAccountRows.Close()
				return err
			}
		}
		if err := newAccountRows.Close(); err != nil {
			return err
		}
		newValue := map[string]any{
			"pending_amount":   moneyx.Commission(newPendingAmount).InexactFloat64(),
			"available_amount": moneyx.Commission(newAvailableAmount).InexactFloat64(),
			"frozen_amount":    moneyx.Commission(newFrozenAmount).InexactFloat64(),
			"debt_amount":      moneyx.Commission(newDebtAmount).InexactFloat64(),
		}
		newValue["delta"] = deltaDec.InexactFloat64()
		newValue["available_delta"] = availableDeltaDec.InexactFloat64()
		newValue["debt_delta"] = debtDeltaDec.InexactFloat64()
		newValue["idempotency_key"] = strings.TrimSpace(input.IdempotencyKey)
		audit := input.Audit
		audit.Action = strings.TrimSpace(audit.Action)
		if audit.Action == "" {
			audit.Action = "affiliate_commission_adjust"
		}
		if audit.AdminUserID <= 0 {
			audit.AdminUserID = input.AdminUserID
		}
		if audit.Reason == "" {
			audit.Reason = strings.TrimSpace(input.Remark)
		}
		audit.OldValue = map[string]any{
			"pending_amount":   moneyx.Commission(pendingAmount).InexactFloat64(),
			"available_amount": moneyx.Commission(availableAmount).InexactFloat64(),
			"frozen_amount":    moneyx.Commission(frozenAmount).InexactFloat64(),
			"debt_amount":      moneyx.Commission(debtAmount).InexactFloat64(),
		}
		audit.NewValue = newValue
		if err := r.recordAdminAuditWithExecutor(txCtx, exec, input.UserID, affiliate.ID, audit); err != nil {
			return err
		}
		out, err = r.getAffiliateByUserIDWithExecutor(txCtx, exec, input.UserID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *customReferralRepository) GetAffiliateByUserID(ctx context.Context, userID int64) (*service.CustomAffiliate, error) {
	exec := txAwareSQLExecutor(ctx, r.sql, r.client)
	if exec == nil {
		return nil, fmt.Errorf("custom referral sql executor is not configured")
	}
	item, err := r.getAffiliateByUserIDWithExecutor(ctx, exec, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrCustomReferralAffiliateNotFound
	}
	return item, err
}

func (r *customReferralRepository) getAffiliateByUserIDWithExecutor(ctx context.Context, exec sqlQueryExecutor, userID int64) (*service.CustomAffiliate, error) {
	rows, err := exec.QueryContext(ctx, `
SELECT a.id,
       a.user_id,
       COALESCE(u.email, ''),
       COALESCE(u.username, ''),
       a.invite_code,
       a.status,
       COALESCE(a.source_type, 'admin_created'),
       a.rate_override::double precision,
       a.acquisition_enabled,
       a.settlement_enabled,
       a.withdrawal_enabled,
       COALESCE(a.risk_reason, ''),
       COALESCE(a.risk_note, ''),
       a.approved_at,
       a.disabled_at
FROM custom_affiliates a
LEFT JOIN users u ON u.id = a.user_id
WHERE a.user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, sql.ErrNoRows
	}
	return scanCustomAffiliate(rows)
}

func (r *customReferralRepository) GetApprovedAffiliateByCode(ctx context.Context, code string) (*service.CustomAffiliate, error) {
	exec := txAwareSQLExecutor(ctx, r.sql, r.client)
	if exec == nil {
		return nil, fmt.Errorf("custom referral sql executor is not configured")
	}
	rows, err := exec.QueryContext(ctx, `
SELECT a.id,
       a.user_id,
       COALESCE(u.email, ''),
       COALESCE(u.username, ''),
       a.invite_code,
       a.status,
       COALESCE(a.source_type, 'admin_created'),
       a.rate_override::double precision,
       a.acquisition_enabled,
       a.settlement_enabled,
       a.withdrawal_enabled,
       COALESCE(a.risk_reason, ''),
       COALESCE(a.risk_note, ''),
       a.approved_at,
       a.disabled_at
FROM custom_affiliates a
LEFT JOIN users u ON u.id = a.user_id
WHERE a.invite_code = $1
  AND a.status = $2
  AND u.status = $3`, strings.ToUpper(strings.TrimSpace(code)), service.CustomAffiliateStatusApproved, service.StatusActive)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrCustomReferralAffiliateNotFound
	}
	return scanCustomAffiliate(rows)
}

func ensureAffiliateUserActive(ctx context.Context, exec sqlQueryExecutor, affiliateID int64) error {
	rows, err := exec.QueryContext(ctx, `
SELECT u.status
FROM custom_affiliates a
JOIN users u ON u.id = a.user_id
WHERE a.id = $1
FOR UPDATE`, affiliateID)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return err
		}
		return service.ErrCustomReferralAffiliateNotFound
	}
	var status string
	if err := rows.Scan(&status); err != nil {
		return err
	}
	if status != service.StatusActive {
		return service.ErrCustomReferralWithdrawDisabled
	}
	return nil
}

func ensureWithdrawalAffiliateUserActive(ctx context.Context, exec sqlQueryExecutor, withdrawalID int64) error {
	rows, err := exec.QueryContext(ctx, `
SELECT u.status
FROM custom_commission_withdrawals w
JOIN custom_affiliates a ON a.id = w.affiliate_id
JOIN users u ON u.id = a.user_id
WHERE w.id = $1
FOR UPDATE`, withdrawalID)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return err
		}
		return service.ErrCustomReferralWithdrawalNotFound
	}
	var status string
	if err := rows.Scan(&status); err != nil {
		return err
	}
	if status != service.StatusActive {
		return service.ErrCustomReferralWithdrawDisabled
	}
	return nil
}

func (r *customReferralRepository) RecordReferralClick(ctx context.Context, affiliateID int64, inviteCode string, click service.CustomReferralClickInput) error {
	exec := txAwareSQLExecutor(ctx, r.sql, r.client)
	if exec == nil {
		return fmt.Errorf("custom referral sql executor is not configured")
	}
	if click.ClickedAt.IsZero() {
		click.ClickedAt = time.Now()
	}
	_, err := exec.ExecContext(ctx, `
INSERT INTO custom_referral_clicks (affiliate_id, invite_code, referer, landing_path, ip_hash, ua_hash, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		affiliateID,
		strings.ToUpper(strings.TrimSpace(inviteCode)),
		strings.TrimSpace(click.Referer),
		strings.TrimSpace(click.LandingPath),
		strings.TrimSpace(click.IPHash),
		strings.TrimSpace(click.UserAgentHash),
		click.ClickedAt,
	)
	return err
}

func (r *customReferralRepository) InviteeInInviterAncestorChain(ctx context.Context, inviteeUserID, inviterUserID int64, maxDepth int) (bool, error) {
	if inviteeUserID <= 0 || inviterUserID <= 0 {
		return false, nil
	}
	if inviteeUserID == inviterUserID {
		return true, nil
	}
	if maxDepth <= 0 {
		maxDepth = 64
	}
	exec := txAwareSQLExecutor(ctx, r.sql, r.client)
	if exec == nil {
		return false, fmt.Errorf("custom referral sql executor is not configured")
	}
	rows, err := exec.QueryContext(ctx, `
WITH RECURSIVE ancestors(user_id, depth, path) AS (
    SELECT b.inviter_user_id, 1, ARRAY[b.invitee_user_id, b.inviter_user_id]
    FROM custom_referral_bindings b
    WHERE b.invitee_user_id = $1
    UNION ALL
    SELECT b.inviter_user_id, ancestors.depth + 1, ancestors.path || b.inviter_user_id
    FROM custom_referral_bindings b
    JOIN ancestors ON b.invitee_user_id = ancestors.user_id
    WHERE ancestors.depth < $3
      AND NOT b.inviter_user_id = ANY(ancestors.path)
)
SELECT
    EXISTS(SELECT 1 FROM ancestors WHERE user_id = $2),
    COALESCE(MAX(depth), 0)
FROM ancestors`, inviterUserID, inviteeUserID, maxDepth)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return false, err
		}
		return false, nil
	}
	var found bool
	var depth int
	if err := rows.Scan(&found, &depth); err != nil {
		return false, err
	}
	if found {
		return true, nil
	}
	if depth >= maxDepth {
		return false, service.ErrCustomReferralChainTooDeep
	}
	return false, nil
}

func (r *customReferralRepository) BindInvitee(ctx context.Context, inviteeUserID, affiliateID, inviterUserID int64, bindSource, bindCode string, boundAt time.Time) (bool, error) {
	var bound bool
	err := r.withTx(ctx, func(txCtx context.Context, exec sqlQueryExecutor) error {
		cyclic, err := r.InviteeInInviterAncestorChain(txCtx, inviteeUserID, inviterUserID, 64)
		if err != nil {
			return err
		}
		if cyclic {
			return service.ErrCustomReferralCycleInvite
		}
		rows, err := exec.QueryContext(txCtx, `SELECT invitee_user_id FROM custom_referral_bindings WHERE invitee_user_id = $1`, inviteeUserID)
		if err != nil {
			return err
		}
		if rows.Next() {
			_ = rows.Close()
			bound = false
			return nil
		}
		if err := rows.Close(); err != nil {
			return err
		}

		_, err = exec.ExecContext(txCtx, `
INSERT INTO custom_referral_bindings (
    invitee_user_id, inviter_user_id, affiliate_id, bind_source, bind_code, bound_at, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())`,
			inviteeUserID,
			inviterUserID,
			affiliateID,
			strings.TrimSpace(bindSource),
			strings.ToUpper(strings.TrimSpace(bindCode)),
			boundAt,
		)
		if err != nil {
			if isPQUniqueViolation(err) {
				bound = false
				return nil
			}
			return err
		}
		bound = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return bound, nil
}

func (r *customReferralRepository) SnapshotOrderAffiliate(ctx context.Context, userID int64, defaultRate float64) (*service.CustomReferralOrderSnapshot, error) {
	if userID <= 0 || defaultRate <= 0 {
		return nil, nil
	}
	exec := txAwareSQLExecutor(ctx, r.sql, r.client)
	if exec == nil {
		return nil, fmt.Errorf("custom referral tx executor is not configured")
	}
	rows, err := exec.QueryContext(ctx, `
SELECT b.affiliate_id,
       COALESCE(a.rate_override, $2)::double precision
FROM custom_referral_bindings b
JOIN custom_affiliates a ON a.id = b.affiliate_id
JOIN users inviter ON inviter.id = a.user_id
WHERE b.invitee_user_id = $1
  AND a.status = $3
  AND a.acquisition_enabled = TRUE
  AND a.settlement_enabled = TRUE
  AND inviter.status = $4
LIMIT 1`, userID, defaultRate, service.CustomAffiliateStatusApproved, service.StatusActive)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, nil
	}
	var snapshot service.CustomReferralOrderSnapshot
	if err := rows.Scan(&snapshot.AffiliateID, &snapshot.Rate); err != nil {
		return nil, err
	}
	if snapshot.AffiliateID <= 0 || snapshot.Rate <= 0 {
		return nil, nil
	}
	snapshot.RateDecimal = moneyx.Rate(snapshot.Rate)
	snapshot.Rate = snapshot.RateDecimal.InexactFloat64()
	return &snapshot, nil
}

func (r *customReferralRepository) CreatePendingCommissionForOrder(ctx context.Context, order service.CustomReferralOrderInput, freezeDays int) (float64, error) {
	var applied float64
	err := r.withTx(ctx, func(txCtx context.Context, exec sqlQueryExecutor) error {
		rows, err := exec.QueryContext(txCtx, `
SELECT a.user_id
FROM custom_affiliates a
JOIN users inviter ON inviter.id = a.user_id
WHERE a.id = $1
  AND a.status = $2
  AND inviter.status = $3
FOR UPDATE`, order.AffiliateID, service.CustomAffiliateStatusApproved, service.StatusActive)
		if err != nil {
			return err
		}
		if !rows.Next() {
			_ = rows.Close()
			return nil
		}
		var inviterUserID int64
		if err := rows.Scan(&inviterUserID); err != nil {
			_ = rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}

		affiliateID := order.AffiliateID
		rateDec := order.RateDecimal
		if rateDec.IsZero() {
			rateDec = moneyx.Rate(order.Rate)
		}
		rateDec = rateDec.Round(moneyx.ScaleRate)
		baseAmountDec := order.BaseAmountDecimal
		if baseAmountDec.IsZero() {
			baseAmountDec = moneyx.Commission(order.BaseAmount)
		}
		baseAmountDec = baseAmountDec.Round(moneyx.ScaleCommission)
		if !rateDec.GreaterThan(decimal.Zero) || !baseAmountDec.GreaterThan(decimal.Zero) {
			return nil
		}
		amountDec := baseAmountDec.Mul(rateDec).Div(decimal.NewFromInt(100)).Round(moneyx.ScaleCommission)
		if !amountDec.GreaterThan(moneyx.Commission(0)) {
			return nil
		}
		amount := amountDec.InexactFloat64()

		settleAt := order.PaidAt
		if settleAt.IsZero() {
			settleAt = time.Now()
		}
		settleAt = settleAt.Add(time.Duration(freezeDays) * 24 * time.Hour)

		commissionColumns, err := loadTableColumns(txCtx, exec, "custom_referral_commissions")
		if err != nil {
			return err
		}
		insertColumns := make([]string, 0, 11)
		insertValues := make([]string, 0, 11)
		insertArgs := make([]any, 0, 11)
		appendInsert := func(column string, value any) {
			insertColumns = append(insertColumns, column)
			insertArgs = append(insertArgs, value)
			insertValues = append(insertValues, fmt.Sprintf("$%d", len(insertArgs)))
		}
		appendInsert("affiliate_id", affiliateID)
		if hasTableColumn(commissionColumns, "inviter_user_id") {
			appendInsert("inviter_user_id", inviterUserID)
		}
		appendInsert("invitee_user_id", order.UserID)
		if hasTableColumn(commissionColumns, "source_order_id") {
			appendInsert("source_order_id", order.OrderID)
		}
		if hasTableColumn(commissionColumns, "order_id") {
			appendInsert("order_id", order.OrderID)
		}
		appendInsert("order_type", strings.TrimSpace(order.OrderType))
		appendInsert("base_amount", baseAmountDec.StringFixed(moneyx.ScaleCommission))
		appendInsert("rate", rateDec.StringFixed(moneyx.ScaleRate))
		appendInsert("commission_amount", amountDec.StringFixed(moneyx.ScaleCommission))
		appendInsert("refunded_amount", 0)
		appendInsert("status", service.CustomReferralCommissionStatusPending)
		appendInsert("settle_at", settleAt)

		insertRows, err := exec.QueryContext(txCtx, fmt.Sprintf(`
INSERT INTO custom_referral_commissions (
    %s, created_at, updated_at
)
VALUES (%s, NOW(), NOW())
ON CONFLICT DO NOTHING
RETURNING id`, strings.Join(insertColumns, ", "), strings.Join(insertValues, ", ")), insertArgs...)
		if err != nil {
			return err
		}
		if !insertRows.Next() {
			_ = insertRows.Close()
			existingRows, err := exec.QueryContext(txCtx, fmt.Sprintf(`
SELECT commission_amount::double precision
FROM custom_referral_commissions
WHERE %s`, customReferralCommissionOrderFilter(commissionColumns, "", "$1")), order.OrderID)
			if err != nil {
				return err
			}
			if existingRows.Next() {
				if err := existingRows.Scan(&applied); err != nil {
					_ = existingRows.Close()
					return err
				}
			}
			if err := existingRows.Close(); err != nil {
				return err
			}
			return nil
		}
		var commissionID int64
		if err := insertRows.Scan(&commissionID); err != nil {
			_ = insertRows.Close()
			return err
		}
		if err := insertRows.Close(); err != nil {
			return err
		}

		if _, err := exec.ExecContext(txCtx, `
INSERT INTO custom_commission_accounts (affiliate_id, user_id, created_at, updated_at)
SELECT a.id, a.user_id, NOW(), NOW()
FROM custom_affiliates a
WHERE a.id = $1
  AND NOT EXISTS (
    SELECT 1
    FROM custom_commission_accounts ca
    WHERE ca.affiliate_id = a.id
  )
  AND NOT EXISTS (
    SELECT 1
    FROM custom_commission_accounts ca
    WHERE ca.user_id = a.user_id
  )
ON CONFLICT DO NOTHING`, affiliateID); err != nil {
			return err
		}
		if _, err := exec.ExecContext(txCtx, `
UPDATE custom_commission_accounts
SET pending_amount = pending_amount + $2,
    updated_at = NOW()
WHERE affiliate_id = $1`, affiliateID, amountDec.StringFixed(moneyx.ScaleCommission)); err != nil {
			return err
		}
		if err := r.insertCommissionLedger(txCtx, exec, customCommissionLedgerInsert{
			UserID:        inviterUserID,
			AffiliateID:   affiliateID,
			CommissionID:  commissionID,
			Type:          "commission_accrue",
			RefType:       "order",
			RefID:         fmt.Sprintf("%d", order.OrderID),
			ExternalRefID: fmt.Sprintf("order:%d", order.OrderID),
			DeltaPending:  amountDec.InexactFloat64(),
			Remark:        fmt.Sprintf("order_type=%s inviter_user_id=%d", strings.TrimSpace(order.OrderType), inviterUserID),
			Operator:      "system",
		}); err != nil {
			return err
		}

		applied = amount
		return nil
	})
	if err != nil {
		return 0, err
	}
	return applied, nil
}

func (r *customReferralRepository) ReverseCommissionForRefund(ctx context.Context, refund service.CustomReferralRefundInput) (float64, error) {
	var reversed float64
	err := r.withTx(ctx, func(txCtx context.Context, exec sqlQueryExecutor) error {
		commissionColumns, err := loadTableColumns(txCtx, exec, "custom_referral_commissions")
		if err != nil {
			return err
		}
		rows, err := exec.QueryContext(txCtx, fmt.Sprintf(`
SELECT c.id,
       c.affiliate_id,
       a.user_id,
       c.base_amount::double precision,
       c.commission_amount::double precision,
       c.refunded_amount::double precision,
       c.status
FROM custom_referral_commissions c
JOIN custom_affiliates a ON a.id = c.affiliate_id
WHERE %s
FOR UPDATE OF c`, customReferralCommissionOrderFilter(commissionColumns, "c", "$1")), refund.OrderID)
		if err != nil {
			return err
		}
		if !rows.Next() {
			_ = rows.Close()
			return nil
		}
		var commissionID int64
		var affiliateID int64
		var affiliateUserID int64
		var baseAmount float64
		var commissionAmount float64
		var refundedAmount float64
		var status string
		if err := rows.Scan(&commissionID, &affiliateID, &affiliateUserID, &baseAmount, &commissionAmount, &refundedAmount, &status); err != nil {
			_ = rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if baseAmount <= 0 || commissionAmount <= 0 || status == service.CustomReferralCommissionStatusReversed {
			return nil
		}

		baseAmountDec := moneyx.Commission(baseAmount)
		commissionAmountDec := moneyx.Commission(commissionAmount)
		refundedAmountDec := moneyx.Commission(refundedAmount)
		refundAmountDec := moneyx.Commission(refund.RefundAmount)

		targetDec := commissionAmountDec
		if refundAmountDec.LessThan(baseAmountDec) {
			targetDec = moneyx.Proportion(commissionAmount, refund.RefundAmount, baseAmount, moneyx.ScaleCommission)
		}
		reverseAmountDec := targetDec.Sub(refundedAmountDec).Round(moneyx.ScaleCommission)
		if !reverseAmountDec.GreaterThan(moneyx.Commission(0)) {
			return nil
		}
		remainingDec := commissionAmountDec.Sub(refundedAmountDec).Round(moneyx.ScaleCommission)
		if reverseAmountDec.GreaterThan(remainingDec) {
			reverseAmountDec = remainingDec
		}
		if !reverseAmountDec.GreaterThan(moneyx.Commission(0)) {
			return nil
		}
		reverseAmount := reverseAmountDec.InexactFloat64()

		nextRefundedDec := refundedAmountDec.Add(reverseAmountDec).Round(moneyx.ScaleCommission)
		nextStatus := status
		reversedAt := any(nil)
		reversedReason := ""
		if nextRefundedDec.Equal(commissionAmountDec) || nextRefundedDec.GreaterThan(commissionAmountDec) {
			nextStatus = service.CustomReferralCommissionStatusReversed
			reversedAt = refund.RefundedAt
			if refund.RefundedAt.IsZero() {
				reversedAt = time.Now()
			}
			reversedReason = strings.TrimSpace(refund.Reason)
		}

		if _, err := exec.ExecContext(txCtx, `
UPDATE custom_referral_commissions
SET refunded_amount = $2,
    status = $3,
    reversed_at = CASE WHEN $4::timestamptz IS NULL THEN reversed_at ELSE $4::timestamptz END,
    reversed_reason = CASE WHEN $5 = '' THEN reversed_reason ELSE $5 END,
    updated_at = NOW()
WHERE id = $1`,
			commissionID,
			nextRefundedDec.InexactFloat64(),
			nextStatus,
			reversedAt,
			reversedReason,
		); err != nil {
			return err
		}

		if _, err := exec.ExecContext(txCtx, `
INSERT INTO custom_commission_accounts (affiliate_id, user_id, created_at, updated_at)
SELECT a.id, a.user_id, NOW(), NOW()
FROM custom_affiliates a
WHERE a.id = $1
  AND NOT EXISTS (
    SELECT 1
    FROM custom_commission_accounts ca
    WHERE ca.affiliate_id = a.id
  )
  AND NOT EXISTS (
    SELECT 1
    FROM custom_commission_accounts ca
    WHERE ca.user_id = a.user_id
  )
ON CONFLICT DO NOTHING`, affiliateID); err != nil {
			return err
		}

		accountRows, err := exec.QueryContext(txCtx, `
SELECT pending_amount::double precision,
       available_amount::double precision
FROM custom_commission_accounts
WHERE affiliate_id = $1
FOR UPDATE`, affiliateID)
		if err != nil {
			return err
		}
		pendingBalance := 0.0
		availableBalance := 0.0
		if accountRows.Next() {
			if err := accountRows.Scan(&pendingBalance, &availableBalance); err != nil {
				_ = accountRows.Close()
				return err
			}
		}
		if err := accountRows.Close(); err != nil {
			return err
		}
		pendingBalanceDec := moneyx.NonNegative(moneyx.Commission(pendingBalance))
		availableBalanceDec := moneyx.NonNegative(moneyx.Commission(availableBalance))

		var pendingDecreaseDec decimal.Decimal
		var availableDecreaseDec decimal.Decimal
		var debtIncreaseDec decimal.Decimal
		switch status {
		case service.CustomReferralCommissionStatusPending:
			pendingDecreaseDec = moneyx.Min(reverseAmountDec, pendingBalanceDec)
			debtIncreaseDec = reverseAmountDec.Sub(pendingDecreaseDec).Round(moneyx.ScaleCommission)
		case service.CustomReferralCommissionStatusAvailable:
			allocatedRows, err := exec.QueryContext(txCtx, `
SELECT allocated_amount::double precision,
       status
FROM custom_commission_withdrawal_items
WHERE commission_id = $1
  AND status IN ($2, $3)
FOR UPDATE`,
				commissionID,
				service.CustomReferralWithdrawalItemStatusFrozen,
				service.CustomReferralWithdrawalItemStatusWithdrawn,
			)
			if err != nil {
				return err
			}
			protectedAmount := 0.0
			for allocatedRows.Next() {
				var allocated float64
				var itemStatus string
				if err := allocatedRows.Scan(&allocated, &itemStatus); err != nil {
					_ = allocatedRows.Close()
					return err
				}
				if itemStatus == service.CustomReferralWithdrawalItemStatusFrozen || itemStatus == service.CustomReferralWithdrawalItemStatusWithdrawn {
					protectedAmount = customRoundTo(protectedAmount+allocated, 8)
				}
			}
			if err := allocatedRows.Close(); err != nil {
				return err
			}
			protectedAmountDec := moneyx.Commission(protectedAmount)
			unallocatedAmountDec := moneyx.NonNegative(commissionAmountDec.Sub(refundedAmountDec).Sub(protectedAmountDec).Round(moneyx.ScaleCommission))
			availableTargetDec := moneyx.Min(reverseAmountDec, unallocatedAmountDec)
			availableDecreaseDec = moneyx.Min(availableTargetDec, availableBalanceDec)
			debtIncreaseDec = reverseAmountDec.Sub(availableDecreaseDec).Round(moneyx.ScaleCommission)
		default:
			debtIncreaseDec = reverseAmountDec
		}

		res, err := exec.ExecContext(txCtx, `
UPDATE custom_commission_accounts
SET pending_amount = pending_amount - $2,
    available_amount = available_amount - $3,
    debt_amount = debt_amount + $4,
    reversed_amount = reversed_amount + $5,
    updated_at = NOW()
WHERE affiliate_id = $1
  AND pending_amount + 0.00000001 >= $2
  AND available_amount + 0.00000001 >= $3
  AND debt_amount + $4 >= -0.00000001`,
			affiliateID,
			pendingDecreaseDec.InexactFloat64(),
			availableDecreaseDec.InexactFloat64(),
			debtIncreaseDec.InexactFloat64(),
			reverseAmount,
		)
		if err != nil {
			return err
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			return service.ErrCustomReferralWithdrawInsufficient
		}

		if err := r.insertCommissionLedger(txCtx, exec, customCommissionLedgerInsert{
			UserID:         affiliateUserID,
			AffiliateID:    affiliateID,
			CommissionID:   commissionID,
			Type:           "commission_reverse",
			RefType:        "refund",
			RefID:          fmt.Sprintf("%d", refund.OrderID),
			ExternalRefID:  fmt.Sprintf("refund:%d:%s", refund.OrderID, nextRefundedDec.StringFixed(moneyx.ScaleCommission)),
			DeltaPending:   pendingDecreaseDec.Neg().InexactFloat64(),
			DeltaAvailable: availableDecreaseDec.Neg().InexactFloat64(),
			DeltaReversed:  reverseAmount,
			DeltaDebt:      debtIncreaseDec.InexactFloat64(),
			Remark:         strings.TrimSpace(refund.Reason),
			Operator:       "system",
		}); err != nil {
			return err
		}

		reversed = reverseAmount
		return nil
	})
	if err != nil {
		return 0, err
	}
	return reversed, nil
}

func (r *customReferralRepository) ReverseCommissionManually(ctx context.Context, input service.CustomReferralManualReverseInput) (*service.CustomReferralCommissionReversal, error) {
	var out *service.CustomReferralCommissionReversal
	err := r.withTx(ctx, func(txCtx context.Context, exec sqlQueryExecutor) error {
		existing, err := r.getCommissionReversalByExternalRefID(txCtx, exec, input.IdempotencyKey)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if existing != nil {
			existing.AlreadyProcessed = true
			out = existing
			return nil
		}
		commissionColumns, err := loadTableColumns(txCtx, exec, "custom_referral_commissions")
		if err != nil {
			return err
		}

		type lockedCommission struct {
			id               int64
			affiliateID      int64
			affiliateUserID  int64
			orderID          int64
			baseAmount       float64
			commissionAmount float64
			refundedAmount   float64
			status           string
		}
		rows, err := exec.QueryContext(txCtx, fmt.Sprintf(`
SELECT c.id,
       c.affiliate_id,
       a.user_id,
       %s,
       c.base_amount::double precision,
       c.commission_amount::double precision,
       c.refunded_amount::double precision,
       c.status
FROM custom_referral_commissions c
JOIN custom_affiliates a ON a.id = c.affiliate_id
WHERE (($1::bigint > 0 AND c.id = $1)
   OR ($1::bigint <= 0 AND %s))
FOR UPDATE OF c`, customReferralCommissionOrderSelectExpr(commissionColumns, "c"), customReferralCommissionOrderFilter(commissionColumns, "c", "$2")), input.CommissionID, input.OrderID)
		if err != nil {
			return err
		}
		if !rows.Next() {
			_ = rows.Close()
			return service.ErrCustomReferralCommissionNotFound
		}
		var commission lockedCommission
		if err := rows.Scan(
			&commission.id,
			&commission.affiliateID,
			&commission.affiliateUserID,
			&commission.orderID,
			&commission.baseAmount,
			&commission.commissionAmount,
			&commission.refundedAmount,
			&commission.status,
		); err != nil {
			_ = rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}

		if _, err := exec.ExecContext(txCtx, `
INSERT INTO custom_commission_accounts (affiliate_id, user_id, created_at, updated_at)
SELECT a.id, a.user_id, NOW(), NOW()
FROM custom_affiliates a
WHERE a.id = $1
  AND NOT EXISTS (
    SELECT 1
    FROM custom_commission_accounts ca
    WHERE ca.affiliate_id = a.id
  )
  AND NOT EXISTS (
    SELECT 1
    FROM custom_commission_accounts ca
    WHERE ca.user_id = a.user_id
  )
ON CONFLICT DO NOTHING`, commission.affiliateID); err != nil {
			return err
		}

		accountRows, err := exec.QueryContext(txCtx, `
SELECT pending_amount::double precision,
       available_amount::double precision,
       frozen_amount::double precision,
       debt_amount::double precision
FROM custom_commission_accounts
WHERE affiliate_id = $1
FOR UPDATE`, commission.affiliateID)
		if err != nil {
			return err
		}
		pendingBalance := 0.0
		availableBalance := 0.0
		frozenBalance := 0.0
		debtBalance := 0.0
		if accountRows.Next() {
			if err := accountRows.Scan(&pendingBalance, &availableBalance, &frozenBalance, &debtBalance); err != nil {
				_ = accountRows.Close()
				return err
			}
		}
		if err := accountRows.Close(); err != nil {
			return err
		}

		baseAmountDec := moneyx.Commission(commission.baseAmount)
		commissionAmountDec := moneyx.Commission(commission.commissionAmount)
		refundedAmountDec := moneyx.Commission(commission.refundedAmount)
		refundAmountDec := moneyx.Commission(input.RefundAmount)
		targetDec := commissionAmountDec
		if baseAmountDec.GreaterThan(decimal.Zero) && refundAmountDec.LessThan(baseAmountDec) {
			targetDec = moneyx.Proportion(commission.commissionAmount, input.RefundAmount, commission.baseAmount, moneyx.ScaleCommission)
		}
		reverseAmountDec := targetDec.Sub(refundedAmountDec).Round(moneyx.ScaleCommission)
		remainingCommissionDec := commissionAmountDec.Sub(refundedAmountDec).Round(moneyx.ScaleCommission)
		if reverseAmountDec.GreaterThan(remainingCommissionDec) {
			reverseAmountDec = remainingCommissionDec
		}
		if reverseAmountDec.IsNegative() {
			reverseAmountDec = decimal.Zero
		}

		frozenAllocatedDec := decimal.Zero
		withdrawnAllocatedDec := decimal.Zero
		nextRefundedDec := refundedAmountDec
		nextStatus := commission.status
		if reverseAmountDec.GreaterThan(decimal.Zero) && commission.status == service.CustomReferralCommissionStatusAvailable {
			frozenAllocatedDec, withdrawnAllocatedDec, err = r.getCommissionAllocatedAmounts(txCtx, exec, commission.id)
			if err != nil {
				return err
			}
		}
		allocation := computeCustomCommissionManualReversalAllocation(customCommissionManualReversalAllocationInput{
			Status:             commission.status,
			ReverseAmount:      reverseAmountDec,
			CommissionAmount:   commissionAmountDec,
			RefundedAmount:     refundedAmountDec,
			PendingBalance:     moneyx.Commission(pendingBalance),
			AvailableBalance:   moneyx.Commission(availableBalance),
			FrozenBalance:      moneyx.Commission(frozenBalance),
			FrozenAllocated:    frozenAllocatedDec,
			WithdrawnAllocated: withdrawnAllocatedDec,
		})
		pendingDecreaseDec := allocation.PendingDecrease
		availableDecreaseDec := allocation.AvailableDecrease
		frozenDecreaseDec := allocation.FrozenDecrease
		debtIncreaseDec := allocation.DebtIncrease

		insertRows, err := exec.QueryContext(txCtx, `
INSERT INTO custom_commission_reversals (
    affiliate_id, commission_id, order_id, admin_user_id, external_ref_id,
    refund_amount, reverse_amount, delta_pending, delta_available,
    delta_frozen, delta_reversed, delta_debt, reason, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW())
ON CONFLICT (external_ref_id) DO NOTHING
RETURNING id`,
			commission.affiliateID,
			commission.id,
			commission.orderID,
			adminIDOrNil(input.AdminUserID),
			input.IdempotencyKey,
			refundAmountDec.InexactFloat64(),
			reverseAmountDec.InexactFloat64(),
			pendingDecreaseDec.Neg().InexactFloat64(),
			availableDecreaseDec.Neg().InexactFloat64(),
			frozenDecreaseDec.Neg().InexactFloat64(),
			reverseAmountDec.InexactFloat64(),
			debtIncreaseDec.InexactFloat64(),
			strings.TrimSpace(input.Reason),
		)
		if err != nil {
			return err
		}
		var reversalID int64
		if !insertRows.Next() {
			_ = insertRows.Close()
			existing, err := r.getCommissionReversalByExternalRefID(txCtx, exec, input.IdempotencyKey)
			if err != nil {
				return err
			}
			existing.AlreadyProcessed = true
			out = existing
			return nil
		}
		if err := insertRows.Scan(&reversalID); err != nil {
			_ = insertRows.Close()
			return err
		}
		if err := insertRows.Close(); err != nil {
			return err
		}

		if reverseAmountDec.GreaterThan(decimal.Zero) {
			if frozenDecreaseDec.GreaterThan(decimal.Zero) {
				reducedDec, err := r.reduceFrozenWithdrawalAllocations(txCtx, exec, commission.id, frozenDecreaseDec)
				if err != nil {
					return err
				}
				if !reducedDec.Equal(frozenDecreaseDec) {
					shortfallDec := frozenDecreaseDec.Sub(reducedDec).Round(moneyx.ScaleCommission)
					frozenDecreaseDec = reducedDec
					availableCapacityDec := moneyx.NonNegative(moneyx.Commission(availableBalance).Sub(availableDecreaseDec).Round(moneyx.ScaleCommission))
					extraAvailableDec := moneyx.Min(shortfallDec, availableCapacityDec)
					availableDecreaseDec = availableDecreaseDec.Add(extraAvailableDec).Round(moneyx.ScaleCommission)
					debtIncreaseDec = debtIncreaseDec.Add(shortfallDec.Sub(extraAvailableDec)).Round(moneyx.ScaleCommission)
					if _, err := exec.ExecContext(txCtx, `
UPDATE custom_commission_reversals
SET delta_available = $2,
    delta_frozen = $3,
    delta_debt = $4
WHERE id = $1`,
						reversalID,
						availableDecreaseDec.Neg().InexactFloat64(),
						frozenDecreaseDec.Neg().InexactFloat64(),
						debtIncreaseDec.InexactFloat64(),
					); err != nil {
						return err
					}
				}
			}

			nextRefundedDec = refundedAmountDec.Add(reverseAmountDec).Round(moneyx.ScaleCommission)
			reversedAt := any(nil)
			reversedReason := ""
			if nextRefundedDec.Equal(commissionAmountDec) || nextRefundedDec.GreaterThan(commissionAmountDec) {
				nextStatus = service.CustomReferralCommissionStatusReversed
				reversedAt = input.ReversedAt
				reversedReason = strings.TrimSpace(input.Reason)
			}
			if _, err := exec.ExecContext(txCtx, `
UPDATE custom_referral_commissions
SET refunded_amount = $2,
    status = $3,
    reversed_at = CASE WHEN $4::timestamptz IS NULL THEN reversed_at ELSE $4::timestamptz END,
    reversed_reason = CASE WHEN $5 = '' THEN reversed_reason ELSE $5 END,
    updated_at = NOW()
WHERE id = $1`,
				commission.id,
				nextRefundedDec.InexactFloat64(),
				nextStatus,
				reversedAt,
				reversedReason,
			); err != nil {
				return err
			}

			res, err := exec.ExecContext(txCtx, `
UPDATE custom_commission_accounts
SET pending_amount = pending_amount - $2,
    available_amount = available_amount - $3,
    frozen_amount = frozen_amount - $4,
    debt_amount = debt_amount + $5,
    reversed_amount = reversed_amount + $6,
    updated_at = NOW()
WHERE affiliate_id = $1
  AND pending_amount + 0.00000001 >= $2
  AND available_amount + 0.00000001 >= $3
  AND frozen_amount + 0.00000001 >= $4
  AND debt_amount + $5 >= -0.00000001`,
				commission.affiliateID,
				pendingDecreaseDec.InexactFloat64(),
				availableDecreaseDec.InexactFloat64(),
				frozenDecreaseDec.InexactFloat64(),
				debtIncreaseDec.InexactFloat64(),
				reverseAmountDec.InexactFloat64(),
			)
			if err != nil {
				return err
			}
			affected, _ := res.RowsAffected()
			if affected == 0 {
				return service.ErrCustomReferralWithdrawInsufficient
			}

			if err := r.insertCommissionLedger(txCtx, exec, customCommissionLedgerInsert{
				UserID:              commission.affiliateUserID,
				AffiliateID:         commission.affiliateID,
				CommissionID:        commission.id,
				RelatedCommissionID: commission.id,
				Type:                "commission_reverse",
				RefType:             "manual_refund",
				RefID:               fmt.Sprintf("%d", commission.orderID),
				ExternalRefID:       input.IdempotencyKey,
				DeltaPending:        pendingDecreaseDec.Neg().InexactFloat64(),
				DeltaAvailable:      availableDecreaseDec.Neg().InexactFloat64(),
				DeltaFrozen:         frozenDecreaseDec.Neg().InexactFloat64(),
				DeltaReversed:       reverseAmountDec.InexactFloat64(),
				DeltaDebt:           debtIncreaseDec.InexactFloat64(),
				Remark:              fmt.Sprintf("reason=%s refund_amount=%s reverse_amount=%s", strings.TrimSpace(input.Reason), refundAmountDec.StringFixed(8), reverseAmountDec.StringFixed(8)),
				Operator:            fmt.Sprintf("admin:%d", input.AdminUserID),
				OperatorType:        "admin",
				OperatorID:          input.AdminUserID,
			}); err != nil {
				return err
			}
		}

		oldValue := map[string]any{
			"affiliate_id":     commission.affiliateID,
			"commission_id":    commission.id,
			"order_id":         commission.orderID,
			"status":           commission.status,
			"refunded_amount":  refundedAmountDec.InexactFloat64(),
			"pending_amount":   moneyx.Commission(pendingBalance).InexactFloat64(),
			"available_amount": moneyx.Commission(availableBalance).InexactFloat64(),
			"frozen_amount":    moneyx.Commission(frozenBalance).InexactFloat64(),
			"debt_amount":      moneyx.Commission(debtBalance).InexactFloat64(),
		}
		newValue := map[string]any{
			"affiliate_id":     commission.affiliateID,
			"commission_id":    commission.id,
			"order_id":         commission.orderID,
			"status":           nextStatus,
			"refunded_amount":  nextRefundedDec.InexactFloat64(),
			"refund_amount":    refundAmountDec.InexactFloat64(),
			"reverse_amount":   reverseAmountDec.InexactFloat64(),
			"delta_pending":    pendingDecreaseDec.Neg().InexactFloat64(),
			"delta_available":  availableDecreaseDec.Neg().InexactFloat64(),
			"delta_frozen":     frozenDecreaseDec.Neg().InexactFloat64(),
			"delta_debt":       debtIncreaseDec.InexactFloat64(),
			"pending_amount":   moneyx.Commission(pendingBalance).Sub(pendingDecreaseDec).Round(moneyx.ScaleCommission).InexactFloat64(),
			"available_amount": moneyx.Commission(availableBalance).Sub(availableDecreaseDec).Round(moneyx.ScaleCommission).InexactFloat64(),
			"frozen_amount":    moneyx.Commission(frozenBalance).Sub(frozenDecreaseDec).Round(moneyx.ScaleCommission).InexactFloat64(),
			"debt_amount":      moneyx.Commission(debtBalance).Add(debtIncreaseDec).Round(moneyx.ScaleCommission).InexactFloat64(),
			"idempotency_key":  input.IdempotencyKey,
		}
		if err := r.recordAdminAuditWithExecutor(txCtx, exec, commission.affiliateUserID, commission.affiliateID, service.CustomReferralAdminAuditContext{
			Action:      "commission_manual_reverse",
			AdminUserID: input.AdminUserID,
			IP:          input.IP,
			UserAgent:   input.UserAgent,
			Reason:      input.Reason,
			OldValue:    oldValue,
			NewValue:    newValue,
		}); err != nil {
			return err
		}

		out, err = r.getCommissionReversalByID(txCtx, exec, reversalID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

type customCommissionManualReversalAllocationInput struct {
	Status             string
	ReverseAmount      decimal.Decimal
	CommissionAmount   decimal.Decimal
	RefundedAmount     decimal.Decimal
	PendingBalance     decimal.Decimal
	AvailableBalance   decimal.Decimal
	FrozenBalance      decimal.Decimal
	FrozenAllocated    decimal.Decimal
	WithdrawnAllocated decimal.Decimal
}

type customCommissionManualReversalAllocation struct {
	PendingDecrease   decimal.Decimal
	AvailableDecrease decimal.Decimal
	FrozenDecrease    decimal.Decimal
	DebtIncrease      decimal.Decimal
}

func computeCustomCommissionManualReversalAllocation(input customCommissionManualReversalAllocationInput) customCommissionManualReversalAllocation {
	out := customCommissionManualReversalAllocation{
		PendingDecrease:   decimal.Zero,
		AvailableDecrease: decimal.Zero,
		FrozenDecrease:    decimal.Zero,
		DebtIncrease:      decimal.Zero,
	}
	reverseAmountDec := input.ReverseAmount.Round(moneyx.ScaleCommission)
	if !reverseAmountDec.GreaterThan(decimal.Zero) {
		return out
	}

	switch input.Status {
	case service.CustomReferralCommissionStatusPending:
		out.PendingDecrease = moneyx.Min(reverseAmountDec, moneyx.NonNegative(input.PendingBalance)).Round(moneyx.ScaleCommission)
		out.DebtIncrease = moneyx.NonNegative(reverseAmountDec.Sub(out.PendingDecrease).Round(moneyx.ScaleCommission))
	case service.CustomReferralCommissionStatusAvailable:
		remainingDec := reverseAmountDec
		frozenTargetDec := moneyx.Min(remainingDec, moneyx.NonNegative(input.FrozenAllocated))
		out.FrozenDecrease = moneyx.Min(frozenTargetDec, moneyx.NonNegative(input.FrozenBalance)).Round(moneyx.ScaleCommission)
		remainingDec = remainingDec.Sub(out.FrozenDecrease).Round(moneyx.ScaleCommission)

		unallocatedAvailableDec := moneyx.NonNegative(input.CommissionAmount.Sub(input.RefundedAmount).Sub(input.FrozenAllocated).Sub(input.WithdrawnAllocated).Round(moneyx.ScaleCommission))
		availableTargetDec := moneyx.Min(remainingDec, unallocatedAvailableDec)
		out.AvailableDecrease = moneyx.Min(availableTargetDec, moneyx.NonNegative(input.AvailableBalance)).Round(moneyx.ScaleCommission)
		remainingDec = remainingDec.Sub(out.AvailableDecrease).Round(moneyx.ScaleCommission)
		out.DebtIncrease = moneyx.NonNegative(remainingDec)
	default:
		out.DebtIncrease = reverseAmountDec
	}
	return out
}

func (r *customReferralRepository) getCommissionAllocatedAmounts(ctx context.Context, exec sqlQueryExecutor, commissionID int64) (decimal.Decimal, decimal.Decimal, error) {
	rows, err := exec.QueryContext(ctx, `
SELECT allocated_amount::double precision,
       status
FROM custom_commission_withdrawal_items
WHERE commission_id = $1
  AND status IN ($2, $3)
FOR UPDATE`,
		commissionID,
		service.CustomReferralWithdrawalItemStatusFrozen,
		service.CustomReferralWithdrawalItemStatusWithdrawn,
	)
	if err != nil {
		return decimal.Zero, decimal.Zero, err
	}
	defer func() { _ = rows.Close() }()
	frozenDec := decimal.Zero
	withdrawnDec := decimal.Zero
	for rows.Next() {
		var amount float64
		var status string
		if err := rows.Scan(&amount, &status); err != nil {
			return decimal.Zero, decimal.Zero, err
		}
		switch status {
		case service.CustomReferralWithdrawalItemStatusFrozen:
			frozenDec = frozenDec.Add(moneyx.Commission(amount)).Round(moneyx.ScaleCommission)
		case service.CustomReferralWithdrawalItemStatusWithdrawn:
			withdrawnDec = withdrawnDec.Add(moneyx.Commission(amount)).Round(moneyx.ScaleCommission)
		}
	}
	if err := rows.Err(); err != nil {
		return decimal.Zero, decimal.Zero, err
	}
	return frozenDec, withdrawnDec, nil
}

func (r *customReferralRepository) reduceFrozenWithdrawalAllocations(ctx context.Context, exec sqlQueryExecutor, commissionID int64, amountDec decimal.Decimal) (decimal.Decimal, error) {
	if !amountDec.GreaterThan(decimal.Zero) {
		return decimal.Zero, nil
	}
	rows, err := exec.QueryContext(ctx, `
SELECT id,
       withdrawal_id,
       allocated_amount::double precision
FROM custom_commission_withdrawal_items
WHERE commission_id = $1
  AND status = $2
ORDER BY id ASC
FOR UPDATE`, commissionID, service.CustomReferralWithdrawalItemStatusFrozen)
	if err != nil {
		return decimal.Zero, err
	}
	type frozenItem struct {
		id           int64
		withdrawalID int64
		amount       float64
	}
	items := make([]frozenItem, 0)
	for rows.Next() {
		var item frozenItem
		if err := rows.Scan(&item.id, &item.withdrawalID, &item.amount); err != nil {
			_ = rows.Close()
			return decimal.Zero, err
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		return decimal.Zero, err
	}

	remainingDec := amountDec
	reducedDec := decimal.Zero
	for _, item := range items {
		if !remainingDec.GreaterThan(decimal.Zero) {
			break
		}
		itemAmountDec := moneyx.Commission(item.amount)
		useDec := moneyx.Min(itemAmountDec, remainingDec)
		nextItemAmountDec := itemAmountDec.Sub(useDec).Round(moneyx.ScaleCommission)
		if nextItemAmountDec.GreaterThan(decimal.Zero) {
			if _, err := exec.ExecContext(ctx, `
UPDATE custom_commission_withdrawal_items
SET allocated_amount = $2
WHERE id = $1`, item.id, nextItemAmountDec.InexactFloat64()); err != nil {
				return decimal.Zero, err
			}
		} else {
			if _, err := exec.ExecContext(ctx, `
UPDATE custom_commission_withdrawal_items
SET allocated_amount = 0,
    status = $2
WHERE id = $1`, item.id, service.CustomReferralWithdrawalItemStatusReleased); err != nil {
				return decimal.Zero, err
			}
		}
		res, err := exec.ExecContext(ctx, `
UPDATE custom_commission_withdrawals
SET amount = amount - $2,
    net_amount = GREATEST(net_amount - $2, 0),
    admin_note = TRIM(BOTH FROM CONCAT(COALESCE(admin_note, ''), ' manual refund reversal adjusted amount')),
    updated_at = NOW()
WHERE id = $1
  AND amount + 0.00000001 >= $2`, item.withdrawalID, useDec.InexactFloat64())
		if err != nil {
			return decimal.Zero, err
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			return decimal.Zero, service.ErrCustomReferralWithdrawInsufficient
		}
		reducedDec = reducedDec.Add(useDec).Round(moneyx.ScaleCommission)
		remainingDec = remainingDec.Sub(useDec).Round(moneyx.ScaleCommission)
	}
	return reducedDec, nil
}

func (r *customReferralRepository) getCommissionReversalByExternalRefID(ctx context.Context, exec sqlQueryExecutor, externalRefID string) (*service.CustomReferralCommissionReversal, error) {
	rows, err := exec.QueryContext(ctx, `
SELECT id,
       affiliate_id,
       commission_id,
       order_id,
       refund_amount::double precision,
       reverse_amount::double precision,
       delta_pending::double precision,
       delta_available::double precision,
       delta_frozen::double precision,
       delta_reversed::double precision,
       delta_debt::double precision,
       COALESCE(reason, ''),
       external_ref_id,
       COALESCE(admin_user_id, 0),
       created_at
FROM custom_commission_reversals
WHERE external_ref_id = $1
FOR UPDATE`, externalRefID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, sql.ErrNoRows
	}
	return scanCustomCommissionReversal(rows)
}

func (r *customReferralRepository) getCommissionReversalByID(ctx context.Context, exec sqlQueryExecutor, id int64) (*service.CustomReferralCommissionReversal, error) {
	rows, err := exec.QueryContext(ctx, `
SELECT id,
       affiliate_id,
       commission_id,
       order_id,
       refund_amount::double precision,
       reverse_amount::double precision,
       delta_pending::double precision,
       delta_available::double precision,
       delta_frozen::double precision,
       delta_reversed::double precision,
       delta_debt::double precision,
       COALESCE(reason, ''),
       external_ref_id,
       COALESCE(admin_user_id, 0),
       created_at
FROM custom_commission_reversals
WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, sql.ErrNoRows
	}
	return scanCustomCommissionReversal(rows)
}

func (r *customReferralRepository) GetDashboardByUserID(ctx context.Context, userID int64) (*service.CustomReferralDashboard, error) {
	exec := txAwareSQLExecutor(ctx, r.sql, r.client)
	if exec == nil {
		return nil, fmt.Errorf("custom referral sql executor is not configured")
	}
	rows, err := exec.QueryContext(ctx, `
SELECT a.status,
       a.invite_code,
       COALESCE(a.rate_override::double precision, NULL),
       COALESCE(a.acquisition_enabled, FALSE),
       COALESCE(a.settlement_enabled, FALSE),
       COALESCE(a.withdrawal_enabled, FALSE),
       COALESCE(clicks.cnt, 0),
       COALESCE(bindings.cnt, 0),
       COALESCE(paid.cnt, 0),
       COALESCE(acc.pending_amount::double precision, 0),
       COALESCE(acc.available_amount::double precision, 0),
       COALESCE(acc.frozen_amount::double precision, 0),
       COALESCE(acc.withdrawn_amount::double precision, 0),
       COALESCE(acc.reversed_amount::double precision, 0),
       COALESCE(acc.debt_amount::double precision, 0)
FROM custom_affiliates a
LEFT JOIN custom_commission_accounts acc ON acc.affiliate_id = a.id
LEFT JOIN (
    SELECT affiliate_id, COUNT(*) AS cnt
    FROM custom_referral_clicks
    GROUP BY affiliate_id
) clicks ON clicks.affiliate_id = a.id
LEFT JOIN (
    SELECT affiliate_id, COUNT(*) AS cnt
    FROM custom_referral_bindings
    GROUP BY affiliate_id
) bindings ON bindings.affiliate_id = a.id
LEFT JOIN (
    SELECT affiliate_id, COUNT(DISTINCT invitee_user_id) AS cnt
    FROM custom_referral_commissions
    WHERE status <> 'reversed'
      AND commission_amount > refunded_amount
    GROUP BY affiliate_id
) paid ON paid.affiliate_id = a.id
WHERE a.user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, nil
	}
	var out service.CustomReferralDashboard
	var rate sql.NullFloat64
	if err := rows.Scan(
		&out.Status,
		&out.InviteCode,
		&rate,
		&out.AcquisitionEnabled,
		&out.SettlementEnabled,
		&out.WithdrawalEnabled,
		&out.ClickCount,
		&out.BoundUserCount,
		&out.PaidUserCount,
		&out.PendingAmount,
		&out.AvailableAmount,
		&out.FrozenAmount,
		&out.WithdrawnAmount,
		&out.ReversedAmount,
		&out.DebtAmount,
	); err != nil {
		return nil, err
	}
	if rate.Valid {
		out.Rate = &rate.Float64
	}
	return &out, nil
}

func (r *customReferralRepository) ListAffiliates(ctx context.Context, params service.CustomReferralListParams) ([]service.CustomAffiliate, int64, error) {
	exec := txAwareSQLExecutor(ctx, r.sql, r.client)
	if exec == nil {
		return nil, 0, fmt.Errorf("custom referral sql executor is not configured")
	}
	offset := (params.Page - 1) * params.PageSize
	keyword := "%" + strings.TrimSpace(params.Keyword) + "%"

	countRows, err := exec.QueryContext(ctx, `
SELECT COUNT(*)
FROM custom_affiliates a
LEFT JOIN users u ON u.id = a.user_id
WHERE ($1 = '' OR a.status = $1)
  AND ($2 = '%%' OR COALESCE(u.email, '') ILIKE $2 OR COALESCE(u.username, '') ILIKE $2 OR a.invite_code ILIKE $2)`,
		strings.TrimSpace(params.Status),
		keyword,
	)
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if countRows.Next() {
		if err := countRows.Scan(&total); err != nil {
			_ = countRows.Close()
			return nil, 0, err
		}
	}
	if err := countRows.Close(); err != nil {
		return nil, 0, err
	}

	rows, err := exec.QueryContext(ctx, `
SELECT a.id,
       a.user_id,
       COALESCE(u.email, ''),
       COALESCE(u.username, ''),
       a.invite_code,
       a.status,
       COALESCE(a.source_type, 'admin_created'),
       a.rate_override::double precision,
       COALESCE(clicks.cnt, 0),
       COALESCE(bindings.cnt, 0),
       COALESCE(paid.cnt, 0),
       COALESCE(acc.pending_amount::double precision, 0),
       COALESCE(acc.available_amount::double precision, 0),
       COALESCE(acc.withdrawn_amount::double precision, 0),
       a.acquisition_enabled,
       a.settlement_enabled,
       a.withdrawal_enabled,
       COALESCE(a.risk_reason, ''),
       COALESCE(a.risk_note, ''),
       a.approved_at,
       a.disabled_at
FROM custom_affiliates a
LEFT JOIN users u ON u.id = a.user_id
LEFT JOIN custom_commission_accounts acc ON acc.affiliate_id = a.id
LEFT JOIN (
    SELECT affiliate_id, COUNT(*) AS cnt
    FROM custom_referral_clicks
    GROUP BY affiliate_id
) clicks ON clicks.affiliate_id = a.id
LEFT JOIN (
    SELECT affiliate_id, COUNT(*) AS cnt
    FROM custom_referral_bindings
    GROUP BY affiliate_id
) bindings ON bindings.affiliate_id = a.id
LEFT JOIN (
    SELECT affiliate_id, COUNT(DISTINCT invitee_user_id) AS cnt
    FROM custom_referral_commissions
    WHERE status <> 'reversed'
      AND commission_amount > refunded_amount
    GROUP BY affiliate_id
) paid ON paid.affiliate_id = a.id
WHERE ($1 = '' OR a.status = $1)
  AND ($2 = '%%' OR COALESCE(u.email, '') ILIKE $2 OR COALESCE(u.username, '') ILIKE $2 OR a.invite_code ILIKE $2)
ORDER BY a.updated_at DESC, a.id DESC
LIMIT $3 OFFSET $4`,
		strings.TrimSpace(params.Status),
		keyword,
		params.PageSize,
		offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]service.CustomAffiliate, 0)
	for rows.Next() {
		item, err := scanCustomAffiliateWithStats(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *customReferralRepository) ListAffiliateBindings(ctx context.Context, affiliateUserID int64, page, pageSize int) ([]service.CustomReferralBindingDetail, int64, error) {
	exec := txAwareSQLExecutor(ctx, r.sql, r.client)
	if exec == nil {
		return nil, 0, fmt.Errorf("custom referral sql executor is not configured")
	}
	offset := (page - 1) * pageSize

	countRows, err := exec.QueryContext(ctx, `
SELECT COUNT(*)
FROM custom_referral_bindings b
JOIN custom_affiliates a ON a.id = b.affiliate_id
WHERE a.user_id = $1`, affiliateUserID)
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if countRows.Next() {
		if err := countRows.Scan(&total); err != nil {
			_ = countRows.Close()
			return nil, 0, err
		}
	}
	if err := countRows.Close(); err != nil {
		return nil, 0, err
	}

	rows, err := exec.QueryContext(ctx, `
SELECT b.id,
       b.invitee_user_id,
       COALESCE(u.email, ''),
       COALESCE(u.username, ''),
       b.bound_at
FROM custom_referral_bindings b
JOIN custom_affiliates a ON a.id = b.affiliate_id
LEFT JOIN users u ON u.id = b.invitee_user_id
WHERE a.user_id = $1
ORDER BY b.bound_at DESC, b.id DESC
LIMIT $2 OFFSET $3`, affiliateUserID, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.CustomReferralBindingDetail, 0)
	for rows.Next() {
		var item service.CustomReferralBindingDetail
		if err := rows.Scan(
			&item.ID,
			&item.InviteeUserID,
			&item.InviteeEmail,
			&item.InviteeName,
			&item.BoundAt,
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

func (r *customReferralRepository) GetAdminOverview(ctx context.Context) (*service.CustomReferralAdminOverview, error) {
	exec := txAwareSQLExecutor(ctx, r.sql, r.client)
	if exec == nil {
		return nil, fmt.Errorf("custom referral sql executor is not configured")
	}
	rows, err := exec.QueryContext(ctx, `
SELECT
    COALESCE((SELECT COUNT(*) FROM custom_affiliates WHERE status IN ('approved', 'disabled')), 0),
    COALESCE((SELECT COUNT(*) FROM custom_affiliates WHERE status = 'approved'), 0),
    COALESCE((SELECT COUNT(*) FROM custom_affiliates WHERE status = 'disabled'), 0),
    COALESCE((SELECT SUM(pending_amount)::double precision FROM custom_commission_accounts), 0),
    COALESCE((SELECT SUM(available_amount)::double precision FROM custom_commission_accounts), 0),
    COALESCE((SELECT SUM(frozen_amount)::double precision FROM custom_commission_accounts), 0),
    COALESCE((SELECT SUM(withdrawn_amount)::double precision FROM custom_commission_accounts), 0),
    COALESCE((SELECT COUNT(*) FROM custom_referral_clicks), 0),
    COALESCE((SELECT COUNT(*) FROM custom_referral_bindings), 0),
    COALESCE((SELECT COUNT(DISTINCT invitee_user_id) FROM custom_referral_commissions WHERE status <> 'reversed' AND commission_amount > refunded_amount), 0)`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return &service.CustomReferralAdminOverview{}, nil
	}
	var out service.CustomReferralAdminOverview
	if err := rows.Scan(
		&out.TotalAffiliates,
		&out.ApprovedAffiliates,
		&out.DisabledAffiliates,
		&out.PendingAmount,
		&out.AvailableAmount,
		&out.FrozenAmount,
		&out.WithdrawnAmount,
		&out.ReferralClickCount,
		&out.BoundUserCount,
		&out.EffectivePaidUserCount,
	); err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *customReferralRepository) ListCommissionsByUserID(ctx context.Context, userID int64, params service.CustomReferralCommissionListParams) ([]service.CustomReferralUserCommission, int64, error) {
	exec := txAwareSQLExecutor(ctx, r.sql, r.client)
	if exec == nil {
		return nil, 0, fmt.Errorf("custom referral sql executor is not configured")
	}
	offset := (params.Page - 1) * params.PageSize
	status := strings.TrimSpace(params.Status)
	countRows, err := exec.QueryContext(ctx, `
SELECT COUNT(*)
FROM custom_referral_commissions c
JOIN custom_affiliates a ON a.id = c.affiliate_id
WHERE a.user_id = $1
  AND ($2 = '' OR c.status = $2)`, userID, status)
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if countRows.Next() {
		if err := countRows.Scan(&total); err != nil {
			_ = countRows.Close()
			return nil, 0, err
		}
	}
	if err := countRows.Close(); err != nil {
		return nil, 0, err
	}
	rows, err := exec.QueryContext(ctx, `
SELECT c.id,
       c.order_type,
       c.commission_amount::double precision,
       c.refunded_amount::double precision,
       c.status,
       c.settle_at,
       c.available_at,
       c.reversed_at,
       c.created_at
FROM custom_referral_commissions c
JOIN custom_affiliates a ON a.id = c.affiliate_id
WHERE a.user_id = $1
  AND ($2 = '' OR c.status = $2)
ORDER BY c.created_at DESC, c.id DESC
LIMIT $3 OFFSET $4`, userID, status, params.PageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]service.CustomReferralUserCommission, 0)
	for rows.Next() {
		var item service.CustomReferralUserCommission
		var availableAt sql.NullTime
		var reversedAt sql.NullTime
		if err := rows.Scan(
			&item.ID,
			&item.OrderType,
			&item.CommissionAmount,
			&item.RefundedAmount,
			&item.Status,
			&item.SettleAt,
			&availableAt,
			&reversedAt,
			&item.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		if availableAt.Valid {
			item.AvailableAt = &availableAt.Time
		}
		if reversedAt.Valid {
			item.ReversedAt = &reversedAt.Time
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *customReferralRepository) ListAffiliateCommissions(ctx context.Context, affiliateUserID int64, params service.CustomReferralCommissionListParams) ([]service.CustomReferralCommission, int64, error) {
	return r.listCommissions(ctx, params, "WHERE a.user_id = $1 AND ($2 = '' OR c.status = $2)", affiliateUserID)
}

func (r *customReferralRepository) ListCommissions(ctx context.Context, params service.CustomReferralCommissionListParams) ([]service.CustomReferralCommission, int64, error) {
	return r.listCommissions(ctx, params, "WHERE ($1 = '' OR c.status = $1)")
}

func (r *customReferralRepository) ListCommissionJobs(ctx context.Context, params service.CustomReferralCommissionListParams) ([]service.CustomReferralCommissionJob, int64, error) {
	exec := txAwareSQLExecutor(ctx, r.sql, r.client)
	if exec == nil {
		return nil, 0, fmt.Errorf("custom referral sql executor is not configured")
	}
	status := strings.TrimSpace(params.Status)
	offset := (params.Page - 1) * params.PageSize
	countRows, err := exec.QueryContext(ctx, `
SELECT COUNT(*)
FROM custom_referral_commission_jobs
WHERE ($1 = '' OR status = $1)`, status)
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if countRows.Next() {
		if err := countRows.Scan(&total); err != nil {
			_ = countRows.Close()
			return nil, 0, err
		}
	}
	if err := countRows.Close(); err != nil {
		return nil, 0, err
	}

	rows, err := exec.QueryContext(ctx, `
SELECT id,
       order_id,
       COALESCE(affiliate_id, 0),
       status,
       attempt_count,
       COALESCE(last_error, ''),
       locked_at,
       succeeded_at,
       failed_at,
       created_at,
       updated_at
FROM custom_referral_commission_jobs
WHERE ($1 = '' OR status = $1)
ORDER BY updated_at DESC, id DESC
LIMIT $2 OFFSET $3`, status, params.PageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]service.CustomReferralCommissionJob, 0)
	for rows.Next() {
		var item service.CustomReferralCommissionJob
		if err := rows.Scan(
			&item.ID,
			&item.OrderID,
			&item.AffiliateID,
			&item.Status,
			&item.AttemptCount,
			&item.LastError,
			&item.LockedAt,
			&item.SucceededAt,
			&item.FailedAt,
			&item.CreatedAt,
			&item.UpdatedAt,
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

func (r *customReferralRepository) listCommissions(ctx context.Context, params service.CustomReferralCommissionListParams, where string, args ...any) ([]service.CustomReferralCommission, int64, error) {
	exec := txAwareSQLExecutor(ctx, r.sql, r.client)
	if exec == nil {
		return nil, 0, fmt.Errorf("custom referral sql executor is not configured")
	}
	offset := (params.Page - 1) * params.PageSize
	status := strings.TrimSpace(params.Status)
	countSQL := `
SELECT COUNT(*)
FROM custom_referral_commissions c
JOIN custom_affiliates a ON a.id = c.affiliate_id
` + where
	listSQL := `
SELECT c.id,
       c.affiliate_id,
       a.user_id,
       COALESCE(au.email, ''),
       c.order_id,
       c.invitee_user_id,
       COALESCE(iu.email, ''),
       COALESCE(iu.username, ''),
       c.order_type,
       c.base_amount::double precision,
       c.rate::double precision,
       c.commission_amount::double precision,
       c.refunded_amount::double precision,
       c.status,
       c.settle_at,
       c.available_at,
       c.reversed_at,
       COALESCE(c.reversed_reason, ''),
       c.created_at
FROM custom_referral_commissions c
JOIN custom_affiliates a ON a.id = c.affiliate_id
LEFT JOIN users au ON au.id = a.user_id
LEFT JOIN users iu ON iu.id = c.invitee_user_id
` + where + `
ORDER BY c.created_at DESC, c.id DESC`

	countArgs := append([]any{}, args...)
	if strings.Contains(where, "$2") {
		countArgs = append(countArgs, status)
	} else {
		countArgs = []any{status}
	}
	countRows, err := exec.QueryContext(ctx, countSQL, countArgs...)
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if countRows.Next() {
		if err := countRows.Scan(&total); err != nil {
			_ = countRows.Close()
			return nil, 0, err
		}
	}
	if err := countRows.Close(); err != nil {
		return nil, 0, err
	}

	listArgs := append([]any{}, args...)
	if strings.Contains(where, "$2") {
		listSQL += `
LIMIT $3 OFFSET $4`
		listArgs = append(listArgs, status, params.PageSize, offset)
	} else {
		listSQL += `
LIMIT $2 OFFSET $3`
		listArgs = []any{status, params.PageSize, offset}
	}

	rows, err := exec.QueryContext(ctx, listSQL, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]service.CustomReferralCommission, 0)
	for rows.Next() {
		item, err := scanCustomReferralCommission(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *customReferralRepository) RunSettlementBatch(ctx context.Context, now time.Time) (*service.CustomReferralSettlementBatch, error) {
	exec := txAwareSQLExecutor(ctx, r.sql, r.client)
	if exec == nil {
		return nil, fmt.Errorf("custom referral sql executor is not configured")
	}
	batchNo, err := generateSettlementBatchNo()
	if err != nil {
		return nil, err
	}
	row, err := exec.QueryContext(ctx, `
INSERT INTO custom_commission_settlement_batches (batch_no, status, started_at, finished_at, scanned_count, settled_count, skipped_count, failed_count, error_summary)
VALUES ($1, 'running', $2, NULL, 0, 0, 0, 0, '')
RETURNING id`, batchNo, now)
	if err != nil {
		return nil, err
	}
	var batchID int64
	if !row.Next() {
		_ = row.Close()
		return nil, fmt.Errorf("failed to create settlement batch")
	}
	if err := row.Scan(&batchID); err != nil {
		_ = row.Close()
		return nil, err
	}
	if err := row.Close(); err != nil {
		return nil, err
	}

	result := &service.CustomReferralSettlementBatch{
		ID:        batchID,
		BatchNo:   batchNo,
		Status:    "running",
		StartedAt: now,
	}

	processErr := r.withTx(ctx, func(txCtx context.Context, txExec sqlQueryExecutor) error {
		return r.settleDueCommissions(txCtx, txExec, now, result)
	})
	if processErr != nil {
		result.Status = "failed"
		result.ErrorSummary = processErr.Error()
		result.FinishedAt = &now
		result.FailedCount = result.ScannedCount - result.SettledCount - result.SkippedCount
		_, _ = exec.ExecContext(ctx, `
UPDATE custom_commission_settlement_batches
SET status = $2,
    finished_at = $3,
    scanned_count = $4,
    settled_count = $5,
    skipped_count = $6,
    failed_count = $7,
    error_summary = $8
WHERE id = $1`,
			batchID,
			result.Status,
			now,
			result.ScannedCount,
			result.SettledCount,
			result.SkippedCount,
			result.FailedCount,
			result.ErrorSummary,
		)
		return result, processErr
	}

	result.Status = "completed"
	result.FinishedAt = &now
	_, err = exec.ExecContext(ctx, `
UPDATE custom_commission_settlement_batches
SET status = $2,
    finished_at = $3,
    scanned_count = $4,
    settled_count = $5,
    skipped_count = $6,
    failed_count = $7,
    error_summary = $8
WHERE id = $1`,
		batchID,
		result.Status,
		now,
		result.ScannedCount,
		result.SettledCount,
		result.SkippedCount,
		result.FailedCount,
		result.ErrorSummary,
	)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *customReferralRepository) SettleDueCommissions(ctx context.Context, now time.Time) error {
	return r.withTx(ctx, func(txCtx context.Context, exec sqlQueryExecutor) error {
		return r.settleDueCommissions(txCtx, exec, now, nil)
	})
}

func (r *customReferralRepository) settleDueCommissions(ctx context.Context, exec sqlQueryExecutor, now time.Time, result *service.CustomReferralSettlementBatch) error {
	rows, err := exec.QueryContext(ctx, `
SELECT c.id,
       c.affiliate_id,
       a.user_id,
       c.commission_amount::double precision,
       c.refunded_amount::double precision
FROM custom_referral_commissions c
JOIN custom_affiliates a ON a.id = c.affiliate_id
JOIN users u ON u.id = a.user_id
WHERE c.status = $1
  AND c.settle_at <= $2
  AND a.status = $3
  AND u.status = $4
  AND a.settlement_enabled = TRUE
ORDER BY c.settle_at ASC, c.id ASC
FOR UPDATE`, service.CustomReferralCommissionStatusPending, now, service.CustomAffiliateStatusApproved, service.StatusActive)
	if err != nil {
		return err
	}

	type dueCommission struct {
		commissionID     int64
		affiliateID      int64
		affiliateUserID  int64
		commissionAmount float64
		refundedAmount   float64
	}
	dueItems := make([]dueCommission, 0)
	for rows.Next() {
		var item dueCommission
		if err := rows.Scan(&item.commissionID, &item.affiliateID, &item.affiliateUserID, &item.commissionAmount, &item.refundedAmount); err != nil {
			_ = rows.Close()
			return err
		}
		dueItems = append(dueItems, item)
		if result != nil {
			result.ScannedCount++
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}

	for _, item := range dueItems {
		amountDec := moneyx.Commission(item.commissionAmount).Sub(moneyx.Commission(item.refundedAmount)).Round(moneyx.ScaleCommission)
		if !amountDec.GreaterThan(moneyx.Commission(0)) {
			if _, err := exec.ExecContext(ctx, `
UPDATE custom_referral_commissions
SET status = $2,
    reversed_at = COALESCE(reversed_at, $3),
    updated_at = NOW()
WHERE id = $1
  AND status = $4`,
				item.commissionID,
				service.CustomReferralCommissionStatusReversed,
				now,
				service.CustomReferralCommissionStatusPending,
			); err != nil {
				return err
			}
			if result != nil {
				result.SkippedCount++
			}
			continue
		}
		amount := amountDec.InexactFloat64()
		if _, err := exec.ExecContext(ctx, `
UPDATE custom_referral_commissions
SET status = $2,
    available_at = $3,
    updated_at = NOW()
WHERE id = $1
  AND status = $4`,
			item.commissionID,
			service.CustomReferralCommissionStatusAvailable,
			now,
			service.CustomReferralCommissionStatusPending,
		); err != nil {
			return err
		}
		if _, err := exec.ExecContext(ctx, `
INSERT INTO custom_commission_accounts (affiliate_id, user_id, created_at, updated_at)
SELECT a.id, a.user_id, NOW(), NOW()
FROM custom_affiliates a
WHERE a.id = $1
  AND NOT EXISTS (
    SELECT 1
    FROM custom_commission_accounts ca
    WHERE ca.affiliate_id = a.id
  )
  AND NOT EXISTS (
    SELECT 1
    FROM custom_commission_accounts ca
    WHERE ca.user_id = a.user_id
  )
ON CONFLICT DO NOTHING`, item.affiliateID); err != nil {
			return err
		}

		debtRows, err := exec.QueryContext(ctx, `
SELECT debt_amount::double precision
FROM custom_commission_accounts
WHERE affiliate_id = $1
FOR UPDATE`, item.affiliateID)
		if err != nil {
			return err
		}
		debtAmount := 0.0
		if debtRows.Next() {
			if err := debtRows.Scan(&debtAmount); err != nil {
				_ = debtRows.Close()
				return err
			}
		}
		if err := debtRows.Close(); err != nil {
			return err
		}
		debtAmountDec := moneyx.NonNegative(moneyx.Commission(debtAmount))
		debtRepaidDec := moneyx.Min(amountDec, debtAmountDec)
		availableIncreaseDec := amountDec.Sub(debtRepaidDec).Round(moneyx.ScaleCommission)

		res, err := exec.ExecContext(ctx, `
UPDATE custom_commission_accounts
SET pending_amount = pending_amount - $2,
    available_amount = available_amount + $3,
    debt_amount = debt_amount - $4,
    updated_at = NOW()
WHERE affiliate_id = $1
  AND pending_amount + 0.00000001 >= $2
  AND debt_amount + 0.00000001 >= $4`, item.affiliateID, amount, availableIncreaseDec.InexactFloat64(), debtRepaidDec.InexactFloat64())
		if err != nil {
			return err
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			return service.ErrCustomReferralWithdrawInsufficient
		}
		if err := r.insertCommissionLedger(ctx, exec, customCommissionLedgerInsert{
			UserID:         item.affiliateUserID,
			AffiliateID:    item.affiliateID,
			CommissionID:   item.commissionID,
			Type:           "commission_settle",
			RefType:        "commission",
			RefID:          fmt.Sprintf("%d", item.commissionID),
			ExternalRefID:  fmt.Sprintf("settle:%d", item.commissionID),
			DeltaPending:   -amount,
			DeltaAvailable: availableIncreaseDec.InexactFloat64(),
			DeltaDebt:      debtRepaidDec.Neg().InexactFloat64(),
			Operator:       "system",
		}); err != nil {
			return err
		}
		if result != nil {
			result.SettledCount++
		}
	}
	return nil
}

func (r *customReferralRepository) CreateWithdrawal(ctx context.Context, input service.CustomReferralWithdrawalCreateInput, feeAmount float64) (*service.CustomReferralWithdrawal, error) {
	if strings.TrimSpace(input.IdempotencyKey) == "" {
		return nil, service.ErrCustomReferralInvalidIdempotency
	}
	var out *service.CustomReferralWithdrawal
	err := r.withTx(ctx, func(txCtx context.Context, exec sqlQueryExecutor) error {
		affiliate, err := r.getAffiliateByUserIDWithExecutor(txCtx, exec, input.UserID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return service.ErrCustomReferralPermissionDenied
			}
			return err
		}
		if affiliate.Status != service.CustomAffiliateStatusApproved || !affiliate.WithdrawalEnabled {
			return service.ErrCustomReferralWithdrawDisabled
		}
		if err := ensureAffiliateUserActive(txCtx, exec, affiliate.ID); err != nil {
			return err
		}

		accountRows, err := exec.QueryContext(txCtx, `
SELECT available_amount::double precision,
       debt_amount::double precision
FROM custom_commission_accounts
WHERE affiliate_id = $1
FOR UPDATE`, affiliate.ID)
		if err != nil {
			return err
		}
		availableAmount := 0.0
		debtAmount := 0.0
		if accountRows.Next() {
			if err := accountRows.Scan(&availableAmount, &debtAmount); err != nil {
				_ = accountRows.Close()
				return err
			}
		}
		if err := accountRows.Close(); err != nil {
			return err
		}
		if moneyx.NonNegative(moneyx.Commission(debtAmount)).GreaterThan(moneyx.Commission(0)) {
			return service.ErrCustomReferralOutstandingDebt
		}
		if key := strings.TrimSpace(input.IdempotencyKey); key != "" {
			existingRows, err := exec.QueryContext(txCtx, `
SELECT id
FROM custom_commission_withdrawals
WHERE affiliate_id = $1
  AND idempotency_key = $2
LIMIT 1
FOR UPDATE`, affiliate.ID, key)
			if err != nil {
				return err
			}
			if existingRows.Next() {
				var existingID int64
				if err := existingRows.Scan(&existingID); err != nil {
					_ = existingRows.Close()
					return err
				}
				_ = existingRows.Close()
				out, err = r.getWithdrawalByID(txCtx, exec, existingID)
				return err
			}
			if err := existingRows.Close(); err != nil {
				return err
			}
		}
		requestAmountDec := moneyx.Commission(input.Amount)
		availableAmountDec := moneyx.Commission(availableAmount)
		if availableAmountDec.LessThan(requestAmountDec) {
			return service.ErrCustomReferralWithdrawInsufficient
		}

		allocRows, err := exec.QueryContext(txCtx, `
SELECT c.id,
       (c.commission_amount::double precision - c.refunded_amount::double precision - COALESCE((
           SELECT SUM(wi.allocated_amount)::double precision
           FROM custom_commission_withdrawal_items wi
           WHERE wi.commission_id = c.id
             AND wi.status IN ('frozen', 'withdrawn')
       ), 0)) AS remaining
FROM custom_referral_commissions c
WHERE c.affiliate_id = $1
  AND c.status = $2
ORDER BY COALESCE(c.available_at, c.created_at) ASC, c.id ASC
FOR UPDATE`, affiliate.ID, service.CustomReferralCommissionStatusAvailable)
		if err != nil {
			return err
		}

		type allocation struct {
			commissionID int64
			amount       float64
		}
		remainingDec := requestAmountDec
		allocations := make([]allocation, 0)
		for allocRows.Next() && remainingDec.GreaterThan(moneyx.Commission(0)) {
			var commissionID int64
			var available float64
			if err := allocRows.Scan(&commissionID, &available); err != nil {
				_ = allocRows.Close()
				return err
			}
			availableDec := moneyx.Commission(available)
			if !availableDec.GreaterThan(moneyx.Commission(0)) {
				continue
			}
			useAmountDec := moneyx.Min(availableDec, remainingDec)
			allocations = append(allocations, allocation{commissionID: commissionID, amount: useAmountDec.InexactFloat64()})
			remainingDec = remainingDec.Sub(useAmountDec).Round(moneyx.ScaleCommission)
		}
		if err := allocRows.Err(); err != nil {
			return err
		}
		if err := allocRows.Close(); err != nil {
			return err
		}
		// Manual commission adjustments are currently reflected directly in
		// custom_commission_accounts.available_amount and ledger records rather
		// than custom_referral_commissions rows. As long as the locked account
		// balance covers the requested amount, we allow the unmatched remainder
		// to flow through this withdrawal without allocating extra
		// custom_commission_withdrawal_items rows.

		netAmountDec := requestAmountDec.Sub(moneyx.Commission(feeAmount)).Round(moneyx.ScaleCommission)
		if !netAmountDec.GreaterThan(moneyx.Commission(0)) {
			return service.ErrCustomReferralWithdrawTooSmall
		}
		netAmount := netAmountDec.InexactFloat64()

		now := time.Now()
		withdrawalColumns, err := loadTableColumns(txCtx, exec, "custom_commission_withdrawals")
		if err != nil {
			return err
		}
		insertColumns := make([]string, 0, 16)
		insertValues := make([]string, 0, 16)
		insertArgs := make([]any, 0, 16)
		appendInsert := func(column string, value any) {
			if !hasTableColumn(withdrawalColumns, column) {
				return
			}
			insertColumns = append(insertColumns, column)
			insertArgs = append(insertArgs, value)
			insertValues = append(insertValues, fmt.Sprintf("$%d", len(insertArgs)))
		}
		appendInsert("user_id", affiliate.UserID)
		appendInsert("affiliate_id", affiliate.ID)
		appendInsert("amount", requestAmountDec.InexactFloat64())
		appendInsert("fee_amount", feeAmount)
		appendInsert("net_amount", netAmount)
		appendInsert("channel", strings.TrimSpace(input.AccountType))
		appendInsert("account_type", strings.TrimSpace(input.AccountType))
		appendInsert("account_name", strings.TrimSpace(input.AccountName))
		appendInsert("real_name", strings.TrimSpace(input.AccountName))
		appendInsert("account_no", strings.TrimSpace(input.AccountNo))
		appendInsert("account_network", strings.TrimSpace(input.AccountNetwork))
		appendInsert("qr_image_url", strings.TrimSpace(input.QRImageURL))
		appendInsert("contact_info", strings.TrimSpace(input.ContactInfo))
		appendInsert("applicant_note", strings.TrimSpace(input.ApplicantNote))
		appendInsert("idempotency_key", strings.TrimSpace(input.IdempotencyKey))
		appendInsert("status", service.CustomReferralWithdrawalStatusPending)
		appendInsert("requested_at", now)
		appendInsert("submitted_at", now)

		withdrawRows, err := exec.QueryContext(txCtx, fmt.Sprintf(`
INSERT INTO custom_commission_withdrawals (
    %s, created_at, updated_at
)
VALUES (%s, NOW(), NOW())
RETURNING id`, strings.Join(insertColumns, ", "), strings.Join(insertValues, ", ")), insertArgs...)
		if err != nil {
			return err
		}
		var withdrawalID int64
		if !withdrawRows.Next() {
			_ = withdrawRows.Close()
			return fmt.Errorf("failed to create withdrawal")
		}
		if err := withdrawRows.Scan(&withdrawalID); err != nil {
			_ = withdrawRows.Close()
			return err
		}
		if err := withdrawRows.Close(); err != nil {
			return err
		}

		for _, item := range allocations {
			if _, err := exec.ExecContext(txCtx, `
INSERT INTO custom_commission_withdrawal_items (withdrawal_id, commission_id, allocated_amount, status, created_at)
VALUES ($1, $2, $3, $4, NOW())`,
				withdrawalID,
				item.commissionID,
				item.amount,
				service.CustomReferralWithdrawalItemStatusFrozen,
			); err != nil {
				return err
			}
		}

		if _, err := exec.ExecContext(txCtx, `
UPDATE custom_commission_accounts
SET available_amount = available_amount - $2,
    frozen_amount = frozen_amount + $2,
    updated_at = NOW()
WHERE affiliate_id = $1`, affiliate.ID, requestAmountDec.InexactFloat64()); err != nil {
			return err
		}
		if err := r.insertCommissionLedger(txCtx, exec, customCommissionLedgerInsert{
			UserID:         affiliate.UserID,
			AffiliateID:    affiliate.ID,
			WithdrawalID:   withdrawalID,
			Type:           "withdrawal_apply",
			RefType:        "withdrawal",
			RefID:          fmt.Sprintf("%d", withdrawalID),
			ExternalRefID:  fmt.Sprintf("withdrawal:%d", withdrawalID),
			DeltaAvailable: requestAmountDec.Neg().InexactFloat64(),
			DeltaFrozen:    requestAmountDec.InexactFloat64(),
			Remark:         strings.TrimSpace(input.ApplicantNote),
			Operator:       "user",
			OperatorType:   "user",
			OperatorID:     affiliate.UserID,
		}); err != nil {
			return err
		}

		withdrawal, err := r.getWithdrawalByID(txCtx, exec, withdrawalID)
		if err != nil {
			return err
		}
		out = withdrawal
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *customReferralRepository) ListWithdrawalsByUserID(ctx context.Context, userID int64, params service.CustomReferralWithdrawalListParams) ([]service.CustomReferralWithdrawal, int64, error) {
	return r.listWithdrawals(ctx, params, "WHERE a.user_id = $1 AND ($2 = '' OR w.status = $2)", userID)
}

func (r *customReferralRepository) ListAffiliateWithdrawals(ctx context.Context, affiliateUserID int64, params service.CustomReferralWithdrawalListParams) ([]service.CustomReferralWithdrawal, int64, error) {
	return r.listWithdrawals(ctx, params, "WHERE a.user_id = $1 AND ($2 = '' OR w.status = $2)", affiliateUserID)
}

func (r *customReferralRepository) ListWithdrawals(ctx context.Context, params service.CustomReferralWithdrawalListParams) ([]service.CustomReferralWithdrawal, int64, error) {
	return r.listWithdrawals(ctx, params, "WHERE ($1 = '' OR w.status = $1)")
}

func (r *customReferralRepository) listWithdrawals(ctx context.Context, params service.CustomReferralWithdrawalListParams, where string, args ...any) ([]service.CustomReferralWithdrawal, int64, error) {
	exec := txAwareSQLExecutor(ctx, r.sql, r.client)
	if exec == nil {
		return nil, 0, fmt.Errorf("custom referral sql executor is not configured")
	}
	offset := (params.Page - 1) * params.PageSize
	status := strings.TrimSpace(params.Status)
	countSQL := `
SELECT COUNT(*)
FROM custom_commission_withdrawals w
JOIN custom_affiliates a ON a.id = w.affiliate_id
LEFT JOIN users u ON u.id = a.user_id
` + where
	listSQL := `
SELECT w.id,
       w.affiliate_id,
       a.user_id,
       COALESCE(u.email, ''),
       a.invite_code,
       w.amount::double precision,
       w.fee_amount::double precision,
       w.net_amount::double precision,
       w.account_type,
       COALESCE(w.account_name, ''),
       COALESCE(w.account_no, ''),
       COALESCE(w.account_network, ''),
       COALESCE(w.qr_image_url, ''),
       COALESCE(w.contact_info, ''),
       COALESCE(w.applicant_note, ''),
       COALESCE(w.admin_note, ''),
       COALESCE(w.payment_proof_url, ''),
       COALESCE(w.payment_txn_no, ''),
       w.status,
       w.submitted_at,
       w.approved_at,
       w.payout_deadline_at,
       w.paid_at,
       w.rejected_at,
       w.canceled_at,
       COALESCE(w.reject_reason, '')
FROM custom_commission_withdrawals w
JOIN custom_affiliates a ON a.id = w.affiliate_id
LEFT JOIN users u ON u.id = a.user_id
` + where + `
ORDER BY w.submitted_at DESC, w.id DESC`

	countArgs := append([]any{}, args...)
	if strings.Contains(where, "$2") {
		countArgs = append(countArgs, status)
	} else {
		countArgs = []any{status}
	}
	countRows, err := exec.QueryContext(ctx, countSQL, countArgs...)
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if countRows.Next() {
		if err := countRows.Scan(&total); err != nil {
			_ = countRows.Close()
			return nil, 0, err
		}
	}
	if err := countRows.Close(); err != nil {
		return nil, 0, err
	}

	listArgs := append([]any{}, args...)
	if strings.Contains(where, "$2") {
		listSQL += `
LIMIT $3 OFFSET $4`
		listArgs = append(listArgs, status, params.PageSize, offset)
	} else {
		listSQL += `
LIMIT $2 OFFSET $3`
		listArgs = []any{status, params.PageSize, offset}
	}
	rows, err := exec.QueryContext(ctx, listSQL, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]service.CustomReferralWithdrawal, 0)
	for rows.Next() {
		item, err := scanCustomReferralWithdrawal(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *customReferralRepository) CancelWithdrawal(ctx context.Context, withdrawalID, userID int64) (*service.CustomReferralWithdrawal, error) {
	return r.releaseWithdrawal(ctx, withdrawalID, userID, "user", service.CustomReferralWithdrawalStatusCanceled, "", true)
}

func (r *customReferralRepository) ApproveWithdrawal(ctx context.Context, input service.CustomReferralWithdrawalReviewInput, deadlineAt time.Time) (*service.CustomReferralWithdrawal, error) {
	var out *service.CustomReferralWithdrawal
	err := r.withTx(ctx, func(txCtx context.Context, exec sqlQueryExecutor) error {
		if err := ensureWithdrawalAffiliateUserActive(txCtx, exec, input.WithdrawalID); err != nil {
			return err
		}
		now := time.Now()
		rows, err := exec.QueryContext(txCtx, `
UPDATE custom_commission_withdrawals
SET status = $2,
    approved_at = $3,
    payout_deadline_at = $4,
    reviewed_by = $5,
    admin_note = $6,
    updated_at = NOW()
WHERE id = $1
  AND status = $7
RETURNING id`,
			input.WithdrawalID,
			service.CustomReferralWithdrawalStatusApproved,
			now,
			deadlineAt,
			adminIDOrNil(input.AdminUserID),
			strings.TrimSpace(input.AdminNote),
			service.CustomReferralWithdrawalStatusPending,
		)
		if err != nil {
			return err
		}
		if !rows.Next() {
			_ = rows.Close()
			return service.ErrCustomReferralWithdrawalNotFound
		}
		var updatedID int64
		if err := rows.Scan(&updatedID); err != nil {
			_ = rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		out, err = r.getWithdrawalByID(txCtx, exec, input.WithdrawalID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *customReferralRepository) RejectWithdrawal(ctx context.Context, input service.CustomReferralWithdrawalReviewInput) (*service.CustomReferralWithdrawal, error) {
	return r.releaseWithdrawalWithNote(ctx, input.WithdrawalID, input.AdminUserID, "admin", service.CustomReferralWithdrawalStatusRejected, input.RejectReason, input.AdminNote, false)
}

func (r *customReferralRepository) MarkWithdrawalPaid(ctx context.Context, input service.CustomReferralWithdrawalPayInput) (*service.CustomReferralWithdrawal, error) {
	var out *service.CustomReferralWithdrawal
	err := r.withTx(ctx, func(txCtx context.Context, exec sqlQueryExecutor) error {
		if err := ensureWithdrawalAffiliateUserActive(txCtx, exec, input.WithdrawalID); err != nil {
			return err
		}
		now := time.Now()
		rows, err := exec.QueryContext(txCtx, `
UPDATE custom_commission_withdrawals
SET status = $2,
    paid_at = $3,
    reviewed_by = $4,
    admin_note = $5,
    payment_proof_url = $6,
    payment_txn_no = $7,
    updated_at = NOW()
WHERE id = $1
  AND status = $8
RETURNING affiliate_id, amount::double precision`,
			input.WithdrawalID,
			service.CustomReferralWithdrawalStatusPaid,
			now,
			adminIDOrNil(input.AdminUserID),
			strings.TrimSpace(input.AdminNote),
			strings.TrimSpace(input.PaymentProofURL),
			strings.TrimSpace(input.PaymentTxnNo),
			service.CustomReferralWithdrawalStatusApproved,
		)
		if err != nil {
			return err
		}
		if !rows.Next() {
			_ = rows.Close()
			return service.ErrCustomReferralWithdrawalNotFound
		}
		var affiliateID int64
		var amount float64
		if err := rows.Scan(&affiliateID, &amount); err != nil {
			_ = rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		affiliateRows, err := exec.QueryContext(txCtx, `
SELECT user_id
FROM custom_affiliates
WHERE id = $1`, affiliateID)
		if err != nil {
			return err
		}
		affiliateUserID := int64(0)
		if affiliateRows.Next() {
			if err := affiliateRows.Scan(&affiliateUserID); err != nil {
				_ = affiliateRows.Close()
				return err
			}
		}
		if err := affiliateRows.Close(); err != nil {
			return err
		}
		debtRows, err := exec.QueryContext(txCtx, `
SELECT debt_amount::double precision
FROM custom_commission_accounts
WHERE affiliate_id = $1
FOR UPDATE`, affiliateID)
		if err != nil {
			return err
		}
		debtAmount := 0.0
		if debtRows.Next() {
			if err := debtRows.Scan(&debtAmount); err != nil {
				_ = debtRows.Close()
				return err
			}
		}
		if err := debtRows.Close(); err != nil {
			return err
		}
		if moneyx.NonNegative(moneyx.Commission(debtAmount)).GreaterThan(moneyx.Commission(0)) {
			return service.ErrCustomReferralOutstandingDebt
		}
		res, err := exec.ExecContext(txCtx, `
UPDATE custom_commission_accounts
SET frozen_amount = frozen_amount - $2,
    withdrawn_amount = withdrawn_amount + $2,
    updated_at = NOW()
WHERE affiliate_id = $1
  AND frozen_amount + 0.00000001 >= $2`, affiliateID, amount)
		if err != nil {
			return err
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			return service.ErrCustomReferralWithdrawInsufficient
		}
		if _, err := exec.ExecContext(txCtx, `
UPDATE custom_commission_withdrawal_items
SET status = $2
WHERE withdrawal_id = $1
  AND status = $3`,
			input.WithdrawalID,
			service.CustomReferralWithdrawalItemStatusWithdrawn,
			service.CustomReferralWithdrawalItemStatusFrozen,
		); err != nil {
			return err
		}
		if err := r.insertCommissionLedger(txCtx, exec, customCommissionLedgerInsert{
			UserID:         affiliateUserID,
			AffiliateID:    affiliateID,
			WithdrawalID:   input.WithdrawalID,
			Type:           "withdrawal_paid",
			RefType:        "withdrawal",
			RefID:          fmt.Sprintf("%d", input.WithdrawalID),
			ExternalRefID:  fmt.Sprintf("withdrawal:%d", input.WithdrawalID),
			DeltaFrozen:    -amount,
			DeltaWithdrawn: amount,
			Remark:         strings.TrimSpace(input.AdminNote),
			Operator:       "admin",
			OperatorType:   "admin",
			OperatorID:     input.AdminUserID,
		}); err != nil {
			return err
		}
		out, err = r.getWithdrawalByID(txCtx, exec, input.WithdrawalID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *customReferralRepository) releaseWithdrawal(ctx context.Context, withdrawalID, actorUserID int64, operator, targetStatus, reason string, actorMustOwn bool) (*service.CustomReferralWithdrawal, error) {
	return r.releaseWithdrawalWithNote(ctx, withdrawalID, actorUserID, operator, targetStatus, reason, "", actorMustOwn)
}

func (r *customReferralRepository) releaseWithdrawalWithNote(ctx context.Context, withdrawalID, actorUserID int64, operator, targetStatus, reason, adminNote string, actorMustOwn bool) (*service.CustomReferralWithdrawal, error) {
	var out *service.CustomReferralWithdrawal
	err := r.withTx(ctx, func(txCtx context.Context, exec sqlQueryExecutor) error {
		now := time.Now()
		type lockedWithdrawal struct {
			id              int64
			affiliateID     int64
			affiliateUserID int64
			amount          float64
		}
		var item lockedWithdrawal
		switch targetStatus {
		case service.CustomReferralWithdrawalStatusCanceled:
			rows, err := exec.QueryContext(txCtx, `
UPDATE custom_commission_withdrawals w
SET status = $2,
    canceled_at = $3,
    canceled_by = $4,
    updated_at = NOW()
FROM custom_affiliates a
WHERE w.id = $1
  AND w.affiliate_id = a.id
  AND w.status = $5
  AND ($6 = FALSE OR a.user_id = $7)
RETURNING w.id, w.affiliate_id, a.user_id, w.amount::double precision`,
				withdrawalID,
				targetStatus,
				now,
				adminIDOrNil(actorUserID),
				service.CustomReferralWithdrawalStatusPending,
				actorMustOwn,
				actorUserID,
			)
			if err != nil {
				return err
			}
			if !rows.Next() {
				_ = rows.Close()
				return service.ErrCustomReferralWithdrawalNotFound
			}
			if err := rows.Scan(&item.id, &item.affiliateID, &item.affiliateUserID, &item.amount); err != nil {
				_ = rows.Close()
				return err
			}
			if err := rows.Close(); err != nil {
				return err
			}
		case service.CustomReferralWithdrawalStatusRejected:
			rows, err := exec.QueryContext(txCtx, `
UPDATE custom_commission_withdrawals w
SET status = $2,
    rejected_at = $3,
    reviewed_by = $4,
    reject_reason = $5,
    admin_note = $6,
    updated_at = NOW()
FROM custom_affiliates a
WHERE w.id = $1
  AND w.affiliate_id = a.id
  AND w.status = $7
RETURNING w.id, w.affiliate_id, a.user_id, w.amount::double precision`,
				withdrawalID,
				targetStatus,
				now,
				adminIDOrNil(actorUserID),
				strings.TrimSpace(reason),
				strings.TrimSpace(adminNote),
				service.CustomReferralWithdrawalStatusPending,
			)
			if err != nil {
				return err
			}
			if !rows.Next() {
				_ = rows.Close()
				return service.ErrCustomReferralWithdrawalNotFound
			}
			if err := rows.Scan(&item.id, &item.affiliateID, &item.affiliateUserID, &item.amount); err != nil {
				_ = rows.Close()
				return err
			}
			if err := rows.Close(); err != nil {
				return err
			}
		default:
			return service.ErrCustomReferralWithdrawalNotFound
		}

		debtRows, err := exec.QueryContext(txCtx, `
SELECT debt_amount::double precision
FROM custom_commission_accounts
WHERE affiliate_id = $1
FOR UPDATE`, item.affiliateID)
		if err != nil {
			return err
		}
		debtAmount := 0.0
		if debtRows.Next() {
			if err := debtRows.Scan(&debtAmount); err != nil {
				_ = debtRows.Close()
				return err
			}
		}
		if err := debtRows.Close(); err != nil {
			return err
		}
		itemAmountDec := moneyx.Commission(item.amount)
		debtAmountDec := moneyx.NonNegative(moneyx.Commission(debtAmount))
		debtRepaidDec := moneyx.Min(itemAmountDec, debtAmountDec)
		availableReturnedDec := itemAmountDec.Sub(debtRepaidDec).Round(moneyx.ScaleCommission)

		res, err := exec.ExecContext(txCtx, `
UPDATE custom_commission_accounts
SET frozen_amount = frozen_amount - $2,
    available_amount = available_amount + $3,
    debt_amount = debt_amount - $4,
    updated_at = NOW()
WHERE affiliate_id = $1
  AND frozen_amount + 0.00000001 >= $2`, item.affiliateID, itemAmountDec.InexactFloat64(), availableReturnedDec.InexactFloat64(), debtRepaidDec.InexactFloat64())
		if err != nil {
			return err
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			return service.ErrCustomReferralWithdrawInsufficient
		}
		if _, err := exec.ExecContext(txCtx, `
UPDATE custom_commission_withdrawal_items
SET status = $2
WHERE withdrawal_id = $1
  AND status = $3`,
			withdrawalID,
			service.CustomReferralWithdrawalItemStatusReleased,
			service.CustomReferralWithdrawalItemStatusFrozen,
		); err != nil {
			return err
		}
		ledgerType := "withdrawal_cancel"
		if targetStatus == service.CustomReferralWithdrawalStatusRejected {
			ledgerType = "withdrawal_reject"
		}
		if err := r.insertCommissionLedger(txCtx, exec, customCommissionLedgerInsert{
			UserID:         item.affiliateUserID,
			AffiliateID:    item.affiliateID,
			WithdrawalID:   item.id,
			Type:           ledgerType,
			RefType:        "withdrawal",
			RefID:          fmt.Sprintf("%d", item.id),
			ExternalRefID:  fmt.Sprintf("withdrawal:%d", item.id),
			DeltaAvailable: availableReturnedDec.InexactFloat64(),
			DeltaFrozen:    itemAmountDec.Neg().InexactFloat64(),
			DeltaDebt:      debtRepaidDec.Neg().InexactFloat64(),
			Remark:         strings.TrimSpace(reason),
			Operator:       operator,
			OperatorType:   operator,
			OperatorID:     actorUserID,
		}); err != nil {
			return err
		}
		out, err = r.getWithdrawalByID(txCtx, exec, withdrawalID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *customReferralRepository) getWithdrawalByID(ctx context.Context, exec sqlQueryExecutor, withdrawalID int64) (*service.CustomReferralWithdrawal, error) {
	rows, err := exec.QueryContext(ctx, `
SELECT w.id,
       w.affiliate_id,
       a.user_id,
       COALESCE(u.email, ''),
       a.invite_code,
       w.amount::double precision,
       w.fee_amount::double precision,
       w.net_amount::double precision,
       w.account_type,
       COALESCE(w.account_name, ''),
       COALESCE(w.account_no, ''),
       COALESCE(w.account_network, ''),
       COALESCE(w.qr_image_url, ''),
       COALESCE(w.contact_info, ''),
       COALESCE(w.applicant_note, ''),
       COALESCE(w.admin_note, ''),
       COALESCE(w.payment_proof_url, ''),
       COALESCE(w.payment_txn_no, ''),
       w.status,
       w.submitted_at,
       w.approved_at,
       w.payout_deadline_at,
       w.paid_at,
       w.rejected_at,
       w.canceled_at,
       COALESCE(w.reject_reason, '')
FROM custom_commission_withdrawals w
JOIN custom_affiliates a ON a.id = w.affiliate_id
LEFT JOIN users u ON u.id = a.user_id
WHERE w.id = $1`, withdrawalID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrCustomReferralWithdrawalNotFound
	}
	return scanCustomReferralWithdrawal(rows)
}

func (r *customReferralRepository) withTx(ctx context.Context, fn func(txCtx context.Context, exec sqlQueryExecutor) error) error {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		exec := txAwareSQLExecutor(ctx, r.sql, r.client)
		if exec == nil {
			return fmt.Errorf("custom referral tx executor is not configured")
		}
		return fn(ctx, exec)
	}

	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	exec := txAwareSQLExecutor(txCtx, r.sql, r.client)
	if exec == nil {
		return fmt.Errorf("custom referral tx executor is not configured")
	}
	if err := fn(txCtx, exec); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *customReferralRepository) generateUniqueInviteCode(ctx context.Context, exec sqlQueryExecutor) (string, error) {
	for i := 0; i < customInviteCodeMaxAttempts; i++ {
		code, err := generateCustomInviteCode()
		if err != nil {
			return "", err
		}
		rows, queryErr := exec.QueryContext(ctx, `SELECT 1 FROM custom_affiliates WHERE invite_code = $1`, code)
		if queryErr != nil {
			return "", queryErr
		}
		exists := rows.Next()
		if closeErr := rows.Close(); closeErr != nil {
			return "", closeErr
		}
		if !exists {
			return code, nil
		}
	}
	return "", fmt.Errorf("unable to generate unique custom invite code")
}

func generateCustomInviteCode() (string, error) {
	buf := make([]byte, customInviteCodeLength)
	_, err := rand.Read(buf)
	if err != nil {
		return "", err
	}
	for i := range buf {
		buf[i] = customInviteCodeCharset[int(buf[i])%len(customInviteCodeCharset)]
	}
	return string(buf), nil
}

func generateSettlementBatchNo() (string, error) {
	buf := make([]byte, 4)
	_, err := rand.Read(buf)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("settle-%s-%x", time.Now().UTC().Format("20060102150405"), buf), nil
}

func (r *customReferralRepository) RecordAdminAudit(ctx context.Context, targetUserID int64, audit service.CustomReferralAdminAuditContext) error {
	return r.withTx(ctx, func(txCtx context.Context, exec sqlQueryExecutor) error {
		var affiliateID int64
		if targetUserID > 0 {
			item, err := r.getAffiliateByUserIDWithExecutor(txCtx, exec, targetUserID)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if item != nil {
				affiliateID = item.ID
			}
		}
		return r.recordAdminAuditWithExecutor(txCtx, exec, targetUserID, affiliateID, audit)
	})
}

func (r *customReferralRepository) recordAdminAuditWithExecutor(ctx context.Context, exec sqlQueryExecutor, targetUserID, affiliateID int64, audit service.CustomReferralAdminAuditContext) error {
	if exec == nil {
		return fmt.Errorf("custom referral audit executor is not configured")
	}
	action := strings.TrimSpace(audit.Action)
	if action == "" {
		action = "custom_referral_admin_update"
	}
	oldValue, err := json.Marshal(nonNilAuditMap(audit.OldValue))
	if err != nil {
		return err
	}
	newValue, err := json.Marshal(nonNilAuditMap(audit.NewValue))
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, `
INSERT INTO custom_referral_admin_audit_logs (
    action, target_user_id, affiliate_id, admin_user_id, reason, ip, user_agent, old_value, new_value, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9::jsonb, NOW())`,
		action,
		adminIDOrNil(targetUserID),
		adminIDOrNil(affiliateID),
		adminIDOrNil(audit.AdminUserID),
		strings.TrimSpace(audit.Reason),
		strings.TrimSpace(audit.IP),
		strings.TrimSpace(audit.UserAgent),
		string(oldValue),
		string(newValue),
	)
	return err
}

func customAffiliateStatusAuditValue(item *service.CustomAffiliate) map[string]any {
	if item == nil {
		return map[string]any{}
	}
	return map[string]any{
		"status":              item.Status,
		"acquisition_enabled": item.AcquisitionEnabled,
		"settlement_enabled":  item.SettlementEnabled,
		"withdrawal_enabled":  item.WithdrawalEnabled,
		"risk_reason":         item.RiskReason,
		"rate_override":       item.RateOverride,
	}
}

func nonNilAuditMap(v map[string]any) map[string]any {
	if v == nil {
		return map[string]any{}
	}
	return v
}

func scanCustomAffiliate(scanner interface{ Scan(dest ...any) error }) (*service.CustomAffiliate, error) {
	var out service.CustomAffiliate
	var rate sql.NullFloat64
	var approvedAt sql.NullTime
	var disabledAt sql.NullTime
	if err := scanner.Scan(
		&out.ID,
		&out.UserID,
		&out.Email,
		&out.Username,
		&out.InviteCode,
		&out.Status,
		&out.SourceType,
		&rate,
		&out.AcquisitionEnabled,
		&out.SettlementEnabled,
		&out.WithdrawalEnabled,
		&out.RiskReason,
		&out.RiskNote,
		&approvedAt,
		&disabledAt,
	); err != nil {
		return nil, err
	}
	if rate.Valid {
		out.RateOverride = &rate.Float64
	}
	if approvedAt.Valid {
		out.ApprovedAt = &approvedAt.Time
	}
	if disabledAt.Valid {
		out.DisabledAt = &disabledAt.Time
	}
	return &out, nil
}

func scanCustomAffiliateWithStats(scanner interface{ Scan(dest ...any) error }) (*service.CustomAffiliate, error) {
	var out service.CustomAffiliate
	var rate sql.NullFloat64
	var approvedAt sql.NullTime
	var disabledAt sql.NullTime
	if err := scanner.Scan(
		&out.ID,
		&out.UserID,
		&out.Email,
		&out.Username,
		&out.InviteCode,
		&out.Status,
		&out.SourceType,
		&rate,
		&out.ClickCount,
		&out.BoundUserCount,
		&out.PaidUserCount,
		&out.PendingAmount,
		&out.AvailableAmount,
		&out.WithdrawnAmount,
		&out.AcquisitionEnabled,
		&out.SettlementEnabled,
		&out.WithdrawalEnabled,
		&out.RiskReason,
		&out.RiskNote,
		&approvedAt,
		&disabledAt,
	); err != nil {
		return nil, err
	}
	if rate.Valid {
		out.RateOverride = &rate.Float64
	}
	if approvedAt.Valid {
		out.ApprovedAt = &approvedAt.Time
	}
	if disabledAt.Valid {
		out.DisabledAt = &disabledAt.Time
	}
	return &out, nil
}

func scanCustomReferralWithdrawal(scanner interface{ Scan(dest ...any) error }) (*service.CustomReferralWithdrawal, error) {
	var out service.CustomReferralWithdrawal
	var approvedAt sql.NullTime
	var payoutDeadlineAt sql.NullTime
	var paidAt sql.NullTime
	var rejectedAt sql.NullTime
	var canceledAt sql.NullTime
	if err := scanner.Scan(
		&out.ID,
		&out.AffiliateID,
		&out.AffiliateUserID,
		&out.AffiliateEmail,
		&out.InviteCode,
		&out.Amount,
		&out.FeeAmount,
		&out.NetAmount,
		&out.AccountType,
		&out.AccountName,
		&out.AccountNo,
		&out.AccountNetwork,
		&out.QRImageURL,
		&out.ContactInfo,
		&out.ApplicantNote,
		&out.AdminNote,
		&out.PaymentProofURL,
		&out.PaymentTxnNo,
		&out.Status,
		&out.SubmittedAt,
		&approvedAt,
		&payoutDeadlineAt,
		&paidAt,
		&rejectedAt,
		&canceledAt,
		&out.RejectReason,
	); err != nil {
		return nil, err
	}
	if approvedAt.Valid {
		out.ApprovedAt = &approvedAt.Time
	}
	if payoutDeadlineAt.Valid {
		out.PayoutDeadlineAt = &payoutDeadlineAt.Time
	}
	if paidAt.Valid {
		out.PaidAt = &paidAt.Time
	}
	if rejectedAt.Valid {
		out.RejectedAt = &rejectedAt.Time
	}
	if canceledAt.Valid {
		out.CanceledAt = &canceledAt.Time
	}
	return &out, nil
}

func scanCustomReferralCommission(scanner interface{ Scan(dest ...any) error }) (*service.CustomReferralCommission, error) {
	var out service.CustomReferralCommission
	var availableAt sql.NullTime
	var reversedAt sql.NullTime
	if err := scanner.Scan(
		&out.ID,
		&out.AffiliateID,
		&out.AffiliateUserID,
		&out.AffiliateEmail,
		&out.OrderID,
		&out.InviteeUserID,
		&out.InviteeEmail,
		&out.InviteeUsername,
		&out.OrderType,
		&out.BaseAmount,
		&out.Rate,
		&out.CommissionAmount,
		&out.RefundedAmount,
		&out.Status,
		&out.SettleAt,
		&availableAt,
		&reversedAt,
		&out.ReversedReason,
		&out.CreatedAt,
	); err != nil {
		return nil, err
	}
	if availableAt.Valid {
		out.AvailableAt = &availableAt.Time
	}
	if reversedAt.Valid {
		out.ReversedAt = &reversedAt.Time
	}
	return &out, nil
}

func scanCustomCommissionReversal(scanner interface{ Scan(dest ...any) error }) (*service.CustomReferralCommissionReversal, error) {
	var out service.CustomReferralCommissionReversal
	if err := scanner.Scan(
		&out.ID,
		&out.AffiliateID,
		&out.CommissionID,
		&out.OrderID,
		&out.RefundAmount,
		&out.ReverseAmount,
		&out.DeltaPending,
		&out.DeltaAvailable,
		&out.DeltaFrozen,
		&out.DeltaReversed,
		&out.DeltaDebt,
		&out.Reason,
		&out.ExternalRefID,
		&out.AdminUserID,
		&out.CreatedAt,
	); err != nil {
		return nil, err
	}
	return &out, nil
}

func adminIDOrNil(v int64) any {
	if v <= 0 {
		return nil
	}
	return v
}

func customRoundTo(v float64, scale int) float64 {
	return moneyx.Round(v, int32(scale))
}

func isPQUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}
