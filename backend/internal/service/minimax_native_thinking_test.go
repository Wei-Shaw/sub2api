//go:build unit

package service

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestNativeAnthropicPassthroughNormalizesMiniMaxThinking(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name         string
		model        string
		mappedModel  string
		thinking     string
		stream       bool
		wantAdaptive bool
	}{
		{name: "m3 enabled buffered", model: "MiniMax-M3", thinking: `,"thinking":{"type":"enabled","budget_tokens":8192}`, wantAdaptive: true},
		{name: "m3 enabled streaming", model: "MiniMax-M3", thinking: `,"thinking":{"type":"enabled","budget_tokens":8192}`, stream: true, wantAdaptive: true},
		{name: "m2 enabled", model: "MiniMax-M2.7", thinking: `,"thinking":{"type":"enabled","budget_tokens":8192}`, wantAdaptive: true},
		{name: "mixed case model", model: "minimax-m3", thinking: `,"thinking":{"type":"enabled"}`, wantAdaptive: true},
		{name: "adaptive unchanged", model: "MiniMax-M3", thinking: `,"thinking":{"type":"adaptive","budget_tokens":8192}`},
		{name: "disabled unchanged", model: "MiniMax-M3", thinking: `,"thinking":{"type":"disabled"}`},
		{name: "omitted unchanged", model: "MiniMax-M3"},
		{name: "other model unchanged", model: "kimi-k2.6", thinking: `,"thinking":{"type":"enabled","budget_tokens":8192}`},
		{name: "alias mapped to minimax", model: "client-model", mappedModel: "MiniMax-M3", thinking: `,"thinking":{"type":"enabled"}`, wantAdaptive: true},
		{name: "minimax name mapped to other model", model: "MiniMax-M3", mappedModel: "kimi-k2.6", thinking: `,"thinking":{"type":"enabled"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"model":%q,"max_tokens":16384,"stream":%t%s,"output_config":{"effort":"high"},"messages":[{"role":"user","content":"hi"}]}`, tt.model, tt.stream, tt.thinking))
			account := nativeAnthropicTestAccount()
			account.Platform = PlatformMiniMax
			account.Credentials["base_url"] = "http://anthropic.example"
			if tt.mappedModel != "" {
				account.Credentials["model_mapping"] = map[string]any{tt.model: tt.mappedModel}
			}
			response := nativeAnthropicBufferedResponse()
			if tt.stream {
				response = nativeAnthropicStreamResponse()
			}
			upstream := &httpUpstreamRecorder{resp: response}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

			_, err := svc.ForwardAsAnthropic(context.Background(),
				adaptiveProtocolTestContext("/v1/messages", body), account, body, "", "")
			require.NoError(t, err)
			require.NotNil(t, upstream.lastReq)
			require.Equal(t, "http://anthropic.example/v1/messages", upstream.lastReq.URL.String())

			want := string(body)
			if tt.mappedModel != "" {
				want = strings.Replace(want, fmt.Sprintf(`"model":%q`, tt.model), fmt.Sprintf(`"model":%q`, tt.mappedModel), 1)
			}
			if tt.wantAdaptive {
				want = strings.Replace(want, `"type":"enabled"`, `"type":"adaptive"`, 1)
			}
			require.JSONEq(t, want, string(upstream.lastBody))
		})
	}
}

func TestNativeAnthropicBridgesNormalizeMiniMaxThinking(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []cnProtocolIngressCase{
		{
			name: "chat completions",
			path: "/v1/chat/completions",
			body: []byte(`{"model":"client-model","messages":[{"role":"user","content":"hi"}],"reasoning_effort":"high","stream":false}`),
			forward: func(svc *OpenAIGatewayService, c *gin.Context, account *Account, body []byte) error {
				_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
				return err
			},
		},
		{
			name: "responses",
			path: "/v1/responses",
			body: []byte(`{"model":"client-model","input":"hi","reasoning":{"effort":"high"},"stream":false}`),
			forward: func(svc *OpenAIGatewayService, c *gin.Context, account *Account, body []byte) error {
				_, err := svc.Forward(context.Background(), c, account, body)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := nativeAnthropicTestAccount()
			account.Platform = PlatformMiniMax
			account.Credentials["base_url"] = "http://anthropic.example"
			account.Credentials["model_mapping"] = map[string]any{"client-model": "MiniMax-M3"}
			upstream := &httpUpstreamRecorder{resp: nativeAnthropicStreamResponse()}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

			require.NoError(t, tt.forward(svc, adaptiveProtocolTestContext(tt.path, tt.body), account, tt.body))
			require.NotNil(t, upstream.lastReq)
			require.Equal(t, "http://anthropic.example/v1/messages", upstream.lastReq.URL.String())
			require.Equal(t, "MiniMax-M3", gjson.GetBytes(upstream.lastBody, "model").String())
			require.Equal(t, "adaptive", gjson.GetBytes(upstream.lastBody, "thinking.type").String())
			require.Equal(t, int64(10240), gjson.GetBytes(upstream.lastBody, "thinking.budget_tokens").Int())
			require.Equal(t, "high", gjson.GetBytes(upstream.lastBody, "output_config.effort").String())
		})
	}
}
