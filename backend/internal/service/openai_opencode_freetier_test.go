package service

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// openCodeGateTestAccount 构造 OpenCode 平台账号，credentials 可覆盖门禁模式。
func openCodeGateTestAccount(credentials map[string]any) *Account {
	creds := map[string]any{"api_key": "sk-test"}
	for k, v := range credentials {
		creds[k] = v
	}
	return &Account{
		ID:          1,
		Platform:    PlatformOpenCodeGo,
		Type:        AccountTypeAPIKey,
		Credentials: creds,
	}
}

func TestCanonicalOpenCodeIDShape(t *testing.T) {
	got := canonicalOpenCodeID("ses", "conversation-1")

	require.Len(t, got, 30, "规范 ID 长度必须恒为 30")
	require.True(t, strings.HasPrefix(got, "ses_"), "必须以 ses_ 开头")

	body := strings.TrimPrefix(got, "ses_")
	require.Len(t, body, 26)

	hexPart, suffix := body[:12], body[12:]
	require.Regexp(t, `^[0-9a-f]{12}$`, hexPart, "前 12 位必须是小写十六进制")
	require.Regexp(t, `^[0-9A-Za-z]{14}$`, suffix, "后 14 位必须是字母数字")

	// 必须与上游实际接受的正则一致。
	require.True(t, openCodeCanonicalSessionRe.MatchString(got))
}

func TestCanonicalOpenCodeIDDeterministicAndDistinct(t *testing.T) {
	require.Equal(t,
		canonicalOpenCodeID("ses", "same-source"),
		canonicalOpenCodeID("ses", "same-source"),
		"同一来源必须恒等映射，否则上游 prompt cache 永不命中",
	)
	require.NotEqual(t,
		canonicalOpenCodeID("ses", "source-a"),
		canonicalOpenCodeID("ses", "source-b"),
	)
	require.NotEqual(t,
		canonicalOpenCodeID("ses", "x"),
		canonicalOpenCodeID("msg", "x"),
		"前缀必须参与结果",
	)
}

func TestIsOpenCodeFreeTierModel(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{"mimo-v2.6-flash-free", true},
		{"deepseek-v4-flash-free", true},
		{"muse-spark-1.3-contributor-free", true},
		{"ling-3.0-flash-fin-free", true},
		{"nemotron-3-ultra-free", true},
		{"MIMO-V2.6-FLASH-FREE", true},
		{"opencode/mimo-v2.6-flash-free", true},
		{"opencode-go/mimo-v2.6-flash-free", true},
		{"  mimo-v2.6-flash-free  ", true},
		// 付费模型，含与免费模型同前缀者，绝不能误判。
		{"deepseek-v4-flash", false},
		{"mimo-v2.5", false},
		{"muse-spark-1.3-contributor", false},
		{"glm-5.3", false},
		{"", false},
		{"free", false},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			require.Equal(t, tt.want, isOpenCodeFreeTierModel(tt.model))
		})
	}
}

func TestShouldApplyOpenCodeFreeTierGate(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		model   string
		want    bool
	}{
		{
			name:    "缺省 auto 对免费模型启用",
			account: openCodeGateTestAccount(nil),
			model:   "mimo-v2.6-flash-free",
			want:    true,
		},
		{
			name:    "缺省 auto 对付费模型不启用",
			account: openCodeGateTestAccount(nil),
			model:   "deepseek-v4-flash",
			want:    false,
		},
		{
			name:    "auto 显式配置",
			account: openCodeGateTestAccount(map[string]any{"free_tier_gate": "auto"}),
			model:   "mimo-v2.6-flash-free",
			want:    true,
		},
		{
			name:    "always 对付费模型也启用",
			account: openCodeGateTestAccount(map[string]any{"free_tier_gate": "always"}),
			model:   "glm-5.3",
			want:    true,
		},
		{
			name:    "off 对免费模型也关闭",
			account: openCodeGateTestAccount(map[string]any{"free_tier_gate": "off"}),
			model:   "mimo-v2.6-flash-free",
			want:    false,
		},
		{
			name:    "非法取值回落 auto",
			account: openCodeGateTestAccount(map[string]any{"free_tier_gate": "sometimes"}),
			model:   "mimo-v2.6-flash-free",
			want:    true,
		},
		{
			name:    "非 OpenCode 平台不启用",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
			model:   "mimo-v2.6-flash-free",
			want:    false,
		},
		{
			name:    "空账号不启用",
			account: nil,
			model:   "mimo-v2.6-flash-free",
			want:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, shouldApplyOpenCodeFreeTierGate(tt.account, tt.model))
		})
	}
}

