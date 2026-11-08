//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// ollamaMaxTokensCapTestAccount 构造带自定义 cap 的 Ollama Cloud usage 账号。
func ollamaMaxTokensCapTestAccount(id int64, cap any) *Account {
	account := ollamaUsageAccount(id)
	account.Extra[OllamaCloudMaxTokensCapExtraKey] = cap
	return account
}

func TestOllamaCloudMaxTokensClamp(t *testing.T) {
	ollama := ollamaUsageAccount(101)

	tests := []struct {
		name    string
		account *Account
		body    string
		want    string
		raw     bool // want 非法 JSON 时按原始字节比较
	}{
		{
			name:    "max_tokens above default cap is clamped",
			account: ollama,
			body:    `{"model":"gpt-oss:120b-cloud","max_tokens":70000}`,
			want:    `{"model":"gpt-oss:120b-cloud","max_tokens":65535}`,
		},
		{
			name:    "max_completion_tokens above default cap is clamped",
			account: ollama,
			body:    `{"model":"gpt-oss:120b-cloud","max_completion_tokens":131072}`,
			want:    `{"model":"gpt-oss:120b-cloud","max_completion_tokens":65535}`,
		},
		{
			name:    "both fields above cap are clamped",
			account: ollama,
			body:    `{"model":"m","max_tokens":80000,"max_completion_tokens":90000}`,
			want:    `{"model":"m","max_tokens":65535,"max_completion_tokens":65535}`,
		},
		{
			name:    "values at or below default cap are kept",
			account: ollama,
			body:    `{"model":"m","max_tokens":65535,"max_completion_tokens":4096}`,
			want:    `{"model":"m","max_tokens":65535,"max_completion_tokens":4096}`,
		},
		{
			name:    "custom extra cap is applied",
			account: ollamaMaxTokensCapTestAccount(102, 32768),
			body:    `{"model":"m","max_tokens":50000}`,
			want:    `{"model":"m","max_tokens":32768}`,
		},
		{
			name:    "extra cap zero disables clamping",
			account: ollamaMaxTokensCapTestAccount(103, 0),
			body:    `{"model":"m","max_tokens":50000}`,
			want:    `{"model":"m","max_tokens":50000}`,
		},
		{
			name:    "non-numeric extra cap falls back to default",
			account: ollamaMaxTokensCapTestAccount(104, "abc"),
			body:    `{"model":"m","max_tokens":100000}`,
			want:    `{"model":"m","max_tokens":65535}`,
		},
		{
			name:    "invalid json is left untouched",
			account: ollama,
			body:    `{"model":"m","max_tokens":`,
			want:    `{"model":"m","max_tokens":`,
			raw:     true,
		},
		{
			name:    "non-integer max_tokens is left untouched",
			account: ollama,
			body:    `{"model":"m","max_tokens":1.5}`,
			want:    `{"model":"m","max_tokens":1.5}`,
		},
		{
			name:    "missing max_tokens is left untouched",
			account: ollama,
			body:    `{"model":"m"}`,
			want:    `{"model":"m"}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := clampOllamaCloudMaxTokens(test.account, []byte(test.body))
			if test.raw {
				require.Equal(t, test.want, string(got))
				return
			}
			require.JSONEq(t, test.want, string(got))
		})
	}
}

func TestOllamaCloudMaxTokensCap(t *testing.T) {
	require.Equal(t, int64(65535), ollamaCloudMaxTokensCap(nil))
	require.Equal(t, int64(65535), ollamaCloudMaxTokensCap(ollamaUsageAccount(201)))

	tests := []struct {
		name string
		cap  any
		want int64
	}{
		{"float64", float64(32768), 32768},
		{"int", 40000, 40000},
		{"int64", int64(50000), 50000},
		{"json.Number", json.Number("60000"), 60000},
		{"json.Number invalid", json.Number("abc"), 65535},
		{"zero disables", 0, 0},
		{"negative disables", int64(-1), -1},
		{"string falls back", "abc", 65535},
		{"bool falls back", true, 65535},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			account := ollamaMaxTokensCapTestAccount(202, test.cap)
			require.Equal(t, test.want, ollamaCloudMaxTokensCap(account))
		})
	}
}

// TestApplyOllamaCloudRawChatCompletionsRequestClampsMaxTokens 验证 reasoning 钩子
// 与 token clamp 已解耦：reasoning 钩子只做 reasoning 归一化，clamp 由独立 token
// 钩子 clampOllamaCloudUpstreamMaxTokens 在出站时接续执行（两条 raw CC 出站路径
// 均依次调用两者，端到端行为不变）。
func TestApplyOllamaCloudRawChatCompletionsRequestClampsMaxTokens(t *testing.T) {
	body := []byte(`{"model":"deepseek-chat","max_tokens":100000}`)

	// Ollama Cloud 账号：reasoning 钩子不再 clamp，字节级原样。
	ollama := ollamaCloudRawChatCompletionsTestAccount()
	require.Equal(t, string(body), string(applyOllamaCloudRawChatCompletionsRequest(ollama, body)))
	// 独立 token 钩子接续 clamp 到既有默认 cap。
	require.JSONEq(t, `{"model":"deepseek-chat","max_tokens":65535}`,
		string(clampOllamaCloudUpstreamMaxTokens(ollama, body)))

	// 官方 DeepSeek（api.deepseek.com + force_chat_completions）→ 字节级不变。
	official := rawChatCompletionsTestAccount()
	official.Credentials["base_url"] = "https://api.deepseek.com"
	official.Extra = map[string]any{
		openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions),
	}
	require.Equal(t, body, applyOllamaCloudRawChatCompletionsRequest(official, body))
	require.Equal(t, string(body), string(clampOllamaCloudUpstreamMaxTokens(official, body)))

	// ollama.com 但无 force_chat_completions：reasoning 钩子不生效；独立钩子按
	// DeepSeek 系模型判定仍 clamp（DeepSeek 覆盖不依赖 responses_mode extra）。
	noForce := ollamaCloudRawChatCompletionsTestAccount()
	noForce.Extra = nil
	require.Equal(t, body, applyOllamaCloudRawChatCompletionsRequest(noForce, body))
	require.JSONEq(t, `{"model":"deepseek-chat","max_tokens":65535}`,
		string(clampOllamaCloudUpstreamMaxTokens(noForce, body)))

	// 空 body → 原样返回。
	require.Equal(t, []byte(nil), applyOllamaCloudRawChatCompletionsRequest(ollama, nil))
	require.Equal(t, []byte{}, applyOllamaCloudRawChatCompletionsRequest(ollama, []byte{}))
}

// ollamaUpstreamTestAccount 构造挂在实际 ollama.com 上游的 APIKey 账号。平台标签
// 不参与 clamp 判定，这里用不同平台仅为了覆盖生产里的挂载方式；base_url 与生产
// 实测一致（https://ollama.com/v1）。
func ollamaUpstreamTestAccount(platform string, id int64) *Account {
	return &Account{
		ID:       id,
		Name:     "ollama-cloud",
		Platform: platform,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://ollama.com/v1",
		},
		Extra: map[string]any{},
	}
}

// officialDeepSeekTestAccount 构造官方 DeepSeek 账号，extra 里带残留的 Ollama
// usage 快照键：usage extra 不得把非 Ollama host 识别为 Ollama 上游。
func officialDeepSeekTestAccount(id int64) *Account {
	return &Account{
		ID:       id,
		Name:     "official-deepseek",
		Platform: PlatformDeepseek,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://api.deepseek.com",
		},
		Extra: map[string]any{
			OllamaCloudUsageSnapshotExtraKey: map[string]any{"status": "ok"},
		},
	}
}

func TestClampOllamaCloudUpstreamMaxTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		account *Account
		body    string
		want    string
	}{
		{
			// 生产问题场景：deepseek 平台 Ollama 账号，无 openai_responses_mode extra，
			// raw CC 直转时 max_tokens 超限被上游 400。
			name:    "deepseek platform ollama.com without responses_mode is clamped",
			account: ollamaUpstreamTestAccount(PlatformDeepseek, 301),
			body:    `{"model":"deepseek-v4-flash","max_tokens":256000}`,
			want:    `{"model":"deepseek-v4-flash","max_tokens":65535}`,
		},
		{
			name:    "max_completion_tokens is clamped too",
			account: ollamaUpstreamTestAccount(PlatformDeepseek, 301),
			body:    `{"model":"deepseek-v4-flash","max_completion_tokens":256000}`,
			want:    `{"model":"deepseek-v4-flash","max_completion_tokens":65535}`,
		},
		{
			// 新增范围只限 DeepSeek 系模型：kimi/zhipu 平台挂 ollama.com 跑非
			// DeepSeek 模型的请求保持不变。
			name:    "kimi platform non-deepseek model untouched",
			account: ollamaUpstreamTestAccount(PlatformKimi, 302),
			body:    `{"model":"k3-256k","max_tokens":256000}`,
			want:    `{"model":"k3-256k","max_tokens":256000}`,
		},
		{
			name:    "zhipu platform non-deepseek model untouched",
			account: ollamaUpstreamTestAccount(PlatformZhipu, 302),
			body:    `{"model":"glm-4.7","max_tokens":256000}`,
			want:    `{"model":"glm-4.7","max_tokens":256000}`,
		},
		{
			// 既有 openai 平台 Ollama 账号对 DeepSeek 模型的 clamp 幂等保持。
			name:    "openai platform ollama.com deepseek model stays clamped",
			account: ollamaUpstreamTestAccount(PlatformOpenAI, 303),
			body:    `{"model":"deepseek-v4-flash","max_tokens":70000}`,
			want:    `{"model":"deepseek-v4-flash","max_tokens":65535}`,
		},
		{
			// 既有 openai 平台 Ollama（真实 ollama.com + force_chat_completions）对
			// 非 DeepSeek 模型的 clamp 保留。
			name:    "openai platform ollama.com non-deepseek model keeps legacy clamp",
			account: ollamaCloudRawChatCompletionsTestAccount(),
			body:    `{"model":"gpt-oss:120b-cloud","max_tokens":70000}`,
			want:    `{"model":"gpt-oss:120b-cloud","max_tokens":65535}`,
		},
		{
			// 非 DeepSeek 模型不扩展到其它平台。
			name: "openai platform ollama.com non-deepseek model without force_cc untouched",
			account: func() *Account {
				account := ollamaUpstreamTestAccount(PlatformOpenAI, 303)
				account.Extra = nil
				return account
			}(),
			body: `{"model":"gpt-oss:120b-cloud","max_tokens":70000}`,
			want: `{"model":"gpt-oss:120b-cloud","max_tokens":70000}`,
		},
		{
			// 残留 usage extra 不得把非 Ollama host 变成 Ollama（旧 reasoning 钩子内的
			// clamp 会命中该账号，解耦后已封堵）。
			name: "openai platform non-ollama host with leftover usage extra untouched",
			account: func() *Account {
				account := ollamaCloudRawChatCompletionsTestAccount()
				account.Credentials["base_url"] = "https://example.invalid/v1"
				account.Extra[OllamaCloudUsageSnapshotExtraKey] = map[string]any{"status": "ok"}
				return account
			}(),
			body: `{"model":"gpt-oss:120b-cloud","max_tokens":70000}`,
			want: `{"model":"gpt-oss:120b-cloud","max_tokens":70000}`,
		},
		{
			name: "extra cap zero disables clamping",
			account: func() *Account {
				account := ollamaUpstreamTestAccount(PlatformDeepseek, 304)
				account.Extra[OllamaCloudMaxTokensCapExtraKey] = 0
				return account
			}(),
			body: `{"model":"deepseek-v4-flash","max_tokens":256000}`,
			want: `{"model":"deepseek-v4-flash","max_tokens":256000}`,
		},
		{
			name:    "value at cap is untouched",
			account: ollamaUpstreamTestAccount(PlatformDeepseek, 304),
			body:    `{"model":"deepseek-v4-flash","max_tokens":65535}`,
			want:    `{"model":"deepseek-v4-flash","max_tokens":65535}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			require.JSONEq(t, test.want, string(clampOllamaCloudUpstreamMaxTokens(test.account, []byte(test.body))))
		})
	}

	// 非 Ollama 实际上游（api.deepseek.com）即使残留 usage extra 也不 clamp。
	official := officialDeepSeekTestAccount(305)
	body := []byte(`{"model":"deepseek-v4-flash","max_tokens":256000,"max_completion_tokens":256000}`)
	require.Equal(t, string(body), string(clampOllamaCloudUpstreamMaxTokens(official, body)))

	// 空 body / nil 账号原样返回。
	require.Equal(t, []byte(nil), clampOllamaCloudUpstreamMaxTokens(official, nil))
}

