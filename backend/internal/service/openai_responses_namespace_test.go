package service

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestShouldFlattenOpenAIResponsesNamespaces(t *testing.T) {
	oauth := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	apiKey := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	grokOAuth := &Account{Platform: PlatformGrok, Type: AccountTypeOAuth}
	// 账号级兼容开关：为不认识 namespace 的兼容上游恢复旧的摊平行为。
	flattenOAuth := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{"openai_responses_flatten_namespaces": true},
	}
	flattenAPIKey := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{"openai_responses_flatten_namespaces": true},
	}

	tests := []struct {
		name               string
		account            *Account
		transport          OpenAIUpstreamTransport
		passthroughEnabled bool
		compactPath        bool
		want               bool
	}{
		// 默认保留：OAuth 出口是 namespace 扩展的定义方，摊平会让模型无法按
		// `to=functions.<namespace>.<tool>` 寻址（issue #4978）。
		{name: "oauth_http_default_preserves", account: oauth, transport: OpenAIUpstreamTransportHTTPSSE, want: false},
		{name: "oauth_http_passthrough_default_preserves", account: oauth, transport: OpenAIUpstreamTransportHTTPSSE, passthroughEnabled: true, want: false},
		{name: "oauth_wsv2_default_preserves", account: oauth, transport: OpenAIUpstreamTransportResponsesWebsocketV2, want: false},
		// compact 端点 schema 更窄且无实测证据，保持既有摊平行为。
		{name: "oauth_compact_flattens", account: oauth, transport: OpenAIUpstreamTransportHTTPSSE, compactPath: true, want: true},
		{name: "oauth_compact_wsv2_preserves", account: oauth, transport: OpenAIUpstreamTransportResponsesWebsocketV2, compactPath: true, want: false},
		{name: "apikey_compact", account: apiKey, transport: OpenAIUpstreamTransportHTTPSSE, compactPath: true, want: false},
		{name: "oauth_flatten_enabled_http", account: flattenOAuth, transport: OpenAIUpstreamTransportHTTPSSE, want: true},
		{name: "oauth_flatten_enabled_http_passthrough", account: flattenOAuth, transport: OpenAIUpstreamTransportHTTPSSE, passthroughEnabled: true, want: true},
		// WSv2 出口原样转发上游事件、不做回程还原，摊平会让客户端收到无法匹配的平名。
		{name: "oauth_flatten_enabled_wsv2", account: flattenOAuth, transport: OpenAIUpstreamTransportResponsesWebsocketV2, want: false},
		// 透传账号先于 WSv2 分支经 HTTP 转发返回，开关打开时仍需摊平。
		{name: "oauth_flatten_enabled_wsv2_passthrough", account: flattenOAuth, transport: OpenAIUpstreamTransportResponsesWebsocketV2, passthroughEnabled: true, want: true},
		// 开关仅对 OAuth 生效：API Key 走 chat completions 回退桥时由桥自行摊平。
		{name: "apikey_flatten_enabled_http", account: flattenAPIKey, transport: OpenAIUpstreamTransportHTTPSSE, want: false},
		{name: "apikey_http", account: apiKey, transport: OpenAIUpstreamTransportHTTPSSE, want: false},
		{name: "grok_oauth_http", account: grokOAuth, transport: OpenAIUpstreamTransportHTTPSSE, want: false},
		{name: "nil_account", account: nil, transport: OpenAIUpstreamTransportHTTPSSE, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, shouldFlattenOpenAIResponsesNamespaces(
				tt.account, tt.transport, tt.passthroughEnabled, tt.compactPath,
			))
		})
	}
}

