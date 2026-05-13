package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type KiroOAuthHandler struct {
	kiroOAuthService *service.KiroOAuthService
}

func NewKiroOAuthHandler(kiroOAuthService *service.KiroOAuthService) *KiroOAuthHandler {
	return &KiroOAuthHandler{kiroOAuthService: kiroOAuthService}
}

type KiroGenerateAuthURLRequest struct {
	ProxyID  *int64 `json:"proxy_id"`
	Provider string `json:"provider"`
}

func (h *KiroOAuthHandler) GenerateAuthURL(c *gin.Context) {
	var req KiroGenerateAuthURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求无效: "+err.Error())
		return
	}
	result, err := h.kiroOAuthService.GenerateAuthURL(c.Request.Context(), &service.KiroGenerateAuthURLInput{
		ProxyID:  req.ProxyID,
		Provider: req.Provider,
	})
	if err != nil {
		response.BadRequest(c, "生成授权链接失败: "+err.Error())
		return
	}
	response.Success(c, result)
}

type KiroGenerateIDCAuthURLRequest struct {
	ProxyID  *int64 `json:"proxy_id"`
	StartURL string `json:"start_url"`
	Region   string `json:"region"`
}

func (h *KiroOAuthHandler) GenerateIDCAuthURL(c *gin.Context) {
	var req KiroGenerateIDCAuthURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求无效: "+err.Error())
		return
	}
	result, err := h.kiroOAuthService.GenerateIDCAuthURL(c.Request.Context(), &service.KiroGenerateIDCAuthURLInput{
		ProxyID:  req.ProxyID,
		StartURL: req.StartURL,
		Region:   req.Region,
	})
	if err != nil {
		response.BadRequest(c, "生成 IDC 授权链接失败: "+err.Error())
		return
	}
	response.Success(c, result)
}

type KiroExchangeCodeRequest struct {
	SessionID    string `json:"session_id" binding:"required"`
	State        string `json:"state" binding:"required"`
	Code         string `json:"code" binding:"required"`
	CallbackPath string `json:"callback_path"`
	LoginOption  string `json:"login_option"`
	ProxyID      *int64 `json:"proxy_id"`
}

func (h *KiroOAuthHandler) ExchangeCode(c *gin.Context) {
	var req KiroExchangeCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求无效: "+err.Error())
		return
	}
	tokenInfo, err := h.kiroOAuthService.ExchangeCode(c.Request.Context(), &service.KiroExchangeCodeInput{
		SessionID:    req.SessionID,
		State:        req.State,
		Code:         req.Code,
		CallbackPath: req.CallbackPath,
		LoginOption:  req.LoginOption,
		ProxyID:      req.ProxyID,
	})
	if err != nil {
		response.BadRequest(c, "Token 交换失败: "+err.Error())
		return
	}
	response.Success(c, tokenInfo)
}

type KiroRefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
	AuthMethod   string `json:"auth_method"`
	Provider     string `json:"provider"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	StartURL     string `json:"start_url"`
	Region       string `json:"region"`
	ProfileArn   string `json:"profile_arn"`
	ProxyID      *int64 `json:"proxy_id"`
}

func (h *KiroOAuthHandler) RefreshToken(c *gin.Context) {
	var req KiroRefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求无效: "+err.Error())
		return
	}
	tokenInfo, err := h.kiroOAuthService.RefreshToken(c.Request.Context(), &service.KiroRefreshTokenInput{
		RefreshToken: req.RefreshToken,
		AuthMethod:   req.AuthMethod,
		Provider:     req.Provider,
		ClientID:     req.ClientID,
		ClientSecret: req.ClientSecret,
		StartURL:     req.StartURL,
		Region:       req.Region,
		ProfileArn:   req.ProfileArn,
		ProxyID:      req.ProxyID,
	})
	if err != nil {
		response.BadRequest(c, "刷新 Kiro Token 失败: "+err.Error())
		return
	}
	response.Success(c, tokenInfo)
}

type KiroImportTokenRequest struct {
	TokenJSON              string `json:"token_json" binding:"required"`
	DeviceRegistrationJSON string `json:"device_registration_json"`
}

func (h *KiroOAuthHandler) ImportToken(c *gin.Context) {
	var req KiroImportTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求无效: "+err.Error())
		return
	}
	tokenInfo, err := h.kiroOAuthService.ImportToken(&service.KiroImportTokenInput{
		TokenJSON:              req.TokenJSON,
		DeviceRegistrationJSON: req.DeviceRegistrationJSON,
	})
	if err != nil {
		response.BadRequest(c, "导入 Kiro Token 失败: "+err.Error())
		return
	}
	response.Success(c, tokenInfo)
}
