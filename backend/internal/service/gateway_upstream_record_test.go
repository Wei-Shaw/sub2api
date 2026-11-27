//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// composite 组 inbound=/v1/chat/completions 经 CC→Anthropic 桥打到 Anthropic
// 上游 404（非 failover 分支，无 ForwardResult）：buildUpstreamRequest 发送前
// 记录的实际出站端点与最终映射模型必须保留，供错误日志如实报告。
func TestForwardAsChatCompletionsRetainsActualUpstreamMetadataOn404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{{
		StatusCode: http.StatusNotFound,
		Header:     http.Header{"X-Request-Id": []string{"nf-404"}},
		Body:       io.NopCloser(http.NoBody),
	}}}
	svc := &GatewayService{
		cfg:                 &config.Config{},
		httpUpstream:        upstream,
		tlsFPProfileService: &TLSFingerprintProfileService{},
	}
	body := []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hello"}]}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	account := &Account{
		ID:          7,
		Name:        "cc-bridge-anthropic",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "test-key"},
	}

	_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "404")

	// 实际出站 path（不含 ?beta=true query）与最终映射模型都被保留。
	require.Equal(t, "/v1/messages", GetOpsUpstreamEndpoint(c))
	require.Equal(t, "claude-sonnet-4-5", c.GetString(OpsUpstreamModelKey))
	require.Equal(t, 1, upstream.callCount)
}

// adaptive/anthropic 协议账号（composite 兼容池常见）：/v1/messages 入站零转换
// 直通供应商原生 /v1/messages。传输层失败也要保留真实端点与映射模型，不得
// 报成按平台推导的 /v1/responses。
func TestForwardAsAnthropicNativeBridgeRecordsActualPathAndModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{err: errors.New("stop after capture")}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	account := adaptiveProtocolTestAccount(PlatformDeepseek, map[string]any{
		APIProtocolAnthropic: "http://anthropic.example",
	})
	body := []byte(`{"model":"deepseek-chat","max_tokens":32,"messages":[{"role":"user","content":"hello"}],"stream":false}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	_, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")
	require.Error(t, err)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "http://anthropic.example/v1/messages", upstream.lastReq.URL.String())

	require.Equal(t, "/v1/messages", GetOpsUpstreamEndpoint(c))
	require.Equal(t, normalizeOpenAIModelForUpstream(account, "deepseek-chat"), c.GetString(OpsUpstreamModelKey))
}

// 固定 chat_completions 协议的 Messages 入站回退桥：端点由共享 CC 管线记录为
// /v1/chat/completions（实际 CC 保持 CC），模型在发送前记录最终映射值。
func TestForwardAsAnthropicRawCCBridgeRecordsMappedModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{err: errors.New("stop after capture")}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	// 注意：固定 chat_completions 协议（非 adaptive）才会走 raw CC 回退桥；
	// adaptive 账号的 /v1/messages 永远直通原生 Anthropic 端点。
	account := &Account{
		ID:          702,
		Name:        "cc-only-cn",
		Platform:    PlatformDeepseek,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":      "sk-test",
			"api_protocol": APIProtocolChatCompletions,
			"account_mode": AccountModePayG,
			"base_url":     "http://chat.example",
		},
	}
	body := []byte(`{"model":"deepseek-chat","max_tokens":32,"messages":[{"role":"user","content":"hello"}],"stream":false}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	_, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")
	require.Error(t, err)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "http://chat.example/v1/chat/completions", upstream.lastReq.URL.String())

	require.Equal(t, "/v1/chat/completions", GetActualOpenAIUpstreamEndpoint(c))
	require.Equal(t, normalizeOpenAIModelForUpstream(account, "deepseek-chat"), c.GetString(OpsUpstreamModelKey))
}
