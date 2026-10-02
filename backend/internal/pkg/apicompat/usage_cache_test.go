package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Client-visible cache counters after protocol conversion must equal what the
// raw accounting path bills, whichever spelling or nesting the upstream used.
func TestCacheUsageMatchesRawAcrossConversions(t *testing.T) {
	for _, tc := range []struct {
		name, counters string
		read, write    int
	}{
		{"relay-anthropic-spelling", `"cache_read_input_tokens":100,"cache_creation_input_tokens":200`, 100, 200},
		{"relay-write-input", `"cache_read_tokens":100,"cache_write_input_tokens":200`, 100, 200},
		{"relay-short", `"cached_tokens":100,"cache_write_tokens":200`, 100, 200},
		{"relay-creation", `"cache_read_input_tokens":100,"cache_creation_tokens":200`, 100, 200},
		{"root-write-first", `"cache_write_tokens":9,"cache_creation_input_tokens":19,"cache_write_input_tokens":29,"cache_creation_tokens":39`, 0, 9},
		{"root-first-positive", `"cache_write_tokens":0,"cache_creation_input_tokens":19,"cache_write_input_tokens":29,"cache_creation_tokens":39`, 0, 19},
		{"details-beat-roots", `"cache_read_input_tokens":100,"cache_write_tokens":200,"input_tokens_details":{"cached_tokens":7,"cache_write_tokens":9},"prompt_tokens_details":{"cached_tokens":7,"cache_write_tokens":9}`, 7, 9},
		{"detail-zero-beats-roots", `"cache_read_input_tokens":100,"cache_write_tokens":200,"input_tokens_details":{"cached_tokens":0,"cache_write_tokens":0},"prompt_tokens_details":{"cached_tokens":0,"cache_write_tokens":0}`, 0, 0},
		{"write-zero-beats-creation", `"input_tokens_details":{"cached_tokens":7,"cache_write_tokens":0,"cache_creation_tokens":19},"prompt_tokens_details":{"cached_tokens":7,"cache_write_tokens":0,"cache_creation_tokens":19}`, 7, 0},
		{"nested-write-beats-nested-creation", `"input_tokens_details":{"cached_tokens":7,"cache_creation_tokens":19},"prompt_tokens_details":{"cached_tokens":8,"cache_write_tokens":9}`, 7, 9},
		{"input-zero-beats-prompt", `"input_tokens_details":{"cached_tokens":0,"cache_write_tokens":0},"prompt_tokens_details":{"cached_tokens":8,"cache_write_tokens":9}`, 0, 0},
		{"roots-fill-absent-detail", `"cache_read_input_tokens":100,"cache_write_tokens":200,"input_tokens_details":{"audio_tokens":3},"prompt_tokens_details":{"audio_tokens":3}`, 100, 200},
		{"input-null-is-zero", `"cache_read_input_tokens":100,"cache_write_tokens":200,"input_tokens_details":{"cached_tokens":null,"cache_write_tokens":null},"prompt_tokens_details":{"cached_tokens":7,"cache_write_tokens":9}`, 0, 0},
		{"prompt-null-is-zero", `"cache_read_input_tokens":100,"cache_write_tokens":200,"prompt_tokens_details":{"cached_tokens":null,"cache_write_tokens":null}`, 0, 0},
		{"creation-null-is-zero", `"cache_read_input_tokens":100,"cache_write_tokens":200,"prompt_tokens_details":{"cache_creation_tokens":null}`, 100, 0},
		{"nested-negative-clamped", `"prompt_tokens_details":{"cache_write_tokens":-4}`, 0, 0},
	} {
		for shape, wire := range map[string]string{
			"chat-wire":      `{"prompt_tokens":1000,"completion_tokens":50,` + tc.counters + `}`,
			"responses-wire": `{"input_tokens":1000,"output_tokens":50,` + tc.counters + `}`,
		} {
			t.Run(tc.name+"/"+shape, func(t *testing.T) {
				raw := gjson.Parse(wire)
				rawRead, _ := CachedTokensFromUsage(raw)
				require.Equal(t, tc.read, rawRead, "billed cache read")
				require.Equal(t, tc.write, CacheCreationTokensFromUsage(raw), "billed cache write")

				visible := func(usage AnthropicUsage, path string) {
					t.Helper()
					require.Equal(t, tc.read, usage.CacheReadInputTokens, path)
					require.Equal(t, tc.write, usage.CacheCreationInputTokens, path)
					require.Equal(t, 1000-tc.read-tc.write, usage.InputTokens, path)
				}
				if shape == "chat-wire" {
					var chat ChatUsage
					require.NoError(t, json.Unmarshal([]byte(wire), &chat))
					visible(chatUsageToAnthropicUsage(&chat), "chat->anthropic")
					visible(anthropicUsageFromResponsesUsage(ChatUsageToResponsesUsage(&chat)), "chat->responses->anthropic")
					visible(chatUsageToAnthropicUsage(chatUsageFromResponsesUsage(ChatUsageToResponsesUsage(&chat))), "chat->responses->chat->anthropic")
				}

				var responses ResponsesUsage
				require.NoError(t, json.Unmarshal([]byte(wire), &responses))
				visible(anthropicUsageFromResponsesUsage(&responses), "responses->anthropic")
				visible(chatUsageToAnthropicUsage(chatUsageFromResponsesUsage(&responses)), "responses->chat->anthropic")
				visible(anthropicUsageFromResponsesUsage(ChatUsageToResponsesUsage(chatUsageFromResponsesUsage(&responses))), "responses->chat->responses->anthropic")
			})
		}
	}
}

