package repository

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type trafficPackRepository struct {
	db         *sql.DB
	isPostgres bool
}

func NewTrafficPackRepository(db *sql.DB) service.TrafficPackRepository {
	return &trafficPackRepository{db: db, isPostgres: isPostgresDriver(db)}
}

func (r *trafficPackRepository) ListForSale(ctx context.Context) ([]service.TrafficPack, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, code, name, description, price, credit_usd, validity_days, platform, for_sale, sort_order
		FROM traffic_packs
		WHERE for_sale = TRUE
		ORDER BY sort_order ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	packs := []service.TrafficPack{}
	for rows.Next() {
		var pack service.TrafficPack
		if err := rows.Scan(&pack.ID, &pack.Code, &pack.Name, &pack.Description, &pack.Price, &pack.CreditUSD, &pack.ValidityDays, &pack.Platform, &pack.ForSale, &pack.SortOrder); err != nil {
			return nil, err
		}
		packs = append(packs, pack)
	}
	return packs, rows.Err()
}

func (r *trafficPackRepository) GetForSaleByID(ctx context.Context, id int64) (*service.TrafficPack, error) {
	var pack service.TrafficPack
	err := r.db.QueryRowContext(ctx, `
		SELECT id, code, name, description, price, credit_usd, validity_days, platform, for_sale, sort_order
		FROM traffic_packs
		WHERE id = $1 AND for_sale = TRUE
	`, id).Scan(&pack.ID, &pack.Code, &pack.Name, &pack.Description, &pack.Price, &pack.CreditUSD, &pack.ValidityDays, &pack.Platform, &pack.ForSale, &pack.SortOrder)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrInvalidInput
	}
	if err != nil {
		return nil, err
	}
	return &pack, nil
}

