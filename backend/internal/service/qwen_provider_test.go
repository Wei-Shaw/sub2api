//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestQwenProviderModesAndRouting(t *testing.T) {
	cases := []struct{ mode, chat, anthropic string }{
		{"token_cn", "https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1", "https://token-plan.cn-beijing.maas.aliyuncs.com/apps/anthropic"},
		{"token_sg", "https://token-plan.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1", "https://token-plan.ap-southeast-1.maas.aliyuncs.com/apps/anthropic"},
		{"coding_cn", "https://coding.dashscope.aliyuncs.com/v1", "https://coding.dashscope.aliyuncs.com/apps/anthropic"},
		{"coding_sg", "https://coding-intl.dashscope.aliyuncs.com/v1", "https://coding-intl.dashscope.aliyuncs.com/apps/anthropic"},
		{"payg_cn", "https://dashscope.aliyuncs.com/compatible-mode/v1", "https://dashscope.aliyuncs.com/apps/anthropic"},
		{"payg_sg", "https://dashscope-intl.aliyuncs.com/compatible-mode/v1", "https://dashscope-intl.aliyuncs.com/apps/anthropic"},
	}
	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			account := &Account{Platform: PlatformQwen, Type: AccountTypeAPIKey, Credentials: map[string]any{"account_mode": tc.mode, "api_protocol": APIProtocolAdaptive}}
			require.Equal(t, tc.chat, account.GetCNProtocolBaseURL(APIProtocolChatCompletions))
			require.Equal(t, tc.anthropic, account.GetCNProtocolBaseURL(APIProtocolAnthropic))
			require.Equal(t, APIProtocolChatCompletions, resolveUpstreamProtocol(account, APIProtocolResponses, "qwen3.6-plus", nil))
			require.Equal(t, APIProtocolAnthropic, resolveUpstreamProtocol(account, APIProtocolAnthropic, "qwen3.6-plus", nil))
			require.False(t, account.SupportsNativeCNResponses())
		})
	}
}

func TestQwenCustomEndpointsAndResponsesConversion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"qwen3.6-plus","input":"hello","max_output_tokens":32,"stream":false}`)
	upstream := &httpUpstreamRecorder{err: errors.New("stop after capture")}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	account := adaptiveProtocolTestAccount(PlatformQwen, map[string]any{APIProtocolChatCompletions: "http://custom.example/v1", APIProtocolAnthropic: "http://custom-anthropic.example"})
	account.Credentials["account_mode"] = "token_cn"
	_, err := svc.Forward(context.Background(), adaptiveProtocolTestContext("/v1/responses", body), account, body)
	require.Error(t, err)
	require.Equal(t, "http://custom.example/v1/chat/completions", upstream.lastReq.URL.String())
	require.True(t, gjson.GetBytes(upstream.lastBody, "messages").IsArray())
	require.Equal(t, "http://custom-anthropic.example", account.GetCNProtocolBaseURL(APIProtocolAnthropic))
}

func TestQwenNativeStreamingRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := adaptiveProtocolTestAccount(PlatformQwen, map[string]any{
		APIProtocolChatCompletions: "http://custom.example/v1",
		APIProtocolAnthropic:       "http://custom-anthropic.example/apps/anthropic",
	})
	account.Credentials["account_mode"] = "token_sg"
	t.Run("chat completions", func(t *testing.T) {
		body := []byte(`{"model":"qwen3.6-plus","messages":[{"role":"user","content":"hello"}],"stream":true}`)
		upstream := &httpUpstreamRecorder{err: errors.New("stop after capture")}
		svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
		_, err := svc.ForwardAsChatCompletions(context.Background(), adaptiveProtocolTestContext("/v1/chat/completions", body), account, body, "", "")
		require.Error(t, err)
		require.Equal(t, "http://custom.example/v1/chat/completions", upstream.lastReq.URL.String())
		require.True(t, gjson.GetBytes(upstream.lastBody, "stream").Bool())
		require.Equal(t, "qwen3.6-plus", gjson.GetBytes(upstream.lastBody, "model").String())
	})
	t.Run("anthropic", func(t *testing.T) {
		body := []byte(`{"model":"qwen3.6-plus","max_tokens":32,"messages":[{"role":"user","content":"hello"}],"stream":true}`)
		upstream := &httpUpstreamRecorder{err: errors.New("stop after capture")}
		svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
		_, err := svc.ForwardAsAnthropic(context.Background(), adaptiveProtocolTestContext("/v1/messages", body), account, body, "", "")
		require.Error(t, err)
		require.Equal(t, "http://custom-anthropic.example/apps/anthropic/v1/messages", upstream.lastReq.URL.String())
		require.True(t, gjson.GetBytes(upstream.lastBody, "stream").Bool())
	})
}
