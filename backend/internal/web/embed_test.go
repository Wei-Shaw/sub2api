//go:build embed

package web

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestInjectSEOTitle(t *testing.T) {
	t.Run("replaces_title", func(t *testing.T) {
		html := []byte(`<html><head><title>Sub2API - AI API Gateway</title></head><body></body></html>`)
		result := injectSEOTitle(html, "MyCustomSite - AI API Gateway")
		assert.Contains(t, string(result), "<title>MyCustomSite - AI API Gateway</title>")
		assert.NotContains(t, string(result), "<title>Sub2API - AI API Gateway</title>")
	})

	t.Run("returns_unchanged_without_title_tag", func(t *testing.T) {
		html := []byte(`<html><head></head><body></body></html>`)
		result := injectSEOTitle(html, "MyCustomSite - AI API Gateway")
		assert.Equal(t, string(html), string(result))
	})

	t.Run("escapes_html", func(t *testing.T) {
		html := []byte(`<html><head><title>Sub2API</title></head><body></body></html>`)
		result := injectSEOTitle(html, `A&B <Test>`)
		assert.Contains(t, string(result), "<title>A&amp;B &lt;Test&gt;</title>")
	})
}

func TestBuildSEOData(t *testing.T) {
	settingsJSON := []byte(`{
		"site_name":"MyCustomSite",
		"site_subtitle":"Unified AI Gateway",
		"frontend_url":"https://example.com",
		"site_logo":"/logo.png",
		"login_agreement_documents":[{"id":"terms","title":"Terms of Service"}],
		"custom_menu_items":[{"id":"pricing","label":"Pricing","visibility":"user"}]
	}`)

	t.Run("home_is_indexable", func(t *testing.T) {
		seo := buildSEOData("/home", settingsJSON)
		assert.Equal(t, "MyCustomSite - AI API Gateway", seo.Title)
		assert.Equal(t, "index, follow", seo.Robots)
		assert.Equal(t, "https://example.com/", seo.CanonicalURL)
		assert.Equal(t, "https://example.com/og/home.svg", seo.ImageURL)
		assert.NotEmpty(t, seo.JSONLD)
	})

	t.Run("legal_document_uses_document_title", func(t *testing.T) {
		seo := buildSEOData("/legal/terms", settingsJSON)
		assert.Equal(t, "Terms of Service - MyCustomSite", seo.Title)
		assert.Equal(t, "article", seo.Type)
		assert.Equal(t, "index, follow", seo.Robots)
	})

	t.Run("private_routes_are_noindex", func(t *testing.T) {
		seo := buildSEOData("/dashboard", settingsJSON)
		assert.Equal(t, "noindex, nofollow", seo.Robots)
	})
}

func TestBuildRobotsAndSitemap(t *testing.T) {
	settingsJSON := []byte(`{
		"frontend_url":"https://example.com",
		"login_agreement_documents":[{"id":"terms","title":"Terms of Service"}],
		"custom_menu_items":[
			{"id":"pricing","label":"Pricing","visibility":"user"},
			{"id":"admin","label":"Admin","visibility":"admin"}
		]
	}`)

	robots := string(buildRobotsTXT(settingsJSON))
	assert.Contains(t, robots, "Sitemap: https://example.com/sitemap.xml")
	assert.Contains(t, robots, "Disallow: /admin")
	assert.Contains(t, robots, "Disallow: /login")

	sitemap := string(buildSitemapXML(settingsJSON))
	assert.Contains(t, sitemap, "<loc>https://example.com/</loc>")
	assert.Contains(t, sitemap, "<loc>https://example.com/docs/tutorial</loc>")
	assert.Contains(t, sitemap, "<loc>https://example.com/custom/pricing</loc>")
	assert.Contains(t, sitemap, "<loc>https://example.com/legal/terms</loc>")
	assert.NotContains(t, sitemap, "/custom/admin")
}

