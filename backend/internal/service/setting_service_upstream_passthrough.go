package service

import (
	"context"
	"encoding/json"
	"fmt"
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

// SetUpstreamPassthroughDefaults validates, persists, and invalidates cache
// for the per-category default policy. Invalid profiles are rejected (no write).
// Unknown toggle keys in overrides are silently dropped during normalization.
func (s *SettingService) SetUpstreamPassthroughDefaults(ctx context.Context, input UpstreamPassthroughDefaults) error {
	normalized, err := normalizeUpstreamPassthroughDefaults(input)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return fmt.Errorf("marshal upstream_passthrough_defaults: %w", err)
	}
	if err := s.settingRepo.Set(ctx, SettingKeyUpstreamPassthroughDefaults, string(raw)); err != nil {
		return fmt.Errorf("persist upstream_passthrough_defaults: %w", err)
	}
	invalidateUpstreamPassthroughDefaultsCache()
	if s.onUpdate != nil {
		s.onUpdate()
	}
	return nil
}

func normalizeUpstreamPassthroughDefaults(in UpstreamPassthroughDefaults) (UpstreamPassthroughDefaults, error) {
	out := UpstreamPassthroughDefaults{}
	var err error
	out.Relay, err = normalizeCategoryDefault(in.Relay, CategoryRelay)
	if err != nil {
		return out, err
	}
	out.Official, err = normalizeCategoryDefault(in.Official, CategoryOfficial)
	if err != nil {
		return out, err
	}
	out.Reverse, err = normalizeCategoryDefault(in.Reverse, CategoryReverse)
	if err != nil {
		return out, err
	}
	return out, nil
}

func normalizeCategoryDefault(in UpstreamPassthroughCategoryDefault, cat UpstreamCategory) (UpstreamPassthroughCategoryDefault, error) {
	out := UpstreamPassthroughCategoryDefault{
		Profile:   in.Profile,
		Overrides: map[string]bool{},
	}
	if out.Profile == "" {
		out.Profile = CategoryDefaultProfile(cat)
	}
	switch out.Profile {
	case ProfileTransparent, ProfileProtected, ProfileStrict:
		// ok
	default:
		return out, fmt.Errorf("invalid profile %q for category %q (must be transparent/protected/strict)", out.Profile, cat)
	}
	knownToggles := []string{
		ToggleForwardClientHeaders,
		ToggleForwardUserNetworkInfo,
		ToggleSkipBodyScrub,
		ToggleSkipSystemPromptInject,
		ToggleForwardClientUA,
		ToggleForwardBetaFlags,
		ToggleSkipModelRewrite,
	}
	for _, k := range knownToggles {
		if v, ok := in.Overrides[k]; ok {
			out.Overrides[k] = v
		}
	}
	// drop empty map to reduce serialized payload (omitempty omits nil)
	if len(out.Overrides) == 0 {
		out.Overrides = nil
	}
	return out, nil
}
