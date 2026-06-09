package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
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
	router.POST("/api/v1/admin/accounts/data/inspect", h.InspectData)
	router.POST("/api/v1/admin/accounts/data/inspect/stream", h.InspectDataStream)
	return router, adminSvc
}

func setupAccountDataRouterWithTestService(accountTestSvc *service.AccountTestService) (*gin.Engine, *stubAdminService) {
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
		accountTestSvc,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	router.POST("/api/v1/admin/accounts/data/inspect", h.InspectData)
	router.POST("/api/v1/admin/accounts/data/inspect/stream", h.InspectDataStream)
	return router, adminSvc
}

type dataInspectHTTPUpstream struct {
	mu                 sync.Mutex
	responses          []*http.Response
	responseForRequest func(*http.Request) (*http.Response, error)
	requests           []*http.Request
}

func (u *dataInspectHTTPUpstream) Do(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return nil, fmt.Errorf("unexpected Do call")
}

func (u *dataInspectHTTPUpstream) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	u.mu.Lock()
	u.requests = append(u.requests, req)
	responseForRequest := u.responseForRequest
	if responseForRequest != nil {
		u.mu.Unlock()
		return responseForRequest(req)
	}
	if len(u.responses) == 0 {
		u.mu.Unlock()
		return nil, fmt.Errorf("no mocked response")
	}
	resp := u.responses[0]
	u.responses = u.responses[1:]
	u.mu.Unlock()
	return resp, nil
}

func newDataInspectResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

type flushRecorder struct {
	header  http.Header
	body    bytes.Buffer
	code    int
	mu      sync.Mutex
	flushCh chan string
}

func newFlushRecorder() *flushRecorder {
	return &flushRecorder{
		header:  make(http.Header),
		code:    http.StatusOK,
		flushCh: make(chan string, 8),
	}
}

func (r *flushRecorder) Header() http.Header {
	return r.header
}

func (r *flushRecorder) WriteHeader(statusCode int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.code = statusCode
}

func (r *flushRecorder) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.body.Write(p)
}

func (r *flushRecorder) Flush() {
	r.mu.Lock()
	body := r.body.String()
	r.mu.Unlock()
	r.flushCh <- body
}

func (r *flushRecorder) waitForFlush(t *testing.T) string {
	t.Helper()
	select {
	case body := <-r.flushCh:
		return body
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for stream flush")
		return ""
	}
}

func (r *flushRecorder) bodyString() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.body.String()
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

func TestImportDataKeepsOnlyOneSelectedGroup(t *testing.T) {
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
					"type":        service.AccountTypeAPIKey,
					"credentials": map[string]any{"api_key": "sk-test"},
					"concurrency": 3,
					"priority":    50,
				},
			},
		},
		"group_ids":               []int64{7, 8},
		"skip_default_group_bind": true,
	}

	body, _ := json.Marshal(dataPayload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/data", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Len(t, adminSvc.createdAccounts, 1)
	require.Equal(t, []int64{7}, adminSvc.createdAccounts[0].GroupIDs)
}

