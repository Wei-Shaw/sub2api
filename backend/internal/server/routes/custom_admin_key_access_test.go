//go:build unit

package routes

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type scopedRouteKeys struct {
	service.ScopedAdminKeyRepository
	item *service.ScopedAdminKey
}

func (r *scopedRouteKeys) InsertScopedAdminKey(_ context.Context, item *service.ScopedAdminKey) error {
	r.item = item
	return nil
}
func (r *scopedRouteKeys) GetScopedAdminKey(context.Context, string) (*service.ScopedAdminKey, error) {
	return r.item, nil
}
func (r *scopedRouteKeys) TouchScopedAdminKey(context.Context, string, string, time.Time, string, time.Time) error {
	return nil
}

type scopedRouteUsers struct{ service.UserRepository }

type scopedRouteModelSettings struct{ service.SettingRepository }

func (*scopedRouteModelSettings) Get(context.Context, string) (*service.Setting, error) {
	return nil, service.ErrSettingNotFound
}

func (*scopedRouteUsers) GetByID(context.Context, int64) (*service.User, error) {
	return &service.User{ID: 9, Role: service.RoleAdmin, Status: service.StatusActive}, nil
}
func (*scopedRouteUsers) GetUserAvatar(context.Context, int64) (*service.UserAvatar, error) {
	return nil, nil
}

type scopedRouteAccounts struct {
	service.AdminService
	t *testing.T
}

func (*scopedRouteAccounts) GetAccount(_ context.Context, id int64) (*service.Account, error) {
	a := &service.Account{ID: id, Type: "oauth", Platform: "openai", GroupIDs: []int64{1, 2}, Credentials: map[string]any{"access_token": "绝不返回的凭证", "model_mapping": map[string]any{"custom-model": "upstream-model"}}, Extra: map[string]any{"secret": "绝不返回的扩展"}}
	if id == 2 {
		a.Type = "apikey"
	}
	if id == 3 {
		a.GroupIDs = []int64{2}
	}
	if id == 4 {
		a.Platform = "anthropic"
	}
	if id == 5 {
		a.Credentials["auth_mode"] = service.OpenAIAuthModePersonalAccessToken
	}
	if id == 6 {
		a.Credentials["auth_mode"] = service.OpenAIAuthModeAgentIdentity
		a.Credentials["agent_private_key"] = "绝不返回的代理身份私钥"
	}
	return a, nil
}
func (s *scopedRouteAccounts) ScopedAdminList(context.Context, pagination.PaginationParams, string, string, string, string) ([]service.Account, int64, error) {
	s.t.Fatal("本测试中列表必须在权限准入阶段被拒绝")
	return nil, 0, nil
}
func (s *scopedRouteAccounts) ScopedAdminWrite(context.Context, int64, string, map[string]json.RawMessage, func(context.Context, *service.Account) (*service.Account, error)) (*service.Account, error) {
	s.t.Fatal("未授予写权限，不应进入写服务")
	return nil, nil
}

