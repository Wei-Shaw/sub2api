package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/oauth"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

const (
	weComOAuthCookiePath         = "/api/v1/auth/oauth/wecom"
	weComOAuthCookieMaxAgeSec    = 10 * 60
	weComOAuthStateCookieName    = "wecom_oauth_state"
	weComOAuthRedirectCookieName = "wecom_oauth_redirect"
	weComOAuthIntentCookieName   = "wecom_oauth_intent"
	weComOAuthModeCookieName     = "wecom_oauth_mode"
	weComOAuthBindUserCookieName = "wecom_oauth_bind_user"
	weComOAuthDefaultRedirectTo  = "/dashboard"
	weComOAuthDefaultFrontendCB  = "/auth/wecom/callback"
	weComOAuthProviderKey        = "wecom-main"
	weComDevTrustEmailEnv        = "SUB2API_DEV_WECOM_TRUST_ENTERED_EMAIL"
)

var (
	weComOAuthWebviewAuthorizeURL = "https://open.weixin.qq.com/connect/oauth2/authorize"
	weComOAuthWebAuthorizeURL     = "https://login.work.weixin.qq.com/wwlogin/sso/login"
	weComOAuthGetTokenURL         = "https://qyapi.weixin.qq.com/cgi-bin/gettoken"
	weComOAuthGetUserInfoURL      = "https://qyapi.weixin.qq.com/cgi-bin/auth/getuserinfo"
	weComOAuthGetUserDetailURL    = "https://qyapi.weixin.qq.com/cgi-bin/auth/getuserdetail"
	weComOAuthGetUserURL          = "https://qyapi.weixin.qq.com/cgi-bin/user/get"

	weComTokenMu    sync.Mutex
	weComTokenCache = map[string]cachedWeComToken{}
)

type cachedWeComToken struct {
	accessToken string
	expiresAt   time.Time
}

type weComOAuthConfig struct {
	corpID           string
	agentID          string
	secret           string
	scope            string
	redirectURI      string
	frontendCallback string
}

type weComResolvedOAuthIdentity struct {
	userID          string
	providerSubject string
	email           string
	username        string
	emailSource     string
	identityRef     service.PendingAuthIdentityKey
	upstreamClaims  map[string]any
}

type weComAccessTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
	ErrCode     int64  `json:"errcode"`
	ErrMsg      string `json:"errmsg"`
}

type weComUserInfoResponse struct {
	UserID         string `json:"userid"`
	OpenID         string `json:"openid"`
	ExternalUserID string `json:"external_userid"`
	UserTicket     string `json:"user_ticket"`
	ErrCode        int64  `json:"errcode"`
	ErrMsg         string `json:"errmsg"`
}

type weComUserDetailResponse struct {
	UserID  string `json:"userid"`
	Name    string `json:"name"`
	Avatar  string `json:"avatar"`
	Email   string `json:"email"`
	BizMail string `json:"biz_mail"`
	Mobile  string `json:"mobile"`
	Gender  string `json:"gender"`
	QRCode  string `json:"qr_code"`
	ErrCode int64  `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

func (r weComUserInfoResponse) normalizedUserID() string {
	return strings.TrimSpace(r.UserID)
}

func (r *weComUserInfoResponse) UnmarshalJSON(data []byte) error {
	type alias weComUserInfoResponse
	var raw struct {
		alias
		UserIDCompat string `json:"UserId"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*r = weComUserInfoResponse(raw.alias)
	if r.UserID == "" {
		r.UserID = raw.UserIDCompat
	}
	return nil
}

func (h *AuthHandler) WeComOAuthStart(c *gin.Context) {
	cfg, err := h.getWeComOAuthConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	state, err := oauth.GenerateState()
	if err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_STATE_GEN_FAILED", "failed to generate oauth state").WithCause(err))
		return
	}
	browserSessionKey, err := generateOAuthPendingBrowserSession()
	if err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_BROWSER_SESSION_GEN_FAILED", "failed to generate oauth browser session").WithCause(err))
		return
	}

	redirectTo := sanitizeFrontendRedirectPath(c.Query("redirect"))
	if redirectTo == "" {
		redirectTo = weComOAuthDefaultRedirectTo
	}
	mode := normalizeWeComOAuthMode(c.Query("mode"), c.Request.UserAgent())
	intent := normalizeWeChatOAuthIntent(c.Query("intent"))
	secureCookie := isRequestHTTPS(c)
	weComSetCookie(c, weComOAuthStateCookieName, encodeCookieValue(state), weComOAuthCookieMaxAgeSec, secureCookie)
	weComSetCookie(c, weComOAuthRedirectCookieName, encodeCookieValue(redirectTo), weComOAuthCookieMaxAgeSec, secureCookie)
	weComSetCookie(c, weComOAuthIntentCookieName, encodeCookieValue(intent), weComOAuthCookieMaxAgeSec, secureCookie)
	weComSetCookie(c, weComOAuthModeCookieName, encodeCookieValue(mode), weComOAuthCookieMaxAgeSec, secureCookie)
	setOAuthPendingBrowserCookie(c, browserSessionKey, secureCookie)
	clearOAuthPendingSessionCookie(c, secureCookie)
	if intent == oauthIntentBindCurrentUser {
		bindCookieValue, err := h.buildOAuthBindUserCookieFromContext(c)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		weComSetCookie(c, weComOAuthBindUserCookieName, encodeCookieValue(bindCookieValue), weComOAuthCookieMaxAgeSec, secureCookie)
	} else {
		weComClearCookie(c, weComOAuthBindUserCookieName, secureCookie)
	}

	authURL, err := buildWeComAuthorizeURL(cfg, mode, state)
	if err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_BUILD_URL_FAILED", "failed to build oauth authorization url").WithCause(err))
		return
	}
	c.Redirect(http.StatusFound, authURL)
}

