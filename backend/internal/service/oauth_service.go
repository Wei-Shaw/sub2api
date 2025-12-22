package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"sub2api/internal/model"
	"sub2api/internal/pkg/oauth"
	"sub2api/internal/service/ports"
)

// ClaudeOAuthClient handles HTTP requests for Claude OAuth flows
type ClaudeOAuthClient interface {
	GetOrganizationUUID(ctx context.Context, sessionKey, proxyURL string) (string, error)
	GetAuthorizationCode(ctx context.Context, sessionKey, orgUUID, scope, codeChallenge, state, proxyURL string) (string, error)
	ExchangeCodeForToken(ctx context.Context, code, codeVerifier, state, proxyURL string) (*oauth.TokenResponse, error)
	RefreshToken(ctx context.Context, refreshToken, proxyURL string) (*oauth.TokenResponse, error)
REDACTED

// OAuthService handles OAuth authentication flows
type OAuthService struct {
	sessionStore *oauth.SessionStore
	proxyRepo    ports.ProxyRepository
	oauthClient  ClaudeOAuthClient
REDACTED

// NewOAuthService creates a new OAuth service
func NewOAuthService(proxyRepo ports.ProxyRepository, oauthClient ClaudeOAuthClient) *OAuthService {
	return &OAuthService{
		sessionStore: oauth.NewSessionStore(),
		proxyRepo:    proxyRepo,
		oauthClient:  oauthClient,
REDACTED
REDACTED

// GenerateAuthURLResult contains the authorization URL and session info
type GenerateAuthURLResult struct {
	AuthURL   string `json:"auth_url"`
	SessionID string `json:"session_id"`
REDACTED

// GenerateAuthURL generates an OAuth authorization URL with full scope
func (s *OAuthService) GenerateAuthURL(ctx context.Context, proxyID *int64) (*GenerateAuthURLResult, error) {
	scope := fmt.Sprintf("%s %s", oauth.ScopeProfile, oauth.ScopeInference)
	return s.generateAuthURLWithScope(ctx, scope, proxyID)
REDACTED

// GenerateSetupTokenURL generates an OAuth authorization URL for setup token (inference only)
func (s *OAuthService) GenerateSetupTokenURL(ctx context.Context, proxyID *int64) (*GenerateAuthURLResult, error) {
	scope := oauth.ScopeInference
	return s.generateAuthURLWithScope(ctx, scope, proxyID)
REDACTED

func (s *OAuthService) generateAuthURLWithScope(ctx context.Context, scope string, proxyID *int64) (*GenerateAuthURLResult, error) {
	// Generate PKCE values
	state, err := oauth.GenerateState()
	if err != nil {
		return nil, fmt.Errorf("failed to generate state: %w", err)
REDACTED

	codeVerifier, err := oauth.GenerateCodeVerifier()
	if err != nil {
		return nil, fmt.Errorf("failed to generate code verifier: %w", err)
REDACTED

	codeChallenge := oauth.GenerateCodeChallenge(codeVerifier)

	// Generate session ID
	sessionID, err := oauth.GenerateSessionID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate session ID: %w", err)
REDACTED

	// Get proxy URL if specified
	var proxyURL string
	if proxyID != nil {
		proxy, err := s.proxyRepo.GetByID(ctx, *proxyID)
		if err == nil && proxy != nil {
			proxyURL = proxy.URL()
	REDACTED
REDACTED

	// Store session
	session := &oauth.OAuthSession{
		State:        state,
		CodeVerifier: codeVerifier,
		Scope:        scope,
		ProxyURL:     proxyURL,
		CreatedAt:    time.Now(),
REDACTED
	s.sessionStore.Set(sessionID, session)

	// Build authorization URL
	authURL := oauth.BuildAuthorizationURL(state, codeChallenge, scope)

	return &GenerateAuthURLResult{
		AuthURL:   authURL,
		SessionID: sessionID,
REDACTED, nil
REDACTED

// ExchangeCodeInput represents the input for code exchange
type ExchangeCodeInput struct {
	SessionID string
	Code      string
	ProxyID   *int64
REDACTED

// TokenInfo represents the token information stored in credentials
type TokenInfo struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	ExpiresAt    int64  `json:"expires_at"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
	OrgUUID      string `json:"org_uuid,omitempty"`
	AccountUUID  string `json:"account_uuid,omitempty"`
REDACTED

// ExchangeCode exchanges authorization code for tokens
func (s *OAuthService) ExchangeCode(ctx context.Context, input *ExchangeCodeInput) (*TokenInfo, error) {
	// Get session
	session, ok := s.sessionStore.Get(input.SessionID)
	if !ok {
		return nil, fmt.Errorf("session not found or expired")
REDACTED

	// Get proxy URL
	proxyURL := session.ProxyURL
	if input.ProxyID != nil {
		proxy, err := s.proxyRepo.GetByID(ctx, *input.ProxyID)
		if err == nil && proxy != nil {
			proxyURL = proxy.URL()
	REDACTED
REDACTED

	// Exchange code for token
	tokenInfo, err := s.exchangeCodeForToken(ctx, input.Code, session.CodeVerifier, session.State, proxyURL)
	if err != nil {
		return nil, err
REDACTED

	// Delete session after successful exchange
	s.sessionStore.Delete(input.SessionID)

	return tokenInfo, nil
REDACTED

// CookieAuthInput represents the input for cookie-based authentication
type CookieAuthInput struct {
	SessionKey string
	ProxyID    *int64
	Scope      string // "full" or "inference"
REDACTED

// CookieAuth performs OAuth using sessionKey (cookie-based auto-auth)
func (s *OAuthService) CookieAuth(ctx context.Context, input *CookieAuthInput) (*TokenInfo, error) {
	// Get proxy URL if specified
	var proxyURL string
	if input.ProxyID != nil {
		proxy, err := s.proxyRepo.GetByID(ctx, *input.ProxyID)
		if err == nil && proxy != nil {
			proxyURL = proxy.URL()
	REDACTED
REDACTED

	// Determine scope
	scope := fmt.Sprintf("%s %s", oauth.ScopeProfile, oauth.ScopeInference)
	if input.Scope == "inference" {
		scope = oauth.ScopeInference
REDACTED

	// Step 1: Get organization info using sessionKey
	orgUUID, err := s.getOrganizationUUID(ctx, input.SessionKey, proxyURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization info: %w", err)
REDACTED

	// Step 2: Generate PKCE values
	codeVerifier, err := oauth.GenerateCodeVerifier()
	if err != nil {
		return nil, fmt.Errorf("failed to generate code verifier: %w", err)
REDACTED
	codeChallenge := oauth.GenerateCodeChallenge(codeVerifier)

	state, err := oauth.GenerateState()
	if err != nil {
		return nil, fmt.Errorf("failed to generate state: %w", err)
REDACTED

	// Step 3: Get authorization code using cookie
	authCode, err := s.getAuthorizationCode(ctx, input.SessionKey, orgUUID, scope, codeChallenge, state, proxyURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get authorization code: %w", err)
REDACTED

	// Step 4: Exchange code for token
	tokenInfo, err := s.exchangeCodeForToken(ctx, authCode, codeVerifier, state, proxyURL)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code: %w", err)
REDACTED

	// Ensure org_uuid is set (from step 1 if not from token response)
	if tokenInfo.OrgUUID == "" && orgUUID != "" {
		tokenInfo.OrgUUID = orgUUID
		log.Printf("[OAuth] Set org_uuid from cookie auth: %s", orgUUID)
REDACTED

	return tokenInfo, nil
REDACTED

// getOrganizationUUID gets the organization UUID from claude.ai using sessionKey
func (s *OAuthService) getOrganizationUUID(ctx context.Context, sessionKey, proxyURL string) (string, error) {
	return s.oauthClient.GetOrganizationUUID(ctx, sessionKey, proxyURL)
REDACTED

// getAuthorizationCode gets the authorization code using sessionKey
func (s *OAuthService) getAuthorizationCode(ctx context.Context, sessionKey, orgUUID, scope, codeChallenge, state, proxyURL string) (string, error) {
	return s.oauthClient.GetAuthorizationCode(ctx, sessionKey, orgUUID, scope, codeChallenge, state, proxyURL)
REDACTED

// exchangeCodeForToken exchanges authorization code for tokens
func (s *OAuthService) exchangeCodeForToken(ctx context.Context, code, codeVerifier, state, proxyURL string) (*TokenInfo, error) {
	tokenResp, err := s.oauthClient.ExchangeCodeForToken(ctx, code, codeVerifier, state, proxyURL)
	if err != nil {
		return nil, err
REDACTED

	tokenInfo := &TokenInfo{
		AccessToken:  tokenResp.AccessToken,
		TokenType:    tokenResp.TokenType,
		ExpiresIn:    tokenResp.ExpiresIn,
		ExpiresAt:    time.Now().Unix() + tokenResp.ExpiresIn,
		RefreshToken: tokenResp.RefreshToken,
		Scope:        tokenResp.Scope,
REDACTED

	if tokenResp.Organization != nil && tokenResp.Organization.UUID != "" {
		tokenInfo.OrgUUID = tokenResp.Organization.UUID
		log.Printf("[OAuth] Got org_uuid: %s", tokenInfo.OrgUUID)
REDACTED
	if tokenResp.Account != nil && tokenResp.Account.UUID != "" {
		tokenInfo.AccountUUID = tokenResp.Account.UUID
		log.Printf("[OAuth] Got account_uuid: %s", tokenInfo.AccountUUID)
REDACTED

	return tokenInfo, nil
REDACTED

// RefreshToken refreshes an OAuth token
func (s *OAuthService) RefreshToken(ctx context.Context, refreshToken string, proxyURL string) (*TokenInfo, error) {
	tokenResp, err := s.oauthClient.RefreshToken(ctx, refreshToken, proxyURL)
	if err != nil {
		return nil, err
REDACTED

	return &TokenInfo{
		AccessToken:  tokenResp.AccessToken,
		TokenType:    tokenResp.TokenType,
		ExpiresIn:    tokenResp.ExpiresIn,
		ExpiresAt:    time.Now().Unix() + tokenResp.ExpiresIn,
		RefreshToken: tokenResp.RefreshToken,
		Scope:        tokenResp.Scope,
REDACTED, nil
REDACTED

// RefreshAccountToken refreshes token for an account
func (s *OAuthService) RefreshAccountToken(ctx context.Context, account *model.Account) (*TokenInfo, error) {
	refreshToken := account.GetCredential("refresh_token")
	if refreshToken == "" {
		return nil, fmt.Errorf("no refresh token available")
REDACTED

	var proxyURL string
	if account.ProxyID != nil {
		proxy, err := s.proxyRepo.GetByID(ctx, *account.ProxyID)
		if err == nil && proxy != nil {
			proxyURL = proxy.URL()
	REDACTED
REDACTED

	return s.RefreshToken(ctx, refreshToken, proxyURL)
REDACTED

// Stop stops the session store cleanup goroutine
func (s *OAuthService) Stop() {
	s.sessionStore.Stop()
REDACTED
