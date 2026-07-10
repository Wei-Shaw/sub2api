package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

const (
	videoFinalizationOutboxCapture = "video.capture_content"
	videoFinalizationOutboxArchive = "video.archive_asset"
	videoFinalizationOutboxCache   = "billing.invalidate_cache"
	videoFinalizationOutboxLow     = "billing.notify_low_balance"
	videoFinalizationOutboxOverrun = "billing.notify_reservation_overrun"
	videoFinalizationOutboxReview  = "billing.reservation_review_required"
	videoFinalizationTaskColumns   = `provider, model, duration, created_by, api_key_id, reservation_id,
		status, version, settlement_status, balance_charged_at,
		worker_claimed_at, worker_claimed_until, archive_status, capture_status, completed_at,
		result_url, error_message, cost_estimate, poll_count, usage_total_tokens,
		actual_resolution, actual_duration, last_frame_url`
)

var _ service.VideoTaskFinalizationRepository = (*videoGatewayRepository)(nil)

type videoFinalizationTaskRow struct {
	provider           string
	model              string
	duration           int
	userID             int64
	apiKeyID           *int64
	reservationID      *int64
	status             string
	version            int64
	settlementStatus   string
	balanceChargedAt   *time.Time
	workerClaimedAt    *time.Time
	workerClaimedUntil *time.Time
	archiveStatus      string
	captureStatus      string
	completedAt        *time.Time
	resultURL          string
	errorMessage       string
	costEstimate       float64
	pollCount          int
	usageTotalTokens   *int64
	actualResolution   string
	actualDuration     *int
	lastFrameURL       string
}

type videoFinalizationReservation struct {
	id          int64
	userID      int64
	apiKeyID    *int64
	reservedUSD decimal.Decimal
	status      string
}