func (r *trafficPackRepository) GetSummary(ctx context.Context, userID int64, now time.Time) (*service.TrafficCreditSummary, error) {
	summary := &service.TrafficCreditSummary{}
	if err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(remaining_usd), 0)
		FROM user_traffic_credits
		WHERE user_id = $1 AND platform = $2 AND remaining_usd > 0 AND expires_at > $3
	`, userID, service.TrafficPackPlatformOpenAI, now).Scan(&summary.TotalRemainingUSD); err != nil {
		return nil, err
	}
	var nextExpiresAt time.Time
	err := r.db.QueryRowContext(ctx, `
		SELECT expires_at, COALESCE(SUM(remaining_usd), 0)
		FROM user_traffic_credits
		WHERE user_id = $1 AND platform = $2 AND remaining_usd > 0 AND expires_at > $3
		GROUP BY expires_at
		ORDER BY expires_at ASC
		LIMIT 1
	`, userID, service.TrafficPackPlatformOpenAI, now).Scan(&nextExpiresAt, &summary.NextExpiringUSD)
	if errors.Is(err, sql.ErrNoRows) {
		return summary, nil
	}
	if err != nil {
		return nil, err
	}
	summary.NextExpiresAt = &nextExpiresAt
	return summary, nil
}

func (r *trafficPackRepository) HasAvailableCredit(ctx context.Context, userID int64, now time.Time) (bool, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		SELECT id FROM user_traffic_credits
		WHERE user_id = $1 AND platform = $2 AND remaining_usd > 0 AND expires_at > $3
		ORDER BY expires_at ASC, credited_at ASC, id ASC
		LIMIT 1
	`, userID, service.TrafficPackPlatformOpenAI, now).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (r *trafficPackRepository) CreditPurchase(ctx context.Context, input service.CreditTrafficPackInput) error {
	validityDays := input.ValidityDays
	if validityDays <= 0 {
		validityDays = service.TrafficPackValidityDays
	}
	creditUSD := roundTrafficPackUSD(input.CreditUSD)
	if input.UserID <= 0 || input.OrderID <= 0 || creditUSD <= 0 {
		return service.ErrInvalidInput
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	expiresAt := input.CreditedAt.AddDate(0, 0, validityDays)
	var creditID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO user_traffic_credits (user_id, order_id, pack_id, platform, initial_usd, remaining_usd, credited_at, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $5, $6, $7, $6, $6)
		ON CONFLICT (order_id) DO NOTHING
		RETURNING id
	`, input.UserID, input.OrderID, input.PackID, service.TrafficPackPlatformOpenAI, creditUSD, input.CreditedAt, expiresAt).Scan(&creditID)
	if errors.Is(err, sql.ErrNoRows) {
		return tx.Commit()
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO traffic_credit_ledger (user_id, credit_id, order_id, request_id, entry_type, amount_usd, balance_after_usd, created_at)
		VALUES ($1, $2, $3, '', $4, $5, $5, $6)
	`, input.UserID, creditID, input.OrderID, service.TrafficCreditLedgerTypePurchase, creditUSD, input.CreditedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *trafficPackRepository) Deduct(ctx context.Context, userID int64, amountUSD float64, requestID string, now time.Time) (bool, []service.TrafficCreditDeduction, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, nil, err
	}
	defer tx.Rollback()
	batches, err := r.listDeductibleCredits(ctx, tx, userID, now)
	if err != nil {
		return false, nil, err
	}
	deductions, covered := service.PlanTrafficCreditDeductions(batches, roundTrafficPackUSD(amountUSD))
	if !covered {
		return false, nil, nil
	}
	for _, deduction := range deductions {
		balanceAfter, err := decrementTrafficCredit(ctx, tx, deduction.CreditID, deduction.AmountUSD, now)
		if err != nil {
			return false, nil, err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO traffic_credit_ledger (user_id, credit_id, order_id, request_id, entry_type, amount_usd, balance_after_usd, created_at)
			VALUES ($1, $2, NULL, $3, $4, $5, $6, $7)
		`, userID, deduction.CreditID, requestID, service.TrafficCreditLedgerTypeDeduction, deduction.AmountUSD, balanceAfter, now); err != nil {
			return false, nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return false, nil, err
	}
	return true, deductions, nil
}

func (r *trafficPackRepository) listDeductibleCredits(ctx context.Context, tx *sql.Tx, userID int64, now time.Time) ([]service.TrafficCreditBatch, error) {
	lockClause := ""
	if r.isPostgres {
		lockClause = " FOR UPDATE"
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id, user_id, order_id, pack_id, initial_usd, remaining_usd, credited_at, expires_at
		FROM user_traffic_credits
		WHERE user_id = $1 AND platform = $2 AND remaining_usd > 0 AND expires_at > $3
		ORDER BY expires_at ASC, credited_at ASC, id ASC
	`+lockClause, userID, service.TrafficPackPlatformOpenAI, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	batches := []service.TrafficCreditBatch{}
	for rows.Next() {
		var batch service.TrafficCreditBatch
		var orderID sql.NullInt64
		var packID sql.NullInt64
		if err := rows.Scan(&batch.ID, &batch.UserID, &orderID, &packID, &batch.InitialUSD, &batch.RemainingUSD, &batch.CreditedAt, &batch.ExpiresAt); err != nil {
			return nil, err
		}
		if orderID.Valid {
			batch.OrderID = &orderID.Int64
		}
		if packID.Valid {
			batch.PackID = &packID.Int64
		}
		batches = append(batches, batch)
	}
	return batches, rows.Err()
}

func decrementTrafficCredit(ctx context.Context, tx *sql.Tx, creditID int64, amountUSD float64, now time.Time) (float64, error) {
	var balanceAfter float64
	err := tx.QueryRowContext(ctx, `
		UPDATE user_traffic_credits SET remaining_usd = remaining_usd - $1, updated_at = $2
		WHERE id = $3 AND remaining_usd + 0.0000000001 >= $1
		RETURNING remaining_usd
	`, amountUSD, now, creditID).Scan(&balanceAfter)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, service.ErrInvalidInput
	}
	if err != nil {
		return 0, err
	}
	return roundTrafficPackUSD(balanceAfter), nil
}

func roundTrafficPackUSD(value float64) float64 {
	return math.Round(value*1e10) / 1e10
}
