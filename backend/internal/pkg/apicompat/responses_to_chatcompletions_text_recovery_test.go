package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// When the terminal event carries a non-empty output array whose message has
// no text, the text that already streamed must be restored. Otherwise the
// client gets an empty reply while usage still bills the terminal
// output_tokens. The accumulator previously rebuilt output only when the
// terminal output array was empty, so this upstream shape silently lost text.
func TestSupplementResponseOutput_RecoversTextWhenTerminalMessageIsEmpty(t *testing.T) {
	acc := NewBufferedResponseAccumulator()
	acc.ProcessEvent(&ResponsesStreamEvent{Type: "response.output_text.delta", Delta: "Hello, "})
	acc.ProcessEvent(&ResponsesStreamEvent{Type: "response.output_text.delta", Delta: "world"})

	resp := &ResponsesResponse{
		ID:     "resp_empty_msg",
		Status: "completed",
		Output: []ResponsesOutput{{
			Type:    "message",
			Role:    "assistant",
			Content: []ResponsesContentPart{},
		}},
		Usage: &ResponsesUsage{InputTokens: 13168, OutputTokens: 3329},
	}

	acc.SupplementResponseOutput(resp)

	chat := ResponsesToChatCompletions(resp, "gpt-5.5")
	require.Len(t, chat.Choices, 1)
	require.NotNil(t, chat.Choices[0].Message.Content, "the client must not receive an empty reply")

	var got string
	require.NoError(t, json.Unmarshal(chat.Choices[0].Message.Content, &got))
	assert.Equal(t, "Hello, world", got)
}

// Whitespace-only terminal text counts as missing too.
func TestSupplementResponseOutput_RecoversTextWhenTerminalTextIsBlank(t *testing.T) {
	acc := NewBufferedResponseAccumulator()
	acc.ProcessEvent(&ResponsesStreamEvent{Type: "response.output_text.delta", Delta: "real text"})

	resp := &ResponsesResponse{
		Status: "completed",
		Output: []ResponsesOutput{{
			Type:    "message",
			Content: []ResponsesContentPart{{Type: "output_text", Text: "   "}},
		}},
	}

	acc.SupplementResponseOutput(resp)

	chat := ResponsesToChatCompletions(resp, "m")
	var got string
	require.NotNil(t, chat.Choices[0].Message.Content)
	require.NoError(t, json.Unmarshal(chat.Choices[0].Message.Content, &got))
	assert.Equal(t, "real text", got)
}

// A terminal output array without any message item gets one appended to carry
// the accumulated text; existing items are kept.
func TestSupplementResponseOutput_AppendsMessageWhenTerminalHasNoMessage(t *testing.T) {
	acc := NewBufferedResponseAccumulator()
	acc.ProcessEvent(&ResponsesStreamEvent{Type: "response.output_text.delta", Delta: "only in the stream"})

	resp := &ResponsesResponse{
		Status: "completed",
		Output: []ResponsesOutput{{
			Type:   "function_call",
			CallID: "call_1",
			Name:   "verify",
		}},
	}

	acc.SupplementResponseOutput(resp)

	var text string
	for _, item := range resp.Output {
		if item.Type != "message" {
			continue
		}
		for _, p := range item.Content {
			text += p.Text
		}
	}
	assert.Equal(t, "only in the stream", text)

	require.Len(t, resp.Output, 2)
	assert.Equal(t, "function_call", resp.Output[0].Type)
	assert.Equal(t, "verify", resp.Output[0].Name)
}

// Non-empty terminal text stays authoritative and is never overwritten by the
// accumulated stream.
func TestSupplementResponseOutput_KeepsTerminalTextAuthoritative(t *testing.T) {
	acc := NewBufferedResponseAccumulator()
	acc.ProcessEvent(&ResponsesStreamEvent{Type: "response.output_text.delta", Delta: "from the stream"})

	resp := &ResponsesResponse{
		Status: "completed",
		Output: []ResponsesOutput{{
			Type:    "message",
			Content: []ResponsesContentPart{{Type: "output_text", Text: "from the terminal event"}},
		}},
	}

	acc.SupplementResponseOutput(resp)

	require.Len(t, resp.Output, 1)
	assert.Equal(t, "from the terminal event", resp.Output[0].Content[0].Text)
}

// Without streamed text no synthetic message is created.
func TestSupplementResponseOutput_NoSyntheticMessageWithoutStreamText(t *testing.T) {
	acc := NewBufferedResponseAccumulator()

	resp := &ResponsesResponse{
		Status: "completed",
		Output: []ResponsesOutput{{
			Type:    "message",
			Content: []ResponsesContentPart{},
		}},
	}

	acc.SupplementResponseOutput(resp)

	require.Len(t, resp.Output, 1)
	assert.Empty(t, resp.Output[0].Content)
}
