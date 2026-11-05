package service

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/compresslevel"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type trafficCompressionRepoStub struct {
	SettingRepository
	mu               sync.Mutex
	values           map[string]string
	getMultipleCalls int
	setMultipleCalls int
}

func (r *trafficCompressionRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.getMultipleCalls++
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (r *trafficCompressionRepoStub) SetMultiple(_ context.Context, settings map[string]string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.setMultipleCalls++
	if r.values == nil {
		r.values = make(map[string]string)
	}
	for key, value := range settings {
		r.values[key] = value
	}
	return nil
}

func (r *trafficCompressionRepoStub) GetAll(context.Context) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]string, len(r.values))
	for key, value := range r.values {
		out[key] = value
	}
	return out, nil
}

func (r *trafficCompressionRepoStub) setLevels(levels compresslevel.Settings) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.values = map[string]string{
		SettingKeyCompressionBrotliLevel:  strconv.Itoa(levels.Brotli),
		SettingKeyCompressionZstdLevel:    strconv.Itoa(levels.Zstd),
		SettingKeyCompressionGzipLevel:    strconv.Itoa(levels.Gzip),
		SettingKeyCompressionDeflateLevel: strconv.Itoa(levels.Deflate),
	}
}

func (r *trafficCompressionRepoStub) calls() (int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.getMultipleCalls, r.setMultipleCalls
}

func newTrafficCompressionCacheTest(t *testing.T) (*miniredis.Miniredis, *redis.Client, *trafficCompressionRepoStub, *SettingService) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	repo := &trafficCompressionRepoStub{values: make(map[string]string)}
	svc := NewSettingService(repo, nil)
	svc.trafficCompression.redis = client
	return server, client, repo, svc
}

func expireTrafficCompressionLocalCache(svc *SettingService) {
	svc.trafficCompression.cache.Load().expiresAt = time.Now().Add(-time.Second)
}

func TestTrafficCompressionLevelsCacheTTLAndFallback(t *testing.T) {
	server, client, repo, svc := newTrafficCompressionCacheTest(t)
	ctx := context.Background()
	initial := compresslevel.Settings{Brotli: 7, Zstd: 9, Gzip: 4, Deflate: 2}
	repo.setLevels(initial)

	require.Equal(t, initial, svc.GetTrafficCompressionLevels(ctx))
	reads, _ := repo.calls()
	require.Equal(t, 1, reads)
	ttl := server.TTL(trafficCompressionRedisKey)
	require.Greater(t, ttl, 4*time.Minute)
	require.LessOrEqual(t, ttl, trafficCompressionRedisTTL)

	updated := compresslevel.Settings{Brotli: 8, Zstd: 10, Gzip: 5, Deflate: 3}
	repo.setLevels(updated)
	require.Equal(t, initial, svc.GetTrafficCompressionLevels(ctx), "local TTL must avoid another backend read")
	expireTrafficCompressionLocalCache(svc)
	require.Equal(t, initial, svc.GetTrafficCompressionLevels(ctx), "Redis hit must avoid a DB read")
	reads, _ = repo.calls()
	require.Equal(t, 1, reads)

	require.NoError(t, client.Del(ctx, trafficCompressionRedisKey).Err())
	expireTrafficCompressionLocalCache(svc)
	require.Equal(t, updated, svc.GetTrafficCompressionLevels(ctx), "Redis miss must fall back to DB")
	reads, _ = repo.calls()
	require.Equal(t, 2, reads)

	third := compresslevel.Settings{Brotli: 9, Zstd: 11, Gzip: 6, Deflate: 4}
	repo.setLevels(third)
	server.FastForward(trafficCompressionRedisTTL + time.Second)
	expireTrafficCompressionLocalCache(svc)
	require.Equal(t, third, svc.GetTrafficCompressionLevels(ctx), "expired Redis entry must be reloaded from DB")
	reads, _ = repo.calls()
	require.Equal(t, 3, reads)
}

func TestTrafficCompressionLevelsWriteThroughRedis(t *testing.T) {
	server, _, repo, writer := newTrafficCompressionCacheTest(t)
	levels := compresslevel.Settings{Brotli: 10, Zstd: 12, Gzip: 7, Deflate: 5}
	writer.refreshTrafficCompressionLevels(context.Background(), &SystemSettings{
		CompressionBrotliLevel:  levels.Brotli,
		CompressionZstdLevel:    levels.Zstd,
		CompressionGzipLevel:    levels.Gzip,
		CompressionDeflateLevel: levels.Deflate,
	})
	require.Greater(t, server.TTL(trafficCompressionRedisKey), 4*time.Minute)

	reader := NewSettingService(repo, nil)
	reader.trafficCompression.redis = writer.trafficCompression.redis
	require.Equal(t, levels, reader.GetTrafficCompressionLevels(context.Background()))
	reads, _ := repo.calls()
	require.Zero(t, reads, "a second instance must load the write-through Redis value without DB access")
}

func TestTrafficCompressionLevelsPersistenceAndValidation(t *testing.T) {
	_, _, repo, svc := newTrafficCompressionCacheTest(t)
	ctx := context.Background()
	levels := compresslevel.Settings{Brotli: 11, Zstd: 19, Gzip: 9, Deflate: 0}
	settings := &SystemSettings{
		CompressionBrotliLevel:  levels.Brotli,
		CompressionZstdLevel:    levels.Zstd,
		CompressionGzipLevel:    levels.Gzip,
		CompressionDeflateLevel: levels.Deflate,
	}
	require.NoError(t, svc.UpdateSettings(ctx, settings))
	for key, want := range map[string]string{
		SettingKeyCompressionBrotliLevel:  "11",
		SettingKeyCompressionZstdLevel:    "19",
		SettingKeyCompressionGzipLevel:    "9",
		SettingKeyCompressionDeflateLevel: "0",
	} {
		require.Equal(t, want, repo.values[key])
	}
	require.Equal(t, levels, svc.GetTrafficCompressionLevels(ctx))

	_, writes := repo.calls()
	err := svc.UpdateSettings(ctx, &SystemSettings{
		CompressionBrotliLevel:  5,
		CompressionZstdLevel:    0,
		CompressionGzipLevel:    6,
		CompressionDeflateLevel: 6,
	})
	require.Error(t, err)
	require.Equal(t, 400, infraerrors.Code(err))
	_, writesAfter := repo.calls()
	require.Equal(t, writes, writesAfter, "invalid levels must not persist any settings")

	require.NoError(t, svc.UpdateSettings(ctx, &SystemSettings{}), "legacy zero-value callers retain defaults")
	require.Equal(t, compresslevel.Default(), svc.GetTrafficCompressionLevels(ctx))
}
