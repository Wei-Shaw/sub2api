package handler

import (
	pkgerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// UserServiceQuotaHandler 是用户端"我的服务限额"入口。
//
// 与 admin 端 ServiceQuotaMonitorHandler 共享同一份 ServiceQuotaMonitorService，
// 但对外只暴露 user_scope 过滤，强制忽略其他 query 字段（即使前端误传也不会泄露）。
type UserServiceQuotaHandler struct {
	svc service.ServiceQuotaMonitorService
}

func NewUserServiceQuotaHandler(svc service.ServiceQuotaMonitorService) *UserServiceQuotaHandler {
	return &UserServiceQuotaHandler{svc: svc}
}

// MyQuota 处理 GET /api/v1/service-quotas/my。
//
// 不接受任何 query；从 jwt context 拿 UserID 做唯一过滤维度。
// 缺失认证态返回 401，service 层错误透传 500 给前端。
func (h *UserServiceQuotaHandler) MyQuota(c *gin.Context) {
	if h == nil || h.svc == nil {
		response.ErrorFrom(c, pkgerrors.NotFound(
			"SERVICE_QUOTA_MONITOR_UNAVAILABLE",
			"service quota monitor unavailable",
		))
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.ErrorFrom(c, pkgerrors.Unauthorized(
			"UNAUTHENTICATED",
			"authentication required",
		))
		return
	}
	snap, err := h.svc.Snapshot(c.Request.Context(), service.MonitorSnapshotFilter{
		UserScope: &service.MonitorUserScope{UserID: subject.UserID},
	})
	if err != nil {
		response.ErrorFrom(c, pkgerrors.InternalServer(
			"SNAPSHOT_FAILED",
			"failed to load quota snapshot",
		).WithCause(err))
		return
	}
	response.Success(c, snap)
}
