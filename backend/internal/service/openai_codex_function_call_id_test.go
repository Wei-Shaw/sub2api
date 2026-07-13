//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestFilterCodexInput_StripsFunctionCallItemID_WhenPreservingReferences
// verifies that function_call items with non-fc id (e.g. item_*) have their
// id stripped even when PreserveReferences is true. OpenAI upstream requires
// function_call ids to begin with "fc" and rejects item_* with 400:
// "Expected an ID that begins with 'fc'." (#3785)
func TestFilterCodexInput_StripsFunctionCallItemID_WhenPreservingReferences(t *testing.T) {
	input := []any{
		map[string]any{
			"type":    "function_call",
			"id":      "item_A9v0SNfS3VaLrfX0j3y4xhyK",
			"call_id": "fc_abc123",
			"name":    "bash",
		},
		map[string]any{
			"type":    "function_call_output",
			"call_id": "fc_abc123",
			"output":  "done",
		},
	}

	filtered := filterCodexInputWithOptions(input, codexInputFilterOptions{
		PreserveReferences: true,
	})

	require.Len(t, filtered, 2)

	fc, ok := filtered[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "function_call", fc["type"])
	_, hasID := fc["id"]
	require.False(t, hasID, "item_* id should be stripped from function_call")
	require.Equal(t, "fc_abc123", fc["call_id"], "call_id must be preserved")
	require.Equal(t, "bash", fc["name"])
}

// TestFilterCodexInput_KeepsFcID_WhenPreservingReferences
// verifies that function_call items with a valid fc* id are kept when
// PreserveReferences is true.
func TestFilterCodexInput_KeepsFcID_WhenPreservingReferences(t *testing.T) {
	input := []any{
		map[string]any{
			"type":    "function_call",
			"id":      "fc_validID123",
			"call_id": "fc_validID123",
			"name":    "bash",
		},
	}

	filtered := filterCodexInputWithOptions(input, codexInputFilterOptions{
		PreserveReferences: true,
	})

	require.Len(t, filtered, 1)
	fc, ok := filtered[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "fc_validID123", fc["id"], "valid fc* id must be preserved")
}

// TestFilterCodexInput_StripsItemIDFromAllToolCallInputTypes verifies that
// item_* ids are stripped from all call-input types (not output types).
func TestFilterCodexInput_StripsItemIDFromAllToolCallInputTypes(t *testing.T) {
	types := []string{"function_call", "tool_call", "local_shell_call", "custom_tool_call", "mcp_tool_call"}

	for _, typ := range types {
		input := []any{
			map[string]any{
				"type":    typ,
				"id":      "item_xyz",
				"call_id": "fc_001",
				"name":    "tool",
			},
		}
		filtered := filterCodexInputWithOptions(input, codexInputFilterOptions{
			PreserveReferences: true,
		})
		require.Len(t, filtered, 1)
		item, ok := filtered[0].(map[string]any)
		require.True(t, ok)
		_, hasID := item["id"]
		require.False(t, hasID, "item_* id should be stripped from %s", typ)
	}
}

// TestFilterCodexInput_OutputTypeKeepsItemID ensures tool-output items
// (e.g. function_call_output) keep their id — only call-input types have
// the fc* constraint.
func TestFilterCodexInput_OutputTypeKeepsItemID(t *testing.T) {
	input := []any{
		map[string]any{
			"type":    "function_call_output",
			"id":      "o1",
			"call_id": "fc_abc",
			"output":  "done",
		},
	}

	filtered := filterCodexInputWithOptions(input, codexInputFilterOptions{
		PreserveReferences: true,
	})

	require.Len(t, filtered, 1)
	out, ok := filtered[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "o1", out["id"], "output item id should be preserved")
}

// TestFilterCodexInput_NonToolCallItemKeepsID ensures items subject to neither
// the fc* (call-input) nor the msg* (message) prefix rule still keep their id
// when PreserveReferences is true.
// message is covered separately in openai_codex_message_item_id_test.go (#3981).
func TestFilterCodexInput_NonToolCallItemKeepsID(t *testing.T) {
	input := []any{
		map[string]any{
			"type": "web_search_call",
			"id":   "ws_001",
		},
	}

	filtered := filterCodexInputWithOptions(input, codexInputFilterOptions{
		PreserveReferences: true,
	})

	require.Len(t, filtered, 1)
	item, ok := filtered[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "ws_001", item["id"], "unconstrained items keep their id in preserve mode")
}
