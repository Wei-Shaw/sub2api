package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	DimAgentOAuthClientID    = "4183be708e6b40a1b25c614e932976d0"
	DimAgentOAuthRedirectURI = "http://localhost:63211/auth/callback"
	DimAgentOAuthScope       = "openid profile email"
	DimAgentOAuthIssuer      = "https://dimagent.cn"

	dimAgentOAuthSessionTTL = 30 * time.Minute
	dimAgentRefreshSkew     = time.Minute
	dimAgentDefaultTokenTTL = time.Hour
)

// DimAgentOAuthClient performs the provider's PKCE token exchange and refresh.
// The client never returns a raw upstream response body so authorization codes
// and credentials cannot leak through an administrative error response.
type DimAgentOAuthClient interface {
	ExchangeCode(ctx context.Context, code, codeVerifier, redirectURI, proxyURL string) (*DimAgentOAuthTokenResponse, error)
	RefreshToken(ctx context.Context, refreshToken, tokenEndpoint, proxyURL string) (*DimAgentOAuthTokenResponse, error)
}

type dimAgentOAuthSession struct {
	State        string
	CodeVerifier string
	ProxyURL     string
	CreatedAt    time.Time
}

type dimAgentOAuthSessionStore struct {
	mu       sync.Mutex
	sessions map[string]dimAgentOAuthSession
}

func newDimAgentOAuthSessionStore() *dimAgentOAuthSessionStore {
	return &dimAgentOAuthSessionStore{sessions: make(map[string]dimAgentOAuthSession)}
}

func (s *dimAgentOAuthSessionStore) set(id string, session dimAgentOAuthSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, item := range s.sessions {
		if time.Since(item.CreatedAt) > dimAgentOAuthSessionTTL {
			delete(s.sessions, key)
		}
	}
	s.sessions[id] = session
}

func (s *dimAgentOAuthSessionStore) get(id string) (dimAgentOAuthSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[id]
	if !ok || time.Since(session.CreatedAt) > dimAgentOAuthSessionTTL {
		delete(s.sessions, id)
		return dimAgentOAuthSession{}, false
	}
	return session, true
}

// consume makes authorization-code sessions single-use. On an exchange failure
// an operator must start a fresh login rather than replaying a code or verifier.
func (s *dimAgentOAuthSessionStore) consume(id string) (dimAgentOAuthSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[id]
	if !ok || time.Since(session.CreatedAt) > dimAgentOAuthSessionTTL {
		delete(s.sessions, id)
		return dimAgentOAuthSession{}, false
	}
	delete(s.sessions, id)
	return session, true
}

// DimAgentOAuthService implements the exact browser authorization contract used
// by the installed DSH DimAgent bundle. The registered OAuth redirect is a
// localhost URI, therefore an administrator pastes that final callback URL into
// Sub2API after browser authorization; Sub2API never asks for an access token.
type DimAgentOAuthService struct {
	sessions  *dimAgentOAuthSessionStore
	proxyRepo ProxyRepository
	client    DimAgentOAuthClient
}

func NewDimAgentOAuthService(proxyRepo ProxyRepository, client DimAgentOAuthClient) *DimAgentOAuthService {
	return &DimAgentOAuthService{sessions: newDimAgentOAuthSessionStore(), proxyRepo: proxyRepo, client: client}
}

type DimAgentAuthURLResult struct {
	AuthURL     string `json:"auth_url"`
	SessionID   string `json:"session_id"`
	RedirectURI string `json:"redirect_uri"`
}

type DimAgentExchangeCodeInput struct {
	SessionID   string
	CallbackURL string
	ProxyID     *int64
}

type DimAgentOAuthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope,omitempty"`
}

type DimAgentTokenInfo struct {
	AccessToken   string `json:"access_token"`
	RefreshToken  string `json:"refresh_token,omitempty"`
	ExpiresIn     int64  `json:"expires_in"`
	ExpiresAt     int64  `json:"expires_at"`
	Issuer        string `json:"issuer"`
	TokenEndpoint string `json:"token_endpoint"`
	RelayBaseURL  string `json:"relay_base_url"`
	Scope         string `json:"scope"`
}

