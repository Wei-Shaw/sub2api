//go:build embed

package handler

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/web/seo"
	"github.com/gin-gonic/gin"
)

// LegalDoc is the minimal shape this handler needs from public settings.
type LegalDoc struct {
	ID        string
	Title     string
	UpdatedAt time.Time
}

// LegalDocSource is implemented by service or test stub.
type LegalDocSource interface {
	ListPublic() []LegalDoc
}

// SEOHandler serves /robots.txt, /sitemap.xml, /llms.txt.
type SEOHandler struct {
	reg     *seo.Registry
	siteURL string
	docs    LegalDocSource

	mu          sync.Mutex
	robotsCache string
	robotsAt    time.Time
}

// NewSEOHandler constructs a SEOHandler.
func NewSEOHandler(reg *seo.Registry, siteURL string, docs LegalDocSource) *SEOHandler {
	return &SEOHandler{reg: reg, siteURL: strings.TrimRight(siteURL, "/"), docs: docs}
}

// Robots serves /robots.txt.
func (h *SEOHandler) Robots(c *gin.Context) {
	h.mu.Lock()
	if h.robotsCache != "" && time.Since(h.robotsAt) < 5*time.Minute {
		body := h.robotsCache
		h.mu.Unlock()
		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(body))
		return
	}
	h.mu.Unlock()

	body := h.renderRobots()
	h.mu.Lock()
	h.robotsCache = body
	h.robotsAt = time.Now()
	h.mu.Unlock()
	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(body))
}

func (h *SEOHandler) renderRobots() string {
	var b strings.Builder
	b.WriteString("User-agent: *\n")
	// Allowlist: indexable static routes + login/register (crawl-allowed but noindex via meta).
	for path, rec := range h.reg.Routes {
		if strings.Contains(path, ":") {
			continue
		}
		if rec.Indexable || path == "/login" || path == "/register" {
			b.WriteString("Allow: " + path + "\n")
		}
	}
	b.WriteString("Allow: /legal/\n")
	disallow := []string{
		"/dashboard", "/admin/", "/api/", "/v1/", "/v1beta/", "/setup/", "/payment/", "/auth/",
		"/keys", "/usage", "/orders", "/invoice", "/image-generation",
		"/redeem", "/affiliate", "/custom/", "/monitor", "/key-usage", "/subscriptions",
	}
	for _, p := range disallow {
		b.WriteString("Disallow: " + p + "\n")
	}

	bots := []struct {
		name string
		full bool
	}{
		{"GPTBot", false}, {"ClaudeBot", false}, {"PerplexityBot", true},
		{"Google-Extended", true}, {"Bytespider", false}, {"Baiduspider", false},
	}
	for _, bot := range bots {
		b.WriteString("\nUser-agent: " + bot.name + "\n")
		if bot.full {
			b.WriteString("Allow: /\n")
		} else {
			b.WriteString("Allow: /home\nAllow: /faq\nAllow: /legal/\nDisallow: /\n")
		}
	}

	b.WriteString("\nSitemap: " + h.siteURL + "/sitemap.xml\n")
	return b.String()
}

// Sitemap serves /sitemap.xml.
func (h *SEOHandler) Sitemap(c *gin.Context) {
	now := time.Now().UTC().Format("2006-01-02")
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"` +
		` xmlns:xhtml="http://www.w3.org/1999/xhtml">` + "\n")

	// Static indexable routes
	for path, rec := range h.reg.Routes {
		if !rec.Indexable || rec.Dynamic != "" {
			continue
		}
		b.WriteString("  <url>\n")
		b.WriteString("    <loc>" + xmlEscape(h.siteURL+path) + "</loc>\n")
		b.WriteString("    <lastmod>" + now + "</lastmod>\n")
		if rec.Changefreq != "" {
			b.WriteString("    <changefreq>" + rec.Changefreq + "</changefreq>\n")
		}
		if p := formatPriority(rec.Priority); p != "" {
			b.WriteString("    <priority>" + p + "</priority>\n")
		}
		// hreflang alternates
		b.WriteString(`    <xhtml:link rel="alternate" hreflang="zh-CN" href="` + xmlEscape(h.siteURL+path) + `" />` + "\n")
		b.WriteString(`    <xhtml:link rel="alternate" hreflang="en" href="` + xmlEscape(h.siteURL+path+"?lang=en") + `" />` + "\n")
		b.WriteString(`    <xhtml:link rel="alternate" hreflang="x-default" href="` + xmlEscape(h.siteURL+path) + `" />` + "\n")
		b.WriteString("  </url>\n")
	}

	// Dynamic: legal docs from public settings
	for _, d := range h.docs.ListPublic() {
		lm := now
		if !d.UpdatedAt.IsZero() {
			lm = d.UpdatedAt.UTC().Format("2006-01-02")
		}
		b.WriteString("  <url>\n")
		b.WriteString("    <loc>" + xmlEscape(h.siteURL+"/legal/"+d.ID) + "</loc>\n")
		b.WriteString("    <lastmod>" + lm + "</lastmod>\n")
		b.WriteString("    <changefreq>monthly</changefreq>\n")
		b.WriteString("    <priority>0.5</priority>\n")
		b.WriteString("  </url>\n")
	}

	b.WriteString("</urlset>\n")

	c.Header("Cache-Control", "public, max-age=3600")
	c.Data(http.StatusOK, "application/xml; charset=utf-8", []byte(b.String()))
}

// LLMsTxt serves /llms.txt (https://llmstxt.org).
func (h *SEOHandler) LLMsTxt(c *gin.Context) {
	zhHome := h.reg.Routes["/home"].Langs["zh"]
	faq, err := seo.LoadFAQ("zh")
	if err != nil {
		faq = nil
	}

	var b strings.Builder
	b.WriteString("# " + h.reg.Site.Name + "\n\n")
	b.WriteString("> " + zhHome.Description + "\n\n")
	b.WriteString("## 核心能力\n\n")
	b.WriteString("- **统一 API**:OpenAI / Anthropic Messages / Gemini 三种协议自动转换\n")
	b.WriteString("- **支持客户端**:Claude Code、Codex CLI、Gemini CLI、Cursor、Continue、Cline、JetBrains AI、VS Code\n")
	b.WriteString("- **计费**:按 token 实时计费,支持余额、订阅、兑换码、推广返佣\n\n")
	b.WriteString("## 文档\n\n")
	b.WriteString("- [首页](" + h.siteURL + "/home)\n")
	b.WriteString("- [常见问题 FAQ](" + h.siteURL + "/faq)\n")
	for _, d := range h.docs.ListPublic() {
		b.WriteString("- [" + d.Title + "](" + h.siteURL + "/legal/" + d.ID + ")\n")
	}
	b.WriteString("\n## 关键问答\n\n")
	for i, item := range faq {
		if i >= 5 {
			break
		}
		b.WriteString("Q: " + item.Q + "\n")
		b.WriteString("A: " + item.A + "\n\n")
	}

	c.Header("Cache-Control", "public, max-age=3600")
	c.Data(http.StatusOK, "text/markdown; charset=utf-8", []byte(b.String()))
}

func formatPriority(p float64) string {
	if p <= 0 {
		return ""
	}
	return strconv.FormatFloat(p, 'f', 1, 64)
}

func xmlEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		`'`, "&apos;",
	)
	return r.Replace(s)
}