func TestReplaceNoncePlaceholder(t *testing.T) {
	t.Run("replaces_single_placeholder", func(t *testing.T) {
		html := []byte(`<script nonce="__CSP_NONCE_VALUE__">console.log('test');</script>`)
		nonce := "abc123xyz"

		result := replaceNoncePlaceholder(html, nonce)

		expected := `<script nonce="abc123xyz">console.log('test');</script>`
		assert.Equal(t, expected, string(result))
	})

	t.Run("replaces_multiple_placeholders", func(t *testing.T) {
		html := []byte(`<script nonce="__CSP_NONCE_VALUE__">a</script><script nonce="__CSP_NONCE_VALUE__">b</script>`)
		nonce := "nonce123"

		result := replaceNoncePlaceholder(html, nonce)

		assert.Equal(t, 2, strings.Count(string(result), `nonce="nonce123"`))
		assert.NotContains(t, string(result), NonceHTMLPlaceholder)
	})

	t.Run("handles_empty_nonce", func(t *testing.T) {
		html := []byte(`<script nonce="__CSP_NONCE_VALUE__">test</script>`)
		nonce := ""

		result := replaceNoncePlaceholder(html, nonce)

		assert.Equal(t, `<script nonce="">test</script>`, string(result))
	})

	t.Run("no_placeholder_returns_unchanged", func(t *testing.T) {
		html := []byte(`<script>console.log('test');</script>`)
		nonce := "abc123"

		result := replaceNoncePlaceholder(html, nonce)

		assert.Equal(t, string(html), string(result))
	})

	t.Run("handles_empty_html", func(t *testing.T) {
		html := []byte(``)
		nonce := "abc123"

		result := replaceNoncePlaceholder(html, nonce)

		assert.Empty(t, result)
	})
}

func TestNonceHTMLPlaceholder(t *testing.T) {
	t.Run("constant_value", func(t *testing.T) {
		assert.Equal(t, "__CSP_NONCE_VALUE__", NonceHTMLPlaceholder)
	})
}

// mockSettingsProvider implements PublicSettingsProvider for testing
type mockSettingsProvider struct {
	settings any
	err      error
	called   int
}

type embedTestSettingRepoStub struct {
	values map[string]string
}

func (s *embedTestSettingRepoStub) Get(_ context.Context, _ string) (*service.Setting, error) {
	panic("unexpected Get call")
}

func (s *embedTestSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	if value, ok := s.values[key]; ok {
		return value, nil
	}
	return "", nil
}

func (s *embedTestSettingRepoStub) Set(_ context.Context, _, _ string) error {
	panic("unexpected Set call")
}

