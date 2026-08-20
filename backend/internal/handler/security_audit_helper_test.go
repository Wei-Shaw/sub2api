package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
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

func TestRunSecurityAuditLogsWebSocketChecksAndCacheHits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := &turnCountingEngine{mode: securityaudit.ModeBlocking}
	coordinator := securityaudit.NewCoordinator(nil, engine)
	core, logs := observer.New(zap.InfoLevel)
	reqLog := zap.New(core)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set(securityAuditWSTurnContextKey, 2)
	payload := []byte(`{"type":"response.create","response":{"input":"same turn"}}`)

	runSecurityAudit(c, reqLog, coordinator, nil, nil, middleware2.AuthSubject{UserID: 7}, "openai_responses", "gpt-test", payload, "subsequent_turn")
	runSecurityAudit(c, reqLog, coordinator, nil, nil, middleware2.AuthSubject{UserID: 7}, "openai_responses", "gpt-test", payload, "subsequent_turn")

	startLogs := logs.FilterMessage("security_audit.gateway_check_start").All()
	require.Len(t, startLogs, 1)
	require.Equal(t, false, startLogs[0].ContextMap()["cached"])

	doneLogs := logs.FilterMessage("security_audit.gateway_check_done").All()
	require.Len(t, doneLogs, 2)
	require.Equal(t, false, doneLogs[0].ContextMap()["cached"])
	require.Equal(t, true, doneLogs[1].ContextMap()["cached"])
	require.Equal(t, "allow", doneLogs[1].ContextMap()["decision"])
	require.Equal(t, "subsequent_turn", doneLogs[1].ContextMap()["stage"])
	require.Equal(t, int64(1), engine.evaluates.Load())
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

type promptAuditFallbackResolverStub struct {
	target          *service.Group
	resolveErr      error
	hasAccounts     bool
	availabilityErr error
	resolveCalls    int
}

func (s *promptAuditFallbackResolverStub) ResolveGroupByID(context.Context, int64) (*service.Group, error) {
	s.resolveCalls++
	if s.resolveErr != nil {
		return nil, s.resolveErr
	}
	return s.target, nil
}

func (s *promptAuditFallbackResolverStub) HasPromptAuditFallbackAccounts(context.Context, *service.Group, string, string) (bool, error) {
	return s.hasAccounts, s.availabilityErr
}

type promptAuditFallbackEngine struct {
	decision *securityaudit.PromptDecision
}

func (e *promptAuditFallbackEngine) EffectiveMode() securityaudit.Mode {
	return securityaudit.ModeBlocking
}
func (e *promptAuditFallbackEngine) Enqueue(context.Context, securityaudit.Request) error {
	return nil
}
func (e *promptAuditFallbackEngine) Evaluate(context.Context, securityaudit.Request) (*securityaudit.PromptDecision, error) {
	return e.decision, nil
}

func newPromptAuditFallbackTestContext(t *testing.T) (*gin.Context, *service.APIKey, *service.Group) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	groupID := int64(11)
	fallbackID := int64(22)
	source := &service.Group{
		ID:                                groupID,
		Name:                              "source",
		Platform:                          service.PlatformOpenAI,
		Status:                            service.StatusActive,
		SubscriptionType:                  service.SubscriptionTypeStandard,
		FallbackGroupIDOnPromptAuditBlock: &fallbackID,
	}
	override := 99
	apiKey := &service.APIKey{
		ID: 7, UserID: 3, GroupID: &groupID, Group: source,
		User: &service.User{ID: 3, UserGroupRPMOverride: &override},
	}
	c.Set(string(middleware2.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware2.ContextKeySubscription), &service.UserSubscription{})
	return c, apiKey, source
}

func TestRunSecurityAuditPromptBlockSwitchesGroupOnce(t *testing.T) {
	c, apiKey, source := newPromptAuditFallbackTestContext(t)
	target := &service.Group{ID: 22, Name: "fallback", Platform: service.PlatformOpenAI, Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeStandard}
	resolver := &promptAuditFallbackResolverStub{target: target, hasAccounts: true}
	coordinator := securityaudit.NewCoordinator(nil, &promptAuditFallbackEngine{
		decision: &securityaudit.PromptDecision{Kind: securityaudit.DecisionBlock, AllowNextStage: false},
	})
	subject := middleware2.AuthSubject{UserID: 3, Concurrency: 1}

	first := runSecurityAudit(c, nil, coordinator, nil, apiKey, subject, service.ContentModerationProtocolOpenAIChat, "gpt-test", nil, "first_turn", resolver)
	require.NotNil(t, first)
	require.True(t, first.AllowNextStage)
	require.Same(t, target, apiKey.Group)
	require.Equal(t, target.ID, *apiKey.GroupID)
	require.Nil(t, apiKey.User.UserGroupRPMOverride)
	require.Same(t, target, c.Request.Context().Value(ctxkey.Group))
	require.Nil(t, c.MustGet(string(middleware2.ContextKeySubscription)))
	require.True(t, promptAuditFallbackUsed(c))
	require.Equal(t, 1, resolver.resolveCalls)

	second := runSecurityAudit(c, nil, coordinator, nil, apiKey, subject, service.ContentModerationProtocolOpenAIChat, "gpt-test", nil, "subsequent_turn", resolver)
	require.NotNil(t, second)
	require.Equal(t, securityaudit.DecisionBlock, second.Kind)
	require.False(t, second.AllowNextStage)
	require.Same(t, target, apiKey.Group)
	require.Equal(t, 1, resolver.resolveCalls)
	require.Equal(t, int64(11), source.ID)
}

