package service

import (
	"hash/fnv"
	"log/slog"
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
		// seed 语义为 accountID：哈希后取模，候选集不变时同账号恒定。
		idx := stickyIndex(seed, len(healthy))
		p := healthy[idx]
		return &p, true
	case ProxyGroupStrategyRoundRobin:
		idx := seed % uint64(len(healthy))
		p := healthy[idx]
		return &p, true
	default:
		slog.Warn("proxy_group_unknown_strategy_fallback_round_robin",
			"strategy", strategy,
			"fallback", ProxyGroupStrategyRoundRobin,
		)
		idx := seed % uint64(len(healthy))
		p := healthy[idx]
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

func stickyIndex(accountID uint64, n int) int {
	if n <= 0 {
		return 0
	}
	h := fnv.New64a()
	var buf [8]byte
	for i := 0; i < 8; i++ {
		buf[i] = byte(accountID >> (8 * i))
	}
	_, _ = h.Write(buf[:])
	return int(h.Sum64() % uint64(n))
}
