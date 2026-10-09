package admin

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *SystemHandler) SetDatabaseMaintenanceService(s *service.DatabaseMaintenanceService) {
	h.databaseSvc = s
}

func (h *SystemHandler) GetDatabaseStats(c *gin.Context) {
	stats, err := h.databaseSvc.Stats(c.Request.Context())
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, stats)
}

func (h *SystemHandler) GetDatabaseMaintenanceJob(c *gin.Context) {
	job, err := h.databaseSvc.LatestJob(c.Request.Context())
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, gin.H{"job": job})
}

func (h *SystemHandler) StartDatabaseMaintenance(c *gin.Context) {
	var req service.DatabaseMaintenanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "Unauthorized")
		return
	}
	executeAdminIdempotentJSON(c, "admin.database.maintenance", req, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		return h.databaseSvc.Start(ctx, req, subject.UserID)
	})
}