func TestScopedAdminRegisteredRoutesEnforceDefaultDenyAndAccountScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, withRead := range []bool{false, true} {
		t.Run(map[bool]string{false: "仅分组不赋权", true: "只读OAuth指定组"}[withRead], func(t *testing.T) {
			rule := adminaccess.Rule{GroupIDs: []int64{1}, AccountTypes: []string{"oauth"}}
			if withRead {
				rule.Permissions = []string{"accounts.read"}
			}
			keys := &scopedRouteKeys{}
			settings := service.NewSettingService(nil, nil)
			settings.SetScopedAdminKeyRepository(keys)
			key, _, err := settings.CreateScopedAdminKey(context.Background(), service.ScopedAdminKeyInput{Name: "路由验收", Enabled: true, Policy: adminaccess.Policy{Rules: []adminaccess.Rule{rule}}}, 9)
			require.NoError(t, err)
			users := service.NewUserService(&scopedRouteUsers{}, nil, nil, nil)
			auth := middleware.NewAdminAuthMiddleware(nil, users, settings, nil)
			accountHandler := adminhandler.NewAccountHandler(&scopedRouteAccounts{t: t}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
			r := gin.New()
			RegisterAdminRoutes(r.Group("/api/v1"), &handler.Handlers{Admin: &handler.AdminHandlers{Account: accountHandler, Setting: adminhandler.NewSettingHandler(settings, nil, nil, nil, nil, nil, nil)}}, auth, middleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() }), middleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() }), nil, nil)
			RegisterPaymentRoutes(r.Group("/api/v1"), &handler.PaymentHandler{}, &handler.PaymentWebhookHandler{}, &adminhandler.PaymentHandler{}, middleware.JWTAuthMiddleware(func(c *gin.Context) { c.Next() }), auth, middleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() }), nil, nil, nil)
			for _, tc := range []struct{ method, path string }{
				{"GET", "/api/v1/admin/accounts/1"},
				{"GET", "/api/v1/admin/accounts/2"},
				{"GET", "/api/v1/admin/accounts/3"},
				{"GET", "/api/v1/admin/accounts/5"},
				{"GET", "/api/v1/admin/accounts/6"},
				{"DELETE", "/api/v1/admin/accounts/1"},
				{"GET", "/api/v1/admin/users"},
				{"PUT", "/api/v1/admin/settings"},
				{"GET", "/api/v1/admin/settings/admin-api-keys"},
				{"GET", "/api/v1/admin/groups"},
				{"GET", "/api/v1/admin/payment/orders"},
				{"POST", "/api/v1/admin/payment/orders/1/refund"},
				{"PUT", "/api/v1/admin/payment/config"},
			} {
				req := httptest.NewRequest(tc.method, tc.path, nil)
				req.Header.Set("x-api-key", key)
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				want := 403
				if withRead && tc.method == "GET" {
					switch tc.path {
					case "/api/v1/admin/accounts/1", "/api/v1/admin/accounts/5", "/api/v1/admin/accounts/6":
						want = 200
					case "/api/v1/admin/accounts/2", "/api/v1/admin/accounts/3":
						want = 404
					}
				}
				require.Equal(t, want, w.Code, tc.method+" "+tc.path+": "+w.Body.String())
				require.NotContains(t, w.Body.String(), "绝不返回")
				require.NotContains(t, w.Body.String(), "auth_mode")
				require.NotContains(t, w.Body.String(), "agent_private_key")
				if want == 200 {
					require.Contains(t, w.Body.String(), `"group_ids":[1]`)
				}
			}
		})
	}
}

func TestScopedAdminRegisteredModelDefaultsRequireExplicitPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, allowed := range []bool{false, true} {
		policy := adminaccess.Policy{}
		if allowed {
			policy.GlobalPermissions = []string{"models.defaults.read"}
		}
		keys := &scopedRouteKeys{}
		settings := service.NewSettingService(&scopedRouteModelSettings{}, nil)
		settings.SetScopedAdminKeyRepository(keys)
		key, _, err := settings.CreateScopedAdminKey(context.Background(), service.ScopedAdminKeyInput{Name: "默认模型读取", Enabled: true, Policy: policy}, 9)
		require.NoError(t, err)
		users := service.NewUserService(&scopedRouteUsers{}, nil, nil, nil)
		auth := middleware.NewAdminAuthMiddleware(nil, users, settings, nil)
		accountHandler := adminhandler.NewAccountHandler(&scopedRouteAccounts{t: t}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
		r := gin.New()
		RegisterAdminRoutes(r.Group("/api/v1"), &handler.Handlers{Admin: &handler.AdminHandlers{Account: accountHandler, Setting: adminhandler.NewSettingHandler(settings, nil, nil, nil, nil, nil, nil)}}, auth, middleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() }), middleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() }), nil, nil)
		for _, path := range []string{"/api/v1/admin/accounts/antigravity/default-model-mapping"} {
			req := httptest.NewRequest("GET", path, nil)
			req.Header.Set("x-api-key", key)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			want := 403
			if allowed {
				want = 200
			}
			require.Equal(t, want, w.Code, w.Body.String())
			if allowed {
				require.Contains(t, w.Body.String(), `"data":{`)
			}
		}
	}
}

