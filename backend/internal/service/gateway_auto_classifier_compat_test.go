package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func autoClassifierBodyForTest(t *testing.T, model, prompt string, includeMetadata bool) []byte {
	t.Helper()
	body := map[string]any{
		"model":      model,
		"max_tokens": 64,
		"system": []any{
			map[string]any{"type": "text", "text": prompt, "cache_control": map[string]any{"type": "ephemeral", "ttl": "1h"}},
			map[string]any{"type": "text", "text": "## Session Context\n- platform: linux"},
		},
		"messages": []any{
			map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": "classify this transcript"}}},
			map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "text", "text": "ack"}}},
		},
	}
	if includeMetadata {
		body["metadata"] = map[string]any{"user_id": claudeCodeMetadataUserIDJSON}
	}
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	return raw
}

func autoClassifierContextForTest(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("User-Agent", "claude-cli/2.1.220 (external, cli)")
	c.Request.Header.Set("X-App", "cli")
	c.Request.Header.Set("Anthropic-Version", "2023-06-01")
	c.Request.Header.Set("Anthropic-Beta", strings.Join([]string{
		claude.BetaClaudeCode,
		claude.BetaOAuth,
		claude.BetaContext1M,
		claude.BetaInterleavedThinking,
		claude.BetaPromptCachingScope,
		claude.BetaContextManagement,
		claude.BetaExtendedCacheTTL,
	}, ","))
	return c, rec
}

