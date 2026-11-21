package apicompat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChatCacheWritePresenceAcrossAnthropicConverters(t *testing.T) {
	for _, test := range []struct {
		name     string
		details  string
		creation int
	}{
		{"missing write falls back", `"cache_creation_tokens":4`, 4},
		{"zero write is authoritative", `"cache_write_tokens":0,"cache_creation_tokens":4`, 0},
		{"null write is authoritative", `"cache_write_tokens":null,"cache_creation_tokens":4`, 0},
		{"negative write clamps", `"cache_write_tokens":-2,"cache_creation_tokens":4`, 0},
		{"positive write wins", `"cache_write_tokens":2,"cache_creation_tokens":4`, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			var response ChatCompletionsResponse
			payload := `{"id":"chat_cache","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"prompt_tokens_details":{"cached_tokens":3,` + test.details + `}}}`
			require.NoError(t, json.Unmarshal([]byte(payload), &response))
			encoded, err := json.Marshal(response)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(encoded, &response))
			if strings.Contains(test.details, `"cache_write_tokens":0`) {
				require.Contains(t, string(encoded), `"cache_write_tokens":0`)
			}
			assertUsage := func(usage AnthropicUsage) {
				require.Equal(t, 7-test.creation, usage.InputTokens)
				require.Equal(t, 2, usage.OutputTokens)
				require.Equal(t, 3, usage.CacheReadInputTokens)
				require.Equal(t, test.creation, usage.CacheCreationInputTokens)
				require.True(t, usage.CacheCreationInputTokensPresent)
				require.True(t, usage.CacheReadInputTokensPresent)
				require.Nil(t, usage.CacheCreation)
			}
			assertUsage(ChatCompletionsResponseToAnthropic(&response, "claude-sonnet-4").Usage)
			converted := ChatCompletionsResponseToResponses(&response, "claude-sonnet-4", nil, nil, false, nil)
			encoded, err = json.Marshal(converted)
			require.NoError(t, err)
			var decoded ResponsesResponse
			require.NoError(t, json.Unmarshal(encoded, &decoded))
			assertUsage(ResponsesToAnthropic(&decoded, "claude-sonnet-4").Usage)
			for _, adapter := range []bool{false, true} {
				state := NewChatCompletionsToAnthropicStreamState("claude-sonnet-4")
				var events []AnthropicStreamEvent
				chunk := &ChatCompletionsChunk{ID: "chat_cache", Usage: response.Usage}
				if adapter {
					responsesState := NewChatCompletionsToResponsesStreamState("claude-sonnet-4")
					anthropicState := NewResponsesEventToAnthropicState()
					responseEvents := ChatCompletionsChunkToResponsesEvents(chunk, responsesState)
					responseEvents = append(responseEvents, FinalizeChatCompletionsResponsesStream(responsesState)...)
					for _, event := range responseEvents {
						events = append(events, ResponsesEventToAnthropicEvents(&event, anthropicState)...)
					}
					events = append(events, FinalizeResponsesAnthropicStream(anthropicState)...)
				} else {
					events = ChatCompletionsChunkToAnthropicEvents(chunk, state)
					events = append(events, FinalizeChatCompletionsAnthropicStream(state)...)
				}
				deltas := 0
				for _, event := range events {
					if event.Type == "message_delta" {
						require.NotNil(t, event.Usage)
						assertUsage(*event.Usage)
						deltas++
					}
				}
				require.Equal(t, 1, deltas)
			}
		})
	}
}

