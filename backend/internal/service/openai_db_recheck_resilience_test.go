package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

type openAIDBRecheckRepoStub struct {
	AccountRepository
	mu      sync.Mutex
	account *Account
	err     error
	delay   time.Duration
	calls   int
}

func (r *openAIDBRecheckRepoStub) GetByID(ctx context.Context, _ int64) (*Account, error) {
	r.mu.Lock()
	r.calls++
	delay, err, account := r.delay, r.err, r.account
	r.mu.Unlock()
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, ErrAccountNotFound
	}
	cloned := *account
	return &cloned, nil
}

func (r *openAIDBRecheckRepoStub) setResult(account *Account, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.account, r.err = account, err
}

func (r *openAIDBRecheckRepoStub) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func newOpenAIDBRecheckTestService(repo AccountRepository, freshMS, staleSeconds int) *OpenAIGatewayService {
	cfg := &config.Config{RunMode: config.RunModeStandard}
	cfg.Gateway.Scheduling.DBRecheckTimeoutMS = 20
	cfg.Gateway.Scheduling.DBRecheckFreshTTLMS = freshMS
	cfg.Gateway.Scheduling.DBRecheckStaleIfErrorSeconds = staleSeconds
	return &OpenAIGatewayService{
		accountRepo:       repo,
		cfg:               cfg,
		schedulerSnapshot: &SchedulerSnapshotService{},
	}
}

func eligibleOpenAIDBRecheckAccount() *Account {
	return &Account{
		ID:          44001,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 2,
	}
}

func recheckTestAccount(svc *OpenAIGatewayService, account *Account, groupID *int64) *Account {
	return svc.recheckSelectedOpenAIAccountFromDB(
		context.Background(),
		account,
		groupID,
		PlatformOpenAI,
		"gpt-test",
		false,
		"",
	)
}

func TestOpenAIAccountDBRecheckCoalescesHealthyReads(t *testing.T) {
	account := eligibleOpenAIDBRecheckAccount()
	repo := &openAIDBRecheckRepoStub{account: account, delay: 10 * time.Millisecond}
	svc := newOpenAIDBRecheckTestService(repo, 1000, 120)

	var wg sync.WaitGroup
	results := make(chan *Account, 24)
	for range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- recheckTestAccount(svc, account, nil)
		}()
	}
	wg.Wait()
	close(results)
	for result := range results {
		require.NotNil(t, result)
		require.Equal(t, account.ID, result.ID)
	}
	require.Equal(t, 1, repo.callCount())
}

func TestOpenAIAccountDBRecheckUsesOnlyRecentlyValidatedStaleAccount(t *testing.T) {
	account := eligibleOpenAIDBRecheckAccount()
	repo := &openAIDBRecheckRepoStub{account: account}
	svc := newOpenAIDBRecheckTestService(repo, 1, 1)

	require.NotNil(t, recheckTestAccount(svc, account, nil))
	time.Sleep(5 * time.Millisecond)
	repo.setResult(nil, context.DeadlineExceeded)
	require.NotNil(t, recheckTestAccount(svc, account, nil))
	require.Equal(t, 2, repo.callCount())

	// Retry suppression must prevent a DB failure storm without extending stale TTL.
	require.NotNil(t, recheckTestAccount(svc, account, nil))
	require.Equal(t, 2, repo.callCount())
}

func TestOpenAIAccountDBRecheckFailsClosedWithoutValidatedHistory(t *testing.T) {
	account := eligibleOpenAIDBRecheckAccount()
	repo := &openAIDBRecheckRepoStub{err: context.DeadlineExceeded}
	svc := newOpenAIDBRecheckTestService(repo, 1, 120)

	require.Nil(t, recheckTestAccount(svc, account, nil))
	require.Equal(t, 1, repo.callCount())
}

func TestOpenAIAccountDBRecheckDoesNotMaskDeletionOrGroupMove(t *testing.T) {
	groupID, otherGroupID := int64(91), int64(92)
	account := eligibleOpenAIDBRecheckAccount()
	account.GroupIDs = []int64{groupID}
	repo := &openAIDBRecheckRepoStub{account: account}
	svc := newOpenAIDBRecheckTestService(repo, 1, 120)

	require.NotNil(t, recheckTestAccount(svc, account, &groupID))
	time.Sleep(5 * time.Millisecond)
	moved := *account
	moved.GroupIDs = []int64{otherGroupID}
	repo.setResult(&moved, nil)
	require.Nil(t, recheckTestAccount(svc, account, &groupID))

	time.Sleep(5 * time.Millisecond)
	repo.setResult(nil, ErrAccountNotFound)
	require.Nil(t, recheckTestAccount(svc, account, &groupID))
}

func TestOpenAIAccountDBRecheckRetrySuppressionNeverExtendsStaleWindow(t *testing.T) {
	account := eligibleOpenAIDBRecheckAccount()
	var cache openAIAccountDBRecheckCache
	now := time.Now()
	cache.store(account, now, time.Millisecond, 10*time.Millisecond)

	require.NotNil(t, cache.stale(account.ID, now.Add(2*time.Millisecond)))
	require.Nil(t, cache.fresh(account.ID, now.Add(11*time.Millisecond)))
	require.Nil(t, cache.stale(account.ID, now.Add(11*time.Millisecond)))
}

type schedulerGroupRepoNoRead struct {
	GroupRepository
	called bool
}

func (r *schedulerGroupRepoNoRead) GetByID(context.Context, int64) (*Group, error) {
	r.called = true
	return nil, context.DeadlineExceeded
}

func TestSchedulerSnapshotUsesAuthenticatedGroupContextWithoutDB(t *testing.T) {
	group := &Group{
		ID:          77,
		Platform:    PlatformOpenAI,
		Status:      StatusActive,
		Hydrated:    true,
		IsExclusive: true,
	}
	repo := &schedulerGroupRepoNoRead{}
	svc := &SchedulerSnapshotService{groupRepo: repo}
	ctx := context.WithValue(context.Background(), ctxkey.Group, group)

	resolved, err := svc.GetGroupByID(ctx, group.ID)
	require.NoError(t, err)
	require.Same(t, group, resolved)
	require.False(t, repo.called)
}
