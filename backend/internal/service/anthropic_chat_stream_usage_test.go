//go:build unit

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAnthropicChatStreamAuthoritativeUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, adapter := range []string{"anthropic", "native"} {
		for _, options := range []string{"", `,"stream_options":{"include_usage":true}`, `,"stream_options":{"include_usage":false}`} {
			for _, tc := range []struct {
				name, start, delta              string
				want                            bool
				prompt, output, cached, created int
			}{
				{"cache", `,"usage":{"input_tokens":308,"cache_read_input_tokens":241,"cache_creation_input_tokens":17}`, `,"usage":{"output_tokens":49}`, true, 566, 49, 241, 17},
				{"zero_start", `,"usage":{"input_tokens":0,"output_tokens":0}`, "", true, 0, 0, 0, 0},
				{"zero_delta", "", `,"usage":{"input_tokens":0,"output_tokens":0}`, true, 0, 0, 0, 0},
				{"absent", "", "", false, 0, 0, 0, 0},
				{"null", `,"usage":null`, `,"usage":null`, false, 0, 0, 0, 0},
			} {
				for _, terminal := range []string{"stop", "eof"} {
					t.Run(adapter+"/"+options+"/"+tc.name+"/"+terminal, func(t *testing.T) {
						sse := fmt.Sprintf("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"k3\",\"content\":[]%s}}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}%s}\n\n", tc.start, tc.delta)
						if terminal == "stop" {
							// Repeated terminal events and EOF must not duplicate usage.
							sse += "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
						}
						upstream := &httpUpstreamRecorder{resp: &http.Response{
							StatusCode: http.StatusOK,
							Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
							Body:       io.NopCloser(strings.NewReader(sse)),
						}}
						body := []byte(`{"model":"k3","messages":[{"role":"user","content":"hi"}],"stream":true` + options + "}")
						rec := httptest.NewRecorder()
						c, _ := gin.CreateTestContext(rec)
						c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(body)))
						if adapter == "native" {
							svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
							result, err := svc.forwardChatCompletionsViaNativeAnthropic(context.Background(), c, nativeAnthropicTestAccount(), body, "")
							require.NoError(t, err)
							require.Equal(t, tc.output, result.Usage.OutputTokens)
							require.Equal(t, tc.cached, result.Usage.CacheReadInputTokens)
							require.Equal(t, tc.created, result.Usage.CacheCreationInputTokens)
						} else {
							svc := &GatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
							account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-test"}}
							result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, nil)
							require.NoError(t, err)
							require.Equal(t, tc.output, result.Usage.OutputTokens)
							require.Equal(t, tc.cached, result.Usage.CacheReadInputTokens)
							require.Equal(t, tc.created, result.Usage.CacheCreationInputTokens)
						}
						count, done := 0, 0
						var last *apicompat.ChatCompletionsChunk
						for _, line := range strings.Split(rec.Body.String(), "\n") {
							if !strings.HasPrefix(line, "data: ") {
								continue
							}
							payload := strings.TrimPrefix(line, "data: ")
							if payload == "[DONE]" {
								done++
								if tc.want {
									require.NotNil(t, last)
									require.NotNil(t, last.Usage)
								}
								continue
							}
							require.Zero(t, done, "chunk after DONE")
							var chunk apicompat.ChatCompletionsChunk
							require.NoError(t, json.Unmarshal([]byte(payload), &chunk))
							last = &chunk
							if chunk.Usage == nil {
								continue
							}
							count++
							require.Empty(t, chunk.Choices)
							require.Equal(t, tc.prompt, chunk.Usage.PromptTokens)
							require.Equal(t, tc.output, chunk.Usage.CompletionTokens)
							require.Equal(t, tc.prompt+tc.output, chunk.Usage.TotalTokens)
							if tc.cached > 0 {
								require.NotNil(t, chunk.Usage.PromptTokensDetails)
								require.Equal(t, tc.cached, chunk.Usage.PromptTokensDetails.CachedTokens)
								require.Equal(t, tc.created, chunk.Usage.PromptTokensDetails.CacheCreationTokens)
							}
						}
						require.Equal(t, 1, done)
						if tc.want {
							require.Equal(t, 1, count)
						} else {
							require.Zero(t, count)
						}
					})
				}
			}
		}
	}
}
