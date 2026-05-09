//go:build embed

package web

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tutorialhtml"
	"github.com/gin-gonic/gin"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	rendererhtml "github.com/yuin/goldmark/renderer/html"
)

var (
	markdownImagePattern   = regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)
	inlineOGLogoDataURLRE  = regexp.MustCompile(`(?i)^data:image/(png|jpeg|gif|webp);base64,[a-z0-9+/=\s]+$`)
	serverMarkdownRenderer = goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Footnote,
			extension.DefinitionList,
			extension.Typographer,
		),
	goldmark.WithParserOptions(
		parser.WithAutoHeadingID(),
	),
	goldmark.WithRendererOptions(
		rendererhtml.WithXHTML(),
	),
	)
)

func (s *FrontendServer) tryServePublicRenderedPage(c *gin.Context, requestPath string) bool {
	requestPath = normalizeRequestPath(requestPath)
	if !strings.HasPrefix(requestPath, "/legal/") && !strings.HasPrefix(requestPath, "/custom/") && requestPath != "/docs/tutorial" {
		return false
	}

	settingsJSON, ok := s.loadSettingsJSON(c)
	if !ok {
		return false
	}
	cfg := parseSEOConfig(settingsJSON)

	switch {
	case strings.HasPrefix(requestPath, "/legal/"):
		docID := strings.TrimPrefix(requestPath, "/legal/")
		for _, doc := range cfg.LoginAgreementDocuments {
			if strings.TrimSpace(doc.ID) == strings.TrimSpace(docID) {
				return s.serveRenderedMarkdownPage(c, settingsJSON, requestPath, doc.Title, doc.ContentMD, "")
			}
		}
	case strings.HasPrefix(requestPath, "/custom/"):
		pageID := strings.TrimPrefix(requestPath, "/custom/")
		for _, item := range cfg.CustomMenuItems {
			if item.Visibility == "admin" || strings.TrimSpace(item.ID) != strings.TrimSpace(pageID) {
				continue
			}
			slug := strings.TrimSpace(item.PageSlug)
			if slug == "" && strings.HasPrefix(strings.TrimSpace(item.URL), "md:") {
				slug = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(item.URL), "md:"))
			}
			if slug == "" {
				embeddedURL, ok := buildPublicEmbeddedURL(item.URL)
				if !ok {
					return false
				}
				return s.serveRenderedEmbeddedPage(c, settingsJSON, requestPath, item.Label, embeddedURL)
			}
			raw, err := s.loadMarkdownFile(slug)
			if err != nil {
				return false
			}
			return s.serveRenderedMarkdownPage(c, settingsJSON, requestPath, item.Label, string(raw), slug)
		}
	case requestPath == "/docs/tutorial":
		if htmlDoc, ok := s.loadTutorialHTML(); ok {
			return s.serveRenderedHTMLPage(c, settingsJSON, requestPath, "教程文档", htmlDoc)
		}
		raw, err := s.loadMarkdownFile("tutorial")
		if err != nil {
			return false
		}
		return s.serveRenderedMarkdownPage(c, settingsJSON, requestPath, "教程文档", string(raw), "tutorial")
	}

	return false
}

func buildPublicEmbeddedURL(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", false
	}

	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		return "", false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", false
	}

	if parsed.Query().Get("ui_mode") == "" {
		query := parsed.Query()
		query.Set("ui_mode", "embedded")
		parsed.RawQuery = query.Encode()
	}

	return parsed.String(), true
}