func TestInspectDataFlagsInvalidAccountsWithoutImporting(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()

	expiredAt := time.Now().Add(-time.Hour).Unix()
	dataPayload := map[string]any{
		"data": map[string]any{
			"type": "sub2api-data",
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
					"name":        "healthy",
					"platform":    service.PlatformOpenAI,
					"type":        service.AccountTypeAPIKey,
					"credentials": map[string]any{"api_key": "sk-test"},
					"proxy_key":   "http|127.0.0.1|8080||",
				},
				{
					"name":        "missing-key",
					"platform":    service.PlatformOpenAI,
					"type":        service.AccountTypeAPIKey,
					"credentials": map[string]any{},
				},
				{
					"name":        "expired",
					"platform":    service.PlatformAnthropic,
					"type":        service.AccountTypeOAuth,
					"credentials": map[string]any{"access_token": "token"},
					"expires_at":  expiredAt,
				},
				{
					"name":        "bad-proxy",
					"platform":    service.PlatformGemini,
					"type":        service.AccountTypeAPIKey,
					"credentials": map[string]any{"api_key": "key"},
					"proxy_key":   "missing-proxy",
				},
				{
					"name":        "missing-service-account-json",
					"platform":    service.PlatformGemini,
					"type":        service.AccountTypeServiceAccount,
					"credentials": map[string]any{"project_id": "vertex-proj"},
				},
			},
		},
	}

	body, _ := json.Marshal(dataPayload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/data/inspect", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Total     int `json:"total"`
			Healthy   int `json:"healthy"`
			Unhealthy int `json:"unhealthy"`
			Results   []struct {
				Index   int      `json:"index"`
				Name    string   `json:"name"`
				Healthy bool     `json:"healthy"`
				Reasons []string `json:"reasons"`
			} `json:"results"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, 5, resp.Data.Total)
	require.Equal(t, 1, resp.Data.Healthy)
	require.Equal(t, 4, resp.Data.Unhealthy)
	require.True(t, resp.Data.Results[0].Healthy)
	require.False(t, resp.Data.Results[1].Healthy)
	require.Contains(t, strings.Join(resp.Data.Results[1].Reasons, ","), "api_key")
	require.False(t, resp.Data.Results[2].Healthy)
	require.Contains(t, strings.Join(resp.Data.Results[2].Reasons, ","), "expired")
	require.False(t, resp.Data.Results[3].Healthy)
	require.Contains(t, strings.Join(resp.Data.Results[3].Reasons, ","), "proxy_key not found")
	require.False(t, resp.Data.Results[4].Healthy)
	require.Contains(t, strings.Join(resp.Data.Results[4].Reasons, ","), "service_account_json")
	require.Empty(t, adminSvc.createdAccounts)
}

func TestInspectDataAcceptsValidProxyKeysFromPreviousChunks(t *testing.T) {
	router, _ := setupAccountDataRouter()

	dataPayload := map[string]any{
		"data": map[string]any{
			"type":    dataType,
			"proxies": []map[string]any{},
			"accounts": []map[string]any{
				{
					"name":        "uses-imported-proxy",
					"platform":    service.PlatformOpenAI,
					"type":        service.AccountTypeAPIKey,
					"credentials": map[string]any{"api_key": "sk-test"},
					"proxy_key":   "http|imported.example|8080||",
				},
			},
		},
		"valid_proxy_keys": []string{"http|imported.example|8080||"},
	}

	body, _ := json.Marshal(dataPayload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/data/inspect", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Healthy        int      `json:"healthy"`
			Unhealthy      int      `json:"unhealthy"`
			ValidProxyKeys []string `json:"valid_proxy_keys"`
			Results        []struct {
				Healthy bool `json:"healthy"`
			} `json:"results"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, 1, resp.Data.Healthy)
	require.Equal(t, 0, resp.Data.Unhealthy)
	require.Len(t, resp.Data.Results, 1)
	require.True(t, resp.Data.Results[0].Healthy)
	require.Contains(t, resp.Data.ValidProxyKeys, "http|imported.example|8080||")
}

