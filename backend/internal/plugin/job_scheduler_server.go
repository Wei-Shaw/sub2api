package plugin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Wei-Shaw/sub2api/internal/pkg/leaderlock"
	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// V5 W2 — JobScheduler host implementation.
//
// JobSchedulerServer fans out scheduler ticks (cron + interval + fixed-delay)
// to plugins over a single bidirectional stream per (plugin, process).
// Plugins register their JobSpec list as the first frame of the stream; the
// host owns the clock, leader-only coordination, and per-trigger correlation.
//
// See V5-DESIGN §2 (W2 JobSchedulerCapability) for the full protocol.

const (
	// jobLeaderLockPrefix is the namespace for per-(plugin, job) leader
	// lock keys. Distinct from the ops_cleanup leader key so the two never
	// collide.
	jobLeaderLockPrefix = "plugin-job:"

	// jobLeaderLockTTL bounds how long a missing host can hold the lease.
	// Matches V5-DESIGN: at most 30s of triggers may be lost on host
	// restart.
	jobLeaderLockTTL = 30 * time.Second

	// jobTriggerSendBuffer is the per-plugin queue depth for outbound
	// triggers. Cron + interval will not produce bursts large enough to
	// overflow this in practice; if the buffer fills the firing goroutine
	// drops the trigger and logs (rather than blocking the cron internals).
	jobTriggerSendBuffer = 64
)

// jobCronParser supports robfig/cron's 5-field standard plus an optional
// leading seconds field, matching what plugin-sdk advertises for cron specs.
var jobCronParser = cron.NewParser(
	cron.SecondOptional |
		cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow |
		cron.Descriptor,
)

// JobHistoryRecorder is the host's persistence hook. The W2.4 admin handler
// supplies a real implementation backed by plugin_job_history; tests pass a
// stub. Nil is allowed — the server logs and skips.
type JobHistoryRecorder interface {
	RecordRun(ctx context.Context, run JobRunRecord)
}

// JobRunRecord is what gets persisted per trigger ack (or per timeout).
type JobRunRecord struct {
	PluginName string
	JobName    string
	TriggerID  string
	FiredAt    time.Time
	AckedAt    time.Time
	Success    bool
	Manual     bool
	Error      string
	Duration   time.Duration
}

// JobSchedulerServer implements the host side of the JobScheduler RPC. One
// instance per process; resolver maps stream → plugin name (delegated to the
// SDKServer's metadata-based caller identity).
type JobSchedulerServer struct {
	pluginsdk.UnimplementedJobSchedulerServer

	resolver   func(ctx context.Context) string
	leaderLock leaderlock.Provider
	history    JobHistoryRecorder
	logger     *slog.Logger

	// schedulers holds the active per-plugin scheduler keyed by plugin name.
	// We only allow one active subscription per plugin at a time — a second
	// stream displaces the first to handle plugin reconnect cleanly.
	schedulersMu sync.Mutex
	schedulers   map[string]*pluginScheduler
}

// NewJobSchedulerServer wires the host job scheduler. resolver is typically
// SDKServer.resolveCaller; passing nil makes every Subscribe reject as
// unauthenticated which is rarely useful outside tests.
func NewJobSchedulerServer(resolver func(context.Context) string, lock leaderlock.Provider, history JobHistoryRecorder, logger *slog.Logger) *JobSchedulerServer {
	if logger == nil {
		logger = slog.Default()
	}
	return &JobSchedulerServer{
		resolver:   resolver,
		leaderLock: lock,
		history:    history,
		logger:     logger.With("component", "plugin_job_scheduler"),
		schedulers: make(map[string]*pluginScheduler),
	}
}

// Stop tears down every active per-plugin scheduler. Safe to call multiple
// times; intended for graceful shutdown via PluginManager.ShutdownAll.
func (s *JobSchedulerServer) Stop() {
	if s == nil {
		return
	}
	s.schedulersMu.Lock()
	defer s.schedulersMu.Unlock()
	for name, ps := range s.schedulers {
		ps.stop()
		delete(s.schedulers, name)
	}
}