// TestForwardAsRawChatCompletions_DeepseekOllamaCloudClampsMaxTokens 端到端验证 raw
// CC 出站钩子：判定用模型映射后的真实出站 model，官方 DeepSeek 字节级不变。
func TestForwardAsRawChatCompletions_DeepseekOllamaCloudClampsMaxTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)

	account := ollamaUpstreamTestAccount(PlatformDeepseek, 311)
	account.Credentials["api_protocol"] = APIProtocolChatCompletions
	account.Credentials["model_mapping"] = map[string]any{"deepseek-chat": "deepseek-v4-flash"}

	body := []byte(`{"model":"deepseek-chat","max_tokens":256000,"messages":[{"role":"user","content":"hi"}],"stream":false}`)
	upstream := &httpUpstreamRecorder{err: errors.New("stop after capture")}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

	_, err := svc.forwardAsRawChatCompletions(context.Background(), adaptiveProtocolTestContext("/v1/chat/completions", body), account, body, "")
	require.Error(t, err)
	require.Equal(t, "deepseek-v4-flash", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, int64(65535), gjson.GetBytes(upstream.lastBody, "max_tokens").Int())

	// 官方 DeepSeek（api.deepseek.com）+ 残留 usage extra：字节级不变。
	official := officialDeepSeekTestAccount(312)
	official.Credentials["api_protocol"] = APIProtocolChatCompletions
	officialBody := []byte(`{"model":"deepseek-v4-flash","max_tokens":256000,"messages":[{"role":"user","content":"hi"}],"stream":false}`)
	officialUpstream := &httpUpstreamRecorder{err: errors.New("stop after capture")}
	officialSvc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: officialUpstream}

	_, err = officialSvc.forwardAsRawChatCompletions(context.Background(), adaptiveProtocolTestContext("/v1/chat/completions", officialBody), official, officialBody, "")
	require.Error(t, err)
	require.Equal(t, string(officialBody), string(officialUpstream.lastBody))
}

