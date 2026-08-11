package web3deposit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCreditWorkerCreditsClaimedJobWithFencingVersion(t *testing.T) {
	jobs := &creditJobStoreStub{jobs: []CreditJob{{DepositID: 9, ClaimVersion: 3}}}
	credited := make(chan CreditDepositRequest, 1)
	creditor := depositCreditorFunc(func(_ context.Context, request CreditDepositRequest) (CreditDepositResult, error) {
		credited <- request
		return CreditDepositResult{}, nil
	})
	runtime := NewCreditWorkerRuntime(jobs, creditor, true)
	runtime.Start(context.Background())
	select {
	case request := <-credited:
		require.Equal(t, int64(9), request.DepositID)
		require.Equal(t, int32(3), request.ClaimVersion)
	case <-time.After(time.Second):
		t.Fatal("credit worker did not process claimed job")
	}
	runtime.Stop()
}

func TestCreditWorkerRetriesTransientFailure(t *testing.T) {
	jobs := &creditJobStoreStub{jobs: []CreditJob{{DepositID: 9, ClaimVersion: 3}}, retried: make(chan CreditJob, 1)}
	creditor := depositCreditorFunc(func(context.Context, CreditDepositRequest) (CreditDepositResult, error) {
		return CreditDepositResult{}, errors.New("database unavailable")
	})
	runtime := NewCreditWorkerRuntime(jobs, creditor, true)
	runtime.Start(context.Background())
	select {
	case job := <-jobs.retried:
		require.Equal(t, int32(3), job.ClaimVersion)
	case <-time.After(time.Second):
		t.Fatal("credit job was not retried")
	}
	runtime.Stop()
}

type creditJobStoreStub struct {
	jobs    []CreditJob
	claimed bool
	retried chan CreditJob
}

func (s *creditJobStoreStub) ClaimCreditJobs(context.Context, time.Time, time.Duration, int) ([]CreditJob, error) {
	if s.claimed {
		return nil, nil
	}
	s.claimed = true
	return s.jobs, nil
}
func (s *creditJobStoreStub) RetryCreditJob(_ context.Context, job CreditJob, _ time.Time, _ error) error {
	if s.retried != nil {
		s.retried <- job
	}
	return nil
}

type depositCreditorFunc func(context.Context, CreditDepositRequest) (CreditDepositResult, error)

func (f depositCreditorFunc) CreditDeposit(ctx context.Context, request CreditDepositRequest) (CreditDepositResult, error) {
	return f(ctx, request)
}
