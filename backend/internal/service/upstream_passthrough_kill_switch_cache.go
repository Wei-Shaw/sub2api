package service

import (
	"context"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

// cachedUpstreamPassthroughGlobalOverride holds the parsed kill switch mode
// with a 10s TTL. Short TTL because this is an emergency lever and admins
// expect changes to propagate quickly; 10s is short enough to feel "instant"
// but long enough to absorb the per-request read amplification when the
// FeatureFlag is on.
type cachedUpstreamPassthroughGlobalOverride struct {
	value     GlobalOverrideMode
	expiresAt int64 // unix nano
}

var (
	upstreamPassthroughGlobalOverrideCache atomic.Value // *cachedUpstreamPassthroughGlobalOverride
	upstreamPassthroughGlobalOverrideSF    singleflight.Group
)

const (
	upstreamPassthroughGlobalOverrideCacheTTL  = 10 * time.Second
	upstreamPassthroughGlobalOverrideErrorTTL  = 5 * time.Second
	upstreamPassthroughGlobalOverrideDBTimeout = 2 * time.Second
)

// loadCachedUpstreamPassthroughGlobalOverride attempts to read from the local
// cache, falling back to the settingRepo via singleflight on miss/expiry.
// Always returns a valid GlobalOverrideMode (auto on any error).
func (s *SettingService) loadCachedUpstreamPassthroughGlobalOverride(ctx context.Context) GlobalOverrideMode {
	if cached, ok := upstreamPassthroughGlobalOverrideCache.Load().(*cachedUpstreamPassthroughGlobalOverride); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return cached.value
		}
	}
	val, _, _ := upstreamPassthroughGlobalOverrideSF.Do("upstream_passthrough_global_override", func() (any, error) {
		// Inner re-check in case another goroutine populated cache while we waited.
		if cached, ok := upstreamPassthroughGlobalOverrideCache.Load().(*cachedUpstreamPassthroughGlobalOverride); ok && cached != nil {
			if time.Now().UnixNano() < cached.expiresAt {
				return cached.value, nil
			}
		}
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), upstreamPassthroughGlobalOverrideDBTimeout)
		defer cancel()
		raw, err := s.settingRepo.GetValue(dbCtx, SettingKeyUpstreamPassthroughGlobalOverride)
		mode := GlobalOverrideAuto
		ttl := upstreamPassthroughGlobalOverrideCacheTTL
		if err != nil {
			// fail-open with short error TTL
			ttl = upstreamPassthroughGlobalOverrideErrorTTL
		} else if raw != "" {
			mode = ParseGlobalOverrideMode(raw)
		}
		upstreamPassthroughGlobalOverrideCache.Store(&cachedUpstreamPassthroughGlobalOverride{
			value:     mode,
			expiresAt: time.Now().Add(ttl).UnixNano(),
		})
		return mode, nil
	})
	if v, ok := val.(GlobalOverrideMode); ok {
		return v
	}
	return GlobalOverrideAuto
}

// invalidateUpstreamPassthroughGlobalOverrideCache drops the local cache so
// the next read fetches fresh. Called from Set methods.
func invalidateUpstreamPassthroughGlobalOverrideCache() {
	upstreamPassthroughGlobalOverrideCache.Store((*cachedUpstreamPassthroughGlobalOverride)(nil))
}