func TestAnthropicCacheCreationAggregatePresence(t *testing.T) {
	for _, test := range []struct {
		name     string
		snapshot string
		want     int
		present  bool
	}{
		{"contradictory aggregate", `{"cache_creation_input_tokens":100,"cache_creation":{"ephemeral_1h_input_tokens":200}}`, 100, true},
		{"explicit zero", `{"cache_creation_input_tokens":0,"cache_creation":{"ephemeral_1h_input_tokens":200}}`, 200, true},
		{"missing aggregate", `{"cache_creation":{"ephemeral_1h_input_tokens":200}}`, 200, false},
		{"null aggregate", `{"cache_creation_input_tokens":null,"cache_creation":{"ephemeral_1h_input_tokens":200}}`, 200, false},
		{"null TTL", `{"cache_creation":{"ephemeral_5m_input_tokens":null,"ephemeral_1h_input_tokens":200}}`, 200, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var usage AnthropicUsage
			require.NoError(t, json.Unmarshal([]byte(test.snapshot), &usage))
			require.Equal(t, test.present, usage.HasCacheCreationInputTokens())
			response := AnthropicToResponsesResponse(&AnthropicResponse{Usage: usage})
			require.Equal(t, test.want, response.Usage.CacheCreationInputTokens)
			require.Equal(t, test.want, response.Usage.InputTokens)
			state := NewAnthropicEventToResponsesState()
			AnthropicEventToResponsesEvents(&AnthropicStreamEvent{
				Type: "message_start", Message: &AnthropicResponse{ID: "msg_cache", Usage: usage},
			}, state)
			for range 2 {
				var delta AnthropicUsage
				require.NoError(t, json.Unmarshal([]byte(`{"cache_creation":{"ephemeral_1h_input_tokens":200}}`), &delta))
				AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "message_delta", Usage: &delta}, state)
				require.Equal(t, test.want, state.CacheCreationInputTokens)
			}
			events := AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "message_stop"}, state)
			require.NotEmpty(t, events)
			completed := events[len(events)-1].Response
			require.NotNil(t, completed)
			require.Equal(t, test.want, completed.Usage.CacheCreationInputTokens)
		})
	}
}

func TestAnthropicUsageJSONRoundtrip(t *testing.T) {
	for _, test := range []struct {
		name    string
		payload string
		present bool
		value   int
	}{
		{"absent", `{"cache_creation":{"ephemeral_1h_input_tokens":30}}`, false, 0},
		{"null", `{"cache_creation_input_tokens":null,"cache_creation":{"ephemeral_1h_input_tokens":30}}`, false, 0},
		{"zero", `{"cache_creation_input_tokens":0,"cache_creation":{"ephemeral_1h_input_tokens":30}}`, true, 0},
		{"positive", `{"cache_creation_input_tokens":20,"cache_creation":{"ephemeral_1h_input_tokens":30}}`, true, 20},
	} {
		t.Run(test.name, func(t *testing.T) {
			var usage AnthropicUsage
			require.NoError(t, json.Unmarshal([]byte(test.payload), &usage))
			for range 2 {
				encoded, err := json.Marshal(usage)
				require.NoError(t, err)
				var fields map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(encoded, &fields))
				_, present := fields["cache_creation_input_tokens"]
				require.Equal(t, test.present, present)
				var roundtrip AnthropicUsage
				require.NoError(t, json.Unmarshal(encoded, &roundtrip))
				require.Equal(t, test.present, roundtrip.HasCacheCreationInputTokens())
				require.Equal(t, test.value, roundtrip.CacheCreationInputTokens)
				require.Equal(t, usage.CacheCreation, roundtrip.CacheCreation)
				usage = roundtrip
			}
		})
	}
}

func TestAnthropicUsageSyntheticJSONRoundtrip(t *testing.T) {
	for _, test := range []struct {
		name    string
		usage   AnthropicUsage
		present bool
	}{
		{"missing", AnthropicUsage{}, false},
		{"inferred nonzero", AnthropicUsage{CacheCreationInputTokens: 25}, true},
		{"explicit zero", AnthropicUsage{CacheCreationInputTokensPresent: true}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.usage.InputTokens = 10
			test.usage.OutputTokens = 7
			test.usage.CacheReadInputTokens = 3
			encoded, err := json.Marshal(test.usage)
			require.NoError(t, err)
			var roundtrip AnthropicUsage
			require.NoError(t, json.Unmarshal(encoded, &roundtrip))
			require.Equal(t, test.present, roundtrip.HasCacheCreationInputTokens())
			require.Equal(t, test.usage.CacheCreationInputTokens, roundtrip.CacheCreationInputTokens)
			require.Equal(t, 10, roundtrip.InputTokens)
			require.Equal(t, 7, roundtrip.OutputTokens)
			require.Equal(t, 3, roundtrip.CacheReadInputTokens)
		})
	}
}

