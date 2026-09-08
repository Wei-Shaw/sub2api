package airelay

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFrontendServesRelayConfigurationPage(t *testing.T) {
	mux := http.NewServeMux()
	RegisterFrontend(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/plugins/ai-relay", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	for _, marker := range []string{
		`/plugins/ai-relay-assets/app.js`,
		`/plugins/ai-relay-assets/app.css`,
		`data-plugin-api-base="/plugins/ai-relay/api"`,
	} {
		if !strings.Contains(rec.Body.String(), marker) {
			t.Fatalf("page is missing %q", marker)
		}
	}
	for _, forbidden := range []string{`/admin/ai-relay`, `window.location.replace`} {
		if strings.Contains(rec.Body.String(), forbidden) {
			t.Fatalf("page contains redirect %q", forbidden)
		}
	}
}

func TestFrontendServesGeneratedRelayAssets(t *testing.T) {
	mux := http.NewServeMux()
	RegisterFrontend(mux)
	for _, asset := range []struct {
		path        string
		contentType string
	}{
		{path: "/plugins/ai-relay-assets/app.js", contentType: "javascript"},
		{path: "/plugins/ai-relay-assets/app.css", contentType: "text/css"},
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, asset.path, nil))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Type"), asset.contentType) || rec.Body.Len() == 0 {
			t.Fatalf("asset %s response = %d %q %d", asset.path, rec.Code, rec.Header().Get("Content-Type"), rec.Body.Len())
		}
	}
}
