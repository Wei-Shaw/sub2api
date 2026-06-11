package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setupAccountListRouter() (*gin.Engine, *stubAdminService) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	adminSvc := newStubAdminService()
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router.GET("/api/v1/admin/accounts", handler.List)
	return router, adminSvc
}

func setupUsageViewerAccountRouter() (*gin.Engine, *stubAdminService) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	adminSvc := newStubAdminService()
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1})
		c.Set(string(middleware.ContextKeyUserRole), service.RoleUsageViewer)
		c.Next()
	})
	router.GET("/api/v1/admin/accounts", handler.List)
	router.GET("/api/v1/admin/accounts/:id", handler.GetByID)
	return router, adminSvc
}

func TestAccountHandlerListIncludesCreatedAt(t *testing.T) {
	router, adminSvc := setupAccountListRouter()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts?page=1&page_size=20&sort_by=created_at&sort_order=desc", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "created_at", adminSvc.lastListAccounts.sortBy)

	var payload struct {
		Data struct {
			Items []struct {
				ID        int64  `json:"id"`
				CreatedAt string `json:"created_at"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	require.Len(t, payload.Data.Items, 1)

	createdAt := payload.Data.Items[0].CreatedAt
	require.NotEmpty(t, createdAt)
	require.True(t, strings.HasSuffix(createdAt, "Z"), "created_at should be serialized as UTC")
	parsed, err := time.Parse(time.RFC3339Nano, createdAt)
	require.NoError(t, err)
	_, offset := parsed.Zone()
	require.Equal(t, 0, offset)
}

func TestUsageViewerAccountListOmitsAdminOnlyFields(t *testing.T) {
	router, adminSvc := setupUsageViewerAccountRouter()
	notes := "internal notes"
	proxyID := int64(9)
	adminSvc.users[0].Role = service.RoleUsageViewer
	adminSvc.users[0].AllowedAccounts = []int64{3}
	adminSvc.accounts = []service.Account{{
		ID:           3,
		Name:         "scoped",
		Notes:        &notes,
		Platform:     service.PlatformAnthropic,
		Type:         service.AccountTypeOAuth,
		Credentials:  map[string]any{"base_url": "https://upstream.example", "access_token": "secret"},
		Extra:        map[string]any{"privacy_mode": "training_off", "quota_limit": 10.0},
		ProxyID:      &proxyID,
		Status:       service.StatusActive,
		ErrorMessage: "upstream failed",
		GroupIDs:     []int64{7},
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	raw := rec.Body.String()
	require.NotContains(t, raw, "credentials")
	require.NotContains(t, raw, "credentials_status")
	require.NotContains(t, raw, "extra")
	require.NotContains(t, raw, "notes")
	require.NotContains(t, raw, "error_message")
	require.NotContains(t, raw, "proxy_id")
	require.NotContains(t, raw, "account_groups")
	require.NotContains(t, raw, "group_ids")
	require.NotContains(t, raw, "privacy_mode")
	require.NotContains(t, raw, "upstream.example")
	require.NotContains(t, raw, "upstream failed")
	require.Contains(t, raw, `"name":"scoped"`)
}

func TestUsageViewerAccountDetailOmitsAdminOnlyFields(t *testing.T) {
	router, adminSvc := setupUsageViewerAccountRouter()
	notes := "detail notes"
	proxyID := int64(11)
	adminSvc.users[0].Role = service.RoleUsageViewer
	adminSvc.users[0].AllowedAccounts = []int64{3}
	adminSvc.accounts = []service.Account{{
		ID:           3,
		Name:         "detail",
		Notes:        &notes,
		Platform:     service.PlatformOpenAI,
		Type:         service.AccountTypeAPIKey,
		Credentials:  map[string]any{"api_key": "sk-secret", "base_url": "https://api.example"},
		Extra:        map[string]any{"privacy_mode": "blocked"},
		ProxyID:      &proxyID,
		Status:       service.StatusActive,
		ErrorMessage: "last error",
		GroupIDs:     []int64{8},
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/3", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	raw := rec.Body.String()
	require.NotContains(t, raw, "credentials")
	require.NotContains(t, raw, "extra")
	require.NotContains(t, raw, "notes")
	require.NotContains(t, raw, "error_message")
	require.NotContains(t, raw, "proxy_id")
	require.NotContains(t, raw, "group_ids")
	require.NotContains(t, raw, "privacy_mode")
	require.NotContains(t, raw, "sk-secret")
	require.NotContains(t, raw, "last error")
	require.Contains(t, raw, `"name":"detail"`)
}
