package service

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
	"time"
)

// LeaderLockCache provides cross-instance mutual exclusion for periodic background
// jobs. It is implemented in the repository layer (Redis-backed) so the service
// layer never depends on Redis directly. Release is a compare-and-delete keyed by
// owner so a stale holder can never delete a peer's lock.
type LeaderLockCache interface {
	// TryAcquireLeaderLock sets key=owner with the given TTL iff key is absent.
	// It returns true when the caller becomes the owner.
	TryAcquireLeaderLock(ctx context.Context, key, owner string, ttl time.Duration) (bool, error)
	// ReleaseLeaderLock deletes key iff it is still owned by owner.
	ReleaseLeaderLock(ctx context.Context, key, owner string) error
}

// LeaderLockRefresher is an optional extension implemented by Redis-backed locks.
// Holders of long critical sections (WARP register create, large health scans)
// should renew TTL so peers cannot enter while work is still running.
type LeaderLockRefresher interface {
	// RefreshLeaderLock extends TTL iff key is still owned by owner.
	// Returns true when the refresh succeeded.
	RefreshLeaderLock(ctx context.Context, key, owner string, ttl time.Duration) (bool, error)
}

// leaderLockHeartbeatInterval is the first/renew tick spacing for long-held locks.
// Uses min(ttl/3, 15s) floored at 2s so multi-minute TTLs (e.g. WARP register create)
// renew sooner than ttl/3 alone would allow, while short TTLs still tick at ttl/3.
func leaderLockHeartbeatInterval(ttl time.Duration) time.Duration {
	interval := ttl / 3
	const maxFirst = 15 * time.Second
	if interval > maxFirst {
		interval = maxFirst
	}
	if interval < 2*time.Second {
		interval = 2 * time.Second
	}
	return interval
}

// startLeaderLockHeartbeat renews the lock on a capped interval while work runs.
// It returns a derived context that is canceled when a refresh attempt fails
// (lost ownership or redis error) so in-flight work can abort, and a stop func
// that ends the heartbeat goroutine (safe to call multiple times).
// No-op when cache does not implement LeaderLockRefresher or ttl is tiny:
// parent is returned unchanged with a no-op stop.
// First refresh is after min(ttl/3, 15s) (≥2s) — does not cancel on start.
func startLeaderLockHeartbeat(parent context.Context, cache LeaderLockCache, key, owner string, ttl time.Duration) (ctx context.Context, stop func()) {
	if parent == nil {
		parent = context.Background()
	}
	refresher, ok := cache.(LeaderLockRefresher)
	if !ok || refresher == nil || ttl < 6*time.Second || key == "" || owner == "" {
		return parent, func() {}
	}
	interval := leaderLockHeartbeatInterval(ttl)
	hbCtx, cancel := context.WithCancel(parent)
	stopCh := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-parent.Done():
				return
			case <-t.C:
				rctx, rcancel := context.WithTimeout(context.Background(), 2*time.Second)
				ok, err := refresher.RefreshLeaderLock(rctx, key, owner, ttl)
				rcancel()
				if err != nil || !ok {
					slog.Warn("leader lock heartbeat lost ownership",
						"key", key,
						"owner", owner,
						"refresh_ok", ok,
						"err", err,
					)
					cancel()
					return
				}
			}
		}
	}()
	var once sync.Once
	return hbCtx, func() {
		once.Do(func() {
			close(stopCh)
			<-done
			cancel()
		})
	}
}

// tryAcquireSingletonLeaderLock provides best-effort single-flight execution of a
// periodic background job across multiple instances.
//
// Semantics (no Redis+DB split-brain):
//   - cache != nil, acquire ok  -> returns redis release and true.
//   - cache != nil, held by peer -> returns (nil, false).
//   - cache != nil, Redis error  -> SKIP (nil, false). Do NOT fall through to DB:
//     peers may still hold the Redis lock; falling through would double-run.
//   - cache == nil, db != nil    -> try Postgres advisory lock.
//   - no backend                 -> ungated no-op release and true (unit tests /
//     single-instance without Redis).
//
// The TTL is purely a crash-safety bound: callers release the lock as soon as the
// job completes, so leadership is re-contested every cycle rather than pinned to
// one instance. The TTL must therefore be larger than the job's worst-case
// runtime so the lock does not expire mid-run.
//
// Callers that must distinguish peer busy 409 vs Redis unavailable 503 should use
// tryAcquireSingletonLeaderLockEx instead.
func tryAcquireSingletonLeaderLock(ctx context.Context, cache LeaderLockCache, db *sql.DB, key, owner string, ttl time.Duration) (func(), bool) {
	rel, ok, _ := tryAcquireSingletonLeaderLockEx(ctx, cache, db, key, owner, ttl)
	return rel, ok
}

// tryAcquireSingletonLeaderLockEx is like tryAcquireSingletonLeaderLock but reports
// whether failure was due to the lock backend being unavailable (peer busy 409 vs Redis unavailable 503).
//
//   - backendUnavailable=true when cache!=nil and TryAcquire errors (Redis down/timeout, 503).
//   - Peer held → ok=false, backendUnavailable=false (409).
//   - Acquire success / DB advisory / ungated → backendUnavailable=false.
func tryAcquireSingletonLeaderLockEx(ctx context.Context, cache LeaderLockCache, db *sql.DB, key, owner string, ttl time.Duration) (release func(), ok bool, backendUnavailable bool) {
	if ctx == nil {
		ctx = context.Background()
	}

	if cache != nil {
		acquired, err := cache.TryAcquireLeaderLock(ctx, key, owner, ttl)
		if err == nil {
			if !acquired {
				return nil, false, false
			}
			release = func() {
				ctx2, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				_ = cache.ReleaseLeaderLock(ctx2, key, owner)
			}
			return release, true, false
		}
		// Redis configured but errored: SKIP. Do not fall through to DB —
		// peers may still hold the Redis lock (split-brain if we ran via DB).
		// Also covers canceled/deadline ctx: skip rather than stampede.
		return nil, false, true
	}

	if db != nil {
		rel, acquired := tryAcquireDBAdvisoryLock(ctx, db, hashAdvisoryLockID(key))
		return rel, acquired, false
	}

	// No coordination backend available (unit tests / single-instance without
	// Redis+DB): run without gating so the job is never silently starved.
	// Production always passes db (and usually cache) via SetLeaderLock.
	return func() {}, true, false
}
