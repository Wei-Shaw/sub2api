//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
)

func zhipuNativeResponsesAccount() *Account {
	return &Account{
		ID:          901,
		Platform:    PlatformZhipu,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":      "sk-test",
			"base_url":     "http://upstream.example",
			"api_protocol": APIProtocolResponses,
		},
	}
}

// 智谱账号显式选择 responses 协议时必须尊重配置，按原生 Responses 端点
// 透传（上游若是 sub2api 等支持 /v1/responses 的网关即可直连），不得静默
// 降级成 Chat Completions。
func TestForwardResponses_ZhipuExplicitResponsesUsesNativeEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"glm-5.3-flash","input":"hello","stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_zhipu_native"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"resp_zhipu_native","object":"response","model":"glm-5.3-flash","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}],"status":"completed"}],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}}`,
		)),
	}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

	result, err := svc.Forward(context.Background(), c, zhipuNativeResponsesAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "http://upstream.example/v1/responses", upstream.lastReq.URL.String())
	require.True(t, gjson.GetBytes(upstream.lastBody, "input").Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "messages").Exists())
}

// 能力探测对显式 responses 协议的智谱账号必须落标 force_responses，与转发
// 路径一致；不能因为平台硬编码而强制重置回 auto + false。
func TestProbeOpenAIAPIKeyResponsesSupport_ZhipuExplicitResponsesForcesResponses(t *testing.T) {
	updateCalls := make(chan map[string]any, 1)
	account := zhipuNativeResponsesAccount()
	account.Extra = map[string]any{
		openai_compat.ExtraKeyResponsesMode:      string(openai_compat.ResponsesSupportModeForceResponses),
		openai_compat.ExtraKeyResponsesSupported: true,
	}
	repo := &snapshotUpdateAccountRepo{
		stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{*account}},
		updateExtraCalls:      updateCalls,
	}
	svc := &AccountTestService{accountRepo: repo}

	svc.ProbeOpenAIAPIKeyResponsesSupport(context.Background(), account.ID)

	updates := <-updateCalls
	require.Equal(t, true, updates[openai_compat.ExtraKeyResponsesSupported])
	require.Equal(t, string(openai_compat.ResponsesSupportModeForceResponses), updates[openai_compat.ExtraKeyResponsesMode])
}

// 回归护栏：adaptive 模式下智谱仍然回退 Chat Completions，不随本次改动变化。
func TestForwardResponses_ZhipuAdaptiveStillFallsBackToChatCompletions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"glm-5.3-flash","input":"hello","stream":true}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_zhipu_adaptive"}},
		Body:       io.NopCloser(strings.NewReader(chatFallbackCompleteStream())),
	}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	account := zhipuNativeResponsesAccount()
	account.Credentials["api_protocol"] = APIProtocolAdaptive

	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Contains(t, upstream.lastReq.URL.String(), "/chat/completions")
	require.True(t, gjson.GetBytes(upstream.lastBody, "messages").Exists())
}
