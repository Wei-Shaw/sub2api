//go:build integration

package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kirocooldown"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

const kiroCooldownRedisImageTag = "redis:8.4-alpine"

func TestRedisKiroCooldownStoreSharesCooldownAcrossInstances(t *testing.T) {
	ctx := context.Background()
	rdb := startKiroCooldownRedis(t, ctx)
	storeA := kirocooldown.NewStore(rdb)
	storeB := kirocooldown.NewStore(rdb)

	cooldown, err := storeA.Mark429(ctx, "token-shared")
	require.NoError(t, err)
	require.Equal(t, time.Minute, cooldown)

	err = storeB.CheckCooldown(ctx, "token-shared")
	require.Error(t, err)
	require.Contains(t, err.Error(), kirocooldown.CooldownReason429)

	require.NoError(t, storeB.MarkSuccess(ctx, "token-shared"))

	require.NoError(t, storeA.CheckCooldown(ctx, "token-shared"))
}

func TestRedisKiroCooldownStoreHealthyChecksDoNotPaceAcrossInstances(t *testing.T) {
	ctx := context.Background()
	rdb := startKiroCooldownRedis(t, ctx)
	storeA := kirocooldown.NewStore(rdb)
	storeB := kirocooldown.NewStore(rdb)

	require.NoError(t, storeA.CheckCooldown(ctx, "token-rate"))
	require.NoError(t, storeB.CheckCooldown(ctx, "token-rate"))
	require.Equal(t, int64(0), rdb.Exists(ctx, kirocooldown.RedisKey("token-rate")).Val())
}

func TestRedisKiroCooldownStoreHealthyChecksAllowConcurrentBurst(t *testing.T) {
	ctx := context.Background()
	rdb := startKiroCooldownRedis(t, ctx)
	store := kirocooldown.NewStore(rdb)

	const requests = 30
	start := make(chan struct{})
	errs := make(chan error, requests)
	var wg sync.WaitGroup
	wg.Add(requests)
	for range requests {
		go func() {
			defer wg.Done()
			<-start
			errs <- store.CheckCooldown(ctx, "token-burst")
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.Equal(t, int64(0), rdb.Exists(ctx, kirocooldown.RedisKey("token-burst")).Val())
}

func TestRedisKiroCooldownStoreRemovesLegacyPacingState(t *testing.T) {
	ctx := context.Background()
	rdb := startKiroCooldownRedis(t, ctx)
	store := kirocooldown.NewStore(rdb)
	key := kirocooldown.RedisKey("token-legacy")

	require.NoError(t, rdb.HSet(ctx, key, "last_request_ms", time.Now().Add(time.Hour).UnixMilli()).Err())
	require.NoError(t, store.CheckCooldown(ctx, "token-legacy"))
	require.False(t, rdb.HExists(ctx, key, "last_request_ms").Val())
}

func TestRedisKiroCooldownStoreSharesSuspendedStateAcrossInstances(t *testing.T) {
	ctx := context.Background()
	rdb := startKiroCooldownRedis(t, ctx)
	storeA := kirocooldown.NewStore(rdb)
	storeB := kirocooldown.NewStore(rdb)

	cooldown, err := storeA.MarkSuspended(ctx, "token-suspended")
	require.NoError(t, err)
	require.Equal(t, kirocooldown.LongCooldown, cooldown)

	err = storeB.CheckCooldown(ctx, "token-suspended")
	require.Error(t, err)
	require.Contains(t, err.Error(), kirocooldown.CooldownReasonSuspended)
}

func TestRedisKiroCooldownStoreSuspendedResetsFailCount(t *testing.T) {
	ctx := context.Background()
	rdb := startKiroCooldownRedis(t, ctx)
	store := kirocooldown.NewStore(rdb)

	_, err := store.Mark429(ctx, "token-reset")
	require.NoError(t, err)
	_, err = store.Mark429(ctx, "token-reset")
	require.NoError(t, err)

	cooldown, err := store.MarkSuspended(ctx, "token-reset")
	require.NoError(t, err)
	require.Equal(t, kirocooldown.LongCooldown, cooldown)

	cooldown, err = store.Mark429(ctx, "token-reset")
	require.NoError(t, err)
	require.Equal(t, time.Minute, cooldown)
}

func TestRedisKiroCooldownStoreCheckDifferentTokenIgnoresOldCooldown(t *testing.T) {
	ctx := context.Background()
	rdb := startKiroCooldownRedis(t, ctx)
	store := kirocooldown.NewStore(rdb)

	_, err := store.Mark429(ctx, "token-old")
	require.NoError(t, err)

	require.NoError(t, store.CheckCooldown(ctx, "token-new"))
}

func TestRedisKiroCooldownStoreUsesExpectedTTLs(t *testing.T) {
	ctx := context.Background()
	rdb := startKiroCooldownRedis(t, ctx)
	store := kirocooldown.NewStore(rdb)

	require.NoError(t, store.CheckCooldown(ctx, "token-ttl-active"))
	require.Equal(t, int64(0), rdb.Exists(ctx, kirocooldown.RedisKey("token-ttl-active")).Val())

	_, err := store.MarkSuspended(ctx, "token-ttl-state")
	require.NoError(t, err)
	stateTTL, err := rdb.PTTL(ctx, kirocooldown.RedisKey("token-ttl-state")).Result()
	require.NoError(t, err)
	require.Greater(t, stateTTL, 24*time.Hour)
	require.LessOrEqual(t, stateTTL, kirocooldown.StateTTL())
}

func startKiroCooldownRedis(t *testing.T, ctx context.Context) *redis.Client {
	t.Helper()
	ensureKiroCooldownDockerAvailable(t)

	redisContainer, err := tcredis.Run(ctx, kiroCooldownRedisImageTag)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = redisContainer.Terminate(ctx)
	})

	host, err := redisContainer.Host(ctx)
	require.NoError(t, err)
	port, err := redisContainer.MappedPort(ctx, "6379/tcp")
	require.NoError(t, err)

	rdb := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%d", host, port.Int()),
		DB:   0,
	})
	require.NoError(t, rdb.Ping(ctx).Err())
	t.Cleanup(func() {
		_ = rdb.Close()
	})
	return rdb
}

func ensureKiroCooldownDockerAvailable(t *testing.T) {
	t.Helper()
	if kiroCooldownDockerAvailable() {
		return
	}
	t.Skip("Docker 未启用，跳过依赖 testcontainers 的 Kiro cooldown 集成测试")
}

func kiroCooldownDockerAvailable() bool {
	if os.Getenv("DOCKER_HOST") != "" {
		return true
	}

	socketCandidates := []string{
		"/var/run/docker.sock",
		filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "docker.sock"),
		filepath.Join(kiroCooldownUserHomeDir(), ".docker", "run", "docker.sock"),
		filepath.Join(kiroCooldownUserHomeDir(), ".docker", "desktop", "docker.sock"),
		filepath.Join("/run/user", strconv.Itoa(os.Getuid()), "docker.sock"),
	}

	for _, socket := range socketCandidates {
		if socket == "" {
			continue
		}
		if _, err := os.Stat(socket); err == nil {
			return true
		}
	}
	return false
}

func kiroCooldownUserHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}
