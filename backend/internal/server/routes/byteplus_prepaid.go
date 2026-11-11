package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/gin-gonic/gin"
)

// The parent account group supplies admin authentication, audit and compliance
// middleware. Each handler additionally rejects delegated/scoped administrators.
func registerBytePlusPrepaidRoutes(accounts *gin.RouterGroup, h *admin.AccountHandler) {
	accounts.GET("/:id/byteplus-prepaid", h.BytePlusPrepaidStatus)
	accounts.GET("/:id/byteplus-prepaid/pools", h.BytePlusPrepaidPools)
	accounts.GET("/:id/byteplus-prepaid/provider-status", h.BytePlusProviderStatus)
	accounts.POST("/:id/byteplus-prepaid/provider-query", h.BytePlusProviderQuery)
	accounts.POST("/:id/byteplus-prepaid/:action", h.BytePlusPrepaidUpdate)
	accounts.POST("/:id/byteplus-prepaid/reservations/:reservation_id/reconcile", h.BytePlusPrepaidReconcile)
}