func (r *videoGatewayRepository) FinalizeVideoTask(ctx context.Context, input service.VideoTaskFinalizationInput) (service.VideoTaskFinalizationResult, error) {
	if r == nil || r.db == nil {
		return service.VideoTaskFinalizationResult{}, fmt.Errorf("video finalization database is required")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return service.VideoTaskFinalizationResult{}, err
	}
	defer func() { _ = tx.Rollback() }()

	task, applied, err := applyVideoTaskTerminalCAS(ctx, tx, input)
	if err != nil {
		return service.VideoTaskFinalizationResult{}, err
	}
	if !applied {
		return classifyVideoTaskFinalizationReplay(task, input)
	}

	event := &service.VideoTaskEvent{
		VideoTaskID: input.TaskID,
		EventType:   input.TerminalStatus,
		Message:     videoTaskTerminalMessage(input.TerminalStatus),
		Payload:     input.ProviderPayload,
	}
	if err := addVideoTaskEventWith(ctx, tx, event); err != nil {
		return service.VideoTaskFinalizationResult{}, fmt.Errorf("insert terminal video task event: %w", err)
	}
	if err := insertVideoFinalizationUsage(ctx, tx, task, input); err != nil {
		return service.VideoTaskFinalizationResult{}, fmt.Errorf("insert terminal video usage: %w", err)
	}

	transactionID, overrun, chargeApplied, reviewRequired, err := settleVideoFinalizationBilling(ctx, tx, task, input)
	if err != nil {
		return service.VideoTaskFinalizationResult{}, err
	}
	if err := enqueueVideoFinalizationOutbox(ctx, tx, task, input, chargeApplied, overrun, reviewRequired); err != nil {
		return service.VideoTaskFinalizationResult{}, fmt.Errorf("enqueue terminal video outbox: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return service.VideoTaskFinalizationResult{}, err
	}

	result := videoTaskFinalizationResultFromRow(task)
	result.Applied = true
	result.ReservationOverrun = overrun
	if transactionID > 0 {
		result.TransactionID = &transactionID
	}
	return result, nil
}

func applyVideoTaskTerminalCAS(ctx context.Context, tx *sql.Tx, input service.VideoTaskFinalizationInput) (videoFinalizationTaskRow, bool, error) {
	originalCost := input.PricingSnapshot.AmountOriginal.String()
	if input.TerminalStatus != service.VideoStatusSucceeded {
		originalCost = service.MustUSD("0").String()
	}
	row, err := scanVideoFinalizationTaskRow(tx.QueryRowContext(ctx, `
		UPDATE video_tasks
		SET status = $3,
			result_url = $4,
			error_message = $5,
			cost_estimate = $6::numeric,
			completed_at = $7::timestamptz,
			poll_count = $8,
			usage_total_tokens = $9,
			actual_resolution = $10,
			actual_duration = $11,
			last_frame_url = $12,
			settlement_status = CASE
				WHEN provider = 'mock' THEN 'not_required'
				WHEN $3 = 'succeeded' AND reservation_id IS NOT NULL THEN 'settled'
				WHEN $3 IN ('failed', 'cancelled') AND reservation_id IS NOT NULL THEN 'released'
				ELSE 'not_required'
			END,
			archive_status = CASE WHEN $3 = 'succeeded' THEN 'pending' ELSE 'not_required' END,
			capture_status = CASE WHEN $3 = 'succeeded' THEN 'pending' ELSE 'not_required' END,
			balance_charged_at = CASE
				WHEN $3 = 'succeeded' AND provider <> 'mock' AND reservation_id IS NOT NULL THEN $7::timestamptz
				ELSE NULL
			END,
			version = version + 1,
			worker_claimed_at = NULL,
			worker_claimed_until = NULL,
			updated_at = NOW()
		WHERE id = $1
		  AND version = $2
		  AND status NOT IN ('succeeded', 'failed', 'cancelled')
		RETURNING `+videoFinalizationTaskColumns,
		input.TaskID,
		input.ExpectedVersion,
		input.TerminalStatus,
		strings.TrimSpace(input.ProviderResultURL),
		strings.TrimSpace(input.ProviderErrorMessage),
		originalCost,
		input.CompletedAt,
		input.PollCount,
		nullableInt64Ptr(input.ActualTokens),
		nullableNonEmptyString(input.ActualResolution),
		nullableVideoIntPtr(input.ActualDuration),
		nullableNonEmptyString(input.LastFrameURL),
	))
	if errors.Is(err, sql.ErrNoRows) {
		current, readErr := readVideoFinalizationCurrentTask(ctx, tx, input.TaskID)
		if readErr != nil {
			return videoFinalizationTaskRow{}, false, readErr
		}
		return current, false, nil
	}
	if err != nil {
		return videoFinalizationTaskRow{}, false, fmt.Errorf("terminal video task CAS: %w", err)
	}
	return row, true, nil
}

func readVideoFinalizationCurrentTask(ctx context.Context, tx *sql.Tx, taskID int64) (videoFinalizationTaskRow, error) {
	row, err := scanVideoFinalizationTaskRow(tx.QueryRowContext(ctx, `
		SELECT `+videoFinalizationTaskColumns+`
		FROM video_tasks
		WHERE id = $1
		FOR UPDATE
	`, taskID))
	if errors.Is(err, sql.ErrNoRows) {
		return videoFinalizationTaskRow{}, service.ErrVideoTaskNotFound
	}
	if err != nil {
		return videoFinalizationTaskRow{}, err
	}
	return row, nil
}

func scanVideoFinalizationTaskRow(scanner sqlRowScanner) (videoFinalizationTaskRow, error) {
	row := videoFinalizationTaskRow{}
	var apiKeyID, reservationID, usageTotalTokens, actualDuration sql.NullInt64
	var actualResolution, lastFrameURL sql.NullString
	var balanceChargedAt, workerClaimedAt, workerClaimedUntil, completedAt sql.NullTime
	err := scanner.Scan(
		&row.provider,
		&row.model,
		&row.duration,
		&row.userID,
		&apiKeyID,
		&reservationID,
		&row.status,
		&row.version,
		&row.settlementStatus,
		&balanceChargedAt,
		&workerClaimedAt,
		&workerClaimedUntil,
		&row.archiveStatus,
		&row.captureStatus,
		&completedAt,
		&row.resultURL,
		&row.errorMessage,
		&row.costEstimate,
		&row.pollCount,
		&usageTotalTokens,
		&actualResolution,
		&actualDuration,
		&lastFrameURL,
	)
	if err != nil {
		return videoFinalizationTaskRow{}, err
	}
	row.apiKeyID = int64PtrFromNull(apiKeyID)
	row.reservationID = int64PtrFromNull(reservationID)
	row.balanceChargedAt = timePtrFromNull(balanceChargedAt)
	row.workerClaimedAt = timePtrFromNull(workerClaimedAt)
	row.workerClaimedUntil = timePtrFromNull(workerClaimedUntil)
	row.completedAt = timePtrFromNull(completedAt)
	row.usageTotalTokens = int64PtrFromNull(usageTotalTokens)
	row.actualResolution = actualResolution.String
	if actualDuration.Valid {
		value := int(actualDuration.Int64)
		row.actualDuration = &value
	}
	row.lastFrameURL = lastFrameURL.String
	return row, nil
}

func timePtrFromNull(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}

func videoTaskFinalizationResultFromRow(row videoFinalizationTaskRow) service.VideoTaskFinalizationResult {
	return service.VideoTaskFinalizationResult{
		Status:             row.status,
		Version:            row.version,
		SettlementStatus:   row.settlementStatus,
		BalanceChargedAt:   row.balanceChargedAt,
		WorkerClaimedAt:    row.workerClaimedAt,
		WorkerClaimedUntil: row.workerClaimedUntil,
		ArchiveStatus:      row.archiveStatus,
		CaptureStatus:      row.captureStatus,
		CompletedAt:        row.completedAt,
		ResultURL:          row.resultURL,
		ErrorMessage:       row.errorMessage,
		CostEstimate:       row.costEstimate,
		PollCount:          row.pollCount,
		UsageTotalTokens:   row.usageTotalTokens,
		ActualResolution:   row.actualResolution,
		ActualDuration:     row.actualDuration,
		LastFrameURL:       row.lastFrameURL,
	}
}

func videoTaskPollUpdateResultFromRow(row videoFinalizationTaskRow) service.VideoTaskPollUpdateResult {
	return service.VideoTaskPollUpdateResult{
		Status:             row.status,
		Version:            row.version,
		ResultURL:          row.resultURL,
		ErrorMessage:       row.errorMessage,
		CostEstimate:       row.costEstimate,
		PollCount:          row.pollCount,
		UsageTotalTokens:   row.usageTotalTokens,
		ActualResolution:   row.actualResolution,
		ActualDuration:     row.actualDuration,
		LastFrameURL:       row.lastFrameURL,
		SettlementStatus:   row.settlementStatus,
		BalanceChargedAt:   row.balanceChargedAt,
		WorkerClaimedAt:    row.workerClaimedAt,
		WorkerClaimedUntil: row.workerClaimedUntil,
		ArchiveStatus:      row.archiveStatus,
		CaptureStatus:      row.captureStatus,
		CompletedAt:        row.completedAt,
	}
}

func classifyVideoTaskFinalizationReplay(current videoFinalizationTaskRow, input service.VideoTaskFinalizationInput) (service.VideoTaskFinalizationResult, error) {
	if current.status == input.TerminalStatus && service.IsTerminalVideoStatus(current.status) {
		result := videoTaskFinalizationResultFromRow(current)
		result.Idempotent = true
		return result, nil
	}
	return service.VideoTaskFinalizationResult{}, &service.VideoTaskTerminalConflictError{
		TaskID:          input.TaskID,
		RequestedStatus: input.TerminalStatus,
		CurrentStatus:   current.status,
		CurrentVersion:  current.version,
	}
}

func insertVideoFinalizationUsage(ctx context.Context, tx *sql.Tx, task videoFinalizationTaskRow, input service.VideoTaskFinalizationInput) error {
	duration := task.duration
	if input.ActualDuration != nil {
		duration = *input.ActualDuration
	}
	originalCost := input.PricingSnapshot.AmountOriginal.String()
	currency := service.NormalizeBillingCurrency(string(input.PricingSnapshot.AmountOriginal.Currency()))
	if input.TerminalStatus != service.VideoStatusSucceeded {
		originalCost = service.MustUSD("0").String()
		currency = string(service.CurrencyUSD)
	}
	pricingSource := service.NormalizeBillingPricingSource(input.PricingSnapshot.PricingSource)
	pricingVersion := service.NormalizeBillingPricingVersion(input.PricingSnapshot.PricingVersion)
	var pricingVersionArg any
	if pricingVersion != "" {
		pricingVersionArg = pricingVersion
	}
	_, err := tx.ExecContext(ctx, insertVideoUsageLogSQL,
		input.TaskID,
		task.provider,
		task.model,
		input.TerminalStatus,
		originalCost,
		duration,
		currency,
		pricingSource,
		pricingVersionArg,
	)
	return err
}

func settleVideoFinalizationBilling(
	ctx context.Context,
	tx *sql.Tx,
	task videoFinalizationTaskRow,
	input service.VideoTaskFinalizationInput,
) (transactionID int64, overrun bool, chargeApplied bool, reviewRequired bool, err error) {
	if task.reservationID == nil {
		if input.TerminalStatus == service.VideoStatusSucceeded && task.provider != service.VideoProviderMock {
			return 0, false, false, false, fmt.Errorf("succeeded billable video task %d has no reservation", input.TaskID)
		}
		return 0, false, false, false, nil
	}

	reservation, err := lockVideoFinalizationReservation(ctx, tx, *task.reservationID)
	if err != nil {
		return 0, false, false, false, err
	}
	if task.provider == service.VideoProviderMock {
		if reservation.status == service.BillingReservationStatusActive {
			if err := markMockVideoReservationForReview(ctx, tx, reservation.id); err != nil {
				return 0, false, false, false, err
			}
		} else if reservation.status != service.BillingReservationStatusReviewRequired {
			return 0, false, false, false, fmt.Errorf("mock video task reservation has financial state %q", reservation.status)
		}
		return 0, false, false, true, nil
	}
	if reservation.userID != task.userID {
		return 0, false, false, false, fmt.Errorf("video task reservation user mismatch")
	}
	if reservation.status != service.BillingReservationStatusActive {
		return 0, false, false, false, fmt.Errorf("video task reservation is not active")
	}

	balanceBefore, err := lockVideoFinalizationUserBalance(ctx, tx, reservation.userID)
	if err != nil {
		return 0, false, false, false, err
	}
	if input.TerminalStatus != service.VideoStatusSucceeded {
		if err := releaseVideoFinalizationReservation(ctx, tx, reservation, input.CompletedAt); err != nil {
			return 0, false, false, false, err
		}
		transactionID, err = insertVideoFinalizationReleaseLedger(ctx, tx, reservation, input, balanceBefore)
		return transactionID, false, false, false, err
	}

	overrun = input.ActualCostUSD.Decimal().Cmp(reservation.reservedUSD) > 0
	if err := settleVideoFinalizationReservation(ctx, tx, reservation, input.ActualCostUSD, input.CompletedAt); err != nil {
		return 0, false, false, false, err
	}
	balanceAfter := balanceBefore.Sub(input.ActualCostUSD.Decimal()).Round(8)
	transactionID, err = insertVideoFinalizationChargeLedger(ctx, tx, reservation, input, balanceBefore, balanceAfter, overrun)
	if err != nil {
		return 0, false, false, false, err
	}
	var storedBalance string
	err = tx.QueryRowContext(ctx, `
		UPDATE users
		SET balance = ROUND(balance - $2::numeric, 8),
			updated_at = NOW()
		WHERE id = $1
		RETURNING balance::text
	`, reservation.userID, input.ActualCostUSD.String()).Scan(&storedBalance)
	if err != nil {
		return 0, false, false, false, fmt.Errorf("update video finalization balance: %w", err)
	}
	stored, err := decimal.NewFromString(storedBalance)
	if err != nil || !stored.Equal(balanceAfter) {
		return 0, false, false, false, fmt.Errorf("video finalization balance projection mismatch")
	}
	return transactionID, overrun, true, false, nil
}

func markMockVideoReservationForReview(ctx context.Context, tx *sql.Tx, reservationID int64) error {
	result, err := tx.ExecContext(ctx, `
		UPDATE billing_reservations
		SET status = $2,
			updated_at = NOW()
		WHERE id = $1 AND status = $3
	`, reservationID, service.BillingReservationStatusReviewRequired, service.BillingReservationStatusActive)
	return requireOneVideoFinalizationRow(result, err, "mark mock video reservation for review")
}

func lockVideoFinalizationReservation(ctx context.Context, tx *sql.Tx, reservationID int64) (videoFinalizationReservation, error) {
	var apiKeyID sql.NullInt64
	var reservedRaw string
	item := videoFinalizationReservation{}
	err := tx.QueryRowContext(ctx, `
		SELECT id, user_id, api_key_id, reserved_amount_usd::text, status
		FROM billing_reservations
		WHERE id = $1
		FOR UPDATE
	`, reservationID).Scan(&item.id, &item.userID, &apiKeyID, &reservedRaw, &item.status)
	if errors.Is(err, sql.ErrNoRows) {
		return videoFinalizationReservation{}, service.ErrBillingReservationNotFound
	}
	if err != nil {
		return videoFinalizationReservation{}, fmt.Errorf("lock video task reservation: %w", err)
	}
	item.reservedUSD, err = decimal.NewFromString(reservedRaw)
	if err != nil {
		return videoFinalizationReservation{}, fmt.Errorf("parse video task reservation amount: %w", err)
	}
	if apiKeyID.Valid {
		item.apiKeyID = &apiKeyID.Int64
	}
	return item, nil
}

func lockVideoFinalizationUserBalance(ctx context.Context, tx *sql.Tx, userID int64) (decimal.Decimal, error) {
	var raw string
	if err := tx.QueryRowContext(ctx, "SELECT balance::text FROM users WHERE id = $1 FOR UPDATE", userID).Scan(&raw); err != nil {
		return decimal.Zero, fmt.Errorf("lock video finalization user balance: %w", err)
	}
	balance, err := decimal.NewFromString(raw)
	if err != nil {
		return decimal.Zero, fmt.Errorf("parse video finalization user balance: %w", err)
	}
	return balance, nil
}

func settleVideoFinalizationReservation(ctx context.Context, tx *sql.Tx, reservation videoFinalizationReservation, actual service.Money, completedAt time.Time) error {
	result, err := tx.ExecContext(ctx, `
		UPDATE billing_reservations
		SET status = $2,
			settled_amount_usd = $3::numeric,
			settled_at = $4,
			released_at = NULL,
			updated_at = NOW()
		WHERE id = $1 AND status = $5
	`, reservation.id, service.BillingReservationStatusSettled, actual.String(), completedAt, service.BillingReservationStatusActive)
	return requireOneVideoFinalizationRow(result, err, "settle video task reservation")
}

func releaseVideoFinalizationReservation(ctx context.Context, tx *sql.Tx, reservation videoFinalizationReservation, completedAt time.Time) error {
	result, err := tx.ExecContext(ctx, `
		UPDATE billing_reservations
		SET status = $2,
			settled_amount_usd = 0,
			released_at = $3,
			settled_at = NULL,
			updated_at = NOW()
		WHERE id = $1 AND status = $4
	`, reservation.id, service.BillingReservationStatusReleased, completedAt, service.BillingReservationStatusActive)
	return requireOneVideoFinalizationRow(result, err, "release video task reservation")
}

func requireOneVideoFinalizationRow(result sql.Result, err error, operation string) error {
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s rows affected: %w", operation, err)
	}
	if rows != 1 {
		return fmt.Errorf("%s affected %d rows", operation, rows)
	}
	return nil
}

