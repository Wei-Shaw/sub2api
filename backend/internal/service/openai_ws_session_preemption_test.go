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

	"github.com/Wei-Shaw/sub2api/internal/config"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type openAIWSPreemptCloseRecorder struct {
	mu           sync.Mutex
	watchCtx     context.Context
	ctxErrAtSend error
	codes        []coderws.StatusCode
	reason       string
	release      chan struct{}
	closed       chan struct{}
}

func (r *openAIWSPreemptCloseRecorder) Close(code coderws.StatusCode, reason string) error {
	r.mu.Lock()
	r.codes = append(r.codes, code)
	r.reason = reason
	if r.watchCtx != nil {
		r.ctxErrAtSend = r.watchCtx.Err()
	}
	r.mu.Unlock()
	if r.release != nil {
		<-r.release
	}
	close(r.closed)
	return nil
}

func TestOpenAIWSIngressSessionPreemptionSendsCloseFrameBeforeCancel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{}
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	firstMessage := []byte(`{"type":"response.create","input":"hello"}`)
	closer := &openAIWSPreemptCloseRecorder{closed: make(chan struct{})}

	firstCtx, firstCleanup, armed := svc.BeginOpenAIWSIngressSessionPreemptionWithClient(
		context.Background(), newOpenAIWSPreemptCodexContext(11, "thread-a"), account, firstMessage, closer,
	)
	require.True(t, armed)
	defer firstCleanup()
	closer.watchCtx = firstCtx

	_, secondCleanup, armed := svc.BeginOpenAIWSIngressSessionPreemptionWithClient(
		context.Background(), newOpenAIWSPreemptCodexContext(11, "thread-a"), account, firstMessage, nil,
	)
	require.True(t, armed)
	defer secondCleanup()

	require.True(t, isOpenAIWSSessionPreempted(firstCtx), "关闭帧发出前旧连接就应被判定为已抢占")
	select {
	case <-closer.closed:
	case <-time.After(time.Second):
		t.Fatal("旧客户端应收到关闭帧")
	}
	closer.mu.Lock()
	require.Equal(t, []coderws.StatusCode{coderws.StatusTryAgainLater}, closer.codes)
	require.Equal(t, openAIWSSessionPreemptedCloseReason, closer.reason)
	require.NoError(t, closer.ctxErrAtSend, "取消必须等关闭帧发出之后")
	closer.mu.Unlock()
	select {
	case <-firstCtx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("旧连接应在关闭帧之后被取消")
	}
	require.ErrorIs(t, context.Cause(firstCtx), errOpenAIWSSessionPreempted)
}

func TestOpenAIWSIngressSessionPreemptionCancelsAfterCloseGrace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{}
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	firstMessage := []byte(`{"type":"response.create","input":"hello"}`)
	closer := &openAIWSPreemptCloseRecorder{closed: make(chan struct{}), release: make(chan struct{})}
	defer close(closer.release)

	firstCtx, firstCleanup, armed := svc.BeginOpenAIWSIngressSessionPreemptionWithClient(
		context.Background(), newOpenAIWSPreemptCodexContext(11, "thread-a"), account, firstMessage, closer,
	)
	require.True(t, armed)
	defer firstCleanup()

	started := time.Now()
	_, secondCleanup, armed := svc.BeginOpenAIWSIngressSessionPreemptionWithClient(
		context.Background(), newOpenAIWSPreemptCodexContext(11, "thread-a"), account, firstMessage, nil,
	)
	require.True(t, armed)
	defer secondCleanup()
	require.Less(t, time.Since(started), openAIWSSessionPreemptCloseGrace, "新连接不得等待旧客户端的关闭握手")

	select {
	case <-firstCtx.Done():
	case <-time.After(openAIWSSessionPreemptCloseGrace + time.Second):
		t.Fatal("对端不回应关闭帧时也必须在宽限期内取消旧连接")
	}
	require.ErrorIs(t, context.Cause(firstCtx), errOpenAIWSSessionPreempted)
}

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

	_, cleanup, armed, _ := svc.beginOpenAIWSSessionPreemptContext(context.Background(), apiKey, 7, 11, "sess", false, nil)
	cleanup()
	require.False(t, armed)
	_, cleanup, armed, _ = svc.beginOpenAIWSSessionPreemptContext(context.Background(), grok, 7, 11, "sess", false, nil)
	cleanup()
	require.False(t, armed)
	_, cleanup, armed, _ = svc.beginOpenAIWSSessionPreemptContext(context.Background(), oauth, 7, 11, "sess", true, nil)
	cleanup()
	require.False(t, armed, "HTTP-ingress one-shot must not participate")

	firstCtx, firstCleanup, armed, replaced := svc.beginOpenAIWSSessionPreemptContext(context.Background(), oauth, 7, 11, "sess", false, nil)
	require.True(t, armed)
	require.False(t, replaced)
	stateStore.BindSessionTurnState(7, "sess", "turn-state", time.Hour)
	stateStore.BindSessionConn(7, "sess", "conn-1", time.Hour)
	_, secondCleanup, armed, replaced := svc.beginOpenAIWSSessionPreemptContext(context.Background(), oauth, 7, 11, "sess", false, nil)
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

