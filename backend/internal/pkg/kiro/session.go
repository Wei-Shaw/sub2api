package kiro

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/redissession"
	"github.com/redis/go-redis/v9"
)

// SessionTTL 是一次授权流程的存活时间。
// AWS 的设备码通常 10 分钟过期，授权码流程也在同一量级。
const SessionTTL = 10 * time.Minute

// sessionCleanupInterval 是内存回退存储的清理周期。
const sessionCleanupInterval = time.Minute

// OAuthSession 保存一次进行中的授权流程状态。
//
// 注意：它带 ClientSecret，属于敏感数据。Redis 键有 TTL，消费后立即删除。
type OAuthSession struct {
	Method       AuthMethod `json:"method"`
	ClientID     string     `json:"client_id"`
	ClientSecret string     `json:"client_secret"`
	// Verifier 是 PKCE 的 code_verifier（仅 idc 路径）。
	Verifier string `json:"verifier"`
	// State 用于校验回调，防 CSRF（仅 idc 路径）。
	State       string `json:"state"`
	Region      string `json:"region"`
	IssuerURL   string `json:"issuer_url"`
	RedirectURI string `json:"redirect_uri"`
	// DeviceCode 仅 builder_id 路径使用。
	DeviceCode string    `json:"device_code"`
	Interval   int       `json:"interval"`
	ExpiresAt  time.Time `json:"expires_at"`
}

func (s *OAuthSession) expired() bool {
	return !s.ExpiresAt.IsZero() && time.Now().After(s.ExpiresAt)
}

// SessionStore 管理授权会话，优先用 Redis，失败时回退进程内存。
//
// 必须支持 Redis：IdC 与 social 走自建回调页，多副本部署时浏览器回调
// 可能落到另一个副本，进程内存里的会话会直接丢失。
//
// localOnly 记录每个会话 ID 是否落在了内存兜底路径（Redis 写入失败，
// 或压根没配置 Redis）。Get/TryConsume 据此决定该 ID 是否要信任 Redis：
// 一旦某个会话被标记为 localOnly，就永远只走内存，绝不再查 Redis —— 否则
// Redis 在写入失败后又恢复可达时，会如实返回"不存在"（因为这个会话从未
// 真正写进去过），把内存里明明有效的会话误判为丢失。非 localOnly 的会话
// 则继续把 Redis 当唯一权威源，不做内存兜底，避免陈旧本地副本导致
// TryConsume 重复消费。做法与 pkg/xai 的 SessionStore 一致。
type SessionStore struct {
	mu        sync.RWMutex
	memory    map[string]*OAuthSession
	localOnly map[string]struct{}
	remote    *redissession.Store

	stopOnce sync.Once
	stopCh   chan struct{}
}

// NewSessionStore 创建仅内存的存储（单副本部署可用）。
func NewSessionStore() *SessionStore {
	s := &SessionStore{
		memory:    make(map[string]*OAuthSession),
		localOnly: make(map[string]struct{}),
		stopCh:    make(chan struct{}),
	}
	go s.cleanupLoop()
	return s
}

// NewRedisSessionStore 创建带 Redis 后端的存储。
// 由 internal/service/wire.go 调用 —— service 包本身被 depguard 禁止 import redis。
func NewRedisSessionStore(rdb *redis.Client) *SessionStore {
	s := NewSessionStore()
	if rdb != nil {
		s.remote = redissession.New(rdb, "oauth:session:kiro:", SessionTTL)
	}
	return s
}

// Set 写入会话。Redis 写失败（或压根没配置 Redis）时降级为内存，并把这个
// 会话 ID 标记为 localOnly，保证单机/Redis 故障期间仍可完成授权。
// 内存副本无条件写入（不只是失败时才写），这样 Redis 恢复后 Get/TryConsume
// 仍能通过 localOnly 标记找到它。
func (s *SessionStore) Set(ctx context.Context, id string, sess *OAuthSession) {
	if sess == nil {
		return
	}
	if sess.ExpiresAt.IsZero() {
		sess.ExpiresAt = time.Now().Add(SessionTTL)
	}

	var remoteErr error
	if s.remote != nil {
		if err := s.remote.Set(ctx, id, sess); err != nil {
			remoteErr = err
			slog.Warn("kiro oauth session redis write failed; falling back to memory", "error", err)
		}
	}

	s.mu.Lock()
	s.memory[id] = sess
	if s.remote == nil || remoteErr != nil {
		s.localOnly[id] = struct{}{}
	} else {
		delete(s.localOnly, id)
	}
	s.mu.Unlock()
}

// Get 读取会话，过期的视为不存在。
func (s *SessionStore) Get(ctx context.Context, id string) (*OAuthSession, bool) {
	if s.isLocalOnly(id) {
		return s.getMemory(id)
	}

	if s.remote != nil {
		var sess OAuthSession
		if found, err := s.remote.Get(ctx, id, &sess); err == nil && found {
			if sess.expired() {
				return nil, false
			}
			return &sess, true
		}
	}

	return s.getMemory(id)
}

func (s *SessionStore) getMemory(id string) (*OAuthSession, bool) {
	s.mu.RLock()
	sess, ok := s.memory[id]
	s.mu.RUnlock()
	if !ok || sess.expired() {
		return nil, false
	}
	return sess, true
}

// Delete 删除会话。
func (s *SessionStore) Delete(ctx context.Context, id string) {
	if s.remote != nil {
		_ = s.remote.Delete(ctx, id)
	}
	s.mu.Lock()
	delete(s.memory, id)
	delete(s.localOnly, id)
	s.mu.Unlock()
}

// TryConsume 原子地把会话标记为已使用，返回是否是首次消费。
// 用于保证一个授权码只能兑换一次，防止回调 URL 被重放。
//
// localOnly 会话直接走内存消费，绝不查 Redis —— 否则 Redis 在这个会话
// 写入失败后又恢复可达时，会如实返回"不存在"（因为这个会话从未真正写
// 进去过），把回调误判为无效/重放。
func (s *SessionStore) TryConsume(ctx context.Context, id string) bool {
	if s.isLocalOnly(id) {
		return s.tryConsumeMemory(id)
	}

	if s.remote != nil {
		if ok, err := s.remote.TryConsume(ctx, id); err == nil {
			return ok
		}
	}

	return s.tryConsumeMemory(id)
}

func (s *SessionStore) isLocalOnly(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.localOnly[id]
	return ok
}

func (s *SessionStore) tryConsumeMemory(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.memory[id]
	if !ok || sess.expired() {
		return false
	}
	delete(s.memory, id)
	return true
}

// Stop 结束后台清理。重复调用安全。
func (s *SessionStore) Stop() {
	s.stopOnce.Do(func() { close(s.stopCh) })
}

func (s *SessionStore) cleanupLoop() {
	ticker := time.NewTicker(sessionCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.mu.Lock()
			for id, sess := range s.memory {
				if sess.expired() {
					delete(s.memory, id)
					delete(s.localOnly, id)
				}
			}
			s.mu.Unlock()
		}
	}
}

// GenerateSessionID 生成一个 URL 安全的随机会话 ID。
func GenerateSessionID() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("kiro: generate session id: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
