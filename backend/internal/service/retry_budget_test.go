package service

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRetryBudgetExplicitHeaderAndDefaultRule(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, header, ua, origin, rule string
	}{
		{name: "explicit", header: "1", rule: "explicit_header"},
		{name: "embedded", ua: "codex/1.2.3", origin: "codex_cli_rs", rule: "codex-cli"},
		{name: "unknown", ua: "curl/8", origin: "curl", rule: "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
			c.Request.Header.Set(clientRetryHeader, tc.header)
			c.Request.Header.Set("User-Agent", tc.ua)
			c.Request.Header.Set("originator", tc.origin)
			c.Set("api_key", &APIKey{Group: &Group{UnifiedRetryBudgetEnabled: true}})
			budget := GetOrCreateRetryBudget(c)
			require.NotNil(t, budget)
			require.Equal(t, tc.rule, budget.ClientRuleID)
			require.Empty(t, c.Request.Header.Get(clientRetryHeader))
		})
	}
}

func TestEffectiveQuotaSticky(t *testing.T) {
	key := &APIKey{QuotaStickyMode: QuotaStickyModeDisabled, Group: &Group{QuotaStickyDefaultEnabled: true}}
	enabled, source := key.EffectiveQuotaSticky()
	require.True(t, enabled)
	require.Equal(t, "group_default", source)
	key.Group.QuotaStickyUserOverrideAllowed = true
	enabled, source = key.EffectiveQuotaSticky()
	require.False(t, enabled)
	require.Equal(t, "api_key_override", source)
}

type sessionModelTestCache struct{ model string }

func (c *sessionModelTestCache) GetSessionAccountID(context.Context, int64, string) (int64, error) {
	return 0, nil
}
func (c *sessionModelTestCache) SetSessionAccountID(context.Context, int64, string, int64, time.Duration) error {
	return nil
}
func (c *sessionModelTestCache) RefreshSessionTTL(context.Context, int64, string, time.Duration) error {
	return nil
}
func (c *sessionModelTestCache) DeleteSessionAccountID(context.Context, int64, string) error {
	return nil
}
func (c *sessionModelTestCache) GetSessionModel(context.Context, int64, string, string, int64) (string, error) {
	return c.model, nil
}
func (c *sessionModelTestCache) SetSessionModel(_ context.Context, _ int64, _, _ string, _ int64, model string, _ time.Duration) error {
	c.model = model
	return nil
}

func TestSessionModelStabilityFiltersDifferentMapping(t *testing.T) {
	cache := &sessionModelTestCache{model: "gpt-stable"}
	svc := &OpenAIGatewayService{cache: cache}
	ctx := svc.PrepareStableModelContext(context.Background(), &Group{ID: 1, SessionModelStabilityEnabled: true, UpdatedAt: time.Unix(1, 0)}, "session", "gpt-client")
	require.True(t, accountMatchesStableUpstreamModel(ctx, &Account{Credentials: map[string]any{"model_mapping": map[string]any{"gpt-client": "gpt-stable"}}}, "gpt-client"))
	require.False(t, accountMatchesStableUpstreamModel(ctx, &Account{Credentials: map[string]any{"model_mapping": map[string]any{"gpt-client": "gpt-other"}}}, "gpt-client"))
}
