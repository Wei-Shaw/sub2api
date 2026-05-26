package pluginsdk

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"github.com/Wei-Shaw/sub2api/plugin-sdk/streamutil"
)

// JobTriggerKind is the trigger discriminator for a JobSpec.
//
// Each spec carries exactly one populated trigger value (Interval, CronSpec,
// or FixedDelay) chosen by Kind. The host derives the wire JobSpec.kind
// string from this enum so the host scheduler can branch without sniffing
// which numeric field is non-zero.
type JobTriggerKind string

// Trigger kind constants. The literal values cross the wire as JobSpec.kind
// and match the host scheduler's switch arms exactly — do not localise.
const (
	TriggerInterval   JobTriggerKind = "interval"
	TriggerCron       JobTriggerKind = "cron"
	TriggerFixedDelay JobTriggerKind = "fixed_delay"
)

// Default per-spec values used when the plugin leaves them at zero. The host
// also clamps these defensively, but applying them on the SDK side keeps the
// wire payload self-describing for log review.
const (
	defaultJobConcurrency = 1
	defaultJobTimeout     = 5 * time.Minute
)

// Reconnect backoff schedule for the Subscribe stream. The schedule
// itself runs through streamutil.Loop; constants stay here because
// V5-DESIGN §2.6 reads the cadence off this file.
//
// Historical note: the original schedule was 1s → 3s → 9s → 30s (×3).
// The SDK-wide unification dropped to ×2 (1s → 2s → 4s → 8s → … → 30s)
// so a flap that recovers in <30s gets several reconnect attempts in
// the same window instead of three coarse ones. Audit T6 in
// PLUGIN_SYSTEM_HANDOFF tracks the rationale.
const (
	jobReconnectInitialBackoff = 1 * time.Second
	jobReconnectMaxBackoff     = 30 * time.Second
	jobReconnectMultiplier     = 2.0
	jobReconnectJitterRatio    = 0.2
)

// JobTrigger declares how a JobSpec fires. Exactly one of Interval, CronSpec,
// or FixedDelay is meaningful depending on Kind; the others are ignored.
type JobTrigger struct {
	Kind JobTriggerKind

	// Interval is the period for Kind=TriggerInterval.
	Interval time.Duration

	// CronSpec is a robfig/cron/v3 expression for Kind=TriggerCron. The host
	// is configured with WithSeconds(), so 5- or 6-field forms are accepted.
	CronSpec string

	// FixedDelay is the gap the host waits between the end of one run and
	// the start of the next for Kind=TriggerFixedDelay. Used when adjacent
	// runs must NOT overlap and "interval starts from previous completion".
	FixedDelay time.Duration
}

// JobSpec is the plugin-facing job description. The SDK converts it to the
// wire JobSpec at Register time.
type JobSpec struct {
	// Name is the stable identifier admin/UI uses to reference the job (e.g.
	// "monitor.run"). Unique per plugin.
	Name string

	Trigger JobTrigger

	// LeaderOnly=true asks the host to fire only on the leader node, using a
	// per-(plugin, job) lock key. Required for daily rollups and other
	// singleton jobs that must not run on every replica.
	LeaderOnly bool

	// Concurrency caps the number of in-flight handler invocations for THIS
	// job in the plugin process. <=0 means 1.
	Concurrency int

	// Timeout is the wall-clock cap on a single handler invocation. <=0
	// means defaultJobTimeout.
	Timeout time.Duration
}

// JobHandler runs in the plugin process when the host fires a trigger. The
// returned error is reported back to the host via JobAck.error and surfaces
// in admin job history.
type JobHandler func(ctx context.Context, jobName string) error

// JobsClient is the plugin-facing entry point exposed via PluginContext.Jobs().
type JobsClient interface {
	// Register declares one job. May be called multiple times before the
	// plugin returns from Init(); the SDK ships the cumulative spec list to
	// the host on the next Subscribe stream.
	//
	// Registering the same name twice replaces the previous handler/spec.
	Register(spec JobSpec, h JobHandler) error

	// TriggerLocal fires the named handler synchronously inside the plugin
	// process WITHOUT going through the host. Returns ErrJobUnknown when no
	// handler is registered for the name. Provided for plugin-internal unit
	// tests; admin "Run now" must go through the host so leader_only and
	// history are honoured.
	TriggerLocal(ctx context.Context, name string) error
}

