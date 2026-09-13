package devin

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	AuthorizeURL       = "https://app.devin.ai/auth/cli/continue"
	TokenURL           = "https://api.devin.ai/auth/cli/token"
	DefaultAPIHost     = "https://api.devin.ai"
	ChatBaseURL        = "https://server.codeium.com"
	RedirectURI        = "http://127.0.0.1:59653/callback"
	SessionTokenPrefix = "devin-session-token$"
	SessionTTL         = 30 * time.Minute
	FallbackTTL        = 365 * 24 * time.Hour
)

type OAuthSession struct {
	State        string    `json:"state"`
	CodeVerifier string    `json:"code_verifier"`
	Challenge    string    `json:"challenge"`
	ProxyURL     string    `json:"proxy_url,omitempty"`
	RedirectURI  string    `json:"redirect_uri"`
	CreatedAt    time.Time `json:"created_at"`
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
	s.stopOnce.Do(func() { close(s.stopCh) })
}

func (s *SessionStore) cleanup() {
	ticker := time.NewTicker(time.Minute)
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

func GenerateSessionID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func GenerateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	// uuid-shaped state, matching oh-my-pi state "uuid"
	hexID := hex.EncodeToString(b)
	return hexID[0:8] + "-" + hexID[8:12] + "-" + hexID[12:16] + "-" + hexID[16:20] + "-" + hexID[20:32], nil
}

func GeneratePKCE() (verifier, challenge string, err error) {
	raw := make([]byte, 96)
	if _, err = rand.Read(raw); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func BuildAuthorizationURL(state, challenge, redirectURI string) string {
	if strings.TrimSpace(redirectURI) == "" {
		redirectURI = RedirectURI
	}
	values := url.Values{}
	values.Set("response_type", "code")
	values.Set("redirect_uri", redirectURI)
	values.Set("code_challenge", challenge)
	values.Set("code_challenge_method", "S256")
	values.Set("state", state)
	values.Set("prompt", "select_account")
	return AuthorizeURL + "?" + values.Encode()
}

func NormalizeSessionToken(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	if strings.HasPrefix(token, SessionTokenPrefix) {
		return token
	}
	return SessionTokenPrefix + token
}

func TokenExpiry(token string) time.Time {
	if exp, ok := jwtExp(token); ok {
		return time.Unix(exp, 0)
	}
	return time.Now().Add(FallbackTTL)
}

func jwtExp(token string) (int64, bool) {
	parts := strings.Split(strings.TrimPrefix(token, SessionTokenPrefix), ".")
	if len(parts) != 3 {
		return 0, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return 0, false
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return 0, false
	}
	switch exp := payload["exp"].(type) {
	case float64:
		return int64(exp), exp > 0
	default:
		return 0, false
	}
}
