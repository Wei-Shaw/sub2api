//go:build embed

package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/web/seo"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newSEOHandlerForTest(t *testing.T, siteURL string) *SEOHandler {
	t.Helper()
	reg, err := seo.NewRegistry()
	require.NoError(t, err)
	return NewSEOHandler(reg, siteURL, &stubLegalDocs{})
}

type stubLegalDocs struct{}

func (s *stubLegalDocs) ListPublic() []LegalDoc {
	return []LegalDoc{{ID: "terms", Title: "Terms"}}
}

func TestSEOHandler_Robots(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newSEOHandlerForTest(t, "https://example.com")
	r := gin.New()
	r.GET("/robots.txt", h.Robots)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/robots.txt", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/plain; charset=utf-8", w.Header().Get("Content-Type"))
	body := w.Body.String()
	assert.Contains(t, body, "User-agent: *")
	assert.Contains(t, body, "Allow: /home")
	assert.Contains(t, body, "Allow: /faq")
	assert.Contains(t, body, "Disallow: /admin/")
	assert.Contains(t, body, "Sitemap: https://example.com/sitemap.xml")
	assert.Contains(t, body, "GPTBot")
	assert.Contains(t, body, "ClaudeBot")
	assert.Contains(t, body, "Baiduspider")
	assert.True(t, strings.Contains(body, "Bytespider"))
}

func TestSEOHandler_Sitemap(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newSEOHandlerForTest(t, "https://example.com")
	r := gin.New()
	r.GET("/sitemap.xml", h.Sitemap)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/sitemap.xml", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/xml; charset=utf-8", w.Header().Get("Content-Type"))
	body := w.Body.String()
	assert.Contains(t, body, `<?xml version="1.0"`)
	assert.Contains(t, body, `xmlns:xhtml="http://www.w3.org/1999/xhtml"`)
	assert.Contains(t, body, `https://example.com/home`)
	assert.Contains(t, body, `https://example.com/faq`)
	assert.Contains(t, body, `https://example.com/legal/terms`)
	assert.Contains(t, body, `<xhtml:link rel="alternate" hreflang="zh-CN"`)
	assert.Contains(t, body, `<xhtml:link rel="alternate" hreflang="en"`)
}

func TestSEOHandler_LLMsTxt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newSEOHandlerForTest(t, "https://example.com")
	r := gin.New()
	r.GET("/llms.txt", h.LLMsTxt)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/llms.txt", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/markdown; charset=utf-8", w.Header().Get("Content-Type"))
	body := w.Body.String()
	assert.Contains(t, body, "# Sub2API")
	assert.Contains(t, body, "## 文档")
	assert.Contains(t, body, "https://example.com/home")
	assert.Contains(t, body, "https://example.com/faq")
	assert.Contains(t, body, "## 关键问答")
	assert.True(t, strings.Count(body, "Q:") >= 3, "expect at least 3 Q&A excerpts")
}
