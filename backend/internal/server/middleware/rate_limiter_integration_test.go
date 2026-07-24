//go:build integration

package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

const redisImageTag = "redis:8.4-alpine"

func TestRateLimiterSetsTTLAndDoesNotRefresh(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctx := context.Background()
	rdb := startRedis(t, ctx)
	limiter := NewRateLimiter(rdb)

	router := gin.New()
	router.Use(limiter.Limit("ttl-test", 10, 2*time.Second))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	recorder := performRequest(router)
	require.Equal(t, http.StatusOK, recorder.Code)

	redisKey := limiter.prefix + "ttl-test:127.0.0.1"
	ttlBefore, err := rdb.PTTL(ctx, redisKey).Result()
	require.NoError(t, err)
	require.Greater(t, ttlBefore, time.Duration(0))
	require.LessOrEqual(t, ttlBefore, 2*time.Second)

	time.Sleep(50 * time.Millisecond)

	recorder = performRequest(router)
	require.Equal(t, http.StatusOK, recorder.Code)

	ttlAfter, err := rdb.PTTL(ctx, redisKey).Result()
	require.NoError(t, err)
	require.Less(t, ttlAfter, ttlBefore)
}

func TestRateLimiterFixesMissingTTL(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctx := context.Background()
	rdb := startRedis(t, ctx)
	limiter := NewRateLimiter(rdb)

	router := gin.New()
	router.Use(limiter.Limit("ttl-missing", 10, 2*time.Second))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	redisKey := limiter.prefix + "ttl-missing:127.0.0.1"
	require.NoError(t, rdb.Set(ctx, redisKey, 5, 0).Err())

	ttlBefore, err := rdb.PTTL(ctx, redisKey).Result()
	require.NoError(t, err)
	require.Less(t, ttlBefore, time.Duration(0))

	recorder := performRequest(router)
	require.Equal(t, http.StatusOK, recorder.Code)

	ttlAfter, err := rdb.PTTL(ctx, redisKey).Result()
	require.NoError(t, err)
	require.Greater(t, ttlAfter, time.Duration(0))
}

func performRequest(router *gin.Engine) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func startRedis(t *testing.T, ctx context.Context) *redis.Client {
	t.Helper()
	ensureDockerAvailable(t)

	redisContainer, err := tcredis.Run(ctx, redisImageTag)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = redisContainer.Terminate(ctx)
	})

	redisHost, err := redisContainer.Host(ctx)
	require.NoError(t, err)
	redisPort, err := redisContainer.MappedPort(ctx, "6379/tcp")
	require.NoError(t, err)

	rdb := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%d", redisHost, redisPort.Int()),
		DB:   0,
	})
	require.NoError(t, rdb.Ping(ctx).Err())

	t.Cleanup(func() {
		_ = rdb.Close()
	})

	return rdb
}

func ensureDockerAvailable(t *testing.T) {
	t.Helper()
	if dockerAvailable() {
		return
	}
	t.Skip("Docker 未启用，跳过依赖 testcontainers 的集成测试")
}

func dockerAvailable() bool {
	if os.Getenv("DOCKER_HOST") != "" {
		return true
	}

	socketCandidates := []string{
		"/var/run/docker.sock",
		filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "docker.sock"),
		filepath.Join(userHomeDir(), ".docker", "run", "docker.sock"),
		filepath.Join(userHomeDir(), ".docker", "desktop", "docker.sock"),
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

func userHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}
