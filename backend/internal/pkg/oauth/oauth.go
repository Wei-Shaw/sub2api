// Package oauth provides helpers for OAuth flows used by this service.
package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Claude OAuth Constants
const (
	// OAuth Client ID for Claude
	ClientID = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"

	// OAuth endpoints
	AuthorizeURL = "https://claude.ai/oauth/authorize"
	TokenURL     = "https://platform.claude.com/v1/oauth/token"
	RedirectURI  = "https://platform.claude.com/oauth/code/callback"

	// Scopes - Browser URL (includes org:create_api_key for user authorization)
	ScopeOAuth = "org:create_api_key user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload"
	// Scopes - Internal API call (org:create_api_key not supported in API)
	ScopeAPI = "user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload"
	// Scopes - Setup token (inference only)
	ScopeInference = "user:inference"

	// Session TTL
	SessionTTL = 30 * time.Minute
)

// OAuthSession stores OAuth flow state

type OAuthSession struct {
	State        string    `json:"state"`
	CodeVerifier string    `json:"code_verifier"`
	Scope        string    `json:"scope"`
	ProxyURL     string    `json:"proxy_url,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// SessionStore manages OAuth sessions in memory
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*OAuthSession
	stopOnce sync.Once
	stopCh   chan struct{}
}

// NewSessionStore creates a new session store
func NewSessionStore() *SessionStore {
	store := &SessionStore{
		sessions: make(map[string]*OAuthSession),
		stopCh:   make(chan struct{}),
	}
	go store.cleanup()
	return store
}

// Stop stops the cleanup goroutine
func (s *SessionStore) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
}

// Set stores a session
func (s *SessionStore) Set(sessionID string, session *OAuthSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sessionID] = session
}

// Get retrieves a session
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

// Delete removes a session
func (s *SessionStore) Delete(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
}

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
				}
			}
			s.mu.Unlock()
		}
	}
}

// GenerateRandomBytes generates cryptographically secure random bytes
func GenerateRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// GenerateState generates a random state string for OAuth (base64url encoded)
func GenerateState() (string, error) {
	bytes, err := GenerateRandomBytes(32)
	if err != nil {
		return "", err
	}
	return base64URLEncode(bytes), nil
}

// GenerateSessionID generates a unique session ID
func GenerateSessionID() (string, error) {
	bytes, err := GenerateRandomBytes(16)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// GenerateCodeVerifier generates a PKCE code verifier (RFC 7636).
// Uses 32 random bytes → base64url-no-pad, producing a 43-char verifier.
func GenerateCodeVerifier() (string, error) {
	bytes, err := GenerateRandomBytes(32)
	if err != nil {
		return "", err
	}
	return base64URLEncode(bytes), nil
}

// GenerateCodeChallenge generates a PKCE code challenge using S256 method
func GenerateCodeChallenge(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))
	return base64URLEncode(hash[:])
}

// base64URLEncode encodes bytes to base64url without padding
func base64URLEncode(data []byte) string {
	encoded := base64.URLEncoding.EncodeToString(data)
	return strings.TrimRight(encoded, "=")
}

// BuildAuthorizationURL builds the OAuth authorization URL with correct parameter order
func BuildAuthorizationURL(state, codeChallenge, scope string) string {
	encodedRedirectURI := url.QueryEscape(RedirectURI)
	encodedScope := strings.ReplaceAll(url.QueryEscape(scope), "%20", "+")

	return fmt.Sprintf("%s?code=true&client_id=%s&response_type=code&redirect_uri=%s&scope=%s&code_challenge=%s&code_challenge_method=S256&state=%s",
		AuthorizeURL,
		ClientID,
		encodedRedirectURI,
		encodedScope,
		codeChallenge,
		state,
	)
}

// TokenResponse represents the token response from OAuth provider
type TokenResponse struct {
	AccessToken  string       `json:"access_token"`
	TokenType    string       `json:"token_type"`
	ExpiresIn    int64        `json:"expires_in"`
	RefreshToken string       `json:"refresh_token,omitempty"`
	Scope        string       `json:"scope,omitempty"`
	Organization *OrgInfo     `json:"organization,omitempty"`
	Account      *AccountInfo `json:"account,omitempty"`
}

// OrgInfo represents organization info from OAuth response
type OrgInfo struct {
	UUID string `json:"uuid"`
}

// AccountInfo represents account info from OAuth response
type AccountInfo struct {
	UUID         string `json:"uuid"`
	EmailAddress string `json:"email_address"`
}
