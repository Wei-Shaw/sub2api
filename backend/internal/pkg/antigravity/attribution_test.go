package antigravity

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTransformClaudeToGemini_AttributionSystemText(t *testing.T) {
	const attribution = "x-anthropic-billing-header: cc_version=2.1.271.4bf; cc_entrypoint=claude-desktop-3p;"
	tests := []struct {
		name string
		text string
		want string
	}{
		{"metadata only", attribution, ""},
		{"metadata with newline", attribution + "\n", ""},
		{"leading whitespace", " \t\n" + attribution, ""},
		{"following instructions", attribution + "\nKeep these instructions.", "Keep these instructions."},
		{"CRLF", attribution + "\r\n  Keep indentation.\n", "  Keep indentation.\n"},
		{"CR", attribution + "\rKeep these instructions.", "Keep these instructions."},
		{"blank lines preserved", attribution + "\n\nKeep these instructions.", "\nKeep these instructions."},
		{"ordinary text", "  Keep these instructions.\n", "  Keep these instructions.\n"},
		{"no colon", "x-anthropic-billing-header keep", "x-anthropic-billing-header keep"},
		{"different field", "x-anthropic-billing-header-extra: keep", "x-anthropic-billing-header-extra: keep"},
		{"quoted within instructions", "Explain this metadata: " + attribution, "Explain this metadata: " + attribution},
		{"later line", "Example:\n" + attribution, "Example:\n" + attribution},
		{"Codex GPT 5", "You are Codex, based on GPT-5. Keep GPT-5 model ids.", "You are an AI agent. Keep GPT-5 model ids."},
		{"Codex GPT 5.6", "You are Codex, a coding agent based on GPT-5.6.\nKeep indentation.", "You are an AI agent.\nKeep indentation."},
		{"Codex GPT 6", "You are Codex, based on GPT-6.", "You are an AI agent."},
		{"Codex GPT 6 agent", "You are Codex, an agent based on GPT-6. Rest.", "You are an AI agent. Rest."},
		{"CLI comma", "You are GPT-5.6 running in the Codex CLI, a terminal-based coding assistant.", "You are an AI agent, a terminal-based coding assistant."},
		{"Codex model suffix", "You are Codex, based on GPT-5.6-Codex. Rest.", "You are an AI agent. Rest."},
		{"CLI GPT 5.6", "You are GPT-5.6 running in the Codex CLI. Rest.", "You are an AI agent. Rest."},
		{"CLI GPT 6", " \nYou are GPT-6 running in the Codex CLI\nRest.", "You are an AI agent.\nRest."},
		{"Claude CLI", "You are Claude Code, Anthropic's official CLI for Claude. Rest.", "You are an AI agent. Rest."},
		{"Claude SDK", "You are a Claude agent, built on Anthropic's Claude Agent SDK. Rest.", "You are an AI agent. Rest."},
		{"attribution before GPT", attribution + "\r\nYou are Codex, based on GPT-6. Rest.", "You are an AI agent. Rest."},
		{"quoted GPT", `"You are Codex, based on GPT-6."`, `"You are Codex, based on GPT-6."`},
		{"later GPT", "Example:\nYou are GPT-6 running in the Codex CLI.", "Example:\nYou are GPT-6 running in the Codex CLI."},
		{"vendor mentions", "Use OpenAI GPT-5.6 and Anthropic Claude; preserve gpt-6.", "Use OpenAI GPT-5.6 and Anthropic Claude; preserve gpt-6."},
		{"not identity", "You are Codex, do not claim to be based on GPT-6.", "You are Codex, do not claim to be based on GPT-6."},
		{"model token boundary", "You are Codex, based on GPT-6example.", "You are Codex, based on GPT-6example."},
	}
	for _, tt := range tests {
		for _, arrayForm := range []bool{false, true} {
			name := tt.name + "/string"
			var system any = tt.text
			if arrayForm {
				name = tt.name + "/array"
				system = []SystemBlock{{Type: "text", Text: tt.text}, {Type: "text", Text: "Keep the next block."}}
			}
			t.Run(name, func(t *testing.T) {
				systemJSON, err := json.Marshal(system)
				require.NoError(t, err)
				userText := strings.TrimSpace(tt.text)
				userJSON, err := json.Marshal(userText)
				require.NoError(t, err)
				input := &ClaudeRequest{
					Model: "gemini-3.8-flash-high", System: systemJSON,
					Messages: []ClaudeMessage{{Role: "user", Content: userJSON}},
					Tools: []ClaudeTool{{Name: "example", Description: tt.text,
						InputSchema: map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}}}},
				}
				before, err := json.Marshal(input)
				require.NoError(t, err)
				body, err := TransformClaudeToGeminiWithOptions(input, "test-project", input.Model, TransformOptions{})
				require.NoError(t, err)
				var got V1InternalRequest
				require.NoError(t, json.Unmarshal(body, &got))
				var texts []string
				for _, part := range got.Request.SystemInstruction.Parts {
					if part.Text != "\n--- [SYSTEM_PROMPT_END] ---" {
						texts = append(texts, part.Text)
					}
				}
				var want []string
				if tt.want != "" {
					want = append(want, tt.want)
				}
				if arrayForm {
					want = append(want, "Keep the next block.")
				}
				require.Equal(t, want, texts)
				// The same literal text in user content and tool descriptions is not metadata.
				require.Equal(t, userText, got.Request.Contents[0].Parts[0].Text)
				require.Equal(t, tt.text, got.Request.Tools[0].FunctionDeclarations[0].Description)
				after, err := json.Marshal(input)
				require.NoError(t, err)
				require.JSONEq(t, string(before), string(after), "do not mutate the incoming request")
			})
		}
	}
}

