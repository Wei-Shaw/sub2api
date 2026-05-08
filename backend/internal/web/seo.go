//go:build embed

package web

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	defaultSiteName        = "Sub2API"
	defaultSiteDescription = "Sub2API is an AI API gateway platform for unified model access, account pooling, routing, billing, and operations management."
)

var (
	whitespacePattern = regexp.MustCompile(`\s+`)
	htmlTagPattern    = regexp.MustCompile(`<[^>]+>`)
)

type seoConfig struct {
	SiteName                string            `json:"site_name"`
	SiteSubtitle            string            `json:"site_subtitle"`
	SiteLogo                string            `json:"site_logo"`
	FrontendURL             string            `json:"frontend_url"`
	DocURL                  string            `json:"doc_url"`
	HomeContent             string            `json:"home_content"`
	SEODefaultTitle         string            `json:"seo_default_title"`
	SEOHomeTitle            string            `json:"seo_home_title"`
	SEODefaultDescription   string            `json:"seo_default_description"`
	SEOHomeDescription      string            `json:"seo_home_description"`
	SEODefaultOGImage       string            `json:"seo_default_og_image"`
	SEODefaultRobots        string            `json:"seo_default_robots"`
	SEOHomeRobots           string            `json:"seo_home_robots"`
	LoginAgreementEnabled   bool              `json:"login_agreement_enabled"`
	LoginAgreementUpdatedAt string            `json:"login_agreement_updated_at"`
	LoginAgreementDocuments []seoLegalDoc     `json:"login_agreement_documents"`
	CustomMenuItems         []seoCustomMenu   `json:"custom_menu_items"`
}

type seoLegalDoc struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	ContentMD      string `json:"content_md"`
	SEOTitle       string `json:"seo_title"`
	SEODescription string `json:"seo_description"`
	SEOOGImage     string `json:"seo_og_image"`
	SEORobots      string `json:"seo_robots"`
}

type seoCustomMenu struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	URL            string `json:"url"`
	PageSlug       string `json:"page_slug"`
	SEOTitle       string `json:"seo_title"`
	SEODescription string `json:"seo_description"`
	SEOOGImage     string `json:"seo_og_image"`
	SEORobots      string `json:"seo_robots"`
	Visibility     string `json:"visibility"`
}

type seoData struct {
	SiteName         string
	Title            string
	Description      string
	CanonicalURL     string
	ImageURL         string
	Robots           string
	Type             string
	JSONLD           string
	XRobotsTag       string
	OpenGraphLocale  string
	TwitterCard      string
	ShouldInjectHead bool
}

type sitemapURLSet struct {
	XMLName xml.Name      `xml:"urlset"`
	Xmlns   string        `xml:"xmlns,attr"`
	URLs    []sitemapURLEntry `xml:"url"`
}

type sitemapURLEntry struct {
	Loc        string `xml:"loc"`
	LastMod    string `xml:"lastmod,omitempty"`
	ChangeFreq string `xml:"changefreq,omitempty"`
	Priority   string `xml:"priority,omitempty"`
}

