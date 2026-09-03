package handler

import (
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// UserOAuthHandler exposes OAuth auth-url / exchange endpoints for user-owned accounts.
// Business logic is delegated to the same OAuth services used by admin handlers.
type UserOAuthHandler struct {
	settingService          *service.SettingService
	oauthService            *service.OAuthService
	openaiOAuthService      *service.OpenAIOAuthService
	geminiOAuthService      *service.GeminiOAuthService
	antigravityOAuthService *service.AntigravityOAuthService
	grokOAuthService        *service.GrokOAuthService
	ownerStore              *service.OAuthSessionOwnerStore
}

// NewUserOAuthHandler creates a user OAuth handler with an in-memory session owner store.
func NewUserOAuthHandler(
	settingService *service.SettingService,
	oauthService *service.OAuthService,
	openaiOAuthService *service.OpenAIOAuthService,
	geminiOAuthService *service.GeminiOAuthService,
	antigravityOAuthService *service.AntigravityOAuthService,
	grokOAuthService *service.GrokOAuthService,
) *UserOAuthHandler {
	return &UserOAuthHandler{
		settingService:          settingService,
		oauthService:            oauthService,
		openaiOAuthService:      openaiOAuthService,
		geminiOAuthService:      geminiOAuthService,
		antigravityOAuthService: antigravityOAuthService,
		grokOAuthService:        grokOAuthService,
		ownerStore:              service.NewOAuthSessionOwnerStore(),
	}
}

// ---------- shared guards ----------

func (h *UserOAuthHandler) requireUserOwnedEnabled(c *gin.Context) bool {
	if h.settingService == nil || !h.settingService.IsUserOwnedAccountsEnabled(c.Request.Context()) {
		response.ErrorFrom(c, service.ErrUserOwnedAccountsDisabled)
		return false
	}
	return true
}

func (h *UserOAuthHandler) currentUserID(c *gin.Context) (int64, bool) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "User not authenticated")
		return 0, false
	}
	return subject.UserID, true
}

func (h *UserOAuthHandler) bindSession(sessionID string, userID int64) {
	if h.ownerStore != nil && sessionID != "" {
		h.ownerStore.Bind(sessionID, userID)
	}
}

func (h *UserOAuthHandler) assertSession(c *gin.Context, sessionID string, userID int64) bool {
	if h.ownerStore == nil {
		response.ErrorFrom(c, service.ErrOAuthSessionNotFound)
		return false
	}
	if err := h.ownerStore.Assert(sessionID, userID); err != nil {
		response.ErrorFrom(c, err)
		return false
	}
	return true
}

func (h *UserOAuthHandler) unbindSession(sessionID string) {
	if h.ownerStore != nil {
		h.ownerStore.Unbind(sessionID)
	}
}

// ========== Anthropic (Claude) OAuth ==========

type userGenerateAuthURLRequest struct {
	ProxyID *int64 `json:"proxy_id"`
}

// GenerateAuthURL POST /api/v1/user/accounts/oauth/generate-auth-url
func (h *UserOAuthHandler) GenerateAuthURL(c *gin.Context) {
	if !h.requireUserOwnedEnabled(c) {
		return
	}
	userID, ok := h.currentUserID(c)
	if !ok {
		return
	}
	var req userGenerateAuthURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = userGenerateAuthURLRequest{}
	}
	if err := service.RejectProxyID(req.ProxyID); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	result, err := h.oauthService.GenerateAuthURL(c.Request.Context(), nil)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.bindSession(result.SessionID, userID)
	response.Success(c, result)
}

// GenerateSetupTokenURL POST /api/v1/user/accounts/oauth/generate-setup-token-url
func (h *UserOAuthHandler) GenerateSetupTokenURL(c *gin.Context) {
	if !h.requireUserOwnedEnabled(c) {
		return
	}
	userID, ok := h.currentUserID(c)
	if !ok {
		return
	}
	var req userGenerateAuthURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = userGenerateAuthURLRequest{}
	}
	if err := service.RejectProxyID(req.ProxyID); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	result, err := h.oauthService.GenerateSetupTokenURL(c.Request.Context(), nil)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.bindSession(result.SessionID, userID)
	response.Success(c, result)
}