func (h *AuthHandler) WeComOAuthCallback(c *gin.Context) {
	frontendCallback := h.weComOAuthFrontendCallback(c.Request.Context())
	if providerErr := strings.TrimSpace(c.Query("error")); providerErr != "" {
		redirectOAuthError(c, frontendCallback, "provider_error", providerErr, c.Query("error_description"))
		return
	}
	code := strings.TrimSpace(c.Query("code"))
	state := strings.TrimSpace(c.Query("state"))
	if code == "" || state == "" {
		redirectOAuthError(c, frontendCallback, "missing_params", "missing code/state", "")
		return
	}

	secureCookie := isRequestHTTPS(c)
	defer func() {
		weComClearCookie(c, weComOAuthStateCookieName, secureCookie)
		weComClearCookie(c, weComOAuthRedirectCookieName, secureCookie)
		weComClearCookie(c, weComOAuthIntentCookieName, secureCookie)
		weComClearCookie(c, weComOAuthModeCookieName, secureCookie)
		weComClearCookie(c, weComOAuthBindUserCookieName, secureCookie)
	}()
	expectedState, err := readCookieDecoded(c, weComOAuthStateCookieName)
	if err != nil || expectedState == "" || state != expectedState {
		redirectOAuthError(c, frontendCallback, "invalid_state", "invalid oauth state", "")
		return
	}
	redirectTo, _ := readCookieDecoded(c, weComOAuthRedirectCookieName)
	redirectTo = sanitizeFrontendRedirectPath(redirectTo)
	if redirectTo == "" {
		redirectTo = weComOAuthDefaultRedirectTo
	}
	browserSessionKey, _ := readOAuthPendingBrowserCookie(c)
	if strings.TrimSpace(browserSessionKey) == "" {
		redirectOAuthError(c, frontendCallback, "missing_browser_session", "missing oauth browser session", "")
		return
	}
	intent, _ := readCookieDecoded(c, weComOAuthIntentCookieName)
	mode, _ := readCookieDecoded(c, weComOAuthModeCookieName)

	cfg, err := h.getWeComOAuthConfig(c.Request.Context())
	if err != nil {
		redirectOAuthError(c, frontendCallback, "provider_error", infraerrors.Reason(err), infraerrors.Message(err))
		return
	}
	userInfo, err := fetchWeComOAuthIdentity(c.Request.Context(), cfg, code)
	if err != nil {
		redirectOAuthError(c, frontendCallback, "provider_error", "wecom_identity_fetch_failed", singleLine(err.Error()))
		return
	}
	userID := userInfo.normalizedUserID()
	if userID == "" {
		redirectOAuthError(c, frontendCallback, "provider_error", "wecom_member_required", "")
		return
	}

	resolved := h.resolveWeComOAuthIdentity(c.Request.Context(), cfg, userInfo, strings.TrimSpace(mode))
	normalizedIntent := normalizeWeChatOAuthIntent(intent)
	if normalizedIntent == wechatOAuthIntentBind {
		currentUser, err := h.readOAuthBindTargetUser(c, weComOAuthBindUserCookieName)
		if err != nil {
			redirectOAuthError(c, frontendCallback, "auth_required", infraerrors.Reason(err), infraerrors.Message(err))
			return
		}
		if err := h.ensureGenericBindOwnership(c.Request.Context(), currentUser.ID, resolved.identityRef); err != nil {
			redirectOAuthError(c, frontendCallback, "ownership_conflict", infraerrors.Reason(err), infraerrors.Message(err))
			return
		}
		if err := h.createOAuthPendingSession(c, oauthPendingSessionPayload{
			Intent:                 wechatOAuthIntentBind,
			Identity:               resolved.identityRef,
			TargetUserID:           &currentUser.ID,
			ResolvedEmail:          currentUser.Email,
			RedirectTo:             redirectTo,
			BrowserSessionKey:      browserSessionKey,
			UpstreamIdentityClaims: resolved.upstreamClaims,
			CompletionResponse:     map[string]any{"redirect": redirectTo},
		}); err != nil {
			redirectOAuthError(c, frontendCallback, "session_error", infraerrors.Reason(err), infraerrors.Message(err))
			return
		}
		redirectToFrontendCallback(c, frontendCallback)
		return
	}

	existingIdentityUser, err := h.findOAuthIdentityUser(c.Request.Context(), resolved.identityRef)
	if err != nil {
		redirectOAuthError(c, frontendCallback, "session_error", infraerrors.Reason(err), infraerrors.Message(err))
		return
	}
	if existingIdentityUser != nil {
		h.redirectWeComBoundIdentityLogin(c, frontendCallback, redirectTo, entUserToService(existingIdentityUser))
		return
	}
	if strings.EqualFold(strings.TrimSpace(mode), "web") {
		redirectWeComMobileRequired(c, frontendCallback, redirectTo)
		return
	}

	if resolved.emailSource == "synthetic" {
		if err := h.createOAuthPendingSession(c, oauthPendingSessionPayload{
			Intent:                 oauthIntentLogin,
			Identity:               resolved.identityRef,
			ResolvedEmail:          "",
			RedirectTo:             redirectTo,
			BrowserSessionKey:      browserSessionKey,
			UpstreamIdentityClaims: resolved.upstreamClaims,
			CompletionResponse:     buildWeComEmailRequiredResponse(redirectTo),
		}); err != nil {
			redirectOAuthError(c, frontendCallback, "session_error", infraerrors.Reason(err), infraerrors.Message(err))
			return
		}
		redirectToFrontendCallback(c, frontendCallback)
		return
	}

	tokenPair, user, authErr := h.authService.LoginOrRegisterVerifiedEmailOAuth(c.Request.Context(), service.EmailOAuthIdentityInput{
		ProviderType:     "wecom",
		ProviderKey:      weComOAuthProviderKey,
		ProviderSubject:  resolved.providerSubject,
		Email:            resolved.email,
		EmailVerified:    true,
		Username:         resolved.username,
		AvatarURL:        weComResolvedAvatarURL(resolved),
		UpstreamMetadata: resolved.upstreamClaims,
	})
	var targetUserID *int64
	if user != nil && user.ID > 0 {
		if authErr == nil {
			if err := h.applyWeComResolvedAvatar(c.Request.Context(), user.ID, resolved); err != nil {
				redirectOAuthError(c, frontendCallback, "session_error", infraerrors.Reason(err), infraerrors.Message(err))
				return
			}
		}
		targetUserID = &user.ID
	}
	if err := h.createOAuthPendingSession(c, oauthPendingSessionPayload{
		Intent:                 oauthIntentLogin,
		Identity:               resolved.identityRef,
		TargetUserID:           targetUserID,
		ResolvedEmail:          resolved.email,
		RedirectTo:             redirectTo,
		BrowserSessionKey:      browserSessionKey,
		UpstreamIdentityClaims: resolved.upstreamClaims,
		CompletionResponse:     buildWeComCompletionResponse(redirectTo, tokenPair, authErr),
	}); err != nil {
		redirectOAuthError(c, frontendCallback, "session_error", infraerrors.Reason(err), infraerrors.Message(err))
		return
	}
	redirectToFrontendCallback(c, frontendCallback)
}

