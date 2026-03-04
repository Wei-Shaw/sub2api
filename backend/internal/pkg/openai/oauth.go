package openai

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
)

// OpenAI OAuth Constants (from CRS project - Codex CLI client)
const (
	// OAuth Client ID for OpenAI (Codex CLI official)
	ClientID = "app_EMoamEEZ73f0CkXaXp7hrann"
	// OAuth Client ID for Sora mobile flow (aligned with sora2api)
	SoraClientID = "app_LlGpXReQgckcGGUo2JrYvtJK"

	// OAuth endpoints
	AuthorizeURL = "https://auth.openai.com/oauth/authorize"
	TokenURL     = "https://auth.openai.com/oauth/token"

	// Default redirect URI (can be customized)
	DefaultRedirectURI = "http://localhost:1455/auth/callback"

	// Scopes
	DefaultScopes = "openid profile email offline_access"
	// RefreshScopes - scope for token refresh (without offline_access, aligned with CRS project)
	RefreshScopes = "openid profile email"

	// Session TTL
	SessionTTL = 30 * time.Minute
)

const (
	// OAuthPlatformOpenAI uses OpenAI Codex-compatible OAuth client.
	OAuthPlatformOpenAI = "openai"
	// OAuthPlatformSora uses Sora OAuth client.
	OAuthPlatformSora = "sora"
)

