package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type brandingAssetProviderFunc func(context.Context, string) ([]byte, string, bool)

func (f brandingAssetProviderFunc) GetSiteBrandingAsset(ctx context.Context, path string) ([]byte, string, bool) {
	return f(ctx, path)
}

func brandingTestRouter(provider BrandingAssetProvider) *gin.Engine {
	router := gin.New()
	router.GET("/api/v1/settings/branding/:asset", ServeBrandingAsset(provider))
	router.HEAD("/api/v1/settings/branding/:asset", ServeBrandingAsset(provider))
	return router
}

func TestServeBrandingAsset(t *testing.T) {
	const path = "/api/v1/settings/branding/logo-abc.svg"
	for _, tc := range []struct {
		name        string
		content     []byte
		contentType string
	}{
		{name: "svg", content: []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`), contentType: "image/svg+xml"},
		{name: "png", content: []byte{0x89, 'P', 'N', 'G', 0, 0xff, 0x01}, contentType: "image/png"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, method := range []string{http.MethodGet, http.MethodHead} {
				t.Run(method, func(t *testing.T) {
					called := false
					provider := brandingAssetProviderFunc(func(_ context.Context, requestedPath string) ([]byte, string, bool) {
						called = true
						assert.Equal(t, path, requestedPath)
						return tc.content, tc.contentType, true
					})
					w := httptest.NewRecorder()
					brandingTestRouter(provider).ServeHTTP(w, httptest.NewRequest(method, path, nil))

					require.True(t, called)
					assert.Equal(t, http.StatusOK, w.Code)
					assert.Equal(t, tc.contentType, w.Header().Get("Content-Type"))
					assert.Equal(t, "public, max-age=31536000, immutable", w.Header().Get("Cache-Control"))
					csp := w.Header().Get("Content-Security-Policy")
					assert.Contains(t, strings.Split(csp, "; "), "default-src 'none'")
					assert.Contains(t, strings.Split(csp, "; "), "sandbox")
					assert.NotContains(t, csp, "script-src")
					assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
					if method == http.MethodHead {
						assert.Empty(t, w.Body.Bytes())
						assert.Equal(t, strconv.Itoa(len(tc.content)), w.Header().Get("Content-Length"))
					} else {
						assert.Equal(t, tc.content, w.Body.Bytes())
					}
				})
			}
		})
	}
}

func TestServeBrandingAssetNotFound(t *testing.T) {
	const path = "/api/v1/settings/branding/logo-missing.svg"
	for _, tc := range []struct {
		name     string
		provider BrandingAssetProvider
	}{
		{name: "missing_asset", provider: brandingAssetProviderFunc(func(_ context.Context, requestedPath string) ([]byte, string, bool) {
			assert.Equal(t, path, requestedPath)
			return nil, "", false
		})},
		{name: "nil_provider"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, method := range []string{http.MethodGet, http.MethodHead} {
				t.Run(method, func(t *testing.T) {
					w := httptest.NewRecorder()
					brandingTestRouter(tc.provider).ServeHTTP(w, httptest.NewRequest(method, path, nil))
					assert.Equal(t, http.StatusNotFound, w.Code)
					assert.Empty(t, w.Body.Bytes())
					assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
				})
			}
		})
	}
}
