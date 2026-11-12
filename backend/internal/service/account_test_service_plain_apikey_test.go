//go:build unit

package service

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/tidwall/gjson"
)

func anthropicAPIKeyTestAccount(id int64) *Account {
	return &Account{
		ID:          id,
		Name:        "anthropic-apikey-test",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-ant-test",
			"base_url": "http://testserver.example",
		},
	}
}

func anthropicPlainTestService(account *Account, responses ...*http.Response) (*AccountTestService, *queuedHTTPUpstream) {
	repo := &openAIAccountTestRepo{
		mockAccountRepoForGemini: mockAccountRepoForGemini{
			accountsByID: map[int64]*Account{account.ID: account},
		},
	}
	upstream := &queuedHTTPUpstream{responses: responses}
	return &AccountTestService{
		accountRepo:         repo,
		httpUpstream:        upstream,
		cfg:                 rawChatCompletionsTestConfig(),
		tlsFPProfileService: &TLSFingerprintProfileService{},
	}, upstream
}

func anthropicClaudeStreamResponse() *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(`data: {"type":"content_block_delta","delta":{"text":"ok"}}

data: {"type":"message_stop"}

`)),
	}
}

func recordedUpstreamBody(t *testing.T, req *http.Request) []byte {
	t.Helper()
	if req.Body == nil {
		return nil
	}
	data, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	req.Body = io.NopCloser(strings.NewReader(string(data)))
	return data
}

// PlainAPIKey Anthropic 测试连接：出站请求不应携带任何 Claude Code 伪装。
func TestAccountTestService_AnthropicPlainAPIKeyTestOmitsClaudeCodeMimicry(t *testing.T) {
	ctx, recorder := newTestContext()
	account := anthropicAPIKeyTestAccount(501)
	svc, upstream := anthropicPlainTestService(account, anthropicClaudeStreamResponse())

	err := svc.testClaudeAccountConnection(ctx, account, "", true)
	require.NoError(t, err)
	require.Contains(t, recorder.Body.String(), "test_complete")

	require.Len(t, upstream.requests, 1)
	req := upstream.requests[0]

	// 没有 Claude Code 伪装头 / beta。
	require.NotContains(t, req.Header.Get("User-Agent"), "claude-cli")
	require.Empty(t, req.Header.Get("X-Stainless-Lang"))
	require.Empty(t, req.Header.Get("X-Stainless-Runtime"))
	require.Empty(t, req.Header.Get("X-App"))
	require.Empty(t, req.Header.Get("anthropic-beta"))

	// 认证头与基础头保留。
	require.Equal(t, "sk-ant-test", req.Header.Get("x-api-key"))
	require.Equal(t, "2023-06-01", req.Header.Get("anthropic-version"))
	require.Equal(t, "application/json", req.Header.Get("Content-Type"))

	// 朴素 payload：无 system prompt、无 metadata.user_id、无 cache_control。
	body := recordedUpstreamBody(t, req)
	require.Equal(t, account.GetMappedModel(claude.DefaultTestModel), gjson.GetBytes(body, "model").String())
	require.Empty(t, gjson.GetBytes(body, "metadata.user_id").Raw)
	require.Empty(t, gjson.GetBytes(body, "system").Raw)
	require.NotContains(t, string(body), "cache_control")
	require.Equal(t, "hi", gjson.GetBytes(body, "messages.0.content.0.text").String())
}

// 不勾选 plain（默认）：保持原有 Claude Code 伪装行为。
func TestAccountTestService_AnthropicDefaultAPIKeyTestKeepsMimicry(t *testing.T) {
	ctx, _ := newTestContext()
	account := anthropicAPIKeyTestAccount(502)
	svc, upstream := anthropicPlainTestService(account, anthropicClaudeStreamResponse())

	err := svc.testClaudeAccountConnection(ctx, account, "", false)
	require.NoError(t, err)

	require.Len(t, upstream.requests, 1)
	req := upstream.requests[0]
	require.Contains(t, req.Header.Get("User-Agent"), "claude-cli")
	require.NotEmpty(t, req.Header.Get("X-Stainless-Lang"))
	require.Contains(t, req.Header.Get("anthropic-beta"), "claude-code-20250219")

	body := recordedUpstreamBody(t, req)
	require.NotEmpty(t, gjson.GetBytes(body, "metadata.user_id").Raw)
	require.NotEmpty(t, gjson.GetBytes(body, "system").Raw)
}

