package repository

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFilterSchedulerExtraKeepsContextLengthFilter(t *testing.T) {
	filtered := filterSchedulerExtra(map[string]any{
		"context_length_filter": map[string]any{
			"enabled":       true,
			"mode":          "gt",
			"threshold":     5000,
			"allow_percent": 50,
		},
		"unused_large_field": "drop-me",
	})

	require.Equal(t, map[string]any{
		"enabled":       true,
		"mode":          "gt",
		"threshold":     5000,
		"allow_percent": 50,
	}, filtered["context_length_filter"])
	require.NotContains(t, filtered, "unused_large_field")
}
