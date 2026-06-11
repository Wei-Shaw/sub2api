package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/authidentity"
	"github.com/Wei-Shaw/sub2api/ent/authidentitychannel"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/oauth"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

const (
	wechatOAuthCookiePath         = "/api/v1/auth/oauth/wechat"
	wechatOAuthCookieMaxAgeSec    = 10 * 60
	wechatOAuthStateCookieName    = "wechat_oauth_state"
	wechatOAuthRedirectCookieName = "wechat_oauth_redirect"
	wechatOAuthIntentCookieName   = "wechat_oauth_intent"
	wechatOAuthModeCookieName     = "wechat_oauth_mode"
	wechatOAuthBindUserCookieName = "wechat_oauth_bind_user"
	wechatOAuthDefaultRedirectTo  = "/dashboard"
	wechatOAuthDefaultFrontendCB  = "/auth/wechat/callback"
	wechatOAuthProviderKey        = "wechat-main"
	wechatOAuthLegacyProviderKey  = "wechat"
	wechatPaymentOAuthCookiePath  = "/api/v1/auth/oauth/wechat/payment"
	wechatPaymentOAuthStateName   = "wechat_payment_oauth_state"
	wechatPaymentOAuthRedirect    = "wechat_payment_oauth_redirect"
	wechatPaymentOAuthContextName = "wechat_payment_oauth_context"
	wechatPaymentOAuthScope       = "wechat_payment_oauth_scope"
	wechatPaymentOAuthDefaultTo   = "/purchase"
	wechatPaymentOAuthFrontendCB  = "/auth/wechat/payment/callback"

	wechatOAuthIntentLogin      = "login"
	wechatOAuthIntentBind       = "bind_current_user"
	wechatOAuthIntentAdoptEmail = "adopt_existing_user_by_email"
)

var (
	wechatOAuthAccessTokenURL = "https://api.weixin.qq.com/sns/oauth2/access_token"
	wechatOAuthUserInfoURL    = "https://api.weixin.qq.com/sns/userinfo"
)

