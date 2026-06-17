package apicompat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests cover the codex → Responses → Anthropic conversion fixes that
// eliminated upstream 422s:
//  1. tools with no parameters (type "namespace"/"web_search") must get a
//     valid input_schema, never null
//  2. web_search must be a regular function tool, not an Anthropic server tool
//     (some third-party upstreams do not implement server tools → 422)
//  3. codex's top-level `instructions` must map to the Anthropic system field
//  4. `developer` role items must map to system, not leak as user input_text
//  5. an empty/whitespace system must be omitted (Anthropic 422s on empty system)

func anthReqFrom(t *testing.T, body string) *AnthropicRequest {
	t.Helper()
	var req ResponsesRequest
	require.NoError(t, json.Unmarshal([]byte(body), &req))
	out, err := ResponsesToAnthropicRequest(&req)
	require.NoError(t, err)
	return out
}

// systemText extracts the concatenated text from an Anthropic system field,
// which buildSystemJSON emits in array form ([{"type":"text","text":...}]).
func systemText(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	if len(raw) == 0 {
		return ""
	}
	// array form
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err == nil {
		var sb []string
		for _, p := range parts {
			sb = append(sb, p.Text)
		}
		return strings.Join(sb, "\n\n")
	}
	// string form (fallback)
	var s string
	require.NoError(t, json.Unmarshal(raw, &s))
	return s
}

func TestResponsesToAnthropic_ToolWithoutParametersGetsSchema(t *testing.T) {
	// codex namespace tools (mcp__*, multi_agent_v1, codex_app) carry no parameters.
	out := anthReqFrom(t, `{
		"model": "claude-opus-4-8",
		"input": [{"role":"user","content":[{"type":"input_text","text":"hi"}]}],
		"tools": [
			{"type":"namespace","name":"mcp__codegraph","description":"graph"},
			{"type":"namespace","name":"codex_app"}
		]
	}`)
	require.Len(t, out.Tools, 2)
	for _, tool := range out.Tools {
		require.NotEmpty(t, tool.InputSchema, "tool %s must have non-null input_schema", tool.Name)
		assert.NotEqual(t, "null", string(tool.InputSchema))
		// must be a valid object schema
		var sch map[string]any
		require.NoError(t, json.Unmarshal(tool.InputSchema, &sch))
		assert.Equal(t, "object", sch["type"])
	}
}

func TestResponsesToAnthropic_WebSearchIsFunctionToolNotServerTool(t *testing.T) {
	out := anthReqFrom(t, `{
		"model": "claude-opus-4-8",
		"input": [{"role":"user","content":[{"type":"input_text","text":"hi"}]}],
		"tools": [{"type":"web_search"}]
	}`)
	require.Len(t, out.Tools, 1)
	tool := out.Tools[0]
	assert.Equal(t, "web_search", tool.Name)
	// must NOT be emitted as Anthropic server tool web_search_20250305
	assert.NotEqual(t, "web_search_20250305", tool.Type)
	assert.Empty(t, tool.Type, "web_search must be a plain function tool, not a server tool")
	require.NotEmpty(t, tool.InputSchema)
	assert.NotEqual(t, "null", string(tool.InputSchema))
}

func TestResponsesToAnthropic_SkipsToolWithoutName(t *testing.T) {
	out := anthReqFrom(t, `{
		"model": "claude-opus-4-8",
		"input": [{"role":"user","content":[{"type":"input_text","text":"hi"}]}],
		"tools": [
			{"type":"namespace","description":"tool discovery"},
			{"type":"namespace","name":"codex_app"},
			{"type":"web_search"}
		]
	}`)
	require.Len(t, out.Tools, 2)
	assert.Equal(t, "codex_app", out.Tools[0].Name)
	assert.Equal(t, "web_search", out.Tools[1].Name)
	for _, tool := range out.Tools {
		assert.NotEmpty(t, tool.Name)
	}
}

func TestResponsesToAnthropic_FunctionToolSchemaPreserved(t *testing.T) {
	out := anthReqFrom(t, `{
		"model": "claude-opus-4-8",
		"input": [{"role":"user","content":[{"type":"input_text","text":"hi"}]}],
		"tools": [{"type":"function","name":"exec","description":"run","parameters":{"type":"object","properties":{"cmd":{"type":"string"}}}}]
	}`)
	require.Len(t, out.Tools, 1)
	assert.Equal(t, "exec", out.Tools[0].Name)
	var sch map[string]any
	require.NoError(t, json.Unmarshal(out.Tools[0].InputSchema, &sch))
	props, _ := sch["properties"].(map[string]any)
	assert.Contains(t, props, "cmd")
}

