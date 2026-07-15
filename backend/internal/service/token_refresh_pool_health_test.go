package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type poolHealthAccountRepo struct {
	AccountRepository

	mu                   sync.Mutex
	pages                map[int64][]Account
	requests             []OAuthRefreshPageOptions
	updatedCredentialIDs []int64
	setErrorCalls        int
	setTempUnschedCalls  int
	getByIDErr           error
}

func (r *poolHealthAccountRepo) GetByID(_ context.Context, _ int64) (*Account, error) {
	if r.getByIDErr != nil {
		return nil, r.getByIDErr
	}
	return nil, ErrAccountNotFound
}

func (r *poolHealthAccountRepo) ListOAuthRefreshCandidatePage(_ context.Context, options OAuthRefreshPageOptions) (*OAuthRefreshCandidatePage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, options)
	accounts := append([]Account(nil), r.pages[options.AfterID]...)
	page := &OAuthRefreshCandidatePage{Accounts: accounts, HasMore: len(accounts) == options.Limit}
	if len(accounts) > 0 {
		page.NextAfterID = accounts[len(accounts)-1].ID
	}
	return page, nil
}

func (r *poolHealthAccountRepo) UpdateCredentials(_ context.Context, id int64, _ map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updatedCredentialIDs = append(r.updatedCredentialIDs, id)
	return nil
}

func (r *poolHealthAccountRepo) UpdateGrokOAuthCredentialsIfUnchanged(
	_ context.Context,
	id int64,
	_ map[string]any,
	_ *int64,
	_ map[string]any,
) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updatedCredentialIDs = append(r.updatedCredentialIDs, id)
	return true, nil
}

func (r *poolHealthAccountRepo) SetError(context.Context, int64, string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.setErrorCalls++
	return nil
}

func (r *poolHealthAccountRepo) SetGrokOAuthErrorIfCredentialsUnchanged(context.Context, int64, map[string]any, string) (bool, error) {
	return false, nil
}

func (r *poolHealthAccountRepo) SetGrokOAuthRefreshErrorIfCredentialsUnchanged(context.Context, int64, map[string]any, *int64, string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.setErrorCalls++
	return true, nil
}

func (r *poolHealthAccountRepo) SetGrokOAuthRefreshTempUnschedulableIfCredentialsUnchanged(context.Context, int64, map[string]any, *int64, time.Time, string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.setTempUnschedCalls++
	return true, nil
}

func (r *poolHealthAccountRepo) SetTempUnschedulable(context.Context, int64, time.Time, string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.setTempUnschedCalls++
	return nil
}

func (r *poolHealthAccountRepo) snapshot() ([]OAuthRefreshPageOptions, []int64, int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]OAuthRefreshPageOptions(nil), r.requests...), append([]int64(nil), r.updatedCredentialIDs...), r.setErrorCalls, r.setTempUnschedCalls
}

type poolHealthRefresher struct {
	err            error
	delay          time.Duration
	startDelays    []time.Duration
	ignoreContext  bool
	cancel         context.CancelFunc
	newCredentials map[string]any
	calls          atomic.Int64
	active         atomic.Int64
	maxActive      atomic.Int64
	startMu        sync.Mutex
	startTimes     []time.Time
}

type countingRefreshAttemptGate struct {
	calls atomic.Int64
}

type rejectedRefreshAttemptGate struct {
	err error
}

type poolHealthTokenCacheStub struct {
	GeminiTokenCache
}

type tripBeforeRateAdmissionGate struct {
	state *tokenRefreshProviderState
}

func (g *tripBeforeRateAdmissionGate) acquire(ctx context.Context) (func(), error) {
	release, err := g.state.acquire(ctx)
	if err != nil {
		return nil, err
	}
	g.state.mu.Lock()
	g.state.tripped = true
	g.state.mu.Unlock()
	return release, nil
}

func (g *tripBeforeRateAdmissionGate) acquireRate(ctx context.Context) (func(), error) {
	return g.state.acquireRate(ctx)
}

type breakerTripAccountRepo struct {
	*productionPathRateRepo
	setErrorCalls atomic.Int64
	setTempCalls  atomic.Int64
}

func (r *breakerTripAccountRepo) SetGrokOAuthRefreshErrorIfCredentialsUnchanged(context.Context, int64, map[string]any, *int64, string) (bool, error) {
	r.setErrorCalls.Add(1)
	return true, nil
}

func (r *breakerTripAccountRepo) SetGrokOAuthRefreshTempUnschedulableIfCredentialsUnchanged(context.Context, int64, map[string]any, *int64, time.Time, string) (bool, error) {
	r.setTempCalls.Add(1)
	return true, nil
}