// requireGateTools 断言 tools 中同时存在 bash 与 read，且各自只出现一次。
func requireGateTools(t *testing.T, body []byte, namePath string) {
	t.Helper()
	names := map[string]int{}
	for _, item := range gjson.GetBytes(body, "tools").Array() {
		if name := strings.TrimSpace(item.Get(namePath).String()); name != "" {
			names[name]++
		}
	}
	require.Equal(t, 1, names["bash"], "bash 必须恰好存在一次")
	require.Equal(t, 1, names["read"], "read 必须恰好存在一次")
}

func TestApplyOpenCodeFreeTierGateBodyChatDialect(t *testing.T) {
	account := openCodeGateTestAccount(nil)
	body := []byte(`{"model":"mimo-v2.6-flash-free","stream":false,"messages":[{"role":"user","content":"hi"}]}`)

	out, err := applyOpenCodeFreeTierGateBody(account, body, openCodeGateDialectChat)
	require.NoError(t, err)

	require.True(t, gjson.GetBytes(out, "stream").Bool(), "stream 必须被强制为 true")
	requireGateTools(t, out, "function.name")

	// chat 方言必须是 function 嵌套形状。
	first := gjson.GetBytes(out, "tools.0")
	require.Equal(t, "function", first.Get("type").String())
	require.True(t, first.Get("function.parameters").Exists())

	// 原有 messages 不得被改动。
	require.Equal(t, "hi", gjson.GetBytes(out, "messages.0.content").String())
}

func TestApplyOpenCodeFreeTierGateBodyResponsesDialect(t *testing.T) {
	account := openCodeGateTestAccount(nil)
	body := []byte(`{"model":"muse-spark-1.3-contributor-free","stream":true,"input":[{"role":"user","content":"hi"}]}`)

	out, err := applyOpenCodeFreeTierGateBody(account, body, openCodeGateDialectResponses)
	require.NoError(t, err)

	requireGateTools(t, out, "name")
	// responses 方言是扁平形状，不得出现 function 包装。
	first := gjson.GetBytes(out, "tools.0")
	require.Equal(t, "function", first.Get("type").String())
	require.False(t, first.Get("function").Exists(), "responses 方言不得使用 function 包装")
	require.True(t, first.Get("parameters").Exists())
}

func TestApplyOpenCodeFreeTierGateBodyAnthropicDialect(t *testing.T) {
	account := openCodeGateTestAccount(nil)
	body := []byte(`{"model":"mimo-v2.6-flash-free","stream":true,"max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`)

	out, err := applyOpenCodeFreeTierGateBody(account, body, openCodeGateDialectAnthropic)
	require.NoError(t, err)

	requireGateTools(t, out, "name")
	// Anthropic 自定义工具参数键为 input_schema，无 type 包装。
	first := gjson.GetBytes(out, "tools.0")
	require.True(t, first.Get("input_schema").Exists(), "Anthropic 方言必须使用 input_schema")
	require.False(t, first.Get("parameters").Exists())
	require.False(t, first.Get("type").Exists())
}

