package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
)

func TestApplyClaudeCodeModeToolOutputHint_OnlyTargetsClaudeExec(t *testing.T) {
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

	applyClaudeCodeModeToolOutputHint(req)
	require.Contains(t, req.Tools[0].Function.Description, "notify(result.output)")

	other := &apicompat.ChatCompletionsRequest{
		Model: "glm-5.2",
		Tools: []apicompat.ChatTool{{
			Type:     "function",
			Function: &apicompat.ChatFunction{Name: "functions__exec", Description: "Execute code"},
		}},
	}
	applyClaudeCodeModeToolOutputHint(other)
	require.NotContains(t, other.Tools[0].Function.Description, "notify(result.output)")
}

func TestApplyClaudeCodeModeInstructionsHint_OnlyTargetsClaude(t *testing.T) {
	claude := &apicompat.ResponsesRequest{Model: "claude-opus-5", Instructions: "base"}
	applyClaudeCodeModeInstructionsHint(claude)
	require.Contains(t, claude.Instructions, "notify(result.output)")

	other := &apicompat.ResponsesRequest{Model: "glm-5.2", Instructions: "base"}
	applyClaudeCodeModeInstructionsHint(other)
	require.Equal(t, "base", other.Instructions)
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
