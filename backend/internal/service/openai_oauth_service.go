package service

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

var openAISoraSessionAuthURL = "https://sora.chatgpt.com/api/auth/session"

// OpenAIOAuthService handles OpenAI OAuth authentication flows
type OpenAIOAuthService struct {
	sessionStore *openai.SessionStore
	proxyRepo    ProxyRepository
	oauthClient  OpenAIOAuthClient
REDACTED

// NewOpenAIOAuthService creates a new OpenAI OAuth service
func NewOpenAIOAuthService(proxyRepo ProxyRepository, oauthClient OpenAIOAuthClient) *OpenAIOAuthService {
	return &OpenAIOAuthService{
		sessionStore: openai.NewSessionStore(),
		proxyRepo:    proxyRepo,
		oauthClient:  oauthClient,
REDACTED
REDACTED

// OpenAIAuthURLResult contains the authorization URL and session info
type OpenAIAuthURLResult struct {
	AuthURL   string `json:"auth_url"`
	SessionID string `json:"session_id"`
REDACTED

// GenerateAuthURL generates an OpenAI OAuth authorization URL
func (s *OpenAIOAuthService) GenerateAuthURL(ctx context.Context, proxyID *int64, redirectURI string) (*OpenAIAuthURLResult, error) {
	// Generate PKCE values
	state, err := openai.GenerateState()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "OPENAI_OAUTH_STATE_FAILED", "failed to generate state: %v", err)
REDACTED

	codeVerifier, err := openai.GenerateCodeVerifier()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "OPENAI_OAUTH_VERIFIER_FAILED", "failed to generate code verifier: %v", err)
REDACTED

	codeChallenge := openai.GenerateCodeChallenge(codeVerifier)

	// Generate session ID
	sessionID, err := openai.GenerateSessionID()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "OPENAI_OAUTH_SESSION_FAILED", "failed to generate session ID: %v", err)
REDACTED

	// Get proxy URL if specified
	var proxyURL string
	if proxyID != nil {
		proxy, err := s.proxyRepo.GetByID(ctx, *proxyID)
		if err != nil {
			return nil, infraerrors.Newf(http.StatusBadRequest, "OPENAI_OAUTH_PROXY_NOT_FOUND", "proxy not found: %v", err)
	REDACTED
		if proxy != nil {
			proxyURL = proxy.URL()
	REDACTED
REDACTED

	// Use default redirect URI if not specified
	if redirectURI == "" {
		redirectURI = openai.DefaultRedirectURI
REDACTED

	// Store session
	session := &openai.OAuthSession{
		State:        state,
		CodeVerifier: codeVerifier,
		RedirectURI:  redirectURI,
		ProxyURL:     proxyURL,
		CreatedAt:    time.Now(),
REDACTED
	s.sessionStore.Set(sessionID, session)

	// Build authorization URL
	authURL := openai.BuildAuthorizationURL(state, codeChallenge, redirectURI)

	return &OpenAIAuthURLResult{
		AuthURL:   authURL,
		SessionID: sessionID,
REDACTED, nil
REDACTED

// OpenAIExchangeCodeInput represents the input for code exchange
type OpenAIExchangeCodeInput struct {
	SessionID   string
	Code        string
	State       string
	RedirectURI string
	ProxyID     *int64
REDACTED

// OpenAITokenInfo represents the token information for OpenAI
type OpenAITokenInfo struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	IDToken          string `json:"id_token,omitempty"`
	ExpiresIn        int64  `json:"expires_in"`
	ExpiresAt        int64  `json:"expires_at"`
	Email            string `json:"email,omitempty"`
	ChatGPTAccountID string `json:"chatgpt_account_id,omitempty"`
	ChatGPTUserID    string `json:"chatgpt_user_id,omitempty"`
	OrganizationID   string `json:"organization_id,omitempty"`
REDACTED

// ExchangeCode exchanges authorization code for tokens
func (s *OpenAIOAuthService) ExchangeCode(ctx context.Context, input *OpenAIExchangeCodeInput) (*OpenAITokenInfo, error) {
	// Get session
	session, ok := s.sessionStore.Get(input.SessionID)
	if !ok {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_OAUTH_SESSION_NOT_FOUND", "session not found or expired")
REDACTED
	if input.State == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_OAUTH_STATE_REQUIRED", "oauth state is required")
REDACTED
	if subtle.ConstantTimeCompare([]byte(input.State), []byte(session.State)) != 1 {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_OAUTH_INVALID_STATE", "invalid oauth state")
REDACTED

	// Get proxy URL: prefer input.ProxyID, fallback to session.ProxyURL
	proxyURL := session.ProxyURL
	if input.ProxyID != nil {
		proxy, err := s.proxyRepo.GetByID(ctx, *input.ProxyID)
		if err != nil {
			return nil, infraerrors.Newf(http.StatusBadRequest, "OPENAI_OAUTH_PROXY_NOT_FOUND", "proxy not found: %v", err)
	REDACTED
		if proxy != nil {
			proxyURL = proxy.URL()
	REDACTED
REDACTED

	// Use redirect URI from session or input
	redirectURI := session.RedirectURI
	if input.RedirectURI != "" {
		redirectURI = input.RedirectURI
REDACTED

	// Exchange code for token
	tokenResp, err := s.oauthClient.ExchangeCode(ctx, input.Code, session.CodeVerifier, redirectURI, proxyURL)
	if err != nil {
		return nil, err
REDACTED

	// Parse ID token to get user info
	var userInfo *openai.UserInfo
	if tokenResp.IDToken != "" {
		claims, err := openai.ParseIDToken(tokenResp.IDToken)
		if err == nil {
			userInfo = claims.GetUserInfo()
	REDACTED
REDACTED

	// Delete session after successful exchange
	s.sessionStore.Delete(input.SessionID)

	tokenInfo := &OpenAITokenInfo{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		IDToken:      tokenResp.IDToken,
		ExpiresIn:    int64(tokenResp.ExpiresIn),
		ExpiresAt:    time.Now().Unix() + int64(tokenResp.ExpiresIn),
REDACTED

	if userInfo != nil {
		tokenInfo.Email = userInfo.Email
		tokenInfo.ChatGPTAccountID = userInfo.ChatGPTAccountID
		tokenInfo.ChatGPTUserID = userInfo.ChatGPTUserID
		tokenInfo.OrganizationID = userInfo.OrganizationID
REDACTED

	return tokenInfo, nil
REDACTED

// RefreshToken refreshes an OpenAI OAuth token
func (s *OpenAIOAuthService) RefreshToken(ctx context.Context, refreshToken string, proxyURL string) (*OpenAITokenInfo, error) {
	return s.RefreshTokenWithClientID(ctx, refreshToken, proxyURL, "")
REDACTED

// RefreshTokenWithClientID refreshes an OpenAI/Sora OAuth token with optional client_id.
func (s *OpenAIOAuthService) RefreshTokenWithClientID(ctx context.Context, refreshToken string, proxyURL string, clientID string) (*OpenAITokenInfo, error) {
	tokenResp, err := s.oauthClient.RefreshTokenWithClientID(ctx, refreshToken, proxyURL, clientID)
	if err != nil {
		return nil, err
REDACTED

	// Parse ID token to get user info
	var userInfo *openai.UserInfo
	if tokenResp.IDToken != "" {
		claims, err := openai.ParseIDToken(tokenResp.IDToken)
		if err == nil {
			userInfo = claims.GetUserInfo()
	REDACTED
REDACTED

	tokenInfo := &OpenAITokenInfo{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		IDToken:      tokenResp.IDToken,
		ExpiresIn:    int64(tokenResp.ExpiresIn),
		ExpiresAt:    time.Now().Unix() + int64(tokenResp.ExpiresIn),
REDACTED

	if userInfo != nil {
		tokenInfo.Email = userInfo.Email
		tokenInfo.ChatGPTAccountID = userInfo.ChatGPTAccountID
		tokenInfo.ChatGPTUserID = userInfo.ChatGPTUserID
		tokenInfo.OrganizationID = userInfo.OrganizationID
REDACTED

	return tokenInfo, nil
REDACTED

// ExchangeSoraSessionToken exchanges Sora session_token to access_token.
func (s *OpenAIOAuthService) ExchangeSoraSessionToken(ctx context.Context, sessionToken string, proxyID *int64) (*OpenAITokenInfo, error) {
	if strings.TrimSpace(sessionToken) == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "SORA_SESSION_TOKEN_REQUIRED", "session_token is required")
REDACTED

	proxyURL, err := s.resolveProxyURL(ctx, proxyID)
	if err != nil {
		return nil, err
REDACTED

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, openAISoraSessionAuthURL, nil)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "SORA_SESSION_REQUEST_BUILD_FAILED", "failed to build request: %v", err)
REDACTED
	req.Header.Set("Cookie", "__Secure-next-auth.session-token="+strings.TrimSpace(sessionToken))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Origin", "https://sora.chatgpt.com")
	req.Header.Set("Referer", "https://sora.chatgpt.com/")
	req.Header.Set("User-Agent", "Sora/1.2026.007 (Android 15; 24122RKC7C; build 2600700)")

	client := newOpenAIOAuthHTTPClient(proxyURL)
	resp, err := client.Do(req)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "SORA_SESSION_REQUEST_FAILED", "request failed: %v", err)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, infraerrors.Newf(http.StatusBadGateway, "SORA_SESSION_EXCHANGE_FAILED", "status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
REDACTED

	var sessionResp struct {
		AccessToken string `json:"accessToken"`
		Expires     string `json:"expires"`
		User        struct {
			Email string `json:"email"`
			Name  string `json:"name"`
	REDACTED `json:"user"`
REDACTED
	if err := json.Unmarshal(body, &sessionResp); err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "SORA_SESSION_PARSE_FAILED", "failed to parse response: %v", err)
REDACTED
	if strings.TrimSpace(sessionResp.AccessToken) == "" {
		return nil, infraerrors.New(http.StatusBadGateway, "SORA_SESSION_ACCESS_TOKEN_MISSING", "session exchange response missing access token")
REDACTED

	expiresAt := time.Now().Add(time.Hour).Unix()
	if strings.TrimSpace(sessionResp.Expires) != "" {
		if parsed, parseErr := time.Parse(time.RFC3339, sessionResp.Expires); parseErr == nil {
			expiresAt = parsed.Unix()
	REDACTED
REDACTED
	expiresIn := expiresAt - time.Now().Unix()
	if expiresIn < 0 {
		expiresIn = 0
REDACTED

	return &OpenAITokenInfo{
		AccessToken: strings.TrimSpace(sessionResp.AccessToken),
		ExpiresIn:   expiresIn,
		ExpiresAt:   expiresAt,
		Email:       strings.TrimSpace(sessionResp.User.Email),
REDACTED, nil
REDACTED

// RefreshAccountToken refreshes token for an OpenAI/Sora OAuth account
func (s *OpenAIOAuthService) RefreshAccountToken(ctx context.Context, account *Account) (*OpenAITokenInfo, error) {
	if account.Platform != PlatformOpenAI && account.Platform != PlatformSora {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_OAUTH_INVALID_ACCOUNT", "account is not an OpenAI/Sora account")
REDACTED
	if account.Type != AccountTypeOAuth {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_OAUTH_INVALID_ACCOUNT_TYPE", "account is not an OAuth account")
REDACTED

	refreshToken := account.GetCredential("refresh_token")
	if refreshToken == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_OAUTH_NO_REFRESH_TOKEN", "no refresh token available")
REDACTED

	var proxyURL string
	if account.ProxyID != nil {
		proxy, err := s.proxyRepo.GetByID(ctx, *account.ProxyID)
		if err == nil && proxy != nil {
			proxyURL = proxy.URL()
	REDACTED
REDACTED

	clientID := account.GetCredential("client_id")
	return s.RefreshTokenWithClientID(ctx, refreshToken, proxyURL, clientID)
REDACTED

// BuildAccountCredentials builds credentials map from token info
func (s *OpenAIOAuthService) BuildAccountCredentials(tokenInfo *OpenAITokenInfo) map[string]any {
	expiresAt := time.Unix(tokenInfo.ExpiresAt, 0).Format(time.RFC3339)

	creds := map[string]any{
		"access_token":  tokenInfo.AccessToken,
		"refresh_token": tokenInfo.RefreshToken,
		"expires_at":    expiresAt,
REDACTED

	if tokenInfo.IDToken != "" {
		creds["id_token"] = tokenInfo.IDToken
REDACTED
	if tokenInfo.Email != "" {
		creds["email"] = tokenInfo.Email
REDACTED
	if tokenInfo.ChatGPTAccountID != "" {
		creds["chatgpt_account_id"] = tokenInfo.ChatGPTAccountID
REDACTED
	if tokenInfo.ChatGPTUserID != "" {
		creds["chatgpt_user_id"] = tokenInfo.ChatGPTUserID
REDACTED
	if tokenInfo.OrganizationID != "" {
		creds["organization_id"] = tokenInfo.OrganizationID
REDACTED

	return creds
REDACTED

// Stop stops the session store cleanup goroutine
func (s *OpenAIOAuthService) Stop() {
	s.sessionStore.Stop()
REDACTED

func (s *OpenAIOAuthService) resolveProxyURL(ctx context.Context, proxyID *int64) (string, error) {
	if proxyID == nil {
		return "", nil
REDACTED
	proxy, err := s.proxyRepo.GetByID(ctx, *proxyID)
	if err != nil {
		return "", infraerrors.Newf(http.StatusBadRequest, "OPENAI_OAUTH_PROXY_NOT_FOUND", "proxy not found: %v", err)
REDACTED
	if proxy == nil {
		return "", nil
REDACTED
	return proxy.URL(), nil
REDACTED

func newOpenAIOAuthHTTPClient(proxyURL string) *http.Client {
	transport := &http.Transport{REDACTED
	if strings.TrimSpace(proxyURL) != "" {
		if parsed, err := url.Parse(proxyURL); err == nil && parsed.Host != "" {
			transport.Proxy = http.ProxyURL(parsed)
	REDACTED
REDACTED
	return &http.Client{
		Timeout:   120 * time.Second,
		Transport: transport,
REDACTED
REDACTED