func TestApplyOpenCodeFreeTierGateBodyOnlyFillsMissingTool(t *testing.T) {
	account := openCodeGateTestAccount(nil)
	body := []byte(`{"model":"mimo-v2.6-flash-free","stream":true,"messages":[],"tools":[{"type":"function","function":{"name":"bash","description":"mine","parameters":{"type":"object","properties":{"cmd":{"type":"string"}}}}}]}`)

	out, err := applyOpenCodeFreeTierGateBody(account, body, openCodeGateDialectChat)
	require.NoError(t, err)

	requireGateTools(t, out, "function.name")
	// 已有的 bash 定义必须原样保留，不能被覆盖成最小 schema。
	require.Equal(t, "mine", gjson.GetBytes(out, "tools.0.function.description").String())
	require.True(t, gjson.GetBytes(out, "tools.0.function.parameters.properties.cmd").Exists())
	// 客户端自带的其他工具不得丢失。
	require.Equal(t, 2, len(gjson.GetBytes(out, "tools").Array()))
}

func TestApplyOpenCodeFreeTierGateBodyKeepsClientTools(t *testing.T) {
	account := openCodeGateTestAccount(nil)
	body := []byte(`{"model":"mimo-v2.6-flash-free","stream":true,"messages":[],"tools":[{"type":"function","function":{"name":"bash","parameters":{"type":"object"}}},{"type":"function","function":{"name":"read","parameters":{"type":"object"}}},{"type":"function","function":{"name":"my_tool","parameters":{"type":"object"}}}]}`)

	out, err := applyOpenCodeFreeTierGateBody(account, body, openCodeGateDialectChat)
	require.NoError(t, err)

	requireGateTools(t, out, "function.name")
	require.Equal(t, 3, len(gjson.GetBytes(out, "tools").Array()), "不得重复注入或丢弃客户端工具")
}

func TestApplyOpenCodeFreeTierGateBodyPaidModelUntouched(t *testing.T) {
	account := openCodeGateTestAccount(nil)
	original := `{"model":"deepseek-v4-flash","stream":false,"messages":[]}`

	out, err := applyOpenCodeFreeTierGateBody(account, []byte(original), openCodeGateDialectChat)
	require.NoError(t, err)
	require.Equal(t, original, string(out), "auto 模式下付费模型的出站 body 必须逐字节不变")
}

func TestApplyOpenCodeFreeTierGateBodyOffModeUntouched(t *testing.T) {
	account := openCodeGateTestAccount(map[string]any{"free_tier_gate": "off"})
	original := `{"model":"mimo-v2.6-flash-free","stream":false,"messages":[]}`

	out, err := applyOpenCodeFreeTierGateBody(account, []byte(original), openCodeGateDialectChat)
	require.NoError(t, err)
	require.Equal(t, original, string(out))
}

func TestApplyOpenCodeFreeTierGateBodyMalformedIsPassthrough(t *testing.T) {
	account := openCodeGateTestAccount(nil)

	for _, tt := range []struct {
		name string
		body string
	}{
		{"非 JSON", `not json at all`},
		{"截断 JSON", `{"model":"mimo-v2.6-flash-free","stream":`},
		{"空 body", ``},
	} {
		t.Run(tt.name, func(t *testing.T) {
			out, err := applyOpenCodeFreeTierGateBody(account, []byte(tt.body), openCodeGateDialectChat)
			require.NoError(t, err, "畸形 body 不得让请求在网关侧失败")
			require.Equal(t, tt.body, string(out))
		})
	}
}

func TestApplyOpenCodeFreeTierGateBodyToolsNotArrayIsPassthrough(t *testing.T) {
	account := openCodeGateTestAccount(nil)
	body := []byte(`{"model":"mimo-v2.6-flash-free","stream":true,"messages":[],"tools":"weird"}`)

	out, err := applyOpenCodeFreeTierGateBody(account, body, openCodeGateDialectChat)
	require.NoError(t, err)
	// stream 仍被强制，但 tools 形状未知时不得被改写。
	require.True(t, gjson.GetBytes(out, "stream").Bool())
	require.Equal(t, "weird", gjson.GetBytes(out, "tools").String())
}

