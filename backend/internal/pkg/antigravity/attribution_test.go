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
				userJSON, err := json.Marshal(attribution)
				require.NoError(t, err)
				input := &ClaudeRequest{
					Model: "gemini-3.8-flash-high", System: systemJSON,
					Messages: []ClaudeMessage{{Role: "user", Content: userJSON}},
					Tools: []ClaudeTool{{Name: "example", Description: attribution,
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
				require.Equal(t, attribution, got.Request.Contents[0].Parts[0].Text)
				require.Equal(t, attribution, got.Request.Tools[0].FunctionDeclarations[0].Description)
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
