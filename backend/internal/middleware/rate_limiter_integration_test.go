//go:build integration

package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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
		c.JSON(http.StatusOK, gin.H{"ok": trueREDACTED)
REDACTED)

	recorder := performRequest(router)
	require.Equal(t, http.StatusOK, recorder.Code)

	redisKey := limiter.prefix + "ttl-test:127.0.0.1"
	ttlBefore, err := rdb.PTTL(ctx, redisKey).Result()
REDACTED
	require.Greater(t, ttlBefore, time.Duration(0))
	require.LessOrEqual(t, ttlBefore, 2*time.Second)

	time.Sleep(50 * time.Millisecond)

	recorder = performRequest(router)
	require.Equal(t, http.StatusOK, recorder.Code)

	ttlAfter, err := rdb.PTTL(ctx, redisKey).Result()
REDACTED
	require.Less(t, ttlAfter, ttlBefore)
REDACTED

func TestRateLimiterFixesMissingTTL(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctx := context.Background()
	rdb := startRedis(t, ctx)
	limiter := NewRateLimiter(rdb)

	router := gin.New()
	router.Use(limiter.Limit("ttl-missing", 10, 2*time.Second))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": trueREDACTED)
REDACTED)

	redisKey := limiter.prefix + "ttl-missing:127.0.0.1"
	require.NoError(t, rdb.Set(ctx, redisKey, 5, 0).Err())

	ttlBefore, err := rdb.PTTL(ctx, redisKey).Result()
REDACTED
	require.Less(t, ttlBefore, time.Duration(0))

	recorder := performRequest(router)
	require.Equal(t, http.StatusOK, recorder.Code)

	ttlAfter, err := rdb.PTTL(ctx, redisKey).Result()
REDACTED
	require.Greater(t, ttlAfter, time.Duration(0))
REDACTED

func performRequest(router *gin.Engine) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
REDACTED

func startRedis(t *testing.T, ctx context.Context) *redis.Client {
REDACTED

	redisContainer, err := tcredis.Run(ctx, redisImageTag)
REDACTED
	t.Cleanup(func() {
		_ = redisContainer.Terminate(ctx)
REDACTED)

	redisHost, err := redisContainer.Host(ctx)
REDACTED
	redisPort, err := redisContainer.MappedPort(ctx, "6379/tcp")
REDACTED

	rdb := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%d", redisHost, redisPort.Int()),
		DB:   0,
REDACTED)
	require.NoError(t, rdb.Ping(ctx).Err())

	t.Cleanup(func() {
		_ = rdb.Close()
REDACTED)

	return rdb
REDACTED
