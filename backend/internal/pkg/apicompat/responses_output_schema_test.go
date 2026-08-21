package apicompat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 防回归测试集：保证响应路径上构造的 Responses output 项与 OpenAI Responses API
// schema 兼容（@ai-sdk/openai 的 Zod 校验等严格客户端不再因为字段缺失而拒绝）。
//
// 必须满足：
//   1. 每个 output 项的 "id" 字段非空（避免被 omitempty 剔除）。
//   2. type=message 的 content[i] 必须包含 "annotations" 字段，且序列化为 [] 而
//      非 null 或省略。

// outputItemKeys 返回单个 output 项序列化后的顶层 JSON key 集合。
func outputItemKeys(t *testing.T, item ResponsesOutput) map[string]struct{} {
	t.Helper()
	raw, err := json.Marshal(item)
	require.NoError(t, err)
	var m map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &m))
	keys := make(map[string]struct{}, len(m))
	for k := range m {
		keys[k] = struct{}{}
	}
	return keys
}

// contentPartKeys 返回 message output 第一个 content part 序列化后的顶层 JSON key。
func contentPartKeys(t *testing.T, item ResponsesOutput) map[string]json.RawMessage {
	t.Helper()
	require.NotEmpty(t, item.Content, "message output must have content parts")
	raw, err := json.Marshal(item.Content[0])
	require.NoError(t, err)
	var m map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &m))
	return m
}

func TestBuildOutput_TextOutputHasIDAndAnnotations(t *testing.T) {
	acc := NewBufferedResponseAccumulator()
	acc.ProcessEvent(&ResponsesStreamEvent{Type: "response.output_text.delta", Delta: "hi"})

	output := acc.BuildOutput()
	require.Len(t, output, 1)
	msg := output[0]
	assert.Equal(t, "message", msg.Type)
	assert.NotEmpty(t, msg.ID, "message output must carry a non-empty id")
	assert.True(t, strings.HasPrefix(msg.ID, "item_"), "id should use project's item_ prefix; got %q", msg.ID)

	keys := outputItemKeys(t, msg)
	_, hasID := keys["id"]
	assert.True(t, hasID, "serialized message output must contain id key; keys=%v", keys)

	cp := contentPartKeys(t, msg)
	raw, ok := cp["annotations"]
	require.True(t, ok, "serialized content part must contain annotations key; got keys=%v", keys)
	assert.JSONEq(t, "[]", string(raw), "annotations must be empty array, got %s", raw)
}

func TestBuildOutput_ReasoningHasID(t *testing.T) {
	acc := NewBufferedResponseAccumulator()
	acc.ProcessEvent(&ResponsesStreamEvent{Type: "response.reasoning_summary_text.delta", Delta: "thinking"})

	output := acc.BuildOutput()
	require.Len(t, output, 1)
	assert.Equal(t, "reasoning", output[0].Type)
	assert.NotEmpty(t, output[0].ID)
	assert.True(t, strings.HasPrefix(output[0].ID, "item_"))

	keys := outputItemKeys(t, output[0])
	_, hasID := keys["id"]
	assert.True(t, hasID, "serialized reasoning output must contain id key")
}

func TestBuildOutput_FunctionCallHasID(t *testing.T) {
	acc := NewBufferedResponseAccumulator()
	acc.ProcessEvent(&ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 0,
		Item: &ResponsesOutput{
			Type:   "function_call",
			CallID: "call_xyz",
			Name:   "do_thing",
		},
	})
	acc.ProcessEvent(&ResponsesStreamEvent{
		Type:        "response.function_call_arguments.delta",
		OutputIndex: 0,
		Delta:       `{}`,
	})

	output := acc.BuildOutput()
	require.Len(t, output, 1)
	assert.Equal(t, "function_call", output[0].Type)
	assert.NotEmpty(t, output[0].ID)
	assert.True(t, strings.HasPrefix(output[0].ID, "item_"))
}

func TestBuildOutput_AllItemsGetUniqueIDs(t *testing.T) {
	acc := NewBufferedResponseAccumulator()
	acc.ProcessEvent(&ResponsesStreamEvent{Type: "response.reasoning_summary_text.delta", Delta: "think"})
	acc.ProcessEvent(&ResponsesStreamEvent{Type: "response.output_text.delta", Delta: "answer"})
	acc.ProcessEvent(&ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 2,
		Item:        &ResponsesOutput{Type: "function_call", CallID: "call_1", Name: "fn"},
	})
	acc.ProcessEvent(&ResponsesStreamEvent{Type: "response.function_call_arguments.delta", OutputIndex: 2, Delta: "{}"})

	output := acc.BuildOutput()
	require.Len(t, output, 3)
	seen := make(map[string]struct{}, 3)
	for _, item := range output {
		require.NotEmpty(t, item.ID, "every output item must have an id (type=%s)", item.Type)
		if _, dup := seen[item.ID]; dup {
			t.Fatalf("duplicate id %q across output items", item.ID)
		}
		seen[item.ID] = struct{}{}
	}
}

func TestChatMessageToResponsesOutput_TextHasAnnotations(t *testing.T) {
	outputs := chatMessageToResponsesOutput(ChatMessage{
		Role:    "assistant",
		Content: json.RawMessage(`"hello"`),
	})
	require.Len(t, outputs, 1)
	msg := outputs[0]
	assert.Equal(t, "message", msg.Type)
	assert.NotEmpty(t, msg.ID)

	cp := contentPartKeys(t, msg)
	raw, ok := cp["annotations"]
	require.True(t, ok, "content part must contain annotations key")
	assert.JSONEq(t, "[]", string(raw))
}

func TestEmptyResponsesMessageOutput_HasIDAndAnnotations(t *testing.T) {
	out := emptyResponsesMessageOutput()
	assert.NotEmpty(t, out.ID)

	cp := contentPartKeys(t, out)
	raw, ok := cp["annotations"]
	require.True(t, ok, "empty message output must still carry annotations key")
	assert.JSONEq(t, "[]", string(raw))
}

func TestResponsesContentPart_InputBlocksDoNotEmitAnnotations(t *testing.T) {
	// 请求路径（input_text / input_image）不应该带 annotations 字段，避免污染上游
	// 请求。指针 + omitempty 的设计保证未初始化时序列化时剔除该字段。
	for _, part := range []ResponsesContentPart{
		{Type: "input_text", Text: "hi"},
		{Type: "input_image", ImageURL: "data:image/png;base64,AAAA"},
	} {
		raw, err := json.Marshal(part)
		require.NoError(t, err)
		assert.NotContains(t, string(raw), `"annotations"`,
			"input-side ResponsesContentPart should not emit annotations; got %s", raw)
	}
}

func TestEmptyResponsesAnnotations_SerializesAsEmptyArray(t *testing.T) {
	part := ResponsesContentPart{
		Type:        "output_text",
		Text:        "hello",
		Annotations: EmptyResponsesAnnotations(),
	}
	raw, err := json.Marshal(part)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"annotations":[]`,
		"EmptyResponsesAnnotations should serialize as []; got %s", raw)
}