func TestAnthropicUsageAdapterZeroContracts(t *testing.T) {
	for _, test := range []struct {
		name    string
		usage   AnthropicUsage
		present bool
	}{
		{"missing Responses usage", anthropicUsageFromResponsesUsage(nil), true},
		{"explicit Responses totals", anthropicUsageFromResponsesUsage(&ResponsesUsage{}), true},
		{"missing Chat usage", chatUsageToAnthropicUsage(nil), true},
		{"explicit Chat totals", chatUsageToAnthropicUsage(&ChatUsage{}), true},
	} {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := json.Marshal(test.usage)
			require.NoError(t, err)
			var decoded AnthropicUsage
			require.NoError(t, json.Unmarshal(encoded, &decoded))
			require.Equal(t, test.present, decoded.HasCacheCreationInputTokens())
			require.Zero(t, decoded.CacheCreationInputTokens)
		})
	}
	responsesState := NewResponsesEventToAnthropicState()
	startEvents := resToAnthHandleCreated(&ResponsesStreamEvent{}, responsesState)
	chatState := NewChatCompletionsToAnthropicStreamState("claude-sonnet-4")
	startEvents = append(startEvents, ensureCCAnthropicMessageStart(chatState)...)
	for _, event := range startEvents {
		encoded, err := json.Marshal(event)
		require.NoError(t, err)
		var decoded AnthropicStreamEvent
		require.NoError(t, json.Unmarshal(encoded, &decoded))
		require.NotNil(t, decoded.Message)
		require.True(t, decoded.Message.Usage.HasCacheCreationInputTokens())
		require.True(t, decoded.Message.Usage.CacheReadInputTokensPresent)
	}
	terminalEvents := FinalizeResponsesAnthropicStream(responsesState)
	terminalEvents = append(terminalEvents, FinalizeChatCompletionsAnthropicStream(chatState)...)
	usageEvents := 0
	for _, event := range terminalEvents {
		if event.Usage == nil {
			continue
		}
		encoded, err := json.Marshal(event)
		require.NoError(t, err)
		var decoded AnthropicStreamEvent
		require.NoError(t, json.Unmarshal(encoded, &decoded))
		require.NotNil(t, decoded.Usage)
		require.True(t, decoded.Usage.HasCacheCreationInputTokens())
		require.Zero(t, decoded.Usage.CacheCreationInputTokens)
		usageEvents++
	}
	require.Equal(t, 2, usageEvents)
}

func TestAnthropicCacheCreationPartialSnapshots(t *testing.T) {
	state := NewAnthropicEventToResponsesState()
	for index, snapshot := range []string{
		`{"cache_creation":{"ephemeral_5m_input_tokens":20}}`,
		`{"cache_creation":{"ephemeral_1h_input_tokens":30}}`,
		`{"cache_creation_input_tokens":null,"cache_creation":{"ephemeral_5m_input_tokens":null,"ephemeral_1h_input_tokens":null}}`,
		`{"cache_creation":{"ephemeral_1h_input_tokens":30}}`,
		`{"cache_creation_input_tokens":40}`,
		`{"output_tokens":1}`,
		`{"cache_creation_input_tokens":0}`,
		`{"cache_creation":{"ephemeral_1h_input_tokens":200}}`,
		`{"cache_creation_input_tokens":null,"cache_creation":{"ephemeral_1h_input_tokens":null}}`,
		`{"cache_creation_input_tokens":100}`,
	} {
		var usage AnthropicUsage
		require.NoError(t, json.Unmarshal([]byte(snapshot), &usage))
		state.mergeCacheCreationUsage(usage)
		require.Equal(t, []int{20, 50, 50, 50, 40, 40, 40, 40, 40, 100}[index], state.CacheCreationInputTokens)
	}
}

