//go:build unit

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Both first requests with long tool histories and later turns sharing a session
// must retain the original task. A successful response ID is not stored history.
func TestForwardAsAnthropic_StatelessToolHistory(t *testing.T) {
	for _, platform := range []string{PlatformOpenCodeGo, PlatformOpenAI} {
		for _, baseURL := range []string{"https://opencode.ai/zen/go/v1", "https://api.openai.com/v1"} {
			if platform == PlatformOpenCodeGo && baseURL == "https://api.openai.com/v1" {
				continue
			}
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/stream=%t", platform, baseURL, stream), func(t *testing.T) {
					account := &Account{ID: 1, Platform: platform, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{"api_key": "sk-test", "base_url": baseURL}}
					upstream := &httpUpstreamRecorder{}
					svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
					messages := []apicompat.AnthropicMessage{{Role: "user", Content: json.RawMessage(`"EARLY_REQUIREMENT_KEEP_EXISTING_CONFIG"`)}}
					for i := 0; i < 7; i++ {
						messages = append(messages,
							apicompat.AnthropicMessage{Role: "assistant", Content: json.RawMessage(fmt.Sprintf(`[{"type":"tool_use","id":"call_%d","name":"Read","input":{"file_path":"file_%d.go"}}]`, i, i))},
							apicompat.AnthropicMessage{Role: "user", Content: json.RawMessage(fmt.Sprintf(`[{"type":"tool_result","tool_use_id":"call_%d","content":"file_%d contents"}]`, i, i))})
					}
					for turn := 0; turn < 2; turn++ {
						body, err := json.Marshal(&apicompat.AnthropicRequest{Model: "gpt-6-luna", MaxTokens: 128, Stream: stream, Messages: messages, System: json.RawMessage(`"SYSTEM_REQUIREMENT_KEEP_ME"`), Metadata: json.RawMessage(`{"user_id":"{\"device_id\":\"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\",\"session_id\":\"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa\"}"}`), Tools: []apicompat.AnthropicTool{{Name: "Read", InputSchema: json.RawMessage(`{"type":"object","properties":{"file_path":{"type":"string"}}}`)}}})
						require.NoError(t, err)
						upstream.resp = openAICompatSSECompletedResponse(fmt.Sprintf("resp_%d", turn), "gpt-6-luna")
						c := adaptiveProtocolTestContext("/v1/messages", body)
						_, err = svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")
						require.NoError(t, err)
						sent := upstream.lastBody
						require.Contains(t, string(sent), "EARLY_REQUIREMENT_KEEP_EXISTING_CONFIG")
						require.Contains(t, string(sent), "SYSTEM_REQUIREMENT_KEEP_ME")
						require.Equal(t, "false", gjson.GetBytes(sent, "store").Raw)
						require.False(t, gjson.GetBytes(sent, "previous_response_id").Exists())
						require.NotEmpty(t, gjson.GetBytes(sent, "prompt_cache_key").String())
						require.Contains(t, gjson.GetBytes(sent, "prompt_cache_key").String(), "anthropic-metadata-")
						calls, outputs := 0, 0
						for _, item := range gjson.GetBytes(sent, "input").Array() {
							if item.Get("type").String() == "function_call" {
								require.Equal(t, fmt.Sprintf("call_%d", calls), item.Get("call_id").String())
								calls++
							}
							if item.Get("type").String() == "function_call_output" {
								require.Equal(t, fmt.Sprintf("call_%d", outputs), item.Get("call_id").String())
								outputs++
							}
						}
						require.Equal(t, 7, calls)
						require.Equal(t, 7, outputs)
						if turn > 0 {
							require.Equal(t, gjson.GetBytes(upstream.bodies[0], "prompt_cache_key").String(), gjson.GetBytes(sent, "prompt_cache_key").String())
						}
						messages = append(messages, apicompat.AnthropicMessage{Role: "assistant", Content: json.RawMessage(`"ok"`)}, apicompat.AnthropicMessage{Role: "user", Content: json.RawMessage(`"continue with the original requirement"`)})
					}
				})
			}
		}
	}
}