func (g *rejectedRefreshAttemptGate) acquire(context.Context) (func(), error) {
	return nil, g.err
}

type productionPathRateRepo struct {
	AccountRepository

	mu       sync.Mutex
	accounts map[int64]*Account
}

func (r *productionPathRateRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	account := r.accounts[id]
	if account == nil {
		return nil, ErrAccountNotFound
	}
	return snapshotOAuthRefreshAccount(account), nil
}

func (r *productionPathRateRepo) UpdateCredentials(_ context.Context, id int64, credentials map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	account := r.accounts[id]
	if account == nil {
		return ErrAccountNotFound
	}
	account.Credentials = shallowCopyMap(credentials)
	return nil
}

func (r *productionPathRateRepo) UpdateGrokOAuthCredentialsIfUnchanged(
	_ context.Context,
	id int64,
	expectedCredentials map[string]any,
	expectedProxyID *int64,
	credentials map[string]any,
) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	account := r.accounts[id]
	if account == nil || !reflect.DeepEqual(account.Credentials, expectedCredentials) ||
		!reflect.DeepEqual(account.ProxyID, expectedProxyID) {
		return false, nil
	}
	account.Credentials = shallowCopyMap(credentials)
	return true, nil
}

type productionPathRefreshStart struct {
	accountID int64
	at        time.Time
}

type productionPathRateExecutor struct {
	firstStarted chan struct{}
	releaseFirst chan struct{}
	calls        atomic.Int64
	startMu      sync.Mutex
	starts       []productionPathRefreshStart
}

func (e *productionPathRateExecutor) CacheKey(account *Account) string {
	return fmt.Sprintf("production-path-rate:%d", account.ID)
}

func (e *productionPathRateExecutor) CanRefresh(account *Account) bool {
	return account != nil && account.IsGrokOAuth()
}

func (e *productionPathRateExecutor) NeedsRefresh(account *Account, _ time.Duration) bool {
	needsRefresh, _ := account.Credentials["needs_refresh"].(bool)
	return needsRefresh
}

func (e *productionPathRateExecutor) Refresh(ctx context.Context, account *Account) (map[string]any, error) {
	call := e.calls.Add(1)
	e.startMu.Lock()
	e.starts = append(e.starts, productionPathRefreshStart{accountID: account.ID, at: time.Now()})
	e.startMu.Unlock()
	if call == 1 {
		close(e.firstStarted)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-e.releaseFirst:
		}
	}
	return map[string]any{
		"access_token":  fmt.Sprintf("fresh-access-%d", account.ID),
		"refresh_token": fmt.Sprintf("fresh-refresh-%d", account.ID),
		"needs_refresh": false,
	}, nil
}

func (e *productionPathRateExecutor) startsSnapshot() []productionPathRefreshStart {
	e.startMu.Lock()
	defer e.startMu.Unlock()
	return append([]productionPathRefreshStart(nil), e.starts...)
}

func (g *countingRefreshAttemptGate) acquire(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	g.calls.Add(1)
	return func() {}, nil
}

func (r *poolHealthRefresher) CacheKey(account *Account) string {
	return fmt.Sprintf("pool-health:%d", account.ID)
}

func (r *poolHealthRefresher) CanRefresh(account *Account) bool {
	return account != nil && account.Platform == PlatformGrok && account.Type == AccountTypeOAuth
}

func (r *poolHealthRefresher) NeedsRefresh(*Account, time.Duration) bool { return true }

