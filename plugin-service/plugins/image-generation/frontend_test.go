package imagegeneration

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFrontendServesMinimalPluginHost(t *testing.T) {
	mux := http.NewServeMux()
	RegisterFrontend(mux)

	req := httptest.NewRequest(http.MethodGet, "/plugins/image-generation", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("frontend status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	body := rec.Body.String()
	for _, needle := range []string{
		`/plugins/image-generation/assets/app.js`,
		`/plugins/image-generation/assets/app.css`,
		`data-plugin-api-base="/plugins/image-generation/api"`,
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("frontend html missing host marker %q", needle)
		}
	}

	for _, forbidden := range []string{
		`installBatchTrackingFetchBridge`,
		`installHistoryDeleteFetchObserver`,
		`MutationObserver`,
		`document.addEventListener`,
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("frontend html contains runtime application behavior %q", forbidden)
		}
	}
}

func TestFrontendServesGeneratedAssets(t *testing.T) {
	mux := http.NewServeMux()
	RegisterFrontend(mux)

	for _, asset := range []struct {
		path        string
		contentType string
	}{
		{path: "/plugins/image-generation/assets/app.js", contentType: "javascript"},
		{path: "/plugins/image-generation/assets/app.css", contentType: "text/css"},
	} {
		req := httptest.NewRequest(http.MethodGet, asset.path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("asset %s status = %d, want %d", asset.path, rec.Code, http.StatusOK)
		}
		if !strings.Contains(rec.Header().Get("Content-Type"), asset.contentType) {
			t.Fatalf("asset %s content type = %q, want %q", asset.path, rec.Header().Get("Content-Type"), asset.contentType)
		}
		if rec.Body.Len() == 0 {
			t.Fatalf("asset %s is empty", asset.path)
		}
	}
}

func TestGeneratedFrontendAssetsExist(t *testing.T) {
	for _, name := range []string{"app.js", "app.css"} {
		info, err := os.Stat(filepath.Join(webRoot(), "assets", name))
		if err != nil {
			t.Fatalf("generated asset %s: %v", name, err)
		}
		if info.Size() == 0 {
			t.Fatalf("generated asset %s is empty", name)
		}
	}
}