func (s *FrontendServer) serveRenderedHTMLPage(
	c *gin.Context,
	settingsJSON []byte,
	requestPath string,
	title string,
	bodyHTML string,
) bool {
	seo := buildSEOData(requestPath, settingsJSON)
	cfg := parseSEOConfig(settingsJSON)
	if strings.TrimSpace(title) != "" {
		seo.Title = fmt.Sprintf("%s - %s", strings.TrimSpace(title), normalizeSiteName(cfg.SiteName))
	}
	pageHTML := buildPublicMarkdownHTML(cfg, seo, title, bodyHTML)
	c.Header("Cache-Control", "no-cache")
	if seo.XRobotsTag != "" {
		c.Header("X-Robots-Tag", seo.XRobotsTag)
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", pageHTML)
	c.Abort()
	return true
}

func (s *FrontendServer) serveRenderedEmbeddedPage(
	c *gin.Context,
	settingsJSON []byte,
	requestPath string,
	title string,
	embeddedURL string,
) bool {
	seo := buildSEOData(requestPath, settingsJSON)
	cfg := parseSEOConfig(settingsJSON)
	if strings.TrimSpace(title) != "" && strings.TrimSpace(seo.Title) == "" {
		seo.Title = fmt.Sprintf("%s - %s", strings.TrimSpace(title), normalizeSiteName(cfg.SiteName))
	}
	pageHTML := buildPublicEmbeddedHTML(cfg, seo, title, embeddedURL)
	c.Header("Cache-Control", "no-cache")
	if seo.XRobotsTag != "" {
		c.Header("X-Robots-Tag", seo.XRobotsTag)
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", pageHTML)
	c.Abort()
	return true
}

func (s *FrontendServer) serveRenderedMarkdownPage(
	c *gin.Context,
	settingsJSON []byte,
	requestPath string,
	title string,
	markdown string,
	pageSlug string,
) bool {
	seo := buildSEOData(requestPath, settingsJSON)
	cfg := parseSEOConfig(settingsJSON)
	switch {
	case strings.HasPrefix(requestPath, "/legal/"):
		if doc, ok := findLegalDocument(cfg, strings.TrimPrefix(requestPath, "/legal/")); ok && strings.TrimSpace(doc.SEOTitle) == "" && strings.TrimSpace(title) != "" {
			seo.Title = fmt.Sprintf("%s - %s", strings.TrimSpace(title), normalizeSiteName(cfg.SiteName))
		}
	case strings.HasPrefix(requestPath, "/custom/"):
		if page, ok := findPublicCustomPage(cfg, strings.TrimPrefix(requestPath, "/custom/")); ok && strings.TrimSpace(page.SEOTitle) == "" && strings.TrimSpace(title) != "" {
			seo.Title = fmt.Sprintf("%s - %s", strings.TrimSpace(title), normalizeSiteName(cfg.SiteName))
		}
	case requestPath == "/docs/tutorial":
		if strings.TrimSpace(title) != "" {
			seo.Title = fmt.Sprintf("%s - %s", strings.TrimSpace(title), normalizeSiteName(cfg.SiteName))
		}
	}

	bodyHTML, err := renderMarkdownToHTML(markdown, pageSlug)
	if err != nil {
		return false
	}
	pageHTML := buildPublicMarkdownHTML(cfg, seo, title, bodyHTML)
	c.Header("Cache-Control", "no-cache")
	if seo.XRobotsTag != "" {
		c.Header("X-Robots-Tag", seo.XRobotsTag)
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", pageHTML)
	c.Abort()
	return true
}

func (s *FrontendServer) loadMarkdownFile(slug string) ([]byte, error) {
	filePath := filepath.Join(s.pagesDir, slug+".md")
	cleaned := filepath.Clean(filePath)
	raw, err := os.ReadFile(cleaned)
	if err == nil {
		return raw, nil
	}
	if slug == "tutorial" && os.IsNotExist(err) {
		return []byte("# 教程文档\n\n欢迎使用 Sub2API。\n\n你可以在后台的“系统设置”页面直接编辑这份教程文档正文，保存后会立即影响公开页和用户侧边栏固定入口对应的内容。"), nil
	}
	return nil, err
}

func (s *FrontendServer) loadTutorialHTML() (string, bool) {
	filePath := filepath.Join(s.pagesDir, "tutorial.html")
	cleaned := filepath.Clean(filePath)
	raw, err := os.ReadFile(cleaned)
	if err != nil {
		return "", false
	}
	sanitized := tutorialhtml.SanitizeTutorialHTML(string(raw))
	if strings.TrimSpace(sanitized) == "" {
		return "", false
	}
	return sanitized, true
}

func buildPublicMarkdownHTML(cfg seoConfig, seo seoData, pageTitle, bodyHTML string) []byte {
	siteName := normalizeSiteName(cfg.SiteName)
	siteLogo := html.EscapeString(resolveSEOImageURL(normalizeFrontendBaseURL(cfg.FrontendURL), cfg.SiteLogo))
	pageTitle = html.EscapeString(strings.TrimSpace(pageTitle))
	htmlLang := detectHTMLLang(siteName, seo.Title, seo.Description, pageTitle)
	loginLabel := "Login"
	if htmlLang == "zh-CN" {
		loginLabel = "登录"
	}
	var buf bytes.Buffer
	buf.WriteString(`<!doctype html><html lang="` + htmlLang + `"><head><meta charset="UTF-8">`)
	buf.WriteString(`<meta name="viewport" content="width=device-width, initial-scale=1.0">`)
	buf.WriteString(`<title>` + html.EscapeString(seo.Title) + `</title>`)
	buf.Write(buildSEOMetaTags(seo))
	buf.WriteString(`<style>body{margin:0;font-family:ui-sans-serif,system-ui,sans-serif;background:#f8fafc;color:#111827}header{background:#fff;border-bottom:1px solid #e5e7eb}main{max-width:960px;margin:0 auto;padding:32px 16px}article{background:#fff;border:1px solid #e5e7eb;border-radius:20px;overflow:hidden;box-shadow:0 1px 2px rgba(0,0,0,.04)}.hero{padding:24px;border-bottom:1px solid #e5e7eb}.hero p{margin:0;color:#0f766e;font-size:14px;font-weight:600}.hero h1{margin:12px 0 0;font-size:32px;line-height:1.2}.content{padding:32px;line-height:1.75}.content h1,.content h2,.content h3,.content h4{line-height:1.25}.content h1{font-size:32px}.content h2{font-size:28px;margin-top:28px}.content h3{font-size:22px;margin-top:24px}.content p{margin:0 0 16px}.content a{color:#0f766e}.brand{max-width:960px;margin:0 auto;padding:16px;display:flex;align-items:center;justify-content:space-between;gap:16px}.brand-left{display:flex;align-items:center;gap:12px}.brand-logo{width:40px;height:40px;border-radius:12px;overflow:hidden;background:#fff;border:1px solid #e5e7eb}.brand-logo img{width:100%;height:100%;object-fit:contain}.brand-name{font-weight:700;color:#111827;text-decoration:none}.login{display:inline-flex;align-items:center;justify-content:center;padding:10px 16px;border-radius:10px;background:#0f766e;color:#fff;text-decoration:none;font-weight:600}ul,ol{margin:0 0 16px 24px}code{background:#f3f4f6;padding:2px 6px;border-radius:6px}pre{background:#111827;color:#f9fafb;padding:16px;border-radius:12px;overflow:auto}img{max-width:100%;height:auto;border-radius:12px;margin:16px 0}</style>`)
	buf.WriteString(`</head><body>`)
	buf.WriteString(`<header><div class="brand"><div class="brand-left"><div class="brand-logo">`)
	if siteLogo != "" {
		buf.WriteString(`<img src="` + siteLogo + `" alt="Logo">`)
	}
	buf.WriteString(`</div><a class="brand-name" href="/home">` + html.EscapeString(siteName) + `</a></div><a class="login" href="/login">` + loginLabel + `</a></div></header>`)
	buf.WriteString(`<main><article><div class="hero"><p>` + html.EscapeString(siteName) + `</p><h1>` + pageTitle + `</h1></div><div class="content">`)
	buf.WriteString(bodyHTML)
	buf.WriteString(`</div></article></main></body></html>`)
	return buf.Bytes()
}

func buildPublicEmbeddedHTML(cfg seoConfig, seo seoData, pageTitle, embeddedURL string) []byte {
	siteName := normalizeSiteName(cfg.SiteName)
	siteLogo := html.EscapeString(resolveSEOImageURL(normalizeFrontendBaseURL(cfg.FrontendURL), cfg.SiteLogo))
	pageTitle = html.EscapeString(strings.TrimSpace(pageTitle))
	embeddedURL = html.EscapeString(strings.TrimSpace(embeddedURL))
	htmlLang := detectHTMLLang(siteName, seo.Title, seo.Description, pageTitle)
	loginLabel := "Login"
	openLabel := "Open in a new tab"
	if htmlLang == "zh-CN" {
		loginLabel = "登录"
		openLabel = "打开原始页面"
	}

	var buf bytes.Buffer
	buf.WriteString(`<!doctype html><html lang="` + htmlLang + `"><head><meta charset="UTF-8">`)
	buf.WriteString(`<meta name="viewport" content="width=device-width, initial-scale=1.0">`)
	buf.WriteString(`<title>` + html.EscapeString(seo.Title) + `</title>`)
	buf.Write(buildSEOMetaTags(seo))
	buf.WriteString(`<style>body{margin:0;font-family:ui-sans-serif,system-ui,sans-serif;background:#f8fafc;color:#111827}header{background:#fff;border-bottom:1px solid #e5e7eb}main{max-width:1100px;margin:0 auto;padding:32px 16px}.shell{background:#fff;border:1px solid #e5e7eb;border-radius:20px;overflow:hidden;box-shadow:0 1px 2px rgba(0,0,0,.04)}.hero{padding:24px;border-bottom:1px solid #e5e7eb}.hero p{margin:0;color:#0f766e;font-size:14px;font-weight:600}.hero h1{margin:12px 0 0;font-size:32px;line-height:1.2}.hero a{display:inline-flex;margin-top:16px;padding:10px 16px;border-radius:10px;background:#0f766e;color:#fff;text-decoration:none;font-weight:600}.frame-wrap{padding:0 24px 24px}.frame-note{padding:24px 0 16px;line-height:1.7;color:#4b5563}.frame{width:100%;min-height:70vh;border:0;border-top:1px solid #e5e7eb;background:#fff}.brand{max-width:1100px;margin:0 auto;padding:16px;display:flex;align-items:center;justify-content:space-between;gap:16px}.brand-left{display:flex;align-items:center;gap:12px}.brand-logo{width:40px;height:40px;border-radius:12px;overflow:hidden;background:#fff;border:1px solid #e5e7eb}.brand-logo img{width:100%;height:100%;object-fit:contain}.brand-name{font-weight:700;color:#111827;text-decoration:none}.login{display:inline-flex;align-items:center;justify-content:center;padding:10px 16px;border-radius:10px;background:#0f766e;color:#fff;text-decoration:none;font-weight:600}</style>`)
	buf.WriteString(`</head><body>`)
	buf.WriteString(`<header><div class="brand"><div class="brand-left"><div class="brand-logo">`)
	if siteLogo != "" {
		buf.WriteString(`<img src="` + siteLogo + `" alt="Logo">`)
	}
	buf.WriteString(`</div><a class="brand-name" href="/home">` + html.EscapeString(siteName) + `</a></div><a class="login" href="/login">` + loginLabel + `</a></div></header>`)
	buf.WriteString(`<main><article class="shell"><div class="hero"><p>` + html.EscapeString(siteName) + `</p><h1>` + pageTitle + `</h1><a href="` + embeddedURL + `" target="_blank" rel="noopener noreferrer">` + openLabel + `</a></div><div class="frame-wrap"><p class="frame-note">` + html.EscapeString(seo.Description) + `</p><iframe class="frame" src="` + embeddedURL + `" loading="lazy" referrerpolicy="strict-origin-when-cross-origin"></iframe></div></article></main></body></html>`)
	return buf.Bytes()
}

func renderMarkdownToHTML(markdown string, pageSlug string) (string, error) {
	markdown = rewriteRelativeMarkdownImages(markdown, pageSlug)
	var buf bytes.Buffer
	if err := serverMarkdownRenderer.Convert([]byte(markdown), &buf); err != nil {
		return "", err
	}
	return tutorialhtml.SanitizeTutorialHTML(buf.String()), nil
}

func rewriteRelativeMarkdownImages(markdown string, pageSlug string) string {
	if strings.TrimSpace(pageSlug) == "" {
		return markdown
	}
	return markdownImagePattern.ReplaceAllStringFunc(markdown, func(match string) string {
		parts := markdownImagePattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		alt, src := parts[1], parts[2]
		if !isRelativeMarkdownAsset(src) {
			return match
		}
		return fmt.Sprintf("![%s](%s)", alt, buildPageImageURL(pageSlug, src))
	})
}

func isRelativeMarkdownAsset(src string) bool {
	trimmed := strings.TrimSpace(src)
	if trimmed == "" || strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "//") {
		return false
	}
	if matched, _ := regexp.MatchString(`^[a-zA-Z][a-zA-Z0-9+.-]*:`, trimmed); matched {
		return false
	}
	if strings.Contains(trimmed, `\`) {
		return false
	}
	for _, part := range strings.Split(trimmed, "/") {
		if part == ".." {
			return false
		}
	}
	return true
}

func buildPageImageURL(pageSlug, src string) string {
	trimmed := strings.TrimSpace(src)
	parts := strings.SplitN(trimmed, "?", 2)
	pathPart := parts[0]
	query := ""
	if len(parts) == 2 {
		query = "?" + parts[1]
	}
	encodedParts := make([]string, 0, len(strings.Split(pathPart, "/")))
	for _, part := range strings.Split(pathPart, "/") {
		if part == "" || part == "." {
			continue
		}
		encodedParts = append(encodedParts, url.PathEscape(part))
	}
	return fmt.Sprintf("/api/v1/pages/%s/images/%s%s", pageSlug, strings.Join(encodedParts, "/"), query)
}

func (s *FrontendServer) serveOGImage(c *gin.Context) {
	kind := strings.TrimPrefix(normalizeRequestPath(c.Request.URL.Path), "/og/")
	settingsJSON, ok := s.loadSettingsJSON(c)
	if !ok {
		c.Status(http.StatusNotFound)
		c.Abort()
		return
	}
	cfg := parseSEOConfig(settingsJSON)
	image, found := buildDynamicOGSVG(cfg, kind)
	if !found {
		c.Status(http.StatusNotFound)
		c.Abort()
		return
	}
	c.Header("Cache-Control", "public, max-age=300")
	c.Data(http.StatusOK, "image/svg+xml; charset=utf-8", image)
	c.Abort()
}

func buildDynamicOGSVG(cfg seoConfig, kind string) ([]byte, bool) {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return nil, false
	}
	siteName := html.EscapeString(normalizeSiteName(cfg.SiteName))
	subtitle := html.EscapeString(buildHomeDescription(cfg))
	title := siteName
	badge := "SEO Ready"
	footer := "Open Graph · Sitemap · Canonical"
	if kind != "" && kind != "home.svg" {
		title = siteName + " / " + html.EscapeString(strings.TrimSuffix(kind, ".svg"))
	}

	if strings.HasPrefix(kind, "legal-") {
		id := strings.TrimSuffix(strings.TrimPrefix(kind, "legal-"), ".svg")
		if doc, ok := findLegalDocument(cfg, id); ok {
			title = html.EscapeString(firstNonEmpty(strings.TrimSpace(doc.SEOTitle), strings.TrimSpace(doc.Title)))
			subtitle = html.EscapeString(buildPageSEODescription(doc.SEODescription, cfg.SEODefaultDescription, doc.ContentMD, ""))
		} else {
			return nil, false
		}
	}
	if strings.HasPrefix(kind, "custom-") {
		id := strings.TrimSuffix(strings.TrimPrefix(kind, "custom-"), ".svg")
		if id == "tutorial" {
			title = html.EscapeString("教程文档")
			subtitle = html.EscapeString(buildPageSEODescription("", cfg.SEODefaultDescription, "", fmt.Sprintf("阅读教程文档，了解 %s 的接入说明、使用方式与常见问题。", normalizeSiteName(cfg.SiteName))))
		} else if page, ok := findPublicCustomPage(cfg, id); ok {
			title = html.EscapeString(firstNonEmpty(strings.TrimSpace(page.SEOTitle), strings.TrimSpace(page.Label)))
			subtitle = html.EscapeString(buildPageSEODescription(page.SEODescription, cfg.SEODefaultDescription, "", ""))
		} else {
			return nil, false
		}
	}
	if kind != "home.svg" && !strings.HasPrefix(kind, "legal-") && !strings.HasPrefix(kind, "custom-") {
		return nil, false
	}
	if detectHTMLLang(siteName, title, subtitle) == "zh-CN" {
		badge = "SEO 已就绪"
	}
	logoData := ""
	if raw := strings.TrimSpace(cfg.SiteLogo); raw != "" && inlineOGLogoDataURLRE.MatchString(raw) {
		logoData = raw
	}

	var logo string
	if logoData != "" {
		logo = `<image x="120" y="120" width="88" height="88" href="` + html.EscapeString(logoData) + `" />`
	} else {
		logo = `<rect x="120" y="120" width="88" height="88" rx="22" fill="#0F766E"/><path d="M146 164H182" stroke="white" stroke-width="14" stroke-linecap="round"/><path d="M164 146V182" stroke="white" stroke-width="14" stroke-linecap="round"/>`
	}

	svg := fmt.Sprintf(`<svg width="1200" height="630" viewBox="0 0 1200 630" fill="none" xmlns="http://www.w3.org/2000/svg">
<rect width="1200" height="630" fill="#F8FAFC"/>
<rect x="56" y="56" width="1088" height="518" rx="32" fill="url(#bg)"/>
<circle cx="974" cy="122" r="88" fill="#14B8A6" fill-opacity="0.12"/>
<circle cx="1056" cy="204" r="48" fill="#0F766E" fill-opacity="0.16"/>
%s
<text x="120" y="302" fill="#E2FDF8" font-family="system-ui, sans-serif" font-size="28" font-weight="600">Unified AI Gateway</text>
<text x="120" y="382" fill="white" font-family="system-ui, sans-serif" font-size="64" font-weight="800">%s</text>
<text x="120" y="448" fill="#D1FAF5" font-family="system-ui, sans-serif" font-size="30" font-weight="500">%s</text>
<rect x="120" y="488" width="220" height="52" rx="26" fill="white" fill-opacity="0.12"/>
<text x="152" y="522" fill="white" font-family="system-ui, sans-serif" font-size="24" font-weight="700">%s</text>
<text x="382" y="522" fill="#D1FAF5" font-family="system-ui, sans-serif" font-size="24" font-weight="600">%s</text>
<defs>
  <linearGradient id="bg" x1="120" y1="80" x2="1080" y2="550" gradientUnits="userSpaceOnUse">
    <stop stop-color="#0F766E"/>
    <stop offset="1" stop-color="#115E59"/>
  </linearGradient>
</defs>
</svg>`, logo, title, subtitle, html.EscapeString(badge), html.EscapeString(footer))

	return []byte(svg), true
}

func encodeInlineSVG(svg []byte) string {
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(svg)
}