// Subscribe is the only RPC. The first message must be JobRegistration; after
// that the host sends JobTriggers and the plugin sends JobAck (and optional
// ManualTrigger replays from the admin "Run now" button — wired in W2.4).
func (s *JobSchedulerServer) Subscribe(stream pluginsdk.JobScheduler_SubscribeServer) error {
	if s == nil {
		return status.Error(codes.Internal, "job scheduler not initialised")
	}
	pluginName := ""
	if s.resolver != nil {
		pluginName = s.resolver(stream.Context())
	}
	if pluginName == "" {
		return status.Error(codes.Unauthenticated, "missing plugin caller identity")
	}

	first, err := stream.Recv()
	if err != nil {
		return status.Errorf(codes.Aborted, "recv first frame: %v", err)
	}
	reg := first.GetRegister()
	if reg == nil {
		return status.Error(codes.InvalidArgument, "first message must be JobMessage.Register")
	}

	ps, err := s.installScheduler(pluginName, reg.GetSpecs(), stream)
	if err != nil {
		return err
	}
	defer s.removeScheduler(pluginName, ps)
	defer ps.stop()

	ps.start()

	// Two goroutines: one drains acks, one pumps triggers. Either returning
	// breaks the stream.
	errCh := make(chan error, 2)
	go func() { errCh <- ps.recvLoop(stream) }()
	go func() { errCh <- ps.sendLoop(stream) }()
	return <-errCh
}

// installScheduler builds and registers a pluginScheduler atomically; if the
// plugin already had one (reconnect race) the previous scheduler is stopped
// first so we never double-fire triggers.
func (s *JobSchedulerServer) installScheduler(pluginName string, specs []*pluginsdk.JobSpec, stream pluginsdk.JobScheduler_SubscribeServer) (*pluginScheduler, error) {
	if len(specs) == 0 {
		return nil, status.Error(codes.InvalidArgument, "registration must include at least one spec")
	}
	ps := newPluginScheduler(pluginName, s.leaderLock, s.history, s.logger, stream.Context())
	if err := ps.applySpecs(specs); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	s.schedulersMu.Lock()
	if existing := s.schedulers[pluginName]; existing != nil {
		existing.stop()
	}
	s.schedulers[pluginName] = ps
	s.schedulersMu.Unlock()
	return ps, nil
}

// removeScheduler clears the per-plugin entry only if it still matches the
// scheduler we are tearing down. Guards against accidentally removing a
// newer scheduler installed by a reconnect.
func (s *JobSchedulerServer) removeScheduler(pluginName string, ps *pluginScheduler) {
	s.schedulersMu.Lock()
	defer s.schedulersMu.Unlock()
	if s.schedulers[pluginName] == ps {
		delete(s.schedulers, pluginName)
	}
}

