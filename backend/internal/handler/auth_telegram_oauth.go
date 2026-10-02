package handler

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

const (
	telegramOAuthCookiePath         = "/api/v1/auth/oauth/telegram"
	telegramOAuthRedirectCookie     = "telegram_oauth_redirect"
	telegramOAuthIntentCookieName   = "telegram_oauth_intent"
	telegramOAuthBindUserCookieName = "telegram_oauth_bind_user"
	telegramOAuthDefaultFrontendCB  = "/auth/telegram/callback"
	telegramOAuthAuthorizeURL       = "https://oauth.telegram.org/auth"
	// ponytail: freshness window only, no single-use store; the result travels in a URL
	// fragment + POST body. Add a Redis SETNX on the hash if replay ever matters.
	telegramOAuthMaxAuthAge = time.Hour
)

// TelegramOAuthStart starts the Telegram Login redirect flow.
// GET|POST /api/v1/auth/oauth/telegram/start?redirect=/dashboard
func (h *AuthHandler) TelegramOAuthStart(c *gin.Context) {
	if !h.requireActionCaptchaForOAuthLoginStart(c) {
		return
	}
	cfg, err := h.settingSvc.GetTelegramOAuthConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	returnTo, err := url.Parse(cfg.RedirectURL)
	if err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "telegram redirect url invalid"))
		return
	}

	redirectTo := sanitizeFrontendRedirectPath(c.Query("redirect"))
	if redirectTo == "" {
		redirectTo = linuxDoOAuthDefaultRedirectTo
	}
	browserSessionKey, err := generateOAuthPendingBrowserSession()
	if err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_BROWSER_SESSION_GEN_FAILED", "failed to generate oauth browser session").WithCause(err))
		return
	}

	secureCookie := isRequestHTTPS(c)
	telegramSetCookie(c, telegramOAuthRedirectCookie, encodeCookieValue(redirectTo), linuxDoOAuthCookieMaxAgeSec, secureCookie)
	intent := normalizeOAuthIntent(c.Query("intent"))
	telegramSetCookie(c, telegramOAuthIntentCookieName, encodeCookieValue(intent), linuxDoOAuthCookieMaxAgeSec, secureCookie)
	captureOAuthPromoCode(c, secureCookie)
	setOAuthPendingBrowserCookie(c, browserSessionKey, secureCookie)
	clearOAuthPendingSessionCookie(c, secureCookie)
	if intent == oauthIntentBindCurrentUser {
		bindCookieValue, err := h.buildOAuthBindUserCookieFromContext(c)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		telegramSetCookie(c, telegramOAuthBindUserCookieName, encodeCookieValue(bindCookieValue), linuxDoOAuthCookieMaxAgeSec, secureCookie)
	} else {
		telegramSetCookie(c, telegramOAuthBindUserCookieName, "", -1, secureCookie)
	}

	q := url.Values{}
	q.Set("bot_id", cfg.BotID())
	q.Set("origin", returnTo.Scheme+"://"+returnTo.Host)
	q.Set("return_to", cfg.RedirectURL)
	q.Set("request_access", "write")
	respondOAuthStart(c, telegramOAuthAuthorizeURL+"?"+q.Encode())
}

