//go:build unit

package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestForwardAsRawChatCompletions_NormalizesMiniMaxThinking(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name         string
		model        string
		mappedModel  string
		thinking     string
		wantAdaptive bool
	}{
		{name: "m3 enabled", model: "MiniMax-M3", thinking: `,"thinking":{"type":"enabled","budget_tokens":8192}`, wantAdaptive: true},
		{name: "m3.1 enabled", model: "MiniMax-M3.1-Flash-Preview", thinking: `,"thinking":{"type":"enabled","budget_tokens":8192}`, wantAdaptive: true},
		{name: "m2 enabled", model: "MiniMax-M2.7", thinking: `,"thinking":{"type":"enabled"}`, wantAdaptive: true},
		{name: "mixed case model", model: "minimax-m3", thinking: `,"thinking":{"type":"enabled"}`, wantAdaptive: true},
		{name: "adaptive unchanged", model: "MiniMax-M3", thinking: `,"thinking":{"type":"adaptive"}`},
		{name: "disabled unchanged", model: "MiniMax-M3", thinking: `,"thinking":{"type":"disabled"}`},
		{name: "omitted unchanged", model: "MiniMax-M3"},
		{name: "other model unchanged", model: "kimi-k2.6", thinking: `,"thinking":{"type":"enabled"}`},
		{name: "alias mapped to minimax", model: "client-model", mappedModel: "MiniMax-M3", thinking: `,"thinking":{"type":"enabled"}`, wantAdaptive: true},
		{name: "minimax name mapped to other model", model: "MiniMax-M3", mappedModel: "kimi-k2.6", thinking: `,"thinking":{"type":"enabled"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"hi"}],"stream":false%s,"reasoning_effort":"max"}`, tt.model, tt.thinking))
			account := rawChatCompletionsTestAccount()
			account.Platform = PlatformMiniMax
			account.Credentials["api_protocol"] = APIProtocolChatCompletions
			account.Credentials["base_url"] = "http://chat.example"
			if tt.mappedModel != "" {
				account.Credentials["model_mapping"] = map[string]any{tt.model: tt.mappedModel}
			}
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(
					`{"id":"chatcmpl_1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":9,"completion_tokens":3,"total_tokens":12}}`,
				)),
			}}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

			_, err := svc.ForwardAsChatCompletions(context.Background(),
				adaptiveProtocolTestContext("/v1/chat/completions", body), account, body, "", "")
			require.NoError(t, err)
			require.NotNil(t, upstream.lastReq)
			require.Equal(t, "http://chat.example/v1/chat/completions", upstream.lastReq.URL.String())

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