func (r *poolHealthRefresher) Refresh(ctx context.Context, _ *Account) (map[string]any, error) {
	r.calls.Add(1)
	active := r.active.Add(1)
	defer r.active.Add(-1)
	r.startMu.Lock()
	startIndex := len(r.startTimes)
	r.startTimes = append(r.startTimes, time.Now())
	delay := r.delay
	if startIndex < len(r.startDelays) {
		delay = r.startDelays[startIndex]
	}
	r.startMu.Unlock()
	for {
		maxActive := r.maxActive.Load()
		if active <= maxActive || r.maxActive.CompareAndSwap(maxActive, active) {
			break
		}
	}
	if r.cancel != nil {
		r.cancel()
	}
	if delay > 0 {
		if r.ignoreContext {
			time.Sleep(delay)
		} else {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
	}
	if r.err != nil {
		return nil, r.err
	}
	if r.newCredentials != nil {
		credentials := make(map[string]any, len(r.newCredentials))
		for key, value := range r.newCredentials {
			credentials[key] = value
		}
		return credentials, nil
	}
	return map[string]any{"access_token": "new-token", "refresh_token": "new-refresh-token"}, nil
}

func (r *poolHealthRefresher) startsSnapshot() []time.Time {
	r.startMu.Lock()
	defer r.startMu.Unlock()
	return append([]time.Time(nil), r.startTimes...)
}

func grokPoolAccount(id int64) Account {
	return Account{
		ID:       id,
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Credentials: map[string]any{
			"access_token":  "old-token",
			"refresh_token": "refresh-token",
		},
	}
}

func newPoolHealthService(repo *poolHealthAccountRepo, refresher *poolHealthRefresher, cfg config.TokenRefreshConfig) *TokenRefreshService {
	return &TokenRefreshService{
		accountRepo:    repo,
		candidatePager: repo,
		registrations: []tokenRefreshRegistration{{
			platform:  PlatformGrok,
			refresher: refresher,
			executor:  refresher,
		}},
		refreshPolicy: DefaultBackgroundRefreshPolicy(),
		cfg:           &cfg,
	}
}

func TestTokenRefreshService_RegistrationsAreCandidateEligibilitySource(t *testing.T) {
	cfg := &config.Config{}
	svc := NewTokenRefreshService(nil, nil, nil, nil, nil, nil, nil, cfg, nil)

	require.Equal(t, []string{
		PlatformAnthropic,
		PlatformOpenAI,
		PlatformGemini,
		PlatformAntigravity,
		PlatformGrok,
	}, svc.eligiblePlatforms())
	require.Len(t, svc.registrations, 5)
	for _, registration := range svc.registrations {
		require.NotNil(t, registration.refresher)
		require.NotNil(t, registration.executor)
	}
}

func TestTokenRefreshService_ProcessRefreshPagesByStableCursor(t *testing.T) {
	repo := &poolHealthAccountRepo{pages: map[int64][]Account{
		0: {grokPoolAccount(1), grokPoolAccount(2)},
		2: {grokPoolAccount(3)},
	}}
	refresher := &poolHealthRefresher{}
	svc := newPoolHealthService(repo, refresher, config.TokenRefreshConfig{
		RefreshBeforeExpiryHours: 1,
		MaxRetries:               1,
		CandidatePageSize:        2,
		ProviderConcurrency:      4,
		ProviderQPS:              10000,
		AttemptTimeoutSeconds:    1,
		CycleTimeoutSeconds:      2,
	})

	svc.processRefreshContext(context.Background())

	requests, updatedIDs, _, _ := repo.snapshot()
	require.Len(t, requests, 2)
	require.Equal(t, int64(0), requests[0].AfterID)
	require.Equal(t, int64(2), requests[1].AfterID)
	require.Equal(t, []string{PlatformGrok}, requests[0].Platforms)
	require.True(t, requests[0].ActiveOnly)
	require.True(t, requests[0].RequireRefreshToken)
	require.True(t, requests[0].ExcludeRetryCooldown)
	sort.Slice(updatedIDs, func(i, j int) bool { return updatedIDs[i] < updatedIDs[j] })
	require.Equal(t, []int64{1, 2, 3}, updatedIDs)
	require.Zero(t, svc.candidateAfterID(), "a short final page must wrap the next cycle to the beginning")
}

func TestTokenRefreshService_BoundsPerProviderConcurrency(t *testing.T) {
	accounts := make([]Account, 0, 8)
	for id := int64(1); id <= 8; id++ {
		accounts = append(accounts, grokPoolAccount(id))
	}
	repo := &poolHealthAccountRepo{pages: map[int64][]Account{0: accounts}}
	refresher := &poolHealthRefresher{delay: 20 * time.Millisecond}
	svc := newPoolHealthService(repo, refresher, config.TokenRefreshConfig{
		MaxRetries:            1,
		CandidatePageSize:     20,
		ProviderConcurrency:   2,
		ProviderQPS:           10000,
		AttemptTimeoutSeconds: 1,
		CycleTimeoutSeconds:   2,
	})

	svc.processRefreshContext(context.Background())

	require.Equal(t, int64(8), refresher.calls.Load())
	require.Equal(t, int64(2), refresher.maxActive.Load())
}

func TestTokenRefreshRateGate_ReservesSpacedSlotsAndHonorsCancellation(t *testing.T) {
	const interval = 25 * time.Millisecond
	gate := newTokenRefreshRateGateWithInterval(interval)
	base := time.Unix(1_700_000_000, 0)

	require.Equal(t, base, gate.reserveSlot(base))
	require.Equal(t, base.Add(interval), gate.reserveSlot(base))
	require.Equal(t, base.Add(2*interval), gate.reserveSlot(base))
	jumped := base.Add(time.Second)
	require.Equal(t, jumped, gate.reserveSlot(jumped), "an idle gate should not retain stale delay")

	cancelGate := newTokenRefreshRateGateWithInterval(time.Hour)
	require.NoError(t, cancelGate.wait(context.Background()), "the first slot is immediately available")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	require.ErrorIs(t, cancelGate.wait(ctx), context.Canceled)
	require.Less(t, time.Since(started), 100*time.Millisecond, "cancellation must not wait for the reserved slot")
}

func TestTokenRefreshService_RetriesAcquireRateSlotPerAttempt(t *testing.T) {
	repo := &poolHealthAccountRepo{}
	refresher := &poolHealthRefresher{err: errors.New("temporary provider failure")}
	svc := newPoolHealthService(repo, refresher, config.TokenRefreshConfig{MaxRetries: 3})
	gate := &countingRefreshAttemptGate{}
	account := grokPoolAccount(44)

	err := svc.refreshWithRetryWithRateGate(context.Background(), &account, refresher, nil, time.Hour, gate)

	require.Error(t, err)
	require.Equal(t, int64(3), refresher.calls.Load())
	require.Equal(t, int64(3), gate.calls.Load(), "every upstream retry must consume a provider rate slot")
}

func TestTokenRefreshService_ProcessProviderAccountsLegacyNilReleaseGateIsSafe(t *testing.T) {
	repo := &poolHealthAccountRepo{}
	refresher := &poolHealthRefresher{}
	svc := newPoolHealthService(repo, refresher, config.TokenRefreshConfig{
		MaxRetries:          1,
		ProviderConcurrency: 1,
	})
	state := &tokenRefreshProviderState{
		service: svc,
		registration: tokenRefreshRegistration{
			platform:  PlatformGrok,
			refresher: refresher,
			// nil executor deliberately exercises the legacy/direct fallback.
			executor: nil,
		},
		// Admission rejection validly returns no release callback. The direct
		// fallback must propagate the skip without dereferencing that nil handle.
		rateGate: &rejectedRefreshAttemptGate{err: errRefreshSkipped},
		poolGate: nil,
	}
	account := grokPoolAccount(45)

	refreshed, skipped, failed := svc.processProviderAccounts(
		context.Background(),
		state,
		[]*Account{&account},
		time.Hour,
	)

	require.Zero(t, refreshed)
	require.Equal(t, 1, skipped)
	require.Zero(t, failed)
	require.Zero(t, refresher.calls.Load(), "rejected rate admission must not reach the legacy upstream refresher")
}

func TestTokenRefreshService_ProviderRateGateIsSharedAcrossRuns(t *testing.T) {
	svc := &TokenRefreshService{cfg: &config.TokenRefreshConfig{ProviderQPS: 40}}
	first := svc.providerRateGate(PlatformGrok)
	second := svc.providerRateGate(PlatformGrok)
	require.Same(t, first, second, "background cycles and reconciliation must share the process-local provider limiter")

	base := time.Unix(1_700_000_000, 0)
	require.Equal(t, base, first.reserveSlot(base))
	require.Equal(t, base.Add(25*time.Millisecond), second.reserveSlot(base))
}

func TestTokenRefreshService_ProviderConcurrencyGateIsSharedAcrossBackgroundAndConcurrentAdminReconciliation(t *testing.T) {
	accounts := []Account{
		grokPoolAccount(1),
		grokPoolAccount(2),
		grokPoolAccount(3),
		grokPoolAccount(4),
	}
	repo := &poolHealthAccountRepo{pages: map[int64][]Account{0: accounts}}
	refresher := &poolHealthRefresher{delay: 80 * time.Millisecond}
	svc := newPoolHealthService(repo, refresher, config.TokenRefreshConfig{
		RefreshBeforeExpiryHours: 1,
		MaxRetries:               1,
		CandidatePageSize:        20,
		ProviderConcurrency:      2,
		ProviderQPS:              100,
		ProviderFailureThreshold: 20,
		AttemptTimeoutSeconds:    1,
		CycleTimeoutSeconds:      3,
	})

	firstGate := svc.providerConcurrencyGate(PlatformGrok)
	require.Same(t, firstGate, svc.providerConcurrencyGate(PlatformGrok))

	start := make(chan struct{})
	adminErrors := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		<-start
		svc.processRefreshContext(context.Background())
	}()
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			<-start
			_, err := svc.ReconcileGrokOAuth(context.Background(), GrokOAuthReconcileInput{Apply: true, Limit: 20})
			adminErrors <- err
		}()
	}
	close(start)
	wg.Wait()
	close(adminErrors)

	for err := range adminErrors {
		require.NoError(t, err)
	}
	require.Equal(t, int64(12), refresher.calls.Load(), "background and both admin calls must all execute")
	require.Equal(t, int64(2), refresher.maxActive.Load(),
		"all entry points must share the configured per-provider upstream concurrency cap")
}

