package admin

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type dataResponse struct {
	Code int         `json:"code"`
	Data dataPayload `json:"data"`
}

type dataPayload struct {
	Type           string        `json:"type"`
	Version        int           `json:"version"`
	Proxies        []dataProxy   `json:"proxies"`
	Accounts       []dataAccount `json:"accounts"`
	SkippedShadows int           `json:"skipped_shadows"`
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

func setupAccountDataRouterWithModelSync(upstream service.HTTPUpstream) (*gin.Engine, *stubAdminService) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	adminSvc := newStubAdminService()
	accountTestSvc := service.NewAccountTestService(
		nil,
		nil,
		nil,
		nil,
		upstream,
		&config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
		nil,
	)

	h := NewAccountHandler(
		adminSvc,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		accountTestSvc,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

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

// TestExportDataExcludesSparkShadow 验证外审第5轮 P1/P2:导出时排除 spark 影子账号
// (影子无凭据、导入侧强制 credentials 非空,混入会产出无法还原的坏备份),并透出跳过计数。
func TestExportDataExcludesSparkShadow(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()

	parentID := int64(21)
	adminSvc.accounts = []service.Account{
		{
			ID:          parentID,
			Name:        "mother",
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeOAuth,
			Credentials: map[string]any{"token": "secret"},
			Status:      service.StatusActive,
		},
		{
			ID:              22,
			Name:            "mother (Spark)",
			Platform:        service.PlatformOpenAI,
			Type:            service.AccountTypeOAuth,
			Credentials:     map[string]any{}, // 影子恒空凭据
			ParentAccountID: &parentID,        // 影子标记
			QuotaDimension:  service.QuotaDimensionSpark,
			Status:          service.StatusActive,
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/data?include_proxies=false", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp dataResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Len(t, resp.Data.Accounts, 1, "影子应被排除,仅导出母账号")
	require.Equal(t, "mother", resp.Data.Accounts[0].Name)
	require.Equal(t, 1, resp.Data.SkippedShadows, "跳过的影子数量应透出")
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

func TestImportDataDefaultsImportedAccountsUnschedulable(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()

	dataPayload := map[string]any{
		"data": map[string]any{
			"type":    dataType,
			"version": dataVersion,
			"proxies": []map[string]any{},
			"accounts": []map[string]any{
				{
					"name":        "acc",
					"platform":    service.PlatformOpenAI,
					"type":        service.AccountTypeOAuth,
					"credentials": map[string]any{"token": "x"},
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

	require.Len(t, adminSvc.createdAccounts, 1)
	require.NotNil(t, adminSvc.createdAccounts[0].Schedulable)
	require.False(t, *adminSvc.createdAccounts[0].Schedulable)
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

func TestImportDataAcceptsCockpitAccountTransferBundle(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()

	dataPayload := map[string]any{
		"data": map[string]any{
			"schema":      cockpitAccountTransferSchema,
			"version":     1,
			"exported_at": "2026-05-23T00:00:00Z",
			"platforms": map[string]any{
				"codex": map[string]any{
					"account_count": 2,
					"exported_data": []map[string]any{
						{
							"id":              "codex-oauth-1",
							"email":           "codex@example.com",
							"auth_mode":       "oauth",
							"user_id":         "user-1",
							"account_id":      "account-1",
							"organization_id": "org-1",
							"plan_type":       "plus",
							"tokens": map[string]any{
								"access_token":  "codex-at",
								"refresh_token": "codex-rt",
								"id_token":      "codex-id",
							},
						},
						{
							"id":             "codex-key-1",
							"email":          "key@example.com",
							"auth_mode":      "apikey",
							"openai_api_key": "sk-codex",
							"api_base_url":   "https://openai.example.com",
						},
					},
				},
				"gemini": map[string]any{
					"account_count": 1,
					"exported_data": []map[string]any{
						{
							"id":                 "gemini-1",
							"email":              "gemini@example.com",
							"access_token":       "gemini-at",
							"refresh_token":      "gemini-rt",
							"id_token":           "gemini-id",
							"token_type":         "Bearer",
							"scope":              "scope-a",
							"expiry_date":        int64(1893456000000),
							"selected_auth_type": "oauth-personal",
							"project_id":         "gemini-project",
							"tier_id":            "google_ai_pro",
						},
					},
				},
				"antigravity": map[string]any{
					"account_count": 1,
					"exported_data": []map[string]any{
						{
							"id":    "ag-1",
							"email": "ag@example.com",
							"token": map[string]any{
								"access_token":     "ag-at",
								"refresh_token":    "ag-rt",
								"token_type":       "Bearer",
								"project_id":       "ag-project",
								"expiry_timestamp": int64(1893456000),
							},
						},
					},
				},
				"windsurf": map[string]any{
					"account_count": 1,
					"exported_data": []map[string]any{{"id": "windsurf-1", "email": "windsurf@example.com"}},
				},
			},
		},
		"batch_id": int64(7),
	}

	body, _ := json.Marshal(dataPayload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/data", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Code int              `json:"code"`
		Data DataImportResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, 4, resp.Data.AccountCreated)
	require.Equal(t, 1, resp.Data.AccountFailed)
	require.Len(t, resp.Data.Errors, 1)
	require.Contains(t, resp.Data.Errors[0].Message, "no matching sub2api upstream platform")

	require.Len(t, adminSvc.createdAccounts, 4)

	codexOAuth := requireCreatedAccount(t, adminSvc, "codex@example.com")
	require.Equal(t, service.PlatformOpenAI, codexOAuth.Platform)
	require.Equal(t, service.AccountTypeOAuth, codexOAuth.Type)
	require.Equal(t, "codex-at", codexOAuth.Credentials["access_token"])
	require.Equal(t, "codex-rt", codexOAuth.Credentials["refresh_token"])
	require.Equal(t, "codex-id", codexOAuth.Credentials["id_token"])
	require.Equal(t, openai.ClientID, codexOAuth.Credentials["client_id"])
	require.Equal(t, "account-1", codexOAuth.Credentials["chatgpt_account_id"])
	require.Equal(t, "user-1", codexOAuth.Credentials["chatgpt_user_id"])
	require.Equal(t, "org-1", codexOAuth.Credentials["organization_id"])
	require.Equal(t, "plus", codexOAuth.Credentials["plan_type"])
	require.Equal(t, "cockpit-tools", codexOAuth.Extra["import_source"])
	require.Equal(t, "codex", codexOAuth.Extra["source_platform"])

	codexAPIKey := requireCreatedAccount(t, adminSvc, "key@example.com")
	require.Equal(t, service.PlatformOpenAI, codexAPIKey.Platform)
	require.Equal(t, service.AccountTypeAPIKey, codexAPIKey.Type)
	require.Equal(t, "sk-codex", codexAPIKey.Credentials["api_key"])
	require.Equal(t, "https://openai.example.com", codexAPIKey.Credentials["base_url"])

	gemini := requireCreatedAccount(t, adminSvc, "gemini@example.com")
	require.Equal(t, service.PlatformGemini, gemini.Platform)
	require.Equal(t, service.AccountTypeOAuth, gemini.Type)
	require.Equal(t, "gemini-at", gemini.Credentials["access_token"])
	require.Equal(t, "gemini-rt", gemini.Credentials["refresh_token"])
	require.Equal(t, "google_one", gemini.Credentials["oauth_type"])
	require.Equal(t, "gemini-project", gemini.Credentials["project_id"])
	require.Equal(t, "google_ai_pro", gemini.Credentials["tier_id"])
	require.Equal(t, "2030-01-01T00:00:00Z", gemini.Credentials["expires_at"])

	antigravity := requireCreatedAccount(t, adminSvc, "ag@example.com")
	require.Equal(t, service.PlatformAntigravity, antigravity.Platform)
	require.Equal(t, service.AccountTypeOAuth, antigravity.Type)
	require.Equal(t, "ag-at", antigravity.Credentials["access_token"])
	require.Equal(t, "ag-rt", antigravity.Credentials["refresh_token"])
	require.Equal(t, "ag-project", antigravity.Credentials["project_id"])
	require.Equal(t, "2030-01-01T00:00:00Z", antigravity.Credentials["expires_at"])
}

func TestImportDataSkipsDuplicateExistingOAuthCredential(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()
	adminSvc.accounts = []service.Account{
		{
			ID:       42,
			Name:     "existing@example.com",
			Platform: service.PlatformOpenAI,
			Type:     service.AccountTypeOAuth,
			Credentials: map[string]any{
				"access_token":  "existing-at",
				"refresh_token": "same-refresh-token",
			},
		},
	}

	dataPayload := map[string]any{
		"data": map[string]any{
			"type":    dataType,
			"version": dataVersion,
			"proxies": []map[string]any{},
			"accounts": []map[string]any{
				{
					"name":     "duplicate@example.com",
					"platform": service.PlatformOpenAI,
					"type":     service.AccountTypeOAuth,
					"credentials": map[string]any{
						"access_token":  "new-at",
						"refresh_token": "same-refresh-token",
					},
					"concurrency": 3,
					"priority":    50,
				},
			},
		},
	}

	body, _ := json.Marshal(dataPayload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/data", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Code int              `json:"code"`
		Data DataImportResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, 0, resp.Data.AccountCreated)
	require.Equal(t, 1, resp.Data.AccountFailed)
	require.Len(t, adminSvc.createdAccounts, 0)
	require.Len(t, resp.Data.Errors, 1)
	require.Contains(t, resp.Data.Errors[0].Message, "duplicate OAuth refresh_token already exists")
	require.Contains(t, resp.Data.Errors[0].Message, "account #42")
}

func TestImportDataSkipsDuplicateOAuthCredentialInPayload(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()

	dataPayload := map[string]any{
		"data": map[string]any{
			"type":    dataType,
			"version": dataVersion,
			"proxies": []map[string]any{},
			"accounts": []map[string]any{
				{
					"name":     "first@example.com",
					"platform": service.PlatformGemini,
					"type":     service.AccountTypeOAuth,
					"credentials": map[string]any{
						"access_token":  "gemini-at-1",
						"refresh_token": "same-gemini-rt",
					},
					"concurrency": 3,
					"priority":    50,
				},
				{
					"name":     "second@example.com",
					"platform": service.PlatformGemini,
					"type":     service.AccountTypeOAuth,
					"credentials": map[string]any{
						"access_token":  "gemini-at-2",
						"refresh_token": "same-gemini-rt",
					},
					"concurrency": 3,
					"priority":    50,
				},
			},
		},
	}

	body, _ := json.Marshal(dataPayload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/data", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Code int              `json:"code"`
		Data DataImportResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, 1, resp.Data.AccountCreated)
	require.Equal(t, 1, resp.Data.AccountFailed)
	require.Len(t, adminSvc.createdAccounts, 1)
	require.Equal(t, "first@example.com", adminSvc.createdAccounts[0].Name)
	require.Len(t, resp.Data.Errors, 1)
	require.Contains(t, resp.Data.Errors[0].Message, "duplicate OAuth refresh_token in this import payload")
	require.Contains(t, resp.Data.Errors[0].Message, "first@example.com")
}

func TestImportDataWarnsAccessTokenOnlyOAuthAccount(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()

	dataPayload := map[string]any{
		"data": map[string]any{
			"type":    dataType,
			"version": dataVersion,
			"proxies": []map[string]any{},
			"accounts": []map[string]any{
				{
					"name":     "at-only@example.com",
					"platform": service.PlatformOpenAI,
					"type":     service.AccountTypeOAuth,
					"credentials": map[string]any{
						"access_token": "only-at",
					},
					"concurrency": 3,
					"priority":    50,
				},
			},
		},
	}

	body, _ := json.Marshal(dataPayload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/data", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Code int              `json:"code"`
		Data DataImportResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, 1, resp.Data.AccountCreated)
	require.Equal(t, 0, resp.Data.AccountFailed)
	require.Len(t, adminSvc.createdAccounts, 1)
	require.Len(t, resp.Data.Errors, 1)
	require.Equal(t, "account_warning", resp.Data.Errors[0].Kind)
	require.Contains(t, resp.Data.Errors[0].Message, "no refresh_token")
}

func TestImportDataAcceptsCockpitDataTransferBundle(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()

	dataPayload := map[string]any{
		"data": map[string]any{
			"schema":      cockpitDataTransferSchema,
			"version":     1,
			"exported_at": "2026-05-23T00:00:00Z",
			"sections": map[string]any{
				"accounts": true,
				"config":   false,
			},
			"accounts": map[string]any{
				"schema":      cockpitAccountTransferSchema,
				"version":     1,
				"exported_at": "2026-05-23T00:00:00Z",
				"platforms": map[string]any{
					"codex": []map[string]any{
						{
							"email":          "bundle-key@example.com",
							"auth_mode":      "apikey",
							"openai_api_key": "sk-bundle",
						},
					},
				},
			},
		},
		"batch_id": int64(7),
	}

	body, _ := json.Marshal(dataPayload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/data", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Code int              `json:"code"`
		Data DataImportResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, 1, resp.Data.AccountCreated)
	require.Equal(t, 0, resp.Data.AccountFailed)

	created := requireCreatedAccount(t, adminSvc, "bundle-key@example.com")
	require.Equal(t, service.PlatformOpenAI, created.Platform)
	require.Equal(t, service.AccountTypeAPIKey, created.Type)
	require.Equal(t, "sk-bundle", created.Credentials["api_key"])
	require.Equal(t, "https://api.openai.com", created.Credentials["base_url"])
}

func TestConvertCockpitCodexOAuthPreservesTokenExpiry(t *testing.T) {
	item := map[string]any{
		"email":     "codex-oauth@example.com",
		"auth_mode": "oauth",
		"tokens": map[string]any{
			"access_token":     "codex-at",
			"refresh_token":    "codex-rt",
			"expiry_timestamp": int64(1893456000),
		},
	}

	account, err := convertCockpitCodexAccount(item, 1)
	require.NoError(t, err)
	require.Equal(t, service.PlatformOpenAI, account.Platform)
	require.Equal(t, service.AccountTypeOAuth, account.Type)
	require.Equal(t, "codex-rt", account.Credentials["refresh_token"])
	require.Equal(t, openai.ClientID, account.Credentials["client_id"])
	require.Equal(t, "2030-01-01T00:00:00Z", account.Credentials["expires_at"])
	require.Nil(t, account.ExpiresAt)
	require.Nil(t, account.AutoPauseOnExpired)
}

func TestConvertCockpitCodexOAuthPreservesImportedClientID(t *testing.T) {
	item := map[string]any{
		"email":     "codex-oauth@example.com",
		"auth_mode": "oauth",
		"tokens": map[string]any{
			"access_token":  "codex-at",
			"refresh_token": "codex-rt",
			"client_id":     "external-client-id",
		},
	}

	account, err := convertCockpitCodexAccount(item, 1)
	require.NoError(t, err)
	require.Equal(t, "external-client-id", account.Credentials["client_id"])
}

func TestConvertCockpitCodexOAuthWithoutRefreshTokenAutoPausesAtExpiry(t *testing.T) {
	item := map[string]any{
		"email":     "codex-at-only@example.com",
		"auth_mode": "oauth",
		"tokens": map[string]any{
			"access_token": "codex-at",
			"expires_at":   int64(1893456000),
		},
	}

	account, err := convertCockpitCodexAccount(item, 1)
	require.NoError(t, err)
	require.Equal(t, "2030-01-01T00:00:00Z", account.Credentials["expires_at"])
	require.NotNil(t, account.ExpiresAt)
	require.Equal(t, int64(1893456000), *account.ExpiresAt)
	require.NotNil(t, account.AutoPauseOnExpired)
	require.True(t, *account.AutoPauseOnExpired)
	require.NotContains(t, account.Credentials, "refresh_token")
	require.NotContains(t, account.Credentials, "client_id")
}

func TestConvertCockpitGeminiAPIKeyAccount(t *testing.T) {
	item := map[string]any{
		"email":          "gemini-key@example.com",
		"auth_mode":      "apikey",
		"gemini_api_key": "gemini-key",
		"api_base_url":   "https://generativelanguage.googleapis.com/v1beta/openai",
	}

	account, err := convertCockpitGeminiAccount(item, 1)
	require.NoError(t, err)
	require.Equal(t, service.PlatformGemini, account.Platform)
	require.Equal(t, service.AccountTypeAPIKey, account.Type)
	require.Equal(t, "gemini-key", account.Credentials["api_key"])
	require.Equal(t, "https://generativelanguage.googleapis.com/v1beta/openai", account.Credentials["base_url"])
	require.Equal(t, service.GeminiTierAIStudioFree, account.Credentials["tier_id"])
	require.NotContains(t, account.Credentials, "upstream_type")
}

func TestConvertCockpitGeminiAPIKeyRelayAccount(t *testing.T) {
	item := map[string]any{
		"email":        "gemini-relay@example.com",
		"api_key":      "relay-key",
		"api_base_url": "https://relay.example.com",
	}

	account, err := convertCockpitGeminiAccount(item, 1)
	require.NoError(t, err)
	require.Equal(t, service.PlatformGemini, account.Platform)
	require.Equal(t, service.AccountTypeAPIKey, account.Type)
	require.Equal(t, "relay-key", account.Credentials["api_key"])
	require.Equal(t, "https://relay.example.com", account.Credentials["base_url"])
	require.Equal(t, service.GeminiUpstreamCompatibleRelay, account.Credentials["tier_id"])
	require.Equal(t, service.GeminiUpstreamCompatibleRelay, account.Credentials["upstream_type"])
}

func TestImportDataAutoDetectsModelsIntoModelMapping(t *testing.T) {
	upstream := &syncUpstreamHTTPUpstream{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":"gpt-5.4"},{"id":"gpt-image-2"}]}`)),
	}}
	router, adminSvc := setupAccountDataRouterWithModelSync(upstream)

	dataPayload := map[string]any{
		"data": map[string]any{
			"type":    dataType,
			"version": dataVersion,
			"proxies": []map[string]any{},
			"accounts": []map[string]any{
				{
					"name":     "openai-key",
					"platform": service.PlatformOpenAI,
					"type":     service.AccountTypeAPIKey,
					"credentials": map[string]any{
						"api_key":  "sk-test",
						"base_url": "https://openai.example.com/v1",
					},
					"concurrency": 3,
					"priority":    50,
				},
			},
		},
		"auto_detect_models": true,
	}

	body, _ := json.Marshal(dataPayload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/data", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Code int              `json:"code"`
		Data DataImportResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, 1, resp.Data.AccountCreated)
	require.Equal(t, 1, resp.Data.ModelSyncSucceeded)
	require.Equal(t, 0, resp.Data.ModelSyncFailed)

	require.Len(t, adminSvc.updatedAccounts, 1)
	require.Equal(t, int64(300), adminSvc.updatedAccountIDs[0])
	modelMapping, ok := adminSvc.updatedAccounts[0].Credentials["model_mapping"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "gpt-5.4", modelMapping["gpt-5.4"])
	require.Equal(t, "gpt-image-2", modelMapping["gpt-image-2"])
}

func TestImportDataAutoDetectsGeminiRelayModelsViaOpenAICompatibleEndpoint(t *testing.T) {
	upstream := &syncUpstreamHTTPUpstream{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"data":["claude-opus-4-6","gpt-image-2"]}`)),
	}}
	router, adminSvc := setupAccountDataRouterWithModelSync(upstream)

	dataPayload := map[string]any{
		"data": map[string]any{
			"type":    dataType,
			"version": dataVersion,
			"proxies": []map[string]any{},
			"accounts": []map[string]any{
				{
					"name":     "gemini-relay-key",
					"platform": service.PlatformGemini,
					"type":     service.AccountTypeOAuth,
					"credentials": map[string]any{
						"api_key":  "sk-test",
						"base_url": "https://iacc.cc",
					},
					"concurrency": 3,
					"priority":    50,
				},
			},
		},
		"auto_detect_models": true,
	}

	body, _ := json.Marshal(dataPayload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/data", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Code int              `json:"code"`
		Data DataImportResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, 1, resp.Data.AccountCreated)
	require.Equal(t, 1, resp.Data.ModelSyncSucceeded)
	require.Equal(t, 0, resp.Data.ModelSyncFailed)

	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "https://iacc.cc/v1/models", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer sk-test", upstream.lastReq.Header.Get("Authorization"))
	require.Empty(t, upstream.lastReq.Header.Get("x-goog-api-key"))

	created := requireCreatedAccount(t, adminSvc, "gemini-relay-key")
	require.Equal(t, service.AccountTypeAPIKey, created.Type)
	require.Equal(t, service.GeminiUpstreamCompatibleRelay, created.Credentials["tier_id"])
	require.Equal(t, service.GeminiUpstreamCompatibleRelay, created.Credentials["upstream_type"])

	require.Len(t, adminSvc.updatedAccounts, 1)
	modelMapping, ok := adminSvc.updatedAccounts[0].Credentials["model_mapping"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "claude-opus-4-6", modelMapping["claude-opus-4-6"])
	require.Equal(t, "gpt-image-2", modelMapping["gpt-image-2"])
}

func TestImportDataAutoDetectsGeminiRelayModelsWithOpenAIClientAliases(t *testing.T) {
	upstream := &syncUpstreamHTTPUpstream{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":"gemini-3.1-pro"},{"id":"gpt-image-2"}]}`)),
	}}
	router, adminSvc := setupAccountDataRouterWithModelSync(upstream)

	dataPayload := map[string]any{
		"data": map[string]any{
			"type":    dataType,
			"version": dataVersion,
			"proxies": []map[string]any{},
			"accounts": []map[string]any{
				{
					"name":     "gemini-relay-openai-client",
					"platform": service.PlatformGemini,
					"type":     service.AccountTypeOAuth,
					"credentials": map[string]any{
						"apiKey":  "sk-test",
						"baseURL": "https://iacc.cc/v1/models",
					},
					"concurrency": 3,
					"priority":    50,
				},
			},
		},
		"auto_detect_models": true,
	}

	body, _ := json.Marshal(dataPayload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/data", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Code int              `json:"code"`
		Data DataImportResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, 1, resp.Data.AccountCreated)
	require.Equal(t, 1, resp.Data.ModelSyncSucceeded)
	require.Equal(t, 0, resp.Data.ModelSyncFailed)

	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "https://iacc.cc/v1/models", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer sk-test", upstream.lastReq.Header.Get("Authorization"))
	require.Empty(t, upstream.lastReq.Header.Get("x-goog-api-key"))

	created := requireCreatedAccount(t, adminSvc, "gemini-relay-openai-client")
	require.Equal(t, service.AccountTypeAPIKey, created.Type)
	require.Equal(t, "sk-test", created.Credentials["api_key"])
	require.Equal(t, "https://iacc.cc/v1", created.Credentials["base_url"])
	require.Equal(t, service.GeminiUpstreamCompatibleRelay, created.Credentials["tier_id"])
	require.Equal(t, service.GeminiUpstreamCompatibleRelay, created.Credentials["upstream_type"])

	require.Len(t, adminSvc.updatedAccounts, 1)
	modelMapping, ok := adminSvc.updatedAccounts[0].Credentials["model_mapping"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "gemini-3.1-pro", modelMapping["gemini-3.1-pro"])
	require.Equal(t, "gpt-image-2", modelMapping["gpt-image-2"])
}

func requireCreatedAccount(t *testing.T, adminSvc *stubAdminService, name string) *service.CreateAccountInput {
	t.Helper()
	for _, account := range adminSvc.createdAccounts {
		if account.Name == name {
			return account
		}
	}
	require.Failf(t, "created account not found", "name=%s", name)
	return nil
}
