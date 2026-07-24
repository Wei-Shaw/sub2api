package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type deferredLifecycleRepo struct {
	AccountRepository
	updates atomic.Int64
}

func (r *deferredLifecycleRepo) BatchUpdateLastUsed(context.Context, map[int64]time.Time) error {
	r.updates.Add(1)
	return nil
}

func TestDeferredServiceStopBeforeStartDoesNotFlushOrRestart(t *testing.T) {
	timingWheel, err := NewTimingWheelService()
	if err != nil {
		t.Fatal(err)
	}
	defer timingWheel.Stop()
	repo := &deferredLifecycleRepo{}
	svc := NewDeferredService(repo, timingWheel, time.Hour)
	svc.ScheduleLastUsedUpdate(1)
	svc.Stop()
	svc.Stop()
	svc.Start()
	if repo.updates.Load() != 0 {
		t.Fatalf("standby deferred service flushed %d times", repo.updates.Load())
	}
	if svc.started {
		t.Fatal("stopped deferred service restarted")
	}
}

func TestDeferredServiceActiveStopFlushesExactlyOnce(t *testing.T) {
	timingWheel, err := NewTimingWheelService()
	if err != nil {
		t.Fatal(err)
	}
	defer timingWheel.Stop()
	repo := &deferredLifecycleRepo{}
	svc := NewDeferredService(repo, timingWheel, time.Hour)
	svc.Start()
	svc.ScheduleLastUsedUpdate(1)
	svc.Stop()
	svc.Stop()
	if repo.updates.Load() != 1 {
		t.Fatalf("active deferred service flushes = %d, want 1", repo.updates.Load())
	}
}

func TestPricingSchedulerStopIsIdempotentAndPreventsLateStart(t *testing.T) {
	svc := NewPricingService(&config.Config{}, nil)
	svc.Stop()
	svc.Stop()
	svc.startUpdateScheduler()
	if svc.started {
		t.Fatal("pricing scheduler restarted after stop")
	}
}

func TestBatchImageCleanupStopBeforeStartPreventsLateWorker(t *testing.T) {
	svc := &BatchImageCleanupService{Config: &config.Config{}}
	svc.Stop()
	svc.Stop()
	svc.Start()
	if !svc.stopped || svc.cancel != nil {
		t.Fatal("batch-image cleanup restarted after stop")
	}
}

type lifecycleDashboardRepository struct {
	DashboardAggregationRepository
}

type blockingDashboardLifecycleRepository struct {
	DashboardAggregationRepository
	started chan struct{}
	done    chan struct{}
}

func (r *blockingDashboardLifecycleRepository) RecomputeRange(ctx context.Context, _, _ time.Time) error {
	close(r.started)
	<-ctx.Done()
	close(r.done)
	return ctx.Err()
}

func TestDashboardAggregationStopBeforeStartPreventsLateWorker(t *testing.T) {
	timingWheel, err := NewTimingWheelService()
	if err != nil {
		t.Fatal(err)
	}
	defer timingWheel.Stop()
	cfg := &config.Config{DashboardAgg: config.DashboardAggregationConfig{Enabled: true, IntervalSeconds: 3600}}
	svc := NewDashboardAggregationService(&lifecycleDashboardRepository{}, timingWheel, cfg)
	svc.Stop()
	svc.Stop()
	svc.Start()
	if svc.started || !svc.stopped {
		t.Fatal("dashboard aggregation restarted after stop")
	}
}