// Sentinel errors returned by JobsClient.
var (
	ErrJobUnknown     = errors.New("pluginsdk: no handler registered for job")
	ErrJobsNotReady   = errors.New("pluginsdk: jobs client not initialised")
	ErrJobsRegistered = errors.New("pluginsdk: cannot register after Subscribe stream is open")
)

// jobsClient is the concrete JobsClient. It is created during Init and lives
// for the plugin process lifetime; it owns the Subscribe stream goroutine.
type jobsClient struct {
	pluginName string
	logger     *slog.Logger

	// dial returns a new pb.JobScheduler_SubscribeClient. Captured as a
	// closure so the reconnect loop can re-dial without keeping the gRPC
	// client around in this struct.
	dial func(context.Context) (pb.JobScheduler_SubscribeClient, error)

	mu       sync.Mutex
	specs    map[string]*pb.JobSpec
	handlers map[string]JobHandler
	timeouts map[string]time.Duration
	semas    map[string]chan struct{}
	started  bool

	streamMu sync.Mutex
	stream   pb.JobScheduler_SubscribeClient

	// loopCancel cancels the run loop ctx when the plugin shuts down.
	loopCancel context.CancelFunc
	loopDone   chan struct{}
}

// newJobsClient builds an unstarted jobs client. The caller wires it into
// PluginContext.Jobs() and triggers start() once Init() succeeds so handlers
// registered during Plugin.Init() are visible on the first Subscribe.
func newJobsClient(pluginName string, logger *slog.Logger, dial func(context.Context) (pb.JobScheduler_SubscribeClient, error)) *jobsClient {
	if logger == nil {
		logger = slog.Default()
	}
	return &jobsClient{
		pluginName: pluginName,
		logger:     logger.With("component", "plugin_jobs"),
		dial:       dial,
		specs:      make(map[string]*pb.JobSpec),
		handlers:   make(map[string]JobHandler),
		timeouts:   make(map[string]time.Duration),
		semas:      make(map[string]chan struct{}),
	}
}

