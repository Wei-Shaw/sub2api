package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/oauth"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// ─── 常量 ──────────────────────────────────────────────────────────────────

const (
	feishuOAuthCookiePath         = "/api/v1/auth/oauth/feishu"
	feishuOAuthStateCookieName    = "feishu_oauth_state"
	feishuOAuthRedirectCookie     = "feishu_oauth_redirect"
	feishuOAuthIntentCookieName   = "feishu_oauth_intent"
	feishuOAuthBindUserCookieName = "feishu_oauth_bind_user"
	feishuOAuthCookieMaxAgeSec    = 10 * 60
	feishuOAuthDefaultRedirectTo  = "/dashboard"
	feishuOAuthDefaultFrontendCB  = "/auth/feishu/callback"
)

// ─── Config helper ─────────────────────────────────────────────────────────

func (h *AuthHandler) getFeishuOAuthConfig(ctx context.Context) (config.FeishuConnectConfig, error) {
	if h != nil && h.settingSvc != nil {
		return h.settingSvc.GetFeishuConnectOAuthConfig(ctx)
	}
	if h == nil || h.cfg == nil {
		return config.FeishuConnectConfig{}, infraerrors.ServiceUnavailable("CONFIG_NOT_READY", "config not loaded")
	}
	if !h.cfg.Feishu.Enabled {
		return config.FeishuConnectConfig{}, infraerrors.NotFound("OAUTH_DISABLED", "feishu oauth login is disabled")
	}
	return h.cfg.Feishu, nil
}

// ─── Cookie helpers（feishu path）──────────────────────────────────────────

func setFeishuCookie(c *gin.Context, name string, value string, maxAgeSec int, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     feishuOAuthCookiePath,
		MaxAge:   maxAgeSec,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearFeishuCookie(c *gin.Context, name string, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     feishuOAuthCookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// ─── FeishuOAuthStart ──────────────────────────────────────────────────────

// FeishuOAuthStart 启动飞书 OAuth 登录流程。
// GET /api/v1/auth/oauth/feishu/start?redirect=/dashboard&intent=login
func (h *AuthHandler) FeishuOAuthStart(c *gin.Context) {
	cfg, err := h.getFeishuOAuthConfig(c.Request.Context())
	if err != nil {
		redirectOAuthError(c, feishuOAuthDefaultFrontendCB, "feishu_not_enabled", "", "")
		return
	}

	state, err := oauth.GenerateState()
	if err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_STATE_GEN_FAILED", "failed to generate oauth state").WithCause(err))
		return
	}

	redirectTo := sanitizeFrontendRedirectPath(c.Query("redirect"))
	if redirectTo == "" {
		redirectTo = feishuOAuthDefaultRedirectTo
	}

	browserSessionKey, err := generateOAuthPendingBrowserSession()
	if err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_BROWSER_SESSION_GEN_FAILED", "failed to generate oauth browser session").WithCause(err))
		return
	}

	secureCookie := isRequestHTTPS(c)
	setFeishuCookie(c, feishuOAuthStateCookieName, encodeCookieValue(state), feishuOAuthCookieMaxAgeSec, secureCookie)
	setFeishuCookie(c, feishuOAuthRedirectCookie, encodeCookieValue(redirectTo), feishuOAuthCookieMaxAgeSec, secureCookie)

	intent := normalizeOAuthIntent(c.Query("intent"))
	setFeishuCookie(c, feishuOAuthIntentCookieName, encodeCookieValue(intent), feishuOAuthCookieMaxAgeSec, secureCookie)
	captureOAuthPromoCode(c, secureCookie)

	setOAuthPendingBrowserCookie(c, browserSessionKey, secureCookie)
	clearOAuthPendingSessionCookie(c, secureCookie)

	if intent == oauthIntentBindCurrentUser {
		bindCookieValue, err := h.buildOAuthBindUserCookieFromContext(c)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		setFeishuCookie(c, feishuOAuthBindUserCookieName, encodeCookieValue(bindCookieValue), feishuOAuthCookieMaxAgeSec, secureCookie)
	} else {
		clearFeishuCookie(c, feishuOAuthBindUserCookieName, secureCookie)
	}

	authURL, err := buildFeishuAuthorizeURL(cfg, state)
	if err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_BUILD_URL_FAILED", "failed to build feishu authorization url").WithCause(err))
		return
	}

	c.Redirect(http.StatusFound, authURL)
}