func TestResponsesToAnthropic_InstructionsBecomeSystem(t *testing.T) {
	out := anthReqFrom(t, `{
		"model": "claude-opus-4-8",
		"instructions": "You are a coding agent.",
		"input": [{"role":"user","content":[{"type":"input_text","text":"hi"}]}]
	}`)
	require.NotEmpty(t, out.System)
	sys := systemText(t, out.System)
	assert.Contains(t, sys, "You are a coding agent.")
}

func TestResponsesToAnthropic_DeveloperRoleBecomesSystem(t *testing.T) {
	out := anthReqFrom(t, `{
		"model": "claude-opus-4-8",
		"input": [
			{"role":"developer","content":[{"type":"input_text","text":"Follow the rules."}]},
			{"role":"user","content":[{"type":"input_text","text":"hi"}]}
		]
	}`)
	// developer content must be in system, not leaked into a user message
	require.NotEmpty(t, out.System)
	sys := systemText(t, out.System)
	assert.Contains(t, sys, "Follow the rules.")

	// no message content may carry input_text (Anthropic only knows "text")
	for _, m := range out.Messages {
		assert.NotContains(t, string(m.Content), "input_text",
			"input_text must not leak into Anthropic messages")
	}
}

func TestResponsesToAnthropic_InstructionsAndDeveloperConcatenated(t *testing.T) {
	out := anthReqFrom(t, `{
		"model": "claude-opus-4-8",
		"instructions": "Primary prompt.",
		"input": [
			{"role":"developer","content":[{"type":"input_text","text":"Extra context."}]},
			{"role":"user","content":[{"type":"input_text","text":"hi"}]}
		]
	}`)
	sys := systemText(t, out.System)
	assert.Contains(t, sys, "Primary prompt.")
	assert.Contains(t, sys, "Extra context.")
}

func TestResponsesToAnthropic_EmptySystemOmitted(t *testing.T) {
	// No instructions, no system/developer items → System must be nil/absent,
	// never an empty or whitespace string (Anthropic 422s on empty system).
	out := anthReqFrom(t, `{
		"model": "claude-opus-4-8",
		"instructions": "   ",
		"input": [
			{"role":"developer","content":[{"type":"input_text","text":"  "}]},
			{"role":"user","content":[{"type":"input_text","text":"hi"}]}
		]
	}`)
	if len(out.System) > 0 {
		sys := systemText(t, out.System)
		assert.NotEqual(t, "", strings.TrimSpace(sys), "system must never be empty/whitespace")
	}
}

// codex reads the tool call from the OutputItemDone item, so a streamed
// function_call's output_item.done must carry call_id, name and arguments —
// without them codex cannot execute the tool and stalls.
func TestAnthropicStream_FunctionCallDoneCarriesCallFields(t *testing.T) {
	state := &AnthropicEventToResponsesState{}
	idx := 0
	var all []ResponsesStreamEvent
	all = append(all, AnthropicEventToResponsesEvents(&AnthropicStreamEvent{
		Type: "message_start", Message: &AnthropicResponse{ID: "msg_1", Model: "claude-opus-4-8"},
	}, state)...)
	all = append(all, AnthropicEventToResponsesEvents(&AnthropicStreamEvent{
		Type: "content_block_start", Index: &idx,
		ContentBlock: &AnthropicContentBlock{Type: "tool_use", ID: "tu_1", Name: "exec"},
	}, state)...)
	all = append(all, AnthropicEventToResponsesEvents(&AnthropicStreamEvent{
		Type: "content_block_delta", Index: &idx,
		Delta: &AnthropicDelta{Type: "input_json_delta", PartialJSON: `{"cmd":"ls"}`},
	}, state)...)
	all = append(all, AnthropicEventToResponsesEvents(&AnthropicStreamEvent{
		Type: "content_block_stop", Index: &idx,
	}, state)...)

	var fcDone *ResponsesOutput
	for _, e := range all {
		if e.Type == "response.output_item.done" && e.Item != nil && e.Item.Type == "function_call" {
			fcDone = e.Item
		}
	}
	require.NotNil(t, fcDone, "must emit function_call output_item.done")
	assert.NotEmpty(t, fcDone.CallID, "call_id required")
	assert.Equal(t, "exec", fcDone.Name)
	assert.JSONEq(t, `{"cmd":"ls"}`, fcDone.Arguments)
}