func insertVideoFinalizationChargeLedger(
	ctx context.Context,
	tx *sql.Tx,
	reservation videoFinalizationReservation,
	input service.VideoTaskFinalizationInput,
	balanceBefore decimal.Decimal,
	balanceAfter decimal.Decimal,
	overrun bool,
) (int64, error) {
	metadata, err := json.Marshal(map[string]any{
		"actual_amount_usd":   input.ActualCostUSD.String(),
		"reserved_amount_usd": reservation.reservedUSD.StringFixed(10),
		"reservation_overrun": overrun,
	})
	if err != nil {
		return 0, err
	}
	exchangeRate, err := decimal.NewFromString(input.PricingSnapshot.ExchangeRate)
	if err != nil {
		return 0, err
	}
	return insertVideoFinalizationLedger(ctx, tx, videoFinalizationLedgerInput{
		transactionKey:   fmt.Sprintf("video_task:%d:charge", input.TaskID),
		taskID:           input.TaskID,
		kind:             "charge",
		reservation:      reservation,
		amountOriginal:   input.PricingSnapshot.AmountOriginal.String(),
		currencyOriginal: string(input.PricingSnapshot.AmountOriginal.Currency()),
		amountUSD:        input.ActualCostUSD.String(),
		exchangeRate:     exchangeRate.StringFixed(10),
		exchangeRateAt:   input.CompletedAt,
		pricingSource:    service.NormalizeBillingPricingSource(input.PricingSnapshot.PricingSource),
		pricingVersion:   service.NormalizeBillingPricingVersion(input.PricingSnapshot.PricingVersion),
		balanceBefore:    balanceBefore.StringFixed(10),
		balanceAfter:     balanceAfter.StringFixed(10),
		metadata:         metadata,
	})
}