// Register stores the spec + handler so they go out on the next Subscribe
// stream. Registering after the stream is already open returns
// ErrJobsRegistered to flag the lifecycle mistake — plugins must register
// every job before returning from Init().
func (c *jobsClient) Register(spec JobSpec, h JobHandler) error {
	if c == nil {
		return ErrJobsNotReady
	}
	if spec.Name == "" {
		return errors.New("pluginsdk: job spec name is required")
	}
	if h == nil {
		return fmt.Errorf("pluginsdk: job %q handler is nil", spec.Name)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.started {
		return ErrJobsRegistered
	}
	pbSpec, err := convertJobSpec(spec)
	if err != nil {
		return err
	}
	concurrency := spec.Concurrency
	if concurrency <= 0 {
		concurrency = defaultJobConcurrency
	}
	timeout := spec.Timeout
	if timeout <= 0 {
		timeout = defaultJobTimeout
	}
	c.specs[spec.Name] = pbSpec
	c.handlers[spec.Name] = h
	c.timeouts[spec.Name] = timeout
	c.semas[spec.Name] = make(chan struct{}, concurrency)
	return nil
}

// TriggerLocal runs the handler synchronously without involving the host.
// Useful for unit tests.
func (c *jobsClient) TriggerLocal(ctx context.Context, name string) error {
	if c == nil {
		return ErrJobsNotReady
	}
	c.mu.Lock()
	h := c.handlers[name]
	timeout := c.timeouts[name]
	c.mu.Unlock()
	if h == nil {
		return ErrJobUnknown
	}
	if timeout <= 0 {
		timeout = defaultJobTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return h(runCtx, name)
}

// start kicks off the Subscribe loop goroutine. Called by the SDK runner
// after Plugin.Init() returns successfully and there is at least one spec.
func (c *jobsClient) start(parent context.Context) {
	c.mu.Lock()
	if c.started || len(c.specs) == 0 {
		c.mu.Unlock()
		return
	}
	c.started = true
	c.mu.Unlock()

	ctx, cancel := context.WithCancel(parent)
	c.loopCancel = cancel
	c.loopDone = make(chan struct{})
	go c.run(ctx)
}

// stop cancels the run loop and waits for the goroutine to exit. Best-effort
// — caller should bound this with a timeout.
func (c *jobsClient) stop() {
	if c == nil || c.loopCancel == nil {
		return
	}
	c.loopCancel()
	if c.loopDone != nil {
		<-c.loopDone
	}
}

// run is the reconnect loop: open Subscribe → send Register → drain triggers
// → on broken stream sleep with backoff → repeat. Exits when ctx is
// cancelled (plugin shutdown) OR when the host returns a fatal status
// code that retrying cannot fix. The reconnect schedule comes from
// streamutil.LoopWithRetryClass so jobs / settings / events / log_remote
// share one cadence; GRPCDefaultClassifier marks PermissionDenied (e.g.
// the host's job_scheduler_server now rejects anonymous callers) and
// InvalidArgument (e.g. an undeclared job spec) as fatal so a
// mis-configured plugin fails fast instead of hammering the host.
//
// jobs.start is invoked fire-and-forget from runner.go; if run exits
// fatally, the plugin process keeps serving the rest of its surfaces
// (settings / events / migrations / DB), it just no longer receives job
// triggers. We log at error level here so operators can correlate the
// loss of job dispatch with the underlying capability / spec error.
//
// 复用 (1)：与 events / settings / log_remote 共用 GRPCDefaultClassifier，
// 没人需要再 copy 同一份 fatal-code 表。
func (c *jobsClient) run(ctx context.Context) {
	defer close(c.loopDone)
	err := streamutil.LoopWithRetryClass(ctx, streamutil.Config{
		Name:        "jobs.subscribe",
		Initial:     jobReconnectInitialBackoff,
		Max:         jobReconnectMaxBackoff,
		Multiplier:  jobReconnectMultiplier,
		JitterRatio: jobReconnectJitterRatio,
		Logger:      c.logger,
	}, streamutil.GRPCDefaultClassifier, c.runOnce)
	if err != nil && !errors.Is(err, context.Canceled) &&
		!errors.Is(err, context.DeadlineExceeded) && c.logger != nil {
		// LoopWithRetryClass already logged at error level when the
		// classifier marked the failure fatal; we add the plugin name so
		// operators can grep the host's log for the corresponding
		// capability / spec rejection.
		c.logger.Error("jobs subscribe stopped; not retrying — plugin will no longer receive triggers",
			"plugin", c.pluginName,
			"error", err)
	}
}

// runOnce opens one stream, sends the Register frame, and drains triggers
// until the stream breaks or ctx is cancelled.
func (c *jobsClient) runOnce(ctx context.Context) error {
	stream, err := c.dial(ctx)
	if err != nil {
		return fmt.Errorf("dial subscribe: %w", err)
	}
	c.streamMu.Lock()
	c.stream = stream
	c.streamMu.Unlock()
	defer func() {
		c.streamMu.Lock()
		c.stream = nil
		c.streamMu.Unlock()
	}()

	c.mu.Lock()
	pbSpecs := make([]*pb.JobSpec, 0, len(c.specs))
	for _, s := range c.specs {
		pbSpecs = append(pbSpecs, s)
	}
	c.mu.Unlock()

	if err := stream.Send(&pb.JobMessage{
		Msg: &pb.JobMessage_Register{Register: &pb.JobRegistration{Specs: pbSpecs}},
	}); err != nil {
		return fmt.Errorf("send register: %w", err)
	}
	c.logger.Info("job registration sent",
		"plugin", c.pluginName,
		"specs", len(pbSpecs),
	)

	for {
		if ctx.Err() != nil {
			return nil
		}
		trig, err := stream.Recv()
		if err != nil {
			return fmt.Errorf("recv trigger: %w", err)
		}
		go c.dispatch(ctx, trig)
	}
}

// dispatch is the per-trigger goroutine: acquire concurrency slot, run the
// handler under a timeout, send back JobAck.
func (c *jobsClient) dispatch(ctx context.Context, trig *pb.JobTrigger) {
	if trig == nil {
		return
	}
	name := trig.GetJobName()
	triggerID := trig.GetTriggerId()

	c.mu.Lock()
	handler := c.handlers[name]
	timeout := c.timeouts[name]
	sem := c.semas[name]
	c.mu.Unlock()

	if handler == nil {
		c.sendAck(triggerID, false, "no handler registered", 0)
		return
	}
	if timeout <= 0 {
		timeout = defaultJobTimeout
	}

	// Concurrency slot is non-blocking: an over-cap trigger acks failure
	// immediately so the host history records the throttle rather than the
	// SDK accumulating goroutines waiting on the semaphore.
	if sem != nil {
		select {
		case sem <- struct{}{}:
			defer func() { <-sem }()
		default:
			c.sendAck(triggerID, false, "concurrency limit reached", 0)
			return
		}
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	err := handler(runCtx, name)
	dur := time.Since(start)
	if err != nil {
		c.sendAck(triggerID, false, err.Error(), dur.Nanoseconds())
		return
	}
	c.sendAck(triggerID, true, "", dur.Nanoseconds())
}

// sendAck is the only path that writes JobAck back to the host. It guards
// against concurrent Send calls (gRPC streams require single-writer
// discipline) and silently drops the ack if the stream is gone — the host
// records timeout / missing ack as failure when correlation expires.
func (c *jobsClient) sendAck(triggerID string, success bool, errMsg string, durationNanos int64) {
	c.streamMu.Lock()
	stream := c.stream
	c.streamMu.Unlock()
	if stream == nil {
		c.logger.Warn("dropping ack: stream not open",
			"trigger_id", triggerID,
			"success", success,
		)
		return
	}
	if err := stream.Send(&pb.JobMessage{
		Msg: &pb.JobMessage_Ack{Ack: &pb.JobAck{
			TriggerId:     triggerID,
			Success:       success,
			Error:         errMsg,
			DurationNanos: durationNanos,
		}},
	}); err != nil {
		c.logger.Warn("send ack failed",
			"trigger_id", triggerID,
			"error", err,
		)
	}
}

// convertJobSpec maps the SDK-facing JobSpec onto the wire form. Validation
// catches the obvious lifecycle mistakes (kind/duration mismatch) early so
// the host does not have to reject a malformed Register frame at startup.
func convertJobSpec(spec JobSpec) (*pb.JobSpec, error) {
	pbSpec := &pb.JobSpec{
		Name:         spec.Name,
		LeaderOnly:   spec.LeaderOnly,
		Concurrency:  int32(spec.Concurrency),
		TimeoutNanos: spec.Timeout.Nanoseconds(),
	}
	switch spec.Trigger.Kind {
	case TriggerInterval:
		if spec.Trigger.Interval <= 0 {
			return nil, fmt.Errorf("pluginsdk: job %q interval must be > 0", spec.Name)
		}
		pbSpec.Kind = string(TriggerInterval)
		pbSpec.IntervalNanos = spec.Trigger.Interval.Nanoseconds()
	case TriggerCron:
		if spec.Trigger.CronSpec == "" {
			return nil, fmt.Errorf("pluginsdk: job %q cron spec must be non-empty", spec.Name)
		}
		pbSpec.Kind = string(TriggerCron)
		pbSpec.CronSpec = spec.Trigger.CronSpec
	case TriggerFixedDelay:
		if spec.Trigger.FixedDelay <= 0 {
			return nil, fmt.Errorf("pluginsdk: job %q fixed_delay must be > 0", spec.Name)
		}
		pbSpec.Kind = string(TriggerFixedDelay)
		pbSpec.FixedDelayNanos = spec.Trigger.FixedDelay.Nanoseconds()
	default:
		return nil, fmt.Errorf("pluginsdk: job %q unknown trigger kind %q", spec.Name, spec.Trigger.Kind)
	}
	return pbSpec, nil
}
