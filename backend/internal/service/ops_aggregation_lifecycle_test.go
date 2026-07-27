package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type lifecycleOpsRepository struct {
	OpsRepository
}

type bootstrapOpsRepository struct {
	opsRepoMock
	invalidateHourlyCalls int
	invalidateDailyCalls  int
	upsertHourlyCalls     int
	upsertDailyCalls      int
	invalidateHourlyErr   error
}

func (r *bootstrapOpsRepository) InvalidateHourlyMetricsVersion(context.Context, time.Time, time.Time, int) error {
	r.invalidateHourlyCalls++
	return r.invalidateHourlyErr
}

func (r *bootstrapOpsRepository) InvalidateDailyMetricsVersion(context.Context, time.Time, time.Time, int) error {
	r.invalidateDailyCalls++
	return nil
}

func (r *bootstrapOpsRepository) UpsertHourlyMetrics(context.Context, time.Time, time.Time) error {
	r.upsertHourlyCalls++
	return nil
}

func (r *bootstrapOpsRepository) UpsertDailyMetrics(context.Context, time.Time, time.Time) error {
	r.upsertDailyCalls++
	return nil
}

type blockingOpsSettingRepository struct {
	SettingRepository
	started chan struct{}
	release chan struct{}
}

func (r *blockingOpsSettingRepository) GetValue(context.Context, string) (string, error) {
	r.started <- struct{}{}
	<-r.release
	return "false", nil
}

func TestOpsAggregationStopContextHonorsCallerBudget(t *testing.T) {
	settings := &blockingOpsSettingRepository{
		started: make(chan struct{}, 2),
		release: make(chan struct{}),
	}
	svc := NewOpsAggregationService(
		&lifecycleOpsRepository{},
		settings,
		nil,
		nil,
		&config.Config{Ops: config.OpsConfig{Enabled: true, Aggregation: config.OpsAggregationConfig{Enabled: true}}},
	)
	svc.Start()
	for i := 0; i < 2; i++ {
		select {
		case <-settings.started:
		case <-time.After(time.Second):
			t.Fatal("aggregation worker did not start")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	err := svc.StopContext(ctx)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("StopContext error=%v, want deadline exceeded", err)
	}

	close(settings.release)
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Second)
	defer cleanupCancel()
	if err := svc.StopContext(cleanupCtx); err != nil {
		t.Fatalf("workers did not finish after release: %v", err)
	}
}

func TestAggregationWindowStartKeepsOverlapAndBoundsCatchup(t *testing.T) {
	end := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		latest time.Time
		want   time.Time
	}{
		{
			name:   "recent bucket reprocesses overlap",
			latest: end.Add(-time.Hour),
			want:   end.Add(-3 * time.Hour),
		},
		{
			name:   "missed runs catch up from latest with overlap",
			latest: end.Add(-12 * time.Hour),
			want:   end.Add(-14 * time.Hour),
		},
		{
			name:   "stale state is bounded",
			latest: end.Add(-7 * 24 * time.Hour),
			want:   end.Add(-opsAggInitialBackfillWindow),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := aggregationWindowStart(
				end,
				tt.latest,
				opsAggBackfillWindow,
				opsAggHourlyOverlap,
				opsAggInitialBackfillWindow,
			)
			if !got.Equal(tt.want) {
				t.Fatalf("aggregationWindowStart() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestOpsAggregationBootstrapInvalidatesBeforePublishingV2(t *testing.T) {
	repo := &bootstrapOpsRepository{}
	svc := NewOpsAggregationService(repo, nil, nil, nil, nil)

	svc.aggregateDaily()
	if repo.invalidateDailyCalls != 0 || repo.upsertDailyCalls != 0 {
		t.Fatal("daily aggregation must wait for hourly definition bootstrap")
	}

	svc.aggregateHourly()
	if repo.invalidateHourlyCalls != 1 {
		t.Fatalf("hourly invalidation calls = %d, want 1", repo.invalidateHourlyCalls)
	}
	if repo.upsertHourlyCalls == 0 {
		t.Fatal("hourly bootstrap did not repopulate invalidated buckets")
	}
	if !svc.hourlyDefinitionReady.Load() {
		t.Fatal("hourly definition should be ready after successful bootstrap")
	}

	svc.aggregateHourly()
	if repo.invalidateHourlyCalls != 1 {
		t.Fatalf("hourly invalidation repeated after bootstrap: %d", repo.invalidateHourlyCalls)
	}
}

func TestOpsAggregationBootstrapFailureStaysFailSafe(t *testing.T) {
	repo := &bootstrapOpsRepository{invalidateHourlyErr: errors.New("invalidation failed")}
	svc := NewOpsAggregationService(repo, nil, nil, nil, nil)

	svc.aggregateHourly()
	if repo.upsertHourlyCalls != 0 {
		t.Fatalf("upsert calls = %d after invalidation failure, want 0", repo.upsertHourlyCalls)
	}
	if svc.hourlyDefinitionReady.Load() {
		t.Fatal("failed bootstrap must not publish the hourly definition as ready")
	}
}