type wechatOAuthConfig struct {
	mode             string
	appID            string
	appSecret        string
	authorizeURL     string
	scope            string
	redirectURI      string
	frontendCallback string
	openEnabled      bool
	mpEnabled        bool
REDACTED

type wechatOAuthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	OpenID       string `json:"openid"`
	Scope        string `json:"scope"`
	UnionID      string `json:"unionid"`
	ErrCode      int64  `json:"errcode"`
	ErrMsg       string `json:"errmsg"`
REDACTED

type wechatOAuthUserInfoResponse struct {
	OpenID     string `json:"openid"`
	Nickname   string `json:"nickname"`
	HeadImgURL string `json:"headimgurl"`
	UnionID    string `json:"unionid"`
	ErrCode    int64  `json:"errcode"`
	ErrMsg     string `json:"errmsg"`
REDACTED

type wechatPaymentOAuthContext struct {
	PaymentType string `json:"payment_type"`
	Amount      string `json:"amount,omitempty"`
	OrderType   string `json:"order_type,omitempty"`
	PlanID      int64  `json:"plan_id,omitempty"`
REDACTED

// WeChatOAuthStart starts the WeChat OAuth login flow and stores the short-lived
// browser cookies required by the rebuild pending-auth bridge.
func (h *AuthHandler) WeChatOAuthStart(c *gin.Context) {
	cfg, err := h.getWeChatOAuthConfig(c.Request.Context(), c.Query("mode"), c)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	state, err := oauth.GenerateState()
	if err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_STATE_GEN_FAILED", "failed to generate oauth state").WithCause(err))
		return
REDACTED

	redirectTo := sanitizeFrontendRedirectPath(c.Query("redirect"))
	if redirectTo == "" {
		redirectTo = wechatOAuthDefaultRedirectTo
REDACTED

	browserSessionKey, err := generateOAuthPendingBrowserSession()
	if err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_BROWSER_SESSION_GEN_FAILED", "failed to generate oauth browser session").WithCause(err))
		return
REDACTED

	intent := normalizeWeChatOAuthIntent(c.Query("intent"))
	secureCookie := isRequestHTTPS(c)
	wechatSetCookie(c, wechatOAuthStateCookieName, encodeCookieValue(state), wechatOAuthCookieMaxAgeSec, secureCookie)
	wechatSetCookie(c, wechatOAuthRedirectCookieName, encodeCookieValue(redirectTo), wechatOAuthCookieMaxAgeSec, secureCookie)
	wechatSetCookie(c, wechatOAuthIntentCookieName, encodeCookieValue(intent), wechatOAuthCookieMaxAgeSec, secureCookie)
	wechatSetCookie(c, wechatOAuthModeCookieName, encodeCookieValue(cfg.mode), wechatOAuthCookieMaxAgeSec, secureCookie)
	captureOAuthPromoCode(c, secureCookie)
	setOAuthPendingBrowserCookie(c, browserSessionKey, secureCookie)
	clearOAuthPendingSessionCookie(c, secureCookie)
	if intent == oauthIntentBindCurrentUser {
		bindCookieValue, err := h.buildOAuthBindUserCookieFromContext(c)
		if err != nil {
			response.ErrorFrom(c, err)
			return
	REDACTED
		wechatSetCookie(c, wechatOAuthBindUserCookieName, encodeCookieValue(bindCookieValue), wechatOAuthCookieMaxAgeSec, secureCookie)
REDACTED else {
		wechatClearCookie(c, wechatOAuthBindUserCookieName, secureCookie)
REDACTED

	authURL, err := buildWeChatAuthorizeURL(cfg, state)
	if err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_BUILD_URL_FAILED", "failed to build oauth authorization url").WithCause(err))
		return
REDACTED

	c.Redirect(http.StatusFound, authURL)
REDACTED

// WeChatOAuthCallback exchanges the code with WeChat, resolves openid/unionid,
// and stores the result in the unified pending-auth flow.
func (h *AuthHandler) WeChatOAuthCallback(c *gin.Context) {
	frontendCallback := h.wechatOAuthFrontendCallback(c.Request.Context())

	if providerErr := strings.TrimSpace(c.Query("error")); providerErr != "" {
		redirectOAuthError(c, frontendCallback, "provider_error", providerErr, c.Query("error_description"))
		return
REDACTED

	code := strings.TrimSpace(c.Query("code"))
	state := strings.TrimSpace(c.Query("state"))
	if code == "" || state == "" {
		redirectOAuthError(c, frontendCallback, "missing_params", "missing code/state", "")
		return
REDACTED

	secureCookie := isRequestHTTPS(c)
	defer func() {
		wechatClearCookie(c, wechatOAuthStateCookieName, secureCookie)
		wechatClearCookie(c, wechatOAuthRedirectCookieName, secureCookie)
		wechatClearCookie(c, wechatOAuthIntentCookieName, secureCookie)
		wechatClearCookie(c, wechatOAuthModeCookieName, secureCookie)
		wechatClearCookie(c, wechatOAuthBindUserCookieName, secureCookie)
		clearOAuthPromoCodeCookie(c, secureCookie)
REDACTED()

	expectedState, err := readCookieDecoded(c, wechatOAuthStateCookieName)
	if err != nil || expectedState == "" || state != expectedState {
		redirectOAuthError(c, frontendCallback, "invalid_state", "invalid oauth state", "")
		return
REDACTED

	redirectTo, _ := readCookieDecoded(c, wechatOAuthRedirectCookieName)
	redirectTo = sanitizeFrontendRedirectPath(redirectTo)
	if redirectTo == "" {
		redirectTo = wechatOAuthDefaultRedirectTo
REDACTED
	browserSessionKey, _ := readOAuthPendingBrowserCookie(c)
	if strings.TrimSpace(browserSessionKey) == "" {
		redirectOAuthError(c, frontendCallback, "missing_browser_session", "missing oauth browser session", "")
		return
REDACTED

	intent, _ := readCookieDecoded(c, wechatOAuthIntentCookieName)
	mode, err := readCookieDecoded(c, wechatOAuthModeCookieName)
	if err != nil || strings.TrimSpace(mode) == "" {
		redirectOAuthError(c, frontendCallback, "invalid_state", "missing oauth mode", "")
		return
REDACTED

	cfg, err := h.getWeChatOAuthConfig(c.Request.Context(), mode, c)
	if err != nil {
		redirectOAuthError(c, frontendCallback, "provider_error", infraerrors.Reason(err), infraerrors.Message(err))
		return
REDACTED

	tokenResp, userInfo, err := fetchWeChatOAuthIdentity(c.Request.Context(), cfg, code)
	if err != nil {
		redirectOAuthError(c, frontendCallback, "provider_error", "wechat_identity_fetch_failed", singleLine(err.Error()))
		return
REDACTED

	unionid := strings.TrimSpace(firstNonEmpty(userInfo.UnionID, tokenResp.UnionID))
	openid := strings.TrimSpace(firstNonEmpty(userInfo.OpenID, tokenResp.OpenID))
	providerSubject := unionid
	if providerSubject == "" {
		if cfg.requiresUnionID() {
			redirectOAuthError(c, frontendCallback, "provider_error", "wechat_missing_unionid", "")
			return
	REDACTED
		providerSubject = openid
REDACTED
	if providerSubject == "" {
		redirectOAuthError(c, frontendCallback, "provider_error", "wechat_missing_unionid", "")
		return
REDACTED

	username := firstNonEmpty(userInfo.Nickname, wechatFallbackUsername(providerSubject))
	email := wechatSyntheticEmail(providerSubject)
	upstreamClaims := map[string]any{
		"email":                  email,
		"username":               username,
		"subject":                providerSubject,
		"openid":                 openid,
		"unionid":                unionid,
		"mode":                   cfg.mode,
		"channel":                cfg.mode,
		"channel_app_id":         strings.TrimSpace(cfg.appID),
		"channel_subject":        openid,
		"suggested_display_name": strings.TrimSpace(userInfo.Nickname),
		"suggested_avatar_url":   strings.TrimSpace(userInfo.HeadImgURL),
REDACTED
	identityRef := service.PendingAuthIdentityKey{
		ProviderType:    "wechat",
		ProviderKey:     wechatOAuthProviderKey,
		ProviderSubject: providerSubject,
REDACTED

	normalizedIntent := normalizeWeChatOAuthIntent(intent)
	if normalizedIntent == wechatOAuthIntentBind {
		if err := h.createWeChatBindPendingSession(c, cfg, providerSubject, openid, redirectTo, browserSessionKey, upstreamClaims); err != nil {
			switch infraerrors.Code(err) {
			case http.StatusConflict:
				redirectOAuthError(c, frontendCallback, "ownership_conflict", infraerrors.Reason(err), infraerrors.Message(err))
			case http.StatusUnauthorized, http.StatusForbidden:
				redirectOAuthError(c, frontendCallback, "auth_required", infraerrors.Reason(err), infraerrors.Message(err))
			default:
				redirectOAuthError(c, frontendCallback, "session_error", infraerrors.Reason(err), infraerrors.Message(err))
		REDACTED
			return
	REDACTED
		redirectToFrontendCallback(c, frontendCallback)
		return
REDACTED

	existingIdentityUser, err := h.findOAuthIdentityUser(c.Request.Context(), identityRef)
	if err != nil {
		redirectOAuthError(c, frontendCallback, "session_error", infraerrors.Reason(err), infraerrors.Message(err))
		return
REDACTED
	if existingIdentityUser == nil {
		existingIdentityUser, err = h.findWeChatUserByLegacyOpenID(c.Request.Context(), identityRef, cfg, openid)
		if err != nil {
			redirectOAuthError(c, frontendCallback, "session_error", infraerrors.Reason(err), infraerrors.Message(err))
			return
	REDACTED
REDACTED
	if existingIdentityUser != nil {
		if err := h.ensureWeChatRuntimeIdentityBinding(c.Request.Context(), existingIdentityUser.ID, identityRef, upstreamClaims); err != nil {
			redirectOAuthError(c, frontendCallback, "session_error", infraerrors.Reason(err), infraerrors.Message(err))
			return
	REDACTED
		if err := h.createWeChatPendingSession(c, normalizedIntent, providerSubject, existingIdentityUser.Email, redirectTo, browserSessionKey, upstreamClaims, nil, nil, &existingIdentityUser.ID); err != nil {
			redirectOAuthError(c, frontendCallback, "session_error", "failed to continue oauth login", "")
			return
	REDACTED
		redirectToFrontendCallback(c, frontendCallback)
		return
REDACTED

	if h.isForceEmailOnThirdPartySignup(c.Request.Context()) {
		if err := h.createWeChatChoicePendingSession(
			c,
			identityRef,
			email,
			email,
			redirectTo,
			browserSessionKey,
			upstreamClaims,
			"",
			nil,
			true,
		); err != nil {
			redirectOAuthError(c, frontendCallback, "session_error", "failed to continue oauth login", "")
			return
	REDACTED
		redirectToFrontendCallback(c, frontendCallback)
		return
REDACTED

	if err := h.createWeChatChoicePendingSession(
		c,
		identityRef,
		email,
		email,
		redirectTo,
		browserSessionKey,
		upstreamClaims,
		"",
		nil,
		false,
	); err != nil {
		redirectOAuthError(c, frontendCallback, "session_error", "failed to continue oauth login", "")
		return
REDACTED
	redirectToFrontendCallback(c, frontendCallback)
REDACTED

// WeChatPaymentOAuthStart starts the WeChat payment OAuth flow.
// GET /api/v1/auth/oauth/wechat/payment/start?payment_type=wxpay&redirect=/purchase
func (h *AuthHandler) WeChatPaymentOAuthStart(c *gin.Context) {
	cfg, err := h.getWeChatOAuthConfig(c.Request.Context(), "mp", c)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	paymentType := normalizeWeChatPaymentType(c.Query("payment_type"))
	if paymentType == "" {
		response.BadRequest(c, "Invalid payment type")
		return
REDACTED

	state, err := oauth.GenerateState()
	if err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_STATE_GEN_FAILED", "failed to generate oauth state").WithCause(err))
		return
REDACTED

	redirectTo := normalizeWeChatPaymentRedirectPath(sanitizeFrontendRedirectPath(c.Query("redirect")))
	if redirectTo == "" {
		redirectTo = wechatPaymentOAuthDefaultTo
REDACTED
	rawContext, err := encodeWeChatPaymentOAuthContext(wechatPaymentOAuthContext{
		PaymentType: paymentType,
		Amount:      strings.TrimSpace(c.Query("amount")),
		OrderType:   strings.TrimSpace(c.Query("order_type")),
		PlanID:      parseWeChatPaymentPlanID(c.Query("plan_id")),
REDACTED)
	if err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_CONTEXT_ENCODE_FAILED", "failed to encode oauth context").WithCause(err))
		return
REDACTED

	scope := normalizeWeChatPaymentScope(c.Query("scope"))
	secureCookie := isRequestHTTPS(c)
	wechatPaymentSetCookie(c, wechatPaymentOAuthStateName, encodeCookieValue(state), wechatOAuthCookieMaxAgeSec, secureCookie)
	wechatPaymentSetCookie(c, wechatPaymentOAuthRedirect, encodeCookieValue(redirectTo), wechatOAuthCookieMaxAgeSec, secureCookie)
	wechatPaymentSetCookie(c, wechatPaymentOAuthContextName, encodeCookieValue(rawContext), wechatOAuthCookieMaxAgeSec, secureCookie)
	wechatPaymentSetCookie(c, wechatPaymentOAuthScope, encodeCookieValue(scope), wechatOAuthCookieMaxAgeSec, secureCookie)

	cfg.redirectURI = h.resolveWeChatPaymentOAuthCallbackURL(c.Request.Context(), c)
	cfg.scope = scope
	authURL, err := buildWeChatAuthorizeURL(cfg, state)
	if err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_BUILD_URL_FAILED", "failed to build oauth authorization url").WithCause(err))
		return
REDACTED

	c.Redirect(http.StatusFound, authURL)
REDACTED

// WeChatPaymentOAuthCallback exchanges a payment OAuth code for an OpenID and
// forwards the browser back to the frontend callback route.
func (h *AuthHandler) WeChatPaymentOAuthCallback(c *gin.Context) {
	frontendCallback := wechatPaymentOAuthFrontendCB

	if providerErr := strings.TrimSpace(c.Query("error")); providerErr != "" {
		redirectOAuthError(c, frontendCallback, "provider_error", providerErr, c.Query("error_description"))
		return
REDACTED

	code := strings.TrimSpace(c.Query("code"))
	state := strings.TrimSpace(c.Query("state"))
	if code == "" || state == "" {
		redirectOAuthError(c, frontendCallback, "missing_params", "missing code/state", "")
		return
REDACTED

	secureCookie := isRequestHTTPS(c)
	defer func() {
		wechatPaymentClearCookie(c, wechatPaymentOAuthStateName, secureCookie)
		wechatPaymentClearCookie(c, wechatPaymentOAuthRedirect, secureCookie)
		wechatPaymentClearCookie(c, wechatPaymentOAuthContextName, secureCookie)
		wechatPaymentClearCookie(c, wechatPaymentOAuthScope, secureCookie)
REDACTED()

	expectedState, err := readCookieDecoded(c, wechatPaymentOAuthStateName)
	if err != nil || expectedState == "" || state != expectedState {
		redirectOAuthError(c, frontendCallback, "invalid_state", "invalid oauth state", "")
		return
REDACTED

	redirectTo, _ := readCookieDecoded(c, wechatPaymentOAuthRedirect)
	redirectTo = normalizeWeChatPaymentRedirectPath(sanitizeFrontendRedirectPath(redirectTo))
	if redirectTo == "" {
		redirectTo = wechatPaymentOAuthDefaultTo
REDACTED

	rawContext, _ := readCookieDecoded(c, wechatPaymentOAuthContextName)
	paymentContext, err := decodeWeChatPaymentOAuthContext(rawContext)
	if err != nil {
		redirectOAuthError(c, frontendCallback, "invalid_context", "invalid oauth context", "")
		return
REDACTED
	if paymentContext.PaymentType == "" {
		paymentContext.PaymentType = payment.TypeWxpay
REDACTED

	scope, _ := readCookieDecoded(c, wechatPaymentOAuthScope)
	scope = normalizeWeChatPaymentScope(scope)

	cfg, err := h.getWeChatOAuthConfig(c.Request.Context(), "mp", c)
	if err != nil {
		redirectOAuthError(c, frontendCallback, "provider_error", infraerrors.Reason(err), infraerrors.Message(err))
		return
REDACTED
	cfg.redirectURI = h.resolveWeChatPaymentOAuthCallbackURL(c.Request.Context(), c)
	tokenResp, err := exchangeWeChatOAuthCode(c.Request.Context(), cfg, code)
	if err != nil {
		redirectOAuthError(c, frontendCallback, "token_exchange_failed", "failed to exchange oauth code", err.Error())
		return
REDACTED

	openid := strings.TrimSpace(tokenResp.OpenID)
	if openid == "" {
		redirectOAuthError(c, frontendCallback, "missing_openid", "missing openid", "")
		return
REDACTED
	if strings.TrimSpace(tokenResp.Scope) != "" {
		scope = strings.TrimSpace(tokenResp.Scope)
REDACTED

	resumeToken, err := h.wechatPaymentResumeService().CreateWeChatPaymentResumeToken(service.WeChatPaymentResumeClaims{
		OpenID:      openid,
		PaymentType: paymentContext.PaymentType,
		Amount:      paymentContext.Amount,
		OrderType:   paymentContext.OrderType,
		PlanID:      paymentContext.PlanID,
		RedirectTo:  redirectTo,
		Scope:       scope,
REDACTED)
	if err != nil {
		redirectOAuthError(c, frontendCallback, "invalid_context", "failed to encode payment resume context", "")
		return
REDACTED

	fragment := url.Values{REDACTED
	fragment.Set("wechat_resume_token", resumeToken)
	fragment.Set("redirect", redirectTo)
	redirectWithFragment(c, frontendCallback, fragment)
REDACTED

func (h *AuthHandler) wechatPaymentResumeService() *service.PaymentResumeService {
	var legacyKey []byte
	key, err := payment.ProvideEncryptionKey(h.cfg)
	if err == nil {
		legacyKey = []byte(key)
REDACTED
	return service.NewLegacyAwarePaymentResumeService(legacyKey)
REDACTED

type completeWeChatOAuthRequest struct {
	InvitationCode   string `json:"invitation_code" binding:"required"`
	AffCode          string `json:"aff_code,omitempty"`
	AdoptDisplayName *bool  `json:"adopt_display_name,omitempty"`
	AdoptAvatar      *bool  `json:"adopt_avatar,omitempty"`
REDACTED

// CompleteWeChatOAuthRegistration completes a pending WeChat OAuth registration by
// validating the invitation code and consuming the current pending browser session.
// POST /api/v1/auth/oauth/wechat/complete-registration
func (h *AuthHandler) CompleteWeChatOAuthRegistration(c *gin.Context) {
	var req completeWeChatOAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "INVALID_REQUEST", "message": err.Error()REDACTED)
		return
REDACTED

	secureCookie := isRequestHTTPS(c)
	sessionToken, err := readOAuthPendingSessionCookie(c)
	if err != nil {
		clearOAuthPendingSessionCookie(c, secureCookie)
		clearOAuthPendingBrowserCookie(c, secureCookie)
		response.ErrorFrom(c, service.ErrPendingAuthSessionNotFound)
		return
REDACTED
	browserSessionKey, err := readOAuthPendingBrowserCookie(c)
	if err != nil {
		clearOAuthPendingSessionCookie(c, secureCookie)
		clearOAuthPendingBrowserCookie(c, secureCookie)
		response.ErrorFrom(c, service.ErrPendingAuthBrowserMismatch)
		return
REDACTED
	pendingSvc, err := h.pendingIdentityService()
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	session, err := pendingSvc.GetBrowserSession(c.Request.Context(), sessionToken, browserSessionKey)
	if err != nil {
		clearOAuthPendingSessionCookie(c, secureCookie)
		clearOAuthPendingBrowserCookie(c, secureCookie)
		response.ErrorFrom(c, err)
		return
REDACTED
	if err := ensurePendingOAuthCompleteRegistrationSession(session); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	if updatedSession, handled, err := h.legacyCompleteRegistrationSessionStatus(c, session); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED else if handled {
		c.JSON(http.StatusOK, buildPendingOAuthSessionStatusPayload(updatedSession))
		return
REDACTED else {
		session = updatedSession
REDACTED
	if err := h.ensureBackendModeAllowsNewUserLogin(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	email := strings.TrimSpace(session.ResolvedEmail)
	username := pendingSessionStringValue(session.UpstreamIdentityClaims, "username")
	if email == "" || username == "" {
		response.ErrorFrom(c, infraerrors.BadRequest("PENDING_AUTH_SESSION_INVALID", "pending auth registration context is invalid"))
		return
REDACTED

	tokenPair, user, err := h.authService.LoginOrRegisterOAuthWithTokenPairAndPromoCode(
		c.Request.Context(),
		email,
		username,
		req.InvitationCode,
		req.AffCode,
		pendingOAuthPromoCode(session),
		"wechat",
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	decision, err := h.ensurePendingOAuthAdoptionDecision(c, session.ID, oauthAdoptionDecisionRequest{
		AdoptDisplayName: req.AdoptDisplayName,
		AdoptAvatar:      req.AdoptAvatar,
REDACTED)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	if err := applyPendingOAuthAdoption(c.Request.Context(), h.entClient(), h.authService, h.userService, session, decision, &user.ID); err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("PENDING_AUTH_ADOPTION_APPLY_FAILED", "failed to apply oauth profile adoption").WithCause(err))
		return
REDACTED
	h.authService.RecordSuccessfulLogin(c.Request.Context(), user.ID)
	if _, err := pendingSvc.ConsumeBrowserSession(c.Request.Context(), sessionToken, browserSessionKey); err != nil {
		clearOAuthPendingSessionCookie(c, secureCookie)
		clearOAuthPendingBrowserCookie(c, secureCookie)
		response.ErrorFrom(c, err)
		return
REDACTED
	clearOAuthPendingSessionCookie(c, secureCookie)
	clearOAuthPendingBrowserCookie(c, secureCookie)

	c.JSON(http.StatusOK, gin.H{
		"access_token":  tokenPair.AccessToken,
		"refresh_token": tokenPair.RefreshToken,
		"expires_in":    tokenPair.ExpiresIn,
		"token_type":    "Bearer",
REDACTED)
REDACTED

func (h *AuthHandler) createWeChatPendingSession(
	c *gin.Context,
	intent string,
	providerSubject string,
	email string,
	redirectTo string,
	browserSessionKey string,
	upstreamClaims map[string]any,
	tokenPair *service.TokenPair,
	authErr error,
	targetUserID *int64,
) error {
	completionResponse := map[string]any{
		"redirect": redirectTo,
REDACTED
	if authErr != nil {
		if errors.Is(authErr, service.ErrOAuthInvitationRequired) {
			completionResponse["error"] = "invitation_required"
	REDACTED else {
			return authErr
	REDACTED
REDACTED else if tokenPair != nil {
		completionResponse["access_token"] = tokenPair.AccessToken
		completionResponse["refresh_token"] = tokenPair.RefreshToken
		completionResponse["expires_in"] = tokenPair.ExpiresIn
		completionResponse["token_type"] = "Bearer"
REDACTED

	return h.createOAuthPendingSession(c, oauthPendingSessionPayload{
		Intent: intent,
		Identity: service.PendingAuthIdentityKey{
			ProviderType:    "wechat",
			ProviderKey:     wechatOAuthProviderKey,
			ProviderSubject: providerSubject,
	REDACTED,
		TargetUserID:           targetUserID,
		ResolvedEmail:          email,
		RedirectTo:             redirectTo,
		BrowserSessionKey:      browserSessionKey,
		UpstreamIdentityClaims: upstreamClaims,
		CompletionResponse:     completionResponse,
REDACTED)
REDACTED

func (h *AuthHandler) createWeChatChoicePendingSession(
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
) error {
	suggestionEmail := strings.TrimSpace(suggestedEmail)
	canonicalEmail := strings.TrimSpace(resolvedEmail)
	if suggestionEmail == "" {
		suggestionEmail = canonicalEmail
REDACTED

	completionResponse := map[string]any{
		"step":                      oauthPendingChoiceStep,
		"adoption_required":         true,
		"redirect":                  strings.TrimSpace(redirectTo),
		"email":                     suggestionEmail,
		"resolved_email":            canonicalEmail,
		"existing_account_email":    "",
		"existing_account_bindable": false,
		"create_account_allowed":    true,
		"force_email_on_signup":     forceEmailOnSignup,
		"choice_reason":             "third_party_signup",
REDACTED
	if strings.TrimSpace(compatEmail) != "" {
		completionResponse["compat_email"] = strings.TrimSpace(compatEmail)
REDACTED
	if compatEmailUser != nil {
		completionResponse["email"] = strings.TrimSpace(compatEmailUser.Email)
		completionResponse["existing_account_email"] = strings.TrimSpace(compatEmailUser.Email)
		completionResponse["existing_account_bindable"] = true
		completionResponse["choice_reason"] = "compat_email_match"
REDACTED
	if forceEmailOnSignup {
		completionResponse["choice_reason"] = "force_email_on_signup"
REDACTED

	resolvedChoiceEmail := suggestionEmail
	if compatEmailUser != nil {
		resolvedChoiceEmail = strings.TrimSpace(compatEmailUser.Email)
REDACTED

	return h.createOAuthPendingSession(c, oauthPendingSessionPayload{
		Intent:                 oauthIntentLogin,
		Identity:               identity,
		ResolvedEmail:          resolvedChoiceEmail,
		RedirectTo:             redirectTo,
		BrowserSessionKey:      browserSessionKey,
		UpstreamIdentityClaims: upstreamClaims,
		CompletionResponse:     completionResponse,
REDACTED)
REDACTED

func (h *AuthHandler) createWeChatBindPendingSession(
	c *gin.Context,
	cfg wechatOAuthConfig,
	providerSubject string,
	channelSubject string,
	redirectTo string,
	browserSessionKey string,
	upstreamClaims map[string]any,
) error {
	currentUser, err := h.readOAuthBindTargetUser(c, wechatOAuthBindUserCookieName)
	if err != nil {
		return err
REDACTED
	if err := h.ensureWeChatBindOwnership(c.Request.Context(), currentUser.ID, providerSubject, cfg, channelSubject); err != nil {
		return err
REDACTED
	return h.createWeChatPendingSession(
		c,
		wechatOAuthIntentBind,
		providerSubject,
		currentUser.Email,
		redirectTo,
		browserSessionKey,
		upstreamClaims,
		nil,
		nil,
		&currentUser.ID,
	)
REDACTED

func (h *AuthHandler) readOAuthBindTargetUser(c *gin.Context, cookieName string) (*dbent.User, error) {
	client := h.entClient()
	if client == nil {
		return nil, infraerrors.ServiceUnavailable("PENDING_AUTH_NOT_READY", "pending auth service is not ready")
REDACTED
	userID, err := h.readOAuthBindUserIDFromCookie(c, cookieName)
	if err != nil {
		return nil, infraerrors.Unauthorized("AUTH_REQUIRED", "current user is required to bind wechat account")
REDACTED
	userEntity, err := client.User.Get(c.Request.Context(), userID)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, infraerrors.Unauthorized("AUTH_REQUIRED", "current user is required to bind wechat account")
	REDACTED
		return nil, infraerrors.InternalServer("WECHAT_BIND_USER_LOOKUP_FAILED", "failed to load current user").WithCause(err)
REDACTED
	return userEntity, nil
REDACTED

func (h *AuthHandler) ensureWeChatBindOwnership(
	ctx context.Context,
	userID int64,
	providerSubject string,
	cfg wechatOAuthConfig,
	channelSubject string,
) error {
	client := h.entClient()
	if client == nil {
		return infraerrors.ServiceUnavailable("PENDING_AUTH_NOT_READY", "pending auth service is not ready")
REDACTED

	identities, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("wechat"),
			authidentity.ProviderKeyIn(wechatCompatibleProviderKeys(wechatOAuthProviderKey)...),
			authidentity.ProviderSubjectEQ(strings.TrimSpace(providerSubject)),
		).
		All(ctx)
	if err != nil {
		return infraerrors.InternalServer("WECHAT_BIND_LOOKUP_FAILED", "failed to inspect wechat identity ownership").WithCause(err)
REDACTED
	for _, identity := range identities {
		if identity != nil && identity.UserID != userID {
			activeOwner, lookupErr := findActiveUserByID(ctx, client, identity.UserID)
			if lookupErr != nil {
				return lookupErr
		REDACTED
			if activeOwner != nil {
				return infraerrors.Conflict("AUTH_IDENTITY_OWNERSHIP_CONFLICT", "auth identity already belongs to another user")
		REDACTED
	REDACTED
REDACTED

	channelSubject = strings.TrimSpace(channelSubject)
	channelAppID := strings.TrimSpace(cfg.appID)
	if channelSubject == "" || channelAppID == "" {
		return nil
REDACTED

	channels, err := client.AuthIdentityChannel.Query().
		Where(
			authidentitychannel.ProviderTypeEQ("wechat"),
			authidentitychannel.ProviderKeyIn(wechatCompatibleProviderKeys(wechatOAuthProviderKey)...),
			authidentitychannel.ChannelEQ(strings.TrimSpace(cfg.mode)),
			authidentitychannel.ChannelAppIDEQ(channelAppID),
			authidentitychannel.ChannelSubjectEQ(channelSubject),
		).
		WithIdentity().
		All(ctx)
	if err != nil {
		return infraerrors.InternalServer("WECHAT_BIND_CHANNEL_LOOKUP_FAILED", "failed to inspect wechat identity channel ownership").WithCause(err)
REDACTED
	for _, channel := range channels {
		if channel != nil && channel.Edges.Identity != nil && channel.Edges.Identity.UserID != userID {
			activeOwner, lookupErr := findActiveUserByID(ctx, client, channel.Edges.Identity.UserID)
			if lookupErr != nil {
				return lookupErr
		REDACTED
			if activeOwner != nil {
				return infraerrors.Conflict("AUTH_IDENTITY_CHANNEL_OWNERSHIP_CONFLICT", "auth identity channel already belongs to another user")
		REDACTED
	REDACTED
REDACTED
	return nil
REDACTED

func (h *AuthHandler) findWeChatUserByLegacyOpenID(
	ctx context.Context,
	identity service.PendingAuthIdentityKey,
	cfg wechatOAuthConfig,
	openid string,
) (*dbent.User, error) {
	client := h.entClient()
	if client == nil {
		return nil, infraerrors.ServiceUnavailable("PENDING_AUTH_NOT_READY", "pending auth service is not ready")
REDACTED

	providerType := strings.TrimSpace(identity.ProviderType)
	providerSubject := strings.TrimSpace(identity.ProviderSubject)
	providerKeys := wechatCompatibleProviderKeys(identity.ProviderKey)
	if providerSubject != "" {
		records, err := client.AuthIdentity.Query().
			Where(
				authidentity.ProviderTypeEQ(providerType),
				authidentity.ProviderKeyIn(providerKeys...),
				authidentity.ProviderSubjectEQ(providerSubject),
			).
			WithUser().
			All(ctx)
		if err != nil {
			return nil, infraerrors.InternalServer("AUTH_IDENTITY_LOOKUP_FAILED", "failed to inspect auth identity ownership").WithCause(err)
	REDACTED
		if user, err := singleWeChatIdentityUser(records); err != nil || user != nil {
			if err != nil || user == nil {
				return user, err
		REDACTED
			return findActiveUserByID(ctx, client, user.ID)
	REDACTED
REDACTED

	openid = strings.TrimSpace(openid)
	channel := strings.TrimSpace(cfg.mode)
	channelAppID := strings.TrimSpace(cfg.appID)
	if openid != "" && channel != "" && channelAppID != "" {
		records, err := client.AuthIdentityChannel.Query().
			Where(
				authidentitychannel.ProviderTypeEQ(providerType),
				authidentitychannel.ProviderKeyIn(providerKeys...),
				authidentitychannel.ChannelEQ(channel),
				authidentitychannel.ChannelAppIDEQ(channelAppID),
				authidentitychannel.ChannelSubjectEQ(openid),
			).
			WithIdentity(func(q *dbent.AuthIdentityQuery) {
				q.WithUser()
		REDACTED).
			All(ctx)
		if err != nil {
			return nil, infraerrors.InternalServer("AUTH_IDENTITY_CHANNEL_LOOKUP_FAILED", "failed to inspect auth identity channel ownership").WithCause(err)
	REDACTED
		if user, err := singleWeChatChannelUser(records); err != nil || user != nil {
			if err != nil || user == nil {
				return user, err
		REDACTED
			return findActiveUserByID(ctx, client, user.ID)
	REDACTED
REDACTED

	if openid == "" {
		return nil, nil
REDACTED

	records, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ(providerType),
			authidentity.ProviderKeyIn(providerKeys...),
			authidentity.ProviderSubjectEQ(openid),
		).
		WithUser().
		All(ctx)
	if err != nil {
		return nil, infraerrors.InternalServer("AUTH_IDENTITY_LOOKUP_FAILED", "failed to inspect auth identity ownership").WithCause(err)
REDACTED
	user, err := singleWeChatIdentityUser(records)
	if err != nil || user == nil {
		return user, err
REDACTED
	return findActiveUserByID(ctx, client, user.ID)
REDACTED

func wechatCompatibleProviderKeys(providerKey string) []string {
	preferred := strings.TrimSpace(providerKey)
	if preferred == "" {
		preferred = wechatOAuthProviderKey
REDACTED
	keys := []string{preferredREDACTED
	if !strings.EqualFold(preferred, wechatOAuthLegacyProviderKey) {
		keys = append(keys, wechatOAuthLegacyProviderKey)
REDACTED
	return keys
REDACTED

func singleWeChatIdentityUser(records []*dbent.AuthIdentity) (*dbent.User, error) {
	var resolved *dbent.User
	for _, record := range records {
		if record == nil || record.Edges.User == nil {
			continue
	REDACTED
		if resolved == nil {
			resolved = record.Edges.User
			continue
	REDACTED
		if resolved.ID != record.Edges.User.ID {
			return nil, infraerrors.Conflict("AUTH_IDENTITY_OWNERSHIP_CONFLICT", "auth identity already belongs to another user")
	REDACTED
REDACTED
	return resolved, nil
REDACTED

func singleWeChatChannelUser(records []*dbent.AuthIdentityChannel) (*dbent.User, error) {
	var resolved *dbent.User
	for _, record := range records {
		if record == nil || record.Edges.Identity == nil || record.Edges.Identity.Edges.User == nil {
			continue
	REDACTED
		if resolved == nil {
			resolved = record.Edges.Identity.Edges.User
			continue
	REDACTED
		if resolved.ID != record.Edges.Identity.Edges.User.ID {
			return nil, infraerrors.Conflict("AUTH_IDENTITY_CHANNEL_OWNERSHIP_CONFLICT", "auth identity channel already belongs to another user")
	REDACTED
REDACTED
	return resolved, nil
REDACTED

func (h *AuthHandler) ensureWeChatRuntimeIdentityBinding(
	ctx context.Context,
	userID int64,
	identity service.PendingAuthIdentityKey,
	upstreamClaims map[string]any,
) error {
	client := h.entClient()
	if client == nil {
		return infraerrors.ServiceUnavailable("PENDING_AUTH_NOT_READY", "pending auth service is not ready")
REDACTED

	tx, err := client.Tx(ctx)
	if err != nil {
		return infraerrors.InternalServer("AUTH_IDENTITY_BIND_FAILED", "failed to begin wechat identity repair transaction").WithCause(err)
REDACTED
	defer func() { _ = tx.Rollback() REDACTED()

	_, err = ensurePendingOAuthIdentityForUser(dbent.NewTxContext(ctx, tx), tx, &dbent.PendingAuthSession{
		ProviderType:           strings.TrimSpace(identity.ProviderType),
		ProviderKey:            strings.TrimSpace(identity.ProviderKey),
		ProviderSubject:        strings.TrimSpace(identity.ProviderSubject),
		UpstreamIdentityClaims: cloneOAuthMetadata(upstreamClaims),
REDACTED, userID)
	if err != nil {
		return err
REDACTED
	return tx.Commit()
REDACTED

func (h *AuthHandler) getWeChatOAuthConfig(ctx context.Context, rawMode string, c *gin.Context) (wechatOAuthConfig, error) {
	mode, err := resolveWeChatOAuthMode(rawMode, c)
	if err != nil {
		return wechatOAuthConfig{REDACTED, err
REDACTED

	if h == nil || h.settingSvc == nil {
		return wechatOAuthConfig{REDACTED, infraerrors.ServiceUnavailable("CONFIG_NOT_READY", "wechat oauth settings service not ready")
REDACTED

	apiBaseURL := ""
	if h != nil && h.settingSvc != nil {
		settings, err := h.settingSvc.GetAllSettings(ctx)
		if err == nil && settings != nil {
			apiBaseURL = strings.TrimSpace(settings.APIBaseURL)
	REDACTED
REDACTED

	effective, err := h.settingSvc.GetWeChatConnectOAuthConfig(ctx)
	if err != nil {
		return wechatOAuthConfig{REDACTED, err
REDACTED
	if !effective.SupportsMode(mode) {
		return wechatOAuthConfig{REDACTED, infraerrors.NotFound("OAUTH_DISABLED", "wechat oauth is disabled")
REDACTED

	cfg := wechatOAuthConfig{
		mode:             mode,
		appID:            strings.TrimSpace(effective.AppIDForMode(mode)),
		appSecret:        strings.TrimSpace(effective.AppSecretForMode(mode)),
		redirectURI:      firstNonEmpty(strings.TrimSpace(effective.RedirectURL), resolveWeChatOAuthAbsoluteURL(apiBaseURL, c, "/api/v1/auth/oauth/wechat/callback")),
		frontendCallback: firstNonEmpty(strings.TrimSpace(effective.FrontendRedirectURL), wechatOAuthDefaultFrontendCB),
		scope:            effective.ScopeForMode(mode),
		openEnabled:      effective.OpenEnabled,
		mpEnabled:        effective.MPEnabled,
REDACTED

	switch mode {
	case "mp":
		cfg.authorizeURL = "https://open.weixin.qq.com/connect/oauth2/authorize"
	default:
		cfg.authorizeURL = "https://open.weixin.qq.com/connect/qrconnect"
REDACTED
	if strings.TrimSpace(cfg.redirectURI) == "" {
		return wechatOAuthConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "wechat oauth redirect url not configured")
REDACTED

	return cfg, nil
REDACTED

func (cfg wechatOAuthConfig) requiresUnionID() bool {
	return cfg.openEnabled && cfg.mpEnabled
REDACTED

func (h *AuthHandler) wechatOAuthFrontendCallback(ctx context.Context) string {
	if h != nil && h.settingSvc != nil {
		cfg, err := h.settingSvc.GetWeChatConnectOAuthConfig(ctx)
		if err == nil && strings.TrimSpace(cfg.FrontendRedirectURL) != "" {
			return strings.TrimSpace(cfg.FrontendRedirectURL)
	REDACTED
REDACTED
	return wechatOAuthDefaultFrontendCB
REDACTED

func resolveWeChatOAuthMode(rawMode string, c *gin.Context) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(rawMode))
	if mode == "" {
		if isWeChatBrowserRequest(c) {
			return "mp", nil
	REDACTED
		return "open", nil
REDACTED
	if mode != "open" && mode != "mp" {
		return "", infraerrors.BadRequest("INVALID_MODE", "wechat oauth mode must be open or mp")
REDACTED
	return mode, nil
REDACTED

func isWeChatBrowserRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
REDACTED
	return strings.Contains(strings.ToLower(strings.TrimSpace(c.GetHeader("User-Agent"))), "micromessenger")
REDACTED

func normalizeWeChatOAuthIntent(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "login":
		return wechatOAuthIntentLogin
	case "bind", "bind_current_user":
		return wechatOAuthIntentBind
	case "adopt", "adopt_existing_user_by_email":
		return wechatOAuthIntentAdoptEmail
	default:
		return wechatOAuthIntentLogin
REDACTED
REDACTED

func buildWeChatAuthorizeURL(cfg wechatOAuthConfig, state string) (string, error) {
	u, err := url.Parse(cfg.authorizeURL)
	if err != nil {
		return "", fmt.Errorf("parse authorize url: %w", err)
REDACTED
	query := u.Query()
	query.Set("appid", cfg.appID)
	query.Set("redirect_uri", cfg.redirectURI)
	query.Set("response_type", "code")
	query.Set("scope", cfg.scope)
	query.Set("state", state)
	u.RawQuery = query.Encode()
	u.Fragment = "wechat_redirect"
	return u.String(), nil
REDACTED

func resolveWeChatOAuthAbsoluteURL(apiBaseURL string, c *gin.Context, callbackPath string) string {
	callbackPath = strings.TrimSpace(callbackPath)
	if callbackPath == "" {
		return ""
REDACTED

	if raw := strings.TrimSpace(apiBaseURL); raw != "" {
		if parsed, err := url.Parse(raw); err == nil && parsed.Scheme != "" && parsed.Host != "" {
			basePath := strings.TrimRight(parsed.EscapedPath(), "/")
			targetPath := callbackPath
			if basePath != "" && strings.HasSuffix(basePath, "/api/v1") && strings.HasPrefix(callbackPath, "/api/v1") {
				targetPath = basePath + strings.TrimPrefix(callbackPath, "/api/v1")
		REDACTED else if basePath != "" {
				targetPath = basePath + callbackPath
		REDACTED
			return parsed.Scheme + "://" + parsed.Host + targetPath
	REDACTED
REDACTED

	if c == nil || c.Request == nil {
		return ""
REDACTED
	scheme := "http"
	if isRequestHTTPS(c) {
		scheme = "https"
REDACTED
	host := strings.TrimSpace(c.Request.Host)
	if forwardedHost := strings.TrimSpace(c.GetHeader("X-Forwarded-Host")); forwardedHost != "" {
		host = forwardedHost
REDACTED
	if host == "" {
		return ""
REDACTED
	return scheme + "://" + host + callbackPath
REDACTED

func fetchWeChatOAuthIdentity(ctx context.Context, cfg wechatOAuthConfig, code string) (*wechatOAuthTokenResponse, *wechatOAuthUserInfoResponse, error) {
	tokenResp, err := exchangeWeChatOAuthCode(ctx, cfg, code)
	if err != nil {
		return nil, nil, err
REDACTED
	userInfo, err := fetchWeChatUserInfo(ctx, tokenResp)
	if err != nil {
		return nil, nil, err
REDACTED
	return tokenResp, userInfo, nil
REDACTED

func exchangeWeChatOAuthCode(ctx context.Context, cfg wechatOAuthConfig, code string) (*wechatOAuthTokenResponse, error) {
	endpoint, err := url.Parse(wechatOAuthAccessTokenURL)
	if err != nil {
		return nil, fmt.Errorf("parse wechat access token url: %w", err)
REDACTED

	query := endpoint.Query()
	query.Set("appid", cfg.appID)
	query.Set("secret", cfg.appSecret)
	query.Set("code", strings.TrimSpace(code))
	query.Set("grant_type", "authorization_code")
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build wechat access token request: %w", err)
REDACTED

	client := &http.Client{Timeout: 30 * time.SecondREDACTED
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request wechat access token: %w", err)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read wechat access token response: %w", err)
REDACTED
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("wechat access token status=%d", resp.StatusCode)
REDACTED

	var tokenResp wechatOAuthTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("decode wechat access token response: %w", err)
REDACTED
	if tokenResp.ErrCode != 0 {
		return nil, fmt.Errorf("wechat access token error=%d %s", tokenResp.ErrCode, strings.TrimSpace(tokenResp.ErrMsg))
REDACTED
	if strings.TrimSpace(tokenResp.AccessToken) == "" {
		return nil, fmt.Errorf("wechat access token missing access_token")
REDACTED
	return &tokenResp, nil
REDACTED

func fetchWeChatUserInfo(ctx context.Context, tokenResp *wechatOAuthTokenResponse) (*wechatOAuthUserInfoResponse, error) {
	if tokenResp == nil {
		return nil, fmt.Errorf("wechat token response is nil")
REDACTED

	endpoint, err := url.Parse(wechatOAuthUserInfoURL)
	if err != nil {
		return nil, fmt.Errorf("parse wechat userinfo url: %w", err)
REDACTED
	query := endpoint.Query()
	query.Set("access_token", strings.TrimSpace(tokenResp.AccessToken))
	query.Set("openid", strings.TrimSpace(tokenResp.OpenID))
	query.Set("lang", "zh_CN")
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build wechat userinfo request: %w", err)
REDACTED

	client := &http.Client{Timeout: 30 * time.SecondREDACTED
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request wechat userinfo: %w", err)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read wechat userinfo response: %w", err)
REDACTED
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("wechat userinfo status=%d", resp.StatusCode)
REDACTED

	var userInfo wechatOAuthUserInfoResponse
	if err := json.Unmarshal(body, &userInfo); err != nil {
		return nil, fmt.Errorf("decode wechat userinfo response: %w", err)
REDACTED
	if userInfo.ErrCode != 0 {
		return nil, fmt.Errorf("wechat userinfo error=%d %s", userInfo.ErrCode, strings.TrimSpace(userInfo.ErrMsg))
REDACTED
	return &userInfo, nil
REDACTED

func wechatSyntheticEmail(subject string) string {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return ""
REDACTED
	return "wechat-" + subject + service.WeChatConnectSyntheticEmailDomain
REDACTED

func wechatFallbackUsername(subject string) string {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return "wechat_user"
REDACTED
	return "wechat_" + truncateFragmentValue(subject)
REDACTED

func wechatSetCookie(c *gin.Context, name string, value string, maxAgeSec int, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     wechatOAuthCookiePath,
		MaxAge:   maxAgeSec,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
REDACTED)
REDACTED

func wechatClearCookie(c *gin.Context, name string, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     wechatOAuthCookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
REDACTED)
REDACTED

func normalizeWeChatPaymentType(raw string) string {
	switch strings.TrimSpace(raw) {
	case payment.TypeWxpay, payment.TypeWxpayDirect:
		return strings.TrimSpace(raw)
	default:
		return ""
REDACTED
REDACTED

func normalizeWeChatPaymentScope(raw string) string {
	for _, part := range strings.FieldsFunc(strings.TrimSpace(raw), func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
REDACTED) {
		switch strings.TrimSpace(part) {
		case "snsapi_userinfo":
			return "snsapi_userinfo"
		case "snsapi_base":
			return "snsapi_base"
	REDACTED
REDACTED
	return "snsapi_base"
REDACTED

func normalizeWeChatPaymentRedirectPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return wechatPaymentOAuthDefaultTo
REDACTED
	if path == "/payment" {
		return "/purchase"
REDACTED
	if strings.HasPrefix(path, "/payment?") {
		return "/purchase" + strings.TrimPrefix(path, "/payment")
REDACTED
	return path
REDACTED

func (h *AuthHandler) resolveWeChatPaymentOAuthCallbackURL(ctx context.Context, c *gin.Context) string {
	apiBaseURL := ""
	if h != nil && h.settingSvc != nil {
		if settings, err := h.settingSvc.GetAllSettings(ctx); err == nil && settings != nil {
			apiBaseURL = strings.TrimSpace(settings.APIBaseURL)
	REDACTED
REDACTED
	return resolveWeChatOAuthAbsoluteURL(apiBaseURL, c, "/api/v1/auth/oauth/wechat/payment/callback")
REDACTED

func encodeWeChatPaymentOAuthContext(ctx wechatPaymentOAuthContext) (string, error) {
	data, err := json.Marshal(ctx)
	if err != nil {
		return "", err
REDACTED
	return string(data), nil
REDACTED

func decodeWeChatPaymentOAuthContext(raw string) (wechatPaymentOAuthContext, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return wechatPaymentOAuthContext{REDACTED, nil
REDACTED
	var ctx wechatPaymentOAuthContext
	if err := json.Unmarshal([]byte(raw), &ctx); err != nil {
		return wechatPaymentOAuthContext{REDACTED, err
REDACTED
	return ctx, nil
REDACTED

func parseWeChatPaymentPlanID(raw string) int64 {
	id, _ := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	return id
REDACTED

func wechatPaymentSetCookie(c *gin.Context, name string, value string, maxAgeSec int, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     wechatPaymentOAuthCookiePath,
		MaxAge:   maxAgeSec,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
REDACTED)
REDACTED

func wechatPaymentClearCookie(c *gin.Context, name string, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     wechatPaymentOAuthCookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
REDACTED)
REDACTED
