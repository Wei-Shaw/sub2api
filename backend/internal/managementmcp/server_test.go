package managementmcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type testTokenAuth struct {
	mu         sync.Mutex
	principals map[string]*service.MCPPrincipal
}

func (a *testTokenAuth) Authenticate(_ context.Context, token string) (*service.MCPPrincipal, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if p := a.principals[token]; p != nil {
		copy := *p
		if copy.Resource == "" {
			copy.Resource = "https://platform.example/api/v1/mcp"
		}
		return &copy, nil
	}
	return nil, service.ErrMCPOAuthInvalidToken
}

func newTestEndpoint(dispatcher http.Handler) (*gin.Engine, *testTokenAuth) {
	gin.SetMode(gin.TestMode)
	auth := &testTokenAuth{principals: map[string]*service.MCPPrincipal{
		"reader":       {UserID: 1, GrantID: "one", Role: service.RoleUser, Scope: service.MCPScopeUser, ReadOnly: true},
		"writer":       {UserID: 1, GrantID: "two", Role: service.RoleUser, Scope: service.MCPScopeUser},
		"other":        {UserID: 2, GrantID: "three", Role: service.RoleUser, Scope: service.MCPScopeUser},
		"admin":        {UserID: 3, GrantID: "four", Role: service.RoleAdmin, Scope: service.MCPScopeAdmin},
		"admin-reader": {UserID: 3, GrantID: "five", Role: service.RoleAdmin, Scope: service.MCPScopeAdmin, ReadOnly: true},
	}}
	r := gin.New()
	r.Any("/api/v1/mcp", Authenticate(auth, []string{"https://console.example"}, func(context.Context) (string, error) {
		return "https://platform.example/api/v1/mcp", nil
	}), gin.WrapH(NewHandler(dispatcher, []string{"https://console.example"})))
	return r, auth
}

func rpc(t *testing.T, handler http.Handler, token, method string, params any) (int, map[string]any) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	require.NoError(t, err)
	req := httptest.NewRequest("POST", "http://platform.example/api/v1/mcp", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", "2025-06-18")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	var result map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result), w.Body.String())
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	return w.Code, result
}

func rpcObject(t *testing.T, value any) map[string]any {
	t.Helper()
	object, ok := value.(map[string]any)
	require.True(t, ok, "expected JSON object, got %T", value)
	return object
}

func toolNames(t *testing.T, result map[string]any) []string {
	t.Helper()
	var names []string
	tools, ok := rpcObject(t, result["result"])["tools"].([]any)
	require.True(t, ok)
	for _, tool := range tools {
		name, ok := rpcObject(t, tool)["name"].(string)
		require.True(t, ok)
		names = append(names, name)
	}
	return names
}

func TestMCPInitializeAndPermissionFilteredDiscovery(t *testing.T) {
	r, _ := newTestEndpoint(http.NotFoundHandler())
	status, initialized := rpc(t, r, "reader", "initialize", map[string]any{
		"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test", "version": "1"},
	})
	require.Equal(t, 200, status)
	require.Equal(t, "sub2api-management", rpcObject(t, rpcObject(t, initialized["result"])["serverInfo"])["name"])
	for _, tc := range []struct {
		token              string
		hasAdmin, hasWrite bool
	}{
		{"reader", false, false}, {"writer", false, true}, {"admin", true, true}, {"admin-reader", true, false},
	} {
		t.Run(tc.token, func(t *testing.T) {
			status, listed := rpc(t, r, tc.token, "tools/list", map[string]any{})
			require.Equal(t, 200, status)
			names := toolNames(t, listed)
			// Verify the permission boundary, not an exact catalogue size.
			require.Contains(t, names, "user_get_profile")
			if tc.hasAdmin {
				require.Contains(t, names, "admin_get_dashboard")
			} else {
				require.NotContains(t, names, "admin_get_dashboard")
			}
			if tc.hasWrite {
				require.Contains(t, names, "user_create_api_key")
			} else {
				require.NotContains(t, names, "user_create_api_key")
			}
		})
	}
}

