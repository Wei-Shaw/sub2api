package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGatewayService_Forward_ClaudeAutoClassifierOAuthCompat(t *testing.T) {
	gin.SetMode(gin.TestMode)

	monitorPrompt, err := os.ReadFile("testdata/security_monitor_system_prompt.txt")
	require.NoError(t, err)

	originalMessages := []any{
		map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": "classify this transcript"}}},
		map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "text", "text": "ack"}}},
	}
	body, err := json.Marshal(map[string]any{
		"model":      "claude-opus-5",
		"max_tokens": 64,
		"system": []any{
			map[string]any{"type": "text", "text": string(monitorPrompt), "cache_control": map[string]any{"type": "ephemeral", "ttl": "1h"}},
			map[string]any{"type": "text", "text": "## Session Context\n- platform: linux"},
		},
		"messages": originalMessages,
		"metadata": map[string]any{"user_id": claudeCodeMetadataUserIDJSON},
	})
	require.NoError(t, err)

	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
	require.NoError(t, err)

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

	upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"msg_classifier","type":"message","role":"assistant","model":"claude-sonnet-5","content":[{"type":"text","text":"<block>no</block>"}],"usage":{"input_tokens":12,"output_tokens":7}}`,
		)),
	}}
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}
	svc := &GatewayService{
		cfg:                  cfg,
		responseHeaderFilter: compileResponseHeaderFilter(cfg),
		httpUpstream:         upstream,
		rateLimitService:     &RateLimitService{},
		deferredService:      &DeferredService{},
	}
	account := &Account{
		ID:          301,
		Name:        "anthropic-oauth-auto-classifier",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{"access_token": "oauth-token"},
		Status:      StatusActive,
		Schedulable: true,
	}

	result, err := svc.Forward(context.Background(), c, account, parsed)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, claude.AutoModeClassifierModel, result.Model)
	require.Equal(t, claude.AutoModeClassifierModel, result.UpstreamModel)
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
	require.Len(t, gjson.GetBytes(upstream.lastBody, "messages").Array(), len(originalMessages))
}
