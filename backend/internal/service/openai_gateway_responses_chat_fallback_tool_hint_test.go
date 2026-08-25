package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
)

func TestApplyClaudeCodeModeToolOutputHint_OnlyTargetsClaudeExec(t *testing.T) {
	mapping := apicompat.ResponsesClientToolMapping{
		NamespaceTools: map[string]apicompat.ResponsesNamespaceName{
			"functions__exec": {Namespace: "functions", Name: "exec", Custom: true},
		},
	}
	req := &apicompat.ChatCompletionsRequest{
		Model: "claude-opus-5",
		Tools: []apicompat.ChatTool{{
			Type: "function",
			Function: &apicompat.ChatFunction{
				Name:        "functions__exec",
				Description: "Execute code",
			},
		}},
	}

	applyClaudeCodeModeToolOutputHint(req, mapping)
	require.Contains(t, req.Tools[0].Function.Description, "notify(result.output)")

	other := &apicompat.ChatCompletionsRequest{
		Model: "glm-5.2",
		Tools: []apicompat.ChatTool{{
			Type:     "function",
			Function: &apicompat.ChatFunction{Name: "functions__exec", Description: "Execute code"},
		}},
	}
	applyClaudeCodeModeToolOutputHint(other, mapping)
	require.NotContains(t, other.Tools[0].Function.Description, "notify(result.output)")

	ordinaryExec := &apicompat.ChatCompletionsRequest{
		Model: "claude-opus-5",
		Tools: []apicompat.ChatTool{{
			Type: "function",
			Function: &apicompat.ChatFunction{
				Name:        "database__exec",
				Description: "Run a database statement",
			},
		}},
	}
	applyClaudeCodeModeToolOutputHint(ordinaryExec, apicompat.ResponsesClientToolMapping{
		FunctionTools: map[string]bool{"database__exec": true},
	})
	require.NotContains(t, ordinaryExec.Tools[0].Function.Description, "notify(result.output)")
}

func TestApplyClaudeCodeModeInstructionsHint_OnlyTargetsClaude(t *testing.T) {
	mapping := apicompat.ResponsesClientToolMapping{CustomTools: map[string]bool{"exec": true}}
	claude := &apicompat.ResponsesRequest{Model: "claude-opus-5", Instructions: "base"}
	applyClaudeCodeModeInstructionsHint(claude, "claude-opus-5", mapping)
	require.Contains(t, claude.Instructions, "notify(result.output)")

	other := &apicompat.ResponsesRequest{Model: "glm-5.2", Instructions: "base"}
	applyClaudeCodeModeInstructionsHint(other, "glm-5.2", mapping)
	require.Equal(t, "base", other.Instructions)

	mappedToClaude := &apicompat.ResponsesRequest{Model: "public-alias", Instructions: "base"}
	applyClaudeCodeModeInstructionsHint(mappedToClaude, "claude-sonnet-5", mapping)
	require.Contains(t, mappedToClaude.Instructions, "notify(result.output)")

	mappedAwayFromClaude := &apicompat.ResponsesRequest{Model: "claude-alias", Instructions: "base"}
	applyClaudeCodeModeInstructionsHint(mappedAwayFromClaude, "glm-5.2", mapping)
	require.Equal(t, "base", mappedAwayFromClaude.Instructions)

	withoutExec := &apicompat.ResponsesRequest{Model: "claude-opus-5", Instructions: "base"}
	applyClaudeCodeModeInstructionsHint(withoutExec, "claude-opus-5", apicompat.ResponsesClientToolMapping{
		FunctionTools: map[string]bool{"database__exec": true},
	})
	require.Equal(t, "base", withoutExec.Instructions)
}

func TestEnableChatFallbackCodeModeExecNormalizationOnlyGLMUpstream(t *testing.T) {
	mapping := apicompat.ResponsesClientToolMapping{CustomTools: map[string]bool{"exec": true}}

	glm := enableChatFallbackCodeModeExecNormalization(mapping, "glm-5.2")
	require.True(t, glm.CodeModeExecTools["exec"])

	zhipu := enableChatFallbackCodeModeExecNormalization(mapping, "zhipu/glm-5.2")
	require.True(t, zhipu.CodeModeExecTools["exec"])

	for _, model := range []string{"deepseek-v4-flash", "kimi-k2.6", "claude-sonnet-5"} {
		other := enableChatFallbackCodeModeExecNormalization(mapping, model)
		require.Empty(t, other.CodeModeExecTools, model)
	}
}