// 使用真实注册路由，验证旧 Key 的模型读写入口也能进入受限处理器。
func TestScopedAdminRegisteredLegacyModelInterfaces(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, permission := range []string{"accounts.read", "accounts.models.write", "models.defaults.read"} {
		t.Run(permission, func(t *testing.T) {
			policy := adminaccess.Policy{}
			if permission == "models.defaults.read" {
				policy.GlobalPermissions = []string{permission}
			} else {
				policy.Rules = []adminaccess.Rule{{Permissions: []string{permission}, GroupIDs: []int64{1}, AccountTypes: []string{"oauth"}, Platforms: []string{"openai"}}}
			}
			keys := &scopedRouteKeys{}
			settings := service.NewSettingService(nil, nil)
			settings.SetScopedAdminKeyRepository(keys)
			key, _, err := settings.CreateScopedAdminKey(context.Background(), service.ScopedAdminKeyInput{Name: "原模型接口", Enabled: true, Policy: policy}, 9)
			require.NoError(t, err)
			users := service.NewUserService(&scopedRouteUsers{}, nil, nil, nil)
			auth := middleware.NewAdminAuthMiddleware(nil, users, settings, nil)
			accounts := &scopedRouteModelAccounts{scopedRouteAccounts: scopedRouteAccounts{t: t}, t: t}
			accountHandler := adminhandler.NewAccountHandler(accounts, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
			r := gin.New()
			RegisterAdminRoutes(r.Group("/api/v1"), &handler.Handlers{Admin: &handler.AdminHandlers{Account: accountHandler}}, auth, middleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() }), middleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() }), nil, nil)
			for _, tc := range []struct {
				method, path, body string
				want               int
			}{
				{"GET", "/api/v1/admin/accounts/1/models", "", 200},
				{"GET", "/api/v1/admin/accounts/2/models", "", 404},
				{"GET", "/api/v1/admin/accounts/3/models", "", 404},
				{"GET", "/api/v1/admin/accounts/4/models", "", 404},
				{"GET", "/api/v1/admin/accounts/1", "", 200},
				{"POST", "/api/v1/admin/accounts/bulk-update", `{"account_ids":[1],"credentials":{"model_mapping":{"custom-model":"upstream-model"}}}`, 200},
				{"PUT", "/api/v1/admin/accounts/1", `{"credentials":{"model_mapping":{"custom-model":"upstream-model"}}}`, 200},
			} {
				want := tc.want
				if (tc.method == "GET" && permission != "accounts.read") || (tc.method != "GET" && permission != "accounts.models.write") {
					want = 403
				}
				req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
				req.Header.Set("x-api-key", key)
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				require.Equal(t, want, w.Code, tc.method+" "+tc.path+": "+w.Body.String())
				require.NotContains(t, w.Body.String(), "绝不返回")
				if tc.method == "GET" && want == 200 {
					require.Contains(t, w.Body.String(), "custom-model")
				}
			}
			if permission == "accounts.models.write" {
				require.Equal(t, []int64{1, 1}, accounts.writes)
			} else {
				require.Empty(t, accounts.writes)
			}
		})
	}
}

type scopedRouteModelAccounts struct {
	scopedRouteAccounts
	t      *testing.T
	writes []int64
}

func (s *scopedRouteModelAccounts) ScopedAdminWrite(_ context.Context, id int64, action string, fields map[string]json.RawMessage, run func(context.Context, *service.Account) (*service.Account, error)) (*service.Account, error) {
	require.Empty(s.t, action)
	require.Nil(s.t, run)
	body, err := json.Marshal(fields)
	require.NoError(s.t, err)
	require.JSONEq(s.t, `{"credentials":{"model_mapping":{"custom-model":"upstream-model"}}}`, string(body))
	s.writes = append(s.writes, id)
	return &service.Account{ID: id}, nil
}

func TestScopedAdminCodexImportRouteReachesValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	keys := &scopedRouteKeys{}
	settings := service.NewSettingService(nil, nil)
	settings.SetScopedAdminKeyRepository(keys)
	policy := adminaccess.Policy{Rules: []adminaccess.Rule{{Permissions: []string{"accounts.create", "accounts.credentials"}, GroupIDs: []int64{2}, AccountTypes: []string{"oauth"}}}}
	key, _, err := settings.CreateScopedAdminKey(context.Background(), service.ScopedAdminKeyInput{Name: "导入回归", Enabled: true, Policy: policy}, 9)
	require.NoError(t, err)
	users := service.NewUserService(&scopedRouteUsers{}, nil, nil, nil)
	auth := middleware.NewAdminAuthMiddleware(nil, users, settings, nil)
	accountHandler := adminhandler.NewAccountHandler(&scopedRouteAccounts{t: t}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := gin.New()
	RegisterAdminRoutes(r.Group("/api/v1"), &handler.Handlers{Admin: &handler.AdminHandlers{Account: accountHandler}}, auth, middleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() }), middleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() }), nil, nil)
	req := httptest.NewRequest("POST", "/api/v1/admin/accounts/import/codex-session", strings.NewReader(`{"content":"","group_ids":[2]}`))
	req.Header.Set("x-api-key", key)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, 400, w.Code, w.Body.String())
}