// ─── FeishuOAuthCallback ───────────────────────────────────────────────────

// FeishuOAuthCallback 处理飞书授权回调。
// GET /api/v1/auth/oauth/feishu/callback?code=...&state=...
func (h *AuthHandler) FeishuOAuthCallback(c *gin.Context) {
	cfg, cfgErr := h.getFeishuOAuthConfig(c.Request.Context())
	if cfgErr != nil {
		response.ErrorFrom(c, cfgErr)
		return
	}

	frontendCallback := strings.TrimSpace(cfg.FrontendRedirectURL)
	if frontendCallback == "" {
		frontendCallback = feishuOAuthDefaultFrontendCB
	}

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
		clearFeishuCookie(c, feishuOAuthStateCookieName, secureCookie)
		clearFeishuCookie(c, feishuOAuthRedirectCookie, secureCookie)
		clearFeishuCookie(c, feishuOAuthIntentCookieName, secureCookie)
		clearOAuthPromoCodeCookie(c, secureCookie)
	}()

	expectedState, err := readCookieDecoded(c, feishuOAuthStateCookieName)
	if err != nil || expectedState == "" || state != expectedState {
		redirectOAuthError(c, frontendCallback, "csrf", "state mismatch", "")
		return
	}
	redirectTo, _ := readCookieDecoded(c, feishuOAuthRedirectCookie)
	redirectTo = sanitizeFrontendRedirectPath(redirectTo)
	if redirectTo == "" {
		redirectTo = feishuOAuthDefaultRedirectTo
	}
	intent, _ := readCookieDecoded(c, feishuOAuthIntentCookieName)
	intent = normalizeOAuthIntent(intent)
	browserSessionKey, _ := readOAuthPendingBrowserCookie(c)
	if strings.TrimSpace(browserSessionKey) == "" {
		redirectOAuthError(c, frontendCallback, "missing_browser_session", "missing browser session cookie", "")
		return
	}

	redirectURI := strings.TrimSpace(cfg.RedirectURL)

	// Step 1: code 换 user_access_token（飞书 v2，JSON body）
	client := h.feishuClient(cfg)
	userToken, err := client.ExchangeCodeForUserToken(c.Request.Context(), code, redirectURI)
	if err != nil {
		feishuUpstreamRedirect(c, frontendCallback, "exchange_code", err)
		return
	}

	// Step 2: 拉取用户信息（union_id / open_id / name / email / tenant_key）
	info, err := client.GetUserInfo(c.Request.Context(), userToken.AccessToken)
	if err != nil {
		feishuUpstreamRedirect(c, frontendCallback, "get_user_info", err)
		return
	}

	// 企业限制：仅允许白名单内的 tenant_key 登录。
	if !checkFeishuTenantAllowed(cfg, info.TenantKey) {
		// 不透传 tenant_key，避免内部企业标识泄露到前端。
		redirectOAuthError(c, frontendCallback, "tenant_rejected", "", "")
		return
	}

	unionID := strings.TrimSpace(info.UnionID)
	resolvedEmail := info.ResolvedEmail()
	username := strings.TrimSpace(info.Name)
	if username == "" {
		username = "feishu_" + unionID
	}

	identityKey := service.PendingAuthIdentityKey{ProviderType: "feishu", ProviderKey: "feishu", ProviderSubject: unionID}
	upstreamClaims := map[string]any{
		"email":                  resolvedEmail,
		"username":               username,
		"subject":                unionID,
		"union_id":               unionID,
		"open_id":                strings.TrimSpace(info.OpenID),
		"tenant_key":             strings.TrimSpace(info.TenantKey),
		"suggested_display_name": strings.TrimSpace(info.Name),
		"suggested_avatar_url":   strings.TrimSpace(info.AvatarURL),
	}

	// ─── 主动绑定分支 ───
	if intent == oauthIntentBindCurrentUser {
		targetUserID, err := h.readOAuthBindUserIDFromCookie(c, feishuOAuthBindUserCookieName)
		if err != nil {
			redirectOAuthError(c, frontendCallback, "invalid_state", "invalid bind user cookie", "")
			return
		}
		bindResolvedEmail := resolvedEmail
		if bindResolvedEmail == "" {
			bindResolvedEmail = buildFeishuSyntheticEmail(unionID)
		}
		if err := h.createOAuthPendingSession(c, oauthPendingSessionPayload{
			Intent: oauthIntentBindCurrentUser, Identity: identityKey,
			TargetUserID: &targetUserID, ResolvedEmail: bindResolvedEmail,
			RedirectTo: redirectTo, BrowserSessionKey: browserSessionKey,
			UpstreamIdentityClaims: upstreamClaims,
			CompletionResponse:     map[string]any{"redirect": redirectTo},
		}); err != nil {
			redirectOAuthError(c, frontendCallback, "session_error", infraerrors.Reason(err), infraerrors.Message(err))
			return
		}
		clearFeishuCookie(c, feishuOAuthBindUserCookieName, secureCookie)
		redirectToFrontendCallback(c, frontendCallback)
		return
	}

	// ─── 已有身份直接登录 ───
	existingIdentityUser, err := h.findOAuthIdentityUser(c.Request.Context(), identityKey)
	if err != nil {
		redirectOAuthError(c, frontendCallback, "session_error", infraerrors.Reason(err), infraerrors.Message(err))
		return
	}
	if existingIdentityUser != nil {
		if err := h.createOAuthPendingSession(c, oauthPendingSessionPayload{
			Intent: oauthIntentLogin, Identity: identityKey, TargetUserID: &existingIdentityUser.ID,
			ResolvedEmail: existingIdentityUser.Email, RedirectTo: redirectTo, BrowserSessionKey: browserSessionKey,
			UpstreamIdentityClaims: upstreamClaims,
			CompletionResponse:     map[string]any{"redirect": redirectTo},
		}); err != nil {
			redirectOAuthError(c, frontendCallback, "session_error", "failed to continue oauth login", "")
			return
		}
		redirectToFrontendCallback(c, frontendCallback)
		return
	}

	// ─── 新用户：拿到企业邮箱直接建号 ───
	// 决策：仅允许特定飞书企业（tenant 白名单）+ 拿到 email 直接建号。
	// 因此新用户默认走 LoginOrRegister 直接发 token；仅当站点要求邀请码/邮箱验证/强制补邮箱时才降级到注册表单。
	if resolvedEmail == "" {
		// 企业用户理应有邮箱；为空多半是飞书应用未申请 email scope。
		redirectOAuthError(c, frontendCallback, "email_required",
			"feishu user_info returned no email; grant contact:user.email scope to the Feishu app", "")
		return
	}

	emailVerificationRequired := h != nil && h.authService != nil && h.authService.IsEmailVerifyEnabled(c.Request.Context())
	forceEmailOnSignup := h.isForceEmailOnThirdPartySignup(c.Request.Context())
	if !emailVerificationRequired && !forceEmailOnSignup {
		if err := h.ensureBackendModeAllowsNewUserLogin(c.Request.Context()); err != nil {
			redirectOAuthError(c, frontendCallback, "session_error", infraerrors.Reason(err), infraerrors.Message(err))
			return
		}
		tokenPair, user, regErr := h.authService.LoginOrRegisterOAuthWithTokenPairAndPromoCode(
			c.Request.Context(),
			resolvedEmail,
			username,
			"",
			"",
			readOAuthPromoCode(c),
			"feishu",
		)
		if regErr == nil {
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
					ResolvedEmail:          resolvedEmail,
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
		if !errors.Is(regErr, service.ErrOAuthInvitationRequired) {
			redirectOAuthError(c, frontendCallback, "session_error", infraerrors.Reason(regErr), infraerrors.Message(regErr))
			return
		}
		// 需要邀请码 → 降级到注册表单
	}

	// ─── 降级：需要邀请码 / 邮箱验证 / 强制补邮箱 → 注册完成表单 ───
	if err := h.createFeishuOAuthRegistrationPendingSession(
		c, identityKey, resolvedEmail, redirectTo, browserSessionKey, upstreamClaims,
		emailVerificationRequired, forceEmailOnSignup,
	); err != nil {
		redirectOAuthError(c, frontendCallback, "session_error", "failed to continue oauth login", "")
		return
	}
	redirectToFrontendCallback(c, frontendCallback)
}