func TestTokenRefreshService_SaturatedProviderPreservesConcurrencyAndActualQPSStartSpacing(t *testing.T) {
	const (
		providerConcurrency = 2
		providerQPS         = 20
		attemptCount        = 8
	)
	repo := &poolHealthAccountRepo{}
	refresher := &poolHealthRefresher{
		// The first two QPS-spaced attempts finish together. If queued callers
		// reserve QPS slots before acquiring provider capacity, two expired
		// reservations can then burst upstream at the same time.
		startDelays: []time.Duration{
			220 * time.Millisecond,
			170 * time.Millisecond,
			20 * time.Millisecond,
			20 * time.Millisecond,
			20 * time.Millisecond,
			20 * time.Millisecond,
			20 * time.Millisecond,
			20 * time.Millisecond,
		},
	}
	svc := newPoolHealthService(repo, refresher, config.TokenRefreshConfig{
		MaxRetries:            1,
		ProviderConcurrency:   providerConcurrency,
		ProviderQPS:           providerQPS,
		AttemptTimeoutSeconds: 1,
	})
	registration := svc.registrations[0]
	sharedRateGate := svc.providerRateGate(PlatformGrok)
	sharedPoolGate := svc.providerConcurrencyGate(PlatformGrok)

	start := make(chan struct{})
	errorsCh := make(chan error, attemptCount)
	var wg sync.WaitGroup
	for i := 0; i < attemptCount; i++ {
		account := grokPoolAccount(int64(i + 1))
		state := &tokenRefreshProviderState{
			service:      svc,
			registration: registration,
			rateGate:     sharedRateGate,
			poolGate:     sharedPoolGate,
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errorsCh <- svc.refreshWithRetryWithRateGate(context.Background(), &account, refresher, nil, time.Hour, state)
		}()
	}
	close(start)
	wg.Wait()
	close(errorsCh)

	for err := range errorsCh {
		require.NoError(t, err)
	}
	require.Equal(t, int64(providerConcurrency), refresher.maxActive.Load(),
		"the scripted attempts must actually saturate the provider semaphore")
	starts := refresher.startsSnapshot()
	require.Len(t, starts, attemptCount)
	configuredSpacing := time.Second / time.Duration(providerQPS)
	minimumObservedSpacing := configuredSpacing - 10*time.Millisecond
	actualMinimumSpacing := starts[1].Sub(starts[0])
	for i := 1; i < len(starts); i++ {
		spacing := starts[i].Sub(starts[i-1])
		if spacing < actualMinimumSpacing {
			actualMinimumSpacing = spacing
		}
		require.GreaterOrEqualf(t, spacing, minimumObservedSpacing,
			"upstream starts %d and %d violated configured QPS spacing", i-1, i)
	}
	t.Logf("max_active=%d configured_concurrency=%d minimum_start_spacing=%s configured_spacing=%s",
		refresher.maxActive.Load(), providerConcurrency, actualMinimumSpacing, configuredSpacing)
}

