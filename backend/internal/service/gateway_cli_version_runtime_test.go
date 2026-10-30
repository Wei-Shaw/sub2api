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
	"github.com/tidwall/sjson"
)

// withCLIVersionResolverForTest 注入解析器并在用例结束时还原（置 nil），避免用例间污染。
func withCLIVersionResolverForTest(t *testing.T, resolver func() string) {
	t.Helper()
	claude.SetCLIVersionResolver(resolver)
	t.Cleanup(func() { claude.SetCLIVersionResolver(nil) })
}

// 一致性铁律（d）：模拟一次 OAuth + mimic 的请求构建（messages 与 count_tokens 两条
// 出站路径），断言出站 User-Agent 头里的版本号与请求体 x-anthropic-billing-header
// 中 cc_version 的版本号相同，且都等于运行期注入的版本号。头/体不一致会被 Anthropic
// 判为非正版客户端。
func TestBuildOAuthMimicRequest_RuntimeVersionConsistentBetweenHeaderAndBilling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const upgraded = "9.9.9"
	withCLIVersionResolverForTest(t, func() string { return upgraded })

	for _, endpoint := range []string{"messages", "count_tokens"} {
		t.Run(endpoint, func(t *testing.T) {
			resetGatewayForwardingSettingsCacheForTest(t)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			body := []byte(`{"model":"claude-haiku-4-5","system":[{"type":"text","text":""}],"messages":[{"role":"user","content":"hello world"}]}`)
			billing, err := buildBillingAttributionText(body, "2.1.81")
			require.NoError(t, err)
			body, err = sjson.SetBytes(body, "system.0.text", billing)
			require.NoError(t, err)

			svc := &GatewayService{cfg: &config.Config{}}
			account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeOAuth}

			var req *http.Request
			var wireBody []byte
			if endpoint == "messages" {
				req, wireBody, err = svc.buildUpstreamRequest(context.Background(), c, account,
					body, "test-token", "oauth", "claude-haiku-4-5", false, true)
			} else {
				req, wireBody, err = svc.buildCountTokensRequest(context.Background(), c, account,
					body, "test-token", "oauth", "claude-haiku-4-5", true)
			}
			require.NoError(t, err)

			wantUA := "claude-cli/" + upgraded + " (external, cli)"
			outboundUA := getHeaderRaw(req.Header, "User-Agent")
			require.Equal(t, wantUA, outboundUA)

			billingText := gjson.GetBytes(wireBody, "system.0.text").String()
			require.Contains(t, billingText, "x-anthropic-billing-header")
			require.Contains(t, billingText, "cc_version="+upgraded+"."+computeClaudeCodeFingerprint(wireBody, upgraded)+";")
			require.NotContains(t, billingText, "cc_version=2.1.81")

			// 头/体版本号必须来自同一字符串。
			headerVersion := ExtractCLIVersion(outboundUA)
			require.Equal(t, upgraded, headerVersion)
			require.Contains(t, billingText, "cc_version="+headerVersion+".")
		})
	}
}

// e. 注入更高版本后，floorClaudeCLIUserAgentVersion 把存量低版本指纹抬升到新版本；
//
//	等于或高于新版本的指纹保持不动（只升不降）。
func TestFloorClaudeCLIUserAgentVersion_UsesRuntimeVersion(t *testing.T) {
	const upgraded = "9.9.9"
	withCLIVersionResolverForTest(t, func() string { return upgraded })

	floored, changed := floorClaudeCLIUserAgentVersion("claude-cli/2.1.100 (external, cli)")
	require.True(t, changed)
	require.Equal(t, "claude-cli/"+upgraded+" (external, cli)", floored)

	// 等于运行期版本：不动。
	same, changed := floorClaudeCLIUserAgentVersion("claude-cli/" + upgraded + " (external, cli)")
	require.False(t, changed)
	require.Equal(t, "claude-cli/"+upgraded+" (external, cli)", same)

	// 高于运行期版本：不动。
	greater, changed := floorClaudeCLIUserAgentVersion("claude-cli/99.0.0 (external, cli)")
	require.False(t, changed)
	require.Equal(t, "claude-cli/99.0.0 (external, cli)", greater)

	// resolver 返回非法值时回退内置基线，floor 语义仍然成立。
	withCLIVersionResolverForTest(t, func() string { return "abc" })
	baselineFloored, changed := floorClaudeCLIUserAgentVersion("claude-cli/1.0.0 (external, cli)")
	require.True(t, changed)
	require.Equal(t, "claude-cli/"+claude.CLIVersion()+" (external, cli)", baselineFloored)
}

// 注入更高版本后，isAcceptableFingerprintUserAgent 以运行期版本为基准，允许客户端
// 上报的新版本。
func TestIsAcceptableFingerprintUserAgent_UsesRuntimeVersion(t *testing.T) {
	const upgraded = "9.9.9"
	withCLIVersionResolverForTest(t, func() string { return upgraded })
	require.True(t, isAcceptableFingerprintUserAgent("claude-cli/"+upgraded+" (external, cli)"))
	// 允许的最大主版本超前量是 +2（9.9.9 → 11.x）。
	require.True(t, isAcceptableFingerprintUserAgent("claude-cli/11.0.0 (external, cli)"))
	// 超前 +3 会被拒（与静态基线下的既有语义一致，只是基准换成运行期版本）。
	require.False(t, isAcceptableFingerprintUserAgent("claude-cli/12.0.0 (external, cli)"))
}

// defaultFingerprint 现取运行期版本，不再在 init 固化。
func TestDefaultFingerprintUsesRuntimeVersion(t *testing.T) {
	const upgraded = "9.9.9"
	withCLIVersionResolverForTest(t, func() string { return upgraded })
	require.Equal(t, "claude-cli/"+upgraded+" (external, cli)", defaultFingerprint().UserAgent)
}
