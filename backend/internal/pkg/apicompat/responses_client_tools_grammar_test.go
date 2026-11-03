package apicompat

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const testApplyPatchGrammar = "start: begin_patch hunk+ end_patch\nbegin_patch: \"*** Begin Patch\" LF"

func TestAdaptResponsesClientTools_KeepsCustomToolGrammarInDescription(t *testing.T) {
	req := map[string]any{
		"tools": []any{
			map[string]any{
				"type":        "custom",
				"name":        "apply_patch",
				"description": "Use the `apply_patch` tool to edit files.",
				"format": map[string]any{
					"type":       "grammar",
					"syntax":     "lark",
					"definition": testApplyPatchGrammar,
				},
			},
		},
	}

	_, changed, err := AdaptResponsesClientTools(req)
	require.NoError(t, err)
	require.True(t, changed)

	tools := requireResponsesClientToolValue[[]any](t, req["tools"])
	tool := requireResponsesClientToolValue[map[string]any](t, tools[0])
	require.Equal(t, "function", tool["type"])
	require.NotContains(t, tool, "format")
	description := requireResponsesClientToolValue[string](t, tool["description"])
	require.Contains(t, description, "Use the `apply_patch` tool to edit files.\n\n")
	require.Contains(t, description, "must match this lark grammar")
	require.Contains(t, description, "```\n"+testApplyPatchGrammar+"\n```")
}

func TestAdaptResponsesClientTools_LeavesDescriptionWithoutGrammarDefinition(t *testing.T) {
	req := map[string]any{
		"tools": []any{
			map[string]any{"type": "custom", "name": "exec", "description": "Run a command.", "format": map[string]any{"type": "grammar"}},
			map[string]any{"type": "custom", "name": "note", "description": "Free text.", "format": map[string]any{"type": "text"}},
			map[string]any{"type": "custom", "name": "raw"},
		},
	}

	_, _, err := AdaptResponsesClientTools(req)
	require.NoError(t, err)

	tools := requireResponsesClientToolValue[[]any](t, req["tools"])
	require.Equal(t, "Run a command.", requireResponsesClientToolValue[map[string]any](t, tools[0])["description"])
	require.Equal(t, "Free text.", requireResponsesClientToolValue[map[string]any](t, tools[1])["description"])
	require.NotContains(t, requireResponsesClientToolValue[map[string]any](t, tools[2]), "description")
}

// A WS continuation re-adapts declarations restored from the lowered form; the
// grammar must not be appended a second time.
func TestAdaptResponsesClientTools_GrammarDescriptionNotDuplicatedOnReadapt(t *testing.T) {
	req := map[string]any{
		"tools": []any{
			map[string]any{
				"type": "custom", "name": "apply_patch", "description": "Edit files.",
				"format": map[string]any{"type": "grammar", "syntax": "lark", "definition": testApplyPatchGrammar},
			},
		},
	}
	mapping, _, err := AdaptResponsesClientTools(req)
	require.NoError(t, err)
	lowered := requireResponsesClientToolValue[[]any](t, req["tools"])
	first := requireResponsesClientToolValue[map[string]any](t, lowered[0])["description"]

	again := map[string]any{"tools": restoreInheritedResponsesClientToolDeclarations(lowered, mapping)}
	_, _, err = AdaptResponsesClientTools(again)
	require.NoError(t, err)
	tools := requireResponsesClientToolValue[[]any](t, again["tools"])
	require.Equal(t, first, requireResponsesClientToolValue[map[string]any](t, tools[0])["description"])
}