// createFeishuOAuthRegistrationPendingSession 构建注册完成用的 pending session。
// 飞书始终有真实企业邮箱，因此不做 compat email 兼容匹配；仅在需要邀请码/邮箱验证/强制补邮箱时使用。
func (h *AuthHandler) createFeishuOAuthRegistrationPendingSession(
	c *gin.Context,
	identity service.PendingAuthIdentityKey,
	resolvedEmail string,
	redirectTo string,
	browserSessionKey string,
	upstreamClaims map[string]any,
	emailVerificationRequired bool,
	forceEmailOnSignup bool,
) error {
	email := strings.TrimSpace(resolvedEmail)
	completionResponse := map[string]any{
		"step":                   oauthPendingChoiceStep,
		"adoption_required":      true,
		"redirect":               strings.TrimSpace(redirectTo),
		"email":                  email,
		"resolved_email":         email,
		"create_account_allowed": true,
		"force_email_on_signup":  forceEmailOnSignup,
		"choice_reason":          "third_party_signup",
	}
	resolvedChoiceEmail := email
	if forceEmailOnSignup {
		completionResponse["choice_reason"] = "force_email_on_signup"
	}
	if emailVerificationRequired || forceEmailOnSignup {
		completionResponse["step"] = "create_account_required"
		completionResponse["email_binding_required"] = true
		completionResponse["force_email_on_signup"] = true
		if emailVerificationRequired {
			completionResponse["choice_reason"] = "email_verification_required"
		}
		delete(completionResponse, "email")
		delete(completionResponse, "resolved_email")
		resolvedChoiceEmail = ""
	}

	return h.createOAuthPendingSession(c, oauthPendingSessionPayload{
		Intent:                 oauthIntentLogin,
		Identity:               identity,
		TargetUserID:           nil,
		ResolvedEmail:          resolvedChoiceEmail,
		RedirectTo:             redirectTo,
		BrowserSessionKey:      browserSessionKey,
		UpstreamIdentityClaims: upstreamClaims,
		CompletionResponse:     completionResponse,
	})
}

