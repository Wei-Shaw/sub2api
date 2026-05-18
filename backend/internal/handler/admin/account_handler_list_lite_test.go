package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setupAccountListRouter(adminSvc *stubAdminService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(gin.Recovery())
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router.GET("/api/v1/admin/accounts", handler.List)
	return router
}

func TestAccountHandlerList_LiteSkipsHeavyRuntimeMetrics(t *testing.T) {
	now := time.Now().UTC()
	adminSvc := newStubAdminService()
	adminSvc.accounts = []service.Account{{
		ID:          9,
		Name:        "heavy-account",
		Platform:    service.PlatformAnthropic,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 8,
		Extra: map[string]any{
			"window_cost_limit":            25.0,
			"max_sessions":                 4,
			"session_idle_timeout_minutes": 15,
			"base_rpm":                     30,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}}

	router := setupAccountListRouter(adminSvc)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts?lite=1", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Items []map[string]any `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.True(t, body.Success)
	require.Len(t, body.Data.Items, 1)
	require.NotContains(t, body.Data.Items[0], "current_concurrency")
	require.NotContains(t, body.Data.Items[0], "current_window_cost")
	require.NotContains(t, body.Data.Items[0], "active_sessions")
	require.NotContains(t, body.Data.Items[0], "current_rpm")
}
