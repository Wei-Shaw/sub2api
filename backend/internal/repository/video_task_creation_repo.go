package repository

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

const videoTaskReservationKeyPrefix = "video_task:create:"

type videoTaskCreationRepository struct {
	db *sql.DB
}

func NewVideoTaskCreationRepository(db *sql.DB) service.VideoTaskCreationRepository {
	return &videoTaskCreationRepository{db: db}
}

func (r *videoTaskCreationRepository) ReplayExisting(ctx context.Context, input service.VideoTaskCreationReplayInput) (*service.VideoTaskCreationResult, bool, error) {
	if !isSHA256Hex(input.CreationKey) || !isSHA256Hex(input.CreationFingerprint) {
		return nil, false, service.ErrIdempotencyKeyInvalid
	}
	existing, err := findVideoTaskByCreationKey(ctx, r.db, input.CreationKey)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, service.ErrBillingReservationBalanceUnavailable.WithCause(err)
	}
	if !sameVideoTaskCreationIdentity(existing, input.CreationFingerprint, input.CreatedBy, input.APIKeyID) {
		return nil, false, service.ErrIdempotencyKeyConflict
	}
	if existing.ReservationID == nil {
		if existing.Provider == service.VideoProviderMock {
			return &service.VideoTaskCreationResult{Task: existing, Replayed: true}, true, nil
		}
		return nil, false, service.ErrBillingReservationBalanceUnavailable.WithCause(errors.New("idempotent video task has no reservation"))
	}
	reservation, err := scanBillingReservation(r.db.QueryRowContext(ctx, `
		SELECT `+billingReservationColumns+`
		FROM billing_reservations
		WHERE id = $1
	`, *existing.ReservationID))
	if err != nil {
		return nil, false, service.ErrBillingReservationBalanceUnavailable.WithCause(err)
	}
	return &service.VideoTaskCreationResult{Task: existing, Reservation: reservation, Replayed: true}, true, nil
}

