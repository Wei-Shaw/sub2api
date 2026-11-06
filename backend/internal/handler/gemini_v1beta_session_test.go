//go:build unit

package handler

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeGeminiGatewayCache struct {
	mu       sync.RWMutex
	sessions map[string]int64
}

func newFakeGeminiGatewayCache() *fakeGeminiGatewayCache {
	return &fakeGeminiGatewayCache{
		sessions: make(map[string]int64),
	}
}

func (c *fakeGeminiGatewayCache) key(groupID int64, sessionHash string) string {
	return fmt.Sprintf("%d:%s", groupID, sessionHash)
}

func (c *fakeGeminiGatewayCache) GetSessionAccountID(_ context.Context, groupID int64, sessionHash string) (int64, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	id, ok := c.sessions[c.key(groupID, sessionHash)]
	if !ok {
		return 0, service.ErrStickySessionNotFound
	}
	return id, nil
}

func (c *fakeGeminiGatewayCache) SetSessionAccountID(_ context.Context, groupID int64, sessionHash string, accountID int64, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sessions[c.key(groupID, sessionHash)] = accountID
	return nil
}

func (c *fakeGeminiGatewayCache) RefreshSessionTTL(_ context.Context, _ int64, _ string, _ time.Duration) error {
	return nil
}

func (c *fakeGeminiGatewayCache) DeleteSessionAccountID(_ context.Context, groupID int64, sessionHash string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.sessions, c.key(groupID, sessionHash))
	return nil
}

func (c *fakeGeminiGatewayCache) SetGrokVideoPendingBilling(_ context.Context, _ string, _ []byte, _ time.Duration) error {
	return nil
}

func (c *fakeGeminiGatewayCache) GetGrokVideoPendingBilling(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}

func (c *fakeGeminiGatewayCache) ClaimGrokVideoBilled(_ context.Context, _ string, _ time.Duration) (bool, error) {
	return true, nil
}

func (c *fakeGeminiGatewayCache) ReleaseGrokVideoBilled(_ context.Context, _ string) error {
	return nil
}

func (c *fakeGeminiGatewayCache) SetReasoningContent(_ context.Context, _ string, _ string, _ time.Duration) error {
	return nil
}

func (c *fakeGeminiGatewayCache) GetReasoningContent(_ context.Context, _ string) (string, error) {
	return "", service.ErrReasoningContentNotFound
}

func newTestGeminiContext(body []byte, headers map[string]string) *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent", bytes.NewReader(body))
	req.RemoteAddr = "192.168.1.10:54321"
	req.Header.Set("User-Agent", "Antigravity/2.0.0")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	c.Request = req
	return c
}

func TestResolveAndPersistGeminiStickySession_DigestAndAnchorAndFailover(t *testing.T) {
	cache := newFakeGeminiGatewayCache()
	digestStore := service.NewDigestSessionStore()

	gwService := service.NewGatewayService(
		nil, nil, nil, nil, nil, nil, nil,
		cache, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		digestStore, nil, nil, nil, nil, nil, nil, nil,
	)
	h := &GatewayHandler{gatewayService: gwService}
	groupID := int64(7)
	apiKey := &service.APIKey{
		ID:      42,
		GroupID: &groupID,
		Group:   &service.Group{ID: groupID, Platform: service.PlatformAntigravity},
	}
	reqLog := zap.NewNop()

	turn1Body := []byte(`{"systemInstruction":{"parts":[{"text":"You are a coding assistant"}]},"contents":[{"role":"user","parts":[{"text":"Fix the cache bug"}]}]}`)
	c1 := newTestGeminiContext(turn1Body, nil)
	state1 := h.resolveGeminiStickySession(c1, reqLog, 1001, apiKey, "gemini-2.5-pro", turn1Body, nil)

	require.True(t, state1.useDigestFallback)
	require.Equal(t, int64(0), state1.sessionBoundAccountID)
	require.True(t, strings.HasPrefix(state1.sessionKey, "gemini:digest:"), "turn 1 should replace full-body hash with stable digest sessionKey, got %q", state1.sessionKey)
	require.NotEmpty(t, state1.geminiAnchorKey)

	// Turn 1 selects account 101 and persists sticky state
	_ = gwService.BindStickySession(c1.Request.Context(), apiKey.GroupID, state1.sessionKey, 101)
	h.persistGeminiStickySession(c1, reqLog, apiKey.GroupID, 101, &state1)

	// Turn 2: DigestSessionStore hits -> binds account 101 + injects PrefetchedStickySession
	turn2Body := []byte(`{"systemInstruction":{"parts":[{"text":"You are a coding assistant"}]},"contents":[{"role":"user","parts":[{"text":"Fix the cache bug"}]},{"role":"model","parts":[{"text":"Here is the cause"}]},{"role":"user","parts":[{"text":"Now write tests"}]}]}`)
	c2 := newTestGeminiContext(turn2Body, nil)
	state2 := h.resolveGeminiStickySession(c2, reqLog, 1001, apiKey, "gemini-2.5-pro", turn2Body, nil)

	require.Equal(t, int64(101), state2.sessionBoundAccountID)
	require.Equal(t, state1.sessionKey, state2.sessionKey)
	prefetchedAcc, ok := service.PrefetchedStickyAccountIDFromContext(c2.Request.Context())
	require.True(t, ok)
	require.Equal(t, int64(101), prefetchedAcc)
	h.persistGeminiStickySession(c2, reqLog, apiKey.GroupID, 101, &state2)

	// Turn 3: Simulate DigestSessionStore eviction/restart + middle-turn context compression;
	// Redis anchor key still recovers account 101 because systemInstruction + first user turn are unchanged.
	freshDigestStore := service.NewDigestSessionStore()

	gwServiceAfterRestart := service.NewGatewayService(
		nil, nil, nil, nil, nil, nil, nil,
		cache, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		freshDigestStore, nil, nil, nil, nil, nil, nil, nil,
	)
	hRestarted := &GatewayHandler{gatewayService: gwServiceAfterRestart}

	turn3CompressedBody := []byte(`{"systemInstruction":{"parts":[{"text":"You are a coding assistant"}]},"contents":[{"role":"user","parts":[{"text":"Fix the cache bug"}]},{"role":"model","parts":[{"text":"[compressed summary]"}]},{"role":"user","parts":[{"text":"Final check"}]}]}`)
	c3 := newTestGeminiContext(turn3CompressedBody, nil)
	state3 := hRestarted.resolveGeminiStickySession(c3, reqLog, 1001, apiKey, "gemini-2.5-pro", turn3CompressedBody, nil)

	require.Equal(t, int64(101), state3.sessionBoundAccountID, "Redis anchor key should recover sticky account when digest store misses")
	require.Equal(t, state1.sessionKey, state3.sessionKey, "restart recovery must preserve the upstream session identity")
	prefetchedAcc3, ok3 := service.PrefetchedStickyAccountIDFromContext(c3.Request.Context())
	require.True(t, ok3)
	require.Equal(t, int64(101), prefetchedAcc3)

	// Simulate account failover on Turn 3 to account 202
	_ = gwServiceAfterRestart.BindStickySession(c3.Request.Context(), apiKey.GroupID, state3.sessionKey, 202)
	hRestarted.persistGeminiStickySession(c3, reqLog, apiKey.GroupID, 202, &state3)

	// Turn 4: Subsequent request routes to the new failover account 202
	turn4Body := []byte(`{"systemInstruction":{"parts":[{"text":"You are a coding assistant"}]},"contents":[{"role":"user","parts":[{"text":"Fix the cache bug"}]},{"role":"model","parts":[{"text":"[compressed summary]"}]},{"role":"user","parts":[{"text":"Final check"}]},{"role":"model","parts":[{"text":"Done"}]},{"role":"user","parts":[{"text":"Thanks"}]}]}`)
	c4 := newTestGeminiContext(turn4Body, nil)
	state4 := hRestarted.resolveGeminiStickySession(c4, reqLog, 1001, apiKey, "gemini-2.5-pro", turn4Body, nil)
	require.Equal(t, int64(202), state4.sessionBoundAccountID, "after failover, sticky session should route to new account 202")
}

