package repository

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/redis/go-redis/v9"
)

const leaderLockKeyPrefix = "leader:lock:"

// leaderLockReleaseScript releases a leader lock only when the caller still owns
// it (compare-and-delete by owner token). This prevents a previous holder whose
// lock already expired — and was re-acquired by another instance — from deleting
// the new owner's lock.
var leaderLockReleaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)

// leaderLockRefreshScript extends TTL only while the caller still owns the lock.
var leaderLockRefreshScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
return 0
`)

type leaderLockCache struct {
	rdb *redis.Client
}

// NewLeaderLockCache returns a Redis-backed implementation of
// service.LeaderLockCache used by periodic background jobs to elect a single
// runner across instances. Also implements service.LeaderLockRefresher.
func NewLeaderLockCache(rdb *redis.Client) service.LeaderLockCache {
	return &leaderLockCache{rdb: rdb}
}

func (c *leaderLockCache) TryAcquireLeaderLock(ctx context.Context, key, owner string, ttl time.Duration) (bool, error) {
	return c.rdb.SetNX(ctx, leaderLockKeyPrefix+key, owner, ttl).Result()
}

func (c *leaderLockCache) ReleaseLeaderLock(ctx context.Context, key, owner string) error {
	return leaderLockReleaseScript.Run(ctx, c.rdb, []string{leaderLockKeyPrefix + key}, owner).Err()
}

// RefreshLeaderLock extends the lock TTL iff still owned by owner.
func (c *leaderLockCache) RefreshLeaderLock(ctx context.Context, key, owner string, ttl time.Duration) (bool, error) {
	if c == nil || c.rdb == nil || key == "" || owner == "" {
		return false, nil
	}
	if ttl <= 0 {
		ttl = time.Second
	}
	ms := ttl.Milliseconds()
	if ms < 1 {
		ms = 1
	}
	res, err := leaderLockRefreshScript.Run(ctx, c.rdb, []string{leaderLockKeyPrefix + key}, owner, ms).Int()
	if err != nil {
		return false, err
	}
	return res == 1, nil
}