func parseSEOConfig(settingsJSON []byte) seoConfig {
	var cfg seoConfig
	_ = json.Unmarshal(settingsJSON, &cfg)
	return cfg
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func buildSEOData(requestPath string, settingsJSON []byte) seoData {
	cfg := parseSEOConfig(settingsJSON)
	siteName := normalizeSiteName(cfg.SiteName)
	baseURL := normalizeFrontendBaseURL(cfg.FrontendURL)
	canonicalURL := resolveCanonicalURL(baseURL, requestPath)
	if requestPath == "/" || requestPath == "/home" {
		canonicalURL = resolveCanonicalURL(baseURL, "/")
	}

	data := seoData{
		SiteName:         siteName,
		Title:            resolveDefaultSEOTitle(cfg, siteName),
		Description:      buildDefaultDescription(cfg),
		CanonicalURL:     canonicalURL,
		ImageURL:         resolveDefaultSEOImage(baseURL, cfg),
		Robots:           resolveDefaultRobots(cfg),
		XRobotsTag:       resolveDefaultRobots(cfg),
		Type:             "website",
		OpenGraphLocale:  "en_US",
		TwitterCard:      "summary_large_image",
		ShouldInjectHead: true,
	}

	switch {
	case requestPath == "/" || requestPath == "/home":
		data.Title = resolveHomeSEOTitle(cfg, siteName)
		data.Description = buildHomeDescription(cfg)
		data.Robots = resolveHomeRobots(cfg)
		data.XRobotsTag = data.Robots
		data.JSONLD = buildWebsiteJSONLD(siteName, data.Description, canonicalURL, data.ImageURL)
	case strings.HasPrefix(requestPath, "/legal/"):
		if doc, ok := findLegalDocument(cfg, strings.TrimPrefix(requestPath, "/legal/")); ok {
			if title := strings.TrimSpace(doc.SEOTitle); title != "" {
				data.Title = title
			} else {
				data.Title = fmt.Sprintf("%s - %s", doc.Title, siteName)
			}
			if desc := strings.TrimSpace(doc.SEODescription); desc != "" {
				data.Description = desc
			} else {
				data.Description = fmt.Sprintf("Read %s on %s.", doc.Title, siteName)
			}
			if robots := strings.TrimSpace(doc.SEORobots); robots != "" {
				data.Robots = robots
			} else {
				data.Robots = resolveLegalRobots(cfg)
			}
			data.XRobotsTag = data.Robots
			data.Type = "article"
			if rawImage := strings.TrimSpace(doc.SEOOGImage); rawImage != "" {
				data.ImageURL = resolveSEOImageURL(baseURL, rawImage)
			} else if baseURL != "" {
				data.ImageURL = strings.TrimRight(baseURL, "/") + "/og/legal-" + url.PathEscape(strings.TrimSpace(doc.ID)) + ".svg"
			}
			data.JSONLD = buildArticleJSONLD(data.Title, data.Description, canonicalURL, data.ImageURL, cfg.LoginAgreementUpdatedAt)
		}
	case strings.HasPrefix(requestPath, "/custom/"):
		if page, ok := findPublicCustomPage(cfg, strings.TrimPrefix(requestPath, "/custom/")); ok {
			if title := strings.TrimSpace(page.SEOTitle); title != "" {
				data.Title = title
			} else {
				data.Title = fmt.Sprintf("%s - %s", page.Label, siteName)
			}
			if desc := strings.TrimSpace(page.SEODescription); desc != "" {
				data.Description = desc
			} else {
				data.Description = fmt.Sprintf("Learn more about %s on %s.", page.Label, siteName)
			}
			if robots := strings.TrimSpace(page.SEORobots); robots != "" {
				data.Robots = robots
			} else {
				data.Robots = resolvePublicRobots(cfg)
			}
			data.XRobotsTag = data.Robots
			if rawImage := strings.TrimSpace(page.SEOOGImage); rawImage != "" {
				data.ImageURL = resolveSEOImageURL(baseURL, rawImage)
			} else if baseURL != "" {
				data.ImageURL = strings.TrimRight(baseURL, "/") + "/og/custom-" + url.PathEscape(strings.TrimSpace(page.ID)) + ".svg"
			}
			if strings.HasPrefix(strings.TrimSpace(page.URL), "md:") || strings.TrimSpace(page.PageSlug) != "" {
				data.Type = "article"
				data.JSONLD = buildArticleJSONLD(data.Title, data.Description, canonicalURL, data.ImageURL, cfg.LoginAgreementUpdatedAt)
			} else {
				data.JSONLD = buildWebPageJSONLD(data.Title, data.Description, canonicalURL, data.ImageURL)
			}
		}
	default:
		data.Robots = resolveDefaultRobots(cfg)
		data.XRobotsTag = data.Robots
	}

	return data
}

func buildNotFoundSEOData(requestPath string, settingsJSON []byte) seoData {
	cfg := parseSEOConfig(settingsJSON)
	siteName := normalizeSiteName(cfg.SiteName)
	baseURL := normalizeFrontendBaseURL(cfg.FrontendURL)
	canonicalURL := resolveCanonicalURL(baseURL, requestPath)
	return seoData{
		SiteName:         siteName,
		Title:            firstNonEmpty(strings.TrimSpace(cfg.SEODefaultTitle), siteName) + " - 404",
		Description:      firstNonEmpty(normalizePlainText(cfg.SEODefaultDescription), "Page not found."),
		CanonicalURL:     canonicalURL,
		ImageURL:         resolveDefaultSEOImage(baseURL, cfg),
		Robots:           "noindex, nofollow",
		XRobotsTag:       "noindex, nofollow",
		Type:             "website",
		OpenGraphLocale:  "en_US",
		TwitterCard:      "summary_large_image",
		ShouldInjectHead: true,
	}
}

func buildSEOMetaTags(data seoData) []byte {
	if !data.ShouldInjectHead {
		return nil
	}

	var buf strings.Builder
	writeMetaTag(&buf, "description", data.Description)
	writeMetaTag(&buf, "robots", data.Robots)
	writeCanonicalTag(&buf, data.CanonicalURL)
	writePropertyMetaTag(&buf, "og:type", data.Type)
	writePropertyMetaTag(&buf, "og:title", data.Title)
	writePropertyMetaTag(&buf, "og:description", data.Description)
	writePropertyMetaTag(&buf, "og:url", data.CanonicalURL)
	writePropertyMetaTag(&buf, "og:site_name", data.SiteName)
	writePropertyMetaTag(&buf, "og:locale", data.OpenGraphLocale)
	if data.ImageURL != "" {
		writePropertyMetaTag(&buf, "og:image", data.ImageURL)
	}
	writeMetaTag(&buf, "twitter:card", data.TwitterCard)
	writeMetaTag(&buf, "twitter:title", data.Title)
	writeMetaTag(&buf, "twitter:description", data.Description)
	if data.ImageURL != "" {
		writeMetaTag(&buf, "twitter:image", data.ImageURL)
	}
	if data.JSONLD != "" {
		buf.WriteString(`<script type="application/ld+json">`)
		buf.WriteString(data.JSONLD)
		buf.WriteString(`</script>`)
	}
	return []byte(buf.String())
}

func buildRobotsTXT(settingsJSON []byte) []byte {
	cfg := parseSEOConfig(settingsJSON)
	baseURL := normalizeFrontendBaseURL(cfg.FrontendURL)
	sitemapURL := ""
	if baseURL != "" {
		sitemapURL = strings.TrimRight(baseURL, "/") + "/sitemap.xml"
	}

	var lines []string
	lines = append(lines, "User-agent: *")
	lines = append(lines, "Allow: /")
	lines = append(lines, "Disallow: /admin")
	lines = append(lines, "Disallow: /dashboard")
	lines = append(lines, "Disallow: /keys")
	lines = append(lines, "Disallow: /usage")
	lines = append(lines, "Disallow: /profile")
	lines = append(lines, "Disallow: /subscriptions")
	lines = append(lines, "Disallow: /orders")
	lines = append(lines, "Disallow: /purchase")
	lines = append(lines, "Disallow: /payment")
	lines = append(lines, "Disallow: /monitor")
	lines = append(lines, "Disallow: /affiliate")
	lines = append(lines, "Disallow: /available-channels")
	lines = append(lines, "Disallow: /login")
	lines = append(lines, "Disallow: /register")
	lines = append(lines, "Disallow: /forgot-password")
	lines = append(lines, "Disallow: /reset-password")
	lines = append(lines, "Disallow: /auth/")
	if sitemapURL != "" {
		lines = append(lines, "Sitemap: "+sitemapURL)
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

func buildSitemapXML(settingsJSON []byte) []byte {
	cfg := parseSEOConfig(settingsJSON)
	baseURL := normalizeFrontendBaseURL(cfg.FrontendURL)
	if baseURL == "" {
		return nil
	}

	urls := []sitemapURLEntry{
		{
			Loc:        strings.TrimRight(baseURL, "/") + "/",
			ChangeFreq: "daily",
			Priority:   "1.0",
		},
	}

	lastMod := normalizeDate(cfg.LoginAgreementUpdatedAt)
	for _, doc := range cfg.LoginAgreementDocuments {
		id := strings.TrimSpace(doc.ID)
		if id == "" {
			continue
		}
		urls = append(urls, sitemapURLEntry{
			Loc:        strings.TrimRight(baseURL, "/") + "/legal/" + url.PathEscape(id),
			LastMod:    lastMod,
			ChangeFreq: "monthly",
			Priority:   "0.5",
		})
	}

	for _, item := range cfg.CustomMenuItems {
		if item.Visibility == "admin" {
			continue
		}
		if strings.TrimSpace(item.ID) == "" {
			continue
		}
		urls = append(urls, sitemapURLEntry{
			Loc:        strings.TrimRight(baseURL, "/") + "/custom/" + url.PathEscape(strings.TrimSpace(item.ID)),
			ChangeFreq: "weekly",
			Priority:   "0.7",
		})
	}

	sort.Slice(urls, func(i, j int) bool { return urls[i].Loc < urls[j].Loc })

	payload, err := xml.MarshalIndent(sitemapURLSet{
		Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9",
		URLs:  urls,
	}, "", "  ")
	if err != nil {
		return nil
	}
	return append([]byte(xml.Header), payload...)
}

func injectSEOTags(htmlDoc []byte, metaTags []byte) []byte {
	if len(metaTags) == 0 {
		return htmlDoc
	}
	headClose := []byte("</head>")
	return bytes.Replace(htmlDoc, headClose, append(metaTags, headClose...), 1)
}

func injectSEOTitle(htmlDoc []byte, title string) []byte {
	if strings.TrimSpace(title) == "" {
		return htmlDoc
	}
	titleStart := bytes.Index(htmlDoc, []byte("<title>"))
	titleEnd := bytes.Index(htmlDoc, []byte("</title>"))
	if titleStart == -1 || titleEnd == -1 || titleEnd <= titleStart {
		return htmlDoc
	}
	newTitle := []byte("<title>" + html.EscapeString(title) + "</title>")
	var buf bytes.Buffer
	buf.Write(htmlDoc[:titleStart])
	buf.Write(newTitle)
	buf.Write(htmlDoc[titleEnd+len("</title>"):])
	return buf.Bytes()
}

func normalizeFrontendBaseURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/")
}

func normalizeSiteName(siteName string) string {
	siteName = strings.TrimSpace(siteName)
	if siteName == "" {
		return defaultSiteName
	}
	return siteName
}

func resolveCanonicalURL(baseURL, requestPath string) string {
	if baseURL == "" {
		return ""
	}
	if requestPath == "" {
		requestPath = "/"
	}
	cleanPath := path.Clean("/" + strings.TrimSpace(requestPath))
	if cleanPath == "." {
		cleanPath = "/"
	}
	if cleanPath == "/" {
		return strings.TrimRight(baseURL, "/") + "/"
	}
	return strings.TrimRight(baseURL, "/") + cleanPath
}

func resolveSEOImageURL(baseURL, image string) string {
	image = strings.TrimSpace(image)
	if image == "" {
		if baseURL == "" {
			return ""
		}
		return strings.TrimRight(baseURL, "/") + "/og/home.svg"
	}
	if strings.HasPrefix(image, "http://") || strings.HasPrefix(image, "https://") {
		return image
	}
	if strings.HasPrefix(image, "data:image/") {
		return image
	}
	if baseURL == "" {
		return image
	}
	if !strings.HasPrefix(image, "/") {
		image = "/" + image
	}
	return strings.TrimRight(baseURL, "/") + image
}

func buildDefaultDescription(cfg seoConfig) string {
	if desc := normalizePlainText(cfg.SEODefaultDescription); desc != "" {
		return desc
	}
	subtitle := normalizePlainText(cfg.SiteSubtitle)
	if subtitle != "" {
		return subtitle
	}
	return defaultSiteDescription
}

func buildHomeDescription(cfg seoConfig) string {
	if desc := normalizePlainText(cfg.SEOHomeDescription); desc != "" {
		return desc
	}
	if desc := normalizePlainText(cfg.SiteSubtitle); desc != "" {
		return desc
	}
	if desc := normalizePlainText(cfg.HomeContent); desc != "" {
		return desc
	}
	return defaultSiteDescription
}

func normalizePlainText(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return ""
	}
	withoutTags := htmlTagPattern.ReplaceAllString(raw, " ")
	normalized := whitespacePattern.ReplaceAllString(strings.TrimSpace(html.UnescapeString(withoutTags)), " ")
	return strings.TrimSpace(normalized)
}

func findLegalDocument(cfg seoConfig, id string) (seoLegalDoc, bool) {
	id = strings.TrimSpace(id)
	for _, doc := range cfg.LoginAgreementDocuments {
		if strings.TrimSpace(doc.ID) == id {
			return doc, true
		}
	}
	return seoLegalDoc{}, false
}

func findPublicCustomPage(cfg seoConfig, id string) (seoCustomMenu, bool) {
	id = strings.TrimSpace(id)
	for _, item := range cfg.CustomMenuItems {
		if item.Visibility == "admin" {
			continue
		}
		if strings.TrimSpace(item.ID) == id {
			return item, true
		}
	}
	return seoCustomMenu{}, false
}

func buildWebsiteJSONLD(siteName, description, canonicalURL, imageURL string) string {
	payload := map[string]any{
		"@context":    "https://schema.org",
		"@type":       "WebSite",
		"name":        siteName,
		"description": description,
		"url":         canonicalURL,
	}
	if imageURL != "" {
		payload["image"] = imageURL
	}
	return marshalJSONLD(payload)
}

func resolveDefaultSEOTitle(cfg seoConfig, siteName string) string {
	if title := strings.TrimSpace(cfg.SEODefaultTitle); title != "" {
		return title
	}
	return siteName + " - AI API Gateway"
}

func resolveHomeSEOTitle(cfg seoConfig, siteName string) string {
	if title := strings.TrimSpace(cfg.SEOHomeTitle); title != "" {
		return title
	}
	return siteName + " - AI API Gateway"
}

func resolveDefaultSEOImage(baseURL string, cfg seoConfig) string {
	if image := resolveSEOImageURL(baseURL, cfg.SEODefaultOGImage); strings.TrimSpace(image) != "" {
		return image
	}
	return resolveSEOImageURL(baseURL, cfg.SiteLogo)
}

func resolveDefaultRobots(cfg seoConfig) string {
	if robots := strings.TrimSpace(cfg.SEODefaultRobots); robots != "" {
		return robots
	}
	return "noindex, nofollow"
}

func resolvePublicRobots(cfg seoConfig) string {
	if robots := strings.TrimSpace(cfg.SEODefaultRobots); robots != "" {
		return robots
	}
	return "index, follow"
}

func resolveHomeRobots(cfg seoConfig) string {
	if robots := strings.TrimSpace(cfg.SEOHomeRobots); robots != "" {
		return robots
	}
	return resolvePublicRobots(cfg)
}

func resolveLegalRobots(cfg seoConfig) string {
	if robots := strings.TrimSpace(cfg.SEODefaultRobots); robots != "" {
		if strings.EqualFold(strings.ReplaceAll(robots, " ", ""), "noindex,nofollow") {
			return "noindex, nofollow"
		}
		return "noindex, follow"
	}
	return "noindex, follow"
}

func buildArticleJSONLD(title, description, canonicalURL, imageURL, updatedAt string) string {
	payload := map[string]any{
		"@context":    "https://schema.org",
		"@type":       "Article",
		"headline":    title,
		"description": description,
		"mainEntityOfPage": canonicalURL,
	}
	if imageURL != "" {
		payload["image"] = imageURL
	}
	if lastMod := normalizeDate(updatedAt); lastMod != "" {
		payload["dateModified"] = lastMod
	}
	return marshalJSONLD(payload)
}

func buildWebPageJSONLD(title, description, canonicalURL, imageURL string) string {
	payload := map[string]any{
		"@context":    "https://schema.org",
		"@type":       "WebPage",
		"name":        title,
		"description": description,
		"url":         canonicalURL,
	}
	if imageURL != "" {
		payload["image"] = imageURL
	}
	return marshalJSONLD(payload)
}

func marshalJSONLD(payload map[string]any) string {
	data, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(data)
}

func writeMetaTag(buf *strings.Builder, name, content string) {
	if strings.TrimSpace(content) == "" {
		return
	}
	buf.WriteString(`<meta name="`)
	buf.WriteString(html.EscapeString(name))
	buf.WriteString(`" content="`)
	buf.WriteString(html.EscapeString(content))
	buf.WriteString(`">`)
}

func writePropertyMetaTag(buf *strings.Builder, property, content string) {
	if strings.TrimSpace(content) == "" {
		return
	}
	buf.WriteString(`<meta property="`)
	buf.WriteString(html.EscapeString(property))
	buf.WriteString(`" content="`)
	buf.WriteString(html.EscapeString(content))
	buf.WriteString(`">`)
}

func writeCanonicalTag(buf *strings.Builder, href string) {
	if strings.TrimSpace(href) == "" {
		return
	}
	buf.WriteString(`<link rel="canonical" href="`)
	buf.WriteString(html.EscapeString(href))
	buf.WriteString(`">`)
}

func extractSiteNameFromTitle(title string) string {
	parts := strings.Split(title, " - ")
	if len(parts) == 0 {
		return defaultSiteName
	}
	return strings.TrimSpace(parts[len(parts)-1])
}

func normalizeDate(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed.Format("2006-01-02")
	}
	if parsed, err := time.Parse("2006-01-02", raw); err == nil {
		return parsed.Format("2006-01-02")
	}
	return ""
}