func (h *AuthHandler) CompleteWeComOAuthRegistration(c *gin.Context) {
	var req completeWeChatOAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "INVALID_REQUEST", "message": err.Error()})
		return
	}

	secureCookie := isRequestHTTPS(c)
	sessionToken, err := readOAuthPendingSessionCookie(c)
	if err != nil {
		clearOAuthPendingSessionCookie(c, secureCookie)
		clearOAuthPendingBrowserCookie(c, secureCookie)
		response.ErrorFrom(c, service.ErrPendingAuthSessionNotFound)
		return
	}
	browserSessionKey, err := readOAuthPendingBrowserCookie(c)
	if err != nil {
		clearOAuthPendingSessionCookie(c, secureCookie)
		clearOAuthPendingBrowserCookie(c, secureCookie)
		response.ErrorFrom(c, service.ErrPendingAuthBrowserMismatch)
		return
	}
	pendingSvc, err := h.pendingIdentityService()
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	session, err := pendingSvc.GetBrowserSession(c.Request.Context(), sessionToken, browserSessionKey)
	if err != nil {
		clearOAuthPendingSessionCookie(c, secureCookie)
		clearOAuthPendingBrowserCookie(c, secureCookie)
		response.ErrorFrom(c, err)
		return
	}
	if err := ensurePendingOAuthCompleteRegistrationSession(session); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if updatedSession, handled, err := h.legacyCompleteRegistrationSessionStatus(c, session); err != nil {
		response.ErrorFrom(c, err)
		return
	} else if handled {
		c.JSON(http.StatusOK, buildPendingOAuthSessionStatusPayload(updatedSession))
		return
	} else {
		session = updatedSession
	}
	if !strings.EqualFold(strings.TrimSpace(session.ProviderType), "wecom") {
		response.ErrorFrom(c, infraerrors.BadRequest("PENDING_AUTH_PROVIDER_MISMATCH", "pending oauth session provider mismatch"))
		return
	}
	if err := h.ensureBackendModeAllowsNewUserLogin(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	email := strings.TrimSpace(session.ResolvedEmail)
	username := pendingSessionStringValue(session.UpstreamIdentityClaims, "username")
	providerSubject := strings.TrimSpace(session.ProviderSubject)
	if email == "" || username == "" || providerSubject == "" {
		response.ErrorFrom(c, infraerrors.BadRequest("PENDING_AUTH_SESSION_INVALID", "pending auth registration context is invalid"))
		return
	}

	input := service.EmailOAuthIdentityInput{
		ProviderType:     "wecom",
		ProviderKey:      firstNonEmptyString(session.ProviderKey, weComOAuthProviderKey),
		ProviderSubject:  providerSubject,
		Email:            email,
		EmailVerified:    true,
		Username:         username,
		AvatarURL:        pendingSessionStringValue(session.UpstreamIdentityClaims, "suggested_avatar_url"),
		UpstreamMetadata: session.UpstreamIdentityClaims,
	}
	tokenPair, user, err := h.authService.LoginOrRegisterVerifiedEmailOAuthWithInvitation(
		c.Request.Context(),
		input,
		req.InvitationCode,
		req.AffCode,
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	decision, err := h.ensurePendingOAuthAdoptionDecision(c, session.ID, oauthAdoptionDecisionRequest{
		AdoptDisplayName: req.AdoptDisplayName,
		AdoptAvatar:      req.AdoptAvatar,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if err := applyPendingOAuthAdoption(c.Request.Context(), h.entClient(), h.authService, h.userService, session, decision, &user.ID); err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("PENDING_AUTH_ADOPTION_APPLY_FAILED", "failed to apply oauth profile adoption").WithCause(err))
		return
	}
	h.authService.RecordSuccessfulLogin(c.Request.Context(), user.ID)
	if _, err := pendingSvc.ConsumeBrowserSession(c.Request.Context(), sessionToken, browserSessionKey); err != nil {
		clearOAuthPendingSessionCookie(c, secureCookie)
		clearOAuthPendingBrowserCookie(c, secureCookie)
		response.ErrorFrom(c, err)
		return
	}
	clearOAuthPendingSessionCookie(c, secureCookie)
	clearOAuthPendingBrowserCookie(c, secureCookie)

	c.JSON(http.StatusOK, gin.H{
		"access_token":  tokenPair.AccessToken,
		"refresh_token": tokenPair.RefreshToken,
		"expires_in":    tokenPair.ExpiresIn,
		"token_type":    "Bearer",
	})
}
func (h *AuthHandler) BindWeComOAuthLogin(c *gin.Context) { h.bindPendingOAuthLogin(c, "wecom") }
func (h *AuthHandler) CreateWeComOAuthAccount(c *gin.Context) {
	h.createPendingOAuthAccountWithOptions(c, "wecom", createPendingOAuthAccountOptions{
		TrustEnteredEmail: h.allowDevWeComEnteredEmailTrust(),
	})
}

func (h *AuthHandler) allowDevWeComEnteredEmailTrust() bool {
	if h == nil || h.cfg == nil || h.cfg.RunMode != config.RunModeSimple {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(os.Getenv(weComDevTrustEmailEnv)), "true")
}

func (h *AuthHandler) getWeComOAuthConfig(ctx context.Context) (weComOAuthConfig, error) {
	if h == nil || h.settingSvc == nil {
		return weComOAuthConfig{}, infraerrors.ServiceUnavailable("OAUTH_CONFIG_UNAVAILABLE", "wecom oauth settings are unavailable")
	}
	settings, err := h.settingSvc.GetAllSettings(ctx)
	if err != nil {
		return weComOAuthConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_LOAD_FAILED", "failed to load wecom oauth settings").WithCause(err)
	}
	cfg := weComOAuthConfig{
		corpID:           strings.TrimSpace(settings.WeComOAuthCorpID),
		agentID:          strings.TrimSpace(settings.WeComOAuthAgentID),
		secret:           strings.TrimSpace(settings.WeComOAuthSecret),
		scope:            strings.TrimSpace(settings.WeComOAuthScope),
		redirectURI:      strings.TrimSpace(settings.WeComOAuthRedirectURL),
		frontendCallback: strings.TrimSpace(settings.WeComOAuthFrontendRedirectURL),
	}
	if cfg.scope == "" {
		cfg.scope = "snsapi_base"
	}
	if cfg.frontendCallback == "" {
		cfg.frontendCallback = weComOAuthDefaultFrontendCB
	}
	if !settings.WeComOAuthEnabled {
		return weComOAuthConfig{}, infraerrors.NotFound("OAUTH_DISABLED", "wecom oauth is disabled")
	}
	if cfg.corpID == "" || cfg.agentID == "" || cfg.secret == "" {
		return weComOAuthConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "wecom oauth corp id, agent id, or secret not configured")
	}
	if cfg.redirectURI == "" {
		return weComOAuthConfig{}, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "wecom oauth redirect url not configured")
	}
	return cfg, nil
}

