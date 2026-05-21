package handler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	dbuser "github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/oauth"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// dingTalkUpstreamRedirect 在 4 步链上游调用失败时记录详细错误日志并跳错误页。
// 把钉钉 errcode/errmsg 写进 backend log + URL fragment，避免被泛 "internal error" 吞掉。
func dingTalkUpstreamRedirect(c *gin.Context, frontendCallback, step string, err error) {
	var apiErr *DingTalkAPIError
	dtCode := ""
	dtMsg := ""
	dtHTTP := 0
	if errors.As(err, &apiErr) {
		dtCode = apiErr.Code
		dtMsg = apiErr.Message
		dtHTTP = apiErr.HTTP
	}
	slog.Error("dingtalk upstream call failed",
		"step", step,
		"dingtalk_code", dtCode,
		"dingtalk_msg", dtMsg,
		"http_status", dtHTTP,
		"error", err.Error(),
	)
	msg := dtMsg
	if strings.TrimSpace(msg) == "" {
		msg = infraerrors.Message(err)
	}
	if strings.TrimSpace(dtCode) != "" {
		msg = "dingtalk[" + dtCode + "] " + msg
	}
	redirectOAuthError(c, frontendCallback, mapDingTalkErrorCode(err), msg, "")
}

// ─── 常量 ──────────────────────────────────────────────────────────────────

const (
	dingTalkOAuthCookiePath         = "/api/v1/auth/oauth/dingtalk"
	dingTalkOAuthStateCookieName    = "dingtalk_oauth_state"
	dingTalkOAuthRedirectCookie     = "dingtalk_oauth_redirect"
	dingTalkOAuthIntentCookieName   = "dingtalk_oauth_intent"
	dingTalkOAuthBindUserCookieName = "dingtalk_oauth_bind_user"
	dingTalkOAuthCookieMaxAgeSec    = 600 // 10 分钟
	dingTalkOAuthDefaultRedirectTo  = "/dashboard"
	dingTalkOAuthDefaultFrontendCB  = "/auth/dingtalk/callback"

	dingTalkLevelThreeEnabled = true
)

// ─── Config helper ─────────────────────────────────────────────────────────

// getDingTalkOAuthConfig 返回 DingTalk OAuth 最终生效配置。
// 优先从 settingSvc（settings 表）读取，回退到 h.cfg.DingTalk。
func (h *AuthHandler) getDingTalkOAuthConfig(ctx context.Context) (config.DingTalkConnectConfig, error) {
	if h != nil && h.settingSvc != nil {
		return h.settingSvc.GetDingTalkConnectOAuthConfig(ctx)
	}
	if h == nil || h.cfg == nil {
		return config.DingTalkConnectConfig{}, infraerrors.ServiceUnavailable("CONFIG_NOT_READY", "config not loaded")
	}
	if !h.cfg.DingTalk.Enabled {
		return config.DingTalkConnectConfig{}, infraerrors.NotFound("OAUTH_DISABLED", "dingtalk oauth login is disabled")
	}
	return h.cfg.DingTalk, nil
}

// ─── Cookie helpers（使用 dingtalk path）─────────────────────────────────

func setDingTalkCookie(c *gin.Context, name string, value string, maxAgeSec int, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     dingTalkOAuthCookiePath,
		MaxAge:   maxAgeSec,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearDingTalkCookie(c *gin.Context, name string, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     dingTalkOAuthCookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// ─── DingTalkOAuthStart ────────────────────────────────────────────────────

// DingTalkOAuthStart 启动 DingTalk Connect OAuth 登录流程。
// GET /api/v1/auth/oauth/dingtalk/start?redirect=/dashboard&intent=login
func (h *AuthHandler) DingTalkOAuthStart(c *gin.Context) {
	cfg, err := h.getDingTalkOAuthConfig(c.Request.Context())
	if err != nil {
		frontendCB := dingTalkOAuthDefaultFrontendCB
		redirectOAuthError(c, frontendCB, "dingtalk_not_enabled", "", "")
		return
	}

	state, err := oauth.GenerateState()
	if err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_STATE_GEN_FAILED", "failed to generate oauth state").WithCause(err))
		return
	}

	redirectTo := sanitizeFrontendRedirectPath(c.Query("redirect"))
	if redirectTo == "" {
		redirectTo = dingTalkOAuthDefaultRedirectTo
	}

	browserSessionKey, err := generateOAuthPendingBrowserSession()
	if err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_BROWSER_SESSION_GEN_FAILED", "failed to generate oauth browser session").WithCause(err))
		return
	}

	secureCookie := isRequestHTTPS(c)
	setDingTalkCookie(c, dingTalkOAuthStateCookieName, encodeCookieValue(state), dingTalkOAuthCookieMaxAgeSec, secureCookie)
	setDingTalkCookie(c, dingTalkOAuthRedirectCookie, encodeCookieValue(redirectTo), dingTalkOAuthCookieMaxAgeSec, secureCookie)

	intent := normalizeOAuthIntent(c.Query("intent"))
	setDingTalkCookie(c, dingTalkOAuthIntentCookieName, encodeCookieValue(intent), dingTalkOAuthCookieMaxAgeSec, secureCookie)

	setOAuthPendingBrowserCookie(c, browserSessionKey, secureCookie)
	clearOAuthPendingSessionCookie(c, secureCookie)

	if intent == oauthIntentBindCurrentUser {
		bindCookieValue, err := h.buildOAuthBindUserCookieFromContext(c)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		setDingTalkCookie(c, dingTalkOAuthBindUserCookieName, encodeCookieValue(bindCookieValue), dingTalkOAuthCookieMaxAgeSec, secureCookie)
	} else {
		clearDingTalkCookie(c, dingTalkOAuthBindUserCookieName, secureCookie)
	}

	authURL, err := buildDingTalkAuthorizeURL(cfg, state)
	if err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_BUILD_URL_FAILED", "failed to build dingtalk authorization url").WithCause(err))
		return
	}

	c.Redirect(http.StatusFound, authURL)
}

