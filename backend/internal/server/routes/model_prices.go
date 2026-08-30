package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func RegisterModelPriceRoutes(v1 *gin.RouterGroup, h *handler.Handlers, optionalJWTAuth middleware.OptionalJWTAuthMiddleware, _ *service.SettingService, panelRateLimiter *middleware.PanelRateLimiter) {
	prices := v1.Group("/model-prices")
	prices.Use(panelRateLimiter.PublicIP())
	prices.Use(gin.HandlerFunc(optionalJWTAuth))
	prices.GET("", h.ModelPrice.Get)
}