func TestInspectDataRunsLiveProbeAndFlagsUpstreamFailures(t *testing.T) {
	upstream := &dataInspectHTTPUpstream{
		responseForRequest: func(req *http.Request) (*http.Response, error) {
			require.Equal(t, http.MethodGet, req.Method)
			require.Equal(t, "https://api.openai.com/v1/models", req.URL.String())
			require.Nil(t, req.Body)
			switch req.Header.Get("Authorization") {
			case "Bearer sk-ok":
				return newDataInspectResponse(http.StatusOK, `{"data":[]}`), nil
			case "Bearer sk-bad":
				return newDataInspectResponse(http.StatusUnauthorized, "{\"error\":{\"message\":\"bad key\"}}"), nil
			default:
				return nil, fmt.Errorf("unexpected authorization header: %s", req.Header.Get("Authorization"))
			}
		},
	}
	accountTestSvc := service.NewAccountTestService(
		nil,
		nil,
		nil,
		nil,
		upstream,
		&config.Config{},
		nil,
	)
	router, _ := setupAccountDataRouterWithTestService(accountTestSvc)

	dataPayload := map[string]any{
		"data": map[string]any{
			"type":    dataType,
			"version": dataVersion,
			"proxies": []map[string]any{},
			"accounts": []map[string]any{
				{
					"name":        "live-ok",
					"platform":    service.PlatformOpenAI,
					"type":        service.AccountTypeAPIKey,
					"credentials": map[string]any{"api_key": "sk-ok"},
					"concurrency": 1,
					"priority":    50,
				},
				{
					"name":        "live-bad",
					"platform":    service.PlatformOpenAI,
					"type":        service.AccountTypeAPIKey,
					"credentials": map[string]any{"api_key": "sk-bad"},
					"concurrency": 1,
					"priority":    50,
				},
			},
		},
	}

	body, _ := json.Marshal(dataPayload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/data/inspect", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Healthy   int `json:"healthy"`
			Unhealthy int `json:"unhealthy"`
			Results   []struct {
				Name    string   `json:"name"`
				Healthy bool     `json:"healthy"`
				Reasons []string `json:"reasons"`
			} `json:"results"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, 1, resp.Data.Healthy)
	require.Equal(t, 1, resp.Data.Unhealthy)
	require.True(t, resp.Data.Results[0].Healthy)
	require.False(t, resp.Data.Results[1].Healthy)
	require.Contains(t, strings.Join(resp.Data.Results[1].Reasons, ","), "live probe failed")
	require.Len(t, upstream.requests, 2)
	authHeaders := []string{
		upstream.requests[0].Header.Get("Authorization"),
		upstream.requests[1].Header.Get("Authorization"),
	}
	require.ElementsMatch(t, []string{"Bearer sk-ok", "Bearer sk-bad"}, authHeaders)
}

func TestInspectDataUsesWhamAccountCheckForOpenAIOAuthAccounts(t *testing.T) {
	upstream := &dataInspectHTTPUpstream{
		responseForRequest: func(req *http.Request) (*http.Response, error) {
			require.Equal(t, http.MethodGet, req.Method)
			require.Equal(t, "https://chatgpt.com/backend-api/wham/accounts/check", req.URL.String())
			require.Equal(t, "chatgpt.com", req.Host)
			require.Equal(t, "Bearer oauth-token", req.Header.Get("Authorization"))
			require.Equal(t, "acct-1", req.Header.Get("ChatGPT-Account-Id"))
			require.Nil(t, req.Body)
			return newDataInspectResponse(http.StatusOK, `{"accounts":[{"id":"acct-1","name":"Team","structure":"workspace"}]}`), nil
		},
	}
	accountTestSvc := service.NewAccountTestService(
		nil,
		nil,
		nil,
		nil,
		upstream,
		&config.Config{},
		nil,
	)
	router, _ := setupAccountDataRouterWithTestService(accountTestSvc)

	dataPayload := map[string]any{
		"data": map[string]any{
			"type":    dataType,
			"version": dataVersion,
			"proxies": []map[string]any{},
			"accounts": []map[string]any{
				{
					"name":     "chatgpt-oauth",
					"platform": service.PlatformOpenAI,
					"type":     service.AccountTypeOAuth,
					"credentials": map[string]any{
						"access_token":       "oauth-token",
						"chatgpt_account_id": "acct-1",
					},
					"concurrency": 1,
					"priority":    50,
				},
			},
		},
	}

	body, _ := json.Marshal(dataPayload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/data/inspect", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Len(t, upstream.requests, 1)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Healthy   int `json:"healthy"`
			Unhealthy int `json:"unhealthy"`
			Results   []struct {
				Healthy bool     `json:"healthy"`
				Reasons []string `json:"reasons"`
			} `json:"results"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, 1, resp.Data.Healthy)
	require.Equal(t, 0, resp.Data.Unhealthy)
	require.True(t, resp.Data.Results[0].Healthy)
	require.Empty(t, resp.Data.Results[0].Reasons)
}

