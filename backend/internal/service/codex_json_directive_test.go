package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// #7027: json_object mode must not duplicate the full system prompt.
func TestEnsureMinimalJSONDirectiveInCodexInput(t *testing.T) {
	t.Run("injects minimal directive", func(t *testing.T) {
		body := map[string]any{
			"input": []any{
				map[string]any{"role": "user", "content": "hello"},
			},
		}
		ensureMinimalJSONDirectiveInCodexInput(body)
		input, ok := body["input"].([]any)
		require.True(t, ok)
		require.Len(t, input, 2)
		first, ok := input[0].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "developer", first["role"])
		require.Contains(t, first["content"], "JSON")
	})

	t.Run("does not duplicate existing json hint", func(t *testing.T) {
		body := map[string]any{
			"input": []any{
				map[string]any{"role": "developer", "content": "Return a valid JSON object."},
				map[string]any{"role": "user", "content": "hello"},
			},
		}
		ensureMinimalJSONDirectiveInCodexInput(body)
		require.Len(t, body["input"], 2)
	})

	t.Run("nil safe", func(t *testing.T) {
		require.NotPanics(t, func() { ensureMinimalJSONDirectiveInCodexInput(nil) })
		require.NotPanics(t, func() { ensureMinimalJSONDirectiveInCodexInput(map[string]any{}) })
	})
}