type scopedRouteCodexAccounts struct {
	service.AdminService
	created bool
}

func (*scopedRouteCodexAccounts) ScopedAdminCodexCandidates(context.Context, service.ScopedCodexIdentity) ([]service.Account, error) {
	return nil, nil
}
func (s *scopedRouteCodexAccounts) ScopedAdminCreateCodex(_ context.Context, input *service.CreateAccountInput) (*service.Account, error) {
	s.created = true
	return &service.Account{ID: 100, Credentials: input.Credentials}, nil
}
func (*scopedRouteCodexAccounts) ScopedAdminUpdateCodex(context.Context, int64, *service.UpdateAccountInput, func(*service.Account, *service.UpdateAccountInput) error) (*service.Account, error) {
	panic("本用例不能更新已有账户")
}
func TestScopedAdminCodexImportAuthenticatedRoute(t *testing.T) {
	for _, allowed := range []bool{false, true} {
		t.Run(map[bool]string{false: "未勾选拒绝", true: "无读取权限可导入"}[allowed], func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			rule := adminaccess.Rule{Permissions: []string{"accounts.credentials", "accounts.schedule"}, GroupIDs: []int64{2}, AccountTypes: []string{"oauth"}}
			if allowed {
				rule.Permissions = append(rule.Permissions, "accounts.create")
			}
			settings := service.NewSettingService(nil, nil)
			settings.SetScopedAdminKeyRepository(&scopedRouteKeys{})
			key, _, err := settings.CreateScopedAdminKey(context.Background(), service.ScopedAdminKeyInput{Name: "导入鉴权", Enabled: true, Policy: adminaccess.Policy{Rules: []adminaccess.Rule{rule}}}, 9)
			require.NoError(t, err)
			users := service.NewUserService(&scopedRouteUsers{}, nil, nil, nil)
			auth := middleware.NewAdminAuthMiddleware(nil, users, settings, nil)
			api := &scopedRouteCodexAccounts{}
			h := adminhandler.NewAccountHandler(api, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
			r := gin.New()
			RegisterAdminRoutes(r.Group("/api/v1"), &handler.Handlers{Admin: &handler.AdminHandlers{Account: h}}, auth, middleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() }), middleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() }), nil, nil)
			req := httptest.NewRequest("POST", "/api/v1/admin/accounts/import/codex-session", strings.NewReader(`{"content":"{\"access_token\":\"test-private-access\",\"refresh_token\":\"test-private-refresh\"}","group_ids":[2]}`))
			req.Header.Set("x-api-key", key)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if allowed {
				require.Equal(t, 200, w.Code, w.Body.String())
				require.Contains(t, w.Body.String(), `"created":1`)
			} else {
				require.Equal(t, 403, w.Code, w.Body.String())
			}
			require.Equal(t, allowed, api.created)
			require.NotContains(t, w.Body.String(), "test-private")
		})
	}
}

type scopedRouteProxyOptions struct {
	service.AdminService
	page, size int
}

func (s *scopedRouteProxyOptions) ListProxyOptions(_ context.Context, page, size int, _ string) ([]service.Proxy, int64, error) {
	s.page = page
	s.size = size
	return []service.Proxy{{ID: 10, Name: "代理候选", Host: "192.0.2.1", Port: 8080, Username: "秘密用户", Password: "秘密密码", Status: service.StatusActive}}, 20001, nil
}
func (s *scopedRouteProxyOptions) SampleProxyOptions(_ context.Context, _ string) ([]service.Proxy, int64, error) {
	s.page = 0
	s.size = 50
	return []service.Proxy{{ID: 12345, Name: "全池候选", Username: "秘密用户", Password: "秘密密码"}}, 20001, nil
}

