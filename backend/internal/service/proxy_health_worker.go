package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const proxyHealthWorkerLockKey = "proxy_health_worker"

// ProxyHealthWorker periodically runs ProxyHealthService.RunOnce under a leader lock.
type ProxyHealthWorker struct {
	cfg        *config.Config
	svc        *ProxyHealthService
	lock       LeaderLockCache
	instanceID string
	log        *slog.Logger

	mu   sync.Mutex
	wg   sync.WaitGroup
	stop chan struct{}
	on   bool
}

// ProvideProxyHealthWorker constructs and starts the worker when enabled.
// metrics is optional (may be nil); counters live on the service when provided.
// db is used only when lock is nil (no Redis): Postgres advisory single-flight.
// When Redis lock is configured, acquire errors SKIP the tick (no DB fallthrough)
// to avoid Redis+DB split-brain; see tryAcquireSingletonLeaderLock.
func ProvideProxyHealthWorker(
	cfg *config.Config,
	svc *ProxyHealthService,
	lock LeaderLockCache,
	db *sql.DB,
) *ProxyHealthWorker {
	host, _ := os.Hostname()
	w := &ProxyHealthWorker{
		cfg:        cfg,
		svc:        svc,
		lock:       lock,
		instanceID: fmt.Sprintf("%s-%d", host, os.Getpid()),
		log:        slog.Default().With("component", "proxy_health_worker"),
		stop:       make(chan struct{}),
	}
	if svc != nil {
		svc.SetWorker(w)
		// Share the same leader lock with admin RunScan so both paths single-flight.
		svc.SetLeaderLock(lock, w.instanceID, db)
	}
	w.Start()
	return w
}

// Start begins the background loop (idempotent).
func (w *ProxyHealthWorker) Start() {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.on {
		return
	}
	if w.cfg == nil || !w.cfg.ProxyHealth.Enabled || w.svc == nil {
		w.log.Info("proxy health worker not started (disabled or service nil)")
		return
	}
	if w.stop == nil {
		w.stop = make(chan struct{})
	}
	w.on = true
	w.wg.Add(1)
	go w.run()
	w.log.Info("proxy health worker started", "interval_sec", w.intervalSec())
}

// Stop ends the background loop and allows a later Start/Apply.
func (w *ProxyHealthWorker) Stop() {
	if w == nil {
		return
	}
	w.mu.Lock()
	if !w.on {
		w.mu.Unlock()
		return
	}
	select {
	case <-w.stop:
	default:
		close(w.stop)
	}
	w.on = false
	w.mu.Unlock()
	w.wg.Wait()
	w.mu.Lock()
	w.stop = make(chan struct{})
	w.mu.Unlock()
	w.log.Info("proxy health worker stopped")
}

// Apply re-reads cfg.ProxyHealth.Enabled and starts or stops the loop.
func (w *ProxyHealthWorker) Apply() {
	if w == nil {
		return
	}
	enabled := w.cfg != nil && w.cfg.ProxyHealth.Enabled
	if enabled {
		// Restart so interval changes take effect promptly.
		if w.Running() {
			w.Stop()
		}
		w.Start()
		return
	}
	if w.Running() {
		w.Stop()
	}
}

// Running reports whether the background loop is active.
func (w *ProxyHealthWorker) Running() bool {
	if w == nil {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.on
}

// InstanceID returns the leader-lock owner token for this process.
func (w *ProxyHealthWorker) InstanceID() string {
	if w == nil {
		return ""
	}
	return w.instanceID
}

func (w *ProxyHealthWorker) intervalSec() int {
	sec := 60
	if w.cfg != nil && w.cfg.ProxyHealth.IntervalSec > 0 {
		sec = w.cfg.ProxyHealth.IntervalSec
	}
	if sec < 10 {
		sec = 10
	}
	return sec
}

func (w *ProxyHealthWorker) run() {
	defer w.wg.Done()
	// Short boot delay so Redis/DB are ready.
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-w.stop:
			return
		case <-timer.C:
			w.tick()
			timer.Reset(time.Duration(w.intervalSec()) * time.Second)
		}
	}
}

func (w *ProxyHealthWorker) tickTimeout() time.Duration {
	// Scale with batch × per-proxy timeout / concurrency, floor 2m, cap 15m.
	timeout := 2 * time.Minute
	if w.svc == nil {
		return timeout
	}
	cfg := w.svc.conf()
	batch := cfg.BatchSize
	if batch <= 0 {
		batch = 100
	}
	conc := cfg.Concurrency
	if conc <= 0 {
		conc = 8
	}
	perProxyMS := cfg.TimeoutMS
	if perProxyMS <= 0 {
		perProxyMS = 10000
	}
	if cfg.ProbeMode == "quality" && perProxyMS < 30000 {
		perProxyMS = 30000
	}
	// Rough wall time: ceil(batch/conc) * per-proxy, plus 30s slack.
	waves := (batch + conc - 1) / conc
	est := time.Duration(waves*perProxyMS)*time.Millisecond + 30*time.Second
	if est > timeout {
		timeout = est
	}
	if timeout > 15*time.Minute {
		timeout = 15 * time.Minute
	}
	return timeout
}

func (w *ProxyHealthWorker) tick() {
	if w.svc == nil {
		return
	}
	// Bound whole tick: probes themselves use per-proxy timeout.
	// Leader lock + process singleflight live inside RunOnce so admin RunScan
	// shares the same gate (avoids double-acquire if we locked here too).
	ctx, cancel := context.WithTimeout(context.Background(), w.tickTimeout())
	defer cancel()

	res, err := w.svc.RunOnce(ctx)
	if err != nil {
		if errors.Is(err, ErrProxyHealthScanBusy) {
			w.log.Debug("proxy health tick skipped: scan already running")
			return
		}
		w.log.Warn("proxy health run failed", "err", err)
		return
	}
	if res == nil {
		return
	}
	if res.Isolated > 0 || res.Recovered > 0 || res.Errors > 0 {
		w.log.Info("proxy health tick",
			"probed", res.Probed,
			"isolated", res.Isolated,
			"recovered", res.Recovered,
			"skipped", res.Skipped,
			"errors", res.Errors,
		)
	} else {
		w.log.Debug("proxy health tick",
			"probed", res.Probed,
			"skipped", res.Skipped,
		)
	}
}
