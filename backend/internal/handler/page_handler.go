package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tutorialhtml"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	rendererhtml "github.com/yuin/goldmark/renderer/html"
)

var validSlugPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

const maxPageFileSize = 1 << 20 // 1MB
const maxPageImageUploadBytes int64 = 5 * 1024 * 1024
const maxPageJSONRequestBytes int64 = maxPageFileSize + 64*1024

var tutorialMarkdownRenderer = goldmark.New(
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

type PageHandler struct {
	pagesDir       string
	settingService *service.SettingService
}

const builtInTutorialSlug = "tutorial"

type updatePageContentRequest struct {
	ContentMD string `json:"content_md"`
}

type pageImageInfo struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Size int64  `json:"size"`
}

type tutorialDocumentResponse struct {
	ContentHTML string `json:"content_html"`
}

type updateTutorialDocumentRequest struct {
	ContentHTML string `json:"content_html"`
}

func NewPageHandler(dataDir string, settingService *service.SettingService) *PageHandler {
	pagesDir := filepath.Join(dataDir, "pages")
	_ = os.MkdirAll(pagesDir, 0755)
	return &PageHandler{pagesDir: pagesDir, settingService: settingService}
}

// GetPageContent serves raw markdown content for a given slug.
// GET /api/v1/pages/:slug
func (h *PageHandler) GetPageContent(c *gin.Context) {
	slug := c.Param("slug")
	if !validSlugPattern.MatchString(slug) || len(slug) > 64 {
		response.BadRequest(c, "Invalid page slug")
		return
	}

	// Visibility check: slug must be configured in custom_menu_items
	// and the user must have permission based on visibility setting
	if !h.checkSlugVisibility(c, slug) {
		c.JSON(http.StatusNotFound, gin.H{"error": "page not found"})
		return
	}

	content, err := h.readPageContent(slug)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			c.JSON(http.StatusNotFound, gin.H{"error": "page not found"})
			return
		}
		if errors.Is(err, errPageTooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "page too large"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read page"})
		return
	}

	if len(content) > maxPageFileSize {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "page too large"})
		return
	}

	c.Data(http.StatusOK, "text/markdown; charset=utf-8", content)
}

// GetPublicPageContent serves raw markdown content for a public page slug.
// GET /api/v1/public/pages/:slug
func (h *PageHandler) GetPublicPageContent(c *gin.Context) {
	slug := c.Param("slug")
	if !validSlugPattern.MatchString(slug) || len(slug) > 64 {
		c.JSON(http.StatusNotFound, gin.H{"error": "page not found"})
		return
	}

	if !h.isUserVisibleSlug(c, slug) {
		c.JSON(http.StatusNotFound, gin.H{"error": "page not found"})
		return
	}

	content, err := h.readPageContent(slug)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "page not found"})
		return
	}

	c.Data(http.StatusOK, "text/markdown; charset=utf-8", content)
}

// UpdatePageContent saves markdown content for a given slug.
// PUT /api/v1/pages/:slug
func (h *PageHandler) UpdatePageContent(c *gin.Context) {
	slug := c.Param("slug")
	if !validSlugPattern.MatchString(slug) || len(slug) > 64 {
		response.BadRequest(c, "Invalid page slug")
		return
	}

	var req updatePageContentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}

	content := strings.ReplaceAll(req.ContentMD, "\r\n", "\n")
	if len([]byte(content)) > maxPageFileSize {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "page too large"})
		return
	}

	filePath, ok := h.pageFilePath(slug)
	if !ok {
		response.BadRequest(c, "Invalid page slug")
		return
	}
	if err := os.MkdirAll(h.pagesDir, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to prepare pages directory"})
		return
	}
	if err := writeFileAtomically(filePath, []byte(content), 0o644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save page"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"slug":       slug,
		"content_md": content,
	})
}

// GetTutorialDocument returns the rich HTML content for tutorial editor.
// GET /api/v1/tutorial-document
func (h *PageHandler) GetTutorialDocument(c *gin.Context) {
	content, err := h.loadTutorialDocumentHTML()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read tutorial document"})
		return
	}
	response.Success(c, tutorialDocumentResponse{ContentHTML: content})
}

