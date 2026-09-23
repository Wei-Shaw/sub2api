package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIResponsesToolLoopGuardSuppressesRepeatedInvalidHandle(t *testing.T) {
	body := []byte(`{
		"model":"gpt-5.6-sol",
		"instructions":"work carefully",
		"tools":[
			{"type":"function","name":"wait","parameters":{"type":"object"}},
			{"type":"custom","name":"exec"}
		],
		"tool_choice":{"type":"function","name":"wait"},
		"parallel_tool_calls":true,
		"input":[
			{"type":"message","role":"user","content":"deploy"},
			{"type":"function_call","call_id":"call_1","name":"wait","arguments":"{\"cell_id\":\"m1\"}"},
			{"type":"function_call_output","call_id":"call_1","output":"Script failed\nScript error:\nexec cell m1 not found"},
			{"type":"function_call","call_id":"call_2","name":"wait","arguments":"{\"cell_id\":\"m2\"}"},
			{"type":"function_call_output","call_id":"call_2","output":[{"type":"input_text","text":"Script failed"},{"type":"input_text","text":"exec cell m2 not found"}]}
		]
	}`)

	normalized, result, changed, err := applyOpenAIResponsesToolLoopGuard(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "wait", result.ToolName)
	require.Equal(t, "invalid_handle", result.ErrorClass)
	require.Equal(t, 2, result.FailureCount)
	require.Len(t, gjson.GetBytes(normalized, "tools").Array(), 1)
	require.Equal(t, "exec", gjson.GetBytes(normalized, "tools.0.name").String())
	require.Equal(t, "auto", gjson.GetBytes(normalized, "tool_choice").String())
	require.True(t, gjson.GetBytes(normalized, "parallel_tool_calls").Bool())
	require.Contains(t, gjson.GetBytes(normalized, "instructions").String(), openAIToolLoopGuardMarker)
}

func TestOpenAIResponsesToolLoopGuardRemovesEmptyToolControls(t *testing.T) {
	body := []byte(`{
		"tools":[{"type":"function","name":"wait"}],
		"tool_choice":{"type":"function","function":{"name":"wait"}},
		"parallel_tool_calls":true,
		"input":[
			{"type":"function_call","call_id":"a","name":"wait","arguments":"{}"},
			{"type":"function_call_output","call_id":"a","output":"exec cell x1 not found"},
			{"type":"function_call","call_id":"b","name":"wait","arguments":"{}"},
			{"type":"function_call_output","call_id":"b","output":"exec cell x2 not found"}
		]
	}`)

	normalized, _, changed, err := applyOpenAIResponsesToolLoopGuard(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(normalized, "tools").Exists())
	require.False(t, gjson.GetBytes(normalized, "tool_choice").Exists())
	require.False(t, gjson.GetBytes(normalized, "parallel_tool_calls").Exists())
}

func TestOpenAIResponsesToolLoopGuardDoesNotSuppressSingleFailure(t *testing.T) {
	body := []byte(`{
		"tools":[{"type":"function","name":"wait"}],
		"input":[
			{"type":"function_call","call_id":"a","name":"wait","arguments":"{}"},
			{"type":"function_call_output","call_id":"a","output":"exec cell x1 not found"}
		]
	}`)

	normalized, result, changed, err := applyOpenAIResponsesToolLoopGuard(body)
	require.NoError(t, err)
	require.False(t, changed)
	require.Empty(t, result.ToolName)
	require.Equal(t, string(body), string(normalized))
}

func TestOpenAIResponsesToolLoopGuardResetsAtNewUserMessage(t *testing.T) {
	body := []byte(`{
		"tools":[{"type":"function","name":"wait"}],
		"input":[
			{"type":"function_call","call_id":"a","name":"wait","arguments":"{}"},
			{"type":"function_call_output","call_id":"a","output":"exec cell x1 not found"},
			{"type":"function_call","call_id":"b","name":"wait","arguments":"{}"},
			{"type":"function_call_output","call_id":"b","output":"exec cell x2 not found"},
			{"type":"message","role":"user","content":"start another task"}
		]
	}`)

	_, _, changed, err := applyOpenAIResponsesToolLoopGuard(body)
	require.NoError(t, err)
	require.False(t, changed)
}

