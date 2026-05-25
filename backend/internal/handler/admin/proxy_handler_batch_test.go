package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProxyHandlerBatchCreateUsesNamePrefix(t *testing.T) {
	router, adminSvc := setupAdminRouter()

	body, _ := json.Marshal(map[string]any{
		"name_prefix": "tokyo",
		"proxies": []map[string]any{
			{"protocol": "http", "host": "10.0.0.1", "port": 8080},
			{"protocol": "https", "host": "10.0.0.2", "port": 443},
		},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/proxies/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	adminSvc.mu.Lock()
	defer adminSvc.mu.Unlock()
	require.Len(t, adminSvc.createdProxies, 2)
	require.Equal(t, "tokyo-001", adminSvc.createdProxies[0].Name)
	require.Equal(t, "tokyo-002", adminSvc.createdProxies[1].Name)
}

func TestProxyHandlerBatchCreateFallsBackWithoutNamePrefix(t *testing.T) {
	router, adminSvc := setupAdminRouter()

	body, _ := json.Marshal(map[string]any{
		"proxies": []map[string]any{
			{"protocol": "http", "host": "10.0.0.1", "port": 8080},
		},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/proxies/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	adminSvc.mu.Lock()
	defer adminSvc.mu.Unlock()
	require.Len(t, adminSvc.createdProxies, 1)
	require.Equal(t, "default", adminSvc.createdProxies[0].Name)
}

func TestProxyHandlerBatchCreateKeepsNamesSequentialForCreatedItems(t *testing.T) {
	router, adminSvc := setupAdminRouter()

	adminSvc.mu.Lock()
	adminSvc.existingProxyKeys["10.0.0.1|8080||"] = true
	adminSvc.mu.Unlock()

	body, _ := json.Marshal(map[string]any{
		"name_prefix": "tokyo",
		"proxies": []map[string]any{
			{"protocol": "http", "host": "10.0.0.1", "port": 8080},
			{"protocol": "http", "host": "10.0.0.2", "port": 8080},
			{"protocol": "http", "host": "10.0.0.3", "port": 8080},
		},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/proxies/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	adminSvc.mu.Lock()
	defer adminSvc.mu.Unlock()
	require.Len(t, adminSvc.createdProxies, 2)
	require.Equal(t, "tokyo-001", adminSvc.createdProxies[0].Name)
	require.Equal(t, "tokyo-002", adminSvc.createdProxies[1].Name)
}
