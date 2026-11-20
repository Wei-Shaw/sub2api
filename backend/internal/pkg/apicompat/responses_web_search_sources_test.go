//go:build unit

package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	webSearchCallWithTwoSources = `{"type":"web_search_call","id":"ws_1","status":"completed","action":{"type":"search","query":"q1","sources":[` +
		`{"type":"url","url":"https://a.example/1"},{"type":"url","url":"https://a.example/2"}]}}`
	webSearchCallWithOneSource = `{"type":"web_search_call","id":"ws_2","status":"completed","action":{"type":"search","query":"q2","sources":[` +
		`{"type":"url","url":"https://b.example/1"}]}}`
	webSearchSourcesWantWS1 = `[{"type":"web_search_result","url":"https://a.example/1","title":"https://a.example/1"},` +
		`{"type":"web_search_result","url":"https://a.example/2","title":"https://a.example/2"}]`
	webSearchSourcesWantWS2 = `[{"type":"web_search_result","url":"https://b.example/1","title":"https://b.example/1"}]`
)

func decodeWebSearchItem(t *testing.T, raw string) ResponsesOutput {
	t.Helper()
	var item ResponsesOutput
	require.NoError(t, json.Unmarshal([]byte(raw), &item))
	return item
}

// webSearchResultContents returns each web_search_tool_result content keyed by
// the server_tool_use id it answers.
func webSearchResultContents(blocks []AnthropicContentBlock) map[string]string {
	out := map[string]string{}
	for _, block := range blocks {
		if block.Type == "web_search_tool_result" {
			out[block.ToolUseID] = string(block.Content)
		}
	}
	return out
}

func nonStreamingWebSearchResults(t *testing.T, items ...string) map[string]string {
	t.Helper()
	resp := &ResponsesResponse{ID: "resp_ws", Status: "completed"}
	for _, raw := range items {
		resp.Output = append(resp.Output, decodeWebSearchItem(t, raw))
	}
	return webSearchResultContents(ResponsesToAnthropic(resp, "gpt-5.5").Content)
}

func streamingWebSearchResults(t *testing.T, items ...string) map[string]string {
	t.Helper()
	state := NewResponsesEventToAnthropicState()
	events := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:     "response.created",
		Response: &ResponsesResponse{ID: "resp_ws", Model: "gpt-5.5"},
	}, state)
	for i, raw := range items {
		item := decodeWebSearchItem(t, raw)
		events = append(events, ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
			Type:        "response.output_item.done",
			OutputIndex: i,
			Item:        &item,
		}, state)...)
	}
	var blocks []AnthropicContentBlock
	for _, evt := range events {
		if evt.Type == "content_block_start" && evt.ContentBlock != nil {
			blocks = append(blocks, *evt.ContentBlock)
		}
	}
	return webSearchResultContents(blocks)
}

func TestResponsesEventToAnthropic_WebSearchSources(t *testing.T) {
	got := streamingWebSearchResults(t, webSearchCallWithTwoSources, webSearchCallWithOneSource)
	require.Len(t, got, 2)
	require.JSONEq(t, webSearchSourcesWantWS1, got["srvtoolu_ws_1"])
	require.JSONEq(t, webSearchSourcesWantWS2, got["srvtoolu_ws_2"])
}

func TestResponsesToAnthropic_WebSearchSources(t *testing.T) {
	got := nonStreamingWebSearchResults(t, webSearchCallWithTwoSources, webSearchCallWithOneSource)
	require.Len(t, got, 2)
	require.JSONEq(t, webSearchSourcesWantWS1, got["srvtoolu_ws_1"])
	require.JSONEq(t, webSearchSourcesWantWS2, got["srvtoolu_ws_2"])
}

func TestWebSearchSourcesEntryRules(t *testing.T) {
	got := nonStreamingWebSearchResults(t, `{"type":"web_search_call","id":"ws_1","status":"completed","action":{"type":"search","query":"q",`+
		`"sources":[{"url":"https://a"},{"url":""},{"url":1},"https://b",{"title":"x"}]}}`)
	require.JSONEq(t, `[{"type":"web_search_result","url":"https://a","title":"https://a"}]`, got["srvtoolu_ws_1"])
}

func TestWebSearchOpenPageReturnsSinglePage(t *testing.T) {
	got := nonStreamingWebSearchResults(t, `{"type":"web_search_call","id":"ws_1","status":"completed",`+
		`"action":{"type":"open_page","url":"https://docs.example/page?id=1#top"}}`)
	require.JSONEq(t, `[{"type":"web_search_result","url":"https://docs.example/page?id=1#top","title":"https://docs.example/page?id=1#top"}]`,
		got["srvtoolu_ws_1"])
}

func TestWebSearchOpenPageRequiresCompletedURL(t *testing.T) {
	for name, item := range map[string]string{
		"empty url": `{"type":"web_search_call","id":"ws_1","status":"completed","action":{"type":"open_page","url":""}}`,
	} {
		t.Run(name, func(t *testing.T) {
			require.JSONEq(t, `[]`, nonStreamingWebSearchResults(t, item)["srvtoolu_ws_1"])
		})
	}
}

func TestWebSearchNonStringActionURLKeepsEvent(t *testing.T) {
	for name, url := range map[string]string{"number": `123`, "object": `{"a":1}`, "array": `["a"]`} {
		t.Run(name, func(t *testing.T) {
			raw := `{"type":"web_search_call","id":"ws_1","status":"completed","action":{"type":"open_page","url":` + url + `}}`
			got := nonStreamingWebSearchResults(t, raw)
			require.JSONEq(t, `[]`, got["srvtoolu_ws_1"])
			require.JSONEq(t, `[]`, streamingWebSearchResults(t, raw)["srvtoolu_ws_1"])
		})
	}
}

func TestWebSearchCallsThatDidNotCompleteProduceNoBlocks(t *testing.T) {
	for _, status := range []string{"failed", "in_progress"} {
		t.Run(status, func(t *testing.T) {
			raw := `{"type":"web_search_call","id":"ws_1","status":"` + status + `","action":{"type":"search","query":"q","sources":[{"url":"https://a.example/1"}]}}`
			require.Empty(t, nonStreamingWebSearchResults(t, raw))
			require.Empty(t, streamingWebSearchResults(t, raw))
		})
	}
}
