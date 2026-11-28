package handler

import (
	"crypto/subtle"
	"net/http"
	"net/url"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/oauth"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const feishuCookiePath = "/api/v1/auth/oauth/feishu"
const feishuFrontendCallback = "/auth/feishu/callback"

func setFeishuCookie(c *gin.Context, name, value string, age int) {
	http.SetCookie(c.Writer, &http.Cookie{Name: "feishu_oauth_" + name, Value: value, Path: feishuCookiePath, MaxAge: age, HttpOnly: true, Secure: isRequestHTTPS(c), SameSite: http.SameSiteLaxMode})
}

func (h *AuthHandler) feishuConfig() (config.FeishuConnectConfig, error) {
	if h == nil || h.cfg == nil || !h.cfg.Feishu.Enabled {
		return config.FeishuConnectConfig{}, infraerrors.NotFound("OAUTH_DISABLED", "Feishu sign-in is disabled")
	}
	cfg := h.cfg.Feishu
	if err := config.ValidateFeishuConfig(cfg); err != nil {
		return cfg, infraerrors.ServiceUnavailable("OAUTH_CONFIG_INVALID", "Feishu sign-in is not configured")
	}
	return cfg, nil
}

func (h *AuthHandler) FeishuOAuthStart(c *gin.Context) {
	if !h.requireActionCaptchaForOAuthLoginStart(c) {
		return
	}
	cfg, err := h.feishuConfig()
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	state, err := oauth.GenerateState()
	if err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_STATE_GEN_FAILED", "could not create sign-in state"))
		return
	}
	browser, err := generateOAuthPendingBrowserSession()
	if err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_BROWSER_SESSION_GEN_FAILED", "could not create browser session"))
		return
	}
	redirect := sanitizeFrontendRedirectPath(c.Query("redirect"))
	if redirect == "" {
		redirect = "/dashboard"
	}
	intent := normalizeOAuthIntent(c.Query("intent"))
	if intent == oauthIntentBindCurrentUser {
		bindUser, err := h.buildOAuthBindUserCookieFromContext(c)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		setFeishuCookie(c, "bind_user", encodeCookieValue(bindUser), 600)
	} else {
		setFeishuCookie(c, "bind_user", "", -1)
	}
	setFeishuCookie(c, "state", encodeCookieValue(state), 600)
	setFeishuCookie(c, "redirect", encodeCookieValue(redirect), 600)
	setFeishuCookie(c, "intent", encodeCookieValue(intent), 600)
	setOAuthPendingBrowserCookie(c, browser, isRequestHTTPS(c))
	clearOAuthPendingSessionCookie(c, isRequestHTTPS(c))
	captureOAuthPromoCode(c, isRequestHTTPS(c))
	authorize := "https://accounts.feishu.cn/open-apis/authen/v1/authorize?" + url.Values{
		"client_id": {cfg.ClientID}, "redirect_uri": {cfg.RedirectURL}, "response_type": {"code"}, "state": {state},
	}.Encode()
	respondOAuthStart(c, authorize)
}

func (h *AuthHandler) FeishuOAuthCallback(c *gin.Context) {
	defer func() {
		for _, name := range []string{"state", "redirect", "intent", "bind_user"} {
			setFeishuCookie(c, name, "", -1)
		}
		clearOAuthPromoCodeCookie(c, isRequestHTTPS(c))
	}()
	fail := func(code, message string) { redirectOAuthError(c, feishuFrontendCallback, code, message, "") }
	cfg, err := h.feishuConfig()
	if err != nil {
		fail("feishu_not_enabled", "Feishu sign-in is not available")
		return
	}
	state, err := readCookieDecoded(c, "feishu_oauth_state")
	got, code := c.Query("state"), strings.TrimSpace(c.Query("code"))
	if err != nil || state == "" || subtle.ConstantTimeCompare([]byte(state), []byte(got)) != 1 || code == "" || len(code) > 2048 {
		fail("invalid_state", "Invalid or expired Feishu sign-in state")
		return
	}
	browser, err := readOAuthPendingBrowserCookie(c)
	if err != nil || browser == "" {
		fail("missing_browser_session", "Sign-in browser session is missing")
		return
	}
	profile, err := fetchFeishuProfile(c.Request.Context(), h.feishuHTTPClient, cfg, code)
	if err != nil {
		fail("feishu_identity_failed", "Could not verify an allowed Feishu identity")
		return
	}
	redirect, _ := readCookieDecoded(c, "feishu_oauth_redirect")
	redirect = sanitizeFrontendRedirectPath(redirect)
	if redirect == "" {
		redirect = "/dashboard"
	}
	intent, _ := readCookieDecoded(c, "feishu_oauth_intent")
	intent = normalizeOAuthIntent(intent)
	candidates := feishuSubjectCandidates(profile)
	identity := service.PendingAuthIdentityKey{ProviderType: "feishu", ProviderKey: cfg.ClientID, ProviderSubject: candidates[0]}
	var targetID *int64
	for _, subject := range candidates {
		candidate := service.PendingAuthIdentityKey{ProviderType: "feishu", ProviderKey: cfg.ClientID, ProviderSubject: subject}
		user, err := h.findOAuthIdentityUser(c.Request.Context(), candidate)
		if err != nil {
			fail("session_error", "Could not resolve Feishu identity")
			return
		}
		if user == nil {
			continue
		}
		if targetID != nil && *targetID != user.ID {
			fail("identity_conflict", "Feishu identity requires administrator reconciliation")
			return
		}
		id := user.ID
		targetID, identity = &id, candidate
	}
	if intent == oauthIntentBindCurrentUser {
		id, err := h.readOAuthBindUserIDFromCookie(c, "feishu_oauth_bind_user")
		if err != nil || (targetID != nil && *targetID != id) {
			fail("identity_conflict", "Feishu identity cannot be bound to this account")
			return
		}
		targetID = &id
	}
	// Never use a third-party email to claim an existing local account.
	// The shared pending flow requires local authentication for explicit linking,
	// applies registration/2FA policy and consumes browser-bound sessions once.
	email := "feishu-" + identity.ProviderSubject + service.FeishuConnectSyntheticEmailDomain
	claims := map[string]any{"email": email, "subject": identity.ProviderSubject, "username": profile.Name, "suggested_display_name": profile.Name, "tenant_key": profile.TenantKey}
	if targetID == nil && intent == oauthIntentLogin {
		err = h.createLinuxDoOAuthChoicePendingSession(c, identity, email, email, redirect, browser, claims, "", nil, h.authService.IsEmailVerifyEnabled(c.Request.Context()), h.isForceEmailOnThirdPartySignup(c.Request.Context()))
	} else {
		err = h.createOAuthPendingSession(c, oauthPendingSessionPayload{Intent: intent, Identity: identity, TargetUserID: targetID, ResolvedEmail: email, RedirectTo: redirect, BrowserSessionKey: browser, UpstreamIdentityClaims: claims, CompletionResponse: map[string]any{"redirect": redirect}})
	}
	if err != nil {
		fail("session_error", "Could not continue Feishu sign-in")
		return
	}
	redirectToFrontendCallback(c, feishuFrontendCallback)
}

func (h *AuthHandler) BindFeishuOAuthLogin(c *gin.Context) { h.bindPendingOAuthLogin(c, "feishu") }
func (h *AuthHandler) CreateFeishuOAuthAccount(c *gin.Context) {
	h.createPendingOAuthAccount(c, "feishu")
}
