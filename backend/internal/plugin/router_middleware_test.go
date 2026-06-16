//go:build unit

package plugin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	pluginsdkroot "github.com/Wei-Shaw/sub2api/plugin-sdk"
)

func newAuthCtx(t *testing.T, fn func(c *gin.Context)) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/x", nil)
	c.Request.Header.Set("X-Forwarded-For", "203.0.113.42")
	if fn != nil {
		fn(c)
	}
	return c
}

func TestInjectRequestContext_FullAuthCtx(t *testing.T) {
	authCtx := newAuthCtx(t, func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
		c.Set(string(middleware.ContextKeyUserRole), "admin")
		c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 1234})
	})

	req := httptest.NewRequest(http.MethodGet, "/plugin/x", nil)
	req = injectRequestContext(req, authCtx, "channel-management")

	if got := req.Header.Get(HeaderPluginUserID); got != "7" {
		t.Fatalf("user id = %q", got)
	}
	if got := req.Header.Get(HeaderPluginUserRole); got != "admin" {
		t.Fatalf("role = %q", got)
	}
	if got := req.Header.Get(HeaderPluginAPIKeyID); got != "1234" {
		t.Fatalf("api key id = %q", got)
	}
	if got := req.Header.Get(HeaderPluginName); got != "channel-management" {
		t.Fatalf("plugin name = %q", got)
	}
	if got := req.Header.Get(HeaderPluginRequestID); got == "" {
		t.Fatalf("request id should be auto-generated")
	}
	tp := req.Header.Get(HeaderTraceparent)
	if tp == "" || !pluginsdkroot.IsValidTraceparent(tp) {
		t.Fatalf("traceparent should be valid, got %q", tp)
	}
	if ip := req.Header.Get(HeaderPluginClientIP); ip == "" {
		t.Fatalf("client ip should be set, got empty")
	}
}

func TestInjectRequestContext_TraceparentPassThrough(t *testing.T) {
	const validTP = "00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01"
	req := httptest.NewRequest(http.MethodGet, "/plugin/x", nil)
	req.Header.Set(HeaderTraceparent, validTP)
	req = injectRequestContext(req, nil, "p")

	if got := req.Header.Get(HeaderTraceparent); got != validTP {
		t.Fatalf("valid traceparent should pass through, got %q", got)
	}
}

func TestInjectRequestContext_InvalidTraceparentRewritten(t *testing.T) {
	const bogus = "garbage-trace"
	req := httptest.NewRequest(http.MethodGet, "/plugin/x", nil)
	req.Header.Set(HeaderTraceparent, bogus)
	req = injectRequestContext(req, nil, "p")

	got := req.Header.Get(HeaderTraceparent)
	if got == bogus {
		t.Fatalf("invalid traceparent leaked through")
	}
	if !pluginsdkroot.IsValidTraceparent(got) {
		t.Fatalf("rewritten traceparent invalid: %q", got)
	}
}

func TestInjectRequestContext_RequestIDPassThrough(t *testing.T) {
	const incoming = "11111111-2222-3333-4444-555555555555"
	req := httptest.NewRequest(http.MethodGet, "/plugin/x", nil)
	req.Header.Set(HeaderPluginRequestID, incoming)
	req = injectRequestContext(req, nil, "p")

	if got := req.Header.Get(HeaderPluginRequestID); got != incoming {
		t.Fatalf("request id should pass through, got %q", got)
	}
}

func TestInjectRequestContext_NoAuthCtxStillSetsName(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/plugin/public", nil)
	req = injectRequestContext(req, nil, "hello-world")

	if got := req.Header.Get(HeaderPluginName); got != "hello-world" {
		t.Fatalf("plugin name missing, got %q", got)
	}
	if req.Header.Get(HeaderPluginUserID) != "" {
		t.Fatalf("user id should be empty for unauth request")
	}
	if req.Header.Get(HeaderPluginAPIKeyID) != "" {
		t.Fatalf("api key id should be empty for unauth request")
	}
	if req.Header.Get(HeaderTraceparent) == "" {
		t.Fatalf("traceparent should still be generated")
	}
}

// TestInjectRequestContext_StampsTraceparentOnCtx covers the T21 fix:
// HTTP middleware must populate req.Context() through pluginsdkroot.WithTraceparent
// so any host-side handler / nested gRPC call observes the same id via
// TraceparentFromContext (matching the gRPC server interceptor's behaviour).
// Before T21 only req.Header was populated and the ctx stayed untouched,
// breaking trace propagation for HTTP-driven plugin endpoints.
func TestInjectRequestContext_StampsTraceparentOnCtx(t *testing.T) {
	const validTP = "00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01"
	req := httptest.NewRequest(http.MethodGet, "/plugin/x", nil)
	req.Header.Set(HeaderTraceparent, validTP)
	req = injectRequestContext(req, nil, "p")

	tp, ok := pluginsdkroot.TraceparentFromContext(req.Context())
	if !ok {
		t.Fatalf("expected traceparent on ctx, got none")
	}
	if tp != validTP {
		t.Fatalf("ctx traceparent = %q, want %q", tp, validTP)
	}
}