func TestMCPRejectsUnauthorizedToolsAndUnknownArgumentsBeforeDispatch(t *testing.T) {
	calls := 0
	r, _ := newTestEndpoint(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	for _, tc := range []struct {
		token, name string
		args        any
	}{
		{"reader", "user_create_api_key", map[string]any{"body": map[string]any{"name": "test"}}},
		{"writer", "admin_list_users", map[string]any{}},
		{"writer", "user_get_api_key", map[string]any{"id": "../admin/users"}},
		{"writer", "user_get_api_key", map[string]any{"id": -1}},
		{"writer", "user_list_api_keys", map[string]any{"query": map[string]any{"user_id": 2}}},
		{"writer", "user_list_api_keys", map[string]any{"query": map[string]any{"page_size": 101}}},
		{"writer", "user_create_api_key", map[string]any{"body": map[string]any{"name": "test", "user_id": 2}}},
		{"admin", "admin_update_user", map[string]any{"id": 2, "body": map[string]any{"role": "admin"}}},
		{"admin", "admin_update_account", map[string]any{"id": 2, "body": map[string]any{"credentials": map[string]any{"api_key": "x"}}}},
	} {
		_, result := rpc(t, r, tc.token, "tools/call", map[string]any{"name": tc.name, "arguments": tc.args})
		require.Contains(t, result, "error", tc.name)
	}
	require.Zero(t, calls)
}

func TestMCPDispatchPreservesOwnershipAndRedactsExistingSecrets(t *testing.T) {
	inner := gin.New()
	inner.GET("/api/v1/keys/:id", DispatchAuth(false), func(c *gin.Context) {
		subject, ok := middleware.GetAuthSubjectFromContext(c)
		require.True(t, ok)
		require.Empty(t, c.GetHeader("Authorization"))
		require.Equal(t, AuthMethod, c.GetString("auth_method"))
		if subject.UserID != 1 {
			c.JSON(403, gin.H{"message": "Key belongs to another user"})
			return
		}
		c.JSON(200, gin.H{"data": gin.H{"id": 7, "name": "key name", "key": "existing-secret", "usage": gin.H{"access_token": "nested-secret"}}})
	})
	inner.POST("/api/v1/keys", DispatchAuth(false), func(c *gin.Context) {
		c.JSON(201, gin.H{"data": gin.H{"id": 8, "key": "new-secret", "group": gin.H{"key": "unrelated-secret"}}})
	})
	r, auth := newTestEndpoint(inner)
	for _, token := range []string{"writer", "other"} {
		_, result := rpc(t, r, token, "tools/call", map[string]any{"name": "user_get_api_key", "arguments": map[string]any{"id": 7}})
		encoded, _ := json.Marshal(result)
		require.NotContains(t, string(encoded), "existing-secret")
		require.NotContains(t, string(encoded), "nested-secret")
		if token == "other" {
			require.Equal(t, true, rpcObject(t, result["result"])["isError"])
		} else {
			require.Contains(t, string(encoded), "key name")
		}
	}
	_, created := rpc(t, r, "writer", "tools/call", map[string]any{"name": "user_create_api_key", "arguments": map[string]any{"idempotency_key": "create-test-1", "body": map[string]any{"name": "created"}}})
	encoded, _ := json.Marshal(created)
	require.Contains(t, string(encoded), "new-secret")
	require.NotContains(t, string(encoded), "unrelated-secret")
	// Tokens are verified on every HTTP request, including after initialization.
	auth.mu.Lock()
	delete(auth.principals, "writer")
	auth.mu.Unlock()
	status, _ := rpc(t, r, "writer", "tools/list", map[string]any{})
	require.Equal(t, 401, status)
}

func TestMCPDispatchAuthCannotBeForgedOrUsedForAnotherRoute(t *testing.T) {
	inner := gin.New()
	inner.GET("/api/v1/user/mcp/grants", DispatchAuth(false), func(c *gin.Context) { t.Error("unexpected credential access") })
	req := httptest.NewRequest("GET", "/api/v1/user/mcp/grants", nil)
	req.Header.Set("Authorization", "Bearer writer")
	w := httptest.NewRecorder()
	inner.ServeHTTP(w, req)
	require.Equal(t, 403, w.Code)
	identity := &authenticatedRequest{principal: &service.MCPPrincipal{UserID: 1, Scope: "user"}, request: req}
	ctx := context.WithValue(req.Context(), requestKey{}, identity)
	ctx = context.WithValue(ctx, dispatchKey{}, ToolSpec{Path: "/api/v1/user/profile", Method: "GET", ReadOnly: true})
	w = httptest.NewRecorder()
	inner.ServeHTTP(w, req.WithContext(ctx))
	require.Equal(t, 403, w.Code)
}

func TestMCPPreservesLargeIntegerQueryFilters(t *testing.T) {
	var observed string
	r, _ := newTestEndpoint(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observed = r.URL.Query().Get("group_id")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	_, result := rpc(t, r, "writer", "tools/call", map[string]any{"name": "user_list_api_keys", "arguments": map[string]any{"query": map[string]any{"group_id": 12345678}}})
	require.NotContains(t, result, "error")
	require.Equal(t, "12345678", observed)
}

func TestMCPAccountStatusFiltersMatchManagementAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var observed []string
	inner := gin.New()
	inner.GET("/api/v1/admin/accounts", DispatchAuth(true), func(c *gin.Context) {
		observed = append(observed, c.Query("status"))
		c.JSON(http.StatusOK, gin.H{"data": []any{}})
	})
	r, _ := newTestEndpoint(inner)
	for _, status := range []string{"active", "inactive", "error", "rate_limited", "temp_unschedulable", "unschedulable"} {
		t.Run(status, func(t *testing.T) {
			before := len(observed)
			_, result := rpc(t, r, "admin-reader", "tools/call", map[string]any{
				"name": "admin_list_accounts", "arguments": map[string]any{"query": map[string]any{"status": status}},
			})
			require.NotContains(t, result, "error")
			require.Len(t, observed, before+1)
			require.Equal(t, status, observed[before])
		})
	}
	t.Run("reject unsupported overloaded filter", func(t *testing.T) {
		before := len(observed)
		_, result := rpc(t, r, "admin-reader", "tools/call", map[string]any{
			"name": "admin_list_accounts", "arguments": map[string]any{"query": map[string]any{"status": "overloaded"}},
		})
		require.Contains(t, result, "error")
		require.Len(t, observed, before, "unsupported status must be rejected before dispatch")
	})
}

func TestMCPIdempotencyKeysAreRequiredAndIsolatedPerCredential(t *testing.T) {
	var keys []string
	r, _ := newTestEndpoint(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		_, _ = w.Write([]byte(`{"data":{}}`))
	}))
	args := map[string]any{"body": map[string]any{"name": "created"}}
	_, missing := rpc(t, r, "writer", "tools/call", map[string]any{"name": "user_create_api_key", "arguments": args})
	require.Contains(t, missing, "error")
	require.Empty(t, keys)
	args["idempotency_key"] = "intent-123"
	for _, token := range []string{"writer", "writer", "other"} {
		_, result := rpc(t, r, token, "tools/call", map[string]any{"name": "user_create_api_key", "arguments": args})
		require.NotContains(t, result, "error")
	}
	require.Len(t, keys, 3)
	require.NotEmpty(t, keys[0])
	require.Equal(t, keys[0], keys[1])
	require.NotEqual(t, keys[0], keys[2])
}