// TestForwardResponsesViaRawChatCompletions_DeepseekOllamaCloudClampsMaxTokens 验证
// /v1/responses 降级到 raw CC 的路径确实经过同一个独立 token 钩子（Responses 请求的
// max_output_tokens 经 apicompat 转成 CC 的 max_completion_tokens）。
func TestForwardResponsesViaRawChatCompletions_DeepseekOllamaCloudClampsMaxTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)

	account := ollamaUpstreamTestAccount(PlatformDeepseek, 321)
	account.Credentials["api_protocol"] = APIProtocolChatCompletions

	body := []byte(`{"model":"deepseek-v4-flash","input":"Reply with exactly OK and nothing else.","max_output_tokens":256000,"stream":false}`)
	upstream := &httpUpstreamRecorder{err: errors.New("stop after capture")}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

	_, err := svc.Forward(context.Background(), adaptiveProtocolTestContext("/v1/responses", body), account, body)
	require.Error(t, err)
	require.Equal(t, "https://ollama.com/v1/chat/completions", upstream.lastReq.URL.String())
	require.Equal(t, "deepseek-v4-flash", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, int64(65535), gjson.GetBytes(upstream.lastBody, "max_completion_tokens").Int())
}

// TestForwardResponsesClampsOllamaCloudMaxOutputTokens 覆盖原生 /v1/responses 路径
// （openai force_responses 与 deepseek api_protocol=responses）的 max_output_tokens
// clamp，实测依据：POST ollama.com/v1/responses max_output_tokens=256000 被上游以
// "max_tokens (256000) exceeds model's maximum output tokens (65536)" 400 拒绝。
func TestForwardResponsesClampsOllamaCloudMaxOutputTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)

	responsesBody := []byte(`{"model":"deepseek-v4-flash","input":"Reply with exactly OK and nothing else.","max_output_tokens":256000,"stream":false}`)

	run := func(account *Account, body []byte) (*httpUpstreamRecorder, error) {
		upstream := &httpUpstreamRecorder{err: errors.New("stop after capture")}
		svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
		_, err := svc.Forward(context.Background(), adaptiveProtocolTestContext("/v1/responses", body), account, body)
		return upstream, err
	}

	t.Run("deepseek platform native responses is clamped", func(t *testing.T) {
		account := ollamaUpstreamTestAccount(PlatformDeepseek, 331)
		account.Credentials["api_protocol"] = APIProtocolResponses
		upstream, err := run(account, responsesBody)
		require.Error(t, err)
		require.Equal(t, "https://ollama.com/v1/responses", upstream.lastReq.URL.String())
		require.Equal(t, int64(65535), gjson.GetBytes(upstream.lastBody, "max_output_tokens").Int())
	})

	t.Run("openai platform force_responses is clamped", func(t *testing.T) {
		account := ollamaUpstreamTestAccount(PlatformOpenAI, 332)
		account.Extra[openai_compat.ExtraKeyResponsesMode] = string(openai_compat.ResponsesSupportModeForceResponses)
		upstream, err := run(account, responsesBody)
		require.Error(t, err)
		require.Equal(t, int64(65535), gjson.GetBytes(upstream.lastBody, "max_output_tokens").Int())
	})

	t.Run("official deepseek with leftover usage extra is untouched", func(t *testing.T) {
		account := officialDeepSeekTestAccount(333)
		account.Credentials["api_protocol"] = APIProtocolResponses
		upstream, err := run(account, responsesBody)
		require.Error(t, err)
		require.Equal(t, int64(256000), gjson.GetBytes(upstream.lastBody, "max_output_tokens").Int())
	})

	t.Run("at cap is untouched", func(t *testing.T) {
		account := ollamaUpstreamTestAccount(PlatformDeepseek, 334)
		account.Credentials["api_protocol"] = APIProtocolResponses
		atCap := []byte(`{"model":"deepseek-v4-flash","input":"hi","max_output_tokens":65535,"stream":false}`)
		upstream, err := run(account, atCap)
		require.Error(t, err)
		require.Equal(t, int64(65535), gjson.GetBytes(upstream.lastBody, "max_output_tokens").Int())
	})

	t.Run("extra cap zero disables clamping", func(t *testing.T) {
		account := ollamaUpstreamTestAccount(PlatformDeepseek, 335)
		account.Credentials["api_protocol"] = APIProtocolResponses
		account.Extra[OllamaCloudMaxTokensCapExtraKey] = 0
		upstream, err := run(account, responsesBody)
		require.Error(t, err)
		require.Equal(t, int64(256000), gjson.GetBytes(upstream.lastBody, "max_output_tokens").Int())
	})
}

