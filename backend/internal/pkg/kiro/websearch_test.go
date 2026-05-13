package kiro

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestReplaceWebSearchToolDescriptionUsesTypeFallback(t *testing.T) {
	body := []byte(`{
		"tools":[{"type":"web_search_20250305","description":"old"}],
		"messages":[{"role":"user","content":"golang"}]
	}`)

	updated, err := ReplaceWebSearchToolDescription(body)
	require.NoError(t, err)
	require.Equal(t, "web_search", gjson.GetBytes(updated, "tools.0.name").String())
	require.Equal(t, minimalWebSearchDescription, gjson.GetBytes(updated, "tools.0.description").String())
	require.Equal(t, "string", gjson.GetBytes(updated, "tools.0.input_schema.properties.query.type").String())
	require.Equal(t, "The search query to execute", gjson.GetBytes(updated, "tools.0.input_schema.properties.query.description").String())
	require.Equal(t, "query", gjson.GetBytes(updated, "tools.0.input_schema.required.0").String())
	require.True(t, gjson.GetBytes(updated, "tools.0.input_schema.additionalProperties").Bool() == false)
}

func TestInjectToolResultsClaudeAppendsMessages(t *testing.T) {
	body := []byte(`{
		"messages":[{"role":"user","content":"what is golang"}]
	}`)
	results := &WebSearchResults{
		Results: []WebSearchResult{
			{Title: "Go", URL: "https://go.dev"},
		},
	}

	updated, err := InjectToolResultsClaude(body, "srvtoolu_test", "golang", results)
	require.NoError(t, err)
	require.Equal(t, "assistant", gjson.GetBytes(updated, "messages.1.role").String())
	require.Equal(t, "tool_use", gjson.GetBytes(updated, "messages.1.content.0.type").String())
	require.Equal(t, "srvtoolu_test", gjson.GetBytes(updated, "messages.1.content.0.id").String())
	require.Equal(t, "user", gjson.GetBytes(updated, "messages.2.role").String())
	require.Equal(t, "tool_result", gjson.GetBytes(updated, "messages.2.content.0.type").String())
	require.Contains(t, gjson.GetBytes(updated, "messages.2.content.0.content").String(), "https://go.dev")
	require.Contains(t, gjson.GetBytes(updated, "messages.2.content.0.content").String(), `"title": "Go"`)
	require.Contains(t, gjson.GetBytes(updated, "messages.2.content.1.text").String(), "<search_guidance>")
}

func TestExtractWebSearchToolUseFromResponse(t *testing.T) {
	response := []byte(`{
		"content":[
			{"type":"text","text":"let me search"},
			{"type":"tool_use","id":"srvtoolu_next","name":"remote_web_search","input":{"query":"golang concurrency"}}
		]
	}`)

	toolUseID, query, ok := ExtractWebSearchToolUseFromResponse(response)
	require.True(t, ok)
	require.Equal(t, "srvtoolu_next", toolUseID)
	require.Equal(t, "golang concurrency", query)
}

func TestInjectSearchIndicatorsInResponse(t *testing.T) {
	response := []byte(`{
		"id":"msg_1",
		"type":"message",
		"role":"assistant",
		"model":"kiro",
		"content":[{"type":"text","text":"final"}],
		"stop_reason":"end_turn",
		"usage":{"input_tokens":1,"output_tokens":1}
	}`)

	snippet := "result snippet"
	updated, err := InjectSearchIndicatorsInResponse(response, []SearchIndicator{
		{
			ToolUseID: "srvtoolu_test",
			Query:     "golang",
			Results: &WebSearchResults{
				Results: []WebSearchResult{{Title: "Go", URL: "https://go.dev", Snippet: &snippet}},
			},
		},
	})
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(updated, &decoded))
	require.Equal(t, "server_tool_use", gjson.GetBytes(updated, "content.0.type").String())
	require.Equal(t, "srvtoolu_test", gjson.GetBytes(updated, "content.0.id").String())
	require.Equal(t, "web_search_tool_result", gjson.GetBytes(updated, "content.1.type").String())
	require.False(t, gjson.GetBytes(updated, "content.1.tool_use_id").Exists())
	require.Equal(t, "result snippet", gjson.GetBytes(updated, "content.1.content.0.encrypted_content").String())
	require.Equal(t, "null", gjson.GetBytes(updated, "content.1.content.0.page_age").Raw)
	require.False(t, gjson.GetBytes(updated, "content.1.content.0.page_content").Exists())
	require.Equal(t, "text", gjson.GetBytes(updated, "content.2.type").String())
}

func TestParseSearchResults_PreservesExtendedFields(t *testing.T) {
	resp := &MCPResponse{
		Result: &struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			Tools []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"tools"`
		}{
			Content: []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}{
				{
					Type: "text",
					Text: `{"results":[{"title":"Go","url":"https://go.dev","snippet":"snippet","publishedDate":1710000000,"id":"doc-1","domain":"go.dev","maxVerbatimWordLimit":25,"publicDomain":true}]}`,
				},
			},
		},
	}

	results := ParseSearchResults(resp)
	require.NotNil(t, results)
	require.Len(t, results.Results, 1)
	require.Equal(t, int64(1710000000), *results.Results[0].PublishedDate)
	require.Equal(t, "doc-1", *results.Results[0].ID)
	require.Equal(t, "go.dev", *results.Results[0].Domain)
	require.Equal(t, 25, *results.Results[0].MaxVerbatimWordLimit)
	require.True(t, *results.Results[0].PublicDomain)
}

func TestSearchGuidanceText_IsStructured(t *testing.T) {
	guidance := searchGuidanceText()
	require.Contains(t, guidance, "<search_guidance>")
	require.Contains(t, guidance, "Current date:")
	require.Contains(t, guidance, "Then you MUST use the web_search tool again with a refined query.")
	require.Contains(t, guidance, "Rephrasing in English for better coverage")
}