// TestInjectRequestContext_GeneratedTraceparentOnCtx verifies the fresh-mint
// path also stamps ctx, not just the wire header.
func TestInjectRequestContext_GeneratedTraceparentOnCtx(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/plugin/x", nil)
	req = injectRequestContext(req, nil, "p")

	header := req.Header.Get(HeaderTraceparent)
	if header == "" || !pluginsdkroot.IsValidTraceparent(header) {
		t.Fatalf("header traceparent invalid: %q", header)
	}
	tp, ok := pluginsdkroot.TraceparentFromContext(req.Context())
	if !ok {
		t.Fatalf("ctx traceparent not stamped despite header=%q", header)
	}
	if tp != header {
		t.Fatalf("ctx traceparent = %q, want %q (must match header)", tp, header)
	}
}

// TestEnsureTraceparentOnRequest_Idempotent verifies that calling the helper
// twice on the same request keeps the original traceparent: ServeHTTP feeds
// it pre-auth and injectRequestContext re-runs it post-auth, so the second
// invocation must not regenerate or churn ctx.
func TestEnsureTraceparentOnRequest_Idempotent(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/plugin/x", nil)
	first := ensureTraceparentOnRequest(req)
	tp1 := first.Header.Get(HeaderTraceparent)
	ctx1, ok1 := pluginsdkroot.TraceparentFromContext(first.Context())
	if !ok1 || tp1 == "" || tp1 != ctx1 {
		t.Fatalf("first call must set both header and ctx (header=%q ctx=%q ok=%v)", tp1, ctx1, ok1)
	}

	second := ensureTraceparentOnRequest(first)
	tp2 := second.Header.Get(HeaderTraceparent)
	ctx2, ok2 := pluginsdkroot.TraceparentFromContext(second.Context())
	if tp2 != tp1 {
		t.Fatalf("second call regenerated header: %q -> %q", tp1, tp2)
	}
	if !ok2 || ctx2 != ctx1 {
		t.Fatalf("second call mutated ctx traceparent: %q -> %q", ctx1, ctx2)
	}
	// Same ctx must be returned untouched when traceparent already matches —
	// otherwise we waste an allocation per request after auth.
	if second != first {
		t.Fatalf("second call should reuse the request when ctx already matches")
	}
}

// TestPluginRouter_TraceparentVisibleToFailingAuth covers the T28 fix:
// trace must be injected into req.Context() BEFORE auth middleware runs so
// that 401/403 paths can log a consistent trace_id. We register a fake
// auth handler that aborts with 401 and snapshots req.Context() at entry —
// the snapshot must already contain the traceparent.
func TestPluginRouter_TraceparentVisibleToFailingAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var capturedTrace string
	var capturedOK bool
	failingAuth := func(c *gin.Context) {
		// Snapshot what the auth middleware sees the moment it is invoked.
		capturedTrace, capturedOK = pluginsdkroot.TraceparentFromContext(c.Request.Context())
		c.AbortWithStatus(http.StatusUnauthorized)
	}

	core := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("core handler should not be called for plugin route")
		w.WriteHeader(http.StatusInternalServerError)
	})
	pr := NewPluginRouter(core, nil, failingAuth, nil)
	pr.SwapRouteTable(NewRouteTable().AddPlugin("demo", []RouteEntry{{
		Method:     "*",
		PathPrefix: "/api/v1/plugin/demo/",
		AuthType:   AuthTypeAdmin,
		ProxyURL:   "http://127.0.0.1:1",
	}}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/plugin/demo/foo", nil)
	pr.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 from failing auth, got %d", rec.Code)
	}
	if !capturedOK {
		t.Fatalf("auth handler saw no traceparent in ctx; trace must be injected before auth")
	}
	if !pluginsdkroot.IsValidTraceparent(capturedTrace) {
		t.Fatalf("auth handler saw invalid traceparent %q", capturedTrace)
	}
}

