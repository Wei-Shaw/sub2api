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

func TestDefaultModelsContainsOpus55(t *testing.T) {
	for _, model := range DefaultModels {
		if model.ID == "claude-opus-5-5" {
			if model.DisplayName != "Claude Opus 5.5" || model.CreatedAt != "2026-09-22T00:00:00Z" {
				t.Fatalf("unexpected Opus 5.5 descriptor: %+v", model)
			}
			return
		}
	}
	t.Fatal("claude-opus-5-5 missing")
}
