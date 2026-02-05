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
REDACTED

type proxyImportResponse struct {
	Code int              `json:"code"`
	Data DataImportResult `json:"data"`
REDACTED

func setupProxyDataRouter() (*gin.Engine, *stubAdminService) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	adminSvc := newStubAdminService()

	h := NewProxyHandler(adminSvc)
	router.GET("/api/v1/admin/proxies/data", h.ExportData)
	router.POST("/api/v1/admin/proxies/data", h.ImportData)

	return router, adminSvc
REDACTED

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
	REDACTED,
		{
			ID:       2,
			Name:     "proxy-b",
			Protocol: "https",
			Host:     "10.0.0.2",
			Port:     443,
			Username: "u",
			Password: "p",
			Status:   service.StatusDisabled,
	REDACTED,
REDACTED

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/proxies/data?protocol=https", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp proxyDataResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, dataType, resp.Data.Type)
	require.Len(t, resp.Data.Proxies, 1)
	require.Len(t, resp.Data.Accounts, 0)
	require.Equal(t, "https", resp.Data.Proxies[0].Protocol)
REDACTED

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
	REDACTED,
REDACTED

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
			REDACTED,
				{
					"proxy_key": "https|10.0.0.2|443|u|p",
					"name":      "proxy-b",
					"protocol":  "https",
					"host":      "10.0.0.2",
					"port":      443,
					"username":  "u",
					"password":  "p",
					"status":    "active",
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

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
		return len(adminSvc.testedProxyIDs) == 2
REDACTED, time.Second, 10*time.Millisecond)
REDACTED
