// Package leaderlock provides a Redis-first / DB-fallback leader lock primitive
// shared by the ops cleanup loop and the plugin job scheduler.
//
// Why a separate package:
//
//   - OpsCleanupService had this logic inlined; the plugin JobScheduler (V5
//     W2) needs the same semantics for leader_only triggers, and we did not
//     want a circular import between internal/service and internal/plugin.
//   - The contract is small enough (TryAcquire + release closure) that
//     re-implementing it in two places would invite drift in TTL handling and
//     advisory-lock fallback behaviour.
//
// Semantics:
//
//   - Redis SetNX + Lua-CAS-DEL release script when a *redis.Client is
//     supplied. Self-expiring on TTL so a host crash does not wedge the lock.
//   - PostgreSQL session-scoped pg_try_advisory_lock fallback when Redis is
//     unconfigured or returns an error. The advisory key is hashed with FNV-1a
//     to fit in int64; release closes the held connection so the lock is
//     released even if pg_advisory_unlock fails.
//   - In single-instance ("simple") deployments the caller can pass
//     SingleInstance=true to skip locking entirely and always run the work.
//
// All Acquire calls return (release, isLeader). When isLeader=false the lock
// is held by another instance; the caller should skip its work for this tick
// and try again later. release is nil in that case.
package leaderlock

import (
	"context"
	"database/sql"
	"hash/fnv"
	"log/slog"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// DefaultTTL is the conservative TTL applied when a caller does not pin its
// own. 30 minutes matches OpsCleanupService's historical lease.
const DefaultTTL = 30 * time.Minute

// dbReleaseTimeout caps how long pg_advisory_unlock is given before we close
// the held conn anyway. The Postgres backend will release the lock when the
// session ends, so timing out here is purely cosmetic — but we want to avoid
// wedging the caller's defer.
const dbReleaseTimeout = 2 * time.Second

// releaseScript matches OpsCleanupService.opsCleanupReleaseScript: only the
// holder can DEL its own key, and a stale read is treated as already-released.
var releaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)

// Provider is the runtime entry point: callers TryAcquire by key, run their
// work if isLeader, then defer release().
//
// All implementations are safe for concurrent use across goroutines so a
// single Provider can serve every cron job in the host.
type Provider interface {
	TryAcquire(ctx context.Context, key string) (release func(), isLeader bool)
}

// Config captures the pieces a redisProvider needs at construction time.
type Config struct {
	// InstanceID identifies this host so the Lua release script can refuse
	// to DEL a key the lease was reassigned to. UUIDs work; any unique-per-
	// process string is fine.
	InstanceID string

	// TTL is the lease lifetime. <=0 falls back to DefaultTTL.
	TTL time.Duration

	// SingleInstance signals "this is a simple-mode deployment, always
	// declare the caller leader and skip all locking". Mirrors the
	// RunModeSimple shortcut OpsCleanupService had inline.
	SingleInstance bool

	// Logger is used for the one-shot warnings when redis is missing or
	// errors out. nil falls back to slog.Default().
	Logger *slog.Logger
}

// New constructs a Provider that prefers Redis and falls back to a Postgres
// advisory lock. Either backend may be nil; if both are nil and
// SingleInstance is false, every TryAcquire returns isLeader=false so callers
// fail closed rather than running unlocked work on every node.
func New(rdb *redis.Client, db *sql.DB, cfg Config) Provider {
	if cfg.TTL <= 0 {
		cfg.TTL = DefaultTTL
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &provider{
		rdb: rdb,
		db:  db,
		cfg: cfg,
	}
}

type provider struct {
	rdb *redis.Client
	db  *sql.DB
	cfg Config

	warnRedisErrOnce sync.Once
	warnNoRedisOnce  sync.Once
	warnNoBackend    sync.Once
}

func (p *provider) TryAcquire(ctx context.Context, key string) (func(), bool) {
	if p.cfg.SingleInstance {
		return noop, true
	}
	if p.rdb != nil {
		release, ok, redisErr := p.tryRedis(ctx, key)
		if redisErr == nil {
			return release, ok
		}
		p.warnRedisErrOnce.Do(func() {
			p.cfg.Logger.Warn("leaderlock: redis SetNX failed; falling back to DB advisory lock",
				"error", redisErr, "key", key)
		})
	} else {
		p.warnNoRedisOnce.Do(func() {
			p.cfg.Logger.Info("leaderlock: redis not configured; using DB advisory lock", "key", key)
		})
	}
	if p.db != nil {
		return tryAcquireDBAdvisoryLock(ctx, p.db, hashLockID(key))
	}
	p.warnNoBackend.Do(func() {
		p.cfg.Logger.Warn("leaderlock: no redis or db backend; refusing leadership", "key", key)
	})
	return nil, false
}

// tryRedis returns (release, isLeader, err). err!=nil signals "Redis is
// flaky right now, try the DB fallback"; (nil, false, nil) means Redis is
// healthy but the lock is held elsewhere.
func (p *provider) tryRedis(ctx context.Context, key string) (func(), bool, error) {
	ok, err := p.rdb.SetNX(ctx, key, p.cfg.InstanceID, p.cfg.TTL).Result()
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}
	rdb := p.rdb
	instanceID := p.cfg.InstanceID
	logger := p.cfg.Logger
	release := func() {
		// Use a fresh background ctx with a tight cap so a cancelled caller
		// ctx does not leak the lease until the TTL elapses.
		releaseCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if _, runErr := releaseScript.Run(releaseCtx, rdb, []string{key}, instanceID).Result(); runErr != nil {
			logger.Warn("leaderlock: redis release script failed (lease will expire by TTL)",
				"error", runErr, "key", key)
		}
	}
	return release, true, nil
}

// tryAcquireDBAdvisoryLock holds the connection that owns the
// session-scoped advisory lock so release() can call pg_advisory_unlock on
// the same backend. Returns (nil, false) when the lock is held by another
// session or the DB is misbehaving.
func tryAcquireDBAdvisoryLock(ctx context.Context, db *sql.DB, lockID int64) (func(), bool) {
	if db == nil {
		return nil, false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, false
	}
	acquired := false
	if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", lockID).Scan(&acquired); err != nil {
		_ = conn.Close()
		return nil, false
	}
	if !acquired {
		_ = conn.Close()
		return nil, false
	}
	release := func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), dbReleaseTimeout)
		defer cancel()
		_, _ = conn.ExecContext(unlockCtx, "SELECT pg_advisory_unlock($1)", lockID)
		_ = conn.Close()
	}
	return release, true
}

// hashLockID is the FNV-1a hash used to project an arbitrary string key into
// the int64 namespace pg_try_advisory_lock expects.
func hashLockID(key string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	return int64(h.Sum64()) //nolint:gosec
}

// noop is the release closure returned in single-instance mode where there
// is nothing to release.
func noop() {}
