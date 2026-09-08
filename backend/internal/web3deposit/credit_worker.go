package web3deposit

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

const (
	DefaultCreditClaimLimit = 20
	DefaultCreditLease      = 45 * time.Second
	DefaultCreditPoll       = 5 * time.Second
)

type CreditJob struct {
	DepositID    int64
	ClaimVersion int32
}

type CreditJobStore interface {
	ClaimCreditJobs(context.Context, time.Time, time.Duration, int) ([]CreditJob, error)
	RetryCreditJob(context.Context, CreditJob, time.Time, error) error
}

type DepositCreditor interface {
	CreditDeposit(context.Context, CreditDepositRequest) (CreditDepositResult, error)
}

type CreditWorkerRuntime struct {
	jobs     CreditJobStore
	creditor DepositCreditor
	enabled  bool
	mu       sync.Mutex
	cancel   context.CancelFunc
	done     chan struct{}
}

func NewCreditWorkerRuntime(jobs CreditJobStore, creditor DepositCreditor, enabled bool) *CreditWorkerRuntime {
	return &CreditWorkerRuntime{jobs: jobs, creditor: creditor, enabled: enabled}
}

func (r *CreditWorkerRuntime) Start(ctx context.Context) {
	if r == nil || !r.enabled || r.jobs == nil || r.creditor == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		return
	}
	runCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	r.done = make(chan struct{})
	go func() { defer close(r.done); r.run(runCtx) }()
}

func (r *CreditWorkerRuntime) run(ctx context.Context) {
	for ctx.Err() == nil {
		now := time.Now()
		jobs, err := r.jobs.ClaimCreditJobs(ctx, now, DefaultCreditLease, DefaultCreditClaimLimit)
		if err == nil {
			for _, job := range jobs {
				_, creditErr := r.creditor.CreditDeposit(ctx, CreditDepositRequest{DepositID: job.DepositID, ClaimVersion: job.ClaimVersion, Now: time.Now()})
				if creditErr != nil && !errors.Is(creditErr, ErrCreditClaimLost) {
					web3RuntimeMetrics.creditFailures.Add(1)
					if retryErr := r.jobs.RetryCreditJob(ctx, job, time.Now(), creditErr); retryErr == nil {
						web3RuntimeMetrics.creditRetries.Add(1)
					}
					slog.Error("web3_deposit_credit_failed", "deposit_id", job.DepositID, "claim_version", job.ClaimVersion, "error", creditErr)
				}
			}
		} else {
			web3RuntimeMetrics.creditFailures.Add(1)
			slog.Error("web3_deposit_credit_claim_failed", "error", err)
		}
		timer := time.NewTimer(DefaultCreditPoll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (r *CreditWorkerRuntime) Stop() {
	if r == nil {
		return
	}
	r.mu.Lock()
	cancel, done := r.cancel, r.done
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}