// UpdateTutorialDocument saves the rich HTML content for tutorial editor.
// PUT /api/v1/tutorial-document
func (h *PageHandler) UpdateTutorialDocument(c *gin.Context) {
	var req updateTutorialDocumentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	if len([]byte(req.ContentHTML)) > maxPageFileSize {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "document too large"})
		return
	}
	sanitized := tutorialhtml.SanitizeTutorialHTML(req.ContentHTML)
	if len([]byte(sanitized)) > maxPageFileSize {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "document too large"})
		return
	}
	filePath, ok := h.tutorialHTMLFilePath()
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid tutorial document path"})
		return
	}
	if err := os.MkdirAll(h.pagesDir, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to prepare tutorial directory"})
		return
	}
	if strings.TrimSpace(sanitized) == "" {
		if err := os.Remove(filePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reset tutorial document"})
			return
		}
		content, err := h.loadTutorialDocumentHTML()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read tutorial document"})
			return
		}
		response.Success(c, tutorialDocumentResponse{ContentHTML: content})
		return
	}
	if err := writeFileAtomically(filePath, []byte(sanitized), 0o644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save tutorial document"})
		return
	}
	response.Success(c, tutorialDocumentResponse{ContentHTML: sanitized})
}

// UploadPageImage saves an image file under data/pages/{slug}/
// POST /api/v1/pages/:slug/images
func (h *PageHandler) UploadPageImage(c *gin.Context) {
	slug := c.Param("slug")
	if !validSlugPattern.MatchString(slug) || len(slug) > 64 {
		response.BadRequest(c, "Invalid page slug")
		return
	}
	if slug != builtInTutorialSlug {
		response.BadRequest(c, "Image upload is only supported for tutorial page")
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		response.BadRequest(c, "File is required")
		return
	}
	if fileHeader.Size <= 0 {
		response.BadRequest(c, "File is empty")
		return
	}
	if fileHeader.Size > maxPageImageUploadBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "image too large"})
		return
	}

	src, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to open upload"})
		return
	}
	defer func() { _ = src.Close() }()

	data, err := io.ReadAll(io.LimitReader(src, maxPageImageUploadBytes+1))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read upload"})
		return
	}
	if int64(len(data)) > maxPageImageUploadBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "image too large"})
		return
	}

	ext, ok := detectPageImageExtension(data, fileHeader.Header.Get("Content-Type"), fileHeader.Filename)
	if !ok {
		response.BadRequest(c, "Unsupported image type")
		return
	}

	baseName := sanitizePageAssetBaseName(strings.TrimSuffix(fileHeader.Filename, filepath.Ext(fileHeader.Filename)))
	if baseName == "" {
		baseName = "image"
	}
	targetDir := filepath.Join(h.pagesDir, slug)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to prepare image directory"})
		return
	}

	fileName := ensureUniquePageImageName(targetDir, baseName, ext)
	targetPath := filepath.Join(targetDir, fileName)
	if err := os.WriteFile(targetPath, data, 0o644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save image"})
		return
	}

	response.Success(c, pageImageInfo{
		Name: fileName,
		URL:  buildPageImagePublicURL(slug, fileName),
		Size: int64(len(data)),
	})
}

