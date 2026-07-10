package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type recordingBillingReservationReaperRepo struct {
	mu      sync.Mutex
	calls   int
	now     time.Time
	limit   int
	results []BillingReservationReapResult
	err     error
}

func (r *recordingBillingReservationReaperRepo) ReapExpiredVideoReservations(_ context.Context, now time.Time, limit int) ([]BillingReservationReapResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.now = now
	r.limit = limit
	return append([]BillingReservationReapResult(nil), r.results...), r.err
}

func (r *recordingBillingReservationReaperRepo) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func TestBillingReservationReaperDefaultsToSixtySeconds(t *testing.T) {
	reaper := NewBillingReservationReaper(&recordingBillingReservationReaperRepo{}, &config.Config{
		ReliabilityCore: config.ReliabilityCoreConfig{VideoEnabled: true},
	})
	if reaper.interval != 60*time.Second {
		t.Fatalf("reaper interval = %s, want 60s", reaper.interval)
	}
}

func TestBillingReservationReaperFlagFalseDoesNotWrite(t *testing.T) {
	repo := &recordingBillingReservationReaperRepo{}
	reaper := NewBillingReservationReaper(repo, &config.Config{
		ReliabilityCore: config.ReliabilityCoreConfig{VideoEnabled: false, ReservationReapIntervalSeconds: 1},
	})

	results, err := reaper.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("disabled reaper: %v", err)
	}
	if len(results) != 0 || repo.callCount() != 0 {
		t.Fatalf("disabled reaper results=%#v calls=%d", results, repo.callCount())
	}
}

func TestBillingReservationReaperRunOnceUsesRepository(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	repo := &recordingBillingReservationReaperRepo{results: []BillingReservationReapResult{
		{ReservationID: 11, TaskID: 21, Action: BillingReservationReapActionReleased},
	}}
	reaper := NewBillingReservationReaper(repo, &config.Config{
		ReliabilityCore: config.ReliabilityCoreConfig{VideoEnabled: true, ReservationReapIntervalSeconds: 60},
	})
	reaper.now = func() time.Time { return now }

	results, err := reaper.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if len(results) != 1 || results[0].Action != BillingReservationReapActionReleased || repo.callCount() != 1 {
		t.Fatalf("run once results=%#v calls=%d", results, repo.callCount())
	}
	if !repo.now.Equal(now) || repo.limit != billingReservationReaperDefaultBatch {
		t.Fatalf("repository args now=%s limit=%d", repo.now, repo.limit)
	}
}

type blockingReaperVideoRepo struct {
	*memoryVideoGatewayRepo
	started chan struct{}
	once    sync.Once
}

func (r *blockingReaperVideoRepo) ReapExpiredVideoReservations(ctx context.Context, _ time.Time, _ int) ([]BillingReservationReapResult, error) {
	r.once.Do(func() { close(r.started) })
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestBillingReservationReaperWorkerStopCancelsWithoutProviderWork(t *testing.T) {
	repo := &blockingReaperVideoRepo{memoryVideoGatewayRepo: newMemoryVideoGatewayRepo(), started: make(chan struct{})}
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)
	cfg := &config.Config{
		VideoGateway: config.VideoGatewayConfig{WorkerEnabled: false, PollIntervalSeconds: 3600},
		ReliabilityCore: config.ReliabilityCoreConfig{
			VideoEnabled:                   true,
			ReservationReapIntervalSeconds: 1,
		},
	}
	worker := NewVideoGatewayWorker(svc, cfg)
	worker.reaper.interval = time.Millisecond
	worker.Start()

	select {
	case <-repo.started:
	case <-time.After(time.Second):
		t.Fatal("reaper did not start")
	}
	stopped := make(chan struct{})
	go func() {
		worker.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("worker stop did not cancel in-flight reaper")
	}
}

func TestBillingReservationReaperPropagatesRepositoryError(t *testing.T) {
	repo := &recordingBillingReservationReaperRepo{err: errors.New("transaction rolled back")}
	reaper := NewBillingReservationReaper(repo, &config.Config{
		ReliabilityCore: config.ReliabilityCoreConfig{VideoEnabled: true},
	})
	if _, err := reaper.RunOnce(context.Background()); err == nil {
		t.Fatal("expected repository error")
	}
}
