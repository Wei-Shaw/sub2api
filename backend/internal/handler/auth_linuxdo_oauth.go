package handler

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/oauth"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/imroc/req/v3"
	"github.com/tidwall/gjson"
)

const (
	linuxDoOAuthCookiePath        = "/api/v1/auth/oauth/linuxdo"
	linuxDoOAuthStateCookieName   = "linuxdo_oauth_state"
	linuxDoOAuthVerifierCookie    = "linuxdo_oauth_verifier"
	linuxDoOAuthRedirectCookie    = "linuxdo_oauth_redirect"
	linuxDoOAuthCookieMaxAgeSec   = 10 * 60 // 10 minutes
	linuxDoOAuthDefaultRedirectTo = "/dashboard"
	linuxDoOAuthDefaultFrontendCB = "/auth/linuxdo/callback"

	linuxDoOAuthMaxRedirectLen      = 2048
	linuxDoOAuthMaxFragmentValueLen = 512
	linuxDoOAuthMaxSubjectLen       = 64 - len("linuxdo-")
)

type linuxDoTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
REDACTED

type linuxDoTokenExchangeError struct {
	StatusCode          int
	ProviderError       string
	ProviderDescription string
	Body                string
REDACTED

func (e *linuxDoTokenExchangeError) Error() string {
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

// LinuxDoOAuthStart 启动 LinuxDo Connect OAuth 登录流程。
// GET /api/v1/auth/oauth/linuxdo/start?redirect=/dashboard
func (h *AuthHandler) LinuxDoOAuthStart(c *gin.Context) {
	cfg, err := h.getLinuxDoOAuthConfig(c.Request.Context())
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
		redirectTo = linuxDoOAuthDefaultRedirectTo
REDACTED

	secureCookie := isRequestHTTPS(c)
	setCookie(c, linuxDoOAuthStateCookieName, encodeCookieValue(state), linuxDoOAuthCookieMaxAgeSec, secureCookie)
	setCookie(c, linuxDoOAuthRedirectCookie, encodeCookieValue(redirectTo), linuxDoOAuthCookieMaxAgeSec, secureCookie)

	codeChallenge := ""
	if cfg.UsePKCE {
		verifier, err := oauth.GenerateCodeVerifier()
		if err != nil {
			response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_PKCE_GEN_FAILED", "failed to generate pkce verifier").WithCause(err))
			return
	REDACTED
		codeChallenge = oauth.GenerateCodeChallenge(verifier)
		setCookie(c, linuxDoOAuthVerifierCookie, encodeCookieValue(verifier), linuxDoOAuthCookieMaxAgeSec, secureCookie)
REDACTED

	redirectURI := strings.TrimSpace(cfg.RedirectURL)
	if redirectURI == "" {
		response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth redirect url not configured"))
		return
REDACTED

	authURL, err := buildLinuxDoAuthorizeURL(cfg, state, codeChallenge, redirectURI)
	if err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_BUILD_URL_FAILED", "failed to build oauth authorization url").WithCause(err))
		return
REDACTED

	c.Redirect(http.StatusFound, authURL)
REDACTED

// LinuxDoOAuthCallback 处理 OAuth 回调：创建/登录用户，然后重定向到前端。
// GET /api/v1/auth/oauth/linuxdo/callback?code=...&state=...
func (h *AuthHandler) LinuxDoOAuthCallback(c *gin.Context) {
	cfg, cfgErr := h.getLinuxDoOAuthConfig(c.Request.Context())
	if cfgErr != nil {
		response.ErrorFrom(c, cfgErr)
		return
REDACTED

	frontendCallback := strings.TrimSpace(cfg.FrontendRedirectURL)
	if frontendCallback == "" {
		frontendCallback = linuxDoOAuthDefaultFrontendCB
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
		clearCookie(c, linuxDoOAuthStateCookieName, secureCookie)
		clearCookie(c, linuxDoOAuthVerifierCookie, secureCookie)
		clearCookie(c, linuxDoOAuthRedirectCookie, secureCookie)
REDACTED()

	expectedState, err := readCookieDecoded(c, linuxDoOAuthStateCookieName)
	if err != nil || expectedState == "" || state != expectedState {
		redirectOAuthError(c, frontendCallback, "invalid_state", "invalid oauth state", "")
		return
REDACTED

	redirectTo, _ := readCookieDecoded(c, linuxDoOAuthRedirectCookie)
	redirectTo = sanitizeFrontendRedirectPath(redirectTo)
	if redirectTo == "" {
		redirectTo = linuxDoOAuthDefaultRedirectTo
REDACTED

	codeVerifier := ""
	if cfg.UsePKCE {
		codeVerifier, _ = readCookieDecoded(c, linuxDoOAuthVerifierCookie)
		if codeVerifier == "" {
			redirectOAuthError(c, frontendCallback, "missing_verifier", "missing pkce verifier", "")
			return
	REDACTED
REDACTED

	redirectURI := strings.TrimSpace(cfg.RedirectURL)
	if redirectURI == "" {
		redirectOAuthError(c, frontendCallback, "config_error", "oauth redirect url not configured", "")
		return
REDACTED

	tokenResp, err := linuxDoExchangeCode(c.Request.Context(), cfg, code, redirectURI, codeVerifier)
	if err != nil {
		description := ""
		var exchangeErr *linuxDoTokenExchangeError
		if errors.As(err, &exchangeErr) && exchangeErr != nil {
			log.Printf(
				"[LinuxDo OAuth] token exchange failed: status=%d provider_error=%q provider_description=%q body=%s",
				exchangeErr.StatusCode,
				exchangeErr.ProviderError,
				exchangeErr.ProviderDescription,
				truncateLogValue(exchangeErr.Body, 2048),
			)
			description = exchangeErr.Error()
	REDACTED else {
			log.Printf("[LinuxDo OAuth] token exchange failed: %v", err)
			description = err.Error()
	REDACTED
		redirectOAuthError(c, frontendCallback, "token_exchange_failed", "failed to exchange oauth code", singleLine(description))
		return
REDACTED

	email, username, subject, err := linuxDoFetchUserInfo(c.Request.Context(), cfg, tokenResp)
	if err != nil {
		log.Printf("[LinuxDo OAuth] userinfo fetch failed: %v", err)
		redirectOAuthError(c, frontendCallback, "userinfo_failed", "failed to fetch user info", "")
		return
REDACTED

	// 安全考虑：不要把第三方返回的 email 直接映射到本地账号（可能与本地邮箱用户冲突导致账号被接管）。
	// 统一使用基于 subject 的稳定合成邮箱来做账号绑定。
	if subject != "" {
		email = linuxDoSyntheticEmail(subject)
REDACTED

	// 传入空邀请码；如果需要邀请码，服务层返回 ErrOAuthInvitationRequired
	tokenPair, _, err := h.authService.LoginOrRegisterOAuthWithTokenPair(c.Request.Context(), email, username, "")
	if err != nil {
		if errors.Is(err, service.ErrOAuthInvitationRequired) {
			pendingToken, tokenErr := h.authService.CreatePendingOAuthToken(email, username)
			if tokenErr != nil {
				redirectOAuthError(c, frontendCallback, "login_failed", "service_error", "")
				return
		REDACTED
			fragment := url.Values{REDACTED
			fragment.Set("error", "invitation_required")
			fragment.Set("pending_oauth_token", pendingToken)
			fragment.Set("redirect", redirectTo)
			redirectWithFragment(c, frontendCallback, fragment)
			return
	REDACTED
		// 避免把内部细节泄露给客户端；给前端保留结构化原因与提示信息即可。
		redirectOAuthError(c, frontendCallback, "login_failed", infraerrors.Reason(err), infraerrors.Message(err))
		return
REDACTED

	fragment := url.Values{REDACTED
	fragment.Set("access_token", tokenPair.AccessToken)
	fragment.Set("refresh_token", tokenPair.RefreshToken)
	fragment.Set("expires_in", fmt.Sprintf("%d", tokenPair.ExpiresIn))
	fragment.Set("token_type", "Bearer")
	fragment.Set("redirect", redirectTo)
	redirectWithFragment(c, frontendCallback, fragment)
REDACTED

type completeLinuxDoOAuthRequest struct {
	PendingOAuthToken string `json:"pending_oauth_token" binding:"required"`
	InvitationCode    string `json:"invitation_code"     binding:"required"`
REDACTED

// CompleteLinuxDoOAuthRegistration completes a pending OAuth registration by validating
// the invitation code and creating the user account.
// POST /api/v1/auth/oauth/linuxdo/complete-registration
func (h *AuthHandler) CompleteLinuxDoOAuthRegistration(c *gin.Context) {
	var req completeLinuxDoOAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "INVALID_REQUEST", "message": err.Error()REDACTED)
		return
REDACTED

	email, username, err := h.authService.VerifyPendingOAuthToken(req.PendingOAuthToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "INVALID_TOKEN", "message": "invalid or expired registration token"REDACTED)
		return
REDACTED

	tokenPair, _, err := h.authService.LoginOrRegisterOAuthWithTokenPair(c.Request.Context(), email, username, req.InvitationCode)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED

	c.JSON(http.StatusOK, gin.H{
		"access_token":  tokenPair.AccessToken,
		"refresh_token": tokenPair.RefreshToken,
		"expires_in":    tokenPair.ExpiresIn,
		"token_type":    "Bearer",
REDACTED)
REDACTED

func (h *AuthHandler) getLinuxDoOAuthConfig(ctx context.Context) (config.LinuxDoConnectConfig, error) {
	if h != nil && h.settingSvc != nil {
		return h.settingSvc.GetLinuxDoConnectOAuthConfig(ctx)
REDACTED
	if h == nil || h.cfg == nil {
		return config.LinuxDoConnectConfig{REDACTED, infraerrors.ServiceUnavailable("CONFIG_NOT_READY", "config not loaded")
REDACTED
	if !h.cfg.LinuxDo.Enabled {
		return config.LinuxDoConnectConfig{REDACTED, infraerrors.NotFound("OAUTH_DISABLED", "oauth login is disabled")
REDACTED
	return h.cfg.LinuxDo, nil
REDACTED

func linuxDoExchangeCode(
	ctx context.Context,
	cfg config.LinuxDoConnectConfig,
	code string,
	redirectURI string,
	codeVerifier string,
) (*linuxDoTokenResponse, error) {
	client := req.C().SetTimeout(30 * time.Second)

	form := url.Values{REDACTED
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", cfg.ClientID)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	if cfg.UsePKCE {
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
		return nil, &linuxDoTokenExchangeError{
			StatusCode:          resp.StatusCode,
			ProviderError:       providerErr,
			ProviderDescription: providerDesc,
			Body:                body,
	REDACTED
REDACTED

	tokenResp, ok := parseLinuxDoTokenResponse(body)
	if !ok || strings.TrimSpace(tokenResp.AccessToken) == "" {
		return nil, &linuxDoTokenExchangeError{
			StatusCode: resp.StatusCode,
			Body:       body,
	REDACTED
REDACTED
	if strings.TrimSpace(tokenResp.TokenType) == "" {
		tokenResp.TokenType = "Bearer"
REDACTED
	return tokenResp, nil
REDACTED

func linuxDoFetchUserInfo(
	ctx context.Context,
	cfg config.LinuxDoConnectConfig,
	token *linuxDoTokenResponse,
) (email string, username string, subject string, err error) {
	client := req.C().SetTimeout(30 * time.Second)
	authorization, err := buildBearerAuthorization(token.TokenType, token.AccessToken)
	if err != nil {
		return "", "", "", fmt.Errorf("invalid token for userinfo request: %w", err)
REDACTED

	resp, err := client.R().
		SetContext(ctx).
		SetHeader("Accept", "application/json").
		SetHeader("Authorization", authorization).
		Get(cfg.UserInfoURL)
	if err != nil {
		return "", "", "", fmt.Errorf("request userinfo: %w", err)
REDACTED
	if !resp.IsSuccessState() {
		return "", "", "", fmt.Errorf("userinfo status=%d", resp.StatusCode)
REDACTED

	return linuxDoParseUserInfo(resp.String(), cfg)
REDACTED

func linuxDoParseUserInfo(body string, cfg config.LinuxDoConnectConfig) (email string, username string, subject string, err error) {
	email = firstNonEmpty(
		getGJSON(body, cfg.UserInfoEmailPath),
		getGJSON(body, "email"),
		getGJSON(body, "user.email"),
		getGJSON(body, "data.email"),
		getGJSON(body, "attributes.email"),
	)
	username = firstNonEmpty(
		getGJSON(body, cfg.UserInfoUsernamePath),
		getGJSON(body, "username"),
		getGJSON(body, "preferred_username"),
		getGJSON(body, "name"),
		getGJSON(body, "user.username"),
		getGJSON(body, "user.name"),
	)
	subject = firstNonEmpty(
		getGJSON(body, cfg.UserInfoIDPath),
		getGJSON(body, "sub"),
		getGJSON(body, "id"),
		getGJSON(body, "user_id"),
		getGJSON(body, "uid"),
		getGJSON(body, "user.id"),
	)

	subject = strings.TrimSpace(subject)
	if subject == "" {
		return "", "", "", errors.New("userinfo missing id field")
REDACTED
	if !isSafeLinuxDoSubject(subject) {
		return "", "", "", errors.New("userinfo returned invalid id field")
REDACTED

	email = strings.TrimSpace(email)
	if email == "" {
		// LinuxDo Connect 的 userinfo 可能不提供 email。为兼容现有用户模型（email 必填且唯一），使用稳定的合成邮箱。
		email = linuxDoSyntheticEmail(subject)
REDACTED

	username = strings.TrimSpace(username)
	if username == "" {
		username = "linuxdo_" + subject
REDACTED

	return email, username, subject, nil
REDACTED

func buildLinuxDoAuthorizeURL(cfg config.LinuxDoConnectConfig, state string, codeChallenge string, redirectURI string) (string, error) {
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
	if cfg.UsePKCE {
		q.Set("code_challenge", codeChallenge)
		q.Set("code_challenge_method", "S256")
REDACTED

	u.RawQuery = q.Encode()
	return u.String(), nil
REDACTED

func redirectOAuthError(c *gin.Context, frontendCallback string, code string, message string, description string) {
	fragment := url.Values{REDACTED
	fragment.Set("error", truncateFragmentValue(code))
	if strings.TrimSpace(message) != "" {
		fragment.Set("error_message", truncateFragmentValue(message))
REDACTED
	if strings.TrimSpace(description) != "" {
		fragment.Set("error_description", truncateFragmentValue(description))
REDACTED
	redirectWithFragment(c, frontendCallback, fragment)
REDACTED

func redirectWithFragment(c *gin.Context, frontendCallback string, fragment url.Values) {
	u, err := url.Parse(frontendCallback)
	if err != nil {
		// 兜底：尽力跳转到默认页面，避免卡死在回调页。
		c.Redirect(http.StatusFound, linuxDoOAuthDefaultRedirectTo)
		return
REDACTED
	if u.Scheme != "" && !strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https") {
		c.Redirect(http.StatusFound, linuxDoOAuthDefaultRedirectTo)
		return
REDACTED
	u.Fragment = fragment.Encode()
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.Redirect(http.StatusFound, u.String())
REDACTED

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			return v
	REDACTED
REDACTED
	return ""
REDACTED

func parseOAuthProviderError(body string) (providerErr string, providerDesc string) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", ""
REDACTED

	providerErr = firstNonEmpty(
		getGJSON(body, "error"),
		getGJSON(body, "code"),
		getGJSON(body, "error.code"),
	)
	providerDesc = firstNonEmpty(
		getGJSON(body, "error_description"),
		getGJSON(body, "error.message"),
		getGJSON(body, "message"),
		getGJSON(body, "detail"),
	)

	if providerErr != "" || providerDesc != "" {
		return providerErr, providerDesc
REDACTED

	values, err := url.ParseQuery(body)
	if err != nil {
		return "", ""
REDACTED
	providerErr = firstNonEmpty(values.Get("error"), values.Get("code"))
	providerDesc = firstNonEmpty(values.Get("error_description"), values.Get("error_message"), values.Get("message"))
	return providerErr, providerDesc
REDACTED

func parseLinuxDoTokenResponse(body string) (*linuxDoTokenResponse, bool) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, false
REDACTED

	accessToken := strings.TrimSpace(getGJSON(body, "access_token"))
	if accessToken != "" {
		tokenType := strings.TrimSpace(getGJSON(body, "token_type"))
		refreshToken := strings.TrimSpace(getGJSON(body, "refresh_token"))
		scope := strings.TrimSpace(getGJSON(body, "scope"))
		expiresIn := gjson.Get(body, "expires_in").Int()
		return &linuxDoTokenResponse{
			AccessToken:  accessToken,
			TokenType:    tokenType,
			ExpiresIn:    expiresIn,
			RefreshToken: refreshToken,
			Scope:        scope,
	REDACTED, true
REDACTED

	values, err := url.ParseQuery(body)
	if err != nil {
		return nil, false
REDACTED
	accessToken = strings.TrimSpace(values.Get("access_token"))
	if accessToken == "" {
		return nil, false
REDACTED
	expiresIn := int64(0)
	if raw := strings.TrimSpace(values.Get("expires_in")); raw != "" {
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil {
			expiresIn = v
	REDACTED
REDACTED
	return &linuxDoTokenResponse{
		AccessToken:  accessToken,
		TokenType:    strings.TrimSpace(values.Get("token_type")),
		ExpiresIn:    expiresIn,
		RefreshToken: strings.TrimSpace(values.Get("refresh_token")),
		Scope:        strings.TrimSpace(values.Get("scope")),
REDACTED, true
REDACTED

func getGJSON(body string, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
REDACTED
	res := gjson.Get(body, path)
	if !res.Exists() {
		return ""
REDACTED
	return res.String()
REDACTED

func truncateLogValue(value string, maxLen int) string {
	value = strings.TrimSpace(value)
	if value == "" || maxLen <= 0 {
		return ""
REDACTED
	if len(value) <= maxLen {
		return value
REDACTED
	value = value[:maxLen]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
REDACTED
	return value
REDACTED

func singleLine(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
REDACTED
	return strings.Join(strings.Fields(value), " ")
REDACTED

func sanitizeFrontendRedirectPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
REDACTED
	if len(path) > linuxDoOAuthMaxRedirectLen {
		return ""
REDACTED
	// 只允许同源相对路径（避免开放重定向）。
	if !strings.HasPrefix(path, "/") {
		return ""
REDACTED
	if strings.HasPrefix(path, "//") {
		return ""
REDACTED
	if strings.Contains(path, "://") {
		return ""
REDACTED
	if strings.ContainsAny(path, "\r\n") {
		return ""
REDACTED
	return path
REDACTED

func isRequestHTTPS(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
REDACTED
	proto := strings.ToLower(strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")))
	return proto == "https"
REDACTED

func encodeCookieValue(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
REDACTED

func decodeCookieValue(value string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return "", err
REDACTED
	return string(raw), nil
REDACTED

func readCookieDecoded(c *gin.Context, name string) (string, error) {
	ck, err := c.Request.Cookie(name)
	if err != nil {
		return "", err
REDACTED
	return decodeCookieValue(ck.Value)
REDACTED

func setCookie(c *gin.Context, name string, value string, maxAgeSec int, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     linuxDoOAuthCookiePath,
		MaxAge:   maxAgeSec,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
REDACTED)
REDACTED

func clearCookie(c *gin.Context, name string, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     linuxDoOAuthCookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
REDACTED)
REDACTED

func truncateFragmentValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
REDACTED
	if len(value) > linuxDoOAuthMaxFragmentValueLen {
		value = value[:linuxDoOAuthMaxFragmentValueLen]
		for !utf8.ValidString(value) {
			value = value[:len(value)-1]
	REDACTED
REDACTED
	return value
REDACTED

func buildBearerAuthorization(tokenType, accessToken string) (string, error) {
	tokenType = strings.TrimSpace(tokenType)
	if tokenType == "" {
		tokenType = "Bearer"
REDACTED
	if !strings.EqualFold(tokenType, "Bearer") {
		return "", fmt.Errorf("unsupported token_type: %s", tokenType)
REDACTED

	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return "", errors.New("missing access_token")
REDACTED
	if strings.ContainsAny(accessToken, " \t\r\n") {
		return "", errors.New("access_token contains whitespace")
REDACTED
	return "Bearer " + accessToken, nil
REDACTED

func isSafeLinuxDoSubject(subject string) bool {
	subject = strings.TrimSpace(subject)
	if subject == "" || len(subject) > linuxDoOAuthMaxSubjectLen {
		return false
REDACTED
	for _, r := range subject {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r == '_' || r == '-':
		default:
			return false
	REDACTED
REDACTED
	return true
REDACTED

func linuxDoSyntheticEmail(subject string) string {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return ""
REDACTED
	return "linuxdo-" + subject + service.LinuxDoConnectSyntheticEmailDomain
REDACTED