func (r *videoTaskCreationRepository) CreateWithReservation(ctx context.Context, input service.VideoTaskCreationInput) (*service.VideoTaskCreationResult, error) {
	if err := validateVideoTaskCreationInput(input); err != nil {
		return nil, err
	}
	if err := service.ApplyVideoPricingSnapshotToTask(input.Task, input.PricingSnapshot); err != nil {
		return nil, fmt.Errorf("apply video creation pricing snapshot: %w", err)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, service.ErrBillingReservationBalanceUnavailable.WithCause(err)
	}
	defer func() { _ = tx.Rollback() }()

	// Serialize all availability checks for a user before looking up the
	// creation key. A concurrent same-user replay waits here, then observes the
	// committed task and returns it rather than making another reservation.
	var lockedUserID int64
	if err := tx.QueryRowContext(ctx, `
		SELECT id
		FROM users
		WHERE id = $1 AND deleted_at IS NULL
		FOR UPDATE
	`, input.Task.CreatedBy).Scan(&lockedUserID); err != nil {
		return nil, service.ErrBillingReservationBalanceUnavailable.WithCause(err)
	}

	existing, err := findVideoTaskByCreationKey(ctx, tx, input.Task.CreationKey)
	if err == nil {
		return replayVideoTaskCreation(ctx, tx, existing, input)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrBillingReservationBalanceUnavailable.WithCause(err)
	}

	var dailyTrialReservationID int64
	if input.DailyTrialProvider != "" {
		err := tx.QueryRowContext(ctx, `
			INSERT INTO video_daily_trial_reservations (provider, created_by, trial_date)
			VALUES ($1, $2, $3)
			ON CONFLICT (provider, created_by, trial_date) DO NOTHING
			RETURNING id
		`, input.DailyTrialProvider, input.Task.CreatedBy, input.DailyTrialDate).Scan(&dailyTrialReservationID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrVideoTrialLimitExceeded
		}
		if err != nil {
			return nil, err
		}
	}

	reservation, err := reserveBillingInTx(ctx, tx, &service.BillingReservation{
		ReservationKey:    videoTaskReservationKeyPrefix + input.Task.CreationKey,
		SourceType:        "video_task",
		SourceID:          0,
		UserID:            input.Task.CreatedBy,
		APIKeyID:          input.Task.APIKeyID,
		ReservedAmountUSD: input.ReservedAmountUSD,
		Status:            service.BillingReservationStatusActive,
		ExpiresAt:         input.ReservationExpiresAt,
	})
	if err != nil {
		if errors.Is(err, service.ErrBillingReservationConflict) {
			return nil, service.ErrIdempotencyKeyConflict
		}
		return nil, err
	}

	input.Task.ReservationID = &reservation.ID
	if err := insertVideoTaskWith(ctx, tx, input.Task); err != nil {
		return nil, err
	}
	if dailyTrialReservationID > 0 {
		result, err := tx.ExecContext(ctx, `
			UPDATE video_daily_trial_reservations
			SET video_task_id = $2
			WHERE id = $1 AND video_task_id IS NULL
		`, dailyTrialReservationID, input.Task.ID)
		if err != nil {
			return nil, err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return nil, err
		}
		if rows != 1 {
			return nil, fmt.Errorf("backfill daily trial task: expected 1 row, got %d", rows)
		}
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE billing_reservations
		SET source_id = $2, updated_at = NOW()
		WHERE id = $1 AND source_id = 0 AND status = $3
	`, reservation.ID, input.Task.ID, service.BillingReservationStatusActive)
	if err != nil {
		return nil, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rows != 1 {
		return nil, fmt.Errorf("backfill video reservation source: expected 1 row, got %d", rows)
	}
	reservation.SourceID = input.Task.ID

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &service.VideoTaskCreationResult{Task: input.Task, Reservation: reservation}, nil
}

func replayVideoTaskCreation(ctx context.Context, tx *sql.Tx, existing *service.VideoTask, input service.VideoTaskCreationInput) (*service.VideoTaskCreationResult, error) {
	if !sameVideoTaskCreationIdentity(existing, input.Task.CreationFingerprint, input.Task.CreatedBy, input.Task.APIKeyID) {
		return nil, service.ErrIdempotencyKeyConflict
	}
	if existing.ReservationID == nil {
		return nil, service.ErrBillingReservationBalanceUnavailable.WithCause(errors.New("idempotent video task has no reservation"))
	}
	reservation, err := scanBillingReservation(tx.QueryRowContext(ctx, `
		SELECT `+billingReservationColumns+`
		FROM billing_reservations
		WHERE id = $1
	`, *existing.ReservationID))
	if err != nil {
		return nil, service.ErrBillingReservationBalanceUnavailable.WithCause(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &service.VideoTaskCreationResult{Task: existing, Reservation: reservation, Replayed: true}, nil
}

func findVideoTaskByCreationKey(ctx context.Context, queryer sqlQueryRower, creationKey string) (*service.VideoTask, error) {
	query := "SELECT" + videoTaskSelectColumnsWithReliability + videoTaskJoinSQL + " WHERE vt.creation_key = $1"
	return scanVideoTaskWithReliability(queryer.QueryRowContext(ctx, query, creationKey))
}

func sameVideoTaskCreationIdentity(task *service.VideoTask, fingerprint string, createdBy int64, apiKeyID *int64) bool {
	return task != nil && task.CreationFingerprint == fingerprint && task.CreatedBy == createdBy && sameOptionalInt64(task.APIKeyID, apiKeyID)
}

func validateVideoTaskCreationInput(input service.VideoTaskCreationInput) error {
	if input.Task == nil {
		return fmt.Errorf("video task is required")
	}
	if input.Task.CreatedBy <= 0 {
		return service.ErrBillingReservationBalanceUnavailable
	}
	if !isSHA256Hex(input.Task.CreationKey) {
		return service.ErrIdempotencyKeyInvalid
	}
	if !isSHA256Hex(input.Task.CreationFingerprint) {
		return service.ErrIdempotencyInvalidPayload
	}
	if input.ReservedAmountUSD.Currency() != service.CurrencyUSD {
		return fmt.Errorf("video reservation amount must be USD")
	}
	if input.ReservationExpiresAt.IsZero() {
		return fmt.Errorf("video reservation expiry is required")
	}
	if (input.DailyTrialProvider == "") != input.DailyTrialDate.IsZero() {
		return fmt.Errorf("daily trial provider and date must be provided together")
	}
	return nil
}

func isSHA256Hex(value string) bool {
	if len(value) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}

var _ service.VideoTaskCreationRepository = (*videoTaskCreationRepository)(nil)
var _ service.VideoTaskCreationReplayRepository = (*videoTaskCreationRepository)(nil)
