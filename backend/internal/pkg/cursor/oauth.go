package cursor

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"time"
)

const (
	LoginURL             = "https://cursor.com/loginDeepControl"
	PollURL              = "https://api2.cursor.sh/auth/poll"
	RefreshURL           = "https://api2.cursor.sh/auth/exchange_user_api_key"
	UsageURL             = "https://api2.cursor.sh/auth/usage"
	AuthMeURL            = "https://cursor.com/api/auth/me"
	APIBaseURL           = "https://api2.cursor.sh"
	SessionTTL           = 30 * time.Minute
	ExpirySkew           = 5 * time.Minute
	DefaultClientVersion = "cli-2026.07.23-e383d2b"
)

// OAuthSession stores one Cursor Deep Control PKCE login.
type OAuthSession struct {
	UUID      string    `json:"uuid"`
	Verifier  string    `json:"verifier"`
	Challenge string    `json:"challenge"`
	ProxyURL  string    `json:"proxy_url,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	mu        sync.Mutex
	consumed  bool
}

func (s *OAuthSession) TryConsume() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.consumed {
		return false
	}
	s.consumed = true
	return true
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

func BuildLoginURL(challenge, uuid string) string {
	return LoginURL + "?challenge=" + challenge + "&uuid=" + uuid + "&mode=login&redirectTarget=cli"
}

type TokenResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
}

func TokenExpiry(accessToken string) time.Time {
	if exp, ok := jwtExp(accessToken); ok {
		return time.Unix(exp, 0).Add(-ExpirySkew)
	}
	return time.Now().Add(time.Hour)
}

func ExtractUserID(accessToken string) string {
	payload, ok := jwtPayload(accessToken)
	if !ok {
		return ""
	}
	sub, _ := payload["sub"].(string)
	sub = strings.TrimSpace(sub)
	if sub == "" {
		return ""
	}
	if parts := strings.Split(sub, "|"); len(parts) > 1 {
		if id := strings.TrimSpace(parts[1]); id != "" {
			return id
		}
	}
	return sub
}

func jwtExp(token string) (int64, bool) {
	payload, ok := jwtPayload(token)
	if !ok {
		return 0, false
	}
	switch exp := payload["exp"].(type) {
	case float64:
		return int64(exp), exp > 0
	case json.Number:
		n, err := exp.Int64()
		return n, err == nil && n > 0
	default:
		return 0, false
	}
}

func jwtPayload(token string) (map[string]any, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, false
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, false
	}
	return payload, true
}
