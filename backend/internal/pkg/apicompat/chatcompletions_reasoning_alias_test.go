package apicompat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func readIssue5302Fixture(t *testing.T, name string, dst any) {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join("testdata", "issue5302", name))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(payload, dst))
}

func TestChatReasoningAlias_AnthropicNonStreaming(t *testing.T) {
	var response ChatCompletionsResponse
	readIssue5302Fixture(t, "nonstream_reasoning.json", &response)

	out := ChatCompletionsResponseToAnthropic(&response, "claude-sonnet-4-20250514")
	require.Len(t, out.Content, 2)
	require.Equal(t, "thinking", out.Content[0].Type)
	require.Equal(t, "fallback reasoning", out.Content[0].Thinking)
	require.Equal(t, "final answer", out.Content[1].Text)
}

func TestChatReasoningAlias_AnthropicStreaming(t *testing.T) {
	var chunk ChatCompletionsChunk
	readIssue5302Fixture(t, "stream_reasoning.json", &chunk)

	events := ChatCompletionsChunkToAnthropicEvents(&chunk, NewChatCompletionsToAnthropicStreamState("reasoning-model"))
	var thinking string
	for _, event := range events {
		if event.Delta != nil && event.Delta.Type == "thinking_delta" {
			thinking += event.Delta.Thinking
		}
	}
	require.Equal(t, "streamed fallback", thinking)
}

func TestChatReasoningAlias_ResponsesSharedPaths(t *testing.T) {
	var response ChatCompletionsResponse
	readIssue5302Fixture(t, "nonstream_reasoning.json", &response)
	nonStream := ChatCompletionsResponseToResponses(&response, "reasoning-model", nil, nil, false, nil)
	require.Len(t, nonStream.Output, 2)
	require.Equal(t, "reasoning", nonStream.Output[0].Type)
	require.Equal(t, "fallback reasoning", nonStream.Output[0].Summary[0].Text)

	var chunk ChatCompletionsChunk
	readIssue5302Fixture(t, "stream_reasoning.json", &chunk)
	events := ChatCompletionsChunkToResponsesEvents(&chunk, NewChatCompletionsToResponsesStreamState("reasoning-model"))
	var deltas []string
	for _, event := range events {
		if event.Type == "response.reasoning_summary_text.delta" {
			deltas = append(deltas, event.Delta)
		}
	}
	require.Equal(t, []string{"streamed fallback"}, deltas)
}

func TestChatReasoningAlias_ReasoningContentTakesPrecedence(t *testing.T) {
	var chunk ChatCompletionsChunk
	readIssue5302Fixture(t, "reasoning_content_precedence.json", &chunk)

	events := ChatCompletionsChunkToResponsesEvents(&chunk, NewChatCompletionsToResponsesStreamState("reasoning-model"))
	var deltas []string
	for _, event := range events {
		if event.Type == "response.reasoning_summary_text.delta" {
			deltas = append(deltas, event.Delta)
		}
	}
	require.Equal(t, []string{"preferred reasoning"}, deltas)
}