func TestTokenRefreshService_ProductionPathRatesOnlyActualRefreshAfterSameAccountContention(t *testing.T) {
	const interval = 200 * time.Millisecond
	accountOne := grokPoolAccount(71)
	accountOne.Credentials["needs_refresh"] = true
	accountTwo := grokPoolAccount(72)
	accountTwo.Credentials["needs_refresh"] = true
	firstSelection := snapshotOAuthRefreshAccount(&accountOne)
	contendingSelection := snapshotOAuthRefreshAccount(&accountOne)
	differentSelection := snapshotOAuthRefreshAccount(&accountTwo)
	repo := &productionPathRateRepo{accounts: map[int64]*Account{
		accountOne.ID: snapshotOAuthRefreshAccount(&accountOne),
		accountTwo.ID: snapshotOAuthRefreshAccount(&accountTwo),
	}}
	executor := &productionPathRateExecutor{
		firstStarted: make(chan struct{}),
		releaseFirst: make(chan struct{}),
	}
	svc := &TokenRefreshService{
		accountRepo:            repo,
		refreshAPI:             NewOAuthRefreshAPI(repo, nil),
		refreshPolicy:          DefaultBackgroundRefreshPolicy(),
		cfg:                    &config.TokenRefreshConfig{MaxRetries: 1},
		attemptTimeoutOverride: 2 * time.Second,
	}
	state := &tokenRefreshProviderState{
		service:  svc,
		rateGate: newTokenRefreshRateGateWithInterval(interval),
		poolGate: newTokenRefreshConcurrencyGate(2),
	}

	errorsCh := make(chan error, 3)
	go func() {
		errorsCh <- svc.refreshWithRetryWithRateGate(context.Background(), firstSelection, executor, executor, time.Hour, state)
	}()
	select {
	case <-executor.firstStarted:
	case <-time.After(time.Second):
		require.FailNow(t, "first production-path refresh did not reach the upstream executor")
	}

	go func() {
		errorsCh <- svc.refreshWithRetryWithRateGate(context.Background(), contendingSelection, executor, executor, time.Hour, state)
	}()
	require.Eventually(t, func() bool {
		return len(state.poolGate.slots) == 2
	}, time.Second, time.Millisecond, "same-account contender must hold the second provider slot while waiting on the local refresh lock")

	go func() {
		errorsCh <- svc.refreshWithRetryWithRateGate(context.Background(), differentSelection, executor, executor, time.Hour, state)
	}()
	close(executor.releaseFirst)

	skipped := 0
	for i := 0; i < 3; i++ {
		err := <-errorsCh
		if errors.Is(err, errRefreshSkipped) {
			skipped++
			continue
		}
		require.NoError(t, err)
	}
	require.Equal(t, 1, skipped, "the same-account contender must reread the refreshed row and skip without upstream admission")

	starts := executor.startsSnapshot()
	require.Len(t, starts, 2, "only the two accounts that actually refresh may consume QPS admission")
	require.Equal(t, int64(71), starts[0].accountID)
	require.Equal(t, int64(72), starts[1].accountID)
	spacing := starts[1].at.Sub(starts[0].at)
	require.GreaterOrEqual(t, spacing, interval-30*time.Millisecond)
	require.Less(t, spacing, 350*time.Millisecond,
		"a same-account lock waiter must not consume a rate slot and push the different-account refresh to the second interval")
	t.Logf("actual_refresh_calls=%d actual_start_spacing=%s configured_spacing=%s", executor.calls.Load(), spacing, interval)
}

