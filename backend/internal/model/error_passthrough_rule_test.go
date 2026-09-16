package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAllPlatformsIncludesEveryConcretePlatform(t *testing.T) {
	require.ElementsMatch(t, []string{
		"anthropic",
		"openai",
		"gemini",
		"antigravity",
		"kiro",
		"grok",
		"kimi",
		"zhipu",
		"deepseek",
		"minimax",
		"opencode_go",
	}, AllPlatforms())
}