func (h *AuthHandler) weComOAuthFrontendCallback(ctx context.Context) string {
	cfg, err := h.getWeComOAuthConfig(ctx)
	if err != nil || strings.TrimSpace(cfg.frontendCallback) == "" {
		return weComOAuthDefaultFrontendCB
	}
	return cfg.frontendCallback
}

func buildWeComAuthorizeURL(cfg weComOAuthConfig, mode, state string) (string, error) {
	if normalizeWeComOAuthMode(mode, "") == "webview" {
		u, err := url.Parse(weComOAuthWebviewAuthorizeURL)
		if err != nil {
			return "", err
		}
		q := u.Query()
		q.Set("appid", cfg.corpID)
		q.Set("redirect_uri", cfg.redirectURI)
		q.Set("response_type", "code")
		q.Set("scope", cfg.scope)
		q.Set("state", state)
		q.Set("agentid", cfg.agentID)
		u.RawQuery = q.Encode()
		u.Fragment = "wechat_redirect"
		return u.String(), nil
	}
	u, err := url.Parse(weComOAuthWebAuthorizeURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("login_type", "CorpApp")
	q.Set("appid", cfg.corpID)
	q.Set("agentid", cfg.agentID)
	q.Set("redirect_uri", cfg.redirectURI)
	q.Set("state", state)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (h *AuthHandler) resolveWeComOAuthIdentity(ctx context.Context, cfg weComOAuthConfig, userInfo weComUserInfoResponse, mode string) weComResolvedOAuthIdentity {
	userID := userInfo.normalizedUserID()
	providerSubject := cfg.corpID + "/" + userID
	email := weComSyntheticEmail(providerSubject)
	username := weComFallbackUsername(userID, providerSubject)
	upstreamClaims := map[string]any{
		"email":            email,
		"username":         username,
		"subject":          providerSubject,
		"corpid":           cfg.corpID,
		"agentid":          cfg.agentID,
		"userid":           userID,
		"openid":           strings.TrimSpace(userInfo.OpenID),
		"external_userid":  strings.TrimSpace(userInfo.ExternalUserID),
		"user_ticket":      strings.TrimSpace(userInfo.UserTicket),
		"mode":             strings.TrimSpace(mode),
		"suggested_source": "wecom",
	}
	username = enrichWeComProfileClaims(ctx, cfg, userInfo, username, upstreamClaims)
	email = selectWeComRegistrationEmail(ctx, h.entClient(), email, upstreamClaims)
	upstreamClaims["email"] = email
	return weComResolvedOAuthIdentity{
		userID:          userID,
		providerSubject: providerSubject,
		email:           email,
		username:        username,
		emailSource:     pendingSessionStringValue(upstreamClaims, "wecom_registration_email_source"),
		identityRef: service.PendingAuthIdentityKey{
			ProviderType:    "wecom",
			ProviderKey:     weComOAuthProviderKey,
			ProviderSubject: providerSubject,
		},
		upstreamClaims: upstreamClaims,
	}
}

func enrichWeComProfileClaims(ctx context.Context, cfg weComOAuthConfig, userInfo weComUserInfoResponse, username string, upstreamClaims map[string]any) string {
	detail, err := maybeFetchWeComUserDetail(ctx, cfg, userInfo)
	if err != nil {
		upstreamClaims["wecom_profile_detail_error"] = singleLine(err.Error())
		return username
	}
	if detail == nil {
		return username
	}
	username = applyWeComProfileDetailClaims(username, detail, upstreamClaims)
	return supplementWeComDirectoryProfile(ctx, cfg, userInfo, username, detail, upstreamClaims)
}

func supplementWeComDirectoryProfile(ctx context.Context, cfg weComOAuthConfig, userInfo weComUserInfoResponse, username string, detail *weComUserDetailResponse, claims map[string]any) string {
	if !isWeComPrivateInfoDetailFlow(cfg, userInfo) {
		return username
	}
	if !weComProfileNeedsDirectorySupplement(detail) {
		return username
	}
	directoryDetail, err := fetchWeComDirectoryUserDetail(ctx, cfg, userInfo)
	if err != nil {
		claims["wecom_directory_profile_error"] = singleLine(err.Error())
		return username
	}
	if directoryDetail == nil {
		return username
	}
	return applyWeComProfileDetailClaims(username, directoryDetail, claims)
}

func isWeComPrivateInfoDetailFlow(cfg weComOAuthConfig, userInfo weComUserInfoResponse) bool {
	return strings.TrimSpace(cfg.scope) == "snsapi_privateinfo" && strings.TrimSpace(userInfo.UserTicket) != ""
}

func weComProfileNeedsDirectorySupplement(detail *weComUserDetailResponse) bool {
	if detail == nil {
		return false
	}
	return strings.TrimSpace(detail.Name) == "" || strings.TrimSpace(detail.Avatar) == ""
}

func applyWeComProfileDetailClaims(username string, detail *weComUserDetailResponse, claims map[string]any) string {
	username = applyWeComDisplayNameClaim(username, detail.Name, claims)
	applyWeComAvatarClaim(detail.Avatar, claims)
	applyWeComStringClaim(claims, "wecom_email", detail.Email)
	applyWeComStringClaim(claims, "wecom_biz_mail", detail.BizMail)
	applyWeComStringClaim(claims, "wecom_mobile", detail.Mobile)
	applyWeComStringClaim(claims, "wecom_gender", detail.Gender)
	applyWeComStringClaim(claims, "wecom_qr_code", detail.QRCode)
	applyWeComStringClaim(claims, "wecom_detail_userid", detail.UserID)
	return username
}

func applyWeComDisplayNameClaim(username string, name string, claims map[string]any) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return username
	}
	claims["username"] = name
	claims["name"] = name
	claims["suggested_display_name"] = name
	return name
}

