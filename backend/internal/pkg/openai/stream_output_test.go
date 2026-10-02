package openai

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResponsesStreamHasOutput(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{"text", `{"type":"response.output_text.delta","delta":"OK"}`},
		{"whitespace token", `{"type":"response.output_text.delta","delta":" \n"}`},
		{"reasoning", `{"type":"response.reasoning_text.delta","delta":"thinking"}`},
		{"summary", `{"type":"response.reasoning_summary_text.delta","delta":"summary"}`},
		{"arguments", `{"type":"response.function_call_arguments.delta","delta":"{}"}`},
		{"custom tool", `{"type":"response.custom_tool_call_input.delta","delta":"input"}`},
		{"refusal", `{"type":"response.refusal.delta","delta":"refused"}`},
		{"audio", `{"type":"response.audio.delta","delta":"YQ=="}`},
		{"output audio", `{"type":"response.output_audio.delta","delta":"YQ=="}`},
		{"transcript", `{"type":"response.audio_transcript.delta","delta":"hello"}`},
		{"partial image", `{"type":"response.image_generation_call.partial_image","partial_image_b64":"YQ=="}`},
		{"image result", `{"type":"response.output_item.done","item":{"type":"image_generation_call","result":"YQ=="}}`},
		{"text done", `{"type":"response.output_text.done","text":"OK"}`},
		{"refusal done", `{"type":"response.refusal.done","refusal":"refused"}`},
		{"arguments done", `{"type":"response.function_call_arguments.done","arguments":"{}"}`},
		{"custom tool done", `{"type":"response.custom_tool_call_input.done","input":"input"}`},
		{"code done", `{"type":"response.code_interpreter_call_code.done","code":"print(1)"}`},
		{"transcript done", `{"type":"response.output_audio_transcript.done","text":"hello"}`},
		{"summary part", `{"type":"response.reasoning_summary_part.added","part":{"type":"summary_text","text":"summary"}}`},
		{"refusal part", `{"type":"response.content_part.done","part":{"type":"refusal","refusal":"refused"}}`},
		{"summary item", `{"type":"response.output_item.added","item":{"type":"reasoning","summary":[{"type":"summary_text","text":"summary"}]}}`},
		{"refusal item", `{"type":"response.output_item.done","item":{"type":"message","content":[{"type":"refusal","refusal":"refused"}]}}`},
		{"terminal with output", `{"type":"response.completed","response":{"output":[{"type":"message","content":[{"type":"output_text","text":"OK"}]}]}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, encrypted := range []bool{false, true} {
				require.True(t, ResponsesStreamHasOutput(tt.data, "", encrypted))
			}
		})
	}
	for _, data := range []string{
		`{"type":"response.output_item.added","item":{"type":"reasoning","encrypted_content":"cipher","summary":[]}}`,
		`{"type":"response.output_item.done","item":{"type":"compaction","encrypted_content":"cipher"}}`,
		`{"type":"response.completed","response":{"output":[{"type":"reasoning","encrypted_content":"cipher"}]}}`,
	} {
		require.True(t, ResponsesStreamHasOutput(data, "", true), data)
		require.False(t, ResponsesStreamHasOutput(data, "", false), data)
	}
	// SSE may carry the event type in its event: line rather than its JSON.
	require.True(t, ResponsesStreamHasOutput(`{"delta":"OK"}`, " response.output_text.delta ", false))
}

func TestResponsesStreamHasOutputRejectsEmptyOrInvalidPayloads(t *testing.T) {
	for _, data := range []string{
		``, `[DONE]`, `null`, `{}`, `[]`, `{"type":"response.output_text.delta","delta":"broken"`,
		`{"type":"keepalive"}`, `{"type":"response.created"}`, `{"type":"response.in_progress"}`,
		`{"type":"response.output_text.delta","delta":"","SSE-Keep-Alive":true}`,
		`{"type":"response.output_text.delta","delta":""}`,
		`{"type":"response.output_text.delta"}`,
		`{"type":"response.output_text.delta","delta":null}`,
		`{"type":"response.output_text.delta","delta":{}}`,
		`{"type":"response.output_text.delta","delta":[]}`,
		`{"type":"response.output_text.delta","delta":true}`,
		`{"type":"response.output_text.delta","delta":42}`,
		`{"type":"response.output_text.done","text":42}`,
		`{"type":"response.output_audio.done"}`,
		`{"type":"response.output_text.annotation.added","annotation":{}}`,
		`{"type":"response.image_generation_call.in_progress"}`,
		`{"type":"response.image_generation_call.partial_image","partial_image_b64":{}}`,
		`{"type":"response.output_item.added","item":{}}`,
		`{"type":"response.output_item.added","item":{"type":"reasoning","encrypted_content":{},"summary":[{}]}}`,
		`{"type":"response.output_item.added","item":{"type":"reasoning","encrypted_content":"","summary":[]}}`,
		`{"type":"response.output_item.done","item":{"content":{"text":"not an array"}}}`,
		`{"type":"response.output_item.added","item":{"type":"message","content":[{"type":"output_text","text":null}]}}`,
		`{"type":"response.content_part.added","part":{"type":"refusal","refusal":""}}`,
		`{"type":"response.completed","response":{"output":[],"usage":{"input_tokens":1,"output_tokens":2}}}`,
		`{"type":"response.done","response":{"usage":{"input_tokens":1,"output_tokens":2}}}`,
		`{"type":"response.failed","response":{"error":{"message":"failed"}}}`,
		`{"type":"error","error":{"code":"content_policy","message":"blocked"}}`,
	} {
		for _, encrypted := range []bool{false, true} {
			require.False(t, ResponsesStreamHasOutput(data, "", encrypted), data)
		}
	}
}
