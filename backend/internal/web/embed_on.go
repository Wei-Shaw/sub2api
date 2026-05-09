//go:build embed

package web

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/setup"
	"github.com/gin-gonic/gin"
)

const (
	// NonceHTMLPlaceholder is the placeholder for nonce in HTML script tags
	NonceHTMLPlaceholder = "__CSP_NONCE_VALUE__"
	frontendStatusCodeKey = "frontend_status_code"
	frontendNotFoundKey   = "frontend_not_found"
)

//go:embed all:dist
var frontendFS embed.FS

// PublicSettingsProvider is an interface to fetch public settings
type PublicSettingsProvider interface {
	GetPublicSettingsForInjection(ctx context.Context) (any, error)
}

// FrontendServer serves the embedded frontend with settings injection
type FrontendServer struct {
	distFS      fs.FS
	fileServer  http.Handler
	baseHTML    []byte
	cache       *HTMLCache
	settings    PublicSettingsProvider
	overrideDir string // local file override directory
	pagesDir    string
}

// NewFrontendServer creates a new frontend server with settings injection
func NewFrontendServer(settingsProvider PublicSettingsProvider) (*FrontendServer, error) {
	distFS, err := fs.Sub(frontendFS, "dist")
	if err != nil {
		return nil, err
	}

	// Read base HTML once
	file, err := distFS.Open("index.html")
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	baseHTML, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	cache := NewHTMLCache()
	cache.SetBaseHTML(baseHTML)

	return &FrontendServer{
		distFS:      distFS,
		fileServer:  http.FileServer(http.FS(distFS)),
		baseHTML:    baseHTML,
		cache:       cache,
		settings:    settingsProvider,
		overrideDir: filepath.Join(setup.GetDataDir(), "public"),
		pagesDir:    filepath.Join(setup.GetDataDir(), "pages"),
	}, nil
}

// InvalidateCache invalidates the HTML cache (call when settings change)
func (s *FrontendServer) InvalidateCache() {
	if s != nil && s.cache != nil {
		s.cache.Invalidate()
	}
}

// Middleware returns the Gin middleware handler
func (s *FrontendServer) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := normalizeRequestPath(c.Request.URL.Path)

		// Skip API routes
		if shouldBypassEmbeddedFrontend(path) {
			c.Next()
			return
		}

		if path == "/robots.txt" {
			s.serveRobotsTXT(c)
			return
		}
		if path == "/sitemap.xml" {
			s.serveSitemapXML(c)
			return
		}
		if strings.HasPrefix(path, "/og/") {
			s.serveOGImage(c)
			return
		}
		if s.tryServePublicRenderedPage(c, path) {
			return
		}

		cleanPath := strings.TrimPrefix(path, "/")
		if cleanPath == "" {
			cleanPath = "index.html"
		}

		// Prefer prerendered directory pages for markdown-heavy public routes only.
		// Home and general SPA routes must continue through settings injection so
		// admin SEO changes are reflected immediately in the first HTML response.
		if !strings.Contains(filepath.Base(cleanPath), ".") &&
			path != "/home" &&
			(strings.HasPrefix(path, "/legal/") || strings.HasPrefix(path, "/custom/") || path == "/docs/tutorial") {
			prerenderedPath := filepath.ToSlash(filepath.Join(cleanPath, "index.html"))
			if s.fileExists(prerenderedPath) {
				if s.tryServeOverride(c, prerenderedPath) {
					return
				}
				c.Request.URL.Path = "/" + prerenderedPath
				s.fileServer.ServeHTTP(c.Writer, c.Request)
				c.Abort()
				return
			}
		}

		if strings.HasPrefix(path, "/legal/") || strings.HasPrefix(path, "/custom/") || path == "/docs/tutorial" {
			if !s.publicPageExists(c, path) {
				s.serveNotFoundHTML(c)
				return
			}
		}

		if strings.Contains(filepath.Base(cleanPath), ".") && !s.fileExists(cleanPath) {
			c.String(http.StatusNotFound, "Frontend not found")
			c.Abort()
			return
		}

		// For index.html or SPA routes, serve with injected settings
		if cleanPath == "index.html" || !s.fileExists(cleanPath) {
			if !isKnownSPARoute(path) {
				s.serveNotFoundHTML(c)
				return
			}
			s.serveIndexHTML(c)
			return
		}

		// Try local override first
		if s.tryServeOverride(c, cleanPath) {
			return
		}

		// Serve static files normally
		s.fileServer.ServeHTTP(c.Writer, c.Request)
		c.Abort()
	}
}

