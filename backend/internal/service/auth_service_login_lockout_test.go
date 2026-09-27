//go:build unit

package service

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

const (
	loginThrottleTestEmail    = "admin@example.com"
	loginThrottleTestPassword = "correct-password"
)

func newLoginThrottleTestService(t *testing.T) *AuthService {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(loginThrottleTestPassword), bcrypt.MinCost)
	require.NoError(t, err)
	return &AuthService{
		userRepo: &userRepoStub{user: &User{
			ID:           1,
			Email:        loginThrottleTestEmail,
			PasswordHash: string(hash),
			Role:         RoleAdmin,
			Status:       StatusActive,
		}},
		cfg: &config.Config{JWT: config.JWTConfig{Secret: strings.Repeat("s", 32), AccessTokenExpireMinutes: 30}},
	}
}

func TestLoginFromIPThrottlesRepeatedFailuresPerEmailAndIP(t *testing.T) {
	svc := newLoginThrottleTestService(t)
	ctx := context.Background()

	for i := 0; i <= loginThrottleFreeFailures; i++ {
		_, _, err := svc.LoginFromIP(ctx, loginThrottleTestEmail, "wrong", "10.0.0.1")
		require.ErrorIs(t, err, ErrInvalidCredentials, "attempt %d", i+1)
	}

	// 冷却期内即使密码正确也直接拒绝，且大小写/空白变体不能绕过。
	_, _, err := svc.LoginFromIP(ctx, loginThrottleTestEmail, loginThrottleTestPassword, "10.0.0.1")
	require.ErrorIs(t, err, ErrLoginThrottled)
	_, _, err = svc.LoginFromIP(ctx, " ADMIN@Example.com ", loginThrottleTestPassword, "10.0.0.1")
	require.ErrorIs(t, err, ErrLoginThrottled)

	// 不是按邮箱的硬锁定：其他 IP 仍可正常登录。
	_, user, err := svc.LoginFromIP(ctx, loginThrottleTestEmail, loginThrottleTestPassword, "10.0.0.2")
	require.NoError(t, err)
	require.Equal(t, int64(1), user.ID)
}

func TestLoginFromIPSuccessResetsFailureCount(t *testing.T) {
	svc := newLoginThrottleTestService(t)
	ctx := context.Background()
	failN := func(n int) {
		for i := 0; i < n; i++ {
			_, _, err := svc.LoginFromIP(ctx, loginThrottleTestEmail, "wrong", "10.0.0.1")
			require.ErrorIs(t, err, ErrInvalidCredentials, "attempt %d", i+1)
		}
	}

	failN(loginThrottleFreeFailures)
	_, _, err := svc.LoginFromIP(ctx, loginThrottleTestEmail, loginThrottleTestPassword, "10.0.0.1")
	require.NoError(t, err)

	// 成功后计数清零：再失败同样次数也不会进入冷却。
	failN(loginThrottleFreeFailures)
	_, _, err = svc.LoginFromIP(ctx, loginThrottleTestEmail, loginThrottleTestPassword, "10.0.0.1")
	require.NoError(t, err)
}

func TestLoginThrottleDelayDoublesCapsAndExpires(t *testing.T) {
	var th loginThrottle
	now := time.Unix(1_700_000_000, 0)

	for i := 0; i < loginThrottleFreeFailures; i++ {
		th.recordFailure("k", now)
	}
	require.False(t, th.blocked("k", now))

	th.recordFailure("k", now) // 第一次超额：冷却 1s
	require.True(t, th.blocked("k", now.Add(999*time.Millisecond)))
	require.False(t, th.blocked("k", now.Add(time.Second)))

	th.recordFailure("k", now) // 第二次超额：冷却翻倍到 2s
	require.True(t, th.blocked("k", now.Add(1999*time.Millisecond)))
	require.False(t, th.blocked("k", now.Add(2*time.Second)))

	for i := 0; i < 100; i++ {
		th.recordFailure("k", now)
	}
	require.True(t, th.blocked("k", now.Add(loginThrottleMaxDelay-time.Millisecond)))
	require.False(t, th.blocked("k", now.Add(loginThrottleMaxDelay)))

	// 超过 TTL 没有新失败则重新计数。
	later := now.Add(loginThrottleFailureTTL + time.Second)
	th.recordFailure("k", later)
	require.False(t, th.blocked("k", later))
}

func TestLoginThrottleCapacityIsBounded(t *testing.T) {
	var th loginThrottle
	now := time.Unix(1_700_000_000, 0)
	for i := 0; i < loginThrottleCapacity; i++ {
		th.recordFailure(strconv.Itoa(i), now)
	}
	th.recordFailure("overflow", now)
	require.Len(t, th.entries, loginThrottleCapacity)
	require.NotContains(t, th.entries, "overflow")

	// 容量满时清理过期条目，新 key 可以重新被跟踪。
	later := now.Add(loginThrottleFailureTTL + time.Second)
	th.recordFailure("fresh", later)
	require.Len(t, th.entries, 1)
	require.Contains(t, th.entries, "fresh")
}

func TestLoginUnknownEmailStillRunsBcrypt(t *testing.T) {
	svc := newLoginThrottleTestService(t)
	ctx := context.Background()

	_, _, _ = svc.Login(ctx, "ghost@example.com", "whatever") // 预热 dummy hash
	start := time.Now()
	_, _, err := svc.Login(ctx, "ghost@example.com", "whatever")
	elapsed := time.Since(start)

	require.ErrorIs(t, err, ErrInvalidCredentials)
	// DefaultCost 的 bcrypt 比较在任何硬件上都远超 5ms；不做 dummy 比较时这条路径只是一次内存查找。
	require.GreaterOrEqual(t, elapsed, 5*time.Millisecond)
}
