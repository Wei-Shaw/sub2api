package frontendhost

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegisterHostedPluginInjectsAuthBridgeAndServesAssets(t *testing.T) {
	webRoot := t.TempDir()
	assetRoot := filepath.Join(webRoot, "assets")
	if err := os.MkdirAll(assetRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webRoot, "index.html"), []byte(`<!doctype html><html><head><script type="module" crossorigin src="/plugins/demo/assets/app.js"></script><link rel="stylesheet" crossorigin href="/plugins/demo/assets/app.css"></head><body></body></html>`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetRoot, "app.js"), []byte(`console.log("demo")`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetRoot, "app.css"), []byte(`body{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	RegisterHostedPlugin(mux, HostedPluginOptions{
		PluginKey: "demo",
		WebRoot:   webRoot,
	})

	pageReq := httptest.NewRequest(http.MethodGet, "/plugins/demo", nil)
	pageRec := httptest.NewRecorder()
	mux.ServeHTTP(pageRec, pageReq)
	if pageRec.Code != http.StatusOK {
		t.Fatalf("page status = %d, want %d; body=%s", pageRec.Code, http.StatusOK, pageRec.Body.String())
	}

	pageBody := pageRec.Body.String()
	for _, needle := range []string{
		`localStorage.getItem("auth_token")`,
		`window.location.search`,
		`Authorization`,
		`/\/plugins\/[^/?#]+\/api(?:\/|$)/`,
		`/plugins/demo/assets/app.js?v=`,
		`/plugins/demo/assets/app.css?v=`,
	} {
		if !strings.Contains(pageBody, needle) {
			t.Fatalf("page missing %q", needle)
		}
	}

	assetReq := httptest.NewRequest(http.MethodGet, "/plugins/demo/assets/app.js", nil)
	assetRec := httptest.NewRecorder()
	mux.ServeHTTP(assetRec, assetReq)
	if assetRec.Code != http.StatusOK {
		t.Fatalf("asset status = %d, want %d; body=%s", assetRec.Code, http.StatusOK, assetRec.Body.String())
	}
	if !strings.Contains(assetRec.Body.String(), `"demo"`) {
		t.Fatalf("asset body = %q", assetRec.Body.String())
	}
}

func TestRegisterHostedPluginRedirectsTrailingSlashToPage(t *testing.T) {
	webRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(webRoot, "index.html"), []byte(`<!doctype html><html><head></head><body></body></html>`), 0o644); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	RegisterHostedPlugin(mux, HostedPluginOptions{PluginKey: "demo", WebRoot: webRoot})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/plugins/demo/", nil))

	if rec.Code != http.StatusPermanentRedirect {
		t.Fatalf("trailing slash status = %d, want %d", rec.Code, http.StatusPermanentRedirect)
	}
	if location := rec.Header().Get("Location"); location != "/plugins/demo" {
		t.Fatalf("redirect location = %q, want %q", location, "/plugins/demo")
	}
}

func TestRegisterHostedPluginInjectsFaviconFromResolver(t *testing.T) {
	webRoot := t.TempDir()
	assetRoot := filepath.Join(webRoot, "assets")
	if err := os.MkdirAll(assetRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webRoot, "index.html"), []byte(`<!doctype html><html><head><title>demo</title></head><body></body></html>`), 0o644); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	RegisterHostedPlugin(mux, HostedPluginOptions{
		PluginKey: "demo",
		WebRoot:   webRoot,
		FaviconResolver: func(*http.Request) string {
			return "/brand/logo.png"
		},
	})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/plugins/demo", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("page status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `rel="icon"`) {
		t.Fatalf("page missing favicon link: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `href="/brand/logo.png"`) {
		t.Fatalf("page missing resolver favicon: %s", rec.Body.String())
	}
}

func TestRegisterHostedPluginFallsBackToDefaultFavicon(t *testing.T) {
	webRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(webRoot, "index.html"), []byte(`<!doctype html><html><head><title>demo</title></head><body></body></html>`), 0o644); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	RegisterHostedPlugin(mux, HostedPluginOptions{
		PluginKey: "demo",
		WebRoot:   webRoot,
		FaviconResolver: func(*http.Request) string {
			return ""
		},
	})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/plugins/demo", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("page status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `href="/logo.png"`) {
		t.Fatalf("page missing default favicon fallback: %s", rec.Body.String())
	}
}

func TestRegisterHostedPluginInjectsMenuTitleFromResolver(t *testing.T) {
	webRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(webRoot, "index.html"), []byte(`<!doctype html><html><head><title>demo</title></head><body></body></html>`), 0o644); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	RegisterHostedPlugin(mux, HostedPluginOptions{
		PluginKey: "image-generation",
		WebRoot:   webRoot,
		PageTitleResolver: func(*http.Request) string {
			return "测试"
		},
	})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/plugins/image-generation", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("page status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `<title>测试</title>`) {
		t.Fatalf("page missing menu title: %s", rec.Body.String())
	}
}

func TestRegisterHostedPluginPreservesOriginalTitleWhenResolverEmpty(t *testing.T) {
	webRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(webRoot, "index.html"), []byte(`<!doctype html><html><head><title>demo</title></head><body></body></html>`), 0o644); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	RegisterHostedPlugin(mux, HostedPluginOptions{
		PluginKey: "image-generation",
		WebRoot:   webRoot,
		PageTitleResolver: func(*http.Request) string {
			return ""
		},
	})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/plugins/image-generation", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("page status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `<title>demo</title>`) {
		t.Fatalf("page title should keep original value: %s", rec.Body.String())
	}
}

func TestRegisterHostedPluginRequiresIndexHTML(t *testing.T) {
	webRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(webRoot, "plugin-image-generation.html"), []byte(`legacy`), 0o644); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	RegisterHostedPlugin(mux, HostedPluginOptions{
		PluginKey: "demo",
		WebRoot:   webRoot,
	})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/plugins/demo", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("page status = %d, want %d; body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}