func TestTokenRefreshService_ProviderTripBeforeRateAdmissionSkipsWithoutAccountMutation(t *testing.T) {
	account := grokPoolAccount(73)
	stored := snapshotOAuthRefreshAccount(&account)
	repo := &breakerTripAccountRepo{productionPathRateRepo: &productionPathRateRepo{
		accounts: map[int64]*Account{account.ID: stored},
	}}
	refresher := &poolHealthRefresher{}
	svc := &TokenRefreshService{
		accountRepo:   repo,
		refreshAPI:    NewOAuthRefreshAPI(repo, nil),
		refreshPolicy: DefaultBackgroundRefreshPolicy(),
		cfg:           &config.TokenRefreshConfig{MaxRetries: 1},
	}
	state := &tokenRefreshProviderState{
		service:  svc,
		rateGate: newTokenRefreshRateGate(1),
		poolGate: newTokenRefreshConcurrencyGate(1),
	}
	gate := &tripBeforeRateAdmissionGate{state: state}

	err := svc.refreshWithRetryWithRateGate(context.Background(), &account, refresher, refresher, time.Hour, gate)

	require.ErrorIs(t, err, errRefreshSkipped)
	require.Zero(t, refresher.calls.Load(), "a tripped provider must not reach upstream rate admission")
	require.Zero(t, repo.setErrorCalls.Load())
	require.Zero(t, repo.setTempCalls.Load(), "provider skip must never fall through to per-account cooldown")
}

func TestTokenRefreshService_ConfigBounds(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	svc := &TokenRefreshService{cfg: &config.TokenRefreshConfig{
		MaxRetries:               maxInt,
		RetryBackoffSeconds:      maxInt,
		ProviderFailureThreshold: maxInt,
		AttemptTimeoutSeconds:    maxInt,
		CycleTimeoutSeconds:      maxInt,
	}}

	require.Equal(t, maxTokenRefreshMaxRetries, svc.maxRetries())
	require.Equal(t, maxTokenRefreshProviderFailureThreshold, svc.providerFailureThreshold())
	require.Equal(t, maxTokenRefreshAttemptTimeout, svc.attemptTimeout())
	require.Equal(t, maxTokenRefreshCycleTimeout, svc.cycleTimeout())
	require.LessOrEqual(t, svc.retryBackoff(1, maxTokenRefreshMaxRetries), maxTokenRefreshRetryBackoff)
	require.Equal(t, maxGrokOAuthReconcilePageSize, svc.grokOAuthReconcileMaxPageSize())
}

