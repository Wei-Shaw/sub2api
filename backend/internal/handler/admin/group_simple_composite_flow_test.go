package admin_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Exercise real handlers, policy, services, resolver and key authorization.
// Only persistence is in memory: mocking AdminService would miss mode guards.
type simpleFlowGroups struct {
	service.AdminGroupRepository
	rows map[int64]*service.Group
}

func (r *simpleFlowGroups) Create(_ context.Context, g *service.Group) error {
	g.ID = int64(len(r.rows) + 1)
	cp := *g
	r.rows[g.ID] = &cp
	return nil
}
func (r *simpleFlowGroups) Update(_ context.Context, g *service.Group) error {
	cp := *g
	r.rows[g.ID] = &cp
	return nil
}
func (r *simpleFlowGroups) GetByID(_ context.Context, id int64) (*service.Group, error) {
	g := r.rows[id]
	if g == nil {
		return nil, service.ErrGroupNotFound
	}
	cp := *g
	return &cp, nil
}
func (r *simpleFlowGroups) GetByIDLite(ctx context.Context, id int64) (*service.Group, error) {
	return r.GetByID(ctx, id)
}
func (r *simpleFlowGroups) ListActive(_ context.Context) ([]service.Group, error) {
	out := []service.Group{}
	for _, g := range r.rows {
		if g.Status == service.StatusActive {
			out = append(out, *g)
		}
	}
	return out, nil
}
func (r *simpleFlowGroups) ListWithFilters(_ context.Context, p pagination.PaginationParams, platform, status, search string, _ *bool) ([]service.Group, *pagination.PaginationResult, error) {
	rows := []service.Group{}
	for _, g := range r.rows {
		if (platform == "" || g.Platform == platform) && (status == "" || g.Status == status) && (search == "" || strings.Contains(g.Name, search)) {
			rows = append(rows, *g)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	result := &pagination.PaginationResult{Total: int64(len(rows)), Page: p.Page, PageSize: p.PageSize}
	start := (p.Page - 1) * p.PageSize
	if start > len(rows) {
		start = len(rows)
	}
	end := start + p.PageSize
	if end > len(rows) {
		end = len(rows)
	}
	return rows[start:end], result, nil
}

type simpleFlowRoutes struct {
	rows map[int64]*service.CompositeModelRoute
}

func (r *simpleFlowRoutes) ListByGroup(_ context.Context, id int64, disabled bool) ([]service.CompositeModelRoute, error) {
	out := []service.CompositeModelRoute{}
	for _, v := range r.rows {
		if v.GroupID == id && (disabled || v.Enabled) {
			out = append(out, *v)
		}
	}
	return out, nil
}
func (r *simpleFlowRoutes) Create(_ context.Context, v *service.CompositeModelRoute) error {
	v.ID = int64(len(r.rows) + 1)
	cp := *v
	r.rows[v.ID] = &cp
	return nil
}
func (r *simpleFlowRoutes) Update(_ context.Context, v *service.CompositeModelRoute) error {
	cp := *v
	r.rows[v.ID] = &cp
	return nil
}
func (r *simpleFlowRoutes) Delete(_ context.Context, id int64) error { delete(r.rows, id); return nil }
func (r *simpleFlowRoutes) DeleteByGroup(_ context.Context, id int64) error {
	for key, v := range r.rows {
		if v.GroupID == id {
			delete(r.rows, key)
		}
	}
	return nil
}

type simpleFlowUsers struct{ service.UserRepository }

func (*simpleFlowUsers) GetByID(_ context.Context, id int64) (*service.User, error) {
	return &service.User{ID: id, Status: service.StatusActive}, nil
}

type simpleFlowSubscriptions struct {
	service.UserSubscriptionRepository
}

func (*simpleFlowSubscriptions) ListActiveByUserID(context.Context, int64) ([]service.UserSubscription, error) {
	return nil, nil
}

type simpleFlowKeys struct {
	service.APIKeyRepository
	key *service.APIKey
}

func (r *simpleFlowKeys) Create(_ context.Context, key *service.APIKey) error {
	key.ID = 1
	r.key = key
	return nil
}

func TestSimpleCompositeHTTPFlowPreservesBasicGroups(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{RunMode: config.RunModeSimple}
	groups := &simpleFlowGroups{rows: map[int64]*service.Group{}}
	routes := &simpleFlowRoutes{rows: map[int64]*service.CompositeModelRoute{}}
	resolver := service.NewCompositeRouteResolver(routes)
	svc := service.NewAdminService(cfg, nil, groups, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, routes, resolver, nil)
	h := admin.NewGroupHandlerWithConfig(svc, nil, nil, cfg)
	keys := &simpleFlowKeys{}
	keyService := service.NewAPIKeyService(keys, &simpleFlowUsers{}, groups, &simpleFlowSubscriptions{}, nil, nil, cfg)
	keyHandler := handler.NewAPIKeyHandler(keyService)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
		c.Next()
	})
	router.POST("/groups", h.Create)
	router.GET("/groups", h.List)
	router.GET("/groups/all", h.GetAll)
	router.GET("/groups/:id", h.GetByID)
	router.PUT("/groups/:id", h.Update)
	router.GET("/groups/:id/composite-routes", h.ListCompositeRoutes)
	router.POST("/groups/:id/composite-routes", h.CreateCompositeRoute)
	router.PUT("/groups/:id/composite-routes/:route_id", h.UpdateCompositeRoute)
	router.DELETE("/groups/:id/composite-routes/:route_id", h.DeleteCompositeRoute)
	router.POST("/groups/:id/composite-routes/preview", h.PreviewCompositeRoute)
	router.GET("/groups/:id/rate-multipliers", h.GetGroupRateMultipliers)
	router.GET("/available-groups", keyHandler.GetAvailableGroups)
	router.POST("/keys", keyHandler.Create)
	request := func(method, path, body string, status int) map[string]any {
		t.Helper()
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		require.Equal(t, status, w.Code, "%s %s: %s", method, path, w.Body.String())
		var result map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
		return result
	}
	for _, platform := range []string{"openai", "composite"} {
		request("POST", "/groups", fmt.Sprintf(`{"name":"%s","platform":"%s","rate_multiplier":7,"subscription_type":"subscription","daily_limit_usd":10}`, platform, platform), 200)
	}
	require.Equal(t, 1.0, groups.rows[2].RateMultiplier)
	require.Equal(t, service.SubscriptionTypeStandard, groups.rows[2].SubscriptionType)
	require.Nil(t, groups.rows[2].DailyLimitUSD)
	listed := request("GET", "/groups?page_size=1&page=2", "", 200)["data"].(map[string]any)
	require.Equal(t, float64(2), listed["total"])
	require.Equal(t, "composite", listed["items"].([]any)[0].(map[string]any)["platform"])
	require.Len(t, request("GET", "/groups/all", "", 200)["data"].([]any), 2)
	request("PUT", "/groups/1", `{"name":"basic preserved"}`, 200)
	require.Equal(t, "openai", groups.rows[1].Platform)
	request("PUT", "/groups/2", `{"name":"routing renamed","rate_multiplier":9}`, 200)
	require.Equal(t, "routing renamed", groups.rows[2].Name)
	request("GET", "/groups/2", "", 200)
	route := `{"public_model":"team-model","target_platform":"openai","upstream_model":"gpt-5.2","endpoint":"responses","enabled":true}`
	request("POST", "/groups/2/composite-routes", route, 201)
	require.Len(t, request("GET", "/groups/2/composite-routes", "", 200)["data"].([]any), 1)
	preview := request("POST", "/groups/2/composite-routes/preview", `{"model":"team-model","endpoint":"responses"}`, 200)["data"].(map[string]any)
	require.Equal(t, true, preview["matched"])
	require.Equal(t, "gpt-5.2", preview["upstream_model"])
	request("PUT", "/groups/2/composite-routes/1", strings.ReplaceAll(route, "gpt-5.2", "gpt-5.1"), 200)
	decision, err := resolver.Resolve(context.Background(), 2, "team-model", "responses")
	require.NoError(t, err)
	require.Equal(t, "gpt-5.1", decision.UpstreamModel)
	require.Len(t, request("GET", "/available-groups", "", 200)["data"].([]any), 2)
	request("POST", "/keys", `{"name":"composite client","group_id":2}`, 200)
	require.NotNil(t, keys.key.GroupID)
	require.Equal(t, int64(2), *keys.key.GroupID)
	request("GET", "/groups/2/rate-multipliers", "", 403)
	request("DELETE", "/groups/2/composite-routes/1", "", 200)
	preview = request("POST", "/groups/2/composite-routes/preview", `{"model":"team-model","endpoint":"responses"}`, 200)["data"].(map[string]any)
	require.Equal(t, false, preview["matched"], "unknown alias fails closed after route deletion")
}
