package service

// 回归测试：ChatGPT internal 不支持的顶层字段既要在出口被兜底剥离，也要在上游
// 点名拒绝时被剥掉重试一次。见 openai_upstream_egress_strip.go 头部说明与 #5803。

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestStripChatGPTInternalUnsupportedFieldsAtEgress(t *testing.T) {
	oauthAccount := newOpenAIOAuthNamespaceTestAccount()

	t.Run("strips every unsupported top-level field for OAuth", func(t *testing.T) {
		body := []byte(`{"model":"gpt-5.6-sol","input":[{"type":"message","role":"user","content":"hi"}],` +
			`"prompt_cache_retention":"24h","safety_identifier":"sid","user":"u1",` +
			`"metadata":{"k":"v"},"stream_options":{"include_usage":true}}`)

		out, stripped := stripChatGPTInternalUnsupportedFieldsAtEgress(oauthAccount, body)

		require.ElementsMatch(t,
			[]string{"user", "metadata", "prompt_cache_retention", "safety_identifier", "stream_options"},
			stripped)
		for _, field := range stripped {
			require.False(t, gjson.GetBytes(out, field).Exists(), "field %s must be gone", field)
		}
		// 语义字段必须保持原样。
		require.Equal(t, "gpt-5.6-sol", gjson.GetBytes(out, "model").String())
		require.Equal(t, "hi", gjson.GetBytes(out, "input.0.content").String())
	})

	t.Run("keeps nested occurrences that merely share the field name", func(t *testing.T) {
		body := []byte(`{"model":"gpt-5.6-sol","prompt_cache_retention":"24h",` +
			`"input":[{"type":"message","role":"user","content":{"prompt_cache_retention":"keep"}}]}`)

		out, stripped := stripChatGPTInternalUnsupportedFieldsAtEgress(oauthAccount, body)

		require.Equal(t, []string{"prompt_cache_retention"}, stripped)
		require.False(t, gjson.GetBytes(out, "prompt_cache_retention").Exists())
		require.Equal(t, "keep", gjson.GetBytes(out, "input.0.content.prompt_cache_retention").String())
	})

	t.Run("no-op when nothing unsupported is present", func(t *testing.T) {
		body := []byte(`{"model":"gpt-5.6-sol","input":"hi"}`)

		out, stripped := stripChatGPTInternalUnsupportedFieldsAtEgress(oauthAccount, body)

		require.Empty(t, stripped)
		require.Equal(t, body, out)
	})

	// APIKey 账号走 Platform API 或第三方 OpenAI 兼容上游，这些字段在那边是合法的，
	// 出口不能替它们做决定。
	t.Run("leaves APIKey accounts untouched", func(t *testing.T) {
		body := []byte(`{"model":"gpt-5.5","prompt_cache_retention":"24h","user":"u1"}`)

		out, stripped := stripChatGPTInternalUnsupportedFieldsAtEgress(newOpenAIRejectedFieldTestAccount(), body)

		require.Empty(t, stripped)
		require.Equal(t, body, out)
		require.True(t, gjson.GetBytes(out, "prompt_cache_retention").Exists())
	})

	t.Run("tolerates nil account and empty body", func(t *testing.T) {
		out, stripped := stripChatGPTInternalUnsupportedFieldsAtEgress(nil, []byte(`{"prompt_cache_retention":"24h"}`))
		require.Empty(t, stripped)
		require.True(t, gjson.GetBytes(out, "prompt_cache_retention").Exists())

		out, stripped = stripChatGPTInternalUnsupportedFieldsAtEgress(oauthAccount, nil)
		require.Empty(t, stripped)
		require.Empty(t, out)
	})
}

