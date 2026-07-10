package routes

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
)

var mockVideoAssetNamePattern = regexp.MustCompile(`^\d+\.svg$`)

// RegisterCommonRoutes registers health, setup status, and safe public utility routes.
func RegisterCommonRoutes(r *gin.Engine) {
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	r.GET("/api/v1/video/mock-assets/:id", serveMockVideoAsset)

	r.POST("/api/event_logging/batch", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// The frontend uses this endpoint to detect when the service has restarted after setup.
	r.GET("/setup/status", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"data": gin.H{
				"needs_setup": false,
				"step":        "completed",
			},
		})
	})
}

func serveMockVideoAsset(c *gin.Context) {
	name := strings.TrimSpace(c.Param("id"))
	if !mockVideoAssetNamePattern.MatchString(name) {
		c.String(http.StatusNotFound, "mock asset not found")
		return
	}

	id := strings.TrimSuffix(name, ".svg")
	c.Header("Cache-Control", "no-store")
	c.Header("Content-Disposition", fmt.Sprintf(`inline; filename="mock-video-%s.svg"`, id))
	c.Data(http.StatusOK, "image/svg+xml; charset=utf-8", []byte(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="1280" height="720" viewBox="0 0 1280 720" role="img" aria-label="Sub2API mock video result %s">
  <defs>
    <linearGradient id="bg" x1="0" x2="1" y1="0" y2="1">
      <stop offset="0" stop-color="#102030"/>
      <stop offset="0.55" stop-color="#24645f"/>
      <stop offset="1" stop-color="#f0b35a"/>
    </linearGradient>
  </defs>
  <rect width="1280" height="720" fill="url(#bg)"/>
  <rect x="80" y="80" width="1120" height="560" rx="28" fill="rgba(255,255,255,0.10)" stroke="rgba(255,255,255,0.34)" stroke-width="2"/>
  <circle cx="640" cy="330" r="86" fill="rgba(255,255,255,0.20)"/>
  <polygon points="612,276 612,384 704,330" fill="#fff"/>
  <text x="640" y="486" text-anchor="middle" fill="#fff" font-family="Arial, sans-serif" font-size="40" font-weight="700">Sub2API Mock Video Result #%s</text>
  <text x="640" y="540" text-anchor="middle" fill="rgba(255,255,255,0.82)" font-family="Arial, sans-serif" font-size="24">Local preview asset, no real provider call</text>
</svg>`, id, id)))
}
