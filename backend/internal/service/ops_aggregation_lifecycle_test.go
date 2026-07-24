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