// TelegramOAuthCallback verifies the Telegram auth result and continues the shared OAuth pending flow.
// POST /api/v1/auth/oauth/telegram/callback (form field tg_auth_result = the #tgAuthResult fragment value)
func (h *AuthHandler) TelegramOAuthCallback(c *gin.Context) {
	frontendCallback := telegramOAuthDefaultFrontendCB
	cfg, err := h.settingSvc.GetTelegramOAuthConfig(c.Request.Context())
	if err != nil {
		redirectOAuthError(c, frontendCallback, "config_error", infraerrors.Message(err), "")
		return
	}

	secureCookie := isRequestHTTPS(c)
	defer func() {
		telegramSetCookie(c, telegramOAuthRedirectCookie, "", -1, secureCookie)
		telegramSetCookie(c, telegramOAuthIntentCookieName, "", -1, secureCookie)
		telegramSetCookie(c, telegramOAuthBindUserCookieName, "", -1, secureCookie)
		clearOAuthPromoCodeCookie(c, secureCookie)
	}()

	// The redirect cookie doubles as proof that this browser started the flow.
	redirectTo, err := readCookieDecoded(c, telegramOAuthRedirectCookie)
	if err != nil {
		redirectOAuthError(c, frontendCallback, "invalid_state", "oauth flow was not started in this browser", "")
		return
	}
	redirectTo = sanitizeFrontendRedirectPath(redirectTo)
	if redirectTo == "" {
		redirectTo = linuxDoOAuthDefaultRedirectTo
	}
	browserSessionKey, _ := readOAuthPendingBrowserCookie(c)
	if strings.TrimSpace(browserSessionKey) == "" {
		redirectOAuthError(c, frontendCallback, "missing_browser_session", "missing oauth browser session", "")
		return
	}
	intent, _ := readCookieDecoded(c, telegramOAuthIntentCookieName)
	intent = normalizeOAuthIntent(intent)

	tgUser, err := verifyTelegramAuthResult(c.PostForm("tg_auth_result"), cfg.BotToken, time.Now())
	if err != nil {
		redirectOAuthError(c, frontendCallback, "invalid_auth_result", "telegram authorization failed", err.Error())
		return
	}

	subject := tgUser.ID
	email := telegramSyntheticEmail(subject)
	username := tgUser.Username
	if username == "" {
		username = "tg_" + subject
	}
	displayName := strings.TrimSpace(tgUser.FirstName + " " + tgUser.LastName)
	if displayName == "" {
		displayName = username
	}
	identityKey := service.PendingAuthIdentityKey{
		ProviderType:    "telegram",
		ProviderKey:     "telegram",
		ProviderSubject: subject,
	}
	upstreamClaims := map[string]any{
		"email":                  email,
		"username":               username,
		"subject":                subject,
		"suggested_display_name": displayName,
		"suggested_avatar_url":   tgUser.PhotoURL,
	}

	if intent == oauthIntentBindCurrentUser {
		targetUserID, err := h.readOAuthBindUserIDFromCookie(c, telegramOAuthBindUserCookieName)
		if err != nil {
			redirectOAuthError(c, frontendCallback, "invalid_state", "invalid oauth bind target", "")
			return
		}
		if err := h.createOAuthPendingSession(c, oauthPendingSessionPayload{
			Intent:                 oauthIntentBindCurrentUser,
			Identity:               identityKey,
			TargetUserID:           &targetUserID,
			ResolvedEmail:          email,
			RedirectTo:             redirectTo,
			BrowserSessionKey:      browserSessionKey,
			UpstreamIdentityClaims: upstreamClaims,
			CompletionResponse:     map[string]any{"redirect": redirectTo},
		}); err != nil {
			redirectOAuthError(c, frontendCallback, "session_error", "failed to continue oauth bind", "")
			return
		}
		redirectToFrontendCallback(c, frontendCallback)
		return
	}

	existingIdentityUser, err := h.findOAuthIdentityUser(c.Request.Context(), identityKey)
	if err != nil {
		redirectOAuthError(c, frontendCallback, "session_error", infraerrors.Reason(err), infraerrors.Message(err))
		return
	}
	if existingIdentityUser != nil {
		if err := h.createOAuthPendingSession(c, oauthPendingSessionPayload{
			Intent:                 oauthIntentLogin,
			Identity:               identityKey,
			TargetUserID:           &existingIdentityUser.ID,
			ResolvedEmail:          existingIdentityUser.Email,
			RedirectTo:             redirectTo,
			BrowserSessionKey:      browserSessionKey,
			UpstreamIdentityClaims: upstreamClaims,
			CompletionResponse:     map[string]any{"redirect": redirectTo},
		}); err != nil {
			redirectOAuthError(c, frontendCallback, "session_error", "failed to continue oauth login", "")
			return
		}
		redirectToFrontendCallback(c, frontendCallback)
		return
	}

	// Telegram has no email to verify, so email_verify_enabled does not gate signup here;
	// only the explicit "force email on third-party signup" toggle asks for one.
	forceEmailOnSignup := h.isForceEmailOnThirdPartySignup(c.Request.Context())
	if !forceEmailOnSignup {
		if err := h.ensureBackendModeAllowsNewUserLogin(c.Request.Context()); err != nil {
			redirectOAuthError(c, frontendCallback, "session_error", infraerrors.Reason(err), infraerrors.Message(err))
			return
		}
		tokenPair, user, err := h.authService.LoginOrRegisterOAuthWithTokenPairAndPromoCode(
			c.Request.Context(), email, username, "", "", readOAuthPromoCode(c), "telegram",
		)
		if err == nil {
			if err := applyPendingOAuthBinding(
				c.Request.Context(),
				h.entClient(),
				h.authService,
				h.userService,
				&dbent.PendingAuthSession{
					Intent:                 oauthIntentLogin,
					ProviderType:           identityKey.ProviderType,
					ProviderKey:            identityKey.ProviderKey,
					ProviderSubject:        identityKey.ProviderSubject,
					ResolvedEmail:          email,
					UpstreamIdentityClaims: upstreamClaims,
				},
				nil,
				&user.ID,
				true,
				false,
			); err != nil {
				redirectOAuthError(c, frontendCallback, "session_error", "failed to bind oauth identity", "")
				return
			}
			h.authService.RecordSuccessfulLogin(c.Request.Context(), user.ID)
			clearOAuthPendingSessionCookie(c, secureCookie)
			clearOAuthPendingBrowserCookie(c, secureCookie)
			redirectOAuthTokenPair(c, frontendCallback, tokenPair, redirectTo)
			return
		}
		if !errors.Is(err, service.ErrOAuthInvitationRequired) {
			redirectOAuthError(c, frontendCallback, "session_error", infraerrors.Reason(err), infraerrors.Message(err))
			return
		}
	}
	if err := h.createLinuxDoOAuthChoicePendingSession(
		c, identityKey, email, email, redirectTo, browserSessionKey, upstreamClaims,
		"", nil, false, forceEmailOnSignup,
	); err != nil {
		redirectOAuthError(c, frontendCallback, "session_error", "failed to continue oauth login", "")
		return
	}
	redirectToFrontendCallback(c, frontendCallback)
}