// ListPageImages returns uploaded images for a page.
// GET /api/v1/pages/:slug/images
func (h *PageHandler) ListPageImages(c *gin.Context) {
	slug := c.Param("slug")
	if !validSlugPattern.MatchString(slug) || len(slug) > 64 {
		response.BadRequest(c, "Invalid page slug")
		return
	}
	if slug != builtInTutorialSlug {
		response.Success(c, []pageImageInfo{})
		return
	}

	targetDir := filepath.Join(h.pagesDir, slug)
	entries, err := os.ReadDir(targetDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			response.Success(c, []pageImageInfo{})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read image directory"})
		return
	}

	items := make([]pageImageInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		switch ext {
		case ".png", ".jpg", ".jpeg", ".webp", ".gif":
		default:
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		items = append(items, pageImageInfo{
			Name: name,
			URL:  buildPageImagePublicURL(slug, name),
			Size: info.Size(),
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	response.Success(c, items)
}

// ListPages returns available page slugs.
// GET /api/v1/pages
func (h *PageHandler) ListPages(c *gin.Context) {
	entries, err := os.ReadDir(h.pagesDir)
	if err != nil {
		response.Success(c, []string{})
		return
	}

	slugs := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".md") {
			slugs = append(slugs, strings.TrimSuffix(name, ".md"))
		}
	}
	response.Success(c, slugs)
}

// ServePageImage serves images from data/pages/{slug}/ directory.
// GET /api/v1/pages/:slug/images/*filename
// No JWT required (browser img tags can't carry tokens), but visibility is checked.
func (h *PageHandler) ServePageImage(c *gin.Context) {
	slug := c.Param("slug")
	filename := c.Param("filename")
	filename = strings.TrimPrefix(filename, "/")

	if !validSlugPattern.MatchString(slug) || len(slug) > 64 {
		c.Status(http.StatusNotFound)
		return
	}

	if !h.checkImageSlugVisibility(c, slug) {
		c.Status(http.StatusNotFound)
		return
	}

	imagesDir := filepath.Join(h.pagesDir, slug)
	cleaned, ok := resolvePageImagePath(h.pagesDir, imagesDir, filename)
	if !ok {
		c.Status(http.StatusNotFound)
		return
	}

	info, err := os.Stat(cleaned)
	if err != nil || info.IsDir() {
		c.Status(http.StatusNotFound)
		return
	}

	c.File(cleaned)
}

func resolvePageImagePath(pagesDir, imagesDir, filename string) (string, bool) {
	relPath, ok := cleanPageImageRelativePath(filename)
	if !ok {
		return "", false
	}

	cleanedPagesDir := filepath.Clean(pagesDir)
	cleanedImagesDir := filepath.Clean(imagesDir)
	cleanedTarget := filepath.Clean(filepath.Join(cleanedImagesDir, relPath))
	if !isPathWithinBase(cleanedTarget, cleanedImagesDir) {
		return "", false
	}

	realPagesDir, err := filepath.EvalSymlinks(cleanedPagesDir)
	if err != nil {
		return "", false
	}
	realImagesDir, err := filepath.EvalSymlinks(cleanedImagesDir)
	if err != nil || !isPathWithinBase(realImagesDir, realPagesDir) {
		return "", false
	}
	realTarget, err := filepath.EvalSymlinks(cleanedTarget)
	if err != nil || !isPathWithinBase(realTarget, realImagesDir) {
		return "", false
	}
	return realTarget, true
}

func cleanPageImageRelativePath(filename string) (string, bool) {
	if filename == "" {
		return "", false
	}
	if strings.HasPrefix(filename, "/") {
		return "", false
	}
	decoded, err := url.PathUnescape(filename)
	if err != nil {
		return "", false
	}
	if decoded == "" || strings.HasPrefix(decoded, "/") || strings.Contains(decoded, "\\") || strings.ContainsRune(decoded, 0) {
		return "", false
	}

	parts := make([]string, 0)
	for _, part := range strings.Split(decoded, "/") {
		switch part {
		case "", ".":
			continue
		case "..":
			return "", false
		default:
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		return "", false
	}

	relPath := filepath.Join(parts...)
	if filepath.IsAbs(relPath) || filepath.VolumeName(relPath) != "" {
		return "", false
	}
	return relPath, true
}

func isPathWithinBase(path, base string) bool {
	rel, err := filepath.Rel(filepath.Clean(base), filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// findSlugVisibility looks up the slug in custom_menu_items and returns (visibility, found).
func (h *PageHandler) findSlugVisibility(c *gin.Context, slug string) (string, bool) {
	if slug == builtInTutorialSlug {
		return "user", true
	}
	if h.settingService == nil {
		return "", false
	}

	raw := h.settingService.GetCustomMenuItemsRaw(c.Request.Context())
	if raw == "" || raw == "[]" {
		return "", false
	}

	var items []struct {
		URL        string `json:"url"`
		PageSlug   string `json:"page_slug"`
		Visibility string `json:"visibility"`
	}
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return "", false
	}

	for _, item := range items {
		itemSlug := item.PageSlug
		if itemSlug == "" && strings.HasPrefix(item.URL, "md:") {
			itemSlug = strings.TrimPrefix(item.URL, "md:")
		}
		if itemSlug == slug {
			return item.Visibility, true
		}
	}
	return "", false
}

// checkSlugVisibility verifies the slug is configured in custom_menu_items
// and the authenticated user has permission to view it.
func (h *PageHandler) checkSlugVisibility(c *gin.Context, slug string) bool {
	visibility, found := h.findSlugVisibility(c, slug)
	if !found {
		return false
	}
	if visibility == "admin" {
		role, _ := middleware2.GetUserRoleFromContext(c)
		return role == "admin"
	}
	return true
}

// checkImageSlugVisibility checks visibility for image requests (no JWT available).
// Only allows user-visible pages; admin-only pages are blocked.
func (h *PageHandler) checkImageSlugVisibility(c *gin.Context, slug string) bool {
	visibility, found := h.findSlugVisibility(c, slug)
	if !found {
		return false
	}
	return visibility != "admin"
}

func (h *PageHandler) isUserVisibleSlug(c *gin.Context, slug string) bool {
	visibility, found := h.findSlugVisibility(c, slug)
	if !found {
		return false
	}
	return visibility != "admin"
}

func detectPageImageExtension(data []byte, contentType, fileName string) (string, bool) {
	declared := strings.ToLower(strings.TrimSpace(contentType))
	if idx := strings.Index(declared, ";"); idx >= 0 {
		declared = strings.TrimSpace(declared[:idx])
	}
	if strings.EqualFold(strings.TrimSpace(fileName), "") {
		return "", false
	}
	if strings.EqualFold(filepath.Ext(fileName), ".svg") {
		return "", false
	}
	detected := http.DetectContentType(data)
	switch detected {
	case "image/png":
		return ".png", declared == "" || declared == "image/png"
	case "image/jpeg":
		return ".jpg", declared == "" || declared == "image/jpeg"
	case "image/gif":
		return ".gif", declared == "" || declared == "image/gif"
	case "application/octet-stream":
		ext := strings.ToLower(filepath.Ext(fileName))
		switch ext {
		case ".png", ".jpg", ".jpeg", ".gif", ".webp":
			return ext, true
		}
	}
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return ".webp", declared == "" || declared == "image/webp"
	}
	return "", false
}

func sanitizePageAssetBaseName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	name = strings.ReplaceAll(name, " ", "-")
	var b strings.Builder
	for _, ch := range name {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' {
			b.WriteRune(ch)
		}
	}
	return strings.Trim(b.String(), "-_")
}

func ensureUniquePageImageName(dir, baseName, ext string) string {
	candidate := baseName + ext
	if _, err := os.Stat(filepath.Join(dir, candidate)); errors.Is(err, os.ErrNotExist) {
		return candidate
	}
	for i := 2; ; i++ {
		candidate = baseName + "-" + strconv.Itoa(i) + ext
		if _, err := os.Stat(filepath.Join(dir, candidate)); errors.Is(err, os.ErrNotExist) {
			return candidate
		}
	}
}

func buildPageImagePublicURL(slug, fileName string) string {
	parts := strings.Split(fileName, "/")
	encoded := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." {
			continue
		}
		encoded = append(encoded, url.PathEscape(part))
	}
	return "/api/v1/pages/" + url.PathEscape(strings.TrimSpace(slug)) + "/images/" + strings.Join(encoded, "/")
}

