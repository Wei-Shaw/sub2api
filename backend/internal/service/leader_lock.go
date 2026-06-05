package service

import (
	"context"
	"database/sql"
	"time"

	"github.com/redis/go-redis/v9"
)

// leaderLockReleaseScript releases a leader lock only when the caller still owns
// it (compare-and-delete by instance ID). This prevents a previous holder whose
// lock already expired — and was re-acquired by another instance — from deleting
// the new owner's lock.
var leaderLockReleaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)

// tryAcquireSingletonLeaderLock provides best-effort single-flight execution of a
// periodic background job across multiple instances. It prefers a Redis lock
// (SetNX with a TTL) and falls back to a Postgres advisory lock when Redis is
// unavailable, mirroring the approach used by the Ops background services.
//
// Semantics:
//   - acquired      -> returns a non-nil release func and true; callers should
//     defer the release once the job finishes.
//   - held by peer  -> returns (nil, false); callers should skip this cycle.
//   - no backend    -> when neither Redis nor DB is configured (e.g. unit tests,
//     or a single-instance deployment without Redis) it runs without gating,
//     returning a no-op release and true, so the job is never silently starved.
//
// The TTL is purely a crash-safety bound: callers release the lock as soon as the
// job completes, so leadership is re-contested every cycle rather than pinned to
// one instance. The TTL must therefore be larger than the job's worst-case
// runtime so the lock does not expire mid-run.
func tryAcquireSingletonLeaderLock(ctx context.Context, rdb *redis.Client, db *sql.DB, key, instanceID string, ttl time.Duration) (func(), bool) {
	if ctx == nil {
		ctx = context.Background()
	}

	if rdb != nil {
		ok, err := rdb.SetNX(ctx, key, instanceID, ttl).Result()
		if err == nil {
			if !ok {
				return nil, false
			}
			release := func() {
				ctx2, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				_, _ = leaderLockReleaseScript.Run(ctx2, rdb, []string{key}, instanceID).Result()
			}
			return release, true
		}
		// Redis error: fall through to the DB advisory lock so a flaky Redis does
		// not stampede the job across every instance.
	}

	if db != nil {
		return tryAcquireDBAdvisoryLock(ctx, db, hashAdvisoryLockID(key))
	}

	// No coordination backend available: run without gating.
	return func() {}, true
}
