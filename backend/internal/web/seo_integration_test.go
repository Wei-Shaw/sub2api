//go:build embed

package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFrontendServer_seoInjection(t *testing.T) {
	t.Run("home_emits_full_seo_when_enabled", func(t *testing.T) {
		fs := newSEOTestFrontendServer(t, true, `{"site_name":"Sub2API","login_agreement_documents":[]}`)
		w := doSEOGet(t, fs, "/home", map[string]string{"Accept-Language": "zh-CN"})
		body := w.Body.String()

		assert.Contains(t, body, `<meta name="description"`)
		assert.Contains(t, body, `<meta name="robots" content="index,follow"`)
		assert.Contains(t, body, `<meta property="og:title"`)
		assert.Contains(t, body, `application/ld+json`)
		assert.Contains(t, body, `<link rel="alternate" hreflang="zh-CN"`)
	})

	t.Run("login_is_noindex", func(t *testing.T) {
		fs := newSEOTestFrontendServer(t, true, `{"site_name":"Sub2API"}`)
		w := doSEOGet(t, fs, "/login", nil)
		body := w.Body.String()
		assert.Contains(t, body, `<meta name="robots" content="noindex,follow"`)
	})

	t.Run("flag_off_falls_back_to_legacy_behavior", func(t *testing.T) {
		fs := newSEOTestFrontendServer(t, false, `{"site_name":"Sub2API"}`)
		w := doSEOGet(t, fs, "/home", nil)
		body := w.Body.String()
		assert.NotContains(t, body, `<meta name="description"`)
	})
}

type seoStubProvider struct{ payload []byte }

func (s *seoStubProvider) GetPublicSettingsForInjection(_ context.Context) (any, error) {
	var v any
	_ = json.Unmarshal(s.payload, &v)
	return v, nil
}

func newSEOTestFrontendServer(t *testing.T, seoOn bool, settingsJSON string) *FrontendServer {
	t.Helper()
	fs, err := NewFrontendServer(&seoStubProvider{payload: []byte(settingsJSON)})
	require.NoError(t, err)
	fs.SetSEOEnabled(seoOn)
	return fs
}

func doSEOGet(t *testing.T, fs *FrontendServer, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(middleware.CSPNonceKey, "test-nonce")
		c.Next()
	})
	r.Use(fs.Middleware())
	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	r.ServeHTTP(w, req)
	return w
}
