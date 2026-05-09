package routes

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/gin-gonic/gin"
)

// RegisterCommonRoutes 注册通用路由（健康检查、状态等）
func RegisterCommonRoutes(r *gin.Engine, h *handler.Handlers) {
	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	if h != nil && h.GeneratedImage != nil {
		r.GET("/sub2api/generated-images/:filename", h.GeneratedImage.Get)
	}

	if h != nil && h.ConfigGuide != nil {
		configGuides := r.Group("/config-guides")
		{
			omp := configGuides.Group("/omp-openai")
			omp.GET("/manifest.json", h.ConfigGuide.GetOMPManifest)
			omp.GET("/plugin.txt", h.ConfigGuide.GetOMPPluginInstructions)
			omp.GET("/models.yml", h.ConfigGuide.GetOMPModelsYAML)
			omp.GET("/config.yml", h.ConfigGuide.GetOMPConfigYAML)
			omp.GET("/image-generator.md", h.ConfigGuide.GetOMPImageGenerator)

			opencode := configGuides.Group("/opencode-openai")
			opencode.GET("/manifest.json", h.ConfigGuide.GetOpenCodeManifest)
			opencode.GET("/opencode.json", h.ConfigGuide.GetOpenCodeJSON)
		}
	}

	// Claude Code 遥测日志（忽略，直接返回200）
	r.POST("/api/event_logging/batch", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// Setup status endpoint (always returns needs_setup: false in normal mode)
	// This is used by the frontend to detect when the service has restarted after setup
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
