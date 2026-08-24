//go:build unit

package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIWSSessionPreemptRegistryCancelsSameScopedSessionOnly(t *testing.T) {
	var registry openAIWSSessionPreemptRegistry
	key := openAIWSSessionPreemptKey{groupID: 7, apiKeyID: 11, sessionHash: "sess"}
	other := openAIWSSessionPreemptKey{groupID: 7, apiKeyID: 12, sessionHash: "sess"}
	firstCtx, firstCancel := context.WithCancel(context.Background())
	firstCleanup, replaced := registry.Begin(key, firstCancel)
	require.False(t, replaced)
	otherCtx, otherCancel := context.WithCancel(context.Background())
	otherCleanup, replaced := registry.Begin(other, otherCancel)
	require.False(t, replaced)
	secondCtx, secondCancel := context.WithCancel(context.Background())
	secondCleanup, replaced := registry.Begin(key, secondCancel)
	require.True(t, replaced)

	require.ErrorIs(t, firstCtx.Err(), context.Canceled)
	require.NoError(t, otherCtx.Err())
	require.NoError(t, secondCtx.Err())
	firstCleanup()
	require.NoError(t, secondCtx.Err(), "stale cleanup must not remove the replacement")

	secondCleanup()
	otherCleanup()
}

type openAIWSSessionPreemptCacheStub struct {
	GatewayCache
	mu     sync.Mutex
	owners map[string][]byte
}

func (c *openAIWSSessionPreemptCacheStub) key(groupID int64, hash string) string {
	return fmt.Sprintf("%d:%s", groupID, hash)
}

func (c *openAIWSSessionPreemptCacheStub) ClaimOpenAIResponsesSessionWindow(_ context.Context, groupID int64, hash string, owner []byte, _ time.Duration) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.owners == nil {
		c.owners = make(map[string][]byte)
	}
	key := c.key(groupID, hash)
	previous := append([]byte(nil), c.owners[key]...)
	c.owners[key] = append([]byte(nil), owner...)
	return previous, nil
}

func (c *openAIWSSessionPreemptCacheStub) CompareAndRefreshOpenAIResponsesSessionWindow(_ context.Context, groupID int64, hash string, expected []byte, _ time.Duration) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return string(c.owners[c.key(groupID, hash)]) == string(expected), nil
}

func (c *openAIWSSessionPreemptCacheStub) CompareAndDeleteOpenAIResponsesSessionWindow(_ context.Context, groupID int64, hash string, expected []byte) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := c.key(groupID, hash)
	if string(c.owners[key]) != string(expected) {
		return false, nil
	}
	delete(c.owners, key)
	return true, nil
}

func TestOpenAIWSSessionPreemptContextEligibilityAndLocalCancellation(t *testing.T) {
	stateStore := NewOpenAIWSStateStore(nil)
	svc := &OpenAIGatewayService{openaiWSStateStore: stateStore}
	oauth := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	apiKey := &Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	grok := &Account{ID: 3, Platform: PlatformGrok, Type: AccountTypeOAuth}

	_, cleanup, armed, _ := svc.beginOpenAIWSSessionPreemptContext(context.Background(), apiKey, 7, 11, "sess", false)
	cleanup()
	require.False(t, armed)
	_, cleanup, armed, _ = svc.beginOpenAIWSSessionPreemptContext(context.Background(), grok, 7, 11, "sess", false)
	cleanup()
	require.False(t, armed)
	_, cleanup, armed, _ = svc.beginOpenAIWSSessionPreemptContext(context.Background(), oauth, 7, 11, "sess", true)
	cleanup()
	require.False(t, armed, "HTTP-ingress one-shot must not participate")

	firstCtx, firstCleanup, armed, replaced := svc.beginOpenAIWSSessionPreemptContext(context.Background(), oauth, 7, 11, "sess", false)
	require.True(t, armed)
	require.False(t, replaced)
	stateStore.BindSessionTurnState(7, "sess", "turn-state", time.Hour)
	stateStore.BindSessionConn(7, "sess", "conn-1", time.Hour)
	_, secondCleanup, armed, replaced := svc.beginOpenAIWSSessionPreemptContext(context.Background(), oauth, 7, 11, "sess", false)
	require.True(t, armed)
	require.True(t, replaced)
	require.True(t, isOpenAIWSSessionPreempted(firstCtx))
	require.True(t, IsOpenAIWSSessionPreemptedError(context.Cause(firstCtx)))
	_, turnStateExists := stateStore.GetSessionTurnState(7, "sess")
	_, sessionConnExists := stateStore.GetSessionConn(7, "sess")
	require.False(t, turnStateExists)
	require.False(t, sessionConnExists)
	firstCleanup()
	secondCleanup()
}

