package service

import (
	"context"
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
func ProvideProxyHealthWorker(
	cfg *config.Config,
	svc *ProxyHealthService,
	lock LeaderLockCache,
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
	w.on = true
	w.wg.Add(1)
	go w.run()
	w.log.Info("proxy health worker started", "interval_sec", w.intervalSec())
}

// Stop ends the background loop.
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
	w.mu.Unlock()
	w.wg.Wait()
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

func (w *ProxyHealthWorker) lockTTL() time.Duration {
	sec := 50
	if w.cfg != nil && w.cfg.ProxyHealth.LeaderLockTTLSec > 0 {
		sec = w.cfg.ProxyHealth.LeaderLockTTLSec
	}
	ttl := time.Duration(sec) * time.Second
	// Keep lock longer than one interval so multi-instance ticks do not pile up.
	minTTL := time.Duration(w.intervalSec()) * time.Second
	if ttl < minTTL {
		ttl = minTTL
	}
	return ttl
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

func (w *ProxyHealthWorker) tick() {
	if w.svc == nil {
		return
	}
	// Bound whole tick: probes themselves use per-proxy timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if w.lock != nil {
		ok, err := w.lock.TryAcquireLeaderLock(ctx, proxyHealthWorkerLockKey, w.instanceID, w.lockTTL())
		if err != nil {
			w.log.Warn("proxy health leader lock error, skip tick", "err", err)
			return
		}
		if !ok {
			return
		}
		defer func() {
			_ = w.lock.ReleaseLeaderLock(context.Background(), proxyHealthWorkerLockKey, w.instanceID)
		}()
	}

	res, err := w.svc.RunOnce(ctx)
	if err != nil {
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
