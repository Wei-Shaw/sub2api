package service

import (
	"context"
	"database/sql"
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

// tryAcquireSingletonLeaderLock provides best-effort single-flight execution of a
// periodic background job across multiple instances. It prefers the Redis-backed
// LeaderLockCache and falls back to a Postgres advisory lock when the cache is
// unavailable or errors, mirroring the approach used by the Ops background
// services.
//
// Semantics:
//   - acquired      -> returns a non-nil release func and true; callers should
//     defer the release once the job finishes.
//   - held by peer  -> returns (nil, false); callers should skip this cycle.
//   - no backend    -> when neither the cache nor a DB is configured (e.g. unit
//     tests, or a single-instance deployment without Redis) it runs without
//     gating, returning a no-op release and true, so the job is never silently
//     starved.
//
// The TTL is purely a crash-safety bound: callers release the lock as soon as the
// job completes, so leadership is re-contested every cycle rather than pinned to
// one instance. The TTL must therefore be larger than the job's worst-case
// runtime so the lock does not expire mid-run.
func tryAcquireSingletonLeaderLock(ctx context.Context, cache LeaderLockCache, db *sql.DB, key, owner string, ttl time.Duration) (func(), bool) {
	if ctx == nil {
		ctx = context.Background()
	}

	if cache != nil {
		ok, err := cache.TryAcquireLeaderLock(ctx, key, owner, ttl)
		if err == nil {
			if !ok {
				return nil, false
			}
			release := func() {
				ctx2, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				_ = cache.ReleaseLeaderLock(ctx2, key, owner)
			}
			return release, true
		}
		// Cache error: fall through to the DB advisory lock so a flaky Redis does
		// not stampede the job across every instance.
	}

	if db != nil {
		return tryAcquireDBAdvisoryLock(ctx, db, hashAdvisoryLockID(key))
	}

	// No coordination backend available: run without gating.
	return func() {}, true
}
