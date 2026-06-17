package handler

import (
	"context"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/oauth"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

const (
	weComMobileOAuthSessionTTL      = 10 * time.Minute
	weComMobileOAuthPollIntervalMS  = 2000
	weComMobileOAuthCallbackPath    = "/api/v1/auth/oauth/wecom/mobile/callback"
	weComMobileOAuthStateKey        = "wecom_mobile_oauth_state_hash"
	weComMobileOAuthStatusKey       = "wecom_mobile_oauth_status"
	weComMobileOAuthErrorCodeKey    = "wecom_mobile_oauth_error_code"
	weComMobileOAuthErrorMessageKey = "wecom_mobile_oauth_error_message"
	weComMobileOAuthPrivateInfoKey  = "wecom_mobile_oauth_privateinfo_required"
	weComMobileOAuthStatusPending   = "pending"
	weComMobileOAuthStatusCompleted = "completed"
	weComMobileOAuthStatusFailed    = "failed"
	weComMobileOAuthMode            = "mobile_oauth2"
)

type weComMobileOAuthStartRequest struct {
	Redirect  string `json:"redirect"`
	Intent    string `json:"intent"`
	PromoCode string `json:"promo_code"`
}

type weComMobilePendingSessionInput struct {
	state             string
	browserSessionKey string
	intent            string
	redirectTo        string
	authorizeURL      string
	promoCode         string
	targetUserID      *int64
}

func (h *AuthHandler) WeComMobileOAuthStart(c *gin.Context) {
	var req weComMobileOAuthStartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
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

	redirectTo := sanitizeFrontendRedirectPath(req.Redirect)
	if redirectTo == "" {
		redirectTo = weComOAuthDefaultRedirectTo
	}
	intent := normalizeWeChatOAuthIntent(req.Intent)
	targetUserID, err := h.resolveWeComMobileBindTarget(c, intent)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	authURL, err := buildWeComMobileAuthorizeURL(c, cfg, state)
	if err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_BUILD_URL_FAILED", "failed to build oauth authorization url").WithCause(err))
		return
	}

	session, err := h.createWeComMobilePendingSession(c.Request.Context(), weComMobilePendingSessionInput{
		state:             state,
		browserSessionKey: browserSessionKey,
		intent:            intent,
		redirectTo:        redirectTo,
		authorizeURL:      authURL,
		promoCode:         req.PromoCode,
		targetUserID:      targetUserID,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	setOAuthPendingBrowserCookie(c, browserSessionKey, isRequestHTTPS(c))
	setOAuthPendingSessionCookie(c, session.SessionToken, isRequestHTTPS(c))

	response.Success(c, gin.H{
		"session_id":       session.SessionToken,
		"authorize_url":    authURL,
		"expires_at":       session.ExpiresAt.UTC().Format(time.RFC3339),
		"poll_interval_ms": weComMobileOAuthPollIntervalMS,
	})
}

func (h *AuthHandler) WeComMobileOAuthStatus(c *gin.Context) {
	sessionToken := strings.TrimSpace(c.Query("session_id"))
	if sessionToken == "" {
		response.ErrorFrom(c, service.ErrPendingAuthSessionNotFound)
		return
	}
	browserSessionKey, err := readOAuthPendingBrowserCookie(c)
	if err != nil || strings.TrimSpace(browserSessionKey) == "" {
		response.ErrorFrom(c, service.ErrPendingAuthBrowserMismatch)
		return
	}
	session, err := h.loadWeComMobileStatusSession(c.Request.Context(), sessionToken, browserSessionKey)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, buildWeComMobileStatusPayload(session))
}

func (h *AuthHandler) WeComMobileOAuthCallback(c *gin.Context) {
	code := strings.TrimSpace(c.Query("code"))
	state := strings.TrimSpace(c.Query("state"))
	if code == "" || state == "" {
		h.renderWeComMobileCallbackPage(c, false, "missing code/state")
		return
	}

	session, err := h.loadWeComMobileCallbackSession(c.Request.Context(), state)
	if err != nil {
		h.renderWeComMobileCallbackPage(c, false, infraerrors.Message(err))
		return
	}
	if err := h.completeWeComMobileOAuth(c.Request.Context(), session, code); err != nil {
		_ = h.markWeComMobileSessionFailed(c.Request.Context(), session, infraerrors.Reason(err), infraerrors.Message(err))
		h.renderWeComMobileCallbackPage(c, false, infraerrors.Message(err))
		return
	}
	h.renderWeComMobileCallbackPage(c, true, "")
}

func (h *AuthHandler) resolveWeComMobileBindTarget(c *gin.Context, intent string) (*int64, error) {
	if intent != wechatOAuthIntentBind {
		return nil, nil
	}
	userID, err := h.resolveOAuthBindTargetUserID(c)
	if err != nil || userID == nil || *userID <= 0 {
		return nil, err
	}
	return userID, nil
}

func (h *AuthHandler) completeWeComMobileOAuth(ctx context.Context, session *dbent.PendingAuthSession, code string) error {
	cfg, err := h.getWeComOAuthConfig(ctx)
	if err != nil {
		return err
	}
	userInfo, err := fetchWeComOAuthIdentity(ctx, cfg, code)
	if err != nil {
		return infraerrors.InternalServer("WECOM_IDENTITY_FETCH_FAILED", "wecom identity fetch failed").WithCause(err)
	}
	if userInfo.normalizedUserID() == "" {
		return infraerrors.BadRequest("WECOM_MEMBER_REQUIRED", "wecom member identity is required")
	}

	cfg.scope = "snsapi_privateinfo"
	resolved := h.resolveWeComOAuthIdentity(ctx, cfg, userInfo, weComMobileOAuthMode)
	if strings.TrimSpace(session.Intent) == wechatOAuthIntentBind {
		return h.completeWeComMobileBind(ctx, session, resolved)
	}
	if h.weComLoginPanelNeedsMobilePrivateInfo(ctx, session, resolved) {
		return h.markWeComMobilePrivateInfoRequired(ctx, session)
	}
	return h.completeWeComMobileLogin(ctx, session, resolved)
}

func (h *AuthHandler) weComLoginPanelNeedsMobilePrivateInfo(ctx context.Context, session *dbent.PendingAuthSession, resolved weComResolvedOAuthIdentity) bool {
	if pendingSessionBoolValue(session.LocalFlowState, weComMobileOAuthPrivateInfoKey) {
		return false
	}
	if resolved.emailSource != "synthetic" || strings.TrimSpace(resolved.email) == "" {
		return false
	}
	user, err := h.findOAuthIdentityUser(ctx, resolved.identityRef)
	return err == nil && user == nil
}

func (h *AuthHandler) markWeComMobilePrivateInfoRequired(ctx context.Context, session *dbent.PendingAuthSession) error {
	client := h.entClient()
	if client == nil {
		return infraerrors.ServiceUnavailable("PENDING_AUTH_NOT_READY", "pending auth service is not ready")
	}
	localState := clonePendingMap(session.LocalFlowState)
	localState[weComMobileOAuthStatusKey] = weComMobileOAuthStatusPending
	localState[weComMobileOAuthPrivateInfoKey] = true
	_, err := client.PendingAuthSession.UpdateOneID(session.ID).SetLocalFlowState(localState).Save(ctx)
	return err
}

func (h *AuthHandler) completeWeComMobileBind(ctx context.Context, session *dbent.PendingAuthSession, resolved weComResolvedOAuthIdentity) error {
	if session.TargetUserID == nil || *session.TargetUserID <= 0 {
		return infraerrors.Unauthorized("AUTH_REQUIRED", "current user is required to bind wecom account")
	}
	if err := h.ensureGenericBindOwnership(ctx, *session.TargetUserID, resolved.identityRef); err != nil {
		return err
	}
	userEntity, err := h.entClient().User.Get(ctx, *session.TargetUserID)
	if err != nil {
		if dbent.IsNotFound(err) {
			return infraerrors.Unauthorized("AUTH_REQUIRED", "current user is required to bind wecom account")
		}
		return infraerrors.InternalServer("WECOM_BIND_USER_LOOKUP_FAILED", "failed to load current user").WithCause(err)
	}
	_, err = h.updateWeComMobilePendingSession(ctx, session, resolved, userEntity.Email, session.TargetUserID, map[string]any{"redirect": session.RedirectTo})
	return err
}

func (h *AuthHandler) completeWeComMobileLogin(ctx context.Context, session *dbent.PendingAuthSession, resolved weComResolvedOAuthIdentity) error {
	if completed, err := h.completeWeComMobileExistingIdentityLogin(ctx, session, resolved); completed || err != nil {
		return err
	}
	if resolved.emailSource == "synthetic" {
		_, err := h.updateWeComMobilePendingSession(ctx, session, resolved, "", nil, buildWeComEmailRequiredResponse(session.RedirectTo))
		return err
	}
	tokenPair, user, authErr := h.authService.LoginOrRegisterVerifiedEmailOAuthWithSignupCodes(
		ctx,
		service.EmailOAuthIdentityInput{
			ProviderType:     "wecom",
			ProviderKey:      weComOAuthProviderKey,
			ProviderSubject:  resolved.providerSubject,
			Email:            resolved.email,
			EmailVerified:    true,
			Username:         resolved.username,
			AvatarURL:        weComResolvedAvatarURL(resolved),
			UpstreamMetadata: resolved.upstreamClaims,
		},
		"",
		"",
		pendingOAuthPromoCode(session),
	)
	var targetUserID *int64
	if user != nil && user.ID > 0 {
		if authErr == nil {
			if err := h.applyWeComResolvedAvatar(ctx, user.ID, resolved); err != nil {
				return err
			}
		}
		targetUserID = &user.ID
	}
	_, err := h.updateWeComMobilePendingSession(ctx, session, resolved, resolved.email, targetUserID, buildWeComCompletionResponse(session.RedirectTo, tokenPair, authErr))
	return err
}

func (h *AuthHandler) completeWeComMobileExistingIdentityLogin(ctx context.Context, session *dbent.PendingAuthSession, resolved weComResolvedOAuthIdentity) (bool, error) {
	userEntity, err := h.findOAuthIdentityUser(ctx, resolved.identityRef)
	if err != nil || userEntity == nil {
		return false, err
	}
	user := entUserToService(userEntity)
	if err := ensureLoginUserActive(user); err != nil {
		return true, err
	}
	if err := h.ensureBackendModeAllowsUser(ctx, user); err != nil {
		return true, err
	}
	tokenPair, err := h.authService.GenerateTokenPair(ctx, user, "")
	if err != nil {
		return true, err
	}
	h.authService.RecordSuccessfulLogin(ctx, user.ID)
	_, err = h.updateWeComMobilePendingSession(ctx, session, resolved, user.Email, &user.ID, buildWeComCompletionResponse(session.RedirectTo, tokenPair, nil))
	return true, err
}
