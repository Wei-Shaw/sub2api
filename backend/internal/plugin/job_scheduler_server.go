package plugin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

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
// instance per process; caller identity is supplied by the
// RequirePluginIdentityStream interceptor — the server reads it from the
// stream context via CallerFromContext.
type JobSchedulerServer struct {
	pluginsdk.UnimplementedJobSchedulerServer

	leaderLock leaderlock.Provider
	history    JobHistoryRecorder
	logger     *slog.Logger

	// schedulers holds the active per-plugin scheduler keyed by plugin name.
	// We only allow one active subscription per plugin at a time — a second
	// stream displaces the first to handle plugin reconnect cleanly.
	schedulersMu sync.Mutex
	schedulers   map[string]*pluginScheduler
}

// NewJobSchedulerServer wires the host job scheduler. caller identity is
// 由 RequirePluginIdentityStream 拦截器注入 stream ctx, Subscribe 走
// CallerFromContext 取出, 不再持有 resolver 字段。
func NewJobSchedulerServer(lock leaderlock.Provider, history JobHistoryRecorder, logger *slog.Logger) *JobSchedulerServer {
	return &JobSchedulerServer{
		leaderLock: lock,
		history:    history,
		logger:     loggerOrDefault(logger).With("component", "plugin_job_scheduler"),
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
	pluginName, ok := CallerFromContext(stream.Context())
	if !ok || pluginName == "" {
		// Defence-in-depth: 生产路径上拦截器已经拒绝匿名调用; 这里覆盖
		// 直接调用 Subscribe 的测试场景。
		return status.Error(codes.PermissionDenied, "missing plugin caller identity")
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
//
// ManualFire 由 admin handler 直接调用(不经 gRPC), 这里仍返回普通 error;
// 调用方负责把它映射到 HTTP 4xx/5xx。
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
