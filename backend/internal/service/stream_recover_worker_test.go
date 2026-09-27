//go:build unit

package service

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type panicOnceOutboxRepo struct {
	SchedulerOutboxRepository
	calls      atomic.Int32
	secondOnce sync.Once
	second     chan struct{}
}

func (r *panicOnceOutboxRepo) ListAfterAndReleaseDedup(context.Context, int64, int) ([]SchedulerOutboxEvent, error) {
	if r.calls.Add(1) == 1 {
		panic("outbox decode bug")
	}
	r.secondOnce.Do(func() { close(r.second) })
	return nil, nil
}

func TestSchedulerOutboxWorkerSurvivesPanickingTick(t *testing.T) {
	repo := &panicOnceOutboxRepo{second: make(chan struct{})}
	svc := &SchedulerSnapshotService{cache: &outboxCleanupCache{}, outboxRepo: repo, stopCh: make(chan struct{})}

	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.runOutboxWorker(5 * time.Millisecond)
	}()

	select {
	case <-repo.second:
	case <-time.After(2 * time.Second):
		t.Fatal("the tick after the panicking one never ran")
	}
	close(svc.stopCh)
	<-done
}

func TestSchedulerFullRebuildPanicBecomesErrorAndNextRoundRuns(t *testing.T) {
	svc := &SchedulerSnapshotService{}

	err := svc.coalesceFullRebuild(func() error { panic("rebuild bug") })
	require.ErrorContains(t, err, "rebuild bug")

	ran := false
	require.NoError(t, svc.coalesceFullRebuild(func() error {
		ran = true
		return nil
	}))
	require.True(t, ran, "a panicking round must not be reused as the result of the next request")
}

type panicOnceRefresher struct {
	*poolHealthRefresher
	panicked atomic.Bool
}

func (r *panicOnceRefresher) Refresh(ctx context.Context, account *Account) (map[string]any, error) {
	if r.panicked.CompareAndSwap(false, true) {
		panic("refresher bug")
	}
	return r.poolHealthRefresher.Refresh(ctx, account)
}

type panicOncePager struct {
	*poolHealthAccountRepo
	panicked atomic.Bool
}

func (p *panicOncePager) ListOAuthRefreshCandidatePage(ctx context.Context, options OAuthRefreshPageOptions) (*OAuthRefreshCandidatePage, error) {
	if p.panicked.CompareAndSwap(false, true) {
		panic("pager bug")
	}
	return p.poolHealthAccountRepo.ListOAuthRefreshCandidatePage(ctx, options)
}

func recoverTestTokenRefreshConfig() config.TokenRefreshConfig {
	return config.TokenRefreshConfig{
		MaxRetries:            1,
		CandidatePageSize:     10,
		ProviderConcurrency:   1,
		ProviderQPS:           10000,
		AttemptTimeoutSeconds: 1,
		CycleTimeoutSeconds:   2,
	}
}

func TestTokenRefreshPanickingAccountDoesNotStopTheWorker(t *testing.T) {
	repo := &poolHealthAccountRepo{pages: map[int64][]Account{0: {grokPoolAccount(1), grokPoolAccount(2)}}}
	base := &poolHealthRefresher{}
	svc := newPoolHealthService(repo, base, recoverTestTokenRefreshConfig())
	refresher := &panicOnceRefresher{poolHealthRefresher: base}
	svc.registrations[0].refresher = refresher
	svc.registrations[0].executor = refresher

	svc.processRefreshContext(context.Background())

	_, updatedIDs, _, _ := repo.snapshot()
	require.True(t, refresher.panicked.Load())
	require.Len(t, updatedIDs, 1, "the account after the panicking one must still be refreshed")
}

func TestTokenRefreshPanickingCycleDoesNotStopTheNextCycle(t *testing.T) {
	repo := &poolHealthAccountRepo{pages: map[int64][]Account{0: {grokPoolAccount(1)}}}
	svc := newPoolHealthService(repo, &poolHealthRefresher{}, recoverTestTokenRefreshConfig())
	svc.candidatePager = &panicOncePager{poolHealthAccountRepo: repo}

	svc.processRefreshContext(context.Background())
	svc.processRefreshContext(context.Background())

	_, updatedIDs, _, _ := repo.snapshot()
	require.Equal(t, []int64{1}, updatedIDs)
}

type panickingNotificationUserRepo struct {
	UserRepository
	ctxs chan context.Context
}

func (r *panickingNotificationUserRepo) GetByID(ctx context.Context, _ int64) (*User, error) {
	r.ctxs <- ctx
	panic("user repo bug")
}

func TestPaymentFulfillmentNotificationRecoversPanic(t *testing.T) {
	repo := &panickingNotificationUserRepo{ctxs: make(chan context.Context, 1)}
	svc := &PaymentService{notificationEmailService: &NotificationEmailService{}, userRepo: repo}

	svc.dispatchPaymentFulfillmentNotification(&dbent.PaymentOrder{ID: 1, UserID: 2}, "RECHARGE_SUCCESS")

	var ctx context.Context
	select {
	case ctx = <-repo.ctxs:
	case <-time.After(2 * time.Second):
		t.Fatal("notification goroutine never ran")
	}
	// The send context is canceled by a defer that runs after the recover,
	// so observing the cancellation proves the goroutine unwound without crashing.
	select {
	case <-ctx.Done():
		require.ErrorIs(t, ctx.Err(), context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("notification goroutine did not unwind")
	}
}