func TestDashboardManualRecomputeIsTrackedAndCanceledByStop(t *testing.T) {
	repo := &blockingDashboardLifecycleRepository{started: make(chan struct{}), done: make(chan struct{})}
	svc := NewDashboardAggregationService(repo, nil, &config.Config{
		DashboardAgg: config.DashboardAggregationConfig{Enabled: true},
	})
	now := time.Now().UTC()
	if err := svc.TriggerRecomputeRange(now.Add(-time.Hour), now); err != nil {
		t.Fatal(err)
	}
	select {
	case <-repo.started:
	case <-time.After(time.Second):
		t.Fatal("manual recompute did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := svc.StopContext(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-repo.done:
	default:
		t.Fatal("manual recompute was not canceled before StopContext returned")
	}
}

type lifecycleUsageCleanupRepository struct {
	UsageCleanupRepository
}

type blockingUsageCleanupLifecycleRepository struct {
	UsageCleanupRepository
	started chan struct{}
	done    chan struct{}
}

func (r *blockingUsageCleanupLifecycleRepository) CreateTask(_ context.Context, task *UsageCleanupTask) error {
	task.ID = 1
	return nil
}

func (r *blockingUsageCleanupLifecycleRepository) ClaimNextPendingTask(ctx context.Context, _ int64) (*UsageCleanupTask, error) {
	close(r.started)
	<-ctx.Done()
	close(r.done)
	return nil, ctx.Err()
}

func TestUsageCleanupStopBeforeStartPreventsLateWorker(t *testing.T) {
	timingWheel, err := NewTimingWheelService()
	if err != nil {
		t.Fatal(err)
	}
	defer timingWheel.Stop()
	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: true, WorkerIntervalSeconds: 3600}}
	svc := NewUsageCleanupService(&lifecycleUsageCleanupRepository{}, timingWheel, nil, cfg)
	svc.Stop()
	svc.Stop()
	svc.Start()
	if svc.started || !svc.stopped {
		t.Fatal("usage cleanup restarted after stop")
	}
}

func TestUsageCleanupImmediateRunIsTrackedAndCanceledByStop(t *testing.T) {
	repo := &blockingUsageCleanupLifecycleRepository{started: make(chan struct{}), done: make(chan struct{})}
	svc := NewUsageCleanupService(repo, nil, nil, &config.Config{
		UsageCleanup: config.UsageCleanupConfig{Enabled: true},
	})
	now := time.Now().UTC()
	if _, err := svc.CreateTask(context.Background(), UsageCleanupFilters{
		StartTime: now.Add(-time.Hour),
		EndTime:   now,
	}, 1); err != nil {
		t.Fatal(err)
	}
	select {
	case <-repo.started:
	case <-time.After(time.Second):
		t.Fatal("immediate usage cleanup did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := svc.StopContext(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-repo.done:
	default:
		t.Fatal("usage cleanup worker was not canceled before StopContext returned")
	}
}

func TestConcurrencyCleanupStopBeforeStartPreventsLateWorker(t *testing.T) {
	cache := &slotCleanupCache{}
	svc := NewConcurrencyService(cache)
	svc.StopSlotCleanupWorker()
	svc.StopSlotCleanupWorker()
	svc.StartSlotCleanupWorker(nil, time.Millisecond)
	time.Sleep(10 * time.Millisecond)
	if cache.calls.Load() != 0 {
		t.Fatalf("stopped concurrency cleanup ran %d times", cache.calls.Load())
	}
}

type lifecycleUserMessageQueueCache struct {
	UserMsgQueueCache
	calls atomic.Int64
}

func (c *lifecycleUserMessageQueueCache) ReconcileExpiredLockCandidates(context.Context, int) (int, error) {
	c.calls.Add(1)
	return 0, nil
}

func TestUserMessageQueueCleanupStopBeforeStartPreventsLateWorker(t *testing.T) {
	cache := &lifecycleUserMessageQueueCache{}
	svc := NewUserMessageQueueService(cache, nil, &config.UserMessageQueueConfig{})
	svc.Stop()
	svc.Stop()
	svc.StartCleanupWorker(time.Millisecond)
	time.Sleep(10 * time.Millisecond)
	if cache.calls.Load() != 0 {
		t.Fatalf("stopped user-message cleanup ran %d times", cache.calls.Load())
	}
}
