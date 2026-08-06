package handler

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

const (
	web3BrowserCookieName   = "web3_auth_browser_session"
	web3BrowserCookiePath   = "/api/v1/auth/web3"
	web3BrowserCookieMaxAge = 10 * 60
)

type Web3AuthHandler struct {
	web3Service           *service.Web3AuthService
	authService           *service.AuthService
	totpService           *service.TotpService
	settingSvc            *service.SettingService
	browserCookieSameSite http.SameSite
}

func NewWeb3AuthHandler(
	cfg *config.Config,
	web3Service *service.Web3AuthService,
	authService *service.AuthService,
	totpService *service.TotpService,
	settingSvc *service.SettingService,
) *Web3AuthHandler {
	return &Web3AuthHandler{
		web3Service:           web3Service,
		authService:           authService,
		totpService:           totpService,
		settingSvc:            settingSvc,
		browserCookieSameSite: resolveWeb3BrowserCookieSameSite(cfg),
	}
}

type web3ChallengeRequest struct {
	Address        string `json:"address" binding:"required"`
	ChainID        string `json:"chain_id" binding:"required"`
	TurnstileToken string `json:"turnstile_token"`
}

type web3VerifyRequest struct {
	ChallengeToken string `json:"challenge_token" binding:"required"`
	Signature      string `json:"signature" binding:"required"`
}

type web3RegistrationVerifyRequest struct {
	web3VerifyRequest
	Username       string `json:"username"`
	InvitationCode string `json:"invitation_code"`
	PromoCode      string `json:"promo_code"`
	AffCode        string `json:"aff_code"`
}

func (h *Web3AuthHandler) CreateLoginChallenge(c *gin.Context) {
	h.createChallenge(c, false)
}

func (h *Web3AuthHandler) CreateRegistrationChallenge(c *gin.Context) {
	h.createChallenge(c, true)
}

func (h *Web3AuthHandler) createChallenge(c *gin.Context, registration bool) {
	var req web3ChallengeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if err := h.authService.VerifyTurnstile(c.Request.Context(), req.TurnstileToken, ip.GetClientIP(c)); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	browserSession, err := ensureWeb3BrowserSession(c, h.browserCookieSameSite)
	if err != nil {
		response.InternalError(c, "Failed to create browser session")
		return
	}
	input := service.Web3ChallengeInput{
		Address:        req.Address,
		ChainID:        req.ChainID,
		BrowserSession: browserSession,
	}
	var (
		result *service.Web3ChallengeResult
	)
	if registration {
		result, err = h.web3Service.CreateRegistrationChallenge(c.Request.Context(), input)
	} else {
		result, err = h.web3Service.CreateLoginChallenge(c.Request.Context(), input)
	}
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Web3AuthHandler) VerifyLogin(c *gin.Context) {
	var req web3VerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	result, err := h.web3Service.VerifyLogin(c.Request.Context(), service.Web3VerifyInput{
		ChallengeToken: req.ChallengeToken,
		Signature:      req.Signature,
		BrowserSession: readWeb3BrowserSession(c),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if h.settingSvc != nil && h.settingSvc.IsBackendModeEnabled(c.Request.Context()) && !result.User.IsAdmin() {
		response.ErrorFrom(c, infraerrors.Forbidden("BACKEND_MODE_ADMIN_ONLY", "Backend mode is active. Only admin login is allowed."))
		return
	}
	middleware2.SetAuditActor(c, result.User.ID, result.User.Email)
	c.Set("auth_method", service.AuditAuthMethodWeb3)
	if h.totpService != nil && h.settingSvc != nil && h.settingSvc.IsTotpEnabled(c.Request.Context()) && result.User.TotpEnabled {
		tempToken, err := h.totpService.CreateLoginSession(c.Request.Context(), result.User.ID, result.User.Email)
		if err != nil {
			response.InternalError(c, "Failed to create 2FA session")
			return
		}
		response.Success(c, TotpLoginResponse{
			Requires2FA:     true,
			TempToken:       tempToken,
			UserEmailMasked: maskWeb3Address(result.Address),
		})
		return
	}
	h.authService.RecordSuccessfulLogin(c.Request.Context(), result.User.ID)
	respondWithTokenPair(c, h.authService, result.User)
}

func (h *Web3AuthHandler) VerifyRegistration(c *gin.Context) {
	var req web3RegistrationVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	user, err := h.web3Service.VerifyRegistration(c.Request.Context(), service.Web3RegistrationVerifyInput{
		Web3VerifyInput: service.Web3VerifyInput{
			ChallengeToken: req.ChallengeToken,
			Signature:      req.Signature,
			BrowserSession: readWeb3BrowserSession(c),
		},
		Username:       req.Username,
		InvitationCode: req.InvitationCode,
		PromoCode:      req.PromoCode,
		AffiliateCode:  req.AffCode,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	middleware2.SetAuditActor(c, user.ID, user.Email)
	c.Set("auth_method", service.AuditAuthMethodWeb3)
	respondWithTokenPair(c, h.authService, user)
}

func maskWeb3Address(address string) string {
	address = strings.TrimSpace(address)
	if len(address) <= 12 {
		return address
	}
	return address[:6] + "..." + address[len(address)-4:]
}

func ensureWeb3BrowserSession(c *gin.Context, sameSite http.SameSite) (string, error) {
	if session := readWeb3BrowserSession(c); session != "" {
		return session, nil
	}
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	session := base64.RawURLEncoding.EncodeToString(buffer)
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     web3BrowserCookieName,
		Value:    session,
		Path:     web3BrowserCookiePath,
		MaxAge:   web3BrowserCookieMaxAge,
		HttpOnly: true,
		Secure:   isRequestHTTPS(c) || sameSite == http.SameSiteNoneMode,
		SameSite: sameSite,
	})
	return session, nil
}

func resolveWeb3BrowserCookieSameSite(cfg *config.Config) http.SameSite {
	if cfg != nil && strings.EqualFold(strings.TrimSpace(cfg.Web3Auth.BrowserCookieSameSite), config.Web3BrowserCookieSameSiteNone) {
		return http.SameSiteNoneMode
	}
	return http.SameSiteLaxMode
}

func readWeb3BrowserSession(c *gin.Context) string {
	cookie, err := c.Cookie(web3BrowserCookieName)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cookie)
}
