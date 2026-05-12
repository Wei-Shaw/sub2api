package admin

import (
	"encoding/json"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/plugin"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type AntigravityOAuthHandler struct {
	antigravityOAuthService *service.AntigravityOAuthService
	platformRegistry        *plugin.PlatformRegistry
}

func NewAntigravityOAuthHandler(antigravityOAuthService *service.AntigravityOAuthService) *AntigravityOAuthHandler {
	return &AntigravityOAuthHandler{antigravityOAuthService: antigravityOAuthService}
}

type AntigravityGenerateAuthURLRequest struct {
	ProxyID *int64 `json:"proxy_id"`
}

// GenerateAuthURL generates Google OAuth authorization URL
// POST /api/v1/admin/antigravity/oauth/auth-url
func (h *AntigravityOAuthHandler) GenerateAuthURL(c *gin.Context) {
	var req AntigravityGenerateAuthURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求无效: "+err.Error())
		return
	}

	// Try plugin delegation first.
	var proxyID int64
	if req.ProxyID != nil {
		proxyID = *req.ProxyID
	}
	if authURL, sessionID, handled := tryPluginGenerateAuthURL(
		c.Request.Context(), h.platformRegistry,
		service.PlatformAntigravity, "antigravity", proxyID, "", nil,
	); handled {
		response.Success(c, gin.H{"auth_url": authURL, "session_id": sessionID})
		return
	}

	result, err := h.antigravityOAuthService.GenerateAuthURL(c.Request.Context(), req.ProxyID)
	if err != nil {
		response.InternalError(c, "生成授权链接失败: "+err.Error())
		return
	}

	response.Success(c, result)
}

type AntigravityExchangeCodeRequest struct {
	SessionID string `json:"session_id" binding:"required"`
	State     string `json:"state" binding:"required"`
	Code      string `json:"code" binding:"required"`
	ProxyID   *int64 `json:"proxy_id"`
}

// ExchangeCode 用 authorization code 交换 token
// POST /api/v1/admin/antigravity/oauth/exchange-code
func (h *AntigravityOAuthHandler) ExchangeCode(c *gin.Context) {
	var req AntigravityExchangeCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求无效: "+err.Error())
		return
	}

	// Try plugin delegation first.
	var proxyID int64
	if req.ProxyID != nil {
		proxyID = *req.ProxyID
	}
	if credsJSON, extraJSON, accountName, tierID, handled := tryPluginExchangeOAuthCode(
		c.Request.Context(), h.platformRegistry,
		service.PlatformAntigravity, "antigravity", req.SessionID, req.Code, req.State,
		proxyID, "", nil,
	); handled {
		result := gin.H{"credentials_json": json.RawMessage(credsJSON)}
		if len(extraJSON) > 0 {
			result["extra_json"] = json.RawMessage(extraJSON)
		}
		if accountName != "" {
			result["account_name"] = accountName
		}
		if tierID != "" {
			result["tier_id"] = tierID
		}
		response.Success(c, result)
		return
	}

	tokenInfo, err := h.antigravityOAuthService.ExchangeCode(c.Request.Context(), &service.AntigravityExchangeCodeInput{
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

// AntigravityRefreshTokenRequest represents the request for validating Antigravity refresh token
type AntigravityRefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
	ProxyID      *int64 `json:"proxy_id"`
}

// RefreshToken validates an Antigravity refresh token and returns full token info
// POST /api/v1/admin/antigravity/oauth/refresh-token
func (h *AntigravityOAuthHandler) RefreshToken(c *gin.Context) {
	var req AntigravityRefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求无效: "+err.Error())
		return
	}

	// Try plugin delegation first.
	var proxyID int64
	if req.ProxyID != nil {
		proxyID = *req.ProxyID
	}
	if credsJSON, extraJSON, accountName, tierID, handled := tryPluginValidateRefreshToken(
		c.Request.Context(), h.platformRegistry,
		service.PlatformAntigravity, req.RefreshToken, proxyID, nil,
	); handled {
		result := gin.H{"credentials_json": json.RawMessage(credsJSON)}
		if len(extraJSON) > 0 {
			result["extra_json"] = json.RawMessage(extraJSON)
		}
		if accountName != "" {
			result["account_name"] = accountName
		}
		if tierID != "" {
			result["tier_id"] = tierID
		}
		response.Success(c, result)
		return
	}

	tokenInfo, err := h.antigravityOAuthService.ValidateRefreshToken(c.Request.Context(), req.RefreshToken, req.ProxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, tokenInfo)
}

// SetPlatformRegistry sets the platform registry after construction.
func (h *AntigravityOAuthHandler) SetPlatformRegistry(registry *plugin.PlatformRegistry) {
	h.platformRegistry = registry
}
