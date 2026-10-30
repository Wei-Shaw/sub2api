package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
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

func TestAccountHandlerGrokModelDiagnosticsGetByID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &grokDiagHandlerUpstream{}
	account := grokDiagDetailAccount(t)
	account.Credentials["access_token"] = "access-secret"
	account.Credentials["refresh_token"] = "refresh-secret"
	handler, router := newGrokDiagDetailRouter(&simpleModeAccountService{stubAdminService: newStubAdminService(), account: account}, upstream)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/accounts/25", nil))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	body := rec.Body.String()
	require.Contains(t, body, `"current_concurrency"`)
	diag := grokDiagResponseObject(t, body)
	require.Equal(t, "fresh", diag["state"])
	require.Equal(t, "https://cli-chat-proxy.grok.com/v1/models", diag["endpoint"])
	require.Equal(t, []any{"grok-4.7"}, diag["models"])
	mappings, ok := diag["mappings"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, mappings)
	got := grokDiagMappingsByRequested(t, mappings)
	require.Equal(t, "listed", got["friendly"]["status"])
	require.Equal(t, "grok-4.7", got["friendly"]["upstream"])
	require.Equal(t, "not_listed", got["composer-2.5"]["status"])
	require.Equal(t, "grok-composer-2.5-fast", got["composer-2.5"]["upstream"])
	require.Zero(t, upstream.calls)
	require.NotContains(t, body, "access-secret")
	require.NotContains(t, body, "refresh-secret")
	require.NotContains(t, body, `"models":null`)
	require.NotContains(t, body, `"mappings":null`)

	raw, err := json.Marshal(handler.buildAccountResponseWithRuntime(context.Background(), &account))
	require.NoError(t, err)
	require.NotContains(t, string(raw), "grok_model_diagnostics")
}

func TestAccountHandlerGrokModelDiagnosticsOmitsNonGrokAndMissingSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	nonGrok := service.Account{ID: 3, Name: "claude", Platform: service.PlatformAnthropic, Type: service.AccountTypeOAuth, Status: service.StatusActive}
	_, nonGrokRouter := newGrokDiagDetailRouter(&simpleModeAccountService{stubAdminService: newStubAdminService(), account: nonGrok}, &grokDiagHandlerUpstream{})
	nonGrokRec := httptest.NewRecorder()
	nonGrokRouter.ServeHTTP(nonGrokRec, httptest.NewRequest(http.MethodGet, "/accounts/3", nil))
	require.Equal(t, http.StatusOK, nonGrokRec.Code, nonGrokRec.Body.String())
	require.NotContains(t, nonGrokRec.Body.String(), "grok_model_diagnostics")

	old := service.Account{
		ID: 25, Name: "old-grok", Platform: service.PlatformGrok, Type: service.AccountTypeOAuth, Status: service.StatusActive,
		Credentials: map[string]any{"access_token": "old-access-secret"},
	}
	_, oldRouter := newGrokDiagDetailRouter(&simpleModeAccountService{stubAdminService: newStubAdminService(), account: old}, &grokDiagHandlerUpstream{})
	oldRec := httptest.NewRecorder()
	oldRouter.ServeHTTP(oldRec, httptest.NewRequest(http.MethodGet, "/accounts/25", nil))
	require.Equal(t, http.StatusOK, oldRec.Code, oldRec.Body.String())
	diag := grokDiagResponseObject(t, oldRec.Body.String())
	require.Equal(t, "unknown", diag["state"])
	require.Empty(t, diag["models"])
	require.IsType(t, []any{}, diag["models"])
	require.IsType(t, []any{}, diag["mappings"])
	require.NotContains(t, oldRec.Body.String(), "old-access-secret")
	require.NotContains(t, oldRec.Body.String(), `"models":null`)
	require.NotContains(t, oldRec.Body.String(), `"mappings":null`)
}

