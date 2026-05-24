package admin

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitizeExtraUpstreamPassthrough_DropsUnknownKeys(t *testing.T) {
	extra := map[string]any{
		"upstream_passthrough": map[string]any{
			"profile":           "transparent",
			"category_override": "relay",
			"overrides": map[string]any{
				"forward_client_headers": true,
				"unknown_key":            true,        // must be dropped
				"skip_body_scrub":        "not-a-bool", // must be dropped
			},
			"unknown_top_level": "garbage", // must be dropped
		},
	}
	sanitizeExtraUpstreamPassthrough(extra)

	up, ok := extra["upstream_passthrough"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "transparent", up["profile"])
	require.Equal(t, "relay", up["category_override"])
	overrides, _ := up["overrides"].(map[string]any)
	require.Len(t, overrides, 1)
	require.Equal(t, true, overrides["forward_client_headers"])

	_, hasUnknown := up["unknown_top_level"]
	require.False(t, hasUnknown)
}

func TestSanitizeExtraUpstreamPassthrough_RejectsInvalidProfile(t *testing.T) {
	extra := map[string]any{
		"upstream_passthrough": map[string]any{
			"profile": "garbage-profile",
		},
	}
	sanitizeExtraUpstreamPassthrough(extra)
	up, _ := extra["upstream_passthrough"].(map[string]any)
	// invalid profile must be dropped (empty string is fine — means "inherit category default")
	_, present := up["profile"]
	require.False(t, present)
}

func TestSanitizeExtraUpstreamPassthrough_RejectsInvalidCategoryOverride(t *testing.T) {
	extra := map[string]any{
		"upstream_passthrough": map[string]any{
			"category_override": "not-a-category",
		},
	}
	sanitizeExtraUpstreamPassthrough(extra)
	up, _ := extra["upstream_passthrough"].(map[string]any)
	_, present := up["category_override"]
	require.False(t, present)
}

func TestSanitizeExtraUpstreamPassthrough_NilExtraSafe(t *testing.T) {
	sanitizeExtraUpstreamPassthrough(nil) // must not panic
}

func TestSanitizeExtraUpstreamPassthrough_AbsentKeyIsNoOp(t *testing.T) {
	extra := map[string]any{"other_field": 42}
	sanitizeExtraUpstreamPassthrough(extra)
	require.Equal(t, 42, extra["other_field"])
	_, present := extra["upstream_passthrough"]
	require.False(t, present)
}

func TestSanitizeExtraUpstreamPassthrough_AllInvalidRemovesEntireKey(t *testing.T) {
	// When every sub-field is dropped, the whole upstream_passthrough key
	// should be removed (don't leave an empty map sitting in extra).
	extra := map[string]any{
		"upstream_passthrough": map[string]any{
			"profile":           "bad",
			"category_override": "also-bad",
			"overrides":         map[string]any{"unknown": true},
			"junk":              "more-junk",
		},
	}
	sanitizeExtraUpstreamPassthrough(extra)
	_, present := extra["upstream_passthrough"]
	require.False(t, present, "all-invalid input must remove the upstream_passthrough key entirely")
}