// ─── Complete Registration ─────────────────────────────────────────────────

type completeFeishuOAuthRequest struct {
	InvitationCode   string `json:"invitation_code" binding:"required"`
	AffCode          string `json:"aff_code,omitempty"`
	AdoptDisplayName *bool  `json:"adopt_display_name,omitempty"`
	AdoptAvatar      *bool  `json:"adopt_avatar,omitempty"`
}

// CompleteFeishuOAuthRegistration 校验邀请码并完成飞书 OAuth 注册。
// POST /api/v1/auth/oauth/feishu/complete-registration
func (h *AuthHandler) CompleteFeishuOAuthRegistration(c *gin.Context) {
	var req completeFeishuOAuthRequest
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
	if err := h.ensureBackendModeAllowsNewUserLogin(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	email := strings.TrimSpace(session.ResolvedEmail)
	username := pendingSessionStringValue(session.UpstreamIdentityClaims, "username")
	if username == "" {
		if at := strings.Index(email, "@"); at > 0 {
			username = email[:at]
		} else {
			username = email
		}
	}
	if email == "" || username == "" {
		response.ErrorFrom(c, infraerrors.BadRequest("PENDING_AUTH_SESSION_INVALID", "pending auth registration context is invalid"))
		return
	}

	client := h.entClient()
	if client == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("PENDING_AUTH_NOT_READY", "pending auth service is not ready"))
		return
	}
	if err := ensurePendingOAuthRegistrationIdentityAvailable(c.Request.Context(), client, session); err != nil {
		respondPendingOAuthBindingApplyError(c, err)
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
	tokenPair, user, err := h.authService.LoginOrRegisterOAuthWithTokenPairAndPromoCode(
		c.Request.Context(),
		email,
		username,
		req.InvitationCode,
		req.AffCode,
		pendingOAuthPromoCode(session),
		"feishu",
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if err := applyPendingOAuthAdoptionAndConsumeSession(c.Request.Context(), client, h.authService, h.userService, session, decision, user.ID); err != nil {
		respondPendingOAuthBindingApplyError(c, err)
		return
	}
	h.authService.RecordSuccessfulLogin(c.Request.Context(), user.ID)
	clearOAuthPendingSessionCookie(c, secureCookie)
	clearOAuthPendingBrowserCookie(c, secureCookie)

	c.JSON(http.StatusOK, gin.H{
		"access_token":  tokenPair.AccessToken,
		"refresh_token": tokenPair.RefreshToken,
		"expires_in":    tokenPair.ExpiresIn,
		"token_type":    "Bearer",
	})
}

// CreateFeishuOAuthAccount 从 pending 会话创建新账户。
// POST /api/v1/auth/oauth/feishu/create-account
func (h *AuthHandler) CreateFeishuOAuthAccount(c *gin.Context) {
	h.createPendingOAuthAccount(c, "feishu")
}

// BindFeishuOAuthLogin 处理已有账户绑定飞书登录。
// POST /api/v1/auth/oauth/feishu/bind-login
func (h *AuthHandler) BindFeishuOAuthLogin(c *gin.Context) {
	h.bindPendingOAuthLogin(c, "feishu")
}

// ─── helpers ───────────────────────────────────────────────────────────────

func buildFeishuSyntheticEmail(unionID string) string {
	return "feishu-" + strings.ToLower(strings.TrimSpace(unionID)) + service.FeishuConnectSyntheticEmailDomain
}

// checkFeishuTenantAllowed 校验 tenant_key 是否被允许。
// 未开启限制时放行所有；开启限制时仅放行白名单内的 tenant_key。
func checkFeishuTenantAllowed(cfg config.FeishuConnectConfig, tenantKey string) bool {
	if !cfg.RestrictTenant {
		return true
	}
	allowed := service.FeishuAllowedTenantKeySet(cfg.AllowedTenantKeys)
	if len(allowed) == 0 {
		// fail-closed：开启限制但未配置任何 tenant_key，拒绝所有登录。
		return false
	}
	_, ok := allowed[strings.TrimSpace(tenantKey)]
	return ok
}

// feishuUpstreamRedirect 记录上游错误日志并跳错误页。
func feishuUpstreamRedirect(c *gin.Context, frontendCallback, step string, err error) {
	var apiErr *FeishuAPIError
	code := 0
	msg := ""
	httpStatus := 0
	if errors.As(err, &apiErr) {
		code = apiErr.Code
		msg = apiErr.Message
		httpStatus = apiErr.HTTP
	}
	slog.Error("feishu upstream call failed",
		"step", step,
		"feishu_code", code,
		"feishu_msg", msg,
		"http_status", httpStatus,
		"error", err.Error(),
	)
	message := msg
	if strings.TrimSpace(message) == "" {
		message = infraerrors.Message(err)
	}
	redirectOAuthError(c, frontendCallback, "upstream_error", message, "")
}

// feishuClient 构造或返回缓存的 client 实例（cfg 关键字段变化时重建）。
func (h *AuthHandler) feishuClient(cfg config.FeishuConnectConfig) *FeishuClient {
	h.feishuClientMu.Lock()
	defer h.feishuClientMu.Unlock()
	newCfg := feishuClientConfig{
		AppID:       cfg.AppID,
		AppSecret:   cfg.AppSecret,
		TokenURL:    cfg.TokenURL,
		UserInfoURL: cfg.UserInfoURL,
	}
	if h.feishuClientInstance == nil || h.feishuClientInstance.cfg != newCfg {
		h.feishuClientInstance = &FeishuClient{
			cfg:        newCfg,
			httpClient: &http.Client{Timeout: 10 * time.Second},
		}
	}
	return h.feishuClientInstance
}

// buildFeishuAuthorizeURL 构建飞书 OAuth 授权 URL。
func buildFeishuAuthorizeURL(cfg config.FeishuConnectConfig, state string) (string, error) {
	base := strings.TrimSpace(cfg.AuthorizeURL)
	if base == "" {
		return "", infraerrors.InternalServer("FEISHU_AUTHORIZE_URL_EMPTY", "feishu authorize_url not configured")
	}
	redirectURI := strings.TrimSpace(cfg.RedirectURL)
	if redirectURI == "" {
		return "", infraerrors.InternalServer("FEISHU_REDIRECT_URL_EMPTY", "feishu redirect_url not configured")
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", infraerrors.InternalServer("FEISHU_AUTHORIZE_URL_PARSE_FAILED", "failed to parse feishu authorize_url").WithCause(err)
	}
	q := u.Query()
	q.Set("client_id", cfg.AppID)
	q.Set("redirect_uri", redirectURI)
	q.Set("response_type", "code")
	if scopes := strings.TrimSpace(cfg.Scopes); scopes != "" {
		q.Set("scope", scopes)
	}
	q.Set("state", state)
	u.RawQuery = q.Encode()
	return u.String(), nil
}
