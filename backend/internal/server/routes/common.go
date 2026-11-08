package routes

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/handler"

	"github.com/gin-gonic/gin"
)

// RegisterCommonRoutes 注册通用路由（健康检查、状态、对外公开接口等）
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

	// 公开模型价格接口：刻意挂在 /api/v1 之外，且不挂任何鉴权中间件——
	// 第三方调用方无需登录直接读取价格，因此这里不能出现 jwtAuth/adminAuth。
	//
	// 响应体不是本项目通用的 {code,message,data} 信封，而是旧 companion 的
	// {schema_version,success,message,data} 形状：这是已经对外发布的第三方契约，
	// 逐字节保持不变优先于风格统一。
	if h != nil && h.ProviderPricing != nil {
		r.GET("/api/provider/pricing", h.ProviderPricing.GetPricing)
	}
}
