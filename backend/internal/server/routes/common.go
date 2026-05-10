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

	// 公开监控面板
	monitoring := r.Group("/api/v1/monitoring")
	{
		monitoring.GET("/overview", h.Admin.Monitoring.GetOverview)
		monitoring.GET("/summary", h.Admin.Monitoring.GetSummary)
		monitoring.GET("/group-models", h.Admin.Monitoring.GetGroupModels)
		monitoring.GET("/model-latency", h.Admin.Monitoring.GetModelLatency)
	}

	// 公开模型定价查询
	r.GET("/api/v1/public/pricing", h.Admin.ModelPricing.GetPublicPricing)
}
