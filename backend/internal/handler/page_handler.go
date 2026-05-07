package handler

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

var validSlugPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

const maxPageFileSize = 1 << 20 // 1MB

type PageHandler struct {
	pagesDir       string
	settingService *service.SettingService
REDACTED

func NewPageHandler(dataDir string, settingService *service.SettingService) *PageHandler {
	pagesDir := filepath.Join(dataDir, "pages")
	_ = os.MkdirAll(pagesDir, 0755)
	return &PageHandler{pagesDir: pagesDir, settingService: settingServiceREDACTED
REDACTED

// GetPageContent serves raw markdown content for a given slug.
// GET /api/v1/pages/:slug
func (h *PageHandler) GetPageContent(c *gin.Context) {
	slug := c.Param("slug")
	if !validSlugPattern.MatchString(slug) || len(slug) > 64 {
		response.BadRequest(c, "Invalid page slug")
		return
REDACTED

	// Visibility check: slug must be configured in custom_menu_items
	// and the user must have permission based on visibility setting
	if !h.checkSlugVisibility(c, slug) {
		c.JSON(http.StatusNotFound, gin.H{"error": "page not found"REDACTED)
		return
REDACTED

	filePath := filepath.Join(h.pagesDir, slug+".md")
	cleaned := filepath.Clean(filePath)
	if !strings.HasPrefix(cleaned, filepath.Clean(h.pagesDir)) {
		response.BadRequest(c, "Invalid page slug")
		return
REDACTED

	info, err := os.Stat(cleaned)
	if err != nil || info.IsDir() {
		c.JSON(http.StatusNotFound, gin.H{"error": "page not found"REDACTED)
		return
REDACTED
	if info.Size() > maxPageFileSize {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "page too large"REDACTED)
		return
REDACTED

	content, err := os.ReadFile(cleaned)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read page"REDACTED)
		return
REDACTED

	c.Data(http.StatusOK, "text/markdown; charset=utf-8", content)
REDACTED

// ListPages returns available page slugs.
// GET /api/v1/pages
func (h *PageHandler) ListPages(c *gin.Context) {
	entries, err := os.ReadDir(h.pagesDir)
	if err != nil {
		response.Success(c, []string{REDACTED)
		return
REDACTED

	slugs := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
	REDACTED
		name := e.Name()
		if strings.HasSuffix(name, ".md") {
			slugs = append(slugs, strings.TrimSuffix(name, ".md"))
	REDACTED
REDACTED
	response.Success(c, slugs)
REDACTED

// ServePageImage serves images from data/pages/{slugREDACTED/ directory.
// GET /api/v1/pages/:slug/images/*filename
// No JWT required (browser img tags can't carry tokens), but visibility is checked.
func (h *PageHandler) ServePageImage(c *gin.Context) {
	slug := c.Param("slug")
	filename := c.Param("filename")
	filename = strings.TrimPrefix(filename, "/")

	if !validSlugPattern.MatchString(slug) || len(slug) > 64 {
		c.Status(http.StatusNotFound)
		return
REDACTED

	if !h.checkImageSlugVisibility(c, slug) {
		c.Status(http.StatusNotFound)
		return
REDACTED

	if filename == "" || strings.Contains(filename, "..") || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		c.Status(http.StatusNotFound)
		return
REDACTED

	imagesDir := filepath.Join(h.pagesDir, slug)
	filePath := filepath.Join(imagesDir, filename)
	cleaned := filepath.Clean(filePath)
	if !strings.HasPrefix(cleaned, filepath.Clean(imagesDir)) {
		c.Status(http.StatusNotFound)
		return
REDACTED

	info, err := os.Stat(cleaned)
	if err != nil || info.IsDir() {
		c.Status(http.StatusNotFound)
		return
REDACTED

	c.File(cleaned)
REDACTED

// findSlugVisibility looks up the slug in custom_menu_items and returns (visibility, found).
func (h *PageHandler) findSlugVisibility(c *gin.Context, slug string) (string, bool) {
	if h.settingService == nil {
		return "", false
REDACTED

	raw := h.settingService.GetCustomMenuItemsRaw(c.Request.Context())
	if raw == "" || raw == "[]" {
		return "", false
REDACTED

	var items []struct {
		URL        string `json:"url"`
		PageSlug   string `json:"page_slug"`
		Visibility string `json:"visibility"`
REDACTED
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return "", false
REDACTED

	for _, item := range items {
		itemSlug := item.PageSlug
		if itemSlug == "" && strings.HasPrefix(item.URL, "md:") {
			itemSlug = strings.TrimPrefix(item.URL, "md:")
	REDACTED
		if itemSlug == slug {
			return item.Visibility, true
	REDACTED
REDACTED
	return "", false
REDACTED

// checkSlugVisibility verifies the slug is configured in custom_menu_items
// and the authenticated user has permission to view it.
func (h *PageHandler) checkSlugVisibility(c *gin.Context, slug string) bool {
	visibility, found := h.findSlugVisibility(c, slug)
	if !found {
		return false
REDACTED
	if visibility == "admin" {
		role, _ := middleware2.GetUserRoleFromContext(c)
		return role == "admin"
REDACTED
	return true
REDACTED

// checkImageSlugVisibility checks visibility for image requests (no JWT available).
// Only allows user-visible pages; admin-only pages are blocked.
func (h *PageHandler) checkImageSlugVisibility(c *gin.Context, slug string) bool {
	visibility, found := h.findSlugVisibility(c, slug)
	if !found {
		return false
REDACTED
	return visibility != "admin"
REDACTED

// RegisterPageRoutes registers page routes on a router group.
func RegisterPageRoutes(v1 *gin.RouterGroup, dataDir string, jwtAuth gin.HandlerFunc, adminAuth gin.HandlerFunc, settingService *service.SettingService) {
	h := NewPageHandler(dataDir, settingService)

	// Authenticated page content (JWT required + visibility check)
	pages := v1.Group("/pages")
	pages.Use(jwtAuth)
	{
		pages.GET("/:slug", h.GetPageContent)
REDACTED

	// Images: no JWT (browser img tags can't carry tokens), visibility check in handler
	pageImages := v1.Group("/pages")
	{
		pageImages.GET("/:slug/images/*filename", h.ServePageImage)
REDACTED

	// Admin-only: list all available pages
	adminPages := v1.Group("/pages")
	adminPages.Use(adminAuth)
	{
		adminPages.GET("", h.ListPages)
REDACTED
REDACTED
