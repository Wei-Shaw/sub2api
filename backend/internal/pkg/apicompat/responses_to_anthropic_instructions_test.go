package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponsesToAnthropicRequest_Instructions(t *testing.T) {
	t.Run("instructions_becomes_system", func(t *testing.T) {
		req := &ResponsesRequest{
			Model:        "claude-sonnet-4-20250514",
			Instructions: "You are a helpful assistant.",
			Input:        json.RawMessage(`[{"role":"user","content":"hello"}]`),
		}

		result, err := ResponsesToAnthropicRequest(req)
		require.NoError(t, err)

		var system string
		require.NoError(t, json.Unmarshal(result.System, &system))
		assert.Equal(t, "You are a helpful assistant.", system)
		assert.NotEmpty(t, result.Messages)
	})

	t.Run("empty_instructions_no_system", func(t *testing.T) {
		req := &ResponsesRequest{
			Model: "claude-sonnet-4-20250514",
			Input: json.RawMessage(`[{"role":"user","content":"hello"}]`),
		}

		result, err := ResponsesToAnthropicRequest(req)
		require.NoError(t, err)
		assert.Nil(t, result.System)
	})

	t.Run("instructions_and_system_item_concatenated", func(t *testing.T) {
		req := &ResponsesRequest{
			Model:        "claude-sonnet-4-20250514",
			Instructions: "Top-level instruction.",
			Input: json.RawMessage(`[
				{"role":"system","content":"Input-level system prompt."},
				{"role":"user","content":"hello"}
			]`),
		}

		result, err := ResponsesToAnthropicRequest(req)
		require.NoError(t, err)

		var system string
		require.NoError(t, json.Unmarshal(result.System, &system))
		assert.Contains(t, system, "Top-level instruction.")
		assert.Contains(t, system, "Input-level system prompt.")
	})

	t.Run("instructions_with_string_input", func(t *testing.T) {
		req := &ResponsesRequest{
			Model:        "claude-sonnet-4-20250514",
			Instructions: "Be concise.",
			Input:        json.RawMessage(`"What is Go?"`),
		}

		result, err := ResponsesToAnthropicRequest(req)
		require.NoError(t, err)

		var system string
		require.NoError(t, json.Unmarshal(result.System, &system))
		assert.Equal(t, "Be concise.", system)
		require.Len(t, result.Messages, 1)
		assert.Equal(t, "user", result.Messages[0].Role)
	})
}

func TestConvertResponsesInputToAnthropic_DeveloperRole(t *testing.T) {
	t.Run("developer_becomes_system", func(t *testing.T) {
		input := `[
			{"role":"developer","content":[{"type":"input_text","text":"You are a code reviewer."}]},
			{"role":"user","content":"review this code"}
		]`

		system, messages, err := convertResponsesInputToAnthropic("", json.RawMessage(input))
		require.NoError(t, err)

		var systemText string
		require.NoError(t, json.Unmarshal(system, &systemText))
		assert.Equal(t, "You are a code reviewer.", systemText)

		require.Len(t, messages, 1)
		assert.Equal(t, "user", messages[0].Role)
	})

	t.Run("developer_does_not_become_user", func(t *testing.T) {
		input := `[
			{"role":"developer","content":[{"type":"input_text","text":"System prompt."}]},
			{"role":"user","content":"hi"}
		]`

		_, messages, err := convertResponsesInputToAnthropic("", json.RawMessage(input))
		require.NoError(t, err)

		for _, m := range messages {
			if m.Role == "user" {
				var s string
				if json.Unmarshal(m.Content, &s) == nil {
					assert.NotContains(t, s, "System prompt.")
				}
			}
		}
	})

	t.Run("instructions_and_developer_concatenated_in_order", func(t *testing.T) {
		input := `[
			{"role":"developer","content":"Extra context."},
			{"role":"user","content":"hello"}
		]`

		system, _, err := convertResponsesInputToAnthropic("Main instruction.", json.RawMessage(input))
		require.NoError(t, err)

		var systemText string
		require.NoError(t, json.Unmarshal(system, &systemText))
		assert.Equal(t, "Main instruction.\n\nExtra context.", systemText)
	})
}
