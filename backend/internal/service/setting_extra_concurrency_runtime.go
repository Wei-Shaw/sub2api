package service

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"
)

var extraConcurrencyRuntimeSettingKeys = []string{
	SettingKeyExtraConcurrencyEnabled,
	SettingKeyExtraConcurrencyWaitTimeoutSeconds,
	SettingKeyExtraConcurrencyReservePercent,
	SettingKeyExtraConcurrencyMinReservedSlots,
	SettingKeyExtraConcurrencyPlatformReserves,
}

const (
	extraConcurrencyRuntimeCacheKey = "extra_concurrency_runtime"
	extraConcurrencyRuntimeCacheTTL = 10 * time.Second
)

var errExtraConcurrencyRuntimeInvalidated = errors.New("extra concurrency runtime settings invalidated")

type ExtraConcurrencyRuntimeSettings struct {
	Enabled            bool
	WaitTimeoutSeconds int
	ReservePercent     float64
	MinReservedSlots   int
	PlatformReserves   map[string]ExtraConcurrencyPlatformReserve
}

type cachedExtraConcurrencyRuntimeSettings struct {
	settings   ExtraConcurrencyRuntimeSettings
	expiresAt  int64
	generation uint64
}

func (s *SettingService) GetExtraConcurrencyRuntimeSettings(ctx context.Context) ExtraConcurrencyRuntimeSettings {
	fallback := disabledExtraConcurrencyRuntimeSettings()
	if s == nil || s.settingRepo == nil {
		return fallback
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		if cached := s.loadExtraConcurrencyRuntimeSettings(); cached != nil {
			return cloneExtraConcurrencyRuntimeSettings(cached.settings)
		}
		result, err, _ := s.extraConcurrencyRuntimeSF.Do(extraConcurrencyRuntimeCacheKey, func() (any, error) {
			if cached := s.loadExtraConcurrencyRuntimeSettings(); cached != nil {
				return cached, nil
			}
			generation := s.extraConcurrencyRuntimeGen.Load()
			values, err := s.settingRepo.GetMultiple(ctx, extraConcurrencyRuntimeSettingKeys)
			snapshot := fallback
			if err == nil {
				snapshot = parseExtraConcurrencyRuntimeSettings(values)
			}
			if s.extraConcurrencyRuntimeGen.Load() != generation {
				return nil, errExtraConcurrencyRuntimeInvalidated
			}
			cached := s.storeExtraConcurrencyRuntimeSettings(snapshot, generation)
			if s.extraConcurrencyRuntimeGen.Load() != generation {
				return nil, errExtraConcurrencyRuntimeInvalidated
			}
			return cached, nil
		})
		if errors.Is(err, errExtraConcurrencyRuntimeInvalidated) {
			continue
		}
		cached, ok := result.(*cachedExtraConcurrencyRuntimeSettings)
		if !ok || cached == nil {
			return fallback
		}
		return cloneExtraConcurrencyRuntimeSettings(cached.settings)
	}
}

func (s *SettingService) loadExtraConcurrencyRuntimeSettings() *cachedExtraConcurrencyRuntimeSettings {
	cached, _ := s.extraConcurrencyRuntimeCache.Load().(*cachedExtraConcurrencyRuntimeSettings)
	if cached == nil || cached.generation != s.extraConcurrencyRuntimeGen.Load() || cached.expiresAt <= s.extraConcurrencyRuntimeTime().UnixNano() {
		return nil
	}
	return cached
}

func (s *SettingService) storeExtraConcurrencyRuntimeSettings(settings ExtraConcurrencyRuntimeSettings, generation uint64) *cachedExtraConcurrencyRuntimeSettings {
	cached := &cachedExtraConcurrencyRuntimeSettings{
		settings:   cloneExtraConcurrencyRuntimeSettings(settings),
		expiresAt:  s.extraConcurrencyRuntimeTime().Add(extraConcurrencyRuntimeCacheTTL).UnixNano(),
		generation: generation,
	}
	s.extraConcurrencyRuntimeCache.Store(cached)
	return cached
}

