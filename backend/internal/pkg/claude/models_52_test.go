package claude

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClaude52CatalogAndCapabilities(t *testing.T) {
	for _, family := range []string{"fable", "mythos"} {
		id := "claude-" + family + "-5-2"
		t.Run(id, func(t *testing.T) {
			count := 0
			for _, model := range DefaultModels {
				if model.ID == id {
					count++
					require.Equal(t, "model", model.Type)
					require.Equal(t, "2026-11-09T00:00:00Z", model.CreatedAt)
				}
			}
			require.Equal(t, 1, count)
			for _, variant := range []string{id, "anthropic/" + id, "global.anthropic." + id, id + "-thinking", id + "-20261109"} {
				require.Equal(t, []string{"low", "medium", "high", "xhigh", "max"}, EffortLevelsForModel(variant))
				require.Equal(t, family == "fable", IsFable52(variant))
				require.Equal(t, family == "mythos", IsMythos52(variant))
			}
		})
	}
	for _, model := range []string{"claude-fable-5-1", "claude-fable-5-20", "claude-fable-5-2-preview", "claude-mythos-5-20", "claude-mythos-5-2-preview"} {
		require.False(t, IsFable52(model))
		require.False(t, IsMythos52(model))
	}
}

func TestCLICurrentVersionSatisfiesFable52Gate(t *testing.T) {
	// https://code.claude.com/docs/en/model-config
	require.GreaterOrEqual(t, compareSemver(t, CLICurrentVersion, "2.1.321"), 0)
}