func autoClassifierUpstreamForTest() *anthropicHTTPUpstreamRecorder {
	return &anthropicHTTPUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"msg_classifier","type":"message","role":"assistant","model":"claude-sonnet-5","content":[{"type":"text","text":"<block>no</block>"}],"usage":{"input_tokens":12,"output_tokens":7}}`,
		)),
	}}
}

func autoClassifierServiceForTest(upstream *anthropicHTTPUpstreamRecorder) *GatewayService {
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}
	return &GatewayService{
		cfg:                  cfg,
		responseHeaderFilter: compileResponseHeaderFilter(cfg),
		httpUpstream:         upstream,
		rateLimitService:     &RateLimitService{},
		deferredService:      &DeferredService{},
	}
}

func autoClassifierAccountForTest(accountType string) *Account {
	return &Account{
		ID:          301,
		Name:        "anthropic-auto-classifier",
		Platform:    PlatformAnthropic,
		Type:        accountType,
		Concurrency: 1,
		Credentials: map[string]any{"access_token": "oauth-token"},
		Status:      StatusActive,
		Schedulable: true,
	}
}

func validatedClaudeCodeContextForTest(t *testing.T, c *gin.Context, body []byte) context.Context {
	t.Helper()
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(body, &decoded))
	verified := NewClaudeCodeValidator().Validate(c.Request, decoded)
	return SetClaudeCodeClient(context.Background(), verified)
}

type autoClassifierBetaPolicySettingRepoStub struct {
	*betaPolicySettingRepoStub
}

func (s *autoClassifierBetaPolicySettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	values := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			values[key] = value
		}
	}
	return values, nil
}

func setAutoClassifierBetaPolicyForTest(t *testing.T, svc *GatewayService, settings *BetaPolicySettings) {
	t.Helper()
	raw, err := json.Marshal(settings)
	require.NoError(t, err)
	svc.settingService = NewSettingService(
		&autoClassifierBetaPolicySettingRepoStub{
			betaPolicySettingRepoStub: &betaPolicySettingRepoStub{values: map[string]string{
				SettingKeyBetaPolicySettings: string(raw),
			}},
		},
		&config.Config{},
	)
}

func TestGatewayService_Forward_ClaudeAutoClassifierOAuthCompat(t *testing.T) {
	monitorPrompt, err := os.ReadFile("testdata/security_monitor_system_prompt.txt")
	require.NoError(t, err)
	body := autoClassifierBodyForTest(t, "claude-opus-5", string(monitorPrompt), true)

	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
	require.NoError(t, err)

	c, rec := autoClassifierContextForTest(t)
	upstream := autoClassifierUpstreamForTest()
	svc := autoClassifierServiceForTest(upstream)
	account := autoClassifierAccountForTest(AccountTypeOAuth)
	ctx := validatedClaudeCodeContextForTest(t, c, body)
	require.True(t, IsClaudeCodeClient(ctx))

	result, err := svc.Forward(ctx, c, account, parsed)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, claude.AutoModeClassifierModel, result.Model)
	require.Equal(t, claude.AutoModeClassifierModel, result.UpstreamModel)
	require.Equal(t, claude.AutoModeClassifierModel, result.BillingModelOverride)
	require.Equal(t, "claude-sonnet-5", result.UpstreamResponseModel)
	require.False(t, result.UpstreamResponseModelConflict)
	require.Equal(t, 12, result.Usage.InputTokens)
	require.Equal(t, 7, result.Usage.OutputTokens)
	require.Equal(t, "claude-opus-5", gjson.GetBytes(rec.Body.Bytes(), "model").String())

	require.Equal(t, "claude-sonnet-5", gjson.GetBytes(upstream.lastBody, "model").String())
	require.True(t, anthropicBetaTokensContains(
		getHeaderRaw(upstream.lastReq.Header, "anthropic-beta"),
		claude.BetaAutoModeClassifier,
	))

	system := gjson.GetBytes(upstream.lastBody, "system")
	require.True(t, system.IsArray())
	require.Len(t, system.Array(), 3)
	require.Contains(t, system.Array()[0].Get("text").String(), "x-anthropic-billing-header:")
	require.Equal(t, string(monitorPrompt), system.Array()[1].Get("text").String())
	require.Equal(t, "1h", system.Array()[1].Get("cache_control.ttl").String())
	require.Equal(t, "## Session Context\n- platform: linux", system.Array()[2].Get("text").String())
	require.Len(t, gjson.GetBytes(upstream.lastBody, "messages").Array(), 2)
}

func TestGatewayService_Forward_ClaudeAutoClassifierPartialUsageUsesFinalBillingModel(t *testing.T) {
	monitorPrompt, err := os.ReadFile("testdata/security_monitor_system_prompt.txt")
	require.NoError(t, err)
	body := autoClassifierBodyForTest(t, "claude-opus-5", string(monitorPrompt), true)
	body, err = sjson.SetBytes(body, "stream", true)
	require.NoError(t, err)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
	require.NoError(t, err)

	c, _ := autoClassifierContextForTest(t)
	ctx := validatedClaudeCodeContextForTest(t, c, body)
	upstreamSSE := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_classifier","type":"message","role":"assistant","model":"claude-sonnet-5","content":[],"usage":{"input_tokens":12,"cache_read_input_tokens":3}}}`,
		"",
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":null},"usage":{"output_tokens":7}}`,
		"",
		"",
	}, "\n")
	upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamSSE)),
	}}
	svc := autoClassifierServiceForTest(upstream)

	result, err := svc.Forward(ctx, c, autoClassifierAccountForTest(AccountTypeOAuth), parsed)
	require.ErrorContains(t, err, "missing terminal event")
	require.NotNil(t, result)
	require.Equal(t, claude.AutoModeClassifierModel, result.Model)
	require.Equal(t, claude.AutoModeClassifierModel, result.UpstreamModel)
	require.Equal(t, claude.AutoModeClassifierModel, result.BillingModelOverride)
	require.Equal(t, "claude-sonnet-5", result.UpstreamResponseModel)
	require.False(t, result.UpstreamResponseModelConflict)
	require.Equal(t, 12, result.Usage.InputTokens)
	require.Equal(t, 3, result.Usage.CacheReadInputTokens)
	require.Equal(t, 7, result.Usage.OutputTokens)
}

