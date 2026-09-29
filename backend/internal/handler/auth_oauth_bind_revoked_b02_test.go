//go:build unit

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// b02HandlerRevocationCacheStub 在 refresh token 缓存桩之上增加按用户的撤销水位。
type b02HandlerRevocationCacheStub struct {
	userHandlerRefreshTokenCacheStub
	revokedAt map[int64]int64
}

func (s *b02HandlerRevocationCacheStub) SetUserTokensRevokedAt(_ context.Context, userID int64, revokedAtUnix int64, _ time.Duration) error {
	s.revokedAt[userID] = revokedAtUnix
	return nil
}

func (s *b02HandlerRevocationCacheStub) GetUserTokensRevokedAt(_ context.Context, userID int64) (int64, error) {
	return s.revokedAt[userID], nil
}

func b02ResolveBindTarget(h *AuthHandler, token string) (*int64, error) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/linuxdo/start?intent=bind_current_user", nil)
	req.AddCookie(&http.Cookie{Name: oauthBindAccessTokenCookieName, Value: url.QueryEscape(token)})
	c.Request = req
	return h.resolveOAuthBindTargetUserID(c)
}

func TestB02OAuthBindTargetRejectsAccessTokenAfterRevokeAll(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &service.User{
		ID:       31,
		Email:    "bind-revoke@example.com",
		Username: "bind-revoke",
		Role:     service.RoleUser,
		Status:   service.StatusActive,
	}
	repo := &userHandlerRepoStub{user: user}
	cache := &b02HandlerRevocationCacheStub{revokedAt: map[int64]int64{}}
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "test-secret", ExpireHour: 1}}
	authService := service.NewAuthService(nil, repo, nil, cache, cfg, nil, nil, nil, nil, nil, nil, nil, nil)
	h := &AuthHandler{authService: authService, userService: service.NewUserService(repo, nil, nil, nil)}

	token, err := authService.GenerateToken(context.Background(), user)
	require.NoError(t, err)

	userID, err := b02ResolveBindTarget(h, token)
	require.NoError(t, err)
	require.NotNil(t, userID)
	require.Equal(t, user.ID, *userID)

	require.NoError(t, authService.RevokeAllUserTokens(context.Background(), user.ID))

	userID, err = b02ResolveBindTarget(h, token)
	require.ErrorIs(t, err, service.ErrInvalidToken)
	require.Nil(t, userID)
}