// ManualFire pushes a synthetic JobTrigger for an admin "Run now" call. The
// W2.4 admin handler is the expected caller; returns an error if the plugin
// has no live subscription or the named job is unknown.
func (s *JobSchedulerServer) ManualFire(pluginName, jobName string) error {
	if s == nil {
		return errors.New("job scheduler not initialised")
	}
	s.schedulersMu.Lock()
	ps := s.schedulers[pluginName]
	s.schedulersMu.Unlock()
	if ps == nil {
		return fmt.Errorf("plugin %q has no active job subscription", pluginName)
	}
	return ps.manualFire(jobName)
}

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
	if logger == nil {
		logger = slog.Default()
	}
	if streamCtx == nil {
		streamCtx = context.Background()
	}
	return &pluginScheduler{
		pluginName: pluginName,
		leaderLock: lock,
		history:    history,
		logger:     logger.With("plugin", pluginName),
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
		case <-time.After(2 * time.Second):
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

// fire is the per-trigger entry point: leader check, build trigger, enqueue.
// All scheduler kinds (cron / interval / fixed_delay / manual) flow through
// here so the leader-only check and history bookkeeping live in one place.
func (ps *pluginScheduler) fire(jobName string, manual bool) {
	if ps.leaderOnly[jobName] {
		release, isLeader := ps.leaderLock.TryAcquire(ps.streamCtx,
			jobLeaderLockPrefix+ps.pluginName+":"+jobName)
		if !isLeader {
			return
		}
		// Release the lease as soon as we have queued the trigger; the
		// plugin handler is gated by its own concurrency cap on the SDK
		// side, not by holding the lock for the run duration. This mirrors
		// V5-DESIGN §2.5 and is critical for short TTLs (30s).
		if release != nil {
			defer release()
		}
	}

	triggerID := uuid.NewString()
	now := time.Now()
	ps.pendingMu.Lock()
	ps.pending[triggerID] = &pendingTrigger{
		jobName: jobName,
		firedAt: now,
		manual:  manual,
	}
	ps.pendingMu.Unlock()

	trigger := &pluginsdk.JobTrigger{
		JobName:          jobName,
		TriggerId:        triggerID,
		FireTimeUnixNano: now.UnixNano(),
		Manual:           manual,
	}
	select {
	case ps.sendCh <- trigger:
	default:
		// Buffer is full — surface as a failed run rather than deadlocking
		// the caller. This is an SLA event, not a user error.
		ps.pendingMu.Lock()
		delete(ps.pending, triggerID)
		ps.pendingMu.Unlock()
		ps.recordHistory(JobRunRecord{
			PluginName: ps.pluginName,
			JobName:    jobName,
			TriggerID:  triggerID,
			FiredAt:    now,
			AckedAt:    now,
			Success:    false,
			Manual:     manual,
			Error:      "send buffer full",
		})
		ps.logger.Warn("dropping trigger: send buffer full", "job", jobName)
	}
}

// manualFire is the public entry for ManualTrigger / admin Run-now. Returns
// an error so the admin handler can map it to HTTP 4xx if the job is
// unknown.
func (ps *pluginScheduler) manualFire(jobName string) error {
	if _, ok := ps.specs[jobName]; !ok {
		return fmt.Errorf("job %q is not registered", jobName)
	}
	go ps.fire(jobName, true)
	return nil
}

// sendLoop pumps triggers out to the plugin until the stream context cancels.
func (ps *pluginScheduler) sendLoop(stream pluginsdk.JobScheduler_SubscribeServer) error {
	for {
		select {
		case <-ps.stopCh:
			return nil
		case <-stream.Context().Done():
			return stream.Context().Err()
		case trig := <-ps.sendCh:
			if err := stream.Send(trig); err != nil {
				return err
			}
		}
	}
}

// recvLoop reads JobAck and ManualTrigger frames from the plugin until the
// stream closes. ManualTrigger arriving on this channel mirrors the protocol
// in V5-DESIGN §2.4 and is fanned out via fire() so the leader_only check
// still applies even to manual runs.
func (ps *pluginScheduler) recvLoop(stream pluginsdk.JobScheduler_SubscribeServer) error {
	for {
		msg, err := stream.Recv()
		if err != nil {
			return err
		}
		if ack := msg.GetAck(); ack != nil {
			ps.handleAck(ack)
			continue
		}
		if mt := msg.GetManual(); mt != nil {
			if ferr := ps.manualFire(mt.GetJobName()); ferr != nil {
				ps.logger.Warn("manual trigger rejected", "job", mt.GetJobName(), "error", ferr)
			}
			continue
		}
		// Register frames after the first are ignored — V5 protocol allows
		// only one Register per stream lifetime.
		ps.logger.Warn("unexpected message after registration", "kind", fmt.Sprintf("%T", msg.GetMsg()))
	}
}

func (ps *pluginScheduler) handleAck(ack *pluginsdk.JobAck) {
	id := ack.GetTriggerId()
	ps.pendingMu.Lock()
	pt, ok := ps.pending[id]
	if ok {
		delete(ps.pending, id)
	}
	ps.pendingMu.Unlock()
	if !ok {
		ps.logger.Debug("ack for unknown trigger", "trigger_id", id)
		return
	}
	now := time.Now()
	ps.recordHistory(JobRunRecord{
		PluginName: ps.pluginName,
		JobName:    pt.jobName,
		TriggerID:  id,
		FiredAt:    pt.firedAt,
		AckedAt:    now,
		Success:    ack.GetSuccess(),
		Manual:     pt.manual,
		Error:      ack.GetError(),
		Duration:   time.Duration(ack.GetDurationNanos()),
	})
}

// expirePending is called on shutdown: every still-pending trigger is logged
// as a failure with the supplied reason so admins can see "host shutdown
// during run" in the history.
func (ps *pluginScheduler) expirePending(reason string) {
	ps.pendingMu.Lock()
	pending := ps.pending
	ps.pending = make(map[string]*pendingTrigger)
	ps.pendingMu.Unlock()
	now := time.Now()
	for id, pt := range pending {
		ps.recordHistory(JobRunRecord{
			PluginName: ps.pluginName,
			JobName:    pt.jobName,
			TriggerID:  id,
			FiredAt:    pt.firedAt,
			AckedAt:    now,
			Success:    false,
			Manual:     pt.manual,
			Error:      reason,
			Duration:   now.Sub(pt.firedAt),
		})
	}
}

func (ps *pluginScheduler) recordHistory(rec JobRunRecord) {
	if ps.history == nil {
		return
	}
	// Use a detached background ctx so a shutting-down stream does not
	// abort the history write mid-flight.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ps.history.RecordRun(ctx, rec)
}
