package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFableMythos52ResponsesAdaptiveThinking(t *testing.T) {
	for _, model := range []string{"claude-fable-5-2", "claude-mythos-5-2"} {
		t.Run(model, func(t *testing.T) {
			for _, effort := range []string{"", "low", "medium", "high", "xhigh", "max", "none", "minimal", "invalid"} {
				t.Run("effort="+effort, func(t *testing.T) {
					out, err := ResponsesToAnthropicRequest(&ResponsesRequest{Model: model, Input: json.RawMessage(`"hello"`), Reasoning: &ResponsesReasoning{Effort: effort}})
					if effort == "none" || effort == "minimal" || effort == "invalid" {
						require.ErrorContains(t, err, "reasoning effort")
						return
					}
					require.NoError(t, err)
					require.NotNil(t, out.Thinking)
					require.Equal(t, "adaptive", out.Thinking.Type)
					require.Zero(t, out.Thinking.BudgetTokens)
					want := effort
					if want == "" {
						want = "high"
					}
					require.Equal(t, want, out.OutputConfig.Effort)
				})
			}
			out, err := ResponsesToAnthropicRequest(&ResponsesRequest{Model: model, Input: json.RawMessage(`"hello"`)})
			require.NoError(t, err)
			require.Equal(t, "adaptive", out.Thinking.Type)
			require.Equal(t, "high", out.OutputConfig.Effort)
			for _, choice := range []string{`"required"`, `{"type":"function","name":"lookup"}`, `"auto"`, `"none"`} {
				_, err := ResponsesToAnthropicRequest(&ResponsesRequest{Model: model, Input: json.RawMessage(`"hello"`), ToolChoice: json.RawMessage(choice)})
				if choice == `"auto"` || choice == `"none"` {
					require.NoError(t, err)
				} else {
					require.ErrorContains(t, err, "forced tool_choice")
				}
			}
		})
	}
}

func TestFableMythos52SignedThinkingResponsesRoundTrip(t *testing.T) {
	for _, model := range []string{"claude-fable-5-2", "claude-mythos-5-2"} {
		for _, block := range []AnthropicContentBlock{
			{Type: "thinking", Thinking: "", Signature: "upstream-signed-block"},
			{Type: "redacted_thinking", Data: "upstream-redacted-block"},
		} {
			t.Run(model+"/"+block.Type, func(t *testing.T) {
				response := AnthropicToResponsesResponse(&AnthropicResponse{Model: model, Content: []AnthropicContentBlock{block, {Type: "text", Text: "progress"}, {Type: "tool_use", ID: "toolu_1", Name: "lookup", Input: json.RawMessage(`{}`)}}})
				require.Len(t, response.Output, 3)
				raw, err := json.Marshal(response.Output)
				require.NoError(t, err)
				var items []ResponsesInputItem
				require.NoError(t, json.Unmarshal(raw, &items))
				items = append(items, ResponsesInputItem{Type: "function_call_output", CallID: response.Output[2].CallID, Output: "ok"})
				raw, err = json.Marshal(items)
				require.NoError(t, err)
				converted, err := ResponsesToAnthropicRequest(&ResponsesRequest{Model: model, Input: raw})
				require.NoError(t, err)
				require.Len(t, converted.Messages, 2)
				var blocks []AnthropicContentBlock
				require.NoError(t, json.Unmarshal(converted.Messages[0].Content, &blocks))
				require.Len(t, blocks, 3)
				require.Equal(t, block, blocks[0])
				require.Equal(t, "text", blocks[1].Type)
				require.Equal(t, "tool_use", blocks[2].Type)
			})
		}
	}
}

func TestFableMythos52ChatAdapter(t *testing.T) {
	for _, model := range []string{"claude-fable-5-2", "claude-mythos-5-2"} {
		t.Run(model, func(t *testing.T) {
			for _, effort := range []string{"", "xhigh", "max", "none"} {
				responses, err := ChatCompletionsToResponses(&ChatCompletionsRequest{Model: model, ReasoningEffort: effort, Messages: []ChatMessage{{Role: "user", Content: json.RawMessage(`"hello"`)}}})
				require.NoError(t, err)
				out, err := ResponsesToAnthropicRequest(responses)
				if effort == "none" {
					require.ErrorContains(t, err, "reasoning effort")
					continue
				}
				require.NoError(t, err)
				require.Equal(t, "adaptive", out.Thinking.Type)
				want := effort
				if want == "" {
					want = "high"
				}
				require.Equal(t, want, out.OutputConfig.Effort)
			}
			for _, choice := range []string{`"required"`, `{"type":"function","function":{"name":"lookup"}}`} {
				responses, err := ChatCompletionsToResponses(&ChatCompletionsRequest{Model: model, ToolChoice: json.RawMessage(choice), Messages: []ChatMessage{{Role: "user", Content: json.RawMessage(`"hello"`)}}})
				require.NoError(t, err)
				_, err = ResponsesToAnthropicRequest(responses)
				require.ErrorContains(t, err, "forced tool_choice")
			}
		})
	}
}

func TestFableMythos52DoesNotChangeOlderModelConversion(t *testing.T) {
	for _, model := range []string{"claude-opus-4-6", "claude-sonnet-4-6", "claude-haiku-4-5"} {
		out, err := ResponsesToAnthropicRequest(&ResponsesRequest{Model: model, Input: json.RawMessage(`"hello"`), Reasoning: &ResponsesReasoning{Effort: "xhigh"}, ToolChoice: json.RawMessage(`"required"`)})
		require.NoError(t, err)
		require.Equal(t, "max", out.OutputConfig.Effort)
		require.Equal(t, "enabled", out.Thinking.Type)
		require.Positive(t, out.Thinking.BudgetTokens)
	}
}

func TestFableMythos52StreamPreservesThinking(t *testing.T) {
	for _, model := range []string{"claude-fable-5-2", "claude-mythos-5-2"} {
		state := NewAnthropicEventToResponsesState()
		AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{Model: model}}, state)
		require.True(t, state.PreserveThinkingSignatures)
		index := 0
		AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "content_block_start", Index: &index, ContentBlock: &AnthropicContentBlock{Type: "thinking"}}, state)
		AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "content_block_delta", Index: &index, Delta: &AnthropicDelta{Type: "signature_delta", Signature: "signed-stream"}}, state)
		events := AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "content_block_stop", Index: &index}, state)
		found := false
		for _, event := range events {
			if event.Type == "response.output_item.done" && event.Item != nil {
				require.Contains(t, event.Item.EncryptedContent, anthropicThinkingEnvelopePrefix)
				found = true
			}
		}
		require.True(t, found)
	}
}

func TestFableMythos52SamplingValidation(t *testing.T) {
	for _, model := range []string{"claude-fable-5-2", "claude-mythos-5-2"} {
		temperature, topP := 0.5, 1.0
		_, err := ResponsesToAnthropicRequest(&ResponsesRequest{Model: model, Input: json.RawMessage(`"hi"`), Temperature: &temperature})
		require.ErrorContains(t, err, "temperature")
		_, err = ResponsesToAnthropicRequest(&ResponsesRequest{Model: model, Input: json.RawMessage(`"hi"`), TopP: &topP})
		require.ErrorContains(t, err, "top_p")
	}
}
