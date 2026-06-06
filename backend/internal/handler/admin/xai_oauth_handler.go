package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// XAIOAuthHandler handles xAI OAuth-related operations.
type XAIOAuthHandler struct {
	xaiOAuthService *service.XAIOAuthService
	adminService    service.AdminService
}

func NewXAIOAuthHandler(xaiOAuthService *service.XAIOAuthService, adminService service.AdminService) *XAIOAuthHandler {
	return &XAIOAuthHandler{
		xaiOAuthService: xaiOAuthService,
		adminService:    adminService,
	}
}

type XAIGenerateAuthURLRequest struct {
	ProxyID     *int64 `json:"proxy_id"`
	RedirectURI string `json:"redirect_uri"`
}

// GenerateAuthURL generates xAI OAuth authorization URL.
// POST /api/v1/admin/xai/oauth/auth-url
func (h *XAIOAuthHandler) GenerateAuthURL(c *gin.Context) {
	var req XAIGenerateAuthURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = XAIGenerateAuthURLRequest{}
	}

	result, err := h.xaiOAuthService.GenerateAuthURL(c.Request.Context(), req.ProxyID, req.RedirectURI)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, result)
}

type XAIExchangeCodeRequest struct {
	SessionID   string `json:"session_id" binding:"required"`
	State       string `json:"state" binding:"required"`
	Code        string `json:"code" binding:"required"`
	RedirectURI string `json:"redirect_uri"`
	ProxyID     *int64 `json:"proxy_id"`
}

// ExchangeCode exchanges authorization code for xAI tokens.
// POST /api/v1/admin/xai/oauth/exchange-code
func (h *XAIOAuthHandler) ExchangeCode(c *gin.Context) {
	var req XAIExchangeCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求无效: "+err.Error())
		return
	}

	tokenInfo, err := h.xaiOAuthService.ExchangeCode(c.Request.Context(), &service.XAIExchangeCodeInput{
		SessionID:   req.SessionID,
		State:       req.State,
		Code:        req.Code,
		RedirectURI: req.RedirectURI,
		ProxyID:     req.ProxyID,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, tokenInfo)
}

type XAIRefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
	ProxyID      *int64 `json:"proxy_id"`
}

// RefreshToken validates an xAI refresh token and returns full token info.
// POST /api/v1/admin/xai/oauth/refresh-token
func (h *XAIOAuthHandler) RefreshToken(c *gin.Context) {
	var req XAIRefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求无效: "+err.Error())
		return
	}

	tokenInfo, err := h.xaiOAuthService.ValidateRefreshToken(c.Request.Context(), req.RefreshToken, req.ProxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, tokenInfo)
}

// RefreshAccountToken refreshes token for a specific xAI account.
// POST /api/v1/admin/xai/accounts/:id/refresh
func (h *XAIOAuthHandler) RefreshAccountToken(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}

	account, err := h.adminService.GetAccount(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if account.Platform != service.PlatformXAI {
		response.BadRequest(c, "Account platform does not match OAuth endpoint")
		return
	}
	if !account.IsOAuth() {
		response.BadRequest(c, "Cannot refresh non-OAuth account credentials")
		return
	}

	tokenInfo, err := h.xaiOAuthService.RefreshAccountToken(c.Request.Context(), account)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	newCredentials := h.xaiOAuthService.BuildAccountCredentials(tokenInfo)
	newCredentials = service.MergeCredentials(account.Credentials, newCredentials)

	updatedAccount, err := h.adminService.UpdateAccount(c.Request.Context(), accountID, &service.UpdateAccountInput{
		Credentials: newCredentials,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.AccountFromService(updatedAccount))
}
