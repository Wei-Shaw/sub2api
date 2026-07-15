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
REDACTED

func (r *poolHealthAccountRepo) GetByID(_ context.Context, _ int64) (*Account, error) {
	if r.getByIDErr != nil {
		return nil, r.getByIDErr
REDACTED
	return nil, ErrAccountNotFound
REDACTED

func (r *poolHealthAccountRepo) ListOAuthRefreshCandidatePage(_ context.Context, options OAuthRefreshPageOptions) (*OAuthRefreshCandidatePage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, options)
	accounts := append([]Account(nil), r.pages[options.AfterID]...)
	page := &OAuthRefreshCandidatePage{Accounts: accounts, HasMore: len(accounts) == options.LimitREDACTED
	if len(accounts) > 0 {
		page.NextAfterID = accounts[len(accounts)-1].ID
REDACTED
	return page, nil
REDACTED

func (r *poolHealthAccountRepo) UpdateCredentials(_ context.Context, id int64, _ map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updatedCredentialIDs = append(r.updatedCredentialIDs, id)
	return nil
REDACTED

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
REDACTED

func (r *poolHealthAccountRepo) SetError(context.Context, int64, string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.setErrorCalls++
	return nil
REDACTED

func (r *poolHealthAccountRepo) SetGrokOAuthErrorIfCredentialsUnchanged(context.Context, int64, map[string]any, string) (bool, error) {
	return false, nil
REDACTED

func (r *poolHealthAccountRepo) SetGrokOAuthRefreshErrorIfCredentialsUnchanged(context.Context, int64, map[string]any, *int64, string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.setErrorCalls++
	return true, nil
REDACTED

func (r *poolHealthAccountRepo) SetGrokOAuthRefreshTempUnschedulableIfCredentialsUnchanged(context.Context, int64, map[string]any, *int64, time.Time, string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.setTempUnschedCalls++
	return true, nil
REDACTED

func (r *poolHealthAccountRepo) SetTempUnschedulable(context.Context, int64, time.Time, string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.setTempUnschedCalls++
	return nil
REDACTED

func (r *poolHealthAccountRepo) snapshot() ([]OAuthRefreshPageOptions, []int64, int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]OAuthRefreshPageOptions(nil), r.requests...), append([]int64(nil), r.updatedCredentialIDs...), r.setErrorCalls, r.setTempUnschedCalls
REDACTED

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
REDACTED

type countingRefreshAttemptGate struct {
	calls atomic.Int64
REDACTED

type rejectedRefreshAttemptGate struct {
	err error
REDACTED

type poolHealthTokenCacheStub struct {
	GeminiTokenCache
REDACTED

type tripBeforeRateAdmissionGate struct {
	state *tokenRefreshProviderState
REDACTED

func (g *tripBeforeRateAdmissionGate) acquire(ctx context.Context) (func(), error) {
	release, err := g.state.acquire(ctx)
	if err != nil {
		return nil, err
REDACTED
	g.state.mu.Lock()
	g.state.tripped = true
	g.state.mu.Unlock()
	return release, nil
REDACTED

func (g *tripBeforeRateAdmissionGate) acquireRate(ctx context.Context) (func(), error) {
	return g.state.acquireRate(ctx)
REDACTED

type breakerTripAccountRepo struct {
	*productionPathRateRepo
	setErrorCalls atomic.Int64
	setTempCalls  atomic.Int64
REDACTED

func (r *breakerTripAccountRepo) SetGrokOAuthRefreshErrorIfCredentialsUnchanged(context.Context, int64, map[string]any, *int64, string) (bool, error) {
	r.setErrorCalls.Add(1)
	return true, nil
REDACTED

func (r *breakerTripAccountRepo) SetGrokOAuthRefreshTempUnschedulableIfCredentialsUnchanged(context.Context, int64, map[string]any, *int64, time.Time, string) (bool, error) {
	r.setTempCalls.Add(1)
	return true, nil
REDACTED

func (g *rejectedRefreshAttemptGate) acquire(context.Context) (func(), error) {
	return nil, g.err
REDACTED

type productionPathRateRepo struct {
	AccountRepository

	mu       sync.Mutex
	accounts map[int64]*Account
REDACTED

func (r *productionPathRateRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	account := r.accounts[id]
	if account == nil {
		return nil, ErrAccountNotFound
REDACTED
	return snapshotOAuthRefreshAccount(account), nil
REDACTED

func (r *productionPathRateRepo) UpdateCredentials(_ context.Context, id int64, credentials map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	account := r.accounts[id]
	if account == nil {
		return ErrAccountNotFound
REDACTED
	account.Credentials = shallowCopyMap(credentials)
	return nil
REDACTED

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
REDACTED
	account.Credentials = shallowCopyMap(credentials)
	return true, nil
REDACTED

type productionPathRefreshStart struct {
	accountID int64
	at        time.Time
REDACTED

type productionPathRateExecutor struct {
	firstStarted chan struct{REDACTED
	releaseFirst chan struct{REDACTED
	calls        atomic.Int64
	startMu      sync.Mutex
	starts       []productionPathRefreshStart
REDACTED

func (e *productionPathRateExecutor) CacheKey(account *Account) string {
	return fmt.Sprintf("production-path-rate:%d", account.ID)
REDACTED

func (e *productionPathRateExecutor) CanRefresh(account *Account) bool {
	return account != nil && account.IsGrokOAuth()
REDACTED

func (e *productionPathRateExecutor) NeedsRefresh(account *Account, _ time.Duration) bool {
	needsRefresh, _ := account.Credentials["needs_refresh"].(bool)
	return needsRefresh
REDACTED

func (e *productionPathRateExecutor) Refresh(ctx context.Context, account *Account) (map[string]any, error) {
	call := e.calls.Add(1)
	e.startMu.Lock()
	e.starts = append(e.starts, productionPathRefreshStart{accountID: account.ID, at: time.Now()REDACTED)
	e.startMu.Unlock()
	if call == 1 {
		close(e.firstStarted)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-e.releaseFirst:
	REDACTED
REDACTED
	return map[string]any{
		"access_token":  fmt.Sprintf("fresh-access-%d", account.ID),
		"refresh_token": fmt.Sprintf("fresh-refresh-%d", account.ID),
		"needs_refresh": false,
REDACTED, nil
REDACTED

func (e *productionPathRateExecutor) startsSnapshot() []productionPathRefreshStart {
	e.startMu.Lock()
	defer e.startMu.Unlock()
	return append([]productionPathRefreshStart(nil), e.starts...)
REDACTED

func (g *countingRefreshAttemptGate) acquire(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
REDACTED
	g.calls.Add(1)
	return func() {REDACTED, nil
REDACTED

func (r *poolHealthRefresher) CacheKey(account *Account) string {
	return fmt.Sprintf("pool-health:%d", account.ID)
REDACTED

func (r *poolHealthRefresher) CanRefresh(account *Account) bool {
	return account != nil && account.Platform == PlatformGrok && account.Type == AccountTypeOAuth
REDACTED

func (r *poolHealthRefresher) NeedsRefresh(*Account, time.Duration) bool { return true REDACTED

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
REDACTED
	r.startMu.Unlock()
	for {
		maxActive := r.maxActive.Load()
		if active <= maxActive || r.maxActive.CompareAndSwap(maxActive, active) {
			break
	REDACTED
REDACTED
	if r.cancel != nil {
		r.cancel()
REDACTED
	if delay > 0 {
		if r.ignoreContext {
			time.Sleep(delay)
	REDACTED else {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-timer.C:
		REDACTED
	REDACTED
REDACTED
	if r.err != nil {
		return nil, r.err
REDACTED
	if r.newCredentials != nil {
		credentials := make(map[string]any, len(r.newCredentials))
		for key, value := range r.newCredentials {
			credentials[key] = value
	REDACTED
		return credentials, nil
REDACTED
	return map[string]any{"access_token": "new-token", "refresh_token": "new-refresh-token"REDACTED, nil
REDACTED

func (r *poolHealthRefresher) startsSnapshot() []time.Time {
	r.startMu.Lock()
	defer r.startMu.Unlock()
	return append([]time.Time(nil), r.startTimes...)
REDACTED

func grokPoolAccount(id int64) Account {
	return Account{
		ID:       id,
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
REDACTED
			"access_token":  "old-token",
			"refresh_token": "refresh-token",
	REDACTED,
REDACTED
REDACTED

func newPoolHealthService(repo *poolHealthAccountRepo, refresher *poolHealthRefresher, cfg config.TokenRefreshConfig) *TokenRefreshService {
	return &TokenRefreshService{
		accountRepo:    repo,
		candidatePager: repo,
		registrations: []tokenRefreshRegistration{{
			platform:  PlatformGrok,
			refresher: refresher,
			executor:  refresher,
REDACTED
		refreshPolicy: DefaultBackgroundRefreshPolicy(),
		cfg:           &cfg,
REDACTED
REDACTED

func TestTokenRefreshService_RegistrationsAreCandidateEligibilitySource(t *testing.T) {
	cfg := &config.Config{REDACTED
	svc := NewTokenRefreshService(nil, nil, nil, nil, nil, nil, nil, cfg, nil)

	require.Equal(t, []string{
		PlatformAnthropic,
		PlatformOpenAI,
		PlatformGemini,
		PlatformAntigravity,
		PlatformGrok,
REDACTED, svc.eligiblePlatforms())
	require.Len(t, svc.registrations, 5)
	for _, registration := range svc.registrations {
		require.NotNil(t, registration.refresher)
		require.NotNil(t, registration.executor)
REDACTED
REDACTED

func TestTokenRefreshService_ProcessRefreshPagesByStableCursor(t *testing.T) {
	repo := &poolHealthAccountRepo{pages: map[int64][]Account{
		0: {grokPoolAccount(1), grokPoolAccount(2)REDACTED,
		2: {grokPoolAccount(3)REDACTED,
REDACTEDREDACTED
	refresher := &poolHealthRefresher{REDACTED
	svc := newPoolHealthService(repo, refresher, config.TokenRefreshConfig{
		RefreshBeforeExpiryHours: 1,
		MaxRetries:               1,
		CandidatePageSize:        2,
		ProviderConcurrency:      4,
		ProviderQPS:              10000,
		AttemptTimeoutSeconds:    1,
		CycleTimeoutSeconds:      2,
REDACTED)

	svc.processRefreshContext(context.Background())

	requests, updatedIDs, _, _ := repo.snapshot()
	require.Len(t, requests, 2)
	require.Equal(t, int64(0), requests[0].AfterID)
	require.Equal(t, int64(2), requests[1].AfterID)
	require.Equal(t, []string{PlatformGrokREDACTED, requests[0].Platforms)
	require.True(t, requests[0].ActiveOnly)
	require.True(t, requests[0].RequireRefreshToken)
	require.True(t, requests[0].ExcludeRetryCooldown)
	sort.Slice(updatedIDs, func(i, j int) bool { return updatedIDs[i] < updatedIDs[j] REDACTED)
	require.Equal(t, []int64{1, 2, 3REDACTED, updatedIDs)
	require.Zero(t, svc.candidateAfterID(), "a short final page must wrap the next cycle to the beginning")
REDACTED

func TestTokenRefreshService_BoundsPerProviderConcurrency(t *testing.T) {
	accounts := make([]Account, 0, 8)
	for id := int64(1); id <= 8; id++ {
		accounts = append(accounts, grokPoolAccount(id))
REDACTED
	repo := &poolHealthAccountRepo{pages: map[int64][]Account{0: accountsREDACTEDREDACTED
	refresher := &poolHealthRefresher{delay: 20 * time.MillisecondREDACTED
	svc := newPoolHealthService(repo, refresher, config.TokenRefreshConfig{
		MaxRetries:            1,
		CandidatePageSize:     20,
		ProviderConcurrency:   2,
		ProviderQPS:           10000,
		AttemptTimeoutSeconds: 1,
		CycleTimeoutSeconds:   2,
REDACTED)

	svc.processRefreshContext(context.Background())

	require.Equal(t, int64(8), refresher.calls.Load())
	require.Equal(t, int64(2), refresher.maxActive.Load())
REDACTED

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
REDACTED

func TestTokenRefreshService_RetriesAcquireRateSlotPerAttempt(t *testing.T) {
	repo := &poolHealthAccountRepo{REDACTED
	refresher := &poolHealthRefresher{err: errors.New("temporary provider failure")REDACTED
	svc := newPoolHealthService(repo, refresher, config.TokenRefreshConfig{MaxRetries: 3REDACTED)
	gate := &countingRefreshAttemptGate{REDACTED
	account := grokPoolAccount(44)

	err := svc.refreshWithRetryWithRateGate(context.Background(), &account, refresher, nil, time.Hour, gate)

REDACTED
	require.Equal(t, int64(3), refresher.calls.Load())
	require.Equal(t, int64(3), gate.calls.Load(), "every upstream retry must consume a provider rate slot")
REDACTED

func TestTokenRefreshService_ProcessProviderAccountsLegacyNilReleaseGateIsSafe(t *testing.T) {
	repo := &poolHealthAccountRepo{REDACTED
	refresher := &poolHealthRefresher{REDACTED
	svc := newPoolHealthService(repo, refresher, config.TokenRefreshConfig{
		MaxRetries:          1,
		ProviderConcurrency: 1,
REDACTED)
	state := &tokenRefreshProviderState{
		service: svc,
		registration: tokenRefreshRegistration{
			platform:  PlatformGrok,
			refresher: refresher,
			// nil executor deliberately exercises the legacy/direct fallback.
			executor: nil,
	REDACTED,
		// Admission rejection validly returns no release callback. The direct
		// fallback must propagate the skip without dereferencing that nil handle.
		rateGate: &rejectedRefreshAttemptGate{err: errRefreshSkippedREDACTED,
		poolGate: nil,
REDACTED
	account := grokPoolAccount(45)

	refreshed, skipped, failed := svc.processProviderAccounts(
		context.Background(),
		state,
		[]*Account{&accountREDACTED,
		time.Hour,
	)

	require.Zero(t, refreshed)
	require.Equal(t, 1, skipped)
	require.Zero(t, failed)
	require.Zero(t, refresher.calls.Load(), "rejected rate admission must not reach the legacy upstream refresher")
REDACTED

func TestTokenRefreshService_ProviderRateGateIsSharedAcrossRuns(t *testing.T) {
	svc := &TokenRefreshService{cfg: &config.TokenRefreshConfig{ProviderQPS: 40REDACTEDREDACTED
	first := svc.providerRateGate(PlatformGrok)
	second := svc.providerRateGate(PlatformGrok)
	require.Same(t, first, second, "background cycles and reconciliation must share the process-local provider limiter")

	base := time.Unix(1_700_000_000, 0)
	require.Equal(t, base, first.reserveSlot(base))
	require.Equal(t, base.Add(25*time.Millisecond), second.reserveSlot(base))
REDACTED

func TestTokenRefreshService_ProviderConcurrencyGateIsSharedAcrossBackgroundAndConcurrentAdminReconciliation(t *testing.T) {
	accounts := []Account{
		grokPoolAccount(1),
		grokPoolAccount(2),
		grokPoolAccount(3),
		grokPoolAccount(4),
REDACTED
	repo := &poolHealthAccountRepo{pages: map[int64][]Account{0: accountsREDACTEDREDACTED
	refresher := &poolHealthRefresher{delay: 80 * time.MillisecondREDACTED
	svc := newPoolHealthService(repo, refresher, config.TokenRefreshConfig{
		RefreshBeforeExpiryHours: 1,
		MaxRetries:               1,
		CandidatePageSize:        20,
		ProviderConcurrency:      2,
		ProviderQPS:              100,
		ProviderFailureThreshold: 20,
		AttemptTimeoutSeconds:    1,
		CycleTimeoutSeconds:      3,
REDACTED)

	firstGate := svc.providerConcurrencyGate(PlatformGrok)
	require.Same(t, firstGate, svc.providerConcurrencyGate(PlatformGrok))

	start := make(chan struct{REDACTED)
	adminErrors := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		<-start
		svc.processRefreshContext(context.Background())
REDACTED()
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			<-start
			_, err := svc.ReconcileGrokOAuth(context.Background(), GrokOAuthReconcileInput{Apply: true, Limit: 20REDACTED)
			adminErrors <- err
	REDACTED()
REDACTED
	close(start)
	wg.Wait()
	close(adminErrors)

	for err := range adminErrors {
	REDACTED
REDACTED
	require.Equal(t, int64(12), refresher.calls.Load(), "background and both admin calls must all execute")
	require.Equal(t, int64(2), refresher.maxActive.Load(),
		"all entry points must share the configured per-provider upstream concurrency cap")
REDACTED

func TestTokenRefreshService_SaturatedProviderPreservesConcurrencyAndActualQPSStartSpacing(t *testing.T) {
	const (
		providerConcurrency = 2
		providerQPS         = 20
		attemptCount        = 8
	)
	repo := &poolHealthAccountRepo{REDACTED
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
	REDACTED,
REDACTED
	svc := newPoolHealthService(repo, refresher, config.TokenRefreshConfig{
		MaxRetries:            1,
		ProviderConcurrency:   providerConcurrency,
		ProviderQPS:           providerQPS,
		AttemptTimeoutSeconds: 1,
REDACTED)
	registration := svc.registrations[0]
	sharedRateGate := svc.providerRateGate(PlatformGrok)
	sharedPoolGate := svc.providerConcurrencyGate(PlatformGrok)

	start := make(chan struct{REDACTED)
	errorsCh := make(chan error, attemptCount)
	var wg sync.WaitGroup
	for i := 0; i < attemptCount; i++ {
		account := grokPoolAccount(int64(i + 1))
		state := &tokenRefreshProviderState{
			service:      svc,
			registration: registration,
			rateGate:     sharedRateGate,
			poolGate:     sharedPoolGate,
	REDACTED
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errorsCh <- svc.refreshWithRetryWithRateGate(context.Background(), &account, refresher, nil, time.Hour, state)
	REDACTED()
REDACTED
	close(start)
	wg.Wait()
	close(errorsCh)

	for err := range errorsCh {
	REDACTED
REDACTED
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
	REDACTED
		require.GreaterOrEqualf(t, spacing, minimumObservedSpacing,
			"upstream starts %d and %d violated configured QPS spacing", i-1, i)
REDACTED
	t.Logf("max_active=%d configured_concurrency=%d minimum_start_spacing=%s configured_spacing=%s",
		refresher.maxActive.Load(), providerConcurrency, actualMinimumSpacing, configuredSpacing)
REDACTED

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
REDACTEDREDACTED
	executor := &productionPathRateExecutor{
		firstStarted: make(chan struct{REDACTED),
		releaseFirst: make(chan struct{REDACTED),
REDACTED
	svc := &TokenRefreshService{
		accountRepo:            repo,
		refreshAPI:             NewOAuthRefreshAPI(repo, nil),
		refreshPolicy:          DefaultBackgroundRefreshPolicy(),
		cfg:                    &config.TokenRefreshConfig{MaxRetries: 1REDACTED,
		attemptTimeoutOverride: 2 * time.Second,
REDACTED
	state := &tokenRefreshProviderState{
		service:  svc,
		rateGate: newTokenRefreshRateGateWithInterval(interval),
		poolGate: newTokenRefreshConcurrencyGate(2),
REDACTED

	errorsCh := make(chan error, 3)
	go func() {
		errorsCh <- svc.refreshWithRetryWithRateGate(context.Background(), firstSelection, executor, executor, time.Hour, state)
REDACTED()
	select {
	case <-executor.firstStarted:
	case <-time.After(time.Second):
		require.FailNow(t, "first production-path refresh did not reach the upstream executor")
REDACTED

	go func() {
		errorsCh <- svc.refreshWithRetryWithRateGate(context.Background(), contendingSelection, executor, executor, time.Hour, state)
REDACTED()
	require.Eventually(t, func() bool {
		return len(state.poolGate.slots) == 2
REDACTED, time.Second, time.Millisecond, "same-account contender must hold the second provider slot while waiting on the local refresh lock")

	go func() {
		errorsCh <- svc.refreshWithRetryWithRateGate(context.Background(), differentSelection, executor, executor, time.Hour, state)
REDACTED()
	close(executor.releaseFirst)

	skipped := 0
	for i := 0; i < 3; i++ {
		err := <-errorsCh
		if errors.Is(err, errRefreshSkipped) {
			skipped++
			continue
	REDACTED
	REDACTED
REDACTED
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
REDACTED

func TestTokenRefreshService_ProviderTripBeforeRateAdmissionSkipsWithoutAccountMutation(t *testing.T) {
	account := grokPoolAccount(73)
	stored := snapshotOAuthRefreshAccount(&account)
	repo := &breakerTripAccountRepo{productionPathRateRepo: &productionPathRateRepo{
		accounts: map[int64]*Account{account.ID: storedREDACTED,
REDACTEDREDACTED
	refresher := &poolHealthRefresher{REDACTED
	svc := &TokenRefreshService{
		accountRepo:   repo,
		refreshAPI:    NewOAuthRefreshAPI(repo, nil),
		refreshPolicy: DefaultBackgroundRefreshPolicy(),
		cfg:           &config.TokenRefreshConfig{MaxRetries: 1REDACTED,
REDACTED
	state := &tokenRefreshProviderState{
		service:  svc,
		rateGate: newTokenRefreshRateGate(1),
		poolGate: newTokenRefreshConcurrencyGate(1),
REDACTED
	gate := &tripBeforeRateAdmissionGate{state: stateREDACTED

	err := svc.refreshWithRetryWithRateGate(context.Background(), &account, refresher, refresher, time.Hour, gate)

	require.ErrorIs(t, err, errRefreshSkipped)
	require.Zero(t, refresher.calls.Load(), "a tripped provider must not reach upstream rate admission")
	require.Zero(t, repo.setErrorCalls.Load())
	require.Zero(t, repo.setTempCalls.Load(), "provider skip must never fall through to per-account cooldown")
REDACTED

func TestTokenRefreshService_ConfigBounds(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	svc := &TokenRefreshService{cfg: &config.TokenRefreshConfig{
		MaxRetries:               maxInt,
		RetryBackoffSeconds:      maxInt,
		ProviderFailureThreshold: maxInt,
		AttemptTimeoutSeconds:    maxInt,
		CycleTimeoutSeconds:      maxInt,
REDACTEDREDACTED

	require.Equal(t, maxTokenRefreshMaxRetries, svc.maxRetries())
	require.Equal(t, maxTokenRefreshProviderFailureThreshold, svc.providerFailureThreshold())
	require.Equal(t, maxTokenRefreshAttemptTimeout, svc.attemptTimeout())
	require.Equal(t, maxTokenRefreshCycleTimeout, svc.cycleTimeout())
	require.LessOrEqual(t, svc.retryBackoff(1, maxTokenRefreshMaxRetries), maxTokenRefreshRetryBackoff)
	require.Equal(t, maxGrokOAuthReconcilePageSize, svc.grokOAuthReconcileMaxPageSize())
REDACTED

func TestTokenRefreshService_AttemptTimeoutStaysInsideDistributedLockLease(t *testing.T) {
	cache := &poolHealthTokenCacheStub{REDACTED
	svc := &TokenRefreshService{
		cfg:        &config.TokenRefreshConfig{AttemptTimeoutSeconds: int(maxTokenRefreshAttemptTimeout / time.Second)REDACTED,
		refreshAPI: NewOAuthRefreshAPI(&poolHealthAccountRepo{REDACTED, cache),
REDACTED

	require.Equal(t, 55*time.Second, svc.attemptTimeout())
	require.Less(t, svc.attemptTimeout(), defaultRefreshLockTTL)
REDACTED

func TestTokenRefreshService_SharedProviderFailureContainsCycleWithoutAccountMutation(t *testing.T) {
	accounts := make([]Account, 0, 5)
	for id := int64(1); id <= 5; id++ {
		accounts = append(accounts, grokPoolAccount(id))
REDACTED
	repo := &poolHealthAccountRepo{pages: map[int64][]Account{0: accountsREDACTEDREDACTED
	refresher := &poolHealthRefresher{err: errors.New("invalid_client: provider configuration rejected")REDACTED
	svc := newPoolHealthService(repo, refresher, config.TokenRefreshConfig{
		MaxRetries:               1,
		CandidatePageSize:        10,
		ProviderConcurrency:      4,
		ProviderQPS:              10000,
		ProviderFailureThreshold: 3,
		AttemptTimeoutSeconds:    1,
		CycleTimeoutSeconds:      2,
REDACTED)

	svc.processRefreshContext(context.Background())

	_, _, setErrorCalls, setTempUnschedCalls := repo.snapshot()
	require.Equal(t, int64(1), refresher.calls.Load(), "shared provider configuration failures must open the in-cycle breaker immediately")
	require.Zero(t, setErrorCalls, "shared provider failures must not mass-disable accounts")
	require.Zero(t, setTempUnschedCalls, "shared provider failures must not mutate per-account scheduling state")
REDACTED

func TestTokenRefreshService_SharedDBRereadFailureContainsCycleWithoutAccountMutation(t *testing.T) {
	accounts := []Account{grokPoolAccount(1), grokPoolAccount(2), grokPoolAccount(3)REDACTED
	repo := &poolHealthAccountRepo{
		pages:      map[int64][]Account{0: accountsREDACTED,
		getByIDErr: errors.New("database unavailable"),
REDACTED
	refresher := &poolHealthRefresher{REDACTED
	svc := newPoolHealthService(repo, refresher, config.TokenRefreshConfig{
		MaxRetries:            3,
		CandidatePageSize:     10,
		ProviderConcurrency:   4,
		ProviderQPS:           10000,
		AttemptTimeoutSeconds: 1,
		CycleTimeoutSeconds:   2,
REDACTED)
	svc.refreshAPI = NewOAuthRefreshAPI(repo, nil)

	svc.processRefreshContext(context.Background())

	_, _, setErrorCalls, setTempUnschedCalls := repo.snapshot()
	require.Zero(t, refresher.calls.Load(), "refresh must fail closed before using stale account credentials")
	require.Zero(t, setErrorCalls)
	require.Zero(t, setTempUnschedCalls, "a shared DB outage must not mutate the selected account")
REDACTED

func TestTokenRefreshService_GenericGrokForbiddenContainsCycleWithoutAccountMutation(t *testing.T) {
	accounts := []Account{grokPoolAccount(1), grokPoolAccount(2), grokPoolAccount(3)REDACTED
	repo := &poolHealthAccountRepo{pages: map[int64][]Account{0: accountsREDACTEDREDACTED
	refresher := &poolHealthRefresher{err: errors.New(`GROK_OAUTH_ENTITLEMENT_DENIED: token refresh failed: status 403, body: <html>request blocked</html>`)REDACTED
	svc := newPoolHealthService(repo, refresher, config.TokenRefreshConfig{
		MaxRetries:               1,
		CandidatePageSize:        10,
		ProviderConcurrency:      4,
		ProviderQPS:              10000,
		ProviderFailureThreshold: 3,
		AttemptTimeoutSeconds:    1,
		CycleTimeoutSeconds:      2,
REDACTED)

	svc.processRefreshContext(context.Background())

	_, _, setErrorCalls, setTempUnschedCalls := repo.snapshot()
	require.Equal(t, int64(1), refresher.calls.Load(), "an ambiguous Grok 403 must contain the provider immediately")
	require.Zero(t, setErrorCalls, "a generic 403 is not evidence that an account credential is permanently invalid")
	require.Zero(t, setTempUnschedCalls, "provider containment must not mutate account scheduling state")
REDACTED

func TestTokenRefreshService_ExplicitGrokEntitlementDenialIsPermanent(t *testing.T) {
	repo := &poolHealthAccountRepo{pages: map[int64][]Account{0: {grokPoolAccount(1)REDACTEDREDACTEDREDACTED
	refresher := &poolHealthRefresher{err: errors.New(`GROK_OAUTH_ENTITLEMENT_DENIED: token refresh failed: status 403, body: {"error":"subscription required"REDACTED`)REDACTED
	svc := newPoolHealthService(repo, refresher, config.TokenRefreshConfig{
		MaxRetries:            1,
		CandidatePageSize:     10,
		ProviderConcurrency:   1,
		ProviderQPS:           10000,
		AttemptTimeoutSeconds: 1,
		CycleTimeoutSeconds:   2,
REDACTED)

	svc.processRefreshContext(context.Background())

	_, _, setErrorCalls, setTempUnschedCalls := repo.snapshot()
	require.Equal(t, int64(1), refresher.calls.Load())
	require.Equal(t, 1, setErrorCalls, "explicit entitlement evidence is an account-permanent failure")
	require.Zero(t, setTempUnschedCalls)
REDACTED

func TestTokenRefreshService_AttemptTimeoutTripsRetryableProviderThreshold(t *testing.T) {
	accounts := []Account{grokPoolAccount(1), grokPoolAccount(2), grokPoolAccount(3)REDACTED
	repo := &poolHealthAccountRepo{pages: map[int64][]Account{0: accountsREDACTEDREDACTED
	refresher := &poolHealthRefresher{delay: time.SecondREDACTED
	svc := newPoolHealthService(repo, refresher, config.TokenRefreshConfig{
		MaxRetries:               1,
		CandidatePageSize:        10,
		ProviderConcurrency:      1,
		ProviderQPS:              10000,
		ProviderFailureThreshold: 2,
		CycleTimeoutSeconds:      2,
REDACTED)
	svc.attemptTimeoutOverride = 20 * time.Millisecond

	svc.processRefreshContext(context.Background())

	_, _, setErrorCalls, setTempUnschedCalls := repo.snapshot()
	require.Equal(t, int64(2), refresher.calls.Load(), "two attempt timeouts should trip the retryable provider threshold")
	require.Zero(t, setErrorCalls)
	require.Equal(t, 2, setTempUnschedCalls, "attempt timeouts remain account-transient failures before containment opens")
REDACTED

func TestTokenRefreshService_ParentCancellationStopsRetryWithoutAccountMutation(t *testing.T) {
	repo := &poolHealthAccountRepo{REDACTED
	ctx, cancel := context.WithCancel(context.Background())
	refresher := &poolHealthRefresher{
		err:    errors.New("temporary provider failure"),
		cancel: cancel,
REDACTED
	svc := newPoolHealthService(repo, refresher, config.TokenRefreshConfig{
		MaxRetries:            3,
		RetryBackoffSeconds:   1,
		AttemptTimeoutSeconds: 1,
REDACTED)
	account := grokPoolAccount(42)

	err := svc.refreshWithRetry(ctx, &account, refresher, nil, time.Hour)

	require.ErrorIs(t, err, context.Canceled)
	_, _, setErrorCalls, setTempUnschedCalls := repo.snapshot()
	require.Zero(t, setErrorCalls)
	require.Zero(t, setTempUnschedCalls)
REDACTED

func TestTokenRefreshService_LateSuccessPastAttemptDeadlineIsRejected(t *testing.T) {
	repo := &poolHealthAccountRepo{REDACTED
	refresher := &poolHealthRefresher{
		delay:         30 * time.Millisecond,
		ignoreContext: true,
REDACTED
	svc := newPoolHealthService(repo, refresher, config.TokenRefreshConfig{MaxRetries: 1REDACTED)
	svc.attemptTimeoutOverride = 10 * time.Millisecond
	account := grokPoolAccount(43)

	err := svc.refreshWithRetry(context.Background(), &account, refresher, nil, time.Hour)

	var timeoutErr *refreshAttemptTimeoutError
	require.ErrorAs(t, err, &timeoutErr)
	_, updatedIDs, setErrorCalls, setTempUnschedCalls := repo.snapshot()
	require.Empty(t, updatedIDs, "credentials returned after the deadline must not be persisted")
	require.Zero(t, setErrorCalls)
	require.Equal(t, 1, setTempUnschedCalls)
REDACTED

func TestTokenRefreshService_NonRetryableGrokFailureInvalidatesTokenCache(t *testing.T) {
	repo := &poolHealthAccountRepo{REDACTED
	invalidator := &reconcileInvalidator{REDACTED
	refresher := &poolHealthRefresher{err: errors.New("invalid_grant: revoked")REDACTED
	svc := newPoolHealthService(repo, refresher, config.TokenRefreshConfig{MaxRetries: 1REDACTED)
	svc.cacheInvalidator = invalidator
	account := grokPoolAccount(77)

	err := svc.refreshWithRetry(context.Background(), &account, refresher, nil, time.Hour)

REDACTED
	_, _, setErrorCalls, setTempUnschedCalls := repo.snapshot()
	require.Equal(t, 1, setErrorCalls)
	require.Zero(t, setTempUnschedCalls)
	require.Equal(t, 1, invalidator.count())
REDACTED