func TestOpenAIWSIngressSessionPreemptionSurvivesNestedForwardCleanup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(7)
	newContext := func() *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
		c.Set("api_key", &APIKey{ID: 11, GroupID: &groupID})
		return c
	}

	svc := &OpenAIGatewayService{}
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	firstMessage := []byte(`{"type":"response.create","prompt_cache_key":"session-1","input":"hello"}`)

	firstCtx, firstCleanup, armed := svc.BeginOpenAIWSIngressSessionPreemption(
		context.Background(), newContext(), account, firstMessage,
	)
	require.True(t, armed)
	defer firstCleanup()

	// ProxyResponsesWebSocketFromClient enters the same helper for each upstream
	// attempt. Its cleanup must not release the handler-owned registration.
	nestedCtx, nestedCleanup, armed := svc.BeginOpenAIWSIngressSessionPreemption(
		firstCtx, newContext(), account, firstMessage,
	)
	require.True(t, armed)
	require.Equal(t, firstCtx, nestedCtx)
	nestedCleanup()
	require.NoError(t, firstCtx.Err())

	_, secondCleanup, armed := svc.BeginOpenAIWSIngressSessionPreemption(
		context.Background(), newContext(), account, firstMessage,
	)
	require.True(t, armed)
	defer secondCleanup()
	require.True(t, IsOpenAIWSSessionPreemptedError(context.Cause(firstCtx)))
}

func TestOpenAIWSSessionPreemptRemoteClaimAndStaleReleaseAreAtomic(t *testing.T) {
	cache := &openAIWSSessionPreemptCacheStub{}
	svc := &OpenAIGatewayService{cache: cache}
	key := openAIWSSessionPreemptKey{groupID: 7, apiKeyID: 11, sessionHash: "sess"}

	previous, ok := svc.claimOpenAIWSSessionPreemptOwner(context.Background(), key, "owner-a")
	require.True(t, ok)
	require.Empty(t, previous)
	previous, ok = svc.claimOpenAIWSSessionPreemptOwner(context.Background(), key, "owner-b")
	require.True(t, ok)
	require.Equal(t, "owner-a", previous)
	svc.releaseOpenAIWSSessionPreemptOwner(context.Background(), key, "owner-a")

	cache.mu.Lock()
	current := string(cache.owners[cache.key(key.groupID, openAIWSSessionPreemptCacheHash(key.apiKeyID, key.sessionHash))])
	cache.mu.Unlock()
	require.Equal(t, "owner-b", current, "stale cleanup must preserve the replacement owner")
}

func TestNewOpenAIWSSessionPreemptKeyRequiresFullIsolationScope(t *testing.T) {
	_, ok := newOpenAIWSSessionPreemptKey(0, 11, "sess")
	require.False(t, ok)
	_, ok = newOpenAIWSSessionPreemptKey(7, 0, "sess")
	require.False(t, ok)
	_, ok = newOpenAIWSSessionPreemptKey(7, 11, " ")
	require.False(t, ok)
	key, ok := newOpenAIWSSessionPreemptKey(7, 11, " sess ")
	require.True(t, ok)
	require.Equal(t, "sess", key.sessionHash)
}
