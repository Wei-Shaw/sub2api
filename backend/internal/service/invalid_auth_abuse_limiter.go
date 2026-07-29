package service

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const invalidAuthAbuseShardCount = 16

type invalidAuthAbuseEntry struct {
	failures     int
	windowStart  time.Time
	blockedUntil time.Time
}

type invalidAuthAbuseShard struct {
	mu      sync.Mutex
	entries map[string]*invalidAuthAbuseEntry
}

type invalidAuthOverflow struct {
	mu           sync.Mutex
	failures     int
	windowStart  time.Time
	blockedUntil time.Time
}

type invalidAuthAbuseLimiter struct {
	threshold int
	window    time.Duration
	block     time.Duration
	capacity  int64
	shards    [invalidAuthAbuseShardCount]invalidAuthAbuseShard
	overflow  invalidAuthOverflow
	now       func() time.Time

	tracked       atomic.Int64
	recorded      atomic.Uint64
	blocked       atomic.Uint64
	rejected      atomic.Uint64
	expired       atomic.Uint64
	overflowed    atomic.Uint64
	globalBlocked atomic.Uint64
	cleanupNext   atomic.Int64
	cleanupCursor atomic.Uint32
}

type InvalidAuthAbuseHealth struct {
	Enabled       bool   `json:"enabled"`
	Tracked       int64  `json:"tracked"`
	Capacity      int64  `json:"capacity"`
	Recorded      uint64 `json:"recorded"`
	Blocks        uint64 `json:"blocks"`
	Rejected      uint64 `json:"rejected"`
	Expired       uint64 `json:"expired"`
	Overflowed    uint64 `json:"overflowed"`
	GlobalBlocked uint64 `json:"global_blocked"`
}

func newInvalidAuthAbuseLimiter(cfg *config.Config) *invalidAuthAbuseLimiter {
	if cfg == nil || !cfg.APIKeyAuth.InvalidAbuse.Enabled {
		return nil
	}
	c := cfg.APIKeyAuth.InvalidAbuse
	if c.Threshold <= 0 || c.WindowSeconds <= 0 || c.BlockSeconds <= 0 || c.Capacity <= 0 {
		return nil
	}
	l := &invalidAuthAbuseLimiter{
		threshold: c.Threshold,
		window:    time.Duration(c.WindowSeconds) * time.Second,
		block:     time.Duration(c.BlockSeconds) * time.Second,
		capacity:  int64(c.Capacity),
		now:       time.Now,
	}
	for i := range l.shards {
		l.shards[i].entries = make(map[string]*invalidAuthAbuseEntry)
	}
	return l
}

func (s *APIKeyService) CheckInvalidAuthAbuse(clientKey string) (time.Duration, bool) {
	if s == nil || s.invalidAuthAbuse == nil {
		return 0, false
	}
	return s.invalidAuthAbuse.check(clientKey)
}

func (s *APIKeyService) RecordInvalidAuthFailure(clientKey string) {
	if s == nil || s.invalidAuthAbuse == nil {
		return
	}
	s.invalidAuthAbuse.record(clientKey)
}

func (s *APIKeyService) InvalidAuthAbuseHealth() InvalidAuthAbuseHealth {
	if s == nil || s.invalidAuthAbuse == nil {
		return InvalidAuthAbuseHealth{}
	}
	return s.invalidAuthAbuse.health()
}

func (l *invalidAuthAbuseLimiter) check(clientKey string) (time.Duration, bool) {
	if l == nil || clientKey == "" {
		return 0, false
	}
	now := l.now()
	l.maybeCleanupAtCapacity(now)
	shard := l.shard(clientKey)
	shard.mu.Lock()
	entry := shard.entries[clientKey]
	if entry != nil && l.entryExpired(entry, now) {
		delete(shard.entries, clientKey)
		l.tracked.Add(-1)
		l.expired.Add(1)
		entry = nil
	}
	if entry != nil && entry.blockedUntil.After(now) {
		retry := entry.blockedUntil.Sub(now)
		shard.mu.Unlock()
		l.rejected.Add(1)
		return retry, true
	}
	shard.mu.Unlock()

	if entry == nil && l.tracked.Load() >= l.capacity {
		if retry, blocked := l.checkOverflow(now); blocked {
			l.rejected.Add(1)
			l.globalBlocked.Add(1)
			return retry, true
		}
	}
	return 0, false
}

