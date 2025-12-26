package server

import (
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"

	"github.com/gin-gonic/gin"
	"github.com/google/wire"
)

// ProviderSet 提供服务器层的依赖
var ProviderSet = wire.NewSet(
	ProvideRouter,
	ProvideHTTPServer,
)

// ProvideRouter 提供路由器
func ProvideRouter(
	cfg *config.Config,
	handlers *handler.Handlers,
	jwtAuth middleware2.JWTAuthMiddleware,
	adminAuth middleware2.AdminAuthMiddleware,
	apiKeyAuth middleware2.ApiKeyAuthMiddleware,
) *gin.Engine {
	if cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
REDACTED

	r := gin.New()
	r.Use(middleware2.Recovery())

	return SetupRouter(r, handlers, jwtAuth, adminAuth, apiKeyAuth)
REDACTED

// ProvideHTTPServer 提供 HTTP 服务器
func ProvideHTTPServer(cfg *config.Config, router *gin.Engine) *http.Server {
	return &http.Server{
		Addr:    cfg.Server.Address(),
		Handler: router,
		// ReadHeaderTimeout: 读取请求头的超时时间，防止慢速请求头攻击
		ReadHeaderTimeout: time.Duration(cfg.Server.ReadHeaderTimeout) * time.Second,
		// IdleTimeout: 空闲连接超时时间，释放不活跃的连接资源
		IdleTimeout: time.Duration(cfg.Server.IdleTimeout) * time.Second,
		// 注意：不设置 WriteTimeout，因为流式响应可能持续十几分钟
		// 不设置 ReadTimeout，因为大请求体可能需要较长时间读取
REDACTED
REDACTED
