//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// inlineToolDefinitionBody is a follow-up /v1/messages request whose history
// contains an assistant turn that declared a new tool inline (tool_addition
// carrying a tool_definition), which CN Anthropic-compatible endpoints reject.
const inlineToolDefinitionBody = `{"model":"claude-opus-5-5","max_tokens":1024,"messages":[` +
	`{"role":"user","content":[{"type":"text","text":"look up the weather"}]},` +
	`{"role":"assistant","content":[` +
	`{"type":"tool_addition","tool":{"type":"tool_definition","name":"lookup","description":"look up a value","input_schema":{"type":"object","properties":{"q":{"type":"string"}}}}},` +
	`{"type":"text","text":"I have declared a lookup tool"}]},` +
	`{"role":"user","content":[{"type":"text","text":"use it now"}]}]}`

// toolReferenceBody carries a tool_addition that references an already-declared
// tool (tool_reference), which CN endpoints do accept and must be kept.
const toolReferenceBody = `{"model":"claude-opus-5-5","max_tokens":1024,"messages":[` +
	`{"role":"assistant","content":[` +
	`{"type":"tool_addition","tool":{"type":"tool_reference","tool_name":"lookup"}},` +
	`{"type":"text","text":"use lookup"}]}]}`

// plainBody has no tool changes at all.
const plainToolBody = `{"model":"claude-opus-5-5","max_tokens":1024,"messages":[` +
	`{"role":"user","content":[{"type":"text","text":"hello"}]}]}`

func TestFilterInlineToolDefinitionBlocks(t *testing.T) {
	t.Run("strips inline tool definition", func(t *testing.T) {
		out := FilterInlineToolDefinitionBlocks([]byte(inlineToolDefinitionBody))
		var types []string
		for _, block := range gjson.GetBytes(out, "messages.1.content").Array() {
			types = append(types, block.Get("type").String())
		}
		require.Equal(t, []string{"text"}, types)
		require.Equal(t, "I have declared a lookup tool",
			gjson.GetBytes(out, "messages.1.content.0.text").String())
	})

	t.Run("keeps tool reference", func(t *testing.T) {
		out := FilterInlineToolDefinitionBlocks([]byte(toolReferenceBody))
		require.Equal(t, toolReferenceBody, string(out))
	})

	t.Run("unchanged without tool definition", func(t *testing.T) {
		out := FilterInlineToolDefinitionBlocks([]byte(plainToolBody))
		require.Equal(t, plainToolBody, string(out))
	})

	t.Run("placeholder when assistant content becomes empty", func(t *testing.T) {
		body := `{"messages":[{"role":"assistant","content":[{"type":"tool_definition","name":"x","input_schema":{}}]}]}`
		out := FilterInlineToolDefinitionBlocks([]byte(body))
		require.Equal(t, "(assistant content removed)",
			gjson.GetBytes(out, "messages.0.content.0.text").String())
	})
}
