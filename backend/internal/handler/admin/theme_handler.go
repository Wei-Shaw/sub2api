package admin

import (
	"io"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ThemeHandler handles admin theme management endpoints
type ThemeHandler struct {
	themeService *service.ThemeService
}

// NewThemeHandler creates a new ThemeHandler
func NewThemeHandler(themeService *service.ThemeService) *ThemeHandler {
	return &ThemeHandler{themeService: themeService}
}

// List returns all installed themes
// GET /api/v1/admin/themes
func (h *ThemeHandler) List(c *gin.Context) {
	themes, err := h.themeService.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"themes": themes})
}

// Get returns a specific theme
// GET /api/v1/admin/themes/:short
func (h *ThemeHandler) Get(c *gin.Context) {
	short := c.Param("short")
	theme, err := h.themeService.Get(c.Request.Context(), short)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, theme)
}

// GetActive returns the currently active theme
// GET /api/v1/admin/themes/active
func (h *ThemeHandler) GetActive(c *gin.Context) {
	theme, err := h.themeService.GetActiveTheme(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if theme == nil {
		c.JSON(http.StatusOK, gin.H{"active": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{"active": true, "theme": theme})
}

// Install handles theme installation from a zip upload
// POST /api/v1/admin/themes/install
func (h *ThemeHandler) Install(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, service.ThemeMaxZipSize)

	file, _, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing or invalid file field"})
		return
	}
	defer func() { _ = file.Close() }()

	zipData, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read file: " + err.Error()})
		return
	}

	theme, err := h.themeService.InstallFromZip(c.Request.Context(), zipData)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, theme)
}

// InstallFromGitHub handles theme installation from a GitHub URL
// POST /api/v1/admin/themes/install-github
func (h *ThemeHandler) InstallFromGitHub(c *gin.Context) {
	var req struct {
		URL string `json:"url" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing url field"})
		return
	}

	theme, err := h.themeService.InstallFromGitHub(c.Request.Context(), req.URL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, theme)
}

// Activate activates a theme
// POST /api/v1/admin/themes/:short/activate
func (h *ThemeHandler) Activate(c *gin.Context) {
	short := c.Param("short")
	if err := h.themeService.Activate(c.Request.Context(), short); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "theme activated"})
}

// Deactivate deactivates the current theme
// POST /api/v1/admin/themes/deactivate
func (h *ThemeHandler) Deactivate(c *gin.Context) {
	if err := h.themeService.Deactivate(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "theme deactivated"})
}

// Delete removes a theme
// DELETE /api/v1/admin/themes/:short
func (h *ThemeHandler) Delete(c *gin.Context) {
	short := c.Param("short")
	if err := h.themeService.Delete(c.Request.Context(), short); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "theme deleted"})
}

// UpdateConfig updates theme configuration
// PUT /api/v1/admin/themes/:short/config
func (h *ThemeHandler) UpdateConfig(c *gin.Context) {
	short := c.Param("short")
	var config map[string]string
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid config format"})
		return
	}
	if err := h.themeService.UpdateConfig(c.Request.Context(), short, config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "config updated"})
}