func TestTransformClaudeToGemini_AttributionDoesNotHideIdentity(t *testing.T) {
	system, err := json.Marshal("x-anthropic-billing-header: cc_version=example;\nYou are Antigravity. Keep this identity.")
	require.NoError(t, err)
	got := buildSystemInstruction(system, "gemini-3.8-flash-high", DefaultTransformOptions(), nil)
	require.Len(t, got.Parts, 1)
	require.Equal(t, "You are Antigravity. Keep this identity.", got.Parts[0].Text)
	require.False(t, strings.Contains(got.Parts[0].Text, "cc_version="))
}

func TestNeutralizeClaudeIdentity(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "agent sdk opener",
			text: "You are a Claude agent, built on Anthropic's Claude Agent SDK. You are naming a coding session.",
			want: "You are an AI agent. You are naming a coding session.",
		},
		{
			name: "cli opener",
			text: "You are Claude Code, Anthropic's official CLI for Claude. Help with engineering tasks.",
			want: "You are an AI agent. Help with engineering tasks.",
		},
		{
			name: "leading whitespace",
			text: "\n  You are a Claude agent, built on Anthropic's Claude Agent SDK.\nRest.",
			want: "You are an AI agent.\nRest.",
		},
		// User instructions that merely mention the vendor must stay untouched:
		// they are content, not a client identity declaration.
		{
			name: "mention inside instructions",
			text: "Follow CLAUDE.md. Prefer claude-sonnet-4-6 for long tasks.",
			want: "Follow CLAUDE.md. Prefer claude-sonnet-4-6 for long tasks.",
		},
		{
			name: "opener not at start",
			text: "Example: You are a Claude agent, built on Anthropic's Claude Agent SDK.",
			want: "Example: You are a Claude agent, built on Anthropic's Claude Agent SDK.",
		},
		{name: "ordinary text", text: "Keep these instructions.", want: "Keep these instructions."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, NeutralizeSystemIdentity(tt.text))
		})
	}
}

// The attribution line and the identity sentence arrive as separate system
// blocks in real Claude Code traffic; both have to be handled.
func TestAttributionAndIdentityTogether(t *testing.T) {
	attribution := "x-anthropic-billing-header: cc_version=2.1.278.0a9; cc_entrypoint=claude-vscode;"
	identity := "You are a Claude agent, built on Anthropic's Claude Agent SDK."
	require.Equal(t, "", stripClaudeAttribution(attribution))
	require.Equal(t, "You are an AI agent.", NeutralizeSystemIdentity(identity))
}

// End-to-end through the transformer: the attribution block and the identity
// sentence arrive as separate system blocks in real Claude Code traffic. The
// same words inside user content must survive untouched, because there they are
// conversation content rather than a client identity declaration.
func TestTransformClaudeToGemini_IdentitySystemBlock(t *testing.T) {
	const (
		attribution = "x-anthropic-billing-header: cc_version=2.1.278.0a9; cc_entrypoint=claude-vscode;"
		identity    = "You are a Claude agent, built on Anthropic's Claude Agent SDK."
		task        = "You are naming a coding session."
		userText    = "Does Claude Code support claude-sonnet-4-6?"
	)
	system := []SystemBlock{
		{Type: "text", Text: attribution},
		{Type: "text", Text: identity},
		{Type: "text", Text: task},
	}
	systemJSON, err := json.Marshal(system)
	require.NoError(t, err)
	userJSON, err := json.Marshal(userText)
	require.NoError(t, err)
	input := &ClaudeRequest{
		Model:    "gemini-3.8-flash-high",
		System:   systemJSON,
		Messages: []ClaudeMessage{{Role: "user", Content: userJSON}},
	}
	body, err := TransformClaudeToGeminiWithOptions(input, "test-project", input.Model, TransformOptions{})
	require.NoError(t, err)
	var got V1InternalRequest
	require.NoError(t, json.Unmarshal(body, &got))

	var texts []string
	for _, part := range got.Request.SystemInstruction.Parts {
		if part.Text != "\n--- [SYSTEM_PROMPT_END] ---" {
			texts = append(texts, part.Text)
		}
	}
	require.Equal(t, []string{"You are an AI agent.", task}, texts)
	for _, text := range texts {
		require.NotContains(t, text, "Claude")
		require.NotContains(t, text, "Anthropic")
	}
	require.Equal(t, userText, got.Request.Contents[0].Parts[0].Text)
}
