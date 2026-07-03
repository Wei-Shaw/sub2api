package kiro

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateSearchIndicatorEvents_UsesInputJSONDelta(t *testing.T) {
	snippet := "result snippet"
	events := GenerateSearchIndicatorEvents("golang concurrency", "srvtoolu_test", &WebSearchResults{
		Results: []WebSearchResult{
			{Title: "Go", URL: "https://go.dev", Snippet: &snippet},
		},
	}, 0)

	require.Len(t, events, 5)
	require.Contains(t, string(events[0]), `"type":"server_tool_use"`)
	require.Contains(t, string(events[0]), `"input":{}`)
	require.Contains(t, string(events[1]), `"type":"input_json_delta"`)
	require.Contains(t, string(events[1]), `"{\"query\":\"golang concurrency\"}"`)
	require.Contains(t, string(events[3]), `"type":"web_search_tool_result"`)
	require.NotContains(t, string(events[3]), `"tool_use_id"`)
	require.Contains(t, string(events[3]), `"encrypted_content":"result snippet"`)
}

func TestAnalyzeBufferedStream_ExtractsWebSearchToolUse(t *testing.T) {
	chunks := [][]byte{
		[]byte("event: message_start\ndata: {\"type\":\"message_start\"}\n\n"),
		[]byte("event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":1,\"content_block\":{\"type\":\"tool_use\",\"id\":\"srvtoolu_next\",\"name\":\"web_search\",\"input\":{}}}\n\n"),
		[]byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"query\\\":\\\"golang concurrency\\\"}\"}}\n\n"),
		[]byte("event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":1}\n\n"),
		[]byte("event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\"}}\n\n"),
	}

	result := AnalyzeBufferedStream(chunks)
	require.True(t, result.HasWebSearchToolUse)
	require.Equal(t, "golang concurrency", result.WebSearchQuery)
	require.Equal(t, "srvtoolu_next", result.WebSearchToolUseID)
	require.Equal(t, 1, result.WebSearchToolUseIndex)
	require.Equal(t, "tool_use", result.StopReason)
}

func TestFilterChunksForClient_RemovesInternalToolUseAndOffsetsIndices(t *testing.T) {
	chunks := [][]byte{
		[]byte("event: message_start\ndata: {\"type\":\"message_start\"}\n\n"),
		[]byte("event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n"),
		[]byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Searching...\"}}\n\n"),
		[]byte("event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n"),
		[]byte("event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":1,\"content_block\":{\"type\":\"tool_use\",\"id\":\"srvtoolu_next\",\"name\":\"web_search\",\"input\":{}}}\n\n"),
		[]byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"query\\\":\\\"golang concurrency\\\"}\"}}\n\n"),
		[]byte("event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":1}\n\n"),
		[]byte("event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\"}}\n\n"),
	}

	filtered := FilterChunksForClient(chunks, 1, 2)
	require.NotEmpty(t, filtered)
	joined := string(filtered[0]) + string(filtered[1]) + string(filtered[2])
	require.NotContains(t, joined, `"type":"message_start"`)
	require.NotContains(t, joined, `"type":"message_delta"`)
	require.NotContains(t, joined, `"name":"web_search"`)
	require.Contains(t, joined, `"index":2`)
	require.Equal(t, 2, MaxContentBlockIndex(filtered))
}

func TestAdjustSSEChunk_OffsetsIndicesAndDropsMessageStart(t *testing.T) {
	_, shouldForward := AdjustSSEChunk([]byte("event: message_start\ndata: {\"type\":\"message_start\"}\n\n"), 2)
	require.False(t, shouldForward)

	adjusted, shouldForward := AdjustSSEChunk([]byte("event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n"), 3)
	require.True(t, shouldForward)
	require.Contains(t, string(adjusted), `"index":3`)
}