func TestApplyOpenCodeFreeTierGateSession(t *testing.T) {
	canonical := "ses_0123456789abABCDEFGHIJKLMN"

	t.Run("客户端已提供规范值时原样保留", func(t *testing.T) {
		account := openCodeGateTestAccount(nil)
		headers := http.Header{}
		headers.Set(openCodeSessionHeader, canonical)
		body := []byte(`{"model":"mimo-v2.6-flash-free"}`)

		applyOpenCodeFreeTierGateSession(account, headers, body)

		require.Equal(t, canonical, headers.Get(openCodeSessionHeader))
	})

	t.Run("UUID 被改写为规范形状", func(t *testing.T) {
		account := openCodeGateTestAccount(nil)
		headers := http.Header{}
		headers.Set(openCodeSessionHeader, "3f2504e0-4f89-11d3-9a0c-0305e82c3301")
		body := []byte(`{"model":"mimo-v2.6-flash-free"}`)

		applyOpenCodeFreeTierGateSession(account, headers, body)

		got := headers.Get(openCodeSessionHeader)
		require.True(t, openCodeCanonicalSessionRe.MatchString(got), "改写结果必须匹配上游形状，实际 %q", got)
		require.NotEqual(t, "3f2504e0-4f89-11d3-9a0c-0305e82c3301", got)
	})

	t.Run("同一来源跨轮次稳定", func(t *testing.T) {
		account := openCodeGateTestAccount(nil)
		body := []byte(`{"model":"mimo-v2.6-flash-free"}`)

		first := http.Header{}
		first.Set(openCodeSessionHeader, "same-uuid-value")
		applyOpenCodeFreeTierGateSession(account, first, body)

		second := http.Header{}
		second.Set(openCodeSessionHeader, "same-uuid-value")
		applyOpenCodeFreeTierGateSession(account, second, body)

		require.Equal(t, first.Get(openCodeSessionHeader), second.Get(openCodeSessionHeader))
	})

	t.Run("付费模型不改写", func(t *testing.T) {
		account := openCodeGateTestAccount(nil)
		headers := http.Header{}
		headers.Set(openCodeSessionHeader, "3f2504e0-4f89-11d3-9a0c-0305e82c3301")
		body := []byte(`{"model":"deepseek-v4-flash"}`)

		applyOpenCodeFreeTierGateSession(account, headers, body)

		require.Equal(t, "3f2504e0-4f89-11d3-9a0c-0305e82c3301", headers.Get(openCodeSessionHeader))
	})

	t.Run("无会话头时自行补出规范值", func(t *testing.T) {
		// 回归 Zen 缺口：shouldGenerateOpenCodeSession 要求路径含 /zen/go，
		// Zen 账号（/zen/v1）不满足，故 applyOpenCodeSessionHeader 不会补会话头。
		// 门禁必须自行补，否则出站请求无会话头，免费模型必然 403。
		account := openCodeGateTestAccount(nil)
		headers := http.Header{}
		body := []byte(`{"model":"mimo-v2.6-flash-free","messages":[{"role":"user","content":"hi"}]}`)

		applyOpenCodeFreeTierGateSession(account, headers, body)

		got := headers.Get(openCodeSessionHeader)
		require.True(t, openCodeCanonicalSessionRe.MatchString(got), "必须补出规范会话值，实际 %q", got)
	})

	t.Run("无会话头时按内容派生且跨轮次稳定", func(t *testing.T) {
		account := openCodeGateTestAccount(nil)
		first := http.Header{}
		firstTurn := []byte(`{"model":"mimo-v2.6-flash-free","messages":[{"role":"user","content":"hello"}]}`)
		applyOpenCodeFreeTierGateSession(account, first, firstTurn)

		second := http.Header{}
		// 后续轮次首条用户消息不变，仅追加历史：会话值必须保持一致。
		laterTurn := []byte(`{"model":"mimo-v2.6-flash-free","messages":[{"role":"user","content":"hello"},{"role":"assistant","content":"hi"},{"role":"user","content":"more"}]}`)
		applyOpenCodeFreeTierGateSession(account, second, laterTurn)

		require.Equal(t, first.Get(openCodeSessionHeader), second.Get(openCodeSessionHeader),
			"同一会话跨轮次必须稳定，否则 prompt cache 永不命中")
	})

	t.Run("付费模型无会话头时不补", func(t *testing.T) {
		account := openCodeGateTestAccount(nil)
		headers := http.Header{}
		body := []byte(`{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi"}]}`)

		applyOpenCodeFreeTierGateSession(account, headers, body)

		require.Empty(t, headers.Get(openCodeSessionHeader), "auto 模式下付费模型不得被注入会话头")
	})
}

