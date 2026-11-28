package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// DeepSeek extension fields (safety_check/title) and DeepSeek server tools
// (deepseek_web_search/deepseek_read_url) keep the same shape in Chat
// Completions and Responses, so conversion must pass them through verbatim.
func TestChatCompletionsToResponses_DeepSeekExtensions(t *testing.T) {
	req := &ChatCompletionsRequest{
		Model:       "deepseek-flash",
		Messages:    []ChatMessage{{Role: "user", Content: json.RawMessage(`"DeepSeek V4.1 有哪些更新？"`)}},
		SafetyCheck: &SafetyCheckParam{Level: "high"},
		Title:       &TitleParam{Enabled: true},
		Tools: []ChatTool{
			{Type: "deepseek_web_search", Name: "deepseek_web_search"},
			{Type: "deepseek_read_url", Name: "deepseek_read_url"},
			{
				Type:     "function",
				Function: &ChatFunction{Name: "get_weather", Parameters: json.RawMessage(`{"type":"object"}`)},
			},
		},
	}

	resp, err := ChatCompletionsToResponses(req)
	require.NoError(t, err)

	require.Equal(t, &SafetyCheckParam{Level: "high"}, resp.SafetyCheck)
	require.Equal(t, &TitleParam{Enabled: true}, resp.Title)
	require.Len(t, resp.Tools, 3)
	require.Equal(t, ResponsesTool{Type: "deepseek_web_search", Name: "deepseek_web_search"}, resp.Tools[0])
	require.Equal(t, ResponsesTool{Type: "deepseek_read_url", Name: "deepseek_read_url"}, resp.Tools[1])
	require.Equal(t, "function", resp.Tools[2].Type)
	require.Equal(t, "get_weather", resp.Tools[2].Name)

	// Wire shape: the deepseek tool items must carry both type and name.
	toolsJSON, err := json.Marshal(resp.Tools[:2])
	require.NoError(t, err)
	require.JSONEq(t, `[
		{"type":"deepseek_web_search","name":"deepseek_web_search"},
		{"type":"deepseek_read_url","name":"deepseek_read_url"}
	]`, string(toolsJSON))
}

// Without the extension fields, the converted request must not carry them at
// all (no zero-value pollution).
func TestChatCompletionsToResponses_NoDeepSeekExtensionsOmitted(t *testing.T) {
	req := &ChatCompletionsRequest{
		Model:    "deepseek-flash",
		Messages: []ChatMessage{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	}

	resp, err := ChatCompletionsToResponses(req)
	require.NoError(t, err)
	require.Nil(t, resp.SafetyCheck)
	require.Nil(t, resp.Title)
	require.Empty(t, resp.Tools)

	out, err := json.Marshal(resp)
	require.NoError(t, err)
	require.NotContains(t, string(out), "safety_check")
	require.NotContains(t, string(out), `"title"`)
}

// The reverse direction (Responses → Chat Completions) keeps the same
// symmetric passthrough for the extension fields and server tools.
func TestResponsesToChatCompletionsRequest_DeepSeekExtensions(t *testing.T) {
	req := &ResponsesRequest{
		Model:       "deepseek-flash",
		Input:       json.RawMessage(`"DeepSeek V4.1 有哪些更新？"`),
		SafetyCheck: &SafetyCheckParam{Level: "high"},
		Title:       &TitleParam{Enabled: true},
		Tools: []ResponsesTool{
			{Type: "deepseek_web_search", Name: "deepseek_web_search"},
			{Type: "deepseek_read_url"},
		},
	}

	out, err := ResponsesToChatCompletionsRequest(req)
	require.NoError(t, err)

	require.Equal(t, &SafetyCheckParam{Level: "high"}, out.SafetyCheck)
	require.Equal(t, &TitleParam{Enabled: true}, out.Title)
	require.Len(t, out.Tools, 2)
	require.Equal(t, ChatTool{Type: "deepseek_web_search", Name: "deepseek_web_search"}, out.Tools[0])
	// Missing name falls back to the tool type.
	require.Equal(t, ChatTool{Type: "deepseek_read_url", Name: "deepseek_read_url"}, out.Tools[1])
}
