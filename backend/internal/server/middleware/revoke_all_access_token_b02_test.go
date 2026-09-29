//go:build unit

package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// b02RevocationCacheStub 是 RefreshTokenCache 的空实现，额外带有按用户的撤销水位存储。
type b02RevocationCacheStub struct {
	mu        sync.Mutex
	revokedAt map[int64]int64
	ttls      map[int64]time.Duration
	getErr    error
}

func b02NewRevocationCacheStub() *b02RevocationCacheStub {
	return &b02RevocationCacheStub{revokedAt: map[int64]int64{}, ttls: map[int64]time.Duration{}}
}

func (s *b02RevocationCacheStub) StoreRefreshToken(context.Context, string, *service.RefreshTokenData, time.Duration) error {
	return nil
}

func (s *b02RevocationCacheStub) GetRefreshToken(context.Context, string) (*service.RefreshTokenData, error) {
	return nil, service.ErrRefreshTokenNotFound
}

func (s *b02RevocationCacheStub) DeleteRefreshToken(context.Context, string) error { return nil }

func (s *b02RevocationCacheStub) DeleteUserRefreshTokens(context.Context, int64) error { return nil }

func (s *b02RevocationCacheStub) DeleteTokenFamily(context.Context, string) error { return nil }

func (s *b02RevocationCacheStub) AddToUserTokenSet(context.Context, int64, string, time.Duration) error {
	return nil
}

func (s *b02RevocationCacheStub) AddToFamilyTokenSet(context.Context, string, string, time.Duration) error {
	return nil
}

func (s *b02RevocationCacheStub) GetUserTokenHashes(context.Context, int64) ([]string, error) {
	return nil, nil
}

func (s *b02RevocationCacheStub) GetFamilyTokenHashes(context.Context, string) ([]string, error) {
	return nil, nil
}

func (s *b02RevocationCacheStub) IsTokenInFamily(context.Context, string, string) (bool, error) {
	return false, nil
}

func (s *b02RevocationCacheStub) SetUserTokensRevokedAt(_ context.Context, userID int64, revokedAtUnix int64, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revokedAt[userID] = revokedAtUnix
	s.ttls[userID] = ttl
	return nil
}

func (s *b02RevocationCacheStub) GetUserTokensRevokedAt(_ context.Context, userID int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.getErr != nil {
		return 0, s.getErr
	}
	return s.revokedAt[userID], nil
}

func b02NewEnv(t *testing.T, user *service.User, admin bool) (*gin.Engine, *service.AuthService, *b02RevocationCacheStub) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{}
	cfg.JWT.Secret = "test-jwt-secret-32bytes-long!!!"
	cfg.JWT.AccessTokenExpireMinutes = 60
	cfg.JWT.ExpireHour = 24

	cache := b02NewRevocationCacheStub()
	userRepo := &stubJWTUserRepo{users: map[int64]*service.User{user.ID: user}}
	authSvc := service.NewAuthService(nil, userRepo, nil, cache, cfg, nil, nil, nil, nil, nil, nil, nil, nil)
	userSvc := service.NewUserService(userRepo, nil, nil, nil)

	r := gin.New()
	if admin {
		r.Use(gin.HandlerFunc(NewAdminAuthMiddleware(authSvc, userSvc, nil, nil)))
	} else {
		r.Use(gin.HandlerFunc(NewJWTAuthMiddleware(authSvc, userSvc, nil, nil)))
	}
	r.GET("/protected", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return r, authSvc, cache
}

func b02Do(r *gin.Engine, token string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	return w
}

func b02RequireRevoked(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	require.Equal(t, http.StatusUnauthorized, w.Code, "body=%s", w.Body.String())
	var body ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "TOKEN_REVOKED", body.Code)
}

func b02User(role string) *service.User {
	return &service.User{
		ID:          1,
		Email:       "b02@example.com",
		Role:        role,
		Status:      service.StatusActive,
		Concurrency: 1,
	}
}

func TestB02RevokeAllUserTokensRejectsExistingAccessToken(t *testing.T) {
	user := b02User(service.RoleUser)
	r, authSvc, cache := b02NewEnv(t, user, false)

	token, err := authSvc.GenerateToken(context.Background(), user)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, b02Do(r, token).Code)

	require.NoError(t, authSvc.RevokeAllUserTokens(context.Background(), user.ID))

	b02RequireRevoked(t, b02Do(r, token))
	// 水位 TTL 至少覆盖 access token 最长有效期
	require.GreaterOrEqual(t, cache.ttls[user.ID], 24*time.Hour)
}

func TestB02AdminAuthRejectsAccessTokenAfterRevokeAll(t *testing.T) {
	admin := b02User(service.RoleAdmin)
	r, authSvc, _ := b02NewEnv(t, admin, true)

	token, err := authSvc.GenerateToken(context.Background(), admin)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, b02Do(r, token).Code)

	require.NoError(t, authSvc.RevokeAllUserTokens(context.Background(), admin.ID))

	b02RequireRevoked(t, b02Do(r, token))
}

func TestB02TokenIssuedAfterWatermarkIsAccepted(t *testing.T) {
	user := b02User(service.RoleUser)
	r, authSvc, cache := b02NewEnv(t, user, false)

	cache.revokedAt[user.ID] = time.Now().Add(-5 * time.Second).Unix()
	token, err := authSvc.GenerateToken(context.Background(), user)
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, b02Do(r, token).Code)
}

func TestB02WatermarkReadErrorFailsOpen(t *testing.T) {
	user := b02User(service.RoleUser)
	r, authSvc, cache := b02NewEnv(t, user, false)

	token, err := authSvc.GenerateToken(context.Background(), user)
	require.NoError(t, err)
	cache.revokedAt[user.ID] = time.Now().Add(time.Minute).Unix()
	cache.getErr = errors.New("redis down")

	require.Equal(t, http.StatusOK, b02Do(r, token).Code)
}
