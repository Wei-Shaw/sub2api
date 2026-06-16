package handler

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/oauth"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/imroc/req/v3"
	"github.com/tidwall/gjson"
)

const (
	oidcOAuthCookiePath         = "/api/v1/auth/oauth/oidc"
	oidcOAuthStateCookieName    = "oidc_oauth_state"
	oidcOAuthVerifierCookie     = "oidc_oauth_verifier"
	oidcOAuthRedirectCookie     = "oidc_oauth_redirect"
	oidcOAuthNonceCookie        = "oidc_oauth_nonce"
	oidcOAuthIntentCookieName   = "oidc_oauth_intent"
	oidcOAuthBindUserCookieName = "oidc_oauth_bind_user"
	oidcOAuthCookieMaxAgeSec    = 10 * 60 // 10 minutes
	oidcOAuthDefaultRedirectTo  = "/dashboard"
	oidcOAuthDefaultFrontendCB  = "/auth/oidc/callback"
)

type oidcTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
REDACTED

type oidcTokenExchangeError struct {
	StatusCode          int
	ProviderError       string
	ProviderDescription string
	Body                string
REDACTED

func (e *oidcTokenExchangeError) Error() string {
	if e == nil {
		return ""
REDACTED
	parts := []string{fmt.Sprintf("token exchange status=%d", e.StatusCode)REDACTED
	if strings.TrimSpace(e.ProviderError) != "" {
		parts = append(parts, "error="+strings.TrimSpace(e.ProviderError))
REDACTED
	if strings.TrimSpace(e.ProviderDescription) != "" {
		parts = append(parts, "error_description="+strings.TrimSpace(e.ProviderDescription))
REDACTED
	return strings.Join(parts, " ")
REDACTED

type oidcIDTokenClaims struct {
	Email             string `json:"email,omitempty"`
	EmailVerified     *bool  `json:"email_verified,omitempty"`
	PreferredUsername string `json:"preferred_username,omitempty"`
	Name              string `json:"name,omitempty"`
	Nonce             string `json:"nonce,omitempty"`
	Azp               string `json:"azp,omitempty"`
	jwt.RegisteredClaims
REDACTED

type oidcUserInfoClaims struct {
	Email         string
	Username      string
	Subject       string
	EmailVerified *bool
	DisplayName   string
	AvatarURL     string
REDACTED

type oidcJWKSet struct {
	Keys []oidcJWK `json:"keys"`
REDACTED

type oidcJWK struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`

	N string `json:"n"`
	E string `json:"e"`

	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
REDACTED

// OIDCOAuthStart 启动通用 OIDC OAuth 登录流程。
// GET /api/v1/auth/oauth/oidc/start?redirect=/dashboard
func (h *AuthHandler) OIDCOAuthStart(c *gin.Context) {
	cfg, err := h.getOIDCOAuthConfig(c.Request.Context())
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
		redirectTo = oidcOAuthDefaultRedirectTo
REDACTED

	browserSessionKey, err := generateOAuthPendingBrowserSession()
	if err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_BROWSER_SESSION_GEN_FAILED", "failed to generate oauth browser session").WithCause(err))
		return
REDACTED

	secureCookie := isRequestHTTPS(c)
	oidcSetCookie(c, oidcOAuthStateCookieName, encodeCookieValue(state), oidcOAuthCookieMaxAgeSec, secureCookie)
	oidcSetCookie(c, oidcOAuthRedirectCookie, encodeCookieValue(redirectTo), oidcOAuthCookieMaxAgeSec, secureCookie)
	intent := normalizeOAuthIntent(c.Query("intent"))
	oidcSetCookie(c, oidcOAuthIntentCookieName, encodeCookieValue(intent), oidcOAuthCookieMaxAgeSec, secureCookie)
	captureOAuthPromoCode(c, secureCookie)
	setOAuthPendingBrowserCookie(c, browserSessionKey, secureCookie)
	clearOAuthPendingSessionCookie(c, secureCookie)
	if intent == oauthIntentBindCurrentUser {
		bindCookieValue, err := h.buildOAuthBindUserCookieFromContext(c)
		if err != nil {
			response.ErrorFrom(c, err)
			return
	REDACTED
		oidcSetCookie(c, oidcOAuthBindUserCookieName, encodeCookieValue(bindCookieValue), oidcOAuthCookieMaxAgeSec, secureCookie)
REDACTED else {
		oidcClearCookie(c, oidcOAuthBindUserCookieName, secureCookie)
REDACTED

	codeChallenge := ""
	if cfg.UsePKCE {
		verifier, genErr := oauth.GenerateCodeVerifier()
		if genErr != nil {
			response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_PKCE_GEN_FAILED", "failed to generate pkce verifier").WithCause(genErr))
			return
	REDACTED
		codeChallenge = oauth.GenerateCodeChallenge(verifier)
		oidcSetCookie(c, oidcOAuthVerifierCookie, encodeCookieValue(verifier), oidcOAuthCookieMaxAgeSec, secureCookie)
REDACTED

	nonce := ""
	if cfg.ValidateIDToken {
		nonce, err = oauth.GenerateState()
		if err != nil {
			response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_NONCE_GEN_FAILED", "failed to generate oauth nonce").WithCause(err))
			return
	REDACTED
		oidcSetCookie(c, oidcOAuthNonceCookie, encodeCookieValue(nonce), oidcOAuthCookieMaxAgeSec, secureCookie)
REDACTED

	redirectURI := strings.TrimSpace(cfg.RedirectURL)
	if redirectURI == "" {
		response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth redirect url not configured"))
		return
REDACTED

	authURL, err := buildOIDCAuthorizeURL(cfg, state, nonce, codeChallenge, redirectURI)
	if err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_BUILD_URL_FAILED", "failed to build oauth authorization url").WithCause(err))
		return
REDACTED

	c.Redirect(http.StatusFound, authURL)
REDACTED

// OIDCOAuthCallback 处理 OIDC 回调：校验 id_token、创建/登录用户并重定向到前端。
// GET /api/v1/auth/oauth/oidc/callback?code=...&state=...
func (h *AuthHandler) OIDCOAuthCallback(c *gin.Context) {
	cfg, cfgErr := h.getOIDCOAuthConfig(c.Request.Context())
	if cfgErr != nil {
		response.ErrorFrom(c, cfgErr)
		return
REDACTED

	frontendCallback := strings.TrimSpace(cfg.FrontendRedirectURL)
	if frontendCallback == "" {
		frontendCallback = oidcOAuthDefaultFrontendCB
REDACTED

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
		oidcClearCookie(c, oidcOAuthStateCookieName, secureCookie)
		oidcClearCookie(c, oidcOAuthVerifierCookie, secureCookie)
		oidcClearCookie(c, oidcOAuthRedirectCookie, secureCookie)
		oidcClearCookie(c, oidcOAuthNonceCookie, secureCookie)
		oidcClearCookie(c, oidcOAuthIntentCookieName, secureCookie)
		oidcClearCookie(c, oidcOAuthBindUserCookieName, secureCookie)
		clearOAuthPromoCodeCookie(c, secureCookie)
REDACTED()

	expectedState, err := readCookieDecoded(c, oidcOAuthStateCookieName)
	if err != nil || expectedState == "" || state != expectedState {
		redirectOAuthError(c, frontendCallback, "invalid_state", "invalid oauth state", "")
		return
REDACTED

	redirectTo, _ := readCookieDecoded(c, oidcOAuthRedirectCookie)
	redirectTo = sanitizeFrontendRedirectPath(redirectTo)
	if redirectTo == "" {
		redirectTo = oidcOAuthDefaultRedirectTo
REDACTED
	browserSessionKey, _ := readOAuthPendingBrowserCookie(c)
	if strings.TrimSpace(browserSessionKey) == "" {
		redirectOAuthError(c, frontendCallback, "missing_browser_session", "missing oauth browser session", "")
		return
REDACTED
	intent, _ := readCookieDecoded(c, oidcOAuthIntentCookieName)
	intent = normalizeOAuthIntent(intent)

	codeVerifier := ""
	if cfg.UsePKCE {
		codeVerifier, _ = readCookieDecoded(c, oidcOAuthVerifierCookie)
		if codeVerifier == "" {
			redirectOAuthError(c, frontendCallback, "missing_verifier", "missing pkce verifier", "")
			return
	REDACTED
REDACTED

	expectedNonce := ""
	if cfg.ValidateIDToken {
		expectedNonce, _ = readCookieDecoded(c, oidcOAuthNonceCookie)
		if expectedNonce == "" {
			redirectOAuthError(c, frontendCallback, "missing_nonce", "missing oauth nonce", "")
			return
	REDACTED
REDACTED

	redirectURI := strings.TrimSpace(cfg.RedirectURL)
	if redirectURI == "" {
		redirectOAuthError(c, frontendCallback, "config_error", "oauth redirect url not configured", "")
		return
REDACTED

	tokenResp, err := oidcExchangeCode(c.Request.Context(), cfg, code, redirectURI, codeVerifier)
	if err != nil {
		description := ""
		var exchangeErr *oidcTokenExchangeError
		if errors.As(err, &exchangeErr) && exchangeErr != nil {
			log.Printf(
				"[OIDC OAuth] token exchange failed: status=%d provider_error=%q provider_description=%q body=%s",
				exchangeErr.StatusCode,
				exchangeErr.ProviderError,
				exchangeErr.ProviderDescription,
				truncateLogValue(exchangeErr.Body, 2048),
			)
			description = exchangeErr.Error()
	REDACTED else {
			log.Printf("[OIDC OAuth] token exchange failed: %v", err)
			description = err.Error()
	REDACTED
		redirectOAuthError(c, frontendCallback, "token_exchange_failed", "failed to exchange oauth code", singleLine(description))
		return
REDACTED

	var idClaims *oidcIDTokenClaims
	if cfg.ValidateIDToken {
		if strings.TrimSpace(tokenResp.IDToken) == "" {
			redirectOAuthError(c, frontendCallback, "missing_id_token", "missing id_token", "")
			return
	REDACTED

		idClaims, err = oidcParseAndValidateIDToken(c.Request.Context(), cfg, tokenResp.IDToken, expectedNonce)
		if err != nil {
			log.Printf("[OIDC OAuth] id_token validation failed: %v", err)
			redirectOAuthError(c, frontendCallback, "invalid_id_token", "failed to validate id_token", "")
			return
	REDACTED
REDACTED

	userInfoClaims, err := oidcFetchUserInfo(c.Request.Context(), cfg, tokenResp)
	if err != nil {
		log.Printf("[OIDC OAuth] userinfo fetch failed: %v", err)
		redirectOAuthError(c, frontendCallback, "userinfo_failed", "failed to fetch user info", "")
		return
REDACTED

	subject := ""
	if idClaims != nil {
		subject = strings.TrimSpace(idClaims.Subject)
REDACTED
	if subject == "" {
		subject = strings.TrimSpace(userInfoClaims.Subject)
REDACTED
	if subject == "" {
		redirectOAuthError(c, frontendCallback, "missing_subject", "missing subject claim", "")
		return
REDACTED
	issuer := ""
	if idClaims != nil {
		issuer = strings.TrimSpace(idClaims.Issuer)
REDACTED
	if issuer == "" {
		issuer = strings.TrimSpace(cfg.IssuerURL)
REDACTED
	if issuer == "" {
		redirectOAuthError(c, frontendCallback, "missing_issuer", "missing issuer claim", "")
		return
REDACTED

	emailVerified := userInfoClaims.EmailVerified
	if emailVerified == nil && idClaims != nil {
		emailVerified = idClaims.EmailVerified
REDACTED
	if idClaims != nil && userInfoClaims.Subject != "" && idClaims.Subject != "" && strings.TrimSpace(userInfoClaims.Subject) != strings.TrimSpace(idClaims.Subject) {
		redirectOAuthError(c, frontendCallback, "subject_mismatch", "userinfo subject does not match id_token", "")
		return
REDACTED

	identityKey := oidcIdentityKey(issuer, subject)
	compatEmail := strings.TrimSpace(userInfoClaims.Email)
	if compatEmail == "" && idClaims != nil {
		compatEmail = strings.TrimSpace(idClaims.Email)
REDACTED
	email := oidcSyntheticEmailFromIdentityKey(identityKey)
	username := firstNonEmpty(
		userInfoClaims.Username,
		func() string {
			if idClaims != nil {
				return idClaims.PreferredUsername
		REDACTED
			return ""
	REDACTED(),
		func() string {
			if idClaims != nil {
				return idClaims.Name
		REDACTED
			return ""
	REDACTED(),
		oidcFallbackUsername(subject),
	)
	identityRef := service.PendingAuthIdentityKey{
		ProviderType:    "oidc",
		ProviderKey:     issuer,
		ProviderSubject: subject,
REDACTED
	upstreamClaims := map[string]any{
		"email":             email,
		"username":          username,
		"subject":           subject,
		"issuer":            issuer,
		"email_verified":    emailVerified != nil && *emailVerified,
		"provider_fallback": strings.TrimSpace(cfg.ProviderName),
		"suggested_display_name": firstNonEmpty(userInfoClaims.DisplayName, func() string {
			if idClaims != nil {
				return idClaims.Name
		REDACTED
			return ""
	REDACTED(), username),
		"suggested_avatar_url": userInfoClaims.AvatarURL,
REDACTED
	if compatEmail != "" && !strings.EqualFold(strings.TrimSpace(compatEmail), strings.TrimSpace(email)) {
		upstreamClaims["compat_email"] = compatEmail
REDACTED
	if intent == oauthIntentBindCurrentUser {
		targetUserID, err := h.readOAuthBindUserIDFromCookie(c, oidcOAuthBindUserCookieName)
		if err != nil {
			redirectOAuthError(c, frontendCallback, "invalid_state", "invalid oauth bind target", "")
			return
	REDACTED
		if err := h.createOAuthPendingSession(c, oauthPendingSessionPayload{
			Intent:                 oauthIntentBindCurrentUser,
			Identity:               identityRef,
			TargetUserID:           &targetUserID,
			ResolvedEmail:          email,
			RedirectTo:             redirectTo,
			BrowserSessionKey:      browserSessionKey,
			UpstreamIdentityClaims: upstreamClaims,
			CompletionResponse: map[string]any{
				"redirect": redirectTo,
		REDACTED,
	REDACTED); err != nil {
			redirectOAuthError(c, frontendCallback, "session_error", "failed to continue oauth bind", "")
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
	if existingIdentityUser != nil {
		if err := h.createOAuthPendingSession(c, oauthPendingSessionPayload{
			Intent:                 oauthIntentLogin,
			Identity:               identityRef,
			TargetUserID:           &existingIdentityUser.ID,
			ResolvedEmail:          existingIdentityUser.Email,
			RedirectTo:             redirectTo,
			BrowserSessionKey:      browserSessionKey,
			UpstreamIdentityClaims: upstreamClaims,
			CompletionResponse: map[string]any{
				"redirect": redirectTo,
		REDACTED,
	REDACTED); err != nil {
			redirectOAuthError(c, frontendCallback, "session_error", "failed to continue oauth login", "")
			return
	REDACTED
		redirectToFrontendCallback(c, frontendCallback)
		return
REDACTED

	compatEmailUser, err := h.findOIDCCompatEmailUser(c.Request.Context(), compatEmail)
	if err != nil {
		redirectOAuthError(c, frontendCallback, "session_error", infraerrors.Reason(err), infraerrors.Message(err))
		return
REDACTED

	if cfg.RequireEmailVerified {
		if emailVerified == nil || !*emailVerified {
			redirectOAuthError(c, frontendCallback, "email_not_verified", "email is not verified", "")
			return
	REDACTED
REDACTED

	// 快捷路径：当上游返回已验证邮箱、部署不要求额外确认且本地没有同邮箱账号时，
	// 直接信任上游身份完成注册/登录，避免展示 choice 页。
	if compatEmailUser == nil &&
		strings.TrimSpace(compatEmail) != "" &&
		emailVerified != nil && *emailVerified {
		if handled := h.tryOIDCVerifiedEmailFastPath(
			c,
			frontendCallback,
			redirectTo,
			identityRef,
			compatEmail,
			username,
			upstreamClaims,
		); handled {
			return
	REDACTED
REDACTED

	if h.isForceEmailOnThirdPartySignup(c.Request.Context()) {
		if err := h.createOIDCOAuthChoicePendingSession(
			c,
			identityRef,
			email,
			email,
			redirectTo,
			browserSessionKey,
			upstreamClaims,
			compatEmail,
			compatEmailUser,
			true,
		); err != nil {
			redirectOAuthError(c, frontendCallback, "session_error", "failed to continue oauth login", "")
			return
	REDACTED
		redirectToFrontendCallback(c, frontendCallback)
		return
REDACTED

	if err := h.createOIDCOAuthChoicePendingSession(
		c,
		identityRef,
		email,
		email,
		redirectTo,
		browserSessionKey,
		upstreamClaims,
		compatEmail,
		compatEmailUser,
		h.isForceEmailOnThirdPartySignup(c.Request.Context()),
	); err != nil {
		redirectOAuthError(c, frontendCallback, "session_error", "failed to continue oauth login", "")
		return
REDACTED
	redirectToFrontendCallback(c, frontendCallback)
REDACTED

func (h *AuthHandler) findOIDCCompatEmailUser(ctx context.Context, email string) (*dbent.User, error) {
	client := h.entClient()
	if client == nil {
		return nil, infraerrors.ServiceUnavailable("PENDING_AUTH_NOT_READY", "pending auth service is not ready")
REDACTED

	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" ||
		strings.HasSuffix(email, service.LinuxDoConnectSyntheticEmailDomain) ||
		strings.HasSuffix(email, service.OIDCConnectSyntheticEmailDomain) ||
		strings.HasSuffix(email, service.WeChatConnectSyntheticEmailDomain) ||
		strings.HasSuffix(email, service.DingTalkConnectSyntheticEmailDomain) {
		return nil, nil
REDACTED

	userEntity, err := findUserByNormalizedEmail(ctx, client, email)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			return nil, nil
	REDACTED
		return nil, infraerrors.InternalServer("COMPAT_EMAIL_LOOKUP_FAILED", "failed to look up compat email user").WithCause(err)
REDACTED
	return userEntity, nil
REDACTED

func (h *AuthHandler) createOIDCOAuthChoicePendingSession(
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
	if forceEmailOnSignup && compatEmailUser == nil {
		completionResponse["choice_reason"] = "force_email_on_signup"
REDACTED

	resolvedChoiceEmail := suggestionEmail
	if compatEmailUser != nil {
		resolvedChoiceEmail = strings.TrimSpace(compatEmailUser.Email)
REDACTED
	var targetUserID *int64
	if compatEmailUser != nil && compatEmailUser.ID > 0 {
		targetUserID = &compatEmailUser.ID
REDACTED

	return h.createOAuthPendingSession(c, oauthPendingSessionPayload{
		Intent:                 oauthIntentLogin,
		Identity:               identity,
		TargetUserID:           targetUserID,
		ResolvedEmail:          resolvedChoiceEmail,
		RedirectTo:             redirectTo,
		BrowserSessionKey:      browserSessionKey,
		UpstreamIdentityClaims: upstreamClaims,
		CompletionResponse:     completionResponse,
REDACTED)
REDACTED

type completeOIDCOAuthRequest struct {
	InvitationCode   string `json:"invitation_code" binding:"required"`
	AffCode          string `json:"aff_code,omitempty"`
	AdoptDisplayName *bool  `json:"adopt_display_name,omitempty"`
	AdoptAvatar      *bool  `json:"adopt_avatar,omitempty"`
REDACTED

// CompleteOIDCOAuthRegistration completes a pending OAuth registration by validating
// the invitation code and creating the user account.
// POST /api/v1/auth/oauth/oidc/complete-registration
func (h *AuthHandler) CompleteOIDCOAuthRegistration(c *gin.Context) {
	var req completeOIDCOAuthRequest
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

	client := h.entClient()
	if client == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("PENDING_AUTH_NOT_READY", "pending auth service is not ready"))
		return
REDACTED
	if err := ensurePendingOAuthRegistrationIdentityAvailable(c.Request.Context(), client, session); err != nil {
		respondPendingOAuthBindingApplyError(c, err)
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
	tokenPair, user, err := h.authService.LoginOrRegisterOAuthWithTokenPairAndPromoCode(
		c.Request.Context(),
		email,
		username,
		req.InvitationCode,
		req.AffCode,
		pendingOAuthPromoCode(session),
		"oidc",
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	if err := applyPendingOAuthAdoptionAndConsumeSession(c.Request.Context(), client, h.authService, h.userService, session, decision, user.ID); err != nil {
		respondPendingOAuthBindingApplyError(c, err)
		return
REDACTED
	h.authService.RecordSuccessfulLogin(c.Request.Context(), user.ID)
	clearOAuthPendingSessionCookie(c, secureCookie)
	clearOAuthPendingBrowserCookie(c, secureCookie)

	c.JSON(http.StatusOK, gin.H{
		"access_token":  tokenPair.AccessToken,
		"refresh_token": tokenPair.RefreshToken,
		"expires_in":    tokenPair.ExpiresIn,
		"token_type":    "Bearer",
REDACTED)
REDACTED

func (h *AuthHandler) getOIDCOAuthConfig(ctx context.Context) (config.OIDCConnectConfig, error) {
	if h != nil && h.settingSvc != nil {
		return h.settingSvc.GetOIDCConnectOAuthConfig(ctx)
REDACTED
	if h == nil || h.cfg == nil {
		return config.OIDCConnectConfig{REDACTED, infraerrors.ServiceUnavailable("CONFIG_NOT_READY", "config not loaded")
REDACTED
	if !h.cfg.OIDC.Enabled {
		return config.OIDCConnectConfig{REDACTED, infraerrors.NotFound("OAUTH_DISABLED", "oauth login is disabled")
REDACTED
	return h.cfg.OIDC, nil
REDACTED

func oidcExchangeCode(
	ctx context.Context,
	cfg config.OIDCConnectConfig,
	code string,
	redirectURI string,
	codeVerifier string,
) (*oidcTokenResponse, error) {
	client := req.C().SetTimeout(30 * time.Second)

	form := url.Values{REDACTED
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", cfg.ClientID)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	if strings.TrimSpace(codeVerifier) != "" {
		form.Set("code_verifier", codeVerifier)
REDACTED

	r := client.R().
		SetContext(ctx).
		SetHeader("Accept", "application/json")

	switch strings.ToLower(strings.TrimSpace(cfg.TokenAuthMethod)) {
	case "", "client_secret_post":
		form.Set("client_secret", cfg.ClientSecret)
	case "client_secret_basic":
		r.SetBasicAuth(cfg.ClientID, cfg.ClientSecret)
	case "none":
	default:
		return nil, fmt.Errorf("unsupported token_auth_method: %s", cfg.TokenAuthMethod)
REDACTED

	resp, err := r.SetFormDataFromValues(form).Post(cfg.TokenURL)
	if err != nil {
		return nil, fmt.Errorf("request token: %w", err)
REDACTED
	body := strings.TrimSpace(resp.String())
	if !resp.IsSuccessState() {
		providerErr, providerDesc := parseOAuthProviderError(body)
		return nil, &oidcTokenExchangeError{
			StatusCode:          resp.StatusCode,
			ProviderError:       providerErr,
			ProviderDescription: providerDesc,
			Body:                body,
	REDACTED
REDACTED

	tokenResp, ok := oidcParseTokenResponse(body)
	if !ok {
		return nil, &oidcTokenExchangeError{StatusCode: resp.StatusCode, Body: bodyREDACTED
REDACTED
	if strings.TrimSpace(tokenResp.TokenType) == "" {
		tokenResp.TokenType = "Bearer"
REDACTED
	if strings.TrimSpace(tokenResp.AccessToken) == "" && strings.TrimSpace(tokenResp.IDToken) == "" {
		return nil, &oidcTokenExchangeError{StatusCode: resp.StatusCode, Body: bodyREDACTED
REDACTED
	return tokenResp, nil
REDACTED

func oidcParseTokenResponse(body string) (*oidcTokenResponse, bool) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, false
REDACTED

	accessToken := strings.TrimSpace(getGJSON(body, "access_token"))
	idToken := strings.TrimSpace(getGJSON(body, "id_token"))
	if accessToken != "" || idToken != "" {
		tokenType := strings.TrimSpace(getGJSON(body, "token_type"))
		refreshToken := strings.TrimSpace(getGJSON(body, "refresh_token"))
		scope := strings.TrimSpace(getGJSON(body, "scope"))
		expiresIn := gjson.Get(body, "expires_in").Int()
		return &oidcTokenResponse{
			AccessToken:  accessToken,
			TokenType:    tokenType,
			ExpiresIn:    expiresIn,
			RefreshToken: refreshToken,
			Scope:        scope,
			IDToken:      idToken,
	REDACTED, true
REDACTED

	values, err := url.ParseQuery(body)
	if err != nil {
		return nil, false
REDACTED
	accessToken = strings.TrimSpace(values.Get("access_token"))
	idToken = strings.TrimSpace(values.Get("id_token"))
	if accessToken == "" && idToken == "" {
		return nil, false
REDACTED
	expiresIn := int64(0)
	if raw := strings.TrimSpace(values.Get("expires_in")); raw != "" {
		if v, parseErr := strconv.ParseInt(raw, 10, 64); parseErr == nil {
			expiresIn = v
	REDACTED
REDACTED
	return &oidcTokenResponse{
		AccessToken:  accessToken,
		TokenType:    strings.TrimSpace(values.Get("token_type")),
		ExpiresIn:    expiresIn,
		RefreshToken: strings.TrimSpace(values.Get("refresh_token")),
		Scope:        strings.TrimSpace(values.Get("scope")),
		IDToken:      idToken,
REDACTED, true
REDACTED

func oidcFetchUserInfo(
	ctx context.Context,
	cfg config.OIDCConnectConfig,
	token *oidcTokenResponse,
) (*oidcUserInfoClaims, error) {
	if strings.TrimSpace(cfg.UserInfoURL) == "" {
		return &oidcUserInfoClaims{REDACTED, nil
REDACTED
	if token == nil || strings.TrimSpace(token.AccessToken) == "" {
		return nil, errors.New("missing access_token for userinfo request")
REDACTED

	client := req.C().SetTimeout(30 * time.Second)
	authorization, err := buildBearerAuthorization(token.TokenType, token.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("invalid token for userinfo request: %w", err)
REDACTED

	resp, err := client.R().
		SetContext(ctx).
		SetHeader("Accept", "application/json").
		SetHeader("Authorization", authorization).
		Get(cfg.UserInfoURL)
	if err != nil {
		return nil, fmt.Errorf("request userinfo: %w", err)
REDACTED
	if !resp.IsSuccessState() {
		return nil, fmt.Errorf("userinfo status=%d", resp.StatusCode)
REDACTED

	return oidcParseUserInfo(resp.String(), cfg), nil
REDACTED

func oidcParseUserInfo(body string, cfg config.OIDCConnectConfig) *oidcUserInfoClaims {
	claims := &oidcUserInfoClaims{REDACTED
	claims.Email = firstNonEmpty(
		getGJSON(body, cfg.UserInfoEmailPath),
		getGJSON(body, "email"),
		getGJSON(body, "user.email"),
		getGJSON(body, "data.email"),
		getGJSON(body, "attributes.email"),
	)
	claims.Username = firstNonEmpty(
		getGJSON(body, cfg.UserInfoUsernamePath),
		getGJSON(body, "preferred_username"),
		getGJSON(body, "username"),
		getGJSON(body, "name"),
		getGJSON(body, "user.username"),
		getGJSON(body, "user.name"),
	)
	claims.Subject = firstNonEmpty(
		getGJSON(body, cfg.UserInfoIDPath),
		getGJSON(body, "sub"),
		getGJSON(body, "id"),
		getGJSON(body, "user_id"),
		getGJSON(body, "uid"),
		getGJSON(body, "user.id"),
	)
	if verified, ok := getGJSONBool(body, "email_verified"); ok {
		claims.EmailVerified = &verified
REDACTED
	claims.DisplayName = firstNonEmpty(
		getGJSON(body, "name"),
		getGJSON(body, "nickname"),
		getGJSON(body, "display_name"),
		getGJSON(body, "preferred_username"),
		getGJSON(body, "username"),
	)
	claims.AvatarURL = firstNonEmpty(
		getGJSON(body, "picture"),
		getGJSON(body, "avatar_url"),
		getGJSON(body, "avatar"),
		getGJSON(body, "profile_image_url"),
		getGJSON(body, "user.avatar"),
		getGJSON(body, "user.avatar_url"),
	)
	claims.Email = strings.TrimSpace(claims.Email)
	claims.Username = strings.TrimSpace(claims.Username)
	claims.Subject = strings.TrimSpace(claims.Subject)
	claims.DisplayName = strings.TrimSpace(claims.DisplayName)
	claims.AvatarURL = strings.TrimSpace(claims.AvatarURL)
	return claims
REDACTED

func getGJSONBool(body string, path string) (bool, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return false, false
REDACTED
	res := gjson.Get(body, path)
	if !res.Exists() {
		return false, false
REDACTED
	return res.Bool(), true
REDACTED

func buildOIDCAuthorizeURL(cfg config.OIDCConnectConfig, state, nonce, codeChallenge, redirectURI string) (string, error) {
	u, err := url.Parse(cfg.AuthorizeURL)
	if err != nil {
		return "", fmt.Errorf("parse authorize_url: %w", err)
REDACTED

	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", cfg.ClientID)
	q.Set("redirect_uri", redirectURI)
	if strings.TrimSpace(cfg.Scopes) != "" {
		q.Set("scope", cfg.Scopes)
REDACTED
	q.Set("state", state)
	if strings.TrimSpace(nonce) != "" {
		q.Set("nonce", nonce)
REDACTED
	if strings.TrimSpace(codeChallenge) != "" {
		q.Set("code_challenge", codeChallenge)
		q.Set("code_challenge_method", "S256")
REDACTED

	u.RawQuery = q.Encode()
	return u.String(), nil
REDACTED

func oidcParseAndValidateIDToken(ctx context.Context, cfg config.OIDCConnectConfig, idToken string, expectedNonce string) (*oidcIDTokenClaims, error) {
	idToken = strings.TrimSpace(idToken)
	if idToken == "" {
		return nil, errors.New("missing id_token")
REDACTED
	allowed := oidcAllowedSigningAlgs(cfg.AllowedSigningAlgs)
	if len(allowed) == 0 {
		return nil, errors.New("empty allowed signing algorithms")
REDACTED

	jwks, err := oidcFetchJWKSet(ctx, cfg.JWKSURL)
	if err != nil {
		return nil, err
REDACTED
	leeway := time.Duration(cfg.ClockSkewSeconds) * time.Second
	claims := &oidcIDTokenClaims{REDACTED

	parsed, err := jwt.ParseWithClaims(
		idToken,
		claims,
		func(token *jwt.Token) (any, error) {
			alg := strings.TrimSpace(token.Method.Alg())
			if !containsString(allowed, alg) {
				return nil, fmt.Errorf("unexpected signing algorithm: %s", alg)
		REDACTED
			kid, _ := token.Header["kid"].(string)
			return oidcFindPublicKey(jwks, strings.TrimSpace(kid), alg)
	REDACTED,
		jwt.WithValidMethods(allowed),
		jwt.WithAudience(cfg.ClientID),
		jwt.WithIssuer(cfg.IssuerURL),
		jwt.WithLeeway(leeway),
	)
	if err != nil {
		return nil, err
REDACTED
	if !parsed.Valid {
		return nil, errors.New("id_token invalid")
REDACTED
	if strings.TrimSpace(claims.Subject) == "" {
		return nil, errors.New("id_token missing sub")
REDACTED
	if expectedNonce != "" && strings.TrimSpace(claims.Nonce) != strings.TrimSpace(expectedNonce) {
		return nil, errors.New("id_token nonce mismatch")
REDACTED
	if len(claims.Audience) > 1 {
		if strings.TrimSpace(claims.Azp) == "" || strings.TrimSpace(claims.Azp) != strings.TrimSpace(cfg.ClientID) {
			return nil, errors.New("id_token azp mismatch")
	REDACTED
REDACTED
	return claims, nil
REDACTED

func oidcAllowedSigningAlgs(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{"RS256", "ES256", "PS256"REDACTED
REDACTED
	seen := make(map[string]struct{REDACTED)
	out := make([]string, 0, 4)
	for _, part := range strings.Split(raw, ",") {
		alg := strings.ToUpper(strings.TrimSpace(part))
		if alg == "" {
			continue
	REDACTED
		if _, ok := seen[alg]; ok {
			continue
	REDACTED
		seen[alg] = struct{REDACTED{REDACTED
		out = append(out, alg)
REDACTED
	return out
REDACTED

func oidcFetchJWKSet(ctx context.Context, jwksURL string) (*oidcJWKSet, error) {
	jwksURL = strings.TrimSpace(jwksURL)
	if jwksURL == "" {
		return nil, errors.New("missing jwks_url")
REDACTED
	resp, err := req.C().
		SetTimeout(30*time.Second).
		R().
		SetContext(ctx).
		SetHeader("Accept", "application/json").
		Get(jwksURL)
	if err != nil {
		return nil, fmt.Errorf("request jwks: %w", err)
REDACTED
	if !resp.IsSuccessState() {
		return nil, fmt.Errorf("jwks status=%d", resp.StatusCode)
REDACTED
	set := &oidcJWKSet{REDACTED
	if err := json.Unmarshal(resp.Bytes(), set); err != nil {
		return nil, fmt.Errorf("parse jwks: %w", err)
REDACTED
	if len(set.Keys) == 0 {
		return nil, errors.New("jwks empty keys")
REDACTED
	return set, nil
REDACTED

func oidcFindPublicKey(set *oidcJWKSet, kid, alg string) (any, error) {
	if set == nil {
		return nil, errors.New("jwks not loaded")
REDACTED
	alg = strings.ToUpper(strings.TrimSpace(alg))
	kid = strings.TrimSpace(kid)

	var lastErr error
	for i := range set.Keys {
		k := set.Keys[i]
		if strings.TrimSpace(k.Use) != "" && !strings.EqualFold(strings.TrimSpace(k.Use), "sig") {
			continue
	REDACTED
		if kid != "" && strings.TrimSpace(k.Kid) != kid {
			continue
	REDACTED
		if strings.TrimSpace(k.Alg) != "" && !strings.EqualFold(strings.TrimSpace(k.Alg), alg) {
			continue
	REDACTED
		pk, err := k.publicKey()
		if err != nil {
			lastErr = err
			continue
	REDACTED
		if pk != nil {
			return pk, nil
	REDACTED
REDACTED
	if lastErr != nil {
		return nil, lastErr
REDACTED
	if kid != "" {
		return nil, fmt.Errorf("jwk not found for kid=%s", kid)
REDACTED
	return nil, errors.New("jwk not found")
REDACTED

func (k oidcJWK) publicKey() (any, error) {
	switch strings.ToUpper(strings.TrimSpace(k.Kty)) {
	case "RSA":
		n, err := decodeBase64URLBigInt(k.N)
		if err != nil {
			return nil, fmt.Errorf("decode rsa n: %w", err)
	REDACTED
		eBytes, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(k.E))
		if err != nil {
			return nil, fmt.Errorf("decode rsa e: %w", err)
	REDACTED
		if len(eBytes) == 0 {
			return nil, errors.New("empty rsa e")
	REDACTED
		e := 0
		for _, b := range eBytes {
			e = (e << 8) | int(b)
	REDACTED
		if e <= 0 {
			return nil, errors.New("invalid rsa exponent")
	REDACTED
		if n.Sign() <= 0 {
			return nil, errors.New("invalid rsa modulus")
	REDACTED
		return &rsa.PublicKey{N: n, E: eREDACTED, nil
	case "EC":
		var curve elliptic.Curve
		switch strings.TrimSpace(k.Crv) {
		case "P-256":
			curve = elliptic.P256()
		case "P-384":
			curve = elliptic.P384()
		case "P-521":
			curve = elliptic.P521()
		default:
			return nil, fmt.Errorf("unsupported ec curve: %s", k.Crv)
	REDACTED
		x, err := decodeBase64URLBigInt(k.X)
		if err != nil {
			return nil, fmt.Errorf("decode ec x: %w", err)
	REDACTED
		y, err := decodeBase64URLBigInt(k.Y)
		if err != nil {
			return nil, fmt.Errorf("decode ec y: %w", err)
	REDACTED
		if !curve.IsOnCurve(x, y) {
			return nil, errors.New("ec point is not on curve")
	REDACTED
		return &ecdsa.PublicKey{Curve: curve, X: x, Y: yREDACTED, nil
	default:
		return nil, fmt.Errorf("unsupported jwk kty: %s", k.Kty)
REDACTED
REDACTED

func decodeBase64URLBigInt(raw string) (*big.Int, error) {
	buf, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
REDACTED
	if len(buf) == 0 {
		return nil, errors.New("empty value")
REDACTED
	return new(big.Int).SetBytes(buf), nil
REDACTED

func containsString(values []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, v := range values {
		if strings.EqualFold(strings.TrimSpace(v), target) {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func oidcIdentityKey(issuer, subject string) string {
	issuer = strings.TrimSpace(strings.ToLower(issuer))
	subject = strings.TrimSpace(subject)
	return issuer + "\x1f" + subject
REDACTED

func oidcSyntheticEmailFromIdentityKey(identityKey string) string {
	identityKey = strings.TrimSpace(identityKey)
	if identityKey == "" {
		return ""
REDACTED
	sum := sha256.Sum256([]byte(identityKey))
	return "oidc-" + hex.EncodeToString(sum[:16]) + service.OIDCConnectSyntheticEmailDomain
REDACTED

func oidcFallbackUsername(subject string) string {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return "oidc_user"
REDACTED
	sum := sha256.Sum256([]byte(subject))
	return "oidc_" + hex.EncodeToString(sum[:])[:12]
REDACTED

func oidcSetCookie(c *gin.Context, name, value string, maxAgeSec int, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     oidcOAuthCookiePath,
		MaxAge:   maxAgeSec,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
REDACTED)
REDACTED

func oidcClearCookie(c *gin.Context, name string, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     oidcOAuthCookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
REDACTED)
REDACTED

// tryOIDCVerifiedEmailFastPath 在 OIDC 上游已返回已验证邮箱时尝试跳过 choice/pending 页。
// 返回 true 表示已经写出重定向响应；返回 false 表示调用方应继续回退到常规 choice 流程。
func (h *AuthHandler) tryOIDCVerifiedEmailFastPath(
	c *gin.Context,
	frontendCallback string,
	redirectTo string,
	identity service.PendingAuthIdentityKey,
	compatEmail string,
	username string,
	upstreamClaims map[string]any,
) bool {
	if h == nil || h.authService == nil || h.settingSvc == nil {
		return false
REDACTED
	ctx := c.Request.Context()
	if h.isForceEmailOnThirdPartySignup(ctx) {
		return false
REDACTED
	if h.settingSvc.IsInvitationCodeEnabled(ctx) {
		return false
REDACTED
	if err := h.ensureBackendModeAllowsNewUserLogin(ctx); err != nil {
		log.Printf("[OIDC OAuth] verified-email fast path blocked by backend mode: reason=%s", infraerrors.Reason(err))
		clearOAuthPendingSessionCookie(c, isRequestHTTPS(c))
		clearOAuthPendingBrowserCookie(c, isRequestHTTPS(c))
		redirectOAuthError(c, frontendCallback, "login_blocked", infraerrors.Reason(err), infraerrors.Message(err))
		return true
REDACTED

	verifiedEmail := strings.TrimSpace(strings.ToLower(compatEmail))
	upstreamMetadata := make(map[string]any, len(upstreamClaims)+1)
	for k, v := range upstreamClaims {
		upstreamMetadata[k] = v
REDACTED
	if syntheticEmail := pendingSessionStringValue(upstreamClaims, "email"); syntheticEmail != "" && !strings.EqualFold(syntheticEmail, verifiedEmail) {
		upstreamMetadata["synthetic_email"] = syntheticEmail
REDACTED
	upstreamMetadata["email"] = verifiedEmail
	input := service.EmailOAuthIdentityInput{
		ProviderType:     strings.TrimSpace(identity.ProviderType),
		ProviderKey:      strings.TrimSpace(identity.ProviderKey),
		ProviderSubject:  strings.TrimSpace(identity.ProviderSubject),
		Email:            verifiedEmail,
		EmailVerified:    true,
		Username:         strings.TrimSpace(username),
		DisplayName:      pendingSessionStringValue(upstreamClaims, "suggested_display_name"),
		AvatarURL:        pendingSessionStringValue(upstreamClaims, "suggested_avatar_url"),
		UpstreamMetadata: upstreamMetadata,
REDACTED
	tokenPair, _, err := h.authService.LoginOrRegisterVerifiedEmailOAuthWithSignupCodes(
		ctx,
		input,
		"",
		"",
		readOAuthPromoCode(c),
	)
	if err != nil {
		log.Printf("[OIDC OAuth] verified-email fast path skipped: reason=%s", infraerrors.Reason(err))
		return false
REDACTED

	fragment := url.Values{REDACTED
	fragment.Set("access_token", tokenPair.AccessToken)
	fragment.Set("refresh_token", tokenPair.RefreshToken)
	fragment.Set("expires_in", fmt.Sprintf("%d", tokenPair.ExpiresIn))
	fragment.Set("token_type", "Bearer")
	fragment.Set("redirect", redirectTo)
	clearOAuthPendingSessionCookie(c, isRequestHTTPS(c))
	clearOAuthPendingBrowserCookie(c, isRequestHTTPS(c))
	redirectWithFragment(c, frontendCallback, fragment)
	return true
REDACTED