func insertVideoFinalizationReleaseLedger(
	ctx context.Context,
	tx *sql.Tx,
	reservation videoFinalizationReservation,
	input service.VideoTaskFinalizationInput,
	balance decimal.Decimal,
) (int64, error) {
	metadata, err := json.Marshal(map[string]any{
		"released_amount_usd": reservation.reservedUSD.StringFixed(10),
		"terminal_status":     input.TerminalStatus,
	})
	if err != nil {
		return 0, err
	}
	return insertVideoFinalizationLedger(ctx, tx, videoFinalizationLedgerInput{
		transactionKey:   fmt.Sprintf("video_task:%d:release", input.TaskID),
		taskID:           input.TaskID,
		kind:             "release",
		reservation:      reservation,
		amountOriginal:   reservation.reservedUSD.StringFixed(10),
		currencyOriginal: string(service.CurrencyUSD),
		amountUSD:        reservation.reservedUSD.StringFixed(10),
		exchangeRate:     "1.0000000000",
		exchangeRateAt:   input.CompletedAt,
		pricingSource:    "reservation_release",
		pricingVersion:   "reliability-core-v1",
		balanceBefore:    balance.StringFixed(10),
		balanceAfter:     balance.StringFixed(10),
		metadata:         metadata,
	})
}

type videoFinalizationLedgerInput struct {
	transactionKey   string
	taskID           int64
	kind             string
	reservation      videoFinalizationReservation
	amountOriginal   string
	currencyOriginal string
	amountUSD        string
	exchangeRate     string
	exchangeRateAt   time.Time
	pricingSource    string
	pricingVersion   string
	balanceBefore    string
	balanceAfter     string
	metadata         []byte
}

