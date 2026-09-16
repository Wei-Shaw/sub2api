//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestFilterCodexInput_StripsMessageItemID_WhenPreservingReferences
// verifies that message items with a non-msg id (e.g. item_*) have their id
// stripped even when PreserveReferences is true. OpenAI upstream requires
// message ids to begin with "msg" and rejects item_* with 400:
// "Expected an ID that begins with 'msg'." (#3981)
func TestFilterCodexInput_StripsMessageItemID_WhenPreservingReferences(t *testing.T) {
	input := []any{
		map[string]any{
			"type": "message",
			"id":   "item_3bc5a3fa8ccde25f1c0000d4",
			"role": "user",
			"content": []any{
				map[string]any{"type": "input_text", "text": "hello"},
			},
		},
	}

	filtered := filterCodexInputWithOptions(input, codexInputFilterOptions{
		PreserveReferences: true,
	})

	require.Len(t, filtered, 1)

	msg, ok := filtered[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "message", msg["type"])
	_, hasID := msg["id"]
	require.False(t, hasID, "item_* id should be stripped from message")
	require.Equal(t, "user", msg["role"], "role must be preserved")
	require.NotNil(t, msg["content"], "content must be preserved")
}

// TestFilterCodexInput_KeepsMsgID_WhenPreservingReferences
// verifies that message items with a valid msg* id are kept when
// PreserveReferences is true, so context references are not lost.
func TestFilterCodexInput_KeepsMsgID_WhenPreservingReferences(t *testing.T) {
	input := []any{
		map[string]any{
			"type": "message",
			"id":   "msg_validID123",
			"role": "assistant",
		},
	}

	filtered := filterCodexInputWithOptions(input, codexInputFilterOptions{
		PreserveReferences: true,
	})

	require.Len(t, filtered, 1)
	msg, ok := filtered[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "msg_validID123", msg["id"], "valid msg* id must be preserved")
}

// TestFilterCodexInput_StripsMessageIDWhenNotPreservingReferences ensures the
// non-continuation path still drops every message id regardless of prefix.
func TestFilterCodexInput_StripsMessageIDWhenNotPreservingReferences(t *testing.T) {
	for _, id := range []string{"item_abc", "msg_validID123"} {
		input := []any{
			map[string]any{
				"type": "message",
				"id":   id,
				"role": "user",
			},
		}

		filtered := filterCodexInputWithOptions(input, codexInputFilterOptions{
			PreserveReferences: false,
		})

		require.Len(t, filtered, 1)
		msg, ok := filtered[0].(map[string]any)
		require.True(t, ok)
		_, hasID := msg["id"]
		require.False(t, hasID, "id %q should be stripped when not preserving references", id)
	}
}

// TestFilterCodexInput_MessageIDStripDoesNotMutateInput ensures the original
// input map is not modified in place when the id is stripped.
func TestFilterCodexInput_MessageIDStripDoesNotMutateInput(t *testing.T) {
	original := map[string]any{
		"type": "message",
		"id":   "item_abc",
		"role": "user",
	}

	filtered := filterCodexInputWithOptions([]any{original}, codexInputFilterOptions{
		PreserveReferences: true,
	})

	require.Len(t, filtered, 1)
	require.Equal(t, "item_abc", original["id"], "original input must not be mutated")
}

// TestFilterCodexInput_MessageStripKeepsFunctionCallBehavior guards against a
// regression of #3785: message and function_call id rules are independent.
func TestFilterCodexInput_MessageStripKeepsFunctionCallBehavior(t *testing.T) {
	input := []any{
		map[string]any{
			"type": "message",
			"id":   "item_msg_001",
			"role": "user",
		},
		map[string]any{
			"type":    "function_call",
			"id":      "fc_validID123",
			"call_id": "fc_validID123",
			"name":    "bash",
		},
		map[string]any{
			"type":    "function_call",
			"id":      "item_A9v0SNfS3VaLrfX0j3y4xhyK",
			"call_id": "fc_abc123",
			"name":    "bash",
		},
		map[string]any{
			"type":    "function_call_output",
			"id":      "o1",
			"call_id": "fc_abc123",
			"output":  "done",
		},
	}

	filtered := filterCodexInputWithOptions(input, codexInputFilterOptions{
		PreserveReferences: true,
	})

	require.Len(t, filtered, 4)

	msg, ok := filtered[0].(map[string]any)
	require.True(t, ok)
	_, hasID := msg["id"]
	require.False(t, hasID, "message item_* id should be stripped")

	fcValid, ok := filtered[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "fc_validID123", fcValid["id"], "valid fc* id must be preserved")

	fcBad, ok := filtered[2].(map[string]any)
	require.True(t, ok)
	_, hasID = fcBad["id"]
	require.False(t, hasID, "function_call item_* id should still be stripped")
	require.Equal(t, "fc_abc123", fcBad["call_id"], "call_id pairing must survive")

	out, ok := filtered[3].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "o1", out["id"], "output item id should be preserved")
	require.Equal(t, "fc_abc123", out["call_id"], "call_id pairing must survive")
}