func (l *invalidAuthAbuseLimiter) record(clientKey string) {
	if l == nil || clientKey == "" {
		return
	}
	l.recorded.Add(1)
	now := l.now()
	l.maybeCleanupAtCapacity(now)
	shard := l.shard(clientKey)
	shard.mu.Lock()
	entry := shard.entries[clientKey]
	if entry != nil && l.entryExpired(entry, now) {
		delete(shard.entries, clientKey)
		l.tracked.Add(-1)
		l.expired.Add(1)
		entry = nil
	}
	if entry == nil {
		if !l.reserveEntry() {
			shard.mu.Unlock()
			l.recordOverflow(now)
			return
		}
		entry = &invalidAuthAbuseEntry{windowStart: now}
		shard.entries[clientKey] = entry
	}
	if entry.blockedUntil.After(now) {
		shard.mu.Unlock()
		return
	}
	if entry.windowStart.After(now) || !now.Before(entry.windowStart.Add(l.window)) {
		entry.windowStart = now
		entry.failures = 0
	}
	entry.failures++
	if entry.failures >= l.threshold {
		entry.failures = 0
		entry.blockedUntil = now.Add(l.block)
		entry.windowStart = entry.blockedUntil
		l.blocked.Add(1)
	}
	shard.mu.Unlock()
}

func (l *invalidAuthAbuseLimiter) reserveEntry() bool {
	for {
		current := l.tracked.Load()
		if current >= l.capacity {
			return false
		}
		if l.tracked.CompareAndSwap(current, current+1) {
			return true
		}
	}
}

func (l *invalidAuthAbuseLimiter) maybeCleanupAtCapacity(now time.Time) {
	if l.tracked.Load() < l.capacity {
		return
	}
	nowUnixNano := now.UnixNano()
	for {
		next := l.cleanupNext.Load()
		if nowUnixNano < next {
			return
		}
		if l.cleanupNext.CompareAndSwap(next, now.Add(100*time.Millisecond).UnixNano()) {
			break
		}
	}
	index := l.cleanupCursor.Add(1) - 1
	shard := &l.shards[index%invalidAuthAbuseShardCount]
	shard.mu.Lock()
	for key, entry := range shard.entries {
		if l.entryExpired(entry, now) {
			delete(shard.entries, key)
			l.tracked.Add(-1)
			l.expired.Add(1)
		}
	}
	shard.mu.Unlock()
}

func (l *invalidAuthAbuseLimiter) entryExpired(entry *invalidAuthAbuseEntry, now time.Time) bool {
	return entry != nil && !entry.blockedUntil.After(now) && !entry.windowStart.After(now) && !now.Before(entry.windowStart.Add(l.window))
}

func (l *invalidAuthAbuseLimiter) shard(clientKey string) *invalidAuthAbuseShard {
	const fnvOffset32 = uint32(2166136261)
	const fnvPrime32 = uint32(16777619)
	hash := fnvOffset32
	for i := 0; i < len(clientKey); i++ {
		hash ^= uint32(clientKey[i])
		hash *= fnvPrime32
	}
	return &l.shards[hash%invalidAuthAbuseShardCount]
}

func (l *invalidAuthAbuseLimiter) recordOverflow(now time.Time) {
	l.overflowed.Add(1)
	l.overflow.mu.Lock()
	defer l.overflow.mu.Unlock()
	if l.overflow.blockedUntil.After(now) {
		return
	}
	if l.overflow.windowStart.IsZero() || !now.Before(l.overflow.windowStart.Add(l.window)) {
		l.overflow.windowStart = now
		l.overflow.failures = 0
	}
	l.overflow.failures++
	if l.overflow.failures >= l.threshold {
		l.overflow.failures = 0
		l.overflow.blockedUntil = now.Add(l.block)
		l.overflow.windowStart = l.overflow.blockedUntil
		l.blocked.Add(1)
	}
}

func (l *invalidAuthAbuseLimiter) checkOverflow(now time.Time) (time.Duration, bool) {
	l.overflow.mu.Lock()
	defer l.overflow.mu.Unlock()
	if l.overflow.blockedUntil.After(now) {
		return l.overflow.blockedUntil.Sub(now), true
	}
	return 0, false
}

func (l *invalidAuthAbuseLimiter) health() InvalidAuthAbuseHealth {
	return InvalidAuthAbuseHealth{
		Enabled:       true,
		Tracked:       l.tracked.Load(),
		Capacity:      l.capacity,
		Recorded:      l.recorded.Load(),
		Blocks:        l.blocked.Load(),
		Rejected:      l.rejected.Load(),
		Expired:       l.expired.Load(),
		Overflowed:    l.overflowed.Load(),
		GlobalBlocked: l.globalBlocked.Load(),
	}
}
