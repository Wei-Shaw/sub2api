package service

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/compresslevel"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type trafficCompressionRepoStub struct {
	SettingRepository
	mu               sync.Mutex
	values           map[string]string
	getMultipleCalls int
	setMultipleCalls int
	getMultipleErr   error
}

func (r *trafficCompressionRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.getMultipleCalls++
	if r.getMultipleErr != nil {
		return nil, r.getMultipleErr
	}
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

type trafficCompressionCacheStub struct {
	levels   *compresslevel.Settings
	getErr   error
	setErr   error
	getCalls int
	setCalls int
}

func (c *trafficCompressionCacheStub) Get(context.Context) (*compresslevel.Settings, error) {
	c.getCalls++
	return c.levels, c.getErr
}

func (c *trafficCompressionCacheStub) Set(_ context.Context, levels compresslevel.Settings) error {
	c.setCalls++
	if c.setErr != nil {
		return c.setErr
	}
	c.levels = &levels
	return nil
}

func newTrafficCompressionCacheTest() (*trafficCompressionCacheStub, *trafficCompressionRepoStub, *SettingService) {
	cache := &trafficCompressionCacheStub{}
	repo := &trafficCompressionRepoStub{values: make(map[string]string)}
	svc := NewSettingService(repo, nil)
	svc.trafficCompression.shared = cache
	return cache, repo, svc
}

func expireTrafficCompressionLocalCache(svc *SettingService) {
	cached := *svc.trafficCompression.cache.Load()
	cached.expiresAt = time.Now().Add(-time.Second)
	svc.trafficCompression.cache.Store(&cached)
}

func TestTrafficCompressionLevelsCacheTTLAndFallback(t *testing.T) {
	cache, repo, svc := newTrafficCompressionCacheTest()
	ctx := context.Background()
	initial := compresslevel.Settings{Brotli: 7, Zstd: 9, Gzip: 4, Deflate: 2}
	repo.setLevels(initial)

	require.Equal(t, initial, svc.GetTrafficCompressionLevels(ctx))
	reads, _ := repo.calls()
	require.Equal(t, 1, reads)
	require.Equal(t, initial, *cache.levels)
	ttl := time.Until(svc.trafficCompression.cache.Load().expiresAt)
	require.Greater(t, ttl, 29*time.Second)
	require.LessOrEqual(t, ttl, trafficCompressionLocalTTL)

	updated := compresslevel.Settings{Brotli: 8, Zstd: 10, Gzip: 5, Deflate: 3}
	repo.setLevels(updated)
	require.Equal(t, initial, svc.GetTrafficCompressionLevels(ctx), "local TTL must avoid another backend read")
	require.Equal(t, 1, cache.getCalls)
	expireTrafficCompressionLocalCache(svc)
	require.Equal(t, initial, svc.GetTrafficCompressionLevels(ctx), "shared cache hit must avoid a DB read")
	reads, _ = repo.calls()
	require.Equal(t, 1, reads)

	cache.levels = nil
	expireTrafficCompressionLocalCache(svc)
	require.Equal(t, updated, svc.GetTrafficCompressionLevels(ctx), "shared cache miss must fall back to DB")
	reads, _ = repo.calls()
	require.Equal(t, 2, reads)
}

func TestTrafficCompressionLevelsWriteThroughSharedCache(t *testing.T) {
	cache, repo, writer := newTrafficCompressionCacheTest()
	levels := compresslevel.Settings{Brotli: 10, Zstd: 12, Gzip: 7, Deflate: 5}
	writer.refreshTrafficCompressionLevels(context.Background(), &SystemSettings{
		CompressionBrotliLevel:  levels.Brotli,
		CompressionZstdLevel:    levels.Zstd,
		CompressionGzipLevel:    levels.Gzip,
		CompressionDeflateLevel: levels.Deflate,
	})
	require.Equal(t, levels, *cache.levels)
	require.Equal(t, 1, cache.setCalls)

	reader := NewSettingService(repo, nil)
	reader.trafficCompression.shared = cache
	require.Equal(t, levels, reader.GetTrafficCompressionLevels(context.Background()))
	reads, _ := repo.calls()
	require.Zero(t, reads, "a second instance must load the write-through cache value without DB access")
}

func TestTrafficCompressionLevelsSharedCacheFailureFallsBackToDB(t *testing.T) {
	for _, tc := range []struct {
		name   string
		levels *compresslevel.Settings
		err    error
	}{
		{name: "unavailable", err: errors.New("cache unavailable")},
		{name: "invalid levels", levels: &compresslevel.Settings{Brotli: 12}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cache, repo, svc := newTrafficCompressionCacheTest()
			cache.levels, cache.getErr = tc.levels, tc.err
			cache.setErr = errors.New("cache write failed")
			levels := compresslevel.Settings{Brotli: 7, Zstd: 9, Gzip: 4, Deflate: 2}
			repo.setLevels(levels)
			require.Equal(t, levels, svc.GetTrafficCompressionLevels(context.Background()))
			reads, _ := repo.calls()
			require.Equal(t, 1, reads)
			require.Equal(t, 1, cache.setCalls)
		})
	}
}

func TestTrafficCompressionLevelsDBFailureUsesPreviousSnapshot(t *testing.T) {
	cache, repo, svc := newTrafficCompressionCacheTest()
	levels := compresslevel.Settings{Brotli: 7, Zstd: 9, Gzip: 4, Deflate: 2}
	repo.setLevels(levels)
	require.Equal(t, levels, svc.GetTrafficCompressionLevels(context.Background()))
	cache.levels = nil
	repo.getMultipleErr = errors.New("database unavailable")
	expireTrafficCompressionLocalCache(svc)
	require.Equal(t, levels, svc.GetTrafficCompressionLevels(context.Background()))
	require.LessOrEqual(t, time.Until(svc.trafficCompression.cache.Load().expiresAt), trafficCompressionErrorTTL)
}

func TestTrafficCompressionLevelsPersistenceAndValidation(t *testing.T) {
	_, repo, svc := newTrafficCompressionCacheTest()
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
