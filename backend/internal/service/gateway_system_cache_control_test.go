package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// 客户端打在 system 上的 cache_control 是它自己的缓存意图，网关不该替它删掉。
//
// 这些用例守的是 #2369 里被漏掉的那一半：messages 层的同类改写已经收进
// rewrite_message_cache_control 开关（默认不动客户端断点），system 层却一直没有
// 对应的保护——mimic 路径无条件剥离，且不打日志、不报错。
//
// 剥离在两种情形下都没有成立的前提：注入开启时 system 已被整个替换、客户端断点
// 早就不在了；注入关闭时 system 是客户端原样，断点就是它的意图。

// 多块 system 各自带断点时，一块都不能少：Anthropic 允许最多 4 个，
// 真正的上限兜底是 enforceCacheControlLimit 的事，不该在这里提前砍。
func TestNormalizeClaudeOAuthRequestBody_KeepsEveryClientSystemBreakpoint(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4-6","system":[` +
		`{"type":"text","text":"block one","cache_control":{"type":"ephemeral","ttl":"5m"}},` +
		`{"type":"text","text":"block two"},` +
		`{"type":"text","text":"block three","cache_control":{"type":"ephemeral","ttl":"1h"}}` +
		`],"messages":[{"role":"user","content":"hi"}]}`)

	out, _ := normalizeClaudeOAuthRequestBody(body, "claude-sonnet-4-6", claudeOAuthNormalizeOptions{})

	require.Equal(t, "5m", gjson.GetBytes(out, "system.0.cache_control.ttl").String(),
		"客户端选的 TTL 要原样留着")
	require.False(t, gjson.GetBytes(out, "system.1.cache_control").Exists(), "没有的不该凭空长出来")
	require.Equal(t, "1h", gjson.GetBytes(out, "system.2.cache_control.ttl").String())
}

// 保留断点不等于放弃 system 文本的规范化：OpenCode 身份句该改的还得改。
// 这条用例区分「拿掉剥离」与「把整段 normalize 关掉」——后者会让第三方指纹漏上去。
func TestNormalizeClaudeOAuthRequestBody_StillSanitizesTextWhileKeepingBreakpoint(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4-6","system":[{"type":"text","text":"You are OpenCode, the best coding agent on the planet.","cache_control":{"type":"ephemeral"}}],"messages":[{"role":"user","content":"hi"}]}`)

	out, _ := normalizeClaudeOAuthRequestBody(body, "claude-sonnet-4-6", claudeOAuthNormalizeOptions{})

	require.Equal(t, strings.TrimSpace(claudeCodeSystemPrompt), gjson.GetBytes(out, "system.0.text").String())
	require.True(t, gjson.GetBytes(out, "system.0.cache_control").Exists(),
		"文本被规范化，断点仍要留着")
}

// 注入开启这条路径上，system 是我们自己拼的；blocks 配置里自带的 cache_control
// 是稳定缓存锚点（对齐真实 CLI 的形态），同样不能在 normalize 里掉。
func TestNormalizeClaudeOAuthRequestBody_KeepsInjectedSystemBreakpoint(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4-6","system":"client instructions","messages":[{"role":"user","content":"hi"}]}`)
	rewritten := rewriteSystemForNonClaudeCode(body, "client instructions")

	out, _ := normalizeClaudeOAuthRequestBody(rewritten, "claude-sonnet-4-6", claudeOAuthNormalizeOptions{})

	// 只断言「注入后的 system 仍带着锚点」，不写死是第几块：
	// 块数与布局由 blocks 配置决定，焊进断言会让配置一改就误报。
	_, _, _, systemPaths := collectCacheControlPaths(out)
	require.NotEmpty(t, systemPaths, "注入路径自带的稳定锚点不该在 normalize 里掉")
}

// count_tokens 是五条出口里唯一没有在自己转发路径上调过 enforceCacheControlLimit 的
// 那条。不再剥离客户端 system 断点之后，客户端自带的断点会和 tools[-1] 上注入的那个
// 叠加，可以直接顶破 4 块上限——而上游对超限是 400。
//
// 用 setup-token 账号构造 mimic 分支：IsOAuth 认它，取 token 只读 credentials，
// 不碰 DB。UA 非 claude-cli 且无 metadata.user_id，于是 shouldMimicClaudeCode 成立。
func TestForwardCountTokens_EnforcesCacheControlLimitOnMimicPath(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", nil)
	c.Request.Header.Set("User-Agent", "third-party-client/1.0")

	// 客户端自己打满 4 个断点：system 一个、messages 三个。
	// 加上 mimic 注入的 tools[-1] 与 system blocks 自带的锚点，必然超过 4。
	body := []byte(`{"model":"claude-sonnet-4-6",` +
		`"system":[{"type":"text","text":"stable client prefix","cache_control":{"type":"ephemeral","ttl":"5m"}}],` +
		`"tools":[{"name":"probe","description":"d","input_schema":{"type":"object"}}],` +
		`"messages":[` +
		`{"role":"user","content":[{"type":"text","text":"one","cache_control":{"type":"ephemeral"}}]},` +
		`{"role":"assistant","content":[{"type":"text","text":"two","cache_control":{"type":"ephemeral"}}]},` +
		`{"role":"user","content":[{"type":"text","text":"three","cache_control":{"type":"ephemeral"}}]}` +
		`]}`)
	parsed := &ParsedRequest{Body: NewRequestBodyRef(body), Model: "claude-sonnet-4-6"}

	upstream := &anthropicHTTPUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"input_tokens":42}`)),
		},
	}
	svc := &GatewayService{
		cfg:              &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
		httpUpstream:     upstream,
		rateLimitService: &RateLimitService{},
	}
	account := &Account{
		ID:          401,
		Name:        "count-tokens-ceiling-test",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeSetupToken,
		Concurrency: 1,
		Credentials: map[string]any{"access_token": "test-oauth-token"},
		Status:      StatusActive,
		Schedulable: true,
	}

	require.NoError(t, svc.ForwardCountTokens(context.Background(), c, account, parsed))

	_, messagePaths, toolPaths, systemPaths := collectCacheControlPaths(upstream.lastBody)
	total := len(messagePaths) + len(toolPaths) + len(systemPaths)
	require.LessOrEqual(t, total, maxCacheControlBlocks,
		"出站 body 的 cache_control 块数必须被砍到上限内，实测 %d 块", total)
}