// A Chat upstream that reports cache writes only at the top level must not
// reach Responses or Anthropic clients as zero cache writes.
func TestChatTopLevelCacheWriteReachesConvertedClients(t *testing.T) {
	const usage = `{"prompt_tokens":1000,"completion_tokens":5,"total_tokens":1005,"cache_creation_input_tokens":800}`
	var resp ChatCompletionsResponse
	require.NoError(t, json.Unmarshal([]byte(`{"id":"chatcmpl-1","object":"chat.completion","model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":`+usage+`}`), &resp))

	responsesWire, err := json.Marshal(ChatCompletionsResponseToResponses(&resp, "m", nil, nil, false, nil))
	require.NoError(t, err)
	require.Equal(t, int64(800), gjson.GetBytes(responsesWire, "usage.cache_creation_input_tokens").Int(), string(responsesWire))
	anthropic := ChatCompletionsResponseToAnthropic(&resp, "m")
	require.Equal(t, 800, anthropic.Usage.CacheCreationInputTokens)
	require.Equal(t, 200, anthropic.Usage.InputTokens)

	var chunk ChatCompletionsChunk
	require.NoError(t, json.Unmarshal([]byte(`{"id":"chatcmpl-1","object":"chat.completion.chunk","model":"m","choices":[],"usage":`+usage+`}`), &chunk))
	responsesState := NewChatCompletionsToResponsesStreamState("m")
	ChatCompletionsChunkToResponsesEvents(&chunk, responsesState)
	streamed, err := json.Marshal(FinalizeChatCompletionsResponsesStream(responsesState))
	require.NoError(t, err)
	require.Contains(t, gjson.GetBytes(streamed, "#.response.usage.cache_creation_input_tokens").Raw, "800", string(streamed))
	anthropicState := NewChatCompletionsToAnthropicStreamState("m")
	ChatCompletionsChunkToAnthropicEvents(&chunk, anthropicState)
	streamed, err = json.Marshal(FinalizeChatCompletionsAnthropicStream(anthropicState))
	require.NoError(t, err)
	require.Contains(t, gjson.GetBytes(streamed, "#.usage.cache_creation_input_tokens").Raw, "800", string(streamed))
}

// Normalizing cache counters keeps exactly one spelling of the winning counter
// and leaves every non-cache detail as the upstream sent it.
func TestChatUsageCacheWriteKeepsWinningSpelling(t *testing.T) {
	var root ChatUsage
	require.NoError(t, json.Unmarshal([]byte(`{"prompt_tokens":1000,"completion_tokens":50,"cache_read_input_tokens":100,"cache_creation_input_tokens":200,"prompt_tokens_details":{"audio_tokens":3},"completion_tokens_details":{"reasoning_tokens":17,"audio_tokens":4,"accepted_prediction_tokens":5,"rejected_prediction_tokens":6}}`), &root))
	require.Zero(t, root.TotalTokens)
	require.Equal(t, &ChatTokenDetails{CachedTokens: 100, CacheCreationTokens: 200, AudioTokens: 3}, root.PromptTokensDetails)
	require.Equal(t, &ChatTokenDetails{ReasoningTokens: 17, AudioTokens: 4, AcceptedPredictionTokens: 5, RejectedPredictionTokens: 6}, root.CompletionTokensDetails)

	var nested ChatUsage
	require.NoError(t, json.Unmarshal([]byte(`{"prompt_tokens":1000,"input_tokens_details":{"cache_creation_tokens":19},"prompt_tokens_details":{"cache_write_tokens":9}}`), &nested))
	require.Equal(t, &ChatTokenDetails{CacheWriteTokens: 9}, nested.PromptTokensDetails)

	var zero ChatUsage
	require.NoError(t, json.Unmarshal([]byte(`{"prompt_tokens":1000,"prompt_tokens_details":{"cache_write_tokens":0,"cache_creation_tokens":19}}`), &zero))
	require.Equal(t, &ChatTokenDetails{}, zero.PromptTokensDetails)

	var absent ChatUsage
	require.NoError(t, json.Unmarshal([]byte(`{"prompt_tokens":1000,"cache_write_tokens":0,"cache_read_tokens":0}`), &absent))
	require.Nil(t, absent.PromptTokensDetails)
}
