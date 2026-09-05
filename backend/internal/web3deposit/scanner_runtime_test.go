package web3deposit

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestScannerRuntimeAcquiresRenewsScansAndReleasesLease(t *testing.T) {
	clock := &scannerRuntimeTestClock{now: time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC)}
	leases := &scannerRuntimeLeaseStoreStub{acquireResults: []bool{true}, renewResults: []bool{true}}
	scanner := &scannerRunnerStub{calls: make(chan scannerRunnerCall, 4)}
	waits := make(chan scannerRuntimeWait, 4)
	runtime := newTestScannerRuntime(t, scanner, leases, clock, waits)

	require.NoError(t, runtime.Start(context.Background()))
	require.Equal(t, "lease-token-01", (<-scanner.calls).token)
	firstWait := <-waits
	require.Equal(t, 5*time.Second, firstWait.duration)
	clock.Advance(firstWait.duration)
	close(firstWait.resume)
	require.Equal(t, "lease-token-01", (<-scanner.calls).token)
	require.Eventually(t, func() bool { return leases.RenewCalls() == 1 }, time.Second, time.Millisecond)

	runtime.Stop()
	require.Equal(t, 1, leases.InitializeCalls())
	require.Equal(t, 1, leases.AcquireCalls())
	require.Equal(t, 1, leases.ReleaseCalls())
	require.Equal(t, ScannerRuntimeStateStopped, runtime.Status().State)
}

func TestScannerRuntimeStaysStandbyWithoutLease(t *testing.T) {
	clock := &scannerRuntimeTestClock{now: time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC)}
	leases := &scannerRuntimeLeaseStoreStub{acquireResults: []bool{false}}
	scanner := &scannerRunnerStub{calls: make(chan scannerRunnerCall, 1)}
	waits := make(chan scannerRuntimeWait, 2)
	runtime := newTestScannerRuntime(t, scanner, leases, clock, waits)

	require.NoError(t, runtime.Start(context.Background()))
	wait := <-waits
	require.Equal(t, 3*time.Second, wait.duration)
	require.Equal(t, ScannerRuntimeStateStandby, runtime.Status().State)
	select {
	case <-scanner.calls:
		t.Fatal("scanner ran without holding the lease")
	default:
	}
	runtime.Stop()
	require.Zero(t, leases.ReleaseCalls())
}

func TestScannerRuntimeStopsLeaseSessionWhenRenewalIsLost(t *testing.T) {
	clock := &scannerRuntimeTestClock{now: time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC)}
	leases := &scannerRuntimeLeaseStoreStub{acquireResults: []bool{true}, renewResults: []bool{false}}
	scanner := &scannerRunnerStub{calls: make(chan scannerRunnerCall, 2)}
	waits := make(chan scannerRuntimeWait, 4)
	runtime := newTestScannerRuntime(t, scanner, leases, clock, waits)

	require.NoError(t, runtime.Start(context.Background()))
	<-scanner.calls
	wait := <-waits
	clock.Advance(wait.duration)
	close(wait.resume)
	nextWait := <-waits
	require.Equal(t, 3*time.Second, nextWait.duration)
	require.Eventually(t, func() bool { return leases.RenewCalls() == 1 }, time.Second, time.Millisecond)
	require.Equal(t, 1, leases.ReleaseCalls())
	select {
	case <-scanner.calls:
		t.Fatal("scanner continued after lease renewal was rejected")
	default:
	}

	runtime.Stop()
}

func TestScannerRuntimeRecordsScannerErrorsAndRetries(t *testing.T) {
	clock := &scannerRuntimeTestClock{now: time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC)}
	leases := &scannerRuntimeLeaseStoreStub{acquireResults: []bool{true}}
	wantErr := errors.New("rpc unavailable")
	scanner := &scannerRunnerStub{err: wantErr, calls: make(chan scannerRunnerCall, 2)}
	waits := make(chan scannerRuntimeWait, 2)
	runtime := newTestScannerRuntime(t, scanner, leases, clock, waits)

	require.NoError(t, runtime.Start(context.Background()))
	<-scanner.calls
	require.Eventually(t, func() bool { return leases.RecordErrorCalls() == 1 }, time.Second, time.Millisecond)
	require.ErrorContains(t, errors.New(runtime.Status().LastError), wantErr.Error())

	runtime.Stop()
}

