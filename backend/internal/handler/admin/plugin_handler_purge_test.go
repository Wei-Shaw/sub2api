//go:build unit

package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/plugin"
	"github.com/gin-gonic/gin"
)

// newPurgeRouter wires every plugin endpoint plus the new DELETE so URL
// param parsing matches admin.go in production.
func newPurgeRouter(h *PluginHandler) *gin.Engine {
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
		g.DELETE("/:name", h.Delete)
	}
	return r
}

func TestDeleteHandler_RequiresPurgeQuery(t *testing.T) {
	t.Parallel()
	mgr := &fakePluginManager{}
	r := newPurgeRouter(NewPluginHandler(mgr))

	body := bytes.NewBufferString(`{"name":"demo"}`)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/plugins/demo", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without purge=true, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestDeleteHandler_BodyNameMustMatchURL(t *testing.T) {
	t.Parallel()
	mgr := &fakePluginManager{}
	r := newPurgeRouter(NewPluginHandler(mgr))

	body := bytes.NewBufferString(`{"name":"other"}`)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/plugins/demo?purge=true", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on body/URL mismatch, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestDeleteHandler_NotSoftUninstalledReturns409(t *testing.T) {
	t.Parallel()
	mgr := &fakePluginManager{
		purgeFn: func(ctx context.Context, name string) error {
			return plugin.ErrPluginNotSoftUninstalled
		},
	}
	r := newPurgeRouter(NewPluginHandler(mgr))

	body := bytes.NewBufferString(`{"name":"demo"}`)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/plugins/demo?purge=true", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 when not soft-uninstalled, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestDeleteHandler_Success(t *testing.T) {
	t.Parallel()
	var called string
	mgr := &fakePluginManager{
		purgeFn: func(ctx context.Context, name string) error {
			called = name
			return nil
		},
	}
	r := newPurgeRouter(NewPluginHandler(mgr))

	body := bytes.NewBufferString(`{"name":"demo"}`)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/plugins/demo?purge=true", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if called != "demo" {
		t.Fatalf("Purge called with %q, want demo", called)
	}
	var resp struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v body=%s", err, w.Body.String())
	}
	if resp.Data["status"] != "purged" {
		t.Fatalf("expected status=purged, got %+v", resp.Data)
	}
}

func TestDeleteHandler_BuiltinReturns400(t *testing.T) {
	t.Parallel()
	mgr := &fakePluginManager{
		purgeFn: func(ctx context.Context, name string) error {
			return plugin.ErrPluginIsBuiltin
		},
	}
	r := newPurgeRouter(NewPluginHandler(mgr))

	body := bytes.NewBufferString(`{"name":"builtin"}`)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/plugins/builtin?purge=true", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for builtin, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestDeleteHandler_ManagerNilReturns503(t *testing.T) {
	t.Parallel()
	r := newPurgeRouter(NewPluginHandler(nil))
	body := bytes.NewBufferString(`{"name":"demo"}`)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/plugins/demo?purge=true", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}
