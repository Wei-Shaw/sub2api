package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAccountHandlerCreateWebImagePreservesTypeAndCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adminSvc := newStubAdminService()
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.POST("/api/v1/admin/accounts", handler.Create)

	payload, err := json.Marshal(map[string]any{
		"name": "web-image-fixture", "platform": "openai", "type": "web-image",
		"credentials": map[string]any{"access_token": "fixture-access", "refresh_token": "fixture-refresh"},
		"concurrency": 1,
	})
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Len(t, adminSvc.createdAccounts, 1)
	created := adminSvc.createdAccounts[0]
	require.Equal(t, service.PlatformOpenAI, created.Platform)
	require.Equal(t, service.AccountTypeWebImage, created.Type)
	require.Equal(t, "fixture-access", created.Credentials["access_token"])
	require.Equal(t, "fixture-refresh", created.Credentials["refresh_token"])
}