func TestScannerRuntimeStopCancelsActiveScan(t *testing.T) {
	leases := &scannerRuntimeLeaseStoreStub{acquireResults: []bool{true}}
	started := make(chan struct{})
	scanner := scannerRunnerFunc(func(ctx context.Context, _ string, _ time.Time) (ScannerResult, error) {
		close(started)
		<-ctx.Done()
		return ScannerResult{}, ctx.Err()
	})
	runtime, err := NewScannerRuntime(scanner, leases, ScannerRuntimeOptions{
		Enabled:         true,
		ScannerKey:      "conflux_espace_mainnet:usdt0",
		Owner:           "scanner-instance-01",
		ChainID:         1030,
		TokenContract:   "0xaf37e8b6c9ed7f6318979f56fc287d76c30847ff",
		PollInterval:    time.Second,
		LeaseTTL:        time.Minute,
		RenewInterval:   20 * time.Second,
		AcquireInterval: time.Second,
		newToken:        func() string { return "lease-token-01" },
	})
	require.NoError(t, err)
	require.NoError(t, runtime.Start(context.Background()))
	<-started

	done := make(chan struct{})
	go func() {
		runtime.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runtime stop did not wait for active scan cancellation")
	}
	require.Equal(t, 1, leases.ReleaseCalls())
}

func TestScannerRuntimeStartFailsWhenCursorInitializationFails(t *testing.T) {
	wantErr := errors.New("database unavailable")
	leases := &scannerRuntimeLeaseStoreStub{initializeErr: wantErr}
	runtime := newTestScannerRuntime(t, &scannerRunnerStub{}, leases, &scannerRuntimeTestClock{}, make(chan scannerRuntimeWait))

	err := runtime.Start(context.Background())
	require.ErrorIs(t, err, wantErr)
	require.Equal(t, ScannerRuntimeStateUnhealthy, runtime.Status().State)
	require.Zero(t, leases.AcquireCalls())
}

func TestScannerRuntimeAllowsOnlyOneLeaderAndStandbyTakesOver(t *testing.T) {
	leases := &exclusiveScannerLeaseStore{}
	firstCalls := make(chan struct{}, 1)
	secondCalls := make(chan struct{}, 1)
	first := blockingScannerRuntime(t, leases, "scanner-instance-01", "lease-token-01", firstCalls)
	second := blockingScannerRuntime(t, leases, "scanner-instance-02", "lease-token-02", secondCalls)
	require.NoError(t, first.Start(context.Background()))
	require.NoError(t, second.Start(context.Background()))

	var leader *ScannerRuntime
	var standby *ScannerRuntime
	var standbyCalls chan struct{}
	select {
	case <-firstCalls:
		leader, standby, standbyCalls = first, second, secondCalls
	case <-secondCalls:
		leader, standby, standbyCalls = second, first, firstCalls
	case <-time.After(time.Second):
		t.Fatal("neither scanner acquired the lease")
	}
	select {
	case <-standbyCalls:
		t.Fatal("both scanner runtimes ran concurrently")
	case <-time.After(20 * time.Millisecond):
	}

	leader.Stop()
	select {
	case <-standbyCalls:
	case <-time.After(time.Second):
		t.Fatal("standby scanner did not take over after leader stopped")
	}
	standby.Stop()
}

func newTestScannerRuntime(
	t *testing.T,
	scanner ScannerRunner,
	leases ScannerLeaseStore,
	clock *scannerRuntimeTestClock,
	waits chan scannerRuntimeWait,
) *ScannerRuntime {
	t.Helper()
	runtime, err := NewScannerRuntime(scanner, leases, ScannerRuntimeOptions{
		Enabled:         true,
		ScannerKey:      "conflux_espace_mainnet:usdt0",
		Owner:           "scanner-instance-01",
		ChainID:         1030,
		TokenContract:   "0xaf37e8b6c9ed7f6318979f56fc287d76c30847ff",
		ScanStartBlock:  100,
		PollInterval:    10 * time.Second,
		LeaseTTL:        20 * time.Second,
		RenewInterval:   5 * time.Second,
		AcquireInterval: 3 * time.Second,
		now:             clock.Now,
		newToken:        func() string { return "lease-token-01" },
		wait: func(ctx context.Context, duration time.Duration) bool {
			wait := scannerRuntimeWait{duration: duration, resume: make(chan struct{})}
			select {
			case waits <- wait:
			case <-ctx.Done():
				return false
			}
			select {
			case <-wait.resume:
				return true
			case <-ctx.Done():
				return false
			}
		},
	})
	require.NoError(t, err)
	return runtime
}

