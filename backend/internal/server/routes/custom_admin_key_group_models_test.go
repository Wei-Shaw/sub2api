//go:build unit

package routes

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type scopedRouteGroupModels struct {
	service.AdminService
	writes []int64
}

func (*scopedRouteGroupModels) GetGroup(_ context.Context, id int64) (*service.Group, error) {
	if id == 99 {
		return nil, service.ErrGroupNotFound
	}
	platform := "openai"
	if id == 4 {
		platform = "anthropic"
	}
	return &service.Group{ID: id, Name: "模型分组", Platform: platform, RateMultiplier: 9, Description: "不可泄露的分组备注", ModelAllowlist: service.GroupModelAllowlist{Enabled: true, Models: []string{"gpt-test"}}}, nil
}

func (*scopedRouteGroupModels) GetGroupModelsListCandidates(context.Context, int64, string) ([]string, error) {
	return []string{"gpt-test", "gpt-default"}, nil
}

func (s *scopedRouteGroupModels) ScopedAdminListGroupModels(ctx context.Context, _ pagination.PaginationParams, _ bool, _, _, _ string) ([]service.Group, int64, error) {
	g, err := s.GetGroup(ctx, 2)
	return []service.Group{*g}, 1, err
}

func (s *scopedRouteGroupModels) ScopedAdminUpdateGroupModels(ctx context.Context, id int64, cfg service.GroupModelAllowlist) (*service.Group, error) {
	s.writes = append(s.writes, id)
	g, err := s.GetGroup(ctx, id)
	if err == nil {
		g.ModelAllowlist = cfg
	}
	return g, err
}

func TestScopedAdminRegisteredGroupModelInterfaces(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, permission := range []string{"groups.models.read", "groups.models.write", "accounts.models.write", "models.defaults.read", ""} {
		t.Run(permission, func(t *testing.T) {
			keys := &scopedRouteKeys{}
			settings := service.NewSettingService(nil, nil)
			settings.SetScopedAdminKeyRepository(keys)
			policy := adminaccess.Policy{}
			if permission != "" {
				policy.Rules = []adminaccess.Rule{{Permissions: []string{permission}, GroupIDs: []int64{2}, AccountTypes: []string{"oauth"}, Platforms: []string{"openai"}}}
			}
			if permission == "models.defaults.read" {
				policy = adminaccess.Policy{GlobalPermissions: []string{permission}}
			}
			key, _, err := settings.CreateScopedAdminKey(context.Background(), service.ScopedAdminKeyInput{Name: "分组模型接口", Enabled: true, Policy: policy}, 9)
			require.NoError(t, err)
			users := service.NewUserService(&scopedRouteUsers{}, nil, nil, nil)
			auth := middleware.NewAdminAuthMiddleware(nil, users, settings, nil)
			api := &scopedRouteGroupModels{}
			r := gin.New()
			RegisterAdminRoutes(r.Group("/api/v1"), &handler.Handlers{Admin: &handler.AdminHandlers{Group: adminhandler.NewGroupHandler(api, nil, nil)}}, auth, middleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() }), middleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() }), nil, nil)
			for _, tc := range []struct {
				method, path, body, allowed string
			}{
				{"GET", "/api/v1/admin/groups", "", "groups.models.read"},
				{"GET", "/api/v1/admin/groups/all", "", "groups.models.read"},
				{"GET", "/api/v1/admin/groups/2", "", "groups.models.read"},
				{"GET", "/api/v1/admin/groups/2/model-allowlist-candidates", "", "groups.models.read"},
				{"GET", "/api/v1/admin/groups/0/model-allowlist-candidates?platform=openai", "", "models.defaults.read"},
				{"PUT", "/api/v1/admin/groups/2", `{"model_allowlist":{"enabled":true,"models":["gpt-test"]}}`, "groups.models.write"},
			} {
				req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
				req.Header.Set("x-api-key", key)
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				want := 403
				if permission == tc.allowed {
					want = 200
				}
				if permission == "models.defaults.read" && strings.Contains(tc.path, "/groups/2/") {
					want = 404 // 默认目录权限不允许读取现有分组中的账户模型。
				}
				require.Equal(t, want, w.Code, tc.method+" "+tc.path+": "+w.Body.String())
				require.NotContains(t, w.Body.String(), "不可泄露")
				require.NotContains(t, w.Body.String(), "models_list_config")
				require.NotContains(t, w.Body.String(), "display_only")
				if want == 200 {
					require.Contains(t, w.Body.String(), "gpt-test")
				}
			}
		})
	}
}

