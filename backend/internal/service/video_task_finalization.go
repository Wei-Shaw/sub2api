package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type VideoTaskFinalizationInput struct {
	TaskID               int64
	ExpectedVersion      int64
	TerminalStatus       string
	ProviderResultURL    string
	ProviderErrorMessage string
	ProviderPayload      map[string]any
	ActualDuration       *int
	ActualResolution     string
	ActualTokens         *int64
	LastFrameURL         string
	PollCount            int
	ActualCostUSD        Money
	PricingSnapshot      PricingSnapshot
	CompletedAt          time.Time
}

type VideoTaskFinalizationResult struct {
	Applied            bool
	Idempotent         bool
	Status             string
	Version            int64
	SettlementStatus   string
	BalanceChargedAt   *time.Time
	WorkerClaimedAt    *time.Time
	WorkerClaimedUntil *time.Time
	ArchiveStatus      string
	CaptureStatus      string
	CompletedAt        *time.Time
	ResultURL          string
	ErrorMessage       string
	CostEstimate       float64
	PollCount          int
	UsageTotalTokens   *int64
	ActualResolution   string
	ActualDuration     *int
	LastFrameURL       string
	TransactionID      *int64
	ReservationOverrun bool
}

type VideoTaskTerminalConflictError struct {
	TaskID          int64
	RequestedStatus string
	CurrentStatus   string
	CurrentVersion  int64
}

func (e *VideoTaskTerminalConflictError) Error() string {
	if e == nil {
		return ErrVideoTaskTerminalConflict.Error()
	}
	return fmt.Sprintf(
		"video task %d terminal conflict: requested %q, current %q at version %d",
		e.TaskID,
		e.RequestedStatus,
		e.CurrentStatus,
		e.CurrentVersion,
	)
}

func (e *VideoTaskTerminalConflictError) Unwrap() error {
	return ErrVideoTaskTerminalConflict
}

type VideoTaskFinalizer struct {
	repo VideoTaskFinalizationRepository
}

func NewVideoTaskFinalizer(repo VideoTaskFinalizationRepository) *VideoTaskFinalizer {
	return &VideoTaskFinalizer{repo: repo}
}

func (s *VideoGatewayService) SetVideoTaskFinalizer(finalizer *VideoTaskFinalizer) {
	if s == nil {
		return
	}
	s.taskFinalizer = finalizer
}

func (s *VideoGatewayService) videoTaskFinalizer() *VideoTaskFinalizer {
	if s == nil {
		return nil
	}
	return s.taskFinalizer
}

func applyVideoTaskFinalizationResult(task *VideoTask, result VideoTaskFinalizationResult) {
	if task == nil {
		return
	}
	if result.Status != "" {
		task.Status = result.Status
	}
	if result.Version > 0 {
		task.Version = result.Version
	}
	if result.SettlementStatus != "" {
		task.SettlementStatus = result.SettlementStatus
	}
	task.BalanceChargedAt = cloneVideoFinalizationTime(result.BalanceChargedAt)
	task.WorkerClaimedAt = cloneVideoFinalizationTime(result.WorkerClaimedAt)
	task.WorkerClaimedUntil = cloneVideoFinalizationTime(result.WorkerClaimedUntil)
	if result.ArchiveStatus != "" {
		task.ArchiveStatus = result.ArchiveStatus
	}
	if result.CaptureStatus != "" {
		task.CaptureStatus = result.CaptureStatus
	}
	if result.CompletedAt != nil {
		task.CompletedAt = cloneVideoFinalizationTime(result.CompletedAt)
	}
	task.ResultURL = result.ResultURL
	task.ErrorMessage = result.ErrorMessage
	task.CostEstimate = result.CostEstimate
	task.PollCount = result.PollCount
	task.UsageTotalTokens = cloneInt64Ptr(result.UsageTotalTokens)
	task.ActualResolution = result.ActualResolution
	task.ActualDuration = cloneVideoFinalizationInt(result.ActualDuration)
	task.LastFrameURL = result.LastFrameURL
}

func applyVideoTaskPollUpdateResult(task *VideoTask, result VideoTaskPollUpdateResult) {
	if task == nil {
		return
	}
	task.Status = result.Status
	task.Version = result.Version
	task.ResultURL = result.ResultURL
	task.ErrorMessage = result.ErrorMessage
	task.CostEstimate = result.CostEstimate
	task.PollCount = result.PollCount
	task.UsageTotalTokens = cloneInt64Ptr(result.UsageTotalTokens)
	task.ActualResolution = result.ActualResolution
	task.ActualDuration = cloneVideoFinalizationInt(result.ActualDuration)
	task.LastFrameURL = result.LastFrameURL
	task.SettlementStatus = result.SettlementStatus
	task.BalanceChargedAt = cloneVideoFinalizationTime(result.BalanceChargedAt)
	task.WorkerClaimedAt = cloneVideoFinalizationTime(result.WorkerClaimedAt)
	task.WorkerClaimedUntil = cloneVideoFinalizationTime(result.WorkerClaimedUntil)
	task.ArchiveStatus = result.ArchiveStatus
	task.CaptureStatus = result.CaptureStatus
	task.CompletedAt = cloneVideoFinalizationTime(result.CompletedAt)
}

func cloneVideoFinalizationTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneVideoFinalizationInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func (f *VideoTaskFinalizer) Finalize(ctx context.Context, input VideoTaskFinalizationInput) (VideoTaskFinalizationResult, error) {
	if f == nil || f.repo == nil {
		return VideoTaskFinalizationResult{}, fmt.Errorf("video task finalization repository is required")
	}
	if err := validateVideoTaskFinalizationInput(&input); err != nil {
		return VideoTaskFinalizationResult{}, err
	}
	if input.TerminalStatus != VideoStatusSucceeded {
		zero := MustUSD("0")
		input.ActualCostUSD = zero
		input.PricingSnapshot = PricingSnapshot{
			AmountOriginal: zero,
			ExchangeRate:   "1.0000000000",
			PricingSource:  NormalizeBillingPricingSource(input.PricingSnapshot.PricingSource),
			PricingVersion: NormalizeBillingPricingVersion(input.PricingSnapshot.PricingVersion),
		}
	}
	input.ProviderPayload = cloneFinalizationPayload(input.ProviderPayload)
	input.CompletedAt = input.CompletedAt.UTC().Truncate(time.Microsecond)
	result, err := f.repo.FinalizeVideoTask(ctx, input)
	if err != nil {
		var conflict *VideoTaskTerminalConflictError
		if errors.As(err, &conflict) {
			RecordReliabilityMetricAdd("video_finalization_conflict_total", 1, nil)
		} else if input.TerminalStatus == VideoStatusSucceeded {
			// A terminal settlement write is retried by the worker; recording the
			// failed attempt makes that operationally visible without changing the
			// durable state machine.
			RecordReliabilityMetricAdd("billing_settlement_retry_total", 1, nil)
		}
		return VideoTaskFinalizationResult{}, err
	}
	if result.Applied {
		RecordReliabilityMetricAdd("video_finalization_total", 1, map[string]string{"status": result.Status})
		if result.ReservationOverrun {
			RecordReliabilityMetricAdd("billing_reservation_overrun_total", 1, nil)
		}
		if result.SettlementStatus == VideoSettlementStatusSettled || result.SettlementStatus == VideoSettlementStatusReleased {
			RecordReliabilityMetricAdd("billing_reservation_active_total", -1, nil)
		}
	}
	return result, nil
}

func validateVideoTaskFinalizationInput(input *VideoTaskFinalizationInput) error {
	if input == nil {
		return fmt.Errorf("video task finalization input is required")
	}
	if input.TaskID <= 0 {
		return fmt.Errorf("video task id must be positive")
	}
	if input.ExpectedVersion <= 0 {
		return fmt.Errorf("video task expected version must be positive")
	}
	if !IsTerminalVideoStatus(input.TerminalStatus) {
		return ErrVideoInvalidStatus
	}
	if input.CompletedAt.IsZero() {
		return fmt.Errorf("video task completed time is required")
	}
	if input.ActualDuration != nil && *input.ActualDuration < 0 {
		return fmt.Errorf("video task actual duration must not be negative")
	}
	if input.ActualTokens != nil && *input.ActualTokens < 0 {
		return fmt.Errorf("video task actual tokens must not be negative")
	}
	if input.PollCount < 0 {
		return fmt.Errorf("video task poll count must not be negative")
	}
	if input.TerminalStatus != VideoStatusSucceeded {
		return nil
	}
	if input.ActualCostUSD.Currency() != CurrencyUSD || input.ActualCostUSD.IsNegative() {
		return fmt.Errorf("succeeded video task actual cost must be non-negative USD Money")
	}
	if strings.TrimSpace(string(input.PricingSnapshot.AmountOriginal.Currency())) == "" {
		return fmt.Errorf("succeeded video task original pricing amount is required")
	}
	exchangeRate, err := decimal.NewFromString(strings.TrimSpace(input.PricingSnapshot.ExchangeRate))
	if err != nil || !exchangeRate.IsPositive() {
		return fmt.Errorf("succeeded video task exchange rate must be positive")
	}
	if strings.TrimSpace(input.PricingSnapshot.PricingSource) == "" {
		return fmt.Errorf("succeeded video task pricing source is required")
	}
	if strings.TrimSpace(input.PricingSnapshot.PricingVersion) == "" {
		return fmt.Errorf("succeeded video task pricing version is required")
	}
	return nil
}

func cloneFinalizationPayload(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