func TestNormalizeOpenCodeFreeTierGateCredentials(t *testing.T) {
	require.NoError(t, NormalizeOpenCodeFreeTierGateCredentials(nil))
	require.NoError(t, NormalizeOpenCodeFreeTierGateCredentials(map[string]any{}))

	// 未携带该字段时必须是 no-op，使旧账号继续按 auto 处理。
	creds := map[string]any{"api_key": "sk-1"}
	require.NoError(t, NormalizeOpenCodeFreeTierGateCredentials(creds))
	require.NotContains(t, creds, "free_tier_gate")

	for _, mode := range []string{"auto", "always", "off", " AUTO ", "Off"} {
		creds := map[string]any{"free_tier_gate": mode}
		require.NoError(t, NormalizeOpenCodeFreeTierGateCredentials(creds), mode)
		normalized, ok := creds["free_tier_gate"].(string)
		require.True(t, ok)
		require.Contains(t, []string{"auto", "always", "off"}, normalized)
	}

	for _, bad := range []any{"sometimes", "", 1, true, []any{"auto"}} {
		creds := map[string]any{"free_tier_gate": bad}
		err := NormalizeOpenCodeFreeTierGateCredentials(creds)
		require.Error(t, err, "非法取值 %v 必须被拒绝", bad)
	}
}

func TestDefaultOpenCodeZenFreeModelIDsRouting(t *testing.T) {
	// 免费层 ID 必须出现在平台目录回退中，否则 /v1/models 与白名单预填看不到它们。
	catalog := DefaultOpenCodeGoModelIDs()
	for _, id := range DefaultOpenCodeZenFreeModelIDs() {
		require.Contains(t, catalog, id)
	}

	// muse-spark-*-free 必须命中 muse-spark-* → Responses 规则。
	require.Equal(t, APIProtocolResponses, OpenCodeGoModelProtocol("muse-spark-1.3-contributor-free"))
	// 其余免费模型走 Chat Completions。
	require.Equal(t, APIProtocolChatCompletions, OpenCodeGoModelProtocol("mimo-v2.6-flash-free"))
	require.Equal(t, APIProtocolChatCompletions, OpenCodeGoModelProtocol("deepseek-v4-flash-free"))
}

// TestOpenCodeFreeTierGateEndToEndBodyShape 把四项硬要求一起断言，作为门禁的
// 验收测试：会话形状、UA 版本下限、bash+read、stream:true。
func TestOpenCodeFreeTierGateEndToEndBodyShape(t *testing.T) {
	account := openCodeGateTestAccount(nil)
	headers := http.Header{}
	headers.Set(openCodeSessionHeader, "3f2504e0-4f89-11d3-9a0c-0305e82c3301")
	body := []byte(`{"model":"mimo-v2.6-flash-free","stream":false,"messages":[{"role":"user","content":"hi"}]}`)

	out, err := applyOpenCodeFreeTierGateBody(account, body, openCodeGateDialectChat)
	require.NoError(t, err)
	applyOpenCodeFreeTierGateSession(account, headers, body)

	// 1. 会话形状
	require.True(t, openCodeCanonicalSessionRe.MatchString(headers.Get(openCodeSessionHeader)))
	// 2. UA 版本下限
	version := strings.TrimPrefix(strings.Fields(openCodeUpstreamUserAgent)[0], "opencode/")
	require.True(t, version >= "1.18.0", "UA 版本 %s 必须不低于 1.18.0", version)
	// 3. bash + read
	requireGateTools(t, out, "function.name")
	// 4. stream:true
	require.True(t, gjson.GetBytes(out, "stream").Bool())

	// body 必须是合法 JSON（门禁改写不得破坏载荷）。
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(out, &decoded))
}