var errPageTooLarge = errors.New("page too large")

func (h *PageHandler) pageFilePath(slug string) (string, bool) {
	filePath := filepath.Join(h.pagesDir, slug+".md")
	cleaned := filepath.Clean(filePath)
	if !strings.HasPrefix(cleaned, filepath.Clean(h.pagesDir)) {
		return "", false
	}
	return cleaned, true
}

func (h *PageHandler) tutorialHTMLFilePath() (string, bool) {
	filePath := filepath.Join(h.pagesDir, builtInTutorialSlug+".html")
	cleaned := filepath.Clean(filePath)
	if !strings.HasPrefix(cleaned, filepath.Clean(h.pagesDir)) {
		return "", false
	}
	return cleaned, true
}

func (h *PageHandler) loadTutorialDocumentHTML() (string, error) {
	htmlPath, ok := h.tutorialHTMLFilePath()
	if !ok {
		return "", os.ErrInvalid
	}
	if raw, err := os.ReadFile(htmlPath); err == nil {
		sanitized := tutorialhtml.SanitizeTutorialHTML(string(raw))
		if strings.TrimSpace(sanitized) != "" {
			return sanitized, nil
		}
	}
	content, err := h.readPageContent(builtInTutorialSlug)
	if err != nil {
		return "", err
	}
	return renderTutorialMarkdownToHTML(string(content))
}