// ─── buildDingTalkAuthorizeURL ─────────────────────────────────────────────

// ─── findDingTalkCompatEmailUser ───────────────────────────────────────────

// findDingTalkCompatEmailUser 通过真实邮箱查找可与 DingTalk 账号兼容绑定的现有用户。
func (h *AuthHandler) findDingTalkCompatEmailUser(ctx context.Context, email string) (*dbent.User, error) {
	if !dingTalkLevelThreeEnabled {
		return nil, nil
	}

	client := h.entClient()
	if client == nil {
		return nil, infraerrors.ServiceUnavailable("PENDING_AUTH_NOT_READY", "pending auth service is not ready")
	}

	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" ||
		strings.HasSuffix(email, service.DingTalkConnectSyntheticEmailDomain) ||
		strings.HasSuffix(email, service.LinuxDoConnectSyntheticEmailDomain) ||
		strings.HasSuffix(email, service.OIDCConnectSyntheticEmailDomain) ||
		strings.HasSuffix(email, service.WeChatConnectSyntheticEmailDomain) {
		return nil, nil
	}

	userEntities, err := client.User.Query().
		Where(userNormalizedEmailPredicate(email)).
		Order(dbent.Asc(dbuser.FieldID)).
		All(ctx)
	if err != nil {
		return nil, infraerrors.InternalServer("COMPAT_EMAIL_LOOKUP_FAILED", "failed to look up compat email user").WithCause(err)
	}
	switch len(userEntities) {
	case 0:
		return nil, nil
	case 1:
		return userEntities[0], nil
	default:
		return nil, infraerrors.Conflict("USER_EMAIL_CONFLICT", "normalized email matched multiple users")
	}
}

// ─── createDingTalkOAuthChoicePendingSession ───────────────────────────────

// createDingTalkOAuthChoicePendingSession 创建 DingTalk OAuth 三方注册/绑定的 choice pending session。
// signupBlocked=true 时关闭"创建新账户"出口；若同时没有 compat email 匹配的已有账户，
// 直接把 step 切到 bind_login_required，避免前端展示一个没有实际可点选项的 choice 界面。
func (h *AuthHandler) createDingTalkOAuthChoicePendingSession(
	c *gin.Context,
	identity service.PendingAuthIdentityKey,
	suggestedEmail string,
	resolvedEmail string,
	redirectTo string,
	browserSessionKey string,
	upstreamClaims map[string]any,
	compatEmail string,
	compatEmailUser *dbent.User,
	forceEmailOnSignup bool,
	signupBlocked bool,
) error {
	suggestionEmail := strings.TrimSpace(suggestedEmail)
	canonicalEmail := strings.TrimSpace(resolvedEmail)
	if suggestionEmail == "" {
		suggestionEmail = canonicalEmail
	}

	completionResponse := map[string]any{
		"step":                      oauthPendingChoiceStep,
		"adoption_required":         true,
		"redirect":                  strings.TrimSpace(redirectTo),
		"email":                     suggestionEmail,
		"resolved_email":            canonicalEmail,
		"existing_account_email":    "",
		"existing_account_bindable": false,
		"create_account_allowed":    !signupBlocked,
		"force_email_on_signup":     forceEmailOnSignup,
		"choice_reason":             "third_party_signup",
	}
	if strings.TrimSpace(compatEmail) != "" {
		completionResponse["compat_email"] = strings.TrimSpace(compatEmail)
	}
	resolvedChoiceEmail := suggestionEmail
	if compatEmailUser != nil {
		completionResponse["email"] = strings.TrimSpace(compatEmailUser.Email)
		completionResponse["existing_account_email"] = strings.TrimSpace(compatEmailUser.Email)
		completionResponse["existing_account_bindable"] = true
		completionResponse["choice_reason"] = "compat_email_match"
		resolvedChoiceEmail = strings.TrimSpace(compatEmailUser.Email)
	}
	if forceEmailOnSignup && compatEmailUser == nil {
		completionResponse["choice_reason"] = "force_email_on_signup"
	}
	// 注册被拦：无论是否匹配到 compat email user，都跳过 choice，直接进 bind_login。
	// "开放注册" 关闭 且 "钉钉企业模式豁免" 也关闭时，唯一合法出口是绑定已有账户，
	// 不应该让用户看到"创建新账户"按钮；compat user 命中只是让 bind_login 的邮箱字段预填得更准。
	if signupBlocked {
		completionResponse["step"] = "bind_login_required"
		completionResponse["existing_account_bindable"] = true
		completionResponse["choice_reason"] = "signup_blocked_redirect_to_bind"
	}

	var targetUserID *int64
	if compatEmailUser != nil && compatEmailUser.ID > 0 {
		targetUserID = &compatEmailUser.ID
	}

	return h.createOAuthPendingSession(c, oauthPendingSessionPayload{
		Intent:                 oauthIntentLogin,
		Identity:               identity,
		TargetUserID:           targetUserID,
		ResolvedEmail:          resolvedChoiceEmail,
		RedirectTo:             redirectTo,
		BrowserSessionKey:      browserSessionKey,
		UpstreamIdentityClaims: upstreamClaims,
		CompletionResponse:     completionResponse,
	})
}

