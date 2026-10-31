package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/managementmcp"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func mcpRouteHandlers() *handler.Handlers {
	// Nil method receivers are sufficient for route registration. Individual
	// behavior tests populate only the handlers whose dependencies they exercise.
	return &handler.Handlers{
		Admin:    &handler.AdminHandlers{},
		MCPOAuth: handler.NewMCPOAuthHandler(nil, nil, nil, nil, &config.Config{MCP: config.MCPConfig{PublicURL: "https://platform.example/api/v1/mcp"}}),
	}
}

func mcpRouteNext(c *gin.Context) { c.Next() }

func TestMCPToolCatalogTargetsRegisteredManagementRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	h := mcpRouteHandlers()
	require.NotPanics(t, func() {
		RegisterUserRoutes(v1, h, middleware.JWTAuthMiddleware(mcpRouteNext), middleware.AuditLogMiddleware(mcpRouteNext), nil, nil)
		RegisterAdminRoutes(v1, h, middleware.AdminAuthMiddleware(mcpRouteNext), middleware.AuditLogMiddleware(mcpRouteNext), middleware.StepUpAuthMiddleware(mcpRouteNext), nil, nil)
	})
	registered := map[string]bool{}
	for _, route := range router.Routes() {
		registered[route.Method+" "+route.Path] = true
	}
	catalog := managementmcp.ToolCatalog()
	require.NotEmpty(t, catalog)
	for _, tool := range catalog {
		require.True(t, registered[tool.Method+" "+tool.Path], "%s targets missing route %s %s", tool.Name, tool.Method, tool.Path)
	}

	// Exercise the real factory as well: transport setup must not introduce
	// duplicate Gin paths or panic while building its permission-filtered servers.
	mcpRouter := gin.New()
	require.NotPanics(t, func() {
		RegisterMCPRoutes(mcpRouter.Group("/api/v1"), h, middleware.AuditLogMiddleware(mcpRouteNext), middleware.StepUpAuthMiddleware(mcpRouteNext), nil, nil, nil, nil)
	})
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		w := httptest.NewRecorder()
		mcpRouter.ServeHTTP(w, httptest.NewRequest(method, "/api/v1/mcp", nil))
		require.Equal(t, http.StatusUnauthorized, w.Code, method)
	}
}

type mcpRouteTokenAuth struct{ principal *service.MCPPrincipal }

func (a mcpRouteTokenAuth) Authenticate(context.Context, string) (*service.MCPPrincipal, error) {
	copy := *a.principal
	copy.Resource = "https://platform.example/api/v1/mcp"
	return &copy, nil
}

func newMCPRouteTestEndpoint(h *handler.Handlers, principal *service.MCPPrincipal, settings *service.SettingService, audit middleware.AuditLogMiddleware) *gin.Engine {
	inner := gin.New()
	v1 := inner.Group("/api/v1")
	RegisterUserRoutes(v1, h, middleware.JWTAuthMiddleware(managementmcp.DispatchAuth(false)), audit, settings, nil)
	RegisterAdminRoutes(v1, h, middleware.AdminAuthMiddleware(managementmcp.DispatchAuth(true)), audit, middleware.StepUpAuthMiddleware(mcpRouteNext), settings, nil)
	outer := gin.New()
	outer.POST("/api/v1/mcp", managementmcp.Authenticate(mcpRouteTokenAuth{principal: principal}, nil, h.MCPOAuth.ResourceURL), gin.WrapH(managementmcp.NewHandler(inner, nil)))
	return outer
}

type mcpRouteToolPayload struct {
	HTTPStatus int             `json:"http_status"`
	Result     json.RawMessage `json:"result"`
}

