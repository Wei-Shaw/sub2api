package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func RegisterModelPriceRoutes(v1 *gin.RouterGroup, h *handler.Handlers, jwtAuth middleware.JWTAuthMiddleware, settingService *service.SettingService, panelRateLimiter *middleware.PanelRateLimiter) {
	prices := v1.Group("/model-prices")
	prices.Use(panelRateLimiter.PublicIP())
	prices.Use(gin.HandlerFunc(jwtAuth))
	prices.Use(middleware.BackendModeUserGuard(settingService))
	prices.GET("", h.ModelPrice.Get)
}