func insertVideoFinalizationLedger(ctx context.Context, tx *sql.Tx, input videoFinalizationLedgerInput) (int64, error) {
	var transactionID int64
	err := tx.QueryRowContext(ctx, `
		INSERT INTO billing_transactions (
			transaction_key, source_type, source_id, transaction_kind,
			user_id, api_key_id, reservation_id,
			amount_original, currency_original, amount_usd,
			exchange_rate, exchange_rate_as_of, pricing_source, pricing_version,
			balance_before, balance_after, metadata
		)
		VALUES ($1, 'video_task', $2, $3, $4, $5, $6,
			$7::numeric, $8, $9::numeric, $10::numeric, $11, $12, $13,
			$14::numeric, $15::numeric, $16::jsonb)
		RETURNING id
	`,
		input.transactionKey,
		input.taskID,
		input.kind,
		input.reservation.userID,
		input.reservation.apiKeyID,
		input.reservation.id,
		input.amountOriginal,
		input.currencyOriginal,
		input.amountUSD,
		input.exchangeRate,
		input.exchangeRateAt,
		input.pricingSource,
		input.pricingVersion,
		input.balanceBefore,
		input.balanceAfter,
		string(input.metadata),
	).Scan(&transactionID)
	if err != nil {
		return 0, fmt.Errorf("insert immutable video billing transaction: %w", err)
	}
	return transactionID, nil
}