func applyWeComAvatarClaim(avatar string, claims map[string]any) {
	avatar = strings.TrimSpace(avatar)
	if avatar == "" {
		return
	}
	claims["avatar"] = avatar
	claims["avatar_url"] = avatar
	claims["suggested_avatar_url"] = avatar
}

func weComResolvedAvatarURL(resolved weComResolvedOAuthIdentity) string {
	return pendingSessionStringValue(resolved.upstreamClaims, "suggested_avatar_url")
}

func (h *AuthHandler) applyWeComResolvedAvatar(ctx context.Context, userID int64, resolved weComResolvedOAuthIdentity) error {
	avatarURL := weComResolvedAvatarURL(resolved)
	if avatarURL == "" || userID <= 0 {
		return nil
	}
	if h == nil || h.userService == nil {
		return infraerrors.ServiceUnavailable("USER_SERVICE_NOT_READY", "user service is not ready")
	}
	if err := service.ValidateUserAvatar(avatarURL); err != nil {
		return err
	}
	if _, err := h.userService.SetAvatar(ctx, userID, avatarURL); err != nil {
		return infraerrors.InternalServer("WECOM_AVATAR_APPLY_FAILED", "failed to apply wecom avatar").WithCause(err)
	}
	return nil
}

func applyWeComStringClaim(claims map[string]any, key string, value string) {
	value = strings.TrimSpace(value)
	if value == "" || pendingSessionStringValue(claims, key) != "" {
		return
	}
	claims[key] = value
}

