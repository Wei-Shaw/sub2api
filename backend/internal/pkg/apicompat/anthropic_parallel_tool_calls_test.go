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