func TestShouldKeepOpenAIResponsesToolCallNamespaces(t *testing.T) {
	oauth := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	apiKey := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	setupToken := &Account{Platform: PlatformOpenAI, Type: AccountTypeSetupToken}
	flattenOAuth := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{"openai_responses_flatten_namespaces": true},
	}

	tests := []struct {
		name               string
		account            *Account
		transport          OpenAIUpstreamTransport
		passthroughEnabled bool
		compactPath        bool
		want               bool
	}{
		// 上游按 namespace 解析历史调用，缺字段会 400 "Missing namespace for function_call"。
		{name: "oauth_http_keeps", account: oauth, transport: OpenAIUpstreamTransportHTTPSSE, want: true},
		{name: "oauth_http_passthrough_keeps", account: oauth, transport: OpenAIUpstreamTransportHTTPSSE, passthroughEnabled: true, want: true},
		// compact 端点 schema 不含该字段，携带即 400 "Unknown parameter: input[N].namespace"。
		{name: "oauth_compact_strips", account: oauth, transport: OpenAIUpstreamTransportHTTPSSE, compactPath: true, want: false},
		// 摊平后调用项已是平名，残留 namespace 指向的声明不存在。
		{name: "oauth_flatten_enabled_strips", account: flattenOAuth, transport: OpenAIUpstreamTransportHTTPSSE, want: false},
		// WSv2 实际由 shouldStrip 提前短路，此处只钉住策略本身的取值。
		{name: "oauth_wsv2_keeps", account: oauth, transport: OpenAIUpstreamTransportResponsesWebsocketV2, want: true},
		// WSv2 + compact 是唯一「不摊平但仍必须清理」的组合，钉住 compact 判定本身，
		// 使其不会被误当成可由 shouldFlatten 推导出的冗余分支。
		{name: "oauth_compact_wsv2_strips", account: oauth, transport: OpenAIUpstreamTransportResponsesWebsocketV2, compactPath: true, want: false},
		// API Key 出口是标准 Responses API，不认识该字段。
		{name: "apikey_strips", account: apiKey, transport: OpenAIUpstreamTransportHTTPSSE, want: false},
		{name: "setup_token_strips", account: setupToken, transport: OpenAIUpstreamTransportHTTPSSE, want: false},
		{name: "nil_account", account: nil, transport: OpenAIUpstreamTransportHTTPSSE, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, shouldKeepOpenAIResponsesToolCallNamespaces(
				tt.account, tt.transport, tt.passthroughEnabled, tt.compactPath,
			))
		})
	}
}

func TestShouldStripOpenAIResponsesInputNamespaces(t *testing.T) {
	oauth := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	apiKey := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	setupToken := &Account{Platform: PlatformOpenAI, Type: AccountTypeSetupToken}
	grokOAuth := &Account{Platform: PlatformGrok, Type: AccountTypeOAuth}

	tests := []struct {
		name               string
		account            *Account
		transport          OpenAIUpstreamTransport
		passthroughEnabled bool
		want               bool
	}{
		{name: "oauth_http", account: oauth, transport: OpenAIUpstreamTransportHTTPSSE, want: true},
		{name: "apikey_http", account: apiKey, transport: OpenAIUpstreamTransportHTTPSSE, want: true},
		{name: "oauth_wsv2", account: oauth, transport: OpenAIUpstreamTransportResponsesWebsocketV2, want: false},
		{name: "apikey_wsv2", account: apiKey, transport: OpenAIUpstreamTransportResponsesWebsocketV2, want: false},
		{name: "oauth_wsv2_passthrough", account: oauth, transport: OpenAIUpstreamTransportResponsesWebsocketV2, passthroughEnabled: true, want: true},
		{name: "apikey_wsv2_passthrough", account: apiKey, transport: OpenAIUpstreamTransportResponsesWebsocketV2, passthroughEnabled: true, want: true},
		{name: "setup_token_http", account: setupToken, transport: OpenAIUpstreamTransportHTTPSSE, want: false},
		{name: "grok_oauth_http", account: grokOAuth, transport: OpenAIUpstreamTransportHTTPSSE, want: false},
		{name: "nil_account", account: nil, transport: OpenAIUpstreamTransportHTTPSSE, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, shouldStripOpenAIResponsesInputNamespaces(tt.account, tt.transport, tt.passthroughEnabled))
		})
	}
}

func TestStripOpenAIResponsesInputNamespaces(t *testing.T) {
	body := []byte(`{
		"meta":9007199254740993,
		"scientific":1.25e+42,
		"escaped":"line\\n\\u003ctag\\u003e",
		"tools":[{"type":"function","name":"keep","namespace":"tool-namespace"}],
		"input":[
			{"type":"function_call","namespace":"n0","name":"one","content":{"namespace":"nested"},"large":9007199254740993},
			{"type":"message","namespace":"n1","content":[{"type":"input_text","text":"hello","namespace":"nested-content"}]},
			{"type":"custom_tool_call","namespace":"n2","input":"{}"},
			{"type":"function_call_output","namespace":"n3","output":"ok"},
			{"type":"item","namespace":"n4"},
			{"type":"item","namespace":"n5"},
			{"type":"item","namespace":"n6"},
			{"type":"item","namespace":"n7"}
		]
	}`)

	stripped, err := stripOpenAIResponsesInputNamespaces(body, false)
	require.NoError(t, err)
	for index := 0; index < 8; index++ {
		require.False(t, gjson.GetBytes(stripped, "input."+strconv.Itoa(index)+".namespace").Exists())
	}
	require.Equal(t, "nested", gjson.GetBytes(stripped, "input.0.content.namespace").String())
	require.Equal(t, "nested-content", gjson.GetBytes(stripped, "input.1.content.0.namespace").String())
	require.Equal(t, "tool-namespace", gjson.GetBytes(stripped, "tools.0.namespace").String())
	require.Equal(t, gjson.GetBytes(body, "meta").Raw, gjson.GetBytes(stripped, "meta").Raw)
	require.Equal(t, gjson.GetBytes(body, "scientific").Raw, gjson.GetBytes(stripped, "scientific").Raw)
	require.Equal(t, gjson.GetBytes(body, "escaped").Raw, gjson.GetBytes(stripped, "escaped").Raw)
	require.Equal(t, gjson.GetBytes(body, "input.0.large").Raw, gjson.GetBytes(stripped, "input.0.large").Raw)
}

