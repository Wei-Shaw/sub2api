package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAntigravityGeminiToolProgressReminder(t *testing.T) {
	for _, tc := range []struct {
		name, model, instruction, mode string
		functionTools                  bool
		wantReminder                   bool
	}{
		{"desktop_multi_turn", "gemini-3.8-flash-high", "Before each tool batch, briefly state its purpose.", "AUTO", true, true},
		{"validated_tools", "gemini-3.8-flash", "Before each tool batch, briefly state its purpose.", "VALIDATED", true, true},
		{"other_model", "gemini-3.1-pro-high", "Before each tool batch, briefly state its purpose.", "AUTO", true, false},
		{"no_progress_requested", "gemini-3.8-flash-high", "Return the requested result.", "AUTO", true, false},
		{"no_function_tools", "gemini-3.8-flash-high", "Before each tool batch, briefly state its purpose.", "AUTO", false, false},
		{"forced_function_only", "gemini-3.8-flash-high", "Before each tool batch, briefly state its purpose.", "ANY", true, false},
		{"tools_disabled", "gemini-3.8-flash-high", "Before each tool batch, briefly state its purpose.", "NONE", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parts := []any{map[string]any{"text": tc.instruction}, map[string]any{"text": "Project context and tool history follow. " + strings.Repeat("context ", 100)}}
			request := map[string]any{
				"systemInstruction": map[string]any{"parts": parts},
				"contents":          []any{map[string]any{"role": "user", "parts": []any{map[string]any{"functionResponse": map[string]any{"name": "inspect_project", "response": map[string]any{"version": "1.0.0"}}}}}},
				"toolConfig":        map[string]any{"functionCallingConfig": map[string]any{"mode": tc.mode}},
				"tools":             []any{map[string]any{"googleSearch": map[string]any{}}},
			}
			if tc.functionTools {
				request["tools"] = []any{map[string]any{"functionDeclarations": []any{map[string]any{"name": "inspect_project", "parameters": map[string]any{"type": "OBJECT"}}}}}
			}
			original, err := json.Marshal(request)
			require.NoError(t, err)
			svc := &AntigravityGatewayService{}
			wrapped, err := svc.wrapV1InternalRequest("test-project", tc.model, original)
			require.NoError(t, err)
			var result struct {
				Request map[string]any `json:"request"`
			}
			require.NoError(t, json.Unmarshal(wrapped, &result))
			system, ok := result.Request["systemInstruction"].(map[string]any)
			require.True(t, ok)
			gotParts, ok := system["parts"].([]any)
			require.True(t, ok)
			if tc.wantReminder {
				require.Len(t, gotParts, 3, "the multi-turn progress requirement must be reinforced after the long context")
				part, ok := gotParts[2].(map[string]any)
				require.True(t, ok)
				reminder, ok := part["text"].(string)
				require.True(t, ok)
				require.Contains(t, reminder, "user-visible progress sentence before each batch of function calls")
				require.Contains(t, reminder, "normal assistant output text, not reasoning or function arguments")
				require.Contains(t, reminder, "Respect an explicit user request for silence or a strict output-only format")
				system["parts"] = gotParts[:2]
			}
			unchanged, err := json.Marshal(result.Request)
			require.NoError(t, err)
			require.JSONEq(t, string(original), string(unchanged), "caller instructions, history, tools, and mode must be preserved")
			if tc.wantReminder {
				system["parts"] = gotParts
				once, err := json.Marshal(result.Request)
				require.NoError(t, err)
				twice, err := svc.wrapV1InternalRequest("test-project", tc.model, once)
				require.NoError(t, err)
				var again struct {
					Request json.RawMessage `json:"request"`
				}
				require.NoError(t, json.Unmarshal(twice, &again))
				require.JSONEq(t, string(once), string(again.Request), "retry must not append duplicate reminders")
			}
		})
	}
}