func blockingScannerRuntime(
	t *testing.T,
	leases ScannerLeaseStore,
	owner string,
	token string,
	calls chan struct{},
) *ScannerRuntime {
	t.Helper()
	scanner := scannerRunnerFunc(func(ctx context.Context, _ string, _ time.Time) (ScannerResult, error) {
		calls <- struct{}{}
		<-ctx.Done()
		return ScannerResult{}, ctx.Err()
	})
	runtime, err := NewScannerRuntime(scanner, leases, ScannerRuntimeOptions{
		Enabled:         true,
		ScannerKey:      "conflux_espace_mainnet:usdt0",
		Owner:           owner,
		ChainID:         1030,
		TokenContract:   "0xaf37e8b6c9ed7f6318979f56fc287d76c30847ff",
		PollInterval:    time.Hour,
		LeaseTTL:        time.Hour,
		RenewInterval:   20 * time.Minute,
		AcquireInterval: 5 * time.Millisecond,
		newToken:        func() string { return token },
	})
	require.NoError(t, err)
	return runtime
}

type scannerRuntimeWait struct {
	duration time.Duration
	resume   chan struct{}
}

type scannerRuntimeTestClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *scannerRuntimeTestClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *scannerRuntimeTestClock) Advance(duration time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(duration)
	c.mu.Unlock()
}

type scannerRunnerCall struct {
	token string
}

type scannerRunnerStub struct {
	err   error
	calls chan scannerRunnerCall
}

func (s *scannerRunnerStub) ScanNext(_ context.Context, token string, _ time.Time) (ScannerResult, error) {
	if s.calls != nil {
		s.calls <- scannerRunnerCall{token: token}
	}
	return ScannerResult{Advanced: true}, s.err
}

type scannerRunnerFunc func(context.Context, string, time.Time) (ScannerResult, error)

func (f scannerRunnerFunc) ScanNext(ctx context.Context, token string, now time.Time) (ScannerResult, error) {
	return f(ctx, token, now)
}

type scannerRuntimeLeaseStoreStub struct {
	mu             sync.Mutex
	initializeErr  error
	acquireResults []bool
	renewResults   []bool
	initialize     int
	acquire        int
	renew          int
	release        int
	recordError    int
}

func (s *scannerRuntimeLeaseStoreStub) Initialize(context.Context, string, uint64, string, uint64) (ScannerCursor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initialize++
	return ScannerCursor{}, s.initializeErr
}

func (s *scannerRuntimeLeaseStoreStub) AcquireLease(context.Context, string, string, string, time.Time, time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := s.acquire
	s.acquire++
	if index >= len(s.acquireResults) {
		return false, nil
	}
	return s.acquireResults[index], nil
}

func (s *scannerRuntimeLeaseStoreStub) RenewLease(context.Context, string, string, string, time.Time, time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := s.renew
	s.renew++
	if index >= len(s.renewResults) {
		return false, nil
	}
	return s.renewResults[index], nil
}

func (s *scannerRuntimeLeaseStoreStub) ReleaseLease(context.Context, string, string, string) (bool, error) {
	s.mu.Lock()
	s.release++
	s.mu.Unlock()
	return true, nil
}

func (s *scannerRuntimeLeaseStoreStub) RecordError(context.Context, string, string, time.Time, string) error {
	s.mu.Lock()
	s.recordError++
	s.mu.Unlock()
	return nil
}

func (s *scannerRuntimeLeaseStoreStub) InitializeCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.initialize
}

func (s *scannerRuntimeLeaseStoreStub) AcquireCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.acquire
}

func (s *scannerRuntimeLeaseStoreStub) RenewCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.renew
}

func (s *scannerRuntimeLeaseStoreStub) ReleaseCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.release
}

func (s *scannerRuntimeLeaseStoreStub) RecordErrorCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.recordError
}

type exclusiveScannerLeaseStore struct {
	mu    sync.Mutex
	token string
}

func (s *exclusiveScannerLeaseStore) Initialize(context.Context, string, uint64, string, uint64) (ScannerCursor, error) {
	return ScannerCursor{}, nil
}

func (s *exclusiveScannerLeaseStore) AcquireLease(_ context.Context, _ string, _ string, token string, _ time.Time, _ time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token != "" {
		return false, nil
	}
	s.token = token
	return true, nil
}

func (s *exclusiveScannerLeaseStore) RenewLease(_ context.Context, _ string, _ string, token string, _ time.Time, _ time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.token == token, nil
}

func (s *exclusiveScannerLeaseStore) ReleaseLease(_ context.Context, _ string, _ string, token string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token != token {
		return false, nil
	}
	s.token = ""
	return true, nil
}

func (s *exclusiveScannerLeaseStore) RecordError(context.Context, string, string, time.Time, string) error {
	return nil
}