func TestScopedAdminProxyReadPermissionAndBoundedRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, allowed := range []bool{false, true} {
		t.Run(map[bool]string{false: "网络权限不含列表", true: "独立代理读取"}[allowed], func(t *testing.T) {
			policy := adminaccess.Policy{Rules: []adminaccess.Rule{{Permissions: []string{"accounts.network"}, AllGroups: true, AccountTypes: []string{"oauth"}}}}
			if allowed {
				policy.GlobalPermissions = []string{"proxies.read"}
			}
			settings := service.NewSettingService(nil, nil)
			settings.SetScopedAdminKeyRepository(&scopedRouteKeys{})
			key, _, err := settings.CreateScopedAdminKey(context.Background(), service.ScopedAdminKeyInput{Name: "代理读取", Enabled: true, Policy: policy}, 9)
			require.NoError(t, err)
			users := service.NewUserService(&scopedRouteUsers{}, nil, nil, nil)
			auth := middleware.NewAdminAuthMiddleware(nil, users, settings, nil)
			svc := &scopedRouteProxyOptions{}
			r := gin.New()
			RegisterAdminRoutes(r.Group("/api/v1"), &handler.Handlers{Admin: &handler.AdminHandlers{Proxy: adminhandler.NewProxyHandler(svc)}}, auth, middleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() }), middleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() }), nil, nil)
			for _, path := range []string{"/api/v1/admin/proxies", "/api/v1/admin/proxies/all"} {
				req := httptest.NewRequest("GET", path+"?page=2&page_size=20000", nil)
				req.Header.Set("x-api-key", key)
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				if !allowed {
					require.Equal(t, 403, w.Code, w.Body.String())
					continue
				}
				require.Equal(t, 200, w.Code, w.Body.String())
				require.Equal(t, 2, svc.page)
				require.Equal(t, 100, svc.size)
				require.NotContains(t, w.Body.String(), "username")
				require.NotContains(t, w.Body.String(), "password")
				require.NotContains(t, w.Body.String(), "秘密")
				if strings.HasSuffix(path, "/all") {
					require.Equal(t, "20001", w.Header().Get("X-Total-Count"))
					require.Equal(t, "100", w.Header().Get("X-Page-Size"))
					require.Contains(t, w.Body.String(), `"data":[`)
				} else {
					require.Contains(t, w.Body.String(), `"total":20001`)
					require.Contains(t, w.Body.String(), `"page_size":100`)
				}
			}
			req := httptest.NewRequest("GET", "/api/v1/admin/proxies/all", nil)
			req.Header.Set("x-api-key", key)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if allowed {
				require.Equal(t, 200, w.Code, w.Body.String())
				require.Equal(t, 0, svc.page)
				require.Equal(t, 50, svc.size)
				require.Contains(t, w.Body.String(), `"id":12345`)
				require.NotContains(t, w.Body.String(), "秘密")
			} else {
				require.Equal(t, 403, w.Code)
			}
			for _, path := range []string{"/api/v1/admin/proxies/data", "/api/v1/admin/proxies/10/accounts", "/api/v1/admin/proxies/10/stats"} {
				req := httptest.NewRequest("GET", path, nil)
				req.Header.Set("x-api-key", key)
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				require.Equal(t, 403, w.Code)
			}
		})
	}
}

func TestScopedAdminRegisteredRoutesExcludeForkOnlyEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterAdminRoutes(r.Group("/api/v1"), &handler.Handlers{Admin: &handler.AdminHandlers{}}, middleware.AdminAuthMiddleware(func(c *gin.Context) { c.Next() }), middleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() }), middleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() }), nil, nil)
	paths := map[string]bool{}
	for _, route := range r.Routes() {
		paths[route.Method+" "+route.Path] = true
	}
	for _, path := range []string{"GET /api/v1/admin/model-catalog", "GET /api/v1/admin/proxies/options", "GET /api/v1/admin/groups/:id/models-list-candidates"} {
		require.False(t, paths[path], path)
	}
	for _, path := range []string{"GET /api/v1/admin/proxies", "GET /api/v1/admin/proxies/all", "GET /api/v1/admin/groups/:id/model-allowlist-candidates", "POST /api/v1/admin/accounts/import/codex-session"} {
		require.True(t, paths[path], path)
	}
}