// OAuthSession stores OAuth flow state for OpenAI
type OAuthSession struct {
	State        string    `json:"state"`
	CodeVerifier string    `json:"code_verifier"`
	ClientID     string    `json:"client_id,omitempty"`
	ProxyURL     string    `json:"proxy_url,omitempty"`
	RedirectURI  string    `json:"redirect_uri"`
	CreatedAt    time.Time `json:"created_at"`
REDACTED

// SessionStore manages OAuth sessions in memory
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*OAuthSession
	stopOnce sync.Once
	stopCh   chan struct{REDACTED
REDACTED

// NewSessionStore creates a new session store
func NewSessionStore() *SessionStore {
	store := &SessionStore{
		sessions: make(map[string]*OAuthSession),
		stopCh:   make(chan struct{REDACTED),
REDACTED
	// Start cleanup goroutine
	go store.cleanup()
	return store
REDACTED

// Set stores a session
func (s *SessionStore) Set(sessionID string, session *OAuthSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sessionID] = session
REDACTED

// Get retrieves a session
func (s *SessionStore) Get(sessionID string) (*OAuthSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok := s.sessions[sessionID]
	if !ok {
		return nil, false
REDACTED
	// Check if expired
	if time.Since(session.CreatedAt) > SessionTTL {
		return nil, false
REDACTED
	return session, true
REDACTED

// Delete removes a session
func (s *SessionStore) Delete(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
REDACTED

// Stop stops the cleanup goroutine
func (s *SessionStore) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
REDACTED)
REDACTED

// cleanup removes expired sessions periodically
func (s *SessionStore) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.mu.Lock()
			for id, session := range s.sessions {
				if time.Since(session.CreatedAt) > SessionTTL {
					delete(s.sessions, id)
			REDACTED
		REDACTED
			s.mu.Unlock()
	REDACTED
REDACTED
REDACTED

// GenerateRandomBytes generates cryptographically secure random bytes
func GenerateRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return nil, err
REDACTED
	return b, nil
REDACTED

// GenerateState generates a random state string for OAuth
func GenerateState() (string, error) {
	bytes, err := GenerateRandomBytes(32)
	if err != nil {
		return "", err
REDACTED
	return hex.EncodeToString(bytes), nil
REDACTED

// GenerateSessionID generates a unique session ID
func GenerateSessionID() (string, error) {
	bytes, err := GenerateRandomBytes(16)
	if err != nil {
		return "", err
REDACTED
	return hex.EncodeToString(bytes), nil
REDACTED

// GenerateCodeVerifier generates a PKCE code verifier (64 bytes -> hex for OpenAI)
// OpenAI uses hex encoding instead of base64url
func GenerateCodeVerifier() (string, error) {
	bytes, err := GenerateRandomBytes(64)
	if err != nil {
		return "", err
REDACTED
	return hex.EncodeToString(bytes), nil
REDACTED

// GenerateCodeChallenge generates a PKCE code challenge using S256 method
// Uses base64url encoding as per RFC 7636
func GenerateCodeChallenge(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))
	return base64URLEncode(hash[:])
REDACTED

// base64URLEncode encodes bytes to base64url without padding
func base64URLEncode(data []byte) string {
	encoded := base64.URLEncoding.EncodeToString(data)
	// Remove padding
	return strings.TrimRight(encoded, "=")
REDACTED

// BuildAuthorizationURL builds the OpenAI OAuth authorization URL
func BuildAuthorizationURL(state, codeChallenge, redirectURI string) string {
	return BuildAuthorizationURLForPlatform(state, codeChallenge, redirectURI, OAuthPlatformOpenAI)
REDACTED

// BuildAuthorizationURLForPlatform builds authorization URL by platform.
func BuildAuthorizationURLForPlatform(state, codeChallenge, redirectURI, platform string) string {
	if redirectURI == "" {
		redirectURI = DefaultRedirectURI
REDACTED

	clientID, codexFlow := OAuthClientConfigByPlatform(platform)

	params := url.Values{REDACTED
	params.Set("response_type", "code")
	params.Set("client_id", clientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("scope", DefaultScopes)
	params.Set("state", state)
	params.Set("code_challenge", codeChallenge)
	params.Set("code_challenge_method", "S256")
	// OpenAI specific parameters
	params.Set("id_token_add_organizations", "true")
	if codexFlow {
		params.Set("codex_cli_simplified_flow", "true")
REDACTED

	return fmt.Sprintf("%s?%s", AuthorizeURL, params.Encode())
REDACTED

// OAuthClientConfigByPlatform returns oauth client_id and whether codex simplified flow should be enabled.
// Sora 授权流程复用 Codex CLI 的 client_id（支持 localhost redirect_uri），
// 但不启用 codex_cli_simplified_flow；拿到的 access_token 绑定同一 OpenAI 账号，对 Sora API 同样可用。
func OAuthClientConfigByPlatform(platform string) (clientID string, codexFlow bool) {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case OAuthPlatformSora:
		return ClientID, false
	default:
		return ClientID, true
REDACTED
REDACTED

// TokenRequest represents the token exchange request body
type TokenRequest struct {
	GrantType    string `json:"grant_type"`
	ClientID     string `json:"client_id"`
	Code         string `json:"code"`
	RedirectURI  string `json:"redirect_uri"`
	CodeVerifier string `json:"code_verifier"`
REDACTED

// TokenResponse represents the token response from OpenAI OAuth
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
REDACTED

// RefreshTokenRequest represents the refresh token request
type RefreshTokenRequest struct {
	GrantType    string `json:"grant_type"`
	RefreshToken string `json:"refresh_token"`
	ClientID     string `json:"client_id"`
	Scope        string `json:"scope"`
REDACTED

// IDTokenClaims represents the claims from OpenAI ID Token
type IDTokenClaims struct {
	// Standard claims
	Sub           string   `json:"sub"`
	Email         string   `json:"email"`
	EmailVerified bool     `json:"email_verified"`
	Iss           string   `json:"iss"`
	Aud           []string `json:"aud"` // OpenAI returns aud as an array
	Exp           int64    `json:"exp"`
	Iat           int64    `json:"iat"`

	// OpenAI specific claims (nested under https://api.openai.com/auth)
	OpenAIAuth *OpenAIAuthClaims `json:"https://api.openai.com/auth,omitempty"`
REDACTED

// OpenAIAuthClaims represents the OpenAI specific auth claims
type OpenAIAuthClaims struct {
	ChatGPTAccountID string              `json:"chatgpt_account_id"`
	ChatGPTUserID    string              `json:"chatgpt_user_id"`
	UserID           string              `json:"user_id"`
	Organizations    []OrganizationClaim `json:"organizations"`
REDACTED

// OrganizationClaim represents an organization in the ID Token
type OrganizationClaim struct {
	ID        string `json:"id"`
	Role      string `json:"role"`
	Title     string `json:"title"`
	IsDefault bool   `json:"is_default"`
REDACTED

// BuildTokenRequest creates a token exchange request for OpenAI
func BuildTokenRequest(code, codeVerifier, redirectURI string) *TokenRequest {
	if redirectURI == "" {
		redirectURI = DefaultRedirectURI
REDACTED
	return &TokenRequest{
		GrantType:    "authorization_code",
		ClientID:     ClientID,
		Code:         code,
		RedirectURI:  redirectURI,
		CodeVerifier: codeVerifier,
REDACTED
REDACTED

// BuildRefreshTokenRequest creates a refresh token request for OpenAI
func BuildRefreshTokenRequest(refreshToken string) *RefreshTokenRequest {
	return &RefreshTokenRequest{
		GrantType:    "refresh_token",
		RefreshToken: refreshToken,
		ClientID:     ClientID,
		Scope:        RefreshScopes,
REDACTED
REDACTED

// ToFormData converts TokenRequest to URL-encoded form data
func (r *TokenRequest) ToFormData() string {
	params := url.Values{REDACTED
	params.Set("grant_type", r.GrantType)
	params.Set("client_id", r.ClientID)
	params.Set("code", r.Code)
	params.Set("redirect_uri", r.RedirectURI)
	params.Set("code_verifier", r.CodeVerifier)
	return params.Encode()
REDACTED

// ToFormData converts RefreshTokenRequest to URL-encoded form data
func (r *RefreshTokenRequest) ToFormData() string {
	params := url.Values{REDACTED
	params.Set("grant_type", r.GrantType)
	params.Set("client_id", r.ClientID)
	params.Set("refresh_token", r.RefreshToken)
	params.Set("scope", r.Scope)
	return params.Encode()
REDACTED

// ParseIDToken parses the ID Token JWT and extracts claims.
// 注意：当前仅解码 payload 并校验 exp，未验证 JWT 签名。
// 生产环境如需用 ID Token 做授权决策，应通过 OpenAI 的 JWKS 端点验证签名：
//
//	https://auth.openai.com/.well-known/jwks.json
func ParseIDToken(idToken string) (*IDTokenClaims, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT format: expected 3 parts, got %d", len(parts))
REDACTED

	// Decode payload (second part)
	payload := parts[1]
	// Add padding if necessary
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
REDACTED

	decoded, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		// Try standard encoding
		decoded, err = base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to decode JWT payload: %w", err)
	REDACTED
REDACTED

	var claims IDTokenClaims
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return nil, fmt.Errorf("failed to parse JWT claims: %w", err)
REDACTED

	// 校验 ID Token 是否已过期（允许 2 分钟时钟偏差，防止因服务器时钟略有差异误判刚颁发的令牌）
	const clockSkewTolerance = 120 // 秒
	now := time.Now().Unix()
	if claims.Exp > 0 && now > claims.Exp+clockSkewTolerance {
		return nil, fmt.Errorf("id_token has expired (exp: %d, now: %d, skew_tolerance: %ds)", claims.Exp, now, clockSkewTolerance)
REDACTED

	return &claims, nil
REDACTED

// UserInfo represents user information extracted from ID Token claims.
type UserInfo struct {
	Email            string
	ChatGPTAccountID string
	ChatGPTUserID    string
	UserID           string
	OrganizationID   string
	Organizations    []OrganizationClaim
REDACTED

// GetUserInfo extracts user info from ID Token claims
func (c *IDTokenClaims) GetUserInfo() *UserInfo {
	info := &UserInfo{
		Email: c.Email,
REDACTED

	if c.OpenAIAuth != nil {
		info.ChatGPTAccountID = c.OpenAIAuth.ChatGPTAccountID
		info.ChatGPTUserID = c.OpenAIAuth.ChatGPTUserID
		info.UserID = c.OpenAIAuth.UserID
		info.Organizations = c.OpenAIAuth.Organizations

		// Get default organization ID
		for _, org := range c.OpenAIAuth.Organizations {
			if org.IsDefault {
				info.OrganizationID = org.ID
				break
		REDACTED
	REDACTED
		// If no default, use first org
		if info.OrganizationID == "" && len(c.OpenAIAuth.Organizations) > 0 {
			info.OrganizationID = c.OpenAIAuth.Organizations[0].ID
	REDACTED
REDACTED

	return info
REDACTED