func (s *embedTestSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *embedTestSettingRepoStub) SetMultiple(_ context.Context, _ map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (s *embedTestSettingRepoStub) GetAll(_ context.Context) (map[string]string, error) {
	out := make(map[string]string, len(s.values))
	for key, value := range s.values {
		out[key] = value
	}
	return out, nil
}

func (s *embedTestSettingRepoStub) Delete(_ context.Context, _ string) error {
	panic("unexpected Delete call")
}

func (m *mockSettingsProvider) GetPublicSettingsForInjection(ctx context.Context) (any, error) {
	m.called++
	return m.settings, m.err
}

type emptySitemapFrontendServer struct {
	*FrontendServer
}

func (s *emptySitemapFrontendServer) buildSitemapXML(_ []byte) []byte {
	return nil
}

func TestFrontendServer_InjectSettings(t *testing.T) {
	t.Run("injects_settings_with_nonce_placeholder", func(t *testing.T) {
		provider := &mockSettingsProvider{
			settings: map[string]string{"key": "value"},
		}

		server, err := NewFrontendServer(provider)
		require.NoError(t, err)

		settingsJSON := []byte(`{"test":"data"}`)
		result, seo := server.injectSettings("/home", settingsJSON)

		// Should contain the script with nonce placeholder
		assert.Contains(t, string(result), `<script nonce="__CSP_NONCE_VALUE__">`)
		assert.Contains(t, string(result), `window.__APP_CONFIG__={"test":"data"};`)
		assert.Contains(t, string(result), `</script></head>`)
		assert.Equal(t, "index, follow", seo.Robots)
	})

	t.Run("injects_before_head_close", func(t *testing.T) {
		provider := &mockSettingsProvider{
			settings: map[string]string{"key": "value"},
		}

		server, err := NewFrontendServer(provider)
		require.NoError(t, err)

		settingsJSON := []byte(`{}`)
		result, _ := server.injectSettings("/home", settingsJSON)

		// Script should be injected before </head>
		headCloseIndex := bytes.Index(result, []byte("</head>"))
		scriptIndex := bytes.Index(result, []byte(`<script nonce="`))

		assert.True(t, scriptIndex < headCloseIndex, "script should be before </head>")
	})

	t.Run("handles_complex_settings", func(t *testing.T) {
		provider := &mockSettingsProvider{
			settings: map[string]any{
				"nested": map[string]any{
					"array": []int{1, 2, 3},
				},
			},
		}

		server, err := NewFrontendServer(provider)
		require.NoError(t, err)

		settingsJSON := []byte(`{"nested":{"array":[1,2,3]},"special":"<>&"}`)
		result, _ := server.injectSettings("/home", settingsJSON)

		assert.Contains(t, string(result), `window.__APP_CONFIG__={"nested":{"array":[1,2,3]},"special":"<>&"};`)
	})
}

func TestFrontendServer_ServeIndexHTML(t *testing.T) {
	t.Run("serves_html_with_nonce", func(t *testing.T) {
		provider := &mockSettingsProvider{
			settings: map[string]string{"test": "value"},
		}

		server, err := NewFrontendServer(provider)
		require.NoError(t, err)

		// Create a gin context with nonce
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

		// Set nonce in context (simulating SecurityHeaders middleware)
		testNonce := "test-nonce-12345"
		c.Set(middleware.CSPNonceKey, testNonce)

		server.serveIndexHTML(c)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "text/html")

		body := w.Body.String()
		// Nonce placeholder should be replaced
		assert.NotContains(t, body, NonceHTMLPlaceholder)
		assert.Contains(t, body, `nonce="`+testNonce+`"`)
	})

	t.Run("caches_html_content", func(t *testing.T) {
		provider := &mockSettingsProvider{
			settings: map[string]string{"test": "value"},
		}

		server, err := NewFrontendServer(provider)
		require.NoError(t, err)

		// First request
		w1 := httptest.NewRecorder()
		c1, _ := gin.CreateTestContext(w1)
		c1.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		c1.Set(middleware.CSPNonceKey, "nonce1")

		server.serveIndexHTML(c1)
		assert.Equal(t, 1, provider.called)

		// Second request - should use cache
		w2 := httptest.NewRecorder()
		c2, _ := gin.CreateTestContext(w2)
		c2.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		c2.Set(middleware.CSPNonceKey, "nonce2")

		server.serveIndexHTML(c2)
		// Settings provider should not be called again
		assert.Equal(t, 1, provider.called)

		// But nonce should be different
		assert.Contains(t, w2.Body.String(), `nonce="nonce2"`)
	})

	t.Run("sets_etag_header", func(t *testing.T) {
		provider := &mockSettingsProvider{
			settings: map[string]string{"test": "value"},
		}

		server, err := NewFrontendServer(provider)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		c.Set(middleware.CSPNonceKey, "nonce123")

		server.serveIndexHTML(c)

		etag := w.Header().Get("ETag")
		assert.NotEmpty(t, etag)
		assert.True(t, strings.HasPrefix(etag, `"`))
		assert.True(t, strings.HasSuffix(etag, `"`))
	})

	t.Run("returns_304_for_matching_etag", func(t *testing.T) {
		provider := &mockSettingsProvider{
			settings: map[string]string{
				"test": "value",
				"frontend_url": "https://example.com",
			},
		}

		server, err := NewFrontendServer(provider)
		require.NoError(t, err)

		// Use a real router for proper 304 handling
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set(middleware.CSPNonceKey, "test-nonce")
			c.Next()
		})
		router.Use(server.Middleware())

		// First request to populate cache and get ETag
		w1 := httptest.NewRecorder()
		req1 := httptest.NewRequest(http.MethodGet, "/login", nil)
		router.ServeHTTP(w1, req1)
		etag := w1.Header().Get("ETag")
		require.NotEmpty(t, etag)

		// Second request with If-None-Match
		w2 := httptest.NewRecorder()
		req2 := httptest.NewRequest(http.MethodGet, "/login", nil)
		req2.Header.Set("If-None-Match", etag)
		router.ServeHTTP(w2, req2)

		assert.Equal(t, http.StatusNotModified, w2.Code)
		assert.Empty(t, w2.Body.String())
	})

	t.Run("sets_cache_control_header", func(t *testing.T) {
		provider := &mockSettingsProvider{
			settings: map[string]string{"test": "value"},
		}

		server, err := NewFrontendServer(provider)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		c.Set(middleware.CSPNonceKey, "nonce123")

		server.serveIndexHTML(c)

		assert.Equal(t, "no-cache", w.Header().Get("Cache-Control"))
	})

	t.Run("fallback_on_settings_error", func(t *testing.T) {
		provider := &mockSettingsProvider{
			err: context.DeadlineExceeded,
		}

		server, err := NewFrontendServer(provider)
		require.NoError(t, err)

		// Invalidate cache to force settings fetch
		server.InvalidateCache()

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		c.Set(middleware.CSPNonceKey, "nonce123")

		server.serveIndexHTML(c)

		// Should still return 200 with base HTML
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "text/html")
	})
}

