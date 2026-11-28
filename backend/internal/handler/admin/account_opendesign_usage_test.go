package admin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type openDesignTestRepo struct {
	service.AccountRepository
	account  service.Account
	conflict bool
}

func (r *openDesignTestRepo) GetByID(_ context.Context, _ int64) (*service.Account, error) {
	copy := r.account
	return &copy, nil
}

func (r *openDesignTestRepo) CompareAndSwapOpenDesignUsage(_ context.Context, _ *service.Account, next json.RawMessage) (bool, error) {
	if r.conflict {
		return false, nil
	}
	if next == nil {
		delete(r.account.Extra, service.OpenDesignUsageExtraKey)
	} else {
		r.account.Extra[service.OpenDesignUsageExtraKey] = next
	}
	return true, nil
}

type openDesignTestEncryptor struct{}

func (openDesignTestEncryptor) Encrypt(value string) (string, error) {
	return "encrypted:" + value, nil
}

func (openDesignTestEncryptor) Decrypt(value string) (string, error) {
	return strings.TrimPrefix(value, "encrypted:"), nil
}

type openDesignTestUpstream struct {
	service.HTTPUpstream
	t         *testing.T
	status    int
	workspace string
	calls     int
	paths     []string
	token     bool
	member    string
	redeemed  bool
}

func (u *openDesignTestUpstream) Do(req *http.Request, proxy string, id int64, _ int) (*http.Response, error) {
	u.calls++
	u.paths = append(u.paths, req.URL.Path)
	require.Empty(u.t, proxy)
	require.Equal(u.t, int64(32), id)
	require.Equal(u.t, "https", req.URL.Scheme)
	require.Equal(u.t, "amr-api.open-design.ai", req.URL.Host)
	require.True(u.t, service.HTTPUpstreamRedirectsDisabled(req.Context()))
	require.True(u.t, service.HTTPUpstreamPublicHostsOnly(req.Context()))
	if strings.HasPrefix(req.URL.Path, "/api/v1/cli/device-authorizations") {
		require.Equal(u.t, http.MethodPost, req.Method)
		require.Empty(u.t, req.Header.Get("Cookie"))
		require.Empty(u.t, req.Header.Get("Authorization"))
		payload, err := io.ReadAll(req.Body)
		require.NoError(u.t, err)
		switch req.URL.Path {
		case "/api/v1/cli/device-authorizations":
			require.JSONEq(u.t, `{"cliVersion":"0.1.10","host":"Sub2API","profile":"prod"}`, string(payload))
			body := `{"deviceId":"device_123","deviceSecret":"secret_fixture","userCode":"ABC123",
				"activationUrl":"https://open-design.ai/cloud/cli/activate?deviceId=device_123&userCode=ABC123&extra=fixture",
				"pollIntervalSeconds":2,"expiresAt":"` + time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano) + `"}`
			return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
		case "/api/v1/cli/device-authorizations/device_123/token":
			require.JSONEq(u.t, `{"deviceSecret":"secret_fixture"}`, string(payload))
			u.redeemed = true
			body := `{"deviceId":"device_123","status":"approved","controlKey":"vela_ctrl_fixture","runtimeKey":"runtime_fixture"}`
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
		default:
			u.t.Fatalf("unexpected device authorization endpoint: %s", req.URL.Path)
		}
	}
	require.Equal(u.t, http.MethodGet, req.Method)
	if req.URL.Path == "/api/v1/workspaces/current" {
		require.Empty(u.t, req.Header.Get("X-Vela-Workspace-Id"))
	} else {
		require.Equal(u.t, "ws_123", req.Header.Get("X-Vela-Workspace-Id"))
	}
	if u.token {
		require.Equal(u.t, "Bearer vela_ctrl_fixture", req.Header.Get("Authorization"))
		require.Empty(u.t, req.Header.Get("Cookie"))
	} else {
		require.Equal(u.t, "test.session_token=fixture", req.Header.Get("Cookie"))
		require.Empty(u.t, req.Header.Get("Authorization"))
	}
	body := ""
	switch req.URL.Path {
	case "/api/v1/workspaces/current":
		body = `{"workspaceId":"ws_123","workspaceMemberId":"member_123"}`
	case "/api/v1/workspaces/ws_123/billing/preflight":
		body = `{"schemaVersion":1,"workspaceId":"ws_123","workspaceMemberId":"` + u.member + `","balanceUsd":"1.0000",
			"codingPlan":{"workspaceId":"ws_123","eligible":true,"windows":[
				{"durationSeconds":18000,"limitCredits":"50000","remainingCredits":"49993","resetsAt":null},
				{"durationSeconds":604800,"limitCredits":"100000","remainingCredits":"70000","resetsAt":"2026-11-30T10:57:18Z"}]}}`
	case "/api/v1/workspaces/ws_123/billing/coding-plan-usage":
		body = `{"workspaceId":"` + u.workspace + `","eligible":true,"windows":[
			{"durationSeconds":18000,"limitCredits":"50000","remainingCredits":"49993","resetsAt":"2026-11-24T15:57:18Z"},
			{"durationSeconds":604800,"limitCredits":"100000","remainingCredits":"70000","resetsAt":"2026-11-30T10:57:18Z"}]}`
	case "/api/v1/workspaces/ws_123/wallet/balance":
		body = `{"workspaceId":"ws_123","balanceUsd":"1.0000"}`
	default:
		u.t.Fatal("unexpected OpenDesign endpoint")
	}
	return &http.Response{StatusCode: u.status, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
}