type mcpRouteToolResult struct {
	Result struct {
		IsError bool `json:"isError"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		StructuredContent json.RawMessage `json:"structuredContent"`
		// Payload is decoded from the single text block that carries the result.
		Payload mcpRouteToolPayload `json:"-"`
	} `json:"result"`
	Error json.RawMessage `json:"error"`
}

// decodeMCPRouteToolResult requires exactly one JSON text block, so results are
// never duplicated into structuredContent.
func decodeMCPRouteToolResult(t *testing.T, raw []byte) mcpRouteToolResult {
	t.Helper()
	var result mcpRouteToolResult
	require.NoError(t, json.Unmarshal(raw, &result), string(raw))
	require.Empty(t, result.Error, string(raw))
	require.Empty(t, result.Result.StructuredContent, string(raw))
	require.Len(t, result.Result.Content, 1, string(raw))
	require.Equal(t, "text", result.Result.Content[0].Type)
	require.NoError(t, json.Unmarshal([]byte(result.Result.Content[0].Text), &result.Result.Payload), string(raw))
	return result
}

func callMCPRouteTool(t *testing.T, endpoint http.Handler, name string, args map[string]any) mcpRouteToolResult {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": name, "arguments": args},
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", "2025-06-18")
	w := httptest.NewRecorder()
	endpoint.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	return decodeMCPRouteToolResult(t, w.Body.Bytes())
}

type mcpRouteAPIKeyRepo struct{ service.APIKeyRepository }

func (r *mcpRouteAPIKeyRepo) GetByID(context.Context, int64) (*service.APIKey, error) {
	return &service.APIKey{ID: 9, UserID: 7, Name: "owner-key", Key: "must-not-leak", Status: service.StatusActive}, nil
}

func TestMCPDispatchRetainsRealAPIKeyOwnershipAndAudit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := mcpRouteHandlers()
	h.APIKey = handler.NewAPIKeyHandler(service.NewAPIKeyService(&mcpRouteAPIKeyRepo{}, nil, nil, nil, nil, nil, nil))
	for _, userID := range []int64{7, 8} {
		principal := &service.MCPPrincipal{GrantID: "test", UserID: userID, Role: service.RoleUser, Scope: service.MCPScopeUser, ReadOnly: true}
		auditCalls := 0
		audit := middleware.AuditLogMiddleware(func(c *gin.Context) {
			subject, ok := middleware.GetAuthSubjectFromContext(c)
			require.True(t, ok)
			require.Equal(t, userID, subject.UserID)
			require.Equal(t, "/api/v1/keys/:id", c.FullPath())
			require.Equal(t, managementmcp.AuthMethod, c.GetString("auth_method"))
			auditCalls++
			c.Next()
		})
		result := callMCPRouteTool(t, newMCPRouteTestEndpoint(h, principal, nil, audit), "user_get_api_key", map[string]any{"id": 9})
		require.Equal(t, 1, auditCalls)
		require.NotContains(t, string(result.Result.Payload.Result), "must-not-leak")
		if userID == 7 {
			require.False(t, result.Result.IsError)
			require.Equal(t, http.StatusOK, result.Result.Payload.HTTPStatus)
			require.Contains(t, string(result.Result.Payload.Result), "owner-key")
		} else {
			require.True(t, result.Result.IsError)
			require.Equal(t, http.StatusNotFound, result.Result.Payload.HTTPStatus)
			require.NotContains(t, string(result.Result.Payload.Result), "owner-key")
		}
	}
}

type mcpRouteSettingsRepo struct {
	service.SettingRepository
	values map[string]string
}

func (r *mcpRouteSettingsRepo) GetValue(_ context.Context, key string) (string, error) {
	if value, ok := r.values[key]; ok {
		return value, nil
	}
	return "", service.ErrSettingNotFound
}

func (r *mcpRouteSettingsRepo) SetMultiple(_ context.Context, values map[string]string) error {
	for key, value := range values {
		r.values[key] = value
	}
	return nil
}

func TestMCPDispatchRetainsBackendModeAndAdminCompliance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	settings := service.NewSettingService(&mcpRouteSettingsRepo{values: map[string]string{}}, &config.Config{})
	// UpdateSettings also updates the global backend-mode cache. Reset it on
	// cleanup so this integration test cannot affect unrelated route tests.
	require.NoError(t, settings.UpdateSettings(context.Background(), &service.SystemSettings{BackendModeEnabled: true}))
	t.Cleanup(func() {
		require.NoError(t, settings.UpdateSettings(context.Background(), &service.SystemSettings{BackendModeEnabled: false}))
	})
	h := mcpRouteHandlers()
	user := &service.MCPPrincipal{GrantID: "user", UserID: 7, Role: service.RoleUser, Scope: service.MCPScopeUser, ReadOnly: true}
	result := callMCPRouteTool(t, newMCPRouteTestEndpoint(h, user, settings, middleware.AuditLogMiddleware(mcpRouteNext)), "user_get_profile", map[string]any{})
	require.True(t, result.Result.IsError)
	require.Equal(t, http.StatusForbidden, result.Result.Payload.HTTPStatus)
	require.Contains(t, string(result.Result.Payload.Result), "Backend mode is active")

	admin := &service.MCPPrincipal{GrantID: "admin", UserID: 8, Role: service.RoleAdmin, Scope: service.MCPScopeAdmin, ReadOnly: true}
	result = callMCPRouteTool(t, newMCPRouteTestEndpoint(h, admin, settings, middleware.AuditLogMiddleware(mcpRouteNext)), "admin_get_dashboard", map[string]any{})
	require.True(t, result.Result.IsError)
	require.Equal(t, http.StatusLocked, result.Result.Payload.HTTPStatus)
	require.Contains(t, string(result.Result.Payload.Result), "ADMIN_COMPLIANCE_ACK_REQUIRED")
}