func TestFrontendServer_InvalidateCache(t *testing.T) {
	t.Run("invalidates_cache", func(t *testing.T) {
		provider := &mockSettingsProvider{
			settings: map[string]string{"test": "value"},
		}

		server, err := NewFrontendServer(provider)
		require.NoError(t, err)

		// First request to populate cache
		w1 := httptest.NewRecorder()
		c1, _ := gin.CreateTestContext(w1)
		c1.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		c1.Set(middleware.CSPNonceKey, "nonce1")

		server.serveIndexHTML(c1)
		assert.Equal(t, 1, provider.called)

		// Invalidate cache
		server.InvalidateCache()

		// Update settings
		provider.settings = map[string]string{"test": "new_value"}

		// Second request should fetch new settings
		w2 := httptest.NewRecorder()
		c2, _ := gin.CreateTestContext(w2)
		c2.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		c2.Set(middleware.CSPNonceKey, "nonce2")

		server.serveIndexHTML(c2)
		assert.Equal(t, 2, provider.called)
	})

	t.Run("handles_nil_server", func(t *testing.T) {
		var server *FrontendServer
		// Should not panic
		assert.NotPanics(t, func() {
			server.InvalidateCache()
		})
	})

	t.Run("handles_nil_cache", func(t *testing.T) {
		server := &FrontendServer{}
		// Should not panic
		assert.NotPanics(t, func() {
			server.InvalidateCache()
		})
	})
}

