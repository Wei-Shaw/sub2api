package middleware

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
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

func TestRateLimiterGroupsIPv6By64(t *testing.T) {
	gin.SetMode(gin.TestMode)

	callCounts := make(map[string]int64)
	originalRun := rateLimitRun
	rateLimitRun = func(_ context.Context, _ *redis.Client, key string, _ int64) (int64, bool, error) {
		callCounts[key]++
		return callCounts[key], false, nil
	}
	t.Cleanup(func() { rateLimitRun = originalRun })

	limiter := NewRateLimiter(redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"}))
	router := gin.New()
	router.Use(limiter.LimitWithOptions("register", 1, time.Minute, RateLimitOptions{FailureMode: RateLimitFailClose}))
	router.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })

	request := func(remoteAddr string) int {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = remoteAddr
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		return recorder.Code
	}

	require.Equal(t, http.StatusOK, request("[2001:db8:1:2::1]:1234"))
	require.Equal(t, http.StatusTooManyRequests, request("[2001:db8:1:2::ffff]:5678"))
	require.Equal(t, http.StatusOK, request("[2001:db8:1:3::1]:1234"))
	require.Contains(t, callCounts, "rate_limit:register:2001:db8:1:2::/64")
}

func TestEmailSlidingLimitHashesNormalizedEmailAndPreservesBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var capturedKey string
	originalRun := slidingLimitRun
	slidingLimitRun = func(_ context.Context, _ *redis.Client, key string, _, _ int64, _ int, _ string) (bool, int64, error) {
		capturedKey = key
		return true, 1, nil
	}
	t.Cleanup(func() { slidingLimitRun = originalRun })

	body := []byte(`{"email":" User@Example.COM ","other":"kept"}`)
	limiter := NewRateLimiter(redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"}))
	router := gin.New()
	router.Use(limiter.LimitEmailSlidingWithOptions("verify", 5, time.Hour, RateLimitOptions{FailureMode: RateLimitFailClose}))
	router.POST("/test", func(c *gin.Context) {
		seen, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		require.Equal(t, body, seen)
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(body))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.True(t, strings.HasPrefix(capturedKey, "rate_limit:v2:verify:email:"))
	require.NotContains(t, capturedKey, "user@example.com")
}

func TestSuccessfulLimitCommitsSuccessAndReleasesFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalReserve := successfulLimitReserveRun
	originalCommit := successfulLimitCommitRun
	originalRelease := successfulLimitReleaseRun
	var commits, releases int
	var seenCommittedKey string
	successfulLimitReserveRun = func(_ context.Context, _ *redis.Client, _, _ string, _, _ int64, _ int, _ string, _ int64) (bool, int64, error) {
		return true, 1, nil
	}
	successfulLimitCommitRun = func(_ context.Context, _ *redis.Client, committedKey, _, _ string, _, _ int64) (bool, error) {
		commits++
		seenCommittedKey = committedKey
		return true, nil
	}
	successfulLimitReleaseRun = func(_ context.Context, _ *redis.Client, _, _ string) error {
		releases++
		return nil
	}
	t.Cleanup(func() {
		successfulLimitReserveRun = originalReserve
		successfulLimitCommitRun = originalCommit
		successfulLimitReleaseRun = originalRelease
	})

	limiter := NewRateLimiter(redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"}))
	middleware := limiter.LimitSuccessfulWithOptions("registration-success", 5, 24*time.Hour, RateLimitOptions{FailureMode: RateLimitFailClose})
	request := func(status int) {
		router := gin.New()
		router.Use(middleware)
		router.GET("/test", func(c *gin.Context) { c.Status(status) })
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.0.2.50:1234"
		router.ServeHTTP(httptest.NewRecorder(), req)
	}

	request(http.StatusCreated)
	request(http.StatusBadRequest)
	require.Equal(t, 1, commits)
	require.Equal(t, 1, releases)
	require.Contains(t, seenCommittedKey, "192.0.2.50/32")
}

func TestSuccessfulLimitReserveScriptPreventsConcurrentOversubscription(t *testing.T) {
	server := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	const requests = 20
	const limit = 5
	var allowed atomic.Int64
	var wg sync.WaitGroup
	wg.Add(requests)
	for i := 0; i < requests; i++ {
		go func(index int) {
			defer wg.Done()
			now := time.Now()
			ok, _, err := successfulLimitReserveRun(
				context.Background(), rdb,
				"rate_limit:test:committed", "rate_limit:test:pending",
				now.UnixMilli(), int64((24*time.Hour)/time.Millisecond), limit,
				fmt.Sprintf("request-%d", index), now.Add(5*time.Minute).UnixMilli(),
			)
			if err != nil {
				t.Errorf("reserve request %d: %v", index, err)
				return
			}
			if ok {
				allowed.Add(1)
			}
		}(i)
	}
	wg.Wait()
	require.Equal(t, int64(limit), allowed.Load())
}
