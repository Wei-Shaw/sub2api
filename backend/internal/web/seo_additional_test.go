//go:build embed

package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildNotFoundSEOData_IsNoindex(t *testing.T) {
	settingsJSON := []byte(`{
		"site_name":"示例站点",
		"frontend_url":"https://example.com",
		"seo_default_title":"示例站点"
	}`)

	seo := buildNotFoundSEOData("/missing-page", settingsJSON)
	assert.Equal(t, "noindex, nofollow", seo.Robots)
	assert.Equal(t, "noindex, nofollow", seo.XRobotsTag)
	assert.Equal(t, "https://example.com/missing-page", seo.CanonicalURL)
}

func TestFrontendServer_NotFoundRouteSetsNoindexHeader(t *testing.T) {
	provider := &mockSettingsProvider{
		settings: map[string]any{
			"site_name": "示例站点",
			"frontend_url": "https://example.com",
		},
	}

	server, err := NewFrontendServer(provider)
	require.NoError(t, err)

	router := gin.New()
	router.Use(server.Middleware())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/totally-missing-route", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "noindex, nofollow", w.Header().Get("X-Robots-Tag"))
	assert.Contains(t, w.Body.String(), "<meta name=\"robots\" content=\"noindex, nofollow\">")
}

func TestBuildDynamicOGSVG_UsesChineseTutorialCopy(t *testing.T) {
	image, ok := buildDynamicOGSVG(seoConfig{
		SiteName: "示例站点",
	}, "custom-tutorial.svg")
	require.True(t, ok)

	svg := string(image)
	assert.True(t, strings.Contains(svg, "教程文档") || strings.Contains(svg, "示例站点"))
	assert.NotContains(t, svg, "<script")
	assert.NotContains(t, svg, "onload=")
	assert.NotContains(t, svg, "foreignObject")
}

func TestBuildDynamicOGSVG_RejectsUnsafeInlineLogoData(t *testing.T) {
	image, ok := buildDynamicOGSVG(seoConfig{
		SiteName: "示例站点",
		SiteLogo: `data:image/svg+xml;base64,PHN2ZyBvbmxvYWQ9YWxlcnQoMSk+`,
	}, "home.svg")
	require.True(t, ok)

	svg := string(image)
	assert.NotContains(t, svg, `href="data:image/svg+xml`)
	assert.NotContains(t, svg, "onload=")
}

func TestBuildSitemapXML_ExcludesHomeAndTutorialWhenNoindex(t *testing.T) {
	settingsJSON := []byte(`{
		"site_name":"示例站点",
		"frontend_url":"https://example.com",
		"seo_default_robots":"noindex, nofollow",
		"seo_home_robots":"noindex, nofollow"
	}`)

	sitemap := string(buildSitemapXML(settingsJSON))
	assert.NotContains(t, sitemap, "<loc>https://example.com/</loc>")
	assert.NotContains(t, sitemap, "<loc>https://example.com/docs/tutorial</loc>")
}