func TestFrontendServer_Middleware(t *testing.T) {
	t.Run("skips_api_routes", func(t *testing.T) {
		provider := &mockSettingsProvider{
			settings: map[string]string{"test": "value"},
		}

		server, err := NewFrontendServer(provider)
		require.NoError(t, err)

		apiPaths := []string{
			"/api/v1/users",
			"/v1/models",
			"/v1beta/chat",
			"/backend-api/codex/responses",
			"/backend-api/codex/responses/compact",
			"/antigravity/test",
			"/setup/init",
			"/health",
			"/responses",
			"/responses/compact",
		}

		for _, path := range apiPaths {
			t.Run(path, func(t *testing.T) {
				router := gin.New()
				router.Use(server.Middleware())
				nextCalled := false
				router.GET(path, func(c *gin.Context) {
					nextCalled = true
					c.String(http.StatusOK, "ok")
				})

				w := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodGet, path, nil)
				router.ServeHTTP(w, req)

				assert.True(t, nextCalled, "next handler should be called for API route")
			})
		}
	})

	t.Run("skips_responses_compact_post_routes", func(t *testing.T) {
		provider := &mockSettingsProvider{
			settings: map[string]string{"test": "value"},
		}

		server, err := NewFrontendServer(provider)
		require.NoError(t, err)

		router := gin.New()
		router.Use(server.Middleware())
		nextCalled := false
		router.POST("/responses/compact", func(c *gin.Context) {
			nextCalled = true
			c.String(http.StatusOK, `{"ok":true}`)
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/responses/compact", strings.NewReader(`{"model":"gpt-5"}`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		assert.True(t, nextCalled, "next handler should be called for compact API route")
		assert.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `{"ok":true}`, w.Body.String())
	})

	t.Run("serves_index_for_spa_routes", func(t *testing.T) {
		provider := &mockSettingsProvider{
			settings: map[string]string{"test": "value"},
		}

		server, err := NewFrontendServer(provider)
		require.NoError(t, err)

		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set(middleware.CSPNonceKey, "test-nonce")
			c.Next()
		})
		router.Use(server.Middleware())

		spaPaths := []string{
			"/",
			"/users/123",
			"/settings/profile",
		}

		for _, path := range spaPaths {
			t.Run(path, func(t *testing.T) {
				w := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodGet, path, nil)
				router.ServeHTTP(w, req)

				assert.Equal(t, http.StatusOK, w.Code)
				assert.Contains(t, w.Header().Get("Content-Type"), "text/html")
			})
		}
	})

	t.Run("serves_robots_and_sitemap", func(t *testing.T) {
		provider := &mockSettingsProvider{
			settings: map[string]any{
				"frontend_url": "https://example.com",
				"login_agreement_documents": []map[string]string{
					{"id": "terms", "title": "Terms of Service"},
				},
			},
		}

		server, err := NewFrontendServer(provider)
		require.NoError(t, err)

		router := gin.New()
		router.Use(server.Middleware())

		wRobots := httptest.NewRecorder()
		reqRobots := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
		router.ServeHTTP(wRobots, reqRobots)
		assert.Equal(t, http.StatusOK, wRobots.Code)
		assert.Contains(t, wRobots.Body.String(), "Sitemap: https://example.com/sitemap.xml")

		wSitemap := httptest.NewRecorder()
		reqSitemap := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
		router.ServeHTTP(wSitemap, reqSitemap)
		assert.Equal(t, http.StatusOK, wSitemap.Code)
		assert.Contains(t, wSitemap.Body.String(), "<loc>https://example.com/</loc>")
		assert.Contains(t, wSitemap.Body.String(), "<loc>https://example.com/docs/tutorial</loc>")
		assert.Contains(t, wSitemap.Body.String(), "<loc>https://example.com/legal/terms</loc>")
	})

	t.Run("sitemap_falls_back_to_base_generator_when_runtime_filter_returns_empty", func(t *testing.T) {
		provider := &mockSettingsProvider{
			settings: map[string]any{
				"frontend_url": "https://example.com",
				"login_agreement_documents": []map[string]string{
					{"id": "terms", "title": "Terms of Service"},
				},
			},
		}

		baseServer, err := NewFrontendServer(provider)
		require.NoError(t, err)
		server := &emptySitemapFrontendServer{FrontendServer: baseServer}

		router := gin.New()
		router.GET("/sitemap.xml", server.serveSitemapXML)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "<loc>https://example.com/</loc>")
		assert.Contains(t, w.Body.String(), "<loc>https://example.com/docs/tutorial</loc>")
	})

	t.Run("serves_dynamic_og_image", func(t *testing.T) {
		provider := &mockSettingsProvider{
			settings: map[string]any{
				"site_name": "MyCustomSite",
				"frontend_url": "https://example.com",
				"site_subtitle": "Unified AI Gateway",
			},
		}

		server, err := NewFrontendServer(provider)
		require.NoError(t, err)

		router := gin.New()
		router.Use(server.Middleware())

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/og/home.svg", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "image/svg+xml")
		assert.Contains(t, w.Body.String(), "MyCustomSite")
	})

	t.Run("returns_not_found_for_unknown_og_image", func(t *testing.T) {
		provider := &mockSettingsProvider{
			settings: map[string]any{
				"site_name": "MyCustomSite",
				"frontend_url": "https://example.com",
			},
		}

		server, err := NewFrontendServer(provider)
		require.NoError(t, err)

		router := gin.New()
		router.Use(server.Middleware())

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/og/not-found.svg", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("caches_per_route", func(t *testing.T) {
		provider := &mockSettingsProvider{
			settings: map[string]any{
				"site_name": "MyCustomSite",
				"frontend_url": "https://example.com",
				"login_agreement_documents": []map[string]string{
					{"id": "terms", "title": "Terms of Service"},
				},
			},
		}

		server, err := NewFrontendServer(provider)
		require.NoError(t, err)

		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set(middleware.CSPNonceKey, "test-nonce")
			c.Next()
		})
		router.Use(server.Middleware())

		wLogin := httptest.NewRecorder()
		router.ServeHTTP(wLogin, httptest.NewRequest(http.MethodGet, "/login", nil))
		wLegal := httptest.NewRecorder()
		router.ServeHTTP(wLegal, httptest.NewRequest(http.MethodGet, "/legal/terms", nil))

		assert.Contains(t, wLogin.Body.String(), "<title>MyCustomSite - AI API Gateway</title>")
		assert.Contains(t, wLegal.Body.String(), "<title>Terms of Service - MyCustomSite</title>")
		assert.NotEmpty(t, wLogin.Header().Get("ETag"))
		assert.NotEqual(t, wLogin.Header().Get("ETag"), wLegal.Header().Get("ETag"))
	})

	t.Run("renders_public_markdown_page_server_side", func(t *testing.T) {
		provider := &mockSettingsProvider{
			settings: map[string]any{
				"site_name": "MyCustomSite",
				"frontend_url": "https://example.com",
				"custom_menu_items": []map[string]any{
					{"id": "guide", "label": "Guide", "visibility": "user", "page_slug": "guide"},
				},
			},
		}

		server, err := NewFrontendServer(provider)
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(server.pagesDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(server.pagesDir, "guide.md"), []byte("# Guide\n\nHello world"), 0o644))

		router := gin.New()
		router.Use(server.Middleware())

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/custom/guide", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "<h1>Guide</h1>")
		assert.Contains(t, w.Body.String(), "<p>Hello world</p>")
	})

	t.Run("renders_home_page_server_side", func(t *testing.T) {
		provider := &mockSettingsProvider{
			settings: map[string]any{
				"site_name": "MyCustomSite",
				"frontend_url": "https://example.com",
				"site_subtitle": "Readable public home page",
			},
		}

		server, err := NewFrontendServer(provider)
		require.NoError(t, err)

		router := gin.New()
		router.Use(server.Middleware())

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/home", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "Readable public home page")
		assert.Contains(t, w.Body.String(), "查看教程文档")
		assert.NotContains(t, w.Body.String(), `<div id="app"></div>`)
	})

	t.Run("frontend server uses configured pricing data dir for public markdown pages", func(t *testing.T) {
		root := t.TempDir()
		dataDir := filepath.Join(root, "runtime-data")
		require.NoError(t, os.MkdirAll(filepath.Join(dataDir, "pages"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dataDir, "pages", "guide.md"), []byte("# Guide\n\nConfigured data dir body"), 0o644))
		origWD, err := os.Getwd()
		require.NoError(t, err)
		require.NoError(t, os.Chdir(root))
		t.Cleanup(func() {
			_ = os.Chdir(origWD)
		})
		require.NoError(t, os.WriteFile(filepath.Join(root, "config.yaml"), []byte("pricing:\n  data_dir: "+filepath.ToSlash(dataDir)+"\n"), 0o644))

		repo := &embedTestSettingRepoStub{
			values: map[string]string{
				service.SettingKeyFrontendURL: "https://example.com",
				service.SettingKeyCustomMenuItems: `[{"id":"guide","label":"Guide","visibility":"user","page_slug":"guide"}]`,
			},
		}
		settingSvc := service.NewSettingService(repo, &config.Config{Pricing: config.PricingConfig{DataDir: dataDir}})

		server, err := NewFrontendServer(settingSvc)
		require.NoError(t, err)

		router := gin.New()
		router.Use(server.Middleware())

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/custom/guide", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "Configured data dir body")
	})

	t.Run("renders_public_embedded_page_server_side", func(t *testing.T) {
		provider := &mockSettingsProvider{
			settings: map[string]any{
				"site_name": "MyCustomSite",
				"frontend_url": "https://example.com",
				"custom_menu_items": []map[string]any{
					{"id": "pricing", "label": "Pricing", "visibility": "user", "url": "https://billing.example.com/embed"},
				},
			},
		}

		server, err := NewFrontendServer(provider)
		require.NoError(t, err)

		router := gin.New()
		router.Use(server.Middleware())

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/custom/pricing", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "<iframe")
		assert.Contains(t, w.Body.String(), "https://billing.example.com/embed?ui_mode=embedded")
		assert.Contains(t, w.Body.String(), "<title>Pricing - MyCustomSite</title>")
		assert.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))
	})

	t.Run("renders_builtin_tutorial_markdown_page", func(t *testing.T) {
		provider := &mockSettingsProvider{
			settings: map[string]any{
				"site_name": "MyCustomSite",
				"frontend_url": "https://example.com",
			},
		}

		server, err := NewFrontendServer(provider)
		require.NoError(t, err)

		router := gin.New()
		router.Use(server.Middleware())

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/docs/tutorial", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "<h1>教程文档</h1>")
		assert.Contains(t, w.Body.String(), "你可以在后台的“教程文档”页面直接编辑这份教程文档正文")
	})

	t.Run("rewrites_legal_relative_images_to_scoped_public_image_urls", func(t *testing.T) {
		rendered, err := renderMarkdownToHTML("![法律图](images/legal-demo.png)", "legal-terms")
		require.NoError(t, err)
		assert.Contains(t, rendered, `/api/v1/pages/legal-terms/images/images/legal-demo.png`)
	})

	t.Run("sitemap_excludes_missing_markdown_custom_page", func(t *testing.T) {
		provider := &mockSettingsProvider{
			settings: map[string]any{
				"site_name": "MyCustomSite",
				"frontend_url": "https://example.com",
				"custom_menu_items": []map[string]any{
					{"id": "guide", "label": "Guide", "visibility": "user", "page_slug": "guide"},
					{"id": "missing", "label": "Missing", "visibility": "user", "page_slug": "missing"},
				},
			},
		}

		server, err := NewFrontendServer(provider)
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(server.pagesDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(server.pagesDir, "guide.md"), []byte("# Guide"), 0o644))

		router := gin.New()
		router.Use(server.Middleware())

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "https://example.com/custom/guide")
		assert.NotContains(t, w.Body.String(), "https://example.com/custom/missing")
	})

	t.Run("renders_markdown_lists_and_inline_elements", func(t *testing.T) {
		rendered, err := renderMarkdownToHTML("# Title\n\n- item one\n- item two\n\n1. first\n2. second\n\n**bold** and *em* and `code`", "")
		require.NoError(t, err)
		assert.Contains(t, rendered, ">Title</h1>")
		assert.Contains(t, rendered, "<ul>")
		assert.Contains(t, rendered, "<li>item one</li>")
		assert.Contains(t, rendered, "<li>item two</li>")
		assert.Contains(t, rendered, "<ol>")
		assert.Contains(t, rendered, "<li>first</li>")
		assert.Contains(t, rendered, "<li>second</li>")
		assert.Contains(t, rendered, "<strong>bold</strong>")
		assert.Contains(t, rendered, "<em>em</em>")
		assert.Contains(t, rendered, "<code>code</code>")
	})

	t.Run("rewrites_relative_html_images_for_markdown_pages", func(t *testing.T) {
		rendered, err := renderMarkdownToHTML(`<img src="assets/教程截图-中文.png" alt="教程图">`, "guide")
		require.NoError(t, err)
		assert.Contains(t, rendered, `/api/v1/pages/guide/images/assets/%E6%95%99%E7%A8%8B%E6%88%AA%E5%9B%BE-%E4%B8%AD%E6%96%87.png`)
		assert.Contains(t, rendered, `alt="教程图"`)
	})

	t.Run("serves_static_files", func(t *testing.T) {
		provider := &mockSettingsProvider{
			settings: map[string]string{"test": "value"},
		}

		server, err := NewFrontendServer(provider)
		require.NoError(t, err)

		router := gin.New()
		router.Use(server.Middleware())

		// Request for existing static file
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/logo.png", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "image/png")
	})
}

