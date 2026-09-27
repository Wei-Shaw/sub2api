//go:build unit

package service

import (
	"context"
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