func TestTokenRefreshService_AttemptTimeoutStaysInsideDistributedLockLease(t *testing.T) {
	cache := &poolHealthTokenCacheStub{}
	svc := &TokenRefreshService{
		cfg:        &config.TokenRefreshConfig{AttemptTimeoutSeconds: int(maxTokenRefreshAttemptTimeout / time.Second)},
		refreshAPI: NewOAuthRefreshAPI(&poolHealthAccountRepo{}, cache),
	}

	require.Equal(t, 55*time.Second, svc.attemptTimeout())
	require.Less(t, svc.attemptTimeout(), defaultRefreshLockTTL)
}

func TestTokenRefreshService_SharedProviderFailureContainsCycleWithoutAccountMutation(t *testing.T) {
	accounts := make([]Account, 0, 5)
	for id := int64(1); id <= 5; id++ {
		accounts = append(accounts, grokPoolAccount(id))
	}
	repo := &poolHealthAccountRepo{pages: map[int64][]Account{0: accounts}}
	refresher := &poolHealthRefresher{err: errors.New("invalid_client: provider configuration rejected")}
	svc := newPoolHealthService(repo, refresher, config.TokenRefreshConfig{
		MaxRetries:               1,
		CandidatePageSize:        10,
		ProviderConcurrency:      4,
		ProviderQPS:              10000,
		ProviderFailureThreshold: 3,
		AttemptTimeoutSeconds:    1,
		CycleTimeoutSeconds:      2,
	})

	svc.processRefreshContext(context.Background())

	_, _, setErrorCalls, setTempUnschedCalls := repo.snapshot()
	require.Equal(t, int64(1), refresher.calls.Load(), "shared provider configuration failures must open the in-cycle breaker immediately")
	require.Zero(t, setErrorCalls, "shared provider failures must not mass-disable accounts")
	require.Zero(t, setTempUnschedCalls, "shared provider failures must not mutate per-account scheduling state")
}

func TestTokenRefreshService_SharedDBRereadFailureContainsCycleWithoutAccountMutation(t *testing.T) {
	accounts := []Account{grokPoolAccount(1), grokPoolAccount(2), grokPoolAccount(3)}
	repo := &poolHealthAccountRepo{
		pages:      map[int64][]Account{0: accounts},
		getByIDErr: errors.New("database unavailable"),
	}
	refresher := &poolHealthRefresher{}
	svc := newPoolHealthService(repo, refresher, config.TokenRefreshConfig{
		MaxRetries:            3,
		CandidatePageSize:     10,
		ProviderConcurrency:   4,
		ProviderQPS:           10000,
		AttemptTimeoutSeconds: 1,
		CycleTimeoutSeconds:   2,
	})
	svc.refreshAPI = NewOAuthRefreshAPI(repo, nil)

	svc.processRefreshContext(context.Background())

	_, _, setErrorCalls, setTempUnschedCalls := repo.snapshot()
	require.Zero(t, refresher.calls.Load(), "refresh must fail closed before using stale account credentials")
	require.Zero(t, setErrorCalls)
	require.Zero(t, setTempUnschedCalls, "a shared DB outage must not mutate the selected account")
}

func TestTokenRefreshService_GenericGrokForbiddenContainsCycleWithoutAccountMutation(t *testing.T) {
	accounts := []Account{grokPoolAccount(1), grokPoolAccount(2), grokPoolAccount(3)}
	repo := &poolHealthAccountRepo{pages: map[int64][]Account{0: accounts}}
	refresher := &poolHealthRefresher{err: errors.New(`GROK_OAUTH_ENTITLEMENT_DENIED: token refresh failed: status 403, body: <html>request blocked</html>`)}
	svc := newPoolHealthService(repo, refresher, config.TokenRefreshConfig{
		MaxRetries:               1,
		CandidatePageSize:        10,
		ProviderConcurrency:      4,
		ProviderQPS:              10000,
		ProviderFailureThreshold: 3,
		AttemptTimeoutSeconds:    1,
		CycleTimeoutSeconds:      2,
	})

	svc.processRefreshContext(context.Background())

	_, _, setErrorCalls, setTempUnschedCalls := repo.snapshot()
	require.Equal(t, int64(1), refresher.calls.Load(), "an ambiguous Grok 403 must contain the provider immediately")
	require.Zero(t, setErrorCalls, "a generic 403 is not evidence that an account credential is permanently invalid")
	require.Zero(t, setTempUnschedCalls, "provider containment must not mutate account scheduling state")
}

