package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// deepSeekResponsesLiteBody 是 Codex 对 GPT 系模型名发出的 Responses Lite 形状：
// 顶层没有 tools，工具声明挂在 input[].additional_tools 上。
const deepSeekResponsesLiteBody = `{
  "model": "gpt-5.6-sol",
  "input": [
    {"type": "message", "role": "user", "content": "run echo hi"},
    {"type": "additional_tools", "role": "developer", "tools": [
      {"type": "namespace", "name": "functions", "tools": [
        {"type": "custom", "name": "exec"}
      ]}
    ]}
  ]
}`

// deepSeekClassicToolsBody 是 Codex 对上游原生模型名发出的传统形状：顶层 tools。
const deepSeekClassicToolsBody = `{
  "model": "deepseek-flash",
  "tools": [
    {"type": "function", "name": "exec_command", "parameters": {"type": "object"}}
  ],
  "input": [{"type": "message", "role": "user", "content": "run echo hi"}]
}`

func TestOpenAIRequestBodyHasAdditionalTools(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{name: "responses_lite_shape", body: deepSeekResponsesLiteBody, want: true},
		{name: "classic_top_level_tools", body: deepSeekClassicToolsBody, want: false},
		{name: "additional_tools_empty", body: `{"input":[{"type":"additional_tools","tools":[]}]}`, want: false},
		{name: "additional_tools_missing_tools", body: `{"input":[{"type":"additional_tools"}]}`, want: false},
		{name: "additional_tools_not_array", body: `{"input":[{"type":"additional_tools","tools":{}}]}`, want: false},
		{name: "input_not_array", body: `{"input":"nope"}`, want: false},
		{name: "empty_body", body: ``, want: false},
		{name: "no_input", body: `{"model":"gpt-5.6-sol"}`, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, openAIRequestBodyHasAdditionalTools([]byte(tt.body)))
		})
	}
}

func TestOpenAIRequestBodyHasToolsCoversAdditionalTools(t *testing.T) {
	// 回归：hasTools 既要认传统顶层 tools，也要认 Responses Lite 的
	// input[].additional_tools。DeepSeek 原生 /responses 只处理前者。
	require.True(t, openAIRequestBodyHasTools([]byte(deepSeekResponsesLiteBody)))
	require.True(t, openAIRequestBodyHasTools([]byte(deepSeekClassicToolsBody)))
	require.False(t, openAIRequestBodyHasTools([]byte(`{"input":[]}`)))
}

func TestShouldForwardDeepSeekResponsesLiteViaChatCompletions(t *testing.T) {
	// adaptive 协议：入站 responses 走原生 CN Responses 端点。
	nativeAdaptive := &Account{
		Platform:    PlatformDeepseek,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_protocol": APIProtocolAdaptive},
	}
	// 显式 responses 协议。
	nativeResponses := &Account{
		Platform:    PlatformDeepseek,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_protocol": APIProtocolResponses},
	}
	// 只走 chat completions 的账号本来就不会命中原生端点。
	chatOnly := &Account{
		Platform:    PlatformDeepseek,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_protocol": APIProtocolChatCompletions},
	}
	// 未配置 api_protocol 的旧账号：GetAPIProtocol 兜底 chat completions。
	unspecified := &Account{Platform: PlatformDeepseek, Type: AccountTypeAPIKey}
	// 非 deepseek 平台的原生 responses 账号不受影响。
	kimiNative := &Account{
		Platform:    PlatformKimi,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_protocol": APIProtocolAdaptive},
	}
	oauthDeepseek := &Account{Platform: PlatformDeepseek, Type: AccountTypeOAuth}

	// 「platform 填 openai、base_url 指向 DeepSeek」：把 GPT 模型名映射到
	// DeepSeek 时 Codex 的典型接入方式，platform 字段不代表真实上游。
	openaiDeepseekBase := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Extra:       map[string]any{"openai_responses_supported": true},
		Credentials: map[string]any{"base_url": "https://api.deepseek.com"},
	}
	openaiDeepseekBasePath := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Extra:       map[string]any{"openai_responses_supported": true},
		Credentials: map[string]any{"base_url": "https://api.deepseek.com/v1/"},
	}
	openaiOtherBase := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"base_url": "https://api.openai.com/v1"},
	}
	openaiNoBase := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	// 后缀伪造 api.deepseek.com 的域名不得命中（按完整 hostname 比较）。
	openaiLookalikeHost := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"base_url": "https://api.deepseek.com.evil.example"},
	}

	lite := []byte(deepSeekResponsesLiteBody)
	classic := []byte(deepSeekClassicToolsBody)

	tests := []struct {
		name    string
		account *Account
		body    []byte
		want    bool
	}{
		// 触发条件：DeepSeek 原生 Responses + Responses Lite 形状。
		{name: "adaptive_lite_forwards_to_chat", account: nativeAdaptive, body: lite, want: true},
		{name: "responses_lite_forwards_to_chat", account: nativeResponses, body: lite, want: true},
		// 传统顶层 tools：DeepSeek 原生端点能正确处理，保持原路径。
		{name: "adaptive_classic_stays_native", account: nativeAdaptive, body: classic, want: false},
		{name: "responses_classic_stays_native", account: nativeResponses, body: classic, want: false},
		// 非原生 Responses 的账号不额外改道。
		{name: "chat_only_untouched", account: chatOnly, body: lite, want: false},
		{name: "unspecified_protocol_untouched", account: unspecified, body: lite, want: false},
		// 其它平台/OAuth 账号不属于本判定的范围。
		{name: "other_platform_untouched", account: kimiNative, body: lite, want: false},
		{name: "oauth_untouched", account: oauthDeepseek, body: lite, want: false},
		{name: "nil_account", account: nil, body: lite, want: false},
		// 平台是 openai 但上游是 DeepSeek：同样命中。
		{name: "openai_platform_deepseek_upstream_lite", account: openaiDeepseekBase, body: lite, want: true},
		{name: "openai_platform_deepseek_upstream_with_path", account: openaiDeepseekBasePath, body: lite, want: true},
		{name: "openai_platform_deepseek_upstream_classic", account: openaiDeepseekBase, body: classic, want: false},
		{name: "openai_platform_other_upstream", account: openaiOtherBase, body: lite, want: false},
		{name: "openai_platform_no_base_url", account: openaiNoBase, body: lite, want: false},
		{name: "lookalike_host_not_matched", account: openaiLookalikeHost, body: lite, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, shouldForwardDeepSeekResponsesLiteViaChatCompletions(tt.account, tt.body))
		})
	}
}

