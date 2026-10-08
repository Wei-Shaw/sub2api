//go:build unit

package handler

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var brandingPNG = []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x01, 0x02, 0x03}

// brandingHandlerRepoStub supports both the public-settings read and the
// on-demand site_logo reload performed by a branding asset cache miss.
type brandingHandlerRepoStub struct {
	values map[string]string
}

func (s *brandingHandlerRepoStub) Get(context.Context, string) (*service.Setting, error) {
	panic("unexpected Get call")
}

func (s *brandingHandlerRepoStub) GetValue(_ context.Context, key string) (string, error) {
	return s.values[key], nil
}

func (s *brandingHandlerRepoStub) Set(context.Context, string, string) error {
	panic("unexpected Set call")
}

func (s *brandingHandlerRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *brandingHandlerRepoStub) SetMultiple(context.Context, map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (s *brandingHandlerRepoStub) GetAll(context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *brandingHandlerRepoStub) Delete(context.Context, string) error {
	panic("unexpected Delete call")
}

func newBrandingHandler(t *testing.T, values map[string]string) (*SettingHandler, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := NewSettingHandler(service.NewSettingService(&brandingHandlerRepoStub{values: values}, &config.Config{}), "test")
	settings, err := h.settingService.GetPublicSettings(t.Context())
	require.NoError(t, err)
	require.NotContains(t, settings.SiteLogo, "data:")
	return h, settings.SiteLogo
}

func brandingHandlerRouter(h *SettingHandler) *gin.Engine {
	router := gin.New()
	router.GET("/api/v1/settings/public/:asset", h.GetPublicBrandingAsset)
	router.HEAD("/api/v1/settings/public/:asset", h.GetPublicBrandingAsset)
	return router
}

func TestSettingHandler_GetPublicBrandingAsset(t *testing.T) {
	raw := "data:image/png;base64," + base64.StdEncoding.EncodeToString(brandingPNG)
	h, path := newBrandingHandler(t, map[string]string{"site_logo": raw})
	require.Contains(t, path, "/api/v1/settings/public/logo-")
	require.True(t, strings.HasSuffix(path, ".png"))

	router := brandingHandlerRouter(h)
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(method, path, nil))

			require.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, "image/png", w.Header().Get("Content-Type"))
			assert.Equal(t, "public, max-age=31536000, immutable", w.Header().Get("Cache-Control"))
			assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
			csp := w.Header().Get("Content-Security-Policy")
			assert.Contains(t, strings.Split(csp, "; "), "default-src 'none'")
			assert.Contains(t, strings.Split(csp, "; "), "sandbox")
			assert.NotContains(t, csp, "script-src")
			if method == http.MethodHead {
				assert.Empty(t, w.Body.Bytes())
				assert.Equal(t, strconv.Itoa(len(brandingPNG)), w.Header().Get("Content-Length"))
			} else {
				assert.Equal(t, brandingPNG, w.Body.Bytes())
			}
		})
	}
}

func TestSettingHandler_GetPublicBrandingAssetNotFound(t *testing.T) {
	// A non-inline (external URL) logo never becomes a hashed asset.
	h, _ := newBrandingHandler(t, map[string]string{"site_logo": "https://cdn.example.com/logo.png"})
	router := brandingHandlerRouter(h)

	for _, path := range []string{
		"/api/v1/settings/public/logo-" + strings.Repeat("0", 64) + ".png",
		"/api/v1/settings/public/not-a-logo.png",
	} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			t.Run(method+" "+path, func(t *testing.T) {
				w := httptest.NewRecorder()
				router.ServeHTTP(w, httptest.NewRequest(method, path, nil))
				assert.Equal(t, http.StatusNotFound, w.Code)
				assert.Empty(t, w.Body.Bytes())
				assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
			})
		}
	}
}

// A cold instance must serve a URL minted by another instance, without having
// seen the settings update itself.
func TestSettingHandler_GetPublicBrandingAssetColdInstance(t *testing.T) {
	raw := "data:image/png;base64," + base64.StdEncoding.EncodeToString(brandingPNG)
	writer := service.NewSettingService(&brandingHandlerRepoStub{values: map[string]string{"site_logo": raw}}, &config.Config{})
	settings, err := writer.GetPublicSettings(t.Context())
	require.NoError(t, err)

	cold := service.NewSettingService(&brandingHandlerRepoStub{values: map[string]string{"site_logo": raw}}, &config.Config{})
	w := httptest.NewRecorder()
	brandingHandlerRouter(NewSettingHandler(cold, "test")).ServeHTTP(w, httptest.NewRequest(http.MethodGet, settings.SiteLogo, nil))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, brandingPNG, w.Body.Bytes())
}
