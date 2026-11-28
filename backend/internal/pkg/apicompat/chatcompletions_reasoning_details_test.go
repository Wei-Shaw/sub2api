//go:build unit

package apicompat

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func reasoningDetailJSON(block AnthropicContentBlock, index int) string {
	return fmt.Sprintf(`{"type":"reasoning.encrypted","data":%q,"format":"anthropic-claude-v1","index":%d}`, encodeAnthropicThinking(block), index)
}

func TestChatReasoningDetailsRestoreAnthropicThinkingBlocks(t *testing.T) {
	// Entries arrive out of index order and mixed with entries this bridge must ignore.
	reqJSON := `{"model":"claude-opus-5-5","messages":[
		{"role":"user","content":"look it up"},
		{"role":"assistant","content":"checking","reasoning_content":"summary must not be injected",
		 "tool_calls":[{"id":"toolu_1","type":"function","function":{"name":"lookup","arguments":"{}"}}],
		 "reasoning_details":[` +
		reasoningDetailJSON(AnthropicContentBlock{Type: "redacted_thinking", Data: "redacted-blob=="}, 1) + `,
			{"type":"reasoning.text","text":"unsigned","format":"anthropic-claude-v1","index":2},
			{"type":"reasoning.encrypted","data":"not-an-envelope","format":"anthropic-claude-v1","index":3},
			{"type":"reasoning.encrypted","data":"` + encodeAnthropicThinking(AnthropicContentBlock{Type: "thinking", Thinking: "other vendor", Signature: "x"}) + `","format":"openai-responses-v1","index":4},
			` + reasoningDetailJSON(AnthropicContentBlock{Type: "thinking", Thinking: "", Signature: "sig-A/B+=="}, 0) + `
		 ]},
		{"role":"tool","tool_call_id":"toolu_1","content":"found"}]}`
	var req ChatCompletionsRequest
	require.NoError(t, json.Unmarshal([]byte(reqJSON), &req))

	responsesReq, err := ChatCompletionsToResponsesWithOptions(&req, ChatCompletionsToResponsesOptions{ReplayAnthropicThinking: true})
	require.NoError(t, err)
	anthropicReq, err := ResponsesToAnthropicRequest(responsesReq)
	require.NoError(t, err)

	require.Len(t, anthropicReq.Messages, 3)
	var blocks []AnthropicContentBlock
	require.NoError(t, json.Unmarshal(anthropicReq.Messages[1].Content, &blocks))
	require.Len(t, blocks, 4, "assistant content: %s", anthropicReq.Messages[1].Content)
	require.Equal(t, AnthropicContentBlock{Type: "thinking", Thinking: "", Signature: "sig-A/B+=="}, blocks[0])
	require.Equal(t, AnthropicContentBlock{Type: "redacted_thinking", Data: "redacted-blob=="}, blocks[1])
	require.Equal(t, AnthropicContentBlock{Type: "text", Text: "checking"}, blocks[2])
	require.Equal(t, "tool_use", blocks[3].Type)
	require.NotContains(t, string(anthropicReq.Messages[1].Content), "summary must not be injected")
}

func TestChatReasoningDetailsIgnoredWithoutReplay(t *testing.T) {
	reqJSON := `{"model":"claude-opus-5-5","messages":[
		{"role":"assistant","content":"checking","reasoning_content":"summary",
		 "reasoning_details":[` + reasoningDetailJSON(AnthropicContentBlock{Type: "redacted_thinking", Data: "redacted-blob=="}, 0) + `]}]}`
	var req ChatCompletionsRequest
	require.NoError(t, json.Unmarshal([]byte(reqJSON), &req))

	responsesReq, err := ChatCompletionsToResponsesWithOptions(&req, ChatCompletionsToResponsesOptions{})
	require.NoError(t, err)

	require.NotContains(t, string(responsesReq.Input), anthropicThinkingEnvelopePrefix)
	var items []ResponsesInputItem
	require.NoError(t, json.Unmarshal(responsesReq.Input, &items))
	require.Len(t, items, 1)
	var parts []ResponsesContentPart
	require.NoError(t, json.Unmarshal(items[0].Content, &parts))
	require.Len(t, parts, 1)
	require.Equal(t, "<thinking>summary</thinking>\nchecking", parts[0].Text)
}

// A reasoning_details value of the wrong shape is ignored, as it was before the
// field was read, and must not fail the whole request.
func TestChatReasoningDetailsMalformedValueIsIgnored(t *testing.T) {
	for name, details := range map[string]string{
		"index is a string": `[{"type":"reasoning.text","text":"x","index":"0"}]`,
		"not an array":      `{"type":"reasoning.text"}`,
		"entry is a string": `["x"]`,
	} {
		t.Run(name, func(t *testing.T) {
			reqJSON := `{"model":"claude-opus-5-5","messages":[
				{"role":"assistant","content":"yo","reasoning_content":"summary","reasoning_details":` + details + `}]}`
			var req ChatCompletionsRequest
			require.NoError(t, json.Unmarshal([]byte(reqJSON), &req))

			responsesReq, err := ChatCompletionsToResponsesWithOptions(&req, ChatCompletionsToResponsesOptions{ReplayAnthropicThinking: true})
			require.NoError(t, err)
			require.NotContains(t, string(responsesReq.Input), anthropicThinkingEnvelopePrefix)
			require.Contains(t, string(responsesReq.Input), "summary")
		})
	}
}

func signedThinkingResponsesItem() (ResponsesOutput, string) {
	envelope := encodeAnthropicThinking(AnthropicContentBlock{Type: "thinking", Thinking: "plan", Signature: "sig"})
	return ResponsesOutput{Type: "reasoning", ID: "rs_1", EncryptedContent: envelope, Summary: []ResponsesSummary{{Type: "summary_text", Text: "plan"}}}, envelope
}

// Callers that do not replay the field keep their previous output.
func TestResponsesToChatNoReasoningDetailsByDefault(t *testing.T) {
	item, _ := signedThinkingResponsesItem()
	resp := &ResponsesResponse{ID: "resp_1", Status: "completed", Output: []ResponsesOutput{item}}
	require.Empty(t, ResponsesToChatCompletions(resp, "m").Choices[0].Message.ReasoningDetails)

	chunks := ResponsesEventToChatChunks(&ResponsesStreamEvent{Type: "response.output_item.done", Item: &item}, NewResponsesEventToChatState())
	require.Empty(t, chunks)
}

func TestResponsesToChatReasoningDetailsWhenRequested(t *testing.T) {
	item, envelope := signedThinkingResponsesItem()
	resp := &ResponsesResponse{ID: "resp_1", Status: "completed", Output: []ResponsesOutput{item}}
	want := fmt.Sprintf(`[{"type":"reasoning.encrypted","data":%q,"format":"anthropic-claude-v1","index":0}]`, envelope)

	got := ResponsesToChatCompletionsWithOptions(resp, "m", ResponsesToChatCompletionsOptions{AnthropicReasoningDetails: true})
	require.JSONEq(t, want, string(got.Choices[0].Message.ReasoningDetails))

	state := NewResponsesEventToChatState()
	state.AnthropicReasoningDetails = true
	chunks := ResponsesEventToChatChunks(&ResponsesStreamEvent{Type: "response.output_item.done", Item: &item}, state)
	require.Len(t, chunks, 1)
	require.JSONEq(t, want, string(chunks[0].Choices[0].Delta.ReasoningDetails))
}
