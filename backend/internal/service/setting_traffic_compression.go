package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/compresslevel"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

const (
	trafficCompressionRedisKey     = "settings:traffic_compression_levels"
	trafficCompressionRedisTTL     = 5 * time.Minute
	trafficCompressionLocalTTL     = 30 * time.Second
	trafficCompressionErrorTTL     = 5 * time.Second
	trafficCompressionLoadTimeout  = 5 * time.Second
	trafficCompressionSingleflight = "traffic_compression_levels"
)

type cachedTrafficCompressionLevels struct {
	settings  compresslevel.Settings
	expiresAt time.Time
}

type trafficCompressionRuntime struct {
	redis *redis.Client
	cache atomic.Pointer[cachedTrafficCompressionLevels]
	sf    singleflight.Group
}

var trafficCompressionSettingKeys = []string{
	SettingKeyCompressionBrotliLevel,
	SettingKeyCompressionZstdLevel,
	SettingKeyCompressionGzipLevel,
	SettingKeyCompressionDeflateLevel,
}

func trafficCompressionLevelsFromSystemSettings(settings *SystemSettings) compresslevel.Settings {
	if settings == nil {
		return compresslevel.Default()
	}
	return compresslevel.Settings{
		Brotli:  settings.CompressionBrotliLevel,
		Zstd:    settings.CompressionZstdLevel,
		Gzip:    settings.CompressionGzipLevel,
		Deflate: settings.CompressionDeflateLevel,
	}
}

// normalizeTrafficCompressionLevelsForWrite keeps legacy direct service callers
// compatible: a completely zero SystemSettings predates these fields and means
// defaults. The admin handler resolves omitted fields from stored settings, so
// an explicit invalid value still reaches validation and is rejected.
func normalizeTrafficCompressionLevelsForWrite(settings *SystemSettings) compresslevel.Settings {
	levels := trafficCompressionLevelsFromSystemSettings(settings)
	if settings != nil && settings.CompressionBrotliLevel == 0 && settings.CompressionZstdLevel == 0 &&
		settings.CompressionGzipLevel == 0 && settings.CompressionDeflateLevel == 0 {
		levels = compresslevel.Default()
		settings.CompressionBrotliLevel = levels.Brotli
		settings.CompressionZstdLevel = levels.Zstd
		settings.CompressionGzipLevel = levels.Gzip
		settings.CompressionDeflateLevel = levels.Deflate
	}
	return levels
}

func parseTrafficCompressionLevels(values map[string]string) compresslevel.Settings {
	levels := compresslevel.Default()
	parse := func(key string, target *int) {
		raw, ok := values[key]
		if !ok || strings.TrimSpace(raw) == "" {
			return
		}
		value, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil {
			slog.Warn("invalid traffic compression level; using safe defaults", "setting", key, "value", raw, "error", err)
			return
		}
		*target = value
	}
	parse(SettingKeyCompressionBrotliLevel, &levels.Brotli)
	parse(SettingKeyCompressionZstdLevel, &levels.Zstd)
	parse(SettingKeyCompressionGzipLevel, &levels.Gzip)
	parse(SettingKeyCompressionDeflateLevel, &levels.Deflate)
	if err := levels.Validate(); err != nil {
		slog.Warn("out-of-range traffic compression level; using safe defaults", "error", err)
		return compresslevel.Default()
	}
	return levels
}

// GetTrafficCompressionLevels resolves a per-request settings snapshot without
// querying the database on every request. The hot path uses a 30-second local
// cache, then a five-minute Redis cache, and only then falls back to the DB.
func (s *SettingService) GetTrafficCompressionLevels(ctx context.Context) compresslevel.Settings {
	if s == nil {
		return compresslevel.Default()
	}
	if cached := s.trafficCompression.cache.Load(); cached != nil && time.Now().Before(cached.expiresAt) {
		return cached.settings
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result, _, _ := s.trafficCompression.sf.Do(trafficCompressionSingleflight, func() (any, error) {
		if cached := s.trafficCompression.cache.Load(); cached != nil && time.Now().Before(cached.expiresAt) {
			return cached.settings, nil
		}
		previous := s.trafficCompression.cache.Load()
		loadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), trafficCompressionLoadTimeout)
		defer cancel()

		if client := s.trafficCompression.redis; client != nil {
			raw, err := client.Get(loadCtx, trafficCompressionRedisKey).Result()
			switch {
			case err == nil:
				var levels compresslevel.Settings
				if unmarshalErr := json.Unmarshal([]byte(raw), &levels); unmarshalErr != nil {
					slog.Warn("invalid cached traffic compression levels", "error", unmarshalErr)
				} else if validateErr := levels.Validate(); validateErr != nil {
					slog.Warn("out-of-range cached traffic compression levels", "error", validateErr)
				} else {
					return s.cacheTrafficCompressionLevels(levels, trafficCompressionLocalTTL), nil
				}
			case errors.Is(err, redis.Nil):
				// Cache miss: load the persistent settings below.
			default:
				slog.Warn("failed to read traffic compression levels from Redis", "error", err)
			}
		}

		if s.settingRepo == nil {
			return s.cacheTrafficCompressionLevels(compresslevel.Default(), trafficCompressionLocalTTL), nil
		}
		values, err := s.settingRepo.GetMultiple(loadCtx, trafficCompressionSettingKeys)
		if err != nil {
			slog.Warn("failed to load traffic compression levels", "error", err)
			levels := compresslevel.Default()
			if previous != nil {
				levels = previous.settings
			}
			return s.cacheTrafficCompressionLevels(levels, trafficCompressionErrorTTL), nil
		}
		levels := parseTrafficCompressionLevels(values)
		s.storeTrafficCompressionLevelsRedis(loadCtx, levels)
		return s.cacheTrafficCompressionLevels(levels, trafficCompressionLocalTTL), nil
	})
	if levels, ok := result.(compresslevel.Settings); ok {
		return levels
	}
	return compresslevel.Default()
}

func (s *SettingService) cacheTrafficCompressionLevels(levels compresslevel.Settings, ttl time.Duration) compresslevel.Settings {
	s.trafficCompression.cache.Store(&cachedTrafficCompressionLevels{
		settings:  levels,
		expiresAt: time.Now().Add(ttl),
	})
	return levels
}

func (s *SettingService) storeTrafficCompressionLevelsRedis(ctx context.Context, levels compresslevel.Settings) {
	client := s.trafficCompression.redis
	if client == nil {
		return
	}
	payload, err := json.Marshal(levels)
	if err != nil {
		slog.Warn("failed to encode traffic compression levels", "error", err)
		return
	}
	if err := client.Set(ctx, trafficCompressionRedisKey, payload, trafficCompressionRedisTTL).Err(); err != nil {
		slog.Warn("failed to cache traffic compression levels in Redis", "error", err)
	}
}

func (s *SettingService) refreshTrafficCompressionLevels(ctx context.Context, settings *SystemSettings) {
	if s == nil {
		return
	}
	levels := trafficCompressionLevelsFromSystemSettings(settings)
	s.trafficCompression.sf.Forget(trafficCompressionSingleflight)
	s.cacheTrafficCompressionLevels(levels, trafficCompressionLocalTTL)
	if ctx == nil {
		ctx = context.Background()
	}
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), trafficCompressionLoadTimeout)
	defer cancel()
	s.storeTrafficCompressionLevelsRedis(writeCtx, levels)
}
