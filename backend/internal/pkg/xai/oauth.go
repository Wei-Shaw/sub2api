package xai

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyurl"
	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyutil"
)

const (
	Issuer       = "https://auth.x.ai"
	DiscoveryURL = "https://auth.x.ai/.well-known/openid-configuration"
	ClientID     = "b1a00492-073a-47ea-816f-4c329264a828"
	Scope        = "openid profile email offline_access grok-cli:access api:access"

	RedirectHost = "127.0.0.1"
	CallbackPort = 56121
	RedirectPath = "/callback"
	SessionTTL   = 30 * time.Minute
)

var DefaultRedirectURI = fmt.Sprintf("http://%s:%d%s", RedirectHost, CallbackPort, RedirectPath)

type OAuthSession struct {
	State         string    `json:"state"`
	Nonce         string    `json:"nonce"`
	CodeVerifier  string    `json:"code_verifier"`
	RedirectURI   string    `json:"redirect_uri"`
	TokenEndpoint string    `json:"token_endpoint"`
	ProxyURL      string    `json:"proxy_url,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*OAuthSession
	stopOnce sync.Once
	stopCh   chan struct{}
}

func NewSessionStore() *SessionStore {
	store := &SessionStore{
		sessions: make(map[string]*OAuthSession),
		stopCh:   make(chan struct{}),
	}
	go store.cleanup()
	return store
}

func (s *SessionStore) Set(sessionID string, session *OAuthSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sessionID] = session
}

func (s *SessionStore) Get(sessionID string) (*OAuthSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok := s.sessions[sessionID]
	if !ok {
		return nil, false
	}
	if time.Since(session.CreatedAt) > SessionTTL {
		return nil, false
	}
	return session, true
}

func (s *SessionStore) Delete(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
}

func (s *SessionStore) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
}

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
				}
			}
			s.mu.Unlock()
		}
	}
}

func GenerateState() (string, error) {
	return randomHex(32)
}

func GenerateNonce() (string, error) {
	return randomHex(32)
}

func GenerateSessionID() (string, error) {
	return randomHex(16)
}

func GenerateCodeVerifier() (string, error) {
	b := make([]byte, 96)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func GenerateCodeChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func randomHex(size int) (string, error) {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

type DiscoveryDocument struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
}

type AuthorizeParams struct {
	AuthorizationEndpoint string
	State                 string
	Nonce                 string
	CodeChallenge         string
	RedirectURI           string
}

func BuildAuthorizeURL(params AuthorizeParams) (string, error) {
	endpoint := strings.TrimSpace(params.AuthorizationEndpoint)
	if err := ValidateOAuthEndpoint(endpoint, "authorization_endpoint"); err != nil {
		return "", err
	}
	redirectURI := strings.TrimSpace(params.RedirectURI)
	if redirectURI == "" {
		redirectURI = DefaultRedirectURI
	}

	values := url.Values{}
	values.Set("response_type", "code")
	values.Set("client_id", ClientID)
	values.Set("redirect_uri", redirectURI)
	values.Set("scope", Scope)
	values.Set("code_challenge", params.CodeChallenge)
	values.Set("code_challenge_method", "S256")
	values.Set("state", params.State)
	values.Set("nonce", params.Nonce)
	values.Set("plan", "generic")
	values.Set("referrer", "cli-proxy-api")
	return endpoint + "?" + values.Encode(), nil
}

func ValidateOAuthEndpoint(rawURL, field string) error {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed == nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("%s is invalid", field)
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("%s must use https", field)
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "x.ai" && !strings.HasSuffix(host, ".x.ai") {
		return fmt.Errorf("%s host is not trusted", field)
	}
	return nil
}

type Client struct {
	httpClient *http.Client
}

const (
	clientTimeout            = 15 * time.Second
	proxyDialTimeout         = 5 * time.Second
	proxyTLSHandshakeTimeout = 5 * time.Second
)

func NewClient(proxyURL string) (*Client, error) {
	httpClient := &http.Client{Timeout: clientTimeout}
	_, parsed, err := proxyurl.Parse(proxyURL)
	if err != nil {
		return nil, err
	}
	if parsed != nil {
		transport := &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: proxyDialTimeout,
			}).DialContext,
			TLSHandshakeTimeout: proxyTLSHandshakeTimeout,
		}
		if err := proxyutil.ConfigureTransportProxy(transport, parsed); err != nil {
			return nil, fmt.Errorf("configure proxy: %w", err)
		}
		httpClient.Transport = transport
	}
	return &Client{httpClient: httpClient}, nil
}

func (c *Client) Discover(ctx context.Context) (*DiscoveryDocument, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, DiscoveryURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("xai discovery request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("xai discovery failed (HTTP %d): %s", resp.StatusCode, string(body))
	}
	var doc DiscoveryDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parse xai discovery: %w", err)
	}
	if strings.TrimSpace(doc.Issuer) != Issuer {
		return nil, fmt.Errorf("xai discovery issuer mismatch")
	}
	if err := ValidateOAuthEndpoint(doc.AuthorizationEndpoint, "authorization_endpoint"); err != nil {
		return nil, err
	}
	if err := ValidateOAuthEndpoint(doc.TokenEndpoint, "token_endpoint"); err != nil {
		return nil, err
	}
	return &doc, nil
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	ExpiresIn    int64  `json:"expires_in,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

func (c *Client) ExchangeCodeForTokens(ctx context.Context, code, redirectURI, codeVerifier, tokenEndpoint string) (*TokenResponse, error) {
	values := url.Values{}
	values.Set("grant_type", "authorization_code")
	values.Set("code", strings.TrimSpace(code))
	values.Set("redirect_uri", strings.TrimSpace(redirectURI))
	values.Set("client_id", ClientID)
	values.Set("code_verifier", strings.TrimSpace(codeVerifier))
	return c.postTokenForm(ctx, tokenEndpoint, values)
}

func (c *Client) RefreshTokens(ctx context.Context, refreshToken, tokenEndpoint string) (*TokenResponse, error) {
	values := url.Values{}
	values.Set("grant_type", "refresh_token")
	values.Set("client_id", ClientID)
	values.Set("refresh_token", strings.TrimSpace(refreshToken))
	return c.postTokenForm(ctx, tokenEndpoint, values)
}

func (c *Client) postTokenForm(ctx context.Context, tokenEndpoint string, values url.Values) (*TokenResponse, error) {
	if err := ValidateOAuthEndpoint(tokenEndpoint, "token_endpoint"); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("xai token request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("xai token request failed (HTTP %d): %s", resp.StatusCode, string(body))
	}
	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parse xai token response: %w", err)
	}
	if strings.TrimSpace(tokenResp.AccessToken) == "" {
		return nil, errors.New("xai token response missing access_token")
	}
	return &tokenResp, nil
}

