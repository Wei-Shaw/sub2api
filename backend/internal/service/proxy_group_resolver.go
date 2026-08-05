package service

import (
	"context"
	"log/slog"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// cachedGroupMembers 是组成员 + 组元数据的内存快照。
type cachedGroupMembers struct {
	group     ProxyGroup
	members   []Proxy
	fetchedAt time.Time
	gen       int64  // cross-instance generation at fill time (0 if versions nil)
	ExitIP    string // added for ExitIP honesty in group resolution
}

// DefaultProxyGroupResolver 实现 ProxyGroupResolver：
// 组成员列表走进程内缓存，选择走 SelectProxyFromGroup 纯函数。
type DefaultProxyGroupResolver struct {
	groupRepo ProxyGroupRepository
	proxyRepo ProxyRepository
	versions  ProxyGroupCacheVersionStore // optional; nil-safe
	now       func() time.Time
	ttl       time.Duration

	mu         sync.RWMutex
	cache      map[int64]*cachedGroupMembers
	rrCounters sync.Map // groupID -> *atomic.Uint64
	// bumpFailed tracks groups whose cross-instance generation bump failed.
	// While now < stored deadline, load() treats the group as a cache miss so
	// this instance keeps reloading from DB even though peers still hold the
	// old generation (and would otherwise re-serve a refilled local snap).
	bumpFailed sync.Map // groupID (int64) -> time.Time (force-miss until)
}

const (
	proxyGroupBumpAttempts     = 3
	proxyGroupBumpTimeout      = 2 * time.Second
	proxyGroupBumpFailForceTTL = 30 * time.Second
)

// NewDefaultProxyGroupResolver 构造默认解析器。
// ttl <= 0 时使用 30s 默认缓存。
func NewDefaultProxyGroupResolver(groupRepo ProxyGroupRepository, proxyRepo ProxyRepository) *DefaultProxyGroupResolver {
	return &DefaultProxyGroupResolver{
		groupRepo: groupRepo,
		proxyRepo: proxyRepo,
		now:       time.Now,
		ttl:       30 * time.Second,
		cache:     make(map[int64]*cachedGroupMembers),
	}
}

// NewDefaultProxyGroupResolverWithVersions 构造带跨实例 generation store 的解析器。
func NewDefaultProxyGroupResolverWithVersions(
	groupRepo ProxyGroupRepository,
	proxyRepo ProxyRepository,
	versions ProxyGroupCacheVersionStore,
) *DefaultProxyGroupResolver {
	r := NewDefaultProxyGroupResolver(groupRepo, proxyRepo)
	r.versions = versions
	return r
}

// SetVersionStore injects a cross-instance generation store (nil-safe).
func (r *DefaultProxyGroupResolver) SetVersionStore(store ProxyGroupCacheVersionStore) {
	if r == nil {
		return
	}
	r.versions = store
}

func (r *DefaultProxyGroupResolver) InvalidateGroup(groupID int64) {
	if r == nil {
		return
	}
	r.mu.Lock()
	delete(r.cache, groupID)
	r.mu.Unlock()
	r.rrCounters.Delete(groupID)

	if r.versions == nil || groupID <= 0 {
		return
	}

	var lastErr error
	for i := 0; i < proxyGroupBumpAttempts; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), proxyGroupBumpTimeout)
		_, err := r.versions.BumpGeneration(ctx, groupID)
		cancel()
		if err == nil {
			r.bumpFailed.Delete(groupID)
			return
		}
		lastErr = err
		// Short backoff between attempts: 10ms, 20ms, 30ms.
		time.Sleep(time.Duration(10*(i+1)) * time.Millisecond)
	}
	slog.Warn("proxy_group_cache_gen_bump_failed",
		"group_id", groupID,
		"attempts", proxyGroupBumpAttempts,
		"error", lastErr,
	)
	// Local cache already cleared; force subsequent load() to miss for a short
	// window so this instance does not re-serve a snap under the un-bumped gen.
	until := r.now()
	if until.IsZero() {
		until = time.Now()
	}
	r.bumpFailed.Store(groupID, until.Add(proxyGroupBumpFailForceTTL))
}

func (r *DefaultProxyGroupResolver) ResolveProxy(ctx context.Context, groupID, accountID int64) (*Proxy, error) {
	if r == nil || groupID <= 0 {
		return nil, nil
	}
	snap, err := r.load(ctx, groupID)
	if err != nil {
		return nil, err
	}
	// 账号已绑定 proxy_group_id 时：组缺失/未激活/无健康成员一律 fail-closed，
	// 禁止返回 (nil,nil) 让调用方静默降级为直连出口。
	if snap == nil || !snap.group.IsActive() {
		slog.Warn("proxy_group_unavailable",
			"group_id", groupID,
			"account_id", accountID,
			"missing", snap == nil,
		)
		return nil, ErrProxyGroupNoHealthyMember
	}

	strategy := snap.group.EffectiveStrategy()
	seed := r.seedFor(groupID, accountID, strategy)
	selected, ok := SelectProxyFromGroup(snap.members, strategy, r.now(), seed)
	if !ok {
		slog.Warn("proxy_group_no_healthy_member",
			"group_id", groupID,
			"account_id", accountID,
			"member_count", len(snap.members),
			"strategy", strategy,
		)
		return nil, ErrProxyGroupNoHealthyMember
	}
	return selected, nil
}