func maybeFetchWeComUserDetail(ctx context.Context, cfg weComOAuthConfig, userInfo weComUserInfoResponse) (*weComUserDetailResponse, error) {
	if strings.TrimSpace(cfg.scope) != "snsapi_privateinfo" {
		return fetchWeComDirectoryUserDetail(ctx, cfg, userInfo)
	}
	userTicket := strings.TrimSpace(userInfo.UserTicket)
	if userTicket != "" {
		detail, err := fetchWeComUserDetail(ctx, cfg, userTicket)
		if err != nil {
			return nil, err
		}
		return &detail, nil
	}
	return fetchWeComDirectoryUserDetail(ctx, cfg, userInfo)
}

func fetchWeComDirectoryUserDetail(ctx context.Context, cfg weComOAuthConfig, userInfo weComUserInfoResponse) (*weComUserDetailResponse, error) {
	userID := userInfo.normalizedUserID()
	if userID == "" {
		return nil, nil
	}
	detail, err := fetchWeComUser(ctx, cfg, userID)
	if err != nil {
		return nil, err
	}
	return &detail, nil
}

func fetchWeComOAuthIdentity(ctx context.Context, cfg weComOAuthConfig, code string) (weComUserInfoResponse, error) {
	accessToken, err := fetchWeComAccessToken(ctx, cfg)
	if err != nil {
		return weComUserInfoResponse{}, err
	}
	u, err := url.Parse(weComOAuthGetUserInfoURL)
	if err != nil {
		return weComUserInfoResponse{}, err
	}
	q := u.Query()
	q.Set("access_token", accessToken)
	q.Set("code", code)
	u.RawQuery = q.Encode()
	var result weComUserInfoResponse
	if err := getJSON(ctx, u.String(), &result); err != nil {
		return result, err
	}
	if result.ErrCode != 0 {
		return result, fmt.Errorf("wecom getuserinfo failed: %d %s", result.ErrCode, result.ErrMsg)
	}
	return result, nil
}