func TestOpenAIWSIngressSessionPreemptionRespectsResolvedMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(7)
	newContext := func() *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
		c.Set("api_key", &APIKey{ID: 11, GroupID: &groupID})
		return c
	}
	newAccount := func(mode string) *Account {
		return &Account{
			ID:       1,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"openai_oauth_responses_websockets_v2_mode": mode,
			},
		}
	}
	firstMessage := []byte(`{"type":"response.create","prompt_cache_key":"session-1","input":"hello"}`)
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.IngressModeDefault = OpenAIWSIngressModeCtxPool
	svc := &OpenAIGatewayService{cfg: cfg}

	passthrough := newAccount(OpenAIWSIngressModePassthrough)
	firstCtx, firstCleanup, armed := svc.BeginOpenAIWSIngressSessionPreemption(
		context.Background(), newContext(), passthrough, firstMessage,
	)
	require.False(t, armed)
	defer firstCleanup()
	secondCtx, secondCleanup, armed := svc.BeginOpenAIWSIngressSessionPreemption(
		context.Background(), newContext(), passthrough, firstMessage,
	)
	require.False(t, armed)
	defer secondCleanup()
	require.NoError(t, firstCtx.Err(), "concurrent passthrough request must remain isolated")
	require.NoError(t, secondCtx.Err())

	ctxPool := newAccount(OpenAIWSIngressModeCtxPool)
	sharedCtx, sharedCleanup, armed := svc.BeginOpenAIWSIngressSessionPreemption(
		context.Background(), newContext(), ctxPool, firstMessage,
	)
	require.True(t, armed)
	defer sharedCleanup()
	_, replacementCleanup, armed := svc.BeginOpenAIWSIngressSessionPreemption(
		context.Background(), newContext(), ctxPool, firstMessage,
	)
	require.True(t, armed)
	defer replacementCleanup()
	require.True(t, IsOpenAIWSSessionPreemptedError(context.Cause(sharedCtx)), "ctx_pool must retain shared-session preemption")
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

func TestOpenAIWSHTTPBridgeSessionPreemptionEligibility(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name          string
		routerEnabled bool
		defaultMode   string
		accountMode   string
		wantArmed     bool
	}{
		{name: "explicit HTTP bridge", routerEnabled: true, defaultMode: OpenAIWSIngressModeCtxPool, accountMode: OpenAIWSIngressModeHTTPBridge},
		{name: "default HTTP bridge", routerEnabled: true, defaultMode: OpenAIWSIngressModeHTTPBridge},
		{name: "ctx pool overrides bridge default", routerEnabled: true, defaultMode: OpenAIWSIngressModeHTTPBridge, accountMode: OpenAIWSIngressModeCtxPool, wantArmed: true},
		{name: "disabled router retains legacy preemption", defaultMode: OpenAIWSIngressModeHTTPBridge, accountMode: OpenAIWSIngressModeHTTPBridge, wantArmed: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = tt.routerEnabled
			cfg.Gateway.OpenAIWS.IngressModeDefault = tt.defaultMode
			cache := &openAIWSSessionPreemptCacheStub{}
			svc := &OpenAIGatewayService{cfg: cfg, cache: cache}
			groupID := int64(7)
			newContext := func() *gin.Context {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
				c.Request.Header.Set("session-id", "shared-session")
				c.Set("api_key", &APIKey{ID: 11, GroupID: &groupID})
				return c
			}
			account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
				Extra: map[string]any{"openai_oauth_responses_websockets_v2_mode": tt.accountMode}}
			firstMessage := []byte(`{"type":"response.create","prompt_cache_key":"cache-a","input":"first request"}`)
			secondMessage := []byte(`{"type":"response.create","prompt_cache_key":"cache-b","input":"second request"}`)
			first, cleanupFirst, firstArmed := svc.BeginOpenAIWSIngressSessionPreemption(context.Background(), newContext(), account, firstMessage)
			defer cleanupFirst()
			second, cleanupSecond, secondArmed := svc.BeginOpenAIWSIngressSessionPreemption(context.Background(), newContext(), account, secondMessage)
			defer cleanupSecond()
			require.Equal(t, tt.wantArmed, firstArmed)
			require.Equal(t, tt.wantArmed, secondArmed)
			require.NoError(t, second.Err())
			if tt.wantArmed {
				require.True(t, IsOpenAIWSSessionPreemptedError(context.Cause(first)))
			} else {
				require.NoError(t, first.Err(), "independent HTTP bridges must not cancel each other")
				cache.mu.Lock()
				ownerCount := len(cache.owners)
				cache.mu.Unlock()
				require.Zero(t, ownerCount, "HTTP bridges must not claim a distributed preemption owner")
			}
		})
	}
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

