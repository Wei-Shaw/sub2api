//go:build unit

package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// stubExternalAuth 模拟真实 LDAP 插件的契约行为
type stubExternalAuth struct {
	loginFunc func(ctx context.Context, identifier, password string) (*service.User, error)
	syncFunc  func(ctx context.Context) (*service.LDAPSyncResult, error)
	testFunc  func(ctx context.Context) error
}

func (m *stubExternalAuth) Login(ctx context.Context, identifier, password string) (*service.User, error) {
	if m.loginFunc != nil {
		return m.loginFunc(ctx, identifier, password)
	}
	return nil, service.ErrInvalidCredentials
}

func (m *stubExternalAuth) TestConnection(ctx context.Context) error {
	if m.testFunc != nil {
		return m.testFunc(ctx)
	}
	return nil
}

func (m *stubExternalAuth) SyncNow(ctx context.Context) (*service.LDAPSyncResult, error) {
	if m.syncFunc != nil {
		return m.syncFunc(ctx)
	}
	return &service.LDAPSyncResult{}, nil
}

func (m *stubExternalAuth) Start() {}
func (m *stubExternalAuth) Stop()  {}

// stubRefreshTokenCache 模拟 Refresh Token 的缓存存储
type stubRefreshTokenCache struct{}

func (s *stubRefreshTokenCache) StoreRefreshToken(ctx context.Context, tokenHash string, data *service.RefreshTokenData, ttl time.Duration) error {
	return nil
}
func (s *stubRefreshTokenCache) GetRefreshToken(ctx context.Context, tokenHash string) (*service.RefreshTokenData, error) {
	return nil, nil
}
func (s *stubRefreshTokenCache) DeleteRefreshToken(ctx context.Context, tokenHash string) error {
	return nil
}
func (s *stubRefreshTokenCache) DeleteUserRefreshTokens(ctx context.Context, userID int64) error {
	return nil
}
func (s *stubRefreshTokenCache) DeleteTokenFamily(ctx context.Context, familyID string) error {
	return nil
}
func (s *stubRefreshTokenCache) AddToUserTokenSet(ctx context.Context, userID int64, tokenHash string, ttl time.Duration) error {
	return nil
}
func (s *stubRefreshTokenCache) AddToFamilyTokenSet(ctx context.Context, familyID string, tokenHash string, ttl time.Duration) error {
	return nil
}
func (s *stubRefreshTokenCache) GetUserTokenHashes(ctx context.Context, userID int64) ([]string, error) {
	return nil, nil
}
func (s *stubRefreshTokenCache) GetFamilyTokenHashes(ctx context.Context, familyID string) ([]string, error) {
	return nil, nil
}
func (s *stubRefreshTokenCache) IsTokenInFamily(ctx context.Context, familyID string, tokenHash string) (bool, error) {
	return true, nil
}

func TestLDAPLoginContract(t *testing.T) {
	// 1. 设置精简依赖
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:                   "test-secret",
			AccessTokenExpireMinutes: 60,
		},
	}

	// 启用 LDAP 设置
	settingRepo := newStubSettingRepo()
	settingRepo.SetAll(map[string]string{
		service.SettingKeyLDAPEnabled: "true",
	})
	settingService := service.NewSettingService(settingRepo, cfg)

	// 模拟返回 LDAP 映射用户的 Provider
	mockUser := &service.User{
		ID:         100,
		Email:      "ldap_user@example.com",
		Username:   "LDAP User",
		Role:       "user",
		AuthSource: "ldap",
		Status:     "active",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	extAuth := &stubExternalAuth{
		loginFunc: func(ctx context.Context, identifier, password string) (*service.User, error) {
			if identifier == "valid_user" && password == "secret" {
				return mockUser, nil
			}
			return nil, service.ErrInvalidCredentials
		},
	}

	userRepo := &stubUserRepo{
		users: map[int64]*service.User{},
	}

	authService := service.NewAuthService(
		nil, // entClient
		userRepo,
		extAuth,
		nil,                      // redeemRepo
		&stubRefreshTokenCache{}, // refreshTokenCache
		cfg,
		settingService,
		nil, // emailService
		nil, // turnstileService
		nil, // emailQueueService
		nil, // promoService
		nil, // defaultSubAssigner
		nil, // affiliateService
	)

	// 2. 初始化路由
	gin.SetMode(gin.TestMode)
	r := gin.New()
	authHandler := handler.NewAuthHandler(cfg, authService, nil, settingService, nil, nil, nil, nil)

	v1 := r.Group("/api/v1")
	v1.POST("/auth/login", authHandler.Login)

	// 3. 执行契约测试：正确的凭证应返回 200 并包含 auth_source=ldap
	t.Run("Valid LDAP Login returns auth_source", func(t *testing.T) {
		body := map[string]interface{}{
			"email":    "valid_user",
			"password": "secret",
		}
		jsonBody, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		require.Equal(t, float64(0), resp["code"]) // 成功 code 为 0

		data := resp["data"].(map[string]interface{})
		require.NotEmpty(t, data["access_token"])

		user := data["user"].(map[string]interface{})
		require.Equal(t, "ldap_user@example.com", user["email"])
		require.Equal(t, "ldap", user["auth_source"])
	})

	// 4. 执行契约测试：错误的凭证应返回 401
	t.Run("Invalid LDAP Login returns 401", func(t *testing.T) {
		body := map[string]interface{}{
			"email":    "valid_user",
			"password": "wrong",
		}
		jsonBody, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusUnauthorized, w.Code)
	})
}
