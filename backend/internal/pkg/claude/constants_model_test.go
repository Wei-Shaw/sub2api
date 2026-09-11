package claude

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultModelsContainsClaudeFable51(t *testing.T) {
	t.Parallel()

	for _, model := range DefaultModels {
		if model.ID == "claude-fable-5-1" {
			require.Equal(t, "Claude Fable 5.1", model.DisplayName)
			require.Equal(t, "2026-09-01T00:00:00Z", model.CreatedAt)
			return
		}
	}
	t.Fatal("claude-fable-5-1 missing from DefaultModels")
}
