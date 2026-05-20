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

type dataResponse struct {
	Code int         `json:"code"`
	Data dataPayload `json:"data"`
}

type dataPayload struct {
	Type     string        `json:"type"`
	Version  int           `json:"version"`
	Proxies  []dataProxy   `json:"proxies"`
	Accounts []dataAccount `json:"accounts"`
}

type dataProxy struct {
	ProxyKey string `json:"proxy_key"`
	Name     string `json:"name"`
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	Status   string `json:"status"`
}

type dataAccount struct {
	Name        string         `json:"name"`
	Platform    string         `json:"platform"`
	Type        string         `json:"type"`
	Credentials map[string]any `json:"credentials"`
	Extra       map[string]any `json:"extra"`
	ProxyKey    *string        `json:"proxy_key"`
	Concurrency int            `json:"concurrency"`
	Priority    int            `json:"priority"`
}

func setupAccountDataRouter() (*gin.Engine, *stubAdminService) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	adminSvc := newStubAdminService()

	h := NewAccountHandler(
		adminSvc,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	router.GET("/api/v1/admin/accounts/data", h.ExportData)
	router.POST("/api/v1/admin/accounts/data", h.ImportData)
	return router, adminSvc
}

func TestExportDataIncludesSecrets(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()

	proxyID := int64(11)
	adminSvc.proxies = []service.Proxy{
		{
			ID:       proxyID,
			Name:     "proxy",
			Protocol: "http",
			Host:     "127.0.0.1",
			Port:     8080,
			Username: "user",
			Password: "pass",
			Status:   service.StatusActive,
		},
		{
			ID:       12,
			Name:     "orphan",
			Protocol: "https",
			Host:     "10.0.0.1",
			Port:     443,
			Username: "o",
			Password: "p",
			Status:   service.StatusActive,
		},
	}
	adminSvc.accounts = []service.Account{
		{
			ID:          21,
			Name:        "account",
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeOAuth,
			Credentials: map[string]any{"token": "secret"},
			Extra:       map[string]any{"note": "x"},
			ProxyID:     &proxyID,
			Concurrency: 3,
			Priority:    50,
			Status:      service.StatusDisabled,
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/data", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp dataResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Empty(t, resp.Data.Type)
	require.Equal(t, 0, resp.Data.Version)
	require.Len(t, resp.Data.Proxies, 1)
	require.Equal(t, "pass", resp.Data.Proxies[0].Password)
	require.Len(t, resp.Data.Accounts, 1)
	require.Equal(t, "secret", resp.Data.Accounts[0].Credentials["token"])
}

func TestExportDataWithoutProxies(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()

	proxyID := int64(11)
	adminSvc.proxies = []service.Proxy{
		{
			ID:       proxyID,
			Name:     "proxy",
			Protocol: "http",
			Host:     "127.0.0.1",
			Port:     8080,
			Username: "user",
			Password: "pass",
			Status:   service.StatusActive,
		},
	}
	adminSvc.accounts = []service.Account{
		{
			ID:          21,
			Name:        "account",
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeOAuth,
			Credentials: map[string]any{"token": "secret"},
			ProxyID:     &proxyID,
			Concurrency: 3,
			Priority:    50,
			Status:      service.StatusDisabled,
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/data?include_proxies=false", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp dataResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Len(t, resp.Data.Proxies, 0)
	require.Len(t, resp.Data.Accounts, 1)
	require.Nil(t, resp.Data.Accounts[0].ProxyKey)
}

func TestExportDataPassesAccountFiltersAndSort(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()
	adminSvc.accounts = []service.Account{
		{ID: 1, Name: "acc-1", Status: service.StatusActive},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/admin/accounts/data?platform=openai&type=oauth&status=active&group=12&privacy_mode=blocked&search=keyword&sort_by=priority&sort_order=desc",
		nil,
	)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Equal(t, 1, adminSvc.lastListAccounts.calls)
	require.Equal(t, "openai", adminSvc.lastListAccounts.platform)
	require.Equal(t, "oauth", adminSvc.lastListAccounts.accountType)
	require.Equal(t, "active", adminSvc.lastListAccounts.status)
	require.Equal(t, int64(12), adminSvc.lastListAccounts.groupID)
	require.Equal(t, "blocked", adminSvc.lastListAccounts.privacyMode)
	require.Equal(t, "keyword", adminSvc.lastListAccounts.search)
	require.Equal(t, "priority", adminSvc.lastListAccounts.sortBy)
	require.Equal(t, "desc", adminSvc.lastListAccounts.sortOrder)
}

func TestExportDataSelectedIDsOverrideFilters(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/admin/accounts/data?ids=1,2&platform=openai&search=keyword&sort_by=priority&sort_order=desc",
		nil,
	)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp dataResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Len(t, resp.Data.Accounts, 2)
	require.Equal(t, 0, adminSvc.lastListAccounts.calls)
}

func TestImportDataReusesProxyAndSkipsDefaultGroup(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()

	adminSvc.proxies = []service.Proxy{
		{
			ID:       1,
			Name:     "proxy",
			Protocol: "socks5",
			Host:     "1.2.3.4",
			Port:     1080,
			Username: "u",
			Password: "p",
			Status:   service.StatusActive,
		},
	}

	dataPayload := map[string]any{
		"data": map[string]any{
			"type":    dataType,
			"version": dataVersion,
			"proxies": []map[string]any{
				{
					"proxy_key": "socks5|1.2.3.4|1080|u|p",
					"name":      "proxy",
					"protocol":  "socks5",
					"host":      "1.2.3.4",
					"port":      1080,
					"username":  "u",
					"password":  "p",
					"status":    "active",
				},
			},
			"accounts": []map[string]any{
				{
					"name":        "acc",
					"platform":    service.PlatformOpenAI,
					"type":        service.AccountTypeOAuth,
					"credentials": map[string]any{"token": "x"},
					"proxy_key":   "socks5|1.2.3.4|1080|u|p",
					"concurrency": 3,
					"priority":    50,
				},
			},
		},
		"skip_default_group_bind": true,
	}

	body, _ := json.Marshal(dataPayload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/data", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Len(t, adminSvc.createdProxies, 0)
	require.Len(t, adminSvc.createdAccounts, 1)
	require.True(t, adminSvc.createdAccounts[0].SkipDefaultGroupBind)
}

func TestImportDataAppliesSharedAccountOptions(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()

	proxyID := int64(12)
	batchID := int64(7)
	groupIDs := []int64{2, 3}
	loadFactor := 25
	rateMultiplier := 0.5
	expiresAt := int64(1893456000)
	autoPauseOnExpired := false
	schedulable := true

	dataPayload := map[string]any{
		"data": map[string]any{
			"type":    dataType,
			"version": dataVersion,
			"proxies": []map[string]any{},
			"accounts": []map[string]any{
				{
					"name":            "acc",
					"platform":        service.PlatformOpenAI,
					"type":            service.AccountTypeOAuth,
					"credentials":     map[string]any{"token": "x", "keep": "file"},
					"extra":           map[string]any{"file_extra": true},
					"concurrency":     3,
					"priority":        50,
					"rate_multiplier": 2,
				},
			},
		},
		"skip_default_group_bind":    false,
		"batch_id":                   batchID,
		"group_ids":                  groupIDs,
		"proxy_id":                   proxyID,
		"concurrency":                9,
		"priority":                   4,
		"rate_multiplier":            rateMultiplier,
		"load_factor":                loadFactor,
		"expires_at":                 expiresAt,
		"auto_pause_on_expired":      autoPauseOnExpired,
		"schedulable":                schedulable,
		"credential_extras":          map[string]any{"intercept_warmup_requests": true, "keep": "override"},
		"extra":                      map[string]any{"shared_extra": "yes"},
		"confirm_mixed_channel_risk": true,
	}

	body, _ := json.Marshal(dataPayload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/data", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Len(t, adminSvc.createdAccounts, 1)
	created := adminSvc.createdAccounts[0]
	require.Equal(t, &batchID, created.BatchID)
	require.Equal(t, &proxyID, created.ProxyID)
	require.Equal(t, 9, created.Concurrency)
	require.Equal(t, 4, created.Priority)
	require.Equal(t, &rateMultiplier, created.RateMultiplier)
	require.Equal(t, &loadFactor, created.LoadFactor)
	require.Equal(t, groupIDs, created.GroupIDs)
	require.Equal(t, &expiresAt, created.ExpiresAt)
	require.Equal(t, &autoPauseOnExpired, created.AutoPauseOnExpired)
	require.Equal(t, &schedulable, created.Schedulable)
	require.False(t, created.SkipDefaultGroupBind)
	require.True(t, created.SkipMixedChannelCheck)
	require.Equal(t, "override", created.Credentials["keep"])
	require.Equal(t, true, created.Credentials["intercept_warmup_requests"])
	require.Equal(t, true, created.Extra["file_extra"])
	require.Equal(t, "yes", created.Extra["shared_extra"])
}
