package service

import (
	"hash/fnv"
	"log/slog"
	"sort"
	"time"
)

// SelectProxyFromGroup 从候选代理中按策略选出一个健康代理。
//
// 纯函数、无 I/O，便于穷举单测。调用方负责：
//   - 提供候选快照 candidates
//   - 为 round_robin 提供递增 seed（通常是原子计数器）
//   - 为 sticky 提供 accountID 作为 seed
//
// 返回 (proxy, true) 表示命中；(nil, false) 表示无可用候选。
func SelectProxyFromGroup(candidates []Proxy, strategy string, now time.Time, seed uint64) (*Proxy, bool) {
	healthy := filterHealthyProxies(candidates, now)
	if len(healthy) == 0 {
		return nil, false
	}
	if len(healthy) == 1 {
		p := healthy[0]
		return &p, true
	}

	switch strategy {
	case ProxyGroupStrategyRandom:
		idx := seed % uint64(len(healthy))
		p := healthy[idx]
		return &p, true
	case ProxyGroupStrategySticky:
		// seed 语义为 accountID。按 (accountID, proxyID) 最小哈希选人：
		// 候选集合顺序变化不影响结果；成员被摘除时，未命中该成员的账号保持原出口。
		p := stickySelectByProxyID(seed, healthy)
		return &p, true
	case ProxyGroupStrategyRoundRobin:
		// Sort by ID so RR is deterministic regardless of slice order from DB/cache.
		sorted := sortProxiesByID(healthy)
		idx := seed % uint64(len(sorted))
		p := sorted[idx]
		return &p, true
	default:
		slog.Warn("proxy_group_unknown_strategy_fallback_round_robin",
			"strategy", strategy,
			"fallback", ProxyGroupStrategyRoundRobin,
		)
		sorted := sortProxiesByID(healthy)
		idx := seed % uint64(len(sorted))
		p := sorted[idx]
		return &p, true
	}
}

func filterHealthyProxies(candidates []Proxy, now time.Time) []Proxy {
	if len(candidates) == 0 {
		return nil
	}
	out := make([]Proxy, 0, len(candidates))
	for i := range candidates {
		p := &candidates[i]
		if p.IsActive() && !p.IsExpired(now) {
			out = append(out, *p)
		}
	}
	return out
}

func sortProxiesByID(in []Proxy) []Proxy {
	out := append([]Proxy(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// stickySelectByProxyID picks the healthy proxy with the lowest FNV hash of
// (accountID || proxyID). Removing a different proxy does not change the score
// of the preferred one, so sticky exit IPs stay put across isolate/recover.
func stickySelectByProxyID(accountID uint64, healthy []Proxy) Proxy {
	best := healthy[0]
	bestScore := stickyScore(accountID, best.ID)
	for i := 1; i < len(healthy); i++ {
		p := healthy[i]
		score := stickyScore(accountID, p.ID)
		if score < bestScore || (score == bestScore && p.ID < best.ID) {
			best = p
			bestScore = score
		}
	}
	return best
}

func stickyScore(accountID uint64, proxyID int64) uint64 {
	h := fnv.New64a()
	var buf [16]byte
	for i := 0; i < 8; i++ {
		buf[i] = byte(accountID >> (8 * i))
	}
	u := uint64(proxyID)
	for i := 0; i < 8; i++ {
		buf[8+i] = byte(u >> (8 * i))
	}
	_, _ = h.Write(buf[:])
	return h.Sum64()
}