// TestOpenAIGatewayService_EgressStripsPromptCacheRetentionForCodexCLIOAuth 是
// #5803 的端到端回归：OAuth + Codex CLI + GPT-5.6 + prompt_cache_retention，
// 断言真正发往上游的 body 不含该字段。
func TestOpenAIGatewayService_EgressStripsPromptCacheRetentionForCodexCLIOAuth(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-sol","stream":false,"prompt_cache_retention":"24h",` +
		`"safety_identifier":"sid","input":[{"type":"message","role":"user","content":"hi"}]}`)
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		newOpenAIRejectedFieldTestResponse(http.StatusOK,
			`{"output":[],"usage":{"input_tokens":1,"output_tokens":1,"input_tokens_details":{"cached_tokens":0}}}`),
	}}

	c := newOpenAIRejectedFieldTestContext(body)
	c.Request.Header.Set("User-Agent", buildCodexCLIUserAgent("0.148.0"))
	c.Request.Header.Set("originator", "codex_cli_rs")

	result, err := newOpenAIRejectedFieldTestService(upstream).Forward(
		context.Background(), c, newOpenAIOAuthNamespaceTestAccount(), body,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 1)
	require.False(t, gjson.GetBytes(upstream.bodies[0], "prompt_cache_retention").Exists())
	require.False(t, gjson.GetBytes(upstream.bodies[0], "safety_identifier").Exists())
}

// buildUpstreamRequest 是出口本身：直接喂一个带字段的 body，模拟「上游所有转换
// 分支都漏了」的最坏情况，断言出口仍然兜住。
func TestOpenAIGatewayService_BuildUpstreamRequestStripsUnsupportedFieldsAsLastResort(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-sol","input":"hi","prompt_cache_retention":"24h","metadata":{"k":"v"}}`)
	svc := newOpenAIRejectedFieldTestService(&httpUpstreamRecorder{})

	req, err := svc.buildUpstreamRequest(
		context.Background(), newOpenAIRejectedFieldTestContext(body),
		newOpenAIOAuthNamespaceTestAccount(), body, "oauth-token", false, "", true,
	)

	require.NoError(t, err)
	require.NotNil(t, req)
	sentBody, readErr := readAllUpstreamRequestBody(req)
	require.NoError(t, readErr)
	require.False(t, gjson.GetBytes(sentBody, "prompt_cache_retention").Exists())
	require.False(t, gjson.GetBytes(sentBody, "metadata").Exists())
	require.Equal(t, "gpt-5.6-sol", gjson.GetBytes(sentBody, "model").String())
}

func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyStripsRejectedOptionalParams(t *testing.T) {
	tests := []struct {
		name         string
		body         []byte
		responseBody []byte
		wantReason   string
		wantGone     string
	}{
		{
			// 线上实测形态：code 是 invalid_parameter，文案是 "is not supported on this model"，
			// 两者都不在旧的 unknown/unsupported parameter 判定里。
			name: "invalid_parameter is not supported on this model",
			body: []byte(`{"model":"gpt-5.6-sol","input":"hi","prompt_cache_retention":"24h"}`),
			responseBody: []byte(`{"error":{"code":"invalid_parameter",` +
				`"message":"prompt_cache_retention is not supported on this model",` +
				`"param":"prompt_cache_retention","type":"invalid_request_error"}}`),
			wantReason: "prompt_cache_retention parameter rejection",
			wantGone:   "prompt_cache_retention",
		},
		{
			name: "nested param strips the whole top-level object",
			body: []byte(`{"model":"gpt-5.6-sol","input":"hi","prompt_cache_options":{"ttl":"30m"}}`),
			responseBody: []byte(`{"error":{"code":"invalid_parameter",` +
				`"message":"prompt_cache_options.ttl is not supported on this model",` +
				`"param":"prompt_cache_options.ttl"}}`),
			wantReason: "prompt_cache_options parameter rejection",
			wantGone:   "prompt_cache_options",
		},
		{
			name: "falls back to the message when error.param is absent",
			body: []byte(`{"model":"gpt-5.6-sol","input":"hi","safety_identifier":"sid"}`),
			responseBody: []byte(`{"error":{"code":"invalid_parameter",` +
				`"message":"safety_identifier is not supported on this model"}}`),
			wantReason: "safety_identifier parameter rejection",
			wantGone:   "safety_identifier",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			retryBody, reason, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(
				http.StatusBadRequest, tc.body, tc.responseBody)

			require.NoError(t, err)
			require.True(t, changed)
			require.Equal(t, tc.wantReason, reason)
			require.False(t, gjson.GetBytes(retryBody, tc.wantGone).Exists())
			require.Equal(t, "gpt-5.6-sol", gjson.GetBytes(retryBody, "model").String())
		})
	}
}

