package admin

import (
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// KimiOAuthHandler 提供 Kimi（kimi.com 订阅）OAuth 设备码登录的管理接口。
// 流程：device-code 发起 → 前端按 interval 驱动 poll 单次轮询 →
// 成功后走 create-from-oauth 一步建号（或前端自取 token 走通用账号创建）。
type KimiOAuthHandler struct {
	kimiOAuthService *service.KimiOAuthService
	adminService     service.AdminService
}

func NewKimiOAuthHandler(
	kimiOAuthService *service.KimiOAuthService,
	adminService service.AdminService,
) *KimiOAuthHandler {
	return &KimiOAuthHandler{
		kimiOAuthService: kimiOAuthService,
		adminService:     adminService,
	}
}

type KimiStartDeviceAuthRequest struct {
	ProxyID *int64 `json:"proxy_id"`
}

// StartDeviceAuth 发起设备授权，返回 user_code 与验证链接供用户确认。
func (h *KimiOAuthHandler) StartDeviceAuth(c *gin.Context) {
	var req KimiStartDeviceAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = KimiStartDeviceAuthRequest{}
	}
	result, err := h.kimiOAuthService.StartDeviceAuth(c.Request.Context(), req.ProxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

type KimiPollDeviceTokenRequest struct {
	SessionID string `json:"session_id" binding:"required"`
	ProxyID   *int64 `json:"proxy_id"`
}

// PollDeviceToken 单次轮询设备码（前端定时器驱动）。
// pending/slow_down 以业务码（reason）返回，前端据此前进或调整间隔。
func (h *KimiOAuthHandler) PollDeviceToken(c *gin.Context) {
	var req KimiPollDeviceTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	tokenInfo, err := h.kimiOAuthService.PollDeviceToken(c.Request.Context(), req.SessionID, req.ProxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, tokenInfo)
}

type KimiRefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token"`
	RT           string `json:"rt"`
	DeviceID     string `json:"device_id"`
	ProxyID      *int64 `json:"proxy_id"`
}

// RefreshToken 校验/刷新一个 Kimi refresh_token（device_id 需与签发时一致）。
func (h *KimiOAuthHandler) RefreshToken(c *gin.Context) {
	var req KimiRefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	refreshToken := strings.TrimSpace(req.RefreshToken)
	if refreshToken == "" {
		refreshToken = strings.TrimSpace(req.RT)
	}
	if refreshToken == "" {
		response.BadRequest(c, "refresh_token is required")
		return
	}

	var proxyURL string
	if req.ProxyID != nil {
		proxy, err := h.adminService.GetProxy(c.Request.Context(), *req.ProxyID)
		if err == nil && proxy != nil {
			proxyURL = proxy.URL()
		}
	}
	tokenInfo, err := h.kimiOAuthService.RefreshToken(c.Request.Context(), refreshToken, proxyURL, req.DeviceID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, tokenInfo)
}

// RefreshAccountToken 刷新指定 Kimi 账号的 token 并写回 credentials（含轮换后的 refresh_token）。
func (h *KimiOAuthHandler) RefreshAccountToken(c *gin.Context) {
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
	if account.Platform != service.PlatformKimi {
		response.BadRequest(c, "Account platform does not match Kimi OAuth endpoint")
		return
	}
	if !account.IsOAuth() {
		response.BadRequest(c, "Cannot refresh non-OAuth account credentials")
		return
	}
	tokenInfo, err := h.kimiOAuthService.RefreshAccountToken(c.Request.Context(), account)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	newCredentials := h.kimiOAuthService.BuildAccountCredentials(tokenInfo)
	newCredentials = service.MergeCredentials(account.Credentials, newCredentials)
	if baseURL := strings.TrimSpace(account.GetCredential("base_url")); baseURL != "" {
		newCredentials["base_url"] = baseURL
	}
	if deviceID := strings.TrimSpace(account.GetKimiDeviceID()); deviceID != "" {
		newCredentials["device_id"] = deviceID
	}
	updatedAccount, err := h.adminService.UpdateAccount(c.Request.Context(), accountID, &service.UpdateAccountInput{
		Credentials: newCredentials,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.AccountFromService(updatedAccount))
}

// CreateAccountFromOAuth 用已完成轮询的会话一步创建 Kimi OAuth 账号。
func (h *KimiOAuthHandler) CreateAccountFromOAuth(c *gin.Context) {
	var req struct {
		SessionID   string  `json:"session_id" binding:"required"`
		ProxyID     *int64  `json:"proxy_id"`
		Name        string  `json:"name"`
		Concurrency int     `json:"concurrency"`
		Priority    int     `json:"priority"`
		GroupIDs    []int64 `json:"group_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	tokenInfo, err := h.kimiOAuthService.GetSessionToken(req.SessionID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	credentials := h.kimiOAuthService.BuildAccountCredentials(tokenInfo)

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "Kimi OAuth Account"
	}

	account, err := h.adminService.CreateAccount(c.Request.Context(), &service.CreateAccountInput{
		Name:        name,
		Platform:    service.PlatformKimi,
		Type:        service.AccountTypeOAuth,
		Credentials: credentials,
		ProxyID:     req.ProxyID,
		Concurrency: req.Concurrency,
		Priority:    req.Priority,
		GroupIDs:    req.GroupIDs,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	// 建号成功，会话作废
	h.kimiOAuthService.DeleteSession(req.SessionID)
	response.Success(c, dto.AccountFromService(account))
}
