package kiro

import (
	"sync"
	"time"
)

// IdCSession holds the state required to complete a PKCE auth-code flow.
// Created by StartIdCLogin, consumed by CompleteIdCLogin.
type IdCSession struct {
	ClientID     string
	ClientSecret string
	CodeVerifier string
	State        string
	Region       string
	StartURL     string
	RedirectURI  string
	ProxyURL     string
	ExpiresAt    time.Time
}

// BuilderIDSession holds the state required to poll a device-code flow.
// Created by StartBuilderIDLogin, polled by PollBuilderIDLogin.
type BuilderIDSession struct {
	ID              string
	ClientID        string
	ClientSecret    string
	DeviceCode      string
	UserCode        string
	VerificationURI string
	Interval        int
	Region          string
	ProxyURL        string
	ExpiresAt       time.Time
}

// SessionStore is the in-memory store for active OAuth flows. Sessions
// auto-expire after their ExpiresAt timestamp. Concurrent access is safe.
//
// For multi-server deployments the admin must start and complete the flow
// on the same instance — same trade-off as antigravity.SessionStore.
type SessionStore struct {
	mu          sync.RWMutex
	idc         map[string]*IdCSession
	builderID   map[string]*BuilderIDSession
	stopCleanup chan struct{}
	once        sync.Once
}

// NewSessionStore returns a store with a background goroutine that purges
// expired entries every minute.
func NewSessionStore() *SessionStore {
	s := &SessionStore{
		idc:         make(map[string]*IdCSession),
		builderID:   make(map[string]*BuilderIDSession),
		stopCleanup: make(chan struct{}),
	}
	go s.cleanupLoop()
	return s
}

// SetIdC stores an IdC session under the given session id.
func (s *SessionStore) SetIdC(id string, sess *IdCSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.idc[id] = sess
}

// GetIdC fetches an IdC session. Returns nil, false if missing.
func (s *SessionStore) GetIdC(id string) (*IdCSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.idc[id]
	return sess, ok
}

// DeleteIdC removes a session (typically after a successful completion).
func (s *SessionStore) DeleteIdC(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.idc, id)
}

// SetBuilderID stores a Builder ID session under the given session id.
func (s *SessionStore) SetBuilderID(id string, sess *BuilderIDSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.builderID[id] = sess
}

// GetBuilderID fetches a Builder ID session. Returns nil, false if missing.
func (s *SessionStore) GetBuilderID(id string) (*BuilderIDSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.builderID[id]
	return sess, ok
}

// DeleteBuilderID removes a session.
func (s *SessionStore) DeleteBuilderID(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.builderID, id)
}

// UpdateBuilderIDInterval increases the polling interval after a slow_down
// response, while holding the write lock.
func (s *SessionStore) UpdateBuilderIDInterval(id string, deltaSeconds int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.builderID[id]; ok {
		sess.Interval += deltaSeconds
	}
}

// Stop terminates the cleanup goroutine. Safe to call multiple times.
func (s *SessionStore) Stop() {
	s.once.Do(func() { close(s.stopCleanup) })
}

func (s *SessionStore) cleanupLoop() {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-s.stopCleanup:
			return
		case <-t.C:
			s.purgeExpired(time.Now())
		}
	}
}

// purgeExpired removes any session past ExpiresAt. Separate from
// cleanupLoop for direct unit testing.
func (s *SessionStore) purgeExpired(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, sess := range s.idc {
		if now.After(sess.ExpiresAt) {
			delete(s.idc, id)
		}
	}
	for id, sess := range s.builderID {
		if now.After(sess.ExpiresAt) {
			delete(s.builderID, id)
		}
	}
}
