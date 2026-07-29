//go:build unit

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// newOptionalJWTTestEnv 创建 OptionalJWT 中间件测试环境。
// handler 回写「是否携带 AuthSubject」,便于断言匿名 vs 登录两种路径。
func newOptionalJWTTestEnv(users map[int64]*service.User) (*gin.Engine, *service.AuthService) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{}
	cfg.JWT.Secret = "test-jwt-secret-32bytes-long!!!"
	cfg.JWT.AccessTokenExpireMinutes = 60

	userRepo := &stubJWTUserRepo{users: users}
	authSvc := service.NewAuthService(nil, userRepo, nil, nil, cfg, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	userSvc := service.NewUserService(userRepo, nil, nil, nil)
	mw := NewOptionalJWTAuthMiddleware(authSvc, userSvc, nil, nil)

	r := gin.New()
	r.Use(gin.HandlerFunc(mw))
	r.GET("/plaza", func(c *gin.Context) {
		subject, authed := GetAuthSubjectFromContext(c)
		c.JSON(http.StatusOK, gin.H{"authed": authed, "user_id": subject.UserID})
	})
	return r, authSvc
}

func TestOptionalJWTAuth_NoHeaderPassesAnonymously(t *testing.T) {
	router, _ := newOptionalJWTTestEnv(nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/plaza", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"authed":false`)
}

func TestOptionalJWTAuth_ValidTokenSetsSubject(t *testing.T) {
	user := &service.User{
		ID:           7,
		Email:        "plaza@example.com",
		Role:         "user",
		Status:       service.StatusActive,
		Concurrency:  5,
		TokenVersion: 1,
	}
	router, authSvc := newOptionalJWTTestEnv(map[int64]*service.User{7: user})

	token, err := authSvc.GenerateToken(context.Background(), user)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/plaza", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"authed":true`)
	require.Contains(t, w.Body.String(), `"user_id":7`)
}

func TestOptionalJWTAuth_InvalidTokenRejected401(t *testing.T) {
	// 带了 header 就必须通过严格校验:坏 token 返回 401 而非静默降级为匿名,
	// 前端 401 拦截器会走 refresh-token 重试。
	router, _ := newOptionalJWTTestEnv(nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/plaza", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestOptionalJWTAuth_BlankHeaderTreatedAsAnonymous(t *testing.T) {
	// 空白 header(如 "Authorization: ")按匿名处理,不进入严格校验。
	router, _ := newOptionalJWTTestEnv(nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/plaza", nil)
	req.Header.Set("Authorization", "   ")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"authed":false`)
}
