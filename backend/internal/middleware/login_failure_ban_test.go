package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newLoginBanTestRouter(t *testing.T, options LoginBanOptions) (*gin.Engine, *redis.Client, *miniredis.Miniredis) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })

	guard := NewLoginFailureBanGuard(redisClient, options)
	router := gin.New()
	router.POST("/login", guard.Guard(), func(c *gin.Context) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
	})
	return router, redisClient, redisServer
}

func doLogin(t *testing.T, router *gin.Engine, remoteAddr string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.RemoteAddr = remoteAddr
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder.Code
}

func TestLoginFailureBanDisabledNeverBans(t *testing.T) {
	router, redisClient, _ := newLoginBanTestRouter(t, LoginBanOptions{
		Enabled:     false,
		Threshold:   3,
		Window:      time.Minute,
		BanDuration: time.Hour,
	})

	for i := 0; i < 10; i++ {
		require.Equal(t, http.StatusUnauthorized, doLogin(t, router, "203.0.113.10:1234"))
	}
	keys, err := redisClient.Keys(t.Context(), "login_ban:*").Result()
	require.NoError(t, err)
	require.Empty(t, keys)
}

func TestLoginFailureBanPrivateIPSkipped(t *testing.T) {
	router, redisClient, _ := newLoginBanTestRouter(t, LoginBanOptions{
		Enabled:     true,
		Threshold:   3,
		Window:      time.Minute,
		BanDuration: time.Hour,
	})

	for _, addr := range []string{"127.0.0.1:1", "10.0.0.5:1", "192.168.1.5:1", "172.18.0.1:1", "[::1]:1"} {
		for i := 0; i < 5; i++ {
			require.Equal(t, http.StatusUnauthorized, doLogin(t, router, addr))
		}
	}
	keys, err := redisClient.Keys(t.Context(), "login_ban:*").Result()
	require.NoError(t, err)
	require.Empty(t, keys)
}

func TestLoginFailureBanExemptIPsSkipped(t *testing.T) {
	router, redisClient, _ := newLoginBanTestRouter(t, LoginBanOptions{
		Enabled:     true,
		Threshold:   2,
		Window:      time.Minute,
		BanDuration: time.Hour,
		ExemptIPs:   []string{"203.0.113.10", "198.51.100.0/24"},
	})

	// 精确 IP 与 CIDR 命中的地址均不计数
	for i := 0; i < 5; i++ {
		require.Equal(t, http.StatusUnauthorized, doLogin(t, router, "203.0.113.10:1234"))
		require.Equal(t, http.StatusUnauthorized, doLogin(t, router, "198.51.100.77:1234"))
	}
	keys, err := redisClient.Keys(t.Context(), "login_ban:*").Result()
	require.NoError(t, err)
	require.Empty(t, keys)

	// 未命中豁免的地址正常封禁
	require.Equal(t, http.StatusUnauthorized, doLogin(t, router, "203.0.113.11:1234"))
	require.Equal(t, http.StatusUnauthorized, doLogin(t, router, "203.0.113.11:1234"))
	require.Equal(t, http.StatusTooManyRequests, doLogin(t, router, "203.0.113.11:1234"))
}

func TestLoginFailureBanTriggersAndBlocks(t *testing.T) {
	router, redisClient, _ := newLoginBanTestRouter(t, LoginBanOptions{
		Enabled:     true,
		Threshold:   3,
		Window:      10 * time.Minute,
		BanDuration: time.Hour,
	})

	// 阈值前 2 次：正常进入 handler 返回 401
	require.Equal(t, http.StatusUnauthorized, doLogin(t, router, "203.0.113.10:1234"))
	require.Equal(t, http.StatusUnauthorized, doLogin(t, router, "203.0.113.10:1234"))

	// 第 3 次失败达到阈值：写入封禁 key
	require.Equal(t, http.StatusUnauthorized, doLogin(t, router, "203.0.113.10:1234"))
	exists, err := redisClient.Exists(t.Context(), "login_ban:blocked:203.0.113.10").Result()
	require.NoError(t, err)
	require.Equal(t, int64(1), exists)

	// 第 4 次起：前置拦截，429 + Retry-After
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.RemoteAddr = "203.0.113.10:1234"
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	require.NotEmpty(t, recorder.Header().Get("Retry-After"))

	// 其他 IP 不受影响
	require.Equal(t, http.StatusUnauthorized, doLogin(t, router, "203.0.113.11:1234"))
}

func TestLoginFailureBanWindowExpiryResetsCount(t *testing.T) {
	router, _, redisServer := newLoginBanTestRouter(t, LoginBanOptions{
		Enabled:     true,
		Threshold:   3,
		Window:      time.Minute,
		BanDuration: time.Hour,
	})

	require.Equal(t, http.StatusUnauthorized, doLogin(t, router, "203.0.113.10:1234"))
	require.Equal(t, http.StatusUnauthorized, doLogin(t, router, "203.0.113.10:1234"))

	// 窗口过期后计数归零，需要重新累计
	redisServer.FastForward(61 * time.Second)
	require.Equal(t, http.StatusUnauthorized, doLogin(t, router, "203.0.113.10:1234"))
	require.Equal(t, http.StatusUnauthorized, doLogin(t, router, "203.0.113.10:1234"))
	require.Equal(t, http.StatusUnauthorized, doLogin(t, router, "203.0.113.10:1234"))

	// 第 4 次失败（新一轮窗口的第 3 次）触发封禁
	require.Equal(t, http.StatusTooManyRequests, doLogin(t, router, "203.0.113.10:1234"))
}

