//go:build unit

package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// blockingTemporaryBalanceCache makes an asynchronous invalidation observable:
// GrantUserTemporaryBalance must not return until the stale balance entry has
// actually been invalidated.
type blockingTemporaryBalanceCache struct {
	billingCacheWorkerStub
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (c *blockingTemporaryBalanceCache) InvalidateUserBalance(context.Context, int64) error {
	c.once.Do(func() { close(c.entered) })
	<-c.release
	return nil
}

type temporaryBalanceGrantRepo struct {
	*userRepoStub
	grant *TemporaryBalanceGrant
}

func (r *temporaryBalanceGrantRepo) GetLatestUsedAtByUserID(context.Context, int64) (*time.Time, error) {
	return nil, nil
}

func (r *temporaryBalanceGrantRepo) GetTemporaryBalance(context.Context, int64) (*TemporaryBalanceGrant, error) {
	return r.grant, nil
}

func (r *temporaryBalanceGrantRepo) GetTemporaryBalances(context.Context, []int64) (map[int64]*TemporaryBalanceGrant, error) {
	return map[int64]*TemporaryBalanceGrant{r.grant.UserID: r.grant}, nil
}

func (r *temporaryBalanceGrantRepo) GrantTemporaryBalance(context.Context, int64, float64, time.Time, int64, string) (*TemporaryBalanceGrant, error) {
	return r.grant, nil
}

func (r *temporaryBalanceGrantRepo) ClearExpiredTemporaryBalances(context.Context, int) (int, error) {
	return 0, nil
}

func TestGrantUserTemporaryBalanceWaitsForCacheInvalidation(t *testing.T) {
	expiresAt := time.Now().UTC().Add(time.Hour)
	cache := &blockingTemporaryBalanceCache{entered: make(chan struct{}), release: make(chan struct{})}
	repo := &temporaryBalanceGrantRepo{
		userRepoStub: &userRepoStub{user: &User{ID: 7, Balance: 2}},
		grant:        &TemporaryBalanceGrant{UserID: 7, Amount: 5, ActiveAmount: 5, ExpiresAt: &expiresAt},
	}
	svc := &adminServiceImpl{
		userRepo:            repo,
		billingCacheService: &BillingCacheService{cache: cache},
	}

	result := make(chan error, 1)
	go func() {
		_, err := svc.GrantUserTemporaryBalance(context.Background(), 7, 5, expiresAt, 1, "test")
		result <- err
	}()
	select {
	case <-cache.entered:
	case <-time.After(time.Second):
		t.Fatal("cache invalidation was not attempted")
	}
	select {
	case err := <-result:
		t.Fatalf("grant returned before cache invalidation completed: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(cache.release)
	require.NoError(t, <-result)
}
