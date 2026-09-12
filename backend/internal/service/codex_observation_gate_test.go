package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func quotaGateSnapshot(at time.Time, used float64, remaining int) *OpenAICodexUsageSnapshot {
	minutes := 300
	return &OpenAICodexUsageSnapshot{PrimaryUsedPercent: &used, PrimaryResetAfterSeconds: &remaining, PrimaryWindowMinutes: &minutes, UpdatedAt: at.Format(time.RFC3339Nano)}
}

func TestCodexObservationGatePreservesResetAndExhaustion(t *testing.T) {
	var gate codexObservationGate
	at := time.Now().UTC()
	fresh, critical := gate.classify(1, quotaGateSnapshot(at, 80, 600), at)
	require.True(t, fresh)
	require.True(t, critical)
	// Remaining seconds decrease with elapsed time: the boundary did not move.
	fresh, critical = gate.classify(1, quotaGateSnapshot(at.Add(time.Second), 90, 599), at.Add(time.Second))
	require.True(t, fresh)
	require.False(t, critical)
	_, critical = gate.classify(1, quotaGateSnapshot(at.Add(2*time.Second), 100, 598), at.Add(2*time.Second))
	require.True(t, critical, "the final exhausted observation bypasses throttling")
	_, critical = gate.classify(1, quotaGateSnapshot(at.Add(3*time.Second), 1, 18000), at.Add(3*time.Second))
	require.True(t, critical, "an early/new reset bypasses throttling")
	fresh, _ = gate.classify(1, quotaGateSnapshot(at, 99, 600), at.Add(4*time.Second))
	require.False(t, fresh, "old responses cannot move the gate backwards")
}

func TestCodexCriticalSnapshotBypassesWriteInterval(t *testing.T) {
	repo := &snapshotUpdateAccountRepo{updateExtraCalls: make(chan map[string]any, 4)}
	svc := &OpenAIGatewayService{accountRepo: repo, codexSnapshotThrottle: newAccountWriteThrottle(time.Hour)}
	at := time.Now().UTC()
	for i, used := range []float64{90, 100, 0} {
		remaining := 600 - i
		if i == 2 {
			remaining = 18000
		}
		svc.updateCodexUsageSnapshot(context.Background(), 1, quotaGateSnapshot(at.Add(time.Duration(i)*time.Millisecond), used, remaining))
	}
	for i := 0; i < 3; i++ {
		select {
		case <-repo.updateExtraCalls:
		case <-time.After(2 * time.Second):
			t.Fatal("critical quota observation was dropped")
		}
	}
}

type quotaHeaderUpstream struct {
	HTTPUpstream
	response *http.Response
}

func (u *quotaHeaderUpstream) Do(*http.Request, string, int64, int) (*http.Response, error) {
	return u.response, nil
}

type unreadQuotaBody struct{ t *testing.T }

func (b unreadQuotaBody) Read([]byte) (int, error) {
	b.t.Fatal("quota capture must not read the streaming body")
	return 0, nil
}
func (b unreadQuotaBody) Close() error { return nil }

func TestCodexQuotaObservedBeforeStreamingBody(t *testing.T) {
	repo := &snapshotUpdateAccountRepo{updateExtraCalls: make(chan map[string]any, 1)}
	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", "99")
	headers.Set("x-codex-primary-reset-after-seconds", "600")
	headers.Set("x-codex-primary-window-minutes", "300")
	upstream := &quotaHeaderUpstream{response: &http.Response{StatusCode: 200, Header: headers, Body: unreadQuotaBody{t}}}
	svc := &OpenAIGatewayService{accountRepo: repo, httpUpstream: upstream}
	response, err := svc.doOpenAIUpstream(httptest.NewRequest(http.MethodPost, "https://example.test/responses", nil), "", &Account{ID: 98, Platform: PlatformOpenAI, Type: AccountTypeOAuth})
	require.NoError(t, err)
	require.Same(t, upstream.response, response)
	select {
	case update := <-repo.updateExtraCalls:
		require.Equal(t, 99.0, update["codex_5h_used_percent"])
	case <-time.After(2 * time.Second):
		t.Fatal("quota should be captured while the body remains unread")
	}
}

type quotaOrderedWriterRepo struct {
	stubOpenAIAccountRepo
	write func(context.Context, map[string]any) error
}

func (r *quotaOrderedWriterRepo) UpdateExtra(ctx context.Context, _ int64, updates map[string]any) error {
	return r.write(ctx, updates)
}