func (r *DefaultProxyGroupResolver) seedFor(groupID, accountID int64, strategy string) uint64 {
	switch strategy {
	case ProxyGroupStrategySticky:
		if accountID < 0 {
			return 0
		}
		return uint64(accountID)
	case ProxyGroupStrategyRandom:
		return rand.Uint64()
	default: // round_robin
		counter := &atomic.Uint64{}
		if existing, loaded := r.rrCounters.LoadOrStore(groupID, counter); loaded {
			if c, ok := existing.(*atomic.Uint64); ok && c != nil {
				counter = c
			}
		}
		return counter.Add(1) - 1
	}
}

// getGeneration returns (gen, ok). ok=false on store error → callers must treat
// as cache miss (fail-closed). Do NOT use gen=0 on error for equality with hit.gen.
// When versions is nil, returns (0, true) so nil-store paths keep old behavior.
func (r *DefaultProxyGroupResolver) getGeneration(ctx context.Context, groupID int64) (int64, bool) {
	if r == nil || r.versions == nil || groupID <= 0 {
		return 0, true
	}
	gen, err := r.versions.GetGeneration(ctx, groupID)
	if err != nil {
		slog.Warn("proxy_group_cache_gen_get_failed",
			"group_id", groupID,
			"error", err,
		)
		return 0, false
	}
	return gen, true
}

// GetGeneration is public for admin stats honesty (cross-instance gen).
func (r *DefaultProxyGroupResolver) GetGeneration(ctx context.Context, groupID int64) (int64, bool) {
	return r.getGeneration(ctx, groupID)
}

func (r *DefaultProxyGroupResolver) forceMissActive(groupID int64, now time.Time) bool {
	if r == nil || groupID <= 0 {
		return false
	}
	v, ok := r.bumpFailed.Load(groupID)
	if !ok {
		return false
	}
	until, ok := v.(time.Time)
	if !ok {
		r.bumpFailed.Delete(groupID)
		return false
	}
	if now.Before(until) {
		return true
	}
	r.bumpFailed.Delete(groupID)
	return false
}

func (r *DefaultProxyGroupResolver) load(ctx context.Context, groupID int64) (*cachedGroupMembers, error) {
	now := r.now()
	forceMiss := r.forceMissActive(groupID, now)
	r.mu.RLock()
	if !forceMiss {
		if hit, ok := r.cache[groupID]; ok && hit != nil && now.Sub(hit.fetchedAt) < r.ttl {
			// Cross-instance generation check: remote InvalidateGroup bumps gen.
			// GetGeneration error is fail-closed (miss), never equal-check gen=0 on error.
			if r.versions != nil {
				gen, genOK := r.getGeneration(ctx, groupID)
				if genOK && hit.gen == gen {
					r.mu.RUnlock()
					return hit, nil
				}
				// gen mismatch or store error → treat as miss
			} else {
				r.mu.RUnlock()
				return hit, nil
			}
		}
	}
	r.mu.RUnlock()

	if r.groupRepo == nil || r.proxyRepo == nil {
		return nil, nil
	}

	// Capture gen before DB read so we can detect concurrent bumps.
	loadGen, loadOK := r.getGeneration(ctx, groupID)

	group, err := r.groupRepo.GetByID(ctx, groupID)
	if err != nil {
		if err == ErrProxyGroupNotFound {
			// Local-only drop; avoid BumpGeneration (would thrash gen on every miss).
			r.mu.Lock()
			delete(r.cache, groupID)
			r.mu.Unlock()
			r.rrCounters.Delete(groupID)
			return nil, nil
		}
		return nil, err
	}
	members, err := r.proxyRepo.ListByGroupID(ctx, groupID)
	if err != nil {
		return nil, err
	}

	// Re-check generation after DB load: if bumped during read, prefer not caching
	// a potentially stale fill (still return the snap for this request).
	// Store errors also skip caching (fail-closed).
	postGen, postOK := r.getGeneration(ctx, groupID)
	snap := &cachedGroupMembers{
		group:     *group,
		members:   members,
		fetchedAt: now,
		gen:       postGen,
		ExitIP:    "", // ExitIP honesty from prober; group resolver returns proxy without it
	}
	if !loadOK || !postOK || postGen != loadGen {
		// Concurrent invalidation or gen-store error — return without caching.
		return snap, nil
	}

	r.mu.Lock()
	// Final guard under write lock: skip cache write if gen moved again or errored.
	if r.versions != nil {
		again, againOK := r.getGeneration(ctx, groupID)
		if !againOK || again != snap.gen {
			r.mu.Unlock()
			return snap, nil
		}
	}
	r.cache[groupID] = snap
	r.mu.Unlock()
	return snap, nil
}