func TestMCPOriginAuthenticationAndRequestLimits(t *testing.T) {
	r, _ := newTestEndpoint(http.NotFoundHandler())
	for _, tc := range []struct {
		origin, token string
		size, status  int
	}{
		{"https://attacker.example", "reader", 0, 403}, {"null", "reader", 0, 403},
		{"https://console.example", "invalid", 0, 401}, {"", "", 0, 401},
		{"https://platform.example", "reader", maxRequestBytes + 1, 413},
	} {
		req := httptest.NewRequest("POST", "http://platform.example/api/v1/mcp", strings.NewReader(strings.Repeat("x", tc.size)))
		req.Header.Set("Origin", tc.origin)
		if tc.token != "" {
			req.Header.Set("Authorization", "Bearer "+tc.token)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, tc.status, w.Code, tc.origin)
	}
	for _, method := range []string{"GET", "DELETE"} {
		req := httptest.NewRequest(method, "http://platform.example/api/v1/mcp", nil)
		req.Header.Set("Authorization", "Bearer reader")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, 405, w.Code)
	}
}

// The SDK's own cross-origin and Content-Type checks must agree with
// Authenticate: allowlisted browser origins and JSON with parameters succeed.
func TestMCPTransportAcceptsAuthenticatedOriginsAndJSONMediaTypes(t *testing.T) {
	r, _ := newTestEndpoint(http.NotFoundHandler())
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`
	for _, tc := range []struct {
		name, origin, site, contentType string
		status                          int
	}{
		{"native", "", "", "application/json", 200},
		{"same host", "https://platform.example", "same-origin", "application/json", 200},
		{"allowlisted origin", "https://console.example", "cross-site", "application/json", 200},
		{"allowlisted origin without fetch metadata", "https://console.example", "", "application/json", 200},
		{"unlisted origin", "https://attacker.example", "cross-site", "application/json", 403},
		{"json with charset", "", "", "application/json; charset=utf-8", 200},
		{"text body", "", "", "text/plain", 415},
		{"form body", "", "", "application/x-www-form-urlencoded", 415},
	} {
		req := httptest.NewRequest("POST", "http://platform.example/api/v1/mcp", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer reader")
		req.Header.Set("Content-Type", tc.contentType)
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("MCP-Protocol-Version", "2025-06-18")
		if tc.origin != "" {
			req.Header.Set("Origin", tc.origin)
		}
		if tc.site != "" {
			req.Header.Set("Sec-Fetch-Site", tc.site)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, tc.status, w.Code, "%s: %s", tc.name, w.Body.String())
	}
}

func TestMCPResultLimitAndCancellation(t *testing.T) {
	r, _ := newTestEndpoint(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", maxResultBytes+1)))
	}))
	_, result := rpc(t, r, "reader", "tools/call", map[string]any{"name": "user_get_profile", "arguments": map[string]any{}})
	require.Equal(t, true, rpcObject(t, result["result"])["isError"])

	started, finished := make(chan struct{}), make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	original := httptest.NewRequest("POST", "/api/v1/mcp", nil).WithContext(ctx)
	identity := &authenticatedRequest{principal: &service.MCPPrincipal{UserID: 1, Scope: "user"}, request: original}
	// Simulate an SDK context detached from the HTTP lifetime.
	detached := context.WithValue(context.Background(), requestKey{}, identity)
	go func() {
		defer close(finished)
		executeTool(detached, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			close(started)
			<-r.Context().Done()
		}), ToolSpec{Path: "/api/v1/user/profile", Method: "GET", ReadOnly: true}, nil)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("dispatcher did not start")
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("HTTP cancellation did not reach dispatcher")
	}
}

func TestMCPRejectsJSONRPCBatchesBeforeDispatch(t *testing.T) {
	calls := 0
	r, _ := newTestEndpoint(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"data":{}}`))
	}))
	call := `{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"user_get_profile","arguments":{}}}`
	list := `{"jsonrpc":"2.0","id":%d,"method":"tools/list"}`
	batch := "[" + strings.Join([]string{fmt.Sprintf(call, 1), fmt.Sprintf(list, 2), fmt.Sprintf(call, 3)}, ",") + "]"
	for _, tc := range []struct{ name, body, version string }{
		// Without a protocol header the SDK assumes 2025-03-26, which allows batches.
		{"legacy batch", batch, ""},
		{"current batch", batch, "2025-06-18"},
		{"whitespace batch", " \r\n\t" + batch, ""},
		{"scalar", `"tools/list"`, ""},
		{"empty", " ", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "http://platform.example/api/v1/mcp", strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer admin")
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			if tc.version != "" {
				req.Header.Set("MCP-Protocol-Version", tc.version)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		})
	}
	require.Zero(t, calls)
	// A single request without the protocol header is still served.
	req := httptest.NewRequest("POST", "http://platform.example/api/v1/mcp", strings.NewReader(fmt.Sprintf(call, 1)))
	req.Header.Set("Authorization", "Bearer admin")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, 1, calls)
}

