package admin

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type DevinOAuthHandler struct {
	devinOAuthService *service.DevinOAuthService
}

func NewDevinOAuthHandler(devinOAuthService *service.DevinOAuthService) *DevinOAuthHandler {
	return &DevinOAuthHandler{devinOAuthService: devinOAuthService}
}

type devinGenerateAuthURLRequest struct {
	ProxyID *int64 `json:"proxy_id"`
}

func (h *DevinOAuthHandler) GenerateAuthURL(c *gin.Context) {
	var req devinGenerateAuthURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = devinGenerateAuthURLRequest{}
	}
	result, err := h.devinOAuthService.GenerateAuthURL(c.Request.Context(), req.ProxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

type devinExchangeCodeRequest struct {
	SessionID string `json:"session_id" binding:"required"`
	Code      string `json:"code" binding:"required"`
	State     string `json:"state"`
	ProxyID   *int64 `json:"proxy_id"`
}

func (h *DevinOAuthHandler) ExchangeCode(c *gin.Context) {
	var req devinExchangeCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	tokenInfo, err := h.devinOAuthService.ExchangeCode(c.Request.Context(), &service.DevinExchangeCodeInput{
		SessionID: req.SessionID,
		Code:      req.Code,
		State:     req.State,
		ProxyID:   req.ProxyID,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, tokenInfo)
}

type devinImportTokenRequest struct {
	SessionToken string `json:"session_token"`
	AccessToken  string `json:"access_token"`
}

func (h *DevinOAuthHandler) ImportToken(c *gin.Context) {
	var req devinImportTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	token := strings.TrimSpace(req.SessionToken)
	if token == "" {
		token = strings.TrimSpace(req.AccessToken)
	}
	tokenInfo, err := h.devinOAuthService.ImportSessionToken(token)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, tokenInfo)
}
