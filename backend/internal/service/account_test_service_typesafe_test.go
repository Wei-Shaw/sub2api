//go:build unit

package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// realHTTPUpstream 在单测里真正发出请求，让 httptest 服务端能断言实际到达的
// path / header / body（httpUpstreamRecorder 只记录，不发出请求）。
type realHTTPUpstream struct{}

func (realHTTPUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return http.DefaultClient.Do(req)
}

func (realHTTPUpstream) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return http.DefaultClient.Do(req)
}

func typesafeAccountTestService(account *Account, upstream HTTPUpstream) *AccountTestService {
	repo := &openAIAccountTestRepo{
		mockAccountRepoForGemini: mockAccountRepoForGemini{
			accountsByID: map[int64]*Account{account.ID: account},
		},
	}
	return &AccountTestService{
		accountRepo:  repo,
		httpUpstream: upstream,
		cfg:          rawChatCompletionsTestConfig(),
	}
}

// 测试连接必须命中 {base}/v1/systemone 并带上账号 api_key 的 Bearer 头，
// 且不能携带 Anthropic 协议形状（anthropic-version）。
func TestAccountTestService_TypeSafeConnectionProbesSystemOneWithBearerKey(t *testing.T) {
	var (
		hits                int
		gotMethod           string
		gotPath             string
		gotAuth             string
		gotAnthropicVersion string
		gotBody             []byte
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotAnthropicVersion = r.Header.Get("anthropic-version")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"jev-1.13.0","usage":{"input_tokens":3,"output_tokens":1},"answers":{"connectivity":{"type":"noul","noul":0}}}`))
	}))
	defer server.Close()

	account := &Account{
		ID:          401,
		Name:        "typesafe-apikey",
		Platform:    PlatformTypeSafe,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "ts-probe-key",
			"base_url": server.URL,
		},
	}
	svc := typesafeAccountTestService(account, realHTTPUpstream{})
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "jev-judge-v1", "", AccountTestModeDefault)

	require.NoError(t, err)
	require.Equal(t, 1, hits)
	require.Equal(t, http.MethodPost, gotMethod)
	require.Equal(t, "/v1/systemone", gotPath)
	require.Equal(t, "Bearer ts-probe-key", gotAuth)
	require.Empty(t, gotAnthropicVersion)

	require.Equal(t, "jev-judge-v1", gjson.GetBytes(gotBody, "model").String())
	require.Equal(t, typeSafeTestProbeState, gjson.GetBytes(gotBody, "state").String())
	require.True(t, gjson.GetBytes(gotBody, "questions."+typeSafeTestProbeQuestionID+".type").Exists())
	require.Equal(t, "noul", gjson.GetBytes(gotBody, "questions."+typeSafeTestProbeQuestionID+".type").String())

	body := recorder.Body.String()
	require.Contains(t, body, `"type":"test_complete"`)
	require.Contains(t, body, "已通过原生 /v1/systemone 验证（上游模型 jev-1.13.0）")
}

// 没有配置 base_url 的 typesafe 账号：连接测试走平台默认上游 api.typesafe.ai，
// 绝不向 api.anthropic.com 发出任何请求（否则等于把 typesafe 的 api_key 明文
// 发给 Anthropic 官方域名）。未指定 model_id 时用平台默认探测模型。
func TestAccountTestService_TypeSafeConnectionWithoutBaseURLNeverTargetsAnthropic(t *testing.T) {
	account := &Account{
		ID:          402,
		Name:        "typesafe-no-base-url",
		Platform:    PlatformTypeSafe,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "ts-no-base-url"},
	}
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		newJSONResponse(http.StatusOK, `{"model":"jev-1.13.0","answers":{"connectivity":{"type":"noul","noul":0}}}`),
	}}
	svc := typesafeAccountTestService(account, upstream)
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "", "", AccountTestModeDefault)

	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "api.typesafe.ai", upstream.requests[0].URL.Host)
	require.Equal(t, "/v1/systemone", upstream.requests[0].URL.Path)
	require.Equal(t, "Bearer ts-no-base-url", upstream.requests[0].Header.Get("Authorization"))
	require.Equal(t, DefaultTypeSafeTestModel, gjson.GetBytes(upstream.bodies[0], "model").String())
	require.False(t, gjson.GetBytes(upstream.bodies[0], "messages").Exists())
	// fail-closed 断言：整个测试连接里没有任何请求指向 Anthropic 官方域名。
	for _, req := range upstream.requests {
		require.NotContains(t, req.URL.Host, "anthropic")
	}
	require.Contains(t, recorder.Body.String(), `"type":"test_complete"`)
}

// 非 typesafe 平台的测试连接行为不变：仍走 {base}/v1/messages?beta=true 的
// Anthropic 形状探测（含 anthropic-version 与 x-api-key）。
func TestAccountTestService_NonTypeSafeConnectionStillUsesAnthropicProbe(t *testing.T) {
	account := &Account{
		ID:          403,
		Name:        "anthropic-apikey",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-ant-relay",
			"base_url": "http://anthropic-relay.example",
		},
	}
	resp := newJSONResponse(http.StatusOK, "")
	resp.Header.Set("Content-Type", "text/event-stream")
	resp.Body = io.NopCloser(strings.NewReader("data: {\"type\":\"message_stop\"}\n\n"))
	upstream := &httpUpstreamRecorder{responses: []*http.Response{resp}}
	svc := typesafeAccountTestService(account, upstream)
	c, _ := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "", "", AccountTestModeDefault)

	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "http://anthropic-relay.example/v1/messages?beta=true", upstream.requests[0].URL.String())
	require.Equal(t, "2023-06-01", upstream.requests[0].Header.Get("anthropic-version"))
	require.Equal(t, "sk-ant-relay", upstream.requests[0].Header.Get("x-api-key"))
}
