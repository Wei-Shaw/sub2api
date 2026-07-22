package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestFilterSchedulerExtraPreservesReasoningEffortPreferences(t *testing.T) {
	preferences := []any{"low", "high"}
	filtered := filterSchedulerExtra(map[string]any{
		service.OpenAIReasoningEffortPreferencesExtraKey: preferences,
		"unrelated_setting": true,
	})

	require.Equal(t, preferences, filtered[service.OpenAIReasoningEffortPreferencesExtraKey])
	require.NotContains(t, filtered, "unrelated_setting")
}