func TestExtractGeminiExplicitSessionHash_HeadersAndBodyAndIsolation(t *testing.T) {
	bodyWithCamel := []byte(`{"sessionId":"sess-abc","contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	bodyWithSnake := []byte(`{"session_id":"sess-abc","contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)

	cHeader := newTestGeminiContext([]byte(`{}`), map[string]string{"x-gemini-session-id": "sess-abc"})
	cXSession := newTestGeminiContext([]byte(`{}`), map[string]string{"x-session-id": "sess-abc"})
	cCamel := newTestGeminiContext(bodyWithCamel, nil)
	cSnake := newTestGeminiContext(bodyWithSnake, nil)

	h1 := extractGeminiExplicitSessionHash(cHeader, 10, []byte(`{}`))
	h2 := extractGeminiExplicitSessionHash(cXSession, 10, []byte(`{}`))
	h3 := extractGeminiExplicitSessionHash(cCamel, 10, bodyWithCamel)
	h4 := extractGeminiExplicitSessionHash(cSnake, 10, bodyWithSnake)
	require.NotEmpty(t, h1)
	require.Equal(t, h1, h2)
	require.Equal(t, h1, h3)
	require.Equal(t, h1, h4)

	// Different API key ID must produce a different hash to prevent cross-key collision
	hOtherKey := extractGeminiExplicitSessionHash(cHeader, 99, []byte(`{}`))
	require.NotEqual(t, h1, hOtherKey)
}

func TestGeminiCLIStickySessionScopesCallerControlledIdentity(t *testing.T) {
	cache := newFakeGeminiGatewayCache()
	gw := service.NewGatewayService(
		nil, nil, nil, nil, nil, nil, nil,
		cache, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		service.NewDigestSessionStore(), nil, nil, nil, nil, nil, nil, nil,
	)
	h := &GatewayHandler{gatewayService: gw}
	body := []byte(fmt.Sprintf(`{"contents":[{"role":"user","parts":[{"text":"/home/user/.gemini/tmp/%s"}]}]}`, strings.Repeat("a", 64)))
	headers := map[string]string{"x-gemini-api-privileged-user-id": "same-client-value"}
	groupID := int64(7)
	firstKey := &service.APIKey{ID: 11, GroupID: &groupID}
	secondKey := &service.APIKey{ID: 12, GroupID: &groupID}
	c1 := newTestGeminiContext(body, headers)
	first := h.resolveGeminiStickySession(c1, zap.NewNop(), 1, firstKey, "gemini-4-argon", body, nil)
	require.NoError(t, gw.BindStickySession(c1.Request.Context(), &groupID, first.sessionKey, 101))
	c2 := newTestGeminiContext(body, headers)
	second := h.resolveGeminiStickySession(c2, zap.NewNop(), 1, secondKey, "gemini-4-argon", body, nil)
	require.NotEqual(t, first.sessionKey, second.sessionKey)
	require.Zero(t, second.sessionBoundAccountID)
	bound := h.resolveGeminiStickySession(c2, zap.NewNop(), 1, firstKey, "gemini-4-argon", body, &service.GeminiCachedContent{})
	require.Empty(t, bound.sessionKey, "explicit cachedContent ownership must bypass implicit session affinity")
}
