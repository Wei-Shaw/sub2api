package service

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// UpstreamPriceMonitorRunner owns only scheduling. The durable single-running
// constraint and stale-run recovery in the repository are the cross-instance
// backstop, while the service run mutex prevents overlap in one process.
type UpstreamPriceMonitorRunner struct {
	monitor *UpstreamPriceMonitorService
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	once    sync.Once
}

func NewUpstreamPriceMonitorRunner(monitor *UpstreamPriceMonitorService) *UpstreamPriceMonitorRunner {
	ctx, cancel := context.WithCancel(context.Background())
	return &UpstreamPriceMonitorRunner{monitor: monitor, ctx: ctx, cancel: cancel}
}

func (r *UpstreamPriceMonitorRunner) Start() {
	if r == nil || r.monitor == nil {
		return
	}
	r.once.Do(func() {
		r.wg.Add(1)
		go r.loop()
	})
}

func (r *UpstreamPriceMonitorRunner) Stop() {
	if r == nil {
		return
	}
	r.cancel()
	r.wg.Wait()
}

func (r *UpstreamPriceMonitorRunner) loop() {
	defer r.wg.Done()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	r.runIfDue()
	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			r.runIfDue()
		}
	}
}

func (r *UpstreamPriceMonitorRunner) runIfDue() {
	ctx, cancel := context.WithTimeout(r.ctx, 14*time.Minute)
	defer cancel()
	cfg, err := r.monitor.GetConfig(ctx)
	if err != nil || !cfg.Enabled {
		return
	}
	runtime, err := r.monitor.GetRuntime(ctx)
	if err != nil {
		slog.Warn("upstream_price_monitor_runtime_failed", "error", err)
		return
	}
	if runtime.NextRunAt != nil && runtime.NextRunAt.After(time.Now()) {
		return
	}
	_, err = r.monitor.RunOnce(ctx, UpstreamPriceRunOptions{
		Trigger: domain.UpstreamPriceMonitorRunTriggerScheduled,
		DryRun:  cfg.Mode != domain.UpstreamPriceMonitorModeAutoApply,
	})
	if err != nil && !errors.Is(err, ErrUpstreamPriceMonitorRunConflict) {
		slog.Warn("upstream_price_monitor_run_failed", "error", err)
	}
}
