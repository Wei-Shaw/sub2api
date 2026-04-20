package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

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
	wechatOAuthDefaultRedirectTo  = "/dashboard"
	wechatOAuthDefaultFrontendCB  = "/auth/wechat/callback"
	wechatOAuthProviderKey        = "wechat-main"

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
	setOAuthPendingBrowserCookie(c, browserSessionKey, secureCookie)
	clearOAuthPendingSessionCookie(c, secureCookie)

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
	frontendCallback := wechatOAuthFrontendCallback()

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
	providerSubject := firstNonEmpty(unionid, openid)
	if providerSubject == "" {
		redirectOAuthError(c, frontendCallback, "provider_error", "wechat_missing_subject", "")
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
		"suggested_display_name": strings.TrimSpace(userInfo.Nickname),
		"suggested_avatar_url":   strings.TrimSpace(userInfo.HeadImgURL),
REDACTED

	tokenPair, _, err := h.authService.LoginOrRegisterOAuthWithTokenPair(c.Request.Context(), email, username, "")
	if err != nil {
		if err := h.createWeChatPendingSession(c, normalizeWeChatOAuthIntent(intent), providerSubject, email, redirectTo, browserSessionKey, upstreamClaims, tokenPair, err); err != nil {
			redirectOAuthError(c, frontendCallback, "session_error", "failed to continue oauth login", "")
			return
	REDACTED
		redirectToFrontendCallback(c, frontendCallback)
		return
REDACTED

	if err := h.createWeChatPendingSession(c, normalizeWeChatOAuthIntent(intent), providerSubject, email, redirectTo, browserSessionKey, upstreamClaims, tokenPair, nil); err != nil {
		redirectOAuthError(c, frontendCallback, "session_error", "failed to continue oauth login", "")
		return
REDACTED
	redirectToFrontendCallback(c, frontendCallback)
REDACTED

type completeWeChatOAuthRequest struct {
	InvitationCode   string `json:"invitation_code" binding:"required"`
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

	email := strings.TrimSpace(session.ResolvedEmail)
	username := pendingSessionStringValue(session.UpstreamIdentityClaims, "username")
	if email == "" || username == "" {
		response.ErrorFrom(c, infraerrors.BadRequest("PENDING_AUTH_SESSION_INVALID", "pending auth registration context is invalid"))
		return
REDACTED

	tokenPair, user, err := h.authService.LoginOrRegisterOAuthWithTokenPair(c.Request.Context(), email, username, req.InvitationCode)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	decision, err := h.upsertPendingOAuthAdoptionDecision(c, session.ID, oauthAdoptionDecisionRequest{
		AdoptDisplayName: req.AdoptDisplayName,
		AdoptAvatar:      req.AdoptAvatar,
REDACTED)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	if err := applyPendingOAuthAdoption(c.Request.Context(), h.entClient(), session, decision, &user.ID); err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("PENDING_AUTH_ADOPTION_APPLY_FAILED", "failed to apply oauth profile adoption").WithCause(err))
		return
REDACTED
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
		ResolvedEmail:          email,
		RedirectTo:             redirectTo,
		BrowserSessionKey:      browserSessionKey,
		UpstreamIdentityClaims: upstreamClaims,
		CompletionResponse:     completionResponse,
REDACTED)
REDACTED

func (h *AuthHandler) getWeChatOAuthConfig(ctx context.Context, rawMode string, c *gin.Context) (wechatOAuthConfig, error) {
	mode, err := resolveWeChatOAuthMode(rawMode, c)
	if err != nil {
		return wechatOAuthConfig{REDACTED, err
REDACTED

	apiBaseURL := ""
	if h != nil && h.settingSvc != nil {
		settings, err := h.settingSvc.GetAllSettings(ctx)
		if err == nil && settings != nil {
			apiBaseURL = strings.TrimSpace(settings.APIBaseURL)
	REDACTED
REDACTED

	cfg := wechatOAuthConfig{
		mode:             mode,
		redirectURI:      resolveWeChatOAuthAbsoluteURL(apiBaseURL, c, "/api/v1/auth/oauth/wechat/callback"),
		frontendCallback: wechatOAuthFrontendCallback(),
REDACTED

	switch mode {
	case "mp":
		cfg.appID = strings.TrimSpace(os.Getenv("WECHAT_OAUTH_MP_APP_ID"))
		cfg.appSecret = strings.TrimSpace(os.Getenv("WECHAT_OAUTH_MP_APP_SECRET"))
		cfg.authorizeURL = "https://open.weixin.qq.com/connect/oauth2/authorize"
		cfg.scope = "snsapi_userinfo"
	default:
		cfg.appID = strings.TrimSpace(os.Getenv("WECHAT_OAUTH_OPEN_APP_ID"))
		cfg.appSecret = strings.TrimSpace(os.Getenv("WECHAT_OAUTH_OPEN_APP_SECRET"))
		cfg.authorizeURL = "https://open.weixin.qq.com/connect/qrconnect"
		cfg.scope = "snsapi_login"
REDACTED

	if cfg.appID == "" || cfg.appSecret == "" {
		return wechatOAuthConfig{REDACTED, infraerrors.NotFound("OAUTH_DISABLED", "wechat oauth is disabled")
REDACTED
	if strings.TrimSpace(cfg.redirectURI) == "" {
		return wechatOAuthConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "wechat oauth redirect url not configured")
REDACTED

	return cfg, nil
REDACTED

func wechatOAuthFrontendCallback() string {
	return firstNonEmpty(strings.TrimSpace(os.Getenv("WECHAT_OAUTH_FRONTEND_REDIRECT_URL")), wechatOAuthDefaultFrontendCB)
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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

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