func (s *FrontendServer) publicPageExists(c *gin.Context, requestPath string) bool {
	settingsJSON, ok := s.loadSettingsJSON(c)
	if !ok {
		return false
	}
	cfg := parseSEOConfig(settingsJSON)

	switch {
	case strings.HasPrefix(requestPath, "/legal/"):
		_, found := findLegalDocument(cfg, strings.TrimPrefix(requestPath, "/legal/"))
		return found
	case strings.HasPrefix(requestPath, "/custom/"):
		page, found := findPublicCustomPage(cfg, strings.TrimPrefix(requestPath, "/custom/"))
		if !found {
			return false
		}
		slug := strings.TrimSpace(page.PageSlug)
		if slug == "" && strings.HasPrefix(strings.TrimSpace(page.URL), "md:") {
			slug = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(page.URL), "md:"))
		}
		if slug == "" {
			return true
		}
		if _, err := s.loadMarkdownFile(slug); err != nil {
			return false
		}
		return true
	case requestPath == "/docs/tutorial":
		if _, err := s.loadMarkdownFile("tutorial"); err != nil {
			return false
		}
		return true
	default:
		return false
	}
}

func (s *FrontendServer) serveNotFoundHTML(c *gin.Context) {
	c.Set(frontendStatusCodeKey, http.StatusNotFound)
	c.Set(frontendNotFoundKey, true)
	c.Header("Cache-Control", "no-cache")
	s.serveIndexHTML(c)
}

func (s *FrontendServer) fileExists(path string) bool {
	file, err := s.distFS.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// tryServeOverride checks if a local override file exists and serves it.
// Files in overrideDir take precedence over embedded files.
func (s *FrontendServer) tryServeOverride(c *gin.Context, cleanPath string) bool {
	if s.overrideDir == "" {
		return false
	}
	filePath := filepath.Join(s.overrideDir, filepath.Clean("/"+cleanPath))
	info, err := os.Stat(filePath)
	if err != nil || info.IsDir() {
		return false
	}
	c.File(filePath)
	c.Abort()
	return true
}

func (s *FrontendServer) serveIndexHTML(c *gin.Context) {
	// Get nonce from context (generated by SecurityHeaders middleware)
	nonce := middleware.GetNonceFromContext(c)
	requestPath := normalizeRequestPath(c.Request.URL.Path)
	cacheKey := requestPath
	statusCode := http.StatusOK
	if rawStatus, ok := c.Get(frontendStatusCodeKey); ok {
		if code, valid := rawStatus.(int); valid && code >= http.StatusBadRequest {
			statusCode = code
		}
	}
	isNotFound, _ := c.Get(frontendNotFoundKey)
	notFound := isNotFound == true

	// Check cache first
	cached := s.cache.Get(cacheKey)
	if cached != nil {
		// Check If-None-Match for 304 response
		if match := c.GetHeader("If-None-Match"); match == cached.ETag {
			c.Status(http.StatusNotModified)
			c.Abort()
			return
		}

		// Replace nonce placeholder with actual nonce before serving
		content := replaceNoncePlaceholder(cached.Content, nonce)

		c.Header("ETag", cached.ETag)
		c.Header("Cache-Control", "no-cache") // Must revalidate
		seo := buildSEOData(requestPath, extractSettingsJSON(cached.Content))
		if notFound {
			seo = buildNotFoundSEOData(requestPath, extractSettingsJSON(cached.Content))
		}
		c.Header("X-Robots-Tag", seo.XRobotsTag)
		c.Data(statusCode, "text/html; charset=utf-8", content)
		c.Abort()
		return
	}

	// Cache miss - fetch settings and render
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	settings, err := s.settings.GetPublicSettingsForInjection(ctx)
	if err != nil {
		// Fallback: serve without injection
		c.Data(statusCode, "text/html; charset=utf-8", s.baseHTML)
		c.Abort()
		return
	}

	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		// Fallback: serve without injection
		c.Data(statusCode, "text/html; charset=utf-8", s.baseHTML)
		c.Abort()
		return
	}

	seo := buildSEOData(requestPath, settingsJSON)
	if notFound {
		seo = buildNotFoundSEOData(requestPath, settingsJSON)
	}
	rendered := s.injectSettingsWithSEO(settingsJSON, seo)
	s.cache.Set(cacheKey, rendered, settingsJSON)

	// Replace nonce placeholder with actual nonce before serving
	content := replaceNoncePlaceholder(rendered, nonce)

	cached = s.cache.Get(cacheKey)
	if cached != nil {
		c.Header("ETag", cached.ETag)
	}
	c.Header("Cache-Control", "no-cache")
	if seo.XRobotsTag != "" {
		c.Header("X-Robots-Tag", seo.XRobotsTag)
	}
	c.Data(statusCode, "text/html; charset=utf-8", content)
	c.Abort()
}