func TestShouldForwardOpenAIResponsesViaChatCompletions(t *testing.T) {
	// 账号级配置要求回退时，无论请求形状如何都走 chat 桥。
	forcedAccount := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Extra:       map[string]any{"openai_responses_mode": "force_chat_completions"},
		Credentials: map[string]any{"api_protocol": APIProtocolAdaptive},
	}
	// 探测结论支持 Responses，且请求形状正常：保持原生路径。
	nativeAccount := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{"openai_responses_supported": true},
	}
	deepseekLite := &Account{
		Platform:    PlatformDeepseek,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_protocol": APIProtocolAdaptive},
	}
	// 平台 openai + DeepSeek 上游：不需要账号级开关也能自动改道。
	openaiDeepseekBase := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Extra:       map[string]any{"openai_responses_supported": true},
		Credentials: map[string]any{"base_url": "https://api.deepseek.com"},
	}

	lite := []byte(deepSeekResponsesLiteBody)
	classic := []byte(deepSeekClassicToolsBody)

	tests := []struct {
		name    string
		account *Account
		body    []byte
		want    bool
	}{
		{name: "account_config_forces_chat", account: forcedAccount, body: classic, want: true},
		{name: "account_config_forces_chat_lite", account: forcedAccount, body: lite, want: true},
		{name: "native_supported_stays", account: nativeAccount, body: lite, want: false},
		{name: "deepseek_lite_falls_back", account: deepseekLite, body: lite, want: true},
		{name: "deepseek_classic_stays", account: deepseekLite, body: classic, want: false},
		{name: "openai_platform_deepseek_upstream_lite", account: openaiDeepseekBase, body: lite, want: true},
		{name: "openai_platform_deepseek_upstream_classic", account: openaiDeepseekBase, body: classic, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, shouldForwardOpenAIResponsesViaChatCompletions(tt.account, tt.body))
		})
	}
}

func TestStripDeepSeekUnsupportedChatResponseFormat(t *testing.T) {
	deepseekViaOpenAI := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Extra:       map[string]any{"openai_responses_supported": true},
		Credentials: map[string]any{"base_url": "https://api.deepseek.com"},
	}
	deepseekNative := &Account{
		Platform:    PlatformDeepseek,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_protocol": APIProtocolAdaptive},
	}
	openAIBase := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"base_url": "https://api.openai.com/v1"},
	}

	jsonSchema := []byte(`{"model":"gpt-5.6-sol","response_format":{"type":"json_schema","json_schema":{"name":"x","schema":{"type":"object"}}}}`)
	jsonObject := []byte(`{"model":"gpt-5.6-sol","response_format":{"type":"json_object"}}`)
	textFormat := []byte(`{"model":"gpt-5.6-sol","response_format":{"type":"text"}}`)
	noFormat := []byte(`{"model":"gpt-5.6-sol"}`)

	tests := []struct {
		name     string
		account  *Account
		body     []byte
		wantGone bool
	}{
		{name: "deepseek_openai_platform_json_schema_stripped", account: deepseekViaOpenAI, body: jsonSchema, wantGone: true},
		{name: "deepseek_native_json_schema_stripped", account: deepseekNative, body: jsonSchema, wantGone: true},
		{name: "deepseek_json_object_kept", account: deepseekViaOpenAI, body: jsonObject, wantGone: false},
		{name: "deepseek_text_kept", account: deepseekViaOpenAI, body: textFormat, wantGone: false},
		{name: "deepseek_no_format_unchanged", account: deepseekViaOpenAI, body: noFormat, wantGone: false},
		{name: "openai_json_schema_kept", account: openAIBase, body: jsonSchema, wantGone: false},
		{name: "nil_account_kept", account: nil, body: jsonSchema, wantGone: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripDeepSeekUnsupportedChatResponseFormat(tt.account, tt.body)
			if tt.wantGone {
				require.False(t, gjson.GetBytes(got, "response_format").Exists(), "response_format should be removed, got %s", got)
			} else {
				// 非剔除路径必须原样返回（字节一致）。
				require.Equal(t, string(tt.body), string(got))
			}
		})
	}
}

func TestForwardNativeDeepSeekResponsesLiteFallsBackToChatCompletions(t *testing.T) {
	// fork 适配回归：原生 DeepSeek 账号经 forwardDeepSeekResponses 早返，
	// 到不了通用 Forward 的改道路由；Lite 形状必须在此改走 chat 桥，
	// 否则 additional_tools 被上游 /responses 静默忽略、工具退化为 DSML。
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"deepseek-chat",` +
				`"choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],` +
				`"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
		)),
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	c := deepSeekResponsesTestContext(t, []byte(deepSeekResponsesLiteBody))

	_, err := svc.Forward(context.Background(), c, deepSeekResponsesTestAccount(), []byte(deepSeekResponsesLiteBody))
	require.NoError(t, err)
	require.Contains(t, upstream.lastReq.URL.String(), "/chat/completions")
}
