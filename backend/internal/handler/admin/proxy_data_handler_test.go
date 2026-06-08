package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type proxyDataResponse struct {
	Code int         `json:"code"`
	Data DataPayload `json:"data"`
}

type proxyImportResponse struct {
	Code int              `json:"code"`
	Data DataImportResult `json:"data"`
}

func setupProxyDataRouter() (*gin.Engine, *stubAdminService) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	adminSvc := newStubAdminService()

	h := NewProxyHandler(adminSvc)
	router.GET("/api/v1/admin/proxies/data", h.ExportData)
	router.POST("/api/v1/admin/proxies/data", h.ImportData)

	return router, adminSvc
}

func TestProxyExportDataRespectsFilters(t *testing.T) {
	router, adminSvc := setupProxyDataRouter()

	adminSvc.proxies = []service.Proxy{
		{
			ID:       1,
			Name:     "proxy-a",
			Protocol: "http",
			Host:     "127.0.0.1",
			Port:     8080,
			Username: "user",
			Password: "pass",
			Status:   service.StatusActive,
		},
		{
			ID:       2,
			Name:     "proxy-b",
			Protocol: "https",
			Host:     "10.0.0.2",
			Port:     443,
			Username: "u",
			Password: "p",
			Status:   service.StatusDisabled,
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/proxies/data?protocol=https", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp proxyDataResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Empty(t, resp.Data.Type)
	require.Equal(t, 0, resp.Data.Version)
	require.Len(t, resp.Data.Proxies, 1)
	require.Len(t, resp.Data.Accounts, 0)
	require.Equal(t, "https", resp.Data.Proxies[0].Protocol)
	require.Equal(t, 1, adminSvc.lastListProxies.calls)
	require.Equal(t, "https", adminSvc.lastListProxies.protocol)
	require.Equal(t, "id", adminSvc.lastListProxies.sortBy)
	require.Equal(t, "desc", adminSvc.lastListProxies.sortOrder)
}

func TestProxyExportDataWithSelectedIDs(t *testing.T) {
	router, adminSvc := setupProxyDataRouter()

	adminSvc.proxies = []service.Proxy{
		{
			ID:       1,
			Name:     "proxy-a",
			Protocol: "http",
			Host:     "127.0.0.1",
			Port:     8080,
			Username: "user",
			Password: "pass",
			Status:   service.StatusActive,
		},
		{
			ID:       2,
			Name:     "proxy-b",
			Protocol: "https",
			Host:     "10.0.0.2",
			Port:     443,
			Username: "u",
			Password: "p",
			Status:   service.StatusDisabled,
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/proxies/data?ids=2", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp proxyDataResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Len(t, resp.Data.Proxies, 1)
	require.Equal(t, "https", resp.Data.Proxies[0].Protocol)
	require.Equal(t, "10.0.0.2", resp.Data.Proxies[0].Host)
	require.Equal(t, 0, adminSvc.lastListProxies.calls)
}

func TestProxyExportDataIncludesFallbackBackupProxy(t *testing.T) {
	router, adminSvc := setupProxyDataRouter()

	backupID := int64(2)
	adminSvc.proxies = []service.Proxy{
		{
			ID:            1,
			Name:          "primary",
			Protocol:      "http",
			Host:          "127.0.0.1",
			Port:          8080,
			Status:        service.StatusActive,
			FallbackMode:  service.FallbackModeProxy,
			BackupProxyID: &backupID,
		},
		{
			ID:       backupID,
			Name:     "backup",
			Protocol: "http",
			Host:     "127.0.0.2",
			Port:     8081,
			Status:   service.StatusActive,
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/proxies/data?ids=1", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp proxyDataResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Data.Proxies, 2)
	require.Equal(t, "primary", resp.Data.Proxies[0].Name)
	require.Equal(t, "backup", resp.Data.Proxies[0].BackupProxyName)
	require.Equal(t, "backup", resp.Data.Proxies[1].Name)
}

func TestProxyExportDataWritesNullExpiresAtForNeverExpiringProxy(t *testing.T) {
	router, adminSvc := setupProxyDataRouter()

	adminSvc.proxies = []service.Proxy{
		{
			ID:       1,
			Name:     "never-expiring",
			Protocol: "http",
			Host:     "127.0.0.1",
			Port:     8080,
			Status:   service.StatusActive,
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/proxies/data?ids=1", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"expires_at":null`)
}

func TestProxyExportDataPassesSortParams(t *testing.T) {
	router, adminSvc := setupProxyDataRouter()

	adminSvc.proxies = []service.Proxy{
		{
			ID:       1,
			Name:     "proxy-a",
			Protocol: "http",
			Host:     "127.0.0.1",
			Port:     8080,
			Username: "user",
			Password: "pass",
			Status:   service.StatusActive,
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/proxies/data?protocol=http&status=active&search=proxy&sort_by=name&sort_order=asc", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Equal(t, 1, adminSvc.lastListProxies.calls)
	require.Equal(t, "http", adminSvc.lastListProxies.protocol)
	require.Equal(t, "active", adminSvc.lastListProxies.status)
	require.Equal(t, "proxy", adminSvc.lastListProxies.search)
	require.Equal(t, "name", adminSvc.lastListProxies.sortBy)
	require.Equal(t, "asc", adminSvc.lastListProxies.sortOrder)
}

func TestProxyExportDataSortByAccountCountUsesAccountCountListing(t *testing.T) {
	router, adminSvc := setupProxyDataRouter()

	adminSvc.proxies = []service.Proxy{
		{
			ID:       1,
			Name:     "proxy-id-1",
			Protocol: "http",
			Host:     "127.0.0.1",
			Port:     8080,
			Status:   service.StatusActive,
		},
		{
			ID:       2,
			Name:     "proxy-id-2",
			Protocol: "http",
			Host:     "127.0.0.2",
			Port:     8081,
			Status:   service.StatusActive,
		},
	}
	adminSvc.proxyCounts = []service.ProxyWithAccountCount{
		{
			Proxy: service.Proxy{
				ID:       2,
				Name:     "proxy-count-high",
				Protocol: "http",
				Host:     "127.0.0.2",
				Port:     8081,
				Status:   service.StatusActive,
			},
			AccountCount: 9,
		},
		{
			Proxy: service.Proxy{
				ID:       1,
				Name:     "proxy-count-low",
				Protocol: "http",
				Host:     "127.0.0.1",
				Port:     8080,
				Status:   service.StatusActive,
			},
			AccountCount: 1,
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/proxies/data?sort_by=account_count&sort_order=desc", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp proxyDataResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Len(t, resp.Data.Proxies, 2)
	require.Equal(t, "proxy-count-high", resp.Data.Proxies[0].Name)
	require.Equal(t, "proxy-count-low", resp.Data.Proxies[1].Name)
	require.Equal(t, 0, adminSvc.lastListProxies.calls)
}

func TestProxyImportDataReusesAndTriggersLatencyProbe(t *testing.T) {
	router, adminSvc := setupProxyDataRouter()

	adminSvc.proxies = []service.Proxy{
		{
			ID:       1,
			Name:     "proxy-a",
			Protocol: "http",
			Host:     "127.0.0.1",
			Port:     8080,
			Username: "user",
			Password: "pass",
			Status:   service.StatusActive,
		},
	}

	payload := map[string]any{
		"data": map[string]any{
			"type":    dataType,
			"version": dataVersion,
			"proxies": []map[string]any{
				{
					"proxy_key": "http|127.0.0.1|8080|user|pass",
					"name":      "proxy-a",
					"protocol":  "http",
					"host":      "127.0.0.1",
					"port":      8080,
					"username":  "user",
					"password":  "pass",
					"status":    "inactive",
				},
				{
					"proxy_key": "https|10.0.0.2|443|u|p",
					"name":      "proxy-b",
					"protocol":  "https",
					"host":      "10.0.0.2",
					"port":      443,
					"username":  "u",
					"password":  "p",
					"status":    "active",
				},
			},
			"accounts": []map[string]any{},
		},
	}

	body, _ := json.Marshal(payload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/proxies/data", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp proxyImportResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, 1, resp.Data.ProxyCreated)
	require.Equal(t, 1, resp.Data.ProxyReused)
	require.Equal(t, 0, resp.Data.ProxyFailed)

	adminSvc.mu.Lock()
	updatedIDs := append([]int64(nil), adminSvc.updatedProxyIDs...)
	adminSvc.mu.Unlock()
	require.Contains(t, updatedIDs, int64(1))

	require.Eventually(t, func() bool {
		adminSvc.mu.Lock()
		defer adminSvc.mu.Unlock()
		return len(adminSvc.testedProxyIDs) == 1
	}, time.Second, 10*time.Millisecond)
}

func TestProxyImportDataReusedSameStatusRestoresExpiryAndFallback(t *testing.T) {
	router, adminSvc := setupProxyDataRouter()

	expiresAt := time.Now().UTC().Add(72 * time.Hour).Unix()
	adminSvc.proxies = []service.Proxy{
		{
			ID:             1,
			Name:           "primary",
			Protocol:       "http",
			Host:           "127.0.0.1",
			Port:           8080,
			Status:         service.StatusActive,
			FallbackMode:   service.FallbackModeNone,
			ExpiryWarnDays: 7,
		},
		{
			ID:       2,
			Name:     "backup",
			Protocol: "http",
			Host:     "127.0.0.2",
			Port:     8081,
			Status:   service.StatusActive,
		},
	}

	payload := map[string]any{
		"data": map[string]any{
			"type":    dataType,
			"version": dataVersion,
			"proxies": []map[string]any{
				{
					"proxy_key":         "http|127.0.0.1|8080||",
					"name":              "primary",
					"protocol":          "http",
					"host":              "127.0.0.1",
					"port":              8080,
					"status":            "active",
					"expires_at":        expiresAt,
					"fallback_mode":     service.FallbackModeProxy,
					"backup_proxy_name": "backup",
					"expiry_warn_days":  21,
				},
			},
			"accounts": []map[string]any{},
		},
	}

	body, _ := json.Marshal(payload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/proxies/data", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Len(t, adminSvc.updatedProxies, 2)
	first := adminSvc.updatedProxies[0]
	require.True(t, first.ExpiresAt.Set)
	require.True(t, first.ExpiryWarnDays.Set)
	require.Equal(t, 21, first.ExpiryWarnDays.Value)
	second := adminSvc.updatedProxies[1]
	require.True(t, second.FallbackMode.Set)
	require.Equal(t, service.FallbackModeProxy, second.FallbackMode.Value)
	require.True(t, second.BackupProxyID.Set)
	require.NotNil(t, second.BackupProxyID.Value)
	require.Equal(t, int64(2), *second.BackupProxyID.Value)
}

func TestProxyImportDataReusedSameStatusClearsExpiresAt(t *testing.T) {
	router, adminSvc := setupProxyDataRouter()

	existingExpiresAt := time.Now().UTC().Add(24 * time.Hour)
	adminSvc.proxies = []service.Proxy{
		{
			ID:             1,
			Name:           "primary",
			Protocol:       "http",
			Host:           "127.0.0.1",
			Port:           8080,
			Status:         service.StatusActive,
			ExpiresAt:      &existingExpiresAt,
			FallbackMode:   service.FallbackModeNone,
			ExpiryWarnDays: 7,
		},
	}

	payload := map[string]any{
		"data": map[string]any{
			"type":    dataType,
			"version": dataVersion,
			"proxies": []map[string]any{
				{
					"proxy_key":  "http|127.0.0.1|8080||",
					"name":       "primary",
					"protocol":   "http",
					"host":       "127.0.0.1",
					"port":       8080,
					"status":     "active",
					"expires_at": nil,
				},
			},
			"accounts": []map[string]any{},
		},
	}

	body, _ := json.Marshal(payload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/proxies/data", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Len(t, adminSvc.updatedProxies, 1)
	update := adminSvc.updatedProxies[0]
	require.True(t, update.ExpiresAt.Set)
	require.Nil(t, update.ExpiresAt.Value)
}

func TestProxyImportDataRejectsInvalidFallbackMode(t *testing.T) {
	router, adminSvc := setupProxyDataRouter()
	adminSvc.proxies = []service.Proxy{}

	payload := map[string]any{
		"data": map[string]any{
			"type":    dataType,
			"version": dataVersion,
			"proxies": []map[string]any{
				{
					"name":          "invalid",
					"protocol":      "http",
					"host":          "127.0.0.1",
					"port":          8080,
					"fallback_mode": "bad-mode",
				},
			},
			"accounts": []map[string]any{},
		},
	}

	body, _ := json.Marshal(payload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/proxies/data", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp proxyImportResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 1, resp.Data.ProxyFailed)
	require.Contains(t, resp.Data.Errors[0].Message, "fallback_mode is invalid")
	require.Len(t, adminSvc.createdProxies, 0)
}

func TestProxyImportDataResolvesForwardBackupReference(t *testing.T) {
	router, adminSvc := setupProxyDataRouter()
	adminSvc.proxies = []service.Proxy{}

	payload := map[string]any{
		"data": map[string]any{
			"type":    dataType,
			"version": dataVersion,
			"proxies": []map[string]any{
				{
					"name":              "primary",
					"protocol":          "http",
					"host":              "127.0.0.1",
					"port":              8080,
					"status":            "active",
					"fallback_mode":     service.FallbackModeProxy,
					"backup_proxy_name": "backup",
				},
				{
					"name":     "backup",
					"protocol": "http",
					"host":     "127.0.0.2",
					"port":     8081,
					"status":   "active",
				},
			},
			"accounts": []map[string]any{},
		},
	}

	body, _ := json.Marshal(payload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/proxies/data", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp proxyImportResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 2, resp.Data.ProxyCreated)
	require.Empty(t, resp.Data.Errors)
	require.Len(t, adminSvc.updatedProxies, 1)
	update := adminSvc.updatedProxies[0]
	require.Equal(t, service.FallbackModeProxy, update.FallbackMode.Value)
	require.NotNil(t, update.BackupProxyID.Value)
	require.Equal(t, int64(401), *update.BackupProxyID.Value)
}
