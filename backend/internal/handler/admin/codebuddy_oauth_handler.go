package admin

import (
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/codebuddy"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type CodeBuddyOAuthHandler struct {
	codeBuddyOAuthService *service.CodeBuddyOAuthService
	adminService          service.AdminService
}

func NewCodeBuddyOAuthHandler(
	codeBuddyOAuthService *service.CodeBuddyOAuthService,
	adminService service.AdminService,
) *CodeBuddyOAuthHandler {
	return &CodeBuddyOAuthHandler{
		codeBuddyOAuthService: codeBuddyOAuthService,
		adminService:          adminService,
	}
}

type CodeBuddyGenerateAuthURLRequest struct {
	ProxyID     *int64 `json:"proxy_id"`
	RedirectURI string `json:"redirect_uri"`
}

func (h *CodeBuddyOAuthHandler) GenerateAuthURL(c *gin.Context) {
	var req CodeBuddyGenerateAuthURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = CodeBuddyGenerateAuthURLRequest{}
	}
	result, err := h.codeBuddyOAuthService.GenerateAuthURL(c.Request.Context(), req.ProxyID, req.RedirectURI)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

type CodeBuddyExchangeStateRequest struct {
	SessionID string `json:"session_id"`
	State     string `json:"state"`
	ProxyID   *int64 `json:"proxy_id"`
}

func (h *CodeBuddyOAuthHandler) ExchangeState(c *gin.Context) {
	var req CodeBuddyExchangeStateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	tokenInfo, err := h.codeBuddyOAuthService.ExchangeState(c.Request.Context(), &service.CodeBuddyExchangeStateInput{
		SessionID: strings.TrimSpace(req.SessionID),
		State:     strings.TrimSpace(req.State),
		ProxyID:   req.ProxyID,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, tokenInfo)
}

type CodeBuddyRefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token"`
	ProxyID      *int64 `json:"proxy_id"`
}

func (h *CodeBuddyOAuthHandler) RefreshToken(c *gin.Context) {
	var req CodeBuddyRefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	refreshToken := strings.TrimSpace(req.RefreshToken)
	if refreshToken == "" {
		response.BadRequest(c, "refresh_token is required")
		return
	}
	var proxyURL string
	if req.ProxyID != nil {
		if proxy, err := h.adminService.GetProxy(c.Request.Context(), *req.ProxyID); err == nil && proxy != nil {
			proxyURL = proxy.URL()
		}
	}
	tokenInfo, err := h.codeBuddyOAuthService.RefreshToken(c.Request.Context(), refreshToken, proxyURL)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, tokenInfo)
}

func (h *CodeBuddyOAuthHandler) RefreshAccountToken(c *gin.Context) {
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
	if account.Platform != service.PlatformCodeBuddy {
		response.BadRequest(c, "Account platform does not match CodeBuddy OAuth endpoint")
		return
	}
	if !account.IsOAuth() {
		response.BadRequest(c, "Cannot refresh non-OAuth account credentials")
		return
	}
	tokenInfo, err := h.codeBuddyOAuthService.RefreshAccountToken(c.Request.Context(), account)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	newCredentials := h.codeBuddyOAuthService.BuildAccountCredentials(tokenInfo)
	newCredentials = service.MergeCredentials(account.Credentials, newCredentials)
	if baseURL := strings.TrimSpace(account.GetCredential("base_url")); baseURL != "" {
		newCredentials["base_url"] = baseURL
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

func (h *CodeBuddyOAuthHandler) CreateAccountFromOAuth(c *gin.Context) {
	var req struct {
		SessionID   string  `json:"session_id"`
		State       string  `json:"state"`
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
	tokenInfo, err := h.codeBuddyOAuthService.ExchangeState(c.Request.Context(), &service.CodeBuddyExchangeStateInput{
		SessionID: strings.TrimSpace(req.SessionID),
		State:     strings.TrimSpace(req.State),
		ProxyID:   req.ProxyID,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	credentials := h.codeBuddyOAuthService.BuildAccountCredentials(tokenInfo)

	name := strings.TrimSpace(req.Name)
	if name == "" && tokenInfo.Nickname != "" {
		name = tokenInfo.Nickname
	}
	if name == "" {
		name = "CodeBuddy OAuth Account"
	}

	account, err := h.adminService.CreateAccount(c.Request.Context(), &service.CreateAccountInput{
		Name:        name,
		Platform:    service.PlatformCodeBuddy,
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
	response.Success(c, dto.AccountFromService(account))
}

func (h *CodeBuddyOAuthHandler) RuntimeSanity(c *gin.Context) {
	response.Success(c, codebuddy.RuntimeSanity())
}
