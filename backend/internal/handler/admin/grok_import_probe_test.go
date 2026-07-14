//go:build unit

package admin

import (
	"bytes"
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type grokImportProbeStub struct {
	mu           sync.Mutex
	calls        map[int64]int
	failures     map[int64]error
	active       int
	maxActive    int
	deadlineSeen bool
	block        <-chan struct{}
	started      chan int64
	done         chan int64
}

func newGrokImportProbeStub(buffer int) *grokImportProbeStub {
	return &grokImportProbeStub{
		calls:    make(map[int64]int),
		failures: make(map[int64]error),
		started:  make(chan int64, buffer),
		done:     make(chan int64, buffer),
	}
}

func (s *grokImportProbeStub) ProbeUsage(ctx context.Context, accountID int64) (*service.GrokQuotaProbeResult, error) {
	_, deadlineSeen := ctx.Deadline()
	s.mu.Lock()
	s.calls[accountID]++
	s.active++
	if s.active > s.maxActive {
		s.maxActive = s.active
	}
	s.deadlineSeen = s.deadlineSeen || deadlineSeen
	s.mu.Unlock()

	s.started <- accountID
	var ctxErr error
	if s.block != nil {
		select {
		case <-s.block:
		case <-ctx.Done():
			ctxErr = ctx.Err()
		}
	}

	s.mu.Lock()
	s.active--
	failure := s.failures[accountID]
	s.mu.Unlock()
	s.done <- accountID
	if ctxErr != nil {
		return nil, ctxErr
	}
	if failure != nil {
		return nil, failure
	}
	return &service.GrokQuotaProbeResult{
		Source:         "active_probe",
		Model:          "grok-4.5",
		StatusCode:     200,
		ResetSupported: false,
	}, nil
}

func (s *grokImportProbeStub) snapshot() (map[int64]int, int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	calls := make(map[int64]int, len(s.calls))
	for id, count := range s.calls {
		calls[id] = count
	}
	return calls, s.maxActive, s.deadlineSeen
}

type grokImportProbeSchedulerTestSnapshot struct {
	queued     int
	workers    int
	maxWorkers int
}

func snapshotGrokImportProbeScheduler(s *grokImportProbeScheduler) grokImportProbeSchedulerTestSnapshot {
	if s == nil {
		return grokImportProbeSchedulerTestSnapshot{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return grokImportProbeSchedulerTestSnapshot{
		queued:     len(s.queue),
		workers:    s.workers,
		maxWorkers: s.maxWorkers,
	}
}

func newGrokOAuthImportAccount(id int64) *service.Account {
	return &service.Account{
		ID:       id,
		Platform: service.PlatformGrok,
		Type:     service.AccountTypeOAuth,
	}
}

func awaitGrokProbeSignal(t *testing.T, signals <-chan int64) int64 {
	t.Helper()
	select {
	case id := <-signals:
		return id
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Grok import probe")
		return 0
	}
}

func TestGrokImportProbeSchedulerProbesSingleAccountOnce(t *testing.T) {
	scheduler := newGrokImportProbeScheduler(1, time.Second)
	prober := newGrokImportProbeStub(1)

	scheduler.schedule(prober, newGrokOAuthImportAccount(101))
	require.Equal(t, int64(101), awaitGrokProbeSignal(t, prober.done))

	calls, maxActive, deadlineSeen := prober.snapshot()
	require.Equal(t, map[int64]int{101: 1}, calls)
	require.Equal(t, 1, maxActive)
	require.True(t, deadlineSeen)
	require.Eventually(t, func() bool {
		snapshot := snapshotGrokImportProbeScheduler(scheduler)
		return snapshot.queued == 0 && snapshot.workers == 0
	}, time.Second, 10*time.Millisecond)
}

func TestGrokImportProbeSchedulerQueuesBatchWithoutPerTaskGoroutines(t *testing.T) {
	const taskCount = 100
	release := make(chan struct{})
	scheduler := newGrokImportProbeScheduler(3, time.Second)
	prober := newGrokImportProbeStub(taskCount)
	prober.block = release
	prober.failures[150] = infraerrors.New(502, "GROK_TEST_PROBE_FAILED", "sensitive-upstream-body")

	for id := int64(101); id < 101+taskCount; id++ {
		scheduler.schedule(prober, newGrokOAuthImportAccount(id))
	}
	for i := 0; i < 3; i++ {
		awaitGrokProbeSignal(t, prober.started)
	}
	snapshot := snapshotGrokImportProbeScheduler(scheduler)
	require.Equal(t, 97, snapshot.queued)
	require.Equal(t, 3, snapshot.workers)
	require.Equal(t, 3, snapshot.maxWorkers)
	select {
	case id := <-prober.started:
		t.Fatalf("probe %d started before a concurrency slot was released", id)
	case <-time.After(75 * time.Millisecond):
	}
	close(release)
	for i := 0; i < taskCount; i++ {
		awaitGrokProbeSignal(t, prober.done)
	}

	calls, maxActive, _ := prober.snapshot()
	require.Len(t, calls, taskCount)
	for id := int64(101); id < 101+taskCount; id++ {
		require.Equal(t, 1, calls[id])
	}
	require.Equal(t, 3, maxActive)
	require.Eventually(t, func() bool {
		snapshot = snapshotGrokImportProbeScheduler(scheduler)
		return snapshot.queued == 0 && snapshot.workers == 0
	}, time.Second, 10*time.Millisecond)
	require.Equal(t, 3, snapshot.maxWorkers)
}

func TestGrokImportProbeSchedulerTimeoutCancelsProbe(t *testing.T) {
	neverRelease := make(chan struct{})
	scheduler := newGrokImportProbeScheduler(1, 20*time.Millisecond)
	prober := newGrokImportProbeStub(1)
	prober.block = neverRelease

	scheduler.schedule(prober, newGrokOAuthImportAccount(201))
	require.Equal(t, int64(201), awaitGrokProbeSignal(t, prober.done))

	calls, _, _ := prober.snapshot()
	require.Equal(t, 1, calls[201])
}

func TestGrokImportProbeSchedulerSkipsMissingServiceAndNonGrokAccounts(t *testing.T) {
	scheduler := newGrokImportProbeScheduler(1, time.Second)
	prober := newGrokImportProbeStub(1)

	scheduler.schedule(nil, newGrokOAuthImportAccount(301))
	scheduler.schedule(prober, &service.Account{ID: 302, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth})
	scheduler.schedule(prober, &service.Account{ID: 303, Platform: service.PlatformGrok, Type: service.AccountTypeAPIKey})

	select {
	case id := <-prober.started:
		t.Fatalf("unexpected probe for account %d", id)
	case <-time.After(50 * time.Millisecond):
	}
	calls, _, _ := prober.snapshot()
	require.Empty(t, calls)
}

func TestGrokImportProbeFailureLogDoesNotIncludeErrorMessage(t *testing.T) {
	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	defer slog.SetDefault(previousLogger)

	scheduler := newGrokImportProbeScheduler(1, time.Second)
	prober := newGrokImportProbeStub(1)
	prober.failures[401] = infraerrors.New(502, "GROK_TEST_PROBE_FAILED", "refresh-token-secret")
	scheduler.schedule(prober, newGrokOAuthImportAccount(401))
	awaitGrokProbeSignal(t, prober.done)

	require.Eventually(t, func() bool {
		return bytes.Contains(logs.Bytes(), []byte("grok_import_active_probe_failed"))
	}, time.Second, 10*time.Millisecond)
	require.Contains(t, logs.String(), "GROK_TEST_PROBE_FAILED")
	require.NotContains(t, logs.String(), "refresh-token-secret")
}