func TestInspectDataUsesWhamAccountCheckForOpenAISetupTokenAccounts(t *testing.T) {
	upstream := &dataInspectHTTPUpstream{
		responseForRequest: func(req *http.Request) (*http.Response, error) {
			require.Equal(t, http.MethodGet, req.Method)
			require.Equal(t, "https://chatgpt.com/backend-api/wham/accounts/check", req.URL.String())
			require.Equal(t, "chatgpt.com", req.Host)
			require.Equal(t, "Bearer setup-token", req.Header.Get("Authorization"))
			require.Equal(t, "acct-setup", req.Header.Get("ChatGPT-Account-Id"))
			require.Nil(t, req.Body)
			return newDataInspectResponse(http.StatusOK, `{"accounts":[{"id":"acct-setup","name":"Team","structure":"workspace"}]}`), nil
		},
	}
	accountTestSvc := service.NewAccountTestService(
		nil,
		nil,
		nil,
		nil,
		upstream,
		&config.Config{},
		nil,
	)
	router, _ := setupAccountDataRouterWithTestService(accountTestSvc)

	dataPayload := map[string]any{
		"data": map[string]any{
			"type":    dataType,
			"version": dataVersion,
			"proxies": []map[string]any{},
			"accounts": []map[string]any{
				{
					"name":     "chatgpt-setup-token",
					"platform": service.PlatformOpenAI,
					"type":     service.AccountTypeSetupToken,
					"credentials": map[string]any{
						"access_token":       "setup-token",
						"chatgpt_account_id": "acct-setup",
					},
					"concurrency": 1,
					"priority":    50,
				},
			},
		},
	}

	body, _ := json.Marshal(dataPayload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/data/inspect", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, upstream.requests, 1)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Healthy   int `json:"healthy"`
			Unhealthy int `json:"unhealthy"`
			Results   []struct {
				Healthy bool     `json:"healthy"`
				Reasons []string `json:"reasons"`
			} `json:"results"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, 1, resp.Data.Healthy)
	require.Equal(t, 0, resp.Data.Unhealthy)
	require.True(t, resp.Data.Results[0].Healthy)
	require.Empty(t, resp.Data.Results[0].Reasons)
}

func TestInspectDataLimitsLiveProbeConcurrency(t *testing.T) {
	var mu sync.Mutex
	active := 0
	maxActive := 0
	upstream := &dataInspectHTTPUpstream{
		responseForRequest: func(req *http.Request) (*http.Response, error) {
			mu.Lock()
			active++
			if active > maxActive {
				maxActive = active
			}
			mu.Unlock()

			time.Sleep(10 * time.Millisecond)

			mu.Lock()
			active--
			mu.Unlock()
			return newDataInspectResponse(http.StatusOK, "data: {\"type\":\"response.completed\"}\n\n"), nil
		},
	}
	accountTestSvc := service.NewAccountTestService(
		nil,
		nil,
		nil,
		nil,
		upstream,
		&config.Config{},
		nil,
	)
	router, _ := setupAccountDataRouterWithTestService(accountTestSvc)

	accounts := make([]map[string]any, 0, 12)
	for i := 0; i < 12; i++ {
		accounts = append(accounts, map[string]any{
			"name":        fmt.Sprintf("live-%d", i),
			"platform":    service.PlatformOpenAI,
			"type":        service.AccountTypeAPIKey,
			"credentials": map[string]any{"api_key": fmt.Sprintf("sk-%d", i)},
			"concurrency": 1,
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
	}

	body, _ := json.Marshal(dataPayload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/data/inspect", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Len(t, upstream.requests, 12)
	require.LessOrEqual(t, maxActive, dataInspectLiveProbeConcurrency)
}

func TestInspectDataStreamFlushesEachAccountResult(t *testing.T) {
	blockLiveProbe := make(chan struct{})
	upstream := &dataInspectHTTPUpstream{
		responseForRequest: func(req *http.Request) (*http.Response, error) {
			<-blockLiveProbe
			return newDataInspectResponse(http.StatusOK, "data: {\"type\":\"response.completed\"}\n\n"), nil
		},
	}
	accountTestSvc := service.NewAccountTestService(
		nil,
		nil,
		nil,
		nil,
		upstream,
		&config.Config{},
		nil,
	)
	router, _ := setupAccountDataRouterWithTestService(accountTestSvc)

	dataPayload := map[string]any{
		"data": map[string]any{
			"type":    dataType,
			"version": dataVersion,
			"proxies": []map[string]any{},
			"accounts": []map[string]any{
				{
					"name":        "bad-now",
					"platform":    service.PlatformOpenAI,
					"type":        service.AccountTypeAPIKey,
					"credentials": map[string]any{},
					"concurrency": 1,
					"priority":    50,
				},
				{
					"name":        "slow-ok",
					"platform":    service.PlatformOpenAI,
					"type":        service.AccountTypeAPIKey,
					"credentials": map[string]any{"api_key": "sk-ok"},
					"concurrency": 1,
					"priority":    50,
				},
			},
		},
	}

	body, _ := json.Marshal(dataPayload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/data/inspect/stream", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rec := newFlushRecorder()
	done := make(chan struct{})
	go func() {
		router.ServeHTTP(rec, req)
		close(done)
	}()

	firstFlush := rec.waitForFlush(t)
	require.Contains(t, firstFlush, `"type":"item"`)
	require.Contains(t, firstFlush, "bad-now")
	require.Contains(t, firstFlush, "api_key is required")
	require.NotContains(t, firstFlush, `"type":"done"`)

	close(blockLiveProbe)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stream inspect did not finish")
	}
	require.Contains(t, rec.bodyString(), `"type":"done"`)
}

func TestImportDataSupportsBedrockAndServiceAccountTypes(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()

	dataPayload := map[string]any{
		"data": map[string]any{
			"type":    dataType,
			"version": dataVersion,
			"proxies": []map[string]any{},
			"accounts": []map[string]any{
				{
					"name":     "bedrock-api-key",
					"platform": service.PlatformAnthropic,
					"type":     service.AccountTypeBedrock,
					"credentials": map[string]any{
						"auth_mode": "apikey",
						"api_key":   "bedrock-key",
					},
				},
				{
					"name":     "vertex-sa",
					"platform": service.PlatformGemini,
					"type":     service.AccountTypeServiceAccount,
					"credentials": map[string]any{
						"service_account_json": `{"project_id":"vertex-proj"}`,
						"project_id":           "vertex-proj",
					},
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

	require.Len(t, adminSvc.createdAccounts, 2)
	require.Equal(t, service.AccountTypeBedrock, adminSvc.createdAccounts[0].Type)
	require.Equal(t, service.AccountTypeServiceAccount, adminSvc.createdAccounts[1].Type)
}