type userExchangeCodeRequest struct {
	SessionID string `json:"session_id" binding:"required"`
	Code      string `json:"code" binding:"required"`
	ProxyID   *int64 `json:"proxy_id"`
}

// ExchangeCode POST /api/v1/user/accounts/oauth/exchange-code
func (h *UserOAuthHandler) ExchangeCode(c *gin.Context) {
	h.exchangeAnthropicCode(c)
}

// ExchangeSetupTokenCode POST /api/v1/user/accounts/oauth/exchange-setup-token-code
func (h *UserOAuthHandler) ExchangeSetupTokenCode(c *gin.Context) {
	h.exchangeAnthropicCode(c)
}

func (h *UserOAuthHandler) exchangeAnthropicCode(c *gin.Context) {
	if !h.requireUserOwnedEnabled(c) {
		return
	}
	userID, ok := h.currentUserID(c)
	if !ok {
		return
	}
	var req userExchangeCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if err := service.RejectProxyID(req.ProxyID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if !h.assertSession(c, req.SessionID, userID) {
		return
	}

	tokenInfo, err := h.oauthService.ExchangeCode(c.Request.Context(), &service.ExchangeCodeInput{
		SessionID: req.SessionID,
		Code:      req.Code,
		ProxyID:   nil,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.unbindSession(req.SessionID)
	response.Success(c, tokenInfo)
}

type userCookieAuthRequest struct {
	SessionKey string `json:"code" binding:"required"`
	ProxyID    *int64 `json:"proxy_id"`
}

// CookieAuth POST /api/v1/user/accounts/oauth/cookie-auth
func (h *UserOAuthHandler) CookieAuth(c *gin.Context) {
	h.cookieAuth(c, "full")
}

// SetupTokenCookieAuth POST /api/v1/user/accounts/oauth/setup-token-cookie-auth
func (h *UserOAuthHandler) SetupTokenCookieAuth(c *gin.Context) {
	h.cookieAuth(c, "inference")
}

func (h *UserOAuthHandler) cookieAuth(c *gin.Context, scope string) {
	if !h.requireUserOwnedEnabled(c) {
		return
	}
	if _, ok := h.currentUserID(c); !ok {
		return
	}
	var req userCookieAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if err := service.RejectProxyID(req.ProxyID); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	tokenInfo, err := h.oauthService.CookieAuth(c.Request.Context(), &service.CookieAuthInput{
		SessionKey: req.SessionKey,
		ProxyID:    nil,
		Scope:      scope,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, tokenInfo)
}

// ========== OpenAI OAuth ==========

type userOpenAIGenerateAuthURLRequest struct {
	ProxyID     *int64 `json:"proxy_id"`
	RedirectURI string `json:"redirect_uri"`
}

// OpenAIGenerateAuthURL POST /api/v1/user/openai/generate-auth-url
func (h *UserOAuthHandler) OpenAIGenerateAuthURL(c *gin.Context) {
	if !h.requireUserOwnedEnabled(c) {
		return
	}
	userID, ok := h.currentUserID(c)
	if !ok {
		return
	}
	var req userOpenAIGenerateAuthURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = userOpenAIGenerateAuthURLRequest{}
	}
	if err := service.RejectProxyID(req.ProxyID); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	result, err := h.openaiOAuthService.GenerateAuthURL(
		c.Request.Context(),
		nil,
		req.RedirectURI,
		service.PlatformOpenAI,
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.bindSession(result.SessionID, userID)
	response.Success(c, result)
}

type userOpenAIExchangeCodeRequest struct {
	SessionID   string `json:"session_id" binding:"required"`
	Code        string `json:"code" binding:"required"`
	State       string `json:"state" binding:"required"`
	RedirectURI string `json:"redirect_uri"`
	ProxyID     *int64 `json:"proxy_id"`
}

// OpenAIExchangeCode POST /api/v1/user/openai/exchange-code
func (h *UserOAuthHandler) OpenAIExchangeCode(c *gin.Context) {
	if !h.requireUserOwnedEnabled(c) {
		return
	}
	userID, ok := h.currentUserID(c)
	if !ok {
		return
	}
	var req userOpenAIExchangeCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if err := service.RejectProxyID(req.ProxyID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if !h.assertSession(c, req.SessionID, userID) {
		return
	}

	tokenInfo, err := h.openaiOAuthService.ExchangeCode(c.Request.Context(), &service.OpenAIExchangeCodeInput{
		SessionID:   req.SessionID,
		Code:        req.Code,
		State:       req.State,
		RedirectURI: req.RedirectURI,
		ProxyID:     nil,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.unbindSession(req.SessionID)
	response.Success(c, tokenInfo)
}

type userOpenAIRefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token"`
	RT           string `json:"rt"`
	ClientID     string `json:"client_id"`
	ProxyID      *int64 `json:"proxy_id"`
}

// OpenAIRefreshToken POST /api/v1/user/openai/refresh-token
func (h *UserOAuthHandler) OpenAIRefreshToken(c *gin.Context) {
	if !h.requireUserOwnedEnabled(c) {
		return
	}
	if _, ok := h.currentUserID(c); !ok {
		return
	}
	var req userOpenAIRefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if err := service.RejectProxyID(req.ProxyID); err != nil {
		response.ErrorFrom(c, err)
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

	clientID := strings.TrimSpace(req.ClientID)
	if clientID == "" {
		clientID, _ = openai.OAuthClientConfigByPlatform(service.PlatformOpenAI)
	}

	tokenInfo, err := h.openaiOAuthService.RefreshTokenWithClientID(c.Request.Context(), refreshToken, "", clientID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, tokenInfo)
}

// ========== Gemini OAuth ==========

// GeminiGetCapabilities GET /api/v1/user/gemini/oauth/capabilities
func (h *UserOAuthHandler) GeminiGetCapabilities(c *gin.Context) {
	if !h.requireUserOwnedEnabled(c) {
		return
	}
	if _, ok := h.currentUserID(c); !ok {
		return
	}
	cfg := h.geminiOAuthService.GetOAuthConfig()
	response.Success(c, cfg)
}

type userGeminiGenerateAuthURLRequest struct {
	ProxyID   *int64 `json:"proxy_id"`
	ProjectID string `json:"project_id"`
	OAuthType string `json:"oauth_type"`
	TierID    string `json:"tier_id"`
}

// GeminiGenerateAuthURL POST /api/v1/user/gemini/oauth/auth-url
func (h *UserOAuthHandler) GeminiGenerateAuthURL(c *gin.Context) {
	if !h.requireUserOwnedEnabled(c) {
		return
	}
	userID, ok := h.currentUserID(c)
	if !ok {
		return
	}
	var req userGeminiGenerateAuthURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if err := service.RejectProxyID(req.ProxyID); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	oauthType := strings.TrimSpace(req.OAuthType)
	if oauthType == "" {
		oauthType = "code_assist"
	}
	if oauthType != "code_assist" && oauthType != "google_one" && oauthType != "ai_studio" {
		response.BadRequest(c, "Invalid oauth_type: must be 'code_assist', 'google_one', or 'ai_studio'")
		return
	}

	redirectURI := deriveUserGeminiRedirectURI(c)
	result, err := h.geminiOAuthService.GenerateAuthURL(c.Request.Context(), nil, redirectURI, req.ProjectID, oauthType, req.TierID)
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "OAuth client not configured") ||
			strings.Contains(msg, "requires your own OAuth Client") ||
			strings.Contains(msg, "requires a custom OAuth Client") ||
			strings.Contains(msg, "GEMINI_CLI_OAUTH_CLIENT_SECRET_MISSING") ||
			strings.Contains(msg, "built-in Gemini CLI OAuth client_secret is not configured") {
			response.BadRequest(c, "Failed to generate auth URL: "+msg)
			return
		}
		response.InternalError(c, "Failed to generate auth URL: "+msg)
		return
	}
	h.bindSession(result.SessionID, userID)
	response.Success(c, result)
}

type userGeminiExchangeCodeRequest struct {
	SessionID string `json:"session_id" binding:"required"`
	State     string `json:"state" binding:"required"`
	Code      string `json:"code" binding:"required"`
	ProxyID   *int64 `json:"proxy_id"`
	OAuthType string `json:"oauth_type"`
	TierID    string `json:"tier_id"`
}

// GeminiExchangeCode POST /api/v1/user/gemini/oauth/exchange-code
func (h *UserOAuthHandler) GeminiExchangeCode(c *gin.Context) {
	if !h.requireUserOwnedEnabled(c) {
		return
	}
	userID, ok := h.currentUserID(c)
	if !ok {
		return
	}
	var req userGeminiExchangeCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if err := service.RejectProxyID(req.ProxyID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if !h.assertSession(c, req.SessionID, userID) {
		return
	}

	oauthType := strings.TrimSpace(req.OAuthType)
	if oauthType == "" {
		oauthType = "code_assist"
	}
	if oauthType != "code_assist" && oauthType != "google_one" && oauthType != "ai_studio" {
		response.BadRequest(c, "Invalid oauth_type: must be 'code_assist', 'google_one', or 'ai_studio'")
		return
	}

	tokenInfo, err := h.geminiOAuthService.ExchangeCode(c.Request.Context(), &service.GeminiExchangeCodeInput{
		SessionID: req.SessionID,
		State:     req.State,
		Code:      req.Code,
		ProxyID:   nil,
		OAuthType: oauthType,
		TierID:    req.TierID,
	})
	if err != nil {
		response.BadRequest(c, "Failed to exchange code: "+err.Error())
		return
	}
	h.unbindSession(req.SessionID)
	response.Success(c, tokenInfo)
}

func deriveUserGeminiRedirectURI(c *gin.Context) string {
	origin := strings.TrimSpace(c.GetHeader("Origin"))
	if origin != "" {
		return strings.TrimRight(origin, "/") + "/auth/callback"
	}

	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	if xfProto := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")); xfProto != "" {
		scheme = strings.TrimSpace(strings.Split(xfProto, ",")[0])
	}

	host := strings.TrimSpace(c.Request.Host)
	if xfHost := strings.TrimSpace(c.GetHeader("X-Forwarded-Host")); xfHost != "" {
		host = strings.TrimSpace(strings.Split(xfHost, ",")[0])
	}

	return fmt.Sprintf("%s://%s/auth/callback", scheme, host)
}

// ========== Antigravity OAuth ==========

type userAntigravityGenerateAuthURLRequest struct {
	ProxyID *int64 `json:"proxy_id"`
}

// AntigravityGenerateAuthURL POST /api/v1/user/antigravity/oauth/auth-url
func (h *UserOAuthHandler) AntigravityGenerateAuthURL(c *gin.Context) {
	if !h.requireUserOwnedEnabled(c) {
		return
	}
	userID, ok := h.currentUserID(c)
	if !ok {
		return
	}
	var req userAntigravityGenerateAuthURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求无效: "+err.Error())
		return
	}
	if err := service.RejectProxyID(req.ProxyID); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	result, err := h.antigravityOAuthService.GenerateAuthURL(c.Request.Context(), nil)
	if err != nil {
		response.InternalError(c, "生成授权链接失败: "+err.Error())
		return
	}
	h.bindSession(result.SessionID, userID)
	response.Success(c, result)
}

type userAntigravityExchangeCodeRequest struct {
	SessionID string `json:"session_id" binding:"required"`
	State     string `json:"state" binding:"required"`
	Code      string `json:"code" binding:"required"`
	ProxyID   *int64 `json:"proxy_id"`
}

// AntigravityExchangeCode POST /api/v1/user/antigravity/oauth/exchange-code
func (h *UserOAuthHandler) AntigravityExchangeCode(c *gin.Context) {
	if !h.requireUserOwnedEnabled(c) {
		return
	}
	userID, ok := h.currentUserID(c)
	if !ok {
		return
	}
	var req userAntigravityExchangeCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求无效: "+err.Error())
		return
	}
	if err := service.RejectProxyID(req.ProxyID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if !h.assertSession(c, req.SessionID, userID) {
		return
	}

	tokenInfo, err := h.antigravityOAuthService.ExchangeCode(c.Request.Context(), &service.AntigravityExchangeCodeInput{
		SessionID: req.SessionID,
		State:     req.State,
		Code:      req.Code,
		ProxyID:   nil,
	})
	if err != nil {
		response.BadRequest(c, "Token 交换失败: "+err.Error())
		return
	}
	h.unbindSession(req.SessionID)
	response.Success(c, tokenInfo)
}

type userAntigravityRefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
	ProxyID      *int64 `json:"proxy_id"`
}

// AntigravityRefreshToken POST /api/v1/user/antigravity/oauth/refresh-token
func (h *UserOAuthHandler) AntigravityRefreshToken(c *gin.Context) {
	if !h.requireUserOwnedEnabled(c) {
		return
	}
	if _, ok := h.currentUserID(c); !ok {
		return
	}
	var req userAntigravityRefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求无效: "+err.Error())
		return
	}
	if err := service.RejectProxyID(req.ProxyID); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	tokenInfo, err := h.antigravityOAuthService.ValidateRefreshToken(c.Request.Context(), req.RefreshToken, nil)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, tokenInfo)
}

// ========== Grok OAuth ==========

type userGrokGenerateAuthURLRequest struct {
	ProxyID     *int64 `json:"proxy_id"`
	RedirectURI string `json:"redirect_uri"`
}

// GrokGenerateAuthURL POST /api/v1/user/grok/oauth/auth-url
func (h *UserOAuthHandler) GrokGenerateAuthURL(c *gin.Context) {
	if !h.requireUserOwnedEnabled(c) {
		return
	}
	userID, ok := h.currentUserID(c)
	if !ok {
		return
	}
	var req userGrokGenerateAuthURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = userGrokGenerateAuthURLRequest{}
	}
	if err := service.RejectProxyID(req.ProxyID); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	result, err := h.grokOAuthService.GenerateAuthURL(c.Request.Context(), nil, req.RedirectURI)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.bindSession(result.SessionID, userID)
	response.Success(c, result)
}