func TestNewFrontendServer(t *testing.T) {
	t.Run("creates_server_successfully", func(t *testing.T) {
		provider := &mockSettingsProvider{
			settings: map[string]string{"test": "value"},
		}

		server, err := NewFrontendServer(provider)

		require.NoError(t, err)
		assert.NotNil(t, server)
		assert.NotNil(t, server.distFS)
		assert.NotNil(t, server.fileServer)
		assert.NotNil(t, server.baseHTML)
		assert.NotNil(t, server.cache)
		assert.Equal(t, provider, server.settings)
	})

	t.Run("reads_base_html", func(t *testing.T) {
		provider := &mockSettingsProvider{
			settings: map[string]string{"test": "value"},
		}

		server, err := NewFrontendServer(provider)
		require.NoError(t, err)

		assert.NotEmpty(t, server.baseHTML)
		assert.Contains(t, string(server.baseHTML), "<!doctype html>")
	})
}

func TestBuildSEODataEscapesJSONLDScriptClosingSequence(t *testing.T) {
	settingsJSON := []byte(`{
		"site_name":"MyCustomSite",
		"frontend_url":"https://example.com",
		"login_agreement_documents":[{"id":"terms","title":"</script><script>alert(1)</script>"}]
	}`)

	seo := buildSEOData("/legal/terms", settingsJSON)
	assert.NotContains(t, seo.JSONLD, "</script>")
	assert.Contains(t, seo.JSONLD, "\\u003c/script\\u003e")
}

