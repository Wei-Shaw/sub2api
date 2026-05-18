//go:build embed

package seo

import (
	"bytes"
	"fmt"
	"html"
	"strings"
)

// HeadInjector renders SEO <head> content and merges it into a base HTML template.
// Render NEVER returns an error: any internal failure falls back to a noindex default
// so SEO never blocks a page response. This is the documented contract.
type HeadInjector struct {
	reg *Registry
}

// NewHeadInjector creates an injector backed by the given registry.
func NewHeadInjector(reg *Registry) *HeadInjector {
	return &HeadInjector{reg: reg}
}

// Render produces the final HTML for the given path + lang + public settings JSON.
// baseHTML is the embedded index.html (with the placeholder <title>).
// baseURL is the canonical site root (e.g. "https://api.example.com"); pass per-request
// to avoid shared mutable state when serving multiple domains.
func (i *HeadInjector) Render(baseHTML []byte, path, lang string, settingsJSON []byte, baseURL string) []byte {
	rec, lp := i.reg.Resolve(path, lang, settingsJSON)
	head := i.buildHead(rec, lp, path, lang, baseURL)

	out := replaceTitle(baseHTML, lp.Title, i.reg.Site.Name)

	headClose := []byte("</head>")
	idx := bytes.Index(out, headClose)
	if idx < 0 {
		return out
	}
	var buf bytes.Buffer
	buf.Write(out[:idx])
	buf.WriteString(head)
	buf.Write(out[idx:])
	return buf.Bytes()
}

func (i *HeadInjector) buildHead(rec Record, lp LangPayload, path, lang, baseURL string) string {
	site := i.reg.Site
	if baseURL == "" {
		baseURL = site.BaseURL
	}
	pageURL := baseURL + path

	esc := html.EscapeString
	var b strings.Builder

	// (1) Basic meta
	b.WriteString(fmt.Sprintf(`<meta name="description" content="%s" />`, esc(lp.Description)))
	if len(lp.Keywords) > 0 {
		b.WriteString(fmt.Sprintf(`<meta name="keywords" content="%s" />`, esc(strings.Join(lp.Keywords, ","))))
	}
	robotsVal := "noindex,follow"
	if rec.Indexable {
		robotsVal = "index,follow"
	}
	b.WriteString(fmt.Sprintf(`<meta name="robots" content="%s" />`, robotsVal))
	b.WriteString(fmt.Sprintf(`<meta name="author" content="%s" />`, esc(site.Organization.Name)))
	b.WriteString(fmt.Sprintf(`<link rel="canonical" href="%s" />`, esc(pageURL)))

	// (2) hreflang (only for indexable pages)
	if rec.Indexable {
		b.WriteString(fmt.Sprintf(`<link rel="alternate" hreflang="zh-CN" href="%s" />`, esc(pageURL)))
		b.WriteString(fmt.Sprintf(`<link rel="alternate" hreflang="en" href="%s?lang=en" />`, esc(pageURL)))
		b.WriteString(fmt.Sprintf(`<link rel="alternate" hreflang="x-default" href="%s" />`, esc(pageURL)))
	}

	// (3) Open Graph
	b.WriteString(`<meta property="og:type" content="website" />`)
	b.WriteString(fmt.Sprintf(`<meta property="og:site_name" content="%s" />`, esc(site.Name)))
	b.WriteString(fmt.Sprintf(`<meta property="og:title" content="%s" />`, esc(lp.Title)))
	b.WriteString(fmt.Sprintf(`<meta property="og:description" content="%s" />`, esc(lp.Description)))
	b.WriteString(fmt.Sprintf(`<meta property="og:url" content="%s" />`, esc(pageURL)))
	if lp.OgImage != "" {
		ogImg := lp.OgImage
		if !strings.HasPrefix(ogImg, "http") {
			ogImg = baseURL + ogImg
		}
		b.WriteString(fmt.Sprintf(`<meta property="og:image" content="%s" />`, esc(ogImg)))
	}
	ogLocale := "zh_CN"
	if lang == "en" {
		ogLocale = "en_US"
	}
	b.WriteString(fmt.Sprintf(`<meta property="og:locale" content="%s" />`, ogLocale))

	// (4) Twitter Card
	b.WriteString(`<meta name="twitter:card" content="summary_large_image" />`)
	b.WriteString(fmt.Sprintf(`<meta name="twitter:title" content="%s" />`, esc(lp.Title)))
	b.WriteString(fmt.Sprintf(`<meta name="twitter:description" content="%s" />`, esc(lp.Description)))
	if lp.OgImage != "" {
		ogImg := lp.OgImage
		if !strings.HasPrefix(ogImg, "http") {
			ogImg = baseURL + ogImg
		}
		b.WriteString(fmt.Sprintf(`<meta name="twitter:image" content="%s" />`, esc(ogImg)))
	}

	// (5) JSON-LD
	if len(rec.JsonLd) > 0 {
		siteForGraph := site
		siteForGraph.BaseURL = baseURL
		graph := RenderGraph(siteForGraph, rec, lp, path, lang)
		b.WriteString(fmt.Sprintf(`<script type="application/ld+json" nonce="__CSP_NONCE_VALUE__">%s</script>`, graph))
	}

	return b.String()
}

// replaceTitle swaps the <title>...</title> block with newTitle.
func replaceTitle(htmlBytes []byte, newTitle, siteName string) []byte {
	start := bytes.Index(htmlBytes, []byte("<title>"))
	end := bytes.Index(htmlBytes, []byte("</title>"))
	if start < 0 || end < 0 || end <= start {
		return htmlBytes
	}
	final := newTitle
	if final == "" {
		final = siteName
	}
	var buf bytes.Buffer
	buf.Write(htmlBytes[:start])
	buf.WriteString("<title>")
	buf.WriteString(html.EscapeString(final))
	buf.WriteString("</title>")
	buf.Write(htmlBytes[end+len("</title>"):])
	return buf.Bytes()
}