func writeFileAtomically(filePath string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(filePath)
	tmpFile, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
	}()
	if err := tmpFile.Chmod(perm); err != nil {
		return err
	}
	if _, err := tmpFile.Write(data); err != nil {
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, filePath); err != nil {
		if removeErr := os.Remove(filePath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return err
		}
		if retryErr := os.Rename(tmpPath, filePath); retryErr != nil {
			return retryErr
		}
	}
	return nil
}

func renderTutorialMarkdownToHTML(markdown string) (string, error) {
	var buf strings.Builder
	if err := tutorialMarkdownRenderer.Convert([]byte(markdown), &buf); err != nil {
		return "", err
	}
	return tutorialhtml.SanitizeTutorialHTML(buf.String()), nil
}

func (h *PageHandler) readPageContent(slug string) ([]byte, error) {
	filePath, ok := h.pageFilePath(slug)
	if !ok {
		return nil, os.ErrNotExist
	}
	info, err := os.Stat(filePath)
	if err == nil {
		if info.IsDir() {
			return nil, os.ErrNotExist
		}
		if info.Size() > maxPageFileSize {
			return nil, errPageTooLarge
		}
		return os.ReadFile(filePath)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if slug == builtInTutorialSlug {
		return []byte(defaultTutorialMarkdown), nil
	}
	return nil, os.ErrNotExist
}

// RegisterPageRoutes registers page routes on a router group.
func RegisterPageRoutes(v1 *gin.RouterGroup, dataDir string, jwtAuth gin.HandlerFunc, adminAuth gin.HandlerFunc, settingService *service.SettingService) {
	h := NewPageHandler(dataDir, settingService)

	// Authenticated page content (JWT required + visibility check)
	pages := v1.Group("/pages")
	pages.Use(jwtAuth)
	{
		pages.GET("/:slug", h.GetPageContent)
	}

	// Admin-managed page image endpoints must be registered before the public
	// wildcard image route so `/pages/:slug/images` is not shadowed by
	// `/pages/:slug/images/*filename`.
	adminPages := v1.Group("/pages")
	adminPages.Use(adminAuth)
	{
		adminPages.GET("", h.ListPages)
		adminPages.GET("/:slug/images", h.ListPageImages)
		adminPages.POST("/:slug/images", middleware2.RequestBodyLimit(maxPageImageUploadBytes+(1<<20)), h.UploadPageImage)
		adminPages.PUT("/:slug", middleware2.RequestBodyLimit(maxPageJSONRequestBytes), h.UpdatePageContent)
	}

	// Images: no JWT (browser img tags can't carry tokens), visibility check in handler
	pageImages := v1.Group("/pages")
	{
		pageImages.GET("/:slug/images/*filename", h.ServePageImage)
	}

	publicPages := v1.Group("/public/pages")
	{
		publicPages.GET("/:slug", h.GetPublicPageContent)
	}

	v1.GET("/tutorial-document", h.GetTutorialDocument)

	adminTutorial := v1.Group("/tutorial-document")
	adminTutorial.Use(adminAuth)
	{
		adminTutorial.PUT("", middleware2.RequestBodyLimit(maxPageJSONRequestBytes), h.UpdateTutorialDocument)
	}
}
