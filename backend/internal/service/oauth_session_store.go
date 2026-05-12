package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
	"github.com/Wei-Shaw/sub2api/internal/pkg/oauth"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/redis/go-redis/v9"
)

const (
	oauthSessionStoreRedisTimeout = 3 * time.Second

	claudeOAuthSessionKeyPrefix      = "oauth:claude:session:"
	openAIOAuthSessionKeyPrefix      = "oauth:openai:session:"
	geminiOAuthSessionKeyPrefix      = "oauth:gemini:session:"
	antigravityOAuthSessionKeyPrefix = "oauth:antigravity:session:"
)

type ClaudeOAuthSessionStore interface {
	Set(sessionID string, session *oauth.OAuthSession)
	Get(sessionID string) (*oauth.OAuthSession, bool)
	Delete(sessionID string)
	Stop()
}

type OpenAIOAuthSessionStore interface {
	Set(sessionID string, session *openai.OAuthSession)
	Get(sessionID string) (*openai.OAuthSession, bool)
	Delete(sessionID string)
	Stop()
}

type GeminiOAuthSessionStore interface {
	Set(sessionID string, session *geminicli.OAuthSession)
	Get(sessionID string) (*geminicli.OAuthSession, bool)
	Delete(sessionID string)
	Stop()
}

type AntigravityOAuthSessionStore interface {
	Set(sessionID string, session *antigravity.OAuthSession)
	Get(sessionID string) (*antigravity.OAuthSession, bool)
	Delete(sessionID string)
	Stop()
}

type redisStringStore interface {
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
	Get(ctx context.Context, key string) (string, error)
	Delete(ctx context.Context, key string) error
}

type redisStringStoreAdapter struct {
	client *redis.Client
}

func newRedisStringStoreAdapter(client *redis.Client) redisStringStore {
	if client == nil {
		return nil
	}
	return &redisStringStoreAdapter{client: client}
}

func (s *redisStringStoreAdapter) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	return s.client.Set(ctx, key, value, ttl).Err()
}

func (s *redisStringStoreAdapter) Get(ctx context.Context, key string) (string, error) {
	return s.client.Get(ctx, key).Result()
}

func (s *redisStringStoreAdapter) Delete(ctx context.Context, key string) error {
	return s.client.Del(ctx, key).Err()
}

type redisClaudeOAuthSessionStore struct {
	store redisStringStore
}

type redisOpenAIOAuthSessionStore struct {
	store redisStringStore
}

type redisGeminiOAuthSessionStore struct {
	store redisStringStore
}

type redisAntigravityOAuthSessionStore struct {
	store redisStringStore
}

func NewClaudeOAuthSessionStore(client *redis.Client) ClaudeOAuthSessionStore {
	if client == nil {
		return oauth.NewSessionStore()
	}
	return &redisClaudeOAuthSessionStore{store: newRedisStringStoreAdapter(client)}
}

func NewOpenAIOAuthSessionStore(client *redis.Client) OpenAIOAuthSessionStore {
	if client == nil {
		return openai.NewSessionStore()
	}
	return &redisOpenAIOAuthSessionStore{store: newRedisStringStoreAdapter(client)}
}

func NewGeminiOAuthSessionStore(client *redis.Client) GeminiOAuthSessionStore {
	if client == nil {
		return geminicli.NewSessionStore()
	}
	return &redisGeminiOAuthSessionStore{store: newRedisStringStoreAdapter(client)}
}

func NewAntigravityOAuthSessionStore(client *redis.Client) AntigravityOAuthSessionStore {
	if client == nil {
		return antigravity.NewSessionStore()
	}
	return &redisAntigravityOAuthSessionStore{store: newRedisStringStoreAdapter(client)}
}

func (s *redisClaudeOAuthSessionStore) Set(sessionID string, session *oauth.OAuthSession) {
	storeSessionJSON(s.store, claudeOAuthSessionKeyPrefix+strings.TrimSpace(sessionID), session, sessionStoreTTL(session.CreatedAt, oauth.SessionTTL))
}

func (s *redisClaudeOAuthSessionStore) Get(sessionID string) (*oauth.OAuthSession, bool) {
	session, ok := loadSessionJSON(s.store, claudeOAuthSessionKeyPrefix+strings.TrimSpace(sessionID), oauthSessionExpired)
	if !ok {
		return nil, false
	}
	return session, true
}

func (s *redisClaudeOAuthSessionStore) Delete(sessionID string) {
	deleteSessionKey(s.store, claudeOAuthSessionKeyPrefix+strings.TrimSpace(sessionID))
}

func (s *redisClaudeOAuthSessionStore) Stop() {}

func (s *redisOpenAIOAuthSessionStore) Set(sessionID string, session *openai.OAuthSession) {
	storeSessionJSON(s.store, openAIOAuthSessionKeyPrefix+strings.TrimSpace(sessionID), session, sessionStoreTTL(session.CreatedAt, openai.SessionTTL))
}

