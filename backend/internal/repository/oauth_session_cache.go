package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// oauthSessionKeyPrefix is the Redis key prefix for OAuth sessions.
	oauthSessionKeyPrefix = "oauth_session:"

	// defaultOAuthSessionTTL is the default TTL for OAuth sessions.
	defaultOAuthSessionTTL = 30 * time.Minute
)

// OAuthSessionCache provides Redis-based storage for OAuth sessions.
// This enables distributed deployment where any instance can handle
// the OAuth callback regardless of which instance initiated the flow.
type OAuthSessionCache struct {
	rdb *redis.Client
	ttl time.Duration
}

// NewOAuthSessionCache creates a new OAuth session cache.
func NewOAuthSessionCache(rdb *redis.Client) *OAuthSessionCache {
	return &OAuthSessionCache{
		rdb: rdb,
		ttl: defaultOAuthSessionTTL,
	}
}

// NewOAuthSessionCacheWithTTL creates a new OAuth session cache with custom TTL.
func NewOAuthSessionCacheWithTTL(rdb *redis.Client, ttl time.Duration) *OAuthSessionCache {
	return &OAuthSessionCache{
		rdb: rdb,
		ttl: ttl,
	}
}

// OAuthSessionData is a generic OAuth session data structure.
// Different OAuth providers can use this with provider-specific fields.
type OAuthSessionData struct {
	// Common fields
	State        string    `json:"state"`
	CodeVerifier string    `json:"code_verifier,omitempty"`
	ClientID     string    `json:"client_id,omitempty"`
	ProxyURL     string    `json:"proxy_url,omitempty"`
	RedirectURI  string    `json:"redirect_uri,omitempty"`
	CreatedAt    time.Time `json:"created_at"`

	// Provider identifier (openai, claude, gemini, antigravity)
	Provider string `json:"provider,omitempty"`

	// Additional provider-specific data stored as JSON
	Extra map[string]interface{} `json:"extra,omitempty"`
}

// buildKey constructs the Redis key for an OAuth session.
func (c *OAuthSessionCache) buildKey(provider, sessionID string) string {
	return fmt.Sprintf("%s%s:%s", oauthSessionKeyPrefix, provider, sessionID)
}

// Set stores an OAuth session in Redis.
func (c *OAuthSessionCache) Set(ctx context.Context, provider, sessionID string, session *OAuthSessionData) error {
	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now()
	}
	session.Provider = provider

	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("failed to marshal OAuth session: %w", err)
	}

	key := c.buildKey(provider, sessionID)
	if err := c.rdb.Set(ctx, key, data, c.ttl).Err(); err != nil {
		return fmt.Errorf("failed to store OAuth session: %w", err)
	}

	return nil
}

// Get retrieves an OAuth session from Redis.
// Returns nil, nil if the session doesn't exist or has expired.
func (c *OAuthSessionCache) Get(ctx context.Context, provider, sessionID string) (*OAuthSessionData, error) {
	key := c.buildKey(provider, sessionID)
	data, err := c.rdb.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get OAuth session: %w", err)
	}

	var session OAuthSessionData
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal OAuth session: %w", err)
	}

	return &session, nil
}

// Delete removes an OAuth session from Redis.
func (c *OAuthSessionCache) Delete(ctx context.Context, provider, sessionID string) error {
	key := c.buildKey(provider, sessionID)
	if err := c.rdb.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("failed to delete OAuth session: %w", err)
	}
	return nil
}

// GetAndDelete retrieves and removes an OAuth session atomically.
// This is useful for one-time use sessions like OAuth callbacks.
func (c *OAuthSessionCache) GetAndDelete(ctx context.Context, provider, sessionID string) (*OAuthSessionData, error) {
	key := c.buildKey(provider, sessionID)

	// Use GETDEL for atomic get and delete
	data, err := c.rdb.GetDel(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get and delete OAuth session: %w", err)
	}

	var session OAuthSessionData
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal OAuth session: %w", err)
	}

	return &session, nil
}

// Exists checks if an OAuth session exists.
func (c *OAuthSessionCache) Exists(ctx context.Context, provider, sessionID string) (bool, error) {
	key := c.buildKey(provider, sessionID)
	count, err := c.rdb.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check OAuth session existence: %w", err)
	}
	return count > 0, nil
}

// SetWithTTL stores an OAuth session with a custom TTL.
func (c *OAuthSessionCache) SetWithTTL(ctx context.Context, provider, sessionID string, session *OAuthSessionData, ttl time.Duration) error {
	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now()
	}
	session.Provider = provider

	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("failed to marshal OAuth session: %w", err)
	}

	key := c.buildKey(provider, sessionID)
	if err := c.rdb.Set(ctx, key, data, ttl).Err(); err != nil {
		return fmt.Errorf("failed to store OAuth session: %w", err)
	}

	return nil
}

// OpenAIOAuthSessionStore adapts OAuthSessionCache to the openai.SessionStore interface.
// This allows gradual migration from in-memory to Redis-based storage.
type OpenAIOAuthSessionStore struct {
	cache *OAuthSessionCache
}

// NewOpenAIOAuthSessionStore creates a new OpenAI OAuth session store backed by Redis.
func NewOpenAIOAuthSessionStore(cache *OAuthSessionCache) *OpenAIOAuthSessionStore {
	return &OpenAIOAuthSessionStore{cache: cache}
}

