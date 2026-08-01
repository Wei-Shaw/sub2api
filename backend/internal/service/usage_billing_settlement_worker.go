package service

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

const (
	usageBillingSettlementPollInterval = 2 * time.Second
	usageBillingSettlementBatchSize    = 32
	usageBillingSettlementLease        = 30 * time.Second
)

// UsageBillingSettlementWorker re-delivers persisted billing commands. The
// critical balance/quota effects remain guarded by UsageBillingRepository's
// request fingerprint deduplication, so a lease expiry or process restart can
// safely replay a command.
type UsageBillingSettlementWorker struct {
	billing    UsageBillingRepository
	settlement UsageBillingSettlementRepository
	stopOnce   sync.Once
	stopCh     chan struct{}
	doneCh     chan struct{}
}

func NewUsageBillingSettlementWorker(billing UsageBillingRepository) *UsageBillingSettlementWorker {
	worker := &UsageBillingSettlementWorker{
		billing: billing,
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
	}
	if settlement, ok := billing.(UsageBillingSettlementRepository); ok {
		worker.settlement = settlement
		go worker.run()
	} else {
		close(worker.doneCh)
	}
	return worker
}

func (w *UsageBillingSettlementWorker) run() {
	defer close(w.doneCh)
	ticker := time.NewTicker(usageBillingSettlementPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), usageBillingSettlementLease)
			if err := w.RunOnce(ctx); err != nil {
				slog.Warn("usage billing settlement worker cycle failed", "error", err)
			}
			cancel()
		case <-w.stopCh:
			return
		}
	}
}

// RunOnce claims and attempts one batch. It is exported for deterministic
// PostgreSQL integration tests and for controlled recovery hooks.
func (w *UsageBillingSettlementWorker) RunOnce(ctx context.Context) error {
	if w == nil || w.settlement == nil || w.billing == nil {
		return nil
	}
	jobs, err := w.settlement.ClaimDueSettlements(ctx, usageBillingSettlementBatchSize, usageBillingSettlementLease)
	if err != nil {
		return err
	}

	var firstErr error
	for _, job := range jobs {
		if job.Command == nil || job.RequestID == "" || job.APIKeyID == 0 {
			err := errors.New("invalid usage billing settlement job")
			if markErr := w.settlement.MarkSettlementRetry(ctx, job.ID, err.Error(), settlementRetryDelay(job.Attempts)); markErr != nil && firstErr == nil {
				firstErr = markErr
			}
			continue
		}

		if _, err := w.billing.Apply(ctx, job.Command); err != nil {
			if markErr := w.settlement.MarkSettlementRetry(ctx, job.ID, err.Error(), settlementRetryDelay(job.Attempts)); markErr != nil && firstErr == nil {
				firstErr = markErr
			}
			continue
		}
		if err := w.settlement.MarkSettlementApplied(ctx, job.RequestID, job.APIKeyID); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func settlementRetryDelay(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	delay := time.Duration(1<<minSettlementAttempt(attempts-1, 8)) * time.Second
	if delay > 5*time.Minute {
		return 5 * time.Minute
	}
	return delay
}

func minSettlementAttempt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (w *UsageBillingSettlementWorker) Stop() {
	if w == nil {
		return
	}
	w.stopOnce.Do(func() { close(w.stopCh) })
	<-w.doneCh
}