func TestTokenRefreshService_ExplicitGrokEntitlementDenialIsPermanent(t *testing.T) {
	repo := &poolHealthAccountRepo{pages: map[int64][]Account{0: {grokPoolAccount(1)}}}
	refresher := &poolHealthRefresher{err: errors.New(`GROK_OAUTH_ENTITLEMENT_DENIED: token refresh failed: status 403, body: {"error":"subscription required"}`)}
	svc := newPoolHealthService(repo, refresher, config.TokenRefreshConfig{
		MaxRetries:            1,
		CandidatePageSize:     10,
		ProviderConcurrency:   1,
		ProviderQPS:           10000,
		AttemptTimeoutSeconds: 1,
		CycleTimeoutSeconds:   2,
	})

	svc.processRefreshContext(context.Background())

	_, _, setErrorCalls, setTempUnschedCalls := repo.snapshot()
	require.Equal(t, int64(1), refresher.calls.Load())
	require.Equal(t, 1, setErrorCalls, "explicit entitlement evidence is an account-permanent failure")
	require.Zero(t, setTempUnschedCalls)
}

func TestTokenRefreshService_AttemptTimeoutTripsRetryableProviderThreshold(t *testing.T) {
	accounts := []Account{grokPoolAccount(1), grokPoolAccount(2), grokPoolAccount(3)}
	repo := &poolHealthAccountRepo{pages: map[int64][]Account{0: accounts}}
	refresher := &poolHealthRefresher{delay: time.Second}
	svc := newPoolHealthService(repo, refresher, config.TokenRefreshConfig{
		MaxRetries:               1,
		CandidatePageSize:        10,
		ProviderConcurrency:      1,
		ProviderQPS:              10000,
		ProviderFailureThreshold: 2,
		CycleTimeoutSeconds:      2,
	})
	svc.attemptTimeoutOverride = 20 * time.Millisecond

	svc.processRefreshContext(context.Background())

	_, _, setErrorCalls, setTempUnschedCalls := repo.snapshot()
	require.Equal(t, int64(2), refresher.calls.Load(), "two attempt timeouts should trip the retryable provider threshold")
	require.Zero(t, setErrorCalls)
	require.Equal(t, 2, setTempUnschedCalls, "attempt timeouts remain account-transient failures before containment opens")
}

func TestTokenRefreshService_ParentCancellationStopsRetryWithoutAccountMutation(t *testing.T) {
	repo := &poolHealthAccountRepo{}
	ctx, cancel := context.WithCancel(context.Background())
	refresher := &poolHealthRefresher{
		err:    errors.New("temporary provider failure"),
		cancel: cancel,
	}
	svc := newPoolHealthService(repo, refresher, config.TokenRefreshConfig{
		MaxRetries:            3,
		RetryBackoffSeconds:   1,
		AttemptTimeoutSeconds: 1,
	})
	account := grokPoolAccount(42)

	err := svc.refreshWithRetry(ctx, &account, refresher, nil, time.Hour)

	require.ErrorIs(t, err, context.Canceled)
	_, _, setErrorCalls, setTempUnschedCalls := repo.snapshot()
	require.Zero(t, setErrorCalls)
	require.Zero(t, setTempUnschedCalls)
}

func TestTokenRefreshService_LateSuccessPastAttemptDeadlineIsRejected(t *testing.T) {
	repo := &poolHealthAccountRepo{}
	refresher := &poolHealthRefresher{
		delay:         30 * time.Millisecond,
		ignoreContext: true,
	}
	svc := newPoolHealthService(repo, refresher, config.TokenRefreshConfig{MaxRetries: 1})
	svc.attemptTimeoutOverride = 10 * time.Millisecond
	account := grokPoolAccount(43)

	err := svc.refreshWithRetry(context.Background(), &account, refresher, nil, time.Hour)

	var timeoutErr *refreshAttemptTimeoutError
	require.ErrorAs(t, err, &timeoutErr)
	_, updatedIDs, setErrorCalls, setTempUnschedCalls := repo.snapshot()
	require.Empty(t, updatedIDs, "credentials returned after the deadline must not be persisted")
	require.Zero(t, setErrorCalls)
	require.Equal(t, 1, setTempUnschedCalls)
}

func TestTokenRefreshService_NonRetryableGrokFailureInvalidatesTokenCache(t *testing.T) {
	repo := &poolHealthAccountRepo{}
	invalidator := &reconcileInvalidator{}
	refresher := &poolHealthRefresher{err: errors.New("invalid_grant: revoked")}
	svc := newPoolHealthService(repo, refresher, config.TokenRefreshConfig{MaxRetries: 1})
	svc.cacheInvalidator = invalidator
	account := grokPoolAccount(77)

	err := svc.refreshWithRetry(context.Background(), &account, refresher, nil, time.Hour)

	require.Error(t, err)
	_, _, setErrorCalls, setTempUnschedCalls := repo.snapshot()
	require.Equal(t, 1, setErrorCalls)
	require.Zero(t, setTempUnschedCalls)
	require.Equal(t, 1, invalidator.count())
}