func (s *SettingService) InvalidateExtraConcurrencyRuntimeSettings() {
	if s == nil {
		return
	}
	generation := s.extraConcurrencyRuntimeGen.Add(1)
	s.extraConcurrencyRuntimeSF.Forget(extraConcurrencyRuntimeCacheKey)
	settings := disabledExtraConcurrencyRuntimeSettings()
	if cached, _ := s.extraConcurrencyRuntimeCache.Load().(*cachedExtraConcurrencyRuntimeSettings); cached != nil {
		settings = cached.settings
	}
	s.extraConcurrencyRuntimeCache.Store(&cachedExtraConcurrencyRuntimeSettings{
		settings:   cloneExtraConcurrencyRuntimeSettings(settings),
		expiresAt:  0,
		generation: generation,
	})
}

func (s *SettingService) extraConcurrencyRuntimeTime() time.Time {
	if s != nil && s.extraConcurrencyRuntimeNow != nil {
		return s.extraConcurrencyRuntimeNow()
	}
	return time.Now()
}

func parseExtraConcurrencyRuntimeSettings(values map[string]string) ExtraConcurrencyRuntimeSettings {
	fallback := disabledExtraConcurrencyRuntimeSettings()
	for _, key := range extraConcurrencyRuntimeSettingKeys {
		if _, ok := values[key]; !ok {
			return fallback
		}
	}

	enabled, err := strconv.ParseBool(values[SettingKeyExtraConcurrencyEnabled])
	if err != nil {
		return fallback
	}
	waitTimeoutSeconds, err := strconv.Atoi(values[SettingKeyExtraConcurrencyWaitTimeoutSeconds])
	if err != nil {
		return fallback
	}
	reservePercent, err := strconv.ParseFloat(values[SettingKeyExtraConcurrencyReservePercent], 64)
	if err != nil {
		return fallback
	}
	minReservedSlots, err := strconv.Atoi(values[SettingKeyExtraConcurrencyMinReservedSlots])
	if err != nil {
		return fallback
	}
	var platformReserves map[string]ExtraConcurrencyPlatformReserve
	if err := json.Unmarshal([]byte(values[SettingKeyExtraConcurrencyPlatformReserves]), &platformReserves); err != nil || platformReserves == nil {
		return fallback
	}

	settings := &SystemSettings{
		ExtraConcurrencyWaitTimeoutSeconds: waitTimeoutSeconds,
		ExtraConcurrencyReservePercent:     reservePercent,
		ExtraConcurrencyMinReservedSlots:   minReservedSlots,
		ExtraConcurrencyPlatformReserves:   platformReserves,
	}
	if err := validateExtraConcurrencySettings(settings); err != nil {
		return fallback
	}

	return ExtraConcurrencyRuntimeSettings{
		Enabled:            enabled,
		WaitTimeoutSeconds: waitTimeoutSeconds,
		ReservePercent:     reservePercent,
		MinReservedSlots:   minReservedSlots,
		PlatformReserves:   cloneExtraConcurrencyPlatformReserves(platformReserves),
	}
}

func disabledExtraConcurrencyRuntimeSettings() ExtraConcurrencyRuntimeSettings {
	return ExtraConcurrencyRuntimeSettings{
		Enabled:            false,
		WaitTimeoutSeconds: 30,
		ReservePercent:     10,
		MinReservedSlots:   1,
		PlatformReserves:   map[string]ExtraConcurrencyPlatformReserve{},
	}
}

func cloneExtraConcurrencyPlatformReserves(source map[string]ExtraConcurrencyPlatformReserve) map[string]ExtraConcurrencyPlatformReserve {
	cloned := make(map[string]ExtraConcurrencyPlatformReserve, len(source))
	for platform, reserve := range source {
		copy := reserve
		if reserve.ReservePercent != nil {
			value := *reserve.ReservePercent
			copy.ReservePercent = &value
		}
		if reserve.MinReservedSlots != nil {
			value := *reserve.MinReservedSlots
			copy.MinReservedSlots = &value
		}
		cloned[platform] = copy
	}
	return cloned
}

func cloneExtraConcurrencyRuntimeSettings(source ExtraConcurrencyRuntimeSettings) ExtraConcurrencyRuntimeSettings {
	copy := source
	copy.PlatformReserves = cloneExtraConcurrencyPlatformReserves(source.PlatformReserves)
	return copy
}