func TestLoginFailureBanBanExpiryLiftsBan(t *testing.T) {
	router, _, redisServer := newLoginBanTestRouter(t, LoginBanOptions{
		Enabled:     true,
		Threshold:   2,
		Window:      time.Minute,
		BanDuration: time.Hour,
	})

	require.Equal(t, http.StatusUnauthorized, doLogin(t, router, "203.0.113.10:1234"))
	require.Equal(t, http.StatusUnauthorized, doLogin(t, router, "203.0.113.10:1234"))
	require.Equal(t, http.StatusTooManyRequests, doLogin(t, router, "203.0.113.10:1234"))

	// 封禁到期后自动解除
	redisServer.FastForward(time.Hour + time.Minute)
	require.Equal(t, http.StatusUnauthorized, doLogin(t, router, "203.0.113.10:1234"))
}

func TestLoginFailureBanBanKeyUsesConfiguredDuration(t *testing.T) {
	router, redisClient, _ := newLoginBanTestRouter(t, LoginBanOptions{
		Enabled:     true,
		Threshold:   2,
		Window:      time.Minute,
		BanDuration: time.Hour,
	})

	require.Equal(t, http.StatusUnauthorized, doLogin(t, router, "203.0.113.10:1234"))
	require.Equal(t, http.StatusUnauthorized, doLogin(t, router, "203.0.113.10:1234"))

	ttl, err := redisClient.TTL(t.Context(), "login_ban:blocked:203.0.113.10").Result()
	require.NoError(t, err)
	require.Greater(t, ttl, 50*time.Minute)
	require.LessOrEqual(t, ttl, time.Hour)
}

func TestLoginFailureBan429NotCounted(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })

	gin.SetMode(gin.TestMode)
	guard := NewLoginFailureBanGuard(redisClient, LoginBanOptions{
		Enabled:     true,
		Threshold:   3,
		Window:      time.Minute,
		BanDuration: time.Hour,
	})
	router := gin.New()
	router.POST("/login", guard.Guard(), func(c *gin.Context) {
		// 模拟内层限流器节流响应
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
	})

	for i := 0; i < 10; i++ {
		require.Equal(t, http.StatusTooManyRequests, doLogin(t, router, "203.0.113.10:1234"))
	}
	keys, err := redisClient.Keys(t.Context(), "login_ban:*").Result()
	require.NoError(t, err)
	require.Empty(t, keys, "429 节流响应不应计入失败计数")
}

func TestLoginFailureBanFailOpenOnRedisError(t *testing.T) {
	redisClient := redis.NewClient(&redis.Options{
		Addr:         "127.0.0.1:1",
		DialTimeout:  50 * time.Millisecond,
		ReadTimeout:  50 * time.Millisecond,
		WriteTimeout: 50 * time.Millisecond,
	})
	t.Cleanup(func() { _ = redisClient.Close() })

	gin.SetMode(gin.TestMode)
	guard := NewLoginFailureBanGuard(redisClient, LoginBanOptions{
		Enabled:     true,
		Threshold:   2,
		Window:      time.Minute,
		BanDuration: time.Hour,
	})
	router := gin.New()
	router.POST("/login", guard.Guard(), func(c *gin.Context) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
	})

	for i := 0; i < 5; i++ {
		require.Equal(t, http.StatusUnauthorized, doLogin(t, router, "203.0.113.10:1234"))
	}
}

func TestLoginFailureBanIPv6MergedByPrefix(t *testing.T) {
	router, redisClient, _ := newLoginBanTestRouter(t, LoginBanOptions{
		Enabled:        true,
		Threshold:      2,
		Window:         time.Minute,
		BanDuration:    time.Hour,
		IPv6PrefixBits: 64,
	})

	// 同 /64 网段的两个地址合并计数
	require.Equal(t, http.StatusUnauthorized, doLogin(t, router, "[2001:db8:1:2::10]:1234"))
	require.Equal(t, http.StatusUnauthorized, doLogin(t, router, "[2001:db8:1:2::20]:1234"))

	exists, err := redisClient.Exists(t.Context(), "login_ban:blocked:2001:db8:1:2::/64").Result()
	require.NoError(t, err)
	require.Equal(t, int64(1), exists, "同 /64 前缀的地址应共享封禁")

	// 网段内第三个地址被前置拦截
	require.Equal(t, http.StatusTooManyRequests, doLogin(t, router, "[2001:db8:1:2::30]:1234"))
	// 其他网段不受影响
	require.Equal(t, http.StatusUnauthorized, doLogin(t, router, "[2001:db8:9:9::1]:1234"))
}

func TestIsPubliclyRoutableIP(t *testing.T) {
	require.True(t, isPubliclyRoutableIP("203.0.113.10"))
	require.True(t, isPubliclyRoutableIP("2001:db8:1:2::10"))
	require.False(t, isPubliclyRoutableIP("127.0.0.1"))
	require.False(t, isPubliclyRoutableIP("10.1.2.3"))
	require.False(t, isPubliclyRoutableIP("172.18.0.1"))
	require.False(t, isPubliclyRoutableIP("192.168.1.1"))
	require.False(t, isPubliclyRoutableIP("::1"))
	require.False(t, isPubliclyRoutableIP("fd00::1"))
	require.False(t, isPubliclyRoutableIP("fe80::1"))
	require.False(t, isPubliclyRoutableIP("not-an-ip"))
	require.False(t, isPubliclyRoutableIP(""))
}