func TestCodexObservationWriterKeepsExhaustionBeforeReset(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	persisted := make(chan float64, 2)
	repo := &quotaOrderedWriterRepo{write: func(ctx context.Context, updates map[string]any) error {
		used := updates["codex_5h_used_percent"].(float64)
		if used == 100 {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		persisted <- used
		return nil
	}}
	svc := &OpenAIGatewayService{accountRepo: repo, codexSnapshotThrottle: newAccountWriteThrottle(time.Hour)}
	t.Cleanup(svc.codexObservationGate.close)
	at := time.Now().UTC()
	svc.updateCodexUsageSnapshot(context.Background(), 1, quotaGateSnapshot(at, 100, 600))
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first write did not start")
	}
	svc.updateCodexUsageSnapshot(context.Background(), 1, quotaGateSnapshot(at.Add(time.Second), 0, 18000))
	select {
	case used := <-persisted:
		t.Fatalf("reset %v overtook blocked exhaustion", used)
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	for _, want := range []float64{100, 0} {
		select {
		case used := <-persisted:
			require.Equal(t, want, used)
		case <-time.After(time.Second):
			t.Fatal("ordered write did not finish")
		}
	}
	require.Eventually(t, func() bool {
		svc.codexObservationGate.mu.Lock()
		defer svc.codexObservationGate.mu.Unlock()
		return len(svc.codexObservationGate.queues) == 0
	}, time.Second, time.Millisecond)
}

func TestCodexObservationWriterRetriesCriticalBeforeReset(t *testing.T) {
	var attempts atomic.Int32
	persisted := make(chan float64, 2)
	repo := &quotaOrderedWriterRepo{write: func(_ context.Context, updates map[string]any) error {
		if attempts.Add(1) == 1 {
			return errors.New("temporary database error")
		}
		persisted <- updates["codex_5h_used_percent"].(float64)
		return nil
	}}
	svc := &OpenAIGatewayService{accountRepo: repo, codexSnapshotThrottle: newAccountWriteThrottle(time.Hour)}
	t.Cleanup(svc.codexObservationGate.close)
	at := time.Now().UTC()
	svc.updateCodexUsageSnapshot(context.Background(), 1, quotaGateSnapshot(at, 100, 600))
	svc.updateCodexUsageSnapshot(context.Background(), 1, quotaGateSnapshot(at.Add(time.Second), 0, 18000))
	for _, want := range []float64{100, 0} {
		select {
		case used := <-persisted:
			require.Equal(t, want, used)
		case <-time.After(time.Second):
			t.Fatal("critical retry did not finish")
		}
	}
	require.EqualValues(t, 3, attempts.Load())
}

func TestCodexObservationQueueRejectsOverloadWithoutAdvancingState(t *testing.T) {
	var gate codexObservationGate
	at := time.Now().UTC()
	gate.queues = map[int64]*codexObservationQueue{1: {pending: make([]codexObservationWrite, codexObservationQueueSize)}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	snapshot := quotaGateSnapshot(at, 100, 600)
	err := gate.enqueue(ctx, 1, snapshot, at, buildCodexUsageExtraUpdates(snapshot, at), func() bool { return true }, func(context.Context, map[string]any) error { return nil })
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, gate.accounts, "rejected critical observation must remain retryable")
	require.Len(t, gate.queues[1].pending, codexObservationQueueSize)
	gate.close()
}

func TestCodexObservationWriterCloseCancelsInflight(t *testing.T) {
	entered := make(chan struct{})
	exited := make(chan struct{})
	repo := &quotaOrderedWriterRepo{write: func(ctx context.Context, _ map[string]any) error {
		close(entered)
		<-ctx.Done()
		close(exited)
		return ctx.Err()
	}}
	svc := &OpenAIGatewayService{accountRepo: repo}
	at := time.Now().UTC()
	svc.updateCodexUsageSnapshot(context.Background(), 1, quotaGateSnapshot(at, 100, 600))
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("write did not start")
	}
	svc.codexObservationGate.close()
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not cancel write")
	}
	require.Eventually(t, func() bool {
		svc.codexObservationGate.mu.Lock()
		defer svc.codexObservationGate.mu.Unlock()
		return len(svc.codexObservationGate.queues) == 0
	}, time.Second, time.Millisecond)
}

func TestCodexObservationWriterLifecycle(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	// Bootstrap logs run during normal construction and must keep collection alive.
	svc.logOpenAIWSModeBootstrap()
	require.False(t, svc.codexObservationGate.closed)
	// The history writer must close even when no WebSocket pool was ever created.
	svc.CloseOpenAIWSPool()
	require.True(t, svc.codexObservationGate.closed)
	require.NotPanics(t, svc.CloseOpenAIWSPool)
}

func TestCodexObservationWriterRetriesAfterSkippedExhaustion(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	persisted := make(chan float64, 1)
	var attempts atomic.Int32
	repo := &quotaOrderedWriterRepo{write: func(ctx context.Context, updates map[string]any) error {
		attempt := attempts.Add(1)
		if attempt == 1 {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if attempt <= 3 {
			return errors.New("temporary database error")
		}
		persisted <- updates["codex_5h_used_percent"].(float64)
		return nil
	}}
	svc := &OpenAIGatewayService{accountRepo: repo, codexSnapshotThrottle: newAccountWriteThrottle(time.Hour)}
	t.Cleanup(svc.codexObservationGate.close)
	at := time.Now().UTC()
	svc.updateCodexUsageSnapshot(context.Background(), 1, quotaGateSnapshot(at, 100, 600))
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first attempt did not start")
	}
	svc.updateCodexUsageSnapshot(context.Background(), 1, quotaGateSnapshot(at.Add(time.Second), 100, 599))
	close(release)
	require.Eventually(t, func() bool {
		svc.codexObservationGate.mu.Lock()
		defer svc.codexObservationGate.mu.Unlock()
		return len(svc.codexObservationGate.queues) == 0
	}, time.Second, time.Millisecond)
	svc.updateCodexUsageSnapshot(context.Background(), 1, quotaGateSnapshot(at.Add(2*time.Second), 100, 598))
	select {
	case used := <-persisted:
		require.Equal(t, 100.0, used)
	case <-time.After(time.Second):
		t.Fatal("skipped observation suppressed critical retry")
	}
	require.EqualValues(t, 4, attempts.Load())
}
