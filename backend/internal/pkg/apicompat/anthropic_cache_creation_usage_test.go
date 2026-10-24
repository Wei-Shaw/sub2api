package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAnthropicCacheCreationAggregatePresence(t *testing.T) {
	for _, test := range []struct {
		name     string
		snapshot string
		want     int
		present  bool
	}{
		{"contradictory aggregate", `{"cache_creation_input_tokens":100,"cache_creation":{"ephemeral_1h_input_tokens":200}}`, 100, true},
		{"explicit zero", `{"cache_creation_input_tokens":0,"cache_creation":{"ephemeral_1h_input_tokens":200}}`, 0, true},
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
		{"missing Responses usage", anthropicUsageFromResponsesUsage(nil), false},
		{"explicit Responses totals", anthropicUsageFromResponsesUsage(&ResponsesUsage{}), true},
		{"missing Chat usage", chatUsageToAnthropicUsage(nil), false},
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
		require.False(t, decoded.Message.Usage.HasCacheCreationInputTokens())
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
		require.Equal(t, []int{20, 50, 50, 50, 40, 40, 0, 0, 0, 100}[index], state.CacheCreationInputTokens)
	}
}
