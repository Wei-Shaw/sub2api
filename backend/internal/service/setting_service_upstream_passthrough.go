package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

// cachedUpstreamPassthroughDefaults is the hot-path cache for the JSON-parsed
// system default. Follows the same atomic.Value + 60s TTL + singleflight pattern
// as cachedGatewayForwardingSettings.
type cachedUpstreamPassthroughDefaults struct {
	value     UpstreamPassthroughDefaults
	expiresAt int64 // unix nano
}

var (
	upstreamPassthroughDefaultsCache atomic.Value // *cachedUpstreamPassthroughDefaults
	upstreamPassthroughDefaultsSF    singleflight.Group
)

const (
	upstreamPassthroughDefaultsCacheTTL  = 60 * time.Second
	upstreamPassthroughDefaultsErrorTTL  = 5 * time.Second
	upstreamPassthroughDefaultsDBTimeout = 5 * time.Second
)

// GetUpstreamPassthroughDefaults returns the admin-configured per-category
// defaults, with code constants used as fallback. Hot path; cached for 60s.
func (s *SettingService) GetUpstreamPassthroughDefaults(ctx context.Context) UpstreamPassthroughDefaults {
	if cached, ok := upstreamPassthroughDefaultsCache.Load().(*cachedUpstreamPassthroughDefaults); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return cached.value
		}
	}
	val, _, _ := upstreamPassthroughDefaultsSF.Do("upstream_passthrough_defaults", func() (any, error) {
		if cached, ok := upstreamPassthroughDefaultsCache.Load().(*cachedUpstreamPassthroughDefaults); ok && cached != nil {
			if time.Now().UnixNano() < cached.expiresAt {
				return cached.value, nil
			}
		}
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), upstreamPassthroughDefaultsDBTimeout)
		defer cancel()
		raw, err := s.settingRepo.GetValue(dbCtx, SettingKeyUpstreamPassthroughDefaults)
		if err != nil {
			// fail-open: use code defaults
			d := DefaultUpstreamPassthroughDefaults()
			upstreamPassthroughDefaultsCache.Store(&cachedUpstreamPassthroughDefaults{
				value:     d,
				expiresAt: time.Now().Add(upstreamPassthroughDefaultsErrorTTL).UnixNano(),
			})
			return d, nil
		}
		if raw == "" {
			d := DefaultUpstreamPassthroughDefaults()
			upstreamPassthroughDefaultsCache.Store(&cachedUpstreamPassthroughDefaults{
				value:     d,
				expiresAt: time.Now().Add(upstreamPassthroughDefaultsCacheTTL).UnixNano(),
			})
			return d, nil
		}
		var parsed UpstreamPassthroughDefaults
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			slog.Warn("upstream_passthrough_defaults: invalid JSON, using code defaults",
				"error", err, "raw_len", len(raw))
			d := DefaultUpstreamPassthroughDefaults()
			upstreamPassthroughDefaultsCache.Store(&cachedUpstreamPassthroughDefaults{
				value:     d,
				expiresAt: time.Now().Add(upstreamPassthroughDefaultsErrorTTL).UnixNano(),
			})
			return d, nil
		}
		upstreamPassthroughDefaultsCache.Store(&cachedUpstreamPassthroughDefaults{
			value:     parsed,
			expiresAt: time.Now().Add(upstreamPassthroughDefaultsCacheTTL).UnixNano(),
		})
		return parsed, nil
	})
	if v, ok := val.(UpstreamPassthroughDefaults); ok {
		return v
	}
	return DefaultUpstreamPassthroughDefaults()
}

// invalidateUpstreamPassthroughDefaultsCache forcibly drops the local cache
// (used by Set methods after a successful write).
func invalidateUpstreamPassthroughDefaultsCache() {
	upstreamPassthroughDefaultsCache.Store((*cachedUpstreamPassthroughDefaults)(nil))
}
