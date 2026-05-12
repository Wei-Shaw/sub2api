package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
	"github.com/Wei-Shaw/sub2api/internal/pkg/oauth"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type sharedStringStore struct {
	data map[string]string
}

func (s *sharedStringStore) Set(_ context.Context, key string, value string, _ time.Duration) error {
	if s.data == nil {
		s.data = make(map[string]string)
	}
	s.data[key] = value
	return nil
}

func (s *sharedStringStore) Get(_ context.Context, key string) (string, error) {
	if value, ok := s.data[key]; ok {
		return value, nil
	}
	return "", redis.Nil
}

func (s *sharedStringStore) Delete(_ context.Context, key string) error {
	if s.data == nil {
		return nil
	}
	delete(s.data, key)
	return nil
}

func TestClaudeOAuthRedisSessionStore_SharedAcrossInstances(t *testing.T) {
	shared := &sharedStringStore{}
	storeA := &redisClaudeOAuthSessionStore{store: shared}
	storeB := &redisClaudeOAuthSessionStore{store: shared}

	storeA.Set("sid", &oauth.OAuthSession{
		State:        "state-a",
		CodeVerifier: "verifier-a",
		Scope:        oauth.ScopeOAuth,
		CreatedAt:    time.Now(),
	})

	session, ok := storeB.Get("sid")
	require.True(t, ok)
	require.Equal(t, "state-a", session.State)
	require.Equal(t, "verifier-a", session.CodeVerifier)
}

func TestOpenAIOAuthRedisSessionStore_SharedAcrossInstances(t *testing.T) {
	shared := &sharedStringStore{}
	storeA := &redisOpenAIOAuthSessionStore{store: shared}
	storeB := &redisOpenAIOAuthSessionStore{store: shared}

	storeA.Set("sid", &openai.OAuthSession{
		State:        "state-b",
		CodeVerifier: "verifier-b",
		RedirectURI:  openai.DefaultRedirectURI,
		ClientID:     openai.ClientID,
		CreatedAt:    time.Now(),
	})

	session, ok := storeB.Get("sid")
	require.True(t, ok)
	require.Equal(t, "state-b", session.State)
	require.Equal(t, openai.ClientID, session.ClientID)
}

func TestGeminiOAuthRedisSessionStore_SharedAcrossInstances(t *testing.T) {
	shared := &sharedStringStore{}
	storeA := &redisGeminiOAuthSessionStore{store: shared}
	storeB := &redisGeminiOAuthSessionStore{store: shared}

	storeA.Set("sid", &geminicli.OAuthSession{
		State:        "state-c",
		CodeVerifier: "verifier-c",
		RedirectURI:  geminicli.AIStudioOAuthRedirectURI,
		OAuthType:    "ai_studio",
		CreatedAt:    time.Now(),
	})

	session, ok := storeB.Get("sid")
	require.True(t, ok)
	require.Equal(t, "state-c", session.State)
	require.Equal(t, "ai_studio", session.OAuthType)
}

func TestAntigravityOAuthRedisSessionStore_SharedAcrossInstances(t *testing.T) {
	shared := &sharedStringStore{}
	storeA := &redisAntigravityOAuthSessionStore{store: shared}
	storeB := &redisAntigravityOAuthSessionStore{store: shared}

	storeA.Set("sid", &antigravity.OAuthSession{
		State:        "state-d",
		CodeVerifier: "verifier-d",
		CreatedAt:    time.Now(),
	})

	session, ok := storeB.Get("sid")
	require.True(t, ok)
	require.Equal(t, "state-d", session.State)
	require.Equal(t, "verifier-d", session.CodeVerifier)
}

func TestOpenAIOAuthRedisSessionStore_ExpiredSessionReturnsMiss(t *testing.T) {
	shared := &sharedStringStore{}
	store := &redisOpenAIOAuthSessionStore{store: shared}

	store.Set("sid", &openai.OAuthSession{
		State:        "expired-state",
		CodeVerifier: "expired-verifier",
		RedirectURI:  openai.DefaultRedirectURI,
		CreatedAt:    time.Now().Add(-openai.SessionTTL - time.Minute),
	})

	_, ok := store.Get("sid")
	require.False(t, ok)
}

func TestOpenAIOAuthService_ExchangeCode_CanUseSharedStoreAcrossInstances(t *testing.T) {
	shared := &sharedStringStore{}
	storeA := &redisOpenAIOAuthSessionStore{store: shared}
	storeB := &redisOpenAIOAuthSessionStore{store: shared}
	client := &openaiOAuthClientStateStub{}

	svcA := NewOpenAIOAuthServiceWithStore(nil, client, storeA)
	svcB := NewOpenAIOAuthServiceWithStore(nil, client, storeB)
	defer svcA.Stop()
	defer svcB.Stop()

	storeA.Set("sid", &openai.OAuthSession{
		State:        "shared-state",
		CodeVerifier: "shared-verifier",
		RedirectURI:  openai.DefaultRedirectURI,
		ClientID:     openai.ClientID,
		CreatedAt:    time.Now(),
	})

	info, err := svcB.ExchangeCode(context.Background(), &OpenAIExchangeCodeInput{
		SessionID: "sid",
		Code:      "auth-code",
		State:     "shared-state",
	})
	require.NoError(t, err)
	require.NotNil(t, info)
	require.Equal(t, "at", info.AccessToken)
}
