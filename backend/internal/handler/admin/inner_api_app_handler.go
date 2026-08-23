package admin

import (
	"log/slog"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// InnerAPIAppHandler 暴露 admin 端的内部 API app 管理接口。
type InnerAPIAppHandler struct {
	svc    *service.InnerAPIAppService
	ledger *service.BalanceLedgerService
}

// NewInnerAPIAppHandler 构造 InnerAPIAppHandler。
func NewInnerAPIAppHandler(svc *service.InnerAPIAppService, ledger *service.BalanceLedgerService) *InnerAPIAppHandler {
	return &InnerAPIAppHandler{svc: svc, ledger: ledger}
}

// adminInnerAPIApp 是 admin 列表/详情返回的安全视图（永不含 secret hash）。
type adminInnerAPIApp struct {
	ID          int64    `json:"id"`
	AppID       string   `json:"app_id"`
	AppName     string   `json:"app_name"`
	Enabled     bool     `json:"enabled"`
	Permissions []string `json:"permissions"`
}

func toAdminInnerAPIApp(v *service.InnerAPIApp) *adminInnerAPIApp {
	if v == nil {
		return nil
	}
	return &adminInnerAPIApp{
		ID:          v.ID,
		AppID:       v.AppID,
		AppName:     v.AppName,
		Enabled:     v.Enabled,
		Permissions: append([]string(nil), v.Permissions...),
	}
}

type createInnerAPIAppRequest struct {
	AppName     string   `json:"app_name"`
	Permissions []string `json:"permissions"`
}

// createInnerAPIAppResponse 创建响应：附带一次性 token（之后无法再获取）。
type createInnerAPIAppResponse struct {
	*adminInnerAPIApp
	Token string `json:"token"`
}

type setInnerAPIAppEnabledRequest struct {
	Enabled bool `json:"enabled"`
}

type setInnerAPIAppPermissionsRequest struct {
	Permissions []string `json:"permissions"`
}

func auditInnerAPIApp(c *gin.Context, action string, fields ...any) {
	subject, _ := middleware.GetAuthSubjectFromContext(c)
	role, _ := middleware.GetUserRoleFromContext(c)
	args := make([]any, 0, 8+len(fields))
	args = append(args,
		"audit", true,
		"component", "audit.inner_api_app",
		"action", action,
		"operator_id", subject.UserID,
		"role", role,
	)
	args = append(args, fields...)
	slog.Info("inner api app admin operation", args...)
}

// List 列出所有内部 API app。
func (h *InnerAPIAppHandler) List(c *gin.Context) {
	rows, err := h.svc.List(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]*adminInnerAPIApp, 0, len(rows))
	for _, r := range rows {
		out = append(out, toAdminInnerAPIApp(r))
	}
	response.Success(c, out)
}

// Create 创建内部 API app，返回一次性明文 token。
func (h *InnerAPIAppHandler) Create(c *gin.Context) {
	var req createInnerAPIAppRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	view, token, err := h.svc.CreateApp(c.Request.Context(), req.AppName, req.Permissions)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	auditInnerAPIApp(c, "inner_api_app.create", "app_id", view.AppID, "app_name", view.AppName)
	response.Created(c, createInnerAPIAppResponse{
		adminInnerAPIApp: toAdminInnerAPIApp(view),
		Token:            token,
	})
}

// SetPermissions 更新 app 的权限集合。
func (h *InnerAPIAppHandler) SetPermissions(c *gin.Context) {
	appID := c.Param("app_id")
	var req setInnerAPIAppPermissionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if err := h.svc.SetPermissions(c.Request.Context(), appID, req.Permissions); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	auditInnerAPIApp(c, "inner_api_app.set_permissions", "app_id", appID, "permissions", req.Permissions)
	response.Success(c, gin.H{"app_id": appID, "permissions": req.Permissions})
}

// SetEnabled 启用/停用内部 API app。
func (h *InnerAPIAppHandler) SetEnabled(c *gin.Context) {
	appID := c.Param("app_id")
	var req setInnerAPIAppEnabledRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if err := h.svc.SetEnabled(c.Request.Context(), appID, req.Enabled); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	auditInnerAPIApp(c, "inner_api_app.set_enabled", "app_id", appID, "enabled", req.Enabled)
	response.Success(c, gin.H{"app_id": appID, "enabled": req.Enabled})
}

// RefreshToken 刷新 token（旧 token 立即失效），返回一次性新 token。
func (h *InnerAPIAppHandler) RefreshToken(c *gin.Context) {
	appID := c.Param("app_id")
	token, err := h.svc.RefreshToken(c.Request.Context(), appID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	auditInnerAPIApp(c, "inner_api_app.refresh_token", "app_id", appID)
	response.Success(c, gin.H{"app_id": appID, "token": token})
}

// Delete 删除内部 API app（历史流水保留）。
func (h *InnerAPIAppHandler) Delete(c *gin.Context) {
	appID := c.Param("app_id")
	if err := h.svc.DeleteApp(c.Request.Context(), appID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	auditInnerAPIApp(c, "inner_api_app.delete", "app_id", appID)
	response.Success(c, gin.H{"app_id": appID})
}

// Stats 返回某内部 API app 的累计扣/退统计。
func (h *InnerAPIAppHandler) Stats(c *gin.Context) {
	appID := c.Param("app_id")
	stats, err := h.ledger.AppStats(c.Request.Context(), appID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, stats)
}