func TestOpenDesignDeviceAuthorizationRequiresApprovalAndBindsWithoutExposingKeys(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &openDesignTestRepo{account: service.Account{
		ID: 32, Platform: service.PlatformOpenDesign, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "configured inference key"},
		Extra:       map[string]any{},
	}}
	upstream := &openDesignTestUpstream{t: t, status: http.StatusOK, token: true, member: "member_123"}
	cfg := &config.Config{}
	cfg.Totp.EncryptionKeyConfigured = true
	h := &AccountHandler{openDesignUsage: service.NewOpenDesignUsageService(repo, upstream, openDesignTestEncryptor{}, cfg)}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if actor, err := strconv.ParseInt(c.GetHeader("Test-Actor"), 10, 64); err == nil {
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: actor})
		}
	})
	router.POST("/accounts/:id/device/start", h.StartOpenDesignDeviceAuthorization)
	router.POST("/accounts/:id/device/poll", h.PollOpenDesignDeviceAuthorization)
	call := func(actor, endpoint, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/accounts/32/device/"+endpoint, strings.NewReader(body))
		req.Header.Set("Test-Actor", actor)
		r := httptest.NewRecorder()
		router.ServeHTTP(r, req)
		require.NotContains(t, r.Body.String(), "fixture")
		return r
	}
	require.Equal(t, http.StatusUnauthorized, call("", "start", "").Code)
	start := call("1", "start", "")
	require.Equal(t, http.StatusOK, start.Code, start.Body.String())
	require.Equal(t, "no-store", start.Header().Get("Cache-Control"))
	var result struct {
		Data service.OpenDesignDevicePrompt
	}
	require.NoError(t, json.Unmarshal(start.Body.Bytes(), &result))
	require.Equal(t, "https://open-design.ai/cloud/cli/activate?deviceId=device_123&userCode=ABC123", result.Data.ActivateURL)
	require.Equal(t, start.Body.String(), call("1", "start", "").Body.String())
	params := `{"attempt_id":"` + result.Data.AttemptID + `"}`
	require.Equal(t, http.StatusNotFound, call("2", "poll", params).Code)
	require.Contains(t, call("1", "poll", params).Body.String(), `"status":"pending"`)
	require.False(t, upstream.redeemed)
	require.Empty(t, repo.account.Extra)

	time.Sleep(time.Duration(result.Data.PollInterval) * time.Second)
	bound := call("1", "poll", params)
	require.Equal(t, http.StatusOK, bound.Code, bound.Body.String())
	require.Contains(t, bound.Body.String(), `"status":"bound"`)
	require.Contains(t, bound.Body.String(), `"remaining_percent":70`)
	require.True(t, upstream.redeemed)
	raw, ok := repo.account.Extra[service.OpenDesignUsageExtraKey].(json.RawMessage)
	require.True(t, ok, "expected a serialized OpenDesign binding")
	require.NotContains(t, string(raw), "runtime_fixture")
	require.NotContains(t, string(raw), "secret_fixture")
	require.Equal(t, "configured inference key", repo.account.Credentials["api_key"])
	require.Equal(t, http.StatusNotFound, call("1", "poll", params).Code)
}

func TestOpenDesignUsageTokenBindsResolvedWorkspaceAndPinsMember(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &openDesignTestRepo{account: service.Account{
		ID: 32, Platform: service.PlatformOpenDesign, Type: service.AccountTypeAPIKey, Extra: map[string]any{},
	}}
	upstream := &openDesignTestUpstream{t: t, status: http.StatusOK, token: true, member: "member_123"}
	cfg := &config.Config{}
	cfg.Totp.EncryptionKeyConfigured = true
	h := &AccountHandler{openDesignUsage: service.NewOpenDesignUsageService(repo, upstream, openDesignTestEncryptor{}, cfg)}
	router := gin.New()
	router.PUT("/accounts/:id/opendesign-usage/session", h.BindOpenDesignUsage)
	router.POST("/accounts/:id/opendesign-usage/refresh", h.RefreshOpenDesignUsage)
	call := func(method, suffix, body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(method, "/accounts/32/opendesign-usage/"+suffix, strings.NewReader(body)))
		require.NotContains(t, w.Body.String(), "fixture")
		require.NotContains(t, w.Body.String(), "ciphertext")
		require.NotContains(t, w.Body.String(), "member_123")
		return w
	}
	for _, body := range []string{
		`{"token":"sk-amr-fixture"}`,
		`{"token":"vela_ctrl_fixture","session":"test.session_token=fixture"}`,
		`{"token":"vela_ctrl_fixture\r\nbad"}`,
	} {
		require.Equal(t, http.StatusBadRequest, call(http.MethodPut, "session", body).Code)
	}
	require.Empty(t, upstream.paths)

	bound := call(http.MethodPut, "session", `{"token":"vela_ctrl_fixture"}`)
	require.Equal(t, http.StatusOK, bound.Code, bound.Body.String())
	require.Contains(t, bound.Body.String(), `"workspace_id":"ws_123"`)
	require.Contains(t, bound.Body.String(), `"remaining_percent":70`)
	require.Contains(t, bound.Body.String(), `"resets_at":"2026-11-30T10:57:18Z"`)
	require.Contains(t, bound.Body.String(), `"balance_usd":"1.0000"`)
	require.Equal(t, []string{
		"/api/v1/workspaces/current",
		"/api/v1/workspaces/ws_123/billing/preflight",
	}, upstream.paths)

	upstream.member = "changed_member"
	stale := call(http.MethodPost, "refresh", "")
	require.Equal(t, http.StatusOK, stale.Code, stale.Body.String())
	require.Contains(t, stale.Body.String(), `"status":"failed"`)
	require.Contains(t, stale.Body.String(), `"remaining_percent":70`)
	require.Equal(t, []string{"/api/v1/workspaces/ws_123/billing/preflight"}, upstream.paths[2:])
}

