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
	accounts.POST("/:id/byteplus-prepaid/:action", h.BytePlusPrepaidUpdate)
	accounts.POST("/:id/byteplus-prepaid/reservations/:reservation_id/reconcile", h.BytePlusPrepaidReconcile)
}
