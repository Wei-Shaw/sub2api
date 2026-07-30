package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	ippkg "github.com/Wei-Shaw/sub2api/internal/pkg/ip"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestWindowTTLMillis(t *testing.T) {
	require.Equal(t, int64(1), windowTTLMillis(500*time.Microsecond))
	require.Equal(t, int64(1), windowTTLMillis(1500*time.Microsecond))
	require.Equal(t, int64(2), windowTTLMillis(2500*time.Microsecond))
}

func TestRateLimiterFailureModes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rdb := redis.NewClient(&redis.Options{
		Addr:         "127.0.0.1:1",
		DialTimeout:  50 * time.Millisecond,
		ReadTimeout:  50 * time.Millisecond,
		WriteTimeout: 50 * time.Millisecond,
	})
	t.Cleanup(func() {
		_ = rdb.Close()
	})

	limiter := NewRateLimiter(rdb)

	failOpenRouter := gin.New()
	failOpenRouter.Use(limiter.Limit("test", 1, time.Second))
	failOpenRouter.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	recorder := httptest.NewRecorder()
	failOpenRouter.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code)

	failCloseRouter := gin.New()
	failCloseRouter.Use(limiter.LimitWithOptions("test", 1, time.Second, RateLimitOptions{
		FailureMode: RateLimitFailClose,
	}))
	failCloseRouter.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	recorder = httptest.NewRecorder()
	failCloseRouter.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
}

func TestRateLimiterDifferentIPsIndependent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	callCounts := make(map[string]int64)
	originalRun := rateLimitRun
	rateLimitRun = func(ctx context.Context, client *redis.Client, key string, windowMillis int64) (int64, bool, error) {
		callCounts[key]++
		return callCounts[key], false, nil
	}
	t.Cleanup(func() {
		rateLimitRun = originalRun
	})

	limiter := NewRateLimiter(redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"}))

	router := gin.New()
	router.Use(limiter.Limit("api", 1, time.Second))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// 第一个 IP 的请求应通过
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req1.RemoteAddr = "10.0.0.1:1234"
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)
	require.Equal(t, http.StatusOK, rec1.Code, "第一个 IP 的第一次请求应通过")

	// 第二个 IP 的请求应独立通过（不受第一个 IP 的计数影响）
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.RemoteAddr = "10.0.0.2:5678"
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)
	require.Equal(t, http.StatusOK, rec2.Code, "第二个 IP 的第一次请求应独立通过")

	// 第一个 IP 的第二次请求应被限流
	req3 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req3.RemoteAddr = "10.0.0.1:1234"
	rec3 := httptest.NewRecorder()
	router.ServeHTTP(rec3, req3)
	require.Equal(t, http.StatusTooManyRequests, rec3.Code, "第一个 IP 的第二次请求应被限流")
}

func TestRateLimiterAllow(t *testing.T) {
	originalRun := rateLimitRun
	var gotKey string
	count := int64(0)
	rateLimitRun = func(ctx context.Context, client *redis.Client, key string, windowMillis int64) (int64, bool, error) {
		gotKey = key
		count++
		return count, false, nil
	}
	t.Cleanup(func() {
		rateLimitRun = originalRun
	})

	// PTTL 走真实客户端（不可达地址）→ 失败后 RetryAfter 应回退为完整窗口
	limiter := NewRateLimiter(redis.NewClient(&redis.Options{
		Addr:         "127.0.0.1:1",
		DialTimeout:  50 * time.Millisecond,
		ReadTimeout:  50 * time.Millisecond,
		WriteTimeout: 50 * time.Millisecond,
	}))

	res, err := limiter.Allow(context.Background(), "panel:global:user:42", 1, time.Minute)
	require.NoError(t, err)
	require.True(t, res.Allowed)
	require.Equal(t, int64(1), res.Count)
	require.Zero(t, res.RetryAfter)
	require.Equal(t, "rate_limit:panel:global:user:42", gotKey)

	res, err = limiter.Allow(context.Background(), "panel:global:user:42", 1, time.Minute)
	require.NoError(t, err)
	require.False(t, res.Allowed)
	require.Equal(t, int64(2), res.Count)
	require.Equal(t, time.Minute, res.RetryAfter)
}

func TestRateLimiterHonorsForwardedIPSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)

	callCounts := make(map[string]int64)
	originalRun := rateLimitRun
	rateLimitRun = func(ctx context.Context, client *redis.Client, key string, windowMillis int64) (int64, bool, error) {
		callCounts[key]++
		return callCounts[key], false, nil
	}
	t.Cleanup(func() {
		rateLimitRun = originalRun
	})

	limiter := NewRateLimiter(redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"}))

	router := gin.New()
	// 模拟 SessionBindingContext：开启转发 IP 兼容模式快照
	router.Use(func(c *gin.Context) {
		ippkg.SetForwardedIPSettings(c, true, nil)
		c.Next()
	})
	router.Use(limiter.Limit("fwd", 1, time.Second))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	send := func(xff string) int {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		// 所有请求都来自同一个反代地址
		req.RemoteAddr = "127.0.0.1:5678"
		req.Header.Set("X-Forwarded-For", xff)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec.Code
	}

	// 反代后两个不同的真实客户端应各自独立计数，不因共享代理地址被合并限流
	require.Equal(t, http.StatusOK, send("198.51.100.1"))
	require.Equal(t, http.StatusOK, send("198.51.100.2"))
	// 同一真实客户端第二次请求应被限流
	require.Equal(t, http.StatusTooManyRequests, send("198.51.100.1"))

	require.Contains(t, callCounts, "rate_limit:fwd:198.51.100.1")
	require.Contains(t, callCounts, "rate_limit:fwd:198.51.100.2")
}

func TestRateLimiterSuccessAndLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalRun := rateLimitRun
	counts := []int64{1, 2}
	callIndex := 0
	rateLimitRun = func(ctx context.Context, client *redis.Client, key string, windowMillis int64) (int64, bool, error) {
		if callIndex >= len(counts) {
			return counts[len(counts)-1], false, nil
		}
		value := counts[callIndex]
		callIndex++
		return value, false, nil
	}
	t.Cleanup(func() {
		rateLimitRun = originalRun
	})

	limiter := NewRateLimiter(redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"}))

	router := gin.New()
	router.Use(limiter.Limit("test", 1, time.Second))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code)

	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
}
