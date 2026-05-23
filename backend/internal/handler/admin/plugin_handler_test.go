//go:build unit

package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/plugin"
)

// fakePluginManager implements PluginManager for routing-level tests. Each
// hook is set per test to verify wiring (path param parsing, error mapping,
// query parsing) without spinning up a real plugin subsystem.
type fakePluginManager struct {
	listFn        func(ctx context.Context) ([]plugin.PluginInfo, error)
	listExtFn     func(ctx context.Context, includeUninstalled bool) ([]plugin.PluginInfo, error)
	getFn         func(ctx context.Context, name string) (*plugin.PluginInfo, error)
	enableFn      func(ctx context.Context, name string) error
	disableFn     func(ctx context.Context, name string) error
	restartFn     func(ctx context.Context, name string) error
	updateCfgFn   func(ctx context.Context, name string, cfg map[string]any) error
	uninstallFn   func(ctx context.Context, name string) error
	installFn     func(ctx context.Context, name string) error
	purgeFn       func(ctx context.Context, name string) error
	removeFilesFn func(ctx context.Context, name string) error
}

func (f *fakePluginManager) List(ctx context.Context) ([]plugin.PluginInfo, error) {
	if f.listFn != nil {
		return f.listFn(ctx)
	}
	return nil, nil
}
func (f *fakePluginManager) ListExt(ctx context.Context, includeUninstalled bool) ([]plugin.PluginInfo, error) {
	if f.listExtFn != nil {
		return f.listExtFn(ctx, includeUninstalled)
	}
	return nil, nil
}
func (f *fakePluginManager) Get(ctx context.Context, name string) (*plugin.PluginInfo, error) {
	if f.getFn != nil {
		return f.getFn(ctx, name)
	}
	return nil, nil
}
func (f *fakePluginManager) Enable(ctx context.Context, name string) error {
	if f.enableFn != nil {
		return f.enableFn(ctx, name)
	}
	return nil
}
func (f *fakePluginManager) Disable(ctx context.Context, name string) error {
	if f.disableFn != nil {
		return f.disableFn(ctx, name)
	}
	return nil
}
func (f *fakePluginManager) Restart(ctx context.Context, name string) error {
	if f.restartFn != nil {
		return f.restartFn(ctx, name)
	}
	return nil
}
func (f *fakePluginManager) UpdateConfig(ctx context.Context, name string, cfg map[string]any) error {
	if f.updateCfgFn != nil {
		return f.updateCfgFn(ctx, name, cfg)
	}
	return nil
}
func (f *fakePluginManager) Uninstall(ctx context.Context, name string) error {
	if f.uninstallFn != nil {
		return f.uninstallFn(ctx, name)
	}
	return nil
}
func (f *fakePluginManager) Install(ctx context.Context, name string) error {
	if f.installFn != nil {
		return f.installFn(ctx, name)
	}
	return nil
}
func (f *fakePluginManager) Purge(ctx context.Context, name string) error {
	if f.purgeFn != nil {
		return f.purgeFn(ctx, name)
	}
	return nil
}
func (f *fakePluginManager) RemoveFiles(ctx context.Context, name string) error {
	if f.removeFilesFn != nil {
		return f.removeFilesFn(ctx, name)
	}
	return nil
}

// newPluginRouter wires the same routes admin.go does so URL params parse
// the same way they would in production.
func newPluginRouter(h *PluginHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := r.Group("/api/v1/admin/plugins")
	{
		g.GET("", h.List)
		g.GET("/:name", h.Get)
		g.POST("/:name/enable", h.Enable)
		g.POST("/:name/disable", h.Disable)
		g.POST("/:name/restart", h.Restart)
		g.PUT("/:name/config", h.UpdateConfig)
		g.POST("/:name/uninstall", h.Uninstall)
		g.POST("/:name/install", h.Install)
	}
	return r
}

func TestUninstallHandler_Success(t *testing.T) {
	t.Parallel()
	var called string
	mgr := &fakePluginManager{
		uninstallFn: func(ctx context.Context, name string) error {
			called = name
			return nil
		},
	}
	r := newPluginRouter(NewPluginHandler(mgr))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/plugins/demo/uninstall", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if called != "demo" {
		t.Fatalf("Uninstall called with %q, want demo", called)
	}
	var resp struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v body=%s", err, w.Body.String())
	}
	if resp.Data["mode"] != "soft" || resp.Data["status"] != "uninstalled" {
		t.Fatalf("unexpected response data: %+v", resp.Data)
	}
}

func TestUninstallHandler_BuiltinReturns400(t *testing.T) {
	t.Parallel()
	mgr := &fakePluginManager{
		uninstallFn: func(ctx context.Context, name string) error {
			return plugin.ErrPluginIsBuiltin
		},
	}
	r := newPluginRouter(NewPluginHandler(mgr))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/plugins/builtin/uninstall", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestUninstallHandler_NotFoundReturns404(t *testing.T) {
	t.Parallel()
	mgr := &fakePluginManager{
		uninstallFn: func(ctx context.Context, name string) error {
			return plugin.ErrPluginNotFound
		},
	}
	r := newPluginRouter(NewPluginHandler(mgr))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/plugins/ghost/uninstall", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestInstallHandler_RoundTrip(t *testing.T) {
	t.Parallel()
	var called string
	mgr := &fakePluginManager{
		installFn: func(ctx context.Context, name string) error {
			called = name
			return nil
		},
	}
	r := newPluginRouter(NewPluginHandler(mgr))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/plugins/demo/install", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if called != "demo" {
		t.Fatalf("Install called with %q, want demo", called)
	}
}

func TestListHandler_PassesIncludeUninstalled(t *testing.T) {
	t.Parallel()
	cases := []struct {
		query string
		want  bool
	}{
		{"", false},
		{"?include_uninstalled=true", true},
		{"?include_uninstalled=1", true},
		{"?include_uninstalled=yes", true},
		{"?include_uninstalled=on", true},
		{"?include_uninstalled=false", false},
		{"?include_uninstalled=0", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(strings.TrimPrefix(tc.query, "?"), func(t *testing.T) {
			t.Parallel()
			var got bool
			mgr := &fakePluginManager{
				listExtFn: func(ctx context.Context, includeUninstalled bool) ([]plugin.PluginInfo, error) {
					got = includeUninstalled
					return []plugin.PluginInfo{}, nil
				},
			}
			r := newPluginRouter(NewPluginHandler(mgr))
			req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/plugins"+tc.query, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
			}
			if got != tc.want {
				t.Fatalf("query %q → includeUninstalled=%v, want %v", tc.query, got, tc.want)
			}
		})
	}
}

func TestUninstallHandler_InvalidNameReturns400(t *testing.T) {
	t.Parallel()
	mgr := &fakePluginManager{}
	r := newPluginRouter(NewPluginHandler(mgr))

	// Path parameter must satisfy IsValidPluginName; uppercase letters fail.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/plugins/INVALID/uninstall", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid name, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestUninstallHandler_ManagerNilReturns503(t *testing.T) {
	t.Parallel()
	r := newPluginRouter(NewPluginHandler(nil))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/plugins/demo/uninstall", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}
