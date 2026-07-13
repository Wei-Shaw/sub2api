package service

import "context"

type TargetClaimRequest struct {
	Platform           string
	AccountID          int64
	AccountConcurrency int
}

// TargetClaimer performs the authoritative, non-blocking claim for a selected
// upstream target. Request-scoped implementations may carry user and queue
// state internally without exposing it to the scheduler.
type TargetClaimer interface {
	TryClaim(ctx context.Context, target TargetClaimRequest) (release func(), claimed bool, err error)
}

type legacyTargetClaimer struct {
	concurrencyService *ConcurrencyService
}

func (c legacyTargetClaimer) TryClaim(ctx context.Context, target TargetClaimRequest) (func(), bool, error) {
	if c.concurrencyService == nil {
		return func() {}, true, nil
	}

	result, err := c.concurrencyService.AcquireAccountSlot(ctx, target.AccountID, target.AccountConcurrency)
	if err != nil {
		return nil, false, err
	}
	if result == nil || !result.Acquired {
		return nil, false, nil
	}
	return result.ReleaseFunc, true, nil
}

func tryClaimTarget(ctx context.Context, claimer TargetClaimer, account *Account) (*AcquireResult, error) {
	release, claimed, err := claimer.TryClaim(ctx, TargetClaimRequest{
		Platform:           account.Platform,
		AccountID:          account.ID,
		AccountConcurrency: account.Concurrency,
	})
	if err != nil {
		return nil, err
	}
	return &AcquireResult{
		Acquired:    claimed,
		ReleaseFunc: release,
	}, nil
}