func TestFrontendServer_PrivateHTMLRoutesRequireAuth(t *testing.T) {
	provider := &mockSettingsProvider{
		settings: map[string]any{
			"site_name": "MyCustomSite",
			"frontend_url": "https://example.com",
		},
	}

	userAuth := func(c *gin.Context) {
		authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
		if authHeader != "Bearer user-token" && authHeader != "Bearer admin-token" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	}
	adminAuth := func(c *gin.Context) {
		authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
		if authHeader != "Bearer admin-token" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	}

	server, err := NewFrontendServer(provider, userAuth, adminAuth)
	require.NoError(t, err)

	router := gin.New()
	router.Use(server.Middleware())

	wGuest := httptest.NewRecorder()
	reqGuest := httptest.NewRequest(http.MethodGet, "/admin/tutorial", nil)
	router.ServeHTTP(wGuest, reqGuest)
	require.Equal(t, http.StatusFound, wGuest.Code)
	require.Contains(t, wGuest.Header().Get("Location"), "/login")

	wUser := httptest.NewRecorder()
	reqUser := httptest.NewRequest(http.MethodGet, "/admin/tutorial", nil)
	reqUser.AddCookie(&http.Cookie{Name: "auth_token", Value: url.QueryEscape("user-token")})
	router.ServeHTTP(wUser, reqUser)
	require.Equal(t, http.StatusFound, wUser.Code)
	require.Contains(t, wUser.Header().Get("Location"), "/login")
}

func TestHasEmbeddedFrontend(t *testing.T) {
	t.Run("returns_true_when_frontend_embedded", func(t *testing.T) {
		result := HasEmbeddedFrontend()
		assert.True(t, result)
	})
}