func TestPromptAuditFallbackDoesNotOverrideLegacyBlock(t *testing.T) {
	c, apiKey, source := newPromptAuditFallbackTestContext(t)
	target := &service.Group{ID: 22, Platform: service.PlatformOpenAI, Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeStandard}
	decision := securityaudit.Decision{
		Kind:   securityaudit.DecisionBlock,
		Legacy: &securityaudit.LegacyDecision{Blocked: true},
		Prompt: &securityaudit.PromptDecision{Kind: securityaudit.DecisionBlock},
	}

	require.False(t, tryPromptAuditFallback(c, nil, apiKey, service.ContentModerationProtocolOpenAIChat, "gpt-test", "http", "req", &decision, &promptAuditFallbackResolverStub{target: target, hasAccounts: true}))
	require.Equal(t, securityaudit.DecisionBlock, decision.Kind)
	require.Same(t, source, apiKey.Group)
}

func TestPromptAuditFallbackRejectsUnauthorizedExclusiveGroup(t *testing.T) {
	c, apiKey, source := newPromptAuditFallbackTestContext(t)
	target := &service.Group{ID: 22, Platform: service.PlatformOpenAI, Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeStandard, IsExclusive: true}
	decision := securityaudit.Decision{
		Kind:   securityaudit.DecisionBlock,
		Prompt: &securityaudit.PromptDecision{Kind: securityaudit.DecisionBlock},
	}

	require.False(t, tryPromptAuditFallback(c, nil, apiKey, service.ContentModerationProtocolOpenAIChat, "gpt-test", "http", "req", &decision, &promptAuditFallbackResolverStub{target: target, hasAccounts: true}))
	require.Equal(t, securityaudit.DecisionBlock, decision.Kind)
	require.Same(t, source, apiKey.Group)
}

func TestRunSecurityAuditPromptFallbackFailsClosedForUnavailableTargets(t *testing.T) {
	tests := []struct {
		name            string
		target          *service.Group
		resolveErr      error
		hasAccounts     bool
		availabilityErr error
	}{
		{name: "missing", resolveErr: errors.New("not found"), hasAccounts: true},
		{name: "inactive", target: &service.Group{ID: 22, Platform: service.PlatformOpenAI, Status: "inactive", SubscriptionType: service.SubscriptionTypeStandard}, hasAccounts: true},
		{name: "subscription", target: &service.Group{ID: 22, Platform: service.PlatformOpenAI, Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeSubscription}, hasAccounts: true},
		{name: "incompatible", target: &service.Group{ID: 22, Platform: service.PlatformAnthropic, Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeStandard}, hasAccounts: true},
		{name: "no_accounts", target: &service.Group{ID: 22, Platform: service.PlatformOpenAI, Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeStandard}, hasAccounts: false},
		{name: "availability_error", target: &service.Group{ID: 22, Platform: service.PlatformOpenAI, Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeStandard}, hasAccounts: true, availabilityErr: errors.New("accounts unavailable")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, apiKey, source := newPromptAuditFallbackTestContext(t)
			resolver := &promptAuditFallbackResolverStub{target: tt.target, resolveErr: tt.resolveErr, hasAccounts: tt.hasAccounts, availabilityErr: tt.availabilityErr}
			decision := runSecurityAudit(c, nil, securityaudit.NewCoordinator(nil, &promptAuditFallbackEngine{
				decision: &securityaudit.PromptDecision{Kind: securityaudit.DecisionBlock, AllowNextStage: false},
			}), nil, apiKey, middleware2.AuthSubject{UserID: 3}, service.ContentModerationProtocolOpenAIChat, "gpt-test", nil, "http", resolver)
			require.NotNil(t, decision)
			require.Equal(t, securityaudit.DecisionBlock, decision.Kind)
			require.False(t, decision.AllowNextStage)
			require.Same(t, source, apiKey.Group)
			require.False(t, promptAuditFallbackUsed(c))
		})
	}
}

func TestPromptAuditFallbackPlatformCompatibility(t *testing.T) {
	tests := []struct {
		name           string
		sourcePlatform string
		targetPlatform string
		protocol       string
		model          string
		resolvedSource string
		want           bool
	}{
		{name: "openai to grok", sourcePlatform: service.PlatformOpenAI, targetPlatform: service.PlatformGrok, protocol: service.ContentModerationProtocolOpenAIChat, model: "gpt-5", want: true},
		{name: "live stays openai", sourcePlatform: service.PlatformOpenAI, targetPlatform: service.PlatformGrok, protocol: "openai_live", model: "gpt-realtime", want: false},
		{name: "grok search stays grok", sourcePlatform: service.PlatformGrok, targetPlatform: service.PlatformOpenAI, protocol: "grok_web_search", model: "grok-4.5", want: false},
		{name: "gemini to antigravity", sourcePlatform: service.PlatformGemini, targetPlatform: service.PlatformAntigravity, protocol: service.ContentModerationProtocolGemini, model: "gemini-2.5-pro", want: true},
		{name: "composite resolved cn", sourcePlatform: service.PlatformComposite, targetPlatform: service.PlatformKimi, protocol: service.ContentModerationProtocolOpenAIChat, model: "provider-specific", resolvedSource: service.PlatformKimi, want: true},
		{name: "composite resolved cn rejects other cn", sourcePlatform: service.PlatformComposite, targetPlatform: service.PlatformZhipu, protocol: service.ContentModerationProtocolOpenAIChat, model: "provider-specific", resolvedSource: service.PlatformKimi, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.resolvedSource != "" {
				ctx = service.WithResolvedTargetPlatform(ctx, tt.resolvedSource)
			}
			source := &service.Group{Platform: tt.sourcePlatform}
			target := &service.Group{Platform: tt.targetPlatform}
			require.Equal(t, tt.want, promptAuditFallbackPlatformCompatible(ctx, source, target, tt.protocol, tt.model))
		})
	}
}