func fetchWeComUserDetail(ctx context.Context, cfg weComOAuthConfig, userTicket string) (weComUserDetailResponse, error) {
	accessToken, err := fetchWeComAccessToken(ctx, cfg)
	if err != nil {
		return weComUserDetailResponse{}, err
	}
	u, err := url.Parse(weComOAuthGetUserDetailURL)
	if err != nil {
		return weComUserDetailResponse{}, err
	}
	q := u.Query()
	q.Set("access_token", accessToken)
	u.RawQuery = q.Encode()
	var result weComUserDetailResponse
	if err := postJSON(ctx, u.String(), map[string]string{"user_ticket": strings.TrimSpace(userTicket)}, &result); err != nil {
		return result, err
	}
	if result.ErrCode != 0 {
		return result, fmt.Errorf("wecom getuserdetail failed: %d %s", result.ErrCode, result.ErrMsg)
	}
	return result, nil
}

func fetchWeComUser(ctx context.Context, cfg weComOAuthConfig, userID string) (weComUserDetailResponse, error) {
	accessToken, err := fetchWeComAccessToken(ctx, cfg)
	if err != nil {
		return weComUserDetailResponse{}, err
	}
	u, err := url.Parse(weComOAuthGetUserURL)
	if err != nil {
		return weComUserDetailResponse{}, err
	}
	q := u.Query()
	q.Set("access_token", accessToken)
	q.Set("userid", strings.TrimSpace(userID))
	u.RawQuery = q.Encode()
	var result weComUserDetailResponse
	if err := getJSON(ctx, u.String(), &result); err != nil {
		return result, err
	}
	if result.ErrCode != 0 {
		return result, fmt.Errorf("wecom user get failed: %d %s", result.ErrCode, result.ErrMsg)
	}
	return result, nil
}

func fetchWeComAccessToken(ctx context.Context, cfg weComOAuthConfig) (string, error) {
	cacheKey := cfg.corpID + "\x00" + cfg.secret
	now := time.Now()
	weComTokenMu.Lock()
	if cached := weComTokenCache[cacheKey]; cached.accessToken != "" && now.Before(cached.expiresAt) {
		token := cached.accessToken
		weComTokenMu.Unlock()
		return token, nil
	}
	weComTokenMu.Unlock()

	u, err := url.Parse(weComOAuthGetTokenURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("corpid", cfg.corpID)
	q.Set("corpsecret", cfg.secret)
	u.RawQuery = q.Encode()
	var result weComAccessTokenResponse
	if err := getJSON(ctx, u.String(), &result); err != nil {
		return "", err
	}
	if result.ErrCode != 0 || strings.TrimSpace(result.AccessToken) == "" {
		return "", fmt.Errorf("wecom gettoken failed: %d %s", result.ErrCode, result.ErrMsg)
	}
	ttl := time.Duration(result.ExpiresIn-120) * time.Second
	if ttl < time.Minute {
		ttl = time.Minute
	}
	weComTokenMu.Lock()
	weComTokenCache[cacheKey] = cachedWeComToken{accessToken: result.AccessToken, expiresAt: now.Add(ttl)}
	weComTokenMu.Unlock()
	return result.AccessToken, nil
}

func getJSON(ctx context.Context, rawURL string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("provider returned status %d", resp.StatusCode)
	}
	return json.Unmarshal(body, target)
}

func postJSON(ctx context.Context, rawURL string, payload any, target any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("provider returned status %d", resp.StatusCode)
	}
	return json.Unmarshal(respBody, target)
}

func normalizeWeComOAuthMode(raw, userAgent string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "webview":
		return "webview"
	case "web":
		return "web"
	default:
		ua := strings.ToLower(userAgent)
		if strings.Contains(ua, "wxwork") {
			return "webview"
		}
		return "web"
	}
}

func weComSyntheticEmail(subject string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(subject)))
	return hex.EncodeToString(sum[:]) + service.WeComConnectSyntheticEmailDomain
}

func weComFallbackUsername(userID, subject string) string {
	if trimmed := strings.TrimSpace(userID); trimmed != "" {
		return trimmed
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(subject)))
	return "wecom_" + hex.EncodeToString(sum[:])[:12]
}

