package monitorservice

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
)

// Job names declared with the host JobScheduler. Stable strings — admin /
// "Run now" UI references them by name, and history rows are tagged with
// them, so do not rename without an admin migration.
const (
	JobNameMonitorRun     = "monitor.run"
	monitorJobRunInterval = 60 * time.Second
	monitorJobRunTimeout  = 2 * time.Minute
)

// MonitorJobRunner registers the per-tick "scan + check" job with the host
// JobScheduler. It replaces the old in-process ChannelMonitorRunner (291
// lines of pond/sync.Map/leader-lock plumbing) — the host now owns
// scheduling, leader election, and concurrency. Daily maintenance is handled
// separately via the MaintenanceExtension gRPC service.
//
// CRUD writes do not need to notify the runner: the 60-second tick picks up
// newly enabled monitors on the next pass and naturally drops disabled ones,
// so no scheduler hook is exposed (the previous Schedule / Unschedule pair
// was an empty no-op kept only for symmetry with the legacy runner).
type MonitorJobRunner struct {
	svc  *ChannelMonitorService
	jobs pluginsdk.JobsClient
	log  *slog.Logger

	tickerWG sync.WaitGroup

	// inFlightMu / inFlightSet 跟踪当前正在被 RunCheck 的 monitor ID，
	// 防止同一 monitor 在前一次还在体检上游时被新一轮 tick 重复触发。
	// 字段挂在 runner 实例上而非 package-level，确保多 runner 实例（生产 /
	// 测试 fixture / 多插件）互相隔离不会争锁。
	inFlightMu  sync.Mutex
	inFlightSet map[int64]struct{}
}

// NewMonitorJobRunner builds the runner. jobs may be nil (host without W2);
// in that case Register is a no-op and the plugin still serves admin /
// user APIs but never fires periodic checks.
func NewMonitorJobRunner(svc *ChannelMonitorService, jobs pluginsdk.JobsClient, log *slog.Logger) *MonitorJobRunner {
	if log == nil {
		log = slog.Default()
	}
	return &MonitorJobRunner{
		svc:         svc,
		jobs:        jobs,
		log:         log,
		inFlightSet: make(map[int64]struct{}),
	}
}

// Register declares the monitor.run job spec to the host scheduler. Must be
// called from inside Plugin.Init before the SDK opens the Subscribe stream —
// the JobsClient rejects late Register calls with ErrJobsRegistered.
func (r *MonitorJobRunner) Register() error {
	if r == nil || r.svc == nil {
		return nil
	}
	if r.jobs == nil {
		r.log.Warn("channel-monitor: JobScheduler unavailable; periodic checks disabled")
		return nil
	}

	tickSpec := pluginsdk.JobSpec{
		Name: JobNameMonitorRun,
		Trigger: pluginsdk.JobTrigger{
			Kind:     pluginsdk.TriggerInterval,
			Interval: monitorJobRunInterval,
		},
		Concurrency: monitorWorkerConcurrency,
		Timeout:     monitorJobRunTimeout,
	}
	if err := r.jobs.Register(tickSpec, r.runOneTick); err != nil {
		return fmt.Errorf("register %s: %w", JobNameMonitorRun, err)
	}
	// NOTE: the daily maintenance job (monitor.daily-rollup) has been removed.
	// Daily cleanup is now triggered by the host's OpsCleanupService via the
	// MaintenanceExtension gRPC service — see internal/maintenance/server.go.
	r.log.Info("channel-monitor: jobs registered",
		"jobs", []string{JobNameMonitorRun})
	return nil
}

// runOneTick is the job handler the host fires every monitorJobRunInterval.
// It loads enabled monitors, decides which are due based on
// LastCheckedAt + IntervalSeconds, and fires their RunCheck concurrently.
//
// Concurrency is bounded twice over: once by the JobSpec.Concurrency=5 cap
// declared at Register time (so the host serialises overlapping ticks),
// and once by the per-monitor "in-flight" inFlightTicket lock so a single
// slow upstream never doubles up.
func (r *MonitorJobRunner) runOneTick(ctx context.Context, _ string) error {
	monitors, err := r.svc.ListEnabledMonitors(ctx)
	if err != nil {
		return fmt.Errorf("list enabled monitors: %w", err)
	}
	now := time.Now()
	dispatched := 0
	for _, m := range monitors {
		if !isMonitorDue(m, now) {
			continue
		}
		if !r.acquireTicket(m.ID) {
			continue
		}
		dispatched++
		r.tickerWG.Add(1)
		go r.fireOnce(ctx, m)
	}
	if dispatched > 0 {
		r.log.Debug("channel-monitor: tick dispatched checks",
			"due", dispatched, "total_enabled", len(monitors))
	}
	r.tickerWG.Wait()
	return nil
}

// fireOnce runs RunCheck for a single monitor and releases the in-flight
// ticket no matter the outcome.
func (r *MonitorJobRunner) fireOnce(ctx context.Context, m *ChannelMonitor) {
	defer r.tickerWG.Done()
	defer r.releaseTicket(m.ID)
	if _, err := r.svc.RunCheck(ctx, m.ID); err != nil {
		r.log.Warn("channel-monitor: RunCheck failed",
			"monitor_id", m.ID, "name", m.Name, "error", err)
	}
}

// acquireTicket / releaseTicket 维护 runner 自身的 in-flight 集合：基数最多
// 几百个 monitor，互斥锁的争用可以忽略，但字段挂在 runner 上保证不同 runner
// 实例（多插件 / 测试夹具）互相隔离。
func (r *MonitorJobRunner) acquireTicket(id int64) bool {
	r.inFlightMu.Lock()
	defer r.inFlightMu.Unlock()
	if _, ok := r.inFlightSet[id]; ok {
		return false
	}
	r.inFlightSet[id] = struct{}{}
	return true
}

func (r *MonitorJobRunner) releaseTicket(id int64) {
	r.inFlightMu.Lock()
	defer r.inFlightMu.Unlock()
	delete(r.inFlightSet, id)
}

// isMonitorDue returns true when m has never been checked or when the last
// check is older than its IntervalSeconds.
func isMonitorDue(m *ChannelMonitor, now time.Time) bool {
	if m == nil || !m.Enabled {
		return false
	}
	if m.LastCheckedAt == nil {
		return true
	}
	interval := time.Duration(m.IntervalSeconds) * time.Second
	if interval <= 0 {
		return false
	}
	return now.Sub(*m.LastCheckedAt) >= interval
}