func TestStripOpenAIResponsesInputNamespacesLeavesOtherShapesByteExact(t *testing.T) {
	tests := [][]byte{
		[]byte(`{"input":"text","namespace":"top-level"}`),
		[]byte(`{"input":{"namespace":"single-object"}}`),
		[]byte(`{"input":[{"content":{"namespace":"nested-only"}}],"tools":[{"namespace":"keep"}]}`),
	}
	for _, body := range tests {
		for _, keepToolCallNamespaces := range []bool{false, true} {
			stripped, err := stripOpenAIResponsesInputNamespaces(body, keepToolCallNamespaces)
			require.NoError(t, err)
			require.Equal(t, body, stripped)
		}
	}
}

// 保留模式下只有工具调用项留住 namespace：上游按 namespace 解析历史调用，
// 而 message / reasoning / 输出项带该字段会被 schema 拒绝。
func TestStripOpenAIResponsesInputNamespacesKeepsToolCallNamespaces(t *testing.T) {
	body := []byte(`{
		"meta":9007199254740993,
		"input":[
			{"type":"function_call","namespace":"collaboration","name":"spawn_agent","arguments":"{}","large":9007199254740993},
			{"type":"custom_tool_call","namespace":"codex_app","name":"exec","input":"{}"},
			{"type":"tool_call","namespace":"mcp__codex_apps__gmail","name":"send"},
			{"type":"mcp_tool_call","namespace":"mcp__codex_apps__gmail","name":"list"},
			{"type":"message","namespace":"leftover","role":"assistant","content":[{"type":"output_text","text":"hi"}]},
			{"type":"function_call_output","namespace":"leftover","output":"ok"},
			{"type":"reasoning","namespace":"leftover"},
			{"type":"item","namespace":"leftover"}
		]
	}`)

	stripped, err := stripOpenAIResponsesInputNamespaces(body, true)
	require.NoError(t, err)

	require.Equal(t, "collaboration", gjson.GetBytes(stripped, "input.0.namespace").String())
	require.Equal(t, "codex_app", gjson.GetBytes(stripped, "input.1.namespace").String())
	require.Equal(t, "mcp__codex_apps__gmail", gjson.GetBytes(stripped, "input.2.namespace").String())
	require.Equal(t, "mcp__codex_apps__gmail", gjson.GetBytes(stripped, "input.3.namespace").String())
	for index := 4; index < 8; index++ {
		require.False(t, gjson.GetBytes(stripped, "input."+strconv.Itoa(index)+".namespace").Exists())
	}
	// 大整数不得经 float64 往返。
	require.Equal(t, gjson.GetBytes(body, "meta").Raw, gjson.GetBytes(stripped, "meta").Raw)
	require.Equal(t, gjson.GetBytes(body, "input.0.large").Raw, gjson.GetBytes(stripped, "input.0.large").Raw)

	// 类型比对不区分大小写与首尾空白。
	mixedCase := []byte(`{"input":[{"type":" Function_Call ","namespace":"collaboration","name":"spawn_agent"}]}`)
	keptMixedCase, err := stripOpenAIResponsesInputNamespaces(mixedCase, true)
	require.NoError(t, err)
	require.Equal(t, mixedCase, keptMixedCase)

	// 全部为调用项时无改动，应原样返回。
	callsOnly := []byte(`{"input":[{"type":"function_call","namespace":"collaboration","name":"spawn_agent"}]}`)
	unchanged, err := stripOpenAIResponsesInputNamespaces(callsOnly, true)
	require.NoError(t, err)
	require.Equal(t, callsOnly, unchanged)

	// 关闭保留时回到全量清理。
	strippedAll, err := stripOpenAIResponsesInputNamespaces(body, false)
	require.NoError(t, err)
	for index := 0; index < 8; index++ {
		require.False(t, gjson.GetBytes(strippedAll, "input."+strconv.Itoa(index)+".namespace").Exists())
	}
}