func buildWeComCompletionResponse(redirectTo string, tokenPair *service.TokenPair, authErr error) map[string]any {
	completionResponse := map[string]any{"redirect": redirectTo}
	if authErr != nil {
		if errors.Is(authErr, service.ErrOAuthInvitationRequired) {
			completionResponse["error"] = "invitation_required"
			return completionResponse
		}

		if errors.Is(authErr, service.ErrRegDisabled) {
			completionResponse["step"] = oauthPendingChoiceStep
			completionResponse["error"] = "registration_disabled"
			completionResponse["create_account_allowed"] = false
			return completionResponse
		}
		completionResponse["error"] = infraerrors.Reason(authErr)
		return completionResponse
	}
	if tokenPair != nil {
		completionResponse["access_token"] = tokenPair.AccessToken
		completionResponse["refresh_token"] = tokenPair.RefreshToken
		completionResponse["expires_in"] = tokenPair.ExpiresIn
		completionResponse["token_type"] = "Bearer"
	}
	return completionResponse
}

func (h *AuthHandler) redirectWeComBoundIdentityLogin(c *gin.Context, frontendCallback string, redirectTo string, user *service.User) {
	if err := ensureLoginUserActive(user); err != nil {
		redirectOAuthError(c, frontendCallback, "login_blocked", infraerrors.Reason(err), infraerrors.Message(err))
		return
	}
	if err := h.ensureBackendModeAllowsUser(c.Request.Context(), user); err != nil {
		redirectOAuthError(c, frontendCallback, "login_blocked", infraerrors.Reason(err), infraerrors.Message(err))
		return
	}
	tokenPair, err := h.authService.GenerateTokenPair(c.Request.Context(), user, "")
	if err != nil {
		redirectOAuthError(c, frontendCallback, "token_error", "failed to generate token pair", singleLine(err.Error()))
		return
	}
	h.authService.RecordSuccessfulLogin(c.Request.Context(), user.ID)
	fragment := url.Values{}
	fragment.Set("access_token", tokenPair.AccessToken)
	fragment.Set("refresh_token", tokenPair.RefreshToken)
	fragment.Set("expires_in", fmt.Sprintf("%d", tokenPair.ExpiresIn))
	fragment.Set("token_type", "Bearer")
	fragment.Set("redirect", redirectTo)
	fragment.Set("user", encodeOAuthFragmentUser(dto.UserFromService(user)))
	redirectWithFragment(c, frontendCallback, fragment)
}

func entUserToService(user *dbent.User) *service.User {
	if user == nil {
		return nil
	}
	return &service.User{
		ID:             user.ID,
		Email:          user.Email,
		Username:       user.Username,
		Notes:          user.Notes,
		PasswordHash:   user.PasswordHash,
		Role:           user.Role,
		Balance:        user.Balance,
		Concurrency:    user.Concurrency,
		Status:         user.Status,
		SignupSource:   user.SignupSource,
		LastLoginAt:    user.LastLoginAt,
		LastActiveAt:   user.LastActiveAt,
		TotpEnabled:    user.TotpEnabled,
		TotpEnabledAt:  user.TotpEnabledAt,
		RPMLimit:       user.RpmLimit,
		CreatedAt:      user.CreatedAt,
		UpdatedAt:      user.UpdatedAt,
		TotalRecharged: user.TotalRecharged,
	}
}

func encodeOAuthFragmentUser(user *dto.User) string {
	if user == nil {
		return ""
	}
	payload, err := json.Marshal(user)
	if err != nil {
		return ""
	}
	return string(payload)
}

func redirectWeComMobileRequired(c *gin.Context, frontendCallback string, redirectTo string) {
	fragment := url.Values{}
	fragment.Set("error", "wecom_mobile_oauth_required")
	fragment.Set("redirect", redirectTo)
	redirectWithFragment(c, frontendCallback, fragment)
}

func buildWeComEmailRequiredResponse(redirectTo string) map[string]any {
	return map[string]any{
		"redirect":                  redirectTo,
		"step":                      oauthPendingChoiceStep,
		"adoption_required":         true,
		"force_email_on_signup":     true,
		"email_binding_required":    true,
		"existing_account_bindable": true,
		"choice_reason":             "wecom_email_missing",
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func (h *AuthHandler) ensureGenericBindOwnership(ctx context.Context, userID int64, identity service.PendingAuthIdentityKey) error {
	owner, err := h.findOAuthIdentityUser(ctx, identity)
	if err != nil {
		return err
	}
	if owner != nil && owner.ID != userID {
		return infraerrors.Conflict("AUTH_IDENTITY_OWNERSHIP_CONFLICT", "auth identity already belongs to another user")
	}
	return nil
}

func weComSetCookie(c *gin.Context, name, value string, maxAge int, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{Name: name, Value: value, Path: weComOAuthCookiePath, MaxAge: maxAge, HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode})
}

func weComClearCookie(c *gin.Context, name string, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{Name: name, Value: "", Path: weComOAuthCookiePath, MaxAge: -1, HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode})
}