// Tests for legacy ServeEmbeddedFrontend function
func TestServeEmbeddedFrontend(t *testing.T) {
	t.Run("serves_static_files", func(t *testing.T) {
		middleware := ServeEmbeddedFrontend()

		router := gin.New()
		router.Use(middleware)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/logo.png", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "image/png")
	})

	t.Run("serves_index_html_for_root", func(t *testing.T) {
		middleware := ServeEmbeddedFrontend()

		router := gin.New()
		router.Use(middleware)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "text/html")
		assert.Contains(t, w.Body.String(), "<!doctype html>")
	})

	t.Run("serves_index_html_for_spa_routes", func(t *testing.T) {
		middleware := ServeEmbeddedFrontend()

		router := gin.New()
		router.Use(middleware)

		spaPaths := []string{"/dashboard", "/users/123", "/settings"}

		for _, path := range spaPaths {
			t.Run(path, func(t *testing.T) {
				w := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodGet, path, nil)
				router.ServeHTTP(w, req)

				assert.Equal(t, http.StatusOK, w.Code)
				assert.Contains(t, w.Header().Get("Content-Type"), "text/html")
			})
		}
	})

	t.Run("skips_api_routes", func(t *testing.T) {
		middleware := ServeEmbeddedFrontend()

		apiPaths := []string{
			"/api/users",
			"/v1/models",
			"/v1beta/chat",
			"/backend-api/codex/responses",
			"/backend-api/codex/responses/compact",
			"/antigravity/test",
			"/setup/init",
			"/health",
			"/responses",
			"/responses/compact",
		}

		for _, path := range apiPaths {
			t.Run(path, func(t *testing.T) {
				nextCalled := false
				router := gin.New()
				router.Use(middleware)
				router.GET(path, func(c *gin.Context) {
					nextCalled = true
					c.String(http.StatusOK, "ok")
				})

				w := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodGet, path, nil)
				router.ServeHTTP(w, req)

				assert.True(t, nextCalled, "next handler should be called for API route")
			})
		}
	})
}

// Tests for HTMLCache
func TestHTMLCache(t *testing.T) {
	t.Run("new_cache_returns_nil", func(t *testing.T) {
		cache := NewHTMLCache()
		assert.Nil(t, cache.Get("/"))
	})

	t.Run("set_and_get", func(t *testing.T) {
		cache := NewHTMLCache()
		cache.SetBaseHTML([]byte("<html></html>"))

		html := []byte("<html><body>test</body></html>")
		settings := []byte(`{"key":"value"}`)
		cache.Set("/", html, settings)

		result := cache.Get("/")
		require.NotNil(t, result)
		assert.Equal(t, html, result.Content)
		assert.NotEmpty(t, result.ETag)
	})

	t.Run("invalidate_clears_cache", func(t *testing.T) {
		cache := NewHTMLCache()
		cache.SetBaseHTML([]byte("<html></html>"))

		html := []byte("<html><body>test</body></html>")
		settings := []byte(`{"key":"value"}`)
		cache.Set("/", html, settings)

		require.NotNil(t, cache.Get("/"))

		cache.Invalidate()

		assert.Nil(t, cache.Get("/"))
	})

	t.Run("etag_changes_with_settings", func(t *testing.T) {
		cache := NewHTMLCache()
		cache.SetBaseHTML([]byte("<html></html>"))

		html := []byte("<html><body>test</body></html>")

		cache.Set("/home", html, []byte(`{"v":1}`))
		etag1 := cache.Get("/home").ETag

		cache.Invalidate()
		cache.Set("/home", html, []byte(`{"v":2}`))
		etag2 := cache.Get("/home").ETag

		assert.NotEqual(t, etag1, etag2)
	})

	t.Run("etag_format", func(t *testing.T) {
		cache := NewHTMLCache()
		cache.SetBaseHTML([]byte("<html></html>"))

		cache.Set("/", []byte("<html></html>"), []byte(`{}`))
		result := cache.Get("/")

		// ETag should be quoted
		assert.True(t, strings.HasPrefix(result.ETag, `"`))
		assert.True(t, strings.HasSuffix(result.ETag, `"`))
		// Should contain dash separator
		assert.Contains(t, result.ETag[1:len(result.ETag)-1], "-")
	})
}

// Benchmark tests
func BenchmarkReplaceNoncePlaceholder(b *testing.B) {
	html := []byte(`<!DOCTYPE html><html><head><script nonce="__CSP_NONCE_VALUE__">window.__APP_CONFIG__={"test":"data"};</script></head><body></body></html>`)
	nonce := "abcdefghijklmnop123456=="

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		replaceNoncePlaceholder(html, nonce)
	}
}

func TestShouldBypassEmbeddedFrontendAllowsReferralLanding(t *testing.T) {
	t.Parallel()

	assert.True(t, shouldBypassEmbeddedFrontend("/r/ABC123"))
	assert.False(t, shouldBypassEmbeddedFrontend("/register"))
}

func BenchmarkFrontendServerServeIndexHTML(b *testing.B) {
	provider := &mockSettingsProvider{
		settings: map[string]string{"test": "value"},
	}

	server, _ := NewFrontendServer(provider)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		c.Set(middleware.CSPNonceKey, "test-nonce")

		server.serveIndexHTML(c)
	}
}
