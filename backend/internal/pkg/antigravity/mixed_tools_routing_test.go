package antigravity

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// #7080: mixed search+functions drops the search builtin (upstream rejects
// the mix even with the flag) and keeps function declarations. Pure search
// is preserved.
func TestBuildTools_MixedSearchDropped(t *testing.T) {
	mixed := []ClaudeTool{
		{Name: "bash", InputSchema: map[string]any{"type": "object"}},
		{Type: "web_search_20250305", Name: "web_search"},
	}

	t.Run("mixed drops search keeps functions", func(t *testing.T) {
		decls := buildTools(mixed)
		require.Len(t, decls, 1)
		require.NotEmpty(t, decls[0].FunctionDeclarations)
		require.Nil(t, decls[0].GoogleSearch)
		require.False(t, hasMixedToolInvocations(decls))
	})

	t.Run("pure search preserved", func(t *testing.T) {
		decls := buildTools([]ClaudeTool{
			{Type: "web_search_20250305", Name: "web_search"},
		})
		require.Len(t, decls, 1)
		require.NotNil(t, decls[0].GoogleSearch)
	})

	t.Run("hasFunctionToolsForSearchRouting", func(t *testing.T) {
		require.True(t, hasFunctionToolsForSearchRouting(mixed))
		require.False(t, hasFunctionToolsForSearchRouting([]ClaudeTool{
			{Type: "web_search_20250305", Name: "web_search"},
		}))
		require.False(t, hasFunctionToolsForSearchRouting(nil))
	})
}
