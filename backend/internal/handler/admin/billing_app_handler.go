package admin

import (
	"log/slog"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// BillingAppHandler 暴露 admin 端的「扣费 app」(余额 RPC 接入方) 管理接口。
type BillingAppHandler struct {
	svc    *service.BillingAppService
	ledger *service.BalanceLedgerService
}

// NewBillingAppHandler 构造 BillingAppHandler。
func NewBillingAppHandler(svc *service.BillingAppService, ledger *service.BalanceLedgerService) *BillingAppHandler {
	return &BillingAppHandler{svc: svc, ledger: ledger}
}

// adminBillingApp 是 admin 列表/详情返回的安全视图（永不含 secret hash）。
type adminBillingApp struct {
	ID      int64  `json:"id"`
	AppID   string `json:"app_id"`
	AppName string `json:"app_name"`
	Enabled bool   `json:"enabled"`
}

func toAdminBillingApp(v *service.BillingApp) *adminBillingApp {
	if v == nil {
		return nil
	}
	return &adminBillingApp{
		ID:      v.ID,
		AppID:   v.AppID,
		AppName: v.AppName,
		Enabled: v.Enabled,
	}
}

type createBillingAppRequest struct {
	AppName string `json:"app_name"`
}

// createBillingAppResponse 创建响应：附带一次性 token（之后无法再获取）。
type createBillingAppResponse struct {
	*adminBillingApp
	Token string `json:"token"`
}

type setBillingAppEnabledRequest struct {
	Enabled bool `json:"enabled"`
}

func auditBillingApp(c *gin.Context, action string, fields ...any) {
	subject, _ := middleware.GetAuthSubjectFromContext(c)
	role, _ := middleware.GetUserRoleFromContext(c)
	args := make([]any, 0, 8+len(fields))
	args = append(args,
		"audit", true,
		"component", "audit.billing_app",
		"action", action,
		"operator_id", subject.UserID,
		"role", role,
	)
	args = append(args, fields...)
	slog.Info("billing app admin operation", args...)
}

// List 列出所有扣费 app。
func (h *BillingAppHandler) List(c *gin.Context) {
	rows, err := h.svc.List(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]*adminBillingApp, 0, len(rows))
	for _, r := range rows {
		out = append(out, toAdminBillingApp(r))
	}
	response.Success(c, out)
}

// Create 创建扣费 app，返回一次性明文 secret。
func (h *BillingAppHandler) Create(c *gin.Context) {
	var req createBillingAppRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	view, token, err := h.svc.CreateApp(c.Request.Context(), req.AppName)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	auditBillingApp(c, "billing_app.create", "app_id", view.AppID, "app_name", view.AppName)
	response.Created(c, createBillingAppResponse{
		adminBillingApp: toAdminBillingApp(view),
		Token:           token,
	})
}

// SetEnabled 启用/停用扣费 app。
func (h *BillingAppHandler) SetEnabled(c *gin.Context) {
	appID := c.Param("app_id")
	var req setBillingAppEnabledRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if err := h.svc.SetEnabled(c.Request.Context(), appID, req.Enabled); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	auditBillingApp(c, "billing_app.set_enabled", "app_id", appID, "enabled", req.Enabled)
	response.Success(c, gin.H{"app_id": appID, "enabled": req.Enabled})
}

// RefreshToken 刷新 token（旧 token 立即失效），返回一次性新 token。
func (h *BillingAppHandler) RefreshToken(c *gin.Context) {
	appID := c.Param("app_id")
	token, err := h.svc.RefreshToken(c.Request.Context(), appID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	auditBillingApp(c, "billing_app.refresh_token", "app_id", appID)
	response.Success(c, gin.H{"app_id": appID, "token": token})
}

// Delete 删除扣费 app（历史流水保留）。
func (h *BillingAppHandler) Delete(c *gin.Context) {
	appID := c.Param("app_id")
	if err := h.svc.DeleteApp(c.Request.Context(), appID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	auditBillingApp(c, "billing_app.delete", "app_id", appID)
	response.Success(c, gin.H{"app_id": appID})
}

// Stats 返回某扣费 app 的累计扣/退统计。
func (h *BillingAppHandler) Stats(c *gin.Context) {
	appID := c.Param("app_id")
	stats, err := h.ledger.AppStats(c.Request.Context(), appID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, stats)
}
