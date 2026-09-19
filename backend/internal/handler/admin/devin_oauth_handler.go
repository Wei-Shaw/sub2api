// devin_oauth_handler.go 提供 Devin 平台 PKCE 登录的管理端点：
// 生成授权 URL（用户打开后复制授权码）→ 粘贴授权码交换 token。
package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// DevinOAuthHandler 处理 Devin PKCE 授权请求。
type DevinOAuthHandler struct {
	devinOAuthService *service.DevinOAuthService
}

// NewDevinOAuthHandler 创建 handler。
func NewDevinOAuthHandler(devinOAuthService *service.DevinOAuthService) *DevinOAuthHandler {
	return &DevinOAuthHandler{devinOAuthService: devinOAuthService}
}

// DevinGenerateAuthURLRequest 是生成授权链接的请求。
type DevinGenerateAuthURLRequest struct {
	ProxyID *int64 `json:"proxy_id"`
}

// GenerateAuthURL 生成 Devin 授权链接。
// POST /api/v1/admin/devin/oauth/auth-url
func (h *DevinOAuthHandler) GenerateAuthURL(c *gin.Context) {
	var req DevinGenerateAuthURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求无效: "+err.Error())
		return
	}

	result, err := h.devinOAuthService.GenerateAuthURL(c.Request.Context(), req.ProxyID)
	if err != nil {
		response.InternalError(c, "生成授权链接失败: "+err.Error())
		return
	}

	response.Success(c, result)
}

// DevinExchangeCodeRequest 是粘贴授权码交换的请求。
// session_id/state 仅在 code 为 PKCE 授权码时需要；直接粘贴
// devin-session-token$… 形态的 api key 可省略会话。
type DevinExchangeCodeRequest struct {
	SessionID string `json:"session_id"`
	State     string `json:"state"`
	Code      string `json:"code" binding:"required"`
	ProxyID   *int64 `json:"proxy_id"`
}

// ExchangeCode 用粘贴的授权码交换 token 并拉取账号信息。
// POST /api/v1/admin/devin/oauth/exchange-code
func (h *DevinOAuthHandler) ExchangeCode(c *gin.Context) {
	var req DevinExchangeCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求无效: "+err.Error())
		return
	}

	tokenInfo, err := h.devinOAuthService.ExchangeCode(c.Request.Context(), &service.DevinExchangeCodeInput{
		SessionID: req.SessionID,
		State:     req.State,
		Code:      req.Code,
		ProxyID:   req.ProxyID,
	})
	if err != nil {
		response.BadRequest(c, "Token 交换失败: "+err.Error())
		return
	}

	response.Success(c, tokenInfo)
}
