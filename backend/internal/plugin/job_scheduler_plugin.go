// job_scheduler_plugin.go owns the per-plugin scheduler lifecycle:
// pluginScheduler struct + constructor + applySpecs validation +
// installSpec dispatch + start/stop + ticker/fixed-delay loops.
// The matching dispatch concerns (fire, sendLoop, recvLoop, ack
// handling, history) live in job_scheduler_dispatch.go so this file
// stays focused on lifecycle wiring (T43).

package plugin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/Wei-Shaw/sub2api/internal/pkg/leaderlock"
	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// pluginSchedulerStopGracePeriod bounds how long stop() waits for the cron
// scheduler's main loop to exit before falling through to ticker cleanup.
// We do NOT block on possibly long-running user jobs here.
const pluginSchedulerStopGracePeriod = 2 * time.Second

// pluginScheduler owns one plugin's cron / interval / fixed-delay state and
// the bidirectional stream goroutine pair. All public methods are safe for
// concurrent callers.
type pluginScheduler struct {
	pluginName string
	leaderLock leaderlock.Provider
	history    JobHistoryRecorder
	logger     *slog.Logger
	streamCtx  context.Context

	cron       *cron.Cron
	intervals  []*time.Ticker
	intervalWG sync.WaitGroup
	stopCh     chan struct{}
	stopOnce   sync.Once

	specs      map[string]*pluginsdk.JobSpec
	leaderOnly map[string]bool

	sendCh chan *pluginsdk.JobTrigger

	pendingMu sync.Mutex
	pending   map[string]*pendingTrigger // trigger_id -> info
}

type pendingTrigger struct {
	jobName string
	firedAt time.Time
	manual  bool
}

func newPluginScheduler(pluginName string, lock leaderlock.Provider, history JobHistoryRecorder, logger *slog.Logger, streamCtx context.Context) *pluginScheduler {
	if streamCtx == nil {
		streamCtx = context.Background()
	}
	return &pluginScheduler{
		pluginName: pluginName,
		leaderLock: lock,
		history:    history,
		logger:     loggerOrDefault(logger).With("plugin", pluginName),
		streamCtx:  streamCtx,
		cron:       cron.New(cron.WithParser(jobCronParser)),
		stopCh:     make(chan struct{}),
		specs:      make(map[string]*pluginsdk.JobSpec),
		leaderOnly: make(map[string]bool),
		sendCh:     make(chan *pluginsdk.JobTrigger, jobTriggerSendBuffer),
		pending:    make(map[string]*pendingTrigger),
	}
}

// applySpecs validates every spec, then registers cron entries / starts
// interval+fixed_delay goroutines. Validation errors short-circuit before
// any side effect so a malformed registration never half-installs.
func (ps *pluginScheduler) applySpecs(specs []*pluginsdk.JobSpec) error {
	// Pass 1: validate. Pulling this out keeps the schedule-install pass
	// side-effect-free on the failure path.
	for _, sp := range specs {
		if sp == nil {
			return errors.New("nil spec")
		}
		name := strings.TrimSpace(sp.GetName())
		if name == "" {
			return errors.New("spec name is required")
		}
		switch sp.GetKind() {
		case "interval":
			if sp.GetIntervalNanos() <= 0 {
				return fmt.Errorf("job %q interval must be > 0", name)
			}
		case "fixed_delay":
			if sp.GetFixedDelayNanos() <= 0 {
				return fmt.Errorf("job %q fixed_delay must be > 0", name)
			}
		case "cron":
			if _, err := jobCronParser.Parse(sp.GetCronSpec()); err != nil {
				return fmt.Errorf("job %q cron parse: %w", name, err)
			}
		default:
			return fmt.Errorf("job %q unknown kind %q", name, sp.GetKind())
		}
	}

	// Pass 2: install. Errors here are programmer errors (we just validated)
	// so we surface as 500-equivalent rather than caller errors.
	for _, sp := range specs {
		if err := ps.installSpec(sp); err != nil {
			return fmt.Errorf("install spec %q: %w", sp.GetName(), err)
		}
	}
	return nil
}

func (ps *pluginScheduler) installSpec(sp *pluginsdk.JobSpec) error {
	name := sp.GetName()
	ps.specs[name] = sp
	ps.leaderOnly[name] = sp.GetLeaderOnly()
	switch sp.GetKind() {
	case "interval":
		t := time.NewTicker(time.Duration(sp.GetIntervalNanos()))
		ps.intervals = append(ps.intervals, t)
		ps.intervalWG.Add(1)
		go ps.runTickerLoop(name, t.C)
	case "fixed_delay":
		// Fixed-delay is a goroutine that sleeps between runs rather than
		// using a ticker — we cannot start the next run until the previous
		// ack arrives. Approximation here: sleep the configured delay
		// between fires regardless of ack receipt; ack-based gating is a
		// V6+ refinement.
		ps.intervalWG.Add(1)
		go ps.runFixedDelayLoop(name, time.Duration(sp.GetFixedDelayNanos()))
	case "cron":
		if _, err := ps.cron.AddFunc(sp.GetCronSpec(), func() { ps.fire(name, false) }); err != nil {
			return err
		}
	}
	return nil
}

// start kicks off the cron scheduler. Interval/fixed-delay loops were
// launched in installSpec because they need the spec table populated; cron
// must be started here because AddFunc is allowed on a stopped cron but
// nothing fires until Start.
func (ps *pluginScheduler) start() {
	ps.cron.Start()
}

// stop cancels everything. Safe to call multiple times.
func (ps *pluginScheduler) stop() {
	ps.stopOnce.Do(func() {
		close(ps.stopCh)
		ctx := ps.cron.Stop()
		// Wait briefly for any in-flight cron-triggered fire goroutines
		// to settle. cron.Stop returns a ctx that completes when the
		// scheduler's main loop exits — we do NOT block on a possibly
		// long-running user job.
		select {
		case <-ctx.Done():
		case <-time.After(pluginSchedulerStopGracePeriod):
		}
		for _, t := range ps.intervals {
			t.Stop()
		}
		ps.intervalWG.Wait()
		// Drain pending acks as failures so the history reflects reality.
		ps.expirePending("plugin disconnected")
	})
}

// runTickerLoop fires once per ticker tick until stopCh closes.
func (ps *pluginScheduler) runTickerLoop(jobName string, tickC <-chan time.Time) {
	defer ps.intervalWG.Done()
	for {
		select {
		case <-ps.stopCh:
			return
		case <-tickC:
			ps.fire(jobName, false)
		}
	}
}

// runFixedDelayLoop is the analogue for fixed-delay specs. The naming
// matches V5-DESIGN's "TriggerFixedDelay" wording.
func (ps *pluginScheduler) runFixedDelayLoop(jobName string, delay time.Duration) {
	defer ps.intervalWG.Done()
	for {
		select {
		case <-ps.stopCh:
			return
		case <-time.After(delay):
			ps.fire(jobName, false)
		}
	}
}