func TestAnthropicConvertersRawCacheUsageLifecycle(t *testing.T) {
	for _, test := range []struct {
		name      string
		aggregate int
		want      []int
	}{
		{"zero aggregate derives partial TTL", 0, []int{20, 50, 50, 50, 30}},
		{"positive aggregate survives placeholders", 40, []int{40, 40, 40, 40, 40}},
	} {
		for _, terminal := range []string{"message_stop", "eof"} {
			t.Run(test.name+"/"+terminal, func(t *testing.T) {
				state := NewAnthropicEventToResponsesState()
				chatState := NewResponsesEventToChatState()
				chatState.IncludeUsage = true
				var completed *ResponsesResponse
				var chatUsage *ChatUsage
				consume := func(events []ResponsesStreamEvent) {
					for _, event := range events {
						if event.Type == "response.completed" {
							completed = event.Response
						}
						for _, chunk := range ResponsesEventToChatChunks(&event, chatState) {
							if chunk.Usage != nil {
								chatUsage = chunk.Usage
							}
						}
					}
				}
				start := AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{
					ID: "msg_cache", Model: "claude-sonnet-4", Usage: AnthropicUsage{
						InputTokens: 10, CacheReadInputTokens: 9,
						CacheCreationInputTokens: test.aggregate, CacheCreationInputTokensPresent: true,
					},
				}}
				consume(AnthropicEventToResponsesEvents(&start, state))
				for index, snapshot := range []string{
					`{"output_tokens":7,"cache_creation_input_tokens":0,"cache_creation":{"ephemeral_5m_input_tokens":20}}`,
					`{"cache_creation_input_tokens":0,"cache_creation":{"ephemeral_1h_input_tokens":30}}`,
					`{"input_tokens":0,"output_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}`,
					`{"input_tokens":0,"output_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}`,
					`{"cache_creation":{"ephemeral_5m_input_tokens":0}}`,
				} {
					var event AnthropicStreamEvent
					require.NoError(t, json.Unmarshal([]byte(`{"type":"message_delta","usage":`+snapshot+`}`), &event))
					consume(AnthropicEventToResponsesEvents(&event, state))
					require.Equal(t, test.want[index], state.CacheCreationInputTokens, "snapshot %d", index)
					require.Equal(t, 7, state.OutputTokens, "snapshot %d", index)
				}
				if terminal == "message_stop" {
					consume(AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "message_stop"}, state))
				} else {
					consume(FinalizeAnthropicResponsesStream(state))
				}
				want := test.want[len(test.want)-1]
				require.NotNil(t, completed)
				require.NotNil(t, completed.Usage)
				require.Equal(t, want, completed.Usage.CacheCreationInputTokens)
				require.Equal(t, 19+want, completed.Usage.InputTokens)
				require.Equal(t, 7, completed.Usage.OutputTokens)
				require.Equal(t, 26+want, completed.Usage.TotalTokens)
				require.NotNil(t, chatUsage)
				require.Equal(t, 19+want, chatUsage.PromptTokens)
				require.Equal(t, 7, chatUsage.CompletionTokens)
				require.NotNil(t, chatUsage.PromptTokensDetails)
				require.Equal(t, want, chatUsage.PromptTokensDetails.CacheCreationTokens)
				require.Equal(t, 9, chatUsage.PromptTokensDetails.CachedTokens)
				require.Empty(t, FinalizeAnthropicResponsesStream(state))
				require.Empty(t, FinalizeResponsesChatStream(chatState))
			})
		}
	}
}
