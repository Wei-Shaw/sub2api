package handler

import (
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ThemeAssetHandler serves theme static files (CSS, fonts, images)
type ThemeAssetHandler struct {
	themeService *service.ThemeService
}

// NewThemeAssetHandler creates a new ThemeAssetHandler
func NewThemeAssetHandler(themeService *service.ThemeService) *ThemeAssetHandler {
	return &ThemeAssetHandler{themeService: themeService}
}

// ServeAsset serves a file from a theme directory
// GET /api/v1/themes/assets/:short/*filepath
func (h *ThemeAssetHandler) ServeAsset(c *gin.Context) {
	short := c.Param("short")
	filepath := c.Param("filepath")

	// Remove leading slash from filepath
	if len(filepath) > 0 && filepath[0] == '/' {
		filepath = filepath[1:]
	}

	content, contentType, err := h.themeService.ServeThemeAsset(c.Request.Context(), short, filepath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.Header("Content-Type", contentType)
	c.Header("Cache-Control", "public, max-age=86400")
	c.Header("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
	c.Data(http.StatusOK, contentType, content)
}
