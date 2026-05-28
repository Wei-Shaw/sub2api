package dnscache

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type cacheEntry struct {
	addrs     []string
	expiresAt int64
	mu        sync.RWMutex
}

// Resolver is a lightweight in-process DNS cache that resolves hostnames
// and caches the results for a configurable TTL. It reduces DNS lookup
// latency for repeated connections to the same hosts (e.g. api.anthropic.com).
type Resolver struct {
	cache    sync.Map // host → *cacheEntry
	ttl      time.Duration
	resolver *net.Resolver
	counter  uint64 // for round-robin
}

// New creates a DNS cache resolver with the given TTL.
func New(ttl time.Duration) *Resolver {
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	return &Resolver{
		ttl:      ttl,
		resolver: net.DefaultResolver,
	}
}

// LookupHost resolves a hostname, returning cached results if available.
func (r *Resolver) LookupHost(ctx context.Context, host string) ([]string, error) {
	now := time.Now().UnixNano()

	if val, ok := r.cache.Load(host); ok {
		entry := val.(*cacheEntry)
		entry.mu.RLock()
		if entry.expiresAt > now && len(entry.addrs) > 0 {
			addrs := entry.addrs
			entry.mu.RUnlock()
			return addrs, nil
		}
		entry.mu.RUnlock()
	}

	addrs, err := r.resolver.LookupHost(ctx, host)
	if err != nil {
		// On lookup failure, return stale cache if available
		if val, ok := r.cache.Load(host); ok {
			entry := val.(*cacheEntry)
			entry.mu.RLock()
			if len(entry.addrs) > 0 {
				stale := entry.addrs
				entry.mu.RUnlock()
				return stale, nil
			}
			entry.mu.RUnlock()
		}
		return nil, err
	}

	entry := &cacheEntry{
		addrs:     addrs,
		expiresAt: time.Now().Add(r.ttl).UnixNano(),
	}
	r.cache.Store(host, entry)
	return addrs, nil
}

// DialContext is a drop-in replacement for net.Dialer.DialContext that uses
// cached DNS results. It can be assigned to http.Transport.DialContext.
func (r *Resolver) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		// If SplitHostPort fails, fall through to default dialer
		return (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext(ctx, network, address)
	}

	// If host is already an IP, dial directly
	if net.ParseIP(host) != nil {
		return (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext(ctx, network, address)
	}

	addrs, err := r.LookupHost(ctx, host)
	if err != nil {
		return nil, err
	}

	// Round-robin selection for load distribution
	idx := atomic.AddUint64(&r.counter, 1)
	addr := addrs[int(idx)%len(addrs)]

	return (&net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext(ctx, network, net.JoinHostPort(addr, port))
}

// Refresh proactively resolves a host and updates the cache,
// useful for connection pre-warming scenarios.
func (r *Resolver) Refresh(ctx context.Context, host string) error {
	addrs, err := r.resolver.LookupHost(ctx, host)
	if err != nil {
		return err
	}
	entry := &cacheEntry{
		addrs:     addrs,
		expiresAt: time.Now().Add(r.ttl).UnixNano(),
	}
	r.cache.Store(host, entry)
	return nil
}

// SetTTL updates the cache TTL for new entries.
func (r *Resolver) SetTTL(ttl time.Duration) {
	if ttl > 0 {
		r.ttl = ttl
	}
}
