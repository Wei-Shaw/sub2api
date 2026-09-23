package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNaturalAccountTestPrompts(t *testing.T) {
	require.Len(t, naturalAccountTestPrompts, 100)

	seen := make(map[string]struct{}, len(naturalAccountTestPrompts))
	for _, prompt := range naturalAccountTestPrompts {
		require.NotEmpty(t, strings.TrimSpace(prompt))
		_, duplicated := seen[prompt]
		require.Falsef(t, duplicated, "duplicate test prompt: %q", prompt)
		seen[prompt] = struct{}{}
	}
}

func TestResolveAccountTestTextPromptPreservesExplicitPrompt(t *testing.T) {
	require.Equal(t, "A custom probe", resolveAccountTestTextPrompt("  A custom probe  "))
}

func TestResolveAccountTestTextPromptChoosesFromDictionary(t *testing.T) {
	for range 20 {
		prompt := resolveAccountTestTextPrompt("")
		require.Contains(t, naturalAccountTestPrompts, prompt)
	}
}