func newOpenAIWSPreemptCodexContext(apiKeyID int64, threadID string) *gin.Context {
	groupID := int64(7)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	c.Request.Header.Set("session-id", "root-session")
	if threadID != "" {
		c.Request.Header.Set(openAIWSTurnMetadataHeader, `{"session_id":"root-session","thread_id":"`+threadID+`"}`)
	}
	c.Set("api_key", &APIKey{ID: apiKeyID, GroupID: &groupID})
	return c
}

func TestOpenAIWSIngressSessionPreemptionIsolatesCodexThreads(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{}
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	firstMessage := []byte(`{"type":"response.create","input":"hello"}`)
	retryMessage := []byte(`{"type":"response.create","input":[{"role":"user","content":"hello"},{"role":"user","content":"again"}]}`)

	rootCtx, rootCleanup, armed := svc.BeginOpenAIWSIngressSessionPreemption(
		context.Background(), newOpenAIWSPreemptCodexContext(11, "root-session"), account, firstMessage,
	)
	require.True(t, armed)
	defer rootCleanup()

	childCtx, childCleanup, armed := svc.BeginOpenAIWSIngressSessionPreemption(
		context.Background(), newOpenAIWSPreemptCodexContext(11, "child-thread"), account, firstMessage,
	)
	require.True(t, armed)
	defer childCleanup()
	require.NoError(t, rootCtx.Err(), "same session but different codex thread must not preempt")
	require.NoError(t, childCtx.Err())

	otherKeyCtx, otherKeyCleanup, armed := svc.BeginOpenAIWSIngressSessionPreemption(
		context.Background(), newOpenAIWSPreemptCodexContext(12, "root-session"), account, firstMessage,
	)
	require.True(t, armed)
	defer otherKeyCleanup()
	require.NoError(t, rootCtx.Err(), "same thread under another api key must not preempt")
	require.NoError(t, otherKeyCtx.Err())

	_, rootRetryCleanup, armed := svc.BeginOpenAIWSIngressSessionPreemption(
		context.Background(), newOpenAIWSPreemptCodexContext(11, "root-session"), account, retryMessage,
	)
	require.True(t, armed)
	defer rootRetryCleanup()
	require.True(t, IsOpenAIWSSessionPreemptedError(context.Cause(rootCtx)), "same thread reconnect with a different body must still preempt")
	require.NoError(t, childCtx.Err(), "root reconnect must leave the child thread alone")
	require.NoError(t, otherKeyCtx.Err())
}

func newOpenAIWSPreemptCodexKindContext(apiKeyID int64, threadID, requestKind string) *gin.Context {
	c := newOpenAIWSPreemptCodexContext(apiKeyID, threadID)
	c.Request.Header.Set(openAIWSTurnMetadataHeader, `{"session_id":"root-session","thread_id":"`+threadID+`","request_kind":"`+requestKind+`"}`)
	return c
}