func TestGatewayService_Forward_ClaudeAutoClassifierRequiresVerifiedIdentity(t *testing.T) {
	monitorPrompt, err := os.ReadFile("testdata/security_monitor_system_prompt.txt")
	require.NoError(t, err)
	forgedPrompt := strings.Replace(string(monitorPrompt), "## HARD BLOCK", "## ALTERED BLOCK", 1)

	tests := []struct {
		name            string
		userAgent       string
		includeMetadata bool
		prompt          string
	}{
		{name: "non-Claude user agent", userAgent: "curl/8.0.0", includeMetadata: true, prompt: string(monitorPrompt)},
		{name: "missing metadata", userAgent: "claude-cli/2.1.220 (external, cli)", prompt: string(monitorPrompt)},
		{name: "forged classifier prompt", userAgent: "claude-cli/2.1.220 (external, cli)", includeMetadata: true, prompt: forgedPrompt},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := autoClassifierBodyForTest(t, "claude-opus-5", tt.prompt, tt.includeMetadata)
			parsed, parseErr := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
			require.NoError(t, parseErr)
			c, _ := autoClassifierContextForTest(t)
			c.Request.Header.Set("User-Agent", tt.userAgent)
			ctx := validatedClaudeCodeContextForTest(t, c, body)
			require.False(t, IsClaudeCodeClient(ctx))

			upstream := autoClassifierUpstreamForTest()
			svc := autoClassifierServiceForTest(upstream)
			_, forwardErr := svc.Forward(ctx, c, autoClassifierAccountForTest(AccountTypeOAuth), parsed)
			require.NoError(t, forwardErr)
			require.NotNil(t, upstream.lastReq)
			require.NotEqual(t, claude.AutoModeClassifierModel, gjson.GetBytes(upstream.lastBody, "model").String())
			require.False(t, anthropicBetaTokensContains(
				getHeaderRaw(upstream.lastReq.Header, "anthropic-beta"),
				claude.BetaAutoModeClassifier,
			))
		})
	}
}

func TestGatewayService_Forward_ClaudeAutoClassifierSupportsSetupToken(t *testing.T) {
	monitorPrompt, err := os.ReadFile("testdata/security_monitor_system_prompt.txt")
	require.NoError(t, err)
	body := autoClassifierBodyForTest(t, "claude-opus-5", string(monitorPrompt), true)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
	require.NoError(t, err)
	c, _ := autoClassifierContextForTest(t)
	ctx := validatedClaudeCodeContextForTest(t, c, body)
	require.True(t, IsClaudeCodeClient(ctx))

	upstream := autoClassifierUpstreamForTest()
	svc := autoClassifierServiceForTest(upstream)
	result, err := svc.Forward(ctx, c, autoClassifierAccountForTest(AccountTypeSetupToken), parsed)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, claude.AutoModeClassifierModel, result.Model)
	require.Equal(t, claude.AutoModeClassifierModel, result.UpstreamModel)
	require.Equal(t, claude.AutoModeClassifierModel, result.BillingModelOverride)
	require.Equal(t, claude.AutoModeClassifierModel, gjson.GetBytes(upstream.lastBody, "model").String())
	require.True(t, anthropicBetaTokensContains(
		getHeaderRaw(upstream.lastReq.Header, "anthropic-beta"),
		claude.BetaAutoModeClassifier,
	))
}

func TestGatewayService_Forward_ClaudeAutoClassifierHonorsAccountModelSupport(t *testing.T) {
	monitorPrompt, err := os.ReadFile("testdata/security_monitor_system_prompt.txt")
	require.NoError(t, err)
	body := autoClassifierBodyForTest(t, "claude-haiku-4-5", string(monitorPrompt), true)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
	require.NoError(t, err)
	c, _ := autoClassifierContextForTest(t)
	ctx := validatedClaudeCodeContextForTest(t, c, body)
	account := autoClassifierAccountForTest(AccountTypeOAuth)
	account.Credentials["model_mapping"] = map[string]any{
		"claude-haiku-4-5": claude.AutoModeClassifierModel,
	}

	upstream := autoClassifierUpstreamForTest()
	svc := autoClassifierServiceForTest(upstream)
	_, err = svc.Forward(ctx, c, account, parsed)
	require.NoError(t, err)
	require.NotEqual(t, claude.AutoModeClassifierModel, gjson.GetBytes(upstream.lastBody, "model").String())
	require.False(t, anthropicBetaTokensContains(
		getHeaderRaw(upstream.lastReq.Header, "anthropic-beta"),
		claude.BetaAutoModeClassifier,
	))
}