// Set stores an OpenAI OAuth session.
func (s *OpenAIOAuthSessionStore) Set(sessionID string, state, codeVerifier, clientID, proxyURL, redirectURI string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session := &OAuthSessionData{
		State:        state,
		CodeVerifier: codeVerifier,
		ClientID:     clientID,
		ProxyURL:     proxyURL,
		RedirectURI:  redirectURI,
		CreatedAt:    time.Now(),
	}

	return s.cache.Set(ctx, "openai", sessionID, session)
}

// Get retrieves an OpenAI OAuth session.
func (s *OpenAIOAuthSessionStore) Get(sessionID string) (state, codeVerifier, clientID, proxyURL, redirectURI string, found bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session, err := s.cache.Get(ctx, "openai", sessionID)
	if err != nil || session == nil {
		return "", "", "", "", "", false
	}

	return session.State, session.CodeVerifier, session.ClientID, session.ProxyURL, session.RedirectURI, true
}

// Delete removes an OpenAI OAuth session.
func (s *OpenAIOAuthSessionStore) Delete(sessionID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.cache.Delete(ctx, "openai", sessionID)
}

// ClaudeOAuthSessionStore adapts OAuthSessionCache for Claude OAuth.
type ClaudeOAuthSessionStore struct {
	cache *OAuthSessionCache
}

// NewClaudeOAuthSessionStore creates a new Claude OAuth session store backed by Redis.
func NewClaudeOAuthSessionStore(cache *OAuthSessionCache) *ClaudeOAuthSessionStore {
	return &ClaudeOAuthSessionStore{cache: cache}
}

// Set stores a Claude OAuth session.
func (s *ClaudeOAuthSessionStore) Set(sessionID string, state, codeVerifier, proxyURL, redirectURI string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session := &OAuthSessionData{
		State:        state,
		CodeVerifier: codeVerifier,
		ProxyURL:     proxyURL,
		RedirectURI:  redirectURI,
		CreatedAt:    time.Now(),
	}

	return s.cache.Set(ctx, "claude", sessionID, session)
}

// Get retrieves a Claude OAuth session.
func (s *ClaudeOAuthSessionStore) Get(sessionID string) (state, codeVerifier, proxyURL, redirectURI string, found bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session, err := s.cache.Get(ctx, "claude", sessionID)
	if err != nil || session == nil {
		return "", "", "", "", false
	}

	return session.State, session.CodeVerifier, session.ProxyURL, session.RedirectURI, true
}

// Delete removes a Claude OAuth session.
func (s *ClaudeOAuthSessionStore) Delete(sessionID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.cache.Delete(ctx, "claude", sessionID)
}

// GeminiOAuthSessionStore adapts OAuthSessionCache for Gemini OAuth.
type GeminiOAuthSessionStore struct {
	cache *OAuthSessionCache
}

// NewGeminiOAuthSessionStore creates a new Gemini OAuth session store backed by Redis.
func NewGeminiOAuthSessionStore(cache *OAuthSessionCache) *GeminiOAuthSessionStore {
	return &GeminiOAuthSessionStore{cache: cache}
}

// Set stores a Gemini OAuth session.
func (s *GeminiOAuthSessionStore) Set(sessionID string, state, codeVerifier, proxyURL, redirectURI string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session := &OAuthSessionData{
		State:        state,
		CodeVerifier: codeVerifier,
		ProxyURL:     proxyURL,
		RedirectURI:  redirectURI,
		CreatedAt:    time.Now(),
	}

	return s.cache.Set(ctx, "gemini", sessionID, session)
}

// Get retrieves a Gemini OAuth session.
func (s *GeminiOAuthSessionStore) Get(sessionID string) (state, codeVerifier, proxyURL, redirectURI string, found bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session, err := s.cache.Get(ctx, "gemini", sessionID)
	if err != nil || session == nil {
		return "", "", "", "", false
	}

	return session.State, session.CodeVerifier, session.ProxyURL, session.RedirectURI, true
}

// Delete removes a Gemini OAuth session.
func (s *GeminiOAuthSessionStore) Delete(sessionID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.cache.Delete(ctx, "gemini", sessionID)
}

// AntigravityOAuthSessionStore adapts OAuthSessionCache for Antigravity OAuth.
type AntigravityOAuthSessionStore struct {
	cache *OAuthSessionCache
}

// NewAntigravityOAuthSessionStore creates a new Antigravity OAuth session store backed by Redis.
func NewAntigravityOAuthSessionStore(cache *OAuthSessionCache) *AntigravityOAuthSessionStore {
	return &AntigravityOAuthSessionStore{cache: cache}
}

// Set stores an Antigravity OAuth session.
func (s *AntigravityOAuthSessionStore) Set(sessionID string, state, codeVerifier, proxyURL, redirectURI string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session := &OAuthSessionData{
		State:        state,
		CodeVerifier: codeVerifier,
		ProxyURL:     proxyURL,
		RedirectURI:  redirectURI,
		CreatedAt:    time.Now(),
	}

	return s.cache.Set(ctx, "antigravity", sessionID, session)
}

// Get retrieves an Antigravity OAuth session.
func (s *AntigravityOAuthSessionStore) Get(sessionID string) (state, codeVerifier, proxyURL, redirectURI string, found bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session, err := s.cache.Get(ctx, "antigravity", sessionID)
	if err != nil || session == nil {
		return "", "", "", "", false
	}

	return session.State, session.CodeVerifier, session.ProxyURL, session.RedirectURI, true
}

// Delete removes an Antigravity OAuth session.
func (s *AntigravityOAuthSessionStore) Delete(sessionID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.cache.Delete(ctx, "antigravity", sessionID)
}