type Identity struct {
	Email string
	Sub   string
}

func ParseJWTIdentity(token string) Identity {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return Identity{}
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Identity{}
	}
	var claims struct {
		Email string `json:"email"`
		Sub   string `json:"sub"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return Identity{}
	}
	return Identity{Email: strings.TrimSpace(claims.Email), Sub: strings.TrimSpace(claims.Sub)}
}

func BuildResponsesURL(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = DefaultAPIBaseURL
	}
	if strings.HasSuffix(base, "/responses") {
		return base
	}
	return base + "/responses"
}

func BuildModelsURL(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = DefaultAPIBaseURL
	}
	if strings.HasSuffix(base, "/models") {
		return base
	}
	base = strings.TrimSuffix(base, "/responses")
	return base + "/models"
}

func BuildImagesGenerationsURL(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = DefaultAPIBaseURL
	}
	if strings.HasSuffix(base, "/images/generations") {
		return base
	}
	base = strings.TrimSuffix(base, "/responses")
	base = strings.TrimSuffix(base, "/models")
	return base + "/images/generations"
}

func BuildVideosGenerationsURL(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = DefaultAPIBaseURL
	}
	if strings.HasSuffix(base, "/videos/generations") {
		return base
	}
	base = trimAPIEndpointSuffix(base)
	return base + "/videos/generations"
}

func BuildVideoPollURL(baseURL, requestID string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = DefaultAPIBaseURL
	}
	base = trimAPIEndpointSuffix(base)
	return base + "/videos/" + url.PathEscape(strings.TrimSpace(requestID))
}

func trimAPIEndpointSuffix(base string) string {
	for _, suffix := range []string{
		"/responses",
		"/models",
		"/images/generations",
		"/images/edits",
		"/videos/generations",
		"/videos/edits",
	} {
		base = strings.TrimSuffix(base, suffix)
	}
	return base
}
