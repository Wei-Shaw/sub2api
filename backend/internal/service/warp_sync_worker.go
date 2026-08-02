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

const warpSyncWorkerLockKey = "warp_sync_worker"

// WarpSyncWorker periodically health-checks warp-gateway and syncs proxies into DB.
type WarpSyncWorker struct {
	cfg        *config.Config
	svc        *WarpSyncService
	lock       LeaderLockCache
	db         *sql.DB
	instanceID string
	log        *slog.Logger
	mu         sync.Mutex
	wg         sync.WaitGroup
	stop       chan struct{}
	on         bool
}

// ProvideWarpSyncWorker constructs and starts the worker when warp is enabled.
// lock may be nil (single-instance / tests); multi-instance deploys should pass Redis leader lock.
// db is used only when lock is nil: Postgres advisory single-flight. Redis configured
// but errored → skip tick (no DB fallthrough; anti split-brain).
func ProvideWarpSyncWorker(cfg *config.Config, svc *WarpSyncService, lock LeaderLockCache, db *sql.DB) *WarpSyncWorker {
	host, _ := os.Hostname()
	w := &WarpSyncWorker{
		cfg:        cfg,
		svc:        svc,
		lock:       lock,
		db:         db,
		instanceID: fmt.Sprintf("%s-%d", host, os.Getpid()),
		log:        slog.Default().With("component", "warp_sync_worker"),
		stop:       make(chan struct{}),
	}
	if svc != nil {
		// Share lock with admin SyncFromGateway so both paths single-flight.
		svc.SetLeaderLock(lock, w.instanceID, db)
	}
	w.Start()
	return w
}

func (w *WarpSyncWorker) Start() {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.on {
		return
	}
	if w.cfg == nil || !w.cfg.Warp.Enabled || w.svc == nil || !w.svc.Enabled() {
		w.log.Info("warp sync worker not started (disabled or gateway not configured)")
		return
	}
	if w.stop == nil {
		w.stop = make(chan struct{})
	}
	w.on = true
	w.wg.Add(1)
	go w.run()
	w.log.Info("warp sync worker started", "interval_sec", w.intervalSec())
}

// Stop ends the background loop and allows a later Start (recreates stop chan).
func (w *WarpSyncWorker) Stop() {
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
	w.log.Info("warp sync worker stopped")
}

func (w *WarpSyncWorker) intervalSec() int {
	sec := 15
	if w.cfg != nil && w.cfg.Warp.Gateway.ReconcileInterval > 0 {
		sec = w.cfg.Warp.Gateway.ReconcileInterval
	}
	if sec < 5 {
		sec = 5
	}
	return sec
}

func (w *WarpSyncWorker) run() {
	defer w.wg.Done()
	// Boot delay to let gateway come up after process start.
	timer := time.NewTimer(8 * time.Second)
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

func (w *WarpSyncWorker) tick() {
	// Align tick deadline with leadership lock TTL so HealthAll+Sync under lock
	// is not canceled early (lock floor is 90s; was fixed 60s).
	tickTimeout := 90 * time.Second
	if w.svc != nil {
		if ttl := w.svc.syncLockTTL(); ttl > tickTimeout {
			tickTimeout = ttl
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), tickTimeout)
	defer cancel()

	// Leader lock + process singleflight live inside withSyncLeadership so admin
	// Sync / HealthAllAndSync share the same gate (avoids double-acquire).
	if w.svc == nil {
		return
	}
	group := ""
	if w.cfg != nil {
		group = w.cfg.Warp.DefaultGroupName
	}
	res, err := w.svc.HealthAllAndSync(ctx, group)
	if err != nil {
		if errors.Is(err, ErrWarpSyncBusy) {
			w.log.Debug("warp health-sync skipped: sync already running or peer holds lock")
			return
		}
		w.log.Warn("warp health-sync failed", "err", err)
		return
	}
	if res != nil && len(res.Alerts) > 0 {
		for _, a := range res.Alerts {
			w.log.Warn("warp alert", "msg", a)
		}
	}
	if res != nil {
		w.log.Debug("warp health-sync ok",
			"created", len(res.CreatedProxies),
			"updated", len(res.UpdatedProxies),
			"members", len(res.MemberIDs),
			"detached", len(res.DetachedIDs),
		)
	}
}