func TestOpenDesignUsageHandlersBindRefreshAndUnbind(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &openDesignTestRepo{account: service.Account{
		ID: 32, Platform: service.PlatformOpenDesign, Type: service.AccountTypeAPIKey, Extra: map[string]any{"normal": true},
	}}
	upstream := &openDesignTestUpstream{t: t, status: http.StatusOK, workspace: "ws_123"}
	cfg := &config.Config{}
	cfg.Totp.EncryptionKeyConfigured = true
	h := &AccountHandler{openDesignUsage: service.NewOpenDesignUsageService(repo, upstream, openDesignTestEncryptor{}, cfg)}
	router := gin.New()
	router.PUT("/accounts/:id/opendesign-usage/session", h.BindOpenDesignUsage)
	router.DELETE("/accounts/:id/opendesign-usage/session", h.UnbindOpenDesignUsage)
	router.POST("/accounts/:id/opendesign-usage/refresh", h.RefreshOpenDesignUsage)
	call := func(method, suffix, body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(method, "/accounts/32/opendesign-usage/"+suffix, strings.NewReader(body)))
		require.NotContains(t, w.Body.String(), "fixture")
		require.NotContains(t, w.Body.String(), "ciphertext")
		return w
	}
	bindBody := `{"workspace_id":"ws_123","session":"test.session_token=fixture"}`

	invalid := call(http.MethodPut, "session", `{"workspace_id":"../wrong","session":"test.session_token=fixture"}`)
	require.Equal(t, http.StatusBadRequest, invalid.Code)
	require.Zero(t, upstream.calls)

	upstream.workspace = "wrong"
	mismatch := call(http.MethodPut, "session", bindBody)
	require.Equal(t, http.StatusServiceUnavailable, mismatch.Code)
	require.NotContains(t, repo.account.Extra, service.OpenDesignUsageExtraKey)
	upstream.workspace = "ws_123"

	bound := call(http.MethodPut, "session", bindBody)
	require.Equal(t, http.StatusOK, bound.Code, bound.Body.String())
	require.Contains(t, bound.Body.String(), `"remaining_percent":70`)
	require.Contains(t, bound.Body.String(), `"resets_at":"2026-11-24T15:57:18Z"`)
	require.Contains(t, bound.Body.String(), `"balance_usd":"1.0000"`)
	raw, ok := repo.account.Extra[service.OpenDesignUsageExtraKey].(json.RawMessage)
	require.True(t, ok, "expected a serialized OpenDesign binding")
	require.Contains(t, string(raw), `"ciphertext":"encrypted:`)

	full := dto.AccountFromServiceShallow(&repo.account)
	require.Equal(t, "ws_123", full.OpenDesignUsage.WorkspaceID)
	require.NotContains(t, full.Extra, service.OpenDesignUsageExtraKey)
	require.Equal(t, full.OpenDesignUsage, dto.AccountListItemFromAccount(full).OpenDesignUsage)
	require.NotContains(t, exportAccountExtra(repo.account.Extra), service.OpenDesignUsageExtraKey)
	require.Contains(t, repo.account.Extra, service.OpenDesignUsageExtraKey)
	require.Equal(t, true, repo.account.Extra["normal"])

	upstream.status = http.StatusUnauthorized
	stale := call(http.MethodPost, "refresh", "")
	require.Equal(t, http.StatusOK, stale.Code, stale.Body.String())
	require.Contains(t, stale.Body.String(), `"status":"unauthorized"`)
	require.Contains(t, stale.Body.String(), `"remaining_percent":70`)

	repo.conflict = true
	require.Equal(t, http.StatusConflict, call(http.MethodPost, "refresh", "").Code)
	repo.conflict = false
	require.Equal(t, http.StatusOK, call(http.MethodDelete, "session", "").Code)
	require.NotContains(t, repo.account.Extra, service.OpenDesignUsageExtraKey)
	require.Equal(t, http.StatusBadRequest, call(http.MethodPost, "refresh", "").Code)
}