// ─── DingTalkOAuthCallback ─────────────────────────────────────────────────

// DingTalkOAuthCallback 处理钉钉授权回调。
// GET /api/v1/auth/oauth/dingtalk/callback?code=...&state=...
func (h *AuthHandler) DingTalkOAuthCallback(c *gin.Context) {
	cfg, cfgErr := h.getDingTalkOAuthConfig(c.Request.Context())
	if cfgErr != nil {
		response.ErrorFrom(c, cfgErr)
		return
	}

	frontendCallback := strings.TrimSpace(cfg.FrontendRedirectURL)
	if frontendCallback == "" {
		frontendCallback = dingTalkOAuthDefaultFrontendCB
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
		clearDingTalkCookie(c, dingTalkOAuthStateCookieName, secureCookie)
		clearDingTalkCookie(c, dingTalkOAuthRedirectCookie, secureCookie)
		clearDingTalkCookie(c, dingTalkOAuthIntentCookieName, secureCookie)
	}()

	expectedState, err := readCookieDecoded(c, dingTalkOAuthStateCookieName)
	if err != nil || state != expectedState {
		redirectOAuthError(c, frontendCallback, "csrf", "state mismatch", "")
		return
	}
	redirectTo, _ := readCookieDecoded(c, dingTalkOAuthRedirectCookie)
	intent, _ := readCookieDecoded(c, dingTalkOAuthIntentCookieName)
	intent = normalizeOAuthIntent(intent)
	browserSessionKey, _ := readOAuthPendingBrowserCookie(c)
	if strings.TrimSpace(browserSessionKey) == "" {
		redirectOAuthError(c, frontendCallback, "missing_browser_session", "missing browser session cookie", "")
		return
	}
	forceEmailOnSignup := h.isForceEmailOnThirdPartySignup(c.Request.Context())

	// ─── 4 步链（Step 1 + Step 2 必须；Step 3/4 按需 + 跨组织降级）───
	client := h.dingTalkClient(cfg)
	userToken, err := client.ExchangeCodeForUserToken(c.Request.Context(), code)
	if err != nil {
		dingTalkUpstreamRedirect(c, frontendCallback, "exchange_code", err)
		return
	}

	// D: corp 校验提前到 Step 1 之后、Step 2 之前，减少不必要的上游调用
	corpID := strings.TrimSpace(userToken.CorpID)
	if !checkDingTalkCorpAllowed(cfg, corpID) {
		// 不在 URL 中透传 corpID，避免内部企业标识泄露给前端
		redirectOAuthError(c, frontendCallback, "corp_rejected", "", "")
		return
	}

	// Step 2: 必须 — UnionID 是全局唯一，作为 subject + 合成邮箱种子；nick 是用户在 App 自设的昵称
	unionID, oauthNick, err := client.GetUnionIdByUserToken(c.Request.Context(), userToken.AccessToken)
	if err != nil {
		dingTalkUpstreamRedirect(c, frontendCallback, "get_union_id", err)
		return
	}

	identityKey := service.PendingAuthIdentityKey{ProviderType: "dingtalk", ProviderKey: "dingtalk", ProviderSubject: unionID}

	// Step 3/4 调用策略由 policy 决定，与 require_email 解耦。
	// policy=internal_only → 必须成功（hard fail），因为 AppType=internal 已保证用户在应用企业。
	// policy=none / "" → 尝试，失败降级（公网场景跨组织用户属正常预期）。
	// require_email 只影响 Step 3/4 结果后的邮箱处理路径，不影响是否调用。
	var staff *DingTalkStaffInfo
	switch cfg.CorpRestrictionPolicy {
	case "internal_only":
		// AppType=internal 已保证用户在应用企业，Step 3/4 必须成功。
		// 失败 = 钉钉 OAPI 故障或应用配置错误，应 hard fail。
		upstreamUserID, errStep3 := client.GetUserIdByUnionId(c.Request.Context(), unionID)
		if errStep3 != nil {
			dingTalkUpstreamRedirect(c, frontendCallback, "get_user_id", errStep3)
			return
		}
		staffInfo, errStep4 := client.GetStaffInfoByUserId(c.Request.Context(), upstreamUserID)
		if errStep4 != nil {
			dingTalkUpstreamRedirect(c, frontendCallback, "get_staff_info", errStep4)
			return
		}
		staff = staffInfo

	default: // "none" or ""
		// 公网登录，跨组织用户 Step 3/4 可能失败（设计预期），尝试调用，失败降级。
		// 即使 require_email=false 也尝试拿 name（用于 upstreamClaims.username），失败就空着。
		upstreamUserID, errStep3 := client.GetUserIdByUnionId(c.Request.Context(), unionID)
		if errStep3 != nil {
			slog.Debug("dingtalk step3 fallback (none/cross-org)",
				"corp_id", corpID, "union_id", unionID, "err", errStep3.Error())
			staff = &DingTalkStaffInfo{}
			break
		}
		staffInfo, errStep4 := client.GetStaffInfoByUserId(c.Request.Context(), upstreamUserID)
		if errStep4 != nil {
			slog.Debug("dingtalk step4 fallback (none/cross-org)",
				"corp_id", corpID, "union_id", unionID, "err", errStep4.Error())
			staff = &DingTalkStaffInfo{}
			break
		}
		staff = staffInfo
	}

	// nick 来自 OIDC /contact/users/me，优先作为钉钉昵称（user/get.nickname 多数为空）。
	if staff != nil && strings.TrimSpace(oauthNick) != "" {
		staff.Nickname = strings.TrimSpace(oauthNick)
	}

	upstreamClaims := buildDingTalkUpstreamClaims(staff, unionID, corpID)

	// ─── S1 主动绑定分支（PR-3 才走到这里）───
	if intent == oauthIntentBindCurrentUser {
		targetUserID, err := h.readOAuthBindUserIDFromCookie(c, dingTalkOAuthBindUserCookieName)
		if err != nil {
			redirectOAuthError(c, frontendCallback, "invalid_state", "invalid bind user cookie", "")
			return
		}
		// policy=none 跨组织用户绑定时 staff.Email=""，用合成邮箱占位（用于 audit log，不用于注册）
		bindResolvedEmail := staff.Email
		if bindResolvedEmail == "" {
			bindResolvedEmail = buildDingTalkSyntheticEmail(unionID)
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
		clearDingTalkCookie(c, dingTalkOAuthBindUserCookieName, secureCookie)
		redirectToFrontendCallback(c, frontendCallback)
		return
	}

	// ─── Level 1：auth_identities hit ───
	if existing, _ := h.findOAuthIdentityUser(c.Request.Context(), identityKey); existing != nil {
		// 身份同步：已登录用户，直接同步（user_id 已知）。
		// 异步执行避免上游钉钉接口（GetStaffInfoByUserId / 部门递归）阻塞登录跳转。
		runDingTalkSyncAsync(c.Request.Context(), func(ctx context.Context) {
			h.syncDingTalkIdentity(ctx, cfg, client, existing.ID, staff, false)
		})
		if err := h.createOAuthPendingSession(c, oauthPendingSessionPayload{
			Intent: oauthIntentLogin, Identity: identityKey, TargetUserID: &existing.ID,
			ResolvedEmail: existing.Email, RedirectTo: redirectTo, BrowserSessionKey: browserSessionKey,
			UpstreamIdentityClaims: upstreamClaims,
			CompletionResponse:     map[string]any{"redirect": redirectTo},
		}); err != nil {
			redirectOAuthError(c, frontendCallback, "session_error", infraerrors.Reason(err), infraerrors.Message(err))
			return
		}
		redirectToFrontendCallback(c, frontendCallback)
		return
	}

	signupBlocked := h.isDingTalkSignupBlocked(c.Request.Context(), cfg)

	// ─── 非命中：require_email=false 走 synthetic email 直接登录 ───
	if !cfg.RequireEmail {
		if signupBlocked {
			// 注册被拦 + 无邮箱可输：唯一出路是绑定已有账户
			if err := h.createOAuthPendingSession(c, oauthPendingSessionPayload{
				Intent: oauthIntentLogin, Identity: identityKey, TargetUserID: nil,
				ResolvedEmail: "", RedirectTo: redirectTo, BrowserSessionKey: browserSessionKey,
				UpstreamIdentityClaims: upstreamClaims,
				CompletionResponse:     dingTalkBindLoginCompletionResponse(redirectTo),
			}); err != nil {
				redirectOAuthError(c, frontendCallback, "session_error", infraerrors.Reason(err), infraerrors.Message(err))
				return
			}
			redirectToFrontendCallback(c, frontendCallback)
			return
		}
		syntheticEmail := buildDingTalkSyntheticEmail(unionID)
		if err := h.createOAuthPendingSession(c, oauthPendingSessionPayload{
			Intent: oauthIntentLogin, Identity: identityKey, TargetUserID: nil,
			ResolvedEmail: syntheticEmail, RedirectTo: redirectTo, BrowserSessionKey: browserSessionKey,
			UpstreamIdentityClaims: upstreamClaims,
			CompletionResponse:     map[string]any{"redirect": redirectTo, "synthetic_email": syntheticEmail},
		}); err != nil {
			redirectOAuthError(c, frontendCallback, "session_error", infraerrors.Reason(err), infraerrors.Message(err))
			return
		}
		redirectToFrontendCallback(c, frontendCallback)
		return
	}

	// ─── require_email=true 且 staff.Email 空 → 补邮箱（默认）或直接 bind_login（注册被拦时） ───
	if staff.Email == "" {
		completionResponse := map[string]any{
			"step":                      "email_completion",
			"requires_email_completion": true,
			"redirect":                  redirectTo,
		}
		if signupBlocked {
			// 注册被全局关闭且未豁免：跳过补邮箱页，直接进 bind_login 让用户输入已有账户
			completionResponse = dingTalkBindLoginCompletionResponse(redirectTo)
		}
		if err := h.createOAuthPendingSession(c, oauthPendingSessionPayload{
			Intent: oauthIntentLogin, Identity: identityKey, TargetUserID: nil,
			ResolvedEmail: "", RedirectTo: redirectTo, BrowserSessionKey: browserSessionKey,
			UpstreamIdentityClaims: upstreamClaims,
			CompletionResponse:     completionResponse,
		}); err != nil {
			redirectOAuthError(c, frontendCallback, "session_error", infraerrors.Reason(err), infraerrors.Message(err))
			return
		}
		redirectToFrontendCallback(c, frontendCallback)
		return
	}

	// ─── L3/L4 有邮箱：统一 choice pending session ───
	var compatEmailUser *dbent.User
	if dingTalkLevelThreeEnabled && staff.Email != "" {
		compatEmailUser, _ = h.findDingTalkCompatEmailUser(c.Request.Context(), staff.Email)
	}
	if err := h.createDingTalkOAuthChoicePendingSession(
		c, identityKey, staff.Email, staff.Email,
		redirectTo, browserSessionKey, upstreamClaims,
		staff.Email, compatEmailUser, forceEmailOnSignup,
		signupBlocked,
	); err != nil {
		redirectOAuthError(c, frontendCallback, "session_error", infraerrors.Reason(err), infraerrors.Message(err))
		return
	}
	redirectToFrontendCallback(c, frontendCallback)
}

func buildDingTalkSyntheticEmail(userID string) string {
	return "dingtalk-" + strings.ToLower(strings.TrimSpace(userID)) + service.DingTalkConnectSyntheticEmailDomain
}

// isDingTalkSignupBlocked 当注册总开关关闭且未开启钉钉企业模式豁免
// （policy=internal_only + dingtalk_connect_bypass_registration=true）时返回 true。
// 镜像 service.AuthService.canBypassRegistrationDisabledForOAuth 用于 OAuth callback
// 早期路由决策：注册被拦 → 跳过补邮箱页直接进 bind_login，避免用户填完表单才报错。
func (h *AuthHandler) isDingTalkSignupBlocked(ctx context.Context, cfg config.DingTalkConnectConfig) bool {
	if h.settingSvc == nil {
		return false
	}
	if h.settingSvc.IsRegistrationEnabled(ctx) {
		return false
	}
	if cfg.BypassRegistration && cfg.CorpRestrictionPolicy == "internal_only" {
		return false
	}
	return true
}

func dingTalkBindLoginCompletionResponse(redirectTo string) map[string]any {
	return map[string]any{
		"step":                      "bind_login_required",
		"existing_account_bindable": true,
		"create_account_allowed":    false,
		"redirect":                  redirectTo,
	}
}

func buildDingTalkUpstreamClaims(staff *DingTalkStaffInfo, unionID, corpID string) map[string]any {
	primaryDeptID := int64(0)
	if len(staff.DeptIDs) > 0 {
		primaryDeptID = staff.DeptIDs[0]
	}
	return map[string]any{
		"email":           staff.Email,
		"username":        staff.Name,
		"nickname":        staff.Nickname,
		"subject":         unionID,      // 与 identityKey.ProviderSubject 保持一致（全局唯一 unionID）
		"corp_user_id":    staff.UserID, // 企业 userid（跨组织时为空），保留作独立字段用于 audit
		"union_id":        unionID,
		"corp_id":         corpID,
		"primary_dept_id": primaryDeptID, // 首个部门 ID，用于 internal_only 同步路径
	}
}

func checkDingTalkCorpAllowed(cfg config.DingTalkConnectConfig, corpID string) bool {
	switch cfg.CorpRestrictionPolicy {
	case "internal_only":
		// 方案 A：完全跳过 corpID 字段校验，由 step 3 `GetUserIdByUnionId` 做真实判定。
		// 原因：钉钉 /v1.0/oauth2/userAccessToken 在部分授权场景（扫码登录、非企业工作台入口）
		// 不会返回 corpId 字段。而 step 3 用本企业 appToken 查 unionId→userId 映射，
		// 跨企业用户会被钉钉拒绝（错误码 60011/60121），mapDingTalkErrorCode 已将其映射回 "corp_rejected"。
		// AppType=internal 已由 ValidateDingTalkConfig 强制保证应用属性。
		return true
	case "none", "":
		return true
	default:
		return false
	}
}

// decideDingTalkStep34Strategy 根据 policy 和 Step 3/4 运行时错误决定处理方式。
// 返回 (proceed bool, fatal bool)：
//   - proceed=true：继续处理（step 成功或降级）
//   - fatal=true：应 hard fail（upstream_error）
//
// 此 helper 从主链中提取，便于 unit test 独立验证策略决策逻辑。
func decideDingTalkStep34Strategy(policy string, stepErr error) (shouldFallback bool, isFatal bool) {
	if stepErr == nil {
		return false, false // 成功，不需要降级
	}
	switch policy {
	case "internal_only":
		return false, true // hard fail：同企业 Step 3/4 必须成功
	case "none", "":
		return true, false // 降级：公网场景跨组织用户失败属正常预期
	default:
		return false, true // 未知 policy，视为 hard fail
	}
}

// mapDingTalkErrorCode 把 DingTalkAPIError 映射到 redirectOAuthError 用的字符串 code
func mapDingTalkErrorCode(err error) string {
	var apiErr *DingTalkAPIError
	if !errors.As(err, &apiErr) {
		return "upstream_error"
	}
	switch apiErr.Code {
	case "60011", "60121":
		return "corp_rejected"
	case "40014", "50015", "88":
		return "upstream_error"
	default:
		return "upstream_error"
	}
}

// dingTalkClient 构造或返回缓存的 client 实例（h-level 单例）。
// 若 cfg 关键字段（ClientID/ClientSecret/TokenURL/UserInfoURL）与已缓存实例不一致，
// 则丢弃旧实例（含 appToken 缓存）并重建，避免管理员改配置后旧凭据持续生效。
func (h *AuthHandler) dingTalkClient(cfg config.DingTalkConnectConfig) *DingTalkClient {
	h.dingTalkClientMu.Lock()
	defer h.dingTalkClientMu.Unlock()
	newCfg := dingTalkClientConfig{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		TokenURL:     cfg.TokenURL,
		UserInfoURL:  cfg.UserInfoURL,
	}
	if h.dingTalkClientInstance == nil || h.dingTalkClientInstance.cfg != newCfg {
		h.dingTalkClientInstance = &DingTalkClient{
			cfg: newCfg,
			// 与 wechat OAuth client 对齐，避免上游网络抖动时请求悬挂。
			httpClient: &http.Client{Timeout: 10 * time.Second},
		}
	}
	return h.dingTalkClientInstance
}

// ─── buildDingTalkAuthorizeURL ─────────────────────────────────────────────

// buildDingTalkAuthorizeURL 根据配置和 state 构建钉钉 OAuth 授权 URL。
func buildDingTalkAuthorizeURL(cfg config.DingTalkConnectConfig, state string) (string, error) {
	base := strings.TrimSpace(cfg.AuthorizeURL)
	if base == "" {
		return "", infraerrors.InternalServer("DINGTALK_AUTHORIZE_URL_EMPTY", "dingtalk authorize_url not configured")
	}
	redirectURI := strings.TrimSpace(cfg.RedirectURL)
	if redirectURI == "" {
		return "", infraerrors.InternalServer("DINGTALK_REDIRECT_URL_EMPTY", "dingtalk redirect_url not configured")
	}

	u, err := url.Parse(base)
	if err != nil {
		return "", infraerrors.InternalServer("DINGTALK_AUTHORIZE_URL_PARSE_FAILED", "failed to parse dingtalk authorize_url").WithCause(err)
	}

	scopes := strings.TrimSpace(cfg.Scopes)
	if scopes == "" {
		scopes = "openid"
	}

	q := u.Query()
	q.Set("client_id", cfg.ClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("response_type", "code")
	q.Set("scope", scopes)
	q.Set("state", state)
	q.Set("prompt", "consent")
	u.RawQuery = q.Encode()

	return u.String(), nil
}

// ─── Complete Registration ─────────────────────────────────────────────────

type completeDingTalkOAuthRequest struct {
	InvitationCode   string `json:"invitation_code" binding:"required"`
	AffCode          string `json:"aff_code,omitempty"`
	AdoptDisplayName *bool  `json:"adopt_display_name,omitempty"`
	AdoptAvatar      *bool  `json:"adopt_avatar,omitempty"`
}

// CompleteDingTalkOAuthRegistration completes a pending OAuth registration by validating
// the invitation code and creating the user account.
// POST /api/v1/auth/oauth/dingtalk/complete-registration
func (h *AuthHandler) CompleteDingTalkOAuthRegistration(c *gin.Context) {
	var req completeDingTalkOAuthRequest
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
	// E: username 空时退到 email local part（跨组织用户没拿到 staff.Name 也能注册）
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
	tokenPair, user, err := h.authService.LoginOrRegisterOAuthWithTokenPair(c.Request.Context(), email, username, req.InvitationCode, req.AffCode, "dingtalk")
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if err := applyPendingOAuthAdoptionAndConsumeSession(c.Request.Context(), client, h.authService, h.userService, session, decision, user.ID); err != nil {
		respondPendingOAuthBindingApplyError(c, err)
		return
	}
	// 新用户注册完成后执行身份同步（user_id 现在已知）。
	// 异步执行避免阻塞 token 响应。
	if completionCfg, cfgErr := h.getDingTalkOAuthConfig(c.Request.Context()); cfgErr == nil {
		dtClient := h.dingTalkClient(completionCfg)
		claims := session.UpstreamIdentityClaims
		runDingTalkSyncAsync(c.Request.Context(), func(ctx context.Context) {
			h.syncDingTalkIdentityFromClaims(ctx, completionCfg, dtClient, user.ID, claims, true)
		})
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

// CreateDingTalkOAuthAccount creates a new user account from a pending DingTalk OAuth session.
// POST /api/v1/auth/oauth/dingtalk/create-account
func (h *AuthHandler) CreateDingTalkOAuthAccount(c *gin.Context) {
	h.createPendingOAuthAccount(c, "dingtalk")
}

// BindDingTalkOAuthLogin 处理已有账户绑定钉钉 OAuth 登录。
// POST /api/v1/auth/oauth/dingtalk/bind-login
func (h *AuthHandler) BindDingTalkOAuthLogin(c *gin.Context) {
	h.bindPendingOAuthLogin(c, "dingtalk")
}

// ─── DingTalk 身份同步 ─────────────────────────────────────────────────────

// runDingTalkSyncAsync 在后台 goroutine 执行钉钉身份同步，避免阻塞登录响应。
// 与请求 ctx 解耦（handler 返回后会被取消），但保留其 values（trace/request id）。
// 固定 30s 超时上限，防止 goroutine 因上游卡顿无限挂起。
func runDingTalkSyncAsync(parent context.Context, fn func(ctx context.Context)) {
	base := context.WithoutCancel(parent)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("dingtalk sync: panic recovered", "panic", r)
			}
		}()
		ctx, cancel := context.WithTimeout(base, 30*time.Second)
		defer cancel()
		fn(ctx)
	}()
}

// syncDingTalkIdentity 在 internal_only 模式下，按三个 sync 开关把钉钉身份信息
// 同步到用户属性表（以及 users.username）。
// 任何错误仅记日志，不中断登录流程（最终一致性）。
func (h *AuthHandler) syncDingTalkIdentity(ctx context.Context, cfg config.DingTalkConnectConfig, client *DingTalkClient, userID int64, staff *DingTalkStaffInfo, syncUsername bool) {
	slog.Info("dingtalk sync: entry",
		"user_id", userID,
		"policy", cfg.CorpRestrictionPolicy,
		"sync_corp_email", cfg.SyncCorpEmail,
		"sync_display_name", cfg.SyncDisplayName,
		"sync_dept", cfg.SyncDept,
		"sync_username", syncUsername,
		"attr_key_email", cfg.SyncCorpEmailAttrKey,
		"attr_key_name", cfg.SyncDisplayNameAttrKey,
		"attr_key_dept", cfg.SyncDeptAttrKey,
		"staff_nil", staff == nil,
	)
	if cfg.CorpRestrictionPolicy != "internal_only" || staff == nil {
		slog.Info("dingtalk sync: skip, not internal_only or staff nil")
		return
	}
	slog.Info("dingtalk sync: staff snapshot",
		"name", staff.Name, "email", staff.Email, "dept_ids", staff.DeptIDs,
	)
	if !cfg.SyncCorpEmail && !cfg.SyncDisplayName && !cfg.SyncDept {
		slog.Info("dingtalk sync: skip, all flags disabled")
		return
	}
	if h.userAttributeService == nil {
		slog.Warn("dingtalk sync: userAttributeService not available, skipping")
		return
	}

	// 仅首次注册时覆盖 users.username（避免每次登录覆盖用户后续手动改过的名字）。
	// dingtalk_name 属性下面单独每次写入企业 name，不受此条件影响。
	if syncUsername && cfg.SyncDisplayName {
		username := strings.TrimSpace(staff.Nickname)
		source := "nickname"
		if username == "" {
			username = strings.TrimSpace(staff.Name)
			source = "name(fallback)"
		}
		if username != "" && h.userService != nil {
			if _, err := h.userService.UpdateProfile(ctx, userID, service.UpdateProfileRequest{Username: &username}); err != nil {
				slog.Warn("dingtalk sync: failed to update username", "user_id", userID, "err", err)
			} else {
				slog.Info("dingtalk sync: username updated (register)", "user_id", userID, "username", username, "source", source)
			}
		}
	}

	// 属性同步（目标 attr key 从 cfg 读取，默认值由 GetDingTalkConnectOAuthConfig 保证非空）
	type syncField struct {
		key   string
		value string
	}
	var fields []syncField

	if cfg.SyncDisplayName && strings.TrimSpace(staff.Name) != "" {
		fields = append(fields, syncField{cfg.SyncDisplayNameAttrKey, strings.TrimSpace(staff.Name)})
	}
	if cfg.SyncCorpEmail && strings.TrimSpace(staff.Email) != "" {
		fields = append(fields, syncField{cfg.SyncCorpEmailAttrKey, strings.TrimSpace(staff.Email)})
	}
	if cfg.SyncDept && len(staff.DeptIDs) > 0 {
		// 跳过根部门 ID=1，找第一个真实子部门；都是根则保留 1（最终写入空字符串覆盖旧值）。
		primaryDeptID := int64(0)
		for _, id := range staff.DeptIDs {
			if id > 1 {
				primaryDeptID = id
				break
			}
		}
		if primaryDeptID == 0 {
			primaryDeptID = staff.DeptIDs[0]
		}
		slog.Info("dingtalk sync: pick primary dept", "user_id", userID, "all_dept_ids", staff.DeptIDs, "primary", primaryDeptID)
		path, err := h.resolveDingTalkDeptPath(ctx, client, primaryDeptID)
		if err != nil {
			slog.Warn("dingtalk sync: failed to resolve dept path", "user_id", userID, "dept_id", primaryDeptID, "err", err)
		} else {
			// path="" 表示公司直属（仅在根部门下），仍写入空串覆盖旧值。
			fields = append(fields, syncField{cfg.SyncDeptAttrKey, path})
		}
	}

	if len(fields) == 0 {
		return
	}

	// 逐 key 查 definition 并 upsert
	for _, f := range fields {
		if err := h.setUserAttributeByKey(ctx, userID, f.key, f.value); err != nil {
			slog.Warn("dingtalk sync: failed to set attribute", "user_id", userID, "key", f.key, "err", err)
		}
	}
}

// syncDingTalkIdentityFromClaims 从 upstreamClaims 恢复 DingTalkStaffInfo 并调用 syncDingTalkIdentity。
// 用于 pending session 完成阶段（complete-registration / create-account / bind-login）。
// syncUsername=true 表示首次注册场景，需要把 nickname 写入 users.username。
func (h *AuthHandler) syncDingTalkIdentityFromClaims(ctx context.Context, cfg config.DingTalkConnectConfig, client *DingTalkClient, userID int64, claims map[string]any, syncUsername bool) {
	staff := dingTalkStaffFromClaims(claims)
	h.syncDingTalkIdentity(ctx, cfg, client, userID, staff, syncUsername)
}

// maybeSyncDingTalkAfterRegistration 在通用 OAuth 注册路径完成后调用。
// 同步 4 个字段：users.username（首次） + dingtalk_name/email/department（每次）。
func (h *AuthHandler) maybeSyncDingTalkAfterRegistration(ctx context.Context, session *dbent.PendingAuthSession, userID int64) {
	h.dispatchDingTalkPendingSync(ctx, session, userID, true)
}

// maybeSyncDingTalkAfterLogin 在通用 OAuth 登录/绑定路径完成后调用。
// 仅刷新 3 个属性（dingtalk_name/email/department），不动 users.username。
func (h *AuthHandler) maybeSyncDingTalkAfterLogin(ctx context.Context, session *dbent.PendingAuthSession, userID int64) {
	h.dispatchDingTalkPendingSync(ctx, session, userID, false)
}

func (h *AuthHandler) dispatchDingTalkPendingSync(ctx context.Context, session *dbent.PendingAuthSession, userID int64, syncUsername bool) {
	if session == nil || userID <= 0 {
		return
	}
	if !strings.EqualFold(strings.TrimSpace(session.ProviderType), "dingtalk") {
		return
	}
	cfg, err := h.getDingTalkOAuthConfig(ctx)
	if err != nil {
		slog.Debug("dingtalk sync: skip post-login sync, config unavailable", "user_id", userID, "err", err.Error())
		return
	}
	client := h.dingTalkClient(cfg)
	claims := session.UpstreamIdentityClaims
	// 异步执行避免阻塞 token 响应。
	runDingTalkSyncAsync(ctx, func(asyncCtx context.Context) {
		h.syncDingTalkIdentityFromClaims(asyncCtx, cfg, client, userID, claims, syncUsername)
	})
}

// dingTalkStaffFromClaims 从 upstreamClaims 重建最小 DingTalkStaffInfo。
func dingTalkStaffFromClaims(claims map[string]any) *DingTalkStaffInfo {
	if claims == nil {
		return &DingTalkStaffInfo{}
	}
	staff := &DingTalkStaffInfo{}
	if v, ok := claims["username"].(string); ok {
		staff.Name = v
	}
	if v, ok := claims["nickname"].(string); ok {
		staff.Nickname = v
	}
	if v, ok := claims["email"].(string); ok {
		staff.Email = v
	}
	if v, ok := claims["corp_user_id"].(string); ok {
		staff.UserID = v
	}
	// primary_dept_id 存为 int64 或 float64（JSON round-trip）
	switch v := claims["primary_dept_id"].(type) {
	case int64:
		if v > 0 {
			staff.DeptIDs = []int64{v}
		}
	case float64:
		if id := int64(v); id > 0 {
			staff.DeptIDs = []int64{id}
		}
	}
	return staff
}

// setUserAttributeByKey 按 attribute key 查找 definition，再 upsert 用户属性值。
// definition 不存在时记 warn 日志跳过（admin 在 settings 保存时已按需 upsert
// 对应 def；缺失意味着 admin 改了 attr key 但未保存 settings，或 def 被手工删除）。
func (h *AuthHandler) setUserAttributeByKey(ctx context.Context, userID int64, key, value string) error {
	def, err := h.userAttributeService.GetDefinitionByKey(ctx, key)
	if err != nil {
		slog.Warn("dingtalk sync: attribute definition not found, skipping", "key", key, "err", err.Error())
		return nil
	}
	if err := h.userAttributeService.UpdateUserAttributes(ctx, userID, []service.UpdateUserAttributeInput{
		{AttributeID: def.ID, Value: value},
	}); err != nil {
		return err
	}
	slog.Info("dingtalk sync: attribute upserted", "user_id", userID, "key", key, "attr_id", def.ID)
	return nil
}

// resolveDingTalkDeptPath 从叶部门递归向上拼 "公司/部门/子部门" 路径字符串。
// 遇 dept_id=1（根）或 parent_id=0 停止。加 visited set 防循环，最多 50 层。
func (h *AuthHandler) resolveDingTalkDeptPath(ctx context.Context, client *DingTalkClient, deptID int64) (string, error) {
	slog.Info("dingtalk sync: resolve dept path start", "dept_id", deptID)
	const maxDepth = 50
	visited := make(map[int64]bool, maxDepth)
	var parts []string

	current := deptID
	for i := 0; i < maxDepth; i++ {
		if current < 1 || visited[current] {
			break
		}
		visited[current] = true

		info, err := client.GetDeptInfo(ctx, current)
		if err != nil {
			return "", fmt.Errorf("get dept info %d: %w", current, err)
		}
		if strings.TrimSpace(info.Name) != "" {
			parts = append([]string{strings.TrimSpace(info.Name)}, parts...)
		}
		// 钉钉根部门 dept_id=1，ParentID 通常为 0；遇到 0 / self 终止避免循环。
		if info.ParentID < 1 || info.ParentID == current {
			break
		}
		current = info.ParentID
	}

	// 去除根组织名（parts[0] 始终是企业全称），仅保留部门层级。
	// 例：["公司","A","B"] → "A/B"；["公司"] → ""（公司直属）。
	if len(parts) > 0 {
		parts = parts[1:]
	}

	return strings.Join(parts, "/"), nil
}