func (s *DimAgentOAuthService) GenerateAuthURL(ctx context.Context, proxyID *int64) (*DimAgentAuthURLResult, error) {
	proxyURL, err := s.proxyURL(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	state, err := dimAgentRandomURLSafe(32)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "DIMAGENT_OAUTH_STATE_FAILED", "generate OAuth state: %v", err)
	}
	verifier, err := dimAgentRandomURLSafe(32)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "DIMAGENT_OAUTH_VERIFIER_FAILED", "generate PKCE verifier: %v", err)
	}
	sessionID, err := dimAgentRandomURLSafe(16)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "DIMAGENT_OAUTH_SESSION_FAILED", "generate OAuth session: %v", err)
	}
	challengeSum := sha256.Sum256([]byte(verifier))
	values := url.Values{}
	values.Set("response_type", "code")
	values.Set("client_id", DimAgentOAuthClientID)
	values.Set("redirect_uri", DimAgentOAuthRedirectURI)
	values.Set("scope", DimAgentOAuthScope)
	values.Set("code_challenge", base64.RawURLEncoding.EncodeToString(challengeSum[:]))
	values.Set("code_challenge_method", "S256")
	values.Set("state", state)
	s.sessions.set(sessionID, dimAgentOAuthSession{State: state, CodeVerifier: verifier, ProxyURL: proxyURL, CreatedAt: time.Now()})
	return &DimAgentAuthURLResult{
		AuthURL:     DimAgentOAuthIssuer + "/oauth/authorize?" + values.Encode(),
		SessionID:   sessionID,
		RedirectURI: DimAgentOAuthRedirectURI,
	}, nil
}

func (s *DimAgentOAuthService) ExchangeCallbackURL(ctx context.Context, input *DimAgentExchangeCodeInput) (*DimAgentTokenInfo, error) {
	if input == nil || strings.TrimSpace(input.SessionID) == "" || strings.TrimSpace(input.CallbackURL) == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "DIMAGENT_OAUTH_INVALID_INPUT", "session_id and callback_url are required")
	}
	sessionID := strings.TrimSpace(input.SessionID)
	session, ok := s.sessions.get(sessionID)
	if !ok {
		return nil, infraerrors.New(http.StatusBadRequest, "DIMAGENT_OAUTH_SESSION_NOT_FOUND", "OAuth session not found or expired; generate a new authorization URL")
	}
	// The OAuth exchange always sends DimAgentOAuthRedirectURI (the registered
	// localhost value) to the token endpoint. The pasted URL is only a secure
	// carrier for code + state, and an administrator may open that localhost URL
	// through another reachable host (for example a TLS reverse proxy) to make it
	// viewable. Its host and scheme are therefore intentionally not an
	// authorization boundary; the path, query shape and the PKCE-bound state
	// below are the security checks.
	callback, err := url.Parse(strings.TrimSpace(input.CallbackURL))
	if err != nil || (callback.Scheme != "http" && callback.Scheme != "https") || callback.Host == "" || callback.Path != "/auth/callback" || callback.User != nil || callback.Fragment != "" {
		return nil, infraerrors.New(http.StatusBadRequest, "DIMAGENT_OAUTH_CALLBACK_INVALID", "callback_url must be an http(s) /auth/callback URL containing DimAgent code and state")
	}
	if callback.Query().Get("error") != "" {
		return nil, infraerrors.New(http.StatusBadRequest, "DIMAGENT_OAUTH_PROVIDER_REJECTED", "DimAgent authorization was rejected")
	}
	code := strings.TrimSpace(callback.Query().Get("code"))
	state := strings.TrimSpace(callback.Query().Get("state"))
	if code == "" || state == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "DIMAGENT_OAUTH_CALLBACK_INCOMPLETE", "callback_url is missing code or state")
	}
	if subtle.ConstantTimeCompare([]byte(state), []byte(session.State)) != 1 {
		return nil, infraerrors.New(http.StatusBadRequest, "DIMAGENT_OAUTH_INVALID_STATE", "OAuth callback state does not match the authorization session")
	}
	// Only consume after the pasted URL has passed local shape + state checks.
	// A typo must not force a fresh browser authorization, but a valid callback
	// remains single-use once it reaches the code-exchange boundary.
	session, ok = s.sessions.consume(sessionID)
	if !ok {
		return nil, infraerrors.New(http.StatusBadRequest, "DIMAGENT_OAUTH_SESSION_ALREADY_USED", "OAuth session was already used or expired")
	}
	proxyURL := session.ProxyURL
	if input.ProxyID != nil {
		proxyURL, err = s.proxyURL(ctx, input.ProxyID)
		if err != nil {
			return nil, err
		}
	}
	if s.client == nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "DIMAGENT_OAUTH_CLIENT_NOT_CONFIGURED", "DimAgent OAuth client is not configured")
	}
	response, err := s.client.ExchangeCode(ctx, code, session.CodeVerifier, DimAgentOAuthRedirectURI, proxyURL)
	if err != nil {
		return nil, err
	}
	return dimAgentTokenInfoFromResponse(response, ""), nil
}

