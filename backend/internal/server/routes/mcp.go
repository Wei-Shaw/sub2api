package routes

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterMCPRoutes 注册智谱远程 MCP Server 透传路由。入站 path 为
// /api/zhipu/api/mcp/{slug}/mcp：/api/zhipu 前缀 + 智谱官方 path 原样拼接，
// 客户端把官方 origin https://open.bigmodel.cn 替换为 https://<host>/api/zhipu
// 即可接入，官方 path /api/mcp/{slug}/mcp 无需改动。
// 旧入站 path /api/mcp/zhipu/{slug}/mcp 与官方 path 不一致，已废弃且不兼容。
// POST（JSON-RPC / SSE 响应）与 DELETE（终止 session）；GET（server push）
// 第一版不支持，显式返回 405。
func RegisterMCPRoutes(
	r *gin.Engine,
	h *handler.Handlers,
	apiKeyAuth middleware.APIKeyAuthMiddleware,
	cfg *config.Config,
) {
	// CORS 由 router.go 的全局 middleware2.CORS 统一覆盖，这里不重复挂载。
	mcp := r.Group("/api/zhipu")
	// MCP JSON-RPC 请求体与网关其他端点同量级，统一走全局 body 上限。
	mcp.Use(middleware.RequestBodyLimit(cfg.Gateway.MaxBodySize))
	mcp.Use(middleware.ClientRequestID())
	mcp.Use(gin.HandlerFunc(apiKeyAuth))

	mcp.POST("/api/mcp/:slug/mcp", h.Gateway.ZhipuMCPPassthrough)
	mcp.DELETE("/api/mcp/:slug/mcp", h.Gateway.ZhipuMCPPassthrough)
	mcp.GET("/api/mcp/:slug/mcp", zhipuMCPMethodNotAllowed)
}

// zhipuMCPMethodNotAllowed 拒绝 MCP GET（server push）请求。
// 返回 405 + Allow 头，让 MCP 客户端明确本端点仅支持 Streamable HTTP 的
// POST/DELETE 形态，而不是收到无语义的 404。
func zhipuMCPMethodNotAllowed(c *gin.Context) {
	c.Header("Allow", "POST, DELETE")
	c.JSON(http.StatusMethodNotAllowed, gin.H{"error": gin.H{
		"type":    "method_not_allowed",
		"message": "MCP server-push (GET) is not supported; use POST",
	}})
}
