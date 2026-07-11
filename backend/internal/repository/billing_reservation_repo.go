package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/shopspring/decimal"
)

const billingReservationColumns = `
	id, reservation_key, source_type, source_id, user_id, api_key_id,
	reserved_amount_usd::text, settled_amount_usd::text, status,
	expires_at, created_at, updated_at, settled_at, released_at`

type sqlRowScanner interface {
	Scan(dest ...any) error
}

type billingReservationRepository struct {
	db *sql.DB
}

var _ service.BillingReservationReaperRepository = (*billingReservationRepository)(nil)

func NewBillingReservationRepository(db *sql.DB) service.BillingReservationRepository {
	return &billingReservationRepository{db: db}
}

func (r *billingReservationRepository) Reserve(ctx context.Context, reservation *service.BillingReservation) (*service.BillingReservation, error) {
	if err := validateReservationInput(reservation); err != nil {
		return nil, err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, service.ErrBillingReservationBalanceUnavailable.WithCause(err)
	}
	defer func() { _ = tx.Rollback() }()

	stored, err := reserveBillingInTx(ctx, tx, reservation)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return stored, nil
}

// reserveBillingInTx deliberately remains in the repository package so the
// service contract never exposes database/sql transaction types.
func reserveBillingInTx(ctx context.Context, tx *sql.Tx, input *service.BillingReservation) (*service.BillingReservation, error) {
	expiresAt := normalizePostgresTime(input.ExpiresAt)
	var balanceRaw string
	err := tx.QueryRowContext(ctx, `
		SELECT balance::text
		FROM users
		WHERE id = $1 AND deleted_at IS NULL
		FOR UPDATE
	`, input.UserID).Scan(&balanceRaw)
	if err != nil {
		return nil, service.ErrBillingReservationBalanceUnavailable.WithCause(err)
	}
	balance, err := decimal.NewFromString(balanceRaw)
	if err != nil {
		return nil, service.ErrBillingReservationBalanceUnavailable.WithCause(err)
	}

	existing, err := getBillingReservationByKeyWith(ctx, tx, input.ReservationKey)
	if err == nil {
		if !sameReservationRequest(existing, input) {
			return nil, service.ErrBillingReservationConflict
		}
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrBillingReservationBalanceUnavailable.WithCause(err)
	}

	var activeRaw string
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(reserved_amount_usd), 0)::text
		FROM billing_reservations
		WHERE user_id = $1 AND status = $2
	`, input.UserID, service.BillingReservationStatusActive).Scan(&activeRaw); err != nil {
		return nil, service.ErrBillingReservationBalanceUnavailable.WithCause(err)
	}
	active, err := decimal.NewFromString(activeRaw)
	if err != nil {
		return nil, service.ErrBillingReservationBalanceUnavailable.WithCause(err)
	}
	available := balance.Sub(active)
	if available.Cmp(input.ReservedAmountUSD.Decimal()) < 0 {
		return nil, service.ErrBillingReservationInsufficientBalance
	}

	status := input.Status
	if status == "" {
		status = service.BillingReservationStatusActive
	}
	row := tx.QueryRowContext(ctx, `
		INSERT INTO billing_reservations (
			reservation_key, source_type, source_id, user_id, api_key_id,
			reserved_amount_usd, settled_amount_usd, status, expires_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, 0, $7, $8)
		ON CONFLICT (reservation_key) DO NOTHING
		RETURNING `+billingReservationColumns,
		input.ReservationKey,
		input.SourceType,
		input.SourceID,
		input.UserID,
		input.APIKeyID,
		input.ReservedAmountUSD.String(),
		status,
		expiresAt,
	)
	inserted, err := scanBillingReservation(row)
	if err == nil {
		return inserted, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	// A different user can race on the globally unique key without sharing the
	// same user row lock. ON CONFLICT waits for that insert and keeps this
	// transaction usable, allowing a deterministic replay/conflict decision.
	existing, err = getBillingReservationByKeyWith(ctx, tx, input.ReservationKey)
	if err != nil {
		return nil, err
	}
	if !sameReservationRequest(existing, input) {
		return nil, service.ErrBillingReservationConflict
	}
	return existing, nil
}

func (r *billingReservationRepository) GetByID(ctx context.Context, id int64) (*service.BillingReservation, error) {
	reservation, err := scanBillingReservation(r.db.QueryRowContext(ctx, `
		SELECT `+billingReservationColumns+`
		FROM billing_reservations
		WHERE id = $1
	`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrBillingReservationNotFound.WithCause(err)
	}
	return reservation, err
}

func (r *billingReservationRepository) GetByKey(ctx context.Context, reservationKey string) (*service.BillingReservation, error) {
	reservation, err := getBillingReservationByKeyWith(ctx, r.db, reservationKey)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrBillingReservationNotFound.WithCause(err)
	}
	return reservation, err
}

func (r *billingReservationRepository) ListExpired(ctx context.Context, now time.Time, limit int) ([]*service.BillingReservation, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+billingReservationColumns+`
		FROM billing_reservations
		WHERE status = $1 AND expires_at <= $2
		ORDER BY expires_at ASC, id ASC
		LIMIT $3
	`, service.BillingReservationStatusActive, now, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]*service.BillingReservation, 0, limit)
	for rows.Next() {
		item, err := scanBillingReservation(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

type expiredVideoReservationCandidate struct {
	reservationID    int64
	taskID           int64
	userID           int64
	apiKeyID         *int64
	reservedAmount   service.Money
	taskStatus       string
	dispatchState    string
	taskVersion      int64
	settlementStatus string
	balance          service.Money
}

func (r *billingReservationRepository) ReapExpiredVideoReservations(ctx context.Context, now time.Time, limit int) ([]service.BillingReservationReapResult, error) {
	return reapExpiredVideoReservations(ctx, r.db, now, limit)
}

func reapExpiredVideoReservations(ctx context.Context, db *sql.DB, now time.Time, limit int) ([]service.BillingReservationReapResult, error) {
	if limit <= 0 {
		limit = 100
	}
	now = normalizePostgresTime(now)
	results := make([]service.BillingReservationReapResult, 0, limit)
	for len(results) < limit {
		result, found, err := reapOneExpiredVideoReservation(ctx, db, now)
		if err != nil {
			return results, err
		}
		if !found {
			break
		}
		results = append(results, result)
	}
	return results, nil
}

func reapOneExpiredVideoReservation(ctx context.Context, db *sql.DB, now time.Time) (service.BillingReservationReapResult, bool, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return service.BillingReservationReapResult{}, false, err
	}
	defer func() { _ = tx.Rollback() }()

	candidate, err := scanExpiredVideoReservationCandidate(tx.QueryRowContext(ctx, `
		SELECT br.id, vt.id, br.user_id, br.api_key_id, br.reserved_amount_usd::text,
		       vt.status, vt.dispatch_state, vt.version, vt.settlement_status,
		       u.balance::text
		FROM billing_reservations br
		JOIN video_tasks vt ON vt.reservation_id = br.id
		JOIN users u ON u.id = br.user_id AND u.deleted_at IS NULL
		WHERE br.status = $1
		  AND br.expires_at <= $2
		  AND vt.settlement_status = $3
		ORDER BY br.expires_at ASC, br.id ASC
		LIMIT 1
		FOR UPDATE OF br, vt SKIP LOCKED
	`, service.BillingReservationStatusActive, now, service.VideoSettlementStatusPending))
	if errors.Is(err, sql.ErrNoRows) {
		return service.BillingReservationReapResult{}, false, nil
	}
	if err != nil {
		return service.BillingReservationReapResult{}, false, err
	}

	action := service.BillingReservationReapActionReviewRequired
	reservationStatus := service.BillingReservationStatusReviewRequired
	taskSettlementStatus := "error"
	transactionKind := "adjustment"
	transactionAmount := service.MustUSD("0")
	if candidate.taskStatus == service.VideoStatusQueued && candidate.dispatchState == service.VideoDispatchStatePending {
		action = service.BillingReservationReapActionReleased
		reservationStatus = service.BillingReservationStatusReleased
		taskSettlementStatus = "released"
		transactionKind = "release"
		transactionAmount = candidate.reservedAmount
	}

	var nextVersion int64
	if err := tx.QueryRowContext(ctx, `
		UPDATE video_tasks
		SET settlement_status = $3,
		    version = version + 1,
		    worker_claimed_at = NULL,
		    worker_claimed_until = NULL,
		    updated_at = NOW()
		WHERE id = $1 AND version = $2 AND settlement_status = 'pending'
		RETURNING version
	`, candidate.taskID, candidate.taskVersion, taskSettlementStatus).Scan(&nextVersion); err != nil {
		return service.BillingReservationReapResult{}, false, fmt.Errorf("claim expired reservation task version: %w", err)
	}

	var updatedReservationID int64
	if err := tx.QueryRowContext(ctx, `
		UPDATE billing_reservations
		SET status = $3::text,
		    released_at = CASE WHEN $3::text = 'released' THEN $2 ELSE released_at END,
		    updated_at = $2
		WHERE id = $1 AND status = 'active' AND expires_at <= $2
		RETURNING id
	`, candidate.reservationID, now, reservationStatus).Scan(&updatedReservationID); err != nil {
		return service.BillingReservationReapResult{}, false, fmt.Errorf("update expired reservation: %w", err)
	}

	transactionKey := fmt.Sprintf("video_task:%d:reservation_expired", candidate.taskID)
	metadata, err := json.Marshal(map[string]any{
		"action":              action,
		"reservation_id":      candidate.reservationID,
		"reserved_amount_usd": candidate.reservedAmount.String(),
	})
	if err != nil {
		return service.BillingReservationReapResult{}, false, err
	}
	if _, err := appendBillingTransactionInTx(ctx, tx, &service.BillingTransaction{
		TransactionKey:   transactionKey,
		SourceType:       "video_task",
		SourceID:         candidate.taskID,
		TransactionKind:  transactionKind,
		UserID:           candidate.userID,
		APIKeyID:         candidate.apiKeyID,
		ReservationID:    &candidate.reservationID,
		AmountOriginal:   transactionAmount,
		AmountUSD:        transactionAmount,
		ExchangeRate:     decimal.NewFromInt(1),
		ExchangeRateAsOf: now,
		PricingSource:    "reservation_reaper",
		PricingVersion:   "v1",
		BalanceBefore:    candidate.balance,
		BalanceAfter:     candidate.balance,
		Metadata:         metadata,
	}); err != nil {
		return service.BillingReservationReapResult{}, false, err
	}

	payload, err := json.Marshal(map[string]any{
		"task_id":        candidate.taskID,
		"reservation_id": candidate.reservationID,
		"action":         action,
	})
	if err != nil {
		return service.BillingReservationReapResult{}, false, err
	}
	if _, err := enqueueDomainOutboxInTx(ctx, tx, &service.DomainOutboxEvent{
		AggregateType: "video_task",
		AggregateID:   candidate.taskID,
		EventType:     "billing.reservation_expired",
		DedupKey:      transactionKey,
		Payload:       payload,
		Status:        service.DomainOutboxStatusPending,
		NextAttemptAt: now,
	}); err != nil {
		return service.BillingReservationReapResult{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return service.BillingReservationReapResult{}, false, err
	}
	_ = nextVersion
	return service.BillingReservationReapResult{
		ReservationID: candidate.reservationID,
		TaskID:        candidate.taskID,
		Action:        action,
	}, true, nil
}

func scanExpiredVideoReservationCandidate(scanner sqlRowScanner) (*expiredVideoReservationCandidate, error) {
	var (
		candidate                     expiredVideoReservationCandidate
		apiKeyID                      sql.NullInt64
		reservedAmountRaw, balanceRaw string
	)
	if err := scanner.Scan(
		&candidate.reservationID,
		&candidate.taskID,
		&candidate.userID,
		&apiKeyID,
		&reservedAmountRaw,
		&candidate.taskStatus,
		&candidate.dispatchState,
		&candidate.taskVersion,
		&candidate.settlementStatus,
		&balanceRaw,
	); err != nil {
		return nil, err
	}
	reservedAmount, err := service.NewMoney(reservedAmountRaw, service.CurrencyUSD)
	if err != nil {
		return nil, fmt.Errorf("decode expired reservation amount: %w", err)
	}
	balance, err := service.NewMoney(balanceRaw, service.CurrencyUSD)
	if err != nil {
		return nil, fmt.Errorf("decode reservation user balance: %w", err)
	}
	candidate.apiKeyID = nullableInt64(apiKeyID)
	candidate.reservedAmount = reservedAmount
	candidate.balance = balance
	return &candidate, nil
}

type sqlQueryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func getBillingReservationByKeyWith(ctx context.Context, queryer sqlQueryRower, key string) (*service.BillingReservation, error) {
	return scanBillingReservation(queryer.QueryRowContext(ctx, `
		SELECT `+billingReservationColumns+`
		FROM billing_reservations
		WHERE reservation_key = $1
	`, key))
}

func scanBillingReservation(scanner sqlRowScanner) (*service.BillingReservation, error) {
	var (
		item                    service.BillingReservation
		apiKeyID                sql.NullInt64
		reservedRaw, settledRaw string
		settledAt, releasedAt   sql.NullTime
	)
	if err := scanner.Scan(
		&item.ID,
		&item.ReservationKey,
		&item.SourceType,
		&item.SourceID,
		&item.UserID,
		&apiKeyID,
		&reservedRaw,
		&settledRaw,
		&item.Status,
		&item.ExpiresAt,
		&item.CreatedAt,
		&item.UpdatedAt,
		&settledAt,
		&releasedAt,
	); err != nil {
		return nil, err
	}

	reserved, err := service.NewMoney(reservedRaw, service.CurrencyUSD)
	if err != nil {
		return nil, fmt.Errorf("decode reserved amount: %w", err)
	}
	settled, err := service.NewMoney(settledRaw, service.CurrencyUSD)
	if err != nil {
		return nil, fmt.Errorf("decode settled amount: %w", err)
	}
	item.ReservedAmountUSD = reserved
	item.SettledAmountUSD = settled
	item.APIKeyID = nullableInt64(apiKeyID)
	item.SettledAt = nullableTime(settledAt)
	item.ReleasedAt = nullableTime(releasedAt)
	return &item, nil
}

func validateReservationInput(input *service.BillingReservation) error {
	if input == nil {
		return fmt.Errorf("billing reservation is required")
	}
	if strings.TrimSpace(input.ReservationKey) == "" {
		return fmt.Errorf("reservation key is required")
	}
	if input.UserID <= 0 {
		return service.ErrBillingReservationBalanceUnavailable
	}
	if input.ReservedAmountUSD.Currency() != service.CurrencyUSD {
		return fmt.Errorf("reserved amount must be USD")
	}
	if input.ExpiresAt.IsZero() {
		return fmt.Errorf("reservation expiry is required")
	}
	if input.Status != "" && input.Status != service.BillingReservationStatusActive {
		return fmt.Errorf("new reservation status must be active")
	}
	return nil
}

func sameReservationRequest(stored, input *service.BillingReservation) bool {
	if stored == nil || input == nil {
		return false
	}
	return stored.ReservationKey == input.ReservationKey &&
		stored.SourceType == input.SourceType &&
		stored.SourceID == input.SourceID &&
		stored.UserID == input.UserID &&
		sameOptionalInt64(stored.APIKeyID, input.APIKeyID) &&
		stored.ReservedAmountUSD.Decimal().Equal(input.ReservedAmountUSD.Decimal()) &&
		stored.ExpiresAt.Equal(normalizePostgresTime(input.ExpiresAt))
}

func normalizePostgresTime(value time.Time) time.Time {
	return value.UTC().Truncate(time.Microsecond)
}

func sameOptionalInt64(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func nullableInt64(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}

func nullableTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}