func (s *DimAgentOAuthService) RefreshAccountToken(ctx context.Context, account *Account) (*DimAgentTokenInfo, error) {
	if account == nil || !account.IsDimAgent() || account.Type != AccountTypeOAuth {
		return nil, infraerrors.New(http.StatusBadRequest, "DIMAGENT_OAUTH_INVALID_ACCOUNT", "account is not a DimAgent OAuth account")
	}
	refreshToken := strings.TrimSpace(account.GetCredential("refresh_token"))
	if refreshToken == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "DIMAGENT_OAUTH_REFRESH_TOKEN_REQUIRED", "DimAgent refresh token is missing; reauthorize the account")
	}
	if s.client == nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "DIMAGENT_OAUTH_CLIENT_NOT_CONFIGURED", "DimAgent OAuth client is not configured")
	}
	proxyURL, err := s.proxyURL(ctx, account.ProxyID)
	if err != nil {
		return nil, err
	}
	response, err := s.client.RefreshToken(ctx, refreshToken, account.GetCredential("token_endpoint"), proxyURL)
	if err != nil {
		return nil, err
	}
	return dimAgentTokenInfoFromResponse(response, refreshToken), nil
}

func (s *DimAgentOAuthService) BuildAccountCredentials(info *DimAgentTokenInfo) map[string]any {
	if info == nil {
		return nil
	}
	credentials := map[string]any{
		"access_token":   info.AccessToken,
		"expires_at":     time.Unix(info.ExpiresAt, 0).UTC().Format(time.RFC3339),
		"issuer":         info.Issuer,
		"token_endpoint": info.TokenEndpoint,
		"relay_base_url": info.RelayBaseURL,
		"scope":          info.Scope,
	}
	if strings.TrimSpace(info.RefreshToken) != "" {
		credentials["refresh_token"] = info.RefreshToken
	}
	return credentials
}

func dimAgentTokenInfoFromResponse(response *DimAgentOAuthTokenResponse, existingRefresh string) *DimAgentTokenInfo {
	if response == nil {
		return nil
	}
	expiresIn := response.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = int64(dimAgentDefaultTokenTTL / time.Second)
	}
	refresh := strings.TrimSpace(response.RefreshToken)
	if refresh == "" {
		refresh = strings.TrimSpace(existingRefresh)
	}
	return &DimAgentTokenInfo{
		AccessToken:   strings.TrimSpace(response.AccessToken),
		RefreshToken:  refresh,
		ExpiresIn:     expiresIn,
		ExpiresAt:     time.Now().Add(time.Duration(expiresIn) * time.Second).Unix(),
		Issuer:        DimAgentOAuthIssuer,
		TokenEndpoint: DimAgentOAuthIssuer + "/oauth/token",
		RelayBaseURL:  DimAgentOAuthIssuer + "/v1",
		Scope:         dimAgentFirstNonEmpty(strings.TrimSpace(response.Scope), DimAgentOAuthScope),
	}
}

func (s *DimAgentOAuthService) proxyURL(ctx context.Context, proxyID *int64) (string, error) {
	if proxyID == nil {
		return "", nil
	}
	if s.proxyRepo == nil {
		return "", infraerrors.New(http.StatusBadRequest, "DIMAGENT_OAUTH_PROXY_UNAVAILABLE", "proxy repository is not configured")
	}
	proxy, err := s.proxyRepo.GetByID(ctx, *proxyID)
	if err != nil {
		return "", infraerrors.Newf(http.StatusBadRequest, "DIMAGENT_OAUTH_PROXY_NOT_FOUND", "proxy not found: %v", err)
	}
	if proxy == nil {
		return "", infraerrors.New(http.StatusBadRequest, "DIMAGENT_OAUTH_PROXY_NOT_FOUND", "proxy not found")
	}
	return proxy.URL(), nil
}

func dimAgentFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func dimAgentRandomURLSafe(size int) (string, error) {
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func (a *Account) DimAgentAccessToken() string {
	if a == nil || !a.IsDimAgent() || a.Type != AccountTypeOAuth {
		return ""
	}
	return strings.TrimSpace(a.GetCredential("access_token"))
}

func (a *Account) DimAgentRefreshToken() string {
	if a == nil || !a.IsDimAgent() || a.Type != AccountTypeOAuth {
		return ""
	}
	return strings.TrimSpace(a.GetCredential("refresh_token"))
}

func (a *Account) DimAgentNeedsRefresh(window time.Duration) bool {
	if a == nil || !a.IsDimAgent() || a.Type != AccountTypeOAuth || a.DimAgentRefreshToken() == "" {
		return false
	}
	expiresAt := a.GetCredentialAsTime("expires_at")
	return expiresAt == nil || time.Until(*expiresAt) <= window
}