func (s *redisOpenAIOAuthSessionStore) Get(sessionID string) (*openai.OAuthSession, bool) {
	session, ok := loadSessionJSON(s.store, openAIOAuthSessionKeyPrefix+strings.TrimSpace(sessionID), openaiOAuthSessionExpired)
	if !ok {
		return nil, false
	}
	return session, true
}

func (s *redisOpenAIOAuthSessionStore) Delete(sessionID string) {
	deleteSessionKey(s.store, openAIOAuthSessionKeyPrefix+strings.TrimSpace(sessionID))
}

func (s *redisOpenAIOAuthSessionStore) Stop() {}

func (s *redisGeminiOAuthSessionStore) Set(sessionID string, session *geminicli.OAuthSession) {
	storeSessionJSON(s.store, geminiOAuthSessionKeyPrefix+strings.TrimSpace(sessionID), session, sessionStoreTTL(session.CreatedAt, geminicli.SessionTTL))
}

func (s *redisGeminiOAuthSessionStore) Get(sessionID string) (*geminicli.OAuthSession, bool) {
	session, ok := loadSessionJSON(s.store, geminiOAuthSessionKeyPrefix+strings.TrimSpace(sessionID), geminiOAuthSessionExpired)
	if !ok {
		return nil, false
	}
	return session, true
}

func (s *redisGeminiOAuthSessionStore) Delete(sessionID string) {
	deleteSessionKey(s.store, geminiOAuthSessionKeyPrefix+strings.TrimSpace(sessionID))
}

func (s *redisGeminiOAuthSessionStore) Stop() {}

func (s *redisAntigravityOAuthSessionStore) Set(sessionID string, session *antigravity.OAuthSession) {
	storeSessionJSON(s.store, antigravityOAuthSessionKeyPrefix+strings.TrimSpace(sessionID), session, sessionStoreTTL(session.CreatedAt, antigravity.SessionTTL))
}

func (s *redisAntigravityOAuthSessionStore) Get(sessionID string) (*antigravity.OAuthSession, bool) {
	session, ok := loadSessionJSON(s.store, antigravityOAuthSessionKeyPrefix+strings.TrimSpace(sessionID), antigravityOAuthSessionExpired)
	if !ok {
		return nil, false
	}
	return session, true
}

func (s *redisAntigravityOAuthSessionStore) Delete(sessionID string) {
	deleteSessionKey(s.store, antigravityOAuthSessionKeyPrefix+strings.TrimSpace(sessionID))
}

func (s *redisAntigravityOAuthSessionStore) Stop() {}

func storeSessionJSON[T any](store redisStringStore, key string, session *T, ttl time.Duration) {
	if store == nil || strings.TrimSpace(key) == "" || session == nil {
		return
	}
	data, err := json.Marshal(session)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), oauthSessionStoreRedisTimeout)
	defer cancel()
	_ = store.Set(ctx, key, string(data), ttl)
}

func loadSessionJSON[T any](store redisStringStore, key string, isExpired func(*T) bool) (*T, bool) {
	if store == nil || strings.TrimSpace(key) == "" {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), oauthSessionStoreRedisTimeout)
	defer cancel()
	raw, err := store.Get(ctx, key)
	if err != nil {
		return nil, false
	}
	var session T
	if err := json.Unmarshal([]byte(raw), &session); err != nil {
		deleteSessionKey(store, key)
		return nil, false
	}
	if isExpired(&session) {
		deleteSessionKey(store, key)
		return nil, false
	}
	return &session, true
}

func deleteSessionKey(store redisStringStore, key string) {
	if store == nil || strings.TrimSpace(key) == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), oauthSessionStoreRedisTimeout)
	defer cancel()
	_ = store.Delete(ctx, key)
}

func sessionStoreTTL(createdAt time.Time, maxTTL time.Duration) time.Duration {
	if maxTTL <= 0 {
		maxTTL = 30 * time.Minute
	}
	if createdAt.IsZero() {
		return maxTTL
	}
	ttl := time.Until(createdAt.Add(maxTTL))
	if ttl <= 0 {
		return time.Second
	}
	return ttl
}

func oauthSessionExpired(session *oauth.OAuthSession) bool {
	return session == nil || time.Since(session.CreatedAt) > oauth.SessionTTL
}

func openaiOAuthSessionExpired(session *openai.OAuthSession) bool {
	return session == nil || time.Since(session.CreatedAt) > openai.SessionTTL
}

func geminiOAuthSessionExpired(session *geminicli.OAuthSession) bool {
	return session == nil || time.Since(session.CreatedAt) > geminicli.SessionTTL
}

func antigravityOAuthSessionExpired(session *antigravity.OAuthSession) bool {
	return session == nil || time.Since(session.CreatedAt) > antigravity.SessionTTL
}