func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyKeepsSemanticParams(t *testing.T) {
	tests := []struct {
		name         string
		body         []byte
		responseBody []byte
	}{
		{
			// 语义关键字段被拒时必须如实回传客户端，不能悄悄改写请求再重试。
			name:         "model rejection is not silently rewritten",
			body:         []byte(`{"model":"gpt-5.6-sol","input":"hi"}`),
			responseBody: []byte(`{"error":{"code":"invalid_parameter","message":"model is not supported on this model","param":"model"}}`),
		},
		{
			// 线上同期真实存在的另一类 400，绝不能被误当成可剥字段。
			name:         "indexed input param is untouched",
			body:         []byte(`{"model":"gpt-5.6-sol","input":[{"type":"function_call","name":"bad name"}]}`),
			responseBody: []byte(`{"error":{"code":"invalid_value","message":"Invalid 'input[74].name': string does not match pattern.","param":"input[74].name"}}`),
		},
		{
			name:         "whitelisted param absent from body yields no retry",
			body:         []byte(`{"model":"gpt-5.6-sol","input":"hi"}`),
			responseBody: []byte(`{"error":{"code":"invalid_parameter","message":"prompt_cache_retention is not supported on this model","param":"prompt_cache_retention"}}`),
		},
		{
			name:         "non-400 status is ignored",
			body:         []byte(`{"model":"gpt-5.6-sol","prompt_cache_retention":"24h"}`),
			responseBody: []byte(`{"error":{"code":"invalid_parameter","message":"prompt_cache_retention is not supported on this model","param":"prompt_cache_retention"}}`),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status := http.StatusBadRequest
			if tc.name == "non-400 status is ignored" {
				status = http.StatusInternalServerError
			}

			retryBody, reason, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(
				status, tc.body, tc.responseBody)

			require.NoError(t, err)
			require.False(t, changed)
			require.Empty(t, reason)
			require.Nil(t, retryBody)
		})
	}
}

// 兜底重试的目标场景：APIKey 账号 + Codex CLI 客户端。
//
// 这个组合是现有两处清理的真实盲区——applyCodexOAuthTransform 只对 OAuth 生效，
// 而 openai_gateway_forward.go 的顶层清理挂在 `if !isCodexCLI` 下，Codex CLI 会
// 跳过它。字段因此原样到达上游；上游点名拒绝后必须剥掉并同号重试一次，而不是把
// 400 直接抛给客户端（换号 failover 也救不了参数错误）。
func TestOpenAIGatewayService_RetriesInvalidParameterRejectionOnce(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","stream":false,"prompt_cache_retention":"24h","input":"hi"}`)
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		newOpenAIRejectedFieldTestResponse(http.StatusBadRequest,
			`{"error":{"code":"invalid_parameter","message":"prompt_cache_retention is not supported on this model","param":"prompt_cache_retention","type":"invalid_request_error"}}`),
		newOpenAIRejectedFieldTestResponse(http.StatusOK,
			`{"output":[],"usage":{"input_tokens":1,"output_tokens":1,"input_tokens_details":{"cached_tokens":0}}}`),
	}}

	c := newOpenAIRejectedFieldTestContext(body)
	c.Request.Header.Set("User-Agent", buildCodexCLIUserAgent("0.148.0"))
	c.Request.Header.Set("originator", "codex_cli_rs")

	result, err := newOpenAIRejectedFieldTestService(upstream).Forward(
		context.Background(),
		c,
		newOpenAIRejectedFieldTestAccount(),
		body,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 2)
	require.True(t, gjson.GetBytes(upstream.bodies[0], "prompt_cache_retention").Exists())
	require.False(t, gjson.GetBytes(upstream.bodies[1], "prompt_cache_retention").Exists())
	require.Equal(t, "hi", gjson.GetBytes(upstream.bodies[1], "input").String())
}

func readAllUpstreamRequestBody(req *http.Request) ([]byte, error) {
	if req.Body == nil {
		return nil, nil
	}
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(req.Body); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
