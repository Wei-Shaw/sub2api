//go:build unit

package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/minimax"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Exercise the same forwarding methods used by the Messages and Chat handlers,
// with the final HTTP request captured before any network access.
func TestMiniMaxRolloutHTTPBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	endpoints := []struct{ name, saved, chat, messages string }{
		{"default", "", "https://api.minimax.io/v1/chat/completions", "https://api.minimax.io/anthropic/v1/messages"},
		{"global-anthropic", minimax.DefaultAnthropicBaseURL, "https://api.minimax.io/v1/chat/completions", "https://api.minimax.io/anthropic/v1/messages"},
		{"global-openai", minimax.DefaultOpenAIBaseURL, "https://api.minimax.io/v1/chat/completions", "https://api.minimax.io/anthropic/v1/messages"},
		{"cn-anthropic", minimax.CNAnthropicBaseURL, "https://api.minimaxi.com/v1/chat/completions", "https://api.minimaxi.com/anthropic/v1/messages"},
		{"cn-openai", minimax.CNOpenAIBaseURL, "https://api.minimaxi.com/v1/chat/completions", "https://api.minimaxi.com/anthropic/v1/messages"},
		{"custom", "https://proxy.example/custom", "https://proxy.example/custom/v1/chat/completions", "https://proxy.example/custom/v1/messages"},
	}
	for _, endpoint := range endpoints {
		for _, model := range []string{minimax.DefaultTestModel, "MiniMax-M3", "MiniMax-M2.7", "private-model"} {
			for _, effort := range []string{"", "low", "medium", "high", "xhigh", "max"} {
				t.Run(endpoint.name+"/"+model+"/"+effort, func(t *testing.T) {
					account := &Account{ID: 5174, Platform: PlatformMiniMax, Type: AccountTypeAPIKey,
						Credentials: map[string]any{"api_key": "fixture-key", "base_url": endpoint.saved}}
					for _, protocol := range []string{"chat", "messages"} {
						t.Run(protocol, func(t *testing.T) {
							payload := `{"model":%q,"stream":false,"messages":[{"role":"user","content":[{"type":"text","text":"describe"},{"type":"image_url","image_url":{"url":"https://example.com/image.png"}}]},{"role":"assistant","content":"prior answer","reasoning_content":"prior reasoning","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call_1","content":"prior result"}]%s}`
							extra := ""
							response := `{"id":"fixture","object":"chat.completion","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":3}}`
							wantURL := endpoint.chat
							if effort != "" {
								extra = fmt.Sprintf(`,"reasoning_effort":%q`, effort)
							}
							if protocol == "messages" {
								payload = `{"model":%q,"stream":false,"max_tokens":4096,"messages":[{"role":"user","content":[{"type":"text","text":"describe"},{"type":"image","source":{"type":"url","url":"https://example.com/image.png"}}]},{"role":"assistant","content":[{"type":"thinking","thinking":"prior reasoning","signature":"signature"},{"type":"tool_use","id":"call_1","name":"lookup","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":"prior result"}]}]%s}`
								extra = ""
								if effort != "" {
									extra = fmt.Sprintf(`,"thinking":{"type":"adaptive"},"output_config":{"effort":%q}`, effort)
								}
								response = `{"id":"fixture","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":7,"output_tokens":3}}`
								wantURL = endpoint.messages
							}
							body := []byte(fmt.Sprintf(payload, model, extra))
							c, _ := gin.CreateTestContext(httptest.NewRecorder())
							c.Request = httptest.NewRequest(http.MethodPost, "/v1/"+protocol, bytes.NewReader(body))
							upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(response))}}
							if protocol == "chat" {
								svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
								_, err := svc.ForwardMiniMaxChatCompletions(context.Background(), c, account, body, "")
								require.NoError(t, err)
							} else {
								svc := &GatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream, rateLimitService: &RateLimitService{}}
								_, err := svc.Forward(context.Background(), c, account, &ParsedRequest{Body: NewRequestBodyRef(body), Model: model})
								require.NoError(t, err)
							}
							require.NotNil(t, upstream.lastReq)
							require.Equal(t, wantURL, upstream.lastReq.URL.String())
							if protocol == "chat" {
								require.Equal(t, "Bearer fixture-key", upstream.lastReq.Header.Get("Authorization"))
							} else {
								require.Equal(t, "fixture-key", upstream.lastReq.Header.Get("x-api-key"))
							}
							require.JSONEq(t, string(body), string(upstream.lastBody))
							require.Equal(t, endpoint.saved, account.GetCredential("base_url"))
						})
					}
				})
			}
		}
	}
}