type userGrokExchangeCodeRequest struct {
	SessionID   string `json:"session_id" binding:"required"`
	Code        string `json:"code" binding:"required"`
	State       string `json:"state"`
	RedirectURI string `json:"redirect_uri"`
	ProxyID     *int64 `json:"proxy_id"`
}

// GrokExchangeCode POST /api/v1/user/grok/oauth/exchange-code
func (h *UserOAuthHandler) GrokExchangeCode(c *gin.Context) {
	if !h.requireUserOwnedEnabled(c) {
		return
	}
	userID, ok := h.currentUserID(c)
	if !ok {
		return
	}
	var req userGrokExchangeCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if err := service.RejectProxyID(req.ProxyID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if !h.assertSession(c, req.SessionID, userID) {
		return
	}

	tokenInfo, err := h.grokOAuthService.ExchangeCode(c.Request.Context(), &service.GrokExchangeCodeInput{
		SessionID:   req.SessionID,
		Code:        req.Code,
		State:       req.State,
		RedirectURI: req.RedirectURI,
		ProxyID:     nil,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.unbindSession(req.SessionID)
	response.Success(c, tokenInfo)
}

type userGrokRefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token"`
	RT           string `json:"rt"`
	ClientID     string `json:"client_id"`
	ProxyID      *int64 `json:"proxy_id"`
}

// GrokRefreshToken POST /api/v1/user/grok/oauth/refresh-token
func (h *UserOAuthHandler) GrokRefreshToken(c *gin.Context) {
	if !h.requireUserOwnedEnabled(c) {
		return
	}
	if _, ok := h.currentUserID(c); !ok {
		return
	}
	var req userGrokRefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if err := service.RejectProxyID(req.ProxyID); err != nil {
		response.ErrorFrom(c, err)
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

	tokenInfo, err := h.grokOAuthService.RefreshToken(c.Request.Context(), refreshToken, "", req.ClientID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, tokenInfo)
}
