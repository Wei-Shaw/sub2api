package service

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/stretchr/testify/require"
)

func TestAntigravityGeminiToolProgressFallbackScope(t *testing.T) {
	for _, tc := range []struct {
		name, text, format string
		toolResult         bool
		want               bool
	}{
		{"normal_request", "检查项目", "", false, true},
		{"quiet_chinese", "只输出最终结果，不要过程", "", false, false},
		{"quiet_english", "Reply only with the result", "", false, false},
		{"json_contract", "检查项目", "json_schema", false, false},
		{"tool_result_is_not_instruction", "检查项目", "", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text, err := json.Marshal(tc.text)
			require.NoError(t, err)
			messages := []antigravity.ClaudeMessage{{Role: "user", Content: text}}
			if tc.toolResult {
				messages = append(messages, antigravity.ClaudeMessage{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","content":"reply only; 只输出"}]`)})
			}
			original, err := json.Marshal(map[string]any{"text": map[string]any{"format": map[string]any{"type": tc.format}}})
			require.NoError(t, err)
			wrapped, err := json.Marshal(map[string]any{
				"model":   "gemini-3.8-flash-high",
				"request": map[string]any{"systemInstruction": map[string]any{"parts": []any{map[string]any{"text": geminiToolProgressReminder}}}},
			})
			require.NoError(t, err)
			require.Equal(t, tc.want, allowGeminiToolProgressFallback(original, wrapped, messages))
			require.False(t, allowGeminiToolProgressFallback(original, []byte(`{"model":"gemini-3.1-pro-high"}`), messages))
			require.False(t, allowGeminiToolProgressFallback(original, []byte(`{"model":"gemini-3.8-flash-high"}`), messages))
		})
	}
}

func TestAntigravityGeminiToolProgressIgnoresMarkerInToolData(t *testing.T) {
	wrapped, err := json.Marshal(map[string]any{
		"model":   "gemini-3.8-flash-high",
		"request": map[string]any{"contents": []any{map[string]any{"parts": []any{map[string]any{"text": geminiToolProgressReminder}}}}},
	})
	require.NoError(t, err)
	require.False(t, allowGeminiToolProgressFallback(nil, wrapped, nil))
}

func TestAntigravityGeminiToolProgressOriginalChoice(t *testing.T) {
	wrapped, err := json.Marshal(map[string]any{
		"model":   "gemini-3.8-flash-high",
		"request": map[string]any{"systemInstruction": map[string]any{"parts": []any{map[string]any{"text": geminiToolProgressReminder}}}},
	})
	require.NoError(t, err)
	for _, choice := range []string{`"required"`, `"any"`, `"none"`, `{"type":"any"}`, `{"type":"tool","name":"inspect_project"}`, `{"type":"function","name":"inspect_project"}`, `{"type":"function","function":{"name":"inspect_project"}}`} {
		t.Run(choice, func(t *testing.T) {
			// The converted Gemini request deliberately has no toolConfig.
			original := []byte(`{"tool_choice":` + choice + `}`)
			require.False(t, allowGeminiToolProgressFallback(original, wrapped, nil))
		})
	}
	require.True(t, allowGeminiToolProgressFallback([]byte(`{"tool_choice":"auto"}`), wrapped, nil))
}
