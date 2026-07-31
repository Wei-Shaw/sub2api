package service

import (
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// OAuth session owner store errors (user-owned account OAuth).
var (
	ErrOAuthSessionNotFound = infraerrors.NotFound(
		"OAUTH_SESSION_NOT_FOUND",
		"oauth session not found or expired",
	)
	ErrOAuthSessionForbidden = infraerrors.Forbidden(
		"OAUTH_SESSION_FORBIDDEN",
		"oauth session does not belong to current user",
	)
	ErrProxyNotAllowed = infraerrors.BadRequest(
		"PROXY_NOT_ALLOWED",
		"proxy is not allowed for user-owned account OAuth",
	)
)

// DefaultOAuthSessionOwnerTTL matches typical OAuth session TTL (30m).
const DefaultOAuthSessionOwnerTTL = 30 * time.Minute

type oauthSessionOwnerEntry struct {
	userID    int64
	expiresAt time.Time
}

// OAuthSessionOwnerStore binds OAuth session IDs to the user who created them.
// Used on user OAuth paths to prevent session cross-use.
type OAuthSessionOwnerStore struct {
	mu      sync.RWMutex
	entries map[string]oauthSessionOwnerEntry
	ttl     time.Duration
	stopCh  chan struct{}
	once    sync.Once
}

// NewOAuthSessionOwnerStore creates an in-memory session owner store with cleanup.
func NewOAuthSessionOwnerStore() *OAuthSessionOwnerStore {
	s := &OAuthSessionOwnerStore{
		entries: make(map[string]oauthSessionOwnerEntry),
		ttl:     DefaultOAuthSessionOwnerTTL,
		stopCh:  make(chan struct{}),
	}
	go s.cleanupLoop()
	return s
}

// Bind associates sessionID with userID until TTL elapses (or Unbind).
func (s *OAuthSessionOwnerStore) Bind(sessionID string, userID int64) {
	if s == nil || sessionID == "" || userID <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[sessionID] = oauthSessionOwnerEntry{
		userID:    userID,
		expiresAt: time.Now().Add(s.ttl),
	}
}

// Assert checks that sessionID belongs to userID and is not expired.
func (s *OAuthSessionOwnerStore) Assert(sessionID string, userID int64) error {
	if s == nil {
		return ErrOAuthSessionNotFound
	}
	if sessionID == "" {
		return ErrOAuthSessionNotFound
	}
	s.mu.RLock()
	entry, ok := s.entries[sessionID]
	s.mu.RUnlock()
	if !ok || time.Now().After(entry.expiresAt) {
		// Lazy delete expired
		if ok {
			s.Unbind(sessionID)
		}
		return ErrOAuthSessionNotFound
	}
	if entry.userID != userID {
		return ErrOAuthSessionForbidden
	}
	return nil
}

// Unbind removes a session binding (best-effort after exchange).
func (s *OAuthSessionOwnerStore) Unbind(sessionID string) {
	if s == nil || sessionID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, sessionID)
}

// Stop stops the background cleanup goroutine.
func (s *OAuthSessionOwnerStore) Stop() {
	if s == nil {
		return
	}
	s.once.Do(func() {
		close(s.stopCh)
	})
}

func (s *OAuthSessionOwnerStore) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.purgeExpired()
		}
	}
}

func (s *OAuthSessionOwnerStore) purgeExpired() {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, entry := range s.entries {
		if now.After(entry.expiresAt) {
			delete(s.entries, id)
		}
	}
}
