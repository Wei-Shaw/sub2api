// auth_session.go 提供 PKCE 登录流程的临时会话存储（state/verifier 配对）。
// 与 antigravity.SessionStore 同构：进程内存、TTL 过期、后台清理。
package devin

import (
	"sync"
	"time"
)

// AuthSessionTTL 是一次 PKCE 登录会话的有效期。
const AuthSessionTTL = 15 * time.Minute

// AuthSession 保存一次授权流程的临时状态。
type AuthSession struct {
	State     string
	Verifier  string
	ProxyURL  string
	CreatedAt time.Time
}

// AuthSessionStore 是 PKCE 会话的内存存储。
type AuthSessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*AuthSession
	stopCh   chan struct{}
}

// NewAuthSessionStore 创建会话存储并启动后台清理。
func NewAuthSessionStore() *AuthSessionStore {
	store := &AuthSessionStore{
		sessions: make(map[string]*AuthSession),
		stopCh:   make(chan struct{}),
	}
	go store.cleanup()
	return store
}

// Set 写入一个会话（key 为 session_id）。
func (s *AuthSessionStore) Set(sessionID string, session *AuthSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sessionID] = session
}

// Get 读取会话；过期返回 false。
func (s *AuthSessionStore) Get(sessionID string) (*AuthSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok := s.sessions[sessionID]
	if !ok || time.Since(session.CreatedAt) > AuthSessionTTL {
		return nil, false
	}
	return session, true
}

// Delete 删除会话。
func (s *AuthSessionStore) Delete(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
}

// Stop 停止后台清理协程。
func (s *AuthSessionStore) Stop() {
	select {
	case <-s.stopCh:
		return
	default:
		close(s.stopCh)
	}
}

func (s *AuthSessionStore) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case now := <-ticker.C:
			s.mu.Lock()
			for id, session := range s.sessions {
				if now.Sub(session.CreatedAt) > AuthSessionTTL {
					delete(s.sessions, id)
				}
			}
			s.mu.Unlock()
		}
	}
}