func enqueueVideoFinalizationOutbox(
	ctx context.Context,
	tx *sql.Tx,
	task videoFinalizationTaskRow,
	input service.VideoTaskFinalizationInput,
	chargeApplied bool,
	overrun bool,
	reviewRequired bool,
) error {
	if input.TerminalStatus != service.VideoStatusSucceeded && !reviewRequired {
		return nil
	}
	type outboxSpec struct {
		eventType string
		suffix    string
		payload   map[string]any
	}
	specs := make([]outboxSpec, 0, 5)
	if input.TerminalStatus == service.VideoStatusSucceeded {
		specs = append(specs,
			outboxSpec{eventType: videoFinalizationOutboxCapture, suffix: "capture", payload: map[string]any{"task_id": input.TaskID}},
			outboxSpec{eventType: videoFinalizationOutboxArchive, suffix: "archive", payload: map[string]any{"task_id": input.TaskID}},
		)
	}
	if chargeApplied {
		specs = append(specs,
			outboxSpec{eventType: videoFinalizationOutboxCache, suffix: "billing_cache", payload: map[string]any{"task_id": input.TaskID, "user_id": task.userID}},
			outboxSpec{eventType: videoFinalizationOutboxLow, suffix: "low_balance", payload: map[string]any{"task_id": input.TaskID, "user_id": task.userID}},
		)
	}
	if overrun {
		specs = append(specs, outboxSpec{
			eventType: videoFinalizationOutboxOverrun,
			suffix:    "reservation_overrun",
			payload: map[string]any{
				"task_id":             input.TaskID,
				"user_id":             task.userID,
				"reservation_overrun": true,
			},
		})
	}
	if reviewRequired {
		specs = append(specs, outboxSpec{
			eventType: videoFinalizationOutboxReview,
			suffix:    "reservation_review_required",
			payload: map[string]any{
				"task_id":        input.TaskID,
				"user_id":        task.userID,
				"reservation_id": task.reservationID,
				"reason":         "mock_task_has_reservation",
			},
		})
	}
	for _, spec := range specs {
		payload, err := json.Marshal(spec.payload)
		if err != nil {
			return err
		}
		_, err = enqueueDomainOutboxInTx(ctx, tx, &service.DomainOutboxEvent{
			AggregateType: "video_task",
			AggregateID:   input.TaskID,
			EventType:     spec.eventType,
			DedupKey:      fmt.Sprintf("video_task:%d:%s", input.TaskID, spec.suffix),
			Payload:       payload,
			Status:        service.DomainOutboxStatusPending,
			NextAttemptAt: input.CompletedAt,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func videoTaskTerminalMessage(status string) string {
	switch status {
	case service.VideoStatusSucceeded:
		return "video task succeeded"
	case service.VideoStatusFailed:
		return "video task failed"
	case service.VideoStatusCancelled:
		return "video task cancelled"
	default:
		return "video task terminal status updated"
	}
}
