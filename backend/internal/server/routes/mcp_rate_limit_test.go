package routes

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newMCPRateLimitEndpoint(t *testing.T, role string, rpm int) (*gin.Engine, string, *miniredis.Miniredis) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	settingsJSON, err := json.Marshal(service.PanelRateLimitSettings{Enabled: true, UserRPM: rpm, HeavyRPM: 1})
	require.NoError(t, err)
	cfg := &config.Config{MCP: config.MCPConfig{PublicURL: "https://mcp.example/api/v1/mcp"}}
	settings := service.NewSettingService(&mcpRouteSettingsRepo{values: map[string]string{
		service.SettingKeyPanelRateLimitSettings: string(settingsJSON),
	}}, cfg)
	oauth, _, _ := newMCPRouteOAuthService(role)
	scope := service.MCPScopeUser
	if role == service.RoleAdmin {
		scope = service.MCPScopeAdmin
	}
	token := issueMCPRouteOAuthToken(t, oauth, cfg.MCP.PublicURL, scope, true)
	h := mcpRouteHandlers()
	h.MCPOAuth = handler.NewMCPOAuthHandler(oauth, nil, nil, settings, cfg)
	h.APIKey = handler.NewAPIKeyHandler(service.NewAPIKeyService(&mcpRouteAPIKeyRepo{}, nil, nil, nil, nil, nil, nil))
	router := gin.New()
	RegisterMCPRoutes(router.Group("/api/v1"), h, middleware.AuditLogMiddleware(mcpRouteNext),
		middleware.StepUpAuthMiddleware(mcpRouteNext), settings, middleware.NewPanelRateLimiter(rdb, settings), nil, rdb)
	return router, token, mr
}

func mcpRateLimitRPC(t *testing.T, endpoint http.Handler, token, method string, params any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", "2025-06-18")
	w := httptest.NewRecorder()
	endpoint.ServeHTTP(w, req)
	return w
}

func TestMCPToolCallsConsumeGlobalBudgetOnce(t *testing.T) {
	for _, tc := range []struct {
		role, tool string
		args       map[string]any
		status     int
	}{
		{service.RoleUser, "user_get_api_key", map[string]any{"id": 9}, http.StatusOK},
		// The compliance response proves the request passed the admin limiter.
		{service.RoleAdmin, "admin_get_dashboard", map[string]any{}, http.StatusLocked},
	} {
		t.Run(tc.role, func(t *testing.T) {
			endpoint, token, mr := newMCPRateLimitEndpoint(t, tc.role, 2)
			params := map[string]any{"name": tc.tool, "arguments": tc.args}
			for i := 0; i < 2; i++ {
				w := mcpRateLimitRPC(t, endpoint, token, "tools/call", params)
				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
				result := decodeMCPRouteToolResult(t, w.Body.Bytes())
				require.Equal(t, tc.status, result.Result.Payload.HTTPStatus, w.Body.String())
			}
			count, err := mr.Get("rate_limit:panel:global:user:7")
			require.NoError(t, err)
			require.Equal(t, "2", count)
			w := mcpRateLimitRPC(t, endpoint, token, "tools/call", params)
			require.Equal(t, http.StatusTooManyRequests, w.Code, w.Body.String())
		})
	}
}

func TestMCPDiscoveryHasItsOwnBoundedBudget(t *testing.T) {
	endpoint, token, mr := newMCPRateLimitEndpoint(t, service.RoleUser, 2)
	for i := 0; i < 2; i++ {
		w := mcpRateLimitRPC(t, endpoint, token, "tools/list", map[string]any{})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	}
	require.False(t, mr.Exists("rate_limit:panel:global:user:7"), "discovery must not spend the shared REST budget")
	w := mcpRateLimitRPC(t, endpoint, token, "tools/list", map[string]any{})
	require.Equal(t, http.StatusTooManyRequests, w.Code, w.Body.String())
}

func TestMCPDispatchRetainsHeavyRateLimit(t *testing.T) {
	endpoint, token, mr := newMCPRateLimitEndpoint(t, service.RoleUser, 10)
	params := map[string]any{"name": "user_get_usage_stats", "arguments": map[string]any{
		// A malformed date reaches the real handler without needing a usage DB.
		"query": map[string]any{"start_date": "2026-99-99"},
	}}
	for _, status := range []int{http.StatusBadRequest, http.StatusTooManyRequests} {
		w := mcpRateLimitRPC(t, endpoint, token, "tools/call", params)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		result := decodeMCPRouteToolResult(t, w.Body.Bytes())
		require.Equal(t, status, result.Result.Payload.HTTPStatus, w.Body.String())
	}
	count, err := mr.Get("rate_limit:panel:global:user:7")
	require.NoError(t, err)
	require.Equal(t, "2", count)
}