func isKnownSPARoute(path string) bool {
	switch {
	case path == "/" || path == "/home":
		return true
	case hasKnownRoutePrefix(path,
		"/dashboard",
		"/users",
		"/settings",
		"/setup",
		"/login",
		"/register",
		"/email-verify",
		"/forgot-password",
		"/reset-password",
		"/key-usage",
		"/auth",
		"/legal",
		"/docs",
		"/custom",
		"/admin",
		"/keys",
		"/usage",
		"/redeem",
		"/affiliate",
		"/available-channels",
		"/profile",
		"/subscriptions",
		"/purchase",
		"/orders",
		"/payment",
		"/monitor",
	):
		return true
	default:
		return false
	}
}

func hasKnownRoutePrefix(path string, prefixes ...string) bool {
	for _, prefix := range prefixes {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

func (s *FrontendServer) injectSettingsWithSEO(settingsJSON []byte, seo seoData) []byte {
	script := []byte(`<script nonce="` + NonceHTMLPlaceholder + `">window.__APP_CONFIG__=` + string(settingsJSON) + `;</script>`)
	headClose := []byte("</head>")
	result := bytes.Replace(s.baseHTML, headClose, append(script, headClose...), 1)
	result = injectSEOTitle(result, seo.Title)
	result = injectSEOTags(result, buildSEOMetaTags(seo))
	return result
}

func (s *FrontendServer) injectSettings(requestPath string, settingsJSON []byte) ([]byte, seoData) {
	seo := buildSEOData(requestPath, settingsJSON)
	return s.injectSettingsWithSEO(settingsJSON, seo), seo
}

// replaceNoncePlaceholder replaces the nonce placeholder with actual nonce value
func replaceNoncePlaceholder(html []byte, nonce string) []byte {
	return bytes.ReplaceAll(html, []byte(NonceHTMLPlaceholder), []byte(nonce))
}

func (s *FrontendServer) serveRobotsTXT(c *gin.Context) {
	settingsJSON, ok := s.loadSettingsJSON(c)
	if !ok {
		c.Data(http.StatusOK, "text/plain; charset=utf-8", buildRobotsTXT(nil))
		c.Abort()
		return
	}
	c.Data(http.StatusOK, "text/plain; charset=utf-8", buildRobotsTXT(settingsJSON))
	c.Abort()
}

func (s *FrontendServer) serveSitemapXML(c *gin.Context) {
	settingsJSON, ok := s.loadSettingsJSON(c)
	if !ok {
		c.Status(http.StatusNotFound)
		c.Abort()
		return
	}
	content := buildSitemapXML(settingsJSON)
	if len(content) == 0 {
		c.Status(http.StatusNotFound)
		c.Abort()
		return
	}
	c.Data(http.StatusOK, "application/xml; charset=utf-8", content)
	c.Abort()
}

func (s *FrontendServer) loadSettingsJSON(c *gin.Context) ([]byte, bool) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	settings, err := s.settings.GetPublicSettingsForInjection(ctx)
	if err != nil {
		return nil, false
	}
	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return nil, false
	}
	return settingsJSON, true
}

func normalizeRequestPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "/"
	}
	if !strings.HasPrefix(raw, "/") {
		return "/" + raw
	}
	return raw
}

func extractSettingsJSON(htmlDoc []byte) []byte {
	startMarker := []byte("window.__APP_CONFIG__=")
	start := bytes.Index(htmlDoc, startMarker)
	if start == -1 {
		return nil
	}
	start += len(startMarker)
	end := bytes.Index(htmlDoc[start:], []byte(";</script>"))
	if end == -1 {
		return nil
	}
	return htmlDoc[start : start+end]
}

// ServeEmbeddedFrontend returns a middleware for serving embedded frontend
// This is the legacy function for backward compatibility when no settings provider is available
func ServeEmbeddedFrontend() gin.HandlerFunc {
	distFS, err := fs.Sub(frontendFS, "dist")
	if err != nil {
		panic("failed to get dist subdirectory: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(distFS))
	overrideDir := filepath.Join(setup.GetDataDir(), "public")

	return func(c *gin.Context) {
		path := c.Request.URL.Path

		if shouldBypassEmbeddedFrontend(path) {
			c.Next()
			return
		}

		cleanPath := strings.TrimPrefix(path, "/")
		if cleanPath == "" {
			cleanPath = "index.html"
		}

		if file, err := distFS.Open(cleanPath); err == nil {
			_ = file.Close()
			// Try local override first
			if tryServeOverrideFile(c, overrideDir, cleanPath) {
				return
			}
			fileServer.ServeHTTP(c.Writer, c.Request)
			c.Abort()
			return
		}

		serveIndexHTML(c, distFS)
	}
}

// tryServeOverrideFile is a standalone version of tryServeOverride for legacy usage.
func tryServeOverrideFile(c *gin.Context, overrideDir, cleanPath string) bool {
	if overrideDir == "" {
		return false
	}
	filePath := filepath.Join(overrideDir, filepath.Clean("/"+cleanPath))
	info, err := os.Stat(filePath)
	if err != nil || info.IsDir() {
		return false
	}
	c.File(filePath)
	c.Abort()
	return true
}

func shouldBypassEmbeddedFrontend(path string) bool {
	trimmed := strings.TrimSpace(path)
	return strings.HasPrefix(trimmed, "/api/") ||
		strings.HasPrefix(trimmed, "/v1/") ||
		strings.HasPrefix(trimmed, "/v1beta/") ||
		strings.HasPrefix(trimmed, "/backend-api/") ||
		strings.HasPrefix(trimmed, "/antigravity/") ||
		strings.HasPrefix(trimmed, "/r/") ||
		strings.HasPrefix(trimmed, "/referral-assets/") ||
		strings.HasPrefix(trimmed, "/setup/") ||
		trimmed == "/health" ||
		trimmed == "/responses" ||
		strings.HasPrefix(trimmed, "/responses/") ||
		strings.HasPrefix(trimmed, "/images/")
}

func serveIndexHTML(c *gin.Context, fsys fs.FS) {
	file, err := fsys.Open("index.html")
	if err != nil {
		c.String(http.StatusNotFound, "Frontend not found")
		c.Abort()
		return
	}
	defer func() { _ = file.Close() }()

	content, err := io.ReadAll(file)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to read index.html")
		c.Abort()
		return
	}

	c.Data(http.StatusOK, "text/html; charset=utf-8", content)
	c.Abort()
}

func HasEmbeddedFrontend() bool {
	_, err := frontendFS.ReadFile("dist/index.html")
	return err == nil
}