func TestScopedAdminGroupModelsRejectUnauthorizedGroupsAndMixedFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	api := &scopedRouteGroupModels{}
	h := adminhandler.NewGroupHandler(api, nil, nil)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(adminaccess.WithPrincipal(c.Request.Context(), &adminaccess.Principal{Policy: adminaccess.Policy{Rules: []adminaccess.Rule{{Permissions: []string{"groups.models.read", "groups.models.write"}, GroupIDs: []int64{2, 4}, Platforms: []string{"openai"}}}}}))
	})
	r.Use(h.ScopedAdminKeyGroupModels)
	r.GET("/api/v1/admin/groups/:id", h.GetByID)
	r.GET("/api/v1/admin/groups/:id/model-allowlist-candidates", h.GetGroupModelAllowlistCandidates)
	r.PUT("/api/v1/admin/groups/:id", h.Update)
	for _, tc := range []struct {
		method, path, body string
		want               int
	}{
		{"GET", "/api/v1/admin/groups/3", "", 404},
		{"GET", "/api/v1/admin/groups/4", "", 404},
		{"GET", "/api/v1/admin/groups/99", "", 404},
		{"GET", "/api/v1/admin/groups/3/model-allowlist-candidates", "", 404},
		{"GET", "/api/v1/admin/groups/0/model-allowlist-candidates", "", 403},
		{"GET", "/api/v1/admin/groups/2/model-allowlist-candidates?platform=anthropic", "", 400},
		{"PUT", "/api/v1/admin/groups/3", `{"model_allowlist":{"models":["x"]}}`, 404},
		{"PUT", "/api/v1/admin/groups/4", `{"model_allowlist":{"models":["x"]}}`, 404},
		{"PUT", "/api/v1/admin/groups/2", `{"rate_multiplier":1,"model_allowlist":{"models":["x"]}}`, 400},
		{"PUT", "/api/v1/admin/groups/2", `{"copy_accounts_from_group_ids":[3]}`, 400},
		{"PUT", "/api/v1/admin/groups/2", `{"status":"inactive"}`, 400},
		{"PUT", "/api/v1/admin/groups/2", `{"model_allowlist":{},"models_list_config":{}}`, 400},
		{"PUT", "/api/v1/admin/groups/2", `{"model_allowlist":{"foo":true}}`, 400},
		{"PUT", "/api/v1/admin/groups/2", `{"model_allowlist":null}`, 400},
		{"PUT", "/api/v1/admin/groups/2", `{"models_list_config":{"enabled":true,"models":["x"]}}`, 400},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, tc.want, w.Code, tc.method+" "+tc.path+": "+w.Body.String())
		require.NotContains(t, w.Body.String(), "不可泄露")
		require.NotContains(t, w.Body.String(), "模型分组")
	}
	require.Empty(t, api.writes)
}

type scopedRouteAllowlistRepository struct {
	service.AdminGroupRepository
	config service.GroupModelAllowlist
	writes int
}

func (r *scopedRouteAllowlistRepository) GetByID(_ context.Context, id int64) (*service.Group, error) {
	return &service.Group{ID: id, Platform: service.PlatformOpenAI, ModelAllowlist: r.config}, nil
}
func (r *scopedRouteAllowlistRepository) ScopedAdminUpdateGroupModels(ctx context.Context, id int64, cfg service.GroupModelAllowlist) (*service.Group, error) {
	r.config = cfg
	r.writes++
	return r.GetByID(ctx, id)
}

// 注册路由与真实管理服务共同验证白名单准入语义，仓储替身仅代替持久化。
func TestScopedAdminRegisteredGroupModelsUseUpstreamAllowlistSemantics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &scopedRouteAllowlistRepository{}
	api := service.NewAdminService(nil, nil, repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	settings := service.NewSettingService(nil, nil)
	settings.SetScopedAdminKeyRepository(&scopedRouteKeys{})
	key, _, err := settings.CreateScopedAdminKey(context.Background(), service.ScopedAdminKeyInput{Name: "白名单验证", Enabled: true, Policy: adminaccess.Policy{Rules: []adminaccess.Rule{{Permissions: []string{"groups.models.read", "groups.models.write"}, GroupIDs: []int64{2}, Platforms: []string{service.PlatformOpenAI}}}}}, 9)
	require.NoError(t, err)
	auth := middleware.NewAdminAuthMiddleware(nil, service.NewUserService(&scopedRouteUsers{}, nil, nil, nil), settings, nil)
	r := gin.New()
	RegisterAdminRoutes(r.Group("/api/v1"), &handler.Handlers{Admin: &handler.AdminHandlers{Group: adminhandler.NewGroupHandler(api, nil, nil)}}, auth, middleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() }), middleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() }), nil, nil)
	call := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("PUT", "/api/v1/admin/groups/2", strings.NewReader(body))
		req.Header.Set("x-api-key", key)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	for _, body := range []string{`{"model_allowlist":{"enabled":true,"models":[]}}`, `{"models_list_config":{"enabled":true,"models":["gpt-allowed"]}}`} {
		w := call(body)
		require.Equal(t, 400, w.Code, w.Body.String())
		require.Zero(t, repo.writes)
	}
	w := call(`{"model_allowlist":{"enabled":true,"models":[" GPT-Allowed ","gpt-allowed","other-*"]}}`)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, 1, repo.writes)
	require.Equal(t, []string{"GPT-Allowed", "other-*"}, repo.config.Models)
	require.True(t, repo.config.Allows("gpt-allowed"))
	require.True(t, repo.config.Allows("other-model"))
	require.False(t, repo.config.Allows("blocked-model"))
	require.Contains(t, w.Body.String(), `"model_allowlist"`)
	require.NotContains(t, w.Body.String(), "models_list_config")
	require.NotContains(t, w.Body.String(), "display_only")
}
