package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeUsageBillingSettlementRepository struct {
	mu          sync.Mutex
	job         UsageBillingSettlementJob
	available   bool
	applyCalls  int
	retryCalls  int
	appliedCall int
	failOnce    bool
}

func (f *fakeUsageBillingSettlementRepository) Apply(context.Context, *UsageBillingCommand) (*UsageBillingApplyResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.applyCalls++
	if f.failOnce {
		f.failOnce = false
		return nil, errors.New("transient settlement failure")
	}
	return &UsageBillingApplyResult{Applied: true}, nil
}

func (*fakeUsageBillingSettlementRepository) ReserveBatchImageBalance(context.Context, *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error) {
	return nil, nil
}

func (*fakeUsageBillingSettlementRepository) CaptureBatchImageBalance(context.Context, *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error) {
	return nil, nil
}

func (*fakeUsageBillingSettlementRepository) ReleaseBatchImageBalance(context.Context, *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error) {
	return nil, nil
}

func (f *fakeUsageBillingSettlementRepository) EnqueueSettlement(context.Context, *UsageBillingCommand, *UsageLog) error {
	return nil
}

func (f *fakeUsageBillingSettlementRepository) ClaimDueSettlements(context.Context, int, time.Duration) ([]UsageBillingSettlementJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.available {
		return nil, nil
	}
	f.available = false
	return []UsageBillingSettlementJob{f.job}, nil
}

func (f *fakeUsageBillingSettlementRepository) MarkSettlementApplied(context.Context, string, int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.appliedCall++
	return nil
}

func (f *fakeUsageBillingSettlementRepository) MarkSettlementRetry(context.Context, int64, string, time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.retryCalls++
	f.job.Attempts++
	f.available = true
	return nil
}

func TestUsageBillingSettlementWorker_RedeliversAfterTransientFailure(t *testing.T) {
	fake := &fakeUsageBillingSettlementRepository{
		job: UsageBillingSettlementJob{
			ID:        11,
			Attempts:  1,
			RequestID: "settlement-request",
			APIKeyID:  22,
			Command:   &UsageBillingCommand{RequestID: "settlement-request", APIKeyID: 22},
		},
		available: true,
		failOnce:  true,
	}
	worker := NewUsageBillingSettlementWorker(fake)
	defer worker.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := worker.RunOnce(ctx); err != nil {
		t.Fatalf("first worker cycle: %v", err)
	}
	fake.mu.Lock()
	if fake.retryCalls != 1 || fake.appliedCall != 0 {
		fake.mu.Unlock()
		t.Fatalf("first cycle state: retry=%d applied=%d", fake.retryCalls, fake.appliedCall)
	}
	fake.mu.Unlock()

	if err := worker.RunOnce(ctx); err != nil {
		t.Fatalf("redelivery worker cycle: %v", err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.applyCalls != 2 {
		t.Fatalf("apply calls = %d, want 2", fake.applyCalls)
	}
	if fake.appliedCall != 1 {
		t.Fatalf("applied calls = %d, want 1", fake.appliedCall)
	}
}
