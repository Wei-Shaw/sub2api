package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type dataResponse struct {
	Code int         `json:"code"`
	Data dataPayload `json:"data"`
}

type dataErrorResponse struct {
	Code     int               `json:"code"`
	Message  string            `json:"message"`
	Reason   string            `json:"reason"`
	Metadata map[string]string `json:"metadata"`
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

func TestImportDataBindsCreatedAccountsToRequestGroupIDs(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()
	adminSvc.groups = []service.Group{
		{ID: 11, Name: "openai-a", Platform: service.PlatformOpenAI, Status: service.StatusActive},
		{ID: 12, Name: "openai-b", Platform: service.PlatformOpenAI, Status: service.StatusActive},
	}

	dataPayload := map[string]any{
		"data": map[string]any{
			"type":    dataType,
			"version": dataVersion,
			"proxies": []map[string]any{},
			"accounts": []map[string]any{
				{
					"name":        "acc-1",
					"platform":    service.PlatformOpenAI,
					"type":        service.AccountTypeOAuth,
					"credentials": map[string]any{"token": "x"},
					"concurrency": 3,
					"priority":    50,
				},
				{
					"name":        "acc-2",
					"platform":    service.PlatformOpenAI,
					"type":        service.AccountTypeOAuth,
					"credentials": map[string]any{"token": "y"},
					"concurrency": 3,
					"priority":    50,
				},
			},
		},
		"group_ids": []int64{11, 12},
	}

	body, _ := json.Marshal(dataPayload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/data", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Len(t, adminSvc.createdAccounts, 2)
	require.Equal(t, []int64{11, 12}, adminSvc.createdAccounts[0].GroupIDs)
	require.Equal(t, []int64{11, 12}, adminSvc.createdAccounts[1].GroupIDs)
	require.True(t, adminSvc.createdAccounts[0].SkipDefaultGroupBind)
	require.True(t, adminSvc.createdAccounts[1].SkipDefaultGroupBind)
}

func TestImportDataWithoutGroupIDsKeepsCreatedAccountsUngrouped(t *testing.T) {
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
	}

	body, _ := json.Marshal(dataPayload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/data", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Len(t, adminSvc.createdAccounts, 1)
	require.Empty(t, adminSvc.createdAccounts[0].GroupIDs)
	require.True(t, adminSvc.createdAccounts[0].SkipDefaultGroupBind)
}

func TestImportDataWithEmptyGroupIDsKeepsCreatedAccountsUngrouped(t *testing.T) {
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
		"group_ids": []int64{},
	}

	body, _ := json.Marshal(dataPayload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/data", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Len(t, adminSvc.createdAccounts, 1)
	require.Empty(t, adminSvc.createdAccounts[0].GroupIDs)
	require.True(t, adminSvc.createdAccounts[0].SkipDefaultGroupBind)
}

func TestImportDataAllowsDisabledImportTargetGroup(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()
	adminSvc.groups = []service.Group{
		{ID: 22, Name: "disabled", Platform: service.PlatformOpenAI, Status: service.StatusDisabled},
	}

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
		"group_ids": []int64{22},
	}

	body, _ := json.Marshal(dataPayload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/data", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Len(t, adminSvc.createdAccounts, 1)
	require.Equal(t, []int64{22}, adminSvc.createdAccounts[0].GroupIDs)
}

func TestImportDataRejectsTargetGroupsAcrossPlatformsBeforeWriting(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()
	adminSvc.groups = []service.Group{
		{ID: 11, Name: "openai", Platform: service.PlatformOpenAI, Status: service.StatusActive},
		{ID: 12, Name: "anthropic", Platform: service.PlatformAnthropic, Status: service.StatusActive},
	}

	dataPayload := map[string]any{
		"data": map[string]any{
			"type":    dataType,
			"version": dataVersion,
			"proxies": []map[string]any{
				{
					"proxy_key": "http|127.0.0.1|8080||",
					"name":      "proxy",
					"protocol":  "http",
					"host":      "127.0.0.1",
					"port":      8080,
					"status":    "active",
				},
			},
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
		"group_ids": []int64{11, 12},
	}

	body, _ := json.Marshal(dataPayload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/data", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	require.Len(t, adminSvc.createdProxies, 0)
	require.Len(t, adminSvc.createdAccounts, 0)

	var resp dataErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "IMPORT_TARGET_GROUP_PLATFORM_MISMATCH", resp.Reason)
	require.Contains(t, resp.Message, "导入目标分组")
	require.Equal(t, service.PlatformOpenAI, resp.Metadata["expected_platform"])
	require.Equal(t, "1", resp.Metadata["mismatch_count"])
	require.Equal(t, "anthropic,openai", resp.Metadata["selected_platforms"])
}

func TestImportDataRejectsAccountPlatformMismatchBeforeWriting(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()
	adminSvc.groups = []service.Group{
		{ID: 11, Name: "openai", Platform: service.PlatformOpenAI, Status: service.StatusActive},
	}

	dataPayload := map[string]any{
		"data": map[string]any{
			"type":    dataType,
			"version": dataVersion,
			"proxies": []map[string]any{
				{
					"proxy_key": "http|127.0.0.1|8080||",
					"name":      "proxy",
					"protocol":  "http",
					"host":      "127.0.0.1",
					"port":      8080,
					"status":    "active",
				},
			},
			"accounts": []map[string]any{
				{
					"name":        "acc-openai",
					"platform":    service.PlatformOpenAI,
					"type":        service.AccountTypeOAuth,
					"credentials": map[string]any{"token": "x"},
					"concurrency": 3,
					"priority":    50,
				},
				{
					"name":        "acc-anthropic",
					"platform":    service.PlatformAnthropic,
					"type":        service.AccountTypeOAuth,
					"credentials": map[string]any{"token": "y"},
					"concurrency": 3,
					"priority":    50,
				},
				{
					"name":        "acc-gemini",
					"platform":    service.PlatformGemini,
					"type":        service.AccountTypeOAuth,
					"credentials": map[string]any{"token": "z"},
					"concurrency": 3,
					"priority":    50,
				},
			},
		},
		"group_ids": []int64{11},
	}

	body, _ := json.Marshal(dataPayload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/data", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	require.Len(t, adminSvc.createdProxies, 0)
	require.Len(t, adminSvc.createdAccounts, 0)

	var resp dataErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "IMPORT_ACCOUNT_PLATFORM_MISMATCH", resp.Reason)
	require.Contains(t, resp.Message, "上游账号")
	require.Equal(t, service.PlatformOpenAI, resp.Metadata["expected_platform"])
	require.Equal(t, "2", resp.Metadata["mismatch_count"])
	require.Contains(t, resp.Metadata["mismatch_examples"], "acc-anthropic")
	require.Contains(t, resp.Metadata["mismatch_examples"], service.PlatformAnthropic)
	require.Contains(t, resp.Metadata["mismatch_examples"], "acc-gemini")
	require.Contains(t, resp.Metadata["mismatch_examples"], service.PlatformGemini)
}

func TestImportDataCapsAccountPlatformMismatchExamples(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()
	adminSvc.groups = []service.Group{
		{ID: 11, Name: "openai", Platform: service.PlatformOpenAI, Status: service.StatusActive},
	}

	accounts := make([]map[string]any, 0, 7)
	for i := 1; i <= 7; i++ {
		accounts = append(accounts, map[string]any{
			"name":        "acc-mismatch-" + strconv.Itoa(i),
			"platform":    service.PlatformAnthropic,
			"type":        service.AccountTypeOAuth,
			"credentials": map[string]any{"token": "x"},
			"concurrency": 3,
			"priority":    50,
		})
	}

	dataPayload := map[string]any{
		"data": map[string]any{
			"type":     dataType,
			"version":  dataVersion,
			"proxies":  []map[string]any{},
			"accounts": accounts,
		},
		"group_ids": []int64{11},
	}

	body, _ := json.Marshal(dataPayload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/data", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	var resp dataErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "7", resp.Metadata["mismatch_count"])

	var examples []struct {
		Name     string `json:"name"`
		Platform string `json:"platform"`
	}
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata["mismatch_examples"]), &examples))
	require.Len(t, examples, 5)
	require.Equal(t, "acc-mismatch-1", examples[0].Name)
	require.Equal(t, service.PlatformAnthropic, examples[0].Platform)
	require.Equal(t, "acc-mismatch-5", examples[4].Name)
}
