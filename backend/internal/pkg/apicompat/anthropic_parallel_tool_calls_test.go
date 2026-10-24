package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnthropicBridges_DisableParallelToolUse(t *testing.T) {
	choices := []struct {
		name      string
		input     string
		chat      string
		responses string
	}{
		{"auto", `{"type":"auto"`, `"auto"`, `"auto"`},
		{"any", `{"type":"any"`, `"required"`, `"required"`},
		{"tool", `{"type":"tool","name":"get_weather"`, `{"type":"function","function":{"name":"get_weather"}}`, `{"type":"function","name":"get_weather"}`},
	}
	flags := []struct {
		name         string
		input        string
		wantParallel bool
	}{
		{"disabled", `,"disable_parallel_tool_use":true`, false},
		{"enabled", `,"disable_parallel_tool_use":false`, true},
		{"omitted", "", true},
	}
	for _, stream := range []struct {
		name  string
		value bool
	}{{"stream", true}, {"nonstream", false}} {
		for _, choice := range choices {
			for _, flag := range flags {
				t.Run(stream.name+"/"+choice.name+"/"+flag.name, func(t *testing.T) {
					req := &AnthropicRequest{
						Model:     "glm-5.2",
						MaxTokens: 1024,
						Stream:    stream.value,
						Messages:  []AnthropicMessage{{Role: "user", Content: json.RawMessage(`"Check the weather"`)}},
						Tools: []AnthropicTool{{
							Name:        "get_weather",
							InputSchema: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`),
						}},
						ToolChoice: json.RawMessage(choice.input + flag.input + "}"),
					}
					chat, err := AnthropicToChatCompletionsRequest(req)
					require.NoError(t, err)
					require.NotNil(t, chat.ParallelToolCalls)
					assert.Equal(t, flag.wantParallel, *chat.ParallelToolCalls, "Chat Completions")
					require.JSONEq(t, choice.chat, string(chat.ToolChoice))
					require.Equal(t, stream.value, chat.Stream)

					responses, err := AnthropicToResponses(req)
					require.NoError(t, err)
					require.NotNil(t, responses.ParallelToolCalls)
					assert.Equal(t, flag.wantParallel, *responses.ParallelToolCalls, "Responses")
					require.JSONEq(t, choice.responses, string(responses.ToolChoice))
					require.Equal(t, stream.value, responses.Stream)
				})
			}
		}
	}
}

func TestAnthropicBridges_ParallelToolCallsCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name           string
		input          string
		chatChoice     string
		responseChoice string
	}{
		{"omitted", "", "", ""},
		{"null", "null", "", "null"},
		{"none", `{"type":"none","disable_parallel_tool_use":true}`, `"none"`, `"none"`},
		{"unknown", `{"type":"future_choice","disable_parallel_tool_use":true}`, "", `{"type":"future_choice","disable_parallel_tool_use":true}`},
		{"invalid_flag_type", `{"type":"auto","disable_parallel_tool_use":"true"}`, `"auto"`, `"auto"`},
		{"null_flag", `{"type":"auto","disable_parallel_tool_use":null}`, `"auto"`, `"auto"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := &AnthropicRequest{
				Model:      "glm-5.2",
				Messages:   []AnthropicMessage{{Role: "user", Content: json.RawMessage(`"Hello"`)}},
				Tools:      []AnthropicTool{{Name: "get_weather", InputSchema: json.RawMessage(`{"type":"object"}`)}},
				ToolChoice: json.RawMessage(tc.input),
			}
			chat, err := AnthropicToChatCompletionsRequest(req)
			require.NoError(t, err)
			require.NotNil(t, chat.ParallelToolCalls)
			require.True(t, *chat.ParallelToolCalls)
			require.Equal(t, tc.chatChoice, string(chat.ToolChoice))
			responses, err := AnthropicToResponses(req)
			require.NoError(t, err)
			require.NotNil(t, responses.ParallelToolCalls)
			require.True(t, *responses.ParallelToolCalls)
			if tc.responseChoice == "" {
				require.Empty(t, responses.ToolChoice)
			} else {
				require.JSONEq(t, tc.responseChoice, string(responses.ToolChoice))
			}
		})
	}
}

func TestAnthropicToChatCompletionsRequest_DroppedToolChoiceKeepsParallelDefault(t *testing.T) {
	for _, tc := range []struct {
		name  string
		tools []AnthropicTool
	}{
		{"no_tools", nil},
		{"filtered_tools", []AnthropicTool{{Type: "web_search_20250305", Name: "web_search"}}},
		{"undeclared_tool", []AnthropicTool{{Name: "other_tool", InputSchema: json.RawMessage(`{"type":"object"}`)}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := &AnthropicRequest{
				Model:      "glm-5.2",
				Messages:   []AnthropicMessage{{Role: "user", Content: json.RawMessage(`"Hello"`)}},
				Tools:      tc.tools,
				ToolChoice: json.RawMessage(`{"type":"tool","name":"get_weather","disable_parallel_tool_use":true}`),
			}
			chat, err := AnthropicToChatCompletionsRequest(req)
			require.NoError(t, err)
			require.Empty(t, chat.ToolChoice)
			require.NotNil(t, chat.ParallelToolCalls)
			require.True(t, *chat.ParallelToolCalls)
		})
	}
}

func TestAnthropicToResponses_NoToolsKeepsParallelDefault(t *testing.T) {
	req := &AnthropicRequest{
		Model:      "glm-5.2",
		Messages:   []AnthropicMessage{{Role: "user", Content: json.RawMessage(`"Hello"`)}},
		ToolChoice: json.RawMessage(`{"type":"auto","disable_parallel_tool_use":true}`),
	}
	responses, err := AnthropicToResponses(req)
	require.NoError(t, err)
	require.Empty(t, responses.Tools)
	require.JSONEq(t, `"auto"`, string(responses.ToolChoice))
	require.NotNil(t, responses.ParallelToolCalls)
	require.True(t, *responses.ParallelToolCalls)
}

func TestAnthropicBridges_InvalidToolChoiceStillReturnsError(t *testing.T) {
	for _, raw := range []string{`{"type":`, `{"type":3}`} {
		t.Run(raw, func(t *testing.T) {
			req := &AnthropicRequest{
				Model:      "glm-5.2",
				Messages:   []AnthropicMessage{{Role: "user", Content: json.RawMessage(`"Hello"`)}},
				Tools:      []AnthropicTool{{Name: "get_weather", InputSchema: json.RawMessage(`{"type":"object"}`)}},
				ToolChoice: json.RawMessage(raw),
			}
			_, err := AnthropicToChatCompletionsRequest(req)
			require.ErrorContains(t, err, "convert tool_choice")
			_, err = AnthropicToResponses(req)
			require.ErrorContains(t, err, "convert tool_choice")
		})
	}
}