func TestOpenAIWSIngressSessionPreemptionKeepsDetachedRequestsApart(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{}
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	turnMessage := []byte(`{"type":"response.create","input":"hello"}`)
	memoryMessage := []byte(`{"type":"response.create","input":"consolidate"}`)

	turnCtx, turnCleanup, armed := svc.BeginOpenAIWSIngressSessionPreemption(
		context.Background(), newOpenAIWSPreemptCodexKindContext(11, "thread-a", "turn"), account, turnMessage,
	)
	require.True(t, armed)
	defer turnCleanup()

	memoryCtx, memoryCleanup, armed := svc.BeginOpenAIWSIngressSessionPreemption(
		context.Background(), newOpenAIWSPreemptCodexKindContext(11, "thread-a", "memory"), account, memoryMessage,
	)
	require.True(t, armed)
	defer memoryCleanup()
	require.NoError(t, turnCtx.Err(), "memory consolidation on the same thread must not preempt the user turn")
	require.NoError(t, memoryCtx.Err())

	prewarmCtx, prewarmCleanup, armed := svc.BeginOpenAIWSIngressSessionPreemption(
		context.Background(), newOpenAIWSPreemptCodexKindContext(11, "thread-a", "prewarm"), account, turnMessage,
	)
	require.True(t, armed)
	defer prewarmCleanup()
	require.True(t, IsOpenAIWSSessionPreemptedError(context.Cause(turnCtx)), "prewarm shares the turn lane and replaces the stale turn connection")
	require.NoError(t, memoryCtx.Err(), "turn lane reconnect must leave memory consolidation alone")

	_, memoryRetryCleanup, armed := svc.BeginOpenAIWSIngressSessionPreemption(
		context.Background(), newOpenAIWSPreemptCodexKindContext(11, "thread-a", "memory"), account, memoryMessage,
	)
	require.True(t, armed)
	defer memoryRetryCleanup()
	require.True(t, IsOpenAIWSSessionPreemptedError(context.Cause(memoryCtx)), "memory reconnect replaces the stale memory connection")
	require.NoError(t, prewarmCtx.Err(), "memory reconnect must leave the turn lane alone")

	scorer := newOpenAIWSPreemptCodexContext(11, "")
	scorer.Request.Header.Set(openAIWSThreadIDHeader, "thread-a")
	scorer.Request.Header.Set(openAISubagentHeader, "guardian")
	scorerCtx, scorerCleanup, armed := svc.BeginOpenAIWSIngressSessionPreemption(context.Background(), scorer, account, turnMessage)
	require.True(t, armed)
	defer scorerCleanup()
	require.NoError(t, prewarmCtx.Err(), "guardian scorer without thread metadata must not preempt the user turn")
	require.NoError(t, scorerCtx.Err())
}

func TestOpenAIWSIngressSessionPreemptionSkipsContentOnlyIdentity(t *testing.T) {
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
	contentOnly := []byte(`{"type":"response.create","model":"gpt-5.1","instructions":"sys","input":[{"role":"user","content":"same prompt"}]}`)

	firstCtx, firstCleanup, armed := svc.BeginOpenAIWSIngressSessionPreemption(context.Background(), newContext(), account, contentOnly)
	require.False(t, armed, "content-derived seed is not an identity and must not arm preemption")
	defer firstCleanup()
	_, secondCleanup, armed := svc.BeginOpenAIWSIngressSessionPreemption(context.Background(), newContext(), account, contentOnly)
	require.False(t, armed)
	defer secondCleanup()
	require.NoError(t, firstCtx.Err())
}

func TestOpenAIWSIngressSessionPreemptionClaimsRemoteOwnerByExecutionScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &openAIWSSessionPreemptCacheStub{}
	svc := &OpenAIGatewayService{cache: stub}
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	firstMessage := []byte(`{"type":"response.create","input":"hello"}`)
	c := newOpenAIWSPreemptCodexContext(11, "thread-a")

	_, cleanup, armed := svc.BeginOpenAIWSIngressSessionPreemption(context.Background(), c, account, firstMessage)
	require.True(t, armed)
	defer cleanup()

	scope, _ := resolveOpenAIWSExecutionScope(c, firstMessage, 11)
	legacy := svc.GenerateSessionHash(c, firstMessage)
	require.NotEmpty(t, scope)
	require.NotEqual(t, legacy, scope)

	stub.mu.Lock()
	defer stub.mu.Unlock()
	_, scoped := stub.owners[stub.key(7, openAIWSSessionPreemptCacheHash(11, scope))]
	_, legacyKeyed := stub.owners[stub.key(7, openAIWSSessionPreemptCacheHash(11, legacy))]
	require.True(t, scoped, "remote owner must be claimed under the execution scope")
	require.False(t, legacyKeyed, "remote owner must not be claimed under the legacy session hash")
}