// TestPluginRouter_TraceparentPassThroughToAuth ensures an upstream-supplied
// valid traceparent is the exact value the auth middleware observes — guards
// against the helper accidentally regenerating a fresh id between ServeHTTP
// and the auth handler.
func TestPluginRouter_TraceparentPassThroughToAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const validTP = "00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01"
	var seenTrace string
	captureAuth := func(c *gin.Context) {
		seenTrace, _ = pluginsdkroot.TraceparentFromContext(c.Request.Context())
		c.Next()
	}

	pr := NewPluginRouter(http.NotFoundHandler(), nil, captureAuth, nil)
	pr.SwapRouteTable(NewRouteTable().AddPlugin("demo", []RouteEntry{{
		Method:     "*",
		PathPrefix: "/api/v1/plugin/demo/",
		AuthType:   AuthTypeAdmin,
		ProxyURL:   "http://127.0.0.1:1", // unreachable, request will fail at proxy stage
	}}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/plugin/demo/foo", nil)
	req.Header.Set(HeaderTraceparent, validTP)
	pr.ServeHTTP(rec, req)

	if seenTrace != validTP {
		t.Fatalf("auth handler ctx traceparent = %q, want %q", seenTrace, validTP)
	}
}

// backendModeRouter 构造一个挂上 user-facing payment plugin 路由的
// PluginRouter, 鉴权用一个 stub middleware: 把固定 role 写入 gin.Context
// 以便 BackendMode 守卫能读到。enableBackendMode 控制 checker 是否返回 true。
func backendModeRouter(t *testing.T, role string, enableBackendMode bool) *PluginRouter {
	t.Helper()
	gin.SetMode(gin.TestMode)
	stubAuth := func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
		c.Set(string(middleware.ContextKeyUserRole), role)
		c.Next()
	}
	pr := NewPluginRouter(http.NotFoundHandler(), stubAuth, stubAuth, stubAuth)
	pr.SetBackendModeChecker(func(ctx context.Context) bool { return enableBackendMode })
	return pr
}

func backendModeMountPaymentRoute(pr *PluginRouter, prefix, authType string) {
	pr.SwapRouteTable(NewRouteTable().AddPlugin("payment", []RouteEntry{{
		Method:     "*",
		PathPrefix: prefix,
		AuthType:   authType,
		ProxyURL:   "http://127.0.0.1:1", // unreachable; should not be reached when blocked
	}}))
}

// TestPluginRouter_BackendModeBlocksUserOnUserFacingRoute is the BUG #66
// regression: when backend_mode_enabled=true a normal user POSTing to
// /api/v1/payment/orders must be 403'd before the request is proxied to the
// payment plugin process.
func TestPluginRouter_BackendModeBlocksUserOnUserFacingRoute(t *testing.T) {
	pr := backendModeRouter(t, "user", true)
	backendModeMountPaymentRoute(pr, "/api/v1/payment/", AuthTypeUser)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payment/orders", nil)
	pr.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 from backend mode guard, got %d body=%q", rec.Code, rec.Body.String())
	}
}

// TestPluginRouter_BackendModeAllowsAdminOnUserFacingRoute confirms the same
// user-facing route stays open for admin role even with backend mode on,
// matching the legacy BackendModeUserGuard semantics.
func TestPluginRouter_BackendModeAllowsAdminOnUserFacingRoute(t *testing.T) {
	pr := backendModeRouter(t, "admin", true)
	backendModeMountPaymentRoute(pr, "/api/v1/payment/", AuthTypeUser)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payment/orders", nil)
	pr.ServeHTTP(rec, req)

	// Admin passes the guard; the request then fails at the unreachable proxy.
	// Either 502 (bad gateway) or any non-403 indicates the guard did not block.
	if rec.Code == http.StatusForbidden {
		t.Fatalf("admin must not be blocked by backend mode, got 403 body=%q", rec.Body.String())
	}
}

// TestPluginRouter_BackendModeNeverBlocksAdminPath confirms /api/v1/admin/*
// plugin routes are exempt from the user-facing guard regardless of role.
// Admin authn middleware is the gatekeeper for those endpoints.
func TestPluginRouter_BackendModeNeverBlocksAdminPath(t *testing.T) {
	pr := backendModeRouter(t, "user", true)
	backendModeMountPaymentRoute(pr, "/api/v1/admin/payment/", AuthTypeAdmin)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/payment/dashboard", nil)
	pr.ServeHTTP(rec, req)

	if rec.Code == http.StatusForbidden {
		t.Fatalf("admin-prefix routes must skip backend mode guard, got 403 body=%q", rec.Body.String())
	}
}

// TestPluginRouter_BackendModeDisabledLetsUserThrough confirms that when
// the host has no checker installed (or it returns false) the guard is a
// no-op — preserves test/proxy behaviour for environments that never opt in.
func TestPluginRouter_BackendModeDisabledLetsUserThrough(t *testing.T) {
	pr := backendModeRouter(t, "user", false)
	backendModeMountPaymentRoute(pr, "/api/v1/payment/", AuthTypeUser)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payment/orders", nil)
	pr.ServeHTTP(rec, req)

	if rec.Code == http.StatusForbidden {
		t.Fatalf("user must pass when backend mode is disabled, got 403 body=%q", rec.Body.String())
	}
}