// OAuth 账号即使勾选 plain 也不生效——伪装是 OAuth 计费/可用性的前提。
func TestAccountTestService_AnthropicPlainAPIKeyIgnoredForOAuth(t *testing.T) {
	ctx, _ := newTestContext()
	account := anthropicAPIKeyTestAccount(503)
	account.Type = AccountTypeOAuth
	account.Credentials = map[string]any{"access_token": "oauth-token"}
	svc, upstream := anthropicPlainTestService(account, anthropicClaudeStreamResponse())

	err := svc.testClaudeAccountConnection(ctx, account, "", true)
	require.NoError(t, err)

	require.Len(t, upstream.requests, 1)
	req := upstream.requests[0]
	require.Contains(t, req.Header.Get("User-Agent"), "claude-cli")
	require.Equal(t, "Bearer oauth-token", req.Header.Get("Authorization"))
	require.Contains(t, req.Header.Get("anthropic-beta"), "oauth-2025-04-20")
}

// OpenAI apikey Responses 测试连接：仿真模式不再盖 Codex 探测头，也不再注入 Codex instructions。
func TestAccountTestService_OpenAIPlainAPIKeyTestOmitsCodexProbeHeaders(t *testing.T) {
	ctx, _ := newTestContext()
	account := &Account{
		ID:          504,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-openai-test",
			"base_url": "http://testserver.example",
		},
		Extra: map[string]any{"use_responses_api": true},
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(`data: {"type":"response.completed"}

`)),
	}
	svc, upstream := anthropicPlainTestService(account, resp)

	err := svc.testOpenAIAccountConnection(ctx, account, "gpt-5.4", "", "", true)
	require.NoError(t, err)

	require.Len(t, upstream.requests, 1)
	req := upstream.requests[0]

	require.Empty(t, req.Header.Get("originator"))
	require.Empty(t, req.Header.Get("version"))
	require.Empty(t, req.Header.Get("X-Codex-Window-ID"))
	require.Equal(t, "Bearer sk-openai-test", req.Header.Get("Authorization"))

	body := recordedUpstreamBody(t, req)
	require.Empty(t, gjson.GetBytes(body, "instructions").Raw)
	require.Equal(t, "hi", gjson.GetBytes(body, "input.0.content.0.text").String())
}

// CN provider anthropic 协议：plain 模式去掉 CC 伪装头。
func TestAccountTestService_CNProviderAnthropicPlainAPIKeyTest(t *testing.T) {
	ctx, _ := newTestContext()
	account := adaptiveCNAccountTestAccount(505, PlatformZhipu)
	account.Credentials["api_protocol"] = APIProtocolAnthropic
	account.Credentials["base_url"] = "http://anthropic.example"
	svc, upstream := anthropicPlainTestService(account, anthropicClaudeStreamResponse())

	err := svc.testCNProviderAnthropicConnection(ctx, account, "", true)
	require.NoError(t, err)

	require.Len(t, upstream.requests, 1)
	req := upstream.requests[0]
	require.NotContains(t, req.Header.Get("User-Agent"), "claude-cli")
	require.Empty(t, req.Header.Get("anthropic-beta"))
	require.Empty(t, req.Header.Get("X-Stainless-Lang"))

	body := recordedUpstreamBody(t, req)
	require.Empty(t, gjson.GetBytes(body, "metadata.user_id").Raw)
	require.Empty(t, gjson.GetBytes(body, "system").Raw)
}