// CompleteTelegramOAuthRegistration completes a pending Telegram registration with an invitation code.
// POST /api/v1/auth/oauth/telegram/complete-registration
func (h *AuthHandler) CompleteTelegramOAuthRegistration(c *gin.Context) {
	h.completeOAuthRegistration(c, "telegram")
}

type telegramAuthUser struct {
	ID        string
	Username  string
	FirstName string
	LastName  string
	PhotoURL  string
}

// verifyTelegramAuthResult decodes the base64 JSON from #tgAuthResult and checks its hash per
// https://core.telegram.org/widgets/login#checking-authorization:
// hex(HMAC_SHA256(data_check_string, SHA256(bot_token))) over sorted "key=value" lines, hash excluded.
func verifyTelegramAuthResult(raw string, botToken string, now time.Time) (telegramAuthUser, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "=")
	if raw == "" || len(raw) > 4096 {
		return telegramAuthUser{}, errors.New("missing auth result")
	}
	payload, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		if payload, err = base64.RawStdEncoding.DecodeString(raw); err != nil {
			return telegramAuthUser{}, errors.New("malformed auth result")
		}
	}
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.UseNumber()
	var fields map[string]any
	if err := dec.Decode(&fields); err != nil {
		return telegramAuthUser{}, errors.New("malformed auth result")
	}

	values := make(map[string]string, len(fields))
	for k, v := range fields {
		switch tv := v.(type) {
		case string:
			values[k] = tv
		case json.Number:
			values[k] = tv.String()
		default:
			return telegramAuthUser{}, errors.New("unexpected auth result field")
		}
	}
	gotHash := values["hash"]
	delete(values, "hash")
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	lines := make([]string, len(keys))
	for i, k := range keys {
		lines[i] = k + "=" + values[k]
	}
	secret := sha256.Sum256([]byte(botToken))
	mac := hmac.New(sha256.New, secret[:])
	_, _ = mac.Write([]byte(strings.Join(lines, "\n")))
	if !hmac.Equal([]byte(hex.EncodeToString(mac.Sum(nil))), []byte(strings.ToLower(gotHash))) {
		return telegramAuthUser{}, errors.New("invalid auth result signature")
	}

	var authSec int64
	if n, ok := fields["auth_date"].(json.Number); ok {
		authSec, _ = n.Int64()
	}
	if age := now.Sub(time.Unix(authSec, 0)); age > telegramOAuthMaxAuthAge || age < -5*time.Minute {
		return telegramAuthUser{}, errors.New("auth result expired")
	}

	user := telegramAuthUser{
		ID:        values["id"],
		Username:  strings.TrimSpace(values["username"]),
		FirstName: strings.TrimSpace(values["first_name"]),
		LastName:  strings.TrimSpace(values["last_name"]),
		PhotoURL:  strings.TrimSpace(values["photo_url"]),
	}
	if !isSafeLinuxDoSubject(user.ID) {
		return telegramAuthUser{}, errors.New("invalid telegram user id")
	}
	return user, nil
}

func telegramSyntheticEmail(subject string) string {
	return "telegram-" + subject + service.TelegramConnectSyntheticEmailDomain
}

// telegramSetCookie writes (maxAgeSec > 0) or clears (maxAgeSec < 0) a cookie scoped to the Telegram OAuth routes.
func telegramSetCookie(c *gin.Context, name, value string, maxAgeSec int, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     telegramOAuthCookiePath,
		MaxAge:   maxAgeSec,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}