// TestForwardResponsesClampUsesResponsesUpstreamBaseURL 验证原生 Responses clamp 按
// 实际 Responses 上游判定：adaptive 账号取 api_base_urls 的 responses 地址，而非 CC
// 地址（与 buildUpstreamRequest 的 URL 选择一致）。
func TestForwardResponsesClampUsesResponsesUpstreamBaseURL(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"deepseek-v4-flash","input":"hi","max_output_tokens":256000,"stream":false}`)
	run := func(account *Account) (*httpUpstreamRecorder, error) {
		upstream := &httpUpstreamRecorder{err: errors.New("stop after capture")}
		svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
		_, err := svc.Forward(context.Background(), adaptiveProtocolTestContext("/v1/responses", body), account, body)
		return upstream, err
	}

	t.Run("ollama CC base but non-ollama responses base is not clamped", func(t *testing.T) {
		account := ollamaUpstreamTestAccount(PlatformDeepseek, 341)
		account.Credentials["api_protocol"] = APIProtocolAdaptive
		account.Credentials["api_base_urls"] = map[string]any{
			APIProtocolChatCompletions: "https://ollama.com/v1",
			APIProtocolResponses:       "https://api.deepseek.com",
		}
		upstream, err := run(account)
		require.Error(t, err)
		require.Equal(t, "https://api.deepseek.com/responses", upstream.lastReq.URL.String())
		require.Equal(t, int64(256000), gjson.GetBytes(upstream.lastBody, "max_output_tokens").Int())
	})

	t.Run("non-ollama CC base but ollama responses base is clamped", func(t *testing.T) {
		account := ollamaUpstreamTestAccount(PlatformDeepseek, 342)
		account.Credentials["api_protocol"] = APIProtocolAdaptive
		account.Credentials["api_base_urls"] = map[string]any{
			APIProtocolChatCompletions: "https://api.deepseek.com",
			APIProtocolResponses:       "https://ollama.com/v1",
		}
		upstream, err := run(account)
		require.Error(t, err)
		require.Equal(t, "https://ollama.com/v1/responses", upstream.lastReq.URL.String())
		require.Equal(t, int64(65535), gjson.GetBytes(upstream.lastBody, "max_output_tokens").Int())
	})
}

// TestForwardResponsesClampsOllamaCloudMaxOutputTokensForCodexClients 回归：原生
// /v1/responses 的 Ollama Cloud clamp 必须对真实 Codex 客户端同样生效（clamp 曾位于
// !isCodexCLI 块内被跳过，ollama.com 对 >65535 的 max_output_tokens 一律 400）。
func TestForwardResponsesClampsOllamaCloudMaxOutputTokensForCodexClients(t *testing.T) {
	gin.SetMode(gin.TestMode)

	responsesBody := []byte(`{"model":"deepseek-v4-flash","input":"Reply with exactly OK and nothing else.","max_output_tokens":256000,"stream":false}`)

	ollamaResponsesAccount := func() *Account {
		account := ollamaUpstreamTestAccount(PlatformDeepseek, 336)
		account.Credentials["api_protocol"] = APIProtocolResponses
		return account
	}
	officialResponsesAccount := func() *Account {
		account := officialDeepSeekTestAccount(337)
		account.Credentials["api_protocol"] = APIProtocolResponses
		return account
	}
	codexUA := func(c *gin.Context) {
		c.Request.Header.Set("User-Agent", "codex_cli_rs/0.55.0 (Ubuntu 24.04; x86_64)")
	}
	codexOriginator := func(c *gin.Context) { c.Request.Header.Set("originator", "codex-tui") }

	tests := []struct {
		name          string
		account       *Account
		setHeaders    func(*gin.Context)
		forceCodexCLI bool
		wantCap       int64
		wantURL       string
	}{
		{
			name:       "codex UA is clamped",
			account:    ollamaResponsesAccount(),
			setHeaders: codexUA,
			wantCap:    65535,
			wantURL:    "https://ollama.com/v1/responses",
		},
		{
			name:       "codex originator is clamped",
			account:    ollamaResponsesAccount(),
			setHeaders: codexOriginator,
			wantCap:    65535,
			wantURL:    "https://ollama.com/v1/responses",
		},
		{
			name:          "ForceCodexCLI is clamped",
			account:       ollamaResponsesAccount(),
			forceCodexCLI: true,
			wantCap:       65535,
			wantURL:       "https://ollama.com/v1/responses",
		},
		{
			// 非 Codex 正常路径对照：与既有用例行为一致，不回归。
			name:    "non-codex control is clamped",
			account: ollamaResponsesAccount(),
			wantCap: 65535,
			wantURL: "https://ollama.com/v1/responses",
		},
		{
			// 非 Ollama 上游：同样的 Codex 头不 clamp。
			name:       "official deepseek with codex UA is untouched",
			account:    officialResponsesAccount(),
			setHeaders: codexUA,
			wantCap:    256000,
			wantURL:    "https://api.deepseek.com/responses",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{err: errors.New("stop after capture")}
			cfg := rawChatCompletionsTestConfig()
			cfg.Gateway.ForceCodexCLI = test.forceCodexCLI
			svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
			c := adaptiveProtocolTestContext("/v1/responses", responsesBody)
			if test.setHeaders != nil {
				test.setHeaders(c)
			}
			_, err := svc.Forward(context.Background(), c, test.account, responsesBody)
			require.Error(t, err)
			require.Equal(t, test.wantURL, upstream.lastReq.URL.String())
			require.Equal(t, test.wantCap, gjson.GetBytes(upstream.lastBody, "max_output_tokens").Int())
		})
	}
}
