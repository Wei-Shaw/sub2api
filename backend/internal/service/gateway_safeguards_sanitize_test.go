//go:build unit

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Claude Code Auto mode 服务端检查：body.safeguards 与 anthropic-beta
// dangerous-tool-use-2026-09-03 成对。最终 beta 不含该值时上游 400，Claude Code
// 会在整段对话里拒绝 Auto mode 工具调用；剥除字段后上游不返回 safeguard_results，
// 客户端平稳回退本地分类。策略同 fallbacks：剥字段，不注入 beta。

const safeguardsTestBody = `{"model":"claude-sonnet-5","safeguards":{"opaque":{"action_id":"toolu_1"}},"messages":[]}`

func TestSanitizeAnthropicBodyForBetaTokens_SafeguardsRequireDangerousToolUseBeta(t *testing.T) {
	out, changed := sanitizeAnthropicBodyForBetaTokens([]byte(safeguardsTestBody),
		"oauth-2025-04-20,"+claude.BetaDangerousToolUse)
	require.False(t, changed)
	require.Equal(t, "toolu_1", gjson.GetBytes(out, "safeguards.opaque.action_id").String())

	for _, beta := range []string{"", "oauth-2025-04-20,interleaved-thinking-2025-05-14"} {
		out, changed = sanitizeAnthropicBodyForBetaTokens([]byte(safeguardsTestBody), beta)
		require.Truef(t, changed, "beta %q", beta)
		require.False(t, gjson.GetBytes(out, "safeguards").Exists())
		require.True(t, gjson.GetBytes(out, "messages").Exists())
	}
}

func TestBuildUpstreamRequest_OAuthMimic_StripsSafeguardsWithoutInjectingBeta(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("Anthropic-Beta", "claude-code-20250219,"+claude.BetaDangerousToolUse)

	account := &Account{ID: 602, Platform: PlatformAnthropic, Type: AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "oauth-tok"},
		Status:      StatusActive, Schedulable: true,
	}
	svc := &GatewayService{cfg: &config.Config{}}
	req, _, err := svc.buildUpstreamRequest(
		context.Background(), c, account, []byte(safeguardsTestBody),
		"oauth-tok", "oauth", "claude-sonnet-5", false, true,
	)
	require.NoError(t, err)

	require.False(t, gjson.GetBytes(readUpstreamBodyForTest(t, req), "safeguards").Exists(),
		"mimic beta 集合不含 dangerous-tool-use → safeguards 必须剥除，否则上游 400")
	require.False(t, anthropicBetaTokensContains(getHeaderRaw(req.Header, "anthropic-beta"), claude.BetaDangerousToolUse))
}

func TestBuildUpstreamRequestAnthropicAPIKeyPassthrough_Safeguards(t *testing.T) {
	for name, tc := range map[string]struct {
		beta string
		keep bool
	}{
		"client beta has dangerous-tool-use": {beta: "oauth-2025-04-20," + claude.BetaDangerousToolUse, keep: true},
		"client beta missing":                {beta: "oauth-2025-04-20", keep: false},
	} {
		t.Run(name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			c.Request.Header.Set("Anthropic-Beta", tc.beta)

			svc := &GatewayService{cfg: &config.Config{}}
			req, _, err := svc.buildUpstreamRequestAnthropicAPIKeyPassthrough(
				context.Background(), c, newAnthropicAPIKeyPassthroughAccountForBetaTest(), []byte(safeguardsTestBody), "token",
			)
			require.NoError(t, err)
			require.Equal(t, tc.keep, gjson.GetBytes(readUpstreamBodyForTest(t, req), "safeguards").Exists())
		})
	}
}

func TestPrepareBedrockRequestBodyWithTokens_StripsSafeguards(t *testing.T) {
	input := `{"messages":[{"role":"user","content":"hi"}],"max_tokens":100,"safeguards":{"opaque":{}}}`
	result, err := PrepareBedrockRequestBodyWithTokens(
		[]byte(input), "us.anthropic.claude-opus-4-7-v1", []string{claude.BetaDangerousToolUse}, false,
	)
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(result, "safeguards").Exists(),
		"Bedrock beta 白名单不含 dangerous-tool-use → safeguards 总会剥除")
	require.Equal(t, "hi", gjson.GetBytes(result, "messages.0.content").String())
}

func TestBuildCountTokensRequestAnthropicAPIKeyPassthrough_ForwardsFeatureHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", nil)
	c.Request.Header.Set("Anthropic-Future-Feature", "opaque")
	c.Request.Header.Set("X-Claude-Code-Request-Class", "main")
	c.Request.Header.Set("X-Api-Key", "inbound-key")
	c.Request.Header.Set("Cookie", "secret=1")

	svc := &GatewayService{cfg: &config.Config{}}
	req, err := svc.buildCountTokensRequestAnthropicAPIKeyPassthrough(
		context.Background(), c, newAnthropicAPIKeyPassthroughAccountForBetaTest(),
		[]byte(`{"model":"claude-sonnet-5","messages":[]}`), "upstream-key",
	)
	require.NoError(t, err)
	require.Equal(t, "opaque", req.Header.Get("Anthropic-Future-Feature"))
	require.Equal(t, "main", req.Header.Get("X-Claude-Code-Request-Class"))
	require.Equal(t, "upstream-key", req.Header.Get("X-Api-Key"))
	require.Empty(t, req.Header.Get("Cookie"))
}
