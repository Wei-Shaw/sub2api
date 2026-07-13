//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type targetClaimerFunc func(context.Context, TargetClaimRequest) (func(), bool, error)

func (f targetClaimerFunc) TryClaim(ctx context.Context, target TargetClaimRequest) (func(), bool, error) {
	return f(ctx, target)
}

func TestSelectAccountWithTargetClaimer_TriesNextCandidateWhenClaimRejected(t *testing.T) {
	repo := &mockAccountRepoForPlatform{
		accounts: []Account{
			{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 5},
			{ID: 2, Platform: PlatformAnthropic, Priority: 2, Status: StatusActive, Schedulable: true, Concurrency: 5},
		},
		accountsByID: map[int64]*Account{},
	}
	for i := range repo.accounts {
		repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	}

	cfg := testConfig()
	cfg.Gateway.Scheduling.LoadBatchEnabled = true

	// The legacy cache would accept account 1. The injected claimer rejects it,
	// so selecting account 2 proves the scheduler uses the request-scoped seam.
	svc := &GatewayService{
		accountRepo:        repo,
		cache:              &mockGatewayCacheForPlatform{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(&mockConcurrencyCache{}),
	}
	claimer := targetClaimerFunc(func(_ context.Context, target TargetClaimRequest) (func(), bool, error) {
		if target.AccountID == 1 {
			return nil, false, nil
		}
		return func() {}, true, nil
	})

	result, err := svc.selectAccountWithTargetClaimer(
		context.Background(),
		nil,
		"",
		"claude-3-5-sonnet-20241022",
		nil,
		"",
		0,
		claimer,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Account)
	require.Equal(t, int64(2), result.Account.ID)
	require.True(t, result.Acquired)
	require.NotNil(t, result.ReleaseFunc)
	result.ReleaseFunc()
}