func TestMCPToolResultIsReturnedOnceAndBoundedAfterEncoding(t *testing.T) {
	payload := `{"data":{"name":"ok"}}`
	r, _ := newTestEndpoint(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(payload))
	}))
	_, result := rpc(t, r, "reader", "tools/call", map[string]any{"name": "user_get_profile", "arguments": map[string]any{}})
	toolResult := rpcObject(t, result["result"])
	require.NotContains(t, toolResult, "structuredContent", "results must not be duplicated")
	content, ok := toolResult["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 1)
	text, ok := rpcObject(t, content[0])["text"].(string)
	require.True(t, ok)
	require.JSONEq(t, `{"http_status":200,"result":{"data":{"name":"ok"}}}`, text)

	// Under the capture limit as received, but HTML escaping expands it 6x.
	payload = `{"data":"` + strings.Repeat("<", maxResultBytes/3) + `"}`
	_, result = rpc(t, r, "reader", "tools/call", map[string]any{"name": "user_get_profile", "arguments": map[string]any{}})
	toolResult = rpcObject(t, result["result"])
	require.Equal(t, true, toolResult["isError"])
	encoded, _ := json.Marshal(toolResult)
	require.Less(t, len(encoded), 1<<10)
}

func TestMCPRedactsEveryAuthoritativeCredentialKey(t *testing.T) {
	data := map[string]any{"name": "visible"}
	nested := map[string]any{}
	for _, key := range service.SensitiveCredentialKeys {
		data[key] = "canary-" + key
		nested[strings.ToUpper(key)] = "canary-upper-" + key
	}
	data["credentials"] = map[string]any{"nested": nested}
	data["items"] = []any{nested}
	value := map[string]any{"data": data}
	redactResult(value, false, "")
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "canary")
	require.Contains(t, string(encoded), "visible")
}