func TestAccountHandlerGrokModelDiagnosticsSyncRereadsLatestAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	initial := &service.Account{
		ID: 25, Name: "grok", Platform: service.PlatformGrok, Type: service.AccountTypeAPIKey, Status: service.StatusActive,
		Credentials: map[string]any{
			"api_key":       "sync-secret",
			"base_url":      "https://relay.example.com/v1",
			"model_mapping": map[string]any{"friendly": "grok-4.7"},
		},
	}
	scope, err := service.NewGrokCatalogScope(25, service.AccountTypeAPIKey, "https://relay.example.com/v1/models")
	require.NoError(t, err)
	latestSnap := &service.GrokObservedModelsSnapshot{
		Models:    []string{"grok-from-reread"},
		FetchedAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano),
		Scope:     scope,
	}
	rawSnap, err := json.Marshal(latestSnap)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(rawSnap, &decoded))
	latest := *initial
	latest.Extra = map[string]any{"grok_observed_models": decoded}
	adminSvc := &grokDiagRereadAdmin{stubAdminService: newStubAdminService(), first: initial, latest: &latest}
	repo := &grokDiagCatalogRepo{account: cloneGrokDiagAccount(initial)}
	upstream := &grokDiagHandlerUpstream{responses: []*http.Response{
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"grok-4.7"}]}`))},
		{StatusCode: http.StatusBadGateway, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":"registry-down"}`))},
	}}
	router := newGrokDiagSyncRouter(adminSvc, repo, upstream)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/25/models/sync-upstream", nil))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	body := rec.Body.String()
	var payload struct {
		Data struct {
			Models          []string `json:"models"`
			DiscoverySource string   `json:"discovery_source"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	require.Equal(t, []string{"grok-4.7"}, payload.Data.Models)
	require.Equal(t, "upstream", payload.Data.DiscoverySource)
	diag := grokDiagResponseObject(t, body)
	require.Equal(t, []any{"grok-from-reread"}, diag["models"])
	require.Equal(t, "fresh", diag["state"])
	require.GreaterOrEqual(t, adminSvc.calls, 2)
	require.NotContains(t, body, "sync-secret")
	require.NotContains(t, body, "registry-down")
	require.NotContains(t, body, `"models":null`)
	require.NotContains(t, body, `"mappings":null`)
}

func TestAccountHandlerGrokModelDiagnosticsSyncErrorAndPreviewStayUnchanged(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adminSvc := &simpleModeAccountService{stubAdminService: newStubAdminService(), account: service.Account{
		ID: 44, Platform: service.PlatformGrok, Type: service.AccountTypeAPIKey, Status: service.StatusActive,
		Credentials: map[string]any{"base_url": "https://relay.example.com/v1"},
	}}
	router := newGrokDiagSyncRouter(adminSvc, nil, &grokDiagHandlerUpstream{})
	errorRec := httptest.NewRecorder()
	router.ServeHTTP(errorRec, httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/44/models/sync-upstream", nil))
	require.Equal(t, http.StatusBadRequest, errorRec.Code, errorRec.Body.String())
	require.Contains(t, errorRec.Body.String(), "No Grok API key is available")
	require.NotContains(t, errorRec.Body.String(), "grok_model_diagnostics")

	previewRouter := newGrokDiagSyncRouter(newStubAdminService(), nil, &grokDiagHandlerUpstream{responses: []*http.Response{
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"grok-4.7"}]}`))},
		{StatusCode: http.StatusBadGateway, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":"registry-secret"}`))},
	}})
	previewRec := httptest.NewRecorder()
	previewReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/models/sync-upstream-preview", bytes.NewBufferString(`{
		"platform":"grok","type":"apikey","base_url":"https://relay.example.com/v1","api_key":"preview-secret",
		"model_mapping":{"friendly":"grok-4.7"}
	}`))
	previewReq.Header.Set("Content-Type", "application/json")
	previewRouter.ServeHTTP(previewRec, previewReq)
	require.Equal(t, http.StatusOK, previewRec.Code, previewRec.Body.String())
	require.Contains(t, previewRec.Body.String(), `"models":["grok-4.7"]`)
	require.NotContains(t, previewRec.Body.String(), "grok_model_diagnostics")
	require.NotContains(t, previewRec.Body.String(), "preview-secret")
	require.NotContains(t, previewRec.Body.String(), "registry-secret")
}

func newGrokDiagDetailRouter(adminSvc service.AdminService, upstream *grokDiagHandlerUpstream) (*AccountHandler, *gin.Engine) {
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, newGrokDiagTestService(nil, upstream), nil, nil, nil, nil, nil)
	router := gin.New()
	router.GET("/accounts/:id", handler.GetByID)
	return handler, router
}

func newGrokDiagSyncRouter(adminSvc service.AdminService, repo service.AccountRepository, upstream *grokDiagHandlerUpstream) *gin.Engine {
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, newGrokDiagTestService(repo, upstream), nil, nil, nil, nil, nil)
	router := gin.New()
	router.POST("/api/v1/admin/accounts/:id/models/sync-upstream", handler.SyncUpstreamModels)
	router.POST("/api/v1/admin/accounts/models/sync-upstream-preview", handler.SyncUpstreamModelsPreview)
	return router
}

func newGrokDiagTestService(repo service.AccountRepository, upstream service.HTTPUpstream) *service.AccountTestService {
	return service.NewAccountTestService(repo, nil, nil, nil, nil, upstream, &config.Config{
		Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}},
	}, nil)
}

func grokDiagDetailAccount(t *testing.T) service.Account {
	t.Helper()
	scope, err := service.NewGrokCatalogScope(25, service.AccountTypeOAuth, "https://cli-chat-proxy.grok.com/v1/models")
	require.NoError(t, err)
	snap := &service.GrokObservedModelsSnapshot{
		Models:    []string{"grok-4.7"},
		FetchedAt: time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano),
		Scope:     scope,
	}
	raw, err := json.Marshal(snap)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	return service.Account{
		ID: 25, Name: "grok", Platform: service.PlatformGrok, Type: service.AccountTypeOAuth, Status: service.StatusActive,
		Credentials: map[string]any{"model_mapping": map[string]any{
			"friendly":     "grok-4.7",
			"composer-2.5": "composer-2.5",
		}},
		Extra: map[string]any{"grok_observed_models": decoded},
	}
}

func grokDiagResponseObject(t *testing.T, body string) map[string]any {
	t.Helper()
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &payload))
	data, ok := payload["data"].(map[string]any)
	require.True(t, ok, body)
	diag, ok := data["grok_model_diagnostics"].(map[string]any)
	require.True(t, ok, body)
	return diag
}

func grokDiagMappingsByRequested(t *testing.T, mappings []any) map[string]map[string]any {
	t.Helper()
	out := make(map[string]map[string]any, len(mappings))
	for _, raw := range mappings {
		item, ok := raw.(map[string]any)
		require.True(t, ok)
		requested, _ := item["requested"].(string)
		out[requested] = item
	}
	return out
}

type grokDiagRereadAdmin struct {
	*stubAdminService
	first  *service.Account
	latest *service.Account
	calls  int
}

func (s *grokDiagRereadAdmin) GetAccount(context.Context, int64) (*service.Account, error) {
	s.calls++
	if s.calls == 1 {
		return cloneGrokDiagAccount(s.first), nil
	}
	return cloneGrokDiagAccount(s.latest), nil
}

type grokDiagCatalogRepo struct {
	service.AccountRepository
	mu      sync.Mutex
	account *service.Account
}

func (r *grokDiagCatalogRepo) GetByID(_ context.Context, id int64) (*service.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.account == nil || r.account.ID != id {
		return nil, service.ErrAccountNotFound
	}
	return cloneGrokDiagAccount(r.account), nil
}

func (r *grokDiagCatalogRepo) UpdateExtra(context.Context, int64, map[string]any) error {
	return nil
}

func (r *grokDiagCatalogRepo) CompareAndSwapGrokObservedModels(_ context.Context, id int64, expected, next json.RawMessage) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.account == nil || r.account.ID != id {
		return false, nil
	}
	var current any
	if r.account.Extra != nil {
		current = r.account.Extra["grok_observed_models"]
	}
	currentJSON, err := json.Marshal(current)
	if err != nil {
		return false, err
	}
	var expectedValue any
	var currentValue any
	if err := json.Unmarshal(expected, &expectedValue); err != nil {
		return false, err
	}
	if err := json.Unmarshal(currentJSON, &currentValue); err != nil {
		return false, err
	}
	if !reflect.DeepEqual(expectedValue, currentValue) {
		return false, nil
	}
	var decoded any
	if err := json.Unmarshal(next, &decoded); err != nil {
		return false, err
	}
	if r.account.Extra == nil {
		r.account.Extra = map[string]any{}
	}
	r.account.Extra["grok_observed_models"] = decoded
	return true, nil
}

func cloneGrokDiagAccount(src *service.Account) *service.Account {
	if src == nil {
		return nil
	}
	raw, err := json.Marshal(src)
	if err != nil {
		copied := *src
		return &copied
	}
	var dst service.Account
	if err := json.Unmarshal(raw, &dst); err != nil {
		copied := *src
		return &copied
	}
	return &dst
}

type grokDiagHandlerUpstream struct {
	mu        sync.Mutex
	calls     int
	responses []*http.Response
}

func (u *grokDiagHandlerUpstream) Do(*http.Request, string, int64, int) (*http.Response, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.calls++
	if len(u.responses) == 0 {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"grok-4.7"}]}`)), Header: make(http.Header)}, nil
	}
	resp := u.responses[0]
	u.responses = u.responses[1:]
	return resp, nil
}

func (u *grokDiagHandlerUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}