func TestOpenAIResponsesToolLoopGuardStopsAtSuccessfulToolOutput(t *testing.T) {
	body := []byte(`{
		"tools":[{"type":"function","name":"wait"}],
		"input":[
			{"type":"function_call","call_id":"a","name":"wait","arguments":"{}"},
			{"type":"function_call_output","call_id":"a","output":"exec cell x1 not found"},
			{"type":"function_call","call_id":"ok","name":"wait","arguments":"{}"},
			{"type":"function_call_output","call_id":"ok","output":"Script completed\nOutput:\ndone"},
			{"type":"function_call","call_id":"b","name":"wait","arguments":"{}"},
			{"type":"function_call_output","call_id":"b","output":"exec cell x2 not found"}
		]
	}`)

	_, _, changed, err := applyOpenAIResponsesToolLoopGuard(body)
	require.NoError(t, err)
	require.False(t, changed)
}

func TestOpenAIResponsesToolLoopGuardSupportsCustomToolCalls(t *testing.T) {
	body := []byte(`{
		"tools":[{"type":"custom","name":"runner"}],
		"input":[
			{"type":"custom_tool_call","call_id":"a","name":"runner","input":"one"},
			{"type":"custom_tool_call_output","call_id":"a","output":"permission denied"},
			{"type":"custom_tool_call","call_id":"b","name":"runner","input":"two"},
			{"type":"custom_tool_call_output","call_id":"b","output":"access denied"}
		]
	}`)

	_, result, changed, err := applyOpenAIResponsesToolLoopGuard(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "runner", result.ToolName)
	require.Equal(t, "permission_denied", result.ErrorClass)
}

func TestOpenAIResponsesToolLoopGuardSuppressesNamespacedLiteTool(t *testing.T) {
	body := []byte(`{
		"model":"gpt-5.6-sol",
		"instructions":"work carefully",
		"parallel_tool_calls":true,
		"input":[
			{"type":"additional_tools","role":"developer","tools":[
				{"type":"namespace","name":"functions","tools":[
					{"type":"custom","name":"exec"},
					{"type":"function","name":"wait","parameters":{"type":"object"}}
				]},
				{"type":"namespace","name":"collaboration","tools":[
					{"type":"function","name":"send_message","parameters":{"type":"object"}}
				]}
			]},
			{"type":"message","role":"user","content":"deploy"},
			{"type":"function_call","call_id":"call_1","namespace":"functions","name":"wait","arguments":"{\"cell_id\":\"missing-1\"}"},
			{"type":"function_call_output","call_id":"call_1","output":[{"type":"input_text","text":"Script error:\nexec cell missing-1 not found"}]},
			{"type":"function_call","call_id":"call_2","namespace":"functions","name":"wait","arguments":"{\"cell_id\":\"missing-2\"}"},
			{"type":"function_call_output","call_id":"call_2","output":[{"type":"input_text","text":"Script error:\nexec cell missing-2 not found"}]}
		]
	}`)

	normalized, result, changed, err := applyOpenAIResponsesToolLoopGuard(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "wait", result.ToolName)
	require.Equal(t, "functions", result.ToolNamespace)
	require.Equal(t, "invalid_handle", result.ErrorClass)
	require.Equal(t, 2, result.FailureCount)
	require.Equal(t, "exec", gjson.GetBytes(normalized, `input.#(type=="additional_tools").tools.#(name=="functions").tools.0.name`).String())
	require.False(t, gjson.GetBytes(normalized, `input.#(type=="additional_tools").tools.#(name=="functions").tools.#(name=="wait")`).Exists())
	require.Equal(t, "send_message", gjson.GetBytes(normalized, `input.#(type=="additional_tools").tools.#(name=="collaboration").tools.0.name`).String())
	require.True(t, gjson.GetBytes(normalized, "parallel_tool_calls").Bool())
	require.Contains(t, gjson.GetBytes(normalized, "instructions").String(), `Tool "functions.wait"`)
}

func TestOpenAIResponsesToolLoopGuardSuppressesFlattenedNamespacedTool(t *testing.T) {
	body := []byte(`{
		"tools":[
			{"type":"function","name":"functions__wait"},
			{"type":"function","name":"functions__exec"}
		],
		"input":[
			{"type":"function_call","call_id":"a","namespace":"functions","name":"wait","arguments":"{}"},
			{"type":"function_call_output","call_id":"a","output":"exec cell x1 not found"},
			{"type":"function_call","call_id":"b","namespace":"functions","name":"wait","arguments":"{}"},
			{"type":"function_call_output","call_id":"b","output":"exec cell x2 not found"}
		]
	}`)

	normalized, result, changed, err := applyOpenAIResponsesToolLoopGuard(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "functions", result.ToolNamespace)
	require.Len(t, gjson.GetBytes(normalized, "tools").Array(), 1)
	require.Equal(t, "functions__exec", gjson.GetBytes(normalized, "tools.0.name").String())
}
