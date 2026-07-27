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
}

// DefaultProxyGroupResolver 实现 ProxyGroupResolver：
// 组成员列表走进程内缓存，选择走 SelectProxyFromGroup 纯函数。
type DefaultProxyGroupResolver struct {
	groupRepo ProxyGroupRepository
	proxyRepo ProxyRepository
	now       func() time.Time
	ttl       time.Duration

	mu         sync.RWMutex
	cache      map[int64]*cachedGroupMembers
	rrCounters sync.Map // groupID -> *atomic.Uint64
}

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

func (r *DefaultProxyGroupResolver) InvalidateGroup(groupID int64) {
	if r == nil {
		return
	}
	r.mu.Lock()
	delete(r.cache, groupID)
	r.mu.Unlock()
	r.rrCounters.Delete(groupID)
}

func (r *DefaultProxyGroupResolver) ResolveProxy(ctx context.Context, groupID, accountID int64) (*Proxy, error) {
	if r == nil || groupID <= 0 {
		return nil, nil
	}
	snap, err := r.load(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if snap == nil || !snap.group.IsActive() {
		return nil, nil
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
		return nil, nil
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

func (r *DefaultProxyGroupResolver) load(ctx context.Context, groupID int64) (*cachedGroupMembers, error) {
	now := r.now()
	r.mu.RLock()
	if hit, ok := r.cache[groupID]; ok && hit != nil && now.Sub(hit.fetchedAt) < r.ttl {
		r.mu.RUnlock()
		return hit, nil
	}
	r.mu.RUnlock()

	if r.groupRepo == nil || r.proxyRepo == nil {
		return nil, nil
	}
	group, err := r.groupRepo.GetByID(ctx, groupID)
	if err != nil {
		if err == ErrProxyGroupNotFound {
			r.InvalidateGroup(groupID)
			return nil, nil
		}
		return nil, err
	}
	members, err := r.proxyRepo.ListByGroupID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	snap := &cachedGroupMembers{
		group:     *group,
		members:   members,
		fetchedAt: now,
	}

	r.mu.Lock()
	r.cache[groupID] = snap
	r.mu.Unlock()
	return snap, nil
}
