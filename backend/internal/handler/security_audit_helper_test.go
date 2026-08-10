package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCachesSecurityAuditCompletionSkipsWebSocketStages(t *testing.T) {
	require.True(t, cachesSecurityAuditCompletion("http"))
	require.True(t, cachesSecurityAuditCompletion(""))
	require.False(t, cachesSecurityAuditCompletion("first_turn"))
	require.False(t, cachesSecurityAuditCompletion("subsequent_turn"))
	require.True(t, isSecurityAuditWebSocketStage("first_turn"))
	require.True(t, isSecurityAuditWebSocketStage("subsequent_turn"))
	require.False(t, isSecurityAuditWebSocketStage("http"))
}

func TestRunSecurityAuditDoesNotSkipSubsequentWebSocketTurns(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := &turnCountingEngine{mode: securityaudit.ModeAsync}
	coordinator := securityaudit.NewCoordinator(nil, engine)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	subject := middleware2.AuthSubject{UserID: 7, Concurrency: 1}
	first := runSecurityAudit(c, nil, coordinator, nil, nil, subject, "openai_responses", "gpt-test",
		[]byte(`{"type":"response.create","response":{"input":"benign"}}`), "first_turn")
	require.NotNil(t, first)
	require.True(t, first.AllowNextStage)
	require.Equal(t, int64(1), engine.enqueues.Load())
	_, cached := c.Get(securityAuditCompletedContextKey)
	require.False(t, cached, "WebSocket stages must not set the HTTP completion cache")

	// Even if an HTTP path previously cached completion on this Context, WS turns
	// must still audit every response.create payload.
	c.Set(securityAuditCompletedContextKey, true)

	second := runSecurityAudit(c, nil, coordinator, nil, nil, subject, "openai_responses", "gpt-test",
		[]byte(`{"type":"response.create","response":{"input":"malicious follow-up"}}`), "subsequent_turn")
	require.NotNil(t, second)
	require.Equal(t, int64(2), engine.enqueues.Load(), "subsequent WebSocket turns must be audited again")
}

func TestRunSecurityAuditDeduplicatesRepeatedPayloadWithinWebSocketTurn(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := &turnCountingEngine{mode: securityaudit.ModeBlocking}
	coordinator := securityaudit.NewCoordinator(nil, engine)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	payload := []byte(`{"type":"response.create","response":{"input":"same turn"}}`)
	c.Set(securityAuditWSTurnContextKey, 2)
	first := runSecurityAudit(c, nil, coordinator, nil, nil, middleware2.AuthSubject{UserID: 7}, "openai_responses", "gpt-test", payload, "subsequent_turn")
	second := runSecurityAudit(c, nil, coordinator, nil, nil, middleware2.AuthSubject{UserID: 7}, "openai_responses", "gpt-test", payload, "subsequent_turn")
	require.NotNil(t, first)
	require.NotNil(t, second)
	require.True(t, first.AllowNextStage)
	require.True(t, second.AllowNextStage)
	require.Equal(t, int64(1), engine.evaluates.Load())

	// The cache holds only one successful same-turn result.
	entry, exists := c.Get(securityAuditWSDedupeContextKey)
	require.True(t, exists)
	require.IsType(t, securityAuditWSDedupeEntry{}, entry)

	c.Set(securityAuditWSTurnContextKey, 3)
	runSecurityAudit(c, nil, coordinator, nil, nil, middleware2.AuthSubject{UserID: 7}, "openai_responses", "gpt-test", payload, "subsequent_turn")
	require.Equal(t, int64(2), engine.evaluates.Load())
}

func TestRunSecurityAuditDoesNotCacheFailedWebSocketDecision(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := &turnCountingEngine{
		mode: securityaudit.ModeBlocking,
		decisions: []*securityaudit.PromptDecision{
			{Kind: securityaudit.DecisionUnavailable, AllowNextStage: false},
			{Kind: securityaudit.DecisionAllow, AllowNextStage: true},
		},
	}
	coordinator := securityaudit.NewCoordinator(nil, engine)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set(securityAuditWSTurnContextKey, 2)
	payload := []byte(`{"type":"response.create","response":{"input":"retry me"}}`)

	first := runSecurityAudit(c, nil, coordinator, nil, nil, middleware2.AuthSubject{UserID: 7}, "openai_responses", "gpt-test", payload, "subsequent_turn")
	_, cachedAfterFailure := c.Get(securityAuditWSDedupeContextKey)
	second := runSecurityAudit(c, nil, coordinator, nil, nil, middleware2.AuthSubject{UserID: 7}, "openai_responses", "gpt-test", payload, "subsequent_turn")

	require.False(t, first.AllowNextStage)
	require.False(t, cachedAfterFailure)
	require.True(t, second.AllowNextStage)
	require.Equal(t, int64(2), engine.evaluates.Load())
}

func TestRunSecurityAuditDoesNotCacheFlaggedWebSocketDecision(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := &turnCountingEngine{
		mode: securityaudit.ModeBlocking,
		decisions: []*securityaudit.PromptDecision{
			{Kind: securityaudit.DecisionFlag, AllowNextStage: true},
			{Kind: securityaudit.DecisionAllow, AllowNextStage: true},
		},
	}
	coordinator := securityaudit.NewCoordinator(nil, engine)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set(securityAuditWSTurnContextKey, 2)
	payload := []byte(`{"type":"response.create","response":{"input":"retry flagged"}}`)

	first := runSecurityAudit(c, nil, coordinator, nil, nil, middleware2.AuthSubject{UserID: 7}, "openai_responses", "gpt-test", payload, "subsequent_turn")
	_, cachedAfterFlag := c.Get(securityAuditWSDedupeContextKey)
	second := runSecurityAudit(c, nil, coordinator, nil, nil, middleware2.AuthSubject{UserID: 7}, "openai_responses", "gpt-test", payload, "subsequent_turn")

	require.Equal(t, securityaudit.DecisionFlag, first.Kind)
	require.True(t, first.AllowNextStage)
	require.False(t, cachedAfterFlag)
	require.Equal(t, securityaudit.DecisionAllow, second.Kind)
	require.Equal(t, int64(2), engine.evaluates.Load())
}

type turnCountingEngine struct {
	mode      securityaudit.Mode
	enqueues  atomic.Int64
	evaluates atomic.Int64
	decisions []*securityaudit.PromptDecision
}

func (e *turnCountingEngine) EffectiveMode() securityaudit.Mode { return e.mode }
func (e *turnCountingEngine) Enqueue(context.Context, securityaudit.Request) error {
	e.enqueues.Add(1)
	return nil
}
func (e *turnCountingEngine) Evaluate(context.Context, securityaudit.Request) (*securityaudit.PromptDecision, error) {
	call := e.evaluates.Add(1)
	if int(call) <= len(e.decisions) {
		return e.decisions[call-1], nil
	}
	return &securityaudit.PromptDecision{Kind: securityaudit.DecisionAllow, AllowNextStage: true}, nil
}
