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
	JobNameMonitorRun          = "monitor.run"
	JobNameMonitorDailyRollup  = "monitor.daily-rollup"
	monitorJobRunInterval      = 60 * time.Second
	monitorJobDailyRollupSpec  = "0 2 * * *"
	monitorJobRunTimeout       = 2 * time.Minute
	monitorJobDailyRollupLimit = 10 * time.Minute
)

// MonitorJobRunner registers the per-tick "scan + check" job and the daily
// rollup job with the host JobScheduler. It replaces the old in-process
// ChannelMonitorRunner (291 lines of pond/sync.Map/leader-lock plumbing) —
// the host now owns scheduling, leader election, and concurrency.
//
// The runner also implements MonitorScheduler so service-side CRUD writes
// can prompt an immediate one-off check via TriggerNow once the SDK
// supports it. Until then the methods are a soft no-op (logged) — the
// 60-second tick will still pick up new monitors on the next pass.
type MonitorJobRunner struct {
	svc  *ChannelMonitorService
	jobs pluginsdk.JobsClient
	log  *slog.Logger

	mu       sync.Mutex
	tickerWG sync.WaitGroup
}

// NewMonitorJobRunner builds the runner. jobs may be nil (host without W2);
// in that case Register is a no-op and the plugin still serves admin /
// user APIs but never fires periodic checks.
func NewMonitorJobRunner(svc *ChannelMonitorService, jobs pluginsdk.JobsClient, log *slog.Logger) *MonitorJobRunner {
	if log == nil {
		log = slog.Default()
	}
	return &MonitorJobRunner{svc: svc, jobs: jobs, log: log}
}

// Register declares the two job specs to the host scheduler. Must be called
// from inside Plugin.Init before the SDK opens the Subscribe stream — the
// JobsClient rejects late Register calls with ErrJobsRegistered.
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
		Concurrency: 5,
		Timeout:     monitorJobRunTimeout,
	}
	if err := r.jobs.Register(tickSpec, r.runOneTick); err != nil {
		return fmt.Errorf("register %s: %w", JobNameMonitorRun, err)
	}

	rollupSpec := pluginsdk.JobSpec{
		Name: JobNameMonitorDailyRollup,
		Trigger: pluginsdk.JobTrigger{
			Kind:     pluginsdk.TriggerCron,
			CronSpec: monitorJobDailyRollupSpec,
		},
		LeaderOnly: true,
		Timeout:    monitorJobDailyRollupLimit,
	}
	if err := r.jobs.Register(rollupSpec, r.runDailyRollup); err != nil {
		return fmt.Errorf("register %s: %w", JobNameMonitorDailyRollup, err)
	}
	r.log.Info("channel-monitor: jobs registered",
		"jobs", []string{JobNameMonitorRun, JobNameMonitorDailyRollup})
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

// runDailyRollup is the job handler the host fires once per day on the
// leader replica. Delegates to the existing service-level maintenance.
func (r *MonitorJobRunner) runDailyRollup(ctx context.Context, _ string) error {
	if err := r.svc.RunDailyMaintenance(ctx); err != nil {
		return fmt.Errorf("daily maintenance: %w", err)
	}
	return nil
}

// inFlightSet tracks which monitor IDs are currently being checked so the
// per-tick scan does not double-fire while the previous attempt is still
// inside the upstream HTTP call. Replaces the old runner.inFlight sync.Map
// with a smaller mutex-protected map (the cardinality is at most ~hundreds
// of monitors, so contention is negligible).
var (
	inFlightMu  sync.Mutex
	inFlightSet = map[int64]struct{}{}
)

func (r *MonitorJobRunner) acquireTicket(id int64) bool {
	inFlightMu.Lock()
	defer inFlightMu.Unlock()
	if _, ok := inFlightSet[id]; ok {
		return false
	}
	inFlightSet[id] = struct{}{}
	return true
}

func (r *MonitorJobRunner) releaseTicket(id int64) {
	inFlightMu.Lock()
	defer inFlightMu.Unlock()
	delete(inFlightSet, id)
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

// Schedule / Unschedule satisfy the MonitorScheduler interface. With the
// host-driven JobScheduler the per-monitor task lifecycle is implicit (the
// next tick will pick up enabled rows / drop disabled ones), so these
// methods are intentionally no-ops; they exist so service-side CRUD
// writes can stay agnostic of which runner implementation is wired.
func (r *MonitorJobRunner) Schedule(_ *ChannelMonitor)   {}
func (r *MonitorJobRunner) Unschedule(_ int64)           {}
var _ MonitorScheduler = (*MonitorJobRunner)(nil)