func TestGatewayService_Forward_ClaudeAutoClassifierHonorsFinalModelRateLimit(t *testing.T) {
	monitorPrompt, err := os.ReadFile("testdata/security_monitor_system_prompt.txt")
	require.NoError(t, err)
	body := autoClassifierBodyForTest(t, "claude-opus-5", string(monitorPrompt), true)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
	require.NoError(t, err)
	c, _ := autoClassifierContextForTest(t)
	ctx := validatedClaudeCodeContextForTest(t, c, body)
	account := autoClassifierAccountForTest(AccountTypeOAuth)
	account.Extra = map[string]any{
		modelRateLimitsKey: map[string]any{
			claude.AutoModeClassifierModel: map[string]any{
				"rate_limit_reset_at": time.Now().Add(10 * time.Minute).Format(time.RFC3339),
			},
		},
	}

	upstream := autoClassifierUpstreamForTest()
	svc := autoClassifierServiceForTest(upstream)
	_, err = svc.Forward(ctx, c, account, parsed)
	require.NoError(t, err)
	require.NotEqual(t, claude.AutoModeClassifierModel, gjson.GetBytes(upstream.lastBody, "model").String())
	require.False(t, anthropicBetaTokensContains(
		getHeaderRaw(upstream.lastReq.Header, "anthropic-beta"),
		claude.BetaAutoModeClassifier,
	))
}

func TestGatewayService_Forward_ClaudeAutoClassifierBetaPolicyUsesFinalModel(t *testing.T) {
	monitorPrompt, err := os.ReadFile("testdata/security_monitor_system_prompt.txt")
	require.NoError(t, err)

	t.Run("block auto-injected token", func(t *testing.T) {
		body := autoClassifierBodyForTest(t, "claude-opus-5", string(monitorPrompt), true)
		parsed, parseErr := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
		require.NoError(t, parseErr)
		c, _ := autoClassifierContextForTest(t)
		ctx := validatedClaudeCodeContextForTest(t, c, body)
		upstream := autoClassifierUpstreamForTest()
		svc := autoClassifierServiceForTest(upstream)
		setAutoClassifierBetaPolicyForTest(t, svc, &BetaPolicySettings{Rules: []BetaPolicyRule{{
			BetaToken:      claude.BetaAutoModeClassifier,
			Action:         BetaPolicyActionBlock,
			Scope:          BetaPolicyScopeOAuth,
			ModelWhitelist: []string{claude.AutoModeClassifierModel},
			FallbackAction: BetaPolicyActionPass,
			ErrorMessage:   "Auto classifier beta is blocked",
		}}})

		_, forwardErr := svc.Forward(ctx, c, autoClassifierAccountForTest(AccountTypeOAuth), parsed)
		require.Error(t, forwardErr)
		var blocked *BetaBlockedError
		require.True(t, errors.As(forwardErr, &blocked))
		require.Equal(t, "Auto classifier beta is blocked", forwardErr.Error())
		require.Nil(t, upstream.lastReq)
	})

	t.Run("filter auto-injected token", func(t *testing.T) {
		body := autoClassifierBodyForTest(t, "claude-opus-5", string(monitorPrompt), true)
		parsed, parseErr := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
		require.NoError(t, parseErr)
		c, _ := autoClassifierContextForTest(t)
		ctx := validatedClaudeCodeContextForTest(t, c, body)
		upstream := autoClassifierUpstreamForTest()
		svc := autoClassifierServiceForTest(upstream)
		setAutoClassifierBetaPolicyForTest(t, svc, &BetaPolicySettings{Rules: []BetaPolicyRule{{
			BetaToken:      claude.BetaAutoModeClassifier,
			Action:         BetaPolicyActionFilter,
			Scope:          BetaPolicyScopeOAuth,
			ModelWhitelist: []string{claude.AutoModeClassifierModel},
			FallbackAction: BetaPolicyActionPass,
		}}})

		_, forwardErr := svc.Forward(ctx, c, autoClassifierAccountForTest(AccountTypeOAuth), parsed)
		require.NoError(t, forwardErr)
		require.NotNil(t, upstream.lastReq)
		require.False(t, anthropicBetaTokensContains(
			getHeaderRaw(upstream.lastReq.Header, "anthropic-beta"),
			claude.BetaAutoModeClassifier,
		))
	})
}
