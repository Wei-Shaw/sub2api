package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func toolNames(t *testing.T, reqBody map[string]any) []string {
	t.Helper()
	raw, ok := reqBody["tools"].([]any)
	if !ok {
		return nil
	}
	names := make([]string, 0, len(raw))
	for _, tool := range raw {
		names = append(names, codexToolName(tool))
	}
	return names
}

func TestIsCodexConnectorToolName(t *testing.T) {
	connector := []string{
		"codex_apps.github.get_repo",
		"codex_apps__github__get_repo",
		"codex_apps-github-get_repo",
		"CODEX_APPS.github",
		"codex_apps",
		"app/list",
		"mcpServerStatus/list",
		"list_connectors",
		"connectors/list",
		// The whole codex_apps namespace is reserved/blocked, including any
		// sub-tool under it, so a separator-suffixed name is also a connector.
		"codex_apps_github_get_repo",
	}
	for _, name := range connector {
		require.Truef(t, isCodexConnectorToolName(name), "expected %q to be a connector tool", name)
	}

	regular := []string{
		"",
		"shell",
		"apply_patch",
		"web_search",
		"get_weather",
		"codexapps",     // no separator: not the reserved namespace
		"my_codex_apps", // suffix match must not trigger
	}
	for _, name := range regular {
		require.Falsef(t, isCodexConnectorToolName(name), "expected %q NOT to be a connector tool", name)
	}
}

func TestStripCodexConnectorTools_RemovesConnectorKeepsRegular(t *testing.T) {
	reqBody := map[string]any{
		"tools": []any{
			map[string]any{"type": "function", "name": "apply_patch"},
			map[string]any{"type": "function", "name": "codex_apps.github.get_repo"},
			// ChatCompletions-style nested shape should also be matched by name.
			map[string]any{"type": "function", "function": map[string]any{"name": "codex_apps.gmail.search"}},
			map[string]any{"type": "function", "name": "shell"},
		},
	}

	removed := stripCodexConnectorTools(reqBody)
	require.ElementsMatch(t, []string{"codex_apps.github.get_repo", "codex_apps.gmail.search"}, removed)
	require.ElementsMatch(t, []string{"apply_patch", "shell"}, toolNames(t, reqBody))
}

func TestStripCodexConnectorTools_NoToolsOrNoMatch(t *testing.T) {
	// No tools field.
	require.Nil(t, stripCodexConnectorTools(map[string]any{}))

	// Tools present but none are connectors.
	reqBody := map[string]any{
		"tools": []any{
			map[string]any{"type": "function", "name": "apply_patch"},
		},
	}
	require.Nil(t, stripCodexConnectorTools(reqBody))
	require.ElementsMatch(t, []string{"apply_patch"}, toolNames(t, reqBody))
}

func TestStripCodexConnectorTools_ResetsToolChoicePinningRemovedTool(t *testing.T) {
	reqBody := map[string]any{
		"tools": []any{
			map[string]any{"type": "function", "name": "codex_apps.github.get_repo"},
			map[string]any{"type": "function", "name": "apply_patch"},
		},
		"tool_choice": map[string]any{"type": "function", "name": "codex_apps.github.get_repo"},
	}

	removed := stripCodexConnectorTools(reqBody)
	require.Equal(t, []string{"codex_apps.github.get_repo"}, removed)
	require.Equal(t, "auto", reqBody["tool_choice"])
}

func TestStripCodexConnectorTools_KeepsToolChoiceForSurvivingTool(t *testing.T) {
	reqBody := map[string]any{
		"tools": []any{
			map[string]any{"type": "function", "name": "codex_apps.github.get_repo"},
			map[string]any{"type": "function", "name": "apply_patch"},
		},
		"tool_choice": map[string]any{"type": "function", "name": "apply_patch"},
	}

	stripCodexConnectorTools(reqBody)
	choice, ok := reqBody["tool_choice"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "apply_patch", choice["name"])
}

func TestApplyCodexOAuthTransform_BlockConnectorToolsOption(t *testing.T) {
	makeBody := func() map[string]any {
		return map[string]any{
			"model": "gpt-5.2",
			"tools": []any{
				map[string]any{"type": "function", "name": "codex_apps.github.get_repo"},
				map[string]any{"type": "function", "name": "apply_patch"},
			},
		}
	}

	// Default options: connector tool is forwarded unchanged (backward compatible).
	bodyDefault := makeBody()
	applyCodexOAuthTransform(bodyDefault, false, false)
	require.ElementsMatch(t,
		[]string{"codex_apps.github.get_repo", "apply_patch"},
		toolNames(t, bodyDefault),
	)

	// With BlockConnectorTools enabled: connector tool is stripped.
	bodyBlocked := makeBody()
	applyCodexOAuthTransformWithOptions(bodyBlocked, codexOAuthTransformOptions{BlockConnectorTools: true})
	require.ElementsMatch(t, []string{"apply_patch"}, toolNames(t, bodyBlocked))
}
